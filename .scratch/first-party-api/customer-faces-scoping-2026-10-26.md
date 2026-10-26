# 首方客户 API 五口上线计划（只读摸底，基线 main 最新）

取证方式：grep + 局部 sed；未跑测试。凡写"已有/缺"均对到文件。tops-20 = 接线与 `internal/accessidentity/customer_client_*.go`；tops-26 = `internal/parcelshipment/application` 与 PS 应用层票。

## 0. 共性结论

样板（提交口）的链条：`CustomerClientCredentialVerifier` -> `CustomerClientMinter.admit`（令牌 -> 客户渠道册 -> 证书绑定 -> 授予 -> 准入范围）-> `mintCustomerCommand`（读请求 -> 推导来源请求键 -> seal）-> 领域适配器里的 `CustomerSubmissionIntake`（译命令）。送站预报两口（`customer_client_pre_advice.go`）已示范"一口一词一个铸造方法 + 按命令分型信封"。

五口今天共同的缺：
1. 铸造器只有 `MintSubmission`、`MintDeliveryPreAdvice*`、`MintDeliveryHandoverConfirmation`、`MintTrackingViewQuery`；没有撤回、取消、修订、补充、索赔五个 Mint 方法与 `CustomerCapability` 词。
2. `RequestKeyDerivation.DeriveWithdrawalRequestKey` 仍答推导不出（`customer_client_envelope.go:191`）；修订、补充、取消、索赔没有推导方法。
3. 五口的领域 Intake 没有首方实现（`IntakeWithdrawal`/`IntakeCancellation`/`IntakeSourceDataAmendment`/`IntakeSupplement`/`IntakeClaim` 全无实现，只有 `UnconfiguredIntake`）。
4. 每口的"规范化形状 + 形状版本"（ADR-0139 决定八"未定形状的口不开首方渠道"）都没定；客户系统 JSON 载荷契约（ADR-0140）也没有这五口。
5. 租户取值：演示租户的授予、准入范围对照、权威区间（`data/access/`、`data/governance/`）与 `scripts/demo-seeds` 都没有这五口。
6. 账户绑定：PS 命令里的 `Identity` 是 (租户, 客户账户, 来源, 请求键)，租户与账户取自册上绑定，客户只指名"原提交的接入请求标识"。所以首发 1:1 下，跨账户目标天然"找不到"，隔离靠身份构造，不靠额外判断（须各口测试钉住"他账户的标识答找不到、不泄露存在"）。1:N 属 first-party-api/01，未立。
7. 读回：客户侧没有首方的委托状态读口。`/shipment-request-views` 仍走 `isolatedRead`（`endpoints.go` 注释；operator-channel/53 管运营侧租户级读，不是客户读）。客户今天可经首方读的只有 `/v1/customer-tracking-view`（票 96 已换）。因此"能读回"目前只能靠：(a) 写口自己的业务答复；(b) 同一接入请求重放得到同一答复。

## 1. 逐口

图例：有 = 已有；缺 = 没有；半 = 半有。

### 1.1 撤回 `POST /shipment-requests/withdrawals`（UC-PS-005）

| 层 | 状态 | 要做什么 |
|---|---|---|
| 首方 Intake | 缺 | 在 `parcelshipment/adapters/http` 加 `CustomerWithdrawalIntake`，实现 `IntakeWithdrawal`，形照 `customer_submission_intake.go`：先认证后读载荷；载荷取（原提交的接入请求标识、撤回方、原因、可选提交版本）。 |
| 请求键推导 | 缺（显式留白） | 实现 `DeriveWithdrawalRequestKey`，并定撤回自己的 `WithdrawalIdentity` 与原提交 `Identity` 的判重空间如何分（见产品取舍 A）。 |
| 账户绑定与准入 | 半 | 铸造器 `admit` 现成；缺 `MintWithdrawal`、能力词（建议 `shipments:withdraw`，对齐 first-party-api/01 候选词）、撤回是否在铸造侧判准入范围（建议判：生产写，同提交/预报）。 |
| 授权 | 半 | 编排内 `Requester` 引用由 Intake 填（取渠道行客户端，非客户自报）；撤回无 PC 裁定环节，授权 = 账户绑定 + 授予。需 owner 确认"客户系统本身就是有权撤回者"（见 B）。 |
| 编排 | 有 | `buildWithdrawalOrchestration` 已接真。上线前必修在 tops-26：dc/44（保全后未落成的重放短路）、code/99（取锁次序，非阻断但建议同批）。 |
| 存储与读回 | 半 | 撤回决定不进快照、真库读不回（code/36，tops-26 评审中）：客户重发同一撤回答 5xx，撤回后重提（`WITHDRAWN_RESUBMISSION`）5xx。**这是上线硬前置**。 |
| 客户读面/回执 | 缺 | 只有写口的业务答复；无首方委托状态读（见共性 7、取舍 D）。 |

### 1.2 取消包裹 `POST /shipment-requests/parcel-cancellations`（UC-PS-006）

| 层 | 状态 | 要做什么 |
|---|---|---|
| 首方 Intake | 缺 | `CustomerCancellationIntake`：载荷取（目标委托的接入请求标识或委托标识、包裹标识、原因、接入请求标识）；译 `CancelParcelCommand`。注意此命令只有一个 `Identity`（取消自己的身份）和 `ShipmentRequestID`——客户系统拿得到 `ShipmentRequestID` 吗要核（提交答复是否返回），否则 Intake 需按（账户, 接入请求标识）反查委托，且反查必须带账户条件。 |
| 请求键推导 | 缺 | 新加 `DeriveCancellationRequestKey`；只有一个身份，不必另开空间，但仍要与提交键空间分开（同 A）。 |
| 账户绑定与准入 | 半 | 同撤回：缺 Mint 方法与能力词（`shipments:cancel`）。命令不带账户，须保证目标委托属信封账户（应用层不核账户——grep 显示 `cancel_parcel.go` 无 CustomerAccount 引用），要在 Intake 反查或应用层补；建议归 tops-26 一并补闸。 |
| 授权 | 缺（租户半边未登记） | 授权缝是真翻译链但尾端 `AdoptedStageOwner` 为 nil，编排停在 `AUTHORITY_UNCONFIGURED`（`assemble_cancellation.go`）。要装配采用版本源、三个 PC 内容读口与请求方映射（PAR-COM-17 实例半边），并为首个租户登记"谁能在哪个阶段取消"的规则。UC-PS-006 明禁默认放行或默认拒绝。见取舍 C。 |
| 编排 | 半 | 已接真，但**没有委托生命周期闸门**（tops-26 事实①）：已撤回、已拒绝、仅已提交的委托下的包裹都不拦。取消口一通就可达，是接口前提缺口。另 code/98（交付终局不读已成立取消）取消一旦可成立就生效；code/76（冲突无解开面）为上线后可见卡住。 |
| 存储与读回 | 有（取消决定库、按包裹读） | 缺客户侧读回，同 1.1。 |
| 客户读面 | 缺 | 同上；追踪视图或可显示"包裹已取消"，核其是否已含。 |

### 1.3 资料修订 `POST /shipment-requests/source-data-amendments`（UC-PS-002）

| 层 | 状态 | 要做什么 |
|---|---|---|
| 首方 Intake | 缺 | `CustomerSourceDataAmendmentIntake`：载荷最重——范围（`SourceDataScope`）、基准（`SourceDataBasis`）、意图（补充/更正/显式清空）、`Content`（要素内容，由接单入口对同一份规范化输入一次产出，pp-seams/05）、适用时间可缺。需定规范化形状版本，复用提交的 `CanonicalizeSubmission` 要素构造，不得重算。 |
| 请求键推导 | 缺 | `DeriveAmendmentRequestKey`，`AmendmentIdentity` 与原 `Identity` 分空间（同 A）。 |
| 账户绑定与准入 | 半 | 缺 Mint 与能力词（`shipments:amend`）；目标委托经 `Identity` 找（带账户），同 1.1。 |
| 授权 | 半 | PC 授权适配器已接真（ps-port-remainder/03、pc-gaps/08 resolved），**商业坐标映射（法人、等级、范围、原因、证据、时点，及请求方是客户还是代录角色）是实例半边，装配里 `RequestSource` 留 nil**，如实停在 `SourceDataAmendmentAuthorityUnavailable`（`assemble_customer_amendment.go:117-125`）。要做：定首方请求方 -> PC 请求方引用的映射，租户登记授权册与合同委派册。允许矩阵（ADR-0120）也要租户登声明，否则停待复核。见取舍 B。 |
| 编排 | 半 | 接真；dc/44 同撤回为上线前必修。 |
| 存储与读回 | 有 | 资料新版本落库 + 交接意图出站；客户侧读回缺。 |
| 客户读面 | 缺 | 同上。 |

### 1.4 补充 `POST /shipment-requests/supplements`（ADR-0106 决定四）

| 层 | 状态 | 要做什么 |
|---|---|---|
| 首方 Intake | 缺 | `CustomerSupplementIntake`：载荷是一份完整新提交版本（包裹集、画像、要素、服务产品），形状最接近提交口，可复用 `decodeSubmissionDraft` 系列；外加指名目标委托（原接入请求标识）与基准版本。 |
| 请求键推导 | 缺 | `DeriveSupplementRequestKey`，`SupplementIdentity` 分空间（同 A）。 |
| 账户绑定与准入 | 半 | 缺 Mint 与能力词（`shipments:supplement`）。注释所说"谁能替哪个客户账户补充"在 1:1 下就是"册上绑定的账户"，不需要新模型；1:N 才是问题。 |
| 授权 | 半 | 编排无 PC 授权环节（只有渠道认证）。是否需要比账户绑定更多的授权，见取舍 B。 |
| 基准版本怎么译 | 缺（要裁） | 命令要 `BasisVersion domain.SubmissionVersionID`（内部标识）。见取舍 E。 |
| 编排 | 有 | 已接真并携"新提交版本已形成"信封，消费门在 parcel-dispatch；first-tenant-runway/09 已落地使"等待受控补充"可被续办。 |
| 存储与读回 | 有（版本落库）/ 缺客户读 | 同上。 |

### 1.5 索赔 `POST /claims`（VE，UC-VE-007）

即现成票 demo-chain-gaps/102（M，needs-triage，描述完整）。

| 层 | 状态 | 要做什么 |
|---|---|---|
| 首方 Intake | 缺 | `ClaimIntake` 首方实现 + `visibilityexception/adapters/accessidentity` 认证方。命令 `ReceiveClaimCommand` 的 `TenantID`、`Customer` 取自信封；`Batch`/`Item`/`Applicant`/`Contract`/`Target`/`Kind` 由载荷给；提交时间取到达信封（头注已钉）。 |
| 请求键推导 | 缺 | 新加推导方法。索赔另有业务自然键（批次 + 项），要裁请求键与自然键的关系（取舍 F）。 |
| 账户绑定与准入 | 缺 | `MintClaimSubmission` + 能力词（`claims:submit`）+ 铸造侧判准入（ADR-0139 决定八，票 102 已写明）+ access_identity 迁移（CHECK 取并集）+ 准入范围对照种子。 |
| 授权 | 半 | 索赔受理只要求客户账户来自认证；资格审核与责任结论走内部入口，不在客户面。`Applicant`/`Contract` 引用须核属信封账户，要在 Intake 或应用层校（票 102 未写，建议补）。 |
| 编排 | 有 | `buildClaimsOrchestration` 已接；`assemble_claims*_test.go` 已有。 |
| 存储与读回 | 有（运营侧 `/claims-recovery-records`） | 客户读回缺。 |
| 客户读面 | 缺 | 见取舍 D。 |

## 2. 产品取舍（含倾向）

**A. 撤回/修订/补充的来源请求键空间。** 提交口的请求键 = 接入请求标识，原样。PS 命令里原提交 `Identity` 与 `WithdrawalIdentity`/`AmendmentIdentity`/`SupplementIdentity` 同为（租户, 账户, 来源, 请求键），若都直接取同一格，客户用同一标识先提交后撤回会被判成重放（`RequestKeyDerivation` 头注所担心的）。选项：
1. 契约规定标识逐请求唯一，客户必须换标识；Intake 原样推导。优点简单；缺点靠客户自觉，违反时静默误判。
2. 推导时加命令前缀（如 `withdrawal:<id>`），同一标识跨命令不撞。确定、客户无感；代价是契约要写明"标识在命令内唯一"，且重放按（命令, 标识）。
3. 用不同 `Source` 值分空间。要动来源词表，改动大。
倾向 2：既满足 ADR-0139 决定五"标识逐请求唯一"又消除静默误判；送站预报已采取"各在自己册里判重"，委托侧这三口的册是同一本，所以必须显式分。需 owner 对 `DeriveWithdrawalRequestKey` 头注所留的"同一标识先提交后撤回答什么"点头，属 ADR-0139 越权风险点的复核项。

**B. 修订、补充的授权模型（谁能对哪个账户做）。**
1. 客户系统即账户本人：册上绑定 = 授权，请求方 = 渠道行客户端；修订仍走 PC 裁定（请求方为"客户自己"，不涉委派）。1:1，最小。
2. 代理人模型：平台/货代代多个货主，要 1:N 账户集 + 请求认领账户（first-party-api/01）+ PC 合同委派解出实际决定方（代录），补充/修订都要。
3. 双轨：首发做 1，保留 2 的字段位置。
倾向 1（首发），补充只靠账户绑定 + 授予 + 基准版本核对，不再引入 PC 裁定；修订保留 PC 裁定，因为 UC-PS-002 与 ADR-0116 已把它定为要裁定的动作且适配器已接，租户登记坐标即可，不另拟。理由：ADR-0139 已定首发 1:1；2 依赖 first-party-api/01 与合同委派实例，规模 L 以上且没有首个租户的需求证据。**若首个租户是平台型/货代，需立即改选 2。**

**C. 取消的授权规则。** UC-PS-006 禁默认。首个租户须登记"哪些阶段、哪类请求方可取消"（采用版本源 + PC 内容）。选项：登记为"客户系统可在收寄前取消"，或"一律需运营代办（客户口不开）"。这是租户商业事实，开发方不替拟，需用户/租户给；若暂不给，取消口接上后将如实停在 `AUTHORITY_UNCONFIGURED`，不应对外承诺为可用。

**D. 客户读回面。** 选项：(1) 首期只靠写口答复与同请求重放；(2) 加首方委托状态读口（`/v1/shipment-requests/{...}` 之类，按账户切）；(3) 加索赔受理回执读。倾向 (1) 做首期，(2) 作为紧随的独立票（否则"撤回完成后客户无法自证"），(3) 暂不做。理由：写口答复已携业务结果，重放等价读回，改动小；(2) 要定客户可见字段，属新契约。需用户确认首期能否接受没有客户状态读。

**E. 补充的基准版本。** 命令要内部 `SubmissionVersionID`。选项：(a) 提交答复里把版本标识返回给客户，补充载荷带回；(b) 客户只给原接入请求标识，服务端取当前最新版本（有竞态：客户基于旧画面补充会覆盖）；(c) 客户给"版本序号"这类对外稳定词，服务端译。倾向 (a)：沿用内部标识最省，并发语义清楚。前提：提交答复是否已带版本标识要核（未核）；若无需小改提交口答复，影响契约。

**F. 索赔的批次/项标识唯一性。** ClaimIntake 头注称客户侧唯一性口径属 `PAR-INT-01` 待提供。选项：以（账户, 批次, 项）为自然键，请求键仅作传输重放；或请求键即自然键。倾向前者；需 VE owner 定重复项的答复。

## 3. 对应已有的票

| 票 | 状态（头行） | 是否上线前置 |
|---|---|---|
| first-party-api/00 | resolved（2026-10-19 进 main） | 已完成，是样板 |
| first-party-api/04 | resolved（2026-10-26，`7dc590793`）；写开关退场随 operator-channel/53 | 提交口的前件；五口不依赖，但"无写开关生产形态"要它 |
| first-party-api/01 scope 与 1:N | needs-triage，待 00 落地 | **部分前置**：能力词表要定；1:N 只在取舍 B 选 2 时前置 |
| first-party-api/02 限流、03 可观测 | needs-triage | 非前置（可上线后）；但客户 API 对外无限流是风险，见 §5 |
| first-party-api/05 管理台提交页无凭据 | needs-triage | 非前置 |
| docs-consistency/code/36 撤回读回 | needs-triage，R2；tops-26 评审中 | **撤回口前置** |
| docs-consistency/code/44 撤回与修订重放短路 | needs-triage，"撤回口、修订口接上前必修" | **撤回、修订口前置** |
| docs-consistency/code/99 取锁次序 | needs-triage，R2 | 建议同批，不硬阻（死锁后重发可恢复） |
| docs-consistency/code/98 交付终局不读已成立取消 | needs-triage，R2；tops-26 排在后面 | **取消口前置**（仅当租户用网络服务且登记了取消授权） |
| docs-consistency/code/76 取消冲突无解开面 | needs-triage，R2 | 非硬前置；上线后可见卡住，需运维手册 |
| 新（tops-26 事实①）取消生命周期闸门 | 未立 | **取消口硬前置**，须先立票 |
| demo-chain-gaps/102 索赔首方入口 | needs-triage（M） | 即索赔口本体 |
| ps-port-remainder/03、04；party-commercial-context-gaps/08 | resolved | 修订口的授权与入口机制已就位，缺的是实例半边 |
| first-tenant-runway/09 | resolved | 补充续办已自愈，可用 |
| ps-external-mark-relations/09 | resolved | 一口一词铸造的样板 |
| operator-channel/53 | 另案 | 运营读开关退场，非五口前置 |

## 4. 拆票与顺序

| # | 票（建议名） | 规模 | 依赖 | 并行 | 主要文件 | 归属 |
|---|---|---|---|---|---|---|
| U | 用户裁：A、B、C、D、E、F（见 §5） | - | - | 立即 | 记入 ADR-0139 后续记录 | 用户 |
| P0 | 首方命令面共用底座：五个 Mint 方法骨架、能力词、一份 access_identity 迁移（CHECK 取并集，一次登五词，避免各口抢迁移号）、契约形状版本表 | M | 取舍 A、B | 否（先行） | `accessidentity/customer_client_*.go`、`migrations/access_identity/`、`cmd/parcel-access-register` | tops-20 |
| W | 撤回口：首方 Intake、`DeriveWithdrawalRequestKey`、装配、种子、演示 | M | P0；code/36、code/44（必）、code/99（宜） | 与 S、Cl 并行 | `adapters/http/`（新文件）、`customer_client_envelope.go`、`cmd/parcel-api/endpoints_withdrawal.go` | tops-20 |
| S | 补充口 | M | P0；取舍 E | 与 W、Cl 并行 | 同上 + `endpoints_customer_supplement.go` | tops-20 |
| Cl | 索赔口（= dc/102 扩充"账户核引用"） | M | P0 | 与 W、S 并行 | `visibilityexception/adapters/http`、`.../accessidentity`、`endpoints_claims.go` | tops-20 |
| Am | 修订口 + 请求方映射 + 授权册/允许矩阵种子 | L | P0；code/44（必）；取舍 B | 与 W 并行但共享 dc/44 | `assemble_customer_amendment.go`、PS 适配器 | tops-20（dc/44 部分 tops-26） |
| G | 取消生命周期闸门（含目标属信封账户核） | S-M | 无 | 立即，与一切并行 | `parcelshipment/application/cancel_parcel.go`、`form_parcel_final` | tops-26，需新立 |
| Ca | 取消口：Intake、授权链装配（采用版本源 + 请求方映射）、种子 | M-L | P0；G；code/98；取舍 C | 在 G 后 | `assemble_cancellation.go`、`endpoints_cancellation.go` | tops-20；G、code/98 归 tops-26 |
| R | 客户委托状态读口（取舍 D 选 2） | M | P0；code/36 | 可并行，可后置 | 新契约、`endpoints_shipment_views.go` | tops-20（读模型在 PS 则 tops-26 协助） |
| T | 五口演示实走 + 种子 + 契约（ADR-0140）补章 | S | W、S、Cl、Am、Ca 各自完成 | 随各口增量 | `scripts/demo-seeds`、`docs/api` | tops-20 |

tops-26 队列内：code/36（评审中）、code/44、code/99、code/98（排在后面）、dc/76、新的闸门票 G。tops-20：P0、W、S、Cl、Am、Ca、R、T。

建议顺序：U 与 G、code/36、code/44 同时起 -> P0 -> W、S、Cl 并行 -> Am -> Ca（待 G、98、C 的租户事实）-> R -> T 收尾。

## 5. 风险与需要用户裁的

1. 取舍 A（请求键空间）、B（授权模型，尤其首个租户是否平台型/货代，即是否需要 1:N）、D（首期可否没有客户状态读）、E（基准版本来源）、F（索赔唯一性）。其中 B 决定规模：选 1 总体约 7 票；选 2 增 first-party-api/01 完整实现与合同委派实例，另加 L。
2. 取舍 C 是租户事实：首个租户是否允许客户自助取消、哪些阶段。未给则取消口只能对外标"不可用"，不应对外开放。
3. 修订的商业坐标与允许矩阵声明、合同委派都要租户登记；否则修订口通了也只会停在授权不可用或待复核。
4. 限流/配额（first-party-api/02）未做，客户 API 对外无速率保护；是否接受首个租户在没有限流时上线，需用户定。
5. 账户隔离证据：撤回、取消、补充、修订的目标按（账户, 标识）找，须各口加"他账户标识答找不到"测试；取消命令不带账户是当前最大的隔离缺口，在闸门票 G 之前不要接口。
6. 本次未核实的点：提交答复是否返回版本标识与 `ShipmentRequestID`（影响取消、补充载荷）；追踪视图是否已显示取消；客户 JSON 契约（ADR-0140 / `docs/api`）中五口是否已有草稿。开工前需要核。
7. 规模总估：不含 tops-26 项约 M x 5 + L x 1 + S，选 B=1 时约 7 到 9 个工作单元（并行后关键路径为 P0 -> Am/Ca）；tops-26 另有 5 张。
