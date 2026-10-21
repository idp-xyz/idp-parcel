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

