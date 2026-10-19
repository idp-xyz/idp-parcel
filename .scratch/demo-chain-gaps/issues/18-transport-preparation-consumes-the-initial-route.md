# 18 运输准备结果与初始路由消费者（01·三）

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg18（此前 ready-for-agent——2026-10-19 Claude Code 会话立，用户「你作为业务和系统专家自决……解决所有的问题」授权；代决见 [tf-chain-plan](../tf-chain-plan.md)）。
Blocked by: [16](./16-transport-schedule-templates-and-concrete-schedules-are-registered.md)；软依赖 [17](./17-capacity-pools-are-registered.md)（没有池时答 `容量待确认`，不挡落地）
地盘：TF：新 `domain/transport_opportunity.go`；新 `application/prepare_opportunity_on_planned_legs.go`；新 `adapters/networkrouting/planned_leg_source.go`；新 `adapters/parcelshipment/declared_demand_source.go`；新包 `adapters/inbox/`；`postgres` 新 `transport_opportunity.go` + `review_catalogue.go`；迁移 TF 0028；`cmd/parcel-dispatch/assemble.go`(+_test)。
出处：[01](./01-transport-opportunity-has-no-production-entry.md)；方案「01 运输机会与班次」D01-1、D01-5、D01-6、D01-7、D01-8、D01-10、D01-11，「跨票通则」X3、X4、X6，「需要的 ADR」ADR-0201。

## 做什么

- TF 消费 `network-routing.initial-route.formed`：路由表改成 `FanOut(VE, TF)`，各自 inbox、各列未决哨兵（方案 D01-1）。
- 候选与择优只按方案 D01-8 的那一种内置形；不预占，不建占位班次。
- 空册与无候选分格答，`未配置` 入账不重投（方案 D01-5、X6）。
- 收寄前需求量经 TF 消费侧适配器取 PS 申报测量并标来源，维度与换算照方案 D01-6；新端口另立文件（方案 X4）。
- 结果唯一键、重放与冲突照方案 D01-10；不入队准备结果信封（方案 X3）。
- 改路后的新计划不在本票，完成记录登这个缺口，指向 [34](./34-network-routing-replanned-route-envelope-and-tf-preparation-consumer.md)（方案 D01-7）。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 无路线判断零写入。
- `未配置`、`无当前运输机会`、`容量待确认`、`当前机会不可用`、`已准备` 各有真库测试。
- 重放答 `已有结果`。
- TF 未决重投时 VE inbox 不重复投影。
- 路由测试改成两路。
- 完成记录登改路缺口（→[34](./34-network-routing-replanned-route-envelope-and-tf-preparation-consumer.md)）。
- 演示：提交一单后，`registry=transport-preparation` 列出三段，至少一段 `运输机会已准备`；窗口对不上就调 SYN 模板时刻，不改匹配规则。

## 规模

L

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg18`，基于 `38916b00`）

### 接票时核实

票面成立：初始路由信封在路由表上只投 VE（`veInitialRouteRouted`，注释写着「应消费方还有 NO/TF，但两侧消费者今天不存在」）；TF 没有准备结果的对象、库与消费者；票 16 留下了 `ScheduleCandidatesHandler`（候选只算不存）与 `ports.ScheduleCandidateSource`，票 17 留下了逐维池与周期安排的容量声明。NR 计划段的窗口依据写作 `CONNECTION/<代码>@<版本>`（NR `plannedLeg`），起讫是节点代码。PS 申报读口 `DeclaredMeasurementView` 以 main 上的形状为准：封闭五格，已采用版本格带那一版自己的测量（可缺席）。

### 代决（用户授权「你作为业务和系统专家自决，解决所有的问题」）

耐久的几条写进 [ADR-0201](../../../docs/adr/0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 文末「补记：票 18 落地时的补充裁定」（正文不改），这里列名与票内细节：

1. **比对的读法**：「比对只在 `adapters/networkrouting` 里做」读作「NR 拼写怎样读成本上下文的方向只在那里」——适配器把窗口依据里唯一那一格 `CONNECTION/` 去掉版本得连接代码，起讫取段的起讫节点，造成与班次同一个 `ScheduleDirection` 类型；领域里的候选条件是同类值相比。见补记首条。
2. **窗口依据说不清连接就响亮失败**：一格不指或指两格，答 `ErrUntranslatableAnswer`（不进哨兵名单），不折成`未配置`、不按节点对猜（`AT-TF-135`）。既有用例 `initial_route_projection_test.go` 的夹具窗口依据随之改成指名连接。
3. **规则版本 `TF-PREPARE/1`**：覆盖候选条件、择优、需求量取法与单位表；改一条就换串。
4. **单位表**（答票 17 补记风险点 20）：质量 `G/KG/T/LB/OZ`、体积 `MM3/CM3/L/M3/IN3/FT3`、计数 `PCS`，定义值换算，大写原样比、不折大小写（与计价侧同一纪律）；认不出或量纲不符答`单位不可换算` → `容量待确认`。
5. **需求量三维与锚**：重量照抄、体积三边乘积（单位写作长度单位加 `3`）、件数恒一件（来源是对象本身，只配件数维池）；锚 `baseline` 或已采用版本标识（与计价侧同词）。PS 答未定 / 未申报 / 无三格：不带锚、只剩件数，不答未决——重投不会让客户补报。
6. **依据摘要只含依据**：段方向、窗口与需求量快照；结论与候选不进。同依据重投答`已有结果`、不重判；客户更正了申报再来答`结果冲突`、原结果不覆盖。
7. **两种候选各用各的池**：登记册上的班次按登记册上的池（含预占，判断时点取当时的时钟）；只评估时物化的那一班按周期安排当前版声明物化出的空池，只算不存。为此 `ScheduleCandidatesResult` 多交一口 `Entries()`（来历与物化出的池），`Candidates()` 不变。
8. **候选口只问出发落在段窗口内的**：早于窗口出发的不进候选列表，淘汰原因里那一格只在登记册上那一班改了时刻时出现；常见的淘汰是晚于窗口到达与起讫不符。
9. **一封信封各段同一笔事务**：任一段未决整封回滚重投；已答的段重投答`已有结果`。消费入账的格：各段都有答案（含`未配置`、`已有结果`、`结果冲突`）与无路线零写入。
10. **无路线也交编排**：适配器不在翻译层跳过，`PlannedRouteNotFormed` 交编排、编排答零写入。
11. **对象身份**：载运对象引用取 NR 判断键上的声明包裹，拼写不改。
12. **端口另立文件**（方案 X4）：`ports/transport_preparation.go`（申报需求量、按班次读池、准备结果库）；`ports.go` 未动。查阅册加一法 `ListTransportPreparations`，随之补了 `cmd/parcel-api/unwired_orchestration.go` 与 tfhttp 两个替身的占位。
13. **新包 `adapters/inbox`（`tfinbox`）**：消费者名 `transport-fulfillment/prepare-opportunity-from-initial-route`，事件类型串由本包自己写出；与 VE 认的串相等由派发路由表用例一并钉住。
14. **哨兵名单**：只列 `ErrInitialRouteNotVisible` 与 `ErrPreparationUndecided`；记录对不上键、译不过去、编排结果集外都不进。
15. **FanOut 次序**：先 VE 后 TF——准备读三本册子、可能未决，反序会把追踪投影堵在班次册后面。
16. **演示班期**（租户取值，`S`）：首段窗口随提交时刻在上海当地 04:00–22:00 之间移动、长十二小时，按日一班 22:00 历时 600 分钟只接得住一小段；另登两份上海→深圳快线（`SYN-SCHED-SHA-SZX-EXPRESS-1200`、`-2000`，当地 12:00、20:00，历时 240 分钟，重量 3000 KG、件数 600 PCS），三班合起来覆盖首段窗口的全部落点。只加文件、`seed.sh` 只追加一节，既有三份周期安排不动（票 32 的真库用例读着第一份的 22:00）。
17. **AT 号**：`AT-TF-126` 至 `AT-TF-135`（协调方预分配号段）。
18. **TF CONTEXT 不动**：文档令牌次序里没有本票，准备结果的规则已由 ADR-0201 决定七与 UC 验收行承担；UC-TF-003 只在验收表末尾追加行。
19. **新立票 44、45**（预分配号段）：44 节点实测后的再准备（决定八「另立票」），45 管理台运输准备结果区（前端切片）。

### 越权风险点

ADR-0201 补记续编的六条：22 单位表封闭且区分大小写、23 选中的班次可以未登记、24 件数恒为一件、25 窗口依据的形状由 NR 定（NR owner 会签）、26 补登班次不重备已有结果、27 演示快线是为覆盖提交时刻凑的班期。本票另有：

1. **事件放大**（TF owner；NR owner 会签消费边）：正文风险点 1 原样成立——每包裹每段一次准备，每段读一次申报、两本册子与池册。
2. **PS 读口歧义答未决**（PS owner）：同一包裹被多份已接受委托声明时 PS 返 error，本路答未决重投，直到放弃；与 VE 客户视图那一路对歧义的处置同形。
3. **准备结果不发信封**（TF / NR owner）：正文风险点 15 原样成立。
4. **演示发行方**（演示 owner）：本机常驻的集成客户端发行方起得早于首方客户端入册，不认 `syn-customer-client-01`；本票实走另起了一只同配置发行方（同一个 `iss`，换端口），`parcel-api` 只把首方族的公钥集地址指过去。常驻那只要按当前 `integration-dex.yaml` 重启才认得首方客户端——没有替别的会话重启它。

### 改路缺口（→ 票 [34](./34-network-routing-replanned-route-envelope-and-tf-preparation-consumer.md)）

本票只消费初始路由已形成。改路形成的新计划没有信封可接，TF 也不轮询 NR 计划目录（方案 D01-7）：改路之后的新段没有准备结果，原段上的准备结果照旧在册（段引用是「版本#序位」，新版本是新引用，不会被误认）。票 34 由 NR 出复评新计划已形成信封、TF 照 ADR-0049 先登路由，复用 `PrepareOnPlannedLegsHandler`。

### 做了什么

- 迁移 `migrations/transport_fulfillment/0028_transport_preparation.sql`：`transport_preparation`，主键（租户、段引用、对象、规则版本），只插不改；结论五值、来源封闭、只有已准备带选中班次、未配置不带候选、两列 jsonb 快照为数组等 CHECK；按（租户、判断时刻）建查阅索引。
- 领域：新 `domain/transport_opportunity.go`（`PreparationRuleVersion`、`PreparationOutcome`、`CandidateVerdict`、`PoolVerdict`、`PreparationLeg`、`PreparationCandidate`、`PoolAssessment`、`TransportPreparation`、`PrepareTransportOpportunity`、`RehydrateTransportPreparation`）；新 `domain/transport_demand.go`（`DemandSource`、`DemandQuantity`、`DemandSnapshot`、`DeclaredDemand`、单位表与换算）；新 `domain/transport_preparation_canonicalization.go`（`CanonicalizePreparationInput`，不进规范化令牌那个文件）。
- 端口：新 `ports/transport_preparation.go`；`catalogue_read.go` 加准备结果行形与一法。
- 应用：新 `application/prepare_opportunity_on_planned_legs.go`（`PrepareOnPlannedLegsHandler`）；`schedule_candidates.go` 加 `ScheduleCandidate` 与 `Entries()`。
- postgres：新 `adapters/postgres/transport_opportunity.go`（`TransportPreparations`；`CapacityPools.ListPoolsOnSchedules`）；`review_catalogue.go` 加 `ListTransportPreparations`。
- 消费：新包 `adapters/inbox`（`InitialRouteFormedConsumer`）；新 `adapters/networkrouting/planned_leg_source.go`（`PrepareOnInitialRoute`）；新 `adapters/parcelshipment/declared_demand_source.go`（`DeclaredDemand`）；两包头注改成按符号点名各缝。
- tfhttp：`query_transport_fulfillment_records.go` 加 `registry=transport-preparation`（答 `TRANSPORT_PREPARATIONS_LISTED`）。
- cmd：新 `cmd/parcel-dispatch/assemble_transport_preparation.go`（装配与哨兵名单）；`assemble.go` 初始路由一行改挂 `initialRouteFan`，相关三处注释改口；`cmd/parcel-api/unwired_orchestration.go` 补一法。
- 种子：`data/transport/05-…-express-1200.json`、`06-…-express-2000.json`；`seed.sh` 追加一节；README 组成表加一行、数据故事第 8 条追加准备结果的读法。
- 文档：ADR-0201 文末补记；UC-TF-003 验收表追加 `AT-TF-126` 至 `AT-TF-135`；演示脚本第 5 步追加一则补记。

### 用例

- 领域 `transport_opportunity_test.go`：申报译需求量（重量、体积乘积含小数外廓、一件；只报重量；什么都没报；坏数与缺单位拒，`AT-TF-132`）；空册`未配置`（`AT-TF-103`）；四种淘汰原因与排序（`AT-TF-128`）；择优跳过说不准与不够的班、同刻按标识、未登记的那一班可选、单位换算、预占后的可用量、不改池（`AT-TF-129`）；没有池 / 无需求量 / 小写单位 / 量纲不符都`容量待确认`（`AT-TF-130`）；每班一维不够`当前机会不可用`、磅换算六位小数（`AT-TF-131`）；重建门拒四种坏行（`AT-TF-133`）；依据摘要只随依据变。
- 应用 `prepare_opportunity_on_planned_legs_test.go`：受理门五格零写入；申报、班次册、结果库三口读不动各答未决并点名；并发撞键读回赢家答已有结果。
- 真库 `adapters/postgres/transport_opportunity_test.go`：五格各落一行（未配置、无当前运输机会、容量待确认、预占后`当前机会不可用`、按登记册上的池`已准备`），读回依据；重放五段都`已有结果`；改申报后五段都`结果冲突`、原结果不覆盖；查阅册转写、他租户不可见；申报读不动与无路线都零写入；无事务拒写。
- 消费门 `adapters/inbox/initial_route_formed_consumer_test.go`（真库）：只处理一次、毒丸入账、处理失败回滚重投、认不得的类型报错。
- 适配器：`networkrouting/planned_leg_source_test.go`（三段译法、`+` 连着的依据、代码里带 `@`；无路线交编排；读不回 / 读失败 / 两支都带 / 三种说不清连接各落各的哨兵，`AT-TF-135`）；`parcelshipment/declared_demand_source_test.go`（五格加清空的版本六例、读不动原样上抛、集外取值）。
- tfhttp：准备结果册逐格转写；空册空数组、读失败 500、未配置 403 三族加上新册名。
- 派发（生产 `wireDispatcher`、生产 NR 交接口入队、真库）：`TestAFormedInitialRouteReachesTheConsumerThroughTheRouteTable` 改成两路各一行 inbox 并钉两串事件类型相等（`AT-TF-126`）；新 `transport_preparation_on_initial_route_test.go`：三段逐段作答（已准备 / 未配置 / 容量待确认，`AT-TF-126`）、无路线零写入（`AT-TF-127`）、TF 未决（库里坏时区）时 VE 照常投影入账、班次册恢复后重投 VE 不重复投影而 TF 形成结果（`AT-TF-134`）。

### 验证

- `go build ./...`、`go vet ./...`、改动的 Go 文件 `gofmt -l` 干净。
- `IDP_PARCEL_POSTGRES_DSN=…55432/postgres go test -p 1 -count=1`：`./internal/transportfulfillment/...`、`./cmd/parcel-dispatch/`、`./cmd/parcel-api/`、`./cmd/parcel-transport-register/`、`./internal/architecture/`（接线棘轮、类型可达性、写口无事务证据——首跑红在 `TransportPreparations.Save` 缺负向用例，补后绿）、`./internal/platform/...`、`./migrations/`、`./scripts/demo-seeds/...` 全 ok。没跑全量，没重生成机制清点。
- 演示（一次性库 `dcg18_demo`，证据只记 `S`）：`seed.sh` 灌入退 0（末段两行 `TEMPLATE_REGISTERED（SYN-SCHED-SHA-SZX-EXPRESS-1200 v1）`、`…-2000 v1`）。起 `parcel-api`（`127.0.0.1:19418`，操作者族接常驻 Dex，首方族 `iss` 同 `http://127.0.0.1:5558/dex`、公钥集指另起的同配置发行方 `127.0.0.1:5568`，隔离读写两开关 `SYN-TENANT-01`，权威串 `SYN-AUTH/idp-parcel-pilot`）与 `parcel-dispatch`（`NETWORK_SERVICE`，间隔 2s）。14:27:14Z（上海当地 22:27）`submit-one-shipment.sh` 答已提交 `SHR-AORMRKVQZF6ILZWD2VXAHJK5FM`；派发把它推到已接受、初始路由，初始路由信封 `PUBLISHED`，`network-routing/initial-route-on-acceptance`、`visibility-exception/derive-projection-from-initial-route`、`transport-fulfillment/prepare-opportunity-from-initial-route` 三个 inbox 名下各一行 `PROCESSED`，派发日志里没有初始路由的失败行。查阅员令牌 `GET /transport-fulfillment-records?registry=transport-preparation` 答 200 `TRANSPORT_PREPARATIONS_LISTED` 三行：`RPV-000000000001#1`（`SYN-CONN-SHA-SZX`，窗口 2026-10-19T20:00Z–10-20T08:00Z）`OPPORTUNITY_PREPARED`，选中 `SYN-SCHED-SHA-SZX-EXPRESS-1200/2026-10-20`（候选 version 0、`CAPACITY_SUFFICIENT`）；`#2`（`SYN-CONN-SZX-SIN`，10-20T16:00Z–22:00Z）与 `#3`（`SYN-CONN-SIN-LM`，10-21T04:00Z–06:00Z）`NO_CURRENT_OPPORTUNITY`、无候选。需求量三段同为 `1.50 KG`、`3000 CM3`、`1 PCS`，锚 `baseline`。走完已停两进程、删另起的发行方容器、`DROP DATABASE … WITH (FORCE)`。

### 偏离与待裁

- **地盘外动了的文件**：`application/schedule_candidates.go`（加 `Entries()`，票 16 的文件，车道 A 内）；`ports/catalogue_read.go`、`adapters/http/query_transport_fulfillment_records.go`(+test)（车道 A 独占，查阅册新增一册必经）；`cmd/parcel-api/unwired_orchestration.go`（接口加法的占位）；`cmd/parcel-dispatch/initial_route_projection_test.go`（夹具窗口依据，见代决 2）；`adapters/networkrouting/delivery_window_source.go`、`adapters/parcelshipment/delivery_place_source.go`（只改包头注）。都没碰 9b 与 operator-channel/48 的地盘。
- **派发令牌热点**：`assemble.go` 改了路由表上已有的初始路由那一行（挂的值换成 FanOut），这是票面要求，不是追加；其余是新增行与三处注释改口。`origin/main` 自本票基点之后在同一文件路由表末尾追加了一行（NO 外部件号声明撤回），与本票改的那一行不相邻，rebase 应无冲突。
- **待 owner 裁**：ADR-0201 补记风险点 25 待 NR owner 会签；单位表要不要折大小写（风险点 22）、演示快线班期（风险点 27）待 TF owner 与演示 owner。

