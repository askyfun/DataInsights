// Package middleware holds the cross-cutting Gin middleware for the API.
//
// The bearer middleware is the single authentication choke point for both the
// human session token and the machine PAT (issue #183/#184). It delegates token
// resolution to auth.Service.Verify and, when allowPAT is false, refuses PATs —
// the guard that keeps a long-lived machine token from reaching user-management
// endpoints (#184 acceptance 3).
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"data-insights/internal/response"
	"data-insights/internal/router"
	"data-insights/internal/service/auth"
)

// PrincipalKey / RawTokenKey are the gin context keys the middleware writes and
// handlers read. Unexported to keep the string values private.
const (
	principalKey = "authPrincipal"
	rawTokenKey  = "authRawToken"
)

// Bearer returns a middleware that requires a valid `Authorization: Bearer <t>`
// header. allowPAT gates whether machine tokens are accepted at this route.
func Bearer(svc auth.Service, allowPAT bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := bearerToken(c.GetHeader("Authorization"))
		principal, err := svc.Verify(c.Request.Context(), raw, c.ClientIP(), c.Request.UserAgent())
		if err != nil {
			// Verify surfaces auth failures as router.BusinessError (code 20200);
			// anything else is an internal error → fail closed as unauthorized.
			if be, ok := err.(router.BusinessError); ok {
				response.Error(c, be.Code, be.Message)
			} else {
				response.Unauthorized(c, "authentication failed")
			}
			c.Abort()
			return
		}
		if !allowPAT && principal.Kind == "pat" {
			response.Unauthorized(c, "个人访问令牌不允许访问该端点")
			c.Abort()
			return
		}
		c.Set(principalKey, principal)
		c.Set(rawTokenKey, raw)
		c.Next()
	}
}

// bearerToken extracts the credential after a case-insensitive "Bearer " scheme.
func bearerToken(header string) string {
	const p = "bearer "
	if len(header) >= len(p) && strings.EqualFold(header[:len(p)], p) {
		return strings.TrimSpace(header[len(p):])
	}
	return ""
}

// PrincipalFrom reads the authenticated principal the Bearer middleware stored.
func PrincipalFrom(c *gin.Context) (*auth.Principal, bool) {
	v, ok := c.Get(principalKey)
	if !ok {
		return nil, false
	}
	p, ok := v.(*auth.Principal)
	return p, ok
}

// RawTokenFrom reads the plaintext bearer token (for self-revocation on logout).
func RawTokenFrom(c *gin.Context) string {
	v, _ := c.Get(rawTokenKey)
	s, _ := v.(string)
	return s
}
