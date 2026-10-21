# 132 NR 复核不按业务时间排序，较早触发可能在较新复核之后再判一次

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-dcg131（2026-10-21，见文末「完成记录」）。此前 2026-10-21 认领（131、132 同批）；此前 ready-for-agent——2026-10-20 代决复核立（上线前必修），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M11；ADR-0215 越权风险点 11

## 现象与修法

现象、证据与修法的完整写法以复核报告 M11 一条为准（单一权威），此处只记要点：

复核前比较触发的业务时间与路由历史里最近一次控制触发的业务时间；较早的触发入账为「触发早于当前位置」，不判。收寄采用与交接两路同改，并在 ADR-0215 风险点 11 处补指针。

## 完成记录（2026-10-21，Claude Code 作者子代理，分支 `claude-dcg131`，与 131 同批）

### 接票时核实

M11 的现象成立：复核编排只拿当前路由历史判断，复核册的 `reassessed_at` 是处理时钟，册里没有所判触发的业务时间，比较无从做起。另核得两处：

- 交接更正承继原版的判断时刻（TF 领域 `TransportHandover.Correct` 不改 `JudgedAt`，另记 `CorrectedAt`）。所以「同一时刻怎么办」直接决定 ADR-0215 决定三「更正之后仍`已交接`，另成一次复核」还成不成立。
- 两路分属不同分区，派发并发时没有任何东西让同一包裹的两次复核串行，光比时间不加锁照样会双双判过。

### 代决（用户授权「要我裁定的部分，你作为业务和系统专家，请你自决吧」，代 NR owner）

权威落在 [ADR-0235](../../../docs/adr/0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 决定四、五与 NR CONTEXT「路由稳定性与受控改路」新加的一条。这里只记要点：

1. **当前位置**：这一判断键在册复核里业务时间最晚的那一次已判控制触发。复核册每行记下所判触发的业务时间（`trigger_occurred_at`，迁移 0022），在编排提交那一处写上。入账未判的触发不推当前位置。
2. **比较的位置与结局**：在已有结果分界之后、读路由历史之前比。触发严格早于当前位置时答`未判`，原因 `TRIGGER_PRECEDES_CURRENT_POSITION`，未判登记另记比较所用的当前位置时刻；不判，不失效不改路。已判过的触发再投读回原结果。
3. **同一时刻照判**：没有先后可比，不替它们排序；交接更正承继原判断时刻，因此照判，ADR-0215 决定三不受影响；更正一次更早的交接时入账不判。
4. **串行**：读当前位置的口按判断键取事务级咨询锁，锁随事务释放，口径同 ADR-0226 决定二；不在事务里拒。
5. **存量**：迁移之前的复核没有触发业务时间、无从补回，不参与比较；新行必带——适配器拒零值，库里 CHECK 以 NOT VALID 加上。
6. **ADR-0215 越权风险点 11**：只在 Status 行与 Links 加前向指针，正文不改（与票 131 那一条合写在同一处）。

### 越权风险点（全文见 ADR-0235「越权风险点」，本票相关的几条）

1. 当前位置只认已判的复核：较晚的触发因未配置没判时，较早的那次之后若判得成，会从较早的位置判。按到达顺序处理同样如此，不比现状更坏。
2. 同一时刻照判：同一时刻、位置不同的两次控制都判，最后落库的那次决定计划状态。来源只给到分钟时更容易撞上。
3. 迁移 0022 之前的复核不参与比较；首发前只有演示租户，`--reset` 重灌即可。
4. 较早那次不进复核册，只在交接册、收寄采用记录与未判登记里。
5. 判断键串行的代价：同一包裹两路复核互等，锁持到 inbox 事务结束；哈希撞键只多等。
6. 业务时间取自来源：来源把时间填错时，较早与较晚会被判反。

### 做了什么

- `internal/networkrouting/domain/reassessment_trigger.go`：`ReassessmentTrigger.PrecedesPosition`（严格早于才算早）。
- `internal/networkrouting/ports/ports.go`：`ReassessmentRecord.TriggerOccurredAt`；`NotJudgedTriggerPrecedesCurrentPosition`、`NotJudgedTrigger.CurrentPositionAt`；`ReassessmentStore.LatestTriggerAt`。
- `internal/networkrouting/application/reassess_route.go`：分界之后比当前位置；`commit` 写触发业务时间；`notJudged` 带当前位置时刻。
- `internal/networkrouting/adapters/postgres/route_reassessment.go`：`Save` 写、`FindByCorrelation` 读触发业务时间，`Save` 拒零值；`RecordNotJudged` 写当前位置时刻并校验「仅且必在早于当前位置时」；`LatestTriggerAt` 取判断键咨询锁后取最大值。
- 迁移 network_routing 0022。
- 测试替身（application 与两路适配器）随端口改；postgres 既有用例的复核记录补触发业务时间，形状探针补这一列，免得被新 CHECK 先拒、不再证它点名的那条约束。
- 文档：NR CONTEXT「路由稳定性与受控改路」加一条；UC-NR-003 结果`触发未判`补这一格、一致性节引 CONTEXT 与 ADR-0235、新立 `AT-NR-079`–`083`；交接更正用例补 AT-NR-081 点名；AT 盘点加五行与追记（计数未动）。派发装配与路由文件不变。

### 用例

- 领域：严格早于才算早，同一时刻（含换时区写法）与较晚都不算（AT-NR-079/081）。
- 编排：较早触发后到入账不判、不动适用性、记当前位置时刻、再投同答（AT-NR-079）；较新与同一时刻照判、已判触发被超过后再投读回原结果（AT-NR-081）；两种到达顺序下最后一次已判都是较晚那次、较早那次恰在判与未判之一，读不出当前位置答未决（AT-NR-082）。
- 复核库真库 `route_reassessment_position_test.go`：按判断键取最晚、别的键不掺、读回原值、缺值适配器与库各拒、无事务拒（AT-NR-079/083）；「早于当前位置」必带且必早于当前位置时刻（适配器与库各拒）；两笔并发事务读当前位置按判断键串行，后一笔等前一笔提交、读到它刚落的复核（AT-NR-083）。
- 派发真库 `cmd/parcel-dispatch/reassess_business_time_test.go`（交接与收寄采用两路接同一份复核编排，与生产同形，只换证据视图）：
  - 交接一路：更早的交接（进不在计划上的节点）后到，入账不判、计划不失效、另一封信封再投同答；随后更晚的交接照判、计划失效（AT-NR-079/081）。
  - 收寄采用一路：更早的节点收寄采用在已判交接之后到，入账不判（AT-NR-080）。
  - 两种到达顺序的确定答案与后到的再投（AT-NR-082）。
- 改前核对：把 `PrecedesPosition` 临时改成恒假，编排一条与派发真库三条红，改回后绿。

### 验证

同票 131 的「验证」一节（同一分支、同一批跑）：于 `7e7dc058`，build、vet、gofmt 干净；两笔代码提交各自单独 build、vet 通过；`scripts/test-full.sh` 跑改动包及其反向依赖 44 个包，加 `./internal/architecture/...`、`./migrations/...`，全绿。未跑全量；未重生成机制清点、ADR 索引与 AT 计数。
