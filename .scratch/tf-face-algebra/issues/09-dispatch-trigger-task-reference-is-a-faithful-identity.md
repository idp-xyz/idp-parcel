# 09 派送发起的确定性任务引用不是忠实身份：键可被手工建任务口占用、拼接不单射，对象首拍就答「已形成」而它并没有派送任务

Category: bug
Status: in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（tf-face-algebra/10 已进 main），分支 `claude-tfa09`。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 代决（见「裁定」）；排在 tf-face-algebra/10 进 main 之后（同碰手工建任务口）。此前 needs-triage——2026-10-18 会话 `idp-parcel-9b` 立（操作者与集成客户端渠道逐口核代数研判；实测发现未经对抗核验，裁定待做，动手前先复现）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：逐口核研判（workflow review26-face-algebra-sweep）：/transport-fulfillment-delivery-dispatch-triggers, /transport-fulfillment-dispatch-task-registrations
地盘：
- internal/transportfulfillment/application/trigger_delivery_dispatch.go 及其测试
- internal/transportfulfillment/adapters/http/trigger_delivery_dispatch.go 及其测试
- cmd/parcel-api/assemble_delivery_dispatch.go（执行器先查在册时接任务登记册读口；同笔清本文件过期头注）
- cmd/parcel-api/（新增真渠道真库回归测试文件）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决；机制与产品策略，不涉租户取值）

用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。修复方向五条逐条裁：

1. **单射铸法：取长度前缀，带形状标记。** 新引用仍以 `DELIVERY-DISPATCH/` 起头，后接形状版本标记，再把段、对象、入场依据各自写成「字节长度 + 分隔符 + 原串」依次拼上。不取摘要：引用会出现在答复、日志与管理台上，人要读得出它指的是哪个段哪个对象。形状标记让新旧两种拼法在字面上不可能相同。旧拼法兼认照「修复方向」第 1 条：先查新键，查不到再查旧键，命中且第 2 条核得过才算已在册；存量不回写。
2. **在册核对作第二道防线：照做。** 新格名 `DISPATCH_TASK_REFERENCE_TAKEN`，HTTP 200 并回显占用者；不写、不另铸、不发意图。
3. **先查在册再取件：查在册放在参与 `Active` 判断之前。** 理由：同一拍的原样重放应当得到同一个答复并回显在册任务，这是本仓重放代数的通则（ADR-0151 复核记录收的读法）；参与离场之后重放答`已在册`，不形成新任务，ADR-0114 决定二「已离场的参与不触发」照样守住。读回段、确认链尾凭已交接这两步仍在查在册之前——键要靠它们定出来。
4. **手工建任务口拒执行器的保留前缀：取。** 手工口指名以 `DELIVERY-DISPATCH/` 起头的引用答`未受理`（400 一类，照该口现行坏输入答法）。存量里已有的这类手工任务不动，靠第 2 条兜住。
5. **清过期头注：照做。**

「不做（观察）」两条维持不做。

越权风险点：代 TF owner 定了第 3 条的放法（离场后重放答已在册而不是`参与不在场`）与第 4 条（手工口收窄）。owner 复核时看：有没有调用方靠「离场后重拍答参与不在场」做判断；有没有运营流程需要手工建一项与派送发起同键的任务（例如补建执行器漏掉的那一项——若有，应改走派送发起口重拍，而不是手工指名同一键）。

## 现象
- **手工口占用。** 先经运营决定渠道的手工建任务口，用执行器的确定性引用建一项任务（kind=PICKUP，对象是别的）。该对象首拍即答 200 `DISPATCH_TASK_ALREADY_FORMED`，回显别人的种类与对象集。
- **拼接撞键。** 段 `SYN-SEG/A` 加对象 `CU-77`，与段 `SYN-SEG` 加对象 `A/CU-77`（交接版本同为 v1），铸出同一引用，后到的对象首拍即答「已形成」。
- **核验补测一：对象取生产形状也能撞。** 对象取不含斜杠的生产形状（`DPCL-` 开头）时，只靠精心构造的段引用与交接版本也能撞键。
- **核验补测二：核种类与对象集不够。** 同一对象换到第二个派送段，段与交接版本经构造后撞上第一段那项任务；这时在册任务的种类与对象集都对得上，所以只核这两样拦不住。
- **入场依据不变就形成不了。** 上述被撞的对象，在入场依据不变期间一直形成不了派送任务；经交接更正换了入场依据后可以脱出（核验补测）。原研判写的「永远」应修正为「入场依据不变期间」。
- **生产缝下的可达条件。** 地点、时间窗、条件三个所有者都给出答复，才走得到撞键；空库上答 `REQUIREMENT_MISSING`。
- **在册之后仍重拉所有者。** 在册要等重拉所有者之后才认。所有者改答或参与离场后，原样重放分别答 `REQUIREMENT_MISSING`、`DISPATCH_UNDECIDED`、`INPUT_NOT_ACCEPTED` 或 `NOT_A_DELIVERY_TRIGGER`，都不回显在册任务；每次重放还要白读一遍所有者。

## 复现（S）
在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 下执行：
`IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay <overlay> -count=1 -p 1 -run '<用例>' -v ./cmd/parcel-api/`

- 原探针：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/delivery-dispatch-triggers/probe_ddt_test.go 的 `TestProbeDeliveryDispatchTriggerKeyTakenByTheManualPortAgainstARealDatabase`、`TestProbeDeliveryDispatchTriggerKeyConcatenationIsNotInjective`、`TestProbeDeliveryDispatchTriggerReplayAlgebraAgainstARealDatabase`（同目录 overlay.json）。
- 对抗与可达核验：同目录 adv_probe_ddt_test.go 的 `TestAdvDDT…`，verify/verify_ddt_test.go 的 `TestVerifyJUnderProductionSeams`、`TestVerifyCollisionWithProductionShapedObjectIDs`、`TestVerifySameObjectCollisionAcrossSegmentsPassesKindAndObjectCheck`。合挂 overlay 为 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/delivery-dispatch-triggers/synth/overlay-all.json，日志为 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/delivery-dispatch-triggers/synth/run-adv-verify.log。

## 应当
- 「已在册」认的是本拍要的那一项任务。答复不能对一个没有派送任务的对象声称「已形成」。
- 同一拍重跑，不随所有者的后续改答漂移，也不白读所有者。

## 根因
- `deliveryDispatchTaskReference` 把段、对象、入场依据以斜杠直拼，即「"DELIVERY-DISPATCH/" + 段 + "/" + 对象 + "/" + 入场依据」。三者都是只拒空白的不透明串，交接版本串跨对象还可以相同。
- 手工口 `dispatchTaskSpecFrom` 只要求引用非空，能指名任意引用。
- `TriggerDeliveryDispatchHandler.Trigger` 对 `DispatchTaskAlreadyRegistered` 不看记录内容。
- `pullRequirements` 排在 `Dispatch.Open` 之前。

## 修复方向（机制）
1. 引用改成单射铸法：逐段转义、加长度前缀，或用带形状版本的规范化摘要，由实施方定。
   - 必须兼认旧拼法下已在册的任务：先查新键，查不到再按旧拼法查；命中且下面第 2 条核得过，就算已在册。
   - 存量不回写，与 ADR-0014、ADR-0150 守同一纪律。
2. 在册任务核对作第二道防线：种类须为 `DeliveryDispatch`，对象集须恰为该对象。对不上时答新格（例如「任务引用已被另一项任务占用」，200，带回显），不写、不另铸、不发意图。单靠这一条拦不住同对象跨段撞键，所以第 1 条是主修。
3. 先查在册再取件：定出键、读回段并确认链尾凭已交接之后，先按键查在册，在册就不再向所有者取件。查在册放在 Active 判断之前还是之后，决定参与离场后的重拍答已在册还是 `PARTICIPATION_NOT_ACTIVE`，归 TF owner；ADR-0114 决定二「已离场的参与不触发」两种放法都守得住。
4. 可选，归 TF owner：手工建任务口拒执行器的保留前缀。
5. 同笔清 `buildDeliveryDispatchTrigger` 头注里已过期的「端点挂 UnconfiguredIntake{} 起步」。

## 不做（观察）
- 任务登记册读不动时，`DISPATCH_TASK_UNDECIDED` 的续办引用跨租户相同（`TestAdvDDTDispatchTaskUndecidedContinuationOmitsTheTenant`），出自 `OpenDispatchTaskHandler.Open` 的未决摘要。要不要进租户，归 TF owner。
- 入场依据进键（票 tf-segment-lifecycle-closure/09 越权风险点）：交接更正后会另立任务、前一项不动，按 ADR-0114 字面是新身份。

## 是否潜伏
已可触发，但属边角。生产装配上 `deliveryDispatchTriggerIntake = workFactTransport`，三条缝都接真，没有授权映射，作业事实铸造也不判准入。触发还需要两样之一：手工口占了那串引用；或者段引用、交接版本里带斜杠，同时三个所有者都给出答复。

## 完成判据
- [x] 编排单测：
  - 新铸法单射，对拼接撞键那组输入铸出不同引用；
  - 旧拼法在册任务兼认；
  - 被手工口占用答新格；
  - 在册后不再取件（所有者替身计数为零）。
- [x] 真渠道真库回归（`cmd/parcel-api` 新测试文件，作业事实 Intake + 生产 `buildDeliveryDispatchTrigger` 或三缝替身 + 运营决定渠道手工口）。下列各组各成其格：
  - 手工口占用：答新格并回显占用者；
  - 拼接撞键：两个对象各自形成任务；
  - 生产形状对象撞键：两个对象各自形成任务；
  - 同对象跨段：在第二段形成自己的任务；
  - 所有者改答后原样重放：答已在册并回显。
- [x] 并发首拍真库回归照过：一笔 201，另一笔读回赢家答已在册。
- [x] `go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...`，全部 ok。
- [x] 新增生产文件时，在干净检出上重生成 `docs/product/MECHANISM-INVENTORY.md`。
- [ ] 票 15 补记下追加回写行（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：作者自验；非作者评审，两轴分开；推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的实施子代理，分支 claude-tfa09）

基线 `19d9d486`。提交：`465ffe67`（修复、用例与 ADR-0114 复核记录）、`c166efa0`（机制清点）。

### 落点
- `internal/transportfulfillment/application/trigger_delivery_dispatch.go`
  - 第 1 条：`deliveryDispatchTaskReference` 改铸 `DELIVERY-DISPATCH/` + 形状标记 `v2` + 段、对象、入场依据各一节「/字节长度:原串」。例：段 `segment-1`、对象 `parcel-1`、入场依据 `TRANSPORT-HANDOVER/handover-result/parcel-1/v1` 铸出 `DELIVERY-DISPATCH/v2/9:segment-1/8:parcel-1/46:TRANSPORT-HANDOVER/handover-result/parcel-1/v1`。读回由 `parseDeliveryDispatchTaskReference` 担，长度只认不带前导零的十进制。旧拼法留作 `legacyDeliveryDispatchTaskReference`，只用来兼认。
  - 第 2、3 条：新增 `registeredTask`。它排在读回段、确认链尾凭已交接之后，参与 `Active` 判断之前。先查新键，有任务就核：核得过答`已在册`，核不过答新格 `DISPATCH_TASK_REFERENCE_TAKEN` 并回显占用者。新键查不到才查旧键，命中且核得过答`已在册`。在册就返回，不判在场、不查缝、不取件。
  - 核对由 `deliveryDispatchRegisteredAnswer` 做：种类须是 `DeliveryDispatch`，对象集须恰为本对象。任务口落库撞键读回的赢家也走这道核对。
  - `TriggerDeliveryDispatchDeps` 加必备的 `Tasks ports.DispatchTaskReader`。
  - 新增导出 `IsReservedDeliveryDispatchTaskReference`，起头常量单一出处。常量名不以 Prefix 结尾，因为标识前缀门禁不许前缀含横线，而这串不是标识前缀。
- `internal/transportfulfillment/adapters/http/open_dispatch_task.go`：第 4 条。`DispatchTaskPayload.Command` 遇到以 `DELIVERY-DISPATCH/` 起头的 task，答 `ErrMalformedRequest`，也就是 400 MALFORMED_REQUEST：不带 outcome，不进编排。拒在载荷翻译里而不在编排里，因为执行器走的也是那条编排。
- `internal/transportfulfillment/adapters/http/trigger_delivery_dispatch.go`：只改注释。答复形状注释补`引用被占`带占用者，状态码注释补这一格（200）；文件头注同类过期句「Intake 以 UnconfiguredIntake{} 起步」改为「由装配点给」。
- `cmd/parcel-api/assemble_delivery_dispatch.go`
  - 抽出 `deliveryDispatchTriggerOver(db, places, windows, conditions)`：段登记册、任务登记册与事务只接这一处，读口与任务口用同一只 `tfpostgres.DispatchTasks`。`buildDeliveryDispatchTrigger` 建好三条真缝后转交给它；真库回归给它三缝替身。
  - 第 5 条：头注过期句换成现状——端点挂作业事实渠道的 Intake（ADR-0149 决定二；ADR-0151 越权风险点 7）。
- `docs/adr/0114-…md` 文末追加「复核记录（2026-10-19，决定原文不动）」，指回本票裁定，不复述。

### 裁定之内的落法（供评审核）
- **第 4 条「400 一类，照该口现行坏输入答法」**：落为该口现行坏报文的答法，即 400 MALFORMED_REQUEST。没有用 200 INPUT_NOT_ACCEPTED。
- **第 3 条的先后**：「确认链尾凭已交接」移到查在册之前，原先它在 `Active` 判断之后。连带一格答法变了：参与既已离场、又不是凭交接入场时，现答 `ENTRY_NOT_BY_HANDOVER`，原答 `PARTICIPATION_NOT_ACTIVE`。两格都属`不是触发事实`，既有用例没有覆盖这一组合。
- **旧键命中而核不过**：不答新格，照新键形成本拍的任务。依据是第 1 条「命中且第 2 条核得过才算已在册」——旧键只是兼认的去处，不是本拍身份，谈不上「另铸」。第 2 条的新格只给新键上的占用者。
- **旧拼法串本身合新形状时不按旧拼法查**：第 1 条说两种拼法「字面上不可能相同」，但段与交接版本是调用方给的任意串，旧拼法字面上能拼成新形状。实测可构造：段 `v2/1:a/8:parcel-1/50:TRANSPORT-HANDOVER/x` 加交接版本 `y` 的旧拼法串，恰等于段 `a`、交接版本 `x/parcel-1/TRANSPORT-HANDOVER/y` 的新引用。同一对象时核对拦不住，所以在查旧键处守这一条。去掉这道守卫，`TestALegacySpelledTaskIsStillRecognised/旧拼法串本身合新形状：不按旧拼法认` 即红（已做变异核验）。
- **查在册读不回**：答未决 `DISPATCH_TASK_UNDECIDED`，续办引用取执行器自己的那一份（含租户）。
- **已知局限**：新键上若查到一项按旧拼法铸下的存量（前提是存量里有以 `v2/<长度>:` 起头的构造段），核对过得了就认。存量不回写，无从区分来历，只能如实记下。

### 各格对应用例
- 编排单测，`internal/transportfulfillment/application/trigger_delivery_dispatch_task_reference_test.go`（新文件）：
  - 新铸法单射：`TestConcatenationCollidingInputsMintDifferentTaskReferences`；形状字面例子另钉在 `TestTheTaskReferenceSpellsEachPartWithItsLength`。
  - 旧拼法在册任务兼认：`TestALegacySpelledTaskIsStillRecognised`，三个子用例——旧键上是本拍那一项；旧键上是别的任务；旧拼法串本身合新形状。
  - 被手工口占用答新格：`TestAReferenceTakenByAnotherTaskIsAnsweredAsTaken`，四种占用者各一个子用例，另有「任务口读回的赢家不是本拍那一项」。
  - 在册后不再取件：`TestARegisteredTaskIsAnsweredWithoutAskingTheOwnersAgain`。重放前依次让地点答没有、时间窗读不到、条件换引用、地点缝不接、参与离场，每次重放后断言所有者替身计数为零；另有 `TestAnUnreadableTaskRegistryIsUndecidedBeforeAnyOwnerIsAsked`。
- HTTP 单测：
  - `trigger_delivery_dispatch_test.go` 的 `TestATakenTaskReferenceIsAFormedAnswerEchoingTheOccupant`：200 并回显占用者。
  - `open_dispatch_task_test.go` 的 `TestTheDispatchTaskPayloadRefusesTheDeliveryDispatchReservedReference`：保留起头拒，长得像的照常翻译。
- 真渠道真库回归，`cmd/parcel-api/delivery_dispatch_task_reference_test.go`（新文件）。触发走 `workFactTransportForTest` + 生产端点构造 + `deliveryDispatchTriggerOver`（生产段与任务登记册、生产事务包装、三缝替身）；手工口走 `operatorDecisionTransportForTest` + `buildSegmentOperations`。
  - 手工口占用：`TestTheManualFaceCannotTakeTheExecutorsReferenceAndAStockOccupantIsAnsweredAsTaken`。手工口指名新、旧两种拼法都答 400 MALFORMED_REQUEST，无 outcome，不落行。存量占用者经编排落库后，触发答 200 `DISPATCH_TASK_REFERENCE_TAKEN` 并回显它，库仍 1 行，所有者 0 次。
  - 旧拼法存量（兼认与不再被占）：`TestStockTasksOnTheLegacySpellingAreRecognisedOrStepAround`。
  - 拼接撞键：`TestConcatenationCollidingObjectsEachFormTheirOwnTask`。
  - 生产形状对象撞键：`TestProductionShapedObjectsCraftedToCollideEachFormTheirOwnTask`，对象取不含斜杠的 `SYN-DPCL-0901` / `SYN-DPCL-0902`。
  - 同对象跨段：`TestTheSameObjectInASecondDeliverySegmentFormsItsOwnTask`。
  - 所有者改答后原样重放：`TestAReplayAfterTheOwnersChangeTheirAnswersEchoesTheRegisteredTask`。改答含对象凭下一次交接离开派送段；每次重放回显与首拍逐格相同，所有者 0 次，库 1 行。
- 并发首拍：`TestConcurrentFirstTicksFormOneTaskAndTheOtherReadsTheWinner`。两拍在取条件处会合；会合超时即判没测到并发，用例红。结果一笔 201、一笔 200 已在册，回显同一项。

### 先红后绿
- 复现：票面「复现」节的 overlay 未用，改由新写的用例先红。
- 红：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa09-red.log`。跑红之前只加了不改行为的骨架：deps 的 `Tasks` 字段、新结果格及其 String、装配抽出 `deliveryDispatchTriggerOver`。红的原话举几条：
  - 「任务引用 = "DELIVERY-DISPATCH/segment-1/parcel-1/TRANSPORT-HANDOVER/handover-result/parcel-1/v1", want "DELIVERY-DISPATCH/v2/9:segment-1/…"」
  - 「seg 的 A/CU-77：<nil> "DISPATCH_TASK_ALREADY_FORMED"（交回任务 DELIVERY-DISPATCH/seg/A/CU-77/TRANSPORT-HANDOVER/v1 对象 [CU-77]）」
  - 「地点所有者改答没有后原样重放 → 200 {"outcome":"REQUIREMENT_MISSING","missing":["DELIVERY_PLACE"]}」
  - 「手工口指名 DELIVERY-DISPATCH/v2/26:… → 201 {"outcome":"DISPATCH_TASK_OPENED",…}」
  - 「段 SYN-SEG-09-U/SYN-DPCL-0903/TRANSPORT-HANDOVER/X 对象 SYN-DPCL-0903 那一拍 → 200 {"outcome":"DISPATCH_TASK_ALREADY_FORMED",…"objects":["SYN-DPCL-0903"]…}」
  - 两条在旧代码上即绿：并发首拍（本就该「照过」）；「旧拼法串本身合新形状」（只守新路径，靠变异核验证它咬得住）。
- 绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa09-green.log`。

### 门（作者自验，钉 `465ffe67`）
- 改动的 `.go` 文件 `gofmt -l` 无输出；`go build ./...`、`go vet ./...` 退 0。
- 带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...`，全 ok。日志：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa09-gate.log`。
- 并发首拍 `-count=5` 五次皆绿。日志：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa09-concurrent5.log`。
- 未跑全量 `./...`，留给推送方。

### 证据层级
S（隔离合成：`SYN-` 租户与对象、三缝替身；未接任何租户真源）。

### 越出地盘的改动
- `internal/transportfulfillment/adapters/http/open_dispatch_task.go` 及其测试。第 4 条落在手工口的载荷翻译里，该文件不在本票地盘清单内，票 tf-face-algebra/10 已进 main。
- `docs/adr/0114-…md` 文末复核记录（派单许可）。
- `docs/product/MECHANISM-INVENTORY.md` 重生成。只多了一个测试文件的计数，生产文件不变。

### 未做
- 「票 15 补记下追加回写行」：按派单留给推送方合并时补。
- 「走 Go 那条路」：作者自验已做；非作者评审（两轴分开）与推送方在干净检出上带 DSN 跑全量待做。
- 「不做（观察）」两条照旧不做。
- 顺带看到未动：`DeliveryDispatchUndecidedReason` 头注「今天三条缝一条都没接」已过期（三缝已接真）。它不在第 5 条点名范围内，未改，留评审定。

