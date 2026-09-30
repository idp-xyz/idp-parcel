# ADR-0163：供应商审核越权升级只比已匹配金额与已登记上限

Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的组织，就能回答一笔已匹配金额相对权限上限该不该升级。）
Date: 2026-09-30

## Context

`PAR-SET-05` 把越权升级的判断结构划成产品策略。审核角色和申请/决定分权归角色模型，不在本记录。`SupplierAuditAuthorityView` 已经交出审核人（ADR-0160），不交出金额上限。今天审核在授权人和应付账户都在时直接形成应付，金额权限没登记也放行。

## Decision

**一、判断只有一次比较。** 已匹配金额小于或等于已登记上限，在权限内，可以形成审核应付。大于上限，必须升级，不形成审核应付，也不改写成拒绝。不看角色，不看申请人和决定人是不是同一个人。

**二、上限是租户取值，另册登记。** 键是租户、供应商、责任法人、币种。命令是 `parcel-settlement-register audit-escalation-ceiling`。不写进只存审核人的审核授权册。不登任何租户的行，不写死阈值。

**三、没登记答未配置。** 空册与没有相符行都不形成审核应付，不默认放行。上限 0 是一份登记：任何正数金额都超过它，与没登记不是一回事。

**四、只拦会形成应付的已匹配行。** 量差、价差、无匹配和重复计费仍走原来的不可审，不先问上限。

## 候选与反方

- **超过上限记成拒绝。** 拒绝是授权决定，升级是权限不够、要交给更高权限。两格恢复动作不同。否决。
- **没登记时按审核人在册放行。** 审核人在册只说明谁能审，不说明能审多少。否决。
- **把上限写进审核授权行。** 那本册只交审核人。否决。
- **在本记录里定审核角色和谁可以终审。** 那是角色模型。否决。

## Consequences

- 迁移是 `settlement_accounting/0026_audit_escalation_ceiling.sql`。
- 金额文法与分摊分法不在本记录。
- 升级之后由谁终审，不在本记录。

## 越权风险点

1. 上限按供应商、责任法人、币种，不按审核人。同一范围只有一个上限。归 settlement-accounting 的 owner 复核。
2. 等于上限算在权限内。归 settlement-accounting 的 owner 复核。
3. 必须升级不形成应付，也不留下一张待升级的单据。归 settlement-accounting 的 owner 复核。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [ADR-0160](./0160-settlement-read-ports-have-registers.md)：审核授权册
- [settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)
- [UC-SA-004](../application/settlement-accounting/UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md)
- 票 `.scratch/product-strategy-boundary/issues/12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md` 第 5 项
