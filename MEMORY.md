# 经验记录

## 工作方式

> **🚨 零容忍：所有 Bug 修复必须附带单元测试**

每个 bug 修复遵循严格流程，**这是强制要求，没有例外**：

1. **先写失败测试**：用单元测试复现 bug，确保测试失败
2. **再修复代码**：修改代码使测试通过
3. **确认通过**：运行测试验证修复，且不破坏已有测试

**严格禁止**：
- ❌ 禁止没有测试的 bug 修复
- ❌ 禁止删除失败测试（保留复现证据）
- ❌ 禁止提交无法测试的代码
- ❌ 禁止仅手动验证修复

这一规则确保：
- 每次修复都有自动化回归防护
- 问题复现可追溯（历史测试即文档）
- 防止同一问题反复出现

---

- 新增功能时，先定义前后端接口契约，再分别编写双端单元测试，便于问题定位
- 主动反思，当意识到自己可能犯错或需要改进时就记录
- 每次反思后将根因和教训追加到本文档
- 前端测试断言 Ant Design 空状态等通用文案时，避免直接使用 `getByText` 假设唯一节点；应优先限定容器或使用 `getAllByText`/角色断言，防止组件内部生成的同名节点导致误判。
- Go 后端对外返回的集合型字段不能依赖 nil slice/map 的默认序列化；API 契约要求空数组返回 `[]`、空对象返回 `{}`，必要时应在响应组装或 `MarshalJSON` 层显式归一化，避免前端收到 `null`。
- 当该约束适用于多个接口时，优先下沉到统一响应层处理，而不是在各个 handler/service 中零散兜底；这样能覆盖图表查询、分页结果和未来新增接口。
- 健康检查等非 `/api` 的生产响应只要返回 JSON，也应优先走统一响应封装；否则会绕过响应契约与空集合归一化。
- `@dnd-kit/core` 的 `useDraggable` 只负责拖拽状态与事件，不会自动渲染跟随鼠标移动的可视预览；需要在页面级显式维护 active item，并用 `DragOverlay` 提供拖拽中的反馈动画。
- 当前本地前端验证环境存在两类独立阻塞：`pnpm` 对 Node 版本有下限要求，以及 `vitest + jsdom` 在当前依赖组合下可能触发 `ERR_REQUIRE_ESM`。做前端 TDD 时应先确认 Node / pnpm / Vitest 运行矩阵，避免把环境问题误判为代码回归。
- 使用 Codex 工具改文件时，应直接调用 `apply_patch`，不要通过 `exec_command` 包一层 `apply_patch`。虽然当前环境仍会执行成功，但会产生明确告警，属于应主动避免的操作失误。

- ChartBuilder 的图表定义行顺序不能直接当作 `queryConfig.dimensionGroups` / `metricGroups` 下标使用；维度组和指标组是分开存储的，必须先按 kind 计算局部索引，否则会出现“UI 看起来已添加指标，但请求里的 metrics 为空”。

## Batch 1 安全与查询收敛的经验（2026-09-06）

- **参数化通道断裂的根因**：`Connection.Execute` 早期接口设计成只收 SQL 字符串（没有 args），导致 bun_builder 层辛苦收集的值参数无处可传、最终被拼回 SQL 字符串——builder 层等于白做。教训：底层执行接口的签名决定了上层所有安全投入能否生效；先打通“接口能传参”，再做“上层会传参”，顺序不能反。修复时以 `Execute(ctx, sql string, args ...any)` 为锚点，从 builder 收集 args → executor 透传 → 驱动执行逐层贯通并补测试。
- **go.mod 依赖连带升级是确定性副作用**：引入 goose / sentry-go / bcrypt 等新依赖时，`go mod tidy` 常会连带升级既有间接依赖（如 bun、gin 相关），这不是意外而是必然副作用。应在引入新依赖的提交里预期并审查这些升级，避免把“意外的依赖升级”当作回归排查。

## Batch 2 泛型 router 接线经验（2026-09-09）

- **“设计已就绪但从未接线”的包，其自身测试全绿 ≠ 行为正确**：`internal/router` 启用前 6 个测试全部通过，却掩盖了两个会让迁移直接破坏契约的缺陷——`Response` 按值传入 API handler 导致所有 `res.Out` 写入被丢弃（success 响应永远是零值），以及 JSON 绑定门 `ContentLength > 0` 与旧 handler 无条件 `ShouldBindJSON` 不等价（空 body POST 会从 400 EOF 变成落库脏数据）。根因：旧测试只断言 HTTP status/code，从未逐字节断言 data。教训：接线任何休眠包之前，先对被迁移端点建 httptest 逐字节响应基线（迁移前后同一断言必须全绿），并给休眠包补“断言最终输出内容”的测试。
- **gin 绑定的两个静默陷阱**（迁移端点时必须处理，均有实测依据）：① `ShouldBindQuery` 对无 `form` tag 的字段回退用 Go 字段名做 key，router 又总是先绑 query，因此 JSON body struct 必须逐字段 `form:"-"`，否则 `?Name=evil` 会污染 body 值；② query 参数若用 int 直接绑定，`?limit=abc` 会变成 400，而旧 handler 是忽略解析错误用默认值——零行为变化要求 query 字段绑成 string、在绑定后方法里复刻 DefaultQuery+Atoi 容错+clamp。
- **逐字节一致要盯序列化细节**：`map[string]any` 的 JSON key 按字典序、struct 按声明序，投影方式改动会改变响应字节；`response` 包 normalizer 已把 nil slice/map 归一化为 `[]`/`{}`，迁移时勿据此“顺手”删掉旧 nil 兜底以外的逻辑。命名 In struct 会让 JSON 类型错误的报错信息带上类型名（匿名 struct 时为空），属设计强制的不可消除差异，应显式裁定并记录而非默默接受。
- **裸数组 body 端点要用匿名切片类型当 In**（Task 5 dataset UpdateColumns 实测）：`[]entity.DatasetColumn` 直接作泛型 In 时，元素类型错误文本保持 `.0 of type entity.DatasetColumn`，与迁移前 `ShouldBindJSON(&columns)` 逐字节一致；若命名为具名切片类型，错误会带上类型名反而破坏一致。`ShouldBindQuery` 对非 struct In 是 no-op（gin v1.11 `tryToSetValue` 空字段名直接返回），无需 `form:"-"`。
- **共享领域实体不能当泛型 In**（Task 5 dataset Query 裁定）：`entity.QueryConfig` 无法携带 `form:"-"`（跨包领域类型），直接作 In 会让 `?Limit=…` 经 ShouldBindQuery 回退字段名注入 body——比“struct 名进入 json 错误文本”（diff #1）更严重的契约破坏。做法是 handler 本地镜像 struct + `form:"-"` + 转换函数，代价是镜像需与实体手工同步、错误文本 struct 名变化要钉测裁定。

## Batch 2 Task 7b chart-config v1 持久化经验（2026-09-09）

- **组件内重复 `await import('...')` 在 vitest 下可能第二跳逃过 mock 直连真实服务**：ChartBuilder 加载 effect 把 `chartBuilderFields` 加入依赖后会跑第二遍，第二遍的 `await import('../api')` 返回了未被 `vi.mock` 包装的原始模块，请求打到了本机 8080 正在运行的 dev 后端，用真实 seed 数据（`New Chart`/bar/`field-0`）覆盖测试 store，3 个页面测试出现无法从 fixture 解释的 received 状态。根因是"effect 重跑 = 网络请求重发"的隐含假设，叠加 mock 解析不稳定。教训：由依赖变化触发的重跑 effect 若要复用一次性数据，必须按 key 缓存请求 Promise（本次用 `useRef<{id, promise}>`），失败时清缓存允许重试；页面级 mock 测试要警惕本机同端口真实服务，received 与任何 mock 都不符时先查网络侧。
- **fieldId 从位置 id 改为列名后，旧位置 id 只能靠"列顺序重建映射"解析**：`migrateChartConfig` 的 resolver 以 `field.id` 为键，而运行时 `id === name` 之后映射里不再有 `field-N` 键；加载旧配置时必须传入 `chartBuilderFields.map((f, i) => ({id: `field-${i}`, name}))` 的位置映射才能把 `field-N` 解析为列名（v1 列名解析不到时原样保留，无损）。这属于契约文档未明说的缝隙，实现时要在代码注释里写死这个重建规则，否则会被后人"优化掉"。
- **antd Button 带图标时 accessible name 含图标的 aria-label 前缀**（如 `save 更新`），`getByRole('button', {name: '更新'})` 精确匹配会失败，须用 `/更新$/` 锚定——与该文件既有测试 `/饼图$/` 的写法一致。

## Batch 2 收口经验（2026-09-09）

- **"消费生成类型"不是机械替换，推迟到 Batch 3 是对的**：生成 `*Response` 是信封包装、手写同名类型是裸 payload，同名不同义按名字替换会静默改变 `.data.data` 取值深度；`DatasetColumn.typeConfig`(camel)↔生成 `type_config`(snake) 命名冲突（后端实测返回 snake，说明手写 camel 本身可能是既有 bug）；生成物把窄联合压成 `string`，而现有 `as` 断言依赖这些联合，故正确做法是"生成类型打底 + 薄手写联合层 Omit 覆盖"而非整体替换。教训：spec-first 里"生成类型"与"运行时消费生成类型"是两件事，前者是契约基线、后者是有静默风险的大迁移，不要在批次末尾仓促做。
- **全分支评审能抓到"逐任务评审各自局部正确、只有组合才暴露"的文档漂移**：本次两处 Important 都不是代码 bug——Task 3.5 修了代码 `op→operator`、Task 10 改了 AGENTS/architecture，但没人把"提到 filter 字段的文档（api.md）"对着修好的 wire 重读，api.md 示例仍是 `op`（抄它的人会重新引入静默降级 bug）；AGENTS.md 我自己写"31 端点全部经 router"，但 health 是裸 r.GET、View 是 302，实为 30+2，与同段自己的例外清单自相矛盾。教训：文档同步要把"命名了 wire 字段/端点数的文档"纳入同一份 diff 审查；批次收口做一次跨文件一致性扫描，别只信逐任务的局部一致。
- **删除"仅覆盖被移除死代码的测试"是正当清理，但要在 commit message 里明确区分于"删失败测试"**：dialect.go 旧字符串 builder 的专属测试（dialect_test.go、BenchmarkOldBuilder_BuildSelect）随死代码一起删，必须写清"这些测试当前是通过的、只因所覆盖的生产代码被删才移除"，否则触碰零容忍红线。执行前先 grep 每个待删符号确认零活跃调用者——Task 9 的 subagent 正确命中 STOP 规则，抓到了主评审漏判的 `NewMySQLBuilder().BuildSelect` 测试引用。
- **IAB 里 AntD 交互两条实测规避**：① `getByRole('button',{name})`/combobox 的 Playwright 动作常因 antd 把 name 带上图标 aria-label、或 actionability 判定超时而失败——改用 `evaluate` 直接 `btn.click()`（触发 React handler）或坐标 `cua.click`；② `tab.screenshot()` 会偶发 `screenshot activity capture failed for guest`，验证 canvas 是否真渲染改用 `getImageData` 采样非白像素计数（本次证明柱状图确实绘制），不依赖截图。
- **验证 mock 测试是否"偷偷"依赖本机真实服务**：把后端端口 kill 掉再跑全量测试，若通过数不变才是真·隔离测试。Task 7b 的 Promise 缓存修复正是这样被证明有效（:8080 关闭后仍 68/4）。
- **发现即记录的既有产品 bug 不塞进重构批次**：DatasetEdit 保存调用不存在的 `PUT /api/datasets/:id`（后端无路由，实际会 404）属功能缺陷，与"泛型 router 重构"无关，正确做法是记入 Batch 3 backlog 而不是顺手补后端端点，避免重构批掺功能开发扩大爆炸半径。

## Batch 3 收口经验（2026-09-12）

- **把"响亮失败"修成"成功写入"时，必须重审该 API 的全部调用方**：补上 `PUT /api/datasets/:id` 前，列表页内联重命名一直在 404（响亮但无害）；端点一通，其 payload 不含 `shard_enabled`，被新 handler 以零值覆盖——静默关掉用户的分片配置。教训：ghost-route 类修复改变的是所有既有调用方的命运，落地当天就要枚举 caller 核对字段覆盖（本项由全分支评审抓出，逐任务验证不可见），修复用"调用方透传未管理的字段 + 真实渲染路径的交互测试（断言用非默认值，防歪打正着）"。
- **守卫/parity 类测试要做反证**：json-tag parity 守卫补 subset 行时，用两个 scratch 漂移（改 tag 名→FAIL extra、改字段类型→FAIL type mismatch）证明它能抓回归，再 revert——只跑绿的守卫等于没有守卫。
- **E2E 状态目录别放 /tmp**：macOS 清理 3 天未访问的 /tmp，PG 集群连 PG_VERSION 一起没（不可恢复）。测试数据目录用 `~` 下持久路径；重建配方已验证（initdb→seed→API 重建 fixture，约 1 分钟）。
- **IAB webview 帧循环可整体节流至 rAF=0**（ZCode 窗口不在前台时）：ECharts 容器有实例属性但 canvas 永不 paint、screenshot capture 失败——先 evaluate 数 rAF 帧 + 多页面对照，确认是环境再下结论，别误判成渲染回归；像素级验证需用户前台窗口。

## Batch 4 B4-D2 分享取数语义统一经验（2026-09-12）

- **管道内既有 quirk 会被"语义统一"原样复制，别把它当新 bug 修**：executor 生成的 `SUM(amount) AS Revenue`（带大写字母别名）被 Postgres 折叠为 `revenue`，processor 按 `Revenue` 取行值取不到 → axis series 全 null。该缺陷早已存在于 builder 预览（POST /charts/query）；B4-D2 让 GetData 复用同一管道后，分享页与预览"一致地 null"——这正是本任务要的 parity。若要真修得动 query 包（标识符加引号或大小写不敏感取值），属另一张票，勿在统一语义的改动里顺手夹带。
- **对照实测两个端点的 wire 输出是验证 parity 最便宜的手段**：live :8080 上 `GET /charts/1/data` 与等价 `POST /charts/query` 逐字段比对，比任何单测都直接回答"分享是否等于预览"；重启后端前先确认原进程的启动方式（`ps -o command=` 看启动参数），照原样起回去。

## Batch 4 收口经验（2026-09-12）

- **"语义统一"类改动要先审存储默认值再定透传规则**：B4 评审组合发现 preview/share 的 `limit` 不对称——store 给所有新图表默认写 `limit:1000`，预览只对 table 发分页、分享对全类型透传 limit，三处各自正确、合起来在 >1000 分组时截断不一致。教训：跨端一致性任务动手前，把"数据在生产端（store 默认值/持久化）的真实形态"拉出来看一眼，别只看消费端代码；"parity"承诺要精确到每个字段的通道规则并写进函数注释。
- **修复会把潜伏路径变成活路径**：D2 让分享页走聚合管线后，原本不可达的"table 无分页 → TableProcessor 列序随机 + total 误计"分支一夜转正。凡是"让某条 dormant 管线首次被真实使用"的改动，评审必须专门问：这条管线里哪些分支以前从没被执行过，现在会。
- **像素级渲染验证依赖前台窗口，行为验证不依赖**：IAB rAF=0 时 canvas 永不 paint，但 echarts 实例挂载与否、`.ant-result` 有无、DOM 文案、live curl 的 wire 数值足以证明渲染逻辑正确——把"代码正确性证据"与"肉眼可见证据"分开记录，别让环境限制把验收降级成猜测；rAF 探针（evaluate 数帧）是鉴别节流 vs 回归的最便宜手段。

## `make dev` 起不来排查（2026-09-16）

- **"跑不起来"往往不是一个问题**：这次是两件事叠在一起——① `backend/etc/config.toml`（未被 git 跟踪，纯本地文件）指向的 LAN Postgres `192.168.10.81` 主机已关机，后端 `InitDB` 失败即 `os.Exit(1)`；② 旧 `dev: dev-backend dev-frontend` 是串行 prerequisite，后端进程不退出前端就永远轮不到启动。只修任一个都仍会看到"卡住没反应"，所以先分别验证前后端各自能否起来，再谈根因。
- **后端连不上库时表现为静默退出**：`dial tcp ...: connect: host is down` 之后没有任何端口监听，容易误判成"air/端口占用"问题。定位手法：`go build -o /tmp/x ./cmd` 后直接前台跑并 `perl -e 'alarm 20; exec @ARGV' ...; echo EXIT=$?` 拿真实退出码（macOS 无 `timeout`），再配合 `lsof -nP -iTCP:8080 -sTCP:LISTEN` 与 `nc -z -G 5 <host> 5432` 区分"进程死了"还是"依赖没起来"。
- **`make dev` 并行启动要靠 `set -m` + 进程组 kill，直接 `kill $pid` 会留孤儿**：EXIT trap 只对 `pnpm dev` 的父 subshell 发 SIGTERM 时，真正的 vite node 进程会脱离继续占 3000 端口。bash `set -m` 让两个后台任务各自成组，`kill -TERM -<pgid>` 才能连带收掉 pnpm→node；轮询用的 `kill -0` 需 `2>/dev/null`，否则未回收任务会往输出泄漏 "No such process"。
- **本机 Homebrew postgresql@16 有两个坑**：直接 `pg_ctl start` 在 LANG/LC_ALL 为空的 shell 里会 `FATAL: postmaster became multithreaded during startup`（HINT 让设 LC_ALL），带 `LC_ALL=en_US.UTF-8` 可起；改由 `brew services start postgresql@16` 走 launchd 则不受影响且随登录自启。空集群首次用需自建 `insight` 角色 + `insight-dev` 库，表结构由后端启动时 `database.RunMigrations`（goose embed）自动建，无需手工导 schema。
- **杀"疑似诊断残留"的进程前必须先确认它属于谁**：`pgrep -f "^air --build"` 命中的旧 pid 82273 其实是用户终端里一直在跑的 `make dev`——它的 `server` 子进程因 DB 挂了早已消失，air 处于空转等待，看起来像僵尸。我 kill 它导致用户的会话被打断（其 make 随即按旧串行逻辑起了个落在 3001 的前端）。教训：清进程前用 `ps -eo pid,ppid,lstart,command` 反查父子链与启动时刻，确认是自己某次 eval 的后代才动手；无法确认归属就先问，别把用户的运行态当垃圾扫掉。

## 数据源页 "Network Error" 排查（2026-09-16）

- **"页面报错了，没有数据"是两个独立问题，先分开再动手**：报错是 CORS（见下），没数据是**连错了数据库**——`config.toml` 的 Url 指向本机 Homebrew 5432 的空库（今天为绕开挂掉的远端而新建），而有数据的 `~/dataray-pgdata`（5433）集群默认不运行。症状叠在一起时容易只修一个就宣布完成；这里先 `curl` 打接口确认后端返回的是 `data:[]`（200 正常）还是网络失败，再决定查哪边。
- **CORS 拦截的表现是 axios 的 "Network Error"，而后端日志完全正常**：前端 `lib/api/client.ts` 用 `http://${window.location.hostname}:8080` 推导 API 基址（本意是支持任意主机访问），但后端 `[CORS] AllowedOrigins` 是精确白名单。用 `data.localhost:3000` 打开时 Origin 不在名单里 → 中间件不发 `Access-Control-Allow-Origin` → 浏览器丢弃响应，axios 只能报 "Network Error"。最便宜的判定：`curl -s -D - -o /dev/null -H "Origin: http://<host>:3000" http://<host>:8080/api/datasources | grep -i access-control`，出现 `Vary: Origin` 却没有 `Access-Control-Allow-Origin` 就是它，不必去浏览器抓包。注意 `*.localhost` 全部解析到 127.0.0.1、vite 又监听 0.0.0.0，所以任何非 `localhost` 的主机名都会复发——往配置里加一条精确 origin 只是权宜，根治要让中间件放行回环别名。
- **air 只在源码"内容"变化时重建，改配置/碰 mtime/kill 子进程都不行**：`make dev` 下 air 默认只监视 `.go` 等扩展名，改 `etc/config.toml` 永远不会生效；`touch` 一个 .go 文件（只变 mtime）也不触发；**kill 掉 `server` 子进程 air 不会重新拉起**，只会空转等待（这正是上一节那个"像僵尸的 air"的成因，我这次又复现了一次）。真正有效的触发是往 .go 文件写真实内容——本次临时建 `backend/cmd/air_trigger_tmp.go` 让 air 重建，重启后再删掉它（删除会再触发一次重建，两次都读同一份新配置，结果一致）。另外 air 本身不能 kill：`Makefile` 的包装是 `while kill -0 $be && kill -0 $fe`，air 一死 EXIT trap 会连带把前端一起收掉。
- **配置生效要显式重启，验证要挑对探针**（同日补充）：`etc/config.toml` 改动 **不会** 让 air 重启后端；对 `.go` 文件 `touch` 也不会（macOS 上 air 走 FSEvents，纯 mtime 变更无写事件），得做"内容不变的真实写入"（`perl -0pi -e '' backend/cmd/main.go`，不产生 diff），判据是 `backend/server` 二进制 mtime 与 pid 同时变化。另：`GET /health` 是裸 handler、不碰数据库，永远 200，不能作为"库连上了"的证据——用 `/api/datasets` 等真实接口，其错误串会直接印出 `user=... database=... host:port`，是确认实际连接目标最便宜的手段。当天开发库最终切到 `postgres://postgres:postgres@192.168.10.70:5432/insight_dev`（该 PG 18 实例上 `insight_dev` 为新建库，注意是下划线，不是本机那套连字符 `insight-dev`）。
- **`set -m` 并行起 dev 会让 vite 被 SIGTTIN 挂起，必须 `< /dev/null` 断掉 stdin**（同日返工）：把 vite 放后台进程组后，它为自己的"按键盘交互"去读终端 stdin，内核随即以 SIGTTIN 停住整个前端进程组——表现极迷惑：进程活着、端口 `LISTEN`、但请求一律超时无响应（`ps` 里状态是 `T`，不是 `S`）。修复是给两个后台任务都加 `< /dev/null`；验收口径：`lsof -p <vite pid>` 看 fd `0r` 是否 `CHR /dev/null`，并确认进程表里没有 `T`。反面教训：对 `T` 状态的进程直接 `kill -CONT` 唤醒，它读完 stdin 会立刻退出，进而触发我写的 EXIT trap 把后端一起收掉，等于把用户的 dev 环境整个停掉——唤醒挂起的 dev 进程前要先想清楚它恢复后的下一步是什么。

## 全量依赖升级（前端 major 拉最新 + Go 依赖 minor，2026-09-17）

- **升级顺序：先 `pnpm update --latest` 一把装齐，再用 `tsc --noEmit` 当错误清单逐个修**，比边升边修省很多往返。本轮实际破坏点全在类型层可发现：TS7 删了 `tsconfig` 的 `baseUrl`（paths 直接相对配置文件写）；react-router 7 删了 `BrowserRouter future` prop；antd6 `Dropdown.overlay`→`menu={{items}}`、`Space.direction`→`orientation`、`Drawer.width`→`size`、`destroyOnClose`→`destroyOnHidden`（vite8 原生 `cssSideEffectImports` 后 CSS 副作用导入需 `src/vite-env.d.ts` 的 `/// <reference types="vite/client" />`，本仓库一直没有）。
- **批量全局替换会误伤同名 props**：一次 `width=` 全局 sed 把 Modal 的 `width` 也改成了 antd6 已无的 `size`——Modal 的 `width` 在 v6 未废弃，只有 Drawer 废弃。改法必须先按组件上下文逐处看，替换断言（`assert a in s`）只保底存在性、不保底语义。
- **测试基建两类隐性依赖**：jest-dom 7 要 `import '@testing-library/jest-dom/vitest'` 才注入 vitest 断言类型（裸导入只剩类型缺失、37 处 TS2339）；jsdom 30 仍无 `ResizeObserver` 而 antd6 的 rc-resize-observer 直接 `ReferenceError`，测试 setup 需自带 stub。
- **deprecation 警告会伪装成断言失败**：ChartBuilder 有个"禁止 console.error"的用例，antd6 的 Space/Drawer 废弃警告把渲染链路上所有用例连坐打挂——排查时先读断言消息里 warning 原文，它就是待修清单，不用猜。
- **`make dev` 起的 vite/air 持有的是启动那一刻的依赖树**：升级后必须整组重启（确认进程归属后 TERM `make dev` 包装进程，vite 与 air 的子进程组会被 trap 收掉），否则浏览器冒烟测的还是旧版本。

## Ponytail 死代码清理（2026-09-19）

- **删除项**：后端 `service/queryrecord` 整包、`query/sanitizer.go`、v1 适配器 `ChartSpecFromRequest` + `defaultDimGroupName/defaultMetricGroupName`、误构建二进制 `backend/server.new`；前端 `client.ts` 死常量（API_CODE→私有 API_SUCCESS_CODE、PageResult、ApiCode）、`api/index.ts` 三个负载判别函数（isHistogram/isRadar/isBoxplot，仅测试引用）与 `executeQuery`（v1 死链）、store 死面（fetchQueryData/setSelectedChart/selectedChartId/clearError）、`datatypes.ts` 322→104 行（只留 StandardDataType+toStandardType）；10 处 `new Date().toLocaleString()` 复制粘贴收敛为 `lib/format.ts#formatDateTime`。
- **审计报告要抽查再动手**：两份 subagent 审计各有误报——`get/post/put/del` 包装层、`generateRequestId`、`ChartConfigQuery/ConfigFieldGroup` 实际都在用或已不存在；`DEFAULT_CHART_STYLE` 只需去 export。每条"死代码"结论都先 grep 全仓确认再删，测试引用不算生产引用但删函数要连测试一起删。
- **删除后连注释一起清**：`processor*.go`/`dialect.go`/`pivot_correctness_integration_test.go` 里多处注释引用已删符号（ChartSpecFromRequest/defaultDimGroupName/queryrecord 约定/sanitizer 兜底），语义仍有价值就改写措辞、纯历史引用就删句。
- **dayjs 不删**：antd RangePicker 的 `disabledDate(current: Dayjs)` 类型本身来自 dayjs（antd 强依赖），删直接依赖不减少安装体积还断了类型导入来源。
- **遗留坏测试顺手修**：`DatasourceDetail.test.tsx` 在 HEAD 上就挂（缺 IntlProvider + 断言已过时的 "Total: 0 rows"——antd 空表不渲染分页条）；`chartOptions.test.ts` 的 PieOptionView 缺 `color?` 字段（工作区未提交改动遗留的 tsc 错误）。`pnpm build`（tsc 全量）此前没人跑过，所以这些一直在逃。

## 过滤器改造为「过滤字段组」（2026-09-19）

- **动手前先核三件事，省掉整轮猜测**：① 拖拽管道**本来就支持**过滤落点（`FieldDropZone` 有 `filter` 分支、`handleDragEnd` 有 `dropZoneType === 'filter'`），旧 `FilterBuilder` 只是没接上；② 后端 `bun_builder.go` 对 `logic` 非 `OR` 一律按 AND → **后端零改动**，前端恒发 `and` 即可；③ 直连 `insight_dev` 核库发现 **14 个已保存图表 `filters` 全空** → 把过滤改成恒「且」不存在历史 OR 语义静默变更的风险。**"这次改动要不要迁移历史数据"必须先用真实库验证，不能靠推理**。
- **同构交互 ≠ 同构数据结构**：过滤条件不是「绑定」（每条自带操作符与值），复用 `FieldDropZone` 会把组件变成 `if (zoneType === 'filter')` 分支怪；但视觉必须同源，所以复用的是**外壳**（`QueryConfigRow` 新增 `children` 插槽）与**样式常量**（`dropZoneStyles.ts` 的 id/容器/三色板），不是内部实现。`dropZoneId()` 抽成共享函数是刚需——两个组件各写 `dropzone-${type}-${index}` 一旦撞名会互相抢命中。
- **图型无关的能力不要塞进 `chartDefinitions.fieldGroups`**：过滤是所有图型都有的公共行，写进图型定义会让 14 个定义各重复一项，并把 `FieldGroupKind` 逼成三分支、牵动 `getActiveFieldGroups`/`normalizeQueryConfigForChartType` 的二分逻辑。**"统一到同一套 UI 体系"与"登记到同一份能力清单"是两件事**。
- **`Date.now()` 造 id 在"连加"场景必炸**：区间筛选就是同字段连拖两次（`>=`、`<=`），同毫秒内两条条件会拿到同一个 id，按 id 更新/删除随即误伤另一条。前端造业务 id 一律 `crypto.randomUUID()` + 时间戳兜底（与 `X-Request-ID` 同一套路）。
- **纯图标按钮必须补 `aria-label`，否则测试静默失败**：`<Tooltip title={x}><Button icon/></Tooltip>` 里 Tooltip 的 title **不参与 accessible name 计算**，`getByRole('button', { name: /饼图/ })` 直接找不到元素。本次 `ChartBuilder.test.tsx` 有 3 例因此挂了不知多久（改了按钮外观、没人跑测试）。**改 UI 外观时，凡"按钮文字挪进 Tooltip/图标"都要顺手加 aria-label**。
- **工作区有大量未提交改动时，`git stash` 拿不到干净基线**：想验证"这几个失败是不是我引入的"，stash 回 HEAD 只会得到"6 例失败 + 20 errors"（HEAD 自己就落后于工作区），毫无参考价值。可靠做法是**读根因 + 最小修复实验**——本次给按钮加一行 `aria-label` 后 3 例立刻转绿，因果当场坐实。**先证明"这是既有问题"，再决定修不修；不修就无法证明自己的改动无回归，所以阻挡验证的既有失败要修掉并说明**。
