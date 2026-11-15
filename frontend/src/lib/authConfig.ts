import { useSyncExternalStore } from 'react';

// 身份壳（R-93 / #218）从 /api/auth/config 探测到的鉴权配置。
// null = 尚未探测完成（用于区分「加载中」与「探测完成但字段为空」）。
export interface AuthRuntimeConfig {
  enforce_mode: string; // off | shadow | on
  needs_bootstrap: boolean;
}

let config: AuthRuntimeConfig | null = null;
const listeners = new Set<() => void>();

export function setAuthConfig(next: AuthRuntimeConfig | null): void {
  config = next;
  for (const l of listeners) l();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function getSnapshot(): AuthRuntimeConfig | null {
  return config;
}

export function useAuthConfig(): AuthRuntimeConfig | null {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}
