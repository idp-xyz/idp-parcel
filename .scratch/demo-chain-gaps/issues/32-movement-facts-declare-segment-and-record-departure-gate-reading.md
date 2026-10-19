# 32 移动事实声明段与出发门禁读数

Category: enhancement
Status: resolved——2026-10-19 进 main（全量钉 `98affcab`：148 ok / 0 FAIL；TF 链第二波同批，见文末「进 main 记录」）。此前 in-progress——评审修订完，待进 main，分支 claude-dcg32。2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md) 与 [ADR-0205](../../../docs/adr/0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md)。
Blocked by: [31](./31-customs-action-admission-judgement-and-node-customs-zones.md)；[operator-channel/48](../../operator-channel/issues/48-cmd-parcel-api-tests-and-comments-say-what-the-code-does.md)（同一测试文件）
地盘：TF `domain/load_assignment_movement.go`、`movement_fact_rehydration.go`、`payload_canonicalization.go`、新 `departure_gate_reading.go`；`application/record_movement_fact.go`；新 `ports/departure_gate.go`；新 `adapters/customscompliance/departure_gate.go`；`postgres/movement_fact_registry.go`；迁移 TF 0026；tfhttp `record_movement_fact.go`；`cmd/parcel-api/assemble_movement_fact.go`(+_test)、`movement_fact_same_identity_test.go`、`transport_fulfillment_command_lines_test.go`、`storable_values_channel_test.go`；ADR-0205、TF CONTEXT、UC-TF-005。
出处：[05](./05-movement-facts-are-unlinked-and-departure-gate-is-self-reported.md)；方案「05 移动事实与出发门禁」D05-1、D05-2、D05-6、D05-7、D05-8、D05-9，「跨票通则」X4、X5，「需要的 ADR」ADR-0205，「已不成立或已修」05。

## 做什么

- 出发一律落册，同一笔存关务门禁读数（封闭集照方案 D05-1）；退役「`DEPARTURE_GATE_BLOCKED`、不落库」那一格（方案 D05-1、X5）。
- TF 消费侧端口 `DepartureGateView` 在写事务开始前同步问 [31](./31-customs-action-admission-judgement-and-node-customs-zones.md) 的准入判断，TF 只翻译（方案 D05-2、X4）。
- 移动事实带登记方声明的段引用，不加外键；对象取段的当前参与；段不在册也照记（方案 D05-6）。不核班次在册（方案 D05-9）。
- 设备的 `gateRequired` 失去权威但字段暂留，`gateClearance` 只作证据（方案 D05-8）。
- 外包干线的跨境门禁不在本票（方案 D05-7）；不改 `work_fact_intake*.go`、`assemble_operator_decisions.go`（方案「等 peer 的」）。
- 起草 ADR-0205（要点见方案「需要的 ADR」）；TF CONTEXT Rules 补门禁读数；UC-TF-005。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 出发一律落库。
- 五格读数与事实同一笔（真库）。
- CC 读不动也落库，记 `读不动`。
- `gateRequired=false` 照样问 CC。
- 段不在册照记。
- 不改 `work_fact_intake*.go`、`assemble_operator_decisions.go`。
- 演示：自营 SHA→SZX 出发答 201，读数 `不受门禁`。

## 规模

L

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg32`）

### 接票时核实

- 出处 05 的门禁那一半逐条成立：`gateRequired` 为真而没带 `gateClearance` 时编排答 `DEPARTURE_GATE_BLOCKED`、不落库；`gateRequired=false` 就没有门；移动事实只挂班次，`domain.RecordMovementFact` 不读段也不读对象。
- 票 31 立的准入判断（ADR-0206）在 `cmd/parcel-api/assemble_customs_admission.go` 装好、没有生产消费者；它自带事务、在调用方事务里调用即拒，读不动时回滚并指名哪一口——正好是本票要的接口形状。
- 段上没有起讫节点；班次自票 16 起按 NR 连接代码加起讫节点登方向，关务节点区域也按同一套节点代码登。移动事实的「地点」是自由串，不一定是网络节点。
- 一线入口（peer 的 `work_fact_intake.go`）把载荷原样交给本票地盘里的 `MovementFactPayload.Command`，所以载荷加可缺的 `segment`、`resource` 不必改 peer 文件；它的测试钉着 `GateRequired` 与 `GateClearance` 透传，两格保留。TF 作业事实只认 `OFFSITE` 作业范围。

### 代决（用户授权「你作为业务和系统专家自决……解决所有的问题」）

权威落在 ADR-0205。这里逐条记方案没裁、由本票自决的几处，各带它自己的越权风险点；进了 ADR 的写明是 ADR-0205 越权风险点几，只在票面的是装配细节（代决 12）。

1. **起讫取班次当前版的方向，不取设备报的地点或去向。** 方案只说「出发：自 A 到 B」。地点不一定是网络节点；起讫若由设备自报，设备就能报一个同区去向关掉跨区那道门。
   - 越权风险点（TF owner）：班次不在册即答未决、不问关务；出发之后班次登了换方向的下一版，读数取的是登记那一刻的当前版（ADR-0205 越权风险点 4）。
2. **对象取段里每个对象链尾那一版参与，且出发时点落在它的控制起止之内。** 方案原话「取段的当前参与」；按此刻取会让迟到的出发问不到已在下一站交接离场的那几件。
   - 越权风险点：见 ADR-0205 越权风险点 10（TF owner）。
3. **关务的`未配置`落`未决`，五格不加第六格。** 方案五格里没有未配置；对已发生的出发，缺配置与缺案卷都是「此刻判不出」，依据原文的种类仍说得出缺的是配置。
   - 越权风险点：见 ADR-0205 越权风险点 7（CC、TF owner）。
4. **整条出发按压制次序取最重（次序只在 ADR-0205 决定二）。** 读不动压过未决是本票的取舍：有一件根本没问到，比「问到了、关务说还缺东西」说得更少。
   - 越权风险点：见 ADR-0205 越权风险点 9（TF、VE owner）。
5. **本侧停点不问关务。** 段未声明、段不在册、出发那一刻段里没有在控对象、班次不在册答未决；段册、班次册读不动答读不动。载荷不带 `segment` 也照记。
   - 越权风险点（TF owner、一线客户端）：一线客户端今天不报 `segment`，经一线出发会一律停在「段未声明」答未决，读数等于没问（ADR-0205 越权风险点 6）。
6. **一件读不动不停下，其余照问。** 调用失败记读不动、依据种类 `CUSTOMS_ADMISSION_UNAVAILABLE`；答复译不回来记读不动、依据种类 `CUSTOMS_ADMISSION_UNTRANSLATABLE`（评审后分开）；关务自己答读不动时依据种类取关务指名的那一口。端口答非所问（答了另一件、零值不报错）上抛成 500。
   - 越权风险点：见 ADR-0205 越权风险点 2（TF owner、运维）。生产装配有真库用例钉住「答得出不受门禁」，接错时那条会红。
7. **读数不进内容摘要；同内容重放回显首次读数，不另记、不重问关务。** 方案只说「与事实同一笔」。不重问是评审后补的：装配点先用 `RecordedDepartureReading` 从池上认重投。
   - 越权风险点（TF owner）：重放时关务此刻已阻断，设备收到的仍是首次的读数；补问机制没有（ADR-0205 越权风险点 2）。
8. **存量出发由迁移回填。** 0026 之前落册的出发回填为未决、停点「登记于门禁读数之前」；新登记的出发构造门拒这一格。表本是只插不改，这是一次只补新列、不动事实列的形状回填。
   - 越权风险点：见 ADR-0205 越权风险点 11（TF owner）。
9. **资源引用一并加上，段与资源都进内容摘要；加格不换 TFC 版本。** 方案 D05-6 写「资源引用可选」。这一口不存摘要、两侧现算，没有已存摘要要按旧形状回放。
   - 越权风险点：见 ADR-0205 越权风险点 12（TF owner）。
10. **HTTP 响应逐对象只列对象与那一格；关务判断标识与依据原文只落册。** 执行方要知道车上被挡的是哪一件；限制标识、放行来源等是别的客户的关务信息，按 UC-TF-005 最小范围不回给设备（评审后收窄，ADR-0205 决定一）。
    - 越权风险点：见 ADR-0205 越权风险点 13（TF owner）。
11. **更正沿用原读数。** 移动事实的更正没有生产入口，领域的更正版带着原登记那一刻的读数。
    - 越权风险点：见 ADR-0205 越权风险点 8（TF owner）。
12. **关务那一口在 `buildMovementFactOrchestration` 里另装一份。** 不从 main 的 `customsAdmission` 传进来，免得改 main 既有几行的次序。
    - 越权风险点（运维）：进程里多一组无状态的关务适配器对象，接的是同一个库。

### 做了什么

- 领域：
  - 新文件 `domain/departure_gate_reading.go`：五格、本侧停点（各属一格）、逐对象那一格（`NewDepartureGateFinding`）、`FormDepartureGateReading` 与重建门 `RehydrateDepartureGateReading`（核整格与明细一致）、段的 `ObjectsUnderControlAt`。
  - `load_assignment_movement.go`：`TransportResourceReference`；`MovementFactSpec` 去掉 `GateRequired`、加 `Segment`、`Resource`、`GateReading`；`TransportMovementFact` 带三格；出发必带读数、非出发不带、读数与声明的段对得上，否则 `ErrDepartureGateReadingMisplaced`；`ErrDepartureGateBlocked` 退役。
  - `movement_fact_rehydration.go` 随之；`payload_canonicalization.go` 内容摘要加段与资源。
- 应用：新文件 `application/read_departure_gate.go`（`DepartureGateReader`）；`record_movement_fact.go` 的 `Record` 多一个读数参数，`MovementFactGateBlocked` 退役，命令加 `Segment`、`Resource`，`GateRequired` 收而不读。
- 端口：新文件 `ports/departure_gate.go`（`DepartureSegmentView`、`DepartureGateQuery`、`DepartureGateView`）。
- 适配器：
  - 新包 `adapters/customscompliance`（`DepartureGateView`，TF→CC 消费侧，只翻译、只带回同格依据）。
  - `postgres/movement_fact_registry.go` 读写段、资源、读数整格与 jsonb 明细。
  - 迁移 `migrations/transport_fulfillment/0026_movement_fact_segment_and_departure_gate_reading.sql`：四列、存量回填、CHECK（读数只在出发上、整格封闭、明细同在同缺、停点与逐对象恰有其一、段与资源非空串）。
  - tfhttp `record_movement_fact.go`：载荷加 `segment`、`resource`，响应加 `segment`、`resource`、`gateReading`。
- 装配：`cmd/parcel-api/assemble_movement_fact.go` 的事务壳先在写事务之外问读数、再开写事务；`buildMovementFactOrchestration` 接段册、班次册与关务准入判断。`assemble_customs_admission.go` 只改一句头注（生产消费者已接上）。`main.go`、`endpoints.go` 未动。
- 文档：ADR-0205；`docs/adr/README.md` 按号插一行；TF CONTEXT「出发门禁读数」词条与「实际移动与出发门禁读数」一节；GLOSSARY 同名一条；UC-TF-005 输入、结果、流程第 6 步、规则、一致性、试点叠加条件与 `AT-TF-110`–`116`；演示脚本第 5 步补记一条；AT 覆盖盘点七条入①。

### 用例

- 领域 `departure_gate_reading_test.go`：多对象整格取最重（逐对两两）；停点各属一格、读不动压过未决、停点与逐对象恰有其一、起讫与重复对象的形状门；重建门核整格、收存量回填；逐对象那一格的标识与依据门；`ObjectsUnderControlAt` 取出发那一刻在控的链尾参与（离场的、后入场的、被替代的、失效的都不取）。
- 领域 `load_assignment_movement_test.go`：出发一律立得成（阻断照记）、凭据只作证据、读数放错地方是接线错误、本体不成形先答未受理、移动与到达不带凭据与读数。
- 应用 `read_departure_gate_test.go`：逐对象问、起讫取班次方向、时点取业务时间、`gateRequired=false` 照问；关务调用失败那一件读不动、其余照问；本侧各停点不问关务；非出发与受理不了的不读；答非所问上抛、缺口构造即拒。
- 应用 `record_movement_fact_test.go`：五格都落册；设备字段不读、凭据保全；读数放错地方上抛；重放回显首次读数、查不了册答未决；换段、换资源是内容冲突；段或资源含 NUL 不受理。
- tfhttp `record_movement_fact_test.go`：阻断照 201 并透出整格、起讫与逐对象那一格（评审后：不带关务判断标识与依据）；没声明段答未决停点；载荷两格逐字到命令。
- 适配器 `adapters/customscompliance`：对照表与同格依据（内部用例）；真库上同区不受门禁、跨区未决、阻断带限制标识、未配置落未决；问不出答复只交 error。
- 真库 `postgres/movement_fact_registry_test.go`：出发连读数往返（逐对象、依据、读不动那一件）；停点读数往返；迁移文件里原文的回填语句把一行迁移前形状的出发装回为「登记于门禁读数之前」、到达不回填；0026 各道 CHECK。
- 装配 `cmd/parcel-api/assemble_movement_fact_test.go`：
  - 不声明段、班次不在册的出发照记未决，读数与事实同一行；
  - 演示判据：按种子原样登四个节点区域与周期安排、显式登班次、交接立段，经作业事实渠道报出发答 201、读数不受门禁；
  - 五格各落一条（不受门禁、放行、阻断、段不在册的未决、关务限制册读不到的读不动），每条答复与库面同一份；
  - 读数里的关务判断标识在关务判断册里查得到。
- `movement_fact_same_identity_test.go` 的撞键输家沿用生产装配的读数编排。

### 验证

- 于 `fed4ad57`：`go build ./...`、`go vet ./...`、`gofmt -l`（全部已跟踪 Go 文件）干净。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1` 全绿：`./internal/architecture/`（接线棘轮、类型可达、前缀、事务闭包、边界）、`./internal/transportfulfillment/...`、`./internal/customscompliance/...`、`./cmd/parcel-api/`、`./migrations/`、`./scripts/demo-seeds/...`。
- 演示种子未改，没有重灌；演示判据由生产装配上的真库用例取证（只记 `S`），没有起进程按演示脚本重走。
- 未跑全量，未重生成机制清点。
- 续做复核（作者被用量上限中断后接手，工作树干净、五笔提交齐全，未另改代码）：于 `78d8c216` 重跑 `go build ./...`、`go vet`、`gofmt -l` 与上列 `./internal/architecture/`、`./internal/transportfulfillment/...`、`./internal/customscompliance/...`、`./cmd/parcel-api/`、`./migrations/`，全绿。

### 偏离与待裁

- **地盘外的改动**：`assemble_customs_admission.go` 一句头注（票 31 的文件，「今天只有装配测试在用」随本票过时）；GLOSSARY 一条；演示脚本一条补记；AT 覆盖盘点。没碰 `work_fact_intake*.go`、`assemble_operator_decisions.go`、`main.go`、`endpoints.go`。
- **号**：迁移 TF `0026` 照票面；AT 号评审后预先改为 `AT-TF-110`–`116`（见评审记录），进 main 时仍按 `AT-TF-1(0[5-9]|1[0-9])` 全仓 grep 复核。
- **合并预检**：对 `origin/main`（`8e1307d6`）`git merge-tree` 只有 `.scratch/at-coverage-inventory.md` 一处冲突（计数表与追记，与 no-reception-source/03、/04 的追记九、十并行），按惯例两边都留、计数重算。`docs/adr/README.md` 自动合入。对 `1167b4d9` 重做一次，结果相同。
- **发现但没改**：`docs/adr/README.md` 在 main 上有两行 ADR-0206（前一行是评审前的措辞），属票 31 合批的遗留，热点文件只追加，留给协调方删旧行。
- **文档令牌**：方案的文档令牌次序是 16 → 23 → 20 → 25 → 32 → 27，本票先于 23、20、25 完成；TF CONTEXT 与 UC-TF-005 都是追加段落与表行，进 main 时按令牌 rebase。
- **后续**（方案「等 peer 的」与票 33）：
  - 一线客户端报 `segment`；
  - 删命令与载荷里的 `gateRequired` / `gateClearance`，设备出发前读口；
  - 移动事实信封与 VE 投影（票 33，阻断读数派生异常）；
  - 外包干线交接与有效交付的门禁按交出、交付意图读准入判断。

## 评审记录（2026-10-19，隔离评审于 `d9bd373e`，作者子代理修订）

阻断 0 项。非阻断 8 项，其中评审建议进 main 前处理的 3 项全修；其余 5 项修 4 项，1 项部分修（缺口登进 ADR-0205）。

### 修了什么

1. **设备答复暴露同段其他客户的关务信息**（建议进 main 前）。采纳评审的首选修法：
   - 回给设备的 `gateReading.objects[]` 只留 `object` 与 `answer`，去掉 `judgement` 与 `bases`；判断标识与依据原文照旧整份落册。
   - ADR-0205 决定一补一条答复范围（引 UC-TF-005 最小范围与 `AT-TF-063`），越权风险点 13 记「执行方仍看得到同段各件那一格」。
   - UC-TF-005 规则不需例外指针，代码与规则不再相左。
   - 票面去掉「ADR 越权风险点是全集」的说法：代决 2、6、8、9、10 的风险点并进 ADR-0205（越权风险点 10–13，以及 2 的补句）；只有代决 12（装配里多一组关务适配器对象）留在票面，它是装配细节。
2. **AT 号与同波票撞号**（建议进 main 前）。17 用 105–106，23 用 105–107，27 自取 120 起。17、23 先进 main 时合起来占 105–109，所以本票预先改为 110–116，进 main 时不必再改号。一次改完：
   - UC-TF-005 表、ADR-0205 Links、各 Go 测试的 `Covers:`、AT 盘点的行与追记、本票；
   - 改完 `git grep -E "AT-TF-1(0[5-9])\b"` 在分支上为空。
   - 盘点的计数表与追记仍和 main 有文本冲突，进 main 时两边都留、计数重算（同前）。
3. **袋或集运单元在境内出发读成不受门禁**（建议进 main 前）。
   - ADR-0205 越权风险点 5 写明后果：这是一格假的干净读数，读数对集运单元不是干净信号。
   - 本上下文的载运对象引用不分包裹与集运单元，此刻判不出「这一件是袋」，所以代码上没有先改记未决。
   - 成员展开另立票 [42](./42-departure-gate-reading-does-not-expand-consolidation-unit-members.md)。号取 42：同波票 17、23 各自立了一张 40，进 main 时其中一张会改 41。
4. **读不动吞掉错误、分不出暂时与确定性**。
   - 关务一侧分开：端口新哨兵 `ErrDepartureGateAnswerUntranslatable`，适配器的 `ErrUntranslatableAnswer` 包住它。逐对象那一格立不起来也归此类。编排把译不回来记成依据种类 `CUSTOMS_ADMISSION_UNTRANSLATABLE`，与调用失败的 `CUSTOMS_ADMISSION_UNAVAILABLE` 分开；ADR-0205 决定二随改。
   - 本侧没分：段册、班次册的装不回与连不上库仍同记该册读不动。理由与去向见下文「没采纳什么」。
5. **重投每次逐件问关务**。新增 `RecordMovementFactHandler.RecordedDepartureReading`：
   - 装配点在问读数之前从池上按键查册。同内容的出发重投直接取在册读数，不问关务；认不出时照常问，答复仍由 `Record` 在写事务里定。
   - 交回的读数只会被重放那一支丢弃，册只插不改，不会被当成新读数存下。
   - tfhttp 测试里仿组合根的 `readThenRecord` 同步改了次序。
   - ADR-0205 决定六补一条。
   - 用例：应用层 `TestAReplayedDepartureHandsBackItsRecordedReadingBeforeCustomsIsAsked`；真库 `TestAReplayedDepartureDoesNotAskCustomsAgain`，用计数的关务口包住生产适配器。拿掉短路后后一条会红（实测：「同内容重投又问了关务：共 2 次」）。
6. **注释叙述变更、跨文件计数**。
   - `movement_fact_rehydration.go`、`payload_canonicalization.go` 的「已不决定」改为现在时理由。
   - tfhttp 测试的「不再挡」改为「也照记：设门与否不由设备说」。
   - 「关务五格」改指 `ccdomain.AdmissionAnswer`，适配器与它的内部测试各一处。
7. **压制次序写了三处**。
   - UC-TF-005 那一行改引「压制次序见 ADR-0205 决定二」。
   - `DepartureGateAnswer` 的注释只留「常量次序即压制次序，次序与理由只在 ADR-0205 决定二」。
   - `docs/adr/README.md` 本票那一行同样只引决定二（这一行是本票新加的，不是既有行）。
8. **TF CONTEXT 关系一节仍说本上下文「执行限制」**。那一句后补：执行在动作之前那一问；已发生的出发登记时只记录门禁读数、不拒收，并指向「实际移动与出发门禁读数」一节。

### 没采纳什么及理由

- **装配点打日志**（第 4 项的另一种修法）：没做。
  - 要把 logger 传进 `buildMovementFactOrchestration`，就得改 `main.go` 里既有的调用行。`main.go` 是热点文件，只许追加行。`cmd/parcel-api` 也还没有在请求路径上打日志的先例。
  - 本侧段册、班次册的确定性故障（某行装不回）因此仍与连不上库同记该册读不动。框架的读口也没有「装不回」的哨兵可认。
  - 去向：已写进 ADR-0205 越权风险点 2，补问机制落地前要先把这几类分开。
- **关务那一问改成批量**（第 5 项的另一种修法）：没做。要改 CC 的编排与那层自带事务的壳，超出本票地盘。重投短路已经去掉了重放上的 N 笔判断；首登仍是逐件串行，大段的首登延迟留给 owner 看实测再定。

### 修订后的验证

- 于修订提交之上：`go build ./...`；`go vet`（TF、CC、`cmd/parcel-api`、architecture）；对改动的 Go 文件跑 `gofmt -l`。三项都干净。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1` 全绿，覆盖这几个包：
  - `./internal/architecture/`
  - `./internal/transportfulfillment/...`
  - `./internal/customscompliance/...`
  - `./migrations/`
  - `./cmd/parcel-api/`
- 演示种子未改，没有重灌。
- 合并预检：对 main `08254c8c` 跑 `git merge-tree`，只有 `.scratch/at-coverage-inventory.md` 一处冲突，`docs/adr/README.md` 自动合入。

## 进 main 记录（2026-10-19）

- TF 链第二波 17、23、27、32 与 first-party-api/00 合成一批，按 17 → 23 → 27 → 32 → first-party-api/00 叠到 main 上，整批快进推送，与本记录同一次推送。
- 合批时的处理：
  - README 的 ADR 行按号合并。
  - 决定种类：23 与 27 各加一种，枚举与测试两边都留。access_identity 迁移 0008、0009 与另一会话的 0011 三份 CHECK 全集一致（15 种）。
  - `endpoints.go`、`main.go`、`unwired_orchestration.go`、演示种子 JSON 与 `seed.sh` 的追加行两边都留。
  - 23 与 27 在 TF application 包里同名：27 的协议读不动一格改名 `ChargeSupplierAgreementUnavailable`，测试替身加 `charge` 前缀。
  - UC-TF-005 的 AT 表按号排：32 的 110–116，27 的 120–125。
  - AT 覆盖盘点计数表按两侧增量重算。
  - 种子 README 文件表 `operator-channel` 一行去重。
  - 23 立的 40 号票与 17 立的 40 号票撞号，23 那张改为 41，ADR-0202 风险点 11 同步。
- 合批种子在一次性新库上 `seed.sh` 灌通，退 0，用完删库。
- 全量 `go test -p 1 -count=1 ./...`（带 55432 DSN）在干净游离检出上钉 `98affcab`：148 包 ok，0 FAIL，0 SKIP。机制清点在同一 tip 上重生成。
