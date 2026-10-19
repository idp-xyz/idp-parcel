# ADR-0200：BUY 评价按回指的请求形成供应商预期成本——版本标识由请求派生，评价停下的各格入账

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/08。越权风险点见文末，待 SA owner 复核。）
Date: 2026-10-19

## Context

- BUY 消费门（票 sa-cc/01）对每一封评价已记录信封都答「来源引用未登记」。形成命令缺发生项、费用项目与供应商协议的引用。它们记在 BUY 评价请求册上（票 sa-cc/08），评价也已回指请求标识（票 sa-cc/11）。可结算读口没带回指，形成编排也没装进派发。
- 结算没有供应商预期成本版本标识的铸造口。
- 计价按回指至多为一份请求形成一份评价（计价迁移 `0010_evaluation_request_reference.sql` 的部分唯一索引）。评价不可变。
- BUY 请求册只记「问了什么」，没有状态列（结算迁移 `0019_evaluation_request.sql`）。
- 卡没声明金额取整时，BUY 读口报错，信封落发布失败，重投到失败预算耗尽。

## Decision

**一、按回指形成。** BUY·供应商成本评价已记录之后，结算按评价回指读回 BUY 评价请求，以请求上的合格来源引用（发生项、费用项目、供应商协议）形成供应商预期成本首版。金额、币种、换算步骤与规则版本仍整组出自评价。

**二、版本标识由请求派生，写作 `BUYSC-<请求标识>`。** 不新增铸造口。一份请求至多一份评价，所以重放与并发算出同一个标识，库主键挡住第二份。派生的只是首版；计价纠错的版本标识由纠错那条路给。

**三、评价停下的各格入账，不重投，不改记任何册。** 这几格是：不可计价、评价冲突、待判断、评价失败、跨币种却没带换算步骤、卡没声明金额取整。同一发生项、费用项目与规则版本已有另一个标识的首版时，同样入账。评价不可变，计价也不会为同一份请求再形成一份，重投只会耗尽失败预算（[ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 的判据）。为此卡没声明金额取整改成 BUY 读口的一个结果格，不再是错误。

**四、回指与矛盾的去向，与 SELL 那一路同构**（先例 [ADR-0198](./0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md) 决定四、五）。没有回指的评价不归本编排，零写入，入账。下列各格上抛错误，落发布失败：

- 回指查不到 BUY 请求；
- 派生版本上已有另一份内容；
- 评价已完成，形成时却未受理，例如合计为零。

BUY 请求不带结算币种，不设币种核对。跨币种由形成门按评价内的换算步骤判（`AT-SA-177`）。

**五、只有依赖答不出才是未决。** BUY 请求册、评价读口、预期成本库答不出时，整笔回滚等重投。未决结果带依赖原样答出的错误。

三类：以上都是**机制**，不含租户取值。

## 候选与反方

- **铸造版本标识，另立「请求 → 版本」对照。** 多一张表，换不来什么。否决。
- **按评价标识派生。** 同样确定。但读的人从版本标识看不出出自哪份请求，也与 SELL 一侧按请求派生不同形（[ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定五）。否决。
- **评价停下的各格按未决重投。** 评价不可变，重投只耗预算。否决。
- **给 BUY 请求册加状态列，像 SELL 那样记结论。** 要迁移、写口与读口。评价停在哪一格，今天能按回指从计价读回。留给 owner，见风险点 1。
- **金额未声明照旧报错。** 重投无益，只耗预算；也与 SELL 一侧不同口。否决。

## Consequences

- 代码：`application.FormRequestedSupplierExpectedCostHandler`（哨兵 `ErrBuyEvaluationRequestNotRecorded`、`ErrSupplierCostVersionContradiction`、`ErrUnexpectedRequestedExpectedCostFormation`）；`FormSupplierExpectedCostHandler` 多金额未声明一格，未决带根因；`ports.BuyEvaluationAdoption` 带回指，多金额未声明一格；BUY 消费门与派发上 `buyEvaluationRecordedUndecidedSentinels`。读口的 `ErrAmountPrecisionUndeclared` 与消费门的 `ErrSourceReferencesUnrecorded` 退役。
- 没有迁移。路由仍是 `parcel-pricing.evaluation.recorded` 扇出到 BUY 与 SELL 两路。
- UC-SA-002 补 `AT-SA-182` 至 `AT-SA-184`；settlement-accounting `CONTEXT` 补一句。

## 越权风险点

1. **评价停下无痕**（SA owner）。BUY 请求册没有状态列。结算这一侧分不出「计价没形成评价」与「形成了但采用不了」，要按回指去计价读评价。UC-SA-002 生命周期表「待判断/冲突/未受理/未形成」那一行要求带失败阶段和续办入口，本路是例外。
2. **不可计价不存排除依据**（SA owner）。`AT-SA-175` 要求保存排除依据与评价引用。本路不落行，排除依据在评价上。
3. **请求卡死**（SA owner）。评价采用不了之后，补登卡上的取整声明或汇率序列也不回补；同一组来源引用再请求，会命中原请求。续办路径另立票。
4. **同三维已有首版即止**（SA owner）。另一份请求点名同一发生项的新版本、同一费用项目，评价又命中同一规则版本时，本路只答「已有首版」，不追加计价纠错。发生项有效性更正怎样驱动纠错，另立票。
5. **回指查不到就上抛**（SA、PP owner）。本条假定 BUY·供应商成本评价的回指只会指向 BUY 请求册。别的来源日后若形成带回指的 BUY 评价，这里会响亮失败，届时另裁。

## Links

- [ADR-0198：包裹计费时点那一路，SELL 请求怎样下结论，矛盾怎样上抛](./0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md)：同构先例
- [ADR-0190：包裹计费时点发起 SELL 评价，形成预估客户费用](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md)：决定五，SELL 一侧由请求派生费用标识
- [ADR-0164：BUY 评价请求的触发面是发生项形成，原因没登记就不发起](./0164-buy-evaluation-request-trigger-is-occurrence-formed.md)：BUY 请求从哪里来
- [ADR-0107](./0107-evaluation-amount-rounding-is-declared-by-the-price-card-like-weight-rounding.md)：决定五，消费方不在评价之外取整
- [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)：重投与否按恢复动作分
- [UC-SA-002](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md)：`AT-SA-182` 至 `AT-SA-184`
- 票 `.scratch/demo-chain-gaps/issues/08-buy-evaluation-recorded-consumer-never-forms-supplier-expected-cost.md`
