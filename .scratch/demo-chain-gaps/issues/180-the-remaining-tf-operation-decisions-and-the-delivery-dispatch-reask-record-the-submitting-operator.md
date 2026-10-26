# 180 TF 其余运营决定口与派送重问、补拍不记提交操作者

Category: bug
Status: resolved——2026-10-26 进 main `acfda8eee`（见文末「进 main 记录」）。此前 in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-dcg180` tip `34030af1c`（其后一笔只改本票面），见文末「完成记录」。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领，分支 `claude-dcg180`（worktree `idp-parcel-dcg180`）；TF 迁移取 0054（0053 归票 130），ADR 若要取 0308。此前 ready-for-agent——2026-10-26 票 126 隔离评审之后立（上线前必修：真实租户上线后，没记下的决定无法补记，理由同复核报告 M1），见票 [126](./126-tf-operation-decisions-record-the-submitting-operator.md)「评审记录」Spec 非阻断 1、2 与「修订轮」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 126 隔离评审；[ADR-0303](../../../docs/adr/0303-transport-fulfillment-operation-decisions-record-the-submitting-operator-as-operation-evidence.md) 决定五与越权风险点 1、4；代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M1；[ADR-0230](../../../docs/adr/0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 决定七

## 现象（读码于票 126 分支）

票 126 按 ADR-0303 让关段、明确终止、建派送任务、装载分配、两个判断口与运输委托提交七口记下提交操作者。同在操作者渠道「运营决定」能力面上、同样形成决定的还有两组，Intake 仍走 `tfhttp.OperatorDecisionIntake.tenantFor`，认证出的操作者在 Intake 里就丢了：

- 形成决定记录的六口：订舱提交 `/transport-fulfillment-booking-request-submissions`、取消订舱 `/transport-fulfillment-booking-request-cancellations`、取消运输委托 `/transport-fulfillment-transport-commission-cancellations`、履约段采用运输委托 `/transport-fulfillment-segment-commission-adoptions`、容量预占 `/transport-fulfillment-capacity-reservations`、容量释放 `/transport-fulfillment-capacity-releases`。
- 派送补拍 `/transport-fulfillment-delivery-dispatch-triggers` 与按册重问 `/transport-fulfillment-delivery-dispatch-reasks`（决定种类「派送重问」，ADR-0230 决定七）。ADR-0303 决定五定了执行器形成的任务不记建任务人——任务的内容出自所有者给的派送要求；但「这一次再问」是哪一位按的，至今哪里都没留下。产品没有逐请求的接入日志，事后无从补记。

票 126 没并进来，是因为订舱与容量池两组的读写面当时正由票 127 改动（ADR-0303 决定五）。

## 修法

- **六口**：照 ADR-0303 把信封里的提交操作者（「发行方#sub」，`domain.RegistrantReference`）作为操作证据落在各自的决定记录上——订舱那一份、订舱的取消、委托的取消、段采用的那一代、那一份预占、那一次释放。待遇照 ADR-0303 决定三（即 ADR-0187 三条）：只进审计，不进重放与内容冲突比对，不进任何门；写面回执不带；可缺、不回填，存量读作「没记下是谁」。查阅面已展示这些记录的（如订舱册），补出操作者。
- **派送重问与补拍**：仍不记在任务上（ADR-0303 决定五），另记在重问决定处——落点待作者定：例如派送待形成册上「最近一次问」旁记问的人，或另立一份重问记录；补拍对单件、按册重问批量，两口都要留下是谁、何时按的。重问的执行结果与答复形状不变。
- **文档**：扩 ADR-0303 的口径到这几口并定重问的落点（新 ADR 或在 ADR-0303 文末补复核记录，按停用规则挂指针）；相关 UC 立 AT 行（号取当时空号，全仓与在途分支核过）；AT 盘点①补行；迁移号取当时下一个空号。

## 完成判据

- 六口每口：命令收操作者、记录加列、读回带出；同一决定由另一操作者重放答复不变、在册的不被顶替（每口一条用例钉）；不带操作者照样形成；库面拒空白与错位。
- 补拍与重问：按的人可查，带 PG 的用例钉住；执行器形成的任务仍不记建任务人。
- 生产装配真库用例：两个令牌各投一次，库里是首投者，回执不含操作者。

## 完成记录（2026-10-26，Claude Code 作者子代理，会话 `tops-20` 派单，分支 `claude-dcg180`）

起点：`51aca18cb`（含认领）。

| 提交 | 内容 |
|---|---|
| `7c58e11ee` | 代码、TF 迁移 `0054` 与各层用例 |
| `34030af1c` | ADR-0303 文末复核记录；ADR-0151 Links 补一句；UC-TF-004 立 `AT-TF-348`–`353`、UC-TF-006 立 `354`、`355`；AT 盘点①八行 |
| 完成记录那一笔 | 本节与 Status 行 |

### 做了什么

- **八口 Intake** 全改走 `identityFor`（`tenantFor` 已无人用，删）；八个 `…Payload.Command` 改收 `OperatorIdentity`，操作者进命令的 `SubmittedBy`（订舱）、`CancelledBy`（取消订舱、取消委托）、`AdoptedBy`、`ReservedBy`、`ReleasedBy`、`TriggeredBy`、`ReaskedBy`。载荷里自报一律 400（封闭解码）。
- **迁移** `migrations/transport_fulfillment/0054_remaining_operation_decisions_record_the_submitting_operator.sql`：`booking_request.submitted_by`/`cancelled_by`、`transport_commission.cancelled_by`、`segment_commission_adoption.adopted_by`、`capacity_reservation.reserved_by`、`capacity_release.released_by`，全部可空、不回填，CHECK 不准空白，取消人只在已取消的行上；新表 `delivery_dispatch_ask`（问询册）。
- **编排**：六个命令各加一格，只做形状核（空白即缺席、NUL 答未受理，建在任何读写之前）后挂到记录上；重放、冲突比对与各门一行未改。取消那两口在 `Replace` 前给副本写取消人；委托开始（段采用）不写取消人。
- **登记册**：订舱、委托、采用三张读写新列（取消只在取消那一次 `Replace` 写）；**容量池**按值换存时预占与释放明细整组删了再插回，故 `CapacityPoolRecord` 旁加 `ReservedBy`/`ReleasedBy` 两张按引用的表，读回时由明细行填、编排新作时只加那一份的键（复制新表，不原地改）、写回时随明细落列。
- **查阅**：订舱册（`registry=booking-request`）每行带 `submittedBy`/`cancelledBy`（omitempty，只在订舱册行上，不进提交口回执用的 `bookingRequestBody`）。
- **重问落点**：见下节。
- **装配**（`cmd/parcel-api`）：补拍执行器接问询册（与那一拍同笔）；按册重问接 `transactionalAskLedger`（另起一笔事务）。`cmd/parcel-dispatch` 的执行器不接（它发起的那一路不带操作者）。

### 重问落点（代决 1）

**另立只增不改的问询册 `delivery_dispatch_ask`，一次按下一行**：补拍记段与对象，按册重问记问法与件数上限（一次一行，不逐件记）；`asked_by` 不可空，只有命令带了操作者才记，执行器随交接信封发起的那一路与重问逐件交来的都不记。不取「待形成册最近一次问旁记人」：那本册一行只留最近一次、留不下「每一次」，补拍的那一件也可能没有待形成行（一问就形成了不进册）。补拍与执行器那一拍同笔；重问先于逐件去问另起一笔，写不下一件也不问。不新增答复格，写不下答错误。候选与反方写在 ADR-0303 复核记录里；该落点是决定五「另记在重问决定处」里的落脚选择，不构成新取舍，故未另立 ADR-0308。

### 代决与越权风险点（用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；编号对 ADR-0303 复核记录）

1. **重问落点取问询册**（机制；ADR-0303 复核记录越权风险点 9）。
2. **册写不下就不问、不答**（机制；风险点 10）：审计放在可用性之前。
3. **执行器报错的补拍不留痕**（机制；风险点 11）：补拍记录与那一拍同笔，那一拍回滚则没有效果也没有痕迹。
4. **问询册与委托取消人、采用人、预占人、释放人没有 HTTP 查阅面**（机制；风险点 12）。只订舱册带出；「按的人可查」在库与端口（`ListAsks`）层成立。本票不开新读面。
5. **池证据靠记录带回**（机制；风险点 13）：任何以新铸 `CapacityPoolRecord` 调 `Replace` 的新代码会抹掉证据，今天只有读回再改的编排；靠同包用例守。
6. **存量与册建立之前的按下永久缺席**（机制；风险点 14）。
7. **范围里订舱册带出提交人与取消人，其余不带**（机制）：票面只点名订舱册。
8. **新增 AT 号**：UC-TF-004 `AT-TF-348`–`353`、UC-TF-006 `354`、`355`。全仓与各 `claude-*` 分支核过：最大号在 `claude-dcg130` 的 `347`，`348` 起无人用；`347` 留给 130。
9. **没读的**：UC-TF 各篇除验收示例外的正文，ADR-0201 至 0204 正文（风险点 15）。

### 完成判据 → 用例

| 判据 | 用例 |
|---|---|
| 六口命令收操作者、载荷自报 400 | `adapters/http/operation_evidence_remaining_test.go::TestEachRemainingDecisionFaceCarriesTheEnvelopeOperatorIntoItsCommand`（八口逐口，含补拍与重问） |
| 记录加列、读回带出；另一位操作者重放答复不变、在册的不被顶替；不带操作者照样形成 | 编排 `application/operation_evidence_remaining_test.go`：订舱提交、取消订舱、取消委托、段采用、预占、释放各一条（含 NUL 答未受理、零写入）；真库 `adapters/postgres/operation_evidence_remaining_test.go`：订舱、委托、采用、池各一条（撞键 / 再取消写中 0 行、缺席落 NULL、取消人落在没取消的行被 CHECK 拒、池换存之后证据仍在）；订舱册读面 `::TestTheBookingRegistryShowsTheOperatorsAndTheReaskReceiptDoesNot` |
| 库面拒空白与错位 | 真库各用例末尾的 `wantCheckViolation`（含问询册两种形状互斥、问法封闭、上限为正） |
| 补拍与重问：按的人可查（带 PG）；执行器形成的任务仍不记建任务人 | 编排 `::TestADeliveryDispatchTriggerRecordsWhoAskedAndNeverTheTask`、`::TestAReaskRecordsWhoPressedItOnceAndAsksNothingWhenItCannot`；真库 `::TestTheDeliveryDispatchAskLedgerKeepsWhoPressedWhat`、`::TestTheDeliveryDispatchAskLedgerRefusesToRunOutsideATransaction`（无事务即拒，同时满足架构门的写口负向证据）；生产装配 `cmd/parcel-api/operation_decision_operator_remaining_test.go::TestTheDeliveryDispatchAsksRecordWhoPressedThemAgainstARealDatabase` |
| 生产装配真库：两个令牌各投一次，库里是首投者，回执不含操作者 | 同文件 `::TestTheBookingCancellationAndAdoptionDecisionsRecordTheirOperatorsAgainstARealDatabase`（订舱提交、取消订舱、取消委托、采用）、`::TestTheCapacityDecisionsRecordTheirOperatorsAgainstARealDatabase`（预占、释放，含换存池后证据仍在） |

改前核对（临时改坏、跑、改回，已确认工作树干净）：预占写明细时不落 `reserved_by` → `TestTheCapacityPoolKeepsTheReservingAndReleasingOperatorsAcrossReplacements` 红（预占人表为空）；`CancelCommission` 不把取消人挂到副本 → `TestACommissionCancellationRecordsItsOperatorAndAReplayByAnotherKeepsIt` 红；按册重问不记按下 → `TestAReaskRecordsWhoPressedItOnceAndAsksNothingWhenItCannot` 与 `cmd/parcel-api` 补拍重问用例红（问询册只剩两行补拍）。

### 自验

于 `7c58e11ee`（其后只动文档与本票面）：`gofmt -l internal cmd migrations` 无输出；`go build ./...`、`go vet ./internal/transportfulfillment/... ./cmd/...` 退 0；**带 DSN**（`127.0.0.1:55432`）`go test -count=1 -p 1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./cmd/parcel-dispatch/... ./cmd/parcel-transport-register/... ./internal/architecture/... ./migrations/...` 全部 ok（新真库用例 `-v` 核过是 PASS 不是 SKIP）；文档提交之后另跑 `./internal/architecture/... ./migrations/... ./tools/...` 全 ok。`go run ./tools/at-coverage-counts -check` 只报「生成物与文件不一致」，不报缺行、多行。未重生成 ADR 索引、AT 计数、机制清点，未跑全量。已有一条旧用例（`TestTheReaskFaceAnswersItsWholeFaceGrades`）因 Intake 里带操作者而需要问询册，给它的夹具接了个内存替身，断言一字未改。

进 main 时请留意：AT 盘点①八行插在 `AT-TF-346` 之后（`347` 留给 130），UC-TF-004 末尾追加六行（与 130 若也追加同一表会在该处重叠）；ADR-0303 文末追加一节，ADR-0151 只改 Links 里挂 0303 的那一条；迁移 `0054`（`0053` 归 130）。

## 评审记录

评审 ← tops-20 隔离子代理(sonnet) · 钉 `7f570a9d0`（只读 diff 与点名文件，未跑 go test）

- **阻断**：无。
- **非阻断**：
  1. 容量池旁表，越权风险点 13 可接受：`Replace` 对每个预占、释放按 `record.ReservedBy[ref]`/`record.ReleasedBy[ref]` 写回（`insertReservations`）；`findPool` 读回只填有证据的键，`withReservedBy`/`withReleasedBy` 复制旧表再加新键，旧键不丢不错挂；非测试代码只有 `prepare_transport_opportunity.go` 新铸 `CapacityPoolRecord`（新建池，无旧证据可丢），其余经 `replacePool` 用读回的 record；无删除预占/释放的路径，明细先 DELETE 再 INSERT、缺键即 NULL 不残留；`findPool` 对池行 `FOR UPDATE`，预占与释放串行。只靠 `TestTheCapacityPoolKeepsTheReservingAndReleasingOperatorsAcrossReplacements` 守，可带着上线。
  2. 补拍目标段或对象不合法（`deliveryTriggerTargetFrom` 不接受）时不写问询册——与代决 11 同类（按下没效果就没行），ADR-0303 复核记录风险点 11 只写了执行器报错一种，宜补一句（**未改，记在此处**）。
- **无发现**：八口重放与冲突不读操作者（比对只用 `ContentDigest`、`Declares(...)`；重放分支在加键前返回；`Replace` 守卫 `cancelled_at IS NULL` 未改；`AdoptedBy` 不进规范化载荷；`TriggeredBy`/`ReaskedBy` 只决定写不写册）；问询册不影响执行器路径（执行器那一路 `TriggeredBy` 为空不写册；补拍同笔、重问另起一笔；未接册答 `ErrDeliveryDispatchAskLedgerNotWired`、写不下答 `ErrDeliveryDispatchAskUnrecorded`；旧用例 `TestTheReaskFaceAnswersItsWholeFaceGrades` 只动夹具、断言未改）；迁移 0054 各列可空、CHECK 先 `IS NULL`、问询册形状互斥、存量施加不失败；ADR-0303 只追加复核记录、ADR-0151 只改 Links；号不撞。

## 进 main 记录（2026-10-26，推送方会话 tops-20）

- **main**：`acfda8eee`。出处：分支 `claude-dcg180` tip `7f570a9d0`（作者为 Sonnet 子代理）。
- **重放对照**（分支 → main）：`7c58e11ee` → `210eb7f12`（代码、TF 迁移 0054）；`34030af1c` → `b2b185fc8`（ADR-0303 复核记录、UC、AT 盘点）；`7f570a9d0` → `f785f7524`（完成记录）。
- **重放冲突**：`.scratch/at-coverage-inventory.md` ①表 `AT-TF-347`（票 130）与本票 `AT-TF-348`–`355` 插在同一处，按号顺接；UC-TF-006 自动合并。
- **生成物**：AT 盘点总览计数 `915a935e5`；机制清点 `acfda8eee`；ADR 索引无变化。三件 `-check` 退 0（含票 139 表行守卫）。
- **评审**：见上「评审记录」，无阻断；非阻断 2（风险点 11 补一句）按用户 2026-10-26 节省 token 的要求未另开修订轮，记在评审记录。
- **全量真库测试**：`scripts/test-full.sh` 跑在 `acfda8eee`（即所推 SHA）：158 ok / 0 FAIL，退 0。
- **种子**：`scripts/demo-seeds/seed.sh` 灌一次性库（`acfda8eee` 上），退 0，用后删库。
- **号**：TF 迁移 `0054`；`AT-TF-348`–`355`；未立新 ADR（ADR-0303 文末复核记录）。
