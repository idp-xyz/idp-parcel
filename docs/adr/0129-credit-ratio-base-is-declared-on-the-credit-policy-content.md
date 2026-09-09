# ADR-0129：比例额度的基数由信用政策正文自己声明——`CreditLimit` 比例格带封闭集 `CreditRatioBase`（`POSTED_BALANCE` 入账余额 / `PRIOR_PERIOD_CONFIRMED_CHARGES` 上一结算周期已确认费用合计），不给默认、缺席在构造门拒；`settlement-accounting` 凭自己的账本按声明的基数折成金额进授信额度，基数尚无事实时停在`待判断`不折 0 不折无限；`CREDIT_RATIO_BASE_UNDECIDED` 只留给重建门读回的无基数存量正文

Status: Accepted（2026-09-09，通道 1 代裁立票、owner 授权自决口径——裁决方向在票 [wiring-baseline-remainder/10](../../.scratch/wiring-baseline-remainder/issues/10-credit-ratio-base-is-declared-on-the-credit-policy-content.md)「裁决方向」一节；封闭集成员表由本记录按 `/domain-modeling` 一格定。本记录由通道 3 落文并实施。落文时读过：该票全文、ADR-0127 全文（本记录是它决定四留下的那道裁决，不改它的正文）、ADR-0044 / ADR-0126 / ADR-0028 / ADR-0025 / ADR-0054、`party-commercial` `CONTEXT.md` 信用政策那句、`settlement-accounting` `CONTEXT.md` 运营结算余额与对账单那几句、`internal/partycommercial/domain` 的 `credit_limit.go` / `credit_policy.go` / `publication_canonicalization.go` / `publication_vocabulary.go` / `resolution_rehydration.go` / `commercial_registry.go`、`adapters/postgres` 的 `credit_policy.go` / `commercial_resolution.go`、`adapters/http` 的 `publication_draft_payload.go`、`migrations/party_commercial/0020_credit_policy.sql`、`internal/settlementaccounting` 的 `application/apply_pre_acceptance_control.go` / `domain/credit_basis.go` / `domain/credit_exposure.go` / `domain/customer_statement.go` / `adapters/postgres/operational_position.go` / `adapters/postgres/customer_statement.go` / `adapters/partycommercial/credit_basis.go`、`migrations/settlement_accounting` 的 `0002_supplier_bill_and_statement.sql` / `0010_operational_position.sql`。未重读 UC-SA-002 / UC-SA-003 全文；本记录因此只裁**基数在哪一层声明、封闭集有哪几格、SA 怎么取数折算**三件，不动结算政策、不给 PC 合同加任何格）
Date: 2026-09-09

## Context

`party-commercial` `CONTEXT.md` 写「信用政策……按责任法人、业务角色、费用类型、金额或比例形成版本。政策只提供业务判断依据，不直接修改结算余额或形成调整金额」。`CreditLimit` 两格封闭（金额 / 比例），比例格的注释写「比例相对于什么基数由消费方的业务判断给出」；ADR-0127 决定四据此把比例额度停在 `settlement-accounting` 新增的`待判断`格 `CREDIT_RATIO_BASE_UNDECIDED`——不折成金额、不默认，写明「那是 `BD-*` 一类，等它自己的裁决」。

于是今天没有任何一处能声明「比例相对于什么」：比例额度在产品里发布得出来、管理台看得见、经闭包解析交到 SA，然后**永远用不上**。一个「30%」没有分母不是业务判断依据。分母属于同一条政策声明——登记这条政策的人就是能说出「30% 的什么」的人；让 SA 猜，是把租户的商业判断搬进消费方，ADR-0127 决定四拒绝这么做是对的，本记录把那一格补到它该在的地方。

SA 这一侧有什么可作分母，`settlement-accounting` `CONTEXT.md` 已经列好：「运营结算余额必须区分入账余额、当前有效授信额度、冻结金额、已确认未结金额和可用余额」，对账单是「针对一个结算账户和结算周期发布的费用结算文件」，发布后「保存固定单号、费用范围和金额快照」。`operational_balance.posted_minor` 与 `customer_statement` 的费用行都是 SA 自己写下的事实，不必问任何人。

## Decision

**一、基数是信用政策正文的一格，术语「比例基数」（`CreditRatioBase`），封闭集、不给默认；比例格必带、金额格必缺；缺席在构造门拒。** `NewCreditRatioLimit` 自本记录起收基数，集外或缺席答 `ErrInvalidCreditLimit`；`NewCreditAmountLimit` 没有这一格，载荷 / 批文 / 库列上金额与基数并存一律拒（「含则必填、不含则必缺」，ADR-0044 结算选择器同形）。发布路上比例在场而基数缺席由服务端点名 `creditPolicy.ratioBase` 那一格，表单只呈现、不挑、不预选。PCC-1 不换号：`canonicalCreditPolicyBody` 加键 `ratioBase,omitempty`（ADR-0126 决定一「各册接进同一个 `PCC-1` 不换号」同法）；金额正文的规范化文档因此一字不变，既有摘要不动。

**二、封闭集成员由「SA 凭自己的账本与状况登记算得出」一条判据定；成员表与每格的取数路径如下，不在表上的候选不收。**

| 成员 | 领域词 | SA 取数路径 | 基数尚无事实 |
|---|---|---|---|
| `POSTED_BALANCE` | 入账余额——运营结算余额五项之一（SA CONTEXT 原词） | `settlement_accounting.operational_balance.posted_minor`，键取控制请求的结算作用域全部四维（租户、责任法人、结算账户、币种）——作用域不是过滤器而是身份的一部分，少一维就可能拿另一个作用域的钱当分母 | 该作用域未登记运营余额（无行） |
| `PRIOR_PERIOD_CONFIRMED_CHARGES` | 上一结算周期已确认费用合计 | 本账户（租户、结算账户、币种）按 `published_at` 最近的一张已发布对账单，基数 = 其**费用行**金额之和：对账单只纳已确认费用（UC-SA-003），费用行就是那个周期截下来的已确认费用；调整行是调整不是费用，不计。SA 不拥有周期边界（结算归属日与周期由费用类型和合同规则决定），截单留下的快照是它自己手里唯一的周期记录，所以「上一周期」取自快照而不另算 | 从未发布过对账单；或最近一张已作废且尚无替代——作废是整单无效，被作废的数不能再当分母 |

票面候选叫 `DEPOSIT_BALANCE`（预付 / 保证金余额），本记录改名 `POSTED_BALANCE`：SA 没有「保证金」这一格，「预付余额」也不是运营结算余额的五项之一——一个指向 SA 算不出的东西的词，什么都声明不了；术语跟着 SA 算得出的那一项走，两侧的翻译才是一对一。**不收「客户合同声明的基数金额」**：PC 合同今天没有这一格，CONTEXT 没有任何一句说合同有，收它就是给合同长一格，是另一张票。

**三、SA 按声明的基数取自己的数、乘比例、进授信额度；基数尚无事实时停在`待判断`，不折 0 不折无限。** `exposeCredit` 读到比例额度时向新端口 `CreditRatioBaseView.LoadCreditRatioBase(tenant, scope, base)` 取值，三格照 ADR-0054：found=true 带取值；found=false = 本作用域尚无该基数的事实（未登记余额 / 尚无有效对账单），消费方停在新增的 `CREDIT_RATIO_BASE_NOT_ESTABLISHED`，恢复动作是等事实出现（登记余额、发布首张对账单），不是重试也不是补配置；error = 读不回，停在新增的 `CREDIT_RATIO_BASE_UNAVAILABLE`，等重试。两格不并成一格：并了就答不出该等谁。折算在领域（`CreditBasis.LimitOnBase`）：额度 = ⌊基数 × 万分比 ÷ 10000⌋，向下取整到最小货币单位——按比例授信从不多授一个最小单位；**基数为负时额度为 0**——入账余额为负是账户已欠款，账上没有任何可按比例放大的资金，0 是账本如实的答案，不是替取不到的基数补零；折出的 0 让暴露落`业务限制`（`AVAILABLE_CREDIT_INSUFFICIENT`），恢复动作是客户入账，那正是`业务限制`这一格的用途。基数取**当前值**、不按 `asOf` 回溯：与已占用暴露、逾期同一读法（ADR-0127 决定四「已占用暴露与逾期仍取本上下文自己的账本与状况登记」），重放由暴露账本按请求身份幂等交回原暴露，不靠重算。`CREDIT_RATIO_BASE_UNDECIDED` 保留：它的唯一来路是重建门读回的、本记录之前登进去的无基数比例正文（`0020` 存量行、闭包快照存量项）——重建门按 ADR-0028 只校验不重算，那一格是它的合法答案；无租户故理论上为零。

**四、SA 侧的封闭集是镜像不是引用；新依赖 mandatory。** `settlement-accounting` 自有 `CreditRatioBase`，不 import `party-commercial`（纪律同 `SettlementMethod` / `CreditBasis`），消费侧适配器 `creditBasisFrom` 逐格全函数译过来（ADR-0025），集外走 error 不吸收；未声明基数的比例额度译成 SA 侧的「未声明」形，编排据它停 `CREDIT_RATIO_BASE_UNDECIDED`。`ApplyPreAcceptanceControlDeps` 新增 `RatioBases`，与其余依赖同一道构造门（`ErrNilDependency`）——ADR-0127 决定五写明的 contract 段纪律已随票 09 收齐，本记录不再开 expand 段；PS 那份直接构造 `Deps` 的夹具随本记录同笔补替身。

**五、`0020` 加一列、发布路各加一格、词表加一集。** PC 迁移 `0029` 给 `party_commercial.credit_policy` 加 `ratio_base text`：可空**只为存量行**，CHECK 守「金额行必空、非空必在集合内且非空白」而不追溯 NULL 的比例行；读回时 NULL 经重建门（`RehydrateCreditRatioLimit`）如实读回为未声明，构造门（`NewCreditRatioLimit`）不收它。受控批文 `creditPolicyBodyDocument`、表单载荷 `CreditPolicyBodyPayload`、闭包快照 `creditBasisDocument` 各加 `ratioBase` 一键；词表读口 `PublicationVocabulary(CreditPolicyObject)` 答 `ratioBase` 一集（码从 `CreditRatioBase` 的接受判据逐值列出，不另抄常量表）；读面信用政策册的额度带基数。`ViewRevision` 对比例额度的信用政策把基数写进摘要——只改基数、不动版本正文时解析身份仍变（ADR-0044 第四条同款）；金额额度那一段一字不变。

## Consequences

- 解析身份：登记了比例额度信用政策的范围 `VIEW-` 换值；无租户故无迁移。
- 存量：`0020` 里 `limit_ratio_bps` 非空而 `ratio_base` 为 NULL 的行、闭包快照里带 `ratioBasisPoints` 而无 `ratioBase` 的项，读回为未声明，SA 走 `CREDIT_RATIO_BASE_UNDECIDED`；要让它们用得上只能发新版本，不改写已固定的快照。
- `NotFormedReason` 新增 `CREDIT_RATIO_BASE_UNAVAILABLE` / `CREDIT_RATIO_BASE_NOT_ESTABLISHED` 两格；SA 无迁移（两格基数都读既有两表）。
- 用 `PRIOR_PERIOD_CONFIRMED_CHARGES` 的新客户在首张对账单发布前一律停`待判断`：这是该政策选择的诚实后果，租户给新客户的首期授信该用金额格。
- **越权风险点（供评审与 owner 复核）**：① 「基数属政策正文」是从 CONTEXT「只提供业务判断依据」推出的，CONTEXT 此前没有逐字写基数，本记录补半句；② 成员表按「SA 能自算」选，「合同声明基数」明确不收，将来有租户提出时另立；③ `PRIOR_PERIOD_CONFIRMED_CHARGES` 的定义有三处是本记录的裁定——对账单快照作周期代理、结算账户蕴含责任法人（对账单按账户 + 币种键入而作用域三维）、费用行合计不含调整行；产品若另有周期定义或要含调整净额，走 supersede；④ 负入账余额折 0 是裁定，另一种读法（负基数停`待判断`）会让「客户欠款」长得像「配置没齐」；⑤ 两格新 `NotFormedReason` 未经产品单独裁，与 ADR-0127 那三格同一性质（机制半边的结果代数，各答一种恢复动作）。

## Alternatives considered

- **基数由 SA 侧按租户配置。** 否决：那是把「30% 的什么」从政策声明搬进消费方——ADR-0127 决定四拒的正是这个；且 SA 会为一份政策长出第二处声明。
- **基数可选，缺席默认 `POSTED_BALANCE`。** 否决：默认值是替租户拍板一个商业判断，红线「未确认参数不写死为生产默认」；缺席在构造门拒，登记方必须说出来。
- **以可用余额作基数。** 否决：可用余额 = 入账余额 + 当前有效授信额度 − 冻结 − 已确认未结，分母里含着要算的那个额度，循环。
- **「上一结算周期」按结算政策的周期定义算。** 否决：SA 不拥有周期定义，拿来算就得给 SA 一条新缝去问 PC 周期边界，那是另一张票；对账单是 SA 自己留下的周期快照，先用它。
- **基数取不到时折 0 或折无限。** 否决：折无限是 CONTEXT 禁的默认信用通过；折 0 让「没登记余额」与「余额为零」同形，租户会去查一份其实不存在的余额。
- **负入账余额停`待判断`。** 否决：欠款是业务状态不是缺配置，`待判断`会把它说成「等谁把什么补齐」，而该等的是客户入账——`业务限制`才是这句话的格。

## Links

- [ADR-0127](./0127-credit-basis-is-handed-over-by-the-commercial-closure-resolution.md)：决定四留下的「基数今天未裁」——本记录是那一裁；决定五的 contract 段纪律沿用
- [ADR-0044](./0044-settlement-basis-adopts-via-settlement-policy.md)：「含则必填、不含则必缺」与「只改正文不动版本时解析身份仍变」的先例
- [ADR-0126](./0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md)：`PCC-1` 加键不换号
- [ADR-0028](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)：重建门只校验不重算——存量无基数正文如实读回
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：消费侧适配器全函数只译不判
- [ADR-0054](./0054-pre-acceptance-control-policy-view-has-an-unconfigured-grade.md)：`CreditRatioBaseView` 三格的出处
- [settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)：运营结算余额五项、对账单、结算归属日
- [UC-SA-003](../application/settlement-accounting/UC-SA-003-CUT-OFF-PUBLISH-AND-RECONCILE-CUSTOMER-STATEMENT.md)：对账单只纳已确认费用
- 票 [wiring-baseline-remainder/10](../../.scratch/wiring-baseline-remainder/issues/10-credit-ratio-base-is-declared-on-the-credit-policy-content.md)
