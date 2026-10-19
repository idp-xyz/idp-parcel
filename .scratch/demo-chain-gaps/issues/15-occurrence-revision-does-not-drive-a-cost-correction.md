# 15 发生项更正不驱动供应商预期成本的计价纠错

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [08](./08-buy-evaluation-recorded-consumer-never-forms-supplier-expected-cost.md) 的隔离评审。
Blocked by: [07](./07-transport-charge-occurrences-have-no-production-path.md)（发生项先要有生产路径）
出处：[ADR-0200](../../../docs/adr/0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md) 越权风险点 4。

## 现象

另一份请求点名同一发生项的新版本、同一费用项目，评价又命中同一规则版本时，BUY 那一路只答「已有首版」并入账，不追加计价纠错，信封入账后不留痕。settlement-accounting `CONTEXT`「发生项更正 → 追加纠错版本」那条不变式在这一路被跳过。

## 证据（读码）

`internal/settlementaccounting/application/form_requested_supplier_expected_cost.go` 的同三维首版那一格；`domain` 上 `AppendCorrection` 今天没有应用层调用方。

## 三类

机制。规模 M。

## 最小的诚实修法

同三维已有首版、而新评价来自发生项的新版本时，按 ADR-0067「纠错重述整组评价结果」追加一个纠错版本（版本标识由纠错那条路给，不用 `BUYSC-` 派生），前后版本可审计。

## 要裁的

纠错由「发生项新版本」触发，还是由「评价结果不同」触发；同一规则版本、同一金额时是否仍追加（无变化纠错）。归 SA owner。

## 既有票

none
