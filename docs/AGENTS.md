# Docs AGENTS.md

> 最后更新：2026-09-26

本目录是 Data Insights 的**入库文档**（随代码版本化），按**四区**组织：新手入门、用户指南、部署指南、开发者指南。本文件是本目录的索引与维护规则。

## 分区

| 分区 | 目录 | 用途 |
|------|------|------|
| 新手入门 | `getting-started/` | 核心概念、应用场景、对外 roadmap 与极简上手 |
| 用户指南 | `user-guide/` | 配置字典与核心功能使用说明（含「为什么做这个功能」） |
| 部署指南 | `deployment/` | 容器 / Compose、发布流程与生产环境部署实践 |
| 开发者指南 | `developer-guide/` | 架构、API、图表查询设计、排障、本地环境、活跃待办、贡献规范、表面系统（摊平、无子目录） |

## 入库 vs 本机（红线）

- **入库**（可链接）：`docs/**` 下的一切，**含** `developer-guide/troubleshooting.md` 与 `developer-guide/backlog.md` —— 这两份由本机笔记**分流脱敏**而来，是入库文档，可直接链接。
- **本机**（不入库，**不得**从入库文档链接）：`docs/pitfalls.md`、`docs/backlog.md`、`docs/archive/`、`docs/superpowers/`、根 `MEMORY.md` 等。
- 需要提及时**只写「本地工作笔记」，不给路径**。

## 文档清单

| 文件 | 用途 | 维护时机 |
|------|------|----------|
| `getting-started/overview.md` | 核心概念、应用场景与对外 roadmap | 产品定位、核心概念或对外路线调整时 |
| `getting-started/quick-start.md` | 极简上手（四条部署路径的完整步骤：预览站 / 单条 Docker / Compose / 源码） | 上手路径、镜像来源或前置条件变化时 |
| `user-guide/configuration.md` | 配置与参数字典 | 环境变量、默认值或配置优先级变化时 |
| `user-guide/features.md` | 核心功能使用说明（含「为什么做」） | 新增 / 调整面向用户的功能时 |
| `deployment/docker.md` | 容器镜像（发布镜像与本地构建两条路）、Compose、镜像构建细节 | 镜像形态、Compose、镜像来源或构建细节变化时 |
| `deployment/release.md` | 发布流程（打 tag、推 Docker Hub 镜像、建 Release）与失败处置 | 发布工作流、镜像命名/架构、Secrets 或版本号规则变化时 |
| `deployment/production.md` | 生产环境部署实践 | 部署形态、安全实践或可观测性变化时 |
| `developer-guide/architecture.md` | 系统架构 + 关键设计决策 | 新增分层 / 目录、查询链路、部署形态或关键决策变更时 |
| `developer-guide/api.md` | API 接口文档（叙述性视图；契约事实源是 `../api/openapi.yaml`） | 新增或修改端点时，与 `../api/openapi.yaml` 同步更新 |
| `developer-guide/chart-query-design.md` | 图表查询链路设计（契约模型 / SQL 生成 / 处理器） | 图表配置文档结构、查询协议或可视化语义变更时 |
| `developer-guide/troubleshooting.md` | 排障（外部开发者照抄本仓也会踩的坑） | 遇到可复现的公共坑时 |
| `developer-guide/backlog.md` | 活跃待办 / 方言能力矩阵 | 活跃项推进或新增能力缺口时 |
| `developer-guide/dev-setup.md` | 本地开发环境搭建（依赖、数据库、启动命令） | 前置依赖、端口、Makefile 目标或包管理约束变化时 |
| `developer-guide/design-system.md` | 前端表面系统规范（页面骨架、色彩、字体、组件样式） | 全站视觉主题、共享样式或 antd 主题令牌调整时 |
| `developer-guide/list-page-conventions.md` | 列表页交互规范（搜索/排序/标题跳转/分页/内联编辑与复用件） | 新增列表页、或列表复用件交互变化时 |
| `developer-guide/contributing.md` | 贡献流程、提交前门禁与语言级编码规范 | 引入新工具链、团队约定或门禁变化时 |
| `AGENTS.md` | 本目录索引与维护规则 | 本目录增删文档时 |

## 维护规则

- 大型业务逻辑或架构调整必须同步更新本目录下的相关文档。
- 新增 / 修改接口先改 `../api/openapi.yaml`（事实源），再更新 `developer-guide/api.md` 并跑 `make api-gen` 同步双端类型。
- **入库文档不得引用 gitignored 文件**（本机的 `docs/pitfalls.md`、`docs/backlog.md`、`docs/archive/`、`docs/superpowers/`、根 `MEMORY.md` 等）。需要提到时只写「本地工作笔记」，不给路径链接。
- 入库的 `developer-guide/troubleshooting.md` / `developer-guide/backlog.md` 由本机笔记**分流脱敏**而来，可直接链接；但**不得**链接其本机源文件。
- 新增 / 删除文档必须同步本索引与根 `AGENTS.md` 的「开发资源」表。
- 新建 / 重写文档在标题下加一行 `> 最后更新：YYYY-MM-DD`。
