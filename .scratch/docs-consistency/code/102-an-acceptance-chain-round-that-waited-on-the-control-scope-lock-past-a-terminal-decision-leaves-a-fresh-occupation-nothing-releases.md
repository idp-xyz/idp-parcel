# 102 接受链在控制作用域锁上排队、排到时委托已被撤回或拒绝：这一轮新占下的资金没有任何路径去放

Category: bug
Status: needs-triage——建议 R2（上线后尽快）：接受链一轮读到委托仍为`已提交`之后，在接受前资金控制所在作用域的锁（S）上排队；排到锁之前，撤回、主动拒绝或授权处置的`拒绝`先提交并放掉了当时已成立的占用；链拿到 S 之后照常施加控制、在冻结账本上占下一笔新的，随后形成决定那一步读到委托已决，答「已决定」收工并提交。这一笔占用挂在一份已撤回（已拒绝）的委托上，再没有路径会放它。不损坏账务一致性，但客户的资金或额度被一份已终结的委托占着，直到有人在账本上手工处理。
Blocked by: 无（与 [code/99](./99-withdrawal-review-and-disposition-take-the-request-row-before-the-control-scope-lock.md) 无依赖；code/99 改的是取锁次序，本缺陷的时间窗在改前改后都存在）
出处：code/99 作者为「输的一路不泄漏占用」补反向时序用例时在真库上发现（先让撤回拿到 S 并停在保存之前，再让续办的接受链到达）。

## 现象

时序（同一份委托，续办的接受链与撤回类路径并发，S 是（租户，作用域）级，别的委托的控制也会占着它，排队不罕见）：

1. 接受链事务读到委托`已提交`（`AdvanceAcceptanceChainHandler.Handle` 开头的 `FindBySourceIdentity`；此时守卫「已撤回即停」放行）。
2. 撤回类路径事务取到 S、保存委托、释放当时已记录的占用（本版的控制结果还没记，没有东西可放）、提交。
3. 接受链在 `releaseSuperseded` 或施加控制的 `freezeFunds` 取 S 排到锁，施加照常成立：冻结账本新增一笔，控制结果记到判断账。
4. 形成决定那一步重读委托：`FormAcceptanceDecisionHandler.Handle` 见到决定已成立，走 `existing`，答「已决定」，不释放（`existing` 对已决委托一律不释放）。链提交。
5. 结果：委托已撤回，新版本那一笔冻结 `HELD` 挂着；撤回路径的释放已跑完，接受链的守卫只在链开头，以后也不会再有一轮去放它。

真库实测（`cmd/parcel-dispatch`，夹具同 `customer_supplement_pause_release_test.go`；撤回停在领取决定标识那一刻——取到 S 之后、保存之前——再让续办信封驱链，链在 S 上排队，放行撤回）：撤回 `FORMED`，链 `err = nil`，委托 `WITHDRAWN`，新版本冻结 `status = 1`（占着），首版冻结已释放。

## 证据

- `internal/parcelshipment/application/advance_acceptance_chain.go`：撤回守卫和换代守卫都在链开头读一次委托，之后不再复核；施加控制（`advance_financial_control_judgment.go` 的 `ApplyPreAcceptanceFinancialControl`）和记录控制结果都不看委托此刻的状态。
- `internal/parcelshipment/application/form_acceptance_decision.go`：`Handle` 对已有决定的委托直接 `existing`，其中不释放；撤回路径的 `existing`（`withdraw_shipment_request.go`）明说「撞上拒绝时也不释放——那笔冻结归形成拒绝的那条路径处置」，而这里的占用是那条路径放完之后才形成的。

## 三类

机制。无产品策略、无租户取值。

## 规模

S–M：二选一，落地时定。

## 最小的诚实修法

1. 链的形成决定那一步在见到委托已撤回（或已拒绝而决定不是本轮形成）时，对命令指名的那一版（及其余各版）再请求一次释放，提供方按幂等答原释放；已接受不放。放不掉走补偿续办或记册，与终局各路同一套。
2. 或让链在施加控制取到 S 之后、记录控制结果之前重读一次委托，见到已决即整轮回滚（交内部重试的未决），重投时链开头的守卫直接停下。需要把「取到 S 之后」这一刻交给 PS 一侧可见——今天 S 取在 SA 的编排里，PS 看不到。

## 要裁的

- 选哪一侧。倾向第 1 条：释放是幂等的、已有一整套终局释放编排（`occupationRelease`），不需要把 SA 的取锁时刻暴露给 PS；第 2 条要改施加控制的端口形状。
- 已拒绝（非本轮形成）是否同样释放：拒绝路径自己的释放在占用形成之前跑完，同样会落下孤儿；倾向同样放。

## 复现提示

`cmd/parcel-dispatch/control_scope_lock_order_test.go`（code/99 新增）里有真链夹具与屏障（`synRGatedControl`）。反向用例需要再加一只在 `NextAcceptanceDecisionID` 停住的决定标识替身（撤回类路径领标识在取到 S 之后、保存之前），让撤回停在「手里有 S、没有委托行」，再启动 `supplementGate.Consume` 让链排队，放行撤回，读冻结账本。
