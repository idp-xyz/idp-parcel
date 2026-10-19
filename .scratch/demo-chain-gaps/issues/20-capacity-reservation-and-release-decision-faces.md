# 20 容量预占与释放运营决定面

Category: enhancement
Status: resolved——2026-10-19 进 main（全量钉 `edfff8fe`：150 ok / 0 FAIL；TF 链第三波同批，见文末「进 main 记录」）。此前 in-progress——隔离评审已修（见文末评审记录），待进 main，分支 claude-dcg20（此前 ready-for-agent——2026-10-19 Claude Code 会话立，用户「你作为业务和系统专家自决……解决所有的问题」授权；代决见 [tf-chain-plan](../tf-chain-plan.md)）。
Blocked by: [17](./17-capacity-pools-are-registered.md)
地盘：`internal/accessidentity/operation_decision.go`(+_test)；tfhttp `operator_decision_intake.go`(+test)、新 `reserve_capacity.go`/`release_capacity.go`；`application` 的 Reserve/Release 带需求来源；新 `cmd/parcel-api/assemble_capacity_reservation.go` + `endpoints.go` 行；种子 access 02/03、governance 新区间；ADR-0203、TF CONTEXT 一句。
出处：[03](./03-load-assignment-does-not-check-the-schedule.md)；方案「03 装载分配与容量」D03-1、D03-3、D03-6，「跨票通则」X2，「需要的 ADR」ADR-0203。

## 做什么

- 预占与释放各是一种运营决定种类，授予、准入映射与权威区间演示用种子登 `SYN-`（方案 D03-6、X2）。
- 预占必须声明需求量与来源；触硬上限的答法照方案 D03-6。
- 起草 ADR-0203（要点见方案「需要的 ADR」）；TF CONTEXT Rules 补保障判据（方案 D03-3、「随票同步的文档」）。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 两种决定种类。
- 预占必须带需求量与来源。
- `CAPACITY_EXCEEDED` 带余量，没有审批越过（AT-TF-031）。
- 重放幂等。
- 真库测试。
- 演示：配置员对 SYN 池预占答 201。

## 规模

M

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg20`）

### 接票时核实

票面成立：`PrepareTransportOpportunityHandler` 的 `Reserve`、`Release` 在 cmd 下零调用，没有决定种类、没有入口；预占只有三量与有效期，不记需求来源与判断时点；释放没有身份，重交一遍再还一份；余量按判断时点那一刻算，而预占没有起点；池按值换存、读池不锁，两笔并发预占后换存者删掉先到者（真库用例在去掉行锁时复现：后到者答 `CAPACITY_RESERVED`，先到者的预占被删）。

### 代决（用户授权「你作为业务和系统专家自决，解决所有的问题」）

方案与票面没裁、本票必须定的各格，耐久的写进 [ADR-0203](../../../docs/adr/0203-capacity-assurance-requires-a-reservation-on-every-pool-of-the-schedule-and-reservations-are-operation-decisions.md) 决定三至六，这里只列名与票内细节：

1. **需求量三格**：数量、单位、来源。来源种类封闭两种：`OPERATOR_DECLARATION`（运营声明的一版）与 `PREPARATION_DEMAND_SNAPSHOT`（准备结果的需求快照），取方案 D03-5 给分配量定的两种来源，预占与分配同一组词。引用原样保存、不回读（去处票 [47](./47-capacity-demand-from-a-preparation-snapshot-is-not-read-back.md)）。释放也带来源，说为什么不再需要这些量（票面「Reserve/Release 带需求来源」）。
2. **单位须等于池的单位**：按字面比，不换算、不归一大小写，不符答新格 `UNIT_MISMATCH` 并交回池。单位要声明，不从池上默认。
3. **硬上限按预占整段判**：预占从判断时点起占到有效期满，释放从它的时点起归还；新预占在 [判断时点, 有效期] 每一刻都不得超上限，取整段最紧那一刻判（领域 `CapacityPool.AvailableThrough`），`CAPACITY_EXCEEDED` 报的余量就是那一刻的。没取服务端时钟：ADR-0023 业务时间由提交方给，按整段判已经不需要时钟。
4. **释放有身份**：释放引用在池内唯一（库面主键同），同引用同预占同内容答 `EXISTING_RELEASE`、不再还一份；异内容或挪到另一份预占答 `RELEASE_CONFLICT`。预占重放改为整份比对（数量、单位、来源、判断时点、有效期），此前只比数量。
5. **新格与改名**：`POOL_NOT_FOUND`、`RESERVATION_NOT_FOUND`、`UNIT_MISMATCH`、`EXISTING_RELEASE`、`RELEASE_CONFLICT`，以及评审时补的 `RESERVATION_NOT_YET_EFFECTIVE`；冲突、单位不符、超出、尚未生效、过期、守恒撞线各格都交回池（与那一份预占）。容量编排`未受理`的线上名 `SOURCE_NOT_ACCEPTED` 改为 `INPUT_NOT_ACCEPTED`，与 TF 其余运营决定口、登记口同名；容量池登记口随之改名（取证于 `38916b00`：钉这个词的只有容量池登记口的 tfhttp 用例，管理台与 CLI 都不消费）。
6. **并发**：读池一律 `FOR UPDATE`（事务里锁到结束，事务外随语句放）。先写过「只在写事务里加锁」，要先问一次写执行器，`FindByKey` 就被 PBC-08 门禁认成无事务即拒的写口，改成一律加锁。没走乐观版本列：要改 `ports.go` 的容量池端口签名（方案 X4）。
7. **次序**：载荷构造门（不读池）→ 读池 → 重放与冲突 → （释放）预占在册 → 单位 → 上限，或有效区间（尚未生效、过期）与守恒 → 落池。重放判在单位与上限之前，池满时原样重交仍答已有。
8. **迁移 TF 0030**：取协调方分的号。预占表有行即拒绝施加（此前没有生产写入口）；预占加来源两列与判断时点、有效期须晚于判断时点；新 `capacity_release` 子表，释放引用池内唯一、随预占删。`released` 列保留给查阅册求和，它与明细之和相等由领域重建门核（库面不钉跨表）。
9. **access_identity 0012**：决定种类 CHECK 以 main 上 0011 为底加本票两种，另照 0011 的先例预列 ADR-0202 已定名、票 24 加的 `BOOKING_REQUEST_SUBMISSION`（迁移不按编号先后施加）。
10. **端点**：`POST /transport-fulfillment-capacity-reservations`、`POST /transport-fulfillment-capacity-releases`，挂 `operatorDecisions.transport`；载荷封闭解码，没有越过上限的键，带 `approval`、`override` 一类键答 400。只有新预占、新释放 201。
11. **装配**：`cmd/parcel-api/assemble_capacity_reservation.go` 一只编排两参（端点表各收一参，装配测试才盖得住错接）；每次决定一笔事务，编排答未决时整笔回滚再交回答复（委托提交口的先例）；不接班次册与消耗交接，交出去的类型上没有 `Consume`。占位 `unwiredCapacityDecisions` 放在本票测试文件里（发生项登记口的先例），没动 `unwired_orchestration.go`。
12. **不核班次在册、不入队**：池建立时已核班次，池身份由班次派生；预占与释放没有消费者（方案 X3）。
13. **有效期已在过去的预占不拒**：它此刻什么也不占；倒填的释放会提前归还（ADR-0203 风险点 6）。
14. **AT 号**：`AT-TF-136` 至 `AT-TF-144` 落在 UC-TF-004（步 5 在这一用例）；`AT-TF-031` 在 UC-TF-003 不动，用例点名它。
15. **演示**：种子不预登预占；配置员两种授予、两条准入对照，governance 取分配号 28、29。README「数据故事」第 8 步补记演示登法。
16. **ADR-0203 的分配关系那一半**（决定一、二）写成决定、标明由票 21（装载分配）、37（确认订舱）落；新格名照方案「需要的 ADR」。对已接受 ADR 无作用，不加前向指针。

### 越权风险点

见 ADR-0203 文末越权风险点（无池即无保护、声明值、来源不回读、软超配归 psb/09、粒度、判断时点由声明方给、读池一律加锁、逐份预占无查阅册与池行时刻、种类粒度、缩容后余量可为负、预占不记成员范围、消耗不核硬上限；后两条与池行时刻是评审时补的）。本票另有：

1. **容量池登记口`未受理`改名**（TF owner、前端 owner）：`SOURCE_NOT_ACCEPTED` → `INPUT_NOT_ACCEPTED`。仓内没有别的消费者；仓外若有人按旧词判，会认不出。
2. **读锁对同波票 18 的影响**（TF owner）：准备评估若在事务里经 `FindByKey` 读池，会与并发的预占在池行上串行；只读评估不该长时间持锁。
3. **预列 `BOOKING_REQUEST_SUBMISSION`**（access-identity owner）：多列的词此刻 Go 侧未定义，库面只守「不是封闭集外的串」。

### 做了什么

- 领域：新 `domain/capacity_demand.go`（`CapacityDemandSourceKind` 封闭两词、`CapacityDemandSourceReference`、`CapacityDemandSource`、`CapacityDemand`、`CapacityReleaseReference`，哨兵 `ErrInvalidCapacityDemand`、`ErrCapacityUnitMismatch`、`ErrDuplicateRelease`）；`transport_schedule_capacity.go` 预占加单位、来源、判断时点与释放明细，`CapacityRelease`，`Declares` 整份比对，占用按判断时点起算、释放按时点归还，`AvailableThrough`，`Reserve`、`Release` 改收需求量，`ReleaseFor`，快照与重建门带来源、判断时点与释放明细并核和。
- 编排：`prepare_transport_opportunity.go` 的 `Reserve`、`Release` 带需求来源与新次序、新格、`未受理`改名，`OpportunityResult.Reservation`、`Release`；`loadPool` 只受理池引用，不在册答 `POOL_NOT_FOUND`。
- postgres：`capacity_pool.go` 读写来源、判断时点与释放明细，读池 `FOR UPDATE`；迁移 `migrations/transport_fulfillment/0030_capacity_reservation_demand_and_release.sql`。
- access-identity：`operation_decision.go` 追加两种；迁移 `migrations/access_identity/0012_capacity_reservation_and_release_decisions.sql`。
- tfhttp：新 `reserve_capacity.go`、`release_capacity.go`；`operator_decision_intake.go` 追加两个决定常量与接口断言。
- cmd：新 `cmd/parcel-api/assemble_capacity_reservation.go`；`endpoints.go`、`main.go` 只追加。
- 种子：`data/access/02`、`03` 各追加两行，新 `data/governance/28-…capacity-reservation.json`、`29-…capacity-release.json`，`seed.sh` 追加两行，README 追加两段。
- 文档：起草 ADR-0203，`docs/adr/README.md` 在 0202 之后追加一行；TF CONTEXT「运输委托、订舱、容量与装载」补保障判据一句；UC-TF-004 补 `AT-TF-136` 至 `AT-TF-144`。立票 [46](./46-capacity-reservations-have-no-per-reservation-review-registry.md)、[47](./47-capacity-demand-from-a-preparation-snapshot-is-not-read-back.md)。

### 用例

- 领域 `capacity_demand_test.go`：来源种类封闭、需求量构造门；单位须等于池的单位（预占与释放）；硬上限按整段判、判断时点之后才起占的预占也算（`AT-TF-031`）；释放身份池内唯一、按时点归还、早于判断时点拒；`Declares` 整份比对且按登记册精度；重建门核释放明细之和、释放引用不重、缺来源或判断时点拒。`transport_schedule_capacity_test.go`、`capacity_declaration_test.go`、`registry_zero_gates_test.go` 换新签名，判零补判断时点与释放时点两门。
- 编排 `capacity_reservation_decision_test.go`：`AT-TF-136` 至 `AT-TF-142` 各一条（构造门不读池、池与预占不在册、单位不符、整段最紧余量且命令上没有越过格、预占重放与冲突各格、释放重放不再还、守恒撞线与过期交回预占）。`prepare_transport_opportunity_test.go` 换新命令，查无预占改答 `RESERVATION_NOT_FOUND`。
- 真库 `adapters/postgres/capacity_reservation_registry_test.go`：预占与释放往返来源、判断时点与明细，重放不多落，查阅册三个和照实，释放前后起算的余量不同（`AT-TF-140`、`AT-TF-141`）；迁移 0030 有行即拒；库面 CHECK 挡坏行、释放随预占删。
- tfhttp `reserve_capacity_test.go`：两口各格状态码与逐字段回显，带越过键与自报租户 400，未出示令牌 401，载荷逐字段翻译；`operator_decision_intake_test.go` 两口各按自己那一种认证；TF 认证适配器映射两种。
- access-identity 真库：两种授予过得了 CHECK、各自往返，只持预占的不持释放。
- `cmd/parcel-api/assemble_capacity_reservation_test.go`（真库）：经生产装配预占、重交、释放、释放重交、超量带余量；读不回的池答未决不答 500；同池并发两笔预占，后到者等在池行锁上、答容量超出余量 2000，库上只有先到者（`AT-TF-143`）。去掉行锁时这条用例红（实测）。端点探针表与运营决定换口表各加两口（`AT-TF-144` 由换口表的「只缺这一种授予答 403」用例覆盖）。

### 验证

- `go build ./...`；`go vet` 覆盖 `./internal/transportfulfillment/...`、`./internal/accessidentity/...`、`./cmd/parcel-api/`、`./cmd/parcel-transport-register/`、`./internal/architecture/`、`./migrations/`、`./scripts/demo-seeds/...`；`gofmt -l` 干净。
- `IDP_PARCEL_POSTGRES_DSN=…55432… go test -p 1 -count=1`：`./internal/transportfulfillment/...`、`./internal/accessidentity/...`、`./cmd/parcel-api/`、`./cmd/parcel-transport-register/`、`./cmd/parcel-access-register/`、`./cmd/parcel-governance-register/`、`./internal/architecture/`（接线棘轮、PBC-08、前缀与枚举门禁）、`./migrations/`、`./scripts/demo-seeds/...` 全部 ok。没跑全量，机制清点未重生成。
- 一次性库 `dcg20_seed` 上 `seed.sh` 灌入退 0（两条授予、两条对照 `RECORDED`）。其上起 `parcel-api`（`127.0.0.1:19172`，操作者渠道接演示 Dex，带自身权威串），配置员令牌对 `SYN-SCHED-SZX-SIN-DAILY/2026-10-21/WEIGHT` 预占 1200 KG 答 **201** `CAPACITY_RESERVED`、原样重交 200 `EXISTING_RESERVATION`；另一份 3801 KG 答 200 `CAPACITY_EXCEEDED`、`availableQuantity` 3800；单位写 `T` 答 200 `UNIT_MISMATCH`；指向没登过的 `…/2026-10-22/WEIGHT` 答 200 `POOL_NOT_FOUND`；带 `approval` 键答 400；释放 200 KG 答 201、重交 200 `EXISTING_RELEASE`；查阅册重量池 `reserved` 1200、`released` 200；查阅员预占答 403 `OPERATOR_NOT_GRANTED`。证据只记 `S`。走完已停进程、`DROP DATABASE … WITH (FORCE)`。
- 与本地 `main`（`a2ff42f6`）、`claude-dcg18`、`claude-dcg29`、`claude-dcg33` 的 `merge-tree` 无冲突；与 `claude-dcg24` 冲突在装配令牌、决定种类令牌与种子令牌的热点文件（`endpoints.go`、`endpoints_test.go`、`main.go`、`operation_decision.go`(+test)、TF `operator_decision_intake.go`(+test)、认证适配器测试、UC-TF-004、种子 JSON、`seed.sh`、README），都是两边各追加行，按令牌次序 20 先、24 后合。

### 偏离与待裁

- **票 24 的 access_identity 0013 须并入本票两种**：它以 0011 为底加 `BOOKING_REQUEST_SUBMISSION`，没列 `CAPACITY_RESERVATION`、`CAPACITY_RELEASE`；新库按编号施加时 0013 在 0012 之后，会把这两种删出 CHECK，种子登授予即失败。合批时 0013 的全集要取 0012 的并集。
- **地盘外动到的文件**（都随签名或改名必须跟，或只追加）：`postgres/capacity_pool.go`、`domain/transport_schedule_capacity.go`（车道 A 内票 21 的地盘，20 → 21 串行）、`postgres/journey_schedule_capacity_test.go`、`review_catalogue_test.go`、tfhttp `register_capacity_pool_test.go`（改名）、`domain/registry_zero_gates_test.go`、`capacity_declaration_test.go`、TF 认证适配器测试、`cmd/parcel-api/endpoints_test.go`、`assemble_operator_decisions_test.go`（各追加两行）。没碰 peer 地盘与 `apps/admin-web`。
- **main 上既有的漏检**（不在本票改，交协调方）：`cmd/parcel-api/main.go` 里 `capacityPoolRegistration, err := buildCapacityPoolRegistration(db)` 之后没有 `if err != nil`，紧接的委托装配把 `err` 盖掉，建池装配失败会被吞（看着是 17 与 23 合批时丢的）。热点文件只追加，本票没在既有行之间插行。
- **AT 覆盖盘点**未改，进 main 时统一重盘（`AT-TF-136` 至 `144`）。
- **票 21 的接缝**：分配关系挂在预占上（决定二），预占的快照、重建门与 `capacity_reservation` 表都已带来源与判断时点；21 加分配关系时照「迁移不按编号先后施加」再看一次 0030 的拒绝门。
- **演示动线文档** `docs/design/synthetic-demo-journey-script.md` 没补预占一步：动线终态里预占随装载分配出现（票 21），本票只在种子 README 写登法。

## 评审记录（2026-10-19，隔离评审一轮；作者子代理修订）

阻断 0 项。非阻断 12 项，其中建议进 main 前修的 4 项全修；其余 8 项修 3 项，5 项登去处（协调方或另票）。

### 建议进 main 前修的

1. **释放时点早于预占判断时点落成`未受理`**（编排读过池、答复不带预占，与 ADR-0203 决定六「未受理不读池」对不上）——修，取「单立一格」：领域哨兵 `ErrReservationNotYetEffective`，编排新格 `RESERVATION_NOT_YET_EFFECTIVE`，与过期对称、交回那一份预占（判断时点、有效期与三量在上面）。ADR-0203 决定六的释放次序与「各成一格」那条补上，并写明`未受理`只说载荷单看不成立、不读池；UC-TF-004 `AT-TF-142` 扩成三格；编排 `TestReleaseRefusalsEchoTheReservation`、tfhttp `TestCapacityReleaseEndpointAnswersEachGrade` 各补一格。没取「答未受理并交回预占」：那样`未受理`时而读池、时而带预占，决定六给它的定义就不成立；单位不符与它同类（载荷单看成立，要对着在册对象才判得出），本票已单立，这一格照办。
2. **预占不记成员范围，ADR 没交代**——修：ADR-0203 Context 的 CONTEXT 引文补全「成员范围」；决定三加一条，说明范围由需求来源固定、装载分配时逐成员落到分配关系上（决定二）；越权风险点加 11「预占不记成员范围」。没在预占上加范围格，理由见下「代决」。
3. **`establishedPoolRecord` 的文档注释被 `poolDemand` 夹走**——修，挪回函数头上。
4. **`OpportunityOutcome` 注释写成变更叙述**——修，改为陈述规则，并把新格一并写进去。

### 其余非阻断

5. **构造门用截过精度的时刻、交领域的却是原值**——修：`Reserve`、`Release` 把截过的判断时点、有效期、释放时点交给领域与 `AvailableThrough`；新用例 `TestCapacityDecisionsEchoInstantsAtRegistryPrecision` 证首答回显已截（去掉修法时实测红）。
6. **释放读回次序的注释说「与领域追加的次序一致」不成立**——修：`loadReleases` 与领域 `Releases()` 的注释改为「次序不带语义，归还按各次自己的时点算」。没改成领域按时点插入：没有逻辑依赖次序，不为注释改行为。
7. **`Replace` 改写 `capacity_pool.recorded_at` 在生产上生效**——本票不改：改它牵动池登记口回显与编排 `replacePool` 两处，逐份时刻的去处又在票 46。并入[票 46](./46-capacity-reservations-have-no-per-reservation-review-registry.md)「要裁的」（倾向保持建池时刻，交 TF owner），ADR-0203 越权风险点 8 补一句。
8. **`Consume` 不核硬上限，ADR 决定四没声明例外**——修文档：ADR-0203 决定四声明这条只在预占与释放之间成立，越权风险点加 12「消耗不核硬上限」；领域 `Consume` 头注写明接线时要按消耗之后的整段重核。不改代码：消耗仍不接线（方案 D03-7）。
9. **票 24 的 access_identity 0013 会删掉本票两种决定**——交协调方，维持上文「偏离与待裁」第一条：合批时 0013 全集取 0012 的并集，合后在新库上重跑一遍 `seed.sh`。
10. **`cmd/parcel-api/main.go` 里 `buildCapacityPoolRegistration` 之后缺 err 检查**（main 既有）——交协调方在热点文件令牌内补 `if err != nil { return err }`；本票守「热点文件只追加」，不在既有行之间插行。
11. **读池 `FOR UPDATE` 与票 18 `ListPoolsOnSchedules` 串行**——ADR-0203 越权风险点 7 写明票 18 合入时的两条路（不在长事务里读池，或接受串行）；票 18 rebase 时在其票面落定。
12. **AT 覆盖盘点与机制清点没重生成**——进 main 时统一重盘（`AT-TF-136` 至 `AT-TF-144`）、重生成。

### 代决（评审修订，用户授权「你作为业务和系统专家自决……解决所有的问题」）

- **释放早于判断时点单立一格**，线上名 `RESERVATION_NOT_YET_EFFECTIVE`，中文`预占尚未生效`：与 `RESERVATION_EXPIRED` 各是有效区间 [判断时点, 有效期] 的一头，续办是按那一份预占的有效区间重述释放时点。`Consume` 共用同一换算，消耗早于判断时点也答这一格（消耗未接线）。
- **成员范围不在预占上另成一格**：预占先于装载分配，占的时候成员多半未定；加格要改迁移 0030 与两口载荷，而来源不回读之前范围也核不了。

### 越权风险点（评审修订新增）

1. **新线上词 `RESERVATION_NOT_YET_EFFECTIVE`**（TF owner、前端 owner）：此前这一情形答 `INPUT_NOT_ACCEPTED`；本票未进 main，仓内外都没有按旧答法判的消费者。
2. **预占不记成员范围**（TF owner）：见 ADR-0203 越权风险点 11。若 owner 要预占自带范围，须加一格并改迁移与两口载荷。

### 修订后验证

- `go build ./...`；`go vet` 覆盖 `./internal/transportfulfillment/...`、`./cmd/parcel-api/`、`./internal/architecture/`；`gofmt -l` 干净。
- `IDP_PARCEL_POSTGRES_DSN=…55432… go test -p 1 -count=1`：`./internal/transportfulfillment/...`、`./internal/accessidentity/...`、`./cmd/parcel-api/`、`./cmd/parcel-transport-register/`、`./internal/architecture/`、`./migrations/` 全部 ok；`-v` 核过真库容量用例实际执行。种子未改，没重灌；没跑全量，机制清点未重生成。
- `merge-tree`：与本地 `main`（`a2ff42f6`）、`claude-dcg33` 干净；与 `claude-dcg18`（`fcc46fa5`）、`claude-dcg29`（`ed993ac8`）冲突只在热点文件 `scripts/demo-seeds/README.md`，两边各追加行——修订前的 `21b1b211` 对这两个分支头已是如此，是它们完成记录之后加的行，不是本轮修订引入；按令牌次序合。

## 进 main 记录（2026-10-19）

- TF 链第三波 18、20、24、29、33 合成一批，按 20 → 24 → 18 → 33 → 29 叠到 main 上，整批快进推送，与本记录同一次推送。
- 合批时的处理：
  - README 的 ADR 行按号合并。
  - 决定种类：20 加两种、24 加一种，枚举、测试与认证映射都两边保留；access_identity 迁移 0013 的 CHECK 全集含 0012 那两种（评审阻断，已在 24 的修订里补）。
  - `endpoints.go`、`endpoints_test.go`、`main.go` 与演示种子 JSON、`seed.sh` 的追加行两边都留：`main.go` 补回被冲突吃掉的错误检查，种子 JSON 补对象分隔。
  - 18 与 20 的接缝：20 把预占改为带需求量（量、单位、来源），18 的择优领域用例与准备结果真库用例随之改。全量第一次在 `90202334` 上红在后者（`UNIT_MISMATCH`），修后复跑。
  - 24 的两份权威区间同号 30，外部事实那份改为 31。
  - UC-TF-004 验收表去掉合并时插进的空行，免得断成两张表。
- 合批种子在一次性新库上 `seed.sh` 灌通，退 0，用完删库。
- 全量 `go test -p 1 -count=1 ./...`（带 55432 DSN）在干净游离检出上钉 `edfff8fe`：150 包 ok，0 FAIL，0 SKIP。机制清点在合批时重生成（`90202334`），之后只改了一份测试文件。
