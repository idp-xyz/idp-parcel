# 14 预期成本记下供应商：登记账单按供应商筛预期成本，匹配门核供应商

Category: enhancement
Status: in-progress
父票：[spec](../spec.md)
Blocked by: 无（从票 [13](./13-supplier-bill-number-and-ranged-audit-outcomes.md) 的分支 tip `4bcdc59f7` 起，已含票 11、票 13 与 main `3b4720283`；票 11、13 进 main 后并一次 main）
出处：票 [11](./11-supplier-bill-and-audited-payable-admin-pages.md) 评审 Spec 非阻断 3——登记表单的预期成本不按供应商筛（只筛当前版与币种，供应商下拉列全部往来单位），匹配门 `MatchBillLine` 也不核供应商（ADR-0340 越权风险点四），误选一次就把账单对到别家的预期成本；票 11 修订查明预期成本的读口、表与领域类型都没有供应商，只有它依的那一版协议（`agreement_ref`），协议属哪家供应商归 party-commercial，于是在 UC-SA-004「给开发的交接」记了一行缺口（去处「待立票」）、登记表单加了一句提示。用户 2026-11-02 在三个选项（预期成本记下供应商 / 匹配时经往来单位与协议方核对 / 暂不补）里选「预期成本记下供应商」：形成预期成本时从协议取出供应商一并记下（之后不变），登记表单按所选供应商筛，匹配时在结算上下文内直接比对、不对就拒；先写 ADR 再做。

## 要做的

1. **ADR**（取下一个空号）：预期成本形成时供应商从哪来（形成载荷带入，还是由 SA 经消费侧适配器问 party-commercial 协议的供应商，择一说理由），记下后不变，协议换供应商时怎么读；匹配门核供应商、答复形状（新拒因码照既有命名）与 ADR-0357 范围审核的关系；旧数据没有供应商一格的读法与能否回填（不得把租户取值写成默认）；对 ADR-0340 越权风险点四的回答；改到的已接受记录照部分停用写法挂指针、引原句逐字；越权风险点与 Alternatives（含「匹配时经协议方核对」「暂不补」）。
2. **CONTEXT / UC / GLOSSARY** 照 AGENTS「改文档」同步；UC-SA-004 立 AT 行，交接表票 11 记的那行缺口改为已补并写落点。
3. **实现**（settlement-accounting，party-commercial 只加读口）：领域、应用、端口、postgres 迁移（只增、在有数据的库上可施加）、形成编排经消费侧适配器取供应商、读口 `SupplierExpectedCostCatalogueRow` 带出供应商、匹配门核供应商；旧行补记口；管理台登记表单按所选供应商筛预期成本，去掉票 11 加的那句「这里不按供应商筛」提示。
4. **测试**：领域单测（供应商不符拒）、应用与 HTTP、真库（旧数据、补记、跨供应商误配被拒、租户隔离）、适配器测试；前端单测。AT 盘点照惯例加行。

## 不做

不改 party-commercial 的领域规则与协议生命周期；不重判已接收的账单；不在接收口临时去问 party-commercial；不改审核、已匹配金额与取号。

## 完成判据

`go build`、`go vet`、`gofmt -l` 无输出，受影响包与 `./internal/architecture/... ./cmd/... ./migrations/...` 真库测试过；前端 `tsc -b`、`pnpm test`、`vite build` 过；种子经 `demo-run/up.sh` 灌一次性库退 0；浏览器在自起后端连一次性库，登记表单选一家供应商只出那家的预期成本，经接口把账单行对到别家预期成本被拒。只做 [workflow「进 main 的步骤」](../../../docs/agents/workflow.md#进-main-的步骤)第 1 步。

## 完成记录（2026-11-02，作者，分支 `claude-docnum14`，从 `origin/claude-docnum13` 的 `4bcdc59f7` 起，并入票 13 评审修订 `437926b9a`）

### 提交

`14882a436` 立票 → `2d7f3a27c` ADR-0358 与文档 → `8432ad601` PC 窄读口 → `a3f97900b` SA 领域、编排、端口、持久化、迁移与装配 → `1460f1a67` 补记口 → `6b0cd4152` 管理台 → `1f572c37a` 演示脚本 → `c3301de12` 并入 `origin/claude-docnum13` → `8989c5752` AT 盘点两行 → `c5c08c848` 表单一处空格 → 本笔完成记录。

### 做了什么

1. **ADR-0358**（Accepted，代 SA owner 决）：预期成本形成首版与纠错版本时，由结算经消费侧适配器问 party-commercial「这一版协议的供应商是谁」、记在这一版上，失效纠错版本照搬；记下后不改，纠错版本随协议引用重问（后继协议换了一家就记新的那一家）；匹配门对点名了成本的四种分类先核供应商（排在当前版之后、币种之前），不同答 `EXPECTED_COST_SUPPLIER_MISMATCH`、没记答 `EXPECTED_COST_SUPPLIER_UNRECORDED`，都 `200`、不落账单；迁移只加可空一列、不跨上下文回填，旧行经补记口按协议补；读口带出供应商，管理台按所选供应商筛。部分停用 ADR-0340 决定一「重放、版本冲突、未受理、点名的预期成本不是当前版，都是答案（`200`）。」（再扩两格）、ADR-0200 决定四「下列各格上抛错误，落发布失败：」的列举与决定五「BUY 请求册、评价读口、预期成本库答不出时，整笔回滚等重投。」；回答 ADR-0340 越权风险点四里「点名别家预期成本」那一种误选。两份被停记录的 Status 行与 Links 挂了前向指针。
2. **文档**：SA `CONTEXT.md` Language「供应商预期成本」加一句，「供应商成本与共享分摊」加一条（每一版记下协议的供应商、旧行补记），「供应商主张与审核应付」加一条（匹配门核供应商）；GLOSSARY「供应商预期成本」加一条；UC-SA-002 结果契约「供应商预期成本已形成」必须保留里加供应商；UC-SA-004 输入契约「预期成本」与主流程步 4 跟上，立 `AT-SA-236`、`AT-SA-237`，交接表票 11 记的那行缺口改为已补并写落点。
3. **party-commercial**：只加读口 `ports.SupplierAgreementSupplierView` 与 PG 实现 `SupplierAgreementSuppliers`；与协议范围那一口共用一段按两段式指称比对的 `loadQualifiedSupplierAgreement`。领域规则与协议生命周期没动。
4. **SA 领域**：`SupplierExpectedCostSpec`、`CostCorrectionSpec` 必带 `Supplier`，重建可缺；`SupplierExpectedCost.Supplier()`；`MatchBillLine` 加 `ErrBillSupplierMismatch`、`ErrExpectedCostSupplierUnrecorded`（没有对应发生项点名了成本仍答形状错，不答供应商不符）。
5. **SA 编排与端口**：`FormSupplierExpectedCostDeps.Suppliers`（缺了答错误），判过失效与等撤下之后、构造之前问；读不动未决 `SUPPLIER_AGREEMENT_VIEW_UNAVAILABLE`（外层折进未决、不记停因），PC 不认答 `ErrOccurrenceAgreementNotRegistered`、译不过来答 `ErrSupplierAgreementSupplierUntranslatable`，都上抛；供应商不进内容比对。接收编排两种新答复与 `SupplierRefusal()`。补记编排 `BackfillExpectedCostSupplierHandler`（同一版协议只问一次、只写仍空的、PC 不认的留空列出、读不动写不进未决）。消费侧适配器 `adapters/partycommercial.SupplierAgreementSupplier`。
6. **持久化**：迁移 `settlement_accounting/0055` 加可空 `supplier_party_ref`（非空不得空白）与只含未记行的部分索引，不改已有行；预期成本库存取供应商，多两问 `ListExpectedCostsAwaitingSupplier`、`RecordExpectedCostSupplier`（`UPDATE … WHERE supplier_party_ref IS NULL`）；预期成本册读口带 `Supplier`。
7. **写口与读口**：接收答复加 `expectedCostSupplier {line, version, supplier?}`；`GET /settlement-charges?registry=supplier-expected-cost` 每行加 `supplier`（未记整键不出现）。预期成本没有外部写口（只由派发进程按 BUY 评价形成），没有「旧载荷照收」的问题。
8. **装配与命令**：派发进程的形成装配接 PC 读口；`parcel-settlement-register expected-cost-supplier-backfill`（输入只有 `tenantId`；全补上退 0、有补不上的退 2 且其余照写、读不动写不进退 3 整笔回滚）。
9. **管理台**：登记表单只列所选供应商的当前、同币种预期成本，没选不列，未记的只说「另有 N 份……补记之后再选」；换供应商时清掉各行选的别家成本；去掉票 11 那句「这里不按供应商筛」；两种新答复说人话。
10. **演示脚本**：主链记下预期成本的供应商（没记就失败），结算段以那一家登账单。

### 关键取舍与理由

- **问 PC、不由载荷带入**：形成没有外部载荷，命令出自请求，请求上只有协议；另带一份就是第二处权威，还得再问一次才可信。已发布协议正文不可覆盖，问一次就准。
- **不在迁移里回填**：要读 `party_commercial` 的表，跨上下文读表；改为经同一只适配器的补记口，问到的与形成时会问到的是同一家。
- **旧行答「未记」而不放过**：放过就把口子留给旧行；「未记」与「不符」恢复动作不同（补记 / 改录入），分两格。
- **纠错版本随协议重问、不把供应商列进不动的身份**：供应商是协议的属性，协议本就整组重述；旧供应商点名被纠正那一版本来就答不是当前版。
- **供应商排在币种之前**：别家的成本谈不上换算，先答「换算未配置」会把录错说成租户没登参数。

### 验证（2026-11-02，本机 PG `127.0.0.1:55432`，只记 `S`）

- `go build ./...`、`go vet ./...` 退 0；改过的 Go 文件 `gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/settlementaccounting/... ./internal/partycommercial/... ./internal/architecture/... ./cmd/... ./migrations/...` 40 个包 ok、0 FAIL（含 PG，在并入票 13 之后的 tip 上跑）。新用例 `-v` 抽查全 PASS：PC 两条真库、SA 消费侧适配器三条、领域三条、编排四条、SA 真库三条（存取与旧行、补记只动点名租户且已记不改且事务外拒、0055 施加在有成本行的库上）、补记口真库一条、parcel-api 接收口一条、派发进程形成经 PC 真读口记下供应商。
- `scripts/ci/generated-artifacts-pr.sh`：ADR 索引、AT 盘点计数、机制清点过期（留推送方），无红。
- 前端：`npx tsc -b` 无输出，`pnpm test` 1127 pass / 0 fail，`npx vite build` 退 0。
- **种子**：`demo-run/up.sh docnum14_demo`（`IDP_PARCEL_DEMO_API_ADDR=:19104`）退 0（含 0055）；`10-main-chain.sh` 形成的预期成本读口带 `supplier=SYN-PARTY-CARRIER-TRUNK-01`（派发进程经 PC 真读口问得）；`30-settlement.sh` 以那一家登账单、审核成应付 `AP-202611-00001`。
- **浏览器**（前端 :5204 连 :19104，库里另直写一版 LM 的 3000 CNY 成本与一版没记供应商的旧行）：登记表单没选供应商不列任何预期成本，提示「另有 1 份……还没记」；选 TRUNK 只列 TRUNK 的 8500；第 1 行选上后换成 LM，已选被清空，只列 LM 的 3000；跑补记口答「补记 1 份 SYN-BUYSC-OLD-DEMO-1 → SYN-PARTY-CARRIER-LM-01」（退 0，重跑答没有待补记的），再开表单 LM 下多出 1200 那一份、提示句消失。页面无报错、核验脚本没发写请求。
- **接口**：TRUNK 的账单点名 LM 的成本答 `200 EXPECTED_COST_SUPPLIER_MISMATCH`，带 `expectedCostSupplier {line L1, version SYN-BUYSC-LM-DEMO-1, supplier SYN-PARTY-CARRIER-LM-01}`，接收册没落行；LM 自己的账单点名同一份答 `201 BILL_RECEIVED`。验完停了 vite、parcel-api、parcel-dispatch，`down.sh --drop-db` 删了库。

### 留给协调方

1. **越权风险点待 SA owner 复核**（ADR-0358）：已接收的跨供应商匹配不重判；补记靠人跑；PC 不认的旧行永远答未记；协议后继换供应商时成本跟着换（PC 是否该允许同一协议对象换供应商归 PC owner）；形成多一个跨上下文依赖；逐行裁决仍由人给；表单只列读口那一页。
2. **共享面**：ADR-0340、ADR-0200 的 Status 行与 Links；SA `CONTEXT.md`、GLOSSARY、UC-SA-002、UC-SA-004（与票 13 修订合过一次冲突：AT 表与交接表、CONTEXT 一条）；`.scratch/at-coverage-inventory.md`；接收答复结构 `receptionResponse`（与票 13 的 `registeredSupplier` 合过）；管理台 `SupplierBillReceptionDialog.tsx`、`api.ts`、`supplier-bills.ts`（与票 13 的页外原账单检索合过）；迁移号 `settlement_accounting/0055`、ADR 号 0358、AT `AT-SA-236`、`AT-SA-237`。
3. **推送方重生成**：ADR 索引（ADR-0358 一行与 ADR-0340、ADR-0200 注记）、AT 盘点总览计数（SA +2）、机制清点。
4. **已有库上线**：迁移之后旧预期成本读作未记、匹配答未记，要对每个租户跑一次 `parcel-settlement-register expected-cost-supplier-backfill`；`demo_live` 这类有数据的库同此。
5. **并 main**：票 11、13 进 main 后照通知 `git merge origin/main`。

## 评审修订（2026-11-02，作者；评审钉 `082162615`，两轴阻断为零，非阻断三条都修，追加提交）

1. `SupplierExpectedCosts.RecordExpectedCostSupplier` 头注去掉「全仓唯一改写预期成本行的一处」那句跨文件计数，改成不计数的理由：预期成本行按版本只增不改，这里只把一格从空写到有，成本本身不动，所以不另起一版。
2. 管理台 `receptionAnswerText` 的供应商不符分支改用 `supplierName` 说那一版供应商的名称（登记表单传入的那只取不到名称时退回标识），测试改断言名称，另加一条退回标识。
3. PC 库里存的供应商立不成往来单位标识时，PC 读口改报具名错误 `ports.ErrSupplierAgreementSupplierMalformed`（与读不动分开），结算的消费侧适配器把它同指称歧义一样译成 `ErrSupplierAgreementSupplierUntranslatable`：形成照旧上抛落 publish_failed、不重投；补记口把那一版记为 `AGREEMENT_SUPPLIER_UNTRANSLATABLE`、留空列出、退 2，其余照写，不再整笔回滚（ADR-0358 决定三「不能为它们挡住其余的补记」）。新用例：PC 真库 `TestAMalformedStoredSupplierIsReportedAsSuch`（库上 `btrim` 只削空格，制表符过得了检查却立不成标识）、适配器 `TestAMalformedStoredSupplierIsDeterministic`，补记口真库用例加一版存制表符供应商的协议、断言它留空列出而同租户补得上的那份照写。AT 盘点 `AT-SA-237` 行补这两条用例名。ADR-0358 的「译不过来」已涵盖这一种，正文不改。

验证（修订后的 tip）：`go build ./...`、`go vet ./...` 退 0，改过的 Go 文件 `gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/settlementaccounting/... ./internal/partycommercial/... ./internal/architecture/... ./cmd/... ./migrations/...` 40 个包 ok、0 FAIL（含 PG），新用例 `-v` 抽查 PASS；前端 `npx tsc -b` 无输出、`pnpm test` 1127 pass / 0 fail、`npx vite build` 退 0。
