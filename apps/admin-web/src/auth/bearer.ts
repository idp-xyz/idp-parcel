// 发往 /api 的每个请求都带 Bearer（ADR-0100 决定二第一条；票 operator-channel/07）。令牌由 parcel-api 自己校验，
// 这里只负责带上，不替服务端判它好不好——认不认由答复格说（需要登录 / 没有授予 / 渠道未配置）。
//
// 令牌从哪来由装配点注入（main.tsx 接 oidc.ts 的会话），各 fetch 出口共用这一个口、不各自去读会话：读法只此一处，
// 换会话实现时不会有哪个出口还剩一份旧读法。未注入时（node:test、dom-probe 直接挂页面）请求照发、不带令牌，
// 服务端照实答；不在这里造一个令牌顶上。

type BearerSource = () => Promise<string | null>;

let source: BearerSource = () => Promise.resolve(null);

export function configureBearerSource(next: BearerSource): void {
  source = next;
}

/**
 * 给一份请求头补上 `Authorization: Bearer …`；取不到令牌就原样交回，不加空的那一格。
 * 只收普通对象：各出口写的都是对象字面量，交回的也是对象，调用方与测试按键读得到。
 */
export async function withBearer(headers: Record<string, string>): Promise<Record<string, string>> {
  const token = await source();
  return token ? { ...headers, Authorization: `Bearer ${token}` } : headers;
}
