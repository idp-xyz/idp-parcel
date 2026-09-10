# `parcel-shipment` 对不通过的接受前财务控制一律拒绝——策略正文里的失败处置（`REJECT` / `AUTHORIZED_DISPOSITION`）与责任引用没有消费方，「进入授权处置」那条路在本上下文不存在

Category: enhancement
Status: resolved——2026-09-10 14:3x 通道 6（单 task-276950ce），分支 `mcp6-sa04` 基 `84e89dc7`，代码 tip `99ddc8f5`（本笔票面在其上），main 上的 SHA 由推送方重放后补；「完成记录」见文末。此前：in-progress——2026-09-10 11:4x 通道 6 认领，按「做法」1→7 每步一笔并推；ready-for-agent——2026-09-09 通道 6 代裁四问（用户 22:2x 经队列授权，task-103327c7；分支 `mcp6-sa04` 基 `91df9aaa`），裁决落 [ADR-0132](../../../docs/adr/0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md) + PS CONTEXT 词条「授权处置」与`等待授权处置`一格，做法与完成判据见下；只裁未码。此前：draft——由票 [03](./03-parcel-shipment-expresses-per-item-control-results.md) 第 2 问拆出（2026-09-07，通道 3，ADR-0125）；PS 地盘，读口形状归 PC（已在）
Blocked by: 无

## 缺口

ADR-0115 让策略正文逐项带失败处置（`ControlFailureDisposition`：`REJECT` / `AUTHORIZED_DISPOSITION`）与责任引用
（`ControlResponsibilityReference`），并明写它们「只答委托去向，不拥有拒绝决定」；ADR-0122 决定四让它们不随 SA 答复走，
PS 要用时经自己的商业缝读。UC-PS-001 校验组「接受前财务控制」行写「任一必需控制不通过时按策略拒绝或进入授权处置」。

`parcel-shipment` 今天没有读它们的地方：`FinancialControlCheckFor` 把接受侧结论 `RESTRICTED` 一律译成确定性不通过，
`Decide` 据此形成拒绝。等于对每份合同都按 `REJECT` 处置；租户若在正文里登记 `AUTHORIZED_DISPOSITION`，委托会被自动拒绝
而不是进入授权处置。ADR-0125 把这一格记为过渡状态，不是产品口径。

## 要裁的

1. 「授权处置」在本上下文是什么：授权角色对一项不通过的控制能做什么——CONTEXT 写「人工处理不得绕过硬规则或把缺少的
   权威结果改成通过」，所以它大概率不是「放行」而是「决定去向」（拒绝 / 让客户补 / 换范围形成关联新委托…），封闭集要裁。
2. 它是接受判断任务上的一种等待态（与`等待人工复核`、`等待受控补充`并列，各有续办方与读面，ADR-0086 / ADR-0106 的形状）
   还是复用`等待人工复核`。分格判据是续办方是不是同一个角色、完成动作是不是同一个命令。
3. 读处置的时机与范围（票 03 已裁，这里只落地）：只在有项受限时读；只读绑定到本次委托费用范围的那几行——闭包里已采用
   结算政策的 `Applicability().ChargeScope()` → `PreAcceptanceFinancialControlPolicy.ItemsFor`，与 SA 执行时同源；按受限项的
   控制种类对上那一行的处置。读口在 PC（`PreAcceptanceFinancialControlPolicyContentView`），PS 侧适配器归本票，落在
   `internal/parcelshipment/adapters/partycommercial/` 新文件。
4. 责任引用在 PS 侧的落点：拒绝决定的依据里带不带；补偿（释放失败）续办时用不用。

## 边界

- 不动 SA；不改 PC 正文形状（`ControlFailureDisposition` 两值是 ADR-0115 定的）。
- 票 03 落地后 `FinancialControlResult` 已逐项，受限项的种类与顺序可直接对到正文行。
- 读口与流程同票落地：没有消费方的读路不单独建（ADR-0122 决定五、ADR-0125 同一判据）。
- 不动 `apps/`（授权处置队列页归 admin-web 另票）；PC 授权动作词汇要不要加「授权处置」一格归 PC 另票（见「裁决」风险点 2）。

## 裁决

2026-09-09，通道 6（task-103327c7），用户 22:2x 经 IDP 队列通道 1 授权代裁，以 `parcel-shipment` owner 口径裁；硬句「人工处理不得绕过硬规则或把缺少的权威结果改成通过」一字不改。理由、备选与全部依据在 [ADR-0132](../../../docs/adr/0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md)，这里只记结论；能力边界写在该 ADR 的 Status 行。按 /domain-modeling 走了三个场景：信用额度受限且正文登 `AUTHORIZED_DISPOSITION`；同一委托两项受限一 `REJECT` 一 `AUTHORIZED_DISPOSITION`；授权角色选「交客户补充」后客户补了再判。

**第 1 问：「授权处置」是授权角色对当前提交版本决定去向，去向封闭两值——`拒绝`、`交客户补充`；「放行」不在集内。** `拒绝`形成授权角色拒绝决定（留痕同主动拒绝：实际决定方、授权依据、结构化原因、证据、决定时间）；`交客户补充`转`等待受控补充`，由既有「新提交版本已形成」信封续办（ADR-0106），补充越出委托边界时按 CONTEXT 既有规则形成关联新委托——那是受控补充的一种结果，不是第三个去向。不在集内的三条各有理由：`放行`是硬句；`补资金后重判`没有触发（SA 资金事实到 PS 无信封、已记录判断只在依据失效时重判），按 ADR-0094 决定四「触发不同笔不许落地」排除、记为后继；`换控制策略 / 换合同`是 PC 侧商业变更，走「依据失效必须重新判断」。全部受限项都登 `AUTHORIZED_DISPOSITION` 才进授权处置；任一 `REJECT` 即确定不成立、照今天拒绝。两个去向都按 `OccupationFormed` 释放本版本已成立项的占用。

**第 2 问：独立等待态`等待授权处置`，不复用`等待人工复核`。** 票面两条分格判据都答否——续办方是授权处置角色（PC 授权规则说谁是，与复核权、拒绝权互不蕴含，`ports.go` 已用同一理由分开各 Authorizer 端口）；完成动作是处置命令选去向，而复核完成后决定由任务按规则形成、可以是接受——复用就是给一项`业务限制`开一条到接受的路。消费门处置为提交入账（ADR-0094 按恢复动作分格），保存护栏照 ADR-0086 决定一；触发是处置命令自身（`拒绝`在命令事务里形成决定，`交客户补充`转到`等待受控补充`由既有信封续办），不新增信封类型。译法：`RESTRICTED` 且全部受限项 `AUTHORIZED_DISPOSITION` → `无法判定` + 续办路径「授权处置」；任一 `REJECT` → `未通过`照今天。处置记录一版至多一次、不覆盖，形照 `ManualReviewCompletion`。

**第 3 问：不重裁，回指票 03 第 2 问与 ADR-0125 决定五。** 只定落地形：在形成控制判断那一步（SA 交回含受限项的结果时）经 PS 自己的 PC 消费缝读，只读闭包里已采用结算政策 `Applicability().ChargeScope()` 下 `ItemsFor` 的行，按受限项种类对行；读到的失败处置与责任引用作为**采用引用**记在受限的控制项结果上随 `FinancialControlResult` 落库，`Decide` 与读面都从已记录判断取、不重读正文（CONTEXT「保存实际采用的规则」）。对不上（该范围下无该种类的行）停`等待内部续办`，不折成任一去向。

**第 4 问：责任引用随受限项作为采用引用保存（与处置同格）；拒绝决定的依据经采用结果回指、不复制；补偿续办不用它。** 它答「谁承担失败或补偿责任」不答「谁有权处置」；授权处置队列读面透出它供处置角色看；释放按原业务关联请求、续办方是系统，责任方不改变谁来重试；本上下文不据它裁费用、追偿或通知。

**GLOSSARY 不加行**：GLOSSARY 今天没有接受前财务控制这条缝的任何词条（`接受判断任务`、`接受前财务控制采用结果`都不在），单加「授权处置」与它的覆盖面不一致；跨上下文关系已由 PC CONTEXT「失败处置只回答委托的去向——按策略拒绝或进入授权处置」与 PS CONTEXT 新词条两侧各说一半。

**越权风险点**（供 owner 复核，都不阻断开工；全文见 ADR-0132 Consequences）：1. `ResumePath` 加格与 CONTEXT / UC-PS-001 / ADR-0094 里以数目指称等待态的措辞要跟改（UC 归本票实施，ADR-0094 正文不改）；2. 处置`拒绝`由处置授权放行、不另问主动拒绝授权——「授权处置」要不要成为 PC 授权动作词汇的一格归 PC；3. 受限控制项结果加两格采用引用扩了 ADR-0125 决定一的形，ADR-0125 正文不改；4. `交客户补充`即释放本版本占用——把「接受确定未成立」读到了处置时刻；5. `补资金后重判`不入集，SA→PS 资金事实缝或「已记录判断失效重判」任一落地时回看；6. 多项受限一 `REJECT` 一 `AUTHORIZED_DISPOSITION` 时 `REJECT` 优先，今天走不到。

## 做法（顺序固定；实施者以代码为准，名字是提议不是定案）

1. **PS 领域**（`internal/parcelshipment/domain`）：`ResumePath` 加 `ResumeByAuthorizedDisposition`（`AUTHORIZED_DISPOSITION`）；`ControlItemResult` 受限项加两格采用引用——`ControlFailureDisposition`（PS 自有封闭集 `REJECT` / `AUTHORIZED_DISPOSITION`，镜像 PC 词汇不 import，集外报错不吸收）与 `ControlResponsibilityReference`（开放引用），构造门：受限项两格必填、成立项两格必空，`RehydrateFinancialControlResult` 同步；`FinancialControlCheckFor` 按处置分路（全部受限项 `AUTHORIZED_DISPOSITION` → `无法判定` + `ResumeByAuthorizedDisposition`，原因取判断顺序最靠前的受限项；任一 `REJECT` → `未通过`照今天）；处置记录 `AuthorizedDisposition`（去向 × 实际处置方 × 授权引用 × 原因引用 × 证据引用 × 处置时点，三引用必填，形照 `ManualReviewCompletion`）；`ShipmentRequest.DisposeUnderAuthority`：前置 `WaitingOn() == ResumeByAuthorizedDisposition`、未换代、未完结、一版至多一次；`拒绝`分支复用形成拒绝决定的领域门（实际决定方 = 处置方，依据经采用结果回指）；`交客户补充`分支把 `waitingOn` 转 `ResumeByCustomerSupplement`（不经校验的转移，形照 `AwaitOperatorRegistration`）。
2. **PS→PC 适配器**新文件 `internal/parcelshipment/adapters/partycommercial/`（处置读口）：实现新端口 `ports.ControlDispositionView`——按已采用商业解析引用 → 闭包 → 已采用结算政策 `Applicability().ChargeScope()` → 采用的控制策略版本 → PC `PreAcceptanceFinancialControlPolicyContentView.ItemsFor(scope)` → 按控制种类对行 → 译成 PS 采用引用；全函数；`found=false` / 对不上交回「未形成」，不折成任一去向。同目录另加处置授权的翻译适配器（形照主动拒绝 / 复核授权适配器）；PC 侧授权动作词汇未加之前如实答 `AUTHORITY_RULES_NOT_CONFIGURED`。
3. **PS 应用层**：形成控制判断那一步（`AdvanceFinancialControlJudgmentHandler`）在 SA 结果含受限项时经 `ControlDispositionView` 读处置、附到受限项后再记录 `FinancialControlResult`，读不到按 `judgment_continuation.go` 既有内部续办分格停；`FormAcceptanceDecisionHandler` 对 `ResumeByAuthorizedDisposition` 走 ADR-0086 决定一的保存护栏（形照 `pauseForManualReview`），交回新未决原因 `AuthorizedDispositionPending`，`undecidedDisposition` 穷举加该格 → 提交入账；新命令 `DisposeShipmentRequestHandler`（命令带 `Identity` / `ShipmentRequestID` / `SubmissionVersion` / `Disposer` / `Choice` / `Reason` / `Evidence`，不带授权引用——编排问 PC）：授权先于一切写动作，新端口 `ports.AuthorizedDispositionAuthorizer`（与 `ActiveRejectionAuthorizer` / `ManualReviewAuthorizer` 分立，端口注释写同一理由）；结果代数照 `ManualReviewCompletionOutcome` 分格（`RECORDED` / `ALREADY_DISPOSED` / `TASK_ALREADY_CLOSED` 交回既有决定 / `VERSION_SUPERSEDED` / `NOT_WAITING_ON_DISPOSITION` / `REVISION_CONFLICT` / `NOT_AUTHORIZED` / `AUTHORITY_RULES_NOT_CONFIGURED`）；两去向都在命令事务里对本版本按 `OccupationFormed` 发释放（形照 `RejectShipmentRequestHandler` 的释放段），失败按 `ControlReleasePending` 续办；不发信封。`cmd/parcel-api` 装配处置端点（Intake 以 `UnconfiguredIntake{}` 起步，ADR-0055）；`cmd/parcel-dispatch` 无新消费门。
4. **PS 迁移**（号开工时取；`parcel_shipment` 下一号在 `91df9aaa` 上量得为 `0021`——`0020` 已是 `resolution_key_credit_selector`，只作此刻取证）：`acceptance_financial_control_item` 加失败处置与责任引用两列（CHECK：受限行两列非空、成立行两列 NULL；存量行 NULL 如实读回、不补不拒）；`task_waiting_on` 投影列 CHECK 放宽加 `AUTHORIZED_DISPOSITION`；处置记录一版一行（形照复核完成留痕的落法）；`AcceptanceJudgments` / `ShipmentRequests` 读写跟随。
5. **HTTP 读面**：授权处置队列（`task_waiting_on = AUTHORIZED_DISPOSITION AND state = SUBMITTED`），逐行透出受限项、失败处置、责任引用、受限原因；既有复核队列读面的 `financialControl.items[]` 加两格。
6. **文档**：UC-PS-001 校验组「接受前财务控制」行「按策略拒绝或进入授权处置」补落地形一句、`尚未决定`段以数目指称等待态的措辞改口、`AT-PS-034` 补「授权处置从不形成接受」半句；ADR-0125 加「owner 复核记录」一条指本票解除过渡态（正文不改）；PS CONTEXT 已随裁决改（`cf8a3c4a`），实施若发现名字要变回来改。
7. **测试**：领域——处置分路三向（全部 `AUTHORIZED` / 任一 `REJECT` / 对不上）、处置记录门（未在等待态 / 已换代 / 已完结 / 重复）、两去向转移、`Decide` 不因处置记录而通过接受前财务控制那一组（反向一条）；应用层——保存护栏三测（形照 `form_acceptance_decision_test.go`）、`undecidedDisposition` 加格入账、处置命令授权先于写、两去向释放；PS→PC 适配器真库用例一正（`AUTHORIZED_DISPOSITION` 行读回译成采用引用）一反（范围下无该种类行 → 未形成，不折 `REJECT`）；PG——迁移往返 + 存量行 NULL 读回；架构——重建门登记、生产接线棘轮；`cmd/parcel-dispatch` 合成用例补一条走到`等待授权处置`并入账、再经处置命令两去向各收口一次（场景 1、3）。

## 完成判据

正文登 `AUTHORIZED_DISPOSITION` 的受限控制不再自动拒绝——任务停`等待授权处置`并落库（`task_waiting_on = AUTHORIZED_DISPOSITION` 可查）、消费门入账不重投，正文登 `REJECT` 照今天拒绝；处置命令两去向各成立（`拒绝`形成授权角色拒绝决定、留痕齐、依据回指采用结果并释放本版本占用；`交客户补充`转`等待受控补充`并释放，新提交版本信封续办后重判）；任何路径上受限项的`业务限制`原样保留、`Decide` 不因处置记录而通过接受前财务控制那一组；受限项的失败处置与责任引用随 `FinancialControlResult` 落库并可读回，存量行 NULL 如实读回；处置命令未获授权不碰聚合、PC 授权规则未登记答 `AUTHORITY_RULES_NOT_CONFIGURED`；gofmt / vet 0；含 DSN 跑 `./internal/parcelshipment/...`、`./migrations/...`、反向依赖含 `cmd/*` 的包；清点重生成；UC-PS-001 改口落地。

## 地盘

PS：`internal/parcelshipment/{domain,application,ports,adapters/partycommercial,adapters/postgres,adapters/http}`；`migrations/parcel_shipment/`（号开工时取）；`cmd/parcel-api` 装配一段；`internal/architecture` 登记；UC-PS-001、ADR-0125 复核记录。**不动**：SA 任何一格；PC 正文形状（授权动作词汇若要加归 PC 另票）；`apps/`。

## Comments

- 2026-09-07 · 通道 3：由票 03 第 2 问拆出立票，只写票面，未动代码。能力边界同票 03「裁决」节；此外读过 PC
  `pre_acceptance_financial_control_policy.go` 全文与 UC-PS-001 校验组行。
- 2026-09-09 · 通道 6：代裁四问，落 ADR-0132（`d85711ea`）+ PS CONTEXT 词条与等待态格 + ADR README 一行（`cf8a3c4a`）+ 本票面（本笔）；draft → ready-for-agent；只裁未码，`internal/**` 一字未动。GLOSSARY 未加行，理由见「裁决」。越权风险点六条待 owner 复核，任一被推翻改的是 ADR-0132 对应那一句与本票做法对应那一步，不改硬句。
- 2026-09-10 · 通道 6（新会话接手，前任只留下未提交的领域 red）：按「做法」1→7 实施，九笔全部推到 `origin/mcp6-sa04`，完成记录见下。

## 完成记录

分支 `mcp6-sa04` 基 `84e89dc7`（= 派单时的 origin/main），代码 tip `99ddc8f5`；票面本笔在其上。逐笔（分支上的 SHA，重放进 main 后由推送方对照）：

| 笔 | 步 | 内容 |
|---|---|---|
| `e4883039` | 1 领域 | `ResumePath` 加 `ResumeByAuthorizedDisposition`；`ControlFailureDisposition` / `ControlResponsibilityReference` / `AdoptedControlDisposition`（PS 自有封闭集，集外不吸收）；`ControlItemResult.WithAdoptedDisposition`（成立项拒、一次采用不覆盖、结论与原因不改）；`FinancialControlResult.AdoptControlDispositions`（只落受限项，对不上 `ErrControlDispositionNotAdopted` 整份不成立）与 `AwaitsAuthorizedDisposition`；`FinancialControlCheckFor` 按采用处置分路；`Decide` 认出该格（次序：客户补充 > 授权处置 > 运营登记 > 内部重试；已处置为交客户补充的版本再判不退回等处置）；`AuthorizedDisposition` 处置记录（形照 `ManualReviewCompletion`）；`ShipmentRequest.DisposeUnderAuthority`（拒绝复用抽出的 `formActiveRejection` 那道门，交客户补充只转等待态）；重建门收处置记录并拒两种拼出来的行。票面 Status → in-progress 随此笔 |
| `8c05797e` | 2 PS→PC | `ports.ControlDispositionView` / `ControlDispositionQuery`、`ports.AuthorizedDispositionAuthorizer`（与拒绝权 / 复核权端口分立）；`adapters/partycommercial/control_disposition.go`（回指闭包 → 已采用结算政策 `ChargeScope` → 已采用控制策略版本 → `ItemsFor` → 逐种类译成采用引用，与 SA 执行控制读的同源；found=false 三格，坏回指 error）；`unconfigured_authorized_disposition.go`（如实答 `AUTHORITY_RULES_NOT_CONFIGURED`） |
| `ca2c44e3` | 3 应用 | `AdvanceFinancialControlJudgmentHandler` 收 `ControlDispositionView`，受限结果先读处置采用到受限项再记录（`CONTROL_DISPOSITION_UNAVAILABLE` / `CONTROL_DISPOSITION_NOT_FORMED` 两格停内部续办、不记录、不折去向）；`FormAcceptanceDecisionHandler` 对`等待授权处置`走保存护栏、交回 `AUTHORIZED_DISPOSITION_PENDING`；`DisposeShipmentRequestHandler`（授权先于一切写；结果代数八格；两去向都按 `OccupationFormed` 释放；不发信封）；inbox `undecidedDisposition` 加格入账；`cmd/parcel-dispatch` 接受链装配接 `NewControlDispositionAdapter`。签名变更：`NewAdvanceFinancialControlJudgmentHandler` 加一参，六处调用点同笔 |
| `f686e4bd` | 3 HTTP | `adapters/http/dispose_shipment_request.go`（形照主动拒绝端点）+ `UnconfiguredIntake.IntakeAuthorizedDisposition`；`cmd/parcel-api/assemble_disposition.go`（事务壳 + `buildDispositionOrchestration`：PS 真库口 + SA 释放适配器 + 未配置授权器，Intake `UnconfiguredIntake{}`）；端点 `/shipment-requests/authorized-dispositions`；`unwiredDisposition` 占位与探测表 |
| `33fbfb6d` | 4 迁移 | `migrations/parcel_shipment/0021_authorized_disposition.sql`：两条 CHECK 对齐第五格、授权处置队列部分索引、`acceptance_financial_control_item` 加 `failure_disposition` / `responsibility_ref`（成立项必空、受限项同在或同缺、半截拒、处置只认封闭集）；`AcceptanceJudgments` 逐项读写跟随（两列 NULL 如实读回未采用）；`ShipmentRequests` 快照 `acceptanceTask.authorizedDisposition` 一格随任务往返 |
| `1aa5b6e3` | 5 读面 | `ports.AuthorizedDispositionQueue`（行带控制结果标识与逐项受限控制：种类 / 顺序 / 受限原因 / 失败处置 / 责任引用）；`adapters/postgres/authorized_disposition_queue.go`（在 `ShipmentRequestViews` 上，受限项从判断表本版当前采用的控制结果取）；`GET /authorized-disposition-queue`（只列表，详情复用复核队列分支）；复核队列 `financialControl.items[]` 加 `failureDisposition` / `responsibility`；`cmd/parcel-api` 装配（Intake 沿用委托查阅变量，隔离读准入随之放行） |
| `5f34f67b` | 6 文档 | UC-PS-001 三处改口（校验组行落地形、`尚未决定`段去数目措辞、`AT-PS-034` 加授权处置从不形成接受）；ADR-0125 owner 复核记录加「过渡态解除」一条，正文不改 |
| `f66af0d0` | 7 测试 | `cmd/parcel-dispatch/authorized_disposition_loop_test.go`：场景 1 + 交客户补充、场景 3 + 拒绝，两条真库闭环（生产链 + 消费门 + 队列读面 + 处置命令；只有权威口 / 处置读口 / 处置授权口是 synR 替身） |
| `99ddc8f5` | 清点 | `docs/product/MECHANISM-INVENTORY.md` 在 `f66af0d0` 干净 detached 检出重生成（parcelshipment 生产 143→151 / 测试 142→149 / 端口 17→18 / 迁移 20→21 / 接入面端点 11→13 / PS→PC 消费缝 14→16），只对该检出成立 |

第 7 步的其余测试随各步同笔：领域（`authorized_disposition_test.go`，前任 red 本会话对过 ADR-0132 后接 green；`revision_test` 转移表加行）；应用（`dispose_shipment_request_test.go` 保存护栏三测 + 处置命令十余格、`advance_financial_control_judgment_test.go` 读处置四组）；PS→PC 适配器真库一正一反 + found=false 三格 + 坏回指两例；PG 往返 + 存量行 NULL 读回 + 库面形状五探针 + 委托登记册三条 + 队列两条；HTTP 端点两条 + 复核详情两格；架构——`AuthorizedDispositionSpec` 由处置命令消费、类型可达性棘轮无需加基线行，两份基线文件一字未动。

**触及**：`internal/parcelshipment/{domain,ports,application,adapters/partycommercial,adapters/postgres,adapters/http,adapters/inbox}`、`migrations/parcel_shipment/0021`、`cmd/parcel-api`（新文件 + `endpoints.go` / `main.go` / `unwired_orchestration.go` / `endpoints_test.go` / `isolated_read_test.go` 各几行）、`cmd/parcel-dispatch`（`assemble.go` 接受链一段 + 三份既有回路测试各一行 + 新测试文件）、UC-PS-001、ADR-0125 复核记录、清点。**未碰**：SA 任何一格；PC 正文形状与授权动作词汇；`apps/`；ADR-0132 / 0094 / 0086 正文；`internal/architecture/*_baseline.txt`；`migrations.go` / `plan.go`（整目录嵌入）。

**完成判据逐项**：正文登 `AUTHORIZED_DISPOSITION` 的受限控制不再自动拒绝——任务停`等待授权处置`并落库、`task_waiting_on = 5` 可查（`TestAnAwaitingDispositionRequestProjectsItsWaitState`、合成闭环）、消费门入账不重投（`TestAuthorizedDispositionIsCommittedBecauseItsResumeTriggerIsTheCommandItself`、闭环里重投跳过）✓；正文登 `REJECT` 照今天拒绝（领域 `TestFinancialControlCheckRoutesARestrictedResultByItsAdoptedDisposition`、应用 `TestARejectDispositionStillRejectsWithoutPausing`）✓；处置命令两去向各成立——`拒绝`形成授权角色拒绝决定、留痕齐（实际决定方 = 处置方、授权引用来自 PC 答复、原因、证据、时点）、依据经采用结果回指不复制校验、释放本版本占用；`交客户补充`转`等待受控补充`并释放，新版本照旧可形成（领域 `TestDisposingToCustomerSupplementMovesTheWaitWithoutDeciding` 末段）✓；受限项的`业务限制`原样保留、`Decide` 不因处置记录而通过那一组（`TestADispositionRecordNeverPassesTheFinancialControlGroup`）✓；失败处置与责任引用随 `FinancialControlResult` 落库可读回、存量行 NULL 如实读回（PG 两条）✓；处置命令未获授权不碰聚合、未配置答 `AUTHORITY_RULES_NOT_CONFIGURED`（应用三格 + parcel-api 真库装配用例）✓；gofmt / vet 0 ✓；含 DSN 跑 `./internal/parcelshipment/...`、`./migrations/...`、反向依赖含 `cmd/*` ✓；清点重生成 ✓；UC-PS-001 改口 ✓。

**验证强度**（隔离树 `D:/tops/idp-parcel-mcp6-sa04`，tip `99ddc8f5`）：gofmt 空；`go build ./...` / `go vet ./...` 全仓退 0；带 DSN `-p 1 -count=1 -v` 跑 PS ports/domain 的反向依赖 23 包（`go list` 反查）+ `./cmd/...` + `./tests/...` + `./migrations/...` + `./internal/architecture/...`：34 包 ok / 2 无测试，`--- PASS` 2280 / `--- SKIP` 0 / `--- FAIL` 0。未跑全量（推送方那一跑兑底）。每一步的占 / 释 55432 都已广播。

**与 main 碰面**（干跑 `git merge-tree --write-tree origin/main HEAD`，origin/main = `62c87e73`，在 `84e89dc7` 之上 16 笔：psr/06、psr/07、nr/03、pc-gaps/11）：`.go` / `.sql` **零重叠文件**；唯一冲突是生成物 `docs/product/MECHANISM-INVENTORY.md`（两边各自在自己检出上重生成，预期内），推送方在 tip 上重生成即解。psr/06/07 在 PS domain / ports / adapters/postgres 全是新文件，与本票无同名；`production_wiring_baseline.txt` 只有它们动、本票未动。

**给评审的判断题**（每条都可推翻，改的是本票对应那一步，不改硬句）：

1. `Decide` 里`等待授权处置`压过`等待运营登记`与内部重试、让位于客户补充（`acceptance_decision.go` 那段注释）——理由是登记与重试都产不出一次处置、而处置的一个去向本就是交客户补充；反过来排（登记先）也说得通：登记之后整轮重跑，处置那一格自会再得机会。今天两格几乎不会同时到场（财务控制是链的最后一条腿）。
2. 已处置为`交客户补充`的版本再被判一轮时，`Decide` 把等待态留在`等待受控补充`而不退回等处置（`acceptance_decision.go`、领域 `TestADispositionRecordNeverPassesTheFinancialControlGroup`）——处置记录一版一次，退回去等于要处置角色再选一次。
3. `AdoptControlDispositions` 对成立项那一种类的行宽容（读到不记），对受限项缺行严格（整份不成立）——适配器因此交回范围下全部行，「对不上」只在领域一处判。
4. 处置授权的**翻译适配器没建**，只建了 `UnconfiguredAuthorizedDispositionAuthorizer`：PC 授权动作封闭集今天没有「授权处置」一格（ADR-0132 越权风险点 2），翻译适配器全部的意义在动作守卫上，而守卫要比对的动作不存在——照 `UnconfiguredSourceDataAmendmentAuthorizer` 先例。票面「同目录另加处置授权的翻译适配器」这半句因此改成随 PC 那一票落地。
5. 处置命令的决定标识在授权之后、领域转移之前签发（同主动拒绝）：撞领域门的几格（已处置 / 任务已完结 / 没停在等处置）会消耗一个标识；授权那三格不消耗。
6. `拒绝`去向形成的决定不带商业依据快照（`DisposeUnderAuthoritySpec` 没有 Basis 一格）——`RejectByAuthority` 本就允许 Basis 缺席，ADR-0132 决定四「依据经采用结果回指、不复制」；读面要看依据从已记录判断取。
7. 迁移 0021 受限项两列写成「同在或同缺」而不是票面字面「受限行两列非空」：同缺是存量行与采用之前记下的行（票面同一句要求「存量行 NULL 如实读回、不补不拒」），两句只能这样同时成立；领域把同缺读回「未采用」、按 ADR-0125 过渡口径译`未通过`。
8. `CONTROL_DISPOSITION_NOT_FORMED`（found=false 与对不上）归内部重试而不是运营登记：SA 刚按同一份正文执行完控制，正文那一侧「没有可读的行」只可能是换版竞争或坏数据，重读推得动；若把它读成 `PAR-COM-15` 未配置就该归登记——评审可判。
9. 读处置停在记录之前：`AdvanceFinancialControlJudgmentHandler` 读不到处置时不记录结果，下一轮重新向 SA 发起控制——与今天 `JudgmentNotRecorded` 那一支同样依赖 SA 对同一请求身份的幂等（`FinancialControlRequestConflict` 那一格）。
10. 队列读面只有列表，单份详情复用复核队列的 `?shipmentRequestId=` 分支（那里的判断三组已带两格采用引用）；`AcceptanceTaskViewRecord` 没加处置留痕字段——处置一记下等待态就转走，队列行不需要它，详情里要看处置记录归后继（读面票）。
11. 第 3 步拆成两笔（应用 / HTTP），其余按票面顺序 1→7；第 1–3 步的中间态有两处按设计红（类型可达性棘轮点名 `AuthorizedDispositionSpec` 直到第 3 步接上；PG 两条逐格镜像用例直到第 4 步迁移），各自的提交信里写明，未加基线行也未改镜像用例的目的。
12. 处置命令不做决定交接（不发接受决定信封）——同主动拒绝（ADR-0086 决定三保留条款）；`FormAcceptanceDecisionHandler` 的 `handOffDecision` 对拒绝也会发，两条拒绝路径在这一点上本就不同，本票没有改它。

**接续方看到的**：无阻断。可另立票：admin-web 授权处置队列页与处置操作（票面「边界」已划归 admin-web 另票）；PC 授权动作词汇加「授权处置」后把装配点的未配置授权器换成翻译适配器（形照 `manual_review_authorization.go`，动作守卫比对新词）。
