# ADR-0106: 等待受控补充是入账暂停——与人工复核、运营登记同组提交入账，续办由「新提交版本已形成」信封驱动；停用 ADR-0086 Context 对这一格的分侧

Status: Accepted
Date: 2026-09-04

## Context

[ADR-0086](./0086-manual-review-wait-is-a-committed-pause-resumed-by-completion-envelope.md) 的 Context 把接受判断任务的三个等待态分成两侧，`等待受控补充`被判在「回滚重投是对的」那一侧，理由原话是「重投重跑同一轮，等的依赖（客户新提交版本、抖动的内部依赖）会自己回来」。[ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 立了「按恢复动作分格」的原则并把三个等待态里的两个（人工复核、运营登记）做成提交入账，唯独对 `ResumeByCustomerSupplement` 写明「维持回滚重投」——不是因为它按恢复动作该在重投侧，而是因为 [ADR-0045](./0045-new-submission-version-keeps-history-and-reestablishes-the-task.md) 把受控补充的重触发判断划为另一切片，「会自己回来」那条前提当时既核不实也证不伪，推翻一条已接受判断要有证据。它把取证交给票 [first-tenant-runway/09](../../.scratch/first-tenant-runway/issues/09-does-waiting-for-a-customer-supplement-actually-self-heal.md)，并在 `undecidedDisposition` 的注释里写下「若那一票判出它不自愈，改的就是这一行」。

证据现在有了。以下三条都是从代码读出来的事实，不是推断，取证锚 `main = 512b419`（2026-09-04）：

1. **受控补充编排不铸任何信封，也没有生产调用方。** `FormNewSubmissionVersionHandler` 的依赖是 `FormNewSubmissionVersionDeps{Sources, Requests, Identities, Clock}`，没有 outbox 或 handoff 端口，整个文件不出现 `Envelope` / `Publish` / `HandOff` / `EventType` 任何一个符号；`NewFormNewSubmissionVersionHandler` 在全仓非测试代码里的引用只有它自己的声明，`cmd/` 零命中。所以「客户新提交版本会自己回来」在今天的仓里没有任何机制承载——新版本形成之后，没有任何东西会去驱动那条停着的接受链。
2. **`waitingOn = CUSTOMER_SUPPLEMENT` 在结构上从未落库。** 生产上接受判断链只经 `cmd/parcel-dispatch` 的消费门跑；`undecidedDisposition` 对 `ResumeByCustomerSupplement` 交回哨兵、整笔回滚，等待态随本轮一起蒸发；复核队列读面 `acceptance_review_queue.go` 只按 `ResumeByManualReview` 过滤。「等客户补件的都有谁」这个队列在库里恒为空——这与 ADR-0086 当年给人工复核开例外时列的第二条理由（「排队的委托与没人管的委托在读面上长着同一张脸」）逐字同构。
3. **即便新版本另行到达，重投旧信封也推不动链。** 在途那一封信封携带的是旧提交版本；重跑它要么停在同一格白烧一次失败预算，要么因换代落到 `StaleShipmentRequestRevision`（`judgment_continuation.go`）——两种结果都不是「续办」。续办只能来自新版本自己的驱动。也就是说，`等待受控补充`的续办方与人工复核、运营登记一样**不是本进程**：一次客户动作推得动它，重试推不动。

三条合起来：ADR-0086 那一句的两半都不成立——依赖不会「自己回来」，而回来了也不是那一封信封受益。按 ADR-0094 自己立的判据（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格），这一格本就该与人工复核、运营登记同组。ADR-0094 当时刻意不动它是对的——没有证据不推翻已接受判断；现在动它同样是对的——证据齐了。

前提还有一条：ADR-0094 Decision 四要求的「第三格与它的续办触发同笔落地」已经兑现——票 [first-tenant-runway/07](../../.scratch/first-tenant-runway/issues/07-undecided-that-never-self-heals-burns-the-retry-budget.md) 的 D4 两半（`party-commercial` 侧 `542ebc3` 在 main；`parcel-shipment` 侧在待重放分支）与 D5 两片（`a9e3440`、`a3adb75`）把「入账 + 等待态落库 + 登记信封驱动续办」整条形状做实了一次。本记录不是再发明一次，是把同一形状套到第三个等待态上。

## Decision

**一、`ResumeByCustomerSupplement` 改为提交入账，与 `ResumeByManualReview`、`ResumeByOperatorRegistration` 同组。** `undecidedDisposition` 里那一格从交回哨兵改为交回 nil：消费门按「本份投递处理完毕」提交入账，不再重投。自此四格的处置是：`ResumeByInternalRetry` 一格回滚重投，其余三格提交入账；仍然**不留 default**（ADR-0094 Decision 一的理由不变）。

**二、ADR-0086 Decision 一的保存护栏原样扩用到这一格。** 编排在交回`等待受控补充`**之前**先把带等待态的聚合 `Save` 落库（形照 `pauseForManualReview` 与 D5 的 `AwaitOperatorRegistration`）；保存失败或版本冲突时不得交回该原因——那是消费门提交入账的凭据，暂停没落库就交它，等待态随本轮回滚蒸发而投递已被记为完毕，队列从此永远列不出这份委托。改交保存那一格自己的原因（其恢复动作是内部重试），照旧回滚重投，下一轮重新走到暂停。

**三、续办由「新提交版本已形成」信封驱动，且必须与决定一同笔落地。** 受控补充的编排（`FormNewSubmissionVersionHandler`）落新版本时经 Outbox 交出一封信封，事件类型形照 ADR-0086 的 `parcel-shipment.shipment-request.manual-review-completed`，命名按 ADR-0045 的语言取「新提交版本」；`cmd/parcel-api` 为它加一个事务边界壳（新版本落库与信封入队同事务成立或一起消失，形照 `submissionBoundary` 与复核完成边界壳）；`cmd/parcel-dispatch` 为该类型登记消费门，转交**同一个** `AdvanceAcceptanceChainHandler`。重跑的链读到新版本，在受控补充那一步不再停。EventID 按（来源身份 + 新提交版本）导出：同一版本至多形成一次，重放不入队第二份。

**只把回滚改成提交而不给触发是被禁止的**——ADR-0094 Decision 四对第四格写的那句话对这一格逐字成立：那会把 `ABANDONED` 换成一个更安静的永久停滞。所以决定一与决定三在同一笔提交里落地，任何一半单独落地都不许。

**四、受控补充编排的生产入口随本记录一起接，实例半边留空。** 决定三要求编排铸信封，而编排今天没有生产调用方——不接入口，信封永远不会被铸出来。入口的形状按 ADR-0055 / ADR-0085 的两阶段接线：端点进 `cmd/parcel-api` 装配，Intake 以 `UnconfiguredIntake{}` 起步；客户渠道的采信身份属 `PAR-INT-01` 接入契约，是实例半边，本记录不替它定。机制半边（端点、边界壳、信封、消费门）现在就做。

**五、ADR-0086 Context 对`等待受控补充`的分侧停用，ADR-0094 Decision 三对该格的「维持回滚重投」停用；两份记录其余各条不变。** 这两处按 [README](./README.md) 的「部分停用」办法标注：改 `Status` 行与 Links 节加前向指针，正文不改写。ADR-0086 Context 对`等待内部续办`的判断（重投是对的）不在本记录范围内，继续有效。

## Consequences

- **消费门只剩一个回滚格。** `等待内部续办`是四格里唯一「本进程重试推得动」的，也是唯一该烧失败预算的；其余三格的续办方都在进程之外，各有自己的触发信封。ADR-0094 Consequences 那句「失败预算从此只花在真会自愈的依赖上」到此才完全成立——它原先留着一格例外。
- **「等客户补件的都有谁」这个队列第一次在库里成立。** 形照 ADR-0086 给复核队列、ADR-0094 给登记队列开的投影：`task_waiting_on = CUSTOMER_SUPPLEMENT AND state = SUBMITTED` 的行集恰是此刻停等补件的委托集。
- **受控补充这条路第一次在生产上可达。** 今天它连入口都没有，`FormNewSubmissionVersionHandler` 只活在测试里；本记录的实现票要把它接进装配。代价是 `cmd/parcel-api` 多一个 Outbox 交接装配、`cmd/parcel-dispatch` 路由表与消费者名册各多一行——与 ADR-0086 落地时的代价同形。
- **旧信封不再有「重跑到 `StaleShipmentRequestRevision`」这一幕。** 停在受控补充的投递已经入账，没有在途信封等着被新版本换代；`StaleShipmentRequestRevision` 仍然保留，它守的是别的重跑路径。
- **`ResumePath` 的语义从此与 ADR-0029 完全对齐**：四格各自对应一个恢复动作，且处置由恢复动作唯一决定。日后再加一格时，要回答的仍然只有「它的恢复动作是什么」。
- 代价：`undecidedDisposition` 的测试要翻一格（`undecided_disposition_test.go` 里受控补充那条从「回滚」改「入账」），`form_acceptance_decision_test.go` 加暂停三测的受控补充版；消费者测试的例外格从两个变三个。

## Alternatives considered

- **B：维持 ADR-0086，先做 ADR-0045 划出的「重触发判断」切片，让前提成真。** 否决：触发做出来之后，旧信封的回滚重投仍然什么也买不到——它携带旧版本，重跑只会白烧预算或落到换代原因（Context 第 3 条），而链真正靠新版本那一封驱动；同时读面恒空的问题原样留着（Context 第 2 条），那正是 ADR-0086 自己列出的两条理由之一。B 的活（铸信封 + 消费门）在本记录里是决定三，不是替代选项。
- **C：维持现状，只在 ADR-0094 那一格补取证锚。** 否决：等于接受这一格今天照样烧到 `ABANDONED`，且接受一个续办方明确在进程之外的等待态永久没有队列。取证的结论是「前提不成立」，把它写成注释而不改处置，是把证据当装饰。
- **在消费门里判「新版本是否已到达」再决定回滚还是入账。** 否决：那是在适配层重建一份编排已有的判断，且引入第二处对提交版本的读取；ADR-0094 否决「在消费门里判这次未决属哪一格」的理由整段适用。
- **定时扫描停等补件的委托、发现新版本即重驱。** 否决：ADR-0086 与 ADR-0094 各否决过一次轮询，理由不变——给同一件事立第二种节拍与第二个无主的调用方。
- **只改处置、入口留给 `PAR-INT-01` 到位后再接。** 否决：决定三的信封由编排铸，编排没有调用方就永远不会铸；而入口的机制半边（端点、边界壳、`UnconfiguredIntake{}`）今天就做得了，实例半边留空并拒绝默认值，这正是 ADR-0055 那一格存在的理由。

## Links

- [ADR-0086](./0086-manual-review-wait-is-a-committed-pause-resumed-by-completion-envelope.md)：本记录停用其 Context 对`等待受控补充`的分侧；其 Decision 一的保存护栏、Decision 二的信封驱动形状原样扩用
- [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)：本记录停用其 Decision 三对 `ResumeByCustomerSupplement` 的「维持回滚重投」；其 Decision 一、二、四、五不变，Decision 四「触发同笔落地否则不许落地」对本记录逐字成立
- [ADR-0045](./0045-new-submission-version-keeps-history-and-reestablishes-the-task.md)：新提交版本保留历史并重建任务；它划出去的「重触发判断」切片由本记录决定三、四承接
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格的判据
- [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：决定四入口以「未配置即拒」Intake 起步的依据
- [票 first-tenant-runway/09](../../.scratch/first-tenant-runway/issues/09-does-waiting-for-a-customer-supplement-actually-self-heal.md)：三问的取证与裁决
- [票 first-tenant-runway/07](../../.scratch/first-tenant-runway/issues/07-undecided-that-never-self-heals-burns-the-retry-budget.md)：D4 / D5 把同一形状做实的先例
- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：「安全续办」与`尚未决定`的结果语义
