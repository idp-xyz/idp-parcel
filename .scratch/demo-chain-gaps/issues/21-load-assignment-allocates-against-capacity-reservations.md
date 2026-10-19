# 21 装载分配的容量关系（03b）

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg21（2026-10-19 Claude Code 作者子代理，见「完成记录」）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [19](./19-load-assignment-checks-the-schedule-is-registered.md)、[20](./20-capacity-reservation-and-release-decision-faces.md)
地盘：`domain/load_assignment_movement.go`、`load_assignment_rehydration.go`、`transport_schedule_capacity.go`；`ports/load_assignment.go`（不改 `ports.go` 签名）；`postgres/load_assignment_registry.go`、`capacity_pool.go`；迁移 TF 0031；tfhttp `form_load_assignment.go`；`assemble_segment_operations.go`；UC-TF-004 AT。
出处：[03](./03-load-assignment-does-not-check-the-schedule.md)；方案「03 装载分配与容量」D03-1、D03-3、D03-4、D03-5，「需要的 ADR」ADR-0203。

## 做什么

- 保障判据：班次登记了容量池，装载分配就须对每个池引用一份当前有效预占并声明分配量；没登池的班次不要求（方案 D03-3）。
- 预占上新增分配关系，守恒、归还与「不调 `Consume`」照方案 D03-4；池所在班次须等于分配所在班次。
- 分配量逐池逐维度声明并带来源，成员数的用法照方案 D03-5。
- UC-TF-004 补 AT 行；新格名照方案「需要的 ADR」ADR-0203。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 三个新格：`CAPACITY_RESERVATION_REQUIRED`、`RESERVATION_NOT_ON_SCHEDULE`、`ALLOCATION_EXCEEDED`（带余量）。
- 撤回或改版归还，守恒有真库测试。
- 不调 `Consume`。
- 无池班次照旧形成。
- 演示：带预占引用的分配答 201，超量答 `ALLOCATION_EXCEEDED`。

## 规模

L

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg21`）

### 接票时核实

票面成立：装载分配登记口在重放、链头、班次在册（票 19）之后直接落库，不看班次挂没挂池；预占上没有分配关系，`AvailableAt`、守恒只算预占、释放、消耗；`Consume` 在 cmd 下零调用；领域 `ReviseMembers`、`Withdraw` 没有入口（tf-unwired-seven/05 文末）。票 17、20 的池与预占、票 19 的班次在册判定都已在 main。

### 代决（用户授权「你作为业务和系统专家自决……解决所有的问题」）

方案与票面没裁、本票必须定的各格，耐久的写进 [ADR-0203](../../../docs/adr/0203-capacity-assurance-requires-a-reservation-on-every-pool-of-the-schedule-and-reservations-are-operation-decisions.md) 文末「补记：票 21 落地时的补充裁定」，正文不改；这里只列要点。

1. **分配关系存在装载分配的版本上，预占读时派生**：分配量一版一池一行、只插不改（TF 迁移 0031 `load_assignment_allocation`）；读池时取各分配当前那一版（没有后继、未撤回）在本池上的那一行，作为预占上的分配关系。不在预占上另存一份：两处各存，撤回或改版时就得有人记得去改预占，漏改一次那段范围永远还不回来。撤回版不带分配量、改版带自己重新声明的一组，「撤回或改版时归还」于是是当前版换了这件事本身；旧版那一组留在册上，历史不改写。
2. **一版至多一个后继**：派生「当前那一版」靠版本链是一条线；0021 只守一分配一个首版，0031 补（租户、分配、前版）上的部分唯一索引，存量有并列后继即拒绝施加。
3. **预占外键延迟到提交时核**：池按值换存是先删后插回同一组预占，立即核的外键会让每次预占、释放撞在已有分配上，随预占删又会把分配量一并删掉。
4. **守恒口径**：一行分配量对那份预占剩下的量判——预占量减全部已落的释放（不按时点）、已消耗，再减其他分配当前那一版分走的量；同一分配原有的那一份不算（新版接替它）。释放与消耗的守恒同样把已分配算进去；消耗此刻只从未分配余量里取（未接线，风险点 15）。
5. **分配时点取分配时刻**：判那份预占的有效区间，早于判断时点答 `RESERVATION_NOT_YET_EFFECTIVE`、晚于有效期答 `RESERVATION_EXPIRED`；不核预占覆盖到班次出发（风险点 13，票 56）。
6. **分格与次序**：构造门（分配行单看不成立答 `INPUT_NOT_ACCEPTED`，不读池）→ 重放 → 链头 → 班次在册（tf-face-algebra 与票 19 立的这几格不动）→ 读池并锁池行（读不动答 `LOAD_ASSIGNMENT_UNDECIDED`）→ `RESERVATION_NOT_ON_SCHEDULE`（池不在本班次、预占不在那个池上、没挂池却带了行三种并一格，风险点 16）→ `CAPACITY_RESERVATION_REQUIRED`（列缺行的池）→ 逐行按池身份 `UNIT_MISMATCH`、尚未生效、过期、`ALLOCATION_EXCEEDED`（带余量）。逐行三格复用预占口的同名词，同名同义。
7. **成员数**：一行写 `quantityFromMembers` 而不给数量，只许落在件数维的池上；维度由池身份派生，单看载荷判（落在别的维度答未受理，不读池）。得法单记（`DECLARED` / `MEMBER_COUNT`），库面 CHECK 钉成员数只在件数池上；变化版按新成员重数。
8. **一池一行**：同一个池两行答未受理，不替声明方挑一行。
9. **线格式**：载荷 `allocations` 可缺（无池班次照旧），封闭解码；答复回显这一版的分配量，各格带判出那一格所对的东西（`poolsWithoutReservation`、`allocationsNotOnSchedule`、`refusedAllocation`、`pool`、`reservation`、`availableQuantity`）。只有新形成一版用 201。
10. **预占口与释放口回显的预占加 `allocated`**：释放撞守恒时，声明方要看得见挡住它的是分配。这是地盘外的 `reserve_capacity.go`，只加一键、抽出一个共用的回显函数。
11. **装配**：`transactionalLoadAssigner` 答未决时整笔回滚再交回答复（与预占口、委托提交口同一先例）；`buildSegmentOperations` 接容量池册（只读池、锁池行，不换存）。形成分配的编排依赖里没有池册写口与消耗交接，够不着 `Consume`（结构测试钉住）。
12. **`ReviseMembers` 加分配量一参**：变化版重新声明分配量，不沿用前一版——成员变了，沿用旧分配量就是未经重评沿用旧容量结论（CONTEXT「成员……变化时……以新快照重新校验尚未完成的……分配」）。各测试调用点补 `nil`。
13. **归还的证明不等入口**：变化与撤回入口不在本票（票 55）。归还经领域 `ReviseMembers` / `Withdraw` 加生产登记册落版本，在真库与生产装配上证（模拟入口接上之后的那一笔，tf-face-algebra/02 的先例）。
14. **号**：迁移 TF 0031、AT-TF-171 至 180、立票 55、56，都取协调方分的号；本票不需要新 ADR，补记追加在 ADR-0203 文末，`docs/adr/README.md` 不动（该行已写「21、37 落决定一、二」，无部分停用）。
15. **演示证明**：用 `cmd/parcel-api` 的真库装配用例（生产端点、生产准入桥、生产装配，同一班次形），不在一次性库上经演示 Dex 实走；种子数据不动，演示登法只追加进种子 README 与演示脚本。

### 越权风险点

逐条见 ADR-0203 文末补记的越权风险点 13 至 19（分配保障随预占到期失效、变化与撤回没有入口、消耗与分配没接通、三种指错并一格、存量分配不回补、分配量来源不回读、查阅口看不到分配关系）。另有本票作法上的几条：

1. **读池多一条查询**（TF owner）：`CapacityPools.FindByKey` 在锁池行之后另读一遍分配关系，准备评估（票 18）逐池读池时同样多这一条；分配行按（租户、池、预占）建了索引。
2. **池的重建门更严**（TF owner）：已分配 + 已释放 + 已消耗超过预占量的池读不回（答未决）。任何生产路径都造不出这种行；绕过领域直写分配行或改版行才造得出，那时该池上的预占、释放、分配一并停在未决，直到有人修数据。
3. **地盘外动到的文件**：tfhttp `reserve_capacity.go`（回显加 `allocated`）；`ports/load_assignment.go` 只改头注；测试侧 `postgres/schedule_template_test.go`（夹具多存池与 DB 两格）、`storable_values_regression_test.go`、tfhttp `segment_operations_test.go`、`cmd/parcel-api/load_assignment_single_chain_test.go`（随签名或新依赖必须跟）。热点文件只追加：`scripts/demo-seeds/README.md`、`docs/design/synthetic-demo-journey-script.md`、UC-TF-004 AT 表、ADR-0203 文末。没碰 peer 地盘、`endpoints.go`、`main.go`、`cmd/parcel-dispatch/assemble.go`、`seed.sh` 与 `apps/admin-web`。

### 做了什么

- 领域：新 `domain/capacity_allocation.go`（`CapacityAllocationBasis` 封闭两词与 `ParseCapacityAllocationBasis`、`CapacityAllocationDeclaration`、`CapacityAllocation`、`ReservationAllocation`、`CapacityAllocationSnapshot`、`CapacityPoolReference.Dimension`、`CapacityPool.AllocationHeadroom`、`AdmitAllocation`，哨兵 `ErrAllocationExceeded`）；`load_assignment_movement.go`（spec 与聚合带分配量，`ReviseMembers` 加参，`Withdraw` 清空）；`load_assignment_rehydration.go`（`RehydrateCapacityAllocationSpec`，成员数重数、撤回版不带分配量）；`transport_schedule_capacity.go`（预占带分配关系，快照与重建门，守恒算已分配）。
- 编排：`application/form_load_assignment.go` 加 `Pools` 依赖、`Allocations` 命令格、六个新格与 `assureCapacity`。
- postgres：`load_assignment_registry.go` 读写分配行、撤回版带分配量即拒；`capacity_pool.go` 派生分配关系；迁移 `migrations/transport_fulfillment/0031_load_assignment_capacity_allocation.sql`。
- tfhttp：`form_load_assignment.go` 载荷与答复；`reserve_capacity.go` 回显 `allocated`。
- cmd：`assemble_segment_operations.go` 接池册、未决回滚。
- 文档：UC-TF-004 补 `AT-TF-171` 至 `AT-TF-180`；ADR-0203 文末补记；演示脚本第 3 步、种子 README「数据故事」第 8 步各追加一段；立票 [55](./55-load-assignment-revision-and-withdrawal-entries-are-not-wired.md)、[56](./56-allocation-guarantee-lapses-when-the-reservation-expires-before-departure.md)。

### 用例

- 领域 `capacity_allocation_test.go`：一池一行、成员数只在件数池且等于成员数、各种坏行不成立（`AT-TF-177`）；改版带自己的一组、撤回版不带、前一版不动（`AT-TF-180`）；重建门核成员数、撤回版、得法词；`AdmitAllocation` 各拒绝与余量不算同一分配原有的那一份、分配不改池（`AT-TF-175`、`AT-TF-176`）；释放与消耗动不了已分配的那一段、超分的池与同一分配两条读不回。
- 编排 `form_load_assignment_capacity_test.go`：`AT-TF-171` 至 `AT-TF-179` 各一条（含编排依赖够不着 `Consume` 的结构测试、重放与链头不读池）。
- tfhttp `form_load_assignment_capacity_test.go`：201 回显分配量、重投回显同一组；`ALLOCATION_EXCEEDED` 带余量与含已分配的预占；`CAPACITY_RESERVATION_REQUIRED`、`RESERVATION_NOT_ON_SCHEDULE` 的回显键；分配行带越过键 400、`quantityFromMembers` 逐字段翻译、无池班次不带 `allocations` 照旧 201。
- 真库 `postgres/load_assignment_allocation_test.go`：分配量随版本往返、预占取当前那一版、超出余量带余量、已分配的那段释放不出而未分配的释放得出且池换存不丢分配行、改版取新量、撤回归零后释放得出、各版分配行都留着；成员数得法往返；CHECK、延迟外键与「一版至多一个后继」。
- 真库 `cmd/parcel-api/assemble_load_assignment_capacity_test.go`：经生产端点与准入桥，预占 1200 KG 后分 1000 KG 答 **201**、另分 201 KG 答 **`ALLOCATION_EXCEEDED`** 余量 200、不带分配行答 `CAPACITY_RESERVATION_REQUIRED`、释放 201 KG 撞守恒、撤回之后整份 1200 KG 释放得出（演示两格）；同一份预占上两笔并发各 700 KG，后到者卡在池行锁上、读到先到者、答 `ALLOCATION_EXCEEDED` 余量 500；池读不回答 200 未决不答 500、库上零版本。

### 验证

- `go build ./...`；`go vet ./internal/transportfulfillment/... ./cmd/parcel-api/ ./migrations/ ./internal/architecture/`；`gofmt -l internal cmd migrations scripts` 空。
- `IDP_PARCEL_POSTGRES_DSN=…55432… go test -p 1 -count=1`：`./internal/architecture/`、`./migrations/`、`./internal/transportfulfillment/...`、`./cmd/parcel-api/` 全 ok（真库用例实跑，`-v` 核过不是跳过）；`./scripts/demo-seeds/...` 无测试文件。每笔代码提交另在暂存区检出的干净副本上 `go build` 与 `go vet` 过。
- 未跑全量；未重生成机制清点（进 main 时统一做）；种子数据未改，未灌一次性库。

### 偏离与待裁

- **演示未在一次性库经演示 Dex 实走**：票面允许以真库装配用例证明，取了后者。
- **确认订舱（票 37）**要用同一判据：容量保障此刻写在 `FormLoadAssignmentHandler.assureCapacity` 里，37 与票 55 的变化口接线时宜把它提成共用的一段，不另立口径。
- **票 46**（逐份预占查阅册）落地时应带已分配与分配关系（ADR-0203 补记风险点 19），本票没有改它的票面。
- **方案 D03-5 说分配量来源可引用票 18 的需求快照**：来源种类收得下，但与预占同样不回读（ADR-0203 风险点 3，票 47）。
