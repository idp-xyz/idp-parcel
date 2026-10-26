# 承运订舱答复口集成说明（首版）

本文是承运订舱答复口 `POST /transport-fulfillment/booking-answers`（路径前缀是现状，见「路径前缀与错误体是现状」一节）对承运方系统的首版集成说明：谁来调、怎样认证、载荷怎样写、一次订舱能答几份、各格怎样续办，以及现形进不来的答复与以后怎样扩形。

除「以后怎样扩形」一节那条对接方纪律另注依据之外，本文不另立规则。各格的权威在 [ADR-0202](../adr/0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 决定二、五与「补记：票 24 落地时的补充裁定」一节、[ADR-0239](../adr/0239-a-carrier-answer-from-a-non-provider-is-answered-booking-not-found-and-the-basis-is-kept-on-a-bounce-register.md)、[运输履约上下文](../domain/transport-fulfillment/CONTEXT.md)「订舱」词条与生命周期，以及 [UC-TF-004](../application/transport-fulfillment/UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md) 的 `AT-TF-149`、`AT-TF-150`、`AT-TF-151`、`AT-TF-153`、`AT-TF-154`、`AT-TF-326`；路径前缀与错误体的去向在 [ADR-0140](../adr/0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) 决定二、三。本文与它们相抵时以它们为准。

**首版契约**：2026-10-20 代决复核 R8 裁定，这一口按现形作为对承运商的首版集成契约，以后扩形只加取值（[ADR-0202](../adr/0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md)「复核记录（2026-10-20…）」一节）。证据层级：首发前没有真实承运商接入，本文所述行为只在演示租户 `SYN-TENANT-01` 上走通过，只记 `S`。演示里怎样取令牌、用哪一台合成承运商，见[种子 README](../../scripts/demo-seeds/README.md)「集成客户端渠道：演示发行方」一节。

## 路径前缀与错误体是现状

这两格是今天的样子，**不在「只加取值」承诺之内**，随票 [docs-consistency/code/82](../../.scratch/docs-consistency/code/82-operator-and-integration-families-still-answer-the-legacy-problem-shape-and-the-contract-is-not-generated.md) 改；改时承运方要跟着改请求路径与错误体的解析：

- **路径**：今天不带 URL 主版本，是 `/transport-fulfillment/booking-answers`。[ADR-0140](../adr/0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) 决定二把 `/v1` 延到集成客户端族各口，前缀由装配点按族加。
- **错误体**：认证没过、坏报文与 `5xx` 今天答 `{"error":{"code":"…"}}`，媒体类型 `application/json`。ADR-0140 决定三定 4xx 与 5xx 一律 `application/problem+json`（RFC 9457）；`code` 的取值原样保留，变的是它在响应体里的位置与媒体类型。

## 谁来调，怎样认证

- 调用方是承运方系统，归集成客户端族（[ADR-0149](../adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定三，ADR-0202 决定二）。租户给每个承运商登记一个集成客户端：绑定唯一租户与一个来源身份（这个承运商），并授予「承运订舱答复」（`CARRIER_BOOKING_ANSWER`）这一类事实。
- 承运方系统用客户端凭据（OAuth 2.0 client credentials）向租户配置的集成客户端发行方取令牌，请求头带 `Authorization: Bearer <令牌>`。集成客户端册要求证书绑定时，还要在同一条 TLS 连接上出示绑定的那张客户端证书。
- 租户与来源身份只取自集成客户端的绑定，载荷里不收；多写一个字段即坏报文。
- **只能答自己的订舱**：来源身份要等于这次订舱所属运输委托的服务提供方，按同一串字面比、不归一。不是的话答 `BOOKING_NOT_FOUND`，与订舱不在册同一格，见「响应」一节。
- 订舱标识由租户在订舱时定，经租户与承运方之间既有的渠道带给承运方；本产品今天没有出站订舱连接器（ADR-0202「补记：票 24 落地时的补充裁定」一节「订舱与答复都不入队」一条）。

认证没过，不进业务判断，答问题体 `{"error":{"code":"…"}}`（错误体是现状，见「路径前缀与错误体是现状」一节）：

| 状态码 | `code` | 含义 | 续办 |
|---|---|---|---|
| `401` | `INTEGRATION_CLIENT_CREDENTIAL_REJECTED` | 令牌缺失、过期、校验不过，或要求证书绑定而这次出示对不上 | 承运方运维换令牌或证书 |
| `403` | `INTEGRATION_CLIENT_NOT_GRANTED` | 令牌有效，但客户端不在册，或此刻没有承运订舱答复的生效授予 | 找租户登记授予 |
| `403` | `ACCESS_CHANNEL_NOT_CONFIGURED` | 租户没配置集成客户端族的发行方 | 找租户 |
| `403` | `OUTSIDE_ADMISSION_SCOPE` | 不在准入范围 | 找租户 |
| `503` | `IDENTITY_DEPENDENCY_UNAVAILABLE` | 认证所依赖的读口此刻读不动 | 退避后原样重发 |

## 载荷

`Content-Type: application/json`，封闭解码：不认识的字段、多余内容、超出大小上限、`outcome` 不在本文所列取值之内、`decidedAt` 解不出，都答 `400` `MALFORMED_REQUEST`（错误体同认证各格，是现状）。

| 字段 | 含义 |
|---|---|
| `booking` | 订舱标识，照租户给的原样；至多 512 个 UTF-8 字节 |
| `answer` | 承运方给这一份答复的标识；同一份答复重发要带同一个；至多 512 个 UTF-8 字节 |
| `outcome` | `ACCEPTED`（接受）、`REFUSED`（拒绝）、`EXPIRED`（失效）、`WITHDRAWN`（撤回）、`WAITLISTED`（候补）之一 |
| `quantity` | 只随 `ACCEPTED` 给：接受量，正数，单位同订舱申请，不超过申请量；少于申请量就是部分确认，仍是 `ACCEPTED` |
| `basis` | 随其余取值必给：拒绝、失效、撤回的原因，或候补的依据；`ACCEPTED` 不带 |
| `decidedAt` | 承运方作出这份答复的时刻，RFC 3339；不早于订舱申请时刻。服务端不改写、不校正（[ADR-0023](../adr/0023-work-fact-identity-and-time-are-minted-by-the-device.md)） |

## 一次订舱能答几份

- 一次订舱至多一份候补、至多一份定局；接受、拒绝、失效、撤回都是定局。还没有任何答复时，订舱读作`待确认`。
- 候补之后可以接一份定局，定局时刻不早于候补；两份都留，当前那一份是定局。
- 定局之后再来的任何一份（换了答复标识的）答 `ANSWER_CONFLICT`，原定局不变；候补之后再候补同此。
- 答复的身份是（订舱，答复标识）。同一标识、同一内容再交是重放，答 `EXISTING_ANSWER` 并回显在册的那一份；同一标识、内容不同答 `ANSWER_CONFLICT`，不回显。
- 同一次订舱的两份答复同时到达时，先判的那一份落定之后另一份才判，后判的照本节各条作答。

## 响应

状态码只回答「服务端有没有形成答案」，业务判别在响应体的 `outcome`（[ADR-0022](../adr/0022-http-status-carries-answer-formed-not-business-verdict.md)）。只有新登了一份答复用 `201`，其余答案都是 `200`。

| `outcome` | 含义 | 承运方续办 |
|---|---|---|
| `BOOKING_ANSWERED`（`201`） | 新登了这份答复；`answer` 回显它，带 `recordedAt` | 无 |
| `EXISTING_ANSWER` | 同一标识、同一内容已在册，是重放；`answer` 回显在册那一份 | 无 |
| `ANSWER_CONFLICT` | 同一标识换了内容；或这次订舱已有定局；或候补之后又来候补；或定局早于候补 | 不自动重发，交人；在册的答复不会被覆盖，要改约找租户 |
| `BOOKING_NOT_FOUND` | 订舱在本租户下不在册，或答复方不是这次订舱所属委托的服务提供方——同一格、同一副响应，不带续办引用（ADR-0239 决定一） | 找租户核对订舱标识 |
| `INPUT_NOT_ACCEPTED` | 载荷各格不成立：标识为空、带 NUL 或超过字节界；`ACCEPTED` 没带接受量、量超过申请量或带了 `basis`；其余取值带了量或没带 `basis`；`decidedAt` 缺或早于订舱申请时刻 | 改载荷再交 |
| `COMMISSION_UNDECIDED` | 订舱册或答复册此刻读写不动，带 `undecidedReason`（`BOOKING_STORE_UNAVAILABLE` 或 `ANSWER_STORE_UNAVAILABLE`）与 `continuationReference` | 稍后原样重交 |

`5xx` 表示服务端没能形成答案：留在队里，退避后原样重发。

## 现形进不来的答复

按现形作为首版契约（R8），下列答复今天进不来，都答 `ANSWER_CONFLICT`，在册的仍是原来那一份（ADR-0202「补记：票 24 落地时的补充裁定」一节越权风险点 13）。承运方要表达它们，只能请租户另起一次订舱，再对新订舱作答：

- **候补位次更新**：已候补之后，再交一份候补报告新的位次（例如从第五位升到第二位）。一次订舱只收一份候补。
- **候补后分批接受**：候补之后先接受一部分、再补接受余量。第一份接受（接受量少于申请量即部分确认）就是定局，补接受的那一份是定局之后的又一份。没有候补、直接部分接受之后再补接受，同此。

另外，答复只按一次订舱作答：不能逐对象接受，也没有委托一级的接受（ADR-0202 决定五）。

## 以后怎样扩形

以后扩形只加取值（R8）：本文所列各取值的含义不改、不删（这一承诺管 `outcome` 与各格的取值，不管路径前缀与错误体）；候补位次更新、分批接受这一类新形进来时，以新增的 `outcome` 取值（请求里的与响应里的）进来，已有的写法照旧作答。

对接方因此要守一条：**遇到不认识的响应 `outcome`，视为服务端已形成答案、但本端不会处置，出队交人**，不重发，也不当成失败。依据是 R8「只加取值」加 ADR-0140 决定五的写法：那一条的主语是首方契约（给 `outcome` 新增取值不算破坏，条件是契约写明客户端遇到未识别的 `outcome` 交人，见[首方客户 API 契约](./first-party-api-contract.md)「`outcome` 新增取值不算破坏」），这里延到集成客户端族的这一口，是代决，待 TF owner 复核。它不改服务端行为，是「只加取值」对已上线的对接方成立的前提，与 [ADR-0022](../adr/0022-http-status-carries-answer-formed-not-business-verdict.md) 对 4xx 的「出队，交给人，不自动重发」同向。
