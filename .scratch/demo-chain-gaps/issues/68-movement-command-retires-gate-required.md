# 68 移动命令退役 `gateRequired`

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg68。2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
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

- 演示判据由生产装配上的真库用例取证（只记 `S`），没有起进程按演示脚本重走，同票 32、33 的做法。演示种子未改，没有重灌。演示脚本本就不写这个键，没加补记。
- 待裁（TF owner）：退役键何时可删；要不要告知设备键已退役（代决 1、2）。
