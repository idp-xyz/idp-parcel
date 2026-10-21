# ADR-0245：替代链分叉不按业务时间或任何其他维度裁决——分叉各方随投影版本留存、不参与派生，那一条事实流保持信息待确认，并形成适用异常信号

Status: Accepted（2026-10-21，Claude Code 会话代 VE owner 决（代决）：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/04，审计条目 [audit/ve.md](../../.scratch/docs-consistency/audit/ve.md) VE-04（归 B）。给 [ADR-0065](./0065-projection-versions-are-append-only-and-supersession-is-source-given.md) 决定六作读法注，不停用任何已接受记录的条款；ADR-0065 正文不改，只在其 Status 行与 Links 加前向指针。要改的代码另立票 docs-consistency/code/30。越权风险点见文末，待 VE owner 复核。）<!-- 索引注记 -->｜**判据**：替代链分叉是源上下文在同一事实引用下给出了两份以上都指名同一前身的更正；在两个后继之间选出当前那一份，就是判断「谁更正了谁」，ADR-0065 决定三不许本上下文从业务时间、到达先后或任何其他线索推断。业务发生时间是被更正事实的发生时间，不是更正的先后。所以分叉一律无法裁决，不进 `ResolveByBusinessTime` 或任何裁决函数；冲突裁决的各维度只用于跨源上下文、跨事实引用的冲突。**呈现**：前身与各后继都留在投影版本内并标明分叉关系，都不参与派生；那一条事实流（同一源上下文、同一事实引用）在投影上答信息待确认，同包裹其他事实流照常派生；客户视图的相应维答待确认。**留存**：分叉关系随投影版本存下，不靠重放、不另立冲突关系库；信号规则未配置时不形成信号，分叉照样留在版本里。**解开**：今天的源上下文更正形状只能指名一个前身，分叉没有在源侧解开的办法，待确认会一直持续，靠信号进分诊、由案件向源上下文发处置请求。

Date: 2026-10-21

## Context

取证于 main `205eb254`，只记 `S`。

- CONTEXT「追踪投影与里程碑」写的是：「同一份已接受事实被两份以上事实同时指名为前身时，替代链分叉，本上下文不得择一。分叉按冲突处理：保留各项事实及其关系，投影保持信息待确认并形成适用异常信号。」ADR-0065 决定六写的是：「替代链分叉进冲突。同一前身被两份以上事实指名时 VE 不择一：保留各方与关系，投影保持信息待确认并形成适用异常信号。」
- 代码按另一种读法做：
  - `application.DeriveProjectionHandler.judgeForks` 对每处分叉调 `domain.ResolveByBusinessTime`。两个后继的业务发生时间不同，`judgment.Resolved()` 就为真，然后 `continue`：不形成信号，也不进本轮结果。测试 `TestAForkResolvableByBusinessTimeRaisesNoSignal` 把这一点钉住了（「业务时间排得出先后的分叉不是异常」）。
  - 裁出来的顺序没有落在任何地方：两个后继照常归类、照常派生里程碑，投影上同时有两个互斥的结果，也没有答「信息待确认」。
  - 分叉只在当前有效集里找（`domain.SupersessionForks(effective)`）。源上下文日后更正了其中一支，那一支的旧链头离开当前有效集，分叉就看不见了，可两个链头其实仍都当前有效。
  - 冲突关系（`application.FactConflict`）只随本轮结果返回，不持久化。冲突信号规则未配置时，什么都留不下。
- 按业务时间裁决分叉这一判法出自票 [mechanism-executor-triage/08](../../.scratch/mechanism-executor-triage/issues/08-ve-six-executors-behind-existing-uc-steps.md) 的 VE-d 组，不来自任何 ADR。那张票把 ADR-0065 Consequences 所说「把冲突裁决接进编排是另一张票」读成了「接进分叉」。
- 两种读法各有出处。CONTEXT 另一句「多个有效事实冲突时，依据事实类型、对象身份、控制范围、业务发生时间、有效时间、因果顺序和源上下文权威范围形成版本化投影判断」许用业务时间裁决冲突；ADR-0065 决定三「VE 不从业务时间、到达先后或任何其他线索推断谁更正了谁」不许。「分叉按冲突处理」既能读成「先走冲突裁决」，也能读成「按无法裁决的冲突处理」。

## Decision

**一、分叉不进冲突裁决，一律无法裁决。**（机制）

- 分叉的各后继，是同一源上下文对同一事实引用给出的更正，它们都指名同一前身。在它们之间选出当前那一份，回答的就是「后一份更正了前一份」——ADR-0065 决定三明禁的推断。
- 业务发生时间是被更正那件事的发生时间，不是更正的先后。一次交付先后被两份更正改成十一点和九点，说十一点的那份不因此更晚、更对。到达先后与之同理。因果顺序、权威范围在同一源上下文的同一条谱系里也给不出答案：两份更正的权威相同，谁也不是谁的原因。
- 所以 CONTEXT 那句冲突裁决只管**跨源上下文或跨事实引用**的冲突（ADR-0065 决定三后半「跨源上下文或跨事实引用的分歧仍是冲突」），不管分叉。ADR-0065 决定六的「进冲突」读作「进**无法裁决的**冲突」。
- `ResolveByBusinessTime` 与其他裁决函数不再对分叉调用。跨源、跨事实引用的冲突今天在派生编排里没有发现机制，本记录不立。

**二、投影怎么呈现。**（机制）

- 被分叉的前身与各后继都留在投影版本内，前身标为已被替代，各后继标为分叉待确认、指回前身。形状同 ADR-0065 决定七：被替代条目留在版本内但不参与派生，与[票 demo-chain-gaps/117](../../.scratch/demo-chain-gaps/issues/117-projection-entries-are-stored-without-their-supersession.md) 同一车道。
- 分叉以**链头**计：每个后继沿替代链走到当前有效的那一版。同一（源上下文，事实引用）下当前有效的链头多于一份，分叉就未解，与哪一支被更正过几次无关。
- 分叉未解期间，那一条事实流的前身、各后继与各链头都不参与派生：它不贡献任何标准追踪里程碑与维度，投影在它所在的维度上答信息待确认。同一包裹的其他事实流照常派生。
- 客户全程追踪视图照 CONTEXT「某一维信息待确认或不可披露 → 只对该维返回待确认」与 UC-VE-008 `AT-VE-163`：受影响那一维答待确认，其余可靠维照常展示。

**三、分叉关系随投影版本存下。**（机制）

- CONTEXT「必须保留各项事实及冲突关系」的冲突关系，在分叉上就是（前身、各后继、各链头）。它写进每一版投影，随版本只增、可按版本读回，照 ADR-0065 决定二「以存下来为准，不以可重放为准」。不另立冲突关系库：事实库里每份后继的前身指名本来就在，投影版本记的是那一版派生时认定的分叉。
- 运营追踪查阅照 CONTEXT「面向投影的全部并行维度、冲突关系与被替代条目」读得到它。
- 适用异常信号照旧按冲突信号规则（`PAR-VIS-04`）形成、交分诊。规则未配置时如实不形成信号，但分叉照样留在投影版本里，运营看得见。信号那一步失败或搁置，照现有写法留给重放。

**四、分叉怎么解开。**（产品策略）

- 今天的源上下文更正只能指名一个前身。给其中一支再出更正，链头数不变，分叉在源侧没有解开的办法；无后继的撤销或失效 ADR-0065 决定八列为未决。
- 所以未解的分叉持续待确认，处理路径是信号进分诊、建案后由案件向源上下文发处置请求（CONTEXT「处置请求」），本上下文不替源上下文认定哪一支作废。
- 源上下文日后有了撤销一支、或一份更正同时指名几个前身的形状时，链头回到一份，下一版投影按单链派生，原版本照留。那一形状属源上下文与 ADR-0065 决定八，不在本记录。

三类：决定一至三是**机制**；决定四是**产品策略**（内置一种处理法：不自动解、走案件）；冲突信号的类型、规则版本与可信度是**租户取值**（`PAR-VIS-04`），演示租户以 `SYN-` 登记，证据只记 `S`。

## Consequences

- 代码票 [docs-consistency/code/30](../../.scratch/docs-consistency/code/30-forked-supersession-is-judged-by-business-time-and-not-kept-pending.md)：
  - `judgeForks` 不再调 `ResolveByBusinessTime`。分叉按链头从全量事实里找，不只在当前有效集里找。
  - 分叉各方进投影版本、带分叉标记、不参与派生；受影响的事实流答待确认。
  - 客户视图相应维答待确认（与 [code/06](../../.scratch/docs-consistency/code/06-customer-view-shows-dimensions-without-a-visible-judgement.md) 的里程碑维兜底同一处）。
  - 测试 `TestAForkResolvableByBusinessTimeRaisesNoSignal` 改为断言业务时间不同的分叉照样待确认并立信号；`TestAForkedSupersessionKeepsBothSuccessorsInTheProjection` 改为断言双方在场、不参与派生。
- `ResolveByBusinessTime` 在派生编排里重新没有调用方，留给跨源、跨事实引用冲突的发现机制那张票。领域头注「当前唯一产生函数」一句不受影响。
- 文档：VE CONTEXT「追踪投影与里程碑」分叉一条与 Lifecycles 分叉一行改写为本记录的判据；UC-VE-002 主流程第 5 步与 `AT-VE-043` 随之改。

## Alternatives considered

- **维持现状：业务时间排得出先后就不算异常。** 否决。理由见决定一：业务时间不是更正的先后。裁出来的顺序也没人用，两个后继照样都派生，投影上同时呈现两个互斥结果，正撞 ADR-0065 决定七「不参与派生是为不同时呈现两个互斥结果」。
- **按业务时间裁决，并让裁出的「较晚」一方当前有效。** 否决。这就是由 VE 判定另一支不再有效，正撞 CONTEXT「本上下文不得自行使源事实失效」与 UC-VE-002 元数据「源上下文拥有事实有效性」。
- **分叉时让前身继续派生，作为最后一份各方都认的版本。** 否决。两个后继都说前身是错的，展示它等于展示两方都已更正掉的内容。
- **另立冲突关系库，分叉与跨源冲突共用。** 不取。分叉关系从事实的前身指名就能完全读出，投影版本存下派生那一刻的认定已经够审计。跨源冲突的关系形状要等它的发现机制立起来时一并定，现在立是替一个还没有生产方的东西定形。

## 越权风险点

1. **代 VE owner 把分叉定为永远无法裁决。** 源上下文若有理由给两份更正各自的权威（例如一份来自人工复核、一份来自自动采集），本记录也不让 VE 据此择一。那要由源上下文自己给出替代关系，不归 VE。
2. **分叉在源侧解不开，待确认可能持续很久。** 今天交接、交付、终局各源的更正都只指名一个前身。分叉一旦出现，那一条事实流就一直不贡献里程碑，直到源上下文有撤销或多前身更正的形状。上线前若发现某个源会因并发更正频繁产生分叉，应在源侧修（只允许更正当前版），而不是在 VE 放宽。
3. **以链头计分叉，比今天的检测宽。** 今天源上下文更正了其中一支，分叉就消失；按本记录它仍未解。演示动线若依赖「再更正一次就恢复」，结果会变。
4. **客户视图那一维答待确认，依赖 code/06 的里程碑维兜底。** 两票落地次序不同时，中间态可能是投影答待确认、客户视图那一维仍按披露策略答展示。
5. **跨源、跨事实引用冲突仍没有发现机制。** CONTEXT 那句冲突裁决今天没有产生方，本记录也不补；VE owner 若要它，另立票。

## Links

- [ADR-0065](./0065-projection-versions-are-append-only-and-supersession-is-source-given.md)：决定三（不推断谁更正了谁）、决定六（分叉进冲突）、决定七（被替代条目留在版本内但不参与派生）、决定八（无后继的撤销未决）；本记录给决定六作读法注
- [ADR-0005](./0005-source-facts-effective-events-derived-state.md)：来源事实、有效事件与派生状态的分离
- [visibility-exception CONTEXT](../domain/visibility-exception/CONTEXT.md)：「追踪投影与里程碑」替代链分叉一条、冲突裁决两条；Lifecycles「全程追踪投影」
- [UC-VE-002](../application/visibility-exception/UC-VE-002-BUILD-TRACKING-PROJECTION.md)：主流程第 5 步、`AT-VE-043`
- [UC-VE-008](../application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md)：`AT-VE-163`
- [票 mechanism-executor-triage/08](../../.scratch/mechanism-executor-triage/issues/08-ve-six-executors-behind-existing-uc-steps.md)：VE-d 组，按业务时间裁决分叉的出处
- [票 demo-chain-gaps/117](../../.scratch/demo-chain-gaps/issues/117-projection-entries-are-stored-without-their-supersession.md)：被替代条目进投影版本，与决定二同一车道
- [docs-consistency/code/30](../../.scratch/docs-consistency/code/30-forked-supersession-is-judged-by-business-time-and-not-kept-pending.md)：实现票
