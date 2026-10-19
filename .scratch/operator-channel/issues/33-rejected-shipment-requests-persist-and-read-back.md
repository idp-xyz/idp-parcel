# 33 「已拒绝」委托写得进、读不回，主动拒绝留痕不落库：拒绝口与处置「拒绝」去向的重放、提交口原样重放、「已拒绝修正」关联都读不回已拒绝委托

Category: bug
Status: in-progress——2026-10-18 完工；评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-18 作者子代理完工，待评审。此前 in-progress——2026-10-18 会话 `idp-parcel-9b` 立并认领（票 15 之三、之四补核发现；读侧今天可达，优先；用户授权代决）；分支 `claude-oc33`
Blocked by: 无
父票：[15](./15-operation-decision-faces-take-operator-intake.md)
出处：票 26 遗留研判中补核票 15 之三、之四（簇 C6）发现；补记随 operator-channel/39。
地盘：
- internal/parcelshipment/adapters/postgres/shipment_request.go
- internal/parcelshipment/adapters/postgres/shipment_request_test.go
- internal/parcelshipment/domain/rehydration.go 及其测试（必要时 active_rejection.go）
- internal/architecture/rehydration_gate_test.go（新增导出的重建标识符时）
- docs/adr/（新 ADR 一份）
- docs/adr/0061-accepted-shipment-request-rehydration-by-snapshot-expressiveness.md（Links）
- docs/adr/0030-rehydration-admits-one-state-at-a-time-by-snapshot-expressiveness.md（Links）
- docs/adr/README.md（新增一行）
- cmd/parcel-api/assemble_submission_test.go
- cmd/parcel-dispatch/authorized_disposition_loop_test.go
- cmd/parcel-dispatch/manual_review_resume_loop_test.go
- 票 15 补记下的回写行（只加行）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

新 ADR 用号 0188（本会话已占；对端从 0189 起）。

## 现象

1. **写侧丢了留痕。** 经生产仓储主动拒绝（或授权处置选「拒绝」）之后，库里的状态是`已拒绝`，但快照 `decision` 里只有决定标识、`accepted:false`、空依据和决定时刻。`acceptanceDecisionDocument` 里没有主动拒绝的授权引用、实际决定方、结构化原因、证据这几格，`decisionDocumentOf` 也不写，留痕在落库那一刻就丢了。处置「拒绝」的行在任务的处置记录里还留着处置人和原因，决定上没有。
2. **读侧。** 不带商业依据的拒绝在读回时，到 `commercialBasisDocument.snapshot` 解码这一步就报裸错「blank value: commercial resolution ID」，走不到重建门。这类拒绝包括主动拒绝、处置「拒绝」，以及`没有适用合同`形成的规则拒绝（`ShipmentRequest.Decide` 注释：「`没有适用合同`形成的拒绝恰恰带不出依据」）。带依据的规则拒绝能走到 `admitRehydratedState`，得到 `ErrRehydrationStateNotSupported`。前一种情况下，`ErrRehydrationStateNotSupported` 和 `ErrInvalidRehydratedShipmentRequest` 两个哨兵都不中，绕开了 ADR-0029 / ADR-0030 的分格。对真实写下的已拒绝行，下面两处说法都不成立：适配器 `ShipmentRequests` 头注「读回`已拒绝`/`已撤回`仍得到 ErrRehydrationStateNotSupported」，以及 ADR-0061 Consequences「`已拒绝`/`已撤回`仍会让运维看到「等这扇门开到那个状态」」。`TestUnsupportedStatesSurfaceOnRead` 只改了状态列，用的是已提交形状的快照，测不到这个问题。
3. **后果：凡是按来源身份读委托的编排都会出错。**
   - 主动拒绝口：原样重放、换人换原因都答 200 `UNDECIDED` / `SHIPMENT_REQUEST_UNAVAILABLE`，并且每次往 `acceptance_processing_attempt` 追加一行。续办引用指向一场永远不会自己好的重试，等于把已经成立的拒绝说成「请等依赖」。
   - 授权处置口：「拒绝」去向原样重放答 500 `NO_ANSWER_FORMED`。按 ADR-0022，离线客户端看到 5xx 会留在队列里不断重发。生产装配下对已拒绝委托也答 500，而不是 `AUTHORITY_RULES_NOT_CONFIGURED`。
   - 复核完成口：撞上已拒绝委托答 500，而不是`任务已完结`。
   - 提交口（PSC-1）：客户原样重放一份已被拒绝的提交，`SubmitShipmentRequestHandler.resolvePreserved` 报「find existing shipment request」，HTTP 答 500；应当答 `EXISTING_RESULT`。
   - `AT-PS-036`（已拒绝委托修正资料 → 形成关联新委托）：以 `LinkRejectedCorrection` 关联已拒绝的原委托时，`resolvePriorClaim` 报「find prior shipment request」。
   - 接受链：已经排队的「复核已完成」或「委托已提交」信封，撞上已被主动拒绝的委托时，`FormAcceptanceDecisionHandler.Handle` 读不回，交回`委托不可用`的未决，走不到它本来就有的「既有决定」那一支。

## 复现（S；探针不入库，`$DECIDE` = `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide`，用 `go test -overlay` 挂进对应包，带 DSN 跑）

- 仓储：`$DECIDE/C6-adv-impl/zz_c6adv_pg_probe_test.go` · `TestC6AdvRejectedRowsOnRead`。主动拒绝和不带依据的规则拒绝读回都报「blank value: commercial resolution ID」，两个哨兵都不中；带依据的规则拒绝得到 `ErrRehydrationStateNotSupported`（复跑日志：`$DECIDE/C6-ticket15-part3-algebra/rerun_pg.log`）。
- 三口：`$DECIDE/C6/c6_api_probe_test.go` · `TestC6ProbeActiveRejectionThroughTheOperatorIntakeAgainstARealDatabase`：首次答 201，之后两次答 200 `UNDECIDED`，处理尝试从 0 增到 1 再到 2。`$DECIDE/C6/c6_dispatch_probe_test.go` · `TestC6ProbeDispositionReplayAgainstARealDatabase`：「拒绝」重放报错，快照的决定里没有主动拒绝留痕。生产装配：`$DECIDE/C6-adv-impl/zz_c6adv_api_probe_test.go` · `TestC6AdvDecisionFacesOnARuleRejectedRequestUnderProductionWiring`，以及 `$DECIDE/C6-adv-authority/c6a_api_probe_test.go` · `TestC6AProbeProductionAssembliesAgainstARejectedRequest`。
- 提交口与关联：`zz_c6adv_api_probe_test.go` · `TestC6AdvSubmissionFaceOnARuleRejectedRequest`（复跑日志：`$DECIDE/C6-ticket15-part3-algebra/rerun_adv_api.log`）。
- 对照：`$DECIDE/C6/c6_app_probe_test.go` 用替身仓储，同样的重放交回既有决定，或答 `TASK_ALREADY_CLOSED`。说明领域代数是在的，断在持久化。

## 应当

- PS CONTEXT「运营企业主动拒绝尚未接受的委托时……并保存结构化原因、证据、适用范围、授权依据、实际决定方和决定时间」（「适用范围」见「不做」）；「接受或拒绝提交后，另一方只能读取既有结果，不能追加相反决定」。
- ADR-0132 决定一「`拒绝`……留痕与主动拒绝同一份要求：实际决定方、授权依据、结构化原因、证据、决定时间」；决定二「撞上已成立的决定答`任务已完结`并交回那一个」。
- UC-PS-001 `AT-PS-036`：已拒绝委托修正资料时形成关联新委托，「保留原版本或原决定」。
- ADR-0029：按恢复动作分格。一行完好的数据，不能报成既不是「门未开」、也不是「坏数据」的裸错。

## 根因

ADR-0030 / ADR-0061 只挡住了读：`已拒绝`因为快照缺主动拒绝的表达，不准重建（ADR-0061 决定二的表格：「`withdrawal` / 主动拒绝 | **无** | 故`已拒绝`/`已撤回`仍不开」）。写侧却照常落`已拒绝`行，主动拒绝的留痕因为没有表达被静默丢掉；依据解码又对所有决定都要求商业解析标识非空。之三、之四没有核重放代数，所以一直没人走到这条路。

## 裁定（机制；代 PS owner）

照 ADR-0030「某个状态要开门，先补齐它可达字段的表达，再把它从拒绝名单里放出来」，在同一张票里分两笔，按顺序做：

1. **第一笔：先补表达（写侧止损，解码归位）。** 快照文档的决定上增设主动拒绝留痕（授权引用、实际决定方、结构化原因、证据），`decisionDocumentOf` 负责写，解码负责读。依据缺席按**拒绝决定**来放宽：规则拒绝（没有适用合同时）、主动拒绝、处置「拒绝」都可以没有依据；接受决定照旧必须带。如果只对「主动拒绝」放宽，会漏掉规则拒绝那一类。这一笔不动重建门。落地之后，不带依据的已拒绝行读回时改为得到 `ErrRehydrationStateNotSupported`，哨兵归位。
2. **第二笔：再开门，另立新 ADR**（写法照 ADR-0061，例如「已拒绝委托的重建门按快照表达能力开门」），推进 ADR-0061 决定一里`已拒绝`那一半。不在 ADR-0061 后面追加复核记录来开门：复核记录只用来记认可，或者指向改动它的新记录（先例是 ADR-0125 的 owner 复核记录「本正文不改，以 ADR-0132 决定三为准」）。新 ADR 必须包含：
   - 对账表：`已拒绝`三条来路（`ShipmentRequest.Decide` 的规则拒绝、`RejectByAuthority`、`DisposeUnderAuthority` 的「拒绝」）下，每个字段在场还是缺席。字段包括：决定的标识、校验明细、依据、复核状态、决定时刻、主动拒绝留痕；任务状态、等待态、处置记录；基线、承诺、资料版本、撤回；历史版本与任务、关联出处、客户包裹件号。处置「拒绝」形成的行，决定上的留痕要和任务处置记录逐格对得上（即 `DisposeUnderAuthority` 的逐格翻译）。
   - 坏数据判据（`ErrInvalidRehydratedShipmentRequest`，写法照 ADR-0061 决定三）：一个拒绝决定必须恰好带有「失败校验项」和「主动拒绝留痕」中的一样。两样都没有、两样都有，或者带了基线、承诺、资料版本，都算坏数据。补表达之前写下的主动拒绝行和处置「拒绝」行，据此按坏数据拒绝，不从任务处置记录推回来，依据是 ADR-0028「只校验不重算」。而且生产装配上拒绝授权和处置授权从来没接上过，这类行只可能出自带合成授权的一次性测试库。
   - `已撤回`仍然用 `ErrRehydrationStateNotSupported` 拒绝。
   - 文档处理照 ADR-0030 → ADR-0061 的先例，写成「推进」：ADR-0061 的 Links 加前向指针，并写明「Status 与其余各条不变」；ADR-0030 的 Links 加一条推进指针；adr/README 新增一行。
   - `ActiveRejection` 没有公开构造器。按 ADR-0030 的判别式（「看它有没有公开构造器；没有的才是产物，才要新面」），它是产物，它的重建表达要进重建面。如果新增了导出的 `Rehydrate*` 标识符，同一笔更新 `internal/architecture/rehydration_gate_test.go` 里的受限名单。

## 做什么

1. 第一笔：`acceptanceDecisionDocument`、`decisionDocumentOf`、`acceptanceDecisionDocument.spec` 带上主动拒绝留痕；`commercialBasisDocument` 对拒绝决定允许缺席；把适配器 `ShipmentRequests` 的头注改成与事实一致。
2. 立新 ADR（内容见裁定），与第二笔同时或先于它落。
3. 第二笔：重建规格带上主动拒绝留痕；`admitRehydratedState` 放进`已拒绝`；`validForRehydration` 按对账表补上`已拒绝`的跨字段校验和坏数据判据。
4. 回归用例，见完成判据。

## 不做

- 主动拒绝口、处置口撞上已有决定时的答复格，以及重放耗标识（归 operator-channel/36）。
- `已撤回`的表达与开门（撤回口属于客户渠道，生产上挂的是未配置；另立票）。
- PS CONTEXT 里的「适用范围」：领域 `ActiveRejection` 没有这一格（`AT-PS-034` 也写了「记录结构化原因、范围和依据」），这是已有的领域缺口，交 PS owner。本票只按领域可达字段对账，不宣称满足了 CONTEXT 那一整句。

## 是否潜伏

- 写侧丢留痕：潜伏。生产装配上拒绝授权请求映射为 nil（`buildRejectionOrchestration`），授权处置装的是 `UnconfiguredAuthorizedDispositionAuthorizer`（`buildDispositionOrchestration`），形成不了主动拒绝或处置「拒绝」。必须在 psb/07 接上拒绝映射、PC 词表补上「授权处置」之前进 main；否则第一条真实主动拒绝的留痕就会丢，而且补不回来。
- 读侧：不受授权缺口保护。三口的编排都是先 `FindBySourceIdentity`、再问授权，提交口和关联根本不经过授权。只要库里有已拒绝的行就能触发。演示链路已经能形成决定（psb/26 完成记录：委托成`已接受`），形成规则拒绝只差一份校验不过的输入；不过立票时没有在演示库里实跑这一步。优先修。

## 完成判据

- [ ] 新 ADR 已立并接受（含对账表、坏数据判据、文档处理）；ADR-0061、ADR-0030 的 Links 和 adr/README 已改。
- [ ] 仓储真库：规则拒绝（带依据、不带依据各一）、主动拒绝、处置「拒绝」，都经真实写路径写下再读回，决定和留痕逐格相等。缺留痕的旧形状行、两样都有的行，读回得到 `ErrInvalidRehydratedShipmentRequest`。`已撤回`仍得到 `ErrRehydrationStateNotSupported`。改写 `TestUnsupportedStatesSurfaceOnRead`（`已拒绝`不再算门外），并加一例真实写下的已拒绝快照，不只改状态列。
- [ ] 领域重建门：三条来路各一例能过门；每种坏数据形状各一例被拒。
- [ ] 消费方真库回归：
  - 提交口原样重放一份已被规则拒绝的提交，答 `EXISTING_RESULT`；
  - 以 `LinkRejectedCorrection` 关联已拒绝的原委托，关联成立；
  - 处置「拒绝」去向原样重放，答 `TASK_ALREADY_CLOSED` 并交回 `REJECTED`；
  - 「复核已完成」信封撞上已被主动拒绝的委托时，接受链交回既有决定，不落成`委托不可用`。
- [ ] 「先补表达、后开门」两笔的顺序在 `git log` 上看得出来。
- [ ] 在票 15 补记下追加回写，只加行。
- [ ] 走 Go 那条路（并行会话）：开分支；作者自验（gofmt / vet / build，改动包带 DSN 跑 `go test -p 1 -count=1`）；非作者评审；推送方带 DSN 跑全量。

## 完成记录（2026-10-18，会话 idp-parcel-9b 的实施子代理，分支 claude-oc33）

**落点**（基线 `b58b10ef`，五笔，未推送）：

| 笔 | 做了什么 |
|---|---|
| `cd46fd0f` 先补表达 | 快照决定增设 `activeRejection`，`decisionDocumentOf` 写、解码读；依据缺席按拒绝决定放宽（`commercialBasisDocument.absent`），接受决定照旧必带；重建面新增 `RehydrateActiveRejectionSpec` 并随 `RehydrateAcceptanceDecisionSpec` 带进聚合，已接受决定带留痕按坏数据拒绝；受限名单同笔登记；适配器头注改成与事实一致。门不动。 |
| `9e84d743` ADR | [ADR-0188](../../../docs/adr/0188-rejected-shipment-request-rehydration-by-snapshot-expressiveness.md)：对账表、坏数据判据、文档处理；ADR-0061、ADR-0030 的 Links 加推进指针，adr/README 加一行。 |
| `5b458dd9` 再开门 | `admitRehydratedState` 放进`已拒绝`；`validForRehydration` 按对账表补`已拒绝`校验（`rejectedDecisionValidForRehydration` / `ruleRejectionHolds` / `activeRejectionHolds`）。 |
| `c39e960e` 消费方回归 | 三个 `cmd/` 真库用例。 |
| `627ff0b1` 票 15 回写 | 只加行。 |

**与票面「做什么」的一处偏离**：重建规格里的留痕那一格（`RehydrateActiveRejectionSpec`）随第一笔落，没有等到第二笔。第一笔要「解码负责读」，解码出来的留痕得有处可放。第一笔里门仍关着，`已接受`带留痕即拒，所以这一格不会让任何聚合静默带错。第二笔只做开门和补校验。

**完成判据逐格**：

- [x] 新 ADR 已立并接受，ADR-0061、ADR-0030 的 Links 与 adr/README 已改：见 `9e84d743`。
- [x] 仓储真库：`TestRejectionsRoundTripWithTheirTrail` 有四格：带依据的规则拒绝、不带依据的规则拒绝、主动拒绝、处置`拒绝`。四格都经真实写路径写下再读回，快照里的留痕、决定与任务都按 `reflect.DeepEqual` 逐格相等。`TestRejectedRowsWithoutOrWithBothExplanationsAreInvalidOnRead` 覆盖补表达前的主动拒绝行、补表达前的处置`拒绝`行，以及两样都有的行，三者读回都得到 `ErrInvalidRehydratedShipmentRequest`。`TestUnsupportedStatesSurfaceOnRead` 已改写：`已接受`/`已拒绝`缺产物算坏数据，只剩`已撤回`得到 `ErrRehydrationStateNotSupported`。真实写下的已拒绝快照由上面的往返用例覆盖。
- [x] 领域重建门：`TestRehydratingARejectedRequestFromEachPathKeepsItsDecision`，四格都过门；`TestRehydrationRefusesImpossibleRejectedShapes`，二十一种坏形状各一例，逐例核过拒绝理由与用例名对得上；另有 `TestRehydrationRefusesAnAcceptedDecisionCarryingAnActiveRejectionTrail`。
- [x] 消费方真库回归：
  - `TestASubmissionReadsBackARuleRejectedRequest`：原样重放答 `EXISTING_RESULT`，`LinkRejectedCorrection` 关联成立。
  - `TestARejectionDispositionReplayedAnswersTaskAlreadyClosed`：答 `TASK_ALREADY_CLOSED`，交回 `REJECTED` 与留痕，不再释放。
  - `TestACompletionEnvelopeMeetingAnActivelyRejectedRequestHandsBackTheDecision`：交回既有决定，不记`委托不可用`，不加处理尝试，交出的决定信封只能是那份拒绝。
- [x] 两笔顺序：`cd46fd0f`（表达）→ `9e84d743`（ADR）→ `5b458dd9`（开门）。
- [x] 票 15 补记下回写：`627ff0b1`。
- [ ] Go 那条路：作者自验已做（见下）。**非作者评审**与**推送方带 DSN 跑全量**未做。

**先红后绿**（日志在 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/oc33/`）：

- 第一笔：
  - 适配器用例当时名为 `TestRejectionsKeepTheirTrailAndMeetTheGateOnRead`，第二笔改写成往返用例。红在两处：不带依据的规则拒绝读回报裸错「blank value: commercial resolution ID」；主动拒绝与处置`拒绝`的快照留痕为 nil。
  - 领域用例红：已接受决定带留痕时 err = nil。
  - 架构门红：`RehydrateActiveRejectionSpec` 两份名单都没有。
  - 实现后三处全绿。
- 第二笔：
  - 只放门、不补校验时，二十一种坏形状全部过门（`red_domain_gate_open.log`）。
  - 门关着时，往返四例、旧形状与两样都有三例、`TestUnsupportedStatesSurfaceOnRead` 都红（`red_adapter_gate_closed.log`）。
  - 消费方三例全红（`red_consumers_gate_closed.log`）：处置重放与提交重放读回得到 `ErrRehydrationStateNotSupported`；续办信封停在可达性那一腿，未决 `REACHABILITY_AUTHORITY_UNAVAILABLE`。注意：续办这条最先读不回的是可达性一腿，不是票面写的形成决定那一步。
  - 补校验并开门后全绿。

**门**：
- `gofmt -l internal cmd`：无输出。
- `go build ./...`、`go vet ./...`：退 0。
- 测试包集合：改动包、直接引用 `parcelshipment/domain` 或 `parcelshipment/adapters/postgres` 的包，加 `./internal/architecture/...`，共 36 个。带 DSN 跑 `go test -p 1 -count=1`，退 0，35 ok、1 个无测试文件。
- 机制清点：重跑无差，没有单独提交。
- 无迁移：快照是 JSON 列。

**证据层级**：S（隔离合成，`SYN-` 值；处置夹具沿用既有用例的值）。

**遗留**：
- 地盘外过期注释一处，未改：`internal/parcelshipment/adapters/postgres/authorized_disposition_test.go` 的 `TestARejectionDispositionIsSavedWithItsWaitStateCleared` 头注写着「读回那一半今天由重建门挡在门外」，现在不成立。
- 「适用范围」交 PS owner。
- `已撤回`另立票。
- 两口撞上已有决定时的答复格和耗标识，归 36。

评审：待会话读 diff 审

## Comments

- **评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审）· 钉 `083bfd70`（基线 `b58b10ef`）· 2026-10-18**：用户 2026-10-18 因 token 花费选「精简续做」，评审由会话本身读 diff 做；代码出自本会话起的实施子代理，不算非作者评审，如实记下。
  - **Spec**：阻断无。写侧补上主动拒绝留痕（主动拒绝与处置「拒绝」都写），依据缺席只对拒绝决定放宽；重建门开到「已拒绝」，按 ADR-0188 对账表逐格校验，「失败校验项」与「主动拒绝留痕」恰有其一，补表达之前写下的行按坏数据拒（不从处置记录推回留痕，守 ADR-0028）。三个消费方真库回归（提交重放、处置「拒绝」重放、续办信封撞上已拒绝委托）在。
  - **Standards**：阻断无。非阻断 1：`authorized_disposition_test.go` 头注「读回那一半今天由重建门挡在门外」随开门失实，已在 `083bfd70` 改指往返用例（作者报告点出的地盘外一句注释）。ADR-0061、ADR-0030 只加 Links 指针，正文未动。
  - **无发现**：领域包未引 HTTP/pgx；新重建面在架构门名单同笔登记；合成值只用 `SYN-`；没有迁移。
