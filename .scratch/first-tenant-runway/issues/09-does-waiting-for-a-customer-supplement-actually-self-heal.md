# 「等待受控补充」真的会自愈吗——ADR-0086 那条前提没人核过

Category: bug
Status: in-progress——MCP-6（2026-09-04，隔离分支 `mcp6-ftr09`，基线 main `caca1c4a`）按 task-66cd286c 实施「裁决」节末段五条范围；此前已裁（2026-09-04，通道 6，owner 授权）：取第三种结论「自愈不成立」，选 A，落文 [ADR-0106](../../../docs/adr/0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md)；本票转为实施票，范围见「裁决」节末段，处置翻转与续办信封同笔落地否则不许落地
Blocked by: 无

来源：2026-09-02 MCP-1 裁 [ADR-0094](../../../docs/adr/0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 时撞见，当场因证据不足没有动它。锚 `ca7441f`。

## 要核的那一句

[ADR-0086](../../../docs/adr/0086-manual-review-wait-is-a-committed-pause-resumed-by-completion-envelope.md) 的 Context 把三个等待态分成两侧，`等待受控补充`被判在「回滚重投是对的」那一侧，理由原话是：

> **等待受控补充 / 等待内部续办**：重投重跑同一轮，等的依赖（客户新提交版本、抖动的内部依赖）会自己回来，回滚丢掉的中间态在下一轮重新推出。回滚重投是对的。

**「客户新提交版本会自己回来」这半句没人核过它指的是哪一封信封。** 客户补件产生的是**同一委托的新提交版本**（ADR-0045），而在途那一封信封携带的是**旧版本**。重投旧信封只会拿旧版本重跑同一条链，停在同一格。所以自愈成不成立，取决于新版本会不会另铸一封信封把链重新驱起来——那是另一个机制，不是「这一封自己好了」。

## 为什么现在核不了

ADR-0045 的 Consequences 明写：

> 应用编排（来源保全、**受控补充的授权、身份签发、重触发判断**）另成切片。

「重触发判断」被划出去了。它落没落地、落成什么形状，决定了上面那半句成不成立，而这一点不查源码答不出。本票不替它答。

## 核什么

三问，逐个要代码事实，不要推断：

1. **受控补充走完之后，有没有信封被铸出来并进 outbox？** 若有，事件类型是什么、谁消费、消费者转交的是不是同一个 `AdvanceAcceptanceChainHandler`（ADR-0086 Decision 二给复核完成开的形状）。
2. **旧版本那一封在新版本出现之后会怎样？** 它重跑时链是停在同一格、还是因版本换代落到别的原因上（`CommercialBasisSuperseded`、`StaleShipmentRequestRevision` 之类）。这决定了它是「白烧预算」还是「如实转格」。
3. **`waitingOn = CUSTOMER_SUPPLEMENT` 有没有落库过？** 未决整笔回滚意味着没有——若果真如此，「等客户补件的都有谁」这个队列在库里结构上恒为空，与 ADR-0086 用来给人工复核开例外的那条读面理由**逐字同构**。

第三问单独要紧：它不依赖第一问的答案。**即便自愈成立，读面失真也仍然成立**，而那正是 ADR-0086 自己列出的两条理由之一。

## 三种可能的结论，各自的处置

- **自愈成立且读面无碍** → ADR-0086 判得对，本票关掉，在 ADR-0094 的那一格上补一句取证锚。
- **自愈成立但读面恒空** → 不是重投问题，是投影问题；另立一票补 `waitingOn` 的落库路径，不动重投语义。
- **自愈不成立**（新版本不另铸信封，或旧信封只是白烧到 `ABANDONED`）→ 与票 [07](./07-undecided-that-never-self-heals-burns-the-retry-budget.md) 同构，`ResumeByCustomerSupplement` 应当并入 ADR-0094 Decision 三的提交侧。**那要 supersede ADR-0086 Context 的这一句，走新 ADR，不在实现票里定。**

## 不属本票

**`ResumePath` 第四格与消费门按恢复动作折**已由 ADR-0094 裁定并另有实现票，本票不重述也不阻塞它——0094 刻意把这一格维持原判，正是为了不在没有证据时动它。

## 参照

ADR-0086 Context 的三态分侧、[ADR-0045](../../../docs/adr/0045-new-submission-version-keeps-history-and-reestablishes-the-task.md) Consequences 里划出去的那条切片、ADR-0094 Decision 三第四小格与它写明的「不改它」的理由；`internal/parcelshipment/application/form_new_submission_version.go`、`internal/parcelshipment/application/judgment_continuation.go` 的 `resumePath()`、`internal/parcelshipment/adapters/postgres/source_data_handoff.go`。

## 裁决（2026-09-04，通道 6，task-f530ad56 裁决批口径：owner 授权自决，写明能力边界）

### 三问的答案（代码事实，锚 `main = 512b419`；问 1、3 由 report.md B 组先答、本轮复核仍成立，问 2 本轮补读）

1. **受控补充走完之后没有任何信封被铸出来。** `FormNewSubmissionVersionDeps{Sources, Requests, Identities, Clock}` 没有 outbox / handoff 端口，`form_new_submission_version.go` 全文不出现 `Envelope` / `Publish` / `HandOff` / `EventType`；且 `NewFormNewSubmissionVersionHandler` 在全仓非测试代码里只有声明本身，`cmd/` 零命中——**受控补充编排在生产上根本没有入口**。「客户新提交版本会自己回来」在 HEAD 上没有任何机制承载。
2. **旧版本那一封在新版本出现之后：** 重跑落到 `StaleShipmentRequestRevision`（`judgment_continuation.go`）或停在同一格，取决于换代检查在链上的位置；两种结果都不是续办——续办只能来自新版本自己的驱动，而那封驱动今天不存在（问 1）。运行时究竟落哪一格**本轮未量**（无 DSN、不跑进程），但两格的结论相同，不影响裁决。
3. **`waitingOn = CUSTOMER_SUPPLEMENT` 从未落库。** `undecidedDisposition` 对 `ResumeByCustomerSupplement` 交回 `ErrAcceptanceChainUndecided` 整笔回滚，等待态随本轮蒸发；`acceptance_review_queue.go` 只按 `ResumeByManualReview` 过滤。「等客户补件的都有谁」在库里结构上恒为空。

### 结论：第三种——自愈不成立；选 A

ADR-0086 那一句的两半都不成立（依赖不会自己回来；回来了也不是那一封信封受益）。按 ADR-0094 自己立的判据「看恢复动作」，`等待受控补充`的续办方不是本进程，本就该与人工复核、运营登记同组。**落文 [ADR-0106](../../../docs/adr/0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md)**：`ResumeByCustomerSupplement` 改为提交入账；ADR-0086 Decision 一的保存护栏原样扩用；续办由「新提交版本已形成」信封驱动，转交同一个 `AdvanceAcceptanceChainHandler`；处置翻转与信封**同笔落地**（ADR-0094 Decision 四那句对这一格逐字成立）；受控补充编排的生产入口随之接进装配，Intake 以 `UnconfiguredIntake{}` 起步，客户渠道身份（`PAR-INT-01`）是实例半边留空。ADR-0086 Context 那一半与 ADR-0094 Decision 三那一格按 README「部分停用」办法标注，正文不改写。

**前提之一**：ftr/07 的 D4 两半（PC 侧 `542ebc3` 在 main；PS 侧 `mcp6-ftr07-d4-ps@574eb6c` 待重放）与 D5 两片已把「入账 + 等待态落库 + 信封驱动续办」整条形状做实一次，本裁决是把同一形状套到第三个等待态，不是新发明。

**B 与 C 为何不取**：B（先做重触发切片、维持回滚）——触发做出来之后旧信封的重投仍然什么也买不到，读面恒空也原样留着；B 的活在 A 里是「触发那一半」不是替代选项。C（补取证锚）——等于接受一个续办方明确在进程之外的等待态永久没有队列、照样烧到 `ABANDONED`；取证结论是「前提不成立」，把它写成注释而不改处置是把证据当装饰。

### 本票转实施票，范围

1. `undecidedDisposition` 受控补充那一格改交 nil（入账），`undecided_disposition_test.go` 对应翻转；`form_acceptance_decision.go` 在交回`等待受控补充`前 `Save` 带等待态的聚合（形照 `pauseForManualReview` / D5 `AwaitOperatorRegistration`），保存失败改交保存那一格的原因；`form_acceptance_decision_test.go` 加暂停三测的受控补充版。
2. `FormNewSubmissionVersionHandler` 经 Outbox 交出「新提交版本已形成」信封（事件类型命名照 ADR-0086 的 `manual-review-completed` 形，词取 ADR-0045「新提交版本」）；`cmd/parcel-api` 加事务边界壳（形照 `submissionBoundary` 与复核完成边界壳）；`cmd/parcel-dispatch` 路由表与消费者名册各加一行，转交 `AdvanceAcceptanceChainHandler`；EventID 按（来源身份 + 新提交版本）导出。
3. 受控补充的端点进 `cmd/parcel-api` 装配，`UnconfiguredIntake{}` 起步；探针表与 unwired 占位随行。
4. 队列读面：`task_waiting_on = CUSTOMER_SUPPLEMENT AND state = SUBMITTED` 的读口，形照复核队列与登记队列。
5. **1 与 2 同一笔提交**，任何一半单独落地都不许；3、4 可各自成笔紧随。`cmd/parcel-dispatch/**` 与 `cmd/parcel-api` 装配文件是共享接线文件，开工前占号、落笔前排队列。

### 能力边界

读了 ADR-0086 / 0094 / 0045 / 0029 / 0055 全文、`undecided_disposition.go`、`form_new_submission_version.go` 的依赖与符号面、`judgment_continuation.go` 的 `resumePath()`，以及 report.md B 组对本票的取证；**未读** `form_acceptance_decision.go` 里 `pauseForManualReview` 的具体实现与 D5 那两片的 `Save` 护栏代码——本裁决只定处置与形状，护栏怎么写照先例由实施方对；**未跑**任何进程，问 2 的运行时落格未量。
