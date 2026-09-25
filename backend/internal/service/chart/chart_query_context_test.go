package chart

import (
	"context"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// chartContextColumns 是 bi_chart 的读取列（含 deleted_at——本方法的全部意义就是
// 把它读进来判软删）。
func chartContextColumns() []string {
	return []string{"id", "name", "dataset_id", "chart_type", "config", "created_at", "updated_at", "deleted_at"}
}

// newChartContextService 组装一个跑在 sqlmock 上的 chartService，并返回捕获的 SQL。
func newChartContextService(t *testing.T) (*chartService, sqlmock.Sqlmock, *[]string) {
	t.Helper()
	executed := &[]string{}
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(chartCaptureMatcher(executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &chartService{db: bun.NewDB(sqlDB, pgdialect.New())}, mock, executed
}

// v1FilterConfig 是含三条自身过滤条件的 v1 持久化 config，其中 region 出现两次
// （去重保序的素材）。
const v1FilterConfig = `{"chartType":"bar","query":{
	"dimensionGroups":[{"id":"g1","fields":["region"]}],
	"metricGroups":[{"id":"g2","fields":["amount"]}],
	"filters":[
		{"field":"region","operator":"in","value":["华东"]},
		{"field":"amount","operator":"gt","value":100},
		{"field":"region","operator":"eq","value":"华南"}
	]}}`

// TestChartQueryContextMissing 行不存在是「不存在」这一态的信息，不是错误：
// 盘级取数要据此渲染 chart_missing 占位，而不是让整盘失败。
func TestChartQueryContextMissing(t *testing.T) {
	s, mock, executed := newChartContextService(t)
	mock.ExpectQuery(`SELECT .* FROM "bi_chart"`).
		WillReturnRows(sqlmock.NewRows(chartContextColumns()))

	got, err := s.ChartQueryContext(context.Background(), 123)
	if err != nil {
		t.Fatalf("行不存在必须返回 nil error，实际: %v", err)
	}
	if got == nil || got.Exists || got.Deleted {
		t.Fatalf("期望 Exists=false 的元信息，实际 %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	// ⚠️ 哨兵：这里「故意不带」deleted_at IS NULL，否则永远分不出 chart_missing
	// 与 chart_deleted。
	if sel := firstStmtChart(*executed, "SELECT"); strings.Contains(sel, "deleted_at IS NULL") {
		t.Fatalf("ChartQueryContext 不得带软删过滤（否则两态无法区分）: %s", sel)
	}
}

// TestChartQueryContextDeleted 软删行同样读得出来，且 Deleted=true 是它与
// chart_missing 的唯一区别。
func TestChartQueryContextDeleted(t *testing.T) {
	s, mock, _ := newChartContextService(t)
	mock.ExpectQuery(`SELECT .* FROM "bi_chart"`).
		WillReturnRows(sqlmock.NewRows(chartContextColumns()).AddRow(
			42, "已删的图", 7, "bar", "{}", nil, nil, time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)))

	got, err := s.ChartQueryContext(context.Background(), 42)
	if err != nil {
		t.Fatalf("ChartQueryContext: %v", err)
	}
	if !got.Exists || !got.Deleted {
		t.Fatalf("期望 Exists=true Deleted=true，实际 %+v", got)
	}
	if got.DatasetID != 7 {
		t.Errorf("DatasetID = %d, want 7", got.DatasetID)
	}
}

// TestChartQueryContextOwnFilterFields 正常行：数据集 id 与图表自身过滤字段都要
// 带出来，且字段去重保序（同名多条件的先后不影响结果）。
func TestChartQueryContextOwnFilterFields(t *testing.T) {
	s, mock, _ := newChartContextService(t)
	mock.ExpectQuery(`SELECT .* FROM "bi_chart"`).
		WillReturnRows(sqlmock.NewRows(chartContextColumns()).AddRow(
			42, "月度 GMV", 7, "bar", v1FilterConfig, nil, nil, nil))

	got, err := s.ChartQueryContext(context.Background(), 42)
	if err != nil {
		t.Fatalf("ChartQueryContext: %v", err)
	}
	if !got.Exists || got.Deleted {
		t.Fatalf("期望 Exists=true Deleted=false，实际 %+v", got)
	}
	if got.DatasetID != 7 {
		t.Errorf("DatasetID = %d, want 7", got.DatasetID)
	}
	want := []string{"region", "amount"}
	if len(got.OwnFilterFields) != len(want) {
		t.Fatalf("OwnFilterFields = %v, want %v（去重保序）", got.OwnFilterFields, want)
	}
	for i := range want {
		if got.OwnFilterFields[i] != want[i] {
			t.Fatalf("OwnFilterFields = %v, want %v（去重保序）", got.OwnFilterFields, want)
		}
	}
}

// TestChartQueryContextUnparseableConfigIsNotAnError 解析不出查询（旧结构 / 空
// config）不是错误：图表没有自身筛选是正常情况，字段留空切片即可。
func TestChartQueryContextUnparseableConfigIsNotAnError(t *testing.T) {
	for _, config := range []string{"{}", "", "{bad", `{"version":1,"query":{"dimensionGroups":[],"metricGroups":[]}}`} {
		s, mock, _ := newChartContextService(t)
		mock.ExpectQuery(`SELECT .* FROM "bi_chart"`).
			WillReturnRows(sqlmock.NewRows(chartContextColumns()).AddRow(
				42, "裸数据行图", 7, "table", config, nil, nil, nil))

		got, err := s.ChartQueryContext(context.Background(), 42)
		if err != nil {
			t.Fatalf("config %q: 不期望错误，实际 %v", config, err)
		}
		if got.OwnFilterFields == nil || len(got.OwnFilterFields) != 0 {
			t.Errorf("config %q: 期望空切片，实际 %#v", config, got.OwnFilterFields)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("config %q: expectations: %v", config, err)
		}
	}
}
