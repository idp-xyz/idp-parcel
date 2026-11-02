# 03 客户对账单争议的管理台入口：客服代登记、财务核实后处理

Category: enhancement
Status: resolved——2026-11-01 进 main（全量 164 ok / 0 FAIL，种子退 0），见文末「评审记录」「进 main 记录」。此前 in-progress——2026-11-01 作者完工；评审（钉 `679392eaf`）阻断 3 条已修，复评（钉 `7eb16d2aa`）阻断清零，收尾两条已做，待进 main（后端票，作者只做「进 main 的步骤」第 1 步），分支 claude-disputes（从 origin/main `4aade8e49` 起，最后一笔见「完成记录·提交」）
父票：无。由票 [02](./02-statement-confirmation-and-disputes-have-no-production-face.md)（在 `claude-hometodo` 上）转来：02 列的两条缺口里，「争议的提出与处理没有生产口」由本票做；「相对方确认不记」用户裁定不做（ADR-0354 决定五），02 收口时指向本票。
Blocked by: 无
地盘：`internal/accessidentity` 两种决定种类与迁移 `access_identity/0037`；`internal/settlementaccounting/{domain,application,ports,adapters/postgres,adapters/registrationjson,adapters/http}` 的争议开立与处理、迁移 `settlement_accounting/0052`；`cmd/parcel-api` 结算决定特性文件、编排包事务与未接线占位；`apps/admin-web` 对账单页与授予常量；演示种子步骤 `510`、`data/access/34`、`35`、`data/governance/75`、`76`、种子 README；ADR-0354；SA CONTEXT 一句；UC-SA-003 一句规则、两条 AT、交接一行；AT 盘点两行；本票。
出处：用户 2026-11-01 经协调方裁定——「做管理台入口：客户电话或邮件提出异议，客服代登记，财务核实后处理；按授予收起。不做客户自助口。待确认应收（客户确认）不做」。

## 要做的

1. 照 UC-SA-003 接出登记争议、处理争议两个写口：HTTP 端点、parcel-api 装配、两种运营决定授予；幂等、并发、只对已发布单、终局不再改等失败边界照 UC；真库用例。
2. 对账单页右栏两张任务式表单，说人话，提交后显示结果，按授予收起（ADR-0315）。
3. UC「给开发的交接」、AT 与 AT 盘点。

## 完成记录

**起点** `origin/main` `4aade8e49`。分支 `claude-disputes`，只推分支不推 main。

### 怎么做的，为什么

- **两个写口**（ADR-0354 决定一、二）：`POST /settlement-statement-disputes`（`STATEMENT_DISPUTE_REGISTRATION`）与 `POST /settlement-statement-dispute-resolutions`（`STATEMENT_DISPUTE_RESOLUTION`），挂运营决定能力面，进 parcel-api 已装配的截单编排上的 `OpenDispute`、`ResolveDispute`，各包一笔事务。不过结算时点门：门管确认与截单什么时候能动，客户什么时候提异议不由它定。登记与处理分开授予——UC-SA-003 要求争议处理单独授权，客服与财务不是同一批人。
- **「作废之后提出的争议不登记在这张单上」是本票新加的规则**（初版是「作废的单不收新争议」，修订轮按协调方裁定改为比提出时刻与作废时刻），出处与理由：
  - 出处：SA CONTEXT 生命周期「对账单」——「已发布 → 已作废：只适用于整单不应继续有效的明确情况；原单继续保留并记录作废依据」「已作废 → 已替代：需要重新出具时形成新对账单并关联原单」；规则「整单无效时保留原单并建立作废和替代关系」。UC-SA-003 输入契约「争议请求」要求争议「指向明确范围」，CONTEXT「费用争议」词条说争议「只影响所指金额」。
  - 理由：作废说的是这张单从作废那一刻起整单不应继续有效；作废之后才提出的异议没有一张有效的单可以落，替代关系今天也还没做（`AT-SA-069` 交接行），不能转到哪张单上。作废之前提出的照收——客户作废前来电、客服作废后才录入，异议说的是当时有效的那张单。作废之前已开的争议不受影响、照旧能处理到终局（待对账读口照数它，`AT-SA-227`）。
  - 归类（ADR-0146）：这是机制，不是租户取值，对每个租户都一样；UC-SA-003 的 `PAR-SET-09`「争议规则」是租户参数，与它分开。
  - 它不越出 CONTEXT：CONTEXT 没有说作废单可以收新争议，也没有说不可以；本票把这一格定下来，并按「改领域语言才动 CONTEXT」的规矩在 SA CONTEXT「结算账户、对账与争议」补一句、在 ADR-0354 决定三写明，越权风险点 1 留给 SA owner 复核。评审若判它不忠于 CONTEXT，改法是删掉领域里那道门与 CONTEXT 那一句，接口形状不变。
- **失败边界照 UC**：同一份重放答 `EXISTING_DISPUTE`、同标识换内容答 `DISPUTE_CONFLICT`（既有编排）；费用不在单上、金额越行、单号不在册答 `SOURCE_NOT_ACCEPTED`（既有领域门）。新加两道：提出时刻早于发布不收（说的就不是这张单）；**已作废的单不收新争议**（`STATEMENT_VOIDED_TAKES_NO_DISPUTE`，ADR-0354 决定三，SA CONTEXT 同笔补一句），作废前已开的照旧。处理：待复核可再处理、终局后答 `DISPUTE_ALREADY_RESOLVED` 交回那一份（既有）；新加**读写之间另一人先落终局**时同样答已处理，不再答未受理（原先 `Replace` 答 false 一律落成未受理）。
- **争议编号由调用方给**：重发要落回同一项，编号得在第一次发出前定下；管理台在表单打开时铸一个、提交成功或换单才换。
- **操作证据**（决定四）：争议行加 `operation_evidence`（代登记的人）与 `resolution_evidence`（当前那一次处理的人），迁移 `settlement_accounting/0052`；只作审计，判据同 ADR-0340 决定三。
- **不做的**（决定五，用户裁定）：客户自助口；相对方确认（UC-SA-003 交接记一行，指 ADR-0354）。
- **管理台**：对账单页客户对账单签点一张单，右栏两张表单——登记（按客户费用读口的「所在对账单」列出这张单上的费用、客户不认的金额与单上同一写法、客户怎么说、提出时间）与处理（选一项未处理或待复核的争议、四种走向带一句人话说明、核实了什么）；答复译成一句话，提交后重读这一签。各按自己那一种授予收起（ADR-0315），收起处说谁能用。状态列「有异议待裁」把待复核也算进去；客户费用类型补读口早已透出的 `attributionDate`、`billedOn`。
- **演示种子**：步骤 `510` 给配置员登两种授予、准入对照、权威区间（`governance/75`、`76`），README 一句。

### 判据对用例

新立 `AT-SA-228`（登记）、`AT-SA-229`（处理），核过 origin 各分支最大号为 226、`claude-hometodo` 占 227；AT 盘点 ① 桶各一行，总览计数留给进 main 时重算（`at-coverage-counts -check` 只报生成物过期、行齐；`adr-index -check` 只报索引过期）。

| AT | 判据 | 用例 |
|---|---|---|
| SA-228 | 生产装配真库：201 开立、重放已存在、换金额冲突、越行/单号不在册/早于发布未受理、登记人落库、处理授予打登记口 403、作废后新争议答作废那一格且库上只有作废前那一项 | `TestTheDisputeRegistrationFaceRegistersAndRecordsTheClerk` |
| SA-228 | 领域：早于发布拒、发布那一刻收、作废单拒（作废那一格） | `TestADisputeNeedsAStillValidStatementPublishedBeforeItWasRaised` |
| SA-228 | Intake：按登记授予认证、逐格到命令、操作证据；金额零或带符号、理由空白或过长、时刻写坏、自报租户、缺费用都 400 不进编排 | `TestTheDisputeRegistrationIntakeTranslatesTheDisputeAndRecordsTheClerk` |
| SA-229 | 生产装配真库：待复核 201 → 拒绝 201 → 再处理 200 已处理交回拒绝、处理人落库且登记人不变、登记授予打处理口 403、不在册未受理 | `TestTheDisputeResolutionFaceResolvesOnceFinalAndRecordsTheDecider` |
| SA-229 | 编排：读写之间另一人先落终局答已处理并交回那一份 | `TestAResolutionLosingTheRaceToAFinalAnswersAlreadyResolved` |
| SA-229 | Intake：按处理授予认证、走向按原词认；走向写错、带金额 400 | `TestTheDisputeResolutionIntakeAuthenticatesItsOwnDecision` |
| 228/229 | 接入身份：两种授予登得进、读回各是各的一种 | `TestTheSettlementDecisionKindsAreGrantedAndReadBackSeparately` |

### 自验

- Go（含 PG）：`go build ./...`；`go vet ./cmd/parcel-api/ ./internal/settlementaccounting/... ./internal/accessidentity/...`；`gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/settlementaccounting/... ./internal/accessidentity/... ./internal/architecture/... ./cmd/parcel-api/...` 全绿（新真库用例以 `-v` 核过跑了）。
- 前端：`npx tsc -b`、`pnpm test`（0 败）、`npx vite build` 通过。
- 浏览器（5185，parcel-api 起在 19085、一次性库 `demo_disputes` 经 `demo-run/up.sh` 灌种子并跑 `10-main-chain`、`30-settlement` 发出一张单；为看首页那一格，在草稿区另起一棵临时树把 `claude-hometodo` 合进本分支、**未提交**，验完连树带库一起删）：首页「对账单 · 待对账 0」→ 对账单页点那张单，右栏空表提交逐格提示 → 选费用、填 1600、写客户怎么说，登记答 201「已登记争议 DSP-…，等财务核实处理」→ 首页变 1 → 「只看待对账」里点它、处理成待复核答 201，首页仍 1 → 再处理成拒绝答 201「已处理：拒绝。」，首页回 0。第二轮：金额填 9601 本地提示「不能超过这笔费用的金额 9600」；改 800 登记、首页 1；在「只看待对账」里处理成拒绝后那张单出列，右栏留着「已处理：拒绝。」并说这张单已不在当前列表里，首页回 0。全程无页面错误。没碰 `:5173`、`:19080` 与库 `demo_live`；进程已停、库已删。

### 留给协调方

- **与 `claude-hometodo` 合并时的暗礁**（临时树里实测；换基时已照此解，见下「换基」）：`statement_dispute.go` 的 `Replace` 两边都往参数表末尾加了一个 `$11`——本分支是处理人的操作证据，hometodo 是终局词形 `finalDisputeResolutions`。文本冲突好解，但两边都写 `$11`，解冲突时要把终局词形改成 `$12`（`AND (resolved_at IS NULL OR resolution <> ALL($12))`），否则语义错位。另有 `customer_statement.go` 的 import（`fmt` 与 `slices` 都要）、`ReconciliationPage.tsx`（两边各包了一层布局：外层左右两栏，左栏里放 hometodo 的「只看待对账」条与列表、列表加 `onRowClick`，右栏是本票的面板；`gone` 判据在合并后才真正用得上）、UC-SA-003 验收表与 AT 盘点（相邻插行，两边都留）。临时树里按上述解完，结算相关真库用例全绿、浏览器动线如上。
- ADR 号：初取 0353，place-display-names/01 先进 main 占了 0353，换基时改号为 **0354**（协调方指定；origin/main 与各 `claude-*` 远端分支都没占）。迁移 `access_identity/0037`、`settlement_accounting/0052`，种子 `governance/75`、`76`，换基后对 origin/main `0a88def13` 与各远端分支核过不撞。
- ADR-0354 越权风险点待 SA owner 复核，尤其决定三（作废单不收新争议是本票加的规则）。
- 票 02（在 `claude-hometodo` 上）收口时请指向本票：「争议的提出与处理」由本票做；「相对方确认」用户裁定不做，记在 ADR-0354 决定五。

### 换基（2026-11-01，协调方派）

- 换基到 origin/main `0a88def13`（hometodo 已进 main）。冲突照上文「合并暗礁」解：`Replace` 的终局词形留在 main 的 `$11`，处理人的操作证据接在 `$12`；`customer_statement.go` 的 import 两个都留；`ReconciliationPage.tsx` 外层左右两栏、左栏放 hometodo 的「只看待对账」条与列表并加 `onRowClick`、右栏是本票的面板（含 `gone`）；UC-SA-003 验收表两边的行都留，交接表里「争议的提出与处理没有生产口」那一行由本票补上、删掉，「相对方确认」那一行合成一行指 ADR-0354 决定五；AT 盘点两边的行都留。
- ADR 改号 0354（文件名、标题、正文、CONTEXT、UC、票面、注释与迁移头注），`git grep -n 0353` 只剩名称票那一张与它的引用。
- 参数位：真库用例 `TestAPendingReviewDisputeCanBeResolvedAgainUntilAFinalResolution` 补断言处理人那一列（待复核 → 拒绝 → 终局后再来一次，库上处理人停在落下拒绝的那一位）；把 `$11`、`$12` 对调一次实测为红（pgx 拒把终局词形编进 text 列），已还原。
- 换基后自验：Go 受影响包 + architecture + cmd/parcel-api 真库全绿；前端 `tsc -b`、`pnpm test`（1023 过 / 0 败）、`vite build` 通过；种子 `seed.sh` 灌一次性库 `seed_disputes` 退 0，配置员两种争议授予在册，验完删库。

### 修订轮（2026-11-01 作者，应评审意见，评审钉 `679392eaf`、阻断 3 条；协调方裁定非阻断一并修）

- **S1** 种子 README 两处本该指 ADR-0353（显示名称）的引用被换基改号误改成 0354，改回；`git diff 0a88def13 -- .` 里删去含 0353 的行核过一遍，别处没有同类误改。
- **P1** 登记落库撞主键（同一编号的另一份先落了库）那一支原先一律答 `EXISTING_DISPUTE` 并带回赢家，不比内容；改成与读时在册同一个判法（`CompareStoredDigest`）：同内容已存在、不同内容冲突。真库用例 `TestConcurrentRegistrationsOfOneDisputeAreJudgedByContent`：8 轮各并发一对金额不同、一对内容相同的登记；去掉这道比对时实测为红（输家答已存在并带回赢家的金额）。编排用例 `TestARegistrationLosingTheRaceIsJudgedByContent`。
- **P2**（协调方裁：改 ADR 贴合代码）ADR-0354 决定二删去「依据空白答 `SOURCE_NOT_ACCEPTED`」，另起一段写明两口的形状写坏一律在入口答 `400 MALFORMED_REQUEST`、不进编排。
- **P3** 作废判据改按提出时刻比作废时刻：提出在作废之后才拒，之前（含恰在作废那一刻）照收。领域、ADR-0354 决定三（含候选与反方、越权风险点 1）、SA CONTEXT 那一句、UC-SA-003 规则与 `AT-SA-228` 一并改；领域用例覆盖两边，真库用例 `TestADisputeRaisedBeforeTheVoidIsStillRegisteredAndOpenOnesStillResolve` 覆盖作废前提出、作废后登记照收与作废后提出拒收。
- **P4** 前端不再说「请对着替代单登记」（替代关系还没做）：改成「这张单已作废，作废之后提出的异议不再登记在这张单上」；作废单上照摆登记表单，并说明作废时刻与这条规则，提出时间在作废之后的本地就提示。
- **S3** ADR-0354 决定三按 ADR-0146 写明这条属机制，与 `PAR-SET-09`「争议规则」（租户参数）分开。
- **S2** UC-SA-003「给开发的交接」补两行：输入契约的证据与请求方没有格；审计的决定方只留最后一位、没有处理历史，登记人与处理人还没在读口透出（ADR-0354 越权风险点 2–4）。
- **P6** 真库用例补作废前已开的争议仍能处理到终局（同上那条用例的后半）。
- **P7** 登记表单的费用改取这张单自己的费用行：对账单目录行只带行数、不能按单过滤，费用册按页上列且认不出作废单，所以在 SA 读面补 `GET /settlement-statement-lines?statement=`（运营查阅读面，单上的不可覆盖快照，连同币种、发布与作废时刻；单不在册答 `STATEMENT_NOT_FOUND`），真库与端点用例各一，并入目录口拒绝共用组。处理答「已处理」（含终局后再处理）与登记答「已存在」时也重读这一签，右栏不留旧的待处理项。

自验（修订轮）：Go `build`、`vet`、`gofmt -l` 干净，settlement、accessidentity、architecture、cmd/parcel-api 真库全绿；前端 `tsc -b`、`pnpm test`（1024 过 / 0 败）、`vite build` 通过；`seed.sh` 灌一次性库 `seed_disputes2` 退 0、验完删库；`at-coverage-counts -check` 只报生成物过期（行齐）。没碰 `:5173`、`:19080`、`demo_live`。

### 收尾一笔（2026-11-01，复评钉 `7eb16d2aa` 阻断清零后协调方再收两条）

- **N1** 费用行读口不再另立一份行模型：`FindStatementLines` 直接用写侧 `customer_statement.go` 的 `statementLineRow` 解码，删掉同形状的 `statementLineDocument`——写侧改键名时读口跟着改，不会静默读空。
- **N2** 提出时刻加上限：不得晚于登记时刻（编排取服务端此刻，`OpenStatementDispute` 多一个 `registeredAt` 参数）。不留时钟容差——本仓别的操作者陈述的业务时刻（参与方关系的结束时刻不得晚于登记时刻）就是这样判的，没有容差惯例。超出答 `SOURCE_NOT_ACCEPTED`，前端答复话里补「也不晚于现在」，表单本地同一边界先拦。领域用例 `TestADisputeCannotBeRaisedAfterItIsRegistered`（恰在登记那一刻照收），真库用例在登记口那条里加「提出晚于登记」；既有真库用例的提出时刻改成已经过去的整秒（`raisedAfter`），编排夹具的时钟改成可拨，原有用例的数据与摘要金样不变。ADR-0354 决定一与越权风险点 5（提出时刻是操作者陈述的、不核真实性，作废判据可被往前填绕过，登记人与 `recorded_at` 留库可追查，与仓里同类业务时刻同一信任口径）、SA CONTEXT 那一句、UC-SA-003 规则、`AT-SA-228` 与交接「证据与请求方」那一行同步。
- 自验同前：Go 受影响包 + architecture + cmd/parcel-api 真库全绿；前端 tsc / test / build 通过；种子灌一次性库一次退 0、已删库。

### 提交

换基到 `0a88def13` 之前的那四笔（`8d9efbb5a`、`0320186fe`、`2a9083de4`、`9a12d0acc`）已被换基取代。现行：
- `34ec3d3e1` 后端两口、领域与编排、迁移、种子与用例
- `cc4b13a17` 文档：ADR、UC-SA-003、SA CONTEXT、AT 盘点、本票立票
- `afc024a2d` 管理台两张表单
- `ebf05445b` 右栏在单出列时不按旧数据摆表单、留住刚才的答复；本票完成记录
- `679392eaf` ADR 改号 0354、参数位用例、本票换基记录与新规则的出处
- `7eb16d2aa` 修订轮
- 收尾一笔（见 git log，紧接 `7eb16d2aa`）

## 评审记录（2026-11-01，隔离子代理，首轮钉 `679392eaf`，复评钉 `7eb16d2aa`）

首轮阻断 3 条，复评清零。首轮阻断：S1 换基改号把种子 README 里两处指显示名称（ADR-0353）的引用误改成 0354；P1 `OpenDispute` 的 `DisputeAlreadyOpened` 分支不比内容，真库探针同编号金额 100 / 200 并发 10 轮输家都答 `EXISTING_DISPUTE` 并带回赢家金额，违背 UC-SA-003 与 ADR-0354 决定一；P2 ADR-0354 决定二对空白依据的答法与代码不一致（协调方裁改 ADR 原句贴合代码：形状错误入口答 400）。修订轮 `7eb16d2aa` 全数处理：撞主键分支走同一份 `CompareStoredDigest`（复评重跑并发探针 10+10 轮，异内容每轮答 `DISPUTE_CONFLICT`、同内容每轮答已存在）；作废判据改按提出时刻比作废时刻（协调方裁，复评真库三格核过，CONTEXT、ADR 决定三、UC、AT-SA-228 四处同一说法）；作废单已开争议处理到终局有真库用例；新补单张费用行读口 `GET /settlement-statement-lines`（守卫与同文件目录口一致、租户隔离、读不到答 500、不在册答 `STATEMENT_NOT_FOUND`、走主键）；ADR 决定三写明属机制、与 `PAR-SET-09` 分开；UC 交接补证据 / 请求方与决定方历史两格；前端不再指向尚未存在的替代单。复评非阻断两条由收尾笔 `67699d74a` 处理（协调方核过 diff 未再派复评）：读口改用写侧 `statementLineRow`；提出时刻不得晚于登记时刻（同参与方关系结束时刻的口径），ADR-0354 越权风险点 5 写明提出时刻是操作者陈述、系统不核实、可经登记人与 `recorded_at` 追查。「作废单不收新争议」复评判为忠于 SA CONTEXT（CONTEXT 未写作废单收不收新争议，无冲突句；AT-SA-227「作废不了结争议」只管已开争议）。

## 进 main 记录（2026-11-01，推送方 tops-20 / idp-parcel-91）

- 分支 `claude-disputes` tip `67699d74a` 作出处；隔离树把 `0a88def13..67699d74a` cherry-pick 到 main `a69eafec6`，无冲突；重生成 ADR 索引、AT 盘点计数与机制清点各成一笔，两项 `-check` 退 0；对 `7cd3e6b58` 跑全量 164 ok / 0 FAIL（含 PG）；种子一次性库 `seed_disputes` 退 0 后删库；管理台 `tsc -b` 过、`pnpm test` 1040 / 0。本笔票面簿记只动 `.md`，叠在验过的树上推确切 SHA。
- 重放来的前几笔提交信息里写的是「ADR-0353」，是换号之前写的；本票的 ADR 是 **0354**（换号那一笔与票面「换基记录」有说明），0353 是 place-display-names/01 的显示名称。提交信息不改写。
