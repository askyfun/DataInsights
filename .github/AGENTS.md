# .github AGENTS.md

GitHub Actions 工作流配置：一条每次提交都跑的测试门禁（`ci.yml`）、一条不定时的 fuzz 挖掘（`fuzz.yml`）、一条人工触发的发布流水线（`release.yml`）。

## 工作流清单

| 文件 | 触发方式 | 用途 |
|------|----------|------|
| `ci.yml` | `pull_request` + `push: master` | 每次提交的测试门禁：后端 build/vet/test、前端 biome + tsc + vitest（按用例计数判定）、密钥泄漏扫描；master 额外跑 `-race` 与 `pnpm build`；契约漂移只报告不阻断 |
| `fuzz.yml` | 每周二 19:00 UTC + PR 改动碰到有 fuzz 目标的包 + 手动 | 逐目标 `-fuzz` 挖掘。**故意不是门禁**（非确定性），产出是 issue + artifact。PR 上保持绿（只留 warning），定时/手动才亮红 |
| `release.yml` | 手动（`workflow_dispatch`，填版本号） | 发布：校验版本号 + 双端测试 → 构建并冒烟 → 推多架构镜像到 Docker Hub → 打 tag + 建 Release。手册见 `docs/deployment/release.md` |

## 配置

- 密钥（Secrets）：
  - `DOCKERHUB_USERNAME` / `DOCKERHUB_TOKEN` —— 推送镜像用（令牌需 Read & Write）；可选 Variable `VITE_SENTRY_DSN` 用于构建期内联前端 Sentry
  - `GITHUB_TOKEN` 由 Actions 自动提供，无需手工配置（`release` job 需要 `contents: write` 才打得动 tag；`fuzz.yml` 的 `mine` job 需要 `issues: write` 才开得出 issue）
- 运行环境: `ubuntu-latest`
- Go / Node 版本：`release.yml` 会真正构建产品镜像，镜像内的 Go / Node 版本以仓库根 `Dockerfile` 为准（`golang:1.27-alpine` + `node:24-alpine` = **Go 1.27 / Node 24**），工作流里的 `setup-go@v5` / `setup-node@v4` 只是给测试与静态检查提供同版本运行时 —— 改 Dockerfile 基础镜像时**三处**（`ci.yml` / `fuzz.yml` / `release.yml`）都要一起改。
- 分支保护（`master`）：必需检查是三个 job 的**中文名** —— `后端测试`、`前端检查与测试`、`密钥泄漏扫描`。GitHub 的 status check context 就是 job name，改 `name:` 不改保护配置会让 PR 永远卡在「Expected」。设置方式与坑见 `docs/developer-guide/contributing.md`「CI 门禁」。
  ⚠️ 顺序不能反：**先让 workflow 在仓库里真正跑过一次**，再把它挂成必需检查。反过来任何 PR 都会因为「检查永远不会出现」而卡死合不掉。
- ⚠️ **别把 fuzz 或契约漂移挂成必需检查**：前者非确定（同一代码今天挖到明天挖不到），后者当前必然红（生成物引用了 `go.mod` 里没有的 `oapi-codegen/runtime`）。两条的理由都在文件注释里，别只看绿灯就收紧。
- 前端测试判据在 `scripts/gate-frontend-tests.sh`，`ci.yml` 与 `release.yml` 共用这一份。**别在 workflow 里重抄**——那段「不能看退出码」的逻辑一旦两处各写，迟早漂移（原因见脚本头注释）。
- PR 页面上还会看到 `Analyze (go)` / `Analyze (javascript-typescript)`，那是 GitHub 自带的 CodeQL（source 为 `dynamic/github-code-quality/codeql`，仓库里没有对应文件），**不跑任何测试**；另有一条常年停在 `queued`、0 checks 的 Sentry check suite，属噪音。
