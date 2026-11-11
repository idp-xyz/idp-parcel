# 13 同一账户的不同服务产品各用各的售价卡

Category: enhancement
Status: needs-triage
Blocked by: 一篇待写的 ADR（按什么选卡：定价范围之外加服务产品一维，或按产品分范围；见下）

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
