# Docs AGENTS.md

项目文档目录，包含架构设计、API 规范、开发计划和参考资料。

## 文档清单

| 文件 | 用途 |
|------|------|
| `setup.md` | 环境搭建指南（依赖安装、数据库配置、启动命令） |
| `architecture.md` | 系统架构说明（目录结构、技术栈、分层设计） |
| `api.md` | API 接口文档（统一响应格式、各领域接口定义） |
| `api-spec.md` | API 规范详情 |
| `coding-style.md` | 代码风格指南 |
| `chart-builder-plan.md` | 图表构建器功能设计文档 |
| `todo.md` | 开发任务清单 |
| `plans/2026-02-27-frontend-code-quality.md` | 前端代码质量改进计划 |
| `DataWind/01_DataWind_Research_Analysis.md` | DataWind 产品调研分析 |
| `DataWind/02_Product_Requirements_Spec.md` | 产品需求规格 |
| `DataWind/03_Technical_Architecture_Design.md` | 技术架构设计 |

## DataWind 子目录

`DataWind/` 包含竞品研究和产品规划文档，作为 Data Insights 产品设计的参考依据。

## 维护规则

- 大型业务逻辑调整或架构调整必须同步更新此目录下的相关文档
- 新增功能应更新 `api.md` 和 `todo.md`
- 计划文档放入 `plans/` 子目录，以日期命名
