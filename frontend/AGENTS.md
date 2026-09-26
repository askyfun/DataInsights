# Frontend AGENTS.md

Data Insights 前端应用，拖拽式 BI 可视化分析平台的用户界面。

## 技术栈

React 19 + TypeScript + Ant Design 6.x + ECharts 6.x + Zustand 5.x + @dnd-kit + Vite 8 + Biome + Vitest

## 目录结构

```
frontend/src/
├── main.tsx              # 入口：Sentry、i18n、BrowserRouter
├── App.tsx               # 根组件：Layout + 路由 + 导航菜单
├── api/
│   ├── index.ts          # API 模块封装（datasourcesApi/datasetsApi/chartsApi/dashboardsApi/sharesApi/queriesApi）；只消费 lib/api/client 的单一 axios 实例，实体/响应类型来自 idls/gen_types.ts
│   └── datatypes.ts      # 标准数据类型系统 + 多数据库类型映射
├── lib/
│   ├── api/client.ts     # 持有唯一 axios 实例（baseURL 由 resolveApiBaseURL 决定）+ get/post/put/del 泛型辅助
│   ├── chartConfigSchema.ts  # bi_chart.config 的 version:2 文档 schema + migrateChartConfig（旧结构加载时自动迁移）
│   ├── dateFilter.ts     # 日期筛选语义单一事实源（纯逻辑、零 UI 依赖；语义细节见根 AGENTS.md「日期筛选」）
│   ├── dashboardFilterValue.ts  # 盘级筛选器取值下发契约（按算子分流 value 形状）
│   ├── dashboardLayoutSchema.ts # 仪表盘 layout schema（filter widget 的 binding.column 是列 ID）
│   ├── chartOptions.ts   # ECharts option 构建
│   ├── querySpec.ts      # store QueryConfig → 后端 QuerySpec 的映射
│   ├── format.ts         # 数值/单位格式化
│   └── dataTypes.ts      # 数据类型工具
├── store/
│   └── index.ts          # 单一 Zustand Store（全部应用状态 + 异步 actions）
├── idls/                 # API 类型定义（现仅 gen_types.ts 生成物；Batch 2 已删除手写类型）
├── pages/                # 页面组件（每个页面自包含）
│   ├── Datasource.tsx        # 数据源 CRUD 列表
│   ├── DatasourceDetail.tsx  # 数据源详情（表/列浏览）
│   ├── Dataset.tsx           # 数据集 CRUD 列表（多步创建向导）
│   ├── DatasetDetail.tsx     # 数据集详情（字段 + 预览）
│   ├── DatasetEdit.tsx       # 数据集编辑表单
│   ├── ChartBuilder.tsx      # 拖拽式图表构建器（核心功能）
│   ├── Charts.tsx            # 已保存图表列表
│   ├── Dashboards.tsx        # 仪表盘列表（首页 `/`）
│   ├── DashboardEditor.tsx   # 仪表盘编辑器（12 列栅格布局）
│   ├── Share.tsx             # 分享链接管理
│   └── ShareView.tsx         # 公开分享视图（密码保护）
├── components/
│   ├── ChartBuilder/     # 图表构建器组件
│   │   ├── DraggableField.tsx    # 可拖拽字段标签（侧边栏）
│   │   ├── FieldDropZone.tsx     # 可放置区域（维度/指标/过滤器）
│   │   ├── FieldPill.tsx         # 字段标签（含聚合下拉菜单）
│   │   ├── FieldSettingsModal.tsx # 指标属性弹窗（别名/单位/格式）
│   │   ├── FilterDropZone.tsx    # 筛选区放置位
│   │   ├── FilterConfigModal.tsx # 过滤配置弹窗（弹窗式筛选交互，文案全线"筛选"）
│   │   ├── QueryConfigRow.tsx    # 维度/指标配置行包装
│   │   ├── QueryPanel.tsx        # 多选维度/指标面板
│   │   ├── KpiCard.tsx           # KPI 卡片
│   │   ├── PivotTable.tsx        # 透视表
│   │   ├── TableChart.tsx        # 表格/透视图渲染器
│   │   └── chartDefinitions.ts   # 图表类型定义
│   ├── DateFilter/       # 日期筛选（DateFilterModal 完整弹窗 + DateFilterControl 行内控件，共用 DateFilterEditor 编辑体）
│   ├── DashboardFilterBlock/ # 仪表盘盘级筛选器（块 + 添加筛选器弹窗）
│   ├── ChartView/        # 图表渲染容器
│   └── ClickToEdit.tsx / ModalFooter.tsx / LoadingPlaceholder.tsx / PageHeader.tsx / listPage.tsx  # 公共组件
├── i18n/
│   └── useLocale.ts      # 国际化 hook + 中英文消息字典
├── styles/
│   └── index.css         # 全局 CSS + 响应式媒体查询
└── __tests__/            # 测试文件
```

## 核心架构

### 状态管理

单一 Zustand Store（`store/index.ts`），管理所有应用状态：
- `datasources/datasets/charts` — CRUD 数据数组 + loading/error 状态
- `chartBuilderFields` — 当前数据集的可用字段
- `chartBuilderConfig` — 图表类型、标题、轴配置
- `queryConfig` — 维度组、指标组、过滤器、排序、限制
- `metricAggregations/metricAliases` — 每字段聚合和别名覆盖
- `autoQuery` — 自动查询开关
- `tablePagination/tableColumns` — 表格分页状态
- `chartQueryResponse` — 最后查询响应（含生成的 SQL）

### API 层

- `lib/api/client.ts` — 持有**唯一** axios 实例（baseURL 由 `resolveApiBaseURL` 决定：生产同源、开发回退 `http://{hostname}:23352`），提供 `get/post/put/del` 泛型辅助，含 request ID 注入、code 20000 验证、Sentry 错误上报
- `api/index.ts` — 定义所有 API 模块（datasourcesApi/datasetsApi/chartsApi/dashboardsApi/sharesApi/queriesApi），**只消费** `lib/api/client` 的实例，**不得再 `axios.create`**
- 响应拦截器验证 `code === 20000` 才算成功

### 路由

| 路径 | 组件 | 说明 |
|------|------|------|
| `/` | DashboardsPage | 首页（仪表盘列表） |
| `/dashboards/:id` | DashboardEditor | 仪表盘编辑器 |
| `/datasources` | DatasourcePage | 数据源列表 |
| `/datasources/:id` | DatasourceDetailPage | 数据源详情 |
| `/datasets` | DatasetPage | 数据集列表 |
| `/datasets/new` | DatasetEdit | 新建数据集 |
| `/datasets/:id` | DatasetDetail | 数据集详情 |
| `/datasets/:id/edit` | DatasetEdit | 编辑数据集 |
| `/chart-builder` | ChartBuilder | 图表构建器（支持 `?edit={id}&datasetId={id}`） |
| `/charts` | ChartsPage | 图表列表 |
| `/shares` | SharePage | 分享管理 |
| `/share/:token` | ShareView | 公开分享视图 |

### 拖拽实现

使用 `@dnd-kit/core` + `@dnd-kit/sortable`：
- 侧边栏 `DraggableField` → 拖入 `FieldDropZone`
- 字段排序使用左右箭头按钮（非拖拽排序，避免 droppable/sortable 冲突）
- `arrayMove` 重排维度/指标字段顺序

## 常用命令

```bash
cd frontend
pnpm install             # 安装依赖（npm/yarn/bun 会被 preinstall 钩子硬拦截）
pnpm dev                 # 开发服务器，端口 23351
pnpm build               # tsc + vite 构建
pnpm test                # vitest 运行测试
pnpm check               # biome lint + 格式化检查
pnpm build:check         # biome check + vitest（提交前验证）
pnpm format              # biome 格式化
pnpm lint                # biome lint
```

## 约束

- TypeScript strict 模式，禁止 `any`（用 `unknown`）、禁止 `@ts-ignore`/`as any`
- API 通信使用 snake_case（与后端一致）
- 所有组件使用函数式组件 + React.FC + 显式 props 接口
- Biome 格式化：2 空格缩进，100 字符行宽
- 提交前必须通过 `pnpm build:check`
- 路径别名 `@/*` 映射到 `./src/*`
