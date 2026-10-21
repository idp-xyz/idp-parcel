# 70 SA 冻结与暴露账本按作用域整册回写，并发下丢失更新，已释放的钱被静默占回去

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code70（见文末「完成记录」）。此前 ready-for-agent——R1（上线前必修）：碰钱、静默。同一结算作用域下两份委托的控制或释放并发时，后提交的那一笔把它读到的整册快照写回，先提交那一笔刚落的释放（或冻结）被压回去；客户可用余额或额度被吃掉，不报任何错。问题早于 code/20；code/20 让接受链对被取代版本每一轮都请求一次释放，整册回写的次数变多了。2026-10-21 协调方据 code/20 隔离评审 N1 立。
Blocked by: 无
出处：code/20 隔离评审 N1；code/20 完成记录「评审记录」。

## 现象

`ApplyPreAcceptanceControlHandler` 与 `ReleasePreAcceptanceControlHandler` 都是「`LoadForScope` 读整本作用域账本 → 领域改一条 → `Save` 把整本写回」。`sapostgres.FreezeLedgers.Save`（暴露那本同形）对账本里每一条都做 `INSERT … ON CONFLICT DO UPDATE SET status, released_at`。

并发两笔落在同一（租户，作用域）上：
- A 读到账本，此时委托 X 那一笔还是「已冻结」；
- B 释放 X 那一笔并提交；
- A 冻结自己那一笔，`Save` 整册写回，X 那一行被改回「已冻结」、`released_at` 清空。

X 的钱又被占住，没有任何一方报错。两笔都是释放时同理：后写的那一笔把对方的释放压回去。

## 证据

- `internal/settlementaccounting/adapters/postgres/pre_acceptance_control.go`：`FreezeLedgers.Save` 遍历 `ledger.Entries()` 逐条 upsert，`DO UPDATE` 无条件覆盖 `status` 与 `released_at`；暴露账本的 `Save` 同形。
- `internal/settlementaccounting/application/apply_pre_acceptance_control.go` 与 `release_pre_acceptance_control.go`：读与写之间没有作用域级的锁，事务隔离级别是默认的读已提交。
- SA CONTEXT：「重复请求、查询和补偿不得形成重复冻结或重复释放」；冻结金额由账本当场求和（迁移 `settlement_accounting/0010` 头注），一行被改回「已冻结」即重复占用。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：改两本账的写入与一条读写路径，补真库并发用例（两连接、屏障）。

## 最小的诚实修法

两种取一：
1. **作用域级串行**：施加与释放在 `LoadForScope` 之前按（租户，作用域）取 `pg_advisory_xact_lock`，锁与写入在同一笔事务里（先例：code/14 的分户账锁、code/39 的来源身份锁）。改动小，同一作用域的控制排队。
2. **只写变化的条目**：`Save` 只写本次领域改动的那几条，`DO UPDATE` 加条件（例如只允许「已冻结 → 已释放」、已释放的行不再被改回）。不排队，但领域账本要能交出「改了哪几条」。

两种都要补真库用例：两路并发、一路释放 X、一路冻结 Y，提交后 X 仍是已释放、Y 已冻结、可用余额等于两者算出来的数。

附注：code/20 已先止血一格——`ReleasePreAcceptanceControlHandler` 遇到已是「已释放」的冻结或暴露时跳过 `Save`，重复释放不再整册回写。首次释放与施加仍整册写，本票要收的是这两格。

## 要裁的

无。两种修法属机制，落地时按代价选一种，选法写进完成记录。

## 完成记录（2026-10-21，分支 claude-code70，基于 main `3e3b08300`）

### 做了什么

- `60ff9788a` 两种修法兼用：
  - 新端口 `ports.PreAcceptanceControlScopeLock` 与适配器 `sapostgres.PreAcceptanceControlScopeLocks`：按（租户，法人，账户，币种）取 `pg_advisory_xact_lock`，锁空间 `settlement_accounting.pre_acceptance_control.scope`，走 `RequireExecutor`，锁与读写同一笔环境事务。
  - `ApplyPreAcceptanceControlHandler`：预付冻结一项在读运营余额之前取锁，信用校验一项在读信用状况之前取锁（授信依据与比例基数在锁外取）；`ReleasePreAcceptanceControlHandler` 在读冻结账本之前取锁。取不到锁分别停在 `FREEZE_LEDGER_UNAVAILABLE` / `EXPOSURE_LEDGER_UNAVAILABLE`，带续办引用，不读不写。`ApplyPreAcceptanceControlDeps.ScopeLock` 进构造门；`NewReleasePreAcceptanceControlHandler` 多一参。
  - `FreezeLedgers.Save`、`CreditExposureLedgers.Save`：`DO UPDATE` 加 `WHERE 库里占着 AND 这一册已释放`，别的撞键行一行不写。
  - 装配：`cmd/parcel-dispatch` 施加与释放、`cmd/parcel-api` 拒绝、处置、撤回三处释放接上锁（照 ADR-0222，构造仍在各 `assemble*.go`）。
  - 真库用例（`internal/settlementaccounting/adapters/postgres/pre_acceptance_control_concurrency_test.go`，屏障停在先手读完账本之后，后手或已提交、或经 `pg_locks` 看到等在咨询锁上才放；两本账各一遍）：
    - `TestAReleaseCommittedDuringAnotherControlOnTheScopeIsNotWrittenBack`：先手为 Y 施加、后手释放 X；X 保持已释放，Y 占着，本书占用合计只剩 Y。修前红在 X 回到 `HELD` / `RECORDED`。
    - `TestConcurrentControlsOnOneScopeJudgeAgainstEachOthersOccupation`：余额或额度只够一笔，两路施加；先手占用、后手业务限制，两路都提交。修前红在先手整笔回滚（两路编出同一个内部编号撞 `*_id_unique`）。
    - `TestConcurrentReleasesOfOneControlAnswerTheReleaseThatLanded`：同一笔两次释放并发，两路交回的释放时刻都等于库里那一个。
  - 写口用例 `TestAStaleSnapshotNeitherRevivesAReleaseNorRewritesUnchangedEntries`：旧快照写回不改回库里的释放、原释放时刻不变；未变的行行版本（`xmin`）不变，原样再存一遍也不变。`TestTheScopeLockRefusesToRunOutsideATransaction` 是锁的无事务负向证据（PBC-08）。
  - 编排用例：`TestEachControlItemTakesTheScopeLockBeforeReadingItsBook`、`TestAnUnavailableScopeLockHaltsTheItemBeforeItsBookIsRead`、`TestAReleaseTakesTheScopeLockBeforeReadingEitherBook`；构造门用例补「scope lock」一格。
  - code/20 那处止血的注释改写：已释放不回写的理由换成「没有要推进的行，写一遍只是白锁一遍」，旧理由（会压回别人的释放）已由写口守住。
- `ee8e0eb5d` SA CONTEXT「运营余额、收付款与坏账」补规则句；UC-SA-002「一致性、幂等与并发」补一行引它。UC 里没有本问题的现状注，无可改。

### 代决（用户授权「你作为业务和系统专家自决……解决所有的问题」）

1. **两种修法兼用，锁为主、写口为底。** 只改写口（只写变化）收得住票面那一格丢失更新，收不住另外两格：两路施加各自判够时撞内部编号整笔回滚（修前实测），同一笔两次释放各报一个释放时刻。这两格只有排队才对。写口那一层不靠锁：绕过编排的旧快照写回同样压不回释放，代价是一个 `WHERE`。
2. **一把锁管两本账。** 释放按冻结、暴露的次序碰两本，施加的次序由策略正文定（ADR-0122），每本一把锁会有两路按相反次序取锁而互等。锁不承载余额，ADR-0047 的「两本账互不借用」不受影响。
3. **锁取在每一项读本账的事实之前**，不提到 `Handle` 开头：估价、控制策略、授信依据都不是本作用域两本账上的事实，放在锁外少排一段队。组合策略两项各取一次，同事务重入。
4. **取不到锁不新增原因码**，归入那本账读写不了的那一格：恢复动作相同（续办重试），PS 侧的封闭集与译法不动。
5. **无迁移**（咨询锁不改表）、**无 ADR**（机制，票面已列为机制）。

### 越权风险点（待 SA owner 复核）

1. **同一作用域上的控制全部排队，且锁持到外层事务提交。** 锁随环境事务放，PS 消费门那一笔事务提交之前，同一结算账户上的别的委托都等着。大客户单账户高并发时接受链会排队。
2. **死锁可能（未实测）。** 外层事务若在取作用域锁之前已持有别的行锁（如同一委托的聚合行），另一路持作用域锁后又要那一行，PostgreSQL 死锁检测会中止其中一路，走既有的错误重投。两路要碰同一份委托，聚合乐观锁本来也会让其中一路回滚。
3. **锁失败与账本读不回同一格**，运维从原因码看不出是锁等超时还是库读不回。
4. **写口静默跳过。** 绕过编排、拿旧快照写回时，库里已释放的行不动、不报错；那一路手里的账本与库不一致（它交回的释放时刻可能不是库里那个）。编排路径在锁下，不会走到这一格。
5. **`Save` 仍逐行整册。** 写的条数随作用域历史增长（只增不删），没改成只交变化的条目：要改就得让领域账本交出「改了哪几条」，本票不动领域。规模起来后另立票。
6. 授信依据与比例基数在锁外取。基数读的是本上下文的运营余额登记与已发布对账单，它们不由控制写，本票不管。
7. `NewReleasePreAcceptanceControlHandler` 照旧不判 nil，与原有两件同；装配漏传锁会在第一笔释放时 panic。
8. 存量：首发前只有演示租户；被压回去的旧行不会自愈，`--reset` 重灌。

### AT

- 未改 AT 盘点；未重生成 `docs/product/MECHANISM-INVENTORY.md`、ADR 索引与 AT 计数。

### 验证（代码同 `60ff9788a`）

- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/settlementaccounting/... ./internal/parcelshipment/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./cmd/parcel-settlement-register/ ./internal/customscompliance/adapters/settlementaccounting/ ./internal/parcelpricing/adapters/settlementaccounting/ ./tests/bentocontract/... ./internal/architecture/... ./migrations/...`：退 0，40 个包 ok、3 个无测试文件、0 FAIL。`cmd/` 与 `internal/` 下导入 SA 编排、端口或 postgres 适配器的包（`go list` 算）都在这一组里。
- 新用例与改动用例 `-count=3`（SA `adapters/postgres`、SA `application`）：ok。
- 变异：去掉释放那一处锁，同一笔两次释放用例两本都红；去掉施加两处锁，只够一笔用例两本都红；去掉写口 `WHERE`，旧快照用例两本都红；锁与写口都去掉，四条用例全红（即修前）。恢复后绿。
- `go build ./...`、`go vet ./...`、`gofmt -l` 干净。
- 演示种子：一次性库 `code70seed` 上灌入退 0（「种子灌入完成」），用后 `DROP DATABASE code70seed WITH (FORCE)`。无迁移。

