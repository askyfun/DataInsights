package auth

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"golang.org/x/crypto/bcrypt"

	"data-insights/internal/router"
)

// --- pure token primitives ---

func TestNewRawToken_FormatAndUniqueness(t *testing.T) {
	a, err := newRawToken("session")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := newRawToken("pat")
	if !strings.HasPrefix(a, "di_session_") {
		t.Fatalf("session token must carry kind prefix, got %s", a)
	}
	if !strings.HasPrefix(b, "di_pat_") {
		t.Fatalf("pat token must carry kind prefix, got %s", b)
	}
	if a == b {
		t.Fatal("two tokens must differ")
	}
}

func TestHashToken_OneWayAndDeterministic(t *testing.T) {
	raw := "di_pat_s3cr3tvalue"
	h := hashToken(raw)
	if h == raw {
		t.Fatal("hash must not equal plaintext (no plaintext at rest)")
	}
	if len(h) != 64 {
		t.Fatalf("sha256 hex must be 64 chars, got %d", len(h))
	}
	if hashToken(raw) != h {
		t.Fatal("hash must be deterministic")
	}
}

func TestTokenPrefix_HeadOnly(t *testing.T) {
	raw := "di_pat_ABCDEFGHIJKLMNOP"
	p := tokenPrefix(raw)
	if !strings.HasPrefix(p, "di_pat_ABCDEF") {
		t.Fatalf("prefix should be marker+6 secret chars, got %s", p)
	}
	if p == raw {
		t.Fatal("prefix must be a strict head, never the whole token")
	}
}

func TestBcrypt_RoundTrip(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.DefaultCost)
	if bcrypt.CompareHashAndPassword(hash, []byte("hunter2")) != nil {
		t.Fatal("correct password must verify")
	}
	if bcrypt.CompareHashAndPassword(hash, []byte("wrong")) == nil {
		t.Fatal("wrong password must not verify")
	}
}

// --- Register: bootstrap the first admin ---

func newMockAuth(t *testing.T) (*authService, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &authService{db: bun.NewDB(sqlDB, pgdialect.New())}, mock
}

func TestRegister_BootstrapsAdminAndAdoptsOwner(t *testing.T) {
	s, mock := newMockAuth(t)
	// tx begin → count live users = 0
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_user"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// insert admin, RETURNING id → id = 1 (bun runs INSERT-with-Returning as a Query).
	mock.ExpectQuery(`INSERT INTO "bi_user".*RETURNING id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	// 存量归属衔接: three owner_id adoptions (bun aliases the model struct).
	mock.ExpectExec(`UPDATE "bi_datasource" AS "datasource" SET owner_id`).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(`UPDATE "bi_dataset" AS "dataset" SET owner_id`).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`UPDATE "bi_chart" AS "chart" SET owner_id`).WillReturnResult(sqlmock.NewResult(0, 1))
	// session token insert (also RETURNING → Query), then commit.
	mock.ExpectQuery(`INSERT INTO "bi_token".*RETURNING`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "revoked_at"}).AddRow(1, nil))
	mock.ExpectCommit()

	res, err := s.Register(context.Background(), "alice", "pw-secret", "1.2.3.4", "curl")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.Kind != "session" || !strings.HasPrefix(res.Token, "di_session_") {
		t.Fatalf("expected session token, got %+v", res)
	}
	if res.User.Role != "admin" {
		t.Fatalf("first user must be admin, got %q", res.User.Role)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestRegister_ClosedAfterFirstUser(t *testing.T) {
	s, mock := newMockAuth(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_user"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	_, err := s.Register(context.Background(), "bob", "pw", "", "")
	if err == nil {
		t.Fatal("second registration must be rejected")
	}
	var be router.BusinessError
	if !errors.As(err, &be) || be.Code != 20200 {
		t.Fatalf("want 20200 business error, got %#v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

// --- Login ---

func TestLogin_OK(t *testing.T) {
	s, mock := newMockAuth(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.DefaultCost)
	mock.ExpectQuery(`SELECT .* FROM "bi_user"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password_hash", "role", "created_at", "updated_at"}).
			AddRow(1, "alice", string(hash), "admin", time.Now(), time.Now()))
	mock.ExpectQuery(`INSERT INTO "bi_token".*RETURNING`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "revoked_at"}).AddRow(1, nil))

	res, err := s.Login(context.Background(), "alice", "pw", "", "")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !strings.HasPrefix(res.Token, "di_session_") {
		t.Fatalf("login must return session token, got %+v", res)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	s, mock := newMockAuth(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("right"), bcrypt.DefaultCost)
	mock.ExpectQuery(`SELECT .* FROM "bi_user"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password_hash", "role", "created_at", "updated_at"}).
			AddRow(1, "alice", string(hash), "admin", time.Now(), time.Now()))

	_, err := s.Login(context.Background(), "alice", "wrong", "", "")
	if err == nil {
		t.Fatal("wrong password must fail")
	}
	if !regexp.MustCompile("invalid").MatchString(err.Error()) {
		t.Fatalf("generic credentials error expected, got %v", err)
	}
}

// --- Verify / Revoke ---

func TestVerify_HitRefreshesUsage(t *testing.T) {
	s, mock := newMockAuth(t)
	raw := "di_session_zzzz"
	mock.ExpectQuery(`SELECT .* FROM "bi_token"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "kind", "token_hash", "prefix"}).
			AddRow(9, 1, "session", hashToken(raw), "di_session_zzzz"))
	mock.ExpectQuery(`SELECT .* FROM "bi_user"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password_hash", "role", "created_at", "updated_at"}).
			AddRow(1, "alice", "hash", "admin", time.Now(), time.Now()))
	mock.ExpectExec(`UPDATE "bi_token" AS "token" SET last_used_at`).WillReturnResult(sqlmock.NewResult(0, 1))

	p, err := s.Verify(context.Background(), raw, "5.6.7.8", "agent")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if p.UserID != 1 || p.Kind != "session" || p.Role != "admin" {
		t.Fatalf("bad principal %+v", p)
	}
}

// Expired / revoked tokens are filtered by the SELECT WHERE clause → no rows → 20200.
func TestVerify_RejectsExpiredOrRevoked(t *testing.T) {
	s, mock := newMockAuth(t)
	mock.ExpectQuery(`SELECT .* FROM "bi_token"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "kind", "token_hash", "prefix"})) // empty

	_, err := s.Verify(context.Background(), "di_pat_deadbeef", "", "")
	if err == nil {
		t.Fatal("expired/revoked token must be rejected")
	}
	if be, ok := err.(router.BusinessError); !ok || be.Code != 20200 {
		t.Fatalf("want 20200, got %v", err)
	}
}

func TestRevoke_Token(t *testing.T) {
	s, mock := newMockAuth(t)
	mock.ExpectExec(`UPDATE "bi_token" AS "token" SET revoked_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := s.Revoke(context.Background(), "di_pat_abcdef"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

// --- PAT management (#184) ---

func TestCreatePAT_PlaintextOnceAndExpiry(t *testing.T) {
	s, mock := newMockAuth(t)
	// bun renders the autoincrement+nullzero insert as a RETURNING query, not Exec.
	mock.ExpectQuery(`INSERT INTO "bi_token"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "revoked_at"}).AddRow(7, nil))

	res, err := s.CreatePAT(context.Background(), 1, "我的 MCP 客户端", false)
	if err != nil {
		t.Fatalf("CreatePAT: %v", err)
	}
	if !strings.HasPrefix(res.Token, "di_pat_") {
		t.Fatalf("PAT plaintext must carry di_pat_ prefix, got %q", res.Token)
	}
	// Metadata carries only a short head of the secret, never the full plaintext.
	if res.Info.Prefix == "" || res.Info.Prefix == res.Token || !strings.HasPrefix(res.Token, res.Info.Prefix) {
		t.Fatalf("info prefix must be a strict head of the token, got prefix=%q", res.Info.Prefix)
	}
	if res.Info.ExpiresAt == "" {
		t.Fatal("default PAT must carry a 90-day expiry")
	}
	if res.Info.Name != "我的 MCP 客户端" {
		t.Fatalf("name not persisted, got %q", res.Info.Name)
	}
}

func TestCreatePAT_NeverExpires(t *testing.T) {
	s, mock := newMockAuth(t)
	mock.ExpectQuery(`INSERT INTO "bi_token"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "revoked_at"}).AddRow(8, nil))
	res, err := s.CreatePAT(context.Background(), 1, "long", true)
	if err != nil {
		t.Fatalf("CreatePAT: %v", err)
	}
	if res.Info.ExpiresAt != "" {
		t.Fatalf("never-PAT must have empty expiry, got %q", res.Info.ExpiresAt)
	}
}

func TestListPATs_ScopedAndNoSecret(t *testing.T) {
	s, mock := newMockAuth(t)
	mock.ExpectQuery(`SELECT .* FROM "bi_token"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "kind", "name", "token_hash", "prefix", "expires_at", "revoked_at", "created_at"}).
			AddRow(3, 1, "pat", "ci", "deadbeefdeadbeef", "di_pat_ABCDEF", nil, nil, time.Now()))
	out, err := s.ListPATs(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListPATs: %v", err)
	}
	if len(out) != 1 || out[0].Prefix != "di_pat_ABCDEF" || out[0].Revoked {
		t.Fatalf("unexpected list: %+v", out)
	}
}

func TestRevokeByID_ScopedToUser(t *testing.T) {
	s, mock := newMockAuth(t)
	// The WHERE must carry user_id (self-scope) — asserting the aliased UPDATE runs.
	mock.ExpectExec(`UPDATE "bi_token" AS "token" SET revoked_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := s.RevokeByID(context.Background(), 1, 3); err != nil {
		t.Fatalf("RevokeByID: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}
