package response

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// R-99（issue #224）稳定错误码与文案分离。
//
// 信封增量（向后兼容）：错误响应新增 error: { id, code, hint_key }。
//   - id    每次出错生成（uuid v7），供用户复制报障、与日志对账（R-96/R-97 消费）。
//   - code  字符串稳定标识。旧整数 code 不改义；调用方分支判断只允许用 error.code，
//     不允许匹配 msg。对外承诺：新增码 = minor，码义变更 = major。
//   - hint_key 前端 i18n 词典键，不是终端用户文案；后端不下发中文。
//
// 首批只拆最疼的 5 个（不做 50000 的全量拆分）：未分类错误只带 id。

// CodeForbidden 越权业务码：登录了但没权限（20200 保留给未认证）。
const CodeForbidden = 20201

// 字符串稳定码（对外契约，与 api.md §1.3、frontend errorCodes.json、
// api/openapi.yaml 的 ErrorDetail.code 枚举四方一致，由一致性测试守卫）。
const (
	StableUnauthenticated       = "UNAUTHENTICATED"
	StableForbidden             = "FORBIDDEN"
	StableDatasourceUnreachable = "DATASOURCE_UNREACHABLE"
	StableQueryExecFailed       = "QUERY_EXEC_FAILED"
	StableInvalidConfig         = "INVALID_CONFIG"
)

// hint_key 词典键（前端 i18n 渲染 zh/en，见 useLocale.ts 的 errors.* 段）。
const (
	HintUnauthenticated       = "errors.auth.unauthenticated"
	HintForbidden             = "errors.auth.forbidden"
	HintDatasourceUnreachable = "errors.datasource.unreachable"
	HintQueryExecFailed       = "errors.query.execution_failed"
	HintInvalidConfig         = "errors.config.invalid"
)

// internalFallbackMsg 内部错误的通用兜底文案：内部原文（dial tcp / sql: 等）
// 一律不再进入响应体，只进日志（InternalErrorErr）。
const internalFallbackMsg = "internal server error"

// stableByBizCode 整数码 → 稳定码的保守映射：只收语义单一的整数码
// （20200 未认证 / 20201 越权）。50000 一码多义，只有产生源头显式
// WrapCode 标注过的错误才携带稳定码，未分类错误不填 code。
var stableByBizCode = map[int]struct {
	Stable string
	Hint   string
}{
	CodeUnauthorized: {StableUnauthenticated, HintUnauthenticated},
	CodeForbidden:    {StableForbidden, HintForbidden},
}

// ErrorDetail 错误响应信封的稳定标识增量。
type ErrorDetail struct {
	ID      string `json:"id"`
	Code    string `json:"code,omitempty"`
	HintKey string `json:"hint_key,omitempty"`
}

// newErrorDetail 生成一条错误标识；按整数码查保守映射补 code/hint_key。
func newErrorDetail(bizCode int) *ErrorDetail {
	detail := &ErrorDetail{}
	if id, err := uuid.NewV7(); err == nil {
		detail.ID = id.String()
	}
	if m, ok := stableByBizCode[bizCode]; ok {
		detail.Code, detail.HintKey = m.Stable, m.Hint
	}
	return detail
}

// CodedError 内部错误附带的对外稳定码标注。由产生错误的源头（数据源
// 连接、查询执行等）通过 WrapCode 打标，响应层经 CodedFrom 解出写进信封。
type CodedError struct {
	StableCode string
	HintKey    string
	Err        error
}

func (e *CodedError) Error() string { return e.Err.Error() }
func (e *CodedError) Unwrap() error { return e.Err }

// WrapCode 给内部错误打对外稳定码标注。
func WrapCode(err error, stable, hint string) error {
	if err == nil {
		return nil
	}
	return &CodedError{StableCode: stable, HintKey: hint, Err: err}
}

// CodedFrom 从错误链里解出稳定码标注。
func CodedFrom(err error) (*CodedError, bool) {
	var ce *CodedError
	if errors.As(err, &ce) {
		return ce, true
	}
	return nil, false
}

// writeError 写错误信封：detail 由调用方决定（见 newErrorDetail / ErrorCoded）。
func writeError(c *gin.Context, code int, msg string, detail *ErrorDetail) {
	c.JSON(http.StatusOK, Response{
		Code:  code,
		Msg:   msg,
		Trace: getTraceID(c),
		Data:  normalizeResponseData(nil),
		Error: detail,
	})
}

// ErrorCoded 显式三元组的错误响应（整数码 + 稳定码 + hint key）。
// 调用场景：错误源头已经 WrapCode 标注、或固定语义的守卫（Forbidden）。
func ErrorCoded(c *gin.Context, bizCode int, stable, hint, msg string) {
	detail := newErrorDetail(bizCode)
	detail.Code, detail.HintKey = stable, hint
	writeError(c, bizCode, msg, detail)
}

// Forbidden 越权（20201 + FORBIDDEN）。登录了但没权限；未认证请用 20200。
func Forbidden(c *gin.Context, msg string) {
	ErrorCoded(c, CodeForbidden, StableForbidden, HintForbidden, msg)
}

// InternalErrorErr 内部错误的标准出口：原文只进日志（与 error_id 对账），
// 响应一律通用兜底文案；错误链上有 WrapCode 标注时把稳定码写进信封。
func InternalErrorErr(c *gin.Context, err error) {
	detail := newErrorDetail(CodeInternalError)
	msg := internalFallbackMsg
	if coded, ok := CodedFrom(err); ok {
		detail.Code, detail.HintKey = coded.StableCode, coded.HintKey
	}
	if err != nil {
		slog.Error("request failed", "error_id", detail.ID, "trace", getTraceID(c), "error", err)
	}
	writeError(c, CodeInternalError, msg, detail)
}
