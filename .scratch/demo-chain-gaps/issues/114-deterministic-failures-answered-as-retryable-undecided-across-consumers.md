# 114 多处消费者把确定性失败答成可重试的未决

Category: bug
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R2）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-19 分诊为 B（顺手可修），合并 39、41、54、77、90，见 [triage-2026-10-19](../triage-2026-10-19.md)
Blocked by: 无

## 现象

下面五张票是同一个形状：某个消费者或登记口遇到**重放也不会变**的失败（标识超长使信封越限、分区键越限、两侧封闭词表不一致、库里有坏行导致重建失败），却把它答成可重试的未决，派发因此反复重投直到弃置。

- [39](./39-buy-evaluation-request-handoff-does-not-separate-deterministic-rejection.md)：BUY 交接口
- [41](./41-transport-commission-handoff-does-not-separate-deterministic-rejection.md)：委托交接口
- [54](./54-charge-occurrence-registration-does-not-separate-deterministic-agreement-check-failures.md)：发生项登记的协议核对
- [77](./77-charge-occurrence-fact-reads-do-not-separate-unrehydratable-rows.md)：发生项事实读口
- [90](./90-handover-consumers-retry-deterministic-rebuild-failures-as-undecided.md)：交接消费侧

票 [103](./103-a-rederived-final-outcome-enters-the-projection-without-its-predecessor.md) 评审补（2026-10-19）：VE 终局译装器（`internal/visibilityexception/adapters/parcelshipment/derive_on_final_outcome.go`）读 PS 终局采用账时，`FindByKey` 与 `predecessorOf` 里的 `FindByFinalVersion` 把仓储交回的一切错误都包成 `ErrFinalNotVisible`，进本路未决名单。读到行之后解码或重建失败（行坏了、重派生痕迹对不上）是确定性的，也被一并重投。`FindByKey` 那条路是既有的，`FindByFinalVersion` 照它的写法接上；两处同形，随本票统一判据时一起收。

票 [53](./53-movement-facts-recorded-before-their-segment-never-reach-tracking.md) 实施补（2026-10-20，ADR-0224 越权风险点 8）：VE 移动事实补派生（`internal/visibilityexception/adapters/transportfulfillment/backfill_movement_facts.go`）里，按段列事实那一口是本票新开的，坏行已与读不动分开——TF 读口包住 `ports.ErrMovementFactRecordUnreadable`，补派生归 `ErrMovementFactRecordInconsistent`、不进未决名单。仍同形的是既有段册那两问：`FindSegmentsForObject` 失败与段 `FindByKey` 失败（含段行装不回）一律包成 `ErrObjectSegmentsNotVisible`，进交接与揽收两路的未决哨兵名单（`cmd/parcel-dispatch/routes_movement_fact_backfill.go`），段行坏了就反复重投。本票定统一判据时一并点名。

票 [42](./42-departure-gate-reading-does-not-expand-consolidation-unit-members.md) 评审补（2026-10-20）：TF 移动事实登记册按键那一读（`internal/transportfulfillment/adapters/postgres/movement_fact_registry.go` 的 `MovementFacts.FindByKey`）读到行之后重建失败，只包一句 `find movement fact: %w`，与连不上库同形；VE 移动事实投影（`internal/visibilityexception/adapters/transportfulfillment/derive_on_movement_fact.go` 的 `HandleRecordedMovementFact`）把这一读的一切错误包成 `ErrMovementFactNotVisible`，进派发本路的未决哨兵名单（`cmd/parcel-dispatch/assemble_movement_fact.go`），于是一条确定性坏行挂在派发分区队头反复重投。票 42 给出发门禁读数加了几种只能由绕过构造门的写入造成的装不回情形（单元那一格的答复与成员推出的对不上、指名的成员在读数里没有一格、单元互相包含、单元那一格带了关务判断标识，ADR-0225 决定五），这一路的坏行因此多了几种来路。修法方向：`FindByKey` 重建失败也包住 `ports.ErrMovementFactRecordUnreadable`（按段列那一口已这样做，ADR-0224），VE 据它归 `ErrMovementFactRecordInconsistent`、不进未决名单，与补派生那一路同一判据。只登记，票 42 不改。

票 [121](./121-departures-recorded-before-their-segment-are-never-re-asked-at-the-gate.md) 实施补（2026-10-26，ADR-0305 的前置条件）：本票点名的段册与班次册两处已由票 121 收了——段册按键那一读（`internal/transportfulfillment/adapters/postgres/fulfillment_segment_registry.go` 的 `FulfillmentSegments.FindByKey`）与班次册各读（`transport_schedule.go` 的 `scanTransportSchedule`，按键与按列共用）读到行之后装不回的那一类，分别包住 `ports.ErrFulfillmentSegmentRecordUnreadable`、`ports.ErrTransportScheduleRecordUnreadable` 交出，与连不上分开；出发门禁读数据它各另记一个停点，补问不为它再问。用上这两个哨兵的只有 TF 出发读数与补问；VE 补派生与移动事实那一路仍把段坏行包成 `ErrObjectSegmentsNotVisible` / `ErrMovementSegmentNotVisible` 记未决，交接册那一读、移动事实册按键那一读与本票其余各处都没动，统一判据仍归本票。

## 为什么合并

分散在 SA、TF、NR、VE、PS 各处，单看每处都「只在坏数据时触发、最终会弃置」，所以没人会顺手修；而形状一致，合起来一次能定一条统一判据：确定性错误不进未决哨兵名单（这条在派发规范里已有），以及读口重建失败归哪一格。

## 不做什么

不在本票改任何派发重试策略本身。
