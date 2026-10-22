# 50 VE 客户视图派生在 READ COMMITTED 下会被并发的旧投影信封带回旧依据或旧内容

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code50（基于 main `be6b5ad20`，见文末「完成记录」）。此前 needs-triage——建议 R2（上线后尽快）：答错且静默，但要两封同一包裹的投影派生信封并发消费、交错落在一个窄窗口里才出现。2026-10-21 文档自洽 code/05 隔离评审立（用户「你作为业务和系统专家自决」授权），分支 claude-code05。分级由协调方定稿。
Blocked by: 无（建在 [code/05](./05-customer-tracking-view-overwrites-published-versions.md) 的 `ConfirmedOn` 之上，随 code/05 进 main）
出处：code/05 隔离评审 N4。code/05 只修存储形状与换代判据，不修这个竞态。

## 现象

同一包裹先后形成投影 P1、P2，两封派生信封被两个消费事务并发处理。处理 P1 的事务 A 先读到当前投影仍是 P1，于是认定自己的信封不旧；处理 P2 的事务 B 随后提交了 P2 的视图（换代为 V2，或把 V1 的确认依据前移到 P2）。A 接着读当前视图、按 P1 重新得出四维再写：

- 四维与当前版不同：A 以 P1 的内容替代 V2，已经不再有效的内容重新成为当前事实；
- 四维相同：A 把确认依据从 P2 拉回 P1，下一封 P2 重投时又被当成新投影处理。

两种情形下乐观锁都会放行：A 读到的当前版就是 B 刚写的那一版，`Save` 与 `Reaffirm` 的条件对它成立。

## 证据

- 文档：VE CONTEXT「客户可见性与通知」：「原已发布版本保留，但不再有效的内容不得继续显示为当前事实。」同节：「客户全程追踪视图只基于当前有效的全程追踪投影……形成」。
- 代码：
  - `adapters/parcelshipment` 的 `DeriveCustomerViewOnProjectionAdapter.HandleDerivedTrackingProjection`：用 `projections.FindCurrent` 读当前投影，比 `current.Version() != announced` 判旧信封；这一读不加锁，也不在写视图之前再核一次。
  - `adapters/postgres` 的 `Projections.FindCurrent`：普通 `SELECT`，不对 `tracking_projection_current` 那一行加锁。
  - `application.DeriveCustomerViewHandler.Handle`：读当前视图、算四维、写，乐观锁只看视图版本与确认依据，不看投影当前版。
  - 消费事务在本仓未见另设隔离级别，按库默认（READ COMMITTED）：同一事务里两次读之间看得见别人刚提交的行。
- 归属确立补派生 `DeriveCustomerViewOnAcceptanceAdapter` 也按 `projections.FindCurrent` 读当前投影再派生，同形。

## 三类

机制。无产品策略、无租户取值。

## 规模

S–M。

## 最小的诚实修法

二选一，作者裁：

1. 消费事务里读当前投影时，对 `tracking_projection_current` 那一行加 `FOR SHARE`：P2 的投影派生要改这一行就得等 A 提交，A 判新旧时看到的当前版在它的事务里不会变。读口要分出「锁读」一支，只给消费链用；运营查阅读面不加锁。
2. 写视图之前再核一次投影当前版：仍是信封宣告的那一版才写，否则按旧信封入账收工。

两条都要补真库并发用例：两个事务交错处理同一包裹的 P1 与 P2，断言最终当前视图依据 P2、内容是 P2 的。

## 要裁的

- 两种修法取哪一种。前者把投影派生与视图派生串行化到同一行锁上，后者多一次读、窗口缩到最后一读与写之间（还要配合视图写入条件才算闭合）。

## 完成记录（2026-10-21，分支 claude-code50，基于 main `be6b5ad20`）

用户授权原话「你作为业务和系统专家自决……解决所有的问题」；下列「代决」由作者会话代 VE owner 作出，待 VE owner 复核。

### 做了什么

- `ac690959c`：
  - `vepostgres.Projections.LockCurrent`：按（租户，包裹）对 `tracking_projection_current` 那一行 `FOR SHARE`，再按它指名的版本取版本行；走 `RequireExecutor`，锁持到环境事务结束。`FindCurrent`、`ListCurrent`、`FindByVersion` 不变，运营查阅照旧不加锁。
  - `adapters/parcelshipment` 的 `CurrentProjectionView` 方法改为 `LockCurrent`；`DeriveCustomerViewOnProjectionAdapter` 与 `DeriveCustomerViewOnAcceptanceAdapter` 都改走它。生产装配不用改：两处传的都是 `vepostgres.Projections`，方法改名之后只能接锁读，接错在编译期就过不去。
  - 测试（先写、确认红，再实现）：
    - VE 包真库屏障用例 `adapters/parcelshipment/derive_customer_view_concurrency_postgres_test.go`：P1 那一路停在读完投影、进派生编排之前；P2 的投影派生与它那封信在另一条连接上跑，要么整段提交，要么被看见等在锁上，才放 P1。`TestAStaleProjectionEnvelopeCannotCarryTheConfirmedBasisBack`（四维不变：确认依据仍是 P2，不多一版；修前红在确认依据回到 P1）、`TestAStaleProjectionEnvelopeCannotSupersedeTheViewWithStaleContent`（四维变化：当前视图形成依据与内容都是 P2 的，前一版依据 P1；修前红在当前视图依据 P1）。
    - 派发侧经生产装配 `cmd/parcel-dispatch/customer_view_stale_envelope_test.go` 的 `TestAStaleDerivedProjectionEnvelopeCannotCarryTheCustomerViewBack`：P1 那封派生信封交给 `deriveCustomerViewConsumer` 另装的一路，它连同一个库、走一只带 pgx 追踪器屏障的连接池，停在第一次碰客户视图当前标记之前；P2 经生产派发两拍。收工后视图依据 P2、里程碑展示、指回前版、版本行两行。修前红在视图依据 P1。
    - 投影库锁读 `adapters/postgres/projection_lock_test.go`：`TestLockCurrentKeepsTheCurrentVersionUntilTheTransactionEnds`（持锁期间下一版 `Save` 等在标记上，同一事务再读仍是原版，放锁后下一版落库）、`TestLockCurrentWaitsForAnUncommittedMoveAndReadsTheMovedVersion`（撞未提交的移标记时等它提交，读回移过去的那一版连同条目与前版）、`TestLockCurrentAnswersNotFoundWithoutACurrentProjection`（另一租户、无投影的包裹答查无）、`TestLockCurrentRefusesToRunOutsideATransaction`（PBC-08 无事务负向证据）。
    - 变异（于 `ac690959c` 的工作树上临时改、跑完还原）：去掉 `FOR SHARE`，新用例除「查无」「无事务」两格外全红；改成连接上的 `FOR SHARE`，「撞未提交的移标记」一格红在查无。
- 文档：UC-VE-008 没有本问题的现状注，未动。VE CONTEXT「客户全程追踪视图只基于当前有效的全程追踪投影……形成」「不再有效的内容不得继续显示为当前事实」已是本票要守的规则，没有新规则要写，未动。无迁移、无 ADR（机制，票面已列为机制）。

### 代决

1. **取票面修法 1（锁读），不取修法 2（写前复核），也不取咨询锁。** 修法 2 只把窗口缩到最后一读与写之间，要闭合还得给视图写口加「投影当前版仍是 X」的条件，等于把投影的当前版搬进视图库的写条件里，一处决定分两处说。咨询锁要投影派生与视图派生两边都取，投影派生那一侧有多路消费，漏接一路就漏一格；行锁落在投影派生本来就要改的那一行上，投影那一侧一行不用改。
2. **锁与读分两条语句。** 一条带连接的 `FOR SHARE` 在读已提交下撞上并发移标记时，只对被锁的标记行取最新版重核，连接进来的版本行仍是原先那一行，整行被滤掉、读成「没有当前投影」，适配器会把它译成 `ErrDerivedProjectionInconsistent`（硬失败）。2026-10-21 在一次性库上用两会话实测过这一格，变异用例也钉住了。
3. **锁读要求环境事务。** 离了事务锁随语句即放，等于一次不加锁的读；拒绝比静默退化好。无事务那一格的错误经适配器落 `ErrDerivedProjectionUnreadable`（未决哨兵），与本链上视图写口无事务时落 `CUSTOMER_VIEW_STORE_UNAVAILABLE` 同格；生产上两路都由消费门开事务，这一格走不到。未决哨兵名单没有加东西。
4. **归属确立补派生同样走锁读。** 它按当前投影形成视图，同一个窗口同样存在（补派生按 P1 写回时，P2 那一路已把视图带到 P2）。
5. **`FOR SHARE` 而不是 `FOR KEY SHARE`。** 移标记只改 `version_id`、不改主键，取的是 `FOR NO KEY UPDATE`，与 `FOR KEY SHARE` 不冲突、拦不住它；`FOR SHARE` 才拦得住。

### 越权风险点（待 VE owner 复核）

1. **同一包裹的投影派生排在客户视图派生之后。** 视图派生事务（含账户反查、披露策略读、视图写、意图入队）持锁期间，这件包裹的下一版投影等着；常态下这一段很短。归属确立补派生持的是一份委托全部有投影成员的锁，持到整封提交，成员多时等得更久。
2. **死锁（今天走得到，会自愈）。** 一笔投影派生可在同一事务里改多件包裹的标记（例如集运单元的移动事实按成员派生），归属确立补派生也按成员逐件取锁；两边次序相反时互等，PostgreSQL 约 1s 后以 40P01 中止其中一路、整笔回滚，重投即自愈。两边按（租户，包裹引用）排序取锁的修法与投影之间同形的死锁，另立 [code/83](./83-multi-parcel-projection-derivation-and-acceptance-rederive-lock-markers-in-opposite-orders.md)（R3）。
3. **失败码分不出死锁。** 视图派生这一侧，锁读失败与投影库读不回同落 `ErrDerivedProjectionUnreadable`，失败码相同，日志正文里带着原错误的 SQLSTATE 40P01；投影派生那一侧 `DeriveProjectionHandler` 在 `Save` 失败时只答 `ProjectionStoreUnavailable`，连正文也丢了（code/83 附带收）。锁等超时今天走不到（全仓生产代码没设 `lock_timeout`），将来设了 `lock_timeout` 时落同一格。
4. **视图写口仍只看视图版本与确认依据。** 绕过这两路消费、直接调派生编排的调用方不受这把锁保护；今天生产上没有这样的调用方（派生编排只在派发进程装配，经这两路进来）。
5. 存量：首发前只有演示租户；已经被旧信封带回的视图不会自愈，要等这件包裹的下一份投影派生，或 `--reset` 重灌。

### 验证（分支 claude-code50）

- `go build ./...`、`go vet ./...`、`gofmt -l cmd internal migrations tools` 干净。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/visibilityexception/... ./cmd/parcel-dispatch/ ./cmd/parcel-api/ ./cmd/parcel-ve-register/ ./internal/architecture/... ./migrations/...`：20 个包 ok，退出码 0。
- 新用例 `-count=3` 与 `-race` 各一跑，涉及的三个包 ok。

## 评审记录

### 第 1 轮（2026-10-21，隔离评审；协调方转达，无阻断）

评审实测确认锁级别、两条语句之间不留窗口与 EPQ 的论证成立。

| 条 | 意见 | 处理 |
|---|---|---|
| 1 | 死锁另立票 | 立 [code/83](./83-multi-parcel-projection-derivation-and-acceptance-rederive-lock-markers-in-opposite-orders.md)，建议 R3：多成员投影派生几路与补派生按（租户，包裹引用）排序后逐件处理，口径同 ADR-0210；投影之间同形的死锁一并收；附带让 `DeriveProjectionHandler` 保留 `Save` 原错误。越权风险点 2 改指 code/83 |
| 2 | 越权风险点 2、3 不准 | 改写：失败码分不出；视图派生侧日志正文带 40P01，投影派生侧连正文也丢；锁等超时今天走不到（无 `lock_timeout`），将来设了落同一格 |
| 3 | `TestAStaleProjectionEnvelopeCannotSupersedeTheViewWithStaleContent` 没有前版时静默放过 | 改为没有前版即 Fatal，与 `cmd/parcel-dispatch` 那条一致 |
| 4 | `LockCurrent` 头注措辞；等锁 SQL 三份重复 | 头注改为「走 FindCurrent，不排这把锁」；三份等锁 SQL 记进 [code/81](./81-savepoint-decorators-are-three-copies-of-one-helper.md) 票面，本票不抽 |

验证（修订后）：`go build ./...`、`go vet ./...`、`gofmt -l` 干净；`TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/visibilityexception/... ./cmd/parcel-dispatch/`：16 个包 ok，退出码 0。
