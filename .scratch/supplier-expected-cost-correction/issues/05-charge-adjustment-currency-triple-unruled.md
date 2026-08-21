# `ChargeAdjustment` 是否受币种三件组约束未裁，且没有评价引用

Category: bug
Status: needs-triage

由 [`04`](./04-customer-charge-single-currency-contradicts-context.md) 的裁断划出：owner
2026-08-21 裁定客户**费用**受三件组约束（[SA CONTEXT](../../../docs/domain/settlement-accounting/CONTEXT.md)
「每条费用分别保存原币金额、合同结算币金额及换算依据」按字面执行），并明确**费用调整
是否同受约束另票单裁**。本票即那张票。

## 病灶

`internal/settlementaccounting/domain` 的 `ChargeAdjustment` 只带单一币种与单一金额
（`Currency` + `AmountMinor`），带借/贷方向。两个未裁问题：

1. CONTEXT 那句「每条费用」是否覆盖调整——调整是引用费用的差额新对象，不是费用本体；
   但它的金额进入对账单（`StatementAdjustmentLine` 复述其金额入快照），币种表达问题
   同样成立。
2. 若三件组罩住调整，`ChargeAdjustment` 今天**连评价引用都没有**：计价纠错类调整的
   金额语义上出自新的 SELL 评价，「三件从同一个评价采用来的一组」在这个类型上无处挂。
   商业让利类调整的金额是否出自评价、还是出自商业授权本身，亦未裁。

## 与 ADR-0067 的边界

[ADR-0067](../../../docs/adr/0067-cost-correction-restates-the-whole-evaluation-result.md)
决定一只裁了**形状**——客户侧调整是带借/贷方向的差额调整，不是重述全额的新版本——
没有裁调整金额的币种表达。不要把该 ADR 当本票的答案引用。

## 不在本票内

- 本票只记录未裁问题，不提方案；属领域 owner 裁断。
- 客户费用本体的三件组已由 `04` 裁定并实现，不在本票。

## 影响面

初步 grep：无 `charge_adjustment` 独立表，调整金额以对账单行快照（迁移 `0002`）落库；
`FormChargeAdjustment` 的调用方与完整落库面 triage 时取证。

## Comments

- 2026-08-21 · MCP-2：随 `04` 裁断创建，防「另票」落空。
