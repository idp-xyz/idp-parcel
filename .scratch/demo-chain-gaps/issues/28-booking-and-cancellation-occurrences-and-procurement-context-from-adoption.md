# 28 订舱与取消发生项；实际履约经段采用取采购上下文

Category: enhancement
Status: resolved——2026-10-19 进 main（全量钉 `b6e8b7f9`：152 ok / 0 FAIL；59、28 同批，见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-dcg28（见文末「完成记录」；隔离评审 0 阻断，修订见「评审记录」）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [24](./24-booking-references-a-schedule-and-carrier-answers-via-integration-client.md)、[25](./25-segment-adopts-transport-commission-and-cancellation-faces.md)、[27](./27-transport-charge-occurrences-are-registered-and-handed-off.md)
地盘：TF `domain/transport_charge_occurrence.go`；`register_charge_occurrence.go`；新读端口文件；tfhttp `register_charge_occurrence.go`。
出处：[07](./07-transport-charge-occurrences-have-no-production-path.md)；方案「07 运输收费发生项」D07-2、D07-3，「需要的 ADR」ADR-0207，「已不成立或已修」07。

## 做什么

- 同一登记用例补订舱与取消两种原因，各自的事实依据照方案「需要的 ADR」ADR-0207。
- 段已采用委托（[25](./25-segment-adopts-transport-commission-and-cancellation-faces.md)）时，实际履约的采购上下文从委托快照取，与调用方值的冲突答法照方案 D07-2。

## 完成判据

- 订舱的依据是已接受的答复；取消的依据是未开始范围。
- 段已采用委托时可以省略采购上下文，给了但不一致答冲突。
- 真库测试。
- 演示：干线订舱被接受后登订舱费发生项。

## 规模

M

## 完成记录

2026-10-19 作者子代理在分支 `claude-dcg28` 上做完，基于派单时的 origin/main `2c6311f1`；收工前 rebase 到 origin/main `de9e3ad9`（A 波 60、64、68 进 main 之后）。rebase 只有 `docs/adr/README.md` 一处冲突：主线去掉了 ADR-0207 等几行重复，取主线那一行、行尾追加本票的补记说明。号段照协调方预分配：迁移 TF `0038`，AT `AT-TF-206` 至 `215`，立票 `76`，评审后立 `77`。没有新 ADR：补充裁定照派单口径写进 [ADR-0207](../../../docs/adr/0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) 文末补记，正文不改。

中途协调方误起了本票的第二个作者实例，与本实例并写同一 worktree 约两分钟后停下。它留下的半成品（装配接委托一侧各读册、tfhttp 载荷加 `booking` 与冲突回显委托、失败尝试拒带订舱、采用端口头注）逐个审过：与本实例的设计同名同形，并入，没有重复或相左的改动，也没有它的提交。

提交（rebase 之后的号）：`6bf46979`（委托记下协议版本、迁移 TF 0038）、`b41f3f3f`（领域、端口、编排、tfhttp、装配）、`078c2616`（演示种子干线 v3）、`ad72020d`（parcel-api 真库装配用例）、`922f515b`（ADR-0207 补记、UC、CONTEXT、演示文档）、`0de110fd`（立票 76）、本记录。

### 做了什么

- **委托记下指名的协议版本**：`TransportCommission.AgreementVersion()`，提交时从草稿带下，重建可缺；委托册 INSERT 与读回带两列；迁移 `transport_fulfillment/0038_transport_commission_agreement_version.sql` 加 `agreement_object`、`agreement_version` 两列，可缺、成对 CHECK（只缺一列时 btrim 比较得 NULL、CHECK 当通过，所以「都在」一支另写 `IS NOT NULL`——真库用例先撞出了这一格）。
- **领域**（`domain/transport_charge_occurrence.go`）：`BookingChargeFact`（订舱、委托、当前答复）；`BookingOccurrenceGrounds` / `CancellationOccurrenceGrounds` 交依据、范围、业务时间；形成门 `ChargeOccurrenceForBooking` / `ChargeOccurrenceForCancellation`；范围 `BookingOccurrenceScope`；哨兵 `ErrBookingNotAnswered`、`ErrBookingNotAccepted`、`ErrBookingNotCancelled`、`ErrCancelledAfterTransportStart`、`ErrNotACommissionMember`。
- **端口**：新文件 `ports/charge_occurrence_commission_source.go`，只读口 `SegmentCommissionAdoptionView`、`TransportCommissionView`、`BookingView`、`BookingAnswerView`，形与各存取口的读方法相同，postgres 适配器天然满足；不动 `ports.go`。
- **编排**：`register_charge_occurrence.go` 接上订舱与取消（事实一半在新文件 `register_booking_charge.go`）；实际履约按段读采用连线；事实带委托时 `procurementOf` 取委托的服务方与协议版本、`declaredAgainstCommission` 判报值相抵、核协议之后法人与快照取委托；新细格 `PROCUREMENT_CONTEXT_CONFLICT`（冲突）、`PROCUREMENT_SOURCE_UNAVAILABLE`（未决）、`BOOKING_NOT_ANSWERED`、`BOOKING_NOT_ACCEPTED`、`BOOKING_NOT_CANCELLED`、`CANCELLED_AFTER_TRANSPORT_START`、`OBJECT_NOT_IN_COMMISSION`（未受理）；`REASON_NOT_REGISTRABLE` 退场；数据违约上抛（`ErrAdoptionWithoutCommission`、订舱的委托读不回沿用 `ErrBookingWithoutCommission`、答复读口所答非所问 `ErrBookingAnswerMismatch`）。
- **tfhttp**：载荷加 `booking`；冲突那一格回显 `commission`（委托、服务方、协议快照、法人，记下了时另带协议对象与版本）。
- **装配**：`cmd/parcel-api/assemble_charge_occurrence.go` 的真依赖加采用连线、委托、订舱、答复各册。未动 `endpoints.go`、`main.go`、`seed.sh`、生产接线棘轮基线（新函数都是 `New*` 或方法，不进棘轮网）。
- **演示种子**：发布批在干线 `v2` 之后加 `v3`（批准 `SYN-APPROVAL-2026-021`，摘要 `PCC-2:72bdfa80…`，由 PC 发布用例的对账门算出），条件：订舱、取消按订舱计，实际履约按件计，失败尝试不计费。受控发布答复的钉值随之多一项（`cmd/parcel-commercial/publish_answer_test.go`）。
- **文档**：ADR-0207 Status 行指向文末补记、补记全文与越权风险点（接正文编号续写）；ADR-0209 Links 加前向指针（回答其风险点 2）；`docs/adr/README.md` ADR-0207 那一行行尾追加补记与收窄的适用场景；UC-TF-004 步 7 与 `AT-TF-206` 至 `210`、`212` 至 `215`；UC-TF-005 步 9 与 `AT-TF-211`；TF CONTEXT「运输收费发生项与成本来源」加一条；演示文档第 3 步补记、种子 README 第 8 步一段。
- **立票**：[76](./76-cancellation-charge-only-registers-at-booking-level.md)（取消费只认订舱一级、部分接受不核数量，待 TF、PC owner 裁）。

### 代决与越权风险点

用户原话「你作为业务和系统专家自决……解决所有的问题」；以下是方案 D07-2、D07-3 与 ADR-0207 正文都没裁、本票自决的细节，裁定已写进 ADR-0207 文末补记，越权风险点在那里接正文编号续写。

1. **订舱的事实是定局的接受**：没有答复与只有候补是还没定局，拒绝、失效、撤回是没成约，各自成格；业务时间取接受的答复时刻，范围取那一次订舱（ADR-0209 决定二已定），成员须都是委托的成员。越权风险点 8：部分接受不核数量（票 76）。
2. **取消只认订舱一级，且须是接受过的订舱**：整份委托的取消没有一形配得上（回答 ADR-0209 风险点 2）；接受晚于取消不算——取消那一刻没有成约。越权风险点 9（票 76）。
3. **「尚未开始」按业务时间、与 ADR-0204 决定三同一边界，按登记那一刻的委托判**：委托开始被前移后，取消落在开始之后、订舱在开始之前提出的不形成。越权风险点 10：此前已形成的取消项不重判，去处是发生项修订入口（ADR-0207 Consequences「已知限制」），不另立票。
4. **事实带委托时采购上下文取委托，报值相抵答冲突**：复用冲突格、加细格 `PROCUREMENT_CONTEXT_CONFLICT` 并回显委托，不另立结果格（ADR-0207 越权风险点 1 同一取舍）；在委托事实上声明自营同样答冲突。
5. **法人与快照取委托、协议仍在业务时间问 PC**：问的是委托记下的那一版；PC 核回的快照与委托不同是数据走岔，上抛。评审后补：PC 对那一版答不在册、供应商不符同样上抛（委托提交时核过、版本不可变，这两格不会是如实答案）。
6. **委托记下协议对象与版本（迁移 0038），可缺而不拒迁移**：委托提交口已在生产接通，表里可能有行；旧行如实答没记下，此时协议要登记方给；评审后补：报的那一版答不在册、供应商不符或快照不同都答冲突回显委托，未生效、已终止照协议否定格答。越权风险点 13。
7. **段采用后不问实际承运商判断**：收窄 ADR-0207 决定二不适用第二格，只限有采用连线的段。越权风险点 11。
8. **实际履约从委托取上下文时不核段成员是否在委托里**：同 ADR-0204 风险点 4。越权风险点 12（票 58）。
9. **演示协议以 v3 再发，不就地改 v2**：版本一经发布不可变（ADR-0209 决定一），ADR-0209 决定六写的 v2 照旧成立；票 30 就地改成本卡的理由（两版并用答适用冲突）在供应商协议上不成立。越权风险点 14：在册版本再多一份。
10. **订舱费与取消费的 BUY 触发行不登**：演示租户没配那两行，结算答未配置，是如实答案；本票只证到 201。
11. **（评审后）已取消而还没定局答「订舱未定局」**：取消不封答复口，取消之前作出、之后送达的接受仍会落册，所以续办「等承运方定局」不落空；不改答「未被接受」。
12. **（评审后）实际履约读采用连线不加锁**：以登记那一刻读到的为准，晚到或并发的采用不回头重判已形成的项。越权风险点 15，去处同风险点 10。

### 用例

- `AT-TF-206`：`application` `TestAnAcceptedBookingRegistersABookingOccurrenceFromItsCommission`（含重登已存在、另一组成员答范围已计过）。
- `AT-TF-207`：`TestABookingOccurrenceNeedsASettledAcceptanceOverCommissionMembers`（各未受理格、读不动答未决，加委托读不回上抛）。
- `AT-TF-208`：`TestACancellationOfAnUnstartedBookingRegistersBesideTheBookingOccurrence`。
- `AT-TF-209`：`TestACancellationOccurrenceOnlyStopsAnUnstartedScopeByBusinessTime`（含同一刻、开始之后提出、取消早于开始；评审后补已取消而未定局）。
- `AT-TF-210`：`TestADeclaredProcurementContextMustAgreeWithTheBookingsCommission`。
- `AT-TF-211`：`TestAnAdoptedSegmentTakesItsProcurementContextFromTheCommission`；真库装配 `TestAnAdoptedTrunkSegmentRegistersActualFulfillmentFromTheCommission`。
- `AT-TF-212`：`TestACommissionsAgreementIsCheckedAtBusinessTimeAndByItsDeclaredConditions`。
- `AT-TF-213`：`TestALegacyCommissionNeedsTheRegistrantToNameItsAgreement`；真库 `TestACommissionKeepsTheAgreementVersionItWasSubmittedUnder`、`TestTheCommissionAgreementVersionColumnsArePaired`。
- `AT-TF-214`：`TestAnUnreadableAdoptionOrCommissionIsUndecided`。
- `AT-TF-215`（演示判据）：真库装配 `TestABookingAcceptedOnTheSeededTrunkRegistersItsBookingAndCancellationOccurrences`——发布批与干线那一班照种子文件落库，委托指名 `v3`，订舱、合成干线承运客户端答 `ACCEPTED`，订舱费答 201（数量 1 `BOOKING`、快照 `…/v3`、法人 `SYN-LE-01`、一封信封），重登 200 `EXISTING`，报末端承运商 200 `CONFLICT` / `PROCUREMENT_CONTEXT_CONFLICT` 回显委托，取消订舱 201 后取消费 201、两项两封。
- 领域：`TestBookingAndCancellationGroundsComeFromTheFacts`、`TestBookingFactsThatDoNotBelongTogetherAreRefused`、`TestACommissionRemembersTheAgreementVersionItWasSubmittedUnder`。
- 协议核对真库：`TestTheSyntheticSupplierAgreementsAreCheckedThroughTheRealPointRead` 补 v3 声明的各条条件。

### 验证

- `go build ./...` 干净；`go vet` 覆盖 `./internal/transportfulfillment/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-commercial/`、`./migrations/`，干净；本分支改动的 Go 文件 `gofmt -l` 为空。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1`：`./internal/transportfulfillment/...`、`./internal/architecture/`、`./migrations/`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-commercial/`、`./internal/settlementaccounting/adapters/transportfulfillment/`、`./internal/parcelpricing/adapters/transportfulfillment/`、`./scripts/demo-seeds/...`。架构门禁第一次跑撞了事务闭包门（新装配用例在事务回调里调了 `t.Fatalf`），已把断言搬到闭包外。全量与机制清点未跑（进 main 时统一做）。
- 演示种子：一次性库 `dcg28seed` 上 `seed.sh` 退 0，发布批第 11/19 项干线 `v3` `PUBLISHED_EFFECTIVE`，条件子表 `v3` 订舱、取消、实际履约、失败尝试各一行（取证于 `078c2616`）；rebase 之后在 `dcg28seed2` 上重灌一遍，同样退 0、条件行同上；两库用完已删。
- 上面的 go test 一组在 rebase 之后（`de9e3ad9` 之上）重跑。

### 偏离与待裁

- **演示没有在一次性库上经演示 Dex 实走**：派单允许「一次性库实走或真库装配用例证明」，取后者（`AT-TF-215`），它与种子用同一份发布批与班次文件、经两族渠道的生产 Intake 与真编排。
- 立票号 `77` 评审后用于 [77](./77-charge-occurrence-fact-reads-do-not-separate-unrehydratable-rows.md)。
- 热点文件只追加：`docs/adr/README.md` 在 ADR-0207 那一行行尾追加；种子 README 追加一段；发布批在 `v2` 之后插入一项（受控发布钉值随之改）。`endpoints.go`、`main.go`、`seed.sh`、`cmd/parcel-dispatch/assemble.go` 未动。
- `docs/design/synthetic-demo-journey-script.md` 里票 25 的补记段落出现两遍（早先合批留下的），评审后删去较短的那一份（另一份是它的超集）。
- 待裁：票 76（越权风险点 8、9）；风险点 10、15 归发生项修订入口；风险点 11 归票 57；风险点 12 归票 58；事实读口重建失败的分格归票 77。


## 评审记录

2026-10-19 隔离评审：阻断 0 项，非阻断 8 项（1 项建议进 main 前修）。作者子代理在同一分支逐项处理，提交 `e69eb6d5`（代码与迁移头注）、`fc18aee5`（ADR-0207 补记、UC-TF-004、种子 README、演示动线）、`94411fca`（立票 77）、本记录。

### 修了的

1. **ADR-0207 补记「代码与迁移」数端口文件里有几个口**（建议进 main 前修）：改写符号名 `SegmentCommissionAdoptionView`、`TransportCommissionView`、`BookingView`、`BookingAnswerView`。同一条理由顺带清了补记里「四种原因」（改「决定一列的各原因」）与本票完成记录里数别处的几处（「四个只读口」「四册」「九格」「六格」「四条条件」「8–14」），其中「数据违约两格」列的是三项，正是这一类无声变错的现成例子；钉值改述为「多一项」，种子条件行的取证锚到 `078c2616`。
2. **旧委托报的协议被 PC 答否定格，走不到快照比对**：`Register` 在协议核对之后、按格作答之前，事实带委托而 PC 答不在册或供应商不符时，委托没记下版本答冲突 `PROCUREMENT_CONTEXT_CONFLICT` 并回显委托；委托记着版本时上抛（500），与快照走岔同一处理。代决理由：委托提交时 `SubmitCommission` 核过它那一版在册、供应商就是委托的服务方，版本一经发布不可变，这两格不会是委托那一版的如实答案——照协议否定格答，「不在册」叫登记方去 PC 发布，「供应商不符」叫他改一个他没报的服务方。未生效、已终止可能就是委托那一版在业务时间的如实答案，照常答。越权风险点：评审只要求旧委托一侧，记着版本那一侧改上抛是本作者顺着同一推理扩的，若 PC 将来允许已发布版本撤册或改供应商，这一格会从 500 变成该答业务否定格（ADR-0207 补记越权风险点 13 同时补了「报错的那一版恰未生效或已终止时看不出报错」）。用例 `AT-TF-213` 补三格与上抛两格，UC-TF-004 那一行随之改。
3. **采用连线读取不加锁的竞态没写**：`commissionAdoptedBy` 头注写明以登记那一刻读到的为准、采用晚到（含并发）不回头重判，锁住只是把并发改成先后，先后那一种本就合法；ADR-0207 补记新增越权风险点 15，去处同风险点 10（发生项修订入口）。
4. **种子 README 与 ADR 补记复述演示答法**：README 那一段只留 `v3` 再发、批准号与「自此指名 `v3`」，答法与 `v3` 声明了什么改为链到演示动线第 3 步的票 28 补记；ADR 补记的演示一条只留「以 `v3` 再发、`v2` 不改」的取舍，批准号与摘要不复述。
5. **迁移 0038 头注「编排的头注」找不到**：改指 `internal/transportfulfillment/application` 里 `procurementOf` 的头注。迁移未进 main，改头注不撞已施加库的校验和。
6. **已取消而只有候补答 `BOOKING_NOT_ANSWERED`**：读码核实后保留答法，把理由写进 `CancellationOccurrenceGrounds` 与 ADR-0207 补记：`AnswerBooking` 只核答复不早于申请、不因订舱已取消而拒收，承运方在取消之前作出、之后才送达的接受仍会落册，那时取消停的是一份已成约的舱位，所以「等承运方定局」不是落空的续办；改答 `BOOKING_NOT_ACCEPTED` 会把这一格说死。`AT-TF-209` 补「已取消、没有答复」「已取消、只有候补」两格，UC 那一行随之改。
7. **演示动线票 25 补记重复两遍**：删去较短的那一份（另一份是它的逐字超集，多一句评审后补的答法）。

### 没采纳的与去向

- **TF postgres 读口重建失败混进未决**：不在本票修——段读口是票 27 起的既有做法，订舱、答复、委托、连线照同一做法，要改得在 TF `ports` 立哨兵、各读适配器一起包，越出本票地盘。立 [77](./77-charge-occurrence-fact-reads-do-not-separate-unrehydratable-rows.md)，写明可与 [54](./54-charge-occurrence-registration-does-not-separate-deterministic-agreement-check-failures.md)（协议核对的确定性失败）合并。
- **演示动线第 3 步上文的「指名 v2」就地改成 v3**：不改。上文各条是带日期的补记，其中票 23、24、25 的几段记着当时在一次性库上实走的证据（那时指名的就是 `v2`），就地改写会让证据说成走过了没走过的版本；这份文档的既有做法是新补记写「上面各条自此读作」（票 29 把 `v1` 读作 `v2` 同此）。重复段已删（上面第 7 条）。

### 评审后的验证

- `go build ./...` 干净；`go vet` 覆盖 `./internal/transportfulfillment/...`、`./cmd/parcel-api/`、`./migrations/`，干净；本分支改动的 Go 文件 `gofmt -l` 为空。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1`，在 `94411fca` 上（代码与 `e69eb6d5` 相同）：`./internal/transportfulfillment/...`、`./migrations/`、`./internal/architecture/`、`./cmd/parcel-api/`、`./cmd/parcel-commercial/`、`./cmd/parcel-dispatch/`、`./internal/settlementaccounting/adapters/transportfulfillment/`、`./internal/parcelpricing/adapters/transportfulfillment/`、`./scripts/demo-seeds/...`，全部 ok。
- 种子数据没动（只改了 README 一段），没有重灌 `seed.sh`。全量与机制清点未跑（进 main 时统一做）。

## 进 main 记录（2026-10-19）

- 59 与 28 合成一批，按 59 → 28 叠到 main 上，整批快进推送，与本记录同一次推送。只有 UC-TF-005 的追加段要合，两边都留。
- 合批种子在一次性新库上 `seed.sh` 灌通，退 0，用完删库。
- 全量 `go test -p 1 -count=1 ./...`（带 55432 DSN）在干净游离检出上钉 `b6e8b7f9`：152 包 ok，0 FAIL，0 SKIP。机制清点在同一 tip 上重生成。
