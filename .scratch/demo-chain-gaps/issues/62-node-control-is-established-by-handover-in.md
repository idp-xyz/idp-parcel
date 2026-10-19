# 62 节点控制按权威运输交接转入 HANDOVER_IN（UC-NO-005 下半）

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg62（基于 `origin/main` `ea61d108`）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: 无（[59](./59-handover-declares-releasing-receiving-and-onward-nodes.md)、[61](./61-node-control-transfers-out-on-an-authoritative-handover.md) 已进 main）
地盘（**需与 idp-parcel-9b 协调**）：61 新立的同一批 NO 文件（`application/transfer_control_on_handover.go`、`adapters/postgres/physical_control_transition.go`、`adapters/postgres/review_catalogue_control_transition.go`、`domain/control_transition.go`）；UC-NO-005 下半；演示脚本。
与 idp-parcel-9b 的四个共享点同 [61](./61-node-control-transfers-out-on-an-authoritative-handover.md)：`reception.go` 的折叠挂点、NO 迁移号、查阅口注册行、NO CONTEXT「节点控制」与 UC-NO-005 的措辞（详见方案「与 idp-parcel-9b 的协调点」）。
出处：[04](./04-handover-does-not-move-node-control-or-trigger-reassessment.md)；方案「代决」C5、C6，「需要的 ADR」ADR-0214。

## 做什么

- `已交接` 且声明接收节点时，在该节点立 HANDOVER_IN；对不上作业实物的入账，不造待识别实物（方案 C6）。
- 转出时声明的交出节点与在身控制节点不符，照转并记信息格（方案 C5 后半）。
- 进 main 的令牌次序见方案「波次与令牌」。

## 完成判据

- `已交接` + 接收节点：该节点上立 HANDOVER_IN；随后以该节点为交出节点的交接把它转出。
- 没有关联作业实物、未声明接收节点：入账，不立。
- 声明的交出节点不符：照转，信息格有值。
- 真库测试。
- 演示：自营段交接进 SZX-GATE（`receivingNode=SYN-NODE-SZX-GATE`）→ `registry=physical-control` 出现 HANDOVER_IN@SZX-GATE；SZX-GATE 交给干线承运商（`releasingNode=SYN-NODE-SZX-GATE`）→ 转出；SIN-HUB 接收 → HANDOVER_IN@SIN-HUB。

## 规模

M

## 完成记录

2026-10-19，作者子代理，分支 `claude-dcg62`（基于 `origin/main` `ea61d108`，59、61、63、65、69 已在其上）。授权：用户原话「你作为业务和系统专家自决……解决所有的问题」；统一代决见 [handover-dispatch-plan](../handover-dispatch-plan.md) C5 后半、C6、C7，方案与 ADR-0214 正文都没裁的细节下面逐条记「代决」，并写进 ADR-0214 文末「补记：票 62 落决定七时的补充裁定」（正文不改，Status 行加前向指针、Links 加两行）。证据只记 `S`。

### 做了什么

- **领域** `domain/control_transition.go`：
  - `JudgeTransferIn`：没有在身控制、最近一次转出不晚于这一版才在接收节点立 HANDOVER_IN，成立时刻取交接的业务时间；否则入账 `CONTROL_STILL_ACTIVE` 或 `HANDOVER_PRECEDES_RELEASE`（只属转入，只带实物）。
  - `HandoverInControlBasis`：转入立起的控制依据，`HANDOVER-IN/` 后接交接键三件的长度前缀拼接。
  - `JudgeTransferOut` 多收一格声明的交出节点（零值即没声明），推进了的转出在节点不符时带信息格 `ReleasingNodeMismatch`。
  - `PendingControlSourceRecompute` 认转入方向；读回门按方向收窄原因、核转入立起的成立时刻，「交接早于控制成立」读回放宽到不晚于。
- **端口** `ports/physical_control.go`：交接来源答复加 `ReleasingNode`、`ReceivingNode`；转移册加 `FindUnitControls`（同一把实物锁读有过的各份控制）；查阅行加 `MismatchedReleasingNode`。
- **编排**：
  - 新文件 `application/establish_control_on_handover.go`：`Advance` 读回一次、先转出后转入，两半折一个处置；`establishIn` 是转入一半。
  - `transfer_control_on_handover.go`：原 `Transfer` 改为不导出的 `transferOut`（读回挪进 `Advance`）；结果代数加 `CONTROL_ESTABLISHED_IN`、`NO_RECEIVING_NODE_DECLARED`、`ALREADY_ESTABLISHED_IN_CHAIN`；转出在锁下剔掉同链转入立起的那一份。
- **迁移 `node_operations/0018`**：信息格列与形状、转入成立时刻等于业务时间、原因全集加两格、各原因按方向收窄、转入一侧待重算须带 HANDOVER_IN 控制。
- **postgres**：转移册读写信息格；`FindActiveControls` 与新的 `FindUnitControls` 共用一个取锁读（`unitControls`）；查阅册多读一列。
- **HTTP**：`registry=physical-control` 每行多一个可缺键 `mismatchedReleasingNode`。
- **消费侧适配器** `adapters/transportfulfillment/control_transfer.go`：译出交出、接收节点；处理方改调 `Advance`，未决与缺陷的错误正文带两半各自的格。消费门、inbox 名、派发路由与哨兵名单不变，只改注释。
- **派发** `cmd/parcel-dispatch/assemble_node_control_on_handover.go`：只改头注（两半同一路、转入没有自己的未决）。`assemble.go` 不动。
- **文档**：ADR-0214 文末补记；UC-NO-005（状态、输入与结果契约、转入一半流程表、读控制与一致性、`AT-NO-113`–`125`）；NO CONTEXT「节点控制」节末追加一条；CONTEXT-MAP `node-operations ↔ transport-fulfillment` 一句里加一个分句；`docs/application/README.md` UC-NO-005 一行；开发主线 PN-04 补记；演示动线补记；AT 盘点追记；方案「后继」追加一行。
- **新立票**：[91](./91-a-handover-in-that-names-no-unit-has-no-node-reconciliation.md)（交进节点却对不上作业实物的节点对账）、[92](./92-a-node-intake-does-not-see-a-control-established-by-handover-in.md)（节点收寄不认转入立起的在身控制）。

### 代决（方案与 ADR-0214 正文没裁或与字面不同的；逐条已写进 ADR-0214 文末补记）

1. **没声明接收节点：转入零写入，不入账**。票 61 完成记录预留了「IN 专属的入账原因（如未声明接收节点）留给 62 自定」；交给运输方的交接本来没有接收节点，入账会让每次正常交出都留一行。
2. **同一版先转出、后转入**，同一笔事务、同一把可重入的实物锁；读回只读一次，两半折一个处置，转入没有自己的未决哨兵。节点之间直接交接因此一出一进。
3. **仍有在身控制时不转入**（`CONTROL_STILL_ACTIVE`），不另立第二份；**最近一次转出晚于这一版时不转入**（`HANDOVER_PRECEDES_RELEASE`），晚到的旧交接不重新立起已结束的占有。两格只带实物。
4. **转入立起的控制依据由交接键单射铸出**（`HANDOVER-IN/<长度>:<对象>/<长度>:<范围>/<长度>:<版本>`），不复用 TF 的 `TRANSPORT-HANDOVER/<版本>`：版本只在（对象，范围）内唯一，折叠与「一份控制至多转出一次」都按（实物，依据）认。铸法照 tf-face-algebra/09 的长度前缀。
5. **一次交接不转出它自己立起的控制**：后继版仍`已交接`按 ADR-0214 决定五重判转出时，剔掉同链转入立起的那一份。实现时由用例逼出——不剔会把前版刚立起的控制转出去。
6. **转入一侧的撤回控制更正**入账待重算（方向 IN），带先前立起的那一份、不带转出；立起的不撤。
7. **交出节点不符的信息格**落成一列 `mismatched_releasing_node`，只在推进了的转出上、值是声明的那一个；不比交出方（参与方），不因不符而入账。
8. **接收节点照声明立**，不核 NR 目录、不换算：NR 节点代码与本上下文收寄所在节点同一套指称。
9. **读回门「交接早于控制成立」放宽到不晚于**（票 61 评审记录留给本票的截断问题）；构造仍取严格。
10. **编排入口改名**：原导出的 `Transfer` 收为 `transferOut`，生产与用例都走 `Advance`；61 的编排用例经 `fixture.transfer` 取 `Advance(...).Out()`，断言不变。
11. **对象认法转出、转入各认一次**；对不上实物两侧各入账一行。

### 越权风险点

- ADR-0214 文末补记的越权风险点（续编号）逐条待 NO owner 复核、TF owner 会签。按标题：
  - 仍有在身控制时不转入；晚于最近一次转出不转入；
  - 转入依据由本上下文铸、不复用 TF 的转出引用；接收节点照声明立、不核；
  - 运输方交进节点时转出一侧照常入账（处置面归票 81）；
  - 收寄不认交接转入立起的控制（票 92）；对不上作业实物的转入没有节点对账（票 91）；
  - 一次交接不转出它自己立起的控制，靠的仍是同链按（对象，范围）认。
- 另两条只在本票记：
  - **存量不回补**：61 进 main 之后、本票进 main 之前登下的声明了接收节点的`已交接`，NO 那一路已入账 PROCESSED，不会再有转入。演示库与开发库按一次性库用，生产没有存量；owner 若要回补，要一个按交接册重放转入的一次性动作。
  - **查阅口新键名**：`mismatchedReleasingNode` 是本票定的线上名，管理台还没有这一册的页面。

### 用例

- `AT-NO-113`–`125` 全部点名，新立于 UC-NO-005（号段 113–125 由协调方预分，本票用满）。
- 领域（`control_transition_in_test.go`）：113、114、118、119、120、122，另有依据单射与读回门按方向收窄两条。
- 编排（`establish_control_on_handover_test.go`）：113–122 与两半折一个处置；61 的编排用例照旧全绿（经 `Advance(...).Out()`）。
- 消费侧适配器：113、120 的节点译出。
- 真库存储（`physical_control_transition_in_test.go`）：113、114、115、118、120、123 的存储面，0018 的五种坏行逐条断到 23514 与约束名，`FindUnitControls` 无事务拒（PBC-08 负向证据）。
- HTTP：123 的传输面。
- 派发真库（`node_control_in_on_handover_test.go`）：生产路由表上收寄 → 进深圳口岸 → 交给干线 → 进新加坡枢纽（113、114、115、116、125）；对不上实物不造实物（117）；撤回控制的更正两侧入账（122）；两次交进节点并发只留业务时间晚的那一份（124）。

### 验证

- `go build ./...`、`gofmt -l internal cmd migrations` 干净；`go vet` 跑过 `./internal/nodeoperations/...`、`./cmd/parcel-dispatch/`、`./cmd/parcel-api/`、`./migrations/`，干净。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1` 全 ok：`./internal/architecture/`、`./internal/nodeoperations/...`、`./migrations/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-api/`、`./cmd/parcel-frontline-import/`、`./cmd/parcel-node-operations-register/`、`./internal/parcelshipment/adapters/nodeoperations/`。未跑全量，机制清点未重生成（按指示进 main 时统一做）。
- 门禁：PBC-08 写口负向证据（`FindUnitControls`）已补；接线棘轮与类型可达性棘轮不变长；没有新信封、新分区键、新标识前缀常量。
- **演示**（一次性库 `dcg62_demo`，实测，只记 `S`）：
  - `seed.sh` 原样灌入退 0。共享的集成客户端发行方签不出首方令牌（`invalid_client`），另起一只同配置的 Dex（宿主端口 5566、`iss` 不变）；`parcel-api`（:19962）与 `parcel-dispatch` 照演示脚本「前置」起。
  - 委托 `SHR-CGO26VIPOHOUBN7QWY4C7LKOEI` 成`已接受`，包裹 `DPCL-6XOSZNL64T6LKS4AFYYVROURWE`；预报 `PREADV-3UBWBDNZPDHLWSAAV4HJ5CNRBE`；上海枢纽收寄 `201 INTAKE_FORMED`，实物 `SYN-HU-20261019T194138685097983Z`。
  - `SYN-HO-62-A/v1`（华东场外设备，交出上海、接收深圳口岸）`201`：一拍之后上海 OUT·ADVANCED、深圳口岸 IN·ADVANCED（`HANDOVER_IN`）。
  - `SYN-HO-62-B/v1`（交出深圳口岸、去向新加坡枢纽）`201`：深圳口岸那一份 OUT·ADVANCED，没有转入行。
  - `SYN-HO-62-C/v1`（新加坡尾程场外设备，接收新加坡枢纽）`201`：新加坡枢纽 IN·ADVANCED，转出一侧 NOT_ADVANCED / `NO_ACTIVE_CONTROL`。
  - `SYN-HO-62-D/v1`（新加坡枢纽交给尾程承运方，交出节点误声明为深圳口岸）`201`：新加坡枢纽那一份 OUT·ADVANCED，`mismatchedReleasingNode=SYN-NODE-SZX-GATE`。
  - `registry=reception` 上海那一行 `controlReleasedBy=TRANSPORT-HANDOVER/SYN-HO-62-A/v1`。四封交接信封都 `PUBLISHED`，NO、NR、TF 派送发起、VE 投影各自 inbox 名下都 `PROCESSED`；两个进程日志无 ERROR。
  - 走完已停进程、删容器、删库。

### 偏离与待裁

- **地盘**：只动了票面所列 61 那批 NO 文件、新文件与文档；`reception.go`、`review_catalogue.go`、`ports/catalogue_read.go`、`query_node_operations_records.go`、`cmd/parcel-dispatch/assemble.go`、`scripts/demo-seeds/**`、`docs/adr/README.md` 一字未改。票面地盘没列、本票改了的：
  - `adapters/http/query_physical_control_transitions.go`（61 新立，加一个可缺键）；
  - `adapters/transportfulfillment/control_transfer.go` 与 `adapters/inbox/transport_handover_registered_consumer.go`（61 改写或新立，前者改调 `Advance`、译节点，后者只改注释）；
  - `cmd/parcel-dispatch/assemble_node_control_on_handover.go`（61 新立，只改头注）。
- **测试助手**：派发真库用例复用 61 的 `receiveIntoControl`、`newNodeControlLane`、`controlTransitions`、`readReception` 与 TF 那边的 `newHandoverRegistration`；交出门禁读数另写一份带去向的，不改 63 的 `releaseGateReadingFor`。
- **号段**：协调方预分的全部照用，没有自取——`AT-NO-113`–`125` 用满；新立票号 91、92；迁移 `node_operations/0018`。
- **待裁**：ADR-0214 文末补记各条与上面代决 1–11，待 NO owner 复核、TF owner 会签；「运输方交进节点时转出一侧照常入账」要不要改零写入，留给 NO owner 与票 81。

