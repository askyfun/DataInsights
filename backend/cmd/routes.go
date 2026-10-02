package main

import (
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"

	"data-insights/internal/handler"
	"data-insights/internal/middleware"
	"data-insights/internal/router"
	"data-insights/internal/service/auth"
	"data-insights/internal/service/chart"
	"data-insights/internal/service/dashboard"
	"data-insights/internal/service/dataset"
	"data-insights/internal/service/datasource"
	"data-insights/internal/service/queryrecord"
)

// SetupRoutes configures all routes.
//
// extractDatasourceID 是抽取存储数据源 id（EXTRACT_DATASOURCE_ID，issue #118 预留，
// 0=未启用）：数据源列表隐藏它、数据集创建/改指守卫拒绝它、图表查询对 extract
// 模式数据集显式报错。
func SetupRoutes(r *gin.Engine, db *bun.DB, securityKey []byte, extractDatasourceID int) {
	// Initialize services
	dsSvc := datasource.NewService(db)
	dsSvc.SetSecurityKey(securityKey)
	dsSvc.SetExtractDatasourceID(extractDatasourceID)
	dsDatasetSvc := dataset.NewService(db)
	dsDatasetSvc.SetSecurityKey(securityKey)
	dsDatasetSvc.SetExtractDatasourceID(extractDatasourceID)
	dsChartSvc := chart.NewService(db)
	dsChartSvc.SetSecurityKey(securityKey)
	dsChartSvc.SetExtractDatasourceID(extractDatasourceID)
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

	// Auth routes (R-82 / issue #183). Progressive enforcement: only these
	// endpoints are behind the bearer middleware for now — the 40 existing
	// resource endpoints stay open at L0 and opt in as Phase C rolls out.
	// register/login are public (bootstrap + sign-in); me/logout require a
	// token. allowPAT=true here because both are self-service identity
	// operations, not user-management; the PAT-forbidden guard lives on the
	// token-management endpoints (#184).
	authSvc := auth.NewService(db)
	authHandler := handler.NewAuthHandler(authSvc)
	authPublic := api.Group("/auth")
	router.RegisterPostRoute(authPublic, "/register", authHandler.Register)
	router.RegisterPostRoute(authPublic, "/login", authHandler.Login)
	authProtected := api.Group("/auth")
	authProtected.Use(middleware.Bearer(authSvc, true))
	router.RegisterGetRoute(authProtected, "/me", authHandler.Me)
	// DELETE, not POST: the generic router binds a JSON body for every POST,
	// and logout carries none — a POST would fail on "EOF" before the handler
	// runs. DELETE never binds a body, so revocation actually executes.
	router.RegisterDeleteRoute(authProtected, "/logout", authHandler.Logout)
}
