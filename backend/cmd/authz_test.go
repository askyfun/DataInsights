package main

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"tablewave/internal/extract"
	"tablewave/internal/service/auth"
)

// TestAllAPIRoutesClassified 是 R-92（#217）的路由策略守卫：遍历真实注册的
// gin.Engine.Routes()，断言每条 /api 路由都命中「强制 / 匿名白名单」分类表，
// 出现未分类路由即失败；并校验白名单每一项都对应一条真实路由（防止表漂移）。
func TestAllAPIRoutesClassified(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	db := bun.NewDB(sqlDB, pgdialect.New())
	authSvc := auth.NewService(db, auth.GuardOptions{}, auth.AdminConfig{})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetupRoutes(r, db, []byte("0123456789abcdef0123456789abcdef"), extract.Guard{}, "", nil, authSvc, "off", "dev")

	publicSeen := map[string]bool{}
	enforced := 0
	for _, rt := range r.Routes() {
		if !strings.HasPrefix(rt.Path, "/api/") {
			continue
		}
		class, ok := classifyRoute(rt.Method, rt.Path)
		if !ok {
			t.Errorf("unclassified /api route: %s %s", rt.Method, rt.Path)
			continue
		}
		if class == "enforce" {
			enforced++
		}
		if authzPublic[rt.Method+" "+rt.Path] {
			publicSeen[rt.Method+" "+rt.Path] = true
		}
	}
	for key := range authzPublic {
		if !publicSeen[key] {
			t.Errorf("whitelist entry has no matching real route: %s", key)
		}
	}
	if enforced == 0 {
		t.Fatalf("expected some enforced /api routes, got 0")
	}
}
