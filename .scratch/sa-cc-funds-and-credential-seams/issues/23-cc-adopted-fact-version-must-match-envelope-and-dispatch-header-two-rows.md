# sa-cc/13 补评审 Standards 非阻断两条一笔收口：`HandleAdoptedExternalFundsFact` 核回查版本与信封所指相等、`cmd/parcel-dispatch` 资金事实正例头注按 `0021` 改口

Category: chore
Status: resolved——**完工待非作者评审进 main，2026-09-14 19:4x**（分支 `mcp2-tails3` 基远端 main `09596d9a`；立票笔 `8ab6c61c`、条 1 / 2 / 3 与两票完成记录同末笔，见 Comments「完工」；带 DSN `cmd/parcel-api` + `cmd/parcel-dispatch` PASS 129 / FAIL 0 / SKIP 0；清点重生成零差）；此前 in-progress——2026-09-14 19:2x 通道 2 按通道 1 派单 task-e79cc5a9 自立自做（评审尾巴 A 类推送方派单、作者自立票自做，先例 lc/38 / sa-cc/18 / ve-disc/06），分支 `mcp2-tails3` 基远端 main `09596d9a`（树 `D:/tops/idp-parcel-mcp2-tails3`），与 [lc/40](../../label-channel-service-first-release/issues/40-lc35-review-standards-tails-test-header-counts-and-transaction-shell-sentence.md) 同分支；要裁的为零
Blocked by: 无（[13](13-cc-correction-version-inbound-registration-and-rereconciliation.md) 已进 main `8dbd49e2`，两条出处在其 Comments「补评审 ← 通道 4」Standards 2 / 3）。撞点：`cmd/parcel-dispatch/assemble_test.go` 与 `assemble.go`——通道 4 在途 sa-cc/11 会在同两文件加 PP 消费者装配（截至 `09596d9a` 其分支 `mcp4-sacc11` 尚未碰 `cmd/parcel-dispatch/`），本票各只改一处注释，动前广播占号、改完推完释号，不同块，谁后进 main 谁 rebase

## 缺口（出处逐条指到评审原话；取证于 `09596d9a`，开工已重量）

1. **回查交回的版本被丢弃**（13 补评审 ← 通道 4 Standards 2）。`internal/customscompliance/adapters/settlementaccounting/receive_on_adopted_funds_fact.go` `HandleAdoptedExternalFundsFact`：版本取信封 `adopted.Version`、回指取回查到的 `content.Corrects`，但 `LoadAdoptedFundsFact` 交回的 `content.Version` 未与信封所指比对。今天读口按版本取、两者同源（函数内注释明写），只是若 SA 只读视图哪天答非所问，CC 会把别版内容登在信封那一版名下。**重量**：`09596d9a` 上函数体与评审所记同；`ccports.AdoptedFundsFact.Version` 是 `domain.FundsFactVersion`，与信封译出的 `version` 同型可比。本包既有哨兵四只：`ErrAdoptedFactNotVisible` / `ErrFundsFactReceiveUndecided`（续办，进未决名单）、`ErrUnexpectedReceiveOutcome`（封闭集之外）、`ErrUntranslatableReference`（`adopted_funds_fact_source.go`，引用坏了）——**没有**「读回的与信封所指不符」那一格；PS 同族先例 `ErrCarrierPickupRecordInconsistent`（`judge_on_carrier_first_effective_pickup.go`：装配 / 视图缺陷，与可见性滞后分开正是为了别把永久损坏登成可续办，ADR-0029；生产装配不得进 `WithUndecidedSentinels`）。`cmd/parcel-dispatch/assemble.go` `externalFundsFactUndecidedSentinels` 头注列了「不在名单里的几格」，新哨兵该在那里记一句。
2. **邻接头注变旧**（13 补评审 Standards 3）。`cmd/parcel-dispatch/assemble_test.go` `TestAnAdoptedExternalFundsFactReachesTheCustomsRegisterThroughTheRouteTable` 头注「CC 的 external_funds_fact 落一行」——自 `customs_compliance/0021` 起身份表退成身份、内容进版本子表 `external_funds_fact_version`，一封落的是身份行 + 版本子表各一行；13 动了同一函数体（正例扩了更正版本 v2 第二格）、未动这句。**重量**：`git grep -n '落一行' -- cmd/parcel-dispatch/` 于 `09596d9a` 两处，另一处在核对形成那条（`duty_payment_verification_adoption 落一行`，SA 侧一表一行，不旧、不动）。用例实际断言：`LoadFundsFact` 读回那一版且来源 / 付款人 / 币种 / 金额 / 版本照 SA 转述；第二格 `ListFundsFactVersions` 列两版、v2 回指 v1。

**不在本票**：13 补评审 Standards 1（两包测试替身同形，第三份出现再抽）、Spec 1（地盘外两件，推送方已记账）、Spec 2（核对身份缺资金版本，已追写进 [19](19-cc-new-funds-fact-version-forms-a-new-verification-version.md)）；任何 `internal/settlementaccounting/**`；编排 `ReceiveFundsFact` 的语义；`0021` 与任何迁移。

## 做法

1. `/tdd` 先红：`receive_on_adopted_funds_fact_test.go` 新用例——替身让 `LoadAdoptedFundsFact` 对信封所指 `v2` 交回 `Version` 为 `v1` 的内容，断言不落登记、错误是新哨兵、且**不是** `ErrAdoptedFactNotVisible`。再绿：同包新加 `ErrAdoptedFactVersionInconsistent`（形照 `ErrCarrierPickupRecordInconsistent` 头注：与可见性滞后分开、不进未决名单），`HandleAdoptedExternalFundsFact` 在回查 `found` 之后、译命令之前比 `content.Version != version`，不等即以该哨兵停下；函数内「两者同源」那句注释随之改口。不改编排、不改译码其余各维。
2. `cmd/parcel-dispatch/assemble.go` `externalFundsFactUndecidedSentinels` 头注补一句：`ErrAdoptedFactVersionInconsistent` 不在名单里——视图答非所问是装配 / 视图缺陷，重投不自愈，保持 publish_failed。只注释。
3. `cmd/parcel-dispatch/assemble_test.go` 正例头注「CC 的 external_funds_fact 落一行」按实际断言改写：入向登记册按引用读回信封那一版（`0021` 起身份行 + 版本子表各一行），并点一句正例第二格（更正版本 v2 落第二行、回指 v1）在函数内注释。只注释。
4. 条 1 一笔（红绿同笔，含条 2 那句名单注释）；条 3 与两票完成记录同末笔。

## 红线

- 条 1 只加一句相等校验与一只哨兵，不改 `ReceiveFundsFact` 语义、不按 `corrects` 分路（13 红线三）、不动 `internal/settlementaccounting/**`；条 2 / 3 只注释。
- 新哨兵**不进** `externalFundsFactUndecidedSentinels`（与 PS 先例同一条理由：把永久损坏登成可续办是 ADR-0029 禁的）。
- 注释中文；不写行号、不数别处的东西；改中文源文件不用 `Set-Content`。

## 完成判据

1. 新用例 `-run` PASS；`HandleAdoptedExternalFundsFact` 对「回查交回别版」交回 `errors.Is(err, ErrAdoptedFactVersionInconsistent)` 且登记册零行；既有六条用例零 diff、仍 PASS。
2. `git grep -n ErrAdoptedFactVersionInconsistent` 命中：本包声明 + 使用 + 测试，以及 `cmd/parcel-dispatch/assemble.go` 头注一句；`externalFundsFactUndecidedSentinels` 切片字面零 diff。
3. `git grep -n '落一行' -- cmd/parcel-dispatch/` 只剩核对形成那条；资金事实正例头注含「版本子表」或 `0021`。
4. `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1 ./internal/customscompliance/adapters/settlementaccounting/... ./internal/architecture/...` ok；`cmd/parcel-dispatch` 带 DSN ok（正例 `TestAnAdoptedExternalFundsFactReachesTheCustomsRegisterThroughTheRouteTable` PASS 非 SKIP）。
5. 完成记录随末笔同提交（Status → resolved、逐笔 SHA、判据逐项、判断项）；清点预报零差（不增删文件）。

## 地盘

`internal/customscompliance/adapters/settlementaccounting/receive_on_adopted_funds_fact{,_test}.go`、`cmd/parcel-dispatch/assemble.go`（`externalFundsFactUndecidedSentinels` 头注一句）、`cmd/parcel-dispatch/assemble_test.go`（一处头注）、本票面、本 spec 一行。

## 要裁的

零。

## 参照

[13](13-cc-correction-version-inbound-registration-and-rereconciliation.md) Comments「补评审 ← 通道 4」Standards 2 / 3、「推送方处置」；`internal/parcelshipment/adapters/transportfulfillment/judge_on_carrier_first_effective_pickup.go`（`ErrCarrierPickupRecordInconsistent` 头注，形的先例）；`cmd/parcel-dispatch/assemble.go`（`carrierFirstEffectivePickupJudgmentUndecidedSentinels` 头注「不在名单里的几格」的写法）；`migrations/customs_compliance/0021_external_funds_fact_versions.sql`；ADR-0029；AGENTS.md「写代码注释」。

## Comments

- 2026-09-14 19:2x · 通道 2（task-e79cc5a9）：立票，Status 直接 in-progress，作者自立自做。**只写票面，未动代码。** 两条在 `09596d9a` 上重量过：条 1 函数体与评审所记同、本包无「不符」那一格的哨兵，新加一只并在 `assemble.go` 名单头注记「不在」；条 2 `落一行` 于 `cmd/parcel-dispatch/` 两处，只资金事实那处旧。派单地盘只列了 `assemble_test.go`，条 1 的哨兵要在 `assemble.go` 名单头注记一句是派单「进未决名单的对应格」所指，一并占号。前一会话（同通道）18:22 建完 worktree 即 crash，本会话从零接续。
- **2026-09-14 19:3x–19:4x · 通道 2（task-e79cc5a9）· 完工**。分支 `mcp2-tails3` 基远端 main `09596d9a`，与 lc/40 同分支（推送方重放后 main 上 SHA 会换，对照由推送方在「进 main 记录」补）。立票笔 `8ab6c61c`（两票 + 两 spec 行，无代码）；lc/40 两条 `42f524f4`；本票三条 + 两票完成记录 = **本笔**。
  - **条 1（本笔，`/tdd`）**：红——`receive_on_adopted_funds_fact_test.go` 新用例 `TestASourceAnsweringAnotherVersionIsRefusedBeforeRegistering`（替身对信封所指 `bank-fact/v2` 交回 `Version` 为 `bank-fact/v1` 的内容；断言 `errors.Is(err, ErrAdoptedFactVersionInconsistent)`、**不是** `ErrAdoptedFactNotVisible`、登记册零行），首跑编译红 `undefined: adapter.ErrAdoptedFactVersionInconsistent`；绿——同包新哨兵 `ErrAdoptedFactVersionInconsistent`（头注形照 PS `ErrCarrierPickupRecordInconsistent`：与可见性滞后分开是为了别把永久损坏登成可续办，ADR-0029；生产装配不得进 `WithUndecidedSentinels`）、`HandleAdoptedExternalFundsFact` 在 `found` 之后、译命令之前 `content.Version != version` 即停，错误文本带信封所指与视图所答两值；函数内「两者同源」那句改为「本该同源，上面那句相等校验把『本该』钉成『必须』」。既有六条用例零 diff、仍 PASS。
  - **条 2（本笔）**：`cmd/parcel-dispatch/assemble.go` `externalFundsFactUndecidedSentinels` 头注补「`ccsettlement.ErrAdoptedFactVersionInconsistent` 也不在——…重投不自愈，保持 publish_failed（与 `pstf.ErrCarrierPickupRecordInconsistent` 同一条理由）」；切片字面零 diff（`git diff -U0` 非注释差 0）。
  - **条 3（本笔）**：`cmd/parcel-dispatch/assemble_test.go` 正例头注「CC 的 external_funds_fact 落一行」→「CC 入向登记册按引用读回信封那一版且来源 / 付款人 / 币种 / 金额 / 版本照 SA 转述（`customs_compliance/0021` 起落的是身份行 + 版本子表各一行）；正例第二格（更正版本 v2 落第二行、回指 v1）见函数内注释」。用例正文零 diff。
  - **验（本机，钉本笔）**：`gofmt -l` 空；`go build ./...` / `go vet ./...` 0；不带 DSN `go test -count=1` CC `adapters/settlementaccounting` + `internal/architecture` ok；**带 DSN** `go test -count=1 -p 1 -v ./cmd/parcel-api/... ./cmd/parcel-dispatch/...`：两包 ok，**PASS 129 / FAIL 0 / SKIP 0**，探针 `TestAnAdoptedExternalFundsFactReachesTheCustomsRegisterThroughTheRouteTable` PASS 非 SKIP（19:4x 先 `check_messages(waitMs 0)` 排队列——只有本通道占号回声——再广播占 55432，跑完释号）。清点：`tools/mechanism-inventory` 在本树重生成（`go run . -dir ../..`）后 porcelain 只有本笔在途四件、生成物零差，不出清点笔。
  - **判据逐项**：1 ✓ 新用例 `-run -v` PASS，别版内容零登记；2 ✓ `git grep -n ErrAdoptedFactVersionInconsistent` 命中本包声明 / 使用 / 测试三处 + `assemble.go` 头注一句（其余在票面 / spec），切片字面零 diff；3 ✓ `git grep -n '落一行' -- cmd/parcel-dispatch/` 只剩核对形成那条，正例头注含 `0021` 与「版本子表」；4 ✓ 见上；5 ✓ 本条即，清点零差。
  - **红线**：`internal/settlementaccounting/**` 零 diff；不按 `corrects` 分路、编排 `ReceiveFundsFact` 零 diff、`0021` 零 diff；新哨兵不在 `externalFundsFactUndecidedSentinels`；注释中文、无行号、不数别处；未用 `Set-Content`。
  - **撞点**：通道 4 `mcp4-sacc11` 截至本笔（`git diff --name-only 09596d9a mcp4-sacc11`）未碰 `cmd/parcel-dispatch/`，本票两处注释块与其将来的 PP 消费者装配不同块；占号 / 释号广播各一条已发（19:4x）。
  - **判断项（归评审）**：① 哨兵名取 `ErrAdoptedFactVersionInconsistent`（PS 先例 `…RecordInconsistent` 的形，文本「adopted funds fact disagrees with the version the envelope names」）而非 `…Mismatch`——与本仓「不一致 = 装配 / 视图缺陷、不进未决」这一格的既有命名同族。② 校验放在回查 `found` 之后、译命令之前——停在译命令之前，登记册一行不碰，也不让编排替它判；若评审认为该由读口 `SettlementAdoptedFundsFactSource` 自己核，那是把校验搬到提供方词汇那一侧，本票按派单放在消费处理方。③ 派单地盘只列 `assemble_test.go`，本票另动了 `assemble.go` 名单头注一句——派单「进未决名单的对应格」所指就是那处，且 PS 先例 `ErrCarrierPickupRecordInconsistent` 正是在同一头注「不在名单里的几格」记的；只注释。④ 条 3 头注多点了半句「正例第二格见函数内注释」——头注原写一正一反，函数体自 13 起是三段，对齐而非扩题；评审若嫌多，删那半句零行为。⑤ 新用例的替身写法沿用本文件 `adoptedContent(version, corrects, payer)` 夹具，把键与内容的版本写成两个字面，读者一眼看出「答非所问」是怎么造的。
