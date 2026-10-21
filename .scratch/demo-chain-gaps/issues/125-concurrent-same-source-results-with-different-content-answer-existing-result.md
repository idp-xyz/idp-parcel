# 125 同一来源标识、内容不同的两份关务结果并发到达：后到那份答「已有记录」，不答「来源冲突」

Category: bug
Status: needs-triage——2026-10-21 Claude Code 协调方立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [12](./12-successive-same-layer-customs-results-are-judged-conflicts.md) 的评审修订（作者「顺带核得、未改」那一条）。
Blocked by: 无
出处：[12](./12-successive-same-layer-customs-results-are-judged-conflicts.md) 票面「评审记录」；[ADR-0226](../../../docs/adr/0226-successive-same-layer-customs-results-are-successors-unless-they-contradict.md) 决定二（同层一致性比对按提交版本串行，不覆盖幂等读）。

## 现象

CC 收外部结果时先按幂等键读已有记录：读到了就比内容摘要，摘要不同答 `SOURCE_CONFLICT`。两份同一来源标识、内容不同的结果并发到达时，两笔事务在这一步都读不到对方，都往下走；后落册的那笔撞键，读回赢家后直接按已有记录作答，答 `EXISTING_RESULT` 并重发赢家的意图。来源方因此以为自己那份已被接受，实际上被静默丢弃，「同一来源前后说法不一」这件事没有留痕。

票 12 加的咨询锁只串行同层一致性比对那一段（在幂等读之后），挡不住这一格。这一缝在票 12 之前就有。

## 证据（读码）

`internal/customscompliance/application/receive_external_result.go`：`Handle` 开头 `FindByKey` 读到记录时经 `domain.CompareStoredDigest` 区分已有与来源冲突；落册那一步遇 `ports.ExternalResultAlreadyRecorded` 时读回赢家，交给 `existingResult` 作答，不再比摘要。

## 三类

机制。规模 S。

## 最小的诚实修法

落册撞键、读回赢家之后，照开头那一步同样比内容摘要：一致答 `EXISTING_RESULT`，不一致答 `SOURCE_CONFLICT`。补一条真库并发用例：同一来源标识、内容不同的两份并发到达，恰有一份落册，另一份答 `SOURCE_CONFLICT`。顺带核一下 CC 别的接收口（申报回执等）在撞键分支上是否同病。

## 要裁的

无（与开头那一步同一判据，不另立口径）。

## 既有票

none
