# Data Insights 表面系统（Surface System）

> **受众**：维护 Data Insights 前端的工程师，以及在此基础上做二次开发的协作者。
> **主任务**：拖拽式 BI 可视化分析 —— 选数据源 → 建数据集 → 拖字段配图 → 保存/分享。
> **范围**：全站 9 个页面（图表构建 / 图表列表 / 数据集列表·详情·编辑 / 数据源列表·详情 / 分享列表 / 分享只读页）与首页的**表面层级、色彩、字体、页面骨架、组件样式**。
> **不覆盖**：图表本身的视觉（`lib/chartOptions.ts` 的 ECharts option 构造）、国际化文案、后端。
> **依据来源**：全部结论来自源码取证（读 `styles/index.css`、`pages/*.tsx`、antd 6.6.4 发行产物），不含用户调研或可用性测试数据。
> **关键假设**：产品定位为**内部密集型数据工具**，不面向 C 端；因此优先"一屏塞更多"而非"留白显高级"。

---

## 1. 视觉主题

### 1.1 要解决的问题

用户反馈："整个页面基本上都是白色为主，颜色稍微有点单调，过于简陋。"

取证后的根因**不是"白色太多"，而是面与面之间没有明度差**。这是全站性的，不是某一页的问题：

| 现象 | 取证位置 |
|------|----------|
| 页面只有 `#fff` / `#fafafa` 两个明度值；卡片也是纯白 | `pages/Charts.tsx` 等 8 个页面的顶层容器 |
| 卡片唯一的边界线索被本页 CSS 显式摘掉 | `index.css` 旧规则 `.chart-builder-page .ant-card-bordered { border: none }` |
| antd 6 的 Card `headerBg` 默认是 `transparent`，无边框后卡头完全融进卡体 | `antd/es/card/style/index.js` |
| **页面标题渲染在卡片内部** → 卡内同时存在背景白、卡头白、标题区白三层同色 | `Charts.tsx` / `Datasource.tsx` / `Dataset.tsx` 等 |
| 全页最大一片白 = 预览空态 | `ChartCanvas` 空态 `Empty` + `padding:'100px 0'` |
| 顶栏品牌标记从未定义样式 | `App.tsx` 用 `.demo-logo`，全仓无该 CSS 规则 → 渲染成 0×0 空 div |
| 「维度」有两个蓝 | `#1890ff`（4 处）与 antd 预设 blue `#1677ff` 并存 |
| 「指标」有两个色 | 图表构建页用绿 `#52c41a`，数据集详情页用紫 `#722ed1` |
| 灰阶暖冷混用 | `#999`/`#595959`/`#f0f2f5`/`#fafafa`（暖）与 `#f8f9fa`/`#f4f5f7`（冷）并存 |

### 1.2 视觉性格

**克制的工具感（restrained instrument）** —— 信息密度优先，颜色只用作语义信号。

刻意**不**采用：深色模式、玻璃拟态、装饰性渐变、为"高级感"增加留白。理由：本产品的用户是每天看同一批数据的分析人员，装饰不产生价值，留白直接减少一屏可见的数据行。

### 1.3 参考观察（与提议规则分开记录）

- **观察**：GitHub Primer 的 surface/border 三层模型（canvas / surface / sunken）专为浅色密集运维界面设计，与本产品的数据密度接近。
- **观察**：Linear 的强调色纪律 —— 一个强调色只用于选中与主操作。
- **提议规则**（下述第 2–6 节）：把上述观察收敛为 5 条可执行原则。这是**本项目的提案，不是对上述产品的复刻**。

### 1.4 五条不可让步的原则

1. 任何两块相邻的面，必须存在明度差**或**边界线，不能两者皆无。
2. 描边定义边界、投影定义高度，两者同时存在才算"浮起来"。
3. 颜色只出现在有含义的位置（字段类型 / 分组 / 选中 / 主操作 / 状态），不做装饰着色。
4. 同一语义只能有一个色值。
5. 空隙只服务于"看得清"，不为观感增加纵向留白。

---

## 2. 色彩谱系

### 2.1 三层灰 + 一条蓝

中性偏冷灰阶。**禁用暖灰**（`#f5f5f5` / `#fafafa` / `#f0f2f5` 系）—— 它们与冷灰并置时会显出轻微的"发黄"。

| 角色 | CSS 变量 | 值 | 用途 |
|------|----------|-----|------|
| L0 画布 | `--dr-canvas` | `#f4f5f7` | 页面底、表头底、Footer 底 |
| L1 面板 | `--dr-surface` | `#ffffff` | 卡片、浮层、页头图标块 |
| L2 下沉 | `--dr-sunken` | `#f7f8fa` | 拖放区、占位区、分段容器、代码块、斑马纹偶数行 |
| 发丝线 | `--dr-border` | `#e6e8eb` | 卡片描边、卡头下缘、元信息条 |
| 强调线 | `--dr-border-strong` | `#d9dde3` | 表头下缘、虚线框（需要被看见的线） |
| 一级文本 | `--dr-text-1` | `#1f2328` | 页头标题、卡头标题、数值 |
| 二级文本 | `--dr-text-2` | `#57606a` | 正文、表头文字 |
| 三级文本 | `--dr-text-3` | `#8c95a1` | 计数、辅助说明、次要图标 |
| 四级文本 | `--dr-text-4` | `#b0b7c3` | 占位图形、最次要提示、空态圆点 |
| 强调色 | `--dr-accent` | `#1677ff` | 唯一强调色：主操作、选中、焦点环 |
| 强调浅底 | `--dr-accent-soft` | `#f0f6ff` | 表格行悬停、字段行悬停 |
| 卡片投影 | `--dr-shadow-card` | `0 1px 2px rgba(16,24,40,.04)` | 卡片"贴地"级 |

### 2.2 语义色三件套（不允许分叉）

| 语义 | CSS 变量 | 值 | 出现位置 |
|------|----------|-----|----------|
| 维度 | `--dr-dim` | `#1677ff` | 分组色条、槽位色条、拖放区悬停描边、字段类型点、数据集字段名列 |
| 指标 | `--dr-metric` | `#52c41a` | 同上；字段角色开关、查询状态点 |
| 过滤 | `--dr-filter` | `#fa8c16` | 过滤字段组的色条与落点高亮 |
| 日期维度 | `--dr-date` | `#722ed1` | 字段类型点（与 antd Tag `purple` 同源） |

`#722ed1` 另有一处独立用法：**虚拟/计算字段**图标与标签。它与"日期维度"共享色值但不在同一区域共现，暂无歧义；若将来两者需要并列展示，必须为此新增第四个语义色。

### 2.3 CSS 声明

```css
:root {
  --dr-canvas: #f4f5f7;
  --dr-surface: #ffffff;
  --dr-sunken: #f7f8fa;
  --dr-border: #e6e8eb;
  --dr-border-strong: #d9dde3;
  --dr-text-1: #1f2328;
  --dr-text-2: #57606a;
  --dr-text-3: #8c95a1;
  --dr-text-4: #b0b7c3;
  --dr-accent: #1677ff;
  --dr-accent-soft: #f0f6ff;
  --dr-shadow-card: 0 1px 2px rgba(16, 24, 40, 0.04);
  --dr-dim: #1677ff;
  --dr-metric: #52c41a;
  --dr-filter: #fa8c16;
  --dr-date: #722ed1;
}
```

**为什么挂在 `:root` 而不是页面作用域**：三类消费者跨页面边界 —— ① 各页的页面骨架；② portal 到 `body` 的 `Drawer`/`Modal`/`Dropdown`；③ 在 `ShareView` 下复用的 `PivotTable`/`TableChart`/`KpiCard`。挂在页面作用域上，后两者取不到值。

> **对比度状态：未测量。** 本表数值取自 antd 6 的色阶与中性灰推导，未用工具计算 WCAG 对比度，也未做色盲模拟。`--dr-text-3`（`#8c95a1`）在 `--dr-surface` 上的对比度属于"辅助文字"档，是否满足 AA 需实测确认。**不要在本节被引用为已通过无障碍审计。**

### 2.4 不引入的色

- **错误红**：沿用 antd 预设 `#ff4d4f`（`QueryPanel` 两处用法），不新增 token。
- **状态浅底**（拖放区落点高亮 `#e6f4ff` / `#f6ffed` / `#fff7e6`）：仅三处消费，就近以字面量登记在 `dropZoneStyles.ts`，不进 token 表。
- **`rgba(22,119,255,.18)`**（三栏拖拽分隔条悬停态）：CSS 变量无法直接携带 alpha，保留字面量。

---

## 3. 字体排印

### 3.1 字体栈

沿用系统栈，**不加载任何 Web Font**，因此无字体资产与授权依赖：

```css
body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
    "Helvetica Neue", Arial, "Noto Sans", sans-serif;
}
```

中文由系统字体（macOS 的 PingFang SC / Windows 的 Microsoft YaHei）承担；栈内 `Noto Sans` 覆盖 Linux。无内嵌字体 → 无 FOIT/FOUT，也无 CJK 子集化成本。

### 3.2 角色字号

| 角色 | 字号 / 字重 / 行高 | 变量 |
|------|-------------------|------|
| 页面标题（`h1`） | 16px / 600 / 1.4 | `.dr-page-header__title` |
| 页面副标题 | 12px / 400 / 1.5 | `.dr-page-header__desc` |
| 卡片标题 | 13px / 600 | `.ant-card-head-title` |
| 正文、表格、表单 | 13px / 400 / 1.4 | 全局 token |
| 计数、辅助、表头 | 12–13px / 400–600 | `--dr-text-2/3` |
| 空态图形 | 由 antd `Empty` 决定 | — |

**字距恒为 0**（不设 `letter-spacing`）。中英混排下加字距会破坏 CJK 的方块节奏。

⚠️ 未收的口径：全局圆角仍为 6/8 两档混用（antd 默认），字段芯片仍为 5px。属既有约定，本轮未动。

### 3.3 换行与截断

| 场景 | 策略 |
|------|------|
| 页面标题 | `min-width: 0` 允许收缩；超长由 `flex-wrap` 让操作区换行，标题本身**不截断**（标题被截断比换行更糟） |
| 页头副标题 | 单行 `display: block`，超出由浏览器自然换行 |
| 表格单元格 | 全局 `ellipsis` + `title` 属性（既有实现） |
| 数据集字段名列 | `max-width: 170px` + `text-overflow: ellipsis` + `title` 兜底 |
| 字段芯片 | 见 4.2，芯片文案保持最短（产品约定：chip 文案避免换行/溢出） |

---

## 4. 组件样式

### 4.1 页面骨架（新增）

```css
.dr-page {
  min-height: 100%;
  padding: 16px;              /* 原各页内联 padding:24 → 收 1/3 */
  background: var(--dr-canvas);
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.dr-page--center {            /* 加载 / 空 / 结果页 */
  align-items: center;
  justify-content: center;
  min-height: calc(100vh - 200px);
  gap: 16px;
}

.dr-state {                   /* 卡片内的加载态与空态共用，两者高度必须一致 */
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 200px;
  gap: 8px;
}
```

**这是本轮最关键的结构性改动**：标题从卡片内提到卡片外的画布上。只要标题还在卡片里，卡内就同时存在背景白、卡头白、标题区白三层同色——无论怎么调色都救不回来。

### 4.2 页头

| 部件 | 尺寸 / 值 | 说明 |
|------|-----------|------|
| `.dr-page-header` | flex / `gap: 6px 16px` / `flex-wrap: wrap` | 行间距 < 列间距：面包屑换行到标题上方时不该产生段落级留白 |
| `.dr-page-header__icon` | 28×28 / 圆角 6 / 白底 / `1px --dr-border` / `--dr-shadow-card` / 图标 `--dr-accent` | 与顶栏品牌标记同族语汇，给页头一个视觉锚点 |
| `.dr-page-header__title` | `h1` / 16px / 600 / `--dr-text-1` | 全页唯一 `h1` |
| `.dr-page-header__desc` | 12px / `--dr-text-3` | 一句话说清这页能做什么 |
| `.dr-page-header__crumb` | `flex-basis: 100%` | 靠 flex 换行占满整行，不额外包一层 DOM |
| `.dr-page-header__extra` | flex / gap 8 | 右侧操作区，主操作放最右 |

组件位置：`components/PageHeader.tsx`（唯一实现，各页只传 props）。

### 4.3 卡片

```css
.ant-card {
  border: 1px solid var(--dr-border);
  box-shadow: var(--dr-shadow-card);
}
.ant-card-head { border-bottom: 1px solid var(--dr-border); }
.ant-card-head-title { font-size: 13px; font-weight: 600; color: var(--dr-text-1); }
.ant-card-extra { font-size: 12px; color: var(--dr-text-3); }
```

antd 6 的 `Card.headerBg` 默认 `transparent`，卡头无边框时会完全融进卡体，故下缘线必须显式给。卡头保持白底而不染灰——灰卡头会和灰画布同色，反而更糊；层次改由"下缘线 + 标题对比度"建立。

### 4.4 数据表格

```css
.ant-table-wrapper .ant-table-thead > tr > th {
  background-color: var(--dr-canvas) !important;
  color: var(--dr-text-2);
  font-weight: 600;
  border-bottom: 1px solid var(--dr-border-strong) !important;
}
.ant-table-wrapper .ant-table-tbody > tr:nth-child(even) > td {
  background-color: var(--dr-sunken);
  box-shadow: inset 0 1px 0 rgba(16,24,40,.04), inset 0 -1px 0 rgba(16,24,40,.04);
}
.ant-table-wrapper .ant-table-tbody > tr:hover > td {
  background-color: var(--dr-accent-soft) !important;
}
```

保留"紧凑 + 斑马纹"的密集表格策略，但：表头底色由暖灰 `#f0f2f5` 改为画布色；下边线由 2px 暖灰改为 1px 冷灰（2px 线在卡片这种小面积里太重，会和卡头下缘线打架）；行悬停由 antd 默认 `#e6f4ff` 改为与强调色同源的 `--dr-accent-soft`。

### 4.5 两条视觉语法（不要混用）

- **3px 圆角色条 = 这是一个分组/槽位**。出现在：左栏字段分组头、中栏槽位标签、`FieldGroupHeader`。
- **5px 圆点 = 这是一个字段**，颜色 = 字段类型（维度蓝 / 日期紫 / 指标绿），与字段标签色同源。

形状与尺寸不同 → 两个层级不会被读混；颜色同源 → "维度是蓝的"只需要学一次。

⚠️ 例外：左栏字段分组头的圆点与色条**必须**保持在同一文本节点内（`getByText` 只比对直接文本子节点，拆成兄弟元素会让「维度」独立成节点、与中栏槽位标签撞车）。

### 4.6 本次改动清单

| 文件 | 改了什么 |
|------|----------|
| `styles/index.css` | token 由 `.chart-builder-page` 提升为 `:root`；新增 `.dr-page` / `.dr-page--center` / `.dr-state` / `.dr-page-header*` / `.dr-card-toolbar`；卡片与表格规则由页面作用域提升为全局并统一色值 |
| `components/PageHeader.tsx` | **新增**。全站页头唯一实现 |
| `App.tsx` | `Content` 底色 → `--dr-canvas`；`Layout` 底色 → `--dr-canvas`；`Header` 下缘线 → `--dr-border`；`Footer` 去暖灰底改透明 + 上边线；首页改为 `.dr-page` + 页头 + 空态卡 |
| `pages/Charts.tsx` | 页头出卡；图型标签由 `BAR`/`LINE` 全大写英文改为 `chartDefinitions` 的中文图型名 |
| `pages/Datasource.tsx` | 页头出卡 |
| `pages/Dataset.tsx` | 页头出卡；搜索框收进 `.dr-card-toolbar`；字段角色色与开关统一为维度蓝/指标绿 |
| `pages/DatasetDetail.tsx` | 页头 + 面包屑出卡；元信息条转 `--dr-sunken` + `--dr-border`；字段角色色统一 |
| `pages/DatasetEdit.tsx` | 页头出卡；表单内分区上边线统一 |
| `pages/DatasourceDetail.tsx` | 页头 + 面包屑出卡，类型标签移入操作区 |
| `pages/Share.tsx` | 卡头改为页头；加载态与空态统一到 `.dr-state` |
| `pages/ShareView.tsx` | 三态（加载/过期/密码）改用 `.dr-page--center`；主视图页头出卡；**修掉 `minHeight:'100vh'` 造成的页面溢出** |
| `pages/ChartBuilder.tsx` | 页面内字面量改为变量（token 已到 `:root`，portal 亦可用）；状态点与分组色条统一 |
| `components/ChartBuilder/DraggableField.tsx` | 语义色表改引用变量；字段行悬停转 `--dr-accent-soft` |
| `components/ChartBuilder/dropZoneStyles.ts` | 三色改引用变量 |
| `components/ChartBuilder/QueryConfigRow.tsx` | 三个槽位色改引用变量 |
| `components/ChartBuilder/FieldDropZone.tsx` / `FilterDropZone.tsx` | 字段圆点与落点轮廓改引用变量 |
| `components/ChartBuilder/PivotTable.tsx` | 小计/合计行改用变量 |
| `__tests__/pages/surfaces.test.tsx` | **新增**。页面骨架结构契约回归测试 |

---

## 5. 布局

### 5.1 内容顺序

```
.dr-page（灰画布，flex column，gap 12）
├── .dr-page-header（画布上：面包屑 → 图标 + 标题 + 描述 → 右侧操作）
└── .ant-card（白面板承载表格 / 表单 / 图表）
    └── .dr-card-toolbar（可选：搜索、分段按钮，margin-bottom 10）
```

**顺序不可颠倒**：页头必须在卡片之前。若某页确实需要"卡内标题"（例如卡片承载与页面无关的次级内容），用卡头表达，仍不得把页面标题放进卡片。

### 5.2 间距刻度

| 值 | 变量 | 用途 |
|----|------|------|
| 16px | `--dr-gap-page` | 页容器内边距 |
| 12px | `--dr-gap-block` | 页头↔卡片、卡片↔卡片 |
| 10px | — | 卡内工具条与表格之间 |
| 8px | — | 操作按钮之间 |
| 6px | — | 同类元素之间（字段芯片、拖放区内部） |

幅度纪律：**每次只收 1/3~1/2**（如 24→16、11→6）。数值在代码里带注释说明"为什么是这个数"。

### 5.3 空 / 密数据

- 空态与加载态**必须同高**（`.dr-state` 固定 `min-height: 200px`），否则请求返回时容器会跳一下。
- 空态与有内容态也应同高（拖放区：`.dr-core` 空态与有字段态共用 `min-height: 26`）。
- 表格行不因数据为空而塌陷：由 `Empty` 的 `--dr-text-4` 占位图形承担视觉重量。

### 5.4 固定格式版面

`/chart-builder` 是**三栏工作台**（字段库 / 查询配置 + 预览 / 配置面板），有独立的尺寸约束：拖拽分隔条可调宽（下限 120px），中栏卡片间距 6px，三栏内边距为 0。它不套用 `.dr-page`（自带 `padding: 0` 与画布底色），但共用同一套 token 与卡片规则。

---

## 6. 深度与层级

本产品**层数刻意保持为 3**：

| 层 | 面 | 边界手段 |
|----|----|---------|
| L0 画布 | 页面底 | — |
| L1 面板 | 卡片、浮层 | 1px `--dr-border` 描边 + `--dr-shadow-card` 投影 |
| L2 下沉 | 拖放区、占位、代码块、分段容器 | 底色 `--dr-sunken` + 1px 虚线 `--dr-border-strong` |

**描边与投影必须同时存在**：只有描边 = 扁平的白色矩形；只有投影 = 白纸上的一团脏斑。两者合起来才读得出"这张面浮在画布上"。

浮层（`Modal` / `Drawer` / `Dropdown` / `Tooltip`）由 antd 通过 portal 挂到 `body`，沿用 antd 自带投影，不额外定义。因 token 挂在 `:root`，这些 portal 节点同样能取到变量。

---

## 7. 注意事项与风险

### 7.1 禁止模式

| 禁止 | 理由 | 可观察的检查方式 |
|------|------|-----------------|
| 把页面标题放回 `<Card>` 内 | 卡内白+白+白三层同色，退回"白底白卡" | `__tests__/pages/surfaces.test.tsx` 断言 `header.closest('.ant-card') === null` |
| 新增第二处维度蓝 / 指标色 | 同一概念两种颜色，用户需要重新学 | 全仓 grep `#1890ff`、`#722ed1` 应为 0 处（虚拟字段图标除外） |
| 在非语义位置使用彩色 | 颜色失去信号价值，页面变花 | 审查新增 `color:` 是否对应 2.2 表中的语义 |
| 为"好看"增加纵向留白 | 直接减少一屏可见数据行，与产品定位冲突 | 对照 5.2 间距刻度 |
| 空态与加载态用不同高度 | 请求返回时容器跳动 | 两态都应包在 `.dr-state` 内 |
| 改 `PivotTable` 等复用组件的样式时硬编码 HEX | 它们在 `ShareView` 与 `/chart-builder` 两处渲染 | 应引用 `:root` 变量 |
| 改全局 `paddingSM` | 23 个 antd 组件消费该 token | 需要收窄时改 `components.Select` 的组件级 token |

### 7.2 资产约束

- 顶栏品牌标记（`.demo-logo`）是**占位标记**：品牌色圆角方块 + 三道递增柱，不是正式 logo 资产。正式矢量 logo 到位后整块替换。
- 无 Web Font、无图片 CDN 依赖，因此无字体授权与图片版权问题。
- 图表配色由用户配置（`chartStyle.colors`）驱动，默认 `#1677ff`；本设计系统不约束用户自选配色。

### 7.3 未决事项

- **`main.tsx` 全局 token 是否继续下沉**：目前 antd 组件级密度由 `main.tsx` 的 `components.*` token 管，而本设计系统在 `index.css` 的 `:root` 里。两处并存，边界在"antd 组件内部" vs "自绘结构"。是否合并待评估。
- **对比度未测量**（见 2.3）。
- **移动端分支只同步了画布底色与骨架**，未做单独的移动端设计。
- 字段芯片 `padding: 0 5px` + 圆角 5 是产品约定（避免换行），与全局 6/8 圆角不一致，刻意保留。

### 7.4 测试与验证的边界

- 本文件**不声称**已完成真机逐状态目视、无障碍审计、或多端浏览器验证。
- 自动化覆盖范围：类型检查、lint、单元测试、以及 4.5 节所述的结构契约断言。
- 沙箱环境不做浏览器自动化（用户明确拒绝 agent 操控其浏览器），故**渲染效果的最终确认必须由人完成**。

---

## 8. 响应式行为

### 8.1 断点

| 断点 | 变化 |
|------|------|
| ≥ 1920px | 三栏工作台按 Pro 宽度分配；列表页表格充分展开 |
| 1280–1920px | 目标主战场。`.dr-page` padding 16，页头与卡片左右对齐 |
| ≤ 767px | `.dr-page` padding → 12px；表格横向滚动；`.ant-layout-content` padding 12px；顶栏折叠为 Drawer 菜单 |
| ≤ 575px | 控件高度 → 28px；表单项纵向堆叠 |

### 8.2 交互可达性

- 触摸目标：菜单项在移动端为 44px（`.ant-menu-item` height/line-height 44px）；按钮在 767 以下 32px、575 以下 28px。**44px 下限只对主导航成立，表单按钮未达标**——这是既有状态，未在本轮修正。
- 键盘：全局 `:focus-visible` 为 `2px solid var(--dr-accent)` + `offset 2px`；页头提供 skip-link 跳转到 `#main-content`。
- 纯图标按钮必须有 `aria-label`（`Tooltip` 的 `title` 不进 accessible name，缺失会导致测试静默失败）。

### 8.3 动效

- 仅用于状态过渡：拖放区 `transition: all .2s ease`、字段行 `background-color .2s`。
- 未实现 `prefers-reduced-motion` 分支。当前动效均为 200ms 级的颜色过渡，无位移与缩放，风险低；若后续引入位移动画必须补上。

### 8.4 状态覆盖

| 状态 | 处理 |
|------|------|
| 加载 | `.dr-state` / `.dr-page--center` 居中；预览区用虚线占位以免页面高度塌陷 |
| 空 | 同上容器，`Empty` 用 `PRESENTED_IMAGE_SIMPLE` |
| 错误 | 沿用 antd `message` 与 `Result`；错误色 `#ff4d4f` |
| 恢复 | 各页保留"刷新"按钮；分享页保留密码重试（错误以内联文本呈现，不用 toast） |

**已执行的验收**：`npx tsc --noEmit` 通过；`biome check` 零问题；`vitest run` 27 文件 / 309 用例全通过（含新增的 5 条骨架契约用例）。
**提议但未执行的验收**：真机逐状态目视（悬停/拖拽中/禁用、三种空态高度、1280/1440/1920 三档、移动端 Drawer）、对比度测量、色盲模拟。

---

## 9. Agent 实施提示词

```
项目：Data Insights（React 19 + TypeScript + antd 6.6.4 + Vite）
主流程：数据源 → 数据集 → 拖拽配图 → 保存 / 分享

在 Data Insights 新增或修改页面时，遵守以下约束：

1. 页面骨架固定为：<div className="dr-page"> → <PageHeader .../> → <Card>。
   页面标题绝不放回 <Card> 内。详情页的面包屑通过 PageHeader 的 breadcrumb prop 传入。
2. 只使用 styles/index.css 中 :root 已定义的 token（--dr-canvas/surface/sunken/
   border/border-strong/text-1..4/accent/accent-soft/shadow-card/dim/metric/filter/date）。
   禁止新增 HEX 字面量；确需新增语义色时先扩展 token 表并同步 docs/design-system.md。
3. 加载态与空态统一包在 .dr-state 内（高度一致）；整页级的加载/结果态用
   .dr-page--center。
4. 卡片内的工具条用 .dr-card-toolbar。
5. 语义色只在语义位置使用：维度蓝(--dr-dim) / 指标绿(--dr-metric) /
   过滤橙(--dr-filter) / 日期紫(--dr-date)。同一语义不允许有两个色值。
6. 不要为"好看"增加纵向留白；间距只取 5.2 节的刻度。
7. 改 PivotTable / TableChart / KpiCard 时注意它们在 ShareView 下也会渲染 ——
   只能用 :root 变量，不能依赖某个页面的祖先类。

预期产物：修改后的页面/组件 + 必要的 token 扩展 + docs/design-system.md 同步。
验收标准：npx tsc --noEmit 通过；npx biome check src/ 零问题；
npx vitest run 全通过（其中 surfaces.test.tsx 必须仍能断言页头不在卡片内）。

⚠️ 本文件的验证状态以第 8 节为准。自动化检查已通过；真机目视、对比度测量
与无障碍审计尚未执行，不要把它们当作已完成项。
```

---

## 参考

- 评审稿（静态复原，非截图）：`tmp/design-surfaces.html`
- 结构契约测试：`frontend/src/__tests__/pages/surfaces.test.tsx`
- token 定义：`frontend/src/styles/index.css`
- 页头组件：`frontend/src/components/PageHeader.tsx`
