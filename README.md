# DataRay

拖拽式 BI 可视化分析平台 MVP

## 功能特性

- **数据源管理** - 支持 PostgreSQL、MySQL、ClickHouse、StarRocks 数据库连接配置和连接测试
- **数据集管理** - 支持直接查询表模式或自定义 SQL 模式
- **拖拽式图表构建** - 通过拖拽字段快速创建可视化图表
  - 折线图 (Line Chart)
  - 柱状图 (Bar Chart)
  - 饼图 (Pie Chart)
- **分享功能** - 生成分享链接，支持密码保护

## 技术栈

### 前端
- React 19 + TypeScript
- Ant Design 6.x
- ECharts 6.x
- Zustand 5.x
- @dnd-kit 拖拽交互
- Vite 构建工具

### 后端
- Go 1.26+
- Gin API 框架
- uptrace/bun ORM
- PostgreSQL 12+

## 快速开始

### 使用 Docker Compose

```bash
cd data-insights
docker-compose up --build
```

服务启动后：
- 前端: http://localhost:23351
- 后端 API: http://localhost:23352

### 本地开发

#### 后端

```bash
cd backend
go mod tidy
go run ./cmd/main.go -f etc/config.yaml
```

#### 前端

```bash
cd frontend
pnpm install
pnpm dev
```

## 配置

### 后端配置 (backend/etc/config.yaml)

```yaml
Name: dataray
Host: 0.0.0.0
Port: 23352

Database:
  Url: postgres://user:password@localhost:5432/dbname?sslmode=disable
```

## API 文档

| 接口 | 方法 | 说明 |
|------|------|------|
| /api/datasources | GET/POST | 数据源列表/创建 |
| /api/datasources/:id | GET/DELETE | 数据源详情/删除 |
| /api/datasources/test | POST | 测试连接 |
| /api/datasources/:id/tables | GET | 获取数据表列表 |
| /api/datasources/:id/tables/:table/columns | GET | 获取表字段列表 |
| /api/datasets | GET/POST | 数据集列表/创建 |
| /api/datasets/:id | GET/DELETE | 数据集详情/删除 |
| /api/datasets/:id/columns | GET | 获取字段列表 |
| /api/charts | GET/POST | 图表列表/创建 |
| /api/charts/:id | GET/PUT/DELETE | 图表 CRUD |
| /api/charts/:id/data | GET | 获取图表数据 |
| /api/shares | POST | 创建分享 |
| /api/shares/:token | GET | 获取分享信息 |
| /share/:token | GET | 访问分享链接 |
| /health | GET | 后端健康检查（统一响应格式） |

## 项目结构

```
.
├── docker-compose.yml
├── frontend/                 # 前端项目
│   ├── src/
│   │   ├── api/             # API 客户端
│   │   ├── store/           # 状态管理
│   │   └── pages/           # 页面组件
│   └── Dockerfile
└── backend/                  # 后端项目
    ├── cmd/                 # 主程序
    ├── internal/
    │   ├── config/          # 配置
    │   ├── database/        # 数据库
    │   └── model/           # 数据模型
    └── Dockerfile
```

## License

MIT
