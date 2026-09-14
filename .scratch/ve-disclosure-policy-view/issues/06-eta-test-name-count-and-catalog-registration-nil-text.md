# ve-disc/05 非作者评审 Standards 非阻断两条一笔收口：`TestAnETADemandsItsSevenParts` 改名去数、`NewCatalogRegistration` 拒 nil 文本随 `CatalogRegistries` 改口

Category: chore
Status: resolved——**已进 main，2026-09-14 10:4x**（通道 1 推送方重放：main `1ca125a5` 之上 `cherry-pick 6bbf2bf0..mcp4-tails2` 五笔零冲突，本票两条 = `dcd5cefb` / `4bee9fca` + 本簿记笔；非作者评审 ← 通道 1 两轴 0 阻断 / 0 非阻断；`cb440073` 带 DSN 全量 110 ok / 0 FAIL；清点零差）；此前 resolved——完工待非作者评审进 main，2026-09-14 10:2x（分支 `mcp4-tails2` 基远端 main `6bbf2bf0`；条 1 `1735f723`、条 2 本笔，完成记录随条 2 同提交，见 Comments「完工」；清点预报零差）；此前 in-progress——2026-09-14 10:1x 通道 4 按通道 1 派单 task-c0000fd2 自立自做（评审尾巴 A 类推送方派单、作者自立票自做，先例 lc/38 / sa-cc/17 / ve-disc/05），分支 `mcp4-tails2` 基远端 main `6bbf2bf0`（树 `D:/tops/idp-parcel-mcp4-tails2`），与 [sa-cc/18](../../sa-cc-funds-and-credential-seams/issues/18-sa08-count-comments-and-pp-value-helpers-unified.md) 同分支；要裁的为零；本目录无 `spec.md`，不为一张尾巴票新造（ve-disc/05 判断项 ① 同一裁法）
Blocked by: 无（[05](05-ve-review-tails-counts-rename-nil-sentinel-and-adr-0136-addendum.md) 已进 main `a34f439c`，两条出处全在其 Comments「评审 ← 通道 1」）。撞点：推送方同期只改 `.scratch/` 待裁票面、ADR-0136 一句补记、tasks.md，与本票零重叠；共享树不碰

## 缺口（出处逐条指到评审原话；取证于 `6bbf2bf0`，开工先重量）

1. **用例名带计数**（05 评审 ← 通道 1 Standards ①）。`internal/visibilityexception/domain/eta_visibility_gap_test.go` 的 `TestAnETADemandsItsSevenParts`——「Seven」数的是 VE CONTEXT 那一句列了几件，与 05 条 1 收掉的注释同病，只是长在标识符上；05 条 1 红线只许注释行，作者不动是守票面。**重量**：`git grep -n TestAnETADemandsItsSevenParts` 于 `6bbf2bf0`——代码里只有声明一处，`.scratch/` 里的命中全是记录，无他处引用。同包已有 `TestARecoveryMatterOpensIndependentlyWithItsFullShape` 的形可照。
2. **拒 nil 文本没随形参名走**（05 评审 Standards ②）。`internal/visibilityexception/application/register_catalog.go` `NewCatalogRegistration(registry CatalogRegistries)` 拒 nil 仍报「visibility exception application: catalog registry is required」，而 05 已把接口改名 `CatalogRegistries`。**重量**：`git grep -n 'catalog registry is required'` 于 `6bbf2bf0`——VE 这一处，另有 NR `register_network_catalog.go` `NewNetworkCatalogRegistration(registry ports.NetworkCatalogRegistry)` 同文本；NR 那处形参类型就叫 `NetworkCatalogRegistry`，文本与名相符、不是本票的病，不动。全仓无用例断言这串文本。

**不在本票**：任何待裁题；`pptest` 清点口径；任何生产 `.go` 行为；NR 那处文本。

## 做法

1. `TestAnETADemandsItsSevenParts` → `TestAnETADemandsItsFullShape`，形照同包 `TestARecoveryMatterOpensIndependentlyWithItsFullShape`；用例正文与 `Covers:` 头注不动。一笔。
2. 文本 →「visibility exception application: catalog registries are required」，与形参名 `registry CatalogRegistries` 对齐；其余零改。一笔，完成记录随之同提交。

## 红线

- 除条 2 那一串错误文本外零行为；diff 只许一个标识符与一串文本。
- 不动 `internal/visibilityexception/**` 其余文件、`cmd/parcel-api/assemble_claims.go`、ADR-0136、`apps/`。
- 不写行号、不数别处的东西。

## 完成判据

1. `git grep -n 'SevenParts' -- internal/` 零；`git grep -n 'TestAnETADemandsItsFullShape' -- internal/visibilityexception/domain/` 一处；`go test -count=1 -run TestAnETADemandsItsFullShape ./internal/visibilityexception/domain/` PASS。
2. `git grep -n 'catalog registry is required' -- internal/visibilityexception/` 零；`git grep -n 'catalog registries are required' -- internal/visibilityexception/` 一处；VE `application` 用例仍 PASS。
3. `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1 ./internal/visibilityexception/... ./internal/architecture/...` ok（不带 DSN，两条都不碰持久化）。
4. 完成记录随条 2 那一笔同提交（Status → resolved、逐笔 SHA、判据逐项、判断项）；清点预报零差。

## 地盘

`internal/visibilityexception/domain/eta_visibility_gap_test.go`（一个标识符）、`internal/visibilityexception/application/register_catalog.go`（一串文本）、本票面。

## 参照

[05](05-ve-review-tails-counts-rename-nil-sentinel-and-adr-0136-addendum.md) Comments「评审 ← 通道 1」Standards ① ②、「进 main 记录」候选后继；`internal/visibilityexception/domain/recovery_matter_test.go`（`...WithItsFullShape` 的形）；AGENTS.md「写代码注释」（计数与行号同构）。

## Comments

- 2026-09-14 10:1x · 通道 4（task-c0000fd2）：立票，Status 直接 in-progress，作者自立自做。**只写票面，未动代码。** 两条在 `6bbf2bf0` 上重量过：条 1 只一处声明、无引用；条 2 另有 NR 同文本一处，形参名相符、不动。
- **2026-09-14 10:1x–10:2x · 通道 4（task-c0000fd2）· 完工**。分支 `mcp4-tails2` 基远端 main `6bbf2bf0`，与 sa-cc/18 同分支（推送方重放后 main 上 SHA 会换，对照由推送方在「进 main 记录」补）。立票笔 `2a61e0d0`（两票 + sa-cc spec 行，无代码）。
  - **条 1 `1735f723`**：`TestAnETADemandsItsSevenParts` → `TestAnETADemandsItsFullShape`，用例正文与 `Covers:` 头注不动；`-run` PASS。
  - **条 2 本笔**：`NewCatalogRegistration` 拒 nil 文本 →「visibility exception application: catalog registries are required」，与形参 `registry CatalogRegistries` 对齐；其余零改。NR `NewNetworkCatalogRegistration` 的「catalog registry is required」形参类型就叫 `NetworkCatalogRegistry`，文本与名相符，不动。
  - **验（本机，钉本笔）**：`gofmt -l`（暂存 blob）空；`go build ./...` / `go vet ./...` 0；不带 DSN `go test -count=1` VE 全部包 + `internal/architecture` ok（两条都不碰持久化）。未跑全量。
  - **判据逐项**：1 ✓ `git grep -n 'SevenParts' -- internal/` 零、`TestAnETADemandsItsFullShape` 一处、`-run` PASS；2 ✓ `git grep -n 'catalog registry is required' -- internal/visibilityexception/` 零、`catalog registries are required` 一处、VE `application` ok；3 ✓ 见上；4 ✓ 本条即，清点零差（不增删文件）。
  - **红线**：diff 只有一个标识符（条 1）与一串文本（条 2）；VE 其余文件、`cmd/parcel-api/assemble_claims.go`、ADR-0136、`apps/` 零 diff。
  - **判断项（归评审）**：① 名字取 `TestAnETADemandsItsFullShape`（派单举的那个、同包先例的形）；② NR 同文本那处按「文本与形参名相符」判不动——若评审认为两处该同一措辞，另笔零行为。
- **2026-09-14 10:4x · 非作者评审 ← 通道 1 推送方**（钉 `a4da9748` = 重放 tip `cb440073` 同内容；只读通读 diff + 脚本核）：**Standards** 阻断 无 / 非阻断 无——diff 恰是一个标识符与一串文本，`Covers:` 头注未动，`git grep -n SevenParts -- internal/` 0、`catalog registry is required` 于 VE 0；判断项 ① ② 都接受（NR 那处文本与形参名相符，不该为「同一措辞」改它）。**Spec** 阻断 无 / 非阻断 无——两条出处对得上 ve-disc/05 评审 Standards ①②；判据 1–4 复核同结果；零 `apps/`、零生产语句。**结论**：可进 main。
- **2026-09-14 10:4x · 进 main 记录（通道 1 推送方）**：与 [sa-cc/18](../../sa-cc-funds-and-credential-seams/issues/18-sa08-count-comments-and-pp-value-helpers-unified.md) 同一次重放——`%TEMP%\idp-replay-tails2-1035` detached `1ca125a5`，`cherry-pick 6bbf2bf0..mcp4-tails2` 五笔零冲突（sa-cc spec.md 状态行与新增 18 行不同 hunk，自动合），tip `cb440073`；`gofmt -l` 空、build / vet 0；10:3x 占号（先排队列），`cb440073` 带 DSN 全量 **110 ok / 0 FAIL / 16 无测试 / 0 cached**（132 s）；探针 `TestAnETADemandsItsFullShape` / `TestCompletedEvaluationIsReadBackUnchanged` PASS；清点重生成 porcelain 空；10:4x 释号。SHA 对照：`2a61e0d0→bb7aff8c` / `1735f723→dcd5cefb` / `c38a54c0→4bee9fca` / `e7809d23→0588613d` / `a4da9748→cb440073`。`mcp4-tails2` → `merged/`、远端删；作者树与重放树比内容后拆。
