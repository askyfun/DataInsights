# Batch 1：安全地基与查询收敛 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 消除 SQL 注入面（打通参数化、封堵裸拼接、收敛 4 条 SQL 通道为 1 条），完成密码存储安全化（AES-GCM / bcrypt）、可观测性修复（Sentry/requestID）、CORS 收紧，并引入版本化数据库迁移。

**Architecture:** 后端 Go 1.26 + Gin + bun + pgx。核心改动：`datasource.Connection.Execute` 接口增加 variadic args（4 个驱动透传，pgx 需做 `?`→`$N` 重绑定）；`query` 包成为唯一 SQL 生成/执行通道；加密下沉到 service 层读写路径；DDL 从 `model.CreateTables` 迁到 goose 版本化迁移文件。

**Tech Stack:** Go 1.26, Gin, bun ORM, pgx/v5, go-sql-driver/mysql, clickhouse-go/v2, pressly/goose/v3, golang.org/x/crypto/bcrypt, sentry-go。

**Spec:** 12 项已确认决策（grilling 会话产出）：内网团队定位、集中重构、OpenAPI spec 先行、查询彻底收敛、config 立即 schema 化、启用泛型 router、特征测试防回归、安全→契约→前端顺序、AES-GCM、引入迁移工具、前端结构性重组。本仓架构评审结论摘要见文末附录。

## 三批路线图（本计划只实现 Batch 1）

| 批次 | 内容 | 计划文档 |
|------|------|----------|
| **Batch 1（本计划）** | 安全地基 + 查询收敛 + 迁移工具 | 本文档 |
| Batch 2 | OpenAPI spec 先行（oapi-codegen + openapi-typescript）、启用泛型 router、bi_chart.config schema 化 | 待 Batch 1 落地后编写（任务级细节依赖收敛后的接口签名） |
| Batch 3 | 前端结构重组（store 切片、ChartBuilder feature 化、删双 axios/死代码） | 待 Batch 2 生成前端类型后编写 |

## Global Constraints

- 禁止 `as any` / `@ts-ignore`（前端）；禁止空错误块，Go 错误必须 `%w` 包装或记录日志
- Bug 修复一律先写失败测试再修复（本项目零容忍规则）
- 每个任务完成必须通过：`cd backend && go test -race ./...`；涉及前端时加 `cd frontend && pnpm test`
- JSON 字段命名 snake_case；HTTP 恒 200 + 业务码的响应契约**保留不变**（Batch 2 才由 OpenAPI 建模）
- 查询类型 `sql` 的数据集允许用户写任意 SQL —— 这是产品功能，不是注入漏洞；需要封堵的是**标识符类输入**（表名、字段名）进入 SQL 的路径
- 内网团队定位：不做认证/RBAC（阶段八），不做 KMS
- 死代码"要么用要么删"，本批删除项见 Task 14

---

### Task 0: 落地工作区基线

**Files:**
- Modify: `frontend/src/pages/ChartBuilder.tsx`（已改好，待提交）
- Test: `frontend/src/__tests__/pages/ChartBuilder.test.tsx`（已改好，待提交）

**Interfaces:**
- Produces: 干净的工作区，后续所有任务的 diff 都可追溯。

- [ ] **Step 1: 跑前端测试确认已验证的修复仍通过**

Run: `cd frontend && npx vitest run src/__tests__/pages/ChartBuilder.test.tsx src/__tests__/pages/ChartBuilder.dragOverlay.test.tsx`
Expected: 10 passed

- [ ] **Step 2: 提交既有修复（不含 .wolf/.mimosa/MEMORY.md）**

```bash
cd /Users/asky/code/data-insights
git add frontend/src/pages/ChartBuilder.tsx frontend/src/__tests__/pages/ChartBuilder.test.tsx frontend/src/__tests__/pages/ChartBuilder.dragOverlay.test.tsx
git commit -m "fix(chart-builder): map definition row index to kind-local group index"
```

- [ ] **Step 3: 后端测试基线**

Run: `cd backend && go test -race ./...`
Expected: 全部 PASS（若有既有失败，先记录到任务输出中，不得静默跳过）

---

### Task 1: 修复 requestID 中间件（随机字节从未填充）

**Files:**
- Modify: `backend/cmd/main.go:113-123`
- Test: `backend/cmd/main_test.go`（新建）

**Interfaces:**
- Produces: `requestIDMiddleware() gin.HandlerFunc`（签名不变），行为：客户端带 `X-Request-ID` 则透传，否则生成 16 字节 crypto/rand hex（32 字符），并放入 `c.Request.Context()` 供后续日志使用。

- [ ] **Step 1: 写失败测试**

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestIDMiddlewareGeneratesRandomID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(requestIDMiddleware())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))

	got := w.Header().Get("X-Request-ID")
	if got == "" || got == "0000000000000000" {
		t.Fatalf("expected random request id, got %q", got)
	}
}

func TestRequestIDMiddlewarePreservesIncomingID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(requestIDMiddleware())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", "abc123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-ID"); got != "abc123" {
		t.Fatalf("expected preserved id abc123, got %q", got)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -race ./cmd/ -run TestRequestID -v`
Expected: FAIL（`expected random request id, got "0000000000000000"`）

- [ ] **Step 3: 修复实现**

`backend/cmd/main.go` 中 `requestIDMiddleware` 改为：

```go
func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			b := make([]byte, 16)
			if _, err := rand.Read(b); err != nil {
				slog.Error("generate request id failed", "error", err)
				requestID = fmt.Sprintf("%d", time.Now().UnixNano())
			} else {
				requestID = hex.EncodeToString(b)
			}
			c.Request.Header.Set("X-Request-ID", requestID)
		}
		c.Header("X-Request-ID", requestID)
		c.Set("requestID", requestID)
		c.Next()
	}
}
```

import 增加 `crypto/rand` 与 `encoding/hex`（`time` 已有）。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test -race ./cmd/ -run TestRequestID -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/cmd/main.go backend/cmd/main_test.go
git commit -m "fix(middleware): fill random bytes for generated request id"
```

---

### Task 2: NewDriver 未知类型返回错误（消除 nil panic）

**Files:**
- Modify: `backend/internal/datasource/driver.go:80-93`
- Test: `backend/internal/datasource/driver_test.go`（若无则新建）

**Interfaces:**
- Produces: `NewDriver(driverType DriverType) (Driver, error)`——未知类型返回 `(nil, fmt.Errorf(...))`。
- Consumes: 调用方 `service/datasource/impl.go`、`service/dataset/impl.go`、`service/chart/impl.go` 的 `connect()` 需同步确认 err 判断覆盖 nil Driver 场景（修复后天然覆盖）。

- [ ] **Step 1: 写失败测试**

```go
package datasource

import "testing"

func TestNewDriverUnknownTypeReturnsError(t *testing.T) {
	d, err := NewDriver(DriverType("oracle"))
	if err == nil {
		t.Fatal("expected error for unknown driver type")
	}
	if d != nil {
		t.Fatalf("expected nil driver, got %v", d)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -race ./internal/datasource/ -run TestNewDriverUnknown -v`
Expected: FAIL（当前返回 `(nil, nil)`，err==nil）

- [ ] **Step 3: 修复**

`driver.go` 的 default 分支改为：

```go
	default:
		return nil, fmt.Errorf("unsupported driver type: %s", driverType)
```

- [ ] **Step 4: 跑测试与全仓测试**

Run: `cd backend && go test -race ./internal/datasource/ -run TestNewDriverUnknown -v && go build ./...`
Expected: PASS；若编译报错说明有调用方未处理 err（本来就该处理），逐个修复。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/datasource/driver.go backend/internal/datasource/driver_test.go
git commit -m "fix(datasource): return error instead of nil,nil for unknown driver type"
```

---

### Task 3: `Connection.Execute` 接口增加参数化 args（通道打通，保持行为）

**Files:**
- Modify: `backend/internal/datasource/driver.go:76`（接口签名）
- Modify: `backend/internal/datasource/postgresql.go:131`、`mysql.go:130`、`clickhouse.go:117`、`starrocks.go:130`（4 处实现）
- Modify: `backend/internal/query/executor.go:83,93,152,183`、`backend/internal/service/datasource/impl.go:188,225,239,377,397`、`backend/internal/service/chart/impl.go:143`、`backend/internal/service/dataset/impl.go:207,249`（调用点：本任务全部传 `nil` 保持行为不变）
- Test: `backend/internal/datasource/args_test.go`（新建，仅测可编译性与 sqlmock 转发，真实参数化行为在 Task 4 验证）

**Interfaces:**
- Produces: `Execute(ctx context.Context, sql string, args ...any) (*QueryResult, error)`。本任务只改签名并透传，**不改变任何现有调用行为**——这是纯接口先行，Task 4 才让 executor 真正传 args。
- pgx（postgresql.go）注意：pgx 用 `$1` 占位符，需加 `rebind` 把 bun 生成的 `?` 转为 `$N`；MySQL/StarRocks/clickhouse-go v2 原生支持 `?`，直接透传。

- [ ] **Step 1: 写驱动参数透传的失败测试**

`backend/internal/datasource/args_test.go`（用 go-sql-driver 的 sqlmock 只能测 mysql 路径；PG 用 pgx mock 成本高，采用"编译期接口断言 + mysql 转发"策略）：

```go
package datasource

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// 验证 mysql 驱动把 args 透传给底层 QueryContext（占位符 ? 与参数个数匹配才不报错）。
func TestMySQLExecutePassesArgs(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id"}).AddRow(1)
	mock.ExpectQuery("SELECT \\?").
		WithArgs("hello").
		WillReturnRows(rows)

	c := &mysqlConnection{db: db}
	res, err := c.Execute(context.Background(), "SELECT ?", "hello")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(res.Rows))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("args were not passed through: %v", err)
	}
}
```

注意：先读 `mysql.go` 中 `mysqlConnection` 的实际字段名（可能是 `db *sql.DB` 或别名），测试中的结构体字面量按真实字段调整。

- [ ] **Step 2: 跑测试确认失败（接口尚无 args 参数）**

Run: `cd backend && go test -race ./internal/datasource/ -run TestMySQLExecutePassesArgs`
Expected: 编译失败 `too many arguments in call to c.Execute`

- [ ] **Step 3: 改接口与 4 个驱动实现**

`driver.go`：

```go
	// Execute executes a query with positional args (? placeholders).
	Execute(ctx context.Context, sql string, args ...any) (*QueryResult, error)
```

`mysql.go` / `starrocks.go`（database/sql 系，直接透传）：

```go
func (c *mysqlConnection) Execute(ctx context.Context, sql string, args ...any) (*QueryResult, error) {
	rows, err := c.db.QueryContext(ctx, sql, args...)
```

`clickhouse.go`（clickhouse-go v2 同样支持 `?`）：

```go
func (c *clickhouseConnection) Execute(ctx context.Context, sql string, args ...any) (*QueryResult, error) {
	rows, err := c.conn.Query(ctx, sql, args...)
```

`postgresql.go`（pgx 需要 `?`→`$N` 重绑定，pgx 的 Query 本身是 variadic）：

```go
// rebind converts '?' placeholders to pgx's '$N' ordinals.
func rebind(query string) string {
	n := 0
	var b strings.Builder
	for _, r := range query {
		if r == '?' {
			n++
			b.WriteString("$" + strconv.Itoa(n))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (c *postgresqlConnection) Execute(ctx context.Context, sql string, args ...any) (*QueryResult, error) {
	rows, err := c.pool.Query(ctx, rebind(sql), args...)
```

- [ ] **Step 4: 全仓编译并机械更新所有调用点传 `nil`**

Run: `cd backend && go build ./...` —— 报错的每个调用点改为 `Execute(ctx, sql)`（variadic，零 args 无需显式 nil）。
Expected: 编译通过，行为与之前完全一致。

- [ ] **Step 5: 全量测试**

Run: `cd backend && go test -race ./...`
Expected: PASS（不引入任何行为变化）

- [ ] **Step 6: Commit**

```bash
git add backend/internal/datasource/ backend/internal/query/ backend/internal/service/
git commit -m "refactor(datasource): add variadic args to Connection.Execute for parameterized queries"
```

---

### Task 4: Executor 真正传递参数化 args + 修复假 JSON 解析器

**Files:**
- Modify: `backend/internal/query/dialect.go:314`（`BuildQueryStringWithBun` 返回值增加 args）
- Modify: `backend/internal/query/bun_builder.go`（`BunSQLBuilder.BuildSelect/BuildCount` 收集 args；`unmarshalJSON:379` 换成真 `json.Unmarshal`）
- Modify: `backend/internal/query/executor.go:70,83,93,152`（接收并传递 args）
- Test: `backend/internal/query/bun_builder_test.go`（追加）、`internal/query/executor_full_test.go`（追加）

**Interfaces:**
- Consumes: Task 3 的 `Execute(ctx, sql, args ...any)`。
- Produces: `BuildQueryStringWithBun(dialect DialectType, ast *QueryAST) (selectSQL string, countSQL string, args []any)`——Batch 2 之前这是全仓唯一的 SQL 生成出口。
- `sanitizer.go` 的 `escapeString` 保留给旧路径兜底，但参数化链路不再依赖它（值经 args 传入，不进 SQL 字符串）。

- [ ] **Step 1: 写失败测试（filter 值必须走占位符，不落入 SQL 文本）**

追加到 `backend/internal/query/bun_builder_test.go`：

```go
func TestBuildQueryStringWithBunCollectsArgs(t *testing.T) {
	ast := &QueryAST{
		Source:     "orders",
		SourceType: SourceTypeTable,
		SelectItems: []SelectItem{{Expr: "region"}},
		Filters: []FilterExpr{
			{FieldExpr: "region", Op: FilterEq, Value: "'; DROP TABLE users; --"},
		},
	}
	selectSQL, _, args := BuildQueryStringWithBun(DialectPostgreSQL, ast)

	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d: args=%v", len(args), args)
	}
	if args[0] != "'; DROP TABLE users; --" {
		t.Fatalf("malicious value must travel as arg, got %v", args[0])
	}
	if strings.Contains(selectSQL, "DROP TABLE") {
		t.Fatalf("value leaked into SQL text: %s", selectSQL)
	}
	if !strings.Contains(selectSQL, "?") {
		t.Fatalf("expected ? placeholder in SQL: %s", selectSQL)
	}
}
```

（`FilterEq`、`SelectItem` 等类型名以 `internal/query/ast.go` 实际定义为准，先读后写。）

- [ ] **Step 2: 写假 JSON 解析器的失败测试**

```go
func TestUnmarshalJSONActuallyParses(t *testing.T) {
	var cols []map[string]any
	err := unmarshalJSON(`[{"name":"a","expr":"b"}]`, &cols)
	if err != nil {
		t.Fatalf("valid json must parse, got: %v", err)
	}
	if len(cols) != 1 || cols[0]["name"] != "a" {
		t.Fatalf("parsed content wrong: %+v", cols)
	}
}
```

- [ ] **Step 3: 跑测试确认失败**

Run: `cd backend && go test -race ./internal/query/ -run "TestBuildQueryStringWithBunCollectsArgs|TestUnmarshalJSONActuallyParses" -v`
Expected: FAIL（当前签名返回两个值；当前 unmarshalJSON 恒返回 nil 且不填充目标）

- [ ] **Step 4: 实现**

`bun_builder.go`：
1. `unmarshalJSON` 函数体替换为 `return json.Unmarshal([]byte(data), v)`（import `encoding/json`），删除全部假解析逻辑。
2. `BunSQLBuilder.BuildSelect(ast)`/`BuildCount(ast)` 改为同时返回 `[]any`：内部已通过 `buildWhereParts(ast, &args)` 收集，现在把它暴露出来。`BuildQueryStringWithBun` 签名改为：

```go
func BuildQueryStringWithBun(dialect DialectType, ast *QueryAST) (string, string, []any) {
	qb := NewBunSQLBuilder(dialect)
	selectSQL, selectArgs := qb.BuildSelect(ast)
	countSQL, countArgs := qb.BuildCount(ast)
	_ = countArgs // count 与 select 的 filter args 相同
	return selectSQL, countSQL, selectArgs
}
```

3. `executor.go` 三处 `e.conn.Execute(ctx, sql)` 改为 `e.conn.Execute(ctx, sql, args...)`（count 查询同样传 args）。

- [ ] **Step 5: 跑 query 包全量测试**

Run: `cd backend && go test -race ./internal/query/ -v`
Expected: 全部 PASS（旧的 `BuildQueryStringWithBun` 两个返回值调用点会编译报错，逐个更新——全仓只有 executor.go 和测试引用它）

- [ ] **Step 6: 全量测试 + Commit**

Run: `cd backend && go test -race ./...`

```bash
git add backend/internal/query/ backend/internal/service/
git commit -m "feat(query): thread parameterized args from builder to driver; fix fake json unmarshaler"
```

---

### Task 5: 标识符校验统一为白名单

**Files:**
- Modify: `backend/internal/datasource/driver.go:96-103`（导出 `IsValidIdentifier`）
- Modify: `backend/internal/query/bun_builder.go:369-377`（`safeIdentifier` 复用白名单）
- Modify: `backend/internal/datasource/postgresql.go:78-84`（`GetColumns` 拼表名前先校验）
- Test: `backend/internal/datasource/driver_test.go`、`backend/internal/query/bun_builder_test.go`（追加）

**Interfaces:**
- Produces: `datasource.IsValidIdentifier(name string) bool`（白名单 `^[a-zA-Z0-9_.]+$`，为全仓唯一标识符校验实现）。删除 `query` 包的黑名单版 `safeIdentifier` 逻辑。

- [ ] **Step 1: 写失败测试**

```go
func TestSafeIdentifierRejectsInjection(t *testing.T) {
	cases := map[string]bool{
		"region":      true,
		"bi_orders.id": true,
		"a;b":         false,
		"a'b":         false,
		"a\"b":        false,
		"a-b":         false,
		"")(; DROP":   false,
		"":            false,
		"scoreavg":    true,
	}
	for in, want := range cases {
		if got := IsValidIdentifier(in); got != want {
			t.Errorf("IsValidIdentifier(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSafeIdentifierFallsBackOnInvalid(t *testing.T) {
	if got := safeIdentifier("a;b"); got != "_invalid_identifier" {
		t.Fatalf("expected fallback, got %q", got)
	}
}
```

（`safeIdentifier` 保留函数名与 fallback 行为——查询构造器遇到非法标识符时生成占位名并让查询显式失败，而不是 panic；但判定逻辑改为调用 `datasource.IsValidIdentifier`。）

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -race ./internal/datasource/ ./internal/query/ -run "Identifier" -v`
Expected: FAIL（`IsValidIdentifier` 未导出/不存在）

- [ ] **Step 3: 实现**

`driver.go`：`isValidIdentifier` 重命名为导出的 `IsValidIdentifier`（包内 `GetPrimaryKeys` 等引用同步更新）。
`bun_builder.go`：

```go
func safeIdentifier(name string) string {
	name = strings.TrimSpace(name)
	if !datasource.IsValidIdentifier(name) {
		return "_invalid_identifier"
	}
	return name
}
```

`postgresql.go` 的 `GetColumns` 开头加：

```go
	if !IsValidIdentifier(tableName) {
		return nil, fmt.Errorf("invalid table name: %q", tableName)
	}
```

- [ ] **Step 4: 跑测试**

Run: `cd backend && go test -race ./internal/... && go test -race ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/datasource/ backend/internal/query/
git commit -m "fix(security): unify identifier validation to whitelist across drivers and query builder"
```

---

### Task 6: 封堵 service 层标识符注入点

**背景（区分两类输入）**：`querySQL` 是产品允许的用户自定义 SQL（数据集功能），**保留**；`fieldName`、`tableName` 等标识符类输入进入 SQL 前必须过 `IsValidIdentifier`。

**Files:**
- Modify: `backend/internal/service/datasource/impl.go`（`GetFieldDistribution` ~216-233、`Preview` ~169-197）
- Modify: `backend/internal/service/dataset/impl.go`（涉及 tableName 拼接处，读文件定位）
- Test: `backend/internal/service/datasource/impl_test.go`（若无则新建，用 sqlite/pgx 测试连接或接口替身）

**Interfaces:**
- Consumes: Task 5 的 `datasource.IsValidIdentifier`。
- Produces: `GetFieldDistribution` 对非法 `fieldName` 返回 400 语义错误（`fmt.Errorf("invalid field name: %q", fieldName)`）；合法行为不变。

- [ ] **Step 1: 写失败测试**

```go
func TestGetFieldDistributionRejectsInvalidFieldName(t *testing.T) {
	s := newTestDatasourceService(t) // 按现有测试基建构造；无基建则用最小 struct + nil db
	_, err := s.GetFieldDistribution(context.Background(), 1, "orders", "", "table", "1; DROP TABLE x", 10)
	if err == nil || !strings.Contains(err.Error(), "invalid field name") {
		t.Fatalf("expected invalid field name error, got %v", err)
	}
}
```

若 service 测试需要 DB，遵循仓库现有 `service` 测试的模式（先 `grep -rn "_test.go" internal/service/` 确认基建；没有则测试放 handler 层或为 `GetFieldDistribution` 抽出纯函数 `buildFieldDistributionSQL(fieldName, source string, limit int) (string, error)` 并对该纯函数测试——优先抽纯函数，可测性最好）。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -race ./internal/service/datasource/ -run TestGetFieldDistribution -v`
Expected: FAIL（当前未校验，直接拼接）

- [ ] **Step 3: 实现**

`GetFieldDistribution` 两个分支（sql/table）拼接前统一加：

```go
	if !datasource.IsValidIdentifier(fieldName) {
		return nil, fmt.Errorf("invalid field name: %q", fieldName)
	}
```

`Preview` 的 table 分支加同样的 `tableName` 校验（sql 分支的 `querySQL` 是产品功能，保留，但移除 `fmt.Sprintf("%s LIMIT 10", ...)` 的隐患——改为包一层子查询 `SELECT * FROM (%s) AS _preview LIMIT 10`，避免用户 SQL 自带分号/LIMIT 时拼接出错）。

- [ ] **Step 4: 跑测试 + 全量**

Run: `cd backend && go test -race ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/
git commit -m "fix(security): validate identifiers before interpolation in datasource/dataset services"
```

---

### Task 7: SQL 生成收敛到 query 包（消灭 service 层手拼）

**Files:**
- Create: `backend/internal/query/raw.go`（新增 RawSQL 构造：preview 子查询、字段分布、行数统计）
- Create: `backend/internal/query/raw_test.go`
- Modify: `backend/internal/service/datasource/impl.go`（Preview/GetFieldDistribution/GetTableData 的拼串改调 query 包）
- Modify: `backend/internal/service/dataset/impl.go:207,249`（同上）
- Modify: `backend/internal/service/chart/impl.go:114-149`（GetData 的 `SELECT * FROM (...) LIMIT 100` 改调 query 包）
- Delete: `backend/internal/query/builder.go`（旧 Builder，仅测试引用；其测试一并删除——它已被 AST/bun_builder 取代，属于"删除死抽象"决策）
- Modify: `backend/internal/query/dialect.go:125-254`（三个复制粘贴 builder：postgres 委托 mysql 已是现状，保留委托但删除逐行复制注释；本任务不重写方言层，StarRocks 显式映射放到 Task 7 收尾）

**Interfaces:**
- Produces（`backend/internal/query/raw.go`）:

```go
// WrapPreviewSQL wraps a user source (table or subquery) for preview.
func WrapPreviewSQL(source string, sourceType SourceType, limit int) string
// BuildFieldDistributionSQL builds GROUP BY distribution over a source.
func BuildFieldDistributionSQL(field string, source string, sourceType SourceType, limit int) (string, error)
// WrapCountSQL builds SELECT COUNT(*) over a source.
func WrapCountSQL(source string, sourceType SourceType) string
```

- Consumes: Task 5 的标识符校验（`BuildFieldDistributionSQL` 内部校验 field/sourceType==table 时的 source）。

- [ ] **Step 1: 写失败测试**

`backend/internal/query/raw_test.go`：

```go
func TestBuildFieldDistributionSQL(t *testing.T) {
	sql, err := BuildFieldDistributionSQL("region", "orders", SourceTypeTable, 20)
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT region, COUNT(*) as _count FROM orders GROUP BY region ORDER BY _count DESC LIMIT 20"
	if sql != want {
		t.Fatalf("got %s want %s", sql, want)
	}
}

func TestBuildFieldDistributionSQLSubquery(t *testing.T) {
	sql, err := BuildFieldDistributionSQL("region", "SELECT * FROM t", SourceTypeSQL, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sql, "SELECT region, COUNT(*) as _count FROM (SELECT * FROM t) as _subquery") {
		t.Fatalf("unexpected: %s", sql)
	}
}

func TestBuildFieldDistributionSQLInvalidField(t *testing.T) {
	_, err := BuildFieldDistributionSQL("a;b", "orders", SourceTypeTable, 20)
	if err == nil {
		t.Fatal("expected error for invalid identifier")
	}
}

func TestWrapPreviewSQL(t *testing.T) {
	if got := WrapPreviewSQL("SELECT * FROM t", SourceTypeSQL, 10); got != "SELECT * FROM (SELECT * FROM t) AS _preview LIMIT 10" {
		t.Fatalf("got %s", got)
	}
	if got := WrapPreviewSQL("orders", SourceTypeTable, 10); got != "SELECT * FROM orders LIMIT 10" {
		t.Fatalf("got %s", got)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -race ./internal/query/ -run "TestBuildFieldDistribution|TestWrapPreview" -v`
Expected: 编译失败（raw.go 不存在）

- [ ] **Step 3: 实现 raw.go 并迁移 4 处调用**

`raw.go` 按上述接口实现（表源直接拼接已校验标识符；SQL 源包子查询）。然后把 `service/datasource/impl.go` 的 Preview/GetFieldDistribution/GetTableData、`service/dataset/impl.go` 两处、`service/chart/impl.go` 的 GetData 全部改为调用这些函数 + `executor.ExecuteRawQuery`（已存在，executor.go:182）或 `conn.Execute`。每个 service 方法不再出现 `fmt.Sprintf` 拼 SQL。

- [ ] **Step 4: 删除 query/builder.go**

Run: `git rm backend/internal/query/builder.go && cd backend && go build ./...`
若测试引用旧 Builder（`builder_test.go`），一并 `git rm`——旧 Builder 是被 AST 链路取代的死抽象，删除决策已确认。

- [ ] **Step 5: 方言收尾：StarRocks 显式映射 + 不支持的粒度显式报错**

`dialect.go` 的 `ParseDialect`（~294）加 `case DriverStarRocks: return DialectMySQL`；`bun_builder.go` 的 `renderDimensionGroupBy`（~277-302）对 MySQL/CH 下非 `day` 的粒度返回错误（通过把 BuildSelect 签名改为可返回 error，或粒度降级改为在 AST 校验阶段报错——**以现实现最小改动为准，原则是不再静默错误结果**）。

- [ ] **Step 6: 全量测试 + Commit**

Run: `cd backend && go test -race ./...`

```bash
git add backend/internal/
git commit -m "refactor(query): consolidate raw SQL construction into query package; drop legacy builder"
```

---

### Task 8: bi_datasource.password AES-GCM 加密存储

**Files:**
- Create: `backend/internal/crypto/aesgcm.go` + `backend/internal/crypto/aesgcm_test.go`
- Modify: `backend/internal/config/config.go`（增加 `Security.SecurityKey string`）
- Modify: `backend/etc/config.toml`（增加 `SecurityKey`，值从环境变量读取，见 Step 3）
- Modify: `backend/internal/service/datasource/impl.go`（写入时加密、连接时解密）
- Modify: `backend/cmd/routes.go`（把 key 注入 service 构造）
- Test: `backend/internal/crypto/aesgcm_test.go`、`backend/internal/service/datasource/impl_test.go`（追加）

**Interfaces:**
- Produces（`backend/internal/crypto/aesgcm.go`）:

```go
// Encrypt encrypts plaintext with AES-256-GCM, returns base64(nonce||ciphertext).
func Encrypt(key []byte, plaintext string) (string, error)
// Decrypt reverses Encrypt. Returns ErrNotEncrypted if input is not our format
// (legacy plaintext detection for migration).
func Decrypt(key []byte, ciphertext string) (string, error)
var ErrNotEncrypted = errors.New("value is not aes-gcm encrypted")
```

密文格式：`v1:` 前缀 + base64(nonce ‖ ciphertext+tag)。`Decrypt` 遇到无 `v1:` 前缀的值返回 `ErrNotEncrypted`（Task 8 用它兼容存量明文，读取时自动升级加密）。

- [ ] **Step 1: 写失败测试**

```go
package crypto

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	ct, err := Encrypt(key, "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if ct == "s3cret" || len(ct) < 20 {
		t.Fatalf("ciphertext looks wrong: %q", ct)
	}
	pt, err := Decrypt(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if pt != "s3cret" {
		t.Fatalf("got %q", pt)
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	k1, k2 := make([]byte, 32), make([]byte, 32)
	k1[0] = 1
	ct, _ := Encrypt(k1, "s3cret")
	if _, err := Decrypt(k2, ct); err == nil {
		t.Fatal("expected error with wrong key")
	}
}

func TestDecryptLegacyPlaintext(t *testing.T) {
	key := make([]byte, 32)
	_, err := Decrypt(key, "legacy-plaintext")
	if err != ErrNotEncrypted {
		t.Fatalf("expected ErrNotEncrypted, got %v", err)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test -race ./internal/crypto/ -v`
Expected: 编译失败（包不存在）

- [ ] **Step 3: 实现**

`crypto/aesgcm.go`：AES-256-GCM，`crypto/rand` 生成 12 字节 nonce，输出 `v1:` + base64(nonce‖sealed)。config 增加：

```go
type Config struct {
	// ... existing
	Security SecurityConfig `toml:"Security"`
}
type SecurityConfig struct {
	SecurityKey string `toml:"SecurityKey"`
}
```

`config.toml` 增加 `[Security] SecurityKey = "${DATARAY_SECURITY_KEY}"` 不适用（TOML 不做 env 替换）——改为在 `LoadConfig` 后支持 env 覆盖：`if v := os.Getenv("DATARAY_SECURITY_KEY"); v != "" { c.Security.SecurityKey = v }`。main.go 启动时校验：key 长度必须 32 字节（`len(key) != 32` 则 exit 1，提示用 `openssl rand -hex 16` 之外的 32 字节 hex）。key 解析：hex 字符串 → 32 bytes。

- [ ] **Step 4: 写 service 层失败测试（写入加密、读取解密、存量明文自动升级）**

在 service/datasource 测试中验证：Create 后 model 里存的 password 以 `v1:` 开头；`connect()` 能解密出原文；存量明文记录在 connect 时解密失败走 `ErrNotEncrypted` 分支 → 用原文连接并回写加密值（测试用接口替身覆盖 DB 写）。

- [ ] **Step 5: 实现 service 集成并跑全量测试**

Run: `cd backend && go test -race ./... && go build ./...`

- [ ] **Step 6: Commit**

```bash
git add backend/internal/ backend/cmd/ backend/etc/
git commit -m "feat(security): encrypt datasource password at rest with AES-256-GCM, auto-upgrade legacy plaintext"
```

---

### Task 9: share.password bcrypt 哈希 + API 脱敏

**Files:**
- Modify: `backend/internal/service/share/impl.go`（创建时 hash ~97 附近；校验改 `bcrypt.CompareHashAndPassword`）
- Modify: `backend/internal/entity/share.go`（`Password` 字段从 JSON 输出移除）
- Modify: `backend/internal/entity/datasource.go`（`Password` 字段加 `json:"-"`，或用独立 DTO）
- Modify: `backend/go.mod`（`go get golang.org/x/crypto`）
- Test: `backend/internal/service/share/impl_test.go`（追加）

**Interfaces:**
- Produces: `GET /api/shares/:token` 与列表响应中不再出现 `password` 字段；`POST /api/shares` 接受明文密码，存储为 bcrypt hash（cost 10）。校验逻辑兼容存量明文：hash 格式校验失败时明文比较并回写 hash（与 Task 8 同模式）。

- [ ] **Step 1: 写失败测试**

```go
func TestCreateShareHashesPassword(t *testing.T) {
	s := newTestShareService(t)
	share, err := s.Create(ctx, CreateShareInput{ChartID: 1, Password: "pw123"})
	if err != nil {
		t.Fatal(err)
	}
	if share.Password == "pw123" {
		t.Fatal("password stored in plaintext")
	}
	if !strings.HasPrefix(share.Password, "$2") {
		t.Fatalf("expected bcrypt hash, got %q", share.Password)
	}
}

func TestGetShareResponseOmitsPassword(t *testing.T) {
	// 断言 entity.Share 序列化后 JSON 中没有 "password" 键
	data, _ := json.Marshal(entity.Share{Password: "x"})
	if strings.Contains(string(data), "password") {
		t.Fatalf("password leaked in json: %s", data)
	}
}
```

- [ ] **Step 2: 跑测试确认失败 → 实现 → 跑全量**

Run: `cd backend && go test -race ./internal/service/share/ ./internal/entity/ -v` 然后 `go test -race ./...`

- [ ] **Step 3: Commit**

```bash
git add backend/internal/ backend/go.mod backend/go.sum
git commit -m "feat(security): bcrypt share passwords and omit password fields from API responses"
```

---

### Task 10: 接通 Sentry + CORS 收紧

**Files:**
- Modify: `backend/internal/config/config.go`（增加 `Sentry.Dsn string`、`CORS.AllowedOrigins []string`）
- Modify: `backend/cmd/main.go`（sentry.Init + sentrygin 中间件；CORS 从配置读 origin，未配置时默认 `http://localhost:3000`）
- Modify: `backend/go.mod`（`go get github.com/getsentry/sentry-go`、`go get github.com/getsentry/sentry-go/gin`）
- Test: `backend/cmd/cors_test.go`（新建）

**Interfaces:**
- Produces: `etc/config.toml` 中已有的 `[Sentry] Dsn` 真正生效（DSN 为空则完全跳过初始化，测试环境无副作用）；CORS：`AllowedOrigins` 配置项，命中才回 `Access-Control-Allow-Origin: <origin>`，未命中不回（不再 `*`）。

- [ ] **Step 1: 写失败测试**

```go
func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(corsMiddleware([]string{"http://localhost:3000"}))
	r.GET("/ping", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("expected origin echo, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req2.Header.Set("Origin", "http://evil.example")
	r.ServeHTTP(w2, req2)
	if w2.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unexpected origin allowed: %q", w2.Header().Get("Access-Control-Allow-Origin"))
	}
}
```

（当前 CORS 是 main.go 内联匿名函数——先抽成 `corsMiddleware(origins []string) gin.HandlerFunc` 再测试。）

- [ ] **Step 2: 跑失败 → 实现（CORS 抽取 + config 增加 Sentry/CORS 字段 + sentry.Init）→ 跑全量**

Run: `cd backend && go test -race ./...`

- [ ] **Step 3: Commit**

```bash
git add backend/
git commit -m "feat(ops): wire sentry and configurable CORS origins"
```

---

### Task 11: 引入 goose 版本化迁移，替换 CreateTables

**Files:**
- Create: `backend/migrations/00001_init_schema.sql`（从 `model.CreateTables` 的 DDL 转写）
- Create: `backend/migrations/00002_encrypt_and_indexes.sql`（若 Task 8/9 需要表结构变更则放这里，无则跳过）
- Modify: `backend/internal/database/database.go`（`RunMigrations` 改调 goose）
- Modify: `backend/internal/model/model.go`（删除 `CreateTables` 及散落的 ALTER——`model.go:96-102`、`database.go:46-52`）
- Modify: `backend/go.mod`（`go get github.com/pressly/goose/v3`）
- Test: `backend/internal/database/migrations_test.go`（新建；需要可用 PG——用 docker-compose 的测试库，`TEST_DATABASE_URL` 未设置时 `t.Skip`）

**Interfaces:**
- Produces: `RunMigrations(db *sql.DB) error` 内部走 `goose.Up`，迁移文件嵌入二进制（`embed.FS`），部署无需额外步骤。事务抽象：新增 `backend/internal/database/tx.go`——`func WithTx(ctx context.Context, db *bun.DB, fn func(ctx context.Context, tx bun.Tx) error) error`（本批只供 service 的"先写后读"场景选用，不强推全面改造）。

- [ ] **Step 1: 生成迁移文件**

读 `backend/internal/model/model.go:11-123` 的全部 DDL，逐条转写为 `00001_init_schema.sql`（`CREATE TABLE IF NOT EXISTS` 保留 IF NOT EXISTS 语义，兼容存量库；索引一并写入）。用 `-- +goose Up` / `-- +goose Down` 标注（Down 只需 DROP TABLE）。

- [ ] **Step 2: 写迁移测试（需要 PG）**

```go
//go:build integration

func TestMigrationsUpToLatest(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.RunMigrations(db); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	// 幂等：再跑一遍必须成功
	if err := database.RunMigrations(db); err != nil {
		t.Fatalf("migrate idempotent: %v", err)
	}
}
```

- [ ] **Step 3: 切换 RunMigrations 到 goose，删除 CreateTables**

`database.go` 用 `embed.FS` + `goose.SetBaseFS` + `goose.Up`。删除 `model.CreateTables` 与两处散落 ALTER。`go build ./...` 清理引用。

- [ ] **Step 4: 本地实测迁移**

Run: `docker compose up -d postgres && cd backend && TEST_DATABASE_URL=postgres://... go test -race -tags integration ./internal/database/ -v`
Expected: PASS（含幂等）

- [ ] **Step 5: 全量测试 + Commit**

```bash
cd backend && go test -race ./...
git add backend/
git commit -m "feat(db): introduce goose versioned migrations, remove CreateTables"
```

---

### Task 12: 死代码清理

**Files（全部删除）:**
- Delete: `backend/internal/idls/`（4 个文件，零引用）
- Delete: `backend/internal/middleware/`（空目录）
- Delete: `backend/internal/domain/chart/`、`domain/dashboard/`、`domain/dataset/`、`domain/share/`（空目录）
- Delete: `backend/internal/domain/entity/` 中与 service 实现签名不一致的僵尸 Service 接口（保留实际被引用的 entity 类型；逐文件 grep 确认）
- Delete: `backend/internal/query/builder.go`（Task 7 已删，此处查漏）
- Delete: 仓库根 `main`、`server` 二进制、`coverage.out`，并在 `.gitignore` 补 `backend/bin/`、`*.out`
- 注意：`bi_dataset_lineage` 表**保留**（迁移文件里不动它；删除表属于 Batch 2 schema 决策）

**Interfaces:**
- Produces: 无新增接口。验证方式只有编译与全量测试。

- [ ] **Step 1: 逐项确认零引用后删除**

Run: `cd backend && grep -rn "internal/idls\|dataray/internal/idls" --include="*.go" .` 等逐包确认，然后 `git rm`。

- [ ] **Step 2: 全量测试 + Commit**

Run: `cd backend && go build ./... && go test -race ./...`

```bash
git add -A
git commit -m "chore: remove dead code (idls, empty dirs, zombie entity interfaces)"
```

---

### Task 13: 文档同步

**Files:**
- Modify: `AGENTS.md`（后端章节：Sentry 已接入、goose 迁移、router 状态描述修正为"Batch 2 将启用"、`Connection.Execute(ctx, sql, args...)` 签名）
- Modify: `docs/architecture.md`（查询通道收敛后的链路图：唯一通道 = query 包 AST + bun_builder + 参数化）
- Modify: `MEMORY.md`（追加经验：参数化通道断裂的根因——接口设计成只收 SQL 字符串导致 builder 层白做）

**Interfaces:** 无代码接口。

- [ ] **Step 1: 按 Batch 1 落地后的真实状态更新三份文档**
- [ ] **Step 2: Commit**

```bash
git add AGENTS.md docs/architecture.md MEMORY.md
git commit -m "docs: sync architecture docs with batch1 security and query consolidation"
```

---

## 验收标准（Batch 1 完成的定义）

1. `go test -race ./...` 全绿；前端测试不受影响
2. 全仓 `grep -rn "Sprintf" backend/internal/service/ backend/internal/query/` 不再出现 SQL 拼接（`%s` 进 SQL 字符串的仅剩已校验标识符与用户自有 querySQL 的子查询包装）
3. `Connection.Execute` 带 args 的路径有测试覆盖（bun_builder args 收集 + mysql 驱动透传）
4. API 响应中无 `password` 字段；DB 中 datasource 密码 `v1:` 前缀、share 密码 `$2` 前缀
5. `backend/migrations/` 有版本化迁移，`CreateTables` 不存在
6. requestID 非全零；DSN 配置后 Sentry 事件可见；CORS 按配置回 origin

## 附录：Batch 1 对应的评审结论（关键证据位置）

- 参数化死通道：`query/executor.go:83` 调 `conn.Execute(ctx, sql)`，args 无处可传（`datasource/driver.go:76`）
- 假 JSON 解析器：`query/bun_builder.go:379-394`
- 标识符黑名单过弱：`query/bun_builder.go:369-377` vs 白名单 `datasource/driver.go:97`
- service 裸拼接：`service/datasource/impl.go:216-233`（fieldName/querySQL）、`service/chart/impl.go:114-149`、`service/dataset/impl.go:207,249`
- requestID 全零：`cmd/main.go:117`；Sentry 死配置：`internal/config/config.go` 无 Sentry 字段
- 明文密码：`entity/datasource.go:12`、`entity/share.go:8`、`service/share/impl.go:97`
- DDL 藏于 model：`internal/model/model.go:11-123`、散落 ALTER `model.go:96-102`、`database.go:46-52`
- NewDriver `(nil,nil)`：`datasource/driver.go:90-92`
