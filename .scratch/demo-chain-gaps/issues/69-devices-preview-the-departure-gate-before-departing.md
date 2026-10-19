# 69 设备出发前门禁问询

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg69（见文末「完成记录」）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: 无（装配令牌排在 [25](./25-segment-adopts-transport-commission-and-cancellation-faces.md) 之后）
地盘：TF 新 `application/preview_departure_gate.go`（复用 `DepartureGateReader`）、新 tfhttp `preview_departure_gate.go`；新 `cmd/parcel-api/assemble_departure_gate_preview.go` + `endpoints.go` 一行；文档 ADR-0219、UC-TF-005 AT。不改 `internal/accessidentity`。
出处：[tf-chain-plan](../tf-chain-plan.md)「等 peer 的」设备出发前门禁读口；方案「代决」C17，「需要的 ADR」ADR-0219。

## 做什么

- 作业事实渠道开只读问询，交回与出发登记同一套读数；TF 不落记录，授予与记录照方案 C17。
- 文档：起草 ADR-0219。
- 进 main 的令牌次序见方案「波次与令牌」。

## 完成判据

- 答复与随后同参出发的读数同格。
- TF 零写入；CC 判断册照记。
- 未授予答 403。
- 真库测试。
- 演示：出发前问询 → 200，读数 `NOT_GUARDED`；随即出发 → 201，读数相同。

## 规模

M

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg69`）

### 接票时核实

- 票 32 的 `DepartureGateReader.Read` 只吃 `RecordMovementFactCommand`：先过 `movementFactSpecFrom`，事实身份、地点、来源、版本缺一就不读。读数实际只用其中四样：租户、班次、段、出发时点。问询拿不出事实身份与版本，硬拼一条假出发命令去调 `Read` 不对，所以把读数路径收成一条只吃这四样的内部函数。
- 作业事实渠道的授予按作业范围给，不按端点给（`WorkFactMinter.MintWorkFact` 核的是 `HoldsWorkFactAt(scope)`）。所以「持移动事实登记授予即可问」不用碰 access-identity：问询挂同一只 `WorkFactCommandIntake` 即可，`tenantFrom` 那道门（三头、只认 OFFSITE、设备在用）原样适用。
- 关务那层壳（`transactionalActionAdmissionJudge`）在调用方事务里调用即拒，读数随之记成调用失败。问询只能不包事务。
- 关务判断标识按内容派生、不含判断时刻（ADR-0206 决定六）。所以问询之后同参出发命中同一行，判断册上不会多出重复判断；真库用例钉住了这一点。
- 票 25 已进 main，装配令牌空出；文档令牌上 69 排最后，在途的 59、65、66、67 会改同一份 UC-TF-005 与 TF CONTEXT。

### 代决（用户授权「你作为业务和系统专家自决……解决所有的问题」）

方案 C17 定了：作业事实渠道开只读问询，按（段、班次、拟出发时点）交回与出发登记同一套读数；TF 不落记录；CC 照 ADR-0206 决定六记判断；持移动事实登记授予即可问，不新增能力种类。下面几处方案没裁，本票自决，全部写进 ADR-0219：

1. **路径与方法**：`POST /transport-fulfillment/departure-gate-previews`。路径前缀随移动事实口（作业事实那组用 `/transport-fulfillment/`），后缀随本仓「-previews」那一族；方法是 POST 加封闭载荷，与本渠道其余各口同一道门。
   - 越权风险点（TF owner）：只读口用 POST，HTTP 缓存与幂等语义上不是 GET；一线客户端那一侧若要缓存问询结果，要自己处理。
2. **载荷只收三格**：`schedule`、`segment`（可缺）、`departsAt`，封闭解码。照抄整条出发载荷来问会答 400。
   - 越权风险点（TF owner、一线客户端）：设备不能把出发载荷原样转发来问，要另组一份。
3. **拟出发时点缺席答未受理，不补服务端时钟**（ADR-0023；`parseOptionalInstant` 原有的纪律）。
   - 越权风险点（一线客户端）：「我现在出发行不行」要设备自己填此刻。
4. **段缺席照问**：读数停在「段未声明」答未决，与那条出发不报段时同格。不把段缺席答成未受理，否则两口对同一输入的受理不一致。
5. **答复一律 200，`gateReading` 与移动事实口同一线格式类型**，不回显请求三格。outcome 只有 `DEPARTURE_GATE_PREVIEWED` / `INPUT_NOT_ACCEPTED` 两格：读数自己装得下读不动与未决，问询不写库，没有要续办的停点。
6. **同参同读数靠结构，不靠比对**：读数路径收成 `readIntent` 一条，出发登记与问询各自受理后进它。装配上读数编排收成 `buildDepartureGateReader` 一处构造，两口各取一份。两口受理一致也一并钉住：同名三格在出发那口不受理的，在问询这口同样不受理。
7. **问询的读数不交给随后的出发**，出发照 ADR-0205 在它自己那一刻重问。
   - 越权风险点（TF owner）：读到放行不等于出发那一刻仍放行（C17 原风险点）。
8. **不记谁问过、不立问询册**。
   - 越权风险点（TF owner、VE owner）：事后答不出这台设备出发前问没问、问到的是什么。
9. **不挡出发**：挡不挡归一线客户端，本仓今天没有客户端代码。
   - 越权风险点（TF owner、一线客户端 owner）：读到阻断时客户端怎么处置未定。在那之前，ADR-0205 越权风险点 1 的「挡下」只补了服务端那半。
10. **其余越权风险点**见 ADR-0219 文末，这里不复写：用写授予开读口、关务判断册随轮询增长、段里有什么可零成本探知、同格要求两问之间册子不变、集运单元的假干净读数照样继承。

### 做了什么

- 应用：
  - `application/read_departure_gate.go`：`Read` 受理后转交新的 `readIntent`（内部，吃 `departureGateIntent` 四样），读数逻辑一字未改，只换了取值来源。
  - 新 `application/preview_departure_gate.go`：`PreviewDepartureGateHandler`、命令与结果代数。只依赖 `*DepartureGateReader`，结构上没有写口。
- tfhttp：新 `adapters/http/preview_departure_gate.go`，含 Intake 接口、载荷、端点，以及 `UnconfiguredIntake` 与 `WorkFactCommandIntake` 两种答法。答法照 `trigger_delivery_dispatch.go` 的先例声明在本文件。
- 装配：
  - 新 `cmd/parcel-api/assemble_departure_gate_preview.go`：`buildDepartureGatePreview`，以及取 Intake 的 `departureGatePreviewIntake`。
  - `assemble_movement_fact.go` 把读数编排的构造抽成 `buildDepartureGateReader`，出发登记改调它。
  - `endpoints.go` 末尾追加一个参数、一行端点；`main.go` 在移动事实那段之后追加一次构造，调用末尾追加一个实参；`unwired_orchestration.go` 追加占位。
- 测试：
  - 编排：`TestAPreviewAnswersTheSameReadingAsTheDepartureWithTheSameParameters`，覆盖五格与本侧各停点下读数深相等、问关务的问法逐问相同。另有 `TestAPreviewThatCannotBeAcceptedAsksNothing`（两口受理一致）与 `TestAPreviewSurfacesAMisansweringViewAndRefusesAMissingReader`。
  - 线格式：两口 `gateReading` 逐字节相同；未受理、400、405、403（未授予、设备不在用、未配置不读体）、500 不带 outcome。作业事实各口那张表与类型锁各加一行。
  - 真库：新 `assemble_departure_gate_preview_test.go` 三个用例，见下文「完成判据逐条」。`endpoints_test.go` 探针表、`assemble_work_fact_operator_test.go` 渠道答复表、`transport_fulfillment_command_lines_test.go` 主链命令行表各加一行。
- 文档：
  - ADR-0219。
  - ADR-0205 的 Status 行与 Links 各加一句后继指针，正文不动。
  - `docs/adr/README.md`：0205 行尾追加「出发前读口已答」，0217 行后加 0219 一行。
  - UC-TF-005：一条规则，以及 `AT-TF-226`–`231`。
  - TF CONTEXT「实际移动与出发门禁读数」一条；GLOSSARY「出发前门禁问询」词条；演示脚本 33 补记之后加一条补记。
  - `.scratch/at-coverage-inventory.md`：六行与一条追记。

### 地盘外的改动

- `cmd/parcel-api/assemble_movement_fact.go`：抽出 `buildDepartureGateReader`，`buildMovementFactOrchestration` 改调它，头注随之改。行为不变，`TestAReplayedDepartureDoesNotAskCustomsAgain` 等原用例照过。
- `application/read_departure_gate.go`：票面写的是「复用」。复用要先把读数路径从出发命令里拆出来，所以改了这个文件。
- `adapters/http/work_fact_intake_test.go`：作业事实各口那张表加一行，类型锁加一行（文件头注写明「加口时两处都要加」）。没碰 `work_fact_intake.go` 本体：两种答法写在新文件里。
- `cmd/parcel-api/assemble_work_fact_operator_test.go`、`transport_fulfillment_command_lines_test.go`：各加一行。
- `docs/domain/GLOSSARY.md`、`docs/design/synthetic-demo-journey-script.md`、`.scratch/at-coverage-inventory.md`。
- 没碰：`internal/accessidentity`、`internal/nodeoperations/**`、PS 外部标识读口、`assemble_reception*.go`、`cmd/parcel-frontline-import`、`apps/admin-web`、`cmd/parcel-dispatch`、演示种子、CC 代码、CONTEXT-MAP。

### 完成判据逐条

- **答复与随后同参出发的读数同格**：
  - 编排层：五格与本侧各停点下，问询读数与同参 `Read` 深相等，问关务的问法逐问相同。
  - 线格式层：两口答复里的 `gateReading` 逐字节相同。
  - 真库层：演示动线那一段上，不受门禁、阻断两种读数下，两口 `gateReading` 都逐字节相同。
- **TF 零写入；CC 判断册照记**：真库用例按整库取指纹（每张基表整表摘要加全部序列）。问询前后只有 `customs_compliance.action_admission_judgement` 变了。随后同参出发不再让这本册变；出发落册的逐对象依据指回的，正是问询那一次记下的判断。
- **未授予答 403**：
  - 线格式层：`OPERATOR_NOT_GRANTED`、`DEVICE_NOT_IN_USE`。
  - 装配好的路由：读授予不是作业事实授予时答 403 `OPERATOR_NOT_GRANTED`，渠道答复表那一行。
  - 真库：在没有授予的作业范围上问，答 403，整库指纹不变，关务也没被问到。
- **真库测试**：同上，另有 `TestAPreviewIsAReadingNotAPermit`：问询答不受门禁之后落了一条限制，随后出发按它自己那一刻重问、读数阻断。
- **演示**：`TestADemoDeviceAsksBeforeDepartingAndTheDepartureReadsTheSame`。生产装配，关务节点区域与上海→深圳周期安排按演示种子原样登，经作业事实渠道走。问询答 200 `DEPARTURE_GATE_PREVIEWED`，读数 `NOT_GUARDED`；随即同参出发答 201，读数逐字节相同。证据记 `S`。

### 验证

- 变异各红过一次，复原后再绿：
  - 问询时点挪 90 分钟：编排相等用例红。
  - 问询答复改 201：线格式两条用例红。
- `go build ./...`、`go vet ./cmd/parcel-api/ ./internal/transportfulfillment/...` 干净；改动的 Go 文件 `gofmt -l` 干净。
- `IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -p 1 -count=1 ./cmd/parcel-api/ ./internal/architecture/ ./internal/transportfulfillment/... ./internal/customscompliance/...` 全 ok。
- 没跑全量，没重生成机制清点（进 main 时统一做）。

### 偏离与待裁

- **号段**：
  - AT 用了 226–231，232–235 未用。
  - 新立票号 82–83 未用：越权风险点都是待 owner 复核的取舍，没有一条是已裁定、待排期的工作。
  - 没有迁移。
- **演示判据**用真库装配用例取证，没有起进程、按 `seed.sh` 在一次性库上实走。演示种子未改。
- **CONTEXT-MAP** 里 `transport-fulfillment → customs-compliance` 那一句写「运输履约只翻译答复并随自己的事实保存」。问询只翻译、不保存，这句对事实仍成立，所以没改；owner 若要在那句里点到问询，是一处追加。
- **进 main 冲突面**：
  - 文档令牌上 69 排最后。在途的 59、65、66、67 也在 UC-TF-005 验收表末尾与 TF CONTEXT 那一节追加行，合时按号序排。
  - `endpoints.go`、`main.go`、`endpoints_test.go` 与 9b、67 都在末尾追加，冲突只在相邻追加行。
  - `docs/adr/README.md` 的 0205 一行是行尾追加，65 若也给 0205 行追加，要合成一行。
  - `.scratch/at-coverage-inventory.md` 的 TF 行与 28、59 的号段相邻。
