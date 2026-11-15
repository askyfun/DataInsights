package main

import "strings"

// R-92（issue #217）路由鉴权分类表。这张表是本版全部 /api 端点的分组账本：
//   - public：永久匿名白名单（用例钉死，不许随重构漂移）；
//   - enforce：AUTH_ENFORCE=on 时要求有效 Bearer 的端点（默认；未列出即 enforce）。
//
// classifyRoute 被 AuthGuard 用于运行期判定，也被路由策略守卫测试用于遍历
// gin.Engine.Routes() 校验「没有未分类路由」。新增端点若落在未登记的分组前缀下，
// 守卫测试会失败，强制维护者显式归类。

// authzPublic 是「方法 + 完整路由模板」→ public 的精确白名单。
// 只放真正需要匿名的：健康检查、?q= 分享、auth 的 bootstrap/登录/注册/找回。
var authzPublic = map[string]bool{
	"GET /api/queries/:q":            true, // ?q= 分享快照（只读、地址栏即分享）
	"GET /api/auth/bootstrap-status": true,
	"GET /api/auth/config":           true,
	"POST /api/auth/login":           true,
	"POST /api/auth/register":        true,
	"POST /api/auth/recover-admin":   true,
}

// authzEnforcedGroups 是所有「on 时强制鉴权」的资源分组前缀。默认所有 /api 段
// 都应落在这里之一（或其精确白名单）；未命中即未分类。
var authzEnforcedGroups = []string{
	"/api/datasources",
	"/api/datasets",
	"/api/charts",
	"/api/dashboards",
	"/api/dashboard-folders",
	"/api/alerts",
	"/api/sample-dataset",
	"/api/tokens",
	"/api/users",
	"/api/auth/",
	"/api/queries",
}

// classifyRoute 返回 (class, ok)：class ∈ {"public","enforce"}；ok=false 表示未分类。
// 非 /api 路径（/health、静态资源）一律 public。白名单优先于分组前缀。
func classifyRoute(method, fullPath string) (string, bool) {
	if !strings.HasPrefix(fullPath, "/api/") {
		return "public", true
	}
	if authzPublic[method+" "+fullPath] {
		return "public", true
	}
	for _, g := range authzEnforcedGroups {
		if strings.HasPrefix(fullPath, g) {
			return "enforce", true
		}
	}
	return "", false
}
