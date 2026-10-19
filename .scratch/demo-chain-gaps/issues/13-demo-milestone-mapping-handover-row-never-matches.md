# 13 演示里程碑映射的交接那一行永远匹配不上，`IN_TRANSIT` 从不归类

Category: bug
Status: resolved——2026-10-19 进 main（见文末「完成记录」）。此前 needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [11](./11-external-result-received-has-no-subscriber.md) 的作者核对。
Blocked by: 无
出处：[11](./11-external-result-received-has-no-subscriber.md) 完成记录。

## 现象

演示里程碑映射 `SYN-VE-MAP-V1` 登了 `TRANSPORT_FULFILLMENT` / `transport-handover` → `IN_TRANSIT`，但 VE 的交接投影交出的事实类型词是 `handover-handed-over`、`handover-refused`、`handover-pending-confirmation`，从来没有 `transport-handover`。交接之后的投影条目因此一律未归类。

## 证据（读码）

`scripts/demo-seeds/data/visibility/01-milestone-mapping-v1.json` 的交接一行；`internal/visibilityexception/adapters/transportfulfillment/derive_on_transport_handover.go` 按裁决给类型词。

## 三类

租户取值（演示租户的映射行）。规模 S。

## 最小的诚实修法

演示映射只把 `handover-handed-over` 映到 `IN_TRANSIT`；拒收与待确认不映。就地改 v1 并在种子 README 写明已灌的库要 `--reset`（同 11 的做法），或登 v2 并核对映射目录的版本选择不撞适用冲突。加一条用例读真种子文件，钉住交接已交出的条目归 `IN_TRANSIT`。

## 要裁的

none

## 既有票

none

## 完成记录

2026-10-19 Claude Code 会话做。

- 演示映射 v1 就地改：交接那一行的类型词改为 `handover-handed-over`；拒收与待确认不映，照未归类。已灌过的库要 `--reset`，种子 README 那一句同步。
- 新用例 `TestAHandedOverTransportHandoverClassifiesAsInTransitUnderTheDemoMapping`（`cmd/parcel-dispatch`，真库）：经生产翻译与登记用例登真种子文件，一封已交出的交接经生产路由表一拍，归 `IN_TRANSIT`。把种子换回旧行，这条转红（实测）。
- 证据级别 `S`。
