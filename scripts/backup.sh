#!/usr/bin/env bash
# backup.sh —— 数据库一键备份：产出 di-backup-<版本>-<时间戳>.dump 并打印 sha256 校验和。
#
# 用法：
#   scripts/backup.sh [--output-dir <dir>]
#
# 取数路径二选一（自动探测）：
#   1) Compose 部署：检测到 postgres 容器（tablewave-postgres，或品牌更名前的旧名
#      data-insights-postgres）在跑 → docker exec 内的 pg_dump；
#   2) 其余：用本机 pg_dump 备份 DATABASE_URL（来自环境变量或仓库根 .env）。
#
# 版本号来源：GET /health/detail 的 version 字段（R-100）；该端点不存在时回退
# app 容器镜像 tag，再回退 unknown —— R-100 合入后无需改动即自动用上完整判据。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_DIR="$REPO_ROOT/backups"
BASE_URL="http://localhost:${PORT:-23352}"

# 容器名在品牌更名（4dd37df「产品更名为 TableWave」）时由 `data-insights-*` 改成 `tablewave-*`，
# 但存量 compose 部署仍跑着旧名。硬编码任何一个都会在另一种部署上必定找不到容器
# （写新名则旧部署失效，留旧名则新装失效），所以按候选列表探测，两者都认。
container_running() {
  command -v docker >/dev/null 2>&1 || return 1
  docker ps --format '{{.Names}}' 2>/dev/null | grep -qx "$1"
}
container_exists() {
  command -v docker >/dev/null 2>&1 || return 1
  docker inspect "$1" >/dev/null 2>&1
}
# pick_container <running|exists> <候选名…>：命中即返回；都不命中回落首个候选（新名），
# 让后续报错信息指向「本该有的那个」而不是随便一个旧名。
pick_container() {
  local mode="$1" c; shift
  for c in "$@"; do
    if [ "$mode" = running ]; then
      container_running "$c" && { echo "$c"; return 0; }
    else
      container_exists "$c" && { echo "$c"; return 0; }
    fi
  done
  echo "$1"
}

APP_CONTAINER="$(pick_container exists tablewave-app data-insights-app)"
COMPOSE_CONTAINER="$(pick_container running tablewave-postgres data-insights-postgres)"

usage() { sed -n '2,7p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0; }

while [ $# -gt 0 ]; do
  case "$1" in
    --output-dir) OUTPUT_DIR="$2"; shift 2 ;;
    -h|--help) usage ;;
    *) echo "未知参数: $1（见 --help）" >&2; exit 2 ;;
  esac
done

# 读取仓库根 .env 中的 DATABASE_URL（不覆盖已有环境变量，与后端 LoadDotEnv 同优先级）。
load_env_database_url() {
  [ -n "${DATABASE_URL:-}" ] && return 0
  local env_file="$REPO_ROOT/.env"
  [ -f "$env_file" ] || return 0
  DATABASE_URL="$(sed -n 's/^DATABASE_URL=//p' "$env_file" | head -1 | tr -d '"' | tr -d "'")"
}

# 版本号：/health/detail 的 version 字段 → app 容器镜像 tag → unknown。
detect_version() {
  local v=""
  if command -v curl >/dev/null 2>&1; then
    v="$(curl -fsS --max-time 3 "$BASE_URL/health/detail" 2>/dev/null \
      | sed -n 's/.*"version":"\([^"]*\)".*/\1/p' | head -1)" || v=""
  fi
  if [ -z "$v" ] && docker inspect "$APP_CONTAINER" >/dev/null 2>&1; then
    v="$(docker inspect --format '{{.Config.Image}}' "$APP_CONTAINER" | sed 's/^.*://')"
  fi
  echo "${v:-unknown}"
}

# sha256：跨 macOS / Linux。
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}

mkdir -p "$OUTPUT_DIR"
VERSION="$(detect_version)"
TS="$(date +%Y%m%d-%H%M%S)"
# 版本串可能含 / 等路径字符（本地构建镜像名），压成文件名安全形态。
SAFE_VERSION="$(printf '%s' "$VERSION" | tr -c 'A-Za-z0-9._-' '-')"
OUT_FILE="$OUTPUT_DIR/di-backup-${SAFE_VERSION}-${TS}.dump"

if container_running "$COMPOSE_CONTAINER"; then
  PG_USER="$(docker exec "$COMPOSE_CONTAINER" printenv POSTGRES_USER 2>/dev/null || echo insights)"
  PG_DB="$(docker exec "$COMPOSE_CONTAINER" printenv POSTGRES_DB 2>/dev/null || echo insights)"
  echo "备份来源：容器 ${COMPOSE_CONTAINER}（库 ${PG_DB}）"
  docker exec "$COMPOSE_CONTAINER" pg_dump -Fc -U "$PG_USER" "$PG_DB" > "$OUT_FILE"
else
  load_env_database_url
  if [ -z "${DATABASE_URL:-}" ]; then
    echo "错误：未检测到容器 ${COMPOSE_CONTAINER}，且 DATABASE_URL 未设置（环境变量或 ${REPO_ROOT}/.env）" >&2
    exit 1
  fi
  command -v pg_dump >/dev/null 2>&1 || { echo "错误：本机没有 pg_dump" >&2; exit 1; }
  echo "备份来源：DATABASE_URL（本机 pg_dump）"
  pg_dump -Fc "$DATABASE_URL" > "$OUT_FILE"
fi

[ -s "$OUT_FILE" ] || { echo "错误：备份文件为空，pg_dump 可能失败" >&2; exit 1; }

echo "备份完成：$OUT_FILE"
echo "校验和：  $(sha256_of "$OUT_FILE")  ($(basename "$OUT_FILE"))"
