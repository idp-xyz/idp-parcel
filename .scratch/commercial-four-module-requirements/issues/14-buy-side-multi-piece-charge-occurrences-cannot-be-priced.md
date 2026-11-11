# 14 采购一侧一票多件的运输收费发生项算不出成本——发生项带多于一件载运对象时，计价输入答不可得

Category: enhancement
Status: draft
父票：[spec](../spec.md)
Blocked by: 票 [06](./06-shipment-level-weight-aggregation-lands-in-the-domain-and-the-import-template.md)。开工时随票立一篇 ADR：评价主体与输入形状要先定
地盘：`internal/parcelpricing/adapters/transportfulfillment`、`internal/parcelpricing/application`、`internal/settlementaccounting`（供应商预期成本的形成）、`internal/transportfulfillment`（发生项成员读口）

对应要求：Rating「分票计费」的采购一半。它是 2026-11-11 对照行业做法时多出来的一张，出处见[行业做法笔记](../industry-practice-notes.md)「多出来的三处」。

## 现象

取证钉 `48ca93e2d`（`origin/main`）。

- 采购评价的输入由 `internal/parcelpricing/adapters/transportfulfillment` 的输入解析器形成。它按运输收费发生项读成员载运对象：成员恰为一件才往下走，多于一件答输入不可得（`missingSingleMember`）。
- 输入不可得时计价不形成评价，结算在请求上记停因，不按未决重投。这一笔发生项因此没有供应商预期成本。
- 三大快递对多件货件都按票计费，计费重是逐件取实重与体积重的大者再相加（行业做法笔记 R8 至 R10）。租户转售这类渠道时，一张运单下的多件是一笔成本。
- 没有这笔成本，这票货的毛利也算不出：票 [05](./05-operating-results-are-derived-per-parcel-from-direct-charges-and-costs.md) 在供应商一侧缺成本时答判不出。

## 要做的

下面是建议形态，由随票的 ADR 定。

1. **多成员发生项形成聚合输入**：成员清单、逐件实重与尺寸、邮编路线。实重与尺寸取实测还是申报，照 [ADR-0171](../../../docs/adr/0171-pricing-input-resolver-uses-declaration-until-a-measurement-port-exists.md) 的取源，逐件分别取。
2. **按价卡声明的聚合方式算计费重**：合计后派生，或逐件派生后求和，用票 06 落下的那两种读法。
3. **一个发生项形成一份评价、一笔供应商预期成本**，主要范围是这个发生项。
4. **单件发生项照旧**：评价与内容摘要一字不变。

## 要裁的

- **评价主体是什么。** 计价 `CONTEXT.md` 写评价对象有且只有四种：已受理包裹、试算对象、委托、承运总单。一张承运商运单下的多件，在小包托运那边对应的是一笔覆盖多个包裹的面单交易，未必是整份委托，也不是承运总单。是借用其中一种，还是要立第五种，由 ADR 定，并回 `CONTEXT.md` 改那一句。
- **逐包裹的采购卡遇到多件发生项。** 是逐件各形成一份评价，还是照旧答输入不可得、要求租户登一张聚合的卡。
- **各件路线不同。** 同一发生项下的成员若寄往不同邮编，还算不算一票。
- **与收费发生条件里数量形的关系。** 供应商协议按原因声明按段、按件、按尝试或按订舱计费（[ADR-0209](../../../docs/adr/0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md)）。按件计的发生项与多成员是不是同一件事，取证时没有读发生项登记的代码，立 ADR 时先核。
- **摊到每件。** 一笔票级成本归到每件包裹要成本分摊，仍在冻结。在那之前，票 05 的按包裹毛利对这类票怎么答：判不出，还是另给一个按票的范围。

## 三类

主体、输入与成本的形态是机制。每张采购卡选哪种聚合方式是租户取值。

## 完成判据

- 演示租户登一张聚合的采购卡，一个带多件的发生项形成一份评价与一笔供应商预期成本；计费重与手算的逐件取大者之和一致。有真库用例。
- 单件发生项的评价结果与内容摘要一字不变。
- 评价主体的决定落在 ADR 与计价 `CONTEXT.md`；`UC-SA-002` 与 `UC-PP-001` 同步，AT 号取空号。

## 不在本票

- 主单级计费。它等运输履约的承运总单登记册。
- 成本分摊本身。
- 销售一侧的票级计费，见票 [07](./07-shipment-level-sell-charging-enters-production.md)。
