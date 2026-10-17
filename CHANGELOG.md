# 变更日志（CHANGELOG）

> 本文件记录**已合入 `master`** 的对外变更。v0.2.0 及更早版本的发布说明见 GitHub Releases。
> 尚未发布的段落是**草稿**，发版时按实际 tag 改写。

## [0.3.0] - 未发布（草稿）

### 安装

**仅 Docker（自备 PostgreSQL）**

```bash
docker pull kzzhr/datainsights:v0.3.0
docker run -d --name data-insights -p 23352:23352 \
  -e DATABASE_URL='postgres://user:password@your-db-host:5432/dbname?sslmode=disable' \
  kzzhr/datainsights:v0.3.0
```

**Docker Compose（连带 PostgreSQL，零外部依赖）**

```bash
curl -fsSLO https://raw.githubusercontent.com/askyfun/DataInsights/master/docker-compose.allinone.yml
TAG=v0.3.0 docker compose -f docker-compose.allinone.yml up -d
```

启动后访问 http://localhost:23352 。其余方式（源码运行 / 本地开发）见 [README](README.md)。

### 升级须知

- **迁移水位 `00012`**：自 v0.2.0 起新增 `00009`（三表 `owner_id`）、`00010`（用户与令牌）、
  `00011`（预警）、`00012`（内置示例数据集）。迁移随服务启动自动应用，升级前请先备份数据库。
- **登录页出现，但资源端点仍匿名**：本版引入账号体系 v1，仅为 `/api/auth/me`、`/api/auth/logout`
  与 `/api/tokens/*` 加了 Bearer 门禁；数据源 / 数据集 / 图表 / 仪表盘等既有 40 个资源端点
  **仍然免鉴权**。登录页只代表账号能力已就位，不代表对外暴露已被收紧——生产环境仍需靠网络边界
  （内网 / 网关 / 反向代理）控制访问。
- **预警需要出站 SMTP 时**：配置 `SMTP_HOST` 后需能出站访问 587/TCP（STARTTLS）。未配置时评估
  照常运行、只不发信，触发记录的 `notify_error` 会说明原因。详见[配置与参数](docs/user-guide/configuration.md)
  与[生产部署](docs/deployment/production.md)。

### 新增

- **内置示例数据集**：首次启动可一键加载示例订单数据并直达首图，开箱体验零配置（可忽略 / 移除）（#174）。
- **本地上传**：`.csv` / `.xlsx`（单文件 ≤50MB）灌入抽取存储并自动生成数据集；抽取存储支持
  starrocks / mysql / postgresql（`EXTRACT_DATASOURCE_ID`）（#138）。
- **指标阈值预警**：规则 CRUD + 触发历史 + 进程内评估 worker + 可选 SMTP 邮件通知（#155）。
- **账号体系 v1 与 PAT**：注册 / 登录 / 24h 会话、账号设置页、个人访问令牌管理（#183 / #184）。
- **查询容器**：把盘级筛选器收进一块带标题、可置顶的容器控件（#154）。
- **表格增强二期**：条件格式、单元格合并、行列转置、迷你图列（#156）。
- **归属审计列**：`bi_datasource` / `bi_dataset` / `bi_chart` 落 `owner_id`（#171）。

### 改进

- 查询期表达式白名单校验（#170）。
- 图表大数据渲染优化：抽稀 / 大图模式 + 关动画（#76）。
- API 客户端增强：GET 重试、微缓存与在途去重、细粒度错误（#99）。
- 饼图百分比显示与长尾合并（#27）。
- 前端 Store 重构为切片模块（#94）。
- 路由级代码分割（#59）、根级 ErrorBoundary（#87）、`prefers-reduced-motion` 适配（#67）。
- 未落库 / 未保存的盘级筛选器配好取值即时生效（#172）。
