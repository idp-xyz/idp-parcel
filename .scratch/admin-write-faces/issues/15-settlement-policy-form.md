# 15 `SETTLEMENT_POLICY` 版本的运营主路径：逐字段表单（方式 + 法人 / 对手方 / 合同引用 + 范围 + 币种）

Category: enhancement
Status: resolved——2026-09-08 20:5x 通道 6 交付（task-70599fbd；分支 `mcp6-awf15`，代码 tip `23a4ad59`，已变基到 `mcp4-awf20@802ae400` 之上，逐笔 SHA 与验证强度见文末「完成记录」；main 上的 SHA、非作者评审结论待通道 1 重放后补「进 main 记录」）。完成判据三条全落：政策页多一签「发布结算政策版本」，五步走公共半边、发布落定读面切到结算政策册重读；tsc 退 0 / run-tests 132/132；Go 侧本册接进 PCC-1 一格（连同应用层两个方向与载荷一格）。此前 in-progress——2026-09-08 20:0x 通道 6 认领（通道 1 派单 task-70599fbd；分支 `mcp6-awf15`，基 main `c135049b`，树 `D:/tops/idp-parcel-mcp6-awf15`）。此前 ready-for-agent——形状已裁清（逐字段表单，合同引用对象 + 版本一起选；本票无待裁问题），Blocked by 08 未 resolved 前不在前沿；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面，见伞票 Comments）
Blocked by: 08（已进 main）；[20](./20-publication-vocabulary-read-face.md)（词表读口公共半边，2026-09-08 通道 1 代裁立票；落点已于 2026-09-08 19:5x 广播，前端半边在 `mcp4-awf20@17a962a5`，本票 cherry-pick 它接下拉）

## 册与载荷

显示在**政策页·结算政策册**。`declarations.settlementPolicyBody{method, legalEntity, counterparty, contract{objectId,
version}, chargeScope, currency, effective…}`（0011 正文）。`method` 是封闭集（预付 / 账期一族），`contract` 是一个
客户合同版本的二维引用。

## 选形与理由（ADR-0101 决定八）

**逐字段表单。** 频次低、配置员操作、七格无子表。`method` 下拉由服务端词表读口供；`contract` 从客户与合同目录
**选**（对象 + 版本两格一起选，不让操作者手拼版本号）；币种收 ISO 代码串、存在性由构造门答。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；另一条本册特有：结算政策答的是「怎么结」，不答「要不要接受前控制」——那是 0007 / 0024 两层
（pc-gaps/07 完成记录里 SA 今天从结算方式**推**控制方式那条是 SA 侧另一票的事），表单不在这里长出控制字段。

## 完成判据

结算政策册旁多一签「发布结算政策版本」（表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同册立刻可见；
tsc / run-tests 绿；Go 侧只加本册规范化一格。

## 边界

不动 0011；不动 SA 消费侧。

## 完成记录（2026-09-08，通道 6；分支 `mcp6-awf15`，基 main `c135049b`，后变基到 `mcp4-awf20@802ae400`；task-70599fbd）

**逐笔（分支 SHA，变基后；main 上的 SHA 由进 main 记录补）**：

| 分支 SHA | 内容 |
|---|---|
| `d13a6dad` | 票面认领转 in-progress |
| `90b7fc45` | 领域：结算政策册接进 PCC-1 不换号。新文件 `publication_canonicalization_settlement_policy.go` 放正文类型 `SettlementPolicyBody`（方式 + 六维整体收 `SettlementApplicability`，六维齐不齐仍由 `NewSettlementApplicability` 一处判——`valid()` 把六维原样送回它再判一次，不另写一份）、文档节 `canonicalSettlementPolicyBody`（键名镜像批文 `settlementPolicyBodyDocument`）、折回，与导出反查 `SettlementMethodNamed`。共享文件 `publication_canonicalization.go` 纯加行；既有测试里「没接的册」的样本从 SETTLEMENT_POLICY 换成 CUSTOMER_SERVICE_RULE |
| `82c8caa3` | 应用：`publicationContentOf` 一 case + `declarationsOfContent` 一支，两个方向同笔。甲路下必然的一改：既有结算政策用例的壳改配算出的摘要（新 helper `settlementPolicySpec`）；「未接的册仍收声明串」等三处样本换成 CUSTOMER_SERVICE_RULE。新测试 `publication_settlement_policy_test.go` |
| `7d086085` | 传输面：`publication_draft_payload.go` 一格 + `Publication()` 一段；新文件 `publication_draft_payload_settlement_policy.go` 放 `SettlementPolicyBodyPayload` 与 `ContractVersionReferencePayload`（合同维收对象 + 版本两格，不收拼好的串），问题落 `settlementPolicy.<键>`、合同两格各自点名 |
| `502395f4` | **地盘外**：seed `publish-batch.json` 的 `SYN-SETTLEMENT-PREPAID-01/v1` 的 `contentDigest` 由 `sha256:syn-…` 换成算出的 `PCC-1:7d13e3ef…`（票 10 为客户合同那一项做过同一件事）；已用旧串施加过的演示库重放本项会答 `CONTENT_CONFLICT`，要重建库再 seed |
| `b47c5461` | Web 纯逻辑 `settlement-policy-form.ts` + node:test：草稿（壳五格 + 正文九格）→ `CommercialPublicationPayload`、认领路径与载荷键一一对应（合同维展开到两格）、`contractChoiceKey` / `contractChoiceOf`、`methodCodesOf`、`withShellIntervalCopiedIntoPolicy`；`publication-draft-api.ts` 加两型 + `settlementPolicy?` 一格 |
| `ace9611f` | Web 组件与页：`SettlementPolicyPublicationForm.tsx` 挂 `PublicationDraftFlow`；`CommercialPoliciesPage.tsx` 一 import + 一签，发布落定 `notePublished('SETTLEMENT_POLICY')` |
| `23a4ad59` | 机制清点在 `ace9611f` 干净检出重生成（partycommercial 生产 +2、测试 +3、管理台 +1） |

**借了哪一笔**：开工时 cherry-pick 了通道 4 的 `mcp4-awf20@17a962a5`（词表前端半边）；随后发现 `internal/architecture` 的 `TestEveryAdminWebPathIsOnTheParcelAPIEndpointTable` 要求 `/commercial-publication-vocabularies` 在端点表上——只借前端半边这一门必红，于是按派单给的第二条路**整支变基到 `mcp4-awf20@802ae400` 之上**（`git rebase --onto`，那笔 cherry-pick 因「patch 已在上游」被 rebase 自动丢弃）。本分支自己的只有上表八笔，awf/20 的七笔是祖先不是本票的内容，重放时从 `802ae400` 之后取。

**完成判据逐项**：

1. 结算政策册旁多一签「发布结算政策版本」：表单 → 预览摘要 → 存为待批准 → 批准 → 发布五步由公共半边 `PublicationDraftFlow` 走；载体到达发布那一步 `onPublished` → 页面既有 `PublishedNotice` 切到 `SETTLEMENT_POLICY` 册并重读，读面一行未改（`contractLabel` 列显的正是领域那一个两段式串）——`ace9611f`。今天四口与词表口都挂 `UnconfiguredIntake{}`，页面如实显示 403，不假装可用。
2. tsc / run-tests 绿——见验证强度。
3. Go 侧只加本册规范化一格——`90b7fc45`；要真正「接进同一号」还需应用层两个方向（`82c8caa3`）与载荷一格（`7d086085`），三笔合为本册的机制半边。

**硬句在场**：表单不算摘要、不收也不送批准人（`settlement-policy-form.test.ts` 钉载荷里无 tenant / submitter / approver / contentDigest / approval / canonicalization）；预览与录入同一份载荷、同一段解码、同一处算摘要（`TestSettlementPolicyPreviewAndSubmissionShareTheDigest`）；表单不裁任何门——连「必填」都不在本地拦，方式集外、币种存不存在、引用在不在册一律送上去，`localProblems` 不给（十四格全是文本 / 选单）。**本册特有**：结算政策答「怎么结」不答「要不要接受前控制」——草稿、载荷、组件都没有控制字段，测试钉载荷里无 `preAcceptanceControl` / `control` / `requirement` / `jointPass`。

**选形落地**：`method` 下拉只吃 `fetchPublicationVocabulary('SETTLEMENT_POLICY')` 的 `method` 一集 × 本页 `settlementMethodLabels`，不内置 PREPAID / TERMS、不预选，词表 403 / 读不到 / 无那一集时下拉显占位不自造码（`methodCodesOf` 缺席交 null，测试钉）；`contract` 从客户与合同目录选、**对象 + 版本两格一起落**（选单键是两格的 JSON 数组编码，含「/」不歧义，测试钉；目录不可用退回**两个**手填格，仍不收拼好的串——服务端也按形状拒串与 `contractLabel` 键，`TestSettlementPolicyPayloadTakesTheContractAsTwoFieldsAndMustMatchItsKind`）；责任法人从集团法人册选；客户相对方从货主客户账户册选（结算按账户归集，seed 那一格也是账户标识；读面不可用退回手填）；币种收 ISO 代码串、不改大小写不查表，存在性由构造门答。

**规范化文档里 contract 一格的取舍**：写领域的两段式指称串「对象/版本」（`NewQualifiedVersionLabel` 拼出、0011 `contract_label` 存的那一个），不拆回批文的 objectId / version 两格——拆回要另立一处知道分隔符的代码，而 `QualifiedLabel` 的注释正是为「只许一处拼」写的；载荷与批文收两格，进领域那一刻合成这一个串，文档写它就是写领域此刻持有的东西。文档字节整份钉在 `TestSettlementPolicyCanonicalizesIntoTheSameVersionMirroringTheBatchDocument`。

**共享文件各改了哪几处（生产文件全部纯加行；数字为 `git diff --numstat 802ae400 23a4ad59`）**：`domain/publication_canonicalization.go`（+20/−0：版本号注释一行、`PublicationContent` 一格、kind 不符一句 + switch 一支、`IsRegisterCanonicalized` 一 case、`canonicalPublicationDocument` 一格、`Rehydrate` 一支——早返回）；`application/publish_commercial_authority.go`（+10/−0）；`application/publication_draft.go`（+6/−0）；`adapters/http/publication_draft_payload.go`（+6/−0）；`apps/admin-web/src/pages/party/publication-draft-api.ts`（+26/−0，对 `17a962a5`）；`CommercialPoliciesPage.tsx`（+8/−0）。测试文件：`application/publish_commercial_authority_test.go`（+15/−20）与 `publication_draft_test.go`（+5/−4）、`domain/publication_canonicalization_test.go`（+6/−4）与 `publication_draft_test.go`（+6/−5）——全是「样本换册」与「壳改配算出的摘要」，甲路下必然的一改。

**地盘外改动逐条**：(1) `scripts/demo-seeds/data/commercial/publish-batch.json` 一行（上表 `502395f4`，理由与取证见提交信）；(2) 四份既有测试文件里「没接的册」的样本从 SETTLEMENT_POLICY 换成 CUSTOMER_SERVICE_RULE（本册接进后不再是样本；选 18 那册是因为它仍 draft、最不会被下一张子票顺手接走）；(3) `docs/product/MECHANISM-INVENTORY.md` 重生成（`23a4ad59`）。

**验证强度（钉 `23a4ad59`，隔离 detached 检出 `%TEMP%\idp-verify-awf15`，验后已拆）**：`gofmt -l .` 空；`go build ./...` / `go vet ./...` 退 0；`go test -count=1` 动过的三个 PC 包 + 反向依赖（`go list` 反查：`cmd/parcel-api` / `parcel-commercial` / `parcel-dispatch`、PC postgres、PS / NR / SA / TF / VE 各自的 partycommercial 适配器、PS postgres）+ `./internal/architecture/...` 全 ok；**未设 DSN**（本票无 postgres / 迁移改动，派单明写不带），PG 用例跳过；`tools/mechanism-inventory` 在同一检出重跑，`git status` 对清点零行；admin-web `tsc --noEmit` 退 0、`run-tests` 132 / 132（含本票 9 条与祖先 awf/20 的 7 条）。证据层级 **S**（隔离合成）。`.sql` 无变动、不加迁移、不动 `endpoints.go` / `ports.go` / `settlement_policy.go` / `party/api.ts`。

**判断题（留给评审 / 伞票收口，不在本票动）**：(1) `Field` / `ReferencePicker` 在本页四张表单里各有一份私有副本（本票第四份），宜抬到 party 共享层——动别人的文件，另立票；(2) 壳上的指名引用 `references.CUSTOMER_CONTRACT` 本票不给输入格（seed 的结算政策壳带着它；带了发布会在被引合同未发布时答`发布未决`，是操作者的选择），服务端点名时落在「未认领」列，与 16 / 11 同一处置；(3) 客户相对方从货主客户账户册取而不是业务参与方册：CONTEXT 说「客户相对方」、seed 用账户标识，两读都通，选单退回手填所以不锁死。非作者评审由通道 1 派，结论写「进 main 记录」。

**未做（各归其票）**：目录读面不显示正文以外的东西（读面已有 0011 各列，本票只加写签）；五步状态机与各口答案中文归公共半边（票 16）；词表读口归票 20；四口与词表口点亮归操作者接入渠道（机制半边待接线，ADR-0100 那一族）。
