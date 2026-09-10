package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// routingRequest is a placeholder for routes that don't need request body
type routingRequest struct {
	// Empty - query params bound directly
}

// Test request/response types
type testCreateRequest struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type testResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func init() {
	gin.SetMode(gin.TestMode)
}

// TestRegisterGetRoute_QueryParams tests GET route with query parameters
func TestRegisterGetRoute_QueryParams(t *testing.T) {
	router := gin.New()

	type listRequest struct {
		Limit  int    `form:"limit,default=20"`
		Offset int    `form:"offset,default=0"`
		Name   string `form:"name"`
	}

	RegisterGetRoute[listRequest, []testResponse](
		router.Group("/api"),
		"/users",
		func(req Request[listRequest], res *Response[[]testResponse]) error {
			if req.In.Limit != 10 {
				t.Errorf("expected limit 10, got %d", req.In.Limit)
			}
			if req.In.Offset != 5 {
				t.Errorf("expected offset 5, got %d", req.In.Offset)
			}
			if req.In.Name != "test" {
				t.Errorf("expected name 'test', got '%s'", req.In.Name)
			}
			res.Out = []testResponse{{ID: 1, Name: "test"}}
			return nil
		},
	)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/users?limit=10&offset=5&name=test", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// TestRegisterRoute_PathParams tests path parameter access via Ctx
func TestRegisterRoute_PathParams(t *testing.T) {
	router := gin.New()

	RegisterGetRoute[routingRequest, testResponse](
		router.Group("/api"),
		"/users/:id",
		func(req Request[routingRequest], res *Response[testResponse]) error {
			id := req.Ctx.Param("id")
			if id != "123" {
				t.Errorf("expected id '123', got '%s'", id)
			}
			res.Out = testResponse{ID: 123, Name: "testuser"}
			return nil
		},
	)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/users/123", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// TestRegisterRoute_BusinessError tests business error handling
func TestRegisterRoute_BusinessError(t *testing.T) {
	router := gin.New()

	RegisterPostRoute[testCreateRequest, testResponse](
		router.Group("/api"),
		"/users",
		func(req Request[testCreateRequest], res *Response[testResponse]) error {
			return NewBusinessError(20400, "user already exists")
		},
	)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/users", strings.NewReader(`{"name": "testuser", "age": 25}`))
	r.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, r)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["code"] != float64(20400) {
		t.Errorf("expected code 20400, got %v, body: %s", resp["code"], w.Body.String())
	}
	if resp["msg"] != "user already exists" {
		t.Errorf("expected msg 'user already exists', got %v", resp["msg"])
	}
}

// TestRegisterRoute_InvalidRequest tests invalid request binding
func TestRegisterRoute_InvalidRequest(t *testing.T) {
	router := gin.New()

	type requiredRequest struct {
		Name string `json:"name" binding:"required"`
	}

	RegisterPostRoute[requiredRequest, testResponse](
		router.Group("/api"),
		"/users",
		func(req Request[requiredRequest], res *Response[testResponse]) error {
			res.Out = testResponse{ID: 1, Name: "test"}
			return nil
		},
	)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/users", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Logf("response body: %s", w.Body.String())
	}
}

// TestRegisterPutRoute tests PUT route
func TestRegisterPutRoute(t *testing.T) {
	router := gin.New()

	RegisterPutRoute[testCreateRequest, testResponse](
		router.Group("/api"),
		"/users/:id",
		func(req Request[testCreateRequest], res *Response[testResponse]) error {
			id := req.Ctx.Param("id")
			res.Out = testResponse{ID: 123, Name: req.In.Name}
			_ = id
			return nil
		},
	)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/users/123", strings.NewReader(`{"name": "testuser"}`))
	r.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// TestRegisterDeleteRoute tests DELETE route
func TestRegisterDeleteRoute(t *testing.T) {
	router := gin.New()

	type deleteResponse struct {
		Status string `json:"status"`
	}

	RegisterDeleteRoute[routingRequest, deleteResponse](
		router.Group("/api"),
		"/users/:id",
		func(req Request[routingRequest], res *Response[deleteResponse]) error {
			res.Out = deleteResponse{Status: "ok"}
			return nil
		},
	)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/users/123", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["data"] == nil {
		t.Error("expected data in response")
	}
}

// TestRegisterRoute_SuccessPayloadPinsOut proves the router reads the Out
// the handler wrote: Response is passed by reference, so a success response
// carries the payload instead of the zero value (data was previously always
// null / fields zeroed because Response was passed by value).
func TestRegisterRoute_SuccessPayloadPinsOut(t *testing.T) {
	router := gin.New()

	RegisterGetRoute[routingRequest, testResponse](
		router.Group("/api"),
		"/users/:id",
		func(req Request[routingRequest], res *Response[testResponse]) error {
			res.Out = testResponse{ID: 5, Name: "x"}
			return nil
		},
	)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/users/1", nil)
	router.ServeHTTP(w, req)

	want := `{"code":20000,"msg":"success","trace":"","data":{"id":5,"name":"x"}}`
	if got := strings.TrimSpace(w.Body.String()); got != want {
		t.Errorf("body mismatch:\n got: %s\nwant: %s", got, want)
	}
}

// TestRegisterRoute_PostEmptyBody pins the pre-migration handler semantics:
// body-carrying handlers (POST/PUT/PATCH) called ShouldBindJSON
// unconditionally, so an empty body must surface as the same binding error
// (code 20100, msg "EOF") instead of skipping the bind and running the
// handler with a zero In value.
func TestRegisterRoute_PostEmptyBody(t *testing.T) {
	router := gin.New()

	RegisterPostRoute[testCreateRequest, testResponse](
		router.Group("/api"),
		"/users",
		func(req Request[testCreateRequest], res *Response[testResponse]) error {
			res.Out = testResponse{ID: 1, Name: req.In.Name}
			return nil
		},
	)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/users", http.NoBody)
	r.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, r)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(20100) {
		t.Errorf("expected code 20100 for empty POST body, got %v, body: %s", resp["code"], w.Body.String())
	}
	if resp["msg"] != "EOF" {
		t.Errorf("expected msg 'EOF', got %v, body: %s", resp["msg"], w.Body.String())
	}
}

// TestRegisterRoute_PutEmptyBody is the PUT counterpart of
// TestRegisterRoute_PostEmptyBody.
func TestRegisterRoute_PutEmptyBody(t *testing.T) {
	router := gin.New()

	RegisterPutRoute[testCreateRequest, testResponse](
		router.Group("/api"),
		"/users/:id",
		func(req Request[testCreateRequest], res *Response[testResponse]) error {
			res.Out = testResponse{ID: 1, Name: req.In.Name}
			return nil
		},
	)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/users/1", http.NoBody)
	r.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, r)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(20100) {
		t.Errorf("expected code 20100 for empty PUT body, got %v, body: %s", resp["code"], w.Body.String())
	}
	if resp["msg"] != "EOF" {
		t.Errorf("expected msg 'EOF', got %v, body: %s", resp["msg"], w.Body.String())
	}
}

// TestRegisterRoute_GetIgnoresBody pins that GET requests never bind a JSON
// body: pre-migration GET handlers only read path/query params and ignored
// any stray body entirely.
func TestRegisterRoute_GetIgnoresBody(t *testing.T) {
	router := gin.New()

	RegisterGetRoute[routingRequest, testResponse](
		router.Group("/api"),
		"/users/:id",
		func(req Request[routingRequest], res *Response[testResponse]) error {
			res.Out = testResponse{ID: 1, Name: "ok"}
			return nil
		},
	)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/users/1", strings.NewReader(`not json`))
	router.ServeHTTP(w, r)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(20000) {
		t.Errorf("expected GET with stray body to succeed (20000), got %v, body: %s", resp["code"], w.Body.String())
	}
}

// TestNewBusinessError tests business error creation
func TestNewBusinessError(t *testing.T) {
	err := NewBusinessError(20400, "test error")

	if err.Code != 20400 {
		t.Errorf("expected code 20400, got %d", err.Code)
	}
	if err.Message != "test error" {
		t.Errorf("expected message 'test error', got '%s'", err.Message)
	}
	if err.Error() != "test error" {
		t.Errorf("expected Error() to return message, got '%s'", err.Error())
	}
}

// TestRequest_Fields tests that Request has correct fields
func TestRequest_Fields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	RegisterGetRoute[routingRequest, testResponse](
		router.Group("/api"),
		"/test",
		func(req Request[routingRequest], res *Response[testResponse]) error {
			if req.Ctx == nil {
				t.Error("expected Ctx to be set")
			}
			if req.Ctx.Request == nil {
				t.Error("expected Ctx.Request to be set")
			}
			res.Out = testResponse{ID: 1, Name: "test"}
			return nil
		},
	)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/test", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// TestRegisterRoute_DefaultQueryParams tests default query parameter values
func TestRegisterRoute_DefaultQueryParams(t *testing.T) {
	router := gin.New()

	type listRequest struct {
		Limit  int `form:"limit,default=20"`
		Offset int `form:"offset,default=0"`
	}

	received := false
	RegisterGetRoute[listRequest, testResponse](
		router.Group("/api"),
		"/users",
		func(req Request[listRequest], res *Response[testResponse]) error {
			if req.In.Limit != 20 {
				t.Errorf("expected default limit 20, got %d", req.In.Limit)
			}
			if req.In.Offset != 0 {
				t.Errorf("expected default offset 0, got %d", req.In.Offset)
			}
			received = true
			res.Out = testResponse{ID: 1, Name: "test"}
			return nil
		},
	)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/users", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if !received {
		t.Error("handler was not called")
	}
}
