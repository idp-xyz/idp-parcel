# 07 管理台登录门：authorization_code + PKCE、Bearer 与三态文案

Category: enhancement
Status: in-progress——2026-10-12 通道 5：做什么两条已落（`d9f02f6c`、`97c1bd47`），判据的三态浏览器验收已过；走通那一格等 [17](./17-demo-issuer-dex-and-synthetic-operators.md) 交出演示发行方的取值，管理台这一侧接上之后回本票收口（见文末「进度记录」）。此前 in-progress——2026-10-12 通道 5 认领（通道 1 派单 `task-9c227f05`），前端切片，在共享树 `main` 上做（基 `ff3b01fa`）；再此前 ready-for-agent——2026-09-24 拆法经用户授权通道 4 自决认可
Blocked by: 02、03（均已 resolved）；完成判据的走通那一格另等 17
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`apps/admin-web/**`（前端切片，按 [workflow.md「前端切片」](../../../docs/agents/workflow.md) 一人在 main 上直接做）。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定二第一条与 Consequences。

## 做什么

1. 外壳加登录门：authorization_code + PKCE 取令牌，请求带 Bearer；令牌只在内存，不落本地存储。
2. 各页「接入渠道未配置」的单一文案分成三态：渠道未配置、需要登录、没有授予。

## 完成判据

- 在接好发行方的隔离环境里，合成操作者能登进管理台并在 04、05 已换的口上走通；三态文案各有一条浏览器验收。

## 进度记录（通道 5，2026-10-12，钉 `97c1bd47`）

### 落点

- `d9f02f6c`（做什么第 1 条）：`auth/oidc.ts` 的访问令牌、id_token、refresh_token 只放模块内存（`currentSession`），登录中间态（PKCE verifier、state、nonce、回到哪）仍在 `sessionStorage`，要熬过跳签发方的整页导航；`auth/bearer.ts` 的 `withBearer` 由 `main.tsx` 接 `accessTokenForRequest`，`catalogue-api`、委托、全程追踪各出口经它带 Bearer。
- `97c1bd47`（做什么第 2 条）：`auth/access-refusal.ts` 按状态码与码名原名判三格（ADR-0100 决定四），各出口的答复代数共用这一份；`components/states/access-state.tsx` 是三态卡，只有需要登录带一颗「重新登录」（`auth/ReloginButton.tsx`，回到此刻这一页），`OperatorRefusalNote` 是行内一句；`TemplateViewState` 加两态；各页的页面态、动作答案区、表单候选与词表判读、抽屉分段、摘要条都接上两格。

### 判据逐条

- ◑ **合成操作者登进管理台并在 04、05 已换的口上走通**——缺演示发行方。`oidc.ts` 今天接的是 gk.idp.xyz（开发方自己的身份服务，发现文档实测为 Ory Hydra），本机没有它上面的合成操作者账户；演示种子 `scripts/demo-seeds/data/access/` 里操作者的发行方是合成值 `https://syn-issuer-01.example.invalid`，没有哪个真发行方签得出它的令牌。演示发行方（Dex）、按它另登的合成操作者与 `parcel-api` 的三件部署参数归 [17](./17-demo-issuer-dex-and-synthetic-operators.md)。17 的「交给 07」一节交回发行方地址、授权与令牌端点、客户端标识、回调地址和该带哪一枚令牌之后，管理台这一侧的接法（`oidc.ts` 的发行方与端点、`vite.config.ts` 的 `/oidc` 代理目标）回本票做，再走通。
- ✅ **三态文案各有一条浏览器验收**（证据层级 `S`）。做法：Cursor 内置浏览器打开 `97c1bd47` 的 `vite build` 产物，由仓外一次性服务托管；`/oidc/oauth2/token` 回一份合成令牌（前端只解码 id_token、核 state 与 nonce，不验签，验签归 `parcel-api`），`/api/*` 按路径前缀回三格拒绝并记下请求头。
  - 登录门：首屏是登录页，控制台组件树不挂载。注入一份已知的 PKCE 中间态，再从 `/auth/callback?code=…&state=…` 进来：令牌交换带的正是那份 verifier 与本地回调地址，随后落回 `#/price-card-catalog`，顶栏显出合成操作者。此后每个 `/api` 请求都带 `Authorization: Bearer <合成令牌>`；`sessionStorage` 与 `localStorage` 里都查不到令牌原文。
  - 需要登录：价卡目录（`GET /pricing-price-cards` 答 401 `OPERATOR_CREDENTIAL_REJECTED`）呈「需要登录」卡，带续办句、主责上下文与出处、码名原文，恰有一颗「重新登录」。点它即对 gk.idp.xyz 发起 authorization_code + PKCE（落到签发方的登录页）；回到管理台时，新的登录中间态记着 `returnTo=/#/price-card-catalog`，内存里的会话已随整页离开而没了，门退回登录页。
  - 没有授予：计价参考序列（列表、覆盖、挂起评价数三口答 403 `OPERATOR_NOT_GRANTED`）呈「没有授予」卡，续办指向管授予的人，没有按钮；页头摘要条的两行各答「没有授予（403 OPERATOR_NOT_GRANTED）」、今天没有问到。
  - 渠道未配置：委托查阅（`GET /shipment-request-views` 答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`）呈「接入渠道未配置」卡，该页原有说明一字未动，没有按钮。
  - 三页互不串：每页只出自己那一格的码名，另两格的码名在页上查不到。

### 门与 CI

- 三道门（Node 22.23.3，钉 `97c1bd47`）：`tsc -b --noEmit` 退 0；`run-tests` 508/508；`vite build` 退 0。新用例的判别力用四处临时变异证过：三格折回未配置、目录页把两格折回未配置、没有授予也给重新登录、挂起评价数并进故障，各自红，已逐字节还原。
- CI：`d9f02f6c` 的 run 38215973390 success；`97c1bd47` 的 run 38218879934 success（七个 job 全绿，含 `admin-web typecheck, tests, build`）。

### 判断项

1. 三格的判据是「状态码 + 码名原名」一起对上，与未配置一向的判法相同；别的 401 / 403（如集成客户端那一族的码）照旧交回调用方问题，不冒充这三格。
2. 「需要登录」不在答 401 时自动跳走：页上可能有填了一半的表单，跳走就丢了；由人按「重新登录」，回到此刻这一页。
3. 需要登录与没有授予的续办句不随页而异（登录与授予都不随页面变），各页共用 `operatorRefusalRecovery`，页面只给主责上下文与出处；未配置那一格的续办随口而异，各页原有说明不动。

### 未验

- 真发行方签的令牌打进 `parcel-api` 答哪一格：没有账户，未验。gk.idp.xyz 的访问令牌是不是 JWT、`aud` 里有没有 `parcel-api` 的受众，同样未验，归 17「该带哪一枚令牌」那一格。
- refresh_token 续期在真发行方上的行为：只有 `oidc.test.ts` 的节点用例钉着。

### 评审

本票动到 `components/*` 与 `templates/*`（共享面），按 workflow「前端切片」第 5 步要一份 Spec 轴评审，待通道 1 派非作者。
