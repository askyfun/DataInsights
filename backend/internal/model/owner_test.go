package model

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// TestResolveOwnerID 钉死 R-95（#220）的归属换算口径：有已认证主体（id>0）写真实
// 用户 id，匿名（0）与非法负值一律落回系统占位 SystemOwnerID=0。
func TestResolveOwnerID(t *testing.T) {
	cases := []struct {
		principal int
		want      int32
	}{
		{0, SystemOwnerID},
		{7, 7},
		{-3, SystemOwnerID},
	}
	for _, tc := range cases {
		if got := ResolveOwnerID(tc.principal); got != tc.want {
			t.Errorf("ResolveOwnerID(%d) = %d, want %d", tc.principal, got, tc.want)
		}
	}
}

// TestOwnerNames 验证批量解析 bi_user 用户名：只按 id IN 过滤——刻意不带任何
// 状态过滤（R-95 验收 4「禁用不删归属」：用户被禁用/软删后名字仍要能显示）；
// 查不到的 id（匿名 0 或脏引用）不进 map，调用方据此留空。
func TestOwnerNames(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	ctx := context.Background()

	// 空集合不发起查询。
	names, err := OwnerNames(ctx, db, nil)
	if err != nil {
		t.Fatalf("OwnerNames(nil): %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("OwnerNames(nil) = %v, want empty", names)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("empty ids must not query: %v", err)
	}

	// 正常解析；id=2 不在返回行里 → map 不含它。
	mock.ExpectQuery(`FROM "bi_user" AS "user" WHERE \(id IN`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).
			AddRow(1, "alice").
			AddRow(3, "carol"))
	names, err = OwnerNames(ctx, db, []int32{1, 2, 3})
	if err != nil {
		t.Fatalf("OwnerNames: %v", err)
	}
	if names[1] != "alice" || names[3] != "carol" {
		t.Fatalf("OwnerNames = %v, want alice/carol", names)
	}
	if _, ok := names[2]; ok {
		t.Fatalf("missing user id 2 must not resolve, got %v", names)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
