# 96 释放口外那层 savepoint 在暴露账本读不回时连已落库的冻结释放一起撤掉

Category: bug
Status: resolved——2026-10-26 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-code96（见文末「完成记录」）。此前 ready-for-agent——R2（上线后尽快）：组合策略（预付冻结与信用校验同在）下，冻结那本已落库的释放会因为暴露那本读不回被一并撤回，与 ADR-0122 决定三相反；终局各路的补偿引用不持久、没人重放，那一笔冻结就一直占着。只在两本账都有占用、且暴露那本恰好读不回时出现。2026-10-26 协调方据 code/71 隔离评审信息 3 立（用户「你作为业务和系统专家自决」授权）；问题由 code/72 带进来，code/71 未改其行为。
Blocked by: 无
出处：code/71 隔离评审信息 3；[code/72](./72-a-customer-supplement-pause-releases-the-paused-versions-occupation.md) 评审 N1 引入 `pspostgres.SavepointedControlRelease`。

## 现象

- [ADR-0122](../../../docs/adr/0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md) 决定三：「冻结那本已落库的释放在暴露账本读不回时不撤回：续办重放，冻结账本按幂等交回原释放答案，暴露账本再认领一次——两本各自收口，不必同事务。」SA 释放编排照此写：先认领冻结、`Save`，再读暴露账本，读不回就交`未形成`。
- PS 侧的释放跑在调用方那一笔事务里，外面包着 `pspostgres.SavepointedControlRelease`：内层答的不是`已收口`就 `ROLLBACK TO SAVEPOINT`。冻结那本刚落的释放在同一个 savepoint 里，于是一并撤回。
- 后果：终局各路（撤回、拒绝、处置）留一条补偿引用，那条引用不持久、没有东西按它重放；原本已经放掉的冻结又占回去，直到有人碰巧重放。

## 证据

- `internal/settlementaccounting/application/release_pre_acceptance_control.go` 的 `ReleasePreAcceptanceControlHandler.Handle`：冻结 `Save` 成功后才 `LoadForScope` 暴露账本，读不回 `return handler.notFormed(command, ExposureLedgerUnavailable), nil`；注释「冻结那本已经落库的释放在这里不会被撤回」。
- `internal/parcelshipment/adapters/postgres/control_release_savepoint.go` 的 `SavepointedControlRelease.ReleasePreAcceptanceControl`：`releaseErr != nil || answer.Outcome != ports.ControlReleaseSettled` 时一律 `ROLLBACK TO SAVEPOINT`。
- 退回 savepoint 在两种处境下理由不同：暴露账本因 SQL 级错误读不回时，事务已进中止态，不退回整笔都提交不了；因非 SQL 原因读不回（读口交 nil 之类）时，事务没中止，退回只撤掉了一笔本该留下的冻结释放。

## 三类

机制。无产品策略、无租户取值。

## 规模

S–M。

## 最小的诚实修法（候选，见「要裁的」）

1. **只在事务真的中止时退回。** 装饰器在内层答非`已收口`后探一次事务状态（如执行一条无副作用语句看是否答 25P02）：中止了才 `ROLLBACK TO SAVEPOINT`，否则 `RELEASE SAVEPOINT`、保住已落库的那一半。只改 PS 一侧；SQL 级错误那一格冻结释放仍随退回撤掉，但事务本就保不住它。
2. **SA 释放编排两本账各开一个 savepoint。** 冻结那本释放落库后收起自己的点，暴露那本出错只退回暴露那一段；ADR-0122 决定三在共享事务里也成立。改的是 SA 应用层（要 SA owner）。
3. **释放不进调用方事务。** 在独立事务里释放、业务事实先提交：两本各自收口照 ADR-0122 成立，但释放与业务事实不再同笔，接受链开头「先放后判」的保证要重新论证。

## 要裁的

- 选哪一种；倾向 1（只动 PS 装饰器，最小），若 SA owner 愿意改编排则 2 更彻底。
- 若选 1，SQL 级错误那一格仍会撤掉冻结释放，是否在 ADR-0122 决定三上挂一句读法注说明共享事务下的这一格。

## 完成记录

分支 `claude-code96`，基于 main `c8f0f3522`（收尾 rebase 之后以回报里的最终 SHA 为准）。选候选 1，只改 PS 一侧。

### 做了什么

- `SavepointedControlRelease`（`internal/parcelshipment/adapters/postgres/control_release_savepoint.go`）：内层答`已收口`直接 `RELEASE SAVEPOINT`；答类型化的`可重试未形成`时，先在 savepoint 里执行一条 `SELECT 1` 探事务状态——成功才算没中止，收起 savepoint、保住提供方已落库的那一半；失败（25P02、探测自己出错、ctx 已取消、连接断开）一律按已中止，`ROLLBACK TO SAVEPOINT`；答错误或答`确定未受理`不探，不论事务如何一律 `ROLLBACK TO SAVEPOINT`。答复原样交回，开、退、收本身失败仍按`可重试未形成`。框架 `bentopg.Executor` 不暴露连接，读不了 `TxStatus`，所以只能经 SQL 探。
- 文档：新 ADR-0291（部分停用（收窄）ADR-0282 决定一末条前半句；ADR-0122 决定三挂读法注、不停用）；ADR-0282 与 ADR-0122 的 Status 注记与 Links 加前向指针；UC-PS-005 规则一条与 `AT-PS-160` 一行；盘点 `AT-PS-160` 一行入①、一段追记。无迁移，不改 SA、不改端口、不改装配。

### 释放口答非`已收口`的路径，哪些是事务已中止、哪些是正常

| 路径（`ReleasePreAcceptanceControlHandler.Handle` 与适配器） | 事务 |
|---|---|
| `LockControlScope` 的 `pg_advisory_xact_lock` 语句报错（锁等超时、死锁受害者、语句超时） | 已中止（SQL） |
| 冻结或暴露账本 `LoadForScope` 的查询报错，或遍历行时服务端报错 | 已中止（SQL） |
| 冻结或暴露账本 `Save` 的 UPSERT 报错 | 已中止（SQL） |
| 暴露行的指纹不是本适配器写下的形状、`freezeSpecOf` 或 `Rehydrate*Ledger` 重建失败、读口交 nil | 正常——语句没出错；这是票面的"读不回"那一格，也是修的对象 |
| `ReleaseRequestNotAccepted`（身份或作用域立不起来，零值作用域） | 正常——在读任何账本之前短路，没有东西可留可撤；答`确定未受理`，退回 |
| `ledger.Release` / `exposureLedger.Release` 报错（编程错误、时钟异常）→ 适配器译成`ErrUntranslatableAnswer` | 正常；暴露那本的出在冻结已落库之后；答错误，退回，冻结释放随之撤掉 |
| 冻结 `Save` 里"入册的冻结没有幂等指纹"（领域 bug） | 正常；答错误，退回，前面逐行已写的行一并撤掉 |
| 适配器译不动的答复（空原因、集合外的值） | 正常，无写；答错误，退回 |

探测靠"成功才算没中止"区分：中止态的事务对除退回以外的语句一律答 25P02，所以探测成功不可能是中止；反方向（正常事务上探测因 ctx 取消、连接断开失败）被按已中止处理，多退回一次，回到旧的保守落法。

### 对 code/71、code/102 的核对

- code/71 的三格语义不变：装饰器不改答复，只改 savepoint 收起还是退回；`确定未受理`在读账本之前短路，无写，退回，两本账都没动，守卫例 `TestAReleaseTheProviderRefusesLeavesTheTransactionUsableAndTheLedgersUntouched`。可重试未形成仍走补偿续办或停下。
- code/102 与 ADR-0290 的"放不收口整轮回滚"是应用层按自己的保证做的决定（`occupationRelease.settleGap` 之后由形成决定那一步交未决、消费门回滚），与本层保不保住那一半无关：整轮回滚时保住的那一半随整轮撤回，行为与之前一致。`cmd/parcel-dispatch` 与应用层的既有用例全绿。
- ADR-0122 决定三原文「冻结那本已落库的释放在暴露账本读不回时不撤回：续办重放，冻结账本按幂等交回原释放答案，暴露账本再认领一次——两本各自收口，不必同事务」：语句没出错那一格现在与原文一致；SQL 级错误那一格事务已中止，保不住，写进读法注。
- ADR-0282 决定一末条「释放口外那一层 savepoint 对非`已收口`的答复与错误都退回」与新行为相抵（答`可重试未形成`且事务没中止时收起），所以立 ADR-0291 收窄，不是只挂读法注。

### 修前红与修后绿

- 修前（只有测试、装饰器未改）：`TestACombinedPolicyReleaseKeepsTheFreezeReleaseWhenTheExposureLedgerCannotBeReadBack`（读口交错误，冻结行 `status = 1`、无释放时刻）与 `TestACombinedPolicyReleaseKeepsTheFreezeReleaseWhenAnExposureRowCannotBeRebuilt`（暴露行指纹损坏的真数据，同）红；装饰器层 `TestASavepointKeepsWhatARetryableNotFormedReleaseAlreadyPersistedWhileTheTransactionIsHealthy` 的可重试未形成格与 `TestASavepointDecidesEachReleaseByItsOwnAnswerAndTheTransactionState` 红（首版三格都保住、测试名是 `TestASavepointKeepsWhatAReleaseAlreadyPersistedWhileTheTransactionIsHealthy`，评审修订后拆成两个）。
- 修后全绿。SQL 级错误那一格（`TestACombinedPolicyReleaseStillUndoesTheFreezeReleaseWhenTheExposureReadFailsInSQL`：暴露表在同事务里被改名，冻结释放随退回撤掉、冻结行仍占着，调用方事务继续读写、提交）、确定未受理、冻结账本读不回（不动账本）、已收口（两本落库、重放答原释放时刻）四格修前后都绿，是守卫。续办重放（读口第二次读回）：冻结释放时刻不变，暴露行再认领为已释放。

### 测试与变异

- `scripts/test-full.sh` 范围：`./internal/parcelshipment/...`、`./internal/visibilityexception/adapters/parcelshipment/...`、`./tests/bentocontract/...`、`./cmd/...`、`./internal/architecture/...`、`./migrations/...`，全部 ok、退 0；`go build ./...`、`go vet ./...`、`gofmt -l` 干净；`go run ./tools/adr-index` 能生成（README 不提交）、`go run ./tools/at-coverage-counts -check` 退 1（只是总览过期，新号分桶行已补）。新用例 `-count=3` 稳定。
- 变异（备份、还原后 `cmp` 一致），全部被杀：①探测恒判没中止（该退回时收起）：装饰器层四例红、`cmd/parcel-dispatch` 的 SQL 级错误例与既有两例中毒释放例红；②探测恒判已中止（回到旧行为）：装饰器层与 `cmd/parcel-dispatch` 保住那一半的例红；③内层答已收口时误退回（`if settled || !usable`）：装饰器层已收口格、`cmd/parcel-dispatch` 已收口例与组合策略例红；④答错误时一律退回（`|| releaseErr != nil`）：装饰器层错误格红（首版变异，修订后错误一格本就退回）；⑤`确定未受理`不探测（中止的事务上收起）：装饰器层"中止时确定未受理"格红（首版变异；修订后`确定未受理`不论事务如何都退回，该变异不复存在，见「评审记录与修订轮」的变异）。变异①首轮暴露一个测试写法问题：断言写在事务闭包里用 `t.Fatalf`，Goexit 卡住事务收尾、`cmd/parcel-dispatch` 超时；改成闭包交回错误。

### 代决

1. **立 ADR-0291。** ADR-0282 决定一末条是已接受的口径，新行为与之相抵；只挂读法注不够。ADR-0122 决定三不停用，只挂读法注说明共享事务下 SQL 级错误那一格。
2. **保住的一半只限类型化的`可重试未形成`且事务没中止；错误与`确定未受理`一律退回。** 依据：ADR-0122 决定三只为提供方明确承诺「重放会补完另一本」的类型化`可重试未形成`背书；错误是不知道发生了什么，事务没中止只说明连接可用，不说明提供方的写自洽；`确定未受理`是「我没受理」，后面带着已落库的写自相矛盾；这两种按 ADR-0282 是重放同一答复也不会变、待人处理，保住一半后没人补完。`可重试未形成`一格还要探事务状态，因为同一格既可能来自 SQL 级错误也可能不是，格分不出事务状态。装饰器是对 `ports.PreAcceptanceControlRelease` 的通用护栏，不只服务 SA。（首版写成「落法只看事务状态」，评审 N1 后收窄，见「评审记录与修订轮」。）
3. **探测失败一律按已中止。** 保守方向，代价只是偶尔多退回一次。
4. **探测语句用 `SELECT 1`，在 savepoint 里执行。** 探测自己失败（如语句超时）也被随后的退回收拾。
5. 测试放两处：装饰器层用临时表证"留、撤、逐次互不牵连"，`cmd/parcel-dispatch` 用生产的 SA 释放编排与真实两本账证组合策略；不起整条委托闭环，释放口与两本账、作用域锁都是生产实现。

### 越权风险点

1. **SQL 级错误那一格冻结释放仍撤。** 暴露账本因锁等超时、语句超时、权限、表被挪走读不回时，冻结那本的释放照旧随退回撤掉，补偿引用不持久、没人重放，那一笔冻结仍占着。要根治只有候选 2（SA 释放编排两本账各开 savepoint，SA owner）。本票只修了语句没出错的那一格。
2. **保住的只是冻结那一半。** 暴露账本读不回的那一笔仍占着，终局各路的补偿引用不持久（ADR-0282 越权风险点 5），没有东西替它重放，仍待人处理或碰巧重放。
3. **保住的一半只限类型化的`可重试未形成`；错误与`确定未受理`一律退回。** `release exposure` 之类编程错误出在冻结已落库之后，冻结释放随退回撤掉，冻结那本仍占着，那一笔按 ADR-0282 记册待人处理；这是旧行为，不是新退步。要保住错误一格的一半，须提供方先把"错误"收窄成类型化的`可重试未形成`。
4. **探测多一次往返，只在答`可重试未形成`时。** 已收口主路与答错误、答`确定未受理`都不探。
5. **保住的那一半在调用方整轮回滚时一并撤回**（接受链放不收口、形成决定处放不收口）：本层不改应用层的回滚决定。

### 评审记录与修订轮

独立评审结论可合入，提 N1、N2 与一处措辞，协调方 2026-10-26 裁定如下，在已有分支上加新提交修订。

- **N1（采纳，并收窄到只留类型化的`可重试未形成`）。** 首版「内层答非已收口后，只要事务没中止就收起 savepoint」会把答错误、答`确定未受理`时已落库的一半也保住，而这两种都没人补完。改为：只有内层答类型化的`可重试未形成`（Outcome 为 NotFormed 且无错误）且事务没中止才收起；答错误、答`确定未受理`一律退回（旧行为，代价为零）；已收口仍直接收起；事务已中止（探测失败）仍退回。理由写进 ADR-0291（决定一、候选与反方、越权风险点 3 与 6）与装饰器头注。
  - 测试：装饰器层拆成"`可重试未形成`与已收口、事务没中止 → 留"（`TestASavepointKeepsWhatARetryableNotFormedReleaseAlreadyPersistedWhileTheTransactionIsHealthy`）与"`确定未受理`、形不成三格之一的错误、事务没中止 → 退回"（`TestASavepointUndoesTheHalfOfAnAnswerOtherThanARetryableNotFormedEvenWhileTheTransactionIsHealthy`）；"事务已中止 → 退回"三格不动。修前（装饰器未改）后一例两格红。
  - 变异（备份、还原后 `cmp` 一致）：①条件放回"只看事务状态"（`keepsPersistedHalf := transactionUsable(...)`）：新增例的`确定未受理`与错误两格红，只这两格；②`可重试未形成`不探事务（该退回时收起）：事务已中止的可重试未形成格、`TestASavepointKeepsAFailedReleaseFromAbortingTheCaller`、逐次例红；③`可重试未形成`一律退回：留的例（装饰器层可重试未形成格与逐次例、`cmd/parcel-dispatch` 组合策略两例）红。
  - 随之改的文档：ADR-0291 标题、Status、决定一、候选与反方、适用场景、Consequences、越权风险点 3、4、6，ADR 文件名随标题改成 `0291-the-release-savepoint-keeps-what-a-retryable-not-formed-release-persisted-while-the-transaction-is-alive.md`，入向指针（ADR-0282、ADR-0122、UC-PS-005）同步；ADR-0282 Status 注记与 Links 里收窄的适用场景；UC-PS-005 释放那条规则与 `AT-PS-160` 一行；盘点 `AT-PS-160` 的测试名与追记。
- **N2（采纳）。** UC-PS-005 新增那条规则末尾括号原写「接受链形成决定处同此」，失准：接受链形成决定处放不收口是整轮回滚（ADR-0290），保住的一半随之撤回。改为"在 savepoint 一层同此；接受链形成决定处放不收口是整轮回滚，保住的一半随之撤回"，并点名「撤回、主动拒绝抢先提交之后，接受链这一轮仍可能占下资金」那条规则，不用位置词。
- **措辞（采纳）。** 测试与 ADR 里"已收口路径不回归"一类带变更叙述味的说法，改成"已收口路径两本账都落库"；装饰器测试注释里"不因这条改动回归"同改。

## 进 main 记录（2026-10-26，协调方）

- 分支 `claude-code96` 由作者 rebase 到 origin/main `ae370c593` 后快进，tip `ad6f23278`。无迁移；新立 ADR-0291（收窄 ADR-0282 决定一末条前半句；ADR-0122 决定三不停用，只挂读法注）。
- 协调方分诊：R2，选候选 1（只改 PS 一侧的 `SavepointedControlRelease`）。落法：内层答`已收口` → `RELEASE`；答类型化的`可重试未形成`且探测（savepoint 内 `SELECT 1`）确认事务没中止 → `RELEASE`，保住提供方已落库的那一半；答错误、答`确定未受理`、探测失败（按已中止）→ `ROLLBACK TO SAVEPOINT`。
- 隔离评审（Sonnet）可合入，无阻断；核实探测可靠（中止态除 ROLLBACK TO 外一律 25P02，`SELECT 1` 成功不可能是中止；调用方事务进装饰器前已中止时 `SAVEPOINT` 本身就失败、不进内层）、保住一半在重放下幂等、两本账不一致的形状正是 ADR-0122 决定三接受的。N1（评审建议答错误且事务没中止时也退回）协调方采纳并再收窄一格：保住的一半只限类型化的`可重试未形成`，错误与`确定未受理`一律退回（提供方只对前者承诺"重放会补完另一本"，其余两种待人处理，保住一半后没人补完；代价为零，退回就是旧行为）；N2 UC-PS-005 括号失准、措辞由 Sonnet 子代理收掉，ADR-0291 文件名随新决定改名。
- 协调方单独提交重算的 AT 计数、重生成的 ADR 索引与机制清点；检查均退 0。
- 全量 `scripts/test-full.sh` 退 0、158 个包 ok；`go build ./...`、`go vet ./...` 干净。无迁移，种子未灌。
- 去向：无新票。越权风险点：SQL 级错误那一格冻结释放仍随退回撤掉（要根治只有票面候选 2，SA 释放编排两本账各开 savepoint，需 SA owner）；保住的只是冻结那一半，暴露那本仍占着，终局各路补偿引用不持久、没人重放（ADR-0282 越权风险点 5）；保住的那一半在调用方整轮回滚时仍一并撤回。
