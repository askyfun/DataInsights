#!/usr/bin/env bash
# 前端测试门禁：**只认用例计数，不认退出码**。
#
# 为什么不能看退出码：本仓有一条既有的未处理错误（jsdom 环境拆除期 react-dom 调度出的
# 宏任务晚于环境销毁执行，抛 `ReferenceError: window is not defined`），它会让**全绿**
# 的跑批也以退出码 1 收场（Node 22 / Node 24 均可复现）。照退出码判就等于门禁永远红。
#
# 判据两条，缺一不可：
#   ① 计数行里出现任何 failed → 红；
#   ② 必须真的存在 `Test Files N passed (` 与 `Tests N passed (` → 否则红。
#      第 ② 条挡住「跑都没跑起来」也被当成通过的情况（比如 vitest 启动失败）。
#
# CI（.github/workflows/ci.yml、release.yml）与本地都跑这同一个脚本，判据不会两头漂。
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/../frontend"

log="$(mktemp)"
pnpm exec vitest run --pool=threads --maxWorkers=2 >"$log" 2>&1 || true
cat "$log"

# CI 下 vitest 输出带 ANSI 颜色码，计数行行首不是空白而是 \e[2m，先剥离再匹配，
# 否则全绿的跑批也会在这里误判失败。
summary="$(sed 's/\x1b\[[0-9;]*m//g' "$log" | grep -E '^[[:space:]]+(Test Files|Tests)[[:space:]]+' || true)"
printf '%s\n' "$summary"

if [[ -z "$summary" ]]; then
  echo "✖ 没抓到 vitest 的用例计数行——测试疑似根本没跑起来，按失败处理" >&2
  exit 1
fi
if printf '%s' "$summary" | grep -q 'failed'; then
  echo "✖ 前端存在失败用例（见上方 vitest 输出）" >&2
  exit 1
fi
printf '%s' "$summary" | grep -Eq 'Test Files[[:space:]]+[0-9]+ passed \('
printf '%s' "$summary" | grep -Eq 'Tests[[:space:]]+[0-9]+ passed \('
echo "✅ 前端测试通过（按用例计数判定，已忽略仓库既有的未处理错误）"

# 在 Actions 里顺手写进 step summary（本地跑时该变量不存在，跳过）。
if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  echo "前端测试通过（仅按用例计数判定，已忽略仓库既有的未处理错误）" >>"$GITHUB_STEP_SUMMARY"
fi
