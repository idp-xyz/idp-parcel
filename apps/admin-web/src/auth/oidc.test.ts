import { afterEach, test } from 'node:test';
import { equal, ok } from 'node:assert/strict';
import { accessTokenForRequest, beginLogin, completeLogin, currentLocation, ensureSession, logout } from './oidc';

// 票 operator-channel/07「令牌只在内存，不落本地存储」。走的是真的 PKCE 回调路径：beginLogin 写中间态并跳签发方，
// 回调带 code 与 state 回来，completeLogin 换令牌；签发方只在 fetch 这一层替身。判据是存储里找不到任何令牌原文——
// 登录、续期之后都是。completeLogin 成功的那个结果按设计在模块里共享（防同一个 code 被交换两次），所以整条登录
// 只走一遍，续期与退出在同一条用例里接着走。

class MemoryStorage {
  private readonly entries = new Map<string, string>();
  get length(): number {
    return this.entries.size;
  }
  key(index: number): string | null {
    return [...this.entries.keys()][index] ?? null;
  }
  getItem(key: string): string | null {
    return this.entries.get(key) ?? null;
  }
  setItem(key: string, value: string): void {
    this.entries.set(key, String(value));
  }
  removeItem(key: string): void {
    this.entries.delete(key);
  }
  clear(): void {
    this.entries.clear();
  }
  text(): string {
    return [...this.entries].map(([key, value]) => `${key}=${value}`).join('\n');
  }
}

interface FakeLocation {
  origin: string;
  pathname: string;
  search: string;
  hash: string;
  assigned: string[];
  assign(url: string): void;
}

const globals = globalThis as unknown as Record<string, unknown>;
const realFetch = globalThis.fetch;

function installBrowser(): { location: FakeLocation; session: MemoryStorage; local: MemoryStorage } {
  const location: FakeLocation = {
    origin: 'http://localhost:5173',
    pathname: '/',
    search: '',
    hash: '#/price-card-catalog',
    assigned: [],
    assign(url: string) {
      this.assigned.push(url);
    },
  };
  const session = new MemoryStorage();
  const local = new MemoryStorage();
  globals.window = { location, setTimeout: globalThis.setTimeout, clearTimeout: globalThis.clearTimeout };
  globals.sessionStorage = session;
  globals.localStorage = local;
  return { location, session, local };
}

function base64Url(text: string): string {
  return btoa(text).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function idTokenWith(claims: Record<string, unknown>): string {
  return `${base64Url(JSON.stringify({ alg: 'RS256', typ: 'JWT' }))}.${base64Url(JSON.stringify(claims))}.SYN-SIGNATURE`;
}

function stubTokenEndpoint(respond: (form: URLSearchParams) => Record<string, unknown>): URLSearchParams[] {
  const forms: URLSearchParams[] = [];
  globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
    const form = new URLSearchParams(String(init?.body ?? ''));
    forms.push(form);
    return new Response(JSON.stringify(respond(form)), { status: 200 });
  }) as typeof fetch;
  return forms;
}

afterEach(() => {
  globalThis.fetch = realFetch;
  delete globals.window;
  delete globals.sessionStorage;
  delete globals.localStorage;
});

test('登回来的位置带 hash：页面位置在 hash 里', () => {
  installBrowser();
  equal(currentLocation(), '/#/price-card-catalog');
});

test('开始登录时清掉上一版存在 sessionStorage 里的整份会话', async () => {
  const { session } = installBrowser();
  session.setItem('parcel.oidc.session', JSON.stringify({ accessToken: 'SYN-LEGACY-TOKEN' }));

  await beginLogin(currentLocation());

  equal(session.getItem('parcel.oidc.session'), null);
  // 登录中间态照旧在：它要熬过跳到签发方再跳回来的整页导航。
  ok(session.getItem('parcel.oidc.pending'));
});

test('登录与续期换来的令牌只在内存，存储里一枚都找不到；退出即无令牌', async () => {
  const { location, session, local } = installBrowser();
  await beginLogin(currentLocation());
  const authorize = new URL(location.assigned[location.assigned.length - 1]);
  const state = authorize.searchParams.get('state') ?? '';
  const nonce = authorize.searchParams.get('nonce') ?? '';
  equal(authorize.searchParams.get('code_challenge_method'), 'S256');

  location.pathname = '/auth/callback';
  location.search = `?code=SYN-CODE&state=${state}`;
  const idToken = idTokenWith({ sub: 'SYN-OPERATOR-CONFIG-01', nonce });
  const forms = stubTokenEndpoint((form) =>
    form.get('grant_type') === 'authorization_code'
      // 寿命取 1 秒：落在续期提前量之内，下面 ensureSession 当场续一次，续期那一路也要证。
      ? { access_token: 'SYN-ACCESS-TOKEN-1', id_token: idToken, refresh_token: 'SYN-REFRESH-TOKEN-1', expires_in: 1 }
      : { access_token: 'SYN-ACCESS-TOKEN-2', refresh_token: 'SYN-REFRESH-TOKEN-2', expires_in: 3600 },
  );

  const { session: loggedIn, returnTo } = await completeLogin();

  equal(loggedIn.accessToken, 'SYN-ACCESS-TOKEN-1');
  equal(returnTo, '/#/price-card-catalog');
  equal(forms[0].get('code'), 'SYN-CODE');
  for (const secret of ['SYN-ACCESS-TOKEN-1', 'SYN-REFRESH-TOKEN-1', idToken]) {
    equal(session.text().includes(secret), false, `sessionStorage 里不该有 ${secret}`);
    equal(local.text().includes(secret), false, `localStorage 里不该有 ${secret}`);
  }

  equal(await accessTokenForRequest(), 'SYN-ACCESS-TOKEN-2');
  equal(forms[1].get('grant_type'), 'refresh_token');
  equal(forms[1].get('refresh_token'), 'SYN-REFRESH-TOKEN-1');
  for (const secret of ['SYN-ACCESS-TOKEN-2', 'SYN-REFRESH-TOKEN-2', idToken]) {
    equal(session.text().includes(secret), false, `续期后 sessionStorage 里不该有 ${secret}`);
    equal(local.text().includes(secret), false, `续期后 localStorage 里不该有 ${secret}`);
  }

  logout();

  equal(location.assigned[location.assigned.length - 1], '/');
  equal(await ensureSession(), null);
  equal(await accessTokenForRequest(), null);
});
