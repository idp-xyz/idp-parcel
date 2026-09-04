# VE 词到 PC 闭包键的翻译缺登记面——生产装配的 `RuleResolutionKeySource` 今天只能留 nil

Category: enhancement
Status: draft
Blocked by: [03](./03-claim-deadline-and-materials-read-party-commercial-rule-content.md)（立缝的那一票；缝在 `internal/visibilityexception/adapters/partycommercial`）

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

## 要裁的

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

## 要做什么（裁完之后）

- `migrations/visibility_exception/00NN_claim_rule_resolution_key.sql`：一张登记表，形状参照
  `parcel_shipment.commercial_resolution_key`（必需依据集合、锚点、范围、法人候选；CHECK 镜像领域
  封闭集），**不承载合同维**若裁定合同由闭包解出。
- `internal/visibilityexception/adapters/postgres/` 一个 `ResolutionKeyStore` 形状的行搬运（只有基本
  类型，判据同 PS：SQL 归 postgres 适配器，翻译归 adapters/partycommercial）。
- `internal/visibilityexception/adapters/partycommercial/` 一个实现 `RuleResolutionKeySource` 的登记面
  半边，必需依据必含 `CustomerServiceRuleObject`（适配器已把缺它判为 `ErrUntranslatableAnswer`）。
- 受控登记口一节；`cmd/parcel-api` 把 `buildClaimEligibilityRules` 的 nil 换成真登记面。
- 装配测试里 `syntheticRuleKeys` 退役，换成经登记面登一行 SYN 键。

## 边界

- 不动 `party-commercial`；不改 VE 两本册；不在任何一处拿系统时间或默认范围顶键。

## Comments

- 2026-09-04 MCP-4：随票 03 立（draft）。票 03 落地时生产装配 Keys=nil、行为与之前一字不变；本票
  是让那一格真正点亮的那一步。
