# 99 撤回、主动拒绝、授权处置先锁委托行再取控制作用域锁，与接受链次序相反：同一委托并发可死锁

Category: bug
Status: needs-triage——建议 R2（上线后尽快）：同一份已提交委托上，一笔续办的接受链与一笔客户撤回（或主动拒绝、授权处置）并发时两笔互等，PostgreSQL 的死锁检测中止其中一笔、整笔回滚。不损坏数据：消费门会重投那封信封，撤回一侧答未形成、客户重发即可。窗口窄，但已装配在生产上。2026-10-26 协调方据票 [code/77](./77-cancellation-intake-adoption-and-label-final-are-not-serialized-per-parcel.md) 隔离评审（信息 3）立；code/70 越权风险点、code/71 越权风险点里各提过一半。
Blocked by: 无
出处：code/77 隔离评审信息 3（Sonnet 评审，读码取证，未在真库复现）。

## 现象

两条路径对同一份委托取同两把锁，次序相反：

- 撤回、主动拒绝、授权处置：先保存委托（对 `parcel_shipment.shipment_request` 那一行 UPDATE，持行锁 R），再释放受理前控制——`ReleasePreAcceptanceControlHandler.Handle` 无条件先取控制作用域咨询锁 S（`LockControlScope`）。
- 接受链：先走财务控制（`freezeFunds` 先取 S），再在形成受理决定时保存委托（R）。

续办的接受链事务持 S 等 R，客户撤回事务持 R 等 S，两笔互等。S 是（租户，作用域）级，必须是同一份委托才成环。

## 证据

- `internal/parcelshipment/application/withdraw_shipment_request.go`：`Requests.Save` 在 `releaseOccupation` 之前。主动拒绝、授权处置同形（`cmd/parcel-api` 的 `assemble_review.go`、`assemble_disposition.go` 同样装了作用域锁）。
- `internal/parcelshipment/application/advance_acceptance_chain.go`：财务控制（取 S）在 `form_acceptance_decision.go` 的 `Requests.Save` 之前。
- 复现条件：委托状态为已提交，财务控制已施加过、存有作用域认领（例如接受链曾停在人工复核或可达性重校后续办），续办的接受链与客户撤回同时到达。

## 三类

机制。无产品策略、无租户取值。

## 规模

S–M：调整一侧的取锁次序，补真库并发用例。

## 最小的诚实修法

二选一，落地时定：

1. 撤回、主动拒绝、授权处置在保存委托之前先取 S（与接受链同序：S → R）。
2. 接受链在取 S 之前先锁住委托行（R → S），撤回一侧不动。

补一条真库并发用例：同一份委托，一笔接受链续办停在取到 S 之后，另一笔撤回要么等在 S 上、要么已提交，断言不出现死锁中止。

## 要裁的

- 选哪一侧改次序。倾向第 1 条：撤回类是低频的人工操作，提前取 S 的代价小；接受链是信封驱动的主路径，不宜再加一把行锁。
