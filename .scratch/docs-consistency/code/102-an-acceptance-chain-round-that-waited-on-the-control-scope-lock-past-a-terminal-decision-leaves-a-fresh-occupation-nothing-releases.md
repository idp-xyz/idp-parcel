# 102 接受链在控制作用域锁上排队、排到时委托已被撤回或拒绝：这一轮新占下的资金没有任何路径去放

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code102（见文末「完成记录」）。此前 needs-triage——R1（首方撤回口上线前必修）。依据（code/99 独立评审对本票的评估，协调方采纳）：①冻结真的锁资金：`FreezeLedger.Freeze`（`internal/settlementaccounting/domain/funds_freeze.go`）用 `balance.Available()` 判够，孤儿 `HELD` 冻结永久扣减同一（法人、账户、币种）作用域的可用余额，信用暴露同构；该客户后续请求可能被判 `AVAILABLE_BALANCE_INSUFFICIENT` 走错因的拒绝。②无运维补救：`cmd` 里没有释放入口，委托已终结所以没有补偿引用，只能手改库。③悄无声息逐次累积，无 TTL。④违反 `AT-PS-070`「撤回先合法提交 → 接受不得再成立，已成立的占用进入释放补偿」。⑤首方撤回口已在 main，窗口今天就开着。窗口比原先认定的多一条，见「现象」窗口（b）。此前 needs-triage——建议 R2（上线后尽快）：接受链一轮读到委托仍为`已提交`之后，在接受前资金控制所在作用域的锁（S）上排队；排到锁之前，撤回、主动拒绝或授权处置的`拒绝`先提交并放掉了当时已成立的占用；链拿到 S 之后照常施加控制、在冻结账本上占下一笔新的，随后形成决定那一步读到委托已决，答「已决定」收工并提交。这一笔占用挂在一份已撤回（已拒绝）的委托上，再没有路径会放它。不损坏账务一致性，但客户的资金或额度被一份已终结的委托占着，直到有人在账本上手工处理。
Blocked by: [code/99](./99-withdrawal-review-and-disposition-take-the-request-row-before-the-control-scope-lock.md) 进 main 之后做（推荐修法用到 `lockScopesBeforeSave`、`releaseEvery`）；单独做，不并入 code/99。本缺陷的时间窗在 code/99 改前改后都存在
出处：code/99 作者为「输的一路不泄漏占用」补反向时序用例时在真库上发现（先让撤回拿到 S 并停在保存之前，再让续办的接受链到达）。

## 现象

**窗口（a）：链在控制作用域锁上排队。** 时序（同一份委托，续办的接受链与撤回类路径并发，S 是（租户，作用域）级，别的委托的控制也会占着它，排队不罕见）：

1. 接受链事务读到委托`已提交`（`AdvanceAcceptanceChainHandler.Handle` 开头的 `FindBySourceIdentity`；此时守卫「已撤回即停」放行）。
2. 撤回类路径事务取到 S、保存委托、释放当时已记录的占用（本版的控制结果还没记，没有东西可放）、提交。
3. 接受链在 `releaseSuperseded` 或施加控制的 `freezeFunds` 取 S 排到锁，施加照常成立：冻结账本新增一笔，控制结果记到判断账。
4. 形成决定那一步重读委托：`FormAcceptanceDecisionHandler.Handle` 见到决定已成立，走 `existing`，答「已决定」，不释放（`existing` 对已决委托一律不释放）。链提交。
5. 结果：委托已撤回，新版本那一笔冻结 `HELD` 挂着；撤回路径的释放已跑完，接受链的守卫只在链开头，以后也不会再有一轮去放它。

真库实测（`cmd/parcel-dispatch`，夹具同 `customer_supplement_pause_release_test.go`；撤回停在领取决定标识那一刻——取到 S 之后、保存之前——再让续办信封驱链，链在 S 上排队，放行撤回）：撤回 `FORMED`，链 `err = nil`，委托 `WITHDRAWN`，新版本冻结 `status = 1`（占着），首版冻结已释放。

**窗口（b）：链已写下冻结（未提交）、Form 还没读委托时撤回提交。** 这一条在 code/99 的评审里补出，撤回根本不等 S：

1. 链事务读到委托`已提交`，守卫放行；施加控制，取到 S（无人持，不排队），在冻结账本写下冻结。链事务还没提交，控制结果与冻结对别的事务都不可见。
2. 撤回类路径事务到达。首版委托没有被取代版本，判断账里也没有已提交的占用，撤回的 `lockScopesBeforeSave` 读回空，不取 S，因此既不取 S、也不阻塞于链手里的 S；撤回的 Save 成功（链还没保存委托行，R 无人持），提交。
3. 撤回的释放读不到链未提交的冻结，什么也不放。
4. 链形成决定那一步重读委托，见到已撤回，走 `existing`，不释放；链提交，冻结落库。

结果同窗口（a）：委托已撤回，新版本那一笔冻结 `HELD` 挂着。对没有被取代版本的首版委托（最常见），这条窗口比窗口（a）更直接。有被取代版本的委托，撤回会读到被取代版本已提交的占用而取其 S（被取代版本与当前版本占的是同一作用域时，排在链的 S 之后），窗口（b）关上。

[ADR-0286](../../../docs/adr/0286-withdrawn-shipment-request-rehydration-by-snapshot-expressiveness.md) 决定六说读到未撤回之后、占用形成之前撤回成立的窗口由 PS 申报视图对已撤回的状态过滤和 code/99「兜住」；这只覆盖估价读之前撤回成立的窗口（估价读不到已撤回委托，冻结形不成），不覆盖估价之后到链提交之前的窗口；code/99 也没关上它。已接受 ADR 正文不改，这一澄清写在本票。

## 证据

- `internal/parcelshipment/application/advance_acceptance_chain.go`：撤回守卫和换代守卫都在链开头读一次委托，之后不再复核；施加控制（`advance_financial_control_judgment.go` 的 `ApplyPreAcceptanceFinancialControl`）和记录控制结果都不看委托此刻的状态。
- `internal/parcelshipment/application/form_acceptance_decision.go`：`Handle` 对已有决定的委托直接 `existing`，其中不释放；撤回路径的 `existing`（`withdraw_shipment_request.go`）明说「撞上拒绝时也不释放——那笔冻结归形成拒绝的那条路径处置」，而这里的占用是那条路径放完之后才形成的。
- `internal/parcelshipment/application/lock_occupation_scopes.go` 的 `lockScopesBeforeSave`：按各版已记录的判断决定取哪几把 S，读不回或没有成立占用的版本不取；首版委托在链提交前没有已提交的判断，读回空，撤回不取 S（窗口（b））。
- `internal/parcelshipment/application/withdraw_shipment_request.go` 的 `releaseOccupation` / `occupationRelease.releaseEvery`：只放判断账里已成立的占用，读不到链未提交的冻结。
- `internal/settlementaccounting/domain/funds_freeze.go` 的 `FreezeLedger.Freeze`：`request.amountMinor > balance.Available()` 判够，孤儿冻结因此挤占同作用域的可用余额。
- `cmd/` 下没有释放入口：撤回与拒绝的释放只经 PS 编排发出，已终结的委托没有补偿引用可续办。

## 三类

机制。无产品策略、无租户取值。

## 规模

S–M：`FormAcceptanceDecisionHandler.Handle` 的 `existing` 路径加一段，补两条真库并发用例。

## 推荐修法（方案 1）

在 `FormAcceptanceDecisionHandler.Handle` 的 `existing` 路径上（「已有决定」早退，以及 `ErrDecisionAlreadyFormed` 分支）：

1. 若委托状态是已撤回或已拒绝，就按版本形成顺序先取锁（`lockScopesBeforeSave`，在链里是重入），再对各版 `releaseEvery`，经 `settle` 记册。
2. 已接受绝不放。
3. 提供方按幂等答原释放，所以同一份已终结委托被重投信封时再放一次无害。
4. 若放不收口（可重试未形成），不要往下交「已决定」：让这一轮回滚。链是一笔事务，回滚会把刚占下的冻结一起撤回，重投时链开头的撤回守卫就停下（已拒绝没有开头守卫，放不掉时回滚再重投会重复这一轮直到放掉，也无害）。

这样窗口（a）和（b）都关上：撤回在 Form 读之前提交，链自己的事务里就把它放掉；撤回在 Form 读之后才提交，链的 Save 撞修订冲突整轮回滚，也不会有孤儿。

**方案 2（取 S 之后重读委托）不够。** 它覆盖不到窗口（b），因为撤回根本不等 S：重读的那一刻撤回可能还没提交，重读之后、链提交之前撤回才提交，冻结照样成孤儿。

## 要裁的

评审已定：选方案 1；已撤回与已拒绝（非本轮形成）都放；放不收口让这一轮回滚。

## 复现提示

`cmd/parcel-dispatch/control_scope_lock_order_test.go`（code/99 新增）里有真链夹具与屏障（`synRGatedControl`）。反向用例需要再加一只在 `NextAcceptanceDecisionID` 停住的决定标识替身（撤回类路径领标识在取到 S 之后、保存之前），让撤回停在「手里有 S、没有委托行」，再启动 `supplementGate.Consume` 让链排队，放行撤回，读冻结账本。

## 用例建议

沿用 `synRGatedControl`：

1. 补一例首版（无已提交占用）的窗口（b）：链在施加之后停住（冻结已写下、链事务未提交），撤回提交，放行链，断言冻结已释放、委托`已撤回`。
2. 再补一例排队窗口（窗口（a），即本票原先的场景）：撤回拿到 S 停在保存之前，链排队，放行撤回，断言冻结已释放。

## 完成记录

分支 `claude-code102`，基于 main `8c361529d`（收尾 rebase 之后以回报里的最终 SHA 为准）。

### 做了什么

- `FormAcceptanceDecisionHandler.existing`（「已有决定」早退与 `Decide` 的 `ErrDecisionAlreadyFormed` 分支共用）：委托状态是已撤回或已拒绝（不是本轮形成）时，交回之前先 `lockScopesBeforeSave`（在链里是重入）、再 `releaseEvery` 逐版释放、经收口落格；已接受绝不放。收口拆出 `occupationRelease.settleGap`（`settle` 只是它加派生引用），因为这一路要拿缺口原因当未决原因。
- 三格落点：已收口交`已决定`；缺口开着（可重试未形成，含取不到锁、读不回某一版判断、拒收记不下册）交回未决、原因取缺口自带的、续办引用用 `scope.reference(gap)`，恢复动作是内部重试，消费门回滚重投；确定未受理（含口形不成三格之一）记进释放未受理册、`ControlReleaseNotAccepted()` 为真、不回滚、不留补偿引用。
- 已撤回的委托在 `Handle` 入口单独认出走同一支：撤回不写接受或拒绝决定，`AcceptanceDecision()` 对它报告缺席，不认出来要先走完依据重校验、耗一个决定标识才在 `Decide` 撞上边界。
- 装配不动：`cmd/parcel-dispatch/assemble.go` 的 `FormAcceptanceDecisionDeps` 早已接了 `Release`（`SavepointedControlRelease`）、`Refusals`、`ScopeLock`。
- 文档：新 ADR-0290；ADR-0282 与 ADR-0286 的 Status 注记与 Links 加前向指针；UC-PS-005 规则一条、`AT-PS-153`、`AT-PS-154`、`AT-PS-155` 三行、交接一行；UC-PS-001 交接一行；PS CONTEXT 词条「控制项结果」与撤回一条各一句（规则句不挂票号）；`ports.PreAcceptanceControlScopeLock` 头注加本路径；盘点三行入①、`AT-PS-070` 补用例名、一段追记。无迁移。

### 锁序论证

本路径只取 S、不保存委托，不持 R，取 S 的次序是版本形成顺序（被取代各版在前、当前版本在后，同作用域一次），即接受链开头放被取代各版、再施加当前版本、和撤回类路径保存之前取锁的同一次序。链里这些 S 在到达这一步之前都已持有（放被取代各版与施加控制各取过），这里是重入，不等待；单驱这一步时无 R，也不成环。链里这一步之前不持 R（链在形成决定处才保存委托），所以不存在「持 R 再取一把还没持有的 S」的边。两个窗口的真库用例都带 `pg_locks` 轮询与期限，无死锁、无超时。

### 两个窗口的修前红与修后绿

用例在 `cmd/parcel-dispatch/terminal_request_orphan_occupation_test.go`（真链、生产 PS→SA 适配器与两本账、余额恰好只够冻一笔、终局路径是生产编排，只有授权口是替身），撤回与主动拒绝各证一遍（`Form` 只看委托状态，授权处置的`拒绝`去向落在同一支）。修前：把 `form_acceptance_decision.go` 换回基线版本跑同一批用例，`cmd/parcel-dispatch` 这些全红：

- 窗口（b）`TestATerminalDecisionCommittedBeforeTheChainReadsTheRequestReleasesTheChainsFreshFreeze`（撤回、拒绝各一格）：终局路径先提交、什么也没放，链提交之后首版冻结 `status = 1` 占着——孤儿复现。
- 窗口（a）`TestAChainQueuedOnTheScopeLockPastATerminalDecisionReleasesItsFreshFreeze`（撤回停在领决定标识处、链在作用域锁上排队、放行后链排到锁）：新版本冻结 `status = 1`——孤儿复现；链 `err = nil`。
- 放不收口 `TestAChainThatCannotReleaseItsFreshFreezeRollsTheRoundBack`：链返回 nil、照常提交（修前不回滚）；拒收 `TestAChainWhoseReleaseOfItsFreshFreezeIsRefusedRegistersItAndCommits`：册上空；记不下册 `TestAChainWhoseRefusalCannotBeRegisteredRollsTheRoundBack`：链返回 nil。
- 读之后才提交 `TestATerminalDecisionCommittedAfterTheChainReadTheRequestRollsTheChainBack`：撤回一格修前即绿（保存撞修订、重投被守卫挡住），拒绝一格修前红——重投重复这一轮，孤儿落库。
- `TestAnAcceptedRequestsChainRedrivenNeverReleasesItsFreeze`（不回归）修前后都绿。

修后全绿：窗口（a）（b）冻结 `status = 2` 且有释放时刻；放不收口链返回 `ErrAcceptanceChainUndecided`（含 `CONTROL_RELEASE_PENDING`）、冻结随回滚而没有、inbox 不入账、重投后已撤回的不再占资金，已拒绝的这回放掉；拒收册上一行、冻结原样占着、这一轮提交；记不下册（册上挂触发器让写入报 SQL 错误）回滚、册上空。应用层 `application/form_acceptance_decision_terminal_test.go` 十余例覆盖同一批格与取锁先于释放、多版次序、读不回判断、取不到锁、没占下不发释放不取锁。

### 测试与变异

- `scripts/test-full.sh` 范围：`./internal/parcelshipment/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./internal/architecture/...`、`./migrations/...`、`./internal/settlementaccounting/adapters/parcelshipment/...`、`./tests/...`，全部 ok、退 0；`go build ./...`、`go vet ./...`、`gofmt -l` 干净；`go run ./tools/adr-index` 能生成（README 不提交）、`go run ./tools/at-coverage-counts -check` 退 1（只是总览过期，新号分桶行已补）。
- 新用例 `-count=3`（应用层）、`-count=5`（并发的真库用例）稳定。
- 变异（备份、还原后 `cmp` 一致），全部被杀：①去掉形成决定读到终态委托时的释放（应用层十余例红，真库两窗口红）；②放宽「已接受绝不放」，状态不是`已提交`就放（应用层已接受格与真库已接受重驱一拍红）；③放不收口时不回滚、照交`已决定`（应用层与真库回滚用例红）；④确定未受理时回滚而不是记册（应用层与真库拒收用例红）；⑤去掉释放前的取锁（应用层取锁先于释放、取不到锁红；真库看不见，因为链里是重入）；⑥已撤回不在入口单独认出（应用层不耗标识、不先重校验红）。

### 代决

1. **立 ADR-0290。** 核 ADR-0286 决定六「兜住」与 ADR-0282 决定二至四：决定二列了遇确定未受理照常成立并记册的诸路，接受链形成决定处读到终结委托不在其中，加这一路是扩成「适用的路」一句；该路可重试未形成回滚而不留补偿引用，与决定二「`可重试未形成`照旧走 `ControlReleasePending`、留补偿续办引用」相抵，是收窄；ADR-0286 决定六的「兜住」只覆盖估价读之前撤回成立的窗口，按 ADR-0290 Context 读，是读法限定。决定三（链开头放被取代各版遇拒收响亮上抛）不动：那一步新版本还等着被判，这一步委托已终结。
2. **拒收记不下册也回滚。** 派单写的是确定未受理记册、不回滚；册写不进时那一笔既没放掉也没交到人手里，提交这一轮就是留下一笔谁也不知道的孤儿，所以这一格并入「放不收口」回滚。重投：已撤回的被守卫挡住，已拒绝的重复这一轮。
3. **未决原因取缺口自带的**（`ControlReleasePending`、`RecordedJudgmentsUnavailable`、`ControlReleaseRefusalNotRecorded`），取不到锁并入 `ControlReleasePending`。不新增原因取值，消费门、HTTP、cmd、管理台、跨上下文读方没有新落点（ADR-0025）；`ControlReleaseRefusalNotRecorded` 的注释从「只出现在补偿引用里」改准。
4. **释放先于重发决定意图**；回滚的一轮不发意图。已撤回无决定，本不发；已拒绝回滚时这一轮的意图随事务撤回。
5. **已撤回在入口认出，而不是留在 `Decide` 的 `ErrDecisionAlreadyFormed` 分支**：那一分支保留（走同一个 `existing`），但对已撤回委托现在够不到。理由见上；ADR-0290 越权风险点 5。
6. 授权处置的`拒绝`去向不单独加窗口用例：`Form` 只认委托状态，已拒绝的委托无论由自动拒绝、主动拒绝还是处置形成，进的是同一支，用主动拒绝代表。
7. 夹具：`customerSupplementLoopLegs` 加 `identities`（形成决定领标识口，缺省不变），`withdrawal` 拆出 `withdrawalWith(identities)`；既有用例行为不变。
8. 不做 `cmd/parcel-dispatch` 链装配的行为测试（code/99 越权风险点已记同一类缺口，不在本票边界）。

### 越权风险点（待 PS owner 复核）

1. **确定未受理那一笔随这一轮提交落库。** 链提交时这一轮新占下的冻结仍是`HELD`，册上一行待人处理；册上的积压今天没有队列与告警（ADR-0282 越权风险点 1）。另一种取舍是确定未受理也回滚，把这一轮的冻结撤回；没选，理由见 ADR-0290 候选。
2. **已拒绝的委托放不收口会让整轮重复**，重新施加控制再放，直到放掉或失败预算耗尽、信封死信；提供方长时间答不出时会占失败预算。已撤回的不受影响。
3. **「读之后才提交」那一半靠仓储的修订闸门与读已提交隔离。** 真库用例证了它；换更强的隔离级别或换仓储要重证。
4. **链装配的 `ScopeLock`、`Refusals`、`Release` 没有行为测试**，漏接时终结委托上的这一步恒回滚（取锁口没接上落 `ControlReleasePending`），响亮但不在装配处响亮。
5. **S 与 S 之间没有先验全序**（code/99 越权风险点 2，未处理）。
6. **已撤回的委托在 `Form` 入口认出**：日后新增撤回类状态要同步认出。

### 评审提示

重点看三处：`existing` 的三格落点与「拒收记不下册也回滚」的取舍；ADR-0290 对 ADR-0282 决定二两句的部分停用写法；窗口（a）用例里终局路径停在领决定标识处、链在放被取代版本那一步排在锁上的时序是否就是生产里那一幕。
