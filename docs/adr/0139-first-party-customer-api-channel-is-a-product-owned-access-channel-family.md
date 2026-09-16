# ADR-0139：产品自有的客户接入渠道（首方 API）是产品自有的接入渠道族，属机制半边——信任锚、凭据形态、渠道登记册结构与来源信封铸造由产品定义，不等 `PAR-INT-01`；`PAR-INT-01` 收窄为「某租户的客户实际启用哪条渠道」与「租户既有渠道的适配」；ADR-0072 Decision 二余下那一半与 ADR-0085 补记「两族各自要过」的客户半据此收窄

Status: Proposed（**草案**，2026-09-16；**草案修订**同日，用户经 IDP 队列通道 1 令「按建议处置」，据通道 1 的复核修订——改写 Decision 二凭据形态一条的校验顺序（签名与声明校验前置于 `Minter`、不读册，`Minter` 内只核绑定；原句「既有顺序不变」与 Decision 四的三格互斥，会让登记册成员被未认证调用方枚举），Context 补候选租户既有 API 渠道证据（E-02）的归族，不再停用 ADR-0055 Alternatives 那一条、ADR-0085 客户半改为收窄，`/claims` 改归命令面，Decision 五补「接入请求标识」不进摘要，Alternatives 补 API Key 一条的市场取舍，风险点去掉预判并补齐；修订时另读过 `.scratch/syn-wall-door-audit/issues/01` 的「重启范围」节、`.scratch/tenant-implementation-01/instance-register-draft.md` 的 `PAR-INT-01` 行与 `implementation-week-decisions-and-customer-pack.md` 的 D-08 行、`internal/parcelshipment/adapters/http/cancel_parcel.go` 与 `internal/visibilityexception/adapters/http/receive_claim.go` 的端点构造函数注释。用户经 IDP 队列通道 5 先质疑「我们这是一个我们公司的软件产品，有没有客户都需要 internal/public api 的吧，原来的规范老是等真实的客户，不适合一个软件公司开发产品的吧」，再令「起草」；本记录据此起草，**接受与否归用户**，接受前不是依据、不停用任何记录的任何条款。裁决能力边界：读过 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)、[ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)、[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、[ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md) 全文，[ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md) 的 Status 行与 Decision 二同日补记，[ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) Decision 一、三、四、六，[ADR-0018](./0018-product-clients-share-release-boundary-under-apps.md) / [ADR-0019](./0019-product-ships-tenant-facing-operator-clients.md) 谱系中关于 `PAR-INT-01` 的句子，`internal/accessidentity` 全部生产文件，`cmd/parcel-api` 的 `assembleBusinessEndpoints` 与 `internal/platform/httpapi` 路由，[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)的 `PAR-INT-01` 至 `PAR-INT-07` 行，[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)对机制半边的定义句，[parcel-shipment CONTEXT](../domain/parcel-shipment/CONTEXT.md) 邻接上下文里关于共享身份认证与授权技术能力的那句，[UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md) 输入语义契约的来源信封行，以及 [`docs/api` 的 Go + chi + Scalar 方案稿](../api/Go_Chi_Scalar_API_Documentation_Final_V1.0.md)全文；未重读 [ADR-0003](./0003-group-tenant-legal-entity-customer-account.md) 全文、`pilotgovernance` 上下文与 `PAR-GOV-03..07` 相关记录及代码、UC-PS-001 其余各节——本记录因此只裁**首方客户渠道的归属与准入结构**，不裁契约正文、不裁 `PAR-GOV-03..07` 与 ADR-0055 Decision 五两项对客户业务面还拦不拦、不裁任何登记册的领域不变量）
Date: 2026-09-16

## Context

**客户业务面在生产装配下今天一口也进不来，而拦着它的不是缺客户，是一句把产品能力归错半边的话。** `cmd/parcel-api` 的 `assembleBusinessEndpoints` 里，客户业务命令面（委托提交、撤回、逐包裹取消、受控补充、资料修订，以及 `UC-VE-007` 索赔提交 `/claims`）与客户查阅面（`/customer-tracking-view`）除 `/shipment-requests` 一行经 ADR-0091 的隔离提交开关可换值外，其余各行都挂着字面量 `UnconfiguredIntake{}`；生产装配没有任何一行开关，全部对一切请求如实答 `403` `ACCESS_CHANNEL_NOT_CONFIGURED`。`internal/accessidentity` 里铸造那一段已经成型——`Minter` 从 `ChannelRegistry` 取一行 `ChannelRegistration`、经 `CredentialVerifier` 核验、`seal` 出四要素 `SourceEnvelope`，身份三要素结构上取不到请求体——但该包 `doc.go` 自己写着「本轮既没有登记册的表，也没有凭据形态」：`ChannelRegistry` 只有接口没有生产实现，`CredentialVerifier` 同样，`ChannelCredentialProof` 被刻意做成只交出查找键的单方法接口，因为「凭据形态属 ADR-0072 Decision 二挡住的那一半」。

拦着这两件的原句是 ADR-0072 Decision 二：「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据（该租户渠道的现行流程）到位前不立」，其依据是 ADR-0055 Alternatives 对「为接入渠道建运行时登记表」的否决：「渠道配置的形状取决于真实渠道是 API、标准文件还是门户，替它拟表就是替租户拟 `PAR-INT-01` 的样子」。

**这套论证本仓已经拆过一次，拆的是它的另一半。** ADR-0100（2026-09-03，同样由用户在 IDP 队列里的一句质疑触发：「作为一个独立的软件产品……不可能都通过 cli」）裁定管理台运营操作者身份是产品自有的接入渠道族，信任锚、校验方式与操作者—租户授权模型三件由产品定义，不等 `PAR-INT-01`。它给当时的状态定的名是**范畴错误**：「把两族挂在同一个等待项上，后果是产品自己的后台要等客户的 API 契约」。它拆 ADR-0072 那句的方式是指出其前提只对一类渠道成立：「ADR-0072 Decision 二『形状取决于渠道类型、那是一份实例证据』的论证对客户渠道成立（API、标准文件、门户三形各异，替它拟表就是替租户拟样子），对操作者渠道不成立：操作者渠道只有一形，且这一形是产品自己选的。」

ADR-0100 把客户那一族原样留下，写明了理由是能力边界而不是论证不成立：「本记录能力边界只到登记册配置写面与目录查阅面；客户业务面的载荷摘要与准入范围装配是另一个问题，留给读过 PS CONTEXT 与 `UC-PS-001` 的裁决。」它留了一个空位。本记录填的就是这个空位，而且要说清的是：ADR-0100 那句「对客户渠道成立」**本身说宽了**。

**「三形各异」只对租户既有渠道的适配成立，对产品自己的渠道不成立。** ADR-0072 与 ADR-0055 说的「API、标准文件或门户」是 `PAR-INT-01` 最低证据列的原词——「API、标准文件或门户的**现行流程**、委托与资料修订业务标识、重试和冲突边界」。「现行流程」三个字暴露了这一列的视角：它预设客户**已经有**一条渠道在用，产品要做的是接上它。对这类渠道，三形确实各异，替它拟表确实是替租户拟样子，那半论证原样成立。但一个卖给物流企业的软件产品不能预设它的租户的客户各自已经有 API——它必须自己带一条：产品定义的 HTTPS API、产品签发的凭据、产品写的契约。这一形**只有一形，且这一形是产品自己选的**——ADR-0100 拆操作者那一半用的那句，逐字适用于首方客户渠道。没有首方渠道的产品演示不了、集成测试对不上自己的契约、卖出去之后第一件事是替每个租户各写一套接入，而那正是 ADR-0072 要挡的「分摊进业务上下文各自实现」在实施层的翻版。

`internal/accessidentity` 的 `doc.go` 里那句自证——「写成 `{公开键, 秘密}` 那种结构就已经替三种渠道拟了同一种形态——标准文件投递与门户登录都没有『秘密』这一栏」——只在「一张登记册必须装下三形」这个前提下成立。ADR-0100 的解法恰恰是不塞：操作者不复用 `SourceEnvelope`，另立 `OperatorEnvelope`，「两个类型让『拿操作者信封去铸客户委托』在编译期走不通」。首方渠道同理：它是自己一族，有自己的凭据形态与登记册结构；将来某个租户的文件渠道或门户转发真的来了，按 ADR-0072 原句为**那一族**立它自己的册与核验，装配点逐端点替换的缝（ADR-0055 Decision 一）本来就是为多族并存留的。

**`PAR-INT-01` 是试点上线约束，不是产品能力清单。** 它的「已确认约束」栏写的是「只启用客户当前主用的一种渠道」——那是在回答「给这个租户的这个货主开哪一条」，是实例半边的问题。[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)对机制半边的定义是「产品必须具备的能力——规则、判断、版本化、门禁与记录形态」，对实例半边的定义是「把某个租户的真实客户、线路、伙伴、参数和阶段决定填进去」。首方渠道的信任锚、凭据形态、登记册结构、契约与铸造，没有一件取决于任何租户，全部落在前者；「某租户的哪个客户账户绑了哪个 client、启用的是首方 API 还是既有渠道适配」才落在后者。[ADR-0019](./0019-product-ships-tenant-facing-operator-clients.md)（已被取代）当年读 `PAR-INT-01` 为「租户面向它自己客户的既有接入渠道，不是本仓要建的东西」，那句话是在回答产品是否自带**界面**，且已随该记录被取代；本记录把它对渠道的那层引申收回：**租户既有渠道的适配**不是本仓要预建的东西，**产品自己的渠道**是。

**仓内已有的候选租户渠道证据属既有渠道适配那一族；本记录不靠它成立，也不因它失效。** `.scratch/tenant-implementation-01/instance-register-draft.md` 的 `PAR-INT-01` 行与票 `syn-wall-door-audit/01` 的「重启范围」节记着：候选租户的客户生产委托现行渠道是 API（webhook createOrder、apikey 头认证、JSON 报文，证据 E-02），ADR-0072 Decision 二括注的重启门槛「该租户渠道的现行流程」已到，`PAR-INT-01` 四件最低证据只到两件半，缺的两件（资料修订业务标识、重试和冲突边界）列为该票 S0、归用户 → 客户；`doc.go` 「两条门槛不是一条」一节记的就是这件事。那条渠道是租户自己的契约与租户自己的凭据形态（apikey 头），产品要做的是接上它——正是本记录说的「既有渠道适配」，它的册与核验照 ADR-0072 原句等 S0 到齐后按 API 型渠道形状立（该票 S1 原文），本记录一字不改这条路。本记录立的首方渠道与它**并列而不取代**：首租户的客户系统要么迁到首方 API，要么等 S1 的适配——两条路谁先、是否都做，是产品排期与客户关系的决定，归 owner（见风险点）；本记录只确保首方那条路不再挂在 S0 上。也因此，本记录**不把「租户数为零」当论据**：候选租户与证据都在，不在的只是 `PAR-INT-01` 缺的那两件，而它们决定的是既有渠道适配的判重列，不是首方渠道的任何一件。

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

ADR-0055 **不动**：其 Alternatives 对「为接入渠道建运行时登记表，读表判配置」的否决原文已带「（现在）」，且由 ADR-0072 Decision 二「维持」——收窄后者即收窄了它，ADR-0100 走的也是这条路；不为一条 Alternatives 多动一份已接受记录的 `Status` 行。ADR-0055 Decision 一至四原文有效，Decision 三对 `401` 的否决只在首方族随前件解除（见 Decision 四）。

**二、首方渠道的三件由产品定义。**

- **信任锚**是产品自有的发行方，与 ADR-0100 Decision 二为操作者族选定的是同一个发行方；客户系统以机器间授权（OAuth2 client credentials）取访问令牌，令牌的受众（`aud`）是客户 API 自己的一格，与操作者族的受众不同。发行方地址、JWKS 端点与客户 API 受众是 `parcel-api` 的部署形态参数，未设即首方渠道整族未配置、缺省朝拦。**`parcel-api` 自己校验令牌**（签名、`iss`、`aud`、有效期），不采信任何自报头部、不采信网关注入的身份头。
- **凭据形态**是 Bearer 访问令牌，校验**分两步、顺序不可倒**。第一步在 `Minter` 之外：令牌的签名、`iss`、`aud`、有效期先按 JWKS 校验，不过即 Decision 四的第二格，这一步**不读任何登记册**；只有校验过的令牌才能构造出 `ChannelCredentialProof`，`ClaimedChannel()` 交出的查找键取已核验令牌里的客户端标识。第二步才进 `Minter.authenticate`：先按键定位登记行，再由 `CredentialVerifier` 在首方族上的生产实现拿行上的受控引用核对**绑定**——令牌主体与行所登的客户端是否同一个。`mint.go` 「顺序不能反」一句的前提是「核验依赖行上的秘密」，对秘密式凭据成立，对 JWT 不成立：签名校验只依赖 JWKS，不依赖任何一行。若照原顺序把签名校验放进第二步，一个未经任何校验的调用方拿伪造令牌只换 `client_id`，在册的得第二格、不在册的得第三格，登记册成员就此可被枚举——正是 `ErrCredentialRejected` 注释与 Decision 四末句都说要防的事。令牌本体只在校验那一刻存在，不落库；库里只登绑定。凭据本体不进仓库与数据库是 AGENTS 「敏感实例外置」红线的落法，与 ADR-0100 同款。
- **渠道登记册结构**是 `internal/accessidentity` 的**客户渠道册**：一行即一个 `ChannelRegistration`——（租户，客户账户，来源，受控引用，来源请求键推导口的选择）——外加生效区间与撤销。首方行上的「受控引用」装什么（发行方侧的客户端标识、密钥指纹或别的）本记录不定，见风险点；它必须非空是 `NewChannelRegistration` 已有的门。列由产品的绑定模型决定，不取决于任何租户；行（哪个租户的哪个客户账户绑了哪个 client）属实例半边，由参数登记册增一行登记其证据状态，`PAR-INT-01` 行不改。`ChannelRegistry` 与 `CredentialVerifier` 由此各得第一份生产实现，`migrations/access_identity/` 随之成立（ADR-0100 的操作者册若先落地则同一模块内并列，谁先到谁建模块）。

**三、首方渠道与操作者渠道是装配点上互不相认的两族。** 客户族铸 `SourceEnvelope`（经 `SubmissionEnvelope` / `WithdrawalEnvelope` 分型），操作者族铸 `OperatorEnvelope`（ADR-0100 Decision 三）；受众不同让一枚令牌在校验期只过得了一族，信封类型不同让「拿操作者信封去铸客户委托」在编译期走不通。ADR-0085 补记「不得互相顶替」的后半由此从纪律变成结构。不给操作者面开首方客户渠道，也不给客户业务面开操作者渠道——「操作者代客提交」是另一个用例，需要它自己的授权与来源标记，不在本记录（ADR-0100 Decision 五原句）。

**四、客户业务命令面与客户查阅面的真 Intake 就是首方渠道 Intake，在 `assembleBusinessEndpoints` 逐端点换。** 覆盖面是今天在生产装配下答未配置且信封为客户来源信封的那些行：命令面的委托提交、撤回、逐包裹取消、受控补充、资料修订、索赔提交（`/claims`），与查阅面的 `/customer-tracking-view`。答复代数按恢复动作分格（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)），三格不得合并，与 ADR-0100 Decision 四同形：

- 发行方或客户 API 受众未配置 → `403` `ACCESS_CHANNEL_NOT_CONFIGURED`（恢复动作：配部署参数的人，ADR-0055 原格不变）；
- 令牌缺失、过期或签名/声明校验不过 → `401`（恢复动作：持凭据的客户系统去换令牌）。这一格在 Decision 二的第一步就答出，不读册。ADR-0055 Decision 三否决 `401` 的前件是「此刻不存在任何凭证方案」，在首方族上这个前件自本记录接受起不成立，否决随前件解除，仅限此族；
- 令牌有效但客户端不在客户渠道册、已撤销、不在生效区间，或行上绑定核对不过 → `403`（恢复动作：租户运营去登记或恢复该客户账户的渠道行）。与 `accessidentity` 两格的对应写死：`ChannelRegistry` 对已撤销与出区间的行一律答 `found=false`，`Minter` 交回 `ErrAccessChannelNotConfigured`，映到这一格；第二步绑定核对不过交回 `ErrCredentialRejected`，**也**映到这一格而不是第二格——能走到第二步的令牌已经有效，缺的是绑定，恢复动作同样在租户运营那一侧。探针同答约束（ADR-0055 Decision 四）照旧：三种不在册与绑定不符在答复上不分，不披露「哪一半不对」。`ErrAccessChannelNotConfigured` 的名字自此也表达「已撤销」，是刻意合格不是漂移；要不要改名归实施票。

第一格与第三格同为 `403`，按 ADR-0022 4xx 不带 `outcome`，两格只能靠稳定错误码分开；第二、三格的错误码本记录不命名，归实施票按 ADR-0055 Decision 二「错误码命名状态，不命名参数」定，**不可留白**——首方 API 自称机器可读契约，客户系统要据它分流恢复动作。每换一口，该口的「未配置即拒」测试改写为三格测试；未换的口答复不变（逐口纪律沿 ADR-0091 Consequences）。

**五、首方 API 有一份产品拥有的机器可读契约，是客户面端点的单一权威。** 契约的响应形状照 ADR-0022；请求侧「接入请求标识」（UC-PS-001 输入语义契约来源信封行的原词）由契约定义为载荷里的一格并由首方族的 `RequestKeyDerivation` 推导为来源请求键——它是领域层已拥有的幂等机制的输入，**不是**传输层的 `Idempotency-Key`，ADR-0022 对后者的否决一字不动。它虽在载荷里，身份却是**信封元数据**（UC-PS-001 把它列在来源信封行，与 `occurredAt`/`receivedAt` 同列）：PS CONTEXT「信封元数据不进摘要」对它成立，`CanonicalizeSubmissionPayload` 把它从规范化内容摘要里排除——摘要答的是「业务内容是否同一份」，混进一个逐请求变化的标识，两份内容相同的提交在摘要上就永远不相等，ADR-0022 所说「来源身份与载荷摘要」这套领域层幂等机制里靠摘要判的那一半（同内容换标识重发、版本间内容比对、审计）全部失真。契约怎么生产（手写规范再生成代码，还是从既有处理器导出）、URL 主版本、public 与 internal 契约要不要拆、文档门户放哪个入口，**本记录不定**——它们是难逆转的技术取舍，另立记录，`docs/api` 方案稿作参考输入。

**六、不做的，逐条写明以免被读宽。**

- 不替租户预建任何既有渠道的适配——标准文件投递、门户转发、按客户自有契约翻译的 API 适配，全部照 ADR-0072 原句等 `PAR-INT-01` 证据，到时为那一族立它自己的册与核验，在装配点与首方族并列。
- 不做绕过发行方的「开发用」API Key 或本地账号；隔离形态（ADR-0091）继续靠 `SYN-` 写开关服务演示，它与首方渠道是装配点上的两行，互不替代、不合并；ADR-0091 Decision 六「隔离形态不因真渠道出现而自动退场」照旧。
- 不裁 ADR-0055 Decision 五两项（载荷规范化摘要、准入范围装配 `PAR-GOV-03..07`）对客户业务面还拦不拦。它们是「首方 Intake 落地之后编排如何作答」的问题，不是「渠道族归谁」的问题；本记录只裁后者。ADR-0072 Decision 三已把摘要那一项判为机制半边并解耦，本记录不重述。
- 不改 ADR-0022 任何一条；不在本记录里决定契约正文、字段名或版本策略。
- 不做客户人员的浏览器登录（authorization code + PKCE）那一族：租户的货主人员要看什么走租户既有门户或本产品将来的客户面客户端，那是 ADR-0019 谱系的问题，不是接入渠道的问题；首发的首方族只有机器间凭据。

**七、受控 CLI 与隔离形态的定位不变。** `SYN-` 写开关仍是唯一的演示写路径直到首方渠道 Intake 落地；落地后它也不退场（ADR-0091 Decision 六）。

## Consequences

- `internal/accessidentity` 得到客户族的第一份生产实现：客户渠道册（`ChannelRegistry` 的 PostgreSQL 实现、`migrations/access_identity/` 首个或并列模块）、机器间令牌的 `CredentialVerifier` 与 `ChannelCredentialProof` 实现、首方族的 `RequestKeyDerivation`。`doc.go` 中「本轮既没有登记册的表，也没有凭据形态」一段与 `credential.go` 里「凭据形态属 ADR-0072 Decision 二挡住的那一半」诸句随实施改写为「租户既有渠道的适配那一半仍等 `PAR-INT-01`；首方渠道那一半已按 ADR-0139 立」；`doc.go` 「两条门槛不是一条」一节保留——它记的是既有渠道适配那一族的门槛——只加一句「首方族不过这两条门槛」。`mint.go` 里 `authenticate` 注释「顺序不能反」一句随实施补上前提：它对核验依赖行上秘密的凭据形态成立；首方族的签名与声明校验在 `Minter` 之外先做，`Minter` 内只核绑定（Decision 二）。这些注释都等实施笔改，草案期一字不动——接受前本记录不是依据。
- `cmd/parcel-api`：部署形态多客户 API 受众参数（发行方与 JWKS 与 ADR-0100 共用），未设即首方族 `403` 未配置；客户业务命令面与客户查阅面在装配点逐端点换首方渠道 Intake；隔离读放行表与 `SYN-` 写开关零改动。
- 首方 API 的契约与文档成为产品交付物的一部分，位置、生产方式与门户由另立记录定；`docs/api` 方案稿在 `docs/README.md` 的「历史材料」登记不改，它的身份仍是参考。
- 参数登记册增一行「客户首方 API 渠道绑定」（实例半边：某租户的客户账户 × 客户端 × 生效区间），`PAR-INT-01` 行原文不动，其适用面由本记录 Decision 一收窄；**接受时在 `PAR-INT-01` 行的备注或版本历史加一条指向本记录的引用**——只读登记册的人才看得到适用面已收窄，这是红线「单一权威：一决策一处定义，其余只引用」里「引用」那一半，ADR-0100 收窄时漏了这一步，本记录补上，操作者族那一条引用一并加。
- ADR-0072 与 ADR-0085 的 `Status` 行与 Links 节在**本记录被接受时**按 [adr/README](./README.md) 的部分停用机制加前向指针，被收窄条款与适用场景如 Decision 一所述；ADR-0055 不动；草案期间三份记录一字不动。
- 首租户实施节奏里客户渠道那一半的前件拆成两半：首方 API 那半不再以客户答复为前件；既有渠道适配那半照旧等 `PAR-INT-01`。票 `syn-wall-door-audit/01` 的 S1（按 API 型渠道形状立册）与 `.scratch/tenant-implementation-01` 实施周 D-08「accessidentity S1–S3 按 C0 答复推进」在本记录接受时各加一条改指注：S1 那张表是既有渠道适配族的册，`ChannelRegistry` 与 `CredentialVerifier` 的第一份生产实现改由首方族先立、两族同模块并列；S0 的归属（用户 → 客户）与两票其余各步不改。
- 客户业务面的另两项等待（`PAR-GOV-03..07` 与 ADR-0055 Decision 五）一字不变，由读过 `pilotgovernance` 与 UC-PS-001 全文的裁决另行处理。

## Alternatives considered

- **维持「客户族照旧过 `PAR-INT-01` 门」，等首个租户的客户渠道证据齐了再立册。** 否决：与 ADR-0100 否决其对应项的理由同一条——把产品自己的 API 挂在客户既有 API 的证据上是范畴错误。候选租户的证据（E-02）到齐的也只是既有渠道适配那一族的开工门槛，S0 缺的两件归客户答复、没有期限；而下一个租户又从零件起。等待期间产品没有任何一条客户可走的生产路，演示与集成测试都只能靠 `SYN-` 隔离形态，而 ADR-0091 明写它「不是通往生产的捷径」。
- **首方渠道用产品自己签发的 API Key，不走发行方。** 否决：第二套凭据的签发、轮换、撤销与审计要从头做一遍，而产品发行方已按 ADR-0100 立为信任锚且原生支持机器间授权；两套信任锚让「内外令牌不互认」要靠两处各自守。若产品发行方最终不提供机器间授权，复议此项而不是回到等待。**已知代价，记下不藏**：目标客群（货主的 ERP / 电商插件）现行实践多是静态 apikey 头——候选租户的既有渠道 E-02 就是这一形——client credentials 多出「先换令牌」一步，把集成负担推给最小的那些客户。这一格若要补，补的是**首方族的第二种凭据形态**（同一发行方签发与撤销、同一受众、同一册、同三格答复），不是回到既有渠道适配，也不是另一套信任锚；补不补是产品决定，归 owner（见风险点），本记录不预裁。
- **客户 API 复用操作者族的令牌与受众。** 否决：受众混用让一枚令牌同时过两族的校验，「不得互相顶替」退回纪律；ADR-0100 Decision 三分型 `OperatorEnvelope` 的理由在此一字不差地成立。
- **先写「开发用」采信头部的客户 Intake 跑通链路。** 否决：ADR-0055 Alternatives 原句——它从请求内容铸造来源信封，穿透 ADR-0003，且事后没有任何东西能把这些信封与真实认证结果区分开。
- **把首方渠道做成 `SYN-` 隔离写开关的一个取值。** 否决：`SYN-` 前缀门禁把真实租户结构上挡在开关之外是 ADR-0091 Decision 六的设计；一条生产装配里不存在的路径不该长成生产准入（ADR-0100 同款否决）。
- **一张登记册装下首方 API、标准文件与门户三形。** 否决：这正是 `doc.go` 那句「替三种渠道拟了同一种形态」成立的前提；分族分型是 ADR-0100 已立的先例，也是 ADR-0055 装配点缝形本来的用法。
- **不立 ADR，直接在 `internal/accessidentity` 立表与核验。** 否决：ADR-0072 Decision 二是已接受记录里在生效的条款，ADR-0100 收窄它时走的是 ADR，本记录再收窄一次没有理由走别的路；且信任锚与凭据形态是难逆转的取舍，按 AGENTS 「改难逆转技术或产品取舍 → 新 ADR」。

## 越权风险点（归 owner 复核）

本记录能力边界之外、实施时必须先答的几件，逐条列出以免被当作已裁：

1. `PAR-GOV-03..07` 与准入范围装配对首方 Intake 的关系——`SubmissionIntake` 注释写明命令里的准入范围「由试点准入控制按 `PAR-GOV-03..07` 装配」，首方 Intake 落地时这一格从哪里来、登记册为空时编排答哪一格，归读过 `pilotgovernance` 的裁决。
2. 契约的生产方式（手写规范生成处理器胶水，还是从既有处理器与 `outcome` 词表导出）、URL 主版本、public / internal 拆分、文档门户入口——另立记录；`docs/api` 方案稿与已接受 ADR 相冲的三处本记录 Context 末段已列，可借部分由另立记录自己重读原稿取，不引任何未入仓的评估。
3. 撤回、取消、补充、资料修订、索赔提交各命令的来源请求键推导——`Minter` 今天只为提交与撤回各开了铸造口（`MintSubmission` / `MintWithdrawal`），其余命令的信封是否各自分型、`RequestKeyDerivation` 是否按命令分方法，归 `accessidentity` 与 PS / VE owner。
4. `/customer-tracking-view`（`UC-VE-008` 查阅）与 `/claims`（`UC-VE-007` 索赔提交，命令面）是否与 PS 命令面同族——两者都以客户账户为作用域维，本记录按此归入首方族；VE owner 复核。
5. 客户系统的令牌是否允许携带多个客户账户（一个 client 代多个货主账户，平台型客户常见）——本记录按「一凭据绑唯一（租户，客户账户）」立册，与 `ChannelRegistration` 今天的形状一致。这是**首方族自己**的绑定模型 1:1 还是 1:N 的问题：族由谁定义契约与凭据决定，不由绑定基数决定，平台型客户走首方 API 仍是首方族；1:N 要求请求里认领账户并按行上的授予集核对，形状同 ADR-0100 操作者族的「授予集」。归 `accessidentity` 与 PS owner，本记录不预判。
6. 发行方是否必须与操作者族同一个——本记录选同一个以复用信任锚；若部署形态要求客户 API 与运营台分发行方，改的是部署参数的个数，不改三格答复与绑定模型。
7. 首方行上的「受控引用」装什么——发行方侧的客户端标识、密钥指纹（`private_key_jwt` 形）或别的。`NewChannelRegistration` 要求非空，JWT 校验却不依赖行上任何秘密，这一格因此只剩绑定核对的用场；装什么决定绑定核对核的是什么，归 `accessidentity` owner。
8. 第二、三格的稳定错误码——Decision 四写明不可留白；命名归实施票，按 ADR-0055 Decision 二「命名状态，不命名参数」，且第一格 `ACCESS_CHANNEL_NOT_CONFIGURED` 原样不动。
9. 首方族是否补第二种凭据形态（静态密钥头）——Alternatives API Key 一条记下的市场代价；补的话仍是同一发行方、同一受众、同一册；是否补、何时补是产品决定，归 owner。
10. 首方 API 与候选租户既有渠道适配（票 `syn-wall-door-audit/01` S1）的先后——首租户的客户系统迁首方 API 还是等 S1，两条路是否都做、谁先，归 owner 的排期与客户关系决定；本记录只保证首方那条不再挂在 S0 上。

## Links

- [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)：操作者族的同一判据与同一解法；本记录填其 Alternatives 末条留下的空位，并收回其 Context 中「对客户渠道成立」一句说宽的那一半
- [ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)：能力归 `internal/accessidentity` 的出处；本记录接受时把其 Decision 二的适用场景再收窄为租户既有渠道的适配
- [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：未配置格、替换缝、Decision 二的错误码纪律、Decision 三对 `401` 的否决前件、Decision 四的探针同答；本记录不停用其任何条款，其 Alternatives 对运行时渠道登记表的否决经 ADR-0072 Decision 二的收窄而收窄，Decision 五两项不动
- [ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)：Decision 二补记「两族各自要过 `PAR-INT-01` 的真证据门，不得互相顶替」；本记录接受时把其客户半收窄为租户既有渠道的适配，保留并结构化「不得互相顶替」
- [ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md)：`SYN-` 隔离形态与逐口放行纪律；本记录不碰其任何一条
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：首方 API 的契约形状；本记录不改一字，`Idempotency-Key` 的否决原样有效
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：三格按恢复动作分格的判据
- [ADR-0003](./0003-group-tenant-legal-entity-customer-account.md)：一凭据绑唯一（租户，客户账户）所依的三级边界；`seal` 是它在铸造这一步的落法
- [ADR-0019](./0019-product-ships-tenant-facing-operator-clients.md)（已被取代）：「租户面向它自己客户的既有接入渠道，不是本仓要建的东西」一句的出处；本记录收回其对渠道的引申，不改其对界面的结论
- [参数登记册](../product/PILOT-PARAMETER-REGISTER.md)：`PAR-INT-01` 原文出处；本记录不改其状态，只收窄适用面并预告新增一行
- [开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)：机制半边与实例半边定义的唯一权威
- [parcel-shipment CONTEXT](../domain/parcel-shipment/CONTEXT.md)：「共享身份认证与授权技术能力拥有凭据验证、通用授权策略和授权作用域签发；`parcel-shipment` 只消费已授权作用域」——凭据验证归 `accessidentity` 而不归业务上下文的硬句
- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：输入语义契约里来源信封「不可由客户声明、来自接入适配器与接入来源」的出处，「接入请求标识」一词的出处
- [`docs/api` Go + chi + Scalar 方案稿](../api/Go_Chi_Scalar_API_Documentation_Final_V1.0.md)：参考输入；其与 ADR-0022 / ADR-0055 相冲的三处见本记录 Context 末段，此外无入仓评估可引
- `.scratch/syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md`：「重启范围」节——ADR-0072 重启门槛已到、`PAR-INT-01` 只到两件半、S0 归客户、S1 按 API 型渠道形状立册；本记录把 S1 定位为既有渠道适配族的册，接受时加改指注
- `.scratch/tenant-implementation-01/instance-register-draft.md` 的 `PAR-INT-01` 行与 `implementation-week-decisions-and-customer-pack.md` 的 D-08 行：候选租户既有渠道（API、apikey 头、证据 E-02）与「accessidentity S1–S3 按 C0 答复推进」的出处；证据本体按指纹索引制外置，本记录不复录
- `internal/accessidentity`：`SourceEnvelope`、`ChannelRegistration`、`ChannelRegistry`、`CredentialVerifier`、`ChannelCredentialProof`、`Minter`、`seal` 的现状形状，`mint.go` `authenticate` 注释「顺序不能反」的前提；本记录 Decision 二、三在其上立客户族那一半
- `cmd/parcel-api` 的 `assembleBusinessEndpoints`：客户业务面各行（`/shipment-requests` 经隔离提交开关换值除外）今天挂字面量 `UnconfiguredIntake{}` 的出处，本记录 Decision 四的替换点
- `internal/visibilityexception/adapters/http/receive_claim.go`：`/claims` 是 `UC-VE-007` 索赔提交的命令口——本记录把它归入命令面的出处
- 来源：IDP 队列通道 5 的质疑与「起草」指令（2026-09-16）；通道 1 的复核与用户「按建议处置」指令（同日，草案修订）
