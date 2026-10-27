# 83 多成员的投影派生与归属确立补派生按相反次序锁投影当前标记，会互等成死锁

Category: bug
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（线外，复核见 .scratch/launch-line-freeze-2026-10-27.md 第六节）：多成员的投影派生与归属确立补派生按相反次序锁投影当前标记——PostgreSQL 会中止其中一路、整笔回滚，重投即自愈，不答错；解冻条件：生产上死锁回滚计数异常时。此前 needs-triage——建议 R3：PostgreSQL 会中止其中一路、整笔回滚，重投即自愈，不答错；代价是一次回滚与一次重投，失败码看不出是死锁。2026-10-21 协调方据 [code/50](./50-customer-view-derivation-can-be-carried-back-by-a-concurrent-stale-projection-envelope.md) 隔离评审立（作者子代理代笔；用户「你作为业务和系统专家自决……解决所有的问题」授权）。分级由协调方定稿。
Blocked by: 无（建在 code/50 的 `LockCurrent` 之上，随 code/50 进 main）
出处：code/50 隔离评审；code/50 完成记录越权风险点 2。

## 现象

code/50 让客户视图派生的两路消费在消费事务里对投影当前标记 `tracking_projection_current` 持 `FOR SHARE`。归属确立补派生（`DeriveCustomerViewOnAcceptanceAdapter`）按 PS 声明清单的次序逐件 `LockCurrent`，锁持到整封提交。

投影派生那一侧有几路在同一笔事务里逐件改多件包裹的标记（`Projections.Save` 的 `ON CONFLICT DO UPDATE`）：

- `DeriveOnMovementFactAdapter`：集运单元经 `trackedParcelsAt` 展开到成员，逐件派生；
- `derive_on_exception_journey`：按成员逐件派生；
- CC 的 `derive_on_declaration_submission` 与 `derive_on_external_result`：按成员逐件派生。

两边碰同一组包裹而次序相反时：投影那一路已改了 X、要改 Y；补派生那一路已锁了 Y、要锁 X。两路互等，PostgreSQL 约 1s 后以 40P01 中止其中一路，整笔回滚。

投影与投影之间同形：两笔多成员的投影派生按不同次序改同一组标记，同样互等。这一格早于 code/50。

## 证据

- `internal/visibilityexception/adapters/parcelshipment/derive_customer_view_on_acceptance.go`：`HandleFormedAcceptanceDecision` 按 `FindCurrentDeclaredParcels` 交回的次序逐件 `LockCurrent`。
- `internal/visibilityexception/adapters/transportfulfillment/derive_on_movement_fact.go`、`derive_on_exception_journey.go`，`internal/visibilityexception/adapters/customscompliance/derive_on_declaration_submission.go`、`derive_on_external_result.go`：成员循环按来源交回的次序，不排序。
- `internal/visibilityexception/adapters/postgres/projection.go`：`Save` 对标记 `ON CONFLICT DO UPDATE`，与 `LockCurrent` 的 `FOR SHARE` 冲突。
- `internal/visibilityexception/application/derive_projection.go`：`Save` 失败时答 `DeriveUndecided` / `ProjectionStoreUnavailable`，丢掉原错误。投影那一路被中止时，日志里连 SQLSTATE 也看不到；视图那一路经 `ErrDerivedProjectionUnreadable` 包着原错误，正文里带 40P01。

## 三类

机制。无产品策略、无租户取值。

## 规模

S–M。

## 最小的诚实修法

1. 两边都按（租户，包裹引用）排序后再逐件处理：补派生排声明清单的成员，投影派生那几路排展开后的包裹。口径同 ADR-0210「取锁次序……按标识排序」。投影与投影之间同形的死锁一并收掉。
2. 排序只改处理次序，不改每件包裹各自的结果；ADR-0066 的消费侧循环拆分与「一封信一笔事务」不变。
3. 顺手：`DeriveProjectionHandler` 在 `Save` 失败时保留原错误（结果里带上，或交给调用方日志），让 40P01 这类根因能看见。
4. 补真库用例：两路按相反次序碰同一组包裹，修后不出 40P01、两路都提交；去掉排序时红。

## 要裁的

- 排序键用包裹引用的字节序即可，还是要与 ADR-0210 的标识口径逐字对齐。领票时定。
