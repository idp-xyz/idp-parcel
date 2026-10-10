# 16 演示租户受理前财务控制两格：时点采用产品参考配置、种子登结算账户

Category: enhancement
Status: needs-triage——2026-10-10 通道 1 立（用户授权自决），出自 [11](11-demo-network-adopted-as-reference-configuration.md) 的判据一取证：演示动线越过 psb/05 格 4 之后停在格 5
归档：不属 [psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md) 的子票集；放在本目录是因为它挡在 11 判据一的路上。
地盘：`scripts/demo-seeds`（演示租户的采用行与账户登记）；分诊若发现要动执行器，另核地盘。
出处：[psb/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md)「格 5 · 受理前财务控制」两次取证，都写「没找到点名这一格的票」；AGENTS.md「演示租户就是 SYN-TENANT-01，代码按真实租户对待它」；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)。

## 现象（通道 2 实测，代码钉 rfc/11 的 `1bbd0f09`，种子即 `6857e599`）

- 演示委托的可达性答 REACHABLE、越过格 4 之后，停在格 5：`FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED` / `OPERATOR_REGISTRATION`。委托不会被接受，初始路由也就不会被触发。
- 时点一半：种子规则包里 `PRE_ACCEPTANCE_FINANCIAL_CONTROL` 那一格的语义是合成串 `SYN-ASOF-ACCEPT-TIME`，没采用产品参考配置；`SubmissionReceiptAsOf.FormAsOfValue` 只认 `submission-receipt@1` 那一版引用。
- 账户一半：账户目录登记册已有，种子没登演示租户的结算账户；越过时点之后会答 `CONTROL_SCOPE_NOT_CONFIGURED`（psb/05 按代码判）。
- `Amounts` 的估价方法在 [psb/06](../../product-strategy-boundary/issues/06-ps-acceptance-and-label-selection-judgment-methods.md) 第 2 项，needs-triage。

## 待分诊

- 两格都是演示租户的租户取值（采用哪一种时点形态、登哪个账户）：按 ADR-0146 经参考配置采用、用 `SYN-` 数据灌。先核两格是否都只需改种子、不碰执行器。
- `Amounts` 留空时财务控制答什么：若它挡在两格之后，本票 Blocked by psb/06 第 2 项；不挡就不挂。
- 走通之后的下一停点：受理决定形成、初始路由触发后停在成本来源，见 [17](17-initial-route-pricing-input-from-customer-declaration.md)。
