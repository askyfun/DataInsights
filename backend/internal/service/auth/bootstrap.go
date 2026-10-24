package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"

	"data-insights/internal/database"
	"data-insights/internal/model"
	"data-insights/internal/response"
	"data-insights/internal/router"
)

// bootstrapTokenTTL bounds the one-time bootstrap secret printed at startup.
const bootstrapTokenTTL = 30 * time.Minute

// bootstrapAdvisoryLock serializes every bootstrap path (token registration and
// the ADMIN_USERNAME/ADMIN_PASSWORD escape hatch) on the same PostgreSQL lock,
// so two concurrent first-boot requests cannot both observe "no admin".
const bootstrapAdvisoryLock int64 = 0x646961323136 // "dia216"

const (
	settingBootstrapHash     = "bootstrap_token_hash"
	settingBootstrapExpiry   = "bootstrap_token_expires_at"
	settingAdminRecoveredUse = "admin_recover_used_hash"
)

// EnforceModeOff is returned by BootstrapStatus until AUTH_ENFORCE (R-92)
// exists: only the auth/token endpoints require a bearer token today.
const EnforceModeOff = "off"

// BootstrapStatus tells the UI whether it should render first-run setup instead
// of login. needs_bootstrap is true only while no administrator row exists —
// disabled (soft-deleted) admins still keep registration closed.
func (s *authService) BootstrapStatus(ctx context.Context) (bool, string, error) {
	n, err := countAdmins(ctx, s.db)
	if err != nil {
		return false, "", err
	}
	return n == 0, EnforceModeOff, nil
}

// EnsureBootstrap runs once at startup. With ADMIN_USERNAME + ADMIN_PASSWORD it
// idempotently creates the first admin and skips the printed token; otherwise,
// when no admin exists yet, it issues a fresh 30-minute bootstrap token and
// returns it so main can print it exactly once.
func (s *authService) EnsureBootstrap(ctx context.Context) (string, bool, error) {
	user, pass := s.adminCfg.Username, s.adminCfg.Password
	if user != "" && pass != "" {
		if err := s.ensureAdminFromEnv(ctx, user, pass); err != nil {
			return "", false, err
		}
		return "", false, nil
	}
	if user != "" || pass != "" {
		slog.Warn("ADMIN_USERNAME and ADMIN_PASSWORD must both be set; ignoring the partial pair")
	}

	admins, err := countAdmins(ctx, s.db)
	if err != nil {
		return "", false, err
	}
	if admins > 0 {
		return "", false, nil
	}

	raw, err := newRawToken("boot")
	if err != nil {
		return "", false, err
	}
	if err := database.WithTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw("SELECT pg_advisory_xact_lock(?)", bootstrapAdvisoryLock).Exec(ctx); err != nil {
			return fmt.Errorf("acquire bootstrap lock: %w", err)
		}
		n, err := countAdmins(ctx, tx)
		if err != nil {
			return err
		}
		if n > 0 {
			// Another instance created the admin while we waited.
			return errBootstrapAlreadyDone
		}
		if err := settingSet(ctx, tx, settingBootstrapHash, hashToken(raw)); err != nil {
			return err
		}
		return settingSet(ctx, tx, settingBootstrapExpiry, time.Now().Add(bootstrapTokenTTL).UTC().Format(time.RFC3339))
	}); err != nil {
		if errors.Is(err, errBootstrapAlreadyDone) {
			return "", false, nil
		}
		return "", false, err
	}
	return raw, true, nil
}

var errBootstrapAlreadyDone = errors.New("bootstrap already completed")

func (s *authService) ensureAdminFromEnv(ctx context.Context, username, password string) error {
	return database.WithTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw("SELECT pg_advisory_xact_lock(?)", bootstrapAdvisoryLock).Exec(ctx); err != nil {
			return fmt.Errorf("acquire bootstrap lock: %w", err)
		}
		n, err := countAdmins(ctx, tx)
		if err != nil {
			return err
		}
		if n > 0 {
			slog.Info("ADMIN_USERNAME is set but an administrator already exists; leaving it untouched")
			return nil
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}
		u := &model.User{
			Username:     username,
			PasswordHash: string(hash),
			Role:         model.RoleAdmin,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		if _, err := tx.NewInsert().Model(u).Exec(ctx); err != nil {
			return fmt.Errorf("insert admin from env: %w", err)
		}
		if err := adoptSystemOwnedRows(ctx, tx, u.ID); err != nil {
			return err
		}
		return clearBootstrapSettings(ctx, tx)
	})
}

// RecoverAdmin resets the first live administrator's password. The configured
// ADMIN_RESET_TOKEN is single-use: a successful recovery records its hash and
// refuses the same token again, so a leaked env value cannot reset repeatedly.
func (s *authService) RecoverAdmin(ctx context.Context, resetToken, newPassword string) error {
	configured := s.adminCfg.ResetToken
	if configured == "" || resetToken == "" ||
		subtle.ConstantTimeCompare([]byte(resetToken), []byte(configured)) != 1 {
		return router.NewBusinessError(response.CodeUnauthorized, "invalid admin reset token")
	}
	if newPassword == "" {
		return router.NewBusinessError(response.CodeBadRequest, "new password is required")
	}

	usedHash := hashToken(resetToken)
	err := database.WithTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		stored, found, err := settingGet(ctx, tx, settingAdminRecoveredUse)
		if err != nil {
			return err
		}
		if found && subtle.ConstantTimeCompare([]byte(stored), []byte(usedHash)) == 1 {
			return router.NewBusinessError(response.CodeUnauthorized, "admin reset token already used")
		}

		u := &model.User{}
		err = tx.NewSelect().Model(u).
			Where("role = ?", model.RoleAdmin).
			Where("deleted_at IS NULL").
			OrderExpr("id ASC").
			Limit(1).
			Scan(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return router.NewBusinessError(response.CodeNotFound, "no active administrator to recover")
		}
		if err != nil {
			return fmt.Errorf("load administrator: %w", err)
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}
		if _, err := tx.NewUpdate().Model((*model.User)(nil)).
			Set("password_hash = ?", string(hash)).
			Set("updated_at = ?", time.Now()).
			Where("id = ?", u.ID).
			Exec(ctx); err != nil {
			return fmt.Errorf("update administrator password: %w", err)
		}
		// A password reset must not leave previously issued sessions alive.
		if _, err := tx.NewUpdate().Model((*model.Token)(nil)).
			Set("revoked_at = ?", time.Now()).
			Where("user_id = ?", u.ID).
			Where("revoked_at IS NULL").
			Exec(ctx); err != nil {
			return fmt.Errorf("revoke administrator sessions: %w", err)
		}
		if err := settingSet(ctx, tx, settingAdminRecoveredUse, usedHash); err != nil {
			return err
		}
		slog.Info("administrator password recovered", "user_id", u.ID, "username", u.Username)
		return nil
	})
	if err != nil {
		var be router.BusinessError
		if errors.As(err, &be) {
			return be
		}
		return err
	}
	return nil
}

// validateBootstrapToken rejects a missing, expired or mismatched token. The
// message is deliberately distinct from login's "invalid username or password".
func validateBootstrapToken(ctx context.Context, db bun.IDB, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return router.NewBusinessError(response.CodeUnauthorized, "bootstrap token is required")
	}
	stored, found, err := settingGet(ctx, db, settingBootstrapHash)
	if err != nil {
		return err
	}
	expiresRaw, expiresFound, err := settingGet(ctx, db, settingBootstrapExpiry)
	if err != nil {
		return err
	}
	if !found || !expiresFound {
		return router.NewBusinessError(response.CodeUnauthorized, "bootstrap token invalid or expired")
	}
	expiresAt, err := time.Parse(time.RFC3339, expiresRaw)
	if err != nil || time.Now().After(expiresAt) {
		return router.NewBusinessError(response.CodeUnauthorized, "bootstrap token invalid or expired")
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(hashToken(raw))) != 1 {
		return router.NewBusinessError(response.CodeUnauthorized, "bootstrap token invalid or expired")
	}
	return nil
}

func countAdmins(ctx context.Context, db bun.IDB) (int, error) {
	n, err := db.NewSelect().Model((*model.User)(nil)).
		Where("role = ?", model.RoleAdmin).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count admins: %w", err)
	}
	return n, nil
}

func adoptSystemOwnedRows(ctx context.Context, tx bun.Tx, adminID int) error {
	owner := int(model.SystemOwnerID)
	queries := []*bun.UpdateQuery{
		tx.NewUpdate().Model((*model.Datasource)(nil)),
		tx.NewUpdate().Model((*model.Dataset)(nil)),
		tx.NewUpdate().Model((*model.Chart)(nil)),
	}
	for _, q := range queries {
		if _, err := q.Set("owner_id = ?", adminID).
			Where("owner_id = ?", owner).
			Exec(ctx); err != nil {
			return fmt.Errorf("adopt owner_id: %w", err)
		}
	}
	return nil
}

func clearBootstrapSettings(ctx context.Context, db bun.IDB) error {
	for _, key := range []string{settingBootstrapHash, settingBootstrapExpiry} {
		if _, err := db.NewRaw("DELETE FROM bi_setting WHERE key = ?", key).Exec(ctx); err != nil {
			return fmt.Errorf("clear %s: %w", key, err)
		}
	}
	return nil
}

func settingGet(ctx context.Context, db bun.IDB, key string) (string, bool, error) {
	var value string
	err := db.NewRaw("SELECT value FROM bi_setting WHERE key = ?", key).Scan(ctx, &value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read setting %s: %w", key, err)
	}
	return value, true, nil
}

func settingSet(ctx context.Context, db bun.IDB, key, value string) error {
	_, err := db.NewRaw(`INSERT INTO bi_setting (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, key, value).Exec(ctx)
	if err != nil {
		return fmt.Errorf("write setting %s: %w", key, err)
	}
	return nil
}
