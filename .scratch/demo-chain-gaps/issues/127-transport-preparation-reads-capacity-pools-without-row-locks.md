# 127 准备评估读容量池时加行锁，与预占、装载分配串行

Category: bug
Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-dcg127` tip `cad853912`（其后一笔只改 AT 盘点、票面与一句注释的措辞），见文末「完成记录」。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领，与 48、167 同批，分支 `claude-dcg127`（worktree `idp-parcel-dcg127`）。此前 ready-for-agent——2026-10-20 代决复核立（上线后尽快），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M3；ADR-0203 越权风险点 7

## 现象与修法

现象、证据与修法的完整写法以复核报告 M3 一条为准（单一权威），此处只记要点：

给准备评估另开不加锁的池读口，只供准备编排用；装载分配仍走加锁那一口。

## 完成记录（2026-10-26，Claude Code 作者子代理，会话 `tops-20` 派单，分支 `claude-dcg127`，与 48、167 同批）

起点：`origin/main` `449a25508`（含认领）。

| 提交 | 内容 |
|---|---|
| `310154eee` | 本票代码与用例；ADR-0203 文末复核记录；UC-TF-003 `AT-TF-331` |
| `cad853912` | 票 48、167（与本票无交叉文件） |
| 完成记录那一笔 | AT 盘点①三票各行与追记；三张票的完成记录；`ListPoolsOnSchedulesUnlocked` 注释去掉一处跨文件的语句枚举 |

### 接票时核实（读码于 `449a25508`）

M3 的现象成立：准备编排 `PrepareOnPlannedLegsHandler.candidateInputs` 经 `CapacityPools.ListPoolsOnSchedules` 读池，后者逐池调 `FindByKey`，那一句带 `FOR UPDATE`；准备跑在初始路由信封的 inbox 事务里（`tfinbox.InitialRouteFormedConsumer`），锁持到整封提交。另核得一格：装载分配（`FormLoadAssignmentHandler.assureCapacity`）也走 `ListPoolsOnSchedules`，它要这把锁（ADR-0203「补记：票 21 落地时的补充裁定」分格与次序一条），所以不能把那一口的锁直接去掉，只能照报告另开一口。

### 做了什么

- 端口 `ports/transport_preparation.go`：新 `UnlockedCapacityPoolsOnSchedules`（`ListPoolsOnSchedulesUnlocked`），注释写明只给准备评估、为何另立一口而不加参数；`CapacityPoolsOnSchedules` 的注释改成装载分配那一口。
- 编排 `application/prepare_opportunity_on_planned_legs.go`：`PrepareOnPlannedLegsDeps.Pools` 改收不锁那一口，`candidateInputs` 改调它。
- 存储 `adapters/postgres/capacity_pool.go`：`findPool(ctx, key, lock)` 是按池身份读回的唯一读法，`FindByKey` 调它带锁；`transport_opportunity.go`：`listPoolsOnSchedules(…, lock)`，`ListPoolsOnSchedules` 带锁、`ListPoolsOnSchedulesUnlocked` 不带。两口读法、重建门、池身份核对同一条，只差池行那一句的 `FOR UPDATE`。
- 装配：`cmd/parcel-dispatch/assemble_transport_preparation.go` 不变——同一只 `tfpostgres.CapacityPools` 两口都实现，类型决定准备拿到哪一口。
- 文档：ADR-0203 文末加「复核记录（2026-10-26…）」一条，写越权风险点 7 的去向（正文、Decision、Status 行不动）；UC-TF-003 立 `AT-TF-331`；AT 盘点①加行、文末追记（总览计数未动）。ADR-0201 的票 18 补记未改：没走接受串行那一支。

### 代决与越权风险点（用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」）

1. **不接受串行，照 M3 另开不锁的读口**（机制）。读了 ADR-0203 决定四与越权风险点 7、ADR-0201「补记：票 18 落地时的补充裁定」里「两种候选各用各的池」「一段未决整次重投」两条。owner 若要串行：把 `PrepareOnPlannedLegsDeps.Pools` 换回 `ports.CapacityPoolsOnSchedules`，并照报告在 ADR-0201 票 18 补记写明吞吐代价。
2. **半截的池**（机制）。不锁的读一个池分几条语句读，读已提交下各取各的快照；两条之间恰有一笔释放提交时，预占上的已释放与读到的释放明细对不上，重建门拒读，准备答未决 `CAPACITY_POOLS_UNAVAILABLE`、整封重投。没有把不锁那一口拼成一条语句换一致快照：那是第二条读法，要与写编排的读法一起维护。代价是罕见的一次重投。owner 若要一次读成，去处是给不锁那一口单写一条聚合语句。
3. **准备读到的是读那一刻已提交的池**（机制）。在途的预占不等准备、准备也不等它；准备不预占（ADR-0201 决定六），结果里的可用量本是判断时点的依据。
4. **决定四不算被收窄**。ADR-0203 决定四「读池在写事务里锁住池那一行」写的是预占的写编排（「判余量与换存同一笔」），预占、释放、装载分配三口照旧锁；越权风险点 7 的「一律加行锁」是风险描述，不是条款。所以只补复核记录，没有在 ADR-0203 的 Status 行挂停用指针。评审若认定是收窄，改法是在 ADR-0203 Status 行索引注记挂来向——但来向方是票不是 ADR，现行写法没有先例，届时要问协调方。

### 修法 → 用例

| 修法（报告 M3） | 用例 |
|---|---|
| 给准备评估另开一个不加锁的池读口，只供 `PrepareOnPlannedLegsHandler` 使用 | 类型：`PrepareOnPlannedLegsDeps.Pools` 收 `ports.UnlockedCapacityPoolsOnSchedules`；真库 `adapters/postgres/transport_preparation_pool_lock_test.go::TestPreparationHoldsNoPoolRowLockWhileItsTransactionIsOpen`：准备那一笔开着时，另一笔事务按池身份锁读（预占、释放那一口）与按班次锁读（装载分配那一口）都在 300ms 锁等待时限内读完；准备确实按登记册上的两池判出`运输机会已准备` |
| 装载分配仍走加锁的那一口 | 同一用例的对照：握着 `ListPoolsOnSchedules` 时同一探针答 55P03——探针抓得到锁，上一格才算数 |
| （不锁带来的新格）读到半截的池 | `::TestAPreparationThatReadsAPoolHalfwayThroughAReleaseAnswersUndecided`：库里摆出半截（已释放 1、释放明细没有），准备答未决池册读不动、零写入 |

改前核对：把 `ListPoolsOnSchedulesUnlocked` 临时改成带锁，`TestPreparationHoldsNoPoolRowLockWhileItsTransactionIsOpen` 红（`find capacity pool: … lock timeout (SQLSTATE 55P03)`），改回绿。

### 自验

于 `cad853912`（其后一笔只改盘点、票面与一句注释的措辞，改后 `go build`、`gofmt` 复核过）：`go build ./...`、`go vet ./...` 退 0，`gofmt -l internal cmd migrations tools` 无输出；带 DSN（`127.0.0.1:55432`，pgtest 每例自建自删库）`go test -count=1 -p 1` 跑 `./internal/transportfulfillment/...`、`go list` 反查出依赖 TF 的 `./cmd/parcel-api/...`、`./cmd/parcel-dispatch/...`、`./cmd/parcel-transport-register/...`，加 `./internal/architecture/...`、`./migrations/...`，有用例的 17 个包全绿（含 PG）；本批新真库用例逐个 `-v` 确认 PASS、不是 SKIP。`310154eee` 一笔另在临时 worktree 单独 `go build ./...`、`go vet` 通过。`go run ./tools/adr-index -check` 退 0；`go run ./tools/at-coverage-counts -check` 报总览过期（计数由推送方重算）。未跑全量 `scripts/test-full.sh`，未重生成 ADR 索引、AT 计数与机制清点，未灌种子；`apps/admin-web` 未动，无迁移、无新 ADR。
