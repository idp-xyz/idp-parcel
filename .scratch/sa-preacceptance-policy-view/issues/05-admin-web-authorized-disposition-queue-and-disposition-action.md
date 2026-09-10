# admin-web 授权处置队列页与处置操作：读 `/authorized-disposition-queue`，两去向发 `/shipment-requests/authorized-dispositions`（未配置档），照复核页的形

Category: enhancement
Status: resolved——**2026-09-10 16:5x 进 main**（推送方通道 1 重放；通道 5 非作者评审两轴 0 阻断；分支→main 对照与验证见文末「进 main 记录」）。此前 15:5x 通道 6 完成（task-c9aaf964，分支 `mcp6-sa05` 基 `0e8d048a`，代码 tip `ca1233e7`，五笔见「完成记录」，票面笔随其后）；此前 in-progress——2026-09-10 15:2x 通道 6 立票并同笔认领（隔离 worktree 分支 `mcp6-sa05`，基 `0e8d048a`）：「要裁的」为零（页面位置有复核页先例、字段有服务端键名、去向有 ADR-0132、停点显法有复核页对 `UnconfiguredIntake` 的处理），按派单判据立票笔推 origin 后直接开工
Blocked by: 无（后端两口已随 [04](./04-authorized-disposition-flow-and-failure-disposition-read.md) 进 main：`GET /authorized-disposition-queue` 与 `POST /shipment-requests/authorized-dispositions`）

## 从哪里来

票 [04](./04-authorized-disposition-flow-and-failure-disposition-read.md)「边界」把「授权处置队列页归 admin-web 另票」划出，其完成记录「接续方看到的」明写「可另立票：admin-web 授权处置队列页与处置操作」；通道 5 的余工表判「无票・机制」。后端已在 main：

- 读面 `GET /authorized-disposition-queue`（`shipmenthttp.NewQueryAuthorizedDispositionQueueEndpoint`）：列「当前停在`等待授权处置`」的委托，`outcome = DISPOSITION_QUEUE_LISTED`，`entries[]` 每行 = 委托摘要（与复核队列同一套 `requestSummaryBody` 键）+ `lastAttemptReason` / `lastAttemptContinuation` / `lastAttemptedAt`（有过才在场）+ `controlResultId` + `restrictedItems[]`（`kind` / `order` / `basis` / `failureDisposition` / `responsibility`；空数组而不是 null——停在等处置的委托不该没有受限项，空数组是坏数据的可观察征兆）。只有列表；单份详情复用复核队列的 `?shipmentRequestId=` 分支。Intake 沿用委托查阅那个变量（隔离读准入随之放行）。
- 写面 `POST /shipment-requests/authorized-dispositions`（`shipmenthttp.NewDisposeShipmentRequestEndpoint(UnconfiguredIntake{}, …)`）：运营侧写行，挂字面量 `UnconfiguredIntake{}`（ADR-0085 未配置档进端点表、ADR-0055 未配置即拒）——今天必答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。响应封闭形状 `outcome` ∈ `RECORDED` / `ALREADY_DISPOSED` / `TASK_ALREADY_CLOSED` / `VERSION_SUPERSEDED` / `NOT_WAITING_ON_DISPOSITION` / `REVISION_CONFLICT` / `NOT_AUTHORIZED` / `AUTHORITY_RULES_NOT_CONFIGURED`，随附 `requestState` / `dispositionChoice` / `decisionKind` / `currentVersion` / `compensationReference`（各自只在该格在场）。去向原词 `REJECT` / `CUSTOMER_SUPPLEMENT`（`AuthorizedDispositionChoice`）。
- 既有复核队列单份 `recordedJudgments.financialControl.items[]` 已带 `failureDisposition` / `responsibility` 两格（票 04 第 5 步），前端 `ReviewFinancialControlRecord` 今天没有 `items`，复核页只显 `outcome` 与 `basis`。

## 范围

1. **队列页**（新页 `AuthorizedDispositionPage`，照 `AcceptanceReviewPage` 的形：`ReviewFlowTemplate` 队列 → 详情 → 决定 → 审计）：读 `/authorized-disposition-queue`；队列行显委托标识、客户账户、声明包裹数、提交时刻、受限项数；详情复用 `findAcceptanceReviewCase`（同一个 `?shipmentRequestId=` 分支）显委托字段与已记录判断三组；`detailExtra` 逐条列出本行 `restrictedItems[]`（种类 / 顺序 / 受限原因 / 失败处置 / 责任引用，原词直显）；空队列如实显空（「空队列是答案不是缺陷」）；`restrictedItems` 为空数组的行在队列上标出「受限项为空」不遮盖。
2. **处置操作**：决定集**只有两去向**——`拒绝`（`REJECT`）与`交客户补充`（`CUSTOMER_SUPPLEMENT`），ADR-0132 决定一「放行不在集内」；页面上不得出现任何「通过」「放行」「批准」按钮或措辞。发 `/shipment-requests/authorized-dispositions`，草案只送页面手上的事实 `{ shipmentRequestId, submissionVersionId, choice, reason }`（版本取队列行上看到的那一份：处置签在版本的判断任务上，不指名版本分不清签给了谁——`DisposeShipmentRequestCommand` 把版本设为本层必填的门）——处置人、授权依据、证据引用不在草案里（同 `ManualReviewCompletionDraft`「故意只有这两个字段」：那是「谁在签」，由 `PAR-INT-01` 的接入面从已认证的操作员身份翻译）。未配置档的停点原词直显（照复核页 `commandNoteOf` 对 `unconfigured` 的处理：`HTTP 403 ACCESS_CHANNEL_NOT_CONFIGURED`，不退化成「稍后重试」，不造开发用采信身份绕过）；八格 `outcome` 各自的中文标签取 ADR-0132 / `dispose_shipment_request.go` 注释原词，`AUTHORITY_RULES_NOT_CONFIGURED` 标为未配置态（恢复动作是租户登记 `PAR-COM-14`，不是重试、不是越权）；前端不自判权限。
3. **既有复核页**：`ReviewFinancialControlRecord` 加 `items[]`（`kind` / `order` / `conclusion` / `basis` / `failureDisposition` / `responsibility`），`recordedJudgmentsBlock` 的「财务控制」格逐项列出，两格采用引用有才显、没有不补「无」。
4. **导航与页名**：落「委托受理」区、紧挨「接受前人工复核」；条目 id `authorized-disposition`，页名「授权处置」（PS CONTEXT 词条原词）；`moduleInfoById` 记主责 `小包托运（parcel-shipment）` 与出处（CONTEXT「授权处置」词条与`等待授权处置`、ADR-0132）；`page-registry` 登记并进 `liveIds`（页面对真实端点发请求）。

## 不做的

- 不动 Go 侧（两口已在）。若读面缺一格（例如复核单份的 `occupationFormed` / `jointPassCondition` 今天前端类型也没有，处置角色要看「拒绝后有一笔要释放」得看它），票面记、不在本票补。
- 不写实例半边：不填任何真实角色 / 授权依据 / 原因目录取值；不给处置人、证据造占位。
- 不改 `ReviewFlowTemplate`（决定集参数化已够用）；不改 `catalogue-api` / `catalogue-view` 共享层。

## 要裁的

零。逐项对号：页面位置——复核页先例（同区、同模板）；字段——服务端键名逐字；去向与措辞——ADR-0132 决定一（封闭两值、放行不在集内）与硬句「人工处理不得绕过硬规则或把缺少的权威结果改成通过」；停点显法——复核页对 `UnconfiguredIntake` 的处理；草案键名——`ManualReviewCompletionDraft` 先例。

## 完成判据

- `node node_modules/typescript/bin/tsc --noEmit` 退 0；`node scripts/run-tests.mjs` 全 pass。
- 新页三态各至少一例（纯函数层，照本目录其余测试的写法）：空队列 → `empty` 且措辞说「空队列是答案」；有项 → 队列行与受限项文案含服务端原词（种类、失败处置、责任引用）；处置停点 → 未配置答复译成含 `403 ACCESS_CHANNEL_NOT_CONFIGURED` 与 `PAR-INT-01` 的停点说明、不含「稍后重试」。另一例钉决定集恰是 `REJECT` / `CUSTOMER_SUPPLEMENT` 两值且标签不含「通过 / 放行 / 批准」。
- 导航条目、页面登记、`liveIds` 三处齐；`pageTitleById` 由 `moduleInfoById` 派生无需另加。

## 地盘

`apps/admin-web/src/pages/shipment-request/**`（新页文件 + 纯函数模块与测试 + `api.ts` / `presentation.ts` / `index.ts` / `AcceptanceReviewPage.tsx` 各加一段）；共享文件 `apps/admin-web/src/navigation.ts`（条目 + 图标 + `moduleInfoById` 各一行）与 `page-registry.tsx`（登记 + `liveIds` 各一行）——改前广播占号。**同期在途**：通道 5 在 `apps/admin-web` 改注释（表单 / 共享层 / `policy-rows.test.ts`）、通道 3 在 `pages/party` 加交付条件一节，与本票目录不重叠。

## 参照

票 [04](./04-authorized-disposition-flow-and-failure-disposition-read.md)「裁决」与「完成记录」第 3、5 步；ADR-0132 决定一、二；ADR-0085、ADR-0055、ADR-0022；`internal/parcelshipment/adapters/http/query_authorized_disposition_queue.go` 与 `dispose_shipment_request.go` 的响应形状；`apps/admin-web/src/pages/shipment-request/AcceptanceReviewPage.tsx` / `api.ts` / `presentation.ts`；`ReviewFlowTemplate`；PS CONTEXT「授权处置」词条。

## 完成记录（通道 6 · 2026-09-10 · 分支 `mcp6-sa05`，基 `0e8d048a` = 派单时的 origin/main）

**每笔（分支上的 SHA，作封存出处；进 main 的 SHA 由推送方重放后并列补记）**

1. `2f7c3a6d` docs：立票，「要裁的」为零，同笔 in-progress。
2. `b45941d4` feat：`api.ts` 加队列 / 处置命令的响应形状与 `listAuthorizedDispositionQueue` / `disposeShipmentRequest` 两个出口、`AuthorizedDispositionDraft`、复核单份 `ReviewFinancialControlRecord.items[]`（`ReviewControlItemRecord` 带 `failureDisposition` / `responsibility`）；`presentation.ts` 加 `dispositionChoiceLabels` / `controlFailureDispositionLabels` / `authorizedDispositionOutcomeViews`；`authorized-disposition.ts` 纯函数（`dispositionChoiceOptions` 恰两去向、`dispositionQueueRowOf`、`restrictedItemText`、`dispositionCommandNoteOf`、`dispositionQueueViewState` 与空队列措辞）+ `authorized-disposition.test.ts` 十例。红先绿后：测试先编不过（模块未定义），再落实现。
3. `85583479` feat：`AuthorizedDispositionPage.tsx`（`ReviewFlowTemplate`：队列 → 详情复用 `findAcceptanceReviewCase` → 两去向 → 命令答复；受限项逐条原词直显；空数组标坏数据征兆）；已记录判断三组抽成 `recorded-judgments.tsx` 与复核页共用，`financialControl.items[]` 逐项列出、两格采用引用有才显；`AcceptanceReviewPage.tsx` 删本地那份改引用；`index.ts` 导出。
4. `15fc0f75` feat：`navigation.ts`（委托受理区紧挨接受前人工复核加条目 `authorized-disposition`、`Signpost` 图标、`moduleInfoById` 记主责与出处）、`page-registry.tsx`（`pageById` + `liveIds` 各一行）。共享文件占号 15:4x / 释号 15:5x 各广播过，纯加行零删行。
5. `ca1233e7` docs：一处注释去掉对 Go 侧 outcome 封闭集的计数措辞。

**触及**：`apps/admin-web/src/pages/shipment-request/{api.ts, presentation.ts, index.ts, AcceptanceReviewPage.tsx}` 各加一段 / 改引用；新文件 `AuthorizedDispositionPage.tsx`、`recorded-judgments.tsx`、`authorized-disposition.ts`、`authorized-disposition.test.ts`；共享文件 `navigation.ts`、`page-registry.tsx` 各几行；本票面。**未碰**：Go 侧任何文件；`ReviewFlowTemplate` 与 `templates/**`；`catalogue-api` / `catalogue-view`；`pages/party/**`（通道 3 / 5 地盘）；任何 `.sql` / `.go`。

**范围四件对照**

- ① 队列页：读 `GET /authorized-disposition-queue`；队列行显委托标识、客户账户、包裹数、提交版本、提交时刻、受限项数；详情复用复核队列的 `?shipmentRequestId=` 分支（读面只有列表，页面不请求第二份详情）显委托字段、控制结果标识、最近未推进、已形成决定，`detailExtra` 逐条列 `restrictedItems[]`（种类 / 顺序 / 受限原因 / 失败处置原词 + PC CONTEXT 中文 / 责任引用）；空队列 `empty` 态措辞「空队列是答案，不是缺陷」，进队列条件取 ADR-0132 决定一原句；`restrictedItems` 空数组 → 队列行状态字「受限项为空（坏数据征兆）」、详情说明它不是「没有受限项」。
- ② 处置操作：决定集在类型上就只有 `REJECT` / `CUSTOMER_SUPPLEMENT`（`AuthorizedDispositionChoice`），标签「拒绝」/「交客户补充」，图标 `XCircle` / `Undo2`（不用对勾）；理由必填由模板把守；草案 `{ shipmentRequestId, submissionVersionId, choice, reason }`；403 译成未配置停点（原词 `HTTP 403 ACCESS_CHANNEL_NOT_CONFIGURED`、点名 `PAR-INT-01`、不说「稍后重试」、不造采信身份）；结果词表逐格：`NOT_AUTHORIZED` 写「确定的业务答案，不是未决」，`AUTHORITY_RULES_NOT_CONFIGURED` 是唯一未配置态、恢复动作是登记 `PAR-COM-14`；`TASK_ALREADY_CLOSED` 带既有决定、`VERSION_SUPERSEDED` 带当前版本、`RECORDED` 带去向与之后的委托状态、补偿续办引用有才显。命令成立才重读队列（403 后不白跑）。前端零处判权限。
- ③ 复核页：`recordedJudgmentsBlock` 的「财务控制」格在结论 / 依据下逐项列 `items[]`，受限项的失败处置与责任引用有才显；`明确无控制`零项照实为空。两页共用同一份。
- ④ 导航与页名：委托受理区、紧挨「接受前人工复核」；id `authorized-disposition`、页名「授权处置」（PS CONTEXT 词条原词）；`moduleInfoById` 出处指 CONTEXT「授权处置」词条与 ADR-0132 决定一；`liveIds` 登记（页面对真实端点发请求）；`pageTitleById` 由 `moduleInfoById` 派生。

**完成判据逐项**：`node node_modules/typescript/bin/tsc --noEmit` 退 0 ✓；`node scripts/run-tests.mjs` 208 pass / 0 fail ✓（本目录新增十例：决定集恰两去向且标签不含「通过 / 放行 / 批准」；空 → `empty` 且说「空队列是答案」；有项 → `ready`、队列行与受限项文案带 `CREDIT_CHECK` / `CREDIT_LIMIT_EXCEEDED` / `AUTHORIZED_DISPOSITION` / `PARTY-FIN-1` 原词；采用引用缺席不写「无」；空数组标坏数据；403 停点含 `403 ACCESS_CHANNEL_NOT_CONFIGURED` 与 `PAR-INT-01`、不含「稍后重试」；`RECORDED` 带去向原词；`NOT_AUTHORIZED` / `AUTHORITY_RULES_NOT_CONFIGURED` 各说各的恢复动作；`TASK_ALREADY_CLOSED` / `VERSION_SUPERSEDED` 带凭据；4xx / 5xx / 传输层三格）；导航条目、页面登记、`liveIds` 三处齐 ✓。以上两项在 `ca1233e7` 的**干净 detached 检出**上（node_modules 走 junction 借共享树，拆前先 `rmdir` 掉 junction 再拆树）重跑同绿。无 Go 改动未跑 Go。

**给评审的判断题**

1. **「页面上不能有任何通过按钮」怎么证**：`AuthorizedDispositionChoice` 只有两值，`dispositionChoiceOptions` 的类型是它的数组，第三格在类型上写不出；测试钉 id 恰是 `['REJECT', 'CUSTOMER_SUPPLEMENT']` 且标签不含「通过 / 放行 / 批准」；页面组件只给这两格配图标，不另造决定；两个图标都不是对勾。
2. **草案多送 `submissionVersionId`**（派单只写了「两去向」，未点名版本）：`DisposeShipmentRequestCommand` 把版本设为本层必填的门（「不指名版本的处置无从判断它签给了谁」），队列行上正好带着它，送出去让服务端能答`版本已换代`而不是页面猜当前版。Intake 今天是 `UnconfiguredIntake{}` 不读请求体，键名照其余草案是页面侧暂定。若评审认为该等 Intake 落地再加，删一行加一处注释即可。
3. **已记录判断三组抽成共用文件**（`recorded-judgments.tsx`）而不是在复核页原地加 `items[]`：两页审的是同一批行，分两处写就得在两处约定哪一份是准的；复核页因此有一处删本地函数改引用（-45 +1），行为不变、多显 `items[]`。
4. **失败处置的中文标签取「按策略拒绝 / 进入授权处置」**（PC CONTEXT「失败处置只回答委托的去向——按策略拒绝或进入授权处置」），与去向标签「拒绝 / 交客户补充」分两张表：前者是正文登记的去向，后者是处置人选的去向，同一个 `REJECT` 串在两处含义不同，合成一张表会把「正文说拒」显成「处置人选了拒」。
5. **单份详情读不回时页面照样显受限项**：受限项来自队列行（本页自己的读面），已记录判断三组来自复核单份分支（另一次请求）；前者在场后者缺席时如实说「已记录判断未取回：缺在哪一步」，不因一口不通把另一口也藏起来。
6. **不在本票补的读面缺口**：复核单份 `financialControl` 的 `jointPassCondition` / `occupationFormed` 服务端有、前端类型没有——处置角色选「拒绝」前想知道「结论受限时第一项是否仍占着钱」得看 `occupationFormed`；本票按派单「读面缺一格票面记、不补」只记不补，前端类型也未加（加了不显是死字段）。可另立小票随复核页读面一起加。

## Comments

- 2026-09-10 15:2x · 通道 6（task-c9aaf964）：立票；「要裁的」为零，同笔认领 in-progress。能力边界：读过两口的 Go 响应形状与端点表、复核页三件（页 / api / presentation）、`ReviewFlowTemplate`、`navigation.ts` / `page-registry.tsx`、ADR-0132 决定一至四、票 04 全文；**没读** `@idpxyz/ui-*` 组件源码——页面只用复核页已用过的那几个。
- 2026-09-10 15:5x · 通道 6（task-c9aaf964）：五笔落地，Status 转 resolved，细节全在上方「完成记录」；开工前与共享文件落笔前各排过队列，占 / 释号各广播一次，无撞号。等非作者评审与推送方重放；进 main 的 SHA 由推送方补记。
- **评审 ← 通道 5 · 钉 `5213ef4a` · 16:5x**（task-d8b6c412，非作者；隔离检出只读，tsc 0 / run-tests 208 pass；两轴串行互不引用；由推送方代落）。
  - **Standards**：**阻断 0 · 非阻断 2**。(1) 跨文件计数——`AuthorizedDispositionPage.tsx` 头注「那里透出的判断三组」与 `recorded-judgments.tsx` 头注「已记录的权威判断三组」「三组各自缺席」数的是 `api.ts` `RecordedJudgmentsRecord` 的字段（后一句从 AcceptanceReviewPage 搬来的旧措辞，前两句新写）；作者 `ca1233e7` 已自清「八格」，这几处同性质。(2) Duplicated Code 判断题：`authorized-disposition.ts` `restrictedItemText` 与 `recorded-judgments.tsx` `controlItemText` 后半同形（失败处置 withCode + 责任两段），只差 conclusion 一格；可抽共用尾段，不阻。核过：注释全中文；引 ADR / CONTEXT 用决定号与词条名无行号；`moduleInfoById` 唯一来源只读引用；纯函数层与页面分层照复核页先例；`failureDisposition` 用 string 与 api.ts 既有各词表同款。无推测性通用。
  - **Spec**：**阻断 0 · 非阻断 2**。(1) 边外一根：队列行 subtitle 多显「版本 <submissionVersionId>」，票面范围 1 列的五样里没有它；与草案送版本同一理由，完成记录①已如实列，可接受。(2) `AUTHORITY_RULES_NOT_CONFIGURED` 的「未配置态」只在 note 文字里，tone 仍 'outcome'（与 RECORDED 同色），而渠道 403 另有 'unconfigured' tone；票面「标为未配置态」字面已满足，判断题。核过：① 无通过 / 放行——`AuthorizedDispositionChoice` 两值，`dispositionChoiceOptions` 是它的数组，测试钉 id `['REJECT','CUSTOMER_SUPPLEMENT']` 且标签不含通过 / 放行 / 批准。② 键名逐字对 Go：队列 `entries[].{lastAttemptReason,lastAttemptContinuation,lastAttemptedAt,controlResultId,restrictedItems[]{kind,order,basis,failureDisposition,responsibility}}`、`DISPOSITION_QUEUE_LISTED`、响应 `{outcome,requestState,dispositionChoice,decisionKind,currentVersion,compensationReference}`、八串 outcome 与 application 逐字同、复核单份 items[] 同 `reviewControlItemBody`；失败处置中文取 PC CONTEXT 原句。未核 `decisionKind` 两串是否逐字同。③ `dispositionCommandNoteOf('unconfigured')` 含「HTTP 403 ACCESS_CHANNEL_NOT_CONFIGURED」与「PAR-INT-01」，“稍后重试”只在否定句注释里；“权限|role”零命中。④ Go `DisposeShipmentRequestCommand` 有 `SubmissionVersion`，编排比对答 `VERSION_SUPERSEDED`——版本是真入参，送是对的；端点今天 `UnconfiguredIntake{}` 不解请求体，键名 `submissionVersionId` 无 Go 解码方可对，作者已自报「页面侧暂定」。⑤ `recorded-judgments.tsx` 与旧内联版逐行对：两组逐字同，财务控制只在 `items.length>0` 时多一个 `<ul>`；`items` 非可选依赖 Go 总是发（成立）；本目录此前无测试文件，「既有用例仍绿」实为全仓 198 仍绿，该块本身无用例覆盖。⑥ Go `reviewFinancialControlBody` 确有 `jointPassCondition`（omitempty）与 `occupationFormed`，TS 未加；票面「不做的」首条写明，确在范围外。
  - **六道判断题**：1 同意（类型上写不出第三格 + 测试钉标签）；2 同意（服务端真比版本，键名待 Intake 落地对）；3 同意（两页审同一批行，不是推测性通用；Standards (2) 那两条文案函数同理可共用）；4 同意（正文登记的去向与处置人选的去向同串不同义，分两张表对）；5 同意；6 同意（确在「不做的」内，建议小票随复核页读面一起加）。**结论：可重放。**
- **进 main 记录 · 通道 1 · 2026-09-10 16:5x**：分支 `mcp6-sa05` 六笔在 `48ccbb2f` 上 cherry-pick 全干净——`2f7c3a6d→fe239830` / `b45941d4→9243c9e8` / `85583479→796fa8ff` / `15fc0f75→96e8c890` / `ca1233e7→767e4146` / `5213ef4a→46c11d02`；本票 `apps/admin-web/src/pages/shipment-request/**`、`navigation.ts`、`page-registry.tsx` 与票面与分支逐字相同（`apps/` 下其余差异是 main 上通道 5 痕迹清剪那批注释，分支基底早于它）。验证钉 `46c11d02`（隔离 detached 树 `%TEMP%\idp-replay-sa05`，node_modules 走 junction 借共享树、验完拆）：`tsc --noEmit` 退 0；`run-tests` **pass 208 / fail 0**。无 `.go` / `.sql` 改动，不跑 Go 全量（`git diff --stat 48ccbb2f..46c11d02` 11 件全在 `apps/admin-web/src` 与 `.scratch`）。本笔（本条 + tasks.md）在 `46c11d02` 之上，纯 .md；`ls-remote` 核 `48ccbb2f` 未动后 ff 并 `push <sha>:main`。分支指针改 `merged/mcp6-sa05`、远端删；树由作者拆（先 rmdir junction）。评审四条非阻断随票记：Standards (1) 三处「三组」计数措辞 + Spec (2) 未配置态 tone，可随复核页 `occupationFormed` / `jointPassCondition` 两格那张小票一并改；Standards (2) 抽共用尾段、Spec (1) 版本 subtitle 认可不改。
