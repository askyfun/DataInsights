# 排障（Troubleshooting）

> 最后更新：2026-09-26
> 收录「外部开发者 clone 本仓库、照文档部署后**也会踩到**的坑」，按场景 TOC 查阅。只收"不做记录就会再踩一遍"的坑。
> 每条按「**现象 → 根因 → 判据 / 处置**」写，力求**可独立阅读**（不假设你看过任何本机笔记）。

## 目录

| 场景 | 章节 |
|------|------|
| 升级依赖 / 装包 / 查版本 | [版本与依赖](#版本与依赖) |
| 改配置 / PORT / Docker / 静态托管 | [配置与部署](#配置与部署) |
| 手搓 curl 图表查询 / 前后端联调 | [API 契约](#api-契约) |
| 改软删 / 级联 / 落库 | [架构红线](#架构红线) |
| 改分享短码 / 查询记录 | [查询记录与分享短码](#查询记录与分享短码) |
| 改图表槽位 / 加图型 | [图表槽位](#图表槽位) |
| 写前端组件 / 弹窗 / 测试 | [前端陷阱](#前端陷阱) |
| 改日期筛选 / 动态日期 / 盘级区间 | [日期筛选](#日期筛选) |
| 前端 UI 约定 / 文案 | [前端约定](#前端约定) |
| 连 StarRocks / 导数据 | [StarRocks 与其他外部库](#starrocks-与其他外部库) |
| 跑全量回归 / 验证某提交能否独立构建 | [工具与验证](#工具与验证) |

## 版本与依赖

- 钉死版本：Go 1.27.1（`go` 指令写全补丁号）· pnpm 12.4.2 · Node 24（**刻意不升 26**）· antd 6.6.4 · biome 2.5.14。升 biome **必须**同步 `biome.json` 的 `$schema`，否则报 `deserialize`。
- 产品名为 **Data Insights**；历史名 `dataray` 有意保留在少数标识符与默认名里（例如业务库默认名 `dataray_data`），改名时别顺手清掉。
- ⚠️ pnpm 12 会把 **pnpm 自己**写进 lock（每份 lock 多约 158 行），且首次 install 有一次 supply-chain 校验（可能持续数分钟）—— **默认行为，不是异常**。
- ⚠️ **清孤儿依赖**：lock 未变时 `pnpm install` 会返回 `Already up to date`，`--force` / `pnpm prune` 都无效。**正解 = 删掉状态文件再 install**：`rm -f node_modules/.modules.yaml node_modules/.pnpm-workspace-state-v1.json && pnpm install`。判定某包是否孤儿用 `pnpm why <pkg>@<ver>` —— 孤儿是**完全静默**的，须配对照组。
- `idls/gen_types.*` 不含 `info.title` → 改 title 不必 `make api-gen`；改 **description** 会漂进生成物 → **必须重跑**。
  - ⚠️ **根因**：`make api-gen` 的**前端那一步当前必然失败**（不是 PATH / 环境问题）：`openapi-typescript` 把 `typescript` 当**值**用（`ts.factory.createKeywordTypeNode`），而本仓的 `typescript@7` 里 `ts.factory` 是 `undefined` → `TypeError: Cannot read properties of undefined`。崩溃发生在写文件**之前**（生成物不会被写坏）。
  - 绕过：用一份**自带 TS 5.x** 的 `openapi-typescript` 生成器运行，CWD 在仓库根：
    ```bash
    node <openapi-typescript（自带 TS 5.x 的那份）>/bin/cli.js \
      api/openapi.yaml -o frontend/src/idls/gen_types.ts
    ```
  - 后端那半步正常：`go run .../oapi-codegen -config ../api/gen/backend.cfg.yaml ../api/openapi.yaml`。
  - 重跑后**务必 diff 生成物**，确认只漂了你改的那几处（例如若干处 description 与新增 schema 字段）。彻底修法是把生成器与 `typescript@7` 解耦（给它自己的 package.json / 钉一个 TS 5 别名），属独立改动。
- **查镜像 / 语言版本**（网络受限时）：镜像 tag 用 `mirror.gcr.io/v2/library/<repo>/tags/list`；Go 版本用 `go.dev/dl/?mode=json&include=all`。⚠️ zsh 下 URL 带 `?` 必须加引号。

## 配置与部署

- **唯一配置来源 = 环境变量**；`backend/etc/` 已删，**别再引入 config.toml / yaml**。优先级：内置默认 < 根 `.env` < 真实 env；键名清单的唯一事实源是 `.env.example`（后端 6 个**无前缀**，前端只有 `VITE_SENTRY_DSN` / `VITE_API_BASE_URL`）。**空串 = 未设置**。
- ⚠️ **监听地址不是配置项**，固定 `0.0.0.0`；`HOST` env **有意忽略**且有用例钉死 —— 别加回来。
- ⚠️ compose 通过根 `.env` 做变量插值（若存在）；所有运行时变量都有默认值，**不强制要求 `.env` 文件**；**改 `PORT` 已自动同步到 `docker-compose.allinone.yml` 的 `ports`**，无需手工改映射。
- ⚠️ godotenv **不覆盖已存在的 env**（"真实 env > `.env`"就靠它），别改 override。⚠️ 测试里 `t.Setenv(k,"")` **≠** unset（会挡住 `.env` 值）→ 走 `LoadDotEnv` 的用例必须用 `os.Unsetenv`。
- ⚠️ `VITE_*` 是**构建期内联**，改完必须重 build，**绝不可放密钥**；为空则**完全跳过 `Sentry.init`**。它与后端 `SENTRY_DSN` 是两个独立项目。
- 根 `Dockerfile` 三阶段（`backend/`、`frontend/` 各自的 Dockerfile 与 nginx.conf 已删），**构建上下文 = 仓库根**；⚠️ `.dockerignore` **必须排除 `.env`**，否则 Vite 会把 `VITE_*` 内联进浏览器包。
- `webui` 是分流唯一实现，挂在 gin 的 `NoRoute` 上：`/api`、`/mcp`、`/health` 按**路径段边界**匹配 → 未匹配回 **JSON 404**，**绝不回落 HTML**；其余走 `fs.ValidPath` + `fs.Stat`，未命中回落内存里的 `index.html`。⚠️ **穿越防护靠 `fs.ValidPath`，别换成 `filepath.Clean`**。
- ⚠️ **`/share/:token` 只在纯 API 模式注册**：托管前端时必须让给前端路由（分享页是 SPA 的 `/share/:token`；后端插同路径的 302 会把人重定向到 `/#/share/<token>`，前端是 **BrowserRouter** → 只渲染首页）。`ShareHandler.View` 与其测试保留未删。
- **不设 `STATIC_DIR` = 纯 API 模式**：`/api/nope` 回 gin 默认 404（**不是** JSON 信封）。本地验证单进程形态用 `make serve`；⚠️ 若宿主已占用 23352，需换一个空闲端口。
- ⚠️ **`preinstall` 钩子会让 Dockerfile 的依赖层失败**：`RUN pnpm install` 前必须已 COPY `frontend/scripts/only-pnpm.mjs` → 依赖层要一起 COPY `package.json` + `pnpm-lock.yaml` + `pnpm-workspace.yaml` + `scripts/`。漏 `scripts/` 会报 module not found，容易看错方向。
- ⚠️ **后端连不上库时是"静默退出"、不留监听端口**：`dial tcp <host>: connect: host is down` 之后没有任何端口监听，极易误判成 "air / 端口占用"。定位手法：先 `lsof -nP -iTCP:23352 -sTCP:LISTEN` 看有没有进程；再前台跑后端拿真实退出码（macOS 无 `timeout`，用 `perl -e 'alarm 20; exec @ARGV' ./bin/server; echo EXIT=$?`）；`nc -z -G 5 <host> 5432` 区分"进程死了"还是"依赖没起来"。
- ⚠️ **`GET /health` 是裸 handler、不碰数据库，永远 200，不能当"库连上了"的证据**（`ShareHandler.View` 同例）。验证连通要打真实业务接口（如 `/api/datasets`），其错误串会直接印出 `user=... database=... host:port`，是确认实际连接目标最便宜的手段。⚠️ 联动陷阱：运行中的后端常是**旧产物**，旧二进制按旧契约解析新库数据会整片报错 —— 改完后端**先重建再 curl**。

## API 契约

- ⚠️ `POST /api/charts/query` v1：请求用 `dims`（**不是** `dimensions`）+ `metrics[].agg`；过滤符是 **`gte` / `lte` / `neq`**，写成 `ge` / `le` 会**静默退化成等值**；过滤项必带 `id` + `logic`。⚠️ `metrics[].alias` **前端从不发**（它进 SQL 当列别名，中文会被净化成 `_invalid_identifier` 使整列为 `null`）→ 手搓 curl 别加。
- ⚠️ **`bindingId` 是全系统配置索引键**（别名 / 聚合 / 单位 / 格式等多个 Record）→ 搬字段时必须**保留 `bindingId`**。
- ⚠️ **表格分页总数**取自 `Rows[0]["_total"]`（`countSQL` 恒返一行）；按 `len(Rows)` 取会恒为 1。
- ⚠️ **antd Table 受控排序**：`sortOrder` 一旦受控，antd 会忽略本地 state；取消排序走 legacy 分支时，回调里的 `sorter.field` / `column` 均为 `undefined`。
- v2 槽位若不属 `{indicators / series_group / rows / columns}`，后端会**静默回退平表格**。建资源前先 `GET` 复查。
- 契约单一事实源是 `api/openapi.yaml`；生成的 `*Response` 是**信封包装**（裸 payload 对应去后缀类型）；handler 的 In/Out 是 handler-local 镜像（json tag 由 `contract_parity_test.go` 守卫）——详见 `AGENTS.md`。
- ⚠️ **列表接口必须显式 `ORDER BY`**：不写排序时 PostgreSQL 返回的是**堆物理序**，而 `UPDATE` 会把新行版本追加到堆尾 → 凡是编辑过的行都会漂到列表最后（症状："列表顺序随机 / 编辑完就跑到最后"）。`dashboard`（`created_at DESC, id DESC`）与 `share`（`id DESC`）一直是显式的，**dataset / chart / datasource 曾漏**（dataset 已修，chart / datasource 未改）。⚠️ 排序键**必须带唯一列兜底**：只按时间排、且多行 `created_at` 相同时会**同键抖动**。
- ⚠️ **"信封套两层"陷阱**：生成物里的 `*Response` 是**完整信封**（`Envelope & { data: ... }`），而 `lib/api/client.ts` 的 `ApiResponse<T> = Omit<Envelope,'data'> & { data: T }` **自带一层信封**。故 `apiClient.get<ApiResponse<T>>` 的泛型只能传**裸 payload**，即 `G['XxxResponse']['data']`；直接传 `G['XxxResponse']` 会套成两层，`res.data.data` 拿到的是信封而不是 payload（症状：`Property 'xxx' does not exist on type ...`）。

## 架构红线

- **不真删**：5 张业务表用 `deleted_at` 软删，**显式级联 + 同一事务**，`deleted_at` 不出 JSON；查询记录表 (`bi_query`) **永不物理删**。
- ⚠️ **级联软删的子查询故意不带 `deleted_at IS NULL`**（父行刚软删，加过滤会断链）——最易被"顺手优化"改错。已在代码现场注释：`service/dataset/impl.go`、`service/datasource/impl.go`。
- **`owner_id` / `tenant_id`**：目前只加字段、不加鉴权；仅 `bi_query` 已带列，其余 4 张业务表待账号体系阶段补齐。**CORS 默认放开所有来源是有意为之**（无登录态，不构成安全边界）→ 别"顺手收紧"。
- **PG 部分索引谓词必须 IMMUTABLE** → `WHERE expires_at > now()` 这类谓词建不出索引；**键不能选谓词列**（键恒 NULL 会退化成常量）。
- **不引入后台定时任务**。⚠️ 原"进程内有界队列"**未落地**：`bi_query` 落库实为**前端查询成功后同步 POST**，**别再按队列方案改**。
- ⚠️ **bun upsert 两个致命坑**（代码现场：`service/queryrecord/impl.go` + 哨兵测试）：
  - bun 会给插入目标表加别名 → 在 `On("CONFLICT ... DO UPDATE")` 里写 `COALESCE(EXCLUDED.x, bi_query.x)` 会判 **42P01**。正解：按需拼 SET，没上报的列不进 SET。
  - 兜底自赋值**绝不能用主键**：`query_id = EXCLUDED.query_id` 会把存量行的主键改成新 UUID → 已发出的短码当场失效。
  - ⚠️ **sqlmock 抓不出这类错** → 跨库写 SQL 必须真跑一次。回归哨兵：截 `DO UPDATE SET ` 到 ` RETURNING ` 之间断言。
- ⚠️ **count SQL 必须与 select 共用 `buildWhereClause`**：`buildWhereParts` 返回的切片已含 logic 标记元素，count 若改用 `Join(" AND ")` 拼同一份切片会产出 `p0 AND AND AND p1`。各 WHERE 调用点统一用 `if where := ...; where != ""` 防悬空 WHERE。
- ⚠️ **畸形 `in` / `notIn`（值非数组 / 空）fail-closed**：渲染成 `field IN (NULL)`（恒假、0 行），**绝不静默丢弃整条条件**——丢弃等于返回未过滤的全量数据。哨兵：`zz_repro_op_test.go` 同文件的用例。
- ⚠️ **builder 层粒度自守**：`renderDimensionGroupBy` 拼 `DATE_TRUNC('%s',...)` 前必须过 `granularityTokenPattern`（与 `ValidateGranularity` 同一份），非法时退化原始字段。executor 先校验是生产路径，builder 守卫防的是绕过 executor 的新调用点。
- ⚠️ **`InferExpressionResultType` 必须排序后遍历 `SupportedExpressions`**：map 迭代序随机，**未排序**时同一表达式多次调用会返回不同类型（非纯函数）。已有 fuzz 哨兵 `FuzzZZInferExpressionResultType` 看护 —— 改这个函数（`backend/internal/model/datatypes.go`）时**别破坏排序遍历**。

## 查询记录与分享短码

- 库内 `query_id` = **UUID v7**；地址栏短码 = **base58 定长 21 位**（`internal/idcodec`，设计理由见该包注释——代码现场）。
- `?q=` 优先级 **> `edit` > `datasetId`**；带 `q` 时其余参数整体失效。
- 按 `spec_hash`（含 `dataset_id`）去重，命中只更新 `row_count` / `duration_ms`，**绝不动 `created_at` / `expires_at` / 主键**。`spec` 信封为 `{v:1, document:<ChartConfigDocument>}`，应用层有 **32KB** 上限。`queryrecord` **有意不认识数据集**（不校验 `dataset_id` 是否存在）= 快照语义，别当漏校验补。
- 自造短码测错误码：`111111111111111111111`（= `uuid.Nil`）→ **20300**；长度 / 字符集不符 → **20100**。

## 图表槽位

- **内置 13 种图型**（`chartDefinitions.ts` 与 `backend/internal/query/types.go` 双端一致）；**图型中文名的唯一来源 = `chartDefinitions[type].label`**。
- **已下线槽位** `color_group` / `series_group`（某次决策后下线）；后端 `SlotColorGroup` / `SlotSeriesGroup` 常量与归槽分支**刻意保留**，**勿当死代码删**。
- **字段组有两套口径，别混**：`getActiveFieldGroups` 按定义槽位 `slice` 裁剪；`normalizeQueryConfigForChartType` 按 `minGroups` 补齐并把溢出组绑定**搬进**同类型保留组——不搬运则透视表切表格会**静默丢列维度**。**v2 wire 触发** = combo 双轴指标槽位 ∨ 具名维度槽位。⚠️ `maxFields` 是**纯声明、无校验** → **不要为满足它丢字段**。
- **过滤**（`FilterDropZone.tsx`）是**图型无关的公共行，不写进 `chartDefinitions.fieldGroups`**；多条件恒「且」；同字段可重复加入（`>=` + `<=` 即区间）；条件存**列名**。
- **改槽位同步清单**（也注释在 `chartDefinitions.ts` 顶部）：`fieldGroups`（+`styleKeys`）→ `DUAL_AXIS_METRIC_SLOTS` / `SLOT_PROTOCOL_DIMENSION_SLOTS` → 孤儿槽位 → `chartOptions.ts` 渲染分支 → 测试若干处 → 历史图表兼容 → 后端常量默认不删。

## 前端陷阱

- ⚠️ **`StrictMode` 的 mount→cleanup→再 mount**：一次性副作用若把"已认领"记账放在 `await` **之前**、cleanup 又置 `cancelled=true` → 第二次挂载被自己挡回、首次响应被丢 ⇒ **请求真发了但界面毫无反应**。记账须放在成功之后；同 key 并发用 in-flight promise 共用请求。该类用例必须**显式包 `<StrictMode>`**。⚠️ 同源坑：**effect 依赖传内联对象字面量**（父级每次渲染都新建）→ 弹窗打开期间任何父级重渲染都会重置用户输入，依赖要拆成原始值。
- ⚠️ **`frontend/vite.config.js` / `.d.ts` 是 tsc 产物且会遮蔽 `vite.config.ts`** → 症状是**改了 `.ts` 完全不生效**。排查第一步 `ls frontend/vite.config.*`。成因是 `tsconfig.node.json` 的 `composite: true`；⚠️ 别用 `noEmit: true` 堵（TS6306 / TS6310）。
- ⚠️ 本地预览**只用 `http://localhost:23351`**（`insights.localhost` / `data.localhost` 也在白名单，`127.0.0.1` **不在**）。
- antd 6.6.4：`Space` 垂直用 `orientation="vertical"`；`Input` 用 `variant="borderless"`；`Spin.tip` 已弃。以 `tsc` 通过作为 prop 合法性的判据。
- ⚠️ **纯图标按钮必须加 `aria-label`**（`<Tooltip title>` 的 title **不进** accessible name）。⚠️ **`getByText` 只比对直接文本子节点**：「维度 (13)」若拆成兄弟元素会让「维度」独立成节点，撞上中栏槽位标签。
- ⚠️ **`vi.clearAllMocks()` 不清 implementations**（只清 calls）。⚠️ `mockRejectedValue` 会留下未结算的 rejected promise，`console.error` 落到**下一个用例**的 stderr → 用 `mockRejectedValueOnce` + `act` flush 再 restore spy。
- ⚠️ **`// biome-ignore` 在 JSX 属性位不生效**（`//` 注释不抑制，`{/* */}` 在属性位是 parse error）→ 遇到属性级 a11y 报错**只能改结构**，别指望抑制注释。
- ⚠️ **pre-commit 钩子 = `cd frontend && pnpm exec lint-staged`** → 对**暂存**的 `.ts/.tsx` 跑 `biome check`。**任何一条 biome error 都会让提交直接失败**。被挡时先单独复跑 `pnpm exec lint-staged` 看 exit code，别先怀疑环境。
- ⚠️ **`react-grid-layout` v2 的 API 与网上 v1 示例完全不同**（照抄必报错）：主入口导出 `GridLayout` + **分组 props** —— `gridConfig={{ cols, rowHeight, margin, containerPadding }}`、`compactor={verticalCompactor}`（**不是** `compactType`），另有 `dragConfig` / `resizeConfig` / `dropConfig` / `positionStrategy` / `constraints`。**主入口没有 `WidthProvider`**（为 `undefined`），必须用 `useContainerWidth()`，它返回**对象** `{ width, mounted, containerRef, measureWidth }`（不是数字），把 `containerRef` 挂到外层 div。样式**只需** `react-grid-layout/css/styles.css`——v2 已把 react-resizable 的手柄样式并入该文件，而 `react-resizable` 未 hoist 到 `node_modules` 根，**单独 import 它的 css 会解析失败**。子元素必须是能接 `style` / `className` / `ref` 的原生元素：`GridItem` 用 `cloneElement` 把 `createStyle(pos)` 的宽高写进**直接子元素**，自定义组件须 forwardRef。
- ⚠️ **别用 `onLayoutChange` 同步布局**：它在**挂载时也会触发**，而 compactor 会把落库布局里不紧凑的块向上吸附 → 症状是"一打开页面就显示有未保存改动"并写出草稿。改认 `onDragStop` / `onResizeStop`（两者的首参都是**完整 layout**，碰撞下推的邻块也在其中）。
- ⚠️ **jsdom 量不到容器宽度** → `useContainerWidth` 停在 `width=0`，栅格永不渲染。页面测试必须**整模块替换** `react-grid-layout`（替身里给 `width: 1200`）；要覆盖拖拽提交路径，就让替身把收到的 props 存下来供测试直接调 `onDragStop`。
- ⚠️ **antd 按钮的 accessible name 会被图标的 `aria-label` 前缀污染**：`<Button icon={<PlusOutlined />}>新建仪表盘</Button>` 的可达名是 `"plus 新建仪表盘"` → `getByRole('button', { name: '新建仪表盘' })` 取不到，测试里用正则 `/新建仪表盘/`。
- ⚠️ **antd `Select` 在 jsdom 里的展开**：`fireEvent.mouseDown(screen.getByRole('combobox'))`（antd 6 里取不到 `.ant-select-selector`；**页面上有几个下拉就有几个 combobox**，含分页器的 `aria-label="Page Size"`，按索引取要数清楚），选项用 `findByText(label, { selector: '.ant-select-item-option-content' })`。⚠️ **别点 `role="option"`**：这份 listbox 是 `width/height=0` 的**隐藏**无障碍 / 测量节点，其选项**文本是 value 而不是 label**，点它不触发 `onChange`——症状是"选项找得到、点了没反应"。真正可点的是门户里的 `.ant-select-item-option`。antd 6 的 `Alert` 已弃用 `message` → 用 `title`。

## 日期筛选

> 语义单一事实源是 `frontend/src/lib/dateFilter.ts`（纯逻辑、零 UI 依赖）。改之前先读它顶部的设计说明。
> 仪表盘盘级筛选（三族）的下发契约在 `frontend/src/lib/dashboardFilterValue.ts` + 后端 `service/dashboard/query.go`。
> ⚠️ **取值形状按算子分流，三族共用同一条规则**：`in` / `notIn` 数组、`between` 两元素、
> **其余标量算子恰好一个元素**（后端 `buildOverrides` 取 `value[0]` 当绑定参数；给多了会被降级成 `in`）。
> `between` 缺一端时前端**整条不下发**（退回未激活），不补空值造恒假区间。

- ⚠️ **`buildOverrides` 的取值形状必须按算子分流**：请求里的值恒为数组，但 `in` / `notIn` 要数组、`between` 要**两个标量**、**标量算子（eq/neq/gt/gte/lt/lte/like）要单个标量**——`buildFilterPart` 对它们是 `append(f.Value)` 单参数绑定，塞数组进去会渲染成 `col = ARRAY[...]`（PG 42883）。`between` 的拆分还必须排在「多值降级为 in」之前，否则区间被吃成 `IN (下界, 上界)`。
- ⚠️ **`DashboardFilterBinding.column` 必须是列 ID，不是列名**：盘级条件最终以 `entity.Filter.Field` 传给取数，而「哪些字段被盘级条件覆盖」（`overriddenFields`）是拿图表自身过滤条件里的**列 ID**（`OwnFilterFields` ← config 的 `filters[].fieldId`）求交集 —— 存列名这个可见标识永远匹配不上。⚠️ openapi 里 `binding.column` 的描述仍写着「列名」，是改造前的口径，**待同步**（改 description 要重跑 `make api-gen`）。
- ⚠️ **未落库的筛选器后端读不到**：`/query` 是按**已落库的** `layout_json` 逐块取数、并按同一份 layout 建筛选器索引的，请求里出现未知 `widgetId` 会被静默忽略。所以新加的筛选器必须保存后才生效 —— 块上要提示「保存仪表盘后生效」，否则看起来就是「筛选坏了」。
- ⚠️ **盘级筛选每个筛选器每次查询最多一条合并条件**（请求的 values 以 `widgetId` 为键），因此「包含空日期」（区间 OR IS NULL）在仪表盘侧**表达不出来**；图表查询页不受影响。
- ⚠️ **`isNull` / `isNotNull` 在盘级载荷里要放一个占位元素**：契约用「数组非空」判定激活，空数组 = 未激活。占位用 `null`（`ACTIVATION_PLACEHOLDER`）而不是 `''`，免得被误当成真实比较值。
- ⚠️ **别用「跳过 isNull」去找展开后的主条件**：`expandDateFilterIntent` 的主条件恒为 `[0]`，「包含空日期」那条 IS NULL 追加在尾部并带 `logic:'or'`；而特殊值「空日期」的主条件**本身就是 isNull**（实测踩过：按 operator 过滤会让「空日期」与保存快照双双退化成 `eq ''`，静默查出 0 行）。
- ⚠️ **日期语义是「双端镜像」，靠一张共用用例表防漂移**：前端 `lib/dateFilter.ts`、后端 `internal/query/datefilter.go`，两侧共读 `frontend/src/lib/__fixtures__/dateFilterCases.json`（Go 侧 `datefilter_test.go`，TS 侧 `dateFilter.cases.test.ts`）。**改任何一侧的语义都必须同时改另一侧并跑两边**，否则那张表会直接抓出不一致。新增语义时先往表里加例、再加实现。测试 now 用 `time.ParseInLocation` 按本地时区解释（`time.Parse` 会掉进 UTC，导致「同一天」差一天）。
- ⚠️ **后端解析日期时用「单侧边界放 `Value`」的形状**：`buildFilterPart` 对 `gte` / `lte` 只绑 `Value`，`ValueEnd` 仅 `between` 使用 —— 把 lte 的上界塞进 `ValueEnd` 会让它渲染成 `col <= NULL`（恒假、0 行，且全程无报错）。
- ⚠️ **解析不出条件时整条丢弃，绝不回落 operator / value**：未选择的日期条件在内存态里兜底成「等值空串」，回落会把「不过滤」变成「查出 0 行」。丢弃与前端查询路径的行为一致。
- ⚠️ **日期解析的「今天」取服务器本地时区**（`datefilter.go` 顶部有说明）。前端用的是浏览器本地日；同一套部署下一致，跨时区多地域部署会有一天边界差 —— 与「数据集按业务时区落数」的假设配套。
- **意图 ≠ 快照**：`FilterCondition.date` 存**意图**（如 `最近 7 天`），**两端都现算**（前端构造请求时、后端读 config 时）。`materializeDateFilterSnapshot` 写下的快照**不是取数依据**，只是「不认识 `date` 的下游」的兜底与人工排查用；且它有损：一条 `FilterCondition` 只能表达一个 SQL 条件，「包含空日期」在快照里会丢失（取数不受影响，意图现算时会展开成两条）。「所有日期」物化不出条件 → 该条整体被丢弃。
- ⚠️ **`FilterCondition.fieldId` 是列的稳定 id（`DatasetColumn.id`，形如 `0000i529`），不是列名**。按列名反查会让芯片退化成裸 id 并丢字段颜色。列名只用于展示与 SQL 输出别名；wire 上的键仍是 **`field`**（openapi / `entity.Filter` 口径），但值应是列 ID（后端 `columnIndex.byName` 只是过渡态索引）。
- **区间端点一律输出 `YYYY-MM-DD`**（datetime / 小时粒度到秒）；`YYYY-MM`、周序号这类形态**只活在 UI 层**（`formatDateFilterPreview`）——SQL 比较用的是真实日期。
- **区间口径统一以「昨天」为数据上界**（`本周` = 本周首日~昨天、`本月` = 本月 1 日~昨天）。这不是 bug，是产品侧「T+1 落数」的默认假设；要含今天得用「自定义 + 包含今天」显式打开。
- ⚠️ **三项能力暂缺，别照文档想当然**：`工作日`（要后端补「排除周末」算子，涉及 SQL 表达式白名单）、`最近有数 N 天` 与「单个日期」下的 `最近有数 1 天 / 交易日`（要分区落数元数据，当前未采集）、`公共日期筛选器跳转`（没有跨筛选器跳转能力）。日历置灰「不存在的日期」走的是可选 prop `availableDates`，不传就完全不置灰。

## 前端约定

- **骨架硬契约**：`.dr-page` → `<PageHeader>` → `<Card>`；**标题绝不放回 Card 内**。token 放在 `styles/index.css` 的 **`:root`**（portal 组件要取到）；⚠️ 旧色 `#1890ff` 已清零**别写回来**；⚠️ **不要改全局 `paddingSM`**（多个组件消费）；⚠️ **`Select` 横向内边距无公开 token**（写错位置会 TS2353 静默忽略）→ 在 `components.Select` 里就地重定义。
- **术语铁律**：`query_type=table` 称**「物理表」**、`sql` 称**「自定义 SQL」**。
- **API 客户端唯一来源**是 `lib/api/client.ts` 的 `resolveApiBaseURL()`；`api/index.ts` 只消费，**不得再 `axios.create`**。拦截器：业务码非 20000 只 `console.error` + reject（不弹 toast），HTTP ≥ 400 才报 Sentry。
- **CORS 是白名单 echo 模型**：白名单外的预检**仍返 204 但不带 `Access-Control-*` 头** → 前端只见 `Network Error`，极易误判成后端起不来。
- 图表配置文档 schema：`lib/chartConfigSchema.ts` 的 `ChartConfigDocument`（当前 `version: 2`），旧结构加载时经 `migrateChartConfig` 自动迁移。

## StarRocks 与其他外部库

- ⚠️ StarRocks：`VARCHAR(n)` 的单位是**字节**（不是字符）；单 BE 部署必须 `replication_num=1`；用 `pymysql` 连接时必须显式传 `database=`。
- 空集群首次使用需先自建角色与库；**表结构由后端启动时的 goose 迁移（`embed.FS` 内嵌）自动创建**，无需手工导 schema。

## 工具与验证

- ⚠️ **全量回归必须核验"跑的是不是同一棵树"**：只要在全量运行期间有任何一方（另一个会话 / agent / 编辑器）改了文件，结果一律不可信。核验法：运行**前后各算一次** `find src -type f \( -name '*.ts' -o -name '*.tsx' \) | sort | xargs shasum | shasum`，两侧一致才有资格宣称"这轮结果对应这棵树"。
- ⚠️ **`grep -c` 在计数为 0 时退出码是 1** → 会静默掐断 `&&` 链，表现为"命令 2 秒就成功"，其实后半段根本没跑。取计数写 `$(... | grep -c ... || true)`，或别用 `&&` 串联。
- ⚠️ **验证"某个提交能否独立构建"要用隔离工作树**，别靠依赖关系推理：`git worktree add --detach /tmp/wt-verify <sha>`，再把 `node_modules` 与 `.env` 软链进去，在该树里跑 `go build/test` 与 `tsc`。纯推理容易漏。
- ⚠️ **`find -newermt` 在部分受限环境（如沙箱）里会失效**（恒返空）→ 排查"谁在什么时候改了什么"改用 epoch 排序：`find . -type f -name '*.ts' -print0 | xargs -0 stat -f "%m %Sm %N" | sort -rn`。
- ⚠️ **后台起 dev 服务必须断掉 stdin（`< /dev/null`）**：`make dev` 把 vite 放进后台进程组后，它会为"键盘交互"去读终端 stdin，内核随即以 **SIGTTIN** 停住整个前端进程组 —— 症状极迷惑：进程活着、端口 `LISTEN`、但请求一律超时无响应（`ps` 状态是 `T` 而非 `S`）。⚠️ 别对 `T` 状态进程直接 `kill -CONT` 唤醒：它读完 stdin 会立刻退出，触发 `make dev` 的 EXIT trap **把后端一起收掉**。当前 `Makefile` 的 `dev` 已用 `< /dev/null` 规避，改这段时别把重定向删掉。
- ⚠️ **清"疑似诊断残留"的进程前必须先反查归属**：`ps -eo pid,ppid,lstart,command` 看父子链与启动时刻，确认是自己某次操作的后代再动手；无法确认就先问，别误杀他人正在跑的服务。
