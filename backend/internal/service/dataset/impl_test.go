package dataset

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"data-insights/internal/crypto"
	"data-insights/internal/datasource"
	"data-insights/internal/domain/entity"
	"data-insights/internal/model"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// stubConnection 实现 datasource.Connection，用于替换真实驱动。
type stubConnection struct {
	executeSQL    string
	executeErr    error
	resultColumns []string
	resultRows    []map[string]any

	getColumnsCalledWith string
	getColumnsResult     []datasource.ColumnInfo
	getColumnsErr        error
}

func (s *stubConnection) Close() error                   { return nil }
func (s *stubConnection) Ping(ctx context.Context) error { return nil }
func (s *stubConnection) GetTables(ctx context.Context) ([]datasource.TableInfo, error) {
	return nil, errors.New("not implemented")
}

func (s *stubConnection) GetColumns(ctx context.Context, tableName string) ([]datasource.ColumnInfo, error) {
	s.getColumnsCalledWith = tableName
	if s.getColumnsErr != nil {
		return nil, s.getColumnsErr
	}
	return s.getColumnsResult, nil
}

func (s *stubConnection) GetPrimaryKeys(ctx context.Context, tableName string) ([]string, error) {
	return nil, nil
}

func (s *stubConnection) Capabilities(ctx context.Context) (*datasource.DialectCapabilities, error) {
	return &datasource.DialectCapabilities{}, nil
}

func (s *stubConnection) Execute(ctx context.Context, query string, args ...any) (*datasource.QueryResult, error) {
	s.executeSQL = query
	if s.executeErr != nil {
		return nil, s.executeErr
	}
	return &datasource.QueryResult{Columns: s.resultColumns, Rows: s.resultRows}, nil
}

// TestToDatasetModelNormalizesEmptyJSONFields 先红：空字符串写入 JSONB 列
// 会让 PG 报 invalid input syntax for type json；toDatasetModel 必须把
// 空 JSON 字段归一为合法 JSON（与空集合序列化为 [] 的响应契约一致）。
func TestToDatasetModelNormalizesEmptyJSONFields(t *testing.T) {
	empty := ""
	m := toDatasetModel(&entity.Dataset{
		Name:             "fixture",
		DatasourceID:     1,
		QueryType:        "table",
		AccelerateConfig: &empty,
		RefreshStrategy:  &empty,
		PreviewData:      &empty,
	})
	if m.Tags != "[]" {
		t.Fatalf("empty tags must normalize to [], got %q", m.Tags)
	}
	if m.QualityRules != "[]" {
		t.Fatalf("empty quality_rules must normalize to [], got %q", m.QualityRules)
	}
	if m.Columns != "[]" {
		t.Fatalf("empty columns must normalize to [], got %q", m.Columns)
	}
	if m.ShardKeys != "[]" {
		t.Fatalf("empty shard_keys must normalize to [], got %q", m.ShardKeys)
	}
	// 空 JSON 指针字段置 NULL，让列默认值生效，而不是把 "" 写进 JSONB
	if m.AccelerateConfig.Valid || m.RefreshStrategy.Valid || m.PreviewData.Valid {
		t.Fatalf("empty json pointer fields must be stored as NULL, got %+v", m)
	}
}

// TestToDatasetModelMapsTimestamps 先红：PUT /api/datasets/:id 的 handler 以
// 取回行为合并基、把 existing 的 created_at/updated_at 原样带入 svc.Update
// （Batch 3 Task 1 防数据丢失），但 toDatasetModel 此前丢弃这两个字段，
// 整行更新会把时间戳写成 NULL。对齐 datasource 服务 toModel 的既有映射。
func TestToDatasetModelMapsTimestamps(t *testing.T) {
	created, err := time.Parse(time.RFC3339, "2024-01-02T03:04:05Z")
	if err != nil {
		t.Fatalf("fixture time: %v", err)
	}
	updated, err := time.Parse(time.RFC3339, "2024-06-07T08:09:10Z")
	if err != nil {
		t.Fatalf("fixture time: %v", err)
	}
	m := toDatasetModel(&entity.Dataset{
		Name:      "fixture",
		CreatedAt: "2024-01-02T03:04:05Z",
		UpdatedAt: "2024-06-07T08:09:10Z",
	})
	if !m.CreatedAt.Valid || !m.CreatedAt.Time.Equal(created) {
		t.Fatalf("created_at must round-trip through the converter, got %+v", m.CreatedAt)
	}
	if !m.UpdatedAt.Valid || !m.UpdatedAt.Time.Equal(updated) {
		t.Fatalf("updated_at must round-trip through the converter, got %+v", m.UpdatedAt)
	}
	// 空串保持 NULL：与 datasource toModel 一致，Create 路径不受影响
	// （Create 在转换后显式设置 CreatedAt）。
	m2 := toDatasetModel(&entity.Dataset{Name: "fixture"})
	if m2.CreatedAt.Valid || m2.UpdatedAt.Valid {
		t.Fatalf("empty timestamps must stay NULL, got %+v / %+v", m2.CreatedAt, m2.UpdatedAt)
	}
}

// TestGetColumnsSQLDatasetDerivesFromQuery 验证 SQL 型数据集在无已存列时，
// 通过 query 包单一通道执行包装后的查询并从结果列名推导列，
// 不再把 querySQL 拼成表名传给 GetColumns（该路径已被驱动校验拒绝）。
func TestGetColumnsSQLDatasetDerivesFromQuery(t *testing.T) {
	stub := &stubConnection{resultColumns: []string{"id", "name"}}
	s := &datasetService{
		getDatasetModelFn: func(ctx context.Context, id int) (*model.Dataset, error) {
			return &model.Dataset{
				ID:        1,
				QueryType: "sql",
				QuerySQL:  sql.NullString{String: "SELECT id, name FROM users", Valid: true},
				Columns:   "",
			}, nil
		},
		getDatasourceModelFn: func(ctx context.Context, id int) (*model.Datasource, error) {
			return &model.Datasource{ID: 2, Type: "mysql"}, nil
		},
		connectFn: func(ctx context.Context, ds *model.Datasource) (datasource.Connection, error) {
			return stub, nil
		},
	}

	cols, err := s.GetColumns(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 必须"执行查询"而不是调用 GetColumns
	if stub.getColumnsCalledWith != "" {
		t.Fatalf("GetColumns should not be called for sql datasets, got tableName %q", stub.getColumnsCalledWith)
	}
	wantSQL := "SELECT * FROM (SELECT id, name FROM users) AS _preview LIMIT 1"
	if stub.executeSQL != wantSQL {
		t.Fatalf("executed sql = %q, want %q", stub.executeSQL, wantSQL)
	}

	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d (%v)", len(cols), cols)
	}
	if cols[0].Name != "id" || cols[1].Name != "name" {
		t.Fatalf("unexpected column names: %v", cols)
	}
	for _, col := range cols {
		if col.Type != "string" {
			t.Errorf("column %q type = %q, want %q", col.Name, col.Type, "string")
		}
		if col.Role != "dimension" {
			t.Errorf("column %q role = %q, want dimension", col.Name, col.Role)
		}
	}
}

// TestGetColumnsTableDatasetUsesGetColumns 验证 table 型数据集路径保持现状：
// 校验后的 tableName 直连 GetColumns。
func TestGetColumnsTableDatasetUsesGetColumns(t *testing.T) {
	stub := &stubConnection{
		getColumnsResult: []datasource.ColumnInfo{{Name: "id", Type: "int"}},
	}
	s := &datasetService{
		getDatasetModelFn: func(ctx context.Context, id int) (*model.Dataset, error) {
			return &model.Dataset{
				ID:        1,
				QueryType: "table",
				TableName: sql.NullString{String: "users", Valid: true},
				Columns:   "",
			}, nil
		},
		getDatasourceModelFn: func(ctx context.Context, id int) (*model.Datasource, error) {
			return &model.Datasource{ID: 2, Type: "mysql"}, nil
		},
		connectFn: func(ctx context.Context, ds *model.Datasource) (datasource.Connection, error) {
			return stub, nil
		},
	}

	cols, err := s.GetColumns(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.getColumnsCalledWith != "users" {
		t.Fatalf("GetColumns called with %q, want %q", stub.getColumnsCalledWith, "users")
	}
	if stub.executeSQL != "" {
		t.Fatalf("Execute should not be called for table datasets, got %q", stub.executeSQL)
	}
	if len(cols) != 1 || cols[0].Name != "id" {
		t.Fatalf("unexpected columns: %v", cols)
	}
	if !strings.Contains(cols[0].Expr, "id") {
		t.Fatalf("unexpected expr: %q", cols[0].Expr)
	}
}

// TestMapDatasetColumnsBareIdentifier 先红：列表达式不能带 MySQL 反引号——
// 反引号会被查询层原样渲染，PostgreSQL 报 syntax error at or near "`"。
// 方言相关的引号是查询层（safeIdentifier/方言 builder）的职责。
func TestMapDatasetColumnsBareIdentifier(t *testing.T) {
	cols := mapDatasetColumns([]datasource.ColumnInfo{
		{Name: "region", Type: "varchar"},
		{Name: "amount", Type: "numeric"},
	}, "postgresql")
	for _, col := range cols {
		if strings.ContainsAny(col.Expr, "`\"") {
			t.Fatalf("column expr must be a bare identifier, got %q", col.Expr)
		}
		if col.Expr != col.Name {
			t.Fatalf("expr should equal column name, got expr %q for %q", col.Expr, col.Name)
		}
	}
}

// TestMapDatasetColumnsUsesDatasourceMapper 先红：列类型探测按数据源驱动选择
// 映射器，而不是硬编码 starrocks。PG 的 information_schema 类型名
// (integer/numeric/text/date) 此前被 starrocks 映射表拒之门外，全部折叠成
// "unknown"，导致指标字段无法识别。
func TestMapDatasetColumnsUsesDatasourceMapper(t *testing.T) {
	cols := mapDatasetColumns([]datasource.ColumnInfo{
		{Name: "year", Type: "integer"},
		{Name: "gdp_total", Type: "numeric"},
		{Name: "region", Type: "text"},
		{Name: "created_at", Type: "date"},
	}, "postgresql")

	wantTypes := map[string]string{
		"year":       "integer",
		"gdp_total":  "float",
		"region":     "string",
		"created_at": "date",
	}
	for _, col := range cols {
		if wantTypes[col.Name] != col.Type {
			t.Fatalf("column %s: type %q, want %q", col.Name, col.Type, wantTypes[col.Name])
		}
	}
}

// --- password resolution tests (C1 fix) ---

func testAESKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

// TestConnectResolvesPassword 验证 dataset 服务启用 key 后 connect 拿到的是
// 解密后的密码（dialFn 收到的值即写入 ConnectionConfig.Password 的值），
// 且存量明文密码被回写为 v1: 密文。
func TestConnectResolvesPassword(t *testing.T) {
	key := testAESKey()

	t.Run("encrypted password decrypted for dial", func(t *testing.T) {
		ct, err := crypto.Encrypt(key, "fixture-credential-value")
		if err != nil {
			t.Fatal(err)
		}
		var got string
		s := &datasetService{}
		s.SetSecurityKey(key)
		s.dialFn = func(ctx context.Context, ds *model.Datasource, password string) (datasource.Connection, error) {
			got = password
			return &stubConnection{}, nil
		}
		if _, err := s.connect(context.Background(), &model.Datasource{ID: 2, Type: "postgresql", Password: ct}); err != nil {
			t.Fatal(err)
		}
		if got != "fixture-credential-value" {
			t.Fatalf("connect should dial with decrypted password, got %q", got)
		}
	})

	t.Run("legacy plaintext upgraded and used", func(t *testing.T) {
		sqlDB, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer sqlDB.Close()

		var got string
		s := &datasetService{db: bun.NewDB(sqlDB, pgdialect.New())}
		s.SetSecurityKey(key)
		s.dialFn = func(ctx context.Context, ds *model.Datasource, password string) (datasource.Connection, error) {
			got = password
			return &stubConnection{}, nil
		}
		mock.ExpectExec(`UPDATE "bi_datasource"`).WillReturnResult(sqlmock.NewResult(0, 1))

		if _, err := s.connect(context.Background(), &model.Datasource{ID: 2, Type: "postgresql", Password: "fixture-plaintext-credential"}); err != nil {
			t.Fatal(err)
		}
		if got != "fixture-plaintext-credential" {
			t.Fatalf("connection should use original plaintext, got %q", got)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("legacy password was not upgraded: %v", err)
		}
	})
}

// --- updated_at stamping tests (Batch 4 B4-B) ---

// datasetCaptureMatcher 记录实际执行的 SQL，同时保留默认 regexp 匹配语义，
// 便于在 UPDATE/INSERT 语句中断言 updated_at 的写入形态（对齐 datasource 服务
// impl_test.go 的 captureMatcherFunc）。
func datasetCaptureMatcher(record *[]string) sqlmock.QueryMatcherFunc {
	return sqlmock.QueryMatcherFunc(func(expectedSQL, actualSQL string) error {
		*record = append(*record, actualSQL)
		return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
	})
}

// findStmt 返回记录到的第一条以 prefix 开头的语句。
func findStmt(executed []string, prefix string) string {
	for _, q := range executed {
		if strings.HasPrefix(q, prefix) {
			return q
		}
	}
	return ""
}

// timestampLitRE 匹配 bun 内联渲染的时间戳字面量主体（不受本地时区偏移影响）。
var timestampLitRE = regexp.MustCompile(`\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}`)

// TestDatasetUpdateStampsUpdatedAt 先红：PUT /api/datasets/:id 的 handler 以取回
// 行为合并基，把 existing.updated_at 原样带入 svc.Update；service.Update 走整行
// WherePK 更新，此前不刷新 updated_at，于是把旧的 updated_at 写回（更新后时间戳
// 不前进）。修复：Update 显式把 model 的 UpdatedAt 打成当前时间。
//
// 断言策略（无需解析墙钟）：喂入一个"陈旧"的 updated_at 与一个可区分的
// created_at，执行后断言 UPDATE 语句里 (1) 不再出现陈旧值、(2) 不是 NULL、
// (3) 是带引号的时间戳字面量、(4) created_at 透传保持不变。
func TestDatasetUpdateStampsUpdatedAt(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(datasetCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &datasetService{db: db}

	ds := &entity.Dataset{
		ID:           1,
		Name:         "n",
		DatasourceID: 1,
		QueryType:    "table",
		CreatedAt:    "2024-01-02T03:04:05Z",
		UpdatedAt:    "2020-06-07T08:09:10Z", // 合并基带入的陈旧值，必须被覆盖
	}

	reselect := sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "created_at", "updated_at"}).
		AddRow(1, "n", 1, "table", nil, nil)
	mock.ExpectExec(`UPDATE "bi_dataset"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "bi_dataset"`).WillReturnRows(reselect)

	if _, err := s.Update(context.Background(), ds); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}

	upd := findStmt(executed, `UPDATE "bi_dataset"`)
	if upd == "" {
		t.Fatalf("no UPDATE statement captured, got: %q", executed)
	}
	if strings.Contains(upd, "2020-06-07 08:09:10") {
		t.Fatalf("updated_at must not carry the stale merge value, got: %s", upd)
	}
	if strings.Contains(upd, `"updated_at" = NULL`) {
		t.Fatalf("updated_at must not be written as NULL, got: %s", upd)
	}
	if !strings.Contains(upd, `"updated_at" = '`) {
		t.Fatalf("updated_at must be stamped with a timestamp literal, got: %s", upd)
	}
	// created_at 透传语义必须保持不变（设计点：merge 负责 created_at）。
	if !strings.Contains(upd, "2024-01-02 03:04:05") {
		t.Fatalf("created_at passthrough must be preserved, got: %s", upd)
	}
}

// TestDatasetCreateStampsUpdatedAt 记录 create 路径的时间戳对齐决策：
// 修复前 dataset.Create 只打 created_at，updated_at 被 bun 写成显式 NULL
// （bun 对零值 sql.NullTime 发 NULL，绕过列 DEFAULT CURRENT_TIMESTAMP），
// 导致新建行 updated_at 为空。对齐后两者都在插入时打戳。
func TestDatasetCreateStampsUpdatedAt(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(datasetCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &datasetService{db: db}

	insertRows := sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "created_at", "updated_at"}).
		AddRow(1, "n", 1, "table", nil, nil)
	mock.ExpectQuery(`INSERT INTO "bi_dataset"`).WillReturnRows(insertRows)

	if _, err := s.Create(context.Background(), &entity.Dataset{Name: "n", DatasourceID: 1, QueryType: "table"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}

	ins := findStmt(executed, `INSERT INTO "bi_dataset"`)
	if ins == "" {
		t.Fatalf("no INSERT statement captured, got: %q", executed)
	}
	// INSERT 的值是位置化写入 VALUES 子句（不同于 UPDATE 的 "col" = value 相邻形式），
	// 所以按"内联时间戳字面量"计数：仅 created_at 打戳 → 1 处；created_at+updated_at
	// 都打戳 → 2 处。bi_dataset 插入只有这两个时间列，其余列不会误配该正则。
	if got := timestampLitRE.FindAllString(ins, -1); len(got) < 2 {
		t.Fatalf("created row must stamp BOTH created_at and updated_at (expected >=2 timestamp literals, got %d) in: %s", len(got), ins)
	}
}
