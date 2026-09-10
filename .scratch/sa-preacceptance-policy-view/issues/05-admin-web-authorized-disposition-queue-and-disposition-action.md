# admin-web 授权处置队列页与处置操作：读 `/authorized-disposition-queue`，两去向发 `/shipment-requests/authorized-dispositions`（未配置档），照复核页的形

Category: enhancement
Status: in-progress——2026-09-10 15:2x 通道 6 立票并同笔认领（task-c9aaf964，隔离 worktree 分支 `mcp6-sa05`，基 `0e8d048a`）：「要裁的」为零（页面位置有复核页先例、字段有服务端键名、去向有 ADR-0132、停点显法有复核页对 `UnconfiguredIntake` 的处理），按派单判据立票笔推 origin 后直接开工
Blocked by: 无（后端两口已随 [04](./04-authorized-disposition-flow-and-failure-disposition-read.md) 进 main：`GET /authorized-disposition-queue` 与 `POST /shipment-requests/authorized-dispositions`）

## 从哪里来

票 [04](./04-authorized-disposition-flow-and-failure-disposition-read.md)「边界」把「授权处置队列页归 admin-web 另票」划出，其完成记录「接续方看到的」明写「可另立票：admin-web 授权处置队列页与处置操作」；通道 5 的余工表判「无票・机制」。后端已在 main：

- 读面 `GET /authorized-disposition-queue`（`shipmenthttp.NewQueryAuthorizedDispositionQueueEndpoint`）：列「当前停在`等待授权处置`」的委托，`outcome = DISPOSITION_QUEUE_LISTED`，`entries[]` 每行 = 委托摘要（与复核队列同一套 `requestSummaryBody` 键）+ `lastAttemptReason` / `lastAttemptContinuation` / `lastAttemptedAt`（有过才在场）+ `controlResultId` + `restrictedItems[]`（`kind` / `order` / `basis` / `failureDisposition` / `responsibility`；空数组而不是 null——停在等处置的委托不该没有受限项，空数组是坏数据的可观察征兆）。只有列表；单份详情复用复核队列的 `?shipmentRequestId=` 分支。Intake 沿用委托查阅那个变量（隔离读准入随之放行）。
- 写面 `POST /shipment-requests/authorized-dispositions`（`shipmenthttp.NewDisposeShipmentRequestEndpoint(UnconfiguredIntake{}, …)`）：运营侧写行，挂字面量 `UnconfiguredIntake{}`（ADR-0085 未配置档进端点表、ADR-0055 未配置即拒）——今天必答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。响应封闭形状 `outcome` ∈ `RECORDED` / `ALREADY_DISPOSED` / `TASK_ALREADY_CLOSED` / `VERSION_SUPERSEDED` / `NOT_WAITING_ON_DISPOSITION` / `REVISION_CONFLICT` / `NOT_AUTHORIZED` / `AUTHORITY_RULES_NOT_CONFIGURED`，随附 `requestState` / `dispositionChoice` / `decisionKind` / `currentVersion` / `compensationReference`（各自只在该格在场）。去向原词 `REJECT` / `CUSTOMER_SUPPLEMENT`（`AuthorizedDispositionChoice`）。
- 既有复核队列单份 `recordedJudgments.financialControl.items[]` 已带 `failureDisposition` / `responsibility` 两格（票 04 第 5 步），前端 `ReviewFinancialControlRecord` 今天没有 `items`，复核页只显 `outcome` 与 `basis`。

## 范围

1. **队列页**（新页 `AuthorizedDispositionPage`，照 `AcceptanceReviewPage` 的形：`ReviewFlowTemplate` 队列 → 详情 → 决定 → 审计）：读 `/authorized-disposition-queue`；队列行显委托标识、客户账户、声明包裹数、提交时刻、受限项数；详情复用 `findAcceptanceReviewCase`（同一个 `?shipmentRequestId=` 分支）显委托字段与已记录判断三组；`detailExtra` 逐条列出本行 `restrictedItems[]`（种类 / 顺序 / 受限原因 / 失败处置 / 责任引用，原词直显）；空队列如实显空（「空队列是答案不是缺陷」）；`restrictedItems` 为空数组的行在队列上标出「受限项为空」不遮盖。
2. **处置操作**：决定集**只有两去向**——`拒绝`（`REJECT`）与`交客户补充`（`CUSTOMER_SUPPLEMENT`），ADR-0132 决定一「放行不在集内」；页面上不得出现任何「通过」「放行」「批准」按钮或措辞。发 `/shipment-requests/authorized-dispositions`，草案只送页面手上的事实 `{ shipmentRequestId, choice, reason }`——处置人、授权依据、证据引用不在草案里（同 `ManualReviewCompletionDraft`「故意只有这两个字段」：那是「谁在签」，由 `PAR-INT-01` 的接入面从已认证的操作员身份翻译）。未配置档的停点原词直显（照复核页 `commandNoteOf` 对 `unconfigured` 的处理：`HTTP 403 ACCESS_CHANNEL_NOT_CONFIGURED`，不退化成「稍后重试」，不造开发用采信身份绕过）；八格 `outcome` 各自的中文标签取 ADR-0132 / `dispose_shipment_request.go` 注释原词，`AUTHORITY_RULES_NOT_CONFIGURED` 标为未配置态（恢复动作是租户登记 `PAR-COM-14`，不是重试、不是越权）；前端不自判权限。
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

## Comments

- 2026-09-10 15:2x · 通道 6（task-c9aaf964）：立票；「要裁的」为零，同笔认领 in-progress。能力边界：读过两口的 Go 响应形状与端点表、复核页三件（页 / api / presentation）、`ReviewFlowTemplate`、`navigation.ts` / `page-registry.tsx`、ADR-0132 决定一至四、票 04 全文；**没读** `@idpxyz/ui-*` 组件源码——页面只用复核页已用过的那几个。
