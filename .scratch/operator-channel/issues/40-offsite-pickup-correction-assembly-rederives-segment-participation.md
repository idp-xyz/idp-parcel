# 40 生产装配的揽收更正不重派生段上参与：buildOffsitePickupCorrectionOrchestration 没交段登记册

Category: bug
Status: in-progress——2026-10-18 会话 `idp-parcel-9b` 立并认领（票 26 遗留研判（簇 C4）中发现，原属 tf-segment-lifecycle-closure 那条线；用户授权代决）；分支 `claude-oc40`
Blocked by: 无
父票：[26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)（票 26 评审遗留的后续）
出处：
- [ADR-0112](../../../docs/adr/0112-source-correction-rederives-participation-as-a-superseding-version-on-the-same-segment.md) 决定一至四；
- 票 [10](./10-source-correction-rederives-participation.md) 裁决 2「两触点共用 `rederiveFulfillmentParticipation`；段由登记册按对象反查」，以及「同事务，派生一侧」；
- 簇 C4 研判的真库两臂对照（钉 `8d22729f`）。
地盘：
- cmd/parcel-api/assemble_offsite_pickup_correction.go
- cmd/parcel-api/assemble_offsite_pickup_correction_test.go

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

## 现象

- 票 10 让 `RegisterOffsitePickupHandler.Correct` 在落下新版本之后，经 `rederiveParticipation` 替代段上凭前版入场的那条参与（`4785566d`）。
- 揽收更正口的生产装配 `buildOffsitePickupCorrectionOrchestration` 早于这一笔（`54ae9fdc`），构造 `RegisterOffsitePickupDeps` 时没给 `Segments`。
- `rederiveFulfillmentParticipation` 的规则是「`segments` 缺席时不重派生也不算失败」。所以生产上的揽收更正从不重派生，答复里段那一半两格都是空的，看起来与「重派生成功」一模一样。
- 装配头注「不交入段登记册与判断登记册：更正不进段（裁决附问……另立一票覆盖交接与揽收两种来源），编排的 Correct 也不会去碰它们；这里留空是如实，不是漏接」自 `4785566d` 起已经失实，正好把这个缺口遮住了。
- 真库实测（带 DSN）：
  - 生产装配这一臂：首登带段，落 `PRV-000000000001`；经生产装配更正，落 `PRV-000000000002`。之后段上的当前参与仍是 `OFFSITE-PICKUP/PRV-000000000001`，`enteredAt` 仍是更正前的发生时刻，`segmentContinuationReference` 与 `segmentEntryRefusal` 都为空。
  - 对照臂：同一编排接上段登记册，重派生到 `OFFSITE-PICKUP/PRV-000000000002`，`enteredAt` 随更正改变。
  - 研判已用补了段登记册的装配副本复跑：生产这一臂也重派生到 `PRV-000000000002`；`TestTheWiredPickupCorrectionLandsANewVersionAgainstARealDatabase` 与 `TestTheWiredControlFactsEnterTheSegmentAgainstARealDatabase` 照旧 PASS。
- 交接更正走 `buildControlFactOrchestrations` 里带段登记册的那只编排，不受影响。

## 裁定

这是机制缺陷，照 ADR-0112 与票 10 的裁决补装配：揽收更正口背后的编排要交入段登记册（`tfpostgres.NewFulfillmentSegments`），与更正在同一事务里重派生。

判断登记册不交，因为 `Correct` 不碰承运主体判断（票 10 红线「不改承运主体判断的现有版本」）。

可观测变化：
- 进过段的揽收：段上参与按更正版本替代。
- 从没进过段的揽收：更正时答复带 `segmentEntryRefusal` = `NO_PARTICIPATION_TO_REDERIVE`（票 10 裁决 2 里的领域正当拒绝格），不再两格皆空。端点层的 `TestCorrectOffsitePickupReportsCreatedWithTheNewVersionAndItsPredecessor` 与 `TestCorrectOffsitePickupRederivesTheParticipationOnTheSegmentTheObjectEntered` 已经按这一格写好。

## 做什么

1. 在 `buildOffsitePickupCorrectionOrchestration` 里构造并交入段登记册。装配头注改为现行说法：交入段登记册，是为了让更正在段上重派生参与（ADR-0112；票 tf-segment-lifecycle-closure/10）；不交判断登记册的理由同上。
2. 真库用例，写在 `assemble_offsite_pickup_correction_test.go`，先红后绿：
   - 首登经控制事实那一格带段，落 v1；
   - 经 `buildOffsitePickupCorrectionOrchestration` 更正，落 v2；
   - 断言段上当前参与的入场依据是 v2，起点是更正后的发生时刻，原参与行不动；
   - 另断言：未进过段的揽收被更正时，答 `NO_PARTICIPATION_TO_REDERIVE`。

## 不做

- 不改 `cmd/parcel-api/endpoints.go`。
- 不补「承运主体判断随更正重派生」（票 10 红线）。

## 完成判据

- [ ] 新真库用例在 main 上先红（段上参与仍指向 v1），改后转绿；红的输出记进完成记录。
- [ ] `TestTheWiredPickupCorrectionLandsANewVersionAgainstARealDatabase` 与 `TestTheWiredControlFactsEnterTheSegmentAgainstARealDatabase` 照旧 PASS（带 DSN，不是 SKIP）。
- [ ] gofmt、`go build ./...`、`go vet ./...` 通过；带 DSN 跑 `go test -p 1 -count=1 ./cmd/parcel-api/ ./internal/transportfulfillment/...` 全 ok；机制清点重跑零差。
- [ ] 走 Go 那条路（parallel-sessions）：分支、作者自验、非作者评审、推送方全量。
- [ ] 票 10 Comments 追记一条：生产装配的缺口由本票补上，并链回本票。证据只记 `S`。
