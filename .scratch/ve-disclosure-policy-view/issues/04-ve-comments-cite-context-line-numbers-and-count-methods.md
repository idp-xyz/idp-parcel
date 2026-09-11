# VE 注释里的「硬句 NNN」是 CONTEXT 行号；目录读口一族头注数着「六法 / 七法 / 八法」——一笔改口成引文与点名，零行为

Category: chore
Status: ready-for-agent——2026-09-11 14:2x 通道 1 推送方立票并直接转 ready（要裁的为零；收下 12:0x 节「VE 注释『六法 / 八法 / 七法』计数一族归 VE owner」那条候选后继，扩到同族行号引用）；取证锚 main `0b027ab8`
Blocked by: 无（硬）。**软阻**：第五波 ve-disc/02（通道 3）正在 `internal/visibilityexception/application` / `adapters/http` 与 `cmd/parcel-ve-register` 动手——本票在 `internal/visibilityexception/**` 大面积改注释，先于它进 main 会让它 rebase 时逐文件解注释冲突；**等 ve-disc/02 进 main 后再开工**，推送方广播后转派

## 缺口（取证于 `0b027ab8`；数本身是论点，故写数并锚 SHA）

- `git grep -n -E '硬句\s*[0-9]+' -- internal/visibilityexception` **38 处 / 12 文件**（`domain/customer_claim.go` / `customer_disclosure.go` / `evidence.go` / `recovery_matter.go` / `disposition_request.go` / `eta_visibility_gap.go` 及各自 `_test.go`、`adapters/postgres/claim_recovery.go` 等）。`apps/admin-web/src/pages/visibility/` 零。
- 那些数字是 `docs/domain/visibility-exception/CONTEXT.md` 的**行号**（CONTEXT 里没有「硬句 N」标记）。CC 半边（[sa-cc/15](../../sa-cc-funds-and-credential-seams/issues/15-cc-comments-cite-context-line-numbers-and-count-other-packages.md)）已实测一处插句让其后所有引用错一行；VE CONTEXT 自那些注释写下以来也改过（ADR-0136 随笔加了时点语义一句），**本票开工第一件事**：对 38 处逐一核今天的行是不是注释要引的那句，把实际错位的数目写进完成记录——这是 AGENTS.md「行号失效时仍然指得很稳，只是指错了」的第二份实测。
- 跨文件计数：`ports/catalogue_read.go` 两处（「与同族七法同形」「前六法对 CatalogRegistry 的六个方法，后两法对单立的……」）、`adapters/postgres/catalogue_read.go` 一处（「这一族八法对『稳定序』不留例外」）——数的是读口接口的方法数与 `CatalogRegistry` 的方法数；ve-disc/02 正要给 `CatalogRegistration` 加两方法、03 刚给读口加过两法（「六法」→「八法」就是这么来的，两处已经各说各的数）。`ports/ports.go` `ConflictSignalRuleRegistry` 头注「那个接口已有六口」同族。

## 做法

1. **行号 → 引文**：每处 `硬句 NNN` 换成该句一小段原词引文（≤ 20 字，「」），删数字；已引原句的只删数字；`Covers:` 行写成 `Covers: VE CONTEXT「……」`。逐处对着 CONTEXT 当前文本核；指错行的按代码语义找回它真正要引的那句。
2. **计数 → 点名**：「六法 / 七法 / 八法 / 六个方法 / 六口」改成「同族各法」「`CatalogRegistry` 的各方法」这类不带数的写法；说「稳定序不留例外」不必说是几法。
3. 一笔或按包分笔都行；不顺手改任何非注释行。

## 红线

- 零行为改动：diff 只许注释行、`Covers:` 行、测试失败文本字符串。
- 不改 `docs/domain/visibility-exception/CONTEXT.md`；不给 CONTEXT 加编号。
- 注释讲的规则若与 CONTEXT 现文不符（不只是行号错），另记一条交 VE owner，不在本票里替它改。

## 完成判据

1. `git grep -n -E '硬句\s*[0-9]+' -- internal/visibilityexception` **零**。
2. `git grep -n -E '六法|七法|八法|六个方法|已有六口' -- internal/visibilityexception` 零。
3. `git diff <基线> --stat` 只含 `internal/visibilityexception/**`；`+`/`-` 行全部是注释、`Covers:` 或字符串字面量（评审抽验）。
4. `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1 ./internal/visibilityexception/...`（不带 DSN，编译过即可，票面如实记「真库未跑、无行为改动」）。
5. 完成记录写明：38 处里实际指错行的有几处、各自真正要引的是哪句（这一数是论点，锚开工时的 main SHA）。

## 地盘

`internal/visibilityexception/**`（只注释）。**不动** `docs/**`、`apps/admin-web`、其他上下文。开工前 `git fetch` 核 ve-disc/02 已进 main。

## 参照

AGENTS.md「改文档」与「写代码注释」；tasks.md 2026-09-11 12:0x 节「候选后继 · VE：注释『六法 / 八法 / 七法』计数一族」；同族 [sa-cc/15](../../sa-cc-funds-and-credential-seams/issues/15-cc-comments-cite-context-line-numbers-and-count-other-packages.md)（CC 半边，含错位实测）、[lc/36](../../label-channel-service-first-release/issues/36-ps-label-final-review-follow-ups-header-comments-and-constructor-guards.md)（PS）。

## Comments

- 2026-09-11 14:2x · 通道 1 推送方：立票。**只写票面，未动代码。** 能力边界：处数来自 `Select-String` 实测（`0b027ab8`）；VE CONTEXT 的行是否已错位没有逐处核，写进「做法」1 由实施者量。
