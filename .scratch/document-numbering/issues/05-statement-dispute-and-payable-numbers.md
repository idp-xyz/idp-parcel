# 05 对账单、费用争议、审核应付的单据号

Category: enhancement
Status: resolved——2026-11-02 进 main（全量 174 ok / 0 FAIL，种子退 0），见文末「评审记录」「进 main 记录」。此前 in-progress——2026-11-02 作者完工、自验、推分支 `claude-docnum05`；评审修订已追加（见文末「评审修订」）；待复评与进 main
父票：[spec](../spec.md)
Blocked by: [02](./02-platform-numbering-adoption-and-rule-register.md)
出处：spec 子票表；[ADR-0355](../../../docs/adr/0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md) 决定二、九、十；UC-SA-003、UC-SA-004

## 本票的特殊处

- **对账单**（决定九）：「每类单据都要做到」第 3 条（加可空单据号列与无号原因）与第 7 条（无号注明）不适用于对账单——`statement_number` 照旧非空、就是单据号，本决定之前发布的对账单照旧显示它的单号，不注任何「无单据号」。单号在发布事务内按 `CUSTOMER_STATEMENT` 分配，仍是（租户，单号）寻址键；发布幂等从按单号改为按发布意图（结算账户、周期、截止版本），意图到单号的关系由本上下文持有；本决定之前截出的对账单按意图同样查得到，升级后重截同一截止日答原单。手工截单发布口的载荷去掉「单号」一格；自动截单不再拼 `STMT/<账户>/<YYYYMMDD>`，账期 `CYCLE/<截止日>` 与版本 `v1` 照旧（ADR-0339 两句已由 ADR-0355 停用）。已发布对账单的单号不变。时区按结算账户对账安排的业务时区（ADR-0269）。
- **费用争议**：登记时取号；登记载荷里的争议标识仍由管理台在首次提交前生成，做幂等键（ADR-0354）。
- **审核应付**：通过审核的金额形成审核应付时取号；审核载荷里的应付标识改由调用方生成做幂等键（决定十），不再让人编号。
- 管理台：对账单页、争议右栏、应付所在页照「每类单据都要做到」第 7 条。

## 每类单据都要做到（[ADR-0355](../../../docs/adr/0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md) 决定三、七、八、十一）

1. **类型与默认**：类型代码与默认模板照 02 已声明的用，不在本票另写一份。本票为这几类定日期所用的业务时区（决定七），写进本上下文 `CONTEXT.md` 那句规则。
2. **开立事务内取号**：经本上下文自己的消费侧端口（ADR-0025）调 02 的取号编排，取号与单据落库同一事务；开立路径今天不在事务内的，补上事务壳；回滚即释放号码。取号只放在确定要落单的那一步，答业务答案而不落单、事务却照常提交的分支不得先取号（决定三）。开立方没有业务时区、租户也没登单据编号时区时，单据照常开立、不带单据号（决定七）。
3. **落库**：单据表加一列可空的单据号与一格无号原因（旧行留空即「上线前开立」，因单据编号时区未配置而无号的记该原因），与（租户，单据号）唯一约束；迁移要能在有数据的库上施加，旧行不回填。
4. **重放与生命周期**：同一开立意图重放答原号，不再取号；换版、补件、重开不换号；作废后的替代、拆出的关联新单据取新号。
5. **撞号**：已用的号由 02 的取号编排跳过（决定八）；跳过到上限仍撞时这一次开立不落单，本上下文写入结果代数加一格「编号规则撞号」，答复说人话、指到规则册。唯一约束照样建，作最后一道保险。
6. **答复与读口**：开立答复、列表与详情读口带单据号；没有单据号的旧单据不带这一格，不答空串、不拿内部标识顶。
7. **管理台**：这几类单据所在的页以单据号为主列、可按单据号检索，内部标识进技术详情；没有单据号的单据照旧显示内部标识，技术详情按单据上记下的无号原因注明「上线前开立，无单据号」或「开立时单据编号时区未配置，无单据号」，不一律写成前者。
8. **测试**：领域与应用单测；真库用例覆盖同一开立重放答原号、并发开立不撞号、租户隔离、周期重置、开立回滚释放号码、撞号答案。演示种子灌一次性库后，种子开立的这几类单据都带号。
9. **文档**：所引 UC「给开发的交接」里本票那一行改成「已补」并写清落在哪；在所引 UC 立 AT 行，AT 盘点照 `.scratch/at-coverage-inventory.md` 惯例加行（不手改总览计数）。

## 工作方式

只做 `docs/agents/workflow.md`「进 main 的步骤」第 1 步：作者在自己的 worktree 与分支上做完、自验、推分支、写完成记录；非作者评审与进 main 由协调方做。注释中文、无行号与跨文件计数；不手改 `docs/adr/README.md`、AT 盘点总览计数、机制清点。

## 完成记录（2026-11-02，作者，分支 `claude-docnum05`，从 origin/main `3a408b7b0` 起）

三笔提交：`d711c3933`（后端：领域、端口、编排、适配器、迁移、装配与各层用例）、`4694ef9e2`（管理台与演示脚本）、`9775b6a21`（CONTEXT、UC、AT 盘点），加本记录一笔。

### 做了什么

1. **取号端口**：本上下文立消费侧端口 `ports.DocumentNumberAssigner`（三格答案：取到号 / 单据编号时区未配置 / 编号规则撞号），适配器 `adapters/documentnumber` 把编号组件的 `docnumber.Assigner` 逐格全函数翻译过来（多出一格响亮报错）。parcel-api 的 `buildSettlementOrchestrations` 从 `endpointSources.documentNumbers.Assigner` 取、parcel-dispatch 的 `cutOffStatementsOnCycle` 从 `dispatchSources` 那一套取，都不另装；没装编号组件就装不起来。
2. **对账单**（决定九）：`PublishStatementCommand` 去掉单号。截单编排先在事务级咨询锁上排队同一发布意图（租户、结算账户、账期、截止版本），再按意图查重：同内容答原单原号，异内容答 `PUBLICATION_INTENT_CONFLICT`（原 `STATEMENT_CONFLICT` 改名，自动截单照记截止日），都不取号；勾稽不平、本期无费用、截单未配置也都答在取号之前（领域新增 `StatementDraft.CheckPublication`，判勾稽不要单号）。确定落单才经端口按 `CUSTOMER_STATEMENT` 取号，日期按结算账户对账安排（最新一版）的业务时区。手工截单发布口载荷去掉 `number`（带了按未知字段拒）；自动截单不再拼 `STMT/<账户>/<YYYYMMDD>`，账期 `CYCLE/<截止日>` 与 `v1` 照旧。
3. **费用争议**：登记时先排队同一争议标识再查重，重放读回原号不再取号；领域门拒了的（未受理、单已作废）与冲突都不取号；号的日期按被争对账单那个账户对账安排的业务时区，账户读不到对账安排时交空、退到租户单据编号时区。
4. **审核应付**：先排队同一应付身份与同一行（两把锁，次序固定），越权上限判过、确定形成应付才取号；日期按应付归入账户的对账安排业务时区，没登交空退到租户单据编号时区。载荷里的应付标识照旧是调用方生成的幂等键。
5. **时区未配置 / 撞号 / 出错**：争议与应付时区未配置照常开立、记无号原因；对账单落到这一格交回 `ErrStatementNumberingTimeZoneMissing`（单号是寻址键、非空，决定七说它落不到这格，走到就是矛盾）。撞号答 `NUMBERING_RULE_COLLISION`（对账单与争议是 `StatementNumberingRuleCollision`，应付是 `PayableNumberingRuleCollision`），parcel-api 三个事务壳认出它整笔回滚再交回答复；自动截单交回 `ErrCycleCutNumberingCollision`，这一户整笔回滚、不记截止日，日志分类「document numbering rule collision」。取到号之后落库失败或撞上没排队的同键写入，交回 `ErrNumberedIssueInterrupted` 让事务回滚，不答未决让号随提交成空号。
6. **落库与迁移** `settlement_accounting/0053`：`customer_statement` 加 `draft_version` 与部分唯一索引（租户、账户、账期、截止版本）；自动截单旧单按单号形状（`STMT/<账户>/<截止日去横线>` 且账期 `CYCLE/<截止日>`）补 `v1`，手工旧单当时没记版本、留空。`statement_dispute`、`audited_payable` 各加 `document_number`、`document_number_absence` 两列与（租户，单据号）部分唯一索引；有号只填前一列，时区未配置只填后一列，两列都空读作上线前开立；旧行不回填。新写的争议、应付没有「号或无号原因」即拒写，新写的对账单没有截止版本即拒写。
7. **答复与读口**：发布答复的 `statement` 是服务端取的单号；争议登记与处理答复、审核答复带 `documentNumber` 或 `documentNumberAbsence`（恰出其一，没号不出空串、不拿内部标识顶）；撞号答复带 `documentType` 指名规则册上哪一类。对账单册与「待对账」读口上每项争议带 `documentNumber` / `documentNumberAbsence`。
8. **管理台**：对账单页单号列不动（对账单号就是单据号，第 3、7 条不适用）。右栏新增「这张单上的争议」：争议号为主标识，争议标识与无号原因进技术详情，两种原因分开说（词典新增 `document-number-absence` 册）；处理表单选项与左栏技术详情「异议」一格改用争议号；搜索框可按争议号找到它所在的单；登记答复说争议号，撞号说人话并指到「设置 → 单据编号」。管理台没有列审核应付的页，应付号今天只在审核答复里。
9. **文档**：SA CONTEXT 三句规则写明各自日期所用时区；UC-SA-003、UC-SA-004 本票两行交接改「已补」并写清落点；立 `AT-SA-230`（争议号）、`AT-SA-231`（应付号），`AT-SA-222`、`AT-SA-224` 由新代码兑现；AT 盘点加两行、两行补用例（总览计数未动）。演示脚本 `demo-run/30-settlement.sh` 发布载荷去掉单号、从答复读回。

### 关键取舍与理由

- **争议、应付按结算账户业务时区**（票 02 留给本票核定）：争议所在对账单的账户必有对账安排（决定七），争议号与单号落在同一份账期日历里；应付账户对账安排不限方向、登了就是那个账户的业务时区，没登再退到租户单据编号时区——两类都改声明为 `DateTimeZoneFromIssuer`，`cmd/internal/docnumbering` 对照用例随之改。
- **意图记在对账单行上，不另立表**：「意图到单号的关系由本上下文持有」用一列截止版本加部分唯一索引就够，按意图查与按单号查读同一行。手工旧单当时没记截止版本，按意图查重时只能按内容认（摘要含截止版本，相同即同一意图）；内容不同分不出是换了版本还是换了内容，照新意图发——已在旧单上的费用照样被逐件排除，不会两张单各收一次。
- **排队锁而不是撞库后读赢家**：取号在开立事务里占序列行锁到提交，后到的同键请求若在取号之后才撞唯一约束，号已取走、事务又照常提交，就留下决定三不许的空号。所以三类都先用事务级咨询锁按幂等键排队再查重，赢家提交之后输家查得到、答已存在或冲突，一个号也不取；锁之后仍撞同键的分支交回错误回滚。代价是同键请求串行，与序列行锁本来的串行同量级。
- **「是否已用」走读执行器**：判撞要看见本事务里已落下的单，`ReadExecutor` 随 ctx 落在开立事务上；事务外调用已先被编号组件拒。改用读执行器也让结构门禁（无事务负向证据）只管三把锁，三把锁各有事务外即拒的用例。
- **结果代数**：撞号是业务答案（200），三个事务壳与自动截单都回滚；`STATEMENT_CONFLICT` 改名 `PUBLICATION_INTENT_CONFLICT`，照 ADR-0355 决定九的词。

### 验证（2026-11-02，本机 PG `127.0.0.1:55432`）

- `go build ./...`、`go vet ./...` 退 0；`gofmt -l` 对改过的 Go 文件无输出；`go test -count=1 -p 1 ./internal/architecture/... ./internal/platform/... ./cmd/... ./migrations/... ./internal/settlementaccounting/...` 全绿（50 个包 ok、0 FAIL，含 PG）。`-v` 抽查新增真库用例全 PASS、无 SKIP：`adapters/postgres` 的 `TestADisputeNumberIsIssuedInTheRegistrationTransaction`（重放答原号、回滚释放、8 路并发不撞号、同一标识两份并发只取一个号、租户隔离、UTC 11-30 / 上海 12-01 按账户时区月度重置、撞号不登记且回滚释放、对账单册带出争议号、库上唯一）、`TestADisputeWithoutANumberKeepsTheReason`、`TestAStatementNumberIsIssuedInThePublicationTransaction`（勾稽不平不取号、重放原号、意图冲突、同意图两笔并发只出一张）、`TestAnAuditedPayableNumberIsIssuedInTheAuditTransaction`、`TestTheStatementStoreKeepsOneStatementPerIntent`、`TestIssuanceLocksRefuseToRunOutsideATransaction`；parcel-api 结算各装配用例（含新 `TestAPayableNumberCollisionFormsNothingAndReleasesTheSequence`：租户换格式、前 20 个号已用，答撞号、不形成应付，腾出第一个号后再审拿到它）；parcel-dispatch `TestTheDispatchBeatCutsADueCycleIntoAPublishedStatement` 与新 `TestADispatchCycleCutStopsOnANumberingCollisionAndReleasesTheSequence`（撞号整户回滚：不出单、不记截止日、补判的归属日也不落）。
- 前端：`npx tsc -b` 无输出，`pnpm test` 1042 pass / 0 fail，`npx vite build` 退 0。`bash scripts/ci/generated-artifacts-pr.sh` 退 0（AT 计数、机制清点过期，留推送方重生成）；`go run ./tools/at-coverage-counts -check` 只差总览计数（表行与 UC 对得上）。
- **迁移施加在有数据的库**：一次性库先用 origin/main 的迁移助手迁到 0052，直写三张旧单（自动截单形状一张、手工一张、单号与账期对不上的一张）、一项旧争议、一笔旧应付，再用本分支迁移助手施加：只施加 0053 一步；自动截单那张补成 `v1`，另两张留空；旧争议、旧应付两列皆空；三条唯一索引在；再跑 `applied=0`。用后删库。未碰 `demo_live`。
- **种子**：一次性库 `docnum05_seed` 上 `scripts/demo-seeds/seed.sh` 退 0。种子不开立这三类单据（库里对账单、争议、审核应付都是 0 行），「种子开立的都带号」无对象可查；带号由下面的浏览器核验与真库用例给出。
- **浏览器**：自编 parcel-api 起在 :19095 连 `docnum05_seed`，前端 :5195。往库里直写两笔已确认费用、一张手工旧单与它上面的两项旧争议（一项两列皆空、一项记时区未配置），经发布口发单：答 `STMT-202611-00001`，重放答同号，载荷带单号答 400；库上序列 `CUSTOMER_STATEMENT/202611` 值 1。管理台（Dex 回调照 rd-ops/shots.js 浏览器侧改写）在对账单页点那张单、右栏登记一项争议：答复「已登记争议 DSP-202611-00001」，右栏列表以 `DSP-202611-00001` 为主、技术详情是争议标识；点旧单，两项分别注明「上线前开立，无单据号」「开立时单据编号时区未配置，无单据号」；搜索框输入 `DSP-202611-00001` 只剩它所在的那张单（`innerText` 核）。验完停了两个进程、删了库。

### 留给协调方

- **共享面**：词典 `apps/admin-web/src/vocabulary/status.ts` 追加了 `document-number-absence` 一册（两个码）；04、06–09 若也加同名册或同码，合并时取一份，词与色调以先进 main 的为准。HTTP 答复字段名用 `documentNumber` / `documentNumberAbsence`（撞号时 `documentType`），各票最好对齐。
- 管理台没有列审核应付的页，票面「应付所在页」无处可改；应付号今天只在审核答复里（`AT-SA-231` 已注明）。是否另立应付列表页由协调方定。
- 管理台在首次提交前生成的争议标识形如 `DSP-<时间>-<随机>`，与争议号默认模板 `DSP-{yyyy}{mm}-{seq:5}` 同前缀，技术详情里两串长得像；要不要换内部标识的前缀（只动 `dispute-forms.ts` 的 `newDisputeId` 与它的用例）请裁。
- API 可见改动：截单发布口载荷不再收 `number`；发布意图冲突的 outcome 从 `STATEMENT_CONFLICT` 改名 `PUBLICATION_INTENT_CONFLICT`（自动截单截止记录也记新名）。
- 推送方在 main 的 tip 上重生成 AT 盘点总览计数（①、SA 各 +2）与机制清点。

## 评审修订（2026-11-02，作者，应协调方转来的评审意见，评审钉 `df4da3184`）

追加提交、不 rebase：`58cc313cc`（并入 main `620847516`）、`880233730`（管理台）、`214dd3a6b`（后端）、`484c04d10`（文档）、`c7f3232da`（再并入 main `e110f4137`，含 07、08，无冲突）。

1. **单一权威（阻断）**：撤掉本票自加的 `status.ts` `document-number-absence` 册（并入 main 时 `status.ts` 冲突取 main 的 `document-numbering-source` 册）与 `registrationAnswerText` 里自写的撞号说法，改用 `vocabulary/document-numbering.ts` 的 `documentNumberAbsenceNote`、`numberingCollisionNote`；争议号的叫法（右栏技术详情的行名、搜索框占位）取 `documentTypeWords.STATEMENT_DISPUTE.numberName`。`issuerWordsByType` 补 `STATEMENT_DISPUTE`、`AUDITED_PAYABLE` 两条「结算账户」，`rows.test.ts` 加一行钉住。
2. **手工旧单取保守（协调方裁定）**：`findPublished` 按记下的截止版本找不到时，同账户同账期上没记截止版本的旧单里有内容相同的就答原单原号；没有而有一张未作废的这种旧单，交回它、由 `answerExisting` 答 `PUBLICATION_INTENT_CONFLICT`，不落单不取号；旧单都已作废不挡。写进 SA CONTEXT 对账单那句与 UC-SA-003（`AT-SA-222` 预期结果、交接行）。用例：编排半边 `TestAStatementPublishedBeforeNumberingIsFoundByItsIntent`；真库 `TestAManualStatementBeforeNumberingHoldsItsPeriod`（同意图、换截止版本换内容都答冲突，序列一行没动；同内容答旧单原号；旧单作废后替代单取 `STMT-202611-00001`）。
3. **迁移补 `v1` 改按截止记录**：0053 联查 `statement_cycle_cut`（同租户、同账户、同单号，账期是 `CYCLE/<截止日>`）补 `v1`，不再按单号形状。`TestTheDocumentNumberMigrationMarksOnlyCycleCutStatements` 从空库按序施加到 0052、落三张旧单（截止记录上的一张、单号写成 `STMT/<账户>/<日>` 而不在截止记录上的手工单一张、普通手工单一张）与旧争议、旧应付，再施加 0053：只有第一张补 `v1`，两类旧单据两列皆空。
4. **自动截单撞号按停在依赖上办**：交回的错误同时是 `ErrCycleCutUndecided` 与 `ErrCycleCutNumberingCollision`——整户回滚、不记截止日、下一拍重截，不另立第三种回滚；`cycleCutFailureKind` 先认撞号、日志单独一类（`stalled on a document numbering rule collision`）。SA CONTEXT 对账单那句与 UC-SA-003 `AT-SA-224` 写明；dispatch 真库用例断言两个哨兵都在。
5. **争议内部标识换前缀（协调方裁定）**：`newDisputeId` 的前缀 `DSP-` 改 `DISPUTE-`，与十一份默认模板的前缀都不同；旧标识不动。
6. **非阻断**：包注释、端口注释与用例按名说三种结果、按 `docnumber.MaxSkippedNumbers` 说跳号上限，不数；删掉无人调用的 `DocumentNumbering.Valid`，`adapters/http` 与 `adapters/postgres` 重复的 `numberingWords` 收成 `DocumentNumbering.Words`（删 `adapters/http/document_number.go`）；补领域单测 `TestADocumentNumberingIsANumberOrANamedAbsence`、`TestAStatementDraftIsCheckedForPublicationBeforeItHasANumber`。
7. **统一口径核对**：答复与读口字段 `documentNumber`、`documentNumberAbsence`，原因码只有 `ISSUED_BEFORE_NUMBERING`、`NUMBERING_TIME_ZONE_NOT_CONFIGURED`，撞号 `NUMBERING_RULE_COLLISION`，与并入的 07（关务案件）、08（送站预报、交接单）一致。

修订后自验（本机 PG，并入 `e110f4137` 之后的树）：`go build ./...`、`go vet ./...` 退 0，改过的 Go 文件 `gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/architecture/... ./internal/platform/... ./cmd/... ./migrations/... ./internal/settlementaccounting/...` 50 个包 ok、0 FAIL（含 PG），新用例 `-v` 全 PASS、无 SKIP。前端 `npx tsc -b` 无输出、`pnpm test` 1066 pass / 0 fail、`npx vite build` 退 0。种子灌一次性库 `docnum05_seed2` 退 0（三类单据 0 行，种子不开立它们），用后删库。`generated-artifacts-pr.sh` 退 0、`at-coverage-counts -check` 只差总览计数（推送方重生成）。**评审探针照原形状复跑**：一次性库先用 main 的迁移助手迁到 0052，直写一张手工旧单（`SYN-STMT-MANUAL-PROBE`）、一张单号写成 `STMT/<账户>/<日>` 形状的手工旧单与三笔已确认费用，再用本分支 `seed.sh` 施加 0053 并灌种子：两张旧单 `draft_version` 都留空；本分支 parcel-api 起在 :19095，同账户同账期换内容以 `v1`、`v7` 各发一次都答 `PUBLICATION_INTENT_CONFLICT`、序列表一行没有，换账期发单答 `STMT-202611-00001`。浏览器（:5195）复核：右栏登记答「已登记争议 DSP-202611-00001」、技术详情争议标识 `DISPUTE-…`，旧单两项分别注明「上线前开立，无单据号」「开立时单据编号时区未配置，无单据号」，按争议号检索只剩那张单；设置 → 单据编号对争议与审核应付显示「结算账户的业务时区」。进程已停、库已删。

## 评审记录（2026-11-02，隔离子代理，首轮钉 `df4da3184`，复评钉 `257747cf8`）

首轮按基线零阻断，合入面一条（`status.ts` 另立无号说法册、自写撞号说法，与 main 共用词典重复）；修订轮清掉，复评两轴阻断清零。协调方裁定并经复评真库探针重跑证实：发布幂等按发布意图（账户、账期、截止版本），新意图 6 路并发只发一张、同一个号；升级前手工旧单（无截止版本）同账户同账期取保守——同内容答原单原号、异内容答 `PUBLICATION_INTENT_CONFLICT` 不落单不取号（4 路并发换内容全答冲突、序列不动），旧单作废后的替代单与新账期不被误挡；迁移 0053 补 `v1` 改为联查 `statement_cycle_cut`，手工旧单号写成 `STMT/<账户>/<日>` 形状不被误补；自动截单撞号归 ADR-0339 决定三第 4 条「停在依赖上」一类（整户回滚、不记截止日、下一拍重截、日志单独一类），未另立第三种回滚；`STATEMENT_CONFLICT` 改名全仓无残留；手工发布口带 `number` 答 400；争议内部标识前缀改 `DISPUTE-`；设置页对争议与应付说「结算账户的业务时区」。余下非阻断：`pages/settlement/api.ts` 一句过期注释（推送方 `7e6f58334` 改指共用词典）；合并提交 `58cc313cc` 单独检出时 `dispute-forms.test.ts` 会红、到 `880233730` 恢复——以合并提交进 main，main 的第一父链逐笔绿，记此。

## 进 main 记录（2026-11-02，推送方 tops-20 / idp-parcel-91）

- 分支 `claude-docnum05` tip `257747cf8` 在隔离树上以合并提交并入 main `f9115d6f6`（无冲突），叠一句注释修正；重算 AT 盘点总览计数、重生成机制清点各成一笔，两项 `-check` 退 0；对 `12a5d0957` 跑全量 174 ok / 0 FAIL（含 PG）；种子一次性库 `seed_docnum05` 退 0 后删库。本笔票面簿记只动 `.md`，叠在验过的树上推确切 SHA。
- API 可见改动（已写进 UC-SA-003 与 ADR-0355 决定九）：手工截单发布口载荷不再收单号；发布意图冲突的答复码由 `STATEMENT_CONFLICT` 改为 `PUBLICATION_INTENT_CONFLICT`。

