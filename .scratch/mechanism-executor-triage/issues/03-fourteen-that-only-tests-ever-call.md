# 只有测试调得到的那十四条——「真烂会先烂在这两条上」的预言该复核了

Category: chore
Status: in-progress——MCP-3 取证中（2026-09-03，owner 授权自决）；处置规则已裁（见 [spec「处置裁决」](../spec.md) 第 1 条）：答得出调用方的按上下文立实现票接线，答不出的在基线理由行逐条写明在等什么
Blocked by: 无

## 十四条

`settlement-accounting`（4）、`customs-compliance`（4）、`visibility-exception`（6），
逐条核过（锚 `9d6063c`），**全部只出现在测试里，无一有生产调用点**：

| 上下文 | 条目 | 唯一出现处 |
|---|---|---|
| SA | `FormSupplierExpectedCost` | 适配器测试 + 应用测试 + 领域测试 |
| SA | `FormAuditedPayable` | `supplier_bill_test.go` |
| SA | `FormSupplierCreditNote` | `supplier_bill_test.go` |
| SA | `IncludeAdjustmentInSubsequentPeriod` | `customer_statement_test.go` |
| CC | `RegisterCredential` | `compliance_judgment_test.go` |
| CC | `VerifyDutyPayment` | `duty_release_test.go` |
| CC | `ReceiveReleaseOutcome` | `duty_release_test.go` |
| CC | `FormDutyCollaboration` | `duty_collaboration_test.go` |
| VE | `DecideDisclosure` | `notify_customer_test.go` |
| VE | `EstablishCase` | 应用层零引用（连测试都没有，只有 `NewCaseID`） |
| VE | `SubmitEvidence` | 应用层零引用 |
| VE | `PrepareDisclosure` | 应用层零引用 |
| VE | `ResolveByBusinessTime` | 领域测试 |
| VE | `RaiseConflictSignal` | 领域测试 |

## 三处定级与这十四条的张力

- **SA / PN-07**：记「达标—有裁定的显式留待」，留待项是**三口登记面留待实例证据**。这四条
  **不属于**那三口——它们缺的是调用方不是册子内容。
- **CC / PN-05**：记 **达标**，差量列「无」。而 CC 在名单上有四条。
- **VE / PN-06**：记「达标—有裁定的显式留待」，留待项是**真实渠道凭证**
  （`NotificationChannelGateway`）。这六条**没有一条**属于那一项。

三处的共同措辞是「N 个 UC 全部有对应机制触点」。**UC 有触点** 与 **每条规则有执行器** 不是
同一个断言，见[票 01](./01-skeleton-criterion-and-its-instruments-measure-different-things.md)。

## 那句预言，今天仍然成立

`production_wiring_baseline.txt` 对 `FormSupplierExpectedCost` 单独注过一句，说它与
`DecideDisclosure` 是全部条目里仅有的两个**已经走出 domain 层**的：

> 适配器与应用层都在、两边都有测试，唯独没有生产调用路径。真烂会先烂在这两条上，别的都还
> 没开始。

**复核结果：今天仍成立，且一字未变。** `FormSupplierExpectedCost` 出现在
`adapters/postgres/supplier_expected_cost_test.go` 与 `application/receive_supplier_bill_test.go`
——两侧都只有测试，正文都不用。`DecideDisclosure` 同形，只在 `application/notify_customer_test.go`。

这句预言值得单独指出来，因为**它写下时是一句预警，现在它是一句仍在生效的预警**，而中间隔着
一次「八切片全数达标」的宣布。两件事没有对上。

## VE 那两条是一条完整的支路

`ResolveByBusinessTime`（按业务时间裁决事实冲突，产出 `ConflictJudgment`）与
`RaiseConflictSignal`（由该裁决立信号）是**同一条路径的两端**。

而编排 `application/raise_signal.go` **存在**，走的是另一条路：`OpenEpisode` + `ConcludeTriage`。
它从不经过冲突这条支路。

所以 VE 的「冲突裁决 → 信号」这条链今天没有执行器——`docs/domain/visibility-exception/CONTEXT.md`
明写「冲突裁决按业务时间」是本上下文拥有的能力之一，开发主线 PN-06 行也把它列为领域面收口
的正面证据。**领域面收口是真的，能被触发不是。**

## 本票要做的

对这十四条各答一问，逐条不按组：**若它今天就该有调用方，那个调用方应该在哪个用例里？**

答得出来的归「支路未接」并指名用例；答不出来的说明它在等什么（等哪个上游事实来源、哪个
`PAR-*` 参数、还是哪个尚未开工的切片）——**而后一种要重新对照「八切片全数达标」那句话**，
因为按那句话没有切片还没开工。

## 边界

本票**不接线、不改代码、不改基线名单、不改定级**。产出是十四行带举证的判断，交人裁。
