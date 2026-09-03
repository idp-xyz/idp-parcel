# 复核追加表、复核写口、在用解析读口及真库实现

Category: enhancement
Status: resolved——`1b09c2d`（MCP-3，2026-09-03）
Blocked by: 01（已 resolved，`7042a38`）

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

## Comments

- 2026-09-03 MCP-3：落地 `1b09c2d`（父提交 `2a9a76a`，共享树上直接做——全部是新文件与测试追加，无签名
  变更，无红窗口）。四件按票面：`0004_reference_series_review.sql`（复合外键、结论 CHECK、不加四眼库层
  约束并写明理由）；`ports.ReferenceSeriesReviewRegister` / `ports.ReferenceSeriesInForceResolver`（各自
  封闭代数，按恢复动作分格，多出一格 `SeriesKindDisagrees`——方案绑错序列或序列登错种类不是「再登一版」
  能修的）；真库实现 `adapters/postgres/reference_series_review.go`；真库用例七条。
  **两处与票面不同**：① `Record` 对「版本不在册」**先 SELECT 再 INSERT**，不靠撞外键——撞外键会让整个
  事务进 aborted 态，调用方 commit 变 rollback，实测就是这样红的；外键留作最后一道墙。② 新增领域窥视口
  `PeekReferenceSeriesRegistrationReference`（只读引用不整版重建），给挑版用；选中后仍由 `ResolveAt`
  整版重验。迁移接线无需碰共享文件：`migrations.go` 按目录 `all:parcel_pricing` 嵌入，`0004` 自动进计划。
  **验证**：在 `1b09c2d` 的 detached 检出上、DSN 已设：gofmt 零输出，`go build` / `go vet` /
  `go test -count=1 ./...` 全绿（**含 PG**），`adapters/postgres` 用例 `-v` 下 PASS 非 SKIP。
  棘轮基线剪掉 `SelectInForceSeriesVersion`，在 `2a9a76a` 干净内容上数得 31→30。PBC-08 门禁要求的无事务
  负向证据并入 `TestPricingWritesRefuseToRunOutsideATransaction`。`-race` 未跑（Windows 侧无 cgo）。
