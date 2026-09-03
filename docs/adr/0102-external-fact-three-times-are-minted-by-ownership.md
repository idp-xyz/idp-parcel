# ADR-0102: 外部事实的三个时间按归属分铸——接收时间由本仓铸，发生时间只能由源给且缺则不收编，有效时间由所有者显式判断；这是 ADR-0023 在外部源上的适用解释，此后每一个外部源都继承它

Status: Accepted  
Date: 2026-09-03

## Context

[ADR-0088](./0088-label-channel-service-enters-the-first-release-service-forms.md) 要求「末端渠道轨迹回传的事实收编（先由事实所有者收编，不直插投影）」。[轨迹源盘点](../../.scratch/label-channel-service-first-release/tracking-source-seam-inventory.md)第二段查明后半句已由类型守住——`visibility-exception` 的 `SourceContext` 是封闭五值，`MilestoneClassification` 只能由 `AcceptedSourceFact` 构造，外部数据没有绕进投影的类型路径——而前半句整段缺席：没有任何上下文接受「外部系统报来的状态」。

[票 03](../../.scratch/label-channel-service-first-release/issues/03-external-tracking-fact-owner-and-time-minting.md) 裁定收编方为 `transport-fulfillment`，且新立一类「外部承运轨迹事实」而不塞进自营作业事实。裁决同时撞上一条本仓已有的硬约束：

> `AcceptedSourceFact` 要求 `OccurredAt` / `EffectiveAt` / `ReceivedAt` **三个时间齐备**，而外部轨迹源常常只给一个。

[ADR-0023](./0023-work-fact-identity-and-time-are-minted-by-the-device.md) 写的是「凡是只有在现场才能知道的，由设备记录；服务端只能校验和保留，不能重造」，并把外部事实的身份与发生时间一并划进「不代铸」。它立在一线作业设备上，没有说外部轨迹源报来的一条状态，那三个时间里哪些算「只有在现场才能知道的」。**这一问不答，收编执行器就只能自己编一个规则**——而最顺手的两种编法（拿接收时间顶发生时间、让有效时间默默等于发生时间）各自静默抹掉一格区别。

它必须现在定、且定成 ADR，理由有二。其一，[票 15](../../.scratch/label-channel-service-first-release/issues/15-tracking-source-inbound-port.md) 已经按这条口径落了端口：`ports.SourceTime{Given, At}` 让「源未给」在类型上与零值时间分开，`TrackingMaterial` 上只有 `OccurredAt`（源给）与 `ReceivedAt`（本仓铸），**刻意没有 `EffectiveAt`**。端口形状已发布，此后每一家轨迹源适配器都按它写，口径若在收编侧改了，端口得跟着改。其二，这条解释不只管面单渠道的轨迹：任何被某个上下文收编为自己来源事实的外部报文——承运商直连、聚合平台、将来的推送回调——都要过同一道三时间的门，各答一次就会分叉。

## Decision

**外部事实的三个时间不是同一种东西，按归属分三条铸法。** 下列各条是 ADR-0023「不代铸」在外部源上的适用解释，不是对它的修改。

**一、`ReceivedAt` 由本仓铸。** 它答的是「本仓什么时候拿到的」，那本来就是本仓自己的事实，铸它不构成代铸。它是端口这一层唯一铸的时间。

**二、`OccurredAt` 只能由外部源给出；源未给，这条素材就不收编。** 它就是 ADR-0023 所指的发生时间，没有任何填法不算代铸。**这一格不留例外**：收不了的如实留痕为「源未给发生时间」，不进事实库，不进投影，也不许拿 `ReceivedAt` 顶替——顶替之后迟到轨迹与实时轨迹在类型上就分不开了，而 `AcceptedSourceFact` 把三时间分立正是为了防这件事。

**三、`EffectiveAt` 由收编方（事实所有者）铸，但必须作为一次显式判断。** 它答的是「这条外部状态从何时起对本仓有效」，是所有者自己的业务判断，不是外部事件的属性——ADR-0023 禁的是替外部事实补它自己的时间，`EffectiveAt` 不是外部事实自己的时间，因此不在其射程内。**但不得默默等于 `OccurredAt`**：默认相等会让「所有者判断过」与「没人判断过」长成同一张脸。判断的来源只有两种：所有者就这一条显式给出；或依据该源已登记并带版本的规则形成，事实上记下采用了哪一版规则。两者都没有时，事实保持「有效时间待判断」，**留在所有者手上，不提供给 `visibility-exception`**。

**四、身份同理分两路。** 外部事件标识由源给，本仓不代铸；所有者自己的事实身份另铸，两者分开保存、不互相顶替。源未给事件标识的素材不按内容摘要判重——按摘要判重等于替源发明一个身份，那也是代铸；这类素材如实带着空标识交所有者留痕。

**五、原始状态词在收编这一步不解释、不映射。** 「外部承运方报了 `DELIVERED`」是事实，「它对本仓意味着已交付」是判断，前者收编，后者归所有者按该源已登记的状态词规则另行作出。收编不因此推迟，也不因此替源猜一个含义。

**六、适用范围是每一个被收编的外部源。** 本记录立在 `transport-fulfillment` 的外部承运轨迹事实上，但三条铸法与两路身份不依赖承运这一领域；此后任何上下文收编任何外部报文，先按本记录分归属，再谈填法。已按各自 `UC-*` 分层接受外部结果的既有路径（如关务的外部监管结果）不因本记录改写；若发现与之冲突，走 supersede，不就地改。

本记录**不定**「有效时间规则」的形状与取值、各源的状态词表、留痕的存储形式——前者是所有者的模型细节，后两者属实例半边（`PAR-INT-02`）。

## Consequences

- **有一类外部数据永远进不了事实库：没有发生时间的。** 这是刻意让出的。它不是数据丢失——素材如实留痕，所有者随时能看见「这家源给了多少条没有时间的状态」——而是拒绝让一个源的缺陷变成本仓事实里一段分不出真假的时间。
- **收编与提供给投影之间多出一格。** 「有效时间待判断」的事实存在于所有者手上而不在投影里；一家源在有效时间规则登记之前，它的轨迹在客户可见面上是空白而不是错的。空白是真话。
- **有效时间规则成了一条实例参数**，与账号、地址、状态词表同列（`PAR-INT-02`）。机制半边现在就要能表达「该源尚无有效时间规则」，并拒绝任何默认值——包括「等于发生时间」这个最像无害的默认值。
- **里程碑映射在状态词规则到位前只能把外部承运轨迹事实留为未归类。** `SourceFactKind` 是按（源上下文，事实类型）登记的映射键；外部承运轨迹事实只有一个类型词 `external-carrier-tracking`（定义在 [transport-fulfillment/CONTEXT.md](../domain/transport-fulfillment/CONTEXT.md)），状态词不进类型词。`visibility-exception` 既有的 `LeaveUnclassified` 就是这一格的正确落点；何时、如何把状态词解释进里程碑是另一次判断、另一张票，本记录只保证它不会在收编这一步被偷做。
- **两种到达形态共用同一份约束。** 拉取与推送的差别只在素材怎么到达；三时间的归属、身份的两路、状态词不解释，在两种形态上一字不改。
- **与 ADR-0023 不冲突，且互相加固。** 那条记录让设备成为可信来源的一部分并如实标出「设备可以伪造时间」这处敞口；本记录同样让外部源成为其发生时间的唯一来源，敞口同形——源方凭证校验属 ADR-0055 的接入面与 `PAR-INT-02`，本记录不解决它，也不得被读成已经解决。
- 新增一项判断成本：每接一个外部源，先分清它报来的每个时间字段是「事件发生时间」还是「源方处理时间」。判错的方向是把后者当前者收编——那不违反本记录的字面，却让本记录防的那格区别在源那一侧就已经丢了。

## Alternatives considered

- **`OccurredAt` 缺失时以 `ReceivedAt` 填入。** 否决：最顺手，也最致命。它在类型上完全合法，`AcceptedSourceFact` 的构造门拦不住它，而结果是迟到一天的轨迹与刚发生的轨迹在事实库里一模一样。ADR-0023 禁的正是它。
- **`EffectiveAt` 默认等于 `OccurredAt`，所有者有异议再改。** 否决：「默认相等」与「判断后相等」在数据上是同一个值，读的人分不出哪些条目有人判断过。本仓在别处已为这一类形状立过规矩：[ADR-0096](./0096-a-fulfillment-segment-identity-is-declared-not-derived.md) 拒绝为未声明的段身份铸默认号，理由同样是默认值会让「声明过」与「没人声明」分不开。
- **没有有效时间规则时直接拒收。** 否决：那条状态确实发生过、源也确实给了发生时间，缺的只是所有者自己的一次判断。拒收等于让本仓的一项未配置吃掉一条真实的外部事实，与 ADR-0023 否决「拒收时钟偏移超阈值的事实」同一条理由。留为「待判断」且不提供给投影，既不丢事实也不冒充判断。
- **由 `visibility-exception` 在接受时补齐缺失的时间。** 否决：投影只消费已接受的事实，不是第二事实源（CONTEXT-MAP「各事实源 → visibility-exception」）。让它补时间就是让它替所有者判断。
- **在收编时按状态词把外部轨迹译成 TF 既有事实类型（如把 `DELIVERED` 译成有效交付）。** 否决两次：一次在票 03——那是在改自营事实的词义；一次在这里——状态词的含义属该源的已登记规则，是实例半边，收编时解释它等于填了一张不存在的状态码表。
- **让 `SourceFactKind` 携带原始状态词以复用既有映射登记册。** 否决：状态词是外部词表，同一个词在两家源可以含义相反，键里没有源身份就撞、加了源身份就不再是所有者命名的事实类型（票 03 第三问要求类型词由 TF 命名）。映射登记册按（源上下文，事实类型）建键这条不变量不为此松动。

## Links

- [ADR-0023：作业事实的身份与发生时间由设备签发，服务端不重签、不校正、不按接收顺序定序](./0023-work-fact-identity-and-time-are-minted-by-the-device.md)：本记录是它在外部源上的适用解释；「不代铸」的原则与「敞口如实标出」的写法都从那里来
- [ADR-0005：由来源事实形成有效事件并派生状态](./0005-source-facts-effective-events-derived-state.md)：外部承运轨迹事实是来源事实，有效时间的显式判断是它走向有效事件的那一步
- [ADR-0088：面单渠道服务进入首发服务形态](./0088-label-channel-service-enters-the-first-release-service-forms.md)：「先由事实所有者收编，不直插投影」的出处
- [ADR-0090：出向集成的结果代数按调用方的恢复动作分格](./0090-outbound-integration-result-algebra-partitioned-by-recovery-action.md)：素材到达本仓所经的出向端口契约；决定四「锚在已有身份上，不新造传输层的键」与本记录第四条同源
- [ADR-0089：一线作业过渡走受控批量导入并内置结构性拆除期限](./0089-frontline-transition-controlled-import-with-structural-sunset.md)：「导入事实带来源标记以与设备事实可分辨」与本记录「外部事实与自营事实在类型上分得开」是同一种关切
- [运输履约上下文](../domain/transport-fulfillment/CONTEXT.md)：「轨迹源」「外部承运轨迹事实」两词、规则节与生命周期节是本记录在领域语言上的落点；类型词 `external-carrier-tracking` 定义在那里
- [票 03](../../.scratch/label-channel-service-first-release/issues/03-external-tracking-fact-owner-and-time-minting.md)：收编方归属与三时间归属的原始裁决，本记录把其中难逆转的一半落成 ADR
- [票 15](../../.scratch/label-channel-service-first-release/issues/15-tracking-source-inbound-port.md)：按本口径先行落地的端口形状 `ports.TrackingMaterial` / `ports.SourceTime`
- [票 16](../../.scratch/label-channel-service-first-release/issues/16-external-tracking-fact-adoption-executor.md)：收编执行器，本记录是它开工的前置落文之一
