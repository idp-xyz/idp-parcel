# ADR-0060: 包裹反查走当前快照投影列，不另表、不扫 JSONB

Status: Accepted  
Date: 2026-08-18

## Context

收寄与交付要把作业实物上的包裹身份反查到**当前已接受**的委托目标（来源身份 + 委托号 + 当前提交版本号），才能交给采用/履约编排。定位映射属接入编排的下一票；本记录只定提供方怎么按 `(租户, 包裹)` 给出零/一/多的答案。

委托聚合今天整份进 `shipment_request.snapshot` jsonb。按包裹查有三条路：对 jsonb 建表达式索引、另建投影表、或在同一行加查询投影列。另表与 jsonb 路径都会让「当前成员」与快照在两次写入之间分叉；而聚合接受后成员固定，下游要的正是**当前**版本的成员，不是历史版本里曾经出现过的包裹。

## Decision

**一、在 `shipment_request` 同行增加 `current_submission_version_id` 与 `declared_parcel_ids`。** 它们是当前聚合快照的查询投影，不是第二份权威。`Insert` / `Save` 必须与 `snapshot` 写在同一条 SQL 里。迁移从 `snapshot.currentVersion.versionId` / `declaredParcelIds` 回填后收紧 NOT NULL 与「至少一件、成员非空」CHECK。

**二、不建独立投影表，不对 `snapshot` 建 JSONB 表达式索引。** 另表是第二处事务；jsonb 路径把查询钉在文档形状上，换一次字段名索引就瞎，且会扫到 `priorVersions` 里的历史成员。

**三、反查只认当前已接受。** 查询条件是 `tenant_id` + `state = ACCEPTED` + 包裹属于 `declared_parcel_ids`。

| 命中行数 | 答案 |
|---|---|
| 0 | `found=false`：这个包裹此刻没有可采认的已接受委托 |
| 1 | 交回完整 `SourceIdentity` + `ShipmentRequestID` + 当前 `SubmissionVersionID` |
| \>1 | 具名 `ErrAmbiguousParcelTarget`，**不按时间、不按数据库顺序任选** |

已提交、已拒绝、已撤回与历史版本成员都不是可采认目标。同包裹被两份已接受委托同时声明，是机制拒绝自动采认，不是「最近接受者优先」。

**四、部分 GIN 建在 `declared_parcel_ids` 上、谓词 `state = 2`（已接受）。** 反查的过滤集合与索引谓词同一条，submitted 行不进索引。

## Consequences

- 提供方端口是独立只读口，不并进 `ShipmentRequestRepository`：建单/推进的调用方不必实现反查；收寄/交付消费方下一票只依赖只读口。
- `FindBySourceIdentity` 的重建门仍只开到`已提交`（ADR-0030）。反查不走重建，避免已接受行读不回来。
- CONS-INTAKE / CONS-DELIVERY 下一票：用本口的目标填 `TargetShipment`（或交付侧的同等指名），不得在消费适配器里另猜 latest。

## Alternatives considered

- **独立投影表。** 否决：Insert/Save 与投影写入会分成两处事务，中间态被按包裹查到就会采认一份与快照不一致的目标。
- **`snapshot -> currentVersion -> declaredParcelIds` 表达式 GIN。** 否决：钉死文档形状；且容易把 `priorVersions` 扫进来，让已经换代的成员继续命中。
- **命中多行时取 `saved_at` 最新。** 否决：同包裹两份已接受是歧义，自动采认会把作业实物接到错误的委托上。

## Links

- [ADR-0028](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)：重建与写入分门
- [ADR-0030](./0030-rehydration-admits-one-state-at-a-time-by-snapshot-expressiveness.md)：重建门只开到已提交
- [ADR-0045](./0045-new-submission-version-keeps-history-and-reestablishes-the-task.md)：当前版本单指针，历史不覆盖当前
