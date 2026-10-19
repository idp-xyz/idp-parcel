# ADR-0198：包裹计费时点那一路，SELL 请求怎样下结论，矛盾怎样上抛

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 sell-customer-charge/04 落地与评审时遇到的、[ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 没裁的几格。部分停用 [ADR-0169](./0169-sell-evaluation-forms-a-customer-charge.md) 决定四「信封上还没有费用项目时，消费门指名缺这一引用」，**适用场景**仅限包裹计费时点那一路，即回指一份 SELL 请求的 SELL·客户费用评价。越权风险点见文末，待 SA owner 复核。）
Date: 2026-10-19

## Context

ADR-0190 决定五裁了几格：映射齐就形成；费用行映不全或映到多个项目记待判断；评价待判断、不可计价、冲突记在请求册上；计价侧没形成评价时，请求停在「已提交」。落地时还有四格没裁：

- **评价失败。** 计价对失败的评价照样入册，也照样发评价已记录信封，所以结算会收到它。
- **卡没声明金额取整。** 评价已完成，但金额换写不成最小币单位（[ADR-0107](./0107-evaluation-amount-rounding-is-declared-by-the-price-card-like-weight-rounding.md) 决定五）。
- **评价的结算币种与请求快照上的那一格不同。** ADR-0190 决定四要求计价取请求上快照的结算币种。
- **评价回指了一份请求，SELL 请求册上却没有它。**

另有一处与已接受记录不符。ADR-0169 决定四写「消费门指名缺这一引用」，ADR-0190 的 Links 说这句「照旧」。可请求册只有状态与费用标识两列（结算迁移 0037），映射不全时记下的只是`待判断`，缺的是哪个代码不落在任何地方。

## Decision

**一、评价失败，请求停在`已提交`，入账不重投。** 评价不可变；计价按回指幂等，不会再形成第二份；ADR-0190 不设「未形成」那一格。所以重投无益，也没有别的格可记。

**二、卡没声明金额取整，请求记`待判断`，入账不重投。**

- 取整声明与映射行同属租户取值。缺映射记`待判断`，这一格同样记`待判断`。
- BUY 那一路也把金额精度未声明留在未决名单之外，理由相同：评价不可变，要改的是卡。
- 若报错落发布失败，请求会停在`已提交`。可结算其实已经收到了评价，「已提交」就成了假话。

**三、费用行映不全或映到多个项目，请求记`待判断`，缺口不落库。** 缺的是哪个代码，可以拿评价的费用代码对一遍映射册重算出来，请求册不为此加列。本路由此停用 ADR-0169 决定四「消费门指名缺这一引用」；「不发明费用项目」照旧。

**四、两侧说法矛盾的格上抛错误，不入账。** 下列各格整笔回滚，落发布失败，直到失败预算耗尽：

- 回指非空，SELL 请求册上却查不到。SELL·客户费用评价只按本上下文的请求形成，请求又先于请求信封在同一库提交，所以这不是可见性滞后。
- 评价的结算币种与请求快照不同。这说明计价没守住 ADR-0190 决定四。
- 请求册上已有另一份结论。
- 评价刚读到`已完成`，形成时却未受理，例如合计为零。

这些格重投都不自愈，入账却会把矛盾静默吞掉，所以宁可响亮失败。

**五、没有回指的 SELL·客户费用评价不归本编排，零写入，入账。** 它不是为包裹计费时点形成的。

三类：以上各格都是**机制**（结论怎样落、错误怎样分级），不含租户取值。

## 候选与反方

- **失败评价记一格「未形成」。** ADR-0190 已经不设这一格。否决。
- **金额未声明按未决重投。** 评价不可变，重投只会耗尽失败预算。否决。
- **请求册加原因列，记下缺口。** 可行，但要迁移和读口，而缺口今天能重算出来。留给 owner，见风险点 2。
- **币种不符记`待判断`。** 计价侧的缺陷会被永久藏在请求册里，补登任何租户取值都修不好它。否决。
- **回指查不到按「不归本编排」入账。** 两侧数据矛盾会零留痕，事后也无从重投。否决。

## Consequences

- 代码：`application.FormEstimatedCustomerChargeHandler`（新哨兵 `ErrSellEvaluationRequestMissing`、`ErrSellEvaluationCurrencyMismatch`；未决结果带依赖原样答的错误）、`sapricing.FormOnSellEvaluationRecordedAdapter`，以及派发上 `sellEvaluationRecordedUndecidedSentinels` 的头注。
- ADR-0169 的 Status 行与 Links 各加一条前向指针；索引里那一行写明被停条款与适用场景。
- UC-SA-002 补 `AT-SA-179` 至 `AT-SA-181`。settlement-accounting `CONTEXT` 补 SELL 请求在本路的结论格。

## 越权风险点

1. **失败评价无痕**（SA owner）。请求停在`已提交`，在请求册上与「计价根本没形成评价」分不出来。UC-SA-002 生命周期表里「待判断/冲突/未受理/未形成」那一行要求带失败阶段和续办入口，本路是例外。要分得出，请求册得加格或加原因列。
2. **缺口不落库**（SA owner）。同一行要求待判断带缺口，本路只记状态，缺口靠重算。
3. **不可计价不存评价引用**（SA owner）。`AT-SA-175` 要求「保存排除依据与评价引用」，本路请求册只记`不可计价`。评价可以经计价侧按回指读到，排除依据在评价上。
4. **合计为零的已完成评价**（SA owner）。形成门拒零额，本路按矛盾上抛，请求停在`已提交`，信封落发布失败直到耗尽。UC-SA-002 的「负向结束」在请求册上没有对应的格。
5. **回指查不到就上抛**（SA、PP owner）。本条假定 SELL·客户费用评价的回指只会指向本上下文的 SELL 请求。计价日后若为别的来源形成带回指的 SELL 评价，这里会响亮失败，届时另裁。

## Links

- [ADR-0190：包裹计费时点发起 SELL 评价，形成预估客户费用](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md)：决定五没裁的几格在本记录作答
- [ADR-0169：SELL 评价形成客户费用，何时发起另册登记](./0169-sell-evaluation-forms-a-customer-charge.md)：部分停用决定四一句，限本路
- [ADR-0107](./0107-evaluation-amount-rounding-is-declared-by-the-price-card-like-weight-rounding.md)：决定五
- [UC-SA-002](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md)：`AT-SA-179` 至 `AT-SA-181`
- 票 `.scratch/sell-customer-charge/issues/04-sell-evaluation-recorded-forms-estimated-customer-charge.md`
