# ADR-0340：供应商账单接收与审核应付是两种运营决定，各自记下点它的那个人

Status: Accepted（2026-10-28，Claude Code 会话代 SA owner 决（代决）：用户 2026-10-27 对会话提出的拆笔答「按你的建议开始吧」、2026-10-28 答「继续」；票 product-strategy-boundary/22 第 3 笔。越权风险点待 SA owner 复核。）<!-- 索引注记 -->｜**入口**：供应商账单的接收编排与审核（它的 `Audit`）已装在 parcel-api，没有任何口能调到；UC-SA-004 要求接收与审核分别授权、审计记下审核人与决定人，审核应付上的 `auditor_ref` 却是审核授权册登的那一方。**取法**：开两个运营决定口，各一种授予；两份命令带操作证据，落在接收行与应付行上，只作审计。｜**部分停用**（2026-11-02）：其决定一「载荷包括：账单身份与版本、供应商、法人、账期、币种、逐行（行、费用项目、主张金额）、到达时刻、可缺的被追加账单，以及逐行匹配裁决（行、分类、点名的预期成本版本、匹配依据）。」与「重放、版本冲突、未受理、点名的预期成本不是当前版，都是答案（`200`）。」两句，决定二「载荷点名账单、版本、行与要形成的应付标识；金额、越权升级上限、应付归集账户都读册。」与「已存在、冲突、这一行不可审核、超上限要升级，都是答案。」两句，已由 [ADR-0357](./0357-a-supplier-bill-number-is-an-external-identifier-of-the-claim-and-an-audit-passes-disputes-or-rejects-amount-ranges-of-a-line.md) 停用——接收载荷扩成加可缺的供应商账单号（外部标识，另记一册，同一供应商同号只标识一份主张，答复加账单号冲突）；审核载荷改为点名决定标识、结果（通过、争议、拒绝）与这一行上的金额范围，应付金额取范围长度、通过之和不超已匹配金额，答复加争议已记、拒绝已记、范围已裁决；只点名应付标识的旧载荷照收，读作通过整行。其余各条不变｜**由 [ADR-0357](./0357-a-supplier-bill-number-is-an-external-identifier-of-the-claim-and-an-audit-passes-disputes-or-rejects-amount-ranges-of-a-line.md) 回答**：越权风险点一「贷项与争议决定没开口。」的争议决定那一半——争议与拒绝经审核口按范围给出、争议以点名它的再审解除；贷项那一半仍没有口，无条款停用｜**部分停用**（2026-11-02，评审修订补挂）：其决定二「`POST /settlement-supplier-bill-audits`，同一能力面，决定种类 `SUPPLIER_BILL_AUDIT`，与接收分开授予。」一句已由 [ADR-0357](./0357-a-supplier-bill-number-is-an-external-identifier-of-the-claim-and-an-audit-passes-disputes-or-rejects-amount-ranges-of-a-line.md) 停用——审核口按这一次给的结果认授予：通过与拒绝（含再审给出的）认 `SUPPLIER_BILL_AUDIT`，把一段挂成争议与对争议再审认 ADR-0357 新立的 `SUPPLIER_BILL_DISPUTE_DECISION`，再审两种都要；口先认持其中一种，再判载荷形状，再按结果核所需授予。**适用场景**：只限供应商账单审核口；仍是运营决定能力面、仍与接收分开授予，只点名应付标识的旧载荷（通过整行）照旧只认 `SUPPLIER_BILL_AUDIT`。其余各条不变｜**部分停用**（2026-11-02，再扩）：其决定一「重放、版本冲突、未受理、点名的预期成本不是当前版，都是答案（`200`）。」一句（已由 ADR-0357 扩过一次，见上）已由 [ADR-0358](./0358-a-supplier-expected-cost-records-the-supplier-of-its-agreement-and-matching-checks-the-bill-supplier-against-it.md) 再扩两格——账单行点名的预期成本记下的供应商不是这张账单的供应商答 `EXPECTED_COST_SUPPLIER_MISMATCH`，那一版没记供应商答 `EXPECTED_COST_SUPPLIER_UNRECORDED`，都是答案（`200`），不落账单。**适用场景**：点名了预期成本的四种分类（已匹配、量差、价差、重复计费）；其余各条不变｜**由 [ADR-0358](./0358-a-supplier-expected-cost-records-the-supplier-of-its-agreement-and-matching-checks-the-bill-supplier-against-it.md) 回答**：越权风险点四「逐行裁决由录入的人给。」里点名别家预期成本那一种误选——预期成本形成时记下所依协议的供应商，匹配门核账单的供应商与之相同；分类与点名哪一份仍由人给，无条款停用
Date: 2026-10-28

## Context

取证于 main `463d3af1d`，只记 `S`。

- ADR-0170 决定一把 `NewReceiveSupplierBillHandler` 装在 parcel-api，审核是它的 `Audit`。两只都没有入口。
- UC-SA-004「权限、隔离与审计」：「账单接收、匹配规则采用、争议决定、审核应付、贷项和发布分别授权」；「审计至少记录……审核人、决定人」。
- `audited_payable.auditor_ref` 取自审核授权册（按供应商与法人登记的那一方），说的是谁有权审核，不是这一次是谁点的。
- 首发每个日常主合作伙伴只启用一种真实账单接入方式（PILOT-SCOPE）。人照来件录入就是一种，不必先接外部系统。

## Decision

**一、账单接收口。** `POST /settlement-supplier-bill-receptions`，挂操作者渠道「运营决定」能力面，决定种类 `SUPPLIER_BILL_RECEPTION`。载荷包括：账单身份与版本、供应商、法人、账期、币种、逐行（行、费用项目、主张金额）、到达时刻、可缺的被追加账单，以及逐行匹配裁决（行、分类、点名的预期成本版本、匹配依据）。金额按最小币单位的整数串写；完全匹配不带依据。租户取认证结果。接收与匹配在一笔事务里做完，进同一只接收编排。答复：
- 接收了答 `201 BILL_RECEIVED`；审核授权没登时，接收照样成立，答复多一格 `auditAuthority: UNCONFIGURED`。
- 重放、版本冲突、未受理、点名的预期成本不是当前版，都是答案（`200`）。
- 跨币种没有换算依据是租户取值缺席（`200` 带原因）。
- 册读不动、发布信封没交出去，是没形成答案（`500`）。

**二、审核口。** `POST /settlement-supplier-bill-audits`，同一能力面，决定种类 `SUPPLIER_BILL_AUDIT`，与接收分开授予。载荷点名账单、版本、行与要形成的应付标识；金额、越权升级上限、应付归集账户都读册。答复：
- 形成审核应付答 `201 PAYABLE_FORMED`，带金额与币种。
- 已存在、冲突、这一行不可审核、超上限要升级，都是答案。
- 审核授权、应付归集账户、越权升级上限三册没登，是租户取值缺席（`200` 带原因）。
- 其余未决与信封没交出去，是没形成答案。

**三、操作证据只作审计。** 两份命令各带一格操作证据，运营决定口写成 `OPERATOR/<发行方>#<sub>`，落在接收行与应付行新加的 `operation_evidence` 列上（迁移 `settlement_accounting/0050`）。它不进内容摘要，不进重放比对，也不进任何门：同一份由另一位操作者重放答已存在，留下的仍是首次那一位，判据同 ADR-0236 决定三那一列。受控登记口与存量行为空，读作「这一份不记提交方」。`auditor_ref` 照旧取审核授权册。

## 候选与反方

- **接收与审核共用一种授予。** UC-SA-004 明写分别授权。否决。
- **用操作者身份顶替 `auditor_ref`。** 那一格说的是谁有权审核，审核授权册与越权升级上限都按它判；换成点的人，「有权的人」就没了出处。否决，另加一列。
- **先接供应商的外部接口。** 首发只要一种接入方式，人工录入就够；外部接口是另一种连接器形态，等某个伙伴真要用时再做。否决。

## Consequences

- access-identity：决定种类 `SUPPLIER_BILL_RECEPTION`、`SUPPLIER_BILL_AUDIT`；迁移 `access_identity/0035`。
- SA：`adapters/http` 两口与 `OperatorDecisionIntake` 的两个 Intake；`registrationjson` 两份载荷；领域 `MatchClassificationFromName`；两份命令与两份记录的 `OperationEvidence`；迁移 `settlement_accounting/0050`。
- parcel-api：`endpoints_settlement_decisions.go` 多挂两口；`settlementOrchestrations` 多两个包事务的方法。
- 贷项（`Credit`）与争议决定不在本记录里，仍没有口。

## 越权风险点

1. **贷项与争议决定没开口。** UC-SA-004 的追加与贷项、争议决定，今天仍只有编排、没有入口。
2. **超上限要升级，却没有终审的口。** 审核口开了之后，超过越权升级上限的那一行会答 `AUDIT_MUST_ESCALATE`；升级之后由谁终审，ADR-0163 也没有定，今天这一行只能停着。
3. **操作证据只写不读。** 接收与应付的读口还没透出这一格，审计要直查库。
4. **逐行裁决由录入的人给。** 匹配分类与点名的预期成本由人判定后写进载荷，产品在入口核形状与逐行覆盖、在匹配门核金额与册上一致，不替人做匹配。

## Links

- [ADR-0170](./0170-settlement-orchestrations-enter-through-parcel-api.md)：决定一（审核是接收编排上的 `Audit`）
- [ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md)：决定三（操作证据只作审计）
- [ADR-0163](./0163-supplier-audit-escalation-compares-the-matched-amount-to-a-ceiling.md)、[ADR-0151](./0151-unassigned-command-faces-get-their-families.md)
- [UC-SA-004](../application/settlement-accounting/UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md)：`AT-SA-225`、`AT-SA-226`
- 票 [product-strategy-boundary/22](../../.scratch/product-strategy-boundary/issues/22-settlement-orchestrations-assembly-and-triggers.md)
- [ADR-0357](./0357-a-supplier-bill-number-is-an-external-identifier-of-the-claim-and-an-audit-passes-disputes-or-rejects-amount-ranges-of-a-line.md)：部分停用决定一两句（接收载荷加供应商账单号、答复加账单号冲突）与决定二三句（审核按范围给通过、争议或拒绝；审核口按结果认审核应付与争议决定两种授予）；回答越权风险点一的争议决定那一半
- [ADR-0358](./0358-a-supplier-expected-cost-records-the-supplier-of-its-agreement-and-matching-checks-the-bill-supplier-against-it.md)：部分停用决定一「重放、版本冲突、未受理、点名的预期成本不是当前版，都是答案（`200`）。」一句（再扩供应商不符、供应商未记两格）；回答越权风险点四「逐行裁决由录入的人给。」里点名别家预期成本那一种误选
