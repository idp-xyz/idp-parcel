# 73 撤回、拒绝这类终局路径也放掉被取代各版的占用

Category: bug
Status: ready-for-agent——R2（上线后尽快）：撤回、主动拒绝、授权处置拒绝、自动拒绝都只放当前提交版本那一笔。被取代版本那一笔若还没放（取代之后接受链还没有走成一轮，或那一轮回滚了），委托到了终局之后就再没有轮次去放它，成了孤儿。2026-10-21 协调方据 code/20 隔离评审立。
Blocked by: 无
出处：code/20 隔离评审；code/20 完成记录。

## 现象

code/20 把被取代版本的释放放在接受链每一轮的开头（`AdvanceAcceptanceChainHandler.releaseSuperseded`）。委托若在新版本那一轮链提交之前就走到终局，被取代版本那一笔就没有人放了。终局路径的释放只读本版判断：
- `WithdrawShipmentRequestHandler.releaseOccupation`；
- `RejectShipmentRequestHandler` 的释放；
- `DisposeShipmentRequestHandler` 的释放；
- `FormAcceptanceDecisionHandler.releaseIfRejected`。

## 证据

- [UC-PS-005](../../../docs/application/parcel-shipment/UC-PS-005-WITHDRAW-SUBMITTED-SHIPMENT-REQUEST.md) 步骤 6：按原业务关联幂等释放已成立各项的占用。这一句说的是这份委托已成立的占用，不限于当前版本。
- PS CONTEXT 生命周期「已提交期间形成新提交版本」：被取代版本已成立各项的占用按原业务关联释放（code/20 补）。

## 三类

机制。无产品策略、无租户取值。

## 规模

S–M：把 `releaseSuperseded` 的逐版逻辑抽成可复用的一处，终局各路在释放本版之后再遍历 `PriorSubmissionVersions()`；补偿引用按版本各派一条。补用例。

## 最小的诚实修法

1. 逐版释放抽成一个包内函数，入参是判断读口、释放端口、委托与命令范围。接受链与终局各路共用这一处，一决策一处定义。
2. 终局各路在本版释放之后遍历被取代各版，按 `OccupationFormed()` 释放。失败照 `ControlReleasePending` 留补偿，不回滚终局决定（UC-PS-005「撤回提交成功后不等待财务释放才生效」）。
3. 用例：补件之后、新版本那一轮链提交之前撤回，两版的占用都释放；拒绝一路同此。

## 要裁的

无。
