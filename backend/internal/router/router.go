package router

import (
	"net/http"
	"reflect"

	"dataray/internal/response"

	"github.com/gin-gonic/gin"
)

// Request wraps the input type for API handlers
// Supports Query, Path, and JSON body binding
type Request[In any] struct {
	In  In
	Ctx *gin.Context // Access to gin context for path params, headers, etc.
}

// Response wraps the output type for API handlers
type Response[Out any] struct {
	Out Out
}

// API defines a generic API handler function type
// In: request body type
// Out: response body type
// Response is passed by pointer so writes to res.Out are visible to the
// router: a by-value Response would silently discard every handler payload
// (the router would always respond with the zero value of Out).
type API[In, Out any] func(req Request[In], res *Response[Out]) error

// RegisterRoute registers an API handler to gin router
// Supports binding query parameters and JSON body to In type
// Automatically wraps response with unified response format
func RegisterRoute[In, Out any](
	r *gin.RouterGroup,
	method string,
	path string,
	api API[In, Out],
) {
	r.Handle(method, path, func(c *gin.Context) {
		// Create request and response containers
		var in In
		req := Request[In]{In: in, Ctx: c}
		res := &Response[Out]{}

		// Bind query parameters first (if In has query tags)
		if err := c.ShouldBindQuery(&req.In); err != nil {
			response.BadRequest(c, err.Error())
			return
		}

		// Bind JSON body for methods that carry one. This mirrors the
		// per-handler behavior it replaces: POST/PUT/PATCH handlers called
		// ShouldBindJSON unconditionally (so an empty body yields the same
		// EOF binding error), while GET/DELETE handlers ignored bodies.
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			if err := c.ShouldBindJSON(&req.In); err != nil {
				response.BadRequest(c, err.Error())
				return
			}
		}

		// Call the API handler
		if err := api(req, res); err != nil {
			// Check if error is a business error with code
			if bizErr, ok := any(err).(BusinessError); ok {
				response.Error(c, bizErr.Code, bizErr.Message)
				return
			}
			response.InternalError(c, err.Error())
			return
		}

		// Success response
		response.Success(c, res.Out)
	})
}

// RegisterGetRoute registers a GET route (query only)
func RegisterGetRoute[In, Out any](
	r *gin.RouterGroup,
	path string,
	api API[In, Out],
) {
	RegisterRoute(r, "GET", path, api)
}

// RegisterPostRoute registers a POST route (body only typically)
func RegisterPostRoute[In, Out any](
	r *gin.RouterGroup,
	path string,
	api API[In, Out],
) {
	RegisterRoute(r, "POST", path, api)
}

// RegisterPutRoute registers a PUT route
func RegisterPutRoute[In, Out any](
	r *gin.RouterGroup,
	path string,
	api API[In, Out],
) {
	RegisterRoute(r, "PUT", path, api)
}

// RegisterDeleteRoute registers a DELETE route
func RegisterDeleteRoute[In, Out any](
	r *gin.RouterGroup,
	path string,
	api API[In, Out],
) {
	RegisterRoute(r, "DELETE", path, api)
}

// BusinessError represents a business logic error with code
type BusinessError struct {
	Code    int
	Message string
}

func (e BusinessError) Error() string {
	return e.Message
}

// NewBusinessError creates a new business error
func NewBusinessError(code int, msg string) BusinessError {
	return BusinessError{Code: code, Message: msg}
}

// Helper to check if a type has query fields
func hasQueryFields(v any) bool {
	if v == nil {
		return false
	}
	t := reflect.TypeOf(v).Elem()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("form")
		if tag != "" && tag != "-" {
			return true
		}
	}
	return false
}
