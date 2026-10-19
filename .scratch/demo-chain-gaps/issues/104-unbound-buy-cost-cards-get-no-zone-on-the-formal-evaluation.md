# 104 没绑分区目录的 BUY 成本卡在正式评价上取不到区：按发生项计价时没有段计价区域可读

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 作者子代理立（票 [routing-first-cut/20](../../routing-first-cut/issues/20-bound-cost-cards-fold-zones-through-the-parcel-postal-route.md)、[43](./43-demo-cost-card-binds-the-zone-catalogue.md) 同一分支落地时整理出的余项；用户「你作为业务和系统专家自决……解决所有的问题」授权）。
Blocked by: 无（要先有一篇新 ADR，见「要裁的」）
地盘：待裁定后定；可能跨 PP 的发生项计价输入解析（`internal/parcelpricing/adapters/transportfulfillment`）、TF 的实际履约段与计划段的连线、NR 段计价区域读口。
出处：[ADR-0189](../../../docs/adr/0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md) Consequences 第二条与越权风险点 3；票 43「不在本票」那一段（乙路）；票 27 完成记录「偏离与待裁」第 2 条。

## 现象（读码，证据 `S`）

- 选路比价按段计价：没绑分区目录的卡取租户为那一段登的段计价区域（ADR-0189 决定一第二条）。
- 正式 BUY 评价按运输收费发生项计价（`internal/parcelpricing/adapters/transportfulfillment` 的计价输入解析）。发生项不带段的起讫，输入只有成员包裹的申报与邮编路线，不带分区。卡没绑目录、输入又不带分区，评价落 `ZONE_UNRESOLVED`，结算按 ADR-0200 入账、不形成供应商预期成本。
- 同一张卡，择优时出价，事后记成本时待判断——两边对不齐。

演示租户今天不受影响：票 43 让演示成本卡绑了分区目录，两边都从包裹邮编路线经目录读区。演示线路三段登的段计价区域（种子 `22-leg-pricing-zones-cn-sg-v1.json`）因此没有读者，只留作登记那一路的演示行。

## 三类

- 机制：发生项计价时取段计价区域的那条取数路径（实际履约段 → 计划段 → NR 线路段 → 段计价区域）。
- 产品策略：未绑目录的卡在正式评价上的分区给法，与选路比价是否同一口径。
- 租户取值：无（区由租户在段计价区域册上登，已有登记口）。

## 要裁的

- 正式 BUY 评价要不要读段计价区域；要读，按哪一条连线找到「这一次实际履约对应 NR 线路的哪一段」。实际履约段与计划段之间今天只有登记方声明的 `plannedSegment`，不是每条交接都带。
- 这是跨 PP、SA、NR、TF 的产品机制，要一篇新 ADR，由那几位 owner 裁。真实租户若用按线路计价、不绑目录的干线卡，迟早要它。
- 裁成「不读」也是一种答案：那就写明未绑目录的卡只用于选路比价，正式成本卡必须绑目录，并在登记成本卡时提示。

## 规模

M（裁定之后定）
