# 48 订舱与承运答复没有查阅面：`待确认`、`候补`只能从两口的答复里看

Category: enhancement
Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-dcg127` tip `cad853912`（其后一笔只改 AT 盘点、票面与一句注释的措辞），见文末「完成记录」。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领，与 127、167 同批，分支 `claude-dcg127`（worktree `idp-parcel-dcg127`）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [24](./24-booking-references-a-schedule-and-carrier-answers-via-integration-client.md) 完成记录与 [ADR-0202](../../../docs/adr/0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 文末补记风险点 15。
Blocked by: 无（文件归 tf-chain-plan「并行车道」车道 A，落地顺序随车道 A 的令牌）
地盘：TF `ports/catalogue_read.go`、`adapters/postgres/review_catalogue.go`、tfhttp `query_transport_fulfillment_records.go`（+ 各自测试）；不碰订舱与答复的写口。

## 现象

票 24 让订舱册（`transport_fulfillment.booking_request`）与承运答复册（`transport_fulfillment.booking_answer`）有了写入方，但 `GET /transport-fulfillment-records` 没有这两本册的 `registry`，管理台也没有页。运营人员提交订舱之后，只能从提交口的答复里知道它在册；承运方答没答、答的是什么，只有承运系统自己看得到答复口的回显。ADR-0202 补记写的读法（订舱名下没有任何答复读作`待确认`、当前一份是候补读作`候补`、接受量小于申请量读作部分确认）今天没有任何一处按它读。

## 做什么

- 查阅口加 `registry=booking-request`：每次订舱一行，带委托、班次、申请量与单位、申请时刻、取消时刻（若有），以及按 ADR-0202 补记读出的当前状态词与当前那一份答复的摘要（答复值、接受量、答复时刻）。
- 查阅口加 `registry=booking-answer`（或并进上一条的明细，二选一由作者定）：同一订舱名下的全部答复按答复时刻先后，候补与它的后继都在。
- 读法只在读口一处派生（ADR-0202 补记的三条），不在页面或适配器各算一遍。

## 完成判据

- 真库：一次订舱无答复读作`待确认`；候补之后读作`候补`；候补后接受读作已确认（接受量等于申请量）或部分确认（小于）；他租户读不到。
- 读口不触发判断、不写任何行（查阅的性质，ADR-0077）。

## 三类

机制。规模 S–M。

## 完成记录（2026-10-26，Claude Code 作者子代理，会话 `tops-20` 派单，分支 `claude-dcg127`，与 127、167 同批）

起点：`origin/main` `449a25508`（含认领）。与 167 同一笔代码（查阅口同三份文件），两票各记各的判据。

| 提交 | 内容 |
|---|---|
| `cad853912` | 本票与 167 的代码与用例；UC-TF-004 `AT-TF-332`–`334`、删交接表「订舱与答复的查阅面」一行；演示动线补记 |
| 完成记录那一笔 | AT 盘点①各行与追记；三张票的完成记录 |

### 做了什么

- 端口 `ports/catalogue_read.go`：`BookingRequestCatalogueRow`、`BookingAnswerCatalogueRow`，状态词常量与读法注释，`ReviewCatalogueRead.ListBookingRequests`；文件头注补一句「订舱一格的状态词是读法不是判断，只在读口实现里派生一处」。
- 存储 `adapters/postgres/review_catalogue.go`：`ListBookingRequests` 先按页截出订舱，再 `LEFT JOIN LATERAL` 把名下全部答复按（答复时刻、落库先后）聚合进同一行，一条语句；`bookingStanding` 是状态词的唯一派生处。
- 线格式 `adapters/http/query_transport_fulfillment_records.go`：`registry=booking-request`，outcome `BOOKING_REQUESTS_LISTED`，键 `bookings`；行形 `bookingRequestRowBody` 内嵌订舱提交口回显的 `bookingRequestBody`，加 `standing`、`currentAnswer`（缺席即无答复）、`answers`（总在场），答复用承运答复口回显的 `bookingAnswerBody`。没碰订舱与答复的写口，只引用它们的回显类型。
- 装配：`cmd/parcel-api/unwired_orchestration.go` 占位读口补 `ListBookingRequests`、`ListBookingAnswerBounces` 两法；生产装配 `endpoints_operations_records.go` 不变（同一只 `tfpostgres.ReviewCatalogue`、同一个操作者查阅读）。
- 文档：UC-TF-004 立 `AT-TF-332`，「给开发的交接」删「订舱与答复的查阅面」一行；`docs/design/synthetic-demo-journey-script.md` 加 2026-10-26 补记；AT 盘点①加行与追记。TF CONTEXT、GLOSSARY、CONTEXT-MAP、ADR 均未改。

### 代决与越权风险点（用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」）

1. **`registry=booking-answer` 并进订舱行**（票面二选一，机制）。一次订舱至多两份答复（迁移 0029 的两条部分唯一索引），看答复的人总是按订舱看；订舱行带 `answers`（全部，候补与后继都在）与 `currentAnswer`（当前那一份）。owner 若要跨订舱按时间看答复，另加 `registry=booking-answer`，订舱行不变。
2. **状态词**（机制）：`PENDING_CONFIRMATION`（待确认）、`WAITLISTED`（候补）、`CONFIRMED`（已确认）、`PARTIALLY_CONFIRMED`（部分确认）照 ADR-0202 补记的三条读法与 TF CONTEXT「订舱」词条的生命周期词；`REFUSED`、`EXPIRED`、`WITHDRAWN` 照答复值。待确认拼 `PENDING_CONFIRMATION`，与交接结果的待确认同拼写。
3. **取消不进状态词，另列 `cancelledAt`**（机制）。取消是运营决定，不是承运答复（CONTEXT「订舱」词条）；CONTEXT「任一对象的成功、失败或取消不能被压缩成其他对象的状态」。代价：「接受了又被取消」要看两格。owner 若要一个合成词，加在读口，不在页面算。
4. **`待补充` 不派生**。ADR-0202 决定五把它与待确认并列为读法，但补记只写了三条读法，订舱册与答复册里也没有哪一格说得出缺什么；读不出来不代填。若承运答复将来带「要补充」那一类，再加一词。
5. **当前那一份的选法两处各写一遍**。`bookingStanding`（Go）与 `BookingAnswers.FindByKey`（SQL 排序）同一条「有定局取定局，否则取候补」；`FindByKey` 在答复写口的文件里，派单不碰写口，没有合一。去处：让 `FindByKey` 改调同一个函数。
6. **读不出状态词的行整页答 500**（机制）。两份定局、两份候补、集外答复值、接受量超申请量——都只可能绕过答复编排落库——`bookingStanding` 答错误，端点答 `NO_ANSWER_FORMED`，不挑一份凑词；与该口准备结果册快照读不懂时同一做法。代价：一行坏，整页看不到。
7. **线格式沿用两口回显形**（机制）。订舱各格照订舱提交口回显（申请量是 JSON 数，不是该口容量几格的十进制串），答复照承运答复口回显（含 `booking` 键）：同一件东西在命令答复与查阅页上不换写法（先例：班次、周期安排「查阅列面与登记答复同一形状」）。代价：同一查阅口里容量量是串、订舱量是数。
8. **管理台页不做**：票面「做什么」没要求。ADR-0202 越权风险点 15 的「管理台」那半句仍开着，去处未立票。

### 完成判据 → 用例

| 判据 | 用例 |
|---|---|
| 真库：一次订舱无答复读作`待确认`；候补之后读作`候补`；候补后接受读作已确认（接受量等于申请量）或部分确认（小于）；他租户读不到 | `adapters/postgres/review_catalogue_booking_test.go::TestTheBookingRegistryReadsEachBookingsStandingFromItsAnswers`（另含拒绝、接受后取消、答复次序、页大小、空册、limit 非正拒）；生产装配真库 `cmd/parcel-api/booking_records_read_test.go::TestTheBookingAndBounceRegistriesAreReadOnTheOperatorChannelOnly`（经两口真落的订舱与接受读作 `CONFIRMED`） |
| 读口不触发判断、不写任何行（ADR-0077） | 同一真库用例：上列前后取订舱册、答复册、挡回册全部行的指纹，逐字相同；端点只接读口（`ReviewCatalogueReader`），不接编排 |
| 读法只在读口一处派生 | `ReviewCatalogue.bookingStanding` 一处；线格式 `adapters/http/query_booking_records_test.go::TestTheBookingRequestRegistryTranscribesRowsWithTheirStanding` 证端点照抄读口给的词与当前那一份 |
| 做什么：每次订舱一行，带委托、班次、申请量与单位、申请时刻、取消时刻、状态词与当前答复摘要；全部答复按答复时刻先后 | 同上三条用例；`TestEmptyFulfillmentRegistersAnswerEmptyArrays`、`TestTransportFulfillmentRecordsQueryAnswers500WhenTheRegisterFails`、`TestUnconfiguredIntakeRefusesEveryTransportFulfillmentRecordsRequest` 的册名表加了这一格 |

### 自验

于 `cad853912`（其后一笔只改盘点、票面与一句注释的措辞，改后 `go build`、`gofmt` 复核过）：`go build ./...`、`go vet ./...` 退 0，`gofmt -l internal cmd migrations tools` 无输出；带 DSN（`127.0.0.1:55432`，pgtest 每例自建自删库）`go test -count=1 -p 1` 跑 `./internal/transportfulfillment/...`、`go list` 反查出依赖 TF 的 `./cmd/parcel-api/...`、`./cmd/parcel-dispatch/...`、`./cmd/parcel-transport-register/...`，加 `./internal/architecture/...`、`./migrations/...`，有用例的 17 个包全绿（含 PG）；本批新真库用例逐个 `-v` 确认 PASS、不是 SKIP。`310154eee` 一笔另在临时 worktree 单独 `go build ./...`、`go vet` 通过。`go run ./tools/adr-index -check` 退 0；`go run ./tools/at-coverage-counts -check` 报总览过期（计数由推送方重算）。未跑全量 `scripts/test-full.sh`，未重生成 ADR 索引、AT 计数与机制清点，未灌种子；`apps/admin-web` 未动，无迁移、无新 ADR。
