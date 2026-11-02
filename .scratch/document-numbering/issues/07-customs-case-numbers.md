# 07 关务案件的单据号

Category: enhancement
Status: in-progress——2026-11-02 作者完工、自验、推分支 `claude-docnum07`（完成记录见文末）；待非作者评审与进 main
父票：[spec](../spec.md)
Blocked by: [02](./02-platform-numbering-adoption-and-rule-register.md)
出处：spec 子票表；[ADR-0355](../../../docs/adr/0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md) 决定二；UC-CC-001

## 本票的特殊处

- 建立案件时取号（随机案件标识照旧是主键）；重新打开不换号；关联后续案件取新号。监管编号等由监管方分配的号是外部标识，不被取代。
- 管理台：清关单页（案件、放行记录里引到案件处）照「每类单据都要做到」第 7 条。

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

## 完成记录（2026-11-02，作者，分支 `claude-docnum07`，从 origin/main `3a408b7b0` 起）

三笔提交：`295e29320`（后端：领域、端口、编排、适配器、迁移、装配与测试）、`cf89ce7b7`（管理台清关单页）、本记录与文档一笔。

### 做了什么

1. **取号点与时区**：建案编排 `EstablishCaseHandler.Handle` 在领域建立之后、落库之前经消费侧端口 `ports.CaseDocumentNumbers` 取号。适配器 `internal/customscompliance/adapters/docnumbering.CaseNumbers` 接 02 的 `docnumber.Assigner`：类型用 `domain.CustomsCaseDocumentNumberType.Code`（`CUSTOMS_CASE`），不交开立方业务时区（02 已声明「开立方没有业务时区」），判撞由案件库 `CustomsCases.CaseNumberUsed` 在同一事务里查（租户，单据号）。parcel-api 的 `buildCustomsDecisionFaces` 从 `endpointSources.documentNumbers` 取装配好的那一套，不另装；建案本来就在 `customsDecisionTransaction` 事务壳里，不必补壳。CONTEXT「关务案件」那句规则补写了日期按租户单据编号时区、未登时不带号。
2. **只在落单那一步取号**：不适用、未受理、包裹未接受、范围冲突、重放都不取号；重放读回原号；关联后续案件换了范围键走新建一支取新号；重开不碰案件行（关闭与重开在 `case_closure`），号不变。
3. **落库**：迁移 `migrations/customs_compliance/0032_customs_case_document_number.sql` 给 `customs_case` 加可空的 `document_number` 与 `document_number_absence`（只收 `NUMBERING_TIME_ZONE_NOT_CONFIGURED`，与号互斥），（租户，单据号）唯一约束，号的形状 CHECK；旧行不回填，两列皆空读成「上线前开立」。领域 `CustomsCase` 加 `Numbered` / `Unnumbered` / `DocumentNumber` / `DocumentNumberAbsence` / `NumberingSettled`，开立后不换号；案件库 `Save` 拒收单据号未落定的案件（也拒「上线前开立」——那只是旧行的读法）。
4. **撞号与回滚**：结果代数加 `CaseNumberingRuleCollision`（`NUMBERING_RULE_COLLISION`），未决原因加 `CASE_NUMBERING_UNAVAILABLE`（取号报错）。`EstablishCaseResult.RollsBack()` 对未决、撞号、「取过号才发现同一范围被并发建案抢先」三格答真，建案口事务壳据此回滚，取过的流水释放；壳的判据字段由 `undecided` 改名 `rollBack`（同壳的案件关闭、内部限制、放行门禁核对三处随改名，语义不变）。
5. **答复与读口**：`POST /customs-cases` 答复带 `documentNumber` 或 `documentNumberAbsence`（只随案件在场、二者至多其一），撞号另带 `detail` 人话指到「设置 → 单据编号」；`GET /customs-case-records`（列表与 `?case=` 单份读回）带 `documentNumber` 或 `documentNumberAbsence`（`ISSUED_BEFORE_NUMBERING` / `NUMBERING_TIME_ZONE_NOT_CONFIGURED`）；没号不答空串、不拿 `case` 顶。
6. **管理台**：清关单页「清关单」签首列「清关单号」（有号显号，无号显案件标识），技术详情列「内部标识」并按情形注明「上线前开立，无单据号」或「开立时单据编号时区未配置，无单据号」；「报关批次」签技术详情的清关单号、「关闭核对」签的清关单号列同显单据号（关闭核对另读清关单那一页取号，查不到的照旧显标识，内部标识进技术详情）；搜索框覆盖单据号。用语词典 `status.ts` 新开 `document-number-absence` 一册（两条，各类单据可共用）。
7. **文档**：UC-CC-001 新立 `AT-CC-459`–`AT-CC-462`，「给开发的交接」那一行改「已补」并写清落处；AT 盘点①节加四行与一条追记，总览计数未改。

### 关键取舍与理由

- **取号放在领域建立之后**：领域门拒收答未受理而事务照常提交，先取号就是空号（决定三）。
- **并发输家也回滚**：同一范围两笔并发建案，后到的一笔会在序列行锁上等先到的一笔提交，再取到下一个值、插入时撞范围键答「已有」。不回滚这个值就成了空号；所以「取过号却没落案件」与撞号同归 `RollsBack`。
- **案件库拒收没落定的案件**：否则两列皆空的行读回就成了「上线前开立」，冒充一种它不是的情形；为此给几处测试夹具补了号（`cmd/parcel-dispatch` 两个投影用例、本包案件库夹具）。
- **判撞读口要求事务**：事务外答出的「未用」挡不住同一时刻的另一笔建案；架构门禁 `TestEveryPersistenceWriteMethodCarriesTransactionRequiredEvidence` 据此要了负向证据，已补。
- **「放行记录」签不引案件**：动作准入判断册的对象是包裹或申报单元，依据也不带案件标识，没有可换号的地方；清关单页里引到案件的只有清关单、报关批次与关闭核对三处，都改了。
- **关闭核对的号走前端映射**：不为一列显示去改关闭目录读口的连接；代价是清关单读口那一页之外的案件显标识（与「没号显标识」同一退路）。

### 验证（2026-11-02，本机 PG `127.0.0.1:55432`）

- `go build ./...`、`go vet ./...` 退 0；改过的 `.go` 跑 `gofmt -l` 无输出。
- `go test -count=1 -p 1 ./internal/customscompliance/... ./internal/visibilityexception/... ./internal/transportfulfillment/... ./internal/architecture/... ./internal/platform/... ./cmd/... ./migrations/... ./tests/...`：首跑只有架构门禁缺 `CaseNumberUsed` 的无事务负向证据，补后 `./internal/architecture/... ./internal/customscompliance/...` 复跑全绿，其余包首跑即全绿（含 PG）。`-v` 抽查新增真库用例全 PASS、无 SKIP：`adapters/docnumbering` 七条（重放答原号、并发不撞不留空号、租户隔离、上海午夜重置、回滚释放、撞号回滚释放、时区未配置不带号且补登不补）、`adapters/postgres` 新增四条与改动一条、parcel-api `TestACaseIsNumberedInItsEstablishingTransactionThroughTheProductionFace`（生产装配：无号答情形、取号、重放原号、撞号人话不落单且回滚释放、同范围四路并发只落一个且号段连续）。
- 前端：`npx tsc -b` 无输出，`pnpm test` 1041 过 0 败，`npx vite build` 成功。
- 种子：一次性库 `docnum07_seed` 上 `seed.sh` 退 0；种子不建任何关务案件（`customs_case` 0 行），这一条无可查。浏览器核验时在同库经 `demo-run/up.sh`（`IDP_PARCEL_DEMO_API_ADDR=:19097`）起本分支的 parcel-api 与 dispatch，走提交 → 受理 → 收寄 → `declare-one-parcel.sh`：建案答 `CASE-NFKFBN2O37GSOGE3V3BPKV6YHI`、库里 `document_number = CC-20261102-0001`、平台序列 `SYN-TENANT-01/CUSTOMS_CASE/20261102 = 1`；重放答 `EXISTING` 带同号；读口带号。
- 浏览器（前端 5197 代理到 19097，配置员经 Dex 登录）：另在库里直接摆了一行时区未配置、一行上线前的合成案件看无号两态。`innerText` 核：清关单首列 `CC-20261102-0001`，技术详情「内部标识 CASE-NFK…」；两行无号的首列显案件标识，技术详情分别「单据号 上线前开立，无单据号」「单据号 开立时单据编号时区未配置，无单据号」；报关批次技术详情「清关单号 CC-20261102-0001」；关闭核对首列 `CC-20261102-0001`、种子那份引用不存在案件的目录照旧显 `SYN-CASE-CN-SG-01`；搜 `CC-20261102` 两签都只剩那一行。验完停了 vite、parcel-api、dispatch，删了 `docnum07_seed`。

### 留给协调方

- **AT 号**：用了 `AT-CC-459`–`462`（取证时 CC 最高 458，派单没给号段）；与别的分支撞号由协调方改。
- **总览计数与机制清点**：AT 盘点总览未改（本地试跑生成器只动三格：①、合计、CC 各 +4）；新增适配器包与端口会让机制清点过期，由推送方重生成。
- **共享面**：`apps/admin-web/src/vocabulary/status.ts` 末尾新开 `document-number-absence` 一册，04–09 若各自开了同名册，合并时取一份；`cmd/parcel-api` 只动了关务装配与 `endpoints_customs_decisions.go` 里取 `sources.documentNumbers` 的那一句，没动端点表行。
- 规格表（`spec.md` 子票表）里本票的状态未改。


## 评审修订（2026-11-02，作者，应协调方转来的评审意见，评审钉 `a47ebbdfd`）

1. **并入 main**（`620847516`，含票 03 的设置页与单据编号词典）：`status.ts` 冲突两册都保留后，本票那一册随下一条删去。
2. **统一用词典**：删去 `status.ts` 里本票开的 `document-number-absence` 册；清关单页的无号说法改取 `vocabulary/document-numbering.ts` 的 `documentNumberAbsenceNote`，「清关单号」与「清关单」取 `documentTypeWords.CUSTOMS_CASE` 的 `numberName` / `name`（列头、技术详情标签、搜索提示）。管理台没有建案表单，撞号说法不在页上出现；建案口答复的 `detail` 改照 `numberingCollisionNote` 给清关单的那一句写。
3. **事务壳注释**（Standards）：`customsDecisionTransaction` 的注释只指 `EstablishCaseResult.RollsBack`，不再数、不再列它的回滚情形。
4. **UC-CC-001**（Spec）：「结果语义契约」表加「编号规则撞号」一行，「已建立」「已有案件」两行补单据号或无号情形；「首发结果码」表加 `NUMBERING_RULE_COLLISION`，未形成案件那一行的未决原因补 `CASE_NUMBERING_UNAVAILABLE`。
5. **非阻断**：「关闭核对」签的视图状态只由关闭目录读口定，清关单读口没答只让号那一格降级——显案件标识，技术详情注明读不到（`record-rows.ts` 的 `caseNumberReader`，有用例）；关闭核对与「报关批次」两签的技术详情都列清关单内部标识，无号的另按情形注明。
6. **口径核对**：`documentNumber`、`documentNumberAbsence`（`ISSUED_BEFORE_NUMBERING` / `NUMBERING_TIME_ZONE_NOT_CONFIGURED`）、`NUMBERING_RULE_COLLISION` 与全批定的一致，未改。

修订后自验见协调方汇报；这一轮没有重灌种子、没有重做浏览器核验（行为改动只在关闭核对签的降级与技术详情，由前端用例钉住）。
