# ADR-0048: 声明测量是成员级保真画像，不换算不推导

Status: Accepted  
Date: 2026-08-12

## Context

UC-PS-001 声明包裹输入组：「一个或多个客户声明成员及各自客户侧引用、**声明测量**、货物
和服务资料」。PS CONTEXT 把客户声明的任务级快照判给 `parcel-shipment`，同时明写
`node-operations` 拥有节点实测、`parcel-pricing` 形成计价重量（SET-01）。

今天 PS 对声明测量**零建模**（成员只有 `DeclaredParcelID`）。这挡住两条链：接受前估价
的金额缝（`ControlAmountSource` 的装配缺输入——PP 评价的计价输入要从声明测量译出），
与 PN-03 的测量复核链（节点实测 vs 客户声明，词汇缺一半）。

## Decision

**一、声明测量按成员建模为保真画像。** `DeclaredMeasurement` = 毛重（十进制字符串 +
单位引用）+ 可选外廓尺寸（长/宽/高 + 单位引用）。**保真**指 PS 原样保全客户申报的数字
与单位：不换算单位、不算体积重、不折浮点——换算与计价重量是 `parcel-pricing` 的规则，
PS 改写一次数字就成了第二个测量来源；数值只做「合法正十进制」的形状校验。

**二、画像挂在提交版本的成员声明上。** `DeclaredParcelProfile` 把 `DeclaredParcelID`
与其声明测量配对，随提交版本保全（UC-PS-001：声明测量属客户原始提交）。测量必填与否由
真实产品与关务区域决定（用例原句），机制不写死必填——画像缺席合法，读取方各自决定缺席
的后果（估价装配停在未形成，而不是这里拒收提交）。

**三、分增量落地。** 首增量：值对象与画像（本记录随行落地）；第二增量：提交候选/提交
版本携带画像并过重建门（聚合形状变更连锁 `revision_test` 与重建面门禁）；第三增量：
金额缝装配器经 `DeclaredMeasurementSource` 读画像译 PP 评价输入——结构化提取契约属
`PAR-INT-01` 实例半边，装配器对缺席停 `CONTROL_AMOUNT_NOT_CONFIGURED`。

## Consequences

- 金额缝的输入词汇就位；PN-03 测量复核（声明 vs 实测）的 PS 半边词汇同源。
- 保真字符串意味着 PS 不回答「1.5kg 等于多少 g」——问这个问题的读取方去找 PP 的规范化。
- 真实单位目录、必填规则、提取契约全属实例半边（`PAR-INT-01`、`PAR-COM-*`），机制以
  合成声明钉形状。

## Alternatives considered

- **复用 parcel-pricing 的 Dimensions/decimal 类型。** 否决：跨上下文共享领域类型让 PS
  的客户话语随 PP 的计价规范化演化；ADR-0025 的翻译边界正是为此立的。
- **测量放进资料版本内容（UC-PS-002 路线）。** 否决为首选挂点：首次提交就带测量
  （UC-PS-001 输入组），资料版本是**修订**测量的路线，不是它出生的地方；修订路线在
  第二增量后按既有资料版本机制接入。
- **等真实产品定必填字段再建模。** 否决：必填是实例半边，形状是机制半边——分裂正是
  开发主线的红线。

## Links

- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：声明包裹输入组
- [PS CONTEXT](../domain/parcel-shipment/CONTEXT.md)：快照所有权与节点实测/计价重量的边界
- [ADR-0047](./0047-terms-control-forms-credit-exposure-not-a-freeze.md)：金额缝所在的控制链
