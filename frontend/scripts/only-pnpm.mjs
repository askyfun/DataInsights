/**
 * 包管理器守卫：禁止用 npm / yarn / bun 安装依赖。
 *
 * 由 package.json 的 `preinstall` 钩子触发。npm 系工具在安装前都会注入
 * `npm_config_user_agent`（形如 `npm/10.9.7 node/v22.22.2 darwin arm64`），
 * 据此判断是谁在调用。非 pnpm 直接以退出码 1 终止安装。
 *
 * 白名单只放行 pnpm —— 本项目用全局内容寻址 store + 硬链接，
 * 别的包管理器会在 node_modules 里复制实体文件，体积成倍膨胀。
 *
 * 手动执行（`node scripts/only-pnpm.mjs`）不带 UA，会被视为未知并放行。
 */
import { existsSync, rmSync } from 'node:fs';
import { resolve } from 'node:path';

const ua = process.env.npm_config_user_agent ?? '';
const execPath = process.env.npm_execpath ?? '';

if (!ua) {
  // 未经过任何包管理器（人手直接 node 执行），不拦截。
  process.exit(0);
}

const pm = ua.split('/')[0].trim();

if (pm === 'pnpm') {
  process.exit(0);
}

const hint = `当前调用方：${pm}${execPath ? ` (${execPath})` : ''}`;

// npm / yarn 在 preinstall 之前就已经写下了自己的 lockfile。它们是
// pnpm-lock.yaml 之外的第二事实源，留着会被误提交并造成版本漂移，就地清除。
const strayLocks = ['package-lock.json', 'npm-shrinkwrap.json', 'yarn.lock'].filter((f) =>
  existsSync(resolve(process.cwd(), f))
);
for (const f of strayLocks) {
  rmSync(resolve(process.cwd(), f), { force: true });
}

console.error(`
────────────────────────────────────────────────────────────
  包管理器守卫：本项目只允许使用 pnpm
────────────────────────────────────────────────────────────
  ${hint}

  检测到非 pnpm 的安装命令，已中止。
${strayLocks.map((f) => `  已清除误生成的 ${f}`).join('\n')}${strayLocks.length ? '\n' : ''}
  正确用法：
      pnpm install          # 在项目根目录 / frontend 目录

  为什么不能换：
    · pnpm 用全局内容寻址 store + 硬链接，同一份依赖在磁盘上只占一份；
    · pnpm-lock.yaml 是本项目唯一的依赖事实源，换包管理器会产生
      package-lock.json / yarn.lock 等第二事实源，导致版本漂移；
    · 卸载残留的旧大版本（antd 5 / echarts 5 / typescript 5 等）不会
      被其他包管理器清理，node_modules 会越滚越大。

  如果你确实需要临时绕过（不推荐）：
      pnpm install --ignore-scripts
────────────────────────────────────────────────────────────
`);

process.exit(1);
