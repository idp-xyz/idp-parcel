# 65 交出门禁：已交接逐对象读交出准入并照记

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg65（2026-10-19 Claude Code 作者子代理；隔离评审 0 阻断，已按评审修订并 rebase 到 `129e9a38`，见文末「完成记录」「评审记录」；立票时：ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。）
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

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg65`；起初基于 `claude-dcg59` 的 `aa94b0ca`，评审后 rebase 到 `origin/main` `129e9a38`）

授权：用户原话「你作为业务和系统专家自决……解决所有的问题」。统一代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)「代决」C13；落地时补的裁定写进 [ADR-0213](../../../docs/adr/0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 文末补记，那里是唯一权威，本节只列去处。

### 做了什么

- **共用内核**：五格（`GateAnswer`）、依据（`GateBasis`）、逐对象那一格（`GateFinding`）从出发读数抽到 `domain/gate_reading.go`。出发一侧的名字留作别名。持久化里逐对象那一格的序列化与五格名称镜像改为两种读数共用。出发的线格式与明细 jsonb 逐字不变，出发既有用例全过。
- **领域**：`domain/handover_gate_reading.go`，含停点、读数与归属门 `WithGateReading`；「哪些`已交接`不问关务」只定义在 `UnaskedGateReading` 一处。交接带读数；更正版不承继读数。重建门要求`已交接`必带读数、其余裁决不带，读数要属于这一版。
- **端口与编排**：
  - `ports/handover_gate.go`。
  - `application/read_handover_gate.go`：`HandoverGateReader.ReadRegistration` / `ReadCorrection`，写事务之前、在池上问。
  - 登记编排加 `RegisterWithGateReading`、`CorrectWithGateReading`，以及认重放的 `RecordedHandoverGateReading`、`RecordedCorrectionGateReading`。
  - `Register` / `Correct` 留作不必问关务的那一种。
- **关务适配器**：`adapters/customscompliance/handover_gate.go` 以交出意图问。答复对照抽成 `findingFromResult`，与出发同一份。译不回来的哨兵同时包住两个端口的那一个。
- **存储**：
  - TF 迁移 `0041`：两列与几道 CHECK，含问了关务的读数与这一行声明一致的那一道；存量`已交接`回填未决「登记于门禁读数之前」。
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

ADR-0213 决定四与方案 C13 之外补的裁定。内容与理由只在 ADR-0213 文末补记，本节只列编号与去处：

1. 更正成`已交接`也读、不承继：补记「更正」。
2. 更正前读不动前版记「交接登记册读不动」：补记「更正」与「停点」。
3. 时点取 `judgedAt`：补记「时点」。
4. 存量回填未决、不补问：补记「停点」，越权风险点 10。
5. 去向与交出节点相同（评审后改判）：补记「停点」`ONWARD_SAME_AS_RELEASING`，越权风险点 15。
6. 答复不带判断标识与依据原文：补记「答复与查阅」，票 87。
7. 不带读数的入口只补定得出的那一份：补记「编排兜底」。
8. 读数里存意图：补记「读数的形状」，库面一致性见补记「存储」。
9. 读数在认出不是重放之后才挂：补记「重投」。
10. 共用内核留别名：补记「读数的形状」。
11. 写事务里定得出时不落读不动（评审后加）：补记「更正」。
12. 库面守读数意图与这一行声明一致（评审后加）：补记「存储」。

### 越权风险点

编号与正文以 ADR-0213 文末补记为准，这里只列一行：

- 8：同一次交出可能有两份不同的读数（TF、CC owner）。
- 9：读不动没有补问（TF owner、运维）。
- 10：存量`已交接`一律回填未决（TF owner）。
- 11：对象只取交接的那一件，不展开集运单元，境内交出读成不受门禁是一格假的干净读数（TF、NO owner）。
- 12：更正在写事务前后读到的前版不一致时上抛 500，靠离线队列重投补上（TF owner）。
- 13：阻断读数还不派生异常信号，见票 86（TF、VE owner）。
- 14：运营看不到依据原文，见票 87（TF owner）。
- 15：去向与交出节点相同记未决、不问关务（TF、CC owner）。

### 用例

`AT-TF-236`–`245` 全部入 AT 盘点①，半边覆盖在行内注明：

- 领域：`handover_gate_reading_test.go`。
- 编排：`read_handover_gate_test.go`，关务是替身。评审后加 `TestAHandoverWhoseOnwardIsItsReleasingNodeIsUndecidedWithoutAskingCustoms`（`AT-TF-237`）、`TestACorrectionThatNeedsNoCustomsAnswerIsNotLeftUnreadable`（`AT-TF-243`）。
- 关务适配器真库：`handover_gate_test.go`。
- TF 真库：`transport_handover_gate_reading_test.go`，含迁移 `0041` 回填，回填语句取迁移原文；评审后加新停点往返与一致性 CHECK 的五种违例。
- 线格式：`handover_gate_reading_test.go`。
- 生产装配真库：`cmd/parcel-api/handover_gate_reading_test.go`。

演示一条有两份取证，证据都只记 `S`。

生产装配真库用例，区域照种子登：

- 深圳口岸交给干线承运商（`releasingNode=SYN-NODE-SZX-GATE`、`onwardNode=SYN-NODE-SIN-HUB`）答 201 `HANDOVER_REGISTERED`，`gateReading` 为 `UNDECIDED`。依据落册为 `CROSS_CUSTOMS_MOVEMENT` / `CUSTOMS_SCOPE_UNRESOLVED`，即跨关务区域移动无案。
- 之后把新加坡枢纽改登进 CN，原样重发答 200 `EXISTING_VERSION`、回显首次的 `UNDECIDED`，关务判断册不多一行。另一件新交出答 `NOT_GUARDED`，证明关务确实会改答，上一次重发确实没有重问。

一次性库实走（评审后补）：在一次性库 `dcg65_walk` 上 `seed.sh` 灌入退 0，起 `parcel-api`（操作者渠道三件加治理权威串），用 Dex 配置员令牌、华东场外设备（`SYN-DEVICE-OFFSITE-CN-EAST-01`，`OFFSITE` / `SYN-OFFSITE-CN-EAST`）打 `POST /transport-fulfillment/handovers`：

- 深圳口岸交给干线承运商、去向新加坡枢纽：201 `HANDOVER_REGISTERED`，`gateReading` 为 `UNDECIDED`，`at` / `onward` 照声明。库面依据为 `CROSS_CUSTOMS_MOVEMENT` / `CUSTOMS_SCOPE_UNRESOLVED`，带关务判断标识。
- 原样重发：200 `EXISTING_VERSION`，回显同一读数。
- 去向上海枢纽：`NOT_GUARDED`。不带去向：`NOT_GUARDED`，写明 `onwardNotDeclared`。
- 不带交出节点、去向等于交出节点：各答 201 `UNDECIDED`，停点分别为 `RELEASING_NODE_NOT_DECLARED`、`ONWARD_SAME_AS_RELEASING`。
- 拒收：201，不带读数，库面两列皆 NULL。
- 不带令牌：401 `OPERATOR_CREDENTIAL_REJECTED`。
- 交接册逐行列出整格。关务判断册共三行：重发没有重问，两种停点也都没有问。
- 走完已停进程、`DROP DATABASE … WITH (FORCE)`。

### 既有用例的改动（随行为变的，不是放宽）

- 票 59 的编排与线格式节点用例：声明了交出节点的`已交接`改走「写事务之前问读数」的次序。关务那一口用替身。
- 票 59 的 `cmd/parcel-api` 节点用例：答复里多了对象格 `gateReading`，解码改为对象格留原文。
- 直写登记册的 TF 真库夹具、`cmd/parcel-dispatch` 交接投影夹具：照编排的样子给`已交接`挂上读数。空白节点直插用例补两列合法读数，断言仍落在节点那道 CHECK 上。
- `cmd/parcel-api` 交接竞态用例的装配补上读数那一侧，与生产同形。
- 关务适配器真库夹具多建一个交出那一侧的视图。

### 验证（实测，只记 `S`）

以下是评审修订并 rebase 到 `129e9a38` 之后重跑的结果。

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
- 演示种子未改。一次性库实走见上「用例」。没跑全量，也没重生成机制清点。

### 偏离与待裁

- **号**：AT `236`–`245` 用满；票号用 `86`、`87`；TF 迁移 `0041`；没有新取别的号。评审修订也没有新取号：新停点归入 `AT-TF-237`，读不动让位归入 `AT-TF-243`，越权风险点 15 接在 ADR-0213 补记的编号后面。
- **票面地盘之外动的文件**：
  - 共用内核与出发一侧别名：`domain/gate_reading.go`、`domain/departure_gate_reading.go`、`adapters/postgres/movement_fact_registry.go`、`adapters/customscompliance/departure_gate.go` 的共用对照。
  - 查阅册整格：`ports/catalogue_read.go`、`adapters/postgres/review_catalogue.go`、tfhttp `query_transport_fulfillment_records.go`。
  - 文档：TF CONTEXT、ADR-0205 Links、演示动线、AT 盘点。
  - 测试：上一节列的几处既有用例。
- **与并行分支的协调点**：
  - 61/62/63 及以后的票，若用应用层 `Register` / `Correct` 直登声明了交出节点的`已交接`，会收到 `ErrHandoverGateReadingMisplaced`。改走 `RegisterWithGateReading`，或走生产装配。**已知会撞的一处**（评审点名）：`claude-dcg63` 上的 `cmd/parcel-dispatch/reassess_on_handover_test.go`，`handoverCommand` 用 `register.Register` 直登 `HANDED_OVER` 并声明 `ReleasingNode="SYN-NODE-SZX-GATE"`。63 与本票谁后进 main，谁就要把这条路径改走 `RegisterWithGateReading`（或与生产同形的 `transactionalHandover`），并在 rebase 后跑一遍 `./cmd/parcel-dispatch/`。本票不改 63 的分支。`claude-dcg61` 的 `handOverAtShanghai` 没声明交出节点，不受影响。
  - 直写登记册的真库夹具要给`已交接`挂读数：`UnaskedGateReading` 加 `WithGateReading`。新的 CHECK 拒没有读数的`已交接`，交接的重建门也要求它带读数。
  - 票 69 若改关务适配器 `departure_gate.go`，会与本票抽出的 `findingFromResult` 及换成中性类型的 `answerFor`、`basesFor` 相邻，冲突应是文本级的。
  - 票 66 交付门禁直接用共用内核与 `findingFromResult`。
- **待 owner 复核**：上列代决与越权风险点。尤其是第 1 条（更正也读、不承继）、第 5 条（去向与交出节点相同停在本侧，评审后改判）和第 8 条（读数里存意图）。
- **无被权限检查拦下的改动。**

## 评审记录（2026-10-19，作者子代理，分支 `claude-dcg65`）

隔离评审 0 阻断，非阻断 8 项。7 项修了；1 项（通知票 63）不在本分支能改的范围内，登记了去向。

### 修了的

1. **分支基底是票 59 的旧 SHA**（评审建议进 main 前修）。执行 `git rebase --onto origin/main aa94b0ca claude-dcg65`，落到 `129e9a38`。只有 UC-TF-005 验收表一处冲突，两边的行都留：票 28 的 `AT-TF-211` 在前，本票 `AT-TF-236`–`245` 在后。rebase 后 `go build ./...` 通过。机制清点进 main 时统一重生成，本票不做。
2. **关务适配器测试头注数了另一文件夹具里的节点**（评审建议进 main 前修）。去掉计数，只指符号 `newDepartureGateDB`。
3. **更正前读不动、写事务里读得动且不必问关务时仍落读不动**。改在 `withGateReading`：交来的读数停在「交接登记册读不动」，而 `UnaskedGateReading` 定得出时，落定得出的那一份。前版声明了交出节点时照留读不动，因为该问的意图确实没问成。补编排用例 `TestACorrectionThatNeedsNoCustomsAnswerIsNotLeftUnreadable`，去掉修复时它会红。ADR-0213 补记「更正」加一句，AT-TF-243 补一格。
4. **去向等于交出节点记成读不动 / `CUSTOMS_ADMISSION_UNTRANSLATABLE`**。取评审的第一种修法，在 TF 一侧另立具名停点 `ONWARD_SAME_AS_RELEASING`（未决），由 `UnaskedGateReading` 一处定出，不问关务。这只是读数的停点，交接照登，三格之间的关系仍不核。
   - 问来的读数里去向等于交出节点，构造门直接拒。
   - 关务适配器仍把同节点问询归「译不回来」，作防御；注释写明编排本不会问到这里。
   - ADR-0213 补记的「停点」「关务那一问没形成答复」「编排兜底」三处随改，越权风险点 9 补一句「UNTRANSLATABLE 不是暂时故障，补问时单独分诊」，新增越权风险点 15。
   - 领域、编排、真库往返与一次性库实走都覆盖了这一格。
5. **读数意图与这一行的节点、对象一致，库面不守**。迁移 `0041` 加 CHECK `transport_handover_gate_reading_matches_handover`：明细带答复时，`node` 等于 `releasing_node` 且非空，`onward` 与 `onward_node` 相同（含都为空），`finding.object` 等于 `object_ref`。停在本上下文这一侧的读数不受这道约束。迁移还没进 main，直接改原文件，不另立迁移。真库用例逐一证五种违例都落到这道约束上，也证了对得上的一行能进库、能读回。
6. **演示没在一次性库上实走**。补做了，见完成记录「用例」，演示动线补记换成实走记录。
7. **「一决策一处定义」**。
   - UC-TF-005 的「规则」一条与「当前限制」一行压成指针，指向 ADR-0213 决定四与文末补记。
   - TF CONTEXT 的词条与规则只留不变量，答法改指补记。
   - 本票「代决」一节只留编号与去处。

### 去向登记的

8. **票 63 的夹具与本票合流会红**（评审建议进 main 前处理）。本票不改 `claude-dcg63`。协调点写进完成记录「偏离与待裁」，并在返回里报协调方，由协调方转告 63 的作者：后进 main 的一方把 `reassess_on_handover_test.go` 的 `handoverCommand` 改走 `RegisterWithGateReading`（或 `transactionalHandover`），并在 rebase 后跑 `./cmd/parcel-dispatch/`。

### 修订提交

- `7d47e8e4`：去向等于交出节点另立停点、读不动让位于定得出的那一份、库面一致性 CHECK，以及测试头注去掉计数。
- `eb98ea6f`：ADR-0213 补记与越权风险点 15，UC-TF-005 与 TF CONTEXT 压成指针，AT 盘点点名新用例。
- `ee441c3b`：演示动线补记换成一次性库实走记录。
- 本笔：完成记录随评审修订与 rebase 更新，并加本节。
