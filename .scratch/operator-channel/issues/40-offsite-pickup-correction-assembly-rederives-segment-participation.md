# 40 生产装配的揽收更正不重派生段上参与：buildOffsitePickupCorrectionOrchestration 没交段登记册

Category: bug
Status: resolved——2026-10-18 进 main（与 40、47、49 同批，代码验证钉 `b3b246d7`），见「进 main 记录」。此前 in-progress——2026-10-18 完工；评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-18 作者子代理完工，待非作者评审。此前 in-progress——2026-10-18 会话 `idp-parcel-9b` 立并认领（票 26 遗留研判（簇 C4）中发现，原属 tf-segment-lifecycle-closure 那条线；用户授权代决）；分支 `claude-oc40`
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

- [x] 新真库用例在 main 上先红（段上参与仍指向 v1），改后转绿；红的输出记进完成记录。
- [x] `TestTheWiredPickupCorrectionLandsANewVersionAgainstARealDatabase` 与 `TestTheWiredControlFactsEnterTheSegmentAgainstARealDatabase` 照旧 PASS（带 DSN，不是 SKIP）。
- [x] gofmt、`go build ./...`、`go vet ./...` 通过；带 DSN 跑 `go test -p 1 -count=1 ./cmd/parcel-api/ ./internal/transportfulfillment/...` 全 ok；机制清点重跑零差。
- [ ] 走 Go 那条路（parallel-sessions）：分支、作者自验、非作者评审、推送方全量。
- [x] 票 10 Comments 追记一条：生产装配的缺口由本票补上，并链回本票。证据只记 `S`。

## 完成记录（2026-10-18，会话 idp-parcel-9b 的实施子代理，分支 claude-oc40）

- **落点**：`cmd/parcel-api/assemble_offsite_pickup_correction.go` 的 `buildOffsitePickupCorrectionOrchestration` 构造 `tfpostgres.NewFulfillmentSegments` 并作 `Segments` 交入 `RegisterOffsitePickupDeps`；`transactionalPickupCorrection` 那一笔事务原样包住更正与重派生（重派生拿的是同一个事务 ctx）。`Judgments` 照裁定不交。装配头注改为现行说法：交段登记册是为了让更正在段上重派生参与（ADR-0112 决定一、二；票 tf-segment-lifecycle-closure/10），不交判断登记册的理由是票 10 红线；函数注释的缝清单补上段登记册。`endpoints.go` 未动。
- **完成判据逐格**：
  - 新真库用例先红后绿 → `TestTheWiredPickupCorrectionSupersedesTheParticipationOnTheSegmentAgainstARealDatabase`（首登经 `buildControlFactOrchestrations` 的揽收登记口带段 `SYN-SEGMENT-40` 落 v1；经 `buildOffsitePickupCorrectionOrchestration` 更正落 v2；断言段上当前参与凭 `OFFSITE-PICKUP/<v2>` 入场、回指 `OFFSITE-PICKUP/<v1>`、`enteredAt` 是更正后的发生时刻、计划段沿用、链长二且首登已被替代；SQL 核首登那一行 `supersedes_entry_basis` 仍为空、`entered_at` 未变，参与行数为二；`assertPendingFirstJudgment` 核承运主体判断仍只有首版）与 `TestTheWiredPickupCorrectionOfAnObjectOutsideAnySegmentAnswersNoParticipationToRederive`（未进过段的揽收被更正答 `NO_PARTICIPATION_TO_REDERIVE`、不留续办引用、库里不长参与）。
  - 两条既有用例照旧 PASS（带 DSN，`-v` 下是 `--- PASS`，不是 SKIP）：`TestTheWiredPickupCorrectionLandsANewVersionAgainstARealDatabase`、`TestTheWiredControlFactsEnterTheSegmentAgainstARealDatabase`。
  - 门：见下。
  - Go 那条路：分支与作者自验已做；非作者评审、推送方全量待做（本格未勾）。
  - 票 10 Comments 已追记，链回本票（`d05c3bfc`）。
- **先红后绿**：用例单独提交在 `9735981b`（生产代码与 `b58b10ef` 相同），带 DSN 跑两条新用例，都红在行为上：

  ```
  --- FAIL: TestTheWiredPickupCorrectionSupersedesTheParticipationOnTheSegmentAgainstARealDatabase (0.45s)
      assemble_offsite_pickup_correction_test.go:284: 段上当前参与 = (present=true, basis=OFFSITE-PICKUP/PRV-000000000001)，want OFFSITE-PICKUP/PRV-000000000002——生产装配的揽收更正没有在段上重派生
  --- FAIL: TestTheWiredPickupCorrectionOfAnObjectOutsideAnySegmentAnswersNoParticipationToRederive (0.14s)
      assemble_offsite_pickup_correction_test.go:383: refusal = "", want NO_PARTICIPATION_TO_REDERIVE——段那一半两格皆空，与重派生成功同形
  ```

  改装配之后（`9138e344`）两条都 `--- PASS`。
- **门**（在工作树 `claude-oc40`、钉 `9138e344` 的代码上跑）：
  - `gofmt -l` 两个改动的 `.go`：无输出。
  - `go build ./...`、`go vet ./...`：退 0。
  - `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1 ./cmd/parcel-api/ ./internal/transportfulfillment/... ./internal/architecture/...`：退 0，各包 ok（`ports` 无测试文件）。
  - 机制清点 `go run ./tools/mechanism-inventory`：重生成零差，无清点提交。
- **证据层级**：`S`（`SYN-` 合成租户、对象、段与控制引用，只在测试里）。
- **遗留（未改，地盘外）**：`internal/transportfulfillment/application/register_offsite_pickup.go` 里 `CorrectOffsitePickupCommand` 的注释仍写着「段侧「来源更正 → 参与关系重派生」另立票，本编排不进段」——票 10 落地后已失实（「没有 Segment」那半句仍成立：更正命令不带段号）。不在本票地盘，留给评审或后续票决定是否收。
- **评审**：待非作者评审。

## Comments

- **评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审）· 钉 `6eba9a2f`（基线 `b58b10ef`）· 2026-10-18**：用户 2026-10-18 因 token 花费选「精简续做」，评审由会话本身读 diff 做；代码出自本会话起的实施子代理，审者与写者不同上下文但同属一个会话，按 parallel-sessions 不算非作者评审，如实记下。
  - **Spec**：阻断无。装配补交段登记册、判断登记册照裁定不交；两条真库用例在 `9735981b` 上先红（红在段上当前参与与拒绝格，不是编译错）、`9138e344` 上转绿；首登那一行不动、替代行回指首登，与 ADR-0112 决定一、二一致。
  - **Standards**：阻断无。非阻断 1：`register_offsite_pickup.go` 的 `CorrectOffsitePickupCommand` 头注仍写「段侧另立票，本编排不进段」，已失实（完成记录「遗留」那条），已在 `6eba9a2f` 改成照实说（地盘外一句注释，改到最小）。
  - **无发现**：注释中文、无行号与计数；合成值只用 `SYN-`；领域包未动。

### 进 main 记录（2026-10-18，会话 idp-parcel-9b）

- `9735981b`…`efb26842` 直接叠在 main `b58b10ef` 上，快进，SHA 未改写。同批三票 40、47、49 一次推送。
- 推送方全量一次：钉 `b3b246d7` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 142 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上改动 `.go` 的 `gofmt -l` 无输出，`go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。
- 评审见 Comments。
