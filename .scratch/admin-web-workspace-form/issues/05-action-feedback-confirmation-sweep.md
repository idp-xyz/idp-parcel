# 05 写动作反馈与高风险确认一致性：先审计全部写面调用点成对照表，再补 `useToast` / `ConfirmDialog` / 进行中态

Category: enhancement
Status: resolved——码三笔 tip `fef2958c`（分支 `mcp5-wsform05`，基 `11e6111a`；通道 5 20:26 推完最后一笔后会话未再响应，完成记录由推送方按 git 与票面对照表代落，见下）；进 main 记录见 Comments。此前 in-progress
认领：通道 5 · 2026-09-20 20:1x · 分支 `mcp5-wsform05` · 基 `origin/main` `11e6111a` · worktree `D:\tops\idp-parcel-mcp5-wsform05`
Blocked by: 无
地盘：审计段只读全仓 `apps/admin-web/src/pages/**`；改动段落 `apps/admin-web/src/components/`（新 `components/action-feedback.ts` 纯逻辑 + 可能的 `components/ActionButton.tsx`）与
各页写动作调用点（只改「按下 → 请求 → 结果」那几行，不改页面结构）。**不碰** `templates/*`、`shell/*`、`Layout.tsx`、`pages/party/**`（它已用 `useToast`，作为对照基线不动）。
出处：spec「缺口」表第五档；蓝图 21.1「Every action must have feedback：Success / Failure / In progress / Queued / Partial success」、21.2「High-risk actions need confirmation」；
参照 `idp-ui@6751fb2` `apps/myshop-web/src/shell/RightSidebar.tsx` 快捷操作 → `addToast`、`components/ActionModal.tsx`（确认 + 表单 + 反馈一体的动作弹层）。
**参照物里的反馈全是假的**（按钮直接 `addToast('操作成功')`，没有请求），只搬「每个动作三态可见」这条规则，不搬实现。

## 为什么

本仓写面不少（`postMasterData` 的调用点：登记、发布、停用、决定、评估……），反馈各自为政：`pages/party` 用 `useToast`，别处有的行内显结果代数、有的什么都不显、
「进行中」多半没有禁按钮。蓝图 21.1 说的是**用户视角的完备性**：按下去之后必须知道成了没、还在等、还是坏了；21.2 说不可逆的要拦一下。本票不改任何语义，只把这两条铺平。

## 要做的

1. **审计段（先做，单独一笔，只读）**：列全 `apps/admin-web/src/pages/**` 里所有发 `POST` / 非幂等请求的调用点（从 `postMasterData` 与各 `api.ts` 的写函数反查，
   用 `rg` 不凭记忆），每处四格：**成功反馈**（toast / 行内 / 无）、**失败反馈**（toast / 行内 / 无 / 只 console）、**进行中**（按钮禁用 + 文案 / 无）、**是否高风险**
   （停用 / 撤销 / 覆盖 / 删除 / 不可逆发布 → 是）与**有没有二次确认**。表落进本票面「审计对照表」小节，带取证 SHA。`pages/party` 也列，作对照基线。
2. **`components/action-feedback.ts` 纯逻辑**：`feedbackFor(result: ApiResult<unknown>, verb: string)` 把结果代数（`outcome` / `problem` / `transport`）映到统一三态文案
   （成功：「已<动词>」；失败：`problem.detail` 或传输错原文，不改写；进行中：「<动词>中…」），node:test 钉三态 + 边界（`problem` 无 detail、`transport` 空 message）。
   文案里的动词取调用点已有的按钮文字，不自造。
3. **改动段**：对照表里「成功或失败无反馈」的调用点接 `useToast`（`ToastProvider` 已在 `App.tsx`）；「进行中无禁用」的加 pending 态（按钮 `disabled` + 文案）；
   高风险且无确认的接 `ConfirmDialog`（`@idpxyz/ui-primitives` 已有，第一轮 02 在用），确认文案写明**删的 / 停的是什么、影响谁**，不写「确定吗」。
   每页一笔或按上下文分组成笔，提交信写「对照表第 N 行」。
4. **不统一成弹层**：myshop-web 的 `ActionModal` 把确认 + 表单 + 反馈揉成一件；本仓的表单（登记 / 发布）已各有页面位，只补反馈与确认，不换壳。

## 不做

- 不加「排队 / 部分成功」两态——本仓没有异步排队或批量部分成功的端点；对照表里若有就如实标「无此态」。
- 不改结果代数（`ApiResult`）、不改任何 `api.ts` 的请求形状、不改 `problem` 的分类。
- 不碰 `pages/party/**`（已对齐，是基线）；不碰只读页。

## 完成判据

- 审计对照表在票面，行数 = `rg` 实测数（写「实测于 `<sha>`」），每行四格填满；无反馈 / 无确认的行在改动段结束时全部标「已补 `<sha>`」或「不补 + 理由」。
- `action-feedback.test.ts` 三态 + 边界绿；四道门绿；改过的页 `vite build` 产物含新确认文案字面量。
- 浏览器验收做不到如实写「未验」；一次性 esbuild 束断言至少一处：失败结果渲染出 `problem.detail` 原文。

## 审计对照表（通道 5，实测于 `11e6111a`；码面与认领笔 `ef39faf7` 逐字节同）

**量法**：`rg` 在 `apps/admin-web/src` 反查两类写函数的全部调用点——(a) 所有 `import` 了 `postMasterData` 的 `pages/**/*api*.ts` 里导出的写函数
（`registerVisibilityCatalogue` / `registerExternalFundsFact` / `registerPriceCard` / `registerReferenceSeries` / `reviewReferenceSeries` / `previewReferenceSeries` /
`registerReferenceSeriesPayload` / `previewCommercialPublication` / `submitPublicationDraft` / `approvePublicationDraft` / `publishPublicationDraft` / `registerCommercial` /
`judgeEffectiveTime` / `registerNetworkCatalogVersion` / `registerCustomsConfiguration` / `registerDutyReconciliation`）；(b) `pages/shipment-request/api.ts` 自带 `post` 的六个
（`submitShipmentRequest` / `withdrawShipmentRequest` / `cancelParcel` / `disposeShipmentRequest` / `completeManualReview` / `rejectShipmentRequest`）。全仓 `method: 'POST'` 另有一处在
`auth/oidc.ts`（令牌换取，不是页面动作），不入表。命中的非 `*api*.ts` 文件里去掉 `import` 行，剩下的每一行就是一行；共 **32 行**。

**四格判据**：成功 / 失败反馈看 2xx 与非 2xx 各自在页面上渲成什么；进行中看请求在途时按钮是否 `disabled`、文案是否换；高风险按票面口径
（停用 / 撤销 / 覆盖 / 删除 / 不可逆发布 → 是）；二次确认指按下之后、请求发出之前另有一道要人再点一次的门（理由必填不算，它是留痕门槛不是确认）。

**先说结论**：32 行**没有一行**成功或失败无反馈——全部行内渲染结果代数（`RegistrationAnswerNote` / `ResultPanel` / 各面板自己的 `AnswerNote` / `commandNote`），
票面「为什么」里「有的什么都不显」这一句在 `11e6111a` 上不成立；`pages/party` 的 `useToast` 只用在 `useCopyToClipboard`（复制到剪贴板），写动作反馈与别处一样是行内，
spec 把它当 toast 基线是误读，但它仍是「反馈完整」的基线。进行中 32 行全部 `disabled`，其中 **3 行**按钮文案不换（下表标 ◑）。高风险 **7 行**，其中 **6 行无二次确认**，
4 行在本票地盘、2 行在 `pages/party`（不动）。「排队 / 部分成功」两态：`CancelParcelPage` 逐件受理、逐件出卡，部分成功由此自然表达，不另加态；其余无此态。

共享路径先钉一次，下表引名不复述：

- **R** = `components/registration/RegistrationPanel`（含 `MultiRegistrationPanel` 转发）：成功行内（`RegistrationAnswerNote` 逐格译 outcome / refusalReason / cause / declarations）、
  失败行内（`callerProblem` 含 `detail` 原文、`noAnswer`、`transport` 原文、`unconfigured` 整段说明）、进行中 `disabled` + 「提交中…」。登记册只追加：更正翻旧插新、停用走状态推进，
  没有覆盖 / 删除，故登记本身不算高风险。
- **F** = `pages/party/party-registration-fields` 的 `useRegistrationForm`：状态与 R 同一份 `RegistrationPanelState`，答案渲染复用 `RegistrationAnswerNote`；进行中 `locked` + 「提交中…」。
- **T** = `templates/ReviewFlowTemplate`：决定按钮 `decisionPending` 时 `disabled` + 「决定提交中…」，理由必填；结果由页面 `commandNote` 行内渲染。
- **S** = `pages/shipment-request/ResultPanel`：`pending` 时整块「等待服务端答复…」；`callerProblem` 渲 `problemNote(code)`（该 `api.ts` 的 `ApiResult` 没有 `detail` 格，不是漏渲）。

| # | 文件 · 调用点 | 成功反馈 | 失败反馈 | 进行中 | 高风险 | 二次确认 | 处置 |
|---|---|---|---|---|---|---|---|
| 1 | `visibility/TrackingJudgmentRulesPage` · `registerVisibilityCatalogue(candidate, snapshot)` | R 行内 | R 行内 | R 禁用+文案 | 否 | 不需要 | — |
| 2 | `visibility/DisclosurePoliciesPage` · `registerVisibilityCatalogue(candidate, snapshot)` | R 行内 | R 行内 | R 禁用+文案 | 否 | 不需要 | — |
| 3 | `visibility/ClaimPrerequisitesPage` · `registerVisibilityCatalogue(candidate, snapshot)` | R 行内 | R 行内 | R 禁用+文案 | 否 | 不需要 | — |
| 4 | `shipment-request/WithdrawShipmentRequestPage` · `withdrawShipmentRequest(buildDraft(form))` | S 行内 `WithdrawalOutcomeCard` | S 行内 | ◑ 禁用，文案不换（S 另显等待块） | **是**（撤回终止整份委托） | **无** | 补 `ConfirmDialog` + 进行中文案——已补 `caed9588` |
| 5 | `shipment-request/SubmitShipmentRequestPage` · `submitShipmentRequest(buildDraft(head, parcels))` | S 行内 `SubmitOutcomeCard` | S 行内 | ◑ 禁用，文案不换 | 否 | 不需要 | 补进行中文案——已补 `caed9588` |
| 6 | `shipment-request/CancelParcelPage` · `cancelParcel(buildDraft(form, parcelId))`（逐件） | S 行内逐件 `CancellationOutcomeCard` | S 行内逐件 | ◑ 禁用，文案不换（另显「正在逐件请求」） | **是**（取消包裹） | **无** | 补 `ConfirmDialog` + 进行中文案——已补 `caed9588` |
| 7 | `shipment-request/AuthorizedDispositionPage` · `disposeShipmentRequest({…choice…})` | 行内 `commandNote`（`dispositionCommandNoteOf`） | 行内 `commandNote` | T 禁用+文案 | **是**（`REJECT`「拒绝」；`CUSTOMER_SUPPLEMENT` 否） | **无**（理由必填，非确认） | `REJECT` 补 `ConfirmDialog`——已补 `fef2958c` |
| 8 | `shipment-request/AcceptanceReviewPage` · `completeManualReview({ shipmentRequestId, reason })` | 行内 `commandNote`（`commandNoteOf`） | 行内 `commandNote` | T 禁用+文案 | 否（只落留痕，决定由下一轮形成） | 不需要 | — |
| 9 | `shipment-request/AcceptanceReviewPage` · `rejectShipmentRequest({ shipmentRequestId, reason })` | 行内 `commandNote` | 行内 `commandNote` | T 禁用+文案 | **是**（「主动拒绝委托」） | **无**（理由必填，非确认） | 补 `ConfirmDialog`——已补 `fef2958c` |
| 10 | `settlement/SettlementApplicationPage` · `registerExternalFundsFact(kind, snapshot)` | R 行内 | R 行内 | R 禁用+文案 | 否 | 不需要 | — |
| 11 | `pricing/SeriesReviewPanel` · `reviewReferenceSeries({…decision, basis})` | 行内 `ReviewAnswerNote` | 行内（`callerProblem` 不显 `detail`） | 禁用+「提交中…」 | 否（复核是版本之旁的独立事实；`RETURNED` 可再登记） | 不需要 | — |
| 12 | `pricing/SeriesRegistrationForm` · `previewReferenceSeries(payload)` | 行内 `PreviewNote`（不写库） | 行内（`callerProblem` 不显 `detail`） | 禁用+「预览中…」 | 否 | 不需要 | — |
| 13 | `pricing/SeriesRegistrationForm` · `registerReferenceSeriesPayload(payload)` | 行内 `RegistrationNote` | 行内（`callerProblem` 不显 `detail`） | 禁用+「登记中…」 | 否 | 不需要（预览门在前） | — |
| 14 | `pricing/PriceCardCatalogPage` · `submit={registerPriceCard}` | R 行内 | R 行内 | R 禁用+文案 | 否 | 不需要 | — |
| 15 | `pricing/ReferenceSeriesPage` · `submit={registerReferenceSeries}` | R 行内 | R 行内 | R 禁用+文案 | 否 | 不需要 | — |
| 16 | `party/party-registration-fields` · `registerCommercial(spec.kind, snapshot)`（F；消费方 `LegalEntityRegistrationForm` / `BusinessPartyRegistrationForm` / `PartyRelationshipRegistrationForm` / `IdentityDeactivationForm`） | F 行内 | F 行内 | F 锁定+文案 | **是**（`IdentityDeactivationForm` 停用身份；其余否） | **无** | 不补：`pages/party/**` 是基线不动；停用确认另立票 |
| 17 | `party/ServiceProductsPage` · `registerCommercial('service-product-form', snapshot)` | R 行内 | R 行内 | R 禁用+文案 | 否 | 不需要 | — |
| 18 | `party/PublicationDraftFlow` · `previewCommercialPublication(payload)` | 行内 `PreviewNote` | 行内 | 禁用+「预览中…」 | 否 | 不需要 | — |
| 19 | `party/PublicationDraftFlow` · `submitPublicationDraft(payload)` | 行内 `DraftNote` | 行内 | 禁用+「录入中…」 | 否 | 不需要 | — |
| 20 | `party/PublicationDraftFlow` · `approvePublicationDraft(draftReferenceOf(payload))` | 行内 `DraftNote` | 行内 | 禁用+「批准中…」 | 否 | 不需要 | — |
| 21 | `party/PublicationDraftFlow` · `publishPublicationDraft(draftReferenceOf(payload))` | 行内 `PublicationNote` | 行内 | 禁用+「发布中…」 | **是**（发布即生效） | 有——五步流里「批准」是发布前的独立一步，即流程级确认 | — |
| 22 | `party/PartyContractsPage` · `registerCommercial('customer-account', snapshot)` | R 行内 | R 行内 | R 禁用+文案 | 否 | 不需要 | — |
| 23 | `party/CommercialPoliciesPage` · `registerCommercial('publication', snapshot)` | R 行内 | R 行内 | R 禁用+文案 | **是**（JSON 直发商业发布，`PUBLISHED_EFFECTIVE` 自生效、无批准步） | **无** | 不补：`pages/party/**` 是基线不动；与第 16 行同一张后续票 |
| 24 | `party/ChannelProductCatalogPage` · `registerCommercial('product-channel-mapping', snapshot)` | R 行内 | R 行内 | R 禁用+文案 | 否 | 不需要 | — |
| 25 | `operations/EffectiveTimeJudgmentPage` · `judgeEffectiveTime({ fact, effectiveAt })` | 行内 `JudgmentAnswerNote` | 行内（`callerProblem` 不显 `detail`） | 禁用+「提交中…」 | 否（判断回指新版本，原判断保留） | 不需要 | — |
| 26 | `network/NetworkCatalogPage` · `registerNetworkCatalogVersion(family, snapshot)` | R 行内 | R 行内 | R 禁用+文案 | 否 | 不需要 | — |
| 27 | `network/ServiceAreasPage` · `registerNetworkCatalogVersion('service-area', snapshot)` | R 行内 | R 行内 | R 禁用+文案 | 否 | 不需要 | — |
| 28 | `customs/CustomsRestrictionsPage` · `registerDutyReconciliation(kind, snapshot)` | R 行内 | R 行内 | R 禁用+文案 | 否 | 不需要 | — |
| 29 | `customs/CustomsRestrictionsPage` · `registerCustomsConfiguration('gate-catalog', snapshot)` | R 行内 | R 行内 | R 禁用+文案 | 否 | 不需要 | — |
| 30 | `customs/CustomsPortsPathsPage` · `registerCustomsConfiguration(kind, snapshot)` | R 行内 | R 行内 | R 禁用+文案 | 否 | 不需要 | — |
| 31 | `customs/CustomsCasesPage` · `registerCustomsConfiguration('regulatory-credential', snapshot)` | R 行内 | R 行内 | R 禁用+文案 | 否 | 不需要 | — |
| 32 | `customs/ComplianceRulesPage` · `registerCustomsConfiguration(kind, snapshot)` | R 行内 | R 行内 | R 禁用+文案 | 否 | 不需要 | — |

**顺带看见、本票不改的**：第 11 / 12 / 13 / 25 行的 `callerProblem` 分支渲 `problemNote(code)` 而不渲 `detail`——`catalogue-api` 的 `ApiResult` 有这一格，
`RegistrationAnswerNote` 已经渲它（票 admin-web-group-legal-entities/11），这几处面板各有自己的 `AnswerNote` 没跟上。它属「失败反馈有、少半句」，不是「无反馈」，
不在本票三类处置里；`feedbackFor` 的失败文案正是 `detail` 优先，后续谁把这几处换成消费 `feedbackFor` 就顺手补齐了。

## 完成记录（推送方代落，2026-09-20 20:5x；作者通道 5，码 tip `fef2958c`，基 `11e6111a`）

作者五笔推完（末笔 20:26）后会话未再响应、票面未写完成记录；下表按 `git diff 11e6111a..fef2958c` 与上面的对照表「处置」列代落，不取记忆。

| 笔 | 条 | 落点 |
|---|---|---|
| `ef39faf7` | 认领 | Status → in-progress |
| `9f136811` | 第 1 条 | 审计对照表 32 行五格（量法、四格判据、共享路径 R / F / T / S、先说结论），实测于 `11e6111a` |
| `fdb64b91` | 第 2 条 | 新 `components/action-feedback.ts`：`pendingText(verb)` = 「<动词>中…」；`feedbackFor(result, verb)` 把 `ApiResult`（取 `pages/catalogue-api` 那份，与 `RegistrationPanel` 同一处）映到三态——`null` → pending、`outcome` → answered「已<动词>」（头注写明它只说「已送达并得到答复」，负向答案照样是 answered，业务答案归页面词表）、`callerProblem` → failed（`detail` 原文，空白 detail 视同缺席退 `code（HTTP status）`）、`noAnswer` → failed（`code（HTTP status）`）、`transport` → failed（`message` 原文，空退「请求未到达 parcel-api」）、`unconfigured` → failed（403 那一句）。`action-feedback.test.ts` node:test 7 条 |
| `caed9588` | 第 3 条 · 对照表第 4 / 5 / 6 行 | `WithdrawShipmentRequestPage`：`requestWithdraw` 先校验再开 `ConfirmDialog`（tone danger；`withdrawalConfirmText` 写明委托 / 来源请求 / 提交版本、影响（不再等待决定；原始提交与判断历史不删）、以谁的名义、原因，边界句取 UC-PS-005），确认才 `handleWithdraw`；按钮在途 `pendingText('撤回')`，静态文案「确认撤回整份委托」→「撤回整份委托」（确认已挪进弹层）。`SubmitShipmentRequestPage`：按钮在途 `pendingText('提交')`。`CancelParcelPage`：`requestCancel` 校验 → `ConfirmDialog`（`cancellationConfirmText` 逐件列包裹标识，边界句取 UC-PS-006：逐件裁决允许部分成功、越过取消边界的不回退），确认才 `handleCancel`；在途 `pendingText('取消')` |
| `fef2958c` | 第 3 条 · 对照表第 7 / 9 行 | `AcceptanceReviewPage`：`onDecide` 拦 `reject` → `PendingRejection`（模板交出理由时已 `setReason('')`，故连理由一起攥住）→ `ConfirmDialog`「主动拒绝委托」（`rejectionConfirmText` 引 ADR-0086：当场形成决定、不交下一轮、不会变成接受），确认才 `decide`；取消时 `commandNote`（tone problem）说明理由已清空需重填；「记录复核完成」不拦。`AuthorizedDispositionPage`：拦 `REJECT` → `ConfirmDialog`「按策略拒绝委托」（引 ADR-0132 决定一：去向决定不是对受限控制的表决），「交客户补充」不拦 |

第 4 条：未换壳——五页只加 `ConfirmDialog` 与在途文案，表单位、结果渲染（`ResultPanel` / `commandNote` / 逐件卡）原样。

**完成判据逐条**

- ✅ 对照表 32 行 = `rg` 实测（实测于 `11e6111a`），四格填满；「无确认」7 行里 4 / 6 / 7 / 9 标「已补 `<sha>`」（本次代落），16 / 23 「不补 + 理由」（`pages/party/**` 基线不动，另立票），21 有流程级确认；「进行中无文案」3 行（4 / 5 / 6）已补 `caed9588`。
- ✅ `action-feedback.test.ts` 7 条：在途 / 已答（含负向答案照样 answered）/ detail 原文 / 无 detail 与空白 detail 退码 / noAnswer / transport 原文与空 message / unconfigured——三态 + 边界。
- ✅ 四道门：作者未报（会话中断）；推送方在重放 tip `f8d37cf6`（main `d2f11618` + 04 + 05）上实跑 tsc 0 / run-tests **359**（343 + 04 的 9 + 本票 7）/ vite build 0 / `go test ./internal/architecture/ -count=1` ok。产物含「撤回整份委托」「逐件请求取消」「主动拒绝委托」「按策略拒绝委托」各 1 处；「撤回中…」「取消中…」在产物里为 0 处是因为它们由 `pendingText` 运行期拼出，不是漏。
- ◑ 一次性 esbuild 束「失败结果渲染出 `problem.detail` 原文」：**推送方代跑、一次性实测、组件层未钉**（照第一轮 01 那套束，`renderToStaticMarkup`）——`RegistrationAnswerNote` 收 `callerProblem{detail}` 时 detail 原文在 DOM、`problemNote(code)` 同在、无 detail 时不渲「原因：」空行；`feedbackFor` 对同一结果给出的失败文案与页面渲的是同一串。4 / 4。源与产物不入库。
- ❌ 浏览器**未验**：五个 `ConfirmDialog` 的弹出 / 确认 / 取消（Radix Portal，静态渲染不出）、在途按钮换字、取消确认后 `commandNote` 的提示。

**判断项**（推送方按代码与头注代写）

1. **`feedbackFor` 今天没有页面消费**：审计发现 32 行全部已行内渲结果代数，票面第 3 条「无反馈的接 useToast」一行都没有，第 2 条要的一处算的三态文案就只被 `pendingText` 那半用上；`feedbackFor` 按票面第 2 条建好、node:test 钉住，等第 11 / 12 / 13 / 25 行那几处各自的 `AnswerNote` 换成消费它时补齐 detail（对照表末段）。
2. **确认正文不写「确定吗」**：五处都写明对象标识、影响、名义 / 理由，边界句引 UC-PS-005 / UC-PS-006 / ADR-0086 / ADR-0132 的口径。
3. **先校验再开确认**（撤回 / 取消两页）：确认层只问「要不要」，表单没填全在它之前就拦下，免得被读成「撤回被拒」。
4. **复核 / 处置两页的确认放在 `onDecide` 之后**：`ReviewFlowTemplate` 交出理由的同时清空输入框，所以待确认的理由要连同目标行一起攥住；取消确认时说一声理由需重填，不让人以为它还在。

## Comments

### 评审 ← 通道 4 · 钉 `fef2958c`（基 `11e6111a`）· 20:4x（推送方接任后自评——本仓当下仅此一个会话在工作，评审者与作者非同一会话，但门在重放 tip 上跑、非隔离检出；读的是作者树 `D:\tops\idp-parcel-mcp5-wsform05`，只读）

门：见完成记录（重放 tip `f8d37cf6` 上 tsc 0 / run-tests 359 / vite 0 / architecture ok；05 七份文件与作者 tip 逐 blob 同）。

**Standards** — 阻断：无。非阻断：
1. `pages/shipment-request/AuthorizedDispositionPage.tsx`：新插的 `PendingRejection` 接口与 `rejectionConfirmText` 落在 `restrictedItemsBlock` 的 JSDoc 与函数体之间——那段「队列行上的受限项，逐条原词直显……」头注现在挂在 `PendingRejection` 上，说的却是 `restrictedItemsBlock`（AGENTS「写代码注释」：注释讲它所在的那段代码；判断项，挪一下位置即可）。
2. `CancelParcelPage.tsx` / `WithdrawShipmentRequestPage.tsx` 的确认正文与其上注释混用半角标点（`(` `)` `:` `;` `,`），同文件与同票另两页（复核 / 处置）用的是全角；用户可见的弹层正文里「(来源请求 …)中的 N 件包裹逐件请求取消:」尤其显眼（判断项，一致性；仓内无成文标点规矩）。
3. `AcceptanceReviewPage.tsx` 与 `AuthorizedDispositionPage.tsx` 各自一份 `PendingRejection` / `onDecide` 拦截 / `confirmRejection` / `cancelRejection` / `<ConfirmDialog>` 五件同形（可能 Duplicated Code，判断项——两页的 entry 类型与决定 id 不同，票面第 4 条又明写不统一成弹层，抬一个小 hook 是可选项不是义务）。
无发现（实核）：注释全中文、引 UC / ADR 用符号名与决定名，无行号无计数；`components/action-feedback.ts` 从 `pages/catalogue-api` 取 `ApiResult` 与既有 `components/registration/RegistrationPanel` 同一方向，不是新引入的反向依赖；不碰 `templates/*` / `shell/*` / `Layout.tsx` / `pages/party/**`（文件清单实核）；未改 `ApiResult`、任何 `api.ts`、`problem` 分类。

**Spec** — 阻断：无。非阻断：
1. `feedbackFor` 建了、钉了、无人消费（票面第 2 条要它存在，第 3 条的消费点因审计为零而不存在）——不是缺陷，但票面「一处算、各页消费」那半今天只成立 `pendingText`；记在完成记录判断项 1。
2. 完成判据「esbuild 束断言失败结果渲出 detail」作者未做，推送方代跑 4 / 4（见完成记录 ◑）。
无发现（实核）：第 1 条 32 行表在票面、四格满、有取证 SHA；第 2 条三态 + 边界 7 条 node:test；第 3 条对照表「处置」列 4 / 5 / 6 / 7 / 9 全部落码、16 / 23 不补 + 理由；第 4 条未换壳；「不做」两态未加、结果代数与 `api.ts` 未动、`pages/party/**` 未动；确认正文写对象 / 影响 / 名义，无「确定吗」；`WithdrawShipmentRequestPage` 按钮静态文案去「确认」二字属确认挪进弹层的必然，不计文案蔓延。

**Standards 0 / 3 · Spec 0 / 2** → 无阻断，可重放。

### 处置（推送方 · 通道 4 接任 · 20:5x）

- Standards 1（头注挂错符号）→ 只动注释位置，推送方代落一笔（见进 main 记录）。
- Standards 2 / 3、Spec 1 / 2 → 记，不挡合入；标点一致性与两页同形五件留作者或下一张触及这两页的票顺手。
