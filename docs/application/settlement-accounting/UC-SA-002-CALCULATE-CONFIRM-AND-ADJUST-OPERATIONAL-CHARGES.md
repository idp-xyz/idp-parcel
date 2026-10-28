# UC-SA-002 计算、确认并调整运营费用

状态：纯计价评价接入与费用生命周期已形成；真实合同、价卡、计量来源、账户、币种和控制策略待提供，生产分支保持 `No-Go / 待参数化`

## 目标与边界

本用例从一个明确的包裹、面单交易、实际履约段、集运单元、申报单元或结算账户周期及其可采用的来源事实开始，消费 `party-commercial` 的已解析定价依据和 `parcel-pricing` 的 `PricingEvaluation`，形成客户/供应商计费重量的财务采用结果、结算计算展开和普通客户费用/供应商预期成本。费用可以经历预估、暂估和确认；本用例只对这些普通费用形成计价纠错或商业让利调整。合同要求时，本用例还形成接受前估价、资金冻结、信用暴露、转扣费或显式释放结果。

本用例包含：

- 采用当前有效测量、履约、合同、已解析定价依据、`PricingEvaluation`、责任法人、结算账户和币种依据，并保留各版本和业务时点。
- 分别采用客户方向与供应商方向的计价重量结果；任一方变化不覆盖另一方、原始测量或原评价。
- 保存 `PricingEvaluation` 的基础价、附加费、折扣、最低收费、保底量、阶梯规则和取整顺序，并补充费用确认、责任和主要范围的结算解释；本用例不重新实现价卡算法。结算账户周期上的最低消费、保底量、阶梯返利按周期费用形态另算，不改评价。
- 消费运输履约提供的订舱、取消、失败尝试及原/替代旅程运输收费发生项，按采购规则分别形成供应商预期成本。
- 形成普通客户费用、服务费和供应商预期成本的预估、暂估、确认，以及由计价纠错产生的追加调整。SELL·客户费用评价形成客户费用；BUY 评价请求身份不承载这一方向。
- 对普通客户费用形成经过有效授权的商业让利；既有客户应收因此减少时可表现为非索赔客户费用退款义务，但仍以计价纠错或商业让利作为调整原因。
- 在合同明确要求时，对提交版本形成估价、冻结、信用暴露或业务限制结果，并在失败、取消或依据失效时按原关联释放或补偿。

本用例不执行：

- 修改商业合同、价格政策、定价方案绑定、可执行价卡、结算政策、信用政策或人工调整授权；这些版本由 `party-commercial` 与 `parcel-pricing` 按各自边界拥有。
- 修改原始测量、节点扫描、运输履约、关务、异常责任或外部资金事实。
- 重算 `PricingEvaluation` 已完成的原币到合同结算币换算，或为该换算另选汇率；本用例只保存评价内的换算步骤作为换算依据。报告币换算与汇兑差额仍由本上下文另行形成，不覆盖原币和结算币金额。
- 形成客户对账单、供应商账单主张、审核应付、真实收付款、法定发票、总账或 COD 本金。
- 形成供应商账单贷项、客户赔付、索赔责任导致的客户费用退款、应追偿金额或追偿认可金额；它们分别由 [`UC-SA-004`](./UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md) 和 [`UC-SA-007`](./UC-SA-007-SETTLE-CLAIMS-AND-RECOVERY-AMOUNTS.md) 形成。
- 因对账争议、资金退回或核销结果创建新的金额责任；对账只由 `UC-SA-003` 纳入本用例已经形成的调整，真实收付映射和核销只由 `UC-SA-005` 形成。
- 把冻结解释为费用确认、客户已经付款、委托已接受或服务已经发生。
- 把关务实际代垫或客户代垫回收直接当作普通价格规则；关务专项由 [`UC-SA-001`](./UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md) 独立编排。

## 元数据

| 项目 | 内容 |
|---|---|
| 用例 ID | `UC-SA-002` |
| 主责上下文 | `settlement-accounting` |
| 主要参与者 | 结算运营、商业合同责任方、计价运营、节点运营、运输运营、托运业务、信用控制责任方 |
| 业务决定所有者 | `settlement-accounting` 通过本用例拥有财务采用的计费重量、普通费用明细、估价、冻结、信用暴露、计价纠错和商业让利结果；`parcel-pricing` 拥有纯评价和价卡版本；其他金额调整由各自用例拥有 |
| 触发 | 委托接受前财务控制请求、有效测量/履约事实到达、费用确认条件满足、源事实更正、合同允许的人工调整或既有费用需要重判 |
| 成功结束 | 对明确主要计费范围形成估价、冻结、费用明细或追加调整，并保存计算展开 |
| 负向结束 | 适用规则明确证明该范围不产生相应费用或控制结果不适用；包裹计费时点那一路与 BUY·供应商成本那一路上，评价已完成而合计为零属此格，请求册记`负向结束（零额）`，不建费用（成本）行、不续办（`AT-SA-215`） |
| 待判断结束 | 来源测量、计价时点、规则、责任、币种或报告币换算依据不足（结算账户不在册、触发册或确认依据没登记归「未配置结束」）；原币与合同结算币不同而评价未携带换算步骤时同样保持待判断，不由本用例补算 |
| 未配置结束 | 租户取值没登记，如实答未配置、不填默认值，补登之后再办。按编排：BUY 发起的发生项已形成那一路（`TriggerBuyEvaluationOnOccurrenceFormedHandler.Trigger`，装配在 `cmd/parcel-dispatch` 的 `assemble_evaluation_resumption.go` 与 `cmd/parcel-settlement-register` 的 `evaluation_resume.go`）答 `BUY_EVALUATION_TRIGGER_UNCONFIGURED`，原因 `OCCURRENCE_REASON_NOT_REGISTERED` 或 `FEE_ITEM_NOT_REGISTERED`，记进 BUY 发起停办册；ADR-0164 那条触发面（`TriggerBuyEvaluationHandler`）同答 `BUY_EVALUATION_TRIGGER_UNCONFIGURED`，但不带原因、不记停办；SELL 发起（`RequestSellEvaluationOnParcelMomentHandler`）答 `SELL_EVALUATION_REQUEST_UNCONFIGURED`，原因 `SETTLEMENT_POLICY_NOT_ADOPTED`、`SETTLEMENT_ACCOUNT_NOT_REGISTERED` 或 `SELL_BILLING_MOMENT_NOT_ADOPTED` 时记进 SELL 发起停办册（结算政策没采用没有续办路，[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定四与越权风险点 13），原因 `SELL_BILLING_MOMENT_DIFFERS`（这一户采用了别的时点）同答这一格但不记停办（`AT-SA-191`）；确认触发门（`AdmitSettlementMomentHandler`）答 `SETTLEMENT_MOMENT_UNCONFIGURED`，不调用确认（[ADR-0170](../../adr/0170-settlement-orchestrations-enter-through-parcel-api.md) 决定二）；确认（`ConfirmChargeHandler`）答 `CONFIRM_UNDECIDED`，原因 `CONDITION_UNCONFIGURED` 或 `CONFIRMATION_FACTS_UNCONFIGURED`；接受前控制（`ApplyPreAcceptanceControlHandler`）答 `CONTROL_NOT_FORMED`，原因 `CONTROL_POLICY_NOT_CONFIGURED`、`CREDIT_BASIS_NOT_CONFIGURED`、`CONTROL_AMOUNT_NOT_CONFIGURED` 或 `ESTIMATE_PRICE_CARD_NOT_CONFIGURED` |
| 冲突结束 | 合格输入对相同范围、版本或金额互相矛盾，无法依据已确认规则裁决 |
| 未受理结束 | 请求主体、租户、责任法人、客户账户、主要范围或权限不能合法关联 |
| 未形成结束 | 计算、判断、持久化或结果发布因技术原因未完成；与 SELL 请求上的`未形成`结论不是一回事，那是评价失败或计价没形成评价（`AT-SA-180`、`AT-SA-198`） |

## 权威依据

| 引用键 | 权威内容 | 来源 |
|---|---|---|
| `SA-LANG` | 客户/供应商计费重量、费用项目、计算展开、当前有效客户预估费用、费用明细、资金冻结和运营应收 | [结算与经营核算上下文：Language](../../domain/settlement-accounting/CONTEXT.md#language) |
| `SA-RULE` | 评价财务采用、金额精度、费用生命周期、冻结与补偿规则 | [结算与经营核算上下文：Rules and invariants](../../domain/settlement-accounting/CONTEXT.md#rules-and-invariants) |
| `PC-RULE` | 合同、销售/采购商业价格政策、定价方案绑定、结算政策和调整授权版本 | [参与方与商业上下文](../../domain/party-commercial/CONTEXT.md) |
| `PP-RULE` | 可执行价卡、费率表、计价策略、版本清单、纯评价和回放 | [小包计价上下文](../../domain/parcel-pricing/CONTEXT.md) |
| `MEASURE` | 原始测量和当前有效实测 | [节点作业上下文](../../domain/node-operations/CONTEXT.md) |
| `FULFILLMENT` | 实际履约、运输收费发生项、供应商协议和履约成本来源 | [运输履约上下文](../../domain/transport-fulfillment/CONTEXT.md) |
| `PS-SOURCE` | 包裹、面单交易、客户承诺和服务要求快照 | [小包托运上下文](../../domain/parcel-shipment/CONTEXT.md) |
| `ADR-FACT` | 来源事实、有效事件和派生结果分层 | [ADR-0005](../../adr/0005-source-facts-effective-events-derived-state.md) |
| `ADR-FIN` | 运营结算与法定财务分离 | [ADR-0007](../../adr/0007-separate-operational-settlement-from-statutory-finance.md) |
| `ACCEPTANCE` | `SET-01`、`SET-03`、`SET-08`，以及 `SET-10` 的供应商预期成本部分 | [首发试点验收场景矩阵](../../product/PILOT-ACCEPTANCE-MATRIX.md) |

## 输入语义契约

| 输入组 | 权威来源 | 最低业务语义 | 使用约束 |
|---|---|---|---|
| 业务范围 | `parcel-shipment`、`transport-fulfillment` 或账户周期 | 稳定对象身份、租户、责任法人、客户账户、主要计费范围和关联来源 | 不得用显示编号或报表筛选临时推导主要范围 |
| 测量事实 | `node-operations` | 实测身份/版本、测量类型、单位、业务时间、有效性、更正关系和对象范围 | 结算只消费，不覆盖原始测量或当前有效实测 |
| 运输收费发生项 | `transport-fulfillment` | 稳定发生项、采购责任法人、服务提供方、协议快照、原因、主要范围、成员、数量/单位、业务时间、有效性和来源 | 发生项不是金额、账单主张或审核应付；原/替代旅程不得合并覆盖 |
| 客户商业依据 | `party-commercial` | 合同/产品/商业价格政策/定价方案绑定/结算政策版本、适用范围、预付或账期方式、计费口径、税务分类和调整授权 | 缺规则或适用时点不使用默认价格；不从货主客户账户推导结算模式 |
| 纯计价评价 | `parcel-pricing` | 输入快照、计算目的、价格方向、版本清单、计价重量、价卡命中、费用行、精度、取整、解释，以及原币金额、所引用的汇率序列版本和换算步骤 | 结算只消费并保存采用关系；不得把评价直接当作应收应付。原币到合同结算币的换算已在评价内完成，结算采用其换算步骤，不重算也不改用其他汇率 |
| 供应商商业依据 | `party-commercial` / `transport-fulfillment` | 供应商协议、采购价格、责任范围、币种和履约条件快照 | 不把承运接受或账单到达直接解释为审核应付 |
| 费用确认条件 | `settlement-accounting` | 费用类型的预估、暂估、确认和调整条件 | 不等待无关费用或包裹终局才确认 |
| 财务控制请求 | `parcel-shipment` / `party-commercial` | 委托与提交版本、已唯一解析的结算政策及预付/账期适用范围、控制策略、估价范围、账户、授信额度和有效时点。请求不带控制金额：金额由本用例按该账户的受理前估价采用登记形成（[ADR-0179](../../adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md)），没登记答未配置 | 本用例返回控制结果，不形成委托接受或拒绝；同一金额/控制范围存在模式冲突时不执行控制 |
| 既有结算历史 | `settlement-accounting` | 原费用、冻结、释放、调整、对账和发布引用 | 只用于差额、幂等和追加关系，不覆盖历史 |

## 结果语义契约

| 结果 | 业务含义 | 必须保留 | 禁止行为 |
|---|---|---|---|
| 计费输入已接收 | 资料可进入计价判断 | 采用版本、范围、业务时点、缺口和接收记录 | 不表示费用或冻结成立 |
| 纯计价评价已完成 | `parcel-pricing` 已按固定版本清单形成可复算结果 | 评价身份、目的、方向、版本清单、费用行、解释和证据引用 | 不表示费用确认、应收应付、付款或委托决定 |
| 换算依据已保存 | 评价内已完成的原币到合同结算币换算被采用为本上下文的换算依据 | 原币金额、汇率序列版本、换算步骤和评价引用 | 不重算、不改用其他汇率、不以报告币换算覆盖原币或结算币金额 |
| 客户计费重量已形成 | 按客户合同和计量规则确定客户收费依据 | 原始测量、命中规则、单位、精度、取整和版本 | 不覆盖供应商计费重量或原始测量 |
| 供应商计费重量已形成 | 按采购协议和计量规则确定成本收费依据 | 同上及供应商协议版本 | 不因客户规则变化而重写 |
| 供应商预期成本已形成 | 运输收费发生项、采购商业依据和 BUY `PricingEvaluation` 形成内部预期金额 | 发生项、协议、评价、费用项目、主要范围、数量/单位、计算展开、币种和版本 | 不冒充供应商账单主张、审核应付或付款 |
| 客户费用预估/暂估已形成 | 金额可用于经营预测，但尚未满足最终确认条件；受理前估价与冻结不引用它（[ADR-0179](../../adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 决定一）；预估阶段可以派生当前有效客户预估费用 | 费用项目、范围、规则、计算展开、币种、有效期及预估版本采用关系 | 不当作客户运营应收、最终应收应付或会计收入 |
| 费用已确认 | 费用类型的确认条件已满足 | 责任法人、相对方、方向、账户、金额和来源事实 | 不覆盖原测量或规则版本 |
| 计价纠错/商业让利已形成 | 计价输入或适用规则更正，或有效商业授权，改变普通费用金额责任 | 原费用、明确调整类型、证据、新旧依据、借/贷方向和账期 | 不改写原费用或已发布对账单，不形成赔付、索赔退款、追偿或供应商账单贷项 |
| 资金已冻结/已释放/已转扣费 | 依据合同控制策略占用、释放或转为正式费用 | 控制策略、账户、范围、金额、状态和补偿关系 | 不表示收款、支付或委托决定 |
| 计价明确排除已接收 | `parcel-pricing` 返回不可计价，即按该价卡不存在可成立的价格 | 评价身份、方向、目的、版本清单和排除依据 | 不形成任何费用行（含零金额），不自动重试——登了新一版价卡之后经续办另起一代（[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定五），也不推导包裹终局、拒绝接单或退运决定 |
| 待判断/冲突/未受理/未形成 | 分别表示业务依据不足、合格输入冲突、权限/范围不合法或技术未完成；计价那一段没有在用价卡不在这一行，走 SELL 请求上的`未形成`连同停因、BUY 请求上的停因（`AT-SA-198`） | 缺口/冲突/失败阶段和续办入口 | 不互相替代或伪装为零费用；不可计价是确定结论，不并入本行 |

## 主流程

| 步骤 | 责任方 | 处理 | 结果 |
|---:|---|---|---|
| 1 | `settlement-accounting` | 校验租户、责任法人、客户账户、主要范围、权限和请求身份 | 已受理或未受理 |
| 2 | `settlement-accounting` / `parcel-pricing` | BUY·供应商成本：运输履约交出发生项已形成信封时，结算按引用回读那一份发生项，发生项与供应商协议取自它、费用项目取 BUY 触发册那一行、主要范围取协议的商业范围，提交主要范围、计算目的和合格来源引用；没登原因或没登费用项目答未配置（`BUY_EVALUATION_TRIGGER_UNCONFIGURED`，原因 `OCCURRENCE_REASON_NOT_REGISTERED` 或 `FEE_ITEM_NOT_REGISTERED`），不发起请求，在 BUY 发起停办册上记下这一拍（[ADR-0208](../../adr/0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md)、[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定四）。SELL·客户费用按包裹计费时点发起：结算账户采用的时点到来时，结算提交包裹、计价范围与计价基准时点（[ADR-0190](../../adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md)）；发生项原因触发册只回答发不发起、不铸造 SELL 请求（[ADR-0169](../../adr/0169-sell-evaluation-forms-a-customer-charge.md) 决定三与越权风险点 2、ADR-0190 越权风险点 4）。`parcel-pricing` 采用测量、运输收费发生项、其他履约、面单及已解析商业依据形成不可变计价输入快照 | 计价输入版本、未配置或待判断 |
| 3 | `parcel-pricing` | 按价格方向和固定版本清单执行客户/供应商评价，分别形成计价重量、基础价、附加费、折扣和取整结果 | 评价完成、待判断、冲突、不可计价或未形成 |
| 4 | `settlement-accounting` | 校验评价与费用项目、责任法人、主要范围、确认条件和结算账户的映射。包裹计费时点那一路按评价回指读回 SELL 请求，结论记在请求上（[ADR-0190](../../adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定五、[ADR-0198](../../adr/0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md)）。BUY·供应商成本评价按回指读回 BUY 评价请求，取请求上的合格来源引用；BUY 请求册不设状态格，采用不了的各格在请求上记停因（[ADR-0200](../../adr/0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md)、ADR-0229 决定三）。评价已完成而合计为零，两路请求都记`负向结束（零额）`（[ADR-0306](../../adr/0306-a-completed-evaluation-totalling-zero-ends-its-request-negatively-without-a-charge.md)） | 可采用的结算依据、待判断或负向结束 |
| 5 | `settlement-accounting` | 保存评价引用、结算采用快照、责任/范围解释和金额展开；原币与合同结算币不同时，直接采用评价内的原币金额、汇率序列版本和换算步骤作为换算依据 | 预估或暂估费用 |
| 6 | `settlement-accounting` | 按费用类型确认条件形成确认费用，或保持原阶段（预估或暂估）；确认同笔按费用项目的归属日形态与所属结算账户的对账安排形成结算归属日，判不出不挡确认（[ADR-0269](../../adr/0269-business-time-zone-and-cutoff-belong-to-the-statement-arrangement-of-the-settlement-account-and-the-cut-admits-by-attribution-date.md) 决定三） | 确认费用、未配置、待判断或不成立范围 |
| 7 | `settlement-accounting` | 按已唯一解析的结算政策范围和商业策略形成估价、冻结、信用暴露或限制；不适用时保存依据 | 独立财务控制结果 |
| 8 | `settlement-accounting` | 对迟到或更正依据形成计价纠错，对有效授权形成商业让利；非索赔客户费用退款只能作为这两类调整的结果 | 计价纠错、商业让利或当前无需调整；其他调整转交其唯一创建用例 |
| 9 | `settlement-accounting` | 发布结果意图并供接受、对账、供应商匹配和经营分析引用 | 稳定结果或可恢复的发布失败 |

## 生命周期与控制分支

### 费用明细

- 预估或暂估 → 确认只在该费用类型的条件满足时发生；阶段由形成那一步给出，预估不随服务发生或计费时点到来自动转为暂估，包裹计费时点那一路形成预估、停在预估直到确认（[ADR-0270](../../adr/0270-a-charge-stage-is-given-by-the-judgement-that-forms-it-and-an-estimate-does-not-become-provisional-with-time.md)）。不适用的阶段可以直接形成确认或不成立，但必须保存依据。
- 同一主要计费范围和截至时点存在多个客户预估版本时，按有效性、更正和替代关系派生当前有效客户预估费用；新预估、失效或替代只形成新采用结果，不覆盖原预估版本。
- 已确认普通客户费用遇到迟到事实或计价规则更正时追加计价纠错；有效商业授权降低普通客户费用时追加商业让利。渠道作废或退款结果本身不产生调整，只有适用计价或让利规则成立时才能形成非索赔客户费用退款。
- 供应商预期成本的计价纠错仍由本用例形成，但不得修改供应商账单主张或审核应付；供应商账单贷项由 `UC-SA-004` 形成。客户赔付、索赔费用退款、应追偿金额、追偿认可金额及其调整由 `UC-SA-007` 形成。
- 订舱、取消、失败尝试和实际履约的运输收费发生项分别计价。一个发生项可以按规则形成零个或多个费用项目，但同一发生项、费用项目和规则版本不得重复形成预期成本。
- 原旅程与替代、改送或退运旅程分别形成预期成本。替代旅程成功、原运输意图取消或后续供应商贷项均不能静默删除原旅程成本；贷项由 `UC-SA-004` 追加，发生项有效性更正才由本用例追加预期成本计价纠错。
- 同一主要范围可以有多个不同费用项目，但同一费用项目和适用规则版本不得形成两个互相覆盖的基础结果。

### 接受前财务控制

- 合同要求估价时，本用例按结算账户的受理前估价采用登记，以逐件 SELL 试算合计形成估价；估价不引用、也不形成费用、预估费用或应收应付，没登记答未配置（[ADR-0179](../../adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 决定一、二、五）。
- 估价采用与 SELL 计费时点采用两本登记上同一结算账户的计价范围必须相同；登任一本时另一本已有这一户而范围不同，答冲突、不落行、不交那一本的登记信封（[ADR-0280](../../adr/0280-a-settlement-account-registers-one-sell-pricing-scope-across-the-estimate-and-billing-moment-adoptions.md)）。
- `可用余额 → 已冻结` 必须关联明确账户、金额、提交版本和控制策略；重复请求返回原冻结。
- 接受失败、拒绝、放弃或冻结依据失效时形成 `已释放`；接受已经成立但响应或事件投递失败时保留合法冻结并等待查询/补偿。
- `已冻结 → 已扣费` 只在费用确认和业务关联条件满足时发生；最终费用高于冻结时完整形成费用并留下差额，不截断金额。
- 合同明确“无财务控制”时保存商业不适用依据，不制造零金额冻结或虚假信用通过。

## 一致性、幂等与并发

- 相同请求身份、相同范围、来源版本和规则版本返回既有结果；相同身份携带不同金额、规则、账户或范围形成请求冲突。
- 相同输入快照、价格方向、计算目的和版本清单必须引用同一 `PricingEvaluation`；评价未形成、版本清单不一致或解释缺失时，结算不得自行重算或使用默认价。
- 同一结算作用域上的接受前控制施加与释放并发时依次判断、互不覆盖，口径见 [结算与经营核算上下文：Rules and invariants](../../domain/settlement-accounting/CONTEXT.md#rules-and-invariants) 运营余额一节。
- 同一货主的预付与账期结果按实际结算政策、结算账户、金额/控制范围和有效期间隔离；一个范围的余额、冻结、授信或逾期不得满足另一范围。相同金额或控制范围存在两种适用模式时保持冲突，不执行任一财务控制。
- 同一原始测量或履约事实重复到达只被采用一次；更正形成新来源版本并触发追加重算，不按接收顺序覆盖。
- 客户计费重量和供应商计费重量分别加锁/判断；一方待判断不把另一方强行改为待判断或确认。
- 规则发布、费用确认、冻结转扣费和对账截单并发时，先保留各版本，再由本用例按业务适用时间形成计价纠错或商业让利；`UC-SA-003` 只将已形成且符合截单条件的调整纳入后续账期。
- 结果已形成但发布失败时只重试同一发布意图；不得重复费用、冻结或扣费。
- 批量计价逐主要范围返回结果；一个范围的待判断、冲突或技术失败不回滚其他已经合法形成的费用。

## 权限、隔离与审计

- 商业规则发布、计价输入采用、费用确认、财务控制、人工调整和结果发布分别授权；结算权限不能推导修改合同或原始测量权限。
- 按集团租户、责任法人、货主客户账户、结算账户、方向、币种和主要计费范围隔离；跨客户集运只能消费已明确归属的范围。
- 本用例中的人工调整只允许计价纠错和商业让利，并记录申请/决定方和授权额度；非索赔客户费用退款只能是这两类调整的金额结果，不能作为无原因的第三类人工调整。供应商账单贷项、客户赔付、索赔费用退款、追偿金额、资金退回映射和核销必须进入各自用例，不能复用一个“冲销”动作。
- 审计至少保存采用版本、业务时间、计算展开、精度/取整顺序、旧结果、新结果、原因、授权和发布恢复记录。

## 验收示例

| ID | 场景 | 预期结果 |
|---|---|---|
| `AT-SA-035` | 有有效测量、合同和客户价格规则 | 形成客户计费重量、计算展开和可引用的费用结果，不修改实测 |
| `AT-SA-036` | 客户规则与供应商规则使用不同重量 | 分别形成客户/供应商计费重量和费用，任何一方变化不覆盖另一方 |
| `AT-SA-037` | 只有客户声明重量，没有合格实测 | 按合同规则形成待判断或明确不成立，不把声明直接当计费重量（预估阶段的 SELL·客户费用另见 [ADR-0190](../../adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定四） |
| `AT-SA-038` | 实测迟到但业务有效时间早于截单 | 本用例依据适用时点形成计价纠错；已发布周期不变，由 `UC-SA-003` 将该既有调整纳入后续账期 |
| `AT-SA-039` | 同一范围命中两个互斥基础价 | 形成规则冲突，不按最后到达或最低价格猜测 |
| `AT-SA-040` | 附加费、折扣、最低收费同时适用 | 保存组合顺序、基数、上限、取整和最终金额，结果可复算 |
| `AT-SA-041` | 周期最低消费覆盖多个包裹 | 以结算账户周期为主要范围形成费用，包裹分摊只作分析引用 |
| `AT-SA-042` | 普通客户费用确认后出现有授权的计价更正 | 原费用保留，由本用例追加“计价纠错”调整，不删除原费用或改写来源事实 |
| `AT-SA-043` | 渠道交易作废但计价纠错或商业让利条件尚未成立 | 保留渠道结果，不自动删除费用或形成客户费用退款；条件成立后才由本用例追加对应类型调整 |
| `AT-SA-044` | 合同要求预付冻结 | 形成估价和独立冻结，冻结不表示费用确认、付款或委托接受 |
| `AT-SA-045` | 冻结请求重复提交 | 返回原冻结及原关联，不重复占用余额 |
| `AT-SA-046` | 接受判断失败且冻结已形成 | 按原关联显式释放，保留冻结和释放审计 |
| `AT-SA-047` | 接受已成立但事件投递失败 | 保留合法冻结和接受前结果，只恢复同一发布意图 |
| `AT-SA-048` | 最终费用高于冻结 | 完整形成确认费用，差额形成欠款/信用暴露，不截断费用 |
| `AT-SA-049` | 合同明确接受前无财务控制 | 保存商业不适用依据，不制造零金额冻结或信用通过 |
| `AT-SA-050` | 规则或币种版本缺失 | 不形成费用，不使用默认价格、币种或汇率；规则缺失时，包裹计费时点那一路与 BUY 那一路上没有在用价卡见 `AT-SA-198`；币种或汇率缺失时，原币与合同结算币不同而评价没带汇率序列版本或换算步骤见 `AT-SA-177` |
| `AT-SA-051` | 同一请求身份携带不同金额 | 形成请求冲突，原结果不被覆盖 |
| `AT-SA-052` | 同一测量通过两个来源重复到达 | 只采用一次；重复来源保留引用，不重复计费 |
| `AT-SA-053` | 客户计费规则更正但供应商规则未变 | 只追加客户费用计价纠错，供应商预期成本保持原版本 |
| `AT-SA-054` | 供应商计费规则更正但客户规则未变 | 只追加供应商预期成本计价纠错，不形成供应商账单贷项，客户费用保持原版本 |
| `AT-SA-055` | 费用确认与截单并发 | 按确认条件和截单快照决定归属；需要调整时由本用例先形成计价纠错或商业让利，再由 `UC-SA-003` 纳入后续账期 |
| `AT-SA-056` | 人工提交“冲销”但未说明语义 | 拒绝无语义调整；本用例只接受计价纠错或商业让利，其他类型转交唯一创建用例 |
| `AT-SA-057` | 批量含确认、待判断和技术失败范围 | 逐范围返回，合法确认不被局部失败回滚 |
| `AT-SA-058` | 检查本用例形成的对象 | 只形成计费依据、普通费用、控制结果、计价纠错和商业让利；不形成对账单、真实收付款、核销、赔付、索赔退款、追偿或供应商账单贷项 |
| `AT-SA-161` | 外部订舱发生项及采购价格规则有效 | 形成可解释的订舱预期成本，不把订舱确认直接写成账单主张或审核应付 |
| `AT-SA-162` | 原订舱取消产生取消发生项，随后备用伙伴新旅程成功 | 原旅程订舱/取消与新旅程分别形成预期成本，不能用新旅程成本覆盖或净额隐藏原旅程 |
| `AT-SA-163` | 两次外包派送尝试分别形成发生项 | 按每次尝试的范围和规则形成预期成本；后次成功不删除前次失败尝试成本 |
| `AT-SA-164` | 运输履约使原发生项失效，供应商账单尚未到达 | 保留原预期成本并追加计价纠错；不创建供应商账单贷项或资金退回 |
| `AT-SA-171` | 同一货主的业务 A 唯一命中预付政策和账户，业务 B 唯一命中账期政策和账户 | A 只形成适用估价/冻结结果，B 只形成适用信用暴露/限制结果；两者分别保存政策和范围，不共用余额、额度或账期 |
| `AT-SA-172` | 同一金额或接受前控制范围同时命中预付和账期政策 | 形成模式适用冲突，不冻结、不形成信用通过或默认无控制，也不向委托接受提供可通过结果 |
| `AT-SA-173` | 相同输入快照、方向、目的和版本清单重复请求 | 引用同一 `PricingEvaluation` 并幂等返回既有费用采用，不在结算中重复算价 |
| `AT-SA-174` | SELL 评价完成但价格费用代码没有唯一费用项目映射 | 结算保持待判断，不创建占位费用或把价格代码直接当费用项目 |
| `AT-SA-175` | 节点实测重量超出价卡明确排除范围，`parcel-pricing` 返回不可计价 | 不形成任何费用行（含零金额），也不据此形成包裹终局或退运决定；排除依据与评价引用留在评价上、经计价按回指读得到，SELL 请求记`不可计价`连同停因，BUY 请求记停因；不转为待判断、不自动重试，登了新一版价卡之后经续办另起一代（[ADR-0198](../../adr/0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md) 越权风险点 3、[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定五） |
| `AT-SA-176` | BUY 价卡为美元、合同结算币为人民币，评价已在内部完成换算并携带换算步骤 | 采用评价内的原币金额、汇率序列版本和换算步骤作为换算依据形成结算币金额，不重算也不改用其他汇率；报告币金额另行派生且不覆盖原币或结算币 |
| `AT-SA-177` | 原币与合同结算币不同，但评价未携带换算步骤或汇率序列版本 | 保持待判断并记录缺口，不自行取汇率补算，也不以原币金额直接充当结算币金额 |
| `AT-SA-178` | 评价所引用的汇率序列期次事后被更正 | 由 `parcel-pricing` 以新序列版本形成新评价，本用例据其追加计价纠错；原费用与原换算依据保留，不改写已保存的汇率序列版本 |
| `AT-SA-179` | 包裹计费时点那一路，SELL·客户费用评价已完成，但价卡没声明金额取整 | SELL 请求记`待判断`、停因取整未声明（`AMOUNT_ROUNDING_UNDECLARED`），不形成费用、不换写金额，入账不重投；登一版声明了金额取整的新卡并收尾旧版之后，经续办另起新一代（[ADR-0198](../../adr/0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md) 决定二，[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定三、五，[ADR-0234](../../adr/0234-an-older-price-card-version-leaves-new-selections-only-through-an-explicit-closure-for-a-named-successor.md) 决定四） |
| `AT-SA-180` | 包裹计费时点那一路，SELL·客户费用评价失败 | SELL 请求记`未形成`、停因评价失败，入账不重投；补登参数之后经续办另起新一代（[ADR-0198](../../adr/0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md) 决定一，「停在已提交」一句由 [ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定三停用） |
| `AT-SA-181` | 包裹计费时点那一路，评价回指的 SELL 请求不在册上，或评价结算币种与请求快照不同 | 不形成费用、不改记请求，作两侧矛盾上抛为可见的发布失败，不按待判断或不归本路静默入账（[ADR-0198](../../adr/0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md) 决定四） |
| `AT-SA-182` | BUY·供应商成本评价已完成，回指一份在册的 BUY 评价请求 | 以请求上的发生项、费用项目、供应商协议与评价形成供应商预期成本首版，版本标识由请求标识派生；同一封重投或另一封指同一份评价都不多形成（[ADR-0200](../../adr/0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md) 决定一、二） |
| `AT-SA-183` | 回指在册请求的 BUY 评价不可计价、冲突、待判断、失败、跨币种没带换算步骤，或价卡没声明金额取整 | 不形成任何成本行，不换写金额，入账不重投；BUY 请求册记各自的停因，补登参数之后经续办另起新一代（[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定三）。排除依据照 `AT-SA-175` 留在评价上；`AT-SA-177` 记录缺口在本路仍是例外：缺口在评价上（[ADR-0200](../../adr/0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md) 决定三与越权风险点 2） |
| `AT-SA-184` | BUY 评价回指的请求不在 BUY 请求册上，派生版本上已有另一份内容，或评价已完成而形成时未受理（合计不为零，例如原币为零而结算币不为零；合计为零见 `AT-SA-215`） | 不形成成本，作两侧矛盾上抛为可见的发布失败，不按不归本路静默入账（[ADR-0200](../../adr/0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md) 决定四，「例如合计为零」一句由 [ADR-0306](../../adr/0306-a-completed-evaluation-totalling-zero-ends-its-request-negatively-without-a-charge.md) 停用） |
| `AT-SA-185` | 运输履约交出一份首版发生项已形成信封，原因在 BUY 触发册上登了「发生项形成」与费用项目 | 按引用回读发生项，以发生项、它固定的供应商协议、触发册的费用项目和协议的商业范围登记恰一份 BUY 评价请求并交信封；同一封重投答已存在，不登第二份（[ADR-0208](../../adr/0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md) 决定一至四） |
| `AT-SA-186` | 同上，但原因没登、或登了没登费用项目，或这一版是失效版本（失效之后长出的替代版本照常发起，见 `AT-SA-210`） | 不发起请求、不问协议，入账不重投；未配置点名缺哪一格，并在 BUY 发起停办册上记下这一拍，补登触发册之后经续办发起（[ADR-0208](../../adr/0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md) 决定三、五；「补登之后不回补」由 [ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定四停用）。失效版本不发起、不记；替代版本没登触发行时与首版一样记停办、补登后续办（[ADR-0232](../../adr/0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 决定八收窄 ADR-0229 决定四那一格） |
| `AT-SA-187` | 同上，但发生项读不回、触发册或协议范围读口读不动，或发生项上的协议在参与方与商业查无、协议指称比中不止一版 | 前几种是未决，整笔回滚重投；协议查无是两侧矛盾、指称歧义是确定性坏答案，都上抛为可见的发布失败，不替它挑一个范围（[ADR-0208](../../adr/0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md) 决定四、五） |
| `AT-SA-188` | 任一停因 | 落在发起之前、计价、采用三段之一，且只出现在它那一路的册上（发起之前那几格各属一路；缺换算步骤只在 BUY、映射不全只在 SELL）；词表外的停因拒（[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定三） |
| `AT-SA-189` | 一次参数登记（结算账户、SELL 计费时点采用、费用项目映射、BUY 触发册、价卡、序列、目录） | 只续办它解得开的停因：评价待判断按缺的是什么分格（目录、序列、价表段、包裹事实，分不出落笼统那一格），序列解不开缺区、目录解不开序列读不出，两格都另认价卡（卡没绑那一类时只有新一版卡解得开），价表没有这一段另认目录；结算政策没采用、包裹没有商业解析、计价输入不可得、包裹事实不全哪一类登记都不续办；信封带的收窄格随种类必备（计价三类带计价范围与方向、BUY 触发册带发生项原因、SELL 计费时点采用带户），缺了是毒丸（[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定三、五） |
| `AT-SA-190` | 一份停下的请求续办 | 另起下一代：同一个问题、新标识、代次加一、回指前一代、带续办依据；按自然键读回最新一代；一份请求至多一个下一代；前一代的结论、停因与评价原样（[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定一、二） |
| `AT-SA-191` | 包裹计费时点或发生项已形成走到发起就答`未配置`，或包裹没有商业解析 | 不发起请求、入账不重投，在 SELL 或 BUY 发起停办册上记下这一拍的输入与停因（停在计费时点没采用的另记那一户）；记不下是未决；`采用了别的时点`与失效版本不记（[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定四；替代版本照常记，[ADR-0232](../../adr/0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 决定八） |
| `AT-SA-192` | 回指在册请求的 BUY 评价采用不了 | 在 BUY 请求册上记对应的停因（不可计价、冲突、待判断及其分格、失败、缺换算步骤、取整未声明），同一停因重放、另一停因是矛盾上抛；记不下是未决（[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定三） |
| `AT-SA-193` | 取续办队列 | 发起停办在没落终止、这一拍还没有请求时等着，停下的请求在还没有下一代时等着；只取停因在名单里的；凭被排除的依据续办过的不取；按计价范围、方向、发生项原因与户收窄，只属一路的格另一路整路不取；一次序列喂价只续办绑了它的卡所在范围与方向上的（[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定四、五） |
| `AT-SA-194` | 停在发起之前的那一拍，租户补登了解得开它的参数 | 参数登记同笔交信封（结算账户已登记那一封扇出到小包托运与结算两路），续办按记下的输入重跑发起编排，发出带依据的首代请求，评价与费用（成本）照常形成；重跑又停在下一格就换上这一次的停因与依据；重跑答`采用了别的时点`落终止、不发请求；同一份登记重放不出第二份请求（[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定四、五） |
| `AT-SA-195` | 停在计价或采用那一段的请求，租户补登了解得开它的参数（价卡、映射等） | 续办出下一代，计价按它形成评价，结算以下一代派生的标识形成费用（成本）；前一代原样；没有在等的请求时再来一次登记不推出新一代（[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定一、五） |
| `AT-SA-196` | 运营人员点名一件包裹或一版发生项、给续办引用续办 | 续办它上面停着的每一件（停因不设限）；同一个引用重跑是重放、不出新一代；点名与方向对不上、没有续办引用在事务之前拒；任一件未决整笔回滚（[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定六） |
| `AT-SA-197` | BUY 触发册最新一版没登费用项目，再登一版带费用项目的 | 追加下一版、读口取最新一版、旧版留在册上；任一版再交是重放；已登的费用项目换一个仍答冲突（[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定七） |
| `AT-SA-198` | 计价对一份请求答不出评价（没有在用价卡、多于一版同时适用、造不出计价输入快照） | 计价入账、交「评价未形成」；结算在 SELL 请求上记`未形成`连同停因、在 BUY 请求上记停因；请求不在册上、册上另有结论是矛盾上抛（[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定三） |
| `AT-SA-199` | 结算三本评价参数册（SELL 计费时点采用、费用项目映射、BUY 触发册）落一行 | 同一事务交一封「评价参数已登记」，重放重发同一封（被 Outbox 吞掉）、冲突不交；交不出整笔回滚（[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定五） |
| `AT-SA-206` | 运输履约交出一版失效版本的发生项已形成信封，依被失效那一版形成过预期成本 | 记下那一版已失效，每条链头追加一版失效纠错版本：回指它、原因发生项失效、发生项引用换成失效版本、计价结果原样并标不再成立；原版本与计算展开不动，不形成账单贷项；同一封重投不多一版（[ADR-0232](../../adr/0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 决定七） |
| `AT-SA-207` | 发生项失效之后长出替代版本，另一份请求点名它、同一费用项目，评价命中同一规则版本、金额也相同；或失效还没记到结算时，替代版本的评价先到，或早一版的评价晚于替代版本的评价到 | 前者不再答已有首版即止：按新评价重述全额、接在失效纠错版本后面追加一个纠错版本（原因发生项替代，版本标识由这份请求派生），首版不动；重放不多一份。后两种都不接、零写入，答未决 `OCCURRENCE_INVALIDATION_NOT_RECORDED` 重投；失效记下之后，先到的替代评价接在失效纠错版本后面，晚到的早一版评价答 `OCCURRENCE_INVALIDATED`、入账（[ADR-0232](../../adr/0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 决定八，部分停用 ADR-0200 决定三一句） |
| `AT-SA-208` | 评价所依的那一版发生项已被记作失效 | 不形成任何成本，答 `OCCURRENCE_INVALIDATED`、入账不重投（[ADR-0232](../../adr/0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 决定七） |
| `AT-SA-209` | 失效信封先到、被失效那一版的评价还在路上 | 没有可撤的成本，答已了结，但记下了那一版已失效；之后到的那份评价不再形成成本（[ADR-0232](../../adr/0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 决定七） |
| `AT-SA-210` | 发生项失效之后长出替代版本、交出已形成信封；另交来一版失效版本 | 替代版本照常发起 BUY 评价请求（请求自然键带新版本，是另一份请求）；失效版本不发起（[ADR-0232](../../adr/0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 决定八，部分停用 ADR-0208 决定五一句） |
| `AT-SA-214` | 包裹计费时点那一路，SELL·客户费用评价待判断、不可计价或冲突，或评价已完成而费用行映不全、映到多个费用项目 | 不形成费用（含零金额）、不建占位费用，入账不重投；SELL 请求记`待判断`、`不可计价`或`冲突`连同停因，补登参数之后经续办另起新一代（[ADR-0198](../../adr/0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md) 决定三，[ADR-0229](../../adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定三、五）。结果语义契约「待判断/冲突/未受理/未形成」一行要求保存的缺口与冲突、`AT-SA-177` 记录的缺口，在本路请求册上都只落到停因为止，是例外：缺的是哪个费用代码不落库，拿评价的费用代码对映射册重算；评价待判断缺的是哪一类按 `AT-SA-189` 分格记进停因，缺的具体哪一项（如读不出的那一条序列；本路没有缺换算步骤那一格，见 `AT-SA-188`）与冲突的明细留在评价上，经计价按回指读回；排除依据与评价引用照 `AT-SA-175` 留在评价上（ADR-0198 越权风险点 2、3） |
| `AT-SA-215` | 包裹计费时点那一路的 SELL·客户费用评价，或回指在册请求的 BUY·供应商成本评价，已完成而合计为零（例如扣减恰好扣到零；按合同结算币判，跨币种时是评价内换算后的合计） | 负向结束，不是两侧矛盾：不形成费用或成本行（含零金额；替代版本上也不追加纠错版本），入账不重投；SELL 请求记`负向结束（零额）`，BUY 请求册记同名一格，都不带停因、不进续办队列，同一封重投不改；结算币种对不上请求快照、费用项目映射不全、发生项那一版已失效照各自那一格先答（[ADR-0306](../../adr/0306-a-completed-evaluation-totalling-zero-ends-its-request-negatively-without-a-charge.md) 决定一至三；部分停用 [ADR-0198](../../adr/0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md) 决定四、ADR-0200 决定四、ADR-0169 决定二各一句与 ADR-0232 决定八两句）。生命周期「费用明细」一节「不成立……必须保存依据」在本路是例外，同 `AT-SA-214`、`AT-SA-183`：两本请求册上零额那一格不存评价引用与时刻，零额的依据（评价引用、版本清单、费用行与合计）留在评价上，经计价按回指读回 |
| `AT-SA-217` | 包裹计费时点那一路形成一笔预估客户费用 | 同笔在确认事实册登上七项：结算账户、主要计费范围、来源事实取 SELL 请求，责任法人、相对方、方向取请求所指那一户的固定属性，合同依据取那一户的成立依据；册上已有异内容不覆盖；请求所指的账户不在册上是两侧说法矛盾，整笔不作答、按矛盾上抛（信封留在待投、失败码不是等依赖那一格），由人核对账户册（[ADR-0336](../../adr/0336-confirmation-facts-land-with-the-estimated-charge-and-an-effective-delivery-final-is-the-built-in-confirmation-basis.md) 决定一） |
| `AT-SA-218` | 已形成预估客户费用的包裹终局为有效交付；这一户为该费用项目登了`有效交付已终局`确认条件 | 给这笔费用记一份依据（引用为终局采用键）；登了确认时点就当场确认、交出费用确认信封，没登只记依据；终局不采用或不是有效交付不记；确认条件或确认事实没登答未配置、不重投（ADR-0336 决定二） |
| `AT-SA-219` | 终局先于费用形成（例如按终局计费） | 费用形成那一笔回读到已终局的有效交付，同笔记依据并照 `AT-SA-218` 确认（ADR-0336 决定二） |
| `AT-SA-220` | 受控登记口登确认条件 | 同一费用项目登同一种依据答重放，登另一种答冲突、不覆盖；产品不认得的依据种类当场拒（ADR-0336 决定二） |
| `AT-SA-221` | 持`费用确认`授予的人经运营确认口点名一笔费用 | 先过确认时点门：没登答 `SETTLEMENT_MOMENT_UNCONFIGURED`、不调确认；登了照确认编排答确认、已确认、条件未满足，或确认条件、确认事实没登的未决（都是答案）；时点册或依赖读不动、确认信封没交出去不形成答案；不持这一种授予答未授予（[ADR-0339](../../adr/0339-charge-confirmation-and-statement-publication-are-operator-decisions-and-a-registered-statement-cycle-cuts-on-its-closing-dates.md) 决定一） |

## 生产参数与待决风险

| 参数 | 待提供内容 | 未提供时影响 |
|---|---|---|
| `PAR-SET-01` | 结算账户、责任法人、相对方、方向和币种 | 不能形成生产费用责任或冻结 |
| `PAR-SET-02` | 客户商业价格政策、SELL 价卡/费率表/策略版本、费用项目映射、税务口径和可复算样例 | 只能稳定骨架或隔离验证 |
| `PAR-SET-03` | 供应商协议、运输收费发生条件、BUY 价卡/费率表/策略版本、责任范围、币种和可复算样例 | 不能形成生产供应商成本 |
| `PAR-SET-04` | 原币清单、报告币换算依据、适用时点和版本 | 不得形成生产报告币金额或汇兑差额 |
| `PAR-SET-11` | 计价参考序列的登记实例：燃油当周费率的来源标识与取值凭证，汇率的牌价来源、牌价类型、取值时点与加点规则，以及各自的登记责任方；其中取值时点规则与加点规则只作登记，评价的期次按计价基准时点取、不加点（[ADR-0260](../../adr/0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md) 决定三） | 评价无法携带可采用的换算步骤，本用例的结算币金额保持待判断 |
| `PAR-SET-07` | 周期费用和人工调整规则，或权威 `N/A` | 最低消费、保底量、返利和人工调整不得进入生产 |
| `PAR-COM-08/09/10/15` | 结算模式适用范围、合同币种、截单/付款条件和财务控制策略 | 接受前控制和客户计费分支保持待配置 |
| `PAR-NFR-06` | 计算、冻结、发布和恢复目标 | 不能形成生产运行基线 |

## 给开发的交接

| 规则或验收行 | 现状缺口 | 承接 |
|---|---|---|
| 接受前财务控制：估价采用与 SELL 计费时点采用同户一份计价范围 | 两只登记编排各写各的册、互不读，同一户可以登两个范围 | [code/62](../../../.scratch/docs-consistency/code/62-the-estimate-and-billing-moment-adoptions-may-register-different-pricing-scopes.md) |
| 已接受委托下某件包裹取消或失败时，释放它在接受前占用里的份额（CONTEXT「取消或失败时显式释放」的包裹粒度） | 转出册与账本已能表达部分转出、余额可放；但 `parcel-shipment` 在委托已接受之后不再请求释放占用，没有触发口，该份额一直占着 | [ADR-0343](../../adr/0343-a-confirmed-charge-converts-its-parcels-pre-acceptance-occupation-in-the-confirming-transaction.md) 决定八 |

完成本用例只证明纯计价评价、财务采用、费用生命周期和接受前财务控制能够保持独立、可追溯和追加式；客户对账、供应商审核、真实收付款和经营结果分别由其他 PN-07 用例形成。
