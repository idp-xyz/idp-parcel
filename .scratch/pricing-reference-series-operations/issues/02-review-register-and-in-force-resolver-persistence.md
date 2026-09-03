# 复核追加表、复核写口、在用解析读口及真库实现

Category: enhancement
Status: ready-for-agent
Blocked by: 01

## 要建什么

按 ADR-0099 决定二、三，在 `internal/parcelpricing/ports` 与 `adapters/postgres`、`migrations/parcel_pricing` 落地：

1. **迁移 `0004_reference_series_review.sql`**：`parcel_pricing.reference_series_review`，行只增不改。列：租户、序列 ID、序列版本、复核责任方、复核时刻、结论（CHECK 封闭 `APPROVED` / `RETURNED`）、依据（非空）、登记时刻（默认 now）。外键指向 `reference_series_version` 三列键——复核不存在的版本在库层就不成立。**不加**「复核责任方 ≠ 登记责任方」的库层约束（跨表比对属领域门，票 01 已关；迁移注释写明为什么不在这里重复）。允许同一版本多条复核（退回后再通过）。
2. **端口 `ReferenceSeriesReviewRegister`**（写口，不拓宽 `ReferenceSeriesRegister`）：`Record(ctx, review domain.SeriesReview) (ReviewRecorded | ReviewVersionUnknown, error)`——版本不在册答 `ReviewVersionUnknown`，不是 error（消费方恢复动作是先登记）。
3. **端口 `ReferenceSeriesInForceResolver`**（读口）：`ResolveInForce(ctx, tenant, kind, seriesID, at time.Time) (domain.VersionReference, InForceOutcome, error)`；`InForceOutcome` 封闭：`InForce` / `NoRegisteredVersion` / `NoApprovedVersion`。读口只取候选（版本引用 + 登记时刻 + 每条复核的时刻与结论），**选择规则调票 01 的纯函数**，SQL 不排序裁决。
4. **真库实现**与 `pgtest` 用例：多版本多复核的在用选择、退回不进在用、未来时刻的复核不算、跨租户不可见、复核不存在版本被拒。

## 红线

- `reference_series_version` 不改一列。
- 迁移 LF 行尾（校验和按字节）。
- 迁移接线在 `migrations/migrations.go` / `internal/platform/migrate/plan.go`——**核一次**：parcel_pricing 模块是否已按目录嵌入、0004 是否自动被扫到；若要碰这两个共享接线文件，按 parallel-sessions 占号纪律先在频道说。

## 验证

真库用例 `-v` 下 `PASS` 非 `SKIP`（DSN 见 workflow.md 本机环境）；全仓绿。
