# 09 演示价卡没声明金额取整，结算读口拒收

Category: enhancement
Status: 半收——2026-10-19 SELL 那一半由 [sell-customer-charge/05](../../sell-customer-charge/issues/05-demo-tenant-forms-an-estimated-customer-charge.md) 收：售价卡已声明金额取整。另外，SELL 读口对「未声明」那一格已不再报错落 publish_failed，改为在请求册上记`待判断`（ADR-0198 决定二）；下文「现象」与「证据」里两个读口报 `ErrAmountPrecisionUndeclared` 都是立票时的状态：BUY 读口随 [08](./08-buy-evaluation-recorded-consumer-never-forms-supplier-expected-cost.md) 也改为交回结果格、入账（ADR-0200）。余下 BUY 成本卡，上游先挡在 [07](./07-transport-charge-occurrences-have-no-production-path.md)、[08](./08-buy-evaluation-recorded-consumer-never-forms-supplier-expected-cost.md)。BUY 那一半由 [30](./30-buy-cost-card-declares-amount-rounding.md) 收（2026-10-19，代决见 [tf-chain-plan](../tf-chain-plan.md)）。此前 needs-triage——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自同日对演示动线余下各段的只读勘察（隔离子代理，证据 `S`）；下列现象与证据取自勘察结果，本票立时没有逐条复核，接票时先核。
Blocked by: 无
出处：[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 2026-10-18 那次重走。

## 现象

两张演示价卡都没声明金额取整策略，SA 的 BUY / SELL 读口拒 ErrAmountPrecisionUndeclared，消费落 dispatch.publish_failed

## 证据（读码）

seedgen 产出的 price-card-cn-sg-cost.json 与 price-card-cn-sg.json 的 plan 都没有 amountRounding（pricingPlanSnapshot 的 json 键）。internal/parcelpricing/domain/evaluation.go 未声明时追加 AMOUNT_PRECISION_UNDECLARED 问题项；internal/settlementaccounting/adapters/parcelpricing/buy_evaluation.go 的 LoadBuyEvaluation 与 sell_evaluation.go 的 LoadSellEvaluation 遇此即返回 ErrAmountPrecisionUndeclared。cmd/parcel-dispatch/assemble.go 的 buyEvaluationRecordedUndecidedSentinels 有意不收它。受理前试算 trialOf 走 Currency.MinorUnitScale，不受影响，这就是冻结 9600 今天能成的原因。

## 三类

租户取值。规模 S。

## 最小的诚实修法

按 ADR-0107 决定一、二，演示租户在两张卡的 seedgen（buyCardSnapshot、sellCardSnapshot）里声明 amountRounding，取 SYN 值：合计点、CNY 0.01、HALF_UP。重算 contentDigest，在一次性新库里重灌（不要另登 v2：PriceCards.ResolveInForce 遇两版同时在用答 ApplicabilityConflict）。重走时核受理前冻结仍是 9600。

## 要裁的

none（ADR-0107 已裁为卡上实例值，演示只记 S）

## 既有票

none
