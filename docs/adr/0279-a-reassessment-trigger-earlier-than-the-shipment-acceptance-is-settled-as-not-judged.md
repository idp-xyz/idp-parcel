# ADR-0279：业务时间早于委托接受的复核触发入账为「触发早于委托接受」、不判——复核的业务时间下限取判断键的接受时刻，先于当前位置比

Status: Accepted（2026-10-21，Claude Code 会话代 NR owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」，常设授权「你作为业务和系统专家自决」；票 [docs-consistency/code/69](../../.scratch/docs-consistency/code/69-a-trigger-earlier-than-the-shipment-acceptance-is-rejected-by-the-leave-gate.md)，出处为票 [docs-consistency/code/60](../../.scratch/docs-consistency/code/60-initial-route-takes-effect-at-processing-time-unlike-reassessment.md) 完成记录越权风险点 1 与其隔离评审建议，裁向由协调方给出。部分停用（扩成）[ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 决定一一句与决定四一句，被停原句与适用场景见「对已接受记录的作用」；ADR-0235 正文不改，只在其 Status 行与 Links 加前向指针。越权风险点见文末，待 NR owner 复核，PS、TF owner 会签。）<!-- 索引注记 -->｜**下限**：路由复核的业务时间下限是这一判断键的委托接受时刻（接受基线固定的那一刻）；触发的业务时间严格早于它时答`未判`，原因「触发早于委托接受」（`TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE`），未判登记另记比较所用的接受时刻（network_routing 0024），不读路由历史、不判，不失效、不替代、不形成首个计划；同一时刻照判。**次序**：在已有结果分界之后、比当前位置之前先比接受。**来处**：接受时刻随触发带进复核，收寄采用与交接两路适配器各自从 PS 委托聚合的接受基线固定时刻读，与初始路由交接带来的同源。**否决**：离场夹到生效时刻、PS 采用时拒、下限取计划生效时刻、回滚重投｜**理由限定**（2026-10-25）：候选与反方「**在 PS 委托仓储或 PS 读口上加一法取接受时刻。**」一条的未采纳理由「NR 已有的 PS 读口（按包裹反查当前已接受委托再读聚合）就交得出它，不必动 PS。」已由 [ADR-0301](./0301-parcel-shipment-offers-an-acceptance-baseline-geo-projection-and-reassessment-no-longer-reads-the-whole-request.md) 限定——交接与收寄采用两路读的已不是那个读法，而是小包托运的受理基线地理投影读口，接受时刻随投影一并由它交出；未采纳的仍是只为接受时刻单独动小包托运。决定四的取值、同源与分格不变，无条款停用
Date: 2026-10-21

## Context

- 票 code/60 之后，初始路由计划从委托接受的时刻起生效，判断时刻另记处理时钟（[network-routing CONTEXT](../domain/network-routing/CONTEXT.md)「路由计划适用性」）。
- 复核形成的计划从所判触发的业务时间起生效，原计划失效与被替代都在触发的业务时间离场（[ADR-0272](./0272-a-plan-formed-by-route-reassessment-takes-effect-at-the-trigger-business-time.md)）。领域的离场门要求离场不早于生效。
- [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 决定四给复核排了业务时间：触发早于「当前位置」（这一判断键在册复核里业务时间最晚的那一次已判控制触发）时入账不判。当前位置只认路由历史里的已判复核，委托接受不在其中。
- 触发的业务时间可以早于委托接受，而信封在初始路由形成之后才处理：
  - 先揽后受理，或客户先送站、委托后受理；
  - 离线补登；
  - 来源把业务时间写早了。
- 这时复核判出失效或改善替代，离场早于生效，离场门拒，整封信封响亮失败，按派发预算重投到 `ABANDONED`，没有绕法。
- 原先无当前有效路由的那条线不经离场门，却会形成一份从触发业务时间起生效的首个计划，生效早于委托接受。
- 接受之前就处理到的那一次不撞：收寄采用时 PS 答 `REQUEST_NOT_ACCEPTED`；交接时对象还不是当前已接受包裹，按 ADR-0215 入账。撞的只有「早于接受、晚于初始路由形成才处理」这一格。
- 代码（取证于 main `cbcaaa873`，本记录之前）：复核编排失效线 `Lapse`、改善替代 `Supersede` 以触发业务时间离场；`ReassessmentTrigger` 不带接受时刻；收寄采用信封与采用记录都不带接受时刻。

## Decision

**一、复核的业务时间下限是这一判断键的委托接受时刻。**

- 委托接受时刻即接受基线固定的那一刻，与初始路由计划的生效起点同一刻。
- 触发的业务时间严格早于它时，复核答`未判`，原因「触发早于委托接受」（`TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE`）。
- 未判登记同一行另记比较所用的委托接受时刻，审计答得出「早于谁」，口径同 ADR-0235 决定四给当前位置记时刻。
- 不读路由历史，不判：原计划不失效、不被替代，原先无当前有效路由时也不形成首个计划。
- 同一时刻不算更早，照判。这时离场与生效落在同一刻，离场门放行。
- 这是终局入账。业务时间不会变，重投永远推不动；也不等登记，不进续办。

**二、先比接受，再比当前位置；两道都在已有结果分界之后。**

- 已判过的触发再投，读回原结果，不因下限而改答。
- 接受是判断键自己的下限，不依赖路由历史，所以先比。触发同时早于接受与当前位置时，记「触发早于委托接受」。
- 两格分开记：一格比的是委托本身，一格比的是路由历史里的某一次已判复核；合成一格，审计与续办都分不出早于的是哪一样。

**三、下限取判断键的接受时刻，不取当前计划的生效时刻。**

- 原先无当前有效路由那条线上没有计划，取计划生效时刻就缺了这一道下限，照样会形成早于接受就生效的首个计划。
- 复核形成的计划从它所判触发的业务时间起生效，不晚于当前位置；能越过当前位置那一道的触发，自然不早于它们。初始计划的生效起点就是接受时刻。
- 所以有了两道下限，复核能判的触发不早于路由历史里任何一份计划的生效时刻，离场门不再因业务时间而拒。

**四、接受时刻随触发带进复核，由两路消费侧适配器从 parcel-shipment 读。**

- 取值是 PS 委托聚合接受基线的固定时刻，与初始路由交接带来的接受时刻同源，两处取法一不同，初始计划的生效与复核的下限就对不上。
- 交接一路：适配器本就按（租户，包裹）读回当前已接受委托（ADR-0215），接受时刻随那一次读回一并取。
- 收寄采用一路：采用信封与采用记录都不带接受时刻，适配器按同一读法读回，并核对读回的委托与基线就是采用记录指着的那一份。
  - 读不动是依赖故障，登本路未决哨兵，回滚重投。
  - 读不着、读到另一份委托或另一条基线，是不变量破了：采用以当前已接受委托声明了这件包裹为前提（ADR-0060），已接受是终态、基线一经固定不再改。响亮上抛，不进哨兵。
- 复核触发缺接受时刻即不受理：两路都由适配器译出，缺了是适配器缺陷。
- 接受时刻不进触发指纹。它是判断键那条基线的属性，不是这次触发的内容；算进去，同一触发的重放会因读法不同而答冲突。

## 候选与反方

- **复核时把离场夹到生效时刻。** 否决：ADR-0272 已否决同形候选（「生效仍取处理时钟，失效时把离场时刻夹到不早于生效」）；失效时刻就不再是依据到来的业务时间，审计读到的是没人观察到的时刻；交接一路 `AT-NR-064` 钉住失效时刻取交接的业务时间。原先无当前有效路由那条线也夹不住，它不经离场门。
- **PS 采用收寄时就拒。** 否决：所有权错，采用答的是责任起点是否成立，「这次控制早不早于路由」是 NR 路由历史上的事；而且盖不到 TF 交接那一路。
- **下限取当前计划的生效时刻。** 否决：见决定三，无路由那条线没有计划。
- **回滚重投。** 否决：同 ADR-0235 的同名反方，业务时间不会变，重投永远推不动，只把分区烧到 `ABANDONED`。
- **并入「触发早于当前位置」一格，把接受当作一次虚拟的当前位置。** 否决：当前位置是路由历史里的事实（ADR-0235 决定四只认已判复核），塞进委托接受会改它的定义；两格合一，审计分不出早于的是哪一样。
- **接受时刻随信封带。** 否决：要改 PS 采用信封与 TF 交接信封的契约；交接信封的来源根本不知道委托。消费侧适配器读提供方是 ADR-0025 的常规做法，交接一路今天就在读。
- **在 PS 委托仓储或 PS 读口上加一法取接受时刻。** 未采纳：NR 已有的 PS 读口（按包裹反查当前已接受委托再读聚合）就交得出它，不必动 PS。

## 对已接受记录的作用

- **部分停用（扩成）** ADR-0235 决定一「重投推不动的答`未判`（`ReassessTriggerNotJudged`）：没有路由历史、证据未配置、冻结形态未配置。原因归 `ports.NotJudgedReason`。」一句：`未判`的原因封闭集再加一格「触发早于委托接受」（`TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE`，决定一）。**适用场景**：NR 路由复核的收寄采用与交接两路，触发的业务时间严格早于这一判断键的委托接受时刻。同句其余各格不变。
- **部分停用（扩成）** ADR-0235 决定四「复核在已有结果分界之后、读路由历史之前比较：触发的业务时间严格早于当前位置时，答`未判`，原因「触发早于当前位置」（`TRIGGER_PRECEDES_CURRENT_POSITION`）。」一句：比当前位置之前先比委托接受（决定二）。**适用场景**：同上。当前位置的定义、同一时刻照判、交接更正、判断键串行与 ADR-0235 其余各条不变。
- ADR-0272：无条款停用。它只管复核形成的计划；初始计划从接受起生效由 network-routing CONTEXT 定（票 code/60）。它决定三「离场门要求离场不早于生效，这一条对复核形成的计划恒成立」的论证不变，本记录让初始计划那一头同样成立（决定三）。
- ADR-0249：无条款停用。早于受理的揽收没有「受理与揽收之间」那一段窗口：初始路由在处理时判断，晚于揽收，揽收当时的限制已经在那一次判断里核过；揽收复核只按硬约束判的条款不变。
- ADR-0250：无条款停用。「终局入账、不进续办」与它决定二对续办纪律的读法一致。
- ADR-0174：无条款停用。本记录是它「按权威业务时间裁决、不按到达顺序」在复核下限上的又一处落点。

## Consequences

- 代码随票 docs-consistency/code/69：
  - 领域 `ReassessmentTriggerSpec.AcceptedAt` 与 `ReassessmentTrigger.PrecedesAcceptance`；
  - 端口 `NotJudgedTriggerPrecedesShipmentAcceptance`、`NotJudgedTrigger.ShipmentAcceptedAt`；
  - 复核编排在比当前位置之前先比接受；
  - 适配器 `AcceptedCarriedGeo.AcceptedAt`，收寄采用一路的 `ReassessOnNetworkIntakeAdapter` 多一个已接受委托读口，派发装配随之接上 PS 委托仓储，收寄采用那一路的未决哨兵加 `ErrAcceptedParcelUnreadable`。
- 迁移 network_routing 0024：未判登记原因封闭集加一格，另加 `shipment_accepted_at`，只在这一格原因下有值、触发不晚于它。比较用 `<=`：编排按纳秒比，落库两格都只留到微秒。
- NR CONTEXT「路由稳定性与受控改路」复核触发排序那条补一句；UC-NR-003 结果表`触发未判`补这一格，验收补 `AT-NR-087`。

## 越权风险点

1. **那次触发的硬约束检查被跳过，要等下一次触发再判**（NR owner）。早于接受的那次收寄或交接可能带着真实的失效依据，例如选中候选已被关务限制淘汰、包裹交进了不在计划上的节点。入账不判之后，原计划照旧当前有效，直到下一次控制触发从它自己的位置再判。之后若再没有控制触发（最后一个节点），这次失效就不会被判出，与 ADR-0250 越权风险点 4 同形，补法归票 [demo-chain-gaps/154](../../.scratch/demo-chain-gaps/issues/154-network-reassessment-not-judged-triggers-resume-after-registration.md)。owner 若要这次也判，要的是「从接受时刻的位置补判一次」那种续办，不是夹离场（见候选与反方）。
2. **先收后受理成常态的租户**（NR、PS owner）。某租户的首发动线里若先揽或先送站、后受理，再加上晚处理是常态，大量首次收寄复核会入账不判，首个节点之前的硬约束检查整片跳过。票面判为 R2，那时升 R1。缓解：未判登记按原因、按包裹列得出。
3. **收寄采用一路多依赖一次 PS**（NR、PS owner）。PS 读不动时这一路回滚重投，原先它不读 PS。读回的委托与采用记录对不上时响亮失败、占着分区；今天采用以当前已接受委托为前提、已接受是终态，到不了。
4. **业务时间与接受时刻都取自来源**（NR、PS、TF owner）。来源把收寄或交接的业务时间写早了，一次实际晚于接受的控制会被当成早于接受、不判；接受时刻取的是 PS 的时钟，PS 与来源的时钟偏差会挪动这条边界。同 ADR-0235 越权风险点 10。
5. **票 code/60 之前落库的初始计划不受这道下限保护**（NR owner）。那些计划按处理时钟生效，晚于接受；业务时间落在接受与处理之间的触发越过本记录的下限，照旧被离场门拒。首发前只有演示租户，`scripts/demo-seeds --reset` 重灌即可，同 ADR-0272 决定三那一句。

## Links

- [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md)：前驱——部分停用（扩成）决定一「重投推不动的答`未判`……」一句与决定四「复核在已有结果分界之后、读路由历史之前比较……」一句，见「对已接受记录的作用」
- [ADR-0272](./0272-a-plan-formed-by-route-reassessment-takes-effect-at-the-trigger-business-time.md)：复核形成的计划从触发业务时间起生效、离场取同一刻；离场夹到生效的同形候选已被它否决
- [ADR-0249](./0249-offsite-pickup-reassessment-checks-hard-constraints-only-and-does-not-establish-a-controllable-node.md)：揽收复核只按硬约束判；早于受理的揽收由本记录入账不判，无条款停用
- [ADR-0250](./0250-unclear-plan-applicability-in-reassessment-is-settled-as-not-judged-and-resume-in-the-same-change-stays-in-the-acceptance-chain.md)：`未判`先例与续办纪律
- [ADR-0174](./0174-loading-concurrency-is-judged-by-business-time.md)：按权威业务时间裁决、不按到达顺序
- [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md)：交接一路按（租户，包裹）读回当前已接受委托
- [ADR-0301](./0301-parcel-shipment-offers-an-acceptance-baseline-geo-projection-and-reassessment-no-longer-reads-the-whole-request.md)：后继——限定候选「在 PS 委托仓储或 PS 读口上加一法取接受时刻」一条的未采纳理由，无条款停用
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：消费侧适配器读提供方
- [network-routing CONTEXT](../domain/network-routing/CONTEXT.md)「路由计划适用性」「路由稳定性与受控改路」、[UC-NR-003](../application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md)（`触发未判`、`AT-NR-087`）
- 票 `.scratch/docs-consistency/code/69-a-trigger-earlier-than-the-shipment-acceptance-is-rejected-by-the-leave-gate.md`；出处 `.scratch/docs-consistency/code/60-initial-route-takes-effect-at-processing-time-unlike-reassessment.md`
