# .github AGENTS.md

GitHub Actions CI/CD 工作流配置，基于 OpenCode AI Agent 实现自动化开发流程。

## 工作流清单

| 文件 | 触发方式 | 用途 |
|------|----------|------|
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

- AI 模型: `minimax-cn-coding-plan/MiniMax-M2.5`
- 密钥: `MINIMAX_API_KEY`（Secrets）
- 运行环境: `ubuntu-latest`
- Go / Node 版本：工作流自身**不构建**产品产物，产品构建版本以仓库根 `Dockerfile` 为准（`golang:1.27-alpine` + `node:24-alpine` = **Go 1.27 / Node 24**）。
- `opencode-cycle-hourly.yml`（qa-agent）与 `opencode-developer-single.yml` 里的 `setup-go@v5` / `setup-node@v4` 步骤钉在 **Go 1.27 / Node 24**（供 agent 跑测试），与仓库根 `Dockerfile` 的产品构建版本一致。
