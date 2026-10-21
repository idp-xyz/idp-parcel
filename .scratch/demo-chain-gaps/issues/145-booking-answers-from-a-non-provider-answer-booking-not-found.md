# 145 非本服务方的承运商答复与订舱不存在分两格作答，泄露一位信息

Category: bug
Status: resolved——2026-10-21 进 main（全量钉 `7278bf09`：`scripts/test-full.sh` 158 ok / 0 FAIL，见文末「进 main 记录」）；挡回册查阅面转票 [167](./167-booking-answer-bounce-register-has-no-read-face.md)，保留期与聚合转票 [170](./170-booking-answer-bounce-register-has-no-retention-or-aggregation.md)。此前 in-progress——二轮评审无阻断，二轮修订已做，待进 main，分支 claude-dcg145（基于 main `b290b777`）。2026-10-21 认领；此前 ready-for-agent——2026-10-20 代决复核立（上线前必修），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 R9；ADR-0202 越权风险点 14

## 现象与修法

现象、证据与修法的完整写法以复核报告 R9 一条为准（单一权威），此处只记要点：

代决「否」：一律答 `BOOKING_NOT_FOUND`；须在首版集成契约发布前落地，并在 ADR-0202 补复核记录说明这一格的停用。

## 完成记录（2026-10-21，Claude Code 作者子代理，分支 claude-dcg145；评审 B1 与二轮修订后改定）

裁定的单一权威是代决复核报告 R9；决定写成 [ADR-0239](../../../docs/adr/0239-a-carrier-answer-from-a-non-provider-is-answered-booking-not-found-and-the-basis-is-kept-on-a-bounce-register.md)。它部分停用 ADR-0202 文末补记「承运答复的次序与格」里 `SOURCE_NOT_SERVICE_PROVIDER` 那一格，ADR-0202 只在 Status 行与 Links 加前向指针。

### 做了什么

- **对外一格**（ADR-0239 决定一）：承运答复口对「答复方不是订舱所属委托的服务提供方」答 `200` `BOOKING_NOT_FOUND`，与订舱不在册同一格，响应体逐字相同（`{"outcome":"BOOKING_NOT_FOUND"}`）。应用层结果去掉 `BookingSourceNotProvider`，`SOURCE_NOT_SERVICE_PROVIDER` 退出线格式。来源核对仍排在重放之前。
- **对内分清**（决定二）：迁移 transport_fulfillment `0047` 立承运答复挡回册 `booking_answer_bounce`。
  - （租户，答复方来源身份，订舱，答复标识，依据）一行；依据两值（`BOOKING_NOT_REGISTERED` / `SOURCE_NOT_SERVICE_PROVIDER`）库里存 smallint 码 1、2；记首末挡回时刻与次数，同键再来只加次数。
  - 端口 `BookingAnswerBounceLedger`，PostgreSQL 实现 `BookingAnswerBounces`。
  - 编排 `bounceAnswer` 与那一次答复同笔写；写不动答未决 `ANSWER_STORE_UNAVAILABLE`、整笔回滚。
- **两条挡回路一样多的事**（决定三）：
  - 端口 `BookingWithProviderReader`、实现 `BookingRequests.FindBookingWithProvider`，一条语句左连接读回订舱与委托的服务提供方；`FindByKey` 与它共用一份行重建。两条挡回路都是一读一写，写的键等长。
  - 领域 `domain/stored_reference.go`：订舱标识与承运答复标识过 512 字节界（`ErrValueTooLongToStore`），答复依据不受界，超界在查库之前答 `INPUT_NOT_ACCEPTED`。迁移 0047 遇已有超界行拒绝施加。
  - TF 解码门 `decodeClosedPayload` 限读 1 MiB，超限 400 `MALFORMED_REQUEST`。
  - 读不动同答 `BOOKING_STORE_UNAVAILABLE`；答复口不再单读委托。
- **装配**：`cmd/parcel-api/assemble_transport_commission.go` 的 `buildBookingFaces` 接上两口；没碰 endpoints/main/assemble/routes。
- **文档**：
  - ADR-0239，ADR-0202 前向指针；
  - UC-TF-004 改写 `AT-TF-149`、`AT-TF-153` 的结果，增 `AT-TF-326`–`328`；
  - 演示种子 README 第 4 步、演示动线补记；
  - AT 盘点①加三行与追记（总览不动）；
  - TF CONTEXT Language 一节立「承运答复挡回」词条；
  - 票 144 补注一节；立后继票 [167](./167-booking-answer-bounce-register-has-no-read-face.md)（查阅面）、[170](./170-booking-answer-bounce-register-has-no-retention-or-aggregation.md)（保留期与聚合）。

### 代决（用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」与常设指令「你作为业务和系统专家自决……解决所有的问题」）

决定层面的取舍写在 ADR-0239，下面只列结论与越权风险点；实施层面另几处 ADR 里没有，列在后面。

1. **合并做在应用层，不只在 HTTP 层译名**：编排结果里不留区分位，后来接这只编排的适配器无从泄露。
   - 越权风险点：区分只在挡回册，运营今天只能经库读（ADR-0239 风险点 1，去处票 167）。
2. **内部留痕落库，不写日志**：照 ADR-0230 派送待形成册的台账先例。parcel-api 不记请求日志，日志也不是审计依据。
   - 越权风险点：册没有保留期与聚合（风险点 4，票 170）；被挡回报文的内容不留（风险点 6）。
3. **同键计次，不一次一行**：键含答复标识与依据，不含内容。
   - 越权风险点：同一答复标识换了载荷也并进同一个计数；每个新标识多一行（风险点 4）。
4. **册写不动答未决，不答订舱不在册**：未决原因借答复册的名 `ANSWER_STORE_UNAVAILABLE`。
   - 越权风险点：名字是借的（风险点 5）。
5. **两条路同一副答复靠四件**：一读、一写且依据定宽、字节界在查库之前、载荷限读。时差不另处理，不加人为延时、不做常数时间。
   - 越权风险点：剩余微秒级差异按统计仍可能分得开（风险点 2）。
6. **字节界 512，做在 TF 领域构造门，超界答 `未受理`**：先例是 PS 外部标识号码的字节界与 TF 对 NUL 的构造门；不做在 HTTP 层答 400。
   - 越权风险点：界是产品常量（风险点 7）；`NewBookingReference` 也是承运总单、收费发生项登记读订舱引用的门，那几口同样受界。
7. **载荷限读做在 TF 共用解码门，不只给订舱两口**：一处定义。
   - 越权风险点：TF 全部命令口都受 1 MiB（风险点 8）。
8. **迁移 0047 遇超界旧行拒绝施加，不加只给仓储用的还原门**：PS 的先例是另开 `Rehydrate…` 门；TF 这几册在首发前没有真实租户数据，拒绝施加比多开一扇相信输入的门更直。
   - 越权风险点：有超界旧行的库要先人工处理这些行才迁得过去。
9. **订舱在册而委托读不回仍答 500、不记挡回**：照 ADR-0202 补记。
   - 越权风险点：这一格对任何答复方暴露订舱存在（风险点 3），只在库被绕过编排改坏时出现。
10. **改名「挡回」，并在 TF CONTEXT 立词条**：「拒收」在 TF 已指交接`已拒收`与收件人拒收，英文 Refusal 也已用于段进入拒收、交接拒收与 `REFUSED`；评审建议的「不受理」在 TF 代码里又泛指构造门上的输入不受理。UC-TF-004 的 AT 已在用这个词，照 ADR-0230 立「派送待形成」的先例，二轮在 TF CONTEXT Language 一节立「承运答复挡回」词条；票 167 只做查阅面。
    - 越权风险点：owner 若不喜欢「挡回」这个词，改名要连表名、CONTEXT 词条一起改（表还没进 main，进 main 之前改最便宜）。
11. **`AT-TF-149` 原地改写结果，不退号另立**：场景没变，变的是结果；新行为另立 `AT-TF-326`–`328`，字节界那一支并进 `AT-TF-326`。
    - 越权风险点：读旧号的人要按新结果读。149 与 153 不在 AT 盘点里（归票 139），本票没替它们加行。
12. **ADR-0202 文末「复核记录」R9 那一行不改**：它已写「那一格的停用随该票的 ADR 记」。停用照「部分停用」写法记在 ADR-0202 的 Status 行与 Links。
    - 越权风险点：票面原写「在 ADR-0202 补复核记录说明这一格的停用」，本票读作「已有那一行加前向指针」，没在正文里另起一段。
13. **演示只由真库装配用例证，没在一次性库上经演示 Dex 实走**：证据只记 `S`；动线补记照写，并注明这一点。
    - 越权风险点：演示环境里那一答没有人实走过。
14. **票 144 加补注、不改它的修法**：集成说明那份文件不存在，没进 main；补注只告诉 144 的作者这一格的续办怎么写。
    - 越权风险点：144 若有人在途，补注一节可能撞车。按协调方的在途清单，144 不在途。

### 用例

- 领域 `internal/transportfulfillment/domain/stored_reference_test.go::TestBookingAnswerReferencesHaveAStorableByteBound`：订舱标识与答复标识 512 字节收、513 字节拒，按 UTF-8 字节计；答复依据 3000 字节照收；全空白照旧拒。
- 编排 `internal/transportfulfillment/application/booking_schedule_and_answer_source_test.go`：
  - `TestOnlyTheCommissionedProviderAnswersItsBooking`（149）
  - `TestANonProviderAndAnAbsentBookingAreAnsweredAlike`（326）：三条挡回路结果 `reflect.DeepEqual` 相同；各一读一写、不碰答复册；读不动与册写不动各答同一原因。
  - `TestAReferenceAtOrOverTheByteBoundIsAnsweredAlikeOnBothBouncePaths`（326 字节界那一支）：界上两条路同答订舱不在册、各一读一写；答复标识或订舱标识超界，两条路同答未受理、零读零写。
  - `TestTheBounceLedgerTellsTheTwoBasesApart`（327）
  - `TestABouncedAnswerIsTakenOnceTheBookingIsEntered`（328）
  - `TestAnAnswerRecoveryDiscipline`（153 与恢复面）：数据违约不记挡回。
- 线格式 `internal/transportfulfillment/adapters/http/booking_faces_test.go`：
  - `TestTheBookingAnswerFaceAnswersOnTheWire`：别的承运商答在册订舱，与服务方答不在册订舱，状态与响应体逐字节相同；服务方答过之后别人原样重放也一样。
  - `TestTheBookingAnswerFaceBoundsWhatItReads`：超界答复标识两条路同为 200 `INPUT_NOT_ACCEPTED`、逐字节相同；载荷恰好 1 MiB 照收；答复口与订舱提交口多一个字节答 400。
- 存取（真库）`internal/transportfulfillment/adapters/postgres/booking_answer_bounce_test.go`：
  - `TestABookingIsReadBackWithItsCommissionsProviderInOneStatement`：在册、不在册、他租户、委托读不回四格；连读交回的订舱与单读相同。
  - `TestABouncedAnswerIsCountedOnItsOwnRowPerBasis`：计次、首末时刻、依据分行且存码、无事务拒（`ErrTransactionRequired`）、回滚不留、封闭集外与空来源先拒。
  - `TestBothBasesWriteKeysOfTheSameWidthAtTheByteBound`：订舱与答复标识都是 512 字节的随机十六进制串（不可压缩，测最坏情形），两种依据都写得进，两行键宽相同。
  - `TestTwoConcurrentIdenticalBouncesCountTwiceOnOneRow`：后到一笔停在主键上，先到提交后并进同一行，次数 2、首次取早的那一刻。
  - `TestMigration0047RefusesStoredReferencesOverTheByteBound`：订舱册的订舱标识或答复册的答复标识超界时 0047 拒绝施加；长的答复依据不挡。
- 生产装配（真库）`cmd/parcel-api/assemble_booking_faces_test.go`：
  - `TestTheWiredBookingAnswerFaceAnswersANonProviderAsBookingNotFoundAndKeepsTheBasis`（326–328）：末端承运客户端答干线订舱，末端与干线各答一个不在册订舱，三次响应逐字节相同；挡回册三行依据分清，重交计 2；答复册零行；录入订舱后干线重交 201、再交 200 `EXISTING_ANSWER`，挡回册不变。
  - `TestTheWiredBookingAnswerFaceAnswersAlikeWhateverTheIdentifiersLength`（326 字节界那一支，评审 B1 的回归）：答复标识 512 字节、订舱与答复标识都 512 字节，两条路都答 `BOOKING_NOT_FOUND`、逐字节相同、各落一行；513 字节与评审校准的 2605 字节，两条路都答 `INPUT_NOT_ACCEPTED`、逐字节相同、零行。
  - `TestTheWiredBookingAnswerFaceAnswersUndecidedWhenTheBounceCannotBeKept`（327 末支）：挡回册改表名写不进，答 200 `COMMISSION_UNDECIDED` / `ANSWER_STORE_UNAVAILABLE`，两册零行。
  - 原 `TestTheWiredBookingFacesBookTheSeededTrunkScheduleAndTakeTheCarriersAnswer` 那一答改为 `BOOKING_NOT_FOUND`。

### 验证

- 基于 main `b290b777`。代码钉 `35edae55`（二轮），此后只改文档与票面：
  - `go build ./...`、`go vet ./...` 干净，变更的 Go 文件 `gofmt -l` 干净。
  - 每一笔提交都经 `git archive` 导出到 scratchpad，单独 `go build ./...`、`go vet ./...` 通过。代码提交（`0b2dbf95`、`a36fe5da`、`23f33e7a`、`35edae55`）另单独跑持久化写口门禁，都绿。
- 一轮修订钉 `23f33e7a`，`scripts/test-full.sh` 跑了以下 26 个包，全绿：
  - `./internal/transportfulfillment/...`（12 个有测试的包）；
  - 反向依赖 `./cmd/parcel-api/...`、`cmd/parcel-dispatch`、`cmd/parcel-transport-register`，以及 NR、NO、PP、PS、SA、VE 各自的 `adapters/transportfulfillment`；
  - `./internal/architecture/...`、`./migrations/...`、`./tools/...`。
- 二轮钉 `35edae55`，`scripts/test-full.sh ./internal/transportfulfillment/... ./cmd/parcel-api/... ./cmd/parcel-transport-register/ ./cmd/parcel-dispatch/ ./internal/architecture/... ./migrations/...`，17 个包全绿。
- 变异检验：
  - 让 `bounceAnswer` 对「不是服务提供方」不写册，编排三条用例与真库装配用例即红；
  - 把字节界抬到 1 MiB，真库装配的字节界用例即红（513 字节答成 `BOOKING_NOT_FOUND`）；
  - 换回后都绿。
- ADR 索引与 AT 计数两个工具只在 scratchpad 副本上试跑，确认解析得了；仓里没重生成。

### 没做与去向

- 挡回册查阅面：后继票 [167](./167-booking-answer-bounce-register-has-no-read-face.md)，与票 48 同一车道。
- 挡回册保留期与聚合：后继票 [170](./170-booking-answer-bounce-register-has-no-retention-or-aggregation.md)。
- 首版集成说明（票 144）：还没进 main，补注已写，集成说明随 144 写。
- 迁移号：作者按当时 main（TF 止于 `0046`）与在途分支暂用 `0047`；协调方进 main 时本票先落，改为下一个空号 `0047`（在途的票 88、57 顺延），连号。
- 进 main 时由协调方重算 AT 盘点总览、重生成 ADR 索引与机制清点。

## 评审记录（2026-10-21，隔离评审一条阻断，在同一分支修订）

逐条去向：

- **B1 超长答复标识让两条挡回路答复不同**（阻断，评审已在真库与生产装配上复现，校准长度 2605）：已修，`23f33e7a`。
  - 1 依据定宽：挡回册 `basis` 改 smallint（1、2），仓储一处译码。
  - 2 字节界与限读：订舱标识、答复标识、答复依据 512 字节界在 TF 领域构造门，超界在查库之前答 `INPUT_NOT_ACCEPTED`（照 TF 对 NUL 的先例，不答 400）；TF 共用解码门限读 1 MiB、超限 400；迁移 0047 遇已有超界行拒绝施加。
  - 3 回归：真库装配用 512、513 与评审的 2605 三个长度，两条路逐字节相同；编排、线格式、真库存取各补一条。
  - 4 ADR-0239 决定三与风险点 2 的措辞已改，写清四件怎样守住同一副答复，剩下只有时差；「每次试探都留一行」改为只说界内。
- **5 挡回册无上限增长**：立后继票 170，ADR-0239 风险点 4 指向它。
- **6 名字**：改名「挡回」（表 `booking_answer_bounce`），不用评审举的「不受理」——它在 TF 代码里泛指构造门上的输入不受理，同样一词多义。TF CONTEXT 词条随票 167 建页时立。代决 10。
- **7 适配器注释写了迁移号**：改成指名表。
- **8 迁移头注「原样重交」说过头**：改为「同键再来」，写明键里没有内容、换了载荷也并进同一计数。
- **9 中间提交门禁红**：变基时在临时分支上 `cherry-pick -n` 把原三笔代码提交合成 `0b2dbf95`，再依次摘后续各笔，把 `claude-dcg145` 指过去，没用 `rebase -i`。合成后的树与原 HEAD 逐字节相同。
- **10 并发同键的真库用例**：已补 `TestTwoConcurrentIdenticalBouncesCountTwiceOnOneRow`。

未采纳：无。评审举的名字没有照用，理由见第 6 条。

### 二轮（2026-10-21，无阻断；同一分支小修，`35edae55` 与其后的文档提交）

- **1 ADR-0239 风险点 8 点错了载荷**：交接登记只带一个对象，撞不到 1 MiB。已改为点名列表字段并给量级：委托提交与装载分配的 `members`（按演示长度约 4 万个到顶），承运总单的 `associations`（每条约 52 字节、约 2 万条到顶，逐件关联包裹的总单最可能先撞上）；超限答 400 `MALFORMED_REQUEST`。
- **2 词条进 TF CONTEXT**：Language 一节立「承运答复挡回」。写明与那一次答复同笔写入，是台账不是事实来源，不进订舱当前读法，与`已拒收`、收件人拒收、`未受理`不同。票 167 改为只做查阅面。
- **3 答复依据退出字节界**：它不进任何键、不进挡回册，只在服务方那条路、来源核对之后才判。`NewAcceptanceBasisReference` 回到既有构造门（非空、无 NUL）；迁移 0047 的拒绝门不再查 `octet_length(basis)`；用例、ADR-0239 决定三与风险点 7 同步。
- **4 迁移 0047 头注改准**：写清订舱标识进订舱册、答复册与挡回册的键，答复标识进答复册与挡回册的键，答复依据不进键。界只由领域构造门守，库上不加 CHECK（与 PS 先例一致，一个数只定义在一处）。
- **5 ADR-0239 风险点 9 补后果**：依据定宽之后，租户与来源身份在两条挡回路上是同一个值，再长也分不开两条路，过长只会两条路一起答未决——是可用性问题，不是泄露残余。
- **6 测试**：
  - `TestTheBookingAnswerFaceBoundsWhatItReads` 补「恰好 1 MiB 照收」，越界改为只多一个字节；
  - `TestBothBasesWriteKeysOfTheSameWidthAtTheByteBound` 改用随机十六进制串；
  - 迁移拒绝门用例加「长答复依据不挡」一格。

## 进 main 记录

2026-10-21 协调方合入。分支基于 `b290b777`（即当时 main），不需重放，分支 SHA 原样进 main（`750d6cfc`…`958e2c4d`）。一轮隔离评审一条阻断（超长答复标识让两条拒收路答复不同、那一位信息漏回）已由作者修（依据定宽、键内标识 512 字节界、命令载荷限读 1 MiB）；二轮评审无阻断。进 main 时协调方把 TF 迁移 `0050` 改为下一个空号 `0047`（本票先落；在途的票 88、57 顺延到 0048、0049），引用随改（`4f13952b`）；改号后在 55432 一次性库上 `seed.sh` 全程退 0（0047 已施加），用完 `DROP … WITH (FORCE)`。其上按 parallel-sessions.md 重生成 ADR 索引、重算 AT 盘点计数、重生成机制清点（`7278bf09`），各自单独成笔；`adr-index -check` 与 `at-coverage-counts -check` 退 0。在干净的隔离树上对 `7278bf09` 跑 `go build ./...`、`go vet ./...`、`gofmt -l` 干净，`scripts/test-full.sh` 全量 158 ok、0 FAIL（`-count=1`，无缓存）；推 `7278bf09:main`。
