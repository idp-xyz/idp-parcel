# 20 TF 关段、建派送任务、承运商首次有效收寄判断三口的规范化形状与「同身份异内容」格

Category: enhancement
Status: resolved——2026-10-16 进 main：`e558c66e` / `857453d8` / `2d84eb2c` 三笔快进进 main 并推，远端 main = `2d84eb2c`，SHA 未改写；见「进 main 记录」。此前 in-progress——2026-10-16 完工，代码 `857453d8`（分支 `claude-oc20`），待进 main；评审只有作者自审，不算非作者评审，见「完成记录」。此前 in-progress——2026-10-16 Claude Code 会话认领（用户令「直接认领票 20 开始实施」），隔离 worktree `/home/tops/workspace/idp-parcel-claude-oc20`、分支 `claude-oc20`。此前 ready-for-agent——2026-10-16 TF owner（用户）逐口裁毕：关段裁现有格够用、答复补回显在册关闭时刻；建派送任务裁现有格够用（只核对）；承运商首次有效收寄判断裁补 TFC-1 形状、同依据异内容新增 `CONTENT_CONFLICT`。裁定与理由见「裁定记录」，实施见「做什么」。同日先提交过一版三口全裁「现有格够用」并转 resolved 的票面（`11d42fbe`），按本裁定改正，见 Comments。此前 needs-triage——2026-10-13 通道 4 立（通道 1 派单 `task-40698906`，照裁定 A：[15](./15-operation-decision-faces-take-operator-intake.md) 余下四口不换）。三口的重放分支只认身份、不比内容；补形状改的是 TF 领域代数，归 TF owner 逐口裁。现有代数、拟议形状（TFC-1）与要裁的那一问见「逐口」。
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

## 完成记录（2026-10-16，Claude Code 会话，分支 `claude-oc20`）

**基线**：分支基 `e558c66e`（本地 main，改裁与认领那一笔，未推）。代码一笔 `857453d8`；本记录一笔在其后。

**落点**（`857453d8`）

- `domain/payload_canonicalization.go`：`CanonicalizeCarrierFirstEffectivePickupPayload`，face `JUDGE_CARRIER_FIRST_EFFECTIVE_PICKUP`。表达控制时为读法 + `subject_kind` / `subject_reference` + `occurred_at`；不表达时只有读法。
- `application/judge_carrier_first_effective_pickup.go`：新格 `CarrierPickupContentConflict`（`CONTENT_CONFLICT`）；`Judge` 的同依据、非待确认链尾分支改走 `replayOrConflictOnTheSameBasis`，两侧从本体现算再比，异串答冲突、带回链尾、不动链、不发意图。待确认链尾仍走 `reconsiderPending`。
- `application/close_fulfillment_segment.go`：`CloseFulfillmentSegmentResult.ClosedAt()` 只在`早已关闭`时给出；领域判已关那路取读回段的时刻，并发下窄口答已关那路读回赢家，读回不通答未决。
- `adapters/http`：关段响应加 `closedAt`（RFC 3339 UTC，只随`早已关闭`）；承运商判断那一口的映射是通用的（枚举原名、带链尾、非新落版本 200），只补 ADR-0022 那段注释。

**判断项**：命令以名称素材指名、链尾经「待确认 → 身份登记后形成」到了已形成——取「主体轴不参与」，命令一侧取链尾在册主体。理由：已形成版本上没有素材，素材对应哪个在册身份归 party-commercial，本上下文现算不出；票面给的另一条「按本次解析出的主体比」在这一格上无物可比——`resolveSubject` 只解析引用，只给素材时交回的是空主体，照比就是把原命令重放判成冲突。变异取证：把这一格改成照比（命令一侧取空主体），`TestTheOriginalNameMaterialCommandStillReplaysAfterThePendingChainForms` 转红，恢复后绿。

**实施中量到的，票面没写到**

- **业务时间按微秒取，不照 `canonicalInstant` 原样取纳秒**（偏离「做什么」第 2 条的字面，理由是它的前提不成立）。对真库实测（临时探针，未提交）：收寄登记册存 `…09:00:00.123456789Z`，读回 `…09:00:00.123456Z`。这一口两侧都从本体现算，按纳秒比，同一条带纳秒时刻的命令原样重放会被判成冲突——来源不是轨迹事实时业务时间由命令给，HTTP 层 `time.RFC3339` 解析接受小数秒。别的口存的是落库前算好的摘要，不受影响，`canonicalInstant` 本身因此不改。`TestCarrierPickupJudgmentBusinessTimeIsComparedToTheMicrosecond` 钉住。
- 同一依据重放时命令带了主体引用而分支词不在封闭集内，现在答`未受理`，此前答`已在册`；与首判那条路（`resolveSubject`）同答。HTTP 层分支词不合法本就先答 400，这一格只在编排直调时出现。
- 有效时间改判会签新的事实版本（`JudgeEffectiveTimeHandler.Judge`），所以同一依据下轨迹事实来源的业务时间按构造相同，原样重放不会因有效时间改判而被判冲突。

**完成判据**（钉 `857453d8`；Linux（WSL2），go1.26.8，真库为 55432 门禁库）

- ✅ 承运商判断 · TFC-1 往返：`TestCommandPayloadsRoundTripAsTFC1` 两格（表达 / 不表达），期望摘要由字面量文档经 `sha256sum` 另算，不取代码输出。
- ✅ 承运商判断 · 同身份同内容答重放、同身份异内容答 `CONTENT_CONFLICT`：`TestASameBasisWithDifferentContentAnswersContentConflict`——同内容、异读法、异承运主体、异业务时间（扫描来源由命令给时刻）、失效链尾上原样重放与改读法，五格；判断项 `TestTheOriginalNameMaterialCommandStillReplaysAfterThePendingChainForms`；传输层 `TestAContentConflictAnswers200WithTheRecordedTail`（一写即绿：映射通用，不是先红后绿）。
- ✅ 关段 · `SEGMENT_ALREADY_CLOSED` 两条来路各一例带回在册时刻：`TestClosingAnAlreadyClosedSegmentIsItsOwnAnswer`（领域判已关）、`TestAConcurrentlyClosedSegmentEchoesTheWinnersClosingTime`（并发输家读回赢家；读回不通答未决、不带时刻）；传输层 `TestAnAlreadyClosedSegmentEchoesItsRecordedClosingTime`（重投给 `+08:00` 的另一时刻，回显在册的 `13:00:00Z`；首次关段不带 `closedAt`）。
- ✅ 建派送任务：裁为现有格够用，裁定与理由在「裁定记录」，代码未动。
- ✅ 三口结论回写 15 进展记录：随改裁那一笔 `e558c66e` 已写。
- 除传输层承运商那一例外，各新用例都先红后绿，红落在行为上（先加枚举格 / 访问器、不填值），不是只停在编译错。

**门**（作者自验）

- 全仓 `go build ./...`、`go vet ./...` 退 0；动过的 `.go` `gofmt -l` 无输出（一个测试文件的结构体标签对齐经 `gofmt -w` 修过）；改动文件无 CR、无 BOM。
- 动过三包（TF `domain`、`application`、`adapters/http`）的反向依赖经 `go list` 反查，连同 `./internal/architecture/...`，带 DSN `go test -p 1 -count=1` 全 ok；`-v` 下 TF `adapters/postgres` 与 `cmd/parcel-api` 为 `--- SKIP` 0、`--- FAIL` 0。分支树隔离且提交后干净，这一跑验的就是 `857453d8` 的提交状态。未跑全仓全量——按 parallel-sessions「全量只跑一次」，那一跑归推送方。
- `tools/mechanism-inventory` 在 `857453d8` 上重生成，与 `docs/product/MECHANISM-INVENTORY.md` 逐字相同，不需清点笔。

**评审**：作者自审，两轴串行（本仓不用子代理）：Standards 对 AGENTS 注释与引用规矩、TF 分层、ADR-0014 / ADR-0022；Spec 对本票「裁定记录」「做什么」「完成判据」。自审拿住一条并已修：上面那条微秒精度。**不算非作者评审**——进 main 前按 parallel-sessions「合入前独立评审」，有空闲通道就派非作者评，没有则由推送方如实记录。

**证据层级**：全部为合成夹具与门禁库上的 `S`。

**未做**：「不做」节原样未做；`cmd/parcel-api` 的承运商判断口换操作者 Intake 归 15，本票实施完即解除 15 对它的形状前置。

### 进 main 记录（2026-10-16，Claude Code 会话自落，用户令「需要」）

- 分支 `claude-oc20` 三笔 `e558c66e` / `857453d8` / `2d84eb2c` 直接叠在远端 main `11d42fbe` 之上，**快进**进 main，不重放、SHA 未改写；推送前紧贴 push 再 fetch，远端仍为 `11d42fbe`，`origin/main..2d84eb2c` 只有这三笔、底下无他人提交。`git push origin 2d84eb2c:main` 退 0，`ls-remote` 为 `2d84eb2c`。
- 推送方全量一次：钉 `2d84eb2c` 的全新 detached 检出，带 DSN，`go test -p 1 -count=1 -v ./...` 退 0——有测试的包 141 个 ok、0 FAIL；`--- FAIL` 0、`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，助手进程用例，不作为助手启动时照设计跳过）。同一检出上全仓 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0。
- 评审：只有作者自审（见完成记录「评审」），**没有非作者评审**——落地时本机无其他会话可派（`/list-agents` 实测），用户令照落。parallel-sessions「合入前独立评审」这一格本票未满足，如实记下；补评可钉 `e558c66e..857453d8` 另做。

## Comments

**2026-10-16 · 票面改正。** 同日先有一版票面把三口都裁成「现有格够用（只核对）」并转 resolved（`11d42fbe`），用户随后逐口改裁为上文「裁定记录」。被替下的那版有两处与代码不符，记下以免再写回：
- 关段那条说 `ClosedAt`「是记录事实」：它是受领域校验的业务时间，见 `ActualFulfillmentSegment.CloseSegment`。
- 承运商判断那条说同依据异读法「是来源更正，走 `CorrectsSourceVersion` 更正路」：`Judge` 里同依据在非待确认链尾上先答 `ALREADY_RECORDED`，更正那条路只在依据不同（新来源版本回指链尾那一代）时才走得到。

- **评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `857453d8`（基线 `e558c66e`）· 2026-10-16 10:0x–10:2x UTC**（票 [26](./26-non-author-review-backlog-for-claude-code-landings.md) 序 3 补评。非作者：代码出自会话 `idp-parcel-c8`。一个子代理在隔离只读 detached 检出上按 `/code-review` 两轴串行，先写完 Spec 再做 Standards；由会话 `idp-parcel-9b` 代落。）评审侧验证（无 DSN）：改动的 TF `domain`、`application`、`adapters/http` 三包 `go vet`、`go test -count=1`、`gofmt -l` 全过。
  - **Spec**：**阻断 1**：`internal/transportfulfillment/application/judge_carrier_first_effective_pickup.go` · `replayOrConflictOnTheSameBasis` 不论读法，命令只要带了引用就先调 `NewCarrierSubject`。一条失效命令（`expressesControl:false`）带了 `carrierReference`、没带 `carrierKind` 时，首判经 `rederive` 走 `Void`、不看主体，答 201 `PICKUP_VOIDED`；原样重放却答 200 `INPUT_NOT_ACCEPTED`。这违反本票裁定「同串答 `ALREADY_RECORDED` 带回链尾（不变）」与「读法为不表达时只有读法……命令里给了也不构成内容」。完成记录说「这一格只在编排直调时出现」，前提不成立：`CarrierPickupJudgmentPayload.Command` 只在 `carrierKind` 非空时解析种类，引用照写进命令，然后放行到编排。评审会话复核：在 main `4dd937eb` 的隔离检出上用包内夹具 `newCarrierPickupHTTPFixture` 实跑——先 POST `scanPickupBody`，再把载荷改成 `evidenceVersion:"v2"`、加 `correctsSourceVersion:"v1"`、`expressesControl:false`、删去 `carrierKind`，连续 POST 两次：第一次 201 `PICKUP_VOIDED`，第二次 200 `INPUT_NOT_ACCEPTED`，复现。线上可达：生产入口 `OperatorDecisionIntake.IntakeCarrierPickupJudgment` 同样调 `payload.Command(tenant)`，`60244fa6` 起挂在操作者真渠道上。→ 修复票 [27](./27-carrier-pickup-voiding-replay-without-kind-is-not-accepted.md)。
    **非阻断 2**：
    (1) 同一函数里，免比主体轴的条件写成了「没给引用」，而本票判断项只限于「以名称素材指名」。后果：表达控制、但引用与素材都不给的命令，首判答 `INPUT_NOT_ACCEPTED`，落在同依据已形成的链尾上却答 `ALREADY_RECORDED`（探针实测），正是裁定要避免的「把未被采纳的判断读成成功」。不是本笔引入的回归；已记进票 27「修复方向」。
    (2) 关段 `closedAt` 与链尾 `occurredAt` 都按 `time.RFC3339` 回显，只到秒；入口却接受小数秒，收寄比对又精确到微秒，调用方因此比不出亚秒级的差异。回显格式是票面定的，记作票面内部的张力。
  - **Standards**：**阻断 无**。**非阻断 4**：
    (1) `domain/payload_canonicalization.go` · `CanonicalizeCarrierFirstEffectivePickupPayload` 头注「进段三格同 CanonicalizeHandoverPayload 不进摘要」数的是 application 包里命令结构体的字段，违反「改文档」「一律不用行号，也不用计数……Go 注释里的跨文件引用同受此约束」，应点名 `Segment` / `PlannedSegment` / `SegmentServiceAction`。`replayOrConflictOnTheSameBasis` 头注引「凭证登记口的 CCC-1 先例」，按「优先用符号名」应改成 `CanonicalizeCredentialRegistrationPayload`。
    (2) 单一权威：「在册时刻只到微秒」是持久化事实，仓里的先例放在 PG 适配器（`accessidentity` 的 `sameInstant`），本笔却写进 TF 领域形状，而且只管这一口。同上下文里 `RegisterExternalCarrierCredentialHandler.replayAgainstCurrent`（`changedAt.Equal(command.At.UTC())`）与 `ExternalCarrierCredential.Equal` 仍按纳秒比，有同类重放被误判为冲突的风险（既有问题），宜另立票给 TF 定一处精度规则。
    (3) 判断题（Duplicated Code）：`replayOrConflictOnTheSameBasis` 手抄了 `resolveSubject` 的前半段（空引用判断 + `NewCarrierSubject` 失败答未受理），两处口径会各自漂移。评审会话注：Spec 那条阻断就出在这段手抄里。
    (4) `assertConflictLeavesTheChain` 用「行数 == 意图数」断言没动链：冲突路若误落一版并发出意图，两边同时加一，测试照样绿。应断言绝对行数，同一用例的重放分支就是这么写的。
  - **无发现**：截到微秒的截法正确（pgx v5.10.0 二进制编码向下取整，与 `Truncate(time.Microsecond)` 一致，两条往返摘要另算相符）；`CONTENT_CONFLICT` 映射 200 带链尾符合 ADR-0022；关段两条来路都带回在册关闭时刻，读回失败答未决，与 `OpenDispatchTaskHandler.Open` 同形。
  - **结论**：Spec 1 阻断，按票 26 立修复票 27，不回滚已进 main 的提交；27 已由本票的代码作者会话认领（分支 `claude-oc27`）。其余各条在 main `4dd937eb` 上都未修；Standards (2) 的精度规则未另立票，交 TF owner 定。
