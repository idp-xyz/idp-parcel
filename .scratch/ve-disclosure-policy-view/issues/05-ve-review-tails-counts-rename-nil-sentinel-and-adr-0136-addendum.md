# VE 评审尾巴 A 类合票：四处跨文件计数换点名、`application.CatalogRegistry` 跨包同名改名、`assemble_claims.go`「四个协作方」去数、`NewClaimServiceRules` 拒 nil 包哨兵、ADR-0136 越权风险点 2 补一句实测

Category: chore
Status: resolved——2026-09-14 09:3x 通道 3 作者完工（task-0962cb6b；分支 `mcp3-vetails` 基远端 main `cdf17834`，认领 `80564f02`，五条五笔 `fa9d76bd` / `b59d8f74` / `ecff1f67` / `66c6168c` / 条 5 与本完成记录同笔，SHA 见完工报）：条 1 票面四处 6 行 + 同症补收 13 行全部注释、零行为；条 2 `application.CatalogRegistry` → `CatalogRegistries`，清点与接线基线零差；条 3 一行注释；条 4 铸 `ErrNilDependency` + 用例 `errors.Is`，先红后绿；条 5 ADR-0136 越权风险点 2 纯追加。判据 1–6 全过，真库未跑（无 DSN，368 例 SKIP 全是 `IDP_PARCEL_POSTGRES_DSN` 门禁）。等非作者评审 → 推送方重放进 main。此前 in-progress——2026-09-14 09:0x 通道 3 按通道 1 派单 task-0962cb6b 自立自做（先例 sa-cc/16、lc/37、psr/08：已进 main 的票在非作者评审里点出、评审者与推送方认可的 A 类，推送方派单、作者自立票自做；用户 08:5x 经队列裁「派吧」）；隔离树 `D:/tops/idp-parcel-mcp3-vetails`，分支 `mcp3-vetails` 基远端 main `cdf17834`；取证锚同
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

## 完成记录

（待填：逐笔 SHA、动过的文件、五条判据逐项、判断项、验证、「真库未跑」。）

## 参照

AGENTS.md「写代码注释」「改文档」；[ve-disc/02](./02-exception-disclosure-and-conflict-signal-rule-registries-have-no-cli-or-online-entry.md) 评审 Standards ②；[ve-disc/04](./04-ve-comments-cite-context-line-numbers-and-count-methods.md) 完成记录判断项 1 与评审 Standards ①；[ve-claims/04](../../ve-claims-read-seams/issues/04-rule-resolution-key-source-needs-a-registration-face.md) 评审 Standards ①② 与「裁决」5；[ps-port-remainder/09](../../ps-port-remainder/issues/09-resolution-key-face-does-not-accept-customer-service-rule.md) 后继 ③；同族先例 [sa-cc/16](../../sa-cc-funds-and-credential-seams/issues/16-sa-evaluation-request-and-duty-verification-adoption-standards-tails.md)。

## Comments

- 2026-09-14 09:0x · 通道 3（task-0962cb6b）：立票即认领。派单说「ve-disc spec.md 子票表加 05 行」——`.scratch/ve-disclosure-policy-view/` 在 `cdf17834` **没有 `spec.md`**（01–04 四张票都没有父规格，issue-tracker.md「Without a parent, activate the fully wired children directly」），故无处加行、也不为一张尾巴票新造父规格；如实记在此，推送方若要父规格另裁。
