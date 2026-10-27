# ADR-0297：待确认的逐对象尝试结果凭同一次上门的后补来源更正成确定结果——更正只追加、只回指待确认那一版，执行方在作业事实面后补、复核人在运营决定面认定，当前有效结果由「首次提交，被更正则取更正」读出

Status: Accepted（2026-10-26，Claude Code 会话代 TF owner 裁定：用户「你作为业务和系统专家自决」授权；票 [docs-consistency/code/90](../../.scratch/docs-consistency/code/90-a-pending-attempt-object-result-has-no-correction-entry.md)，出处票 code/01 代决 2。裁决能力边界：读过 TF CONTEXT「揽收、派送与交付」「运输交接与实物控制」，UC-TF-002、UC-TF-006 的结果契约、一致性与交接，[ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 全文、[ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md) 与 [ADR-0303](./0303-transport-fulfillment-operation-decisions-record-the-submitting-operator-as-operation-evidence.md) 的 Status 与决定、[ADR-0307](./0307-a-handover-corrected-from-pending-or-refused-to-handed-over-enters-the-declared-segment.md) 全文；揽收尝试执行、对象级揽收登记与更正、派送尝试登记、交付登记、失败尝试费登记、交付门禁读数的编排与适配器，两张结果表的迁移，`cmd/parcel-api` 的运营决定与作业事实装配。没读：ADR-0263 正文，NO 一侧认交接更正链的代码，admin-web 的运输履约查阅页。**部分停用（收窄）** [ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 决定四一句，适用场景只限本记录的四口（作业事实面后补两口、运营决定面认定两口），见「对已接受记录的作用」。越权风险点见文末，待 TF owner 复核。）<!-- 索引注记 -->｜**更正**：待确认那一格凭同一次上门的后补来源（伙伴迟到的签收、复核后认定的地址）更正成确定结果，只追加到各自的更正册（TF 迁移 `0055`），首次提交的结果行一个字不改；外键与 CHECK 把「只回指待确认」守在库面，一格至多一条；确定结果过既有构造门、必带所凭来源。**当前有效结果**读「首次提交，被更正则取更正」，重放与冲突比对读首次提交。**两个接入面**同一份编排：执行方在作业事实面后补，复核人在运营决定面认定（`PICKUP_ATTEMPT_RESULT_CONFIRMATION`、`DELIVERY_ATTEMPT_RESULT_CONFIRMATION` 各一种决定，access_identity `0027`），提交操作者作为操作证据，只进审计。**更正成揽收到手**经对象级揽收登记形成场外揽收、进段、交 parcel-shipment 采认，同（对象，尝试）至多一份揽收，更正晚到而对象此后已有更晚的控制事实时场外揽收照登、不带段声明，答 `LATER_CONTROL_FACTS`；**更正成妥投**只让那一格有了确定结果，有效交付仍由交付登记另登。运输履约查阅页加「待确认复核清单」册
Date: 2026-10-26

## Context

取证于 main `a5544faff`，只记 `S`。

- 规则：TF CONTEXT「揽收、派送与交付」写「之后同一对象的确定结果，凭同一次上门的后补来源以更正形成新判断版本，凭新的上门形成新尝试；待确认那一版留在册上不改写」；UC-TF-002 与 UC-TF-006 的一致性条要求同一对象在同一尝试中只有一个当前有效结果、更正形成新判断版本、外部伙伴状态迟到时按业务发生时间采用；结果契约要求待确认保存「查询或复核入口」。
- 代码（票 code/01 落地之后）：两侧尝试登记口收得下待确认并原样保存，换说法答冲突；没有任何入口能对（尝试，对象）落一版回指待确认的确定结果。后果：派送一侧那件对象登不出有效交付（交付登记答`派送结果待确认`），失败尝试费答`未受理`、续办是「等确定之后再登」，而同一次上门确定不了；揽收一侧的绕法是经对象级登记口登一份同（对象，尝试）的揽收，它不回指待确认那一格，尝试链上那件对象仍读作待确认。没有运营查阅面列出还在待确认的对象。
- 两张结果表一个对象一行、没有版本列：`pickup_attempt_result` 主键（租户，来源身份，对象），按尝试提交的来源身份落；`delivery_attempt_result` 主键（租户，尝试身份，对象）。
- 同族先例：交接的待确认「以更正成`已交接`或`已拒收`收尾」（ADR-0263、ADR-0307），交接、揽收、POD 的更正随原事实归作业事实面（ADR-0151 决定四）；外部资金事实更正另开了操作者手工口（ADR-0236 决定六）；运营决定记提交操作者（ADR-0303）。

## Decision

**一、更正是追加，只回指待确认那一版。**

- 两侧各一张更正册：`pickup_attempt_result_correction`、`delivery_attempt_result_correction`。更正行回指首次提交的待确认行（外键带 `corrects_outcome` 恒为 `PENDING_CONFIRMATION`，CHECK 钉死），所以库面就接不上一格已是确定结果的行；主键同结果行，一格待确认至多一条更正。首次提交的结果行、尝试行、揽收尝试摘要一个字不改。
- 更正带：更正成的确定结果、依据（失败与拒收的原因）、所凭的后补来源（必备）、结果的业务发生时刻、更正这一判断的时刻、登记时刻，运营决定面提交时另带操作者。更正成揽收到手另存更正时所报的控制依据，**只为重放比对**——控制事实本身是对象级揽收登记上的场外揽收，那里才是权威。
- 领域：`CorrectPendingAttemptObjectResult`、`CorrectPendingDeliveryAttemptResult`。确定结果走既有构造门（失败与拒收必带依据、揽收到手与妥投不带依据、不早于到场、对象在尝试范围内）——更正不另立一套门，放不进首次提交进不来的形状。业务发生时刻可以早于待确认那一版（伙伴迟到的签收业务上先发生、后到达，UC-TF-002「按业务发生时间采用」）；更正这一判断不早于它所更正的待确认、也不早于它说的那件事。
- 确定结果不更正成待确认，也不更正另一版确定结果——后者是对已确定判断的更正，另一种更正，不在这条路上（答`并非待确认`，`RESULT_NOT_PENDING`）。

**二、当前有效结果由「首次提交，被更正则取更正」读出，两种读法各读各的。**

- 按当前有效结果读的：交付登记、交付门禁读数与失败尝试费登记（派送一侧经 `DeliveryAttemptView.LoadDeliveryResult`，揽收一侧经 `FailedAttemptSource.FindEffectiveByKey`）。更正成妥投之后同一份交付登记走得过去；更正成失败之后失败尝试费照常登；更正成揽收到手之后失败尝试费答`没有失败`。
- 按首次提交读的：尝试登记的重放与冲突比对（`FindByKey`）——同一次提交的重放不因更正而变成冲突。
- 交付登记对仍是待确认的那一格答复不变：`DELIVERY_RESULT_PENDING_CONFIRMATION`，不并回`未生效`，也不改词；失败尝试费对仍是待确认的那一格仍答 `ATTEMPT_RESULT_PENDING_CONFIRMATION`。更正落地只是让「先把那一格确定下来」有了同一次上门的路。

**三、两个接入面，同一份编排。**

- 执行方后补挂**作业事实登记**能力面（`/transport-fulfillment/offsite-pickup-attempt-result-corrections`、`/transport-fulfillment/delivery-attempt-result-corrections`）：更正随原事实的族，ADR-0151 决定四原样适用，认证与授予就是尝试登记那一份。
- 复核人认定挂**运营决定**能力面（`/transport-fulfillment-pickup-attempt-result-confirmations`、`/transport-fulfillment-delivery-attempt-result-confirmations`）：复核人核对来源之后把一格待确认认定为确定结果，是管理台上作出的生产决定，没有设备（ADR-0151 定族判据：提交方是谁、授予维度是什么）。揽收与派送各一种决定——能认定揽收的不因此能认定派送，授予的粒度等于权力的粒度；也不与承运商首次有效收寄判断共用，那一种判的是承运主体首次接触。授予按租户 × 决定种类登记（access_identity `0027`），谁持哪一种是租户取值，准入照 ADR-0149 决定四判。
- 两个面各自一个端点实现，面的区别只在 Intake；运营决定面把信封里的提交操作者（「发行方#sub」）作为操作证据落在更正行上，待遇照 ADR-0303 三条：只进审计，不进重放与冲突比对（同一更正换一位操作者重放仍答已有，在册的不被顶替），不进任何门；回执不回显。作业事实面不带操作者（设备身份不进载荷，ADR-0023），该列为空。

**四、更正成揽收到手：经对象级揽收登记形成场外揽收，同（对象，尝试）至多一份。**

- 更正命令带控制依据（揽收到手必带，失败与拒收带了即未受理），地点、任务、执行方取自那次尝试；编排转交的是首登口那同一条登记编排（揽收登记册、段登记册、实际承运商判断登记册、结果版本签发、登记意图交付）。于是进段、铸判断首版、交 parcel-shipment 采认（`offsite-pickup.registered`）与首登逐格同形，不另造一条只有这条路才有的链。
- **同（对象，尝试）的对象级登记就是那一份揽收事实，更正成揽收到手是把待确认那一格绑到它**：经对象级登记口先登了同内容揽收的（同（对象，尝试））由更正收口：不登记第二份、不进第二次段；已登的是另一回事（控制依据不同）则更正答冲突、不写更正——要改那份揽收走揽收更正口，更正不顶替它。两条路因此不会对同（对象，尝试）各说一样。
- 更正命令带段声明（整次一个段、对象自己的计划段、段服务动作，三格皆可缺，缺席不立段）：待确认那一格从没带对象进过段，这是对象的第一次揽收，不是对已有参与的重派生，ADR-0112 决定二「更正命令不带段号」说的是后一种，不适用。
- **更正晚到、对象此后已有更晚的控制事实时，场外揽收照登、不带段声明。** 转交登记之前，更正处理器先问 ADR-0307 决定三的那一问（`laterControlFacts`：对象任一段里参与链的链尾有晚于这次揽收发生时刻的入场或离场，在场、已离场、失效都算，同一刻不算更晚）；命中时登记不带命令的段声明，于是不立声明的段、对象不加入它，答复带 `segmentEntryRefusal: LATER_CONTROL_FACTS`、不带续办引用（重试不会变）。取「不带段声明登记」而不取「拒绝这次更正」，因为揽收事实本身是真的，只是不应再造第二个段——凭这一版进声明的段会让对象两段同时在场，此后它的交付与交接按对象找段都答多段在场、整笔回滚。命令没声明段、或同（对象，尝试）已登过同内容揽收（读回在册那份、没碰段）时答复不带这一格。这一问读不动不在更正里作答：转交的登记要读同一份段登记册，读不动它自己会留进段的欠账。答复格 `LATER_CONTROL_FACTS` 是 `SegmentEntryRefusal` 既有的一格，更正两个接入面的 HTTP 译装沿用对象级揽收登记的同一个字段，不新增格。
- 登记写在更正追加之前、同一笔事务；装配的事务壳对「成立」以外的任一格整笔回滚，不留下没有更正回指的场外揽收。

**五、更正成派送各格不形成别的事实。** 更正成妥投不带 POD、不转移控制、不结束参与：有效交付仍由交付登记带着 POD 另登，那一道的构造门、门禁读数与参与结束一字不改。

**六、锁与事务。** 更正处理器必须在调用方的事务里跑：读被更正的那一格时对结果行加锁（`SELECT … FOR UPDATE`），直到事务结束；同一格的并发更正排队，后到的读到先到的更正。锁序：这格结果行 → 对象级揽收登记键与段册（`enterFulfillmentSegment` 自己的顺序）；派送一侧只有这一把。没有任何路径先取段册或登记键再回来取结果行。无事务即拒（PBC-08）。

**七、复核入口：运输履约查阅页加「待确认复核清单」册**（`GET /transport-fulfillment-records?registry=pending-attempt-result`）。列还在待确认的逐对象尝试结果，两侧合并、最早的先看，带待确认那一版的来源与缺口（UC-TF-002「揽收结果待确认」、UC-TF-006「尝试待确认」要保存的已知事实与缺口）和更正口要指名的键（揽收的来源身份，两侧的尝试与对象）；已更正的靠更正册反连接判、不列。排序的时限提醒、超期告警不做（产品策略，另判）。

## 候选与反方

- **结果表加版本列。** 不取：所有读结果表的口（尝试登记的重放比对、读回的接力核对、失败尝试费、交付登记）都得认版本，漏认的那一口会无声地读到过期的一版；追加表让「原版不动」靠形状、读哪一版靠两个具名的读口。
- **更正成揽收到手时在尝试链上另形成一份揽收（`pickup_attempt_pickup`）。** 不取：尝试级的 `offsite-pickup.formed` 信封没有消费者（装配注释「只接对象级的 `registered`」），那样形成的揽收既不被 parcel-shipment 采认，又与对象级登记口对同（对象，尝试）各说一样；要互斥就得再造一道只认这条路的检查。
- **只开作业事实面。** 不取：复核后认定的地址、核对多方来源后的判断没有设备，执行方也未必知道（ADR-0151 定族判据）；只开这一面，票面第二个例子无人能做。
- **只开运营决定面。** 不取：执行方自己后补是常态，ADR-0151 决定四「能登不能改」的担心原样成立；让执行方为了改自己登的东西去找复核人，是倒退。
- **更正命令不带段声明，沿用首登时的声明。** 本记录不做：首登的段声明只在命令里读一次、不落地（待确认的对象更是从没用上），要沿用就得给尝试与结果行加列并改尝试登记口；ADR-0307 决定二为交接做了这件事。记为越权风险点 3。
- **待确认复核清单另票。** 不取：票 code/01 越权风险点 3 已把查阅清单托给本票，没有清单，运营认定面无处下手。

## 对已接受记录的作用

- **部分停用（收窄）[ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 决定四一句**：被停原句「更正与首登的提交方是同一方，认证与授予也应是同一份」。适用场景只限本记录的四口——尝试结果更正的作业事实面两口（`/transport-fulfillment/offsite-pickup-attempt-result-corrections`、`/transport-fulfillment/delivery-attempt-result-corrections`）与运营决定面两口（`/transport-fulfillment-pickup-attempt-result-confirmations`、`/transport-fulfillment-delivery-attempt-result-confirmations`）。交接更正、揽收更正、POD 更正三口随原事实归作业事实面，那一句对它们仍成立。理由：待确认的结果凭复核认定时，认定的人不是登记尝试的设备，「同一方」不成立；作业事实面（执行方后补）仍是同一份认证与授予。同 [ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md) 对外部资金事实更正的做法。
- **回答票 code/01 代决 2 与越权风险点 3、4、5**：同一次上门以更正确定那一条口有了；「查询入口 / 复核入口」有了清单；失败尝试费`未受理`细格的续办「等确定之后再登」有了可走的路。无条款停用。
- **无条款停用**：[ADR-0112](./0112-source-correction-rederives-participation-as-a-superseding-version-on-the-same-segment.md) 决定二（见决定四，说的是对已有参与的重派生）、[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)（各格按恢复动作分）、[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)（跨上下文读方：没有适配器读尝试结果，见 Consequences）、[ADR-0303](./0303-transport-fulfillment-operation-decisions-record-the-submitting-operator-as-operation-evidence.md)（操作证据待遇沿用）、[ADR-0222](./0222-composition-roots-register-endpoints-and-routes-per-feature-file.md)（装配：新特性文件、assemble 文件、未接线占位，未动 endpoints.go、main.go、routes.go）。

## Consequences

- TF：迁移 `0055`（两张更正表、派送结果表唯一键、待确认部分索引）；领域加两个更正构造与两个错误值；编排加揽收、派送两个更正处理器与共用答复格；`FailedAttemptSource` 的读方法改名 `FindEffectiveByKey`（按当前有效结果答），`LoadDeliveryResult` 读当前有效结果；查阅页加一册。access_identity 迁移 `0027`、两种决定种类。
- 新格的消费方落点：更正的答复格 `RESULT_CORRECTED`（201）、`EXISTING_CORRECTION`、`SOURCE_CONFLICT`、`RESULT_NOT_PENDING`、`SOURCE_NOT_ACCEPTED`、`CORRECTION_UNDECIDED`（只有 `RESULT_CORRECTED` 答 201，其余五格都答 200）。HTTP 译装在两个更正端点；`cmd/parcel-api` 装配四行端点（两条作业事实、两条运营决定）并以事务壳整笔回滚非成立格；管理台不读尝试结果、也还没有认定页（越权风险点 4）；跨上下文读方——VE、SA、PS、NO、NR、PP、CC 的适配器都不读尝试结果，只读场外揽收与有效交付两封信封，更正成揽收到手经对象级登记发的就是后者。交付门禁读数、失败尝试费、交付登记读当前有效结果。
- 演示种子：配置员加两种授予，准入对照与权威区间各两行（步 `481`）。
- UC-TF-002、UC-TF-006「给开发的交接」删掉票 code/90 那一行；TF CONTEXT「揽收、派送与交付」补更正的规则句；UC-TF-002 新立 `AT-TF-357`、`358`、`360` 至 `363`，UC-TF-006 新立 `AT-TF-359`。

## 越权风险点

1. **复核人能认定的范围**（TF owner）。运营决定面的两种决定只约束谁能认定，不约束认定时要有什么证据：所凭的后补来源是一个引用，产品不核它的内容。若 owner 要求认定必须带第二人复核或来源的格式门，是产品策略，加在认定口上，不动更正册。
2. **更正成揽收到手以对象级登记为权威**（TF owner）。更正只存所报的控制依据供重放比对，控制事实在场外揽收上；对象级登记之后若又经揽收更正口改了控制依据，更正册里存的那一份不跟着变，重放更正会拿旧控制依据比，可能答冲突——更正册里的控制依据是「更正当时所报」，不是当前控制。若 owner 要两处恒一致，改成重放时不比控制依据、只比结果与来源。
3. **段声明由命令带，复核人未必知道**（TF owner）。运营决定面认定揽收到手时，复核人若不知道当初声明的段，只能留空，对象于是登了揽收而不在任何段里，段那一半另说（归票 demo-chain-gaps/179 那一题）。要沿用首登声明，要给尝试与结果行加列并改尝试登记口，照 ADR-0307 决定二；这里不做，因为那是改已发布的尝试登记口的存储形状。
4. **管理台没有认定页**（产品）。清单与两个认定口都在后端，管理台运输履约查阅页没有这一册的页签、也没有认定动作；本记录没有碰 `apps/admin-web`（共享件与运营页另一会话在改）。上线前认定只能经 API 调用。
5. **晚到更正的段那一半不重算，对象第二次揽收的责任起点归采用先后**（TF owner）。答 `LATER_CONTROL_FACTS` 的更正在声明的段里没有参与：那一段的承运方在这一程上没有参与、不形成实际承运商判断首版，场外揽收的事实仍交 parcel-shipment（决定四）。同一对象因此可能有两个不同尝试的场外揽收、发两封 `offsite-pickup.registered`：parcel-shipment 的采用按责任起点唯一处理，先到的采用为责任起点，后到的（不声明更正关系的新一次尝试）落一份 `SOURCE_NOT_ADOPTED`、依据 `RESPONSIBILITY_ALREADY_STARTED/OFFSITE_PICKUP/<先到的版本>`，确定性、入账收工，不报错也不写第二份采用；责任起点因此取先到的那封而不是业务时间更早的那份，是 parcel-shipment 既有的口径（`AT-PS-049`）。拦截只覆盖「对象此后已有更晚的入场或离场」：对象在待确认之后、更正所说的发生时刻之前已经在某段里在场、尚未离场时，更正成揽收到手仍会让它进声明的段——这与对象级揽收首登对已在别的段里的对象一样不拦（首登路径没有这一问），若 owner 要，在揽收登记的进段门上加同一道检查，两条路一起受益。
6. **已有的对象级登记与尝试链上的揽收到手各说一样**（TF owner）。尝试级「揽收到手」结果（尝试登记口直接收的）与对象级登记对同（对象，尝试）不互斥（尝试链形成 `pickup_attempt_pickup`、对象级登记形成 `offsite_pickup`，各自进段，后一个遇「对象已在段内」被正当拒绝）；本记录只保证更正路径不制造第二份。尝试级揽收到手与对象级登记的关系是更早就有的口径出入（尝试级 `formed` 信封无消费者，见出站信封消费清点），不在本记录。
7. **重放更正不重发登记意图**（TF owner）。更正成揽收到手的重放答已有更正，不调登记编排，所以登记时交意图失败留下的续办引用只能经对象级登记口原样重放来重发；更正处理器本身不重发。
8. **没读的部分**。ADR-0263 正文、NO 一侧认交接更正链的代码、admin-web 运输履约查阅页的数据口若对「待确认更正」另有说法，以原文为准，本记录让位。

## Links

- 票 [docs-consistency/code/90](../../.scratch/docs-consistency/code/90-a-pending-attempt-object-result-has-no-correction-entry.md)：本记录的出处；[code/01](../../.scratch/docs-consistency/code/01-fulfillment-attempt-object-outcomes-have-no-pending-confirmation.md)：待确认两格的来处，其代决 2 与越权风险点 3、4、5 由本记录回答
- [ADR-0151](./0151-unassigned-command-faces-get-their-families.md)：运营决定能力面与定族判据（决定一、四）；本记录部分停用（收窄）其决定四一句，适用场景只限尝试结果更正四口（作业事实面后补两口、运营决定面认定两口）
- [ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md)：更正也开操作者手工口的先例（决定六）
- [ADR-0303](./0303-transport-fulfillment-operation-decisions-record-the-submitting-operator-as-operation-evidence.md)：操作证据只进审计的待遇（决定三沿用）
- [ADR-0307](./0307-a-handover-corrected-from-pending-or-refused-to-handed-over-enters-the-declared-segment.md)：交接待确认更正成`已交接`的先例（决定一、二）；本记录决定四在段声明上走了另一条，理由与风险见越权风险点 3
- [ADR-0112](./0112-source-correction-rederives-participation-as-a-superseding-version-on-the-same-segment.md)：对已有参与的重派生；决定二「更正命令不带段号」不适用于第一次揽收
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)、[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)、[ADR-0222](./0222-composition-roots-register-endpoints-and-routes-per-feature-file.md)
- [TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)：「揽收、派送与交付」；[UC-TF-002](../application/transport-fulfillment/UC-TF-002-PERFORM-OFFSITE-PICKUP.md)、[UC-TF-006](../application/transport-fulfillment/UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md)
