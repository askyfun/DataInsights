# 图表构建页 · 表面系统（Visual System）

> 适用范围：`/chart-builder` 页面及其组件（`pages/ChartBuilder.tsx`、`components/ChartBuilder/*`）。
> token 定义位置：`frontend/src/styles/index.css` 的 `.chart-builder-page` 作用域。
> 评审稿：`tmp/design-chart-builder-surfaces.html`（改动前 / 改动后静态复原稿，非截图）。

## 1. 问题诊断

用户反馈：整页以白为主、颜色单调、显得简陋。

取证后的根因 **不是"白色太多"，而是面与面之间没有明度差**：

| 现象 | 取证位置 |
|------|----------|
| 页面只有 `#fff` / `#fafafa` 两个明度值，且左右栏是 `#fff`、卡片也是 `#fff` | `ChartBuilder.tsx` 三栏内联样式 |
| 卡片被本页 CSS 显式摘掉唯一边界线索 | `index.css`：`.chart-builder-page .ant-card-bordered { border: none }` |
| antd 6 的 Card `headerBg` 默认是 `transparent`，无边框后卡头完全融进卡体 | `antd/es/card/style/index.js`（`headerBg: 'transparent'`） |
| 全页最大一片白 = 预览空态 | `ChartCanvas` 的 `Empty` + `padding: '100px 0'` |
| 顶栏品牌标记从未定义样式 | `App.tsx` 用了 `.demo-logo`，全仓无该 CSS 规则 → 渲染成 0×0 空 div |
| 「维度」有两个蓝 | `#1890ff`（`dropZoneStyles` / `QueryConfigRow` / 两个 DropZone 的菜单圆点）与 antd 预设 `blue` = `#1677ff` 并存 |
| 灰阶暖冷混用 | `#999` / `#595959` / `#f5f5f5` / `#fafafa` / `#f0f0f0`（暖）与 `#f8f9fa`（冷）并存 |

**边界**：此前"屏幕利用率优先"的收紧（三栏 `padding: 0`、卡间距 6、卡体 `4/6`、控件高度）是**刻意**的，本次不动。
本轮要解决的是"密而不精"，不是"太密"。

## 2. 设计方向

**中性灰阶分层（neutral elevation）** —— 一句话：把"一片白"换成"三层灰 + 一条蓝"。

- 谱系：GitHub Primer 的 surface / border 三层模型（专为浅色密集运维界面设计）+ Linear 的强调色纪律（一色只做选中与主操作）。
- 明确**不**采用：深色模式、玻璃拟态、装饰性渐变、为"高级感"加留白。
- 五条不可让步的原则：
  1. 任何两块相邻的面，必须存在明度差**或**边界线，不能两者皆无。
  2. 描边定义边界、投影定义高度，两者同时存在才算"浮起来"。
  3. 颜色只出现在有含义的位置（分组 / 字段类型 / 选中 / 主操作），不做装饰着色。
  4. 同一语义只能有一个色值（维度蓝 = `#1677ff`，不允许第二处 `#1890ff`）。
  5. 空隙只服务于"看得清"，不为观感增加纵向留白。

## 3. 表面系统（token）

定义于 `styles/index.css` 的 `.chart-builder-page`，**页面作用域**——因为本页三栏是自绘的（`Sider`/`Content` 都是内联样式），`main.tsx` 的全局 token 管不到；先在本页定性，撑得住再提升为全局。

| 变量 | 值 | 角色 |
|------|-----|------|
| `--dr-canvas` | `#f4f5f7` | L0 画布：一切非卡片区域（三栏同源） |
| `--dr-surface` | `#ffffff` | L1 面板：卡片、浮层 |
| `--dr-sunken` | `#f7f8fa` | L2 下沉：拖放区、占位区、分段容器 |
| `--dr-border` | `#e6e8eb` | 常规发丝线（卡片描边、卡头下缘） |
| `--dr-border-strong` | `#d9dde3` | 需要被看见的线（表头下缘、虚线框） |
| `--dr-text-1/2/3/4` | `#1f2328` / `#57606a` / `#8c95a1` / `#b0b7c3` | 标题 / 正文 / 计数辅助 / 占位图形 |
| `--dr-accent` | `#1677ff` | 唯一强调色 |
| `--dr-shadow-card` | `0 1px 2px rgba(16,24,40,.04)` | 卡片"贴地"级投影 |

**灰度层级用变量，语义色用常量。** 语义色三处同源，不允许分叉：

| 语义 | HEX | 出现位置 |
|------|-----|----------|
| 维度 | `#1677ff` | 分组色条、槽位色条、拖放区悬停描边、字段类型点、落点高亮 |
| 指标 | `#52c41a` | 同上 |
| 过滤 | `#fa8c16` | 同上 |
| 日期维度 | `#722ed1` | 字段类型点、字段标签（antd `purple`） |

## 4. 视觉语法（两条规则，别混）

- **3px 圆角色条 = 这是一个分组/槽位**。出现在：左栏字段分组头、中栏槽位标签。
- **5px 圆点 = 这是一个字段**，颜色 = 字段类型（文本蓝 / 日期紫 / 指标绿），与字段标签色同源。
- 形状与尺寸不同 → 两个层级不会被读混；颜色同源 → "维度是蓝的"只需要学一次。

## 5. 改动清单

| 文件 | 改了什么 |
|------|----------|
| `styles/index.css` | 新增 `.chart-builder-page` token 块与卡片/卡头/表头规则（替换原 `border: none`）；补齐 `.demo-logo` 样式 |
| `pages/ChartBuilder.tsx` | 三栏底色改 `--dr-canvas`、移除两条竖向分栏线；数据集选择器改描边控件 + 补下拉箭头；新增 `FieldGroupHeader` / `QueryStatusBadge`；预览空态与 loading 改为居中 + 虚线占位；可视化类型分组行加下沉面容器；SQL 弹窗代码块统一 |
| `components/ChartBuilder/QueryConfigRow.tsx` | 槽位标签 48px 彩色文字 → 60px 色条 + 深色文字 |
| `components/ChartBuilder/DraggableField.tsx` | 字段行加类型圆点；悬停黑色蒙层 → 品牌蓝浅底；导出 `fieldTagColor` 收窄为字面量联合类型 |
| `components/ChartBuilder/dropZoneStyles.ts` | 拖放区改用 `--dr-sunken` / `--dr-border-strong`；维度蓝 `#1890ff` → `#1677ff` |
| `components/ChartBuilder/FieldDropZone.tsx` / `FilterDropZone.tsx` | 菜单圆点统一 `#1677ff`；占位文字 `#999` → `--dr-text-3` |
| `components/ChartBuilder/PivotTable.tsx` | 小计/合计行底色转冷调（`#f7f8fa` / `#eef1f5`）；**不用 CSS 变量**——本组件在 `ShareView` 下也会渲染，不在 `.chart-builder-page` 内 |

## 6. 未做 / 待拍板

- 顶栏品牌标记是**占位标记**（品牌色圆角方块 + 三道柱），非正式 logo 资产；正式矢量 logo 到位后整块替换。
- **其余 8 个页面**（Charts / Dataset / DatasetDetail / DatasetEdit / Datasource / DatasourceDetail / Share / ShareView）仍为 `padding: 24px` + 白底白卡，同样存在"面无层次"问题。本轮刻意**未动**，避免超出本次范围。是否把本页 token 提升为全局（改 `main.tsx` + 各页顶层容器），待确认。
- 全局圆角仍为 6/8 两档混用、字段芯片仍为 5px：属既有约定，未动。
- 移动端分支只同步了画布底色与预览卡状态徽标，未做单独设计。

## 7. 验收清单

- [x] `npx tsc --noEmit` 通过
- [x] `npx biome check` 无新增问题（2 个既有 a11y 告警 + 6 个既有 `!important` 告警，均非本次引入）
- [x] `npx vitest run src/__tests__/pages/ChartBuilder*.tsx src/__tests__/components/` → 112/112 通过
- [ ] **未做**：真实浏览器逐状态目视（本沙箱不做浏览器自动化）。需人工在 `http://localhost:23351/chart-builder?edit=14&datasetId=2` 核对：
  - [ ] 默认 / 悬停 / 拖拽中（`isOver` 高亮 ≡ 语义色）/ 选中图型 / 禁用（未选数据集）
  - [ ] 空态（未选数据集 / 已选但未配字段 / 查询返回空）三态高度是否一致
  - [ ] 左栏宽度拖到 120px 下限时，槽位标签「X 轴指标」是否折行（已按 60px 预留，需实测）
  - [ ] 1280 / 1440 / 1920 三档宽度下三栏比例
