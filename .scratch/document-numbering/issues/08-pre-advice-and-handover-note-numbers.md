# 08 送站预报、送站交接单的单据号

Category: enhancement
Status: in-progress——2026-11-02 作者完工、自验、推分支 `claude-docnum08`（完成记录见文末）；待非作者评审与进 main
父票：[spec](../spec.md)
Blocked by: [02](./02-platform-numbering-adoption-and-rule-register.md)
出处：spec 子票表；[ADR-0355](../../../docs/adr/0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md) 决定二；UC-NO-006

## 本票的特殊处

- **送站预报**：进入已登记时取号，与预报标识一起答回客户（首方客户渠道答复新增字段）；同一接入请求原样重投答原号。
- **送站交接单**：服务端收下节点开立时取号；标识仍由设备签发（ADR-0023），单据号不替代它；同一开立重投答原号。离线开立期间现场只有设备标识（ADR-0355 越权风险点 6），设备侧与客户确认环节如何显示单据号，本票核实现状写进完成记录。
- 时区按开立网点的业务时区。
- 管理台：网点作业页的送站预报、送站交接单照「每类单据都要做到」第 7 条。

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

## 完成记录（2026-11-02，作者，分支 `claude-docnum08`，从 origin/main `3a408b7b0` 起）

四笔提交：`b66d69c15`（network-routing 窄读口）、`67727fdeb`（node-operations 取号、落库、答复与读口、parcel-api 装配）、`edf12e7c9`（管理台）、`7aad4b4a0`（CONTEXT、CONTEXT-MAP、UC-NO-006、首方契约说明、AT 盘点），加本记录一笔。

### 做了什么

- **取号点**：送站预报在 `DeliveryPreAdviceHandler.Submit`、送站交接单在 `DeliveryHandoverNoteHandler.Open`，都在 parcel-api 装配点包的那一笔事务里。次序是：形成单据（拒形在前）→ 册上锁住重放身份并读回（`LockDeliveryPreAdviceSource` 按（租户，客户端，来源请求键），`LockDeliveryHandoverNoteOpening` 按（租户，交接单标识），都是事务级咨询锁）→ 在册即按原逻辑答已有结果 / 来源冲突，原号随在册那一份回显、一个号也不取 → 没有才取号 → 号随单 INSERT。经本上下文的消费侧端口 `ports.DocumentNumberIssuer`（适配器 `adapters/documentnumbers` 包 `docnumber.Assigner`），类型取 `domain.DeliveryPreAdviceDocumentNumberType` / `DeliveryHandoverNoteDocumentNumberType`，判撞问册上的 `…DocumentNumberUsed`。
- **业务时区**：两类都取开立网点那一刻（编排时钟，即服务端时刻）在网络目录里生效的节点版本的 `business_timezone`，经新端口 `ports.IssuingNodeTimeZones` → `adapters/networkrouting.NodeBusinessTimeZones` → network-routing 新立的窄读口 `NodeBusinessTimezoneRead.FindNodeBusinessTimezoneAt`（`NetworkCatalog` 实现，取两行、两版同时适用答 `ErrAmbiguousNetworkCatalog`）。网点没有生效版本交空串，取号编排退到租户的单据编号时区；都没有则照常开立、记 `NUMBERING_TIME_ZONE_NOT_CONFIGURED`。离线开立的交接单按服务端收下那一刻出日期，设备声明的 `openedAt` 照旧只作业务时间。
- **落库**：迁移 `node_operations/0026`：两表各加 `document_number`、`document_number_absence`（只收 `NUMBERING_TIME_ZONE_NOT_CONFIGURED`），两格不同在，（租户，单据号）唯一；旧行两格都空即上线前开立，不回填。册的写口拒收单据号没定的新单据（否则新行读回来就成了上线前开立）。
- **撞号**：结果代数各加 `NUMBERING_RULE_COLLISION`（`DeliveryPreAdviceNumberingRuleCollision`、`DeliveryHandoverNumberingRuleCollision`），答案不带单据；parcel-api 的两只事务外壳见到这一格交给事务器一个回滚信号（`errNumberingCollisionRollsBack`），回滚后照交答案。HTTP 答 200 只带 `outcome`。
- **答复与读口**：首方预报答复的 `preAdvice`、作业事实开立答复的 `handoverNote`、两本册的列表与详情读口带 `documentNumber`；没有号时不带它，改带 `documentNumberAbsence`（`ISSUED_BEFORE_NUMBERING` / `NUMBERING_TIME_ZONE_NOT_CONFIGURED`）。两本册列表收 `documentNumber` 查询键，按（租户，单据号）逐字找。首方契约说明（`docs/api/first-party-api-contract.md`）补了预报答复这一格与撞号答复；生成器尚未落（ADR-0140 的 code/82），字段以处理器结构体为准。
- **管理台**：网点作业页「客户送货预报」「送货交接单」两区主列改为预报单号 / 交接单号（用语词典追加 `preAdviceNumber`、`handoverNoteNumber`），没有单据号的照旧显内部标识；筛选条加单据号一格（交服务端检索，不是在页内搜）；详情标题用单据号，技术详情首行是单据号或按原因分开的「上线前开立，无单据号」「开立时单据编号时区未配置，无单据号」，内部标识（交接单写「交接单标识（设备签发）」）随后。
- **文档**：CONTEXT 两句规则写明时区来路；CONTEXT-MAP 的 `network-routing → node-operations` 补一句只读节点业务时区；UC-NO-006 结果契约补单据号与「编号规则撞号」、立 `AT-NO-135`–`137`、交接那一行改「已补」；AT 盘点①追加三行与追记（总览计数未改）。`submit-one-pre-advice.sh` 的说明行带出单据号。

### 关键取舍与理由

1. **先锁重放身份、再取号，而不是先取号再「撞键即回显」或先插后改。** 先取号再落册，重放那一笔取到的流水随提交成了空号（决定三只许跳号成空号）；先插一行空号再 UPDATE 会破两本册「只增、没有 UPDATE」的约定（迁移 0013 头注）。咨询锁是本仓既有做法（settlement-accounting、transport-fulfillment 各处），同一重放身份的并发几笔排成一队，后到的读得到先到那一笔。真库用例钉住「同一接入请求并发五次只登一份、序列只走一个值」。
2. **撞号是业务答案，回滚放在进程级事务外壳。** 编排不拥有事务；外壳按 outcome 判回滚，答案原样交回。真库用例钉住撞号后序列行不存在、预报 / 交接单不落、让出一个号后同一接入请求重交拿到那个号。
3. **网点时区走 NR 新立的窄读口，不读整份目录快照。** `LoadDefinitionsAt` 一次读七族，开立一张单据只要一格时区；窄口与快照的节点族同一选版口径与歧义哨兵。选版时刻用编排时钟（服务端此刻）；取号编排自己的时钟另取一次，两者都是服务端时钟、相差毫秒，跨午夜那一瞬可能一个按前一刻选版、一个按后一刻出日期——网点版本在那一瞬换时区才会显形，记此不另处理。
4. **册拒收单据号没定的新单据。** 测试夹具因此都要带上一格（多数带「单据编号时区未配置」）；换来的是新行不可能落成「上线前开立」的样子。
5. **无号原因上线。** 决定七要「答复与页面如实写单据编号时区未配置」，所以答复带 `documentNumberAbsence` 而不是只缺席 `documentNumber`；上线前开立只从两格都空读出来，开立路径写不出它。

### 设备侧与客户确认环节的现状（ADR-0355 越权风险点 6，核实结果）

- 本仓没有一线作业端应用（`apps/` 下只有 `admin-web`），`cmd/parcel-frontline-import` 不开立交接单。设备离线时只有它自己签的交接单标识；服务端收下开立后，开立口答复的 `handoverNote.documentNumber` 才有号，重投读回原号。作业端如何把号显示给现场，待作业端立项时定。
- 客户确认口（`POST /v1/node-operations/delivery-handover-conclusions`）载荷与答复都只按设备签发的 `noteId`，答复只回显交接单标识与本账户的结论，不带单据号；首方族也没有读交接单的口。客户在确认环节看不到交接单号——本票未改它（UC-NO-006「客户面的回显只给 noteId」的取舍不在本票），是否给客户答复加 `documentNumber` 留待裁。

### 验证（本机 PG `127.0.0.1:55432`）

- `go build ./...`、`go vet ./...` 退 0；改过的 `.go` 跑 `gofmt -l` 无输出；`go mod tidy -diff` 无输出。
- `go test -count=1 -p 1 ./internal/architecture/... ./internal/platform/... ./cmd/... ./internal/nodeoperations/... ./internal/networkrouting/... ./migrations/... ./tests/...`：62 包全 ok（含 PG）。`-v` 抽查新增真库用例 `TestPreAdviceDocumentNumbersOnARealDatabase`、`TestAPreAdviceNumberingCollisionRollsBackOnARealDatabase`、`TestHandoverNoteDocumentNumbersOnARealDatabase`、`TestDeliveryScopeRegistersKeepTheDocumentNumber`、`TestNodeBusinessTimezoneIsReadFromTheVersionEffectiveAtTheInstant` 与改了装配的既有真库用例全 PASS、无 SKIP。真库覆盖：同一开立重放答原号（含并发重投）、并发开立不撞号且连成一段、租户隔离、跨日周期重置、撞号回滚释放流水、撞号答案、网点时区优先 / 退租户时区 / 都没有不带号。
- 管理台：`npx tsc -b`、`pnpm test`（1042 pass）、`npx vite build` 退 0。
- 种子：一次性库 `docnum08_seed`，`seed.sh` 退 0；**种子不开立送站预报与送站交接单**（两表 0 行），「种子开立的这几类单据都带号」对本票是空集。
- 浏览器：本分支编的 parcel-api 起在 19098（连 `docnum08_seed`），前端 5198。经首方口交四份预报，答复依次 `PA-20261102-00001`…`00004`（上海枢纽按 `Asia/Shanghai`；网络目录里没有的网点退到种子登的租户单据编号时区），同一接入请求重投答 `200 EXISTING_RESULT` 原号；经作业事实口开两张交接单，设备声明开立时刻为前一日的那张答 `HO-20261102-00001`（服务端收下那一日），重投答原号。另向一次性库直插一份两格都空的旧预报、一张记 `NUMBERING_TIME_ZONE_NOT_CONFIGURED` 的交接单与一张旧交接单，只为看页面。`innerText` 核：两区表头首列「预报单号」「交接单号」，有号行显号、旧行显内部标识；单据号一格检索 `PA-20261102-00002` / `HO-20261102-00001` 各剩一行、请求带 `documentNumber=`；技术详情依次为「预报单号 PA-20261102-00002 / 预报标识 PREADV-…」「预报单号 上线前开立，无单据号」「交接单号 HO-20261102-00001 / 交接单标识（设备签发） SYN-HANDOVER-DN08-01」「交接单号 开立时单据编号时区未配置，无单据号」。验完已停进程、删库。

### 留给协调方

- 推送方在 main tip 上重算 AT 盘点总览（本票加了三行①）、重生成机制清点（新端口与适配器：`ports.DocumentNumberIssuer`、`ports.IssuingNodeTimeZones`、NR `NodeBusinessTimezoneRead`、`adapters/documentnumbers`）。
- **跨票口径待统一**：答复字段名 `documentNumber` / `documentNumberAbsence` 与原因码 `ISSUED_BEFORE_NUMBERING` / `NUMBERING_TIME_ZONE_NOT_CONFIGURED`、撞号 outcome `NUMBERING_RULE_COLLISION`，04–09 各票若另起名字，合并时宜取一套。管理台的无号原因说法在 `pages/operations/delivery-pre-advice.ts` 的 `documentNumberDetailOf`，若别票也写一份，宜提到共享处。
- NR 的 `NodeBusinessTimezoneRead` 是本票新立的；票 04 若核定委托用发件网点业务时区，可直接复用，不必另立。
- 待裁：客户确认口答复是否带交接单号（见上「现状」）。

估计用量：约 45 万 token。

## 评审修订（2026-11-02，作者，应协调方转来的评审意见，评审钉 `d2d5b461b`、两轴阻断为零）

1. **并入 main**：`git merge origin/main`（`620847516`，含票 03 设置页与单据编号词典），无冲突。
2. **全批统一口径**：无号原因码 `ISSUED_BEFORE_DOCUMENT_NUMBERING` 改为 `ISSUED_BEFORE_NUMBERING`（领域 `IssuedBeforeNumbering` 的线上名、HTTP 注释、首方契约说明、Go 与管理台测试；迁移 0026 的 CHECK 只收 `NUMBERING_TIME_ZONE_NOT_CONFIGURED`，上线前开立不入库，原文不涉这个码，未改）。上文「完成记录」里的旧码名是当时的写法，以本节为准。字段 `documentNumber` / `documentNumberAbsence`、撞号 `NUMBERING_RULE_COLLISION` 原已一致。
3. **共用词典**：管理台无号说法改取 `vocabulary/document-numbering.ts` 的 `documentNumberAbsenceNote`，号名取 `documentTypeWords.DELIVERY_PRE_ADVICE / DELIVERY_HANDOVER_NOTE.numberName`（预报区的号名因此由「预报单号」变为词典里的「预报号」）；`delivery-pre-advice.ts` 的 `documentNumberDetailOf` 只剩「有号显号、没号取词典说法」的组装；删去 `objects.ts` 里追加的两个同义号名。
4. **客户确认答复带交接单号**（协调方裁定，ADR-0355 决定十一）：`POST /v1/node-operations/delivery-handover-conclusions` 的答复新增 `documentNumber` / `documentNumberAbsence`，其余各格不变。用例：HTTP 面 `TestTheCustomerConclusionAnswerCarriesTheHandoverNoteNumber`（有号、无号两种，仍不回显声称），编排面在 `TestOpeningAHandoverNoteIssuesItsDocumentNumberOnTheServersReceipt` 里补客户确认答的交接单带原号、不再取号。首方契约说明、UC-NO-006 交接一行与 `AT-NO-136`、AT 盘点那一行随之改。上文「设备侧与客户确认环节的现状」里客户确认那一条已由本条改掉。
5. **计数与序数**：`NodeBusinessTimezoneRead`（「要读七族」→「全部定义族」）、`routing_basis.go` 包注释、`DocumentNumberAbsence` 与其测试 Covers、`assemble_delivery_scope_document_numbers_test.go` 文件头（改引票面那一条的原文）与撞号用例里的「二十」（改指 `docnumber.MaxSkippedNumbers`）、`issuer_test.go` 与 `issuer.go`、三处 build 函数注释与 `DocumentNumberingDeps`、端口文件头，以及各处「两格都空」「两本册」一类跨文件的数，都改成按名说。

修订后自验（本机 PG）：`go build ./...`、`go vet ./...` 退 0，`gofmt -l` 无输出，`go mod tidy -diff` 无输出；`go test -count=1 -p 1` 覆盖 architecture、platform、cmd、nodeoperations、networkrouting、migrations、tests 全绿（含 PG）；管理台 `tsc -b`、`pnpm test`（1062 pass）、`vite build` 退 0；种子灌一次性库 `docnum08_seed2` 退 0（两表 0 行、0026 的约束在册），用后删库。
