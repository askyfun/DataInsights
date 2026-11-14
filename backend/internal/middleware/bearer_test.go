package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"tablewave/internal/domain/entity"
	"tablewave/internal/listpage"
	"tablewave/internal/response"
	"tablewave/internal/router"
	"tablewave/internal/service/auth"
)

// fakeAuth returns a principal whose Kind mirrors the token's kind marker, so a
// test can exercise the session-vs-PAT branch by choosing the bearer value.
type fakeAuth struct {
	reject bool
	role   string
}

func (f *fakeAuth) Register(context.Context, string, string, string, string, string) (*entity.AuthResult, error) {
	return nil, nil
}
func (f *fakeAuth) EnsureBootstrap(context.Context) (string, bool, error) { return "", false, nil }
func (f *fakeAuth) BootstrapStatus(context.Context) (bool, string, error) { return false, "off", nil }
func (f *fakeAuth) RecoverAdmin(context.Context, string, string) error    { return nil }
func (f *fakeAuth) Login(context.Context, string, string, string, string) (*entity.AuthResult, error) {
	return nil, nil
}
func (f *fakeAuth) CurrentUser(context.Context, int) (*entity.User, error) { return nil, nil }
func (f *fakeAuth) Revoke(context.Context, string) error                   { return nil }
func (f *fakeAuth) CreatePAT(context.Context, int, string, bool) (*entity.PATCreateResult, error) {
	return nil, nil
}
func (f *fakeAuth) ListPATs(context.Context, int) ([]entity.TokenInfo, error) { return nil, nil }
func (f *fakeAuth) RevokeByID(context.Context, int, int) error                { return nil }
func (f *fakeAuth) ListUsers(context.Context, listpage.Params) (listpage.Result[entity.User], error) {
	return listpage.Result[entity.User]{}, nil
}
func (f *fakeAuth) CreateUser(context.Context, auth.CreateUserInput) (*entity.User, error) {
	return nil, nil
}
func (f *fakeAuth) UpdateUser(context.Context, int, auth.UpdateUserInput) (*entity.User, error) {
	return nil, nil
}
func (f *fakeAuth) ResetPassword(context.Context, int, string) error { return nil }
func (f *fakeAuth) Verify(_ context.Context, raw, _, _ string) (*auth.Principal, error) {
	if f.reject || raw == "" || strings.Contains(raw, "invalid") {
		return nil, router.NewBusinessError(response.CodeUnauthorized, "invalid or expired token")
	}
	kind := "session"
	if strings.HasPrefix(raw, "di_pat_") {
		kind = "pat"
	}
	role := f.role
	if role == "" {
		role = "admin"
	}
	return &auth.Principal{UserID: 1, Username: "alice", Role: role, Kind: kind}, nil
}

func newEngine(allowPAT bool, reject bool) (*gin.Engine, *strings.Builder) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	body := &strings.Builder{}
	r.GET("/need", Bearer(&fakeAuth{reject: reject}, allowPAT), func(c *gin.Context) {
		p, _ := PrincipalFrom(c)
		_, _ = body.WriteString(p.Kind)
		c.String(http.StatusOK, "ok")
	})
	return r, body
}

func do(r *gin.Engine, header string) (int, string) {
	req := httptest.NewRequest(http.MethodGet, "/need", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestBearer_MissingHeader_FailsClosed(t *testing.T) {
	r, body := newEngine(true, false)
	_, out := do(r, "")
	if !strings.Contains(out, "20200") || body.Len() != 0 {
		t.Fatalf("missing token must 20200 and not reach handler, got %q", out)
	}
}

func TestBearer_SessionAlwaysAllowed(t *testing.T) {
	r, body := newEngine(false, false)
	do(r, "Bearer di_session_abcdef")
	if body.String() != "session" {
		t.Fatalf("session token must pass even when allowPAT=false, kind=%q", body.String())
	}
}

func TestBearer_PATDeniedOnUserManagement(t *testing.T) {
	r, _ := newEngine(false, false)
	_, out := do(r, "Bearer di_pat_abcdef")
	// R-99：登录了但没权限 → 20202/FORBIDDEN，与未认证（20200）分档。
	if !strings.Contains(out, `"code":20202`) || !strings.Contains(out, "FORBIDDEN") {
		t.Fatalf("PAT must be refused as 20202/FORBIDDEN when allowPAT=false, got %q", out)

	}
}

func TestBearer_PATAllowedWherePermitted(t *testing.T) {
	r, body := newEngine(true, false)
	do(r, "Bearer di_pat_abcdef")
	if body.String() != "pat" {
		t.Fatalf("PAT must pass when allowPAT=true, kind=%q", body.String())
	}
}

func TestBearer_InvalidTokenDenied(t *testing.T) {
	r, _ := newEngine(true, false)
	_, out := do(r, "Bearer di_pat_invalid")
	// R-99：未认证稳定码随 20200 一并下发（UNAUTHENTICATED）。
	if !strings.Contains(out, `"code":20200`) || !strings.Contains(out, "UNAUTHENTICATED") {
		t.Fatalf("invalid/expired/revoked token must be denied as UNAUTHENTICATED, got %q", out)
	}
}

func newAdminEngine(role string) (*gin.Engine, *strings.Builder) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	body := &strings.Builder{}
	r.GET("/admin", Bearer(&fakeAuth{role: role}, false), RequireAdmin(), func(c *gin.Context) {
		_, _ = body.WriteString("reached")
		c.String(http.StatusOK, "ok")
	})
	return r, body
}

func TestRequireAdmin_NonAdminForbidden(t *testing.T) {
	r, body := newAdminEngine("user")
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer di_session_abcdef")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "20202") {
		t.Fatalf("non-admin must get 20202, got %q", w.Body.String())
	}
	if body.Len() != 0 {
		t.Fatalf("handler must not be reached, got %q", body.String())
	}
}

func TestRequireAdmin_AdminAllowed(t *testing.T) {
	r, body := newAdminEngine("admin")
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer di_session_abcdef")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if body.String() != "reached" {
		t.Fatalf("admin must reach handler, body=%q resp=%q", body.String(), w.Body.String())
	}
}

// ---- OptionalBearer（R-95, #220）：资源端点的可选认证 ----

func newOptionalEngine() (*gin.Engine, *strings.Builder) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	body := &strings.Builder{}
	r.GET("/open", OptionalBearer(&fakeAuth{reject: false}), func(c *gin.Context) {
		if p, ok := PrincipalFrom(c); ok {
			_, _ = body.WriteString(p.Username)
		} else {
			_, _ = body.WriteString("anonymous")
		}
		c.String(http.StatusOK, "ok")
	})
	return r, body
}

func doOptional(r *gin.Engine, header string) (int, string) {
	req := httptest.NewRequest(http.MethodGet, "/open", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestOptionalBearer_MissingHeader_AnonymousPasses(t *testing.T) {
	r, body := newOptionalEngine()
	code, out := doOptional(r, "")
	if code != http.StatusOK || out != "ok" || body.String() != "anonymous" {
		t.Fatalf("missing header must pass anonymously, got code=%d body=%q principal=%q", code, out, body.String())
	}
}

func TestOptionalBearer_ValidToken_SetsPrincipal(t *testing.T) {
	r, body := newOptionalEngine()
	code, out := doOptional(r, "Bearer di_session_abcdef")
	if code != http.StatusOK || out != "ok" || body.String() != "alice" {
		t.Fatalf("valid token must set principal, got code=%d body=%q principal=%q", code, out, body.String())
	}
}

func TestOptionalBearer_InvalidToken_FailsClosed(t *testing.T) {
	r, _ := newOptionalEngine()
	code, out := doOptional(r, "Bearer di_session_invalid")
	if !strings.Contains(out, "20200") {
		t.Fatalf("present-but-invalid token must fail closed 20200, got code=%d body=%q", code, out)
	}
}
