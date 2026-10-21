# 39 PS 来源保全之后建单失败的提交，重发只答「已有结果」，这份提交再也不建单

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code39（见文末「完成记录」）。此前 needs-triage——建议 R1（上线前必修）：静默丢单。来源保全与建单是两个事务（首个消费者切片简报「事务边界」），保全提交之后，归属判断依赖失败或建单事务失败都答 5xx；客户照 ADR-0022 原样重发，提交编排按已保全的来源只答 `EXISTING_RESULT`、不指名委托，不再走归属与建单。客户拿到一个「已有结果」，系统里却没有这份委托。第一次数据库抖动就会撞上。2026-10-21 文档自洽 06 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc06。分级由协调方定稿。
Blocked by: 无
出处：文档自洽 06 修订 UC-PS-001 结果语义契约（「没有形成答案」那一段，评审轮补「已有结果不带原归属结果」）与 [ADR-0256](../../../docs/adr/0256-technical-non-formation-is-a-reason-of-undecided-not-an-outcome-of-its-own.md) 时读码所得，不在审计报告条目里；UC-PS-001「给开发的交接」。

## 现象

保全一经提交就不随后续步骤回滚（UC-PS-001 步骤 2 要求）。后续一步失败时，那次请求留下了保全，却没有委托。重发同一请求时，提交编排认出已保全的来源，走重放那条路：只追加一次观察，按来源身份找委托，找不到就答一个不指名委托的 `EXISTING_RESULT`。

## 证据

- `cmd/parcel-api/assemble_submission.go`：`preservationBoundary` 给保全单独开事务，头注写「保全一经提交就不随后续步骤回滚」；`submissionBoundary` 是建单的第二个事务。
- `internal/parcelshipment/application/submit_shipment_request.go`：`Handle` 在 `Preserve` 之后，`DecideProductionOwnership` 出错、签发标识出错、`Insert` 出错都直接上抛；`resolvePreserved` 对同内容的重发只答 `OutcomeExistingResult`，「绝不重判归属、也不建第二份委托」，`FindBySourceIdentity` 找不到时不指名委托。
- 文档：[ADR-0256](../../../docs/adr/0256-technical-non-formation-is-a-reason-of-undecided-not-an-outcome-of-its-own.md) 决定一第 2 格「重发必须接着办」；ADR-0022「没有形成答案」由调用方重发。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：重放那条路要分得出「保全过、建成了」「保全过、归属挡下了」「保全过、什么都没形成」；最后一格续办建单（重新判归属、照常建单），不当作已有结果；补真库用例（建单事务失败后重发建成委托，且只建一份）。

## 最小的诚实修法

1. 保全记录上记下这次处理走到了哪一步（至少分「归属已判」与「委托已建」），或由重放路按来源身份同时查归属判断与委托。
2. 重放时：委托已建 → 照旧 `EXISTING_RESULT` 并指名；归属已判且挡下 → 答原归属结果（今天 `resolvePreserved` 不带归属决定，UC-PS-001 提交答复「已有结果」一行要求返回原归属结果，一并补上）；两者都没有 → 按首次提交继续判归属与建单，并发下仍由建单的唯一键保证只建一份。
3. 补用例：`Insert` 失败后重发建成委托；归属依赖失败后重发重判归属；归属挡下之后重发带回原归属结果。

撤回与资料修订编排的同类短路归 [code/44](./44-withdrawal-and-amendment-replays-short-circuit-after-an-undecided-round.md)，本票只管提交。

## 要裁的

- 归属挡下（`OWNERSHIP_UNRESOLVED`、`ADMISSION_PAUSED`）之后的重发今天也只答已有结果、不重判；UC-PS-001 写归属未决「可安全续办」，而首发试点叠加条件写暂停「不自动恢复」。两格要不要随重发重判，本票落地时提出，交 PS owner 定；本票的必修只是「什么都没形成」那一格。

## 完成记录（2026-10-21，分支 claude-code39，rebase 于 main `35d181d1c`；隔离评审后按协调方裁定重写）

### 做了什么

第一轮（rebase 后 SHA）：
- `ab7a33e5c` 判得的生产归属答案落库：端口、PostgreSQL 适配器与迁移 0028，`SafeHandoffAssessment.FormationSpec`。
- `887549359` 提交编排重发接着办：保全之后那一段抽成 `proceed`，按上一次答案作答。
- `b0089408d` UC-PS-001 与 PS CONTEXT；`9dd53f281` AT 盘点；`bd4d03c40` 第一轮完成记录。

评审修订：
- `894bd69a9` 领域补 `ProductionOwnershipDecision.WithReassessedHandoff`：只把同一次尝试（同一尝试身份、同一范围与对方）的重查评估换到一份交接未确认的他方决定上。
- `2bdf452d8` 编排与存储改按 ADR-0266：
  - 表改名 `parcel_shipment.submission_answer`（迁移 `migrations/parcel_shipment/0028_submission_answer.sql`，未进 main，原地改）。三格拒绝（输入未受理、原委托不可见、关联不适用）也留存；端口改名 `ports.SubmissionAnswers`，加 `SubmissionRefusal`。
  - 重放答`已有结果`带原结果：新访问器 `PriorOutcome`，首方答复加 `priorOutcome`。只有生产归属未决重判；他方且交接未确认时重投同一次尝试、不重判。
  - 新端口 `ports.SubmissionContinuationGuard`，装配 `submissionContinuationGuard` 在 `cmd/parcel-api/assemble_submission.go`：开一笔事务取来源身份的咨询锁（适配器 `LockSubmission`），判归属、记答案、建单、信封并进这一笔。已定的重放在锁外只读作答。
  - `tests/bentocontract` 夹具的计数器与「提交不确定」注入口改包在守卫上；`SubmissionIdentityFactory` 端口注释写明收敛依赖接单入口每次现签委托号。
- `f3b4227e9` 注释里一处把转述写成了 ADR-0128 引文，改掉。
- `8578b561a` 新立 ADR-0266，部分停用 ADR-0081 一句；ADR-0081 Status 注记与 Links 加来向，ADR-0256 索引注记加「缺口已补」（SHA 待填）。
- `07f0c7588` UC-PS-001、PS CONTEXT 与首个消费者切片简报随 ADR-0266 改。
- `28de01a93` AT 盘点。

### 代决（按协调方评审裁定重写）

用户授权「你作为业务和系统专家自决……解决所有的问题」。重放格局由协调方两轮裁定，立 ADR-0266 落定（会话代 PS owner 决）：

1. 委托已建 → `已有结果`，指名委托并带建单所据的原归属结果。
2. 答出过的拒绝（输入未受理、原委托不可见、关联不适用）→ `已有结果`带原结果，不再判。三格的答案留存，保住 ADR-0081「重放一份曾被业务拒绝的输入答已有结果」。第一轮「这三种不留存、重发再判」一条作废。
3. 准入暂停、交接已确认的非本产品生产归属 → `已有结果`带原归属结果与门禁原因，不重判、不再交接。
4. 他方而交接未确认 → 按记着的尝试身份重投、重查，不重判。确认了这一次才答`非本产品生产归属`，仍未确认答`生产归属未决`。协调方原话「确认了就答原结果」，本票读作「答原那份他方决定，这一次交接确认」：首次确认那一次答`非本产品生产归属`，此后重发答`已有结果`。
5. 生产归属未决 → 重判，按这一次的结果作答。这一格部分停用 ADR-0081 那一句。
6. 没有形成答案（5xx 那一格）→ 续办；续办建成答`已提交`。「没有形成答案」只指 5xx 那一格。
7. 同一来源身份的续办串行。本票选 `pg_advisory_xact_lock`，不选 `SELECT … FOR UPDATE`：守卫要持锁到建单落下，FOR UPDATE 会挡住别的重放在锁外追加观察（外键共享锁）。锁与写入并进同一笔事务：第一版让写入另取连接，真库并发用例在两连接的测试池上互等至超时，生产上排队的几路一样会把连接池占满。「记答案前复查」在进锁那一刻做：进锁后同一来源身份不会再有别路落笔。
8. 留存的答案在守卫那一笔事务里与委托、信封同生共死：建单失败，答案一起回滚，重发按没有形成答案接着办。
9. `已有结果`带 `priorOutcome`（首方契约新增字段，只增不改）：拒绝那几格没有归属决定可带，少了它客户不知道上一次答了什么。
10. 续办据保全在册的首次到达那份来源事实立版本；答案记不下即没有形成答案，答 5xx。
11. 建单答「已存在」却按来源身份读不回委托（撞委托号唯一键）由答空的已有结果改为上抛。
12. PS CONTEXT 重放一条与首个消费者切片简报同键同摘要一条，改为指向 ADR-0256 决定一、ADR-0266 决定一与 UC-PS-001「重发接着办」一段。

### 越权风险点（待 PS owner 复核；ADR-0266 越权风险点同列）

1. **暂停或拒绝之后同一请求键永远答原结果**：暂停解除、或关联指名的原委托后来到了终态，原样重发仍答那份已有结果，要入单得换来源请求键。
2. **未决之后重发可能隔很久才建单**：提交时刻是续办那一刻，系统接收时间是首次到达。
3. **串行守不住的窗口**：守卫排住的是经提交编排的同一来源身份。
   - 剩下一格：他方投递已发出、本方记下答案之前进程崩溃或那一笔事务回滚，答案不在。重发按没有形成答案重判，登记册若此间翻成本产品，就会一边已投给他方、一边在本地建单。
   - 缓解：尝试身份由决定派生，他方据它认领；对账要按来源身份把两边对上。
   - 另：一路续办在锁里占一条连接直到建单落下，判归属与对外投递都在锁里；同一来源身份的重放排队等它。
4. **迁移 0028 之前保全的来源**：那时答过准入暂停、交接已确认的他方归属或三格拒绝的，库里没有答案行，重发按没有形成答案重判；那时已建成的委托，重发不带原归属结果。无法回填，只影响演示数据（UC-PS-001「给开发的交接」已列）。
5. **答案行随重判、重查增长**：读口取最近一次或按委托号取，答复不受影响。
6. **`priorOutcome` 是首方契约新增字段**：管理台今天不读它。
7. **「已有结果」一行「已经建单时再附原委托及接受/拒绝/未决结果」**：重发不带接受判断结果，不在本票缺口内。
8. `SafeHandoffAssessment.FormationSpec` 交回含部分确认引用的原始输入，只供持久化读回；业务代码若拿它判「交过去没有」会误读。

### AT

- `AT-PS-111`：真库三条，含两路并发只建一份、后到者不判；应用层两条。
- `AT-PS-112`：真库一条、应用层三条。
- `AT-PS-113`：真库一条、HTTP 一条、应用层三条。
- `AT-PS-114`（新）：真库一条、应用层一条。
- `AT-PS-003` 盘点行补点真库 `TestAReplayOfABuiltRequestNamesItWithTheOwnershipItWasBuiltOn`；`AT-PS-104` 限定。
- 盘点总览计数、`docs/product/MECHANISM-INVENTORY.md`、`docs/adr/README.md` 索引都未重生成。`go run ./tools/adr-index` 在 scratchpad 副本上生成后 `-check` 通过。

### 验证

见下文「评审记录」末段。

## 评审记录

### 第 1 轮（隔离评审，协调方转达）

代码正确性与并发收敛核过无问题。阻断 1 条，建议 10 条，逐条去向：

| 条目 | 去向 |
|---|---|
| B1 ADR-0081 那一句被改无停用指针 | 已修。三格拒绝留存、重放答原结果，`TestARefusedSubmissionStillPreservesItsSource` 改回 ADR-0081 原口径（另断言 `priorOutcome`）。新立 ADR-0266 部分停用 ADR-0081 那一句，只限生产归属未决；UC「重发接着办」与 CONTEXT 不再把已答出的答案归进没有形成答案。`TestTheWiredSubmissionAnswersHonestlyAgainstARealDatabase` 没改回：它停在生产归属未决，正是 ADR-0266 部分停用的那一格，重放重判仍答 `OWNERSHIP_UNRESOLVED`，头注改指 ADR-0266 |
| N1 本方建单与他方交接并存 | 已修。续办守卫（咨询锁，与写入同一笔事务，进锁复查）；他方交接未确认重投同一次尝试、不重判。真库补 `TestTwoConcurrentReplaysAfterARolledBackBuildBuildOnceAndJudgeOnce`（后到者若判会判成他方，断言只判一次、零投递）与 `TestAReplayAfterAnUnconfirmedHandoffRedeliversTheSameAttemptInsteadOfDeciding`；适配器补 `TestSubmissionLockSerializesOneSourceIdentity`；越权风险点 3 写剩余窗口 |
| N2 迁移前数据 | 已修。越权风险点 4；UC「给开发的交接」加一行（不承接，无法回填） |
| N3 未决一行与 `AT-PS-112`、`AT-PS-111` 前提 | 已修 |
| N4 逐次留存不算第二份归属记录 | 已修。「已有结果」一行禁止行为补一句，ADR-0266 决定三 |
| N5 简报「输入与幂等」 | 已修。同键同摘要一条只引 UC-PS-001「重发接着办」那段 |
| N6 `AT-PS-003` 盘点行 | 已修 |
| N7 测试头注「三个」 | 已修 |
| N8 `SubmissionIdentityFactory` 端口注释 | 已修 |
| N9 往返补两种交接评估 | 已修。用例随表改名为 `TestSubmissionAnswersRoundTripWholly`，补「通往他方的通道未配置」「他方超时未答」两格，并补三格拒绝 |
| N10 ADR-0256 注记 | 已修。索引注记追加「缺口已补」，进 main 提交 SHA 留「待填」，正文不动 |

未采纳：无。`TestTheWiredSubmission…` 不改回的理由见 B1 一行。

验证（代码同 `2bdf452d8`，此后只改注释与文档）：
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/parcelshipment/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./tests/bentocontract/... ./internal/architecture/... ./migrations/...`：在 `2bdf452d8` 上 24 ok、2 无测试文件、0 FAIL。收口前（文档提交之后）重跑一遍：23 ok，`cmd/parcel-dispatch` 红在 `TestAClosedAgreementVersionIsTracedAndRecoveredByRevision/同起点退役`（TF 实际履约补形成，「换版到 v2 = APPLIES_BEFORE_SUBMISSION」）。这个用例不经提交编排；在 main `35d181d1c` 的源码快照（`git archive`）上单跑四次三次红，是既有的不稳定用例，不是本票引入的。单独重跑 `cmd/parcel-dispatch` 整包为 ok。
- 新用例与改动用例 `-count=3 -v` 另跑（`cmd/parcel-api`、PS `adapters/postgres`、`application`、`domain`、`adapters/http`、`tests/bentocontract`）：全部 PASS，未跳过。
- `go build ./...`、`go vet ./...`、`gofmt -l` 干净。
- 守卫第一版（写入另取连接）在两连接的测试池上互等至 10 分钟超时，已改为写入并进守卫那一笔事务；改后同一套全绿。
