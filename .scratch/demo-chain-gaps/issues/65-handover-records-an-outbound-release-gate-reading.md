# 65 交出门禁：已交接逐对象读交出准入并照记

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg65（2026-10-19 Claude Code 作者子代理；立票时：ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。）
Blocked by: [59](./59-handover-declares-releasing-receiving-and-onward-nodes.md)
地盘：TF 新 `domain/handover_gate_reading.go`（与出发读数同一封闭集，可抽共用内核，出发那一格的答复字节不变）、新 `ports/handover_gate.go`、新 `application/read_handover_gate.go`、新 `adapters/customscompliance/handover_gate.go`；`application/register_transport_handover.go`（读数随记录）、`adapters/postgres/transport_handover_registry.go` + TF 迁移（认领时现取）、tfhttp 交接响应；`cmd/parcel-api/assemble_control_facts.go`（写事务之前读）；文档 UC-TF-005「当前限制」行与 AT、ADR-0206 Links 加后继指针。
出处：[tf-chain-plan](../tf-chain-plan.md)「等 peer 的」外包干线交接的跨境门禁；方案「代决」C13，「需要的 ADR」ADR-0213。

## 做什么

- 交接登记对 `已交接` 逐对象按交出意图问 CC，读数随交接同笔照记，不拒收；未声明交出节点、未声明去向的答法照方案 C13。
- 进 main 的令牌次序见方案「波次与令牌」。

## 完成判据

- 同区：不受门禁。跨区无案：未决。阻断：照样 201。
- 未声明交出节点：不问 CC，记未决。
- CC 读不动：照登，记读不动。
- 重投回显首次读数，不重问。
- 真库测试。
- 演示：SZX-GATE 交给干线承运商（`releasingNode=SYN-NODE-SZX-GATE`、`onwardNode=SYN-NODE-SIN-HUB`）→ 201 `HANDOVER_REGISTERED`，`gateReading` 为未决，依据是跨关务区域移动无案。

## 规模

M

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg65`，基于 `claude-dcg59` 的 `aa94b0ca`）

授权：用户原话「你作为业务和系统专家自决……解决所有的问题」。统一代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)「代决」C13；落地时补的裁定写进 [ADR-0213](../../../docs/adr/0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 文末补记，那里是唯一权威，本节只列去处。

### 做了什么

- **共用内核**：五格（`GateAnswer`）、依据（`GateBasis`）、逐对象那一格（`GateFinding`）从出发读数抽到 `domain/gate_reading.go`。出发一侧的名字留作别名。持久化里逐对象那一格的序列化与五格名称镜像改为两种读数共用。出发的线格式与明细 jsonb 逐字不变，出发既有用例全过。
- **领域**：`domain/handover_gate_reading.go`，含停点、读数与归属门 `WithGateReading`；「没声明交出节点就不问」只定义在 `UnaskedGateReading` 一处。交接带读数；更正版不承继读数。重建门要求`已交接`必带读数、其余裁决不带，读数要属于这一版。
- **端口与编排**：
  - `ports/handover_gate.go`。
  - `application/read_handover_gate.go`：`HandoverGateReader.ReadRegistration` / `ReadCorrection`，写事务之前、在池上问。
  - 登记编排加 `RegisterWithGateReading`、`CorrectWithGateReading`，以及认重放的 `RecordedHandoverGateReading`、`RecordedCorrectionGateReading`。
  - `Register` / `Correct` 留作不必问关务的那一种。
- **关务适配器**：`adapters/customscompliance/handover_gate.go` 以交出意图问。答复对照抽成 `findingFromResult`，与出发同一份。译不回来的哨兵同时包住两个端口的那一个。
- **存储**：
  - TF 迁移 `0041`：两列与四道 CHECK；存量`已交接`回填未决「登记于门禁读数之前」。
  - 登记册的各条读路都带回读数；查阅册列整格，`ports.TransportHandoverCatalogueRow.GateReading` 与 tfhttp 交接册 `gateReading`。
- **线格式**：交接答复加 `gateReading`，与出发读数同形，`at` / `onward` 代 `from` / `to`，没声明去向写明 `onwardNotDeclared`。不带关务判断标识与依据原文。
- **装配**：`cmd/parcel-api/assemble_control_facts.go` 只改交接那一处，写事务之前认重放、问读数。关务那一口另装一份，抽成 `buildHandoverGateReader`，与生产同形的竞态用例取同一份。`endpoints.go`、`main.go` 未动。
- **文档**：
  - UC-TF-005：「当前限制」一行、结果、流程、规则、一致性各补一句，验收立 `AT-TF-236`–`245`。
  - TF CONTEXT：「交出门禁读数」词条、交接一节规则一条、边界一句。
  - ADR-0213：Status 与 Links 加前向指针，文末补记。
  - ADR-0205：Links 一行。
  - 演示动线补记，AT 盘点补行与追记。
  - ADR-0206 的后继指针票 59 已加，本票不再动。README 未动：本票没有停用条款。
- **另立票**：[86](./86-blocked-handover-gate-readings-raise-exception-signals.md)（交出阻断读数派生异常信号）、[87](./87-handover-register-shows-the-gate-reading-bases.md)（运营查阅看得到依据原文）。

### 代决

ADR-0213 决定四与方案 C13 之外补的裁定，理由都在 ADR-0213 文末补记：

1. 更正成`已交接`的那一版也读：照承继的交出节点、去向节点与业务时间另读一份，不承继前版的读数。票面只写了「交接登记」，没写更正。不读的话，待确认更正成`已交接`这条路上的跨境交出就没有读数。
2. 更正在写事务之前读不动前版：停点「交接登记册读不动」，属读不动，更正照登。这一格只属更正版。
3. 时点取交接的业务时间（`judgedAt`）。
4. 存量`已交接`回填未决「登记于门禁读数之前」，不补问；迁移 `0039` 之后已声明节点的存量行也一样。
5. 去向与交出节点相同时，关务立不起交出意图，记读不动（译不回来）。本上下文仍不核三格之间的关系。
6. 答复不带关务判断标识与依据原文，口径同 ADR-0205 决定一；查阅册只列整格。依据的运营读面另立票 87。
7. 编排的不带读数入口（`Register` / `Correct`）只在不必问关务时由领域补读数，要问却没问就上抛，不补一格未决。
8. 读数里存问关务时的意图（at / onward）。它与这一版的节点列重复，由重建门核一致，好让读数自成一份记录，也与出发读数的起讫同形。
9. 读数在认出不是重放之后、落册之前才挂上：重放与冲突用不上传进来的读数，也就不核它。
10. 共用内核用别名保住出发一侧的名字，不全仓改名，免得撞在途的票 69。

### 越权风险点

编号与正文以 ADR-0213 文末补记为准，这里只列一行：

- 8：同一次交出可能有两份不同的读数（TF、CC owner）。
- 9：读不动没有补问（TF owner、运维）。
- 10：存量`已交接`一律回填未决（TF owner）。
- 11：对象只取交接的那一件，不展开集运单元，境内交出读成不受门禁是一格假的干净读数（TF、NO owner）。
- 12：更正在写事务前后读到的前版不一致时上抛 500，靠离线队列重投补上（TF owner）。
- 13：阻断读数还不派生异常信号，见票 86（TF、VE owner）。
- 14：运营看不到依据原文，见票 87（TF owner）。

### 用例

`AT-TF-236`–`245` 全部入 AT 盘点①，半边覆盖在行内注明：

- 领域：`handover_gate_reading_test.go`。
- 编排：`read_handover_gate_test.go`，关务是替身。
- 关务适配器真库：`handover_gate_test.go`。
- TF 真库：`transport_handover_gate_reading_test.go`，含迁移 `0041` 回填，回填语句取迁移原文。
- 线格式：`handover_gate_reading_test.go`。
- 生产装配真库：`cmd/parcel-api/handover_gate_reading_test.go`。

演示一条在生产装配真库用例上走，区域照种子登，证据 `S`：

- 深圳口岸交给干线承运商（`releasingNode=SYN-NODE-SZX-GATE`、`onwardNode=SYN-NODE-SIN-HUB`）答 201 `HANDOVER_REGISTERED`，`gateReading` 为 `UNDECIDED`。依据落册为 `CROSS_CUSTOMS_MOVEMENT` / `CUSTOMS_SCOPE_UNRESOLVED`，即跨关务区域移动无案。
- 之后把新加坡枢纽改登进 CN，原样重发答 200 `EXISTING_VERSION`、回显首次的 `UNDECIDED`，关务判断册不多一行。另一件新交出答 `NOT_GUARDED`，证明关务确实会改答，上一次重发确实没有重问。

### 既有用例的改动（随行为变的，不是放宽）

- 票 59 的编排与线格式节点用例：声明了交出节点的`已交接`改走「写事务之前问读数」的次序。关务那一口用替身。
- 票 59 的 `cmd/parcel-api` 节点用例：答复里多了对象格 `gateReading`，解码改为对象格留原文。
- 直写登记册的 TF 真库夹具、`cmd/parcel-dispatch` 交接投影夹具：照编排的样子给`已交接`挂上读数。空白节点直插用例补两列合法读数，断言仍落在节点那道 CHECK 上。
- `cmd/parcel-api` 交接竞态用例的装配补上读数那一侧，与生产同形。
- 关务适配器真库夹具多建一个交出那一侧的视图。

### 验证（实测，只记 `S`）

- `go build ./...` 通过。`go vet ./internal/transportfulfillment/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./migrations/` 与 `gofmt -l internal cmd migrations` 都干净。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1`，以下各包全 ok：
  - `./internal/transportfulfillment/...`
  - `./internal/architecture/`
  - `./migrations/`
  - `./cmd/parcel-api/`
  - `./cmd/parcel-dispatch/`
  - `./internal/visibilityexception/adapters/...`
  - `./internal/nodeoperations/adapters/transportfulfillment/`
  - `./internal/customscompliance/...`
  - `./internal/networkrouting/...`
- 用 `-v` 逐条确认真库用例是 PASS 不是 SKIP：`TestHandoverGateReadingsRoundTripThroughEveryReadPath`、`TestTheDatabaseGuardsTheHandoverGateReadingShape`、`TestTheMigrationBackfillsEarlierHandoversAsRecordedBeforeTheGateReading`、`TestAnOutboundReleaseIsJudgedOnTheReleasingAndOnwardNodes`、`TestTheWiredHandoverRecordsTheOutboundReleaseGateReadingAgainstARealDatabase`。
- 演示种子未改，没有灌库；没有做一次性库实走，取证见上「用例」。没跑全量，也没重生成机制清点。

### 偏离与待裁

- **号**：AT `236`–`245` 用满；票号用 `86`、`87`；TF 迁移 `0041`；没有新取别的号。
- **票面地盘之外动的文件**：
  - 共用内核与出发一侧别名：`domain/gate_reading.go`、`domain/departure_gate_reading.go`、`adapters/postgres/movement_fact_registry.go`、`adapters/customscompliance/departure_gate.go` 的共用对照。
  - 查阅册整格：`ports/catalogue_read.go`、`adapters/postgres/review_catalogue.go`、tfhttp `query_transport_fulfillment_records.go`。
  - 文档：TF CONTEXT、ADR-0205 Links、演示动线、AT 盘点。
  - 测试：上一节列的几处既有用例。
- **与并行分支的协调点**：
  - 61/62/63 及以后的票，若用应用层 `Register` / `Correct` 直登声明了交出节点的`已交接`，会收到 `ErrHandoverGateReadingMisplaced`。改走 `RegisterWithGateReading`，或走生产装配。
  - 直写登记册的真库夹具要给`已交接`挂读数：`UnaskedGateReading` 加 `WithGateReading`。新的 CHECK 拒没有读数的`已交接`，交接的重建门也要求它带读数。
  - 票 69 若改关务适配器 `departure_gate.go`，会与本票抽出的 `findingFromResult` 及换成中性类型的 `answerFor`、`basesFor` 相邻，冲突应是文本级的。
  - 票 66 交付门禁直接用共用内核与 `findingFromResult`。
- **待 owner 复核**：上列代决与越权风险点。尤其是第 1 条（更正也读、不承继）和第 8 条（读数里存意图）。
- **无被权限检查拦下的改动。**

