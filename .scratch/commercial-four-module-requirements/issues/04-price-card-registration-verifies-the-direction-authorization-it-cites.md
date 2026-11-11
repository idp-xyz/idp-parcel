# 04 价卡登记只记下它引用的价格方向授权，不核对——引一条不存在或方向不符的授权照样入册；登记客户用哪套销售价时也不核对合同

Category: enhancement
Status: draft
父票：[spec](../spec.md)
Blocked by: 无。Q4 已由会话按行业做法代答、待用户认可：不改评价的选卡路径，在两道登记门上核对（见 [spec](../spec.md)「待用户确认」与[行业做法笔记](../industry-practice-notes.md) R16 至 R18）。开工时随票立一篇 ADR：计价第一次读参与方与商业，`CONTEXT-MAP.md` 的 `party-commercial → parcel-pricing` 一条要补
地盘：`internal/parcelpricing`（新增消费侧适配器）、`internal/partycommercial`（新增只读口）、`internal/settlementaccounting`（采用登记的那道门）、`apps/admin-web/src/pages/pricing` 与 `pages/party`

对应要求：Contract「价卡授权」。

## 现象

取证钉 `11bfa1ffb`。

- 价卡登记必须带一条价格方向授权引用（`PriceCardRegistration`），引用的种类必须是 `ArtifactCommercialAuthorization`。那个常量的注释写着「这里只登引用不解析其内容」。
- `internal/parcelpricing/adapters/` 下没有读参与方与商业的适配器，所以没有任何地方核对这条引用。
- 演示种子里，价卡引的是 `SYN-AUTH-PRICE-DIR-01`、`v1`；它在商业种子里是一版授权规则，指名一份商业价格政策。方向写在那份价格政策上。
- 销售与采购方向的商业价格政策绑定一份定价方案（`CommercialPricePolicy` 的 `PricingPlan`），但按范围选卡不读这个绑定，也不与选出的卡核对（[ADR-0260](../../../docs/adr/0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md) 决定一）。
- 一位客户用哪套销售价，由他的结算账户登记的销售计价范围定。[ADR-0190](../../../docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定三写的采用册，键是租户与结算账户，值是计费时点与计价范围，不带合同或价格政策。
- 头部软件里，客户专属价都经一个受管的客户绑定取到（行业做法笔记 R16 至 R18）。本产品的客户绑定就是上面那一步登记，它今天与合同没有关系。

## 要做的

1. **登记时核对**：价卡入册之前核它引用的授权——在 `party-commercial` 是一版已发布的授权规则；登记时刻落在它的有效区间内，且它没被收尾；它授权的方向与价卡的方向相同。
2. **门只有一处**：放在价卡登记用例里，登记口、草稿发布与受控命令行共用，位置照 [ADR-0352](../../../docs/adr/0352-registering-a-price-card-in-a-canonicalization-newer-than-the-base-one-needs-an-explicit-deployment-switch-that-is-off-by-default.md) 决定一。
3. **答复按恢复动作分格**：授权不在册或未发布、已到期或已收尾、方向不符，这三格不入册；读不回是未决。
4. **预览同样核**，提前告诉操作者。
5. **看得见**：价格政策与授权规则的对象页列出引用它的价卡。
6. **第二道门，登记客户用哪套销售价时核对合同**：结算账户登记销售计价范围（受理前估价采用与计费时点采用两本册）时，核这位客户的合同上有没有一版现行的销售方向价格政策，它绑定的定价方案就在这个计价范围上。没有就不登记，并说明缺的是合同、价格政策，还是绑定对不上。
7. **不改选卡**：评价照旧按范围选卡，不读合同，不读绑定。

## 要裁的

- 方向怎么从授权规则读出来。种子里它经指名的价格政策带出方向；授权规则正文的形状取证时没有读，立 ADR 时先核。
- 已在册、引用悬空的旧卡不追溯。重放过不过这道门要裁：ADR-0352 决定四对部署开关取的是「重放也拒」。
- 授权之后被收尾，引用它的在用价卡要不要离开新的选择。默认不要：那等于让评价去读合同，Q4 的答案是不这样做。
- 第二道门今天有没有类似的核对。取证时只读了 ADR-0190 决定三的文字，没有读采用册的登记代码，立 ADR 时先核。
- 采用册是不设修订的不可变元组。合同或价格政策之后被收尾，已登记的采用怎么办：不追溯，还是给采用册加一条显式的收尾。
- 第二道门与 ADR-0260 决定一「不与选出的卡核对」的关系。那一句说的是评价时不核；本票核在登记时。要不要给 ADR-0260 挂指针，由随票的 ADR 定。

## 三类

全部是机制。授权规则与价格政策的内容是租户取值，本票不碰。

## 完成判据

- 引一条不在册的授权、一条已收尾的授权、一条方向不符的授权，三种登记都不入册，各答各的一格；有真库用例。
- 演示种子的价卡照常登记。
- 预览答出同样的结论。
- 给一位合同上没有对应销售价格政策的客户登记销售计价范围，不登记并说明缺什么；合同、价格政策与绑定都对得上的照常登记。演示种子照常灌入。
- 评价路径上没有新增对参与方与商业的读取。
- `CONTEXT-MAP.md` 那一条关系与相关 `CONTEXT.md` 的边界句写成现行；`UC-PP-001` 与 `UC-SA-002` 补 AT 号。
