package queryrecord

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"data-insights/internal/domain/entity"
	"data-insights/internal/idcodec"
	"data-insights/internal/response"
	"data-insights/internal/router"
)

const (
	// insertedID 是固定的 query_id；短码由 Python 独立算出（见 idcodec 测试向量）。
	insertedID      = "0198f2c3-4d5e-7a6b-8c9d-0e1f2a3b4c5d"
	insertedShortID = "CSatF9qXyRoQXFtAA3iZS"
)

// sqlCapture 记录真实下发给驱动的 SQL。
//
// sqlmock 的宽松正则匹配只保证"跑到了这条语句"，不保证语句内部写对；
// 去重 SQL 里那几处 COALESCE / RETURNING 恰恰是最容易被改错的部分
// （改错的表现是：已发出的链接有效期被静默刷新，或结果元信息被覆盖成 NULL），
// 所以这里把真实 SQL 抓下来逐条断言。
type sqlCapture struct{ stmts []string }

type captureMatcher struct {
	inner   sqlmock.QueryMatcher
	capture *sqlCapture
}

func (m *captureMatcher) Match(expectedSQL, actualSQL string) error {
	m.capture.stmts = append(m.capture.stmts, actualSQL)
	return m.inner.Match(expectedSQL, actualSQL)
}

// newTestService 组装一个跑在 sqlmock 上的服务，并把时间与 id 源固定下来，
// 让「expires_at = created_at + 90 天」「短码能解回写库的 uuid」这类断言可写。
func newTestService(t *testing.T) (*service, sqlmock.Sqlmock, *sqlCapture) {
	t.Helper()
	capture := &sqlCapture{}
	sqldb, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(&captureMatcher{
		inner:   sqlmock.QueryMatcherRegexp,
		capture: capture,
	}))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	svc := &service{
		db:    bun.NewDB(sqldb, pgdialect.New()),
		now:   func() time.Time { return time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC) },
		newID: func() (uuid.UUID, error) { return uuid.MustParse(insertedID), nil },
	}
	return svc, mock, capture
}

func (c *sqlCapture) find(t *testing.T, substr string) string {
	t.Helper()
	for _, stmt := range c.stmts {
		if strings.Contains(stmt, substr) {
			return stmt
		}
	}
	t.Fatalf("未捕获到包含 %q 的语句，已捕获: %v", substr, c.stmts)
	return ""
}

func specBody(t *testing.T, document string) json.RawMessage {
	t.Helper()
	if !json.Valid([]byte(document)) {
		t.Fatalf("测试用 document 不是合法 JSON: %s", document)
	}
	return json.RawMessage(`{"v":1,"document":` + document + `}`)
}

func bizCode(t *testing.T, err error) int {
	t.Helper()
	var bizErr router.BusinessError
	if !errors.As(err, &bizErr) {
		t.Fatalf("期望业务错误，实际为 %v", err)
	}
	return bizErr.Code
}

func recordColumns() []string {
	return []string{
		"query_id", "spec_json", "spec_hash", "dataset_id", "chart_id", "source_type",
		"row_count", "duration_ms", "ip", "created_at", "last_accessed_at", "hit_count",
		"expires_at", "owner_id", "tenant_id",
	}
}

// TestCanonicalize_SameConfigDifferentKeyOrder 去重的前提：键序不同的同一份配置
// 必须落到同一个 hash，否则「每次查询自动落库」会退化成「每次生成一条新记录」。
func TestCanonicalize_SameConfigDifferentKeyOrder(t *testing.T) {
	a := specBody(t, `{"chartType":"bar","title":"x"}`)
	b := json.RawMessage(`{"document":{"title":"x","chartType":"bar"},"v":1}`)

	canonicalA, hashA, err := canonicalize(a, 7)
	if err != nil {
		t.Fatalf("canonicalize(a): %v", err)
	}
	canonicalB, hashB, err := canonicalize(b, 7)
	if err != nil {
		t.Fatalf("canonicalize(b): %v", err)
	}
	if hashA != hashB {
		t.Fatalf("键序不同的同一配置 hash 不一致: %s vs %s", hashA, hashB)
	}
	if string(canonicalA) != string(canonicalB) {
		t.Fatalf("规范化结果不一致: %s vs %s", canonicalA, canonicalB)
	}
}

// TestCanonicalize_DatasetIsPartOfHash 同一份图表文档挂在不同数据集上是两次不同的
// 查询，必须得出不同 hash，否则会把它们错误合并成同一条记录。
func TestCanonicalize_DatasetIsPartOfHash(t *testing.T) {
	spec := specBody(t, `{"chartType":"bar"}`)
	_, hashA, err := canonicalize(spec, 1)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	_, hashB, err := canonicalize(spec, 2)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if hashA == hashB {
		t.Fatal("不同 dataset_id 得到同一 hash，去重会跨数据集误合并")
	}
}

// TestCanonicalize_HashIsSHA256OfCanonicalSpec 钉死 hash 口径：
// sha256("<dataset_id>\n" + 规范化 spec) 的十六进制，长度必须是 64（列宽 VARCHAR(64)）。
func TestCanonicalize_HashIsSHA256OfCanonicalSpec(t *testing.T) {
	spec := specBody(t, `{"chartType":"bar","title":"t"}`)
	canonical, hash, err := canonicalize(spec, 42)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}

	digest := sha256.New()
	digest.Write([]byte("42\n" + string(canonical)))
	want := hex.EncodeToString(digest.Sum(nil))
	if hash != want {
		t.Fatalf("hash = %s, want %s", hash, want)
	}
	if len(hash) != 64 {
		t.Fatalf("hash 长度 = %d, want 64（spec_hash 列宽）", len(hash))
	}
}

func TestCanonicalize_Rejects(t *testing.T) {
	oversized := specBody(t, `{"padding":"`+strings.Repeat("x", MaxSpecBytes)+`"}`)
	cases := []struct {
		name string
		spec json.RawMessage
	}{
		{"空 spec", nil},
		{"空白 spec", json.RawMessage("   ")},
		{"非法 JSON", json.RawMessage(`{"v":1,`)},
		{"超大 spec", oversized},
		{"版本缺失", json.RawMessage(`{"document":{}}`)},
		{"版本不支持", json.RawMessage(`{"v":2,"document":{}}`)},
		{"document 缺失", json.RawMessage(`{"v":1}`)},
		{"document 非对象", json.RawMessage(`{"v":1,"document":[]}`)},
		{"document 空对象字符串", json.RawMessage(`{"v":1,"document":""}`)},
	}
	for _, c := range cases {
		_, _, err := canonicalize(c.spec, 1)
		if err == nil {
			t.Errorf("%s: 期望报错，实际通过", c.name)
			continue
		}
		if code := bizCode(t, err); code != response.CodeBadRequest {
			t.Errorf("%s: 错误码 = %d, want %d", c.name, code, response.CodeBadRequest)
		}
	}
}

// TestSave_Rejects 覆盖 Save 的入参校验：这些分支必须在触库之前失败。
func TestSave_Rejects(t *testing.T) {
	cases := []struct {
		name string
		in   entity.QueryRecordSaveRequest
	}{
		{"dataset_id 缺失", entity.QueryRecordSaveRequest{Spec: specBody(t, `{"a":1}`)}},
		{"spec 非法", entity.QueryRecordSaveRequest{DatasetID: 1, Spec: json.RawMessage("{")}},
		{"source_type 过长", entity.QueryRecordSaveRequest{
			DatasetID:  1,
			Spec:       specBody(t, `{"a":1}`),
			SourceType: strings.Repeat("s", maxSourceTypeLen+1),
		}},
	}
	svc, mock, _ := newTestService(t)
	for _, c := range cases {
		_, err := svc.Save(context.Background(), c.in, "127.0.0.1")
		if err == nil {
			t.Errorf("%s: 期望报错，实际通过", c.name)
			continue
		}
		if code := bizCode(t, err); code != response.CodeBadRequest {
			t.Errorf("%s: 错误码 = %d, want %d", c.name, code, response.CodeBadRequest)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("校验失败路径不应访问数据库: %v", err)
	}
}

// TestSave_UpsertOnSpecHash 钉住去重 SQL 的形状：唯一键冲突走 DO UPDATE，
// 且只更新结果元信息——spec / created_at / expires_at 一旦被覆盖，
// 已经发出去的分享链接就会静默变内容或改变有效期。
func TestSave_UpsertOnSpecHash(t *testing.T) {
	svc, mock, capture := newTestService(t)

	rows := sqlmock.NewRows([]string{"query_id", "created_at", "expires_at"}).AddRow(
		insertedID,
		time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 18, 12, 0, 0, 0, time.UTC),
	)
	mock.ExpectQuery(`INSERT INTO `).WillReturnRows(rows)

	rowCount, durationMs := 120, 340
	saved, err := svc.Save(context.Background(), entity.QueryRecordSaveRequest{
		DatasetID:  3,
		ChartID:    14,
		Spec:       specBody(t, `{"chartType":"bar","title":"t"}`),
		RowCount:   &rowCount,
		DurationMs: &durationMs,
	}, "10.0.0.9")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("期望未满足: %v", err)
	}

	if saved.QueryID != insertedShortID {
		t.Fatalf("query_id = %q, want %q", saved.QueryID, insertedShortID)
	}
	if back, err := idcodec.Decode(saved.QueryID); err != nil || back.String() != insertedID {
		t.Fatalf("短码解不回写库的 uuid: %v / %s", err, back)
	}
	if len(saved.QueryID) != idcodec.EncodedLen {
		t.Fatalf("地址栏短码长度 = %d, want %d", len(saved.QueryID), idcodec.EncodedLen)
	}
	if saved.CreatedAt != "2026-09-19T12:00:00Z" {
		t.Fatalf("created_at = %q", saved.CreatedAt)
	}
	if saved.ExpiresAt == nil || *saved.ExpiresAt != "2026-12-18T12:00:00Z" {
		t.Fatalf("expires_at = %v, want 2026-12-18T12:00:00Z（created_at + 90 天）", saved.ExpiresAt)
	}

	stmt := capture.find(t, "ON CONFLICT")
	for _, want := range []string{
		"ON CONFLICT (spec_hash) DO UPDATE",
		"row_count = EXCLUDED.row_count",
		"duration_ms = EXCLUDED.duration_ms",
		"RETURNING query_id, created_at, expires_at",
	} {
		if !strings.Contains(stmt, want) {
			t.Errorf("upsert SQL 缺少 %q：\n%s", want, stmt)
		}
	}
	// 反向断言：DO UPDATE 里绝不允许出现这几个列，否则会改写已分享链接的语义
	for _, forbidden := range []string{
		"spec_json =",
		"created_at =",
		"expires_at =",
		"hit_count =",
	} {
		if strings.Contains(stmt, forbidden) {
			t.Errorf("upsert SQL 不应更新 %q：\n%s", forbidden, stmt)
		}
	}
	// 反向断言（回归）：bun 会把表别名成 "query_record"，因此 SET 子句里
	// 按物理表名引用旧值会被 PG 判 42P01 invalid reference to FROM-clause entry。
	// 一旦有人把「未上报则保留」写回 COALESCE(..., bi_query.col)，这里立刻红。
	if strings.Contains(stmt, "bi_query.") {
		t.Errorf("upsert SQL 不得按物理表名引用目标表（bun 已加别名，PG 会报 42P01）：\n%s", stmt)
	}
	// 反向断言（回归）：SET 子句里绝不能出现 query_id。`query_id = EXCLUDED.query_id`
	// 像自赋值，实际会把存量行主键改成本次新生成的 UUID，已发出的短码当场失效。
	setClause := setClauseOf(stmt)
	if strings.Contains(setClause, "query_id") {
		t.Errorf("SET 子句不得包含 query_id（会改写主键、作废已发出的短码）：\n%s", setClause)
	}
}

// setClauseOf 截出 `DO UPDATE SET ...` 与 `RETURNING` 之间的部分。
// 不做边界截断会把 RETURNING 的列也算进来，断言就永远红。
func setClauseOf(stmt string) string {
	start := strings.Index(stmt, "DO UPDATE SET ")
	if start < 0 {
		return ""
	}
	rest := stmt[start+len("DO UPDATE SET "):]
	if end := strings.Index(rest, " RETURNING "); end >= 0 {
		return rest[:end]
	}
	return rest
}

// TestSave_UnreportedMetricsKeepExistingValues 未上报的指标不得进 SET 子句：
// 去重命中时把 row_count / duration_ms 写成 NULL 会让已有链接的元信息凭空消失。
func TestSave_UnreportedMetricsKeepExistingValues(t *testing.T) {
	svc, mock, capture := newTestService(t)
	mock.ExpectQuery(`INSERT INTO `).
		WillReturnRows(sqlmock.NewRows([]string{"query_id", "created_at", "expires_at"}).
			AddRow(insertedID, time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC), nil))

	// 两个指标都不上报：只发 spec
	if _, err := svc.Save(context.Background(), entity.QueryRecordSaveRequest{
		DatasetID: 3,
		Spec:      specBody(t, `{"chartType":"bar"}`),
	}, ""); err != nil {
		t.Fatalf("Save: %v", err)
	}

	stmt := capture.find(t, "ON CONFLICT")
	for _, forbidden := range []string{"row_count =", "duration_ms =", "query_id ="} {
		if strings.Contains(stmt, forbidden) {
			t.Errorf("未上报 / 不可改的列不应出现在 SET 子句（%q）：\n%s", forbidden, stmt)
		}
	}
	// SET 子句必须非空，否则 ON CONFLICT DO UPDATE 是语法错误；且兜底只能用
	// 冲突判定列自身的空赋值。
	if !strings.Contains(stmt, "SET spec_hash = EXCLUDED.spec_hash") {
		t.Errorf("两个指标都缺省时必须保留一条可证明为空的兜底赋值（spec_hash）：\n%s", stmt)
	}
}

// TestSave_ExpiresAtSemantics expires_at 恒为 created_at + TTLDays，不随读取刷新。
func TestSave_ExpiresAtSemantics(t *testing.T) {
	svc, mock, _ := newTestService(t)
	mock.ExpectQuery(`INSERT INTO `).
		WillReturnRows(sqlmock.NewRows([]string{"query_id", "created_at", "expires_at"}).
			AddRow(insertedID, time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC), nil))

	saved, err := svc.Save(context.Background(), entity.QueryRecordSaveRequest{
		DatasetID: 1,
		Spec:      specBody(t, `{"a":1}`),
	}, "")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	// RETURNING 回 NULL 时对外必须是 null（= 永久），而不是空字符串
	if saved.ExpiresAt != nil {
		t.Fatalf("expires_at 为 NULL 时应回 nil，实际 %v", *saved.ExpiresAt)
	}
}

func TestNormalizeSourceType(t *testing.T) {
	if got, err := normalizeSourceType("  "); err != nil || got != SourceTypeBuild {
		t.Fatalf("空值应默认 %q，实际 %q / %v", SourceTypeBuild, got, err)
	}
	if got, err := normalizeSourceType(SourceTypeShareURL); err != nil || got != SourceTypeShareURL {
		t.Fatalf("显式值应透传，实际 %q / %v", got, err)
	}
	if _, err := normalizeSourceType(strings.Repeat("x", maxSourceTypeLen+1)); err == nil {
		t.Fatal("超长 source_type 应报错")
	}
}

// TestGetByShortID_Hit 覆盖命中路径：spec 原样还原、过期判定、访问留痕自增，
// 且 expires_at 不被刷新（U-10 的核心语义）。
func TestGetByShortID_Hit(t *testing.T) {
	svc, mock, capture := newTestService(t)

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	expiresAt := time.Date(2026, 4, 2, 3, 4, 5, 0, time.UTC) // 早于固定 now（2026-09-19）→ 已过期
	spec := `{"v":1,"document":{"chartType":"bar","title":"demo"}}`

	mock.ExpectQuery(`SELECT `).
		WillReturnRows(sqlmock.NewRows(recordColumns()).AddRow(
			insertedID, spec, "hash", 3, 14, "build",
			120, 340, "10.0.0.9", createdAt, nil, 4,
			expiresAt, nil, nil,
		))
	mock.ExpectExec(`UPDATE `).WillReturnResult(sqlmock.NewResult(0, 1))

	record, err := svc.GetByShortID(context.Background(), insertedShortID)
	if err != nil {
		t.Fatalf("GetByShortID: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("期望未满足: %v", err)
	}

	if record.QueryID != insertedShortID {
		t.Fatalf("query_id = %q, want %q", record.QueryID, insertedShortID)
	}
	if record.DatasetID != 3 || record.ChartID == nil || *record.ChartID != 14 {
		t.Fatalf("dataset/chart 还原错误: %+v", record)
	}
	if record.Spec.V != 1 || string(record.Spec.Document) != `{"chartType":"bar","title":"demo"}` {
		t.Fatalf("spec 未原样还原: %s", string(record.Spec.Document))
	}
	if !record.Expired {
		t.Fatal("expires_at 早于当前时间，应判定为已过期")
	}
	if record.ExpiresAt == nil || *record.ExpiresAt != "2026-04-02T03:04:05Z" {
		t.Fatalf("expires_at = %v", record.ExpiresAt)
	}
	if record.HitCount != 5 {
		t.Fatalf("hit_count = %d, want 5（原值 4 再 +1）", record.HitCount)
	}

	trace := capture.find(t, "UPDATE ")
	for _, want := range []string{`"hit_count"`, `"last_accessed_at"`} {
		if !strings.Contains(trace, want) {
			t.Errorf("留痕 SQL 缺少 %s：\n%s", want, trace)
		}
	}
	// 留痕只碰这两列：刷新 expires_at 会让「过期」永远不来
	if strings.Contains(trace, `"expires_at"`) {
		t.Errorf("留痕 SQL 不应触碰 expires_at：\n%s", trace)
	}
}

// TestGetByShortID_PermanentRecordNeverExpires expires_at 为 NULL 表示永久。
func TestGetByShortID_PermanentRecordNeverExpires(t *testing.T) {
	svc, mock, _ := newTestService(t)
	spec := `{"v":1,"document":{"chartType":"table"}}`
	mock.ExpectQuery(`SELECT `).
		WillReturnRows(sqlmock.NewRows(recordColumns()).AddRow(
			insertedID, spec, "hash", 1, nil, "build",
			nil, nil, "", time.Now(), nil, 0, nil, nil, nil,
		))
	mock.ExpectExec(`UPDATE `).WillReturnResult(sqlmock.NewResult(0, 1))

	record, err := svc.GetByShortID(context.Background(), insertedShortID)
	if err != nil {
		t.Fatalf("GetByShortID: %v", err)
	}
	if record.Expired {
		t.Fatal("expires_at 为 NULL 表示永久，不应判定为过期")
	}
	if record.ExpiresAt != nil {
		t.Fatalf("expires_at 应为 nil，实际 %v", *record.ExpiresAt)
	}
	if record.ChartID != nil {
		t.Fatalf("chart_id 为 NULL 时应回 nil，实际 %v", *record.ChartID)
	}
}

func TestGetByShortID_Errors(t *testing.T) {
	svc, mock, _ := newTestService(t)

	// 短码格式非法：必须在不触库的前提下给出 20100（手工改坏地址栏是常规操作）
	_, err := svc.GetByShortID(context.Background(), "not-a-valid-short-id")
	if err == nil {
		t.Fatal("非法短码应报错")
	}
	if code := bizCode(t, err); code != response.CodeBadRequest {
		t.Fatalf("非法短码错误码 = %d, want %d", code, response.CodeBadRequest)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("非法短码不应访问数据库: %v", err)
	}

	// 合法但不存在：20300，而不是 500
	mock.ExpectQuery(`SELECT `).WillReturnError(sql.ErrNoRows)
	_, err = svc.GetByShortID(context.Background(), insertedShortID)
	if err == nil {
		t.Fatal("记录不存在应报错")
	}
	if code := bizCode(t, err); code != response.CodeNotFound {
		t.Fatalf("记录不存在错误码 = %d, want %d", code, response.CodeNotFound)
	}
}

// TestGetByShortID_SpecCorruptedFails 库里的 spec_json 损坏时必须显式失败，
// 不能返回半截配置让前端把用户带到一个「看起来正常但条件不对」的图表上。
func TestGetByShortID_SpecCorruptedFails(t *testing.T) {
	svc, mock, _ := newTestService(t)
	mock.ExpectQuery(`SELECT `).
		WillReturnRows(sqlmock.NewRows(recordColumns()).AddRow(
			insertedID, `{"v":1,`, "hash", 1, nil, "build",
			nil, nil, "", time.Now(), nil, 0, nil, nil, nil,
		))

	if _, err := svc.GetByShortID(context.Background(), insertedShortID); err == nil {
		t.Fatal("spec_json 损坏应报错")
	}
}

// TestGetByShortID_TraceFailureIsNotFatal 留痕写失败只告警，读取照常成功。
func TestGetByShortID_TraceFailureIsNotFatal(t *testing.T) {
	svc, mock, _ := newTestService(t)
	mock.ExpectQuery(`SELECT `).
		WillReturnRows(sqlmock.NewRows(recordColumns()).AddRow(
			insertedID, `{"v":1,"document":{}}`, "hash", 1, nil, "build",
			nil, nil, "", time.Now(), nil, 0, nil, nil, nil,
		))
	mock.ExpectExec(`UPDATE `).WillReturnError(errors.New("db down"))

	if _, err := svc.GetByShortID(context.Background(), insertedShortID); err != nil {
		t.Fatalf("留痕失败不应让读取失败: %v", err)
	}
}

// TestShortIDIsURLSafe 短码只允许 base58 字母表内的字符，避免地址栏转义。
func TestShortIDIsURLSafe(t *testing.T) {
	allowed := regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{21}$`)
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("NewV7: %v", err)
	}
	short, err := idcodec.Encode(id)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !allowed.MatchString(short) {
		t.Fatalf("短码 %q 不在 base58 21 位字母表内", short)
	}
}
