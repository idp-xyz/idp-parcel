# 24 订舱提交（引班次）与承运答复集成客户端面

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg24（见文末「完成记录」）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [23](./23-transport-commission-submission-face.md)、[16](./16-transport-schedule-templates-and-concrete-schedules-are-registered.md)
地盘：`operation_decision.go`、`integration_client.go`；tfhttp 新 `submit_booking_request.go`、`answer_booking.go`、`integration_client_intake.go`；TF 新 `adapters/accessidentity/integration_client_authenticator.go`；domain、postgres 订舱（班次引用、`WAITLISTED`）；迁移 TF 0029；`assemble_transport_commission.go`、`assemble_integration_client.go`、`endpoints.go` 行；种子 access 02/03/05/06、governance。
出处：[02](./02-transport-commission-has-no-command-face.md)；方案「02 委托、订舱、承运答复」D02-2、D02-3、D02-8、D02-9，「需要的 ADR」ADR-0202。

## 做什么

- 运营决定种类 `BOOKING_REQUEST_SUBMISSION`（方案 D02-3）；订舱必须引用在册具体班次（方案 D02-9）。
- 承运答复走集成客户端族，新外部事实类型 `CARRIER_BOOKING_ANSWER`，客户端来源身份的核对照方案 D02-2；ADR-0202 补进 ADR-0149 的列举（ADR-0149 Links 加后继指针）。
- 答复封闭集加 `WAITLISTED`；`待确认`、`待补充` 只是读法；逐对象与委托级接受记为缺口（方案 D02-9）。
- 出站订舱连接器不在本票（方案 D02-8）；确认订舱的容量关系归 [37](./37-confirmed-booking-capacity-relationship.md)。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 引用不在册班次答 `SCHEDULE_NOT_FOUND`。
- 客户端来源身份不等于服务方答不受理。
- 答复五值。
- 幂等。
- 真库测试。
- 演示：对 SZX→SIN 物化班次订舱，合成承运客户端用令牌回 `ACCEPTED`。

## 规模

L

## 完成记录

2026-10-19 作者子代理在分支 `claude-dcg24` 上做完，基于 `38916b00`（派单时的 origin/main）。收工时 origin/main 已到 `37afd6e1`，与本分支试合并无冲突（`git merge-tree`），未 rebase，由协调方按令牌次序合入。

提交：`5adb8c0b`（决定种类、事实类型与迁移 access_identity 0013）、`17e9c59a`（领域、编排、订舱与答复两册、迁移 TF 0029）、`8ac7508d`（tfhttp 两口与集成客户端 Intake、TF 集成客户端认证方）、`403cdcac`（parcel-api 装配与端点表、真库用例）、`d8451320`（演示种子）、`b0caffc1`（ADR-0202 补记、ADR-0014 部分停用、UC-TF-004 AT、演示文档）、`5bb7b3a6`（立票 48、49）。

### 做了什么

- **决定种类与事实类型**：`accessidentity.DecisionBookingRequestSubmission`（`BOOKING_REQUEST_SUBMISSION`）、`accessidentity.FactCarrierBookingAnswer`（`CARRIER_BOOKING_ANSWER`），枚举只追加。迁移 `access_identity/0013_booking_request_submission_decision_and_carrier_booking_answer_fact.sql` 同笔重建两条约束：决定种类列 main 上 0011 那份的全集再加本票一种，事实类型列 0003 那份再加本票一类。
- **领域**：`BookingRequest` 多一格具体班次（`Schedule()`），不指名班次建不成。承运答复封闭集加 `BookingWaitlisted`（`WAITLISTED`：不成约、无接受量、带依据）；`Settles` 答是不是定局，`AdmitsSuccessor` 钉「只有候补可被一份定局后继，定局不早于候补、换答复标识、同租户同订舱」。重建门验五值形状。
- **订舱编排** `SubmitBooking`：构造门 → 按（租户、订舱）分重放与冲突 → 委托在册（`COMMISSION_NOT_FOUND`）→ 委托未取消（`COMMISSION_NOT_BOOKABLE`）→ 班次在册（`SCHEDULE_NOT_FOUND`，经既有 `ports.TransportScheduleReader`）→ 落库；读不动各自答未决（新增原因 `SCHEDULE_STORE_UNAVAILABLE`）；并发首交的输家按摘要分已有与冲突（此前读回赢家一律答已有）。不入队。
- **答复编排** `AnswerBooking`：来源身份非空（缺即 `ErrAnswerSourceMissing`）→ 订舱在册（`BOOKING_NOT_FOUND`，此前答 `INPUT_NOT_ACCEPTED`）→ 来源身份等于订舱所属委托的服务提供方（`SOURCE_NOT_SERVICE_PROVIDER`）→ 同一答复标识分重放与冲突 → 构造门 → 已有答复时只接候补的合格后继 → 落库。订舱在册而委托读不回答 `ErrBookingWithoutCommission`（500）。不入队。
- **端口**：新文件 `ports/booking_answer_history.go`（`BookingAnswerHistory`、`BookingAnswerLedger`）；`ports.go` 只改两处注释（`BookingAnswerStore.FindByKey` 交当前那一份）。编排的 `Answers` 依赖改为 `BookingAnswerLedger`，新增 `Schedules` 依赖。
- **两册**（postgres `booking.go`）：订舱读写带班次；答复册按（租户、订舱、答复标识）立身份，`FindAnswers` 交全部、`FindByKey` 交当前那一份（有定局交定局）。迁移 `transport_fulfillment/0029_booking_names_a_schedule_and_answers_admit_a_waitlist.sql`：订舱加 `schedule_id`，答复主键改三列，两条部分唯一索引钉至多一份候补、一份定局，答复值与形状约束加 `WAITLISTED`；两表有行即拒绝施加。
- **摘要**：`CanonicalizeBookingPayload` 加班次一键，沿用 `TFC-1`；订舱口退役无版本那一形（`unversionedBookingDigest` 及其用例）。答复摘要形状不变，无版本那一形保留。
- **命令面**：`POST /transport-fulfillment-booking-request-submissions`（tfhttp `submit_booking_request.go`，挂 `operatorDecisions.transport`，按订舱提交认证）；`POST /transport-fulfillment/booking-answers`（tfhttp `answer_booking.go`，挂新 `IntegrationClientIntake`）。答复载荷 `booking`、`answer`、`outcome`、`quantity`、`basis`、`decidedAt`，封闭解码，自报租户或来源即 400，五值之外的词 400。命令端点加集成客户端两格（`INTEGRATION_CLIENT_CREDENTIAL_REJECTED` 401、`INTEGRATION_CLIENT_NOT_GRANTED` 403）。
- **TF 集成客户端认证方**：`adapters/accessidentity/integration_client_authenticator.go`，按 `CARRIER_BOOKING_ANSWER` 铸信封、按 `EXTERNAL_FACT` 判准入，交回租户与绑定的来源身份。
- **装配**：`assemble_transport_commission.go` 追加 `buildBookingFaces`（委托册、订舱册、答复册、班次只读口、时钟；每次一笔事务、未决整笔回滚）；`assemble_integration_client.go` 的 Intake 组加 `transport`；`endpoints.go` 参数与端点行、`main.go` 构造与传参、`unwired_orchestration.go` 占位都只追加。
- **演示种子**：配置员授订舱提交（`SYN-GRANT-DEX-CONFIG-BOOKING-REQUEST-SUBMISSION-01`），准入对照 `SYN-MAP-OD-BOOKING-REQUEST-SUBMISSION-01`；合成干线承运商客户端（Dex 静态客户端 `syn-integration-carrier-trunk-01`，`sub` `CiBzeW4taW50ZWdyYXRpb24tY2Fycmllci10cnVuay0wMQ`，来源身份 `SYN-PARTY-CARRIER-TRUNK-01`），授承运订舱答复，对照 `SYN-MAP-EF-CARRIER-BOOKING-ANSWER-01`；本产品区间 `governance/30-…` 两份；`seed.sh`、README、`integration-dex.yaml` 只追加。
- **文档**：ADR-0202 Status 加一句指向文末补记，文末追加「补记：票 24 落地时的补充裁定」与风险点 12–17；ADR-0014 Status 与 Links、`docs/adr/README.md` 该行登部分停用；UC-TF-004 补 `AT-TF-146` 至 `AT-TF-153`；演示文档第 3 步补订舱与承运答复。ADR-0149 Links 的前向指针票 23 已加，本票未动。
- **立票**：[48](./48-booking-requests-and-carrier-answers-have-no-read-face.md)（订舱与答复的查阅面）、[49](./49-booking-does-not-check-schedule-against-commission-scope.md)（订舱核班次与委托运输范围，待裁）。

### 代决与越权风险点

用户原话「你作为业务和系统专家自决……解决所有的问题」；以下是方案没裁、本票自决的细节，裁定已写进 ADR-0202 文末补记。

1. **候补不是结局，候补之后可接一份定局**。只加 `WAITLISTED` 而保留「一订舱一应答」，候补就成了不能再被确认的软拒绝，`候补 → 已确认`这条 CONTEXT 生命周期走不通。所以答复按（租户、订舱、答复标识）立身份，至多一份候补、一份定局。
   - 越权风险点：只内置这一种形，更新候补位次、候补后分批接受都进不来（ADR-0202 风险点 13，TF owner）。
2. **答复方不是服务方与订舱不在册分两格**（`SOURCE_NOT_SERVICE_PROVIDER` / `BOOKING_NOT_FOUND`），票面「答不受理」按这一格落，不并进 `INPUT_NOT_ACCEPTED`：续办不同（前者永远不归它答，后者可能只是运营侧还没录）。
   - 越权风险点：同一租户的承运商之间可探知订舱标识存在（ADR-0202 风险点 14，TF / access-identity owner）。
3. **来源核对排在重放之前**，来源不符与冲突都不回显在册答复：拿别人的答复标识重放读不回别人的答复。
4. **答复的未受理沿用 `INPUT_NOT_ACCEPTED`**，不另立来源格（回答票 23 代决 5）：续办同是改载荷，与提交方的族无关。
5. **订舱还核委托在册与未取消**（票面只写了班次）：答复要按委托的服务方核来源，委托必须在册；取消停的正是未消耗的意图，在它下面新订舱等于续上。已开始的委托不挡。
   - 越权风险点：`COMMISSION_NOT_BOOKABLE` 只覆盖取消一种；取消面在票 25，今天生产上到不了这一格（TF owner）。
6. **订舱不核班次方向与委托范围、不核班次运营方**：委托上没有范围一格。一次性库实测末端委托下订得到干线那一班。
   - 越权风险点：见 ADR-0202 风险点 12，去处票 49（TF owner）。
7. **订舱与答复不入队**（计划 X3）：今天没有消费者；票 28 的订舱发生项按读答复册取依据。
8. **订舱的并发输家按摘要分格**：此前读回赢家一律答已有，会把冲突吞掉，与票 23 对委托的修法一致。
9. **订舱摘要加班次不换 `TFC-1`，并把票 23 委托摘要改形一并登成 ADR-0014 部分停用**：票 17 评审确立了这一种改动要登部分停用，票 23 当时没登。
   - 越权风险点：ADR-0202 风险点 16（TF owner）；`unversioned_payload_digest.go` 头注写着「函数体一个字节都不能改」，本票整删了订舱那一个函数，前提是迁移 0029 拒绝有行的库（TF owner，同票 23 代决 3）。
10. **答复主键改三列**：迁移 0029 改的是 0003 建的主键；两表在本票之前没有写入方，有行即拒绝施加。
11. **答复口的路径取 `/transport-fulfillment/`**（控制事实那组），订舱口取 `transport-fulfillment-`（运营决定那组）：读路径就分得出提交方在不在产品之内。答复标识的线名取 `answer` 不取 `acceptance`：候补与拒绝都不是接受。
12. **集成客户端族在 TF 的认证失败用本族自己的码**：沿 CC、SA 两包已有的码，操作者两格与它分开。
13. **装配另立 `buildBookingFaces`，不与委托提交共用一只编排实例**（回答票 23 代决 10）：这一只不装协议核对与委托意图交付，两口都不提交委托。
14. **governance 占号 30 下放两份区间**：协调方只分了一个号，而运营决定与外部事实各要一条区间。
    - 越权风险点：`30-` 前缀下两份文件，与「一号一份」的惯例不同（协调方）。
15. **`main.go` 顺手补上容量池登记那一处被吞掉的 `err` 检查**：票 23 合批时在 `buildCapacityPoolRegistration` 与它的检查之间插了一行，容量池装配失败会被下一行覆盖。只插入，不重排。

### 用例

- 领域：`TestABookingNamesAConcreteSchedule`；`TestOnlyAWaitlistAdmitsASettlingSuccessor`（`AT-TF-151`）；`TestBookingIsNotAcceptanceAndOnlyAcceptanceBinds` 加候补一格并把封闭集改为五值（`AT-TF-150` 形状半边）；规范化定值改为带班次。
- 编排：`TestABookingOnARegisteredScheduleIsSubmittedOnce`（`AT-TF-148`）；`TestABookingNeedsARegisteredCommissionAndSchedule`（`AT-TF-146`、`147`，含他租户班次与重放先于委托核对）；`TestABookingWhoseReferencesCannotBeReadIsUndecided`；`TestAConcurrentBookingLoserIsJudgedByContent`；`TestOnlyTheCommissionedProviderAnswersItsBooking`（`AT-TF-149`）；`TestACarrierAnswersInFiveValues`（`AT-TF-150`）；`TestAWaitlistedBookingSettlesOnce`（`AT-TF-151`）；`TestAnAnswerRecoveryDiscipline`（`AT-TF-153` 与未决各格）；`TestABookingAnswerCannotBeOverwritten` 改走已提交委托并标 `AT-TF-152`；未决原因封闭集加一种。
- 两册（真库）：订舱往返带班次、约束拒空白班次、拒无依据的候补与「待确认」值；`TestABookingKeepsAWaitlistAndOneSettlingAnswer`（`AT-TF-151` 真库面）；`TestMigration0029RefusesBookingsItCannotGiveASchedule`。
- 线格式：`TestTheBookingRequestSubmissionFaceAnswersOnTheWire`、`TestTheBookingAnswerFaceAnswersOnTheWire`、`TestTheBookingAnswerIntakeAuthenticatesBeforeReadingThePayload`；运营决定各口表加订舱提交一口。
- 接入身份：`TestACarrierClientAuthenticatesWithItsBoundSource`、`TestCarrierClientGradesTranslateIntoTransportFulfillmentGrades`；`TestEveryTFDecisionMapsToItsOwnDecisionKind` 加一种；册面（真库）`TestTheBookingRequestSubmissionDecisionIsGrantedAndReadBack`、`TestTheCarrierBookingAnswerFactIsGrantedAndReadBack`。
- 生产装配（真库，`S`）：`TestTheWiredBookingFacesBookTheSeededTrunkScheduleAndTakeTheCarriersAnswer`——种子干线协议与 SZX→SIN 物化班次落库，委托 201 → 对 `SYN-SCHED-SZX-SIN-DAILY/2026-10-21` 订舱 201 → 不在册班次 200 `SCHEDULE_NOT_FOUND` → 干线承运客户端（真库客户端册、对照与区间）答 `ACCEPTED` 201 → 重放 200 `EXISTING_ANSWER` → 末端承运客户端 200 `SOURCE_NOT_SERVICE_PROVIDER`，库里订舱带班次、答复一行。`TestTheWiredBookingSubmissionAnswersUndecidedWhenTheScheduleReadAbortsTheTransaction`：班次表读不到、事务已中止，仍答 200 未决 `SCHEDULE_STORE_UNAVAILABLE`，零写入。端点探测表、换口授予表、集成客户端各口等级表各加本票的口。

### 验证（证据 `S`）

- `go build ./...` 干净；`go vet` 跑 TF、access-identity、architecture、`cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-transport-register`、`cmd/parcel-access-register`、`migrations`，干净；`gofmt -l internal cmd migrations scripts` 为空。
- 带 55432 DSN、`-p 1 -count=1`：`./internal/architecture/`、`./internal/accessidentity/...`、`./internal/transportfulfillment/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-transport-register/`、`./cmd/parcel-access-register/`、`./cmd/parcel-governance-register/`、`./migrations/...`、`./scripts/demo-seeds/...`、`./tests/...` 全 ok。没跑全量。事务闭包门禁先红过一次（真库用例在事务回调里断言），已把断言挪出闭包。
- 一次性库 `dcg24_seed`：`seed.sh` 灌入退 0（订舱提交授予与对照、承运客户端与授予、承运答复对照、两份区间都是 RECORDED / REGISTERED）。另起一只同 `integration-dex.yaml` 的 Dex（宿主端口 5568，`iss` 仍是 `http://127.0.0.1:5558/dex`，共享的 5558 那只未碰），读 claims 核过干线承运客户端的 `sub` 与种子相同；起 `parcel-api`（操作者发行方取共享 5556 那只，集成客户端公钥集取 5568，治理自授权）实走：
  - 干线委托 → `201` `COMMISSION_SUBMITTED`；
  - 对 `SYN-SCHED-SZX-SIN-DAILY/2026-10-21` 订舱 → `201` `BOOKING_SUBMITTED`；对 `…/2026-10-22`（未登记）→ `200` `SCHEDULE_NOT_FOUND`；
  - 合成干线承运客户端用令牌答 `ACCEPTED` → `201` `BOOKING_ANSWERED`；原样重交 → `200` `EXISTING_ANSWER`；
  - 银行客户端令牌 → `403` `INTEGRATION_CLIENT_NOT_GRANTED`；操作者令牌打答复口 → `401` `INTEGRATION_CLIENT_CREDENTIAL_REJECTED`；不带令牌打订舱口 → `401` `OPERATOR_CREDENTIAL_REJECTED`；
  - 末端委托（末端协议）下订同一班 → `201`（风险点 12 的实测），干线承运客户端去答 → `200` `SOURCE_NOT_SERVICE_PROVIDER`，答复表仍一行。
  - 走完已停进程、删 Dex 容器、删库。
- 未重生成机制清点（进 main 时统一做）。

### 偏离与待裁

- **地盘外改动**（都是新文件、测试或只追加）：
  - `internal/accessidentity/adapters/postgres` 新增真库用例文件；
  - tfhttp `command_endpoint.go` 加集成客户端两格映射，`operator_decision_intake.go`(+test) 加一口；
  - TF 领域 `commission_rehydration.go` 改一句注释；`registry_zero_gates_test.go`、`transport_commission_test.go` 随订舱多一格改夹具；
  - 编排既有测试随依赖与格名改写（`commission_transport_test.go`、`commission_agreement_check_test.go`、`unversioned_payload_digest*.go`）；
  - `cmd/parcel-api` 的 `endpoints_test.go`、`assemble_operator_decisions_test.go`、`assemble_integration_client_test.go` 各追加本票的口；`main.go` 多插一处 `err` 检查（代决 15）；
  - ADR-0014 与 `docs/adr/README.md` 该行登部分停用（代决 9）。
- **号段**：迁移 access_identity 0013、TF 0029（照分配）；AT `AT-TF-146` 至 `153`（分配 146–155，154、155 未用）；立票 48、49（照分配）；governance 30（两份，代决 14）。
- **进 main 时要做的**：
  - access_identity 0013 的决定种类 CHECK 只列到 0011 的全集加本票一种。票 20 的 0012 若先进 main，合批时要把它那两种并进 0013 的列表（反过来 0012 也要并进本票这一种），否则后施加的那份会把先施加那份的种类删掉——同票 23 评审阻断 1。
  - AT 覆盖盘点补 `AT-TF-146` 至 `153`；重生成机制清点。
- **没改的**：TF CONTEXT（订舱生命周期「待确认、候补、部分确认、已确认、拒绝或失效」已盖住本票）；ADR-0149（票 23 已加前向指针，ADR-0202 不停用它任何一条）。
- **待裁 / 交接**：
  - ADR-0202 补记风险点 12–17 待 TF、access-identity owner 复核；票 49 待 owner 裁委托范围怎么表达。
  - 票 25（取消订舱）要在答复册上处理「已有定局的订舱能不能取消」与 `BookingStore.Replace`；票 28 的订舱发生项取依据时，当前那一份答复用 `BookingAnswerStore.FindByKey`（有定局交定局）。
  - 票 37（确认订舱的容量关系）可按 `CarrierAcceptance.Binds` 与 `AcceptedQuantity` 接。
