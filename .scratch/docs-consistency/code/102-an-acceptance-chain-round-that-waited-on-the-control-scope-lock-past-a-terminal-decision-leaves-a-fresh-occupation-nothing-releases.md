# 102 接受链在控制作用域锁上排队、排到时委托已被撤回或拒绝：这一轮新占下的资金没有任何路径去放

Category: bug
Status: needs-triage——R1（首方撤回口上线前必修）。依据（code/99 独立评审对本票的评估，协调方采纳）：①冻结真的锁资金：`FreezeLedger.Freeze`（`internal/settlementaccounting/domain/funds_freeze.go`）用 `balance.Available()` 判够，孤儿 `HELD` 冻结永久扣减同一（法人、账户、币种）作用域的可用余额，信用暴露同构；该客户后续请求可能被判 `AVAILABLE_BALANCE_INSUFFICIENT` 走错因的拒绝。②无运维补救：`cmd` 里没有释放入口，委托已终结所以没有补偿引用，只能手改库。③悄无声息逐次累积，无 TTL。④违反 `AT-PS-070`「撤回先合法提交 → 接受不得再成立，已成立的占用进入释放补偿」。⑤首方撤回口已在 main，窗口今天就开着。窗口比原先认定的多一条，见「现象」窗口（b）。此前 needs-triage——建议 R2（上线后尽快）：接受链一轮读到委托仍为`已提交`之后，在接受前资金控制所在作用域的锁（S）上排队；排到锁之前，撤回、主动拒绝或授权处置的`拒绝`先提交并放掉了当时已成立的占用；链拿到 S 之后照常施加控制、在冻结账本上占下一笔新的，随后形成决定那一步读到委托已决，答「已决定」收工并提交。这一笔占用挂在一份已撤回（已拒绝）的委托上，再没有路径会放它。不损坏账务一致性，但客户的资金或额度被一份已终结的委托占着，直到有人在账本上手工处理。
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
