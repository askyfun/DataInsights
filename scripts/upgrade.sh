#!/usr/bin/env bash
# upgrade.sh —— Compose 部署的升级 runbook：备份 → 迁移前置检查 → 换版本 → ready 轮询 → 失败打印回滚命令。
#
# 用法：
#   scripts/upgrade.sh [--dry-run] [--yes] [<TAG>]
#
#   <TAG>       目标版本，如 v0.3.1（与 GitHub Releases 的 tag 一致）；缺省保持当前 tag 重拉
#   --dry-run   只做备份 + 打印迁移前置检查 + 真实调用一次 ready 判据，不换版本
#   --yes       跳过迁移前置检查的交互确认（无人值守用；确认责任在调用方）
#
# 失败判据是 ready 探测拿不到 200，而不是容器状态 —— 迁移失败时进程根本不监听端口。
# ready 端点：优先 /health/ready（R-100，探库）；不存在（404）时回退 /health（只证进程活着）。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
COMPOSE_FILE="$REPO_ROOT/docker-compose.allinone.yml"
APP_CONTAINER="data-insights-app"
COMPOSE_CONTAINER="data-insights-postgres"
BASE_URL="http://localhost:${PORT:-23352}"
READY_TIMEOUT_SECS=60

DRY_RUN=0
ASSUME_YES=0
TARGET_TAG="${TARGET_TAG:-}"

usage() { sed -n '2,10p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0; }

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=1; shift ;;
    --yes) ASSUME_YES=1; shift ;;
    -h|--help) usage ;;
    -*) echo "未知参数: $1（见 --help）" >&2; exit 2 ;;
    *) TARGET_TAG="$1"; shift ;;
  esac
done

die() { echo "错误：$1" >&2; exit 1; }

http_code() { curl -fsS -o /dev/null -w '%{http_code}' --max-time 3 "$1" 2>/dev/null || echo 000; }

# ready 判据：/health/ready 优先（探库），404 回退 /health；200 才算 ready。
probe_ready() {
  [ "$(http_code "$BASE_URL/health/ready")" = "200" ] && return 0
  [ "$(http_code "$BASE_URL/health")" = "200" ] && return 0
  return 1
}

# 记录当前版本用于回滚：容器镜像的 tag（kzzhr/datainsights:vX.Y.Z → vX.Y.Z）。
current_image_tag() {
  local image
  if ! image="$(docker inspect --format '{{.Config.Image}}' "$APP_CONTAINER" 2>/dev/null)"; then
    echo ""
    return 0
  fi
  echo "${image##*:}"
}

[ -f "$COMPOSE_FILE" ] || die "找不到 ${COMPOSE_FILE}（升级脚本面向 Compose 部署）"
command -v docker >/dev/null 2>&1 || die "docker 不可用"
command -v curl >/dev/null 2>&1 || die "curl 不可用"

# ---- 前置检查：升级动作只对在跑的部署做 ------------------------------------
OLD_TAG="$(current_image_tag)"
[ "$(http_code "$BASE_URL/health")" = "200" ] || die "当前部署不健康（$BASE_URL/health 非 200），先处理再升级"
[ "$OLD_TAG" = "latest" ] && echo "⚠️ 当前跑的是 latest，回滚命令无法精确钉回旧版本 —— 日常升级请始终用版本号 tag"

echo "==> 当前版本: $OLD_TAG"

# ---- 步骤 1：备份 ------------------------------------------------------------
echo "==> 步骤 1/4: 备份数据库"
BACKUP_OUTPUT="$(bash "$SCRIPT_DIR/backup.sh")"
echo "$BACKUP_OUTPUT"
BACKUP_FILE="$(echo "$BACKUP_OUTPUT" | sed -n 's/^备份完成：//p' | head -1)"
[ -n "$BACKUP_FILE" ] && [ -f "$BACKUP_FILE" ] || die "备份未产出文件，中止升级"

# ---- 步骤 2：迁移前置检查（顺序红线：dry-run 看报告 → -apply 写库 → 再放量） ----
echo "==> 步骤 2/4: 一次性数据迁移前置检查"
cat <<'EOF'
    「-migrate-column-ids」与「启动自动 schema 迁移」是两套机制，顺序颠倒会静默返回错数。
    正确顺序：dry-run 看报告 → 有待迁移项则 -apply 写库 → 再放量新版本。
    何时需要：CHANGELOG.md 中标注「需先跑一次性迁移」的版本；不确定就先跑 dry-run 看：

      docker exec data-insights-app ./server -migrate-column-ids           # 只扫描，出报告
      docker exec data-insights-app ./server -migrate-column-ids -apply    # 报告确认无误后写库
EOF
if [ "$DRY_RUN" -eq 0 ] && [ "$ASSUME_YES" -eq 0 ]; then
  if [ -t 0 ]; then
    printf "已确认 dry-run 报告无需迁移、或已完成 -apply？[y/N] "
    read -r answer
    case "$answer" in
      y|Y|yes|YES) ;;
      *) die "已中止。请先完成上面的迁移步骤再重跑 upgrade.sh" ;;
    esac
  else
    die "非交互环境无法确认迁移前置检查；确认无误后用 --yes 重跑"
  fi
fi

if [ "$DRY_RUN" -eq 1 ]; then
  echo "==> --dry-run：不换版本。真实调用一次 ready 判据以验证探测可用……"
  if probe_ready; then
    echo "==> ready 判据：OK（200）"
  else
    echo "==> ready 判据：FAIL（60s 内未拿到 200）" >&2
    exit 1
  fi
  echo "==> dry-run 完成：备份已产出（${BACKUP_FILE}），当前部署未做任何变更"
  exit 0
fi

# ---- 步骤 3：换版本 ----------------------------------------------------------
NEW_TAG="${TARGET_TAG:-$OLD_TAG}"
echo "==> 步骤 3/4: TAG=$NEW_TAG docker compose up -d"
TAG="$NEW_TAG" docker compose -f "$COMPOSE_FILE" up -d

# ---- 步骤 4：ready 轮询（判据：200，不看容器状态） ---------------------------
echo "==> 步骤 4/4: 轮询 ready（最长 ${READY_TIMEOUT_SECS}s）"
elapsed=0
until probe_ready; do
  if [ "$elapsed" -ge "$READY_TIMEOUT_SECS" ]; then
    echo "" >&2
    echo "❌ 升级失败：${READY_TIMEOUT_SECS}s 内 ready 未拿到 200（新进程可能没起来或迁移卡住）" >&2
    echo "" >&2
    echo "回滚命令（按顺序执行）：" >&2
    echo "  # 1) 用旧版本镜像把容器拉起来" >&2
    echo "  TAG=$OLD_TAG docker compose -f $COMPOSE_FILE up -d" >&2
    echo "  # 2) 库已被新版本迁移改坏时，恢复升级前的备份" >&2
    echo "  docker exec -i $COMPOSE_CONTAINER pg_restore --clean --if-exists -U insights -d insights < $BACKUP_FILE" >&2
    echo "" >&2
    echo "完整 runbook：docs/deployment/backup-restore.md" >&2
    exit 1
  fi
  sleep 2
  elapsed=$((elapsed + 2))
  echo "  等待 ready... ${elapsed}s"
done

echo "✅ 升级完成：${OLD_TAG} → ${NEW_TAG}（ready OK，备份留存于 ${BACKUP_FILE}）"
