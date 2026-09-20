# Data Insights API 接口文档

> **重要**: 本文档是 Data Insights 项目的统一 API 接口规范。每次修改接口后必须同步更新本文档。
>
> - 规范版本: 1.0.0
> - 最后更新: 2026-05-05

---

## 1. 通用规范

### 1.1 统一响应格式

所有 API 响应以及后端健康检查响应必须使用以下统一格式：

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {}
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| code | int | 是 | 状态码，详见状态码表 |
| msg | string | 是 | 响应信息，通常不需要显示给用户 |
| trace | string | 是 | 链路追踪 ID，从请求头 X-Request-ID 获取 |
| data | object/array | 是 | 主要数据，必须是对象格式，方便后续扩展 |

**集合字段约束**:

- 所有成功响应中的集合型字段（数组/对象）在“无数据”时必须返回空集合：数组返回 `[]`，对象返回 `{}`。
- **禁止** 将集合型字段序列化为 `null`，避免前端额外编写 `null` 分支处理。
- 统一错误响应的 `data` 默认为 `{}`，同样禁止返回 `null`。

### 1.2 状态码表

| 状态码 | 说明 | 用户提示 |
|--------|------|----------|
| 20000 | 成功 | - |
| 20100 | 请求参数错误 | 请求参数有误，请检查输入 |
| 20200 | 认证/授权错误 | 登录状态已失效，请重新登录 |
| 20300 | 资源不存在 | 请求的资源不存在 |
| 20400 | 业务逻辑错误 | 操作失败，请稍后重试 |
| 20500 | 第三方服务错误 | 服务暂时不可用 |
| 50000 | 服务端内部错误 | 服务器内部错误 |

### 1.3 请求头要求

| 请求头 | 必填 | 说明 |
|--------|------|------|
| X-Request-ID | 否 | 链路追踪 ID，由客户端生成，响应会原样返回 |

### 1.4 响应头

| 响应头 | 说明 |
|--------|------|
| X-Request-ID | 链路追踪 ID，原样返回请求中的值 |

### 1.5 字段命名规范

前后端 JSON 通信使用 **snake_case** 命名：

| 后端 (Go) | 前端 (TypeScript) | 用途 |
|-----------|-------------------|------|
| table_name | tableName | 表名 |
| query_sql | querySql | 查询SQL |
| database_name | databaseName | 数据库名 |
| created_at | createdAt | 创建时间 |
| data_type | dataType | 数据类型 |

---

## 2. API 端点

### 2.0 系统接口

#### 2.0.1 健康检查

```
GET /health
```

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "status": "ok"
  }
}
```

### 2.1 数据源管理 (Datasource)

#### 2.1.1 获取数据源列表

```
GET /api/datasources
```

**请求参数** (Query):

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| limit | int | 否 | 返回数量限制，默认 20 |
| offset | int | 否 | 偏移量，默认 0 |

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": [
    {
      "id": 1,
      "name": "My PostgreSQL",
      "type": "postgresql",
      "host": "localhost",
      "port": 5432,
      "database_name": "mydb",
      "username": "user",
      "password": "password",
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  ]
}
```

---

#### 2.1.2 获取单个数据源

```
GET /api/datasources/:id
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 数据源 ID |

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "id": 1,
    "name": "My PostgreSQL",
    "type": "postgresql",
    "host": "localhost",
    "port": 5432,
    "database_name": "mydb",
    "username": "user",
    "password": "password",
    "created_at": "2026-01-01T00:00:00Z",
    "updated_at": "2026-01-01T00:00:00Z"
  }
}
```

---

#### 2.1.3 创建数据源

```
POST /api/datasources
```

**请求体**:

```json
{
  "name": "My PostgreSQL",
  "type": "postgresql",
  "host": "localhost",
  "port": 5432,
  "database_name": "mydb",
  "username": "user",
  "password": "password"
}
```

**字段说明**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 数据源名称 |
| type | string | 是 | 数据库类型: postgresql, mysql, clickhouse, starrocks |
| host | string | 是 | 主机地址 |
| port | int | 是 | 端口号 |
| database_name | string | 是 | 数据库名称 |
| username | string | 是 | 用户名 |
| password | string | 是 | 密码 |

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "id": 1,
    "name": "My PostgreSQL",
    "type": "postgresql",
    "host": "localhost",
    "port": 5432,
    "database_name": "mydb",
    "username": "user",
    "password": "password",
    "created_at": "2026-01-01T00:00:00Z",
    "updated_at": "2026-01-01T00:00:00Z"
  }
}
```

---

#### 2.1.4 更新数据源

```
PUT /api/datasources/:id
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 数据源 ID |

**请求体**: 同创建数据源

**响应**: 返回更新后的数据源

---

#### 2.1.5 删除数据源

```
DELETE /api/datasources/:id
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 数据源 ID |

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "status": "ok"
  }
}
```

---

#### 2.1.6 测试数据源连接

```
POST /api/datasources/test
```

**请求体**:

```json
{
  "type": "postgresql",
  "host": "localhost",
  "port": 5432,
  "database_name": "mydb",
  "username": "user",
  "password": "password"
}
```

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "status": "ok"
  }
}
```

---

#### 2.1.7 获取数据源表列表

```
GET /api/datasources/:id/tables
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 数据源 ID |

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": [
    {
      "Name": "users",
      "Comment": "用户表"
    },
    {
      "Name": "orders",
      "Comment": "订单表"
    }
  ]
}
```

---

#### 2.1.8 获取表字段列表

```
GET /api/datasources/:id/tables/:table/columns
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 数据源 ID |
| table | string | 是 | 表名 (URL编码) |

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": [
    {
      "name": "id",
      "data_type": "integer",
      "comment": "主键ID"
    },
    {
      "name": "username",
      "data_type": "varchar",
      "comment": "用户名"
    }
  ]
}
```

---

#### 2.1.9 获取数据预览

```
POST /api/datasources/:id/preview
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 数据源 ID |

**请求体**:

```json
{
  "table_name": "users",
  "query_sql": "",
  "query_type": "direct"
}
```

**字段说明**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| table_name | string | 是 | 表名 |
| query_sql | string | 否 | 自定义SQL (query_type为accelerated时使用) |
| query_type | string | 是 | 查询类型: direct, accelerated |

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "columns": ["id", "username", "email"],
    "data": [
      {"id": 1, "username": "user1", "email": "user1@example.com"},
      {"id": 2, "username": "user2", "email": "user2@example.com"}
    ]
  }
}
```

---

#### 2.1.10 获取字段分布

```
POST /api/datasources/:id/field-distribution
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 数据源 ID |

**请求体**:

```json
{
  "table_name": "users",
  "query_sql": "",
  "query_type": "direct",
  "field_name": "status",
  "limit": 20
}
```

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "field_name": "status",
    "total_count": 1000,
    "unique_count": 5,
    "distribution": [
      {"value": "active", "count": 600, "percentage": 60},
      {"value": "inactive", "count": 400, "percentage": 40}
    ]
  }
}
```

---

### 2.2 数据集管理 (Dataset)

#### 2.2.1 获取数据集列表

```
GET /api/datasets
```

**请求参数** (Query):

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| limit | int | 否 | 返回数量限制，默认 20 |
| offset | int | 否 | 偏移量，默认 0 |

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": [
    {
      "id": 1,
      "name": "用户数据集",
      "datasource_id": 1,
      "table_name": "users",
      "query_sql": null,
      "query_type": "direct",
      "mode": "direct",
      "description": "用户基础信息",
      "tags": ["用户", "基础数据"],
      "columns": "[]",
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  ]
}
```

---

#### 2.2.2 获取单个数据集

```
GET /api/datasets/:id
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 数据集 ID |

**响应**: 返回数据集详情

---

#### 2.2.3 创建数据集

```
POST /api/datasets
```

**请求体**:

```json
{
  "name": "用户数据集",
  "datasource_id": 1,
  "table_name": "users",
  "query_sql": null,
  "query_type": "direct",
  "mode": "direct",
  "description": "用户基础信息",
  "tags": ["用户", "基础数据"]
}
```

**字段说明**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 数据集名称 |
| datasource_id | int | 是 | 数据源 ID |
| table_name | string | 否 | 表名 (query_type为direct时使用) |
| query_sql | string | 否 | 自定义SQL (query_type为accelerated时使用) |
| query_type | string | 是 | 查询类型: direct, accelerated |
| mode | string | 否 | 模式: direct, accelerated |
| description | string | 否 | 描述 |
| tags | string | 否 | 标签，JSON 字符串（如 `"[\"用户\",\"基础数据\"]"`）；后端按 JSON 串存储 |

**响应**: 返回创建的数据集

---

#### 2.2.4 更新数据集

```
PUT /api/datasets/:id
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 数据集 ID |

**请求体**: 与创建相同的字段集（`name`、`datasource_id`、`table_name`、`query_sql`、`query_type`、`mode`、`description`、`tags`、`columns`、`shard_enabled`、`shard_keys`），其中 `tags`/`columns`/`shard_keys` 为 JSON 字符串（前端在 api 层统一 stringify 数组）。

**语义（项目约定）**：遵循与数据源更新一致的 **"未提供则保留"** 规则——payload 省略或为空的可选字段（description/table_name/query_sql/tags/columns/shard_keys）保留存量值，不会被静默清空；显式发送 `"[]"` 可清空对应字段。`name`/`datasource_id` 为必填，缺失按 20100 参数错误。数据集不存在返回 20300。

**响应**: 返回更新后的数据集（信封 `data` 为完整 Dataset）

---

#### 2.2.5 删除数据集

```
DELETE /api/datasets/:id
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 数据集 ID |

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "status": "ok"
  }
}
```

---

#### 2.2.6 获取数据集字段列表

```
GET /api/datasets/:id/columns
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 数据集 ID |

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": [
    {
      "name": "id",
      "expr": "",
      "type": "integer",
      "comment": "主键ID",
      "role": "dimension"
    },
    {
      "name": "username",
      "expr": "",
      "type": "string",
      "comment": "用户名",
      "role": "dimension"
    }
  ]
}
```

---

#### 2.2.7 更新数据集字段

```
POST /api/datasets/:id/columns
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 数据集 ID |

**请求体**:

```json
[
  {
    "name": "id",
    "expr": "",
    "type": "integer",
    "comment": "主键ID",
    "role": "dimension"
  },
  {
    "name": "username",
    "expr": "upper(username)",
    "type": "string",
    "comment": "用户名(大写)",
    "role": "dimension",
    "isVirtual": true
  }
]
```

**字段说明**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 字段名称 |
| expr | string | 否 | 表达式 (虚拟字段使用) |
| type | string | 是 | 数据类型: number, integer, boolean, string, date, datetime |
| comment | string | 否 | 注释 |
| role | string | 是 | 角色: dimension, metric |
| isVirtual | boolean | 否 | 是否为虚拟字段 |

**响应**: 返回更新后的数据集

---

#### 2.2.8 获取数据集预览

```
GET /api/datasets/:id/preview
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 数据集 ID |

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "columns": ["id", "username", "email"],
    "data": [
      {"id": 1, "username": "user1", "email": "user1@example.com"},
      {"id": 2, "username": "user2", "email": "user2@example.com"}
    ]
  }
}
```

---

#### 2.2.9 执行数据集查询

```
POST /api/datasets/:id/query
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 数据集 ID |

**请求体**: 同图表查询配置

**响应**: 返回查询结果

---

### 2.3 图表管理 (Chart)

#### 2.3.1 获取图表列表

```
GET /api/charts
```

**请求参数** (Query):

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| limit | int | 否 | 返回数量限制，默认 20 |
| offset | int | 否 | 偏移量，默认 0 |

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": [
    {
      "id": 1,
      "name": "用户增长趋势",
      "dataset_id": 1,
      "chart_type": "line",
      "config": "{}",
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  ]
}
```

---

#### 2.3.2 获取单个图表

```
GET /api/charts/:id
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 图表 ID |

**响应**: 返回图表详情

---

#### 2.3.3 创建图表

```
POST /api/charts
```

**请求体**:

```json
{
  "name": "用户增长趋势",
  "dataset_id": 1,
  "chart_type": "line",
  "config": "{\"dims\": [\"date\"], \"metrics\": [{\"field\": \"count\", \"agg\": \"sum\"}]}"
}
```

**字段说明**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 图表名称 |
| dataset_id | int | 是 | 数据集 ID |
| chart_type | string | 是 | 图表类型: line, bar, pie, table, scatter |
| config | string | 是 | 图表配置 JSON |

**响应**: 返回创建的图表

---

#### 2.3.4 更新图表

```
PUT /api/charts/:id
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 图表 ID |

**请求体**:

```json
{
  "name": "用户增长趋势(新)",
  "chart_type": "bar",
  "config": "{\"dims\": [\"date\"], \"metrics\": [{\"field\": \"count\", \"agg\": \"sum\"}]}"
}
```

**响应**: 返回更新后的图表

---

#### 2.3.5 删除图表

```
DELETE /api/charts/:id
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 图表 ID |

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "status": "ok"
  }
}
```

---

#### 2.3.6 获取图表数据

```
GET /api/charts/:id/data
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 图表 ID |

**响应**: 返回图表数据 (格式取决于图表配置)

---

#### 2.3.7 执行图表查询

```
POST /api/charts/query
```

**请求体**:

```json
{
  "dataset_id": 1,
  "chart_type": "line",
  "dims": ["date", "status"],
  "metrics": [
    {"field": "count", "agg": "sum", "alias": "total_count"}
  ],
  "filters": [
    {"field": "date", "operator": "gte", "value": "2026-01-01", "logic": "and"}
  ],
  "pagination": {
    "page": 1,
    "page_size": 10
  },
  "sort": {
    "field": "total_count",
    "order": "desc"
  }
}
```

**字段说明**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| dataset_id | int | 是 | 数据集 ID |
| chart_type | string | 是 | 图表类型 |
| dims | string[] | 是 | 维度字段列表 |
| metrics | MetricConfig[] | 是 | 指标配置 |
| filters | FilterConfig[] | 否 | 过滤条件 |
| pagination | Pagination | 否 | 分页配置 |
| sort | SortConfig | 否 | 排序配置 |

**MetricConfig**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| field | string | 是 | 字段名 |
| agg | string | 是 | 聚合函数: sum, avg, count, max, min |
| alias | string | 否 | 别名 |

**FilterConfig**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| field | string | 是 | 字段名 |
| operator | string | 是 | 操作符: eq, neq, gt, gte, lt, lte, like, in, between, isNull, isNotNull |
| value | any | 是 | 过滤值 |
| value_end | any | 否 | 结束值 (between时使用) |
| logic | string | 否 | 逻辑: and, or |

**Pagination**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | int | 是 | 页码 |
| page_size | int | 是 | 每页数量 |

**SortConfig**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| field | string | 是 | 排序引用键：v1 平铺请求为输出列名/别名；v2 槽位协议请求（spec_version=2）为 binding_id（后端按 AST 中维度/指标的 BindingID 解析为输出别名，未命中回退按列名/别名匹配） |
| order | string | 是 | 排序方向: asc, desc |

**响应 (Line/Bar/Area)**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "x_axis": ["2026-01-01", "2026-01-02", "2026-01-03"],
    "series": [
      {
        "name": "total_count",
        "data": [100, 150, 200]
      }
    ]
  }
}
```

**响应 (Pie)**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "data": [
      {"name": "active", "value": 600, "percentage": 60},
      {"name": "inactive", "value": 400, "percentage": 40}
    ]
  }
}
```

**响应 (Table)**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "columns": ["date", "status", "count"],
    "data": [
      {"date": "2026-01-01", "status": "active", "count": 100}
    ],
    "pagination": {
      "page": 1,
      "page_size": 10,
      "total": 100,
      "total_pages": 10
    }
  }
}
```

**响应 (Scatter)**:

散点图使用两个 metrics：`metrics[0]` 映射 X 轴，`metrics[1]` 映射 Y 轴。dims 可选（不影响散点数据）。

请求示例：

```json
{
  "dataset_id": 1,
  "chart_type": "scatter",
  "dims": [],
  "metrics": [
    {"field": "revenue", "agg": "sum", "alias": "total_revenue"},
    {"field": "cost", "agg": "sum", "alias": "total_cost"}
  ]
}
```

响应示例：

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "data": [[100, 50], [200, 80], [150, 60]]
  }
}
```

**兼容协议说明**:

- 当前实现仍以 `dims` / `metrics` / `filters` / `pagination` / `sort` 为主。
- 服务端内部已接入兼容转换链路：`旧请求 -> QuerySpec -> QueryPlanner -> 旧 executor 请求`，并已新增 `QuerySpec -> QueryAST` 的第一阶段规划能力。
- `chartService.Query` 当前会在内部附带 `PlannedAST` 传给 `query.Executor`，由 executor 优先使用 AST 生成 SQL；对外请求体仍保持旧协议。
- 当前 `QueryAST` 已支持结构化维度/指标元信息、`limit`，以及日期维度 `day` 粒度的 PostgreSQL / MySQL / ClickHouse SQL 生成；对外请求协议暂不变。
- 后续演进目标是在保持当前协议兼容的前提下，引入结构化 `chart_spec`。
- 兼容期内建议前端优先遵循本节旧协议；新协议文档以“草案”形式定义在下文。

**目标契约草案**:

目标请求结构将从扁平的 `dims` / `metrics` 演进为结构化 `chart_spec`：

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
          "field": "count",
          "label": "总数",
          "agg": "sum",
          "alias": "total_count"
        }
      ]
    },
    "style": {
      "smooth": true
    },
    "query_options": {}
  }
}
```

`chart_spec` 设计目标：

- 支持多组维度，如透视表的 `rows` / `columns`。
- 支持多组指标，如双轴图的 `primary_values` / `secondary_values`。
- 支持字段属性，如日期维度的 `granularity`。
- 支持图表样式配置，如颜色、线型、表格行高。
- 支持图表特有查询配置，如饼图低比例合并为“其他”。

**目标响应结构草案**:

图表查询响应会逐步统一为 Envelope 结构：

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "shape": "axis",
    "data": {
      "x_axis": ["2026-01-01"],
      "series": [{"name": "total_count", "data": [100]}]
    },
    "meta": {
      "chart_type": "line",
      "granularity": "day"
    },
    "fields": [
      {"name": "created_at", "label": "日期"},
      {"name": "total_count", "label": "总数"}
    ],
    "sql": {
      "select_sql": "SELECT ...",
      "count_sql": "SELECT ..."
    }
  }
}
```

兼容期内：

- 仍保留当前各图表的 `data` 结构。
- 仍保留顶层 `select_sql` / `count_sql` 字段。
- 新结构作为后续升级目标，前后端需要围绕该结构补齐测试与实现。

---

### 2.4 分享管理 (Share)

#### 2.4.1 创建分享链接

```
POST /api/shares
```

**请求体**:

```json
{
  "chart_id": 1,
  "password": "optional_password",
  "expires_at": "2026-12-31T23:59:59Z"
}
```

**字段说明**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| chart_id | int | 是 | 图表 ID |
| password | string | 否 | 访问密码 |
| expires_at | string | 否 | 过期时间 |

**响应**:

```json
{
  "code": 20000,
  "msg": "success",
  "trace": "req_xxx",
  "data": {
    "id": 1,
    "token": "abc123def456",
    "chart_id": 1,
    "password": null,
    "expires_at": "2026-12-31T23:59:59Z",
    "created_at": "2026-01-01T00:00:00Z"
  }
}
```

---

#### 2.4.2 获取分享信息

```
GET /api/shares/:token
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| token | string | 是 | 分享 token |

**响应**: 返回分享信息 (不包含密码)

---

#### 2.4.3 访问分享链接

```
GET /share/:token
```

**路径参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| token | string | 是 | 分享 token |

**说明**: 这是公开访问接口，不需要认证。如果分享设置了密码，会返回密码验证页面。

**响应**: 返回图表数据或密码验证页面

---

## 3. 前端使用规范

### 3.1 统一响应处理

前端必须使用统一的 API 客户端 (`frontend/src/lib/api/client.ts`)：

```typescript
import { get, post, put, del } from '@/lib/api/client';

// 正确的使用方式
const result = await get<Datasource[]>('/api/datasources');
if (result.code === 20000) {
  const datasources = result.data;
}
```

### 3.2 错误处理

API 客户端会自动处理错误：
- 显示错误消息 (通过 antd message)
- 上报错误到 Sentry
- 抛出 Promise.reject

页面只需处理成功情况：

```typescript
// 推荐：只处理成功情况
const result = await get<Datasource[]>('/api/datasources');
// 错误会被自动处理
const datasources = result.data;

// 或者手动检查
const result = await get<Datasource[]>('/api/datasources');
if (result.code === 20000) {
  const datasources = result.data;
} else {
  // 错误已被自动处理，这里不需要额外逻辑
}
```

### 3.3 禁止直接使用 AxiosResponse

**禁止**:
```typescript
// ❌ 错误 - 绕过统一包装
import apiClient from '@/api';
const response = await apiClient.get('/api/datasources');
const datasources = response.data;  // 错误的访问方式
```

**必须**:
```typescript
// ✅ 正确 - 使用统一包装
import { get } from '@/lib/api/client';
const result = await get<Datasource[]>('/api/datasources');
const datasources = result.data;  // 正确的访问方式
```

---

## 4. 版本历史

| 版本 | 日期 | 变更 |
|------|------|------|
| 1.0.0 | 2026-02-27 | 初始版本，包含所有 API 端点 |
