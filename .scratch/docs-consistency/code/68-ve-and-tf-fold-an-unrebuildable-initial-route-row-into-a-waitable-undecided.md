# 68 VE、TF 读初始路由时把重建不出来的坏行折成可等的未决，重投到耗尽

Category: bug
Status: ready-for-agent——R2（上线后尽快）：坏行是确定性错误，重投推不动，整封信封占着分区重投到 ABANDONED；与 code/52、code/60 在 NR 一侧修掉的是同一类病。2026-10-21 协调方据 code/60 隔离评审（N2）立。
Blocked by: 无
出处：code/60 隔离评审；NR 一侧已由 code/52（复核）、code/60（初始路由）让 `ports.ErrStoredRouteRecordUnreadable` 响亮报错。

## 现象

VE 由初始路由派生源事实、TF 由初始路由取计划段，两处读 `FindByKey` 出错时都包成 `fmt.Errorf("%w: %v", ErrInitialRouteNotVisible, err)`。`%v` 丢掉了 `ports.ErrStoredRouteRecordUnreadable` 的错误链，坏行于是进了各自的未决哨兵名单，按「路由还没形成、稍后再看」重投。

## 证据

- `internal/visibilityexception/adapters/networkrouting/derive_on_initial_route.go` 的 `HandleFormedInitialRoute`；`cmd/parcel-dispatch` 的 `veInitialRouteUndecidedSentinels`。
- `internal/transportfulfillment/adapters/networkrouting/planned_leg_source.go` 读 `FindByKey` 那一处。
- NR 端口 `ports.ErrStoredRouteRecordUnreadable` 的注释：坏行与「读不回」分开交，恢复动作是修数据、不等依赖。

## 三类

机制。

## 规模

S：两处各先 `errors.Is(err, ports.ErrStoredRouteRecordUnreadable)`，认出就响亮上抛、不包未决哨兵；各补一条用例（替身交坏行 → 响亮报错，不进未决）。

## 最小的诚实修法

1. VE、TF 两个适配器认出坏行哨兵就上抛（`%w` 保链），其余读不回照旧折未决。
2. 用例：坏行响亮、读不回仍未决。
