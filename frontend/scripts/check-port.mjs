import { execSync } from 'node:child_process';

const port = 23351;
let out;
try {
  out = execSync(`lsof -nP -iTCP:${port} -sTCP:LISTEN`, { encoding: 'utf8' });
} catch {
  process.exit(0); // 无监听进程，放行
}

const lines = out.trim().split('\n').slice(1);
const pids = [...new Set(lines.map((l) => l.trim().split(/\s+/)[1]))];
console.error(`\n错误：端口 ${port} 已被占用，无法启动前端开发服务器。`);
for (const pid of pids) {
  let cmd = '';
  try {
    cmd = execSync(`ps -p ${pid} -o command=`, { encoding: 'utf8' }).trim();
  } catch {
    cmd = '(进程已退出)';
  }
  console.error(`  PID ${pid}: ${cmd}`);
}
console.error(`如需释放该端口：kill ${pids.join(' ')}\n`);
process.exit(1);
