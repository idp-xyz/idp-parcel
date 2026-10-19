# 81 节点控制转移册上「控制来源链待重算」与入账不推进的各格没有处置面

Category: enhancement
Status: needs-triage——2026-10-19 票 [61](./61-node-control-transfers-out-on-an-authoritative-handover.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0214](../../../docs/adr/0214-node-control-transfers-out-and-in-on-an-authoritative-transport-handover.md) 决定五与越权风险点「对象认法只读链头」「不止一份在身控制时不推进」「交接早于控制成立入账」「控制来源链待重算没有处置面」「收寄晚于交接到达」；方案「后继」一节「控制来源链重算的处置面」那一条（[handover-dispatch-plan](../handover-dispatch-plan.md)）。
Blocked by: 无

## 现象

- 节点控制转移册上有几种入账、不推进的行，`/node-operations-records?registry=physical-control` 都看得见，但没有任何入口能处置：
  - `CONTROL_SOURCE_CHAIN_PENDING_RECOMPUTE`：已转出控制的交接被更正成拒收或待确认；
  - `NO_ASSOCIATED_UNIT` 与 `SEVERAL_ASSOCIATED_UNITS`：对不上一件作业实物，例如身份冲突期间交走的包裹；
  - `SEVERAL_ACTIVE_CONTROLS`：本票之前从无转出，同一件实物在两个节点各收寄过一次，就会有两份在身；
  - `HANDOVER_PRECEDES_CONTROL`：交接的业务时间早于在身控制成立，离线补登与时钟偏差都可能落进来。
- 收寄晚于交接到达（设备离线补登、收寄同步滞后）：交接信封先到，NO 落下 `NO_ASSOCIATED_UNIT` 或 `NO_ACTIVE_CONTROL` 并入账不重投，同一版已在册，再投也只答已在册。收寄随后成立，按业务时间早于交接，却不会再有信封来转出它；这件实物回到原节点时又会并进那条陈旧收寄。票 61 评审提出。
- TF CONTEXT 要求早期交接失效时「重算整段控制来源链并保留后续事实」，允许「当前实物方已明确但控制来源链冲突」。NO 这一侧只停在入账。

## 要裁的

- **处置的形态**：运营决定一类，由持授予的人指名转移册上的那一行，裁「控制仍归接收方」「控制回到某节点」或「指定那一份在身控制转出」，并带依据引用。参照身份冲突处置（ADR-0210）的形状。
- **重算由谁发起**：TF 更正之后自动重算整段链（要读移动、到达与交付事实），还是只给人工处置面。
- **对账读面**：要不要另开一张按实物列的视图，让「哪些实物控制状态可疑」一眼可见。
- **收寄晚到的那一格**：收寄成立时要不要反查转移册上同包裹、业务时间晚于这次收寄的入账行（对不上实物、无在身控制），就地补一次转出；还是只进处置面由人补。前者是机制，要裁它与收寄编排同笔还是另起一拍，以及多版交接同时命中时取哪一版。
- 三类：处置口与读面是机制；授予给谁是租户取值。

## 规模

M
