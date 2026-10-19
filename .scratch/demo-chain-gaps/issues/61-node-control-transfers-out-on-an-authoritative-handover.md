# 61 节点控制按权威运输交接转出（UC-NO-005 上半）

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg61。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: 无
地盘（**需与 idp-parcel-9b 协调**）：NO 新 `domain/control_transition.go`、新 `ports/physical_control.go`（不改 `ports.go`）、新 `application/transfer_control_on_handover.go`、新 `adapters/postgres/physical_control_transition.go` + NO 迁移（认领时现取）、新查阅册 `adapters/postgres/review_catalogue_control_transition.go`、新 `adapters/inbox/transport_handover_registered_consumer.go`；改写 `adapters/transportfulfillment/control_transfer.go`（按键读交接，去掉字符串比对）；`cmd/parcel-dispatch/assemble.go` FanOut 加一路 + 新 `assemble_node_control_on_handover.go` + 新真库用例；文档 ADR-0214、新 UC-NO-005、NO CONTEXT「节点控制」一句、CONTEXT-MAP `node-operations ↔ transport-fulfillment` 一句、开发主线「机制半边现状」PN-04 行更正、演示脚本。
与 idp-parcel-9b 的四个共享点（详见方案「与 idp-parcel-9b 的协调点」）：
1. `adapters/postgres/reception.go` 读控制处的折叠——9b 的 ps-external-mark-relations/11 在改此文件；等它进 main，或事先约定本票只加一个辅助函数调用。
2. `migrations/node_operations/` 的号——9b 在连续取号，认领时现取。
3. NO 查阅口注册新册的那一行（`adapters/postgres/review_catalogue.go` / `ports/catalogue_read.go`）——9b 的 11 也在改。
4. NO CONTEXT「节点控制」一节与 UC-NO-005——9b 在改「送站交接单」段落；本票一律写全称「权威运输交接」。
出处：[04](./04-handover-does-not-move-node-control-or-trigger-reassessment.md)；[勘察报告](../../no-control-transfer-out/report.md)；方案「代决」C2、C3、C4、C5、C7，「需要的 ADR」ADR-0214。

## 做什么

- 新立 UC-NO-005，转出与转入同一用例（方案 C2）。
- 交接对象按正式包裹身份经 NO 自己的当前关联对上作业实物；各格入账照方案 C3。
- 新立只追加的节点控制转移册；收寄行不改写，读控制时折叠（方案 C4）。
- 转出判据照方案 C5；交接更正照方案 C7。
- 文档：起草 ADR-0214（C6 由 [62](./62-node-control-is-established-by-handover-in.md) 落，ADR 里写成决定并标明后继票）；更正开发主线 PN-04 行「交接→控制转出」适配器已接通的说法。
- 进 main 的令牌次序见方案「波次与令牌」。

## 完成判据

- 收寄控制在身、交接 `已交接`：收寄链头 `controlReleasedBy` 有值；重投幂等。
- 拒收、待确认：零写入。
- 零件、多件、集运单元对象：各一格入账，不推进。
- 撤回控制的更正：入账「控制来源链待重算」，查阅口可见。
- 同一实物并发只落一行。
- 转出之后同节点再送站，不再并入旧收寄。
- 真库测试。
- 演示：SHA 收寄 → 交接带 `releasingNode=SYN-NODE-SHA-HUB`（[59](./59-handover-declares-releasing-receiving-and-onward-nodes.md) 未落时不带）→ 派发一拍；`/node-operations-records?registry=reception` 该行 `controlReleasedBy=TRANSPORT-HANDOVER/<版本>`；新册 `registry=physical-control` 列出这次转出。

## 规模

L

## 完成记录

2026-10-19，作者子代理，分支 `claude-dcg61`（基于 `origin/main` `de9e3ad9`）。授权：用户原话「你作为业务和系统专家自决……解决所有的问题」；统一代决见 [handover-dispatch-plan](../handover-dispatch-plan.md) C2–C7，方案没裁的细节下面逐条记「代决」。证据只记 `S`。

### 做了什么

- **迁移 `node_operations/0017`**：节点控制转移册 `physical_control_transition`。
  - 主键是（租户，交接对象，交接范围，交接版本，方向）。
  - CHECK 守各原因的形状。
  - 部分唯一索引「一份控制至多转出一次」，键是（租户，作业实物，控制依据）。
  - 更正链、转入、查阅三本索引。
  - 折叠函数 `node_operations.control_with_transfer_out`。
- **领域** `domain/control_transition.go`：
  - 交接键 `AuthoritativeHandoverReference`。
  - 方向、结果、入账原因三个封闭集。
  - `JudgeTransferOut`：恰一份在身控制、交接不早于它成立才转出。
  - `ObjectNotResolved`、`PendingControlSourceRecompute`，以及与构造同一套形状的读回门 `RehydrateControlTransition`。
- **端口** `ports/physical_control.go`，不改 `ports.go`：
  - 交接来源 `AuthoritativeHandoverSource`；
  - 对象认法 `HandoverObjectResolver`；
  - 转移册 `PhysicalControlRegister`，在身控制读前取锁；
  - 查阅行。
- **编排** `application/transfer_control_on_handover.go`：结果代数 7 格，`Disposition` 分入账、重投、缺陷三格。
- **消费侧适配器** `adapters/transportfulfillment/control_transfer.go` 改写：
  - 按四维键读回那一版，问有没有后继，译出转出引用，业务时间折到微秒；
  - 去掉字符串比对，`ControlTransferAdapter` 退场；
  - 消费门处理方 `TransferControlOnHandoverAdapter`。
- **消费门** `adapters/inbox/transport_handover_registered_consumer.go`：inbox 名 `node-operations/transfer-control-on-transport-handover`。
- **postgres**：
  - `physical_control_transition.go` 是转移册、对象认法与在身控制的读写。在身控制读前取的实物锁，与 `Receptions.FindCurrentJudgmentsAtNode` 逐字同一把。
  - `review_catalogue_control_transition.go` 是查阅册。
  - `reception.go` 的 `receptionColumns` 控制一格改经折叠读；`review_catalogue.go` 的收寄册转出两格同改。
- **HTTP**：`/node-operations-records?registry=physical-control`，答 `PHYSICAL_CONTROL_TRANSITIONS_LISTED`。`cmd/parcel-api` 给未接线占位补这一法（新文件）。
- **派发**：交接那一行的 FanOut 加第三路，次序是 VE、TF、NO；装配在新文件 `assemble_node_control_on_handover.go`，自带哨兵名单，只登 `ErrControlTransferUndecided`。
- **文档**：
  - ADR-0214；UC-NO-005（全称「按权威运输交接推进节点控制」，与送站交接单分开）；`docs/application/README.md` 一行；
  - NO CONTEXT「节点控制」追加三条；CONTEXT-MAP `node-operations ↔ transport-fulfillment` 末尾一句；
  - 开发主线 PN-04 补记（更正「交接→控制转出」已接通的说法）；演示动线补记；
  - AT 盘点追记；方案「后继」一节追加一行。
- **新立票**：[80](./80-a-consolidation-unit-handover-does-not-move-member-control.md)（集运单元交接）、[81](./81-control-source-chain-recompute-has-no-disposition-face.md)（待重算与入账各格的处置面）。

### 代决（方案没裁或与方案字面不同的）

1. **对象认法取「各条判断链的当前有效版本」上的关联**。
   - 方案 C3 写「链头，与 `ParcelContainmentView` 同一读法」，但两者在一码两物上不同：`ParcelContainmentView` 已关联那一路逐行读，冲突版接续之后原判断上的关联仍算。
   - 本票取链头：身份冲突期间那件实物对不上，入账不推进，不拿一件身份正在冲突的实物替包裹交走。
2. **不止一份在身控制：入账 `SEVERAL_ACTIVE_CONTROLS`**，不挑最新、不一并转出。
3. **交接早于在身控制成立：入账 `HANDOVER_PRECEDES_CONTROL`**，带那一份控制。不入账的话，领域 `TransferOut` 会拒，信封成毒丸。
4. **没有在身控制：入账 `NO_ACTIVE_CONTROL`**。包括场外揽收直进、从未在节点收寄的包裹，以及已转出过的实物。
5. **被同链后一版更正了的那一版晚到：零写入**（`HANDOVER_VERSION_SUPERSEDED`）。C7 没裁乱序。这样两版信封谁先到，结果都一样。
6. **同链按（对象，范围）认**。只有同链有过推进的转出时，后继仍`已交接`才零写入；同链前版只有入账行时，这一版按首版重判。认链在取实物锁之前一次、锁下再一次。
7. **主键不含作业实物**。C4 写「键为（租户、作业实物、交接键）」，但对不上实物的入账行没有实物可写。实物维放进「一份控制至多转出一次」那本部分唯一索引。
8. **折叠落在 SQL 函数里**（迁移 0017）。收寄读口只改 `receptionColumns` 那一格，查阅目录两格同改；领域重放 `TransferOut` 照旧验。
9. **转入（IN）行的形状与在身控制的读法本票先立**：表的 CHECK、在身控制认 IN 行并同样折叠。62 只需写行。IN 专属的入账原因（如未声明接收节点）留给 62 自定，届时要一份迁移改 CHECK。
10. **未决只登交接登记册读不动或读不回**。转移册与锁的错误照原样交回，落发布失败；坏键与越出封闭集是缺陷。
11. **查阅口取值**：册名 `physical-control`，outcome `PHYSICAL_CONTROL_TRANSITIONS_LISTED`，键名见 `query_physical_control_transitions.go`。
12. **演示取值**：交接对象写正式包裹号 `DPCL-…`（不是客户包裹参考）；上海枢纽交出由华东场外设备登。

### 越权风险点

- ADR-0214 文末九条，待 NO owner 复核、TF owner 会签。要点：
  - 同串按集运单元判；集运单元交接暂不推进（票 80）；
  - 对象只读链头；不止一份在身控制不推进；交接早于控制成立入账；
  - 同链按（对象，范围）认；一次转出可能掩盖漏登的中间交接；
  - 待重算没有处置面（票 81）；
  - 折叠只覆盖经 `receptionColumns` 与收寄册的读，直接读 `control` 列的新语句读不到转出。
- 另一条：查阅口的册名与键名是本票定的线上名，管理台还没有这一册的页面。

### 用例

- `AT-NO-101`–`112` 全部点名，新立于 UC-NO-005。
- 领域：101、104–107、109。
- 编排：101–109、111。
- 适配器与消费门：转出引用、后继、读不动分格、处置三动作。
- 真库存储：101、107、110 的锁面、111、112。
- HTTP：112。
- 派发真库：101、102、103、109、110、112。生产路由表那一条另把 NO 名下 inbox 行加进 `TestARegisteredTransportHandoverReachesTheConsumerThroughTheRouteTable`。

### 验证

- `go build ./...`、`gofmt -l`（internal、cmd、migrations）干净。
- `go vet` 跑过：`./internal/nodeoperations/...`、`./cmd/parcel-dispatch/`、`./cmd/parcel-api/`、`./migrations/`。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1` 全 ok，覆盖：
  - `./internal/nodeoperations/...`、`./internal/architecture/`、`./migrations/`；
  - `./cmd/parcel-dispatch/`、`./cmd/parcel-api/`、`./cmd/parcel-frontline-import/`、`./cmd/parcel-node-operations-register/`；
  - `./internal/parcelshipment/adapters/nodeoperations/`。
- 门禁两处跟改：PBC-08 写口无事务负向证据门禁要求 `PhysicalControlTransitions.Save` 有负向用例，已补；接线棘轮与类型可达性棘轮不变长。
- **演示**（一次性库 `dcg61_demo`，实测，只记 `S`）：
  - 环境：`seed.sh` 原样灌入退 0。首方客户发行方另起一只同配置的 Dex，宿主端口 5561、`iss` 不变——共享的那一只配置早于首方客户端，签不出。`parcel-api`（:19961）与 `parcel-dispatch` 照演示脚本「前置」起。
  - 提交与收寄：`submit-one-shipment.sh` 提交 `SHR-GBSPLS2KUT3PC3WIYANQ2TKRBA`，成`已接受`，包裹 `DPCL-TTJCLZMX2PZOY65K3GUUP6HTQI`；`submit-one-pre-advice.sh` 交预报 `PREADV-MJ34YUWW7VDL66ENGX2G6KYSEU`；`receive-one-delivery.sh` 在上海枢纽收寄，答 `201 INTAKE_FORMED`，实物 `SYN-HU-20261019T181842468985930Z`。
  - 交接：`SYN-DEVICE-OFFSITE-CN-EAST-01` 登交接 `SYN-HO-61/v1`，范围 `SYN-HO-SCOPE-61`，答 `201 HANDOVER_REGISTERED`。59 未落，不带 `releasingNode`。
  - 派发一拍：信封 `PUBLISHED`，VE、TF、NO 三个 inbox 名各一行 `PROCESSED`。
  - 读册：`registry=physical-control` 一行 OUT·ADVANCED，`transferReference=TRANSPORT-HANDOVER/SYN-HO-61/v1`；`registry=reception` 那一行 `controlReleasedBy=TRANSPORT-HANDOVER/SYN-HO-61/v1`、`controlReleasedAt=2026-10-19T18:18:57Z`。
  - 同一件实物再送到上海枢纽：答 `201 INTAKE_FORMED`，新控制依据 `NODE-INTAKE/INTAKEV-JH3SACG7RT7R5OJLZ7DBKV3UIY`，不再并入。
  - 走完已停进程、删容器、删库。

### 偏离与待裁

- **共享点的实际改法**（票面写「只加一次辅助函数调用」「只追加一行注册」）：
  - `reception.go`：`receptionColumns` 那一行把 `control` 换成本票文件里的常量 `foldedControl`，头注一句。
  - `review_catalogue.go`：没有加注册行，查阅册方法放在新文件里；但 `ListReceptions` 的转出两格改经折叠读——完成判据要求 `registry=reception` 带 `controlReleasedBy`，不改就读不到。
  - `ports/catalogue_read.go`：接口追加一行。
  - NO CONTEXT：只在「节点控制」节末尾追加三条。
- **票面地盘没列、本票改了的**：
  - `adapters/http/query_node_operations_records.go` 三处追加：读口接口一行、册名校验一行、switch 一个 case；
  - `cmd/parcel-api` 新文件一个；
  - `cmd/parcel-dispatch/assemble.go` 的 FanOut 那一行加了第三个参数，这是改既有行，其余为插入；
  - `assemble_test.go` 名单追加一行、头注追加一行；
  - `docs/application/README.md` 在 UC-NO-004 与 006 之间插一行。
- **与 idp-parcel-9b 的耦合**：
  - 编排用例 `TestADeliveryAfterTheTransferOutFormsANewIntake` 复用收寄编排测试的 fixture：`newReceptionFixture`、`receiveCommand`、`deliveredAgain`、`keyOf`。
  - postgres 用例复用 `ref`、`inTx`、`receivedAt`。
  - 9b 改这几个测试助手时要跟改。
  - 9b 若再往 `receptionColumns` 加列，合并时保留 `foldedControl` 那一格。
- **号段**：协调方预分的全部照用，没有自取：
  - `AT-NO-101`–`112` 用满；
  - 新立票号用 80、81；
  - 迁移 `node_operations/0017`；
  - ADR-0214。
- **未做**：机制清点未重生成，按指示进 main 时统一做；管理台转移册页面属前端切片，没立票号。
- **待裁**：ADR-0214 文末九条越权风险点，以及上面代决 1–12，待 NO owner 复核、TF owner 会签。
