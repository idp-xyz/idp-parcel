# 20 TF 关段、建派送任务、承运商首次有效收寄判断三口的规范化形状与「同身份异内容」格

Category: enhancement
Status: needs-triage——2026-10-13 通道 4 立（通道 1 派单 `task-40698906`，照裁定 A：[15](./15-operation-decision-faces-take-operator-intake.md) 余下四口不换）。三口的重放分支只认身份、不比内容；补形状改的是 TF 领域代数，归 TF owner 逐口裁。现有代数、拟议形状（TFC-1）与要裁的那一问见「逐口」。
Blocked by: 无——等 TF owner 逐口裁
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施
地盘（裁完后）：`internal/transportfulfillment/domain/payload_canonicalization.go`（裁为补形状的口各一个 `Canonicalize…Payload`）；`internal/transportfulfillment/application` 里 `CloseFulfillmentSegmentHandler.Close`、`OpenDispatchTaskHandler.Open`、`JudgeCarrierFirstEffectivePickupHandler.Judge` 的重放分支；新增答复格时 `internal/transportfulfillment/adapters/http` 三口的答复映射。
出处：[ADR-0151](../../../docs/adr/0151-unassigned-command-faces-get-their-families.md) 决定三（逐口核载荷规范化，未核完的口不开真渠道）；[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定四第一条、越权风险点 5；[ADR-0014](../../../docs/adr/0014-versioned-canonicalization-shape-for-content-digest.md)；[13](./13-command-payload-canonicalization-per-face.md)（TFC-1；三口都在它「未定形状」清单的 TF 一行里）；15 做什么第 5 条。

## 为什么

15 余下四口换操作者 Intake 之前要逐口核载荷规范化：领域已有同身份重放与内容冲突代数的口只核对，没有的补一版带形状版本的规范化形状（ADR-0151 决定三）。四口里有效时间判断过了（见 15 进展记录）；这三口按身份认重放，同一身份带不同内容也答重放那一格。13 定 TFC-1 时没给它们定形，`payload_canonicalization.go` 头注写着「没有内容判据的口不在这里发明形状」。哪些字段算内容、异内容答哪一格，是 TF 领域代数的事。

三口今天都不存摘要。裁为补形状的口，可照 13 第 4 条（凭证登记口定 CCC-1）的先例：两侧都从在册那一份本体现算摘要再比，不加摘要列、不动迁移，也就没有旧摘要要认。

## 逐口（取证钉 `1e02d2ea`）

### 关段 `/transport-fulfillment-segment-closures`

- **现有代数**：`CloseFulfillmentSegmentHandler.Close` 按（租户、段）读回整段走领域；段已关（领域答 `ErrSegmentClosed`，或并发下窄口答 `ports.SegmentAlreadyClosed`）一律答 `SEGMENT_ALREADY_CLOSED`。命令的内容只有 `ClosedAt`，不比；结果不带在册那一份，调用方看不到原关闭时刻。
- **拟议形状（TFC-1）**：`{"canonicalization":"TFC-1","face":"CLOSE_FULFILLMENT_SEGMENT","closed_at":"…"}`，时刻照 `canonicalInstant`；租户与段身份不进摘要。比对从在册段的 `ClosedAt()` 现算。
- **要裁的一问**：同段异 `closedAt` 新增一格冲突，还是裁定 `SEGMENT_ALREADY_CLOSED` 够用（关段是单向的「不再接受新对象」，再关一次改变不了段的状态）。

### 建派送任务 `/transport-fulfillment-dispatch-task-registrations`

- **现有代数**：`OpenDispatchTaskHandler.Open` 按（租户、任务）找到即答 `DISPATCH_TASK_ALREADY_OPEN` 并回显原任务，并发输家读回赢家同答；种类、对象、地点、时间窗两端、服务条件与业务时间 `OpenedAt` 都不比。编排注释写「同一任务被重投：交回原任务，不顶替。改工作范围不是重投能表达的事」；`DispatchTaskRegistry` 只有首登，没有演进写口。
- **拟议形状（TFC-1）**：`{"canonicalization":"TFC-1","face":"OPEN_DISPATCH_TASK","kind":"…","objects":[…],"place":"…","window_from":"…","window_to":"…","conditions":"…","opened_at":"…"}`，对象照 TFC-1 的集合通例排序（`sortedCopy`）；租户与任务身份不进摘要。比对从在册的 `DispatchTask` 现算。
- **要裁的一问**：同任务异工作范围新增一格冲突，还是裁定 `DISPATCH_TASK_ALREADY_OPEN` 回显原任务够用（差异调用方自己比得出）。

### 承运商首次有效收寄判断 `/transport-fulfillment-carrier-first-effective-pickup-judgments`

- **现有代数**：`JudgeCarrierFirstEffectivePickupHandler.Judge` 读回链尾；链尾按同一依据（证据引用 + 来源版本，`BasedOn`）判出、且不是待确认时，答 `ALREADY_RECORDED` 带回链尾。读法 `ExpressesControl`、承运主体（`SubjectKind` / `SubjectReference` / `NameMaterial`）、业务时间 `OccurredAt`（来源不是外部承运轨迹事实时由命令给）与进段三格都不比。链尾待确认时同一依据再来走 `reconsiderPending`——那是「身份登记之后凭同一证据形成新版本」，不是重放。
- **拟议形状（TFC-1）**：`{"canonicalization":"TFC-1","face":"JUDGE_CARRIER_FIRST_EFFECTIVE_PICKUP","expresses_control":…,"subject_kind":"…","subject_reference":"…","name_material":"…","occurred_at":"…"}`；身份（租户、对象、依据）与进段三格不进摘要，同 `CanonicalizeHandoverPayload`「范围、段与对象身份不进摘要」；来源是外部承运轨迹事实时，`occurred_at` 取读回那一代的有效时间，不取命令的（命令头注：命令里给了也不用）。比对从链尾版本现算：读法由结果反推（`CarrierPickupFormed` ↔ 表达控制，`CarrierPickupVoided` ↔ 不表达），承运主体、业务时间与名称素材都在版本上。
- **要裁的一问**：同依据异读法、异承运主体或异业务时间新增一格冲突，还是裁定 `ALREADY_RECORDED` 带回链尾够用。

新增冲突格时码由 owner 定；TF 已有口的同身份异内容格答 `SOURCE_CONFLICT` 或 `CONTENT_CONFLICT`。

## 不做

- 有效时间判断（只核对，见 15 进展记录）；13「未定形状」清单上的其余各口。
- `cmd/parcel-api` 的换口与撤隔离放行（归 15）。
- TF 要不要记提交操作者作操作证据（ADR-0151 越权风险点 5，另归 TF owner）。

## 完成判据（按裁定）

- 裁为补形状的口：TFC-1 往返用例；同身份同内容答重放、同身份异内容答裁定的那一格，各一例。
- 裁为现有格够用的口：本票记下裁定与理由。
- 三口的结论回写 15 进展记录里逐口核载荷规范化那一格。
