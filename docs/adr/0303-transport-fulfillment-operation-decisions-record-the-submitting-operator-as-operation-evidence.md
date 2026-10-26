# ADR-0303：TF 运营决定把提交操作者作为操作证据记在决定记录上——关段、明确终止、建派送任务、装载分配、有效时间判断、承运商首次有效收寄判断与运输委托提交七口逐口落列，只进审计，不进重放比对，也不进任何门

Status: Accepted（2026-10-26，Claude Code 会话代 TF owner 决：用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 demo-chain-gaps/126，单一权威是代决复核报告 `.scratch/owner-review-20261020/report.md` 的 M1。裁决能力边界：读过 [ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 全文、[ADR-0187](./0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md) 全文、[ADR-0216](./0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md) Status 与决定四、[ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 决定四至七、[ADR-0207](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) 决定三与越权风险点、[ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 决定五；TF CONTEXT 里「运营决定」各句；TF 运营决定 Intake、七口的编排、登记册端口与 PostgreSQL 适配器、`cmd/parcel-api` 的运营决定装配，费用发生登记的登记者在领域、迁移 `0034` 与查阅读面上的落法。没读：UC-TF 各篇除验收示例之外的正文，ADR-0100、0202、0203、0204、0232 正文。部分停用 ADR-0151 决定二「TF 各口：信封给租户，其余照各口既有载荷。」一句（只限决定一的七口），回答其越权风险点 5。越权风险点见文末，待 TF owner 复核。）<!-- 索引注记 -->｜**记**：七口把信封里的提交操作者（「发行方#sub」，与费用发生登记的登记者同一个类型与写法，不加 PS 的 `OPERATOR/` 前缀）作为操作证据落在各自的决定记录上——段 `closed_by`、参与终点 `terminated_by`（替代版本随终点承继）、任务 `opened_by`、分配版本 `decided_by`、显式判断那一版 `effective_judged_by`、收寄判断每一版 `judged_by`、委托 `submitted_by`（TF 迁移 `0051`）。**待遇照 ADR-0187**：只进审计，不进重放与内容冲突比对，不进准入与状态机任何一门；同一决定换一位操作者重放仍答已有，在册的不被顶替；写面回执不带，收寄链与轨迹事实两个查阅面带出；不进交给别的上下文的意图。**可缺**：可空不回填，NULL 读作「没记下是谁」；执行器、规则与素材到达形成的行本就没有。**不在本记录**：派送补拍与按册重问不记在执行器形成的任务上，谁按的另记在重问决定处；订舱提交与取消、取消委托、段采用委托、容量预占与释放同一判据；两件都归票 demo-chain-gaps/180

Date: 2026-10-26

## Context

- **M1 是什么。** TF 在管理台上作的运营决定（ADR-0151 决定一）各口不记是谁作的决定。在 `3e40b51e0` 上，`tfhttp.OperatorDecisionIntake` 对关段、建派送任务、装载分配、终止参与、有效时间判断、承运商首次有效收寄判断与运输委托提交七口只交出认证结果里的租户（`tenantFor`），操作者在 Intake 里就丢了。已经记人的是另一组：费用发生登记把操作者落成登记者（ADR-0207 决定三、ADR-0209 决定五，迁移 `0034` 的 `registered_by`），之后的发生项修订、解除采用与委托协议换版各记各的决定人。复核报告 M1 列的就是不记人的那七口。
- **悬而未决的出处。** ADR-0151 决定二 TF 一项写「信封给租户，其余照各口既有载荷」，越权风险点 5 把「要不要把它作为操作证据落进 TF 的档」留给 TF owner。ADR-0187 为 PS 的受控关闭与重开定了「记」，待遇三条：只进审计，任何门与比对都不读它，重放不比它；它的决定七把 TF 各口仍留给 TF owner。
- **为什么上线前必修。** 首个租户上线之后作出的决定若不记人，事后无从补记：Intake 那一刻是唯一知道是谁的时候。
- **同一判据还覆盖哪些口。** TF 运营决定能力面上另有一组口同样形成决定记录、同样只取租户：订舱提交、取消订舱、取消运输委托、履约段采用运输委托、容量预占、容量释放。M1 没有列它们。其中订舱与容量池两组的存储与查阅面在本记录起草时正由票 demo-chain-gaps/127 改动（它先于本记录进 main）。派送补拍与按册重问（ADR-0230 决定七改挂运营决定，决定种类「派送重问」）形不成自己的决定记录：它们让派送发起执行器再问一次所有者，形成的任务与进程内那一拍出自同一个执行器（ADR-0216 决定四）。
- **形状已有两套。** PS 写作 `OPERATOR/<发行方>#<sub>`（ADR-0187 决定三）；TF 已经记人的各处（登记者、修订人、解除人、换版决定人）写作「发行方#sub」，类型是 `domain.RegistrantReference`。

这一格是机制（ADR-0146）：记不记、记在哪、怎样不进比对与门、存量怎么表示，都是产品机制；操作证据的保存期限与查阅权限是租户取值，本记录不设默认。

## Decision

**一、记：七口把提交操作者作为操作证据落在各自的决定记录上。**

| 口 | 决定记录 | 列（迁移 TF `0051`） | 读回 |
|---|---|---|---|
| 关段 `/transport-fulfillment-segment-closures` | 段的关闭 | `actual_fulfillment_segment.closed_by` | `ActualFulfillmentSegment.ClosedBy` |
| 明确终止 `/transport-fulfillment-participation-terminations` | 那一条参与的终点 | `fulfillment_participation.terminated_by` | `FulfillmentParticipation.TerminatedBy` |
| 建派送任务 `/transport-fulfillment-dispatch-task-registrations` | 任务 | `dispatch_task.opened_by` | `ports.DispatchTaskRecord.OpenedBy` |
| 装载分配 `/transport-fulfillment-load-assignment-registrations` | 分配那一版 | `load_assignment.decided_by` | `ports.LoadAssignmentRecord.DecidedBy` |
| 有效时间判断 `/transport-fulfillment-effective-time-judgments` | 显式判断形成的那一版事实 | `external_carrier_tracking_fact.effective_judged_by` | `ports.ExternalTrackingFactRecord.EffectiveJudgedBy` |
| 承运商首次有效收寄判断 `/transport-fulfillment-carrier-first-effective-pickup-judgments` | 本次落下的那一版 | `carrier_first_effective_pickup.judged_by` | `ports.CarrierFirstEffectivePickupRecord.JudgedBy` |
| 运输委托提交 `/transport-fulfillment-transport-commission-submissions` | 委托 | `transport_commission.submitted_by` | `ports.TransportCommissionRecord.SubmittedBy` |

- 落点按决定记录在哪里定。段上的两件（关段、终止）是聚合内的状态，操作者挂在聚合上：替代参与版本承继终点时连同终止人一起承继，那仍是同一次终止。其余五种决定本身就是登记册上的一条记录（一版），操作者挂在越过提交边界的那条记录上，与登记时刻并列，不进领域本体，领域的门在结构上读不到它；每一版各记各的，不随链承继。
- 取值只来自信封：Intake 用认证结果的操作者（`OperatorIdentity.Operator`）。封闭解码下载荷带操作者是坏报文。

**二、形状照费用发生登记的登记者，不另立第二套。** 类型 `domain.RegistrantReference`，写法「发行方#sub」（ADR-0100 决定二第三条：sub 只在它的发行方之内唯一）。不加 PS 的 `OPERATOR/` 前缀：TF 已经记人的各处都是这一写法，同一上下文里两种写法会让审计按人检索时漏掉一半。

**三、待遇照 ADR-0187 三条。**

- **只进审计。** 落在决定记录上、读回带出。查阅面上：收寄链 `GET /transport-fulfillment-carrier-first-effective-pickups` 每一版带 `judgedBy`，轨迹事实 `GET /transport-fulfillment-external-tracking-facts` 带 `effectiveJudgedBy`，没记下时整键缺席。`GET /transport-fulfillment-records` 各册不展示这七种记录，不改。交给别的上下文的意图载荷不带它（委托、收寄、轨迹事实三种信封逐字节不变）。
- **不进重放与内容冲突比对。** 各口的重放判据一个字不改：关段答`早已关闭`，终止答`已离场`，任务与分配按键答`已在册`，有效时间只比依据与时间（`已按同值判过`），收寄判断按 TFC-1 形状比，委托只比内容摘要。同一个决定由另一位操作者重放，答复与首次重放同格，在册的仍是先到的那一位；摘要里没有操作者。
- **不进任何门。** 准入（Intake 的授予与准入范围）、状态机（关段的在场与时序、终止的终点门、首次判据、协议核对、容量保障）都不读它。编排层命令里可以不带操作者，照样形成。
- **写面回执不带操作证据。** 照 ADR-0187 决定四「决定方、操作证据照旧不回」：七口回执与本记录之前逐字节相同。

**四、可缺；存量不回填。**

- 领域与库面都允许缺。缺席的来源：迁移 `0051` 之前形成的行；同一张表上不经运营决定口形成的行，即派送发起执行器形成的任务、按规则判断或素材到达形成的轨迹事实版本、以有效交付或下一次交接结束的参与；编排层不带操作者的调用方。
- 库面 NULL 读作「没记下是谁」，不读作「没有人作这个决定」。不立 NOT NULL，不回填：存量的提交人从未落在任何地方，拿别的列推一个人出来就是编造证据。
- 给了就不得是空白；只出现在能属于的行上：关段人只在已关闭的段上，终止人只在以明确终止结束的参与上，判断人只在显式判断的版本上。领域重建门与 `0051` 的 CHECK 各守一遍。
- 带 NUL 的操作者存不进登记册，在编排构造期答未受理。这是存储形状的门，与每一格身份值同一条纪律，不是按人放行的门。

**五、不在本记录的口。**

- **派送补拍与按册重问不记在任务上。** 两口作的决定是「再问一次所有者」，形成的任务内容出自所有者给的派送要求，形成它的是与进程内那一拍同一个执行器。执行器形成的任务 `opened_by` 留空。谁在什么时候按了补拍或重问，同样是这一位作出的决定、事后同样无从补记，不记在任务上，另记在重问决定处，归[票 demo-chain-gaps/180](../../.scratch/demo-chain-gaps/issues/180-the-remaining-tf-operation-decisions-and-the-delivery-dispatch-reask-record-the-submitting-operator.md)。
- **订舱提交、取消订舱、取消运输委托、履约段采用运输委托、容量预占、容量释放由[票 demo-chain-gaps/180](../../.scratch/demo-chain-gaps/issues/180-the-remaining-tf-operation-decisions-and-the-delivery-dispatch-reask-record-the-submitting-operator.md)落地。** 判据与本记录相同，也是上线前必修。本记录不落，是因为起草时订舱与容量池两组的读写面正由票 demo-chain-gaps/127 改动，并在一起就要与在途分支改同一批文件；那张票的订舱读面落地之后，这几口的操作者随读面一起带出更顺。落地之前这几口如实是「不记」，见越权风险点 1。
- **已经记人的口不动。** 费用发生登记的登记者（照旧随发生项回显，ADR-0207 越权风险点 7）、发生项修订、解除采用与委托协议换版各自的决定人，形状与本记录相同。

## 候选与反方

- **甲 · 不记**（现状）。否决：M1，真实租户上线后无从补记。
- **乙 · 记在接入通道的审计里，不进决定记录。** 否决，理由同 ADR-0187 候选：accessidentity 没有审计册，与决定记录之间没有关联键，审计要按决定找人时找不到。
- **丙 · 操作者进重放比对。** 否决：两位调度员先后提交同一份关段或同一份委托，后者会被答成冲突或长出第二版，正是各口重放代数要防的。
- **丁 · NOT NULL 并回填存量。** 否决：无源可填；同表本就有不经运营决定口形成的行。
- **戊 · 写面回执回显操作者**（照费用发生登记那一格）。不取：回执的用途是让调用方比内容、决定续办，操作者不是内容；照 ADR-0187 决定四，七口回执也因此逐字节不变。要看是谁，走查阅面；见越权风险点 2。
- **己 · 一律挂进领域本体。** 不取：五种登记册记录挂在领域上，判断与比对代码就读得到它，「不进任何门」只能靠纪律守；关段与终止挂在聚合上，是因为终止人要随终点承继，挂在记录上就承继不了。
- **庚 · 同一票落全部同判据的口。** 不取：起草时票 127 正改订舱与容量池的存储与读面，见决定五。

## 对已接受记录的作用

- **部分停用 ADR-0151 决定二 TF 一项一句**：「TF 各口：信封给租户，其余照各口既有载荷。」——改为：本记录决定一的七口，信封给租户与提交操作者，后者只作操作证据落在决定记录上，不顶替载荷里的任何一格。**适用场景**：只限这七口。ADR-0151 决定二其余各项、决定一与决定三至七不变。
- **回答 ADR-0151 越权风险点 5**：「TF 决定要不要记提交操作者」——记，待遇照 ADR-0187。
- ADR-0187 不改：它为受控关闭与重开定的 `OPERATOR/` 写法照旧；它决定七适用场景里留给 TF owner 的那一问，由本记录在 ADR-0151 越权风险点 5 处作答。
- ADR-0216 决定四、ADR-0230 决定七不变：补拍与重问的渠道、Intake 与答复形状照旧，见决定五。
- ADR-0207、ADR-0209 决定五不变。

## Consequences

- 迁移 `transport_fulfillment/0051_operation_decisions_record_the_submitting_operator.sql`：七列可空、各带 CHECK，存量行恒满足，直接校验。
- 领域：`OptionalRegistrantReference` 与 `RegistrantReference.Present`；`ActualFulfillmentSegment.CloseSegment` 与 `EndParticipationWithTermination` 多收一个可缺的操作者，重派生承继终止人；重建规格多 `ClosedBy`、`TerminatedBy` 两格。
- 端口与适配器：五种登记册记录各多一格；段登记册窄口 `CloseSegment` 多收关段人，`EndParticipation` 与 `Supersede` 随终点写终止人。
- 编排与线格式：七个命令各多一格；七个 `…Payload` 的 `Command` 改收整份 `OperatorIdentity`。
- 查阅面：收寄链 `judgedBy`、轨迹事实 `effectiveJudgedBy`。管理台不改。
- 文档：UC-TF-004 立 `AT-TF-335`、`AT-TF-336`，UC-TF-005 立 `AT-TF-337`–`340`，UC-TF-006 立 `AT-TF-341`。TF CONTEXT 不改：它没有一句说运营决定只收租户或不记人，七种决定的定义里也不列登记时刻一类的出处格；规则的出处是本记录。
- 演示：种子与探针脚本不改。不经运营决定口写进登记册的演示行留 NULL；经管理台或探针走七口时，演示操作者随之落列。证据只记 `S`。

## 越权风险点

1. **同判据的其余各口未落**（TF owner）。订舱提交与取消、取消委托、段采用委托、容量预占与释放在票 180 落地之前照旧不记人；那张票若排在首个租户上线之后，这几种决定的提交人同样补不回来。
2. **回执不回操作者**（TF owner）。同一上下文里费用发生登记回显登记者，委托协议换版回显换版决定人（回执 `decidedBy`），解除采用回显解除人（回执 `withdrawnBy`），这七口不回，两种口径并存；owner 若要求「已有」答复告诉后来者是谁先作的，七口回执各加一键即可，比对与门不受影响。
3. **个人标识入册**（租户合规）。操作证据指向具体的人；保存期限与查阅权限是租户取值，本记录不设默认。两个查阅面向持「主数据与运营查阅读」的人展示判断人，与费用发生登记的登记者处境相同。
4. **补拍与重问不留人**（TF owner）。决定五把它交给票 180；在那张票落地之前，按过补拍或重问的人在产品内无处可查——产品没有逐请求的接入日志。
5. **存量永久缺席**（TF owner）。`0051` 之前形成的决定读回都是「没记下是谁」；生产今天没有租户，演示行是合成。
6. **两种落点**（TF owner）。段上两件挂在聚合上，其余五种挂在登记册记录上（决定一）；若 owner 要统一，挂在聚合上的两件改成窄口额外收参、读回放进段记录，终止人的承继就得另写。
7. **CONTEXT 不写**（TF owner）。若 owner 认为「运营决定记提交操作者」属领域语言，应写进 TF CONTEXT「运营决定」相关句，本记录让位。
8. **没读的文档**。UC-TF 各篇正文与 ADR-0100、0202、0203、0204、0232 正文若另有关于操作者留痕的说法，以原文为准，本记录让位。

## Links

- [ADR-0151](./0151-unassigned-command-faces-get-their-families.md)：部分停用其决定二 TF 一项「TF 各口：信封给租户，其余照各口既有载荷。」一句（只限七口）；回答其越权风险点 5
- [ADR-0187](./0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md)：操作证据的待遇三条与「回执不回操作证据」的出处
- [ADR-0207](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md)、[ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md)：费用发生登记的登记者，形状的出处
- [ADR-0216](./0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md)、[ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md)：派送发起执行器与补拍、重问，决定五不记在任务上的理由
- [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)：操作者主体写成「发行方#sub」的出处
- [ADR-0028](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)：重建门照收存量
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：三类判定
- [UC-TF-004](../application/transport-fulfillment/UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md)：`AT-TF-335`、`AT-TF-336`
- [UC-TF-005](../application/transport-fulfillment/UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md)：`AT-TF-337`–`340`
- [UC-TF-006](../application/transport-fulfillment/UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md)：`AT-TF-341`
- 票 `.scratch/demo-chain-gaps/issues/126-tf-operation-decisions-record-the-submitting-operator.md`；代决复核 `.scratch/owner-review-20261020/report.md` M1；后继票 `.scratch/demo-chain-gaps/issues/180-the-remaining-tf-operation-decisions-and-the-delivery-dispatch-reask-record-the-submitting-operator.md`

## 复核记录（2026-10-26，Claude Code 会话代决，非 owner 本人复核）

用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」。正文、Decision 与上文各节不变。本节兑现决定五与越权风险点 1、4 留给票 [demo-chain-gaps/180](../../.scratch/demo-chain-gaps/issues/180-the-remaining-tf-operation-decisions-and-the-delivery-dispatch-reask-record-the-submitting-operator.md) 的两件，不新立 ADR：待遇与形状一个字不改，只是把同一待遇延到同一能力面上其余的口；重问落点是在决定五已定的「另记在重问决定处」里选一个落脚，不构成新取舍。读过：本记录全文，票 126 的两笔提交（`f445c668e`、`8d19b6588`），六口与补拍、重问的 Intake、编排、登记册端口与 PostgreSQL 适配器，迁移 TF `0030`、`0035`、`0045` 与 `0051`。

- **决定一续：六口落列（TF 迁移 `0054`）。** 同一待遇——只进审计，不进重放与内容冲突比对，不进任何门，写面回执不带，可空不回填，载荷自报一律 400，取值只来自信封。

  | 口 | 决定记录 | 列 | 读回 |
  |---|---|---|---|
  | 订舱提交 `/transport-fulfillment-booking-request-submissions` | 订舱 | `booking_request.submitted_by` | `ports.BookingRecord.SubmittedBy` |
  | 取消订舱 `/transport-fulfillment-booking-request-cancellations` | 订舱的取消 | `booking_request.cancelled_by` | `ports.BookingRecord.CancelledBy` |
  | 取消运输委托 `/transport-fulfillment-transport-commission-cancellations` | 委托的取消 | `transport_commission.cancelled_by` | `ports.TransportCommissionRecord.CancelledBy` |
  | 履约段采用运输委托 `/transport-fulfillment-segment-commission-adoptions` | 采用的那一代 | `segment_commission_adoption.adopted_by` | `ports.SegmentCommissionAdoptionRecord.AdoptedBy` |
  | 容量预占 `/transport-fulfillment-capacity-reservations` | 那一份预占 | `capacity_reservation.reserved_by` | `ports.CapacityPoolRecord.ReservedBy[预占引用]` |
  | 容量释放 `/transport-fulfillment-capacity-releases` | 那一次释放 | `capacity_release.released_by` | `ports.CapacityPoolRecord.ReleasedBy[释放引用]` |

  - 取消人只出现在已取消的行上（CHECK 与 `0051` 的关段人同一条）；取消只改取消时刻与取消人，不碰提交人，两个人各记各的。委托的开始是段采用带出的状态转换，不是一位操作者的决定，不记人，开始那一次 `Replace` 不碰取消人。
  - 容量池按值换存：`CapacityPools.Replace` 把预占与释放明细整组删了再插回，证据若只落在明细行上，下一次换存就丢。所以证据按引用挂在 `CapacityPoolRecord` 旁（两张按引用的表），不挂在池本体上——领域的守恒与有效区间在结构上读不到；读回时由明细行填表，编排新作一份预占或一次释放时只给那一份加键，已有的键原样带回去，另一位操作者重交走重放格、不改表。
  - 订舱册（`registry=booking-request`）每一行带出 `submittedBy` 与 `cancelledBy`，没记下时整键缺席；订舱提交口与取消口的回执不带。委托、采用、预占、释放本无展示这几种记录的查阅面，不新开（见越权风险点 12）。

- **决定五兑现：补拍与按册重问另记在问询册 `delivery_dispatch_ask`（`0054`）。** 不记在任务上、不改执行器的任何一格判断，也不记在派送待形成册上。
  - **候选与反方。** 甲 · 记在派送待形成册「最近一次问」旁：不取。待形成册是随执行器最近一次答复改写的待办台账，一行只留最近一次，留不下「每一次是谁按的」；补拍的那一件也可能根本没有待形成行（一问就形成了，从没缺过的对象不进册，ADR-0230 决定一）；了结的行里最近一次问的时刻被了结时刻顶替。乙 · 记在任务上：决定五已排除。丙 · 另立只增不改的问询册，一次按下一行：取。补拍对单件、按册重问对批量，各自一行，补拍带段与对象、重问带问法与件数上限（库上 CHECK 钉两种形状互斥）；重问不逐件记，逐件交执行器的命令不带操作者，执行器与任务因此一个字不变。
  - **谁进册。** 只有命令带了操作者才记（`asked_by` 不可空）：执行器随交接信封发起的那一路、按册重问逐件交来的，都不是一位操作者的按下，不写这本册。同一件被两位操作者各补拍一次就是两次按下，两行，不去重——这本册是审计，不是决定记录，没有「在册的不被顶替」这一说。
  - **记在哪一笔。** 补拍与执行器那一拍同笔（`TriggerDeliveryDispatchHandler.Trigger` 在占住这一件之后记）：那一拍回滚，这一次按下一并回滚。按册重问先于逐件去问另起一笔事务（装配点的 `transactionalAskLedger`），提交了才开始问，一件问不动也不让这一次按下无处可查。
  - **写不下与没接。** 册写不下答错误（`ErrDeliveryDispatchAskUnrecorded`），补拍整拍不答，重问一件也不问；操作者在场而装配没接册答错误（`ErrDeliveryDispatchAskLedgerNotWired`）。不新增答复格：重问与补拍的结果、答复形状、状态码与回执逐字节不变。带 NUL 的操作者答未受理，不进册。
  - **读。** 端口 `ports.DeliveryDispatchAskLedger.ListAsks` 由近及远按租户列出；本票不开 HTTP 查阅口（越权风险点 12）。

- **对已接受记录的作用。** 决定五「不在本记录的口」两条由本节兑现，不再「如实是不记」；越权风险点 1 与 4 的前提消除。ADR-0151 决定二 TF 一项一句的部分停用从七口延到这八口，同一适用场景（信封给租户与提交操作者，后者只作操作证据）；ADR-0151 Links 里挂本记录的那一条随之补一句。ADR-0230 决定四、五、七不变：渠道、Intake 与答复形状照旧。

- **越权风险点（续上文编号）。**
  9. **问询册是新增的一本册**（TF owner）。只增不改、没有保存期限、不随任何状态收口；保存期限与查阅权限是租户取值（风险点 3），本记录不设默认。若 owner 宁可不另立一本册，唯一的替代是把补拍与重问也记在待形成册上，那要放弃「每一次」与「没有待形成行的补拍」，见上文甲。
  10. **册写不下就不问**（TF owner）。重问为了不留无处可查的按下，把审计放在可用性之前：问询册读写不动时，按册重问一件也不问，补拍整拍不答。要反过来（先问后记、记不下只报警）会重新引入「按过却查不到」。
  11. **执行器报错的补拍不留痕**（TF owner）。补拍的记录与那一拍同笔，那一拍因册读写不动整笔回滚时，这次按下也没有留下；按下没有产生任何效果，调用方得到的是「没形成答案」，原样重按即可。若 owner 要连无效果的按下也留，补拍也得另起一笔事务。
  12. **问询册与另五种记录没有查阅面**（TF owner、前端 owner）。除订舱册外，委托取消人、采用人、预占人、释放人与问询册只在库与端口可查，没有 HTTP 读面；产品没有逐请求的接入日志，审计要看得到它们得另开读面，本票不做。
  13. **池证据靠记录带回**（TF owner）。预占人与释放人挂在 `CapacityPoolRecord` 旁，换存池时必须把读回的两张表原样交给 `Replace`；今天只有「读回再改」的编排，新代码若以一份新铸的 `CapacityPoolRecord` 调 `Replace`，会把已有证据抹成 NULL。架构上没有第二道闸，靠这一条与同包用例（`TestTheCapacityPoolKeepsTheReservingAndReleasingOperatorsAcrossReplacements`）守。
  14. **存量永久缺席**（TF owner）。同风险点 5：`0054` 之前形成的订舱、取消、采用、预占、释放读回都是「没记下是谁」，补拍与重问在这本册建立之前的按下无处可查；生产今天没有租户，演示行是合成。
  15. **没读的文档**。UC-TF 各篇除验收示例之外的正文、ADR-0201 至 0204 正文若另有关于这几口操作者留痕的说法，以原文为准，本节让位。

- **验收示例。** UC-TF-004 立 `AT-TF-348`–`AT-TF-353`（订舱提交、取消订舱、取消委托、段采用、预占、释放），UC-TF-006 立 `AT-TF-354`、`AT-TF-355`（补拍、按册重问）；迁移 `transport_fulfillment/0054_remaining_operation_decisions_record_the_submitting_operator.sql`。
