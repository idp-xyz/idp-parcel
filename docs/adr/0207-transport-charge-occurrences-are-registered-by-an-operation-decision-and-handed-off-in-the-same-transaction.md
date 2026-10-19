# ADR-0207：运输收费发生项由运营决定显式登记——事实取自事实本身，采购上下文由登记方给出、经协议核对，发生条件先由登记者断言，已形成信封与发生项同笔交出

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/27，代决出自 `.scratch/demo-chain-gaps/tf-chain-plan.md` 的「07 运输收费发生项」一节与「需要的 ADR」里 ADR-0207 一条。落在 [ADR-0098](./0098-a-failed-attempt-charge-occurrence-is-not-formed-inside-the-pickup-orchestration.md) 决定二之内，其决定一、三不变。越权风险点见文末，待 TF owner 复核，第 1、4 点同时待 PC owner、第 3 点同时待 SA owner。）
Date: 2026-10-19

## Context

- 运输履约 `CONTEXT`「运输收费发生项与成本来源」一节：只有满足当前采用协议所需事实条件的明确范围，才能形成运输收费发生项；每个发生项固定采购责任法人、服务提供方、采用的供应商协议、原因、唯一主要业务范围、成员、数量与单位、业务时间、有效性和来源事实，不能按当前伙伴、线路或报表分组临时补齐。
- 发生项登记册（迁移 `transport_fulfillment/0007_transport_charge_occurrence.sql`）与领域构造早已齐备，生产上却没有任何写入口：ADR-0098 裁了失败尝试费不在揽收编排里形成、采购上下文由调用方显式给出，又在 Consequences 里写明「生产触发链仍未接通」。四种原因里只有失败尝试有领域构造门，实际履约、订舱、取消都没有；也没有已形成的信封，结算等不到发生项（票 demo-chain-gaps/07 的盘点）。
- 下游已经就位：结算按 [ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md) 消费 `transport-fulfillment.charge-occurrence.formed`，按三维引用回读发生项并发起 BUY 评价；它在决定六里把 `agreement_ref` 的形状定给了生产方。参与方与商业开了按（租户、对象、版本标签）点读供应商协议版本的窄口，运输履约有了核协议的消费侧适配器（票 demo-chain-gaps/22）。
- 供应商协议正文今天只有供应商、法人、范围与采购定价方案，**没有发生条件的文法**；那一格归 ADR-0209（票 demo-chain-gaps/29）。
- 演示租户的干线 `SZX-GATE→SIN-HUB` 外包给 `SYN-PARTY-CARRIER-TRUNK-01`、采用 `SYN-SUPPLIER-TRUNK-CN-SG-01` 的 `v1`（方案跨票通则 X1，租户取值）。

## Decision

**一、一个登记用例覆盖各原因，事实依据、主要业务范围与业务时间都取自事实本身。** 登记方指名事实，用例取回事实再定这三格；调用方报来的只会是第二个口径，所以命令里没有它们。

- **失败尝试**：指名一次揽收尝试（按尝试来源键）或一次派送尝试（按尝试身份）两者之一，外加那一次到场里恰一个对象。派送尝试经交付生效已在用的 `DeliveryAttemptView` 读，不建新存储。依据写成 `ATTEMPT-RESULT/<尝试>/<对象>`（揽收）或 `DELIVERY-ATTEMPT-RESULT/<尝试>/<对象>`（派送），两侧分写前缀：两侧的尝试身份各由自己的登记册签发，同一个串可以各指一次到场。范围取那一次尝试，业务时间取对象结果的时刻。揽收到手与妥投都不是失败，答未受理。
- **实际履约**：指名一个实际履约段与一个或多个成员。每个成员在段里都必须有一条有效参与（判当前版，被更正撤回的不算），且那条参与已经结束（下一次交接、有效交付或明确终止）。依据写成 `FULFILLMENT-SEGMENT/<段>`，范围取段，业务时间取成员终点中最晚的一刻——早于它，还在途的成员会被算成已履约。参与还在场答未受理 `PARTICIPATION_NOT_ENDED`，对象不在段里答 `OBJECT_NOT_PARTICIPATING`。
- **订舱**（依据是已接受的承运答复）与**取消**（依据是未开始的范围）随票 demo-chain-gaps/28 接上；在那之前答未受理 `REASON_NOT_REGISTRABLE`，不拿别的事实凑。

**二、采购上下文由登记方给出，经参与方与商业核对；缺了答未受理，绝不答不适用。** 登记方给服务提供方、协议对象与版本标签，用例经消费侧适配器（[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)）按**业务时间**核那一版协议：协议在业务发生那一刻是否支持采购，后来到期或终止不改写已发生的事实。

- 采购责任法人与协议快照取自协议版本，不由调用方给。快照照 ADR-0208 决定六写成两段式指称「对象/版本」。
- 协议的四个否定格各自成格、答 200：未在册、未生效、已终止、服务方不符（带回这一版约定的供应商）。续办各不相同（去发布、换一版、取得新协议、改服务方），折成一格登记方只能去猜（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)）。核对口读不动答未决。
- 三件缺任一答未受理 `PROCUREMENT_CONTEXT_MISSING`。**不适用只有两格**（ADR-0098 决定三）：登记方显式声明自营（此时服务方与协议必须留空，同时给出答未受理 `SELF_OPERATION_CONTRADICTED`），或那一段当前的实际承运商判断是自营法人。没有判断、判断待确认、判为外部参与方都不说明自营。两格都先问册：同一件事已登成外部发生项时答冲突并回显它——实际承运商判断可以在登记之后才改判，那时直接答不适用，就把册上那份外部成本静默地说成了没有。问册先问成员交集、取决定四那把串行锁，再按身份查：先按身份查，看不见另一笔正在登记同一身份、尚未提交的外部发生项，等锁之后的交集一问又按身份把它排除，两问都落空。
- 票 demo-chain-gaps/28 起，段已采用运输委托时从委托快照取采购上下文；调用方给了而不一致答冲突。不在建段时盖章。

**三、发生条件在协议能声明之前由登记者断言，断言与发生项同行留痕。** 登记方交一句原话断言「采用协议的发生条件已满足」（引哪一版协议、凭什么事实）；缺了答未受理 `CONDITION_NOT_ASSERTED`。发生项记下条件的**依据**（封闭集合，首版只有「登记者断言」一格）、**断言人**与断言原话，库面断言那一格两件都必带、不得为空。只记「断言过」不够：日后复核的人要知道当时是谁断言的、断言的是什么。断言人是做出这一次登记决定的操作者，取自操作者信封的主体，写成「发行方#sub」（[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定二第三条），与租户一样不由载荷给。断言人是登记决定的出处，不是发生项的内容：另一位操作者照原样重报，说的仍是册上那一项，答`已存在`、回显原断言人；原话不同才是异内容。数量与单位同由登记方声明。ADR-0209 落地后多出「按协议声明判」一格，协议没声明答`未配置`；两类发生项在册上仍分得开，审计要问的正是「这一项是谁判的」。

**四、发生项身份由事实确定性派生；首版 `V1`；同一成员在同一件事实上只计一次。**

- 身份由（租户、原因、事实依据、成员集合）派生：成员按字面排序，各维以 NUL 分隔，取 SHA-256 前十六字节，写成 `TCO-<十六进制>`。派生而不签发：同一件事实重登要落回同一个身份，重放才答得出`已存在`；签发的号每次都新，重登就成了第二项。
- 首个有效性版本是 `V1`；更正另起版本、回指它，不在首登处换号。
- 重登同内容答`已存在`，回显原项并重交同一份意图（入队按信封 ID 吞掉重发，先前若有一次没交成，这一次补上）；同身份异内容答冲突，回显原项。
- 同一（租户、原因、事实依据）上，成员与另一项**仍有效**的发生项有交集，答冲突、细格 `MEMBERS_ALREADY_COVERED`，回显那一项。身份按成员集合派生，组合一换就是另一项，不查交集，同一成员的成本会被算两遍。仍有效读每一项的当前版：被替代的旧版由新版接替；失效的那一项说的是「那个范围不成立」，其成员可以另登。查与随后的落册在同一事务里按（租户、原因、事实依据）串行化。
- 旅程由登记方声明，本上下文没有它的权威来源，不核。演示约定 `SYN-JRN-<包裹号>`。

**五、已形成信封与发生项同一事务交出，引用式载荷，指纹 ID。** 类型 `transport-fulfillment.charge-occurrence.formed`；载荷只带登记册键三维 `tenantId`、`occurrence`、`version`，键名与 ADR-0208 决定一一字不差，原因、业务时间、协议与成员都不带——它们只有登记册一处权威。信封 ID 由 `outboxintent.FingerprintEventID` 从键三维铸出，版本在 ID 里，两代各一封；分区键取「租户/charge-occurrence/发生项」，同一发生项的首版与日后的更正排一条队。事务边界归装配点：编排答未决或报错时整笔回滚，不留一份落了册而结算永远等不到的发生项。消费者先进路由表（ADR-0208 决定一，[ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)）。

**六、入口是操作者渠道「运营决定」能力面的一种决定。** 决定种类 `TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION`，口 `POST /transport-fulfillment-charge-occurrence-registrations`。按 [ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 的定族判据，这是操作者做出的生产决定：发生项不是现场的物理事实，是对已发生事实的一次采购判断，所以不挂作业事实登记，也不要设备。租户与登记者（断言人）只取自认证结果，载荷里不收。只有新落一版答 201，已存在、冲突、不适用、协议否定格、未决、未受理都是形成了的答案、答 200（[ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)）；原因词不在封闭集答 400。响应没有任何金额或币种键。

**七、不在任何物理编排里内联，也暂不自动形成。** 揽收、派送、交接、交付照常落册，发生项另行登记。ADR-0098 决定一的两条理由对实际履约原样成立：把形成塞进必须照常成功的物理登记，既压缩了对象状态，又造出一个读失败即静默不发生、无人重试的派生步骤。运输履约自消费交接与有效交付信封来自动形成，等 ADR-0209 与票 demo-chain-gaps/25 落地后另定。

三类：一、四、五、六、七是**机制**；二（不适用只认两格、缺采购上下文答未受理、按业务时间核）与三（断言期的判法）是**产品策略**；谁被授予这一种决定、准入范围对照与权威区间、每一项的旅程、数量与单位、断言原话，是**租户取值**，演示租户由种子登 `SYN-` 值、实测时由配置员登记，证据记 `S`。

## 候选与反方

- **在交接或有效交付的编排里顺手形成实际履约发生项。** 违反 ADR-0098 决定一，理由见决定七。否决。
- **TF 立一本发生条件册，按原因登计不计费。** 成了第二个商业权威：UC-TF-004 的输入行把「收费发生条件引用」指给参与方与商业。否决，条件文法归 ADR-0209。
- **采购上下文从当前伙伴或当前协议推导。** `CONTEXT` 明禁按当前伙伴、当前线路补齐；ADR-0098 决定二的判据「一个可能根本不存在的东西当不了推导的输入」同样适用。否决。
- **缺采购上下文答不适用。** 把「没填」读成「自营」，会把一笔真实外部成本静默吞掉。否决。
- **发生项引用签发而不派生。** 重登就是第二项、成本算两遍，重放答不出`已存在`。否决。
- **实际履约按段一项（身份不含成员）。** 同一段的成员可以在不同时刻、不同交接里结束，逐件计费的协议要按成员登；一段一项会让第二次登记撞冲突。否决；改为成员集合进身份，交集答冲突。
- **信封载荷抄原因、业务时间与协议。** 一份事实两处写，消费方会读到与登记册不一致的那一份。否决，ADR-0208 决定一已按引用读。
- **挂作业事实登记能力面。** 那一面要求设备在册、事实身份与时间由设备签发（ADR-0149 决定二），套在管理台上的采购判断上，是要配置员先登记一台设备才能登发生项。否决。

## Consequences

- 代码：运输履约 `application.RegisterChargeOccurrenceHandler`（失败尝试那一半在 `register_failed_attempt_charge.go`），领域 `ChargeOccurrenceForActualFulfillment`、`ChargeOccurrenceForDeliveryAttempt`、`DeriveChargeOccurrenceReference`、`OccurrenceCondition`、`RegistrantReference`；端口 `ChargeOccurrenceHandoff`、`FulfillmentSegmentView`、`ActualCarrierJudgmentView` 与册口的 `FindCovering`；`adapters/postgres.OutboxChargeOccurrenceHandoff`；`tfhttp.NewRegisterChargeOccurrenceEndpoint`；`cmd/parcel-api/assemble_charge_occurrence.go`。原先独立的失败尝试费登记处理器并入本用例，不再单列。运输履约的运营决定认证方随之交回 `tfhttp.OperatorIdentity`（租户与操作者），其余各口仍只取租户。
- 迁移 `transport_fulfillment/0032_charge_occurrence_condition_and_coverage_index.sql`（发生条件的依据、断言人、原话三列与成员交集一问的索引；表里有行即拒迁移，不替不在场的人补断言）；`access_identity/0009_transport_charge_occurrence_registration_decision.sql`（决定种类）。
- 演示租户的配置员持这一种决定：授予、准入范围对照、权威区间各一行（`scripts/demo-seeds/data/access/02`、`03`，`data/governance/25`）。种子不登任何发生项。
- [UC-TF-005](../application/transport-fulfillment/UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md) 步 9 与 `AT-TF-120` 至 `AT-TF-125`；UC-TF-002 与 UC-TF-006 的发生项一步指向本记录。
- 端到端（一次性库实测，证据 `S`）：干线参与在 SIN-HUB 结束后登 `ACTUAL_FULFILLMENT` 答 201，结算发起一份 BUY 请求，计价形成评价；演示成本卡没绑分区目录、按发生项造的计价输入也不带分区，评价停在待判断 `ZONE_UNRESOLVED`，按 ADR-0200 入账、不形成供应商预期成本。这一格就是 [ADR-0189](./0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md) 越权风险点 3，不在本记录范围内。走通这一格的演示租户取值另立票 demo-chain-gaps/43（演示成本卡绑分区目录，等 routing-first-cut/20）。
- 已知限制，留给发生项修订入口落地那一张票：重放与自营两条路径按身份只读首版 `V1`。首版被失效之后，同内容重登仍答`已存在`并回显那个已失效的首版，声明自营答冲突——与决定四「失效那一项的成员可以另登」口径不一。今天没有生产修订入口（`ReviseValidity` 没有生产调用方），修订入口落地时，这两条路径改读链头，链头已失效时另行作答。

## 越权风险点

1. **断言期内发生条件由人判断**（TF、PC owner）。ADR-0209 落地之前，「协议计不计这一项」没有机器核对；断言留痕（断言人与原话）只保证事后查得到是谁凭什么判的，不保证判得对。
2. **旅程无人核**（TF owner）。同一包裹被不同登记者声明成不同旅程时没有核对；旅程是跨上下文的词，权威来源未定。
3. **不自动形成**（TF、SA owner）。参与结束之后要有人登记，漏登就漏一份成本来源；结算那一侧没有「该有而没有」的提示。
4. **数量与单位由登记方声明**（TF、PC owner）。协议能声明数量形之前不核，`SEGMENT`、件数还是次数由登记方一并断言。按段计费的协议下，同一段上的包裹若逐件各登一项、每项都填「数量 1、单位 `SEGMENT`」，同一段就算出多份段费；成员交集保护按成员判、不按段判，拦不住这一种重复（TF、PC owner 定）。ADR-0209 的数量形落地时，`PER_SEGMENT` 应在段级去重。
5. **成员交集只在同一（原因、事实依据）上查**（TF owner）。同一包裹在同一段上，以实际履约与别的原因各登一项不算交集；一个原因对一个费用项目（ADR-0208 越权风险点 1），这两项本该各自计费，但若 owner 认定某些原因互斥，要另加判据。
6. **自营只认两格**（TF owner）。协议、委托与承运商判断都说是外包、而登记方声明自营时，本用例照声明答不适用；声明本身不核。
7. **断言人不进内容比对**（TF owner）。另一位操作者照原样重报答`已存在`，第二位操作者的断言不另留痕；断言人随发生项回显在登记答复里，持这一种决定授予的操作者都看得到。

## Links

- [ADR-0098：失败尝试费发生项不在揽收编排里形成，采购上下文显式给出而不推导](./0098-a-failed-attempt-charge-occurrence-is-not-formed-inside-the-pickup-orchestration.md)：本记录落在其决定二之内，把决定一扩到实际履约；决定三的不适用格由本记录决定二给出两种读法；接通其 Consequences 所说的生产触发链
- [ADR-0208：BUY 评价由发生项已形成信封发起](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md)：本信封的消费方；决定一的载荷三键与决定六的协议指称形状由本记录照写
- [ADR-0200：BUY 评价按回指的请求形成供应商预期成本](./0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md)：再下游
- [ADR-0189：逐段计价区域按卡给](./0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md)：越权风险点 3 是端到端的停点
- [ADR-0151：未归族的命令口定族](./0151-unassigned-command-faces-get-their-families.md)：决定六的定族判据
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)、[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)、[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)、[ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)
- [UC-TF-005](../application/transport-fulfillment/UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md)：步 9，`AT-TF-120` 至 `AT-TF-125`
- 票 `.scratch/demo-chain-gaps/issues/27-transport-charge-occurrences-are-registered-and-handed-off.md`；后续 28（订舱与取消、经段采用取采购上下文）、29（协议声明发生条件，ADR-0209）
