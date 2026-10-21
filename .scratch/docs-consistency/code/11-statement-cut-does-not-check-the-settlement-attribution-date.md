# 11 SA 对账单纳入不核「结算归属日」，归属日判定也没接进任何进程

Category: bug
Status: ready-for-agent——R2（上线后尽快）：截单发布已在生产装配（`cmd/parcel-api/assemble_settlement_orchestrations.go` 的 `CutOffPublishStatementHandler`），纳入哪些费用由调用方列，系统只核「已确认」；本地时刻过了截单点、本该归下一日的费用会被收进本期账单，碰钱（错期出账）但不丢不重。绕法：财务按归属日自行挑费用清单，可靠但全靠人。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: docs-consistency/09（SA+CR 修订）先裁 [audit/sa-cr.md](../audit/sa-cr.md) M11：业务时区与截单时刻的唯一权威是「对账安排」还是费用项目的归属日登记。纳入判据读哪一处，随这一裁。
出处：[audit/sa-cr.md](../audit/sa-cr.md) M7（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，SA 代码在此期间未改）。

## 现象

CONTEXT 与 UC-SA-003 写费用要「形成结算归属日……且落入截单快照」才能入对账单；代码截单只核费用阶段是不是已确认，没有任何归属日判断。归属日判定编排存在，却没有任何进程调用。

## 证据

- 文档：
  - SA CONTEXT Rules：「费用只有在达到自身确认条件、形成结算归属日、尚未出账且落入截单快照时才能进入对账单」。
  - UC-SA-003 步 2：「读取费用和调整的确认状态、结算归属日……归属日按已登记形态判定」。
  - ADR-0168 越权风险点 3：「截单编排不调用这次判定」。
- 代码：
  - `internal/settlementaccounting/domain/customer_statement.go` 的 `CutStatementDraft`：只核 `charge.Stage() != ChargeConfirmed`（→ `ErrChargeNotConfirmed`）、币种与去重。
  - `application/cut_off_publish_statement.go`：费用清单来自命令的 `ChargeIDs`，`CutOffAt` 由调用方给。
  - `application.JudgeChargeAttributionHandler` 在 `cmd/` 下无构造；只有登记一侧接在 `cmd/parcel-settlement-register`。

## 三类

机制：截单按归属日与截单快照判纳入。产品策略：归属日形态（ADR-0168 已定为登记形态）。租户取值：业务时区、截单时刻、各费用项目的形态——未登记答未配置，不默认。

## 规模

M：截单编排读（或当场判）每件费用的归属日，归属日晚于本期截单的拒收并指名；归属日未形成的答未决、指名缺哪一格；补 UC-SA-003 的用例。

## 最小的诚实修法

截单时对命令列出的每件费用取归属日：已形成的按它判是否落入本期，未形成的当场按已登记形态判；判不出（形态、时区或截单时刻未登记）就答未配置并指名，不按调用方的列表放行。在费用确认时就形成归属日是另一条路，属同一裁定的选项。

## 要裁的

- 见 Blocked by：时区与截单时刻的权威在哪一处（M11）。
- 归属日在费用确认那一刻形成并落库，还是截单时当场判。推荐前者（与「不可变的归属」一致），随 09 的裁定写定。
