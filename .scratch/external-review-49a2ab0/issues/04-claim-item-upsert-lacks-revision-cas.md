# claim_item 整行 UPSERT 无修订守卫，并发转换可互相覆盖

Category: bug
Status: resolved

来自外部评估（基线 `49a2ab0`），协调岗已核实。评估同条里「先 UPSERT 索赔再追加期限
历史，可能提交不完整状态」的前半**不成立**：`Claims.Save` 走 `RequireExecutor`，bento
严格模式下 context 无事务句柄直接报 `ErrTransactionRequired`、不回退连接池；两条语句
同处一个 PG 事务，语句失败即毒化事务、提交变回滚。本票只管后半的丢更新。

## 现象

`Claims.Save`（`internal/visibilityexception/adapters/postgres`，落
`visibility_exception.claim_item`）做 `ON CONFLICT (tenant_id, batch_ref, item_id)
DO UPDATE` 全列重写，无修订/版本条件列。

两个并发事务（如资格审核与撤回、复核与延期）各自 load → 领域校验 → Save：READ
COMMITTED 下两边都能提交，后写者的快照整行覆盖前写者的转换——前一个转换丢失，且
两边都以为自己成功。领域门只在各自加载的旧快照上校验过，拦不住这种交错。

## 已有先例与形状

- 恢复事项那侧已做过并发保护（081701 评审 #5 那张票，「VE 索赔恢复并发保护」）。
- bento repository 合同有 `Update(ctx, key, aggregate, expectedRevision)` 的 CAS 形状：
  单条条件 SQL 完成检查、递增、返回，不先读后写。

## 修法方向

1. `claim_item` 增修订列；迁移**新增**，不改写既有迁移。
2. `Save` 改条件更新：期望修订不符时交回具名冲突哨兵。
3. 应用层把冲突译成明确结果（如实报并发冲突或按幂等语义重读重试），不得静默吞掉，
   也不得折成 `UNDECIDED`——冲突不是「等依赖」。
4. 测试钉住：并发两转换只有一个生效，另一个得到冲突结果。

## 缓解现状

`/claims` 端点的编排未接线（`cmd/parcel-api` 的 `unwiredClaims`），该路径今天无生产
调用方——潜伏缺陷，非当前运行风险；接线前修完即可。

## Comments

- 2026-08-20 MCP-1：外部评估四项可操作发现之一（其第 3 条后半），核实属实后立票。
- 2026-08-20 MCP-1：派 MCP-4，基线 `a097d7f`（非票头的审计基线 `49a2ab0`）。VE 下一个迁移号是
  `0017`（`a097d7f` 上最大为 `0016_tracking_projection_versions.sql`）。与 MCP-2 的 02 号票
  文件不重叠，可并行。
- 2026-08-20 MCP-4：完工。分支 `claim-item-revision-cas`，提交 **`c61ab33`**（基线 `a097d7f`，
  10 文件 +452/−49）。未推 main，待协调岗集成。
- 2026-08-20 MCP-1：已合入，**main 上的 SHA 是 `0f05304`**（不是 `c61ab33`）。原提交基线 `a097d7f`
  期间 02 号票已推进 main 到 `093d53c`，两者为兄弟提交，协调岗在 detached 验证树上 cherry-pick，
  无冲突——04 只碰 `internal/visibilityexception` 与 `migrations/visibility_exception`，02 只碰
  `cmd/parcel-dispatch`，文件不重叠。内容逐字相同已证：`git diff c61ab33 0f05304` 的全部差异
  只有 02 那两个文件，限定到本票路径时为空。迁移号 `0017` 在新基线上仍空（`0016` 为最大）。
  门禁独立复跑而非沿用：gofmt / go build / go vet / `git diff --check` 全过；全仓
  `go test -p 1 -count=1 ./...` 绿，**含 PG**（3 分 19 秒，68 包 ok、零 FAIL）。真库在场经三重取证：
  `docker compose ps` 见 `127.0.0.1:55432->5432/tcp`、并发用例经 `-v` 复核全 `PASS` 无 `SKIP`
  （含单独复核 `TestAMatchingRevisionStillCannotTruncateHistory`）、`docker inspect` 显示容器
  `RestartCount=0` 全程未重启。
- 2026-08-20 MCP-1：**改既有用例断言口径这一项已单独复核，判为不削弱**。两个用例的实质断言
  （已批延期不被旧快照擦掉、赢家 `ClaimSaved` 且库内状态正确）原样保留，只是落后写入现在先撞
  修订守卫因而拿到 `ClaimRevisionConflict`；`TestCompetingClaimWritersSerializeOnTheClaimRow`
  反而更强了——它新增断言输家事务**不报 error**，把「冲突是业务答案不是故障」钉住。
  `ErrClaimHistoryStale` 未失去覆盖。`TestConcurrentClaimTransitionsLetOnlyOneWin` 另断言修订
  推进一版，挡掉「守卫恒真」那种能骗过冲突断言的实现。三判入口共用 `recordClaim` 而不是各写
  一遍，避免日后分叉方向恰好是把冲突并回未决。`Save` 的 `switch` 留了 `default` 分支报错，
  将来新增写入代数不会静默穿过。

## 完成

四条修法方向逐条落地：

1. 迁移 `0017_claim_item_revision.sql` 新增 `revision bigint NOT NULL DEFAULT 1` 与
   `claim_item_revision_positive` CHECK，未改写 `0004`。默认值保留：`0004` 那批三判形状
   CHECK 的负向证据都是不带本列的裸 INSERT，去掉默认会先撞 NOT NULL 而不再证得出那些 CHECK。
2. `Claims.Save` 改条件更新——检查、递增、回报由 `ON CONFLICT ... DO UPDATE ... WHERE` 一条
   语句完成，不先读后写；命中零行交回具名哨兵 `ports.ClaimRevisionConflict`，事务保持可用。
   期望修订由 `ClaimItem.Revision()` 携带，三判转移一律不动它。`RehydrateClaimItem` 只收
   `≥ 1`，零修订的伪造快照进不来。
3. 编排把冲突译成自占一格的 `ClaimConcurrentlyChanged`，不静默吞也不折进未决；三判入口共用
   `recordClaim`。受理侧按幂等语义读回赢家答 `ClaimExistingResult`。结论与复核撞冲突时不交
   结算意图（否则下游会按一份没落库的结论算钱）。
4. 测试：`TestConcurrentClaimTransitionsLetOnlyOneWin`（资格审核 vs 撤回，赢家生效、输家得
   `ClaimRevisionConflict`、修订推进一版）、`TestAConcurrentChangeIsItsOwnAnswerNotUndecided`
   （三判各步的译法与「不交结算意图」）、`TestAReceiptLosingTheCreateRaceAnswersWithTheWinner`、
   `TestRehydrationRefusesASnapshotThatWasNeverPersisted`。

顺带的两点，均已验证：

- 既有的 `TestAStaleClaimSnapshotCannotEraseAnApprovedExtension` 与
  `TestCompetingClaimWritersSerializeOnTheClaimRow` 改判口径：落后写入现在先撞修订守卫，拿到
  的是业务答案 `ClaimRevisionConflict` 而非 `ErrClaimHistoryStale`。两个用例护的东西（已批延期
  不被旧快照擦掉、并发写入在行锁上排队）不变。
- `ErrClaimHistoryStale` 保留，并补 `TestAMatchingRevisionStillCannotTruncateHistory` 让它不致
  失去覆盖——它守的是修订对得上而历史对不上的手工快照，与修订守卫拦的不是同一件事。

变异验证（改完即还原）：拆掉 `WHERE claim_item.revision = $21` 后三个并发用例全红，其中
`TestConcurrentClaimTransitionsLetOnlyOneWin` 拿到 `ClaimSaved`——撤回确实会盖掉资格审核；另两个
退回 `ErrClaimHistoryStale`，说明旧的历史守卫只护得住带补充期限的索赔，护不住资格/撤回这类转换。
拆掉 `snapshot.Revision < 1` 后 `TestRehydrationRefusesASnapshotThatWasNeverPersisted` 转红。

门禁（**含 PG**，`127.0.0.1:55432`，`docker compose ps` 见 `127.0.0.1:55432->5432`）：
`gofmt -l` 空、`go build ./...`、`go vet ./...`、`git diff --check`、
`go test -p 1 -count=1 ./...` 全绿，约 3.2 分钟。
