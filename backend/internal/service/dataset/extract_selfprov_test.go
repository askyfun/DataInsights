package dataset

import (
	"context"
	"errors"
	"strings"
	"testing"

	"data-insights/internal/datasource"
	"data-insights/internal/domain/entity"
	"data-insights/internal/extract"
	"data-insights/internal/model"
	"data-insights/internal/response"
	"data-insights/internal/router"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// errNoSuchDatasource 是打桩里「查不到该数据源」的哨兵：让用例在数据源 id
// 不等于期望值时立刻失败，而不是悄悄走另一条路径。
var errNoSuchDatasource = errors.New("no such datasource")

// errNoSuchDataset 是数据集读取的兜底哨兵（见 withWorkingDatasource 的说明）。
var errNoSuchDataset = errors.New("no such dataset")

// R-85（issue #201）复现测试：抽取存储零配置。
//
// 缺陷背景：EXTRACT_DATASOURCE_ID 未设置时，ImportFile/ReplaceFile 首行即
// `if !s.extract.Enabled()` 返回错误，用户必须先建数据源 → 查出它的数字 id →
// 写 env → 重启进程才能上传。V0.3.0 的旗舰功能带着这个门槛等于不可用。
//
// 修法：启动期自举——未显式指定时，抽取存储复用应用自身的 PostgreSQL
// （DATABASE_URL），落一个隐藏数据源行。服务层因此不再是「配置了才干」。

// recordingConn 记录每一条 Execute 的 SQL。
//
// 不能直接复用 impl_test.go 的 stubConnection：它只保留最后一条（executeSQL
// 被覆盖），而摄入路径依次发 DROP/CREATE/INSERT。拿「最后一条是 INSERT」去论证
// 「表建对了名字」是循环论证——INSERT 发得出去本身就说明 CREATE 过了，但那证明不了
// CREATE 的表名正确。留下全部 SQL，断言才成立。
type recordingConn struct {
	stubConnection
	sqls []string
}

func (c *recordingConn) Execute(ctx context.Context, query string, args ...any) (*datasource.QueryResult, error) {
	c.sqls = append(c.sqls, query)
	return c.stubConnection.Execute(ctx, query, args...)
}

// withWorkingDatasource 把数据源读取与连接换成可用打桩，
// 返回记录全部 SQL 的连接桩。
func withWorkingDatasource(s *datasetService, id int) *recordingConn {
	conn := &recordingConn{}
	s.getDatasourceModelFn = func(_ context.Context, got int) (*model.Datasource, error) {
		if got != id {
			return nil, errNoSuchDatasource
		}
		return &model.Datasource{
			ID: got, Name: "data-insights internal storage", Type: "postgresql",
			Host: "127.0.0.1", Port: 5432, DatabaseName: "insights", Username: "insights",
		}, nil
	}
	s.connectFn = func(context.Context, *model.Datasource) (datasource.Connection, error) {
		return conn, nil
	}
	// 兜住手工构造的 service：NewService 装的默认值在这里不存在，
	// 缺了会变成 nil 调用 panic，而不是一条可读的失败。
	if s.getDatasetModelFn == nil {
		s.getDatasetModelFn = func(_ context.Context, datasetID int) (*model.Dataset, error) {
			return nil, errNoSuchDataset
		}
	}
	return conn
}

// sawSQLPrefix 报告记录到的 SQL 里是否有一条以 prefix 开头。
func (c *recordingConn) sawSQLPrefix(prefix string) bool {
	for _, q := range c.sqls {
		if strings.HasPrefix(q, prefix) {
			return true
		}
	}
	return false
}

// TestImportFileWorksWithoutConfiguredExtractDatasource 是本 issue 的核心复现用例：
// env 未配 EXTRACT_DATASOURCE_ID、但启动期自举已把真实数据源 id 注入 Guard 时，
// 上传必须成功，且真的建出抽取表。
//
// 修复前：Guard 为 0 → Enabled() 为 false → 首行直接返回
// 「extract storage is not configured」，本用例失败。
func TestImportFileWorksWithoutConfiguredExtractDatasource(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()

	s := &datasetService{
		db:      bun.NewDB(sqlDB, pgdialect.New()),
		extract: extract.Guard{DatasourceID: 42, SelfProvisioned: true},
	}
	conn := withWorkingDatasource(s, 42)

	csv := []byte("city,amount\nBeijing,100\nShanghai,200\n")
	mock.ExpectQuery(`INSERT INTO "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id"}).AddRow(7, "sales", 42))
	mock.ExpectExec(`UPDATE "bi_dataset"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "mode", "columns"}).
			AddRow(7, "sales", 42, "table", "extract",
				`[{"id":"c1","name":"city","expr":"city","type":"string","role":"dimension"},`+
					`{"id":"c2","name":"amount","expr":"amount","type":"integer","role":"measure"}]`))

	ds, err := s.ImportFile(context.Background(), ImportRequest{Filename: "sales.csv", Data: csv})
	if err != nil {
		t.Fatalf("upload must work without EXTRACT_DATASOURCE_ID (self-provisioned storage): %v", err)
	}
	if ds == nil || ds.ID != 7 {
		t.Fatalf("expected dataset 7, got %#v", ds)
	}
	// 抽取表必须真的建出来，且落在 di_extract_ 命名空间里。
	if !conn.sawSQLPrefix(`CREATE TABLE "di_extract_7"`) {
		t.Fatalf("expected CREATE TABLE for di_extract_7, got %v", conn.sqls)
	}
	if !conn.sawSQLPrefix(`INSERT INTO "di_extract_7"`) {
		t.Fatalf("expected rows inserted into di_extract_7, got %v", conn.sqls)
	}
}

// TestReplaceFileWorksWithoutConfiguredExtractDatasource 覆盖替换路径：
// 零配置不能只修首次上传，重新上传也必须通。
func TestReplaceFileWorksWithoutConfiguredExtractDatasource(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()

	s := &datasetService{
		db:      bun.NewDB(sqlDB, pgdialect.New()),
		extract: extract.Guard{DatasourceID: 42, SelfProvisioned: true},
	}
	conn := withWorkingDatasource(s, 42)

	// getDatasetModelFn 显式打桩：替换路径先读现有数据集，拿到它的列以复用列 id。
	s.getDatasetModelFn = func(_ context.Context, datasetID int) (*model.Dataset, error) {
		if datasetID != 7 {
			return nil, errNoSuchDataset
		}
		return &model.Dataset{
			ID: 7, DatasourceID: 42, Mode: "extract", QueryType: "table",
			Columns: `[{"id":"c1","name":"city","expr":"city","type":"string","role":"dimension"}]`,
		}, nil
	}

	mock.ExpectExec(`UPDATE "bi_dataset"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "mode", "columns"}).
			AddRow(7, "sales", 42, "table", "extract",
				`[{"id":"c1","name":"city","expr":"city","type":"string","role":"dimension"},`+
					`{"id":"c9","name":"amount","expr":"amount","type":"integer","role":"measure"}]`))

	csv := []byte("city,amount\nBeijing,100\n")
	if _, err := s.ReplaceFile(context.Background(), 7, ImportRequest{Filename: "sales.csv", Data: csv}); err != nil {
		t.Fatalf("replace must work on self-provisioned storage: %v", err)
	}

	// 替换是整表重来：先 DROP 再 CREATE，两条都必须落在同一张抽取表上。
	if !conn.sawSQLPrefix(`DROP TABLE IF EXISTS "di_extract_7"`) {
		t.Fatalf("replace must drop the old extract table, got %v", conn.sqls)
	}
	if !conn.sawSQLPrefix(`CREATE TABLE "di_extract_7"`) {
		t.Fatalf("replace must recreate the extract table, got %v", conn.sqls)
	}
}

// TestSelfProvisionedGuardStillBlocksRegularDatasets 钉住「自举不得放宽既有守卫」：
// 自举出来的数据源与用户自己指定的一样，不能被普通数据集引用。
func TestSelfProvisionedGuardStillBlocksRegularDatasets(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	s := &datasetService{
		db:      bun.NewDB(sqlDB, pgdialect.New()),
		extract: extract.Guard{DatasourceID: 42, SelfProvisioned: true},
	}

	_, err = s.Create(context.Background(), &entity.Dataset{Name: "n", DatasourceID: 42, QueryType: "table"})
	if err == nil {
		t.Fatal("self-provisioned extract storage must still reject regular datasets")
	}
	bizErr, ok := err.(router.BusinessError)
	if !ok || bizErr.Code != response.CodeBadRequest {
		t.Fatalf("must surface as %d BusinessError, got %#v", response.CodeBadRequest, err)
	}
	if !strings.Contains(bizErr.Message, "extract storage") {
		t.Fatalf("message must name the extract storage violation, got %q", bizErr.Message)
	}
}

// TestExplicitlyConfiguredDatasourceIsNotSelfProvisioned 钉住优先级：
// 用户显式配了 EXTRACT_DATASOURCE_ID 时，绝不能被自举覆盖或标记为自举。
func TestExplicitlyConfiguredDatasourceIsNotSelfProvisioned(t *testing.T) {
	g := extract.Guard{DatasourceID: 9}
	if g.SelfProvisioned {
		t.Fatal("an explicitly configured datasource must not be marked self-provisioned")
	}
	if !g.Enabled() {
		t.Fatal("an explicitly configured datasource must be enabled")
	}
}

// TestUploadRejectsTooManyRows 钉住行数上限（issue #201 验收 4）：
// 超过 MaxUploadRows 的文件必须给出可区分的错误文案，而不是灌进数据库。
func TestUploadRejectsTooManyRows(t *testing.T) {
	var b strings.Builder
	b.WriteString("a\n")
	for i := 0; i <= extract.MaxUploadRows; i++ {
		b.WriteString("1\n")
	}
	_, _, err := parseUpload(ImportRequest{Filename: "big.csv", Data: []byte(b.String())})
	if err == nil {
		t.Fatal("upload beyond MaxUploadRows must be rejected")
	}
	bizErr, ok := err.(router.BusinessError)
	if !ok || bizErr.Code != response.CodeBadRequest {
		t.Fatalf("must be a %d BusinessError, got %#v", response.CodeBadRequest, err)
	}
	// 文案必须能被前端区分（不是通用失败提示），且带上真实上限数字。
	msg := strings.ToLower(bizErr.Message)
	if !strings.Contains(msg, "row") || !strings.Contains(msg, "1000000") {
		t.Fatalf("row-limit message must mention rows and the limit, got %q", bizErr.Message)
	}
}

// TestUploadRejectsTooManyColumns 钉住宽表护栏：列数必须有上限，
// 否则 50MB 的宽 CSV 会把 INSERT 的占位符数量推到驱动上限之外。
func TestUploadRejectsTooManyColumns(t *testing.T) {
	header := make([]string, extract.MaxUploadColumns+1)
	row := make([]string, len(header))
	for i := range header {
		header[i] = "c"
		row[i] = "1"
	}
	csv := strings.Join(header, ",") + "\n" + strings.Join(row, ",") + "\n"
	_, _, err := parseUpload(ImportRequest{Filename: "wide.csv", Data: []byte(csv)})
	if err == nil {
		t.Fatal("upload beyond MaxUploadColumns must be rejected")
	}
	bizErr, ok := err.(router.BusinessError)
	if !ok || bizErr.Code != response.CodeBadRequest {
		t.Fatalf("must be a %d BusinessError, got %#v", response.CodeBadRequest, err)
	}
}

// TestUploadRejectsTooManyBytes 钉住体积上限的既有行为不被改动带偏。
func TestUploadRejectsTooManyBytes(t *testing.T) {
	big := make([]byte, extract.MaxUploadBytes+1)
	_, _, err := parseUpload(ImportRequest{Filename: "big.csv", Data: big})
	if err == nil {
		t.Fatal("upload beyond MaxUploadBytes must be rejected")
	}
	bizErr, ok := err.(router.BusinessError)
	if !ok || bizErr.Code != response.CodeBadRequest {
		t.Fatalf("must be a %d BusinessError, got %#v", response.CodeBadRequest, err)
	}
	if !strings.Contains(bizErr.Message, "50 MB") {
		t.Fatalf("byte-limit message must state the MB limit, got %q", bizErr.Message)
	}
}

// TestEmptyUploadStillRejected 钉住既有行为不被零配置改动带偏。
func TestEmptyUploadStillRejected(t *testing.T) {
	_, _, err := parseUpload(ImportRequest{Filename: "empty.csv", Data: nil})
	if err == nil {
		t.Fatal("empty upload must be rejected")
	}
	bizErr, ok := err.(router.BusinessError)
	if !ok || bizErr.Code != response.CodeBadRequest {
		t.Fatalf("must be a %d BusinessError, got %#v", response.CodeBadRequest, err)
	}
}