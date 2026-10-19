# ADR-0204：履约段采用运输委托——声明式连线，采用即以段的首个参与标委托开始，开始之后不可取消

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/25](../../.scratch/demo-chain-gaps/issues/25-segment-adopts-transport-commission-and-cancellation-faces.md) 起草并落全部条款。部分停用 [ADR-0098](./0098-a-failed-attempt-charge-occurrence-is-not-formed-inside-the-pickup-orchestration.md) 决定四，适用场景只限实际履约段。越权风险点见文末，待 TF owner 复核，access-identity owner 知会。）

Date: 2026-10-19

## Context

- 运输委托有「已开始」一格：`TransportCommission.MarkTransportStarted` 以首个控制事实登记运输已开始，`Cancel` 对已开始的委托答 `ErrTransportAlreadyStarted`。两者都没有生产调用方——委托提交口（ADR-0202）接上之后，没有任何一条路径会把委托标为开始。
- 于是取消面不能开：没有开始连线就开取消，已在路上的运输会被答成已取消，违反 TF CONTEXT 生命周期「尚未消耗的运输委托或订舱范围可以取消」与 UC-TF-004 的 `AT-TF-092`（取消到达时服务已经开始，取消只处理尚可停止范围）。
- 运输收费发生项登记（ADR-0207）的实际履约原因要采购上下文。ADR-0098 决定二要它由调用方显式给出、本上下文不推导；票 demo-chain-gaps/28 要在段已采用委托时从委托快照取。前提是段与委托之间有一条可读的连线。
- ADR-0098 决定四把「揽收任务与运输委托之间的连线」显式留空，理由是「建它等于在没有任何租户实证的情况下先定一种采购组织方式」。段与委托的连线落在同一句的射程里。
- 本批代决的单一权威是 [tf-chain-plan](../../.scratch/demo-chain-gaps/tf-chain-plan.md)「02 委托、订舱、承运答复」D02-6 与「需要的 ADR」ADR-0204；本记录把其中的裁定写成决定，计划此后只作索引。

## Decision

**一、履约段采用运输委托是一种运营决定，声明（段、委托、服务方）（机制；种类是产品角色模型）。**

- 决定种类 `FULFILLMENT_SEGMENT_COMMISSION_ADOPTION`（ADR-0202 决定一已列），口 `POST /transport-fulfillment-segment-commission-adoptions`，挂操作者渠道的「运营决定」能力面。租户只取自认证结果。
- 前置条件逐格判，不满足的各答一格业务结果（HTTP 200，ADR-0022），续办两两不同，不并成`未受理`（ADR-0029）：段在册（`SEGMENT_NOT_FOUND`）→ 委托在册（`COMMISSION_NOT_FOUND`）→ 登记方声明的服务方等于委托的服务提供方（`PROVIDER_MISMATCH`，带委托的服务提供方）→ 委托未取消（取消早于段的首个参与答 `COMMISSION_NOT_ADOPTABLE`，不早于它答 `COMMISSION_CANCELLED_AFTER_START`，见决定三「采用与取消的先后」）→ 段有当前参与（`SEGMENT_WITHOUT_EFFECTIVE_PARTICIPATION`）→ 段的首个参与不早于委托提交（`SEGMENT_STARTED_BEFORE_COMMISSION`）。他租户名下的同名段与委托按本租户的键读不回，落「不在册」那一格。
- 服务方排在取消之前：服务方对不上多半是指错了委托，先答「已取消」会让登记方去处理一份本不相干的委托。
- 服务方由登记方声明，不从委托抄过来：抄过来就核不出指错了委托。服务方、协议快照与采购责任法人的权威仍是委托那一行，连线上另存的那一份是声明原样。
- 一段至多采用一份委托：同段原样再交答 `EXISTING_ADOPTION`，换一份委托或换服务方答 `ADOPTION_CONFLICT` 并回显在册那一份，不按最后到达顶替。已有与冲突判在两项读之前。同一份委托可以被多段采用。
- 采用之后不改、不解除：册上只有插入。没有反向解除采用的面（风险点 2）。

**二、采用即标委托开始，开始依据取已采用各段首个参与里最早的那一条（机制）。**

- 首个参与是该段入场最早的那一条当前参与：被替代的版本与失效版本不算（ADR-0112 决定一、四，它们记的入场已被更正），已离场的照算；同刻按入场依据字面序取第一条。开始依据是那一条的入场依据，开始时刻是它的入场时刻，不取采用登记的时刻——采用常常晚于实际开始（风险点 1）。
- 委托尚未开始时由这一次采用标开始；已开始、但这一段的首个参与更早的，开始前移到这一条（依据与时刻一起换）；不更早的原样不动。先被采用的段未必先开始，委托下的运输从最早那一段开始，开始取得偏晚会把开始之后为剩余范围提出的订舱读成开始之前提出、取消不了（决定三的订舱判法）。只前移、不后移：后移就是让一段已经开始过的运输「还没开始」，UC-TF-004「一致性、幂等与并发」的「已发生事实不回退」挡它。「开始」这一转换仍只发生一次，前移换的是依据与时刻，委托始终是已开始。每一段在委托下的开始依据与时刻另记在连线上，是这一段自己的事实。
- 连线先落、委托后写，同一笔事务。首次开始的写口只认尚未开始也未取消的行，前移的写口只认已开始、在册开始晚于这一次的行（`started_at > 新开始`，两段并发前移时只有更早的那一次落得上）。写的那一刻没写上就读回，按此刻的委托用同一个领域判断重判一遍：另一段先开始了它，不更早就照常采用、更早就前移；先被取消了，按决定三「采用与取消的先后」答两格之一。采用之外的任一格都整笔回滚，最后这两格靠回滚撤掉已落的连线。
- 采用不入队：今天没有它的消费者（计划「跨票通则」X3）。运输收费发生项登记按段回读连线（票 demo-chain-gaps/28）。

**三、取消委托、取消订舱的面就此开放，已开始答 `TRANSPORT_ALREADY_STARTED`（机制；判法是产品策略）。**

- 两种运营决定 `TRANSPORT_COMMISSION_CANCELLATION`、`BOOKING_REQUEST_CANCELLATION`（ADR-0202 决定一已列），口 `POST /transport-fulfillment-transport-commission-cancellations`、`POST /transport-fulfillment-booking-request-cancellations`。取消时刻由提交方给（ADR-0023）。
- 委托：已开始答 `TRANSPORT_ALREADY_STARTED` 并回显开始依据，续办是按事实形成中断、改降、折返等实际结果；已取消的重放答 `EXISTING_CANCELLATION` 回显原取消、不二取；不在册答 `COMMISSION_NOT_FOUND`。落取消那一刻撞上并发的开始或取消，读回按此刻的委托答。
- 取消是整份的：委托一旦开始就整份拒，委托层面没有「只停尚未开始的成员」的部分取消；取消成立时也不向尚未消耗的订舱、预占发出取消或释放请求（风险点 9）。
- 订舱：所属委托下的运输开始时这次订舱已经提出（申请时刻不晚于委托的开始时刻），答 `TRANSPORT_ALREADY_STARTED`——那一程可能正在用它；开始之后才提出的订舱是为剩余范围订的，可以取消（产品策略，风险点 3）。委托已取消不挡取消订舱：取消委托今天不往下游传（风险点 9），已取消委托下的订舱只能经这一口逐个停掉，挡了它这些订舱就永远停不掉。订舱不在册答 `BOOKING_NOT_FOUND`，已取消的重放答 `EXISTING_CANCELLATION`。
- 两个取消口的重放同名 `EXISTING_CANCELLATION`，不借提交口的 `EXISTING_COMMISSION` / `EXISTING_BOOKING`：取消口的调用方拿到「已有委托」读不出这是自己那一次取消的重放。
- 取消不碰已形成的承运答复、容量预占与履约事实，它们按各自的规则保留（TF CONTEXT「上游运输意图取消……」）。
- 新取消答 201，其余各格 200。

**采用与取消的先后（机制；是对 UC-TF-004 一条规则的有意收窄，理由在本条）。**

UC-TF-004「一致性、幂等与并发」要求取消与节点交接等并发时「以业务时间和有效边界裁决；已发生事实不回退」。本记录照它判谁在先，但判出冲突时只挂起待更正，不自动执行裁决：

- 取消登记时委托已开始（某一段已采用它）：答 `TRANSPORT_ALREADY_STARTED`，不比取消时刻与开始时刻。这正是 `AT-TF-092` 的「取消到达时服务已经开始」：开始是已发生的控制事实，不回退；取消时刻早于开始，说明取消没赶在运输开始之前送达，可停的范围此刻已经没有了。
- 采用登记时委托已取消：比委托的取消时刻与这一段的首个参与。取消早于段的开始，委托确实是在运输开始之前停掉的，答 `COMMISSION_NOT_ADOPTABLE`，续办是另起委托或照实声明这一段的履约主体。取消不早于段的开始（同一刻算开始在先，与订舱判法同一边界），按业务时间取消到达时运输已经开始，取消本不该成立；它成立，是因为采用登记晚于实际开始，取消那一刻本上下文还不知道这一段属于这份委托。答 `COMMISSION_CANCELLED_AFTER_START`，并排回显委托的取消时刻与段的首个参与，不落连线、不动委托；续办是更正取消或更正控制事实。段没有当前参与、或段早于委托提交，没有一个落在委托下的开始可以拿来比，按前一格答。
- 不自动撤回已成立的取消：那份取消已经以 201 答给了提交方，采用若把它翻成「已开始」，等于事后改写一份已答出去的决定，提交方不会知道，据它停掉的线下安排也不会随之恢复。把两份相抵的事实摆出来交人处置，比静默翻转安全。代价是这一格今天没有可走的更正面（风险点 8）。
- 两种并发窗口同一判法：读到委托时它尚未取消、写开始的那一刻它已被另一笔取消，读回后按上面两格答；已落的连线随整笔回滚撤掉。
- 仍随登记先后而异的一处，如实写在这里：取消时刻早于段的开始时，先登记采用，委托已开始、取消答已开始；先登记取消，委托已取消、采用答不可采用。两边都不撤回已答出去的东西，差别落在一件本上下文没有登记的事实上——取消有没有在运输开始之前送达服务方。送达了，服务方本不该开始，不可采用是对的；没送达，运输照常开始，已开始是对的。补登送达事实之前哪一边都不比另一边更对，本记录不替它判（风险点 8）。

**四、连线是声明，不是推导。**

- 本上下文不从计划履约段、实际承运商判断、交接接收方或协议有无推出哪一段该挂哪份委托（ADR-0096 决定二同一判据：可能根本不存在的东西当不了推导的输入）。
- 没有连线的段不读作自营，也不读作外包：自营与外包不由委托有无推断（TF CONTEXT「履约主体与承运证据」）。自营只能显式声明（ADR-0207 的读法不变）。
- 不在交接或收寄登记里盖章：控制事实的登记方不必知道采购组织，盖章也会把连线绑死在段成立那一刻。

**五、部分停用 ADR-0098 决定四，适用场景只限实际履约段。**

- 被停条款：ADR-0098 决定四「**今天不建**：建它等于在没有任何租户实证的情况下先定一种采购组织方式（揽收是挂在委托下、还是各自独立由第三方关系相连）」。
- 适用场景：只限实际履约段与运输委托之间的连线。停用的理由带前提——声明式连线不替租户定采购组织方式：哪一段挂哪份委托、一份委托覆盖几段，全由登记方逐段声明，产品只提供登记与判断的能力，与 ADR-0096 决定四把粒度政策留给租户同一条路。
- 揽收任务与尝试 ↔ 委托仍按 ADR-0098 决定四不建：失败的揽收尝试不成段（CONTEXT「失败结果不制造实际履约段」），段级连线够不着它，失败尝试费发生项的采购上下文仍由调用方给出（ADR-0098 决定二、ADR-0207 决定二）。成功收寄立起的段是实际履约段，可以被声明采用。ADR-0098 其余各条不变。

三类：决定一、二、四是机制，决定一的种类是产品角色模型（ADR-0146 的产品策略一侧），决定三里订舱「已开始」的判法是产品策略（内置一种），其余是机制；谁被授予哪一种、准入对照与权威区间是租户取值；演示租户哪一段采用哪一份委托是租户取值，只写在演示文档里，证据记 `S`。

## 候选与反方

- **在交接登记里盖章：外包交接成段时顺手指名委托。** 控制事实的登记方（节点、承运系统）多半不知道采购组织；盖章还把连线绑在段成立那一刻，事后补不上；交接登记是在途会话的地盘。否决。
- **从实际承运商判断或交接接收方推出委托。** 签约服务商与实际承运商可以不同（CONTEXT「签约服务商不因签约或接受运输委托而自动成为实际承运商」），推出来的连线在转包时是错的。否决。
- **开始时刻取采用登记的时刻。** 采用常常晚于实际开始，取登记时刻会让「开始之前提出的订舱」这条判据整体后移：实际开始之后、采用登记之前为剩余范围提出的订舱被读作开始之前提出，取消不了，可取消的范围凭空变小。否决。
- **委托的开始只记第一次采用，后采用的段不改写。** 先被采用的段未必先开始；开始停在较晚的那一次，与上一条同一个偏差，只是换成「第一个被登记的段」当基准。否决，改为取最早的首个参与、只前移。
- **采用与取消按登记先后裁：取消已成立，采用一律答不可采用。** 与 UC-TF-004「以业务时间裁决」相悖，且「不可采用」的续办（另起委托）在取消晚于开始时是错的：运输已经在这份委托下开始了。否决。
- **采用发现取消晚于开始时自动撤回取消、把委托改标已开始。** 理由见决定三「采用与取消的先后」第三条。否决。
- **服务方从委托抄、命令只收（段、委托）。** 指错了委托核不出来。否决。
- **一份委托只许一段采用。** 一份干线委托下两车、两班各成一段是常态。否决。
- **取消订舱只要委托已开始就一律拒。** 开始之后仍可为剩余范围订舱（ADR-0202 补记「订舱的核对次序与格」），一律拒会造出永远取消不了的订舱。否决。
- **取消订舱不看委托开始。** 已在用的舱位会被答成已取消，正是 `AT-TF-092` 禁的。否决。

## Consequences

- 代码：access-identity 三种决定种类（`DecisionFulfillmentSegmentCommissionAdoption`、`DecisionTransportCommissionCancellation`、`DecisionBookingRequestCancellation`）；TF 领域 `SegmentCommissionAdoption`、`AdoptCommissionForSegment`（交回 `CommissionStartChange`：标开始、前移、不动）、`ActualFulfillmentSegment.FirstEffectiveParticipation`、`ErrCommissionCancelledAfterStart`；编排 `AdoptSegmentCommissionHandler`、`CommissionTransportHandler.CancelBooking`，`CancelCommission` 不在册改答 `COMMISSION_NOT_FOUND`、已开始的线上名改为 `TRANSPORT_ALREADY_STARTED`，两个取消口的重放答 `EXISTING_CANCELLATION`；端口另立文件（`SegmentCommissionAdoptionStore`、`BookingCancellationStore`、`TransportCommissionStartAdvancement`），不动 `ports.go`；订舱取消写口只落取消时刻、不改写落册时刻。
- 迁移：access-identity `0015`（决定种类约束以 0013 的全集为底再加三种）；TF `0035`（采用册，主键（租户、段），段与委托两条外键，只插不改）。
- 接口：三口挂运营决定面，装配另立 `cmd/parcel-api/assemble_segment_commission_adoption.go`，采用每次一笔事务、采用成立以外整笔回滚。
- 演示种子：配置员授三种决定、准入对照与本产品权威区间各三行。
- 文档：TF CONTEXT 规则补段采用委托；UC-TF-004 补 `AT-TF-181` 至 `AT-TF-189`，「一致性、幂等与并发」补一条指向决定三「采用与取消的先后」；ADR-0098 Status 与 Links 加前向指针；演示文档第 3 步补采用与取消。

## 越权风险点

1. **开始时点取首个参与**（TF owner）。采用登记晚于实际开始时，开始时点回溯到段的首个参与，委托上取已采用各段里最早的那一条；首个参与此后被更正（替代或失效），委托上的开始与连线上的开始都不随之重派生，也不会因此后移。去处是票 [demo-chain-gaps/57](../../.scratch/demo-chain-gaps/issues/57-segment-commission-adoption-has-no-correction-and-start-does-not-rederive.md)。
2. **没有反向解除采用的面**（TF owner）。指错了段或委托只能答冲突，改不了；委托一经采用就开始，也就取消不了。去处待 owner 定：更正式的解除要同时决定「已开始」能否回退，那与 CONTEXT「已成立的实际履约段不存在取消回未开始」相邻。同去票 57。
3. **订舱「已开始」按时刻判**（TF owner）。本上下文没有逐订舱的消费关系，判据是「申请不晚于委托开始」：开始之后为剩余范围订的舱，被后到的段实际用了，仍然取消得了；开始之前订的、实际没被用的，取消不了。委托的开始被后采用的段前移之后，申请落在新旧两个开始之间的订舱由取消不了变为取消得了，此前对它答过的「运输已开始」随之过时。逐订舱的消费关系归确认订舱的容量关系（票 demo-chain-gaps/37）一并考虑。
4. **不核委托成员与段对象的范围**（TF owner）。委托列的是对象成员，段上是参与对象，两者可能在不同层级（包裹与集运单元）；本记录不核两者相交，干线委托也挂得到一个只装末端包裹的段。去处是票 [demo-chain-gaps/58](../../.scratch/demo-chain-gaps/issues/58-adoption-does-not-check-commission-members-against-segment-objects.md)，与票 49 同族。
5. **揽收段与揽收尝试不对称**（TF owner）。成功收寄立起的段可以采用委托，同一揽收任务失败的尝试仍连不上委托，失败尝试费发生项的采购上下文仍靠调用方给。
6. **委托册改写落册时刻**（TF owner）。委托的开始与取消照旧写回 `recorded_at`，委托提交口重放回显的落册时刻成了最后一次转换的时刻；订舱取消没有照这一做。与票 demo-chain-gaps/46 对池行时刻的同一问并案。
7. **种类粒度**（psb/07）。同 ADR-0202 风险点 8。
8. **采用登记之前的取消窗口**（TF owner）。实际开始与采用登记之间，业务上已开始的委托仍取消得了（答 201）；之后的采用答 `COMMISSION_CANCELLED_AFTER_START`，两份事实相抵摆在台面上，但今天没有更正取消的面，委托停在已取消、段挂不上它，只能等更正面，或运营上另起委托再采用。取消时刻早于段的开始时，结果仍随采用与取消的登记先后而异，缺的是「取消是否送达服务方」这一事实（决定三「采用与取消的先后」末条）。去处是票 [demo-chain-gaps/57](../../.scratch/demo-chain-gaps/issues/57-segment-commission-adoption-has-no-correction-and-start-does-not-rederive.md)。
9. **取消只有整份，也不往下游传**（TF owner）。委托层面没有只停尚可停止范围的部分取消，`AT-TF-092`「取消只处理尚可停止范围」只在订舱这一级做到；取消委托成立时也不向尚未消耗的订舱、预占发取消或释放请求，TF CONTEXT「上游运输意图取消只对尚未消耗的下游范围发出取消或释放请求」的下游那一步，今天要运营人员经取消订舱口、释放口逐个登记。部分取消要先知道哪些成员已随哪一段开始，与成员和段对象的关系同源，去处是票 [demo-chain-gaps/58](../../.scratch/demo-chain-gaps/issues/58-adoption-does-not-check-commission-members-against-segment-objects.md)；订舱一级「哪些已消耗」归票 demo-chain-gaps/37。

## Links

- [ADR-0098：失败尝试费发生项不在揽收编排里形成](./0098-a-failed-attempt-charge-occurrence-is-not-formed-inside-the-pickup-orchestration.md)：决定四被本记录决定五部分停用（限实际履约段）
- [ADR-0096：段身份是声明的不是推导的](./0096-a-fulfillment-segment-identity-is-declared-not-derived.md)：决定四的「连线是声明」与决定五的停用理由同源
- [ADR-0202：运输委托、订舱与承运答复的入口族与协议核对](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md)：决定一列了本记录的三种决定
- [ADR-0207：运输收费发生项由运营决定显式登记](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md)：实际履约发生项的采购上下文，段有采用连线时从委托快照取（票 demo-chain-gaps/28）
- [ADR-0112](./0112-source-correction-rederives-participation-as-a-superseding-version-on-the-same-segment.md)：替代与失效版本不作首个参与
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)、[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：前置条件各格答 200 且逐格分开
- [ADR-0023](./0023-work-fact-identity-and-time-are-minted-by-the-device.md)：取消时刻由提交方给
- [UC-TF-004](../application/transport-fulfillment/UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md)：`AT-TF-092`、`AT-TF-181` 至 `AT-TF-189`；「一致性、幂等与并发」以业务时间裁决那一条由决定三「采用与取消的先后」收窄
- 计划 `.scratch/demo-chain-gaps/tf-chain-plan.md`「02 委托、订舱、承运答复」D02-6；票 `.scratch/demo-chain-gaps/issues/25-…`、`28-…`
