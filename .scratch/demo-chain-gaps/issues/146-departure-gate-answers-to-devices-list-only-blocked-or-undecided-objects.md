# 146 出发前问询与出发两口向设备逐对象列出段内全部对象

Category: bug
Status: in-progress——评审两条阻断已修，待复审与进 main，分支 claude-dcg146（2026-10-21，见文末「评审记录」）。此前 in-progress——完工待评审与进 main，分支 claude-dcg146（2026-10-21 Claude Code 作者子代理，变基到 main `b290b777`；见文末「完成记录」）。此前 in-progress——2026-10-21 认领（146、94 同批），分支 claude-dcg146。此前 ready-for-agent——2026-10-20 代决复核立（上线前必修），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 R11；ADR-0219 越权风险点 5（连带 ADR-0205 越权风险点 13）

## 现象与修法

现象、证据与修法的完整写法以复核报告 R11 一条为准（单一权威），此处只记要点：

代决「是」：两口回给设备的逐对象那一格只列被阻断或未决的对象，落册仍记全；回到 AT-TF-063 要求的最小范围。排在票 42 之后（同一读数内核）。

## 完成记录（2026-10-21，Claude Code 作者子代理，分支 `claude-dcg146`，与票 94 同批，变基到 main `b290b777`）

授权：用户原话「你作为业务和系统专家自决……解决所有的问题」与 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」。裁定的单一权威是代决复核报告 R11；落地时补的裁定写进 [ADR-0237](../../../docs/adr/0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md)，那里是唯一权威，本节只列去处。

### 做了什么

- **领域**：`domain/gate_reading.go`（门禁读数共用内核）加 `ExecutorGateCell` 与 `ExecutorGateCells`——「执行方看得到哪几格」的唯一判法：只列阻断、读不动、未决的对象；集运单元那一格只指名同样列出的成员。它是投影，不带关务判断标识与依据原文，推不回单元那一格。读数、重建门、存储都不动。
- **线格式**：tfhttp 加 `gateObjectViewsOf`，出发读数（移动事实口与出发前问询共用的视图）、交出读数、交付读数三种视图的逐对象那一格都只经它取。整格、起讫 / at / onward、停点照实给全；全都得出放行时答复里没有 `objects` 键。
- **不动的**：端口、迁移、信封、VE 信号、运营查阅、派发都不变；落册的那一份照旧每件一格。
- **文档**：
  - ADR-0237 新立；ADR-0205、0213、0219、0225 的 Status 行与 Links 加前向指针（0205 决定一、0225 决定六、0213 两则补记「答复与查阅」各部分停用一句，0219 无条款停用）。`docs/adr/README.md` 未手改。
  - TF CONTEXT「实际移动与出发门禁读数」加一条（规则的权威处）；UC-TF-005「一致性、幂等与并发」最小范围那一句补指针，`AT-TF-110` 补一句，验收立 `AT-TF-316`–`320`；UC-TF-006 立 `AT-TF-321`。
  - 演示脚本补记一条；AT 盘点①补行与追记（只加行，总览未动）。

### 代决

内容与理由只在 ADR-0237，这里只列编号与去处：

1. 读不动与未决、阻断同列（R11 原文只写「被阻断或未决」）：决定一第四条，越权风险点 1。
2. 袋那一格只指名同样列出的成员，放行的成员不随袋带出；问不到成员或空袋的袋在列、`members` 为空数组：决定二，越权风险点 5。
3. 交出与交付这两种单件读数也照同一判法收窄（票面只写出发两口）：决定一「范围」与候选「交出与交付读数不收窄」，越权风险点 6。
4. 判法放在领域投影一处，各口线格式只经它取：决定三。
5. 落册、重放回显、信封、VE 与运营查阅不收窄：决定三。
6. 判法归产品策略（替所有租户定），投影与线格式归机制，没有租户取值：三类一段。
7. （评审修订补）出发意图的起讫与声明的段绑定——班次起讫要就是段里在控各件进段那次`已交接`声明的交出、去向节点，两格都要声明，对不上或说不出就停在本侧、不问关务：决定五，越权风险点 7–11。见文末「评审记录」B1。

### 越权风险点

编号与正文以 ADR-0237 为准，这里只列一行：

- 1：读不动也列，R11 字面只写阻断或未决（TF owner）。
- 2：剩余暴露——任一场外设备仍问得出任一段没得出放行的那几件，不留痕（TF owner）。
- 3：关务或成员关系读不动的故障期，段里每件都在列（TF owner、运维）。
- 4：放行的对象不列，设备无从对账段里问了哪几件（TF owner、一线客户端 owner）。
- 5：`members: []` 的含义从「空袋」变为「没有同样列出的成员」（TF owner、NO owner）。
- 6：单件读数也收窄，答复里那一件放行时没有逐对象那一格（TF owner）。
- 7–11（评审修订补）：交接漏声明节点的段出发不问关务；只核节点不核班次身份；交接声明错了出发就停在本侧（段内多站同样对不上）；每件多读一次交接册；存量读数不回补（TF owner、一线客户端 owner、运维）。越权风险点 2、3 同时改写：剩余暴露改指段自己的方向（跨区段、票 147 之后的未登区域节点），后继票 171。

### 用例

`AT-TF-316`–`321` 全部入 AT 盘点①，半边覆盖在行内注明：

- 领域：`domain/executor_gate_cells_test.go`（`TestOnlyCellsThatDidNotPassAreShownToTheExecutor`、`TestAUnitCellNamesOnlyTheMembersThatAreShownToo`）。
- 线格式：`record_movement_fact_test.go::TestADepartureThatPassesShowsTheExecutorNoObjects`（新）；`departure_gate_consolidation_test.go::TestTheBagCellListsItsMembersOnTheWire`、`handover_gate_reading_test.go::TestTheHandoverEndpointRepliesWithTheGateReading`、`delivery_gate_reading_test.go` 两条（随行为改）。
- 生产装配真库：新 `cmd/parcel-api/departure_gate_executor_scope_test.go`——
  - `TestTheExecutorIsShownOnlyTheObjectsThatDidNotPass`：演示动线自营 SHA→SZX 一段里装两个客户的货（客户 A 一件散件、一件在袋里，都被限制装载出发；客户 B 一件散件、一件在同一袋里，都不受门禁）。问询与同参出发都答阻断，回给设备的只有袋（`members` 只有被挡那一件）与客户 A 的两件，客户 B 的两件、作业实物标识、关务判断标识在两口答复里都不出现；两口 `gateReading` 逐字节相同；库面那一行五件各一格、四件包裹各带关务判断标识。
  - `TestADepartureThatPassesShowsTheExecutorNoObjectsButRecordsThemAll`：不受门禁时两口答复都没有 `objects` 键、逐字节相同，库面照旧记着那一件。
- 交出与交付的真库半边落在既有用例上（`handover_gate_reading_test.go`、`delivery_gate_reading_test.go` 加断言）。

### 既有用例的改动（随行为变的，不是放宽）

- 断言「不受门禁那一件列在答复里」的几处改成「不列」：tfhttp `delivery_gate_reading_test.go`（首登、重放、更正）；`cmd/parcel-api` 的 `assemble_movement_fact_test.go`（演示出发）、`assemble_departure_gate_preview_test.go`（演示问询）、`delivery_gate_reading_test.go`（演示交付）。阻断那几处另加「列出那一件」的断言。
- tfhttp `departure_gate_consolidation_test.go`：袋里两件改成一件阻断、一件不受门禁，断言袋只指名被挡那一件、另一件不出现；为此 `record_movement_fact_test.go` 的关务替身加了按对象点名的答复（缺省照旧）。
- tfhttp 与 `cmd/parcel-api` 的 `handover_gate_reading_test.go`：不受门禁那一件断言不列。

### 验证（实测，只记 `S`）

- `go build ./...`、`go vet ./...` 通过，`gofmt -l internal cmd migrations` 干净；两笔代码提交各自在 `git archive` 出的干净目录里 `go build ./... && go vet ./...` 通过。
- `scripts/test-full.sh` 跑改动包、它们的反向依赖（含 `cmd` 三包）、`./internal/architecture/...`、`./migrations/...`，清单存 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/0d01979a-68c3-4b61-b295-1b04bc643b8b/scratchpad/dcg146/test-packages-final.txt`，变基到 `97841f2d` 后重跑全 ok；收口时再变基到 `b290b777`，相对 `97841f2d` 只多两笔 `.scratch` 文档提交，没有重跑（与票 94 同一跑）。
- `-v` 逐条确认两条新真库用例是 PASS 不是 SKIP。
- 变异：把 `ExecutorGateCells` 的判法改成全列，领域 2 条、线格式 4 条、真库 2 条变红；复原后全绿。
- 没跑全量，没重生成机制清点、ADR 索引与 AT 计数。

### 偏离与待裁

- **号**：ADR-0237；`AT-TF-316`–`321`；无迁移；本票没有新立票。
- **`AT-TF-063` 仍留在 AT 盘点③**：本票只证了门禁读数回给执行方那一半，跨客户共同运输的查询面本身没有落地。
- **演示没做一次性库实走**：取证是生产装配上的真库用例。
- **票面地盘之外**：交出与交付两种读数的答复视图（`register_transport_handover.go`、`register_effective_delivery.go`）也经同一判法收窄，见代决 3；对应的既有用例随之改。
- **待 owner 复核**：上列代决与越权风险点，尤其是代决 1（读不动也列）与越权风险点 2（剩余暴露）。
- **无被权限检查拦下的改动。**

## 评审记录（2026-10-21，隔离评审；作者子代理修订，分支 `claude-dcg146`，main 仍是 `b290b777`，未变基）

阻断 2 项，都修了；顺手 7 项全办（3–5 是票 94 的代码与用例，6、7 立票）。

### 阻断

**B1——146 没堵住 R11：设备给一段境内段配一班跨区班次，段里每件都答未决、都回给设备。** 修了，走的是评审给的第一条路：就在本票把出发意图的起讫与声明的段绑定。

- **为什么走这条**：绑定只用本上下文自己的事实（各件进段那次`已交接`声明的交出、去向节点，ADR-0213 决定一），认进段交接的认法交付读数早已有一份，抽成一处共用即可；改动是领域一个判法、三种停点、一口端口（原交付专用名改成共用名）、装配一处，加测试夹具，没有迁移（停点存在 jsonb 明细里，读回的封闭集随领域加三格）、没有新信封，出发登记与问询走的本就是同一条读数路径，两口一处改。评审给的另一种做法「起讫改从进段交接取」没取：要停用 ADR-0205 决定二「起讫取班次方向」的权威来源，而段里各件的进段交接可以各不相同、一份读数只有一对起讫，还得另立「取哪一件的」规则；核对只在对不上时不问，保住了既有口径。退回最低限度那条路（票 166、改完成记录为「R11 未完全满足」）没有走，166 未用。
- **修法**（提交 `b471feb6`）：
  - 领域：`DepartureEntryStops`——段里出发那一刻在控的每一件，进段交接声明的交出节点要是班次起点、去向节点要是班次终点；对不上停在 `SCHEDULE_NOT_SERVING_SEGMENT`（未决），说不出（凭收寄进段、没声明那两格、交接在册上认不出）停在 `SEGMENT_ENTRY_NODES_UNKNOWN`（未决），交接册读不动停在 `HANDOVER_REGISTRY_UNAVAILABLE`（读不动）。`ActualFulfillmentSegment.ParticipationsUnderControlAt` 交回在控那几件的参与，与 `ObjectsUnderControlAt` 同一口径（后者改为经它取）。
  - 应用：出发读数在问集运成员关系与关务之前核；认进段交接收成 `entryHandoverOf`（新文件 `application/entry_handover.go`），交付读数取 at 也改走它，行为不变。
  - 端口：`ports.DeliveryGateEntryHandoverView` 改名 `ports.EntryHandoverView`，挪到新文件 `ports/entry_handover.go`；出发读数编排多一口交接登记册，`buildDepartureGateReader` 接真交接登记册。
  - 只核节点、不核班次身份：同方向改搭别班照样核得过。
- **用例**：
  - 编排：新 `application/read_departure_gate_entry_test.go`——对不上、两件里一件对不上、没声明去向、一格都没声明、一件说不出一件对不上、凭收寄进段、交接在册上认不出、交接册读不动，八种都停在本侧、不问成员关系与关务、回给设备一件不列，出发登记与问询同答；对得上的照问关务。
  - 领域与库面：停点各属一格的表加三种；停点往返用例加两条带新停点的读数。
  - 真库：`cmd/parcel-api/departure_gate_executor_scope_test.go::TestPairingASegmentWithAnotherSchedulesDirectionNoLongerWidensTheReading` 复现评审 PoC——同一段境内段装两个客户的包裹，照种子登深圳→新加坡那一班；按本班问答不受门禁、一件不列；改配跨区班次问答未决、停在 `SCHEDULE_NOT_SERVING_SEGMENT`、一件不列、整库指纹不变（连关务都没问）；照这么配登出发 201、读数同停、库面没有逐对象那一格；交接没声明节点的段停在 `SEGMENT_ENTRY_NODES_UNKNOWN`、一件不列。
- **变异实测**：把出发读数里的绑定拆掉（`entryStops` 的结果丢弃），上面的真库用例与编排用例都变红；真库用例的失败输出正是 PoC 那一份：`{"answer":"UNDECIDED","from":"SYN-NODE-SZX-GATE","to":"SYN-NODE-SIN-HUB","objects":[{"object":"SYN-PARCEL-146-POC-CUST-A","answer":"UNDECIDED"},{"object":"SYN-PARCEL-146-POC-CUST-B","answer":"UNDECIDED"}]}`。复原后全绿。
- **随行为变的既有用例**（不是放宽）：出发读数现在要进段交接声明了两格才问关务，所以几处夹具照演示动线的样子声明交出、去向节点——编排的 `departureSegments`（改为由声明了 NODE-SHA → NODE-SZX 的`已交接`立段，另给交接册替身）、tfhttp 的 `movementGateReaderOver`、`cmd/parcel-api` 的 `departureLine.handOver`（声明 SYN-NODE-SHA-HUB → SYN-NODE-SZX-GATE，另开 `handOverDeclaring`）、`assemble_movement_fact_test.go` 里手装读数编排那一处补交接册；`TestAMisansweringCustomsViewIsAnErrorAndMissingDependenciesAreRefused` 补「缺交接册那一口也装得起来」一支。
- **文档**（提交 `6a5a4f50`）：ADR-0237 补决定五、候选与反方、对已接受记录的作用（部分停用 ADR-0205 决定二的停点名单；ADR-0219 读数另取决于交接登记册、无条款停用），越权风险点 2、3 改写，补 7–11；ADR-0205、0219 的 Status 与 Links 指针随改。TF CONTEXT「实际移动与出发门禁读数」加一条；UC-TF-005 `AT-TF-114`、`AT-TF-319` 补写这一格，不占新号；演示脚本补记与 AT 盘点两行补点名。剩余暴露（段自己确是跨区、票 147 之后起讫有未登区域节点时仍全列，且问的人可以是任一场外设备）立票 171。

**B2——权威文档把重放回显写成了不收窄。** 修了（提交 `6a5a4f50`）：TF CONTEXT 那一条、ADR-0237 决定三与 Status 行三处都改成「重放回显的是首次读数，回给设备时同样只列没得出放行的」，重放回显从「不收窄的读者」里拿掉。

### 顺手

3. `preview_delivery_gate_test.go` 首条用例注释的「本文件末一条用例」改指 `TestADeliveryPreviewWritesNothingAndIsNotHandedToTheDelivery`（票 94，提交 `45fc6441`）。
4. `work_fact_intake.go` 的 `IntakeDeliveryGatePreview` 注释不再数别的文件的字段（同上）。
5. `cmd/parcel-api/transport_fulfillment_command_lines_test.go` 补 `/transport-fulfillment/delivery-gate-previews` 一行：越过 Intake 答 200 `DELIVERY_GATE_PREVIEWED`（对象不在任何段里、停在本侧），自报 `tenantId` 答 400（同上）。
6. 立票 [171](./171-gate-previews-are-scoped-to-the-asking-devices-work-and-leave-a-trace.md)：两种问询一起按作业范围或派送指派限对象，并在本上下文留痕；ADR-0237 越权风险点 2、ADR-0238 越权风险点 6、7 指向它。
7. 立票 [172](./172-customs-judgement-register-grows-with-every-gate-preview.md)：关务判断册随问询增长，保留期与限流；ADR-0238 越权风险点 4、5 指向它（提交 `1962d6b1`）。另在票 147 追加一节，写明它落地之后与门禁读数收窄的交互。

### 未采纳

- 评审的退回路线（票 166、改写 R11 满足度）：主路线走通了，不需要。
- 「起讫改从进段交接取」：见 B1「为什么走这条」。
- 在本票里按作业范围限问询的对象：段与派送任务今天没有作业范围归属，要先定指派从哪来，归票 171。

### 评审修订后的验证（实测，只记 `S`）

- `go build ./...`、`go vet ./...` 通过，`gofmt -l internal cmd migrations` 干净；两笔代码提交（`b471feb6`、`45fc6441`）各自在 `git archive` 出的干净目录里 `go build ./... && go vet ./...` 通过。
- `scripts/test-full.sh ./internal/transportfulfillment/... ./cmd/parcel-api/... ./cmd/parcel-dispatch/... ./cmd/parcel-transport-register/... ./internal/visibilityexception/... ./internal/architecture/... ./migrations/...`：32 包全 ok、0 FAIL（输出存 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/0d01979a-68c3-4b61-b295-1b04bc643b8b/scratchpad/dcg146/test-run-review.txt`）。
- `-v` 确认新真库用例 PASS；变异实测见 B1。
- 没跑全量，没重生成机制清点、ADR 索引与 AT 计数；无迁移；无被权限检查拦下的仓库改动（scratchpad 里一条带变量路径的清理命令被安全检查拦下，没有执行，改用字面路径清理）。
