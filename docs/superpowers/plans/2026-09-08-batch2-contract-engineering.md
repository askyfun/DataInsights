# Batch 2：契约工程 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 以 openapi.yaml 为唯一契约源驱动双端类型生成（消除手写类型漂移），bi_chart.config schema 化 + 版本迁移（使 ShareView/Charts 能消费新结构图表），启用泛型 router 统一路由层。

**Architecture:** 手写 `openapi.yaml`（spec 先行）→ oapi-codegen 生成后端类型 + openapi-typescript 生成前端类型 → 30 个端点迁移到泛型 `RegisterRoute`（oapi-codegen 只出类型不出 stub，路由层走自研泛型包）→ config v1 schema + 前端版本迁移函数。响应契约 `{code, msg, trace, data}` 与 HTTP 恒 200 保留，由 spec 显式建模。

**Tech Stack:** OpenAPI 3.0、oapi-codegen v2（types 模式）、openapi-typescript、现有 gin + internal/router 泛型包。

**Spec:** 上游决策（grilling 确认）：OpenAPI spec 先行、启用泛型 router、config 立即 schema 化；契约演进草案 `docs/api-spec.md` §7（ChartSpec/QuerySpec 双协议）、`docs/chart-builder-plan.md` §1.5。Batch 1 落地状态见 `docs/superpowers/plans/2026-09-06-batch1-security-query-consolidation.md`。

## Global Constraints

- 响应契约 `{code, msg, trace, data}` 不变；HTTP 恒 200（运维端点除外）；错误码语义按 `response.go:13-22` 常量
- 禁止 `as any` / `@ts-ignore`；Go 错误必须包装；bug 修复先失败测试
- 每任务验收：`go test -race ./...` + `pnpm test`（前端涉及任务）+ `pnpm build:check`
- spec 是唯一权威：实现与 spec 冲突时改实现；spec 与既有行为冲突时先在任务内裁定并记录，不允许默默偏离
- 生成代码文件头带生成标记，不手改生成物；再生成不产生语义 diff
- 不做认证/RBAC；不改 SQL 生成链路（Batch 1 已收敛）

## 与 Batch 1 遗留项的勾稽

- E2E 发现的 ShareView 读旧配置（xAxisField）→ Task 6/7 解决
- `field-N` 不稳定 id → Task 6（config v1 引入稳定 fieldId）
- 前端读 `error.response.data.message` 但后端是 `msg` → Task 3（契约显式建模 + 前端修正）
- `/datasets/new` 路由被 `:id` 吞 → Task 9 顺带修
- Batch 1 minors：dialect.go 死路径删除、aggExprPattern 收紧 → Task 10
- Batch 1 遗留「新图表 ShareView 无法渲染」的根因在 Task 7 闭环

---

### Task 1: openapi.yaml 骨架 + 工具链

**Files:**
- Create: `api/openapi.yaml`（仓库根 api/ 目录，双端共享）
- Create: `backend/tools.go`（tools 依赖模式）+ `backend/go.mod`（oapi-codegen tool 依赖）
- Modify: `Makefile`（加 `api-gen` 目标）
- Modify: `frontend/package.json`（openapi-typescript devDep + `api:gen` script）
- Test: 生成产物可编译

**Interfaces:**
- Produces: `make api-gen` 一键双端生成；`api/openapi.yaml` 含 info/servers/tags/响应 Envelope components（`Envelope`、错误码枚举）与 health 端点作为样例。

- [ ] **Step 1: 建 api/openapi.yaml 骨架**（Envelope schema：code 枚举 20000/20100/…/50000、msg、trace、data；HTTP 200 恒定说明写入 description）
- [ ] **Step 2: 引入工具链**——backend `go get github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen`（tools.go + go:build tools）；frontend `pnpm add -D openapi-typescript`
- [ ] **Step 3: oapi-codegen 配置**——`api/gen/backend.cfg.yaml`：`generate: models` 模式，output `backend/internal/idls/gen_types.go`，package `idls`（复用已删除目录名，作为纯生成类型包）
- [ ] **Step 4: openapi-typescript 配置**——output `frontend/src/idls/gen_types.ts`（json 参数化模板）
- [ ] **Step 5: Makefile + package.json scripts**；跑 `make api-gen` 确认双端产物生成且编译通过（`go build ./...` + `tsc --noEmit`）
- [ ] **Step 6: Commit** `feat(api): openapi skeleton and codegen toolchain`

### Task 2: 数据源/数据集端点契约化

**Files:**
- Modify: `api/openapi.yaml`
- Test: `backend/internal/idls/` 生成物被 handler 引用编译

**Interfaces:**
- Produces: Datasource/Dataset 全部 19 个端点的 path+schema；生成类型 `idls.Datasource`/`idls.Dataset`/`idls.DatasetColumn` 等；password 字段仅出现在请求 schema（响应侧 `has_password` 模式对 share；datasource 响应无 password）
- 错误契约统一建模：`msg` 字段名写入 spec（同时记录前端读 `message` 是 bug，Task 8 修）

- [ ] **Step 1: 写 datasources/datasets 的 paths 与 components**（参数化 path：limit/offset 默认 100/0 上限 500，与 helpers.go 对齐）
- [ ] **Step 2: make api-gen + 双端编译 + Commit** `feat(api): datasource and dataset contracts`

### Task 3: 图表/分享端点契约化 + ChartQueryRequest 对齐

**Files:**
- Modify: `api/openapi.yaml`
- Modify: `backend/internal/query/chart_spec.go`（如需对齐命名）

**Interfaces:**
- Produces: charts 9 端点 + shares 4 端点 + share view 的 schema；`ChartQueryRequest`（旧协议）字段与 `entity.ChartQueryRequest` 逐一核对（含 filter.value_end）；`ChartSpec/QuerySpec` 以 components 形式写入（为 Task 7 的 chart_spec 协议铺路）
- 分页统一 `limit/offset`（图表查询既有 page/page_size 在 spec 中标记 deprecated，实现暂不变——避免行为变更，迁移在 Batch 3）

- [ ] **Step 1: charts/shares paths + components（含 ChartSpec/QuerySpec schema 首版）**
- [ ] **Step 2: 双端生成物编译 + 与 entity 类型的字段级核对清单写入 spec 注释 + Commit**

### Task 4: 泛型 router 启用——datasource handler 迁移（样板批次）

**Files:**
- Modify: `backend/internal/handler/datasource.go`、`cmd/routes.go`
- Test: `backend/internal/handler/datasource_test.go`（迁移既有测试到新签名）

**Interfaces:**
- Consumes: `router.RegisterRoute[In,Out]`、`router.BusinessError`
- Produces: 迁移模式样板——In struct（query form tag + path param 显式提取）、错误→BusinessError 转换表；后续任务照抄
- List 端点的 limit/offset 默认值逻辑移入 In 的绑定后处理（保持行为）

- [ ] **Step 1: 迁 Get（含 path param + NotFound→BusinessError）→ 测试绿 → 作为样板记录到 handler 包 doc 注释**
- [ ] **Step 2: 迁其余 10 个 datasource 端点（分 2-3 个 commit，每个保持测试绿）**
- [ ] **Step 3: routes.go 改用泛型注册；删除对应手写路由；`go test -race ./...` 全绿 + Commit**

### Task 5: 泛型 router 启用——dataset handler 迁移

**Files:** 同 Task 4 模式，`internal/handler/dataset.go` 8 个端点。
- [ ] 照样板迁移 + `:id/columns` 双方法路由 + Commit

### Task 6: 泛型 router 启用——chart/share handler 迁移

**Files:** `internal/handler/chart.go`、`internal/handler/share.go`、`cmd/routes.go`
**Interfaces:**
- Produces: 30 个端点全部走泛型注册；`response.Success/Error` 手写包装从 handler 中消失（由 router 统一）；删除 `router.hasQueryFields` 死代码
- [ ] chart 9 端点（注意 Create 的 config 默认 "{}" 逻辑保留在 handler）+ share 4 端点 + share view + health
- [ ] `grep -rn "response.Success" backend/internal/handler/` 应为零命中（全由 router 出）+ `go test -race ./...` + Commit `refactor(router): migrate all handlers to generic registration`

### Task 7: bi_chart.config v1 schema + 前端迁移函数

**Files:**
- Create: `frontend/src/lib/chartConfigSchema.ts`（v1 zod-free 手写校验，避免新依赖；或经用户同意引入 zod）
- Modify: `frontend/src/pages/ChartBuilder.tsx`（保存写 v1 结构；加载走 migrateChartConfig）
- Modify: `frontend/src/pages/ShareView.tsx`、`frontend/src/pages/Charts.tsx`（读新结构）
- Test: `frontend/src/__tests__/lib/chartConfigSchema.test.ts`

**Interfaces:**
- Produces:
  ```ts
  // ChartConfigDocument v1
  {
    version: 1,
    chartType: 'bar'|'line'|'pie'|'area'|'scatter'|'table'|'pivot',
    title: string,
    query: { dimensionGroups: Group[], metricGroups: Group[], filters, sort, limit },  // Group.fields 为稳定 fieldId（列名）
    fieldMeta: Record<fieldId, { label?, aggregation?, alias?, unit?, format? }>,     // 收敛 6 个平铺 Record
    style: ChartStyleConfig,
    queryOptions: ChartQueryOptions,
  }
  migrateChartConfig(raw: string, fallbackType): ChartConfigDocument
  ```
  旧结构（无 version 或 version<1）：按既有字段集识别并迁移（queryConfig→query；6 个 Record→fieldMeta；xAxisField/yAxisFields 丢弃）；解析失败回退空文档 + chart_type
- fieldId 改为列名（稳定），替换 `field-${index}`（store/index.ts:422）

- [ ] **Step 1: schema + migrateChartConfig 失败测试（旧结构迁移用例、损坏 JSON 用例、ShareView 消费用例）**
- [ ] **Step 2: 实现 schema/迁移函数**
- [ ] **Step 3: ChartBuilder 保存/加载切换到 v1；ShareView/Charts 改读新结构（ShareView 从 query.dimensionGroups/metricGroups 取轴字段）**
- [ ] **Step 4: store fieldId 改列名（`field-${index}` → 列名），回归 ChartBuilder 全部测试**
- [ ] **Step 5: `pnpm test` + `pnpm build:check` + Commit `feat(chart-config): versioned v1 schema with migration`**

### Task 8: 前端类型消费生成物 + 错误字段修正

**Files:**
- Modify: `frontend/src/api/index.ts`（手写类型换 `import type { components } from '../idls/gen_types'`）
- Delete: `frontend/src/idls/chart.ts`、`dataset.ts`、`datasource.ts`、`share.ts`（手写死层）
- Modify: `frontend/src/__tests__/chartQuery.test.ts`（import 路径）、`ShareView.tsx` 等错误读取处（`message`→`msg`）

**Interfaces:**
- Consumes: Task 2/3 生成的 `gen_types.ts`
- Produces: api/index.ts 只保留 API client 函数，类型全部来自生成物；`error.response.data.msg` 修正

- [ ] **Step 1: 替换类型 import + 删手写 idls + 修 msg 读取**
- [ ] **Step 2: `pnpm test` + `tsc --noEmit` + Commit `refactor(frontend): consume generated api types; fix error msg field`**

### Task 9: 后端 handler 消费生成类型 + 死代码清扫

**Files:**
- Modify: 各 handler 的 request struct（换 `idls.*` 生成类型，保留绑定逻辑）
- Delete: Batch 1 minors——`internal/query/dialect.go` 的 `BuildQueryString`/`baseSQLBuilder` 死路径、收紧 `aggExprPattern` 函数名为显式聚合列表
- Modify: `frontend/src/App.tsx` 或路由配置修 `/datasets/new` 顺序

**Interfaces:**
- Produces: handler 入参类型与 spec 同源；全仓 `grep ChartQueryRequest` 后端仅 entity/query 包定义一处

- [ ] **Step 1: handler 类型替换（分 handler 提交）+ 死路径删除（TDD：先补“粒度报错不被死路径绕过”的现有测试确认）**
- [ ] **Step 2: `/datasets/new` 路由顺序修复（先于 `:id` 匹配）+ 前端测试**
- [ ] **Step 3: 全量测试 + Commit**

### Task 10: 文档同步 + Batch 3 路线确认

**Files:**
- Modify: `AGENTS.md`（契约工作流：改接口先改 openapi.yaml → make api-gen → 双端消费生成物；router 已启用）
- Modify: `docs/api-spec.md`（§7 现状更新：chart_spec 双协议仍未对外暴露，标 Batch 3）、`docs/architecture.md`
- Modify: `MEMORY.md`（经验追加）

**Interfaces:** 无代码接口。
- [ ] 按落地状态更新 + Commit `docs: sync with batch2 contract engineering`

## 验收标准（Batch 2 完成的定义）

1. `api/openapi.yaml` 覆盖全部 30 端点；`make api-gen` 可重复生成且无语义 diff
2. 双端类型零手写副本（前端 idls 手写层删除；后端 handler 入参来自生成物）
3. 30 个端点全部经泛型 router 注册，handler 内无 `response.Success/Error` 手写包装
4. 新保存的图表 config 带 `version: 1`；ShareView 能渲染新结构图表（E2E 验证——修正 Batch 1 发现的缺口）；旧图表经 migrateChartConfig 正常加载
5. `go test -race ./...` + `pnpm test` 全绿（前端仅剩 master 既有 DatasourceDetail 4 失败）
6. AGENTS.md 记录契约工作流

## 明确不做（Batch 2 范围外）

- chart_spec 对外双协议暴露（`POST /api/charts/query` 接受 chart_spec 字段）——Batch 3，随前端 adapter 重构一起
- store 切片 / ChartBuilder 拆分 / i18n——Batch 3
- 认证、分页行为变更（page/page_size 的运行时迁移）
