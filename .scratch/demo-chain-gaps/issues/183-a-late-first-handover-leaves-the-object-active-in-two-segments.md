# 183 首登`已交接`晚到时对象落进两段同时在场，此后每次交接都在结束参与那一步响亮失败

Category: bug
Status: in-progress——2026-10-26 会话 `tops-20`（Claude Code）立并认领（用户 2026-10-26「按你的建议开工吧」），分支 `claude-dcg183`（worktree `idp-parcel-dcg183`）。分级 R2（上线后尽快）：离线补登就会撞上，撞上后该对象此后每次交接都失败、要人工处理。
Blocked by: 无
出处：票 [130](./130-handover-correction-from-pending-to-handed-over-enters-the-segment.md) 作者发现；[ADR-0307](../../../docs/adr/0307-a-handover-corrected-from-pending-or-refused-to-handed-over-enters-the-declared-segment.md) 越权风险点 2（「首登晚到同样落进两段同时在场」）。用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」授权代决。

## 现象

首登`已交接`的业务时间早于对象此刻那条参与的起点时（离线补登、信封晚到），结束先前段参与那一步答未受理、进段照走，对象两段同时在场；此后它的每一次交接都在结束参与那一步以 `ErrObjectActiveInSeveralSegments` 响亮失败。票 130 只给更正加了「有更晚的入场或离场就不进段」那道检查（`laterControlFacts`、答格 `LATER_CONTROL_FACTS`），首登没有。

## 修法（作者落时自决并写代决）

- 首登`已交接`在 `handOverIntoSegment` 之前同样先问 `laterControlFacts`。
- 有更晚的入场或离场时，**倾向记下交接事实、不进段**（与段不在册时交接照登、段那一半欠账同形），答复带明确的格说明没进段的原因；不倾向「一笔不写」——首登是真实发生过的交接，离线补登时拒登会丢掉这一事实。若作者读码后认为必须一笔不写，写清理由。
- 首登答复面（UC-TF-005 `AT-TF-051`、`AT-TF-060` 一带）随之多一格；ADR-0307 越权风险点 2 在 Status 注记里挂「已由票 183 收」并写取舍（正文不动）；必要时 ADR 取 0308。

## 完成判据

- [ ] 首登晚到不再让对象两段同时在场（真库用例）；此后的交接正常。
- [ ] 正常时序的首登、首登重放不受影响（用例）。
- [ ] 新 AT 行与 AT 盘点①表行；Go 门禁；全量用 `scripts/test-full.sh`；走 Go 那条路。
