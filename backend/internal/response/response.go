package response

import (
	"net/http"
	"reflect"

	"github.com/gin-gonic/gin"
)

// 状态码定义
const (
	// 2xx 成功
	CodeSuccess = 20000

	// 4xx 客户端错误
	CodeBadRequest   = 20100 // 请求参数错误
	CodeUnauthorized = 20200 // 未认证：没登录 / 凭据失效
	// CodeForbidden 已认证但无权限。与 20200 分开，前端才能决定「去登录」
	// 还是「提示无权限」（issue #202；字符串码 FORBIDDEN 由 #224 落地）。
	CodeForbidden     = 20201 // 越权
	CodeNotFound      = 20300 // 资源不存在
	CodeBusinessError = 20400 // 业务逻辑错误
	CodeThirdPartyErr = 20500 // 第三方服务错误

	// 5xx 服务端错误
	CodeInternalError = 50000 // 服务端内部错误
)

// Response 统一响应结构
type Response struct {
	Code  int    `json:"code"`
	Msg   string `json:"msg"`
	Trace string `json:"trace"`
	Data  any    `json:"data"`
}

// 获取请求追踪 ID
func getTraceID(c *gin.Context) string {
	traceID := c.GetHeader("X-Request-ID")
	if traceID == "" {
		traceID = c.GetString("request_id")
	}
	return traceID
}

// normalizeResponseData recursively normalizes successful or error payloads so collection fields
// never serialize as JSON null. Root-level nil data is converted to an empty object to keep the
// response contract stable for clients.
func normalizeResponseData(data any) any {
	if data == nil {
		return map[string]any{}
	}

	normalized := normalizeReflectValue(reflect.ValueOf(data))
	if !normalized.IsValid() {
		return map[string]any{}
	}
	return normalized.Interface()
}

// normalizeReflectValue walks arbitrary response values and converts nil slices/maps into empty
// collections while preserving concrete types, exported struct fields, and scalar values.
func normalizeReflectValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}

	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return value
		}
		normalized := normalizeReflectValue(value.Elem())
		wrapped := reflect.New(value.Type()).Elem()
		if normalized.IsValid() {
			wrapped.Set(normalized)
		}
		return wrapped
	case reflect.Pointer:
		if value.IsNil() {
			return value
		}
		normalizedElem := normalizeReflectValue(value.Elem())
		clone := reflect.New(value.Type().Elem())
		clone.Elem().Set(normalizedElem)
		return clone
	case reflect.Struct:
		clone := reflect.New(value.Type()).Elem()
		clone.Set(value)
		for i := 0; i < value.NumField(); i++ {
			field := clone.Field(i)
			if !field.CanSet() {
				continue
			}
			field.Set(normalizeReflectValue(value.Field(i)))
		}
		return clone
	case reflect.Slice:
		if value.IsNil() {
			return reflect.MakeSlice(value.Type(), 0, 0)
		}
		clone := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			clone.Index(i).Set(normalizeReflectValue(value.Index(i)))
		}
		return clone
	case reflect.Array:
		clone := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			clone.Index(i).Set(normalizeReflectValue(value.Index(i)))
		}
		return clone
	case reflect.Map:
		if value.IsNil() {
			return reflect.MakeMapWithSize(value.Type(), 0)
		}
		clone := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			clone.SetMapIndex(iter.Key(), normalizeReflectValue(iter.Value()))
		}
		return clone
	default:
		return value
	}
}

// Success 成功响应
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Code:  CodeSuccess,
		Msg:   "success",
		Trace: getTraceID(c),
		Data:  normalizeResponseData(data),
	})
}

// SuccessWithMsg 带自定义消息的成功响应
func SuccessWithMsg(c *gin.Context, data any, msg string) {
	c.JSON(http.StatusOK, Response{
		Code:  CodeSuccess,
		Msg:   msg,
		Trace: getTraceID(c),
		Data:  normalizeResponseData(data),
	})
}

// Error 错误响应
func Error(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, Response{
		Code:  code,
		Msg:   msg,
		Trace: getTraceID(c),
		Data:  normalizeResponseData(nil),
	})
}

// ErrorWithData 带数据的错误响应
func ErrorWithData(c *gin.Context, code int, msg string, data any) {
	c.JSON(http.StatusOK, Response{
		Code:  code,
		Msg:   msg,
		Trace: getTraceID(c),
		Data:  normalizeResponseData(data),
	})
}

// BadRequest 请求参数错误
func BadRequest(c *gin.Context, msg string) {
	Error(c, CodeBadRequest, msg)
}

// Unauthorized 未授权
func Unauthorized(c *gin.Context, msg string) {
	Error(c, CodeUnauthorized, msg)
}

// Forbidden 已认证但无权限
func Forbidden(c *gin.Context, msg string) {
	Error(c, CodeForbidden, msg)
}

// NotFound 资源不存在
func NotFound(c *gin.Context, msg string) {
	Error(c, CodeNotFound, msg)
}

// BusinessError 业务逻辑错误
func BusinessError(c *gin.Context, msg string) {
	Error(c, CodeBusinessError, msg)
}

// ThirdPartyError 第三方服务错误
func ThirdPartyError(c *gin.Context, msg string) {
	Error(c, CodeThirdPartyErr, msg)
}

// InternalError 服务端内部错误
func InternalError(c *gin.Context, msg string) {
	Error(c, CodeInternalError, msg)
}

// PageResult 分页结果
type PageResult struct {
	Items      any   `json:"items"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalPages int   `json:"total_pages"`
}

// SuccessWithPage 带分页的成功响应
func SuccessWithPage(c *gin.Context, items any, total int64, page, pageSize int) {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}
	Success(c, PageResult{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}
