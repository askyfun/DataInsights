# .github AGENTS.md

GitHub Actions 工作流配置：一部分是基于 OpenCode AI Agent 的自动化开发流程，另一部分是人工触发的发布流水线。

## 工作流清单

| 文件 | 触发方式 | 用途 |
|------|----------|------|
| `release.yml` | 手动（`workflow_dispatch`，填版本号） | 发布：校验版本号 + 双端测试 → 构建并冒烟 → 推多架构镜像到 Docker Hub → 打 tag + 建 Release。手册见 `docs/deployment/release.md` |
| `opencode-comment.yml` | Issue/PR 评论含 `/oc` 或 `/opencode` | 响应式 AI Agent，处理评论中的指令 |
| `opencode-review.yml` | PR 创建/同步/重新打开 | 自动代码审查（代码质量、潜在 bug、改进建议） |
| `opencode-cycle-hourly.yml` | 每 4 小时 + 手动触发 | 多 Agent 开发周期（PM → Architect → Developer → QA） |
| `opencode-developer-single.yml` | 被 cycle 工作流调用 | 单 Issue 开发（处理 `problem-confirmed` 标签的 issue） |
| `opencode-scheduled.yml` | 每日 00:00 UTC | 扫描 TODO 注释，创建跟踪 issue |

## Agent 流程

### 开发周期（cycle-hourly）

```
PM Agent → Architect Agent → Developer Agent → QA Agent
```

1. **PM Agent**: 分析 `docs/*.md`，识别待改进需求，创建 issue（避免重复）
2. **Architect Agent**: 分析架构问题，拆分技术任务，创建 issue
3. **Developer Agent**: 扫描 `problem-confirmed` 标签的 issue，触发单 Issue 开发流程
4. **QA Agent**: 扫描 `developer-done` 标签的 issue，运行测试验证，标记 `qa-passed` 或 `qa-failed`

### Issue 标签体系

| 标签 | 含义 |
|------|------|
| `enhancement` | 功能增强 |
| `bug` | Bug 修复 |
| `pm` | PM Agent 创建 |
| `architect` | Architect Agent 创建 |
| `problem-confirmed` | 人工确认，等待开发 |
| `developer-done` | 开发完成，等待 QA |
| `qa-passed` | QA 测试通过 |
| `qa-failed` | QA 测试失败 |

## 配置

- AI 模型: `minimax-cn-coding-plan/MiniMax-M2.5`（仅 opencode 系列工作流用）
- 密钥（Secrets）：
  - `MINIMAX_API_KEY` —— opencode 系列工作流的模型密钥
  - `DOCKERHUB_USERNAME` / `DOCKERHUB_TOKEN` —— `release.yml` 推送镜像用（令牌需 Read & Write）；可选 Variable `VITE_SENTRY_DSN` 用于构建期内联前端 Sentry
  - `GITHUB_TOKEN` 由 Actions 自动提供，无需手工配置（`release` job 需要 `contents: write` 才打得动 tag）
- 运行环境: `ubuntu-latest`
- Go / Node 版本：opencode 系列工作流**不构建**产品产物，只跑测试；`release.yml` 会真正构建产品镜像，但镜像内的 Go / Node 版本以仓库根 `Dockerfile` 为准（`golang:1.27-alpine` + `node:24-alpine` = **Go 1.27 / Node 24**），工作流里的 `setup-go` / `setup-node` 只是给测试与静态检查提供同版本运行时。
- `release.yml`、`opencode-cycle-hourly.yml`（qa-agent）与 `opencode-developer-single.yml` 里的 `setup-go@v5` / `setup-node@v4` 步骤钉在 **Go 1.27 / Node 24**（供测试），与仓库根 `Dockerfile` 的产品构建版本一致 —— 改 Dockerfile 基础镜像时这几处要一起改。
