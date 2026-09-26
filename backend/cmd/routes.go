package main

import (
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"

	"data-insights/internal/handler"
	"data-insights/internal/router"
	"data-insights/internal/service/chart"
	"data-insights/internal/service/dashboard"
	"data-insights/internal/service/dataset"
	"data-insights/internal/service/datasource"
	"data-insights/internal/service/queryrecord"
	"data-insights/internal/service/share"
)

// SetupRoutes configures all routes.
//
// serveWebUI 表示本进程同时托管前端构建产物（单镜像单容器的形态）。它会改变
// /share/:token 的归属：见文末说明。
func SetupRoutes(r *gin.Engine, db *bun.DB, securityKey []byte, serveWebUI bool) {
	// Initialize services
	dsSvc := datasource.NewService(db)
	dsSvc.SetSecurityKey(securityKey)
	dsDatasetSvc := dataset.NewService(db)
	dsDatasetSvc.SetSecurityKey(securityKey)
	dsChartSvc := chart.NewService(db)
	dsChartSvc.SetSecurityKey(securityKey)
	dsShareSvc := share.NewService(db)
	dsQuerySvc := queryrecord.NewService(db)
	dsDashboardSvc := dashboard.NewService(db)
	// 盘级批量取数（POST /api/dashboards/{id}/query）逐块复用图表取数管道：
	// 注入在装配期完成，dashboard 侧只依赖一个窄接口（见 service/dashboard/query.go）。
	dsDashboardSvc.SetChartProvider(dsChartSvc)
	// 归档文件夹树（第一期只归档仪表盘）与仪表盘同库同包，但独立成一个 service，
	// 因为两者的生命周期守卫不同（夹要查子夹/子盘/环）。
	dsFolderSvc := dashboard.NewFolderService(db)

	// Initialize handlers
	datasourceHandler := handler.NewDatasourceHandler(dsSvc)
	datasetHandler := handler.NewDatasetHandler(dsDatasetSvc)
	chartHandler := handler.NewChartHandler(dsChartSvc)
	shareHandler := handler.NewShareHandler(dsShareSvc)
	queryHandler := handler.NewQueryHandler(dsQuerySvc)
	dashboardHandler := handler.NewDashboardHandler(dsDashboardSvc)
	dashboardFolderHandler := handler.NewDashboardFolderHandler(dsFolderSvc)

	// API routes
	api := r.Group("/api")

	// Datasource routes (generic router)
	ds := api.Group("/datasources")
	router.RegisterGetRoute(ds, "", datasourceHandler.List)
	router.RegisterPostRoute(ds, "", datasourceHandler.Create)
	router.RegisterGetRoute(ds, "/:id", datasourceHandler.Get)
	router.RegisterPutRoute(ds, "/:id", datasourceHandler.Update)
	router.RegisterDeleteRoute(ds, "/:id", datasourceHandler.Delete)
	router.RegisterPostRoute(ds, "/test", datasourceHandler.TestConnection)
	router.RegisterGetRoute(ds, "/:id/tables", datasourceHandler.GetTables)
	router.RegisterGetRoute(ds, "/:id/tables/:table/columns", datasourceHandler.GetColumns)
	router.RegisterGetRoute(ds, "/:id/tables/:table/data", datasourceHandler.GetTableData)
	router.RegisterPostRoute(ds, "/:id/preview", datasourceHandler.Preview)
	router.RegisterPostRoute(ds, "/:id/field-distribution", datasourceHandler.GetFieldDistribution)

	// Dataset routes (generic router)
	datasets := api.Group("/datasets")
	router.RegisterGetRoute(datasets, "", datasetHandler.List)
	router.RegisterPostRoute(datasets, "", datasetHandler.Create)
	router.RegisterGetRoute(datasets, "/:id", datasetHandler.Get)
	router.RegisterPutRoute(datasets, "/:id", datasetHandler.Update)
	router.RegisterDeleteRoute(datasets, "/:id", datasetHandler.Delete)
	router.RegisterGetRoute(datasets, "/:id/columns", datasetHandler.GetColumns)
	router.RegisterPostRoute(datasets, "/:id/columns", datasetHandler.UpdateColumns)
	router.RegisterGetRoute(datasets, "/:id/preview", datasetHandler.Preview)
	router.RegisterPostRoute(datasets, "/:id/query", datasetHandler.Query)

	// Chart routes (generic router)
	charts := api.Group("/charts")
	router.RegisterGetRoute(charts, "", chartHandler.List)
	router.RegisterPostRoute(charts, "", chartHandler.Create)
	router.RegisterGetRoute(charts, "/:id", chartHandler.Get)
	router.RegisterPutRoute(charts, "/:id", chartHandler.Update)
	router.RegisterDeleteRoute(charts, "/:id", chartHandler.Delete)
	router.RegisterGetRoute(charts, "/:id/data", chartHandler.GetData)
	router.RegisterPostRoute(charts, "/query", chartHandler.Query)
	// Chart-side reference count (dashboard handler: it scans
	// bi_dashboard.layout_json, but the route belongs to the chart resource).
	router.RegisterGetRoute(charts, "/:id/references", dashboardHandler.ListChartReferences)

	// Share routes (generic router)
	shares := api.Group("/shares")
	router.RegisterGetRoute(shares, "", shareHandler.List)
	router.RegisterPostRoute(shares, "", shareHandler.Create)
	router.RegisterGetRoute(shares, "/:token", shareHandler.Get)
	router.RegisterPostRoute(shares, "/:token/verify", shareHandler.Verify)

	// Query record routes (generic router): 地址栏即分享的落库与寻址。
	// {q} 是 idcodec 的 21 位 base58 短码，不是数据库里的 uuid 原文。
	queries := api.Group("/queries")
	router.RegisterPostRoute(queries, "", queryHandler.Save)
	router.RegisterGetRoute(queries, "/:q", queryHandler.Get)

	// Dashboard routes (generic router): 12 列栅格容器的 CRUD 与软删。
	// {id} 是 UUID 字符串（非自增），非法形态由 handler 转 20100。
	// /:id/query 是盘级批量取数：筛选合并在后端完成，逐块返回结果（PRD §6.3/§8.3）。
	dashboards := api.Group("/dashboards")
	router.RegisterGetRoute(dashboards, "", dashboardHandler.List)
	router.RegisterPostRoute(dashboards, "", dashboardHandler.Create)
	router.RegisterGetRoute(dashboards, "/:id", dashboardHandler.Get)
	router.RegisterPutRoute(dashboards, "/:id", dashboardHandler.Update)
	router.RegisterDeleteRoute(dashboards, "/:id", dashboardHandler.Delete)
	router.RegisterPostRoute(dashboards, "/:id/query", dashboardHandler.Query)

	// Dashboard folder routes (generic router): 归档树的扁平读出口 + CRUD。
	// 独立前缀 /api/dashboard-folders（而非 dashboards 组下的静态段）：文件夹是
	// 自己的资源，且它的 {id} 同样是 UUID，与仪表盘共用一组会让 parseID 的归属含糊。
	// 列表不分页、不返回嵌套结构（树由前端按 parent_id 组装）。
	dashboardFolders := api.Group("/dashboard-folders")
	router.RegisterGetRoute(dashboardFolders, "", dashboardFolderHandler.List)
	router.RegisterPostRoute(dashboardFolders, "", dashboardFolderHandler.Create)
	router.RegisterGetRoute(dashboardFolders, "/:id", dashboardFolderHandler.Get)
	router.RegisterPutRoute(dashboardFolders, "/:id", dashboardFolderHandler.Update)
	router.RegisterDeleteRoute(dashboardFolders, "/:id", dashboardFolderHandler.Delete)

	// Share view route (no /api prefix). Not a generic route on purpose: its
	// success response is a 302 redirect, which the JSON-envelope router
	// cannot emit (see the ShareHandler.View doc comment).
	//
	// 只在纯 API 模式下注册。托管前端时 /share/<token> 必须留给前端路由：
	// 分享页本身就是 SPA 的 /share/:token，后端再插一条同路径的 302 会抢先
	// 命中，把页面重定向到一个渲染不出东西的地址上去（此前 nginx 把 /share
	// 整段挡在后端之外，所以这条路由在生产环境其实一直是死的）。
	if !serveWebUI {
		r.GET("/share/:token", shareHandler.View)
	}
}
