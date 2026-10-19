# 36 主动拒绝撞上既有决定仍答「已形成」201、不回显；主动拒绝与处置「拒绝」去向的重放白耗决定标识

Category: bug
Status: ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立（票 15 之三、之四补核发现；用户授权代决）
Blocked by: operator-channel/33 进 main（真库重放要读回已拒绝委托）；operator-channel/34 进 main（同改 apps/admin-web/src/pages/shipment-request/api.ts 与 cmd/parcel-api/assemble_review_test.go）
父票：[15](./15-operation-decision-faces-take-operator-intake.md)
出处：票 26 遗留研判中补核票 15 之三、之四（簇 C6）发现；补记随 operator-channel/39。
地盘：
- internal/parcelshipment/application/reject_shipment_request.go 及其测试
- internal/parcelshipment/application/dispose_shipment_request.go 及其测试
- internal/parcelshipment/domain/active_rejection.go、authorized_disposition.go（需要只读判定时）
- internal/parcelshipment/adapters/http/reject_shipment_request.go
- internal/parcelshipment/adapters/http/dispose_shipment_request.go
- apps/admin-web/src/pages/shipment-request/api.ts
- cmd/parcel-api/assemble_review_test.go
- cmd/parcel-api/assemble_disposition_test.go
- 票 15 补记下的回写行（只加行）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

## 现象

- **答复误导。** `RejectShipmentRequestHandler.Handle` 撞上已经成立的决定时（`RejectByAuthority` 答 `ErrDecisionAlreadyFormed`），交回的是 `ActiveRejectionFormed`。`writeActiveRejectionOutcome` 对 `FORMED` 一律答 201；`activeRejectionResponse` 只带 `outcome`、`requestState`、`decisionKind`，不回显在册的决定方和原因。换了人、换了原因的后到者读到「已形成」，会把自己没被采纳的拒绝当成成功了；输给自动接受的那一次，也答 201。
- **重放白白耗掉决定标识。** 拒绝口在 `RejectByAuthority` 判「已成立」之前就调用了 `NextAcceptanceDecisionID`；处置口的「拒绝」支同样先签标识，再进 `DisposeUnderAuthority`。而 `RejectShipmentRequestHandler.Handle` 的头注自己写着「标识是本上下文签发的稀缺身份」。

## 复现（S；`$DECIDE` = `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide`）

运行 `go test -count=1 -overlay $DECIDE/C6/overlay_app.json ./internal/parcelshipment/application/ -run TestC6Probe -v`（复跑日志：`$DECIDE/C6-ticket15-part3-algebra/rerun_app.log`）：
- `TestC6ProbeActiveRejectionReplayAlgebra`：首次、原样重放、换人换原因三次都答 `FORMED`，签发标识次数依次为 1、2、3，交回的始终是首份的决定方和原因。
- `TestC6ProbeAuthorizedDispositionReplayAlgebra`：「拒绝」之后原样重放，答 `TASK_ALREADY_CLOSED`，但签发次数从 1 变成 2；「交客户补充」之后改选「拒绝」，答 `ALREADY_DISPOSED`，也签了一个标识。

HTTP 201 是读代码得出的（`writeActiveRejectionOutcome`）；在真库上，这一点被 operator-channel/33 的「读不回」挡住了，暂时看不到。

## 应当

- ADR-0022：「2xx 内部只区分有没有新建对象：新建了用 `201`，其余已形成的答案用 `200`」。
- ADR-0132 决定二：「撞上已成立的决定答`任务已完结`并交回那一个——与 `CompleteManualReview` 的分格同形」；PS CONTEXT：「另一方只能读取既有结果」。
- ADR-0183 复核记录二：「不复用`已形成`——那会让调用方把一次重投读成又形成了一条」；「认在签发标识之前，重放不消耗标识」；「没有授权的操作者不能拿重放探册」。

## 裁定（机制 + 产品策略；代 PS owner）

- 主动拒绝口撞上已越过决定边界的委托时，答新格`任务已完结`（`TASK_ALREADY_CLOSED`，HTTP 200），交回在册的决定并回显：决定种类；决定带有主动拒绝留痕时，再回显实际决定方和结构化原因。这一格与兄弟两口同名同义（ADR-0132 决定二）。`FORMED`（201）只留给本命令确实形成了拒绝的那一次。
- 不分出 `EXISTING_RESULT`。理由：拒绝是一槽一个决定，异内容不可能是新决定；「同内容重放」和「输给了别人」都靠回显让调用方自己比较。这和复核完成 `ALREADY_COMPLETED`、处置 `ALREADY_DISPOSED` 不区分重放的做法一致。PS 里 `EXISTING_RESULT` 的含义是「同一次请求的同内容重放」——提交口遇到异内容答 `INGRESS_CONFLICT`，ADR-0183 复核记录二也限定为「同类同内容」。把它扩到「输给了别人」就改了它的含义，管理台 `presentation.ts` 对它的说明「本次输入被识别为已有逻辑提交的重复或重试」也会随之变成假话。如果 PS owner 想区分，三口一起区分，比较哪几格照 ADR-0183 复核记录二写明。
- 判定的位置：在授权之后、签发决定标识之前。判据由领域提供（一个只读判定，复用转移里同一组前置条件），编排不另写一份状态判断。`RejectByAuthority` 里原有的 `ErrDecisionAlreadyFormed` 分支保留，作为防御映射到同一格。处置口「拒绝」支同理：`DisposeUnderAuthority` 的各项前置（决定已成立、任务已收工、本版已处置、没停在等处置）先于签发判出，答复格不变。保存时撞上版本冲突，照旧走 `saveStallReason` 答未决。
- 授权仍然排在最前。生产装配上拒绝授权映射为 nil 时，对已决的委托照旧答 `UNDECIDED` / `REJECTION_AUTHORITY_UNAVAILABLE`（票 15 完成判据要求「生产装配如实停在该口今天的格」）。
- 处置口的`任务已完结`格，同样回显决定上的主动拒绝留痕（处置「拒绝」形成的决定带有处置人和原因）。

## 做什么

1. application：`ActiveRejectionOutcome` 加`任务已完结`一格；`RejectShipmentRequestHandler.Handle` 和 `DisposeShipmentRequestHandler.Handle` 照裁定调整判定与签发的顺序；结果交回在册的决定（含主动拒绝留痕）。
2. 领域（如有需要）：提供只读判定「此刻还能不能形成拒绝 / 处置」，复用转移里的同一组前置条件，不另写一份。
3. HTTP：`writeActiveRejectionOutcome` 只对 `FORMED` 答 201；`activeRejectionResponse` 和处置响应的`任务已完结`格加上回显。
4. 管理台：`api.ts` 的 `ActiveRejectionResponseBody`、`AuthorizedDispositionResponseBody` 照新的回显字段改；页面照旧原样打印 outcome，呈现方式不改。

## 不做

- 授权的顺序；授权缺口下的未决格及其处理尝试（这是票 15 完成判据定下的停点）。
- 分出 `EXISTING_RESULT`（理由见裁定）。
- 主动拒绝要不要绑定操作员看过的那一版（交 PS owner）。

## 是否潜伏

潜伏。生产装配上拒绝授权映射为 nil，处置授权器未配置，走不到领域判定这一步。必须在 psb/07 接上拒绝映射、PC 词表补上「授权处置」之前进 main。

## 完成判据

- [ ] 编排（替身仓储）：在已拒绝、已接受的委托上主动拒绝，答`任务已完结`，不签标识、不保存、不发释放、不记处理尝试；首次拒绝仍答 `FORMED`。处置「拒绝」原样重放、「交客户补充」之后改选「拒绝」，都不签标识。
- [ ] 真库装配（`cmd/parcel-api`，合成授权，经生产的操作者 Intake）：
  - 同一拒绝原样重放、换人换原因，各答 200 `TASK_ALREADY_CLOSED`，回显先到那一份的决定方和原因，`acceptance_processing_attempt` 不增加；
  - 自动接受之后的拒绝，答 200 `TASK_ALREADY_CLOSED` + `ACCEPTED`；
  - 复核完成撞上已拒绝的委托，答 `TASK_ALREADY_CLOSED` 并交回决定；
  - 生产装配（映射为 nil）下，对已拒绝委托仍答 `UNDECIDED` / `REJECTION_AUTHORITY_UNAVAILABLE`，单独用一例钉住。
- [ ] 处置「拒绝」去向真库重放：答 `TASK_ALREADY_CLOSED`，并回显决定方和原因。
- [ ] 管理台三道门退出码为 0。
- [ ] 在票 15 补记下追加回写，只加行。
- [ ] 走 Go 那条路（并行会话）：开分支、作者自验、非作者评审、推送方带 DSN 跑全量。
