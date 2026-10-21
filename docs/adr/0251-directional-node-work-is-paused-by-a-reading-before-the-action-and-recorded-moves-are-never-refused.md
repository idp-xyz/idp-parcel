# ADR-0251：节点方向性作业的「暂停」是作业端动作之前问的读面——服务端对已经发生的移入与分拣一律照记、同笔留下方向依据，写口不答「方向性作业受阻」

Status: Accepted（2026-10-21，Claude Code 会话代 NO owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/05，出处审计 [audit/nr-no.md](../../.scratch/docs-consistency/audit/nr-no.md) M7。把票 [no-directional-basis/01](../../.scratch/no-directional-basis/issues/01-consolidation-adds-members-without-any-directional-basis.md)「裁定修订」第 1、2、4 条（2026-10-16 会话代决）定为记录，不另起新口径。无条款停用。越权风险点见文末，待 NO owner 复核，第 3 点同时待 NR owner 会签。）<!-- 索引注记 -->｜**门在读面**：方向依据预检按作业实物答此刻能不能做方向性作业、缺哪几样依据（身份、服务结果、路由依据，取法同方向依据记录），作业端在动作之前问，离线时用缓存把方向性动作灰掉；「方向性作业受阻」是这一读面的答案。**写口照记**：在线口、一线过渡导入、离线回传交来的移入一律照记，不因方向依据缺失、未配置或读不动而拒收、悬置或回写，同笔留下方向依据记录；CONTEXT 括注里列的来路是举例，不是限定。**集运兼容**：成员级集运兼容判断是读面的一格依据，不兼容的移入照记、偏离归 NR 判。**今天**：读面随一线作业端排，NR 指令与兼容判断随 no-directional-basis/02；在此之前路由一格答`未配置`
Date: 2026-10-21

## Context

- NO CONTEXT「作业授权与执行」写：当前路由指令无效、缺失或被替代时，「分拣到下一方向、集包出运、装载和交出等方向性作业必须暂停，直至取得有效依据」。同一份 CONTEXT「集运、封装与载具」又写：「已经发生的移入（一线过渡导入、离线回传）照记为集运成员关系事实，不因方向依据缺失或读不动而拒收、悬置或回写」。括注是只限这两种来路，还是举例，没有写清。
- UC-NO-003 取了前一句：启动条件「方向性作业必须取得当前有效路由指令；集运还必须取得成员级集运兼容判断」，结果表有「方向性作业受阻」，`AT-NO-031`「阻止方向性分拣与集包」，`AT-NO-033`「只对兼容成员实际集运」。
- 代码取了后一句：`ConsolidateParcelsHandler.AddMember` 只要求方向依据记录落了册（`basis.Recorded()`），在线口也照落；`ConsolidationOutcome` 里没有「受阻」一格；方向依据的路由一格由 `UnconfiguredRoutingBasis` 对任何实物答`未配置`，因为 NR 今天没有路由指令与成员级集运兼容判断的模型。
- 票 no-directional-basis/01 在 2026-10-16 已经裁过一次：「门是读面，不是写口」，写口不再有「方向性作业受阻」一格。理由是 [ADR-0021](./0021-frontline-operations-client-is-part-of-the-product.md)：离线与弱网容忍是一线作业端的固有要求，离线设备回网后交来的每一条移入都是已经发生的物理动作，服务端答「受阻」时件早已进袋。那次裁定只落在票面上，CONTEXT 与 UC 没有跟着改。
- 同形的先例在运输履约：[ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 把出发门禁定为与移动事实同笔落册的读数、不拒收；[ADR-0219](./0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md) 让设备在出发之前预览同一份读数。

## Decision

**一、「暂停」由作业端在动作之前问读面来执行，不由写口拒绝。**

- 方向依据预检按作业实物答：此刻能不能做方向性作业，缺哪几样依据。依据各格与方向依据记录相同：身份、服务结果、路由依据，取法也相同。
- 作业端在动作之前问：在线时现问；离线时用缓存的参考数据把方向性动作灰掉，并标出缺的依据。
- 「方向性作业受阻」是这一读面的答案：对象、拟执行的动作、缺哪几样依据、此刻仍可做的安全作业。它不是写口的结果。
- 方向性作业指分拣到下一方向、建立面向下一段的集运成员、集包出运、装载和交出。接收、隔离、状况记录、测量与在场核对不问这一读面。

**二、写口对已经发生的方向性作业一律照记。**

- 在线口、一线过渡导入与离线回传交来的移入，不因方向依据缺失、未配置或读不动而拒收、悬置或回写；同一笔留下方向依据记录。日后分拣、装载等方向性作业事实建模时同此办。
- CONTEXT「集运、封装与载具」那条括注里列的来路是举例，不是限定。
- 是否构成路由偏离归 `network-routing` 判，异常与追责归 `visibility-exception`；本上下文只保存事实与当时的依据。

**三、成员级集运兼容判断是读面的一格依据，不是写口的门。**

- 拟组合成员的兼容判断由 `network-routing` 形成。作业端在移入之前，经方向依据预检的路由一格问它。
- 不兼容的成员实际被移入时照记，方向依据记录的路由一格记下当时所知；偏离由 NR 判。

**四、今天的落地范围。**

- 方向依据预检随一线作业端一起建（no-directional-basis/01 读面半边；作业端的排期见该票「排期代决」）。
- NR 的路由指令与成员级集运兼容判断随 [no-directional-basis/02](../../.scratch/no-directional-basis/issues/02-network-routing-has-no-routing-instruction-or-consolidation-compatibility-judgment.md)。在此之前，方向依据记录的路由一格答`未配置`，不替 NR 默认兼容。
- 方向依据记录与偏离标记已在 main 上（no-directional-basis/01 偏离标记半边）。

## 候选与反方

- **在线口挡、离线与导入照记**（读括注为限定）。否决：在线与离线是同一台设备在不同网况下的同一个动作，服务端分不出哪一次是「已经发生」；在线口一挡，弱网下的重投就会在件已进袋之后被拒，作业端只能丢掉事实或另造一条旁路。ADR-0021 已写明后端拒绝非法写入时给不出替代动作。
- **写口挡所有缺依据的移入。** 否决：理由同上，并且今天路由一格对任何实物都答`未配置`，挡住就是挡住全部集运。
- **读面先于作业端建。** 否决：读面唯一的消费方是作业端，先建一个没人问的端点（no-directional-basis/01「裁定修订」第 3 条）。

## 对已接受记录的作用

无条款停用。ADR-0021 的离线容忍与「后端……给不出替代动作」是本记录的依据；ADR-0205、ADR-0219 是同形先例，不受影响。

## Consequences

- NO CONTEXT「作业授权与执行」那条改为：方向性作业由作业端在动作之前问方向依据预检、暂停于读面；「集运、封装与载具」那条的括注写明是举例。
- UC-NO-003 的启动条件、结果「方向性作业受阻」、`AT-NO-031`、`AT-NO-033` 改成读面口径，并注明读面未实现、指票。
- 写口不加「受阻」一格，代码不改。

## 越权风险点

1. **在线口也不拒**（NO owner）。在线设备不问读面就移入，服务端照记。缓解：方向依据记录与偏离标记让运营在复核页上看得见缺依据的成员；作业端的流程负责先问。
2. **读面建成之前没有任何门**（NO owner）。首发若在一线作业端之前上线，方向性作业全凭现场纪律。今天生产上只有一线过渡导入构造集运编排，搬的是已经发生的成员事实，本就不过门。
3. **不兼容的成员照记**（NO、NR owner）。集运兼容判断只在读面上起作用，作业端没问或问后仍装，袋里就会有不兼容的成员；偏离判断（票 demo-chain-gaps/35）与出发门禁（ADR-0225 按成员展开）是后面两道。

## Links

- [ADR-0021](./0021-frontline-operations-client-is-part-of-the-product.md)：离线与弱网容忍、后端拒绝给不出替代动作
- [ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md)、[ADR-0219](./0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md)：门禁是读数、设备在动作之前预览，同形先例
- [ADR-0225](./0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md)：出发门禁按成员展开，是袋里有不兼容成员时的后一道
- [node-operations CONTEXT](../domain/node-operations/CONTEXT.md)「作业授权与执行」「集运、封装与载具」；[UC-NO-003](../application/node-operations/UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md)（`AT-NO-031`、`AT-NO-033`）
- 票 `.scratch/no-directional-basis/issues/01-consolidation-adds-members-without-any-directional-basis.md`（「裁定修订」）、`.scratch/no-directional-basis/issues/02-network-routing-has-no-routing-instruction-or-consolidation-compatibility-judgment.md`、`.scratch/docs-consistency/issues/05-nr-no-docs.md`
