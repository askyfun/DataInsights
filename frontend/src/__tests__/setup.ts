import '@testing-library/jest-dom/vitest';
import { message as antdMessage, Modal as antdModal } from 'antd';
import { afterEach, vi } from 'vitest';

// React 19 的并发渲染经 scheduler 以 setImmediate 排程。组件测试若带着未完成的
// 渲染收尾，该回调会在 vitest 拆除本文件 jsdom 环境后触发（window 已被回收），
// 以 "window is not defined" unhandled error 打挂整个 run（CI 上出现过）。这里在
// 每个用例结束时、环境仍存活时排空一轮排程任务。
afterEach(async () => {
  await new Promise((resolve) => setTimeout(resolve, 0));
});

// antd 静态 message / Modal 会在 @testing-library 清理树之外的独立 React root 里
// 渲染，其 rc-motion 真实定时器在用例收尾后仍排程渲染任务，是上述伪影的主要源头。
// 测试不依赖这些树外 UI 副作用，统一短路（兜底过滤见 vite.config.ts 的
// onUnhandledError）。
const stubMessageType = (): ReturnType<(typeof antdMessage)['success']> => {
  const call = (): void => {};
  return Object.assign(call, {
    // biome-ignore lint/suspicious/noThenProperty: 需满足 antd MessageType 的 PromiseLike 签名
    then: <TResult1 = boolean, TResult2 = never>(
      onFulfilled?: ((value: boolean) => TResult1 | PromiseLike<TResult1>) | undefined | null,
      onRejected?: ((reason: unknown) => TResult2 | PromiseLike<TResult2>) | undefined | null
    ): PromiseLike<TResult1 | TResult2> => Promise.resolve(true).then(onFulfilled, onRejected),
  });
};
for (const method of ['info', 'success', 'error', 'warning', 'loading'] as const) {
  vi.spyOn(antdMessage, method).mockImplementation(stubMessageType);
}
vi.spyOn(antdMessage, 'open').mockImplementation(stubMessageType);
for (const method of ['info', 'success', 'error', 'warning', 'warn', 'confirm'] as const) {
  vi.spyOn(antdModal, method).mockImplementation(() => ({ destroy: () => {}, update: () => {} }));
}

// jsdom provides no ResizeObserver; antd v6's rc-resize-observer requires it.
if (typeof globalThis.ResizeObserver === 'undefined') {
  class ResizeObserverStub {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver;
}

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  }),
});
