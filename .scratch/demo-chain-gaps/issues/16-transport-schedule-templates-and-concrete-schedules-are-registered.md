# 16 周期安排与具体班次登记（01·一）

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg16（此前 ready-for-agent——2026-10-19 Claude Code 会话立，用户「你作为业务和系统专家自决……解决所有的问题」授权；代决见 [tf-chain-plan](../tf-chain-plan.md)）。
Blocked by: 无
地盘：TF：`domain/transport_schedule_capacity.go`、新 `schedule_template.go`、`payload_canonicalization.go`；`adapters/postgres/transport_schedule.go`、新 `schedule_template.go`、`review_catalogue.go`；`ports/catalogue_read.go`、新 `ports/transport_opportunity.go`；tfhttp 新 `register_transport_schedule*.go`、`query_transport_fulfillment_records.go`；迁移 TF 0024；新 `cmd/parcel-transport-register`；新 `cmd/parcel-api/assemble_transport_schedule.go` + `endpoints.go`/`main.go` 行；种子 `data/transport/`、`seed.sh`、README；文档 ADR-0201、TF CONTEXT、UC-TF-003、PAR-NET-08 拆分。
出处：[01](./01-transport-opportunity-has-no-production-entry.md)；方案「01 运输机会与班次」D01-2、D01-3、D01-4、D01-5、D01-9、D01-11，「跨票通则」X1、X2，「需要的 ADR」ADR-0201。

## 做什么

- TF 自有「周期安排」登记册；周期安排与具体班次走登记册配置写，CLI 与端点同一用例（方案 D01-2、D01-3、X2）。
- 班次方向身份改为 NR 连接代码加起讫节点，按 NR 拼写不透明保存；自由串方向退役（方案 D01-4）。
- 周期文法只内置一种；具体班次标识由（周期安排代码、当地运行日期）确定性派生；物化只在准备时评估候选与显式登记两处，另可登临时班次（方案 D01-9）。
- 查阅口加 `transport-schedule-template`；生产空册答 `未配置`，不建占位班次（方案 D01-5）。
- 演示种子登 SYN 周期安排，各段履约方式照方案 X1。
- 文档：起草 ADR-0201（要点见方案「需要的 ADR」）；TF CONTEXT、UC-TF-003 与 PAR-NET-08 拆分照方案「随票同步的文档」。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 周期安排、具体班次两口和 CLI 同用例。同键同内容答已存在，异内容成新版本。
- （模板、日期）物化确定且幂等（真库）。
- 自由串方向退役（表无生产行）。
- 查阅口新增 `transport-schedule-template`。
- 生产空册答 `未配置`。
- 演示：seed 后查阅口列出三条 SYN 周期安排；显式登一个班次答 201。
- 演示（接 [19](./19-load-assignment-checks-the-schedule-is-registered.md) 移交的那一项，见 19 的「评审记录」）：对上面显式登的那个物化班次提交装载分配答 201，对乱填的班次答 `SCHEDULE_NOT_FOUND`。若本票先于 19 进 main，这一项不归本票，回 19 自己走。

## 规模

L

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg16`）

### 接票时核实

票面成立：班次只有身份、一格自由串方向与出发时刻（迁移 0004），唯一写口 `PrepareTransportOpportunityHandler.EstablishSchedule` 在 cmd 下零调用，表无生产行；没有周期安排、没有登记口、没有 CLI，`data/transport/` 不存在。计划履约段只带起讫节点与窗口，窗口依据写作 `CONNECTION/<代码>@<版本>`（NR `plannedLeg`）；演示网络三条连接的代码、起讫与业务时区取自参考配置 `network-routing/network-catalog/SYN-CN-SG@1`。

### 代决（用户授权「你作为业务和系统专家自决，解决所有的问题」）

权威落在 [ADR-0201](../../../docs/adr/0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md)（本票起草：决定一至五由本票落，决定三的容量声明与容量池归 17，决定六至八归 18，ADR 里已写成决定并标明后继票）。方案没裁、本票自决的：

1. **到达只收历时**：D01-9 写「到达时刻或历时」，选历时（跨时区只有一种读法）。
2. **派生身份写作 `<周期安排代码>/<YYYY-MM-DD>`**，不用 `@`（免得与 NR 版本引用混读）；临时班次不得取这一形，读回时身份与来历必须对得上。
3. **版本号由册子按当前版加一给**，不由登记方指名；当前版是版本号最大的一行，只插不改。物化出处的周期安排版本不进班次内容摘要——按新一版物化出一模一样的班次答`已存在`。
4. **周期安排换版不改写已登记班次**；候选评估时，册上已登记的那一班按身份顶替同一（代码、日期）的物化结果，改到窗口或连接之外就不再是候选。
5. **`未配置`按连接代码判、只看当前版**：过期的周期安排也算「这条连接配过」，答`已列出`空集，由 18 记`无当前运输机会`。
6. **候选读口只按连接代码取，不按起讫筛**：起讫不符留作淘汰原因交 18。
7. **服务范围引用必填、只存不判**；演示取 `SYN-SCOPE-01`。
8. **文法的线格式**：星期 MON…SUN、当地时刻 HH:MM、运行日期 YYYY-MM-DD 一律严格认；时区拒空串与 `Local`（`time.LoadLocation` 接受它们却分别读成 UTC 与本机时区）。
9. **查阅口** `transport-schedule` 行把 `direction` 换成 `connection`/`fromNode`/`toNode`，加 `arrivesAt`、`serviceScope`、`version` 与来历三键，只列当前版；`transport-schedule-template` 也只列当前版。
10. **端点路径**取读面册名前缀加册名：`/transport-fulfillment-transport-schedule-template-registrations`、`/transport-fulfillment-transport-schedule-registrations`；具体班次一口两种形状（物化 / 临时）二选一，混着给答`未受理`。
11. **CLI** 命令 `schedule-template` / `schedule`，退出码 0 / 1 / 3，没有冲突格（同键异内容就是下一版）；输入是在线载荷类型原样嵌入再加 `tenantId`，载荷→命令只走在线口那一份 `Command`。
12. **`EstablishSchedule` 连同它的三格结果与 `SCHEDULE_STORE_UNAVAILABLE` 退役**，`PrepareTransportOpportunityDeps.Schedules` 随之摘掉；票 17 给 `EstablishPool` 核班次在册时再接 `TransportScheduleStore`。
13. **迁移 0024 用 DROP + CREATE**：表有行即 `RAISE` 拒绝施加，不猜自由串。
14. **演示时刻**：SHA→SZX 22:00 / 600 分、SZX→SIN 10:00 / 300 分（Asia/Shanghai），SIN→LM 07:00 / 90 分（Asia/Singapore），每天开；时区取连接的业务时区（演示约定，用例钉住）。窗口对不上由 18 调时刻，不改匹配规则。
15. **AT 号从 `AT-TF-100` 起**：在途票 19 已在 UC-TF-004 占了 `AT-TF-099`。

### 越权风险点（全文见 ADR-0201「越权风险点」）

方案带来的：事件放大（PAR-NFR-03）、只有按周一种形、用申报量、`未配置`不自动重备（→36）、改路缺口（→34）、连接代码跨版本相认、NR 侧车次表要录两遍、登记写授予无逐册维度、择优形态是否开放、准备结果不发信封。本票自决新增的：

1. **出发之后仍可成下一版**（TF owner）：本册不读移动事实，首个有效出发后再登新时刻照样成下一版；CONTEXT 只禁出发后取消。
2. **临时班次身份不得取派生形**（TF owner）：租户本来就那样编号的会被拒。
3. **到达只收历时**（TF owner）。
4. **查阅口班次行换键**（前端 owner）：管理台运输履约查阅页「运行方向」列在前端切片改列之前读空（`apps/admin-web` 不在本票地盘）。
5. **服务范围只存不判**（TF owner）：候选不按服务范围筛。

### 做了什么

- 迁移 `migrations/transport_fulfillment/0024_schedule_template_and_schedule_direction.sql`：班次表有行拒绝施加；班次按（租户，班次，版本）只插不改，方向三格、到达、服务范围、来历（TEMPLATE / AD_HOC 两形逐格 CHECK）；新立 `schedule_template`（星期封闭词集合、当地分钟、历时、时区、生效区间）。
- 领域：`domain/schedule_template.go`（周期安排、运行日期、星期与当地时刻的词、`MaterializeSchedule`、`MaterializeDepartingWithin`、派生身份）；`transport_schedule_capacity.go` 的班次改为方向三格加到达、服务范围、版本与来历，`FormTransportSchedule` 只立临时班次、`RehydrateTransportSchedule` 核身份与来历、`Revise` 出下一版；`payload_canonicalization.go` 换成 `REGISTER_SCHEDULE_TEMPLATE` / `REGISTER_TRANSPORT_SCHEDULE` 两形。
- 端口：新 `ports/transport_opportunity.go`（`ScheduleTemplateRegistry`、`ScheduleCandidateSource`）；`catalogue_read.go` 加 `ScheduleTemplateCatalogueRow` 与 `ListScheduleTemplates`，班次行换形。`ports.go` 未动。
- postgres：`transport_schedule.go` 按版本存取；新 `schedule_template.go`（`ScheduleTemplates`、`ScheduleCandidates`）；`review_catalogue.go` 两册只列当前版。
- 应用：新 `register_transport_schedule.go`（`RegisterTransportScheduleHandler`，两口同一用例）、`schedule_candidates.go`（`ScheduleCandidatesHandler`，`未配置` / `已列出` / 未决，评估时物化只算不存）；`prepare_transport_opportunity.go` 摘掉 `EstablishSchedule`。
- tfhttp：新 `register_transport_schedule.go`（两端点、两载荷、`OperatorRegistryIntake` 两法）；`query_transport_fulfillment_records.go` 加 `transport-schedule-template`；`unconfigured_intake.go` 补两法；`operator_registry_intake.go` 头注去计数。
- cmd：新 `cmd/parcel-api/assemble_transport_schedule.go`；`endpoints.go`、`main.go` 各加行；`unwired_orchestration.go` 补占位；新 `cmd/parcel-transport-register`。
- 种子：`scripts/demo-seeds/data/transport/` 三份；`seed.sh` 编译列表与新一节各追加；README 组成表一行、数据故事第 8 条（各段履约方式照 X1）。
- 文档：ADR-0201 与 `docs/adr/README.md` 一行；TF CONTEXT（Language「周期安排」、Rules 四条、Lifecycles 两句）；UC-TF-003 步 4、5 与 `AT-TF-100` 至 `AT-TF-104`；`PAR-NET-08` 加〔ADR-0146 拆分〕；`synthetic-demo-journey-script.md` 第 3 步补记（X1 要求写进演示文档）。

### 用例

- 领域 `schedule_template_test.go`：同一（周期安排、日期）物化两次同一班、出发按当地时刻折算、到达加历时、出处版本；星期与生效区间（终点不含）不开答 `ErrScheduleTemplateNotRunning`；文法各格坏法被拒、星期去重排序、运行日期 / 当地时刻只认一种写法；换版代码不变、同日期再物化身份不变；临时班次不得取派生形、下一版身份与来历不变；读回核身份与来历；窗口内物化只出区间内的出发。`payload_canonicalization_test.go` 钉两形文档与摘要；`transport_schedule_capacity_test.go`、`registry_zero_gates_test.go` 换新构造。
- 应用 `register_transport_schedule_test.go`（替身）：同内容已存在（星期换序带重复仍同）、异内容下一版；文法外各格未受理零写入；物化当前版、周期安排不在册 / 那天不开 / 两形混给各成一格、换版后再物化成下一版、只改生效终点再物化答已存在；两本册子读不动各自未决带续办引用；并发同内容答已存在、异内容留给重跑；候选三格（未配置、已列出空集、未决、倒置区间未受理）。
- 真库 `adapters/postgres/schedule_template_test.go`：周期安排版本累积与当前版、同版本号再落答版本已占、册面只列当前版；（模板、日期）物化确定且幂等、换版后成下一版、册面只列当前版；临时班次登记与下一版、冒用派生形与两形混给未受理；候选：生产空册答 `NOT_CONFIGURED`、物化不落册、区间外已列出空集、另一连接与他租户未配置、已登记那一班改到区间外不由物化结果顶回来；迁移 0024 之后无 `direction` 列、表有行时 0024 拒绝施加；库面 CHECK 挡坏行；两写口无事务即拒。`journey_schedule_capacity_test.go`、`review_catalogue_test.go` 换新形。
- tfhttp `register_transport_schedule_test.go`：首登 201、同内容 200、异内容 201、文法外 200 未受理；物化 201 带派生身份与出处、已存在 / 不在册 / 不开各 200、册子读不动 200 未决；载荷逐字段翻译、自报租户（含大小写变体）、解不出的时刻、未知键、尾随内容答 400，未出示令牌 401，未配置 403。`query_transport_fulfillment_records_test.go` 补新册与新行形。
- `cmd/parcel-api/assemble_transport_schedule_test.go`（真库）：经操作者渠道登记写面登周期安排 201、显式登一个班次 201、同一日期再登 200。端点探针表与登记写面列表各加两口。
- `cmd/parcel-transport-register`：用法与译装面；真库登三份演示种子（重跑已存在）、查阅册列出三条、按日期登一班退 0 答 `SCHEDULE_REGISTERED`、再登答 `EXISTING_SCHEDULE`、周期安排不在册退 1；三份种子的方向与演示网络参考配置的连接逐格相同、每条连接都有周期安排。

### 验证

- `go build ./...`、`go vet ./...`、`gofmt -l` 干净。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1`：`./internal/transportfulfillment/...`、`./cmd/parcel-api/`、`./cmd/parcel-transport-register/`、`./internal/architecture/`、`./migrations/`、`./scripts/demo-seeds/...` 全 ok（含 pbc08 写口负向证据门、接线棘轮、前缀门禁）。没跑全量。
- 一次性库 `dcg16_seed` 上 `seed.sh` 灌入退 0，末段三行 `TEMPLATE_REGISTERED`。在其上起 `parcel-api`（`127.0.0.1:19161`，操作者渠道接演示 Dex），配置员令牌：`GET /transport-fulfillment-records?registry=transport-schedule-template` 答 `SCHEDULE_TEMPLATES_LISTED` 三条；`POST /transport-fulfillment-transport-schedule-registrations` `{"template":"SYN-SCHED-SZX-SIN-DAILY","runningDate":"2026-10-21"}` 答 201 `SCHEDULE_REGISTERED`（`SYN-SCHED-SZX-SIN-DAILY/2026-10-21`，出发 `2026-10-21T02:00:00Z`），再登 200；班次册列出那一班。证据只记 `S`。走完已停进程、`DROP DATABASE … WITH (FORCE)`。
- 机制清点未重生成（进 main 时统一做）。

### 偏离与待裁

- **与票 19 的接缝**：在途 `claude-dcg19` 的测试按旧形补登班次（`FormTransportSchedule{Direction: 串}`、`CanonicalizeEstablishSchedulePayload`）。19 先进 main 时，16 rebase 要把那几处改成 `NewScheduleDirection` 加到达与服务范围的新构造（或改用 `RegisterTransportScheduleHandler` 物化一班）；19 后进则由 19 跟着改。19 读班次在册走 `TransportScheduleStore.FindByKey`，签名未变，现答当前版。
- **地盘外动到的文件**（都是追加或随改名必须跟的）：`application/prepare_transport_opportunity.go`(+test)、`unversioned_payload_digest_test.go`、tfhttp `operator_registry_intake.go`（头注）与 `unconfigured_intake.go`、`cmd/parcel-api/unwired_orchestration.go`、`endpoints_test.go`、`assemble_operator_decisions_test.go`（列表各加两行）、`docs/design/synthetic-demo-journey-script.md`。未碰 peer 地盘与 `apps/admin-web`。
- **本票不接生产调用方的**：`application.ScheduleCandidatesHandler` 与 `postgres.ScheduleCandidates` 是票 18 准备编排要调的候选来源，本票只以真库用例证其答格。
- **管理台**：班次区「运行方向」列读空、没有周期安排区，归前端切片（方案「等 peer 的」admin-web 一行）。
- **演示窗口**：三份周期安排的时刻能否落进计划段窗口，等票 18 实测后调。

