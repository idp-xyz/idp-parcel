# ADR-0224：先于段登记的移动事实由全程追踪在对象凭交接或场外揽收进段时补派生——按段列出在册事实，同一判据、同一幂等键，只补投影

Status: Accepted（2026-10-20，Claude Code 会话代 VE owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」，并于 2026-10-19 同意只做分诊 A 组；票 demo-chain-gaps/53，协调方代决先走「VE 在交接立段时经 TF 只读口按段补派生」这条路，评审后补上场外揽收那一路。回答 [ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 文末补记越权风险点 14；ADR-0205 正文不改，只在其 Status 行与 Links 加前向指针，不停用任何已接受记录的条款。越权风险点见文末，待 VE owner 复核，决定二、决定三的领域门与第 1、2 点同时待 TF owner 会签。）<!-- 索引注记 -->｜**谁补**：全程追踪在消费侧补，运输履约不为此重交信封、不造「段已成立」信封。**读口**：TF ports `MovementFactsOfSegment.FindBySegment` 按（租户，段）交回声明了这一段的全部在册版本，排序契约是先按事实、再按登记时刻、再按版本串，不问段在不在册，一行装不回整次报错；迁移 0044 加部分索引；适配器在 VE 消费侧。**唤醒**：`transport-handover.registered` 与 `offsite-pickup.registered` 的扇出各另起一路、另记一本账，排在同一封信上的投影之后；交接只认转出控制的那一版；只看这一次控制事实让对象进的那一段（领域门 `EnteredByHandover`、新加的 `EnteredByPickup`），只补这一件。**同判据同键**：在控取 `ObjectsUnderControlAt`（事实发生时刻、按消费那一刻段册），派生命令与移动事实那一路是同一段代码，两路谁先谁后、重投几次都只留一份；更正链逐版补。**只补投影**：不补信号、不重问关务，补出的出发没有该件的门禁读数，补问归 ADR-0205 越权风险点 2。揽收执行口（尝试级信封）与承运商首次有效收寄立的段不在本记录（票 120）；上线前已丢的出发不自愈。越权风险点含进段欠账时补不上、补出的出发不带门禁读数、执行口与承运商段不补、每次控制事实多读段与事实、停住时挡同一分区且坏行跨对象波及｜**部分停用**（2026-10-26）：越权风险点 1「进段那一半欠账时补不上」由 [ADR-0302](./0302-the-movement-fact-backfill-scans-every-segment-of-the-object-so-a-later-control-fact-heals-a-segment-entered-without-an-envelope.md) 回答（票 [demo-chain-gaps/136](../../.scratch/demo-chain-gaps/issues/136-segment-continuation-wakes-the-movement-fact-backfill.md)）——补派生按对象扫它有参与的每一段，对象经续办、存量迁移或直写段册不经信封进了段，由它的下一份转出控制的交接或对象级揽收补出；不取唤醒信封（运输履约今天没有续办进段那一路）。**被停条款**：决定三「只看这一次控制事实让对象进的那一段：对象一程走几段，只取参与入场依据指的正是这一次的那一段。」与同一条末句「一次控制事实至多让对象进一段，认到就停」——改为扫对象有参与的每一段。**适用场景**：只限补派生扫哪几段；决定三其余各句、触发、只补这一件、只补投影与失败分格不变，同一条中间「交接用 `EnteredByHandover`；揽收由运输履约领域新加 `EnteredByPickup`……进段、重派生与认都经它」那半句不停用（两法仍是运输履约领域的认法，只是补派生不再调用）。越权风险点 6 的大半随之有了去处。其余各条不变｜**补问已答**（2026-10-26）：越权风险点 3「补出的出发不带该件的门禁读数」由 [ADR-0305](./0305-a-departure-left-without-a-verdict-for-an-object-is-re-asked-at-its-departure-moment-when-the-object-s-control-fact-arrives.md) 回答（票 [demo-chain-gaps/121](../../.scratch/demo-chain-gaps/issues/121-departures-recorded-before-their-segment-are-never-re-asked-at-the-gate.md)）——同一封控制事实信封上另起补问一路（运输履约消费自己的交接与对象级揽收信封），按原出发时点只替这件对象再问一次关务、追加一版补问读数，补问读数里的阻断由全程追踪以那条出发为因果条件形成信号；补派生照旧只补投影、不补信号、不重问关务（决定五不变）；无条款停用，其余各条不变｜**唤醒另有一类**（2026-10-27）：决定三写的唤醒是对象级的控制事实登记信封，段里在控的若是集运单元、成员后来才识别成功，成员没有自己的控制事实，由 [ADR-0299](./0299-a-consolidation-member-identified-after-its-bag-moved-gets-the-movement-fact-backfilled-on-the-node-intake-envelope.md)（票 [docs-consistency/code/74](../../.scratch/docs-consistency/code/74-a-member-identified-after-the-movement-never-gets-that-movement-fact.md)）在节点收寄形成信封上另起一路补派生——向节点作业问这件实物曾装进哪些单元作候选，再对候选单元走按对象扫段的补派生，展开到这件包裹才补；决定四（同一判据、同一幂等键）、决定五（只补投影）、决定六（失败分格）原样引用；接上（扩）唤醒面，无条款停用，其余各条不变
Date: 2026-10-20

## Context

- 现象（读码钉 `6a05b431`，只记 `S`）：移动事实可以先于它声明的段登记，运输履约照记（ADR-0205 决定四）。全程追踪消费移动事实已形成信封时按键回读、沿声明的段取那一刻在控的对象；段此刻不在册就入账不派生（`DeriveOnMovementFactAdapter.objectsOf`）。段日后登了，没有任何东西回头让这封信重投——信封只在登记那一笔交出一次，同内容重投交的是同一个事件 ID、出队那一步吞掉。于是这一条出发永远不进投影，演示与上线判据「出发后追踪出现 `DEPARTED`」在离线补登、交接口与作业事实口各走各的设备时落空，而这种先后颠倒在一线是常态。
- ADR-0205 文末补记越权风险点 14 记下了这一格并立票 53；票面给了两条路：运输履约在段首次成立那一笔里重交声明了这一段的事实信封，或全程追踪订阅段成立、回头按段查在册事实。
- 读码（同钉）：
  - 段只从「进段」一道门成立（`enterFulfillmentSegment`），调用它的是各控制事实的登记编排：场外揽收的登记与执行、权威交接（含结束参与那一条编排里的再进段）、承运商首次有效收寄。参与的入场种类对应地是 `EnteredByOffsitePickup`、`EnteredByTransportHandover`、`EnteredByCarrierFirstEffectivePickup`。
  - 权威交接让对象进段的那一条参与，入场依据由交接键铸（ADR-0221），领域有 `EnteredByHandover` 认它是不是凭某一版交接进的段。场外揽收的入场依据由收寄版本拼成，拼写散在进段与重派生几处，领域没有对应的认法；收寄版本由本上下文的序列签发、不复用（`NextPickupResultVersion`）。
  - 全程追踪已经消费 `transport-handover.registered`（交接投影）与 `offsite-pickup.registered`（对象级揽收投影），按 ADR-0066 在消费侧展开。揽收执行那一口交的是尝试级的 `offsite-pickup.formed`，全程追踪不认（揽收消费门头注）；承运商首次有效收寄的信封只进小包托运。运输履约没有「段已成立」或「参与进入」一类信封。
  - 移动事实投影的事实引用是 `movement-fact/<对象>/<事实>`、版本取事实版本，发生与有效时间取事实的业务发生时间、接收时间取落册时刻；派生编排按（租户，来源，引用，版本）加内容摘要判重放，同键同内容答已有结果。
  - 出发的门禁读数在登记那一刻问；段不在册时读数停在未决、停点「段不在册」，没有逐对象那一格，阻断信号无从形成（ADR-0205 决定二、四）。信号那一步不看段在不在册，所以先后颠倒丢的只是投影，不丢已形成的信号。
  - `transport_movement_fact` 表没有按段的索引（迁移 0010、0026）。

## Decision

1. **补的一方是全程追踪，在消费侧补。** 运输履约不为此重交信封，也不造一类「段已成立」信封。投影怎么从源事实展开归消费方（ADR-0066），哪些事实要补、补成什么，是全程追踪的需要；让运输履约知道它，等于把投影规则搬进源上下文。重交信封还撞出队去重：同一个事件 ID 被吞掉，要补就得另造一类唤醒信封，连同分区登记与生产方合同一起加。
2. **运输履约开一个只读口：按段列出声明了这一段的在册移动事实**（TF ports `MovementFactsOfSegment.FindBySegment`，TF 适配器实现，迁移 0044 加 `(tenant_id, segment_ref, fact_ref, recorded_at, version) WHERE segment_ref IS NOT NULL` 部分索引）。排序契约：交回每一个版本，先按事实、同一条事实按登记时刻、同一时刻按版本串，重放时一字不变。不问段在不在册：段只是声明的引用、没有外键，段不在册时照样交回声明了它的事实。一行装不回整次报错，不跳过：跳过会让调用方把「漏了一条」当成「列全了」。适配器与跨上下文翻译留在全程追踪消费侧（ADR-0025）；全程追踪照这个次序逐版补，理由写在它那一侧——正常路径当初也是按登记先后逐封收到各版的。
3. **唤醒是对象级的控制事实登记信封，各另起一路、另记一本账。** `transport-handover.registered` 与 `offsite-pickup.registered` 的扇出里各加一路「移动事实补派生」，inbox 名与同一封信上的投影分开，名次紧排在投影之后、各主业务路之前（补派生同属追踪，不挡在派送发起、节点控制、路由复核与来源采用后面）。两路共用一个处理方：
   - 按信封键回读控制事实，核对与同一封信上的投影共用一处；
   - 交接只认转出控制的那一版（`已交接`，含更正成`已交接`的那一版）：拒收与待确认不让对象进段，入账收工；对象级揽收只为成功收寄形成，没有这一格；
   - 只看这一次控制事实让对象进的那一段：对象一程走几段，只取参与入场依据指的正是这一次的那一段。交接用 `EnteredByHandover`；揽收由运输履约领域新加 `EnteredByPickup`（入场种类、对象与按收寄版本铸的依据都对上），依据的拼写收进同一处，进段、重派生与认都经它。一次控制事实至多让对象进一段，认到就停；
   - 只补这一件对象：同段别的对象凭各自的控制事实进段，由各自那封信补。
   不订阅「段成立」：控制事实本身就是对象进段的权威事实，凭它唤醒同时覆盖段首次成立与后续对象加入既有段两种情形——后者同样会先后颠倒（后到的对象控制事实的业务时间早于出发）。
4. **同一判据、同一幂等键。** 对象在不在控取 `ActualFulfillmentSegment.ObjectsUnderControlAt(事实发生时刻)`，按消费那一刻段册的内容算，与 ADR-0205 文末补记的展开口径一字不差；派生命令（引用、版本、发生/有效/接收时间、更正版回指前版的替代关系）与移动事实那一路是同一段代码。两路谁先到谁派生，后到的一路、以及任何重投，都答已有结果，不出第二个条目。更正链逐版补，次序取读口交回的次序。
5. **只补投影，不补信号、不重问关务。** 读数是登记那一刻问的：先于段登记的出发，读数停在未决「段不在册」，没有阻断的那一件可交分诊；补派生不改这份读数，也不替它重问。补出的条目因而是一次没有该件门禁读数的出发，演示参考映射下与放行的出发同归 `DEPARTED`。要不要对这类出发按原时点重问关务、追加一版读数，归补问机制（ADR-0205 越权风险点 2），另立票 121。
6. **失败分格照移动事实那一路的口径，名单各路自立。** 控制事实读不回、对象的段列不出或按键读不动（那一问报错）、按段列事实读不动、派生编排未决：整封回滚、记未决待重投。按对象列得出的段按键读得动却答没有（段不能被删除）、列出的事实没声明这一段、按段列出时某一行过不了重建门（读口包住 `ErrMovementFactRecordUnreadable`，与读不动分开报）、记录与键不符：仓储不变量已破，记发布失败，不进未决名单。
7. **揽收执行口与承运商首次有效收寄立的段不在本记录。** 揽收执行那一口交的是尝试级信封、全程追踪不消费，那一口收寄的对象今天既不投影、也不补派生；承运商首次有效收寄立的是实际承运商的段，本产品的移动事实来自自营作业事实口，外部承运商的轨迹走外部追踪事实，判为不适用——移动事实声明这种段时不被拒，但没有登记方这样用。两者另立票 120。

## Consequences

- 演示与上线判据回来了：出发先登、对象后经交接或场外揽收进段，追踪里出现该出发派生的里程碑，演示参考映射下归 `DEPARTED`（AT-VE-178）；重投与两路先后到达不产生第二条（AT-VE-179）。
- 每一次`已交接`与对象级揽收在追踪侧多读对象有参与的段，以及它进的那一段上声明的事实。读口有索引，不分页。
- 交接与揽收信封的扇出各多一路；补派生停在未决重投时，同一封信上的投影那一路按 inbox 跳过，不重复派生。
- 正常路径不变：移动事实那一路遇段不在册仍入账不派生（ADR-0205 文末补记），补的一方是控制事实那一封。
- 先于段登记的出发即使本该阻断，也不形成阻断信号，追踪上显示成一次普通出发；那一格的去处在票 121。
- 上线前已经丢掉的出发不会自愈：已定稿的控制事实信封不会再投给新加的这一路，新 inbox 只对上线之后交出的信封生效。存量要补，得另做一次按段的回扫，本记录不做。
- 验收：UC-VE-002 `AT-VE-178`–`182`（补派生、两路同键、不补的各格、失败分格、更正链与不补信号），UC-TF-005 `AT-TF-273`–`274`（按段列出的读口）；AT-TF-159 那一行补一句指向本记录。

## Alternatives considered

- **运输履约在段首次成立那一笔里重交声明了这一段的事实信封**（票面第一条）。不取：重交的同一个事件 ID 在出队那一步被吞，已经入账过的那一封不会再跑，得另造一类「段已成立」唤醒信封；而且只在段首立时触发，后续对象加入既有段时的先后颠倒照丢；还要运输履约知道全程追踪的需要。
- **全程追踪订阅「段成立」或「参与进入」信封**（票面第二条的字面）。不取：运输履约没有这类信封，造一类要动生产方出口、分区登记与 Envelope ID 指纹；控制事实信封已经在全程追踪的消费面上，参与进入的权威事实正是它们。
- **移动事实那一路遇段不在册就记未决、等段**。不取：段可能永远不登（事实声明了一个没人登的段），派发的失败预算撑不到那一天，等的结果是信封进死信；且 ADR-0205 文末补记已定「段不在册入账不派生」。
- **补派生每次控制事实都扫对象在全部段上的事实**。不取作默认：读量随对象走过的段数线性增长，绝大多数是已有结果；它多出来的只是「早先某次进段时补派生没跑成」那一格的自愈，那一格另有去处（越权风险点 1）。
- **补派生连带补问关务、按原时点重问并补形成信号**。不取：补问是 ADR-0205 越权风险点 2 那一套机制的事——读不动与未决怎么重问、确定性缺陷怎么不无限重问都还没裁；在补派生里顺手问，等于在一条投影路上开第二个门禁判断入口。
- **全程追踪在消费侧自拼揽收依据串来认参与**。不取：拼写就成了两个上下文各写一份，运输履约改了拼法，认的一侧静默认不出；认法放进运输履约领域，与交接那一侧的 `EnteredByHandover` 同形。

## 越权风险点

1. **进段那一半欠账时补不上**（TF owner、VE owner）。控制事实登记时段那一半写不进（答复带续办引用 `SegmentContinuationReference`），这一次控制事实在段里没有参与，补派生那一路找不到它进的段、入账收工；日后续办把对象补进段，若不重交那封信（同 ID 被出队吞掉），那一份出发照丢。修法二选一：续办进段成功时交一封唤醒，或补派生改按对象全扫（见「Alternatives」第四条）。
2. **揽收执行口与承运商段不补**（TF owner、VE owner）。见决定七；揽收执行口那一批对象连投影都没有，是比本记录更大的缺口，票 120 一并记。
3. **补出的出发不带该件的门禁读数**（VE owner、CC owner）。那一份读数停在未决「段不在册」，实际可能该阻断；本记录不补问、不补形成信号，受限包裹在追踪上显示成一次普通的 `DEPARTED`（ADR-0205 越权风险点 2、17），票 121。
4. **补派生按消费那一刻段册算**（VE owner）。与正常路径同口径（ADR-0205 越权风险点 15）；段上参与日后被更正、更正后的起点晚于事实时，已补的那一份不撤——投影不使任何源事实失效，与正常路径同一缺口。
5. **每次控制事实多读段与事实**（运维）。整段连同全部参与读回（认入场依据要看参与）；段上声明的事实多（长段反复往返）时按段那一读变大；有索引、不分页。
6. **只补控制事实那一件**（VE owner）。同段对象凭控制事实以外的路径进段（存量迁移、手工直写段册）时不补，同第 2 点。
7. **次序靠登记时刻**（VE owner）。时钟回拨使更正版登记时刻早于前版时，补派生的次序与当初信封次序不同；替代关系由内容决定（`Supersedes`），结论不变，只是中间多一版投影。
8. **补派生停住时挡同一分区后面的信封，坏行还会跨对象波及**（TF owner、VE owner、运维）。
   - 交接信封与对象级揽收信封各按（租户、对象、口名段）分区（ADR-0074 决定二），Outbox 按分区只交出队头。补派生停在未决（对象的段或按段列事实读不动、派生编排未决），或响亮失败（段册自相矛盾、按段列出的事实没声明这一段或某一行装不回），这封信就留在队头重投，直到重投成功，或烧完失败预算落 `ABANDONED`。
   - 同一对象后面的交接或揽收信封都排在它后面：下一次交接的追踪投影、派送发起与节点控制转出，揽收的来源采用，都跟着等。补派生名次靠前，只保住同一封信上排在它前面的投影；到分区这一层，追踪会挡履约。
   - 跨对象波及：按段列事实时一行装不回就整次报错，每件凭控制事实进那一段的对象，各自的交接或揽收分区都会停在这一封上。本记录已把这一格与读不动分开（读口包住 `ErrMovementFactRecordUnreadable`，补派生归不变量已破、记发布失败），运维从「要人动手」的入口进；但它照样挂在队头烧失败预算。段行装不回仍与连不上库同记未决，与移动事实那一路按键回读同一处缺口（票 [114](../../.scratch/demo-chain-gaps/issues/114-deterministic-failures-answered-as-retryable-undecided-across-consumers.md) 已点名这几处）。
   - 分区队头阻塞本身的去处同 [ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 越权风险点 8：票 [89](../../.scratch/demo-chain-gaps/issues/89-actual-fulfillment-formation-blocks-later-envelopes-in-the-handover-and-delivery-partitions.md)。

## Links

- [ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md)：决定四（段不在册照记）、决定五与文末补记（展开口径；越权风险点 14 由本记录回答；越权风险点 2 补问机制、15、17）
- [ADR-0066](./0066-multi-object-envelope-unrolls-per-member-on-the-consumer-side.md)：消费侧循环展开
- [ADR-0221](./0221-a-handover-participation-basis-is-minted-from-the-handover-key.md)：交接参与入场依据由交接键铸，`EnteredByHandover` 认得出这一版
- [ADR-0222](./0222-composition-roots-register-endpoints-and-routes-per-feature-file.md)：新路由登在 `routes_movement_fact_backfill.go`，名次与哨兵名单写在本路
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：跨上下文适配器在消费方
- [ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)：接得住才登记
- [UC-VE-002](../application/visibility-exception/UC-VE-002-BUILD-TRACKING-PROJECTION.md)：`AT-VE-178`–`182`
- [UC-TF-005](../application/transport-fulfillment/UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md)：`AT-TF-273`–`274`；`AT-TF-159`
- [ADR-0302](./0302-the-movement-fact-backfill-scans-every-segment-of-the-object-so-a-later-control-fact-heals-a-segment-entered-without-an-envelope.md)：后继——回答越权风险点 1，补派生按对象扫它有参与的每一段；部分停用决定三一句（只看这一次控制事实让对象进的那一段），仅限扫哪几段
- [ADR-0305](./0305-a-departure-left-without-a-verdict-for-an-object-is-re-asked-at-its-departure-moment-when-the-object-s-control-fact-arrives.md)：后继——回答越权风险点 3，同一封控制事实信封上的补问那一路补读数与信号；决定五（补派生只补投影）不变
- [ADR-0299](./0299-a-consolidation-member-identified-after-its-bag-moved-gets-the-movement-fact-backfilled-on-the-node-intake-envelope.md)：后继——接上唤醒面，节点收寄形成信封上另起一路补派生，补给后来才识别成功、经成员关系随袋走过移动事实的成员；决定四、五、六原样引用，无条款停用
- 票 [53](../../.scratch/demo-chain-gaps/issues/53-movement-facts-recorded-before-their-segment-never-reach-tracking.md)；后继票 [120](../../.scratch/demo-chain-gaps/issues/120-movement-facts-before-a-pickup-attempt-or-carrier-segment-are-not-backfilled.md)（揽收执行口与承运商段）、[121](../../.scratch/demo-chain-gaps/issues/121-departures-recorded-before-their-segment-are-never-re-asked-at-the-gate.md)（先于段的出发不补问门禁）

## 复核记录（2026-10-20，Claude Code 会话代决，非 owner 本人复核）

用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」。本记录的越权风险点已经 [代决复核报告](../../.scratch/owner-review-20261020/report.md) 逐条复核，其余各条维持；正文与 Decision 不变。

- **M19**：越权风险点 1（进段那一半欠账时补不上）复核为应修订，承接票 [demo-chain-gaps/136](../../.scratch/demo-chain-gaps/issues/136-segment-continuation-wakes-the-movement-fact-backfill.md)，修法二选一在那一票里定。
