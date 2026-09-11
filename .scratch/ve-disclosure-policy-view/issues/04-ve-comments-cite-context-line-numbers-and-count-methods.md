# VE 注释里的「硬句 NNN」是 CONTEXT 行号；目录读口一族头注数着「六法 / 七法 / 八法」——一笔改口成引文与点名，零行为

Category: chore
Status: resolved——2026-09-11 23:1x 通道 3 作者完工（task-7309fe70；分支 `mcp3-vedisc04-2` 基远端 main `609e334b`，认领 `1e2eab1c`，代码 + 本完成记录同笔，SHA 见完工报）：38 处 `硬句 NNN` 与同症 13 处裸行号全部换成 CONTEXT 原词引文、10 处方法计数改点名；零行为（diff 只有注释行）；判据 1–5 全过，真库未跑。**实测：38 处行号在 `609e334b` 的 CONTEXT 上 38 处全部指错行**，其中 3 处连原始编号也指错（见「完成记录」）。等非作者评审 → 推送方重放进 main。此前 in-progress——2026-09-11 22:5x 通道 3 认领（隔离树 `D:/tops/idp-parcel-mcp3-vedisc04`；21:2x 前会话的认领笔只在 salvage 分支上、零代码，不用）。此前 ready-for-agent——2026-09-11 14:2x 通道 1 推送方立票并直接转 ready（要裁的为零；收下 12:0x 节「VE 注释『六法 / 八法 / 七法』计数一族归 VE owner」那条候选后继，扩到同族行号引用）；取证锚 main `0b027ab8`
Blocked by: 无（硬）。**软阻**：第五波 ve-disc/02（通道 3）正在 `internal/visibilityexception/application` / `adapters/http` 与 `cmd/parcel-ve-register` 动手——本票在 `internal/visibilityexception/**` 大面积改注释，先于它进 main 会让它 rebase 时逐文件解注释冲突；**等 ve-disc/02 进 main 后再开工**，推送方广播后转派——**ve-disc/02 已 16:5x 进 main（2026-09-11，通道 1 推送方记），软阻解除，可派**；02 评审又点了一处同族（`application/register_catalog.go` `CatalogRegistry` 头注「ports 里三口分立」「`postgres.CatalogRegistrar` 三口本就齐备」数的是 ports / postgres 两包的东西），随本票一并收；02 评审 Standards ②（`application.CatalogRegistry` 与 `ports.CatalogRegistry` 两包同名不同型）是改名题不是注释题，**不并入本票**，归 VE owner

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

## 完成记录

（通道 3 · task-7309fe70 · 2026-09-11 22:5x–23:1x · 隔离树 `D:/tops/idp-parcel-mcp3-vedisc04`，分支 `mcp3-vedisc04-2` 基远端 main `609e334b`，全程未 rebase。）

**逐笔**：认领 `1e2eab1c`（Status → in-progress）；代码 + 本完成记录同一笔（SHA 见完工报）。

**动过的文件**（18，全在 `internal/visibilityexception/**`，`adapters/partycommercial/` 零改）：`adapters/postgres/catalogue_read{,_test}.go`、`claim_recovery{,_test}.go`；`application/register_catalog.go`；`domain/customer_claim{,_test}.go`、`customer_disclosure{,_test}.go`、`disposition_request.go`、`eta_visibility_gap{,_test}.go`、`evidence{,_test}.go`、`recovery_matter{,_test}.go`；`ports/catalogue_read.go`、`ports/ports.go`。`git diff -U0 -- internal` 的每一条 `+`/`-` 行都以 `//` 起头（脚本核过：非注释改动零行）；未改 `CONTEXT.md`。

**判据 5 · 行号错位实测（钉 `609e334b`）**：`git grep -n -E '硬句\s*[0-9]+' -- internal/visibilityexception` 在 `609e334b` 仍是 **38 处 / 12 文件**，与票面 `0b027ab8` 取证同数——派单说的「10 处 / 9 文件」是 PowerShell `Measure-Object -Line` 对 `git grep` 中文输出的误计，不是他票换掉了。逐处对 CONTEXT 当前文本核：**38 处全部指错行**，无一处今天还指在它要引的那句上。错位不是均匀的一格：ETA / 处置请求一族偏 18 行（`硬句 102` 今天落在「追踪投影与里程碑」节「来源原文、来源代码……分别保存」，`硬句 145` 落在小节标题「案件责任、响应与闭环」）；客户可见性 / 证据 / 索赔一族偏 20 行（`硬句 159` 落在小节标题「原因、责任与处置协调」，`硬句 173` 落在「客户全程追踪视图只展示……」）；追偿一族偏 21 行（`硬句 186` 落在空行；多出的那一行正是票面点名的 ADR-0136 时点语义句）；生命周期一族偏 27 行（`生命周期 251` 落在「异常案件」生命周期「已关闭 → 处理中或待响应」，`254` 落在空行）。**另有 3 处连原始编号就指错**——数字与注释自己引的原词对不上：`domain/customer_claim.go` `ClaimItemSpec` 头注「授权维（硬句 186）」与 `customer_claim_test.go` `Covers: 硬句 186「按申请人授权、客户账户……判断资格」`，引的都是「收到客户索赔……再按申请人授权、客户账户……判断资格」那句（原编号 173）；`adapters/postgres/claim_recovery.go` `AppendAction` 头注「所有尝试与内容版本保留（硬句 185）」引的是「提交或送达失败是外部动作结果……所有尝试和内容版本保留」那句（原编号 186）。三处都按代码语义找回真正要引的句子落引文。**2 处引文原本不是逐字**：`recovery_matter_test.go` 「追偿在通知或主张条件成立时即可独立发起」（CONTEXT 原词是「供应商或保险追偿在相应通知或主张条件成立时」）与「『已追偿』」（CONTEXT 用“已追偿”），已改逐字。

**同症补收**：票面 38 处之外，同一病的裸行号 **13 处**一并收——`git grep -n -E '(生命周期|CONTEXT)\s*[0-9]{2,}'` 9 处（`customer_claim.go` 四处 `生命周期 25x`、`customer_claim_test.go` 两处、`claim_recovery{,_test}.go` 各一处 `CONTEXT 253`、`customer_disclosure.go` 一处 `CONTEXT 159`）与括号裸数 4 处（`customer_disclosure.go` 「（165）」「（164）」、`customer_claim.go` 「（254）」、`evidence.go` 「（171）」）。收后 `git grep -n -E '硬句|(生命周期|CONTEXT)\s*[0-9]{2,}|（[0-9]{3}）' -- internal/visibilityexception` 零。

**引文纪律**：每处 ≤ 20 字、单行不折、逐字取自 CONTEXT 当前文本；原本折行的多行引文一律缩成单行短引文（`Covers:` 行同此，一句要引两处就写两段「」）。收工时用脚本把 diff 里所有带 `CONTEXT` 的 `+` 行上的「」内容逐条对 `CONTEXT.md` 做子串核：65 条全部命中、0 缺。

**计数 → 点名（判据 2）**：10 处 / 5 文件——`adapters/postgres/catalogue_read.go` 「这一族八法」→「这一族」；`catalogue_read_test.go` 三处「六个方法」→「各方法」；`application/register_catalog.go` 「三口分立」「三口本就齐备」→「各写口分立」「各写口本就齐备」（「各补两个方法」→「补齐那两册的写法」，两册在同一头注里点了名）；`ports/catalogue_read.go` 「同族七法」→「同族各法」、「前六法对 CatalogRegistry 的六个方法，后两法对单立的」→「目录册各法对 CatalogRegistry 的各方法，其余各法对单立的」；`ports/ports.go` 「那个接口已有六口，每加一口」→「那个接口每加一口」、「故六个方法」→「故它一类占两法」（数的是本接口自己的方法，同文件）。

**验证**（隔离树，23:0x，Windows 本机，无 DSN）：`gofmt -l ./internal/visibilityexception` 空；`go build ./...` 退 0；`go vet ./...` 退 0；`go test -count=1 ./internal/visibilityexception/...` 全 `ok`（`ports` 无测试文件）。**真库未跑、无行为改动**（票面判据 4 如实记）。清点不涉及：不增删文件、不改任何签名。

**判断项**（请评审与 VE owner 裁，本票未动）：
1. 同族但不在判据里的计数仍在：「七件」（`eta_visibility_gap.go` `ETAPredictionSpec` 头注、`eta_visibility_gap_test.go` / `recovery_matter_test.go` 的 `Covers:` 行）与「六件必备」（`customer_disclosure.go` `CustomerNotification` 头注）数的是 CONTEXT 那一句列了几件，是跨文件计数；「五个装载口」（`catalogue_read_test.go`）、「两组表」（`ports.go` `CatalogRegistry` 头注）分别数 `ports.go` 的装载口与迁移表。这些每处紧跟着逐项点名，去数词零损失，但票面 做法 2 只点了方法数，故未动、留给 VE owner 一并裁。
2. `ports/catalogue_read.go` `CatalogueListRead` 头注「八个方法一口装下而不按页面分组拆三个接口」数的是本接口自己的方法（同文件），未动；它正是票面说的「六法 → 八法」漂移点，若 VE owner 要连同文件计数一起去，一并收。
3. 注释讲的规则与 CONTEXT 现文**无不符**：逐处核时只见行号错、引文不逐字两类，没有一处规则本身与现文矛盾，故无需另记交 VE owner 的条目。

## 参照

AGENTS.md「改文档」与「写代码注释」；tasks.md 2026-09-11 12:0x 节「候选后继 · VE：注释『六法 / 八法 / 七法』计数一族」；同族 [sa-cc/15](../../sa-cc-funds-and-credential-seams/issues/15-cc-comments-cite-context-line-numbers-and-count-other-packages.md)（CC 半边，含错位实测）、[lc/36](../../label-channel-service-first-release/issues/36-ps-label-final-review-follow-ups-header-comments-and-constructor-guards.md)（PS）。

## Comments

- 2026-09-11 14:2x · 通道 1 推送方：立票。**只写票面，未动代码。** 能力边界：处数来自 `Select-String` 实测（`0b027ab8`）；VE CONTEXT 的行是否已错位没有逐处核，写进「做法」1 由实施者量。
- 2026-09-11 23:1x · 通道 3（task-7309fe70）：作者完工。38 处行号在 `609e334b` 上 38 处全部指错行（偏 18 / 20 / 21 / 27 行不等），另 3 处连原始编号也错、2 处引文不逐字；同症裸行号 13 处一并收；10 处方法计数改点名。零行为、真库未跑。三条判断项见「完成记录」。分支已推 origin；等非作者评审后重放。
