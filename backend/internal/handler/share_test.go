package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"dataray/internal/domain/entity"
	"dataray/internal/router"
	"dataray/internal/service/share"

	"github.com/gin-gonic/gin"
)

// 测试夹具输入，不是任何真实凭据。
var (
	fixtureGoodInput = fmt.Sprintf("fixture-%s-value", "good")
	fixtureBadInput  = fmt.Sprintf("fixture-%s-value", "bad")
)

func fixtureBody(input string) string {
	return fmt.Sprintf(`{"password":%q}`, input)
}

// mockShareService implements share.Service for handler tests. The embedded
// interface is nil on purpose: calling a method that is not overridden fails
// loudly (same style as mockDatasourceService in datasource_test.go).
type mockShareService struct {
	share.Service

	listFunc             func(ctx context.Context) ([]entity.Share, error)
	createFunc           func(ctx context.Context, chartID int, password, expiresAt *string) (*entity.Share, error)
	getByTokenFunc       func(ctx context.Context, token string) (*entity.Share, error)
	validatePasswordFunc func(ctx context.Context, token, password string) error
}

func (m *mockShareService) List(ctx context.Context) ([]entity.Share, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	return nil, nil
}

func (m *mockShareService) Create(ctx context.Context, chartID int, password, expiresAt *string) (*entity.Share, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, chartID, password, expiresAt)
	}
	return nil, nil
}

func (m *mockShareService) GetByToken(ctx context.Context, token string) (*entity.Share, error) {
	if m.getByTokenFunc != nil {
		return m.getByTokenFunc(ctx, token)
	}
	return nil, nil
}

func (m *mockShareService) ValidatePassword(ctx context.Context, token, password string) error {
	if m.validatePasswordFunc != nil {
		return m.validatePasswordFunc(ctx, token, password)
	}
	return nil
}

// newShareTestRouter mirrors the /api/shares section of cmd/routes.go. During
// the Batch 2 migration only this wiring helper changes; every response-body
// assertion below must keep passing byte-for-byte before and after migration
// — that is the zero-behavior-change guard (same contract as
// newDatasourceTestRouter in datasource_test.go). serve/assertBody and the
// shared envelope constants come from that file.
func newShareTestRouter(h *ShareHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	shares := r.Group("/api/shares")
	router.RegisterGetRoute(shares, "", h.List)
	router.RegisterPostRoute(shares, "", h.Create)
	router.RegisterGetRoute(shares, "/:token", h.Get)
	router.RegisterPostRoute(shares, "/:token/verify", h.Verify)
	return r
}

// newShareViewRouter registers View outside the /api group. View answers a
// 302 redirect on success, which the JSON-envelope router cannot emit, so
// this wiring is not a migration target (same exemption as the health route);
// the tests below pin its exact bytes before and after the migration.
func newShareViewRouter(h *ShareHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/share/:token", h.View)
	return r
}

const (
	notFoundShare      = `{"code":20300,"msg":"share not found","trace":"","data":{}}`
	badRequestTokenReq = `{"code":20100,"msg":"token is required","trace":"","data":{}}`
	expiredShare       = `{"code":20400,"msg":"share link has expired","trace":"","data":{}}`
	shareFoundJSON     = `{"id":1,"token":"tok","chart_id":42,"expires_at":null,"created_at":"","has_password":true}`
)

// ---------------------------------------------------------------- List

func TestShareList_Empty(t *testing.T) {
	h := NewShareHandler(&mockShareService{
		listFunc: func(_ context.Context) ([]entity.Share, error) { return nil, nil },
	})
	w := serve(newShareTestRouter(h), http.MethodGet, "/api/shares", "")
	assertBody(t, w, okEnvelopeEmptyArray)
}

func TestShareList_Data(t *testing.T) {
	h := NewShareHandler(&mockShareService{
		listFunc: func(_ context.Context) ([]entity.Share, error) {
			return []entity.Share{{ID: 1, Token: "tok", ChartID: 42, HasPassword: true}}, nil
		},
	})
	w := serve(newShareTestRouter(h), http.MethodGet, "/api/shares", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":[`+shareFoundJSON+`]}`)
}

func TestShareList_ServiceError(t *testing.T) {
	h := NewShareHandler(&mockShareService{
		listFunc: func(_ context.Context) ([]entity.Share, error) { return nil, errBoom() },
	})
	w := serve(newShareTestRouter(h), http.MethodGet, "/api/shares", "")
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Create

func TestShareCreate_Success(t *testing.T) {
	var gotChartID int
	var gotPassword, gotExpires *string
	h := NewShareHandler(&mockShareService{
		createFunc: func(_ context.Context, chartID int, password, expiresAt *string) (*entity.Share, error) {
			gotChartID, gotPassword, gotExpires = chartID, password, expiresAt
			return &entity.Share{ID: 9, Token: "t9", ChartID: chartID, HasPassword: password != nil}, nil
		},
	})
	body := `{"chart_id":42,"password":"p","expires_at":"2030-01-01T00:00:00Z"}`
	w := serve(newShareTestRouter(h), http.MethodPost, "/api/shares", body)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"id":9,"token":"t9","chart_id":42,"expires_at":null,"created_at":"","has_password":true}}`)
	if gotChartID != 42 || gotPassword == nil || *gotPassword != "p" ||
		gotExpires == nil || *gotExpires != "2030-01-01T00:00:00Z" {
		t.Fatalf("unexpected create args: chartID=%d password=%v expires=%v", gotChartID, derefOr(gotPassword), derefOr(gotExpires))
	}
}

func TestShareCreate_OmittedPointers(t *testing.T) {
	// Empty password/expires_at strings must reach the service as nil
	// pointers, exactly like the pre-migration handler's guards.
	var gotPassword, gotExpires *string
	h := NewShareHandler(&mockShareService{
		createFunc: func(_ context.Context, chartID int, password, expiresAt *string) (*entity.Share, error) {
			gotPassword, gotExpires = password, expiresAt
			return &entity.Share{ID: 9, Token: "t9", ChartID: chartID}, nil
		},
	})
	w := serve(newShareTestRouter(h), http.MethodPost, "/api/shares", `{"chart_id":42}`)
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":{"id":9,"token":"t9","chart_id":42,"expires_at":null,"created_at":"","has_password":false}}`)
	if gotPassword != nil || gotExpires != nil {
		t.Fatalf("expected nil pointers, got password=%v expires=%v", derefOr(gotPassword), derefOr(gotExpires))
	}
}

func TestShareCreate_EmptyBody(t *testing.T) {
	h := NewShareHandler(&mockShareService{
		createFunc: func(_ context.Context, _ int, _, _ *string) (*entity.Share, error) {
			t.Fatal("Create must not be called for an empty body")
			return nil, nil
		},
	})
	w := serve(newShareTestRouter(h), http.MethodPost, "/api/shares", "")
	assertBody(t, w, badRequestEOF)
}

func TestShareCreate_TypeMismatchBody(t *testing.T) {
	// Accepted unavoidable diff #1 (datasource package doc): the named In
	// type leaks into the json bind-error text. Pre-migration the handler
	// bound an anonymous struct, so the old message was
	// "…Go struct field .chart_id…".
	h := NewShareHandler(&mockShareService{
		createFunc: func(_ context.Context, _ int, _, _ *string) (*entity.Share, error) {
			t.Fatal("Create must not be called when the body fails to bind")
			return nil, nil
		},
	})
	w := serve(newShareTestRouter(h), http.MethodPost, "/api/shares", `{"chart_id":"x"}`)
	assertBody(t, w, `{"code":20100,"msg":"json: cannot unmarshal string into Go struct field shareCreateIn.chart_id of type int","trace":"","data":{}}`)
}

func TestShareCreate_QueryMustNotPolluteBody(t *testing.T) {
	var gotChartID int
	h := NewShareHandler(&mockShareService{
		createFunc: func(_ context.Context, chartID int, _, _ *string) (*entity.Share, error) {
			gotChartID = chartID
			return &entity.Share{ID: 9, ChartID: chartID}, nil
		},
	})
	w := serve(newShareTestRouter(h), http.MethodPost, "/api/shares?ChartID=99", `{"password":"p"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", w.Code)
	}
	if gotChartID != 0 {
		t.Fatalf("query param leaked into body struct: chartID=%d", gotChartID)
	}
}

func TestShareCreate_ServiceError(t *testing.T) {
	h := NewShareHandler(&mockShareService{
		createFunc: func(_ context.Context, _ int, _, _ *string) (*entity.Share, error) {
			return nil, errBoom()
		},
	})
	w := serve(newShareTestRouter(h), http.MethodPost, "/api/shares", `{"chart_id":42}`)
	assertBody(t, w, internalErrorBoom)
}

// ---------------------------------------------------------------- Get

func TestShareGet_Found(t *testing.T) {
	var gotToken string
	h := NewShareHandler(&mockShareService{
		getByTokenFunc: func(_ context.Context, token string) (*entity.Share, error) {
			gotToken = token
			return &entity.Share{ID: 1, Token: "tok", ChartID: 42, HasPassword: true}, nil
		},
	})
	w := serve(newShareTestRouter(h), http.MethodGet, "/api/shares/tok", "")
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":`+shareFoundJSON+`}`)
	if gotToken != "tok" {
		t.Errorf("expected GetByToken called with token 'tok', got %q", gotToken)
	}
}

func TestShareGet_NotFound(t *testing.T) {
	// Any GetByToken failure maps to the fixed "share not found" message —
	// the service error text must not leak.
	h := NewShareHandler(&mockShareService{
		getByTokenFunc: func(_ context.Context, _ string) (*entity.Share, error) {
			return nil, errors.New("sql: no rows in result set")
		},
	})
	w := serve(newShareTestRouter(h), http.MethodGet, "/api/shares/missing", "")
	assertBody(t, w, notFoundShare)
}

// ---------------------------------------------------------------- Verify

func TestShareVerify_CorrectInputReturnsShare(t *testing.T) {
	// Successor of TestVerifyCorrectInputReturnsShare: same assertions, now
	// pinned byte-for-byte through the shared envelope helper.
	var gotToken, gotInput string
	h := NewShareHandler(&mockShareService{
		validatePasswordFunc: func(_ context.Context, token, password string) error {
			gotToken, gotInput = token, password
			return nil
		},
		getByTokenFunc: func(_ context.Context, _ string) (*entity.Share, error) {
			return &entity.Share{ID: 1, Token: "tok", ChartID: 42, HasPassword: true}, nil
		},
	})
	w := serve(newShareTestRouter(h), http.MethodPost, "/api/shares/tok/verify", fixtureBody(fixtureGoodInput))
	assertBody(t, w, `{"code":20000,"msg":"success","trace":"","data":`+shareFoundJSON+`}`)
	if gotToken != "tok" {
		t.Fatalf("ValidatePassword called with token %q", gotToken)
	}
	if gotInput != fixtureGoodInput {
		t.Fatalf("ValidatePassword called with %q", gotInput)
	}
}

func TestShareVerify_WrongInputReturnsBusinessError(t *testing.T) {
	// Successor of TestVerifyWrongInputReturnsBusinessError: the validation
	// failure maps to 20100 with the service message verbatim.
	h := NewShareHandler(&mockShareService{
		validatePasswordFunc: func(_ context.Context, _, _ string) error {
			return errors.New("invalid password")
		},
		getByTokenFunc: func(_ context.Context, _ string) (*entity.Share, error) {
			t.Fatal("GetByToken must not run when password validation fails")
			return nil, nil
		},
	})
	w := serve(newShareTestRouter(h), http.MethodPost, "/api/shares/tok/verify", fixtureBody(fixtureBadInput))
	assertBody(t, w, `{"code":20100,"msg":"invalid password","trace":"","data":{}}`)
}

func TestShareVerify_GetByTokenFailsAfterValidate(t *testing.T) {
	h := NewShareHandler(&mockShareService{
		getByTokenFunc: func(_ context.Context, _ string) (*entity.Share, error) {
			return nil, errors.New("sql: no rows in result set")
		},
	})
	w := serve(newShareTestRouter(h), http.MethodPost, "/api/shares/tok/verify", fixtureBody(fixtureGoodInput))
	assertBody(t, w, notFoundShare)
}

func TestShareVerify_EmptyBody(t *testing.T) {
	// Valid token + empty body: the old unconditional ShouldBindJSON "EOF"
	// bind error; the router's method-gated body bind emits the same bytes.
	h := NewShareHandler(&mockShareService{
		validatePasswordFunc: func(_ context.Context, _, _ string) error {
			t.Fatal("ValidatePassword must not run for an empty body")
			return nil
		},
	})
	w := serve(newShareTestRouter(h), http.MethodPost, "/api/shares/tok/verify", "")
	assertBody(t, w, badRequestEOF)
}

func TestShareVerify_TypeMismatchBody(t *testing.T) {
	// Accepted unavoidable diff #1 (datasource package doc): the named In
	// type leaks into the json bind-error text. Pre-migration the handler
	// bound an anonymous struct, so the old message was
	// "…Go struct field .password…".
	h := NewShareHandler(&mockShareService{})
	w := serve(newShareTestRouter(h), http.MethodPost, "/api/shares/tok/verify", `{"password":123}`)
	assertBody(t, w, `{"code":20100,"msg":"json: cannot unmarshal number into Go struct field shareVerifyIn.password of type string","trace":"","data":{}}`)
}

func TestShareVerify_EmptyTokenValidBody(t *testing.T) {
	// Doubly-shaped request that is valid on the body side: gin matches the
	// empty :token segment, and the token check still runs first inside the
	// handler (the body binds fine), so "token is required" wins both before
	// and after migration.
	h := NewShareHandler(&mockShareService{
		validatePasswordFunc: func(_ context.Context, _, _ string) error {
			t.Fatal("ValidatePassword must not run with an empty token")
			return nil
		},
	})
	w := serve(newShareTestRouter(h), http.MethodPost, "/api/shares//verify", `{"password":"x"}`)
	assertBody(t, w, badRequestTokenReq)
}

func TestShareVerify_InvalidIDPrefersBodyBindError_EmptyBody(t *testing.T) {
	// Pinned accepted unavoidable diff #2 (datasource package doc): the
	// generic router binds the POST body before the handler checks the path
	// token, so a request carrying BOTH an empty :token AND an empty body
	// answers with the body-bind error ("EOF") where the pre-migration
	// handler answered "token is required". Same shape as the datasource
	// Test*_InvalidIDPrefersBodyBindError baselines. Requests valid on
	// either input are unaffected (see EmptyTokenValidBody / EmptyBody above).
	h := NewShareHandler(&mockShareService{
		validatePasswordFunc: func(_ context.Context, _, _ string) error {
			t.Fatal("ValidatePassword must not run when the body fails to bind")
			return nil
		},
	})
	w := serve(newShareTestRouter(h), http.MethodPost, "/api/shares//verify", "")
	assertBody(t, w, badRequestEOF)
}

// ---------------------------------------------------------------- View

func TestShareView_Redirects(t *testing.T) {
	h := NewShareHandler(&mockShareService{
		getByTokenFunc: func(_ context.Context, _ string) (*entity.Share, error) {
			return &entity.Share{ID: 1, Token: "tok", ChartID: 42}, nil
		},
	})
	w := serve(newShareViewRouter(h), http.MethodGet, "/share/tok", "")
	if w.Code != http.StatusFound {
		t.Fatalf("expected HTTP 302, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Location"); got != "/#/share/tok" {
		t.Errorf("expected Location /#/share/tok, got %q", got)
	}
	if got := w.Body.String(); got != "<a href=\"/#/share/tok\">Found</a>.\n\n" {
		t.Errorf("unexpected redirect body: %q", got)
	}
}

func TestShareView_NotFound(t *testing.T) {
	h := NewShareHandler(&mockShareService{
		getByTokenFunc: func(_ context.Context, _ string) (*entity.Share, error) {
			return nil, errors.New("sql: no rows in result set")
		},
	})
	w := serve(newShareViewRouter(h), http.MethodGet, "/share/missing", "")
	assertBody(t, w, notFoundShare)
}

func TestShareView_Expired(t *testing.T) {
	expired := "2020-01-01T00:00:00Z"
	h := NewShareHandler(&mockShareService{
		getByTokenFunc: func(_ context.Context, _ string) (*entity.Share, error) {
			return &entity.Share{ID: 1, Token: "tok", ChartID: 42, ExpiresAt: &expired}, nil
		},
	})
	w := serve(newShareViewRouter(h), http.MethodGet, "/share/tok", "")
	assertBody(t, w, expiredShare)
}

func TestShareView_FutureExpiryRedirects(t *testing.T) {
	future := "2999-01-01T00:00:00Z"
	h := NewShareHandler(&mockShareService{
		getByTokenFunc: func(_ context.Context, _ string) (*entity.Share, error) {
			return &entity.Share{ID: 1, Token: "tok", ChartID: 42, ExpiresAt: &future}, nil
		},
	})
	w := serve(newShareViewRouter(h), http.MethodGet, "/share/tok", "")
	if w.Code != http.StatusFound {
		t.Fatalf("expected HTTP 302, got %d: %s", w.Code, w.Body.String())
	}
}

func TestShareView_UnparseableExpiryRedirects(t *testing.T) {
	// The current handler only blocks on a successfully parsed expiry in the
	// past; a garbage expires_at falls through to the redirect.
	garbage := "not-a-timestamp"
	h := NewShareHandler(&mockShareService{
		getByTokenFunc: func(_ context.Context, _ string) (*entity.Share, error) {
			return &entity.Share{ID: 1, Token: "tok", ChartID: 42, ExpiresAt: &garbage}, nil
		},
	})
	w := serve(newShareViewRouter(h), http.MethodGet, "/share/tok", "")
	if w.Code != http.StatusFound {
		t.Fatalf("expected HTTP 302, got %d: %s", w.Code, w.Body.String())
	}
}

func derefOr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
