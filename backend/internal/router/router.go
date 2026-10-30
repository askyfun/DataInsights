package router

import (
	"net/http"

	"data-insights/internal/response"

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
				reportError(c, bizErr.Code, bizErr)
				response.Error(c, bizErr.Code, bizErr.Message)
				return
			}
			reportError(c, response.CodeInternalError, err)
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

// ErrorReporter 是统一错误出口的上报钩子（R-98）：收到业务码与原始错误，由
// 实现方决定是否/如何上报（monitor 包的 Sentry 实现只上报 50000/20500）。
type ErrorReporter func(c *gin.Context, code int, err error)

// errorReporter 是进程级钩子，由 main 装配处恰好设置一次（Sentry 激活时）；
// 未设置时错误只走响应信封，不产生任何上报副作用。
var errorReporter ErrorReporter

// SetErrorReporter 安装统一错误出口的上报钩子。只在进程装配阶段调用一次；
// 传 nil 撤销（测试用例负责保存并恢复原值）。
func SetErrorReporter(r ErrorReporter) { errorReporter = r }

// ErrorReporterOf 返回当前钩子（测试保存/恢复用；与 SetErrorReporter 成对）。
func ErrorReporterOf() ErrorReporter { return errorReporter }

// reportError 在 RegisterRoute 的错误出口调用上报钩子。响应信封的写出不受
// 钩子影响——上报失败/不存在都不能改变 API 的响应语义。
func reportError(c *gin.Context, code int, err error) {
	if errorReporter != nil {
		errorReporter(c, code, err)
	}
}
