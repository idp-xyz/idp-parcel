# 16 机制缺口：重定级表第一项里尚无票的几处

Category: enhancement
Status: needs-triage——2026-09-24 通道 4 经用户授权自决立（票 02 遗留：开发主线写「缺口逐条交票 product-strategy-boundary/02 立工作票」）；各项分属不同上下文，接单时按上下文拆
Blocked by: 无
地盘：按项各归其上下文（见各项）。
出处：[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「按四项判据重定级」表第一项各格原话；[票 05](./05-demo-journey-criterion-evidence.md) 盘点格 3、5、21。已有票或已预告的不重立：操作者渠道归[票 15](./15-operator-channel-per-adr-0100.md)；节点收寄的身份核对缝归 `ps-external-mark-relations/01`；BUY 评价的来源引用回指归 `sa-cc-funds-and-credential-seams/11`；NO 实际测量登记册归 `pp-pricing-input-seams/04`；`PricingInputResolver` 的消费侧适配器由 `pp-pricing-input-seams` spec「不在本目录」预告另立、归 PP；网络定义登记册的写入方与定义原语归票 04。

## 做什么

1. **可达性资格视图的闭包标识**（network-routing 消费 party-commercial；重定级表 PN-02 行第一项，票 05 格 3）。`cmd/parcel-dispatch/assemble.go` 的 `acceptanceReachability` 以 `nrpartycommercial.NewCommercialEligibility(resolutions, nil)` 装资格视图，闭包标识生产为 nil。ADR-0064 的「从已接受解析回指」只改了初始路由那条链，本项照同一形补可达性这一条。
2. **结算账户登记册**（settlement-accounting；重定级表 PN-02 行第一项，票 05 格 5）。受理前控制的作用域源已接 `PolicyBackedControlScopeSource`，缺的账户目录背后没有登记册（SA 只有 `SettlementAccountID` 值对象），租户今天无处登记 `PAR-SET-01`。
3. **SA 结算读口背后的登记册与读口**（settlement-accounting；重定级表 PN-07 行第一项，票 05 格 21）：`ClaimAmountRuleView`、`ConfirmedChargeFactsView`、`SupplierAuditAuthorityView`、`SupplierPayableAccountView` 没有生产实现，`cmd/` 无一处引用。册里的行是租户取值；金额文法与越权升级的判断结构归[票 12](./12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md)。
4. **面单择优链的接受时解析回指**（parcel-shipment；重定级表 PN-02 行第一项）：`labelChannelSources` 的 `Resolutions`（`AcceptanceResolutionSource`）未接，与第 1 项同形。

## 不做

- 不登任何租户的行（账户、金额规则、审核授权），不给任何默认值。

## 完成判据

- 每项有生产实现（带真库测试），或记明已由别票承接；`PAR-SET-01` 等行的租户取值今后有处可登。
