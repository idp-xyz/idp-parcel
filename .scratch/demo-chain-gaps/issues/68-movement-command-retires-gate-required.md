# 68 移动命令退役 `gateRequired`

Category: enhancement
Status: resolved——2026-10-19 进 main（全量钉 `71aff222`：152 ok / 0 FAIL；交接与派送 A 波同批，见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-dcg68。2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: 无
地盘：TF `application/record_movement_fact.go`(+test)、tfhttp `record_movement_fact.go`(+test)、`work_fact_intake_test.go`；`cmd/parcel-api/assemble_movement_fact_test.go`、`transport_fulfillment_command_lines_test.go`；文档 ADR-0205 复核记录一行、UC-TF-005 `AT-TF-113`。
出处：[tf-chain-plan](../tf-chain-plan.md)「等 peer 的」删掉移动命令与载荷里的 `gateRequired` / `gateClearance`；方案「代决」C16。

## 做什么

- 应用命令删掉 `GateRequired`；HTTP 载荷照收照弃，作退役键注明；`gateClearance` 保留作证据（方案 C16）。
- 不另立 ADR，只在 ADR-0205 复核记录里加一行。

## 完成判据

- 载荷带 `true`、带 `false`、不带：读数与内容摘要逐字相同，均 201。
- `gateClearance` 照存。
- 真库测试。
- 演示：出发载荷带 `"gateRequired":false` → 201 `MOVEMENT_FACT_RECORDED`，`gateReading` 与不带时相同。

## 规模

S

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg68`）

### 接票时核实

- `gateRequired` 只在应用命令 `RecordMovementFactCommand` 与 tfhttp `MovementFactPayload` 上，编排与 `DepartureGateReader` 都不读它（票 32 起）。作业事实这一面 `WorkFactCommandIntake.IntakeMovementFact` 把载荷原样交给 `MovementFactPayload.Command`，不碰这一格，所以删命令字段不必改 `work_fact_intake.go`；只有它的测试钉着透传。
- 载荷经 `decodeClosedPayload`（`DisallowUnknownFields`）解：直接删键，仍发它的设备收 400（方案「等 peer 的」`gateRequired` 一条）。
- 库面本无此列（迁移 TF 0010 头注），内容摘要也从不收它：不需要迁移，TFC-1 形状不变。
- 仓内没有生产代码发这个键：一线导入、admin-web、演示种子都没有；发它的只有测试。

### 代决（用户授权「你作为业务和系统专家自决……解决所有的问题」）

方案 C16 定了删命令字段、载荷照收照弃、`gateClearance` 保留。下面几处是方案没裁、本票自决的：

1. **退役键的形状**：`RetiredGateRequired bool` 带 `json:"gateRequired,omitempty"`。改名是为了让读代码的人一眼看出它已退役，`Command` 不读它。类型仍是布尔，给非布尔值照旧答 400，不借退役把这一格放宽。`omitempty` 让日后若有人编码这份载荷，不会再把它发出去。
   - 越权风险点（TF owner）：渠道契约上这个键永远留着（C16 原风险点），何时可删未裁，例如按设备版本或渠道契约换大版本。给非布尔值的离线设备仍会 400，这与退役前一样，不是本票引入的。
2. **不告知设备键已退役**：响应不加提示格，也不加 `Deprecation` 头。响应是封闭形状，加格要与一线客户端一起改；告知归一线客户端或渠道契约那一侧。
   - 越权风险点（TF owner、一线客户端）：还在发这个键的设备永远不知道它已经不起作用。
3. **ADR 只加复核记录**：不另立 ADR，在 ADR-0205 文末加「复核记录」一节一行（方案「需要的 ADR」末段）。Status 行与 `docs/adr/README.md` 都不改，因为没有停用任何条款：决定三本就写了「暂留……删它要随那一侧一起改」，本票是它的落地。
   - 越权风险点（TF owner）：README 该行仍写「`gateRequired` 不读」，与现状相容，但没提退役；读者要看到 ADR 文末才知道。
4. **AT-TF-113 换层钉**：命令里没有这一格了，「真假都不挡」那一半在编排层无从表达，改在线格式与真库两层钉。线格式层经生产作业事实 Intake 的封闭解码，比较三种写法交出的命令是否相等；真库层比较三条出发的答复与库面。编排层用例改名，只钉凭据。

### 做了什么

- 应用 `application/record_movement_fact.go`：命令删 `GateRequired`，头注改写为「命令不带设备自报的『要不要门禁』」及其理由。`read_departure_gate.go` 头注一句随之改。
- tfhttp `record_movement_fact.go`：`MovementFactPayload.GateRequired` 改为 `RetiredGateRequired`，照收照弃，字段注释写明不删键的理由（封闭解码、离线队列 4xx 出队、ADR-0205 决定一）；`Command` 不再转交；`MovementFactIntake` 与载荷头注随之改。
- 测试：
  - 编排层：`TestTheDeviceGateFlagIsNotReadAndItsClearanceIsKeptAsEvidence` 改名为 `TestTheDeviceClearanceIsKeptAsEvidenceAndDoesNotChangeTheReading`，只钉凭据；`TestTheSameVersionIsAReplayOnlyWhenItsContentIsTheSame` 去掉「只差 GateRequired」一格，「去掉门禁与放行」改为「去掉放行」；`read_departure_gate_test.go` 去掉 `GateRequired = false` 一行与 AT-TF-113 一句。
  - 线格式：新 `TestTheRetiredGateRequiredKeyIsAcceptedAndDiscarded`，`true`、`false`、`null` 与不带交出同一条命令；给字符串值答 `ErrMalformedRequest`。
  - 真库：新 `TestTheRetiredGateRequiredKeyChangesNothingOnTheDemoDeparture`，见下文「完成判据逐条」。
  - `TestTheWiredMovementFactAnswersHonestlyAgainstARealDatabase` 删去 `GateRequired = true` 一行，头注随之改；tfhttp 端点用例的请求体替身删去这一格。
- 文档：ADR-0205 文末「复核记录（2026-10-19，票 demo-chain-gaps/68，不停用；决定原文不动）」一行；UC-TF-005 `AT-TF-113` 按退役键改写；AT 覆盖盘点 `AT-TF-113` 一行换成上面三个用例。

### 地盘外的改动

- `application/read_departure_gate.go`（头注一句）与 `read_departure_gate_test.go`：字段删了，不改编译不过。
- `domain/payload_canonicalization_test.go` 只改了一句注释。`payload_canonicalization.go` 本体是 claude-dcg21 持令牌的文件，没碰。
- `adapters/http/work_fact_intake_test.go`：票面地盘列了它，派单的协作边界又把 `work_fact_intake*.go` 划给 idp-parcel-9b。只改了 `TestWorkFactCommandIntakeTranslatesMovementFactWithTheAuthenticatedTenantAndSource` 的一个判断条件和头注一句，不改编译不过。载荷常量没动，仍带 `"gateRequired":true`，正好钉住封闭解码会收它。进 main 前 9b 若改过这个文件，冲突只在这两处。
- `.scratch/at-coverage-inventory.md` 改了一行。
- 没碰：`work_fact_intake.go`、`endpoints.go`、`main.go`、`docs/adr/README.md`、演示种子与演示脚本，以及 `transport_fulfillment_command_lines_test.go`。最后这个文件票面地盘列了，但里面只有到达的载荷和一句「不带 gateRequired / gateClearance」的注释，仍属实；不改它，也就不撞 operator-channel/48 的注释改动。

### 完成判据逐条

- **载荷带 true、带 false、不带，读数与内容摘要逐字相同，均 201**：由真库用例钉。演示动线上同一段、同一刻三条出发，经生产作业事实渠道各答 201 `MOVEMENT_FACT_RECORDED`，答复除事实标识外逐字相同（读数、段、放行凭据都在答复里），库面读数同为 `NOT_GUARDED`。内容摘要两侧现算、不存，所以用重放验：拿带 true 那条的身份，分别带 false、不带再报，都答 200 `MOVEMENT_FACT_VERSION_EXISTS`，回显同一份读数。线格式层另钉：三种写法加 `null` 交出同一条命令。
- **`gateClearance` 照存**：答复回显，库面 `gate_clearance` 列照存。
- **真库测试**：同上。
- **演示**：同一用例走生产作业事实渠道与生产装配，节点区域与周期安排按演示种子原样登，证据记 `S`。带 `"gateRequired":false` 那条答 201 `MOVEMENT_FACT_RECORDED`，`gateReading` 与不带时相同。

### 验证

- 红绿：线格式和真库两条新用例各红过一次。
  - 把退役键从载荷形状里拿掉：新线格式用例与 `TestWorkFactCommandIntakeTranslatesMovementFact…` 都答 `unknown field "gateRequired"`（400）。
  - 让退役键漏进命令（带真时改资源）：真库用例在第二条出发上因答复不同而红。
  - 两处临时改动都已复原。
- `go build ./...`、`go vet ./...`、`gofmt -l internal cmd` 干净。
- `IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -p 1 -count=1 ./internal/transportfulfillment/... ./internal/architecture/ ./cmd/parcel-api/` 全 ok。
- 没跑全量，没重生成机制清点（进 main 时统一做）。

### 偏离与待裁

- 演示判据先由生产装配上的真库用例取证（只记 `S`），交评审时没有起进程。评审后已在一次性库上灌种子、起 `parcel-api` 与 `parcel-dispatch` 补走，见「评审记录」第 4 条。演示种子未改。演示脚本本就不写这个键，没加补记。
- 待裁（TF owner）：退役键何时可删；要不要告知设备键已退役（代决 1、2）。

## 评审记录（2026-10-19，隔离评审后作者子代理修订）

阻断 0 项，非阻断 7 项（1 项建议进 main 前修）。逐项去向：

1. **`read_departure_gate.go` 的 `DepartureGateReader.Read` 头注前后相反**（建议进 main 前修）：第一段写「出发一律照问」，第二段写「缺哪样就停在哪样、不问关务」。已修：改为「问不问关务不看设备自报：设门与否由关务按对象与起讫判（ADR-0205 决定三）」，去掉「一律」和带历史口吻的「本就」。
2. **`MovementFactPayload` 头注穷举另一个包的字段差集**（「命令里没有的只有退役键」）：已修，改为「退役键 `gateRequired` 不进命令，见该字段」，不再断言差集只有这一格。
3. **注释带变更叙述**（「类型照旧是 bool」「与退役前一样」）：已修，改为「类型是 bool」「给的不是布尔是坏报文（400）」。同一字段注释里的「设备自报失了权威」也是叙述口吻，一并改成现状陈述「设备自报不算数」。
4. **演示没起进程，与方案「拆票表」首句不符**：已补走，证据记 `S`，走完删库。
   - 环境：一次性库照 `seed.sh` 原样灌，退 0。`parcel-api` 配操作者渠道与治理自授权，`parcel-dispatch` 用演示动线脚本里的派发参数。
   - 请求：配置员令牌，加上 OFFSITE 合成设备三头（`SYN-DEVICE-OFFSITE-CN-EAST-01` / `OFFSITE` / `SYN-OFFSITE-CN-EAST`）。同一班次、地点与时刻，带同一份放行凭据，`POST /transport-fulfillment/movement-facts` 报三条出发，载荷分别带 `"gateRequired":false`、不带、带 `true`。
   - 结果：
     - 三条都答 `201` `MOVEMENT_FACT_RECORDED`。答复除 `fact` 外逐字相同，`gateReading` 都是 `{"answer":"UNDECIDED","stops":["SEGMENT_NOT_DECLARED","SCHEDULE_NOT_REGISTERED"]}`，`gateClearance` 照回。
     - 拿带 `true` 那条的身份，分别带 `false`、不带再报：都答 `200` `MOVEMENT_FACT_VERSION_EXISTS`，回显同一份读数。
     - 带 `"gateRequired":"no"` 答 `400` `MALFORMED_REQUEST`。
     - 库面三行读数都是 `UNDECIDED`，凭据照存。`transport-fulfillment.movement-fact.recorded` 只入队三条（重报没有多入），派发进程都投成 `PUBLISHED`，三条载荷除事实标识外相同。
   - 没走的：这次没有声明段，`NOT_GUARDED` 那一格要先登班次并经交接口立段、让包裹在控，进程演示没搭这条链。这一格仍由真库用例 `TestTheRetiredGateRequiredKeyChangesNothingOnTheDemoDeparture` 钉。
5. **`work_fact_intake_test.go` 在 idp-parcel-9b 的地盘**：本轮不动，交进 main 的会话办。进 main 前须向 9b 确认它没改这个文件；若有冲突，只会落在 `TestWorkFactCommandIntakeTranslatesMovementFactWithTheAuthenticatedTenantAndSource` 的判断条件和头注那一句。本子代理够不着 9b，它不在本机。
6. **`AT-TF-113` 结果栏「三种写法」与凭据那一种混在一起，「照样逐对象问关务」只有间接佐证**：两处都改了。
   - UC-TF-005 那一行改成「带真、带假、不带均 201」。
   - `TestADepartureAsksCustomsForEveryObjectUnderControlWithTheScheduleDirection` 的 Covers 补上「AT-TF-113 编排半边『照样逐对象问关务』」，AT 覆盖盘点的 `AT-TF-113` 行把它补回来。
7. **`docs/adr/README.md` 里 ADR-0205 登了两行**（存量，不在本票 diff 内）：另立 [74](./74-adr-readme-lists-adr-0205-twice.md)，号取协调方分给本票的 74–75。README 是跨票热点，只许追加，本票照旧不改它。

修订后验证：

- `go build ./...`、`go vet ./internal/transportfulfillment/... ./cmd/...`、`gofmt -l internal cmd` 都干净。
- 带 DSN 和 `-p 1 -count=1` 跑 `./internal/transportfulfillment/...`、`./internal/architecture/`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`，全部 ok。

## 进 main 记录（2026-10-19）

- 交接与派送 A 波 60、64、68 合成一批，按 68 → 64 → 60 叠到 main 上，整批快进推送，与本记录同一次推送。合批时协调方在合批分支上直接做了票 71（派送发起过期注释改指 ADR-0216）与票 74（ADR 索引 ADR-0205 两行合一）；另把 ADR 索引里 0206、0207、0209、0210、0217 各两行合成一行——那是协调方先前合批时的冲突解法把同一行的新旧两版都留下了，解法已改为按合并基点判新旧。
- 合批种子在一次性新库上 `seed.sh` 灌通，退 0，用完删库。
- 全量 `go test -p 1 -count=1 ./...`（带 55432 DSN）在干净游离检出上先钉 `a141588f`：151 包 ok；推送前 main 进了 ps-external-mark-relations/11，本批重放到它上面、种子重灌退 0，全量在推送 tip `71aff222` 上重跑：152 包 ok，0 FAIL，0 SKIP。机制清点在推送 tip 上重生成。
