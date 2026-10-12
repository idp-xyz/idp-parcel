# 06 管理台「草稿」签

Category: enhancement
Status: resolved——2026-10-12 通道 5 收口进 main（新会话接通道 1 派单 `task-e6763faa`，接替 `task-9c227f05` 未完的部分；第一笔 `00bc85ce`，本笔补草稿册故障格的说法并收口）；完成记录见文末。此前 in-progress——2026-10-12 通道 5 认领（通道 1 派单 `task-9c227f05`），前端切片，在共享树 `main` 上做（基 `b298cd8f`）。此前 ready-for-agent——2026-09-25 通道 3 立票并激活（用户授权自决）
Blocked by: 04、05（均已 resolved）
地盘：`apps/admin-web/src/pages/pricing/`。
出处：[spec](../spec.md)；ADR-0101 Consequences 第二条。

## 要做的

1. 价卡页新增「草稿」签：
   - 列表按 03 的查阅读口取，示状态、源文件身份、摘要；
   - 录入者与批准者两列并排；
   - `草稿` 行展开逐格问题。
2. 批准与发布两个动作，分别对 04 的两个口。
   - 答复逐格如实呈现：`未配置` 的审批职责规则、主体相同、授予不足、登记用例的各格答复。
   - 动作按钮只在对应状态出现。
3. 四态如实，同 05。

## 验收

同 05：三道门，加浏览器验证（真后端 403，其余用真适配器序列化的响应注入）。

## 形态

前端切片，在 `main` 上做。

## 完成记录（2026-10-12，通道 5；前端切片，共享树 `main`；证据只记 `S`）

**落点**

- `00bc85ce`（上一个通道 5 会话，`task-9c227f05`）：草稿签 `PriceCardDraftsPanel`；`api.ts` 的草稿查阅读口与批准、发布两口；`price-card-import.ts` 的两口答案代数、`draftActionOf`、`draftActionNoteOf`、`withProgressedDraft`、`draftListViewState`；价卡页加「草稿」签。本会话逐条核过「要做的」1–3，代码都已在这一笔里。
- 本笔：`draftListViewState` 在没形成答案时按码换成草稿册自己的说法（`draftRegisterNoAnswerNotes`），见下「验证中发现并补上」。

**判据逐条**

- ✅ 要做的 1：
  - 列表打 `GET /pricing-price-card-draft-views`，不带查询串。列为定价方案、状态、源文件身份、内容摘要、录入者、批准者、操作。
  - 状态示中文与原名，`已发布`另示发布时刻；源文件身份示文件名与 SHA-256 前缀；摘要示内容摘要前缀与规范化版本，`草稿`行示「没过构造门，没有摘要」。
  - 录入者与批准者两列并排，各带时刻；没批准的示「—（未批准）」。
  - `草稿`行展开逐格问题，与导入签同一张 `ProblemTable`。注入乱序四条（含同格两条），页面排成 `card/2/baseChargeCode` → `rates_weight_zone/4/amount` → `tables/3/currency` 两条，同格两条保持到达顺序。
- ✅ 要做的 2：
  - 批准打 `/pricing-price-card-draft-approvals`，发布打 `/pricing-price-card-draft-publications`。浏览器发出的这两口请求体键集只有 `planId,planVersion`，`Content-Type: application/json`。
  - 答复逐格如实，下列各格都在浏览器里点过：
    - 批准答 `NOT_CONFIGURED`（审批职责规则未配置，不默认单人或双人）、`NEEDS_ANOTHER_APPROVER`（主体相同）、`APPROVER_NOT_QUALIFIED`（授予不足）：答复接在那一行下，行不动，「批准」仍在。
    - 批准答 `DRAFT_APPROVED`：带回的行就地换上，批准者与批准时刻出现，动作换成「发布」。
    - 发布答 `DRAFT_PUBLISHED`（HTTP 201），后接登记那一格 `RECORDED —— 已入册`：行就地换成`已发布`，按钮消失。
    - 发布答 `PUBLICATION_NOT_LANDED`，后接登记那一格 `CONTENT_CONFLICT` 的原名与中文：行留在`已批准`。
  - 动作按钮只在对应状态出现：`已校验`给「批准」，`已批准`给「发布」，`草稿`只给展开，`已发布`没有按钮。请求在途时「刷新」与全部动作按钮禁用，按下的那颗示「批准中…」或「发布中…」。
  - 渠道各格：
    - 批准真打到 parcel-api，答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`，行下呈未配置一句。
    - 注入 401 呈「需要登录」，带一颗「重新登录」；403 `OPERATOR_NOT_GRANTED` 呈「没有授予」，不带按钮。
    - 发布答 500 呈「未形成答案」，写明登记与否未知。
  - 刷新清掉上一轮答复，各行按册上重读的样子摆。
- ✅ 要做的 3：四态（加载、空、错误、未配置）与渠道另两格。
  - 真后端答 403：页面只呈现未配置，说的是草稿查阅读口、不是「册上没有草稿」，不画表。
  - 加载中「刷新」禁用；空册呈空态（`PRICE_CARD_DRAFTS_LISTED`，不是故障）；401 与 403 未授予各成一态。
  - 500 `NO_ANSWER_FORMED` 与 `INTAKE_FAILED` 呈错误态，带「重试」，点了即重读；传输失败呈「无法连接主数据读取服务」，带「重试」。
- ✅ 验收·三道门（共享树，本笔内容，Node 22.23.3）：`tsc -b --noEmit` 退 0；`run-tests` 517/517；`vite build` 退 0。
- ✅ 验收·浏览器验证，见下节。

**浏览器验证（Cursor 内置浏览器，证据 `S`）**

- **托管与登录**：本笔内容的 `vite build` 产物由仓外一次性服务托管，做法同 [operator-channel/07](../../operator-channel/issues/07-admin-web-login-gate.md) 的进度记录。`/oidc/oauth2/token` 回合成令牌（前端只解 id_token、核 state 与 nonce），注入一份已知的 PKCE 中间态后从 `/auth/callback` 进来。令牌交换带的正是那份 verifier 与本地回调地址；登录后浏览器发出的每个 `/api` 请求都带 `Authorization: Bearer <合成令牌>`，`sessionStorage` 与 `localStorage` 里查不到令牌原文。
- **真后端**：parcel-api 取 `ffb8a0d8` 构建。共享树当时没有未提交的 `.go` 与 `.sql`，`00bc85ce..ffb8a0d8` 只有别的通道的文档笔。它连 55432 上一只临时库（迁移助手施加全部 208 步，验完即删），不设操作者发行方参数，启动日志写明操作者族答 `ACCESS_CHANNEL_NOT_CONFIGURED`。托管服务把 `/api/*` 转给它：草稿签首屏真端点答 403，页面呈未配置；`curl` 直打与经托管转发，草稿查阅、批准、发布三口同答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`。
- **其余各格注入**：操作者渠道今天没有真路径能答出这些格，所以注入。注入体不是手写的：
  - 一次性测试经 `go test -overlay` 加进 `adapters/http` 包（不进工作树、未入库），借该包的测试夹具与 `pptest.Plan`，让真端点 `NewQueryPriceCardDraftsEndpoint`、`NewApprovePriceCardDraftEndpoint`、`NewPublishPriceCardDraftEndpoint` 序列化出答复字节。
  - 列表：草稿册七行（`草稿`一行带乱序问题，`已校验`四行，`已批准`与`已发布`各一行）与空册。
  - 批准：上列各格，`DRAFT_APPROVED` 带回推进后的行。
  - 发布：`DRAFT_PUBLISHED`（201，带 `RECORDED` 与推进后的行）、`PUBLICATION_NOT_LANDED`（带 `CONTENT_CONFLICT`）。
  - 渠道与故障：401、403 未授予、500 `NO_ANSWER_FORMED` 与 `INTAKE_FAILED`。
  - 托管服务按方法、路径与载荷里的 `planId` 逐口回放这些字节，请求照常由页面组装。未配置那一格的导出字节与真后端答的逐字节相同。
- **验证中发现并补上**：草稿册答 500 时，页面说明沿用目录页那套判读，写的是「目录读取依赖未能形成答案」，`INTAKE_FAILED` 写「服务端未读取目录」。草稿不进价卡目录，这么说会让人去查一个没出问题的读口。改为按码换成草稿册自己的说法：先写用例见红（实际值即上面那句），再改；浏览器里两格复验。
- **照实记、未改**：发布答 500 那一句读作「……可稍后重试。；登记与否未知，可稍后重试。」。`problemNote` 的 `NO_ANSWER_FORMED` 原文自带「可稍后重试。」，而同目录导入签、试算、序列登记与复核各页的未形成答案句也是「`problemNote`；……可稍后重试」这一写法，本签照的是复核面板那一句的形。这是跨页的写法，不在本票。
- **没在浏览器里走的**：
  - 批准的 `DRAFT_CHANGED`、`DRAFT_NOT_VALIDATED`、`DRAFT_ALREADY_APPROVED`、`DRAFT_NOT_FOUND`，发布的 `DRAFT_NOT_APPROVED`、`DRAFT_ALREADY_PUBLISHED`、`DRAFT_NOT_FOUND` 与登记那一格的 `CANONICALIZATION_DIFFERS`：只由词表用例（两口答案名与服务端原名一一对上、不多不少）兜住，浏览器里没点。
  - 身份依赖不可用（503）：只由本笔新用例兜住。
  - 合成操作者经真发行方登进来、在真后端上批准与发布：要等操作者渠道的演示发行方（[operator-channel/17](../../operator-channel/issues/17-demo-issuer-dex-and-synthetic-operators.md)）与 07 接上，今天端点答 403。
- **披露**：没留截图。内置浏览器窗口太窄，仿真桌面宽度后截图平铺错位，所以证据取页面渲染出的 DOM 文本、按钮状态与托管服务记下的请求。托管服务、合成令牌、导出用例与注入体都在仓外 `/tmp`，未入库。

**评审**：diff 只碰业务页 `pages/pricing/**`，没碰 `templates/`、`shell/`、`components/`，按 workflow「前端切片」做业务页自查，即上列判据。本记录不算非作者评审。
