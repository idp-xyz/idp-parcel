# 66 交付门禁：有效交付读 FINAL_DELIVERY 准入并照记

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg66（2026-10-19 Claude Code 作者子代理，基于 `origin/main` `ea61d108`；见文末「完成记录」。立票时：ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。）
Blocked by: [59](./59-handover-declares-releasing-receiving-and-onward-nodes.md)、[65](./65-handover-records-an-outbound-release-gate-reading.md)
地盘：TF `application/register_effective_delivery.go`、新 `application/read_delivery_gate.go`、新 `adapters/customscompliance/delivery_gate.go`；`adapters/postgres/effective_delivery.go` + TF 迁移（认领时现取）、tfhttp 交付响应；`cmd/parcel-api/assemble_delivery.go`；文档 UC-TF-006 与 AT。
出处：[tf-chain-plan](../tf-chain-plan.md)「等 peer 的」有效交付的 `FINAL_DELIVERY` 门禁；方案「代决」C14，「需要的 ADR」ADR-0213。

## 做什么

- 有效交付按交付意图问 CC，节点取对象进入该派送段那次交接的交出节点，读数照记；节点不可得的答法照方案 C14。
- 读数形状复用 65 的内核。
- 进 main 的令牌次序见方案「波次与令牌」。

## 完成判据

- 交付节点取进派送段那次交接的交出节点。
- 不受门禁、阻断、未决、读不动各一格，均照登。
- 凭收寄进段或未声明节点：记未决「交付节点不可得」。
- PS 终局照常。
- 真库测试。
- 演示：有效交付 → 201 `DELIVERY_REGISTERED`，`gateReading` 为 `NOT_GUARDED`；PS 终局照常形成。

## 规模

M

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg66`，基于 `origin/main` `ea61d108`）

授权：用户原话「你作为业务和系统专家自决……解决所有的问题」。统一代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)「代决」C14；落地时补的裁定写进 [ADR-0213](../../../docs/adr/0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 文末「补记：票 66 落决定五时的补充裁定」，那里是唯一权威，本节只列去处。

### 做了什么

- **领域**：
  - `domain/delivery_gate_reading.go`：停点、读数、归属门 `WithGateReading`。读数取 65 抽出的共用内核（`domain/gate_reading.go`）。
  - 有效交付带读数，`CorrectProof` 值拷贝即承继；重建门要求每一版都带，只多收存量回填那一种停点。
  - 参与认进段交接：`FulfillmentParticipation.EntryHandoverVersion` 拆出入场依据里的交接版本，`EnteredByHandover` 认哪一版是进段那一次。`TRANSPORT-HANDOVER/` 拼写收成一处常量，拼的与拆的共用。
- **端口与编排**：
  - `ports/delivery_gate.go`：段册两问、按版本找`已交接`、关务交付意图三口。
  - `application/read_delivery_gate.go`：`DeliveryGateReader.ReadRegistration`，写事务之前在池上问。
  - 登记编排加 `RegisterWithGateReading` 与认重放的 `RecordedDeliveryGateReading`；`Register` 留给登不出新交付的各路，新交付没带读数上抛 `ErrDeliveryGateReadingMisplaced`。
- **关务适配器**：`adapters/customscompliance/delivery_gate.go` 以 `NewDeliveryIntent(at)` 问，答复对照与出发、交出共用 `findingFromResult`；`ErrUntranslatableAnswer` 同时包住三个端口的哨兵。
- **存储**：
  - TF 迁移 `0043`：两列不许为空，存量交付回填未决「登记于门禁读数之前」；CHECK 守封闭词表、明细形状（停点与答复恰有其一、问了关务就有 at）与那一格对象。
  - 有效交付登记册按键、按版本、更正各读路都带回读数；查阅册列整格（`ports.EffectiveDeliveryCatalogueRow.GateReading` 与 tfhttp 交付册 `gateReading`）。
  - 交接登记册加 `FindHandedOverByVersion`（新文件 `adapters/postgres/delivery_gate_views.go`，连同两个读口的接口断言）。
- **线格式**：交付答复加 `gateReading`，与出发、交出读数同形，`at` 代 `from`；不带关务判断标识与依据原文。更正答复回显承继来的那一份。
- **装配**：`cmd/parcel-api/assemble_delivery.go` 首登写事务之前认重放、问读数；关务那一口另装一份，抽成 `buildDeliveryGateReader`。`endpoints.go`、`main.go` 未动。
- **文档**：
  - UC-TF-006：输入「当前限制」一行，结果、流程、规则、一致性各补一句，验收立 `AT-TF-256`–`265`。
  - TF CONTEXT：「交付门禁读数」词条、「揽收、派送与交付」规则一条、边界一句。
  - ADR-0213：Status 与 Links 加前向指针，文末补记（正文不改）。README 未动：没有停用条款，0213 那一行已写明票 66 落决定五。
  - 演示动线补记，AT 盘点补行与追记。
- **另立票**：[93](./93-blocked-delivery-gate-readings-raise-exception-signals.md)（交付阻断读数派生异常信号，86 的交付那一半）、[94](./94-a-courier-previews-the-final-delivery-gate-before-dispatching.md)（派送出发之前问一次交付门禁）。票 87 补一句：交付读数的依据原文与停点同属那一问。

### 代决

ADR-0213 决定五与方案 C14 之外补的裁定。内容与理由只在 ADR-0213 文末「补记：票 66 落决定五时的补充裁定」，本节只列编号与去处：

1. 「该派送段」取对象此刻在场的那一段（结束参与同一问找的那一段），不核段服务动作：补记「『该派送段』取哪一段」，越权风险点 16。
2. 「那次交接」按（租户、对象、版本）找回`已交接`各版，由领域按入场依据与起点认：补记「『那次交接』的认法」。
3. 时点取交付业务时间：补记「时点」。
4. 「交付节点不可得」按缺在哪一环分成具名停点（`NO_ACTIVE_SEGMENT`、`ENTERED_BY_PICKUP`、`RELEASING_NODE_NOT_DECLARED`、`ENTRY_HANDOVER_UNRESOLVED`），另有三种读不动：补记「停点」。
5. 对象同时在几段在场时读数那一侧响亮报错：补记「『该派送段』取哪一段」。
6. POD 更正承继前版读数、不重问（与交出那一侧更正另读相反）：补记「POD 更正」，越权风险点 18。
7. 存量交付一律回填未决、不补问：补记「停点」，越权风险点 19。
8. 新交付没带读数一律是接线错误，没有「不带读数也定得出」的那一种：补记「编排兜底」。
9. 答复不带判断标识与依据原文，查阅册列整格：补记「答复与查阅」。

### 越权风险点

编号与正文以 ADR-0213 文末「补记：票 66 落决定五时的补充裁定」为准，这里只列一行：

- 16：「该派送段」不核段服务动作，段内转运时 at 仍取进段节点（TF、CC owner）。
- 17：写事务之前与写事务里读到的段可能不同，库面守不到 at 与段册一致（TF owner）。
- 18：POD 更正不重问（TF、CC owner）。
- 19：存量交付一律回填未决（TF owner）。
- 20：读数挡不住交付，派送出发前那一问另立票 94（TF、CC owner）。
- 21：阻断读数还不派生异常信号（票 93），运营看不到依据原文与停点（随票 87）（TF、VE owner）。

### 用例

`AT-TF-256`–`265` 全部入 AT 盘点①，半边覆盖在行内注明：

- 领域：`delivery_gate_reading_test.go`。
- 编排：`read_delivery_gate_test.go`，段册、交接册、关务、派送尝试读面都是替身。
- 关务适配器真库：`delivery_gate_test.go`。
- TF 真库：`effective_delivery_gate_reading_test.go`，含迁移 `0043` 回填（回填语句取迁移原文）、库面五种违例与 NOT NULL、按版本找回`已交接`各版。
- 线格式：`delivery_gate_reading_test.go`。
- 生产装配真库：`cmd/parcel-api/delivery_gate_reading_test.go`。

演示一条的取证是生产装配上的真库用例，区域照种子登，证据只记 `S`：

- 新加坡枢纽交给尾程（`releasingNode=SYN-NODE-SIN-HUB`，带段）之后有效交付：答 201 `DELIVERY_REGISTERED`，`gateReading` 为 `NOT_GUARDED`，`at` 为 `SYN-NODE-SIN-HUB`。落册依据为 `FINAL_DELIVERY` / `OUTSIDE_CUSTOMS_BOOKS`，带关务判断标识，答复里没有。
- `participationEnd` 为 `PARTICIPATION_ENDED`；有效交付登记信封恰入队一封交给 PS 终局，信封形状不变。
- 原样重发答 200 `EXISTING_VERSION`，回显同一读数，关务判断册不多一行。交付册列出整格。
- 另一件包裹上落了最终交付限制：读数 `BLOCKED`，照样 201、参与照常结束。尾程交接没声明交出节点：`UNDECIDED` / `RELEASING_NODE_NOT_DECLARED`，不问关务。从没进过段的包裹：`UNDECIDED` / `NO_ACTIVE_SEGMENT`，结束参与答 `NO_ACTIVE_PARTICIPATION`，不问关务。

PS 终局一侧：`cmd/parcel-dispatch` 的有效交付终局与追踪投影用例改用带读数的交付登记行，生产路由表照旧投到 PS 终局与 VE，全过；PS 读交付只取交付本身各格，不读读数。

### 既有用例的改动（随行为变的，不是放宽）

- 同包交付编排用例（`register_effective_delivery_test.go`、`delivery_ends_participation_test.go`、`end_fulfillment_participation_test.go`，以及经 `newDeliveryFixture` 的其余各处）：首登照组合根的样子经 `gatedDeliveryHandler` 带一份停点读数。
- tfhttp 交付与作业事实口用例：处理器换成 `gatedDelivery`，带一份停点读数。
- 直写交付登记册的真库夹具（TF postgres 的 `deliveryRecord`、`catalogueDeliveryRecord`，`cmd/parcel-dispatch` 的 `recordRegisteredEffectiveDelivery`）：照编排的样子挂读数。POD 必备那条直插用例补上两列合法读数，断言仍落在 POD 那道 CHECK 上。
- 关务适配器真库夹具多建一个交付那一侧的视图。

### 验证（实测，只记 `S`）

- `go build ./...` 通过；`go vet ./internal/transportfulfillment/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./migrations/` 与 `gofmt -l internal cmd migrations` 都干净。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1`，以下各包全 ok：
  - `./internal/transportfulfillment/...`
  - `./internal/architecture/`
  - `./migrations/`
  - `./cmd/parcel-api/`
  - `./cmd/parcel-dispatch/`
  - `./internal/customscompliance/...`
  - `./internal/visibilityexception/adapters/...`
  - `./internal/parcelshipment/adapters/transportfulfillment/`
- 用 `-v` 逐条确认新加的真库用例是 PASS 不是 SKIP：`TestDeliveryGateReadingsRoundTripThroughEveryReadPath`、`TestTheDatabaseGuardsTheDeliveryGateReadingShape`、`TestTheMigrationBackfillsEarlierDeliveriesAsRecordedBeforeTheGateReading`、`TestTheEntryHandoverIsFoundByVersionAcrossScopes`、`TestAFinalDeliveryIsJudgedAtTheDeliveryNode`、`TestTheWiredDeliveryRecordsAFinalDeliveryGateReadingAgainstARealDatabase`、`TestTheWiredDeliveryRegistersWhateverTheGateReads`。
- 演示种子未改。没跑全量，没重生成机制清点。

### 偏离与待裁

- **号**：AT `256`–`265` 用满；票号用 `93`、`94`；TF 迁移 `0043`；没有新取别的号。票 94 若要立 ADR，号由协调方给。
- **演示没做一次性库实走**：协调方口径允许「一次性库实走或真库装配用例」，本票取后者。实走要从一单受理、收寄、交接一路走到派送尝试，派送任务的形成还牵着票 64 的交付段；若评审要求实走，按 65 那次的做法补。
- **票面地盘之外动的文件**：
  - 领域：`domain/effective_delivery.go`、`domain/delivery_rehydration.go`（交付带读数），`domain/transport_handover.go`、`domain/actual_fulfillment_segment.go`（拼写收成常量，字节不变）。
  - 编排：`application/register_effective_delivery.go`（地盘内）。
  - 适配器：`adapters/customscompliance/departure_gate.go`（共用哨兵与包头注释），`adapters/postgres/review_catalogue.go`、`ports/catalogue_read.go`、tfhttp `query_transport_fulfillment_records.go`（交付册整格），新 `adapters/postgres/delivery_gate_views.go`、`adapters/postgres/effective_delivery_gate_reading.go`。
  - 文档：TF CONTEXT、演示动线、AT 盘点、票 87 一句。
  - 测试：上一节列的几处既有用例。
- **与并行分支的协调点**：
  - 票 67（实际履约发生项随有效交付信封自动形成）若直写交付登记册或经应用层 `Register` 直登新交付，会收到 `ErrDeliveryGateReadingMisplaced` 或 CHECK 拒：改走 `RegisterWithGateReading`、生产装配，或照本票夹具挂一份读数。
  - 演示动线里票 65 的补记有新旧两段并存（合批时留下的），本票没动它们，新补记接在后面。进 main 时可顺手删掉旧的那段。
- **待 owner 复核**：上列代决与越权风险点。尤其是第 1 条（不核段服务动作）、第 6 条（POD 更正承继、与交出那一侧取舍相反）和第 4 条（「交付节点不可得」拆成具名停点）。
- **无被权限检查拦下的改动。**
