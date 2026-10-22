# Data Insights API 接口文档

> 最后更新：2026-10-17

> **契约事实源是 [`api/openapi.yaml`](../../api/openapi.yaml)**（OpenAPI 3.0.3）。本文是它的叙述性视图：
> 端点清单、通用约定与易踩的契约细节。两者不一致时**以 yaml 为准**，并应回头修正本文。
> 双端类型由 `make api-gen` 从 yaml 生成（`backend/internal/idls/gen_types.go`、
> `frontend/src/idls/gen_types.ts`；前端那半步当前因 openapi-typescript 与 typescript@7
> 不兼容而必然失败，属已知问题）。
>
> 本文只描述**已落地契约**，不再保留"目标契约草案"类章节；图表查询链路的完整设计见
> 同目录的 `chart-query-design.md`，系统架构与关键设计决策见 `architecture.md`。

## 1. 通用规范

### 1.1 端点清单（58 个泛型路由端点 + 4 个手工路由）

| 领域 | 端点数 | 前缀 |
|------|--------|------|
| Datasource 数据源 | 11 | `/api/datasources` |
| Dataset 数据集 | 9 | `/api/datasets` |
| Chart 图表 | 8 | `/api/charts` |
| Dashboard 仪表盘 | 6 | `/api/dashboards` |
| DashboardFolder 归档文件夹 | 5 | `/api/dashboard-folders` |
| Alert 指标预警 | 6 | `/api/alerts` |
| SampleDataset 示例数据集 | 4 | `/api/sample-dataset` |
| Auth 认证 | 4 | `/api/auth` |
| Token PAT 管理 | 3 | `/api/tokens` |
| QueryRecord 查询记录 | 2 | `/api/queries` |
| Health 健康检查 | — | `/health` |

所有 `/api` 路由经泛型路由 `router.RegisterXxxRoute[In, Out]` 注册（见
`backend/cmd/routes.go`）。**4 个手工路由**不走泛型注册的统一绑定路径：

- `GET /health`：在 `cmd/main.go` 用裸 `r.GET` 注册，但响应仍是标准 Envelope；
- `POST /api/datasets/import` 与 `POST /api/datasets/:id/replace`：本地文件上传，请求体是
  `multipart/form-data`，与泛型路由对 POST 一律 `ShouldBindJSON` 的语义不兼容，故循 `/health`
  先例以裸 `*gin.Context` handler 注册（见 `handler/dataset.go` 的 package 注释）；
- `POST /api/charts/export`：图表数据导出（issue #120），成功时响应体是 CSV/XLSX 文件流而不是
  JSON 信封，故同样手工注册（见 `handler/chart.go` 的 `Export`）。

### 1.2 统一响应信封（Envelope）

HTTP 状态码恒为 200，业务结果由响应体 Envelope 表达：

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "xxxxxxxx",
  "data": {}
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| code | int | 业务状态码，见 1.3 |
| msg | string | 成功为 `"success"`，错误为可读描述（不保证稳定，勿用于程序判断） |
| trace | string | 请求追踪 ID，回显 `X-Request-ID` |
| data | object / array | 业务负载；**错误或无数据时归一化为 `{}` / `[]`，绝不为 null** |

集合字段红线：数组无数据返回 `[]`、对象无数据返回 `{}`，禁止 `null`。

### 1.3 业务状态码

与 `backend/internal/response/response.go` 常量一一对应：

| code | 常量 | 含义 | 建议用户提示 |
|------|------|------|--------------|
| 20000 | CodeSuccess | 成功 | — |
| 20100 | CodeBadRequest | 请求参数错误（含绑定失败、非法 id/短码） | 请求参数有误，请检查输入 |
| 20200 | CodeUnauthorized | 认证/授权错误（Bearer 校验失败、口令错误、注册已关闭、PAT 被拒于管理端点——当前仅 auth/tokens 端点会产生，见 10/11 节） | 登录状态已失效 |
| 20300 | CodeNotFound | 资源不存在 | 请求的资源不存在 |
| 20400 | CodeBusinessError | 业务逻辑错误（如资源被引用不可删） | 操作失败，请稍后重试 |
| 20500 | CodeThirdPartyErr | 第三方服务错误 | 服务暂时不可用 |
| 50000 | CodeInternalError | 服务端内部错误（含取数执行失败） | 服务器内部错误 |

### 1.4 请求 / 响应头

| 头 | 方向 | 说明 |
|----|------|------|
| `X-Request-ID` | 请求 | 可选，客户端生成；缺省由后端 requestID 中间件生成（非全零），响应头与 Envelope.trace 原样回显 |
| `Content-Type: application/json` | 请求 | POST/PUT 请求体 |
| `Authorization: Bearer <token>` | 请求 | 仅 `GET /api/auth/me`、`DELETE /api/auth/logout`、`/api/tokens/*` 强制（渐进强制，其余端点仍匿名；令牌形态与会话/PAT 边界见 10/11 节） |

### 1.5 分页参数：两套口径，别混用

| 场景 | 参数 | 规则 |
|------|------|------|
| 列表端点（datasource / dataset / chart / dashboard 的 GET 集合） | `limit` / `offset` + `sort` / `order` / `keyword` | limit 缺省 100，≤0 或 >1000 回落 100；offset 缺省 0，负数归一为 0，越界返回空页。**响应 data 是 `{items, total}`**（issue #205）：total 是过滤后全量集合的 COUNT；sort 走字段白名单（未知值回落实体默认排序）。dataset 另收 `datasource_id` / `query_type`，dashboard 另收 `folder_id` |
| 表数据分页（`GET /api/datasources/{id}/tables/{table}/data`） | `page` / `page_size` | page 从 1 起，≤0 归一为 1；page_size 缺省 20，上限 100 |
| 图表查询分页（`pagination` 对象） | `page` / `page_size` | 仅 `chart_type=table` 分支消费；契约上已声明向 limit/offset 收敛（Batch 3），当前实现仍以 page/page_size 为准 |

### 1.6 字段命名与"JSON 字符串"惯例

- 全链路 JSON 键为 **snake_case**，前后端字段名完全一致（不存在 camelCase 映射层）。
  唯一例外：仪表盘 `layout_json` **内部**的布局文档（DashboardLayout）键是 camelCase
  （`widgetId`/`chartId`/…），那是前端自有文档格式，后端只透传。
- **JSON 字符串惯例**：以下字段在 API 面上是 *字符串*（后端以 string 存/回显），不是 JSON 对象/数组本体：
  `dataset.tags / columns / quality_rules / shard_keys`、`chart.config`、`dashboard.layout_json`。
  其中 `layout_json` 列是 JSONB，读接口返回的是 PostgreSQL 规范化形态——**不要按字节比对**。

### 1.7 前端调用规范

- 全前端**唯一 axios 实例**在 `frontend/src/lib/api/client.ts`（`apiClient`），禁止在别处
  `axios.create`；`api/index.ts` 按领域导出 10 个模块（`datasourcesApi` / `datasetsApi` /
  `chartsApi` / `dashboardsApi` / `dashboardFoldersApi` / `alertsApi` / `sampleDatasetApi` /
  `authApi` / `tokensApi` / `queriesApi`），页面/store 只消费这些模块。
- 响应拦截器"非 20000 即错"：`console.error` + reject，**不弹 toast**（提示文案由调用方决定，
  统一弹窗会导致重复提示）。
- 请求拦截器自动注入 `X-Request-ID`（`crypto.randomUUID` 不可用时退回时间戳方案）。
- 类型：实体/请求/响应类型 alias 到 `idls/gen_types.ts` 生成物（`ApiResponse<T>` 即 Envelope
  去信封），生成物禁止手改。

## 2. Health

### GET /health

健康检查。恒 HTTP 200 + Envelope，`data = { "status": "ok" }`。
注意：当前实现**不探库**——只证明进程活着，不代表 `DATABASE_URL` 可连。

## 3. Datasource 数据源（11 个端点）

响应实体 `Datasource`：`id / name / type / host / port / database_name / username / created_at / updated_at`。

- `password` 为 `json:"-"`，**任何响应都不回显**；仅请求侧携带，落库前 AES-GCM 加密
  （密钥来自 `SECURITY_KEY`，留空时首启自动生成并持久化到 `bi_setting`）。
- `type` 后端不做枚举强校验；驱动工厂支持 `postgresql / mysql / clickhouse / starrocks`；
  创建/测试时 type 为空缺省 `"postgresql"`。

| # | 方法与路径 | 说明 |
|---|-----------|------|
| 1 | `GET /api/datasources` | 列表（`{items,total}`；limit/offset/sort/order/keyword，见 1.5） |
| 2 | `POST /api/datasources` | 创建。请求体必填 name/type/host/port/database_name/username/password |
| 3 | `POST /api/datasources/test` | 用未落库参数试连；失败/驱动不支持 → 20100。成功 `data = {status: "ok"}` |
| 4 | `GET /api/datasources/{id}` | 详情；id 非法 → 20100，不存在 → 20300 |
| 5 | `PUT /api/datasources/{id}` | 更新；**password 缺省/空串 = 保留已存密码**（"未提供则保留"约定） |
| 6 | `DELETE /api/datasources/{id}` | 删除，`data = {status: "ok"}` |
| 7 | `GET /api/datasources/{id}/tables` | 实时读目标库表元数据，`data` 为 `TableInfo[]`（`name` / `comment`） |
| 8 | `GET /api/datasources/{id}/tables/{table}/columns` | 列元数据，`data` 为 `ColumnInfo[]`（`name` / `data_type` / `comment` 三键）。表名须为合法标识符，URL 编码传输 |
| 9 | `GET /api/datasources/{id}/tables/{table}/data` | 分页读表数据（`page`/`page_size`/`sort_field`/`sort_order`）。**`sort_field` 必须是主键列之一**，否则 50000；`sort_order` 大小写不敏感匹配 desc，其余归一为 ASC。响应 `TableDataResult`：`columns / data / total / primary_keys / page / page_size` |
| 10 | `POST /api/datasources/{id}/preview` | 预览表或 SQL 结果，固定前 10 行。请求体 `query_type: "table" | "sql"` 决定用 `table_name` 还是 `query_sql`（二选一，后端不强校验）。响应 `PreviewResult`：`columns / data` |
| 11 | `POST /api/datasources/{id}/field-distribution` | 字段值分布。`field_name` 必填（缺失 20100）；`limit` ≤0 或 >50 归一为 20。响应 `FieldDistribution`：`field_name / total_count / unique_count / distribution[]`，每项 `{value, count, percentage}`（percentage 两位小数） |

## 4. Dataset 数据集（9 个端点）

响应实体 `Dataset` 的关键契约：

- `query_type: "table" | "sql"`：决定数据源是 `table_name` 还是 `query_sql`（指针字段，
  键恒在、值可为 null）；创建时为空后端缺省 `"table"`。
- `mode`：后端语义当前恒 `"direct"`（前端类型里有 `"accelerated"`，后端不产出）。
- `tags / columns / quality_rules / shard_keys`：**JSON 数组的字符串形态**（见 1.6），缺省 `"[]"`。

### DatasetColumn（列定义，`GET/POST /api/datasets/{id}/columns` 的元素）

| 字段 | 契约 |
|------|------|
| `id` | 列的**稳定标识**（8 位 base36 短 ID，后端分配后不变）。图表配置、shard_keys、过滤条件一律引用 id，**不是列名**；未落库的列在首次读取时分配并物化 |
| `name` | 可变展示名，同一数据集内唯一（PUT /columns 校验） |
| `expr` | SQL 表达式（物理列即来源表列名）；改展示名不动 expr |
| `type` | 8 类规范词表：`float / integer / boolean / string / date / datetime / array / map`（历史词 number/json/unknown 后端已归一） |
| `type_config` | `{precision, scale}`（注意是 snake_case 的 `type_config`，不存在 camelCase `typeConfig`） |
| `comment` / `role` | role 为 `"dimension" | "metric"` |

| # | 方法与路径 | 说明 |
|---|-----------|------|
| 1 | `GET /api/datasets` | 列表（`{items,total}`；另收 `datasource_id` / `query_type`） |
| 2 | `POST /api/datasets` | 创建。必填 name/datasource_id/query_type；空串可选字段落库为 null |
| 3 | `GET /api/datasets/{id}` | 详情 |
| 4 | `PUT /api/datasets/{id}` | 更新（Batch 3 补齐，修复前端编辑保存 404 的 ghost route）。请求体与创建同构；**取回并保留**：name/datasource_id/query_type 恒覆盖，未传/空串的 tags/columns/shard_keys/table_name/query_sql/description 保留存量（显式 `"[]"` 仍可清空），quality_rules/preview_data/时间戳恒保留。未知 id → 20300 |
| 5 | `DELETE /api/datasets/{id}` | 删除 |
| 6 | `GET /api/datasets/{id}/columns` | 列定义（columns JSON 解析后的 `DatasetColumn[]`） |
| 7 | `POST /api/datasets/{id}/columns` | 全量替换列定义——请求体是 **DatasetColumn 裸数组**，不是 `{columns: [...]}` 包装；成功 data 为更新后的 Dataset |
| 8 | `GET /api/datasets/{id}/preview` | 按数据集源实时预览（固定前 10 行），`PreviewResult` |
| 9 | `POST /api/datasets/{id}/query` | 按 `QueryConfig`（dimension_groups/metric_groups/filters/sort/limit）取数，data 为结果行数组（`DataRow`：列名 → 任意 JSON 值） |

## 5. Chart 图表（8 个端点）

响应实体 `Chart`：`id / name / dataset_id / chart_type / config / created_at / updated_at`。
`config` 是**图表配置文档的 JSON 字符串**（结构见 `frontend/src/lib/chartConfigSchema.ts` 的
`ChartConfigDocument`，当前 version:2；后端只存取不解释）；缺省/空串后端写 `"{}"`。
`chart_type` 后端不做枚举校验；查询层已知取值 `table / bar / line / area / pie / scatter / pivot /
histogram / boxplot`（未知值按 axis 处理器回退）。

| # | 方法与路径 | 说明 |
|---|-----------|------|
| 1 | `GET /api/charts` | 列表（`{items,total}`；limit/offset/sort/order/keyword） |
| 2 | `POST /api/charts` | 创建（name/dataset_id/chart_type 必填；无效 dataset_id 要到取数时才暴露） |
| 3 | `GET /api/charts/{id}` | 详情 |
| 4 | `PUT /api/charts/{id}` | 更新，**全量覆盖**：未提供字段以零值写库（与 dataset/datasource 的"未提供则保留"不同！） |
| 5 | `DELETE /api/charts/{id}` | 删除（不阻断，引用提示见 #8） |
| 6 | `GET /api/charts/{id}/data` | 按持久化 config 预览取数。config 为 v1+ 文档且维度/指标组非空 → 走与 `POST /api/charts/query` 相同的聚合管道（不投影 select_sql/count_sql）；旧结构/损坏/空配置 → 回退为按数据集源取前 100 行的 `DataRow[]`。图表/数据集不存在或取数失败 → **50000**（不区分 404） |
| 7 | `POST /api/charts/query` | 图表语义查询，v1/v2 双协议见 5.1 |
| 8 | `GET /api/charts/{id}/references` | 被多少个未软删仪表盘引用：`data = {count, dashboards: [{id, name}]}`。实现由 dashboard handler 提供（扫 bi_dashboard 的 layout_json，jsonb `@?` + jsonpath，非文本子串匹配），路由归属 chart 资源。**提示不阻断删除** |

### 5.1 `POST /api/charts/query` 协议（v1 平铺 / v2 槽位）

请求体 schema 为 `ChartSpecQueryRequest`（同一结构承载两种协议，按 `spec_version` 判别）：

- **v1**（`spec_version` 缺失或 ≠2）：消费平铺 `dims: string[]` + `metrics: [{field, agg, alias?}]`。
- **v2**（`spec_version: 2`）：消费 `dimension_groups` / `metric_groups` 槽位组
  （组含 `name`（如 `x_axis` / `category` / `values` / `rows`）、`fields`；字段绑定携带
  `field`（**列 id，非展示名**）、`agg`、`granularity`、`binding_id` 等，槽位名与 binding_id
  保留进查询 AST）。
- `filters` / `pagination` / `sort` 两协议共用。`filters` 元素键为 `operator`（**不是 op**）：
  `{id, field, operator, value, value_end?, logic}`。
- `query_options`：histogram 消费 `bin_count`（默认 20）/ `bin_width`；table 消费
  `show_total`（true 时响应 `data.total` 给出合计行，见 `ChartTableResponse.total`）。
  executor 直接从请求读取，不进 QuerySpec。
- `query_options`：扩展袋。histogram 消费 `bin_count`（默认 20）/ `bin_width`；
  Top N（#130，bar/line/area/pie）消费 `top_n: {limit, metric?, order?}` ——
  executor 翻译进查询计划（AST 的 Sort + Limit，数据库完成排序截断）。
  同环比（#129，bar/line/area/table）消费 `comparison: {type: mom|yoy, field?: 列ID}` ——
  executor 追加一条「窗口平移基线查询」并并入 `(上期)`/`(增长率%)` 系列/列。
  均从请求直接读取，不进 QuerySpec。

响应 `data` 为 `ChartDataResult`：`{ data, select_sql, count_sql? }`——`select_sql` 成功恒返回；
`count_sql` 仅 table + pagination 分支。内层 `data` 形状由 `chart_type` 判别（契约上 oneOf，
生成物为无类型 schema，消费方按 chart_type 手动判别）：

| chart_type | 形状 | 结构 |
|------------|------|------|
| table | `ChartTableResponse` | `columns / data(DataRow[]) / pagination{page, page_size, total, total_pages}`；columns 顺序仅 pagination 分支保证"维度在前、指标别名在后" |
| pie | `ChartPieResponse` | `data: [{name, value, percentage}]`；长尾超 20 项截断合并为"其他"（饼图比例阈值合并能力在 query 层保留，但**未接线**到任何请求协议） |
| bar / line / area / 未知 | `ChartAxisResponse` | `x_axis: string[]`、`series: [{name, data}]`，data 与 x_axis 等长、空洞为 null |
| scatter | `ChartScatterResponse` | `data: [[x, y], ...]`，每点二元数组 |
| pivot | `ChartPivotResponse` | `columns / data`（当前实现同 table 不带分页） |
| histogram | `ChartHistogramResponse` | `bins: [{bin_start, bin_end, count}]`；区间半开 `[start, end)`、相邻首尾相接、最大值归末 bin；空数据集 `bins: []` |

> openapi.yaml 中还有一组**仅声明未接线**的响应 schema（`ChartPivotResponseV2`、
> `ChartBoxplotResponse`、`ChartRadarResponse`、`ChartKpiResponse`），为后续图表任务钉死字段名，
> 当前无任何 processor 返回，消费方不应依赖。

## 6. Dashboard 仪表盘（6 个端点）

响应实体 `Dashboard`：`id / name / description / layout_json / status / created_at / updated_at`。

- `id` 为后端生成的 **UUIDv7 字符串**（非自增）；非 UUID → 20100，不存在/已软删 → 20300。
- `layout_json` 是 `{"version":2,"pages":[...],"widgets":[...]}` 文档的字符串形态（见 1.6 的 JSONB 警告）。
  文档结构（DashboardLayout）**归前端所有**（`frontend/src/lib/dashboardLayoutSchema.ts`），
  后端只做防御性投影读取。**v2 = 多页面**：`pages` 至少一页，每个 widget 用 `pageId` 归属一页
  （扁平表达，不嵌套）；v1 文档（无 `pages`）在加载时**无损迁移**到 v2。widget 按 `type` 判别：
  `chart`（存 `chartId` 引用，非快照）/ `text`（markdown，渲染前必须 sanitize）/
  `filter`（盘级筛选器，`binding` 是 `(datasetId, column)` 二元组，**column 存列 id 非列名**；
  `scope` 缺省只作用所在页、`all` 作用所有页）。`widgetId` **整盘**唯一且 ≠ chartId。
- `status`：v1 只会出现 `draft`（`published` 尚未启用），后端不做枚举校验。

| # | 方法与路径 | 说明 |
|---|-----------|------|
| 1 | `GET /api/dashboards` | 列表：未软删，默认 `created_at DESC, id DESC`；`{items,total}`，另收 `folder_id`（直属子项） |
| 2 | `POST /api/dashboards` | 创建（请求体无 id）；name 必填（仅列宽约束），layout_json 非空但非法 JSON → 20100 |
| 3 | `GET /api/dashboards/{id}` | 详情 |
| 4 | `PUT /api/dashboards/{id}` | 更新：五字段**全部可选且"未提供则保留"**；description 空串清空为 NULL；`folder_id` 三态（缺省/null 保留、`""` 移出文件夹、UUID 归档进该夹）；updated_at 后端显式前进 |
| 5 | `DELETE /api/dashboards/{id}` | **软删**（打 `deleted_at`，幂等，绝不物理删除；无级联目标） |
| 6 | `POST /api/dashboards/{id}/query` | 盘级批量取数，见 6.1 |

### 6.1 `POST /api/dashboards/{id}/query` 合并语义

请求体只下筛选器**当前取值**与（可选）当前页：`{page_id, filters: [{widgetId, value[]}]}`。
前端不解析 chart config、不下发合并结果；合并在后端单点完成（运行期覆盖 + 追加，**绝不写回 bi_chart.config**）。

- `page_id` 非空时**只取该页的图表块**（多页面盘不必把没在看的页也跑一遍 SQL），并额外纳入
  任意页里 `scope: all` 的筛选器；**缺 `pageId` 的块不受按页收窄影响**（那是 v1 旧文档，排除它们
  会让旧盘直接空白）。省略或空串 = 不按页收窄（单页语义，旧客户端的兼容底线）。
- 请求里没出现的筛选器 = 未激活：不回落 layout 的 `defaultValue`，不参与合并不触发覆盖。
- 适用条件按 `(binding.datasetId, binding.column)` 命中该图表的数据集（避免跨数据集同名误伤）。
- 取值形状按算子分流：`in/notIn` 数组、`between` 两元素、其余标量一元素、`isNull/notNull`
  靠"数组非空"表示已激活（前端 `lib/dashboardFilterValue.ts` ↔ 后端 `buildOverrides` 同构）。
- 逐块返回：单块失败不整盘失败（该块 `status=error` + `message`）；图表不存在/已软删返回
  `chart_missing` / `chart_deleted` 且**保留该 widget 的 x/y/w/h 原位**，前端渲染占位。
- 响应 `data.results[]`（顺序与 layout 图表块一致）：`widgetId / chartId / status /
  data（同 ChartDataResult，失败时 null）/ appliedFields / overriddenFields（覆盖可见标识）/ message`。
- 布局解析不出来 → 空 `results` 而不是报错。

### 6.2 仪表盘文件夹（归档树，第一期只归档仪表盘）

实体 `DashboardFolder`：`id / name / parent_id / created_at / updated_at`。表 `bi_dashboard_folder`
（migration 00007），`id` 同为 UUIDv7，`parent_id` 自引用表达树、**无 FK**（软删语义下外键会
拒绝合法历史态）。

| # | 方法与路径 | 说明 |
|---|-----------|------|
| 1 | `GET /api/dashboard-folders` | **扁平**全量（`created_at ASC, id ASC`，不分页）：组树归前端 `lib/dashboardFolderTree.ts`，后端不落嵌套也不落物化路径 |
| 2 | `POST /api/dashboard-folders` | 建夹；`parent_id` 缺省/null/`""` 都是根级；父级不存在或已软删 → 20300；name 空 → 20100 |
| 3 | `GET /api/dashboard-folders/{id}` | 详情 |
| 4 | `PUT /api/dashboard-folders/{id}` | 改名 / 移动；`parent_id` **三态**：缺省或 null 保留、`""` 移到根级、UUID 移到该夹下；移动到自己或任意后代（成环）→ 20400 |
| 5 | `DELETE /api/dashboard-folders/{id}` | 软删，**仅空夹可删**：仍有未软删子夹或仪表盘 → 20400（消息带剩余数量），不级联、不孤儿化。**不幂等**：`getFolderModel` 的存在性门控（带 `deleted_at IS NULL`）先于删除语句，重复删除已删/不存在的夹 → 20300（对比 `DELETE /api/dashboards/{id}`：无门控、命中 0 行仍成功，是真幂等） |

`""` 作为「清空归属」的显式哨兵是 JSON 无法区分「缺省」与「null」的必然产物，`dashboard.folder_id`
与 `folder.parent_id` 共用同一口径。悬空引用（指向已软删对象）**不级联清洗**：后端原样回显，
前端把这种行退化成根级展示。

## 7. QueryRecord 查询记录（2 个端点，"地址栏即分享"）

每次成功查询后由前端调用落库，返回 21 位 base58 短码（`idcodec`，非库内 uuid 原文），
复制地址栏 `?q=<短码>` 即分享。

### POST /api/queries

请求体（`QueryRecordSaveRequest`）：`dataset_id`（必填，>0）、`spec`（必填）、`chart_id?`、
`source_type?`（约定 `build` / `share_url`，≤50 字符）、`row_count?`、`duration_ms?`。

`spec` 是记录级信封 `QueryRecordSpec`：`{v: 1, document: {...}}`——外层 `v` 管记录级 schema
演进（当前仅接受 1），内层 `document` 是前端 `ChartConfigDocument`（自带 version 与迁移），
后端**只做信封校验与透传**，不解释内容。

契约细节：

- **去重**：`spec_hash = sha256("<dataset_id>\n" + 规范化 spec JSON)` 唯一约束；重复提交
  ON CONFLICT DO UPDATE，**复用原 query_id**，仅更新 row_count/duration_ms——spec、
  created_at、expires_at、hit_count 一概不动（别人重查不改变已分享链接的内容与有效期）。
- `expires_at` 恒 = created_at + 90 天，打开链接**不刷新**。
- 校验：dataset_id 缺失 → 20100；spec 非 JSON 或超 32KB → 20100；`spec.v ≠ 1` → 20100；
  `spec.document` 非 JSON 对象 → 20100；source_type 超长 → 20100。
- 成功 data 为 `QueryRecordSaved`：`{query_id, created_at, expires_at}`。

### GET /api/queries/{q}

短码还原记录（`QueryRecord`：`query_id / spec / dataset_id / chart_id / created_at /
expires_at / expired / hit_count`）。

- 短码长度/字符集非法 → 20100；合法但无此记录 → 20300。
- **已过期照常返回**（直链保活，条件 100% 还原），过期与否只看 `expired` 字段（服务端派生，
  客户端勿重复实现）。
- 每次读取 hit_count +1、刷新 last_accessed_at；该写失败只记日志不影响读取。
- `hit_count` 是**本次读取之后**的累计值（含本次 +1）。

## 8. Alert 指标预警（6 个端点）

响应实体 `AlertRule`：`id / name / chart_id / metric / operator / threshold / enabled / last_triggered_date / created_at / updated_at`。表 `bi_alert_rule` / `bi_alert_trigger`（migration 00011，**不存在独立的 `bi_alert` 表**），规则 `id` 是后端生成的 UUIDv7 字符串（非自增）。

- `operator` 词表仅 `gt / lt / eq`，其余值 20100；求值是纯数值比较（`eq` 是浮点精确相等）。
- `metric` 是**图表数据行的输出列键**（别名优先；页面下拉候选由前端解析该图表配置的指标槽位得到，见 `frontend/src/pages/Alerts.tsx`）。
- `last_triggered_date` 是最近一次触发的 UTC 日期（`YYYY-MM-DD`），同时是评估器「同 UTC 日去重」标记与前端「最近触发日」展示列；**从未触发时该键整体缺省而非 null**（实体标了 `omitempty`，yaml「null/缺省」的描述以代码为准），前端按 `-` 渲染。
- 路径 `{id}` **不做形态校验**：原样交主键查询，非法形态与不存在/已软删一样都是 20300（不像 Dashboard 另有「非 UUID → 20100」的门槛）。

| # | 方法与路径 | 说明 |
|---|-----------|------|
| 1 | `GET /api/alerts` | 全部存活规则列表（`created_at ASC, id ASC`）；**本期不分页**，data 为裸数组（空 → `[]`） |
| 2 | `POST /api/alerts` | 创建。name/metric 空 → 20100；operator 非法 → 20100；`chart_id` 须为正整数（20100）且指向**存在且未软删的图表**（否则 20300「绑定的图表不存在」——预警只作用已保存图表）。`enabled` 创建时固定 true，启停走 PUT |
| 3 | `GET /api/alerts/{id}` | 详情；不存在/已软删 → 20300 |
| 4 | `PUT /api/alerts/{id}` | 更新（全字段指针，**未提供/null 则保留存量**，行内启停开关只发 `{enabled}`）。name/metric 显式空串 → 20100；chart_id 提供时重做正整数与存活图表校验；threshold/enabled 提供即覆盖。`created_at / last_triggered_date / deleted_at` 恒不写回，updated_at 由后端前进 |
| 5 | `DELETE /api/alerts/{id}` | 软删（只打 `deleted_at`，绝不物理删；触发记录行保留）。`data = {status: "ok"}`。⚠️ 重复删除已删/不存在 id 实际返回 **20300**——service 的存在性门控先于更新语句；openapi.yaml 该端点「重复删除影响 0 行不报错」的描述与代码不符，**以代码为准** |
| 6 | `GET /api/alerts/{id}/triggers` | 触发历史（`created_at DESC`，**上限 50**，本期不分页）；规则本身须存活，否则 20300（防软删规则经此端点被遍历） |

`AlertTrigger`（只追加不改写）：`id / rule_id / metric_value / threshold / message / notified / notify_error / created_at`。`notify_error` 是家族里唯一的可空字段——`null` = 通知已送达；非 null = 未送达原因（伴随 `notified=false`），不影响触发本身成立。

### 8.1 后台评估（不是端点契约，但决定触发记录的内容）

- 评估器是进程内 goroutine（`cmd/main.go` 启动、随主进程退出），每 `ALERT_EVAL_INTERVAL` 秒扫一轮 `enabled` 且未软删的规则（默认 300；非法值回退默认并打 warning，不拦启动）。首轮评估要等**一整个间隔**过去才执行。
- 取数复用 `chart.Service.GetData`（与 Dashboard 同款窄接口 + 装配期注入），绑定图表改配置后规则天然跟随；未保存的构建器草稿不在评估范围（评估按 chart id 读**持久化 config**）。
- 仅两种取数形状参与评估：**行集 `[]map[string]any`**（逐行比较，任一行越线即触发、记录**首个**越线行数值；字符串数值可 coerce，缺列/NaN/不可解析的行跳过）与 **KPI 卡 `*query.KpiResponse`**（单值比较，此形状下 metric 键不参与取值，但字段仍必填）。其余形状（表格的包装响应、轴/饼等聚合形状）本期跳过——只记服务日志，不报错、不触发。
- **同 UTC 日去重**：`last_triggered_date` 与今天同日直接跳过（连取数都不做）；触发时「INSERT 触发记录 + UPDATE 去重标记」在同一事务成型，落库失败一起回滚、下轮重试。
- 通知只有 SMTP 邮件通道（587/STARTTLS 语义，本期不做 465 隐式 TLS，无重试队列）。`SMTP_HOST` 未配置 → 装配注入 nil Notifier：评估与落库照常，`notify_error` 写「未配置邮件通知」文案；配了 host 但 `SMTP_FROM`/收件人为空 → 按发送失败留档。通知失败**从不**影响触发落库。

## 9. SampleDataset 内置示例数据集（4 个端点）

状态实体 `SampleDatasetStatus`：`{loaded, dismissed}`（两个布尔**相互独立**——先忽略后加载、先加载后移除，都只改变其中一个）。`loaded` 的判定是「`bi_datasource` 存在 `is_sample=TRUE` 的存活行」（不是数数据集）；`dismissed` = `bi_setting` 键 `sample_dataset_dismissed` 为 `'true'`。

| # | 方法与路径 | 说明 |
|---|-----------|------|
| 1 | `GET /api/sample-dataset/status` | 引导卡片三态的来源（前端据此决定显隐与「移除示例数据」入口） |
| 2 | `POST /api/sample-dataset/load` | 一键加载，**幂等**：已加载时原样返回当前状态、不重复建行（partial 唯一索引保证至多一个存活示例数据源，migration 00012）。事务内创建隐藏示例数据源（`is_sample=TRUE`，连接信息反解自应用自身 `DATABASE_URL`，口令 AES-GCM 存储）+「示例：电商订单」「示例：商品目录」两个数据集（列类型、维度/指标角色与稳定列 id 已预置，`tags=["sample"]`）。返回加载后的状态 |
| 3 | `POST /api/sample-dataset/dismiss` | 「忽略」持久化（upsert `bi_setting`），重启仍生效；返回状态。**没有「取消忽略」端点** |
| 4 | `DELETE /api/sample-dataset` | 一键移除：事务内软删图表 → 数据集 → 示例数据源；**未加载时静默 no-op 成功**。种子表 `di_sample_*`（migration 00012 灌数）保留，再次加载直接复用。`data = {status: "ok"}` |

易踩点：

- `load / dismiss` 是泛型路由 POST：**请求体必须是合法 JSON**（前端发 `{}`），空 body 会 EOF 绑定失败 → 20100。
- 示例数据源**不出现在** `GET /api/datasources`（列表恒带 `is_sample = FALSE`，`service/datasource/impl.go`），也不能被用户建/改数据集引用（datasource/dataset 两侧守卫）——示例数据集是内部引用它。
- `load` 对 `DATABASE_URL` 只接受 `postgres / postgresql` scheme，且 host 与库名必填；不合法 → 50000（普通内部错误，非业务码）。
- 本族 4 个端点当前**匿名可调**（未接 Bearer，与第 10 节的渐进强制同一口径）。

## 10. Auth 认证（4 个端点）

令牌模型（会话与 PAT 共用 `bi_token` 表，migration 00010）：明文 = `di_session_…` / `di_pat_…` + 32 随机字节 base64url，**只在签发响应回显一次**；库内只存 sha256 摘要与非密钥的可识别前缀（标记 + 前 6 位密钥字符）。校验是**每次实时查库**（无签名、无缓存 → 撤销即时生效、无静默窗口），每次命中顺带 best-effort 刷新 `last_used_at/ip/ua`（刷新失败不会拒绝有效令牌）。`Authorization` 头的 `Bearer` scheme 大小写不敏感。

实体：`AuthResult = {token, kind: "session", expires_at, user}`（register/login 的 data）；`AuthUser = {id, username, role, created_at}`（**绝不回出口令或其哈希**；当前注册产出的 role 为 `admin`）。

| # | 方法与路径 | 说明 |
|---|-----------|------|
| 1 | `POST /api/auth/register` | 公开。**引导式注册**：仅当无存活用户时成功，该账号 `role=admin`，并在同一事务把 datasource/dataset/chart 三表里 `owner_id=0`（L0 占位归属）的存量行认到其名下；已有存活用户 → 20200「注册已关闭：系统管理员已建立」。username/password 空 → 20100。口令 bcrypt。成功即签发 **24h** 会话 token（无自动续期） |
| 2 | `POST /api/auth/login` | 公开。缺字段 / 用户不存在 / 口令错一律**统一 20200 `"invalid username or password"`**（刻意不区分，防用户名枚举）。成功每次签发**新的**会话 token（旧 token 不连带撤销，各自 24h 自然到期） |
| 3 | `GET /api/auth/me` | **Bearer 必需**（`allowPAT=true`：自助身份查询，机器令牌也可用）。`data = AuthUser` |
| 4 | `DELETE /api/auth/logout` | **Bearer 必需**（`allowPAT=true`）。撤销「本次调用所用凭证自身」（会话或 PAT），即时生效。`data = {status: "ok"}`。**用 DELETE 而非 POST**：泛型路由的 POST 必绑 JSON body，登出无 body → EOF 绑定失败（`cmd/routes.go` 注释钉死） |

易踩点：

- **渐进强制**：当前只有本族 me/logout 两个端点与第 11 节的 3 个端点强制 Bearer，`register / login` 是本族仅有的公开端点；其余资源端点仍匿名（Phase C 逐步接入）——不要按「全站已鉴权」理解 20200。
- 20200 不只属于认证：Bearer 中间件对非业务错误也 fail-closed 成 20200（`middleware/bearer.go`）。

## 11. Token PAT 管理（3 个端点）

三个端点全挂 `Bearer(allowPAT=false)`：**用户管理操作，仅限人类会话**——机器令牌在此 20200「个人访问令牌不允许访问该端点」（防机器令牌自我提权，issue #184 验收 3）。

实体 `TokenInfo = {id, name, prefix, created_at, last_used_at, last_used_ip, last_used_ua, expires_at, revoked}`——**绝不含哈希或明文**；`expires_at` 空串 = 不过期。

| # | 方法与路径 | 说明 |
|---|-----------|------|
| 1 | `POST /api/tokens` | 创建。body `{name, never}`（yaml 声明两项 required，服务端不硬校验：name 空 → 存 NULL，never 缺省 → false）。默认有效期 **90 天**；`never=true` → `expires_at=NULL`（长期不过期，UI 侧需显式勾选并提示风险）。`data = PATCreateResult {token, info}`——token 为**唯一一次**明文回显 |
| 2 | `GET /api/tokens` | 仅列**调用者本人**的 PAT（`created_at DESC, id DESC`）；已撤销行**不隐藏**，以 `revoked: true` 标注。空 → `[]` |
| 3 | `DELETE /api/tokens/{id}` | 按数字 id 撤销（非数字 → 20100「invalid id」）。UPDATE 以 `user_id + kind=pat + 未撤销` 三重限域：**跨用户/不存在/已撤销都落 0 行并返回成功**（没有存在性门控，真幂等——与第 8 节 Alert DELETE 的 20300 行为**相反**）。撤销即时生效 |

## 12. 变更纪律

- 改接口三步：**先改 `api/openapi.yaml` → `make api-gen` → 双端消费生成物**；禁止直接编辑生成物。
- 契约测试：`backend/internal/handler/contract_parity_test.go` 反射守卫 handler-local In 镜像与
  entity 的 json tag；泛型路由的 bind 行为差异由 baseline 测试钉死。
- 重跑生成后必须 `diff` 生成物，确认只漂了本次改动。
- 本文（api.md）随端点增删同步更新；叙述与 yaml 冲突时以 yaml 为准并回改本文。
