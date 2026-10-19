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

方案与票面没裁、本票必须定的各格，只写在 [ADR-0203](../../../docs/adr/0203-capacity-assurance-requires-a-reservation-on-every-pool-of-the-schedule-and-reservations-are-operation-decisions.md) 文末「补记：票 21 落地时的补充裁定」（正文不改），这里只留索引、不复述：**分配关系存在哪**（含「一版至多一个后继」与预占外键延迟到提交时核）、**守恒怎么算**（含预占口与释放口回显 `allocated`）、**分配时点**、**分格与次序**、**成员数**（含一池一行）、**线格式**，各见补记同名一条。

下面几条只关本票的作法，不入 ADR：

1. **装配**：`transactionalLoadAssigner` 答未决时整笔回滚再交回答复（与预占口、委托提交口同一先例）；`buildSegmentOperations` 接容量池册（只读池、锁池行，不换存）。形成分配的编排依赖里没有池册写口与消耗交接，够不着 `Consume`（结构测试钉住）。
2. **`ReviseMembers` 加分配量一参**：变化版重新声明分配量，不沿用前一版——成员变了，沿用旧分配量就是未经重评沿用旧容量结论（CONTEXT「成员……变化时……以新快照重新校验尚未完成的……分配」）。各测试调用点补 `nil`。
3. **归还的证明不等入口**：变化与撤回入口不在本票（票 55）。归还经领域 `ReviseMembers` / `Withdraw` 加生产登记册落版本，在真库与生产装配上证（模拟入口接上之后的那一笔，tf-face-algebra/02 的先例）。
4. **号**：迁移 TF 0031、AT-TF-171 至 180、立票 55、56，都取协调方分的号；本票不需要新 ADR，补记追加在 ADR-0203 文末，`docs/adr/README.md` 不动（该行已写「21、37 落决定一、二」，无部分停用）。
5. **演示证明**：用 `cmd/parcel-api` 的真库装配用例（生产端点、生产准入桥、生产装配，同一班次形），不在一次性库上经演示 Dex 实走；种子数据不动，演示登法只写在演示脚本第 3 步的补记里，种子 README「数据故事」第 8 步只指过去。

### 越权风险点

逐条见 ADR-0203 文末补记的越权风险点 13 至 19（分配保障随预占到期失效（含倒填分配时刻）、变化与撤回没有入口、消耗与分配没接通、三种指错并一格、存量分配不回补（含并发建池的幻读窗口）、分配量来源不回读、查阅口看不到分配关系）。另有本票作法上的几条：

1. **读池多一条查询**（TF owner）：`CapacityPools.FindByKey` 在锁池行之后另读一遍分配关系，准备评估（票 18）逐池读池时同样多这一条；分配行按（租户、池、预占）建了索引。
2. **池的重建门更严**（TF owner）：已分配 + 已释放 + 已消耗超过预占量的池读不回（答未决）。任何生产路径都造不出这种行；绕过领域直写分配行或改版行才造得出，那时该池上的预占、释放、分配一并停在未决，直到有人修数据。
3. **地盘外动到的文件**：tfhttp `reserve_capacity.go`（回显加 `allocated`）；`ports/load_assignment.go` 只改头注；测试侧 `postgres/schedule_template_test.go`（夹具多存池与 DB 两格）、`storable_values_regression_test.go`、tfhttp `segment_operations_test.go`、`cmd/parcel-api/load_assignment_single_chain_test.go`（随签名或新依赖必须跟）。热点文件只追加：`scripts/demo-seeds/README.md`、`docs/design/synthetic-demo-journey-script.md`、UC-TF-004 AT 表、ADR-0203 文末。没碰 peer 地盘、`endpoints.go`、`main.go`、`cmd/parcel-dispatch/assemble.go`、`seed.sh` 与 `apps/admin-web`。

### 做了什么

- 领域：新 `domain/capacity_allocation.go`（`CapacityAllocationBasis` 封闭两词与 `ParseCapacityAllocationBasis`、`CapacityAllocationDeclaration`、`CapacityAllocation`、`ReservationAllocation`、`CapacityAllocationSnapshot`、`CapacityPoolReference.Dimension`、`CapacityPool.AllocationHeadroom`、`AdmitAllocation`，哨兵 `ErrAllocationExceeded`）；`load_assignment_movement.go`（spec 与聚合带分配量，`ReviseMembers` 加参，`Withdraw` 清空）；`load_assignment_rehydration.go`（`RehydrateCapacityAllocationSpec`，成员数重数、撤回版不带分配量）；`transport_schedule_capacity.go`（预占带分配关系，快照与重建门，守恒算已分配）。
- 编排：`application/form_load_assignment.go` 加 `Pools` 依赖、`Allocations` 命令格、六个新格与 `assureCapacity`。
- postgres：`load_assignment_registry.go` 读写分配行、撤回版带分配量即拒；`capacity_pool.go` 派生分配关系；迁移 `migrations/transport_fulfillment/0031_load_assignment_capacity_allocation.sql`。
- tfhttp：`form_load_assignment.go` 载荷与答复；`reserve_capacity.go` 回显 `allocated`。
- cmd：`assemble_segment_operations.go` 接池册、未决回滚。
- 文档：UC-TF-004 补 `AT-TF-171` 至 `AT-TF-180`；ADR-0203 文末补记；演示脚本第 3 步追加演示登法，种子 README「数据故事」第 8 步追加一段指过去；立票 [55](./55-load-assignment-revision-and-withdrawal-entries-are-not-wired.md)、[56](./56-allocation-guarantee-lapses-when-the-reservation-expires-before-departure.md)。

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

## 评审记录（2026-10-19，隔离评审一轮，作者子代理修订）

阻断 0 项。非阻断 5 项：标「进 main 前修」的 1 项已修；其余 4 项修 3 项（都只动文档），登记去向 1 项。修订两笔：代码注释 `c9fafeb0`，文档 `c9b7e457`。

### 已修

1. **登记册 `Save` 头注只列了两道唯一**（进 main 前修）：0031 的「一版至多一个后继」部分唯一索引撞上时同样折成`已登记`，头注漏了它。改成不靠枚举的说法：版本行撞表上任何一道唯一都折成这一格，再举主键、0021 首版唯一、0031「一版至多一个后继」为例；首句同时补上「与分配量」，分配行与成员行一样挂在版本行下面。
2. **倒填分配时刻能对已过期的预占形成分配**：ADR-0203 补记越权风险点 13 后面补了这一子情形——分配时刻由提交方给（ADR-0023），倒填进一份早已过期的预占的有效区间照样形成分配，而那份预占的剩余早已还给池。票 56「要裁的」也加了一条，并写明它的修法 1（有效期不早于出发）挡得住出发未到的那种，挡不住出发也已过去、预占曾覆盖到出发的那种；要不要拿登记时刻再核一次、怎么与 ADR-0023 并存，交 TF owner 一并裁。
3. **票 55 照搬 `assureCapacity` 会按首版分配时刻判**：票 55「最小的诚实修法」注明，提成共用判定时分配时点要作参数传入，形成取分配时刻，变化取 `revisedAt`；并写明原因：分配时刻整条链上不变（0009），照搬时，预占在首版之后才立、或在首版之后已过期，结论都会反过来。本票不先改签名：此刻只有形成一个调用方，参数要等变化口接线时才有第二个取值。
4. **演示登法与代决两处复述**：演示登法只留在演示脚本第 3 步的票 21 补记里（线格式、各格答复与状态码都在那里），种子 README「数据故事」第 8 步只留一句「班次挂了池就要带预占」，载荷与答复指过去，种子不登分配与证据级别照旧写明。完成记录「代决」一节改成索引：ADR-0203 补记已有的各条（分配关系存在哪、守恒、分配时点、分格与次序、成员数、线格式）不再复述，只按条名指过去；装配、`ReviseMembers` 加参、归还证明、号、演示证明这五条只关本票的作法，留在票面。

### 登记去向，不在本票改

5. **并发建池的幻读窗口**：分配只锁已在册的池行。班次此刻一个池也没有、或少一个池时，另一笔事务同时在这一班上建池，分配可以不带新池那一行就形成，结果与 0031 之前的存量分配同一个状态。已补进 ADR-0203 补记越权风险点 17，去处交 TF owner 定。本票不堵：要堵就得让建池与分配在班次那一行上互斥，建池口（票 17）也要跟着改锁序，超出本票地盘；生产上建池是配置动作，与同一班次上的分配并发的概率低。没有另立新票：协调方给本票的立票号段 55、56 已用完，又不许自取最大号的下一号，所以登在 ADR 风险点上，交 owner 决定要不要立票。

### 修订后验证

- `go build ./...`；`go vet ./internal/transportfulfillment/... ./internal/architecture/`；`gofmt -l internal cmd migrations scripts` 空。
- `IDP_PARCEL_POSTGRES_DSN=…55432… go test -p 1 -count=1`：`./internal/transportfulfillment/...`、`./internal/architecture/`、`./cmd/parcel-api/` 全部 ok；`./scripts/demo-seeds/...` 没有测试文件。种子数据没改，`seed.sh` 没有重灌。
- `git merge-tree` 对当前 `origin/main`（`0c56e842`）无冲突。
