# 贡献指南（Contributing）

> 最后更新：2026-10-17
> 语言无关的行为准则（先思考再编码、极简优先、精准修改、接口先行等）以仓库根 `AGENTS.md` 为准，
> 本文件收录**协作流程**与**语言级**编码规范。

## 贡献流程（分支 / 提交 / PR）

- **分支**：从 `master` 切主题分支，命名 `<type>/<topic>`（如 `refactor/batch2-contract`、`feat/chart-funnel`）。一个分支只做一件事。
- **提交信息**：遵循 Conventional Commits —— `type(scope): 描述`。`type` 常用 `feat` / `fix` / `refactor` / `test` / `docs` / `chore` / `style`；`scope` 取模块名（如 `query`、`frontend`、`docs`、`dashboard`）。描述用中文，一句话说清「改了什么、为什么」。
- **提交纪律**：一次提交只包含一个可解释的改动；每行改动都应能追溯到需求，不做顺手重构（见根 `AGENTS.md`「精准修改」）。
- **PR / 评审**：`.github/workflows/ci.yml` 对每个 PR 与每次 master push 自动跑双端测试与密钥扫描（详见下节「CI 门禁」），其中三项是 `master` 的**必需检查**，不绿合不进去。另有 GitHub 自带的 CodeQL 静态分析（`Code Quality: PR #…`，仓库里没有对应 workflow 文件）——它**不跑任何测试**，别把它的绿当成"测试过了"。

## 提交前门禁

- **后端**：`go test -race ./...`（新增功能或修 bug 后必须补测试）。
- **前端**：`pnpm build:check`（等价 biome check + vitest 全量）。
- **pre-commit 钩子**（`.husky/pre-commit`）自动执行两步，任一步失败即拒绝提交：
  1. `node scripts/check-secrets.mjs` —— 扫描**新增行**里的敏感串：内网 IP、个人家目录绝对路径、个人工具链路径（正则见 `scripts/check-secrets.mjs`）。命中即拒绝，请改成占位符后再提交。
  2. `cd frontend && pnpm exec lint-staged` —— 对**暂存**的 `.ts/.tsx` 跑 `biome check`，任何 error 都会让提交失败。
   ⚠️ 这个钩子能绕过（`git commit --no-verify`），所以同一套敏感串模式在 CI 上又跑了一遍（`check-secrets.mjs --diff <本次范围>`），绕过也拦得住。

## CI 门禁

> 2026-10-17 之前本仓**没有**每次提交都跑的 CI：双端测试只在有人本地跑、或手动触发 `release.yml` 时执行。
> PR 页面上那两个 `Analyze (go)` / `Analyze (javascript-typescript)` 是 GitHub 自带的 CodeQL 静态分析，不含任何测试。

| 档 | 跑什么 | 何时 | 必需检查 |
|----|--------|------|----------|
| `ci.yml` 后端测试 | `go build` + `go vet` + `go test -count=1 ./...`（本机约 2s，**19 个 fuzz 目标的种子语料就在这 2s 里**） | 每个 PR + 每次 master push | ✅ |
| `ci.yml` 前端检查与测试 | `pnpm check` + `tsc --noEmit` + `scripts/gate-frontend-tests.sh` | 每个 PR + 每次 master push | ✅ |
| `ci.yml` 密钥泄漏扫描 | `check-secrets.mjs --diff <本次范围>` | 每个 PR + 每次 master push | ✅ |
| `ci.yml` 后端竞态检测 | `go test -race -count=1 ./...`（本机实测 80s，PR 上等不起） | 仅 master push | ❌ 不挂 PR |
| `ci.yml` 构建产物 | `pnpm build` | 仅 master push | ❌ |
| `ci.yml` 契约漂移报告 | 重跑 oapi-codegen 到临时文件、与入库生成物比对，只写 Job summary | 每个 PR | ❌ 见下 |
| `fuzz.yml` 挖掘 | 逐目标 `-fuzz` 撒输入，挖到就建/更新 issue | 每周二 + 改动碰到有 fuzz 目标的包 + 手动 | ❌ 非确定，故意不门禁 |

- **必需检查用 job 的中文名**（`后端测试` / `前端检查与测试` / `密钥泄漏扫描`）：GitHub 的 status check context 就是 job name，改 `name:` 会让保护失配、PR 永远卡在「Expected」。
- **切新分支前先同步 master**：`pull_request` 事件用的是**头分支里那份** `ci.yml`。从没有 `ci.yml` 的旧 master 切出来的分支开 PR，不会产生任何必需检查，PR 会永远卡在「Expected — Waiting for status to be reported」。表现为「CI 没过、合不了」时，先确认分支基线晚于 2026-10-17（`git ls-tree HEAD .github/workflows/` 里有 `ci.yml`）。管理员可以绕过保护直接合（`enforce_admins` 未开），别把它当日常通道。
- **契约漂移为什么只报告不阻断**：① 生成物现在引用 `github.com/oapi-codegen/runtime/types`，而 `backend/go.mod` 没有这个依赖，真去同步会把编译跑坏（issue #201 加 multipart 端点后引入）；② `make api-gen` 前端那半步在本仓必然失败（`openapi-typescript@7` 要 `typescript` 提供 `ts.factory`，本仓是 `typescript@7`）。两个前置修好后这项才该转成必需检查。
- **fuzz 挖掘不挂门禁**：同一份代码今天挖到、明天挖不到是常态，挂成必需检查会让 PR 随机卡死。它的产出是 issue + 跑批 artifact；**修好后把崩溃语料留在 `testdata/fuzz/` 下**，它就变成每次提交都跑的确定性回归种子（`--0` 那条就是这么处理的）。⚠️ PR 上这条检查**刻意保持绿**（只留 warning 标注），定时/手动跑批才亮红——非门禁的检查不该借用红绿灯，否则一个红 X 就会被读成「CI 没过、合不了」（PR #213 真踩过这轮空排查）。
- **前端测试判据只有一份**：`scripts/gate-frontend-tests.sh`（`ci.yml` 与 `release.yml` 共用）。它**按用例计数判定而非退出码**——本仓 jsdom 拆除期有一条既有未处理错误，全绿也会以 1 收场，细节见脚本头注释。
- **后端测试一律带 `-count=1`**：有测试在运行时读仓库内的文件（日期筛选的分支覆盖门禁要读前端 union），Go 的测试缓存对这类读取不总能及时失效，见 `troubleshooting.md`「工具与验证」。

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
