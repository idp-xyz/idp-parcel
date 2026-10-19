# ADR-0208：BUY 评价由发生项已形成信封发起——发生项与协议取自发生项，费用项目取触发册，主要范围取协议的商业范围

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/26，代决出自 `.scratch/demo-chain-gaps/tf-chain-plan.md` 的「BUY 发起（接 08，收 10）」一节。越权风险点见文末，待 SA owner 复核，第 2、8 点同时待 PC owner、第 5、7 点同时待 TF owner。）
Date: 2026-10-19

## Context

- [ADR-0164](./0164-buy-evaluation-request-trigger-is-occurrence-formed.md) 定了 BUY 评价请求的触发时点是「发生项形成」，按（租户、原因）登记要不要发起。它的触发面要调用方交齐三件来源引用（发生项、费用项目、供应商协议）和主要范围，决定一末句写明「不从运输履约登记册推导三件来源引用」。这个触发面至今只为 fail-fast 装配，没有调用方：谁在发生项形成那一刻把这几样交进来，没有着落。
- 运输履约的发生项登记册已在（迁移 `transport_fulfillment/0007_transport_charge_occurrence.sql`）。发生项固定了采购责任法人、服务提供方、采用的供应商协议、原因、唯一主要业务范围、成员、数量与单位、业务时间和有效性（运输履约 `CONTEXT`）。发生项已形成的信封 `transport-fulfillment.charge-occurrence.formed` 由票 27 上线（ADR-0207）。
- 费用项目归结算（结算 `CONTEXT`「费用项目」）；发生项与供应商协议上都没有它。
- 计价按请求上的主要范围解析在用价卡（`PriceCards.ResolveInForce`，按租户、范围、方向、目的、时点）。发生项上的「主要业务范围」是运输一侧的范围（例如一段实际履约），不是计价范围。演示的成本卡与干线供应商协议同在 `SYN-SCOPE-01`。
- [ADR-0200](./0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md) 已让 BUY 评价按回指的请求形成供应商预期成本。缺的只是上游：请求从哪来。

## Decision

**一、结算在派发里消费发生项已形成信封，按引用回读发生项。** 消费者先进路由表（[ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)：某类事件的生产方上线之前，它的消费者必须先在路由表里），生产方随票 27 上线。载荷只带三维引用，键名是 `tenantId`、`occurrence`、`version`（有效性版本），缺一即毒丸。结算经消费侧适配器（`internal/settlementaccounting/adapters/transportfulfillment/`，[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)）按这三维读运输履约登记册的那一份。

**二、发生项、原因、有效性版本、业务时间与供应商协议都取自发生项本身。** 协议是发生项形成时就固定下来的来源事实，读它不是「从登记册推导」。本条部分停用 ADR-0164 决定一末句「不从运输履约登记册推导三件来源引用」，**适用场景**仅限本信封那一路：按引用回读的是那一份发生项自己的事实，不是按当前伙伴、线路或报表分组去配。ADR-0164 那条由调用方交齐三件引用的触发面照旧不读运输履约。

**三、费用项目来自 BUY 触发册的新格。** 触发册每行（租户、原因）加一格「费用项目」，属租户取值，可缺（迁移 `settlement_accounting/0040_buy_evaluation_trigger_fee_item.sql`）。没登这个原因、或登了却没登费用项目，答`未配置`并入账，不重投；补登之后，已入账的那一份不回补。册不设修订：同原因换一个费用项目或给没登的行补登，都答冲突。

**四、主要范围取那一版协议的商业范围，经结算→参与方与商业的读口取。** 参与方与商业新开一个只交范围的窄读口：按「对象/版本」两段式指称（`NewQualifiedVersionLabel` 那一处拼出的形状）答那一版协议正文上的适用范围（`ports.SupplierAgreementScopeView`）。取的是正文上的适用范围，不是版本壳上的范围（两者分开存，见候选与反方）。读不动答未决、整笔回滚重投；读回查无是两侧矛盾——发生项登记时已核过协议——上抛为发布失败，不重投，不替它挑一个范围；一份指称比中不止一版协议（对象标识或版本自带斜杠时两段式指称本身有歧义）同样上抛，不挑一行。不核协议此刻是否生效：协议终止不改写在它之下已经发生的事实。

**五、各格的去向。** 已发起（含同一份重放答已存在）、未配置的两格、修订版不发起，都入账。修订版指有效性修订产生的那一版（失效或替代）：本路不为它发起，修订怎样驱动纠错归票 demo-chain-gaps/15。只有依赖答不出才是未决，登进派发的未决哨兵：发生项读不回或登记册读不动、触发册、协议范围读口、请求册、铸造口或交信封任一答不出（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)、[ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)）。交信封失败在这一路答未决：请求编排在事务外的用法里留续办引用、照答已请求，那个形状在消费事务里会让请求行提交而信封缺席。读回的发生项与键对不上、词汇表外（含协议范围读口答出的译不过去与指称歧义）、本路译出的命令被拒，都上抛为发布失败——这几种重投同一份永远同一个结果，进未决名单只会以未决之名耗尽失败预算。

**六、协议指称串的形状由本记录定给生产方。** 发生项上的 `agreement_ref` 必须是参与方与商业的两段式指称「对象/版本」，例如 `SYN-SUPPLIER-TRUNK-CN-SG-01/v1`。票 27 登记发生项时照此写。

**七、请求方是本路自己。** 本路登在请求上的请求方是固定名 `settlement-accounting/buy-evaluation-on-occurrence-formed`，不是任何租户的人。

三类：一、二、五、六、七是**机制**；三、四是**产品策略**（不看任何租户的合同就答得了怎样取费用项目与主要范围）；触发册的行与行上的费用项目是**租户取值**，演示租户由种子登 `SYN-` 值，证据记 `S`。

## 候选与反方

- **费用项目取价卡费目或发生项原因。** 那是把计价或运输的词当成结算的费用项目，第二个口径。否决。
- **费用项目按（原因、协议）分行。** 更准，但演示只有一份干线协议，现在立两维册是替还没出现的租户定形。留给 owner，见风险点 1。
- **主要范围取发生项的主要业务范围。** 那是运输一侧的范围（一段履约），计价拿它解析价卡只会零命中。否决。
- **主要范围取版本壳上的范围（`commercial_version.scope_ref`）。** 版本壳的范围回答「这一版商业对象登在哪个范围下、装载按它过滤」；正文上的适用范围（`supplier_agreement.agreement_scope_ref`）回答「这份协议在哪个范围内绑定了哪份采购定价方案」，计价按主要范围选的正是那份方案所在的卡。两者分开存（迁移 `party_commercial/0021_supplier_agreement.sql` 头注），演示里同值。取壳上的范围，等于拿登记位置顶约定范围。否决，见风险点 2。
- **主要范围由触发册再登一格。** 又一处要租户维护、又会与协议的商业范围分歧的值。否决。
- **借票 22 的协议版本点读口（版本壳加正文）。** 本路只要范围，拿整份正文就把生效判断、供应商与法人一并交给一个用不着它们的消费方；票 22 与本票也是两条并行车道。另开只交范围的窄口，见风险点 8。
- **修订版照样发起。** 失效的那一版也会被估成本；纠错的形状在票 15 才定。否决。
- **协议查无答未配置。** 缺的不是租户取值，是两侧对不上；入账会把一份坏发生项静默吞掉。否决。

## Consequences

- 代码：结算 `sainbox.ChargeOccurrenceFormedConsumer`、`adapters/transportfulfillment.RequestOnChargeOccurrenceFormedAdapter`（哨兵 `ErrChargeOccurrenceNotVisible`、`ErrBuyEvaluationTriggerUndecided`）、`adapters/partycommercial.SupplierAgreementScope`、`application.TriggerBuyEvaluationOnOccurrenceFormedHandler`（矛盾哨兵 `ErrOccurrenceAgreementNotRegistered`）；协议范围的确定性坏答案哨兵：结算 `ports.ErrSupplierAgreementScopeUntranslatable`、参与方与商业 `ports.ErrSupplierAgreementLabelAmbiguous`；触发结果多出`未决`与「修订版不发起」两格，`未配置`点名缺哪一格。`ports.BuyEvaluationTriggerView` 交回整行。参与方与商业 `ports.SupplierAgreementScopeView` 与 `postgres.SupplierAgreementScopes`。派发的路由表登 `transport-fulfillment.charge-occurrence.formed`，哨兵名单 `chargeOccurrenceFormedUndecidedSentinels`。
- 迁移 `settlement_accounting/0040_buy_evaluation_trigger_fee_item.sql`；`parcel-settlement-register buy-evaluation-trigger` 的输入多一个可选键 `feeItem`。
- 演示租户登一行：`ACTUAL_FULFILLMENT` 采用发生项形成，费用项目 `SYN-FEE-ITEM-TRUNK-COST-01`。
- UC-SA-002 步 2 与 `AT-SA-185` 至 `AT-SA-187`；结算 `CONTEXT` 一句；`CONTEXT-MAP` 运输履约→结算一条。
- 没有生产方交出这一类信封时，本路一封也收不到。端到端证据随票 27。

## 越权风险点

1. **一个原因只映一个费用项目，不分协议**（SA owner）。同一原因在两份协议下该记成不同费用项目时，本册表达不了。
2. **主要范围等同协议的商业范围**（SA、PC owner）。一份协议覆盖多条线、各线另有成本卡时不够用；计价按它选卡，选到的是那个范围里唯一在用的卡。取的是正文上的适用范围而不是版本壳上的范围；两者在某个租户那里分了家时，owner 可另裁。
3. **`Request` 仍可被直调**，不经任何触发面（同 ADR-0164 越权风险点 3，SA owner）。
4. **触发册不设修订**（SA owner）。补登或改费用项目答冲突，只能由 owner 另裁修订形状。
5. **修订版不发起**（SA、TF owner）。发生项更正怎样驱动预期成本纠错，归票 demo-chain-gaps/15。
6. **交信封失败不分确定性拒收与暂时故障**（SA owner）。请求编排把两种都折成续办引用，本路一律答未决重投；确定性拒收会重试到失败预算耗尽。
7. **协议指称串的形状由消费方先定**（TF owner）。生产方票 27 要照「对象/版本」写 `agreement_ref`；写成别的形状，本路会把每一份都答成协议矛盾。
8. **协议范围读口与票 22 的版本点读口并存**（PC owner）。两口读同一张正文表、键形不同（本口收两段式指称，那一口收对象与版本两格）。owner 若要一口，合并时以那一口为准、本口改成转调。

## Links

- [ADR-0164：BUY 评价请求的触发面是发生项形成，原因没登记就不发起](./0164-buy-evaluation-request-trigger-is-occurrence-formed.md)：本记录决定二部分停用其决定一末句，限本信封路径；并回答其越权风险点 2
- [ADR-0200：BUY 评价按回指的请求形成供应商预期成本](./0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md)：下游，本路发起的请求形成的评价由它形成成本
- [ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)：消费者先于生产方进路由表
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：跨上下文读经消费侧适配器
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)、[ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)：未配置入账，读不动未决
- [UC-SA-002](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md)：步 2，`AT-SA-185` 至 `AT-SA-187`
- 票 `.scratch/demo-chain-gaps/issues/26-settlement-consumes-charge-occurrence-formed-to-request-buy-evaluation.md`
