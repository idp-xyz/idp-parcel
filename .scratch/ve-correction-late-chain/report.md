# VE 更正与迟到链勘察（VE-CORR-SURVEY）

只读勘察，不含实现代码。取证基准钉在 `origin/main` tip `7d7e138`，在独立只读 worktree 上读取，
未在共享树上读写。跨文件引用一律用符号名或引文，不用行号。

## 结论先行

`AT-VE-044` 是 **(b) 一张要先立领域能力的票**。

源侧已经在发更正了（transport-fulfillment 两条，且都已接进生产路由），提供方那份可重读的记录也
已经带齐 `corrects` / `corrected_at`。接不上的原因在 VE 自己这边：`vedomain.AcceptedSourceFactSpec`
没有一格能装「本份取代哪一份」，翻译那一步把这一维丢了；同时 `AT-VE-044` 要的「原版本保留」在库面
上今天是假的。两样都要先改领域表达，不是接线能补的。

---

## Q1 追加版本、保留原版本，今天到哪一步

分四层答，每层结论不同。

### 一、领域层：追加版本这半边，机制已在

`vedomain.TrackingProjection.Rederive` 就是这一格：换版本、换条目、`priorVersion` 指回前身。它的
注释把依据写死了——「依据迟到事实、来源更正或映射版本变化形成新的当前投影（CONTEXT 生命周期）：
换版本、换条目、指回原版；原投影版本继续保留」。

`veapplication.DeriveProjectionHandler.Handle` 每收一份新键事实都会走到它：`FindCurrent` 有当前
投影就 `Rederive`，没有就 `DeriveTrackingProjection` 开第一版。所以**不是**「只支持首次形成 +
重复返回原结果」。编排把三格分得很清楚：

- 同键同内容 → `FactExistingResult`，按当前投影作答，不重复派生。
- 同键异内容 → `FactSourceConflict`，注释「同一来源版本携带不同内容：冲突保留原事实，不按最后
  到达覆盖」。
- 新键（一份更正带来的新来源版本正落在这一格）→ 落库 → `FindByParcel` 取回该包裹**全部**事实 →
  逐事实归类 → `Rederive`。

### 二、事实层：只增不删，成立

`vepostgres.AcceptedFacts` 的类型注释就是这句：「事实只增不删：来源更正是新版本新键，适配器没有
UPDATE 与 DELETE 语句」。`Save` 走 `ON CONFLICT DO NOTHING`，幂等键是（租户 + 来源上下文 +
事实引用 + 来源版本）。`FindByParcel` 的 `ORDER BY received_at, source_context, fact_ref,
fact_version`——接收序为主排序，注释解释为「投影消费的是『知道了什么』」。迟到事实靠这条自然落在
后面，不覆盖任何早到事实。

### 三、投影持久层：**「原版本保留」不成立**

`vepostgres.Projections.Save` 是整行 UPSERT：

- SQL 是 `ON CONFLICT (tenant_id, parcel_ref) DO UPDATE SET version_id = EXCLUDED.version_id,
  derived_at = ..., prior_version = ..., entries = ...`。
- 迁移 `migrations/visibility_exception/0007_tracking_projection.sql` 的主键是
  `PRIMARY KEY (tenant_id, parcel_ref)`——一个包裹一行。
- 适配器与迁移的注释都自陈「库只管当前版，重派生改同一行，历史由 `prior_version` 指回」。
- `ports.ProjectionStore` 接口只有 `FindCurrent` 与 `Save` 两法，没有任何按版本读回历史的口。

问题在于 `prior_version` 是一列没有外键的裸文本，而它指的那一行**已经被同一句 UPDATE 覆盖掉了**。
指针在，靶子不在。`domain.RehydrateTrackingProjection` 的注释「库只管当前版，历史由 prior 指回」
描述的效果因此对不上：`prior` 指回去也读不出那一版的 `entries` 与 `derived_at`。

结论：`AT-VE-044` 的「原版本保留」在**领域值语义**上成立（`PriorVersion()` 给得出前版编号），在
**可查证的记录**上不成立。这一格直接压到 CONTEXT 的硬句上——「迟到事实、来源更正或事件有效性变化
可以重新派生当前投影，但原来源、原映射、**原投影判断**和已经发布的客户信息必须保留关系」。原投影
判断今天读不回来。

客户视图同形，且注释已经承认：`migrations/visibility_exception/0001_customer_view.sql` 主键是
`(tenant_id, customer_account_id, parcel_id)`，`vedomain.CustomerTrackingView` 的注释直说
「重演需要原视图在手，而**库里只有当前版本**」。

对照组在同一个仓里：`transport_fulfillment.transport_handover` 把版本放进主键，两代天然共存，
`TransportHandovers` 的注释还专门写明为什么不用 `DO UPDATE`——「用 DO UPDATE 换写会让原判断消失，
审计再也答不出改判前是什么」。同一句道理在 VE 侧没有兑现。

### 四、身份层：`veidentity.ProjectionVersions` 给得出新版本，给不出代次

`NextProjectionVersionID` 是 `platformidentity.Minter` 的不透明签发（前缀 `PRJ`），无状态、不带
租户、不带包裹、不带序号。它保证每次重派生拿到一个不重的新标识，这一格是够用的。但它**不承载
顺序**——「哪一代在前」只能靠 `prior_version` 链走，而链上的行按第三条已被覆盖。

### 这一问的收口

追加版本：机制在。事实不丢：成立。原版本保留：**只在内存里成立，不在库里成立**。
而且——三格都没有更正语义。一份更正与被它更正的事实会**并列**进 `entries`，没有任何一格记
「后者取代前者」。

---

## Q2 有没有源上下文已经发布更正 / 迟到 / 撤销 / 替代

VE 的 `vedomain.SourceContext` 是封闭五值（`PARCEL_SHIPMENT` / `NETWORK_ROUTING` /
`NODE_OPERATIONS` / `TRANSPORT_FULFILLMENT` / `CUSTOMS_COMPLIANCE`），逐个核这五个上下文
`adapters/postgres/*_handoff.go` 里的 EventType 常量，共 29 个。

**答案：有，而且正是已经接线的那两路。**

### transport-fulfillment（9 个 EventType）——有两处更正，均已发布，VE 都在消费

**`transport-fulfillment.transport-handover.registered`**

- 应用层有真更正入口：`RegisterTransportHandoverHandler.Correct`，命令是
  `CorrectTransportHandoverCommand`（带 `PredecessorVersion` 与 `NewVersion`），结果码
  `HandoverCorrected`。
- 领域层：`TransportHandover.Correct(HandoverCorrection)` 形成新版本，`corrects` 回指前身，
  `Corrects()` / `CorrectedAt()` 读得出。构造期拒绝沿用原版本号（「沿用原版本号就是覆盖」）。
- 持久层：`TransportHandovers` 以新版本键落新行，注释「更正不换写原行，而是以新版本键落新行——
  版本在主键里，两代因此天然共存，`corrects_version` 回指前身」；迁移
  `0005_pickup_handover_registries_and_delivery_attempt.sql` 有 `corrects_version` /
  `corrected_at` 两列与配对 CHECK。
- 发布：`transportHandoverRegistrationEventID` 含 `Version`，两代各自入队；分区键取
  （租户 + 载运对象）保序，注释写明「ID 管幂等（**每个判断版本一份意图，更正因而不丢**），
  分区键管顺序」。
- VE 侧：`veinbox.TransportHandoverConsumer` + `DeriveOnTransportHandoverAdapter` 已装配在
  `cmd/parcel-dispatch/assemble.go` 的路由表里。

**`transport-fulfillment.effective-delivery.registered`**

- `RegisterEffectiveDeliveryHandler.Correct` → `Deliveries.Supersede`（同一事务里把当前行翻成
  历史、再插指回前版的新行），结果码 `DeliveryCorrected`；`EffectiveDelivery.Corrects()` /
  `CorrectedAt()`；迁移 `0001_effective_delivery.sql` 有 `corrects_version` / `corrected_at`。
- 发布：`effectiveDeliveryEventID` 已把 `DeliveryResultVersion` 编进 ID，注释写明为什么必须带
  ——「POD 更正换出新版本走的是同一个键，ID 少了版本两代就算出同一个字符串，而
  `outboxintent.EnqueueOnce` 先查后插——第二份于是静默不入队」。这一格是
  `.scratch/outbox-partition-key/issues/02-pod-correction-is-silently-swallowed-by-enqueue-once.md`
  修掉的，该票 `Status: resolved`。
- VE 侧：`veinbox.EffectiveDeliveryConsumer` + `DeriveOnEffectiveDeliveryAdapter`，已装配。

**其余七个没有第二次状态变化。** `offsite-pickup.registered` 只有 `Register` 入口——
`register_offsite_pickup.go`、`offsite_pickup.go`、`offsite_pickup_registry.go` 三处
`correct|supersede` 零命中。

### node-operations（4 个）——**没有**

`internal/nodeoperations/**/*.go`（去测试）对 `correct|supersede|revoke|amend|retract` 大小写
不敏感全文检索**零命中**。四个 EventType（`node-intake.formed`、`execution-fact.recorded`、
`collaboration-acceptance.decided`、`sealed-snapshot.recorded`）都是形成/登记。

这里不推测它「应该会有」：`DeriveOnNodeIntakeAdapter` 取 `record.Intake.Version()` 当来源版本，
说明收寄记录本身带版本位，但 NO 侧今天没有任何路径推进这个版本。

### network-routing（2 个）——**没有发布**

`PlanApplicability.Supersede` 在领域里有，但**生产代码没有任何调用方**（只出现在自身定义与
`plan_applicability.go` 的读回分支）。`RouteEvidenceSuperseded`、`JudgmentSuperseded` 是编排的
**结果码**，不是发布出去的事件类型。两个 EventType（`initial-route.formed`、
`reachability-judgment.formed`）都是形成事件。

### customs-compliance（9 个）——**没有发布**

`ComplianceJudgment.Supersede` 在领域里有（注释「Supersede 只追加新版本指回前版」），但**没有
任何应用编排调用它**。被实际调用的 `ReadinessJudgment.Revoke` 与 `SubmissionAuthorization.Revoke`
只出现在 `readiness_view.go` / `submission_authority_view.go` 的**读回重建**路径（从行重放撤销
状态），不是发布路径。

`customs-compliance.regulatory-restriction.changed` 名字带 changed，但它是「监管限制」这个业务
对象自己的新事实，不是对已发布事实的更正——按判据不算。

### parcel-shipment（5 个）——有修订机制，但与 VE 追踪投影接不上

`AmendCustomerSourceDataHandler` 是真的修订入口：`AmendmentIntent`、
`domain.NewAmendmentOfVersion(prior)`、`AmendmentReasonReference`、`AmendmentAuthoritySnapshot`
一应俱全，且发布 `parcel-shipment.source-data-version.formed`。但三条挡住：

1. VE 不消费这个类型（VE 只有四个消费者：节点收寄、场外揽收、有效交付、权威交接）。
2. 它修订的是**受理前的客户原始资料**，不是物理履约事实，进不了追踪投影的事实面。
3. `sourceDataVersionPayload` 的 `DeclaredParcelID` 是 `omitempty` 的**申报**包裹，而
   `vedomain.TrackedParcelReference` 要的是包裹永久身份——身份维度对不上。

`parcel-shipment.parcel-cancellation.recorded` 是「包裹取消决定」这一新业务事实，不是对先前已
发布事实的撤销；按判据不算。

### 按判据核这一问

判据是「提供方那份可重读的记录，是否已经带齐消费侧命令所要的每一维身份」。

**提供方带齐了。** `TransportHandovers.FindByKey` 的 SELECT 明确取 `corrects_version, corrected_at`，
`rebuildHandover` 把它们填进 `RehydrateTransportHandoverSpec`，`Corrects()` / `CorrectedAt()`
读得出。有效交付同形。

**缺口在消费侧的命令形状上。** `vedomain.AcceptedSourceFactSpec` 只有八格——`Source`、`Parcel`、
`Fact`、`Kind`、`Version`、`OccurredAt`、`EffectiveAt`、`ReceivedAt`——**没有一格能装 `corrects`**。
`derive_on_transport_handover.go` 的 `handoverProjectionCommand` 与
`derive_on_effective_delivery.go` 的 `projectionCommand` 都从不调用 `Corrects()`。更正维度是在
**翻译那一步被丢掉的**，不是提供方没给。

这与本仓以往几张票方向相反，值得单独点明：以往是提供方少一维、消费侧接不上；这一张是**提供方
已经给齐，消费侧的命令类型没有那一格**。

---

## 今天真让一份更正走完全程会怎样

以权威交接为例，按代码路径推演（未实测）：

1. TF `Correct` 落 v2，`corrects_version = v1`，两行共存。
2. 信封 ID 含版本 → v2 自成一份；分区键取（租户 + 对象）→ 与 v1 同队保序。
3. VE 消费者按四维（**含版本**）读回 v2。这一路读得对——`DeriveOnTransportHandoverAdapter` 的注释
   已经想到了：「`FindByKey` 必须带版本——更正是新版本新登记，按三维键读『当前版』会把更正与原
   判断叠成一次查找」。
4. `handoverProjectionCommand` 译出：`Fact` = `transport-handover/<对象>/<范围>`（**两代同一个
   引用**）、`Version` = v2、`Kind` 由裁决译（`handover-handed-over` → `handover-refused`）、
   `OccurredAt` = `JudgedAt`——注意 `TransportHandover.Correct` 里写的是 `JudgedAt: handover.judgedAt`，
   **更正刻意沿用原判断的业务时间**。
5. `Facts.FindByKey` 未命中（版本不同）→ 落新行。
6. `FindByParcel` 取回 v1 与 v2 两份。
7. 逐事实归类：映射键是（租户 + 映射版本 + 源上下文 + **事实类型**）（迁移
   `0014_mapping_keyed_on_fact_kind.sql`），两代 `Kind` 不同 → 归成两个不同里程碑。
8. `Rederive` 出新投影版本，`entries` 里**同时**有「已交接」与「已拒收」，没有任何一格说后者
   取代前者。
9. 客户视图据此重派生——读侧无从判断当前有效的是哪一条。

**而且现成的裁决器救不了这一格。** `ConflictResolutionBasis` 三格里唯一有产生函数的是
`ResolveByBusinessTime`，它按 `occurredAt` 排序、同刻即判「无法裁决」。更正刻意保留原业务时间，
所以两代的 `occurredAt` **恒等**——就算把它接进派生编排，一份更正也必然落在「无法裁决」。能分开
两代的那一维正是 `corrects` / `correctedAt`，而 VE 收不到。

（`ResolveByBusinessTime` 与 `RaiseConflictSignal` 目前只被 `fact_conflict_test.go` 调用，
`DeriveProjectionHandler` 里没有调用方。这两样属「已立未接」，与本票缺的那一维不是同一件事。）

### 顺带记一格次生风险（不属本票范围）

有效交付的载荷只带（租户 + 对象 + 尝试）不带版本，VE 按键读**当前版**。若更正信封先于首登信封
被消费，两份信封会读到同一版本，第二次 `FindByKey` 命中 → `FactExistingResult`，v1 从此不会进
投影。交接那一路因为版本在键里没有这个问题。适配器注释「只按键取当前版——结果版本在事件 ID 里
区分两代入队，不从 ID 回解析去查旧行」是知情的取舍，但它成立的前提是保序，而保序只在同一分区内
成立。建议单开一票，不在本票里改。

---

## Q3 判断：(b)，一张要先立领域能力的票

缺的不是接线，是两条领域表达。

### 缺口一：已接受事实之间的更正 / 替代关系，VE 没有这个术语

`vedomain.AcceptedSourceFactSpec` 与 `MilestoneClassification` 都没有「本份取代哪一份」这一维；
`visibility_exception.accepted_fact` 表也没有对应列。

CONTEXT 已经在要求它，但只给了「关系」二字，没有落成术语：

- 硬句：「迟到事实、来源更正或事件有效性变化可以重新派生当前投影，但原来源、原映射、原投影判断
  和已经发布的客户信息必须**保留关系**。」
- UC-VE-002 输入语义：「有效性变化 | 更正、撤销、替代或迟到关系；追加后重新派生，不删除原结果。」
- 生命周期：「迟到事实、来源更正或映射版本适用性变化 → 形成新的当前投影；已经发布的客户信息通过
  追加更正处理。」

三处都说了「有这么个关系」，但没有一处说：这份关系**由谁提供**、**以什么身份表达**、被替代的事实
在投影里**怎么呈现**。

要先在 CONTEXT 的 Language 里立术语，并在 Rules 里钉明至少三条：

1. 替代关系**由源上下文提供**，VE 不自行推断谁更正谁——这一条已被现有硬句蕴含（「本上下文不得
   自行使源事实失效」），但要落成对事实入口的显式要求。
2. 被替代的事实**不删除也不失效**——有效性归所有者，VE 只记关系。
3. 被替代的条目在投影里如何呈现：是给 `ConflictResolutionBasis` 加一格 `SUPERSESSION`，还是走
   一条与冲突裁决平行的独立替代判断。这两条路语义不同——前者把更正当成一种「可裁决的冲突」，
   后者认它是源上下文已经替 VE 裁决好的事实，VE 只照单收下。**这一格该由领域拍，不该由实现票
   默认选一个。**

不先做这一步就写代码，只能在适配器里凭 `Corrects()` 自己发明一套替代语义——那就是 VE 替 TF 判断
事实有效性，正撞上面第 1 条。

### 缺口二：投影版本的历史留存，是个取舍不是缺陷

`AT-VE-044` 的「原版本保留」在库面上今天是假的（见 Q1 第三条）。两条路：

- **A. 立真的版本表。** `visibility_exception.tracking_projection` 主键含 `version_id`，当前版由
  `is_current` 或独立当前表指名，照 `transport_fulfillment.transport_handover` 的做法。
  `ports.ProjectionStore` 相应多一个按版本读回的口。
- **B. 改口径。** 在 CONTEXT / UC 里明说「原版本保留 = 事实可重放重派生，不承诺历史投影行可读」，
  并相应改 `AT-VE-044` 的措辞。

**这也是个取舍，不该由实现票默认选一个。** 无论选哪条，都要先把 `Projections`、
`RehydrateTrackingProjection` 与迁移 `0007` 里那句「库只管当前版，历史由 prior_version 指回」改
对——它现在描述的效果与实际不符，`prior_version` 指向的行已被同一句 UPDATE 覆盖。

### 为什么不是 (a)

`Rederive` 与 `priorVersion` 只解决「新事实来了要不要换版本」，解决不了「这份新事实是来更正谁
的」。缺的那一维在**命令类型**上，加它就是改领域模型，不是接线。

### 为什么不是 (c)

源侧已经在发了。TF 两条更正链今天就在生产装配里（`cmd/parcel-dispatch/assemble.go` 的路由表登了
`transport-handover.registered` 与 `effective-delivery.registered`），提供方记录带齐了
`corrects` / `corrected_at`。接不上的原因在 VE 自己这边。

---

## 建议的依赖序（不排期，只给先后）

1. **先定缺口二那个取舍**（历史投影行读不读得回）。它决定 `ProjectionStore` 端口要不要多一个读口，
   这会改后面所有实现票的形状。
2. **再走 `/domain-modeling`**：在 VE CONTEXT 立「替代关系」术语与三条不变量，同步改 UC-VE-002
   输入表与 `AT-VE-044` 的预期措辞。
3. **之后才是代码票**：`AcceptedSourceFactSpec` 加一维 → 迁移加列 →
   `derive_on_transport_handover.go` / `derive_on_effective_delivery.go` 把 `Corrects()` 译进去 →
   派生编排按替代关系分「当前有效 / 已被替代」两类条目 → 客户视图读侧只呈现当前有效。

第 3 步可以拆 A/B，但**要等 1、2 定了才有形状**，现在拆出来的边界会是猜的。

---

## 未做 / 边界

- 未写实现代码，未改任何 `.go` / `.sql`，未跑测试。
- 未登记 `visibility-exception.tracking-projection.derived`，也不建议为让测试变绿去登记——按
  ADR-0049 那是认下的诚实 `no_subscriber`。
- 未细核 `settlement-accounting` 与 `party-commercial` 的更正机制：它们不在 VE 的 `SourceContext`
  封闭五值里，进不了追踪投影。只记一句它们各自也有 `corrects` / `corrected_at` 或
  `commercial_validity_correction` 表，说明这套写法在本仓已有多处先例，缺口二选 A 时有现成样板。
- 「迟到」这半边只核到机制在（三时间分存、`received_at` 主排序、`ReceivedAt` 取记录的
  `RecordedAt` 而非信封时间）。四个已接线适配器目前一律 `EffectiveAt = OccurredAt`，有效时间与
  发生时间尚未在任何一路上分开取值——这不是缺陷（源侧今天也给不出第三个时间），但一旦替代关系
  落地，有效时间会是第二个要重新看的点。观察窗口属实例半边，见 `PILOT-PARAMETER-REGISTER`。
