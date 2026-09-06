package share

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"golang.org/x/crypto/bcrypt"

	"dataray/internal/model"
)

// captureMatcherFunc 记录实际执行的 SQL 并保留默认的 regexp 匹配语义，
// 便于断言写库语句中的密码形态（bcrypt hash 每次随机，无法提前断言精确值）。
func captureMatcherFunc(record *[]string) sqlmock.QueryMatcherFunc {
	return sqlmock.QueryMatcherFunc(func(expectedSQL, actualSQL string) error {
		*record = append(*record, actualSQL)
		return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
	})
}

func newTestShareService(t *testing.T) (*shareService, sqlmock.Sqlmock, *[]string) {
	t.Helper()
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureMatcherFunc(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	db := bun.NewDB(sqlDB, pgdialect.New())
	return &shareService{db: db}, mock, &executed
}

func shareRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "token", "chart_id", "password", "expires_at", "created_at"})
}

// TestToShareEntityPopulatesHasPassword 验证 model→entity 映射用存储密码的
// 存在性填充 has_password（脱敏后前端唯一的"是否受保护"信号）。
func TestToShareEntityPopulatesHasPassword(t *testing.T) {
	protected := toShareEntity(&model.Share{Token: "tok", Password: sql.NullString{String: "$2a$10$hash", Valid: true}})
	if !protected.HasPassword {
		t.Fatal("expected HasPassword=true for share with stored password")
	}

	passwordless := toShareEntity(&model.Share{Token: "tok"})
	if passwordless.HasPassword {
		t.Fatal("expected HasPassword=false for passwordless share")
	}

	emptyValid := toShareEntity(&model.Share{Token: "tok", Password: sql.NullString{String: "", Valid: true}})
	if emptyValid.HasPassword {
		t.Fatal("expected HasPassword=false for empty stored password")
	}
}

// TestCreateShareHashesPassword 验证 Create 存储的是 bcrypt hash（cost 10），
// 明文不再落库（序列化脱敏由 entity 层 json:"-" 的测试单独保证）。
func TestCreateShareHashesPassword(t *testing.T) {
	s, mock, executed := newTestShareService(t)
	// bun + pgdialect 对 autoincrement pk 会以 RETURNING "id" 发起 Query
	mock.ExpectQuery(`INSERT INTO "bi_share"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	pw := "pw123"
	created, err := s.Create(context.Background(), 1, &pw, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}

	if len(*executed) != 1 {
		t.Fatalf("expected 1 INSERT, got: %q", *executed)
	}
	if strings.Contains((*executed)[0], "'pw123'") {
		t.Fatalf("password stored in plaintext: %s", (*executed)[0])
	}
	if !strings.Contains((*executed)[0], "$2a$10$") {
		t.Fatalf("expected bcrypt cost-10 hash in INSERT, got: %s", (*executed)[0])
	}
	if created.Password == nil || !strings.HasPrefix(*created.Password, "$2") {
		t.Fatalf("expected created entity to carry bcrypt hash, got %v", created.Password)
	}
}

// TestValidatePasswordBcryptHash 验证 bcrypt hash 的校验成功/失败路径。
func TestValidatePasswordBcryptHash(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("pw123"), 10)
	if err != nil {
		t.Fatal(err)
	}

	s, mock, executed := newTestShareService(t)

	// 正确密码：校验通过，无写库
	mock.ExpectQuery(`SELECT .* FROM "bi_share"`).
		WillReturnRows(shareRows().AddRow(1, "tok", 1, string(hash), nil, nil))
	if err := s.ValidatePassword(context.Background(), "tok", "pw123"); err != nil {
		t.Fatalf("expected success for correct password, got %v", err)
	}
	if len(*executed) != 1 {
		t.Fatalf("bcrypt path must not write back, got: %q", *executed)
	}

	// 错误密码：校验失败
	mock.ExpectQuery(`SELECT .* FROM "bi_share"`).
		WillReturnRows(shareRows().AddRow(1, "tok", 1, string(hash), nil, nil))
	if err := s.ValidatePassword(context.Background(), "tok", "wrong"); err == nil {
		t.Fatal("expected error for wrong password")
	}
}

// TestValidatePasswordLegacyPlaintextUpgrades 验证存量明文兼容：
// 明文比较成功后回写 bcrypt hash；失败则报错且不写库。
func TestValidatePasswordLegacyPlaintextUpgrades(t *testing.T) {
	s, mock, executed := newTestShareService(t)

	// 正确密码：通过 + 回写 $2a$10$ hash
	mock.ExpectQuery(`SELECT .* FROM "bi_share"`).
		WillReturnRows(shareRows().AddRow(7, "tok", 1, "old-pass", nil, nil))
	mock.ExpectExec(`UPDATE "bi_share"`).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := s.ValidatePassword(context.Background(), "tok", "old-pass"); err != nil {
		t.Fatalf("expected success for legacy plaintext password, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}

	var upgrades []string
	for _, sql := range *executed {
		if strings.Contains(sql, "UPDATE") {
			upgrades = append(upgrades, sql)
		}
	}
	if len(upgrades) != 1 || !strings.Contains(upgrades[0], "'$2a$10$") {
		t.Fatalf("legacy plaintext must be upgraded to bcrypt hash, got: %q", *executed)
	}

	// 错误密码：失败且不写库
	*executed = nil
	mock.ExpectQuery(`SELECT .* FROM "bi_share"`).
		WillReturnRows(shareRows().AddRow(7, "tok", 1, "old-pass", nil, nil))
	if err := s.ValidatePassword(context.Background(), "tok", "wrong"); err == nil {
		t.Fatal("expected error for wrong legacy password")
	}
	if len(*executed) != 1 {
		t.Fatalf("failed legacy validation must not write back, got: %q", *executed)
	}
}
