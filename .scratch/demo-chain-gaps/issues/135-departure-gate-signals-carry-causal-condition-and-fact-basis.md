# 135 出发门禁阻断信号没有因果条件与事实依据，发作期永不结束

Category: bug
Status: in-progress——2026-10-26 评审无阻断、修订轮完，待进 main，分支 `claude-dcg135` tip 为写评审记录与修订轮的那一笔（已 rebase 到 origin/main，代码与文档最后一笔 `9661b63fa`；见文末「评审记录」「修订轮」）。此前 in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-dcg135` tip `340c1581a`（rebase 前）。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领，与 136 同批，分支 `claude-dcg135`（worktree `idp-parcel-dcg135`）。此前 ready-for-agent——2026-10-20 代决复核立（上线后尽快），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M18；ADR-0205 越权风险点 22、23

## 现象与修法

现象、证据与修法的完整写法以复核报告 M18 一条为准（单一权威），此处只记要点：

以那一条移动事实为因果条件，每次违规各开一期、重新分诊；命令加事实依据（`movement-fact/<对象>/<事实>` 引用、事实版本、逐对象关务判断标识）。冲突信号缺恢复边界的存量问题在同票另列一格。

## 文档自洽审计复核（2026-10-21）

审计条目：[audit/ve.md](../../docs-consistency/audit/ve.md) VE-09（归 C，审计已点名本票，不另立）：异常信号不带事实依据与因果条件，发作期永不结束（出发门禁阻断信号、冲突信号同病）。按 main `67438a0f` 复核，现象仍在：`application.RaiseSignalCommand` 没有事实依据与因果条件两格，`domain.SignalEpisode` 按（包裹，类型）定位，生产代码里发作期的 `End` 没有调用方。票 146、94 改了门禁读数回给设备的形状，没有碰信号。

分级建议（文档自洽 02）：R2，与本票既定「上线后尽快」一致。本票落地时一并修正 `RaiseSignalCommand` 头注里与字段不符的那一句（它引 CONTEXT「每个信号必须保存……事实依据」却说「编排不替命中补任何一样」）；UC-VE-004 的暂挂说明随之删去（由修订票 docs-consistency/04 或本票顺手）。

## 完成记录（2026-10-26，Claude Code 作者子代理，协调方 `tops-20`，分支 `claude-dcg135`）

起点 main `449a25508`。与票 136 同批同分支。

### 提交

| SHA | 内容 |
|---|---|
SHA 是修订轮 rebase 到 origin/main 之后的；rebase 前依次是 `1569dcafe`、`24d870d4c`、`53cfbd1cc`、`7fa90b1b6`、`d1f8308da`、`340c1581a`。

| SHA | 内容 |
|---|---|
| `1fa05d011` | 代码：领域、编排、端口、PG 适配器、TF→VE 适配器、发作期册读口与线格式、管理台详情、VE 迁移 0029，连同各层用例 |
| `3524ec970` | （票 136 的代码，见那一票） |
| `8f2c35b57` | 文档：UC-VE-004 新立 `AT-VE-201`–`203`、删暂挂说明；ADR-0205 Status 注记与 Links；AT 盘点；docs-consistency/code/30 追记 |
| `a442c093d` | （票 136 的文档，见那一票） |
| `3ae88a962` | VE 迁移一度改号 0029 → 0030（当时协调方分派，0029 让给 tops-26 的票 code/09） |
| `ecb0151e4` | 两票完成记录 |
| `9661b63fa` | 修订轮：迁移改号回 0029 与各处引用、过时头注、ADR-0302 收窄、AT-VE-203 执行迁移原文的真库用例、code/30 两格同步（见「修订轮」） |
| 本笔 | 两票评审记录与修订轮 |

### 改了什么

- **领域**（`internal/visibilityexception/domain`）：新文件 `signal_cause.go`——`CausalConditionReference`、两种产品自带写法 `SourceFactCausalCondition`（`source-fact/<源上下文>/<事实引用>`）与 `SupersessionForkCausalCondition`（`supersession-fork/<源上下文>/<事实引用>`）、存量那一格 `CausalConditionRecordedBeforeCauses`、事实依据 `SignalFactBasis`（源上下文、事实引用、版本、可缺的判断标识）、一次命中 `SignalHit`（`Complete` 是受理与开期共用的判据）。`SignalEpisode` 多因果条件与依据两维，`OpenEpisode`、`ReopenAsLinked` 只凭一次完整命中（重开须同一原因，规则版本与依据取再发生那一次的），重建门只放存量那一格没有依据。`ExceptionSignal.Basis` 交回冲突保留各方。
- **编排**：`RaiseSignalCommand` 加 `Cause`、`Basis`，缺一未受理；头注里「编排不替命中补任何一样」那一句改成与字段相符（除发作期与变化历史外各样都随命令来）。发作期按（租户、对象、类型、因果条件）定位（`FindLatestForCause`）。出发门禁阻断信号命令加 `Fact`、`Version`、`Judgement`，因果条件取那一条移动事实；冲突信号的因果条件取分叉的事实流、依据取保留各方。披露决定仍按对象+类型取最近一期（`FindLatest`，头注说明）。
- **端口**：`SignalEpisodeStore` 加 `FindLatestForCause`，两种找法各答什么写在端口头注；分诊意图按发作期认领。发作期册行加因果条件与依据（`SignalEpisodeFactBasisRow`）。
- **PG 适配器与迁移**：VE 迁移 `0029`（一度让给 code/09 改号 0030，code/09 交工没用 0029，修订轮改回，见提交表）：`signal_episode` 加 `causal_condition`、`fact_basis`（jsonb 数组），存量回填 `RECORDED_BEFORE_CAUSAL_CONDITION` 与 `[]`，CHECK 只放存量那一格没有依据，加按因果条件定位的索引。`SignalEpisodes` 写读两维；分诊意图信封 ID 加发作期维（`triage_handoff.go`）；发作期册（`case_review.go`）照列两维。
- **TF→VE 适配器**：`signalBlocked` 每件带上这件包裹上的 `movement-fact/<对象>/<事实>` 引用（与投影同一处 `movementFactSourceReference` 铸）、事实版本与读数里那一格的关务判断标识；阻断那一格没有判断标识归不变量已破。
- **线格式与管理台**：`/exception-triage-records?registry=signal-episode` 每行加 `causalCondition` 与 `factBasis`（恒为数组）；管理台「异常分诊」发作期详情加「因果条件」「事实依据」两格，存量行如实显示「登记于因果条件之前」。
- **文档**：UC-VE-004 新立 `AT-VE-201`–`203`，段末暂挂说明删去，冲突信号恢复边界一格改记「给开发的交接」；ADR-0205 Status 注记加「越权风险点 22、23 已答」（由本票回答、落地，无条款停用），Links 加本票；AT 盘点①三行与追记（总览计数不动）；docs-consistency/code/30 追记冲突信号恢复边界随那一票落。**VE CONTEXT 未改**（它的规则本来就对，审计 VE-09 同此）。
- 没碰：TF、code/01 名单内文件、GLOSSARY、CONTEXT-MAP、ADR 索引、AT 计数、机制清点。

### 与另一会话在途票 code/09 的改动面（协调方要求逐文件列出）

code/09（tops-26，VE 演示冲突信号类型，可能动信号规则登记与种子）可能与本票碰到同一块。本票**没碰**信号规则册的登记、读口、迁移 0025 / 0027 与 `scripts/demo-seeds`。本票改过、与冲突信号或信号形成有关的文件：

- `internal/visibilityexception/application/derive_projection.go`（`judgeForks` 头注一句、`raiseConflict` 签名加因果条件、命令加 `Cause` 与 `Basis`、头注写恢复边界去处）与 `derive_projection_test.go`（分叉用例按因果条件找发作期、断言依据）
- `internal/visibilityexception/domain/fact_conflict.go`（`ExceptionSignal.Basis`）
- `internal/visibilityexception/application/raise_signal.go`、`raise_departure_gate_signal.go` 及其用例
- `internal/visibilityexception/domain/exception_case.go`、新文件 `signal_cause.go` 及用例
- `internal/visibilityexception/ports/ports.go`（`SignalEpisodeStore`、分诊意图头注）、`ports/case_read.go`
- `internal/visibilityexception/adapters/postgres/signal_episode.go`、`case_review.go`、`triage_handoff.go` 及用例
- `migrations/visibility_exception/0029_signal_episode_causal_condition_and_fact_basis.sql`（只动 `signal_episode` 一张表）

### 冲突信号（另列一格）

- **一并收了的**：冲突信号也按因果条件分期（分叉的那一条事实流），带事实依据（保留各方的源上下文、引用、版本）；同包裹另一条事实流分叉另开一期、重新分诊，此前它只给第一期记一次命中。
- **没收的：恢复边界**。分叉解没解要按链头判（ADR-0245），今天的分叉只在当前有效集里找，更正其中一支后旧链头离场、分叉就找不到了，可两个链头其实仍都当前有效——拿「找不到了」结束发作期就是虚构一次恢复。按链头判分叉与据此结束冲突发作期留给票 [docs-consistency/code/30](../../docs-consistency/code/30-forked-supersession-is-judged-by-business-time-and-not-kept-pending.md)（已追记；`raiseConflict` 头注与 UC-VE-004「给开发的交接」都指向它）。

### 代决与越权风险点

代决（裁定出自复核报告 M18；以下是落地时 M18 没写、又必须定下的格）：

1. **不立新 ADR**。M18 已裁「以那一条移动事实为因果条件，每次违规各开一期、重新分诊；命令加三样依据」，落地即回答 ADR-0205 越权风险点 22、23；照票 123 回答越权风险点 20 的先例，在 ADR-0205 Status 注记挂「已答（落地）」，没有条款停用。读了 ADR README 顶部与 ADR-0205 全文；ADR 号 0302 给了票 136（它要部分停用 ADR-0224 一句，必须有停用方）。owner 若认为本票的几格该进 ADR：把下面 2–7 条写成一份新记录，回指本票。
2. **因果条件的形状**：开放引用，产品自带两种写法，前缀分开。出发门禁取 `source-fact/TRANSPORT_FULFILLMENT/movement-fact/<包裹>/<事实>`，不带版本（更正版本不再命中，ADR-0205 越权风险点 8）；冲突取 `supersession-fork/<源上下文>/<事实引用>`，按事实流而不按分叉点的前身版本，因为分叉解没解按链头判、链头说的是事实流。改写法改 `signal_cause.go` 两个构造函数；已落库的不迁。
3. **依据随开期那一次命中固定**：同一期后来的命中只推进命中数与末次命中，不追加依据。出发门禁同一期的再命中只会是同一次出发的重投，依据相同；冲突信号同一事实流多出一个后继再命中时，新后继不进依据（越权风险点 3）。
4. **出发门禁阻断信号的发作期不按恢复结束**：原因是一次已经发生的出发，事后不会「不再成立」，替它结束就是虚构一次恢复（CONTEXT「条件不再成立且满足恢复边界 → 结束」在这一类上不成立是如实）；后续处置归分诊与案件。owner 若要它结束（例如关务放行或案件关闭即结束），给出发门禁阻断信号规则册加恢复边界与规则版本，另立票。
5. **存量回填**：迁移前落库的发作期回填为「登记于因果条件之前」、依据为空，不认领任何新命中；同一原因迁移后再命中另开一期、再分诊一次（越权风险点 2）。不替存量猜原因：一期里可能已记着几次不同出发的命中，说不出是哪一次。
6. **分诊意图信封 ID 加发作期维**：同对象同类型的后一期按旧 ID 会在 `EnqueueOnce` 被当成重放吞掉（恢复后重开那一格此前就有此病，只是没有生产路径走到）。这一类信封今天没有消费者（`.scratch/handoff-consumer-inventory/report.md`），换 ID 不影响下游。
7. **发作期册读口、线格式与管理台详情照列因果条件与依据**：ADR-0205 越权风险点 23 的后半句是「人工复核追不回是哪一次出发」，只存不给看等于没解。线格式只加键，`factBasis` 恒为数组；读口注释里「事实依据在发作期行上没有登记格」那句改掉。
8. **阻断那一格没有关务判断标识归不变量已破**（发布失败、不进未决名单）：领域构造门保证阻断必有判断标识，读回没有是坏行，重投一万次还是没有。

越权风险点：

1. **冲突信号仍无恢复边界**（VE owner）。见上节，去处票 docs-consistency/code/30。
2. **存量那一格各原因迁移后多分诊一次**（VE owner、运维）。分诊走向是自动建案时，这一次还会多建一个案件（新开一期走 `establishCaseFor`），与存量那一期关联的案件各是一个。演示库与首发库里存量发作期很少；冲突信号今天的生产源造不出分叉（code/30 票面），出发门禁那一类同一次出发的重投才会撞上。
3. **同一期依据不追加**（VE owner）。见代决 3。
4. **披露决定取同对象同类型最近一期**（VE owner）。`DecideDisclosureCommand` 只带对象与类型；一件包裹有几次违规出发时，披露针对的是最近那一期，同一对象两期并存时较早那一期取不到披露。`DecideDisclosureHandler` 今天生产未装配，没有暴露。要按期披露，命令改带发作期，归 UC-VE-006 那一侧。
5. **出发门禁阻断信号的发作期一直活跃**（VE owner）。发作期册上这一类的活跃行只增不减；「活跃」对这一类只说明没被恢复，不说明还在违规。
6. **迁移 0029 对存量全表回填一次**（运维）。行数少；表上没有外部写入方，迁移在发版窗口里跑。

### 修法 → 用例

| 修法（M18 / 票面） | 用例 |
|---|---|
| 以那一条移动事实为因果条件，每次违规各开一期、重新分诊 | `AT-VE-201`：`raise_signal_test.go::TestAHitWithAnotherCauseOpensItsOwnEpisodeAndIsTriagedAgain`、`raise_departure_gate_signal_test.go::TestEachBlockedDepartureOfAParcelOpensItsOwnEpisodeWithItsFactBasis`、PG `TestEpisodesOfOneParcelAndKindAreFoundByTheirCause`、`TestTwoEpisodesOfOneParcelAndKindEachHandOffTheirOwnIntent`、派发真库 `TestEachBlockedDepartureOfAParcelIsTriagedOnItsOwnThroughTheProductionRoute` |
| 命令加事实依据：`movement-fact/<对象>/<事实>` 引用、事实版本、逐对象关务判断标识 | `AT-VE-202`：领域 `TestAHitWithoutItsCauseOrFactBasisOpensNoEpisode`、编排受理 `TestACommandWithoutItsSignalFactsIsNotAccepted`、适配器 `TestADepartureProjectsOntoEveryObjectUnderControlAndSignalsTheBlockedOne`、查阅 `TestCaseReviewListsSignalEpisodesWithConclusions` 与 `TestExceptionTriageRecordsListSignalEpisodesVerbatim`、派发真库同上 |
| 冲突信号存量问题另列一格 | 上节；冲突信号的因果条件与依据：`derive_projection_test.go::TestAForkedSupersessionKeepsBothSuccessorsInTheProjection`、领域 `TestSourceFactAndForkCausesDifferAndAConflictSignalCarriesItsRetainedFacts` |
| 存量（迁移落地时已在库的发作期） | `AT-VE-203`：领域 `TestOnlyAnEpisodeRecordedBeforeCausesRehydratesWithoutFactBasis`、PG `TestAnEpisodeRecordedBeforeCausesStaysReadableButClaimsNoNewHit`、修订轮补的 `TestTheMigrationBackfillsEarlierEpisodesAsRecordedBeforeCauses`（执行迁移 0029 原文）、线格式同上 |
| 修正 `RaiseSignalCommand` 头注 | `raise_signal.go`（`1fa05d011`） |
| UC-VE-004 暂挂说明删去 | `8f2c35b57` |

提交前变异抽验：把 `FindLatestForCause` 的因果条件条件去掉（退回按对象+类型定位），派发真库用例转红。

### 自验

- `go build ./...`、`go vet ./...` 全仓过；改过的 `.go` 全部 `gofmt` 干净。
- 带 DSN（`127.0.0.1:55432`）`go test -count=1`：`./internal/visibilityexception/...`、反查出的 `./cmd/parcel-api/...`、`./cmd/parcel-dispatch/...`、`./cmd/parcel-ve-register/...`，连同 `./internal/architecture/...`、`./migrations/...`、`./internal/platform/migrate/...`，21 个包全 ok（含 PG）。新真库用例 `-v` 逐条 PASS、无 SKIP。迁移改号（SQL 不变）之后另跑 `./migrations/...`、`./internal/platform/migrate/...` 与 VE PG 适配器包，全 ok。
- 迁移 0029 的存量回填另在一次性库上手验：0002 形状的表里先写一行旧发作期，跑这份迁移后读回 `RECORDED_BEFORE_CAUSAL_CONDITION` 与 `[]`；库已删。
- `apps/admin-web`：`tsc -b --noEmit` 过、`scripts/run-tests.mjs` 619 pass、`vite build` 过。
- `tools/adr-index -check`、`tools/at-coverage-counts -check` 只报生成物过期（预期，进 main 时由推送方重生成）。没跑全量。

## 评审记录

评审 ← tops-20 隔离子代理 · 钉 `340c1581a` · 00:58

结论：两轴无阻断；非阻断 9 条。

### Standards
阻断：无。
非阻断：
1. `internal/visibilityexception/adapters/http/query_exception_triage_records.go` `signalEpisodeBody` 头注留着旧句「「事实依据」在发作期行上没有登记格，本体不带那一键」，与紧接的「factBasis 恒为数组」相抵；代决 7 说改了读口注释，实际只改了 `ports/case_read.go`。
2. 两处头注过时：`cmd/parcel-dispatch/assemble_movement_fact_backfill.go` `backfillMovementFacts` 仍写「在它让对象进的那一段上按段列出」（已被 ADR-0302 停用）；`adapters/postgres/signal_episode.go` `SaveRaised`、`SaveHit` 仍指 `FindLatest`，命中定位现走 `FindLatestForCause`。
3. ADR-0302「被停条款」整句引决定三，连「交接用 `EnteredByHandover`；揽收由…`EnteredByPickup`…依据的拼写收进同一处，进段、重派生与认都经它」也引进去了，可适用场景又说 `EnteredByPickup` 认法与拼写收口不受影响——被停条款比实际停用范围宽（引文逐字无误）。
4. ADR-0302 Status 行写「第 1、6 点同时待 TF owner」，但越权风险点 3、5 也挂了 TF owner。
无发现：上下文边界（VE 只经 TF ports/领域读，适配器在消费侧）；去向/来向分写、Links 两头；ADR-0205 风险点 22、23 引文逐字。

### Spec
阻断：无。
非阻断：
1. `AT-VE-203` 点名的 `TestAnEpisodeRecordedBeforeCausesStaysReadableButClaimsNoNewHit` 是迁移后直接插回填字面量，没执行迁移 0030 的 UPDATE。仓内先例：TF `TestTheMigrationBackfillsEarlierDeparturesAsRecordedBeforeTheGateReading` 抽出迁移原文执行。评审探针（一次性库：按 0002+0003 表形写旧发作期行，跑 0030，读回 `RECORDED_BEFORE_CAUSAL_CONDITION` 与 `[]`；带依据新行也通过）证行为对，只缺自动化用例。
2. 135 越权风险点 2、4 披露不全：风险点 2 只写「多分诊一次」，没写分诊走向为自动建案时会多建一个案件（`Handle` 新开一期走 `establishCaseFor`）；风险点 4（披露取最近一期）：`DecideDisclosureHandler` 生产未装配故未暴露、与 CONTEXT「信号发作期」不相抵，但同一对象两期并存时较早那一期取不到披露，宜补进风险点。
3. 「另列一格」满足；但 code/30 追记加了「恢复边界随冲突信号规则那一版」，那张票的「规模」与「要裁的：无」没跟着改，宜同步。
4. ADR-0302 越权风险点 4 只提段册按消费那一刻算；对象是集运单元时扫早先各段同样按此刻成员册展开，没提。
5. 管理台没加 node:test（visibility 页本无测试，`episodeDetailFields` 是 TSX 非导出函数）；按 workflow「前端切片」第 5 步业务页自查即可，不挡。文案中文、只读展示。
无发现（附注摘要）：生产 `RaiseSignalCommand{` 只有两处构造点（`raise_departure_gate_signal.go`、`derive_projection.go` `raiseConflict`），都带 `Cause` 与非空 `Basis`；重放幂等成立；缺关务判断标识不会让正常路径失败（TF `NewGateFinding` 构造门）；存量格三处约束一致（迁移 CHECK、`RehydrateSignalEpisode`、`SignalHit.valid`）；分诊意图确无消费者、分区键未变；136 理由属实（TF 无续办进段那一路）；末段照丢已披露；VE CONTEXT 两句与 `AT-VE-180`、`AT-VE-204` 一致；新号不撞；VE 各包带 DSN 跑 go test 通过。

## 修订轮（2026-10-26，作者子代理）

先 rebase 到 origin/main（main 多出 74 笔，含票 139 的 AT 盘点守卫）：只在 `.scratch/at-coverage-inventory.md` 文末追记处冲突一次，两边都留（票 134、code/01、127/48/167、139 的追记在前，本票与 136 的追记在后）；分桶表行无冲突，`AT-VE-201`–`204` 各有一行。

| 评审条目 | 处置 | 落点 |
|---|---|---|
| Standards 1：`signalEpisodeBody` 头注旧句 | 改成「因果条件与事实依据照登记原样透出；factBasis 恒为数组」 | `9661b63fa` |
| Standards 2：`backfillMovementFacts` 头注、`SaveRaised`/`SaveHit` 指 `FindLatest` | 改成扫每一段；改指 `FindLatestForCause` | `9661b63fa` |
| Standards 3：ADR-0302 被停条款过宽 | 收窄到实际停用的两处原句（逐字），`EnteredByHandover`/`EnteredByPickup` 那半句写进适用场景、不停用；ADR-0224 注记同改 | 票 136、`9661b63fa` |
| Standards 4：ADR-0302 Status 漏挂 TF owner | 改为「第 1、3、5、6 点同时待 TF owner」 | `9661b63fa` |
| Spec 1：`AT-VE-203` 没执行迁移 | 新真库用例 `signal_episode_migration_test.go::TestTheMigrationBackfillsEarlierEpisodesAsRecordedBeforeCauses`：把库退回迁移前表形、写两行存量（活跃、已结束）、执行迁移 0029 原文，读回存量格与空依据、命中与结束原样、不认领带原因的命中、迁移后新发作期写得进；AT 盘点该行补点名 | `9661b63fa` |
| Spec 2：本票越权风险点 2、4 | 风险点 2 补「自动建案走向会多建一个案件」；风险点 4 补「两期并存时较早那一期取不到披露」 | 本笔 |
| Spec 3：code/30 「规模」「要裁的」 | 两格随追记同步（只动这两格与追记末句，写明由本票改） | `9661b63fa` |
| Spec 4：ADR-0302 越权风险点 4 | 补集运单元按消费那一刻成员册展开 | 票 136、`9661b63fa` |
| Spec 5 | 不改（协调方定） | — |
| 协调方：迁移改号回 0029 | `0030_*.sql` 改名回 `0029_*.sql`，SQL 不改，代码注释、测试、ADR-0205 注记、code/30 追记、管理台注释、两票面同步 | `9661b63fa`、本笔 |

修订轮验证（rebase 后 tip，代码等于 `9661b63fa`）：

- `go build ./...`、`go vet ./...` 全仓过，`gofmt` 干净。
- 带 DSN `go test -count=1`：`./internal/visibilityexception/...`、`./cmd/parcel-api/...`、`./cmd/parcel-dispatch/...`、`./cmd/parcel-ve-register/...`、`./internal/architecture/...`、`./migrations/...`、`./internal/platform/migrate/...` 全 ok（含 PG）；新用例 `-v` PASS。`go test ./tools/at-coverage-counts/...` ok。
- `go run ./tools/at-coverage-counts -check` 与 `go run ./tools/adr-index -check` 只报生成物过期，没有缺行或多行。
- `apps/admin-web`（只改了一行注释）：`tsc -b --noEmit` 过、`scripts/run-tests.mjs` 647 pass、`vite build` 过。
