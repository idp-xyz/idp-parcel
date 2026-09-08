# 人工复核谓词：「要不要」已由接单规则正文答，「谁有权」这一问的消费方（复核完成授权）尚无端口

Category: enhancement
Status: resolved——2026-09-08 17:3x，MCP-2（task 449f4e64；分支 `mcp2-wbr04` 基 origin/main `62bf6504`，已验 tip `8e52f413`）：判据 2、3 落地——PS 侧 `ports.ManualReviewAuthorizer`（三值）+ `ManualReviewAuthorizationAdapter` 调 `AdjudicateCommercialAuthorizationHandler` 带 `ManualReviewAction`，`CompleteManualReviewHandler` 先问授权、命令去掉 `Authority`，`cmd/parcel-api` 装配接真；同笔删 `ManualReviewRequirementFor` 与其用例、基线条目剪掉（成因第一种，两法同得 6→5 钉 `f4e9a9e2`）。完成记录见 Comments 末条；待非作者评审与推送方重放，进 main 记录由推送方补。此前 in-progress——2026-09-08 16:5x：派单第 ① 步消费点已量（`CompleteManualReviewHandler.Handle` 一处，PS 不变式不动）。此前 ready-for-agent——2026-09-08 11:0x，MCP-1 代裁（owner 授权自决口径，见 Comments 倒数第二条）：取「谁有权」形态、不拆两问，**不改名而是并入 `Authorize`**——PS 侧复核授权端口与 `ActiveRejectionAuthorizer` 同形（三值），适配器调 UC-PC-003 既有裁定用例带 `ManualReviewAction`；`ManualReviewRequirementFor` 及只为它写的用例在 PS 端口接真那一笔删去（基线条目同笔消，成因第一种）。判据 1 已裁并记进 UC-PC-003 第四项；判据 2、3 的 PC 半边与 PS 侧端口**同笔**落地、单独改不了，PS 侧另派 MCP-2，故转 ready-for-agent 等派。不立新 ADR。此前 blocked：2026-09-08，MCP-6（task 5f716c71，用户 02:0x 自 MCP-3 改派；分支 `mcp6-wbr03-05` 基 `4524cfd4`）：完成判据 4 的理由行已补进基线（条目上方六行，族界那句照留在后面并接下票 05 的括注）；判据 1 那格是 UC-PC-003 第四项的语义改口，按派单纪律停下报 MCP-1 裁。此前 draft：只读取证（MCP-6，锚 `2efef58e`），PC 地盘归 MCP-3；交 MCP-1 派
Blocked by: 无（UC-PC-003 第四项已由 MCP-1 2026-09-08 裁；PC 半边与 PS 侧端口同笔落地，PS 侧另派 MCP-2，MCP-6 不碰 PS 地盘）

## 条目

`internal/partycommercial/domain ManualReviewRequirementFor`（`authority_grant.go`）。基线理由行：「返回 bool 是个谓词……留着不排除是有意的」——讲的是族界，不是活性。

## 它是什么

`ManualReviewRequirementFor(grants, scope, at) bool`：授权授予册里该范围此刻有生效的 `ManualReviewAction` 授予即答真；注释「回答某个范围是否要求人工复核。沉默意味着不要求」。`ManualReviewAction` 是 `AuthorizedAction` 封闭二值之一（另一格主动拒绝），登记口已接（`adapters/postgres/authorization_grant.go` 能读回这一格）。

## 「要不要」今天走的是另一条路，且已接通

接单规则正文 `AcceptanceRuleContent.ManualReview()`（`ManualReviewDirective`，UC-PC-001 发布时随规则包声明）→ PS `CommercialBasisSnapshot.ManualReviewPolicy()` → `parcelshipment/application/form_acceptance_decision.go`（未声明即 `ManualReviewPolicyNotDeclared` 未决，不默认）。CONTEXT 硬句「接单规则包必须明确……哪些条件确实需要人工业务复核」落的正是这条。所以按函数今天的注释读，它是那条路的**平行第二写法**——两处各答一遍「要不要」，而 PS 只读其中一处。

## UC-PC-003 已经点破这件事

「四、人工复核那一格的措辞与分工对不齐。ADR-0042 明确把既有的 `ManualReviewRequirementFor(grants…)` 归为授权治理（**谁有权**复核），而该函数今天的名字与注释写的是『回答某个范围是否要求人工复核』（**要不要**）。两种读法在同一个函数上并存，正是 ADR-0042 要拦的并格。……在人工复核经本端口对外暴露之前必须先定。」首切只覆盖主动拒绝，人工复核那格不在首切内。

## 该有的调用方（按 ADR-0042 的读法）

UC-PC-003 的第二切——复核**完成**的授权裁定。PS `application/complete_manual_review.go` 的 `ManualReviewCompletion` 带三项引用（复核授权、复核人、证据），今天由 Intake 从认证结果整组注入（`adapters/http/unconfigured_intake.go` 自注「复核人与授权引用整组来自认证结果，采信自报的……不算」），**没有人向 PC 问「这个角色在这个范围此刻有没有复核授权」**。对应 PS 侧该有与 `ActiveRejectionAuthorizer` 同形的复核授权端口（三值答复），PC 侧经 UC-PC-003 的裁定用例答它——而那个用例读授权授予册时调的就是这个谓词（改名后）。

## 三分

**支路未接**，前置一格命名 / 分工裁决。不是死码：「谁有权复核」这一问在 UC-PC-003 有步骤、在授予册有数据、在 PS 有消费点（复核完成的授权引用），只是三处没连起来。删掉它等于把「谁有权」这一问也删了。

## 能不能归到已认可的留待

不能：缺的是端口与用例，不是实例值。

## 完成判据（落地那笔连理由行一起改；MCP-1 2026-09-07 裁）

1. UC-PC-003「四、人工复核那一格」有裁决：改名为「谁有权」形态（或拆成两问），函数名、注释与 ADR-0042 的归类一致。
2. PS 侧有与 `ActiveRejectionAuthorizer` 同形的复核授权端口（三值答复），PC 侧 UC-PC-003 第二切的裁定用例读授权授予册时真调它（改名后）；`complete_manual_review.go` 的复核授权引用不再只靠 Intake 整组注入。
3. 剪基线行：先按头注三分成因（改名后全仓只此一处声明才是第二种；改名那笔要同步改条目名，否则门禁按旧名找不到声明会红），在自己那笔的干净检出上两法同得记数、钉 SHA。**同一段 PC 组注释「下面两条落在网里但不是工厂……」讲的是族界不是活性**，票 05 删掉 `ValidateBeforeDecision` 后那句要改成只指本条。
4. **若 1–2 之前先要改理由行**，条目上方加（族界那句可留在后面）：

   > 人工复核授权谓词。按 ADR-0042 它答的是「谁有权复核」，不是「要不要复核」——后者已由接单规则正文 `ManualReviewDirective` 经 PS 接通，别把它读成那条路的第二写法。**调用方是 UC-PC-003 第二切「复核完成授权」的裁定用例**（PS `complete_manual_review.go` 的复核授权引用今天由 Intake 整组注入，没人向 PC 问）；那一层缺端口与用例，且前置一格命名 / 分工裁决（UC-PC-003 第四项）。裁决改名那笔同步改这句与条目名，用例真调它那天出名单（wiring-baseline-remainder/04）。

## 边界

本票不改代码、不改基线。裁决落地（改名 / 拆分）时理由行同步改写；剪基线行的时刻是 UC-PC-003 第二切的裁定用例真调它那一笔。
（立票时的边界；落地笔见 Comments。）

## Comments

- 2026-09-08 02:3x · MCP-6（task 5f716c71；分支 `mcp6-wbr03-05` 基 `4524cfd4`）：**补理由行，停下报 MCP-1。** 基线
  `ManualReviewRequirementFor` 条目上方按完成判据 4 加了理由行（ADR-0042 的读法是「谁有权」、「要不要」已由 `ManualReviewDirective`
  经 PS 接通、调用方是 UC-PC-003 第二切的裁定用例、缺端口与用例、前置一格命名 / 分工裁决），族界那句改成只指本条并照留在后面
  （票 05 的括注接在其后），名单一行未动。**现状复核**（锚 `4524cfd4`）：函数注释仍写「回答某个范围是否要求人工复核。沉默意味着
  不要求」——与 ADR-0042 的归类相反，与 UC-PC-003 第四项点破的并格一字不差；PS `application/complete_manual_review.go` 的
  `ManualReviewCompletion` 三项引用仍由 Intake 整组注入，PS 侧没有与 `ActiveRejectionAuthorizer` 同形的复核授权端口。**为何不
  顺手改名**：改名 / 拆两问是 UC-PC-003 的措辞与分工裁决（改的是「这个函数答哪一问」，即语义），派单纪律写明这类改口先停下报
  MCP-1；且判据 2 的 PS 侧端口在 PS 地盘（MCP-2），本单不碰。**待 MCP-1**：裁 UC-PC-003 第四项（改名为「谁有权」形态，建议名
  `ManualReviewAuthorityFor` / 或拆成 `ManualReviewAuthorized` + 保留「要不要」交给规则正文），裁后由 PC 侧改名 + 基线条目名同笔改
  （门禁按名找声明，不同笔会红）+ UC-PC-003 第二切裁定用例，PS 侧端口另派。
- 2026-09-08 11:0x · MCP-1 代裁（owner 授权自决口径；取证钉 `96558acd`；由 MCP-6 落票面）：**取「谁有权」形态、不拆两问，且
  不改名而是并入 `Authorize`。** 取证：`domain.Authorize(grants, delegations, request)` 已是动作无关的「谁有权」裁定，
  `AuthorizedAction` 里 `ManualReviewAction` 与主动拒绝同走一条路（`permits` 按动作 + 法人 + 等级 + 范围 + 时刻，三值：命中 /
  `ErrNotAuthorized` / `ErrAuthorityRulesNotConfigured`，ADR-0029），`AdjudicateCommercialAuthorizationHandler` 就是 UC-PC-003 的
  裁定用例。而 `ManualReviewRequirementFor(grants, scope, at) bool` 只按范围 + 时刻、无法人无等级无三值：答不了「谁有权」（无主体），
  也不该答「要不要」（单一权威在接单规则正文 `ManualReviewDirective`，CONTEXT 硬句）——改名后仍是 `Authorize` 的无主体退化副本，
  第二写法就是 ADR-0042 要拦的并格。**落地**：PS 侧复核授权端口与 `ActiveRejectionAuthorizer` 同形（三值），适配器调同一裁定用例
  带 `ManualReviewAction`；`ManualReviewRequirementFor` 及只为它写的用例在 PS 端口接真那一笔删去（基线条目同笔消，成因记「第一种：
  并入 Authorize」）。**判据按裁决读**：判据 2 的「PC 裁定用例真调它（改名后）」改为「适配器调既有 `AdjudicateCommercialAuthorizationHandler`
  带 `ManualReviewAction`」；判据 3 的三分成因从第二种改为第一种（删）。**不立新 ADR**：ADR-0042 已定归类，这是应用不是新取舍；
  UC-PC-003 第四项那段记裁决（改行为→改 UC，不动 ADR 历史）。PS 侧端口在 PS 地盘，另派 MCP-2，MCP-6 不碰。Status
  blocked→ready-for-agent（PC 半边等 PS 端口同笔，单独改不了）。
- 2026-09-08 16:5x · MCP-2（task 449f4e64；分支 `mcp2-wbr04` 基 origin/main `62bf6504`，与派单写的 `2c0008c3` 只差一笔
  tasks.md）：**派单第 ① 步——量 PS 侧谁需要「复核授权」，先写这里再动代码。** 取证钉 `62bf6504`。
  - **消费点只有一处**：`internal/parcelshipment/application/complete_manual_review.go` 的 `CompleteManualReviewHandler.Handle`
    ——生产代码里唯一构造 `domain.ManualReviewCompletion` 的地方，三项引用（授权、复核人、证据）此刻整组来自
    `CompleteManualReviewCommand`，而命令的唯一生产来源是 `adapters/http/unconfigured_intake.go`（交回零值 +
    `ErrAccessChannelNotConfigured`；`cmd/parcel-api/endpoints.go` 接的就是 `UnconfiguredIntake{}`）。其余构造
    `CompleteManualReviewCommand{` 的三处全是测试（application 用例、`cmd/parcel-api/assemble_review_test.go`、
    `cmd/parcel-dispatch/manual_review_resume_loop_test.go`）。`cmd/parcel-dispatch` 的生产装配不建复核完成编排，
    派单写的「dispatch PS 组那几行」量下来是零行——只有 `cmd/parcel-api/assemble_review.go` 的
    `buildManualReviewOrchestration` 一处生产装配。
  - **用例依据**：UC-PS-001 步骤 8 与 `AT-PS-034`「前者只由规则授权的角色按证据完成复核」；CONTEXT 接受判断任务
    「等待人工复核：由适用规则授权的角色按证据完成复核」。「规则授权的角色」的规则属 `PAR-COM-14`（`BD-PS-002`），
    权威是 PC 的授权治理册——即 UC-PC-003 带 `ManualReviewAction` 的裁定。今天没人问它：`acceptance_task.go` 自注
    「授权规则属 party-commercial，本上下文保存所采用的授权引用」，而引用实际是调用方自报的。
  - **样本对照**：`RejectShipmentRequestHandler` 找到委托后、任何写动作前先问 `ActiveRejectionAuthorizer`；命令
    「不带授权引用：那由 party-commercial 签发，本编排去问，不由调用方声明」；三值落点——已授权带 PC 签发的引用继续、
    不允许答 `ActiveRejectionNotAuthorized`、未配置停未决、错误停未决。
  - **PS 不变式是否要改：不要。** `domain.NewManualReviewCompletion` 仍要求授权、复核人、证据三项必填，一字不动；
    变的只是授权引用的**来源**——从命令（调用方自报）换成 PC 裁定用例交回的所采用授权规则版本。这是应用层接线，
    不是领域规则改口，按派单不必停下。
  - **落地形状（照样本）**：`CompleteManualReviewCommand` 去掉 `Authority`（自带一个等于自己给自己签字）；
    `CompleteManualReviewDeps` 加 `Authorizer ports.ManualReviewAuthorizer`；编排在找到委托后先问授权，再做版本核对与
    领域动作。三值落点：已授权→以 PC 交回的引用构造完成留痕继续；不允许→新增 `ManualReviewNotAuthorized`（确定的业务
    答案，不落库）；未配置→新增 `ManualReviewAuthorityRulesNotConfigured`（UC-PC-003 结果表：保持未决等租户登记，不落库，
    不压成不允许）；权威答不出→error（未形成，HTTP 与本编排其余未形成答案一并 5xx `NO_ANSWER_FORMED`）。
    查询携带身份、委托、提交版本、复核人与证据引用——PC 的 `AuthorizationRequest` 要证据，PS 手上正好有；结构化原因 PS
    没有，由实例半边的请求映射（`ManualReviewAuthorizationRequestSource`，生产留 nil）给。适配器另加一道守卫：映射折出的
    请求动作不是 `ManualReviewAction` 即拒（`ErrUntranslatableAnswer`），「带 `ManualReviewAction`」这句做进结构而不只写注释。
- 2026-09-08 17:3x · MCP-2 **完成记录**（task 449f4e64；分支 `mcp2-wbr04`，已验 tip `8e52f413`；分支上的 SHA 作封存出处，进 main
  的 SHA 由推送方重放后补）。
  - **逐笔**：`67ca20d5` 票面第 ① 步（消费点量法）→ `f4e9a9e2` PS 端口 `ManualReviewAuthorizer` + 适配器
    `ManualReviewAuthorizationAdapter` + 七格适配器用例 → `80d100a2` 消费点接真 + `cmd/parcel-api` 装配 + **同笔** PC 半边删
    `ManualReviewRequirementFor` / 用例 / `acceptance_content.go` 引用 / 基线条目 → `8e52f413` 机制清点重生成（单独成笔）→ 本笔票面。
  - **消费点**：`internal/parcelshipment/application/complete_manual_review.go` `CompleteManualReviewHandler.Handle`——找到委托后、
    版本核对与一切写动作之前先问 `ports.ManualReviewAuthorizer`；`CompleteManualReviewCommand` 去掉 `Authority`，留痕里的授权引用
    自此是 PC 所采用的授权规则版本（`objectID/version`）。三值落点：已授权继续；不允许→`ManualReviewNotAuthorized`（不落库）；
    未配置→`ManualReviewAuthorityRulesNotConfigured`（不落库、不压成不允许）；权威答不出→error（未形成，HTTP 5xx
    `NO_ANSWER_FORMED`）。生产装配 `cmd/parcel-api/assemble_review.go` 的 `buildManualReviewOrchestration` 经
    `manualReviewOrchestrationWith(db, nil)` 接 `ManualReviewAuthorizationAdapter` + `NewAdjudicateCommercialAuthorizationHandler`
    （真授权册）；请求映射留 nil（`PAR-COM-14` 实例半边），与拒绝授权那只同款。`cmd/parcel-dispatch` 生产装配不建此编排（量得零行），
    只有 `manual_review_resume_loop_test.go` 的 fixture 多一个 `synRReviewAuthorizer` 替身。
  - **完成判据逐项**：① 裁决已由 MCP-1 记进 UC-PC-003 第四项（本单不动 UC/ADR；UC-PC-003 那段的将来时「在 PS 端口接真那一笔删去」
    今天兑现，改口归推送方 / 伞票，本单不碰 docs）。② PS 侧端口与 `ActiveRejectionAuthorizer` 同形（三值），适配器调既有
    `AdjudicateCommercialAuthorizationHandler` 带 `ManualReviewAction`（按 MCP-1 裁决读法，不是「改名后真调它」），
    `complete_manual_review.go` 的复核授权引用不再由 Intake 注入——命令里已无那一格。③ 基线 PC 段条目剪掉，成因**第一种**
    （并入 `Authorize`，不是改名也不是接上）；剪前核过全仓该名字只此一处声明；**在隔离 worktree 上 `git show` 取父提交
    `f4e9a9e2` 的本文件两法同得剪前 6，本笔单独作用于其上剪后 5**（PC 4→3；两法：UTF-8 逐行滤非空非注释 / 字节层数行首非 `#`），
    只对该检出成立——同波 wbr/03 在同段剪 `ResolveCreditPolicy`，两笔谁先进 main 都让另一笔的「剪前」不是 tip 真值，头注写明。
    族界那句随条目退场。④ MCP-6 补的理由行改写成历史注（调用方那句以「PS 端口 + 消费适配器」形状兑现，而不是谓词接上）。
  - **触及文件**：`internal/parcelshipment/ports/ports.go`（加端口；`AuthorizationOutcome` 头注两处「三个 / 三处」计数改不计数写法）、
    `internal/parcelshipment/adapters/partycommercial/manual_review_authorization.go` + `_test.go`（新）、
    `internal/parcelshipment/application/complete_manual_review.go` + `_test.go`、`cmd/parcel-api/assemble_review.go` +
    `assemble_review_test.go`、`cmd/parcel-dispatch/manual_review_resume_loop_test.go`、`internal/partycommercial/domain/authority_grant.go`
    （删谓词；`Authorize` 注释补一句为何不另立按范围谓词）+ `authority_grant_test.go`（删 `TestManualReviewIsNotRequiredUnlessDeclared`）
    + `acceptance_content.go`（分格注释改指 `Authorize` 带 `ManualReviewAction`）、`internal/architecture/production_wiring_baseline.txt`
    （PC 段 + 头注）、`docs/product/MECHANISM-INVENTORY.md`（重生成）。不碰：`commercial_resolution.go` / `credit_policy.go` / SA / TF /
    admin-web / PC http / PC `ports.go`；迁移不需；不立 ADR。
  - **验证强度**（detached 干净检出 `8e52f413`，含 DSN）：`gofmt -l .` 无输出；`go build ./...` / `go vet ./...` 退 0；
    `go test -p 1 -count=1 ./...` **100 ok / 0 FAIL / 16 no test files / 0 cached**（`FAIL` 裸 ASCII 子串零命中）。DSN 探针一正一反
    （`cmd/parcel-api -run Review -v`）：DSN 已设 `--- PASS` 3 / `--- SKIP` 0（1.06s），未设 `--- PASS` 0 / `--- SKIP` 3（0.016s），
    两次退 0、包行都 `ok`——复核装配三格真跑在 PG 上。端口三值探针各一（适配器）：`TestManualReviewIsGrantedWhenAMatchingRuleExists`
    / `TestManualReviewIsRefusedWhenTheScopeOnlyGrantsRejection` / `TestManualReviewReportsRulesNotConfiguredWhenTheBookIsEmpty` PASS，
    另三格：读失败上抛、映射折不出 / 未配置不问提供方、映射折出别的动作被拒。编排层六格（问的是命令里的复核人 / 证据 / 版本、不允许
    不落库、未配置不压不允许、未形成上抛、授权先于版本核对、留痕授权 = PC 交回那一版）PASS。装配层真 PG 三格：种一条 grant 后留痕
    授权 = `SYN-REVIEW-RULE-1/v1` 且恰好一封续办信封；空册答 `AUTHORITY_RULES_NOT_CONFIGURED` 零信封零留痕；生产装配（映射 nil）
    error 零信封零留痕。`internal/architecture` 全绿（`TestWiringBaselineHasNoStaleEntry` / `TestNoNewProductionFactoryGoesUnwired`
    / 事务闭包门禁——后者在草稿里抓过一次 `t.Fatalf` 进事务回调，已改）。**评审 ← 待推送方指定非作者通道。**
  - **量到但不在本单地盘的两件**（非阻断，另记）：a) `apps/admin-web/src/pages/shipment-request/api.ts` 的
    `ManualReviewCompletionOutcome` 联合类型少了本笔新增的 `NOT_AUTHORIZED` / `AUTHORITY_RULES_NOT_CONFIGURED`，且此前已与服务端
    对不齐（`TASK_CONCLUDED` vs 服务端 `TASK_ALREADY_CLOSED`，缺 `REVISION_CONFLICT`）——admin-web 归 MCP-6 awf 伞票，本单不碰；
    b) UC-PC-003「首切范围」一节仍写「`AuthorizedAction` 今天是封闭二值」，ADR-0116 之后已是三值——文档陈述过期，与本票无关，记给
    docs 收口的人。
