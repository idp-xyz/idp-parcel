# 46 SA 核销与核销撤销没有任何进程入口

Category: enhancement
Status: ready-for-agent——R2（上线后尽快）：资金映射已在生产装配上，核销却没有命令、端点或编排调用；首批收款一来只能停在未分配，入账余额「按核销分配净额」那一支走不到，已确认未结也不随核销离开。看得见的卡住、不答错金额；绕法是线下对账、留未分配。2026-10-21 文档自洽 09 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc09。
Blocked by: 无（与 [12](./12-funds-mapping-has-no-compensation-or-recovery-targets.md) 改同一只编排，宜同批或排在 12 之后）
出处：[audit/sa-cr.md](../audit/sa-cr.md) M12 的代码一半（文档一半由文档自洽 09 在 ADR-0236 的 Status 行注记更正）。取证于 main `899a4555`。

## 现象

UC-SA-005 主流程按指示或唯一匹配形成核销、处理撤销与退回形成核销撤销，SA CONTEXT 的入账余额按「指向该作用域客户应收的核销分配净额」重算；应用层有 `MapExternalFundsHandler.Apply` 与 `Reverse`，但 `cmd/` 下没有任何构造或调用：

- `cmd/parcel-settlement-register/translate.go` 头注：「核销（Apply）与撤销（Reverse）没有命令：核销面归 UC-SA-005 另一段的票。」那张票并不存在。命令表里只有 `funds-mapping`。
- `cmd/parcel-api` 只有 `GET /settlement-funds-applications`（`endpoints_settlement_catalogue.go`），没有写口。
- `cmd/parcel-dispatch` 没有任何信封驱动核销。

## 证据

- 文档：UC-SA-005 主流程步 4、步 6，`AT-SA-113`、`AT-SA-116`、`AT-SA-117`；SA CONTEXT「运营余额、收付款与坏账」入账余额那一条与核销各条；[ADR-0182](../../../docs/adr/0182-receivable-account-is-a-funds-mapping-target.md) 决定二「核销与核销撤销仍没有命令」。
- [ADR-0236](../../../docs/adr/0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md) 决定四附段与决定七说映射与核销「只有受控 CLI」，与代码不符；文档自洽 09 已在它的 Status 行注记里按 ADR-0182 决定二更正，正文不改。
- 代码：`application.MapExternalFundsHandler.Apply`、`.Reverse`；`ApplySettlementCommand`、`ReverseApplicationCommand`；`unversionedApplyDigest` 已备好载荷摘要。

## 三类

机制：核销与撤销的进程入口（受控 CLI 一路即可首发；在线口与授予照 ADR-0236 的口径另裁）。产品策略：无。租户取值：核销指示与账期顺序是来源数据，不预填。

## 规模

M：`parcel-settlement-register` 加 `funds-application` 与 `funds-application-reversal` 两个命令（照 `funds-mapping` 的译法与重放口径），装配 `MapExternalFundsHandler` 的 Apply/Reverse；补竖切测试；更正 `translate.go` 那句头注。

## 最小的诚实修法

先开受控 CLI 一路：命令只译载荷、不推导分配，分配必须有映射背书（`Apply` 已守）；撤销只登记核销关系失效、原分配不删（`Reverse` 已守）。在线写口与操作者授予不在本票。

## 要裁的

无（在线口另裁，不挡本票）。
