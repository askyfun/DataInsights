// pre-commit 泄露扫描：拦截「新增行」里的内网 IP / 个人家目录 / 个人工具链路径。
// 只检查本次暂存的 diff（新增行），不追溯已提交内容，故不会因历史遗留而误挡日常提交。
// 注：insight_dev / dataray_data 是仓库里合法的默认名（.env.example 等），刻意不列入。
// 需要追加模式就往 PATTERNS 里加；确要放行单次提交用 `git commit --no-verify`。
import { execSync } from 'node:child_process';

// 本脚本自身与钩子文件会包含这些字面量（作为检测规则），扫描时跳过，避免自触发。
const SELF = new Set(['scripts/check-secrets.mjs', '.husky/pre-commit']);

const PATTERNS = [
  { re: /192\.168\.\d{1,3}\.\d{1,3}/, label: '内网 IP (192.168.x.x)' },
  { re: /\/Users\/asky\b/, label: '个人家目录绝对路径 (/Users/asky)' },
  { re: /workbuddy/, label: '个人工具链路径 (workbuddy)' },
];

// 默认扫本次暂存的内容（pre-commit 钩子用）。CI 里没有暂存区，必须显式给提交范围：
// `node scripts/check-secrets.mjs --diff origin/master...HEAD`。这样即使用了
// `git commit --no-verify` 绕过本地钩子，泄漏照样会在 PR 门禁上被挡下来。
const diffFlag = process.argv.indexOf('--diff');
const diffCmd =
  diffFlag >= 0
    ? `git diff ${process.argv[diffFlag + 1]} -U0 --no-color`
    : 'git diff --cached -U0 --no-color';
const mode = diffFlag >= 0 ? 'CI diff 扫描' : 'pre-commit';

let diff = '';
try {
  diff = execSync(diffCmd, { encoding: 'utf8' });
} catch {
  process.exit(0);
}

const offenders = [];
let file = '';
for (const line of diff.split('\n')) {
  const hdr = line.match(/^\+\+\+ b\/(.*)$/);
  if (hdr) {
    file = hdr[1];
    continue;
  }
  if (SELF.has(file)) continue;
  if (line.startsWith('+') && !line.startsWith('+++')) {
    for (const { re, label } of PATTERNS) {
      if (re.test(line)) offenders.push(`  ${label}\n    ${file}\n    ${line.slice(1).trim().slice(0, 120)}`);
    }
  }
}

if (offenders.length) {
  console.error(`\n✖ ${mode}：检测到疑似敏感信息，已阻止：\n`);
  console.error([...new Set(offenders)].join('\n'));
  console.error('\n请改写成占位符（如 <dev-host>、/Users/dev）后再提交。\n');
  process.exit(1);
}
process.exit(0);
