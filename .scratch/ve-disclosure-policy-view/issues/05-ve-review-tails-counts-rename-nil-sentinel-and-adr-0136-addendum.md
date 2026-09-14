# VE 评审尾巴 A 类合票：四处跨文件计数换点名、`application.CatalogRegistry` 跨包同名改名、`assemble_claims.go`「四个协作方」去数、`NewClaimServiceRules` 拒 nil 包哨兵、ADR-0136 越权风险点 2 补一句实测

Category: chore
Status: resolved——**已进 main，2026-09-14 09:4x**（通道 1 推送方重放：main `3ee64735` 之上 `cherry-pick cdf17834..mcp3-vetails` 六笔零冲突 `01df425c` / `2ca8a088` / `c5959a1c` / `cc9d86b2` / `b811a99e` / `a34f439c` + 本簿记笔；非作者评审 ← 通道 1 两轴 0 阻断（Standards 0 / 2 非阻断 · Spec 0 / 2 非阻断，全文见 Comments）；`a34f439c` 带 DSN 全量 110 ok / 0 FAIL；清点零差；**作者会话在条 5 与完成记录成笔之前 crash——条 5 与票面 Status 由推送方 `chore(salvage)` `b9fbd0dd` 原样封存，「完成记录」由推送方按 git 证据代写**，见该节）。此前 resolved——2026-09-14 09:3x 通道 3 作者完工（task-0962cb6b；分支 `mcp3-vetails` 基远端 main `cdf17834`，认领 `80564f02`，五条五笔 `fa9d76bd` / `b59d8f74` / `ecff1f67` / `66c6168c` / 条 5 与本完成记录同笔，SHA 见完工报）：条 1 票面四处 6 行 + 同症补收 13 行全部注释、零行为；条 2 `application.CatalogRegistry` → `CatalogRegistries`，清点与接线基线零差；条 3 一行注释；条 4 铸 `ErrNilDependency` + 用例 `errors.Is`，先红后绿；条 5 ADR-0136 越权风险点 2 纯追加。判据 1–6 全过，真库未跑（无 DSN，368 例 SKIP 全是 `IDP_PARCEL_POSTGRES_DSN` 门禁）。等非作者评审 → 推送方重放进 main。此前 in-progress——2026-09-14 09:0x 通道 3 按通道 1 派单 task-0962cb6b 自立自做（先例 sa-cc/16、lc/37、psr/08：已进 main 的票在非作者评审里点出、评审者与推送方认可的 A 类，推送方派单、作者自立票自做；用户 08:5x 经队列裁「派吧」）；隔离树 `D:/tops/idp-parcel-mcp3-vetails`，分支 `mcp3-vetails` 基远端 main `cdf17834`；取证锚同
Blocked by: 无

五条全是**已进 main 的票在非作者评审里点出、评审者与推送方都认可、要裁的为零**的尾巴；本票只收尾巴，不重开任何一张原票的题。

## 缺口（五条，出处逐条；取证锚 main `cdf17834`，开工先各自重量）

1. **跨文件计数仍在四处**（出处：[ve-disc/04](./04-ve-comments-cite-context-line-numbers-and-count-methods.md) 完成记录判断项 1 + 评审 ← 通道 1 Standards ①「归 VE owner 与 02 评审 S② 合成下一张 VE 小票」）。「七件」——`internal/visibilityexception/domain/eta_visibility_gap.go` `ETAPredictionSpec` 头注、`eta_visibility_gap_test.go` 与 `recovery_matter_test.go` 的 `Covers:` 行；「六件必备」——`domain/customer_disclosure.go` `CustomerNotification` 头注；「五个装载口」——`adapters/postgres/catalogue_read_test.go`；「两组表」——`ports/ports.go` `CatalogRegistry` 头注。数的分别是 VE CONTEXT 那一句列了几件、`ports.go` 里只读装载口有几只、PS 迁移里 `PAR-VIS-08` 落了几张表——全是别处的东西（AGENTS.md「写代码注释」：计数与行号同构）。每处紧跟着已逐项点名，去数词零损失。**同文件计数不动**（ve-disc/04 评审判断项 2 已裁不算违反）。
2. **`application.CatalogRegistry` 与 `ports.CatalogRegistry` 两包同名不同型**（出处：[ve-disc/02](./02-exception-disclosure-and-conflict-signal-rule-registries-have-no-cli-or-online-entry.md) 评审 ← 通道 5 Standards ②「Mysterious Name 判断题……本仓 wiring ratchet / enum 门禁头注都点过跨包重名让按名认的工具误判；建议开个名」；ve-disc/04 票头明写「改名题不是注释题，不并入本票，归 VE owner」）。`application.CatalogRegistry` 是 02 步一新立的合成接口（`CatalogRegistration` 的依赖：目录册写口加异常披露规则、冲突信号规则两册写口）；`ports.CatalogRegistry` 是既有领域词、被 postgres / 测试 / 清点引用更广。改 application 那一只。
3. **`cmd/parcel-api/assemble_claims.go` `buildClaimEligibilityRules` 头注「四个协作方缺一即装配失败」**（出处：[ve-claims/04](../../ve-claims-read-seams/issues/04-rule-resolution-key-source-needs-a-registration-face.md) 评审 ← 通道 1 Standards ① / 进 main 记录「候选后继：VE owner——`assemble_claims.go` 头注『四个协作方』计数」）。数的是另一文件 `ClaimServiceRulesDeps` 的字段数。`claim_service_rules.go` 里同文件那句「收拢四个协作方」不算，留。
4. **`NewClaimServiceRules` 四道拒 nil 是裸 `fmt.Errorf` 文本，装配方按 `errors.Is` 认不出是装配漏了**（出处：ve-claims/04 评审 Standards ② / 进 main 记录「候选后继：`NewClaimServiceRules` 拒 nil 包哨兵」；与 [sa-cc/16](../../sa-cc-funds-and-credential-seams/issues/16-sa-evaluation-request-and-duty-verification-adoption-standards-tails.md) ① 同族）。`internal/visibilityexception/adapters/partycommercial/claim_service_rules.go` 四道 `return nil, fmt.Errorf("…is nil")`；本包与整个 `internal/visibilityexception/**` 没有 `ErrNilDependency` 一类哨兵（`git grep -n -E 'Err[A-Za-z]*Nil|ErrNilDependency' -- internal/visibilityexception` 零）。
5. **ADR-0136 越权风险点 2 的前提写反了**（出处：ve-claims/04「裁决」5 实施中追裁「候选后继（归 ADR-0136 owner）：越权风险点 2 要补一句 `262e8c0a` 实测——『可登』应为『不可登』」/ [ps-port-remainder/09](../../ps-port-remainder/issues/09-resolution-key-face-does-not-accept-customer-service-rule.md) 后继 ③）。`docs/adr/0136-claim-rule-resolution-key-is-the-acceptance-time-commercial-resolution-reference.md` 越权风险点 2 原写客户服务规则「可登」；`262e8c0a` 实测是**不可登**——迁移 0007 立、0008 重加的 CHECK `commercial_resolution_key_registration_bases_closed` 白名单与 PS `commercialKindFrom` 名集均无该格；ps-port-remainder/09 已以迁移 0022 + `commercialKindFrom` 加一格开闸（`9fa2ddc6`，2026-09-12 进 main）。

## 做法

1. **计数 → 点名或去数**：照 ve-disc/04 判据 2 那一批的写法（「各件」「各法」「那几口」）。「七件」→「各件」；「六件必备」→「各件必备」；「五个装载口」→「只读装载口」；「两组表」→「各自成表」。逐处对着被数的那一侧核一次，确认去数后句子仍成立。同一病、同一句式在 `internal/visibilityexception/**` 里若还有票面四处之外的（比如折行导致 `git grep` 单行模式漏掉的），一并收，完成记录分开记「票面四处」与「同症补收」（ve-disc/04 先例：评审接受）。
2. **改名**：`application.CatalogRegistry` → 新名（作者定，完成记录写为何取它与否掉了哪几个）。纯改名零行为：`git grep -n CatalogRegistry -- internal/ cmd/` 全仓核引用与替身（`application/register_catalog{,_test}.go`、`adapters/http/register_catalog_test.go`、`cmd/parcel-ve-register/main_test.go` 的 `var _ application.CatalogRegistry = …` 编译期钉）；头注随名改口，说清它是 `CatalogRegistration` 收拢的各写口合成、不是与 `ports.CatalogRegistry` 同名的第二只目录册写口。`internal/architecture/production_wiring_baseline.txt` 在 `cdf17834` 不含该名（`git grep CatalogRegistry -- internal/architecture` 零），预期零改；若清点有变如实记。
3. **去数**：「四个协作方缺一即装配失败」→「协作方缺一即装配失败」。
4. **包哨兵**：在 `internal/visibilityexception/adapters/partycommercial` 铸一只导出哨兵 `ErrNilDependency`，头注写它是构造期缺件的唯一答复、哪一口缺在包装信息里点名（形照 `settlementaccounting/application` `NewRequestBuyEvaluationHandler` 与 `parcelshipment/application` `operate_label_transaction.go` 那一族）；四道 `fmt.Errorf` 改 `fmt.Errorf("%w: <口名>", ErrNilDependency)`；`TestClaimServiceRulesRefuseToBeBuiltWithoutTheirCollaborators` 改 `errors.Is` + 文本含口名 + 返回值为 nil（形照 sa-cc/16 ① `TestTheHandlerRefusesANilDependencyAtConstruction`）。这是本票**唯一**触及行为的一条，且只在构造期错误路径：错误文本可变、哨兵可 `errors.Is`。
5. **ADR 补记**：越权风险点 2 **不改写原句**，在该条末尾追加一句带日期的补记「补记 2026-09-14：……见 ps-port-remainder/09」，引 SHA 用短 SHA、引代码用符号名，不写行号；不改 Status、不 supersede（AGENTS.md「改文档」：不改写已接受 ADR 历史，补记是追加不是改写）。

每条改完就成一笔推 `origin mcp3-vetails`，不等全部改完；完成记录随最后一笔代码同笔提交。

## 不在本票

- ve-disc/04 评审判断项 2 的同文件计数（已裁不算违反）——含 `ports/ports.go` `CatalogRegistry` 头注里的「五类」「两法」「五个只读装载口」（装载口就在 `ports.go` 里）、`ports/catalogue_read.go` `CatalogueListRead` 头注「八个方法…三个接口」。
- `adapters/partycommercial/claim_service_rules.go` 三段逻辑（ve-claims/04 已进 main，零改）；`RulesForClaim` 路径一字不动。
- ADR-0136 越权风险点 2 本身「必登否」（归 PS owner，psr/09 要裁的 1）。
- 任何 CONTEXT / UC 改动；`apps/` 零改。

## 红线

- 条 1 / 3 diff 只许注释行与 `Covers:` 行；条 2 只许标识符与它的头注；条 4 只许构造器四道拒绝的错误值与对应用例；条 5 只许 ADR-0136 越权风险点 2 那一条的末尾追加。
- 注释中文；不写行号、不数别处的东西（本票就是来收这个的，别新添一处）；引 CONTEXT / ADR 用引文或决定号。
- 不为任何租户预填任何东西；条 4 不改运行期读路径的任何错误代数。

## 完成判据

1. `git grep -n -E '七件|六件必备|五个装载口|两组表' -- internal/visibilityexception` **零**（含折行形：`git grep -n -E '五个$' -- internal/visibilityexception/ports/catalogue_read.go` 零）。
2. `git grep -n -E 'application\.CatalogRegistry|^type CatalogRegistry interface' -- internal/visibilityexception/application cmd/parcel-ve-register` 零；`ports.CatalogRegistry` 引用数与 `cdf17834` 相同；`go build ./...` 过（三处替身的编译期钉随名改）。
3. `git grep -n '四个协作方' -- cmd/parcel-api` 零；`claim_service_rules.go` 同文件那句仍在。
4. `git grep -n 'ErrNilDependency' -- internal/visibilityexception/adapters/partycommercial` 命中哨兵声明 + 四道包装 + 用例；`go test -count=1 -run TestClaimServiceRulesRefuseToBeBuiltWithoutTheirCollaborators -v ./internal/visibilityexception/adapters/partycommercial/` PASS 非 SKIP；用例对四口各断 `errors.Is` + 文本含口名 + 返回 nil。
5. `git diff cdf17834 -- docs/adr/0136*` 只有追加行，原「可登」句一字未改；追加句含「补记 2026-09-14」「`262e8c0a`」「ps-port-remainder/09」「0022」。
6. 每笔 `gofmt -l` 空、`go build ./...` / `go vet ./...` 0；`go test -count=1 ./internal/visibilityexception/... ./cmd/parcel-api/... ./internal/architecture/...`（不带 DSN，真库用例 SKIP 如实记「真库未跑」）；条 2 改名后清点零差（不增删文件）。

## 地盘

`internal/visibilityexception/**`、`cmd/parcel-api/assemble_claims.go`、`docs/adr/0136-*.md`、本票面。**不碰**：`cmd/parcel-dispatch/**`、`internal/settlementaccounting/**`、`internal/parcelpricing/**`、PS `adapters/transportfulfillment` / `domain`（通道 4 在动）、共享树。

## 完成记录（推送方代写，2026-09-14 09:4x；作者通道 3 在条 5 成笔之前 crash，本节全部按 git 与脚本取证，不写作者没说过的判断）

**逐笔**（分支 `mcp3-vetails` 基 `cdf17834`；括号内为重放进 main 后的号）：`80564f02`（立票即认领 → `01df425c`）· 条 1 `fa9d76bd`（→ `2ca8a088`）· 条 2 `b59d8f74`（→ `c5959a1c`）· 条 3 `ecff1f67`（→ `cc9d86b2`）· 条 4 `66c6168c`（→ `b811a99e`）· 条 5 + Status 由推送方封存 `b9fbd0dd`（→ `a34f439c`；`chore(salvage)` 原样入库一字不改：树上 2 件已修改、0 件未跟踪，mtime 09:25:32 / 09:27:33，最后一笔 09:24:15，用户 09:3x 报 crash）。

**动过的文件**（`git diff --name-only cdf17834 a34f439c`，20 件 +65/−46 于 `internal` / `cmd` / `docs/adr` + 本票面）：条 1 十三件全注释——`domain/{eta_visibility_gap,customer_disclosure}.go`、`domain/{eta_visibility_gap,recovery_matter,disposition_request}_test.go`、`application/{form_eta,send_disposition_request}{,_test}.go`、`adapters/postgres/catalogue_read_test.go`、`adapters/http/query_visibility_catalogues.go`、`ports/{ports,catalogue_read}.go`；条 2 三件——`application/register_catalog{,_test}.go`、`cmd/parcel-ve-register/main_test.go`；条 3 一件——`cmd/parcel-api/assemble_claims.go` 一行；条 4 两件——`adapters/partycommercial/claim_service_rules{,_test}.go`；条 5 一件——`docs/adr/0136-*.md`。

**判据逐项**（推送方在重放 tip `a34f439c` 上脚本核）：
1. `git grep -n -E '七件|六件必备|五个装载口|两组表' -- internal/visibilityexception` **0**；折行形 `五个$` 于 `ports/catalogue_read.go` **0** ✓。作者提交信自报：票面四处 + 同症补收 12 行、13 文件 38 行全注释——推送方核 `-U0` diff 里 `internal/visibilityexception` 与 `cmd/parcel-api` 除条 2 / 条 4 四件外的 `+`/`-` 行**非注释行 0** ✓。
2. `application\.CatalogRegistry|^type CatalogRegistry interface` 于 `application` + `cmd/parcel-ve-register` **0**；`ports.CatalogRegistry` 引用 **3 = 3**（`cdf17834` 同数）；新名 `CatalogRegistries` 引用 6；`go build ./...` 0 ✓。新名的理由在头注里：复数标明「几只端口合成的一面」，不是与 `ports.CatalogRegistry` 同名的第二只目录册写口。`production_wiring_baseline.txt` 零差 ✓。
3. `四个协作方` 于 `cmd/parcel-api` **0**；`claim_service_rules.go` 同文件那句仍在（2 处，皆同文件）✓。
4. `ErrNilDependency` 于 `adapters/partycommercial` 命中 9（声明 + 头注 + 四道包装 + 用例）；`go test -count=1 -run TestClaimServiceRulesRefuseToBeBuiltWithoutTheirCollaborators -v` **PASS**（四子例各 `errors.Is` + 文本含口名 + 返回 nil）✓。`RulesForClaim` 零 diff ✓。
5. `docs/adr/0136-*` numstat **1/1**——补记追加在越权风险点 2 **同一行末尾**，`git diff` 因此显示为改行而非增行；推送方核原句是新行的**逐字节前缀**（`StartsWith` 为真，225 → 1059 字符），「原句一字未改」成立；补记含「补记 2026-09-14」「`262e8c0a`」「ps-port-remainder/09」「0022」四件 ✓。判据原文「只有追加行」按行数字面不成立、按意图成立，如实记（见评审 Spec ②）。
6. 每笔 `gofmt -l` 空（重放 tip 上对 20 件 `.go` 核）、`go build ./...` / `go vet ./...` 0 ✓；作者不带 DSN（票面 Status 自记「真库未跑，368 例 SKIP」）；**推送方在 `a34f439c` 带 DSN 全量 `go test -p 1 -count=1 ./...` 110 ok / 0 FAIL / 15 无测试 / 0 cached**（09:37→09:39，133 s）✓；清点在干净检出 `tools/mechanism-inventory` 重生成 **porcelain 空**（改名不增删文件）✓。

**作者留下的判断项**（从提交信与票面 Comments 取，未见完工报）：① ve-disc 目录无 `spec.md`，不为一张尾巴票新造父规格（Comments 09:0x）；② 条 1 同症补收 12 行超出票面四处（与 ve-disc/04 先例同形）。推送方对两条都接受。**推送方另记一条**：`domain/eta_visibility_gap_test.go` 的用例名 `TestAnETADemandsItsSevenParts` 仍带「Seven」——标识符不是注释、不在条 1 红线「只许注释行与 `Covers:` 行」之内，作者不动是守票面；归 VE owner 候选后继（一处改名）。

## 参照

AGENTS.md「写代码注释」「改文档」；[ve-disc/02](./02-exception-disclosure-and-conflict-signal-rule-registries-have-no-cli-or-online-entry.md) 评审 Standards ②；[ve-disc/04](./04-ve-comments-cite-context-line-numbers-and-count-methods.md) 完成记录判断项 1 与评审 Standards ①；[ve-claims/04](../../ve-claims-read-seams/issues/04-rule-resolution-key-source-needs-a-registration-face.md) 评审 Standards ①② 与「裁决」5；[ps-port-remainder/09](../../ps-port-remainder/issues/09-resolution-key-face-does-not-accept-customer-service-rule.md) 后继 ③；同族先例 [sa-cc/16](../../sa-cc-funds-and-credential-seams/issues/16-sa-evaluation-request-and-duty-verification-adoption-standards-tails.md)。

## Comments

- 2026-09-14 09:0x · 通道 3（task-0962cb6b）：立票即认领。派单说「ve-disc spec.md 子票表加 05 行」——`.scratch/ve-disclosure-policy-view/` 在 `cdf17834` **没有 `spec.md`**（01–04 四张票都没有父规格，issue-tracker.md「Without a parent, activate the fully wired children directly」），故无处加行、也不为一张尾巴票新造父规格；如实记在此，推送方若要父规格另裁。
- 2026-09-14 09:3x · 通道 1 推送方：用户报通道 3「做完了，然后 crash」。git 实测：五笔已推 origin（`66c6168c` = origin），树上条 5（ADR 补记）与票面 Status 两件**已修改未提交**、完成记录节仍「待填」、无 `report_task done`、无完工报——与 09-11 晚 sa-cc/01「暂存与提交之间断掉」同一种丢法。封存 `chore(salvage)` `b9fbd0dd` 推 origin；父规格不另造（接受作者 09:0x 那条）。
- 2026-09-14 09:4x · **非作者评审 ← 通道 1 推送方**（钉 `b9fbd0dd` = 重放 tip `a34f439c` 同内容；通道 2 已 crash、通道 4 在做自己的票，无第三人）：
  - **Standards**：阻断 无。非阻断 ① `domain/eta_visibility_gap_test.go` 用例名 `TestAnETADemandsItsSevenParts`——「Seven」数的是 CONTEXT 那一句列了几件，与条 1 收掉的注释同病，只是长在标识符上；条 1 红线只许注释行，作者不动是对的，归 VE owner 一处改名。② `application/register_catalog.go` `NewCatalogRegistration` 拒 nil 文本仍是「catalog registry is required」而形参已叫 `CatalogRegistries`——文本没跟着名走；一字之差、零行为，归同一后继。无发现：13 件条 1 改动逐行核为注释 / `Covers:` 行（`-U0` 非注释差 0）；`CatalogRegistries` 头注把「为什么改名」写成了代码讲不出的取舍（按名认的工具分不开两包同名不同型）；`ErrNilDependency` 头注引 ADR-0079 决定八并点明「装配漏了 / 提供方坏了」恢复动作不同，形照 SA / PS 同名哨兵属实（`git grep -n 'ErrNilDependency = errors.New' -- internal`：accessidentity / CC / PS / SA 四处既有 + 本票一处，五处同形）；四道包装文本口名与 `ClaimServiceRulesDeps` 字段一一对应；用例改形后仍钉「返回 nil」；ADR 补记引 SHA 用短 SHA、引代码用符号名、引票用相对链接，无行号；注释全中文。
  - **Spec**：阻断 无。非阻断 ① 完成记录缺席（作者 crash）——由推送方按 git 与脚本代写，证据层级如实：判据 1–6 是推送方核的，「作者判断项」只取提交信与 Comments 里作者写过的话。② 判据 5「只有追加行」按 `git diff` 行数不成立（1/1 改行）——补记接在原句同一行末尾；推送方核原句为新行逐字节前缀，AGENTS.md「不改写已接受 ADR 历史」的要求成立；接受，不要求另起一行。无发现：五条各自出处与原评审条目对得上（ve-disc/04 判断项 1 / S①、ve-disc/02 S②、ve-claims/04 S①②、ve-claims/04「裁决」5）；条 1 同症补收 12 行属 ve-disc/04 先例接受过的扩围；「不在本票」四条（同文件计数、`RulesForClaim`、必登否、CONTEXT / UC）零触碰——`ports/ports.go` 头注「五个只读装载口」「两法」留着是对的（装载口就在同文件）；`RulesForClaim` 零 diff；`docs/domain/**` 零 diff；`apps/` 零 diff。
  - **结论**：可进 main；非阻断四条不阻，归 VE owner 候选后继两处标识符 / 文本随名改（① + ②），其余接受。
- 2026-09-14 09:4x · **进 main 记录（通道 1 推送方）**：`merge-base(main, mcp3-vetails) = cdf17834`，main 已前进两笔纯 .md（`098fe8d5` / `3ee64735`）→ `%TEMP%\idp-replay-vetails-0933` detached `3ee64735`，`cherry-pick cdf17834..mcp3-vetails` 六笔**零冲突**，tip 对作者分支 tip 除 main 自己那两笔外零差；`gofmt -l` 空、`go build ./...` / `go vet ./...` 0；09:35 占号，`a34f439c` 带 DSN 全量 **110 ok / 0 FAIL / 15 无测试 / 0 cached**（133 s），`-v` 探针 `TestClaimServiceRulesRefuseToBeBuiltWithoutTheirCollaborators` PASS 四子例；清点 `tools/mechanism-inventory` 干净检出重生成 porcelain 空；09:4x 释号。簿记一笔在其上（本票 Status「已进 main」前缀 + 完成记录代写 + 评审 + 本记录；psr/09 后继 ③ 划掉；tasks.md 节）；`ls-remote` 核 `3ee64735` 未动 → 共享树 `merge --ff-only` → `push <sha>:main`。SHA 对照：`80564f02→01df425c` / `fa9d76bd→2ca8a088` / `b59d8f74→c5959a1c` / `ecff1f67→cc9d86b2` / `66c6168c→b811a99e` / `b9fbd0dd→a34f439c`。`mcp3-vetails` → `merged/mcp3-vetails`（指针留本地、远端删）；作者树 `D:/tops/idp-parcel-mcp3-vetails`（干净 = origin）与重放树比内容后拆。**候选后继（归 VE owner）**：`TestAnETADemandsItsSevenParts` 改名、`NewCatalogRegistration` 拒 nil 文本随 `CatalogRegistries` 改口——两处一笔。
