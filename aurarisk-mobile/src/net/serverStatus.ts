import { useSyncExternalStore } from 'react';

// "Waking" means at least one foreground request is slow or being retried, which on the free
// demo API almost always means the server is starting up after a quiet period.
let slowRequests = 0;
const listeners = new Set<() => void>();

export const isServerWaking = (): boolean => slowRequests > 0;

const update = (delta: number) => {
  const before = isServerWaking();
  slowRequests = Math.max(0, slowRequests + delta);
  if (before !== isServerWaking()) listeners.forEach((listener) => listener());
};

/** Marks one request as slow. Call the returned function exactly once when it settles. */
export const markRequestSlow = (): (() => void) => {
  update(1);
  let done = false;
  return () => {
    if (done) return;
    done = true;
    update(-1);
  };
};

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

export const useServerWaking = (): boolean => useSyncExternalStore(subscribe, isServerWaking, isServerWaking);
