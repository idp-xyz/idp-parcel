# 「等待受控补充」真的会自愈吗——ADR-0086 那条前提没人核过

Category: bug
Status: needs-info（要一次取证才谈得上裁；取证做完才知道是不是缺陷）

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
