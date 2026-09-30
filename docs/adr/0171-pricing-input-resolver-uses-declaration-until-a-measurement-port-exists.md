# ADR-0171：计价输入在没有实测读口时用申报，并标明来源

Status: Accepted（2026-09-30。本项是机制。接受依据是 ADR-0146 决定一。）
Date: 2026-09-30

## Context

`PricingInputResolver` 已有端口，生产装配没有实现。运输履约的发生项成员、小包托运的申报测量与地址要素三只只读口已在。节点实测登记册还没有（`pp-pricing-input-seams/04`）。评价对象四种里没有集运单元。

## Decision

**一、一只消费侧编排器。** 落在 `internal/parcelpricing/adapters/transportfulfillment`。问话的键是运输收费发生项。同一只编排器再问小包托运的申报测量与地址要素。不 import 提供方 `application`。`formEvaluationOnEvaluationRequestConsumer` 把这只编排器装进 `Inputs`。

**二、没有实测读口时用申报。** 实重与尺寸优先取仍有效的实际测量。实测读口缺席，或答没有这条实测，就用客户申报，并在快照事实引用的版本串里以 `declaration` 标明来源。实测在场时，申报的重量与尺寸都不得顶替，尺寸缺了也不回退到申报。不填默认重量、尺寸、邮编或分区。不登租户行。

**三、单位只认计价封闭集。** 申报单位是自由串。只有 `KG` / `G` / `LB` / `OZ` 与 `CM` / `IN` 译得进快照。其余单位答输入不可得，并点名那个单位。

**四、一个已受理包裹才成评价对象。** 发生项恰好一个成员，且小包托运认它是已接受委托的声明包裹，评价对象是已受理包裹，快照走不带调用方分区的邮编路线。成员为零、多于一个、或小包托运答「无」（含集运单元与不可见对象），答输入不可得并点名。不给 `EvaluationSubjectKind` 加第五种。目的邮编必备，起点邮编可缺。分区仍由评价内的目录解出。

**五、计算目的不参与选源。** 某一计算目的是否拒用申报保持未决，本编排器不预设。

## 候选与反方

- **没有实测就停住，不用申报。** 提供方口已经能答申报。否决。
- **给集运单元加一种评价对象。** ADR-0111 的四种不在本项改。否决。
- **接一只永远答「没有实测」的替身。** 缺席就是缺席，替身会把「还没有这只口」说成「问过了、没有」。否决。

## Consequences

- 不新增 `parcel_pricing` 迁移。
- `CONTEXT-MAP` 增加计价到运输履约、计价到小包托运两条消费边。到节点作业的那条等实测登记册。
- 调用方仍可不装 `Inputs`；那种调用方继续点名三只读口。生产装配不再走那一格。

## 越权风险点

1. 编排器目录叫 `transportfulfillment`，却同时读小包托运。归 parcel-pricing 的 owner 复核。
2. 实测事实引用的种类先借申报测量那一格，版本串用 `measurement` 与申报区分。实测登记册落地时再换种类。归 parcel-pricing 的 owner 复核。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [ADR-0111](./0111-shipment-and-mawb-level-billing-units-are-evaluation-subjects-in-parcel-pricing-and-settlement-allocates.md)：评价对象仍是四种
- [ADR-0109](./0109-zip-classification-facts-are-owned-by-parcel-pricing-as-a-versioned-reference-catalogue.md)：分区由目录解，调用方不造
- [parcel-pricing CONTEXT「计价输入快照」](../domain/parcel-pricing/CONTEXT.md)
- 票 `.scratch/pp-pricing-input-seams/spec.md`「不在本目录」第一条
