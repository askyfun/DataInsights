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
