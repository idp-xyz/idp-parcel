# 140 管理台手工采用外部资金事实与监管凭证：操作者渠道入口

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg140。此前 in-progress——2026-10-21 认领；ready-for-agent——2026-10-20 代决复核立（上线前必修），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 R1；ADR-0149 决定一、三

## 现象与修法

现象、证据与修法的完整写法以复核报告 R1 一条为准（单一权威），此处只记要点：

代决「是」：操作者渠道另开入口，按决定种类单独授予、记提交操作者，与集成客户端族并存；现管理台三张登记签改走它。

## 完成记录（2026-10-21，分支 claude-dcg140）

授权：用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」与常设指令「你作为业务和系统专家自决……解决所有的问题」。决定落在 [ADR-0236](../../../docs/adr/0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md)（代决，待 owner 复核）。

### 做了什么

- **接入身份**：运营决定加两种——`EXTERNAL_FUNDS_FACT_ADOPTION`（资金事实首版与更正同一种）、`REGULATORY_CREDENTIAL_REGISTRATION`；迁移 `access_identity/0021` 重建决定种类 CHECK。
- **SA**：资金事实版本行加操作证据列（迁移 `settlement_accounting/0041`）；采用与更正命令带 `OperationEvidence`，落在那一版上，不进内容摘要与重放比对，重放不顶替；`settlementhttp.OperatorDecisionIntake` 与 `adapters/accessidentity.OperatorDecisionAuthenticator`（按决定种类铸信封、判准入）；`/settlement-funds-applications` 透出链头那一版的 `operationEvidence`。
- **CC**：凭证行加操作证据列（迁移 `customs_compliance/0026`）；`ports.CredentialRegistry.RegisterCredential` 多收一参操作证据，登记命令带 `OperationEvidence`；既有 `customshttp.OperatorDecisionIntake` 加凭证登记一口；`/customs-credentials` 透出 `operationEvidence`。
- **parcel-api**：新特性文件 `endpoints_manual_external_facts.go` 登三行——`/settlement-external-funds-fact-manual-registrations`、`/settlement-external-funds-fact-correction-manual-registrations`、`/customs-regulatory-credential-manual-registrations`，挂 `decisionFace`，编排取集成客户端那边同一套构造；`assemble_operator_decisions.go` 的 `operatorDecisionIntakes` 加 `settlement` 一格（只加不改）。集成客户端族原三口一行未改。
- **管理台**：收付款核销页「采用资金事实」签（首版、更正）与关务案件页凭证登记签改发手工口，形状提示去掉 `tenantId`、补上要持哪一种授予；两本册读签加「采用人」「登记人」栏（缺席答「未记（非手工…）」）；两页错误码说明补 `OUTSIDE_ADMISSION_SCOPE`、`IDENTITY_DEPENDENCY_UNAVAILABLE`。
- **演示种子**：配置员加两种授予（`data/access/02`）、准入对照两行（`03`）、权威区间两份（`data/governance/45`、`46`，新步 `steps/470-manual-external-facts.sh`）；README 授予表与说明段、演示动线文档补记。
- **文档**：ADR-0236；ADR-0149、ADR-0151 的 Status 行与 Links 加部分停用前向指针（正文未动，`docs/adr/README.md` 未手改，待重生成）；SA CONTEXT（唯一采用口那条补人工采用与操作证据）、CC CONTEXT（监管凭证一节加一条）；UC-SA-005 加 `AT-SA-200`、`201`，UC-CC-003 加 `AT-CC-446`、`447`；AT 盘点 ① 加四行（未改总览计数）。

### 代决与越权风险点

代决「是」照复核报告 R1 落地；票面没写、由本会话自决的细节与各自的越权风险点（逐条展开见 ADR-0236「越权风险点」）：

1. 路径另开带 `manual` 的三口，不把既有三口改成双族（否决理由见 ADR-0236 候选乙）。越权风险点：owner 答否 R1 时撤三张签与三口、两种授予，两列操作证据留着全为 NULL。
2. 资金事实首版与更正同一种授予。越权风险点：要求更正单独授予或四眼时加一种决定。
3. 更正不绑原采用族——财务可手工更正银行接口送来的事实。越权风险点：要绑就先做票 157。
4. 操作证据只记首次那一位，重放的第二人不留痕（只在接入日志）。
5. 集成客户端与受控 CLI 两路不记提交方（NULL），两族留痕不对称 → 票 157。
6. 管理台仍以 JSON 录入 → 票 156。
7. 凭证挂运营决定而不挂登记写面（凭证是外部监管事实的一版，不是租户配置）。
8. 手工口与集成客户端口的准入是两套坐标（`OPERATION_DECISION`+决定种类 / `EXTERNAL_FACT`+事实类型），阶段治理要为同一类事实登两份区间。
9. 读面原样透出 `OPERATOR/<发行方>#<sub>`，不译姓名（产品不存姓名）。
10. 部分停用 ADR-0149 决定一「外部结果与外部资金事实（……）的提交者是外部系统」一句与决定三对两类的独占、ADR-0151 决定四「外部资金事实更正随原事实归集成客户端族」一句，适用场景只限三手工口。

### 用例

| AT | 证据 |
|---|---|
| `AT-SA-200` 手工采用首版与更正、各版记提交人、重放不翻倍不顶替 | 编排、持久化、Intake、读面各一条；`cmd/parcel-api/assemble_manual_external_facts_test.go::TestManualExternalFactsAdoptThroughTheOperatorFacesAndRecordTheSubmitter`（真库，生产装配） |
| `AT-SA-201` 未授予答 `OPERATOR_NOT_GRANTED` 不写；集成客户端族原口照旧、不记提交人 | 接入身份真库、认证、Intake、端点；`::TestEachManualFactFaceOpensOnlyForItsOwnDecisionKind`、`::TestTheIntegrationClientFacesStillWorkAlongsideAndRecordNoOperator` |
| `AT-CC-446` 手工登记凭证、记登记人、重登答已存在、异内容答冲突 | 编排、持久化、Intake、读面；同一真库装配用例 |
| `AT-CC-447` 未授予（含只持资金事实采用或关务三种的）答 `OPERATOR_NOT_GRANTED`；集成客户端原口照旧 | 接入身份真库、认证、Intake；同上两条装配用例 |

逐条文件::函数见 AT 盘点 ①。两种决定互相用不了由接入身份真库用例与装配用例两处证。

### 验证

- `scripts/test-full.sh` 跑改动包及其反向依赖（含全部反向依赖的 cmd 包）、`./internal/architecture/...`、`./migrations/...`，共 70 包：65 ok、5 无测试文件、0 FAIL（清单在会话 scratchpad `dcg140/test-packages.txt`）。
- `go build ./...`、`go vet ./...`、`gofmt -l` 干净；Go 各笔提交逐个检出 `go build ./... && go vet ./...` 都过。
- admin-web 三道门（Node 22）：`tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 598/598；`pnpm exec vite build` 退 0。
- 一次性库（55432 上 `dcg140_seed`，用完已 `DROP … WITH (FORCE)`）按 `seed.sh` 灌入退 0；起 `parcel-api`（:19140，操作者与集成客户端两只演示 Dex），配置员令牌：采用 201 → 重放 200 `EXISTING_FUNDS_FACT` → 更正 201；凭证 201 → 重登 200 `EXISTING`；查阅员两类口都 403 `OPERATOR_NOT_GRANTED`；操作者令牌打集成客户端口 401、银行客户端令牌打手工口 401；合成银行、合成报关服务商经原两口照旧 201；两本册读面上手工那几版带 `OPERATOR/http://127.0.0.1:5556/dex#<配置员 sub>`，其余缺席。只记 `S`。管理台没有在浏览器里走（三张签发的就是这几口与同一枚令牌）。

### 没做与去向

- 逐字段表单 → 票 [156](./156-admin-web-manual-external-fact-forms-replace-json-entry.md)；集成客户端一路记提交方 → 票 [157](./157-integration-client-adoptions-record-the-submitting-client.md)。
- 映射、核销与核销撤销仍只有受控 CLI，本票不开在线口；职责分离不强制，归角色模型（票 product-strategy-boundary/07）。
- **迁移号**：`access_identity/0021`、`customs_compliance/0026` 照号段；**`settlement_accounting/0041` 是自取**——号段没给 SA 迁移号，而记提交人非落在 SA 版本行不可（main 上 SA 最大 0040）。进 main 时协调方按落地顺序改号；在途票 14（SA）若也取 0041 会撞号。
- **0021 的 CHECK 全集**以 main 的 0018 为底加本份两词；并行票占的 0019、0020 若也重建这条 CHECK，最后落地的那一份要取并集（读码所见：票 70 分支上的 0020 加 `DELIVERY_DISPATCH_REASK`，且以 0016 为底，缺 0018 的三个关务词）。
- 不重生成机制清点、ADR 索引与 AT 计数。

## 评审记录（2026-10-21，隔离评审：无阻断项；协调方转作者修订，本轮未变基）

| # | 评审意见 | 去向 |
|---|---|---|
| 1 | SA 资金事实采用与更正撞键（`FundsFactAlreadyAdopted`）读回赢家、不比内容摘要，在线口答复不带记录，输家分不出；`ExternalFundsFacts.Save` 头注写成「有意」与票 125 相抵 | ✅ 票 [125](./125-concurrent-same-source-results-with-different-content-answer-existing-result.md) 加同形追记：三种情形 (a)(b)(c)、两族并发是最现实的一格、修法同票 125；点明 CC 凭证那一路落库后读回比 CCC-1、答 `CONTENT_CONFLICT`，无此问题。`ExternalFundsFacts.Save` 头注、`ports.FundsFactAlreadyAdopted` 注释与 `CorrectFact` 撞键分支注释改指票 125 的缺陷说法（只动注释，修法本身归票 125） |
| 2 | ADR-0236 决定四与越权风险点 3 要正面回应 UC-SA-005「结算人员不能更改银行事实」，并写明两族顺序到达的答复与续办 | ✅ 决定四引原文并回应：手工更正采用的是新来源版本、原版本保留，但职责分离今天不强制、持采用授予的人能更正银行接口送来的金额，写成越权风险点 3，去向票 product-strategy-boundary/07；顺序到达两格写明——手工先录错、银行随后推真值答 `FUNDS_FACT_CONFLICT`，手工先更正、银行随后推更正答 `SOURCE_NOT_ACCEPTED`，外部系统一侧都转人工；并发那一格指向票 125。Status 行索引注记随改 |
| 3 | SA CONTEXT 写了「首版与更正同一种授予」，那是授予粒度 | ✅ 改成「由持相应授予的租户财务人员手工采用」，粒度只在 ADR-0236 决定一 |
| 4 | 关务 `snapshotHint` 给凭证一类仍带「不逐字段建表单，因为…随 PAR-INT-01 提供」 | ✅ 凭证一类改用单拼的 `manualEntryHint`：与 CLI、集成客户端同形，「本页暂以 JSON 录入，逐字段表单尚未提供」；其余各类不动。收付款页那份提示的头注同步去掉「不逐字段建表单」的旧理由 |
| 5 | 两页新加的 `IDENTITY_DEPENDENCY_UNAVAILABLE` 说明「本次什么都没登记」与面板 noAnswer 格后接的「登记与否未知」相抵 | ✅ 删掉那半句，说明改成「接入身份依赖读不动（发行方公钥集、操作者册或准入范围），可稍后重试」；面板不动 |
| 6 | `CustomsCasesPage.tsx` 头注「票 demo-chain-gaps/140 起接手工口…」是变更叙述 | ✅ 改成「接手工口 POST /customs-regulatory-credential-manual-registrations，ADR-0236」 |
| 7 | 「采用人」列显示的是链头那一版的提交方 | ✅ 表头改「采用人（链头那一版）」，注释写明首版与各次更正可能出自不同的人 |
| 8 | SA 手工口 `readSinglePayload(request.Body)` 无请求体上限 | ✅ 加 `io.LimitReader(…, 1 MiB+1)`（与 CC 手工口同值），补用例 `TestTheOperatorDecisionIntakeRefusesAnOversizedBody`（填充放在对象里，去掉上限时这条用例实测转红）；CC 凭证手工口顺带补同形用例 `TestTheCredentialFaceRefusesAnOversizedBody`。集成客户端原口同病，另立票 [159](./159-integration-client-registration-faces-cap-the-request-body.md)，本票不改 |

未采纳：无。

验证（修订后）：`scripts/test-full.sh ./cmd/parcel-api/... ./cmd/parcel-customs-register/... ./cmd/parcel-settlement-register/... ./internal/accessidentity/... ./internal/settlementaccounting/... ./internal/customscompliance/... ./internal/architecture/... ./migrations/...`：33 ok、2 无测试文件、0 FAIL；`go build ./...`、`go vet ./...`、`gofmt -l` 干净，修订里两笔 Go 提交逐个检出 build/vet 都过；admin-web 三道门：tsc 退 0、run-tests 598/598、vite build 退 0。

待变基（协调方通知后办）：票 70 先进 main，占 `access_identity/0019`（加 `DELIVERY_DISPATCH_REASK`）、种子 `data/governance/45` 与 `data/access` 的改动。届时本票的 access_identity 迁移改成 main 上下一个空号、CHECK 取并集；种子数据号改空号；`settlement_accounting/0041` 按 main 定号（票 14 也取了 0041、0042）。

