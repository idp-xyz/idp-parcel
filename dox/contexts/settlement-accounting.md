# 结算与经营核算（`settlement-accounting`）

代码在 `internal/settlementaccounting`。对照 main `99fc0037d`（2026-11-12）写成。

**这份文件核到什么程度**：取值表、SELL 评价请求的结论、停因、入口四节是从代码里读出来的；哪些编排有生产入口，是在 `cmd/` 下逐个查构造与调用得出的。「规则」一节的句子是从旧 `docs/domain/settlement-accounting/CONTEXT.md` 提炼的，每条只确认了代码里有对应的机制（类型、函数或结果码），没有逐条跑测试验证行为；旧文档写了而代码里没有的，以及有编排却没有入口的，单列在最后一节。

## 管什么，不管什么

管：评价请求的发起与续办，把评价采用成预估客户费用与供应商预期成本，费用确认与调整，对账单与费用争议，供应商账单的接收、匹配与审核应付，外部资金事实的采用、映射与核销，运营结算余额，接受前财务控制（估价、资金冻结、信用暴露），客户赔付与追偿金额，实际代垫与客户代垫回收，成本分摊与经营结果。

不管：

- 可执行价卡与纯计价评价——`parcel-pricing`。本上下文只消费评价，不自行选「最新」价卡，不另建算价规则。
- 客户合同、供应商协议、结算政策、接受前财务控制策略、信用政策——`party-commercial`。
- 委托、包裹、接受或拒绝决定——`parcel-shipment`。本上下文只交控制结果，不作接受判断。
- 运输收费发生项与履约事实——`transport-fulfillment`。费用只引用来源，不回写。
- 申报、监管核定、税费付款核对——`customs-compliance`。异常案件、索赔项、责任结论、对方响应——`visibility-exception`。
- 代收本金与代收分户账——`collection-remittance`。代收本金不是物流收入，不进运营余额。
- 银行流水、支付清算、法定税务与发票、总账、会计收入确认、坏账减值——外部财务系统。本上下文是物流业务结算分户账，不是 ERP。

## 词

| 词 | 意思 |
|---|---|
| 结算账户 | 归集同方向费用、余额、对账和核销的业务账户。固定责任法人、结算相对方、收付方向、结算币种、结算政策。不是货主客户账户，不是银行账户 |
| 费用项目 | 一种收费或成本经济含义的稳定定义，不含金额 |
| 主要计费范围 | 一条费用承担金额责任的唯一业务范围 |
| 计价基准时点 | 选版本与输入依据所用的业务时点，由发起评价的那一路内置给出。不是计算执行或消息到达的时间 |
| 费用明细 / 费用调整 | 一个费用项目在一个主要计费范围上形成的金额记录；调整是对既有费用追加的借项或贷项。预估、暂估、确认、调整分别保留，不互相覆盖 |
| 客户运营应收 | 已确认客户费用及其有效调整形成的运营金额责任。不是会计应收 |
| 供应商预期成本 | 依 BUY 评价与运输收费发生项形成的内部预期金额。不是账单主张，不是审核应付 |
| 供应商账单主张 | 供应商提出的收费范围和金额。账单号是它的外部标识，不是主张身份 |
| 已匹配金额 | 一行主张金额里有内部预期成本撑着的那部分，是这一行通过审核的上限 |
| 审核决定 / 审核应付 | 审核人对一行主张上一段金额给的通过、争议或拒绝；通过的那一段形成审核应付 |
| 供应商费用贷项 | 对既有主张或审核应付追加的贷记。不覆盖原应付，也不表示已返款 |
| 客户赔付 | 运营企业对客户的应付或退款义务。不因追偿延后或冲抵 |
| 应追偿金额 / 追偿认可金额 | 有权向责任相对方主张的金额；对方明确认可的那部分。两者都不是到账 |
| 客户代垫回收 | 依实际代垫成立判断（运营企业是否已实际垫付）与客户责任向客户形成的金额责任 |
| 结算归属日 | 判断费用归入哪个结算周期的业务日期 |
| 对账安排 | 一个结算账户对账所用的业务时区、截单时刻与对账周期 |
| 截单快照 / 对账单 | 一期入账费用的不可覆盖集合；据它发布的结算文件，发布后单号、范围、金额固定 |
| 运营结算余额 | 入账余额、授信额度、冻结金额、已确认未结金额与可用余额。不是银行余额 |
| 资金冻结 / 信用暴露 | 委托接受前对预付余额、账期额度的临时占用 |
| 真实收付映射 / 核销 | 把外部已确认的收付款事实关联到账户与未结项；按金额范围分配到应收、应付、贷项等 |
| 成本分摊结果 / 未分摊余额 | 共享成本归因到分析范围的结果；没有合格对象而留下的来源金额 |
| 经营毛利 | 按口径、版本、币种、截至时点派生的经营指标，不可直接修改 |

## 取值表（代码里的封闭集）

| 轴 | 取值 |
|---|---|
| 费用阶段 | `ESTIMATED` `PROVISIONAL` `CONFIRMED` |
| 收付方向 | `RECEIVABLE` `PAYABLE` |
| 调整类型 / 方向 | `PRICING_CORRECTION` `COMMERCIAL_CONCESSION`；`DEBIT` `CREDIT` |
| 包裹计费时点 | `NETWORK_INTAKE_ADOPTED` `EFFECTIVE_DELIVERY_FINALIZED` |
| 确认依据（内置） | `EFFECTIVE_DELIVERY_FINALIZED` |
| 归属日形态 | `SOURCE_OCCURRED` `CHARGE_CONFIRMED` |
| 对账周期切法 | `DAILY` `WEEKLY` `MONTHLY` |
| 争议处置 | `ACCEPTED` `PARTIALLY_ACCEPTED` `REJECTED` `PENDING_REVIEW` |
| 匹配分类 | `MATCHED` `QUANTITY_VARIANCE` `PRICE_VARIANCE` `NO_MATCHING_OCCURRENCE` `DUPLICATE_BILLING` |
| 审核结果 / 权限判断 | `PASSED` `DISPUTED` `REJECTED`；`WITHIN_AUTHORITY` `MUST_ESCALATE` |
| 资金事实种类 | `RECEIPT_CONFIRMED` `PAYMENT_FAILED` `FUNDS_RETURNED` |
| 映射目标 | `STATEMENT` `ACCOUNT` `PAYABLE` `CREDIT_NOTE` `CUSTOMER_COMPENSATION` `RECOVERY_ACKNOWLEDGEMENT` `COLLECTED_PRINCIPAL` |
| 结算方式 / 控制种类 | `PREPAID` `TERMS`；`PREPAID_FREEZE` `CREDIT_CHECK` |
| 冻结状态 / 暴露状态 | `HELD` `RELEASED` `RESTRICTED` `CONVERTED`；`RECORDED` `RELEASED` `RESTRICTED` `CONVERTED` |
| 实际代垫判断 | `ESTABLISHED` `NOT_ESTABLISHED` `UNDECIDED` `CONFLICTING` |
| 客户索赔金额种类 | `COMPENSATION_PAYABLE` `CLAIM_CHARGE_REFUND` |
| 分法 | `BY_WEIGHT` `BY_PIECE` `BY_REVENUE` `NOT_APPLICABLE` |
| 周期费用形态 | `MINIMUM_SPEND` `VOLUME_FLOOR` `TIERED_REBATE` `NOT_APPLICABLE` |
| 经营口径 | `ESTIMATED` `CONFIRMED` `SETTLED` |
| 停下的阶段 | `BEFORE_REQUEST`（发起停办） `IN_PRICING` `AT_ADOPTION` |

## SELL 评价请求的结论

包裹计费时点那一路，一份请求只下一次结论。

| 状态 | 什么时候 | 之后 |
|---|---|---|
| `SUBMITTED` 已提交 | 请求已登记，评价未回 | 等评价 |
| `FORMED` 已形成 | 评价已完成、合计不为零，全部费用行映到同一个费用项目 | 形成预估客户费用 |
| `PENDING_JUDGMENT` 待判断 | 评价待判断，费用行映不全或映到多个项目，或卡没声明金额取整 | 带停因，补登后续办 |
| `UNRATABLE` 不可计价 | 评价不可计价 | 带停因 |
| `CONFLICT` 冲突 | 评价冲突 | 带停因 |
| `NOT_FORMED` 未形成 | 评价失败，或计价没形成评价 | 带停因 |
| `ZERO_TOTAL` 负向结束（零额） | 评价已完成而合计为零 | 不建费用行，不带停因，不续办 |

停因（BUY 与 SELL 共用）：`SETTLEMENT_POLICY_NOT_ADOPTED` `SETTLEMENT_ACCOUNT_NOT_REGISTERED` `SELL_BILLING_MOMENT_NOT_ADOPTED` `PARCEL_HAS_NO_COMMERCIAL_RESOLUTION` `OCCURRENCE_REASON_NOT_REGISTERED` `FEE_ITEM_NOT_REGISTERED` `PRICE_CARD_NOT_CONFIGURED` `PRICE_CARD_APPLICABILITY_CONFLICT` `PRICING_INPUT_UNAVAILABLE` `EVALUATION_PENDING_JUDGMENT` `EVALUATION_UNRATABLE` `EVALUATION_CONFLICT` `EVALUATION_FAILED` `AMOUNT_ROUNDING_UNDECLARED` `CONVERSION_STEP_MISSING` `FEE_ITEM_MAPPING_INCOMPLETE` `EVALUATION_CATALOGUE_UNRESOLVED` `EVALUATION_SERIES_UNRESOLVED` `EVALUATION_RATE_NOT_FOUND` `EVALUATION_INPUT_INCOMPLETE` `PENDING_JUDGMENT_CAUSE_UNRECORDED`。

## 规则

**评价请求与续办**

- 包裹的 SELL 评价按包裹计费时点发起，时点由结算账户采用；没采用不发起。计价范围取自该结算账户的采用登记。
- BUY 评价由运输履约的发生项已形成信封发起：协议取自发生项，费用项目取 BUY 触发册上那个原因的一行；册上没有就不发起。
- 请求停下了不改写。租户补登解得开它的参数后，结算只续办这一次登记解得开的那几件：另起新一代、回指前一代，前一代原样留着。
- `未配置`照实答，不替租户填默认值。缺的事实不在任何参数册上的，由运营点名一件包裹或一版发生项续办。

**费用形成与确认**

- 已完成且合计不为零的 SELL 评价形成预估客户费用，金额整组出自该评价。BUY 评价不形成客户费用。
- 原币金额、合同结算币金额、换算依据是从同一个评价采用来的一组；同币种时两额必须相等且没有换算步骤。换算已在评价内完成，本上下文不重算。
- 确认前，责任法人、结算相对方、收付方向、结算账户、依据、主要计费范围、来源事实登在确认事实册上；册上没有这一行，确认停在未决。
- 确认条件按费用项目点名要哪一种确认依据；用哪种依据确认哪个费用项目是租户取值。依据记下后，这一户采用了确认时点就当场确认，没采用只记依据。

**供应商成本**

- 供应商预期成本首版按 BUY 评价回指的那份请求形成，一份请求至多一份首版；评价不是已完成、或合计为零，不形成成本行。每一版记下所依协议的供应商，取自 `party-commercial`，不设默认。
- 计价纠错是重述全额的新成本版本，不是带方向的差额。
- 发生项某一版失效时，依它的各条成本链头各追加一版失效纠错版本；当前有效供应商预期成本不采用它。替代版本的评价形成时接在后面重述全额。一版至多被纠正一次。

**供应商账单与审核**

- 预期成本、账单主张、审核应付分别存在，主张不因收到而成为审核应付。账单一行点名的预期成本必须是这张账单的供应商的、且是当前有效的那一版；供应商不同或没记供应商都不接收。
- 同一供应商的同一账单号只标识一份主张；主张身份才是接收的幂等键。审核按行按金额范围给结果，同一行上各段互不重叠；通过的各段之和不超过这一行的已匹配金额。
- 争议的那一段不形成审核应付，也不阻塞其他金额；它只以点名它的再审解除，再审结果只能是通过或拒绝。
- 审核授权、应付归哪个结算账户、金额上限各自另册登记；任一册没登记，审核停在未决或不形成应付，不默认放行。
- 已匹配金额大于已登记上限必须升级。比的是整行的已匹配金额，不是某一段。上限 0 是一份登记。

**对账与争议**

- 结算账户登记册上，责任法人、结算相对方、收付方向、结算币种、结算政策对象五格在租户内唯一；改任一格是另一个账户。
- 费用要已确认、有归属日、尚未出账，且归属日不晚于截单命令点名的本期最后一个归属日，才入对账单。登了对账周期的账户到点自动截单，没登的只由授权人员发起。
- 归属日按费用项目登记的形态，在结算账户对账安排的业务时区里落成公历日，过了截单时刻归下一日。形态或对账安排没登记，不形成归属日。
- 对账单总额必须等于所含费用结算金额之和。发布后单号、范围、金额不可覆盖；之后的调整与迟到费用只经后续对账纳入进入后续账期。
- 发布幂等按发布意图：同一账户、周期、截止版本，内容相同答原单原号，内容不同答发布意图冲突。
- 争议只对已发布的对账单提出，指向明确费用及金额范围；作废之后才提出的不登记。对账单、争议、审核应付各在开立的同一事务内取得单据号。

**资金、核销与余额**

- 外部资金事实进产品只有本上下文的采用这一口：集成客户端送来，或持授予的财务人员手工采用。
- 映射、核销、核销撤销分别成立，都不改原费用、对账单或资金事实；撤销是反向关系，不删历史。供应商净付款同时用到审核应付与贷项时，带方向的分配之和必须等于真实付款金额。
- 一条确认到账的事实可以整条认作代收本金；认作之后不核销、不进入账余额，与这条事实上任何别的映射互斥。
- 入账余额与已确认未结在映射、核销、撤销、费用确认的同一事务里按唯一求法重算，不另推增量。已确认未结求出为负是账不平，推进失败，不截成 0。
- 一条事实落到几个客户应收作用域时，没核销的部分不计入任何作用域，由核销去分。
- 外部更正一条已采用事实的金额时照采，不因超核拒收，也不自动撤销核销；答复带出超核金额。

**接受前财务控制**

- 只执行合同策略明确要求的控制项，各自返回结果；不汇总成接受或拒绝决定，也不自设接单门槛。合同规定无控制时照实答不适用，不用零金额冻结冒充。
- 冻结金额是按结算账户的受理前估价采用登记形成的估价（逐件 SELL 试算合计）。估价只留痕，不形成费用。
- 接受失败、拒绝或放弃时，冻结与暴露按原业务关联显式释放；重复请求不形成重复冻结或重复释放。同一作用域上的施加与释放依次判断，后一笔按前一笔已落的账判。
- 费用确认的事务里，把估价中这件包裹的份额从冻结或暴露整份、一次转出；实际确认额完整入账，不截断到冻结金额。

**赔付、追偿与代垫（编排在，无入口，见末节）**

- 赔付、索赔费用退款、应追偿、追偿认可金额各自形成并可追加调整，互不覆盖；赔付不等待追偿。
- 金额文法：主张减免赔，不足记 0；按万分比向下取整；以限额封顶。算出 0 不形成金额。数值没登记不形成金额，全零是一份登记。
- 实际代垫按税费义务、资金事实和金额范围形成不可覆盖的判断版本；回收成立与否独立判断，变化追加调整。

**调整、分摊与经营结果（分摊与经营结果无入口，见末节）**

- 普通客户费用的调整只有计价纠错与商业让利两类；纠错必须引用新评价，商业让利必须引用有效授权。调整只挂在已确认费用上，同一调整标识重投交回原来那一笔。调整登记、对账单作废、后到费用纳入、调整纳入是四种分开授予的运营决定，时刻一律由服务端给；作废必带依据、只成立一次；一笔后到费用、一笔调整各只纳入后续账期一次，已在原账单上的费用不是后到费用，纳回原账期不受理。
- 分法按租户与分摊规则版本登记，没登记答未配置，不默认按重、不均摊。已分摊金额与未分摊余额之和等于来源金额；尾差按最大余数分配，余数相同按目标标识升序；没有正权重时整笔未分摊。
- 经营结果按口径成对采用组成：预估口径用客户预估费用与供应商预期成本，已确认口径用客户运营应收、审核应付与贷项，已结算口径只用核销分配。审核应付与贷项各按方向计入一次。

## 生命周期

- **评价请求**：已提交 → 上表各结论之一；停下的经续办另起一代。
- **费用明细**：预估 → 确认 →（调整追加）。形成即定阶段，预估不随时间自动变暂估。**供应商预期成本**：首版 →（纠错版本、失效纠错版本）追加成链，只有链头当前有效。
- **供应商主张**：已接收并匹配 → 各段通过、争议或拒绝；争议 → 再审。通过与拒绝是终局。
- **对账单**：草稿（可重算）→ 已发布 →（已作废，原单保留）。争议：已登记 → 接受、部分接受或拒绝为终局；待复核可再处置。
- **资金冻结 / 信用暴露**：占用 → 已转出（费用确认）或已释放。
- **外部资金事实**：已采用 →（更正版本）；映射 → 核销 →（核销撤销）。

## 入口

在线口（路由登记在 `cmd/parcel-api`，处理器在 `adapters/http`）：

| 做什么 | 路径 |
|---|---|
| 运营决定 | `/settlement-charge-confirmations` `/settlement-statement-publications` `/settlement-supplier-bill-receptions` `/settlement-supplier-bill-audits` `/settlement-statement-disputes` `/settlement-statement-dispute-resolutions` `/settlement-charge-adjustments` `/settlement-statement-voids` `/settlement-late-charge-inclusions` `/settlement-adjustment-inclusions` |
| 资金事实采用（集成客户端） | `/settlement-external-funds-fact-registrations` `/settlement-external-funds-fact-correction-registrations` |
| 资金事实采用（人工） | `/settlement-external-funds-fact-manual-registrations` `/settlement-external-funds-fact-correction-manual-registrations` |
| 只读 | `/settlement-charges` `/settlement-statements` `/settlement-disputed-statements` `/settlement-statement-lines` `/settlement-pre-acceptance-estimates` `/settlement-funds-applications` `/settlement-operating-results` `/settlement-supplier-bill-lines` `/settlement-audited-payables` |

事件消费（`cmd/parcel-dispatch`）：发生项已形成发起 BUY 评价并处理失效；评价已记录形成预估客户费用与供应商预期成本；评价未形成记停因；参数已登记触发续办；有效交付已终局记确认依据；到点截单；税费付款核对的采用；接受前控制的施加与释放。

离线口 `cmd/parcel-settlement-register`（数据库网络内手跑，不监听端口），子命令：

- 资金：`external-funds-fact` `external-funds-fact-correction` `funds-mapping` `funds-application` `funds-application-reversal`。映射、核销与撤销只有这一路。
- 账户与对账：`settlement-account` `statement-arrangement` `statement-cycle` `charge-attribution` `settlement-moment`。
- 评价与费用：`sell-billing-moment-adoption` `pre-acceptance-estimate-adoption` `fee-item-mapping` `buy-evaluation-trigger` `sell-evaluation-trigger` `charge-confirmation-facts` `charge-confirmation-condition`。
- 审核与金额规则：`supplier-audit-authority` `supplier-payable-account` `audit-escalation-ceiling` `accounting-connector` `claim-amount-rule` `amount-grammar` `allocation-form` `periodic-fee`。
- 运维：`evaluation-resume`（运营续办）`expected-cost-supplier-backfill`（给还没记供应商的预期成本补上）。

## 旧文档写了、代码里还没有的

- **有编排、没有生产入口**：下列用例方法在 `application/` 里有实现和测试，`cmd/` 下没有任何端点、消费者或子命令调用它们——成本分摊与经营结果的形成（`AllocateCostsHandler`，只读口 `/settlement-operating-results` 因此无数据来源）；赔付、应追偿、追偿认可及其调整（`SettleClaimAmountsHandler`）；实际代垫判断与客户代垫回收（`AssessAdvanceRecoveryHandler` 的 `Assess`、`FormRecovery`、`Adjust`）；供应商费用贷项（`Credit`）。
- **SELL 评价按合同声明的有序计价范围列表选卡**：搜 `PricingScopes`、`PriceSource`、`ScopeList` 无结果。请求上只有单个 `PricingScope`，取自结算账户的采用登记。「合同显式声明不计销售价」同样没有。
- **两本采用登记上同一结算账户的计价范围必须相同**：在 Go 代码与迁移里没找到跨册比对。
- **仓储作业收费发生项那一路**（SELL 请求、仓储计费采用登记、`仓储作业已发生`确认依据、更正版本引起的计价纠错与全额贷记）：搜 `Warehouse`、`仓储` 无对应类型。属可选的仓配履约。
- **低毛利提示**：搜 `Margin`、`毛利提示` 无对应。
- **自动核销**（未指定范围时先精确匹配、再按到期日与账期顺序）：核销只按命令写全的分配形成。
- **对账维度与结清维度的状态**：不记相对方确认，对账单上没有这两维状态；替代单与原单之间也没有可读的替代关系。
- **暂估**：阶段 `PROVISIONAL` 在类型上存在，没有任何形成路径给出它。**周期费用的形成**：三套形态的计算函数与登记口都在，没有任何编排调用它们形成费用。
- **服务税金额、报告币金额与汇兑差额、内部法人间结算、净额抵销协议、坏账处置与坏账收回、催收状态、人工调整的授权规则与升级**：均无对应类型；调整只带一个商业授权引用。
- **已接受委托下某件包裹取消或失败时释放它的份额**：没有触发口；份额只在费用确认时转出。
