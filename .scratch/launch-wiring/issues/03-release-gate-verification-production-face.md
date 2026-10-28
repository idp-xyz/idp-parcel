# 03 CC 放行门禁核对编排无装配

Category: enhancement
Status: resolved——2026-10-28 进 main（`2e24f5941`，launch-wiring 01–04 同批；全量 160 ok / 0 FAIL，种子退 0），见文末「进 main 记录」。此前 in-progress——2026-10-28 作者完工，待隔离评审与进 main，分支 claude-lw-cc（代码最后一笔 c5de67335）。此前：in-progress——2026-10-28 会话 tops-20（idp-parcel-91）立并认领（用户「现在就接线」），分支 claude-lw-cc
Blocked by: 无

## 做什么

把 `VerifyReleaseGateHandler` 接成生产入口（触发方照 UC-CC-009），折不出结论答 `UNDECIDED`+`GATE_CONDITION_VIEW_UNAVAILABLE`（code/34）；生产装配真库用例。

## 完成判据

- 生产路径能做放行门禁核对并落册、发意图；折不出结论时不落册（真库用例）。
- 演示动线里「立案 → 放行结果 → 交出门禁从未决变放行」这一格能走到（psb/05「预走」记的收口重走顺序），或写明还差什么。


## 完成记录（2026-10-28，作者子代理，分支 claude-lw-cc）

起点 origin/main `2d0193336`（本票叠在 01、02 之后）；代码一笔 `c5de67335`。

### 改了什么
- 触发方照 UC-CC-009「门禁核对……分别授权」与「人重核与门禁核对的发布失败照答已形成并带续办引用」选**运营命令口**，不是派发消费者（`cmd/parcel-dispatch` 不动）。
- accessidentity：运营决定加 `RELEASE_GATE_VERIFICATION`；迁移 `access_identity/0033`（全集 = 0030 加 `origin/claude-lw-no` 在 0031 加的 `NODE_COLLABORATION_ACCEPTANCE`、02 的 `CUSTOMS_CASE_CLOSURE`、再加本词）。取号：origin/main 到 0029，NO 分支取了 0031，本组 02 因此改取 0032，本票取 0033。
- customshttp（新文件 `release_gate_verification_endpoint.go`）：`POST /customs-release-gate-verifications`，Intake 挂在既有 `OperatorDecisionIntake` 上，租户取认证信封，载荷只有申报范围、动作与监管边界且封闭（动作不在封闭四值答 400）；响应带结论、前置条件、「税费付款」读数三态原值，类型上没有放行字段；未决原因（含 `GATE_CONDITION_VIEW_UNAVAILABLE`）原样过线。
- parcel-api（新文件）：`endpoints_release_gate_verification.go`、`assemble_release_gate_verification.go`、未接线占位。缝全接真：门禁目录与「税费付款」规则行同一只读适配器、付款核对当前版读口（与动作准入判断接同一批）、`GateVerifications`、`OutboxGateVerificationHandoff`、系统时钟；事务壳复用 `customsDecisionTransaction`，未决整笔回滚、不落册。
- 演示种子：access 32/33、governance 68、步骤 486（种子号取号同理避开 NO 分支的 28/29、65/66、484）。

### 代决与越权风险点
1. 授予与建案、关案分开；UC-CC-009 要求税费结果接收、付款核对、门禁核对与外部放行接收分别授权。
2. 「折不出结论答 `UNDECIDED`+`GATE_CONDITION_VIEW_UNAVAILABLE`」靠 code/34 的应用层已有用例证（`verify_release_gate_test.go`）；生产库上很难造出一条折不出结论的合法认定，所以装配用例没单独再造这一格，只证未决不落册这一般性质（目录未登记、规则未登记两个停点）。
3. **新发现，可能挡上线**：门禁目录里「税费付款」那一道的规则行（`RegisterDutyPaymentGateRuleCommand`）**没有任何生产登记面**——应用编排与 postgres 写口有，但 `cmd/parcel-customs-register`、registrationjson、HTTP 登记口都没有它，演示种子也没种。后果：生产上任何门禁核对（以及同读法的动作准入判断）在租户登规则之前都停在 `DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED`，租户没有办法登它。规则取值是租户取值，但「登记面」是机制，属本产品。本票没做（不在票面，且要加登记 CLI、registrationjson 译装、可能还有 HTTP 登记口），见「留给协调方」。
4. 未立新 ADR、未立新 AT 号（测试 `Covers:` 引 UC-CC-009）。

### 判据对用例
- 「生产路径能做放行门禁核对并落册、发意图」：真库用例 `TestAReleaseGateVerificationIsRecordedOrLeftUndecidedThroughTheProductionFace`——目录与规则（登成「不构成前置条件」）齐、认定全满足答 201 `RECORDED` 结论 `MET`，核对记录与核对意图同笔入库；重放答 `EXISTING_RESULT`，记录仍一行、意图不多入。
- 「折不出结论时不落册」：同用例——目录未登记答 `GATE_CATALOG_NOT_CONFIGURED`、规则未登记答 `DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED`，两者都是 200 `UNDECIDED`，核对册零行、核对意图不增；只持建案授予答 403。
- 「演示动线里『立案 → 放行结果 → 交出门禁从未决变放行』这一格能走到，或写明还差什么」：**没走到，还差**：(a) 演示租户的「税费付款」规则行没有种、也没有登记面（上面第 3 条），所以种子里 250 节登的门禁目录（`SYN-SCOPE-01` 的 `CROSS_CUSTOMS_MOVEMENT`）经本口核对只会答 `DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED`；(b) 「交出门禁」的未决/放行读数来自动作准入判断（`JudgeActionAdmissionHandler`）读门禁目录与放行结果，不读本口落的核对册，所以本口接上不会让交出门禁读数自己变；放行结果要经 `POST /customs/external-results`（`RELEASE_RESULT`，先有案件与提交版本）进来；(c) 没有起整套服务重走动线，这一格只在装配层真库用例上证到本口，没有端到端演练。

### 自验
`go build ./...`；`go vet` customscompliance、accessidentity、cmd/parcel-api；`gofmt -l` 无输出；带 PG DSN `go test -count=1 -p 1` 跑 `./internal/customscompliance/...`、`./internal/accessidentity/...`、`./internal/architecture/...`、`./migrations/...`、`./cmd/parcel-api/...` 全绿；种子灌一次性库退 0 后删库；`at-coverage-counts -check` 无输出。

### 留给协调方
- 第 3 条的缺口（「税费付款」规则行无登记面）：它让 03 在生产上建好了口却到不了 `RECORDED`，请裁是否立票；建议比较小（登记 CLI + registrationjson 译装 + 演示租户 S 级种子，规则取值留给租户或用「不构成前置条件」作演示租户值）。
- 迁移号：本组占 access_identity 0030（01）、0032（02）、0033（03），0031 留给 `claude-lw-no`；种子号占 access 26/27、30/31、32/33，governance 63/64、67、68，步骤 483、485、486。进 main 时按实际次序再核一遍；`operation_decision.go` 的 `decisionKinds` 与各份 CHECK 并集会与 NO 分支文本冲突，按并集合。

## 评审记录（2026-10-28，隔离子代理，钉 `f977c587c`，01–03 同一评审）

两轴无阻断。两种「未配置」停点不落册不发意图；`MET` 落册；重放；响应无放行字段。「交出门禁读数读动作准入、不读核对册」与 UC-CC-009/013 不相抵（共用 `assessReleaseGate`）。非阻断：核对记录不带请求人（核对是系统折出的判断，不算违 ADR-0303）。作者报告的缺口——「税费付款」规则行无生产登记面，租户上线当天门禁核对与动作准入都停在 `DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED`、无绕法——评审核实属实、挡上线，转 [06](./06-duty-payment-gate-rule-registration-face.md)。本票不因此留开：接线本身已成。

## 进 main 记录（2026-10-28，推送方 tops-20 / idp-parcel-91）

- 与 launch-wiring 01、02、04 同批：隔离树上先 cherry-pick `claude-lw-cc`（tip `f977c587c`）、再 `claude-lw-no`（tip `272f98c15`）到 main `322b5695c`；`internal/accessidentity/operation_decision.go` 的 `decisionKinds` 文本冲突按并集解。access_identity 迁移 0030（01）、0031（04）、0032（02）、0033（03）依序施加，0033 的 CHECK 为全集（评审逐词核过）。
- 重生成 ADR 索引、AT 盘点计数与机制清点，有变化的各成一笔；两项 `-check` 退 0。
- 全量 `scripts/test-full.sh`：160 ok / 0 FAIL。种子灌一次性库退 0，用后删库。
- 推确切 SHA `2e24f5941` 到 main。
