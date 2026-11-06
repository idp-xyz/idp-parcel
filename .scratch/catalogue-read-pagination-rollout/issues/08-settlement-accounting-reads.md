# 08 settlement-accounting 各册迁到 ADR-0144

Category: enhancement
Status: in-progress——作者完成，待非作者评审（分支 `cursor/crpr-08-settlement`，worktree `idp-parcel-crpr-08`，基 `3a20b2bcc`；迁移编号 settlement-accounting `0056`，进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/settlementaccounting/{ports,adapters/http,adapters/postgres}` 里下表各册的读端口、端点与读面及其测试；`cmd/parcel-api` 里这些读口的占位；
`migrations/settlement_accounting` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

| 路由（选择器） | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/settlement-charges`（两种） | `ChargeCatalogueRead.ListCustomerCharges` / `ListSupplierExpectedCosts` | `c.charge_id` / `occurrence_id, fee_item, version` |
| `/settlement-statements` | `StatementCatalogueRead.ListCustomerStatements` | `s.statement_number` |
| `/settlement-supplier-bill-receptions` | `StatementCatalogueRead.ListSupplierBillReceptions` | `(claim ->> 'received_at')::timestamptz DESC, claim_id, claim_version` |
| `/settlement-disputed-statements` | `DisputedStatementRead.ListDisputedCustomerStatements` | `s.statement_number`，带计数与 `truncated` |
| `/settlement-funds-applications` | `FundsApplicationCatalogueRead.ListExternalFundsFacts` | `f.fact_id` |
| `/settlement-operating-results`（两种） | `OperatingCatalogueRead.ListOperatingResults` / `ListCostAllocations` | `scope_ref, period_ref, basis` / `allocation_id` |
| `/settlement-audited-payables` | `AuditedPayableCatalogueRead.ListAuditedPayables` | `p.audited_at DESC, p.payable_id` |

开工先核 `/settlement-statement-lines` 与 `/settlement-supplier-bill-lines`：按一张对账单、一张账单取行的属单条读取的从属面，按租户上列的则一并迁。
供应商账单接收的排序维今天取自 JSON 文档里的时刻，迁移时导出成非空列再作可排维（取舍 6）。

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：带计数的两册照取舍 5。

## 完成记录（2026-11-06，作者会话）

代码 tip `2403392db`，基 `3a20b2bcc`（在 `cursor/catalogue-pagination-rollout` 上）。用票 01 的共用件，未改 `internal/platform/cataloguepage`。

### 落点

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `6268a2b7b` | 本票面；`migrations/settlement_accounting/0056_catalogue_read_pagination_indexes.sql` | Status 改 in-progress。迁移 0056：账单接收的收到时刻导出成取值函数 `settlement_accounting.supplier_bill_received_at(claim)`，约束 `supplier_bill_reception_received_at_is_an_instant` 担保它非空且带时区；新建索引 `supplier_bill_reception_by_received_at`、`supplier_expected_cost_by_occurrence`、`audited_payable_by_audited_at`，其余各册复用主键索引（迁移头注逐册写明） |
| `237c0cef1` | `ports/catalogue_pages.go`（新）、`ports/catalogue_read.go`、`ports/supplier_bill_read.go`；postgres 读面 `charge_catalogue.go`、`statement_catalogue.go`、`supplier_bill_catalogue.go`、`funds_application_catalogue.go`、`operating_catalogue.go`；http `catalogue_intake.go`、`catalogue_page.go`、各 `query_settlement_*.go` 与 `query_supplier_bills.go`；`cmd/parcel-api/unwired_orchestration.go`、`cmd/parcel-api/assemble_settlement_decisions_test.go`；http 与 postgres 包里旧用例的调用点（含 `catalogue_read_test.go` 的空册用例）；新 `adapters/http/catalogue_page_test.go`、`adapters/postgres/catalogue_page_helpers_test.go` | 各册声明；读端口改为 `(ctx, tenant, limit, cataloguepage.Query)`（账单接收与审核应付在 limit 前保留检索维结构参数），答 `cataloguepage.Result[Row]`（争议队列答 `ports.DisputedCustomerStatements`）；读面 `MustBind` 落列、`Plan` 数总数与取页，游标位置取 ORDER BY 那几列的扫描值；端点「方法 → Intake → registry 分派 → 解码」，答复加 `page`；占位与替身只改签名 |
| `64fb53949` | `adapters/postgres/catalogue_pages_test.go`（新） | 各册真库翻页契约、收到时刻约束、缺省序走表达式索引 |
| `2403392db` | http `query_settlement_statements.go`、`query_supplier_bills.go`、`catalogue_query_test.go` | 两个取行端点的定位参数挪到 Intake 之后；未配置渠道对分派与定位参数一律同答的表驱动用例 |
| 本笔 | 本票面 | 完成记录 |

### 开工核查：两个取行面不迁

`/settlement-statement-lines?statement=`（`FindStatementLines`）与 `/settlement-supplier-bill-lines?claim=&version=`（`FindSupplierBillLines`）各按一张单取它自己的行：缺定位参数答 400，单不在册答 `STATEMENT_NOT_FOUND` / `SUPPLIER_BILL_NOT_FOUND`，不按租户上列、不分页。属单条读取的从属面，不迁、不解码；本票只把它们的定位参数判定挪到 Intake 之后（见判断项）。

### 各册声明（`ports/catalogue_pages.go`；落列在各 postgres 读面）

| 册名 | 路由 → 读端口方法 | 缺省序 | 另放可排维 | 行标识 | 筛选维 | 选择器 | q 覆盖 | 索引 |
|---|---|---|---|---|---|---|---|---|
| `customer-charges` | `GET /settlement-charges?registry=customer-charge` → `ListCustomerCharges` | `charge` 正序 | `formedAt` | 费用标识 | `stage`（`ChargeStage` 词表） | `registry` | 费用标识、费用项目、评价、结算相对方、来源事实、确认依据 | `customer_charge_pkey` |
| `supplier-expected-costs` | `GET /settlement-charges?registry=supplier-expected-cost` → `ListSupplierExpectedCosts` | `occurrence` 正序 | `recordedAt` | （费用项目、版本） | — | `registry` | 版本、发生项、费用项目、协议、供应商、评价 | 新 `supplier_expected_cost_by_occurrence` |
| `customer-statements` | `GET /settlement-statements?registry=customer-statement` → `ListCustomerStatements` | `statementNumber` 正序 | `publishedAt` | 单号 | — | `registry` | 单号、结算账户、账期、单上各项争议的争议号 | `customer_statement_pkey` |
| `supplier-bill-receptions` | `GET /settlement-statements?registry=supplier-bill-reception` → `ListSupplierBillReceptions` | `receivedAt` 倒序 | `period` | （主张身份、版本），同向倒序 | — | `registry`、`claim`、`supplier`、`supplierBillNumber` | 主张身份、账单号、供应商、账期、法人 | 新 `supplier_bill_reception_by_received_at`（表达式索引） |
| `disputed-customer-statements` | `GET /settlement-disputed-statements` → `ListDisputedCustomerStatements` | 同对账单册 | 同对账单册 | 单号 | — | — | 同对账单册 | `customer_statement_pkey` |
| `audited-payables` | `GET /settlement-audited-payables` → `ListAuditedPayables` | `auditedAt` 倒序 | — | 应付标识，同向倒序 | — | `documentNumber`、`claim`、`supplier` | 应付标识、应付单号、主张身份、账单号、供应商、账期 | 新 `audited_payable_by_audited_at` |
| `external-funds-facts` | `GET /settlement-funds-applications` → `ListExternalFundsFacts` | `fact` 正序 | `occurredAt` | （事实、版本），只列链头那一版 | `kind`（`FundsFactKind` 词表） | — | 事实标识、来源登记 | `external_funds_fact_version_pkey` |
| `operating-results` | `GET /settlement-operating-results?registry=operating-result` → `ListOperatingResults` | `scope` 正序 | `recordedAt` | （范围、账期、基准） | `basis`（`OperatingBasis` 词表） | `registry` | 范围、账期、计算版本 | `operating_result_pkey` |
| `cost-allocations` | `GET /settlement-operating-results?registry=cost-allocation` → `ListCostAllocations` | `allocation` 正序 | `recordedAt` | 分摊标识 | — | `registry` | 分摊标识、来源、规则版本 | `cost_allocation_pkey` |

缺省序都与迁移前同（spec「缺省序保今天的次序」）；按登记身份排的册另放一维登记时刻，要看新登的在前用 `sort=-<那一维>`。

### 完成判据（spec「每张迁移子票（02–13）的做法与判据」）

| 判据 | | 证据 |
|---|---|---|
| 真库：每册翻页途中插入新行，后面各页不重出、不漏行 | ✅ | `adapters/postgres/catalogue_pages_test.go` 每册一条：`TestCustomerChargesPageWithoutRepeatsOrGapsWhileChargesArrive`、`TestSupplierExpectedCostsPageAlongTheVersionChains`、`TestCustomerStatementsPageWhileStatementsArriveAndFindTheDisputeNumber`、`TestDisputedStatementCountStaysTheBacklogWhileTheKeywordNarrowsThePage`、`TestSupplierBillReceptionsPageByTheReceivedInstantWithTiesBrokenTheSameWay`、`TestAuditedPayablesPageNewestFirstWithTiesBrokenTheSameWay`、`TestExternalFundsFactsPageOneChainHeadPerFact`、`TestOperatingResultsPageAcrossTheThreePartIdentity`、`TestCostAllocationsPageWhileAllocationsArrive`。倒序册新行落在已翻过的那头；正序册排在已翻过那头的新行不出现、排在后头的在末页出现，原有行各出现恰好一次；每页断言 total |
| 真库：筛选或 q 改变 total | ✅ | 同上各条的表驱动段与 `TestCustomerChargesTotalFollowsTheStageFilterAndTheKeyword` 断言 total：`stage`、`kind`、`basis` 各收窄，q 各命中一格；账单接收与审核应付另证选择器与 q 为与；争议队列证 q 改变 `page.total` 而张数仍是积压 |
| 真库：q 的 `%` 按字面 | ✅ | 每册一例「q 的 % 按字面」，零命中、total 0 |
| 真库：空册答空页、total 0 | ✅ | `adapters/postgres/catalogue_read_test.go` 的 `TestEmptySettlementRegistersAnswerEmptyLists`：各册全空时 total 0、没有 next，争议队列张数 0 |
| 端点：合规一问带 `page`，size 取 Intake 页大小 | ✅ | `adapters/http/catalogue_page_test.go` 的 `TestEveryPagedSettlementCatalogueAnswersThePageTriple` |
| 端点：集外键、集外排序维、词表外筛选值、坏游标各答 400 带 detail | ✅ | `TestPagedSettlementCataloguesRefuseMalformedPageQueries`，每册逐一跑 `malformedPageQueries` 列出的坏请求：未知键、自报 `tenant`/`limit`、集外排序维（正、倒）、排序给两次、坏游标、q 过长；有筛选维的册另加词表外取值与空值 |
| 端点：换条件拿旧游标答 400 | ✅ | `TestEveryPagedSettlementCatalogueRefusesItsCursorUnderOtherConditions`：同条件续翻时读口收到的 `After` 即签发时的位置，换 q、换排序各答 400，detail 是共用件的理由原文；`TestASettlementCursorOnlyContinuesTheListItWasMintedFor`：别册的游标答 400 |
| 端点：渠道未配置时参数写对写错都答 403 | ✅ | `TestUnconfiguredChannelRefusesEveryPagedQueryWhateverItsParameters`：每册不带参数、合规排序与 `malformedPageQueries` 的每一种坏请求都答 403，答复体逐字节相同；`adapters/http/catalogue_query_test.go` 的 `TestAnUnconfiguredChannelAnswersAlikeWhateverTheDispatchOrLocatorParameters`：`registry` 缺、空、未知、属别的路由，两个取行端点缺定位参数，同上 |
| 每迁一册补一条与缺省序同形的索引 | ✅ | 迁移 0056 新建 `supplier_bill_reception_by_received_at`、`supplier_expected_cost_by_occurrence`、`audited_payable_by_audited_at`，其余复用主键索引；`TestSupplierBillReceptionDefaultOrderWalksTheReceivedAtIndex` 证缺省序走表达式索引、计划里没有 Sort |
| 可排维与行标识的列非空 | ✅ | 各列本为 NOT NULL；收到时刻由约束担保，`TestASupplierBillReceptionWithoutAnInstantReceivedCannotBeRecorded` 证 null、不带时区、日在前、不是时刻的取值都被拒 |
| 迁移无 CR、无 BOM | ✅ | 按字节量过：CR 0、无 BOM |
| 管理台与脚本已在用的查询参数不变成 400 | ✅ | 见下「管理台、脚本与 cmd 带的查询参数」 |

PASS 计数取证（代码 tip `2403392db`，带 DSN）：

```sh
export IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable'
go test -count=1 -p 1 -v -run 'TestCustomerChargesPage|TestCustomerChargesTotal|TestSupplierExpectedCostsPage|TestCustomerStatementsPage|TestDisputedStatementCount|TestSupplierBillReceptionsPage|TestASupplierBillReceptionWithout|TestSupplierBillReceptionDefaultOrder|TestAuditedPayablesPage|TestExternalFundsFactsPage|TestOperatingResultsPage|TestCostAllocationsPage|TestEmptySettlementRegisters' ./internal/settlementaccounting/adapters/postgres/ | grep -cE '^--- PASS'
# 13（连子用例 53），--- SKIP 0
go test -count=1 -p 1 -v -run 'TestEveryPagedSettlementCatalogue|TestPagedSettlementCataloguesRefuse|TestUnconfiguredChannelRefusesEveryPagedQuery|TestASettlementCursorOnly|TestAnUnconfiguredChannelAnswersAlike' ./internal/settlementaccounting/adapters/http/ | grep -cE '^--- PASS'
# 6（连子用例 125）
```

### 门

- `go build ./...`、`go vet ./...` 全仓 exit 0；改过与新增的 `.go` 经 `gofmt -l` 为空。
- `go test -count=1 -p 1 -v`（带 DSN）跑改过的包及其反向依赖（`go list -test -deps` 反查，含 `cmd/*`），外加 `./internal/architecture/...`、`./migrations/`、`./internal/platform/migrate/`：72 个包（其中 2 个无测试文件）全 ok，顶层 PASS 4719、连子用例 PASS 7242，SKIP 1，FAIL 0。唯一的 SKIP 是 `internal/platform/pgtest` 的 `TestHelperTemplateOwnerProcess`，只在子进程里跑的助手，设计如此。与本票直接相关的包（顶层/连子用例 PASS，SKIP 均 0）：`internal/settlementaccounting/adapters/postgres` 314/391、`adapters/http` 76/331、`application` 262/580、`cmd/parcel-api` 446/654、`internal/architecture` 58/174、`internal/platform/migrate` 8/8、`migrations` 1/1。
- 迁移号 0056 在基、在 `cursor/catalogue-pagination-rollout` 的 tip `fda0dd80d`、在 `origin/main` 的 `b24359996` 上都未被占（2026-11-06 核）；`origin/main` 自合并基 `a1274a6b8` 以来没有碰本票地盘的提交。

### 管理台、脚本与 cmd 带的查询参数

| 调用方 | 带的参数 | 声明 |
|---|---|---|
| 管理台费用、对账单、经营结果页 | `registry` | 选择器，每个取值一册 |
| 管理台账单登记表单 | `registry=supplier-bill-reception` 另带可选的 `claim`、`supplier`、`supplierBillNumber` | 选择器；端点照旧读进 `ports.SupplierBillReceptionFilter`，空串即不筛 |
| 管理台审核应付页 | `documentNumber` | 选择器（连同 `claim`、`supplier`），端点照旧读进检索维结构，空串即不筛 |
| 管理台待对账页、资金页 | 不带参数 | — |
| `scripts/demo-seeds/demo-run/30-settlement.sh`、`scripts/demo-seeds/demo-run/chain.sh` | `registry` | 选择器 |
| `cmd/parcel-api` 的 `getSettlementBook` 与装配探针 | `registry`、`documentNumber` | 选择器 |

选择器的取值进游标摘要：换了 `claim` 之类再拿旧游标答 400。

### 判断项与风险

- **分派与定位参数挪到 Intake 之后（行为变更）**。`/settlement-charges`、`/settlement-statements`、`/settlement-operating-results` 迁移前先判 `registry` 再 Intake（旧用例 `TestAnUnknownRegistryIsRefusedBeforeTheChannelCheck` 钉过），未配置渠道时坏 `registry` 答 400；现照 spec「端点先 Intake、后解码查询参数」与 ADR-0055 决定三改为先 Intake，旧用例换成 `TestAnUnconfiguredChannelAnswersAlikeWhateverTheDispatchOrLocatorParameters`。两个取行端点不迁，但同一条决定也管它们，定位参数（`statement`；`claim`、`version`）一并挪到 Intake 之后，单独成笔 `2403392db`：评审若认为取行端点不在本票范围，回退这一笔即可，不牵动各册。
- **决胜键改同向**。账单接收迁移前 `received_at DESC, claim_id, claim_version`，现为三段同向倒序；审核应付 `audited_at DESC, payable_id` 现为两段同向倒序。只有同一时刻的几行相对次序变，`supplier_bill_catalogue_test.go` 里同刻两份的断言随之翻过来。其余各册决胜键本就与排序同向，次序不变。
- **行标识不是主键去租户的**。预期成本主键是（租户、版本），行标识取（费用项目、版本），为保迁移前「发生项、费用项目、版本」的链序：版本本身唯一，多一段不改唯一性。首版幂等的唯一索引前缀相同而末段是计价规则版本、且只覆盖首版，不能顶替，故新建 `supplier_expected_cost_by_occurrence`。
- **争议队列答复体自带 Next/Total**。`ports.DisputedCustomerStatements` 不嵌 `cataloguepage.Result`：迁移前它已是带 `Count` 与 `Statements` 的结构体，保留这两格，另加与 `Result` 同义的 `Next`、`Total`，形状同 parcel-shipment 的队列（`31fcbf4f2`）。`disputedCount` 是整个队列的积压，不随 q 变：带 q 时按固定条件另数一次，不带 q 时就是 total、不多一问。张数与本页不在同一快照里，并发挂上新争议时两数可差。
- **只有争议队列带计数**。票面「带计数的两册」与代码不符：账单接收与审核应付迁移前只带 `truncated`，不带张数。两册的 `truncated` 改为「`page.next` 不为空」，与 spec「已带计数与 truncated 的待办队列保留这两格」对 truncated 的改法一致；缺省序不带条件的第一页上，它与迁移前同义。
- **收到时刻导出成取值函数，不是列**。票面写「导出成非空列」。生成列的表达式须 IMMUTABLE，而文本转 timestamptz 是 STABLE（随 DateStyle、TimeZone 变），仍得靠同一个标 IMMUTABLE 的函数；加存储生成列还要重写全表、多存一份，表达式索引已够排序用。IMMUTABLE 只靠约束才诚实：约束把能存的取值限定成带时区的 ISO 8601，这一形的解析与两项会话设置无关。函数体里的类型转换使它不被内联，读面与索引调同一个函数才对得上，`TestSupplierBillReceptionDefaultOrderWalksTheReceivedAtIndex` 证了这一点。
- **风险：约束对存量行即时校验**。0056 加约束时校验全部存量行，有一行不合形迁移即失败。写口按 RFC 3339 序列化 `time.Time`，演示种子也走写口，预期没有；上线前可在目标库先跑 `SELECT count(*) FROM settlement_accounting.supplier_bill_reception WHERE claim ->> 'received_at' IS NULL OR NOT ((claim ->> 'received_at') ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$')`，应为 0。形对而值错的取值（如 13 月）约束放行、写入时由索引表达式的转换报错：迁移前它在读列表时报错，现在提前到写入时。
- **大册的精确总数**。客户费用、预期成本随包裹与发生项计量，外部资金事实随收付款计量；照 spec「total 一律给精确值」每问一次 COUNT。租户规模上来 COUNT 若成瓶颈，回 ADR-0144 改，不在本票。
- **q 不走索引**（ADR-0144 越权风险点 5）。各册 q 是多列 ILIKE。对账单的争议号按单以换行拼成一串再做字面包含，检索词自带换行时可能跨两个号命中；管理台检索框不产生换行。
- **自报 `tenant`/`limit` 改答 400**（ADR-0144 决定四）。既有用例里断言「被忽略照答 200」的改成断言 400 且不带行，并补一问不带参数照答本租户的行。
- **审核应付的应付单号可缺**，不放作可排维（spec「可排维与行标识的列必须非空」）。
- **走索引的用例是同形手写语句**。`TestSupplierBillReceptionDefaultOrderWalksTheReceivedAtIndex` EXPLAIN 的是与读面缺省序同形的手写查询：读面 SQL 由 `Plan` 在方法内拼出、不导出。读面形状若再改，这条用例不会跟着报。
- **票面路由与代码不符**。票面写账单接收的路由是 `/settlement-supplier-bill-receptions`，那是 POST 登记口；列表读是 `GET /settlement-statements?registry=supplier-bill-reception`。

### 未验

- 管理台未改、未在浏览器里点：各页答复多出 `page`，页面照读旧字段；页面翻页归票 14。
- 全量 `scripts/test-full.sh` 未跑，由推送方在重放后的 tip 上跑。
- 迁移 0056 未在演示库（`scripts/demo-seeds` 灌过的库）与任何存量库上跑过；约束对存量行的校验见判断项。
- 大数据量下的计划选择未量：走索引的用例在小表上关掉顺序扫描与排序，证的是「能走」，不是「会选」。
