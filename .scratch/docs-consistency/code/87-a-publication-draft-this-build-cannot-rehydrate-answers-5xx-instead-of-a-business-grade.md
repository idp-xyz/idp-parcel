# 87 本构建折不回来的发布载体答成 5xx，而不是业务格

Category: bug
Status: in-progress——2026-10-28 会话 tops-20（idp-parcel-91）接手 tops-26 断在进 main 前的这一张（作者与修订轮已完、评审无阻断），分支 claude-code87 待变基改号。此前 needs-triage——建议 R3：今天受影响的行为空（没有真实租户；演示种子不经载体发布价格规则），但答法违反 ADR-0022 与 ADR-0029，下一次规范化换号或正文合法集合收窄时同病还会复发。2026-10-25 协调方据票 [docs-consistency/code/25](./25-sell-volumetric-factor-and-amount-rounding-are-read-from-the-price-card.md) 隔离评审 N1 裁走 (b)（本票立、code/25 不改代码），作者会话代立（用户「你作为业务和系统专家自决」授权）。
Blocked by: 无
上下文：PC（参与方与商业），领域层载体重建与应用层载体编排，连带 PC 的 HTTP 载体端点与管理台载体流程。
出处：code/25 隔离评审 N1。

## 现象（读码，只记 `S`）

运营操作者面的发布载体（迁移 `party_commercial/0028` 的 `publication_draft` 表）在库里存规范化文档与摘要，读回时经 `domain.RehydratePublicationDraft` 折回正文再算一遍摘要。本构建折不回来的那一行，在录入读回、批准、发布三口都答成技术失败（HTTP 5xx `NO_ANSWER_FORMED`），而重发不会变好——恢复动作是重新录入或换版本号。

两种行今天都会落到这里：

- **带体积系数的价格规则快照**（code/25 之后）：`canonicalPricePolicyCaliber.body` 对带 `volumetricFactor` 的 `PCC-1` 文档点名拒（`domain.ErrVolumetricFactorOnPricePolicy`）。体积系数是价卡内容，政策上不收（ADR-0260 决定二）；0041 迁移之前录入的价格规则载体可能带着它。
- **本构建不认得的规范化版本**：`domain.RehydratePublicationContent` 对别的版本答 `domain.ErrCanonicalizationUnsupported`（ADR-0014）；滚动发布或回滚时旧构建读新构建写的载体正落在这里。`RehydratePublicationDraft` 的注释说「本构建不认识的规范化版本折不回来，那一行如实拒（操作者重新录入即可，载体不是权威记录）」，但「如实拒」今天落成的是 5xx。

按载体状态分三种后果：

- **`待批准`**：批准口与发布口都答 5xx。按同一版本重新录入可以替换那一行（`ports.PublicationDraftRegistry.SubmitDraft` 的 upsert 只在库上是`待批准`且内容不同时替换），之后照常推进——可恢复，但答复没有告诉操作者该这么做。
- **`已批准`未发布**：发布答 5xx；按同一版本重新录入也不行——upsert 只替换`待批准`行，零行命中后 `LoadDraft` 读回同一行，又撞同一个重建错误，录入口同样答 5xx。只能换版本号重录。
- **`已发布`**：重放批准或发布不再答 `DRAFT_ALREADY_PUBLISHED` / `ALREADY_PUBLISHED` 一类的格，而答 5xx。

## 证据（符号名，读于 code/25 分支）

- `domain.RehydratePublicationDraft`：`content, err := RehydratePublicationContent(...)` 之后 `rehydratedDraftRefusal(fmt.Sprintf("正文快照折不回来：%v", err))`——`%v` 丢了错误链，调用方认不出 `ErrVolumetricFactorOnPricePolicy`、`ErrCanonicalizationUnsupported`，只剩 `ErrInvalidRehydratedPublicationDraft`。
- `postgres.PublicationDrafts.LoadDraft` 原样上抛；`postgres.PublicationDrafts.SubmitDraft` 零行命中后的读回同样上抛。
- `application.ApprovePublicationDraftHandler.Handle` 与 `application.PublishPublicationDraftHandler.Handle`：`LoadDraft` 的 err 一律包成 error 返回；`application.SubmitPublicationDraftHandler.Handle` 对 `SubmitDraft` 的 err 同此。
- PC HTTP 载体端点（`commercialhttp.NewSubmitPublicationDraftEndpoint`、`NewApprovePublicationDraftEndpoint`、`NewPublishPublicationDraftEndpoint`）把编排 error 答成 5xx `NO_ANSWER_FORMED`。
- 规则：ADR-0022 的 5xx 意为「答案没形成，退避后重发」，这里重发不会好；ADR-0029 按恢复动作分格，这里的恢复动作是重录或换版本号。同文件 `application.PublishCommercialAuthorityHandler.Handle` 对「客户合同只有壳」答`未受理`而不是 error（ADR-0178 决定一），注释写明答成 error 会被读成技术失败、叫运维重试——同一判据。

## 三类

机制：本构建折不回来的载体答一格业务结论，带成因，不答 5xx。产品策略：这一格在批准、发布、录入三口各叫什么、管理台怎么提示恢复动作。租户取值：无。

## 规模

S–M：领域一处改 `%w` 保链；三只编排各认一个哨兵；批准口封闭词表加一格，连带 HTTP outcome 词与管理台标签；各口一条用例（可借 code/25 `TestMigration0041KeepsHistoricalFactorsListedAndRefusesNewOnes` 的退表手法造一行带系数的载体，或直接落一行不认得的规范化版本）。

## 最小的诚实修法（评审 (a)）

1. `domain.RehydratePublicationDraft` 用 `%w` 保住折不回来的成因，或另立一个哨兵（如 `ErrPublicationDraftNotRehydratableByThisBuild`），包住 `ErrCanonicalizationUnsupported` 与正文构造门的拒因。
2. **发布口**：认出该哨兵即答 `PublicationDraftPublicationNotLanded`，带`未受理`与成因（该格已有，载体不推进）。
3. **批准口**：封闭词表 `ApprovePublicationDraftOutcome` 加一格（如 `DRAFT_NOT_REHYDRATABLE`），HTTP outcome 原名与管理台 `publication-draft-api.ts` 的标签同步，标签写恢复动作（`待批准`按同版本重录；已批准的换版本号）。
4. **录入读回**：`SubmitDraft` 零行命中后的读回撞上该哨兵时，答`内容已固定`或一格新的「既有行折不回来」，不答 5xx；`已批准`行不能就地替换，所以答复要说「换版本号」。
5. 各口一条用例：同一行在三种状态下各答什么。

## 要裁的

- 批准口新格与录入读回那一格的名字与 HTTP 状态（照 ADR-0022：形成了的答案取 200）。
- `已发布`的载体重放时答什么：照旧答「已发布」（列上的状态与发布痕迹读得出，不必折回正文）还是答「折不回来」。前者要让重建门在只看状态的路径上不折正文，改动面大一些；后者如实但会让管理台把一版已在册的发布显成「出错」。
- 是否顺带给 `cmd/parcel-api` 的载体端点加一条真库用例。
