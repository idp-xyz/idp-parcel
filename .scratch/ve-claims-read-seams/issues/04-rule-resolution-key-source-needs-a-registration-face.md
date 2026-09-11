# VE 词到 PC 闭包键的翻译缺登记面——生产装配的 `RuleResolutionKeySource` 今天只能留 nil

Category: enhancement
Status: resolved——**已进 main，2026-09-11 23:5x**（通道 1 推送方重放到 main `aa2e6a07` 之上：`943d6f2c` / `e4814324` / `876e4280` / `bf722146` + 推送方清点 `637245e7`，见 Comments「进 main 记录」；非作者评审 ← 通道 1 两轴 0 阻断；判据 3 按 23:3x 中途裁 (a) 改口，PS 两道闸另立 [ps-port-remainder/09](../../ps-port-remainder/issues/09-resolution-key-face-does-not-accept-customer-service-rule.md)）；此前 resolved——2026-09-11 23:4x 通道 3 作者完工（task-6afdcc42；分支 `mcp4-veclaims04` 基 `262e8c0a`，代码 + 本完成记录同笔，SHA 见完工报）：ADR-0136 落地——VE→PS 回指窄读缝、`ClaimServiceRules` 三段、退役 `RuleResolutionKeySource` / `ErrCustomerServiceRuleUnresolved`、`buildClaimEligibilityRules` 去 Keys 收两只读口、装配测试三态；带 DSN VE `adapters/partycommercial` + `cmd/parcel-api` + `internal/architecture` 全 ok。**判据 3 原句「SYN 解析键那一行必需依据含 CustomerServiceRuleObject」在 `262e8c0a` 上做不到**——PS 解析键登记面（迁移 0008 CHECK 白名单 + PS `commercialKindFrom` 名集）今天不收 `CUSTOMER_SERVICE_RULE`，属 PS 地盘，已按推送方 23:3x 裁决改口（「裁决」5、判据 3 划掉留痕、PS 票 ps-port-remainder/09）：装配测试对着真登记面把这个事实钉成负断言，「已登记」态的键直给 PC 真解析器解出并固定闭包，「未采用客户服务规则」态真走 PS 登记面，见「完成记录」判断项 1。等非作者评审 → 推送方重放进 main。此前 in-progress——2026-09-11 23:2x **通道 3 接手**（task-6afdcc42；通道 4 会话已无，其 21:36–21:42 未提交现场由推送方 21:45 封存为 chore(salvage)，原样一字未改）：分支 `mcp4-veclaims04` 在树 `D:/tops/idp-parcel-mcp4-veclaims04` 内 `git rebase origin/main` 到 **`262e8c0a`** 零冲突，`--force-with-lease` 推送，SHA 对照 认领 `a41e1669 → f0a454cf`、封存 `3730ef63 → 3d54438e`；从 `3d54438e` 接着做，先判据 5。此前 in-progress——2026-09-11 21:2x 通道 4 按通道 1 派单 task-c28ddfb5 认领，分支 `mcp4-veclaims04` 基 `02e1dfc4`，树 `D:/tops/idp-parcel-mcp4-veclaims04`；开工第一件事 = 判据 5。此前 ready-for-agent——2026-09-10 通道 4 按通道 1 派单 task-d6660969（用户授权代裁）落 [ADR-0136](../../../docs/adr/0136-claim-rule-resolution-key-is-the-acceptance-time-commercial-resolution-reference.md)，四问全裁，「要做什么」按裁决改写；此前 draft（2026-09-04 随票 03 立）
Blocked by: 无（[03](./03-claim-deadline-and-materials-read-party-commercial-rule-content.md) 已 resolved；PS 回指窄读口 `CommercialResolutionReferenceView` 已在 main，见 ADR-0133 Consequences 点名的 ps-port-remainder/07）

## 事实

- 票 03 把索赔资格两维接上了 PC 客户服务规则正文，解析走 PC 既有闭包。闭包键要租户、客户账户、
  责任法人候选、商业范围、目的、锚点（时刻 + 锚点策略版本）与必需依据；`EligibilityQuery` 只有
  前两项。缺的三项与锚点全是实例半边，票 03「裁决」据此在适配器包内立了 `RuleResolutionKeySource`
  接口，nil / formed=false 即显式未配置，两维如实答未登记。
- `cmd/parcel-api` 的 `buildClaimEligibilityRules` 今天传 nil：没有任何租户登记过这份映射，也没有
  登记面。后果是生产路径上 PC 登了正文两维也仍答未登记（装配测试 `TestTheWiredClaimsReadCustomerServiceRulesFromPartyCommercial`
  最后一格钉的正是这一点），只有测试经注入的 SYN 键来源走得到「已登记」态。
- PS 侧同一形状的缝已有登记面：`parcel_shipment.commercial_resolution_key` 表、
  `pspostgres.NewCommercialResolutionKeyStore`、`pspartycommercial.CommercialResolutionKeys`，经
  `parcel-commercial` CLI 登记（syn-wall-door-audit 票 03 件 2）。VE 不能读那张表（跨上下文表），
  且它登的是接受控制的键，不是索赔规则选用的键。

## 要裁的（已裁，见下节「裁决」）

- **锚点从哪来。** PS 的登记面把锚点时刻登成一个固定值（AT-PC-018：不拿系统时间顶锚点）。索赔规则
  的选用时点更像一个按事实取的量（索赔提交时刻？包裹收寄时刻？），那是 `AsOfPolicy` 一族的判断
  （UC-PC-002 步骤 6「消费方逐项形成时点值」），要一份 VE 侧的时点语义声明；今天没有。登固定值
  与按事实取，两条路都不该在没有 owner 裁决的情况下选。
- **键按什么维登。** PS 按（租户 + 客户账户）一行；索赔这边合同责任范围引用（`EligibilityQuery.Contract`）
  是不是也该进键，取决于 VE 的合同责任范围引用与 PC 客户合同对象标识是不是同一个标识空间——
  票 03「裁决」明写今天没有文档立过这一条。
- **要不要一并要 `CustomerContractObject`。** 列进必需依据，闭包的 `namedReferencesConfirmed` 会核
  规则版本指名的合同就是采用的那一版；不列则规则与合同的对应无人核。
- **登记入口。** `parcel-ve-register` 今天只登 VE 自己的册；键映射是 VE 自己的实例参数（形状照
  ADR-0025「实例半边协作者的接口留在适配器包内」），登记入口放 `parcel-ve-register` 合适，但票 03
  派单明写它不接跨上下文读——登记键映射不是读，要不要放进去仍要说一句。

## 裁决（2026-09-10，通道 4 按通道 1 派单 task-d6660969；用户授权代裁，VE owner 口径，键形状连 PC owner 口径；落 ADR-0136，越权风险点五条在 ADR 末尾供 owner 复核）

四问的答案是同一句：**索赔资格规则按目标包裹所属委托接受时固定的商业解析回指选用，VE 不自己解析、不登键。**

1. **锚点从哪来 → 按事实取，事实 = 委托接受时刻，且不是 VE 取的——它已经固定在回指所指的 PC 闭包里**（ADR-0136 决定一）。不登固定值、不取索赔提交时刻、不取收寄时刻、不取系统时间。理由：PC CONTEXT「委托被接受时固定其适用的…」；索赔资格规则是卖出去的服务形态的一部分（ADR-0104、ADR-0133 决定四分族），按提交时刻选会让客户拿到一份下单时不存在的规则。VE CONTEXT「证据、客户索赔与追偿」Rules 已加时点语义一句（随 ADR 同笔）。
2. **键按什么维 → 键 =（租户，商业解析回指）；回指由 PS 按（租户，包裹身份）答（`CommercialResolutionReferenceView`，ADR-0130 / 0133 决定二同一句）；合同由 PC 闭包解出（ADR-0080），VE 不登合同维、不翻译合同标识**（ADR-0136 决定二）。「VE 合同责任范围引用与 PC 客户合同对象标识是否同一标识空间」那道题因此消掉。`EligibilityQuery.Contract` 留着给 VE 自己两本册用，不拿它去 PC 过滤（ADR-0079 决定二同判据）。
3. **`CustomerContractObject` → 必须在接受时闭包的必需依据里；`CustomerServiceRuleObject` 同样**（ADR-0136 决定三）。闭包在场却未采用客户合同版本 → error（ADR-0133 决定二同格）；未采用客户服务规则版本 → 两维未登记，恢复动作是去 PS 解析键登记面把它列进必需依据（与正文未登记同格不同因，注释分开写）。「规则版本指名的合同 = 采用的那一版」由 PC 闭包解析时的 `namedReferencesConfirmed` 守，VE 不重核。能力边界：PS `commercial_resolution_keys.go` 今天只在请求结算依据时强制合同维（`validateSettlement`），要不要改「必登」归 PS owner（ADR-0136 越权风险点 2）。
4. **登记入口 → VE 侧不立键登记面；`parcel-ve-register` 不加命令**（ADR-0136 决定四）。票面原预设的五样（必需依据集合、锚点策略版本、商业范围、目的、责任法人候选）在决定一 / 二之下全是接受时闭包已固定的东西，VE 再登一份即第二次解析（ADR-0079 决定五）。实例半边落在 PS 既有的 `commercial_resolution_key` 登记面（经 `parcel-commercial` CLI）：那一行的必需依据要含 `CustomerServiceRuleObject`。派单方按 A 类裁的「放 `parcel-ve-register`」随之消解；若 owner 复核后回到「VE 自登键」的形，那条裁定照旧适用（ADR-0136 Alternatives 末条）。

5. **实施中追裁（2026-09-11 23:3x，通道 1 推送方裁、通道 3 写入；起因是通道 3 的中途报）**：ADR-0136 越权风险点 2 的前提「客户服务规则可登、只是不必登」在 `262e8c0a` 上**不成立，是不可登**——`CUSTOMER_SERVICE_RULE` 在 `migrations/parcel_shipment` 与 PS `adapters/partycommercial` 零命中：CHECK `commercial_resolution_key_registration_bases_closed`（0007 立、0008 重加）白名单没有它，`commercialKindFrom` 名集也没有；即便行进了库，接受流成键也会报 unknown kind。两道闸是 PS 地盘，本票不动，另立 [ps-port-remainder/09](../../ps-port-remainder/issues/09-resolution-key-face-does-not-accept-customer-service-rule.md)（两道闸 + 是否必登归 PS owner，已进 main）。本票的装配测试因此：「已登记」态的键**直给** PC 真 `ResolveCommercialBasisHandler` + 真 `NewCommercialResolutions` 解出并固定闭包，PS 真 `ShipmentRequests` 照包内夹具同形 Decide → Save、决定上 resolutionId 指它，头注写明为何不经 PS 登记面与「生产路径在 PS 开闸前到不了这一格」；「未采用客户服务规则 → 未登记」态真走 PS 登记面（只含 `CUSTOMER_CONTRACT`）。判据 3 随之改口（原句划掉留痕）。**候选后继（归 ADR-0136 owner）**：越权风险点 2 要补一句 `262e8c0a` 实测——「可登」应为「不可登」，本票不改 ADR。 推送方另记：不取 (b) 挂起——机制半边（VE 读回指 → 闭包 → 正文）与 PS 开闸互不依赖，挂起只是把已验的机制压在别人的票后面。

**与派单预期的一处出入，已在 ADR-0136 越权风险点 4 单列**：派单原话预期 VE 登记面仍登五样；本裁决量出那五样全在闭包里，故不立。owner 若认为索赔规则应按与接受不同的商业范围 / 目的选用，改 ADR-0136 决定二 / 四与本票「要做什么」，决定一不变。


## 要做什么（按裁决写实；形照 TF `internal/transportfulfillment/adapters/partycommercial/delivery_condition_source.go` 那只两个提供方的消费侧适配器）

1. **VE→PS 回指读缝**：在 `internal/visibilityexception/adapters/partycommercial/` 包内立窄接口 `CommercialResolutionReferenceSource`（签名照 PS `ports.CommercialResolutionReferenceView.LoadCommercialResolutionReference`，只 import PS `domain` 不 import PS `application` / `ports`，判据同 TF 那一只的头注）；真实装配交 PS `adapters/postgres` 里实现该 view 的那一只。VE 把 `EligibilityQuery.Target` 原样作 `psdomain.DeclaredParcelID` 问它，不猜目标是不是包裹；PS 答「没有」→ 两维未登记（与今天 `Keys == nil` 同一行为）；PS 答 error → error。
2. **`ClaimServiceRules` 改三段**：回指 → PC `CommercialResolutionView.LoadResolution(tenant, resolution)` 取闭包 → `AdoptedFor(CUSTOMER_SERVICE_RULE)` 取版本 → 既有 `LoadCustomerServiceRule` 点读 → 既有 `translateRule`。结果代数逐格（ADR-0136 决定二第三段）：PS 没有 → 未登记；闭包不在场 → error；未采用客户合同版本 → error；未采用客户服务规则版本 → 未登记（注释写「去 PS 解析键登记面列进必需依据」）；正文 found=false → 未登记（票 03 原格）。**退役** `RuleResolutionKeySource`、`serviceRuleDimensions` 的键形成段、`adoptedRuleVersion`（第一阶段各结局分派）、`requiresCustomerServiceRule`、`ErrCustomerServiceRuleUnresolved`；`ErrUntranslatableAnswer` 留给「闭包采用了别的类别冒名 / 引用译不进构造门」那几格。包注释与 `RuleResolutionKeySource` 头注里「实例半边键来源」那段改口，不留旧话。
3. **装配** `cmd/parcel-api/assemble_claims.go`：`buildClaimEligibilityRules` 去掉 `Keys` 参数，改收 PS 回指读口与 PC `CommercialResolutionView`；nil 半边在装配期拒（ADR-0079 决定八）。`buildClaimsOrchestrationWith(db, nil)` 那条 `nil` 随之消失。
4. **装配测试**：`syntheticRuleKeys` 退役；「已登记」态改为——经 PS 登记面（`pspostgres.NewCommercialResolutionKeyStore` 或 `parcel-commercial` 登记用例）登一行含 `CustomerServiceRuleObject` 与 `CustomerContractObject` 的 SYN 解析键、走一次合成接受形成回指与闭包、按目标包裹读到两维 `Registered=true`；「未登记」态两格：目标不属任何已接受委托（PS 没有）、闭包未采用客户服务规则（键不含该类别）。生产装配那一格照旧钉「行为一字不变」。
5. **不做**：迁移、VE postgres 行搬运、`parcel-ve-register` 命令——三件随裁决 4 取消。

## 完成判据（非作者评审逐项对）

1. `git grep -n RuleResolutionKeySource -- internal/ cmd/` 零命中；`ErrCustomerServiceRuleUnresolved` 零命中。
2. 适配器用例：PS 没有 / 闭包不在场 / 未采用合同 / 未采用客户服务规则 / 正文未登 / 已登记六格各一例，逐格对着 ADR-0136 决定二第三段；租户不符仍报 `ErrUntranslatableAnswer`。
3. 装配测试对真库钉两态，且 ~~SYN 解析键那一行的必需依据含 `CustomerServiceRuleObject` 与 `CustomerContractObject`~~ **闭包采用依据含 `CustomerServiceRuleObject` 与 `CustomerContractObject`（键直给 PC 真解析器解出并固定，不经 PS 登记面）**；生产装配不变那一格照旧。**改口（2026-09-11 23:3x 通道 1 裁、通道 3 写入）**：原句在 `262e8c0a` 上做不到——PS 解析键登记面不收 `CUSTOMER_SERVICE_RULE`（迁移 `parcel_shipment/0008` 的 CHECK `commercial_resolution_key_registration_bases_closed` 白名单与 PS `commercialKindFrom` 名集两道闸），归 PS 票 [ps-port-remainder/09](../../ps-port-remainder/issues/09-resolution-key-face-does-not-accept-customer-service-rule.md)；「未采用客户服务规则 → 未登记」那一态则**真走** PS 登记面（只含客户合同）+ PC 真解析器，那是今天生产路径唯一到得了的结局。
4. `claim_service_rules.go` 包注释与 `cmd/parcel-api/assemble_claims.go` 装配注释改口；`buildClaimEligibilityRules` 签名上没有 `Keys`。
5. **开工第一件事**：核 PC `adapters/postgres/commercial_resolution.go` 闭包快照是否落已采用的客户服务规则版本并读得回（ADR-0136 越权风险点 3）——不对称即先报 PC 地盘，不在本票里绕。
6. `gofmt -l` 空、`go build` / `go vet` 退 0、VE 全包与 `cmd/parcel-api` 带 DSN 绿；机制清点 tip 重生成（跨上下文消费缝 VE→PS +1）。

## 地盘

`internal/visibilityexception/adapters/partycommercial/`（`claim_service_rules.go` 三段改写、新窄接口 `CommercialResolutionReferenceSource` 及其用例；退役件同包）、`cmd/parcel-api/assemble_claims.go`（`buildClaimEligibilityRules` 签名与装配注释）+ 其装配测试（`syntheticRuleKeys` 退役、两态用例）。**不动** `internal/partycommercial/**`（判据 5 核出不对称时先报 PC 地盘，不在本票绕）、`internal/parcelshipment/**`（回指读口 `CommercialResolutionReferenceView` 与 `pspostgres` 实现只消费不改）、VE 两本册与 `parcel-ve-register`、迁移。共享接线文件：`cmd/parcel-api/assemble_claims.go` 今天只有本票动，若同期有人动 `cmd/parcel-api` 其他装配文件互不相干；装配测试若要经 `parcel-commercial` 登记用例登 SYN 解析键，只调用不改它。（2026-09-11 通道 1 推送方补：票面此前缺本节，派单前补齐；内容从「要做什么」1–4 抄出，不加宽。）

## 边界

- 不动 `party-commercial`；不改 VE 两本册；不在任何一处拿系统时间或默认范围顶键。
- 不替明确服务范围为目标的索赔项选键（ADR-0136 越权风险点 1）；它两维照旧未登记。
- 不改 PS 解析键登记面的校验强度（ADR-0136 越权风险点 2 归 PS owner）。

## 完成记录

（通道 4 起手 21:2x–21:4x，通道 3 接手 23:2x–23:4x · task-6afdcc42 · 树 `D:/tops/idp-parcel-mcp4-veclaims04`，分支 `mcp4-veclaims04` 基远端 main `262e8c0a`。）

**逐笔**：认领 `f0a454cf`（通道 4，原 `a41e1669`）；封存 `3d54438e`（推送方 21:45 封存通道 4 未提交现场，原 `3730ef63`；`git rebase origin/main` 到 `262e8c0a` 零冲突后 `--force-with-lease` 换号）；接手笔 `5cead8f5`；代码 + 本完成记录同一笔（SHA 见完工报）。

**谁做了什么**：封存件（通道 4）——`claim_service_rules.go` 三段改写 + 两只新哨兵 + 退役四件、`claim_service_rules_test.go` 重写、`assemble_claims.go` 的 `buildClaimEligibilityRules` 去 Keys 改收 PS 回指读口（`pspostgres.NewShipmentRequests`）与 PC 闭包读口（`pcpostgres.NewCommercialResolutions`），这三件接手时已按「要做什么」1–3 落齐，通道 3 一字未改。通道 3——按 parallel-sessions.md「镜像测试」先写自己的第一片 red `claim_service_rules_resolution_test.go`（六格 + 租户不符 + 两提供方 error + nil）再读封存实现：判据逐格同形，连哨兵名都一致（`ErrCommercialClosureAbsent` / `ErrCustomerContractNotAdopted`），形上只差 deps 字段名（`References`），对齐后两份测试对同一实现全绿——留作交叉验证，不删；`assemble_claims_test.go` 从未编译的半成品改写为三态用例（态零「PS 没有」、态一「闭包未采用客户服务规则」、态二「已登记」）+ 四个夹具（`secondSubmissionCommand` / `resolveClosureOnRealAssembly` / `acceptOnRealAssemblyWith` / `effectiveCommercialShell`），退役 `syntheticRuleKeys` 与 `buildClaimsOrchestrationWith` 的引用。

**判据 5（开工第一件事）· PC 闭包快照读写对称**：**对称。** `internal/partycommercial/adapters/postgres/commercial_resolution.go` `documentOfClosure` 对 `closure.Adopted()` 逐项通写 `kind` + 整份 `versionDocument`，不按类别筛，只有 ServiceProduct / SettlementPolicy / CreditBasis 三类附件按在场追加；`closureDocument.closure()` 逐项通读成 `RehydrateAdoptedBasisSpec` 并以已采用的 kinds 重建 `RequiredBases`；`RehydrateCommercialClosure` 只对那三类附件做类别校验，客户服务规则无附件——`AdoptedFor(CustomerServiceRuleObject)` 读得回。真库实证就是装配测试态二：闭包经 PC 真解析库 `Save`，VE 适配器经同一库 `LoadResolution` 读回并取到 `SYN-CSR-1/v1`。

**判据逐项**：
1. ✓ `git grep -n RuleResolutionKeySource -- internal/ cmd/` 零；`ErrCustomerServiceRuleUnresolved` 零。
2. ✓ 适配器用例六格各有：PS 没有 / 闭包不在场（`ErrCommercialClosureAbsent`）/ 未采用合同（`ErrCustomerContractNotAdopted`）/ 未采用客户服务规则（未登记，不点读）/ 正文未登（未登记）/ 已登记；租户不符（闭包属另一户）→ `ErrUntranslatableAnswer`。两份文件各一套（封存 `claim_service_rules_test.go` + 交叉验证 `claim_service_rules_resolution_test.go`）。
3. **✓（按 23:3x 改口后的判据）**：真库三态钉住——态零 PS 没有 → 未登记、编排停 `ELIGIBILITY_FILING_DEADLINE_NOT_REGISTERED`，这就是生产装配今天的行为，与 Keys=nil 一字不变；态一 **真走 PS 登记面**：只含 `CUSTOMER_CONTRACT` 的一行登进 `commercial_resolution_key_registration` → `CommercialResolutionKeys.FormResolutionKey` 成键 → PC 真 `ResolveCommercialBasisHandler` 对真发布登记册解出、真 `NewCommercialResolutions` 固定闭包（只采用客户合同）→ PS 真 `ShipmentRequests` Decide → Save 接受委托二、决定上 resolutionId 指它 → 两维未登记；态二 **键直给** PC 真解析器（必需依据含客户合同 + 客户服务规则，不经 PS 登记面）→ 闭包采用 `SYN-CSR-1/v1`（断言 `AdoptedFor` 命中、且与态一闭包解析标识不同）→ 接受委托一 → PC 登正文 → 两维 Registered、`SYN-TENANT-1/SYN-CSR-1/v1`、Required 逐项相等、留格三样零值，编排差材料 → `SUPPLEMENT_DEADLINE_UNDERIVABLE`、材料齐 → `FILING_DEADLINE_UNDERIVABLE`；委托二的索赔不借委托一的规则。对真登记面另钉负断言：含 `CUSTOMER_SERVICE_RULE` 的一行登不进去（判断项 1）。
4. ✓ 包注释与 `buildClaimEligibilityRules` 头注改口（封存件已写）；签名 `buildClaimEligibilityRules(db *bentopg.DB)`，无 Keys、无 clock。
5. ✓ 见上节。
6. ✓ `gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；带 DSN `go test -p 1 -count=1` VE `adapters/partycommercial` + `cmd/parcel-api` + `internal/architecture/...` 全 ok（23:37，55432 占 / 释已报通道 1）；无 DSN 同三包亦 ok。机制清点由推送方在干净检出重生成（跨上下文消费缝 VE→PS +1），本记录不预报数字。

**红线核**：`git diff --name-only 262e8c0a` 只有 `internal/visibilityexception/adapters/partycommercial/{claim_service_rules.go,claim_service_rules_test.go,claim_service_rules_resolution_test.go}`、`cmd/parcel-api/{assemble_claims.go,assemble_claims_test.go}` 与本票面；`internal/partycommercial/**`、`internal/parcelshipment/**`、VE 两本册、迁移、`parcel-ve-register` 零改。适配器不拿系统时间、不带默认范围：`buildClaimEligibilityRules` 里没有时钟、没有键。

**判断项**（请评审与 owner 裁）：
1. **「已登记」态不经 PS 登记面**（`262e8c0a` 实测，两道闸；推送方 23:3x 已裁 (a)，PS 票 [ps-port-remainder/09](../../ps-port-remainder/issues/09-resolution-key-face-does-not-accept-customer-service-rule.md) 已进 main）：迁移 `migrations/parcel_shipment/0008_resolution_key_settlement_selector.sql` 的 CHECK `commercial_resolution_key_registration_bases_closed` 白名单没有 `CUSTOMER_SERVICE_RULE`（0020 未改这条）；PS `adapters/partycommercial/commercial_resolution_keys.go` `commercialKindFrom` 的名集也没有 `CustomerServiceRuleObject`——即便行进了库，接受流 `FormResolutionKey` 也会报 unknown kind。所以 ADR-0136 越权风险点 2 写的「客户服务规则可登」在今天不成立，是**不可登**；生产上在 PS owner 开这两道闸之前，没有任何租户能让接受时闭包采用客户服务规则，这条缝恒停在「未采用客户服务规则 → 未登记」——态一钉的正是这一格。态二的键因此直给 PC 真解析器；用例头注用符号名写明两道闸与「生产路径在 PS 开闸前到不了这一格」。装配测试里的负断言在 PS 开门那天会翻红——那是有意的：翻红的处置写在用例头注（态二改为经登记面登、走接受形成）。反方：一条会因别人的正确改动翻红的断言，评审若不接受，去掉负断言只留正断言即可，别的不动。
2. 两份适配器测试并存（封存的 `claim_service_rules_test.go` 用 PC 真解析器 `ResolveCommercialClosure` 造闭包；交叉验证的 `claim_service_rules_resolution_test.go` 用重建门造）：按「镜像测试」纪律保留，判据同、切法不同（一份证闭包由提供方形状定义，一份证快照读回形状）。若评审认为重复，删交叉验证那份，封存那份已覆盖六格。
3. 装配测试两份闭包都由 PC 真 `ResolveCommercialBasisHandler` 解出（按裁决条件 ①），不用同包 `seedAdoptedClosure` 那种重建门造法；代价是接受夹具要把回指开成参数（`acceptOnRealAssemblyWith`），`acceptedOnRealAssembly` 的固定 `SYN-RES-1` 本票不动、不再被本用例引用。

## Comments

- 2026-09-04 MCP-4：随票 03 立（draft）。票 03 落地时生产装配 Keys=nil、行为与之前一字不变；本票
  是让那一格真正点亮的那一步。
- 2026-09-10 · 通道 4（task-d6660969，基 `062f5228`，分支 `mcp4-adr0136`）：四问经用户授权代裁落 ADR-0136，本票「要做什么」按裁决改写、Blocked by 改无、Status → ready-for-agent。**只改 .md，未动代码。** 与派单预期的出入（VE 登记面不立）见「裁决」末段与 ADR-0136 越权风险点 4。
- 2026-09-11 14:1x · 通道 1 推送方：票面缺「地盘」节（11:2x 节记为不派的原因），从「要做什么」1–4 抄出补齐，不加宽；Status 不变（ready-for-agent），下一波可派。**只改 .md，未动代码。**
- 2026-09-11 23:4x · 通道 3（task-6afdcc42，接通道 4 封存 `3d54438e`）：作者完工。判据 5 对称；判据 1 / 2 / 4 / 6 全过；判据 3 原句「解析键含 CSR」做不到——PS 登记面两道闸不收 `CUSTOMER_SERVICE_RULE`（PS 地盘，中途报推送方，23:3x 裁 (a)、改口写入「裁决」5 与判据 3、PS 票 ps-port-remainder/09 已进 main）：态一真走 PS 登记面 + PC 真解析器，态二键直给 PC 真解析器，真库三态全绿。分支已推 origin；等非作者评审后重放。
- **评审 ← 通道 1（推送方，非作者；作者通道 3 / 封存件作者通道 4 已无会话，无他人空闲）· 钉 `3dc07230` · 23:4x**。对象 `git diff 262e8c0a 3dc07230 -- cmd/ internal/`：生产两件逐行读，测试三件按用例名与断言抽读（`assemble_claims_test.go` 三态全读），只读，未跑（验证由重放 tip 全量闭合，见下）。
  - **Standards**：阻断 无。非阻断 ① `cmd/parcel-api/assemble_claims.go` `buildClaimEligibilityRules` 头注「四个协作方缺一即装配失败」数的是另一文件 `ClaimServiceRulesDeps` 的字段数（AGENTS.md「计数与行号同构」；同文件 `claim_service_rules.go` 里那句「收拢四个协作方」是同文件计数，不算）——去数词零损失，归 VE owner 随下一张注释小票。② `NewClaimServiceRules` 四道拒 nil 各用裸 `fmt.Errorf`、不包哨兵——沿用了该文件既有写法（票 03 时就如此），装配方按 `errors.Is` 分不出「装配漏了」；与 sa-cc/16 ① 同族，归 VE owner。无发现：`CommercialResolutionReferenceSource` 只 import PS `domain`（ADR-0025，判据同 TF 交付条件适配器）；结果代数逐格对 ADR-0136 决定二第三段与决定三——PS 没有 → 未登记、PS error 原样上抛、在场却空回指 → `ErrUntranslatableAnswer`、闭包不在场 → `ErrCommercialClosureAbsent`、结局非唯一已解析 / 租户或回指不符 / 规则槽冒名 → `ErrUntranslatableAnswer`（ADR-0003 租户是身份）、未采用合同 → `ErrCustomerContractNotAdopted`、未采用客户服务规则 → 未登记并注明恢复动作是 PS 登记面、正文 found=false → 未登记（票 03 原格）；没有时钟、没有键、没有默认范围；回指经 `pcdomain.NewResolutionID(reference.String())` 不拆不拼（ADR-0080 决定七）；退役五件干净（`RuleResolutionKeySource` / `ErrCustomerServiceRuleUnresolved` 于 `internal/` + `cmd/` 零命中）；注释中文、无行号、引 ADR 用决定号、PC CONTEXT 引文单行可搜。
  - **Spec**：阻断 无。非阻断 ① 判据 3 改口（本条上方已留痕）——用例对真登记面钉负断言「含 CSR 的行登不进去」是有意的绊线，PS 开门那天翻红；**裁：留着**，并把翻线的处置写进 ps-port-remainder/09 的做法与地盘（那张票进 main 时顺手把这一断言改成经登记面登、走接受），而不是删掉——删了之后没有任何东西会提醒人把夹具改回正路（作者判断项 1）。② 判断项 2 两份适配器测试并存：留，按「镜像测试」纪律它们证的是同一判据的两种切法，不是重复；评审只对判据不对条数。③ 判断项 3 装配测试用重建门造闭包：接受，同包 `seedAdoptedClosure` 先例同形；闭包由解析而来那一层 PC 自己的包与封存测试已证。无发现：判据 1 ✓、2 ✓（六格 + 租户不符 + 两提供方 error + nil，两份各一套）、3 三态 ✓（态零就是生产装配今天的可观察行为，与 Keys=nil 一字不变；态一 / 态二各有编排侧停格）、4 ✓、5 ✓（对称，且态二是真库实证）、6 由推送方全量闭合；红线：`internal/partycommercial/**`、`internal/parcelshipment/**`、两册、迁移零 diff（`git diff --name-only 262e8c0a 3dc07230` 只有 VE `adapters/partycommercial` 三件 + `cmd/parcel-api/assemble_claims*` + 本票面）。
- **进 main 记录（通道 1 推送方，23:4x–23:5x）**：`%TEMP%\idp-replay-veclaims04` detached main `aa2e6a07`，`cherry-pick 262e8c0a..mcp4-veclaims04` 四笔零冲突（主干在分支基 `262e8c0a` 之后只多 ps-port-remainder/09 立票一笔 .md）→ `bf722146`，本票文件对分支 tip 零差；`gofmt -l` 空、`go build ./...` / `go vet ./...` 0；清点在 tip 重生成 → 推送方清点笔 **`637245e7`**（VE 测试 92→93，合计 918→919；其余零差——VE→PS 窄口落在 `adapters/partycommercial` 包内，清点按目录归 VE→PC 缝不新增一组，票面判据 6 预报的「VE→PS +1」按生成器为准不成立，如实记）；23:45 占号，`637245e7` 带 DSN 全量 **110 ok / 0 FAIL / 15 无测试 / 0 cached**（23:45:02→23:47:12），`-v` 探针 `cmd/parcel-api` + VE `adapters/partycommercial` PASS 40 / SKIP 0 / FAIL 0（`TestTheWiredClaimsReadTheRuleAdoptedAtAcceptanceThroughParcelShipment` 三态真库实跑，含对 PS 登记面的负 / 正断言）；23:47 释号。SHA 对照：`f0a454cf→943d6f2c` / `3d54438e→e4814324` / `5cead8f5→876e4280` / `3dc07230→bf722146`；推送方清点 `637245e7`。`mcp4-veclaims04` → `merged/mcp4-veclaims04`（指针留，远端删），`D:/tops/idp-parcel-mcp4-veclaims04` 比内容后拆。**候选后继**：VE owner——`assemble_claims.go` 头注「四个协作方」计数、`NewClaimServiceRules` 拒 nil 包哨兵、ADR-0136 越权风险点 2 补一句 `262e8c0a` 实测；PS owner——[ps-port-remainder/09](../../ps-port-remainder/issues/09-resolution-key-face-does-not-accept-customer-service-rule.md)（两道闸 + 必登否 + 翻本票装配用例那条负断言）。
