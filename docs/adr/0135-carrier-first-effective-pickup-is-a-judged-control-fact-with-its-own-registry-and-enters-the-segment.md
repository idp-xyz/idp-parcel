# ADR-0135：实际承运商首次有效收寄是 `transport-fulfillment` 在合格来源之上一次显式判断形成的独立控制事实——自有登记册与出向事件、按（租户，载运对象）一条版本链、依据以（来源事实，版本）引用留在事实上；已形成即参与进入，`ParticipationEntryKind` 长第三格；待确认是带原因的版本但不提供；更正沿来源声明的更正关系换版，`parcel-shipment` 按信封所指版本取回、不取当前版

Status: Accepted（2026-09-10，通道 6 按通道 1 派单 task-f05bc5a3-a675-4157-9667-14406307a22c「用户授权代裁，TF owner 口径：硬句不改、拿不准的单列越权风险点」裁决。裁决能力边界：读过票 [lc/25](../../.scratch/label-channel-service-first-release/issues/25-external-carrier-first-pickup-triggers-label-final-judgment.md) 全文、[TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md) 全文、[CONTEXT-MAP](../domain/CONTEXT-MAP.md) 的 TF 拥有清单与 `transport-fulfillment → parcel-shipment` 那条边、[ADR-0102](./0102-external-fact-three-times-are-minted-by-ownership.md) 决定一至四、[ADR-0103](./0103-actual-carrier-judgment-is-a-versioned-record-per-segment-with-pending-as-a-value.md) 决定一与 Consequences「消费方」那条、[ADR-0112](./0112-source-correction-rederives-participation-as-a-superseding-version-on-the-same-segment.md) 的替代 / 失效版本两式（经 `FulfillmentParticipation` 头注）、[ADR-0133](./0133-delivery-condition-reference-is-the-acceptance-time-commercial-resolution-reference.md) 的 Status 行形、`internal/transportfulfillment/domain/actual_fulfillment_segment.go` 的 `ParticipationEntryKind` / `ParticipationEndKind` / `FulfillmentParticipation` 头注、`internal/transportfulfillment/domain/effective_delivery.go` 全文、`internal/transportfulfillment/adapters/postgres/effective_delivery_handoff.go` 全文、`internal/transportfulfillment/domain/offsite_pickup.go` 全文、`internal/transportfulfillment/domain/actual_carrier_judgment.go` 的 `CarrierEvidenceSource` / `CarrierSubject`、`internal/transportfulfillment/application/enter_fulfillment_segment.go` 的 `enterFulfillmentSegment` 头注、`internal/transportfulfillment/ports/ports.go` 的 `EffectiveDeliveryRegistrations.FindByKeyAndVersion` 头注、`internal/parcelshipment/domain/label_service_final.go` 的 `CarrierTrackingFactReference` / `CarrierFirstEffectivePickupSpec` / `ReferenceCarrierFirstEffectivePickup`、票 lc/16 与 lc/24 票面、lc spec。没读 `register_offsite_pickup.go` / `register_transport_handover.go` / `rederive_fulfillment_participation.go` 全文、TF 迁移文件、PS `judge_label_service_final.go` 与 `adopt_on_effective_delivery.go` 全文、`docs/wooolink/` 与 `docs/reference/xls/` 下任何客户文件。）
Date: 2026-09-10

## Context

`parcel-shipment` 面单渠道服务的默认终局规则是「除已经形成的有效取消结果外，实际承运商首次有效收寄即形成终局」（PS CONTEXT、PC CONTEXT、GLOSSARY 三处同一句），PS 生命周期写的是「`transport-fulfillment` 提供实际承运商首次有效收寄事件 → 面单渠道服务非取消终局结果」，CONTEXT-MAP `transport-fulfillment → parcel-shipment` 那条边写「运输履约提供……实际承运商首次有效收寄……证据冲突或实际承运商待确认时不得提前触发该边界」。PS 半边的形状已定：`JudgeLabelServiceFinalCommand.FirstEffectivePickup` 收 `CarrierFirstEffectivePickupSpec{Fact, Version, EffectiveAt}` 三件，编排把 `EffectiveAt` 当终局的生效时间；票 lc/25 把 PS 侧 inbox 消费者与处理方适配器照有效交付那一路写好了形状。

它等的那封信今天不存在。票 lc/25 量得：TF 十种出向事件里没有收寄那一种；`internal/transportfulfillment` 里的「有效收寄」只有 `OffsitePickup`（自营场外揽收）；`ParticipationEntryKind` 封闭二格（场外揽收、`已交接`权威交接），头注写「刻意没有第三格——扫描、装载、订舱确认都立不起参与」；`actual_carrier_judgment.go` 把外部承运轨迹事实当合格证据，但它答的是「这一段是谁在承运」而不是「该对象已被承运商收寄」。TF 唯一与外部承运商有关的出向信封是 `transport-fulfillment.external-carrier-tracking.judged`，而 TF CONTEXT「外部承运轨迹事实」Rules 明写它「本身不构成实际承运商首次有效收寄……判断仍按各自规则、结合其他证据另行形成」，且「认领不解释、不映射原始状态词——对它的解释是另一次显式判断」。所以 PS 直接消费那封信、把 `{fact, version, effectiveAt}` 折成收寄，就是 PS 替 TF 作「这条状态词算收寄」的判断——lc/11 Answer 那句「`external-carrier-tracking` 信封的 PS 侧 inbox 消费者」是误读，lc/25 已改口。

CONTEXT 对这一事实的硬句有四条：「必须明确关联载运对象、实际承运商、业务发生时间，并表达其已经接收实物或取得运输控制。合格来源可以包括承运商直接收寄扫描、承运商收货凭证、权威运输交接结果或保留原始承运来源的可信渠道回传」；「单个满足规则的证据可以形成有效收寄……来源冲突且尚不足以裁决时保持待确认，不触发实际承运商识别、取消权结束或其他依赖有效收寄的边界」；「面单生成、渠道受理、预报成功、订舱接受、舱单建立、电子数据接收和车辆到场均不构成」；以及段的成立句「载运对象通过有效收寄或权威交接进入运输方控制时，其履约参与关系和适用实际履约段才成立」。ADR-0103 读第一句时已把「载运对象」定为证据的关联对象、把实际承运商判断定为段级独立聚合，并在 Consequences 写下「PS 面单渠道服务的终局判断读的是收寄事实而不是判断——两条读路不合并，判断待确认不阻塞终局」。

用三个场景试过候选形状：

- **面单渠道包裹被承运商揽走。** 租户为包裹向渠道买了面单，客户把货交给承运商 X，X 的轨迹源报来一条原始素材，TF 认领成外部承运轨迹事实 F/v1、按 X 已登记的规则判出有效时间。这时对象在 TF 里没有任何参与关系——没有自营揽收、没有交接。CONTEXT 段成立句说「通过有效收寄……进入运输方控制时……才成立」：X 取得控制这件事若不成为一条控制事实，X 的实际履约段就永远立不起来，X 后面的「在途」「派送中」轨迹事实无段可归，X 的实际承运商判断也无处铸首版（ADR-0103「段成立时即形成首个判断版本」）。所以「首次有效收寄」不能只是发给 PS 的一封信，它得是 TF 自己的控制事实——与 `OffsitePickup` 同一层，不与 `ExternalCarrierTrackingFact` 同一层。
- **网络服务包裹先由自营揽收再交给承运商。** 自营快递员场外揽收（`EnteredByOffsitePickup`），随后在节点凭`已交接`权威交接给承运商 Y（`EnteredByTransportHandover` 立 Y 的段），Y 的轨迹源随后报来「已揽收」。这条轨迹事实若也形成一条「首次有效收寄」，会撞两件事：Y 的段已由交接成立，再进一次是 `ErrObjectAlreadyParticipating`；PS 那头会收到一封对网络服务包裹无意义的面单终局触发。CONTEXT-MAP 那条边写的是「两种判断也不能合并」——场外揽收走有效网络收寄那一路，首次有效收寄走面单终局那一路。所以「首次」按 TF 自己的控制链判：对象已有当前有效参与时，新到的承运证据不构成首次有效收寄，只进实际承运商判断当依据。
- **轨迹源事后更正了那条素材。** X 的源声明 F/v2 更正 F/v1（时间改了，或状态词改成「信息已接收」）。收寄事实已形成、PS 已据它形成终局并结束取消权。ADR-0112 对参与关系的答法是替代版本 / 失效版本两式，CONTEXT 的总句是「保留原事实和原判断，形成失效、替代及重新派生结果」；票 lc/24 的教训是消费方若按键读当前版，信封所指与读回的不是同一代。所以收寄事实按版本链换版、原版本一字不动、每版回指前版，PS 按信封所指版本取回；更正后仍是收寄 → 替代版本，不再是收寄 → 失效版本；两种都提供给 PS，PS 怎么重派生终局是 PS 的事。

## Decision

**一、立新一类事实「实际承运商首次有效收寄」，不是外部承运轨迹事实加一列，不是轨迹事实的新版本，也不是把 `OffsitePickup` 改名复用。** 它必须能从四种合格来源中的任一种形成（CONTEXT 硬句列的四种，与 `CarrierEvidenceSource` 四格同集），轨迹事实只是其中一种，所以它不能长在轨迹事实上；它的成立要件（在册承运主体、控制取得、业务时间、首次）与 `OffsitePickup` 的（任务、尝试、地点、控制依据、执行方）是两套，硬塞进同一类型会让每一格都变成可缺席。形取有效交付那一路：`EffectiveDelivery` 是在尝试结果之上一次判断形成的事实，自有登记册与 outbox 事件，成立即结束参与；收寄与它对称——在合格来源之上一次判断形成，自有登记册与事件，成立即开始参与。

**二、事实固定五件，依据以引用留在事实上，不拷来源内容。** 租户；载运对象；实际承运商——`CarrierSubject`（ADR-0103 决定三两支，在册身份引用，本上下文不铸身份）；业务发生时间；判断形成时间；依据的来源事实引用（一条或多条，每条带来源种类与**版本**——依据是「F 的第 v1 代」而不是「F」，更正才有办法回指到被更正的那一代）。TF 另铸事实身份，与载运对象分开保存（同 ADR-0102 决定四「所有者自己的事实身份另铸」）。一个载运对象至多一条版本链——「首次」由此在登记册上成立，不靠字段。

**三、业务发生时间取自依据、不由本上下文铸；依据是外部承运轨迹事实时取它已判断的有效时间。** CONTEXT 硬句要的是「业务发生时间」；对交接结果与收货凭证它就是那条事实的业务时间；对外部承运轨迹事实，源给的发生时间是源的话（ADR-0102 决定二），本上下文判过的有效时间才是「这条外部状态从何时起对本仓有效」（决定三）——取消权从何时起结束，答的正是后者。**有效时间待判断的轨迹事实版本不得作为依据**（与「待判断的事实不提供给 `visibility-exception`」同一条纪律）。判断形成时间由本上下文记，与业务发生时间分列——两者相等是巧合不是规则。PS 侧 `CarrierFirstEffectivePickupSpec.EffectiveAt` 读的就是这一格的业务发生时间；名字里的「有效」说的是它经过了本上下文的判断，不是第二个时间。

**四、判断的输入与结果封闭。** 输入：一条合格来源事实引用（带版本）、它指名的承运主体（在册引用，或名称素材）、以及「该证据表达已接收实物或取得运输控制」这一读法。读法的来源只有两种，与 ADR-0102 决定三同形：所有者就这一条显式给出；或依据该轨迹源已登记并带版本的**收寄判读规则**形成，事实上记下采用了哪一版规则。规则的内容（哪些状态词表达取得控制）是 `PAR-INT-02` 实例半边，本记录一个都不拟；没有规则时这一路如实答「规则未配置」、不形成任何版本——那不是待确认，待确认是判过了但不够。结果封闭四格：**已形成**（首登）；**待确认**（原因封闭两支：来源冲突、承运主体身份未登记）——带原因的版本，不构成收寄、不进段、不提供，身份登记后凭同一依据形成新版本，冲突由人裁不由到达先后裁（与 ADR-0103 生命周期同句）；**不构成**（证据不表达取得控制）——不形成版本，证据留在原处；**非首次**（对象已有当前有效履约参与）——不形成版本，证据作为实际承运商判断的依据走它既有的路。「无合格证据」不是收寄的待确认原因：判断是被一条证据触发的，没有证据就没有这次判断。

**五、已形成即参与进入：`ParticipationEntryKind` 长第三格。** 段成立句里的「有效收寄」不只是场外揽收，头注「刻意没有第三格」的理由是「扫描、装载、订舱确认都立不起参与」，而已形成的首次有效收寄不是扫描，是本上下文判过「取得运输控制」的控制事实——它正是那句硬话许可的那种进入。进段走 `enterFulfillmentSegment` 同一道门（进段是派生的一侧、失败留续办不回滚来源；段首登后同笔铸实际承运商判断首版——收寄要求承运主体在册，所以首版是已识别，依据即收寄的依据），入场依据引用指向（收寄事实，版本）。段引用照今天各调用方的纪律：由登记方（这里是作出判断的一方）声明，缺席时不进段也不算失败；段身份由谁铸至今没有裁决，本记录不代裁（越权风险点 1）。参与的重新派生走 `rederiveFulfillmentParticipation` 同一道门：收寄替代版本 → 替代参与版本，收寄失效版本 → 失效参与版本（ADR-0112 决定一 / 四）。

**六、版本链与更正只沿来源声明的更正关系走。** 一条链在一个事实身份下追加、每版回指前版、原版一字不动。替代 / 失效版本只由依据的来源事实被更正触发，且被更正的版本必须恰是当前版本的依据（同 ADR-0112 决定二的判据）；更正后仍表达收寄 → 替代版本（新时间、新承运主体或两者），不再表达 → 失效版本。链尾失效即该对象当前无首次有效收寄，再次形成从失效版本长出新版本，不回退。**已形成之后另一来源到达相反证据不收回收寄**：那是段级实际承运商判断的来源冲突（ADR-0103「已识别或待确认 → 待确认（来源冲突）」），判断待确认不阻塞终局（ADR-0103 Consequences 原句）。本上下文不从到达先后、发生时间先后或状态词推断谁取代谁（CONTEXT「外部承运轨迹事实」Rules 原句）。

**七、出向事件与 PS 消费面的最小契约。** 事件类型 `transport-fulfillment.carrier-first-effective-pickup.registered`，载荷指针式四维 `{tenantId, fact, version, object}`——票 lc/25「前提」列的四件一件不少、一件不多；事件 ID 取（租户 + 事实 + 版本 + 类型段），版本必须在 ID 里（`effectiveDeliveryEventID` 头注的教训：少了版本两代算出同一个字符串、第二份被 `EnqueueOnce` 静默吞掉）；分区键取（租户 + 载运对象 + 类型段），一个对象的收寄链在口内保序（`effectiveDeliveryPartitionKey` 同一条理由）。**只有已形成、替代、失效三种版本入队；待确认版本到 handoff 响亮拒绝**（`OutboxExternalTrackingFactHandoff` 对待判断版本的同一道门）。只读取回口按（租户，事实，版本）交回**指名那一代**（`EffectiveDeliveryRegistrations.FindByKeyAndVersion` 的形），记录带载运对象、承运主体、业务发生时间、版本种类（首登 / 替代 / 失效）、回指的前版、依据引用；PS 适配器按信封所指版本取、不取当前版（lc/24 教训），键与本体不符或取回的是待确认版本 → 记录不一致那一格。PS 领域 `CarrierTrackingFactReference` 头注「指名 transport-fulfillment 拥有的一条外部承运轨迹事实」自本记录起不成立，随 lc/25 改口指本事实，类型名是否随之改归实施时判。

**八、触发形状：显式判断是一个应用层入口，规则那一路是异步的一拍。** 显式判断（人就一条证据给出读法）与规则判读（按已登记规则）汇入同一个形成收寄的处理方；规则那一路由已判断有效时间的轨迹事实触发、在下一拍形成（与 CONTEXT「触发形成任务是异步的一拍：控制事实先如实落库，任务在下一拍形成」同形，ADR-0114），不塞进认领或有效时间判断的事务里——认领是认领，判读是另一次判断，两件事写在一笔里就分不开「认领过」与「判过收寄」。今天没有任何一家真源、没有任何一版规则，所以规则那一路今天形成不了任何版本；显式那一路是唯一能跑通的，机制票据此先立显式入口，规则目录与节拍随第一家真源另立（lc/20 / lc/22 同一条理由）。

## Consequences

- TF CONTEXT 加「实际承运商首次有效收寄」词条、「履约主体与承运证据」Rules 两句、Lifecycles 一节、Boundaries 拥有清单一项（随本记录同笔）。CONTEXT-MAP TF 拥有清单已有「实际承运商收寄、交接和交付事实」、`transport-fulfillment → parcel-shipment` 那条边已写「实际承运商待确认时不得提前触发」，两处核过不动。
- TF 地盘实施票 [lc/31](../../.scratch/label-channel-service-first-release/issues/31-carrier-first-effective-pickup-fact-registry-and-handoff.md)：领域聚合与版本链、`ParticipationEntryKind` 第三格与登记册解析、登记册与迁移、outbox handoff、显式判断处理方与进段 / 重派生挂点、在线登记口。PS 侧 lc/25 Blocked by 31，「要裁的」1 由本记录答。
- `ParticipationEntryKind` 头注「刻意没有第三格」那句随 lc/31 改写为「第三格是已形成的首次有效收寄，不是扫描」；`participationEntryKindFrom` 与重建门同步认第三个词。
- 不在本记录内：收寄判读规则目录的形状与登记口（随第一家真源，照 lc/19 有效时间规则目录的形）；规则那一路的节拍（照 lc/20）；PS 收到失效版本后终局怎么重派生（PS owner，见越权风险点 4）；有效交付能不能同样在外部承运轨迹事实之上判出（另一题，同形可循但不在此裁）；段引用由谁铸（越权风险点 1）；UC-PS-004 依据表那行括注「（外部承运轨迹事实）」与本记录不一致，归 PS owner 随 lc/25 改口（越权风险点 5）；GLOSSARY 是否补跨上下文词条归推送方核。

## Alternatives considered

- **PS 直接消费 `external-carrier-tracking.judged`，把状态词折成收寄。** 否决：违反 TF CONTEXT「外部承运轨迹事实本身不构成实际承运商首次有效收寄」与「认领不解释、不映射原始状态词」两句，也让 X 的实际履约段永远立不起来（第一个场景）。lc/11 那句已由 lc/25 改口。
- **给外部承运轨迹事实加一列「构成收寄」或加一种版本。** 否决：收寄必须能从四种来源中的任一种形成，轨迹事实只是其一；加在它身上的那一格对交接结果与收货凭证无处可长。且 ADR-0005 分来源事实与有效事件，轨迹事实是前者、收寄是后者。
- **复用 `OffsitePickup`，把外部承运商揽收登成一次场外揽收。** 否决：两套成立要件（任务 / 尝试 / 地点 / 执行方 vs. 在册承运主体 / 控制取得读法 / 首次）不重合，合并后每格可缺席；CONTEXT-MAP「两种判断也不能合并」；PS 会把它解释成有效网络收寄而不是面单终局触发。
- **不加第三格，收寄事实只发给 PS、不进段。** 否决：段成立句「通过有效收寄……进入运输方控制时……才成立」说的就是它；不进段则承运商的段无从成立、后续轨迹无段可归、实际承运商判断无处铸首版。记为越权风险点 2 供 owner 复核第三格的命名与范围。
- **待确认不落版本，只在触发结果里答。** 否决：CONTEXT「保持待确认」是一个状态不是一次返回值，「为什么取消权还没结束」得有人答得出；ADR-0103「未知也是一个版本」同一条理由。
- **业务发生时间取源给的发生时间。** 否决：那是源的话不是本上下文判过的话（ADR-0102 决定二 / 三）；取消权从何时起结束按本上下文判过的有效时间答。记为越权风险点 3。
- **已形成后另一来源的相反证据使收寄回到待确认。** 否决：终局已据它形成、取消权已结束，靠一条新证据（不是更正）收回边界是「按消息最后到达覆盖」（PRODUCT-BASELINE 禁句）；ADR-0103 已把这种冲突安置在段级判断里且明说不阻塞终局。
- **规则判读塞进认领或有效时间判断的同一事务。** 否决：认领与判读是两次判断（CONTEXT 原句），写在一笔里就分不开「认领过」与「判过收寄」；异步一拍有 ADR-0114 的先例。
- **规则未配置记为待确认版本。** 否决：待确认是判过了但不够，未配置是没人判——两格恢复动作不同（ADR-0029），前者等身份登记或人裁，后者等实例参数。

## 越权风险点

1. **段引用由谁铸。** 今天各调用方都不给段号、`enterFulfillmentSegment` 缺席即不进段，本记录照这条纪律让判断方可选地声明段引用。若 owner 认为承运商首次收寄这一路应由 TF 自己按收寄事实身份铸一个段（一段即该承运商的整段保管），改的是 lc/31 做法里进段那一步的一行，事实与事件形状不变。
2. **第三格的命名与范围。** 我把它定为「已形成的首次有效收寄」一格；若 owner 认为应更一般地叫「有效收寄」并让场外揽收也归入（即改造现有第一格），那是对封闭集既有值的改写，超出本记录，我没取。
3. **业务发生时间取依据的有效时间而不是源发生时间。** 按 ADR-0102 决定二 / 三推得；若 owner 认为取消权边界应锚在源给的发生时间、有效时间只管可见性投影，改的是决定三与 lc/31 构造门的一格，PS 契约不变（仍读一格时间）。
4. **失效版本提供给 PS 之后终局怎么重派生。** 我只裁 TF 这一半（失效版本入队、PS 按版本取回）；PS 那一半——已据收寄形成的非取消终局在依据失效后是否形成新的采用判断版本回指前版（ADR-0117 的形）还是另有格——归 PS owner，随 lc/25 实施时判。
5. **UC-PS-004 依据表「面单渠道服务非取消终局结果」那行括注「（外部承运轨迹事实）」。** 与本记录决定一不一致；改口归 PS owner，我没动 PS 的用例文档。
6. **待确认原因封闭两支。** 我按 ADR-0103 的三原因去掉「无合格证据」得两支；若 owner 认为「证据不表达取得控制」也该留痕为一种待确认（而不是「不构成」不落版本），改的是决定四的一格与 lc/31 的一格。

## Links

- [TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)：「实际承运商首次有效收寄」词条、「履约主体与承运证据」Rules、「实际承运商首次有效收寄」Lifecycles（本记录的落地处）；「外部承运轨迹事实」Rules 那两句硬话
- [CONTEXT-MAP](../domain/CONTEXT-MAP.md)：`transport-fulfillment → parcel-shipment` 那条边（核过不动）
- [ADR-0102](./0102-external-fact-three-times-are-minted-by-ownership.md)：三时间归属、两种判断来源（决定三的形本记录决定四 / 八照搬）
- [ADR-0103](./0103-actual-carrier-judgment-is-a-versioned-record-per-segment-with-pending-as-a-value.md)：主体粒度、待确认是值、消费方两条读路不合并
- [ADR-0112](./0112-source-correction-rederives-participation-as-a-superseding-version-on-the-same-segment.md)：替代 / 失效参与版本（本记录决定五 / 六沿用）
- [ADR-0114](./0114-delivery-dispatch-is-triggered-by-entering-a-declared-delivery-segment-and-pulls-requirements-by-reference.md)：异步一拍的触发形
- [ADR-0117](./0117-same-source-correction-forms-a-superseding-adoption-version-chained-to-the-current-responsibility-start.md)：PS 侧同来源更正的采用版本链（越权风险点 4 所指）
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格
- 票 [lc/25](../../.scratch/label-channel-service-first-release/issues/25-external-carrier-first-pickup-triggers-label-final-judgment.md)（问题出处、PS 半边）、[lc/31](../../.scratch/label-channel-service-first-release/issues/31-carrier-first-effective-pickup-fact-registry-and-handoff.md)（TF 半边实施）、[lc/16](../../.scratch/label-channel-service-first-release/issues/16-external-tracking-fact-adoption-executor.md)（`03 → 16` 先落文再实现的同一条路）、[lc/24](../../.scratch/label-channel-service-first-release/issues/24-source-correction-version-refused-as-second-responsibility-start.md)（按版本取回的教训）
