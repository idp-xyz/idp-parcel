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

## Comments

**评审 ← 通道 1（隔离子代理，非作者）· 钉 `5ac58a96`（通道 2 分支 `mcp2-reachability-closure`，第 1 项，ADR-0156，基 `a40393b7`）· 2026-09-30 20:32**

- **阻断**（两轴各一条，推送方核过原文）：
  - Spec：本票完成判据「每项有生产实现（带真库测试）」未达。`NewCommercialEligibility` 按解析标识经真库 `LoadResolution` 取闭包这条路没有任何真库用例跑到：适配器测试用内存替身 `newClosureStore`，`networkrouting/adapters/postgres` 的 `catalogReachEligibility` 是替身只改了签名，`cmd/parcel-dispatch` 的真图用例 `TestAnUnconfiguredAcceptanceChainStallsAsUndecidedOnTheRealGraph` 停在第一阶段、走不到可达性。完工报「真库测试在 networkrouting/adapters/postgres 与 cmd/parcel-dispatch」一句因此不成立。
  - Standards：ADR-0156 部分停用 ADR-0064 后果一句，只改了 `docs/adr/README.md` 索引行；ADR 索引「部分停用」要求「改被停用记录的 `Status` 行与 Links 节各加一条前向指针」，ADR-0064 正文仍写「可达性那条链的 `ReachabilityClosureIdentity` 不变」，与 0156、与代码两套口径（红线「单一权威」）。
- **非阻断**：ADR-0156 Status 的授权依据查不到出处——通道规则里「继续」是「检查队列」，不是接受 ADR；本项属机制，按 AGENTS 红线开发方本可自决，应改援引这条，并补裁决能力边界与越权风险点一节。解析标识不进 `SameJudgmentScope`，而 UC-NR-002 要「新……商业版本……形成新判断版本」、可达性这条链每轮经 `formAdoptedBasis` 重解——0064 那条理由靠的是已接受解析固定不变，0156 没说明为何照搬，请作者在 ADR 里答（答不上就升为阻断）。`CommercialResolutionReference` 类型注释仍写「已固定」「不是初始路由判断维」；`CommercialEligibility.AssessNetworkEligibility` 与 `RoutingApplicability.AssessRoutingApplicability` 逐行同体；可达性路径复用名字带 Routing 的 `ErrRoutingClosureTenantMismatch` 与测试辅助 `routingResolution`；`CommercialEligibility` 与 `acceptanceReachability` 注释里「不再……」是变更说明；开发主线 PN-02 格「闭包标识生产为 nil」已过时，照该文「上表单元格不改写」补一条补记。
- **核过无发现**：闭包标识取自 `formAdoptedBasis` 本提交版本第一阶段已记下的解析，与 UC-PC-002、ADR-0064 一致；`ReachabilityClosureIdentity` 已删，无判断键到解析的映射；空引用答未配置、编排形成`未形成判断`；租户不一致有哨兵；第 2–4 项、种子、租户行未动；`internal/architecture` 过。
- 推送方验证：隔离检出 `5ac58a96` 上清点重生成无差；gofmt 空、build 与 vet 绿，单跑真库用例为 PASS，带 DSN `go test -p 1 -count=1 ./...` 134 ok / 0 FAIL。
- **结论：不重放**，回作者同一分支修；修完两轴重跑。
