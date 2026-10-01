package main

import (
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"

	"data-insights/internal/handler"
	"data-insights/internal/router"
	"data-insights/internal/service/alert"
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
//
// 返回预警 service，供 main.go 在装配后启动后台评估 goroutine（评估间隔与
// notifier 也由 main 注入，routes.go 不读环境变量）。
func SetupRoutes(r *gin.Engine, db *bun.DB, securityKey []byte, extractDatasourceID int, notifier alert.Notifier) alert.Service {
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
	// 指标预警（issue #155）：规则 CRUD + 后台评估。图表取数与通知都走窄接口
	// 注入；评估循环由 main.go 用返回值启动。
	alertSvc := alert.NewService(db)
	alertSvc.SetChartProvider(dsChartSvc)
	alertSvc.SetNotifier(notifier)

	// Initialize handlers
	datasourceHandler := handler.NewDatasourceHandler(dsSvc)
	datasetHandler := handler.NewDatasetHandler(dsDatasetSvc)
	chartHandler := handler.NewChartHandler(dsChartSvc)
	queryHandler := handler.NewQueryHandler(dsQuerySvc)
	dashboardHandler := handler.NewDashboardHandler(dsDashboardSvc)
	dashboardFolderHandler := handler.NewDashboardFolderHandler(dsFolderSvc)
	alertHandler := handler.NewAlertHandler(alertSvc)

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

	// Alert routes (generic router): 指标预警规则的 CRUD、触发历史与后台评估。
	// {id} 是 UUIDv7 字符串（非自增），按原样交给 service；DELETE 沿用
	// {"status":"ok"} 出口。
	alerts := api.Group("/alerts")
	router.RegisterGetRoute(alerts, "", alertHandler.List)
	router.RegisterPostRoute(alerts, "", alertHandler.Create)
	router.RegisterGetRoute(alerts, "/:id", alertHandler.Get)
	router.RegisterPutRoute(alerts, "/:id", alertHandler.Update)
	router.RegisterDeleteRoute(alerts, "/:id", alertHandler.Delete)
	router.RegisterGetRoute(alerts, "/:id/triggers", alertHandler.ListTriggers)

	return alertSvc
}
