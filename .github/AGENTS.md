# .github AGENTS.md

GitHub Actions 工作流配置：目前只保留人工触发的发布流水线。

## 工作流清单

| 文件 | 触发方式 | 用途 |
|------|----------|------|
| `release.yml` | 手动（`workflow_dispatch`，填版本号） | 发布：校验版本号 + 双端测试 → 构建并冒烟 → 推多架构镜像到 Docker Hub → 打 tag + 建 Release。手册见 `docs/deployment/release.md` |

## 配置

- 密钥（Secrets）：
  - `DOCKERHUB_USERNAME` / `DOCKERHUB_TOKEN` —— 推送镜像用（令牌需 Read & Write）；可选 Variable `VITE_SENTRY_DSN` 用于构建期内联前端 Sentry
  - `GITHUB_TOKEN` 由 Actions 自动提供，无需手工配置（`release` job 需要 `contents: write` 才打得动 tag）
- 运行环境: `ubuntu-latest`
- Go / Node 版本：`release.yml` 会真正构建产品镜像，镜像内的 Go / Node 版本以仓库根 `Dockerfile` 为准（`golang:1.27-alpine` + `node:24-alpine` = **Go 1.27 / Node 24**），工作流里的 `setup-go@v5` / `setup-node@v4` 只是给测试与静态检查提供同版本运行时 —— 改 Dockerfile 基础镜像时这几处要一起改。
