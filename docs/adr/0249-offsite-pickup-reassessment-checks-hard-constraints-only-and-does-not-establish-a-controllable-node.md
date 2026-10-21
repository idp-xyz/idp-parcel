# ADR-0249：场外揽收触发的路由复核不拿揽收地点比计划节点，也不建立当前可控节点——只按硬约束判原计划适用性，不判冻结、不自动改路

Status: Accepted（2026-10-21，Claude Code 会话代 NR owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/05，出处审计 [audit/nr-no.md](../../.scratch/docs-consistency/audit/nr-no.md) H2。部分停用 [ADR-0172](./0172-route-freeze-is-remaining-segment-count.md) 决定三「节点收寄或权威运输交接才能切前缀。」一句与决定四「复核在原计划仍可执行时先看冻结。」一句，部分停用 [ADR-0173](./0173-auto-reroute-is-folded-from-the-strategy-version.md) 决定三「原计划仍可执行时，先看冻结。」与「未越过且改善超过阈值，才自动切换。」两句，各处都只限场外揽收触发的复核，被停原句与适用场景见「对已接受记录的作用」；ADR-0172、ADR-0173 正文不改，只在其 Status 行与 Links 加前向指针。代码随票 [docs-consistency/code/16](../../.scratch/docs-consistency/code/16-pickup-reassessment-compares-the-pickup-place-with-plan-nodes.md)。越权风险点见文末，待 NR owner 复核，第 1 点同时待 TF owner 会签。）<!-- 索引注记 -->｜**比法**：揽收触发的复核只问选中候选的硬约束——被有效限制淘汰即失效、限制状态未知按看不清、其余仍适用；揽收地点原样记入复核依据，不与计划节点比。**可控节点**：揽收不建立当前可控节点，与「交给运输方的交接」同属不在节点上的控制；已执行前缀为空、全部计划段未执行，不判冻结、不试成本改善切换；失效时只形成改路建议，阻塞 `NOT_AT_CONTROLLED_NODE`，决定从包裹交进的首个节点生效。**复核与否**：揽收是有效网络收寄的来源，照复核；节点交给运输方没有新事实可核，照旧不复核。揽收复核计入当前控制位置
Date: 2026-10-21

## Context

- NR CONTEXT「路由形成与选择」要求有效网络收寄形成后按实际接货位置、控制依据、当前限制、截单和网络可用性重新校验，场外揽收时使用运输履约的揽收交接结果。同一节又写「拒收、待确认与交给运输方的交接不建立可控节点，不复核」；「路由稳定性与受控改路」写「可控节点只认节点收寄或权威运输交接」。揽收算不算可控节点，这几句各执一词。
- 代码：收寄采用一路的消费侧适配器（`adapters/parcelshipment/reassess_on_intake.go`）取揽收地点作复核位置，`controlKindFor` 把场外揽收译成 `TransportHandoverControl`。领域 `ReviewPlanApplicability` 拿位置去比计划节点，不在计划节点上即 `PLAN_ORIGIN_MISMATCH/<地点>`，原计划失效。`structuralAutoReroute` 把这一格也当成位于可控节点。
- 揽收地点是运输履约的实际接货位置引用，取值是客户场所一类（TF 领域测试里是 `customer-warehouse-2`），不是物流节点码，也不带地理投影。计划节点只由节点间段推出（[ADR-0217](./0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md)），计划里没有揽收段，也没有「计划接货范围」可比。后果是每一次场外揽收复核都让初始计划失效。UC-NR-003 `AT-NR-034` 期望的是保留原路由。
- [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md) 已把权威运输交接的复核限在「`已交接`且声明了接收节点」，位置取接收节点；没声明接收节点（交给运输方）不复核。

## Decision

**一、揽收触发的复核只按硬约束判原计划适用性，揽收地点不与计划节点比。**

- 适用性只看选中候选的硬约束：被有效硬限制淘汰即失效，限制状态未知按「适用性看不清」（处置见 [ADR-0250](./0250-unclear-plan-applicability-in-reassessment-is-settled-as-not-judged-and-resume-in-the-same-change-stays-in-the-acceptance-chain.md) 决定一），其余仍适用。
- 揽收地点原样记入复核依据，供审计与追踪，不参与判断。计划起点本由受理基线的地理投影经服务区域解析而来；揽收地点没有投影，NR 拿它比不出任何东西。
- 揽收是否发生在委托声明的寄件地点，不归本上下文判：那是运输履约的揽收作业与小包托运的收寄采用。包裹随后交进的首个节点不在计划上时，交接触发的复核按实际节点与计划不符判失效（ADR-0215），偏差在那里接住。

**二、场外揽收不建立当前可控节点。**

- 当前可控节点只认节点收寄，或把包裹交进声明的接收节点的权威运输交接（ADR-0215）。揽收把包裹交到运输方手里：运营企业经运输方掌控包裹，但没有节点能从那里对它施加下一步作业。这与节点交给运输方的交接同属不在节点上的控制。
- 由此，揽收复核的已执行前缀为空，全部计划段都在未执行范围里。
- 原计划仍适用时直接保留，结论`仍适用`；不判冻结边界，也不试成本改善切换。冻结与改善只在可控节点上的复核里问；揽收时没有可控节点，改善要等包裹交进首个节点后的交接复核。
- 原计划失效时照常形成无当前有效路由并评估替代候选。自动改路条件「包裹位于当前可控节点」不成立，只形成改路建议，阻塞原因 `NOT_AT_CONTROLLED_NODE`。授权角色据建议作出的改路决定从下一个可控节点生效，即包裹交进的首个节点。
- 原先无当前有效路由、候选评估选出唯一合格候选时，照常形成首个当前有效路由计划。形成首个计划不是改路，不要求可控节点；计划自包裹交进首个节点起执行。

**三、两种交给运输方的控制，一种复核、一种不复核。**

- 两者都不建立可控节点，分别在于有没有新事实要核。场外揽收是有效网络收寄的来源，网络服务的责任自此开始。受理与揽收之间可能已有新的限制或网络变化，路由要用此刻的限制把原计划核一遍。节点交给运输方没有新的位置或物理事实可核，下一次复核等包裹交进下一个节点。
- 揽收复核是一次已判的控制触发，计入当前控制位置（[ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 决定四称「当前位置」），业务时间取揽收的发生时间。

## 候选与反方

- **拿揽收地点比首节点的服务区域。** 否决：揽收地点是不透明的场所引用，NR 没有它的地理投影；另建读口去取揽收地址，撞 [ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md) 决定一的同版性判据。计划起点本就是受理基线投影经服务区域解析出来的，再比一次只是投影比它自己。
- **计划带一段揽收段**，仿 ADR-0217 的交付段，终点是起点服务区域。未采纳：加了这段也只有服务区域版本可比，比不出地点；段种类、前缀与冻结的算法都要改，换不来判断力。日后运输履约的揽收结果带上地理投影时可以再议。
- **揽收算可控节点，可切前缀、可自动改路。** 否决：没有节点，新计划无从「从当前可控节点」生效。自动改路的前提正是包裹在节点上、只改未执行段。
- **揽收不复核。** 否决：CONTEXT 要求有效网络收寄之后重新校验；受理之后新出现的限制要在包裹进网之初就让原计划失效，不能等到首个节点。
- **比不出时答看不清。** 否决：比不出不是证据缺口，是比法不成立。答看不清会把每一次揽收复核都卡在未决或未判里。

## 对已接受记录的作用

- **部分停用** ADR-0172 决定三「节点收寄或权威运输交接才能切前缀。」一句。**适用场景**：场外揽收触发的复核。揽收是权威运输交接一类的控制，却不在节点上，不切前缀，已执行前缀为空。其余各条不变：扫描、位置、消息顺序不建立可控节点；节点不在计划上，前缀不成立；把包裹交进声明的接收节点的交接照旧切前缀。
- **部分停用** ADR-0172 决定四「复核在原计划仍可执行时先看冻结。」一句。**适用场景**：场外揽收触发的复核。仍适用即保留，不看冻结，因而也不落「冻结形态未配置」那一格未判。可控节点上的复核照旧先看冻结，其余各条不变。
- ADR-0215：无条款停用。它把交接的复核限在交进声明的接收节点，本记录决定二据此认可控节点。
- **部分停用** ADR-0173 决定三「原计划仍可执行时，先看冻结。」与「未越过且改善超过阈值，才自动切换。」两句。**适用场景**：场外揽收触发的复核。仍适用即保留，不看冻结、不试成本改善切换；改善要等包裹交进首个节点后的交接复核。可控节点上的复核照旧先看冻结、改善超过阈值才切换；决定三「硬约束失效仍走失效重判；失效后同样按本形态折自动条件」一句不变，揽收时折出的「位于当前可控节点」为假，只形成建议；ADR-0173 其余各条不变。

## Consequences

- 代码随票 docs-consistency/code/16：控制依据另成「场外揽收」一格，不再译作权威运输交接；适用性复核对这一格不比节点，只看硬约束；已执行前缀为空、全部段未执行；仍适用时不判冻结、不试改善；失效线折自动条件时位于可控节点为假。补 `AT-NR-034`、`AT-NR-084` 的用例。
- NR CONTEXT「路由形成与选择」「路由稳定性与受控改路」与 Language「当前可控节点」改成本记录的口径；UC-NR-003 `AT-NR-034` 改写，新增 `AT-NR-084`。
- ADR-0172、ADR-0173 的 Status 行与 Links 挂前向指针。
- 在 code/16 落地之前，生产上的揽收复核仍按地点比节点，原计划被误判失效。

## 越权风险点

1. **揽收发生在计划起点范围之外，NR 不知道**（NR、TF owner）。客户把包裹交到一个与声明不符的地点，NR 照旧判原计划适用。缓解：包裹交进的首个节点不在计划上时，交接复核判失效；地点与声明是否相符归运输履约的揽收作业。TF owner 若要在揽收结果里带地理投影，本记录的比法可以再议。
2. **揽收复核不试改善切换**（NR owner）。更便宜的路线要等包裹交进首个节点才判。揽收与首个节点之间通常只隔几小时，那时节点上的复核照常问冻结与改善。
3. **揽收时原计划失效只出建议**（NR owner）。授权决定面没落地之前（票 docs-consistency/code/29），包裹停在无当前有效路由。交进首个节点时，交接复核走「原先无当前有效路由」那条线：候选评估选出唯一合格候选即形成首个当前有效路由计划，不是改路，也不比改善阈值；评估未决或并列时继续无路由。
4. **揽收复核计入当前控制位置**（NR owner）。一次业务时间早于揽收的触发后到时入账不判。客户送站与场外揽收在同一件包裹上二选一，这一格只在来源业务时间填错时才撞上。
5. **原先无路由时，揽收复核就形成首个计划**（NR owner）。计划首节点取自候选，包裹后来交进的节点与它不同时，交接复核判失效，计划只活了一段路。不形成而等首个节点也是一种取法，代价是揽收之后到首个节点之前没有任何计划可供运输准备。

## Links

- [ADR-0172](./0172-route-freeze-is-remaining-segment-count.md)：部分停用决定三「节点收寄或权威运输交接才能切前缀。」一句与决定四「复核在原计划仍可执行时先看冻结。」一句，适用场景限场外揽收触发的复核
- [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md)：交接复核只对交进声明的接收节点的交接，位置取接收节点；本记录据此认可控节点
- [ADR-0173](./0173-auto-reroute-is-folded-from-the-strategy-version.md)：部分停用决定三「原计划仍可执行时，先看冻结。」与「未越过且改善超过阈值，才自动切换。」两句，适用场景限场外揽收触发的复核；「位于当前可控节点」是自动改路的结构条件之一
- [ADR-0217](./0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md)：计划节点只由节点间段推，计划里没有揽收段
- [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md)：当前位置（NR CONTEXT 称当前控制位置）按已判控制触发的业务时间取
- [ADR-0250](./0250-unclear-plan-applicability-in-reassessment-is-settled-as-not-judged-and-resume-in-the-same-change-stays-in-the-acceptance-chain.md)：限制状态未知的处置
- [ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md)：NR 不建读取地址的端口
- [network-routing CONTEXT](../domain/network-routing/CONTEXT.md)、[UC-NR-003](../application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md)（`AT-NR-034`、`AT-NR-084`）
- 票 `.scratch/docs-consistency/issues/05-nr-no-docs.md`；代码票 `.scratch/docs-consistency/code/16-pickup-reassessment-compares-the-pickup-place-with-plan-nodes.md`、`.scratch/docs-consistency/code/29-reroute-suggestions-and-ties-have-no-authorized-decision-face.md`
