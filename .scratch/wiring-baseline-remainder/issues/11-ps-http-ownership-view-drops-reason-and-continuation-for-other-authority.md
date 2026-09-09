# HTTP 归属视图对「其他权威」决定不渲染未决原因、续办引用与确认引用：生产上唯一走得到的 Other 路在接入面缺格

Category: bug
Status: resolved——2026-09-09 18:5x 通道 3 完成（18:4x 认领），分支 `mcp3-wbr11` 基 main `d5a35960`，代码 tip `52b53ddb`；判据 1–4 全部，完成记录见文末 Comments；**19:54 进 main `94893c35`**（重放，非作者评审 ← 通道 2 无阻断；进 main 记录见文末）。此前 in-progress——2026-09-09 18:4x 通道 3 认领（用户 18:2x 经 IDP 队列指示由通道 3/4 自派；通道 3 18:29 点名后自领，分支 `mcp3-wbr11`，树 `D:/tops/idp-parcel-mcp3-wbr11`，基 main `d5a35960`；01 已进 main `8a1c403b`，阻断解除）。此前 ready-for-agent——2026-09-09 通道 1 代裁立票（用户授权自决）：wbr/01 评审（通道 3，钉 `a5bff461`）Spec 非阻断 1 点名「拆出物无票」；
作者完成记录「未落 ①」写「归 PS 另立票」，这就是那张票。**Blocked by [01](./01-ps-safe-handoff-is-assessed-nowhere-because-nothing-hands-over.md) 进 main**：本票改的
`newOwnershipView` 读的是 wbr/01 那五笔给 `ProductionOwnershipDecision` 加的评估格，01 未进 main 之前无处可接。PS 地盘。
Blocked by: 01

## 缺口

wbr/01 之后，`UC-PS-001` 步 3B 的「其他权威」路在生产上走得通了：编排在归属决定为 Other 时真调 `AssessSafeHandoff`，生产装配给出的通道是
`UnconfiguredOtherProductionAuthorityChannel{}`（ADR-0063 决定四「生产装配必须给出显式未配置实现」），于是今天每一个 Other 决定都以
「交接未决 · 通道未配置」收场，HTTP 答 `OWNERSHIP_UNRESOLVED`。

但 `adapters/http/submit_shipment_request.go` 的 `newOwnershipView` 只在 `decision.UnresolvedDetails()` 答 present 时才渲染 `unresolvedReason` 与
`continuationReference`，而 `UnresolvedDetails` 只对归属本身未决（Authority = Unresolved）的决定作答；Other 决定即使交接未决，它也答 false。结果：

- **Other + 交接未决**：HTTP 上是 `OWNERSHIP_UNRESOLVED` + `otherAuthority` + `handoffReference`，**没有未决原因、没有续办引用**——而 `UC-PS-001` 结果行
  「生产归属未决」要求带「安全续办引用」，编排层明明已经把 `CONT-PS-HANDOFF/<attemptID>` 那枚续办引用与 `CHANNEL_NOT_CONFIGURED` 那格原因写进了决定记录。
- **Other + 已确认**：HTTP 上是 `OTHER_PRODUCTION_AUTHORITY`，**没有确认引用**（渠道中立关联）；真通道接上那天这一格就会立刻被问到。

ADR-0128 决定五与 Consequences 把这一格点名为拆出物；wbr/01 完成判据 1–5 不含 HTTP，拆出成立，但拆出之后没有人认领——这就是本票。

## 要裁的形（实施者定，理由写完成记录）

JSON 契约两问：① Other + 交接未决 的原因与续办引用，是复用既有 `unresolvedReason` / `continuationReference` 两格（同一结果行、同一语义「未决 + 续办」），
还是另起名字；② Other + 已确认 的确认引用叫什么。倾向 ①复用、②另起一格（如 `handoffConfirmationReference`），但**不在此裁死**：读 ADR-0128 决定四
（决定记录分格）与 `ProductionOwnershipDecision` 上评估结果那两格的形之后定，管理台读这份 JSON 的那一侧（admin-web）若有既有字段名以它为准。

## 完成判据

1. `newOwnershipView` 对 **Other + 交接未决** 渲染未决原因（决定记录里评估结果那一格映射出的原因，今天生产上是 `CHANNEL_NOT_CONFIGURED`）与续办引用；
   对 **Other + 已确认** 渲染确认引用。字段名按上节裁定，写进 http 测试的字面断言。
2. http 测试补「未配置适配器 + Other」**一例**（wbr/01 评审 Spec 非阻断 2：加一例，不替换 `handoffChannelDouble` 给完整确认的那例）；
   `cmd/parcel-api` 装配测试在 `permittingOwnership` 之外补同一路一例，断言生产装配下 Other 决定的 HTTP 答复带续办引用。
3. 不改 domain / application / ports 的签名；不动 UC 文本；不改 ADR-0128（若裁定与决定五字面有出入，走 supersede 不改写）。
4. 验证（作者层）：`go build ./...` / `go vet ./...`；`go test -count=1` PS `adapters/http` + `application` + `./cmd/parcel-api/`（带 DSN）+ `./internal/architecture/...`。

## 边界

通往他方权威的真通道（`PAR-GOV-05..07` 实例半边）不归本票；决定记录落库（wbr/01「未落 ③」）不归本票；wbr/01 评审 Spec 非阻断 3
（`assessedAt` 取投递之前的 `decidedAt`，评估时刻系统性早于确认生效时刻）是作者记下的已知取舍，真通道接上时再看，不在此改。

## 进 main 记录（2026-09-09 19:54，通道 1 推送；本节由 20:0x 的接手会话据 git 现场与通道 1 队列补记——推送那一任 19:55 中断，只改了 Status 一行没写这里）

分支 `mcp3-wbr11` 四笔在隔离树 `%TEMP%\idp-replay-1950` 重放到 `efcdeabc`（= `d5a35960` + tasks.md 19:3x 簿记一笔）之后，零冲突：
`07f99781→d9958e1f` / `1c10b9b6→411d89f4` / `52b53ddb→f6268470` / `a0b3339f→94893c35`——四对 `patch-id --stable` 逐对相等，
`git diff a0b3339f 94893c35 -- <本票四文件>` 为空，全树差只有 main 上多的那一份 tasks.md（20:2x 复核）。清点在 `94893c35` 干净检出上重生成零差，不需清点笔（20:1x 实测）。
**远端 `main = 94893c35`**（19:54 推；20:03 `ls-remote` 同 SHA）。推前那一任有没有跑含 DSN 全量没留下记录，20:14–20:18 接手会话在同一棵干净检出补跑一次：
gofmt -l 空、`go build ./...` / `go vet ./...` 退 0、含 DSN `go test -p 1 -count=1 ./...` **102 ok / 0 FAIL / 15 无测试 / 0 cached，101 s**；探针 `TestFreezeScopesAreInvisibleToEachOther` `-v` PASS（非 SKIP）。
分支指针改名 `merged/mcp3-wbr11`，远端 `mcp3-wbr11` 删；树 `D:/tops/idp-parcel-mcp3-wbr11` 与 `idp-replay-1950` 先比内容再 `worktree remove`（未加 `--force`）。
评审 Spec 非阻断 1、Standards 非阻断 2 随票记不另立；未落 ① admin-web `handoffConfirmationReference` 一格归 awf/22 之后的 shipment-request 页面票。

## Comments

### 2026-09-09 18:5x 通道 3 · 完成记录（分支 `mcp3-wbr11`，基 main `d5a35960`）

**逐笔**

- `07f99781` docs(scratch)：票面认领转 in-progress。
- `1c10b9b6` feat(parcelshipment/http)：`newOwnershipView` 读决定上的 `SafeHandoff()` 那一格——评估未决时把 `UnresolvedReason()` 与
  `ContinuationReference()` 写进既有的 `unresolvedReason` / `continuationReference`，已确认时把 `ConfirmationReference()` 写进新加的
  `handoffConfirmationReference`；http 测试两例 + 响应体测试结构 `ownershipBody` 逐字钉字段名；夹具拆出 `newFixtureHandingOffThrough` 让出向通道可换，
  `newFixture` 仍给完整确认（原例不改）。
- `52b53ddb` test(parcel-api)：`TestAnOtherAuthorityDecisionReachesTheEndpointWithItsHandoffContinuation`——归属替身 `otherAuthorityOwnership`（答 Other + 停写证据，只记 `S`）
  + 生产的 `UnconfiguredOtherProductionAuthorityChannel{}` + 真仓储 / 边界壳 / 标识工厂 + 真端点，断言响应体 `OWNERSHIP_UNRESOLVED` 带 `CHANNEL_NOT_CONFIGURED` 与
  `CONT-PS-HANDOFF/PS-HANDOFF/SYN-OWN-DEC-OTHER-1`、不建单；`envelopeMintingSubmission` 的装配抽成 `submissionAssembledWith(t, db, ownership)` 两路共用，
  `fixedSubmissionIntake` 替未到位的接入契约。

**「要裁的形」的裁定**（写进 `ownershipView` 头注）：① Other + 交接未决 **复用** `unresolvedReason` / `continuationReference`——结果行同是
「生产归属未决 + 安全续办引用」，管理台 `SubmitShipmentRequestPage` 按这两个名字渲染「未决原因」「续办引用」，复用即零前端改动；两套原因词表
（`OwnershipUnresolvedReason` 四项、`HandoffUnresolvedReason` 六项）无同名项，且 `authority` 一格（`OTHER` / `UNRESOLVED`）已说明是哪一种未决。
② Other + 已确认 **另起** `handoffConfirmationReference`——与既有 `handoffReference`（停写证据）分列，ADR-0128 决定四两格不合并；不带 `effectiveAt`，
票面只要求确认引用（渠道中立关联）。ADR-0128 决定五字面「HTTP 面怎么呈现不裁」与本裁定无出入，不动 ADR。

**判据逐项**：1 ✓ 两条 http 用例分别钉 Other + 未决（`CHANNEL_NOT_CONFIGURED` + 续办引用 + 停写证据在、确认引用空）与 Other + 已确认
（`confirmation-1` + 停写证据在、未决两格空）；红先于绿——把 `submit_shipment_request.go` 退回 `07f99781` 版本两例都在断言处红，恢复后绿。
2 ✓ http 加一例不替换（`newFixture` 仍是完整确认替身，表驱动那格 `OTHER_PRODUCTION_AUTHORITY` 未动）；`cmd/parcel-api` 在 `permittingOwnership` 之外补
`otherAuthorityOwnership` 一路经真端点，同法退回验过红。3 ✓ domain / application / ports 零改动（`git diff --stat` 只有 http 两文件 + parcel-api 测试 + 本票面）；
UC 未动；ADR-0128 未改。4 ✓ `go build ./...` / `go vet ./...` 退 0；带 DSN（55432，量前 `pg_stat_activity` 零客户端，窗口先广播后关）
`go test -count=1` PS `adapters/http` + `application` + `cmd/parcel-api` + `internal/architecture/...` 四包 ok；`-v` 计 http + parcel-api 两包 PASS 98 / FAIL 0 / SKIP 0。
gofmt -l 空。

**红线自查**：注释全中文；跨文件引用皆符号名（`WithSafeHandoff`、`SubmitShipmentRequestPage`、ADR 编号）；无默认值——续办引用与原因都读决定上编排已记的格，
适配器不造任何值；适配器只引 application / domain（未新增 import）；不碰 admin-web、不碰 domain / application / ports、不碰 UC / ADR。

**未落 / 拆出**：① admin-web `apps/admin-web/src/pages/shipment-request/api.ts` 的 `ProductionOwnershipView` 类型与 `SubmitShipmentRequestPage` 尚无
`handoffConfirmationReference` 一格（未决两格已在渲染，本票零前端改动即生效）；加一行「确认引用」是 admin-web 地盘（此刻 awf/22 在做全页重构、通道 4 占），
归 admin-web 另立小票或随下一张 shipment-request 页面票收。② 非作者评审：本票由通道 3 实施，与 wbr/01 评审同一通道；两轴评审留通道 4 或用户。

**判断题**（供评审）：① 复用 `unresolvedReason` 而非另立 `handoffUnresolvedReason`——同一结果行一处渲染 vs. 两套词表进一格；② `handoffConfirmationReference` 不带
`effectiveAt`——票面未要求，加了就是替调用方决定它要不要；③ 装配测试用替身答 Other 而不是往治理登记册里登一条他方权威区间——后者是隔离形态（ADR-0091）的路，
要合成坐标与接管记录写侧，本票只证「决定到响应体」这一段。

**评审 ← 通道 2 · 钉 `a0b3339f` · 19:49**（基 `d5a35960`，隔离树 `%TEMP%\idp-review-wbr11` 只读；原文在通道 1 台账 `task-68149649`，全文 19:48 经队列送达通道 1）

- **范围核**：`git diff --stat d5a35960..a0b3339f` 恰四文件（票面 +42/−1、`cmd/parcel-api/assemble_submission_test.go` +139/−1、PS http `submit_shipment_request.go` +35/−10、`_test.go` +83/−6）；domain / application / ports / UC / ADR-0128 零改动（判据 3 ✓）。
- **验证**：`go build ./...` 0 · `go vet ./...` 0（含编译 `cmd/parcel-api` 测试文件）· `go test -count=1` PS `adapters/http`、`application`、`internal/architecture` 三包 ok · gofmt -l 两目录空。`cmd/parcel-api` 带 DSN 那例评审处未跑（无 DSN、55432 让给通道 3），作者自报 PASS 98 未复核——推送方 20:1x 在 `94893c35` 含 DSN 全量 102 ok 覆盖（见进 main 记录）。
- **Spec**：阻断 0。非阻断 ① `TestAnOtherAuthorityDecisionReachesTheEndpointWithItsHandoffContinuation`「不建单」只由响应体 `shipmentRequestId == ""` 钉，`submissionAssembledWith` 交回的 `outbox.Store` 被 `_` 丢弃——UC-PS-001 结果行「不得建立接受或拒绝决定、不得同时投递」是持久化层不变式，同文件 `claimSubmittedEnvelope` 已有取信封的路，补一句「outbox 零信封」即可把「不建单」钉到库里而不只钉到 JSON。无发现：① `newOwnershipView` Other + 未决走 `UnresolvedReason()` / `ContinuationReference()` 复用既有两格、只在评估未决时写，Other + 已确认走 `ConfirmationReference()`，新格与 `handoffReference` 分列、无 `effectiveAt`；Authority = Unresolved 那条 `UnresolvedDetails()` 路字面未动；两分支互斥由 `WithSafeHandoff` 拒非 Other 决定守住。② http 两例是加不是替（`newFixture` 仍交完整确认替身，表驱动 `OTHER_PRODUCTION_AUTHORITY` 格未动；`newFixtureHandingOffThrough` 只换一参，去掉的 `handoff` 字段原本无人读）。③ `submissionAssembledWith` 纯抽取；`fixedSubmissionIntake` 替的是生产 `endpoints.go` 也放 `UnconfiguredIntake{}` 的那一口，其余皆生产件。④ 两套原因词表（`OwnershipUnresolvedReason` 四值 / `HandoffUnresolvedReason` 六值）实核无交集；admin-web `api.ts` 既有三名与 JSON 同字。
- **Standards**：阻断 0。非阻断（皆判断题级）① Duplicated Code——`otherAuthorityOwnership.DecideProductionOwnership` 与 `permittingOwnership.DecideProductionOwnership` 六处逐字相同、只差四格，可抽 `syntheticOwnershipSpec(anchor, decisionID)`；② `newOwnershipView` 未决分支 `ContinuationReference()` 的 `present == false` 不可达（domain 在 `AssessSafeHandoff` 构造期已拒无续办引用的未决，ADR-0128 决定二），若某天可达会零信号，属防御性写法非错。无发现：注释全中文、只写取舍与所引规则；跨文件引用皆符号名 / 编号，无行号无计数；生产文件未新增 import；适配器只读决定上编排已记的格、不造值；http 测试引 `adapters/productionhandoff` 为的是票面点名的「生产同款未配置适配器」，architecture 包 ok。
- **判断题三道**均同意作者裁定：① 复用 `unresolvedReason`——结果行同为「生产归属未决 + 安全续办引用」，词表无交集、`authority` 格已分身份、admin-web 零改动即渲染；② 不带 `effectiveAt`——票面与 UC 3B 只要渠道中立关联，真通道要问时加一格是加不是改；③ 替身答 Other 而不登治理登记册——切在 ADR-0128 决定四那条缝上，走 ADR-0091 隔离形态是 wbr/01 未落 ③ 的地盘。
- **结论**：两轴无阻断，可进 main。推送方处置：三条非阻断随票记，不另立票（Spec ① 作者可选补、Standards ①② 备注）。
