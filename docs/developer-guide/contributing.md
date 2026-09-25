# 贡献指南（Contributing）

> 最后更新：2026-09-26
> 语言无关的行为准则（先思考再编码、极简优先、精准修改、接口先行等）以仓库根 `AGENTS.md` 为准，
> 本文件收录**协作流程**与**语言级**编码规范。

## 贡献流程（分支 / 提交 / PR）

- **分支**：从 `master` 切主题分支，命名 `<type>/<topic>`（如 `refactor/batch2-contract`、`feat/chart-funnel`）。一个分支只做一件事。
- **提交信息**：遵循 Conventional Commits —— `type(scope): 描述`。`type` 常用 `feat` / `fix` / `refactor` / `test` / `docs` / `chore` / `style`；`scope` 取模块名（如 `query`、`frontend`、`docs`、`dashboard`）。描述用中文，一句话说清「改了什么、为什么」。
- **提交纪律**：一次提交只包含一个可解释的改动；每行改动都应能追溯到需求，不做顺手重构（见根 `AGENTS.md`「精准修改」）。
- **PR / 评审**：CI（`.github/workflows/`）对 PR 自动跑代码审查（代码质量、潜在 bug、改进建议）；合并前本地必须过「提交前门禁」。变更应附带对应测试。

## 提交前门禁

- **后端**：`go test -race ./...`（新增功能或修 bug 后必须补测试）。
- **前端**：`pnpm build:check`（等价 biome check + vitest 全量）。
- **pre-commit 钩子**（`.husky/pre-commit`）自动执行两步，任一步失败即拒绝提交：
  1. `node scripts/check-secrets.mjs` —— 扫描**新增行**里的敏感串：内网 IP、个人家目录绝对路径、个人工具链路径（正则见 `scripts/check-secrets.mjs`）。命中即拒绝，请改成占位符后再提交。
  2. `cd frontend && pnpm exec lint-staged` —— 对**暂存**的 `.ts/.tsx` 跑 `biome check`，任何 error 都会让提交失败。

## 通用工具链约定

- **前端 lint/format 只有 Biome**：`pnpm check` 跑完整检查；`pnpm build:check`（biome check + vitest）为提交前验证；pre-commit 钩子对暂存 `.ts/.tsx` 执行 `biome check`。不要引入 ESLint/Prettier。
- **提交前必跑**：后端 `go test -race ./...`、前端 `pnpm build:check`。

## TypeScript 规范

### 配置文件

- `strict: true` - 严格模式
- `noUnusedLocals: true` - 未使用变量报错
- `noUnusedParameters: true` - 未使用参数报错
- 路径别名: `@/*` 指向 `./src/*`

### 前后端字段命名规范 (CRITICAL)

前后端 JSON 通信（wire）使用 **snake_case**，TypeScript 接口字段名必须与后端 Go 的 `json` tag
**逐字一致**——全链路 snake，不做 camel 转换。

```typescript
// ❌ 错误 - 用 camelCase 命名 wire 字段，与后端对不上
interface DatasetColumn {
  dataType: string;     // 后端 json tag 是 type
  typeConfig: unknown;  // 后端 json tag 是 type_config
}

// ✅ 正确 - 与后端 json tag 完全一致（见 entity.DatasetColumn）
interface DatasetColumn {
  id: string;
  name: string;
  expr: string;
  type: string;
  type_config: TypeConfig;
  comment: string;
  role: string;
}
```

常见 wire 字段（双端同名，均为 snake_case）：

| JSON 字段（后端 Go = 前端 TS） | 用途 |
|-------------------------------|------|
| `data_type` | 数据源表结构列类型 |
| `table_name` | 表名 |
| `query_sql` | 查询 SQL |
| `query_config` | 图表查询配置 |
| `type_config` | 列类型专有配置 |
| `layout_json` | 仪表盘布局文档 |
| `created_at` | 创建时间 |

> 例外：`store` 内部的 UI 模型（如 `QueryConfig`）可保留 camelCase，它不是 wire 结构，不发往后端。

### 导入顺序

```typescript
// 1. React/库导入
import React, { useState, useEffect } from 'react';
import { Button, Table, Modal } from 'antd';
import { useNavigate } from 'react-router-dom';

// 2. 项目内部导入
import { datasourcesApi } from '@/api';
import { useStore } from '@/store';

// 3. 类型导入
import type { Datasource, Dataset } from '@/api';
```

### 命名规范

- 组件: PascalCase (`ChartBuilder.tsx`)
- 工具函数/变量: camelCase
- 常量: UPPER_SNAKE_CASE
- 禁止使用 `any`，用 `unknown` 代替；**禁止类型压制**（`as any`、`@ts-ignore`/`@ts-expect-error` 零容忍）

---

## Go 规范

技术栈（Gin + bun + PostgreSQL）与分层架构见仓库根 `AGENTS.md` 与同目录的 `architecture.md`，此处不复述。

### 包导入顺序

```go
import (
    "log/slog"
    "net/http"
    "os"

    "github.com/gin-gonic/gin"
    "github.com/uptrace/bun"

    "data-insights/internal/config"
    "data-insights/internal/database"
    "data-insights/internal/model"
)
```

### 命名规范

- 导出标识符（函数/结构体/接口/常量）: PascalCase
- 非导出标识符: camelCase
- Error 变量: `ErrXXX` 或 `XXXError`

### Handler 模式

Handler 走 `internal/router` 的泛型注册，不再直接操作 `gin.Context`。签名固定为
`API[In, Out] func(req router.Request[In], res *router.Response[Out]) error`——`res` 是**指针**，
值传递会静默丢弃 handler 写入的 payload。成功/错误响应由 router 统一包装信封，handler 内**不再**手写
`response.*`。注册在 `cmd/routes.go`：`router.RegisterGetRoute` / `RegisterPostRoute` /
`RegisterPutRoute` / `RegisterDeleteRoute`（In/Out 由方法值推断）。样板见
`backend/internal/handler/dataset.go` 顶部 package doc（`datasource.go` 为全项目迁移参考）。

```go
// List handles GET /api/datasets
func (h *DatasetHandler) List(req router.Request[datasetListIn], res *router.Response[[]entity.Dataset]) error {
    limit, offset := req.In.pagination()
    datasets, err := h.svc.List(req.Ctx.Request.Context(), limit, offset)
    if err != nil {
        return err // router 统一转成错误信封
    }
    if datasets == nil {
        datasets = []entity.Dataset{}
    }
    res.Out = datasets // 只写 payload，router 负责包装
    return nil
}
```

两条现行约定：

- **PUT 更新「未提供则保留」**：payload 省略/空的可选字段保留存量值而非清零（datasource 密码、
  dataset 可选元数据同例），显式 `"[]"` 仍可清空。见 handler 层实现。
- **handler-local In 镜像与 entity 的 `json` tag 一致性**由 `backend/internal/handler/contract_parity_test.go`
  反射守卫；entity 不带 `form:"-"` 会重新引入 query 参数污染注入面，故 In 用 handler-local 镜像。

**2 个例外**（无法套信封，仍用裸 `gin` 写法）：`/health`（`cmd/main.go` 用裸 `r.GET` + `response.Success`）、
share `View`（返回 302 重定向，且只在纯 API 模式下注册）。

### 数据库模型 (bun ORM)

```go
type Datasource struct {
    bun.BaseModel `bun:"bi_datasource"`
    ID            int    `bun:"id,pk,autoincrement" json:"id"`
    Name          string `bun:"name" json:"name"`
    Host          string `bun:"host" json:"host"`
    // ...
}
```

### 错误处理

所有 API 响应走 `internal/response` 的统一信封 `{code, msg, trace, data}`（成功 `code: 20000`），
**不要**直接 `c.JSON(..., gin.H{"error": ...})`。迁移到泛型 router 后，handler 只需 `return err`，
由 router 统一包装成错误信封；业务错误返回 `router.BusinessError{Code, Message}`。

```go
// 正确：迁移后的 handler 只上报错误，router 负责信封包装
if err != nil {
    slog.Error("操作失败", "error", err)
    return err
}

// 避免：空 catch 块被严格禁止（必须记录日志并向上返回错误）
if err != nil {
    // 空块 - 禁止
}
```
