# Batch 3 改造计划：backlog 收口 + 契约消费谨慎推进

日期：2026-09-12 ｜ 分支：refactor/batch3-backlog（本地 master=42973ae 已含 Batch 2）
前提：Batch 2 已完成并合入本地 master（待推送 origin）。本批消化全分支评审与 grilling 记录中主动延后的项。

## 侦察修正（先于一切计划：2026-09-12 实测，推翻 2 条 backlog 旧假设）

| 旧假设（todo.md §七 / 收口记录） | 实测真相 | 影响 |
|---|---|---|
| `ChartQueryRequest.config` 是"幽灵字段"，后端静默丢弃 | 后端 `query/types.go:89` **有** `Config *ChartRequestConfig` 且 `executor.go:77` 消费 `QueryOptions.MergeOtherBelowRatio`；但 **`entity.ChartQueryRequest` 无 `config`、`chartQueryIn` 镜像也不含** → 前端 `ChartBuilder.tsx:955/1075/1129` 三处发送的 `config.query_options.pie_merge_other_below_ratio` 被 handler 层丢弃；且 `GetChartData`（ShareView 链路，impl.go:149）根本不聚合、直接 `WrapPreviewSQL(source,100)` 取原始行 | 不是"幽灵"，是**半接线断链**：query 层能力存在但被 handler 镜像/服务路径两处截断，且预览（Query）与分享取数（GetData）行为不一致 |
| `DatasetColumn.typeConfig`(camel)↔生成 `type_config`(snake) 替换会波及 4 页面 | 前端**零处**读写 `typeConfig`/`type_config`（grep 实测无命中）——后端 JSON 返回 snake_case、前端类型声明 camelCase，字段实际上**从未被消费** | 替换风险重估：改名/换生成类型不会破坏现有 UI；但需顺手确认是否"声明了但没人用"的死字段 |
| ghost route `PUT /api/datasets/:id` 需补端点 | `service/dataset/impl.go:98` **已有 `Update` 方法**，只是 handler/cmd/routes.go 未注册 + `datasetsApi.update`（store/DatasetEdit 两个活调用方）打到 404；载荷 `DatasetFormData.tags`/`shard_keys` 数组 vs create 约定 JSON 字符串也需对齐 | 工作量小得多：handler + 路由 + 载荷对齐 + openapi 补契约 + api-gen，无新业务逻辑 |

`/datasets/new` 经浏览器 E2E 实测：React Router v6 静态段优先命中 DatasetEdit，其 new 模式设计即重定向回列表（经 modal 创建），**确认非 bug，移出待办**。

## 任务清单（按依赖与风险排序）

### Task 1：修复 ghost route `PUT /api/datasets/:id`（真实用户可见 bug）
- 后端：`handler/dataset.go` 加 `datasetUpdateIn`（镜像 `entity.Dataset` 可编辑字段，逐字段 `form:"-"`，Tags/ShardKeys/Columns 按 create 约定收 **JSON 字符串**）+ `Update` handler（调已存在的 `service.Update`，错误映射按 datasource.go 样板，返回 `*entity.Dataset`）；`cmd/routes.go` 注册 `RegisterPutRoute(datasets, "/:id", ...)`。
- 契约：`api/openapi.yaml` 补 `PUT /api/datasets/{id}` + `DatasetUpdateRequest`；`make api-gen` 双端重生成（验证幂等）。
- 前端：`datasetsApi.update` 载荷把数组字段 `JSON.stringify` 对齐 create 路径（store.updateDataset 或 DatasetEdit 提交处，择一，与 create 同处同法）。
- TDD（AGENTS.md 硬规则）：先写**复现 404 的失败测试**（httptest 走 PUT → 断言旧路由不存在→迁移后断言新 envelope），后端 handler baseline 测试对齐 datasource/dataset 既有风格（逐字节 `assertBody`；若 path+body 则补 `InvalidIDPrefersBodyBindError` 双非法孪生 pin）；前端 store/api 测试。
- 验证：`go test -race ./...` + `pnpm test` + `tsc --noEmit`；浏览器 E2E：编辑数据集保存成功且刷新后字段留存。
- Commit：`fix(dataset): add PUT endpoint so DatasetEdit save works`

### Task 2：pie「合并其他比例」断链决策 + 修复
- **先定产品意图（开放问题，待用户拍板）**，两方案二选一：
  - **A（接线）**：`entity.ChartQueryRequest` 加 `config` 字段（或 handler 透传）→ service.Query 把 config 转进 `query.ChartQueryRequest.Config` → 预览端比率生效；同时处理 `GetChartData`（分享/预览取数）**是否也要聚合**——当前它完全不聚合返回原始行，与 Query 路径语义不一致，需明确统一（建议本批只接 Query 预览路径，GetChartData 聚合单列）。
  - **B（移除）**：若产品上暂不要该功能，删除前端三处 `config` 发送 + UI 控件（ChartBuilder:566 输入框 + store chartQueryOptions + v1 schema `queryOptions` 保留为文档字段但停止发送），避免"有控件无效果"的误导。
- 无论 A/B：openapi.yaml `ChartQueryRequest` 的 config 注释按真相修正（删掉"entity 层不含、整体静默忽略"的错误描述）。
- 验证：A 用后端测试证明比率参数抵达 `PieProcessor`；B 用前端测试断言请求体不含 config + UI 无控件；浏览器 E2E 复核饼图预览表现与设置一致。

### Task 3：死代码清扫（低风险，可与 1/2 并行）
- `query/bun_builder.go:438` `BuildBunQuery`：master 即零调用者（评审确证），删除。
- `ShareView.tsx:84/87/120` 不可达分支：全 200 信封下拦截器 reject 裸 `new Error(msg)` 无 `.response`，401/403 门永不触发；删除死分支、保留 `has_password` 活门（先以测试证明 401/403 路径不可达再删）。
- `lib/api/client.ts:120` baseURL 尾随 `}`：确认该 `apiClient` 仅 `ApiResponse` 类型被引用后修正字符串（或直接确认无调用方并评估）。
- `ChartBuilder.tsx:941` 等 `(f as any).valueEnd`：`FilterCondition` 已声明 `valueEnd`（store:82），去 cast 改类型化访问。
- 验证：`go test -race ./...` + `pnpm test` + `tsc`；commit `refactor: remove dead code paths (BuildBunQuery, ShareView unreachable branches, client baseURL)`

### Task 4：契约消费（谨慎分批，仅零风险子集；本批不做全量替换）
- 前提审计：`DatasetColumn` 的 `typeConfig`/`type_config` 字段前端零消费（侦察已证）→ 评估是否直接删前端声明字段（含 `api/datatypes.ts` 的 `TypeConfig` 使用链，若也零消费则一并删声明）——这属"清理自己引入的谎言"而非扩面。
- 替换原则（Batch 2 评审判定的三类静默风险仍成立）：
  - **禁做**：把 `api/index.ts` 里手写裸 payload 响应类型按同名换成生成 `*Response`（信封包装不同义）。
  - **可做**：字段集完全一致、无 `as` 断言依赖、无可选性变化的裸实体类型，换 `components['schemas'][...]` 打底 + 薄手写联合层（如 `DatasetPreview`、`TableInfo`、`FieldDistribution` 这类叶子类型）。
  - 每个类型替换独立提交，`tsc --noEmit` 即守门。
- 若审计后零风险子集很小，如实记录"全量消费继续挂起"——不制造 churn 凑进度。

### Task 5：文档同步 + 收口
- `AGENTS.md`：`docs/api.md` 补 `PUT /api/datasets/:id` 端点 + dataset 契约状态；`docs/api-spec.md` 兼容协议表加一行；`docs/todo.md` §七 Batch 3 勾选 + 新遗留（如 GetChartData 聚合语义）入 Batch 4/backlog。
- 全分支评审（独立 subagent）+ 内置浏览器完整 E2E（用户硬要求）：数据集编辑保存链路、饼图比例行为按 Task 2 决策复核、数据集列表/详情回归。
- 收口 commit 后提请用户：推送 origin 顺序 = Batch 2（已合本地）+ Batch 3 一起 or 分次。

## 边界（本批不做）
- GetChartData 聚合语义统一（若 Task 2 选 A，单列 Batch 4）。
- 仪表盘/用户权限等产品功能（todo.md 阶段七/八）。
- 顺手重构与需求无关代码（AGENTS.md 零容忍）。

## 验收标准（整体）
1. DatasetEdit 编辑保存端到端可用（浏览器实测）。
2. 饼图比率"设置→效果"一致（接线或删除），无"有控件无效果"状态残留。
3. `go test -race ./...`、`pnpm test`、`tsc --noEmit`、`pnpm build:check` 全绿；评审无 Critical。
4. 死代码清单逐项归零并有测试/证据佐证。
