# 183 首登`已交接`晚到时对象落进两段同时在场，此后每次交接都在结束参与那一步响亮失败

Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 claude-dcg183 tip 见分支（代码笔 e2838233a，其上一笔仅补本票完成记录）。此前：（: bug）
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

- [x] 首登晚到不再让对象两段同时在场（真库用例）；此后的交接正常。
- [x] 正常时序的首登、首登重放不受影响（用例）。
- [x] 新 AT 行与 AT 盘点①表行；Go 门禁（全量 `scripts/test-full.sh` 未跑，按协调方指示只跑范围内包）。

## 完成记录

- 起点 `fe43ee99b`，分支 `claude-dcg183`；代码、用例、文档一笔 `e2838233a`，其上一笔补本记录。
- 改了什么：`handOverIntoSegment`（首登与更正共用）在结束先前段参与之前，对转出控制的一版先问 `laterControlFacts`；答拒绝格时交回 `LATER_CONTROL_FACTS`、不结束不进段。更正那一路原有的同一检查移进这里，不留两份。登记册读不动（欠账）时不在此处作答，落回结束参与那一步响亮失败，与此前一致（`TestRegisterHandoverExposesTheSegmentDebtWhenTheRegistryIsDown` 钉着）。拒收与待确认、段登记册缺席不问。HTTP 层本就透出 `segmentEntryRefusal`，只改了注释。无迁移、无 CONTEXT 改动、无新 ADR。
- 代决：记下交接、不进段、不拒登（理由：首登是真实发生过的交接，离线补登拒登会丢事实；与段不在册时「交接照登、段另说」同形）。答格取 `LATER_CONTROL_FACTS` 而非欠账引用——重试不会变，不是欠账。没发现「记下交接」会破坏别的不变量：交接意图照交，下游 NO/NR 按交接事实自分。
- 越权风险点：（1）晚到首登的对象在声明的段里没有参与、不形成该段的实际履约发生项，与 ADR-0307 风险点 1 同理；（2）读不动登记册时晚到检查让位给结束参与那一步，评审曾设想更正口读册失败也整笔不落，核码并非如此：更正先走重派生，读册失败在那里就答欠账返回，走不到这一检查，行为未变（`TestACorrectionIntoTheSegmentWithTheSegmentRegistryDownWritesNothing` 钉着）；只有首登口读册失败改由结束参与那一步响亮失败（本来如此）。守卫只认拒绝格，源码已注明。
- 判据 → 用例：晚到首登不进段、交接在册、重放答已有版本 → `TestALateFirstHandoverIsRecordedWithoutEnteringTheSegment`（application/late_first_handover_test.go）；真库不两段同时在场、此后交接正常 → `TestALateFirstHandoverLeavesTheObjectInOneSegmentInTheDatabase`；正常时序首登 → 既有首登/结束参与用例未改仍绿。改坏验证：把新检查置为不执行，两条新用例均红（真库那条因结束参与答未受理、对象落进两段）。
- 文档：UC-TF-005 新立 `AT-TF-356`，`AT-TF-051`、`AT-TF-060` 与「一致性」一节补一句；AT 盘点①表补行并追记；ADR-0307 Status 索引注记与 Links 补票 183（正文未动）。
- 自验：`go build ./...`、`go vet ./internal/transportfulfillment/...`、gofmt 干净；带 PG DSN 跑 `./internal/transportfulfillment/... ./cmd/parcel-api/... ./cmd/parcel-dispatch/... ./internal/architecture/... ./migrations/...` 全绿；`at-coverage-counts -check` 只报生成物过期（未重生成）。

