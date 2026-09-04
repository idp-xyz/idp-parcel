# ADR-0094: 未决重投与否由领域的 `ResumePath` 决定，并增设「等运营登记」第四格

Status: Accepted（部分停用：Decision 三对 `ResumeByCustomerSupplement` 的「维持回滚重投」一格，已由 [ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md) 改为提交入账——那一格写明「另立取证票，不在本记录裁」，取证票 first-tenant-runway/09 的三问已从代码答出；Decision 一、二、四、五与其余三格不变）
Date: 2026-09-02

## Context

[ADR-0086](./0086-manual-review-wait-is-a-committed-pause-resumed-by-completion-envelope.md) 已经立过本记录要用的那条原则，它的 Context 原话是：把「等一个会自己回来的依赖」与「等一个要人来做的动作」折进同一格是错的，因为**两者的续办方不同——「这正是领域 `ResumePath` 三分的理由」**。但它只把 `ManualReviewPending` 一个取值挑出来单独处置，落到消费门里是一次常量比较。原则立了一半，兑现了三分之一。

票 [07](../../.scratch/first-tenant-runway/issues/07-undecided-that-never-self-heals-burns-the-retry-budget.md) 撞见的是没兑现的那部分：隔离形态下真发一笔提交，只灌治理权威区间、不灌商业主数据，链如实答未决，重投烧完预算后 outbox 落 `ABANDONED` + `dispatch.consumer_undecided`，委托停在`已提交`，此后即便运维把数据登记齐了也没有任何东西会再驱动这一封（仓内零重驱机制）。

**那次未决属于 `ResumePath` 今天没有的第四类。** `JudgmentPendingReason.resumePath()` 的 default 把「其余取值」全归 `ResumeByInternalRetry`，注释写的是「只有本方推得动」。这句话把两件事折在一起：**本进程重试推得动**，与**本方运营去登记推得动**。前者自动，后者要人。

而这个区别在同一份文件里已经被写下来过，只是没有进 `ResumePath`。`RejectionAuthorityRulesNotConfigured` 那一组的注释说得比票面还清楚：它与 `*AuthorityUnavailable` 分开，是因为「后者是授权服务答不出、等它恢复，前者是这个范围此刻一条现行规则都没有、**等租户把 `PAR-COM-14` 登记上**」，并明写「压成一格会**对着一个没配置的租户参数无休止内部重试，而重试永远等不到一次登记**」。`ReachabilityAsOfNotConfigured` 早为同一个参数写过同一句话。

所以缺的不是判据。[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 早已给出——按**恢复动作**分格，不按提供方的失败原因分格。缺的是把这条判据在 `ResumePath` 上兑现：原因那一层已经分得足够细，恢复动作那一层少一格，于是细分在导出时又被压回去了。

## Decision

**一、消费门按 `ResumePath` 折，不按 `JudgmentPendingReason` 的具体取值折。** `advanceAcceptanceChainThrough` 里那次 `result.PendingReason() == ManualReviewPending` 常量比较，换成按恢复动作逐格分派，**不留 default**——理由与同上下文的 `saveStallReason`、`asOfPendingReasons.forOutcome` 一致：日后多一格时这里要报错，而不是静默继承某一格，而那一格决定的是烧不烧失败预算。

**二、`ResumePath` 增设第四格 `ResumeByOperatorRegistration`：等运营在本产品里登记实例半边参数。** 入格判据是恢复动作而不是缺了什么东西：**重试、客户补件、人工复核三者都推不动它，只有一次登记动作推得动。** 今天应落此格的是 `RejectionAuthorityRulesNotConfigured`、`WithdrawalAuthorityRulesNotConfigured`、`SourceDataAmendmentAuthorityRulesNotConfigured` 与 `ReachabilityAsOfNotConfigured`、`FinancialControlAsOfNotConfigured`——在 `resumePath()` 里逐个显式列出，不靠 default 兜。

**三、四格处置。**

- `ResumeByInternalRetry` → 回滚重投（[ADR-0081](./0081-acceptance-judgment-is-envelope-driven.md) Decision 三原语义不变）。
- `ResumeByManualReview` → 提交入账（ADR-0086 Decision 一不变，只是改由本格承载，不再靠常量比较）。
- `ResumeByOperatorRegistration` → 提交入账，等待态与处理尝试落库，不烧重投预算。
- `ResumeByCustomerSupplement` → **维持回滚重投**。ADR-0086 的 Context 判过这一格（「客户新提交版本会自己回来」），本记录**不改它**：[ADR-0045](./0045-new-submission-version-keeps-history-and-reestablishes-the-task.md) 把「受控补充的授权、身份签发、重触发判断」划为另一切片，因此「会自己回来」这条前提今天既核不实也证不伪，而推翻一条已接受判断要有证据。它另立取证票，不在本记录裁。**它在这里是一个具名的、写着理由的格，不再是一个没有落点的沉默 default**——这本身就是变化。

**四、第三格必须与它的续办触发同笔落地，否则不许落地。** 人工复核有「复核已完成」信封，客户补件有新提交版本，「参数已登记」今天什么都没有。只把回滚改成提交而不给触发，结果是把 `ABANDONED` 换成一个**更安静的永久停滞**：委托仍停在`已提交`，只是不再有一个失败码指向它。续办由**登记动作发信封**驱动，形照 ADR-0086 Decision 二与 `cmd/parcel-*-register` 家族；不采用定时扫描重驱，理由与 ADR-0086 否决轮询那条相同。

**五、ADR-0086 Decision 一的保存护栏原样扩用到第三格。** 那句话是：暂停没落库就不得交回该原因，否则等待态随本轮回滚蒸发而投递已被记为完毕，队列从此永远列不出这份委托。对第三格逐字成立。落此格前必须先把带等待态的聚合 `Save` 落库；保存失败时改交保存那一格自己的原因（其恢复动作是内部重试），照旧回滚重投，下一轮重新走到这里。

## Consequences

- **消费门不再持有任何 `JudgmentPendingReason` 字面量。** 新增一个未决原因时要回答的是「它的恢复动作是哪一格」，而那个问题在 `resumePath()` 这个全函数里本来就必须回答——两处不会再各答一次，也就不会漂开。
- **「等登记的都有谁」这个队列第一次在库里成立**，形照 ADR-0086 给复核队列开的那格投影。它同时兑现了 [ADR-0095](./0095-undecided-stage-and-reason-surface-in-two-layers.md) 说的一半可观测性：提交即留痕，`recordAttempt` 写下的原因、恢复路径与续办引用不再被整笔回滚擦掉。
- **失败预算从此只花在真会自愈的依赖上。**
- 代价：`ResumePath` 是领域封闭集合，加一格会打到接受判断任务、判断译码、重建门与穷尽门禁。按 [parallel-sessions](../agents/parallel-sessions.md) 的判据这属「会让旧调用点对不上」的一类，实现时走三步法或单独 worktree，并在频道占号。
- 代价：这张票的实现范围比票 07 票面大——Decision 四要求续办信封同笔落地，它不是改一处折法。

## Alternatives considered

- **在消费门里判「这次未决属哪一格」**（票 07 候选一）。否决：那等于在适配层重建一份 `resumePath()` 已有的映射，两份必然漂开；而恢复动作是领域语言，不是适配层的判断。
- **未决一律不烧预算**（票 07 候选二）。否决：真会自愈的依赖也失去自动重投。ADR-0086 的 Context 已按同一理由把内部续办那格留在重投侧。
- **补一条受控重投口**（票 07 候选三）。否决：`UC-PS-001` 要的是「安全续办」不是「人工重放」；它把一个设计缺口固化成一道永久运维动作。
- **只加第四格，消费门仍按原因常量比较。** 否决：那把「哪些原因属哪一格」的知识第二次写进适配层，第一次写在 `resumePath()` 里。

## Links

- [ADR-0086](./0086-manual-review-wait-is-a-committed-pause-resumed-by-completion-envelope.md)：本记录把它的 Decision 一从一个取值推广到一族恢复动作，其保存护栏原样扩用
- [ADR-0081](./0081-acceptance-judgment-is-envelope-driven.md)：Decision 三的未决语义是被细分的对象
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格的判据
- [ADR-0045](./0045-new-submission-version-keeps-history-and-reestablishes-the-task.md)：受控补充的重触发判断另成切片——Decision 三不动那一格的理由
- [ADR-0095](./0095-undecided-stage-and-reason-surface-in-two-layers.md)：本记录带走可观测性的一半，那一篇裁另一半
- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：「安全续办」与`尚未决定`的结果语义
- [票 07](../../.scratch/first-tenant-runway/issues/07-undecided-that-never-self-heals-burns-the-retry-budget.md)：现场与三条候选路
- [ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md)：前向指针——Decision 三刻意留待取证的那一格（`ResumeByCustomerSupplement`）证据齐后由它改判为提交入账，Decision 四「触发同笔落地」对那一格逐字沿用
