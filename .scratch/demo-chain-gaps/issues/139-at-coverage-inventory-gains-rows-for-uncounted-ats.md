# 139 AT 盘点约 160 个 AT 没有分桶表行，总览低报

Category: bug
Status: resolved——2026-10-26 进 main `effb2ce27`（见文末「进 main 记录」）。此前 in-progress——2026-10-26 评审无阻断、修订轮完，待进 main，分支 `claude-dcg139` tip `b0e523467`（已 rebase 到 origin/main `e4bd1b150`；票面那一笔在其上；见文末「评审记录」「修订轮」）。此前 in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-dcg139` tip `1f22fbb23`（rebase 前；票面那一笔在其上；见文末「完成记录」）。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领，分支 `claude-dcg139`（worktree `idp-parcel-dcg139`）。此前 ready-for-agent——2026-10-20 代决复核立（文档与工具），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M23；infra-speed/03 越权风险点 4

## 现象与修法

现象、证据与修法的完整写法以复核报告 M23 一条为准（单一权威），此处只记要点：

把这批补成分桶表行；在盘点「方法与口径」写明新 AT 只按表行计数，追记里的加数不再生效。

## 完成记录（2026-10-26，Claude Code 作者子代理，协调方 `tops-20`，分支 `claude-dcg139`，起点 `4f29f861e`）

### 提交

| SHA | 内容 |
|---|---|
| `6ceded3ea` | AT 盘点：补 159 行；「方法与口径」加「表行即计数」一条；文末追记 |
| `1f22fbb23` | `tools/at-coverage-counts`：表行与 UC 立号逐一对照，缺行或多行即不重算；Go 用例 |
| 票面那一笔 | 本完成记录与 Status |

### 做了什么

- **实测缺口**（一次性脚本放会话 scratchpad，不入库）：在 `4f29f861e` 上，`docs/application` 各 `.md` 里出现的不同 AT 号 1574 个，与「表格行首格恰是一个 AT 号」的立号集合逐一相同；盘点分桶表行 1415 个；立了号没有行的 159 个——NO-101–128、NR-054–063/070–072、PC-037–041/043–053、PS-094–105、SA-179–187、TF-098–109/120–144/146–154/166–189/201/206–215，与报告点名的段一致。反方向（表行有、UC 里已没有）为零。同一 AT 号在两处 UC 立号的冲突为零：首格重复的只有 `AT-PC-042`、`AT-PS-108`，都是同一份 UC 的「现状缺口」交接表回引本号，不是撞号。
- **逐个归桶**：照「方法与口径」原有判据。①：扫全仓 `*_test.go` 的注释与函数体（含 `AT-X-a`–`b`、`a 至 b`、`a/b/c` 简写），归到所在测试函数——134 个都有明文点名（没有一个只靠区间简写），入①，行内按文件分组列出测试函数（领域、编排、适配器、`cmd/` 的顺序），点名处写了「某某半边」的照原话注在函数后。其余 25 个没有任何测试点名，逐个对代码：机制都在，现有用例引的是 CONTEXT 硬句、UC 步骤或 ADR，入②并在行内指出机制与现有用例——PC-037–041、043–050（`domain.Authorize` 与 `AdjudicateCommercialAuthorizationHandler`；`AT-PC-050` 是 `ResolutionKey.ServiceProduct` 收窄候选）、PS-099（受控关闭被重开）、SA-182–187（BUY 评价经请求形成预期成本、发生项已形成信封发起 BUY 请求两条编排）、TF-100–102/104（周期安排与班次登记，ADR-0201）、TF-144（访问身份把容量预占与容量释放分作两种运营决定）。无一条归③④。
- **盘点「方法与口径」**加「表行即计数」一条：每个立下的 AT 号在分桶节恰有一行，总览只从表行重算；立号、改号、删号的票同一笔补行、改行或删行；文末追记里「各加 N」的加数不再生效、追记留作历史；工具重算时对照表行与立号。
- **文末追记**一条：本票补了哪些段、各入哪桶，并指回上面那条规则——没有逐条在旧追记后面加指针（理由见代决 7）。
- **工具守卫**：`tools/at-coverage-counts` 在写回与 `-check` 两种模式下，重算之前先收 `docs/application` 立下的 AT 号（`DeclaredATs`：表格行首格恰是一个 AT 号的行），与分桶表行对照（`Reconcile`）。对不上即不写总览、退出码 2（「生成物过期」仍是 1）；报文逐个列出缺行的 AT 与它立号的 UC 文件、多出的 AT 与它所在的桶，末尾写补法（按四桶判据补一行、改号改行、删号删行、追记加数不算）。用例：只认首格立号（正文提到的号、交接表并列几个号的首格、非 `.md` 都不算；`docs/application` 不在就报错，不交回空集）；两个方向的报文都列全且按号排；缺行时写回与 `-check` 都不重写总览、错误不是 `ErrStale`；补上那一行之后照常重算。

### 与报告相抵处

- 报告在 `9045b6fd` 上数的是 1380 对 1220；今天是 1574 对 1415，缺口 159，段与报告点名的一致，报告说的「TF 的多段」即上列六段。没有另长出来的缺口：此后各票都在表里补了行。
- 报告只点了 NO-101–112、126–128 的追记写着「各加 12/3」；同样只写了加数没补行的还有 NO-113–125（「各加 13」）、NR-070–072（「各加 3」），一并补上。

### 代决与越权风险点

1. **加守卫，而且写回与 `-check` 都挡。** 读了：`ci.yml` 那一步只在推到 main 时跑 `-check`；parallel-sessions「生成物不占号」与 workflow「进 main 的步骤」第 4 步（推送方在 tip 上重算、`-check` 退 0）；各在途分支的 AT 与表行（`claude-dcg127` 远端 tip 新立 `AT-TF-331`–`334` 而未补行，它的 worktree 里盘点文件有未提交改动，可能正在补；`claude-dcg142`、`dcg147`、`dcgdocs`、`psxm17`、`code01`、`code12` 与 main 同为只缺本票这 159 个）。没读：各在途分支未推的提交。只挡 `-check` 的话，推送方先跑写回会把少数了的总数写进文件、看上去成功，所以两种模式一起挡。影响：今后带新 AT 的分支若没补行，推送方重算时直接红，须补行（或退回作者）才能进 main；PR 与分支 CI 不受影响（那一门只挂 main 的推送）。owner 若不要：删 `run()` 里调 `Reconcile` 的那几行即回到只数行；只想挡 `-check`：把那几行包进 `if check`。
2. **立号的口径取「表格行首格恰是一个 AT 号」**，不取「`docs/application` 里任何一处出现」。今天两者是同一集合；取首格是为了正文顺带提到一个已改掉的旧号时不凭空要求一行。owner 若要后者：改 `declaredRow` 一个正则。
3. **缺行的退出码取 2 不取 1**：1 在本工具里专指「重跑生成器就能消掉」，缺行重跑消不掉。CI 只看非零，不受影响。
4. **25 条入②、不入④或③**：「判不准归保守那一桶」理解为不多认覆盖——②不声称有测试钉住，④反而会声称机制没落地，与代码不符；③要的是实例取值或闸门阻断，这 25 条都没有。其中 `AT-PC-044`（请求在形状上带不进授权引用，是否另要用例）、`AT-PC-049`（PS 各授权适配器随决定记下授权规则版本，无专门用例）在行内注明判不准。owner 若不同意：移那几行即可，计数随重算。
5. **①的半边注记只取点名句本身写明的**（`Covers: … 的某某半边`），由脚本从点名那一句截出，`AT-NR-071` 那一处「另一半」手改为「没绑目录那一半」；点名句没写半边的，表行也不写——这与既有行「半边覆盖在行内注明」的宁缺勿滥同口径，但不等于逐个函数复核过覆盖面。
6. **不动 `ci.yml`、`docs/agents/*`**：M23 指定规则写在盘点「方法与口径」，单一权威就在那里；`ci.yml` 那段注释说这一门「守的是推送方漏了那一步」，如今也守表行齐不齐，没去补那半句（共享文件，等 owner 定要不要）。
7. **不在每条「各加 N」追记后挂指针**：那样要动文末约二十处、正是多会话追记的热区；改为「方法与口径」一条规则加文末一条本票追记。
8. **新行插在各表按号排好的位置上**，不堆在表尾：与既有行序一致，只增不改，`git diff` 里全是 `+` 行；代价是 hunk 多，与在同一邻位补行的在途分支会有文本冲突（今天查到的在途补行都不在这些邻位）。

### 完成判据 / 修法 → 用例与证据

| 修法（M23） | 落点 | 证据 |
|---|---|---|
| 把没有表行的 AT 补成分桶表行 | 盘点①134 行、②25 行 | 一次性脚本在 `6ceded3ea` 上两个方向都为零；`go run ./tools/at-coverage-counts -check` 在本分支只报生成物过期，不报缺行 |
| 「方法与口径」写明新 AT 只按表行计数、追记加数不再生效 | 盘点「方法与口径」表行即计数一条 | 文本 |
| 防缺口再长（工具半边，本票代决） | `tools/at-coverage-counts` 的 `DeclaredATs`、`Reconcile` | `TestDeclaredATsTakesOnlyTheFirstCellOfATableRow`、`TestReconcileNamesEveryMissingAndExtraRowAndHowToFixThem`、`TestRunRefusesToRecountWhileARowIsMissing`；`TestRunRewritesOnlyTheOverviewAndCheckFollows` 改为带一份对得上的 UC。把 `run()` 里的对照临时短路，`TestRunRefusesToRecountWhileARowIsMissing` 即红 |

### 自验

- `go build ./...`、`go vet ./...` 过；改过的 `.go` 已 `gofmt -w`。
- 带 DSN `go test -count=1 ./tools/... ./internal/architecture/...` 全过（含 PG 环境变量；本票不动任何真库包与 `cmd/`，没有真库用例受影响）。
- `go run ./tools/adr-index -check` 退 0；`go run ./tools/at-coverage-counts -check` 只报生成物过期（预期，作者不重算）。拿 `claude-dcg127` 远端 tip 的 `docs/application` 与盘点试跑：列出缺行 163 个（本票 159 个加 `AT-TF-331`–`334`）、退出码 2。
- 在会话 scratchpad 的一份导出副本上试跑生成器（不入库），推送方重算后总览应为：①618→752、②224→249、③④不变、合计 1415→1574；NO 78→106、NR 80→93、PC 46→62、PS 102→114、SA 201→210、TF 227→308。

## 评审记录（2026-10-26，tops-20 隔离子代理，钉 `e18cf2ef1`）

照抄评审原文（原文的二级标题在此降为三级）：

评审 ← tops-20 隔离子代理 · 钉 `e18cf2ef1` · 00:52

结论：两轴无阻断；非阻断 4 条（含两处行注）加两条参考。

### Standards
阻断：无。
非阻断：
- S1 单一权威：「方法与口径」新加「表行即计数」一条里「新行按上面各桶的判据归：…机制在而测试未点名入②…机制未实现入④」改写了同节①–④定义且意思不同——原②只收「机制面较深的上下文（PS/PC/NO/NR/TF/SA）」，原④是「起步期上下文（CC/VE 为主）」；新句按「机制在不在」分。一条机制在、测试没点名的 CC 或 VE AT 按两句会进不同的桶（今天②里 CC、VE 两列都是 0）。同一段改写又抄进 `coverage.go` 的 `CoverageError.Error()` 补法。建议：规则句只写「按上面各桶判据归」；若本意改判据，写明取代哪几句。报文只指向「方法与口径」，不再复述。
- S2：`ci.yml`「ADR index and AT coverage counts are current」一步注释写「它守的是推送方漏了那一步」，现在不全——缺行也让这一门退 2 变红。该补半句；workflow 第 1 步要不要加「立号同笔补行」由 owner 定（规则现只写在 .scratch 盘点里）。
无发现：红线；注释中文；计数锚 SHA；无新 AT、ADR、迁移号。

### Spec
阻断：无。M23 两项修法都落了。
非阻断：
- P1 `AT-PC-049` 行注不准：写「没有专门用例」，但 `internal/partycommercial/adapters/postgres/authority_closure_test.go` 的 `TestARetiredAuthorizationRuleVersionLeavesTheGrantBook` 在 Covers 句里明写「退役不追溯已作出的那一次裁定」并有断言（以 042/059 名义、只断言内存值，「判不准」仍成立）。桶不动，行注指向这个用例。
- P2 `AT-PC-050` 行注漏了现成用例：UC 三格已由 `internal/partycommercial/domain/requested_service_product_resolution_test.go` 的 `TestADeclaredServiceProductNarrowsTwoProductsInOneScopeToTheDeclaredOne`、`TestWithoutADeclaredServiceProductTwoProductsInOneScopeStillConflict`、`TestADeclaredServiceProductWithoutAPublishedVersionIsNoApplicableBasis` 逐格钉住，只差点名。
- P3 用例两种回退抓不到：把 `declaredRow` 放宽成「首格以 AT 号开头」（去掉尾部 `\s*\|`）或「表格行里第一个 AT 号」，`go test` 仍全绿——夹具里并列首格头一个号 `AT-PS-001` 本来就立过号，也没有「AT 号不在首格、又没在别处立号」的行。另三种回退（短路 Reconcile、不报多出行、缺行错误包成 ErrStale）都会红。
- P4 首格口径残余：交接表首格单独回引已改掉的旧号会被当立号、要求补行（探针：UC-PC-003 加一行 `` `AT-PC-999` ``，工具退 2 报缺行）。报文只说「补一行」，没提「那一行不是立号就改首格」。是会响的方向；今天 `AT-PC-042`、`AT-PS-108` 回引的都是本号，不误判。
参考：`AT-TF-098` 是新补 134 行里唯一不在 Covers 句点名的（只在说明句括注），所引用例只钉「登记入队与保序」一半，行里没注半边——与「半边只照点名句注」一致。
无发现：工具守卫是票面之外的代决，已披露、回退办法写清，站得住。

### 取证（脚本在 scratchpad/rev139/）
完整性：`e18cf2ef1` 上立号 1574 = 表行 1574（①752、②249、③139、④434），两向差 0，无重复；diff 无 `-` 行；新增 159 行插在按号位置；①里两处乱序（CC-436→156、TF-265→226）是旧有。归桶：① 134 行全脚本核过（319 处 `文件::函数` 都存在、每 AT 至少一函数字面点名）；② 25 行测试无任何点名，抽核 PC-037/039/040/041/043/044/046/047/049/050、PS-099、SA-182–187、TF-101/102/104/144，用例与机制都在。无错归。工具：go test/vet/gofmt 干净；基线盘点+tip UC 写回与 -check 都退 2、文件不变、报文列 159 号与 UC 文件并给补法；tip 盘点写回退 0、合计 1574。在途分支各自立号缺行为 0；与 origin/main 合并时文末两条追记相邻冲突，两条都留即可。

## 修订轮（2026-10-26，作者子代理，同一分支 `claude-dcg139`）

评审无阻断，非阻断 S1、S2、P1–P4 全收，一笔 `b0e523467`（rebase 前 `ad8da9dd3`）：

| 条 | 改了什么 |
|---|---|
| S1 | 盘点「方法与口径」的「表行即计数」一条去掉对①–④判据的复述，只写「新行按上面各桶的判据归，判不准的在行内注明」，同节①–④定义一字未动；`CoverageError.Error()` 的补法同样只指向「方法与口径」的分桶判据，不复述。本票 25 条②照原②判据（PS/PC/SA/TF、未被测试引用、行文无实例与闸门关键词）也落在②，归桶不变 |
| S2 | `.github/workflows/ci.yml`「ADR index and AT coverage counts are current」一步的注释补半句：UC 立了号而盘点分桶表缺行或多行时 `at-coverage-counts` 退 2，这一门同样红，要补表行。`docs/agents/workflow.md` 不动：**是否在「进 main 的步骤」第 1 步加「立号同笔补行」留 owner 定**（规则今天只写在 `.scratch/at-coverage-inventory.md`「方法与口径」） |
| P1 | `AT-PC-049` 行注改指 postgres `authority_closure_test.go::TestARetiredAuthorizationRuleVersionLeavesTheGrantBook`（Covers 句写「退役不追溯已作出的那一次裁定」并有断言，但以 `AT-PC-042`、`AT-PC-059` 名义、只断言内存值），仍注判不准，桶不动 |
| P2 | `AT-PC-050` 行注改指 domain `requested_service_product_resolution_test.go` 逐格钉住三格的三条用例，只差点名。核过：三条用例与所在文件都没有 `AT-*` 字样，按①「测试点名」的口径不进①，桶不动 |
| P3 | `TestDeclaredATsTakesOnlyTheFirstCellOfATableRow` 的夹具：并列首格改为以未立号的 `AT-PS-901` 开头，加「首格号后带字」（`AT-PS-903` 一行）与「首格不是号、后格出现未立号的 `AT-PS-904`」两行。把 `declaredRow` 临时放宽成去掉首格尾部 `\s*\|`、或取表格行里第一个 AT 号，该用例都红（分别多收 901/903 与 901/903/904），复原后全绿 |
| P4 | 报文补法加一句：UC 里那一行首格只是回引别处的号、不是立号的，改首格写法（例如写成「`AT-X-001` 一行」），不补行；用例断言报文含这一句与「方法与口径」 |

**rebase 到 origin/main `e4bd1b150`**（main 期间进了票 147、17、142/143、文档批、127/48/167 等，28 笔）：只在盘点文末追记处冲突一处——main 一侧新增「票 docs-consistency/code/01」「票 demo-chain-gaps/127、48、167」两条追记，本票一侧新增本票追记；两边都留，main 的两条在前、本票的在后。表行区没有冲突，main 新补的 `AT-TF-331`–`334` 等行与本票插入的 159 行都在。rebase 后本票各笔：`6ceded3ea`→`045cf5cb8`、`1f22fbb23`→`762e37ef4`、`e18cf2ef1`→`8c9c014ea`、修订 `ad8da9dd3`→`b0e523467`；相对 origin/main 的盘点 diff 没有 `-` 行。

**rebase 后的自验**（tip `b0e523467`）：

- `go run ./tools/at-coverage-counts -check` 只报「生成物与文件不一致」（预期，作者不重算），不报缺行或多行；一次性脚本两向复核：立号 1578 = 表行 1578，两向差 0。
- `go build ./...`、`go vet ./tools/...` 过，`gofmt -l tools/` 空；`go test -count=1 ./tools/...` 全绿。
- 各桶行序：除 ①里两处旧有乱序（CC-436→156、TF-265→226）外按号排。

## 进 main 记录（2026-10-26，推送方会话 tops-20）

- **main**：`effb2ce27`。出处：分支 `claude-dcg139` tip `bab001471`（作者修订轮后自行 rebase 到 main `e4bd1b150`，推送方重放为快进，SHA 不变：`045cf5cb8` 补行、`762e37ef4` 工具守卫、`8c9c014ea` 完成记录、`b0e523467` 修订轮、`bab001471` 票面；rebase 前出处 `6ceded3ea`、`1f22fbb23`、`e18cf2ef1`、`ad8da9dd3`）。
- **生成物**：AT 盘点总览计数 `effb2ce27`（守卫生效后的首次重算，立号与表行两向差 0）；ADR 索引与机制清点无变化。三件 `-check` 退 0。
- **评审**：隔离子代理钉 `e18cf2ef1`，两轴无阻断，非阻断 S1、S2、P1–P4 在修订轮全收（见「评审记录」「修订轮」）。修订轮只收非阻断，推送方读了修订 diff，未另派复评。
- **全量真库测试**：`scripts/test-full.sh` 跑在 `effb2ce27`（即所推 SHA）：158 ok / 0 FAIL，退 0。
- **种子**：`scripts/demo-seeds/seed.sh` 灌一次性库（`effb2ce27` 上），退 0，用后删库。
- **生效提醒**：自本笔起，任何分支新立 AT 而盘点无表行，推送方重算 `at-coverage-counts`（写回与 `-check`）都退 2；已向另一会话 tops-26 广播。
