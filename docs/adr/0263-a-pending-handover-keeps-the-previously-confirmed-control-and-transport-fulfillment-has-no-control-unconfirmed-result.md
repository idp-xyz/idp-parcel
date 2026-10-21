# ADR-0263：权威运输交接的待确认保持此前已确认的控制关系，运输履约不另立「控制待确认/冲突」——实物占有已变而身份认不出也记待确认，更正撤回的控制由失效参与版本与节点作业的「控制来源链待重算」合起来表达

Status: Accepted（2026-10-21，Claude Code 会话代 TF owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/08，出处审计 [audit/tf.md](../../.scratch/docs-consistency/audit/tf.md) M1（归 B）。不停用任何已接受记录的条款：改的是 TF CONTEXT「运输交接与实物控制」与生命周期「权威运输交接」里把「控制待确认/冲突」写成一种结果的句子，使之与 GLOSSARY「权威运输交接结果」、CONTEXT-MAP「node-operations ↔ transport-fulfillment」和代码一致。越权风险点见文末，待 TF owner 复核，第 2、3 点同时待 NO owner 会签，第 3 点另待 VE owner 会签。）<!-- 索引注记 -->｜**三格封闭**：权威运输交接结果只有已交接、已拒收、待确认（`HANDED_OVER`、`REFUSED`、`PENDING_CONFIRMATION`），交接结果与履约参与上都不另立「控制待确认/冲突」。**待确认保持原控制**：已拒收与待确认都保持此前已确认的控制关系；证据已表明实物占有变化、却认不出对象身份、成员范围或合法控制来源的，同样是待确认，那一版保存全部证据与复核入口，以更正成已交接或已拒收收尾；接收一侧认不出身份的实物归节点作业的待识别实物与身份冲突处置。**更正撤回控制**：`已交接`更正成已拒收或待确认时，运输履约形成失效参与版本、不回退到更早的控制版本，节点作业记「控制来源链待重算」、不把控制回退给交出节点，两者合起来就是「当前实物方已明确但控制来源链冲突」。**集运单元**：单元本身照三格判；成员随单元受控的派生规则与节点作业的单元控制规则同一处确立，确立之前不为成员形成参与或控制判断；封签异常时不得推出成员随单元受控，也不为成员另立待确认/冲突一格。越权风险点含待确认期间控制记在已无实物的一方、单元成员控制规则未立、长期待确认没有信号

Date: 2026-10-21

## Context

- 取证于 main `3619f5c0e`（本记录随票 docs-consistency/08 改写了下面引的 TF CONTEXT 与 UC 原句）。TF CONTEXT「运输交接与实物控制」里讲交接逐对象判断的那一条写：「若证据已经表明实物占有发生变化，却无法确认对象身份、成员范围或合法控制来源，则必须形成`控制待确认/冲突`，不能继续把已经没有实际保管能力的一方记为当前实物控制方。」同节「封签缺失、破损或不一致时」那一条写成员「当前成员实物控制形成待确认/冲突」。UC-TF-005 `AT-TF-055` 照这些句子写。
- 与之相抵的：
  - GLOSSARY「权威运输交接结果」：「只有已交接转移控制，已拒收和待确认均保持此前已确认的控制关系」。
  - CONTEXT-MAP「node-operations ↔ transport-fulfillment」：「只有“已交接”结果转移控制，已拒收或待确认均保持原控制」。
  - TF CONTEXT 自己的生命周期「权威运输交接」：「此前控制方在结果成立前继续保持控制」。
- 代码只与后一边一致：`domain.HandoverVerdict` 封闭三格（已交接、已拒收、待确认），TF 领域里没有「控制待确认/冲突」这一种结果；节点作业对拒收与待确认的首登零写入，对撤回控制的更正记「控制来源链待重算」、控制不回退（[ADR-0214](./0214-node-control-transfers-out-and-in-on-an-authoritative-transport-handover.md)、[ADR-0220](./0220-node-control-reads-the-handover-correction-chain-by-the-transport-fulfillment-back-pointer.md)）。
- 没有任何已接受的 ADR 定过「控制待确认/冲突」这一格。CONTEXT 里两种说法各执一词，代码只做了一边，所以要裁。

## Decision

**一、权威运输交接结果封闭三格。**（机制）

已交接、已拒收、待确认，逐载运对象各一格。本上下文不在交接结果上、也不在履约参与上另立「控制待确认/冲突」一格。

**二、待确认保持此前已确认的控制关系，包括「占有已变而认不出」的那一种。**（机制）

- 本上下文的控制关系说的是谁对这件实物承担运输控制责任，由控制事实（有效收寄、权威交接、有效交付）形成；它不随实物此刻在谁手里自动改写。
- 证据表明实物占有变了，却认不出是哪一件、哪几件、凭什么交过去，就没有一份可以确认的控制转移。这时如果把控制从原方拿走，这件实物在那段时间就没有任何责任方；如果记给接收方，等于在没有证明的情况下判了已交接。
- 所以这一种同样是待确认：那一版保存全部证据（包括表明占有已变的那一份）、冲突范围与复核入口；续办是补证据，以更正成`已交接`或`已拒收`收尾。原控制方在结果成立之前继续承担控制责任。
- 实物在接收一侧、身份认不出的那一半，由 `node-operations` 的待识别实物与身份冲突处置接住（NO CONTEXT「身份冲突处置」、[ADR-0210](./0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md)）。本上下文不替它造一个身份或一份成员范围。

**三、更正撤回控制，由两侧已有的记录合起来表达。**（机制）

- `已交接`被更正成已拒收或待确认：本上下文在那一段的同一条参与链上形成失效参与版本，链尾失效即该对象在本段当前无有效参与，不回退到更早的控制版本（[ADR-0112](./0112-source-correction-rederives-participation-as-a-superseding-version-on-the-same-segment.md)）。
- 节点作业一侧不把控制回退给交出节点，记「控制来源链待重算」（ADR-0214、ADR-0220）。
- TF CONTEXT 所说「当前实物方已明确但控制来源链冲突」就是这两者合起来的读法，不另立一格。

**四、集运单元。**（机制）

- 单元本身的交接照三格判，单元作为载运对象形成自己的履约参与。
- 成员「随单元受控」的派生规则与节点作业的单元控制规则同一处确立（票 demo-chain-gaps/80）；确立之前，单元的交接不为成员形成参与或控制判断，与 NO CONTEXT「接收、控制与交接」同一口径。
- 封签缺失、破损或不一致时，不得据原成员快照推出成员已经到达或随单元受控；原成员的来源链与交出责任主张保留，等授权开封与实际核对。本上下文不为成员另立「控制待确认/冲突」一格。

## 候选与反方

- **另立第四格「控制待确认/冲突」，控制在这一格里挂空。** 否决。挂空的那段时间这件实物没有责任方，下游（派送发起、实际履约发生项、节点控制）都要为它另写一套读法；而接收一侧认不出身份的实物，节点作业已经有待识别实物接住。运输履约与节点作业各存一份同一件事，迟早对不上。
- **待确认时把控制交给接收方。** 否决。这就是没有证明的已交接；更正回已拒收时还要把控制再拿回来，等于在两份互相抵消的事实上来回改写。
- **占有已变时按证据的强弱自动裁成已交接或已拒收。** 否决。证据规则是租户取值，本上下文不替登记方裁；裁错了的代价落在承运责任上。

## Consequences

- TF CONTEXT「运输交接与实物控制」里「权威运输交接逐载运对象判断并允许部分成立」那一条、以「封签连续且相符」与「封签缺失、破损或不一致时」起头的封签规则、以「后续证据、更正或有效性变化」起头的那一条，生命周期「权威运输交接」里以「待确认 → 已交接或已拒收」与「已交接形成控制转移」起头的条目，改成本记录的口径。
- UC-TF-005 `AT-TF-055` 改写；`AT-TF-054` 的单元控制派生在规则确立之前不推进成员，写进 UC-TF-005「给开发的交接」。
- GLOSSARY 与 CONTEXT-MAP 已经是本记录的口径，不需要改；`docs/product/IDP-PARCEL-BUSINESS-PROCESS-AND-MODULE-GUIDE.md` 里写「控制待确认/冲突」的地方交票 docs-consistency/10 改。
- 代码不改。

## 越权风险点

1. **待确认期间，控制记在已经没有实物的一方**（TF owner）。读控制关系的下游（全程追踪、小包托运）在这段时间看到的是原控制方，而实物可能已在接收方手里。缓解：待确认那一版带着全部证据，节点作业一侧的待识别实物也看得见；代价是今天没有人会被提醒，见第 3 点。
2. **成员随单元受控的规则没立**（TF、NO owner）。整袋交接是常态时，成员在本上下文没有参与、在节点作业仍挂着在身的控制，直到规则确立（票 demo-chain-gaps/80）。
3. **长期待确认没有信号**（TF、NO、VE owner）。一份待确认挂多久都不会自己了结，今天也没有一条异常信号规则据它形成信号。要提醒，去处是全程追踪的信号规则（阈值是租户取值）；本记录不立票，owner 复核时定。

## Links

- [ADR-0214](./0214-node-control-transfers-out-and-in-on-an-authoritative-transport-handover.md)：节点控制只在已交接时转出与转入，撤回控制的更正记「控制来源链待重算」
- [ADR-0220](./0220-node-control-reads-the-handover-correction-chain-by-the-transport-fulfillment-back-pointer.md)：节点作业按运输履约的更正回指认更正链
- [ADR-0112](./0112-source-correction-rederives-participation-as-a-superseding-version-on-the-same-segment.md)：更正撤回入场控制时形成失效参与版本
- [ADR-0210](./0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md)：身份冲突由节点作业的运营决定处置
- [transport-fulfillment CONTEXT](../domain/transport-fulfillment/CONTEXT.md)「运输交接与实物控制」、[UC-TF-005](../application/transport-fulfillment/UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md)（`AT-TF-054`、`AT-TF-055`）
- 票 `.scratch/docs-consistency/issues/08-tf-docs.md`；`.scratch/demo-chain-gaps/issues/80-a-consolidation-unit-handover-does-not-move-member-control.md`
