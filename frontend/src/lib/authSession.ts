import { useSyncExternalStore } from 'react';
import type { ApiUser } from '../api';

// 当前会话用户的极小外部 store：登录 / 退出后 App 的品牌栏与菜单要立刻重算，
// 而会话 token 只在内存里（刷新即失），所以这里也不做任何持久化。
let currentUser: ApiUser | null = null;
const listeners = new Set<() => void>();

export function setSessionUser(user: ApiUser | null): void {
  currentUser = user;
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function getSnapshot(): ApiUser | null {
  return currentUser;
}

export function useSessionUser(): ApiUser | null {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}
