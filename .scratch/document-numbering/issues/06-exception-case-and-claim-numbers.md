# 06 异常案件、客户索赔项的单据号

Category: enhancement
Status: in-progress
父票：[spec](../spec.md)
Blocked by: [02](./02-platform-numbering-adoption-and-rule-register.md)
出处：spec 子票表；[ADR-0355](../../../docs/adr/0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md) 决定二；UC-VE-004、UC-VE-007

## 本票的特殊处

- **异常案件**：建立案件时取号（分诊自动立案与人工立案同一处）；重开不换号；拆出的关联新案件取新号；归并不改两边各自的号。
- **客户索赔项**：客户提交被记录时取号（早于资格审核）；客户自己的索赔项标识与批次号是外部标识，不被取代。客户渠道的索赔答复带上索赔单据号（新增字段）。
- 管理台：异常处理、异常案件、索赔与追偿各页照「每类单据都要做到」第 7 条；首页「今日待办」异常格点进去的列表同样显示单据号。

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

## 完成记录（2026-11-02，作者，分支 `claude-docnum06`，自 main `3a408b7b0` 起）

提交：`654652213`（后端：领域、端口、适配器、迁移、读口、装配与测试）、`3afa5193d`（管理台）、`9696f51cf`（CONTEXT、UC 交接、AT、首方契约、Covers 点名），本记录一笔。

### 做了什么

- **开立点与时区**：异常案件在分诊自动建案那一步（`RaiseSignalHandler.Handle`，走向已定、标识已签、写库之前）取号；客户索赔项在受理编排里（批次、项）未在案、要落单的那一步取号（`HandleClaimHandler.ReceiveClaim`，早于资格审核）。两类单据的开立方都没有业务时区，按租户的单据编号时区（类型声明 02 已写 `DateTimeZoneFromTenant`，不另传 `IssuerTimeZone`），这句写进了 VE CONTEXT 两条规则。人工立案今天没有入口，日后也只经 `EstablishCase` 一个门；`EstablishCase` 与 `ReceiveClaimItem` 都要求开立这一刻形成的单据号那一格（号，或时区未配置），零值只留给重建的旧行。
- **消费侧端口与适配器**：`ports.CaseNumberIssuer`、`ports.ClaimNumberIssuer`；`vepostgres.DocumentNumbers` 接 `docnumbering.Components.Assigner`，「是否已用」查本上下文自己的表。parcel-api 的 `buildClaimsOrchestration`、parcel-dispatch 的 `newTenantBoundProjectionDerive`（冲突信号与出发门禁阻断信号的自动建案都经它）都接同一个 DB 上装的那一套。
- **落库**：VE 迁移 `0033`，两表各加可空的 `document_number`、`numberless_reason`（只认 `NUMBERING_TIME_ZONE_NOT_CONFIGURED`，不与号同在），（租户，单据号）唯一；旧行两格都空即上线前开立，不回填。号随单同一条 INSERT 落下，不进索赔的更新集。
- **答复与读口**：案件册、索赔册带 `documentNumber`，没号时不带它、带 `numberlessReason`（`ISSUED_BEFORE_NUMBERING` / `NUMBERING_TIME_ZONE_NOT_CONFIGURED`）；处置请求册、追偿册带所属案件的 `caseDocumentNumber`（没号缺席）。首方索赔回执新增 `documentNumber` / `numberlessReason`，撞号答 `200 NUMBERING_RULE_COLLISION`、不带回执，`docs/api/first-party-api-contract.md` 已写。
- **管理台**：异常案件页（首页「今日待办」异常格点进来的那一页）案件号为主列、可检索，案件内部标识进技术详情；异常处理页的处置请求签与索赔与追偿页的追偿签以案件号指认案件；客户索赔签索赔号为主列、可检索，客户自己的索赔项编号与批次号留在技术详情。没号的行主列照旧显标识，技术详情按读口给的原因说「上线前开立，无单据号」或「开立时单据编号时区未配置，无单据号」（`document-number.ts` 一处）。用语词典追加「案件号」「索赔号」。
- **文档**：VE CONTEXT 两条单据号规则补时区、无号与撞号；UC-VE-004、UC-VE-007 交接改「已补」并写落点；新立 `AT-VE-227`、`228`（案件）、`AT-VE-229`、`230`（索赔），AT 盘点加行与追记。

### 关键取舍与理由（请协调方裁）

1. **撞号退号放在取号这一步的保存点里，而不是一律回滚整笔开立事务。** 02 交接写「收到撞号必须回滚开立事务」。案件的开立事务是 dispatch 的投影派生：冲突信号那一路分诊答未决时派生照常提交（`ConflictSignalDeferred`，「信号那一步失败不推翻派生」），为一件案件撞号让整封回滚会改掉这条既有语义，而照常提交又会把取过的 20 个流水留成空号。`vepostgres.DocumentNumbers` 因此在 `SAVEPOINT` 里调 `Assign`，撞号或出错 `ROLLBACK TO SAVEPOINT` 把取过的值退回，端口契约写明「答撞号时取过的已退回，调用方只需不落单」。效果与回滚整笔相同：单不落、号不空。真库用例钉住「撞号后同一事务里别的写入照常提交、腾出旧号后下一次拿到的正是第一个流水」。
2. **索赔「取了号却没落单」的分支要求回滚。** 同一（批次、项）并发受理时，输家在取号之后才在写库撞上赢家（取号在序列行锁上排队，排到时赢家已提交），答原项照旧，但它取过的号若随事务提交就成空号。`HandleClaimResult.DiscardIssuing()` 在这一支与写库失败那一支为真，`transactionalClaims.ReceiveClaim` 见它即回滚、答案照交。变异检验：去掉这一步，并发用例下一项拿到 `00003`。
3. **案件撞号的答案是未决 `CASE_NUMBERING_RULE_COLLISION`**（`RaiseSignalUndecidedReason`），发作期、结论与案件整轮不落——结论「自动建案」与案件必须同一提交。出发门禁那一路随之整封记未决待重投，冲突信号那一路记搁置；dispatch 没有给人看的答复面，「说人话、指到规则册」只落在索赔答复的 outcome 与契约文档上。若要在运维面可见，需另起一票给未决原因一个出口。
4. **JSON 字段名与无号原因取值**：`documentNumber`、`numberlessReason`（`ISSUED_BEFORE_NUMBERING` / `NUMBERING_TIME_ZONE_NOT_CONFIGURED`）、引用别的单据时 `caseDocumentNumber`。04–09 若取了别的名字，请协调方择一统一。
5. **索赔没有内部随机标识**：没号的索赔主列显客户自己的索赔项编号（（批次、项）就是它在册上的键）。
6. **重开、拆出的命令面未建**：「重开不换号、拆出取新号、归并不改两边的号」由领域上没有改号的方法、新案件只能经 `EstablishCase` 取号担保，领域用例钉了接单、关闭、归并不换号。
7. **追偿册只带关联案件的号，不带案件的无号原因**：没号时追偿签悬停只说「无单据号」，原因到异常案件页看。

### 验证（本机 PG `127.0.0.1:55432`）

- `go build ./...`、`go vet ./...` 退 0；改过的 `.go` 跑 `gofmt -l` 无输出。
- `go test -count=1 -p 1 ./internal/visibilityexception/... ./internal/architecture/... ./internal/platform/... ./cmd/... ./migrations/...` 全绿（含 PG，实测于 `9696f51cf`）；`-v` 抽查新真库用例（`vepostgres` 的 `document_numbers_test.go`、parcel-dispatch 的 `exception_case_number_test.go`、parcel-api 的 `assemble_claims_document_number_test.go` 与改过的索赔口用例）全 PASS、无 SKIP。覆盖：重放答原号、并发不撞号（不同项各一个号连成一段；同一项只落一次、输家退号）、租户隔离、周期重置（案件按日、索赔按月，按上海时区跨日）、开立回滚释放号码、撞号答案与退号、事务外拒、唯一约束与原因取值域、`0033` 在有数据的库上施加不回填。
- 前端：`npx tsc -b`、`pnpm test`（1042 pass）、`npx vite build` 退 0。
- 种子：一次性库 `docnum06_seed` 灌 `scripts/demo-seeds/seed.sh` 退 0；**种子不开立异常案件与客户索赔项**（两表零行，单据编号时区一版在册），所以「种子开立的这几类都带号」无可查之行。用后删库。
- 浏览器：自编 parcel-api 起在 `19096` 连 `docnum06_seed`，前端 `5196`。为了有行可看，经真编排（`RaiseSignalHandler` + `HandleClaimHandler` + 真取号）在库里立了两件自动建案（`EX-20261102-0001/0002`）与两项索赔（`CLM-202611-00001/00002`），另用 SQL 摆了上线前开立与时区未配置的案件、索赔各一行和引用它们的处置请求、追偿事项。`innerText` 核过：首页「今日待办」异常格点进异常案件页，主列是案件号、旧行显内部标识；按案件号检索命中一行；详情技术详情分别写「上线前开立，无单据号」「开立时单据编号时区未配置，无单据号」；处置请求签与追偿签以案件号指认、可按案件号检索；客户索赔签索赔号为主列、可检索，技术详情列客户的索赔项编号与批次。过程中无写请求。进程已停，库已删。

### 留给协调方的事

- 上面取舍 1、3、4 待裁。
- AT 盘点总览计数（`tools/at-coverage-counts -check` 只报总览过期，表行与 UC 立号对得上）与机制清点由推送方在 main 的 tip 上重生成。
- 共享面：`cmd/parcel-dispatch/assemble.go` 的 `wireDispatcher` 里把 `buildDocumentNumbers` 挪到 `newTenantBoundProjectionDerive` 之前（后者要用它）；`newTenantBoundProjectionDerive` 多了一个参数，两个既有测试随之改了调用。`apps/admin-web/src/vocabulary/objects.ts` 只在「异常与索赔」段末追加两词。
- `EstablishCase` 与 `ReceiveClaimItem` 的签名或入参变了：别的票若在 VE 里新建案件或索赔，要交单据号那一格。

## 评审修订（2026-11-02，作者，应协调方转来的评审意见，评审钉 `3b63bd7c8`、两轴阻断为零）

协调方接受保存点退号的偏离。以下各条追加提交，未 rebase：

1. **并入 main**：`git merge origin/main`（`620847516`，含票 03 与单据编号词典）。
2. **平台注释**：`docnumber.NumberingRuleCollision`、`Assigner.Assign`、`docnumbering.Components.Assigner` 改成「必须让取过的流水不提交：回滚开立事务，或在取号那一步包保存点只退号」。
3. **统一口径**：读口与首方回执的 `numberlessReason` 改名 `documentNumberAbsence`；迁移 0033 的列 `numberless_reason` 改名 `document_number_absence`；Go 侧随之改名（`domain.DocumentNumberAbsence`、`DocumentNumber.Absence()` / `StoredAbsence()`、读行的 `DocumentNumberAbsence`）；首方契约、`case-api.ts` 同改。
4. **共用词典**：删掉 `numberlessNote` 与 `objectNouns.exceptionCaseNumber` / `claimNumber`，号名取 `documentTypeWords.EXCEPTION_CASE` / `CLAIM` 的 `numberName`，无号说法取 `documentNumberAbsenceNote`。三页的「有号 / 无号」显示收成 `pages/visibility/DocumentNumberCell.tsx` 一个组件（分支逻辑在 `document-number.ts::primaryReference`，node:test 钉着）。这几页都不开立单据、没有开立答复可显示，`numberingCollisionNote` 在本票没有调用点。
5. **案件撞号的可见与恢复**（取舍见下）：
   - 撞号从未决原因改成业务答案 `CaseNumberingCollided`（`CASE_NUMBERING_RULE_COLLISION`）。编排把那一次命中记进案件撞号册（迁移 0033 的 `case_numbering_collision`，键是租户、对象、类型、因果条件，存够重建一次命中的各样与撞上的格式）。
   - 出发门禁那一路答单独一格 `DepartureGateSignalCaseNumberingCollided`，带撞上的格式；冲突信号那一路记成单独的去向 `ConflictSignalCaseNumberingCollided`。两路的信封都照常入账、投影照落。
   - 恢复由 parcel-dispatch 的 `caseNumberingRedriveBeat` 做：每拍在派发之后取撞号册上到期的命中，用同一只信号编排原样再分诊。仍撞就推后 `CaseNumberingRedriveInterval`（五分钟，产品策略，理由写在常量注释上），走通即建案并销账；这一原因的发作期一落，`SaveRaised` 同一笔销掉那一行。仍撞的那几件作为本拍错误交给循环记成 `dispatch beat failed`，错误正文以 `exception case numbering rule collision` 起头，带租户、包裹、信号类型与撞上的格式，单列一类（`ErrCaseNumberingRuleCollision`）。
   - 索赔侧 `ClaimNumberingRuleCollision` 与案件侧都写明「不是未决」，以及恢复路为何不同：索赔有提交方，答案回给客户，客户原样重发即可；案件的命中没有提交方，所以记进撞号册、由重驱再分诊。
6. **AT-VE-228 撞号那一半的生产路由用例**：`cmd/parcel-dispatch/exception_case_number_test.go::TestACaseNumberCollisionSettlesTheEnvelopeAndTheRedriveEstablishesTheCaseAfterTheFormatChanges`。租户登一份会与旧号重复的格式，经生产路由：信封 `PUBLISHED`、失败零次，投影在，案件与发作期都不落，撞号册一行；同一拍报 `ErrCaseNumberingRuleCollision`，推后期间下一拍不出声。换格式、到了重试时刻后的下一拍建案，号是新格式的首号，撞号册清空。撞号册另有真库用例：读回、覆盖、推后、销账、`SaveRaised` 销账、事务外拒。重驱与两路去向有编排用例。

### 恢复做法的取舍（两种都没选，请协调方裁）

协调方给的两种做法在派发框架上都做不成：

- **现有机制把耗尽预算的那条再处理**：框架没有这个口。认领时就 `attempt + 1`，`attempt >= MaxAttempts` 当场落 `ABANDONED` 并推进分区，`Store` 只有 `Claim`、`MarkPublished`、`RecordFailure`、`Abandon`，没有把 `ABANDONED` 重新入队的口。本仓文档也没有手工救回的操作，各 ADR 都把 `ABANDONED` 当终态。
- **撞号不计入 `MaxAttempts`、原样重投**：预算是在认领时扣的，不是失败时扣的；不改框架就没法让一条信封不耗预算地重投。即便能，也会让出发信封所在的分区（同一对象的后续信封）一直堵到租户换格式为止，正是 ADR-0235 记过的那种病。

所以按本仓既有的「重投改变不了的就入账，另给续办」那一路做（ADR-0094 决定四、ADR-0106、ADR-0235）：撞号是业务答案，信封入账；命中进撞号册；派发进程每拍定时重驱，形状照 `cycleCutBeat`（按截止日截单）。实际效果对得上协调方要的「不烧失败预算」「换格式后能重新驱动」，而且不用任何人工操作。没有做「规则册登记新版时发信封触发重驱」：那要平台编号组件在登记事务里交 outbox 信封、派发再登一条路，跨票 02 的地盘；五分钟一次的到期重试已能兜住换格式、换周期、旧号让出三种情形。若要改成登记即触发，另起一票。

### 修订后自验（本机 PG）

- `go build ./...`、`go vet ./...` 退 0，`gofmt -l` 无输出。
- 受影响包加 architecture、platform、`cmd/...`、migrations 的 `go test -count=1 -p 1` 全绿，含 PG；新用例无 SKIP。
- 评审探针 `zz_review_probe_test.go` 放进 `vepostgres` 跑 `-run TestReviewProbe` 全过，跑完删除。
- 前端 `npx tsc -b`、`pnpm test`（1063 通过）、`npx vite build` 都过。
- `tools/at-coverage-counts -check` 只报总览过期，表行与 UC 立号对得上；总览计数与机制清点留给推送方重生成。

## 复评修订（2026-11-02，作者，应协调方转来的复评意见，复评钉 `c8c7a8ceb`，R2 并发重复立案按阻断级修）

先并入 origin/main `f9d2b059c`（AT 盘点追记冲突取并集），再追加提交：

1. **并发重驱去重**：
   - 重驱先用 `ClaimDueCaseNumberingCollision` 抢册上那一行（`FOR UPDATE SKIP LOCKED`，且仍到期）。抢到的是此刻库里那一行，不是取页时那一份；抢不到答 `CaseNumberingRedriveSkipped`，不等也不分诊。
   - 分诊编排每一轮在找发作期之前，先用 `HoldCaseNumberingCollision`（`FOR UPDATE`，会等）锁住这一原因在撞号册上的那一行（若有）。重驱与同一原因的正常命中交叠时，后到的等先到的提交，再读发作期就看见它，只记一次命中；先到的在 `SaveRaised` 同笔销账。
   - 不靠唯一约束兜：发作期表上没有按因果条件的唯一约束，加一条会牵动既有的重开语义，超出本票。
   - 真库用例 `case_numbering_redrive_test.go`：两笔重驱交叠、正常命中先到、重驱先到，三种都只落一个发作期、一件案件，撞号册清空。
   - 变异检验：去掉锁行，「重驱先到」那一格答 `EPISODE_OPENED`、重复立案；去掉 `SKIP LOCKED`，后到的重驱等在锁上（用例给了期限）。两种都会让用例失败。
2. **原样 = 首次命中**：同一原因再撞，`RecordCaseNumberingCollision` 只加撞号次数、换成最近撞上的格式、立刻到期，首次命中的规则版本、可信度、事实依据、判断时间都不动。重驱建出的发作期按首次命中开期、命中一次。用例照探针 R3 的形状。
3. **页首饿死**：重驱停在依赖上或出错时，那一笔事务回滚，派发再另开一笔 `Defer`，推后同一个 `CaseNumberingRedriveInterval`。取舍：撞号命中已经等过了，晚一个间隔建案不改变什么；不推后，读不动的几件到期时刻不变、每拍排在页首，页一满后面能建成的就永远轮不到。没有改成按到期时刻轮转：推后本身就让它们退到队尾，效果相同，而且只需一处。
4. **非阻断各条**：
   - `NumberlessForUnconfiguredTimeZone` 改名 `DocumentNumberAbsentForUnconfiguredTimeZone`。
   - `document-number.ts` 头注去掉计数。
   - 删掉没有生产读者的 `CollidedTemplate()`（编排结果与出发门禁结果两处）。撞上的格式只记在撞号册上，重驱的日志会带出来。
   - 票 02「按答案办」补了一句「也可在取号那一步包保存点只退号」。
   - UC-VE-004 交接补写并发、首次命中与推后，AT 盘点 `AT-VE-228` 行加了新用例。

自验（本机 PG）：
- `go build ./...`、`go vet ./...` 通过，`gofmt -l` 无输出；受影响包全量测试全绿。
- 复评探针 R1、R2a、R2b 都是发作期 1、案件 1、撞号册 0；R2a 后到的答 `SKIPPED`；R3 的发作期从首次命中那一刻开期、命中 1 次。探针跑完已删。
- 前端 `tsc`、`pnpm test`（1079 通过）、`vite build` 都过。
