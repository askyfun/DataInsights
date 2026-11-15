# 备份与恢复（Backup & Restore）

> **硬前提：schema 迁移随启动自动应用且不可逆。一旦新版本带着迁移启动，回滚的唯一手段就是 restore 备份** —— 没有 down 迁移脚本，升级失败时「把镜像换回旧 tag」不能还原数据库。
>
> 恢复到的环境必须使用与备份时**一致的 `SECURITY_KEY`**（数据源密码用 AES-256 加密落库，key 换了密文解不开）。key 的来源与优先级见[生产部署 §7](production.md)。

一键脚本：仓库内 `scripts/backup.sh`（备份 + sha256 校验和）与 `scripts/upgrade.sh`（备份 → 迁移前置检查 → 换版本 → ready 探测 → 失败打印回滚命令）。下文是两个脚本等价的手工命令，排查脚本行为时以这里为准。

> **容器名提示**：v0.4.0 起 compose 的容器名由 `data-insights-app` / `data-insights-postgres` 改为
> `tablewave-app` / `tablewave-postgres`（产品更名 TableWave）。下文统一用新名；从 v0.3.1 升上来的
> 部署此刻跑着的仍是**旧名**，两个脚本已同时识别这两个名字（按 `docker ps` 自动探测），
> 手工命令请照你机器上 `docker ps` 的实际输出替换。**升级时先读 §3.0** —— 名字变了而旧容器还在跑，
> 会让 `compose up` 直接撞端口。

## 1. 备份（pg_dump 自定义格式）

`-Fc` 产出压缩的自定义格式，配合 `pg_restore` 可选择性恢复，是本文唯一推荐的备份形态。

### 1.1 Compose 部署（容器内 pg_dump，推荐）

```bash
# 备份到宿主机当前目录，文件名自带时间戳
docker exec tablewave-postgres pg_dump -Fc -U insights insights \
  > "di-backup-$(date +%Y%m%d-%H%M%S).dump"

# 打印校验和（记录下来，恢复前先验）
shasum -a 256 di-backup-*.dump     # macOS；Linux 用 sha256sum
```

或直接用一键脚本（自动命名 `di-backup-<版本>-<时间戳>.dump` 并打印 sha256）：

```bash
scripts/backup.sh                    # 输出到 ./backups/
scripts/backup.sh --output-dir /path/to/dir
```

### 1.2 外部 PostgreSQL（本机有 pg_dump 时）

```bash
pg_dump -Fc "postgres://user:password@db-host:5432/dbname?sslmode=disable" \
  > "di-backup-$(date +%Y%m%d-%H%M%S).dump"
```

### 1.3 命名卷整卷备份（pg_dump 的兜底等价物）

pg_dump 拿不到（如库已起不来）时，可直接打包数据卷。卷名以 `docker volume ls` 实际输出为准（compose 项目名前缀；从 v0.3.1 升上来的部署前缀可能仍是旧目录名，例如 `data-insights_postgres_data`，不要照抄示例）：

```bash
# 备份整卷（容器在不在都能跑）
docker run --rm -v tablewave_postgres_data:/data:ro -v "$PWD:/backup" alpine \
  tar czf "/backup/di-pgdata-$(date +%Y%m%d-%H%M%S).tar.gz" -C /data .

# 恢复整卷（先确保卷为空或不存在，再解包）
docker run --rm -v tablewave_postgres_data:/data -v "$PWD:/backup:ro" alpine \
  sh -c 'rm -rf /data/* /data/..?* /data/.[!.]* ; tar xzf /backup/di-pgdata-<ts>.tar.gz -C /data'
```

⚠️ 整卷备份绑定的是**当时的 PostgreSQL 大版本数据目录**，跨大版本恢复可能不被新版 postgres 镜像接受；`pg_dump` 逻辑备份无此限制，优先用 §1.1/§1.2。

## 2. 恢复（pg_restore）

```bash
# 恢复前先验校验和
shasum -a 256 di-backup-<file>.dump

# 1) 停掉 app（防止恢复过程中有写入）
docker compose -f docker-compose.allinone.yml stop app

# 2) 恢复：--clean 先删对象再建，--if-exists 不因对象不存在而中断
docker exec -i tablewave-postgres \
  pg_restore --clean --if-exists -U insights -d insights < di-backup-<file>.dump

# 3) 重新拉起（版本可以与备份时相同；换更旧的版本 = 回滚，SECURITY_KEY 必须一致）
docker compose -f docker-compose.allinone.yml up -d
```

外部数据库等价命令：

```bash
pg_restore --clean --if-exists \
  "postgres://user:password@db-host:5432/dbname?sslmode=disable" \
  di-backup-<file>.dump
```

## 3. 升级与回滚 runbook

### 3.0 从 v0.3.1 升到 v0.4.0：先删旧容器（本版特有，必须）

v0.4.0 的 compose 文件把 `container_name` 从 `data-insights-*` 改成了 `tablewave-*`。compose 按
`container_name` 认容器，名字一变它就**找不到旧容器**，于是新建一套；而 23352 / 5432 还被旧容器
占着，`up -d` 直接失败（`port is already allocated`）—— 报错指向端口与网络，很容易被误判成环境问题。

**数据在命名卷里，删容器不丢数据。** 升级前先执行：

```bash
docker rm -f data-insights-app data-insights-postgres   # 只删容器，卷保留
curl -fsSLO https://raw.githubusercontent.com/askyfun/TableWave/master/docker-compose.allinone.yml
TAG=v0.4.0 docker compose -f docker-compose.allinone.yml up -d
```

`scripts/upgrade.sh` 会在前置检查阶段拦下这个场景并打印同样的命令 —— 它自己**不**删容器
（删容器是破坏性动作，交给操作者拍板）。

完整步骤（含**一次性数据迁移的顺序红线**）由 `scripts/upgrade.sh` 承载：

```bash
scripts/upgrade.sh --dry-run            # 只做备份 + 打印迁移前置检查 + 验证 ready 探测，不换版本
scripts/upgrade.sh v0.3.1               # 备份 → 迁移前置检查 → TAG=v0.3.1 换版本 → 轮询 ready 60s
```

### 迁移顺序红线（为什么顺序不能颠倒）

「一次性数据迁移」与「启动自动 schema 迁移」是**两套机制**：

- `-migrate-column-ids`（`backend/cmd/main.go`）是**一次性业务数据迁移**：把图表/仪表盘配置里的列引用从「列名」改成「稳定 id」，默认 dry-run 只扫描出报告，带 `-apply` 才写库；
- schema 迁移（goose）随新版本**启动自动应用**。

正确顺序是 **dry-run 看报告 → `-apply` 写库 → 再放量新版本**。顺序颠倒时，新进程读到未迁移的旧引用会**静默丢弃过滤条件**（返回放大后的错数而非报错），不会有任何报错提示你出了问题。

```bash
# 升级到「列 ID 契约」版本（何时需要：CHANGELOG 中标注需先跑一次性迁移的版本）
docker exec tablewave-app ./server -migrate-column-ids           # ① dry-run，只出报告
docker exec tablewave-app ./server -migrate-column-ids -apply    # ② 报告确认无误后写库
# ③ 然后才执行升级（scripts/upgrade.sh <TAG> 或手工换镜像 tag）
```

### 升级失败回滚

`upgrade.sh` 轮询 ready 60 秒拿不到 200 时会**自动打印**下面两条命令（判据是 ready 探测，不是容器状态 —— 迁移失败时进程根本不监听端口，容器看着是重启循环但 `docker ps` 未必报错）：

```bash
# 1) 用旧版本镜像把容器拉起来（<old-tag> 换成升级前的版本）
TAG=<old-tag> docker compose -f docker-compose.allinone.yml up -d

# 2) 库已被新版本的迁移改坏时，恢复到升级前的备份
docker exec -i tablewave-postgres \
  pg_restore --clean --if-exists -U insights -d insights < backups/di-backup-<file>.dump
```

回滚到旧版本时同样注意 `SECURITY_KEY` 一致性，见[生产部署 §7](production.md)；跨大版本升级前先读 `CHANGELOG.md` 的破坏性变更段。
