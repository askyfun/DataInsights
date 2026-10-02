package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"data-insights/internal/domain/entity"
	"data-insights/internal/response"
	"data-insights/internal/router"
	"data-insights/internal/service/auth"
)

// fakeAuth returns a principal whose Kind mirrors the token's kind marker, so a
// test can exercise the session-vs-PAT branch by choosing the bearer value.
type fakeAuth struct{ reject bool }

func (f *fakeAuth) Register(context.Context, string, string, string, string) (*entity.AuthResult, error) {
	return nil, nil
}
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
func (f *fakeAuth) Verify(_ context.Context, raw, _, _ string) (*auth.Principal, error) {
	if f.reject || raw == "" || strings.Contains(raw, "invalid") {
		return nil, router.NewBusinessError(response.CodeUnauthorized, "invalid or expired token")
	}
	kind := "session"
	if strings.HasPrefix(raw, "di_pat_") {
		kind = "pat"
	}
	return &auth.Principal{UserID: 1, Username: "alice", Role: "admin", Kind: kind}, nil
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
	if !strings.Contains(out, "20200") {
		t.Fatalf("PAT must be refused when allowPAT=false, got %q", out)
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
	if !strings.Contains(out, "20200") {
		t.Fatalf("invalid/expired/revoked token must be denied, got %q", out)
	}
}
