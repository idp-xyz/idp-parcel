# lc/40 评审 Standards 一条非阻断收口：`ErrChannelBasisTranslationStopped` 头注「三个实例半边源」跨包计数换点名

Category: chore
Status: resolved——**完工待进 main，2026-09-14 20:5x 通道 1**（推送方自立自做：点名无人应答、除通道 1 外全部 crash；分支 `mcp1-tails4` 基远端 main `eca6dba1`，树 `%TEMP%\idp-parcel-mcp1-tails4`；与 [sa-cc/24](../../sa-cc-funds-and-credential-seams/issues/24-pp-sacc11-review-standards-tails-sentinel-comment-and-evidence-closed-set.md) 同笔；评审门：单通道无非作者可派，推送方自审，票面如实写）。此前 in-progress——2026-09-14 20:4x 通道 1 自立；要裁的为零
Blocked by: 无（[40](40-lc35-review-standards-tails-test-header-counts-and-transaction-shell-sentence.md) 已进 main `f1be6687`，出处在其 Comments「评审 ← 通道 6」Standards ①）。撞点：在途分支零，无人共写

## 缺口（出处指到评审原话；取证于 `eca6dba1`）

**跨包计数**（lc/40 评审 ← 通道 6 Standards ①；AGENTS.md「改文档」计数条，Go 注释同受约束）。`internal/parcelshipment/application/establish_selected_label_transaction.go` `ErrChannelBasisTranslationStopped` 头注「三个实例半边源未配置」数的是另一包 `internal/parcelshipment/adapters/partycommercial/channel_selection_basis.go` `ChannelSelectionBasisTranslatorDeps` 的三个字段（`Accounts` / `Agreements` / `Resolutions`）；那一侧增减一只源是正当改动，改的人不会路过这一句。lc/40 把同病的两处**测试**头注改了，这一处生产头注是评审在 40 之后才点到的、既有非 40 引入。

**不在本票**：`channel_selection_basis.go` 里与 struct 同文件的「三个实例半边源与两个 PC 读口」「三个源允许为 nil」——与被数 struct 同文件，lc/40 评审明判可接受；任何生产语句。

## 做法

头注改为点名：「实例半边源（`ChannelSelectionBasisTranslatorDeps` 的 `Accounts` / `Agreements` / `Resolutions`）未配置」。数词去掉，被数的东西以符号名指过去——那一侧改字段名时 `git grep` 能把这里带出来。只注释。

## 红线

- 零行为：diff 只许注释行。
- 不动 `internal/parcelshipment/domain/**`、`adapters/partycommercial/**`、任何测试。
- 注释中文；不写行号、不数别处的东西。

## 完成判据

1. `git grep -n '三个实例半边源' -- internal/parcelshipment/application/` 零命中；`git grep -n 'ChannelSelectionBasisTranslatorDeps' -- internal/parcelshipment/application/establish_selected_label_transaction.go` 命中头注一处。
2. `gofmt -l` 空、`go build ./...` / `go vet` 退 0；`go test -count=1 ./internal/parcelshipment/application/... ./internal/architecture/...` ok。
3. 完成记录同笔；清点预报零差。

## 地盘

`internal/parcelshipment/application/establish_selected_label_transaction.go`（一处头注）、本票面、lc spec 一行。

## 参照

[40](40-lc35-review-standards-tails-test-header-counts-and-transaction-shell-sentence.md) Comments「评审 ← 通道 6」Standards ①、「进 main 记录」；`internal/parcelshipment/adapters/partycommercial/channel_selection_basis.go`（`ChannelSelectionBasisTranslatorDeps`，被点名的那一侧）；AGENTS.md「改文档」（计数与行号同构）。

## Comments

- **2026-09-14 20:4x–20:5x · 通道 1（推送方自做）· 完工**。分支 `mcp1-tails4` 基远端 main `eca6dba1`，与 sa-cc/24 同笔（本笔）。头注「三个实例半边源未配置」→「实例半边源（ChannelSelectionBasisTranslatorDeps 的 Accounts / Agreements / Resolutions）未配置」，一处、只注释（+3 −2 含折行）。**验**：`gofmt -l` 空、`go build ./...` 0、`go vet` PS application 0、`go test -count=1` PS application + architecture ok。**判据逐项**：1 ✓（`三个实例半边源` 于 application 零命中；符号名在头注）；2 ✓；3 ✓ 本条。**判断项**：① 点名用的是 struct 字段名不是接口名（`ChannelAccountUseSource` / `SupplierAgreementSource` / `AcceptanceResolutionSource`）——字段名是装配方看得见的那一层，接口名改了字段名多半不改，指字段更稳；评审若嫌不够具体，两组名并列也行、零行为。**评审门**：单通道，推送方自审（理由同 sa-cc/24 Comments 末条）。
