# ADR-0166：账单与财务交换的内置连接器形态是规范文书，没登记就不交换

Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的供应商或财务系统，就能回答外部账单和资金事实以什么形态进产品。）
Date: 2026-09-30

## Context

供应商账单的接收编排已经收一份规范主张。外部资金事实的采用面已经收一份规范事实。两边都没有把某一家的文件或报文译成那份规范文书的导入器，也没有把产品里的金额导出成某一家财务系统报文的导出器。

登记册把账单格式、传输方式、财务系统身份和映射样本留成租户取值。缺的是连接器形态：产品认不认一套通用版式。

## Decision

**一、内置形态只有规范文书。** 供应商账单与外部资金事实都可以登记为 `CANONICAL`：载荷已经是产品自己的主张或事实。产品不附带账单版式，也不附带财务系统报文。

**二、哪个对方采用这一形态，另册登记。** 命令是 `parcel-settlement-register accounting-connector`。键是租户、交换种类（供应商账单或外部资金事实）与对方标识。不预列供应商，不预列财务系统。不登任何租户的行。

**三、没登记答未配置。** 空册与没有相符对方都不放行这次外部交换，也不改用一份默认版式。放行只表示可以走既有的规范接收或规范采用，本记录不改那两条编排，也不在放行时调用它们。

## 候选与反方

- **附带一种通用 CSV 或某家财务系统报文。** 那是把一家的格式写成产品默认。否决。
- **把规范接收本身当成已经接上的导入器。** 接收编排要的是已经成形的主张，不是文件。否决。
- **没登记就直接接收规范主张。** 那是把「对方已采用规范文书」写成默认。否决。外部交换没登记仍是未配置；已经在产品内部成形的主张不经过这条门。

## Consequences

- 迁移是 `settlement_accounting/0029_accounting_connector.sql`。`0028` 留给周期费用。
- 不接真实财务系统。格式映射仍是租户取值。
- 周期费用、费用归属日、经营指标方法不在本记录。

## 越权风险点

1. 放行不调用账单接收，也不调用资金事实采用。归 settlement-accounting 的 owner 复核。
2. 只做规范文书，不做导出报文。归 settlement-accounting 的 owner 复核。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [UC-SA-004](../application/settlement-accounting/UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md)
- [UC-SA-005](../application/settlement-accounting/UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md)
- 票 `.scratch/product-strategy-boundary/issues/12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md` 第 7 项
