package dashboard

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"data-insights/internal/domain/entity"
)

// zz_ 前缀是本仓库的探针约定（留工作区、不入库，见 docs/pitfalls.md 的
// zz_repro_op_test.go）。本探针按 docs/pitfalls.md「跨库写 SQL 必须真跑一次」的
// 规矩，对真实 PostgreSQL 跑一遍完整往返：sqlmock 抓不出 JSONB 列收到 string
// 参数、uuid 主键绑定、bun 的 `@\?` 转义在真实驱动下的形态。
//
// 未设置 TEST_DATABASE_URL 时跳过，因此不影响 go test ./...。它会写入一行
// synthetic 数据并在结尾物理删除（仅此一处允许真删）。
// 主理人可决定：转成 //go:build integration 的正式集成用例，或直接删除。
func TestZZRealPGDashboardRoundTrip(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// t.Cleanup 是 LIFO：先注册关连接，后注册删行 → 删行先跑。
	t.Cleanup(func() { _ = sqldb.Close() })
	db := bun.NewDB(sqldb, pgdialect.New())
	svc := NewService(db).(*dashboardService)

	const chartID = 987654321
	layout := `{"version":1,"grid":{"cols":12},"widgets":[{"widgetId":"w-real","type":"chart","x":0,"y":0,"w":6,"h":4,"chartId":987654321}]}`

	created, err := svc.Create(context.Background(), entity.DashboardCreateRequest{
		Name: "zz-realpg-verification", Description: ptr("真库验证用，跑完即删"),
		LayoutJSON: layout,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DELETE FROM bi_dashboard WHERE id = ?", created.ID); err != nil {
			t.Errorf("清理失败，请手工删除 %s: %v", created.ID, err)
		}
	})
	t.Logf("created id=%s status=%s layout=%s", created.ID, created.Status, created.LayoutJSON)

	if created.Status != entity.DashboardStatusDraft {
		t.Errorf("status 缺省应为 draft, 实际 %q", created.Status)
	}
	var canonical any
	if err := json.Unmarshal([]byte(created.LayoutJSON), &canonical); err != nil {
		t.Errorf("回读的 layout_json 不是合法 JSON: %v", err)
	}

	got, err := svc.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != created.Name || got.CreatedAt == "" || got.UpdatedAt == "" {
		t.Errorf("回读不一致: %+v", got)
	}

	refs, err := svc.CountChartReferences(context.Background(), chartID)
	if err != nil {
		t.Fatalf("CountChartReferences: %v", err)
	}
	found := false
	for _, r := range refs {
		if r.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("jsonpath 未命中刚写入的盘（refs=%+v）", refs)
	}
	if other, err := svc.CountChartReferences(context.Background(), chartID+1); err != nil {
		t.Fatalf("CountChartReferences(other): %v", err)
	} else if len(other) != 0 {
		t.Errorf("不存在的图表 id 不该有引用，实际 %+v", other)
	}

	updated, err := svc.Update(context.Background(), created.ID, entity.DashboardUpdateRequest{Name: ptr("zz-realpg-renamed")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "zz-realpg-renamed" {
		t.Errorf("name 未更新: %+v", updated)
	}
	// PG 的 jsonb 会规范化键序与空白，所以按语义（解析后逐字段）比对，
	// 不按字节比对。
	var doc struct {
		Widgets []struct {
			WidgetID string `json:"widgetId"`
			ChartID  int    `json:"chartId"`
		} `json:"widgets"`
	}
	if err := json.Unmarshal([]byte(updated.LayoutJSON), &doc); err != nil {
		t.Fatalf("回读 layout_json 解析失败: %v (%s)", err, updated.LayoutJSON)
	}
	if len(doc.Widgets) != 1 || doc.Widgets[0].ChartID != chartID || doc.Widgets[0].WidgetID != "w-real" {
		t.Errorf("layout_json 未保留（语义比对）: %s", updated.LayoutJSON)
	}
	// updated_at 必须由 UPDATE 真的前进（DB 里是微秒精度，别按秒比字符串）。
	var advanced bool
	if err := db.NewRaw("SELECT updated_at > created_at FROM bi_dashboard WHERE id = ?", created.ID).Scan(context.Background(), &advanced); err != nil {
		t.Fatalf("读 updated_at: %v", err)
	}
	if !advanced {
		t.Error("updated_at 未前进")
	}

	if err := svc.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(context.Background(), created.ID); err == nil {
		t.Error("软删后 Get 应返回 NotFound")
	} else if code := bizCode(t, err); code != 20300 {
		t.Errorf("软删后 Get 错误码 = %d, want 20300", code)
	}
	after, err := svc.CountChartReferences(context.Background(), chartID)
	if err != nil {
		t.Fatalf("CountChartReferences(after delete): %v", err)
	}
	if len(after) != 0 {
		t.Errorf("软删后不该再被引用计数命中: %+v", after)
	}
}
