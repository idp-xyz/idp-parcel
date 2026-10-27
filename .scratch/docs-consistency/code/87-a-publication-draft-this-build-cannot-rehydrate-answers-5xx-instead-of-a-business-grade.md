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

## 完成记录（2026-10-27，分支 claude-code87，首轮；SHA 见修订轮一节开头）

### 做了什么

- `4c53d8442` 代码与用例：
  - **领域**：`RehydratePublicationDraft` 先校痕迹与壳（壳摘要格取列上的摘要）、再折正文。折正文这一步拒，且成因只有两类（列上的规范化版本本构建不认得；文档带着构造门收紧后拒绝的内容，按 `tightenedGateRefusals` 登记表认，今天只有体积系数）时交回 `*domain.UnreadablePublicationDraft`——同时认 `ErrPublicationDraftNotReadable` 与成因（`%w` 保链：`ErrCanonicalizationUnsupported`、`ErrVolumetricFactorOnPricePolicy`），并带库上剩下的事实（身份、状态与批准 / 发布痕迹、录入者与时刻、列上的规范化版本与内容摘要、壳摘要）；文档自身立不住（非法 JSON、正文缺席、册名不认得、自称的规范化版本与列不一致、必填格为空、集外词、时刻格式不对）、折得回而摘要对不上、痕迹对不上、类别对不上都是 `ErrInvalidRehydratedPublicationDraft`（评审修订轮收窄，见文末）。壳摘要的算法落到 `CommercialVersion.shellDigest`，读得回的载体与读不回的行共用一份。
  - **端口 / 适配器**：`LoadDraft` 合同补读不回的答法（错误链带该值）；`ListOpenDrafts` 改交 `[]ports.OpenPublicationDraft`（整份载体或读不回的事实，二者互斥）；`SubmitDraft` 零行命中后的读回撞上读不回行：已过`待批准`的答 `CONTENT_FIXED`，仍在`待批准`的按错误交回；列表把读不回的行留在结果里。
  - **用例**：批准结果多 `DRAFT_NOT_READABLE`，发布结果多同名一格，结果各带读不回的事实。判序：摘要绑定（列上摘要与壳摘要）→ `已发布`答 `DRAFT_ALREADY_PUBLISHED` → 其余答 `DRAFT_NOT_READABLE`；不读审批职责规则、不交发布用例、一字节不写。
  - **HTTP**：载体响应多 `readable` 与 `unreadableCause`；批准与发布答复在读不回一格带 `status` 与 `cause`、不带摘要；单份读回答 200 加 `DRAFT_NOT_READABLE`（`draft` 只有库上事实，没有 `content`）；读取失败与行本身坏了仍 5xx。`cmd/parcel-api` 只改读口占位的签名（`unwired_orchestration.go`），未动 endpoints.go、main.go、endpoints_test.go、routes.go 与 `cmd/parcel-dispatch/routes.go`。
  - **用例**：领域（`publication_draft_unreadable_test.go`）、真库（列表与点读、体积系数、录入撞读不回行、坏行与重录）、编排、传输各层，另有生产装配真库一条（`cmd/parcel-api/assemble_publication_unreadable_draft_test.go`：真铸造、真操作者 Intake、真事务编排，行改成 `PCC-9` 后批准与发布答 200 加 `DRAFT_NOT_READABLE`、按同版本重录答 `DRAFT_REVISED`、之后读得回）。
- `d9a974bca` 管理台（`pages/party/**`，未碰共享件）：类型多 `readable` / `unreadableCause`；批准与发布答复栏补 `DRAFT_NOT_READABLE` 的中文（`draftNotReadableWords`），发布答复栏显示状态与成因；待批准页状态旁标「本构建读不回」、计数摘要另报、检索认得；抽屉（`PendingDraftDrawer.tsx`）不摆批准与发布、不给规范化文档，`UnreadableNote` 按状态说恢复动作（`unreadableRecovery`）。纯逻辑 3 条 node:test。
- `978b51a5c` 文档：ADR-0333（部分停用（收窄）ADR-0314 越权风险点 5 一句，被停原句逐字相同，ADR-0314 Status 注记与 Links 各加前向指针）；`CONTEXT.md`「待批准发布」词条补读不回一格；UC-PC-001 的 `AT-PC-068` 读失败一句改口径，新立 `AT-PC-071`（读不回的载体在读面与批准、发布上的答案）、`AT-PC-072`（录入撞上读不回行的落点）；AT 盘点两行加文末追记。

### 代决（用户授权「你作为业务和系统专家自决」，协调方派单）

1. **「读不回」只收两类成因：列上的规范化版本本构建不认得；文档带着「更早的构建合法、此刻构造门拒绝」的内容（登记表 `tightenedGateRefusals`，今天只有体积系数）。** 其余折不回的情形在任何构建下都不合法，是数据损坏，仍 5xx。理由：前者的恢复动作在操作者手里（重录 / 换版本号），后者要去查库，ADR-0029 按恢复动作分格。初版把折正文的任何拒绝都收进读不回，评审指出会把真损坏答成 200 且「同版本重录」对它们不灵，已收窄（见文末修订轮）。已有用例 `TestServiceProductSnapshotRehydratesWithoutABody` 的伪造快照（类别不符）因此是行坏了，未改。
2. **不用票面设想的「只包一个哨兵」，用哨兵加类型化值。** `ErrPublicationDraftNotReadable` 是哨兵，`UnreadablePublicationDraft` 是带事实的错误值：已发布要如实答、摘要要继续绑定、列表要列出它，这三件都需要库上的状态与摘要，只有哨兵就得回头再读一次或丢掉事实。事实在重建门里校过（痕迹与壳先于正文），上层不碰正文。
3. **批准口新格 `DRAFT_NOT_READABLE`，HTTP 200；答复带 `status` 与 `cause`，不带内容摘要 / 壳摘要 / 规范化版本。** 读不回的载体没有可批准的内容，摘要若被原样交回去批，批的是调用方看不到的东西。
4. **发布口不复用 `PUBLICATION_NOT_LANDED`，新增同名 `DRAFT_NOT_READABLE`。** 前者的语义是发布用例答了未决 / 冲突 / 未受理之一、载体留在已批准、续办照发布用例的答案；读不回的载体没有发布用例的答案可带，恢复动作是重录或换版本号（票面最小修法 (a) 第 2 条在此改判）。
5. **`已发布`的读不回行重放批准或发布答 `DRAFT_ALREADY_PUBLISHED`（票面「要裁的」二，取前者）。** 权威记录早已是那一册里的商业版本，答「出错」会让管理台把已在册的发布显成故障。改动面比票面预计的小：载体册读行时本就扫了状态与摘要列，重建门把它们随错误值交回即可，不必让「只看状态」的路径绕开重建门。摘要绑定先于它（调用方带的摘要与列上的不符答 `DRAFT_CHANGED`），与 ADR-0314 决定二的判序一致。
6. **`已批准`的读不回行不答 `DRAFT_ALREADY_APPROVED`；`待批准`的读不回行发布不答 `DRAFT_NOT_APPROVED`。** 两个状态答案都会把操作者引向一次必败的下一步（发布、批准），读不回的这一格先于它们。
7. **录入口不新增格，读不回行已过`待批准`的答 `CONTENT_FIXED`（票面「要裁的」一后半）。** 恢复动作同为换版本号，ADR-0029 不另立；读不回的行不可能与这次录入是同一次录入（同一次录入的摘要本构建就算得出来），所以不会把「重放」答成「已固定」。`待批准`的读不回行由登记册既有的修订替换（摘要不同），不必改写入条件。仍在`待批准`而修订落空（摘要相同、文档却折不回）只可能是行坏了，按错误交回，不装成内容已固定。
8. **草稿读不回时摘要绑定照旧（票面外、tops-54 点名的兼容点）。** 用列上的内容摘要与按壳列算出的壳摘要比较，先于状态答案；批准与发布的答复不带读不回行的摘要（决定 3）；读面的事实照带列上摘要。`expectedDigest` / `expectedShellDigest` 载荷、`operator-grants`、`code/params` 与两个读口的 digest 字段语义都没动。
9. **列表带着读不回的行（位置按录入时刻不变），不单列数组、不跳过；每份多一格 `readable`。** 单份读回答 200 加 `DRAFT_NOT_READABLE`，`draft` 带事实、无 `content`。读不回行的 `canonicalization` 与 `contentDigest` 是列上原值。行本身坏了与存储读失败仍整次 5xx（ADR-0314 决定一不动）。
10. **管理台：状态旁标记、抽屉不摆动作并按状态说恢复动作，不碰共享件。** 动作按钮按 `draftDecisions`（读不回一概不摆）；服务端仍是最后一道。
11. **顺带给 `cmd/parcel-api` 加一条真库用例（票面「要裁的」三）：加。** 装配点的事务编排对「业务格不回滚、重录能换掉行」这件事有独立的证据价值。
12. **ADR / AT / CONTEXT：** 用预留号 ADR-0333；AT 取 `AT-PC-071`、`AT-PC-072`（核过在途分支无占用）；`AT-PC-068` 的说法按新行为改，不挂票号。无迁移（读不回的行就在 0028 的表里）。

### 越权风险点

1. **对 ADR-0314 越权风险点 5 的收窄**（PC owner）：读不回的行不再让整页答 5xx；运维若靠「待批准列表 5xx」发现坏数据，这一类不再触发，改看 `readable:false` 的行与成因。已按部分停用走 ADR-0333。
2. **读不回的边界是一张登记表加一个版本集合，不是错误类别**（PC owner）：以后新收紧一道会拒存量快照的构造门，要同笔登记，漏登的后果是那一类存量行答 5xx（偏安全的一侧）；同一规范化版本号下文档形状的变化表现为摘要对不上，也答 5xx（ADR-0014 要求形状变化换号）。
3. **`已批准`的读不回载体只给「换版本号」**（PC owner、运维）：滚动发布窗口里旧构建实例读不回新构建写的行，新构建铺满后它自己就读得回，那时「换版本号」是过度建议；分辨窗口与永久读不回要知道各构建实例认得的规范化版本集合，不在本票。
4. **`已发布`的读不回行答已发布时只核对摘要、不看正文**（PC owner）。
5. **答复里的 `cause` 是领域错误原文**（PC owner）：与录入`未受理`的 `cause` 同一可见范围，不是稳定的机器码，管理台只原样显示。
6. **`ports.PublicationDraftRead.ListOpenDrafts` 的返回类型变了**（装配 owner）：唯一的实现是载体册适配器，唯一的其他实现者是 `cmd/parcel-api` 的未接线占位，已同步；日后另有实现者要交 `OpenPublicationDraft`。

### 验证（首轮；修订轮的结果见文末）

- 先写用例再实现：领域 / 真库 / 编排 / 传输各层用例随实现同批落地，下列变异证明它们会红。
- 变异（改一处、跑用例、从备份拷回并 `cmp` 一致）：M1 折正文失败改按行坏了处理 → 领域 2 条、编排 3 条红；M2 批准口把「已发布」判在摘要绑定之前 → `TestTheDigestBindingStillHoldsOnAnUnreadableDraft` 红；M3 发布口把`已批准`的读不回行当已发布 → 编排与传输各 1 条红；M4 录入撞读不回的待批准行也答内容已固定 → `TestABrokenDraftRowStillFailsInsteadOfBeingTreatedAsNotReadable` 红；M4b 录入撞已批准 / 已发布的读不回行不答内容已固定 → `TestSubmittingOverAnUnreadableDraftRevisesAPendingOneAndFixesTheRest` 红；M5 列表悄悄跳过读不回行 → 真库列表用例红；M6 列表读得回的行丢 `readable` → 传输用例红；M7 批准答复丢读不回的状态与成因 → 传输用例红；M8 单份读回不认读不回、答 5xx → 传输用例红；M9 读不回行的壳摘要恒空 → 领域、真库各红。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh` 跑 `./cmd/parcel-api/ ./cmd/parcel-commercial/ ./cmd/parcel-dispatch/ ./cmd/parcel-settlement-register/ ./internal/partycommercial/...` 与 `go list -test` 算出的反向依赖（networkrouting / nodeoperations / parcelshipment / settlementaccounting / transportfulfillment / visibilityexception 的 partycommercial 适配器，`transportfulfillment/application`）、`./internal/architecture/...`、`./migrations/...`：全绿。新用例 `-count=3` 绿；没有新锁、新写者与并发用例，不跑 `-count=5`。
- `go build ./... && go vet ./...` 绿，`gofmt -l` 无输出；`go run ./tools/adr-index`（生成后还原 README）格式无错；`go run ./tools/at-coverage-counts -check` 退 1（仅总览过期，无缺分桶行），总览与 `docs/adr/README.md` 由进 main 一方统一重生成。
- 管理台：`tsc -b --noEmit`、`node scripts/run-tests.mjs`（806 通过）、`vite build` 绿；`./internal/architecture/...`（含 `TestEveryAdminWebPathIsOnTheParcelAPIEndpointTable`）绿；软链与产物已删。
- 无迁移，故不灌种子。

## 评审记录与修订轮（2026-10-27，分支 claude-code87，rebase 于 origin/main `c0b66cf49`；代码改动 `87c51bf18`，文档改动 `e5b531ca2`；下文首轮 SHA 均为本次 rebase 后的）

独立评审判可合入、无阻断；分格次序、批准绑定攻击序列、ADR-0025 落点、管理台范围、ADR-0222 合规都核过，不重做。建议合入前修 N1–N5，本轮全修。

- **N1 分格边界太宽（最要紧）。** 评审在真库把待批准信用政策行的 `content_document` 改成缺正文 / `[]` / `"x"` / 未知 kind / 自称版本与列不一致 / 删必填格，五种全落「读不回」（200 业务格）；而「同版本重录」的修订条件只比摘要与壳，对它们不灵，代码与注释自相矛盾。修法：
  - **新边界。** 读不回只收两类：① 列上的规范化版本不在本构建支持集合里（`!supportedPublicationCanonicalization(spec.Canonicalization)`，不论文档长什么样）；② 折正文的拒因命中 `tightenedGateRefusals` 登记表（按哨兵认，不按文本；今天只有 `ErrVolumetricFactorOnPricePolicy`）。**仍 5xx（`ErrInvalidRehydratedPublicationDraft`）的**：非法 JSON 与形状不符、正文缺席、册名不认得（含未知 kind）、文档自称的规范化版本与列不一致（PCC-2 对 PCC-1 等）、必填格为空、封闭集集外词、时刻格式不对，加上原有的痕迹 / 壳 / 类别 / 摘要对不上。「必填格缺失」按判据裁：在任何构建下都不合法，是损坏，不进登记表。
  - **(4) 做了：`SubmitDraft` 的修订条件多比 `content_document IS DISTINCT FROM`。** 评估：摘要是文档的派生，读得回的行文档与摘要必然相配，重放的文档（jsonb 归一化后）逐位相同，所以正常行的重放 / 修订判定一字不变（`TestSubmittingAgainDistinguishesReplayRevisionAndFixedContent` 与新用例里的重放断言守着）；只有「文档与摘要对不上而摘要列、痕迹正常」的坏行会因此被同内容重录改好，对「一份内容只能被批准一次」与摘要绑定无影响（批准要读得回，坏行读不回；修订更新录入者与时刻，与任何一次修订相同）。已批准 / 已发布的坏行内容已固定，录入按错误交回，要查库。
  - **注释与承诺对齐。** `SubmitDraft`、`ports.PublicationDraftRegistry`、领域 `ErrPublicationDraftNotReadable` / `RehydratePublicationDraft` 的注释改成同一口径；ADR-0333 决定一、三与候选与反方、风险点 2（改写）与 5（新增修订条件）、CONTEXT 词条、UC 的 `AT-PC-071` / `AT-PC-072` 行、管理台 `unreadableRecovery` 注释都限定到两类成因；ADR-0314 注记的适用场景同步。
  - **用例。** 领域：`TestADocumentFlawNoBuildAcceptsIsABrokenRowNotANotReadableOne`（八种文档瑕疵加集外方向词，均为行坏了）、`TestOnlyAnUnknownColumnVersionOrATightenedGateMakesARowNotReadable`、`TestABrokenRowIsStillRefusedAsBrokenNotAsNotReadable` 收紧；真库：`TestABrokenDraftRowStillFailsAndAPendingOneIsHealedByResubmittingTheSameContent`（八种坏文档点读 / 列表答 5xx，同内容重录答 `DRAFT_REVISED` 且读得回，重放仍 `DRAFT_REPLAYED`，已批准坏行录入按错误交回）、`TestAnUnknownColumnVersionIsNotReadableWhateverTheDocumentLooksLike`。
- **N2 成因文字没被钉住。** HTTP 列表、单份读回、批准、发布四处断言成因正是领域成因（`unreadable.Cause().Error()`），不含哨兵那句英文（新增 `assertCauseIsTheDomainCause`）。
- **N3 指针不对称。** ADR-0314 注记去掉「决定二的摘要绑定按口解」半句，Links 一条同改；ADR-0333 对决定二写成「不停用，依它的判序办」，不再用「按口解」。
- **N4 「不交摘要」说法。** ADR-0333 候选与反方改写为「批准与发布的答复不带摘要，读面的事实照带列上摘要」，完成记录代决 3 / 8 同口径。
- **N5 规范小项。** 测试注释「状态与痕迹照旧」、ADR 标题与 Status 的「摘要绑定照旧」、抽屉注释「上面的事实」、CONTEXT 词条「仍是技术失败」，以及本分支新增行里其余不必要的「仍」，都改成现行规则句。

变异（备份 → 改 → 跑 → 拷回 → `cmp`）：N1-M1 折正文任何拒绝都收进读不回 → 领域、真库各 1 条红；N1-M2 改按 `ErrCanonicalizationUnsupported` 认（自称版本与列不一致混进来）→ 领域、真库红；N1-M3 去掉「列版本不认得」一类 → 领域 2 条、编排 3 条红；N1-M4 登记表清空 → 领域、真库体积系数用例红；N1-M5 修订条件去掉文档 → 真库「同内容重录改好坏行」红；N2-M6 / M7 / M8 列表 / 批准 / 发布的成因换成整个错误值的文字 → 传输用例红。
