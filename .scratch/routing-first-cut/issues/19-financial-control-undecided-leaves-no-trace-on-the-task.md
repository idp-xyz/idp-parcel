# 19 财务控制未形成时消费方整笔回滚：任务与可达性判断都不落，动线上看不出停在哪一格

Category: enhancement
Status: 完工，待评审与重放——2026-10-13 通道 3 接续会话收口（派单 `task-6ebd4303` ← 通道 1，接替 `task-a95f74d3`），代码 tip `8dce3223`、清点 `344e6b2d`，见文末「完成记录」。此前 in-progress——2026-10-12 通道 3 认领（派单 `task-a95f74d3` ← 通道 1），分支 `mcp3-rfc19` 基 `1e02d2ea`，隔离树 `/home/tops/workspace/idp-parcel-mcp3-rfc19`；按「分诊」裁定分笔。此前 ready-for-agent——2026-10-12 通道 3 分诊（派单 `task-85d7350d` ← 通道 1），见文末「分诊」。此前 needs-triage——2026-10-11 通道 1 立（用户授权自决），出自 [16](16-demo-tenant-pre-acceptance-financial-control-cells.md) 判据二的取证与其「评审 ← 通道 5」Spec 非阻断
Blocked by: 无
地盘（按「分诊」裁定分笔）：
- `internal/parcelshipment`：`ports` 的 `PreAcceptanceControlOutcome` 增「未配置」一格；`application` 的 `controlStallReasons`、新未决原因与 `resumePath()`、控制停那一支经 `awaitOperatorRegistration`、`undecided` 带提供方原因；`adapters/settlementaccounting` 的原因译法；`adapters/inbox` 收结算登记信封的续办门（照 `OperatorRegistrationCompletedConsumer`）。
- `internal/partycommercial`：`announceOperatorRegistrations` 扩通道，`ports.OperatorRegistrationKind` 增格。
- `internal/settlementaccounting`：结算账户登记与登记行同一事务交信封的交接口与 Outbox 适配器。
- 装配：`cmd/parcel-dispatch`、`cmd/parcel-settlement-register`；做可选那一笔时加 `cmd/parcel-commercial` 的解析键登记。
- 架构门禁：`internal/architecture/partition_subject_registry_test.go` 只加本票的行——SA 新交接口一行、PC「参数已登记」那行加裁段（2026-10-12 开工核码时报出、通道 1 认可，见「分诊」下「开工核码更正」）。
- 文档：两处带日期的补记（见「现象」末条）；CONTEXT-MAP 补 SA → PS「参数已登记」类信封那条关系。
出处：16「分诊裁定」第 4 条（「要不要立票，留到下一次走动线时定」）；16 完成记录判据二的三行对照表；16 Comments「评审 ← 通道 5」与通道 1 处置。

## 现象（取自 16 的完成记录与评审，钉 `f84515c2`；证据层级 `S`，立票时未另跑）

- 原样种子（停在时点格）：`acceptance_processing_attempt` 落一行 `FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED` / `OPERATOR_REGISTRATION`，可达性判断也落一行。
- 越过时点之后（16 的「只改时点」与「两格都登」两次）：dispatch 三次投递都是 `dispatch.consumer_undecided`，`stage FINANCIAL_CONTROL_JUDGMENT, reason FINANCIAL_CONTROL_NOT_FORMED`；未决整笔回滚，任务行与可达性判断都是零行。
- 所以 16 分诊裁定第 4 条的前提「作用域、金额两停在任务上都记 `FINANCIAL_CONTROL_NOT_FORMED`」不成立：任务上什么都不落。作用域停与金额停在适配器里各有原因码，到 `AdvanceFinancialControlJudgmentHandler` 都折成 `FINANCIAL_CONTROL_NOT_FORMED`；16 只能靠未入库的探针分出停在哪一格。
- 停点前移之后，两处带日期的文档补记写的仍是 16 之前的状态：[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md) 2026-09-30 补记（票 psb/06 第 1 项）末句「演示种子的可达性格采用了这一形态，财务控制格没有」；[合成演示动线](../../../docs/design/synthetic-demo-journey-script.md)「墙三」下 2026-10-10 补记（票 rfc/11）写的停点 `FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED`。

## 待分诊

- 消费方未决整笔回滚出自哪条规则（先找 CONTEXT 或 ADR 的出处）；若是有意为之，「看不出停在哪」靠什么补——任务尝试留痕、只进日志，还是读面另答。
- `assessment.Reason` 要不要带进续办，让作用域停与金额停在任务上分得开。
- 那两处文档补记怎么跟上：补一条新的带日期补记，还是归某张已有的文档票。
- 与 [psb/06](../../product-strategy-boundary/issues/06-ps-acceptance-and-label-selection-judgment-methods.md) 第 2 项（`Amounts` 的估价方法）的先后：越过格 5 之前，演示动线都停在这一格。

## 分诊（通道 3 · 2026-10-12 · 钉 main `ff3b01fa` · 派单 `task-85d7350d` ← 通道 1）

### 取证

读码钉 `ff3b01fa`。另照[合成演示动线](../../../docs/design/synthetic-demo-journey-script.md)「前置」在 55432 的一次性库 `idp_mcp3_psb06a` 上重走一遍，走完已删库，证据层级 `S`：

- 原样 `seed.sh` 灌库；`parcel-api` 与 `parcel-dispatch` 按 `ff3b01fa` 构建，dispatch 带 `IDP_PARCEL_DISPATCH_MAX_ATTEMPTS=3`、`IDP_PARCEL_DISPATCH_RETRY_AFTER=10s`。`submit-one-shipment.sh` 于 2026-10-12T07:04:03Z 提交一笔，委托 `SHR-C7OF6LBJUCNPL4RYAI6JDUNJOI`。
- 同一封 `parcel-shipment.shipment-request.submitted` 投递三次，次次 `dispatch.consumer_undecided`：`stage FINANCIAL_CONTROL_JUDGMENT, reason FINANCIAL_CONTROL_NOT_FORMED`。预算烧完，`bento.outbox` 那一行落 `ABANDONED`（attempt 3，failure_code `dispatch.consumer_undecided`）。
- 任务上什么都没落：`parcel_shipment` 下 `acceptance_adopted_resolution`、`acceptance_reachability_judgment`、`acceptance_financial_control`、`acceptance_financial_control_item`、`acceptance_processing_attempt` 与 `network_routing.reachability_judgment` 都是零行，委托停在`已提交`。`settlement_accounting.settlement_account` 有 `SYN-SETTLEMENT-ACCOUNT-01` 一行。
- 这一跑没放探针，停在金额那一格是推出来的：16「两格都登」那次探针读回 `CONTROL_AMOUNT_NOT_CONFIGURED`；`f84515c2..ff3b01fa` 之间，`internal/parcelshipment/application`、PS 的结算与计价两个消费侧适配器、`adapters/inbox`、`internal/settlementaccounting/application`、`internal/platform/dispatch` 都没改，`cmd/parcel-dispatch/assemble.go` 在受理链那一段只改了路由成本的装配（`routeCosts(db, clock)`）。种子的结算段没动，账户 01 的解析键只多了一项必需依据 `CUSTOMER_SERVICE_RULE`，本跑也已越过商业依据、停在财务控制。

### 一问：整笔回滚出自哪条规则，看不出停在哪靠什么补

出自 [ADR-0081](../../../docs/adr/0081-acceptance-judgment-is-envelope-driven.md) 决定三「任一步未决即整笔回滚等重投」；[ADR-0094](../../../docs/adr/0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 决定一、三把它细化成按 `ResumePath` 折：`等待内部续办`回滚重投，其余几格提交入账。对会自愈的依赖这是有意的：任务上不落是设计，日志里的 stage 与 reason 就是 [ADR-0095](../../../docs/adr/0095-undecided-stage-and-reason-surface-in-two-layers.md) 决定二给那一格的可观测性。

这一停却不属会自愈的那一类。`controlStallReasons` 把 `PreAcceptanceControlNotFormed` 一律折成 `FinancialControlNotFormed`，后者在 `resumePath()` 里落 default 的`等待内部续办`；而折进去的原因里有一族是「未配置」——PS 适配器自己的 `CONTROL_SCOPE_NOT_CONFIGURED`、`CONTROL_AMOUNT_NOT_CONFIGURED`，以及结算交回、经适配器冠 `SA-` 的 `SA-CONTROL_POLICY_NOT_CONFIGURED`、`SA-CREDIT_BASIS_NOT_CONFIGURED`、`SA-CREDIT_RATIO_BASE_UNDECIDED`。按 ADR-0094 决定二的入格判据（重试、客户补件、人工复核都推不动，只有一次登记推得动），它们该落`等待运营登记`。落在内部重试上，重演的就是 ADR-0094 Context 那一幕：信封烧完预算落 `ABANDONED`，委托停在`已提交`，配置补齐之后也没有任何东西再驱动它。上面的实跑正是这一幕。

所以补法不是改日志，也不是读面另答，而是兑现 ADR-0095 决定一：把未配置这一族挪进`等待运营登记`、提交入账，`recordAttempt` 写下的原因、续办路径与续办引用就留在库里，可查、可按原因统计。挪格受 ADR-0094 决定四约束：每一格都要与它的续办触发同笔落地，否则只是把 `ABANDONED` 换成更安静的永久停滞。

### 二问：`assessment.Reason` 要不要带进续办

要，分两层：

1. **端口按恢复动作分格。** `PreAcceptanceControlOutcome` 增一格「未配置」。PS 的结算适配器把触发已到位的那几个未配置原因译到这一格，其余留在 `PreAcceptanceControlNotFormed`；PS 应用层不解析提供方的原因串，判据同 ADR-0094 决定一「不按 `JudgmentPendingReason` 的具体取值折」：按恢复动作分格，不按名字分格。编排侧增一个未决原因（名如 `FinancialControlNotConfigured`），在 `resumePath()` 里显式列进`等待运营登记`；控制停那一支像时点那一支一样，先经 `awaitOperatorRegistration` 落等待态（ADR-0094 决定五）。
2. **续办引用带原因。** 控制停那一支调 `undecided` 时把 `assessment.Reason` 放进范围参数，与商业依据那一支在 `formAdoptedBasis` 里把 `resolution.Reason` 放进范围同一纪律。作用域、金额、策略、授信几种停于是各拿一条续办引用。

各格今天有没有续办触发——ADR-0094 决定四要的就是这一栏：

| 原因 | 恢复动作 | 触发 |
|---|---|---|
| `SA-CONTROL_POLICY_NOT_CONFIGURED` | 商业侧登记受理前财务控制策略（`PAR-COM-15`） | 无。`announceOperatorRegistrations` 只对时点策略通道交「参数已登记」；要扩到这一类声明的通道（开工时核 SA 的策略读口读的是哪条），`OperatorRegistrationKind` 增格 |
| `SA-CREDIT_RATIO_BASE_UNDECIDED` | 发新版信用政策正文 | 同上，扩到信用政策正文那条通道 |
| `SA-CREDIT_BASIS_NOT_CONFIGURED` | 解析键与信用政策正文都登上 | 两处都要发：信用政策那半同上；PS 的解析键登记（`parcel-commercial register-resolution-key`，登记面在 PS）今天不发信封 |
| `CONTROL_SCOPE_NOT_CONFIGURED` | 合着两种来由（`PolicyBackedControlScopeSource.FormControlScope`）：解析不含结算政策，或结算账户册里查不到这一户 | 后一种的恢复动作是结算账户登记，`RegisterSettlementAccountHandler` 今天不发信封；前一种归商业侧补结算政策。挪这一格要两种触发都在，或先在适配器里把两种来由拆成两个原因码，只挪触发已到位的那一种 |
| `CONTROL_AMOUNT_NOT_CONFIGURED` | 登记估价形态的采用 | 要等 ADR-0179 的采用登记册。不在本票，随 [psb/25](../../product-strategy-boundary/issues/25-pre-acceptance-estimate-formed-by-settlement.md) 挪 |

`*_UNAVAILABLE` 一族与 `SA-CREDIT_RATIO_BASE_NOT_ESTABLISHED` 不在本票，留在原格：前者会自愈；后者等的是事实出现（运营余额、有效对账单），归不归第四格本票不裁。

### 三问：两处文档补记怎么跟上

补新的带日期补记，不改旧的——开发主线那条自己写着「上表单元格不改写」，动线「墙三」下 2026-09-24 那条写着「上两段写于当时，原样保留」，同一惯例。归本票，不另归文档票：本票地盘本来就列着它们。写钉 `ff3b01fa` 的实测：

- 开发主线 2026-09-30 补记之后：演示种子财务控制那一格的时点自 16 进 main 起也采用了「提交接收」，种子还登了演示租户的结算账户；越过时点之后停在金额未配置。
- 动线「墙三」2026-10-10 补记之后：停点前移，不再是 `FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED`，而是财务控制那一步的 `FINANCIAL_CONTROL_NOT_FORMED`（金额未配置）；投递烧完预算落 `ABANDONED`，任务上什么都不落。

本票后几笔不挪金额那一格，所以补记写下的事实到 psb/25、psb/26 落地前都成立；那两张落地时各自再补一条。

### 四问：与 psb/06 第 2 项的先后

互不阻塞。本票不依赖 [ADR-0179](../../../docs/adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 定的归属与形态。但单落本票，演示动线看不出变化：演示租户越过了作用域那一格、停在金额那一格，而适配器在问结算之前就答了金额未配置，策略与授信两格这一跑根本问不到。金额那一格要等 psb/24、25、26 依次落地，后两张还在等用户接受 ADR-0179。

两边只在一处交叠：psb/25 挪金额格时复用本票拆出的「未配置」端口格与未决原因。哪张先落哪张拆，另一张接着用；psb/25「金额那一格的续办」那一步已写明这一句。

### 裁定

1. 转 ready-for-agent，Blocked by 无；地盘见票头。
2. 分笔：
   - **只动文档的一笔**：三问那两条补记，可以先于代码单独进 main。
   - **拆格一笔**：二问的两层，连同 `SA-CONTROL_POLICY_NOT_CONFIGURED`、`SA-CREDIT_RATIO_BASE_UNDECIDED` 两格与它们在商业侧的触发，同笔落地。
   - **结算账户一笔**：结算账户登记与登记行同一事务发「参数已登记」类信封，PS 收它重驱，挪 `CONTROL_SCOPE_NOT_CONFIGURED` 里结算账户那一种；CONTEXT-MAP 补 SA → PS 这条关系。
   - **可选一笔**：PS 解析键登记也发信封，`SA-CREDIT_BASIS_NOT_CONFIGURED` 的两半触发到齐才挪。不做时它留在原格，完成记录里写明。
3. 不做：不挪金额那一格；不改 `*_UNAVAILABLE` 一族；不动 `undecidedDisposition` 的分派（新原因经 `resumePath()` 落进已有的`等待运营登记`格，消费门照旧按恢复动作折）；不另开读面。

### 完成判据

- 作用域未配置（结算账户没登记）：提交后委托进`等待运营登记`，处理尝试落库，带新未决原因与按原因派生的续办引用；信封入账，不落 `ABANDONED`。登记结算账户之后信封到达，重驱同一委托，越过作用域、停到金额。带 DSN 的真库用例，写明 `-v` 下为 `PASS` 不是 `SKIP`。
- 控制策略未配置：商业侧发布受理前财务控制策略之后，同样重驱越过这一格。
- 作用域停、金额停、策略停三者的续办引用两两不同。
- 会自愈的 `*_UNAVAILABLE` 与金额未配置照旧回滚重投，既有用例绿；`TestEveryPendingReasonHasAStringAndAResumePath` 绿。
- 两条补记在现行文档里。
- 架构门禁绿；机制清点重生成并提交；带 DSN `go test -p 1 -count=1 ./...` 绿。

### 开工核码更正（通道 3 · 2026-10-12 · 钉 `1e02d2ea`；2026-10-13 接续会话补记）

开工时对着代码复核二问那张表，三处与上文不符。上文不改，更正记在这里；当晚已分两条报通道 1，第二条收回了第一条里的「可选笔改为必做」。

1. **漏的那一格。** 表里 `SA-CONTROL_POLICY_NOT_CONFIGURED` 只写了「商业侧登记受理前财务控制策略」一种恢复动作。它还有一种来由：登记的解析键没要求 `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY`——闭包只采用键里要求的类，settlement-accounting `adapters/partycommercial` 的 `requiredPolicyFrom` 在「闭包没采用接受前财务控制策略」那一格同样答未配置。作用域那一行「前一种归商业侧补结算政策」同理：解析不含结算政策是键没要求 `SETTLEMENT_POLICY`，补登一份结算政策推不动它。
2. **可选那一笔不做。** 解析键按（租户 + 客户）只有一行，改登一律答内容冲突、不覆盖（parcel-shipment `adapters/postgres` 的 `CommercialResolutionKeyStore.RegisterResolutionKey`），所以「键没要求某类依据」这一来由没有任何登记动作推得动，给解析键登记发信封没有受益者。`SA-CREDIT_BASIS_NOT_CONFIGURED` 因此留在`未形成`：走到控制这一步时闭包已唯一解析，没采用信用政策只能是键没要求它。
3. **拆码口径。** 适配器分得开的拆出新码、留原格，只挪触发已到位的那一种（ADR-0094 决定四）。作用域停拆成两码：`CONTROL_SCOPE_NOT_CONFIGURED` 只留给账户没登记（[ADR-0158](../../../docs/adr/0158-settlement-account-register-is-an-immutable-tuple.md) 决定三钉的就是这个码），挪`未配置`，触发是结算账户一笔；解析未采用结算政策换新码 `CONTROL_SCOPE_SETTLEMENT_POLICY_NOT_ADOPTED`，留`未形成`。`SA-CONTROL_POLICY_NOT_CONFIGURED` 在 PS 适配器分不开，整格照裁定挪（合同声明、策略正文两条触发到位），键那一来由记「越权风险点 · 待 owner 复核」，分不开的理由写在完成记录。

地盘因此只多出 `internal/architecture/partition_subject_registry_test.go` 一处，已补进票头。

## 完成记录（通道 3 · 2026-10-13 · 分支 `mcp3-rfc19` 基 `1e02d2ea`，代码 tip `8dce3223`）

推送方自审不算非作者评审；评审与重放归通道 1。

### 各笔

| SHA | 内容 |
|---|---|
| `e130fd22` | 认领 |
| `afc162e5` | 只动文档的一笔：开发主线与动线「墙三」各补一条 2026-10-12 补记 |
| `716d3d5e` | 拆格一笔：端口增`未配置`格，`FinancialControlNotConfigured` 落`等待运营登记`，续办引用带提供方原因；PC 增 `PRE_ACCEPTANCE_CONTROL_POLICY`、`CREDIT_POLICY` 两类「参数已登记」 |
| `eca14e52` | 结算账户一笔的 PS 半边：作用域停拆码（前任会话未成笔的现场，接续前原样成笔） |
| `8dce3223` | 结算账户一笔：SA 登记同事务交「结算账户已登记」，PS 新登记续办门与路由，分区登记表两行，CONTEXT-MAP 一条 |
| `344e6b2d` | 机制清点在 `8dce3223` 干净检出上重生成 |

可选那一笔不做，理由见「开工核码更正」第 2 条。重放进 main 时 `eca14e52` 与 `8dce3223` 要一起进：前者把账户没登记挪进`未配置`，它的续办触发在后者。

### 完成判据

- ✅ 作用域未配置（结算账户没登记）。`cmd/parcel-dispatch` 的 `TestAScopeWaitIsResumedByTheSettlementAccountRegistrationEnvelope`：控制那一口装生产的 PS 适配器、作用域源与账户目录，账户目录读真 SA 账户册，只有金额那一口是计数替身。提交门 inbox 1 行；`acceptance_processing_attempt` 落 `FINANCIAL_CONTROL_NOT_CONFIGURED` / `OPERATOR_REGISTRATION` / `CONT-` 续办引用 1 行；委托列在`等待运营登记`队列里。真 SA 登记编排铸出 `settlement-accounting.settlement-account.registered` 1 封，结算账户那一份登记续办门重驱同一委托：问到了金额（作用域已形成），停在 `FINANCIAL_CONTROL_NOT_FORMED`，整封交回未决哨兵、账本无痕等重投。带 DSN `-v` 为 `PASS`，不是 `SKIP`。登记口装配另由 `cmd/parcel-settlement-register` 的 `TestSettlementAccountRegistrationHandsOffItsEnvelopeInTheSameTransaction` 真库证：一行一封、重放不翻倍、冲突不交、超长账户归用法格且行不落。
- ✅ 控制策略未配置。`TestAControlPolicyWaitIsResumedByTheControlPolicyRegistrationEnvelope`（`716d3d5e`；控制端口为替身，PC 交接适配器为真；本会话带 DSN `-v` 复跑 `PASS`）。PC 发布编排对两条控制策略通道交 `PRE_ACCEPTANCE_CONTROL_POLICY`：`TestPublishingControlPolicyAndCreditPolicyDeclarationsHandsOffTheirRegistrations`；SA 原因译进`未配置`格：`TestNotFormedReasonsLandOnTheCellOfTheirRecoveryAction`。
- ✅ 作用域停、金额停、策略停三者的续办引用两两不同：`TestEachControlStallCarriesItsOwnContinuation`。作用域与策略同落`未配置`、同一个未决原因，只靠提供方原因引用分开；金额落`未形成`。
- ✅ 会自愈的 `*_UNAVAILABLE` 与金额未配置照旧回滚重投，既有用例绿。`*_UNAVAILABLE` 落`未形成`由 `TestNotFormedReasonsLandOnTheCellOfTheirRecoveryAction` 证；金额未配置回滚重投见第一条那条用例的重驱一拍；`TestEveryPendingReasonHasAStringAndAResumePath` `-v` 为 `PASS`。
- ✅ 两条补记在现行文档里：`afc162e5`。
- ◑ 架构门禁绿、机制清点重生成并提交：`./internal/architecture/...` 带 DSN 绿，分区主体登记两查、PBC-08 无事务证据、生产接线棘轮都过；清点 `344e6b2d`。带 DSN `go test -p 1 -count=1 ./...` 归推送方重放后那一次，本会话跑的是动过的包及其反向依赖（见下）。

### 验证（作者那层，go1.26.8，证据层级 `S`）

- 改过的 `.go` 经 `gofmt -w` 后 `gofmt -l` 空，无 CR、无 BOM；`go build ./...` 退 0；`go vet ./...` 退 0。
- 带 `IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`，在成笔前的工作树上（内容即 `8dce3223`）跑 `go test -count=1 -p 1`：分支自 `1e02d2ea` 起动过的 12 个包及其反向依赖，用 `go list -test` 的 `Deps` 反查，共 52 包，含 `cmd/parcel-api`、`cmd/parcel-commercial`、`cmd/parcel-dispatch`、`cmd/parcel-settlement-register` 与 `internal/architecture`。48 ok / 0 FAIL / 4 无测试文件，退 0。
- `-v` 单跑上面判据里点名的真库用例与 `8dce3223` 新增的用例，全为 `PASS`，无 `SKIP`。

### 判断项

1. **两条新码的去处。**
   - `CONTROL_SCOPE_SETTLEMENT_POLICY_NOT_ADOPTED`（PS 适配器自有码，`eca14e52`）：停在`未形成`，即 `FINANCIAL_CONTROL_NOT_FORMED` / `等待内部续办`，回滚重投、烧完预算落 `ABANDONED`，与拆码之前同一格。推得动它的只有让解析键要求 `SETTLEMENT_POLICY`，而键改登即内容冲突——今天没有产品动作推得动它，要不要开一条归 owner。
   - `FINANCIAL_CONTROL_NOT_CONFIGURED`（编排新未决原因，`716d3d5e`）：停在`等待运营登记`，提交入账、处理尝试留痕。推得动它的是三类登记信封，都按租户重驱整张队列、消费门不认原因名字：PC「参数已登记」的 `PRE_ACCEPTANCE_CONTROL_POLICY`（合同上的接受前控制声明、策略正文两条通道）与 `CREDIT_POLICY`（信用政策正文），以及 SA「结算账户已登记」（`8dce3223`）。落进这一格的提供方原因是 `CONTROL_SCOPE_NOT_CONFIGURED`、`SA-CONTROL_POLICY_NOT_CONFIGURED`、`SA-CREDIT_RATIO_BASE_UNDECIDED`。
2. **「结算账户已登记」契约**（两边各写各的字面，原文记在这里）：类型 `settlement-accounting.settlement-account.registered`，来源 `idp-parcel/settlement-accounting`；ID 为 `outboxintent.FingerprintEventID("settlement-account", 租户, 账户)`；Scope 与分区键为租户；Subject 为 `SETTLEMENT_ACCOUNT/<账户>`；载荷只两键 `tenantId`、`registrationKind`（取 `SETTLEMENT_ACCOUNT`），与 PC「参数已登记」同形，PS 两份登记续办门共用一份译码。`已登记`与`已存在`都交、`冲突`不交；账户标识长到主题过不了框架上限时包成 `ErrSettlementAccountHandoffRejected`，登记口归用法格（同 sa-cc/32 裁决 2）。
3. **登了账户之后那一封可能烧完预算。** 重驱越过作用域之后停在金额那一格，那一格仍是内部重试，所以「结算账户已登记」那一封会整封回滚重投到预算耗尽、落 `ABANDONED`；委托留在`等待运营登记`队列里不受影响，下一封登记信封照样驱得到它。金额格随 psb/25 挪进`未配置`之后这一封会入账。演示种子先登账户再提交，今天的动线不经过这一幕。

### 越权风险点 · 待 owner 复核

1. **`SA-CONTROL_POLICY_NOT_CONFIGURED` 整格挪进`等待运营登记`**（`716d3d5e`；本会话按通道 1 的口径提醒复核，分不开，不改）。SA 的 party-commercial 适配器（`PreAcceptanceControlPolicy.LoadControlPolicy` 与 `requiredPolicyFrom`，钉 `8dce3223`）在五种情形下答未配置：合同声明未登记、策略正文未登记——这两种有触发；闭包没采用接受前财务控制策略（解析键没要求该类，或合同没绑）、正文对本费用范围一项控制都没写（同版本正文改登答 `PreAcceptanceFinancialControlPolicyContentConflict`）——这两种推不动；合同要求控制而闭包没采用结算政策——生产路径上走不到，作用域那一步先停在 `CONTROL_SCOPE_SETTLEMENT_POLICY_NOT_ADOPTED`。**分不开的理由**：PS 适配器只看得到 SA 的封闭原因 `CONTROL_POLICY_NOT_CONFIGURED`。SA 的读口 `PreAcceptanceControlPolicyView.LoadControlPolicy` 把五种情形交成同一个 `configured=false`，`ApplyPreAcceptanceControlHandler` 再折成同一个 `ControlPolicyNotConfigured`；PS 的 `CommercialBasisSnapshot` 也不带「闭包采用了哪几类依据」，PS 适配器没有第二个来源可分。要分开，得把 SA 读口的布尔换成按恢复动作分格的落点（形照 `eca14e52` 的 `ControlScopeGap`），并给 SA 的 `NotFormedReason` 增格——那是 settlement-accounting 的语言与端口，不在本票地盘。取舍：不挪，推得动的两种来由照旧烧完预算落 `ABANDONED`、任务上什么都不落；挪了，推不动的两种成了查得到、等不来重驱的等待（ADR-0094 决定四说的那一种）。要 owner 定：接受这个取舍，还是另立 SA 票把读口拆开。
2. **分区主体同队**（`8dce3223`）：SA「结算账户已登记」与 PC「参数已登记」同取租户字符串，进同一条队，让同一租户的重驱一轮接一轮、不并发去推同一份委托。分区登记表两行按显式待裁写，没有裁定记录。
