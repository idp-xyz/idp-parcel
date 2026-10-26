# 167 承运答复挡回册没有查阅面：运营分不清订舱不在册的那一答是「不归你答」还是「还没录」

Category: enhancement
Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-dcg127` tip `cad853912`（其后一笔只改 AT 盘点、票面与一句注释的措辞），见文末「完成记录」。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领，与 127、48 同批，分支 `claude-dcg127`（worktree `idp-parcel-dcg127`）。此前 ready-for-agent——2026-10-21 票 [145](./145-booking-answers-from-a-non-provider-answer-booking-not-found.md) 的作者子代理立（用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」与常设指令「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0239](../../../docs/adr/0239-a-carrier-answer-from-a-non-provider-is-answered-booking-not-found-and-the-basis-is-kept-on-a-bounce-register.md) 决定二末条与越权风险点 1。
Blocked by: 无（文件与票 [48](./48-booking-requests-and-carrier-answers-have-no-read-face.md) 同属 tf-chain-plan「并行车道」车道 A，落地顺序随车道 A 的令牌；与 48 同做或先后都可）
地盘：TF `ports/catalogue_read.go`、`adapters/postgres/review_catalogue.go`、tfhttp `query_transport_fulfillment_records.go`（+ 各自测试）；不碰承运答复口与挡回册的写口。

## 现象

票 145 之后，承运答复口对「订舱在本租户下不在册」与「答复方不是该订舱所属委托的服务提供方」答同一格 `BOOKING_NOT_FOUND`，凭什么挡回只记在承运答复挡回册（`transport_fulfillment.booking_answer_bounce`，ADR-0239 决定二）。承运方拿到这一答，续办是找租户核对；运营要回答它，就得看挡回册——可 `GET /transport-fulfillment-records` 没有这本册的 `registry`，管理台也没有页，今天只能经库查。

## 做什么

- 查阅口加 `registry=booking-answer-bounce`：每行一个（答复方、订舱、答复标识、依据），带首次与最近一次挡回时刻、次数；按最近一次挡回时刻倒序。
- 依据按 `ports.BookingAnswerBounceBasis` 的词透出（`BOOKING_NOT_REGISTERED`、`SOURCE_NOT_SERVICE_PROVIDER`）；库里存的是定宽码，由仓储一处译，读面不另抄一份码表。这是运营读面，不是承运方读面：只挂操作者渠道，不得开给集成客户端族——开给承运方就把 ADR-0239 决定一合并掉的那一位又交了回去。
- 可按订舱、按答复方筛；有了票 48 的订舱查阅之后，订舱那一行能链到它名下的挡回。
- 只做查阅面。词条已由票 145 立在 TF CONTEXT 的 Language 一节（「承运答复挡回」），页面与查阅口的用词照它。

## 完成判据

- 真库：一个不是服务方的答复与一个订舱不在册的答复各落一行，查阅口两行依据不同、他租户读不到；同一份答复重交只见次数变。
- 读口不触发判断、不写任何行（查阅的性质，ADR-0077）。
- 集成客户端令牌调这一口被拒（认证族不对）。

## 三类

机制。规模 S。

## 完成记录（2026-10-26，Claude Code 作者子代理，会话 `tops-20` 派单，分支 `claude-dcg127`，与 127、48 同批）

起点：`origin/main` `449a25508`（含认领）。与 48 同一笔代码，提交表与「做什么」里共有的几件见票 [48](./48-booking-requests-and-carrier-answers-have-no-read-face.md) 完成记录，这里只记本票的。

### 做了什么

- 端口 `ports/catalogue_read.go`：`BookingAnswerBounceCatalogueRow`、`BookingAnswerBounceFilter`（订舱、答复方两维，空串不筛），`ReviewCatalogueRead.ListBookingAnswerBounces`。
- 存储 `adapters/postgres/review_catalogue.go`：`ListBookingAnswerBounces`（最近一次挡回倒序、同刻按答复方、订舱、答复标识、依据）；`bounceBasisWord` 是 `bounceBasisCode` 的逆——逐个依据问 `bounceBasisCode` 要码，码表仍只在那一处；挡回册的写口文件 `booking_answer_bounce.go` 未改。
- 线格式 `adapters/http/query_transport_fulfillment_records.go`：`registry=booking-answer-bounce`，outcome `BOOKING_ANSWER_BOUNCES_LISTED`，键 `bounces`，每行 `source`、`booking`、`answer`、`basis`、`firstBouncedAt`、`lastBouncedAt`、`count`（十进制计数串）；筛选参数 `booking`、`source`，在册名门之后、渠道门之前判。
- 装配：同票 48（占位读口两法；生产装配不变，这一口本就挂操作者查阅读 `operationsReadFace()`）。
- 文档：UC-TF-004 立 `AT-TF-333`（行、依据、次数、倒序、筛选、他租户、零写入）与 `AT-TF-334`（只挂操作者渠道，集成客户端令牌答 401）；演示动线补记里「不必再查库」；AT 盘点①加行与追记。TF CONTEXT 词条「承运答复挡回」未改，页面与查阅口用词照它。

### 代决与越权风险点（用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」）

1. **立不起来的筛选答坏请求**（机制）。给了却是空串、全空白、带 NUL，或订舱标识超过 512 字节界，答 400 `MALFORMED_REQUEST`，不静默放宽成全册（那会让读的人以为那个订舱没有挡回）。订舱照领域构造门 `NewBookingReference` 判；答复方是集成客户端绑定的来源身份，领域里没有它的类型，只判非空白、无 NUL，不设长度界。
2. **两维同给取交集；只对这一册生效**。别的册收到这两个参数照旧不理（该口对未知参数一贯如此）。
3. **「订舱那一行能链到它名下的挡回」做成链接目标，不做成订舱行上的一格**。读面给 `registry=booking-answer-bounce&booking=<订舱>`；订舱行不带挡回计数——ADR-0239 决定二「订舱的当前读法不读本册」，计数挂进订舱行容易被读成订舱的状态。页面怎么链是管理台的事，票面没要求页面，不做。
4. **次数按十进制串透出**，照该口总单关联数的写法。
5. **只挂操作者渠道**：生产装配里这一口本就挂操作者查阅读；另加类型半边的守卫——集成客户端族的 `IntegrationClientIntake` 不实现 `CatalogueQueryIntake`，哪天实现了，用例先红。
6. **没有为筛选与排序加索引**。挡回册主键（租户、答复方、订舱、答复标识、依据）覆盖按答复方筛；按订舱筛与按最近一次挡回时刻排序是租户内全扫。挡回册没有保留期与聚合（票 170），量起来之后与 170 一起看要不要加索引，那会占一个 TF 迁移号。
7. **读不懂的依据码整页答 500**。库上 CHECK 只许 1、2；读到别的码不造一个词。

### 完成判据 → 用例

| 判据 | 用例 |
|---|---|
| 真库：一个不是服务方的答复与一个订舱不在册的答复各落一行，查阅口两行依据不同、他租户读不到；同一份答复重交只见次数变 | `adapters/postgres/review_catalogue_booking_test.go::TestTheBounceRegistryTellsTheTwoBasesApartAndCountsResubmissions`（另含倒序、四种筛法、页大小、limit 非正拒）；生产装配真库 `cmd/parcel-api/booking_records_read_test.go::TestTheBookingAndBounceRegistriesAreReadOnTheOperatorChannelOnly`（两条真挡回路落的两行、按订舱筛） |
| 读口不触发判断、不写任何行（ADR-0077） | 同一真库用例：重交之后、上列前后三册指纹逐字相同 |
| 集成客户端令牌调这一口被拒（认证族不对） | 生产装配真库用例：两族核验方都是真的 OIDC 校验器、同一只合成发行方签令牌，集成客户端令牌在答复口 201、在查阅口订舱与挡回两格都答 401 `OPERATOR_CREDENTIAL_REJECTED`、不带 outcome；`adapters/http/query_booking_records_test.go::TestTheIntegrationClientIntakeCannotServeTheRecordsQuery`（类型半边） |
| 做什么：按订舱、按答复方筛；依据按 `ports.BookingAnswerBounceBasis` 的词透出、读面不另抄码表 | 线格式 `TestTheBookingAnswerBounceRegistryTranscribesRowsAndPassesTheFilter`、`TestABookingAnswerBounceFilterThatCannotStandIsRefusedBeforeTheChannel`；真库用例里依据是词；`bounceBasisWord` 不带码表 |

### 自验

于 `cad853912`（其后一笔只改盘点、票面与一句注释的措辞，改后 `go build`、`gofmt` 复核过）：`go build ./...`、`go vet ./...` 退 0，`gofmt -l internal cmd migrations tools` 无输出；带 DSN（`127.0.0.1:55432`，pgtest 每例自建自删库）`go test -count=1 -p 1` 跑 `./internal/transportfulfillment/...`、`go list` 反查出依赖 TF 的 `./cmd/parcel-api/...`、`./cmd/parcel-dispatch/...`、`./cmd/parcel-transport-register/...`，加 `./internal/architecture/...`、`./migrations/...`，有用例的 17 个包全绿（含 PG）；本批新真库用例逐个 `-v` 确认 PASS、不是 SKIP。`310154eee` 一笔另在临时 worktree 单独 `go build ./...`、`go vet` 通过。`go run ./tools/adr-index -check` 退 0；`go run ./tools/at-coverage-counts -check` 报总览过期（计数由推送方重算）。未跑全量 `scripts/test-full.sh`，未重生成 ADR 索引、AT 计数与机制清点，未灌种子；`apps/admin-web` 未动，无迁移、无新 ADR。
