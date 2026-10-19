# 44 TF：节点实测之后按实测量再准备（接 NO 实测，ADR-0201 决定八「另立票」）

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权），出自 [18](./18-transport-preparation-consumes-the-initial-route.md) 的完成记录。后继票，不在演示主链上；地盘与完成判据分诊时定稿。
Blocked by: [18](./18-transport-preparation-consumes-the-initial-route.md)；另等 peer 会话 idp-parcel-9b 在 `internal/nodeoperations/**` 的收寄、实测各票收工（方案「等 peer 的」那一行「NO 实测触发的再准备」）
地盘：TF 新消费者（NO 实测已形成或节点收寄信封 → 准备编排）、TF 消费侧适配器 `adapters/nodeoperations` 的实测读口、`cmd/parcel-dispatch/assemble.go`（派发令牌）。
出处：[ADR-0201](../../../docs/adr/0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 决定八「节点实测后的再准备另立票」与越权风险点 3「用申报量做容量判断」；方案「01 运输机会与班次」D01-6。

## 现象

票 18 落地后，运输准备的需求量只取收寄前的客户申报（来源 `declaration`）。包裹进了节点、有了有效实测之后，准备结果仍是按申报判的那一份；实测偏大时，容量判断偏乐观，只能靠运营再准备（票 [36](./36-tf-authorized-re-preparation-request-face.md)）补救。UC-TF-003「容量与班次规则」要求「成员或当前有效测量变化时形成新需求快照……未重评时不得沿用旧结论」。

## 三类

- 机制：实测触发再准备的消费边、需求量快照的第二个来源词、新快照不覆盖原结果。
- 产品策略：实测与申报并存时取哪一个（实测优先、缺席取申报，与计价侧 ADR-0171 决定二同一取法是候选）、哪些段要重备（未出发的剩余段）。
- 租户取值：无。

## 做什么（分诊时定稿）

- TF 消费 NO 的实测已形成（或节点收寄已形成）信封，经消费侧适配器读当前有效实测，按 [18](./18-transport-preparation-consumes-the-initial-route.md) 的准备用例对剩余段再判；需求量快照来源记实测、锚记实测版本。
- 结果身份要分得开按申报与按实测的两判（规则版本或身份加一维，分诊时裁），原结果保留。
- 消费者先于生产方进路由表（ADR-0049）。

## 完成判据（分诊时定稿）

- 同一段同一包裹在实测之后多一份按实测判的结果，原按申报的那一份不被改写。
- 实测比申报大、使原选班次某一维不够时，新结果如实答不够或换班。
- 真库测试。

## 规模

M
