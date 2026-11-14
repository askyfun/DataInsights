package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"tablewave/internal/domain/entity"
	"tablewave/internal/router"
	"tablewave/internal/service/auth"
)

// mockAuthService implements auth.Service for handler tests. The embedded
// interface is nil on purpose (same style as mockQueryRecordService): calling a
// non-overridden method fails loudly instead of silently returning zero values.
type mockAuthService struct {
	auth.Service

	loginFunc func(ctx context.Context, username, password, ip, ua string) (*entity.AuthResult, error)
}

func (m *mockAuthService) Login(
	ctx context.Context, username, password, ip, ua string,
) (*entity.AuthResult, error) {
	if m.loginFunc != nil {
		return m.loginFunc(ctx, username, password, ip, ua)
	}
	return nil, nil
}

func newAuthTestRouter(h *AuthHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	auth := r.Group("/api/auth")
	router.RegisterPostRoute(auth, "/login", h.Login)
	return r
}

// TestLogin_RateLimited_SetsRetryAfterAndCode 钉住限流响应的对外语义
// （issue #203 acceptance 4）：HTTP 头带 Retry-After，信封业务码 20201。
func TestLogin_RateLimited_SetsRetryAfterAndCode(t *testing.T) {
	h := NewAuthHandler(&mockAuthService{
		loginFunc: func(_ context.Context, _, _, _, _ string) (*entity.AuthResult, error) {
			return nil, auth.ErrRateLimited{RetryAfter: 42 * time.Second}
		},
	})

	w := serve(newAuthTestRouter(h), http.MethodPost, "/api/auth/login",
		`{"username":"alice","password":"pw"}`)

	if got := w.Header().Get("Retry-After"); got != "42" {
		t.Fatalf("expected Retry-After=42, got %q", got)
	}
	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Code != 20201 {
		t.Fatalf("expected business code 20201 (rate limited), got %d", body.Code)
	}
	if !strings.Contains(body.Msg, "频繁") {
		t.Fatalf("expected distinct rate-limit message, got %q", body.Msg)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("unified envelope should use HTTP 200, got %d", w.Code)
	}
}

// TestLogin_RateLimited_RetryAfterCeilsToAtLeastOne 钉住「不足 1 秒也回 1 秒」的
// 边界（Retry-After 必须是正整数）。
func TestLogin_RateLimited_RetryAfterCeilsToAtLeastOne(t *testing.T) {
	h := NewAuthHandler(&mockAuthService{
		loginFunc: func(_ context.Context, _, _, _, _ string) (*entity.AuthResult, error) {
			return nil, auth.ErrRateLimited{RetryAfter: 300 * time.Millisecond}
		},
	})
	w := serve(newAuthTestRouter(h), http.MethodPost, "/api/auth/login",
		`{"username":"alice","password":"pw"}`)
	if got := w.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("expected Retry-After=1 (ceil), got %q", got)
	}
}
