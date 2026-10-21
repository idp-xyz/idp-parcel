# 07 VE ETA 存储整行覆盖，键里没有来源维，两种来源的预测互相覆盖

Category: bug
Status: ready-for-agent——R3（不急）：`FormETAHandler` 在 `cmd/` 下没有调用方，今天不上生产路径；但谁来接线都会继承这个错，接线之前必须先修（接线即升 R1：历史预测丢失、来源混淆）。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/ve.md](../audit/ve.md) VE-11（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象

ETA 按（租户，包裹，里程碑）一行，刷新整行改写，历史预测留不下；键里没有来源这一维，承运商预测与运营企业派生预测会互相覆盖。存储形状与 [05](./05-customer-tracking-view-overwrites-published-versions.md) 同属 ADR-0065 否决过的「库只管当前」，但多一层来源维缺失，且未接线、分级不同，单独成票。

## 证据

- 文档：
  - VE CONTEXT「ETA 与可见性缺口」：「承运商提供的预测与运营企业派生的预测分别保留来源和口径。新的 ETA 形成新版本，不覆盖历史预测」。
  - GLOSSARY「预计到达时间（ETA）」：「新的 ETA 不覆盖历史」。
  - UC-VE-003 `AT-VE-051`：「两个来源都提供 ETA | 来源和口径分别保留」；`AT-VE-057`：「原判断保留」。
- 代码：
  - `internal/visibilityexception/adapters/postgres/eta_gap_notification.go` 的 `ETAs.Save`：注释「同（租户+包裹+里程碑）整行更新——刷新不是第二行」，SQL `ON CONFLICT (tenant_id, parcel_ref, milestone_ref) DO UPDATE`。
  - `ports.ETAStore` 注释：「历史版本由 Refresh 的指回关系承担，库只管当前」。
  - `application.FormETAHandler.FormETA` 按（租户，包裹，里程碑）`FindCurrent`，取到的可能是另一种来源的预测，再 `Refresh` 盖掉它；同函数头注「历史预测不覆盖」与存储行为相反。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：迁移另取号，ETA 改版本表、键加来源维；`FindCurrent` 按来源取；改正两处注释。

## 最小的诚实修法

照 `0016_tracking_projection_versions.sql` 的形状：每一版一行、只增不改写，当前版按（租户，包裹，里程碑，来源）各一条；刷新写新行并回指同来源的前一版。

## 要裁的

无（CONTEXT 已定）。对客可见判断从哪一来源取由 UC-VE-003 既有规则判，不在本票。
