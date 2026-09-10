# VE 词到 PC 闭包键的翻译缺登记面——生产装配的 `RuleResolutionKeySource` 今天只能留 nil

Category: enhancement
Status: ready-for-agent——2026-09-10 通道 4 按通道 1 派单 task-d6660969（用户授权代裁）落 [ADR-0136](../../../docs/adr/0136-claim-rule-resolution-key-is-the-acceptance-time-commercial-resolution-reference.md)，四问全裁，「要做什么」按裁决改写；此前 draft（2026-09-04 随票 03 立）
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
3. 装配测试对真库钉两态，且 SYN 解析键那一行的必需依据含 `CustomerServiceRuleObject` 与 `CustomerContractObject`；生产装配不变那一格照旧。
4. `claim_service_rules.go` 包注释与 `cmd/parcel-api/assemble_claims.go` 装配注释改口；`buildClaimEligibilityRules` 签名上没有 `Keys`。
5. **开工第一件事**：核 PC `adapters/postgres/commercial_resolution.go` 闭包快照是否落已采用的客户服务规则版本并读得回（ADR-0136 越权风险点 3）——不对称即先报 PC 地盘，不在本票里绕。
6. `gofmt -l` 空、`go build` / `go vet` 退 0、VE 全包与 `cmd/parcel-api` 带 DSN 绿；机制清点 tip 重生成（跨上下文消费缝 VE→PS +1）。

## 边界

- 不动 `party-commercial`；不改 VE 两本册；不在任何一处拿系统时间或默认范围顶键。
- 不替明确服务范围为目标的索赔项选键（ADR-0136 越权风险点 1）；它两维照旧未登记。
- 不改 PS 解析键登记面的校验强度（ADR-0136 越权风险点 2 归 PS owner）。

## Comments

- 2026-09-04 MCP-4：随票 03 立（draft）。票 03 落地时生产装配 Keys=nil、行为与之前一字不变；本票
  是让那一格真正点亮的那一步。
- 2026-09-10 · 通道 4（task-d6660969，基 `062f5228`，分支 `mcp4-adr0136`）：四问经用户授权代裁落 ADR-0136，本票「要做什么」按裁决改写、Blocked by 改无、Status → ready-for-agent。**只改 .md，未动代码。** 与派单预期的出入（VE 登记面不立）见「裁决」末段与 ADR-0136 越权风险点 4。
