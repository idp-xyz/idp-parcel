# ADR-0251：节点方向性作业的「暂停」由作业端按随对象下发、带有效区间的方向依据判——服务端对已经发生的移入与分拣一律照记、同笔留下方向依据，写口不答「方向性作业受阻」

Status: Accepted（2026-10-21，Claude Code 会话代 NO owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/05，出处审计 [audit/nr-no.md](../../.scratch/docs-consistency/audit/nr-no.md) M7；隔离评审指出初稿与 [ADR-0024](./0024-directional-work-basis-carries-a-validity-interval.md) 相抵，协调方裁定取 ADR-0024 的形状。把票 [no-directional-basis/01](../../.scratch/no-directional-basis/issues/01-consolidation-adds-members-without-any-directional-basis.md)「裁定修订」第 1、2、4 条（2026-10-16 会话代决）定为记录，读面的形状照 ADR-0024。无条款停用。越权风险点见文末，待 NO owner 复核，第 3 点同时待 NR owner 会签。）<!-- 索引注记 -->｜**门在作业端**：方向依据（身份、服务结果、路由依据，取法同方向依据记录）随对象下发、带有效区间，作业端据本地时钟判断是否仍在区间内，出区间或缺依据即暂停方向性作业；依据有效而网络不通时照做、事实入本地队列，与 ADR-0024 一致。「方向性作业受阻」是这份依据的答案，不是写口的结果。**写口照记**：在线口、一线过渡导入、离线回传交来的移入一律照记，不因方向依据缺失、未配置、过期或读不动而拒收、悬置或回写，同笔留下方向依据记录；CONTEXT 括注里列的来路是举例。**集运兼容**：成员级集运兼容判断是下发依据的一格，不兼容的移入照记、偏离归 NR 判。**今天**：依据下发随一线作业端排，NR 指令与兼容判断随 no-directional-basis/02；在此之前路由一格答`未配置`
Date: 2026-10-21

## Context

- NO CONTEXT「作业授权与执行」写：当前路由指令无效、缺失或被替代时，「分拣到下一方向、集包出运、装载和交出等方向性作业必须暂停，直至取得有效依据」。同一份 CONTEXT「集运、封装与载具」又写：「已经发生的移入（一线过渡导入、离线回传）照记为集运成员关系事实，不因方向依据缺失或读不动而拒收、悬置或回写」。括注是只限这两种来路，还是举例，没有写清。
- UC-NO-003 取了前一句：启动条件「方向性作业必须取得当前有效路由指令；集运还必须取得成员级集运兼容判断」，结果表有「方向性作业受阻」，`AT-NO-031`「阻止方向性分拣与集包」，`AT-NO-033`「只对兼容成员实际集运」。
- 代码取了后一句：`ConsolidateParcelsHandler.AddMember` 只要求方向依据记录落了册（`basis.Recorded()`），在线口也照落；`ConsolidationOutcome` 里没有「受阻」一格；方向依据的路由一格由 `UnconfiguredRoutingBasis` 对任何实物答`未配置`，因为 NR 今天没有路由指令与成员级集运兼容判断的模型。
- [ADR-0024](./0024-directional-work-basis-carries-a-validity-interval.md) 已裁定「暂停」怎样执行：方向性作业的依据随对象下发并携带有效区间，作业端据本地时钟判断是否仍在区间内，出区间等同无有效依据；依据有效而网络不通时方向性作业照做、事实入本地队列；它明文否决「作业端每次方向性作业前同步问一次服务端」。
- 票 no-directional-basis/01 在 2026-10-16 已经裁过「门是读面，不是写口」，写口不再有「方向性作业受阻」一格。理由是 [ADR-0021](./0021-frontline-operations-client-is-part-of-the-product.md)：离线与弱网容忍是一线作业端的固有要求，离线设备回网后交来的每一条移入都是已经发生的物理动作，服务端答「受阻」时件早已进袋。那次裁定只落在票面上，CONTEXT 与 UC 没有跟着改；票面写的「在线时现问」也与 ADR-0024 相抵。
- 同形的先例在运输履约：[ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 把出发门禁定为与移动事实同笔落册的读数、不拒收。

## Decision

**一、「暂停」由作业端按随对象下发、带有效区间的方向依据判断，不由写口拒绝。与 ADR-0024 一致。**

- 方向依据随作业实物下发：身份、服务结果、路由依据各一格，取法与方向依据记录相同；每一格带给出它的上下文所定的有效区间（ADR-0024），不得只给结论。
- 作业端据本地时钟判断手上这份依据是否仍在区间内。缺某一格、某一格答不成立，或已出区间，即暂停方向性作业，并标出是哪几格。
- 依据有效而网络不通时，方向性作业照做，产生的事实入本地队列；依据已过期时，即使网络通畅也暂停。作业端不在每次方向性作业前同步问服务端，那是 ADR-0024 否决的路。
- 「方向性作业受阻」是这份依据的答案：对象、拟执行的动作、缺哪几格或哪几格已过期、此刻仍可做的安全作业。它不是写口的结果。
- 方向性作业指分拣到下一方向、建立面向下一段的集运成员、集包出运、装载和交出。接收、隔离、状况记录、测量与在场核对不受这份依据约束（ADR-0024「安全作业不受本记录约束」）。
- 区间长度由各下发上下文按其治理规则给出，属租户取值，本记录不定、不内置默认。

**二、写口对已经发生的方向性作业一律照记。**

- 在线口、一线过渡导入与离线回传交来的移入，不因方向依据缺失、未配置、过期或读不动而拒收、悬置或回写；同一笔留下方向依据记录。日后分拣、装载等方向性作业事实建模时同此办。
- CONTEXT「集运、封装与载具」那条括注里列的来路是举例，不是限定。
- 是否构成路由偏离归 `network-routing` 判，异常与追责归 `visibility-exception`；本上下文只保存事实与当时的依据。

**三、成员级集运兼容判断是下发依据的一格，不是写口的门。**

- 拟组合成员的兼容判断由 `network-routing` 形成，随路由依据一格下发、带有效区间。作业端在移入之前据它判断。
- 不兼容的成员实际被移入时照记，方向依据记录的路由一格记下当时所知；偏离由 NR 判。

**四、今天的落地范围。**

- 方向依据的下发随一线作业端一起建（no-directional-basis/01 读面半边；作业端的排期见该票「排期代决」）。
- NR 的路由指令与成员级集运兼容判断随 [no-directional-basis/02](../../.scratch/no-directional-basis/issues/02-network-routing-has-no-routing-instruction-or-consolidation-compatibility-judgment.md)，下发时照 ADR-0024 带有效区间。在此之前，方向依据记录的路由一格答`未配置`，不替 NR 默认兼容。
- 方向依据记录与偏离标记已在 main 上（no-directional-basis/01 偏离标记半边）。

## 候选与反方

- **作业端在每次方向性作业前现问服务端**（初稿与票 no-directional-basis/01「裁定修订」第 1 条的「在线时现问」）。否决：ADR-0024 已否决，它等于取消离线容忍，弱网下每件包裹的处理时间挂在一次网络往返上；网络通不通也不是依据新不新鲜的证据。
- **在线口挡、离线与导入照记**（读括注为限定）。否决：在线与离线是同一台设备在不同网况下的同一个动作，服务端分不出哪一次是「已经发生」；在线口一挡，弱网下的重投就会在件已进袋之后被拒，作业端只能丢掉事实或另造一条旁路。ADR-0021 已写明后端拒绝非法写入时给不出替代动作。
- **写口挡所有缺依据的移入。** 否决：理由同上，并且今天路由一格对任何实物都答`未配置`，挡住就是挡住全部集运。
- **依据下发先于作业端建。** 否决：唯一的消费方是作业端，先建一个没人取的下发口（no-directional-basis/01「裁定修订」第 3 条）。

## 对已接受记录的作用

无条款停用。ADR-0024 定下依据的形状与作业端的判法，本记录照它把 NO CONTEXT 的「暂停」落到作业端；ADR-0021 的离线容忍与「后端……给不出替代动作」是写口照记的依据；ADR-0205 是同形先例，不受影响。票 no-directional-basis/01「裁定修订」第 1 条「在线时现问；离线时用缓存的参考数据把方向性动作灰掉」一句按 ADR-0024 读，不取「现问」。

## Consequences

- NO CONTEXT「作业授权与执行」那条改为：暂停由作业端按随对象下发、带有效区间的方向依据判断，与 ADR-0024 一致；写口照记。「集运、封装与载具」那条的括注写明是举例。
- UC-NO-003 的启动条件、结果「方向性作业受阻」、`AT-NO-031`、`AT-NO-033` 改成这一口径，并注明依据下发未实现、指票。
- 写口不加「受阻」一格，代码不改。no-directional-basis/02 的路由依据下发要带有效区间。

## 越权风险点

1. **在线口也不拒**（NO owner）。作业端不看依据就移入，服务端照记。缓解：方向依据记录与偏离标记让运营在复核页上看得见缺依据的成员；作业端的流程负责先判。
2. **依据下发建成之前没有任何门**（NO owner）。首发若在一线作业端之前上线，方向性作业全凭现场纪律。今天生产上只有一线过渡导入构造集运编排，搬的是已经发生的成员事实，本就不过门。
3. **区间之内的变化看不见；不兼容的成员照记**（NO、NR owner）。依据仍在区间内时，路由在服务端已经失效或改路，作业端照旧按手上的依据放行，直到区间到期——这是 ADR-0024 接受的代价，区间多长由 NR 按其治理规则定。集运兼容判断只在作业端起作用，作业端没看或看后仍装，袋里就会有不兼容的成员；偏离判断（票 demo-chain-gaps/35）与出发门禁（ADR-0225 按成员展开）是后面两道。

## Links

- [ADR-0024](./0024-directional-work-basis-carries-a-validity-interval.md)：方向性作业的依据随对象下发并携带有效区间，作业端据本地时钟判，出区间即暂停；否决每次作业前同步问服务端——本记录决定一照此
- [ADR-0021](./0021-frontline-operations-client-is-part-of-the-product.md)：离线与弱网容忍、后端拒绝给不出替代动作
- [ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md)：门禁是与事实同笔落册的读数、不拒收，同形先例
- [ADR-0225](./0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md)：出发门禁按成员展开，是袋里有不兼容成员时的后一道
- [node-operations CONTEXT](../domain/node-operations/CONTEXT.md)「作业授权与执行」「集运、封装与载具」；[UC-NO-003](../application/node-operations/UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md)（`AT-NO-031`、`AT-NO-033`）
- 票 `.scratch/no-directional-basis/issues/01-consolidation-adds-members-without-any-directional-basis.md`（「裁定修订」）、`.scratch/no-directional-basis/issues/02-network-routing-has-no-routing-instruction-or-consolidation-compatibility-judgment.md`、`.scratch/docs-consistency/issues/05-nr-no-docs.md`
