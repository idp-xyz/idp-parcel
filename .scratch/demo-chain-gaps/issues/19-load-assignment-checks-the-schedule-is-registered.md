# 19 装载分配核班次在册（03a）

Category: enhancement
Status: resolved——2026-10-19 进 main（全量钉 `60f8d75c`：146 ok / 0 FAIL；TF 链第一波同批，见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-dcg19（见文末「完成记录」；隔离评审 0 阻断，已修订，见「评审记录」）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: 无（演示那一步要 [16](./16-transport-schedule-templates-and-concrete-schedules-are-registered.md) 或 [18](./18-transport-preparation-consumes-the-initial-route.md) 的在册班次；已移交票 16 的完成判据，见「评审记录」）
地盘：`application/form_load_assignment.go`(+_test)；`cmd/parcel-api/assemble_segment_operations.go`(+_test)、`load_assignment_single_chain_test.go`、`storable_values_channel_test.go`；tfhttp `form_load_assignment_test.go`、`segment_operations_test.go`；UC-TF-004 AT 行。
出处：[03](./03-load-assignment-does-not-check-the-schedule.md)；方案「03 装载分配与容量」D03-1、D03-2，「已不成立或已修」03。

## 做什么

- 装载分配在版本重放与链头判定之后按租户读班次登记，不在册与读失败分格答（方案 D03-2）。
- 落在 UC-TF-004 步 1 现行文字之内，不需要 ADR，只补 AT 行（方案 D03-2，及「需要的 ADR」里「不需要新 ADR」那段）。
- 不碰 `OpenDispatchTaskDeps`；与 peer 同函数相邻，落地前 rebase（方案「等 peer 的」）。

## 完成判据

- 新格 `SCHEDULE_NOT_FOUND`（200），放在重放与链头判定之后。
- 读失败答 `UNDECIDED`。
- 他租户引用落同一格。
- 真库测试先补登班次行。
- 不碰 `OpenDispatchTaskDeps`。
- 演示：对物化班次分配答 201，对乱填班次答 `SCHEDULE_NOT_FOUND`。——已移交票 [16](./16-transport-schedule-templates-and-concrete-schedules-are-registered.md) 的完成判据，见「评审记录」。

## 规模

S

## 完成记录（2026-10-19，作者子代理，分支 `claude-dcg19`，基于 main `47c860a5`）

没有新 ADR：照方案「03 装载分配与容量」D03-2，落在 UC-TF-004 步 1 现行文字之内，只补验收行。

**提交**

| 提交 | 内容 |
|---|---|
| `548c60d1` | 装载分配核班次在册：新只读口 `ports.TransportScheduleReader`；`FormLoadAssignmentDeps` 加 `Schedules`；`Form` 在重放与链头判定之后按（租户 + 班次）读班次登记，不在册答 `SCHEDULE_NOT_FOUND`（200），读不动答 `LOAD_ASSIGNMENT_UNDECIDED`；生产装配 `buildSegmentOperations` 接 `TransportSchedules`；单测、tfhttp 测试、真库测试 |
| `9ea15f73` | UC-TF-004 新增 `AT-TF-099` |
| 本笔 | 本票完成记录 |

**做了什么**

- 新格 `LoadAssignmentScheduleNotFound` → `SCHEDULE_NOT_FOUND`，追加在结果代数末尾。HTTP 层不用改分派：只有 `已形成` 答 201，其余都是 200。tfhttp 那句列举 200 答案的注释补了「班次不在册」。
- 顺序：受理 → 按键重放 → 链头 → 班次在册 → 登记。重放与链头两格交回的是库里那一版，跟这次送来的班次无关，所以班次核对排在它们后面。用例 `TestReplayAndChainHeadAnswerBeforeTheScheduleIsChecked` 钉住：即使班次登记为空或读不动，同版本重投仍答已在册、换版本号仍答分配已形成，班次登记一次也不读。真库那边，`TestTheLoadAssignmentFaceAnswersAlreadyFormedForASecondVersion` 只登了 S1，第二投引用不在册的 S2，照样答分配已形成。
- 他租户的引用落同一格：班次登记的键带租户，他租户名下的同名班次按本租户的键读不回，答复只有 `outcome` 一键，不透露它在别处存在。
- 真库测试先补登班次行：
  - cmd 侧新增 helper `registerTransportSchedules`，走生产班次登记适配器加领域构造门，不手插行。`load_assignment_single_chain_test.go` 各用例、`storable_values_channel_test.go`、`assemble_segment_operations_test.go`、`assemble_admission_scope_test.go` 都先登所引班次。
  - postgres 侧 `storable_values_regression_test.go` 复用包内已有的 `formedSchedule`。
- 新增真库用例 `TestTheLoadAssignmentFaceChecksTheScheduleIsRegistered`，走生产端点构造与生产准入桥：
  - 乱填的班次答 200 `{"outcome":"SCHEDULE_NOT_FOUND"}`，只登在 SYN-TENANT-02 名下的同名班次也答这一格。版本行、成员行都是零行。
  - 同一分配随后换成在册班次，答 201。
- 没碰 `OpenDispatchTaskDeps`。`buildSegmentOperations` 里那一行，以及 `storable_values_regression_test.go` 里同一函数中的那一行，都原样未动。

**代决与越权风险点**（授权同上；TF owner 复核）

1. **另开只读口，不直接依赖 `TransportScheduleStore`。** 新文件 `ports/transport_schedule_reader.go`，理由与 `DispatchTaskReader` 相同：核对在册只读，依赖带 `Save` 的口等于声明自己可能建立班次。另立文件是照方案 X4，不改 `ports.go`。
   - 越权风险点：这个文件在票面地盘之外。票 17（`EstablishPool` 核班次）、24（订舱引班次）可以复用它。票 16 若改 `ScheduleKey` / `ScheduleRecord`，它要跟着改。
2. **`SCHEDULE_NOT_FOUND` 不回显所引班次，也不带续办引用。** 没有形成任何版本可回显；原样重投仍是这一格，续办引用没有意义。这和 `SEGMENT_NOT_FOUND` 的先例同形。
   - 越权风险点：调用方得自己记住送的是哪个班次。
3. **读不动时的续办引用与装载分配登记册读不动共用一个**，都按（租户、分配、版本）派生。续办的是同一次分配提交，不按失败阶段分。
   - 越权风险点：只看答复，分不出是哪一道读失败。
4. **「在册」只判存在。** 不比班次出发时刻与分配时刻，不比方向，也不看取消或替代：班次现在还没有取消态。
   - 越权风险点：分配到已出发的班次照常形成。班次取消、替代落地时要回头加一格（TF owner）。
5. **既有行不回填。** 核对上线之前已登记、所引班次不在册的分配版本照旧可以重放，回显原版本。
6. **`AT-TF-099`。** 落笔前 grep 了全仓和在途分支 dcg16/22/26/31、nrs02、psxm05、fpa-accept：AT-TF 最大号是 098，099 起没有被占。
7. **地盘外改动**，都是 `FormLoadAssignmentDeps` 加字段带来的连带：
   - `internal/transportfulfillment/adapters/postgres/storable_values_regression_test.go`
   - `cmd/parcel-api/assemble_admission_scope_test.go`（只补登班次，原断言「形成一版」不变）
   - tfhttp `form_load_assignment.go` 一句注释
   - 新 ports 文件

**验证**（证据级别 `S`）

- `go build ./...`、`go vet ./...`、`gofmt -l cmd internal` 都干净。
- `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/ ./internal/architecture/`（真库 DSN 55432）：全部 ok。
- 变异实测：把「不在册」那一支短路掉，三条新用例转红：application `TestALoadAssignmentOnAnUnregisteredScheduleAnswersScheduleNotFound`、tfhttp `TestFormLoadAssignmentAnswersScheduleNotFound`、cmd `TestTheLoadAssignmentFaceChecksTheScheduleIsRegistered`。恢复后转绿。
- 没跑全量，没重生成机制清点（进 main 时统一做）。演示种子没改，所以没跑 `seed.sh`。

**偏离与待裁**

- **完成判据里的「演示」没有在演示库上走。** main 上还没有班次登记写入口，种子里也没有班次；那一步要 [16](./16-transport-schedule-templates-and-concrete-schedules-are-registered.md) 或 [18](./18-transport-preparation-consumes-the-initial-route.md) 的在册班次（票面 Blocked by 已注明）。暂由上面那条真库用例代证，它走的是生产端点构造与生产准入桥。16 进 main 之后，在一次性库上补走一遍：seed 后登一个物化班次，对它分配答 201，乱填班次答 `SCHEDULE_NOT_FOUND`。
- **给票 16 的提示。** 「自由串方向退役」若改了 `TransportScheduleSpec` 或方向写法，测试里登班次的地方各只有一处要跟：cmd 的 `registerTransportSchedules`、postgres 包的 `formedSchedule`。
- 演示动线不倒退：方案「已不成立或已修」03 已核，psb/05 重走的步 1–8 没有装载分配这一步。

## 评审记录（2026-10-19，隔离评审，评的是 `9ca6f8d4`；修订由作者子代理做）

阻断 0 项。非阻断 6 项，其中 3 项标了「建议进 main 前处理」：修了 2 项，第 3 项（重生成机制清点）按派单口径留到进 main。其余 3 项里修了 1 项，2 项登记去向。

**修了**

- **班次适配器缺 `TransportScheduleReader` 的编译期断言**（建议进 main 前）。`transport_schedule.go` 补了一组 var 断言，`TransportScheduleStore`、`TransportScheduleReader` 各一行，照 `DispatchTasks` 的写法。生成器输出到临时文件实测，端口基线口径回到「缺 28」，精确口径仍是「缺 2」。提交 `ab4c3d62`。
  - 和在途的 dcg16 在同一处相邻：dcg16 工作树在类型声明之后单加了一行 Store 断言。后落地的一方合成一组即可。重复的空白标识符断言照样能编译，但只留一组。
- **演示一项没有票接**（建议进 main 前）。二选一里选了「写进票 16 的完成判据」：
  - 票 16 的完成判据新增一行：对它显式登的那个物化班次提交装载分配答 201，对乱填的班次答 `SCHEDULE_NOT_FOUND`。
  - 本票完成判据的演示那一项注明已移交，Blocked by 同步改。
  - 本票因此可以照常标 resolved，不留只剩演示的尾巴。
  - 代决：选 16，不选 18，也不留尾巴。理由：
    - 方案「并行车道」波 1 的落地顺序里，19 在 16 之前。
    - 16 的演示本来就要显式登一个班次，对它分配是顺手的下一步。
    - 16 的作者落地前要 rebase，会看到这一行。
    - 18 在波 3，离得太远。
  - 越权风险点：改了别票票面（16 的完成判据）。如果实际上 16 先于本票进 main，这一项回到本票自己走。新增那一行里已写明这个条件。
- **夹具丢掉了班次登记的结果**。`storable_values_regression_test.go` 的 `newStorableFixture` 现在接住 `Save` 的结果，不是 `ports.ScheduleSaved` 就当场 `t.Fatalf`，不让后面的用例红在 `SCHEDULE_NOT_FOUND` 上、看不出病根。同在 `ab4c3d62`。

**没采纳**

- **重生成 `docs/product/MECHANISM-INVENTORY.md`**（建议进 main 前）。本步不做：派单口径明确「不要重生成机制清点（进 main 时统一做）」，方案「并行车道」也定了每次落地重新生成、冲突时以重新生成为准。补完断言后，本分支实测是：transportfulfillment 文件 158，合计 1290，端口声明 488，基线缺 28，精确缺 2。进 main 时在干净检出上重生成即可对齐。
- **读回过不了构造门时答 `LOAD_ASSIGNMENT_UNDECIDED`**（非阻断）。本票不改，也不另开只查存在的轻读口。理由：
  - 过不了构造门的班次行是库里的坏行，属于系统故障，不是调用方换个载荷就能修好的。坏行修好之后，同一提交原样重投就能形成，带续办引用的 `UNDECIDED` 对的正是这个恢复动作。答成 `SCHEDULE_NOT_FOUND` 反而会把运营引去登一个其实已经在册的班次。
  - 只查存在的轻读口会把坏行算作在册，分配就形成在一个谁也读不回的班次上。坏行被藏起来了，而不是暴露出来。
  - 票 16 的方向改形：它的完成判据已有「自由串方向退役（表无生产行）」。dcg16 在途的迁移草稿是表里有行就拒绝迁移，迁移之后不会留下过不了新读回门的旧行，所以不再给 16 加判据。
- **UC-TF-004 步 1 的文字和 AT-TF-099 字面上有点拉扯**（非阻断）。不改：方案 D03-2 有意只补 AT 行；ADR-0029 优先于 UC，`输入未受理`本来就只留给查询之前就短路的那一支。去向：以后哪张票改 UC-TF-004 的步骤文字，顺手在步 1 补半句「引用对象不在册按恢复动作另答，见 AT-TF-099」。不另立票。

**修订后验证**（证据级别 `S`）

- `go build ./...` 干净；`go vet ./internal/transportfulfillment/... ./cmd/parcel-api/` 通过；`gofmt -l internal cmd` 干净。
- `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/ ./internal/architecture/`（真库 DSN 55432）：全部 ok。
- 机制清点只输出到临时文件核端口口径，没有改提交的清点文件。

## 进 main 记录（2026-10-19）

- TF 链第一波 16、19、22、26、30、31 合成一批，按 19 → 22 → 26/30 → 31 → 16 叠到 main 上，整批快进推送，与本记录同一次推送。
- 合批时的处理：
  - README 的 ADR 行按号合并。
  - `.scratch/at-coverage-inventory.md` 计数表：CC 行取 31，NO 行取 main，合计重算；两段追记都留。
  - 19 与 16 的接缝在 `d3c39a77`：19 的班次夹具改用 16 的方向与临时班次构造；19 在 `transport_schedule.go` 加的两行端口断言保留。
  - 26 立的 38 号票与 16 立的 38 号票撞号，26 那张改为 39，ADR-0208 风险点 6 同步。
- 合批种子在一次性新库上 `seed.sh` 灌通，退 0，用完删库。
- 全量 `go test -p 1 -count=1 ./...`（带 55432 DSN）在干净游离检出上钉 `60f8d75c`：146 包 ok，0 FAIL，0 SKIP。机制清点在同一 tip 上重生成。
