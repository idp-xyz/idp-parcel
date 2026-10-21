# 53 先于段登记的移动事实永远进不了全程追踪

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg53。此前 ready-for-agent——2026-10-19 分诊为 A（挡演示/上线），批次见 [triage-2026-10-19](../triage-2026-10-19.md)。此前 needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [33](./33-movement-fact-recorded-envelope-and-tracking-projection.md) 的完成记录（代决与越权风险点）。
Blocked by: 无
出处：[ADR-0205](../../../docs/adr/0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 决定四（段不在册照记）与文末补记越权风险点 14。

## 现象

移动事实可以先于它声明的段登记（ADR-0205 决定四），TF 照记。VE 消费移动事实信封时按键回读、沿声明的段取那一刻在控的对象；段此刻不在册就入账不派生——等不来，派发的失败预算也撑不到段登记那一天。段日后登了，没有任何东西回头让这封信重投，于是这条出发（以及它读数里的阻断）永远不进投影、不形成信号。

离线补登、交接口与作业事实口各走各的设备时，这种先后颠倒在一线是常态，不是偶发。

## 证据（读码）

- `internal/visibilityexception/adapters/transportfulfillment/derive_on_movement_fact.go` 的 `objectsOf`：段不在册交回空，入账不派生。
- `internal/transportfulfillment/application/record_movement_fact.go`：信封只在登记那一笔交出一次；同内容重投会重交，但没有人会为「段登了」去重投一条已在册的事实。

## 三类

机制。规模 M。

## 最小的诚实修法

两条路，二选一由 TF / VE owner 裁：

1. TF 在段首次成立（`EstablishSegmentWith*` 落册）那一笔里，按段查「声明了这一段、发生时刻落在段内某个参与区间里」的在册移动事实，逐条重交它们的已形成信封（ID 不变，入队那一步吞掉已交过的；已交过而 VE 那边入账了的，要换一个「段已成立」的唤醒信封，否则重交的同 ID 被吞）。
2. VE 订阅段成立（或参与进入）信封，回头按段查在册移动事实再派生。要 TF 给一个按段列移动事实的读口。

第二条把「谁来补」放在消费方，与 ADR-0066 消费侧展开同向；第一条要 TF 知道 VE 的需要。

## 要裁的

- 走哪一条；补投时对象仍按事实发生那一刻在控取（与 ADR-0205 决定四同口径）。
- 读数是登记那一刻问的（段不在册答未决），补投不重问关务；要不要连带补问，归补问机制（ADR-0205 越权风险点 2）。

## 既有票

none

## 完成记录（2026-10-20，Claude Code 作者子代理，分支 `claude-dcg53`）

授权：用户原话「你作为业务和系统专家自决……解决所有的问题」，并于 2026-10-19 同意只做分诊 A 组。协调方代决走票面第二条的变体：全程追踪在消费侧补，经运输履约新开的只读口按段列出在册移动事实；评审（另一协调会话的隔离评审）后补上场外揽收那一路。

### 做了什么

- **决定**：[ADR-0224](../../../docs/adr/0224-tracking-backfills-movement-facts-recorded-before-their-segment-when-a-control-fact-puts-the-object-in-it.md)（`5b46613f`）。回答 ADR-0205 文末补记越权风险点 14；ADR-0205 只在 Status 行与 Links 加前向指针，不停用任何条款。
- **TF 只读口**（`54fcafac`、`b588226a`）：ports `MovementFactsOfSegment.FindBySegment`，postgres 实现与按键取共用一份行装回（`movementFactRow`）；迁移 `0044` 加 `(tenant_id, segment_ref, fact_ref, recorded_at, version) WHERE segment_ref IS NOT NULL` 部分索引。领域加 `FulfillmentParticipation.EnteredByPickup`，揽收入场依据的拼写收进 `pickupParticipationBasis` 一处。
- **VE 补派生**（`b61da14e`、`63658eb8`）：`BackfillMovementFactsAdapter` 一个处理方接交接与对象级揽收两类信封。交接只认转出控制的那一版；只看这一次控制事实让对象进的那一段（`EnteredByHandover` / `EnteredByPickup`），认到即停；按段列事实，事实发生那一刻对象在控（`ObjectsUnderControlAt`）的逐版补派生。派生与移动事实那一路共用 `deriveMovementFactOnto`，同一把幂等键；只补投影，不补信号、不重问关务。交接与揽收的回读核对各抽成一处（`findRegisteredHandover`、`findRegisteredPickup`），与同一封信上的投影共用。两个新消费门各记一本账。
- **派发**（`dfa125bf`、`408f373f`）：新文件 `routes_movement_fact_backfill.go` 与 `assemble_movement_fact_backfill.go`，交接与揽收信封的扇出各加一路，名次 `positionProjection + 10`，各带自己的未决哨兵名单。没改 `routes.go`、`assemble.go` 等共享文件。
- **用例与盘点**（`5b46613f`）：UC-VE-002 新立 `AT-VE-178`–`182`；UC-TF-005 新立 `AT-TF-273`–`274`，`AT-TF-159` 加指向补派生的一句。AT 盘点只加行与追记，「总览」计数不改。`AT-TF-275`–`277` 没用到。
- **后继票**：[120](./120-movement-facts-before-a-pickup-attempt-or-carrier-segment-are-not-backfilled.md)（揽收执行口与承运商段）、[121](./121-departures-recorded-before-their-segment-are-never-re-asked-at-the-gate.md)（先于段的出发不补问门禁）。票 [114](./114-deterministic-failures-answered-as-retryable-undecided-across-consumers.md) 加了一段，点名本票新开的几处未决包装。
- 演示种子不改：参考映射里早有 `movement-departure → DEPARTED`。NO 地盘（`internal/nodeoperations/**` 等）没碰。

### 撞车与核对

另一协调会话向本 worktree 派过一个同票作者，比本作者晚约 40 秒开工，两边一度同时写这棵树。按它的 journal，它只重写过 `internal/transportfulfillment/adapters/postgres/movement_fact_registry.go` 里从 `FindByKey` 到 `Save` 之前那一段，用来贴合本作者的 `FindBySegment`；它自建的 `ports/movement_fact_by_segment.go` 已删，别的没落盘。本作者逐行重读过那一段：选列多了 `fact_ref` 与 `version`，WHERE、逐列构造门、重建与错误包装都同原样；`Save` 一字未动。TF postgres 真库全包测试照过。那一段注释是对方的措辞，保留。分支上其余改动与文件对过 `git diff 6a05b431`，都是本作者写的。

### 代决与越权风险点

代决（逐条理由在 ADR-0224 Decision 与 Alternatives）：

1. 由全程追踪在消费侧补；运输履约不重交信封，也不造「段已成立」信封。
2. 运输履约只开一个只读口，排序契约是先按事实、再按登记时刻、再按版本串；不问段在不在册；一行装不回整次报错。
3. 用对象级的控制事实登记信封唤醒，交接与对象级揽收各一路；只认这一次控制事实让对象进的那一段，只补这一件。
4. 判据与幂等键同正常路径；更正链逐版补，按读口次序。
5. 只补投影，不补信号、不重问关务。
6. 失败分格同移动事实那一路，名单各路自立。
7. 揽收执行口（尝试级信封）与承运商首次有效收寄立的段不在本票（票 120）。承运商段判为不适用。

越权风险点（详见 ADR-0224 文末）：

1. **进段欠账时补不上**（TF、VE owner）。答复带 `SegmentContinuationReference`、信封已消费过之后，续办再把对象补进段，不触发任何东西，那一份出发照丢。
2. **揽收执行口与承运商段不补**（TF、VE owner）。执行口收寄的对象连投影都没有，票 120。
3. **补出的出发不带该件的门禁读数**（VE、CC owner）。受限包裹在追踪上显示成普通 `DEPARTED`，不形成阻断信号（ADR-0205 越权风险点 2、17），票 121。
4. **按消费那一刻段册算**（VE owner）。参与日后被更正、起点晚于事实时，已补的不撤。
5. **每次控制事实多读段与事实**（运维）。要读整段连同全部参与；按段列事实不分页。
6. **只补控制事实那一件**（VE owner）。凭存量迁移、手工直写段册进段的对象不补。
7. **次序靠登记时刻**（VE owner）。时钟回拨时中间多一版投影，结论不变。
8. **确定性坏行记成未决、反复重投**（运维）。票 114 已点名这几处。

### 评审意见的处置（另一协调会话隔离评审，看的是约 20:45 的未提交树）

- 阻断 1（只挂交接）：补上对象级场外揽收那一路（`b588226a`、`63658eb8`、`408f373f`）。揽收执行口没有对象级信封，承运商段判为不适用，两者归票 120；本票就这两类段而言是部分修复。
- 阻断 2（文档写全）：ADR-0224、本记录、UC 行与 AT-TF-159 指针都已落。进段欠账记为越权风险点 1，补出的出发不带门禁读数记为越权风险点 3 与票 121。
- 建议 3（生产路由真库用例与失败分区）：真库用例在 `cmd/parcel-dispatch/movement_fact_backfill_test.go`，交接、揽收各一条。失败分区用例两路各跑：未决哨兵进 `consumer_undecided`，`ErrMovementFactRecordInconsistent` 等进 `publish_failed`。
- 建议 4（坏行被包成未决）：本票不修，票 114 已加一段。
- 建议 5（注释计数）：去掉了 `routes_movement_fact_backfill.go` 的「多三站」与 `deriveMovementFactOnto` 的「三个时间」，同类的「信封四维 / 三维」改成「信封键」。
- 建议 6（TF 端口注释）：只留排序契约；为何按这个次序重放，写在 VE 的 `backfill` 那一侧。
- 可后补：
  - 读量：现在一次控制事实认到它进的那一段就停，从入场最晚的段往前认。读整段连同全部参与的代价仍在，记为越权风险点 5。
  - 端口风格：`facts` 换成本包窄口 `SegmentMovementFactLister`。
  - 票面状态：已改。

### 用例

- VE 替身用例（`internal/visibilityexception/adapters/transportfulfillment/backfill_movement_facts_test.go`）：交接与揽收各补出一份；补派生与正常路径不论先后、补派生重投，都只留一份；不补的各格；失败哨兵；更正链。
- 消费门真库用例（`internal/visibilityexception/adapters/inbox/movement_fact_backfill_consumer_test.go`）：同一封信与投影各记一本账；毒丸。
- TF 真库用例（`internal/transportfulfillment/adapters/postgres/movement_fact_segment_lookup_test.go`）：版本、次序、排除项；空段；坏行整次报错。
- 领域用例（`internal/transportfulfillment/domain/pickup_participation_basis_test.go`）：`EnteredByPickup` 认进段那一版与替代参与，不认别的版本或别的对象。
- 派发真库用例（`cmd/parcel-dispatch/movement_fact_backfill_test.go`）：先登出发、后经生产登记编排凭交接或揽收立段，生产路由一拍后追踪出现 DEPARTED；两路越过 inbox 重投不出第二条；失败分区。
- 提交前对变异抽验过：去掉入场认法或在控判据，对应用例转红。

### 验证

- `go build ./...`、`go vet ./...`、`gofmt -l` 干净；`go test ./tools/...` 绿。
- `scripts/test-full.sh` 跑了本票动过的包及其反向依赖（含测试导入），连同 `./internal/architecture/...`、`./migrations/...`：`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-transport-register/`、`./internal/transportfulfillment/...`、`./internal/visibilityexception/...`，以及 NR、NO、PP、PS、SA 各自的 `adapters/transportfulfillment`。36 个包全绿：首轮在 `dfa125bf` 的树上，二轮在补揽收之后的树上，代码等于 `408f373f`。
- 没跑全量；ADR 索引、机制清点与 AT 计数没有重生成。`tools/adr-index -check` 因 ADR 索引没重生成而不一致，这是预期，进 main 时由推送方重生成。
