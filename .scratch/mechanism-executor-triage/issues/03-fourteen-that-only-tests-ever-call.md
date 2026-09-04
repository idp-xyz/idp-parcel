# 只有测试调得到的那十四条——「真烂会先烂在这两条上」的预言该复核了

Category: chore
Status: resolved——十四行取证已写完（2026-09-04，MCP-3，锚 `1af59c8`，见文末 `## Answer`）：**十四条全部答得出调用方**，无一条在等 `PAR-*` 或未开工切片；按 [spec「处置裁决」](../spec.md) 第 1 条立成三张实现票 [06](./06-sa-four-executors-behind-existing-uc-steps.md) / [07](./07-cc-four-executors-behind-existing-uc-steps.md) / [08](./08-ve-six-executors-behind-existing-uc-steps.md)，同日三张全部 resolved（SA/CC/VE 十四条在两份棘轮基线上清空），本票的产出至此被消费完，2026-09-04 由 MCP-3 转 resolved（MCP-1 14:3x 频道提议）
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

## Answer

取证于 `1af59c8`（2026-09-04，MCP-3）。先复核一遍前提：`git grep -w <符号> -- 'internal/*.go' 'cmd/*.go' ':(exclude)*_test.go'` 十四条**仍全部只命中各自 domain 包内的定义处**，无一有 application / adapters / cmd 调用点（唯一例外是 `supplier_cost_rehydration.go` 的一句注释提到 `FormSupplierExpectedCost`，说的正是「它不走这条」）。前提成立，下面逐条。

方法：读定义处的 Go 注释取领域语义 → 在该上下文的 `UC-*` 里找描述这一步的原文 → 看 `internal/<ctx>/application/` 里对应用例文件今天调的是哪些领域函数、为什么没调它。每条末尾一格是判定。**十四条都归「支路未接」**，没有一条是「答不出」。

### settlement-accounting（4）

| 条目 | 该有的调用方 | 应用层今天的样子 | 判定 |
|---|---|---|---|
| `FormSupplierExpectedCost` | `UC-SA-002` 步 5「保存评价引用、结算采用快照、责任/范围解释和金额展开；原币与合同结算币不同时，直接采用评价内的原币金额、汇率序列版本和换算步骤作为换算依据」——**BUY 方向那一半**。`UC-SA-004` 步 3、`UC-SA-006` 步 6 都只是读它 | `application/` 里 `confirm_charge.go`、`record_charge_adjustment.go` 只有 SELL 侧（`NewSellEvaluationReference`）；`ports/` 与 `application/` **零处**提到 BUY。`parcel-pricing` 有 BUY 价格方向（`domain/value_objects.go`、`price_card_catalog.go`），但 BUY 评价到达 SA 的入向缝不存在 | 支路未接；**且先缺一条入向缝**（BUY `PricingEvaluation` → SA），那是机制半边不是实例半边。与 `label-channel/13`（BUY 评价 → 成本分）相邻但不是同一条 |
| `FormAuditedPayable` | `UC-SA-004` 步 5「授权审核责任方：对无争议范围形成审核应付；争议范围形成待审/争议/拒绝」 | `receive_supplier_bill.go` 只调 `ReceiveSupplierBillClaim` + `MatchBillLine`（步 1–4），到匹配就停。函数注释「到达不是应付，匹配也不是应付，审核通过才是」正是步 5 这道门 | 支路未接（`UC-SA-004` 步 5） |
| `FormSupplierCreditNote` | `UC-SA-004` 步 6「接收供应商更正、贷项、追加主张和迟到主张」；步 7 发布「供应商费用贷项引用」 | 同上文件，无步 6 | 支路未接（`UC-SA-004` 步 6） |
| `IncludeAdjustmentInSubsequentPeriod` | `UC-SA-003` 步 7「接收发布后的新确认费用**或既有调整**……关联原账单并归入后续周期」 | `cut_off_publish_statement.go` 调了 `IncludeLateChargeInSubsequentPeriod`——步 7 的**费用半边**接了，**调整半边**没接。函数注释「调整必须挂在原账单内的费用上；纳入原周期就是回填已发布快照，构造期拒绝」 | 支路未接（`UC-SA-003` 步 7 的调整半边，同一步只接了一半） |

### customs-compliance（4）

| 条目 | 该有的调用方 | 应用层今天的样子 | 判定 |
|---|---|---|---|
| `RegisterCredential` | 无编号 UC——它是**登记面**：`RegulatoryCredential` 是「监管凭证的不可变版本」，与 `register_ports_paths.go`、`register_case_requirement_rule.go`、`register_case_configuration.go` 同族。第一个读它的步骤是 `UC-CC-003` 步 7「核验监管凭证身份、适用性、有效期和截至当前的可用依据」，其后 `UC-CC-005` 步 7/9 占用、`UC-CC-006` 步 7 释放/核销 | CC 三个登记用例登口岸路径、案件要求规则、案件配置，**不登凭证**。`customs-slice-0` 交接明列「对监管凭证登记身份、范围、有效期、额度、占用单位……的权威结果依据」为必需 | 支路未接（登记面缺一口；PN-05 的登记面不止三口） |
| `VerifyDutyPayment` | `UC-CC-009` 步 7「按税费版本、外部引用、范围、付款人、金额、币种、业务时间和真实规则关联——分别形成**覆盖、差额和有效性**三个维度；无法权威关联时保持外部资金事实待关联」。`DutyPaymentVerification` 的三个字段 `coverage`/`delta`/`validity` 与这一步逐字对应 | `application/` 里**没有 `UC-CC-009` 步 2–7 的任何文件**；`verify_release_gate.go` 只做步 10（`VerifyReleaseGate` + `FoldGateConclusion`）。`ExternalFundsFactReference` 在 CC 非测试代码里只出现在 `duty_release.go` 自己——外部资金事实（`UC-SA-005` 那头有 `map_external_funds.go`）到 CC 的缝也不存在 | 支路未接（`UC-CC-009` 步 7），且先缺 SA→CC 的外部资金事实入向缝 |
| `ReceiveReleaseOutcome` | `UC-CC-006` 步 5「按来源权威语义拆分技术、监管接收、业务受理、过程决定、税费、**放行**和处置结果——各层各范围分别形成；缺失层不补造」 | `receive_external_result.go` 调 `InterpretExternalResult` + `CheckLayerConsistency`；`CustomsReleaseOutcome`/`ReleaseKind` 在 CC 非测试代码里**只在 `duty_release.go`**——放行层被解释了但没落成放行结果对象 | 支路未接（`UC-CC-006` 步 5 的放行层） |
| `FormDutyCollaboration` | `UC-CC-009` 步 4「按真实程序分别确定当前范围是否需要付款……形成付款协作事项、无需付款、付款不构成当前动作前置条件或未决」+ 步 5「确认责任交接目标、法定义务人、实际付款安排和核对范围——形成范围化协作入口；不创建支付交易」。函数注释的两格（核定税费 / 明确无需付款）与 `ErrCollaborationNotFundable`「编排据以保持未决」正是步 4 的四个结果 | 同 `VerifyDutyPayment`：`UC-CC-009` 步 2–7 无编排 | 支路未接（`UC-CC-009` 步 4–5） |

### visibility-exception（6）

| 条目 | 该有的调用方 | 应用层今天的样子 | 判定 |
|---|---|---|---|
| `DecideDisclosure` | `UC-VE-006`「本用例拥有异常披露决定和通知义务判断」；`UC-VE-001` 步 7「独立判断客户可见性、敏感范围、披露内容和授权——暂不披露、待授权或形成通知决定」 | `notify_customer.go` 的命令**把 `domain.DisclosureDecision` 当输入**（`command.Disclosure`），只调 `GenerateNotification`——用例从「决定已经有了」开始，而没有任何生产代码形成这个决定。输入侧的策略已在：`ports.DisclosurePolicyView.AssessDisclosure` 与披露策略登记（`PAR-VIS-09`） | 支路未接（`UC-VE-006` 前半：形成披露决定；`UC-VE-001` 步 7 同缺） |
| `EstablishCase` | `UC-VE-004` 结果「已建立案件」/ `AT-VE-062`「高可信高影响命中自动规则→建案并固定范围/责任/目标」；`UC-VE-001` 步 4–5「建立新案件」「固定案件影响范围、责任团队、响应目标和下一行动」 | `raise_signal.go` 调 `OpenEpisode` + `ConcludeTriage`：分诊结论形成了，**结论为「建案」时没有人建案**。票面原记「应用层零引用（连测试都没有）」复核仍成立 | 支路未接（`UC-VE-004` 分诊结论为建案那一支） |
| `SubmitEvidence` | `UC-VE-007` 触发「证据到达」→ `AT-VE-113`「证据材料收到但尚未核实→形成证据项，不形成事实或责任」。函数注释「评价起点是`已收到`，不由调用方指定」就是这条 AT | `handle_claim.go` 调 `ReceiveClaimItem`、`OpenRecoveryMatter`、`RecordRecoveryAction`、`NewSupplementRequirement`——索赔项与追偿有了，**证据项没有** | 支路未接（`UC-VE-007` 证据到达那一支） |
| `PrepareDisclosure` | `UC-VE-007` `AT-VE-132`「通知内容和证据已准备但尚未对外提交→保持准备完成」、`AT-VE-147`「同一证据被多个案件和追偿事项引用→受控复用」；证据项自带「访问范围」。函数注释「脱敏指纹不得与原件指纹相同（相同即原件外流）」 | 同上文件，无证据披露版本 | 支路未接（`UC-VE-007` 证据对外披露那一支；依赖 `SubmitEvidence` 先接） |
| `ResolveByBusinessTime` | `UC-VE-002` 结果「冲突待确认：有效事实无法按权威范围、时间和因果裁决；保留各事实和冲突关系」/ `AT-VE-043`。函数注释「CONTEXT 允许的裁决维度之一……其余维度各有自己的裁决函数」 | `derive_projection.go` 调 `DeriveTrackingProjection`、`ClassifyMilestone`、`LeaveUnclassified` 等；`ConflictJudgment`/`ExceptionSignal`/`ResolveBy*` 在 VE 的 application、ports、adapters 非测试代码里**零处** | 支路未接（`UC-VE-002` 冲突裁决） |
| `RaiseConflictSignal` | 同上一条的另一端；`UC-VE-004` 的输入端（信号）。函数注释引 CONTEXT「冲突仍无法裁决时……投影保持信息待确认并形成适用异常信号」 | `derive_projection_test.go` 里 `TestAForkedSupersessionKeepsBothSuccessorsInTheProjection` 的注释自己写着：「适用异常信号走冲突机制，不在本编排（**接 RaiseConflictSignal 是另一张票**）」——全仓 `.scratch/` 搜 `RaiseConflictSignal`，**那张票不存在**（只命中本目录与 `production-wiring-ratchet-gate/census-*`） | 支路未接（`UC-VE-002` → `UC-VE-004` 的冲突信号；代码里承诺过的票没立） |

### 对「八切片全数达标」那句

十四条没有一条能归到「等上游事实来源 / 等 `PAR-*` / 等未开工切片」——每一条的 UC 步骤都写在案，输入侧的依据（披露策略、协议、评价、外部结果）也都有各自的机制。两处例外是**缝**不是**等**：BUY 评价到 SA、外部资金事实到 CC，两条入向缝今天不存在，但它们本身就是要做的机制。所以基线那句预言（「真烂会先烂在 `FormSupplierExpectedCost` 与 `DecideDisclosure` 上」）今天不但成立，而且这两条各带一个更大的缺口：一个缺整条 BUY 入向缝，一个是用例把自己该形成的决定当成了输入。

「UC 有触点」与「每条规则有执行器」在这十四条上的差就是：SA 七个 UC 的步骤表里有这四步，CC 十二个里有这四步，VE 八个里有这六支——**步骤在，执行器不在**。

### 立票：按处置裁决第 1 条，三张（SA / CC / VE），票内按编排文件分组

拆成几张已由 [spec「处置裁决」](../spec.md) 第 1 条定死——**按上下文各一张，票内逐条列**，这里不另拆。票内分组按「同一个用例文件接完就绿」，每组一个应用层文件，可各自成一笔提交：

- SA-a `UC-SA-004` 步 5–6：`FormAuditedPayable` + `FormSupplierCreditNote`（同一文件 `receive_supplier_bill.go` 往后接两步）
- SA-b `UC-SA-003` 步 7 调整半边：`IncludeAdjustmentInSubsequentPeriod`（`cut_off_publish_statement.go` 补半步）
- SA-c BUY 入向缝 + `UC-SA-002` 步 5 BUY 侧：`FormSupplierExpectedCost`（先定缝的形状，与 `label-channel/13` 对一下边界）
- CC-a 凭证登记面一口：`RegisterCredential`（与三个 `register_*.go` 同形）
- CC-b `UC-CC-006` 步 5 放行层：`ReceiveReleaseOutcome`（`receive_external_result.go` 补一层）
- CC-c `UC-CC-009` 步 4–7：`FormDutyCollaboration` + `VerifyDutyPayment`（新用例文件；外部资金事实入向缝的形状先定）
- VE-a `UC-VE-006` 前半：`DecideDisclosure`（新用例文件；`notify_customer.go` 的输入改为引用它的产物）
- VE-b `UC-VE-004` 建案支：`EstablishCase`（`raise_signal.go` 分诊结论为建案时续办）
- VE-c `UC-VE-007` 证据两支：`SubmitEvidence` + `PrepareDisclosure`（`handle_claim.go` 或新文件）
- VE-d `UC-VE-002`→`004` 冲突支：`ResolveByBusinessTime` + `RaiseConflictSignal`（`derive_projection.go` 分叉时续办；把测试注释里那张「另一张票」真的立出来）

三票十组、十四条、零取值——全是机制半边。三张票分别为本目录 [06](./06-sa-four-executors-behind-existing-uc-steps.md)、[07](./07-cc-four-executors-behind-existing-uc-steps.md)、[08](./08-ve-six-executors-behind-existing-uc-steps.md)；先接哪张、哪组，归人排。

### 没能确认的

- `InterpretExternalResult` 内部是否已经把放行语义**解释**到某个非 `CustomsReleaseOutcome` 的形状里（若是，`ReceiveReleaseOutcome` 那条更接近 `parcel-pricing` 组那种「平行第二写法」而不是未接）。只核了类型引用，没读解释函数正文。
- `JudgeReady`（`UC-CC-003`）今天核凭证时读的是什么——若它根本不读凭证，`RegisterCredential` 的第一个消费者也还没写；只核了登记用例不登凭证。
