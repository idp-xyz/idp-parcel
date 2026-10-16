# 20 TF 关段、建派送任务、承运商首次有效收寄判断三口的规范化形状与「同身份异内容」格

Category: enhancement
Status: in-progress——2026-10-16 Claude Code 会话认领（用户令「直接认领票 20 开始实施」），隔离 worktree `/home/tops/workspace/idp-parcel-claude-oc20`、分支 `claude-oc20`。此前 ready-for-agent——2026-10-16 TF owner（用户）逐口裁毕：关段裁现有格够用、答复补回显在册关闭时刻；建派送任务裁现有格够用（只核对）；承运商首次有效收寄判断裁补 TFC-1 形状、同依据异内容新增 `CONTENT_CONFLICT`。裁定与理由见「裁定记录」，实施见「做什么」。同日先提交过一版三口全裁「现有格够用」并转 resolved 的票面（`11d42fbe`），按本裁定改正，见 Comments。此前 needs-triage——2026-10-13 通道 4 立（通道 1 派单 `task-40698906`，照裁定 A：[15](./15-operation-decision-faces-take-operator-intake.md) 余下四口不换）。三口的重放分支只认身份、不比内容；补形状改的是 TF 领域代数，归 TF owner 逐口裁。现有代数、拟议形状（TFC-1）与要裁的那一问见「逐口」。
Blocked by: 无
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
- 关段：`SEGMENT_ALREADY_CLOSED` 的两条来路（领域判已关、并发下窄口答已关）各一例，答复带回在册关闭时刻。
- 三口的结论回写 15 进展记录里逐口核载荷规范化那一格。

## 裁定记录（2026-10-16，TF owner = 用户）

三口各自裁，不同列。三类归属：三格都是机制 + 产品策略，没有租户取值，不留 `未配置`。

- **关段 · 现有格够用，答复补回显在册关闭时刻。** 不补摘要形状、不新增冲突格。`ClosedAt` 不只是记录事实：它是调用方给的业务时间，领域还拿它校验（关闭时刻不得早于任何已结束参与的终点，`closesBeforeAParticipationEnded`）。但段是单向的，已关之后第二个时刻没有落点，冲突格给不出可续办的去处，所以格不加。代价在于今天的 `segmentClosureResponse` 只带 `outcome` / `segment` / `continuationReference`，异时刻重投的调用方看不到原时刻——补上回显，让它和建派送任务口一样「差异调用方自己比得出」。
- **建派送任务 · 现有格够用（只核对）。** `DISPATCH_TASK_ALREADY_OPEN` 已交回在册任务，差异调用方自己比得出；改工作范围不经此口表达——`DispatchTaskRegistry` 只有首登、没有演进写口，冲突格没有可指的更正口。不补形状，`payload_canonicalization.go` 与答复集都不动。
- **承运商首次有效收寄判断 · 补 TFC-1 形状，同依据异内容答 `CONTENT_CONFLICT`。**
  - 同一依据（证据引用 + 来源版本）走不到更正路：更正要带**新的**来源版本、由 `CorrectsSourceVersion` 指回前代，依据随之变了，`rederive` 只接这一种；同一依据异读法、异承运主体或异业务时间在今天的代数里只会落进 `ALREADY_RECORDED`。读法与主体是这一口判出来的语义内容（形成 / 失效、谁承运），把另一份判断收成「已记录」会让调用方把未被采纳的判断读成成功。`ALREADY_RECORDED` 虽带回链尾那一版，但这里的差异不是调用方该自己比的工作范围细节，而是判断本身。
  - 码取 `CONTENT_CONFLICT` 不取 `SOURCE_CONFLICT`：TF 里 `SOURCE_CONFLICT` 用在同一来源事实带来不同内容的登记口（有效交付、交接、异地揽收）；这里依据相同、来源事实相同，冲突在判断方给的内容上，与有效时间规则、主单、承运商凭证这些登记口的 `CONTENT_CONFLICT` 同义。

## 做什么

照 13 第 4 条（CCC-1）的先例：两侧都从在册那一份本体现算再比，不加摘要列、不动迁移，没有旧摘要要认。

1. **关段回显**（`application/close_fulfillment_segment.go`、`adapters/http/close_fulfillment_segment.go`）：`CloseFulfillmentSegmentResult` 在 `SEGMENT_ALREADY_CLOSED` 时带在册关闭时刻，`segmentClosureResponse` 加一格透出（RFC 3339 UTC，与本包其余时间格同形）。领域判已关那路从读回段的 `ClosedAt()` 取；并发下窄口答 `ports.SegmentAlreadyClosed` 那路照 `OpenDispatchTaskHandler.Open` 并发输家的先例读回赢家再答，读回失败按该处读失败的既有答复作答。其余各格不带。
2. **承运商判断的形状**（`domain/payload_canonicalization.go`）：加 `CanonicalizeCarrierFirstEffectivePickupPayload`，`face` 为 `JUDGE_CARRIER_FIRST_EFFECTIVE_PICKUP`。
   - 进摘要：读法 `expresses_control`；读法为表达控制时再进承运主体两键 `subject_kind` / `subject_reference` 与业务时间 `occurred_at`（照 `canonicalInstant`）。读法为不表达时只有读法——失效版本上没有主体与时间，命令里给了也不构成内容。
   - 不进摘要：租户、对象、依据（身份）；进段三格（同 `CanonicalizeHandoverPayload`「范围、段与对象身份不进摘要」）；**名称素材**——`Material()` 只在待确认版本上给出，比对只发生在非待确认链尾上，已形成版本上没有它可算（本票「逐口」的拟议形状曾列 `name_material`，说它在版本上，不对，按此去掉）。
   - 来源是外部承运轨迹事实时 `occurred_at` 取读回那一代的有效时间，不取命令的。
3. **承运商判断的重放分支**（`application/judge_carrier_first_effective_pickup.go` 的 `Judge`）：同依据、非待确认链尾时两侧现算——链尾一侧读法由结果反推（`CarrierPickupFormed` ↔ 表达，`CarrierPickupVoided` ↔ 不表达）、主体取 `Carrier()`、时间取 `OccurredAt()`；同串答 `ALREADY_RECORDED` 带回链尾（不变），异串答新格 `CONTENT_CONFLICT`、不动链。链尾待确认时仍走 `reconsiderPending`，不进比对。HTTP 答复映射与 `carrierPickupJudgmentOutcome` 补这一格。
   - **判断项（实施时定，须守的约束）**：命令以名称素材而非主体引用指名承运主体、链尾经「待确认 → 身份登记后形成」到了已形成——同一条命令原样重放必须仍答 `ALREADY_RECORDED`。命令一侧此时给不出主体两键，主体轴怎样比（按本次解析出的主体比，或该轴不参与）由实施方择一并在完成记录写明理由。

完成判据见「完成判据（按裁定）」。不做的照「不做」。

## Comments

**2026-10-16 · 票面改正。** 同日先有一版票面把三口都裁成「现有格够用（只核对）」并转 resolved（`11d42fbe`），用户随后逐口改裁为上文「裁定记录」。被替下的那版有两处与代码不符，记下以免再写回：
- 关段那条说 `ClosedAt`「是记录事实」：它是受领域校验的业务时间，见 `ActualFulfillmentSegment.CloseSegment`。
- 承运商判断那条说同依据异读法「是来源更正，走 `CorrectsSourceVersion` 更正路」：`Judge` 里同依据在非待确认链尾上先答 `ALREADY_RECORDED`，更正那条路只在依据不同（新来源版本回指链尾那一代）时才走得到。
