package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"tablewave/internal/service/auth"
)

// guardStub 是 auth.Service 的窄替身：只覆盖 Verify，其余方法经内嵌接口得到 nil。
type guardStub struct {
	auth.Service
	principal *auth.Principal
	err       error
}

func (g guardStub) Verify(_ context.Context, _, _, _ string) (*auth.Principal, error) {
	return g.principal, g.err
}

func clsOf(class string) func(string, string) (string, bool) {
	return func(_, _ string) (string, bool) { return class, true }
}

func buildGuardEngine(svc auth.Service, mode string, class string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", AuthGuard(svc, mode, clsOf(class)), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

func doGuard(r *gin.Engine, authz string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if authz != "" {
		req.Header.Set("Authorization", "Bearer "+authz)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAuthGuard_On_DeniesAnonymous(t *testing.T) {
	svc := guardStub{}
	w := doGuard(buildGuardEngine(svc, "on", "enforce"), "")
	if !strings.Contains(w.Body.String(), `"code":20200`) {
		t.Fatalf("on + anonymous must be 20200, got %q", w.Body.String())
	}
}

func TestAuthGuard_On_AllowsWithToken(t *testing.T) {
	svc := guardStub{principal: &auth.Principal{UserID: 1, Kind: "session"}}
	w := doGuard(buildGuardEngine(svc, "on", "enforce"), "di_session_x")
	if !strings.Contains(w.Body.String(), "ok") {
		t.Fatalf("on + valid token must pass, got %q", w.Body.String())
	}
}

func TestAuthGuard_PublicBypass(t *testing.T) {
	svc := guardStub{}
	w := doGuard(buildGuardEngine(svc, "on", "public"), "")
	if !strings.Contains(w.Body.String(), "ok") {
		t.Fatalf("public route must bypass regardless of mode, got %q", w.Body.String())
	}
}

func TestAuthGuard_OffAndShadow_PassAnonymous(t *testing.T) {
	for _, mode := range []string{"off", "shadow"} {
		svc := guardStub{}
		w := doGuard(buildGuardEngine(svc, mode, "enforce"), "")
		if !strings.Contains(w.Body.String(), "ok") {
			t.Fatalf("%s + anonymous must pass (no enforcement), got %q", mode, w.Body.String())
		}
	}
}

func TestAuthGuard_InvalidTokenFailClosedAllModes(t *testing.T) {
	for _, mode := range []string{"off", "shadow", "on"} {
		svc := guardStub{err: errFakeInvalid}
		w := doGuard(buildGuardEngine(svc, mode, "public"), "di_bad")
		if !strings.Contains(w.Body.String(), `"code":20200`) {
			t.Fatalf("%s + invalid token must fail closed 20200, got %q", mode, w.Body.String())
		}
	}
}
