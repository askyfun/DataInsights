# 发布流程（Release）

> 最后更新：2026-09-26
> 一句话：在 GitHub Actions 里手动跑一次 `Release` 工作流、填一个版本号，它会校验版本号与测试、构建并推送多架构镜像到 Docker Hub、打 git tag、创建 GitHub Release。
> 工作流文件：[`.github/workflows/release.yml`](../../.github/workflows/release.yml)。镜像怎么用见[容器部署](docker.md)。

## 1. 一次性配置

### 1.1 Docker Hub 侧

1. 在 Docker Hub 建一个仓库：命名空间 `kzzhr`、名称 `datainsights`（即镜像 `kzzhr/datainsights`）。首次推送通常也会自动创建，显式建一个更稳。
2. 生成访问令牌：**Account Settings → Personal access tokens → Generate new token**，权限选 **Read & Write**。
   ⚠️ 用令牌，不要用登录密码 —— Docker Hub 已不支持密码推送，且令牌可单独吊销。

### 1.2 GitHub 仓库侧

**Settings → Secrets and variables → Actions**：

| 类型 | 名称 | 值 | 必需 |
|------|------|-----|------|
| Secret | `DOCKERHUB_USERNAME` | Docker Hub 用户名（命名空间 `kzzhr`） | ✅ |
| Secret | `DOCKERHUB_TOKEN` | 上面生成的访问令牌 | ✅ |
| Variable | `VITE_SENTRY_DSN` | 前端 Sentry DSN，构建期内联进产物 | 可选 |

- 只改镜像位置（比如换命名空间）时，改工作流顶部 `env.IMAGE` 一行即可。
- `GITHUB_TOKEN` 由 Actions 自动提供，**不需要**手工配置。

## 2. 发一个版本

1. 进 **Actions → Release → Run workflow**。
2. 在「Use workflow from」选要发布的 ref，**通常就是 `master`**。
3. 填 `version`：形如 `v1.2.3`（见 §3）。
4. `prerelease` 保持默认 `false`；发 RC 类版本时勾上（不移动 `latest`，Release 标记为 Pre-release）。
5. Run。全程约 15–30 分钟，大头在 QEMU 下的 arm64 构建。

## 3. 版本号规则

- 必须匹配 `^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?$`（后缀首字符必须是字母数字），例如 `v1.2.3`、`v0.9.0-rc.1`。不合法会**在跑测试前**直接失败。
- 同一个版本号只能发一次：tag 已存在时工作流立即失败（撤回旧版本见 §7）。
- 版本号同时用作 **git tag** 与 **Docker 镜像 tag**，两者一一对应。
- `latest` 只在 `prerelease = false` 时移动，始终指向最近一个正式版。

## 4. 流水线做了什么

四个 job 严格串行，**前面失败就不会产生任何对外可见的产物**：

| # | Job | 内容 | 失败会怎样 |
|---|-----|------|-----------|
| 1 | `verify` | 校验版本号格式与 tag 可用性、检查 Docker Hub 凭据已配置、`go test -race ./...`、`pnpm check`、`pnpm exec vitest run` | 不产生 tag、不推镜像 |
| 2 | `image-smoke` | 构建 `linux/amd64` 镜像并**真跑起来**：起一个 PostgreSQL 服务容器，`docker run --network host` 启动应用，轮询 `/health` 就绪，再验证 `/` 能返回页面 | 同上 |
| 3 | `publish` | `linux/amd64` + `linux/arm64` 构建并推送 Docker Hub，打 OCI labels | 不产生 tag / Release |
| 4 | `release` | 在本次运行的 SHA 上打 tag 并推送、生成 Release 说明、创建 GitHub Release | 镜像已在 Docker Hub 上，但版本号尚未对外发布（重跑即可） |

把 tag 与 Release 排在镜像推送**之后**是刻意的：等产物真的在了，再让这个版本号对外存在。

产物清单：

- git tag `<version>`（指向本次运行的 commit）
- GitHub Release `<version>`，说明里带 `docker pull` / compose 的直接命令
- Docker Hub `kzzhr/datainsights:<version>`（正式版额外多一个 `latest`），同时支持 amd64 与 arm64

## 5. 先本地验一遍（可选）

工作流里的每一步都能在本地复现，改完流水线别拿真版本试错：

```bash
make docker-up                              # 等同 image-smoke 的构建 + 启动
docker compose logs -f                      # 看启动日志
curl -fsS http://localhost:23352/health     # 探活
curl -fsS http://localhost:23352/ | head    # 首页由同一进程提供
```

## 6. 失败处置

| 现象 | 原因 / 处置 |
|------|------------|
| `缺少仓库 Secret: DOCKERHUB_TOKEN` | 按 §1.2 补齐 Secret 后重跑。 |
| `tag vX.Y.Z 已存在` | 换一个版本号；或先按 §7 撤回旧版本。 |
| `go test` / `vitest` 红 | 是真实门禁，修代码后重跑；不要为了发版跳过测试。 |
| `vitest` 打 `Errors 1 error` 但计数全绿 | 这是仓库**既有噪声**（jsdom 拆除期 react-dom 抛 `window is not defined`），会让全绿的跑批也以退出码 1 收场。`verify` 里的前端测试**只按用例计数判定**，不看退出码；出现 `failed` 计数或没有 `passed` 计数才会红。想要治本见下方「已知限制」。 |
| 登录 Docker Hub 报 `unauthorized` | 令牌权限不足或已吊销 —— 重新生成 Read & Write 令牌并更新 Secret。 |
| arm64 阶段超时 | QEMU 模拟构建较慢。可退一步只发 `linux/amd64`（改 `platforms`），Apple Silicon 用户会走 Rosetta 模拟，仍可运行。 |

## 7. 撤回一个版本

发布出错时，**三处都要清**，否则下次发同一个版本号会因 tag 已存在而失败：

```bash
# 1. GitHub Release 与 tag
gh release delete <version> --yes
git push --delete origin <version>
git tag -d <version>

# 2. Docker Hub 上的镜像 tag（网页端 Repository → Tags → Delete，
#    或命令行删 manifest；latest 若已被移动需另行指回上一个正式版）

# 3. 本地通知使用方换版本号
```

已被人拉走的镜像无法回收，所以**优先用 `prerelease` 试跑**，再发正式版。

## 8. 已知限制

- **`vitest` 的退出码在本仓不可信**：存在一条既有的未处理错误（jsdom 环境拆除期，react-dom 调度出的宏任务晚于环境销毁执行，抛 `ReferenceError: window is not defined`），会让 **584/584 全绿**的跑批同样以退出码 1 收场（Node 22 / Node 24 均可复现）。因此 `verify` 里的前端测试**只按用例计数判定**。**治本方向**：定位把页面组件拉进 jsdom 的用例（`frontend/src/__tests__/chartQuery.test.ts` 是一条线索，它 import 了 `@/pages/ChartBuilder`），在该用例或 `setup.ts` 里补 cleanup / 等宏任务 flush；退一步可在 `frontend/vite.config.ts` 的 `test` 块加 `dangerouslyIgnoreUnhandledErrors: true`（全局放宽，代价是不再兜住未来真正的 unhandled error）。
- 前端 Sentry DSN 是**构建期内联**的：镜像一旦发布，DSN 就固定了，改它必须重发一个版本。
- 构建缓存走 GitHub Actions cache（`type=gha`），换版本号不会让缓存失效；依赖清单变更后第一次构建会明显变慢。
