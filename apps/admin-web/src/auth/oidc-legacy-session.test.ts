import { test } from 'node:test';
import { equal } from 'node:assert/strict';

// 票 operator-channel/07「令牌只在内存，不落本地存储」：上一版在 sessionStorage 的 parcel.oidc.session 下存整份会话，
// 含 refresh_token 原文；升级后刷新、没再点登录的标签页里它也不能留下。要证的是「模块一加载就清」，存储得在加载
// 本模块之前摆好，与 oidc.test.ts 顶上的静态 import 不相容，所以单独成文件：node --test 一个文件一个进程，本模块
// 在这里只随下面那次 import 加载一次。

test('升级前留下的整份会话，模块一加载就清掉，不等开始登录；登录中间态不动', async () => {
  const entries = new Map<string, string>([
    ['parcel.oidc.session', JSON.stringify({ accessToken: 'SYN-LEGACY-TOKEN', refreshToken: 'SYN-LEGACY-REFRESH' })],
    ['parcel.oidc.pending', 'SYN-PENDING-LOGIN'],
  ]);
  (globalThis as unknown as Record<string, unknown>).sessionStorage = {
    getItem: (key: string) => entries.get(key) ?? null,
    setItem: (key: string, value: string) => void entries.set(key, value),
    removeItem: (key: string) => void entries.delete(key),
  };

  await import('./oidc');

  equal(entries.has('parcel.oidc.session'), false);
  // 回调页加载本模块时，中间态正等着 completeLogin 来读；这一下清掉它，从签发方回来就登不进。
  equal(entries.get('parcel.oidc.pending'), 'SYN-PENDING-LOGIN');
});
