// 管理台登录走 gk.idp.xyz 的 OIDC authorization_code + PKCE 公开客户端流。
//
// 边界先说清：这是 UI 登录门（谁能打开控制台页面），不是后端准入形——parcel-api
// 的查阅准入仍由隔离读独任（ADR-0078），登记写面的准入形另有裁决在途（ADR-0085），
// 两处都不因本模块存在而改变；本模块取得的令牌今天不随 /api 请求外送。
//
// 令牌交换经 vite 代理 `/oidc` 同源转发：gk.idp.xyz 的 token 端点未对本地开发源
// 放 CORS（实测预检 204 无 Access-Control-Allow-Origin），顶层跳转不受 CORS 约束，
// 故只有 XHR 的一段走代理，授权跳转仍直达签发方。
//
// 下面两个值是开发部署形态（开发方自己的身份服务与其上注册的公开客户端），不是
// 租户实例参数；换部署时改这里并同步签发方上的回调白名单。
const ISSUER = 'https://gk.idp.xyz';
const CLIENT_ID = 'idp-parcel';
const SCOPE = 'openid profile email offline_access';

// 同源代理前缀：/oidc/oauth2/token → {ISSUER}/oauth2/token（见 vite.config.ts）。
const TOKEN_URL = '/oidc/oauth2/token';
const AUTHORIZE_URL = `${ISSUER}/oauth2/auth`;

export const CALLBACK_PATH = '/auth/callback';

const PENDING_KEY = 'parcel.oidc.pending';
const SESSION_KEY = 'parcel.oidc.session';

// 提前量：到期前这么久就当作不可用并续期，避免拿着临界令牌发请求。
const RENEW_SKEW_MS = 30_000;

// 签发方漏给 expires_in 时的兜底寿命。不能记成「此刻已过期」——那会让每次
// 会话检查都去换一次令牌，把轮换型 refresh_token 磨完。
const FALLBACK_LIFETIME_SECONDS = 300;

export interface OidcSession {
  accessToken: string;
  idToken: string;
  refreshToken?: string;
  /** epoch 毫秒的令牌到期时刻；实际何时视为不可用见 RENEW_SKEW_MS。 */
  expiresAt: number;
  claims: Record<string, unknown>;
}

interface PendingLogin {
  verifier: string;
  state: string;
  nonce: string;
  returnTo: string;
}

function randomUrlSafe(byteLength: number): string {
  const bytes = new Uint8Array(byteLength);
  crypto.getRandomValues(bytes);
  return base64UrlEncode(bytes);
}

function base64UrlEncode(bytes: Uint8Array): string {
  let binary = '';
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

async function pkceChallenge(verifier: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier));
  return base64UrlEncode(new Uint8Array(digest));
}

// atob 产出的是一串 Latin-1 字节，直接当文本 JSON.parse 会把多字节字符按字节
// 拆开——操作员姓名多为中文，那样解出来就是乱码。必须按 UTF-8 重解一次。
function base64UrlDecodeToText(value: string): string {
  const base64 = value.replace(/-/g, '+').replace(/_/g, '/');
  const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4);
  const binary = atob(padded);
  return new TextDecoder().decode(Uint8Array.from(binary, (ch) => ch.charCodeAt(0)));
}

// id_token 载荷只做 base64url 解码取声明展示，不在前端验签：签名的受益方是
// 资源服务器，本模块拿它只当「登录了谁」的显示来源，信道安全由 TLS 承担。
function decodeJwtPayload(token: string): Record<string, unknown> {
  const payload = token.split('.')[1] ?? '';
  if (!payload) return {};
  try {
    return JSON.parse(base64UrlDecodeToText(payload)) as Record<string, unknown>;
  } catch {
    // 解不开就当没有声明：这只影响徽章显示什么名字，不该把整次登录判死。
    return {};
  }
}

export function isCallback(): boolean {
  return window.location.pathname === CALLBACK_PATH;
}

export async function beginLogin(returnTo: string): Promise<void> {
  const verifier = randomUrlSafe(32);
  const pending: PendingLogin = {
    verifier,
    state: randomUrlSafe(16),
    nonce: randomUrlSafe(16),
    returnTo,
  };
  sessionStorage.setItem(PENDING_KEY, JSON.stringify(pending));

  const url = new URL(AUTHORIZE_URL);
  url.searchParams.set('client_id', CLIENT_ID);
  url.searchParams.set('response_type', 'code');
  url.searchParams.set('scope', SCOPE);
  url.searchParams.set('redirect_uri', redirectUri());
  url.searchParams.set('state', pending.state);
  url.searchParams.set('nonce', pending.nonce);
  url.searchParams.set('code_challenge', await pkceChallenge(verifier));
  url.searchParams.set('code_challenge_method', 'S256');
  window.location.assign(url.toString());
}

function redirectUri(): string {
  return `${window.location.origin}${CALLBACK_PATH}`;
}

// React StrictMode 在开发模式会把挂载 effect 跑两遍，两次并发交换会拿同一个
// code 各 POST 一次——授权码单次有效，第二次必 invalid_grant，且规范允许签发方
// 就此吊销第一次已发的令牌。模块级单例让两次调用共享同一个在途 Promise。
let inFlightCompletion: Promise<{ session: OidcSession; returnTo: string }> | null = null;

/** 回调页调用：核对 state、用 code 换令牌，成功后返回应回到的页内路径。 */
export function completeLogin(): Promise<{ session: OidcSession; returnTo: string }> {
  inFlightCompletion ??= doCompleteLogin().catch((error: unknown) => {
    // 失败后放行下一次重试；成功的结果则保持共享，防重复消费。
    inFlightCompletion = null;
    throw error;
  });
  return inFlightCompletion;
}

async function doCompleteLogin(): Promise<{ session: OidcSession; returnTo: string }> {
  const params = new URLSearchParams(window.location.search);
  const authError = params.get('error');
  if (authError) {
    throw new Error(`签发方拒绝授权：${authError}（${params.get('error_description') ?? '无说明'}）`);
  }
  const code = params.get('code');
  const state = params.get('state');
  const rawPending = sessionStorage.getItem(PENDING_KEY);
  if (!code || !state || !rawPending) {
    throw new Error('回调缺少 code/state 或本地无进行中的登录，请重新登录');
  }
  let pending: PendingLogin;
  try {
    pending = JSON.parse(rawPending) as PendingLogin;
  } catch {
    sessionStorage.removeItem(PENDING_KEY);
    throw new Error('本地的登录中间态已损坏，请重新登录');
  }
  if (state !== pending.state) {
    throw new Error('state 不匹配，登录请求可能被伪造，请重新登录');
  }
  sessionStorage.removeItem(PENDING_KEY);

  const session = await exchange({
    grant_type: 'authorization_code',
    code,
    redirect_uri: redirectUri(),
    client_id: CLIENT_ID,
    code_verifier: pending.verifier,
  });
  // 先要有 id_token 才谈得上核 nonce：没有它就无从判断登录的是谁，此时报
  // 「nonce 不匹配」会把成因指错方向。
  if (!session.idToken) {
    sessionStorage.removeItem(SESSION_KEY);
    throw new Error('签发方未返回 id_token，无法确认登录的是谁，请重新登录');
  }
  if (session.claims.nonce !== pending.nonce) {
    sessionStorage.removeItem(SESSION_KEY);
    throw new Error('nonce 不匹配，令牌不来自本次登录，请重新登录');
  }
  return { session, returnTo: pending.returnTo || '/' };
}

// previous 只在续期时传：续期响应按规范可以省略 id_token 与 refresh_token，
// 省略处沿用上一次的值，不能当成「身份没了」。
async function exchange(form: Record<string, string>, previous?: OidcSession): Promise<OidcSession> {
  let response: Response;
  try {
    response = await fetch(TOKEN_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams(form).toString(),
    });
  } catch {
    throw new Error('连不上令牌端点，请检查开发代理 /oidc 是否可达');
  }
  const body = (await response.json().catch(() => ({}))) as Record<string, unknown>;
  if (!response.ok) {
    throw new Error(
      `令牌交换失败（HTTP ${response.status}）：${String(body.error ?? '')} ${String(body.error_description ?? '')}`.trim(),
    );
  }

  const accessToken = typeof body.access_token === 'string' ? body.access_token : '';
  if (!accessToken) {
    // 交换「成功」却没拿到令牌，等于没登录。这里必须硬失败：把空令牌当会话存
    // 下去就是造了一个看起来登录了的身份，红线「不造开发用采信身份」不允许。
    throw new Error('签发方未返回访问令牌，登录未完成');
  }
  const idToken = typeof body.id_token === 'string' && body.id_token ? body.id_token : (previous?.idToken ?? '');
  const lifetimeSeconds = Number(body.expires_in);

  const session: OidcSession = {
    accessToken,
    idToken,
    // 轮换型 refresh_token：给了新的就换，没给则沿用旧的。
    refreshToken: typeof body.refresh_token === 'string' ? body.refresh_token : previous?.refreshToken,
    expiresAt:
      Date.now() +
      (Number.isFinite(lifetimeSeconds) && lifetimeSeconds > 0 ? lifetimeSeconds : FALLBACK_LIFETIME_SECONDS) * 1000,
    claims: decodeJwtPayload(idToken),
  };
  sessionStorage.setItem(SESSION_KEY, JSON.stringify(session));
  return session;
}

function readStoredSession(): OidcSession | null {
  const raw = sessionStorage.getItem(SESSION_KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as OidcSession;
  } catch {
    // 存坏了就当没登录，别让一条坏记录把门永久卡在异常上。
    sessionStorage.removeItem(SESSION_KEY);
    return null;
  }
}

// 定时续期、页面回到前台、门自己的挂载检查会并发触到续期。轮换型 refresh_token
// 下第二次续期必失败，且规范允许签发方就此吊销整条令牌链——与授权码交换同理，
// 用在途 Promise 单例把并发收成一次。
let inFlightRenewal: Promise<OidcSession | null> | null = null;

/** 取当前可用会话；临近到期且有 refresh_token 时静默续期，续不动返回 null。 */
export async function ensureSession(): Promise<OidcSession | null> {
  const session = readStoredSession();
  if (!session) return null;
  if (Date.now() < session.expiresAt - RENEW_SKEW_MS) return session;
  if (!session.refreshToken) {
    sessionStorage.removeItem(SESSION_KEY);
    return null;
  }
  inFlightRenewal ??= renew(session).finally(() => {
    inFlightRenewal = null;
  });
  return inFlightRenewal;
}

async function renew(previous: OidcSession): Promise<OidcSession | null> {
  try {
    return await exchange(
      {
        grant_type: 'refresh_token',
        refresh_token: previous.refreshToken ?? '',
        client_id: CLIENT_ID,
      },
      previous,
    );
  } catch {
    sessionStorage.removeItem(SESSION_KEY);
    return null;
  }
}

/** 距该会话下次该续期还有多久（毫秒）；已到点返回 0。供登录门排定时器。 */
export function millisUntilRenewal(session: OidcSession): number {
  return Math.max(0, session.expiresAt - RENEW_SKEW_MS - Date.now());
}

// 只清本地会话，不做 RP 发起的签发方登出：签发方上登记的登出回调还是占位值，
// 跳过去只会落在别人的域名上。签发方侧会话由其自身过期策略结束。
export function logout(): void {
  sessionStorage.removeItem(SESSION_KEY);
  sessionStorage.removeItem(PENDING_KEY);
  window.location.assign('/');
}

/** 展示用：从声明里挑一个人能认出来的名字。 */
export function displayName(session: OidcSession): string {
  const claims = session.claims;
  return String(claims.email ?? claims.name ?? claims.preferred_username ?? claims.sub ?? '已登录');
}
