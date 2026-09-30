# 16 机制缺口：重定级表第一项里尚无票的几处

Category: enhancement
Status: needs-triage——2026-09-24 通道 4 经用户授权自决立（票 02 遗留：开发主线写「缺口逐条交票 product-strategy-boundary/02 立工作票」）；各项分属不同上下文，接单时按上下文拆
Blocked by: 无
地盘：按项各归其上下文（见各项）。
出处：[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「按四项判据重定级」表第一项各格原话；[票 05](./05-demo-journey-criterion-evidence.md) 盘点格 3、5、21。已有票或已预告的不重立：操作者渠道归[票 15](./15-operator-channel-per-adr-0100.md)；节点收寄的身份核对缝归 `ps-external-mark-relations/01`；BUY 评价的来源引用回指归 `sa-cc-funds-and-credential-seams/11`；NO 实际测量登记册归 `pp-pricing-input-seams/04`；`PricingInputResolver` 的消费侧适配器由 `pp-pricing-input-seams` spec「不在本目录」预告另立、归 PP；网络定义登记册的写入方与定义原语归票 04。

## 做什么

1. **可达性资格视图的闭包标识**（network-routing 消费 party-commercial；重定级表 PN-02 行第一项，票 05 格 3）。`cmd/parcel-dispatch/assemble.go` 的 `acceptanceReachability` 以 `nrpartycommercial.NewCommercialEligibility(resolutions, nil)` 装资格视图，闭包标识生产为 nil。ADR-0064 的「从已接受解析回指」只改了初始路由那条链，本项照同一形补可达性这一条。2026-09-30 通道 2 按 [ADR-0156](../../../docs/adr/0156-reachability-closure-identity-comes-from-the-adopted-resolution.md) 落地这一项：标识随命令带过，不进判断键。本票第 2–4 项未动，Status 不因此改。
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

**评审 ← 通道 1（隔离子代理，非作者）· 钉 `f6289f96`（同一分支，基 `a40393b7`，两笔净改动）· 2026-09-30 20:55**

- 上一轮两条阻断已消：`TestCommercialEligibilityReadsAStoredClosureByTheCommandResolution` 用真库 `pcpostgres.NewCommercialResolutions`，带 DSN `-v` 为 PASS；ADR-0064 只在 Status 与 Links 各加前向指针，正文未改写。
- **阻断**（两轴各一条，推送方核过代码）：
  - Spec：派单问「商业解析换了版本时，旧轮的可达性判断会不会被当成同一次判断复用；会就改」。会，且没改。NR 侧无碍（`reachability_judgment` 主键是租户 + 关联，换解析另存一行）；PS 判断账 `RecordReachabilityJudgment` 撞键 `ON CONFLICT DO NOTHING`，键里有版本、成员、时点，没有解析，其注释原话「同成员同时点再来一次是权威重放既有判断，保留先到者」。复现链：`revalidateOrResolve` 判依据被推翻 → `resolveAdoptedAgain` 把采用解析改记成 RES-2 → 下一轮按 RES-2 形成的判断因「提交接收」对同一版本给同一时点而撞键被吞 → 读回仍是 RES-1 那份，`revalidateReachability` 按它的 `JudgmentID` 只核视图修订、答仍当前 → 决定用 RES-2 的依据配 RES-1 的判断，正是 `resolveAdoptedAgain` 注释要防的「在旧依据下形成的判断」。ADR-0156 决定五说新版本「靠提交版本或判断时点」成为另一份判断，这条路径上两者都不变；越权风险点把它推给 PS owner 等于承认会复用。此前闭包标识为 nil、可达性形成不了，这条路本笔才变得可达。
  - Standards：`docs/adr/README.md` 里 0156 的索引行写「用户授权继续」，0156 Status 写那次答复「不是本记录的接受依据」，两处口径相反（红线「单一权威」）。
- **非阻断**：`reachability.go` 的 `correlationIdentity` 与 `RevalidateReachabilityJudgment` 注释仍写「同一派生回指」，重校已改按 `JudgmentID`；`routing.go` 保留 `ErrRoutingClosureTenantMismatch` 别名只为一处测试，改测试后删别名；`reachability_closure_store_test.go` 头注「空引用……不被读成未配置」与断言和 `CommercialEligibility` 注释相反；ADR-0156 两处引文不是原句（UC-NR-002 原句「新地址、声明、商业版本……需要形成新判断版本」；「机制与产品策略开发方现在就做」出自 ADR-0146 决定一而非 AGENTS）；ADR-0064 Context「`ReachabilityClosureIdentity` 仍属实例半边」一句也已失效且符号已删，按「部分停用」同一机制在 Status 标注；`loadClosure(tenantRaw string)` 先 `String()` 再解析。
- **核过无发现**：`initial_route.go` 只改注释；`routing.go` 抽 `loadClosure` 行为不变；`form_acceptance_decision.go` 只给可达性重校加 `JudgmentID`，初始路由不变；不需新迁移之说对 NR 成立；第 2–4 项、种子、租户行未动；`internal/architecture` 过。
- 推送方预演：隔离树把两笔重放到 `01d7d5e8` 之上得 `6bbb0709`、`16d78781`，零冲突，清点无差；链尖 gofmt 空、build 与 vet 绿，新真库用例单跑 PASS，带 DSN `go test -p 1 -count=1 ./...` 134 ok / 0 FAIL。预演不推。
- **结论：不重放**，回作者同一分支修；修完两轴重跑。

**评审 ← 通道 1（隔离子代理，非作者）· 钉 `489e77a5`（同一分支，基 `a40393b7`，三笔净改动，含迁移 `0023`）· 2026-09-30 21:12**

- 上一轮两条阻断：README 索引口径已消；判断账吞新判断那段代码已修——`TestANewResolutionAtTheSameInstantIsTheJudgmentTheDecisionReads` 带 DSN `-v` 为 PASS，放到 `f6289f96` 的判断账上为红（Spec 轴实测）。
- **阻断**：
  - Spec（推送方复现过变异）：修复没有测试守住。那条用例自己调 `FormedUnder`，只验判断账，没走 `resolveAdoptedAgain` → 推进 → 决定这条链。删掉 `AdvanceAcceptanceJudgmentHandler.Handle` 里唯一那处 `FormedUnder`，带 DSN 的 `parcelshipment/application`、`parcelshipment/adapters/postgres`、`cmd/parcel-dispatch` 仍全绿；`loadReachabilityJudgments` 又放行 `resolution_id = ''` 配任意采用解析，`0023` 无非空 CHECK——删了那一处，原缺陷原样回来。
  - 两轴同一处：ADR-0156 越权风险点 2 仍写同版本同时点「消费方仍只留先到者」，与改写后的决定五、与迁移 `0023`（解析进主键）相反（红线「单一权威」）。
  - Standards：ADR-0064 部分停用的范围各处不一——Status 已标 Context 那句失效，README 的 0064 索引行、0064 的 Links、0156 的 Consequences 与 Links 仍只说后果那一句（ADR 索引「部分停用」）。
- **非阻断**：`RecordReachabilityJudgment` 遇零值写 `''`、读口 `''` 兜底、`formedUnder` 注释里的「旧形状」库里不可能存在（Speculative Generality）；决定分支复用 `ReachabilityJudgmentSuperseded`，与 `judgment_continuation.go`「压格就会让不同的缺口共用一条引用」相抵，其上 AT-PS-037 注释讲的是网络视图换代，应用层无测试；`reachabilityFormedUnderAnotherResolution` 错误前缀照抄 `load reachability judgments`，失效判定写在适配器 SQL 里；决定五与 `ReachabilityStaleResolution` 注释写「没有一份是在当前解析下形成的」，实现是逐成员判；`0023` 头注以「尚无租户」为理由与 ADR-0150 不合，实际理由是只有 `ReachabilityAssessed` 才记行、本系列之前闭包标识为 nil 只能`未形成判断`，任何环境都无存量行（推送方核过记账路径）；0156 裁决能力边界里仍有非原文引文。
- 另一条 Standards「分支没重生成清点」不作阻断：作者写明留给推送方，推送方在链尖兜底（预演里是 `247c7fde`，`parcel_shipment` 迁移 22→23 份）。
- **核过无发现**：`0023` 在 `migrations/parcel_shipment`，编号与 main 不撞，按目录嵌入不需另接线，`internal/platform/migrate` 带 DSN 过；决定读口优先取当前解析下的判断；README 0156 行与 Status 一致；上一轮非阻断中「同一派生回指」注释、别名、测试头注、`loadClosure` 参数已改；第 2–4 项、租户行、解析号未碰；`internal/architecture` 过。
- 推送方预演：三笔重放到 `cc5e2c83` 之上得 `7001c6cd`、`dcfc70db`、`7a2a1c07`，清点另成 `247c7fde`；链尖 gofmt 空、build 与 vet 绿，两条新真库用例单跑 PASS，带 DSN `go test -p 1 -count=1 ./...` 134 ok / 0 FAIL。预演不推。
- **结论：不重放**，回作者同一分支修；修完两轴重跑。
