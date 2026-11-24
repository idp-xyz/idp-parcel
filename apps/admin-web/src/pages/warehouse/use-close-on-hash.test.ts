import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { subscribeCloseOnHashChange } from './use-close-on-hash';

test('hashchange 触发关闭，解除订阅后不再触发', () => {
  const listeners = new Map<string, Set<() => void>>();
  const fakeWindow = {
    addEventListener(type: string, handler: () => void) {
      const set = listeners.get(type) ?? new Set();
      set.add(handler);
      listeners.set(type, set);
    },
    removeEventListener(type: string, handler: () => void) {
      listeners.get(type)?.delete(handler);
    },
  };
  const original = globalThis.window;
  // 测的是订阅契约，不依赖真实浏览器 window。
  (globalThis as { window: typeof fakeWindow }).window = fakeWindow;

  let closed = 0;
  const unsub = subscribeCloseOnHashChange(() => {
    closed += 1;
  });
  equal(listeners.get('hashchange')?.size, 1);

  for (const handler of listeners.get('hashchange') ?? []) handler();
  equal(closed, 1);

  unsub();
  equal(listeners.get('hashchange')?.size, 0);
  for (const handler of listeners.get('hashchange') ?? []) handler();
  equal(closed, 1);

  (globalThis as { window: typeof original }).window = original;
});
