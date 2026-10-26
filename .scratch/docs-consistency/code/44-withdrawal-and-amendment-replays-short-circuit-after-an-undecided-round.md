# 44 PS 撤回与资料修订：保全之后没落成的那一次，重放短路成「已有结果」，这次请求从此不会形成

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code44。此前 needs-triage——建议 R2（客户面撤回口、资料修订口接上首方 Intake 之前必修）：答错且静默。两只编排都先保全来源、再判断与保存；保全落下之后、撤回或版本没落成（保存失败、并发抢先，或业务未决），客户照 [ADR-0256](../../../docs/adr/0256-technical-non-formation-is-a-reason-of-undecided-not-an-outcome-of-its-own.md) 重放同一请求，编排认出来源已保全就走已有结果那条路：撤回对仍为`已提交`的委托答 `DECISION_ALREADY_FORMED` 却不带任何决定；资料修订答 `ALREADY_HANDLED` 却不带版本。这次请求从此不会形成。两口今天在生产装配下仍是 `UnconfiguredIntake`，接口接上即生效。2026-10-21 文档自洽 06 评审轮立（用户「你作为业务和系统专家自决」授权），分支 claude-dc06。分级由协调方定稿。
Blocked by: 无（撤回在真库上读不回的那一半归 [code/36](./36-withdrawn-shipment-requests-cannot-be-read-back-from-the-store.md)，宜同批）
出处：文档自洽 06 隔离评审阻断 B2；ADR-0256 决定一第 2 格「重放须接着办」；UC-PS-002、UC-PS-005「给开发的交接」。提交编排的同类问题归 [code/39](./39-a-preserved-submission-whose-build-failed-is-never-built-on-replay.md)，不在本票。

## 现象

来源保全一经落下，重放就被当成同一请求已经办过，不再判断、不再保存。可上一次在保全之后停下了，什么都没形成。

## 证据

- 撤回：
  - `cmd/parcel-api/assemble_withdrawal.go` 的 `transactionalWithdrawal` 头注：「编排交回业务答案（含未决与已有决定）时事务提交——被拦下的撤回也已保全来源」。所以撤回停在业务未决与读不回那几格（`SHIPMENT_REQUEST_UNAVAILABLE`、`WITHDRAWAL_AUTHORITY_UNAVAILABLE`、`WITHDRAWAL_AUTHORITY_RULES_NOT_CONFIGURED`、`DECISION_IDENTITY_UNAVAILABLE`）与 `STALE_SHIPMENT_REQUEST_REVISION`（保存答版本冲突，事务照常提交）时，保全照样提交。`DECISION_NOT_RECORDED` 走不到这里：保存的 SQL 出错后事务已中止，`Commit` 失败，整笔回滚、答 5xx，保全一并回滚，重放从头办。
  - `internal/parcelshipment/application/withdraw_shipment_request.go` 的 `resolvePreserved` 对同内容重放只追加观察、读委托、走 `existing`；`existing` 不论委托此刻是否仍为`已提交`都答 `WithdrawalDecisionAlreadyFormed`。委托仍为`已提交`时这一答复不带任何决定，违反 ADR-0256 决定三（不得答成已形成结果）。
- 资料修订：
  - 来源保全与版本落库是两层边界（`AmendmentHandler` 头注：「来源保全一层、版本落库一层」）。`Save` 失败答 `AMENDED_REQUEST_NOT_SAVED` 时保全已提交。
  - `internal/parcelshipment/application/amend_customer_source_data.go` 的 `resolvePreserved` 走 `existing`：在委托的资料版本里找不到这次请求身份的版本，就答 `AmendmentAlreadyHandled`、不带版本。
  - 同文件保存失败那一支的注释「重放这次修订时来源保全会认出同一个身份，走 existing」把这条短路当成了续办——这正是误判。来源保全自成一笔事务（`preservationBoundary`），所以授权、阶段、矩阵等业务未决与读不回之后重放，同样走这条短路；允许声明未声明答等复核之后的重放同样答 `ALREADY_HANDLED`、不带版本。
  - 停在 `SOURCE_DATA_VERSION_NOT_HANDED_OFF` 的那一格不在其中：版本已落库，`existing` 读回版本并重发交接意图，是接着办了的。
- 对照：受控补充编排 `FormNewSubmissionVersionHandler` 重放时接着办。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：两只编排的重放路径分出「保全过、已形成」与「保全过、没形成」两格，后一格按首次请求接着办；补用例（每一种未决之后重放都能形成，且只形成一次）；改那段注释。

## 最小的诚实修法

1. 撤回：重放时按委托状态判——已撤回、已接受或已拒绝，答已有决定并带上那份决定；仍为`已提交`，就按首次请求继续走授权与决定提交。不按请求身份找撤回：`domain.Withdrawal` 不带请求身份。
2. 资料修订：重放时在委托的资料版本里找这次请求身份的版本，找到照旧 `ALREADY_HANDLED` 带版本；找不到就按首次请求继续走授权、阶段、矩阵与保存。
3. 改 `amend_customer_source_data.go` 保存失败那一支的注释：重放要靠第 2 条接着办，不是靠来源保全认出身份。
4. 用例：撤回在授权未决、`STALE_SHIPMENT_REQUEST_REVISION` 之后重放形成撤回（`DECISION_NOT_RECORDED` 整笔回滚，不在本票）；资料修订在 `AMENDED_REQUEST_NOT_SAVED`、授权未决之后重放形成版本；各自重复重放只形成一次。

## 要裁的

无（ADR-0256 决定一第 2 格已定「重放须接着办」）。

## 完成记录

分支 `claude-code44`（基于 main `f1932c72d`，已含 code/36）。分级：协调方分诊定 R2，撤回口、资料修订口接首方 Intake 之前必修。实现照「最小的诚实修法」1–4，无迁移，无新 ADR。

### 实现

- 撤回 `WithdrawShipmentRequestHandler`：授权与决定提交抽成 `decide`，首次到达与重放共用。`resolvePreserved` 同内容重放追加观察后读委托，按委托状态判——已撤回、已接受、已拒绝走 `existing`，答 `DECISION_ALREADY_FORMED` 并带那份决定（已撤回带真库读得回的撤回决定与原补偿关联，不再问授权、不耗决定标识）；仍为`已提交`就进 `decide`，按首次请求走授权、决定标识、转移与保存。不按请求身份找撤回。取锁次序未动（code/99 的事）。
- 资料修订 `AmendCustomerSourceDataHandler`：受理两道（意图未声明、内容与范围不配）抽成 `admit`，授权起的判断与保存抽成 `decide`。`resolvePreserved` 同内容重放追加观察后读委托，`existing` 在委托的资料版本里按这次请求身份找版本：找到照旧 `ALREADY_HANDLED` 带版本并重发同一份交接意图；找不到（`existing` 改交回是否找到）就过 `admit`、进 `decide`，按首次请求走授权、阶段、矩阵与保存。保存失败那一支的注释改正：重放靠版本查找接着办，不靠来源保全认出身份。
- 同身份不同内容仍在 `resolvePreserved` 最先答冲突，不进接着办。

### 测试（先红后绿）

- 红：新用例在原代码上红——应用层替身用例（撤回 7 格、资料修订 11 格，逐格「停下、条件恢复、重放形成、再重放读回」），真库用例（`cmd/parcel-api`：撤回在授权未决与真库版本冲突之后重放形成撤回；资料修订在授权未决、允许声明未声明答等复核、存储拒写——库里触发器拒绝 UPDATE，`AMENDED_REQUEST_NOT_SAVED`——与并发抢先——另一连接抢先写版本号，`STALE_SHIPMENT_REQUEST_REVISION`——之后重放形成版本，意图恰好一封），并发重放各四路只形成一次。已撤回 / 已接受 / 已拒绝态重放答已有决定且带决定、同身份不同内容仍答冲突、确定拒绝重放得同一答案、意图未声明重放仍是调用方的错，这几格原代码上就绿，作回归守卫。
- 原装配用例 `TestTheWiredWithdrawalAnswersHonestlyAgainstARealDatabase` 把「重放不再问授权」当作来源保全已提交的证据，那正是本票要改的行为；改为重放仍停在授权未决，用来源保全行数与观察行数证首笔事务已提交。
- 变异（应用层用例，逐个改后跑全包、再从备份拷回并 `cmp`）全被杀：M1 撤回重放一律 `existing`（仍为`已提交`时答已有决定不带决定）；M3 资料修订找不到版本仍答 `ALREADY_HANDLED`；M4 撤回已决态重放又进 `decide`（又问授权）；M5 重放形成的版本记 `incoming` 而非保全下来的指纹；M6 重放跳过 `admit`；M7 资料修订冲突分类被吞；M8 `existing` 不按请求身份匹配（见到任一版本就当办过）；M9 撤回冲突分类被吞；M10 `existing` 读回版本不重发交接意图。M1、M3 的真库对应就是原代码，上面的红即是。
- 跑过：`scripts/test-full.sh` 范围 `./internal/parcelshipment/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./internal/architecture/...`、`./migrations/...`、`./tests/bentocontract/...` 全绿；新用例 `-count=3`、并发用例 `-count=5` 绿；`go build ./... && go vet ./...`、`gofmt -l` 无输出。`at-coverage-counts -check` 退 1（只是总览过期，新立 AT 已入①），`adr-index -check` 退 1（只差 ADR-0256 注记，生成物不提交）。

### 文档

- UC-PS-005：步骤 1、规则一条、结果语义「已有决定」禁止行、验收新立 `AT-PS-125`，「给开发的交接」本票行改写为已补并写明 `DECISION_NOT_RECORDED` 整笔回滚不在其中。
- UC-PS-002：步骤 2、「已有结果」行（形成的是版本，不是任何结果）、版本规则一条、验收新立 `AT-PS-126`，「给开发的交接」本票行改写为已补。
- PS CONTEXT：「来源请求身份」重放规则句补撤回与资料修订按什么判（不挂票号）。
- ADR-0256：Status 注记补「缺口已补」（正文不变）。不立 ADR-0287：本票是 ADR-0256 决定一第 2 格的原句落地，不改任何已接受 ADR 的口径；预留号未用。
- AT 盘点：`AT-PS-016`、`017`、`031`、`068`、`069` 补用例名，`AT-PS-125`、`126` 入①，文末追记。

### 代决

1. 撤回「办没办过」只看委托此刻的状态，已越过提交边界的重放不再问授权（授权可能已失效，同一个已成立的撤回不该读出两种回执）；仍为`已提交`的重放重新问授权，因为撤回从未形成过，用此刻的授权判就是首次请求的语义。
2. 资料修订找不到版本一律接着办，包括上一轮是确定业务答案（未获授权、不允许、等复核）的：这些答案没落任何记录，没法读回「原答案」；重放重判，条件没变得到同一个答案，条件变了（授权补上、声明补登）才形成版本。因此 `ALREADY_HANDLED` 只留给已形成版本的请求，UC-PS-002 的「已有结果」行相应收窄。
3. 重放形成的版本，请求留痕记保全下来的那一份（首次到达的发生时间与接收时间），不记重放这一次到达的——版本上的请求留痕要与来源保全表里的那一条对得上。
4. 重放接着办时受理两道（意图、内容与范围）照样跑：调用方的错重放仍是调用方的错，不被「到过」吞成不带版本的已有结果。
5. 新立 AT 号取 `AT-PS-125`（UC-PS-005）、`AT-PS-126`（UC-PS-002）：fpa06 分支已占 `AT-PS-120`–`124`，进 main 时若再有占用由协调方改号。
6. 真库用例的撤回装配用生产同形件加授权与释放两只替身，不改 `assemble_withdrawal.go`（撤回授权的请求映射是实例半边，生产装配走不到授权放行之后的格；另一会话在接首方口，避免碰同一文件）。

### 越权风险点

1. **确定的业务拒绝重放会被重判**（PS owner）。客户端若假定「同一请求重发永远得到同一个答案」，授权或允许声明在两次之间变了，未获授权或等复核的请求重放会形成版本。这是 ADR-0256「按这一次实际走到的位置接着办」的直接后果，不是本票额外加的；要冻结拒绝答案须另有一张记录拒绝答案的册，不在本票。
2. **并发重放的输者答未决不是已有结果**（PS owner）。两路重放同时到达，委托行版本闸门放一路，另一路答 `STALE_SHIPMENT_REQUEST_REVISION` 带续办引用，客户需再重放一次读到已有结果；输者白耗一个决定标识或版本标识（本上下文签发的稀缺身份，不回收），与首次请求输给并发写入时同一格。
3. **受控补充编排重放形成版本时记的是重放这次到达的指纹**（PS owner）。本票资料修订记保全下来的指纹，两只编排不一致；补充编排不在本票，未动。
4. `DECISION_NOT_RECORDED` 整笔回滚（保全一并回滚、重放从头办）按票面不在本票；撤回与拒绝、处置的取锁次序归 code/99，本票未动。
5. 首方五口接线（撤回口、资料修订口接 Intake）在另一会话；两口接上时 `outcome` 与未决原因的词表不需要因本票增加新格，但客户端须把撤回的未决当作可重放，把 `DECISION_ALREADY_FORMED` 当作必带一份决定。
