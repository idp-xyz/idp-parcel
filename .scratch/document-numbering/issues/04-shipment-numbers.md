# 04 委托的单据号（界面词「运单号」）

Category: enhancement
Status: in-progress——2026-11-02 作者完工、自验、推分支 `claude-docnum04`（完成记录见文末）；待非作者评审与进 main
父票：[spec](../spec.md)
Blocked by: [02](./02-platform-numbering-adoption-and-rule-register.md)
出处：spec 子票表；[ADR-0355](../../../docs/adr/0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md) 决定二、十一；UC-PS-001

## 本票的特殊处

- 开立时点：草稿 → 已提交（首方客户渠道提交、管理台代提交等全部提交入口）。同一接入请求重投（按既有重放键）答原号；补件形成新提交版本不换号。
- **首方客户接口**：提交答复与委托查询带运单单据号，新增字段，不改既有字段与语义（ADR-0140；契约由端点描述生成，跟着重生成）；客户追踪视图能否按运单单据号定位，本票核实现状，能做就做、不能就记进完成记录。
- 管理台：运单列表、运单详情、待受理复核、取消与处置等页的「运单号」改为显示单据号（今天是去掉 `SHR-` 前缀的委托标识，见 `pages/shipment-request/ref-no.ts`）；按运单号检索走单据号。
- 时区：委托开立方若有可用的已登记业务时区（例如发件网点），按决定七取它并写进 CONTEXT；没有就按租户的单据编号时区，未登则不带单据号。界面词「运单号」经管理台用语词典，领域与代码里说委托的单据号。

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

## 完成记录（2026-11-02，作者，分支 `claude-docnum04`，从 origin/main `3a408b7b0` 起）

三笔提交：`d4610251b`（后端：领域、端口、编排、适配器、迁移、装配与测试）、`e347ff0bf`（CONTEXT、UC-PS-001、首方契约、AT 盘点）、`ea230b91f`（管理台），加本记录一笔。

### 做了什么

1. **时区核定**：委托开立时没有可取的开立方业务时区——提交时还没有发件网点（网点在接受之后的路由里才定，提交载荷里也没有这一格），客户账户本身不登业务时区（对账安排的业务时区属结算账户）。按 ADR-0355 决定七取租户的单据编号时区，类型声明 `ShipmentDocumentNumberType` 维持 `DateTimeZoneFromTenant`（只改注释），规则写进 PS CONTEXT「委托」草稿 → 已提交那一句。
2. **开立事务内取号**：消费侧端口 `ports.ShipmentDocumentNumberIssuer`（适配器 `internal/parcelshipment/adapters/docnumbering`，把平台取号编排的三格全函数译成本上下文三格，判撞查本上下文的表）与 `ports.ShipmentDocumentNumberRecorder`（`pspostgres.ShipmentRequests.RecordShipmentDocumentNumbering`）。提交编排在建单答「已建成」之后、同在续办守卫那一笔里取号并记号；取到号随委托同一笔落定；时区未配置照常建成、记「单据编号时区未配置」；撞号交回哨兵错误让守卫整笔回滚（答案、委托、信封、取过的流水一起消失），守卫外译回 `NUMBERING_RULE_COLLISION`。parcel-api 把 main 在同一个 db 上装的那一套 `docnumbering.Components.Assigner` 交给提交编排，不另装。
3. **领域**：`ShipmentDocumentNumber`、`DocumentNumberAbsence`（`ISSUED_BEFORE_NUMBERING` / `NUMBERING_TIME_ZONE_NOT_CONFIGURED`）、`ShipmentDocumentNumbering`；委托聚合带这一格，新转移 `RecordDocumentNumbering` 只记一次、只对`已提交`（开立后不变、事后不补号）；重建随两列读回。
4. **落库**：迁移 `parcel_shipment/0035_shipment_request_document_number.sql`：`document_number`、`document_number_absence` 两列可空（后者只收时区未配置一格，两列皆空即上线前开立、不回填），互斥 CHECK，（租户，单据号）唯一。记号的 UPDATE 只写两列皆空的那一行、零行即错、不推进 revision。三处重建读面（按来源身份、按包裹找已接受委托、提交版本声明）都读两列。
5. **答复与读口**：首方提交答复（含 `EXISTING_RESULT` 重放）新增 `documentNumber`，无号时整键不出现、改带 `documentNumberAbsence`；委托查阅列表、单份、复核队列、授权处置队列、补充队列共用的概要列面与释放未受理册都带这两格（概要列面改成共享常量 `summaryColumns` + `targets()`）。
6. **管理台**：运单列表主列与排序按运单号（单据号），搜索命中单据号、委托标识照旧可搜；详情栏、详情页页头、待受理复核、授权处置、释放未受理册、新建运单结果显单据号；技术详情按记下的情形注明「上线前开立，无单据号」或「开立时单据编号时区未配置，无单据号」；新建运单结果收撞号一格、指到「设置 → 单据编号」；撤回与取消表单指名委托的那一格改叫「委托标识」（两口照旧按委托标识找委托，填运单号会答找不到）。界面词仍经 `objectNouns.shipmentNumber`，没有往共享词典里加新词（无号说明是本目录纯函数 `ref-no.ts`）。
7. **文档**：PS CONTEXT 规则句；UC-PS-001 提交答复「已提交」「已有结果」两行带单据号、新增「编号规则撞号」一行、立 `AT-PS-177`–`AT-PS-179`、交接那一行改「已补」；首方客户 API 契约补两格字段与新 `outcome`；AT 盘点①补三行（总览计数未动）。

### 关键取舍与理由

- **先建单、后取号，不先取号再 INSERT**：建单答「已存在」的那一路不落单而事务照常提交（答案已记），先取号就留下空号（决定三）；而且建单的 `INSERT … ON CONFLICT DO NOTHING` 会把单据号唯一约束的冲突也吞成「已存在」。所以取号与记号放在建单答「已建成」之后，记号是同一笔里的一次 UPDATE。
- **单据号两列不进快照**：号开立后不变，Save 改写快照不碰它们；放进文档就是第二个来源。两列皆空在读回时译成「上线前开立」而不留零值——零值是「还没开立」，能读出来就能被补号。
- **取号与记号不套事务壳**：其余边界壳是为让守卫外的调用不撞 `RequireExecutor`；这两口套了壳，守卫外的调用会自己开一笔事务取号，号就不随委托落定。不套，事务外调用在事务判上就失败。
- **撞号是答案不是故障**：首方答 200 `NUMBERING_RULE_COLLISION`、不指名委托；答案不留存，原样重发接着办（规则册换格式前仍撞，换了即建单，流水从回滚处接着数）。UC 结果表随之加行，单一权威仍是 ADR-0355 决定八。
- **首方字段名**：照首方族既有的平铺 camelCase，答复层级上与 `shipmentRequestId` 并列，取领域词「单据号」的 `documentNumber`；无号说明 `documentNumberAbsence` 一并给客户，ADR-0355 决定七要答复如实写「单据编号时区未配置」。
- **契约「重生成」**：OpenAPI 生成器与端点描述符至今未落（票 docs-consistency/code/82），字段以 `adapters/http` 响应结构体为准，所以改的是结构体与 `docs/api/first-party-api-contract.md`，没有可跑的生成物。
- **客户追踪视图不按运单号定位（核实未做）**：视图按包裹出（ADR-0223，`GET /v1/customer-tracking-view?parcel=`），一张运单可有多件，按运单号定位要改答复形状（一对多）且归 VE，VE 读模型也不持有委托单据号——需另立 VE 的票与决定；契约文档已写明「追踪视图仍按 `parcelId` 定位，不收运单号」。
- **释放未受理册的单据号另起一句读**：册行的过滤与计数共用一段 `FROM … WHERE`，连表要拆开它；按清单上的委托另查一次，读不到的委托不带这一格（不替它说「上线前开立」）。

### 验证（2026-11-02，本机 PG `127.0.0.1:55432`）

- `go build ./...`、`go vet ./...` 退 0；改过与新增的 `.go` 跑 `gofmt -l` 无输出。
- `go test -count=1 -p 1 ./internal/parcelshipment/... ./internal/architecture/... ./internal/platform/... ./cmd/... ./tests/... ./migrations/...`：58 个包全 ok（含 PG）；改释放未受理册之后对 `./internal/parcelshipment/... ./internal/architecture/... ./cmd/parcel-api/...` 复跑全 ok。`-v` 抽查新增真库用例全 PASS、无 SKIP：parcel-api 五条（`TestASubmittedShipmentTakesItsDocumentNumberAndAReplayAnswersTheOriginal`、`TestAShipmentWithoutANumberingTimeZoneIsSubmittedWithoutANumber`、`TestARolledBackSubmissionReleasesItsDocumentNumber`、`TestConcurrentSubmissionsTakeDistinctDocumentNumbers`（12 路并发）、`TestANumberingRuleCollisionRollsTheSubmissionBack`）、pspostgres 四条（读回与查阅、时区未配置、事务内记号与判撞 + 唯一约束 + 跨租户、释放未受理册带号）。架构门（含无事务负向证据门、生产接线棘轮）过。
- 管理台：`npx tsc -b` 过，`pnpm test` 1044 过 0 败，`npx vite build` 过。
- 种子：一次性库 `docnum04_seed` 上 `scripts/demo-seeds/seed.sh` 退 0（050 答 `VERSION_REGISTERED`，迁移 0035 施加）。种子本身不开立委托（库里委托 0 行），委托的号经下面的真提交核。用后删库。
- 真提交（自编 parcel-api `:19094` 连 `docnum04_seed`，首方客户令牌）：两笔答 201 `SUBMITTED`、`documentNumber` 为 `P20261102000001`、`P20261102000002`（上海时区 19:33，按日周期 `20261102`）；对第一笔原样重发答 200 `EXISTING_RESULT` 带原号、序列值不动；操作者读 `/shipment-request-views` 各行带号。
- 浏览器（前端 `:5194` 代理到 `:19094`，`innerText` 核）：列表主列显 `P20261102000001`/`P20261102000002`，按 `P20261102000002` 检索只剩那一行；另两笔在库里用 SQL 改成无号（一笔两列皆空模拟上线前开立、一笔记时区未配置）后，列表显去前缀的委托标识，详情栏技术详情首行「运单号 上线前开立，无单据号」，详情页技术详情首行「运单号 开立时单据编号时区未配置，无单据号」，有号的详情页页头为单据号、委托标识在技术详情；无页面错误、无写请求。待受理复核与授权处置两队列在这个库里没有行，靠单测与类型检查覆盖。核完停掉 `:19094`、`:5194`。
- 迁移施加在有数据的库上：用 origin/main `3a408b7b0` 编出的迁移助手建库 `docnum04_mig`（364 步），从 `docnum04_seed` 拷入 4 行委托（旧列），再用本分支迁移助手施加：`applied=1`（`parcel_shipment/0035`），4 行两列皆空、不回填；重跑 `applied=0`。用后删库。未碰 `demo_live`。

### 留给协调方

- 生成物：AT 盘点总览计数、机制清点（新适配器包 `adapters/docnumbering`、新迁移）由推送方在 main tip 上重生成。
- 迁移号 `parcel_shipment/0035`：合并时若别的票也占了这个号，改号即可（资产只增不改，内容不依赖号）。
- 其他上下文的页面今天仍显去前缀的委托标识：网络路由三页（路线规划、改路建议、初始路由并列）表头「运单」、搜索「搜运单号」——网络路由读模型不带委托单据号，按单据号搜不到；要一致需另票（读模型随接受信封带上单据号，或管理台按委托查阅合并）。
- 物流动态、异常、清关、计费等页把**包裹号**标成「运单号」（`objectNouns.shipmentNumber` 或字面量）——自本票起「运单号」专指委托单据号，这些标签应另票改成「包裹号」。
- 壳层多标签里详情标签名取 hash 里的委托标识，不是运单号（壳层地盘，本票未动）。
- 撞号文案指向「设置 → 单据编号」，那一页属票 03。
