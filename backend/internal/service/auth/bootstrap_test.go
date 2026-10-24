package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"data-insights/internal/router"
)

func TestBootstrapStatus_NeedsBootstrapOnlyWithoutAdmins(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows int
		want bool
	}{
		{"no admins", 0, true},
		{"admin exists", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, mock := newMockAuth(t)
			mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_user"( AS "user")? WHERE \(role = `).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(tc.rows))
			needs, mode, err := s.BootstrapStatus(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if needs != tc.want {
				t.Fatalf("needs_bootstrap = %v, want %v", needs, tc.want)
			}
			if mode != EnforceModeOff {
				t.Fatalf("enforce_mode = %q, want %q", mode, EnforceModeOff)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
		})
	}
}

func TestRegister_RejectsMissingBootstrapToken(t *testing.T) {
	s, mock := newMockAuth(t)
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_user"( AS "user")? WHERE \(role = `).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()

	_, err := s.Register(context.Background(), "alice", "pw", "", "", "")
	var be router.BusinessError
	if !errors.As(err, &be) || be.Code != 20200 {
		t.Fatalf("want 20200, got %#v", err)
	}
	if !strings.Contains(be.Message, "bootstrap token is required") {
		t.Fatalf("message must be distinguishable from login, got %q", be.Message)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestRegister_RejectsExpiredBootstrapToken(t *testing.T) {
	s, mock := newMockAuth(t)
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_user"( AS "user")? WHERE \(role = `).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	expectBootstrapToken(mock, time.Now().Add(-time.Minute))
	mock.ExpectRollback()

	_, err := s.Register(context.Background(), "alice", "pw", testBootstrapToken, "", "")
	var be router.BusinessError
	if !errors.As(err, &be) || be.Code != 20200 || !strings.Contains(be.Message, "invalid or expired") {
		t.Fatalf("want expired-token 20200, got %#v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestRegister_RejectsReusedUsernameIncludingDisabled(t *testing.T) {
	s, mock := newMockAuth(t)
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_user"( AS "user")? WHERE \(role = `).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	expectBootstrapToken(mock, time.Now().Add(time.Minute))
	// The uniqueness check deliberately has NO deleted_at filter.
	mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_user"( AS "user")? WHERE \(username = `).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	_, err := s.Register(context.Background(), "alice", "pw", testBootstrapToken, "", "")
	var be router.BusinessError
	if !errors.As(err, &be) || be.Code != 20100 || !strings.Contains(be.Message, "already used") {
		t.Fatalf("want username-in-use 20100, got %#v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestEnsureBootstrap_GeneratesTokenOnlyWithoutAdmin(t *testing.T) {
	s, mock := newMockAuth(t)
	mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_user"( AS "user")? WHERE \(role = `).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_user"( AS "user")? WHERE \(role = `).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`INSERT INTO bi_setting`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO bi_setting`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	raw, generated, err := s.EnsureBootstrap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !generated || !strings.HasPrefix(raw, "di_boot_") {
		t.Fatalf("expected a generated bootstrap token, got generated=%v raw=%q", generated, raw)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestEnsureBootstrap_SkipsTokenWhenAdminExists(t *testing.T) {
	s, mock := newMockAuth(t)
	mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_user"( AS "user")? WHERE \(role = `).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	raw, generated, err := s.EnsureBootstrap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if generated || raw != "" {
		t.Fatalf("no token should be printed when an admin exists, got %q", raw)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestEnsureBootstrap_CreatesAdminFromEnv(t *testing.T) {
	s, mock := newMockAuth(t)
	s.adminCfg = AdminConfig{Username: "root", Password: "root-pass"}
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_user"( AS "user")? WHERE \(role = `).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`INSERT INTO "bi_user".*RETURNING`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "deleted_at"}).AddRow(1, nil))
	mock.ExpectExec(`UPDATE "bi_datasource" AS "datasource" SET owner_id`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE "bi_dataset" AS "dataset" SET owner_id`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE "bi_chart" AS "chart" SET owner_id`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM bi_setting WHERE key = 'bootstrap_token_hash'`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM bi_setting WHERE key = 'bootstrap_token_expires_at'`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	raw, generated, err := s.EnsureBootstrap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if generated || raw != "" {
		t.Fatalf("env admin must suppress the bootstrap token, got %q", raw)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestRecoverAdmin_RejectsWrongTokenWithoutTouchingDB(t *testing.T) {
	s, mock := newMockAuth(t)
	s.adminCfg = AdminConfig{ResetToken: "reset-secret"}

	err := s.RecoverAdmin(context.Background(), "wrong", "new-password")
	var be router.BusinessError
	if !errors.As(err, &be) || be.Code != 20200 {
		t.Fatalf("want 20200, got %#v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("no DB query expected: %v", err)
	}
}

func TestRecoverAdmin_RejectsAlreadyUsedToken(t *testing.T) {
	s, mock := newMockAuth(t)
	s.adminCfg = AdminConfig{ResetToken: "reset-secret"}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM bi_setting WHERE key = 'admin_recover_used_hash'`).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(hashToken("reset-secret")))
	mock.ExpectRollback()

	err := s.RecoverAdmin(context.Background(), "reset-secret", "new-password")
	var be router.BusinessError
	if !errors.As(err, &be) || be.Code != 20200 || !strings.Contains(be.Message, "already used") {
		t.Fatalf("want already-used 20200, got %#v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestRecoverAdmin_ResetsFirstAdminAndRevokesSessions(t *testing.T) {
	s, mock := newMockAuth(t)
	s.adminCfg = AdminConfig{ResetToken: "reset-secret"}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM bi_setting WHERE key = 'admin_recover_used_hash'`).
		WillReturnRows(sqlmock.NewRows([]string{"value"}))
	mock.ExpectQuery(`SELECT .* FROM "bi_user" AS "user" WHERE \(role = `).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password_hash", "role", "created_at", "updated_at", "deleted_at"}).
			AddRow(7, "root", "old-hash", "admin", time.Now(), time.Now(), nil))
	mock.ExpectExec(`UPDATE "bi_user" AS "user" SET password_hash = `).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "bi_token" AS "token" SET revoked_at = `).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`INSERT INTO bi_setting`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := s.RecoverAdmin(context.Background(), "reset-secret", "new-password"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}
