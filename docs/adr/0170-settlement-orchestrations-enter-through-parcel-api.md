# ADR-0170：结算编排从 parcel-api 进入；确认与截单没登记就不做

Status: Accepted（2026-09-30。本项里装配与入口是机制，确认与截单的触发是产品策略。接受依据是 ADR-0146 决定一。）
Date: 2026-09-30

## Context

票面点名的七个结算编排在 `cmd/` 没有引用。确认与截单的时点不能写死。账期是租户取值。SELL 评价形成客户费用的编排不进 `cmd/parcel-dispatch`。

## Decision

**一、六个构造接在 `parcel-api` 的 `buildSettlementOrchestrations`。** `NewConfirmChargeHandler`、`NewCutOffPublishStatementHandler`、`NewRecordChargeAdjustmentHandler`、`NewAllocateCostsHandler`、`NewReceiveSupplierBillHandler`、`NewSettleClaimAmountsHandler`。没有 `NewAuditSupplierBillHandler`：审核是同一只接收编排上的 `Audit`。

**二、确认与截单先问触发册。** 命令是 `parcel-settlement-register settlement-moment`，取值只有 `CONFIRM` 与 `CUT_OFF`。没登记答未配置，不调用确认或截单。不写钟点。不写账期。不登任何租户的行。

**三、其余入口不另设触发册。** 调整、分摊、收单、审核、赔付金额沿各自编排已有的未配置语义。空册仍按那一口原来的答案停住。

## 候选与反方

- **确认和截单没登记也照样做。** 那是把时点写成产品默认。否决。
- **把 SELL 形成编排接进派发。** ADR-0169 已经排除。否决。

## Consequences

- 迁移是 `settlement_accounting/0033_settlement_moment.sql`。
- 启动时装配这些编排，确认与截单的调用走 `Confirm` 与 `CutOff`。

## 越权风险点

1. 触发册只有「采用了」，没有钟点。归 settlement-accounting 的 owner 复核。
2. 审核不另造构造函数。归 settlement-accounting 的 owner 复核。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [ADR-0169](./0169-sell-evaluation-forms-a-customer-charge.md)：形成编排不进派发，本记录不改那一条
- 票 `.scratch/product-strategy-boundary/issues/12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md` 第 10 项
