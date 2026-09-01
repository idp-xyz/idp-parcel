# ADR-0086: 等待人工复核是入账暂停——暂停与等待态同事务落库，续办由「复核已完成」信封另行驱动

Status: Accepted
Date: 2026-08-31

## Context

[ADR-0081](./0081-acceptance-judgment-is-envelope-driven.md) Decision 三把接受链的未决处置裁成一格：任一步未决即整笔回滚等重投。这一格对三个等待态里的两个成立，对第三个不成立，而票 [admin-skeleton-closure-batch/09](../../.scratch/admin-skeleton-closure-batch/issues/09-acceptance-review-wiring.md) 要接的复核队列读面恰好踩在第三个上：

- **等待受控补充 / 等待内部续办**：重投重跑同一轮，等的依赖（客户新提交版本、抖动的内部依赖）会自己回来，回滚丢掉的中间态在下一轮重新推出。回滚重投是对的。
- **等待人工复核**：CONTEXT 明写它是第三条续办路径——「内部重试推进不了它，客户也补不出它」。重投重跑一万轮，`FormAcceptanceDecision` 都停在同一格，每一轮消耗一次失败预算；预算耗尽后信封停投，此后即便复核完成也没有任何投递会再来推这条链。更早发生的是读面失真：未决整笔回滚让 `waitingOn = MANUAL_REVIEW` 从不落库，「等复核的都有谁」这个队列在库里恒为空——**排队的委托与没人管的委托在读面上长着同一张脸**。

矛盾不在 ADR-0081 选信封驱动，在它的未决语义把「等一个会自己回来的依赖」与「等一个要人来做的动作」折进了同一格。两者的续办方不同（这正是领域 `ResumePath` 三分的理由），处置就不能同格。

## Decision

**一、`等待人工复核`是暂停，不是可重投的未决。** `FormAcceptanceDecisionHandler` 在交回该原因**之前**先把带等待态的聚合 `Save` 落库（`pauseForManualReview`）；消费门（`ShipmentRequestSubmittedConsumer`）读到 `ManualReviewPending` 时按「本份投递处理完毕」提交入账，不再重投——暂停与入账在消费门的同一个事务里成立。保存失败或版本冲突时**不得**交回`等待人工复核`：该原因是消费门提交入账的凭据，暂停没落库就交它，等待态随本轮回滚蒸发且投递已被记为完毕，队列从此永远列不出这份委托。改交保存那一格自己的原因（`决定没落库`/`换代冲突`），消费门照旧回滚重投，下一轮重新走到暂停。

**二、续办由「复核已完成」信封驱动，事件类型 `parcel-shipment.shipment-request.manual-review-completed`。** 复核完成命令（`CompleteManualReviewHandler`）本身不驱链——在编排里顺手推链，判断就有第二个驱动点（该处理器注释记着这条）。信封交接由 `cmd/parcel-api` 的事务边界壳承担：完成落库与信封入队在同一个事务里成立或一起消失，形状照提交装配点的 `submissionBoundary` + Outbox 交接先例。`cmd/parcel-dispatch` 为该类型登记第二个消费门，转交**同一个** `AdvanceAcceptanceChainHandler`；重跑的链在可达性与财务控制两步读回已记录判断即过，形成决定一步读到 `ManualReviewCompleted` 即成决定。

**三、这不是 ADR-0081 否决的「第二条推进路径」。** 那一条否决的是把链的中间态拆成中间信封接力——中间态已落在接受判断任务里，第二处状态要与第一处对账。「复核已完成」不是链的中间态：它是链外一次人的动作的完成事实，发生时链已停摆且没有任何在途投递。推进者仍然只有接受链这一个编排；两类信封是同一次驱动的两个触发时机，不是两条推进路径。同理它也不动 ADR-0081 对人工复核与主动拒绝命令面的保留条款——主动拒绝（`RejectShipmentRequestHandler`）在自己的命令事务里直接形成决定，不需要也不得再发续办信封。

**四、失败面照旧分格。** 「复核已完成」信封重跑的链若停在**其它**未决原因（时点、依赖抖动），按 ADR-0081 原语义回滚重投——那些依赖会自己回来；若再次停在`等待人工复核`（构造上只剩一种可能：新提交版本换代重开了复核），按 Decision 一再暂停一次。毒丸、集合外结果、装配缺件三格维持响亮，与「委托已提交」消费门同一张哨兵名单。

## Consequences

- **复核队列读面从此如实**：`task_waiting_on = MANUAL_REVIEW AND state = SUBMITTED`（迁移 0009 投影列）的行集恰是「此刻停等复核」的委托集，队列语义不靠读侧推断。
- **失败预算不再被复核烧穿**：等复核的委托不占用重投；预算留给真正抖动的依赖。
- `cmd/parcel-api` 出现提交之外的第二个 Outbox 交接装配（复核完成边界壳）；`cmd/parcel-dispatch` 的路由表与 inbox 消费者名册各多一行。EventID 按（来源身份 + 提交版本）导出：同一版本至多一次`已记录`完成（领域重复完成拒绝），重放不入队第二份。
- 复核完成后的推进是**下一拍**：命令的同步答复只到`已记录`为止，与 ADR-0081「接受在下一拍」同形。信封消费失败由它自己的重投续办，不回头改命令答复。
- 代价：暂停那一格的写入从零次变一次（含投影列），消费门对 `ManualReviewPending` 的入账语义与其余未决相反——两处都有测试钉着（`form_acceptance_decision_test.go` 暂停三测、消费者测试的例外格）。

## Alternatives considered

- **纯重投直到复核完成**（票 09 事实基线最初按 ADR-0081 的读法）。否决：失败预算耗尽后信封停投，完成的复核再也推不动链；等待态从不落库，队列读面结构上建不出来。
- **复核完成命令同步驱链。** 否决：同一份判断任务两处驱动，未决语义各自漂移；ADR-0081 对路 A 的否决理由整段适用于这半条路。
- **定时轮询重驱停等复核的委托。** 否决：给同一件事立第二种节拍与第二个无主的调用方；outbox/inbox 的续办语义已经覆盖「完成即驱」，轮询只买到延迟与空转。
- **消费门对 `ManualReviewPending` 也入账但不发续办信封（留给人工重放）。** 否决：把「谁来续办」变成新的实例半边缺口，与 ADR-0081 否决路 A 的第一条理由同构。

## Links

- [ADR-0081：接受判断由信封驱动](./0081-acceptance-judgment-is-envelope-driven.md)：Decision 三的未决语义是本记录细分的对象；命令面保留条款是 Decision 三的边界
- [票 09：接受前人工复核页接线](../../.scratch/admin-skeleton-closure-batch/issues/09-acceptance-review-wiring.md)：队列读面与两个命令口的驱动票
- [CONTEXT.md](../domain/parcel-shipment/CONTEXT.md)：接受判断任务三个等待态「使用不同原因和续办路径」——本记录把处置也分开
- [ADR-0031](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)：`决定没落库`/`换代冲突`的原因分格，Decision 一的保存失败语义沿用它
