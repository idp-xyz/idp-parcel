# ADR-0095: 未决的停站与原因分两层出声——不自愈那格靠入账留痕，自愈那格靠装配方注入的失败观察口

Status: Accepted
Date: 2026-09-02

## Context

消费门刻意把停在哪一站与未决原因都写进错误正文（`advanceAcceptanceChainThrough` 那句 `stage %s, reason %s`），`ErrAcceptanceChainUndecided` 的注释写着「原错误原样留在链上供运维读」。

**生产接线里没有任何东西在读那条链。** `Dispatcher.DispatchOnce` 逐条发布失败时，只把 `failureCodeFor(err)` 交给 `RecordFailure` 写进库，随即 `continue`——`err` 本体就地丢弃，不返回也不记；`cmd/parcel-dispatch` 的 `Loop.report` 只在 `DispatchOnce` **整拍**返回错误时才打日志，而逐条失败按上一条根本不构成整拍错误。实测（票 [08](../../.scratch/first-tenant-runway/issues/08-undecided-stage-and-reason-are-invisible-on-a-real-process.md)，锚 `9d6063c`）：dispatch 连跑 11 分钟、库里那一行累计到 `attempt = 24`，进程输出零行。

落到库里的只有一个失败码，而失败码按 `failureCodeFor` 的判据取值——它答的是「运维该做哪一类动作」，本来就不负责答「停在哪一站」。两者都需要，但不是同一件东西。

**而领域其实一直在记。** `recordAttempt` 会把未决原因、恢复路径与续办引用写成一条处理尝试挂到接受判断任务上，`UC-PS-001` 要的「保存当前判断、失败位置和安全续办依据」正是它。**是消费门的整笔回滚把它擦掉了**——该消费门自己的注释记着这条代价：「推进途中记下的处理尝试与已记录判断一并回滚」。

所以这不是一个缺口，是两个，而且它们的正确处方相反。

## Decision

**一、不自愈那一格由 [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 带走，本记录不另设机制。** 那一格改为提交入账之后，`recordAttempt` 写下的东西就留在库里，可查询、可做队列、可按原因统计。它本来就该落在持久面上：那是要人来看、要能按范围查回来的东西，日志承载不了。

**二、自愈那一格由装配方注入的失败观察口承载。** `dispatch.Dispatcher` 增一个**可选**的逐条失败观察回调，随 `NewDispatcher` 的既有依赖给出，在 `RecordFailure` 成功之后、`continue` 之前调用，交出投递引用、已分格的失败码与**原始 `err`**。`cmd/parcel-dispatch` 用自己的 `slog.Logger` 实现它。

这一格该进日志而不是进库，正因为它按定义是瞬时的：等的依赖会自己回来，回滚是对的，回滚就意味着库里不该留下这一轮的痕迹。要留的是「它此刻在等什么」这个观察，不是一条业务事实。

**三、不改 `Beat`，不给平台层塞 logger。** `internal/platform/dispatch` 的包注释明写循环、批量与重试节奏是装配职责，它对事件类型一无所知、十个上下文共用同一拍。观察口只把它手上**已经有**的那个 `err` 交出去，不决定怎么出声、也不决定出声到哪里——那两件仍归装配方。

**四、观察口不得改变派发行为。** 只观察：返回值不看，失败不影响这一拍的推进（一条日志写不出去不该变成一次投递失败），回调为 `nil` 时行为与今天逐字相同。

## Consequences

- 「原错误原样留在链上供运维读」这句话第一次成立。
- **可观测性分两层，与两种未决一一对应**：会自愈的进日志，不自愈的进库。这个对应不是巧合——它就是 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 那条判据（按恢复动作分格）在可观测性这一面的投影。
- 代价：`NewDispatcher` 多一个可选依赖；既有装配点不受影响（`nil` 即旧行为）。
- 代价：日志里会出现按重投节奏重复的未决行。那是真实的，不遮；要压噪由装配方在自己那一层决定——它有 logger，也只有它知道这个部署的节奏。
- 这道缝只补**观察**，不补**告警**。「未决持续多久算异常」是运营口径，属实例半边，本记录不填。

## Alternatives considered

- **派发器自己打日志**（票 08 候选一）。否决：把「怎么出声」从装配方收回平台层，与该包既有的分工相反；而那条分工不是风格偏好，是十个上下文共用一拍的前提。
- **`DispatchOnce` 交回逐条结果**（票 08 候选二）。否决：`Beat` 是所有装配点共用的接口，为一件观察需求拓宽它代价与收益不成比例；且「交回什么」（整个 `error` 还是已分格的摘要）会立刻变成第二个要裁的形状问题。
- **只靠 ADR-0094 的入账留痕，自愈那格不管。** 否决：自愈那一格恰恰是「一直在等某个依赖」看起来最像正常的那一格——消费门的注释原话——而它今天连一次都不出声。
- **把 stage 与 reason 编进失败码。** 否决：失败码按恢复动作分格且受框架 128 字节与字符集约束，塞进位置信息会让码面同时答两个问题，`failureCodeFor` 的分格判据随之失去单一含义。

## Links

- [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)：不自愈那一格的处置，本记录的第一层依赖它落地
- [ADR-0081](./0081-acceptance-judgment-is-envelope-driven.md)：失败码三格与未决哨兵的翻译位置
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：两层划分沿用的同一条判据
- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：「保存当前判断、失败位置和安全续办依据」
- [票 08](../../.scratch/first-tenant-runway/issues/08-undecided-stage-and-reason-are-invisible-on-a-real-process.md)：实测现场与两条候选路
