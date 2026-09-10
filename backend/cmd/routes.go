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

	// Datasource routes
	ds := api.Group("/datasources")
	ds.GET("", datasourceHandler.List)
	ds.POST("", datasourceHandler.Create)
	router.RegisterGetRoute(ds, "/:id", datasourceHandler.Get)
	ds.PUT("/:id", datasourceHandler.Update)
	ds.DELETE("/:id", datasourceHandler.Delete)
	ds.POST("/test", datasourceHandler.TestConnection)
	ds.GET("/:id/tables", datasourceHandler.GetTables)
	ds.GET("/:id/tables/:table/columns", datasourceHandler.GetColumns)
	ds.GET("/:id/tables/:table/data", datasourceHandler.GetTableData)
	ds.POST("/:id/preview", datasourceHandler.Preview)
	ds.POST("/:id/field-distribution", datasourceHandler.GetFieldDistribution)

	// Dataset routes
	datasets := api.Group("/datasets")
	datasets.GET("", datasetHandler.List)
	datasets.POST("", datasetHandler.Create)
	datasets.GET("/:id", datasetHandler.Get)
	datasets.DELETE("/:id", datasetHandler.Delete)
	datasets.GET("/:id/columns", datasetHandler.GetColumns)
	datasets.POST("/:id/columns", datasetHandler.UpdateColumns)
	datasets.GET("/:id/preview", datasetHandler.Preview)
	datasets.POST("/:id/query", datasetHandler.Query)

	// Chart routes
	charts := api.Group("/charts")
	charts.GET("", chartHandler.List)
	charts.POST("", chartHandler.Create)
	charts.GET("/:id", chartHandler.Get)
	charts.PUT("/:id", chartHandler.Update)
	charts.DELETE("/:id", chartHandler.Delete)
	charts.GET("/:id/data", chartHandler.GetData)
	charts.POST("/query", chartHandler.Query)

	// Share routes
	shares := api.Group("/shares")
	shares.GET("", shareHandler.List)
	shares.POST("", shareHandler.Create)
	shares.GET("/:token", shareHandler.Get)
	shares.POST("/:token/verify", shareHandler.Verify)

	// Share view route (no /api prefix)
	r.GET("/share/:token", shareHandler.View)
}
