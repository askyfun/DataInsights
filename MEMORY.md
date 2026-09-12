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
