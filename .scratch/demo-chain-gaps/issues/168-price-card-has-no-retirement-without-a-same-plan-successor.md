# 168 价卡没有不指名同方案后继的退役：错登到同一范围的另一方案、方案下线，旧卡退不了场

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R2）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-21 票 [150](./150-price-card-has-no-controlled-cut-off-for-a-superseded-version.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权；150 只开同一定价方案内的截止与退役）。是否认领、归哪一位 owner 待分诊。
Blocked by: 无
地盘（建议）：PP `domain` 收尾一族（`price_card_closure.go`）、`application` 收尾用例、`postgres` 退役册；需要新 ADR 或 supersede ADR-0234 决定七一项。
出处：[ADR-0234](../../../docs/adr/0234-an-older-price-card-version-leaves-new-selections-only-through-an-explicit-closure-for-a-named-successor.md) 决定七与越权风险点 5。

## 现象

票 150 落地后，价卡收尾只收同一定价方案的两版：后继晚于旧版起点的截止，后继不晚于旧版起点的退役。以下几格仍没有出路：

- **让位给另一定价方案**：租户把改正过的卡登成了另一个方案（新的方案标识），同一计价范围、方向与目的上两张卡同时适用，结算交来的评价请求在在用解析上一直答`适用冲突`；旧卡指名不了另一方案的后继，收尾口答未受理。票 14 的派发用例 `TestARequestStalledOnAnUnboundZoneCatalogueResumesOnACardThatBindsIt` 钉的正是这一格。
- **不指名后继的退役**：一张卡下线、或错登了一张要撤，没有后继可指。
- 今天的出路只有一种：把新卡登成旧方案的新一版再收尾；已经错登成另一方案的那一张永远退不了场。

## 要裁的

- 让位给另一方案的退役与试算多候选的关系：试算的适用装载把不同方案同时适用读成正当的多候选（择优归 network-routing），把一个方案的一段让给另一个方案，会改变择优看到的候选集。
- 不指名后继的退役要不要「接得住」那道门：没有后继，旧版答过的时段就变成没有在用价卡，是否允许、要不要登记方显式确认。

## 三类

退役写口与读法是机制；是否允许跨方案让位、无后继退役要不要额外确认是产品策略；哪一张卡何时退场是租户取值。

## 规模

S–M
