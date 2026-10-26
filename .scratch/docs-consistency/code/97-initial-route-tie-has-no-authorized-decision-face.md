# 97 NR 初始路由最低成本并列没有授权裁决口：并列候选落了未形成登记，却读不回、也重判不了

Category: enhancement
Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 claude-code97（代码最后一笔 e9285b177）。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）分诊升 R1（上线前必修：并列的包裹永远没有初始路由、无绕法，见 [分诊报告](../../needs-triage-sweep-2026-10-26.md)；降级条件见报告）并认领（tops-26 的 code/ 队列已停），分支 `claude-code97`；新 ADR 若要取 0309。此前 needs-triage——建议 R2（上线后尽快），与 code/29 同级：成本单维择优下并列是常见情形，并列的包裹停在`路由未形成`，原因与候选可查，可没有任何口能裁。绕法：没有——之后每一次复核都落「没有路由历史」入账未判。2026-10-26 票 [code/29](./29-reroute-suggestions-and-ties-have-no-authorized-decision-face.md) 作者子代理立（协调方分诊：demo-chain-gaps/155 之后并列候选读不回就不在 code/29 做，立新票写清依赖），分支 claude-code29。分级由协调方定稿。
Blocked by: [code/29](./29-reroute-suggestions-and-ties-have-no-authorized-decision-face.md)（授权决定口的形状，以及它停下等的那份地理投影取法记录）；[demo-chain-gaps/154](../../demo-chain-gaps/issues/154-network-reassessment-not-judged-triggers-resume-after-registration.md)「要裁的」第二条（初始路由重驱的地理投影从哪里来）
出处：UC-NR-001「给开发的交接」并列一行；code/29「最小的诚实修法」第 3 条与它的「停工记录」。

## 现象

初始路由遇最低成本并列时入账为`路由未形成`（`CANDIDATES_TIED`）。NR CONTEXT「路由形成与选择」写并列「交授权角色裁决」，UC-NR-001 结果「路由未形成」写它等「授权角色的裁决」。没有任何口能作这个裁决，包裹一直没有初始路由。

## 证据

- 票 155 之后，并列落 `network_routing.initial_route_not_formed`：全部候选、并列的那几家与各自打平的金额与币种。端口 `InitialRouteNotFormedRegister` 只有写法 `RecordNotFormed`；生产代码与管理台都没有读这一册的地方。
- 候选行只存标识、结局与理由（`candidateRow`），没有路径。组计划要此刻的证据：段链、交付段（ADR-0217），以及选中候选的逐段成本出处。
- 初始路由的证据要随接受决定信封携带的地理投影与商业解析标识（`CreateInitialRouteCommand` 的 `Resolution`、`Carried`）。NR 不落投影本体（ADR-0075 决定三）。经 `AcceptedCarriedGeoSource` 读受理基线那一版投影的适用场景只到受理之后、不由小包托运信封触发的复核（ADR-0215 决定二、ADR-0301）。票 155 完成记录的代决对初始路由续办核得同一处，并入了票 154。

## 三类

- 机制：读回未形成登记里的并列行、按此刻的证据重判所择候选、形成首个当前有效路由计划的那道口，以及它的门：不得绕过硬限制、商业服务范围与他上下文的既有责任。
- 产品策略：口的形状同 code/29（操作者渠道运营决定族）。
- 租户取值：谁持有这一种授予，未登记答未配置。

## 规模

M：未形成登记的读法、运营决定口（随 code/29 的形）、编排（读回并列行、核它仍是这件包裹最近一次停住的原因、重判所择候选、形成首个计划）、管理台读面、用例。

## 最小的诚实修法

1. 未形成登记加读法：按判断键读回最近一次停住的原因与并列的那几家。
2. 运营决定口与 code/29 同族：指名包裹与所择候选，带决定依据引用，操作者从会话取。
3. 编排：在判断键上串行。这件包裹已有初始路由（判断库有行）即答已有结果；登记行不是并列，或所择候选不在并列的那几家里，即拒。重取证据与成本，重判所择候选，被淘汰、不合格或证据未知各自具名拒。成立时形成首个当前有效路由计划，同笔落判断库与交接意图，与初始路由形成同一条提交边界。
4. 授予未登记答未配置，不默认给任何角色。

## 要裁的

- 地理投影与商业解析标识从哪里来：与票 154「要裁的」第二条同一问，并与 code/29 等的那份记录一并裁。
- 授权裁决形成的初始计划从何时起生效：照 CONTEXT「路由计划适用性」里初始计划的口径取委托接受的时刻，还是照 code/29 草案取决定的时刻。
- 授予与 code/29 同一种，还是另成一种。

## 完成记录（2026-10-26，作者，分支 claude-code97，起点 `b6f0c10ba`）

### 提交

- `2b5055592` feat(network-routing)：端口、`DecideInitialRouteHandler`、未形成登记读法、裁决证据册、迁移 `network_routing/0030_initial_route_tie_decision.sql`。
- `8653f422d` feat(parcel-api)：networkhttp 的裁决口与读面、parcel-api 特性文件与装配。
- `931c9162f` feat(admin-web)：路由计划页「最低成本并列」一册。
- `e9285b177` test(parcel-api)：真锁并发一条（同笔含编排一处注释的步骤号）。
- `df001265c` docs(adr)：ADR-0309、ADR-0075 / 0215 / 0284 的 Status 注记与 Links、NR CONTEXT、UC-NR-001、AT 盘点。

### 各层改了什么

- 端口 `InitialRouteNotFormedRead`（按判断键、按包裹读回最近一次停住的原因与并列的那几家）、`InitialRouteTieDecisions`、`InitialRouteTieCatalogueRead`；`ParseInitialRouteNotFormedReason`。
- 应用 `DecideInitialRouteHandler`：借判断键锁（`LatestTriggerAt`，与复核、授权改路同一把）→ 已有结果 → 并列登记 → 所择在并列里 → 受理基线投影 → 重判 → 取成本、组计划 → 落判断库、裁决证据、交发布意图。重判判法复用 `refusalOf`、`evaluateRouteCandidates`、`loadRouteCandidateCosts`，没另写一套。
- postgres：`InitialRoutes.FindNotFormed` / `FindLatestNotFormedOfParcel`；`InitialRouteTieDecisions`（`Record` 与 `ListInitialRouteTies`）。迁移 0030（外键指 `initial_route` 与 `initial_route_not_formed`）。取号：network_routing main 末号 0029（code/29），在途分支只见同一份 0029，取 0030；access_identity 无迁移。
- HTTP / 装配：`POST /route-initial-route-decisions`（`decisionFace(DecisionAuthorizedReroute)`）、`GET /route-initial-route-ties`（`operationsReadFace`）；`transactionalInitialRouteTieDecider` 事务壳；意图走真 outbox（`NewOutboxInitialRouteHandoff`）。
- 管理台：code/29 有同形的改路建议一册，照做并列一册（`InitialRouteTiesView`、`initial-route-tie.ts` 纯逻辑与 node:test、`api.ts` 两个函数）；演示种子不动（配置员已持这一种授予）。
- 文档：ADR-0309（号取 0309，`git branch -r` 与各 worktree 无撞）；NR CONTEXT 两句；UC-NR-001 结果后一段、候选判断规则末句、步骤 8D、交接表两行、AT-NR-100–103（全仓与各 `claude-*` 分支最大 AT-NR 号为 099，无撞）；AT 盘点分桶表四行与一段追记。

### 三问怎么裁（详见 ADR-0309）

1. 地理投影与商业解析标识：投影经 `AcceptedCarriedGeoSource` 取受理基线那一版（ADR-0075 决定一被停用的适用场景再扩一格，限 NR 授权决定编排）；**商业解析标识不取**（改了倾向）：证据视图与候选成本取数侧签名里没有它，它只喂服务级适用性判断，登记行在场即入账时已判过。
2. 生效时刻：取决定的时刻，判断时刻同取，与 code/29 一致；委托接受到决定之间的无路由照留。
3. 授予：与授权改路同一种（`AUTHORIZED_REROUTE`），不另立决定种类、不加 access_identity 迁移。

### 代决与越权风险点

代决（作者读码后定，写进了 ADR-0309）：商业解析标识不取（风险点 2）；授予同一种（风险点 4）；所择候选只能在并列的那几家里（风险点 3）；意图交不出去整笔回滚而不是留续办引用（一次只有一件包裹）；读面状态三格含「已另行形成」；管理台一册照 code/29 做。

ADR-0309 的越权风险点编号：1 NR 读 PS 多一个调用方（PS 会签）；2 不重问商业适用性；3 并列之外的合格候选不能经这个口选；4 授予粒度（PC owner）；5 委托接受到决定之间业务发生、决定之后才处理的控制触发（与 code/100 同类，不吞）；6 下游当裁决形成的计划是普通初始路由；7 登记行裁决前被改写，口在锁里重读。

### 判据对用例

| 判据 | 用例 |
|---|---|
| 已有结果 | `TestADecisionOnAParcelWithAnInitialRouteAnswersTheExistingResult` |
| 非并列拒（含无登记行） | `TestADecisionWithoutATieRegistrationIsRefused` |
| 所择不在并列里拒 | `TestACandidateOutsideTheTieIsRefused` |
| 重判被淘汰 / 不合格 / 证据未知拒；未配置、未决、基线对不上 | `TestAChosenCandidateThatNoLongerHoldsIsRefusedWithItsReason`、`TestTheTieDecisionNamesWhatItCannotAnswer` |
| 成立形成首个计划（生效取决定时刻，意图带原交接关联，投影取受理基线） | `TestAnAuthorizedRoleDecidesALowestCostTie`；意图交不出整笔失败 `TestAFailedHandoffFailsTheWholeTieDecision` |
| 真库：并列入账后经决定口形成首个计划，判断库有行 | `cmd/parcel-api` `TestAnAuthorizedOperatorDecidesAnInitialRouteTieOnTheDatabase`（含裁决证据行、outbox 意图、载荷自报操作者 400、第二次答已有结果） |
| 生产装配：授予未登记答未配置，登记后成立 | `TestAnUnregisteredGrantAnswersNotConfiguredOnTheTieDecisionFace` + 上一条 |
| 判断键锁串行 | `TestTwoConcurrentTieDecisionsFormOnlyOneFirstPlan` |
| 登记读法、裁决证据册、读面三格 | postgres `TestTheNotFormedRegisterReadsBackTheLatestReasonAndTheTiedOnes`、`TestTieDecisionEvidenceAndTheTieReadFaceStates` |

### 自验

含 PG（`IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres`）：`go build ./...`、`go vet ./internal/networkrouting/... ./cmd/...`、`gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/networkrouting/... ./cmd/parcel-api/... ./cmd/parcel-dispatch/... ./internal/architecture/... ./migrations/...` 全过；admin-web `tsc -b`、`npm test`（686 过）、`vite build` 过（node_modules 借主树的，未入库）；`go run ./tools/at-coverage-counts -check` 只报生成物过期。未跑全量。ADR 索引、AT 盘点计数与机制清点由推送方重生成。

### 给评审的提醒

- 登记行的 `settled_at` 同刻时按判断键各维收尾取最近一行；一件包裹换基线重新受理才会有第二行。
- 决定口先由包裹读到判断键、再取锁、锁里重读，不是先锁：包裹到判断键这一步在锁外，读到的键只用来取锁。
- NR CONTEXT 改了两句：客户地址那一条补「授权角色裁初始路由最低成本并列同样，取法同此」；路由计划适用性首条补「授权角色裁最低成本并列形成的首个计划同样从决定的时刻起生效，委托接受到决定之间的无当前有效路由照留」。
