# ADR-0139：产品自有的客户接入渠道（首方 API）是产品自有的接入渠道族，属机制半边——信任锚、凭据形态、渠道登记册结构与来源信封铸造由产品定义，不等 `PAR-INT-01`；`PAR-INT-01` 收窄为「某租户的客户实际启用哪条渠道」与「租户既有渠道的适配」；ADR-0072 Decision 二余下那一半与 ADR-0085 补记「两族各自要过」的客户半据此收窄

Status: Accepted（2026-10-19 **修订后接受**：用户在 Claude Code 会话里答「接受」，按会话复核建议修订。修订要点：凭据机制与校验顺序照集成客户端族（[ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定三），首方客户族、操作者族、集成客户端族三族分立（Decision 二、三）；首方族自带三件部署参数；客户渠道册行形照集成客户端册，生效区间与撤销挂在授予上；答复扩为按恢复动作分的各格并命名错误码（Decision 四）；ADR-0055 Decision 五两项照 ADR-0149 决定四延到客户面各口（新增 Decision 八）；隔离形态照 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 退场（Decision 六、七）；「既有渠道适配」按 [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 收窄为按某客户自有契约的适配；部分停用清单增 ADR-0100、ADR-0101、ADR-0106；越权风险点逐条按复核建议处置。本记录凡言「机制半边」，按 ADR-0146 读作产品交付（机制与产品策略）；「实例半边」读作租户取值。此前 Proposed——**草案**，2026-09-16；**草案修订**同日，用户经 IDP 队列通道 1 令「按建议处置」，据通道 1 的复核修订——改写 Decision 二凭据形态一条的校验顺序（签名与声明校验前置于 `Minter`、不读册，`Minter` 内只核绑定；原句「既有顺序不变」与 Decision 四的三格互斥，会让登记册成员被未认证调用方枚举），Context 补候选租户既有 API 渠道证据（E-02）的归族，不再停用 ADR-0055 Alternatives 那一条、ADR-0085 客户半改为收窄，`/claims` 改归命令面，Decision 五补「接入请求标识」不进摘要，Alternatives 补 API Key 一条的市场取舍，风险点去掉预判并补齐；修订时另读过 `.scratch/syn-wall-door-audit/issues/01` 的「重启范围」节、`.scratch/tenant-implementation-01/instance-register-draft.md` 的 `PAR-INT-01` 行与 `implementation-week-decisions-and-customer-pack.md` 的 D-08 行、`internal/parcelshipment/adapters/http/cancel_parcel.go` 与 `internal/visibilityexception/adapters/http/receive_claim.go` 的端点构造函数注释。用户经 IDP 队列通道 5 先质疑「我们这是一个我们公司的软件产品，有没有客户都需要 internal/public api 的吧，原来的规范老是等真实的客户，不适合一个软件公司开发产品的吧」，再令「起草」；本记录据此起草，**接受与否归用户**，接受前不是依据、不停用任何记录的任何条款。裁决能力边界：读过 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)、[ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)、[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、[ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md) 全文，[ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md) 的 Status 行与 Decision 二同日补记，[ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) Decision 一、三、四、六，[ADR-0018](./0018-product-clients-share-release-boundary-under-apps.md) / [ADR-0019](./0019-product-ships-tenant-facing-operator-clients.md) 谱系中关于 `PAR-INT-01` 的句子，`internal/accessidentity` 全部生产文件，`cmd/parcel-api` 的 `assembleBusinessEndpoints` 与 `internal/platform/httpapi` 路由，[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)的 `PAR-INT-01` 至 `PAR-INT-07` 行，[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)对机制半边的定义句，[parcel-shipment CONTEXT](../domain/parcel-shipment/CONTEXT.md) 邻接上下文里关于共享身份认证与授权技术能力的那句，[UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md) 输入语义契约的来源信封行，以及 [`docs/api` 的 Go + chi + Scalar 方案稿](../api/Go_Chi_Scalar_API_Documentation_Final_V1.0.md)全文；未重读 [ADR-0003](./0003-group-tenant-legal-entity-customer-account.md) 全文、`pilotgovernance` 上下文与 `PAR-GOV-03..07` 相关记录及代码、UC-PS-001 其余各节——本记录因此只裁**首方客户渠道的归属与准入结构**，不裁契约正文、不裁 `PAR-GOV-03..07` 与 ADR-0055 Decision 五两项对客户业务面还拦不拦、不裁任何登记册的领域不变量）
Date: 2026-09-16

## Context

**客户业务面在生产装配下今天一口也进不来，而拦着它的不是缺客户，是一句把产品能力归错半边的话。** `cmd/parcel-api` 的 `assembleBusinessEndpoints` 里，客户业务命令面（委托提交、撤回、逐包裹取消、受控补充、资料修订，以及 `UC-VE-007` 索赔提交 `/claims`）与客户查阅面（`/customer-tracking-view`）除 `/shipment-requests` 一行经 ADR-0091 的隔离提交开关可换值外，其余各行都挂着字面量 `UnconfiguredIntake{}`；生产装配没有任何一行开关，全部对一切请求如实答 `403` `ACCESS_CHANNEL_NOT_CONFIGURED`。`internal/accessidentity` 里铸造那一段已经成型——`Minter` 从 `ChannelRegistry` 取一行 `ChannelRegistration`、经 `CredentialVerifier` 核验、`seal` 出四要素 `SourceEnvelope`，身份三要素结构上取不到请求体——但该包 `doc.go`「客户渠道那一半仍等 PAR-INT-01；操作者与集成客户端两族已立」一节写明：客户渠道一侧只有装载口 `ChannelRegistry` 这个接口，`migrations/access_identity/` 里没有客户渠道的表；`CredentialVerifier` 同样没有生产实现，`ChannelCredentialProof` 被刻意做成只交出查找键的单方法接口，它的头注给的理由是凭据形态属 ADR-0072 Decision 二挡住的那一半。同包另两族已有生产实现：操作者族（ADR-0100）与集成客户端族（ADR-0149 决定三）——后者的集成客户端册（`IntegrationClientRegistry`）、客户端凭据令牌校验（`adapters/oidc` 的 `ClientVerifier`，含 RFC 8705 证书绑定）与铸造器（`IntegrationClientMinter`），正是本记录为首方族选的那一形凭据。（本段按接受修订时的现状改写；起草时两族都还没有落地。）

拦着这两件的原句是 ADR-0072 Decision 二：「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据（该租户渠道的现行流程）到位前不立」，其依据是 ADR-0055 Alternatives 对「为接入渠道建运行时登记表」的否决：「渠道配置的形状取决于真实渠道是 API、标准文件还是门户，替它拟表就是替租户拟 `PAR-INT-01` 的样子」。

**这套论证本仓已经拆过一次，拆的是它的另一半。** ADR-0100（2026-09-03，同样由用户在 IDP 队列里的一句质疑触发：「作为一个独立的软件产品……不可能都通过 cli」）裁定管理台运营操作者身份是产品自有的接入渠道族，信任锚、校验方式与操作者—租户授权模型三件由产品定义，不等 `PAR-INT-01`。它给当时的状态定的名是**范畴错误**：「把两族挂在同一个等待项上，后果是产品自己的后台要等客户的 API 契约」。它拆 ADR-0072 那句的方式是指出其前提只对一类渠道成立：「ADR-0072 Decision 二『形状取决于渠道类型、那是一份实例证据』的论证对客户渠道成立（API、标准文件、门户三形各异，替它拟表就是替租户拟样子），对操作者渠道不成立：操作者渠道只有一形，且这一形是产品自己选的。」

ADR-0100 把客户那一族原样留下，写明了理由是能力边界而不是论证不成立：「本记录能力边界只到登记册配置写面与目录查阅面；客户业务面的载荷摘要与准入范围装配是另一个问题，留给读过 PS CONTEXT 与 `UC-PS-001` 的裁决。」它留了一个空位。本记录填的就是这个空位，而且要说清的是：ADR-0100 那句「对客户渠道成立」**本身说宽了**。

**「三形各异」只对租户既有渠道的适配成立，对产品自己的渠道不成立。** ADR-0072 与 ADR-0055 说的「API、标准文件或门户」是 `PAR-INT-01` 最低证据列的原词——「API、标准文件或门户的**现行流程**、委托与资料修订业务标识、重试和冲突边界」。「现行流程」三个字暴露了这一列的视角：它预设客户**已经有**一条渠道在用，产品要做的是接上它。对这类渠道，三形确实各异，替它拟表确实是替租户拟样子，那半论证原样成立。但一个卖给物流企业的软件产品不能预设它的租户的客户各自已经有 API——它必须自己带一条：产品定义的 HTTPS API、产品签发的凭据、产品写的契约。这一形**只有一形，且这一形是产品自己选的**——ADR-0100 拆操作者那一半用的那句，逐字适用于首方客户渠道。没有首方渠道的产品演示不了、集成测试对不上自己的契约、卖出去之后第一件事是替每个租户各写一套接入，而那正是 ADR-0072 要挡的「分摊进业务上下文各自实现」在实施层的翻版。

`internal/accessidentity` 的 `doc.go` 里那句自证——「写成 `{公开键, 秘密}` 那种结构就已经替三种渠道拟了同一种形态——标准文件投递与门户登录都没有『秘密』这一栏」——只在「一张登记册必须装下三形」这个前提下成立。ADR-0100 的解法恰恰是不塞：操作者不复用 `SourceEnvelope`，另立 `OperatorEnvelope`，「两个类型让『拿操作者信封去铸客户委托』在编译期走不通」。首方渠道同理：它是自己一族，有自己的凭据形态与登记册结构；将来某个租户的文件渠道或门户转发真的来了，按 ADR-0072 原句为**那一族**立它自己的册与核验，装配点逐端点替换的缝（ADR-0055 Decision 一）本来就是为多族并存留的。

**`PAR-INT-01` 是试点上线约束，不是产品能力清单。** 它的「已确认约束」栏写的是「只启用客户当前主用的一种渠道」——那是在回答「给这个租户的这个货主开哪一条」，是实例半边的问题。[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)对机制半边的定义是「产品必须具备的能力——规则、判断、版本化、门禁与记录形态」，对实例半边的定义是「把某个租户的真实客户、线路、伙伴、参数和阶段决定填进去」。首方渠道的信任锚、凭据形态、登记册结构、契约与铸造，没有一件取决于任何租户，全部落在前者；「某租户的哪个客户账户绑了哪个 client、启用的是首方 API 还是既有渠道适配」才落在后者。[ADR-0019](./0019-product-ships-tenant-facing-operator-clients.md)（已被取代）当年读 `PAR-INT-01` 为「租户面向它自己客户的既有接入渠道，不是本仓要建的东西」，那句话是在回答产品是否自带**界面**，且已随该记录被取代；本记录把它对渠道的那层引申收回：**租户既有渠道的适配**不是本仓要预建的东西，**产品自己的渠道**是。

**仓内已有的候选租户渠道证据属既有渠道适配那一族；本记录不靠它成立，也不因它失效。** `.scratch/tenant-implementation-01/instance-register-draft.md` 的 `PAR-INT-01` 行与票 `syn-wall-door-audit/01` 的「重启范围」节记着：候选租户的客户生产委托现行渠道是 API（webhook createOrder、apikey 头认证、JSON 报文，证据 E-02），ADR-0072 Decision 二括注的重启门槛「该租户渠道的现行流程」已到，`PAR-INT-01` 四件最低证据只到两件半，缺的两件（资料修订业务标识、重试和冲突边界）列为该票 S0、归用户 → 客户；`doc.go` 「两条门槛不是一条」一节记的就是这件事。（接受修订：2026-09-24 起这一状况已变——ADR-0146 拆分后 `PAR-INT-01` 行注明「重试与请求冲突怎样判由产品机制固定」，票 `syn-wall-door-audit/01` 经 product-strategy-boundary/02 改为等本记录、不再等 S0。）那条渠道是租户自己的契约与租户自己的凭据形态（apikey 头），产品要做的是接上它——正是本记录说的「既有渠道适配」，它的册与核验照 ADR-0072 原句等 S0 到齐后按 API 型渠道形状立（该票 S1 原文），本记录一字不改这条路。本记录立的首方渠道与它**并列而不取代**：首租户的客户系统要么迁到首方 API，要么等 S1 的适配——首方先行，既有渠道适配是租户上线时的实施项（风险点 10 代决）；本记录只确保首方那条路不再挂在 S0 上。也因此，本记录**不把「租户数为零」当论据**：候选租户与证据都在，不在的只是 `PAR-INT-01` 缺的那两件，而它们决定的是既有渠道适配的判重列，不是首方渠道的任何一件。

**判据与 ADR-0100 同一条。** 一族身份的证据由谁出，看它的三件取决于谁：

- 信任锚：产品自有的发行方（ADR-0100 Decision 二已为操作者族选定产品自有 OIDC 发行方）。给客户系统签发机器凭据的是同一个发行方，只是走机器间授权而不是浏览器登录。谁签发是产品的部署形态参数，不是租户参数。
- 凭据形态：标准的机器间访问令牌——签名、发行方、受众、有效期。标准定的，不取决于任何租户。
- 渠道—客户账户绑定模型：一个渠道凭据绑定唯一（租户，客户账户）——`ChannelRegistration` 今天就是这个形状，且 ADR-0003 三级边界的入口断言已由 `seal` 落实；绑定按区间生效、可撤销。模型是产品的；**哪个租户的哪个客户账户绑了哪个 client** 才是实例半边。

三件都落在机制半边的定义里。ADR-0072 Decision 二对首方渠道因此不成立，理由与 ADR-0100 对操作者渠道给出的一字不差。

**契约形状不在本记录里重定，它已经有权威。** ADR-0022 定了客户端据以行动的契约：状态码只答「有没有形成答案」、业务判别进封闭的 `outcome` 词表、不新造 `Idempotency-Key`、`!found` 统一 `404`。首方 API 的响应形状就是它，本记录一字不改。`docs/api` 下那份 Go + chi + Scalar 方案稿是**参考**而不是权威——它的 public / internal 双契约、机器间授权、只读文档门户、对已发布基线的破坏性变更检查，因本记录而有了消费者；它与 ADR-0022 / ADR-0055 相冲的三处——传输层 `Idempotency-Key`、按语义分状态码（`409`/`422`）、Router 级认证中间件替处理器判准入——以已接受 ADR 为准；这三处即本记录对该方案稿的全部取舍依据，此外没有入仓的评估可引。

## Decision

**一、首方客户接入渠道是产品自有的接入渠道族，属机制半边；`PAR-INT-01` 的适用面收窄为「某租户的客户实际启用哪条渠道」与「租户既有渠道的适配所需证据」。** 首方渠道的信任锚、凭据形态、渠道登记册结构、来源信封铸造与机器可读契约由产品定义，现在就立；`PAR-INT-01` 的登记状态不改，其「已确认约束」（只启用客户当前主用的一种渠道）与最低证据列继续管两件事：给某个租户的某个客户账户开哪一条渠道，以及该客户既有渠道（标准文件、门户、非本产品契约的 API）的适配。接受本记录时，下列条款按 [adr/README](./README.md) 的部分停用机制加前向指针，**适用场景**各如下：

- ADR-0072 Decision 二「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据到位前不立」——ADR-0100 已把它收窄到客户接入渠道；本记录再收窄到**租户既有渠道的适配**。首方渠道的册与凭据形态由本记录 Decision 二定。Decision 一（能力归 `internal/accessidentity`、业务上下文只消费已铸造信封）与 Decision 三不变，本记录正是在 Decision 一划定的落点上立客户族的第一份生产实现。
- ADR-0085 Decision 二同日补记「两族各自要过 `PAR-INT-01` 的真证据门，不得互相顶替」——前半的操作者半已由 ADR-0100 停用，**客户半由本记录收窄为租户既有渠道的适配**（那一族仍要过 `PAR-INT-01`；首方族不过）；「不得互相顶替」后半保留并由本记录 Decision 三加强。
- ADR-0100 决定四末段「客户业务命令面（委托提交与撤回、作业事实登记、外部结果接收）照旧被两项未决拦着，本记录一个字不改它们」——作业事实登记与外部结果接收那一半已由 ADR-0149 停用；**余下的客户委托一半由本记录停用**，适用场景：首方族的客户面各口，ADR-0055 Decision 五两项改按本记录 Decision 八按口解。
- [ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md) 决定一把「渠道原始载荷 → 登记快照」的翻译「适用场景收窄为客户渠道载荷」——本记录**再收窄为客户既有渠道的载荷**；首方 API 的载荷形状是产品契约（[ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md)）。
- [ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md) 决定四「客户渠道的采信身份属 `PAR-INT-01` 接入契约，是实例半边，本记录不替它定」——适用场景**收窄为租户既有渠道的适配**；首方族的采信身份由本记录 Decision 二定。

ADR-0055 **不动**：其 Alternatives 对「为接入渠道建运行时登记表，读表判配置」的否决原文已带「（现在）」，且由 ADR-0072 Decision 二「维持」——收窄后者即收窄了它，ADR-0100 走的也是这条路；不为一条 Alternatives 多动一份已接受记录的 `Status` 行。ADR-0055 Decision 一至四原文有效，Decision 三对 `401` 的否决只在首方族随前件解除（见 Decision 四）。

**二、首方渠道的三件由产品定义。**

- **信任锚**是首方族自己的一组三件部署形态参数——发行方、JWKS 端点、受众，形状同集成客户端族的 `IDP_PARCEL_INTEGRATION_CLIENT_OIDC_*`（名字建议 `IDP_PARCEL_CUSTOMER_API_OIDC_ISSUER`、`IDP_PARCEL_CUSTOMER_API_OIDC_JWKS_URL`、`IDP_PARCEL_CUSTOMER_API_OIDC_AUDIENCE`，归实施票）。三件都未设即首方渠道整族未配置、缺省朝拦；只设一部分或值不成形，进程启动即拒。受众必须与操作者族、集成客户端族的都不同；三族是否落在同一个发行方实例上是部署决定（演示环境每族起一只 Dex 是先例）。客户系统以 OAuth 2.0 client credentials 取访问令牌。**`parcel-api` 自己校验令牌**（签名、`iss`、`aud`、有效期），不采信任何自报头部、不采信网关注入的身份头；核证书绑定所用的客户端证书取自本次 TLS 连接，同集成客户端族。
- **凭据形态**是 Bearer 访问令牌，**复用集成客户端族的凭据机制**（ADR-0149 决定三），校验顺序照 `IntegrationClientMinter.MintIntegrationClient`，不可倒：先由 `adapters/oidc` 的 `ClientVerifier` 校验令牌的签名、`iss`、`aud`、有效期——这一步**不读任何登记册**，不过即 Decision 四的令牌格；主体取（发行方，`sub`），不取 `client_id`，理由见 `IntegrationClientSubject` 的头注。再以已核验的主体为键查客户渠道册——查到即绑定成立，不另设一道「令牌主体与行所登客户端是否同一个」的核对；再核该行的证书绑定要求（RFC 8705），再核授予与准入范围（Decision 四、八），最后经 `seal` 铸信封。`mint.go` 里 `authenticate` 那条「先查册、后拿行上受控引用核验」的路径不给首方族用：它「顺序不能反」的前提是核验依赖行上的秘密，对秘密式凭据成立，对 JWT 不成立；若照它把签名校验放到查册之后，一个未经任何校验的调用方拿伪造令牌只换主体，在册与不在册就得两种答复，登记册成员就此可被枚举——正是 `ErrCredentialRejected` 注释与 Decision 四探针同答要防的事。那条路径留给依赖行上秘密的既有渠道适配族。令牌本体只在校验那一刻存在，不落库；库里只登绑定与凭据引用。凭据本体不进仓库与数据库是 AGENTS 「敏感实例外置」红线的落法，与 ADR-0100、ADR-0149 同款。
- **渠道登记册结构**是 `internal/accessidentity` 的**客户渠道册**，行形照集成客户端册（`IntegrationClientBinding`）：一行 =（客户端主体（发行方 + `sub`），租户，客户账户，凭据引用，是否要求证书绑定令牌，登记依据）。绑定一次登定、不可改指——同一个客户端先后代表两个客户账户，它交进来的委托就说不清是谁的。生效区间与撤销挂在**授予**上：授予按客户面能力显式登记、可撤销、带生效区间，同操作者册与集成客户端册；能力词表归产品策略，粒度随票 `first-party-api/01` 定。凭据引用指向受限存储里该客户端凭据的出处，供轮换与审计，核验不读它（`PAR-INT-09` 同款）。铸造时由行与推导口拼出 `ChannelRegistration` 交 `seal`——`seal` 仍是唯一产生非零 `SourceEnvelope` 的地方。列由产品定，不取决于任何租户；行（哪个租户的哪个客户账户绑了哪个客户端、授了什么）是租户取值，由参数登记册 `PAR-INT-13` 登记其证据状态，`PAR-INT-01` 行不改。客户渠道册是 `migrations/access_identity/` 的下一号迁移，与操作者册、集成客户端册同模块并列。

**三、首方客户族、操作者族与集成客户端族是装配点上互不相认的三族。** 首方族铸 `SourceEnvelope`（经 `SubmissionEnvelope` / `WithdrawalEnvelope` 等按命令分型），操作者族铸 `OperatorEnvelope`（ADR-0100 Decision 三），集成客户端族铸 `IntegrationClientEnvelope`（ADR-0149 决定三）；受众两两不同让一枚令牌在校验期只过得了一族，信封类型不同让「拿别族的信封去铸客户委托」在编译期走不通。首方族与集成客户端族**共用凭据机制**（客户端凭据令牌、`ClientVerifier`、答复代数），**不共用**册、受众与信封：两族按 ADR-0149「按提交者分族」的判据分开——外部系统交外部事实，绑一个来源身份，按事实类型授予；货主客户的系统交客户命令与查阅，绑（租户，客户账户），按客户面能力授予。ADR-0085 补记「不得互相顶替」的后半由此从纪律变成结构。不给操作者面开首方客户渠道，也不给客户业务面开操作者渠道——「操作者代客提交」是另一个用例，需要它自己的授权与来源标记，不在本记录（ADR-0100 Decision 五原句）。

**四、客户业务命令面与客户查阅面的真 Intake 就是首方渠道 Intake，在 `assembleBusinessEndpoints` 逐端点换。** 覆盖面是今天在生产装配下答未配置且信封为客户来源信封的那些行：命令面的委托提交、撤回、逐包裹取消、受控补充、资料修订、索赔提交（`/claims`），与查阅面的 `/customer-tracking-view`。答复代数按恢复动作分格（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)），各格不得合并，与操作者族、集成客户端族同形（ADR-0100 决定四、ADR-0149 决定四）：

- 发行方参数未设 → `403` `ACCESS_CHANNEL_NOT_CONFIGURED`（恢复动作：配部署参数的人，ADR-0055 原格不变）；
- 令牌缺失、过期或签名/声明校验不过，或该行要求证书绑定而令牌没绑在本次连接出示的证书上 → `401` `CUSTOMER_CLIENT_CREDENTIAL_REJECTED`（恢复动作：持凭据的客户系统去换令牌）。令牌校验那一半在查册之前答出（Decision 二）。ADR-0055 Decision 三否决 `401` 的前件是「此刻不存在任何凭证方案」，在首方族上这个前件自本记录接受起不成立，否决随前件解除，仅限此族；
- 令牌有效，但客户端不在客户渠道册，或此刻没有这一能力的生效授予（撤销、出区间同此）→ `403` `CUSTOMER_CLIENT_NOT_GRANTED`（恢复动作：租户运营去登记或恢复授予）。几种情形同答这一格、不说哪一半不对——探针同答约束（ADR-0055 Decision 四），理由同 `ErrIntegrationClientNotGranted`。这一格由首方族自己的「未授予」错误承载，`ErrAccessChannelNotConfigured` 只留给发行方参数未设；
- 生产写口上这笔写不落在准入范围内 → `403` `OUTSIDE_ADMISSION_SCOPE`（恢复动作：阶段治理去登记区间；哪些口判见 Decision 八）；
- 发行方公钥集、客户渠道册或准入范围读不动 → `503` `IDENTITY_DEPENDENCY_UNAVAILABLE`（恢复动作：运维去救依赖）。

错误码照两族先例命名（ADR-0055 Decision 二「命名状态，不命名参数」），`ACCESS_CHANNEL_NOT_CONFIGURED`、`OUTSIDE_ADMISSION_SCOPE`、`IDENTITY_DEPENDENCY_UNAVAILABLE` 与两族同码。各个 `403` 按 ADR-0022 不带 `outcome`，只能靠稳定错误码分开——首方 API 自称机器可读契约，客户系统要据它分流恢复动作。每换一口，该口的「未配置即拒」测试改写为分格测试；未换的口答复不变（逐口纪律沿 ADR-0091 Consequences）。

**五、首方 API 有一份产品拥有的机器可读契约，是客户面端点的单一权威。** 契约的响应形状照 ADR-0022；请求侧「接入请求标识」（UC-PS-001 输入语义契约来源信封行的原词）由契约定义为载荷里的一格并由首方族的 `RequestKeyDerivation` 推导为来源请求键——它是领域层已拥有的幂等机制的输入，**不是**传输层的 `Idempotency-Key`，ADR-0022 对后者的否决一字不动。它虽在载荷里，身份却是**信封元数据**（UC-PS-001 输入语义契约把它列在「来源信封」行，与 `occurredAt`/`receivedAt` 同列）：PS CONTEXT 对来源信封元数据「不进入规范化业务内容摘要」的处置对它同样成立，`CanonicalizeSubmissionPayload` 把它从规范化内容摘要里排除——摘要答的是「业务内容是否同一份」，混进一个逐请求变化的标识，两份内容相同的提交在摘要上就永远不相等，ADR-0022 所说「来源身份与载荷摘要」这套领域层幂等机制里靠摘要判的那一半（同内容换标识重发、版本间内容比对、审计）全部失真。契约怎么生产、URL 主版本、契约按族怎么拆、文档门户放哪个入口，由 [ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) 定。

**六、不做的，逐条写明以免被读宽。**

- 不替租户预建按某客户**自有**契约的适配——客户自有的 API、自有的文件格式、既有门户——那是租户上线时的实施，照 `PAR-INT-01` 的租户取值办，到时为那一族立它自己的册与核验，在装配点与首方族并列。产品自定形态的其他渠道（如产品定义的批量文件模板）按 ADR-0146 属产品策略，要做时按本记录同一判据另立记录，不等 `PAR-INT-01`。
- 不做绕过发行方的「开发用」API Key 或本地账号。隔离形态照 ADR-0150 决定三退场：首方渠道 Intake 换上 `/shipment-requests` 的同一笔，撤下该口的隔离提交放行（`isolatedWriteAdmittedCommandLines` 里那一行）；若它是两个隔离开关里最后换完的一口，同一笔删两个开关。演示租户以合成客户端与合成客户渠道行经首方渠道进出（ADR-0150 决定二）。
- 不改 ADR-0022 任何一条；不在本记录里决定契约正文、字段名或版本策略。
- 不做客户人员的浏览器登录（authorization code + PKCE）那一族：租户的货主人员要看什么走租户既有门户或本产品将来的客户面客户端，那是 ADR-0019 谱系的问题，不是接入渠道的问题；首发的首方族只有机器间凭据。

**七、受控 CLI 的定位不变，隔离形态是过渡。** 受控 CLI 仍是 ADR-0085 Decision 一的「受控批量口」；`SYN-` 写开关在首方渠道 Intake 落地前是提交口的演示写路径，落地的同一笔撤下（ADR-0150 决定三，见 Decision 六第二条）。

**八、ADR-0055 Decision 五两项照 ADR-0149 决定四按口解，本记录把它明文延到客户面各口**（接受修订新增；ADR-0149 决定一把客户委托排除在它的范围外，故须明文）。

- **载荷规范化摘要**：每个客户面写口由所属上下文定一版带形状版本的规范化形状，摘要进信封，作同身份重放与内容冲突的判据；提交口已有 `PSC-1`（ADR-0072 Decision 三已把摘要判为机制并解耦）。未定形状的口不开首方渠道。
- **准入范围**：提交口的生产归属由 `UC-PS-001` 编排自己判——它与准入范围桥同读 `parcel-api` 的权威串部署参数（[ADR-0180](./0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md) 决定三），铸造侧不再判第二遍；其余生产写口（撤回、逐包裹取消、受控补充、资料修订、索赔提交）在铸造侧按 ADR-0149 决定四第二条判，答 Decision 四的准入格，能力词照 ADR-0180 决定四末项随首方 Intake 实施票定；查阅口不判准入范围。区间内容（`PAR-GOV-03..07`）仍是租户取值。

## Consequences

- `internal/accessidentity` 得到首方族的生产实现：客户渠道册（PostgreSQL 实现，`migrations/access_identity/` 下一号迁移）与授予、复用 `adapters/oidc` 的 `ClientVerifier` 的首方铸造器（产出经 `seal` 的按命令分型信封）、首方族的 `RequestKeyDerivation`。`doc.go`「客户渠道那一半仍等 PAR-INT-01；操作者与集成客户端两族已立」一节与 `ChannelCredentialProof`、`CredentialVerifier`、`ChannelRegistry`、`RequestKeyDerivation` 的头注随实施改写为「租户既有渠道的适配那一半仍等 `PAR-INT-01`；首方渠道那一半已按 ADR-0139 立」；`doc.go`「两条门槛不是一条」一节保留——它记的是既有渠道适配那一族的门槛——只加一句「首方族不过这两条门槛」。`mint.go` 里 `authenticate` 注释「顺序不能反」随实施补上前提：它对核验依赖行上秘密的凭据形态成立，首方族不走这条路径（Decision 二）。实施票是 `.scratch/first-party-api/issues/00-first-party-customer-intake.md`。
- `cmd/parcel-api`：部署形态多首方族的三件参数（Decision 二），未设即首方族 `403` 未配置；客户业务命令面与客户查阅面在装配点逐端点换首方渠道 Intake，提交口换上的同一笔撤下隔离提交放行（Decision 六第二条）。演示种子增合成客户端与客户渠道行，演示环境的发行方增首方受众。
- 首方 API 的契约与文档成为产品交付物的一部分，位置、生产方式与门户由 ADR-0140 定；`docs/api` 方案稿在 `docs/README.md` 的「历史材料」登记不改，它的身份仍是参考。
- 参数登记册增 `PAR-INT-13`「首方客户端与客户渠道行」（租户取值，形状同 `PAR-INT-09`）。`PAR-INT-01` 行原文不动，其〔ADR-0146 拆分〕注已指向本记录，接受时把注里的「Proposed」改为已接受；操作者族的引用已在 `PAR-INT-08` 行，不另加。
- ADR-0072、ADR-0085、ADR-0100、ADR-0101、ADR-0106 的 `Status` 行与 Links 节按 [adr/README](./README.md) 的部分停用机制加前向指针，被收窄条款与适用场景如 Decision 一所述；ADR-0055 不动。
- 首租户实施节奏里客户渠道那一半的前件拆成两半：首方 API 那半不再以客户答复为前件；既有渠道适配那半照旧等 `PAR-INT-01` 的租户取值。票 `syn-wall-door-audit/01` 已由 product-strategy-boundary/02 改为等本记录，接受时其客户一半改由首方 Intake 实施票承接，候选租户 E-02 的 apikey 头适配记为租户上线实施项；`.scratch/tenant-implementation-01` 实施周 D-08 一行留待该租户实施时据此改指。
- ADR-0055 Decision 五两项对客户面各口按 Decision 八解；`PAR-GOV-03..07` 的区间内容仍是租户取值。

## Alternatives considered

- **维持「客户族照旧过 `PAR-INT-01` 门」，等首个租户的客户渠道证据齐了再立册。** 否决：与 ADR-0100 否决其对应项的理由同一条——把产品自己的 API 挂在客户既有 API 的证据上是范畴错误。候选租户的证据（E-02）到齐的也只是既有渠道适配那一族的开工门槛，S0 缺的两件归客户答复、没有期限；而下一个租户又从零件起。等待期间产品没有任何一条客户可走的生产路，演示与集成测试都只能靠 `SYN-` 隔离形态，而 ADR-0091 明写它「不是通往生产的捷径」。
- **首方渠道用产品自己签发的 API Key，不走发行方。** 否决：第二套凭据的签发、轮换、撤销与审计要从头做一遍，而产品发行方已按 ADR-0100 立为信任锚且原生支持机器间授权；两套信任锚让「内外令牌不互认」要靠两处各自守；ADR-0149 候选丙对集成客户端族否决共享 API Key 的理由同此。若产品发行方最终不提供机器间授权，复议此项而不是回到等待。**已知代价，记下不藏**：目标客群（货主的 ERP / 电商插件）现行实践多是静态 apikey 头——候选租户的既有渠道 E-02 就是这一形——client credentials 多出「先换令牌」一步，把集成负担推给最小的那些客户。这一格若要补，补的是**首方族的第二种凭据形态**（同一发行方签发与撤销、同一受众、同一册、同三格答复），不是回到既有渠道适配，也不是另一套信任锚；首发不补，触发条件与补法见风险点 9 的代决。
- **客户 API 复用操作者族的令牌与受众。** 否决：受众混用让一枚令牌同时过两族的校验，「不得互相顶替」退回纪律；ADR-0100 Decision 三分型 `OperatorEnvelope` 的理由在此一字不差地成立。
- **先写「开发用」采信头部的客户 Intake 跑通链路。** 否决：ADR-0055 Alternatives 原句——它从请求内容铸造来源信封，穿透 ADR-0003，且事后没有任何东西能把这些信封与真实认证结果区分开。
- **把首方渠道做成 `SYN-` 隔离写开关的一个取值。** 否决：代码路径不按 `SYN-` 分支、隔离形态是过渡并逐口退场（ADR-0150 决定一、三）；一条生产装配里不存在的路径不该长成生产准入（ADR-0100 同款否决）。
- **一张登记册装下首方 API、标准文件与门户三形。** 否决：这正是 `doc.go` 那句「替三种渠道拟了同一种形态」成立的前提；分族分型是 ADR-0100 已立的先例，也是 ADR-0055 装配点缝形本来的用法。
- **不立 ADR，直接在 `internal/accessidentity` 立表与核验。** 否决：ADR-0072 Decision 二是已接受记录里在生效的条款，ADR-0100 收窄它时走的是 ADR，本记录再收窄一次没有理由走别的路；且信任锚与凭据形态是难逆转的取舍，按 AGENTS 「改难逆转技术或产品取舍 → 新 ADR」。

## 越权风险点（归 owner 复核）

本记录能力边界之外、实施时必须先答的几件，逐条列出以免被当作已裁：

**复核处置（2026-10-19，Claude Code 会话的复核建议，随用户接受一并认可）**：逐条标在各条之下；标「留待」的仍归 owner 复核，其余 owner 不认可的走 supersede。

1. `PAR-GOV-03..07` 与准入范围装配对首方 Intake 的关系——`SubmissionIntake` 注释写明命令里的准入范围「由试点准入控制按 `PAR-GOV-03..07` 装配」，首方 Intake 落地时这一格从哪里来、登记册为空时编排答哪一格，归读过 `pilotgovernance` 的裁决。
   - **改（已作答）**：按 Decision 八——ADR-0149 决定四加 ADR-0180。
2. 契约的生产方式（手写规范生成处理器胶水，还是从既有处理器与 `outcome` 词表导出）、URL 主版本、public / internal 拆分、文档门户入口——另立记录；`docs/api` 方案稿与已接受 ADR 相冲的三处本记录 Context 末段已列，可借部分由另立记录自己重读原稿取，不引任何未入仓的评估。
   - **认可**：由 ADR-0140 作答。
3. 撤回、取消、补充、资料修订、索赔提交各命令的来源请求键推导——`Minter` 今天只为提交与撤回各开了铸造口（`MintSubmission` / `MintWithdrawal`），其余命令的信封是否各自分型、`RequestKeyDerivation` 是否按命令分方法，归 `accessidentity` 与 PS / VE owner。
   - **复核建议**：客户面各命令的信封按命令分型，编译期不可互铸；`RequestKeyDerivation` 按命令分方法，沿其头注「提交与撤回各有一个方法」的理由；摘要照 Decision 八逐口定形状；日后若开货主自助的受控关闭与重开口，照 [ADR-0183](./0183-operator-channel-addresses-controlled-closure-and-reopening-by-parcel-and-derives-the-shipper-from-the-accepted-request.md) 不加请求键、请求方写 `CUSTOMER-ACCOUNT/<标识>`。
4. `/customer-tracking-view`（`UC-VE-008` 查阅）与 `/claims`（`UC-VE-007` 索赔提交，命令面）是否与 PS 命令面同族——两者都以客户账户为作用域维，本记录按此归入首方族；VE owner 复核。
   - **认可**：[ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 决定七已把客户侧各口留在本记录名下，两者都以客户账户为作用域维。
5. 客户系统的令牌是否允许携带多个客户账户（一个 client 代多个货主账户，平台型客户常见）——本记录按「一凭据绑唯一（租户，客户账户）」立册，与 `ChannelRegistration` 今天的形状一致。这是**首方族自己**的绑定模型 1:1 还是 1:N 的问题：族由谁定义契约与凭据决定，不由绑定基数决定，平台型客户走首方 API 仍是首方族；1:N 要求请求里认领账户并按行上的授予集核对，形状同 ADR-0100 操作者族的「授予集」。归 `accessidentity` 与 PS owner，本记录不预判。
   - **复核建议**：首发 1:1（同 `seal` 与集成客户端「一客户端一来源身份」）；1:N 以「请求认领账户 + 行上账户集」后加，是契约的可加字段、不破坏，随票 `first-party-api/01`。
6. 发行方是否必须与操作者族同一个——本记录选同一个以复用信任锚；若部署形态要求客户 API 与运营台分发行方，改的是部署参数的个数，不改三格答复与绑定模型。
   - **改**：首方族自带三件参数，受众与两族都不同；同一发行方实例与否归部署（Decision 二）。
7. 首方行上的「受控引用」装什么——发行方侧的客户端标识、密钥指纹（`private_key_jwt` 形）或别的。`NewChannelRegistration` 要求非空，JWT 校验却不依赖行上任何秘密，这一格因此只剩绑定核对的用场；装什么决定绑定核对核的是什么，归 `accessidentity` owner。
   - **改（已作答）**：照 `IntegrationClientBinding` 与 `PAR-INT-09`——凭据引用只供轮换与审计，核验不读；绑定靠以已核验主体为键查册（Decision 二）。
8. 第二、三格的稳定错误码——Decision 四写明不可留白；命名归实施票，按 ADR-0055 Decision 二「命名状态，不命名参数」，且第一格 `ACCESS_CHANNEL_NOT_CONFIGURED` 原样不动。
   - **改（已作答）**：照两族先例命名，见 Decision 四。
9. 首方族是否补第二种凭据形态（静态密钥头）——Alternatives API Key 一条记下的市场代价；补的话仍是同一发行方、同一受众、同一册；是否补、何时补是产品决定，归 owner。
   - **留待（首发不补），仍归 owner 复核**：ADR-0149 候选丙已对同类机器客户端否决共享密钥；触发条件是首个租户的客户系统做不了 client credentials（候选租户 E-02 即此形）；要补也补在发行方一侧（密钥换令牌），`parcel-api` 仍只认 JWT，另立记录。
10. 首方 API 与候选租户既有渠道适配（票 `syn-wall-door-audit/01` S1）的先后——首租户的客户系统迁首方 API 还是等 S1，两条路是否都做、谁先，归 owner 的排期与客户关系决定；本记录只保证首方那条不再挂在 S0 上。
    - **复核建议**：首方先行（产品交付，ADR-0146）；既有渠道适配是租户上线时的实施项；票 `syn-wall-door-audit/01` 的客户一半改指首方 Intake 实施票。

## Links

- [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)：操作者族的同一判据与同一解法；本记录填其 Alternatives 末条留下的空位，并收回其 Context 中「对客户渠道成立」一句说宽的那一半
- [ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)：能力归 `internal/accessidentity` 的出处；本记录接受时把其 Decision 二的适用场景再收窄为租户既有渠道的适配
- [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：未配置格、替换缝、Decision 二的错误码纪律、Decision 三对 `401` 的否决前件、Decision 四的探针同答；本记录不停用其任何条款，其 Alternatives 对运行时渠道登记表的否决经 ADR-0072 Decision 二的收窄而收窄，Decision 五两项对客户面各口按本记录 Decision 八解
- [ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)：Decision 二补记「两族各自要过 `PAR-INT-01` 的真证据门，不得互相顶替」；本记录接受时把其客户半收窄为租户既有渠道的适配，保留并结构化「不得互相顶替」
- [ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md)：`SYN-` 隔离形态与逐口放行纪律；其决定六已由 ADR-0150 部分停用，本记录不碰其任何一条
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：首方 API 四篇属产品策略；「既有渠道适配」按其三分收窄、本记录「机制半边」一词的读法
- [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)：集成客户端族（本记录复用其凭据机制与校验顺序）、「按提交者分族」判据、决定四按口解 ADR-0055 Decision 五两项、候选丙对共享 API Key 的否决
- [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)：合成租户经真渠道进出、隔离形态逐口退场——本记录 Decision 六第二条与 Decision 七的依据
- [ADR-0151](./0151-unassigned-command-faces-get-their-families.md)：客户侧各口归本记录
- [ADR-0180](./0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md)：准入范围对照与权威串部署参数——本记录 Decision 八的依据
- [ADR-0183](./0183-operator-channel-addresses-controlled-closure-and-reopening-by-parcel-and-derives-the-shipper-from-the-accepted-request.md)：客户渠道日后接受控关闭与重开时请求方引用的写法
- [ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md)、[ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md)：本记录部分停用的两句，见 Decision 一
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：首方 API 的契约形状；本记录不改一字，`Idempotency-Key` 的否决原样有效（`429` 一格的部分停用归 ADR-0140）
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：各格按恢复动作分格的判据
- [ADR-0003](./0003-group-tenant-legal-entity-customer-account.md)：一凭据绑唯一（租户，客户账户）所依的三级边界；`seal` 是它在铸造这一步的落法
- [ADR-0019](./0019-product-ships-tenant-facing-operator-clients.md)（已被取代）：「租户面向它自己客户的既有接入渠道，不是本仓要建的东西」一句的出处；本记录收回其对渠道的引申，不改其对界面的结论
- [参数登记册](../product/PILOT-PARAMETER-REGISTER.md)：`PAR-INT-01` 原文出处；本记录不改其状态，只收窄适用面；`PAR-INT-09` 是客户渠道册行形的先例，`PAR-INT-13` 登首方族的租户实例行
- [开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)：机制半边与实例半边定义的唯一权威
- [parcel-shipment CONTEXT](../domain/parcel-shipment/CONTEXT.md)：「共享身份认证与授权技术能力拥有凭据验证、通用授权策略和授权作用域签发；`parcel-shipment` 只消费已授权作用域」——凭据验证归 `accessidentity` 而不归业务上下文的硬句
- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：输入语义契约里来源信封「不可由客户声明、来自接入适配器与接入来源」的出处，「接入请求标识」一词的出处
- [`docs/api` Go + chi + Scalar 方案稿](../api/Go_Chi_Scalar_API_Documentation_Final_V1.0.md)：参考输入；其与 ADR-0022 / ADR-0055 相冲的三处见本记录 Context 末段，此外无入仓评估可引
- `.scratch/syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md`：「重启范围」节——ADR-0072 重启门槛已到、`PAR-INT-01` 只到两件半、S0 归客户、S1 按 API 型渠道形状立册；本记录把 S1 定位为既有渠道适配族的册，接受时加改指注
- `.scratch/tenant-implementation-01/instance-register-draft.md` 的 `PAR-INT-01` 行与 `implementation-week-decisions-and-customer-pack.md` 的 D-08 行：候选租户既有渠道（API、apikey 头、证据 E-02）与「accessidentity S1–S3 按 C0 答复推进」的出处；证据本体按指纹索引制外置，本记录不复录
- `internal/accessidentity`：`SourceEnvelope`、`ChannelRegistration`、`ChannelRegistry`、`CredentialVerifier`、`ChannelCredentialProof`、`Minter`、`seal` 的现状形状，`mint.go` `authenticate` 注释「顺序不能反」的前提；`IntegrationClientBinding`、`IntegrationClientMinter` 与 `adapters/oidc` 的 `ClientVerifier` 是本记录复用的凭据机制；本记录 Decision 二、三在其上立客户族那一半
- `cmd/parcel-api` 的 `assembleBusinessEndpoints`：客户业务面各行（`/shipment-requests` 经隔离提交开关换值除外）今天挂字面量 `UnconfiguredIntake{}` 的出处，本记录 Decision 四的替换点
- `internal/visibilityexception/adapters/http/receive_claim.go`：`/claims` 是 `UC-VE-007` 索赔提交的命令口——本记录把它归入命令面的出处
- 来源：IDP 队列通道 5 的质疑与「起草」指令（2026-09-16）；通道 1 的复核与用户「按建议处置」指令（同日，草案修订）；Claude Code 会话复核（2026-10-19），用户在会话里答「接受」，修订后接受
