package main

import (
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"

	"data-insights/internal/handler"
	"data-insights/internal/router"
	"data-insights/internal/service/chart"
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

	// Initialize handlers
	datasourceHandler := handler.NewDatasourceHandler(dsSvc)
	datasetHandler := handler.NewDatasetHandler(dsDatasetSvc)
	chartHandler := handler.NewChartHandler(dsChartSvc)
	shareHandler := handler.NewShareHandler(dsShareSvc)
	queryHandler := handler.NewQueryHandler(dsQuerySvc)

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
