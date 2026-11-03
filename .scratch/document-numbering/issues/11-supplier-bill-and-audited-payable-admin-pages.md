# 11 管理台补「供应商账单与应付」：账单册与应付册读口、查阅页、接收与审核两张任务式表单

Category: enhancement
Status: resolved——2026-11-02 进 main（记录见文末「进 main 记录」）；复评非阻断四条转票 13 一并修
父票：[spec](../spec.md)
Blocked by: 无
出处：票 05 记下「管理台没有列审核应付的页，应付号只在审核答复里看得到」；协调方核实整个供应商账单接收与审核在管理台都没有页面、后端只有两个写口（`POST /settlement-supplier-bill-receptions`、`POST /settlement-supplier-bill-audits`，ADR-0340）。用户 2026-11-02 裁定「补全套：查阅加接收 / 审核表单」。

## 要做的

1. **后端读口**（settlement-accounting，照该上下文既有读口分层与授予）：
   - 供应商账单册：按租户列出已接收的供应商账单主张（主张、版本、供应商、账期、金额、各行），每行带匹配结果（已匹配 / 部分匹配 / 未匹配）与审核结果（通过 / 争议 / 拒绝）的汇总；单份详情带各行与它们的匹配、审核去向。
   - 审核应付册：列出审核应付，带应付单号（`documentNumber` / `documentNumberAbsence`，ADR-0355）、所属主张与行、金额、审核人；可按应付单号、主张、供应商检索。
   - 只读、租户隔离、读不到答 500 不报空；照 UC-SA-004 的结果语义契约取词，不另造状态。
2. **管理台「账务 → 供应商账单与应付」页**（工作台形态，照改版样板）：两签——供应商账单、审核应付。主列用业务字段（供应商名经往来单位册、账单号、账期、金额、状态胶囊），应付签以应付单号为主列（号名与无号说法取 `vocabulary/document-numbering.ts`），内部标识进技术详情；右栏看明细；检索与指标即筛；诚实态分开。
3. **接收表单「登记供应商账单」**：任务式，按 UC-SA-004 输入语义契约逐格收（供应商、账单号、账期、币种、各行金额与依据等）。供应商自己的账单号是外部标识（GLOSSARY「外部标识」），按外部标识收与显示；本方内部的主张标识由页面在首次提交前生成做幂等键（接收口载荷里的账单身份由调用方给，ADR-0340 决定一；原票面挂在 ADR-0355 决定十名下是引错了——决定十列的是订舱、运输委托、审核应付与手工发布对账单，没有供应商账单），不让人填。答复说人话（受理、待解析、未受理及理由）。
4. **审核表单「审核这张账单」**：在账单右栏，按行给出通过 / 争议 / 拒绝与金额范围、依据；审核载荷里的应付标识由页面生成做幂等键（ADR-0340、ADR-0355 决定十）；答复里交回应付单号并在应付签可见；越权上限、编号规则撞号（`numberingCollisionNote('AUDITED_PAYABLE')`）、时区未配置各说人话。
5. 两张表单都按各自的运营决定授予收起（`SUPPLIER_BILL_RECEPTION`、`SUPPLIER_BILL_AUDIT`；`useMayMakeDecision`，ADR-0315）。

## 不做

不改两个写口的语义与载荷形状（只在管理台生成原来由调用方给的标识）；不做供应商门户；不改匹配与审核规则。

## 完成判据

读口有 HTTP 与真库用例（含租户隔离）；UC-SA-004 交接与 AT 照惯例补；管理台 `tsc -b`、`pnpm test`、`vite build` 过；浏览器在自起后端连一次性库（灌种子后经两张表单真登一张账单、审一次）核过两签、右栏、两张表单、收起；后端部分照「进 main 的步骤」第 1 步交评审。

## 完成记录（2026-11-02，作者，分支 `claude-docnum11`，从 origin/main `060a6e72f` 起）

三笔提交：`faeff46d0`（后端读口与各层用例）、`906218b92`（管理台），加文档与本记录一笔。

### 做了什么

1. **账单册汇总**：没另立第二个列表口，扩 `GET /settlement-statements?registry=supplier-bill-reception` 那一本的目录行（`ports.SupplierBillReceptionCatalogueRow`）：主张金额之和、按逐行匹配分类原名计的行数（`classificationCounts`，不归并成已匹配 / 部分匹配 / 未匹配）、这一版各行上已形成应付的行数与金额、已匹配而没有应付的行数、到达时刻、被追加的原主张、接收操作者（ADR-0340 越权风险点三说的「操作证据只写不读」，接收与应付两处现在读得到）。一条语句、同一快照；「已匹配」的字面由领域交给 SQL，不在 SQL 里另写。
2. **账单各行** `GET /settlement-supplier-bill-lines?claim=&version=`（`ports.SupplierBillLineRead`）：各行按主张原序，带匹配分类、所引预期成本、差额（主张减预期）、依据、匹配时刻，以及挂在这一行上的审核应付（应付标识、应付单号或无号原因恰出其一、金额、审核授权方、点审核的人、审核时刻）。不在本租户册上答 `SUPPLIER_BILL_NOT_FOUND`（200），缺 claim 或 version 答 400。
3. **审核应付册** `GET /settlement-audited-payables`（`ports.AuditedPayableCatalogueRead`）：新形成的在前；带应付单号或无号原因（有号、时区未配置、上线前开立三种分开）、所属主张与行、预期成本版本、供应商与账期（取这份主张最早落库的那一版接收记录）、法人、应付归集账户、审核授权方与点审核的人；`documentNumber`、`claim`、`supplier` 三个可选精确检索维，同给取交集。找不到接收记录的应付是册上矛盾，答 `NO_ANSWER_FORMED`，不当成没有供应商上列、也不悄悄漏掉。
4. 两个新口挂主数据与运营查阅读面（`endpoints_settlement_catalogue.go` 追加两行），与对账单册同一个适配器（`StatementCatalogue`）；未接线占位同步。
5. **管理台「账务 → 供应商账单与应付」**（`SupplierBillsPage`）：两签工作台。供应商账单签：账单号、供应商名（往来单位册）、账期、金额、对上情况、状态胶囊（待审核 / 审核人未配置 / 有差异待核实 / 已审完，是从计数派生的呈现词，不是领域状态）、收到时间；指标即筛、表头排序、右栏看账单全貌与各行去向。审核应付签：应付单号为主列（号名与无号说法取 `vocabulary/document-numbering.ts`），没号显应付标识并悬停说原因；右栏分开「点审核的人」与「有权审核的一方」，应付标识、主张标识、操作证据进技术详情。
6. **登记供应商账单**（页头按钮 → 对话框，`SupplierBillReceptionDialog`）：按 UC-SA-004 输入逐格收——供应商、开给本公司哪个主体、供应商的账单号与版本、账期、币种、收到时间、被追加的原账单（可缺）；逐行选对的预期成本（只列同币种、当前那一版：去掉失效纠错版本与有后继的）或「找不到对应运输」，对上方式只给匹配门收的那几种（金额相等才能选已对上），差异带依据。答复逐格说人话。
7. **审核这张账单**（右栏 `SupplierBillPanel`）：已对上、还没有应付的行摆「审核通过，形成应付」；应付标识页面在首次提交前生成（`PAYABLE-<时间>-<随机>`，前缀避开应付单号默认的 `AP-`），同一行重发落回同一份；答复交回应付单号并可一键跳到应付签按号检索；越权上限、撞号（`numberingCollisionNote('AUDITED_PAYABLE')`）、审核授权 / 应付归集账户 / 越权上限三种未配置、行不可审各说各的。
8. 两处写入口各按 `SUPPLIER_BILL_RECEPTION`、`SUPPLIER_BILL_AUDIT` 授予收起（`useMayMakeDecision`，ADR-0315），收起时原处说只对持哪种授予的人显示。
9. **对账单页**去掉原来的「供应商账单」签（移到新页，两处列同一本册只会各说一套），地址里不再写签，`reconciliation-address` 只留「只看待对账」；首页待对账格的地址不变。导航「账务」在「对账单」后加一条，对账单页的一句话说明改为客户对账单与争议。
10. **文档**：UC-SA-004 新立 `AT-SA-232`，交接里 `AT-SA-231` 那一行改说应付号在哪里读得到，另加两行缺口（见下）；AT 盘点加一行与追记（总览计数未动）。

### 关键取舍与理由

- **主张标识取「供应商/账单号」而不取随机串**（票面写「页面生成做幂等键」，这里仍是页面在首次提交前生成，只是确定地生成）：接收口载荷与接收行都没有单放供应商账单号的一格，票面又不许改载荷；随机串会把账单号丢掉、主列无号可显，而且同一张纸面账单两个人各录一遍就成两份主张、可以对同一份预期成本对上两次账（编排不跨主张查重）。确定地生成，同一家供应商同一个号不论谁录都落回同一个主张（重放答已存在，内容不同答版本冲突，AT-SA-092/093 在人工录入下也成立）；带上供应商，两家供应商同号不撞。读面从主张标识里取回账单号显示，取不到（接口或演示脚本送入的主张）就照旧显主张标识。这是借法，见「留给协调方」第 1 条。
- **审核只有「通过」**：ADR-0340 只开了形成审核应付的口，供应商账单行的争议与拒绝没有口（越权风险点一）；票面第 4 条的「争议 / 拒绝与金额范围、依据」在领域与写口里都不存在，没有自造按钮或状态，对差异行只说要先核实。金额范围也不收：审核不改数字，应付金额就是这一行的主张金额（`domain.FormAuditedPayable`）。
- **匹配汇总不归并**：UC 结果语义里的「未匹配 / 部分匹配 / 已匹配」三格在领域里是逐行五值分类；读口按原名计数，管理台经用语词典逐类说中文（词典追加五个码），归并成三格就是读面替匹配门再判一次。
- **账单列表不另立口**：扩既有的接收册目录行，不在 `/settlement-supplier-bills` 另开一本同表的列表；详情与应付册各开一口。
- **应付册的供应商取接收记录**：应付行不带供应商与账期，按主张回指；同一主张多个版本取最早落库那一版（供应商与账期是主张身份的一部分）。

### 验证（2026-11-02，本机 PG `127.0.0.1:55432`）

- `go build ./...`、`go vet ./...` 退 0；改过的 Go 文件 `gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/architecture/... ./internal/platform/... ./cmd/... ./migrations/... ./internal/settlementaccounting/... ./internal/accessidentity/...` 55 个包 ok、0 FAIL（含 PG）；`bash scripts/ci/generated-artifacts-pr.sh` 退 0（AT 计数、机制清点过期，留推送方）。`-v` 抽查新真库用例全 PASS、无 SKIP：`TestSupplierBillReceptionCatalogueSummarisesLinesAndPayables`、`TestSupplierBillLinesCarryMatchAndPayablePerLine`、`TestAuditedPayableCatalogueListsNumbersAndFilters`（四种检索、页大小、租户隔离）、`TestAnAuditedPayableWithoutItsReceptionIsAnError`、parcel-api `TestTheSupplierBillBooksShowTheAuditedPayableNumber`（经两口真登真审后三本册读回同一个应付单号）。
- 前端：`npx tsc -b` 无输出，`pnpm test` 1099 pass / 0 fail，`npx vite build` 退 0。
- `go run ./tools/at-coverage-counts -check` 只差总览计数（①、合计、SA 各 +1），表行与 UC 对得上，留推送方重生成。
- **种子**：一次性库 `docnum11_seed` 上 `scripts/demo-seeds/seed.sh` 退 0；种子不开立供应商账单与审核应付（两表 0 行），也没有供应商预期成本，「种子开立的都带号」无对象可查——往库里直写两版预期成本（8500、12000 CNY）后由下面的浏览器核验真登真审。
- **浏览器**：自编 parcel-api 起在 :19101 连 `docnum11_seed`，前端 :5201（Dex 回调照 rd-ops/shots.js 浏览器侧改写），`innerText` 核：配置员空态页头有「登记供应商账单」；经对话框登记 `INV-2610-001`（第 1 行对 8500 已对上自动选上，第 2 行对 12000、写 13000 选价格不符带依据）答「已登记账单 INV-2610-001，对上的行可以在右栏审核。」；列表一行、状态「待审核」、对上情况「对上 1/2 行 · 价格不符 1」，右栏两行各自去向，差异行说先核实；第 1 行点审核答「第 1 行审核通过，形成应付 AP-202611-00001，金额 8500 CNY。」，审后右栏挂上应付单号、状态变「有差异待核实」；点「在「审核应付」里看」落到应付签、按号筛出那一笔，主列 `AP-202611-00001`，右栏分开点审核的人与 `SYN-AUDITOR-SETTLEMENT-01`，技术详情是应付标识 `PAYABLE-…`；对账单页只剩客户对账单。再登一张 `INV-2610-002`（只一行已对上、不审）：配置员右栏有 1 个审核按钮；查阅员登录后页头说「登记供应商账单入口只对持供应商账单接收授予的操作者显示。」、右栏说「审核入口只对持供应商账单审核授予的操作者显示。」，两个按钮都是 0 个。验完停了两个进程、删了库。

### 留给协调方

1. **供应商账单号没有自己的一格**（要裁）：接收口载荷与 `supplier_bill_reception` 都没有单放供应商账单号（外部标识，GLOSSARY 要记标识类型、分配方、适用范围）的位置；本票按票面「不改写口载荷」，让页面把主张标识生成为「供应商/账单号」、读面再从中取回显示。要正经记外部标识，需改 ADR-0340 决定一的载荷（加 `supplierBillNumber` 一类的格）与接收行一列，属写口契约变更，另立票；届时 `supplier-bills.ts` 的 `claimIdFor` / `billNumberOf` 换掉即可。UC-SA-004 交接已记这一行。
2. **供应商账单行的争议与拒绝没有口**：UC-SA-004 结果语义与 SA CONTEXT 要「通过 / 争议 / 拒绝」三种结果，ADR-0340 只开了通过（越权风险点一）。管理台对差异行只说先核实，没造按钮；要补得先有领域决定与写口，另立票。UC-SA-004 交接已记。
3. 同一张账单换个收到时间重录，内容摘要不同（摘要含到达时刻），答「版本冲突」——这是现行摘要口径的诚实结果，页面答复说清了「先核对已登记的那份」；若要「同号同内容仅时间不同算重放」，是摘要口径的事，不在本票。
4. **共享面**：`vocabulary/status.ts` 在总表末尾追加五个匹配分类码；`operator-grants.ts` 追加两个决定原词；`cmd/parcel-api/endpoints_settlement_catalogue.go` 追加两行与两只依赖；`.scratch/at-coverage-inventory.md` 追加一行与一条追记。`AT-SA-232` 若与别的分支撞号，以先进 main 的为准、本票改号。
5. API 可见改动：账单接收册目录行多出汇总各格（只增不改）；新增两个只读口。对账单页的「供应商账单」签撤掉、移到新页。
6. 推送方在 main 的 tip 上重生成 AT 盘点总览计数（①、SA 各 +1）与机制清点。

## 评审修订（2026-11-02，作者，钉 `64cfae0c0` 的非作者评审）

提交：合并 `9b208dfd4`、后端 `2546a25f9`、管理台 `0d4a459b8`，加文档与本节一笔。先合 origin/main `3b4720283`（票 10 已进）：`.scratch/at-coverage-inventory.md` 追记两边各加一行，两行都留，票 10 在前、票 11 在后。

**Standards 1（注释数别处的东西）**：`ports.SupplierBillReceptionCatalogueRow` 头注改说「领域里逐行的走向是 `domain.MatchClassification` 的各个取值」，UC 那一层也不再数格；`vocabulary/status.ts` 改说「除它之外的分类」；`supplier-bills.ts` 的 `differingLineCount` 改说「分类不是已对上」、`payableNumberState` 改说「没有号的原因」。顺手清了同类的几处（`SupplierBillPanel` 头注「只有一种」、`seedAuditedPayable` 与用例头注里数形状的词）。分类码不再写进界面句子：`supplier-bills.ts` 立 `matchClassificationHints` 取值表（原 `hints`，登记表单的提示也从它取），`differingClassificationWords()` 从表里取除已对上之外的界面词；`SupplierBillPanel`「对不上的行（…）」与审核答复 `LINE_NOT_AUDITABLE` 那一句都用它。

**Standards 2（变更说明）**：`reconciliation-address.ts` 头注改说地址里为何只有「只看待对账」一项；`ReconciliationPage.tsx` 头注改说供应商账单接收册为何只在「供应商账单与应付」页列（两处列同一本册就各说一套状态与指标）。

**Standards 3（孤儿应付带供应商检索被滤掉）**：`ListAuditedPayables` 的供应商条件改为 `r.supplier_ref IS NULL OR r.supplier_ref = $4`——找不到接收记录的那一笔不因对不上供应商被滤掉，照样走到扫描时的矛盾检查，带不带供应商检索都答错误。`TestAnAuditedPayableWithoutItsReceptionIsAnError` 加三种情形（不检索、按那家供应商、按另一家供应商）都答错误，另证按应付单号点名有接收记录的那一笔照常答。

**Spec 1（审核人未配置是接收时的快照）**：取第一种改法——读口带出此刻的配置，不改快照语义。理由：审核步骤在审核那一刻读审核授权册（`ReceiveSupplierBillHandler.Audit` 调 `LoadSupplierAuditAuthority`），页面要说「现在审核会不会停在未配置」，该看的就是这本册此刻有没有这一组；只把横幅改成「接收时」又不据此说审核结果，租户补登审核人之后状态胶囊就永远说不出「待审核」。账单册目录行加 `AuditAuthorityConfiguredNow`（JSON `auditAuthorityConfiguredNow`，只增），同一条语句里 `EXISTS` 查 `supplier_audit_authority` 的（租户、供应商、责任法人）；`auditAuthorityConfigured` 照旧是接收时写下的快照、语义不动。页面 `billProgress` 与右栏横幅改看此刻那一格，快照进技术详情「接收时审核授权」。真库 `TestSupplierBillReceptionCatalogueReadsTheAuditAuthorityAsItIsNow`（后来补登的：快照 false、此刻 true；别的租户、别的责任法人登的不算），前端用例补快照与此刻相反的两种。

**Spec 2（两本册截在 200 行）**：账单册服务端改按到达的业务时刻倒序截页（同刻按主张身份与版本），最新收到的总在页上；两个列表端点向读口多要一行判页外有没有，答复加 `truncated`（只增），多出的那一行不上列。页面工具条截了就不说「共几份」，改说「只列出最近收到的 N 份，更早的没有列出、指标也不算它们」（应付签同理，按审核时间）。应付签的检索词同时当应付单号走服务端精确检索维（`listAuditedPayables({ documentNumber })`，250 毫秒防抖），命中的那一笔并进检索池（`withExactHits`），页外的应付也搜得到；这一问没答出时工具条明说只在这一页里找。真库 `TestSupplierBillReceptionCatalogueKeepsTheLatestReceivedOnThePage`（主张身份排最前的那份收到最早，截页后不在页上），端点 `TestSupplierBillReceptionQueryFlagsAPageCut` 与应付册用例里的截页一段，前端用例证工具条那句与检索池。

**Spec 3（登记表单的预期成本不按供应商筛）**：查了，**没有可用的格，没做，记缺口**。预期成本的读口（`ports.SupplierExpectedCostCatalogueRow`）与表（迁移 `settlement_accounting/0008` 的 `supplier_expected_cost`）都没有供应商或承运方，领域 `SupplierExpectedCostSpec` 与 `TransportChargeOccurrence` 也没有；唯一沾边的是 `agreement_ref`——它依的那一版供应商协议的版本标签，协议是哪家供应商的归 party-commercial（管理台的 `/commercial-supplier-agreements` 只在协议内容已登记时带供应商，同一标签在 PC 一侧还可能比中不止一版，见 `ErrSupplierAgreementLabelAmbiguous`）。要按供应商筛，得在结算这边新记一格或跨上下文对一次协议，都超出本票「不新增领域概念」的界，按吩咐不硬做。UC-SA-004「给开发的交接」加一行缺口（不挂票号，去处「待立票」）；`currentExpectedCosts` 注释写明为何不筛；登记表单在「账单各行」说明里如实提醒录入的人：预期成本上只记了协议、没记供应商，这里不筛、登记时也不核，选之前看清协议是不是这家的。

**引文纠正**：票面「要做的」第 3 条把「主张标识由页面生成」挂在 ADR-0355 决定十名下，改为 ADR-0340 决定一（载荷里的账单身份由调用方给），并注明原引错了；代码注释与完成记录里没有这处引用。主张标识编码供应商账单号这一权宜做法本身没动（票 13 另行替换）。

**文档**：UC-SA-004 `AT-SA-232` 补截页与 `truncated`、审核授权两格、全册检索、按供应商检索时孤儿也答未形成；交接加上面那一行缺口。AT 盘点 `AT-SA-232` 那一行补三条新用例名。

### 修订后的验证（2026-11-02，本机 PG `127.0.0.1:55432`）

- `go build ./...`、`go vet ./...` 退 0；改过的 Go 文件 `gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/settlementaccounting/... ./internal/architecture/... ./cmd/... ./migrations/...` 34 个包 ok、0 FAIL（含 PG）；`-v` 抽查真库与装配用例全 PASS、无 SKIP（含新的 `TestSupplierBillReceptionCatalogueKeepsTheLatestReceivedOnThePage`、`TestSupplierBillReceptionCatalogueReadsTheAuditAuthorityAsItIsNow`、改过的 `TestAnAuditedPayableWithoutItsReceptionIsAnError` 与 parcel-api `TestTheSupplierBillBooksShowTheAuditedPayableNumber`）。
- 前端：`npx tsc -b` 无输出，`pnpm test` 1115 pass / 0 fail，`npx vite build` 退 0。
- 没有迁移，不另起一次性库；`go run ./tools/at-coverage-counts -check` 仍只差总览计数（合并 origin/main 前后同样），留推送方重生成。本轮没有重走浏览器。

## 评审记录（2026-11-02，非作者隔离评审）

- 初评钉 `64cfae0c0`：两轴阻断为零；非阻断见上「评审修订」逐条处理。对「主张标识编码供应商账单号」的判断（契约内可行、与 GLOSSARY「外部标识」部分缺口、查重应移到服务端、旧主张标识永久存在）转给票 13，由 ADR-0357 接替。
- 复评钉 `28a40f174`：两轴阻断为零。非阻断四条，因票 13 重写同一片代码，转票 13 一并修：
  1. `pages/settlement/api.ts` 的 `listAuditedPayables` 头注「两维」数了 Go 读口 `ports.AuditedPayableFilter` 的维数。
  2. `SupplierBillPanel` 技术详情「接收时审核授权」的值写死「已配置 / 未配置」，没经词典。
  3. 截页信息没传到登记表单：`SupplierBillReceptionDialog` 的「被追加」选项取自页内账单，截页时更早的原账单选不了；新登账单若早于页尾，右栏为空。
  4. 应付号精确检索区分大小写（服务端 `document_number = $2`，页内不分），工具条说「按完整的应付单号搜」。
- 协调方对作者所问：两个列表答复新增 `truncated`、`auditAuthorityConfiguredNow`（只增字段）与账单册改按收到时间倒序，都是操作者族读面，接受。Spec 3 的缺口用户 2026-11-02 裁定「预期成本记下供应商」，立票 14。

## 进 main 记录（2026-11-02，协调方）

- 在 origin/main `3b4720283` 上 `git merge --no-ff origin/claude-docnum11`（tip `28a40f174`，已含作者合 main 的 `9b208dfd4`），无冲突；合并提交 `e5fd624f8`。
- main tip 上重生成 AT 盘点总览计数（`5243386f0`）、机制清点（`ded2420a0`）；ADR 索引无变化。
- 全量真库测试 `scripts/test-full.sh`：退 0，174 个包 ok。
- 一次性库 `seed_docnum11` 灌种子：退 0，`种子灌入完成：租户 SYN-TENANT-01`。
- 无迁移；演示环境换 parcel-api 二进制与前端。
