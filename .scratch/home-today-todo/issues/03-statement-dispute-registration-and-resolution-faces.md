# 03 客户对账单争议的管理台入口：客服代登记、财务核实后处理

Category: enhancement
Status: in-progress——2026-11-01 作者完工，待隔离评审与进 main（后端票，作者只做「进 main 的步骤」第 1 步），分支 claude-disputes（从 origin/main `4aade8e49` 起，最后一笔见「完成记录·提交」）
父票：无。由票 [02](./02-statement-confirmation-and-disputes-have-no-production-face.md)（在 `claude-hometodo` 上）转来：02 列的两条缺口里，「争议的提出与处理没有生产口」由本票做；「相对方确认不记」用户裁定不做（ADR-0353 决定五），02 收口时指向本票。
Blocked by: 无
地盘：`internal/accessidentity` 两种决定种类与迁移 `access_identity/0037`；`internal/settlementaccounting/{domain,application,ports,adapters/postgres,adapters/registrationjson,adapters/http}` 的争议开立与处理、迁移 `settlement_accounting/0052`；`cmd/parcel-api` 结算决定特性文件、编排包事务与未接线占位；`apps/admin-web` 对账单页与授予常量；演示种子步骤 `510`、`data/access/34`、`35`、`data/governance/75`、`76`、种子 README；ADR-0353；SA CONTEXT 一句；UC-SA-003 一句规则、两条 AT、交接一行；AT 盘点两行；本票。
出处：用户 2026-11-01 经协调方裁定——「做管理台入口：客户电话或邮件提出异议，客服代登记，财务核实后处理；按授予收起。不做客户自助口。待确认应收（客户确认）不做」。

## 要做的

1. 照 UC-SA-003 接出登记争议、处理争议两个写口：HTTP 端点、parcel-api 装配、两种运营决定授予；幂等、并发、只对已发布单、终局不再改等失败边界照 UC；真库用例。
2. 对账单页右栏两张任务式表单，说人话，提交后显示结果，按授予收起（ADR-0315）。
3. UC「给开发的交接」、AT 与 AT 盘点。

## 完成记录

**起点** `origin/main` `4aade8e49`。分支 `claude-disputes`，只推分支不推 main。

### 怎么做的，为什么

- **两个写口**（ADR-0353 决定一、二）：`POST /settlement-statement-disputes`（`STATEMENT_DISPUTE_REGISTRATION`）与 `POST /settlement-statement-dispute-resolutions`（`STATEMENT_DISPUTE_RESOLUTION`），挂运营决定能力面，进 parcel-api 已装配的截单编排上的 `OpenDispute`、`ResolveDispute`，各包一笔事务。不过结算时点门：门管确认与截单什么时候能动，客户什么时候提异议不由它定。登记与处理分开授予——UC-SA-003 要求争议处理单独授权，客服与财务不是同一批人。
- **失败边界照 UC**：同一份重放答 `EXISTING_DISPUTE`、同标识换内容答 `DISPUTE_CONFLICT`（既有编排）；费用不在单上、金额越行、单号不在册答 `SOURCE_NOT_ACCEPTED`（既有领域门）。新加两道：提出时刻早于发布不收（说的就不是这张单）；**已作废的单不收新争议**（`STATEMENT_VOIDED_TAKES_NO_DISPUTE`，ADR-0353 决定三，SA CONTEXT 同笔补一句），作废前已开的照旧。处理：待复核可再处理、终局后答 `DISPUTE_ALREADY_RESOLVED` 交回那一份（既有）；新加**读写之间另一人先落终局**时同样答已处理，不再答未受理（原先 `Replace` 答 false 一律落成未受理）。
- **争议编号由调用方给**：重发要落回同一项，编号得在第一次发出前定下；管理台在表单打开时铸一个、提交成功或换单才换。
- **操作证据**（决定四）：争议行加 `operation_evidence`（代登记的人）与 `resolution_evidence`（当前那一次处理的人），迁移 `settlement_accounting/0052`；只作审计，判据同 ADR-0340 决定三。
- **不做的**（决定五，用户裁定）：客户自助口；相对方确认（UC-SA-003 交接记一行，指 ADR-0353）。
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

- **与 `claude-hometodo` 合并时的暗礁**（临时树里实测）：`statement_dispute.go` 的 `Replace` 两边都往参数表末尾加了一个 `$11`——本分支是处理人的操作证据，hometodo 是终局词形 `finalDisputeResolutions`。文本冲突好解，但两边都写 `$11`，解冲突时要把终局词形改成 `$12`（`AND (resolved_at IS NULL OR resolution <> ALL($12))`），否则语义错位。另有 `customer_statement.go` 的 import（`fmt` 与 `slices` 都要）、`ReconciliationPage.tsx`（两边各包了一层布局：外层左右两栏，左栏里放 hometodo 的「只看待对账」条与列表、列表加 `onRowClick`，右栏是本票的面板；`gone` 判据在合并后才真正用得上）、UC-SA-003 验收表与 AT 盘点（相邻插行，两边都留）。临时树里按上述解完，结算相关真库用例全绿、浏览器动线如上。
- ADR 号取 0353：核过 origin 各 `claude-*` 分支无 035x 新号，主工作树未见 0350/0351 文件（协调方说已占），`claude-placenames` 未推、没法核；撞号请协调方改。迁移 `access_identity/0037`、`settlement_accounting/0052`，种子 `governance/75`、`76` 同理。
- ADR-0353 越权风险点待 SA owner 复核，尤其决定三（作废单不收新争议是本票加的规则）。
- 票 02（在 `claude-hometodo` 上）收口时请指向本票：「争议的提出与处理」由本票做；「相对方确认」用户裁定不做，记在 ADR-0353 决定五。

### 提交

- `8d9efbb5a` 后端两口、领域与编排、迁移、种子与用例
- `0320186fe` 文档：ADR-0353、UC-SA-003、SA CONTEXT、AT 盘点、本票立票
- `2a9083de4` 管理台两张表单
- 其后一笔：右栏在单出列时不按旧数据摆表单、留住刚才的答复；本票完成记录
