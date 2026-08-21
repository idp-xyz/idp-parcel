# ADR-0069: 关务案件链乱序由重读与重试消化，不由分区保证；关闭信封携关闭周期维

Status: Accepted  
Date: 2026-08-21

## Context

[outbox-partition-key 票 03](../../.scratch/outbox-partition-key/issues/03-step-two-scope-eight-ports-and-four-undecided.md)悬置的问句：「同一案件的建立 → 申报提交 → 核对 → 关闭是否需要保序？」今天三口三个键公式，必然落三个分区，链上零顺序保证；且案件有两种身份表达（范围五维键与铸造 `CustomsCaseID`），链上两口各用一种，不统一改什么键都白改（取证：[四处待裁的裁断输入](../../.scratch/outbox-partition-key/evidence-four-undecided.md)）。

消费模型已由既有决定钉死：路由表显式清单（[ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)）、投影只增不改写且取代关系随来源给出（[ADR-0065](./0065-projection-versions-are-append-only-and-supersession-is-source-given.md)）、多对象信封消费侧拆分（[ADR-0066](./0066-multi-object-envelope-unrolls-per-member-on-the-consumer-side.md)）；两条 CC 消费路的载荷都是指针（只带键），处理方按键重读权威态，「按键重读不可见」登记为可重试未决哨兵。

业务侧（国际小包关务运营）的刚性要求是两条：下游读到的**当前案件状态可信**，与**审计链完整**——每份申报版本、每个关闭周期都必须到达且留痕；清关后稽查重开案件是常态（[UC-CC-010](../application/customs-compliance/UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md) 的多关闭周期即为此而写）。跨口到达顺序不在其中：因果先后在写入侧已被应用层钉死（申报只发生在已建立案件的进行中态），下游按键重读时读到的是当前态，不依赖信封先后。

关闭周期序数的取值来源在裁断时另核了一次，结论与取证文件的措辞有出入，照实记：`ports.CaseClosureStore` 接口只有 `FindByCase` 与 `Save` 两个方法，但重开**是**持久化的——`CaseClosures.Save` 在同案已有关闭时走一条只写 `reopenings` 列的 UPDATE，`rebuildCaseClosure` 逐条 `Reopen` 重建，`TestCaseClosureRoundTripsAndReopeningAppendsInPlace` 守着这条往返。因此「重开次数」在关闭记录自身上取得到，且跨库往返不丢。

## Decision

**一、案件链四口不建立跨口同分区保序。** 本决定的成立前提有四，任一被打破须重裁：(1) 链上信封载荷保持指针式（只带键，不带可变业务态）；(2) 消费者按键重读权威态，不用信封先后拼装状态；(3) 「按键重读不可见」在消费侧归**可重试**未决哨兵，不归毒丸；(4) 关闭周期序数可从关闭记录自身推出——今天即 `CustomsCaseClosure.Reopenings()` 的条数加一。

第四条前提点的是第二条决定的地基：UC-CC-010 的多周期若实现成一案多条关闭记录，而新记录从零条重开起算，该推法退回 1、撞 ID 原样复活。改成那个形状时必须同时给出新的序数来源，否则重裁。

**二、关闭信封身份携关闭周期维，分区键收窄到案件。** `caseClosureEventID = tenant/caseRef/关闭周期序数`（首次关闭为 1，重开后再次关闭递增；序数即「使当次关闭期成立的决定」的序号，由 `Reopenings()` 条数加一推出），`PartitionKey = tenant/caseRef`——同案各关闭周期同分区先后保序。今天应用层没有重开入口，序数恒为 1，改动无行为差异；它拆掉的是 UC-CC-010 多周期落地那天 C2 与 C1 同 ID 被 `EnqueueOnce` 静默吞掉的引信（与 POD 更正[票 02](../../.scratch/outbox-partition-key/issues/02-pod-correction-is-silently-swallowed-by-enqueue-once.md) 同构）。

序数由适配器从意图里的关闭记录推出，不经 `CaseClosureHandoffIntent` 注入。注入要加宽意图契约，而加宽之后今天没有任何调用方给得出 1 以外的值——那只是把常量挪进编排，引信照留。

**三、信封上的案件维引用统一用铸造 `CustomsCaseID`。** 五维范围键的职责收敛为**建案幂等**（「同一法律行为一案」），建立口 eventID 维持五维键不变；除此之外任何口要携带案件维（Subject、载荷、将来可能的分区维），一律用铸造 ID，不得再用范围键充当案件引用。建立口信封 Subject 已带铸造 ID，即现成形状。

**四、申报口的案件维是建模欠账，不在本记录内解决。** 申报单元 → 案件的关联今天在域模型里缺席（文档「一个案件可以关联多个申报单元」只活在文档），另票排期；关联落地后申报意图在载荷/Subject 补案件引用（按第三条用铸造 ID），**不进分区键**。申报信封 ID 缺版本维一事继续归 `declaration-envelope-version-dedup` 票，两票互不吸收。

## Consequences

- 门禁例外清单三行处置：建立行、申报行由「待裁」改写为「无先后」批注并引本记录；关闭行随第二条的修复删除（ID 与分区键不再同源，门禁自然放行）。
- 关闭口修复与已落地的乙类同形（ID 加维＋分区键收窄到业务主体）；系统无生产数据，ID 公式变更无迁移负担。
- 消费侧「不可见＝可重试」的分界从代码注释升格为本记录的成立前提；将来 CC 新增消费者时必须沿用，否则触发重裁。
- 第四条前提把「多周期怎么实现」与「序数从哪来」绑在一起。实现多周期的那张票必须同时答后者，答不出就回到本记录重裁，而不是让序数悄悄退回 1。

## Alternatives considered

- **四口统一案件分区（全链保序）。** 否决：申报口今天连案件维都取不出（关联缺席），建立口换键公式会丢掉建案幂等；换来的顺序保证在重读模型下没有任何现有或已规划消费者需要，而真正的业务不变量（版本不丢）它一条也没多保。
- **关闭 ID 等 UC-CC-010 多周期实现时再改。** 否决：引信与拆弹分离，落地那天大概率忘——POD 更正被静默吞正是这样被发现的。
- **序数由 `CaseClosureHandoffIntent` 注入。** 否决：意图今天只有租户与关闭记录两个字段，加宽契约换来的是一个没有调用方能给出非 1 值的参数——形式上「可配置」，实质上是把常量 1 从适配器挪进编排，正是上一条否决的那种分离的缩小版。
- **案件维统一取五维范围键。** 否决：范围键是建案时的唯一性约束，不是引用身份；关闭口与建立口 Subject 已在用铸造 ID，反向统一改动更大且语义更差。

## Links

- [UC-CC-010](../application/customs-compliance/UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md)：多关闭周期（`AT-CC-327`/`AT-CC-337`）
- [ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)、[ADR-0065](./0065-projection-versions-are-append-only-and-supersession-is-source-given.md)、[ADR-0066](./0066-multi-object-envelope-unrolls-per-member-on-the-consumer-side.md)：消费模型三前提的出处
- [ADR-0043](./0043-publish-intent-claimed-by-result-identity.md)：发布意图由结果标识认领——关闭周期维进 ID 正是「结果标识」在多周期下的形状
- 裁断输入：[.scratch/outbox-partition-key/evidence-four-undecided.md](../../.scratch/outbox-partition-key/evidence-four-undecided.md)
