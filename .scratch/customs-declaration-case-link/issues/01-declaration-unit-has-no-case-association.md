# 申报单元 → 关务案件的关联在域模型里缺席，只活在文档

Category: bug
Status: needs-triage

CC CONTEXT 的「关务案件」词条写着「**一个案件可以关联多个申报单元和多次提交**」，而代码里
没有这条关联：`DeclarationSubmissionKey` 三维（租户+申报单元+程序）不含案件维，申报提交口的
载荷同样不含，`submit_declaration.go` 全文对 `Case`／`案件` 零命中。文档说得出的关系，代码里
取不出来。

从 [outbox-partition-key 票 03](../../outbox-partition-key/issues/03-step-two-scope-eight-ports-and-four-undecided.md)
的裁断轮里分出来（[ADR-0069](../../../docs/adr/0069-customs-case-chain-ordering-absorbed-by-reread-and-retry.md)
决定四点名另票）。那一轮要答「案件链要不要保序」，答案的一半卡在这里：**申报口今天连案件维
都取不出，想把它归进案件分区就无从谈起**。裁断因此绕开了它——四口不建立跨口保序，乱序由重读
与重试消化——但绕开的是保序需求，不是这条关联本身。

## 卡在哪

要把案件维放进申报侧的任何位置（载荷、Subject、将来的分区维），先得有其中之一：

- 申报单元 → 案件的可查关联（域模型里的一条边，带持久化）；或
- 提交意图在输入侧就携带案件引用。

两者今天都没有。缺哪一个、由谁提供、关联建在申报单元上还是建在案件上，是建模问题不是接线
问题——`customs-compliance` 当前无主，需要先定所有权再动。

## 已经定死的边界（不要在本票里重开）

- 关联落地后，申报意图在**载荷/Subject** 补案件引用，**不进分区键**（ADR-0069 决定四）。
- 案件维一律用铸造 `CustomsCaseID`，不用五维范围键——范围键的职责收敛为建案幂等
  （ADR-0069 决定三）。
- 申报信封 ID 缺版本维是**另一件事**，归
  [declaration-envelope-version-dedup/01](../../declaration-envelope-version-dedup/issues/01-envelope-id-lacks-version-dimension.md)，
  两票互不吸收。

## 本票不做的事

- 不改分区键。申报口的分区键已由 ADR-0069 判为「无先后」，门禁例外行已按此改写。
- 不替 `customs-compliance` 定所有权。
