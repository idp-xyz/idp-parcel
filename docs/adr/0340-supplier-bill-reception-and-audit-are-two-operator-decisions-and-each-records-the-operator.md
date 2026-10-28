# ADR-0340：供应商账单接收与审核应付是两种运营决定，各自记下点它的那个人

Status: Accepted（2026-10-28，Claude Code 会话代 SA owner 决（代决）：用户 2026-10-27 对会话提出的拆笔答「按你的建议开始吧」、2026-10-28 答「继续」；票 product-strategy-boundary/22 第 3 笔。越权风险点待 SA owner 复核。）<!-- 索引注记 -->｜**入口**：供应商账单的接收编排与审核（它的 `Audit`）已装在 parcel-api，没有任何口能调到；UC-SA-004 要求接收与审核分别授权、审计记下审核人与决定人，审核应付上的 `auditor_ref` 却是审核授权册登的那一方。**取法**：开两个运营决定口，各一种授予；两份命令带操作证据，落在接收行与应付行上，只作审计。
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
2. **操作证据只写不读。** 接收与应付的读口还没透出这一格，审计要直查库。
3. **逐行裁决由录入的人给。** 匹配分类与点名的预期成本由人判定后写进载荷，产品只核它们与册上一致，不替人做匹配。

## Links

- [ADR-0170](./0170-settlement-orchestrations-enter-through-parcel-api.md)：决定一（审核是接收编排上的 `Audit`）
- [ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md)：决定三（操作证据只作审计）
- [ADR-0163](./0163-supplier-audit-escalation-compares-the-matched-amount-to-a-ceiling.md)、[ADR-0151](./0151-unassigned-command-faces-get-their-families.md)
- [UC-SA-004](../application/settlement-accounting/UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md)：`AT-SA-225`、`AT-SA-226`
- 票 [product-strategy-boundary/22](../../.scratch/product-strategy-boundary/issues/22-settlement-orchestrations-assembly-and-triggers.md)
