# ADR-0280：一个结算账户的 SELL 客户费用只按一份计价范围选卡——受理前估价采用与 SELL 计费时点采用两本册上同一户的计价范围必须相同，后登的一本不同即冲突

Status: Accepted（2026-10-21，Claude Code 会话代 SA owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/11，出处审计 [audit/cross.md](../../.scratch/docs-consistency/audit/cross.md) B2·B2-10。部分停用 [ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 越权风险点 9 一句并回答这一点，被停原句与适用场景见「对已接受记录的作用」；ADR-0190 正文不改，只在其 Status 行与 Links 加前向指针。[ADR-0179](./0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 无条款停用。代码随票 [docs-consistency/code/62](../../.scratch/docs-consistency/code/62-the-estimate-and-billing-moment-adoptions-may-register-different-pricing-scopes.md)。越权风险点见文末，待 SA owner 复核，PP owner 知会。）<!-- 索引注记 -->｜**一份范围**：同一（租户、结算账户）在受理前估价采用册与 SELL 计费时点采用册上登的计价范围必须相同；受理前估价与随后形成的预估客户费用因此不只按同一条规则、也按同一个范围选卡。**登记时核**：登任一本时，另一本已有这一户且计价范围不同，答冲突、不落行、不交那一本的登记信封；另一本没有这一户，照常登记；同一户在两本上的登记串行，并发的两笔不能各自通过。**不变**：两本册各自的键、值形与不设修订照旧，只登一本的户不受影响；选哪个范围仍是租户取值

Date: 2026-10-21

## Context

- [ADR-0179](./0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 决定三「**三、选卡与正式 SELL·客户费用评价同口径。**」，并写「估价与随后形成的客户费用因此按同一条规则选卡。」决定五：「结算新开一册：键是（租户、结算账户），值是（形态、计价范围），不设修订」。
- [ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定三另开一册：「结算新开一册「SELL 计费时点采用」：键是（租户、结算账户），值是（计费时点、计价范围），不设修订」。它的越权风险点 9 自认：「同一个结算账户在 ADR-0179 的估价采用册与本册各登一份计价范围，两册允许不同；不同时，受理前估价与随后的预估客户费用会用不同的卡，与 ADR-0179 决定三「同一条规则选卡」的意图有张力。」Consequences 的验收却以两册同值为前提：「验收：同一件包裹的预估客户费用与受理前冻结同额。」
- 选卡在 [ADR-0260](./0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md) 之后只按（租户、计价范围、方向、目的、时点）从 `parcel-pricing` 自己的价卡登记册选，计价范围就是选哪一张卡的那一维。两本册的范围不同，同一件包裹的冻结金额与预估客户费用就出自两张卡。
- 代码（取证于 main `b18f662bc`）：`internal/settlementaccounting/domain` 的 `PreAcceptanceEstimateAdoption` 与 `SellBillingMomentAdoption` 各带一个 `EstimatePricingScope`；两只登记编排 `RegisterPreAcceptanceEstimateAdoptionHandler`、`RegisterSellBillingMomentAdoptionHandler` 各写各的册（迁移 settlement_accounting `0034`、`0038`），互不读。演示租户两本册登的是同一个范围（`scripts/demo-seeds/steps/500-settlement.sh`）。

## Decision

**一、一个结算账户的 SELL 客户费用只按一份计价范围选卡。**（产品策略）

同一（租户、结算账户）在受理前估价采用册与 SELL 计费时点采用册上登的计价范围必须相同。受理前估价是为随后形成的客户费用冻结资金（ADR-0179 决定一），两者若按两个范围选卡，冻结与费用就出自两张价目，「冻结不足」或「冻结过多」不再是计量差，而是一次登记分歧。

**二、登记时核，后登的一本不同即冲突。**（机制）

- 登任一本时，读另一本上同一户的行：没有，照常登记；有且计价范围相同，照常登记；有而计价范围不同，答冲突（两本各自已有的冲突格），不落行，不交那一本的登记信封——估价采用那一本交给小包托运续办的那一封（ADR-0179 决定五），计费时点采用那一本的「评价参数已登记」（ADR-0229）；两本既有的冲突不交信封照旧。冲突答复指名另一本上的那一行与它的范围。
- 同一户在两本上的登记串行：读另一本与写本册在同一笔事务里、取同一把按（租户、结算账户）的锁，并发的两笔不能各自读到「另一本没有」而各自通过。
- 两本册都不设修订（ADR-0158 的不可变元组先例），所以这一核一旦两本都有行就永远成立；要换范围，与今天一样没有改口的路，不在本记录。

**三、不变。**

两本册各自的键、值形与不设修订照旧；只登一本的户（不要受理前估价，或不按包裹计费时点发起）不受这一核影响；选哪个计价范围是租户取值，产品不预选。

三类：一是**产品策略**（同户一份范围），二是**机制**（登记时的核与串行），某个户登哪个范围是**租户取值**。演示租户两册已登同一个 `SYN-` 范围，演示动线不变，证据只记 `S`。

## 候选与反方

- **明认两册可以不同：收窄 ADR-0179 决定三「因此」一句的适用场景，改 ADR-0190 那句验收。** 否决：两个范围分属两张价卡时，冻结额与费用额的差不来自计量，来自登记；没有一种业务上说得通的理由要用一张卡冻结、另一张卡收费——要多冻一点，是冻结政策的事，不是换卡的事。
- **另立一本「SELL 计价范围」册，两本采用册都不再带范围、改读它。** 未采纳：单一权威更干净，但要改两本册的值形、两条迁移与两只读路，登记面也要多一本；今天两本册都不设修订，登记时一核就能守住同一件事。日后两本册要加修订或再多一本读范围的册时，再改成单册。
- **一本读另一本（计费时点采用不带范围、读估价采用的）。** 否决：不要受理前估价的户就没有范围可读，按包裹计费时点发起的 SELL 评价跟着停住。
- **不在登记时核，只在形成预估客户费用时比对两本的范围。** 否决：到那一步冻结早已按另一张卡形成，比对只能报错，挡不住；登记时挡下，运营在委托进来之前就能更正。

## 对已接受记录的作用

- **部分停用** ADR-0190 越权风险点 9「同一个结算账户在 ADR-0179 的估价采用册与本册各登一份计价范围，两册允许不同」一句，并回答这一点。**适用场景**：两本册都有同一（租户、结算账户）的行时——两册的计价范围必须相同，后登的一本不同即冲突；ADR-0190 决定三的键、值形与不设修订不变，Consequences「验收：同一件包裹的预估客户费用与受理前冻结同额。」自此有了前提；其余各条不变。
- ADR-0179：无条款停用。决定三「估价与随后形成的客户费用因此按同一条规则选卡。」由本记录在范围那一维兑现；决定五的键、值形不变。

## Consequences

- 代码随票 [code/62](../../.scratch/docs-consistency/code/62-the-estimate-and-billing-moment-adoptions-may-register-different-pricing-scopes.md)：两只登记编排在同一笔事务里读另一本、按（租户、结算账户）取锁，范围不同答冲突；受控 CLI 与 HTTP 登记面照编排的答复透出；补编排与真库用例。
- settlement-accounting CONTEXT 受理前冻结那一条补一句；UC-SA-002「接受前财务控制」与「给开发的交接」各补一行。
- ADR-0190 的 Status 行与 Links 补指针。

## 越权风险点

1. **代 SA owner 定了同户一份范围**（SA owner）。若 owner 认为某类户要按一张卡冻结、另一张卡收费（例如冻结按公开价、收费按合同价），推翻的是决定一；那时 ADR-0190 Consequences 的验收句要改，冻结不足的差额也要另有说法。
2. **先登的一本赢**（SA owner）。两本都不设修订，先登的那一本定下范围，后登的只能照它；登错了的户，今天没有改口的路，与两本册各自登错时一样。
3. **两本册之间多了一道依赖**（SA owner）。登记编排要读另一本，另一本读不动时这一笔答不出；它照该册登记编排既有的读不动处置办，不默认放行。

## Links

- [ADR-0179](./0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md)：决定三「同一条规则选卡」由本记录在范围那一维兑现；决定五估价采用册
- [ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md)：部分停用越权风险点 9 一句并回答；决定三 SELL 计费时点采用册
- [ADR-0158](./0158-settlement-account-register-is-an-immutable-tuple.md)：不可变元组先例
- [ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md)：「评价参数已登记」信封
- [ADR-0260](./0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md)：按范围选卡
- [settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)、[UC-SA-002](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md)
- 票 `.scratch/docs-consistency/issues/11-unclaimed-cross-adr-contradictions.md`；代码票 `.scratch/docs-consistency/code/62-the-estimate-and-billing-moment-adoptions-may-register-different-pricing-scopes.md`
