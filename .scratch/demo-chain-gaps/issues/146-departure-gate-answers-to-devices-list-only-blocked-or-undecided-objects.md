# 146 出发前问询与出发两口向设备逐对象列出段内全部对象

Category: bug
Status: in-progress——二轮评审两条阻断已修，待复审与进 main，分支 claude-dcg146（2026-10-21，变基到 main `4384a85a`，见文末「评审记录」二轮一节）。此前 in-progress——评审两条阻断已修，待复审与进 main，分支 claude-dcg146（2026-10-21，见文末「评审记录」）。此前 in-progress——完工待评审与进 main，分支 claude-dcg146（2026-10-21 Claude Code 作者子代理，变基到 main `b290b777`；见文末「完成记录」）。此前 in-progress——2026-10-21 认领（146、94 同批），分支 claude-dcg146。此前 ready-for-agent——2026-10-20 代决复核立（上线前必修），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
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
5. 落册、重放回显、信封、VE 与运营查阅不收窄：决定三。（已由评审 B2 改口径：重放回显的是首次读数，回给设备时同样只列没得出放行的；落册、信封、VE 与运营查阅不收窄。）
6. 判法归产品策略（替所有租户定），投影与线格式归机制，没有租户取值：三类一段。
7. （评审修订补）出发意图的起讫与声明的段绑定：决定五，越权风险点 7–12。见文末「评审记录」B1 与二轮一节。（已由二轮评审改口径：声明了的那一格与班次起讫对不上才停在本侧、不问关务；说不出——凭收寄进段、交接没声明全、在册认不出——照班次方向问关务、落册记全，回给设备的只有整格、起讫与 `entryUnbound`，一件不列。一轮写的「两格都要声明，对不上或说不出就停在本侧」不再成立。）

### 越权风险点

编号与正文以 ADR-0237 为准，这里只列一行：

- 1：读不动也列，R11 字面只写阻断或未决（TF owner）。
- 2：剩余暴露——任一场外设备仍问得出任一段没得出放行的那几件，不留痕（TF owner）。
- 3：关务或成员关系读不动的故障期，段里每件都在列（TF owner、运维）。
- 4：放行的对象不列，设备无从对账段里问了哪几件（TF owner、一线客户端 owner）。
- 5：`members: []` 的含义从「空袋」变为「没有同样列出的成员」（TF owner、NO owner）。
- 6：单件读数也收窄，答复里那一件放行时没有逐对象那一格（TF owner）。
- 7–12（评审修订补，二轮改口径）：凭收寄进段与交接没声明全的段上设备只见整格；只核节点不核班次身份；交接声明错了或段内多站时诚实出发停在本侧、不形成信号；每件多读一次交接册；存量读数不回补；核不上的段上配假班次换得到一位汇总，照这么登出发会落一份按错误意图问来的读数并派生信号，持交接登记授予的一方塞一笔不声明节点的交接能把段降成只回整格（TF owner、VE owner、一线客户端 owner、运维）。越权风险点 2、3 同时改写：剩余暴露改指段自己的方向（跨区段、票 147 之后的未登区域节点），后继票 171。一轮写的「交接漏声明节点的段出发不问关务」已由二轮撤掉。

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

**B1——146 没堵住 R11：设备给一段境内段配一班跨区班次，段里每件都答未决、都回给设备。** 修了，走的是评审给的第一条路：就在本票把出发意图的起讫与声明的段绑定。（本节记的是一轮的做法：凡写「说不出停在本侧」「`SEGMENT_ENTRY_NODES_UNKNOWN`」「要两格都声明才问关务」的地方，二轮都已撤掉——说不出照问、只回整格，停点撤掉；以下文二轮一节与 ADR-0237 决定五为准。）

- **为什么走这条**：绑定只用本上下文自己的事实（各件进段那次`已交接`声明的交出、去向节点，ADR-0213 决定一），认进段交接的认法交付读数早已有一份，抽成一处共用即可；改动是领域一个判法、几种新停点（一轮三种，二轮撤掉「说不出」那一种、余两种）、一口端口（原交付专用名改成共用名）、装配一处，加测试夹具，没有迁移（停点存在 jsonb 明细里，读回的封闭集随领域加格）、没有新信封，出发登记与问询走的本就是同一条读数路径，两口一处改。评审给的另一种做法「起讫改从进段交接取」没取：要停用 ADR-0205 决定二「起讫取班次方向」的权威来源，而段里各件的进段交接可以各不相同、一份读数只有一对起讫，还得另立「取哪一件的」规则；核对只在对不上时不问，保住了既有口径。（二轮评审改准：一轮修订其实把「说不出」也停在了本侧，见下文二轮一节；二轮之后才是对不上的不问、核不上的照问而只回整格。）退回最低限度那条路（票 166、改完成记录为「R11 未完全满足」）没有走，166 未用。
- **修法**（提交 `b471feb6`）：
  - 领域（一轮，二轮已改）：`DepartureEntryStops`——段里出发那一刻在控的每一件，进段交接声明的交出节点要是班次起点、去向节点要是班次终点；对不上停在 `SCHEDULE_NOT_SERVING_SEGMENT`（未决），说不出（凭收寄进段、没声明那两格、交接在册上认不出）停在 `SEGMENT_ENTRY_NODES_UNKNOWN`（未决），交接册读不动停在 `HANDOVER_REGISTRY_UNAVAILABLE`（读不动）。二轮换成 `BindDepartureEntries`，说不出不再是停点。`ActualFulfillmentSegment.ParticipationsUnderControlAt` 交回在控那几件的参与，与 `ObjectsUnderControlAt` 同一口径（后者改为经它取）。
  - 应用：出发读数在问集运成员关系与关务之前核；认进段交接收成 `entryHandoverOf`（新文件 `application/entry_handover.go`），交付读数取 at 也改走它，行为不变。
  - 端口：`ports.DeliveryGateEntryHandoverView` 改名 `ports.EntryHandoverView`，挪到新文件 `ports/entry_handover.go`；出发读数编排多一口交接登记册，`buildDepartureGateReader` 接真交接登记册。
  - 只核节点、不核班次身份：同方向改搭别班照样核得过。
- **用例**：
  - 编排（一轮，二轮已改）：新 `application/read_departure_gate_entry_test.go`——一轮把对不上与说不出的几种都断言成停在本侧；二轮拆成两条用例，说不出的几种改断言照问关务、标未绑定、一件不列。
  - 领域与库面：停点各属一格的表加三种；停点往返用例加两条带新停点的读数。
  - 真库：`cmd/parcel-api/departure_gate_executor_scope_test.go::TestPairingASegmentWithAnotherSchedulesDirectionNoLongerWidensTheReading` 复现评审 PoC——同一段境内段装两个客户的包裹，照种子登深圳→新加坡那一班；按本班问答不受门禁、一件不列；改配跨区班次问答未决、停在 `SCHEDULE_NOT_SERVING_SEGMENT`、一件不列、整库指纹不变（连关务都没问）；照这么配登出发 201、读数同停、库面没有逐对象那一格；交接没声明节点的段一轮断言停在 `SEGMENT_ENTRY_NODES_UNKNOWN`，二轮改为照问关务、标 `entryUnbound`、一件不列。
- **变异实测**：把出发读数里的绑定拆掉（`entryStops` 的结果丢弃），上面的真库用例与编排用例都变红；真库用例的失败输出正是 PoC 那一份：`{"answer":"UNDECIDED","from":"SYN-NODE-SZX-GATE","to":"SYN-NODE-SIN-HUB","objects":[{"object":"SYN-PARCEL-146-POC-CUST-A","answer":"UNDECIDED"},{"object":"SYN-PARCEL-146-POC-CUST-B","answer":"UNDECIDED"}]}`。复原后全绿。
- **随行为变的既有用例**（不是放宽）：出发读数要核进段交接声明的两格（一轮写的是「声明了两格才问关务」，二轮之后没声明全的照问、只回整格），既有用例原本断言的是列出对象的那一种，所以几处夹具照演示动线的样子声明交出、去向节点——编排的 `departureSegments`（改为由声明了 NODE-SHA → NODE-SZX 的`已交接`立段，另给交接册替身）、tfhttp 的 `movementGateReaderOver`、`cmd/parcel-api` 的 `departureLine.handOver`（声明 SYN-NODE-SHA-HUB → SYN-NODE-SZX-GATE，另开 `handOverDeclaring`）、`assemble_movement_fact_test.go` 里手装读数编排那一处补交接册；`TestAMisansweringCustomsViewIsAnErrorAndMissingDependenciesAreRefused` 补「缺交接册那一口也装得起来」一支。
- **文档**（提交 `6a5a4f50`）：ADR-0237 补决定五、候选与反方、对已接受记录的作用（部分停用 ADR-0205 决定二的停点名单；ADR-0219 读数另取决于交接登记册、无条款停用），越权风险点 2、3 改写，补 7–11（二轮又改 7、补 12）；ADR-0205、0219 的 Status 与 Links 指针随改。TF CONTEXT「实际移动与出发门禁读数」加一条；UC-TF-005 `AT-TF-114`、`AT-TF-319` 补写这一格，不占新号；演示脚本补记与 AT 盘点两行补点名。剩余暴露（段自己确是跨区、票 147 之后起讫有未登区域节点时仍全列，且问的人可以是任一场外设备）立票 171。

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

### 二轮（2026-10-21，评审区间 `d794d805..95cca3e9`；作者子代理修订，变基到 main `4384a85a`）

阻断 2 项，都修了；顺手 3 项全办。修订提交（变基后）：`cd62da18`（代码）、`fd31f702`（文档）、`df8d5bca`（变基时 AT 盘点合并整理）。

**B-A——凭收寄进段的段出发读数再也读不出阻断，VE 信号也没了。** 修了，照评审推荐，不重开 B1：把「说不出」与「对不上」分开。
- **判法**：领域 `BindDepartureEntries` 答三格——已绑定、未绑定、对不上（取代一轮的 `DepartureEntryStops`）。声明了的那一格只要有一格与班次起讫不同就是对不上，哪怕另一格没声明；没有对不上、但有说不出的（凭场外揽收或实际承运商首次有效收寄进段、进段交接没声明全、在册认不出）是未绑定。
- **对不上**与**交接册读不动**照旧停在本侧（`SCHEDULE_NOT_SERVING_SEGMENT`、`HANDOVER_REGISTRY_UNAVAILABLE`），不问关务、一件不列。`SEGMENT_ENTRY_NODES_UNKNOWN` 停点撤掉。
- **未绑定**照班次方向问关务：落册记全（库面逐对象格全在），VE 照旧派生信号；读数多一位 `EntryUnbound`，存进 jsonb 明细（`entryUnbound` 键，不写即没有这一位，存量照读、重放回显一致）；回给执行方的只有整格、from/to 与这一位，一件对象都不列——判法在领域 `DepartureGateReading.ExecutorCells`，tfhttp 只经它取。没有放宽「停点与逐对象恰有其一」：未绑定的读数问了关务、没有停点，那道库面 CHECK 不动；这一位另立字段，构造门只许它出现在问了关务的读数上。无迁移。
- **用例**：
  - 领域：`departure_entry_binding_test.go`——表驱动覆盖两格都对、没声明去向、一格都没声明、认不出、一件也认不出、起讫都错、只声明交出节点且错、只声明去向节点且错、说不出在前对不上在后、对不上在前说不出在后；未绑定读数记全每格、回给执行方一件不给、重建带回那一位、停在本侧带那一位是读数不成形。
  - 编排：`read_departure_gate_entry_test.go` 两条——对不上的五种停在本侧；说不出的四种（凭收寄进段、没声明去向、一格都没声明、认不出）照问两件、整格阻断、标未绑定、回给执行方一件不列，问询与出发同一份读数同样问法；对得上的照常列出被挡那一件。
  - 线格式：`TestAnUnboundDepartureShowsTheWholeCellAndNoObjects`，答复正是 `{"answer":"BLOCKED","from":"NODE-SHA","to":"NODE-SZX","entryUnbound":true}`，问询同字节。
  - 库面：`TestAnUnboundDepartureReadingRoundTripsWithItsFindings`，未绑定那一位与每一格往返，没有这一位的行库面没有这个键。
  - 真库（评审探针那一格）：`TestASegmentEnteredByPickupStillAsksCustomsAndRecordsTheBlock`——凭场外揽收进段、一件被限制装载出发：问询与出发都答 `{"answer":"BLOCKED","from":"SYN-NODE-SHA-HUB","to":"SYN-NODE-SZX-GATE","entryUnbound":true}`，库面两件各一格、被扣那一件 `BLOCKED` / `RESTRICTION_IN_FORCE`、带关务判断标识、记着那一位；交接只报交出节点、没报去向的一段同理。
  - 真库（B1 PoC）：`TestPairingASegmentWithAnotherSchedulesDirectionNoLongerWidensTheReading`——已绑定的境内段配跨区班次仍停在 `SCHEDULE_NOT_SERVING_SEGMENT`、一件不列、整库不变；没声明节点的段按本班问答不受门禁、标未绑定、一件不列；给它配跨区班次，回给设备的只有 `{"answer":"UNDECIDED",…,"entryUnbound":true}`。
  - 派发：`cmd/parcel-dispatch` 的 `TestAnUnboundBlockedDepartureStillRaisesTheSignal`——未绑定的阻断读数经生产路由照旧给被挡那一件形成一个发作期。
- **变异实测**（各自复原后全绿）：
  - 把未绑定改回停在本侧：探针格真库用例红（`SYN-SEG-146-PICKUP 回给设备的` 不是那一份）、B1 PoC 用例里「没声明节点的段按本班问」那一支红（答成 `"stops":["SCHEDULE_NOT_SERVING_SEGMENT"]`）、编排用例四支全红（`0 格，未绑定 false，关务 0 次`）。
  - 把投影改为未绑定时照列对象：B1 PoC 用例红，失败输出 `{"answer":"UNDECIDED","from":"SYN-NODE-SZX-GATE","to":"SYN-NODE-SIN-HUB","entryUnbound":true,"objects":[{"object":"SYN-PARCEL-146-POC-UNDECLARED","answer":"UNDECIDED"}]}`；探针格真库用例、线格式、编排、领域各一条同时红。
- **文档**：ADR-0237 决定五改写（对不上不问；说不出照问、落册全、设备只见整格；为什么说不出不停）、对已接受记录的作用里 ADR-0205 停点名单改成多两种停点；越权风险点 7 改写为「两类段上设备只见整格」，补 12「核不上的段上配假班次换得到一位汇总」；候选与反方补「说不出也停在本侧」「以揽收地点作交出节点」「只核起点（揽收进段也没有起点）」「不绑定只列阻断（会部分重开 B1：配跨区假班次时在案包裹在跨关务区域移动那一道上折成阻断，境内段上别的客户的出口包裹会被列出）」，「起讫改从进段交接取」那条的「只在对不上时不问」改准。ADR-0205、0219 指针随改。TF CONTEXT 一条改写；UC-TF-005 `AT-TF-114` 只留对不上那一支、`AT-TF-317` 补写未绑定那一格；演示脚本补记与 AT 盘点随改。

**B-B——ADR-0205 的索引注记仍把重放回显写成不收窄。** 修了（`fd31f702`）：改成「落册、信封、VE 与运营查阅不收窄；重放回显首次读数，回给设备时同样只列没得出放行的」。

**顺手**：
1. 给判法补领域层表驱动用例：见上 `departure_entry_binding_test.go`。
2. 完成记录代决 5 补一句「已由评审 B2 改口径」；评审记录一轮里「核对……只在对不上时不问」那句加了改准的括注。
3. `read_delivery_gate.go::deliveryNode` 与 `read_departure_gate.go::entryBinding`（原 `entryStops`）对 `entryHandoverFinding` 的 switch 补了显式 `case entryHandoverFound` 与 default 报错（`errEntryHandoverFindingUnknown`，落在封闭集外是实现坏了）；出发读数对核对结果的 switch 同样穷尽、default 报错。

**未采纳**：
- 以揽收地点作交出节点、只核起点：揽收事实没有网络节点，见 ADR-0237 候选与反方。
- 不绑定、只把回给执行方的收成只列阻断：会部分重开 B1，同上。
- 放宽「停点与逐对象恰有其一」：没必要，未绑定的读数问了关务、没有停点，另立一位就够，库面 CHECK 不动。

**演示链实走**（一次性库 `dcg146_walk`，`seed.sh` 灌入退 0，起本分支的 `parcel-api` 与 `parcel-dispatch`，华东场外设备、配置员令牌，合成数据，只记 `S`；走完已停进程、删库；脚本与输出在 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/0d01979a-68c3-4b61-b295-1b04bc643b8b/scratchpad/dcg146/walk/`，输出 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/0d01979a-68c3-4b61-b295-1b04bc643b8b/scratchpad/dcg146/walk/walk.txt`）：
- 两件交给自营干线、两格都声明 SHA-HUB → SZX-GATE：按本班问 `NOT_GUARDED`、没有 `objects`；配深圳→新加坡那一班问同一段：`UNDECIDED` / `SCHEDULE_NOT_SERVING_SEGMENT`。
- 交接没声明节点的段：按本班问 `NOT_GUARDED` 带 `entryUnbound`；配跨区那一班问：`UNDECIDED` 带 `entryUnbound`，一件不列。
- 凭场外揽收进段、一件被限制装载出发：问询与出发都答 `BLOCKED` 带 `entryUnbound`、没有 `objects`；库面 `BLOCKED`，两件各一格，被扣那一件依据 `RESTRICTION_IN_FORCE`、带关务判断标识，明细 `"entryUnbound": true`；派发一拍之后全程追踪给被扣那一件形成发作期 `DEPARTURE_GATE_BLOCKED` / `SYN-VE-DEPARTURE-GATE-V1`。
- 交接只报交出节点、没报去向、一件被限制：问询 `BLOCKED` 带 `entryUnbound`。
- 本班出发（已绑定）：201、`NOT_GUARDED`。

**验证**（实测，只记 `S`）：
- `go build ./...`、`go vet ./...` 通过，`gofmt -l internal cmd migrations` 干净；变基后五笔代码提交（`fdd85c5a`、`05862bbd`、`6431ac80`、`92045fe0`、`cd62da18`）各自在 `git archive` 出的干净目录里 `go build ./... && go vet ./...` 通过。
- `scripts/test-full.sh ./internal/transportfulfillment/... ./cmd/parcel-api/... ./cmd/parcel-dispatch/... ./cmd/parcel-transport-register/... ./internal/visibilityexception/... ./internal/architecture/... ./migrations/...`：32 包全 ok、0 FAIL（输出存 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/0d01979a-68c3-4b61-b295-1b04bc643b8b/scratchpad/dcg146/test-run-review2.txt`）。
- 变基到 `4384a85a`：只在 AT 盘点上有冲突，两边都留；合并时 146 那段追记留下了两份旧稿，已在 `df8d5bca` 去掉，表行没有重号。
- 无迁移；无被权限检查拦下的仓库改动。
