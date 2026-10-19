# ADR-0209：供应商协议声明收费发生条件——逐原因计不计费、计费带数量形，发生项登记按采用的那一版判，断言期退场

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/29，代决出自 `.scratch/demo-chain-gaps/tf-chain-plan.md` 的「07 运输收费发生项」D07-1 与「需要的 ADR」里 ADR-0209 一条。部分停用 [ADR-0207](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) 决定三的断言期判法与决定四串行化那一句的键，适用场景见决定四、六与文末「对已接受记录的作用」。越权风险点见文末，待 PC owner 与 TF owner 复核。）
Date: 2026-10-19

## Context

- 运输履约 `CONTEXT`「运输收费发生项与成本来源」一节：只有满足**当前采用协议所需事实条件**的明确范围，才能形成运输收费发生项。UC-TF-004 的输入行把「收费发生条件引用」指给参与方与商业：条件是供应商协议的内容。
- 供应商协议正文（迁移 `party_commercial/0021`）只有供应商、责任法人、范围、采购方案与区间，**没有发生条件的文法**。ADR-0207 因此先开了一段断言期：登记者断言「采用协议的发生条件已满足」并留痕，数量与单位也由登记者报；它在决定三里写明 ADR-0209 落地后多出「按协议声明判」一格、协议没声明答`未配置`，在越权风险点 4 里写明按段计费的协议下逐件各登「数量 1、单位段」会把一段算成多段，要数量形落地时在段级去重。
- 商业发布的内容摘要由服务端规范化算出（ADR-0126），摘要带规范化版本、只在同一版本内可比（[ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)）。供应商协议一节今天按 `PCC-1` 写；`docs/adr/README.md` 里 ADR-0014 那一行写明「此后任何一册再改规范化形状照本记录换号」。
- 演示租户的干线 `SYN-SUPPLIER-TRUNK-CN-SG-01` 在种子里只有 `v1`，摘要 `PCC-1:72457b0d…`；票 demo-chain-gaps/27 的演示按断言期登了一项实际履约。

## Decision

**一、供应商协议正文加一节「收费发生条件」，逐原因声明计不计费，计费的带数量形；整节可缺。**（机制）

- 原因是运输收费发生项的四个原因词：`BOOKING`、`CANCELLATION`、`FAILED_ATTEMPT`、`ACTUAL_FULFILLMENT`，与运输履约逐字相同。本上下文只用这四个字面量指名，不解释原因背后的事实怎么取。
- 每个原因至多一条：计费（带一个数量形），或明确不计费（不带数量形）。「不计费」是一句声明，不是缺席；协议根本没提这个原因才是没声明。整节在场却一条不声明拒，与整节缺席说的是同一句话，一句话只许一种写法。
- 条件只能随正文同一次发布固定。要改条件就发新版本，旧版本不可变（CONTEXT「采购服务、价格、结算或责任条件变化时形成新版本」）。
- 批文与表单载荷同键：`supplierAgreementBody.chargeConditions[]`，每条 `reason`、`charged`（必给，缺键不替登记方选「计」或「不计」）、`quantityForm`（只随计费）。凡把原词折成声明的入口（规范化折回、批文、表单载荷、库面读回）都走 `ChargeOccurrenceConditionOf`，「不计费却带了数量形」只有一种读法——拒。
- 库面是子表 `party_commercial.supplier_agreement_charge_condition`（迁移 0038），外键挂在正文行上；供应商协议的读口与正文同一条语句读回。

**二、数量形四种，按原因配。**（产品策略）

| 原因 | 可声明的数量形 |
|---|---|
| `BOOKING` | `PER_BOOKING`、`PER_PIECE` |
| `CANCELLATION` | `PER_BOOKING`、`PER_PIECE` |
| `FAILED_ATTEMPT` | `PER_ATTEMPT`、`PER_PIECE` |
| `ACTUAL_FULFILLMENT` | `PER_SEGMENT`、`PER_PIECE` |

按件对每个原因都成立；按范围计的那一形只配主要业务范围就是那种东西的原因——失败尝试的范围是一次到场，实际履约的范围是一个段，订舱与取消的范围是一次订舱。表只在参与方与商业的领域一处（`quantityFormsByReason`），发布时判；运输履约按数量形计数时不再判原因。

**三、带条件的正文以 `PCC-2` 规范化，不带的仍是 `PCC-1`、逐字节不变；本构建两版都认。**（机制）

- 改的是供应商协议一节的形状，照 ADR-0014 换号。按内容取能表达它的最低一版：带条件写 `PCC-2`，否则写 `PCC-1`。不按构建取最新一版——那会让 v1 那类不带条件的正文换一个摘要，册上已存的串再也对不上。
- 折回两版都认：标 `PCC-1` 却带条件、标 `PCC-2` 却不带条件、别的册标 `PCC-2`，都不是本构建写下的文档，答不支持。只认 `PCC-1` 的旧构建读到 `PCC-2` 的文档答不支持，而不是把条件当未知键丢掉、再算出一个对不上的摘要——这正是换号要守的。
- 对账门：声明的串带本构建认得的另一版（例如正文带条件、批文抄的却是 `PCC-1` 的串）答`不等`、改批文；本构建不认的版本才答不支持、换构建。

**四、运输履约登记发生项时按采用的那一版协议判条件；断言期退场。**（产品策略）

- 协议核对口在在册格交回这一版声明的条件（运输履约的词，按字面搬，消费侧适配器翻译）。登记用例在协议四个否定格之后判：
  - 这一版没声明这个原因（整节缺席，或只声明了别的原因）→ `NOT_CONFIGURED`，细格 `CHARGE_CONDITION_NOT_DECLARED`。是租户还没配，不是依赖读不动：入账、不重投，续办是发一版声明了条件的协议。
  - 声明不计费 → `NOT_CHARGED`（「协议不计此原因」）。它是正确答案，但不是`不适用`：不适用说本来就没有外部成本来源（自营，ADR-0098 决定三、ADR-0207 决定二只认两格），这一格说外部供应商在、协议约定这一种事实不收费；登记方若认为该收，续办是发新版协议。
  - 两格都带回作答的那一版协议（「对象/版本」），都不落册、不问册、不交意图。
  - 计费 → 按数量形数出数量与单位：按件是成员数、单位 `PIECE`；按段、按尝试、按订舱是 1，单位 `SEGMENT`、`ATTEMPT`、`BOOKING`。登记方不再报数量与单位。
- 发生条件的依据多出 `AGREEMENT_DECLARED` 一格，ADR-0209 之后新形成的发生项只落这一格；依据不带断言人与原话，判断出自协议，协议已记在发生项上。
- 断言期退场：登记载荷的 `conditionAssertion`、`quantity`、`unit` 三键随之删去，带来按未知键拒（400）——静默丢掉，登记方会以为自己的断言或数量算了数。命令里也没有这三格，`CONDITION_NOT_ASSERTED` 一格随之退出结果代数。册上断言期形成的项（`REGISTRANT_ASSERTED`）照旧读得出，不改写、不补「按协议判过」。
- **按范围计的一项盖住整个范围。** 同一（租户、原因、主要业务范围）上另一项仍有效的发生项若按范围计过（单位是 `SEGMENT`、`ATTEMPT`、`BOOKING` 之一，断言期登记方报的同词同样算），或本次按范围计、范围上已有任何一项，答冲突、细格 `SCOPE_ALREADY_CHARGED`，回显那一项；同一事实依据上成员有交集照旧答 `MEMBERS_ALREADY_COVERED`。这回答 ADR-0207 越权风险点 4：一段只计一段，一次到场只计一次。自营的两格问册时也认按范围计过的项——同一段不能既有一笔段费又是自营。
- **「同一范围」连同事实依据的出处认。** 失败尝试的范围就是尝试身份，而揽收与派送的尝试身份各由自己的登记册签发、同一个串可以各指一次到场（ADR-0207 决定一两侧依据分写前缀的理由）。两侧同串是两次到场：一侧按尝试计过不挡另一侧，对另一侧声明自营也不答冲突。出处取依据串第一道斜杠之前那一词（`OccurrenceBasisReference.Source`）；范围串本身不带侧别，断言期已落册的项因此照旧认得出。
- 查与随后的落册改按（租户、原因、主要业务范围）串行化，不再按（租户、原因、事实依据）：按范围计的一项要挡住同一范围上任何事实依据的另一笔，按依据上锁两笔各握各的锁、各自看不见对方。事实依据定得出范围，同一依据上的两笔照旧排在一把锁上。锁键只取范围串、不取出处：两侧同串的两次到场共用一把锁，只多等，不错判。

**五、每一项发生项记登记者。**（机制）

断言期只在断言那一格记人；换成按协议判之后若不另记，审计就找不到是谁报的服务方、协议版本、成员与旅程。登记者取自操作者信封的主体（「发行方#sub」，ADR-0100 决定二第三条），与租户一样不由载荷给；库面另加 `registered_by` 一列（迁移 TF `0034`），0032 的断言人一列含义不改，断言行由断言人回填、此后两列相等。登记者是登记决定的出处，不是发生项的内容：另一位操作者照原样重报答`已存在`、回显原登记者（同 ADR-0207 决定三对断言人的处理）。登记答复回显 `registeredBy`。

**六、演示租户以 v2 重发干线协议。**（租户取值，证据只记 `S`）

`SYN-SUPPLIER-TRUNK-CN-SG-01` 的 `v2`：实际履约按件计（演示 BUY 成本卡按包裹计重计价，按件与它同口径），失败尝试不计费（干线承运方不做揽派），订舱与取消不声明。区间与 v1 同，`v1` 原样保留、仍在册、不带条件——按它登记答`未配置`。演示的段采用与登记都指名 `v2`。

三类：一、三、五是**机制**；二、四是**产品策略**；某个租户的协议对哪个原因计不计费、按什么数量形，是**租户取值**，由租户发布协议版本时声明，演示租户由种子登 `SYN-` 值、证据记 `S`。

## 候选与反方

- **运输履约自立条件册。** 成了与协议并行的第二个商业权威（ADR-0207 候选与反方已否）。否决。
- **只加「计不计费」，不加数量形，数量与单位仍由登记方报。** 按段计费的协议下逐件各登仍把一段算成多段（ADR-0207 越权风险点 4），数量口径照旧由人判。否决。
- **数量形自由取值（开放词表）。** 运输履约按词计数，开放词表就要在运输履约另立一张「词 → 怎么数」的表，等于第二个口径；未知词只能答未决。否决，四形封闭，加形改领域一处。
- **不配原因，任何原因可声明任何数量形。** 「失败尝试按段计」读不出意思，运输履约要么数错、要么另判。否决，发布时按表判。
- **`PCC-1` 不换号、只在供应商协议一节加一个可缺键（先例：服务产品册加交付条件一节、客户合同册加客户账户一键）。** 不带条件的旧文档确实逐字节不变；但只认 `PCC-1` 的构建读到带条件的文档时会把条件当未知键丢掉，算出的摘要与列上对不上却报不出是形状变了。ADR-0014 那一行写明此后照换号，前两次不换号各有「没有需要回放的已保存摘要」的前提，本册今天已有演示与将来的租户摘要。否决。
- **按构建一律写 `PCC-2`。** v1 这类不带条件的正文摘要会变，种子与册上已存的串对不上，重放答不等。否决，按内容取最低一版。
- **没声明答`不适用`，或答未受理。** 前者把「租户还没配」读成「本来就没有外部成本」，一笔真实成本被静默吞掉；后者让登记方去改一份本来没错的请求。否决，答`未配置`。
- **声明不计费并进`不适用`（加一个细格）。** 两格续办不同（不适用什么都不用做；不计费要么认、要么发新版协议），也会改写 ADR-0207 决定二「不适用只有两格」。否决，单独成格。
- **断言期保留为兜底：协议没声明时仍收断言。** 协议能声明之后，由人判就是与协议并行的第二个口径，ADR-0207 决定三本身写的就是「协议没声明答未配置」。否决。
- **按段计的一项只在成员有交集时挡。** 拦不住逐件各登。否决，按范围挡。

## 对已接受记录的作用

- **部分停用 [ADR-0207](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) 决定三**「登记方交一句原话断言『采用协议的发生条件已满足』……缺了答未受理 `CONDITION_NOT_ASSERTED`」与「数量与单位同由登记方声明」两句。**适用场景**：本记录之后新形成的发生项；停用的理由是条件与数量形已由采用的协议声明，由人再判就是第二个口径——这一前提只对新登记成立，断言期已形成的项照旧按断言读，不改写。决定三其余各句（断言人是登记决定的出处、不进内容比对、两类在册上分得开）不变。
- **部分停用 ADR-0207 决定四**「查与随后的落册在同一事务里按（租户、原因、事实依据）串行化」一句的键，改为（租户、原因、主要业务范围）。**适用场景**：全部登记；理由见决定四末条——按范围计的一项要挡住同一范围上别的事实依据。事实依据定得出范围，原句要守的那一种并发照旧守得住。决定四其余各句不变。
- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：照其 Decision 换号，不停用。
- [ADR-0098](./0098-a-failed-attempt-charge-occurrence-is-not-formed-inside-the-pickup-orchestration.md)、[ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md)：不变。信封载荷仍是三键；结算回读的发生项多了按协议判的依据与登记者，不进结算的判断。

## Consequences

- 代码：参与方与商业 `domain.TransportChargeReason`、`ChargeQuantityForm`、`ChargeOccurrenceCondition(s)`、`SupplierAgreement.WithChargeConditions`、`ChargeOccurrenceConditionOf`，规范化 `supplierChargeConditionCanonicalizationVersion`；批文 `chargeConditionDocument`、表单 `ChargeConditionPayload`；`postgres` 供应商协议读写口。运输履约 `domain.ChargeQuantityForm`、`AgreementChargeConditions`、`ConditionDeclaredByAgreement`、`NewAgreementDeclaredOccurrenceCondition`、`AttemptOccurrenceScope` / `SegmentOccurrenceScope`、`OccurrenceBasisReference.Source`、`ScopeCountingUnits`；`ports.SupplierAgreementCheck.ChargeConditions`、`ChargeOccurrenceCoverageQuery` 的范围三格；登记用例的 `NOT_CONFIGURED` / `NOT_CHARGED` 与 `SCOPE_ALREADY_CHARGED`；登记载荷去掉三键、答复加 `agreement` 与 `registeredBy`。
- 迁移：`party_commercial/0038_supplier_agreement_charge_condition.sql`；`transport_fulfillment/0034_charge_occurrence_agreement_declared_condition_and_registrant.sql`。
- 种子：`scripts/demo-seeds/data/commercial/publish-batch.json` 加干线 `v2`（`PCC-2:55a33e7a…`）。
- 文档：PC 与 TF `CONTEXT`、GLOSSARY「收费发生条件」、CONTEXT-MAP 的 PC→TF 一条；UC-PC-001 `AT-PC-051` 至 `053`；UC-TF-005 步 9 与 `AT-TF-166` 至 `170`；参数登记册 `PAR-SET-03` 的登口。
- 已知限制：今天只有失败尝试与实际履约两种原因能登（订舱与取消随票 demo-chain-gaps/28）；按订舱计的两形已能声明，运输履约数出来的是「1 BOOKING」，订舱与取消的范围怎么取由票 28 定。重放与自营两条路径按身份只读首版 `V1`（ADR-0207 已知限制）不变。

## 越权风险点

1. **文法偏简**（PC、TF owner）。没有阈值（如「第二次失败尝试起计」）、免费额度、按重量或体积段的数量形、按原因之外的维度（服务、线路）分条件。真实协议要这些时加形或加格，都要换规范化版本。
2. **数量形与原因的配表**（PC owner）。取消只配按订舱与按件；取消一份未订舱的委托若要计费，没有一形配得上（票 28 定取消的事实依据时复核）。
3. **按件以成员数计**（TF owner）。集运单元作成员时按件数的是单元数，不展开成员；协议若按包裹计，要等成员展开另裁。
4. **按范围计的覆盖认单位词**（TF owner）。断言期登记方报的单位只要是 `SEGMENT`、`ATTEMPT`、`BOOKING` 之一就算整体计过；报成别的写法（小写、`LEG`）的断言项挡不住按范围计的新登记。今天册上只有演示的断言项。
5. **v1 与 v2 并存在册**（演示租户、PC owner）。批文没有替代或退役的写口，v1 不会被收尾；按 v1 登记答`未配置`。真实租户换版时要有收尾旧版的入口（替代或退役），本记录不开。
6. **断言期的项不迁**（TF owner）。断言期形成的项留在 `REGISTRANT_ASSERTED`，同一事实按 v2 重登时身份相同、内容不同（依据、数量单位）答冲突并回显断言项；要换成按协议判的项得先失效断言项，今天没有生产修订入口。
7. **登记者不进内容比对**（TF owner）。同 ADR-0207 越权风险点 7：第二位操作者照原样重报不另留痕。
8. **`NOT_CHARGED` 不落任何记录**（TF、SA owner）。协议不计此原因的那一次判断只在答复里，册上没有「此事实按协议不计费」的痕迹；结算侧看不到「该有而没有」与「明说不收」的区别。
9. **「同一范围」靠依据串的出处分侧**（TF owner）。出处取依据串第一道斜杠之前那一词，库面查询与领域同一读法；日后某种依据的前缀若带斜杠，或两种事实共用一个前缀，覆盖判断会把两侧拆开（漏挡）或并成一侧（多挡）。范围串本身不带侧别，是为了认得出断言期已落册的项；将来若改成范围串自带侧别，要连同存量行一起迁。

## Links

- [ADR-0207：运输收费发生项由运营决定显式登记](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md)：本记录部分停用其决定三的断言期判法与决定四的串行化键，回答其越权风险点 1、4
- [ADR-0014：为内容摘要的规范化形状引入版本号](./0014-versioned-canonicalization-shape-for-content-digest.md)：`PCC-2` 照它换号
- [ADR-0126：商业发布摘要由服务端按册规范化](./0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md)：对账门与载体快照沿用
- [ADR-0098](./0098-a-failed-attempt-charge-occurrence-is-not-formed-inside-the-pickup-orchestration.md)、[ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md)、[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)、[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md)：`AT-PC-051` 至 `053`
- [UC-TF-005](../application/transport-fulfillment/UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md)：步 9，`AT-TF-166` 至 `170`
- 票 `.scratch/demo-chain-gaps/issues/29-supplier-agreements-declare-charge-occurrence-conditions.md`
