# 177 订舱当前答复的选法在读面与写面各实现一遍

Category: enhancement
Status: in-progress——2026-10-26 评审无阻断、修订轮完，待进 main，分支 `claude-dcg177` tip `11d4cba56`（其后一笔只改票面与 AT 盘点），见文末「评审记录」「修订轮」。此前 in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-dcg177` tip `982101163`（其后一笔只改票面与 AT 盘点两行），见文末「完成记录」。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领，分支 `claude-dcg177`（worktree `idp-parcel-dcg177`）。此前 ready-for-agent——2026-10-26 Claude Code 作者子代理按票 [48](./48-booking-requests-and-carrier-answers-have-no-read-face.md) 隔离评审 Spec 非阻断 1 立（会话 `tops-20` 派单；用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」授权）。分级 R3（不急）：迁移 0029 的两条部分唯一索引兜着，凡能经答复编排落库的答复，两处选出的是同一份，今天没有答错；规则一变（例如 ADR-0202 越权风险点 13 放宽候补的形）两处才会分叉，到那时须与规则改动同笔改，不能先上。
Blocked by: 无
出处：票 48 完成记录「代决与越权风险点」第 5 条；票 48、127、167 隔离评审（钉 `c428ad2fc`）Spec 非阻断 1；[ADR-0202](../../../docs/adr/0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md)「补记：票 24 落地时的补充裁定」里「候补之后可接一份定局」一条的读法。
地盘：TF 领域 `domain/transport_commission.go`（`CarrierAcceptance` 一组）与它的测试、`adapters/postgres/booking.go` 的 `BookingAnswers.FindByKey`、`adapters/postgres/review_catalogue.go` 的 `bookingStanding`（+ 各自测试）。

## 现象

一次订舱名下「当前那一份答复」——有定局取定局，否则取候补——今天写在两处：

- 写面：`BookingAnswers.FindByKey` 用 SQL 排序选（`ORDER BY (outcome <> 'WAITLISTED') DESC, decided_at DESC LIMIT 1`），遇到绕过编排落库的异常行（两份定局、两份候补）静默取最新那一份；运输收费发生项的订舱事实经它读当前答复。
- 读面：票 48 的 `ReviewCatalogue.bookingStanding` 在 Go 里按答复值字符串比 `WAITLISTED` 选，遇到同样的异常行答错误、整页 500。

两处的选法今天等价，靠的是库上 `booking_answer_one_settling`、`booking_answer_one_waitlist` 两条部分唯一索引；一处单一权威写成了两份，异常行上的处置也不同。

## 做什么

- 把选法收进领域层：`CarrierAcceptance` 已有 `Settles()`（候补以外四值都是定局），在旁边加一个从同一订舱名下的答复里选出当前那一份的函数（有定局取定局，否则取候补；两份定局或两份候补答错误），作为唯一定义。
- `BookingAnswers.FindByKey` 改为读回名下全部答复再调它，不再靠 SQL 排序选；`bookingStanding` 也调它，用 `Settles()` 判，不比字符串。
- 两处对异常行的处置随之统一为答错误（不静默挑一份）；写面上这会让收费发生项登记在异常行上答没形成答案而不是按最新那一份登——实施时核一遍调用方的错误代数（ADR-0029），写进完成记录。

## 完成判据

- 领域用例：只有候补、候补加定局、只有定局、无答复、两份定局、两份候补各一格。
- 真库：`FindByKey` 与查阅口订舱册在同一组答复上选出同一份；把异常行直插进库，两处同样答错误。
- 全仓再找不到第二处按答复值字符串或 SQL 排序挑当前答复的写法。

## 三类

机制。规模 S。

## 完成记录（2026-10-26，Claude Code 作者子代理，会话 `tops-20` 派单，分支 `claude-dcg177`）

起点：`origin/main` `b63cf1997`（含认领）。

| 提交 | 内容 |
|---|---|
| `982101163` | 领域选法、写面与读面改调、收费发生项登记的错误代数，及四层用例 |
| 完成记录那一笔 | 本票完成记录；AT 盘点 `AT-TF-151`、`AT-TF-332` 两行补点名 |

### 做了什么

- 领域 `domain/transport_commission.go`：`CurrentCarrierAcceptance(answers []CarrierAcceptance) (int, error)` 以 `Settles()` 为判据从一次订舱名下的全部答复里选当前那一份，交位置（没有答复交 -1），有定局取定局、否则取候补，与排列次序无关；名下答复凑不成 ADR-0202「补记：票 24 落地时的补充裁定」那一种答复史——两份定局、两份候补、定局不是候补的合格后继（`AdmitsSuccessor`：早于候补、在别的订舱上）、混进未成形的答复——答新哨兵 `ErrBookingAnswersInconsistent`。
- 写面 `adapters/postgres/booking.go`：`BookingAnswers.FindByKey` 不再 `ORDER BY (outcome <> 'WAITLISTED') DESC, decided_at DESC LIMIT 1`，改为读回名下全部答复（与 `FindAnswersForUpdate` 共用一条语句 `bookingAnswersOfBooking`）再调领域选法，选不出时错误包着哨兵。行的读回拆出 `rehydrateCarrierAcceptance`，写面与读面共用。
- 读面 `adapters/postgres/review_catalogue.go`：`bookingStanding` 把每行经 `rehydrateCarrierAcceptance` 读回领域值，调领域选法，再按 `Settles()`、`Outcome()`、`AcceptedQuantity()` 派生状态词，不再比答复值字符串；错误照旧整页答 500（`ListBookingRequests` 以 `%w` 包着，哨兵透得出来）。状态词的派生仍只在这里（要申请量，是读口的事）。
- 应用 `application/register_booking_charge.go`：`bookingFact` 读答复遇 `ErrBookingAnswersInconsistent` 上抛（端点答 500 `NO_ANSWER_FORMED`），与上面「委托读不回」同属数据违约；其余读答复的错误照旧答未决 `FACT_SOURCE_UNAVAILABLE`。订舱费与取消费两种原因都走这一处。
- 端口注释：`ports/charge_occurrence_commission_source.go` 的 `BookingAnswerView` 写明选法出处与哨兵契约；`ports/catalogue_read.go` 状态词注释指向领域选法。
- 未改：迁移（0029 两条部分唯一索引照旧兜着）、ADR、CONTEXT、GLOSSARY、CONTEXT-MAP、UC、`cmd/` 装配。

### 代决与越权风险点（用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」）

1. **凑不成答复史的形一律响亮失败**（机制）。其中定局早于候补是库约束兜不住的那一种，另两种有索引兜着、索引不在时才露出来。两面此前的答法与现在：
   - 定局早于候补（库上两条部分唯一索引、CHECK 都拦不住，迁移 0029 注释写明由编排判）：此前两面都静默取那份定局（查阅口读作已确认等、收费发生项按它登）；现在两面都答错误——查阅口整页 500，收费发生项登记 500 `NO_ANSWER_FORMED`、零写入。
   - 两份定局、两份候补（索引兜着，只有拆了索引才落得进）：此前写面静默取最新一份、读面报错；现在两面都答错误。
   - 能经答复编排落库的形（无答复、只有候补、候补加不早于它的定局、只有定局）两面答案不变。
   取这个答法的理由：写面答未决会把登记方引去重试一个重试一万次也不变的读，正是 ADR-0029 否掉的误并；按最新或定局优先挑一份是替承运方下结论；与查阅口坏行整页 500（票 48 代决 6）同一做法。读了：ADR-0029 Decision 与 Consequences、ADR-0202 补记「候补之后可接一份定局」、迁移 0029 注释、`AnswerBooking` 判后继那段。代价：一次订舱的答复册被绕过编排写坏，它的订舱费、取消费登不进去，查阅页那一租户的订舱册整页看不到，直到有人修数。owner 若要写面宽容（例如按定局登），改 `bookingFact` 对哨兵的译法，不改领域选法。
2. **写面错误代数靠哨兵分格**（机制）。`BookingAnswerView.FindByKey` 的错误里，包着 `ErrBookingAnswersInconsistent` 的是「重试不会变」，其余是「读不动、可重试」；`bookingFact` 用 `errors.Is` 分开（`TestABrokenAnswerHistoryFormsNoChargeAnswer` 两格对照）。单行读不回（集外答复值、形状不对）在写面仍落未决，没有并进来：那两种由库上 `booking_answer_outcome_closed`、`booking_answer_outcome_shape` 钉住，不是本票的「库约束兜不住」，也不是选法的事。评审 Spec 4 补一句：`FindByKey` 读回的是名下全部行，不是当前那一份的坏行也落这一格（未决）；现有约束下这种行落不进来，不改代码、不另立票，留作已知的不对称。
3. **读面改经重建门读回**（机制）。`bookingStanding` 此前直接比行上的串，形状不对的行（例如拒绝带接受量）也照读；现在过不了 `RehydrateCarrierAcceptance` 就整页答错误。那些形都被上一条两个 CHECK 钉住，能落库的行上答案不变。
4. **选法交位置不交记录**（机制）。两面手上的行类型不同（`ports.BookingAnswerRecord`、`ports.BookingAnswerCatalogueRow`），交位置两面各取自己那一行，领域层不必认识端口类型，也不必泛型。
5. **第三处选法没收：`application.currentAnswer`**（越权风险点，留给协调方）。答复编排 `CommissionTransportHandler.AnswerBooking` 判后继用的「头」是 `commission_transport.go` 里的 `currentAnswer`（有定局交定局，没有定局交最后一份候补），同一条规则的第三份实现。它用 `Settles()` 判、不比字符串也不靠 SQL 排序，所以完成判据第三条字面成立；但单一权威没有收全，且它在坏答复史上静默挑一份（两份定局取第一份、两份候补取最后一份）。没动，因为 `commission_transport.go` 在票 126（分支 `claude-dcg126`，评审中）的地盘里；126 的改动离这个函数远，进 main 之后改成调 `domain.CurrentCarrierAcceptance` 是一个函数的事，但要先定 `AnswerBooking` 在坏答复史上答什么（倾向：上抛不形成答案，同本票；今天对两份定局实际答冲突）。因此领域与端口注释没写「只在一处定义」，只写两面经它。
6. **`ports.go` 的 `BookingAnswerStore` 注释没改**。它复述了「有定局交定局，没有定局交候补」，仍然对；文件同在票 126 地盘里，留着，等第 5 条一起处理。
7. **不立新 AT**。这是重构收口：能落库的形答案不变；凑不成答复史只可能是绕过编排写的行，属数据违约，同票 48 代决 6、UC-TF-004 `AT-TF-207`「订舱在册而委托读不回是数据违约」的口径，没有单立验收号。新用例点名已有的 `AT-TF-151`（当前答复是定局）、`AT-TF-332`（行上带当前那一份），AT 盘点①这两行补了用例名；写面两条用例只点本票。若 owner 要把「订舱名下答复凑不成答复史也是数据违约」写进 `AT-TF-207` 的失败边界，UC-TF-004 同在票 126 的地盘里，留给它进 main 之后。
8. **测试里拆索引**。真库用例为让「两份定局」「两份候补」落得进来，在 pgtest 自建的一次性库上 `DROP INDEX` 那两条部分唯一索引；迁移不改，生产库照旧。

### 完成判据 → 用例

| 判据 | 用例 |
|---|---|
| 领域：只有候补、候补加定局、只有定局、无答复、两份定局、两份候补各一格 | `domain/booking_schedule_and_waitlist_test.go::TestTheCurrentAnswerIsTheSettlingOneElseTheWaitlist`（另含定局排在候补之前、与候补同刻的定局、四种定局值、定局早于候补、定局在别的订舱上、混进未成形的答复） |
| 真库：`FindByKey` 与查阅口订舱册在同一组答复上选出同一份；把异常行直插进库，两处同样答错误 | `adapters/postgres/booking_current_answer_test.go::TestTheWriteAndReadFacesChooseTheSameCurrentAnswer`（七种形各一个租户，两面分别断言选出的答复标识或哨兵；两份定局、两份候补在拆了索引的一次性库上落，定局早于候补不拆索引也落得进） |
| 全仓再找不到第二处按答复值字符串或 SQL 排序挑当前答复的写法 | `git grep -n "WAITLISTED\|BookingWaitlisted" -- '*.go' '*.sql' ':!*_test.go'`（于 `982101163`）只剩：领域枚举与 `Settles`/`AdmitsSuccessor`、写面列值译码 `carrierOutcomeFrom`、答复口线格式枚举表、查阅口状态词常量、迁移 0029 的索引与 CHECK；没有按答复值或排序挑当前答复的。按 `Settles()` 另选的 `application.currentAnswer` 见代决 5 |
| 做什么：写面在异常行上答没形成答案，核调用方的错误代数（ADR-0029） | 应用 `application/register_booking_charge_test.go::TestABrokenAnswerHistoryFormsNoChargeAnswer`（订舱费、取消费两种原因：哨兵上抛、不问协议、零写入；同一读口读不动照旧答未决）；生产装配真库 `cmd/parcel-api/booking_charge_broken_answer_history_test.go::TestABookingChargeOnABrokenAnswerHistoryFormsNoAnswer`（经两口真落订舱与接受，直插一份答复时刻在后的候补，登订舱费答 500 `NO_ANSWER_FORMED`、发生项与信封零行；删掉那一行，同一请求答 201 `REGISTERED`） |

**变异**（于 `982101163` 的工作树，改完即复原，`git status` 干净）：

- 把领域选法改成「有候补取候补」：领域用例三格红；写面 `TestABookingKeepsAWaitlistAndOneSettlingAnswer`（「有定局时当前那一份不是定局」）与本票真库用例「写面选出 "W"，want "A"」红，读面 `TestTheBookingRegistryReadsEachBookingsStandingFromItsAnswers`（SYN-BOOKING-FULL 读成 WAITLISTED）与本票真库用例「读面选出 "W"，want "A"」红——两面同时变红。
- 把「定局须是候补的合格后继」一格关掉（即此前两面在定局早于候补上的行为）：领域用例两格红；本票真库用例「定局早于候补」写面、读面各报一条红（写面交回那份接受、读面读作 CONFIRMED）；生产装配用例红，登订舱费答 201 `REGISTERED`。

### 自验

于 `982101163`：`go build ./...`、`go vet ./...` 退 0，`gofmt -l internal cmd migrations tools` 无输出；带 DSN（`127.0.0.1:55432`，pgtest 每例自建自删库）`go test -count=1 -p 1` 跑 `./internal/transportfulfillment/...`、`go list` 反查出依赖 TF 的 `./cmd/parcel-api/...`、`./cmd/parcel-dispatch/...`、`./cmd/parcel-transport-register/...` 与六个上下文的 `adapters/transportfulfillment` 包，加 `./internal/architecture/...`、`./migrations/...`：有用例的 23 个包全绿（含 PG），0 FAIL。新真库用例 `TestTheWriteAndReadFacesChooseTheSameCurrentAnswer`、`TestABookingChargeOnABrokenAnswerHistoryFormsNoAnswer` 逐个 `-v` 确认 PASS、不是 SKIP。未跑全量 `scripts/test-full.sh`，未重生成 ADR 索引、AT 计数与机制清点；无迁移、无新 ADR、`apps/admin-web` 未动。

## 评审记录（2026-10-26，协调方 `tops-20` 隔离子代理，钉 `9449e58bd`：两轴无阻断，非阻断 Standards 2 条、Spec 4 条；照抄，小节标题降两级挂在本节下）

评审 ← tops-20 隔离子代理 · 钉 `9449e58bd` · 02:10

结论：两轴无阻断；非阻断 Standards 2 条、Spec 4 条。检出未改。

#### Standards
非阻断：
1. `adapters/postgres/review_catalogue.go` `ReviewCatalogue` 类型注释仍写「目录上列是照实转写，不经领域重建门」，而 `bookingStanding` 现经 `rehydrateCarrierAcceptance` 读回——同文件注释与代码相抵，给订舱册补一句例外。
2. `ports/ports.go` `BookingAnswerStore` 注释仍复述「有定局交定局，没有定局交候补」（规则第三份文字，作者代决 6）；票 126 已进 main，改指 `domain.CurrentCarrierAcceptance`，与 Spec 1 同笔。
无发现：边界、中文注释、跨文件引用；无新 AT/迁移/ADR 号；对 `803b97e5f` merge-tree 无冲突。

#### Spec
非阻断：
1. 第三处选法 `application.currentAnswer` 未收。建议本票修订轮收：重放判（`answerWithAcceptance`）与构造门仍在前；判后继改调 `domain.CurrentCarrierAcceptance`，遇哨兵上抛、零写入、不记挡回（与同编排里「订舱在册而委托读不回是数据违约（500）」同格，ADR-0202 补记「承运答复的次序与格」）。不答冲突：冲突是承运方据以停止重发的业务结论，修数时若删的恰是直插的定局，承运方真答复会永远丢；答 500 让它退避重发。同笔改 `ports.go` 注释；UC-TF-004 `AT-TF-152` 或 `AT-TF-154` 失败边界补一句；应用层加一格用例：[定局(t0), 候补(t1)] 上新答复上抛哨兵，同标识重放仍答 `EXISTING_ANSWER`。
2. UC-TF-004 `AT-TF-207` 失败边界没写「订舱名下的答复凑不成一段答复史，是数据违约（500）」——不相抵，只是缺一格，修订轮补。
3. 变异「取最新、同刻取后排」存活：领域注释说「answers 的次序不影响选法」，同刻那格没用例钉——补一格「与候补同刻的定局排在候补之前」。
4. 写面错误代数剩一处不对称：答复史凑不成答 500，单行过不了重建门（CHECK 钉住）仍答未决 `FACT_SOURCE_UNAVAILABLE`；`FindByKey` 读回名下全部行，非当前那份的坏行也落这一格。作者代决 2 已披露；现有约束下落不进这种行。
无发现：完成判据四条兑现；领域、应用、真库 postgres、`cmd/parcel-api` 四组用例 PASS 无 SKIP；三包 go vet 干净。

#### 附注
「定局早于候补」经 `AnswerBooking` 落不下来（先落定局时 `AdmitsSuccessor` 对非候补恒假答冲突；先落候补时 `!next.decidedAt.Before(...)` 挡住；判前锁读 `FindAnswersForUpdate`；微秒截断单调；`AT-TF-154` 明文；`booking_answer` 唯一写入方 `BookingAnswers.Save`，种子走 HTTP）——没把合法路径改成 500。错误代数：写面哨兵 500 与 `ErrBookingWithoutCommission`、`ErrBookingAnswerMismatch` 同格，合 `AT-TF-207`；订舱费、取消费只经 HTTP，无自动重试回路被卡；读面整页 500 与票 48 代决 6 一致。基线 overlay 实证：四种合法形两面答案与新实现相同，三种坏形按预期变红。变异：M2 抓到、M1b 抓到、M1 存活（合法答复史上与规格同结果，只是用例没钉「与次序无关」）。探针：scratchpad/probe177/。

## 修订轮（2026-10-26，作者子代理，同一分支）

先 rebase 到 `origin/main` `803b97e5f`（票 126 已进 main），修订笔 `11d4cba56`：`982101163` → `b553d2d5f`，`9449e58bd` → `3697b8217`，无冲突；完成记录里的 SHA 指 rebase 前那两笔，内容逐字相同。逐条去向：

- **Spec 1 第三处选法**：已收。`AnswerBooking` 的重放判（`answerWithAcceptance`）与构造门仍在前，判后继改调 `domain.CurrentCarrierAcceptance`，删掉 `currentAnswer`；遇 `ErrBookingAnswersInconsistent` 上抛（端点 500 `NO_ANSWER_FORMED`）、零写入、不记挡回，不答冲突（理由照评审：冲突是承运方停止重发的依据，修数时删掉的若恰是直插那一份，真答复会永远丢）。用例 `application/booking_schedule_and_answer_source_test.go::TestAnAnswerOnABrokenAnswerHistoryFormsNoAnswer`：[定局(t0)，直落的候补(t1)] 上另一份新答复上抛哨兵、不落、不记挡回，那份定局原样重放仍答 `EXISTING_ANSWER`。领域注释改回「只在这里定义」，三处都经它。完成记录代决 5、6 自此作废。
- **Standards 1**：`ReviewCatalogue` 类型注释补订舱册的例外（答复行先经重建门读回、转写各列照旧原样）。
- **Standards 2**：`ports.go` `BookingAnswerStore` 注释改指 `domain.CurrentCarrierAcceptance` 与哨兵契约。
- **Spec 2**：UC-TF-004 `AT-TF-207` 预期结果改为「订舱在册而委托读不回、订舱名下的答复凑不成一段答复史，都是数据违约（500）」；`AT-TF-154` 预期结果补「册上已有绕过答复编排落下的这类行、订舱名下的答复凑不成一段答复史时，新答复（同一答复标识的重放除外，照 `AT-TF-152`）是数据违约（500）：不落、不答冲突、不记挡回」。写面两条用例的 Covers 改点 `AT-TF-207`，新应用用例点 `AT-TF-154`；AT 盘点①这两行补用例名，不加行。完成记录代决 7 的最后一句自此作废。
- **Spec 3**：领域用例补「与候补同刻的定局排在候补之前」一格；变异「取最新、同刻取后排」现在只这一格红（其余格同规格，符合评审所说 M1 只在次序上可分）。
- **Spec 4**：只在完成记录代决 2 补一句，不改代码。

重跑（于 `11d4cba56`）：`go build ./...`、`go vet`（TF 与 `cmd/parcel-api`）退 0，`gofmt -l` 无输出；`go run ./tools/at-coverage-counts -check` 退 0（没加行，总览不变）；带 DSN `go test -count=1 -p 1` 跑 `./internal/transportfulfillment/...`、`./cmd/parcel-api/...`、`./cmd/parcel-dispatch/...`、`./internal/architecture/...`：有用例的 15 个包全绿（含 PG），0 FAIL；`TestTheWriteAndReadFacesChooseTheSameCurrentAnswer`、`TestABookingChargeOnABrokenAnswerHistoryFormsNoAnswer` `-v` 确认 PASS 不是 SKIP。
