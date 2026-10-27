# ADR-0292：业务时间早于起点计划生效时刻、本会失效或被改善替代的复核触发入账为「触发早于计划生效」、不判——复核的第三道业务时间下限，在判出之后、离场与换班之前比，判出仍适用不拦

Status: Accepted（2026-10-26，Claude Code 会话代 NR owner 决：用户授权协调方「你作为业务和系统专家自决」，协调方分诊 R2、采票面倾向的甲（这次触发入账为未判），并裁定原因名照 NR 既有未判原因构词、新未判原因不进续办触发；票 [docs-consistency/code/100](../../.scratch/docs-consistency/code/100-a-control-trigger-between-a-suggestion-and-its-decision-reviews-a-plan-not-yet-effective.md)，出处为 [ADR-0284](./0284-an-authorized-role-decides-a-current-reroute-suggestion-through-an-operator-decision-face.md) 越权风险点 6 与 [ADR-0309](./0309-an-authorized-role-decides-an-initial-route-lowest-cost-tie-through-the-operator-decision-face-forming-the-first-plan-from-the-decision-moment.md) 越权风险点 5。部分停用（扩成）[ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 决定一一句，被停原句与适用场景见「对已接受记录的作用」；ADR-0235 正文不改，只在其 Status 行与 Links 加前向指针。越权风险点见文末，待 NR owner 复核。）<!-- 索引注记 -->｜**下限**：触发的业务时间严格早于起点计划的生效时刻，且判出失效或改善替代时，复核答`未判`，原因「触发早于计划生效」（`TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM`），未判登记另记比较所用的生效时刻（network_routing 0031）；不调离场门，原计划的适用性不动，不失效、不替代、不形成新计划；同一时刻照判。**仍适用不拦**：判出仍适用（含判不出改善）不离场也不形成新计划，不撞门，照常记。**次序**：已有结果分界、委托接受、当前位置之后，读起点、取证据、判出结论，再在失效离场之前、改善换班之前比计划生效；适用性看不清与冻结形态未配置先答。**终局**：业务时间与生效时刻都不会变，重投推不动；恢复动作是等下一次控制触发从它的位置判，不等登记，不进续办（票 demo-chain-gaps/154）。**范围**：判据不问计划是谁形成的——授权改路决定（ADR-0284）与授权裁并列的首个计划（ADR-0309）同落此格。**否决**：决定推当前控制位置、对着业务时间那一刻的状态判、夹离场到生效时刻、起点一读出就拦、决定编排看在途信封、回滚重投｜**回答**：ADR-0284 越权风险点 6 与 ADR-0309 越权风险点 5；无条款停用
Date: 2026-10-26

## Context

- 复核形成的计划从所判触发的业务时间起生效，原计划失效与被替代都在那一刻离场（[ADR-0272](./0272-a-plan-formed-by-route-reassessment-takes-effect-at-the-trigger-business-time.md)）。领域的离场门要求离场不早于生效（`PlanApplicability.leaveCurrent`，不满足答 `ErrInvalidPlanApplicability`）。
- 让离场门不因业务时间而拒，靠两道下限：触发不早于当前控制位置（ADR-0235 决定四），不早于这一判断键的委托接受时刻（[ADR-0279](./0279-a-reassessment-trigger-earlier-than-the-shipment-acceptance-is-settled-as-not-judged.md)）。两道的论证有同一个前提：路由历史里每份计划的生效时刻，要么是某次已判触发的业务时间（复核形成的计划），要么是委托接受（自动形成的初始计划）。
- 另有两种计划不是这样生效的：
  - 授权角色据复核建议形成的改路决定，新计划从决定的时刻起生效，决定不推当前控制位置（[ADR-0284](./0284-an-authorized-role-decides-a-current-reroute-suggestion-through-an-operator-decision-face.md) 决定四）；
  - 授权角色裁初始路由最低成本并列，首个计划同样从决定的时刻起生效（[ADR-0309](./0309-an-authorized-role-decides-an-initial-route-lowest-cost-tie-through-the-operator-decision-face-forming-the-first-plan-from-the-decision-moment.md) 决定三）。
- 决定的时刻不是任何一次触发的业务时间，于是有一个窗口：
  - 原计划在 T1 失效、复核形成建议（或初始路由在 T0 并列未形成），授权角色在 T3 决定，新计划从 T3 起生效；
  - 窗口里（T1 或 T0 到 T3 之间的 T2）业务发生的控制触发——包裹在无当前有效路由期间仍被交进下一个节点，节点写口对已经发生的作业照记（[ADR-0251](./0251-directional-node-work-is-paused-outside-the-validity-interval-of-the-delivered-basis-and-recorded-moves-are-never-refused.md)）——若在 T3 之后才处理，它不早于当前控制位置（决定不推它），也不早于委托接受，两道下限都放行；
  - 复核的起点却是从 T3 才生效的新计划。
- 判出失效，失效在触发的业务时间 T2 离场；判出改善替代，原计划在 T2 离场、接班计划也取 T2 生效。两者都早于 T3，离场门拒。复核编排把离场门的拒法当编程错误响亮报错、不折未决，整封信封响亮失败，消费门回滚重投到 `ABANDONED`：
  - 交接信封按（租户，载运对象，类型段）分区，同一对象的交接信封占着分区头，同一封信上的追踪投影与派送发起、之后的每一封交接信封都被推迟（[ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md) 越权风险点 10）；
  - 收寄采用一路占着小包托运的分区，同一封信上排在后面的计费那一路同样被拖住。
- 判出仍适用的不撞：不离场，也不形成新计划。
- 决定编排判过时只看在册的复核（`SuggestionOvertaken`）；还在派发管道里、业务时间晚于建议的触发它看不见，所以这个窗口不是决定能自己关上的。
- 代码（取证于 main `c8f0f3522`，本记录之前）：复核编排失效线 `Lapse(basis, trigger.OccurredAt())`，改善替代 `Supersede(..., trigger.OccurredAt())` 且接班计划 `EffectiveFrom` 取 `trigger.OccurredAt()`；真库夹具（决定编排接在揽收复核与交接复核之间）复现：揽收失效并形成建议，决定后一次判断时刻在两者之间的交接登记处理，整封信封以 `lapse plan applicability: network routing: invalid plan applicability` 失败。

## Decision

**一、第三道下限：触发严格早于起点计划的生效时刻、且判出失效或改善替代时，答`未判`，原因「触发早于计划生效」（`TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM`）。**

- 起点计划的生效时刻即这份计划在适用性里「当前有效」的那一刻：适用性有登记行取其转移时刻，没有则取计划的生效时刻。它与离场门比较的是同一个值；比较也是同一条（`PlanApplicability.NotYetEffectiveAt`）——门拒的正是这一格，编排在门之前分流，两处不会各认各的。
- 未判登记同一行另记比较所用的计划生效时刻（network_routing 0031 `plan_effective_at`），审计答得出「早于谁」，口径同 ADR-0279 对委托接受、ADR-0235 对当前位置。三格各记各的列，不合用。
- 不动计划：不调离场门，起点计划的适用性不动，不失效、不替代、不形成新计划，也不为这一次另写复核。
- 同一时刻不算更早，照判。这时离场与生效落在同一刻，离场门放行。
- 终局入账。业务时间与生效时刻都不会变，重投推不动；也不等登记，不进续办（票 demo-chain-gaps/154 的续办是「登记之后重判」）。这一格的恢复动作是等下一次控制触发从它自己的位置判，与等某个登记不是同一类。

**二、判出仍适用不拦；这一道放在判出之后、离场与换班之前。**

- 仍适用不离场、不形成新计划，不撞离场门。这样的复核照常记引用起点计划的`仍适用`：位置在计划上这个事实不该因为计划生效得晚而丢掉。
- 仍适用线上判不出改善的各格同属仍适用，照常记：策略版本没声明自动改路、成本读不回、改善够不上阈值、已越过冻结边界、场外揽收不试改善。
- 失效线：失效依据判出之后、离场之前比。
- 改善替代线：改善够格、要换班的那一步之前比。先判出改善才知道这是换班而不是仍适用，所以成本照取、候选照排。
- 原计划适用性看不清（`PLAN_APPLICABILITY_UNCLEAR`）与冻结形态未配置（`FREEZE_FORM_UNCONFIGURED`）先于本格答：它们不是业务时间的问题，判得出的先答。
- 合起来的次序：已有结果分界 → 委托接受（ADR-0279）→ 当前位置（ADR-0235）→ 读起点、取证据、判 → 计划生效。前两道不读路由历史，在读之前比；这一道要起点和判出的结论，只能在判出之后比。

**三、范围不按计划来源分。**

- 判据是「起点计划在触发的业务时间还没生效」，不问计划是谁形成的。授权改路决定形成的计划（ADR-0284）与授权裁并列形成的首个计划（ADR-0309）是今天的两个来处；将来再有生效晚于可被触发的业务时间的计划，同样落在这一格。
- 自动形成的初始计划（从委托接受起生效）与复核形成的计划（从触发业务时间起生效）不会撞：前者被委托接受下限盖住，后者不晚于当前控制位置。本记录不改它们。

**四、两路消费侧适配器不变。**

- `未判`入账是两路适配器对「重投推不动」的既有折法（ADR-0235 决定一），新原因自动落在那一格；适配器不持有任何原因字面量。
- 未决哨兵名单不变：本格不是未决，不进重投。

## 候选与反方

- **乙：决定推当前控制位置。** 否决：要改 ADR-0235 决定四「当前位置」的定义（路由历史里已判复核的最晚业务时间），也要撤 ADR-0284 决定四「不推当前控制位置」；T2 那次触发按「触发早于当前位置」入账，结局与本记录相同，还把一个非控制事实（授权角色的决定）塞进当前位置，此后真正落在 T1 与 T3 之间的触发都被它判成过时。
- **丙：这次触发对着它业务时间那一刻的状态判，即 T2 时无当前有效路由，走无路由那条线。** 否决：可能另形成一份从 T2 起生效的首个计划，与决定形成的计划并存，「同一业务时点只能有一个当前有效计划」要另裁谁让谁；复核起点读的是适用性现状，不按业务时点回放。
- **把离场夹到生效时刻，或把接班计划的生效夹到起点计划的生效。** 否决：ADR-0272、ADR-0279 已否决同形候选；失效时刻就不再是依据到来的业务时间。
- **起点一读出就拦（仍适用也入账为未判）。** 否决：见决定二，仍适用不撞门，拦它是让一次完全判得出的复核白白不判。
- **决定编排把在途触发也算进「过时」。** 否决：决定编排读不到派发管道里的信封，要读就得跨上下文读 TF、PS 的发件箱；「在途」没有边界，决定与触发谁先谁后在派发下本来就不定。
- **决定时把生效时刻回溯到建议所在触发的业务时间。** 否决：ADR-0284 决定四、ADR-0309 决定三已否决，抹掉无当前有效路由那一段。
- **回滚重投。** 否决：同 ADR-0235、ADR-0279 的同名反方，业务时间与生效时刻不会变，重投永远推不动，只把分区烧到 `ABANDONED`。

## 对已接受记录的作用

- **部分停用（扩成）** ADR-0235 决定一「重投推不动的答`未判`（`ReassessTriggerNotJudged`）：没有路由历史、证据未配置、冻结形态未配置。原因归 `ports.NotJudgedReason`。」一句（该句已被 [ADR-0250](./0250-unclear-plan-applicability-in-reassessment-is-settled-as-not-judged-and-resume-in-the-same-change-stays-in-the-acceptance-chain.md) 与 ADR-0279 各扩过一格）：`未判`的原因封闭集再加一格「触发早于计划生效」（`TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM`，决定一）。**适用场景**：NR 路由复核的收寄采用与交接两路，起点计划在触发的业务时间还没生效，且判出失效或改善替代；同句其余各格不变。
- ADR-0235 决定四：无条款停用。当前位置的定义、同一时刻照判、交接更正与判断键串行不变；本记录的比较在它之后。
- ADR-0272：无条款停用。决定三「复核能判的下一次触发都不早于当前控制位置，也就不早于这一次形成的计划的生效时刻」对复核形成的计划仍成立；对决定形成的计划不成立（ADR-0284 已记），这一头由本记录承接。
- ADR-0279：无条款停用。决定三末句「有了两道下限，复核能判的触发不早于路由历史里任何一份计划的生效时刻，离场门不再因业务时间而拒」对决定形成的计划不成立（那两种计划在它之后才有），第三道下限补上；决定三「下限取判断键的接受时刻，不取当前计划的生效时刻」的取舍不变，本记录不替代它——无路由那条线上没有计划，仍要接受时刻。
- ADR-0284：无条款停用。决定四「不推当前控制位置」不变（乙被否决）；越权风险点 6 由本记录回答。
- ADR-0309：无条款停用。越权风险点 5（委托接受到决定之间业务发生、决定之后才处理的控制触发）由本记录回答；决定三的生效时刻不变。
- ADR-0250：无条款停用。「终局入账、不进续办」与它决定二对续办纪律的读法一致。
- ADR-0251：无条款停用。节点写口对已经发生的作业照记不变；本记录只管复核怎样对待这些事实的业务时间。
- ADR-0215：无条款停用。越权风险点 10 说的是重投期间占着分区；这一格不重投，不再占。

## Consequences

- 代码随票 docs-consistency/code/100：
  - 领域 `PlanApplicability.NotYetEffectiveAt`，离场门（`leaveCurrent`）也用它比；
  - 端口 `NotJudgedTriggerPrecedesPlanEffectiveFrom`、`NotJudgedTrigger.PlanEffectiveAt`；
  - 复核编排在失效离场之前、改善换班之前分流；
  - 未判登记适配器核对「生效时刻与这一格原因同来同去」。
- 迁移 network_routing 0031：未判登记原因封闭集加一格，另加 `plan_effective_at`，只在这一格原因下有值、触发不晚于它。比较用 `<=`：编排按纳秒比，落库两格都只留到微秒（理由同 0024）。
- 未判原因的消费方逐一核过（ADR-0025）：两路消费适配器按结论折、不看原因，`未判`入账；未判登记写者是唯一读原因的一层，自带校验；未判登记今天没有读面（票 demo-chain-gaps/154），管理台没有未判原因的标签表，HTTP 不暴露——新原因没有别的落点。
- 文档：NR CONTEXT「路由稳定性与受控改路」复核触发排序那条补一句；UC-NR-003 结果`触发未判`补这一格，验收补 `AT-NR-104`、`AT-NR-105`，「给开发的交接」撤掉对应一行。

## 越权风险点

1. **交进节点的那一次不判，要等下一次控制触发再判**（NR owner）。窗口里的触发可能带着真实的失效依据，例如包裹被交进了不在决定形成的计划上的节点。入账不判之后，决定形成的计划照旧当前有效，直到下一次控制触发从它自己的位置再判。之后若再没有控制触发（最后一个节点），这次失效就不会被判出，与 ADR-0279 越权风险点 1 同形。owner 若要这次也判，要的是「从生效时刻的位置补判一次」那种续办，不是夹离场（见候选与反方）；它不是票 [demo-chain-gaps/154](../../.scratch/demo-chain-gaps/issues/154-network-reassessment-not-judged-triggers-resume-after-registration.md) 的「登记之后重判」，本记录的新原因不进 154 的续办触发（决定一）。
2. **窗口长度取决于授权角色多久决定**（NR、运营 owner）。无当前有效路由期间包裹仍会被交进下一个节点（节点写口照记），建议放几天，这几天里到的每一次交接都落进窗口。票面判为 R2；若某租户的运营常态是先交接、后决定，大量复核会入账不判，升 R1。缓解：未判登记按原因、按包裹列得出。
3. **改善替代线上，被拦的那一次照取了成本、照排了候选**（NR owner）。先判出改善才知道这是换班，所以成本取数与取号都发生在拦之前；取号的号段因此留一个缺口。仍适用线上判不出改善的复核也取号，缺口不是新现象。
4. **业务时间与生效时刻的来源不同**（NR、TF owner）。触发的业务时间取自来源（交接的判断时刻、收寄的业务时间），决定的时刻取 NR 决定编排的时钟；来源与 NR 的时钟偏差会挪动这条边界。同 ADR-0279 越权风险点 4、ADR-0235 越权风险点 10。
5. **决定编排的「过时」仍只看在册的复核**（NR owner）。一次业务时间晚于建议、还在派发管道里的触发，决定时看不见，决定照成；它此后处理，业务时间早于 T3 的被本记录拦下，不早于 T3 的照判。决定的过时核对不改。
6. **判出仍适用时，起点计划还没生效这个事实不留痕**（NR owner）。仍适用的复核照常落册，复核记录带所判触发的业务时间，审计可自行与计划的生效时刻比；未判登记只在入账不判时记生效时刻。

## Links

- [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md)：前驱——部分停用（扩成）决定一「重投推不动的答`未判`……」一句，见「对已接受记录的作用」；当前控制位置与判断键锁
- [ADR-0279](./0279-a-reassessment-trigger-earlier-than-the-shipment-acceptance-is-settled-as-not-judged.md)：同族先例——委托接受下限，终局入账、不进续办，次序在它之后
- [ADR-0272](./0272-a-plan-formed-by-route-reassessment-takes-effect-at-the-trigger-business-time.md)：复核形成的计划从触发业务时间起生效、离场取同一刻；夹离场的同形候选已被它否决
- [ADR-0284](./0284-an-authorized-role-decides-a-current-reroute-suggestion-through-an-operator-decision-face.md)：计划从决定的时刻起生效、不推当前控制位置；越权风险点 6 由本记录回答
- [ADR-0309](./0309-an-authorized-role-decides-an-initial-route-lowest-cost-tie-through-the-operator-decision-face-forming-the-first-plan-from-the-decision-moment.md)：首个计划从决定的时刻起生效；越权风险点 5 由本记录回答
- [ADR-0250](./0250-unclear-plan-applicability-in-reassessment-is-settled-as-not-judged-and-resume-in-the-same-change-stays-in-the-acceptance-chain.md)：`未判`先例与续办纪律
- [ADR-0251](./0251-directional-node-work-is-paused-outside-the-validity-interval-of-the-delivered-basis-and-recorded-moves-are-never-refused.md)：节点写口对已经发生的作业照记
- [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md)：交接触发复核；越权风险点 10 的占分区后果
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)、[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：消费方逐一落点、按恢复动作分格
- [network-routing CONTEXT](../domain/network-routing/CONTEXT.md)「路由计划适用性」「路由稳定性与受控改路」、[UC-NR-003](../application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md)（`触发未判`、`AT-NR-104`、`AT-NR-105`）
- 票 `.scratch/docs-consistency/code/100-a-control-trigger-between-a-suggestion-and-its-decision-reviews-a-plan-not-yet-effective.md`；续办票 `.scratch/demo-chain-gaps/issues/154-network-reassessment-not-judged-triggers-resume-after-registration.md`
