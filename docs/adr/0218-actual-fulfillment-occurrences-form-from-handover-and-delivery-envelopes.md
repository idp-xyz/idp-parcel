# ADR-0218：实际履约发生项由交接与有效交付信封自动形成——只对已采用委托的段，找出被这件事实结束的参与，经既有登记用例形成；旅程取替代 / 退运旅程，否则按包裹派生

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/67，统一代决见 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C15 与「需要的 ADR」ADR-0218。**部分停用** [ADR-0207](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) 决定七后半句、越权风险点 3 与决定四末句的演示约定，以及 [ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 决定五「登记者取自操作者信封的主体」一句，被停条款与适用场景见「对已接受记录的作用」。越权风险点见文末，待 TF owner 复核，第 2、3 点同时待 SA owner、第 5 点同时待 PC owner。）
Date: 2026-10-19

## Context

- ADR-0207 决定七：「不在任何物理编排里内联，也暂不自动形成……运输履约自消费交接与有效交付信封来自动形成，等 ADR-0209 与票 demo-chain-gaps/25 落地后另定。」越权风险点 3「不自动形成」：参与结束之后要有人登记，漏登就漏一份成本来源。
- 两个前提都已进 main：
  - ADR-0209：发生条件、数量与单位由采用的那一版协议声明，登记方不再断言。
  - ADR-0204：段可以声明采用一份运输委托。ADR-0207 文末补记接着裁了：段已采用委托时，采购上下文取委托快照，登记方可以不给。
- 交接登记（`已交接`）与有效交付登记，在同一笔里结束对象在前一段的参与，并交出引用式信封。参与的终点记着结束它的那件事实：交接的转出依据，或交付的那一版结果。
- 先例 [ADR-0216](./0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md)：TF 已在派发进程里自消费交接信封（派送发起）。同一封信扇出给几路，各记各的 inbox 账；处置按「原样重投会不会改变结果」分三格。
- 方案「复核」指出的缺口：发生项的旅程必填，而 ADR-0207 决定四只给了「登记方声明」一个来源，演示约定是 `SYN-JRN-<包裹号>`，自动路径无从取值。TF 自己有替代 / 退运旅程册：每一趟记着成员、开始时刻与它关联的原旅程。
- ADR-0209 决定五：每一项发生项记登记者，取自操作者信封的主体。自动形成那一路没有操作者。

## Decision

**一、自消费交接与有效交付两种信封，找出被这件事实结束的那一条参与。**（机制）

- 派发进程给交接登记信封、有效交付登记信封各多扇出一路，交给本上下文的自动形成编排。这一路排在既有各路之后，消费者名各自分家。
- 交接那一路：读回那一版交接。不是`已交接`的入账，不再往下查。是`已交接`的，终点依据取交接自己交出的转出依据。
- 有效交付那一路：终点依据取信封所指的那一版交付结果，不读交付登记册的当前版。POD 更正之后当前版换了，而结束参与的是首登那一版。信封载荷缺了版本是毒丸。
- 在对象有过参与的各段里，找当前参与（替代链的链尾、不是失效版本）终点种类与依据恰是这件事实的那一条：
  - 零条：入账。拒收与待确认不转出控制，更正出的新版本没有结束什么，对象本就不在段里，都落这一格。
  - 一条：往下走。
  - 多于一条：库面不一致，响亮失败，不挑一段去计费。
- 成员只有这一个对象。同一段上的别的成员，在各自的交接或交付上各形成各的。

**二、只对已采用委托的段形成；没有采用连线的段不形成，也不推断自营。**（产品策略）

- 采用连线是登记方对「这一段按哪份委托向外采购」的声明（ADR-0204 决定一、二）。有了它，采购上下文取委托快照，自动形成不需要任何人另报。
- 没有连线的段入账、零写入，也不交给登记用例。登记用例遇到没有连线的段，会去问实际承运商判断，可能答不适用；那就成了从承运商判断推断自营。连线缺席本身既不说外包也不说自营（ADR-0204 决定四）。
- 这些段的发生项照旧经运营决定人工登记（ADR-0207 决定六不变）。

**三、走既有登记用例，不另立形成门。**（机制）

- 交给登记用例的命令只有：原因实际履约、那一段、这一个对象、旅程（决定四）、登记者（决定五）。不报服务方与协议，也不声明自营。
- 依据、主要业务范围、业务时间、协议核对、发生条件、数量与单位、覆盖一问，以及已形成信封同笔交出，都由 `RegisterChargeOccurrenceHandler` 一处判（ADR-0207、ADR-0209 与 ADR-0207 文末补记）。在这里再判一遍，就是同一条规则两处定义。
- 身份仍由（租户、原因、事实依据、成员集合）派生，与人工登记落在同一个身份上。登记者不进内容比对，旅程进。所以人工按派生拼写登同一段同一件，与自动形成互相答`已存在`；人工写了别的旅程，后到的一方答冲突。
- 发生项与已形成信封落在派发进程那一笔 inbox 事务里。未决与缺陷都整笔回滚。

**四、旅程。**（产品策略）

- 先看对象在 TF 替代 / 退运旅程册上，有没有一趟在这条参与入场那一刻已经开始。有就取开始得最晚的那一趟；同一刻开始两趟不挑，响亮失败。
- 没有，就按对象的正式包裹身份确定性派生 `JRN/<包裹>`。派生而不签发：重放与人工登记要落回同一个身份、同一份内容。
- 按入场时刻判，不按处理这封信的此刻判。退运可能在对象还在原段里时就已启动，而原段那一程属于原旅程（TF CONTEXT「退运……不得通过倒退原实际履约段……表达」）。按此刻判，信封晚到或重投就会把原段的成本挂到新旅程上。
- 演示约定改用派生拼写 `JRN/<包裹号>`。沿用 `SYN-JRN-<包裹号>`，人工登记与自动形成会撞在同一身份上、内容不同，答冲突。

**五、登记者是本上下文的系统身份。**（机制）

- 自动形成那一路，登记者记 `idp-parcel/transport-fulfillment#form-actual-fulfillment-on-participation-end`。
- 写成「发行方#sub」同一形状，发行方取本上下文的信封来源。操作者的发行方是 OIDC 发行方的 URL，两者不会读成同一个人。
- 登记者是登记决定的出处，不是发生项内容（ADR-0209 决定五后半）。人工后登同一身份答`已存在`，回显的是这一格。

**六、处置：原样重投会不会改变结果。**（机制，判据同 ADR-0216 决定三）

- **入账，不重投**：
  - 编排自己答的三格：交接不是`已交接`；没有参与被这件事实结束；段没有采用连线。
  - 登记用例答的几格：已登记、已存在、冲突（采购上下文冲突除外）、协议未生效与已终止、未配置、不计费，以及未受理「缺采购上下文」。未生效与已终止可能就是委托那一版在业务时间的如实答案。缺采购上下文只出现在迁移 TF 0038 之前提交的委托上，委托没记下协议版本，要人工登记时报出。
- **重投**：编排读不动，即交接还读不回或读不动、段登记册、采用连线、旅程册读不动；以及登记用例的未决。两路各登自己的未决哨兵名单。
- **响亮失败**，不入账也不登哨兵：
  - 本上下文自己交出的键过不了自己的构造门；
  - 同一件事实在多个段里都结束了参与；
  - 旅程册上同一刻开始两趟；
  - 段登记册按对象列得出、按键读不回；
  - 登记用例答不适用、答「缺采购上下文」以外的未受理，或答出集外一格；
  - 登记用例答协议不在册、供应商不符或采购上下文冲突。本路不报服务方、协议与自营，段又已采用委托：委托记着协议版本时，前两格由登记用例上抛；没记着时先答缺采购上下文。三格都走不到，走到了就是两边走岔，要响起来让人重看，不静默入账；
  - 登记用例上抛的数据违约（采用连线指名的委托不在册，协议核对口与委托走岔）。
- 三格怎么分，由应用层的 `Disposition` 判；消费门只译成 inbox 的三种动作。

**七、不内联，人工口保留。**（机制）

- 交接与交付编排照常落册，不为形成发生项失败（ADR-0207 决定七前半句、ADR-0098 决定一不变）。
- 运营决定那一口（ADR-0207 决定六）照旧开着。下面几种情形要靠它：没有采用连线的段；迁移 0038 之前提交的委托；采用晚于派发处理那封信的段；派发处理时协议答未配置、后来才另发一版补上条件的段。

三类：一、三、五、六、七是**机制**；二（只对已采用委托的段）与四（旅程的取法）是**产品策略**；哪些段采用哪份委托、演示约定的旅程拼写是**租户取值**，演示租户证据只记 `S`。

## 对已接受记录的作用

- **ADR-0207 决定七后半句与越权风险点 3，部分停用。**
  - 被停条款：标题里「也暂不自动形成」，正文「运输履约自消费交接与有效交付信封来自动形成，等 ADR-0209 与票 demo-chain-gaps/25 落地后另定」，越权风险点 3「不自动形成」。
  - 适用场景：只限有采用连线的段。停用的理由带前提「采购上下文已由委托给出、条件已由协议声明」，没有连线的段没有这个前提，仍不自动形成、由人工登记。
  - 决定七前半句「不在任何物理编排里内联」不变。
- **ADR-0207 决定四末句，停用。**
  - 被停条款：「演示约定 `SYN-JRN-<包裹号>`」。
  - 适用场景：演示租户，自本记录起改用 `JRN/<包裹号>`。「旅程由登记方声明」对人工登记不变。
- **ADR-0209 决定五，部分停用。**
  - 被停条款：「登记者取自操作者信封的主体」。
  - 适用场景：只限自动形成那一路。那一路没有操作者信封，登记者记本上下文的系统身份（决定五）。运营决定那一路不变。
- 回答 ADR-0207 越权风险点 2「旅程无人核」的一半：自动形成的旅程有了取法。人工声明的旅程仍不核。

## 候选与反方

- **在交接或交付编排里同笔形成。** 否决：违反 ADR-0207 决定七前半句与 ADR-0098 决定一。物理登记会为一次协议核对读不动而失败；或者形成静默失败，没人重试。
- **没有采用连线也自动形成，采购上下文从承运商判断、交接接收方或当前协议推。** 否决：ADR-0204 决定四与 TF CONTEXT 禁止按当前伙伴、线路补齐。
- **没有连线时也交给登记用例，让它答缺采购上下文或不适用。** 否决：不适用那一格是从实际承运商判断推出自营；缺采购上下文那一格对每一段非外包的段都会响一次，是噪声。
- **等段关闭时按段一次形成，成员取段上全部对象。** 否决：关段是一个决定，不是可推导的状态（TF CONTEXT 生命周期④），产品没有自动关段。按件计的协议要按成员登；按段计的协议，第一件已经盖住整段。
- **旅程取处理这封信那一刻的当前旅程。** 否决：信封晚到或重投会把原段的成本挂到新旅程上。
- **旅程签发新号，或旅程改为可空。** 否决：旅程进身份之外的内容比对；签发的号让重放与人工登记对不上同一项。旅程可空要改领域与库面的必填，而原旅程与新旅程分别关联发生项是 CONTEXT 的要求。
- **登记者留空，或借用某个操作者。** 否决：库面登记者必填；借用操作者是伪造出处。
- **读交付登记册的当前版作依据。** 否决：POD 更正之后，首登那封信会被判成「没结束参与」，这件事就永远不形成。

## Consequences

- TF：
  - 领域：`ActualFulfillmentJourney`、`EffectiveDeliveryEndBasis`、`ActualFulfillmentSegment.ParticipationEndedBy`；交付结束参与改用 `EffectiveDeliveryEndBasis`，两处同一出处。
  - 端口另立文件 `ports/alternate_journey_member_view.go`（`AlternateJourneyMemberView`）；postgres `AlternateJourneys.FindByMember`。
  - 应用：`application/form_actual_fulfillment_on_participation_end.go`（`FormActualFulfillmentOnParticipationEndHandler`、结果代数与 `Disposition`、`ActualFulfillmentFormationRegistrant`）。
  - 消费门：`adapters/inbox/actual_fulfillment_formation_consumers.go`，两个消费者名 `transport-fulfillment/form-actual-fulfillment-occurrence-from-transport-handover`、`…-from-effective-delivery`。
  - 不加迁移，不加信封；登记用例与已形成信封不变。
- 派发进程：交接与有效交付两张扇出各加一路；装配另立 `assemble_actual_fulfillment_on_participation_end.go`，两份哨兵名单。登记用例照 `cmd/parcel-api` 的接法另接一份，不跨 main 包复用。
- 文档：
  - [UC-TF-005](../application/transport-fulfillment/UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md) 目标与边界一条、步 9 与 `AT-TF-246`–`255`；
  - [TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)「运输收费发生项与成本来源」一条；
  - [演示动线](../design/synthetic-demo-journey-script.md)「第 3 步」补记、种子 README 第 8 步一段。
- 演示（证据 `S`）：干线段采用委托之后在 SIN-HUB 交接。派发两拍——交接信封那一拍形成发生项，发生项信封那一拍发起 BUY 请求——不用人工登记。
- 已知代价，本记录接受、不在本票处理：
  - 本路停住时挡同一分区后面的信封，见越权风险点 8。
  - 每封信都按对象列出它走过的各段、逐段整段重建，登记用例随后再把结束参与的那一段重建一次。干线段上有 N 件包裹时，单封信是 O(N)，整段是 O(N²)。人工登记时无所谓，自动形成逐件触发就放大了。去处：负载证据出现时，加一个按（租户、对象、终点种类与依据）直接查出那一条参与与它所在段的窄读口；在此之前不另立。

## 越权风险点

1. **自动形成没赶上的参与不补**（TF owner）。形成与否看的是派发处理这封信的那一刻：段那时有没有采用连线，委托那一版协议那时答什么。
   - 采用晚于这封信被派发处理（ADR-0204 允许参与结束之后才采用），这条参与不会回头形成，要人工登记。采用若落在参与结束与派发处理之间，照样会形成。所以同样是参与结束之后的采用，形不形成随派发时序而定，不是确定的。
   - 派发处理时协议答未配置，租户后来另发一版补上条件，同样不回头形成。
   - 结算那一侧仍没有「该有而没有」的提示。去处是票 [demo-chain-gaps/88](../../.scratch/demo-chain-gaps/issues/88-actual-fulfillment-not-formed-when-adoption-or-agreement-arrives-late.md)。
2. **交接更正撤回控制之后，已形成的发生项不回退**（TF、SA owner）。`已交接`被更正成拒收或待确认，参与的终点不回退（TF CONTEXT 禁止），已自动形成的那一项也照留。去处同 ADR-0207 文末补记风险点 10：发生项修订入口。
3. **登记者是系统身份**（TF、SA owner）。审计在这些项上看到的是本上下文，不是一个人；人工后登答`已存在`，回显的也是系统身份，人工那一次的出处不另留痕。
4. **旅程取法**（TF owner）。
   - 对象是集运单元时，派生的 `JRN/<单元>` 指的是单元，不是某件包裹的旅程。TF 的载运对象引用不带种类，分不出来。
   - 替代 / 退运旅程册今天没有生产写口，生产上旅程一律是派生拼写。
   - 旅程跨上下文的权威来源仍未定（ADR-0207 越权风险点 2）。
5. **按段计的协议下，发生项成员只有第一件**（TF、PC owner）。第一件的参与结束就盖住整段，后到的各件答范围已计过、入账。段费只计一次，但那一项的成员名单只有第一件，审计从发生项上看不出这份段费覆盖了哪些包裹。这是 ADR-0207 越权风险点 4 的另一面。
6. **入账的业务格不会自愈**（TF owner）。未配置、不计费、协议否定格、缺采购上下文都入账；补齐之后没有东西回头重跑，与风险点 1 同一去处。
7. **演示约定改拼写**（演示租户）。按 `SYN-JRN-<包裹号>` 登过的旧项照留。同一段同一件再自动形成时答冲突、入账，不会多出第二项。
8. **本路停住时挡同一分区后面的信封**（TF owner）。
   - 交接信封与有效交付信封各按（租户、对象、口名段）分区（ADR-0074 决定二），Outbox 按分区只交出队头。本路停在未决（协议核对口读不动等），或响亮失败（旅程平局、连线指名的委托不在册、协议核对与委托走岔），这封信就留在队头重投，直到重投成功，或烧完失败预算落 `ABANDONED`。
   - 同一对象后面的交接或交付信封都排在它后面：下一次交接的追踪投影与派送发起（ADR-0216），交付分区上 POD 更正那一封的包裹终局与追踪投影，都跟着等。扇出里殿后只保住同一封信上的各路不被它挡；到分区这一层，计费会挡履约与追踪。
   - 放大：票 [54](../../.scratch/demo-chain-gaps/issues/54-charge-occurrence-registration-does-not-separate-deterministic-agreement-check-failures.md)、[77](../../.scratch/demo-chain-gaps/issues/77-charge-occurrence-fact-reads-do-not-separate-unrehydratable-rows.md) 把确定性失败答成未决。在人工口上，登记方只是看到一次未决；在本路上，会以未决之名挂在队头烧失败预算。
   - 接受现状的理由：同一张交接扇出上的派送发起那一路已是同一形状；未决只在依赖读不动时出现，读得动就过；缺陷本该响亮，修好即放行。
   - 去处是票 [demo-chain-gaps/89](../../.scratch/demo-chain-gaps/issues/89-actual-fulfillment-formation-blocks-later-envelopes-in-the-handover-and-delivery-partitions.md)：把自动形成挪到它自己的分区上，或凭运行证据维持现状。

## Links

- [ADR-0207](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md)：部分停用其决定七后半句、越权风险点 3 与决定四末句的演示约定；人工登记口（决定六）与登记用例不变
- [ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md)：部分停用其决定五「登记者取自操作者信封的主体」（限自动形成那一路）；条件与数量仍按它判
- [ADR-0204](./0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md)：采用连线是只对已采用委托的段形成的依据；决定四「没有连线不读作自营」
- [ADR-0216](./0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md)：TF 自消费交接信封、扇出各记 inbox 账、处置三格的先例
- [ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md)：已形成信封的消费方，BUY 评价由它发起
- [ADR-0098](./0098-a-failed-attempt-charge-occurrence-is-not-formed-inside-the-pickup-orchestration.md)：决定一「不在物理编排里形成」不变
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)、[ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)：确定性错误不进未决名单，重投与否按恢复动作分
- [UC-TF-005](../application/transport-fulfillment/UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md)：步 9，`AT-TF-246`–`255`
- [ADR-0074](./0074-tf-object-partitions-carry-a-port-segment-apart-from-ve-parcel-partitions.md)：交接与有效交付的分区键带口名段；本路停住挡的是这一分区（越权风险点 8）
- 票 `.scratch/demo-chain-gaps/issues/67-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md`；后续票 `88`、`89`
