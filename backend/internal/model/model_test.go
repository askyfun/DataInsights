package model

import (
	"database/sql"
	"testing"
)

// TestDatasourceFields tests that Datasource struct has expected fields
func TestDatasourceFields(t *testing.T) {
	ds := Datasource{
		ID:           1,
		Name:         "test-datasource",
		Host:         "localhost",
		Port:         5432,
		DatabaseName: "testdb",
		Username:     "user",
		Password:     "pass",
	}

	if ds.ID != 1 {
		t.Errorf("expected ID=1, got %d", ds.ID)
	}
	if ds.Name != "test-datasource" {
		t.Errorf("expected Name=test-datasource, got %s", ds.Name)
	}
	if ds.Host != "localhost" {
		t.Errorf("expected Host=localhost, got %s", ds.Host)
	}
	if ds.Port != 5432 {
		t.Errorf("expected Port=5432, got %d", ds.Port)
	}
	if ds.DatabaseName != "testdb" {
		t.Errorf("expected DatabaseName=testdb, got %s", ds.DatabaseName)
	}
	if ds.Username != "user" {
		t.Errorf("expected Username=user, got %s", ds.Username)
	}
	if ds.Password != "pass" {
		t.Errorf("expected Password=pass, got %s", ds.Password)
	}
}

// TestDatasetFields tests that Dataset struct has expected fields
func TestDatasetFields(t *testing.T) {
	ds := Dataset{
		ID:           1,
		Name:         "test-dataset",
		DatasourceID: 1,
		TableName:    sql.NullString{String: "test_table", Valid: true},
		QuerySQL:     sql.NullString{String: "SELECT * FROM test", Valid: true},
		QueryType:    "sql",
		Columns:      `[]`,
	}

	if ds.ID != 1 {
		t.Errorf("expected ID=1, got %d", ds.ID)
	}
	if ds.Name != "test-dataset" {
		t.Errorf("expected Name=test-dataset, got %s", ds.Name)
	}
	if ds.DatasourceID != 1 {
		t.Errorf("expected DatasourceID=1, got %d", ds.DatasourceID)
	}
	if !ds.TableName.Valid || ds.TableName.String != "test_table" {
		t.Errorf("expected TableName=test_table, got %v", ds.TableName)
	}
	if !ds.QuerySQL.Valid || ds.QuerySQL.String != "SELECT * FROM test" {
		t.Errorf("expected QuerySQL, got %v", ds.QuerySQL)
	}
	if ds.QueryType != "sql" {
		t.Errorf("expected QueryType=sql, got %s", ds.QueryType)
	}
}

// TestChartFields tests that Chart struct has expected fields
func TestChartFields(t *testing.T) {
	c := Chart{
		ID:        1,
		Name:      "test-chart",
		DatasetID: 1,
		ChartType: "bar",
		Config:    `{"xAxis": "region", "yAxis": "sales"}`,
	}

	if c.ID != 1 {
		t.Errorf("expected ID=1, got %d", c.ID)
	}
	if c.Name != "test-chart" {
		t.Errorf("expected Name=test-chart, got %s", c.Name)
	}
	if c.DatasetID != 1 {
		t.Errorf("expected DatasetID=1, got %d", c.DatasetID)
	}
	if c.ChartType != "bar" {
		t.Errorf("expected ChartType=bar, got %s", c.ChartType)
	}
	if c.Config != `{"xAxis": "region", "yAxis": "sales"}` {
		t.Errorf("expected Config, got %s", c.Config)
	}
}
