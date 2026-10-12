# 07 管理台登录门：authorization_code + PKCE、Bearer 与三态文案

Category: enhancement
Status: in-progress——2026-10-12 通道 5：做什么两条与接演示发行方共三笔已落（`d9f02f6c`、`97c1bd47`、`22f0529e`），判据两条都已 ✅（证据 `S`，见文末「进度记录」）；只差 Spec 轴非作者评审（通道 1 派通道 3，`task-4764fef1`，钉 `97c1bd47`），交回照录进 Comments 即转 resolved。此前 in-progress——2026-10-12 通道 5：做什么两条已落（`d9f02f6c`、`97c1bd47`），判据的三态浏览器验收已过；走通那一格等 [17](./17-demo-issuer-dex-and-synthetic-operators.md) 交出演示发行方的取值。再此前 in-progress——2026-10-12 通道 5 认领（通道 1 派单 `task-9c227f05`），前端切片，在共享树 `main` 上做（基 `ff3b01fa`）；更早 ready-for-agent——2026-09-24 拆法经用户授权通道 4 自决认可
Blocked by: 02、03（均已 resolved）；17 已交值（「交给 07」钉 `mcp4-oc17@4c9dc789`），走通那一格不再等它
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`apps/admin-web/**`（前端切片，按 [workflow.md「前端切片」](../../../docs/agents/workflow.md) 一人在 main 上直接做）。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定二第一条与 Consequences。

## 做什么

1. 外壳加登录门：authorization_code + PKCE 取令牌，请求带 Bearer；令牌只在内存，不落本地存储。
2. 各页「接入渠道未配置」的单一文案分成三态：渠道未配置、需要登录、没有授予。

## 完成判据

- 在接好发行方的隔离环境里，合成操作者能登进管理台并在 04、05 已换的口上走通；三态文案各有一条浏览器验收。

## 进度记录（通道 5，2026-10-12，钉 `22f0529e`）

### 落点

- `d9f02f6c`（做什么第 1 条）：`auth/oidc.ts` 的访问令牌、id_token、refresh_token 只放模块内存（`currentSession`），登录中间态（PKCE verifier、state、nonce、回到哪）仍在 `sessionStorage`，要熬过跳签发方的整页导航；`auth/bearer.ts` 的 `withBearer` 由 `main.tsx` 接 `accessTokenForRequest`，`catalogue-api`、委托、全程追踪各出口经它带 Bearer。
- `97c1bd47`（做什么第 2 条）：`auth/access-refusal.ts` 按状态码与码名原名判三格（ADR-0100 决定四），各出口的答复代数共用这一份；`components/states/access-state.tsx` 是三态卡，只有需要登录带一颗「重新登录」（`auth/ReloginButton.tsx`，回到此刻这一页），`OperatorRefusalNote` 是行内一句；`TemplateViewState` 加两态；各页的页面态、动作答案区、表单候选与词表判读、抽屉分段、摘要条都接上两格。
- `22f0529e`（接演示发行方，照 17「交给 07」）：`auth/oidc.ts` 的 `ISSUER` 取 `http://127.0.0.1:5556/dex`（与 `parcel-api` 的 `IDP_PARCEL_OPERATOR_OIDC_ISSUER` 逐字相同），`CLIENT_ID` 取 `idp-parcel-admin-web`（即操作者族受众），授权端点 `<issuer>/auth`；换令牌照旧经同源代理，路径 `/oidc/dex/token`，`vite.config.ts` 的 `/oidc` 目标随之换，系统代理的 agent 只对 https 目标接；登录页不再写死 gk.idp.xyz，示发行方地址，按钮为「前往发行方登录」；`oidc.test.ts` 钉授权端点、客户端标识、回调地址、scope 与换令牌路径。

### 判据逐条

- ✅ **合成操作者登进管理台并在 04、05 已换的口上走通**（证据层级 `S`，2026-10-12 21:1x）。
  - 环境：发行方是通道 4 跑着的 `idp-parcel-demo-issuer`（`ghcr.io/dexidp/dex:v2.45.1`，`127.0.0.1:5556`，配置出自 `mcp4-oc17@88ca1100`，未重启）；库是 55432 上的一次性库 `mcp5_oc07`，在 `mcp4-oc17@88ca1100` 的检出上 `seed.sh --reset` 整套灌，退 0，操作者册就是该分支 `data/access` 的两份批文（Dex 的 issuer 加真 `sub`；配置员有登记册配置写与查阅两格授予，查阅员只有查阅）；`parcel-api` 由 `22f0529e` 构建（与 `88ca1100` 的 Go 代码和迁移零差异），带三件参数起在 `127.0.0.1:19087`；管理台是 `22f0529e` 的 vite dev，`http://localhost:5173`，`PARCEL_API_TARGET` 指 19087。
  - 自检：17 的 `probe-operator-channel.sh` 对这只 `parcel-api` 直连跑两遍、经 vite 的 `/api` 代理跑一遍，三格每遍都答 200 `REFUSED/VERSION_NOT_OVERWRITABLE`、403 `OPERATOR_NOT_GRANTED`、401 `OPERATOR_CREDENTIAL_REJECTED`。
  - 登录：首屏是登录页，示 `http://127.0.0.1:5556/dex`；「前往发行方登录」落到 Dex 的本地口令页，以合成配置员登录后回到原页 `#/disclosure-policies`，顶栏显出它的邮箱；`sessionStorage`、`localStorage` 里查不到 JWT。页内记下此后每个 `/api` 请求：全带 `Authorization: Bearer <JWT>`，解出 `iss` 为 `http://127.0.0.1:5556/dex`、`aud` 为 `idp-parcel-admin-web`、`sub` 为配置员在 Dex 的真 `sub`——访问令牌就是 `parcel-api` 认的那一枚。
  - 04 已换的口，配置员：
    - 可见性·通知策略登记（`POST /visibility-catalogue-notification-policy-registrations`，「披露与通知策略」页的「登记披露口径」签）：种子那条去掉 `tenantId` 再登，答 200 `REFUSED` / `VERSION_NOT_OVERWRITABLE`，页上是「登记册答复：REFUSED……（治理答案,不是失败）」，与 17 取证的同一格；换新策略引用 `SYN-VE-DISCLOSE-OC07-WALK` 再登，答 201 `REGISTERED`。
    - 网络·节点登记（`POST /network-catalog-node-registrations`，「网络目录」页的「登记网络目录」签）：`SYN-NODE-SHA-HUB` 第 3 版，答 201 `REGISTERED`。
    - 计价·价卡导入（price-card-import 那几口，同挂操作者 Intake）：售价卡工作簿由模板包夹具经 `go test -overlay` 一次性写出（不进树）。预览答 200 `VALIDATED`（`SYN-PLAN-CN-SG-01@v1`，没有逐格问题）；存草稿答 201 `DRAFT_SUBMITTED`，册上那一行的录入者是 `http://127.0.0.1:5556/dex#<配置员 sub>`，取自令牌、不是页面自报；草稿签列表答 200 `PRICE_CARD_DRAFTS_LISTED`；批准答 200 `NOT_CONFIGURED`（本租户没登记价卡发布审批职责规则，那是租户取值，页上如实说未配置，不默认单人或双人）。
  - 查阅员（同一浏览器「退出」回到登录页，再以合成查阅员登录）：同一个通知策略登记口答 403 `OPERATOR_NOT_GRANTED`，页上行内一句「没有授予（403 OPERATOR_NOT_GRANTED）……」，没有「重新登录」；价卡草稿签（`GET /pricing-price-card-draft-views`）呈「没有授予」卡，只有「刷新」。
  - 05：没有已换的口（05 仍 ready-for-agent），这一半今天是空集。目录读口照旧答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`，「披露与通知策略」「网络目录」「价卡目录」三页的查阅签都呈「接入渠道未配置」，配置员与查阅员相同。
- ✅ **三态文案各有一条浏览器验收**（证据层级 `S`）。做法：Cursor 内置浏览器打开 `97c1bd47` 的 `vite build` 产物，由仓外一次性服务托管；`/oidc/oauth2/token` 回一份合成令牌（前端只解码 id_token、核 state 与 nonce，不验签，验签归 `parcel-api`），`/api/*` 按路径前缀回三格拒绝并记下请求头。
  - 登录门：首屏是登录页，控制台组件树不挂载。注入一份已知的 PKCE 中间态，再从 `/auth/callback?code=…&state=…` 进来：令牌交换带的正是那份 verifier 与本地回调地址，随后落回 `#/price-card-catalog`，顶栏显出合成操作者。此后每个 `/api` 请求都带 `Authorization: Bearer <合成令牌>`；`sessionStorage` 与 `localStorage` 里都查不到令牌原文。
  - 需要登录：价卡目录（`GET /pricing-price-cards` 答 401 `OPERATOR_CREDENTIAL_REJECTED`）呈「需要登录」卡，带续办句、主责上下文与出处、码名原文，恰有一颗「重新登录」。点它即对 gk.idp.xyz 发起 authorization_code + PKCE（落到签发方的登录页）；回到管理台时，新的登录中间态记着 `returnTo=/#/price-card-catalog`，内存里的会话已随整页离开而没了，门退回登录页。
  - 没有授予：计价参考序列（列表、覆盖、挂起评价数三口答 403 `OPERATOR_NOT_GRANTED`）呈「没有授予」卡，续办指向管授予的人，没有按钮；页头摘要条的两行各答「没有授予（403 OPERATOR_NOT_GRANTED）」、今天没有问到。
  - 渠道未配置：委托查阅（`GET /shipment-request-views` 答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`）呈「接入渠道未配置」卡，该页原有说明一字未动，没有按钮。
  - 三页互不串：每页只出自己那一格的码名，另两格的码名在页上查不到。
  - 真后端补证（上面走通那一趟顺带看到的）：没有授予、渠道未配置两格由真 `parcel-api` 答出，页上与注入时同形；需要登录那一格的真 401 只在 probe「不带令牌」那一枪里，浏览器里没造（见「未验」）。

### 门与 CI

- 三道门（Node 22.23.3，钉 `97c1bd47`）：`tsc -b --noEmit` 退 0；`run-tests` 508/508；`vite build` 退 0。新用例的判别力用四处临时变异证过：三格折回未配置、目录页把两格折回未配置、没有授予也给重新登录、挂起评价数并进故障，各自红，已逐字节还原。
- CI：`d9f02f6c` 的 run 38215973390 success；`97c1bd47` 的 run 38218879934 success（七个 job 全绿，含 `admin-web typecheck, tests, build`）。
- `22f0529e`：三道门（Node 22.23.3）`tsc -b --noEmit` 退 0、`run-tests` 518/518、`vite build` 退 0；新钉的几格用两处临时变异证过——客户端标识换回旧值、换令牌路径换回旧路径，各自红，已逐字节还原；CI run 38251468923 success。

### 判断项

1. 三格的判据是「状态码 + 码名原名」一起对上，与未配置一向的判法相同；别的 401 / 403（如集成客户端那一族的码）照旧交回调用方问题，不冒充这三格。
2. 「需要登录」不在答 401 时自动跳走：页上可能有填了一半的表单，跳走就丢了；由人按「重新登录」，回到此刻这一页。
3. 需要登录与没有授予的续办句不随页而异（登录与授予都不随页面变），各页共用 `operatorRefusalRecovery`，页面只给主责上下文与出处；未配置那一格的续办随口而异，各页原有说明不动。
4. 换令牌仍走同源代理 `/oidc`：演示发行方对本地开发源放了 CORS，直连也通；留着代理，换令牌那一下就不依赖签发方放不放 CORS，换到不放的签发方时只改代理目标（`oidc.ts` 文件头）。
5. 发行方地址与客户端标识仍是 `oidc.ts` 里的源码常量，只从 gk.idp.xyz 换成演示发行方：「做什么」没有要求把它们做成部署期参数，本票不越过去；同一文件头写明它们是演示部署形态，不是租户实例参数，也不是任何部署的默认值。

### 未验

- 走通靠的是 17 尚未进 `main` 的内容：Dex 的配置与按 Dex 另登的操作者册种子都在 `mcp4-oc17@88ca1100`。17 进 `main` 时若改了发行方地址、客户端标识或回调地址，`oidc.ts` 与 `vite.config.ts` 要跟着改；`oidc.test.ts` 钉着这几格，不跟会红。
- refresh_token 续期在真发行方上的行为：演示发行方的 `dex.yaml` 没设 `expiry`，令牌按 Dex 缺省过期，走查时长内等不到续期那一下；仍只有 `oidc.test.ts` 的节点用例钉着。
- 需要登录那一格在浏览器里的真后端答复：没造无效或过期的令牌（Dex 不重启，令牌也没到期），真后端的 401 只有 probe「不带令牌」那一枪；浏览器那一格是注入的（见三态那条）。

### 评审

本票动到 `components/*` 与 `templates/*`（共享面），按 workflow「前端切片」第 5 步要一份 Spec 轴评审：通道 1 已派通道 3（`task-4764fef1`，钉 `97c1bd47`，范围 `ff3b01fa..97c1bd47` 的 `apps/admin-web`），交回照录进 Comments，有阻断在 `main` 上另提一笔修。`22f0529e` 只动 `auth/*` 与 `vite.config.ts`，不在共享面。
