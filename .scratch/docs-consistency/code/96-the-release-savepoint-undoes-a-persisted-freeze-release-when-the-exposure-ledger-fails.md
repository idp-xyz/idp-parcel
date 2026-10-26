# 96 释放口外那层 savepoint 在暴露账本读不回时连已落库的冻结释放一起撤掉

Category: bug
Status: ready-for-agent——R2（上线后尽快）：组合策略（预付冻结与信用校验同在）下，冻结那本已落库的释放会因为暴露那本读不回被一并撤回，与 ADR-0122 决定三相反；终局各路的补偿引用不持久、没人重放，那一笔冻结就一直占着。只在两本账都有占用、且暴露那本恰好读不回时出现。2026-10-26 协调方据 code/71 隔离评审信息 3 立（用户「你作为业务和系统专家自决」授权）；问题由 code/72 带进来，code/71 未改其行为。
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
