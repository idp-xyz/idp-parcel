# 接入与身份（`accessidentity`）

代码在 `internal/accessidentity`，受控登记口在 `cmd/parcel-access-register`。对照 main `1bdbc1b7b`（2026-11-12）写成。

**这份文件核到什么程度**：旧文档里没有这个包的专属上下文文档，全文是从代码与 Go 注释读出来的。取值表、答复、入口照常量与路径字面量抄；「规则」一节每条都在代码里找到了对应的类型或函数，但没有跑测试验证行为。包注释自己写明：它是共享技术能力，不是限界上下文，不拥有业务领域语言，所以没有 `domain` 子包。

## 管什么，不管什么

管：谁在调用、代表哪个租户、此刻有没有这一格权。具体是六本册（操作者册、作业设备册、集成客户端册、推送源册、客户渠道册、准入范围对照册）、令牌校验、以及把一次出示铸成信封。

不管：

- 业务规则与业务数据。各业务上下文只消费已铸好的信封，自己不验凭据；信封里的租户、客户账户、来源是字符串，业务含义由消费方的领域类型承载。
- 凭据本体。库里只存发行方、`sub` 与受控引用；令牌只在核验那一刻存在。
- 「谁在写生产」的权威。那是 `pilotgovernance` 的生产权威区间；本包的对照册只做坐标换算。
- 某次订舱属于谁、取消授权规则这类要读业务对象才答得出的核对——在各业务上下文的编排里。
- 某个租户有哪些操作者、给谁授了什么。那是册上的行，属租户取值；本包只定册的列。

## 词

| 词 | 意思 |
|---|---|
| 来源信封 | 铸造出来的四要素：租户、客户账户、来源、来源请求键（`SourceEnvelope`）。包外造不出，拿到即表示经过铸造 |
| 铸造 | 校验一次出示、查册、核授予之后产出信封的那一步 |
| 四族 | 操作者族、集成客户端族、首方客户族、租户既有渠道的适配。前三族已立；最后一族只有接口 |
| 操作者主体 | 发行方加 `sub`，两件合起来才是身份 |
| 绑定 | 一个主体绑定唯一租户的那一行。集成客户端另绑一个来源身份，首方客户端另绑一个客户账户 |
| 授予 | 按能力单位显式登记的一笔权，带登记标识、生效区间与依据 |
| 撤销 | 对一笔授予另登的一行：自撤销时刻起（含）不再生效 |
| 能力面 | 操作者授予的单位：能在哪一族端点上做什么 |
| 决定种类 | 「运营决定」能力面下的细分，授予按种类登 |
| 作业范围 | 「作业事实登记」授予与设备登记的单位：一个节点或一个场外作业范围 |
| 作业设备 | 一线作业端登事实所用、登在册上的设备 |
| 外部事实类型 | 集成客户端授予的单位：外部系统能交哪一类事实 |
| 客户面能力 | 首方客户端授予的单位：一口一词 |
| 来源身份 | 集成客户端代表的外部方（某报关服务商、某承运商、某银行） |
| 推送源 | 只能推送、不出示令牌的外部源，绑在一个集成客户端主体上 |
| 准入范围 | 一笔生产写此刻落不落在治理登记的生产权威区间内 |
| 准入范围对照 | 某租户在（能力、事实类型）上的生产写落在治理登记册的哪个对象范围 |
| 接入请求标识 | 首方客户系统在载荷里给的 `accessRequestId`，来源请求键由它推导 |
| 受控引用 | 指向受限存储里凭据或密钥出处的不透明串，本包不解读 |

## 取值表（代码里的封闭集）

| 轴 | 取值 |
|---|---|
| 能力面 `CapabilityFace` | `REGISTRY_CONFIGURATION_WRITE`（登记册配置写） `MASTER_DATA_AND_OPERATIONS_READ`（主数据与运营查阅读） `OPERATION_DECISION`（运营决定） `WORK_FACT_REGISTRATION`（作业事实登记） `OPERATOR_ACCESS_ADMINISTRATION`（操作者接入管理） |
| 预留、此刻不可授 | `GOVERNANCE_REGISTRATION`（治理登记） |
| 作业范围种类 | `NODE` `OFFSITE` |
| 外部事实类型 | `CUSTOMS_EXTERNAL_RESULT` `REGULATORY_CREDENTIAL` `CARRIER_TRACKING` `CARRIER_FIRST_EFFECTIVE_PICKUP_EVIDENCE` `EXTERNAL_FUNDS_FACT` `CARRIER_BOOKING_ANSWER` `EXTERNAL_REGULATORY_MANIFEST` |
| 客户面能力 | `SHIPMENT_REQUEST_SUBMISSION` `SHIPMENT_REQUEST_WITHDRAWAL` `PARCEL_CANCELLATION` `SOURCE_DATA_AMENDMENT` `SHIPMENT_REQUEST_SUPPLEMENT` `CLAIM_SUBMISSION` `DELIVERY_PRE_ADVICE_SUBMISSION` `DELIVERY_PRE_ADVICE_WITHDRAWAL` `DELIVERY_HANDOVER_CONFIRMATION` `CUSTOMER_TRACKING_VIEW_QUERY` `CUSTOMER_SHIPMENT_TRACKING_QUERY` |
| 铸造侧判准入的作业事实 `WorkFactKind` | `DELIVERY_HANDOVER_NOTE_OPENING` `DELIVERY_HANDOVER_NOTE_ASSIGNMENT` `DELIVERY_HANDOVER_NOTE_CLOSURE` `NODE_COLLABORATION_EXECUTION` |
| 准入要求的能力词 | `OPERATION_DECISION` `WORK_FACT_REGISTRATION` `EXTERNAL_FACT` `CUSTOMER_COMMAND` |
| 推送签名形态 | `HMAC_SHA256` `ED25519` |
| 设备查阅状态 | `REGISTERED` `DEACTIVATED` |
| 信封来源（固定） | `ADMIN_CONSOLE`（操作者信封） `FRONTLINE_CLIENT`（作业事实信封） |

决定种类 `DecisionKind`（另有 `REGULATORY_RESTRICTION_ESTABLISHMENT` `REGULATORY_RESTRICTION_RELEASE` `EXCEPTION_CASE_TAKE_UP` `EXCEPTION_CASE_CLOSURE` `EXCEPTION_CASE_MERGE` `EVIDENCE_SUBMISSION` `EVIDENCE_APPRAISAL` `EVIDENCE_DISCLOSURE_PREPARATION` `CUSTOMER_DISCLOSURE_DECISION` `CUSTOMER_DISCLOSURE_AUTHORIZATION` `CUSTOMER_EXCEPTION_NOTIFICATION` `CHARGE_ADJUSTMENT_REGISTRATION` `STATEMENT_VOID` `LATE_CHARGE_INCLUSION` `ADJUSTMENT_INCLUSION` `CUSTOMS_EXTERNAL_RESULT_ENTRY` `EXTERNAL_TRACKING_ENTRY` `CUSTOMS_MANUAL_LODGEMENT` `BOOKING_ANSWER_ENTRY` `EXTERNAL_MANIFEST_ENTRY`）：`MANUAL_REVIEW_COMPLETION` `ACTIVE_REJECTION` `AUTHORIZED_DISPOSITION` `CONTROLLED_CLOSURE` `CONTROLLED_REOPENING` `SEGMENT_CLOSURE` `DISPATCH_TASK_REGISTRATION` `LOAD_ASSIGNMENT` `PARTICIPATION_TERMINATION` `EFFECTIVE_TIME_JUDGMENT` `CARRIER_FIRST_EFFECTIVE_PICKUP_JUDGMENT` `PENDING_IDENTITY_RECHECK` `IDENTITY_CONFLICT_DISPOSITION` `TRANSPORT_COMMISSION_SUBMISSION` `BOOKING_REQUEST_SUBMISSION` `TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION` `CAPACITY_RESERVATION` `CAPACITY_RELEASE` `FULFILLMENT_SEGMENT_COMMISSION_ADOPTION` `TRANSPORT_COMMISSION_CANCELLATION` `BOOKING_REQUEST_CANCELLATION` `DELIVERY_HANDOVER_CONFIRMATION` `CUSTOMS_CASE_ESTABLISHMENT` `CUSTOMS_SUBMISSION_AUTHORIZATION` `CUSTOMS_DECLARATION_SUBMISSION` `DELIVERY_DISPATCH_REASK` `EXTERNAL_FUNDS_FACT_ADOPTION` `REGULATORY_CREDENTIAL_REGISTRATION` `TRANSPORT_COMMISSION_AGREEMENT_REVISION` `FULFILLMENT_SEGMENT_COMMISSION_ADOPTION_WITHDRAWAL` `TRANSPORT_CHARGE_OCCURRENCE_INVALIDATION` `AUTHORIZED_REROUTE` `CONTROL_RELEASE_REFUSAL_HANDLING` `CANCELLATION_ORDER_CONFLICT_RESOLUTION` `PICKUP_ATTEMPT_RESULT_CONFIRMATION` `DELIVERY_ATTEMPT_RESULT_CONFIRMATION` `POST_INTAKE_SERVICE_DISPOSITION` `REGULATORY_DISPOSITION_ACCEPTANCE` `ALTERNATE_JOURNEY_START` `INTERNAL_RESTRICTION_ESTABLISHMENT` `INTERNAL_RESTRICTION_DECISION` `CUSTOMS_CASE_CLOSURE` `RELEASE_GATE_VERIFICATION` `NODE_COLLABORATION_ACCEPTANCE` `CHARGE_CONFIRMATION` `STATEMENT_PUBLICATION` `SUPPLIER_BILL_RECEPTION` `SUPPLIER_BILL_AUDIT` `SUPPLIER_BILL_DISPUTE_DECISION` `POST_INTAKE_MANUAL_ITEM_HANDLING` `STATEMENT_DISPUTE_REGISTRATION` `STATEMENT_DISPUTE_RESOLUTION`。每种的含义在 `operation_decision.go` 的常量注释里；一口一种，持一种不因此取得另一种。

## 答复

**铸造不成的各格**（每格对一种恢复动作，不得合并）

| 错误 | 什么时候 | 谁去处理 |
|---|---|---|
| `ErrAccessChannelNotConfigured` | 这一族的发行方部署参数没设；既有渠道一族是册上没有那一行 | 配部署参数或渠道的人 |
| `ErrCredentialRejected` | 令牌缺失、过期、校验不过；或册上要求证书绑定而令牌没绑 | 持凭据的人 |
| `ErrCredentialVerifierUnavailable` | 取不回发行方公钥集 | 运维 |
| `ErrOperatorNotGranted` `ErrIntegrationClientNotGranted` `ErrCustomerClientNotGranted` | 不在册、不绑所请求的租户、或此刻没有这一格的生效授予。几种情形同答，不说哪一半不对 | 管授予的人 |
| `ErrDeviceNotInUse` | 设备不在本租户册上、已停用、或登在别的作业范围上 | 管设备的人 |
| `ErrOutsideAdmissionScope` | 这笔生产写不在准入范围内 | 阶段治理去登区间或对照 |
| `ErrRequestKeyNotDerived` | 接入请求标识缺席，或提交的标识以保留命令前缀开头 | 接入方补齐请求 |
| `ErrPushSignatureRejected` `ErrPushTimestampOutsideWindow` `ErrPushReplayed` | 推送签名不成立；签名成立但越窗；同一份签名原文已收过 | 推送方 |
| `Err…RegistryUnavailable` `ErrAdmissionScopeUnavailable` `ErrPushSigningKeyUnavailable` `ErrPushReplayLedgerUnavailable` | 册、准入范围、密钥或重放账读不动 | 运维 |

在线口上的问题码：`ACCESS_CHANNEL_NOT_CONFIGURED` `OPERATOR_CREDENTIAL_REJECTED` `OPERATOR_NOT_GRANTED` `IDENTITY_DEPENDENCY_UNAVAILABLE` `MALFORMED_REQUEST` `METHOD_NOT_ALLOWED` `NO_ANSWER_FORMED` `INTAKE_FAILED`。

**登记口的答复**（各本册共用前三格）

| 答复 | 什么时候 |
|---|---|
| `RECORDED` | 新登一行 |
| `ALREADY_REGISTERED` | 同键同内容重放，答原结果 |
| `CONTENT_CONFLICT` | 同键异内容，册上那一行原样不动 |
| `SUBJECT_BOUND_TO_ANOTHER_TENANT` `CLIENT_BOUND_TO_ANOTHER_TENANT` `PUSH_SOURCE_BOUND_TO_ANOTHER_TENANT` | 主体已绑在别的租户上。不说是哪个租户 |
| `OPERATOR_NOT_REGISTERED` `CLIENT_NOT_REGISTERED` `GRANT_NOT_REGISTERED` `DEVICE_NOT_REGISTERED` `PUSH_SOURCE_NOT_REGISTERED` `SIGNING_KEY_NOT_REGISTERED` `MAPPING_NOT_REGISTERED` | 本租户册上没有被指名的那一行 |
| `MAPPING_OVERLAPS` | 待登的对照与同键已有的一行生效期重叠 |
| `REVOCATION_WOULD_LOCK_OUT_CALLER` | 在线撤销口上，撤的是调用者自己此刻生效的操作者接入管理授予 |

## 规则

**身份只来自册**

- 租户、客户账户、来源身份只取自册上的绑定。请求里指名的租户只用来比对，不一致即答未授予；集成客户端与首方客户端的请求不带租户。
- 信封的字段不导出、包外没有构造函数；零值不可用，消费方先问 `Minted`。
- 各族信封分型：`OperatorEnvelope`、`WorkFactEnvelope`、`IntegrationClientEnvelope`、`SourceEnvelope` 互不通用；客户命令的信封再按命令分型（提交、撤回、取消、修订、补充、索赔、送站预报提交与撤回、送站交接确认）。递错口编译不过。
- 凭据与密钥类型的 `String`、`GoString`、`LogValue` 一律只交出替身，印不出本体。

**绑定与授予**

- 一个主体绑定唯一租户。绑定没有生效区间，也不可撤销或改指；停用一个人或一个客户端，撤它名下的授予。
- 每一行登记都要带依据；起点、撤销时刻不代填，没有「从现在起」的缺省。
- 授予带自己的登记标识。撤了再授是两笔授予，不覆盖前一笔。
- 生效区间含起点、不含终点；终点缺席即不设终点。「是否生效」由区间与撤销对时点导出，不存成状态。
- 「运营决定」的授予必须带决定种类，「作业事实登记」的授予必须带作业范围；整格问 `HoldsAt` 对这两格一律答否，要按种类问 `HoldsDecisionAt`、按范围问 `HoldsWorkFactAt`。
- 预留的治理登记一格登不进册（`ErrCapabilityFaceReserved`），与认不得的格（`ErrCapabilityFaceUnknown`）分开答。
- 操作者接入管理与登记册配置写分格：能登业务数据的人不因此能给人授权，反之亦然。
- 在线撤销不许把调用者自己锁在操作者接入管理面之外（`LocksOutCallerIfRevoked`）；只挡调用者自己这一条，不数全租户还剩几个管理员。

**铸造的顺序**

- 操作者（`MintOperator`）：校验令牌 → 查操作者册 → 按租户与能力面核授予 → 判准入范围（这一口要判时）。
- 作业事实（`MintWorkFact`）：校验令牌 → 按作业范围核授予 → 查设备册 → 判准入范围。信封记操作者主体与设备两件，缺一不铸。设备在不在用按铸造那一刻判，不按事实自报的发生时间。
- 集成客户端（`MintIntegrationClient`）：校验令牌 → 查册 → 核证书绑定要求 → 按事实类型核授予 → 判准入范围。
- 首方客户（`CustomerClientMinter`）：顺序同集成客户端；认证过了才读请求载荷、推导来源请求键。
- 靠后的检查不向没过前一道的人泄露信息：没有授予的人看不出设备登没登、租户登没登区间。
- 管理台信封的授予集不含作业事实登记；查阅口与登记写面不带准入要求。

**来源请求键（首方客户族）**

- 提交的键是接入请求标识本身；撤回、取消、修订、补充、索赔的键是命令前缀加标识（`withdrawal:` `parcel-cancellation:` `source-data-amendment:` `supplement:` `claim:`）。同一标识先提交后撤回是两条不同的键。
- 提交的标识不得以这些保留前缀开头。

**准入范围**

- 铸造器的准入范围不许缺席。权威串没配时用 `UnconfiguredAdmissionScope`，对一切生产写答不在准入范围；首个租户登记区间之前任何生产写都被拦是设计。
- 配了之后经 `adapters/pilotgovernance` 的桥：先取对照换出对象范围，再问治理的生产权威区间。对照没登即答不在准入范围。
- 对照的键是（租户、能力、事实类型），任一时刻至多一行生效。同键重叠在登记时拒；读到两行同时生效答 `ErrConflictingAdmissionScopeMappings`，不任选。
- 对照只收各族认证方今天会问的词（`checkAdmissionRequirement`）。委托提交不在铸造侧判准入，它的词单独认下，供小包托运的生产归属目录读对照。
- 客户查阅两口（追踪视图、按运单号查询）不判准入范围。

**令牌与部署参数**

- 三族各有一组发行方、JWKS 地址、受众，全部必填、不给默认。三件都未设即整族答未配置；只设一部分或不成形，进程拒绝启动。
- 首方客户族的受众与另两族任一相同，进程拒绝启动。
- 集成客户端与首方客户端可逐个要求证书绑定令牌（租户取值）；要求了而令牌没绑，答令牌不过。

**签名推送**

- 顺序：认推送源 → 验签名 → 判窗口 → 查所绑客户端、核授予、判准入 → 记重放账。前面任何一步拒了都不入账。
- 窗口与时钟偏差是租户对这个推送源的取值，产品不给缺省。密钥只存受控引用；轮换是登新钥再给旧钥登停用。
- 重放按签名原文的摘要判，不按推送标识：带新时间戳重签的重试不算重放，交各命令口按载荷判。

## 生命周期

- **绑定**（操作者、集成客户端、首方客户端、推送源）：登记后不变。
- **授予**：已登记 → 区间内生效 →（区间到期，或另登一笔撤销）不再生效。记录只增不改。
- **作业设备**：已登记 → 停用（另登一行，带时刻与依据）。
- **推送签名密钥**：已登记 → 区间内生效 → 停用。
- **准入范围对照**：已登记 → 结束（另登一行）。换试点范围版本时先在切换时刻结束旧行，再自同一时刻登新行。
- **信封**：每次请求铸一次，不落库。

## 入口

在线口（`cmd/parcel-api`，路径在 `adapters/http`）：

| 做什么 | 路径 | 守口的能力面 |
|---|---|---|
| 查本租户操作者册、产品词表与信任的发行方 | `/operators` | 操作者接入管理 |
| 登操作者、登授予、撤授予 | `/operator-registrations` `/operator-grant-registrations` `/operator-grant-revocations` | 操作者接入管理 |
| 本会话操作者绑在哪个租户、持有哪几格授予 | `/operator-binding` `/operator-grants` | 主数据与运营查阅读 |
| 查作业设备册 | `/work-devices` | 主数据与运营查阅读 |

其余各上下文的在线口经各自 `adapters/accessidentity` 下的认证件调用本包的铸造器；`parcel-api` 进程里每族一只铸造器，共用一只准入范围。

部署参数：`IDP_PARCEL_OPERATOR_OIDC_*`、`IDP_PARCEL_INTEGRATION_CLIENT_OIDC_*`、`IDP_PARCEL_CUSTOMER_API_OIDC_*`（各含 `ISSUER` `JWKS_URL` `AUDIENCE`），以及 `IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY`（本产品在治理登记册里的权威串）。

离线口 `cmd/parcel-access-register`（受控批量登记，批文每一格来自输入文件，不内置默认；退出码 0 全部落定、1 技术失败、2 有冲突或被拒）：`operator-register` `operator-grant` `operator-revoke` `integration-client-register` `integration-client-grant` `integration-client-revoke` `device-register` `device-deactivate` `admission-scope-mapping-register` `admission-scope-mapping-end` `push-source-register` `push-signing-key-register` `push-signing-key-retire` `customer-client-register` `customer-client-grant` `customer-client-revoke`。

设备、集成客户端、首方客户端、推送源、准入范围对照只有离线登记口，没有在线登记口。

## 有代码、但没有生产入口的

- **签名推送的铸造**：`SignedPushMinter`、`ParseStandardWebhook`、`PushSigningKeyResolver`（只有 `UnconfiguredPushSigningKeyResolver`，没有接受限存储的实现）在 `cmd/` 下没有任何装配。推送源与签名密钥登得进册（离线口），重放账在 `adapters/postgres` 有实现，但没有一个在线口收推送。
- **租户既有渠道的适配**：`Minter`（`NewMinter`、`MintSubmission`、`MintWithdrawal`）、`ChannelRegistry`、`CredentialVerifier` 只有接口与铸造逻辑，没有生产实现，也没有装配。这一族的登记表与凭据形态等租户的渠道证据，判重口径（`RequestKeyDerivation`）不带默认。
