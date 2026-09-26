/// <reference types="vitest" />
import path from 'node:path';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [react()],
  // 环境变量统一放仓库根目录的 .env（前后端共用一份），而不是 frontend/ 下。
  // Vite 只会把 VITE_ 前缀的变量暴露给浏览器包，其余裸名变量（DATABASE_URL、SECURITY_KEY 等）不会泄漏。
  // 单容器镜像里 frontend/ 被 COPY 到 /app，此时 .. 解析为 /，没有 .env 文件，
  // Vite 读不到即为空，不报错；真正生效的是 Dockerfile 里的 ENV（进程环境变量优先）。
  envDir: path.resolve(__dirname, '..'),
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    host: '0.0.0.0',
    port: 23351,
    strictPort: true,
    open: false,
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: './src/__tests__/setup.ts',
    include: ['src/**/*.{test,spec}.{js,mjs,cjs,ts,mts,cts,jsx,tsx}'],
    onUnhandledError(error) {
      // React 19 的 scheduler 用 setImmediate 排程渲染回调；组件测试收尾后仍挂着的
      // 渲染任务（antd 静态 message/Modal 的独立 root、rc-motion 定时器等）会在 vitest
      // 拆除该文件 jsdom 环境后触发，访问已被回收的 window 抛出 ReferenceError。
      // 这是环境拆除的伪影而非被测代码缺陷（用例本身全部通过），且源头分散在所有
      // 使用 antd 静态 API 的页面，无法逐个清理——这里只忽略这一种已知伪影。
      // 注意：错误跨 worker 序列化后 instanceof 失效，只能按 name/stack 判定。
      if (
        error.name === 'ReferenceError' &&
        error.message === 'window is not defined' &&
        /react-dom|scheduler/.test(error.stack ?? '')
      ) {
        return false;
      }
      return undefined;
    },
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
    },
  },
});
