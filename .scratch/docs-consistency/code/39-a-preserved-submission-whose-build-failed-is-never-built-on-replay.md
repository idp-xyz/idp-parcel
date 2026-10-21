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

## 完成记录（2026-10-21，分支 claude-code39，基于 main `544e41de1`）

### 做了什么

- `d36769430` 判得的生产归属答案落库：新端口 `ports.SubmissionOwnershipAnswers`（追加、取最近一次、按委托号取建单所据那一次）与 PostgreSQL 适配器 `SubmissionOwnershipAnswers`，迁移 `migrations/parcel_shipment/0028_submission_ownership_answer.sql`（只追加；放行与「有将要建的委托号」两格由库约束守成同真同假；外键挂已保全来源）。决定整份进 jsonb，读回经 `NewProductionOwnershipDecision`、`AssessSafeHandoff`、`WithSafeHandoff` 重建；交接评估经新加的 `SafeHandoffAssessment.FormationSpec` 原样存取（部分确认那种被读口收起的确认引用也不丢）。
- `7c67c7186` 提交编排重发接着办：`Handle` 保全之后那一段抽成 `proceed`，首次与续办共用；`resolvePreserved` 按上一次走到的位置作答——委托已在答`已有结果`、指名并带建单所据的原归属结果；最近一次答案是`准入暂停`或`非本产品生产归属`（由 `settledOnReplay` 一处判定）答`已有结果`带原归属结果与门禁原因、不重判；其余（什么都没留下、放行了却没建成、`生产归属未决`）走 `proceed` 接着判归属与建单。挡下的答案记下了才答，放行的答案在建单之前、建单那一笔之外记下（装配 `ownershipAnswerBoundary` 逐笔开事务）。并发撞「已存在」时也带胜者建单所据的答案。构造器加一参，调用点随改（`cmd/parcel-api` 装配与测试、`cmd/parcel-dispatch` 两个测试、`tests/bentocontract` 夹具、HTTP 适配器测试、应用测试）。
- `e87caf4f4` UC-PS-001：提交答复表「已有结果」限到原处理已形成结果，「非本产品生产归属」「生产归属未决」「准入暂停」三行写明重发怎么答；表下文「重发接着办」一段替掉现状缺口句；幂等一条改为重放分两路；`AT-PS-003`、`AT-PS-104` 限定，新立 `AT-PS-111`–`AT-PS-113`；「给开发的交接」code/39 两行改为已补。PS CONTEXT 重放一条同改并指 ADR-0256 决定一。ADR-0256 正文未动。
- `76953566d` AT 盘点：①加 `AT-PS-111`–`AT-PS-113` 三行，文末追记一条；总览计数未改。

### 代决

用户授权「你作为业务和系统专家自决……解决所有的问题」；协调方先行裁定本票「要裁的」各格，本票照办并逐条落在 UC-PS-001 提交答复表：

1. 什么都没形成（保全已落、无归属判断、无委托）→ 重发续办：照首次提交继续判归属与建单，并发下由建单唯一键（`shipment_request` 主键即来源身份）保证只建一份。
2. `OWNERSHIP_UNRESOLVED` → 重发重判归属；判得出照常建单，仍判不出再答原格（带这一次的决定）。
3. `ADMISSION_PAUSED` → 重发答`已有结果`带原归属结果，不重判（首发试点叠加条件「不自动恢复」）。
4. 委托已建 → 照旧`已有结果`指名委托，并补上原归属结果。

本票另行代决（协调方裁定没写到的细节）：

5. `非本产品生产归属`（交接已确认）与准入暂停同格：重发答`已有结果`带原归属结果，不重判、不再投递——UC-PS-001「不得同时投递多个生产系统」。交接未确认的`其他权威`答的是`生产归属未决`，照第 2 条重判、重投同一次尝试（尝试身份由决定派生，对方据它认领重放）。
6. 归属答案只追加、每一次判断留一行（UC-PS-001「归属未决时保留缺口与续办记录」）；放行的答案带将要建的委托号、在建单之前记下，委托已在时按委托号取最早那一次，不取最近一次。
7. 关联互证（`PRIOR_REQUEST_NOT_FOUND`、`LINK_INELIGIBLE`）与输入校验（`INPUT_NOT_ACCEPTED`）的答案不留存，重发按同一请求再判一次——今天无处可读「原接入处理结果」，答一个空的已有结果正是本票要堵的样子。
8. 续办建成委托时答`已提交`（不是`已有结果`）：客户上一次拿到的是 5xx 或未决，这一次才真建成。
9. 续办据保全在册的那份来源事实立当前提交版本（首次到达的 `occurredAt`/`receivedAt`），这一次的观察只追加在旁边。
10. 归属答案记不下即没有形成答案，上抛 5xx（ADR-0256 决定一：提交编排保全之后出错答 5xx）——挡下的答案不记就答，重发会把一次暂停重判成放行。
11. 建单答「已存在」却按来源身份读不回委托（撞的是委托号唯一键）由答不指名委托的`已有结果`改为上抛：那份提交并没建成，客户原样重发、接单入口另签号即可接着办。
12. PS CONTEXT 重放一条改写为两路并指 ADR-0256 决定一：原句「与相同规范化内容摘要返回已有结果」与已接受的 ADR-0256「不得因来源已保全就短路成已有结果」相抵，按 AGENTS「冲突时 ADR 优先于用例」把 CONTEXT 对齐到 ADR，不另立 ADR。
13. `tests/bentocontract` PBC-07 回滚方向用例 `TestCommitUncertainOverARollbackAnswersTruthfullyByStableCorrelation` 原本钉的是旧口径（重发答不附委托的已有结果、不再开提交事务），改为接着办：重发建成那份委托、提交事务恰好再开一次、一行委托一份意图——仍是框架合同「先查原结果，确认不存在才重执行」。`cmd/parcel-api` 两个真库用例（目录未配置的未决、跨客户指名的查无原委托）的重放断言同改为照原格再答、追加一次观察。

### 越权风险点（待 PS owner 复核）

1. **暂停之后同一请求键永远不建单**：暂停解除之后，客户原样重发拿到的仍是那份暂停的已有结果；要入单得换一个来源请求键重新提交。若要「明确恢复决定之后原请求可续办」，需要恢复决定能指名受影响的已保全提交——那是新机制。
2. **未决之后重发可能隔很久才建单**：委托的提交时刻是续办那一刻，当前提交版本记的系统接收时间却是首次到达，两者可以相隔很远；「提交接收」时点形态按后者取值。
3. **关联互证重判可能换答案**：首次答`关联不适用`、其后原委托到了终态，重发会建成关联新委托；首次答`原委托不可见`同理。按客户意图这是对的，但与「已有结果返回原接入处理结果」的字面不同。
4. **答案留存之前建成的委托**重发只指名委托、不带原归属结果——库里没有那一次的记录，不拿别的一次顶替。
5. **答案行随失败的重试增长**：放行之后建单屡次失败，每次重发都记一行放行答案；并发续办的落败方也留一行指着从未建成的委托号。读口按委托号取，答复不受影响，只是历史里多几行。
6. **并发下登记册恰在两路之间变化**：一路放行建单、另一路判成暂停，后者照实答`准入暂停`而委托已在；此后再重发按委托在答`已有结果`。答案各自如实，但同一请求两路答复不同。
7. **「已有结果」一行「已经建单时再附原委托及接受/拒绝/未决结果」**：今天重发指名委托与逐件包裹，不带接受判断结果；不在本票缺口内，本票未动，如需补另立票。
8. `SafeHandoffAssessment.FormationSpec` 交回含部分确认引用的原始输入，只供持久化读回；业务代码若拿它判「交过去没有」会把部分确认当确认。

### AT

- 新立并钉住：`AT-PS-111`（真库 `TestAReplayAfterTheBuildTransactionRolledBackBuildsTheRequestOnce`、`TestAReplayAfterTheOwnershipDependencyFailedDecidesAgain`、`TestTwoConcurrentReplaysAfterARolledBackBuildBuildOnce`，应用层两条）、`AT-PS-112`（真库一条、应用层两条）、`AT-PS-113`（真库一条、应用层两条）。
- 改写：`AT-PS-003`（限到原处理已经建单、带建单所据的原归属结果；真库 `TestAReplayOfABuiltRequestNamesItWithTheOwnershipItWasBuiltOn` 点它，盘点里该行未改）、`AT-PS-104`（重放一支限到这次也没有建单）。
- 盘点总览计数、`docs/product/MECHANISM-INVENTORY.md` 与 ADR 索引未重生成，由推送方处理。

### 验证（代码同 `7c67c7186`）

- `go build ./...`、`go vet ./...`、`gofmt -l`（全部已跟踪 Go 文件）干净。
- `scripts/test-full.sh` 跑受影响的 41 个包（`internal/parcelshipment/...`、`internal/architecture/...`、`cmd/parcel-api`、`tests/bentocontract`，以及代码或测试直接导入 PS `domain`、`ports`、`application`、`adapters/postgres` 的全部包，含 `cmd/parcel-dispatch`）：39 ok、2 包无测试文件、0 FAIL。
- 新真库用例另以 `-count=3 -v` 跑一遍，均 PASS（未跳过）。
- 未跑全量；未灌演示种子（种子不走提交编排的重放路）。
- 迁移号取 0028：基于 main `544e41de1` 时 `migrations/parcel_shipment/` 最大号 0027；进 main 前若别的分支先占 0028 需改号。

### 未决

- 撤回、资料修订两只编排的同类短路仍归 [code/44](./44-withdrawal-and-amendment-replays-short-circuit-after-an-undecided-round.md)。
- ADR-0256 越权风险点 3 写「code/39 落地之前重发得到一个不指名委托的已有结果」，本票落地后那句成为历史描述；按「不改写已接受 ADR」未动，要不要挂注记由推送方或 PS owner 定。
