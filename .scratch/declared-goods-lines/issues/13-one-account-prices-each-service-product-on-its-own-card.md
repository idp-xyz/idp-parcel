# 13 同一账户的不同服务产品各用各的售价卡

Category: enhancement
Status: in-progress
Blocked by: 14、15、16、17（「要定的」两项已由 [ADR-0378](../../../docs/adr/0378-a-sell-price-card-may-name-the-service-products-it-prices-and-an-unnamed-card-prices-the-rest.md) 定下，实现拆成这四张）

依据：票 12 评审（2026-11-11）；[ADR-0260](../../../docs/adr/0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md)（评价按范围选卡、计价条款只从价卡读）；[ADR-0376](../../../docs/adr/0376-fba-submission-conditions-are-judged-as-required-documents-platform-identifiers-may-be-supplied-before-acceptance-and-rule-packages-follow-the-service-product.md) 决定三（一个货主账户用几个服务产品）。

## 现状

- 接受前试算与评价按（租户，定价范围，方向，目的，时点）解出在用价卡，没有服务产品一维。定价范围取自结算账户的采用记录，一个账户一个范围。
- 于是同一账户下的快递小包与 FBA 头程只能共用一张售价卡。另登一张同范围的售价卡解成适用冲突；登在别的范围，这个账户的委托永远选不到它。
- 演示租户 `SYN-TENANT-01` 的 FBA 产品因此沿用按件计重的快递卡（票 12 完成记录）。头部系统里两者计价方式不同：FBA 头程常按整票计费重、按公斤段计价，快递按件。

## 要定的

- 选卡加服务产品一维（卡声明适用的服务产品，评价请求带上申报的产品），还是让一个账户按产品采用几个定价范围。两者都碰 ADR-0260 的「按范围选卡」与结算的采用记录，属难逆转的取舍，先写 ADR。
- 没声明产品的既有价卡怎么解：照旧对全部产品适用，还是只在没有更具体的卡时适用。

## 不做的

价格方向、成本侧的选卡（同一问题，等售价侧定了再照办）；演示数据里的整票计费卡（ADR 落地后随演示另立）。

## 决定（2026-11-11）

按头部系统的做法定，写成 [ADR-0378](../../../docs/adr/0378-a-sell-price-card-may-name-the-service-products-it-prices-and-an-unnamed-card-prices-the-rest.md)：价格等级是客户的，价格表是产品的。

- 选卡在计价范围之外加服务产品一维。计价范围仍是客户这一侧的价格分层，结算账户只采用一个 SELL 范围，不按产品分范围。
- 售价卡可以声明它计价的服务产品；不声明的卡接其余产品——这正是既有卡的原义，不用重登。指名卡与不声明的卡在同一个产品上不同时适用，计价照旧不挑卡。
- 产品取委托的申报；接受前估价、正式 SELL 评价与试算三条路同一笔改。
