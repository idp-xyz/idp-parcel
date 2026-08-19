# ADR-0065: 追踪投影版本只增不改写；替代关系由源上下文给出，不进冲突裁决

Status: Accepted  
Date: 2026-08-19

## Context

[UC-VE-002](../application/visibility-exception/UC-VE-002-BUILD-TRACKING-PROJECTION.md) 的 `AT-VE-044` 要求「迟到/更正事实到达 → 追加投影版本，原版本保留」。勘察（[更正与迟到链报告](../../.scratch/ve-correction-late-chain/report.md)）查明两件事。

**一、追加那半边已在，保留那半边不成立。** `TrackingProjection.Rederive` 换版本并由 `PriorVersion()` 指回前身，编排每收一份新键事实都会走到它。但 `vepostgres.Projections.Save` 是按（租户 + 包裹）整行 UPSERT，迁移 `0007_tracking_projection.sql` 的主键也只有这两维。`prior_version` 是一列没有外键的裸文本，而被它指的那一行已被同一句 `DO UPDATE` 覆盖——指针在，靶子不在。`ports.ProjectionStore` 只有 `FindCurrent` 与 `Save`，没有按版本读回的口。客户视图同形，`CustomerTrackingView` 的注释已自陈「库里只有当前版本」。

**二、更正已经在发，VE 收得到却装不下。** `transport-fulfillment` 有两个更正入口——`RegisterTransportHandoverHandler.Correct` 与 `RegisterEffectiveDeliveryHandler.Correct`。两者都形成新版本并回指前身（`TransportHandover.Corrects()`、`EffectiveDelivery.Corrects()`），各自的持久化都有 `corrects_version` 与 `corrected_at` 两列且 `FindByKey` 读得回，两类信封的 ID 都含结果版本因而两代各自入队，两条链今天都登记在 `wireDispatcher` 的路由表里。缺口在消费侧：`vedomain.AcceptedSourceFactSpec` 只有来源、包裹、事实引用、事实类型、来源版本与三个时间共八格，没有一格装得下「本份取代哪一份」，两个消费适配器因而从不调用 `Corrects()`。更正维度是在跨上下文翻译那一步丢掉的，不是提供方没给。

还有一条使「靠冲突裁决顺带解决」走不通：`ResolveByBusinessTime` 是 `ConflictResolutionBasis` 三格中唯一有产生函数的一格，它按业务发生时间排序、同刻即判无法裁决；而 `TransportHandover.Correct` 刻意沿用原判断的 `judgedAt`，两代业务时间恒等。就算把它接进派生编排，一份更正也必然落在无法裁决。

两件事都是难逆转取舍：持久化形状一旦定型，所有读口随之定型；替代走裁决还是走独立判断，决定的是事实有效性的决定权落在哪一侧。

## Decision

**一、投影版本只增不改写。** 重新派生形成新的当前投影版本，原版本连同其条目、所用映射版本和派生时间一并留存，并可按版本读回。当前版由显式标记指名，读当前版不得退化为对历史的扫描。列名、索引与当前版的标记形状留给实现票；本记录只锁「不得覆盖」与「可按版本读回」两条。

**二、原版本的留存以存下来为准，不以可重放为准。** 重放证明的是「今天会派生出什么」，证明不了「当时形成过什么」——归类代码、映射目录的适用性与事实到达面都会漂。[ADR-0005](./0005-source-facts-effective-events-derived-state.md) 已经把「只保存最终状态」列为否决替代，理由正是「无法审计状态为何形成，也不能可靠重新判断」。

**三、来源事实替代关系由源上下文给出，本上下文只登记不裁决。** 关系只在同一源上下文、同一事实引用的版本之间成立。VE 不从业务时间、到达先后或任何其他线索推断谁更正了谁；跨源上下文或跨事实引用的分歧仍是冲突。

**四、替代不进 `ConflictResolutionBasis`，不增 `SUPERSESSION` 格。** 替代走与冲突裁决平行的独立判断。冲突裁决回答「VE 依据哪一维给多份有效事实排序」，替代回答「源上下文已经说了哪一份取代哪一份」——两者的决定权不在同一侧，合进一个封闭集合会把后者读成前者。

**五、「已被替代」是派生问答，不是可变标记。** 一份已接受事实处于已被替代，当且仅当另一份已接受事实指名它为前身。因此替代先于被替代到达不需要等待态，也不需要事后回填。

**六、替代链分叉进冲突。** 同一前身被两份以上事实指名时 VE 不择一：保留各方与关系，投影保持信息待确认并形成适用异常信号。这是两套机制唯一的接缝。

**七、被替代条目留在版本内但不参与派生。** 标准追踪里程碑、各追踪维度与追踪摘要只由当前有效即未被替代的条目派生。保留是为可追溯，不参与派生是为不同时呈现两个互斥结果。

**八、无后继的撤销或失效不预先建模。** 今天没有任何源上下文产出它：`node-operations` 全上下文没有任何更正、替代或撤销符号；`network-routing` 的 `PlanApplicability.Supersede` 与 `customs-compliance` 的 `ComplianceJudgment.Supersede` 在领域里存在但生产代码无调用方。UC-VE-002 的输入表如实记为未决，不预先给形状。

## Consequences

- `ports.ProjectionStore` 要长出按版本读回的口，`FindCurrent` 的语义从「读那一行」变成「读当前标记指名的那一版」。归实现票。
- 存储按包裹的事实数平方增长：第 k 版存 k 条条目。条目只含引用与映射版本、不复制源事实内容，单行因而小；保留期限与归档属实例半边（见[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)），本记录不设默认值。
- `AcceptedSourceFactSpec` 要增一维承载前身引用，`visibility_exception.accepted_fact` 随之加列，两个 `transport-fulfillment` 消费适配器把 `Corrects()` 译进去。第三个源接上来时依同一条不变量，不各写一份。
- `ResolveByBusinessTime` 与 `RaiseConflictSignal` 今天只被 `fact_conflict_test.go` 调用，派生编排里没有调用方。本记录不接它们，只钉住替代不从这条路走；把冲突裁决接进编排是另一张票。
- `Projections` 的「库只管当前版，重派生改同一行，历史由 prior_version 指回」、`RehydrateTrackingProjection` 的「库只管当前版，历史由 prior 指回」与迁移 `0007_tracking_projection.sql` 的同义句，描述与它们自己造成的效果不符——被指回的那一行已被覆盖。随实现票一并改，本记录不动代码。
- 没有为替代新立验收编号：`AT-VE` 现有 001 至 170 连续无空位，而全仓没有字母后缀的先例，自造一个等于单方面新设编号约定。替代的两支分别并进 `AT-VE-044`（追加与留存）与 `AT-VE-043`（分叉按冲突）。
- 客户视图那一层不变：CONTEXT 既有硬句「已经向客户发布的里程碑、ETA、异常说明或通知出现更正时，必须以追加更新或明确替代关系表达」原样适用，本记录只补内部投影这一层。

## Alternatives considered

- **保留 UPSERT，改 `AT-VE-044` 口径为「事实可重放重派生，不承诺历史投影可读」。** 否决：要改的那一头错了。三处文档硬句要的都是不删除原结果——CONTEXT「原来源、原映射、原投影判断和已经发布的客户信息必须保留关系」、UC-VE-002 输入表「追加后重新派生，不删除原结果」、UC-VE-002 规则「投影规则版本变化不得删除原映射、原判断或已发布客户信息」。为实现捷径收窄口径，是把三处硬句一起改小。
- **只存版本元数据（版本号、派生时间、所用映射版本、事实集指纹），条目按 `accepted_fact` 重算。** 否决：比今天的裸重放忠实，但仍然只证明「用今天的归类代码重算会得到什么」。归类逻辑一改，历史投影跟着变，审计问的那句话仍然答不了。
- **历史进聚合切片，照 [ADR-0045](./0045-new-submission-version-keeps-history-and-reestablishes-the-task.md) 的 `priorVersions`。** 否决：ADR-0045 的对象是委托提交版本，由人重新提交产生因而有界，且每一版都要过重建门；投影版本由事实到达产生，无界且机器驱动，把全史挂在聚合值上会让每次读当前投影都拖着它。ADR-0045 自己把「聚合外另存历史（仓储层版本表）」记为「等真实仓储落地再议归属，属可再议项而非本记录的锁定项」——VE 侧的真实仓储已经落地，这里就是再议它的地方。
- **给 `ConflictResolutionBasis` 增 `SUPERSESSION` 格。** 否决：该封闭集合的成员由 CONTEXT 硬句「依据事实类型、对象身份、控制范围、业务发生时间、有效时间、因果顺序和源上下文权威范围形成版本化投影判断」钉住，替代不在这七维里。更要紧的是，把替代当成一种裁决依据等于由 VE 判定前一版不再有效，正撞「本上下文不得自行使源事实失效」与 UC-VE-002 元数据里的「源上下文拥有事实有效性」。
- **在适配器里凭 `Corrects()` 直接决定哪一版当前有效，不入领域语言。** 否决：那是把一条领域不变量藏进翻译层，两个 `transport-fulfillment` 适配器各写一份口径，第三个源接上来时无处可依。

## Links

- [ADR-0005](./0005-source-facts-effective-events-derived-state.md)：来源事实、有效事件与派生状态的分离；「只保存最终状态」在那里已被否决
- [ADR-0038](./0038-validity-correction-keeps-original-and-relationship.md)：有效性更正保留原版本与更正关系、原版本继续可得；本记录把同一取舍用在追踪投影上
- [ADR-0045](./0045-new-submission-version-keeps-history-and-reestablishes-the-task.md)：当前版本单指针加历史追加；其留作可再议的「仓储层版本表」由本记录在 VE 侧作答
- [UC-VE-002](../application/visibility-exception/UC-VE-002-BUILD-TRACKING-PROJECTION.md)：`AT-VE-043` 与 `AT-VE-044`
- [全程追踪与异常上下文](../domain/visibility-exception/CONTEXT.md)：「来源事实替代关系」词条与追踪投影各条不变量
