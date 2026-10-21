# ADR-0264：容量保障只挂在装载分配上——承运方确认订舱不要预占引用，出发不读容量与需求快照；需求快照变了，重评落在尚未消耗的预占与装载分配上

Status: Accepted（2026-10-21，Claude Code 会话代 TF owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/08，出处审计 [audit/tf.md](../../.scratch/docs-consistency/audit/tf.md) M2、M3（均归 B）。部分停用 [ADR-0203](./0203-capacity-assurance-requires-a-reservation-on-every-pool-of-the-schedule-and-reservations-are-operation-decisions.md) 决定一一句里确认订舱那一半，回答 [ADR-0202](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 越权风险点 17，被停原句与适用场景见「对已接受记录的作用」；ADR-0203、ADR-0202 正文不改，只在其 Status 行与 Links 加前向指针。代码不改，只有一处注释随票 docs-consistency/code/02 改。越权风险点见文末，待 TF owner 复核。）<!-- 索引注记 -->｜**确认订舱不挂保障**：承运答复的`接受`不要预占引用、不读池，接受量是承运方对自己舱位的陈述，可以超过本上下文在那一班池上登记的余量；订舱与答复都不占、不读容量池。**保护落在装载分配**：班次挂了池，分配逐池引用当前有效预占并声明分配量（ADR-0203 决定一装载分配那一半与决定二原样）；按确认订舱的量去预占是运营人员的决定，本上下文不从答复推一份预占。**出发不读容量**：出发照实落册，出发门禁读数只是关务那一问，不读容量与需求快照。**需求快照变了**：原快照与判断历史保留，尚未消耗的预占与装载分配由运营按新快照重核（另作或释放预占，分配改版重新声明分配量；分配的变化入口今天未接线）；已出发范围的差异作为实际装载与分配的比较事实留下。票 demo-chain-gaps/37 的保障判据作废，范围改为逐订舱的消费关系。越权风险点含池上看不出承运方已接受多少、需求变了没人提醒
Date: 2026-10-21

## Context

- [ADR-0203](./0203-capacity-assurance-requires-a-reservation-on-every-pool-of-the-schedule-and-reservations-are-operation-decisions.md) 决定一要求「装载分配或确认订舱」所在班次挂了容量池，就逐池引用当前有效预占并声明分配量；装载分配那一半由票 21 落地，确认订舱那一半归票 demo-chain-gaps/37，该票一直停在 needs-triage。
- 确认订舱的提交方是承运系统：答复走集成客户端族（[ADR-0202](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 决定二），线格式里没有预占引用，承运方也不知道本上下文的预占。判据挂在这一口上，答复方永远答不出来。
- 代码：`application.CommissionTransportHandler.AnswerBooking` 对`接受`不读池、不要预占引用；UC-TF-004 `AT-TF-041` 写「订舱确认但无容量预占依据 | 保留订舱结果，不能伪造有效预占」。ADR-0202 越权风险点 17 记着「承运方接受的量可以超过那一班池上的余量」，等票 37。
- 另一处（取证于 main `3619f5c0e`，下面引的 TF CONTEXT、UC 原句随票 docs-consistency/08 改写）：TF CONTEXT「运输委托、订舱、容量与装载」要求需求快照变化时「以新快照重新校验尚未完成的预占、分配、舱单范围和出发门禁」，UC-TF-003 写「在预占、分配、舱单冻结和出发前重新校验」，`AT-TF-089` 写「出发门禁仍引用过期需求快照 | 阻断出发范围并要求重评」。可「出发门禁读数」在 TF 是关务五格读数的专名，出发一律落册、读数不挡（[ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md)）；`domain.DepartureGateReading` 与 `application.ReadDepartureGate` 都没有容量或需求快照输入。需求快照过期挂在哪个门上，没有记录裁过。

## Decision

**一、确认订舱不挂容量保障。**（产品策略）

- 承运答复的`接受`不要预占引用，不读容量池。接受量是承运方对它自己舱位的陈述；本上下文在那一班上登的池是自己这一侧的运力账，两者不是同一本账。
- 接受量可以超过那一班池上的余量。ADR-0202 越权风险点 17 所记的情形由此成为定论，不再等后继票。
- 订舱申请与它的答复都不占、不读池（TF CONTEXT「订舱申请本身不占用容量」不变）。

**二、本上下文对容量的保护落在装载分配上。**（产品策略）

- 班次挂了池，装载分配就要逐池引用一份当前有效预占并声明分配量；没有池的班次不要求。这是 ADR-0203 决定一装载分配那一半与决定二，原样不变。
- 按确认订舱的数量去预占，是运营人员的决定（容量预占是运营决定，ADR-0203 决定三）。本上下文不从承运答复推一份预占，也不在答复到达时替运营占池。

**三、出发不读容量与需求快照。**（机制）

- 出发、移动、到达是已经发生的实际事实，一律照实落册；出发门禁读数只是关务那一问（ADR-0205）。容量够不够、需求快照新不新，都不进出发那一步。
- 需求快照（成员、当前有效测量或换算依据）变了：原快照与判断历史保留；尚未消耗的预占与装载分配由运营按新快照重核——另作或释放预占；装载分配改版时重新声明逐池分配量，不从前一版带过来（领域 `LoadAssignment` 的改版已如此）。装载分配的变化与撤回今天没有生产入口（ADR-0203「补记：票 21 落地时的补充裁定」越权风险点 14），接上之前重核只能经预占与释放。
- 已经出发的范围不回头改写：实际装载与分配、预占之间的差异，作为比较事实留下（TF CONTEXT「容量后来缩减……」那一条同一纪律）。

**四、票 demo-chain-gaps/37 改范围。**

- 「确认订舱用装载分配那条保障判据」作废。
- [ADR-0204](./0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md) 越权风险点 3、9 交给票 37 的「逐订舱的消费关系」（哪些订舱已随哪一段消耗）仍归该票。

## 对已接受记录的作用

- **部分停用** ADR-0203 决定一「装载分配或确认订舱所在的具体班次只要登记了至少一个容量池，就必须对该班次的每个池引用一份当前有效预占，并逐池声明分配量；没有登记容量池的班次不要求。」一句。**适用场景**：只限句中的确认订舱——承运方确认订舱不要预占引用、不读池。装载分配照旧适用这一句；决定一其余几句、决定二至六不变。
- ADR-0202：越权风险点 17「接受量不对班次余量判」由本记录决定一回答，无条款停用。
- ADR-0205：不停用。本记录决定三是它「出发一律落册」的直接推论，写在这里只为把 TF CONTEXT 那一句里的「出发门禁」拿掉。

## 候选与反方

- **确认订舱改由运营侧确认，确认时逐池引用预占。** 否决。承运方答了`接受`，订舱就是接受了；再要运营人员补一次「确认」，是把一件已经发生的外部事实拆成两步，第二步不做时订舱停在什么状态说不清。要保护运力，运营人员在装载分配那一步本来就要引用预占。
- **承运方答`接受`时，本上下文自动按接受量在那一班池上预占。** 否决。预占是运营决定（ADR-0203 决定三），有判断时点、有效期与需求来源，答复里一样都没有；自动预占等于替运营人员作一次决定。
- **需求快照过期时挡出发。** 否决。出发已经发生，挡登记只会让事实不落册；挡下违规出发是动作之前那一问的事，而那一问问的是关务。

## Consequences

- TF CONTEXT「运输委托、订舱、容量与装载」里讲确认订舱要容量保障的两条、讲需求快照变化要重新校验的那一条改写。
- UC-TF-003 容量与班次规则那一条、`AT-TF-089` 改写；UC-TF-004 `AT-TF-041` 的读法不变，补引本记录。
- 票 demo-chain-gaps/37 票面加 2026-10-21 更正。
- `application/commission_transport.go` 里 `SubmitBooking` 头注「确认订舱与容量的关系另归一张票」一句改指本记录，并入票 docs-consistency/code/02。

## 越权风险点

1. **池上看不出承运方已经接受了多少**（TF owner）。运营人员读池时，只看得到预占与分配；某一班承运方已接受的总量要另读订舱册。订舱与答复今天没有查阅面（ADR-0202 越权风险点 15，票 demo-chain-gaps/48）。
2. **需求变了没人提醒，分配也改不了**（TF owner）。重评靠运营人员按新快照重核；本上下文不监视测量或成员的变化，旧预占照旧有效到期满。装载分配的变化与撤回入口接线之前，已形成的分配改不了版，只能等预占到期（ADR-0203 补记越权风险点 14）。节点实测之后的再准备另立票（[ADR-0201](./0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 决定八）。
3. **租户可能要按确认订舱挡超卖**（TF owner）。真实租户若要求承运方接受的量不得超过已登的池，那是一种另加的产品策略，今天的答复口不挡；要加，改的是答复口的判法，并要先让答复带得出预占引用。

## Links

- [ADR-0203](./0203-capacity-assurance-requires-a-reservation-on-every-pool-of-the-schedule-and-reservations-are-operation-decisions.md)：部分停用决定一一句里确认订舱那一半；装载分配那一半与决定二至六不变
- [ADR-0202](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md)：承运答复走集成客户端族（决定二）；越权风险点 17 由本记录回答
- [ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md)：出发一律落册，读数不挡
- [ADR-0204](./0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md)：越权风险点 3、9 的逐订舱消费关系仍归票 37
- [transport-fulfillment CONTEXT](../domain/transport-fulfillment/CONTEXT.md)「运输委托、订舱、容量与装载」、[UC-TF-003](../application/transport-fulfillment/UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md)（`AT-TF-089`）、[UC-TF-004](../application/transport-fulfillment/UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md)（`AT-TF-041`）
- 票 `.scratch/docs-consistency/issues/08-tf-docs.md`；`.scratch/demo-chain-gaps/issues/37-confirmed-booking-capacity-relationship.md`；代码注释票 `.scratch/docs-consistency/code/02-tf-comments-cite-stale-decisions-and-context-line-numbers.md`
