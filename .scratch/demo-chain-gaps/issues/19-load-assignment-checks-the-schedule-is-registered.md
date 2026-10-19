# 19 装载分配核班次在册（03a）

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg19（见文末「完成记录」）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: 无（演示那一步要 [16](./16-transport-schedule-templates-and-concrete-schedules-are-registered.md) 或 [18](./18-transport-preparation-consumes-the-initial-route.md) 的在册班次）
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
- 演示：对物化班次分配答 201，对乱填班次答 `SCHEDULE_NOT_FOUND`。

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

