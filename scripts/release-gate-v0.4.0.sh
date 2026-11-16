#!/usr/bin/env bash
# V0.4.0 发版闸（静态半程）：逐项核对 #230 的出口判据里**能从仓库判定**的那一半。
#
# 为什么需要它：出口判据写在 issue 正文里，发版时靠人逐条回忆等于没写。
#
# 两种结果，含义严格限定：
#   MISSING  该项的独有字面量在仓库里查不到 → 对应 PR 确定还没合入。
#   PRESENT  字面量命中 —— **必要不充分**：只说明这条能力大概落进来了，
#            出口判据里「实机跑出来的那半」（并发只成功一个、断库 60s 内判失败、
#            未登录被弹回登录页……）本脚本代不了劳，必须按
#            docs/deployment/release-readiness.md 的实机清单逐条跑过才能放行。
#
# 判据字面量的选取纪律：每项钉的是**该项独有、写本脚本时对 master 逐项反证过不存在**的串。
# 反过来说，若某条 PR 用词与这里不同，会误报 MISSING —— 那就改本脚本的字面量并在文档
# 对应行注一句，别把 MISSING 当成「对方没做」。
#
# 用法：
#   bash scripts/release-gate-v0.4.0.sh                # 完整判据（发版前跑，须 16 项 PRESENT）
#   bash scripts/release-gate-v0.4.0.sh --self-check   # 只验闸自身健康（CI 每次 push 跑）
# 退出码：0 = 16 项静态痕迹齐备（仍不等于可发版）；1 = 仍有缺口。
#         --self-check 下：0 = 闸健康（不因某项 MISSING 判红，发版期间本就有未合入项）；1 = 闸坏了。
set -uo pipefail

self_check_only=0
[[ "${1:-}" == "--self-check" ]] && self_check_only=1

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root" || exit 1

# 扫描集 = 显式文件清单，**自我排除**本闸与它的判据文档。
# 为什么必须排除：这两份文件把全部 16 条判据的字面量原样写在里面（为了让人看懂要钉什么），
# 纳进扫描集会让每一项都命中 —— 这个坑在本次开发里真实踩过两次（第一次是 scripts/ 目录，
# 第二次是 docs/deployment/release-readiness.md），故改成「先列文件、再排除」而不是「列目录」。
# go.sum / bun.lock 之类的锁文件也不进来：数字串成百上千，`20201` 这类判据必然误命中。
scan_files() {
  find backend/cmd backend/internal backend/migrations api frontend/src docs \
    -type f \( -name '*.go' -o -name '*.ts' -o -name '*.tsx' -o -name '*.sql' -o -name '*.yaml' -o -name '*.md' \) \
    2>/dev/null | grep -Ev 'release-gate-v0\.4\.0\.sh|release-readiness\.md'
}
SCAN="$(scan_files)"

present=0 missing=0 rows=0 FAILED=""

# hits <正则>：输出命中的文件清单（可能为空）。**判定只看输出是否为空，不看退出码。**
#
# 为什么不能写成 `printf … | xargs grep -lE "$pat" | grep -q .`：
# 本脚本开着 `set -o pipefail`，而 `grep -q` 命中首行后立即退出，上游 xargs 紧接着写
# 后续行会收到 SIGPIPE（退出码 141）；pipefail 取管道最右侧的非零状态 → 整个管道判非负。
# 结果是**命中文件越多越容易判成「未命中」**：单文件命中时恰好赶在 SIGPIPE 之前写完，
# 看起来正常；多文件命中就翻成 MISSING。2026-11-15 实测到 10 项假红 —— 同一份文件清单
# 查 AUTH_ENFORCE，去掉 pipefail 时命中 16 个文件，带 pipefail 时判 no-hit。
# 教训与本仓「判退出码绝不接管道」同源：要么取输出、要么取 ${PIPESTATUS[0]}，别读管道末段的码。
hits() {
  local pat="$1"
  [[ -n "$SCAN" ]] || return 0
  printf '%s\n' "$SCAN" | xargs grep -lE "$pat" 2>/dev/null || true
}

# static <issue> <判据说明> <正则> [...]：任一正则命中 → PRESENT，全不命中 → MISSING。
static() {
  local issue="$1" desc="$2"; shift 2
  local pat
  rows=$((rows + 1))
  for pat in "$@"; do
    if [[ -n "$(hits "$pat")" ]]; then
      printf 'PRESENT  #%-4s %s\n' "$issue" "$desc"
      present=$((present + 1)); return 0
    fi
  done
  printf 'MISSING  #%-4s %s\n' "$issue" "$desc"
  missing=$((missing + 1)); FAILED="$FAILED $issue"
}

# absent <issue> <判据说明> <必须查不到的正则>：命中即 MISSING（用于「清除旧口径」类判据）。
absent() {
  local issue="$1" desc="$2" pat="$3"
  rows=$((rows + 1))
  if [[ -n "$(hits "$pat")" ]]; then
    printf 'MISSING  #%-4s %s\n' "$issue" "$desc"
    missing=$((missing + 1)); FAILED="$FAILED $issue"
  else
    printf 'PRESENT  #%-4s %s\n' "$issue" "$desc"
    present=$((present + 1))
  fi
}

# exists <issue> <判据说明> <文件> [...]：全部存在才 PRESENT（用于「脚本类交付物」判据）。
exists() {
  local issue="$1" desc="$2"; shift 2
  local f
  rows=$((rows + 1))
  for f in "$@"; do
    [[ -f "$root/$f" ]] || {
      printf 'MISSING  #%-4s %s（缺 %s）\n' "$issue" "$desc" "$f"
      missing=$((missing + 1)); FAILED="$FAILED $issue"; return
    }
  done
  printf 'PRESENT  #%-4s %s\n' "$issue" "$desc"
  present=$((present + 1))
}

echo "== V0.4.0 发版闸 · 静态半程（实机清单见 docs/deployment/release-readiness.md）=="
echo "   仓库：$root"
echo

# —— 负控自检（不计入 16 项判据）——
# 拿一个「迁移文件里必然存在」的字面量跑一遍命中链，它必须命中。不命中就说明扫描集或
# hits() 这条链本身坏了 —— 本仓真实发生过：pipefail + `grep -q` 让 10 项判成 MISSING，
# 而读输出的人只会去怀疑「对应 PR 还没合」，白找一轮。
# 闸坏了比闸红了更危险：红了会有人去补活，坏了会让人开始无视闸。
if [[ -z "$(hits 'CREATE TABLE')" ]]; then
  echo "✖ 闸自身失效：负控字面量 'CREATE TABLE' 在扫描集里查不到（扫描集或命中链坏了）" >&2
  echo "  → 此时任何 MISSING 都不可信，先修闸，别照结论去找 PR。" >&2
  exit 1
fi

# —— A 组 · 身份与门禁 ——
static 202 '最小用户管理：建号/重置/禁用端点 + disabled_at 语义' 'CreateUser|DisableUser|ResetPassword' 'disabled_at'
static 203 'R-87 认证端点限流' 'AUTH_RATE_LIMIT_PER_MINUTE' 'Retry-After'
static 216 '首管 bootstrap：一次性凭据 + 状态探测端点' 'bootstrap_token|BOOTSTRAP_TOKEN' 'bootstrap-status'
static 217 'R-92 渐进开关：三态 env + shadow 日志 + 探测端点' 'AUTH_ENFORCE' 'auth_would_deny' 'auth/config'
static 218 'R-93 前端身份壳：/login 路由 + 会话持久化' 'Route path="/login"' 'sessionStorage'
static 219 'R-94 compose 只绑环回 + HOST_BIND' 'HOST_BIND'
# ⚠️ 判据必须用**实现里真实存在的标识符**，不要用同义词猜。本行原为
# 'created_by|ownerName|creatorName'，而 R-95 落地用的是 Go/TS 导出名 OwnerName
# 与 JSON 名 owner_name —— 三个猜测串全仓零命中，那段时间的 PRESENT 实际是扫到
# docs/deployment/release-readiness.md 自己（自指假绿），已修正。
# 区分能力反证过：6c5ddba^（实现前）零命中，6c5ddba（实现后）5+ 文件命中。
static 220 'R-95 创建人可见（归属写入的可见出口）' 'owner_name|OwnerName'
static 221 'R-96 失败日志带业务码' 'biz_code'
static 222 'R-97 错误编号与可复制报障' 'error_id|errorId'
static 223 'R-98 非 panic 错误集中上报' 'CaptureException' 'BeforeSend'
static 224 'R-99 稳定码 + 越权独立码' 'hint_key|hintKey' 'CodeForbidden'

# —— C 组 · 首启与运维 ——
static 225 'R-100 版本与运行状态可见' 'ldflags' '"/version"|/version"' 'health/ready|healthReady'
static 226 'R-101 首启向导与空状态引导' '首启向导|SetupWizard'
absent 228 'R-103 文档清除「无登录、无鉴权」旧口径' '无登录、无鉴权'
exists 227 'R-102 备份 / 升级脚本作为文件交付' scripts/backup.sh scripts/upgrade.sh
static 229 'R-104 诊断包导出（脱敏仍需实机 grep 导出物）' 'diagnostic|诊断包'

echo
echo "== 静态判据 ${rows} 项：PRESENT=${present} MISSING=${missing} =="
if [[ $rows -ne 16 ]]; then
  echo "✖ 检查项数量与文档登记的 16 条不符（实到 ${rows}）—— 闸本身被改坏了，按失败处理" >&2
  exit 1
fi

# --self-check：CI 每次 push 只验闸健康（负控命中 + 项数），不因 MISSING 判红。
# 为什么不在 CI 里要求 16 项全 PRESENT：发版窗口之间有 PR 在飞，全绿会变成常态红；
# 而闸腐化（扫描集失效、命中链坏、判据被删）才是没人会主动发现的那类问题。
if [[ $self_check_only -eq 1 ]]; then
  echo "✔ 闸自检通过：负控命中正常，判据项数 = ${rows}（PRESENT=${present} MISSING=${missing} 仅供参考，本次不判红）"
  exit 0
fi

# 迁移水位只报不判：它决定「换镜像能否直接升级」，是 Release 正文素材，不是完成度判据。
water="$(ls backend/migrations/*.sql 2>/dev/null | tail -1 | sed -E 's#.*/([0-9]+)_.*#\1#')"
echo "   迁移水位：${water:-未知}（发版前按实际改 CHANGELOG 的「升级须知 / 无需数据库迁移」一节）"

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  {
    echo "### V0.4.0 发版闸（静态半程）"
    echo "- PRESENT=${present} MISSING=${missing}"
    echo "- 未合入条目：${FAILED:-（无，但仍需跑实机清单）}"
  } >>"$GITHUB_STEP_SUMMARY"
fi

if [[ $missing -gt 0 ]]; then
  echo "✖ v0.4.0 不可发：以下条目在 master 上还没有痕迹 →${FAILED}" >&2
  echo "  → 逐条判据与 PR 对应关系见 docs/deployment/release-readiness.md" >&2
  exit 1
fi
echo "⚠ 静态痕迹齐备 ≠ 可发版：PRESENT 只说明字面量命中，16 条实机判据必须逐条跑过。"
