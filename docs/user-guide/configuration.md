# 配置与参数（Configuration）

> 最后更新：2026-09-26
> 本文是环境变量与配置优先级的**参数字典**。完整本地环境搭建见[本地开发环境](../developer-guide/dev-setup.md)。

## 1. 配置的唯一来源

**项目只认环境变量，不提供任何配置文件**（没有 `config.toml` / `config.yaml`）。这条是刻意的：容器化部署下，每个要用户填的值都必须能从外部注入（`docker run -e` / `--env-file` / compose `environment` / K8s env），再挂一份文件只会制造"改了没生效"的歧义。

优先级（低 → 高）：

```
内置默认值  <  仓库根 .env 文件  <  真实系统环境变量
```

- 变量名**不加前缀**：后端进程是唯一读者；而 `DATABASE_URL` / `PORT` 是 12-factor 标准名，云平台（Heroku / Railway / Render / Fly）自动注入的正是裸名。
- **空字符串一律等同"未设置"**，所以 `.env` 里留空占位不会打掉内置默认值。
- **唯一例外**：前端变量必须带 `VITE_` 前缀（Vite 的硬性要求，不是风格选择）。

## 2. 起步：复制模板

```bash
cp .env.example .env    # 然后按需修改
```

`.env` 放在**仓库根**且**不入库**，前后端共用这一份（文件保持 `KEY=value` 裸格式：不加引号、不写 `export`，这样才能被 `docker run --env-file` 直接使用）。

## 3. 变量表

| 变量 | 必填 | 默认 | 说明 |
|------|------|------|------|
| `DATABASE_URL` | **是** | — | PostgreSQL 连接串。缺失时启动 fail-fast 并打印变量名，不会退化成驱动层看不懂的连接失败。 |
| `PORT` | 否 | `23352` | HTTP 监听端口。监听地址固定 `0.0.0.0`，**不提供**覆盖。 |
| `SECURITY_KEY` | 否 | 首启随机生成 | 数据源密码 AES-256-GCM 的加密密钥（64 位 hex）。**留空 = 系统首次启动随机生成（32 字节）并持久化到数据库**，重启 / 容器重建复用同一把，保证已加密密码始终可解；显式配置时环境变量优先。**生产强烈建议显式设置**。 |
| `SENTRY_DSN` | 否 | 空 | 后端 Sentry DSN，留空 = 关闭上报。 |
| `CORS_ALLOWED_ORIGINS` | 否 | 空 | 逗号分隔白名单；**留空 = 放开所有来源**（平台 API 无登录态，CORS 不构成安全边界）。填非空白名单则只回显名单内来源，供将来引入认证后收紧。 |
| `STATIC_DIR` | 否 | 空 | 前端构建产物目录。设置后本进程一并托管页面（未命中回落 `index.html`）；**留空 = 只提供 API**（本地开发默认形态，页面走 Vite dev server）。目录配错即启动失败。 |
| `VITE_API_BASE_URL` | 否 | 空 | 前端 API 基础地址。未配置时：生产构建自动同源、开发回退 `http://<当前访问主机名>:23352`。**仅前后端分开部署时**才需配成非空绝对地址。 |
| `VITE_SENTRY_DSN` | 否 | 空 | 前端 Sentry DSN。留空 = 完全跳过 `Sentry.init`，不打点也不上报。 |

> 前端 API 基础地址的**唯一来源**是代码里的 `resolveApiBaseURL`（前端 `lib/api/client.ts`），`VITE_API_BASE_URL` 只是它读取的环境变量。

## 4. 前后端共用一份 `.env`

- 后端读**裸名**变量（如 `DATABASE_URL`），前端只消费 `VITE_` 前缀；Vite 通过配置把 `.env` 目录指向仓库根，因此两者共读同一份文件。
- `docker run --env-file .env ...` 可直接使用；编排平台则用各自的 env 注入方式。

## 5. 注意事项

- ⚠️ **`VITE_*` 是「构建期内联」**：值在 `vite build` 时被写进 JS 产物，改完必须重新 build 才生效；也正因如此**绝不可**在里面放密钥。
- ⚠️ `.dockerignore` **必须排除 `.env`**，否则 Vite 会把 `VITE_*` 内联进浏览器包。
- compose 会读根 `.env` **做变量插值**（`docker-compose.yml` 里的 `${...}`），同时又用 `env_file` 把整份变量注入容器 —— 别把无关工具的变量混进这个文件。
- 静态托管下 `/api`、`/mcp`、`/health` 及其子路径走后端（匹配不到即 JSON 404，**不会回落成 HTML**），其余路径查 `STATIC_DIR` 里的文件、未命中回落 `index.html` 交给前端路由。带内容哈希的 `assets/` 产物走 `immutable` 长缓存，其余 `no-cache`。
