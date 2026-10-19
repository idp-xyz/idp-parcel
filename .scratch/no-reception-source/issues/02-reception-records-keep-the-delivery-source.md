# 02 第一期：收寄记录保全本次交付的来源文档，三种接收观察都存；每版判断有版本、追加版指名前版，已有结果带回原判断种类与当前有效版本

Category: enhancement
Status: in-progress——2026-10-19 评审 0 阻断（阻断一条已修，会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领，分支 `claude-nrs02`（与 ps-external-mark-relations/05 并行，同碰收寄编排）。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 的作者子代理照票 01 拆出（代决）
Blocked by: 01 进 main（ADR-0199、NO CONTEXT 与 UC-NO-002 先落）。同改收寄编排与 PG 适配器的 operator-channel/45、no-reception-identity/03 已于 main `91ea0883` 进 main，以它们为底。
父票：[01](./01-node-reception-preserves-the-delivery-source.md)
出处：票 01 裁定分期之「一、来源保全」与「已有结果的答复带回原判断种类与当前有效版本」那一问；ADR-0199 决定一、决定二。
地盘：
- internal/nodeoperations/domain/payload_canonicalization.go（来源文档的形状）及其测试
- internal/nodeoperations/ports/ports.go（`ReceptionRecord` 加来源文档、判断版本、前版）
- internal/nodeoperations/application/receive_delivered_unit.go 及其测试
- internal/nodeoperations/adapters/postgres/reception.go 及其测试
- migrations/node_operations/（下一个空号；今天是 0007）
- internal/nodeoperations/adapters/http/receive_delivered_unit.go 及其测试（已有结果两格信息）
- cmd/parcel-frontline-import/intake_import.go（明细写原判断种类与当前有效版本）
- cmd/parcel-api 下新起一份真库用例文件
- docs/product/MECHANISM-INVENTORY.md（重生成）
- .scratch/at-coverage-inventory.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象（钉 main `91ea0883`，读码）

- `ports.ReceptionRecord`、`domain.NodeIntake` 与 PG 行模型都没有号码格；`Handle` 把 `domain.CanonicalizeDeliveryPayload` 交回的规范化文档写成 `_`，只留摘要。
- 拒收只存拒收原因；仅扫描只存键、摘要、种类与时刻。证据、交付方只在形成与待识别两格的 intake 里有。
- 已有结果的 HTTP 答复只有 `outcome` 与记录上本来就有的几格；仅扫描一支重投时只有 `outcome`。
- 未收寄与待确认两格没有版本；ADR-0196 追加的冲突版不指名它接续的是哪一版。

## 做什么

照 ADR-0199 决定一、决定二：

1. **来源文档**。domain 定收寄面来源文档的形状：自带形状版本；作业实物、外部标识观察（原样）、接收观察、证据、拒收原因、交付方、节点、发生时间（UTC）；缺席格记空串、不省键。编排三支（明确接收、明确拒收、仅扫描）都把它写进记录。指纹照旧按 NOC-1 收寄面那几格算，`CanonicalizeDeliveryPayload` 与 `unversionedDeliveryContentDigest` 一个字节都不改。
2. **ADR-0196 冲突版**带引起它的那次后到送站的来源文档（与它沿用的摘要同源）。
3. **判断版本与前版**。未收寄、待确认两支也签判断版本（同一签发器）；形成与待识别两格的判断版本就是收寄结果版本，不另签。ADR-0196 冲突版指名前版：先到那件被依的那一行所在链此刻的当前有效版本。
4. **存储**。迁移加来源文档列、判断版本列、前版列；库面以（租户，前版）唯一守住「一版至多一个后继」。既有行不回填来源文档（号码从未留存）；新行必带来源文档与判断版本的守法由实施定（例如按约束只拦新写入），但不得给既有行补造来源。迁移头注写明既有行为空的理由。
5. **已有结果的答复**带 `recordedOutcome`（原判断种类）与 `currentVersion`（沿前版链走到头的那一版），HTTP 答复与前线导入明细都带；`outcome` 仍是 `EXISTING_RESULT`，两格不进 outcome 词表。
6. **AT 点名**：`AT-NO-047`、`AT-NO-048`；`.scratch/at-coverage-inventory.md` 照它的扫描口径更正。

## 不做

- 重核（票 03）、同一作业实物的并入与仅扫描待确认的续办（票 04）、身份核对读不动的翻转与身份核对态（票 05）。
- 把证据、交付方、拒收原因并进指纹（ADR-0199 越权风险点）。
- 来源冲突那一次的内容落册（ADR-0199 能力边界）。
- 复核目录与管理台展示来源文档（票 03 为人工重核要用时再透出外部标识观察）。

## 完成判据

- [x] 编排用例：明确接收、明确拒收、仅扫描各一条，记录带来源文档，外部标识观察逐字等于入参（带两端空白的号在线口原样存）；同一来源重投答已有结果，首存的来源文档不被覆盖——同一来源只在证据上不同也如此（`AT-NO-047`）。
- [x] 编排用例：ADR-0196 一码两物之后，重投先到那件原判断的来源，答已有结果，`recordedOutcome` 是 `INTAKE_FORMED`、`currentVersion` 是追加的冲突版；冲突版的前版是原判断（`AT-NO-048`）。仅扫描一支重投时 `recordedOutcome` 是 `RECEPTION_UNDECIDED`。
- [x] 真库用例（cmd/parcel-api，经生产装配）：三种观察落册后读回来源文档逐字一致；来源文档为空的既有形状行照常重放，不答冲突、不报错。
- [x] 指纹不变：NOC-1 与无版本两版既有摘要的重放与冲突用例照旧绿。
- [x] 适配器用例：（租户，前版）重复写入被库面拦下。
- [x] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-frontline-import`、`internal/architecture/...`、`migrations/...` 全绿；机制清点在本笔的干净检出上重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-nrs02`，基于 main `423b517a`）

**提交**

| 提交 | 内容 |
|---|---|
| `2387d911` | 实施与用例：来源文档形状、记录三格、编排三支与冲突版、收寄库读链与写入、迁移 0007、HTTP 两格、前线导入明细 |
| `3b7ee610` | 机制清点在 `2387d911` 的干净检出上重生成：cmd 测试文件 157→158，node_operations 迁移 6→7 |
| `6ed9ec11` | 本票完成记录；AT 覆盖盘点补 `AT-NO-047` 至 `AT-NO-051` |
| `0a3a441d` | 评审阻断修补：观察号码带 NUL 在在线收寄口答 400 `MALFORMED_REQUEST`、在前线导入收寄模板按坏行整批拒（见「评审阻断修补」） |
| `c8cc32c5` | 机制清点在 `0a3a441d` 的干净检出上重生成：cmd 测试文件 158→159 |
| 本笔 | 完成记录补评审阻断修补 |

**落点**

- 领域（`payload_canonicalization.go`）：`DeliverySource`（零值即来源未保全，`Preserved()`）、`CanonicalizeDeliverySource`、`ParseDeliverySource`、`ErrInvalidDeliverySource`。`CanonicalizeDeliveryPayload` 与 `unversionedDeliveryContentDigest` 一字未改（该文件对 `423b517a` 的 diff 只有新增行）。
- 端口：`ReceptionRecord` 加 `JudgmentVersion`、`Predecessor`、`Source`；`ReceptionStore` 加 `CurrentVersion`；新哨兵 `ErrPredecessorAlreadySucceeded`。
- 编排：`ReceptionClaimKind.String`；拒收与仅扫描两支经新的 `commitWithoutIntake` 签判断版本、带来源文档；明确接收那一支只在记录字面量里加两格，身份核对那一段没动；`appendIdentityConflicts` 给冲突版填判断版本、前版与后到那次送站的来源文档；`existingResult` 改为交回 error，读当前有效版本；结果加 `RecordedOutcome()`、`CurrentVersion()`。
- PG 适配器：三列读写；`CurrentVersion` 用一条递归查询沿后继走；`Save` 的 `ON CONFLICT` 收窄到主键，撞（租户，前版）唯一译成 `ErrPredecessorAlreadySucceeded`。
- HTTP：`recordedOutcome`、`currentVersion` 两格。前线导入：重放明细。

**存储形状（票面交给实施定的几格）**

- **来源文档**：jsonb 列 `source_document`，形状版本 `NRS-1`，与 NOC-1 各自换版。键：`shape`、`unit`、`mark`、`claim`、`evidence`、`refusal`、`delivered_by`、`node`、`occurred_at`，不带 omitempty，缺席格写空串。`claim` 记两个入口共用的词 `RECEIVED` / `REFUSED` / `SCAN_ONLY`，指纹里照旧记封闭码。读回时形状版本认不出就报错，不按 NRS-1 硬读；适配器把它当读失败，编排因此不形成答案，口径与「已存摘要认不出」相同。
- **判断版本**：text 列 `judgment_version`。类型沿用 `domain.IntakeResultVersion`，不另立类型：同一签发器、同一命名空间，也不必另起签发口。形成与待识别两格就是收寄结果版本，库面用 `IS NOT DISTINCT FROM` 守两处同值。
- **前版**：text 列 `predecessor_version`。部分唯一索引 `reception_predecessor_unique`（`tenant_id, predecessor_version`，只覆盖带前版的行）。CHECK：带前版的行自己必有判断版本，前版不等于自己；两个版本都不为空白；来源文档是 JSON 对象。
- **当前有效版本**：`Receptions.CurrentVersion` 用一条递归 CTE，按（租户，前版）一步步往后继走，走到没有后继为止，封顶 `maxJudgmentChain` 步。到顶就报错，防的是绕过编排写出的环。
- **新行必带来源文档与判断版本：库面不拦，由编排守。** 要在库面拦，就得按写入先后分新旧（`NOT VALID` 的 CHECK）。但 node_operations 适配器、HTTP、cmd/parcel-dispatch、cmd/parcel-frontline-import、cmd/parcel-api 都有用例直接经 `Save` 或 SQL 落记录形状的夹具，收紧就要连带改这些用例。理由与迁移 0006 头注「INTAKE_FORMED 不带核对态这一格库里不拦」相同，写进了迁移 0007 头注。编排每一支都写这两格，由 `TestEveryObservationKeepsItsDeliverySource` 钉住。

**迁移号与回填**：`migrations/node_operations/0007_reception_delivery_source.sql`。

- 来源文档：既有行一律不回填，留空。理由写在头注：号码从未留存，摘要是哈希还原不出来，拒收行与待确认行连别的格也没存。
- 判断版本：既有的形成与待识别两格回填 `intake ->> 'version'`。按定义那就是判断版本，同一个值换一处写，不算推定。既有的未收寄与待确认两格从未签过版本，留空。
- 前版：一律留空，既有的 ADR-0196 冲突版也不回填。

迁移用例 `TestTheDeliverySourceMigrationLeavesExistingSourcesEmpty` 逐行钉住以上三条。

**已有结果新增的两格**（HTTP 信息格，只随 `EXISTING_RESULT` 出现，omitempty，不进 outcome 词表）

- `recordedOutcome`：原记录种类的原名，取值 `INTAKE_FORMED` / `PENDING_IDENTIFICATION` / `INTAKE_NOT_FORMED` / `RECEPTION_UNDECIDED`。
- `currentVersion`：沿前版链走到头那一版的判断版本串。来源保全之前落下的未收寄与待确认行没有判断版本，这一格缺席。

前线导入的重放明细由「原结果 X；…」改为「原判断 X，当前有效版本 V；…」，没有版本时写「没有判断版本（来源保全之前落下的行）」。

**与 ADR-0196 现行实现的相容性**（派活点名要先核）

- `appendIdentityConflicts` 追加的那一行今天有前版可指。被依的那一行 `earlier` 带关联：`FindOtherUnitIntakes` 只交回带关联的行；同一件先到实物收过多次时，取接收时刻最晚那一行。它是形成格，判断版本就是它的收寄结果版本。前版取那条链此刻的当前有效版本：第三件再来时，那条链已由上一版冲突版接续，前版就是上一版冲突版，不是原判断。这一点由 `TestAReplayAfterAnIdentityConflictAnswersTheCurrentVersion` 的第三件那段钉住。相容。
- 并发也相容。读链发生在 `FindOtherUnitIntakes` 取得（租户、包裹）咨询锁之后，与收寄同一笔事务；READ COMMITTED 下每条语句另取快照，另一笔并发送站先提交的冲突版这里读得到。（租户，前版）唯一只在锁被绕过时才会撞上。
- 写入代数为此改了一处。原来的 `ON CONFLICT DO NOTHING` 不带冲突目标，撞前版唯一也会吞成「已有记录」，再被 `appendIdentityConflicts` 当成 `ErrUnexpectedReceptionSave`。结果同样是不形成答案，但错因说错了，所以收窄到主键。
- 不相容的一格照 ADR-0199「第一期之前落下的冲突版没有前版，不回填」办，不改裁：本期之前追加的冲突版没有前版，链上看不见它。
  - 在这之后重投先到那件，`currentVersion` 答的是原判断自己，不是那版旧冲突版。
  - 若再来第三件，新冲突版从原判断接下去：册上原判断实际有两版接续，只有新的一版连在链上。
  - 方向依据的身份一格（`JudgeIdentityBasis` 读全部行）不受影响，照旧判身份冲突。

**完成判据与用例**

| 判据格 | 用例 |
|---|---|
| 三支记录带来源文档、号原样、证据不同的重投不覆盖（`AT-NO-047`） | `internal/nodeoperations/application`：`TestEveryObservationKeepsItsDeliverySource`；形状另有领域 `TestDeliverySourceRoundTripsAsNRS1`、`TestAnIncompleteOrUnknownDeliverySourceIsRefused` |
| 一码两物之后重投先到那件，`recordedOutcome` 是 INTAKE_FORMED、`currentVersion` 是冲突版，冲突版前版是原判断（`AT-NO-048`）；仅扫描重投 `recordedOutcome` 是 RECEPTION_UNDECIDED | `TestAReplayAfterAnIdentityConflictAnswersTheCurrentVersion`；`TestAReplayOfAnUndecidedOrRefusedJudgmentAnswersItsOwnVersion`（仅扫描与拒收）；签不出版本、读不出链两支不形成答案：`TestJudgmentVersionFailuresFormNoAnswer`；HTTP 面：`TestAnExistingResultCarriesTheRecordedOutcomeAndTheCurrentVersion` |
| 真库：三种观察读回来源文档逐字一致；来源为空的既有形状行照常重放 | `cmd/parcel-api/reception_delivery_source_test.go`：`TestTheWiredReceptionKeepsTheDeliverySourceOfEveryObservation`（另直读 `source_document ->> 'mark'`，两端空白原样）、`TestTheWiredReceptionReplaysRowsWrittenBeforeSourcesWerePreserved`；`AT-NO-048` 的真库与 HTTP 面：`TestTheWiredReplayAfterAnIdentityConflictAnswersTheCurrentVersion` |
| 指纹不变 | 领域 `TestDeliveryPayloadRoundTripsAsNOC1`、`TestStoredDigestIsComparedWithinItsOwnShape`；编排 `TestRecordsStoredBeforeNOC1AreComparedUnderTheirOwnShape`、`TestNewRecordsAreStoredUnderNOC1`、`TestAnUnknownStoredShapeIsNoAnswerNotAConflict`；冲突 `TestReplayConflictAndFailedIntentStayDisciplined`——均未改，照旧绿 |
| （租户，前版）重复写入被库面拦下 | `internal/nodeoperations/adapters/postgres`：`TestASecondSuccessorOfOneVersionIsRefusedByTheTable`；另 `TestTheDeliverySourceAndJudgmentVersionsAreReadBackUnchanged`、`TestTheCurrentVersionFollowsSuccessorsToTheEnd`、`TestTheReceptionTableRefusesMisshapenVersions`、`TestTheDeliverySourceMigrationLeavesExistingSourcesEmpty` |

**先红后绿**：红在 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nrs02-red.log`，分两轮。

- 第一轮只写了用例，生产代码一字未改，结果是编译红，例如 `undefined: domain.DeliverySource`、`record.Predecessor undefined`。
- 第二轮只补了编译骨架（类型、字段、签名，方法体是桩），结果是断言红，例如：
  - 「先到那件的冲突版没有指名原判断 "" 为前版：map[]」
  - 「recordedOutcome = <nil>, want "INTAKE_FORMED"」
  - 「同一前版的第二个后继 err = <nil>, want ErrPredecessorAlreadySucceeded」
  - 「重放明细 = "原结果 PENDING_IDENTIFICATION；…", want 含 "原判断 PENDING_IDENTIFICATION，当前有效版本 "」

绿在同目录 `nrs02-green.log`。

**门**（钉 `3b7ee610`；评审阻断修补之后在 `0a3a441d` 的工作树上照同一组再跑一遍，见「评审阻断修补」）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 均退 0；带 DSN 跑 `go test -p 1 -count=1 ./internal/nodeoperations/... ./cmd/parcel-api/... ./cmd/parcel-frontline-import/... ./migrations/... ./internal/architecture/...`，全部 ok。额外跑了门外的收寄记录消费方，`cmd/parcel-dispatch`、`internal/parcelshipment/adapters/nodeoperations`、`internal/visibilityexception/adapters/nodeoperations` 均 ok。没有跑全量 `./...`。

**证据层级**：S（隔离测试库，夹具全为合成 `SYN-` 值，不涉租户取值）。

**越出地盘的改动**

- `cmd/parcel-frontline-import/vertical_test.go` 的重放段加了一条明细断言，用来钉「前线导入明细带原判断种类与当前有效版本」。票面地盘只列了 `intake_import.go`。
- 评审阻断修补按评审指派动了两个入口：`internal/nodeoperations/adapters/http/reception_payload.go` 及 `work_fact_intake_test.go`，`cmd/parcel-frontline-import/intake_template.go` 及新文件 `intake_mark_nul_test.go`。

**AT 盘点**：票 01 新增的 `AT-NO-047` 至 `AT-NO-051` 当时没进 `.scratch/at-coverage-inventory.md`，本笔照扫描口径补上。047、048 入①；049、050、051 入②，分别待票 04、04、05。①290→292，②225→228，合计 1083→1088（NO 46→51）。

**未做**

- Go 那条路的后两格：非作者评审、推送方在干净检出上带 DSN 跑全量。
- 票面「不做」各项照旧：重核、并入与续办、翻转、证据等并进指纹、来源冲突内容落册、复核目录与管理台展示来源文档。
- 库面拦「新行必带」（见上「存储形状」）。

**发现**（都不改裁定）

- **外部标识观察带 NUL 字符时，jsonb 拒收**（PostgreSQL 22P05）。**已处理**，评审列为阻断，修法见下「评审阻断修补」。
- **来源保全之前的未收寄与待确认行没有判断版本**，不能被指名为前版。票 04 续办仅扫描待确认时要面对这些旧行：可以续办时再签，也可以不续办，由那张票定。迁移头注已点名。
- **与并行的 ps-external-mark-relations/05 同碰 `receive_delivered_unit.go`。** 本票在 `receive` 里只在记录字面量加了两行，身份核对那一段与身份读口一行没碰；其余改动都在 `Handle` 的拒收与仅扫描两支、`appendIdentityConflicts`、`commit` 与 `existingResult`。合并预期只在字面量附近有上下文重叠。

### 评审阻断修补（2026-10-19，同一作者子代理，分支 `claude-nrs02`）

**阻断**：外部标识观察带 NUL（`\x00`）时，拒收与仅扫描两支经本票变成 5xx，设备永远重投；前线导入那一行永远记未决。来源文档原样保全这个号，jsonb 存不下它（22P05）。这是本票带进来的回归。

**修法**（照票 operator-channel/45、54 的路数，在入口挡，编排与收寄库不动）

- 在线收寄口：`nodeopshttp` 的 `receptionCommand` 遇到 `mark` 含 NUL 答 400 `MALFORMED_REQUEST`，头注写了理由。只拒 NUL：扫描附带的回车、换行、Tab 照 UC-NO-002 与 ADR-0023 原样收。
- 前线导入：收寄模板译装时用新的 `intakeMarkStorable`（放在 `intakeSourceIDCarriable` 旁边）核 mark 格，含 NUL 就按坏行整批拒，报文点名行号与 `mark` 列。
- **旧缺陷顺带收掉**：明确接收那一支在本票之前就会先拿这个号经身份核对查 PS，在那一步撞 22021（`invalid byte sequence for encoding "UTF8": 0x00`），同样 5xx 或未决。现在一并由入口挡住，红的输出里看得到这一行。

**用例**

| 入口 | 用例 |
|---|---|
| 在线收寄口，传输面 | `internal/nodeoperations/adapters/http`：`TestWorkFactReceptionIntakeRefusesAMarkCarryingNUL`。三种观察各一格，经端点答 400、编排替身零落册；另一格钉 Tab 与回车换行照原样进命令 |
| 在线收寄口，生产装配 | `cmd/parcel-api`：`TestTheWiredReceptionRefusesAMarkCarryingNUL`。生产的作业事实收寄口加生产装配的收寄编排，三种观察各一格答 400、册上 0 行；带 Tab 与回车换行的拒收照常 200 落册，原样读回 |
| 前线导入 | `cmd/parcel-frontline-import`：`TestFrontlineImportRejectsAMarkCellCarryingNUL`（真库，生产的身份核对读口）。三种观察各一格在译装时整批拒、册上不落 |

**先红后绿**：红在 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nrs02-red2.log`，只写了用例、生产代码钉 `6ed9ec11` 一字未改。原话举例：

- 生产装配三格都是「带 NUL 的号答 500 {"error":{"code":"NO_ANSWER_FORMED"}}」。
- 前线导入：
  - 仅扫描与拒收是「第 2 行 → NO_ANSWER_FORMED [未决] 编排未形成答案，已回滚：record reception: save reception: ERROR: unsupported Unicode escape sequence (SQLSTATE 22P05)」。
  - 明确接收是「… resolve parcel identity: … find parcels by external identifier: ERROR: invalid byte sequence for encoding "UTF8": 0x00 (SQLSTATE 22021)」。
- 传输面三格都是 200 或 201 带业务结果，没有答坏报文。

绿在同目录 `nrs02-green2.log`。

**门**（`0a3a441d` 的工作树）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 均退 0；带 DSN 跑 `go test -p 1 -count=1 ./internal/nodeoperations/... ./cmd/parcel-api/... ./cmd/parcel-frontline-import/... ./migrations/... ./internal/architecture/...`，全部 ok。机制清点有差，单独一笔 `c8cc32c5`。

**证据层级**：S。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `6ed9ec11`，修补后钉 `fd065937`）

两轴合审。

- **阻断（已修于 `0a3a441d`）**：外部标识带 NUL 时，来源文档写进 jsonb 撞 22P05，拒收与仅扫描两支由本票变成 5xx、设备永远重投。照票 45、54 的路数在两处入口拒成坏报文（在线收寄口 400 `MALFORMED_REQUEST`、前线导入整批拒并点名列），只拒 NUL，回车、换行、Tab 照旧原样收。明确接收那一支今天在 PS 身份读口撞 22021，是旧缺陷，同由入口挡住。三种观察经生产装配真库各一格先红后绿。
- **Spec**：ADR-0199 决定一、二落到。来源文档 NRS-1 与 NOC-1 指纹分开，指纹两个函数逐字不动；判断版本与收寄结果版本同一签发者，形成与待识别行以 CHECK 守「判断版本即收寄版本」；前版（租户，前版）部分唯一索引守「一版至多一个后继」，撞上译成 `ErrPredecessorAlreadySucceeded`；`Save` 的 `ON CONFLICT` 收窄到主键，前版撞键不再被吞成已有记录。ADR-0196 冲突版指名先到那件此刻的当前版本，在（租户、包裹）咨询锁之后同一事务里读，并发可见；冲突版带后到那次送站的来源文档，照 ADR-0199 决定一原文。已有结果带 `recordedOutcome` 与 `currentVersion` 两个信息格，不进 outcome 词表。
- **Standards**：「新行必带来源与版本」由编排守、不立 NOT VALID CHECK，理由（多个包直写夹具）写在迁移 0007 头注，与 0006 同一取舍。
- **已知缺口（照 ADR-0199「不回填」，不另立票）**：第一期之前落下的冲突版没有前版、不在链上，先到那件重放时 `currentVersion` 是原判断；第三件再来时新冲突版也从原判断接，原判断实际有两个后继、只一个上链。方向依据读全部行，不受影响。第一期之前的拒收与仅扫描行没有判断版本，票 04 续办时要处理（作者已写进「发现」）。
