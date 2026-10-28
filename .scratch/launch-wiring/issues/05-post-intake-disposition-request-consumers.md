# 05 收寄后服务处置请求信封无下游消费

Category: enhancement
Status: in-progress——2026-10-28 作者完工第二阶段（按用户裁定做待人工处理项与登记口，见文末「第二阶段完成记录」），待隔离评审与进 main，分支 claude-lw-pid2（代码最后一笔见完成记录）。此前 in-progress——2026-10-28 用户裁定改做法（见文末「用户裁定（2026-10-28）」），分支 claude-lw-pid2。此前 in-progress——2026-10-28 TF 半边进 main（`ec3f0848f`；全量 160 ok / 0 FAIL，种子退 0），票不收口，见文末「收口条件」。此前 in-progress——2026-10-28 作者完工 TF 一半并按评审修订（见「评审修订」），余下需 NO、NR 所有者新模型，待协调方裁与隔离评审，分支 claude-lw-pid；此前：2026-10-28 会话 tops-20（idp-parcel-91）立并认领（用户「现在就接线」）
Blocked by: 无

## 做什么

ADR-0332 的范围化请求信封入队后今天没有消费者。按 ADR-0332 请求表接下游（code/37 完成记录「下游承接」一段）：

- TF：拦截、改送、退运、服务终止的运输请求——消费信封，以 `POST_INTAKE_DISPOSITION/<包裹>/<版本>` 作服务处置决定依据启动备用/退运旅程（UC-TF-007，`StartAlternateJourneyHandler`，code/43 已接成口）。
- NO：拦截扣住、退运按退运装载、服务终止扣住。
- NR：改送、退运重新规划，服务终止释放。
- 承接结果读回（`AT-PS-088`）；监管退运转 UC-TF-001（`AT-PS-084`）。

规模大：作者先按各所有者现有能力出切分，能在本票内接的接，需要所有者新模型才接得上的写明并回报协调方，不自造他上下文的模型。

## 完成判据

- 演示租户上做一个「退运」决定，TF 旅程、NO 扣住/装载、NR 规划各有可见落点（真库进程用例）；其余去向同理或写明去处。
- `AT-PS-083`、`084`、`088` 移出④，或写明还差什么。

## 完成记录

起点 `2d0193336`。

**切分（每去向 × 每所有者，今天有没有现成编排可消费信封）**

| 去向 | TF | NO | NR |
|---|---|---|---|
| 退运 | 已接：另起退运旅程（`StartAlternateJourneyHandler`） | 差：按退运装载并交出，NO 无对应模型 | 差：规划退运路径，NR 只有初始路由与重评（`ReassessRoute`），无「按去向规划」 |
| 改送 | 已接：另起备用旅程 | — | 差：按目的地重新规划，目的地引用由交付去向所有者解读，NR 无此入口 |
| 拦截 | 差：「不再安排向前移动」，TF 无扣住模型 | 差：在控制处扣住，NO 无扣住模型 | — |
| 服务终止 | 同拦截 | 同拦截 | 差：释放尚未消耗的计划，NR 无释放入口 |

NO 三项、NR 三项、TF 两项扣住，全需所有者先立模型（扣住事实与解除、退运装载、按去向规划、计划释放），本票不自造。按派单，做完 TF 可接的一半即停。

**改了什么**
- `internal/transportfulfillment/adapters/inbox/post_intake_disposition_consumer.go`：消费门，译载荷，缺键维或时刻为毒丸。
- `internal/transportfulfillment/adapters/parcelshipment/post_intake_disposition_journey.go`：只承接 TF 的 `START_ALTERNATE_JOURNEY`/`START_RETURN_JOURNEY`，调与运营决定口同一只 `StartAlternateJourneyHandler`，依据取信封的 `POST_INTAKE_DISPOSITION/<包裹>/<版本>`、依据种类恒为服务处置决定。
- `cmd/parcel-dispatch/routes_post_intake_disposition.go`、`assemble_post_intake_disposition.go`（新文件，`assemble.go` 未动）。
- `cmd/parcel-dispatch/post_intake_disposition_journey_test.go`：真库进程用例，退运决定经生产交接口入队、派发后落退运旅程；重入队同信封 ID 不开第二条；下一版改送的原旅程指向退运旅程；拦截入账不开旅程且无失败码。
- UC-PS-006「给开发的交接」一行改写已接与未补。

**代决与越权风险点**
1. 新旅程身份取 `JRN/<包裹>/PID-<决定版本>`（确定性派生，仿 ADR-0218 的 `JRN/<对象>`），是作者的产品策略取舍，待 TF owner 复核。
2. 原旅程取 `ActualFulfillmentJourney` 在决定时刻对包裹的判定（旅程册无记录时为 `JRN/<包裹>`），排除以同一依据启动的旅程以保重投稳定；信封本身不带原旅程标识（ADR-0332 载荷只带关系），故由 TF 一侧派生，待 TF owner 复核。
3. 成员取 `[包裹]`：信封不带成员，整件包裹作为一个成员；集运单元等成员展开未做。
4. 启动时刻取决定生效时刻（信封 `decidedAt`），启动人留空（不经运营口）。
5. 旅程册未决或意图未交出时返回哨兵整笔回滚重投；依据种类不是服务处置决定、未受理、来源冲突为非未决，保持 publish_failed。
6. 拦截与服务终止信封到 TF 路原样入账，没有可做动作；其余所有者尚无路，信封只有 TF 一路消费。

**判据对用例**：TF 一半达成（演示租户 `SYN-TENANT-01` 同租户身份、真库进程用例，证据 `S`）；NO 扣住/装载、NR 规划无可见落点，未达。`AT-PS-083` 仅 TF 半边，`AT-PS-084`（监管退运转 UC-TF-001）、`088`（承接结果读回）未动，仍在④。

**自验**：`go build ./...`、`go vet`（cmd/parcel-dispatch、transportfulfillment）、`gofmt -l` 无输出；带 PG DSN `go test -count=1 -p 1` 跑 `./cmd/parcel-dispatch`、`./internal/transportfulfillment/...`、`./internal/architecture/...`、`./internal/platform/...` 全绿；`at-coverage-counts -check` 无报（未新增 AT 号）。未取迁移、ADR、种子号。

**留给协调方**
- 余下 NO（扣住、退运装载）、NR（按去向重新规划、退运路径、计划释放）、TF（扣住）需各所有者新模型，各立票；`AT-PS-088` 承接结果读回依赖各所有者的承接事实。
- 演示动线上「退运」从决定到 TF 旅程可见的是派发进程这一段；管理台页与决定追加口的演示灌入未碰。

## 评审修订

隔离评审（钉 `7911ec678`）阻断一条：TF 一路入账即标已发布，拦截、服务终止与 NO、NR 的请求无痕丢失，之后补消费者也不重放。协调方裁 (a)+(c)。

- (a) 处理方对信封里任何不是「TF 另起旅程」的请求项（TF 扣住、NO、NR 各项）整封返回 `ErrPostIntakeRequestUnhandledOwner`，装配把它列进未决哨兵，信封落 `dispatch.consumer_undecided`，留在可见失败状态；不用自造的新失败码（码集合由派发器封闭持有）。`继续` 无请求项，照常入账。
- (c) 见下方越权风险点 7；UC-PS-006 交接行同写：`POST_INTAKE_SERVICE_DISPOSITION` 授予在 NO、NR 与 TF 扣住的消费者到位前不发给真实租户。
- 补用例：直接调两次处理方 Handle 的幂等用例（一条旅程、一份异常链意图）、下一版原旅程取前一条；原「拦截后无失败码」断言改为四个有未承接项的去向各落一封可见失败、无旅程，继续入账。

**越权风险点 7**：顺序择「整封答未决、不开旅程」而非「开旅程再留痕」。旅程与未决同在一个事务，开了旅程再失败会回滚重投，没有"只开一半"的状态；先提交旅程再另行留痕需第二个落点，且会让 NO、NR 补上后重投再走一遍旅程门。代价：改送、退运在 NO、NR 消费者到位前也开不了 TF 旅程，演示动线上退运只在直接调处理方的用例里可见（`S`），不再经派发进程走通；NO、NR 消费者到位后，被卡住的信封要靠失败信封的重投恢复（运维动作），待协调方确认现有重投口径。

**已知（非阻断）**：集运单元内在运的包裹，原旅程判定 `covers` 只按包裹成员匹配，旅程册以集运单元为成员时可能漏判原旅程，退回派生项 `JRN/<包裹>`。

## 评审记录（2026-10-28，隔离子代理）

- 第一轮（钉 `7911ec678`）：上线口径阻断——登了 TF 一路后信封一经 TF 入账即标已发布，拦截、服务终止对 TF 的请求与 NO、NR 各项原样入账、无痕，补消费者后也不重放。协调方裁 (a) 信封含任何非「TF 另起旅程」项即整封答未决、不开旅程；(c) NO、NR、TF 扣住消费者到位前不给真实租户发 `POST_INTAKE_SERVICE_DISPOSITION` 授予。
- 第二轮（钉 `80975202f`，重跑探针）：阻断清零。四去向各落 `dispatch.consumer_undecided`、不开旅程，`继续` 照常入账；双调 Handle 幂等用例有效；未决只堵该包裹分区。发现：未决烧尽重投预算后落 `ABANDONED`，框架 outbox 没有重新入队，NO/NR 到位后不会自愈（ADR-0094「仓内零重驱机制」的反模式形状）；同包裹的后续信封（如「继续」撤回拦截）排在失败信封后面，延迟约 MAX_ATTEMPTS × RETRY_AFTER。(c) 今天只是文档约束，代码不拦。

## 进 main 记录（2026-10-28，推送方 tops-20 / idp-parcel-91）

- 分支 `claude-lw-pid` tip `80975202f` 作出处；隔离树 cherry-pick 到 main `2d0193336`，无冲突；生成物重生成、两项 `-check` 退 0；全量 `scripts/test-full.sh` 160 ok / 0 FAIL；种子一次性库退 0；推确切 SHA `ec3f0848f`。

## 收口条件

1. NO（扣住、退运装载）、NR（按去向重新规划、释放未消耗计划）、TF（扣住）各有消费者——今天各所有者都没有对应模型，需所有者新模型票。
2. 落 `ABANDONED` 的处置信封有文档化的重驱程序（或上一条先于授予上线，使真实租户的信封不会落到这一态）。
3. 在此之前 `POST_INTAKE_SERVICE_DISPOSITION` 授予不发给真实租户。

## 用户裁定（2026-10-28）

用户答「实际业务不是所有的都系统打通的，有些可能就在我们的系统人工更新处理」，随后选「照这个做」：

- 系统能做的自动做：TF 的改送、退运照现有消费者自动开旅程。
- 其余请求项（NO 扣住、退运装载；NR 重新规划、释放；TF 扣住）不再让整封答未决，而是进「待人工处理」清单，运营人员在系统里逐项登记处理结果（已处理 / 无法处理，附说明，记操作者）；PS 能读回各项结果（`AT-PS-088`）。
- 信封不再因未承接项卡死，「重驱机制」不再是收口前提。
- 新 ADR 部分停用 ADR-0332 里「每项由对象所有者自己承接」的口径。
- 收口条件改为：上述清单与登记口生产可用、演示租户走通一个「退运」与一个「拦截」决定；之后 `POST_INTAKE_SERVICE_DISPOSITION` 授予可发给真实租户。原「收口条件」一节作废。

## 第二阶段完成记录

起点 `05909894d`。新 ADR [ADR-0341](../../../docs/adr/0341-system-executable-disposition-requests-run-in-the-owners-consumers-and-the-rest-become-manual-items-that-parcel-shipment-holds-and-operators-register-results-for.md)（部分停用 ADR-0332 决定四「不立承接结果读回面」一句，写法照 adr/README 顶部；PS CONTEXT 同笔改为现行）。

**改了什么**
- 待人工处理项归 PS 持有：决定形成时（`DecidePostIntakeDispositionHandler`）同笔落项，新迁移 `parcel_shipment/0034`（项、结果两表，主键含决定版本、所有者、动作；结果每项至多一份）。哪些动作系统执行是内置策略 `DispositionOwnerAction.ExecutedBySystem`（今天仅 TF 两个另起旅程动作）。
- TF 消费者只认自己的两个动作，其余项忽略、信封照常发布；去掉 `ErrPostIntakeRequestUnhandledOwner` 与未决哨兵。拦截、服务终止的 TF 扣住进人工项。
- 两个口：`GET /post-intake-disposition-manual-items`（未登记清单；`?parcel=` 读回该包裹全部项与结果）、`POST /post-intake-disposition-manual-item-results`（登记）。新决定种类 `POST_INTAKE_MANUAL_ITEM_HANDLING`，`access_identity/0035` 的 CHECK 取全集；操作者取自认证信封（发行方#sub）。重放答 200 `ALREADY_REGISTERED`，异内容答 409 `RESULT_CONFLICT` 且不覆盖，项查无/他租户答 404 统一不可见，系统执行的动作答 400。
- 演示租户种子：新步骤 `487-access-post-intake-disposition.sh`，补 `POST_INTAKE_SERVICE_DISPOSITION` 与新种类的授予（access 34）、准入对照（35）、权威区间（governance 69、70）。
- 测试：`TestAPostIntakeEnvelopeWithManualRequestsIsPublishedAndOnlyTheSystemExecutedOnesOpenJourneys` 取代原「未承接请求保持可见失败」用例；真库进程用例 `TestAReturnAndAnInterceptDecisionLeaveManualItemsThatOperatorsRegisterAndPSReadsBack`（退运决定经生产编排与派发一拍：退运旅程开出、NR/NO 两项进清单、TF 不进；拦截不开旅程、两项进清单；登记、重放、冲突、PS 读回、清单清空）；cmd/parcel-api 的 HTTP 用例（授权 403、201/200/409/404/400、他租户不可见、按包裹读回）；适配器无事务负向用例；领域单测。

**判据对用例**：演示租户同形租户上「退运」与「拦截」各走通（证据 `S`）。`AT-PS-083`、`AT-PS-088` 由④移入①（088 的"无法处理"即承接拒绝的保留）；`AT-PS-084`（监管退运转 UC-TF-001）不在本票范围，仍在④。UC-PS-006 交接行与 PS CONTEXT 已改为现行。收口条件（用户裁定）：清单与登记口生产可用——已装配于 parcel-api；之后 `POST_INTAKE_SERVICE_DISPOSITION` 授予可发给真实租户（ADR-0341 之前的"不发给真实租户"约束作废）。

**代决与越权风险点**：见 ADR-0341 越权风险点 1 至 4（策略集合、归 PS 持有、授予单立、登记不要求链尾且冲突不覆盖）。另：清单不过滤"已被后来的决定取代"的项，只给 `currentVersion` 由操作者判断（作者取舍）。

**自验（含 PG）**：`go build ./...`、`go vet`、`gofmt -l` 无输出；带 PG DSN `go test -count=1 -p 1` 跑 `./internal/parcelshipment/...`、`./internal/accessidentity/...`、`./internal/transportfulfillment/...`、`./internal/architecture/...`、`./migrations/...`、`./cmd/parcel-dispatch/...`、`./cmd/parcel-api` 通过；种子灌一次性库退 0（三条新授予/对照/区间登记成功）。`adr-index -check`、`at-coverage-counts -check` 只报生成物过期。

**留给协调方**：重生成 ADR 索引、AT 盘点计数、机制清点；管理台页（清单与登记）未做；`AT-PS-084` 仍待立票。

## 第二阶段评审修订

隔离评审（钉 `789897626`）实现无阻断；三处阻断是基线落后：psb/22 已在 origin/main 占了 ADR-0340（供应商账单）与 `access_identity/0035`。
- 变基到 origin/main，`decisionKinds` 与其测试列表取并集。ADR 改号 0341（取号前核过 origin/main 与各 claude-* 分支），连带 ADR-0332 的 Status 索引注记与 Links 前向指针、PS CONTEXT、UC-PS-006、代码/迁移/种子注释与本票面引用；`access_identity` 迁移改 `0036`，CHECK 取 `0035` 供应商账单那一份全集加 `POST_INTAKE_MANUAL_ITEM_HANDLING`，头注不再以编号作锚。`parcel_shipment/0034` 未撞，不改号。
- 护栏：`TestTheActionsMarkedSystemExecutedAreExactlyTheOnesTheTransportAdapterExecutes` 钉住 `ExecutedBySystem` 为真的集合恰好等于 TF 适配器执行的集合（适配器导出 `HandledPostIntakeActions`），并核全集每个动作要么系统执行、要么在某去向的人工项里。
- 清单与读回面每项加派生标志 `supersededByLaterDecision`（决定版本小于链尾版本），用例断言之；ADR-0341 越权风险点 4 补一句。
- 迁移 `parcel_shipment/0034` 头注关于 CHECK 的说法改准；AT-PS-088 盘点行写明「非 TF 拒绝，而是人工项登记无法处理」。
