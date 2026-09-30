# 16 机制缺口：重定级表第一项里尚无票的几处

Category: enhancement
Status: needs-triage——第 1 项 2026-09-30 进 main（`66ea4bd4`），第 4 项同日进 main（`695ac2f8`），第 2 项同日进 main（`7dfe7e2a`），各见文末对应「进 main 记录」；第 3 项未动。2026-09-24 通道 4 经用户授权自决立（票 02 遗留：开发主线写「缺口逐条交票 product-strategy-boundary/02 立工作票」）；各项分属不同上下文，接单时按上下文拆
Blocked by: 无
地盘：按项各归其上下文（见各项）。
出处：[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「按四项判据重定级」表第一项各格原话；[票 05](./05-demo-journey-criterion-evidence.md) 盘点格 3、5、21。已有票或已预告的不重立：操作者渠道归[票 15](./15-operator-channel-per-adr-0100.md)；节点收寄的身份核对缝归 `ps-external-mark-relations/01`；BUY 评价的来源引用回指归 `sa-cc-funds-and-credential-seams/11`；NO 实际测量登记册归 `pp-pricing-input-seams/04`；`PricingInputResolver` 的消费侧适配器由 `pp-pricing-input-seams` spec「不在本目录」预告另立、归 PP；网络定义登记册的写入方与定义原语归票 04。

## 做什么

1. **可达性资格视图的闭包标识**（network-routing 消费 party-commercial；重定级表 PN-02 行第一项，票 05 格 3）。`cmd/parcel-dispatch/assemble.go` 的 `acceptanceReachability` 以 `nrpartycommercial.NewCommercialEligibility(resolutions, nil)` 装资格视图，闭包标识生产为 nil。ADR-0064 的「从已接受解析回指」只改了初始路由那条链，本项照同一形补可达性这一条。2026-09-30 通道 2 按 [ADR-0156](../../../docs/adr/0156-reachability-closure-identity-comes-from-the-adopted-resolution.md) 落地这一项：标识随命令带过，不进判断键。本票第 2、3 项未动，Status 不因此改。
2. **结算账户登记册**（settlement-accounting；重定级表 PN-02 行第一项，票 05 格 5）。受理前控制的作用域源已接 `PolicyBackedControlScopeSource`，缺的账户目录背后没有登记册（SA 只有 `SettlementAccountID` 值对象），租户今天无处登记 `PAR-SET-01`。2026-09-30 通道 3 按 [ADR-0158](../../../docs/adr/0158-settlement-account-register-is-an-immutable-tuple.md) 落地这一项：登记册一行固定五格，绑结算政策对象不绑某一版，命令 `parcel-settlement-register settlement-account`，生产装配的目录读这本册，没登记的应收行仍答未配置。本票第 3 项未动。
3. **SA 结算读口背后的登记册与读口**（settlement-accounting；重定级表 PN-07 行第一项，票 05 格 21）：`ClaimAmountRuleView`、`ConfirmedChargeFactsView`、`SupplierAuditAuthorityView`、`SupplierPayableAccountView` 没有生产实现，`cmd/` 无一处引用。册里的行是租户取值；金额文法与越权升级的判断结构归[票 12](./12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md)。
4. **面单择优链的接受时解析回指**（parcel-shipment；重定级表 PN-02 行第一项）：`labelChannelSources` 的 `Resolutions`（`AcceptanceResolutionSource`）未接，与第 1 项同形。2026-09-30 通道 2 按 [ADR-0159](../../../docs/adr/0159-label-channel-resolution-comes-from-the-accepted-decision.md) 落地这一项：择优查询带来源身份与声明包裹，解析标识从该成员的接受决定读，不另造映射。本票第 2、3 项未动，Status 不因此改。

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

**评审 ← 通道 1（隔离子代理，非作者）· 钉 `4a54f7bb`（同一分支，基 `a40393b7`，四笔净改动）· 2026-09-30 21:36**

- 上一轮三条阻断：修复有测试守住——删掉推进里的 `FormedUnder`，`cmd/parcel-dispatch` 六条红（推送方与 Spec 轴各复现一次）；推进改打旧解析、让 `reachabilityFormedUnderAnotherResolution` 恒答否，`TestAChangedResolutionAdvancesANewJudgmentTheDecisionReads` 都红；去掉读口按解析过滤，由适配器用例 `TestANewResolutionAtTheSameInstantIsTheJudgmentTheDecisionReads` 守；去掉 `0023` 的 CHECK，`TestAcceptanceJudgmentShapesArePinnedInTheDatabase` 红。越权风险点 2 与决定五、`0023` 已一致。ADR-0064 部分停用范围：0064 的 Status、Links、README 0064 行与 0156 的 Consequences、Links 已写两句。
- **阻断**：Spec 一条——`docs/adr/README.md` 里 ADR-0156 自己那一行仍只写「部分停用 ADR-0064 后果里可达性那一句」，漏了 Context 那句（修复卡 3「各处一致」）。纯索引一行，推送方在进 main 那一笔补齐，不回作者。Standards 轴称「五处一致」漏看了这一行。
- **非阻断**（随票记，建议作者另立收尾票）：`resolution_change_acceptance_test.go` 头注说删掉 `FormedUnder` 后「决定停在形成于别的解析」，实测是首轮推进就因 `ErrReachabilityResolutionRequired` 落进 `JUDGMENT_NOT_RECORDED`，注里的 `RES-1` 与代码 `SYN-RES-R1` 不一；字段 `RecordedJudgments.ReachabilityStaleResolution` 未随查询与新原因改名，同一概念三个名字；`form_acceptance_decision.go` 新注复述常量注的理由，两处都把 AT-PS-037 只归给网络视图换代；ADR-0156 决定五写「按已失效重做」而代码另立 `ReachabilityJudgmentFormedUnderAnotherResolution`；`0023` 头注「本系列之前」是变更说明；README 0064 行漏「其余各条不变」；「判断须带形成时的解析」只落在 postgres 适配器与 CHECK 上，端口 `AcceptanceJudgmentRecorder` 未声明；领域 `formedUnder` 注释「零值是……旧形状」与 `rebuildReachabilityJudgment` 的 `!= ""` 分支在 CHECK 之下走不到。
- **核过无发现**：新用例接真 PS 库与两个真编排，断言决定读的是 `SYN-NRJ-RES-2` / `SYN-RES-R2` 并形成接受；新续办原因有 `String()`、归内部重试，别处无穷举映射；`0023` 与 `internal/platform/migrate`、`internal/architecture` 带 DSN 过；引文是原句；ADR-0064 只改 Status 与 Links；第 2–4 项、租户行、生产代码里的解析号未碰。
- **结论：可重放**，阻断由进 main 那一笔补齐。

**完成记录（通道 2 · 据四次完工报转录，第 1 项）**

分支 `mcp2-reachability-closure`，四笔均快进推送、未改写：`5ac58a96` 闭包标识从本轮已采用的商业解析回指（ADR-0156，删 `ReachabilityClosureIdentity`，装配不再传 nil）；`f6289f96` 真库用例 `TestCommercialEligibilityReadsAStoredClosureByTheCommandResolution`，ADR-0064 前向指针，关联带上解析、换解析不重放旧判断；`489e77a5` 迁移 `0023` 把 `resolution_id` 并进可达性判断主键，决定只采用当前解析下的判断；`4a54f7bb` 编排用例 `TestAChangedResolutionAdvancesANewJudgmentTheDecisionReads`（删 `FormedUnder` 即红），空解析写入报 `ErrReachabilityResolutionRequired`，`0023` 加非空 CHECK，换解析另立续办原因 `ReachabilityJudgmentFormedUnderAnotherResolution`。作者自验：`go build` / `go vet` 绿；networkrouting、parcelshipment、`cmd/*` 含 DSN `-count=1 -p 1` 绿，新真库用例 `-v` 无 SKIP。未改 `migrations.go` / `plan.go`（按目录嵌入）；未动第 2–4 项，未登租户行，未造解析号。

**进 main 记录（2026-09-30 21:3x，通道 1 推送）**

分支 `mcp2-reachability-closure@4a54f7bb`（已推 origin）在隔离树重放到 `713537f3` 之上，零冲突：`5ac58a96→3a600e5c`、`f6289f96→e7b4cd1b`、`489e77a5→6b670743`、`4a54f7bb→66ea4bd4`；清点在代码链尖重生成为 `9b21ff41`（`parcel_shipment` 迁移 22→23 份，`cmd` 测试 105→106）。本记录一笔另补 README 0156 行（见上阻断）。
推送方验证：钉 `9b21ff41`（与本记录一笔只差 `.md`），`gofmt -l` 空，build 与 vet 退 0，三条相关真库用例单跑 PASS 非 SKIP，带 DSN `go test -p 1 -count=1 ./...` 134 ok / 0 FAIL。分支作封存出处。

**评审 ← 通道 1（隔离子代理，非作者）· 钉 `220fd8b1`（通道 2 分支 `mcp2-psb16-04`，第 4 项，基 `d3942581`）· 2026-09-30 22:10**

派单记录：21:51 点名通道 2、3，截止 21:54 前两个应答；第 4 项派通道 2（`task-a2b043d0`），第 2 项派通道 3（`task-02ca259f`）。通道 2 的 ADR 先写成 0158，与通道 3 在频道先占的 0158 撞号，随后在 `a8f632d2` 只改号为 0159（推送方逐文件核过：把 0159 换回 0158 后七个文件与 `220fd8b1` 逐字节一致）。

- **阻断**（Standards）：本笔让既有注释成了假话，且都是 AGENTS「改文档」禁止的计数：`buildLabelChannelOrchestration`「六个实例半边缝全部显式未配置」、`labelChannelSeams`「六个实例半边缝」、`channel_selection_basis.go` 文件头「三问是消费方自己的实例半边」、`ChannelSelectionBasisTranslatorDeps`「三个实例半边源」、`ErrChannelBasisTranslationStopped` 仍把 Resolutions 算作实例半边、领域 `ChannelSelectionSubject` 头注「那两者进入参那天，本引用随之加格」与 ADR 决定一相反；新写的「测试替身把前五个配上」又是计数且不实（测试只换端口级替身）。
- **非阻断**：`labelChannelSources` 的约定是「nil 即按显式未配置装配」，`Resolutions` 却是 nil 时装真读口，同一结构两种相反语义，无测试设这一格——第 1 项的做法是删缝、直接装配；删掉装配里 `NewAcceptedDecisionResolution(requests)` 那一行，带 DSN 的 `cmd/parcel-api` 仍全绿（Spec 轴变异）；已有按（租户，包裹）回答的 `ports.CommercialResolutionReferenceView`（ADR-0133）读同一回指，本笔另开一条读路，ADR 候选与 Links 未权衡；`EstablishSelectedLabelTransactionCommand` 的 `CoveredParcels` 与 `Selection.Shipment`、`Selection.Parcel` 不互核，多包裹交易取哪个成员 ADR 未说；`NewAcceptedDecisionResolution` 构造期不拒 nil，与同包构造函数及 `NewChannelSelectionBasisTranslator` 头注「构造期拒……」不一；`acceptedShipmentRequests` 与同包 finder 同形；`ErrAcceptanceResolutionNotConfigured` 也承载未接受、非成员、查无委托，把`未形成`答成`未配置`；行为测试只在 `adapters/postgres`，实现包无单测；`query.Shipment.TenantID().String() == ""` 判空。
- **核过无发现**：取 `CommercialResolutionReferenceFor`，即接受决定上固定的解析，与 ADR-0064 同形；接受后不再形成新提交版本，未接受答未形成；按租户取行、租户不一致有哨兵；`TestTheAcceptedDecisionResolutionSourceReadsTheStoredDecision` 带 DSN `-v` 为 PASS，走生产 `ShipmentRequests`，末行改答未配置、删租户核对都红；链仍停在约束那一格，属实；票 16 第 4 项落地句与开发主线补记与代码一致；第 2–3 项、票 06 第 3 项、租户行、解析号未碰；`internal/architecture` 过。
- 推送方验证：隔离检出 `220fd8b1` 上清点另成 `6e741abb`；gofmt 空、build 与 vet 绿，真库用例单跑 PASS，带 DSN `go test -p 1 -count=1 ./...` 134 ok / 0 FAIL。
- **结论：不重放**，回作者同一分支修（`a8f632d2` 之上）；修完两轴重跑。

**评审 ← 通道 1（隔离子代理，非作者）· 钉 `55a0d7f1`（通道 3 分支 `mcp3-psb16-02`，第 2 项，ADR-0158，基 `d3942581`）· 2026-09-30 22:20**

- **阻断**：
  - 推送方全量（重放到 `1712eccc` 之上为 `ba9e0253`、清点 `1cfd506b`，带 DSN `-p 1 -count=1`）红：`internal/architecture` 的 `TestEveryPersistenceWriteMethodCarriesTransactionRequiredEvidence`——新写口 `SettlementAccounts.Save` 没有「无事务即拒」的负向用例。作者自验未跑 `internal/architecture`。
  - Spec（推送方核过代码）：目录键里的「结算政策」实为政策版本。生产 `settlementTermsFor` 交回的回显形如 `<对象>/<版本>`，`RegisteredAccountDirectory.FindSettlementAccount` 原样拿它比 `settlement_policy_id`，而登记格名为 `settlementPolicyId`、`SettlementPolicyReference` 注释写「版本生命周期不在本上下文」。租户按政策标识登记，生产永远查不到；政策升版原账户即失配，另登要么冲突要么拆账。用例两侧都写同一版本串，把它盖住了；ADR-0158 未讨论。
  - Standards：ADR-0158 推翻 ADR-0081 决定六「结算账户目录……全部留 nil」，未按 ADR 索引标「部分停用」（Status、Links 前向指针与索引行）；「候选与反方」把对账周期、业务时区、截单时刻、付款条件说成 `PAR-SET-01` 点名的条件，前三格属 `PAR-SET-09`、付款条件属 `PAR-COM-10`（商业侧结算政策）——账户上再必填一份原文，就是第二套口径（红线「单一权威」）。
- **非阻断**：结算相对方、责任法人、租户三个谓词与重放/冲突分类无用例守住（Spec 变异：去掉任一个或让重放答已登记，全绿），`settlement-account` 命令与 `SettlementAccountFromJSON` 无测试；生产装配无守卫（把 `acceptanceFinancialControl` 里的目录换回 nil，`cmd/parcel-dispatch` 全绿）；`SettlementAccountDirectory`、`NewPolicyBackedControlScopeSource` 注释仍写目录属「实例半边」、nil「正是首发要停的地方」，`registrationjson` 包头注只讲资金事实；「五格唯一」「不设修订」是新不变量，只进了 ADR 没先进 SA `CONTEXT`；`ChargeDirectionFromName` 与 postgres 包 `chargeDirectionFrom` 重复且把词表解析放进领域层，`SettlementPolicyReference`、`ResponsibilityBasis` 与既有 `AdoptedPolicyReference`、`ContractBasisReference` 重复，`FindSettlementAccount` 与 `FormControlScope` 各译一遍法人与币种且错误归类不一；`classify` 第二次查询结果不被使用；`Save` 是全仓唯一用 SAVEPOINT 的写法（SA 其余写口 `ON CONFLICT DO NOTHING` 再读回）；「付款方与相对方重复」「方向词不在词表」包成 `ErrBlankValue`；PS 适配器依赖带 `Save` 的 `SettlementAccountRegister`，宜收窄为只读口（先例 `AdoptedFundsFactView`）。
- **核过无发现**：「只查应收」有 ADR 决定三与注释支撑；五格唯一约束落在库上；未登种子行；第 3–4 项与 `ControlAmountSource` 未动；两条真库用例带 DSN `-v` 为 PASS，去掉政策谓词会红。
- **结论：不重放**，回作者同一分支修；修完两轴重跑。

**评审 ← 通道 1（隔离子代理，非作者）· 钉 `18e01663`（通道 2 分支 `mcp2-psb16-04`，第 4 项，ADR-0159，基 `d3942581`，三笔净改动）· 2026-09-30 22:28**

- 上一轮阻断（注释失真与计数）已修；非阻断里 nil 回落真读口、装配无守卫、未权衡 `CommercialResolutionReferenceView`、多包裹取哪个成员、构造期拒 nil、未形成混进未配置、`acceptedShipmentRequests` 重复、租户判空已处理。
- **阻断**：Spec 一条——`channel_selection_basis_test.go` 里 `TestTheTranslatorRefusesNilReadersAndNamesEachUnwiredSource` 头注仍把接受时解析源称为「实例半边源」，与 ADR-0159「本项是机制」相反。一行注释，推送方在进 main 那一笔改掉，不回作者。
- **非阻断**（随票记）：翻译器 `resolutionFor` 把源的错误一律收成未配置、源对「已提交未接受」答 `(false,nil)`，两个变异都全绿——修复卡⑤「未形成分开报」只在源这一层有用例守，全链没有用例断言停因是 `ErrAcceptanceResolutionNotFormed`；「未接受」一格与构造期拒 nil 无用例；ADR 决定一说 `CommercialResolutionReferenceView`「找任一已接受成员」不准确（多于一行答 `ErrAmbiguousParcelTarget`），不复用的理由仍成立、两口都经 `CommercialResolutionReferenceFor` 取值；「一笔面单交易只盖一份委托」PS `CONTEXT` 没写也无处拦，建议记进票 06 第 3 项触发面那张票；`NewChannelSelectionBasisTranslator` 头注一句里三次「账号使用授权读口」；`assemble_label_channel.go` 文件头与 `labelChannelSources` 注释有变更叙述；`acceptedDecisionResolution.ResolutionFor` 的 `requests == nil` 分支走不到，查询租户为零值时静默跳过租户核对；`saveAcceptedLabelChannelRequest` 与同包 `acceptOnRealAssemblyWith` 重复。
- **核过无发现**：仍读接受决定上固定的解析（ADR-0064 同形），不造映射、不造解析号；装配改交 nil、删租户核对、源对非成员答未找到，都有用例红；ADR-0159 节次齐、未部分停用任何已接受 ADR；`channel_selection_decision.go`、`establish_selected_label_transaction.go`、`main.go` 只改注释；第 2–3 项、票 06 第 3 项、租户行未碰；`internal/architecture` 过。
- **结论：可重放**，阻断由进 main 那一笔补齐。

**完成记录（通道 2 · 据三次完工报转录，第 4 项）**

分支 `mcp2-psb16-04`（基 `d3942581`），三笔均快进推送、未改写：`220fd8b1` 生产装配 `NewAcceptedDecisionResolution`，查询带来源身份与声明包裹，解析标识从接受决定读，真库用例 `TestTheAcceptedDecisionResolutionSourceReadsTheStoredDecision`（丢掉 `CommercialResolutionReferenceFor` 的结果即红）；`a8f632d2` 只改号 ADR 0158→0159（通道 3 先占 0158）；`18e01663` 注释按现状改写不再计数，删 `Resolutions` 缝直接装配，未形成（`ErrAcceptanceResolutionNotFormed`）与没装分开，构造期拒 nil，ADR-0159 写明不复用 `CommercialResolutionReferenceView`、多包裹不互核，守装配用例 `TestProductionAssemblyReadsTheAcceptedDecisionResolution`（装配改传 nil 即红）。作者自验：`go build` / `go vet` 绿；`parcelshipment` 与 `cmd/*` 带 DSN `-count=1 -p 1` 绿。无 `.sql`。

**进 main 记录（2026-09-30 22:3x，通道 1 推送，第 4 项）**

分支 `mcp2-psb16-04@18e01663`（已推 origin）在隔离树重放到 `5167c2b8` 之上，零冲突：`220fd8b1→ac5dd62b`、`a8f632d2→462c3065`、`18e01663→695ac2f8`；清点在代码链尖重生成为 `05060ee9`（`parcelshipment` 生产 194→195、测试 188→189）。本记录一笔另改上面阻断那一行测试注释。
推送方验证：钉 `05060ee9`，`gofmt -l` 空，build 与 vet 退 0，两条相关真库用例单跑 PASS 非 SKIP，带 DSN `go test -p 1 -count=1 ./...` 134 ok / 0 FAIL（含 `internal/architecture`）；本记录一笔只多一行测试注释与 `.md`，另在其上重跑 gofmt、build、vet 与 `internal/parcelshipment/adapters/partycommercial` 包测试。分支作封存出处。

**评审 ← 通道 1（隔离子代理，非作者）· 钉 `2bdf6306`（通道 3 分支 `mcp3-psb16-02`，第 2 项，ADR-0158，基 `d3942581`，两笔净改动）· 2026-09-30 22:4x**

- 上一轮三条阻断已消（推送方核过代码与真库）：
  - architecture：`TestSavingASettlementAccountRequiresATransaction` 点名 `ErrTransactionRequired`，`TestEveryPersistenceWriteMethodCarriesTransactionRequiredEvidence` 带 DSN `-v` 为 PASS。
  - Spec：`RegisteredAccountDirectory.FindSettlementAccount` 取 `SettlementPolicyEcho.Object()` 建键，不比 `Policy().String()`；`TestAPolicyObjectRegisteredOnceAnswersBothAdoptedVersions` 带 DSN `-v` 为 PASS 非 SKIP。若目录仍比版本整串，该用例会对 v1/v2 查不到已登对象而行红。
  - Standards：ADR-0158 改写四格否决与目录读对象；ADR-0081 `Status`、Links、README 0081/0158 行均标部分停用决定六「结算账户目录留 nil」；`0022` 与领域行已无对账周期、业务时区、截单时刻、付款条件。
- **阻断**：无。
- **非阻断**（随票记）：领域 `SameRegistration` 仍只覆盖方向；`registrationjson` 包头仍写「外部资金事实」；`ChargeDirectionFromName` 与 `ChargeDirection.String` 双开关、`Save` 独用 SAVEPOINT、`ErrBlankValue` 包裹非空白，本笔未触及。`NewRegisteredAccountDirectory` 空参报错仍写 `register is nil`。
- **核过无发现**：生产装配 `acceptanceFinancialControl` 接 `NewRegisteredAccountDirectory`，`TestAcceptanceFinancialControlDoesNotPassANilAccountDirectory` 守住；命令译装在事务前，`TestTheSettlementAccountCommandTranslatesBeforeTheTransaction` 绿；CONTEXT 已写五格唯一、绑政策对象；种子无账户行；第 1、3、4 项无生产代码；读口收窄为 `SettlementAccountView`。
- **结论：可重放**。

**完成记录（通道 3 · 据两次完工报转录，第 2 项）**

分支 `mcp3-psb16-02`（基 `d3942581`），两笔均快进推送、未改写：`55a0d7f1` 结算账户登记册接上接受前控制的账户目录（ADR-0158，命令 `parcel-settlement-register settlement-account`，空册与没有相符应收行停在 `CONTROL_SCOPE_NOT_CONFIGURED`）；`2bdf6306` 目录按政策对象查找、账户行去掉对账四格、ADR-0081 决定六「结算账户目录留 nil」部分停用，无事务拒证 `TestSavingASettlementAccountRequiresATransaction`，跨版本 `TestAPolicyObjectRegisteredOnceAnswersBothAdoptedVersions`。作者自验：`go build` 绿；SA/PS postgres 与 `cmd/parcel-dispatch` `-count=1` 绿。迁移 `settlement_accounting/0022_settlement_account.sql`。未登租户行，未动第 3 项。

**进 main 记录（2026-09-30 22:4x，通道 1 推送，第 2 项）**

分支 `mcp3-psb16-02@2bdf6306`（已推 origin）在隔离树重放到 `42733478` 之上。三处文档冲突（票面、ADR 索引、开发主线补记）按意图解：保 main 已进的第 4 项补记与 ADR-0159 行，插入 ADR-0158 与第 2 项补记；第 2 项补记剩余未满足只列网络定义登记册写入方与控制金额源（面单择优已在本 tip 上）。对照：`55a0d7f1→b3dab343`、`2bdf6306→7dfe7e2a`；清点在代码链尖重生成为 `d49c73e8`（`parcelshipment` 生产 195→196、测试 189→190；`settlementaccounting` 生产 101→105、测试 82→84；`settlement_accounting` 迁移 21→22；`cmd` 测试 106→108）。
推送方验证：钉 `d49c73e8`，`gofmt -l` 空，build 与 vet 退 0，三条相关真库用例单跑 PASS 非 SKIP，带 DSN `go test -p 1 -count=1 ./...` 134 ok / 0 FAIL（含 `internal/architecture`）。本记录一笔只多本票面 `.md`。分支作封存出处。
