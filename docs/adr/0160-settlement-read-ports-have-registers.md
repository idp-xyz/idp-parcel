# ADR-0160：四本结算读口各有登记册，空册仍答未配置

Status: Accepted（2026-09-30。本项是机制，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」。）
Date: 2026-09-30

## Context

`ClaimAmountRuleView`、`ConfirmedChargeFactsView`、`SupplierAuditAuthorityView`、`SupplierPayableAccountView` 已有端口和消费它们的编排，没有生产实现，`cmd/` 也没有登记入口。册里的行是租户取值。金额文法与越权升级的判断结构不在本记录。

确认费用的七项事实已经作为确认结果写在 `customer_charge` 上（ADR-0087，迁移 0014）。确认当时那些列还是空的，读口不能从费用行自己把事实读出来。

供应商应付账户读口交回的是结算账户标识。结算账户登记册已经存在（ADR-0158）。这一问不是再立一套账户五格。

## Decision

**一、四口各有一本登记册，一行一个键，不设修订。** 同一键再登不同内容拒绝，已落的行不改。命令走 `parcel-settlement-register`：`supplier-audit-authority`、`supplier-payable-account`、`claim-amount-rule`、`charge-confirmation-facts`。不登任何租户的行，不造默认授权人、规则或账户。

**二、空册与没有相符行都答 found=false。** 审核停在未决，不默认放行、不虚构授权人。没有金额规则版本不形成金额。没有确认事实不用空值凑格。没有应付账户查问不从供应商身份推导账户。

**三、供应商应付账户与确认事实里的结算账户标识必须指向已有的结算账户行。** 账户五格仍只在结算账户登记册上。指向尚未登记的账户则拒绝，不代为建账户。

**四、确认事实册与费用行上的七列不是同一份数据。** 册是确认前的读口。确认结果仍按 ADR-0087 钉在费用行上，本记录不改那次写入。

**五、三只消费编排今天没有进程入口。** 本记录不新开消费者或 HTTP 端点。读口适配器由登记命令的进程构造；后继装配传入这四只适配器，不传 nil。空册仍按各口已有语义停住。

## 候选与反方

- **确认事实直接读 `customer_charge` 的七列。** 确认前那些列按约束必须为空，读口会永远答没有。否决。
- **供应商应付账户查问再存一套法人、相对方、方向、政策和币种。** 那是结算账户登记册的五格。再存一份就有了第二权威。否决。
- **空册时编一个授权人或从供应商身份填一个账户。** 端口已经禁止。否决。

## Consequences

- 迁移是 `settlement_accounting/0023_settlement_catalogue.sql`，按目录嵌入。
- 金额怎样由规则版本算出来，以及审核越权如何升级，仍不在本记录。
- 控制金额源、网络定义登记册的写入方不在本记录。

## 越权风险点

1. 确认事实册与费用行七列分开存。归 settlement-accounting 的 owner 复核。
2. 应付账户查问只保存账户标识，不核对方向是应付。归 settlement-accounting 的 owner 复核。
3. 三只编排未进进程。归 settlement-accounting 的 owner 复核。

## Links

- [ADR-0087](./0087-settlement-registers-carry-the-facts-their-hard-sentences-require-checking.md)：确认结果的七列
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [ADR-0158](./0158-settlement-account-register-is-an-immutable-tuple.md)：结算账户登记册
- [settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)
- 票 `.scratch/product-strategy-boundary/issues/16-mechanism-gaps-without-a-ticket.md` 第 3 项
