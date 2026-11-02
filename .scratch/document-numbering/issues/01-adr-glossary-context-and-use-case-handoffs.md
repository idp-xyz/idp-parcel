# 01 单据号的决定与领域文档：ADR-0355、GLOSSARY、六个上下文 CONTEXT、八份 UC 交接

Category: enhancement
Status: in-progress——2026-11-02 协调方完工，待非作者评审与进 main，分支 claude-docnumber
父票：[spec](../spec.md)
Blocked by: —
出处：用户 2026-11-02 裁定加一套单据编号规则（见 spec「出处」）

## 做什么

把 spec「定下的几条」落成现行规则：一份 ADR，GLOSSARY 两个词条，六个上下文各几句规则，八份 UC 记下与实现的差距。本票只动文档，不动代码。

## 完成判据

- [ADR-0355](../../../docs/adr/0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md) Accepted，覆盖十一类单据的开立时点、平台编号能力与本仓编排的分工、默认模板、规则册归属、时区、撞号、对账单发布幂等改键、手填编号的去处与呈现；整份取代 ADR-0143，部分停用 ADR-0339 决定二、决定三各一句，两边指针照 `docs/adr/README.md` 顶部写法。
- GLOSSARY 新立「单据号」「单据编号规则」。
- parcel-shipment、settlement-accounting、visibility-exception、customs-compliance、node-operations、transport-fulfillment 六个 `CONTEXT.md` 在各单据的生命周期处补规则句。
- UC-PS-001、UC-SA-003、UC-SA-004、UC-VE-004、UC-VE-007、UC-CC-001、UC-NO-006、UC-TF-004 的「给开发的交接」各记一行差距，指向 04–09。

## 完成记录（2026-11-02 协调方）

- 新 `docs/adr/0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md`。
- `docs/adr/0143-…`：`Status` 改为被 ADR-0355 整份取代（草案从未接受）。
- `docs/adr/0339-…`：`Status` 索引注记与 Links 各加前向指针，引被停两句原文，其余各条不变。
- `docs/domain/GLOSSARY.md`：「外部标识」之后新立两个词条。
- 六个 `CONTEXT.md` 共九句规则（委托、对账单、费用争议、审核应付、异常案件、客户索赔项、关务案件、送站预报、送站交接单、运输委托与订舱）。
- 八份 UC 交接：有表格的加行，只有段落的加段，UC-SA-004、UC-CC-001 原无此节，新开。
- 核对：ADR-0355 链到的各文件都在；引 ADR-0339 的两句逐字命中原文；`go run ./tools/adr-index -check` 只报生成物过期（进 main 时重生成）。
- 取舍：客户索赔项取「客户提交被记录」而不取「受理」，客服在资格审核之前就要拿号沟通；订舱与运输委托、送站预报与交接单的交接分别记在 UC-TF-004、UC-NO-006（规则所在的用例），spec 初稿写的 UC-TF-003、UC-NO-002 已改。
