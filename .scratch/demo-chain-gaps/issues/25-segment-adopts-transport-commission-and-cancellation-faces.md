# 25 履约段采用运输委托；取消委托与订舱

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg25（见文末「完成记录」）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [23](./23-transport-commission-submission-face.md)、[24](./24-booking-references-a-schedule-and-carrier-answers-via-integration-client.md)
地盘：`operation_decision.go`；tfhttp 新 `adopt_segment_commission.go`、`cancel_transport_commission.go`、`cancel_booking_request.go`；`application` 新 `adopt_segment_commission.go` + `CancelBooking`；`domain` 新 `segment_commission_adoption.go`；ports 新文件（含 `BookingStore.Replace`）；postgres 新表，迁移 TF 0030；装配与 `endpoints.go` 行；种子；ADR-0204（ADR-0098 加后继指针）、TF CONTEXT。
出处：[02](./02-transport-commission-has-no-command-face.md)；方案「02 委托、订舱、承运答复」D02-3、D02-6，「需要的 ADR」ADR-0204。

## 做什么

- 新运营决定 `FULFILLMENT_SEGMENT_COMMISSION_ADOPTION`：声明（段、委托）这一对，前置条件照方案 D02-6 与「需要的 ADR」ADR-0204。
- 采用即调 `MarkTransportStarted`，开始依据照方案 D02-6。
- 取消委托 `TRANSPORT_COMMISSION_CANCELLATION`、取消订舱 `BOOKING_REQUEST_CANCELLATION` 随本票开放，已开始的答法照方案 D02-6（方案 D02-3）。
- 起草 ADR-0204：部分停用 ADR-0098 决定四，范围照方案「需要的 ADR」；ADR-0098 Links 加后继指针，README 状态格注明「部分停用」。TF CONTEXT Rules 补段采用委托。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 段不在册答 `SEGMENT_NOT_FOUND`。
- 服务方不一致答冲突。
- 采用即标开始，随后取消答 `TRANSPORT_ALREADY_STARTED`（AT-TF-092）。
- 未开始的可以取消。
- 真库测试。
- 演示：用现有交接口立干线段后采用委托答 201，再取消委托答已开始。

## 规模

M

## 完成记录

2026-10-19 作者子代理在分支 `claude-dcg25` 上做完，基于派单时的 origin/main `6b02b547`，未 rebase，由协调方按令牌次序合入。号段照协调方预分配：迁移 access_identity `0015`、TF `0035`（票面原写 0030，已被票 20 占用），governance 种子 `32`、`33`、`34`，AT `AT-TF-181` 至 `188`，立票 `57`、`58`，ADR-0204。

提交：`a8c15b03`（三种决定种类、迁移 access_identity 0015）、`143c234e`（领域、端口、编排）、`2cb65683`（采用册迁移 TF 0035、订舱取消写口）、`6e956ced`（tfhttp 三口）、`5214152e`（写口无事务负向证据）、`f2375e3d`（parcel-api 装配与端点表、真库用例）、`6fb67ee9`（演示种子）、`195777e6`（真库用例 Covers）、`4fcaf056`（ADR-0204、ADR-0098 部分停用、TF CONTEXT、UC-TF-004、演示文档）、`b52c87c7`（立票 57、58）、`da74265a`（ADR-0204 风险点指向 57、58）。

### 做了什么

- **决定种类**：`accessidentity` 追加 `DecisionFulfillmentSegmentCommissionAdoption`（`FULFILLMENT_SEGMENT_COMMISSION_ADOPTION`）、`DecisionTransportCommissionCancellation`（`TRANSPORT_COMMISSION_CANCELLATION`）、`DecisionBookingRequestCancellation`（`BOOKING_REQUEST_CANCELLATION`），只追加。迁移 `access_identity/0015_segment_commission_adoption_and_cancellation_decisions.sql` 以 0013 那份决定种类全集为底再加三种（0014 重建的是客户面能力那条 CHECK，不并）。
- **领域**（新文件 `domain/segment_commission_adoption.go`、`segment_commission_adoption_canonicalization.go`）：`SegmentCommissionAdoption`（段、委托、服务方、这一段的开始依据与时刻）；`AdoptCommissionForSegment` 判服务方不符、委托已取消、段没有当前参与、段早于委托提交就已开始，委托未开始时以段的首个参与 `MarkTransportStarted`，已开始的原样交回；`ActualFulfillmentSegment.FirstEffectiveParticipation`（未被替代、未失效的最早一条，已离场照算，同刻按入场依据字面序）；重建门与 TFC-1 摘要（只收委托与服务方）。
- **端口**（新文件，不动 `ports.go`）：`ports/segment_commission_adoption.go`（`SegmentCommissionAdoptionStore`：`FindBySegment`、`Save`，没有改与删）、`ports/booking_cancellation.go`（`BookingCancellationStore.Replace`——票面写的 `BookingStore.Replace` 另立一口，理由同计划 X4）。段只读用既有 `ports.FulfillmentSegmentView`。
- **采用编排**（`application/adopt_segment_commission.go`）：声明过构造门 → 按（租户、段）分 `EXISTING_ADOPTION` / `ADOPTION_CONFLICT`（回显在册那一份）→ 段在册（`SEGMENT_NOT_FOUND`）→ 委托在册（`COMMISSION_NOT_FOUND`）→ 领域判（`PROVIDER_MISMATCH` 带 `commissionProvider`、`COMMISSION_NOT_ADOPTABLE`、`SEGMENT_WITHOUT_EFFECTIVE_PARTICIPATION`、`SEGMENT_STARTED_BEFORE_COMMISSION`）→ 先落连线、后标委托开始；标不上就读回（另一段先开始了照常采用、不改写首次开始；先被取消了答不可采用）。未决三原因：采用册、段册、委托册。不入队。
- **取消两口**（`application/commission_transport.go`）：`CancelCommission` 不在册改答 `COMMISSION_NOT_FOUND`（此前 `INPUT_NOT_ACCEPTED`）、已开始答 `TRANSPORT_ALREADY_STARTED` 并回显委托（线上名此前是 `TRANSPORT_STARTED`，没有线上消费方）、落取消撞上并发的开始或取消时读回按先落者答（此前答未受理）；新 `CancelBooking`：订舱在册（`BOOKING_NOT_FOUND`）→ 已取消即重放 → 所属委托开始时这次订舱已提出（申请不晚于开始时刻）答 `TRANSPORT_ALREADY_STARTED` → 领域取消 → 只落取消时刻。新格 `BOOKING_CANCELLED`；`BookingCommissionNotFound` 改名 `CommissionNotFound`（取消委托与订舱共用这一格，线上名不变）；`CommissionTransportDeps` 加 `BookingCancellations`。
- **两册**（postgres）：新 `segment_commission_adoption.go`（`SegmentCommissionAdoptions`，只 INSERT 与按段读）；`booking.go` 加 `BookingRequests.Replace`（只 `cancelled_at`、只作用于未取消的行，不改写 `recorded_at`）。迁移 `transport_fulfillment/0035_segment_commission_adoption.sql`：主键（租户、段），段与委托两条外键，按委托回查的索引，只插不改。
- **命令面**（tfhttp 新文件 `adopt_segment_commission.go`、`cancel_transport_commission.go`、`cancel_booking_request.go`；`operator_decision_intake.go` 只追加三种决定常量、三条 Intake 与断言）：`POST /transport-fulfillment-segment-commission-adoptions`（载荷 `segment`、`commission`、`provider`，开始依据与时刻不收）、`POST /transport-fulfillment-transport-commission-cancellations`（`commission`、`cancelledAt`）、`POST /transport-fulfillment-booking-request-cancellations`（`booking`、`cancelledAt`）。采用成立、取消成立答 201，其余各格 200；封闭解码，自报租户与未知键 400。
- **装配**：新 `cmd/parcel-api/assemble_segment_commission_adoption.go`（采用每次一笔事务、采用成立以外整笔回滚；两个取消口借订舱两口那只壳的 `within`）；`endpoints.go` 参数与端点行、`main.go` 构造与传参、`unwired_orchestration.go` 占位、`endpoints_test.go` 探针、`assemble_operator_decisions_test.go` 换口表都只追加。
- **演示种子**：配置员三种授予、三条准入对照（`data/access/02`、`03`），`data/governance/32`、`33`、`34` 三份本产品区间；`seed.sh` 三行、README 授予表一格与一段说明。
- **文档**：ADR-0204（新）；ADR-0098 Status 行与 Links 加前向指针，`docs/adr/README.md` 该行写被停条款与适用场景，ADR-0204 一行追加在已接受节末；TF CONTEXT Rules 补段采用委托一条、Lifecycles 补「已提交 → 运输已开始」一条；UC-TF-004 补 `AT-TF-181` 至 `AT-TF-188`；演示文档第 3 步补采用与取消一段（含本票实测）。
- **立票**：[57](./57-segment-commission-adoption-has-no-correction-and-start-does-not-rederive.md)（采用没有更正与解除面，首个参与被更正后开始不重派生）、[58](./58-adoption-does-not-check-commission-members-against-segment-objects.md)（采用不核委托成员与段对象），都待 TF owner 裁。

### 代决与越权风险点

用户原话「你作为业务和系统专家自决……解决所有的问题」；以下是方案没裁、本票自决的细节，裁定已写进 ADR-0204。

1. **「服务方一致」比的是登记方声明的服务方与委托的服务提供方**（ADR-0204 决定一）。不比实际承运商判断，也不比交接接收方：签约服务商与实际承运商可以不同（AT-TF-046），拿它们比会把转包拒掉。命令因此收（段、委托、服务方）三格，服务方不从委托抄——抄过来就核不出指错了委托。越权风险点：声明方自己核自己，只挡得住指错委托，挡不住两格一起填错。
2. **首个参与的读法**（决定二）：未被替代、未失效的最早一条，已离场照算，同刻按入场依据字面序。越权风险点 1：首个参与此后被更正，委托与连线上的开始都不重派生（票 57）。
3. **一段至多一份委托，一份委托可以多段采用；后到的段不改写首次开始，连线上另记本段的开始依据**（决定一、二）。越权风险点 2：没有反向解除采用的面（票 57）。
4. **前置条件的格与判序**（决定一）：新格 `SEGMENT_WITHOUT_EFFECTIVE_PARTICIPATION`、`SEGMENT_STARTED_BEFORE_COMMISSION`、`COMMISSION_NOT_ADOPTABLE`；服务方排在取消之前。「服务方不一致答冲突」落为 `PROVIDER_MISMATCH`（200），与委托提交口同名；同段换委托是另一格 `ADOPTION_CONFLICT`。
5. **连线先落、委托后标，采用成立以外整笔回滚**（决定二）：并发取消那一格连线已落，靠装配回滚撤掉——这是编排对装配的契约，写在编排头注。越权风险点：绕过这只壳直接调编排的调用方要自己守它。
6. **取消订舱的「已开始」按时刻判**（决定三，产品策略）：申请不晚于委托开始时刻的答已开始，开始之后为剩余范围订的可取消；委托已取消不挡取消订舱。越权风险点 3：没有逐订舱的消费关系，时刻是最近一层能判的事实（归票 37 一并考虑）。
7. **取消委托不在册答 `COMMISSION_NOT_FOUND`、已开始的线上名改为 `TRANSPORT_ALREADY_STARTED`、落取消撞上并发读回作答**（决定三）。此前两格没有线上消费方，改名不破任何契约。
8. **订舱取消不改写 `recorded_at`，委托的开始与取消照旧改写**。越权风险点 6：两册不一致，与票 [46](./46-capacity-reservations-have-no-per-reservation-review-registry.md) 对池行时刻的同一问并案，未另立票。
9. **采用与两个取消都不入队**（计划 X3）：今天没有消费者；运输收费发生项登记按段回读连线（票 28）。
10. **任何实际履约段都可以被采用，包括成功收寄立起的揽收段**（决定五）。越权风险点 5：同一揽收任务失败的尝试仍连不上委托。
11. **不核委托成员与段上对象**。越权风险点 4（票 58）。
12. **部分停用 ADR-0098 决定四「今天不建」一句，适用场景只限实际履约段与委托之间的连线**（决定五）：停用理由的前提（「先定一种采购组织方式」）在逐段声明这一形态上不成立；揽收任务与尝试 ↔ 委托仍不建。
13. **种类粒度**：三种决定各自授予（越权风险点 7，归 psb/07）。

### 用例

- `AT-TF-181`、`AT-TF-182`：`application/adopt_segment_commission_test.go` 的 `TestAdoptingACommissionStartsItAndCancellationThenAnswersAlreadyStarted`；`domain/segment_commission_adoption_test.go` 的 `TestAdoptingACommissionStartsItFromTheSegmentsFirstParticipation`；真库 `adapters/postgres/segment_commission_adoption_test.go` 的 `TestAWiredAdoptionStartsTheCommissionAndCancellationThenAnswersAlreadyStarted`、`cmd/parcel-api/assemble_segment_commission_adoption_test.go` 的 `TestTheWiredAdoptionFaceStartsTheCommissionOfASegmentEstablishedByHandover`；线格式 `adapters/http/commission_adoption_and_cancellation_faces_test.go`。
- `AT-TF-183`、`AT-TF-184`：`TestAnAdoptionAnswersEachUnmetPreconditionWithoutWriting`、`TestAnAdoptionRefusesEachUnmetPrecondition`、`TestTheFirstEffectiveParticipationSkipsSupersededAndVoidedVersions`、线格式两个用例、`TestCancellationOnlyReachesUnstartedCommissions`。
- `AT-TF-185`、`AT-TF-186`：`application/cancel_transport_intent_test.go` 的 `TestBookingCancellationOnlyStopsUnconsumedBookings`；真库 `TestABookingCancellationLandsOnceAndKeepsTheRecordingTime`。
- `AT-TF-187`：`TestCommissionCancellationRacingAStartAnswersFromTheWinner`、`TestAdoptionRecoveryAndConcurrency`；真库面「取消后再采用答不可采用且零写入」在 cmd 用例里。
- `AT-TF-188`：`TestTheWiredAdoptionFaceRefusesAnOperatorWithoutTheAdoptionGrant`、`TestEachTFDecisionFaceAuthenticatesForItsOwnDecision`、TF 认证方逐种互斥用例、access-identity 真库册面 `TestTheAdoptionAndCancellationDecisionsAreGrantedAndReadBack`。
- 库面：同段第二份答已登记、外键挡住不在册的两端（`TestTheAdoptionRegisterKeepsOneAdoptionPerSegmentAndNeedsBothEnds`），两写口无事务即拒（`TestAdoptionAndBookingCancellationWritesRefuseToRunOutsideATransaction`）。

### 验证

- `go build ./...`、`go vet`（accessidentity、transportfulfillment、cmd/parcel-api、scripts/demo-seeds、migrations）、`gofmt -l`（本分支改动的 Go 文件）干净。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1 ./internal/accessidentity/... ./internal/transportfulfillment/... ./cmd/parcel-api/ ./internal/architecture/ ./migrations/` 全 ok。全量与机制清点未跑（进 main 时统一做）。
- 演示种子：一次性库 `dcg25_seed` 上 `seed.sh` 灌入退 0，三种授予与对照落库，用完删库。
- 演示（S，实测）：一次性库 `dcg25_demo` 上 `seed.sh` 退 0，起 `parcel-api`（操作者渠道与治理自授权）经配置员令牌：干线委托（`v2`）`201`；场外设备经现有交接口在 SZX 交给干线承运方立 `SYN-SEG-SZX-SIN-DCG25` `201`；服务方填末端承运商 `200 PROVIDER_MISMATCH`；段写错 `200 SEGMENT_NOT_FOUND`；**采用 `201 COMMISSION_ADOPTED`**，委托开始依据是那一次交接、开始时刻是交接时刻；再采用 `200 EXISTING_ADOPTION`；**取消委托 `200 TRANSPORT_ALREADY_STARTED`**；不带令牌 `401`；订舱不在册 `200 BOOKING_NOT_FOUND`。走完已停进程并删库。

### 偏离与待裁

- 迁移 TF 用 `0035`（票面 0030 已被票 20 占，号由协调方分配）。
- 票面「`BookingStore.Replace`」落为新口 `BookingCancellationStore.Replace`，不改 `ports.go`。
- 票面「服务方不一致答冲突」落为 `PROVIDER_MISMATCH`（见代决 4）。
- `CancelCommission` 的两处线上答法变了（不在册、已开始的名字），此前没有线上口，不破契约；评审请核一遍没有别处按旧串比。
- 待裁：票 57、58；越权风险点 6 并入票 46；风险点 3 归票 37。
- 热点文件都只追加：`endpoints.go`、`main.go`、`operation_decision.go`、`operator_decision_intake.go`、`seed.sh`、`docs/adr/README.md`（ADR-0098 那一行按「部分停用」规则在行尾追加被停条款与适用场景）、种子 README（授予表那一格在括号末尾追加三种）。与票 21 同期合入时，UC-TF-004 验收表尾、`docs/adr/README.md` 已接受节末会有表尾追加的文本冲突。
