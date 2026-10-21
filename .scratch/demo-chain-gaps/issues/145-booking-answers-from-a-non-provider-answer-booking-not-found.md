# 145 非本服务方的承运商答复与订舱不存在分两格作答，泄露一位信息

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-dcg145。2026-10-21 认领；此前 ready-for-agent——2026-10-20 代决复核立（上线前必修），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 R9；ADR-0202 越权风险点 14

## 现象与修法

现象、证据与修法的完整写法以复核报告 R9 一条为准（单一权威），此处只记要点：

代决「否」：一律答 `BOOKING_NOT_FOUND`；须在首版集成契约发布前落地，并在 ADR-0202 补复核记录说明这一格的停用。

## 完成记录（2026-10-21，Claude Code 作者子代理，分支 claude-dcg145）

裁定的单一权威是代决复核报告 R9；决定写成 [ADR-0239](../../../docs/adr/0239-a-carrier-answer-from-a-non-provider-is-answered-booking-not-found-and-the-basis-is-kept-on-a-bounce-register.md)。它部分停用 ADR-0202 文末补记「承运答复的次序与格」里 `SOURCE_NOT_SERVICE_PROVIDER` 那一格，ADR-0202 只在 Status 行与 Links 加前向指针。

### 做了什么

- **对外一格**（ADR-0239 决定一）：承运答复口对「答复方不是订舱所属委托的服务提供方」答 `200` `BOOKING_NOT_FOUND`，与订舱不在册同一格，响应体逐字相同（`{"outcome":"BOOKING_NOT_FOUND"}`）。应用层结果去掉 `BookingSourceNotProvider`，`SOURCE_NOT_SERVICE_PROVIDER` 退出线格式。来源核对仍排在重放之前。
- **对内分清**（决定二）：迁移 transport_fulfillment `0050` 立承运答复挡回册 `booking_answer_bounce`。
  - （租户，答复方来源身份，订舱，答复标识，依据）一行；依据 `BOOKING_NOT_REGISTERED` / `SOURCE_NOT_SERVICE_PROVIDER`；记首末挡回时刻与次数，同键再来只加次数。
  - 端口 `BookingAnswerBounceLedger`，PostgreSQL 实现 `BookingAnswerBounces`。
  - 编排 `bounceAnswer` 与那一次答复同笔写；写不动答未决 `ANSWER_STORE_UNAVAILABLE`、整笔回滚。
- **两条挡回路一样多的事**（决定三）：端口 `BookingWithProviderReader`、实现 `BookingRequests.FindBookingWithProvider`，一条语句左连接读回订舱与委托的服务提供方。两条挡回路都是一读一写；读不动同答 `BOOKING_STORE_UNAVAILABLE`。答复口不再单读委托。`FindByKey` 与它共用一份行重建。
- **装配**：`cmd/parcel-api/assemble_transport_commission.go` 的 `buildBookingFaces` 接上两口；没碰 endpoints/main/assemble/routes。
- **文档**：
  - ADR-0239，ADR-0202 前向指针；
  - UC-TF-004 改写 `AT-TF-149`、`AT-TF-153` 的结果，增 `AT-TF-326`–`328`；
  - 演示种子 README 第 4 步、演示动线补记；
  - AT 盘点①加三行与追记（总览不动）；
  - 票 144 补注一节；立后继票 [167](./167-booking-answer-bounce-register-has-no-read-face.md)。

### 代决（用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」与常设指令「你作为业务和系统专家自决……解决所有的问题」）

决定层面的取舍写在 ADR-0239，下面只列结论与越权风险点；实施层面另几处 ADR 里没有，列在后面。

1. **合并做在应用层，不只在 HTTP 层译名**：编排结果里不留区分位，后来接这只编排的适配器无从泄露。
   - 越权风险点：区分只在挡回册，运营今天只能经库读（ADR-0239 风险点 1，去处票 167）。
2. **内部留痕落库，不写日志**：照 ADR-0230 派送待形成册的台账先例。parcel-api 不记请求日志，日志也不是审计依据。
   - 越权风险点：册没有保留期与清扫（风险点 4）；被挡回报文的内容不留（风险点 6）。
3. **同键计次，不一次一行**：键含答复标识与依据。
   - 越权风险点：探测者每试一个新标识多一行（风险点 4）。
4. **册写不动答未决，不答订舱不在册**：未决原因借答复册的名 `ANSWER_STORE_UNAVAILABLE`。
   - 越权风险点：名字是借的（风险点 5）。
5. **时差处理到「同一条语句一读一写」为止**：不加人为延时，不做常数时间。
   - 越权风险点：剩余微秒级差异按统计仍可能分得开（风险点 2）。
6. **订舱在册而委托读不回仍答 500、不记挡回**：照 ADR-0202 补记。
   - 越权风险点：这一格对任何答复方暴露订舱存在（风险点 3），只在库被绕过编排改坏时出现。
7. **`AT-TF-149` 原地改写结果，不退号另立**：场景没变，变的是结果；新行为另立 `AT-TF-326`–`328`。
   - 越权风险点：读旧号的人要按新结果读。149 与 153 不在 AT 盘点里（归票 139），本票没替它们加行。
8. **TF CONTEXT 不加词条**：挡回册是答复口的审计台账，不是领域语言；ADR-0202 的来源核对本身也没进 CONTEXT。
   - 越权风险点：owner 若把「承运答复挡回」当领域语言，补一条 Language 词条并改 UC 引用。
9. **ADR-0202 文末「复核记录」R9 那一行不改**：它已写「那一格的停用随该票的 ADR 记」。停用照「部分停用」写法记在 ADR-0202 的 Status 行与 Links。
   - 越权风险点：票面原写「在 ADR-0202 补复核记录说明这一格的停用」，本票读作「已有那一行加前向指针」，没在正文里另起一段。
10. **演示只由真库装配用例证，没在一次性库上经演示 Dex 实走**：证据只记 `S`；动线补记照写，并注明这一点。
    - 越权风险点：演示环境里那一答没有人实走过。
11. **票 144 加补注、不改它的修法**：集成说明那份文件不存在，没进 main；补注只告诉 144 的作者这一格的续办怎么写。
    - 越权风险点：144 若有人在途，补注一节可能撞车。按协调方的在途清单，144 不在途。

### 用例

- 编排 `internal/transportfulfillment/application/booking_schedule_and_answer_source_test.go`：
  - `TestOnlyTheCommissionedProviderAnswersItsBooking`（149）
  - `TestANonProviderAndAnAbsentBookingAreAnsweredAlike`（326）：三条挡回路结果 `reflect.DeepEqual` 相同；各一读一写、不碰答复册；读不动与册写不动各答同一原因。
  - `TestTheBounceLedgerTellsTheTwoBasesApart`（327）
  - `TestABouncedAnswerIsTakenOnceTheBookingIsEntered`（328）
  - `TestAnAnswerRecoveryDiscipline`（153 与恢复面）：数据违约不记挡回。
- 线格式 `internal/transportfulfillment/adapters/http/booking_faces_test.go::TestTheBookingAnswerFaceAnswersOnTheWire`：别的承运商答在册订舱，与服务方答不在册订舱，状态与响应体逐字节相同；服务方答过之后别人原样重放也一样。
- 存取（真库）`internal/transportfulfillment/adapters/postgres/booking_answer_bounce_test.go`：
  - `TestABookingIsReadBackWithItsCommissionsProviderInOneStatement`：在册、不在册、他租户、委托读不回四格；连读交回的订舱与单读相同。
  - `TestABouncedAnswerIsCountedOnItsOwnRowPerBasis`：计次、首末时刻、依据分行、无事务拒（`ErrTransactionRequired`）、回滚不留、封闭集外与空来源先拒。
- 生产装配（真库）`cmd/parcel-api/assemble_booking_faces_test.go`：
  - `TestTheWiredBookingAnswerFaceAnswersANonProviderAsBookingNotFoundAndKeepsTheBasis`（326–328）：末端承运客户端答干线订舱，末端与干线各答一个不在册订舱，三次响应逐字节相同；挡回册三行依据分清，重交计 2；答复册零行；录入订舱后干线重交 201、再交 200 `EXISTING_ANSWER`，挡回册不变。
  - `TestTheWiredBookingAnswerFaceAnswersUndecidedWhenTheBounceCannotBeKept`（327 末支）：挡回册改表名写不进，答 200 `COMMISSION_UNDECIDED` / `ANSWER_STORE_UNAVAILABLE`，两册零行。
  - 原 `TestTheWiredBookingFacesBookTheSeededTrunkScheduleAndTakeTheCarriersAnswer` 那一答改为 `BOOKING_NOT_FOUND`。

### 验证

- 钉 `9ec426ab`，此后只改文档与票面：
  - `go build ./...`、`go vet ./...` 干净，变更的 Go 文件 `gofmt -l` 干净。
  - 每一笔提交都经 `git archive` 导出到 scratchpad，单独 `go build ./...`、`go vet ./...` 通过。
- `scripts/test-full.sh` 跑了以下 26 个包，全绿：
  - `./internal/transportfulfillment/...`（12 个有测试的包）；
  - 反向依赖 `cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-transport-register`，以及 NR、NO、PP、PS、SA、VE 各自的 `adapters/transportfulfillment`；
  - `./internal/architecture/...`、`./migrations/...`、`./tools/...`。
- 中途一次 `internal/architecture` 红过：持久化写口门禁要求挡回册写口有引用 `ErrTransactionRequired` 的负向断言，补在 `2a8973a7`。
- 变异检验：让 `bounceAnswer` 对「不是服务提供方」不写册，编排三条用例与真库装配用例即红；换回后绿。
- ADR 索引与 AT 计数两个工具只在 scratchpad 副本上试跑，确认解析得了：ADR-0239 与 ADR-0202 的索引注记各一处；AT 盘点 ①、TF、合计各 +3。仓里没重生成。

### 没做与去向

- 挡回册查阅面：后继票 [167](./167-booking-answer-bounce-register-has-no-read-face.md)，与票 48 同一车道。
- 首版集成说明（票 144）：还没进 main，补注已写，集成说明随 144 写。
- 迁移号 `0050` 是暂定：收口前 `git rebase main`，届时取 main 上当时的下一个空号，并同步迁移文件头、ADR-0239 与本记录里的号。
- 进 main 时由协调方重算 AT 盘点总览、重生成 ADR 索引与机制清点。
