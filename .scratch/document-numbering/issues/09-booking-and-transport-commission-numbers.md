# 09 订舱、运输委托的单据号

Category: enhancement
Status: in-progress
父票：[spec](../spec.md)
Blocked by: [02](./02-platform-numbering-adoption-and-rule-register.md)
出处：spec 子票表；[ADR-0355](../../../docs/adr/0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md) 决定二、十；UC-TF-004

## 本票的特殊处

- **运输委托**：进入已提交时取号；换版不换号。
- **订舱**：订舱申请登记时取号；承运方答复里的订舱号是外部标识，不取代本方单据号。承运答复口照旧按订舱标识认单（ADR-0202 决定二、ADR-0239），不改认单键；单据号随订舱交给承运方供人核对，集成说明（`docs/api/carrier-booking-answer-integration.md`）补一句它与订舱标识的关系。
- 决定十：载荷里点名的委托、订舱标识改由调用方生成做幂等键；管理台若有登记这两类的表单，不再让人填编号（今天若只有受控 CLI 或集成口，照实写进完成记录）。
- 管理台：运输班次等显示委托、订舱处照「每类单据都要做到」第 7 条。

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

## 完成记录（2026-11-02，作者，分支 `claude-docnum09`，起点 main `3a408b7b0`）

### 做了什么

- **开立点**：委托在 `CommissionTransportHandler.SubmitCommission`、订舱在 `SubmitBooking`，两处都在 `Save` 答已保存之后、同一笔开立事务里经 TF 自己的消费侧端口 `ports.DocumentNumberIssuer`（适配器 `internal/transportfulfillment/adapters/docnumber`，接 `docnumbering.Components.Assigner`）取号，再经 `ports.TransportCommissionNumbers` / `ports.BookingNumbers` 把号或无号原因记到刚落的那一行上。parcel-api 的 `buildTransportCommissionSubmission`、`buildBookingFaces` 从 `endpointSources.documentNumbers` 取那一套编号组件；两只事务壳（`transactionalTransportCommissionSubmitter`、`transactionalBookingFaces`）对 `NUMBERING_RULE_COLLISION` 与`未决`一样整笔回滚。取消、答复、段采用三只编排实例不装取号。
- **业务时区**：核过，没有可用的开立方业务时区——委托与订舱都由租户的运输运营提出，开立方不是网点、也不是带对账安排的结算账户；订舱指名的班次若出自周期安排，那份时区说的是时刻表怎么读，临时班次连这一格都没有。两类照 02 的声明按租户单据编号时区，声明处与 TF CONTEXT「运输委托、订舱与承运接受」那句规则写明理由。
- **落库**：迁移 `transport_fulfillment/0057`，`transport_commission`、`booking_request` 各加 `document_number`、`document_number_absence` 两列（可空、旧行不回填，两列都空读作上线前开立），（租户，单据号）唯一，号不空白、原因只认 `NUMBERING_TIME_ZONE_NOT_CONFIGURED`、两者互斥。
- **答复与读口**：委托提交答复、订舱提交与取消答复、订舱册各带 `documentNumber`，没号时改带 `documentNumberAbsence`（`ISSUED_BEFORE_NUMBERING` / `NUMBERING_TIME_ZONE_NOT_CONFIGURED`），不答空串、不拿标识顶；订舱册另带 `commissionDocumentNumber`。撞号答 200 `NUMBERING_RULE_COLLISION`，带 `numberingRule`（撞的那一类的类型代码），不回显单据。承运答复口不变，照旧按订舱标识认单；`docs/api/carrier-booking-answer-integration.md` 补了单据号与订舱标识的关系。
- **管理台**：运输班次页订舱区主列「订舱号」取单据号、无号照旧显示订舱标识，订舱标识、运输委托号、委托标识与无号原因进技术详情，可按订舱号检索；挡回区那一列与筛选改叫「订舱标识」；词典追加 `bookingNumber`、`transportCommissionNumber`。
- **决定十**：管理台没有登记委托、订舱的表单（两口只有运营决定 API，演示脚本 `demo-run/chain.sh` 由调用方生成标识），没有可改的人填编号处；载荷照旧收委托、订舱标识作幂等键。
- **文档**：TF CONTEXT 规则句（时区来路、换版取消开始不换号、单据号不作认单键）；UC-TF-004 结果语义契约补「编号规则撞号」一行与两行「必须保留」，立 `AT-TF-371`–`373`，交接表本票那一行改「已补」；AT 盘点①加三行并追记（总览计数未动）。

### 关键取舍

1. **先落库后取号**：`Save` 用 `ON CONFLICT DO NOTHING`，并发首交的输家在那里等赢家提交、答已登记，一个号也不取；若先取号，输家那笔「已有委托」照常提交，取到的号就成了决定八之外的空号。代价是一笔开立两次写（INSERT 后 UPDATE 两列），记号的 UPDATE 要求两列都还空，重复记即报错回滚。
2. **取号错误交 error（5xx），不另立`未决`原因**：错误要么是部署故障（加扰密钥被撤）、编程错误（事务外、类型没声明），要么是事务里的库错误（事务已 aborted）；与 5xx「服务端没形成答案、退避后原样重发」续办相同，不为它多开一格。
3. **撞号的「人话」在答复上落成 `numberingRule`**：运营决定口的答复是机读的，把类型代码交回去，页面据此指到「设置 → 单据编号」那一类；今天管理台没有提交这两类的表单，没有页面要写那句话。
4. 渲染出的号立不成 TF 的单据号（超字节界）时适配器交错误，不在「是否已用」那一问上答已用——那会把存不下的号说成撞号、再烧一段流水。

### 验证（本机 PG `127.0.0.1:55432`）

- `go build ./...`、`go vet ./...` 退 0，改过的 `.go` 跑 `gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/transportfulfillment/... ./internal/architecture/... ./internal/platform/... ./cmd/... ./migrations/...` 全绿（首轮架构门 `TestEveryPersistenceWriteMethodCarriesTransactionRequiredEvidence` 指出两个记号写口缺无事务负向证据，已补进 `TestCommissionWritesRefuseToRunOutsideATransaction` 后复跑通过）；`-v` 抽查新真库用例 `TestCommissionAndBookingNumbersAreIssuedInTheIssuingTransaction`、`TestTheDocumentNumberColumnsGuardUniquenessAndShape`、`TestTheWiredCommissionAndBookingFacesIssueDocumentNumbers`、`TestAWiredCommissionThatCollidesIsRolledBackWithItsSequence`、`TestConcurrentWiredCommissionSubmissionsNeverShareOrSkipANumber` 全 PASS、无 SKIP，并发那条 `-count=5` 稳定。
- 前端 `tsc -b`、`pnpm test`（1042 过）、`vite build` 过。
- 种子灌一次性库 `docnum09_seed` 退 0（050 答 `VERSION_REGISTERED`）。**种子不开立任何委托或订舱**（两表 0 行），「种子开立的这几类单据都带号」无从核；开委托、订舱的是演示驱动 `demo-run/chain.sh`，种子已登时区，它开的会带号。
- 浏览器（真后端）：自编 parcel-api 起在 19099 连 `docnum09_seed`，经运营决定口真提交一份委托与一份订舱，答 `TC-202611-00001`、`BK-202611-00001`，订舱原样重交回显原号；另在该库手插两行无号订舱（两列都空 / 记时区未配置）看呈现。前端 5199 登录后运输班次页「订舱」区：主列 `BK-202611-00001` 与两行订舱标识，技术详情分别为「订舱标识 SYN-BOOKING-DOCNUM09-1 · 运输委托号 TC-202611-00001 · 运输委托标识 …」「单据号 上线前开立，无单据号」「单据号 开立时单据编号时区未配置，无单据号」；检索 `BK-202611-00001` 只剩那一行；挡回区筛选框为「按订舱标识筛」。验完已停 19099、5199 的进程并删库。

### 留给协调方

- 委托没有自己的列表与详情读口：委托单据号今天只在提交答复与订舱册（`commissionDocumentNumber`）上看得到；总单区技术详情「运输委托 / 订舱」两格与派发侧「实际履约待形成」等处仍显标识。是否另立委托册读口或给这些引用连读单据号，待裁。
- 新增端口、适配器与迁移会让机制清点过期，由推送方在 main 的 tip 上重生成；AT 盘点总览计数未重算。
- 共享面：`apps/admin-web/src/vocabulary/objects.ts` 在末尾追加两词；`.scratch/at-coverage-inventory.md` 在 ① 的 `AT-TF-370` 之后插三行、文末追记一段；`cmd/parcel-api` 只动了 TF 自己的装配文件（`assemble_transport_commission.go`、`endpoints_transport_commission.go`），端点表未改。
