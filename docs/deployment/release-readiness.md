# V0.4.0 发版就绪判据（release readiness）

> 最后更新：2026-11-06
> 一句话：v0.3.1 → v0.4.0 之间**还差什么、怎么证明差的那些已经补齐**，以及「补齐」之后仍需人工跑实的清单。
> 条目清单与立项目标来自 issue #230；本文件只负责把它的「出口判据」落成**可执行**的形态。
> 快速核对：`bash scripts/release-gate-v0.4.0.sh`（静态半程，见 §4）。

## 1. 这份文档解决什么问题

#230 登记了 16 条出口判据，全部写在 issue 正文里。发版时靠人回忆这 16 条等于没写——
本仓已有过一次同类事故：`release.yml` 的说明写成 commit 标题清单，对外看不到迁移水位与鉴权边界，
后来才改成从 `CHANGELOG.md` 抽取（见[发布流程](release.md) §2 第 6 步）。所以这里把判据拆成两半：

| 半程 | 谁能判 | 形态 |
|------|--------|------|
| **静态**（代码/文档里有没有这条能力的痕迹） | 机器 | `scripts/release-gate-v0.4.0.sh`，16 项逐条 PRESENT / MISSING |
| **实机**（判据要求「跑出来的现象」） | 人 | §3 的第三条列，发版前逐条签核 |

**静态痕迹命中不等于完成**。脚本对这一点写死在结尾提示里：`PRESENT` 只证明字面量存在，
16 条实机判据一条没跑就不能放行。反过来 `MISSING` 几乎总是真的（见 §4 的字面量纪律）。

## 2. 当前结论（照 2026-11-06 的 `origin/master` = `cac4d85` 取证）

**v0.4.0 现在不可发**，且瓶颈不在「待开始」队列，而在**评审队列**：

1. **19 条 PR 全部停在 `needs-review`，没有一条 `ready-to-merge`**。本版 A/B 两组的主干工作
   都已经写成 PR 躺在那儿（#202/#216/#217 之外基本齐），评审闸口不收，发版就一直不动。
   顺带一条取证口径：这 19 条里只有 PR #241 / #242 通过 `closingIssuesReferences` 关联了 issue，
   其余都靠标题的 `<issue号>:` 前缀对齐——**反查某条目有没有 PR，要用 `headRefName == agent/<n>`，
   别用关联字段**，否则会得出「没人在做」的错误结论。
2. **三条 P0 条目连 PR 都没有**：#203（认证限流）、#217（`AUTH_ENFORCE` 渐进开关）、#226（首启向导）。
   其中 **#203 与 #217 根本没有看板条目**（issue 是 OPEN 的，但不在 Plan 看板的任何一列），
   所以「从待开始列领任务」的自动循环永远看不到它们——这是本版最容易被漏掉的一条。
3. **#217 是本版主题成立的那一条**：没有它，其余身份工作做完后资源端点仍全部匿名可读写
   （`backend/cmd/routes.go` 只有 `/api/auth/*` 4 条与 `/api/tokens/*` 3 条挂了 `middleware.Bearer`）。
   「让别人用起来」如果不含「数据真的被保护」，就只是把登录页做给人看。
4. **#218 前端身份壳已被判为受阻**：它依赖的 `GET /api/auth/config`、越权独立码、用户管理页
   都还在未合并的 PR 里（master 上 `backend/internal/response/response.go` 只有 `CodeUnauthorized = 20200` 一档）。
   它必须排在 #202 / #216 / #224 / #217 之后，不能并行猜契约。
5. **#228 文档口径收口必须最后做**：`docs/` 里现存 2 处「无登录、无鉴权」原句
   （`getting-started/quick-start.md`、`deployment/docker.md`）。在能力还没落地时把它们改掉，
   等于把文档从「过期」改成「说谎」。

### 依赖红线（照 issue 正文，不按 PR 号）

```
#202 用户管理 ─┬─ #216 首管 bootstrap ── #217 AUTH_ENFORCE ── #219 环回默认（已具备条件）
               ├─ #203 认证限流                            └─ #218 前端身份壳
               └─ #224 稳定码/越权码 ────────────────────────┘
#221 请求关联 ── #222 错误编号 ── #223 Sentry 上报 ── #229 诊断包
#226 首启向导（要 #216 的首管流程 + #217 的门禁语义才可测）
最后：#228 文档收口（清除「无登录、无鉴权」)
```

`AUTH_ENFORCE=on` 必须晚于 **#202 + #216 + #218** 全部合入——否则升级当天会把所有人（含管理员自己）
锁在门外，且没有建号端点可以自救。这条是硬红线，写进 #230 正文，也在闸脚本注释里对应 #217/#218 两项。

## 3. 16 条出口判据总表

「静态标记」= 闸脚本为该条钉的字面量（命中即 PRESENT）；「实机」= 机器代不了劳、发版前必须跑的现象。

| # | 条目 | 静态标记 | 实机判据（发版前逐条签核） |
|---|------|----------|-----------------------------|
| #202 | 最小用户管理 | `CreateUser`/`DisableUser`/`ResetPassword` + `disabled_at` | 管理员建第二个号 → 新号能登录 → 被禁用后**下一次**请求即被拒（不是等 token 过期） |
| #203 | 认证端点限流与失败锁定 | `AUTH_RATE_LIMIT_PER_MINUTE`、`Retry-After` | 第 N+1 次被拒；窗口滑过恢复；锁定到期自动解锁；IP 维与用户名维互不干扰；`go test -race` 绿且用例用假时钟不 `sleep` |
| #216 | 首管 bootstrap 与恢复 | `bootstrap_token`/`BOOTSTRAP_TOKEN`、`bootstrap-status` | 无一次性凭据时注册被拒；并发 bootstrap 只有一个成功；禁用最后一个管理员后注册**仍关闭** |
| #217 | `AUTH_ENFORCE` 三态 + 路由守卫 | `AUTH_ENFORCE`、`auth_would_deny`、`auth/config` | 路由策略守卫测试遍历 `gin.Engine.Routes()` 全绿；`shadow` 出 `auth_would_deny` 且不改响应；`on` 后 `?q=` 分享直链**仍匿名可读**、PAT 仍能取数、未登录访问写端点被拒 |
| #218 | 前端身份壳 | `Route path="/login"`、`sessionStorage` | 真实浏览器：登录 → 刷新仍在 → 退出 → 再访问受保护页被弹回 `/login?next=`；「无权限」与「去登录」文案不混用 |
| #219 | compose 只绑环回 + `HOST_BIND` | `HOST_BIND` | `docker compose config` 渲染出 `127.0.0.1`；同网段另一台机器连不上该端口 |
| #220 | 归属写入与创建人可见 | `created_by`/`ownerName`/`creatorName` | 登录后创建的 datasource/dataset/chart/dashboard/queryrecord/alert，列表页显示创建人且库里 `owner_id` 为当前用户 |
| #221 | 请求关联与错误日志 | `biz_code` | 任意失败请求 stdout **恰好一行**且含 `request_id`/`biz_code`/`user_id`；带与不带 `X-Request-ID` 两种情况下 `trace` 都由 `request_id` 供值 |
| #222 | 错误编号与可复制报障 | `error_id`/`errorId` | 断网 → 文案是「网络已断开」且**无编号**；服务端错 → 出编号且一键复制能带齐上下文 |
| #223 | 非 panic 错误集中上报 | `CaptureException`、`BeforeSend` | 一次失败只产生一个 Sentry event；payload 里不含连接串 / token |
| #224 | 稳定错误码与文案分离 | `hint_key`/`hintKey`、`CodeForbidden` | 首批稳定码 ≥5；`openapi.yaml` ↔ 后端常量 ↔ 前端词表 三方一致性测试绿 |
| #225 | 版本与运行状态可见 | `ldflags`、`/version`、`health/ready` | 断库时 `/health`=200、`/health/ready`=503；detail 关闭时 `/version` 的 detail 段返 404；二进制 `-version` 输出的版本号与 tag 一致 |
| #226 | 首启向导与空状态引导 | `首启向导`/`SetupWizard` | 全新部署（空库）5 分钟内走完「建管理员 → 载示例 → 出第一张图」 |
| #227 | 备份 / 升级 / 回滚 | 文件存在：`scripts/backup.sh`、`scripts/upgrade.sh` | `upgrade.sh` dry-run 出备份文件；故意断库 60s 内判失败并打印两条回滚命令 |
| #228 | 文档口径收口 | **反向判据**：`无登录、无鉴权` 必须查不到 | 新 env 全部进 `user-guide/configuration.md` 字典；first-run 章节步骤照抄可跑通 |
| #229 | 诊断包导出与自检面板 | `diagnostic`/`诊断包` | 对**导出物**逐串查 `DATABASE_URL=`、`password=`、`di_pat_`，三者都必须查不到（这条判据 grep 的对象是导出物而不是仓库，静态闸代不了劳） |

> 表中 `#203` / `#217` / `#226` 三条**当前没有任何 PR**，所以它们的静态行必然 MISSING，
> 且无法靠「评审合并现有 PR」消掉——需要的是开工，不是评审。

## 4. 闸脚本怎么用，以及它怎么保证自己不假绿

```bash
bash scripts/release-gate-v0.4.0.sh   # 退出码 0 = 16 项静态痕迹齐备（仍不等于可发版）
```

三条防线：

1. **自我排除**。本闸与本文档都把 16 条判据的字面量原样写在里面（否则人看不懂要钉什么），
   纳进扫描集会让每一项都命中。这个坑在本次开发里真实踩到两次：第一版扫描范围含 `scripts/`，
   跑出来 14/16 PRESENT 而 master 上一条都没做；第二版改成显式排除该目录后，补上本文档又让
   15 项翻绿。现在的做法是 `scan_files()` 先列文件再按名字排除 `release-gate-v0.4.0.sh` 与
   `release-readiness.md`，同时把 `go.sum` / `bun.lock` 这类锁文件挡在外面（`20201` 这种数字判据
   在 `backend/go.sum` 里必然误命中）。
2. **检查项计数守卫**：`rows` 必须等于 16，否则直接判红。「挖着挖着把某条判据删掉了就全绿」
   是本仓 fuzz 流水线踩过的同类失效模式（`fuzz.yml` 零命中曾以 `crashes=0` 正常绿着收场）。
3. **判据字面量是反证过的**：每条都挑该项独有、且写脚本时在 `origin/master` 上确实不存在的串。
   双向验证做过：注入一条含全部字面量的注释后，14 项 grep 判据翻成 PRESENT，而两条非 grep 判据
   （#228 反向、#227 文件存在）纹丝不动；再补 `scripts/backup.sh` + `scripts/upgrade.sh` 并改掉
   两处旧口径，退出码才转 0；还原全部注入后回到 16 项 MISSING。

**已知的口径边界（不要把闸当成完备）**：

- `#219` 的 `HOST_BIND` 只扫代码与文档，**不含 `docker-compose.allinone.yml`**（该文件在仓库根，
  不在 `scan_files()` 的 find 路径里）。所以 #219 的 PR 合入后这一行仍可能报 MISSING——届时以实机判据为准，
  或把该文件加进 `scan_files()` 的 find 路径。
- 判据用词若与这里钉的不同，会误报 MISSING。处置是**改脚本的字面量并在本表对应行注明**，
  不是把 MISSING 当成「那条 PR 没做」。

## 5. 发版前操作顺序

1. 收评审队列：把 19 条 `needs-review` 的 PR 推到 `ready-to-merge` 或直接评审合并。
   取实时对应关系（不要抄本文档的快照）：
   ```bash
   gh pr list -R <owner>/<repo> --state open --json number,title,labels,headRefName
   ```
2. 为 #203、#217、#226 立项开工（#203/#217 还需先补进看板，否则自动领任务的循环看不见它们）。
3. 按 §2 的依赖顺序合并：A 组 → B 组 → C 组，#218 等 #202/#216/#224/#217 落 master 后再做，#228 最后。
4. 跑 `bash scripts/release-gate-v0.4.0.sh` → 应为 16 项 PRESENT、退出码 0。
5. 逐条跑 §3 的实机判据并签核（前端类条目必须在真实浏览器里看，构建 + 单测不算验证）。
6. 更新 `CHANGELOG.md` 的 `[0.4.0]` 段落并按实际写「升级须知 / 迁移水位」，把水位行改成正式日期
   ——未定稿状态会被 `release.yml` 的标题行守卫判红。同步 `docs/user-guide/configuration.md`
   （本版新增 env 不少：`AUTH_ENFORCE`、`AUTH_RATE_LIMIT_PER_MINUTE`、`HOST_BIND` 等）。
7. Actions → Release → 填 `v0.4.0`（先发 `-rc.1` 走 `prerelease=true` 更稳，见[发布流程](release.md) §2）。

## 6. 本版明确不做（照 #230，防范围蔓延）

开放注册；三档以上角色与自定义权限矩阵（属资源级 grant #150）；httpOnly Cookie 会话；
「记住我」与多设备会话列表；前端权限矩阵（哪个按钮谁能点）；分布式限流；审计日志落库（#63）；
LOD 表达式、级联筛选、地图等触发式条目。

## 7. 维护本文件

- 每条判据**合入后不要删行**，改在「静态标记」列尾加 ✅ 与合入日期——发版时要能看到全貌。
- 新增/删除条目时，同步改 `scripts/release-gate-v0.4.0.sh` 的 `rows` 守卫数字与 §3 表格，二者不一致时脚本判红。
- 版本号是本文件名的一部分：v0.4.0 发布后，把本文件改名或重写为下一版（`release-readiness.md` 是通用名，
  但内容必须是单版本的判据，避免「上一版的清单还挂着」）。
