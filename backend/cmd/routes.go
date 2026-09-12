package main

import (
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"

	"dataray/internal/handler"
	"dataray/internal/router"
	"dataray/internal/service/chart"
	"dataray/internal/service/dataset"
	"dataray/internal/service/datasource"
	"dataray/internal/service/share"
)

// SetupRoutes configures all routes
func SetupRoutes(r *gin.Engine, db *bun.DB, securityKey []byte) {
	// Initialize services
	dsSvc := datasource.NewService(db)
	dsSvc.SetSecurityKey(securityKey)
	dsDatasetSvc := dataset.NewService(db)
	dsDatasetSvc.SetSecurityKey(securityKey)
	dsChartSvc := chart.NewService(db)
	dsChartSvc.SetSecurityKey(securityKey)
	dsShareSvc := share.NewService(db)

	// Initialize handlers
	datasourceHandler := handler.NewDatasourceHandler(dsSvc)
	datasetHandler := handler.NewDatasetHandler(dsDatasetSvc)
	chartHandler := handler.NewChartHandler(dsChartSvc)
	shareHandler := handler.NewShareHandler(dsShareSvc)

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

	// Share view route (no /api prefix). Not a generic route on purpose: its
	// success response is a 302 redirect, which the JSON-envelope router
	// cannot emit (see the ShareHandler.View doc comment).
	r.GET("/share/:token", shareHandler.View)
}
