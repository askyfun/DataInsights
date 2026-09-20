# Data Insights API 规范

## 1. 统一响应格式

所有 API 响应以及后端健康检查响应必须使用以下统一格式：

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "xxxxxxx",
  "data": {}
}
```

### 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| code | int | 是 | 状态码，详见状态码表 |
| msg | string | 是 | 响应信息，通常不需要显示给用户 |
| trace | string | 是 | 链路追踪 ID，从请求头 X-Request-ID 获取 |
| data | object/array | 是 | 主要数据，必须是对象格式，方便后续扩展 |

### 集合字段约束

- 集合型字段无数据时必须返回空集合：数组返回 `[]`，对象返回 `{}`。
- 禁止将集合型字段或错误响应的 `data` 序列化为 `null`。

### 状态码表

| 状态码 | 说明 |
|--------|------|
| 20000 | 成功 |
| 20100 | 请求参数错误 |
| 20200 | 认证/授权错误 |
| 20300 | 资源不存在 |
| 20400 | 业务逻辑错误 |
| 20500 | 第三方服务错误 |
| 50000 | 服务端内部错误 |

## 2. 请求头要求

### 请求头

| 请求头 | 必填 | 说明 |
|--------|------|------|
| X-Request-ID | 否 | 链路追踪 ID，由客户端生成，响应会原样返回 |

### 响应头

| 响应头 | 说明 |
|--------|------|
| X-Request-ID | 链路追踪 ID，原样返回请求中的值 |

## 3. 后端实现

### 目录结构

```
backend/
├── internal/
│   ├── response/          # 统一响应封装
│   │   └── response.go
│   └── idls/             # 接口协议定义
│       ├── datasource.go
│       ├── dataset.go
│       ├── chart.go
│       └── share.go
```

### 响应封装示例

```go
// 成功响应
response.Success(c, data)

// 失败响应
response.Error(c, 20100, "invalid parameter")
```

## 4. 前端实现

### 目录结构

```
frontend/src/
├── lib/
│   └── api/              # 统一响应处理
│       └── client.ts
└── idls/                # 接口协议定义
    ├── datasource.ts
    ├── dataset.ts
    ├── chart.ts
    └── share.ts
```

### 响应处理示例

```typescript
// 统一处理响应
const result = await apiClient.get<ResponseData<Datasource[]>>('/api/datasources');
if (result.code === 20000) {
  // 成功处理
}
```

## 5. IDL 定义规范

### 命名规范

- 文件名使用领域名称，如 `datasource.go`、`dataset.go`
- 同一模块的接口协议放在同一个文件中
- 类型定义使用 PascalCase

### 文件结构

```go
// 请求类型定义
type CreateDatasourceRequest struct {
    Name string `json:"name"`
    Type string `json:"type"`
    // ...
}

// 响应类型定义
type DatasourceResponse struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
    // ...
}

// 列表响应（带分页）
type DatasourceListResponse struct {
    Items  []DatasourceResponse `json:"items"`
    Total  int                  `json:"total"`
    Page   int                  `json:"page"`
    Size   int                  `json:"size"`
}
```

## 6. 错误处理

### 前端错误处理

```typescript
// 响应拦截器自动处理
apiClient.interceptors.response.use(
  (response) => {
    const { code, msg, data } = response.data;
    if (code !== 20000) {
      // 显示错误信息
      message.error(msg);
      return Promise.reject(new Error(msg));
    }
    return response;
  }
);
```

### 状态码与错误提示映射

| 状态码 | 用户提示 |
|--------|----------|
| 20100 | 请求参数有误，请检查输入 |
| 20200 | 登录状态已失效，请重新登录 |
| 20300 | 请求的资源不存在 |
| 20400 | 操作失败，请稍后重试 |
| 20500 | 服务暂时不可用 |
| 50000 | 服务器内部错误 |


## 7. 图表查询契约演进草案

### 7.1 职责边界

- Chart 模块负责图表语义：图表类型、维度组/指标组定义、展示名、样式配置、图表特有查询配置。
- Query 模块负责查询语义：字段表达式、聚合、时间粒度、分桶、过滤、排序、分页、SQL 生成。
- Query 模块不直接理解折线图、饼图、透视表、双轴图等视觉概念。

### 7.2 目标请求模型

兼容期内，`POST /api/charts/query` 同时支持旧协议与新协议：

- 旧协议：`dataset_id` + `chart_type` + `dims` + `metrics` + `filters` + `pagination` + `sort`
- 新协议：`dataset_id` + `chart_spec`

新协议目标结构：

```json
{
  "dataset_id": 1,
  "chart_spec": {
    "chart_type": "line",
    "dimension_groups": {
      "x_axis": [
        {
          "field": "created_at",
          "label": "日期",
          "granularity": "day"
        }
      ]
    },
    "metric_groups": {
      "values": [
        {
          "field": "amount",
          "label": "销售额",
          "agg": "sum",
          "alias": "total_amount"
        }
      ]
    },
    "style": {
      "smooth": true,
      "colors": ["#1677ff"]
    },
    "query_options": {}
  }
}
```

### 7.3 ChartDefinition 标准

每种图表类型都应有对应的 `ChartDefinition`，至少声明：

- `chart_type`：图表类型标识。
- `dimension_groups`：维度组定义，如 `x_axis`、`rows`、`columns`、`primary_axis`。
- `metric_groups`：指标组定义，如 `values`、`primary_values`、`secondary_values`。
- `style_schema`：样式配置结构。
- `query_options_schema`：图表特有查询配置结构。
- `response_shape`：统一响应形态，如 `axis`、`table`、`pie`、`matrix`、`multi_axis`。
- `constraints`：字段数量约束、字段类型约束、是否允许多字段。

### 7.4 ChartSpec 标准

`ChartSpec` 表示一个图表实例实际绑定的字段和配置，至少包含：

- `chart_type`
- `dimension_groups`
- `metric_groups`
- `style`
- `query_options`

其中每个字段绑定都允许携带附加属性，例如：

- 维度：`field`、`label`、`granularity`、`format`、`sort`
- 指标：`field`、`label`、`agg`、`alias`、`unit`、`number_format`

### 7.5 QuerySpec 标准

Chart 模块需要将 `ChartSpec` 转换为 Query 模块可理解的 `QuerySpec`。`QuerySpec` 至少包含：

- 数据集 ID
- 结构化维度表达式
- 结构化指标表达式
- 过滤条件或过滤组
- 排序
- 分页
- limit / topN
- 时间粒度信息
- 分桶信息

### 7.6 统一响应 Envelope

图表查询响应目标统一为：

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "shape": "axis",
    "data": {},
    "meta": {},
    "fields": [],
    "sql": {
      "select_sql": "SELECT ...",
      "count_sql": "SELECT ..."
    }
  }
}
```

字段含义：

- `shape`：响应形态，决定前端如何解释 `data`。
- `data`：图表数据主体。
- `meta`：分页、时间粒度、TopN 截断、“其他”合并等元信息。
- `fields`：字段元数据和格式信息。
- `sql`：调试 SQL。

### 7.7 兼容策略

兼容期内：

- 服务端继续接受旧协议。
- 服务端内部当前已落地 `旧请求 -> QuerySpec -> QueryPlanner -> 旧 executor 请求` 的兼容链路，并已新增 `QuerySpec -> QueryAST` 的第一阶段规划能力。
- `chartService.Query` 当前会在内部构造 `PlannedAST` 并传递给 `query.Executor`，executor 优先消费 AST 生成 SQL；旧的平铺参数仍保留为兼容回退路径。
- 当前 `QueryAST` 已支持结构化维度/指标元信息、`limit`，以及日期维度 `day` 粒度的 PostgreSQL / MySQL / ClickHouse SQL 生成。
- 服务端内部仍未直接接受 `chart_spec` 作为正式对外请求字段，`ChartSpec` 目前主要用于内部建模和演进设计。
- 服务端继续保留顶层 `select_sql` / `count_sql`，同时逐步补充 `data.sql`。
- 文档中的旧协议视为“当前兼容协议”，新协议视为“目标契约协议”。

### 7.8 首批目标图表示例

- 折线图：`x_axis` 维度组 + `values` 指标组，重点支持日期字段时间粒度。
- 透视表：`rows` / `columns` 维度组 + `values` 指标组。
- 双轴图：`primary_axis` / `secondary_axis` 维度组 + `primary_values` / `secondary_values` 指标组。
- 饼图：`category` 维度组 + `value` 指标组，支持 `merge_other_below_ratio` 查询配置。
- 表格：`columns` 字段组，支持分页、排序、行高等样式配置。
