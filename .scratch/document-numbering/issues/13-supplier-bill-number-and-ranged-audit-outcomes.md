# 13 供应商账单补全：接收口加供应商账单号（按外部标识记），审核补争议、拒绝并按金额范围审

Category: enhancement
Status: in-review
父票：[spec](../spec.md)
Blocked by: 无（从票 [11](./11-supplier-bill-and-audited-payable-admin-pages.md) 的分支起，票 11 进 main 后并一次 main）
出处：票 11 完成记录「留给协调方」第 1、2 条查出两处领域缺口——接收口载荷与接收表都没有放供应商账单号的格，票 11 只得把主张标识生成为「供应商/账单号」再从里面拆出来显示，与 GLOSSARY「外部标识」相悖；审核只能「通过」，而 SA `CONTEXT.md`「供应商主张与审核应付」与 UC-SA-004 要通过、争议、拒绝按金额范围并存，争议解除后只处理原争议金额。用户 2026-11-02 裁定「两处都做」：新立一份 ADR，再实现，管理台页面跟上，并替换票 11 的权宜做法。

## 要做的

1. **ADR**（取下一个空号）：供应商账单号是外部标识——接收载荷新增一格，记分配方（该供应商）与被标识对象（这份主张）；同一供应商同一账单号的重录语义（重放 / 版本 / 冲突）与主张标识幂等的关系，查重在服务端；不同供应商同号不冲突；旧数据没有这一格时的读法。审核结果三种（通过、争议、拒绝），按行按金额范围，一行可拆成若干范围，范围守恒（不超出已匹配金额、不重叠）；只有通过的范围形成审核应付并取应付单号；写口怎么表达「解除争议后再审」；拒绝是否终局。对 ADR-0340 的部分停用照 `docs/adr/README.md` 顶部写法挂指针、引原句逐字。越权风险点与 Alternatives（含「维持只通过」「账单号继续编进内部标识」）。
2. **CONTEXT / UC / GLOSSARY** 照 AGENTS「改文档」同步；UC-SA-004 立 AT 行、交接表更新。
3. **实现**（settlement-accounting）：领域、应用、端口、postgres 迁移（新列 / 新表只增不改、在有数据的库上可施加）、两个写口载荷扩展（新增格、旧载荷照收）、读口（账单册 / 行 / 应付册）带出账单号与各行的审核结果与范围、管理台表单与页面替换票 11 的权宜做法（主张标识不再编码账单号，旧数据照旧能显示）。审核表单按行给范围与结果、依据，答复说人话；应付单号仍只在通过的范围形成。
4. **测试**：领域单测（范围守恒、拆分、重复、终局）、应用与 HTTP、真库（并发审同一行、重放、争议解除后再审只处理原争议金额、旧载荷照收）；前端单测。AT 盘点照惯例加行。

## 不做

不做供应商门户、不改匹配门与五值分类、不开贷项口、不另立争议决定的授予（记越权风险点）。

## 完成判据

`go build`、`go vet`、`gofmt -l` 无输出，受影响包与 `./internal/architecture/... ./cmd/... ./migrations/...` 真库测试过；前端 `tsc -b`、`pnpm test`、`vite build` 过；种子灌一次性库退 0；浏览器在自起后端连一次性库，经表单登一张带供应商账单号的账单、对不同行分别给通过 / 争议 / 拒绝、解除争议后再审。只做 [workflow「进 main 的步骤」](../../../docs/agents/workflow.md#进-main-的步骤)第 1 步。

## 完成记录（2026-11-02，作者，分支 `claude-docnum13`，从 origin/main `060a6e72f` 起，先后并入 `origin/claude-docnum11` 的 `64cfae0c0` 与评审修订 `28a40f174`）

### 做了什么

1. **ADR-0357**（Accepted，代 SA owner 决）：供应商账单号是主张的外部标识，另记一册，（租户，供应商，账单号）只标识一份主张、服务端查重；审核按行按金额范围给通过、争议或拒绝，范围不重叠、通过之和不超已匹配金额（价差行取主张与预期之小，没有对应发生项与重复计费是零）；只有通过的形成审核应付并取应付单号，越权上限比这一行的已匹配金额；争议以点名它的再审解除、只能通过或拒绝、只落在原争议之内；通过与拒绝终局。部分停用 ADR-0340 决定一两句、决定二两句（Status 行与 Links 挂了指针），回答其越权风险点一的争议决定那一半。
2. **文档**：SA `CONTEXT.md` Language 加「已匹配金额」「审核决定」、改写「供应商主张与审核应付」三条、上限那条注明比整行；GLOSSARY「外部标识」举例加供应商账单号、「供应商账单主张」加两条；UC-SA-004 输入契约、立 `AT-SA-233`、`AT-SA-234`、`AT-SA-235`，交接两行改为已补；AT 盘点三行与追记。
3. **领域**：`SupplierBillNumber` 挂在主张上；`MatchedAmountFor` / `BillLineMatch.MatchedMinor` 是已匹配金额的唯一定义；`supplier_bill_audit.go` 的审核决定、`DecideBillLineAudit` 门、重建门与 `TallyLineDecisions`；`FormAuditedPayable` 改为从通过的决定形成应付，金额取范围长度。
4. **编排**：接收按主张与账单号排队、查号（`SUPPLIER_BILL_NUMBER_CONFLICT`），重放补号答版本冲突；审核在新的审核决定库上按身份与行排队，三种结果、争议与拒绝不取号也不交下游，同一段同一结果换个标识答先到的那份；旧载荷（只点名应付标识）照收、读作通过整行、摘要沿用旧形。新面 `AUDIT_BILL_LINE_RANGE` 的规范化摘要。
5. **迁移 `settlement_accounting/0054`**：供应商账单号册（主键即查重）、审核决定册（`btree_gist` 范围排他约束兜底不重叠）、按「通过整行」回填既有审核应付（标识、摘要、审核方、操作证据照搬，版本留空）、撤下 `audited_payable_one_per_line`、加按行索引。不改任何已有行。
6. **写口**：接收载荷加 `supplierBillNumber`；审核载荷加 `decision`、`verdict`、`rangeFrom`、`rangeTo`、`basis`、`resolvesDispute`，两种形状混给答 400；答复加 `DISPUTE_RECORDED`、`REJECTION_RECORDED`（201）、`EXISTING_DECISION`、`DECISION_CONFLICT`、`RANGE_ALREADY_DECIDED`，通过的带范围，行不可审带已匹配与已通过金额。
7. **读口**：账单册目录行加供应商账单号与已拒绝、仍在争议中、还没有结果三个和，有应付的行数按行数；账单各行改为带已匹配金额、各份决定（通过的带应付与应付单号）与四个和（取领域 `TallyLineDecisions`、`MatchedAmountFor`）；应付册加通过的那一段、所解争议与账单号。
8. **管理台**：登记表单把账单号单放一格，主张标识开表单时随机生成（`newClaimId`），同一供应商同一号已在册上就沿用那份（`claimIdForBill`）；账单号先取账单号册、旧行从「供应商/账单号」形主张标识取回（`legacyBillNumberOf`，只作显示）；账单与一行的进度按还没审、争议中金额说（待审核 / 审核人未配置 / 有争议待再审 / 已审完）；右栏各行列出每段结果、范围、依据、应付单号与和，「审核这一行」「核实后再审」共用 `SupplierBillLineAuditForm`，一段一份决定、逐段提交逐段答复，每段都记下就收起；应付右栏带通过的那一段。

### 关键取舍与理由

- **账单号另记一册、不进内容摘要**：号标识的是主张不是某一版，册的主键就是查重；进摘要要给接收口换规范化版本。同一版本两种号单独判成版本冲突。
- **账单号冲突一格两义**（号已标识别的主张 / 这份主张已有别的号）：恢复动作都是改录入，照 ADR-0029 并格，答复带出撞的是哪一边。
- **差异行能通过预期撑得住的那部分**：否则差异那一段卡住整行，与 CONTEXT「争议部分不得阻塞其他金额」相悖；这是越权风险点一。
- **上限比整行的已匹配金额**：按段比，拆段就绕过上限；ADR-0163 决定一不变，决定四（量差、价差不先问上限）由本票部分停用，见「评审修订」第 1 条。
- **范围记位置**：库上的排他约束兜得住「不重叠」与「再审只落在原争议之内」，人只填金额，页面替他排。
- **审核决定与审核应付两张表**：决定册担范围守恒，应付册照旧是应付；通过的决定标识即应付标识，回填让旧载荷重放照旧认得。
- **撤下「一行一份应付」**：一行可以有几段通过；这是对已有表的约束改动（不是只增），在有数据的库上可施加，真库用例证过。

### 验证（2026-11-02，本机 PG `127.0.0.1:55432`）

- `go build ./...`、`go vet ./...` 退 0；改过的 Go 文件 `gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/architecture/... ./cmd/... ./migrations/... ./internal/settlementaccounting/... ./internal/accessidentity/... ./internal/platform/...` 55 个包 ok、0 FAIL（含 PG）。新真库用例 `-v` 抽查全 PASS、无 SKIP：三种结果与再审、并发审同一行（重叠的一笔答范围已裁决、同段的一笔答先到的那份、只取两个号）、库上兜底与事务外拒、账单号并发同号只落一份、0054 在有旧应付的库上回填、旧载荷重放回填的应付、读面各段与三个和。
- `scripts/ci/generated-artifacts-pr.sh`：ADR 索引、AT 盘点总览计数、机制清点过期，留推送方；AT 表行与 UC 对得上。
- 前端：`npx tsc -b` 无输出，`pnpm test` 1124 pass / 0 fail，`npx vite build` 退 0。
- **种子**：一次性库 `docnum13_seed` 上 `scripts/demo-seeds/seed.sh` 退 0（含 0054）；往库里直写三版预期成本（8500、12000、3000 CNY）与一份票 11 形的旧接收行（主张标识 `SYN-PARTY-CARRIER-TRUNK-01/INV-2610-OLD`）。
- **浏览器**：自编 parcel-api 在 :19103 连 `docnum13_seed`，前端 :5203（Dex 回调照 rd-ops/shots.js 浏览器侧改写），配置员登录，`innerText` 核：登记 `INV-2611-001` 三行（8500 已对上、13000 对 12000 价格不符带依据、3000 已对上）答「已登记账单 INV-2611-001」，账单号册写下（分配方即该供应商、主张标识是 `BILL-…` 随机串）；同号再登一份（只一行）沿用那份主张、答版本冲突；列表旧行显 `INV-2610-OLD`。右栏第 1 行整行通过答「形成应付 AP-202611-00001」；第 2 行通过 12000、拒绝 1000 两段各答（`AP-202611-00002`、「已拒绝……另开追加账单」）；第 3 行整行争议答「记为争议……再审」，账单状态变「有争议待再审」、右栏各段与和对得上；第 3 行「核实后再审争议中的 3,000 CNY」通过 2000、拒绝 1000，答 `AP-202611-00003` 与拒绝，状态变「已审完」、已形成应付 3 行 22,500 CNY、已拒绝 2,000 CNY；应付签三笔以应付单号列出、账单号列是 `INV-2611-001`。页面无报错。验完停了两个进程、删了库。

### 留给协调方

1. **越权风险点待 SA owner 复核**（ADR-0357）：差异行能通过预期撑得住的那部分；账单号大小写不折算；账单号按供应商全期唯一；没号不查重；旧行不进查重；争议与拒绝不交下游；贷项仍没有口；撤下一行一份应付。
2. **并 main**：票 11 的评审修订已并进本分支（`28a40f174`）；票 11 进 main 之后要再 `git merge origin/main` 一次（内容应无冲突）。
3. **推送方重生成**：ADR 索引（ADR-0357 一行与 ADR-0340 注记）、AT 盘点总览计数（①、SA 各 +3）、机制清点。
4. **共享面**：ADR-0340 Status 行与 Links、SA `CONTEXT.md`、GLOSSARY 两条、UC-SA-004 交接表（与票 11 的修订行合过一次冲突）、`.scratch/at-coverage-inventory.md`、`AuditedPayableStore` 端口去掉 `FindByLine` 与 `LockIssuance`（只本上下文用）、迁移号 `0054`（若别的分支也取了 0054，以先进 main 的为准、本票改号）、ADR 号 0357（0356 留给票 12）。
5. **API 可见改动**：账单各行的 `payable` 单格换成 `decisions` 数组与四个和（票 11 新开、尚未进 main 的读口）；目录行与应付册只增；两个写口只增格，旧载荷照收。
6. 演示脚本 `scripts/demo-seeds/demo-run/30-settlement.sh` 仍用旧载荷（顺带证旧载荷照收），没有加账单号；要演示账单号可另加。

## 评审修订（2026-11-02，作者；非作者评审钉 `4bcdc59f7`，另并入票 11 复评转来的几条；追加提交、不 rebase）

0. **并 main**：`git merge origin/main`（`c5a9d879b`，票 11 已进 main），无冲突；AT 盘点追记按时间先后排（票 11 在前、票 13 在后）。
1. **ADR-0163 决定四挂指针**（阻断）：ADR-0357 让量差、价差行里已匹配的部分可以通过、通过时也问上限，扩了 ADR-0163 决定四「只拦会形成应付的已匹配行。量差、价差、无匹配和重复计费仍走原来的不可审，不先问上限。」——0163 的 Status 注记逐字引这句、写适用场景（只限按金额范围给出的通过，含再审通过；争议与拒绝不问上限）与「其余各条不变」，Links 加前向指针；0357 前段的去向、决定三、越权风险点一与 Links 改正（原先写「决定一不变」只说了一半）；本票关键取舍那一条同改。CONTEXT 与 UC 没有被停的句子（它们说的是「比整行的已匹配金额」，照旧成立）。
2. **争议决定另立授予**（协调方裁，照 UC-SA-004「权限」现行规则）：新决定种类 `SUPPLIER_BILL_DISPUTE_DECISION`（`accessidentity.DecisionSupplierBillDisputeDecision`，迁移 `access_identity/0039`，0038 留给票 12）。划分：给出结论——通过（含再审通过）与拒绝（含再审拒绝）——要 `SUPPLIER_BILL_AUDIT`；挂争议与对争议再审要 `SUPPLIER_BILL_DISPUTE_DECISION`；再审两种都要。拒绝归审核应付：它与通过同是终局结论，归到争议那一边，持争议授予的人就能不经审核应付把一段金额永久挡掉。审核口仍是一个口，Intake 先粗读结果与所解争议、按要的那几种逐一认证再判形状（读不出的请求体按旧形认审核应付，没有授予的人先答未授予）。ADR-0357 决定二、五、Consequences、Alternatives 改写，删去「暂不另立」；UC-SA-004 `AT-SA-234`、`AT-SA-235` 与交接行同改。演示种子给配置员登这一种授予、准入对照与权威区间（`data/access/34`、`35`，`data/governance/77`，步骤 `510`）。管理台按两种授予分别收起：结果下拉只给持有的那几种，「核实后再审」要两种都持，缺哪种就说哪种收起了。用例：`TestTheSupplierBillAuditIntakeAsksTheGrantsTheVerdictNeeds`、parcel-api `TestDisputeDecisionsAndAuditsAreGrantedSeparately`（只持审核的人给争议、再审答 403；只持争议的人通过、拒绝、再审、旧形答 403；两种都持再审通过形成应付）。
3. **账单号逐字收**（协调方裁）：`domain.NewSupplierBillNumber` 拒首尾空白（含全角空格）与控制字符（`ErrMalformedSupplierBillNumber`），接收口因此答 400、不进编排；大小写不折算。ADR-0357 决定一把「照来件录」改写成这一条（产品策略），越权风险点二换成「大小写不同视为不同号，是否折算待 SA owner 复核」，Alternatives 加「去空白、折大写再收」。页面去首尾空白再发（原就如此）。用例：领域、HTTP、parcel-api 各一。
4. **新版本换供应商**（Spec 非阻断）：定为账单号冲突——号的分配方是供应商，（另一家供应商，同一个号）是另一个外部标识，而这份主张已经有它的那一个。`FindClaimBillNumber` 连分配方一起答，编排比号也比分配方，答 `SUPPLIER_BILL_NUMBER_CONFLICT` 并带出 `registeredSupplierBillNumber` 与 `registeredSupplier`；号册写入的最后一道核验也认分配方。原先落成号册上的一致性错误、答 500。ADR-0357 决定一、CONTEXT、`AT-SA-233` 写明。用例：编排与真库各一。
5. **CONTEXT 幂等键那句**（Spec 非阻断）：改为旧形载荷是应付标识（`payable`）、按范围给结果的载荷是审核决定标识（`decision`），通过时它就是应付标识，都由调用方生成。
6. **审核结果的词与色**（Standards）：`vocabulary/status.ts` 的 `scopedStatusVocabulary` 加一册 `supplier-bill-audit`（通过 / 争议 / 拒绝），`supplier-bill-audit.ts` 改为 `verdictLabel` 取词典，右栏胶囊用 `code` + `scope`，去掉页内的码到词、码到色调两张表。
7. **跨文件计数**（Standards）：`supplier_bill_audit_decision.go` 头注改为不计数的说法；本票完成记录里数迁移约束的那处同改。
8. **`listAuditedPayables` 头注「两维」**：改为不计数的说法。
9. **技术详情「接收时审核授权」**：取词典（`CONFIGURED` / `NOT_CONFIGURED`，前者新增在总表）。
10. **截页与登记表单、右栏**：账单册读口加三个精确检索维 `claim`、`supplier`、`supplierBillNumber`（`ports.SupplierBillReceptionFilter`，在全册里逐字比）。登记表单「被追加」那一格下面加「找原账单」：按账单号在全册里找（找不到再按票 11 形的旧主张标识找一次），找到就并进选项并选上，同时说明列表只有最近收到的；新登的账单不在页上时，页面按主张单独读回它，右栏照样显示。用例：真库 `TestSupplierBillBooksCarryRangedDecisionsAndTheBillNumber` 补页外检索，端点 `TestSupplierBillReceptionQueryPassesTheExactLookup`，前端 `claimOfBillKey`。
11. **应付号检索的大小写**：应付单号是本产品签发的号，租户模板可以含小写字面，页面不折算；工具条提示改成「照号原样输入，区分大小写」。

验证：`go build`、`go vet ./...` 退 0，`gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/architecture/... ./cmd/... ./migrations/... ./internal/settlementaccounting/... ./internal/accessidentity/... ./internal/platform/...` 全过（含 PG）。评审探针放进 `internal/settlementaccounting/adapters/postgres/` 重跑：范围与终局、上限不可拆段绕过、五笔并发守住已匹配金额、0054 回填各项照旧；换供应商的新版本由 500 变为账单号冲突；带尾空格的号在领域构造即拒（探针那一行按设计失败）；跑完删掉、不入库。前端 `tsc -b` 无输出、`pnpm test` 1125 pass、`vite build` 退 0。一次性库 `docnum13_seed2` 灌种子退 0（含 `access_identity/0039`，配置员读得回三种供应商账单授予），已删。

## 评审修订（复评，2026-11-02，作者；复评钉 `437926b9a`；追加提交、不 rebase）

0. **并 main**：`git merge origin/main`（`969205ae9`，票 12 已进），只有 `.scratch/at-coverage-inventory.md` 追记冲突，两边都留、按时间先后排（票 12 的两条在前，票 13 在后）。0038 与 0039 各重建各的 CHECK，互不相干。
1. **ADR-0340 决定二首句挂指针**（阻断）：上一轮把审核口扩成按结果认两种授予，改的是 0340 决定二「`POST /settlement-supplier-bill-audits`，同一能力面，决定种类 `SUPPLIER_BILL_AUDIT`，与接收分开授予。」——0340 注记逐字引这句、写适用场景（只限供应商账单审核口；仍是运营决定能力面、仍与接收分开授予，旧载荷照旧只认 `SUPPLIER_BILL_AUDIT`）与「其余各条不变」，Links 改成「决定二三句」；0357 前段去向与 Links 同改，决定五写明认证次序。CONTEXT 与 UC 没有说审核口只认一种授予的句子（UC「权限」本就写分别授权）。
2. **主张各版本供应商不变**：换了供应商的新版本，不论带不带号，答 `CLAIM_SUPPLIER_CONFLICT`（`200`，带 `registeredSupplier`）。没有并进账单号冲突：换供应商的来件可能一个号都没带，账单号冲突说不出撞的是哪个号；这一格先于账单号两格判。新端口方法 `FindClaimSupplier`（最早落库那一版的供应商）。账单号冲突不再带分配方。账单册按账单号检索时供应商比号册上的分配方（`n.supplier_ref`），不给号时比接收行的供应商。ADR-0357 决定一、五、六与 Alternatives、CONTEXT、`AT-SA-233` 同改。用例：编排（带号、不带号两种）、真库（两种都不落库）、读面（存量里各版本供应商不一的主张，按另一家加号不命中、按分配方加号命中两版）。管理台答复说清是哪家供应商。
3. **格式字符**：`NewSupplierBillNumber` 同时拒 Cc 与 Cf（零宽空格、字节序标记、零宽连接符、双向控制符、软连字符……），ADR 措辞改为「控制字符（Cc）与格式字符（Cf）」。用例：领域六种 Cf、HTTP 三种。
4. **审核口认证次序**：先认持审核应付与争议决定之一（都不持答 403，请求体不读）→ 读体、判形状（坏形状答 400）→ 按结果核所需授予（不足答 403）。顺带去掉了认证前读 1 MiB 体。用例 `TestTheSupplierBillAuditIntakeChecksTheGrantsTheVerdictNeeds` 把只持审核、只持争议、两种都持、两种都不持四种人对八种载荷（含拼错的结果、小写的结果、读不出的请求体）逐格钉住。
5. **`findSupplierBillReceptions` 头注计数**：改为不计数的说法。
6. **审核口的接入面声明两种决定**：`accessFace` 加 `alsoDecisions`，审核口声明 `SUPPLIER_BILL_AUDIT` 与 `SUPPLIER_BILL_DISPUTE_DECISION`；换口用例 `swappedDecisionFaces` 改为每口一组决定种类——「只缺这一口的授予」那一条现在给的是这一口那一组之外的全部种类，两种都不持才答 403。路由清单那一格写成 `运营决定/SUPPLIER_BILL_AUDIT+SUPPLIER_BILL_DISPUTE_DECISION`。
7. **号册账单号的库约束**（0054 未进 main，直接改）：`supplier_bill_number_verbatim`——首尾不带 Go `unicode.IsSpace` 认的那几种空白，中间不含 C0、C1 控制字符与 Unicode 15 的 Cf 码位段；表在 0054 里新建，施加时是空的。真库用例钉住八种坏号落不进、中间空格与小写照收；一次性库 `docnum13_seed3` 灌种子退 0、约束在册，已删。
8. **新检索维带 NUL / 非法 UTF-8 答 500**：不在本票修，见「留给协调方」。

验证：`go build`、`go vet ./...` 退 0，改过的 Go 文件 `gofmt -l` 无输出，`go test -count=1 -p 1 ./internal/architecture/... ./cmd/... ./migrations/... ./internal/settlementaccounting/... ./internal/accessidentity/... ./internal/platform/...` 全过（含 PG）；前端 `tsc -b` 无输出、`pnpm test` 1125 pass、`vite build` 退 0。

### 留给协调方（复评追加）

- 账单册新检索维（`claim`、`supplier`、`supplierBillNumber`）带 NUL 或非法 UTF-8 时答 500：票 15（`claude-docnum15`）在全仓 HTTP 读口共用层加的入站检查进 main 后会覆盖，本票不另修。
- `CLAIM_SUPPLIER_CONFLICT` 是接收口新增的答复格，与账单号冲突并列；越权风险点照旧待 SA owner 复核。

## 评审修订（第三轮，2026-11-02，作者；复评钉 `4435dee67`；追加提交、不 rebase）

1. **两种授予各自的拒因不被吞掉**（阻断，上一轮引入）：审核口第一步把审核应付与争议决定两种授予都认一遍，认得过的记为已持有，各自的拒因按种存下。一种都认不过时答两种拒因里最能说明问题的那个（`telling`：身份依赖不可用 503 先于认不出的错误，再先于准入范围外 403，再先于未授予 403；令牌不成立、渠道没配直接答，不再试）；第三步缺哪一种就答那一种自己的拒因。由此：只持争议决定而争议那种准入未登答 403 `OUTSIDE_ADMISSION_SCOPE`、身份依赖不可用答 503；两种都持而只有审核那种准入未登的人给争议进编排，给通过答 403 `OUTSIDE_ADMISSION_SCOPE`。用例 `TestTheSupplierBillAuditIntakeKeepsEachGrantsOwnRefusal` 四格。ADR-0357 决定五写明这一次序。
2. **同一版本换供应商**：定为覆盖——同一份主张不论哪一版（新版本或同一版本的重发）供应商都不变，同一版本重发换了供应商也答 `CLAIM_SUPPLIER_CONFLICT`（这一格先于账单号与内容比对判）。ADR-0357 决定一、CONTEXT、`AT-SA-233` 写成这样，实现本就如此；编排用例补同一版本那一格。
3. **`CLAIM_SUPPLIER_CONFLICT` 的答复体**：端点 `TestAClaimSupplierConflictAnswersTheOriginalSupplier`（真编排、只走到查原供应商那一步的接收库替身）钉 200、`registeredSupplier`、不带账单号冲突的两格；parcel-api `TestTheReceptionFaceRegistersTheSupplierBillNumber` 补换供应商的新版本那一步。
4. **ADR-0357 决定一的层级**：「同一主张、同一版本」「新版本可带同号」两条子项挂回主张标识那一条下面，供应商不变那一条移到它们之后。
5. **库约束措辞**：改为「首尾空白照 Go `unicode.IsSpace` 认的那几种，C0 与 C1 控制字符，Unicode 15 的全部格式字符（Cf）码位段」。
6. **`alsoDecisions` 注释说的那一格**：补用例 `TestDecisionFacesOfSeveralKindsAdmitEitherKind`——一口认几种决定时，只持其中任一种的人过得了授予那一格（准入没登时停在 403 `OUTSIDE_ADMISSION_SCOPE`，不答未授予）；注释改成指名两条用例各核什么。
7. **名实**：`NumberConflict()` 只答账单号冲突，主张供应商冲突另出 `SupplierConflict()`（结果里另存原先的供应商），端点分别取；`Handle` 头注补上「这份主张已接收的版本是不是同一家供应商」那一步；真库用例注释改成反事实写法（「不先这样挡，带号的那一笔会……」）。

验证：`go build`、`go vet ./...` 退 0，改过的 Go 文件 `gofmt -l` 无输出，`go test -count=1 -p 1 ./internal/architecture/... ./cmd/... ./migrations/... ./internal/settlementaccounting/... ./internal/accessidentity/... ./internal/platform/...` 全过（含 PG）。本轮没动前端与迁移。

