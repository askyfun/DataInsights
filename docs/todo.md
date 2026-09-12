# DataRay 开发计划汇总

> 本文档合并了项目根目录和 backend 目录的 TODO 任务，作为项目的统一任务清单。

---

## 一、项目开发计划 (MVP v1.0)

### MVP 核心功能

- **拖拽式图表构建**：通过拖拽字段快速创建图表
- **分享链接**：生成分享链接给其他人查看
- **技术栈**：Go + Gin + bun + PostgreSQL
- **测试重点**：单元测试覆盖率

> 注：MVP 版本不包含完整仪表盘和权限管理功能

---

### 阶段一：项目基础架构 ✅ 已完成

#### 1.1 项目初始化

- [x] 创建 Go 后端项目骨架
  - [x] 初始化 Go 项目结构 (go.mod)
  - [x] 配置 go.mod 依赖 (Gin, bun, pgdriver 等)
  - [x] 配置 config.toml 基础配置
- [x] 创建前端 React 项目骨架
  - [x] 使用 Vite 创建 React + TypeScript 项目
  - [x] 安装依赖 (Ant Design, ECharts, Zustand, dnd-kit, Axios, React Router)
- [x] 配置 Docker 开发环境
  - [x] 编写 docker-compose.yml (PostgreSQL)
  - [x] 编写后端 Dockerfile
  - [x] 编写前端 Dockerfile

#### 1.2 数据库设计

- [x] 设计并创建 PostgreSQL 数据库表
  - [x] bi_datasource (数据源表)
  - [x] bi_dataset (数据集表)
  - [x] bi_chart (图表表)
  - [x] bi_share (分享链接表)
- [x] 编写数据库初始化 (通过 bun 自动迁移)

#### 1.3 后端基础框架

- [x] 配置基础 HTTP 服务
- [x] 实现统一响应结构
- [x] 实现全局错误处理

---

### 阶段二：（MVP 跳过）用户与认证模块

> MVP 版本不包含用户认证功能

---

### 阶段三：数据源管理模块 ✅ 已完成

#### 3.1 后端数据源服务

- [x] 创建数据源模型
- [x] 实现数据源 CRUD 接口
  - [x] 数据源列表查询 (GET /api/datasources)
  - [x] 数据源详情查询 (GET /api/datasources/:id)
  - [x] 创建数据源 (POST /api/datasources)
  - [x] 删除数据源 (DELETE /api/datasources/:id)
- [x] 实现数据库连接测试接口 (POST /api/datasources/test)
- [x] 实现获取数据源表列表 (GET /api/datasources/:id/tables)
- [x] 实现获取表字段列表 (GET /api/datasources/:id/tables/:table/columns)
- [x] 支持多数据源类型 (抽象驱动接口)
  - [x] PostgreSQL 驱动
  - [x] MySQL 驱动
  - [x] ClickHouse 驱动
  - [x] StarRocks 驱动

#### 3.2 前端数据源模块

- [x] 创建数据源管理页面 (Datasource.tsx)
  - [x] 数据源列表组件
  - [x] 数据源新增/编辑弹窗
  - [x] 连接测试功能
  - [x] 数据源类型选择 (PostgreSQL/MySQL/ClickHouse/StarRocks)
- [x] 创建数据源详情页 (DatasourceDetail.tsx)
  - [x] 查看数据源基本信息
  - [x] 查看数据源下所有表
  - [x] 查看每个表的字段信息

---

### 阶段四：数据集管理模块 ✅ 已完成

#### 4.1 后端数据集服务

- [x] 创建数据集模型
- [x] 实现数据集 CRUD 接口
  - [x] 数据集列表查询 (GET /api/datasets)
  - [x] 数据集详情查询 (GET /api/datasets/:id)
  - [x] 创建数据集 (POST /api/datasets)
  - [x] 删除数据集 (DELETE /api/datasets/:id)
- [x] 实现获取数据源表结构接口
  - [x] 获取字段列表 (GET /api/datasets/:id/columns)

#### 4.2 前端数据集模块

- [x] 创建数据集管理页面 (Dataset.tsx)
  - [x] 数据集列表组件
  - [x] 数据集新增页面
    - [x] 数据源选择
    - [x] 表查询模式 UI
    - [x] 自定义 SQL 模式 UI

---

### 阶段五：可视化分析模块 (核心) ✅ 已完成

#### 5.1 后端查询引擎

- [x] 实现查询配置解析器
  - [x] 解析维度配置 (xAxis)
  - [x] 解析指标配置 (yAxis)
  - [x] 解析图表类型
- [x] 实现 SQL 生成器
  - [x] PostgreSQL SQL 方言生成
- [x] 实现查询执行器
  - [x] 执行查询
  - [x] 处理查询结果

#### 5.2 后端图表服务

- [x] 创建图表模型
- [x] 实现图表 CRUD 接口
  - [x] 图表列表查询 (GET /api/charts)
  - [x] 图表详情查询 (GET /api/charts/:id)
  - [x] 创建图表 (POST /api/charts)
  - [x] 更新图表 (PUT /api/charts/:id)
  - [x] 删除图表 (DELETE /api/charts/:id)
- [x] 实现图表数据查询接口 (GET /api/charts/:id/data)

#### 5.3 前端可视化分析

- [x] 创建图表工作台页面 (ChartBuilder.tsx)
  - [x] 左侧字段列表面板
  - [x] 中间画布区域 (图表预览)
  - [x] 右侧配置面板
    - [x] 图表类型选择 (柱状图、折线图、饼图)
    - [x] 维度/指标配置
- [x] 实现拖拽功能 (dnd-kit)
  - [x] 字段拖入 X 轴/Y 轴
- [x] 实现图表组件
  - [x] 柱状图 (BarChart)
  - [x] 折线图 (LineChart)
  - [x] 饼图 (PieChart)
- [x] 图表列表页面 (Charts.tsx)

---

### 阶段六：分享功能模块 (核心) ✅ 已完成

#### 6.1 后端分享服务

- [x] 创建分享链接模型
- [x] 实现分享链接接口
  - [x] 创建分享链接 (POST /api/shares)
  - [x] 获取分享链接详情 (GET /api/shares/:token)
- [x] 实现公开访问接口
  - [x] 通过分享链接查看图表 (GET /share/:token)

#### 6.2 前端分享功能

- [x] 创建分享管理页面 (Share.tsx)
  - [x] 生成分享链接
  - [x] 设置访问密码 (可选)
- [x] 实现分享链接访问页面 (ShareView.tsx)
  - [x] 密码验证 (如设置)
  - [x] 图表只读视图

---

### 阶段七：仪表盘模块 (待开发)

#### 7.1 后端仪表盘服务

- [ ] 创建仪表盘模型
- [ ] 实现仪表盘 CRUD 接口
  - [ ] 仪表盘列表查询 (GET /api/dashboards)
  - [ ] 仪表盘详情查询 (GET /api/dashboards/:id)
  - [ ] 创建仪表盘 (POST /api/dashboards)
  - [ ] 更新仪表盘 (PUT /api/dashboards/:id)
  - [ ] 删除仪表盘 (DELETE /api/dashboards/:id)
- [ ] 实现仪表盘布局管理
  - [ ] 网格布局配置
  - [ ] 自由布局配置

#### 7.2 前端仪表盘模块

- [ ] 创建仪表盘页面 (Dashboard.tsx)
  - [ ] 仪表盘列表组件
  - [ ] 仪表盘编辑页面
  - [ ] 布局调整功能（拖拽）
  - [ ] 多图表组合展示

---

### 阶段八：用户与权限模块 (待开发)

#### 8.1 后端用户服务

- [ ] 创建用户模型
- [ ] 实现用户管理接口
  - [ ] 用户列表查询 (GET /api/users)
  - [ ] 用户详情查询 (GET /api/users/:id)
  - [ ] 创建用户 (POST /api/users)
  - [ ] 更新用户 (PUT /api/users/:id)
  - [ ] 删除用户 (DELETE /api/users/:id)
- [ ] 实现角色管理接口
  - [ ] 角色列表 (GET /api/roles)
  - [ ] 创建角色 (POST /api/roles)
- [ ] 实现权限控制
  - [ ] JWT 认证
  - [ ] RBAC 权限校验

#### 8.2 前端用户模块

- [ ] 创建登录页面 (Login.tsx)
- [ ] 创建用户管理页面 (UserManagement.tsx)
- [ ] 创建角色权限页面

---

### 阶段九：协作与增强功能 (待开发)

#### 9.1 数据导出功能

- [ ] 实现数据导出接口
  - [ ] 导出为 Excel
  - [ ] 导出为 CSV
  - [ ] 导出为图片/ PDF

#### 9.2 订阅功能

- [ ] 实现定期推送
  - [ ] 邮件推送配置
  - [ ] 站内消息推送

#### 9.3 高级图表类型

- [ ] 散点图 (ScatterChart)
- [ ] 地图图表 (MapChart)
- [ ] 漏斗图 (FunnelChart)
- [ ] 仪表盘图 (GaugeChart)

#### 9.4 更多数据源支持

- [ ] ClickHouse 数据源支持
- [ ] MySQL 数据源支持 (完善)
- [ ] Oracle 数据源支持

---

### 阶段十：性能与安全 (待开发)

#### 10.1 性能优化

- [ ] 查询缓存 (Redis)
- [ ] 前端首屏加载优化
- [ ] 图表渲染性能优化

#### 10.2 安全增强

- [ ] 密码加密存储
- [ ] 审计日志
- [ ] LDAP/ OAuth 集成

---

## 二、数据集增强功能 v1.1

### 需求说明

数据集本质上是对数据库表的视图（VIEW）封装，可以在原始数据表基础上进行扩展，包括：

- 添加虚拟字段（计算字段）
- 切换字段类型（维度/指标）
- 修改字段数据类型
- 多语言支持

### 4.3 数据集详情页

- [ ] 创建数据集详情页 (DatasetDetail.tsx)
  - [ ] 显示数据集基本信息（名称、数据源、查询类型、源）
  - [ ] 显示字段列表（支持展开查看详情）
  - [ ] 显示数据预览
  - [ ] 支持编辑入口
  - [ ] 支持删除

### 4.4 数据集编辑页

- [ ] 创建数据集编辑页 (DatasetEdit.tsx)
  - [ ] 完整的数据集配置表单
  - [ ] 数据源选择
  - [ ] 表/SQL 查询模式切换
  - [ ] 字段管理（添加虚拟字段、修改类型）
  - [ ] 维度/指标切换
  - [ ] 数据类型修改

### 4.5 虚拟字段支持

- [ ] 后端模型扩展
  - [ ] 添加虚拟字段结构定义
  - [ ] 支持计算公式（SQL 表达式）
- [ ] 前端字段管理
  - [ ] 添加虚拟字段按钮
  - [ ] 虚拟字段编辑器（名称、表达式、类型）
  - [ ] 虚拟字段列表展示

### 4.6 维度/指标切换

- [ ] 前端字段管理 UI
  - [ ] 字段类型切换器（维度 ↔ 指标）
  - [ ] 维度字段列表
  - [ ] 指标字段列表
- [ ] 后端字段元数据更新
  - [ ] 保存字段角色配置

### 4.7 数据类型修改

- [ ] 前端字段管理 UI
  - [ ] 数据类型下拉选择器
  - [ ] 支持类型：字符串、数值、日期、布尔等
- [ ] 后端字段元数据更新
  - [ ] 保存数据类型配置

### 4.8 多语言支持

- [ ] 完善 i18n 翻译
  - [ ] 数据集列表页面翻译
  - [ ] 数据集详情页翻译
  - [ ] 数据集编辑页翻译
  - [ ] 字段管理翻译
  - [ ] 虚拟字段翻译
  - [ ] 维度/指标翻译
  - [ ] 数据类型翻译

---

## 三、后端优化任务清单

### 优先级 P0 (安全 - 必须修复)

- [x] 1. SQL 注入防护 - buildSQL() 和所有 SQL 拼接使用参数化查询
- [x] 2. 修复弱 Token 生成 - 使用 crypto/rand 替代可预测算法
- [x] 3. 外部数据源连接添加 timeout - 防止连接泄漏

### 优先级 P1 (稳定性)

- [x] 4. 添加 Graceful Shutdown - 优雅关闭服务器
- [x] 5. 请求参数验证 - 统一验证中间件/函数

### 优先级 P2 (可维护性)

- [x] 6. 提取重复代码 - datasource lookup 逻辑抽取为独立函数
- [ ] 7. Handler 拆分 - 从 main.go 拆分到 handler/ 目录 (取消 - 需要更大范围重构)

### 优先级 P3 (性能)

- [x] 8. 添加分页 - list 接口支持 limit/offset
- [x] 9. 数据库索引 - 为 token, datasource_id 等添加索引

### 优先级 P4 (代码质量)

- [x] 10. 添加 Request ID 追踪
- [ ] 11. 结构化日志 (取消 - 需要引入新依赖)

---

## 四、Web Interface Guidelines 修复任务

### 🚨 高优先级

#### 1. Accessibility (无障碍)

- [x] **1.1** 为 Icon Buttons 添加 aria-label
  - 文件: `Datasource.tsx`, `Dataset.tsx`, `ChartBuilder.tsx`, `Charts.tsx`, `Share.tsx`
  - 操作: 为所有带图标的按钮添加 `aria-label` 或 `title`

- [x] **1.2** 添加 Focus Visible 样式
  - 文件: 全局样式或 App.tsx
  - 操作: 添加 `:focus-visible` 样式

- [x] **1.3** 添加 Skip Link
  - 文件: `App.tsx`
  - 操作: 添加跳到主要内容区的链接

#### 2. 键盘支持

- [x] **2.1** 增强拖拽键盘支持
  - 文件: `ChartBuilder.tsx`
  - 操作: 验证 KeyboardSensor 已正确配置

### ⚠️ 中优先级

#### 3. 表单行为

- [x] **3.1** 添加表单 autocomplete
  - 文件: `Datasource.tsx`
  - 操作: 为 username/password 等字段添加 autocomplete 属性

#### 4. 焦点管理

- [ ] **4.1** 模态框焦点管理
  - 文件: `Datasource.tsx`, `Dataset.tsx`, `Share.tsx`
  - 操作: 使用 Ant Design Modal 的 `getContainer` 或 autofocus 属性

#### 5. 性能

- [ ] **5.1** 大表格虚拟化 (可选，取决于数据量)
  - 文件: Table 组件使用处
  - 操作: 评估是否需要 react-window

### 💡 低优先级

#### 6. 动画/动效

- [ ] **6.1** 添加 reduced-motion 支持
  - 文件: `ChartBuilder.tsx`
  - 操作: 检测 prefers-reduced-motion

#### 7. 其他 UI/UX

- [x] **7.1** 修复 footer 年份
  - 文件: `App.tsx`
  - 操作: ©2024 → ©2026

- [x] **7.2** 添加动态文档标题
  - 文件: 各页面组件
  - 操作: 使用 react-helmet 或 useEffect 设置 document.title

---

## 五、项目状态总结

### 模块完成状态

| 模块 | 后端 | 前端 |
|------|------|------|
| 数据源管理 | ✅ | ✅ |
| 数据集管理 | ✅ | ✅ |
| 图表构建 | ✅ | ✅ |
| 分享功能 | ✅ | ✅ |
| 仪表盘 | ❌ | ❌ |
| 用户权限 | ❌ | ❌ |
| 单元测试 | ✅ (部分) | ❌ |

**MVP 核心功能开发完成，还需补充：仪表盘、用户权限。单元测试已补充，覆盖率待提升。**

### 测试覆盖率

| 包 | 覆盖率 |
|----|--------|
| config | 100% |
| model | 0% (需要数据库) |
| database | 23% |
| cmd | 8.9% (buildSQL 100%) |

### 技术栈清单

| 层级 | 技术 | 状态 |
|------|------|------|
| 后端语言 | Go | ✅ |
| 后端框架 | Gin | ✅ |
| ORM | bun | ✅ |
| 数据库 | PostgreSQL | ✅ |
| 数据源驱动 | 抽象接口 (Driver Interface) | ✅ |
| - PostgreSQL | 驱动实现 | ✅ |
| - ClickHouse | 驱动实现 | ✅ |
| 前端框架 | React 18 + TypeScript | ✅ |
| UI 组件库 | Ant Design 5.x | ✅ |
| 可视化库 | ECharts 5.x | ✅ |
| 状态管理 | Zustand 4.x | ✅ |
| 拖拽库 | @dnd-kit | ✅ |
| HTTP 客户端 | Axios | ✅ |
| 容器化 | Docker + docker-compose | ✅ |

---

## 六、MVP 后续版本规划

### v1.1 版本

- [ ] 可视化建模（拖拽式 ETL）
- [ ] ClickHouse 数据源
- [ ] 更多图表类型

### v1.2 版本

- [ ] 完整权限体系（行级/列级安全）
- [ ] 仪表盘订阅推送
- [ ] 移动端适配

### v2.0 版本

- [ ] AI 增强能力
- [ ] 智能图表推荐
- [ ] 自然语言查询
- [ ] 插件架构

---

## 七、架构重构进度（顶层评审 → 分批落地）

> 源自 2026-09 架构评审 + grilling 的 12 项决策，分三批执行。计划与逐项验收记录见 `docs/superpowers/plans/` 与 `.superpowers/sdd/`。

### Batch 1：安全 + 查询收敛 ✅ 已合并 master

- [x] datasource 密码 AES-GCM 可逆加密、API `password` 脱敏、share bcrypt + `has_password` 契约
- [x] SQL 构造收敛到 `query` 包（AST + bun_builder 唯一出口 + raw.go）、`Connection.Execute` 参数化
- [x] goose 版本化迁移、`database.WithTx`、CORS/Sentry/requestID 运维基线

### Batch 2：契约工程 ✅ 本分支 `refactor/batch2-contract`（待最终评审 + E2E 后合并）

- [x] `api/openapi.yaml` 单一事实源；`make api-gen` 生成 `backend/internal/idls/gen_types.go` + `frontend/src/idls/gen_types.ts`（19+ 端点契约 + ChartSpec/QuerySpec）
- [x] 泛型 router 全面启用：31 个 API 端点（datasource 11 / dataset 8 / chart 7 / share 4 / health）经 `Register{Get,Post,Put,Delete}Route` 注册，行为保持由 baseline 测试逐端点钉死；修复 2 个 router 缺陷（`res` 值→指针、body 绑定按方法门控）
- [x] 图表 `bi_chart.config` 升级 v1 文档（`lib/chartConfigSchema.ts` + `migrateChartConfig`），`fieldId` 位置 `field-N`→稳定列名，**修复 ShareView 对新结构图表无法渲染**（旧门控依赖恒为 null 的 `xAxisField`）
- [x] 前端删除手写 idl 类型（chart/dataset/datasource/share.ts），错误消息读取修正为 `error.message`（信封字段是 `msg`，旧 `.response.data.message` 恒 undefined）
- [x] filters 键 `op`→`operator` 对齐后端；后端删除死代码（`getPaginationParams`、手写字符串 SQL builder 路径）；`aggExprPattern` 收紧为聚合函数白名单堵注入面；新增镜像↔entity json tag 反射守卫测试

### Batch 3：backlog 收口 + 类型消费 ✅ 分支 `refactor/batch3-backlog`（待评审+E2E 后合入）

- [x] **ghost route 修复**：新增 `PUT /api/datasets/:id`（handler+路由+契约+api-gen，复用既有 `service.Update`），前端 `datasetsApi.update` 单点 stringify；并修复全行更新会清空编辑表单未发送字段的数据丢失问题（"未提供则保留"约定，对齐 datasource 密码先例，显式 `"[]"` 仍可清空）
- [x] **饼图无效设置移除**（产品决策）：前端比例控件+三处 `config` 发送+类型删除、后端 `req.Config` 死输入链删除、openapi 注释按真相重写；`PieProcessor.MergeOtherBelowRatio` 能力保留未接线
- [x] 死代码：`query.BuildBunQuery`、ShareView 不可达 401/403 分支（先钉行为测试再删）、`lib/api/client.ts` baseURL 尾 `}`、`ChartBuilder` `valueEnd as any`×3、前端死类型导出（`GeneratedSQL`/`TestConnectionResponse`/`typeConfig` 判死/datatypes 三胞胎）
- [x] **前端类型消费生成物**（9 步迁移，每步 tsc 门+vitest 子集）：实体/响应/请求类型 alias 到 `components['schemas']` + 薄手写联合层；envelope 同名碰撞零出现（`ChartQueryResponse`→`ChartDataResult` wrapper）；`ApiResponse<T>` 重声明自生成 Envelope；净 -88 行
- [ ] **Batch 4 重评**：后端 handler In/Out 消费 `idls.*`（生成 Go 类型无 gin `form:"-"` 语义，需自定义 codegen 模板，收益/风险待评）；`updated_at` 在 PUT 保留旧值不刷新（datasource 同病，无 DB trigger）；`GetChartData` 聚合语义统一（现返回原始行、图表配置不参与取数）；`Dataset`/`Chart` 等生成类型 timestamp 必填 vs 后端部分端点返回空串的语义核对；ChartBuilder 三处 filters/query 构造点 DRY 合并；`/datasets/new` 经实测非 bug（v6 静态段优先命中 DatasetEdit，new 模式重定向回列表为既有设计），销账

### 全分支评审结论（2026-09 收口）

- 结论：**SHIP**（可合并 master），无 Critical、无行为回归；上述 Important 均为文档/测试卫生，不阻断合并。`make api-gen` 端到端重跑零漂移；15 个后端测试包 green；前端 tsc clean、vitest 68/4（4 为 master 既有 DatasourceDetail Intl 基线）；浏览器 E2E 实测 ShareView 渲染 v1 图表通过。评审驱动的文档修正（api.md `op`→`operator`、AGENTS.md 端点数 30+2 例外）已在收口 commit。

