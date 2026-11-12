# 19 结算两条发起路按合同声明的销售价格来源请计价，两本采用册不再说计价范围

Category: enhancement
Status: draft——由票 [15](./15-one-authority-for-which-price-a-customer-uses-and-the-customer-price-structure.md) 拆出；[ADR-0380](../../../docs/adr/0380-the-customer-contract-version-declares-ordered-sell-price-sources-and-settlement-hands-them-to-pricing-from-the-fixed-resolution.md) 进 `main` 之后由用户激活
父票：[spec](../spec.md)
Blocked by: 票 [17](./17-sell-price-sources-are-fixed-with-the-acceptance-time-closure-and-read-by-settlement.md)、票 [18](./18-pricing-selects-the-price-card-across-an-ordered-scope-list.md)
地盘：`internal/settlementaccounting`（两条发起路、评价请求、停因与续办、两本采用册的登记）、`cmd/parcel-settlement-register`、`docs/application/settlement-accounting`

对应要求：Contract「价卡授权」落到金额上的那一半。规则在结算 `CONTEXT.md` 讲接受前财务控制的那一条，与 ADR-0380 决定二、三、七。

## 现象

取证钉 `54b32c1b8`。受理前估价与包裹计费时点两条路找到结算账户之后，评价请求上的范围都取采用册上的那一格（`PricingScope: adoption.PricingScope()`）。两本册不可变，只有命令行一条登记路；ADR-0280 要求的两本同值的核，代码里没有。

## 要做的

1. **两条路读合同的列表。** 合同版本带销售价格约定时，请求带合同声明的有序列表，不读采用册上的范围。
2. **旧规范化版本的合同照旧。** 没有销售价格约定的合同，照旧读采用册上的范围，请求只带一个。
3. **显式不计销售价。** 包裹计费时点不发起销售评价，这一拍入账、不记停办；受理前估价答`未配置`，不拿零顶。
4. **请求的身份。** 列表进不进评价请求的自然键摘要要定：同一件包裹、同一计费时点只发起一次照旧。
5. **停因与续办。** 停因记下问过的全部范围；「参数已登记」带来的范围在其中即命中收窄。续办出来的下一代照抄前一代的列表，不重走链。
6. **两本采用册的登记面。** 受理前估价采用只必填形态，SELL 计费时点采用只必填计费时点；计价范围可缺。已登的行不删不改。
7. **留痕。** 估价与预估客户费用的留痕带出用的是列表里的哪一个范围。

## 要裁的

- ADR-0280 的那道核还做不做（ADR-0380 越权风险点 8）：票 `docs-consistency/code/62` 是关掉还是只对旧形状照做，待 SA owner 定。
- 一个结算账户下同时有新旧两种形状的合同时，采用册上的范围对新形状的合同不被读。登记面要不要提示这一点。

## 三类

全部是机制。哪个账户采用哪种形态、哪个计费时点，仍是租户取值。

## 完成判据

- 同一件包裹：受理前估价与随后形成的预估客户费用出自同一份列表、同一个范围，金额相同。
- 合同发了新一版、列表不同：已接受的委托下的包裹仍按旧列表计费。
- 列表里前一个范围没有卡时停办不发生，落到后一个；全部落空时停办，任何一个范围登了新卡都续办。
- 旧规范化版本合同的包裹，行为与今天相同。
- 合同声明不计销售价：计费时点不发起、不停办；要估价时答`未配置`。
- `UC-SA-002`「给开发的交接」里对应的那一行随之删去或改成现状。

## Comments

- 2026-11-12：由票 15 拆出。
