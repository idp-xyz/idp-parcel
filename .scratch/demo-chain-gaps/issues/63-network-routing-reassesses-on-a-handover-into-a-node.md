# 63 交接进入节点时路由复核

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg63（基于 claude-dcg59，见文末「完成记录」）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: [59](./59-handover-declares-releasing-receiving-and-onward-nodes.md)
地盘：NR 新 `adapters/inbox/transport_handover_consumer.go`、新 `adapters/transportfulfillment/reassess_on_handover.go`、新 `adapters/parcelshipment/accepted_carried_geo.go`；`cmd/parcel-dispatch/assemble.go` FanOut 加一路 + 新 `assemble_reassess_on_handover.go` + 新真库用例；文档 ADR-0215（ADR-0075 Links 加后继指针、`docs/adr/README.md` 状态格注明部分停用）、UC-NR-003 触发行与 AT、NR CONTEXT 一句。
出处：[04](./04-handover-does-not-move-node-control-or-trigger-reassessment.md)；方案「复核」04「票面漏掉的真阻断」，「代决」C8、C9，「需要的 ADR」ADR-0215。

## 做什么

- NR 消费交接信封，只对「`已交接` 且声明接收节点」形成复核触发；其余入账（方案 C8）。
- 所携地理投影经消费侧适配器取受理基线那一版提交的投影，不读地址本体（方案 C9）。
- 读的是 `ShipmentRequests.FindCurrentAcceptedByParcel` 与委托聚合，不碰 PS 外部标识读口。
- 文档：起草 ADR-0215。
- 进 main 的令牌次序见方案「波次与令牌」。

## 完成判据

- `已交接` + 接收节点：一条复核，控制证据 `TRANSPORT_HANDOVER_CONTROL`，位置为接收节点；所携投影摘要等于基线那一版的。
- 其余裁决、无接收节点、对象不是当前已接受包裹：入账，零复核。
- PS 读不动、复核未决：登哨兵重投。
- 同一交接重投答已有结果。
- 真库测试。
- 演示：SIN-HUB 接收交接（`receivingNode=SYN-NODE-SIN-HUB`）→ `/route-plans?registry=reassessment` 多一行 `STILL_APPLICABLE`@SYN-NODE-SIN-HUB。

## 规模

M

## 完成记录（2026-10-19，作者子代理，分支 `claude-dcg63`，基于 `claude-dcg59` 的 `aa94b0ca`）

### 做了什么

- **NR 消费门**：`adapters/inbox/transport_handover_consumer.go`，消费者名 `network-routing/reassess-on-transport-handover`，认 `transport-fulfillment.transport-handover.registered`，译四维键，缺一即毒丸。
- **交接触发适配器**：`adapters/transportfulfillment/reassess_on_handover.go`，按键读回那一版交接。只有`已交接`、声明了接收节点、对象是当前已接受包裹三者同时成立才复核：
  - 控制依据 `TRANSPORT_HANDOVER_CONTROL`，位置取接收节点，来源版本取交接结果版本，业务时间取交接判断时刻；
  - 拒收、待确认、没声明接收节点、对象不是当前已接受包裹，入账；
  - 复核结论按「重投会不会变」折成消费门两格。
- **受理基线投影读取**：`adapters/parcelshipment/accepted_carried_geo.go` 的 `AcceptedCarriedGeoSource`。
  - 读法：`FindCurrentAcceptedByParcel` 反查，`FindBySourceIdentity` 读委托聚合，取受理基线那一版提交的 `GeoProjection()`，只到地理维。
  - 基线引用取基线的提交版本标识，与初始路由、收寄复核同源。
  - 包裹不在受理基线成员里时答「不是」。
  - 不碰 PS 外部标识读口那几个文件，只调 `ShipmentRequests` 已有的两个方法。
- **派发**：
  - `cmd/parcel-dispatch/assemble.go` 交接那一行的 FanOut 末尾加 NR 一路；本路哨兵名单 `reassessOnHandoverUndecidedSentinels` 与装配在新文件 `assemble_reassess_on_handover.go`。
  - 复核编排的生产接法抽成 `reassessRouteHandler` / `reassessRouteHandlerOver`，`networkIntakeConsumer` 改调它，见「偏离与待裁」第一条。
- **文档**：
  - 起草 ADR-0215（方案 C8、C9）；ADR-0075 的 Status 行与 Links 加部分停用的前向指针；`docs/adr/README.md` 新增 0215 一行，0075 那一行只在行尾追加被停条款与适用场景。
  - NR CONTEXT 补一句；CONTEXT-MAP 的 `parcel-shipment → network-routing`、`node-operations / transport-fulfillment → network-routing` 两条边各在句尾补一句，见代决 6。
  - UC-NR-003 的触发行、输入契约两行，验收表立 `AT-NR-064`–`069`；AT 盘点点名并加追记。
  - 演示动线补记一条。
- **立票**：[84](./84-route-reassessment-register-shows-the-trigger-control-and-location.md) 复核册列出触发的控制依据、位置与来源版本。新立票号 85 未用。

### 提交

`a8a5822e` NR 消费门、交接触发适配器与受理基线投影读取 · `3382967f` 派发 FanOut 加一路、复核编排共用一份接法、真库用例 · `8774ea78` ADR-0215 与 ADR-0075 前向指针、索引 · `cda420fc` CONTEXT、上下文地图、UC-NR-003、AT 盘点 · `412c4b53` 立票 84 · `080a51c0` 演示动线补记 · 本完成记录单独一笔。

### 代决与越权风险点

方案 C8、C9 已裁的部分照做。下面是方案没裁、本票自决的细节，越权风险点逐条列出，待 NR owner 复核、PS owner 会签。ADR-0215 文末另有越权风险点九条。

1. **AT-NR 号取 064 起，不取 UC-NR-003 自己的最大号 059 之后的 060。** AT-NR 是全上下文一条序列，`AT-NR-060`–`063` 已由票 64 立在 UC-NR-001。
   - 越权风险点：协调方给的号段写的是「UC-NR-003 现有最大号之后的号」，本票按全上下文序列读成 064 起。若协调方本意是逐 UC 编号，只改 UC-NR-003 验收表、AT 盘点与测试 Covers 里的号。
2. **关联写成 `handover/` 加四维「字节长度:原串」**，不照 `intake/` 那样直拼。
   - 理由：对象、范围、版本是登记方给的串，可能含斜杠，直拼会让两次不同的交接撞成同一个关联，第二次被当成重放。
   - 越权风险点：复核册上这一族关联比另两族难读。
3. **一版交接一个触发；更正仍`已交接`时另成一次复核，更正成拒收或待确认入账不回退。** 方案 C7 只裁了 NO 侧，NR 侧本票自决。
   - 越权风险点：见 ADR-0215 越权风险点 6、7。
4. **反查歧义与已接受委托自相矛盾响亮失败**，不进未决哨兵，也不入账。
   - 理由：与 PS 采用、面单终局各路对 `ErrAmbiguousParcelTarget` 的处置一致：要人去看，重投不会变。
   - 越权风险点：同一封交接信封整封按派发预算重投后弃置；追踪投影、派送发起两路已按 inbox 入账，不受影响。
5. **两次读之间委托不再是已接受，按「PS 读不动」重投。** 场景是反查命中而聚合读回已不在已接受，或聚合读不回。
   - 理由：同一行两次读之间被并发改了，重投时反查给出一致答案。
   - 越权风险点：真是持久不一致时会烧完重投预算。
6. **CONTEXT-MAP 两条边各补一句。** 票面地盘没列 CONTEXT-MAP，但 ADR-0075 自称是 `parcel-shipment → network-routing` 边「提供路径」半句的定义处，不补就有两处说法相反。只在句尾追加，不改原句。
7. **复核册不列位置，不在本票扩列。** 加列要动 NR 迁移、复核记录、查阅册、HTTP 契约夹具与管理台类型，管理台在另一会话地盘；改为立票 84。
   - 越权风险点：演示的「@SYN-NODE-SIN-HUB」今天只能两跳对上：复核行的关联指回交接，交接册写着 `receivingNode`。真库用例另用「接收节点不在计划上则失效依据点名该节点」证位置取的是接收节点。
8. **不在复核前核接收节点是否在 NR 节点目录在册。** 与 ADR-0213「不核目录」同一立场。
   - 越权风险点：声明错了会按错误位置复核，拼错的节点会让原计划失效，见 ADR-0215 越权风险点 4。

### 用例

`AT-NR-064`–`069`（UC-NR-003）。Covers 点名分布在下列文件，逐条去向见 `.scratch/at-coverage-inventory.md`：

- `internal/networkrouting/adapters/inbox/transport_handover_consumer_test.go`
- `internal/networkrouting/adapters/parcelshipment/accepted_carried_geo_test.go`
- `internal/networkrouting/adapters/transportfulfillment/reassess_on_handover_test.go`
- `cmd/parcel-dispatch/reassess_on_handover_test.go`
- `cmd/parcel-dispatch/assemble_test.go`（路由表那一条只追加了 NR 的消费者名与一行 Covers）

完成判据逐条：

- 一条复核、控制证据与位置：适配器用例与真库用例都有；位置另由非计划节点失效、失效依据点名接收节点来证。
- 投影摘要等于基线那一版：真库用例从 PS 读回基线那一版提交，按 NR 摘要形状算期望值，与复核取证据时所携投影比。
- 其余裁决、无接收节点、对象不是当前已接受包裹，入账零复核：适配器与真库两面都有，含委托还没接受与集运单元号。
- PS 读不动、复核未决，登哨兵重投：适配器面逐格证哨兵与可重投；真库面证未决翻成 `ErrConsumerUndecided`，inbox 无痕。
- 同一交接重投答已有结果：消费门重投跳过，同一版另一封信封答已有结果，仍一行。
- 生产装配：
  - 生产路由表上 NR 一路入账；
  - 演示网络、关务目录与已接受委托都在时，交进新加坡枢纽经生产路由一拍，答仍适用（`TestAHandoverIntoSingaporeHubIsStillApplicableUnderTheDemoNetworkThroughTheProductionRoute`）。

### 验证（实测，只记 `S`）

- `go build ./...`；`go vet ./internal/networkrouting/... ./cmd/parcel-dispatch/`；`gofmt -l internal cmd`，三者干净。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1 ./internal/networkrouting/... ./cmd/parcel-dispatch/ ./internal/architecture/` 全 ok。
- 没跑全量，没重生成机制清点。
- **演示**：一次性库 `dcg63_walk`，`seed.sh` 灌入退 0。
  - 起法：首方客户发行方另起一只同配置的 Dex（宿主端口 5563，常驻那只 5558 不认首方客户端）；`parcel-api` 起在 `:19163`，`parcel-dispatch` 照「前置」七个变量。
  - 提交 `SHR-UGAXJ3TYJWTSCERUQSVWNCTM44` 成`已接受`，计划 `RPV-000000000001`。
  - 预报后上海枢纽收寄答 `201 INTAKE_FORMED`，复核册一行 `STILL_APPLICABLE`（`intake/…`）。
  - 新加坡尾程场外设备登交进新加坡枢纽的交接（`releasingNode=SYN-NODE-SZX-GATE`、`receivingNode=SYN-NODE-SIN-HUB`），答 `201`。交接信封 `PUBLISHED`，追踪投影、派送发起、路由复核三个消费者名下各一行 inbox。`GET /route-plans?register=reassessment` 多一行 `STILL_APPLICABLE`，关联 `handover/13:SYN-TENANT-01/31:DPCL-WI5ERZZWXB766F2QM42PNX5P6M/13:SYN-HO-SIN-63/12:SYN-HO-63/v1`，`reviewedPlan` 同一份计划。交接册那一版 `receivingNode` 为 `SYN-NODE-SIN-HUB`。
  - 随后一件只声明交出节点 `SYN-NODE-SIN-HUB` 的交接答 `201`，信封 `PUBLISHED`，NR 一路入账，复核册仍两行。
  - 走完已停进程、删 Dex 容器、`DROP DATABASE … WITH (FORCE)`。

### 偏离与待裁

- **`cmd/parcel-dispatch/assemble.go` 不只是追加行。** `networkIntakeConsumer` 里构造复核编排的那一段搬进新文件的 `reassessRouteHandlerOver`，原处删掉、改成一行调用。理由是「一决策一处定义」：两种触发复核同一份路由判断，证据、自动改路事实与判断库接法若两处各写一份，一路改了另一路会停在旧接法上。
  - 冲突面：删的那段只在 `networkIntakeConsumer` 函数体内，票 61 与另一会话 idp-parcel-9b 的在途改动不碰这个函数。
  - 交接那一行的 FanOut 是就地加参数，票 61 也在同一行加 NO 一路，合并时两路都留、NR 排最后即可。路由表注释只追加一行。
- **新立票号只用了 84。** 85 未用。
- **迁移、种子都没动。** NR 不需要迁移，演示种子不需要改。
- **待裁**：代决 1 的 AT 号读法、代决 7 的复核册扩列去向（票 84），待协调方或 NR owner 确认。

