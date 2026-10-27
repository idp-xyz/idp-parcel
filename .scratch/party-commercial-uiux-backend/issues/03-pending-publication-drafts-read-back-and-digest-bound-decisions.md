# 03 待批准发布稿的列表与读回口，批准与发布带期望内容摘要

Category: enhancement
Status: in-progress——作者完工，分支 claude-pcub3，待隔离评审与进 main
Blocked by: 无（ADR-0314 由本票会话代决，待 PC owner 复核）
地盘：`internal/partycommercial` 的载体读口（`ports/publication_draft.go`、`adapters/postgres/publication_draft.go`）、批准与发布用例（`application/publication_draft.go`）、`adapters/http` 的载荷与两个只读端点、`cmd/parcel-api/endpoints_commercial_catalogue.go` 的端点表与装配；管理台新页「待批准发布」（前端跟进，同票）。
出处：[party-commercial-uiux 票 19](../../party-commercial-uiux/issues/19-decision-area-and-body-summary.md) 与 spec 第三段：待批准载体今天只在录入答复里出现一次，换人批准、刷新页面、隔天再来都找不回；批准口只收（类别、对象、版本）三格，批准者看的内容与批准落下的内容可以不是同一份。

## 现象（读码，钉 main `7457d23e9` 之前的 tip）

- `PublicationDraftRegistry` 只有 `LoadDraft` 与具名写；没有任何 HTTP 读口。批准者看不到「有什么在等我批」。
- 批准口载荷三格，`ApprovalDraftChanged`（`DRAFT_CHANGED`）这一格在用例里早已存在，但只在写回时的前态比对里触发，调用方没有办法声明自己看到的是哪一份。待批准阶段同一键再次录入会换掉正文快照与摘要，于是批准者看的是 A 内容、批准落在 B 内容上；内容摘要又不盖版本壳，换范围或区间而正文不变的修订连摘要都不变。
- 发布口同理：已批准的载体发布时，调用方没有办法声明自己要发的是哪一份。

## 要做的

- 两个只读端点，挂操作者查阅读面：`GET /commercial-open-publication-drafts`（列表，不带正文，录入时刻由新到旧；空是 200 加空数组；读失败答 5xx）与 `GET /commercial-publication-draft-readbacks/{kind}/{objectId}/{version}`（单份，带规范化文档；不在册与跨租户答 200 加 `DRAFT_NOT_FOUND`）。路径不用 `/commercial-publication-drafts`：那是录入口，端点表按路径匹配、重复会 panic。
- 批准与发布的命令各多 `ExpectedDigest`（内容）与 `ExpectedShellDigest`（版本壳：范围、区间、指名引用）；用例在读到载体后、判状态前各自比较，任一个不同答 `DRAFT_CHANGED`（批准沿用既有格，发布新增一格）。运营操作者面的 HTTP 载荷两个都必带，缺席或空白逐格问题点名。
- 管理台：新页「待批准发布」（主数据区，紧随商业规则与策略）列出在途载体，点一行打开抽屉读回内容与摘要，批准与发布在抽屉里做，带的是抽屉里读回的摘要；各册发布页签的五步流程从录入答复取摘要带上；`DRAFT_CHANGED` 的批准与发布中文各一句。
- ADR-0314；CONTEXT 里批准那条规则句补「批准与发布都绑定调用方看到的那份内容」；UC-PC-001 待批准载体那一路补读回与摘要绑定，新增 AT-PC-068、AT-PC-069。
- 不在本票：载体撤回 / 作废的口；已发布载体的历史查阅；列表游标。

## 完成判据

- [x] 真库用例：列出租户名下待批准与已批准的载体，录入时刻由新到旧，别的租户与已发布的不在内。
- [x] 应用层用例：批准与发布各自绑定摘要；摘要比较先于状态答案；空摘要不带前置条件。
- [x] HTTP 用例：列表不带正文、空与失败分明；单份读回的文档就是摘要盖住的那份；不在册与跨租户同形；批准与发布缺摘要逐格点名；两个命令端点答 `DRAFT_CHANGED`。
- [x] 端点表与隔离读用例（`cmd/parcel-api`）通过；`internal/architecture` 通过。
- [x] 管理台：演示库里录入两份、模拟另一个操作者修订其一，抽屉点批准得到 `DRAFT_CHANGED` 并随即重读显示新内容，再批得「已批准」、发布得到生效起点未到的一格；另一份自批准到发布走完，列表随之少一行。
- [ ] 全量 `scripts/test-full.sh`、管理台三道门（三道门已过，全量待推送方）。

## 三类

读面与摘要绑定是机制；谁能批准、要不要换人是租户取值（审批职责规则，ADR-0126 已定，本票不动）。

## 规模

M

## 完成记录

- 起点：分支 `claude-pcub3` 自 main 一笔早于 `7457d23e9` 的 tip 起，进 main 前要 rebase。
- 新增：`domain.PublicationDraft.ShellDigest`（`PCS-1`，评审阻断后补）；`ports.PublicationDraftRead`（`LoadDraft`、`ListOpenDrafts`），载体册适配器同时满足；`commercialhttp.PublicationDraftReader`、`NewQueryPublicationDraftsEndpoint`、`NewQueryPublicationDraftEndpoint`（新文件 `query_publication_drafts.go`）；`PublicationDraftReferencePayload.ExpectedDigest`；`application.PublicationDraftChanged`（发布结果 `DRAFT_CHANGED`）；`cmd/parcel-api` 端点表两行带探针，占位 `unwiredPublicationDraftRead`。
- AT：`AT-PC-068`（读回）、`AT-PC-069`（摘要绑定），UC-PC-001 与 `.scratch/at-coverage-inventory.md` 已补行；盘点总览计数与机制清点由推送方在 tip 上重算。
- 既有测试补摘要：批准与发布的载荷现在必带摘要，应用层、HTTP 与装配层共八处旧用例改为从录入答复取摘要再批；这是 ADR-0314 越权风险点 1 点名的收紧。
- 管理台：`publication-draft-api.ts`（读回类型、两个读函数、批准与发布载荷带 `expectedDigest`、两句 `DRAFT_CHANGED`）；`publication-draft-flow.ts` 的 `draftReferenceOf(payload, digest)` 与 `decisionDigestOf`（只取同键的录入答复，拿不到按钮收回）；新页 `PendingPublicationsPage.tsx`、抽屉 `PendingDraftDrawer.tsx`、纯逻辑 `pending-publications.ts`（6 条用例）；答复呈现件从 `PublicationDraftFlow.tsx` 抽到 `PublicationAnswerNotes.tsx` 两处共用；`publicationSummary` 参数放宽为版本壳；导航、页注册与 `liveIds` 各一行。
- 代决与越权风险点：见 ADR-0314（五点），其中点 1（载荷收紧）与点 2（读口挂查阅读授予、不另立读授予）需 PC owner 与访问身份 owner 复核。
- 已知：新页的列表不分页，用既有 `limit`；录入者与批准者只示操作者主体引用原串，没有显示名；批准按钮对没有批准职责的操作者照样亮，按下得到服务端的 `NEEDS_ANOTHER_APPROVER` / `NOT_CONFIGURED` 等答案——按授予隐藏写入口是票 04 的事。

## 评审记录

第一轮 ← 隔离子代理(sonnet) · 钉 `33ba08db2`：阻断一条，非阻断八条，总评需修订。

- **阻断（已修）**：内容摘要只盖正文，不盖版本壳（范围、有效区间、指名引用），而待批准期间壳变了正文没变是合法的就地修订（`SameSubmissionAs`）——批准者看的是范围 S1、批准落在 S2 上，摘要一字不差。修法：载体新增 `ShellDigest()`（`PCS-1`，范围、区间起止、指名引用，时刻取 UTC 微秒），批准与发布命令各多 `ExpectedShellDigest`，运营操作者面载荷两个摘要都必带，列表、读回与录入、批准答复都带 `shellDigest`；管理台抽屉与五步流程带那一对摘要，抽屉里复述指名引用。补用例：域层壳摘要的敏感面与微秒稳定、应用层批准与发布各绑壳、HTTP 载荷两个摘要缺任一个都点名；ADR-0314 补决定与风险点 3。
- **非阻断（已修）**：抽屉里批准或发布的答复回来时核对抽屉身份，不把 X 的答复写进 Y 的抽屉；「读不到」文案不再说「可能刚被发布」；ADR 与票面改正两处与代码不符的说法（`DRAFT_CHANGED` 本就有写回时的前态比对触发；按钮带的是抽屉读回的摘要）；单份读回用例改为与摘要盖住的文档逐节比较；去掉代码注释里的票号与两处变更叙事；`ListOpenDrafts` 的状态条件改绑参数；ADR 风险点 5 补「超限时藏起来的是等得最久的」与「坏行致整页失败、无撤回口」；CONTEXT 新句加「运营操作者面」限定。
- 未改：`ListOpenDrafts` 整页失败与同刻排序没有专门用例（行为由重建门与排序列定，ADR 已点名）。
