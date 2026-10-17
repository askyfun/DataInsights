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
  与 `/api/tokens/*` 加了 Bearer 门禁；数据源 / 数据集 / 图表 / 仪表盘等其余 58 个资源端点
  **仍然免鉴权**。登录页只代表账号能力已就位，不代表对外暴露已被收紧——生产环境仍需靠网络边界
  （内网 / 网关 / 反向代理）控制访问。
- **口令分享其实早在 v0.2.0 就下线了**（补记，非本版变更）：图表口令分享（免登录只读视图 +
  密码 + 过期）连同 `/api/shares` 四端点与 `GET /share/:token` 浏览器入口，自 `6af5732`
  （issue #112，随 **v0.2.0** 发布）起已整体移除，历史分享链接一律失效；`bi_share` 表由迁移
  `00008` 标注 RETIRED 但**不 DROP**（历史行留着可审计）。v0.2.0 的发布说明与仓库文档当时都
  没有同步这件事，`AGENTS.md` / `backend/AGENTS.md` / `api.md` 等仍把 share 写成现状——
  本版已一并校正。**现在唯一的对外分享形态是查询记录短码直链 `?q=<短码>`**（不可变快照）。
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
- **列表服务端分页与排序**：数据源 / 数据集 / 图表 / 仪表盘四类列表改为服务端分页，
  消除此前「最多 100 条」的静默截断，并支持显式排序键（#205）。
- **本地上传零配置**：未显式配置抽取存储时，自动复用应用自身的 PostgreSQL 作为上传目标，
  单条 `docker run` 起来即可用文件上传建数据集，无需再建一个数据源（#201 / R-85）。

### 改进

- 查询期表达式白名单校验（#170）。
- 图表大数据渲染优化：抽稀 / 大图模式 + 关动画（#76）。
- API 客户端增强：GET 重试、微缓存与在途去重、细粒度错误（#99）。
- 饼图百分比显示与长尾合并（#27）。
- 前端 Store 重构为切片模块（#94）。
- 路由级代码分割（#59）、根级 ErrorBoundary（#87）、`prefers-reduced-motion` 适配（#67）。
- 未落库 / 未保存的盘级筛选器配好取值即时生效（#172）。
- 内置示例数据集可忽略 / 移除，「加载」创建的示例数据源不出现在数据源列表、也不能被用户
  新建数据集取数（#174 的守卫）。

### 修复

- **表达式围栏的两处绕过**（安全，均影响 `bi_dataset.columns` 的虚拟字段 `expr`，它经
  `query` 包原样内插进 SQL）：
  - 一元负号位漏过的 SQL 行注释序列——拒绝 `--` 的守卫原先只住在二元运算符分支，
    `--foo` 会被一元分支把两个减号各自吃掉而放行，落到单行 SQL 后整条尾巴被当注释吞掉。
  - 字符串字面量内的反斜杠——围栏只认标准 SQL 的引号双写转义，而 MySQL / StarRocks /
    ClickHouse 默认把「反斜杠+引号」当转义引号，同一串文本在两种方言下的字符串边界不同，
    字面量里的 `OR`、保留字与注释序列会在这些方言上翻进代码位。现含反斜杠的字面量一律拒绝。
    两处都配了拒绝用例，并留为 `go test` 每次提交都跑的确定性 fuzz 种子。
- **未知过滤算子显式拒绝 + 别名归一化**（#204）：此前未识别的算子会静默降级，查询照跑但
  过滤条件被丢弃（表现为「筛了没反应」），现在直接报错；输出列别名一并归一。
- 查询容器内筛选器残留的 `unsaved` prop（#185 删该 prop 后遗留，随 #200 合并清掉）。
- `kin-openapi` 依赖升级（安全修复）。
