// Package auth implements R-82 (issue #183): the account system and the single
// opaque-token validator that BOTH the human session token and the machine PAT
// flow through (issue #184).
//
// Tokens are opaque random secrets; the DB stores only sha256(raw) plus a short
// non-secret Prefix. The plaintext is returned exactly once at issuance. Because
// validation is a live DB lookup (no signing, no cache), revocation via
// revoked_at takes effect on the next request with no silent window.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"

	"data-insights/internal/database"
	"data-insights/internal/domain/entity"
	"data-insights/internal/model"
	"data-insights/internal/response"
	"data-insights/internal/router"
)

// sessionTokenTTL is the 24h lifetime of a human session token (#183/#184).
const sessionTokenTTL = 24 * time.Hour

// defaultPATTTL is the default lifetime of a machine PAT. "Never expires" is an
// explicit opt-in (never=true), matching #184 acceptance ("默认 90 天；支持长期/不过期
// 但需显式勾选并给出风险提示").
const defaultPATTTL = 90 * 24 * time.Hour

// ErrInvalidCredentials collapses "unknown user" and "wrong password" into one
// message so login does not leak which usernames exist.
var errInvalidCredentials = router.NewBusinessError(response.CodeUnauthorized, "invalid username or password")

// Principal is the authenticated caller resolved from a bearer token. Kind
// distinguishes a human session from a machine PAT so the middleware can deny
// PATs on user-management endpoints (#184 acceptance 3).
type Principal struct {
	UserID   int
	Username string
	Role     string
	Kind     string // model.TokenKindSession | model.TokenKindPAT
}

// AdminConfig carries the startup escape hatches for the first administrator.
// All three are optional; an empty value means "not configured".
type AdminConfig struct {
	Username   string
	Password   string
	ResetToken string
}

// Service is the auth use-case surface consumed by handlers and the middleware.
type Service interface {
	Register(ctx context.Context, username, password, bootstrapToken, ip, ua string) (*entity.AuthResult, error)
	Login(ctx context.Context, username, password, ip, ua string) (*entity.AuthResult, error)
	Verify(ctx context.Context, rawToken, ip, ua string) (*Principal, error)
	Revoke(ctx context.Context, rawToken string) error
	CurrentUser(ctx context.Context, userID int) (*entity.User, error)
	CreatePAT(ctx context.Context, userID int, name string, never bool) (*entity.PATCreateResult, error)
	ListPATs(ctx context.Context, userID int) ([]entity.TokenInfo, error)
	RevokeByID(ctx context.Context, userID, id int) error
	EnsureBootstrap(ctx context.Context) (string, bool, error)
	BootstrapStatus(ctx context.Context) (bool, string, error)
	RecoverAdmin(ctx context.Context, resetToken, newPassword string) error
}

type authService struct {
	db       *bun.DB
	adminCfg AdminConfig
}

// NewService wires the bun-backed auth service.
func NewService(db *bun.DB, admin AdminConfig) Service {
	return &authService{db: db, adminCfg: admin}
}

// --- token primitives (pure, unit-tested) ---

// newRawToken builds a prefixed opaque secret: "di_<kind>_" + 32 random bytes
// (base64url, no padding). The kind prefix lets a bearer string self-describe.
func newRawToken(kind string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return "di_" + kind + "_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// tokenPrefix is a short NON-secret head for recognition in listings: the kind
// marker plus the first 6 secret chars. Enough to tell two tokens apart, far
// too little to reconstruct the secret.
func tokenPrefix(raw string) string {
	marker := "di_pat_"
	if len(raw) >= len("di_session_") && raw[:len("di_session_")] == "di_session_" {
		marker = "di_session_"
	}
	end := len(marker) + 6
	if end > len(raw) {
		end = len(raw)
	}
	return raw[:end]
}

func toUserEntity(m *model.User) *entity.User {
	return &entity.User{
		ID:        m.ID,
		Username:  m.Username,
		Role:      m.Role,
		CreatedAt: m.CreatedAt.Format(time.RFC3339),
	}
}

// --- lifecycle ---

// Register creates the FIRST administrator as admin and closes registration
// afterwards. It requires the one-time bootstrap token printed at startup; an
// advisory lock makes concurrent first-boot requests converge on one winner.
// On success it also adopts every row still stamped with the L0 placeholder
// owner (model.SystemOwnerID) onto the new admin — the R-22 存量归属衔接
// (#183 acceptance 6). All of this is one transaction so a partially-applied
// bootstrap cannot be observed.
func (s *authService) Register(ctx context.Context, username, password, bootstrapToken, ip, ua string) (*entity.AuthResult, error) {
	if username == "" || password == "" {
		return nil, router.NewBusinessError(response.CodeBadRequest, "username and password are required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	var result *entity.AuthResult
	err = database.WithTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw("SELECT pg_advisory_xact_lock(?)", bootstrapAdvisoryLock).Exec(ctx); err != nil {
			return fmt.Errorf("acquire bootstrap lock: %w", err)
		}
		// Closed once ANY admin row exists, including disabled ones: disabling
		// the last administrator must not reopen self-service registration.
		admins, err := countAdmins(ctx, tx)
		if err != nil {
			return err
		}
		if admins > 0 {
			return router.NewBusinessError(response.CodeUnauthorized, "注册已关闭：系统管理员已建立")
		}
		if err := validateBootstrapToken(ctx, tx, bootstrapToken); err != nil {
			return err
		}
		// Usernames stay reserved after soft delete (unique index covers all
		// rows), otherwise a deleted admin's name could be silently re-used.
		reused, err := tx.NewSelect().Model((*model.User)(nil)).
			Where("username = ?", username).Count(ctx)
		if err != nil {
			return fmt.Errorf("check username: %w", err)
		}
		if reused > 0 {
			return router.NewBusinessError(response.CodeBadRequest, "username already used")
		}

		u := &model.User{
			Username:     username,
			PasswordHash: string(hash),
			Role:         model.RoleAdmin,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		if _, err := tx.NewInsert().Model(u).Returning("id").Exec(ctx); err != nil {
			return fmt.Errorf("insert user: %w", err)
		}
		if err := adoptSystemOwnedRows(ctx, tx, u.ID); err != nil {
			return err
		}

		res, err := s.issueSession(ctx, tx, u, ip, ua)
		if err != nil {
			return err
		}
		result = res
		return clearBootstrapSettings(ctx, tx)
	})
	if err != nil {
		// WithTx wraps the closure error; business errors must reach the router
		// UNWRAPPED so they map to their own code, not 50000.
		var be router.BusinessError
		if errors.As(err, &be) {
			return nil, be
		}
		return nil, err
	}
	return result, nil
}

func (s *authService) Login(ctx context.Context, username, password, ip, ua string) (*entity.AuthResult, error) {
	if username == "" || password == "" {
		return nil, errInvalidCredentials
	}
	u := &model.User{}
	err := s.db.NewSelect().Model(u).
		Where("username = ?", username).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("lookup user: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return nil, errInvalidCredentials
	}
	return s.issueSession(ctx, s.db, u, ip, ua)
}

// issueSession persists a fresh 24h session token and returns the payload with
// the plaintext token (shown once). exec accepts either *bun.DB or bun.Tx
// (both satisfy bun.IDB), so Register can issue inside its bootstrap tx.
func (s *authService) issueSession(ctx context.Context, exec bun.IDB, u *model.User, ip, ua string) (*entity.AuthResult, error) {
	raw, err := newRawToken(model.TokenKindSession)
	if err != nil {
		return nil, err
	}
	expires := time.Now().Add(sessionTokenTTL)
	tok := &model.Token{
		UserID:     u.ID,
		Kind:       model.TokenKindSession,
		TokenHash:  hashToken(raw),
		Prefix:     tokenPrefix(raw),
		ExpiresAt:  sql.NullTime{Time: expires, Valid: true},
		LastUsedAt: sql.NullTime{Time: time.Now(), Valid: true},
		LastUsedIP: sql.NullString{String: ip, Valid: ip != ""},
		LastUsedUA: sql.NullString{String: ua, Valid: ua != ""},
		CreatedAt:  time.Now(),
	}
	if _, err := exec.NewInsert().Model(tok).Exec(ctx); err != nil {
		return nil, fmt.Errorf("insert session token: %w", err)
	}
	return &entity.AuthResult{
		Token:     raw,
		Kind:      model.TokenKindSession,
		ExpiresAt: expires.Format(time.RFC3339),
		User:      *toUserEntity(u),
	}, nil
}

// Verify resolves a bearer token to its principal. It is the single choke point
// shared by session tokens and PATs (#184 acceptance 3). The last_used_* columns
// are refreshed best-effort: a failure to record usage must not reject a valid
// token.
func (s *authService) Verify(ctx context.Context, rawToken, ip, ua string) (*Principal, error) {
	if rawToken == "" {
		return nil, router.NewBusinessError(response.CodeUnauthorized, "missing bearer token")
	}
	tok := &model.Token{}
	err := s.db.NewSelect().Model(tok).
		Where("token_hash = ?", hashToken(rawToken)).
		Where("revoked_at IS NULL").
		Where("(expires_at IS NULL OR expires_at > ?)", time.Now()).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, router.NewBusinessError(response.CodeUnauthorized, "invalid or expired token")
	}
	if err != nil {
		return nil, fmt.Errorf("lookup token: %w", err)
	}
	u := &model.User{}
	err = s.db.NewSelect().Model(u).
		Where("id = ?", tok.UserID).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, router.NewBusinessError(response.CodeUnauthorized, "invalid token: user gone")
	}
	if err != nil {
		return nil, fmt.Errorf("lookup user: %w", err)
	}

	if _, err := s.db.NewUpdate().Model((*model.Token)(nil)).
		Set("last_used_at = ?", time.Now()).
		Set("last_used_ip = ?", ip).
		Set("last_used_ua = ?", ua).
		Where("id = ?", tok.ID).
		Exec(ctx); err != nil {
		// Recording usage is advisory; never fail an otherwise-valid token on it.
		_ = err
	}

	return &Principal{
		UserID:   u.ID,
		Username: u.Username,
		Role:     u.Role,
		Kind:     tok.Kind,
	}, nil
}

// Revoke stamps revoked_at on the token identified by its plaintext (idempotent:
// revoking an already-revoked/unknown token is a no-op, not an error).
func (s *authService) Revoke(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	if _, err := s.db.NewUpdate().Model((*model.Token)(nil)).
		Set("revoked_at = ?", time.Now()).
		Where("token_hash = ?", hashToken(rawToken)).
		Where("revoked_at IS NULL").
		Exec(ctx); err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	return nil
}

func (s *authService) CurrentUser(ctx context.Context, userID int) (*entity.User, error) {
	u := &model.User{}
	err := s.db.NewSelect().Model(u).
		Where("id = ?", userID).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, router.NewBusinessError(response.CodeNotFound, "user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("load user: %w", err)
	}
	return toUserEntity(u), nil
}

// --- PAT management (issue #184) ---

// tokenInfoOf projects a stored token to its API shape. It deliberately drops
// token_hash: only the short prefix and metadata leave the service.
func tokenInfoOf(t *model.Token) entity.TokenInfo {
	info := entity.TokenInfo{
		ID:        t.ID,
		Name:      t.Name.String,
		Prefix:    t.Prefix,
		CreatedAt: t.CreatedAt.Format(time.RFC3339),
		Revoked:   t.RevokedAt.Valid,
	}
	if t.ExpiresAt.Valid {
		info.ExpiresAt = t.ExpiresAt.Time.Format(time.RFC3339)
	}
	if t.LastUsedAt.Valid {
		info.LastUsedAt = t.LastUsedAt.Time.Format(time.RFC3339)
	}
	info.LastUsedIP = t.LastUsedIP.String
	info.LastUsedUA = t.LastUsedUA.String
	return info
}

// CreatePAT mints a long-lived machine token. The plaintext is returned once;
// only its sha256 + prefix are stored (#184 acceptance 2).
func (s *authService) CreatePAT(ctx context.Context, userID int, name string, never bool) (*entity.PATCreateResult, error) {
	raw, err := newRawToken(model.TokenKindPAT)
	if err != nil {
		return nil, err
	}
	tok := &model.Token{
		UserID:    userID,
		Kind:      model.TokenKindPAT,
		Name:      sql.NullString{String: name, Valid: name != ""},
		TokenHash: hashToken(raw),
		Prefix:    tokenPrefix(raw),
		CreatedAt: time.Now(),
	}
	if !never {
		tok.ExpiresAt = sql.NullTime{Time: time.Now().Add(defaultPATTTL), Valid: true}
	}
	if _, err := s.db.NewInsert().Model(tok).Exec(ctx); err != nil {
		return nil, fmt.Errorf("create PAT: %w", err)
	}
	return &entity.PATCreateResult{Token: raw, Info: tokenInfoOf(tok)}, nil
}

// ListPATs returns the user's PATs (revoked ones included, flagged) newest-first.
func (s *authService) ListPATs(ctx context.Context, userID int) ([]entity.TokenInfo, error) {
	var toks []model.Token
	err := s.db.NewSelect().Model(&toks).
		Where("user_id = ?", userID).
		Where("kind = ?", model.TokenKindPAT).
		OrderExpr("created_at DESC, id DESC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list PATs: %w", err)
	}
	out := make([]entity.TokenInfo, 0, len(toks))
	for i := range toks {
		out = append(out, tokenInfoOf(&toks[i]))
	}
	return out, nil
}

// RevokeByID stamps revoked_at on the user's own PAT (idempotent). It is scoped
// by user_id so one user can never revoke another's token.
func (s *authService) RevokeByID(ctx context.Context, userID, id int) error {
	if _, err := s.db.NewUpdate().Model((*model.Token)(nil)).
		Set("revoked_at = ?", time.Now()).
		Where("id = ?", id).
		Where("user_id = ?", userID).
		Where("kind = ?", model.TokenKindPAT).
		Where("revoked_at IS NULL").
		Exec(ctx); err != nil {
		return fmt.Errorf("revoke PAT: %w", err)
	}
	return nil
}
