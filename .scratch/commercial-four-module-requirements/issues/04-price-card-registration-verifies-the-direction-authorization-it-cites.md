# 04 价卡登记只记下它引用的价格方向授权，不核对——引一条不存在或方向不符的授权照样入册

Category: enhancement
Status: draft
父票：[spec](../spec.md)
Blocked by: 用户决定 Q4（见 [spec](../spec.md)「待用户确认」）。开工时随票立一篇 ADR：计价第一次读参与方与商业，`CONTEXT-MAP.md` 的 `party-commercial → parcel-pricing` 一条要补
地盘：`internal/parcelpricing`（新增消费侧适配器）、`internal/partycommercial`（新增只读口）、`apps/admin-web/src/pages/pricing` 与 `pages/party`

对应要求：Contract「价卡授权」。

## 现象

取证钉 `11bfa1ffb`。

- 价卡登记必须带一条价格方向授权引用（`PriceCardRegistration`），引用的种类必须是 `ArtifactCommercialAuthorization`。那个常量的注释写着「这里只登引用不解析其内容」。
- `internal/parcelpricing/adapters/` 下没有读参与方与商业的适配器，所以没有任何地方核对这条引用。
- 演示种子里，价卡引的是 `SYN-AUTH-PRICE-DIR-01`、`v1`；它在商业种子里是一版授权规则，指名一份商业价格政策。方向写在那份价格政策上。
- 销售与采购方向的商业价格政策绑定一份定价方案（`CommercialPricePolicy` 的 `PricingPlan`），但按范围选卡不读这个绑定，也不与选出的卡核对（[ADR-0260](../../../docs/adr/0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md) 决定一）。

## 要做的

1. **登记时核对**：价卡入册之前核它引用的授权——在 `party-commercial` 是一版已发布的授权规则；登记时刻落在它的有效区间内，且它没被收尾；它授权的方向与价卡的方向相同。
2. **门只有一处**：放在价卡登记用例里，登记口、草稿发布与受控命令行共用，位置照 [ADR-0352](../../../docs/adr/0352-registering-a-price-card-in-a-canonicalization-newer-than-the-base-one-needs-an-explicit-deployment-switch-that-is-off-by-default.md) 决定一。
3. **答复按恢复动作分格**：授权不在册或未发布、已到期或已收尾、方向不符，这三格不入册；读不回是未决。
4. **预览同样核**，提前告诉操作者。
5. **看得见**：价格政策与授权规则的对象页列出引用它的价卡。
6. **不改选卡**：评价照旧按范围选卡。

## 要裁的

- 方向怎么从授权规则读出来。种子里它经指名的价格政策带出方向；授权规则正文的形状取证时没有读，立 ADR 时先核。
- 已在册、引用悬空的旧卡不追溯。重放过不过这道门要裁：ADR-0352 决定四对部署开关取的是「重放也拒」。
- 授权之后被收尾，引用它的在用价卡要不要离开新的选择。默认不要：那是按合同授权选卡，属 Q4 的后一种答案。

## 三类

全部是机制。授权规则与价格政策的内容是租户取值，本票不碰。

## 完成判据

- 引一条不在册的授权、一条已收尾的授权、一条方向不符的授权，三种登记都不入册，各答各的一格；有真库用例。
- 演示种子的价卡照常登记。
- 预览答出同样的结论。
- `CONTEXT-MAP.md` 那一条关系与两份 `CONTEXT.md` 的边界句写成现行；`UC-PP-001` 补 AT 号。
