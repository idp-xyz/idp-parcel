# 13 `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` 版本的运营主路径：逐字段表单（共同通过条件 + 控制项可加行）

Category: enhancement
Status: resolved——2026-09-08 21:5x 通道 6 交付（分支 `mcp6-awf13`，代码 tip `5d56ad2a`、含清点的 tip `bdebd840`，基 `mcp4-awf20@802ae400`；逐笔 SHA 与验证强度见文末「完成记录」；main 上的 SHA、非作者评审结论待通道 1 重放后补「进 main 记录」）。完成判据三条全落：政策页多一签「发布接受前财务控制策略版本」，五步走公共半边、发布落定读面切到本册重读；tsc 退 0 / run-tests 134 / 134；Go 侧本册接进 PCC-1 一格（连同应用层两个方向与载荷一格，均在封存现场与 `86b55e86` 上，本会话补传输面测试）。此前 in-progress——2026-09-08 20:5x 通道 6 接手（用户直授「接吧」）：通道 5 会话 19:5x 认领（通道 1 派单，树 `D:/tops/idp-parcel-mcp5-awf13`）后 20:12 crash，六份未提交文件 mtime 停在 20:12:13、分支零提交未推；20:5x 由通道 6 原样封存为 `mcp5-awf13@feaf5bbb`（非集成候选，只防丢），另起分支 `mcp6-awf13`（树 `D:/tops/idp-parcel-mcp6-awf13`，基 `mcp4-awf20@802ae400`——词表读口的端点表门要它在场，同票 15）从封存笔接着做。此前 ready-for-agent——形状已裁清（逐字段表单 + 控制项可加行；本票无待裁问题），读面已随票 [06](./06-pre-acceptance-financial-control-policy-versions-have-no-read-face.md) 进 main（`a1890506` / `ea2293e5`），Blocked by 08 未 resolved 前不在前沿；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面，见伞票 Comments）
Blocked by: 08（已进 main）；[20](./20-publication-vocabulary-read-face.md)（词表读口公共半边，2026-09-08 通道 1 代裁立票；落点广播前表单里的下拉先按票面写成占位、不内置枚举）

## 册与载荷

显示在**政策页·接受前财务控制策略册**（票 06，第九册）。`declarations.preAcceptanceFinancialControlPolicyBody{
jointPassCondition, controls[{control, chargeScope, order, onFailure, responsibility}]}`（0024 正文，ADR-0115）：
三个封闭集（控制种类两值、失败处置两值、共同通过条件首发一值）、判断顺序版本内唯一且从 1 起、（种类 × 范围）唯一、
至少一项。

## 选形与理由（ADR-0101 决定八）

**逐字段表单，控制项可加行。** 频次低、配置员操作、正文是一格 + 一张几行的表。三个封闭集由服务端词表读口供
下拉——**控制种类下拉里没有「无控制」**，那一格属合同声明（ADR-0115 Decision 一），表单不得在这里长出它。顺序
唯一、范围 × 种类唯一、至少一项，都由构造门答；表单可以在提交前提示重复，但拒绝的话由服务端说。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；另一条本册特有：责任引用与费用范围引用是开放引用（0024 头注），表单收串不校验存在性。

## 完成判据

接受前财务控制策略册旁多一签「发布策略版本」（表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同册立刻可见
（正文列从「未登记」变「已登记」）；tsc / run-tests 绿；Go 侧只加本册规范化一格。

## 边界

不动 0024；不动 settlement-accounting 的读路径（sa-preacceptance-policy-view/02）。

## 完成记录（2026-09-08，通道 6 两任会话；分支 `mcp6-awf13`，基 `mcp4-awf20@802ae400`）

**接手链**：通道 5 会话 20:12 crash，六份未提交文件由通道 6 前一会话原样封存为 `mcp5-awf13@feaf5bbb`（基 `c135049b`）并在 `802ae400` 之上重放为 `896994ac`——两笔的六份文件逐字节同（`git diff feaf5bbb 896994ac` 只差 awf/20 那批祖先文件）。前一会话做到 `86b55e86` 后无响应；21:1x 起由本会话（通道 6 新会话，无前任上下文，通道 5 21:2x 明确移交）从 git 上的状态接着做，未凭任何自报。

**逐笔（分支 SHA；main 上的 SHA 由进 main 记录补）**：

| 分支 SHA | 内容 | 会话 |
|---|---|---|
| `896994ac` | 封存通道 5 现场（非集成候选，只防丢）：领域 `publication_canonicalization_pre_acceptance_financial_control_policy.go` + 测试、`publication_canonicalization.go` 一格、`pre_acceptance_financial_control_policy.go` 把跨行的门抽成 `filedPreAcceptanceControlItems`（发布时与预览时同一条规则、一处定义）、传输面 `publication_draft_payload_pre_acceptance_financial_control_policy.go` + `publication_draft_payload.go` 一格 | 通道 5（封存于通道 6） |
| `1a87d061` | 票面转 in-progress | 通道 6 前任 |
| `86b55e86` | 应用：`publicationContentOf` 一 case + `declarationsOfContent` 一支，两个方向同笔；既有策略用例的壳改配算出的摘要（`controlPolicySpec`）；新测试 `publication_pre_acceptance_financial_control_policy_test.go` | 通道 6 前任 |
| `eac0f65b` | 传输面测试 `publication_draft_payload_pre_acceptance_financial_control_policy_test.go`：同一份载荷预览 / 录入同摘要、换行序摘要不变、逐格问题按 `preAcceptanceFinancialControlPolicy.controls[i].<键>` 收齐（含「无控制」那个词集外）、顺序非 JSON 整数按形状拒、跨行门 / 正文缺席 / 错册在预览上答未受理不带摘要。三条在 `86b55e86` 的实现上直接通过，未改生产代码 | 本会话 |
| `c4bbed53` | Web 纯逻辑 `pre-acceptance-financial-control-policy-form.ts` + node:test（11 条）：草稿（壳五格 + 共同通过条件一格 + 控制项若干行）→ 载荷、认领路径随行数走并含行本身、顺序整数格空即缺席 / 非整数文本记本地问题、`controlPolicyCodesOf` 按名取词表一集、`controlRowHints` 只提示不拒、行增删改；`publication-draft-api.ts` 加一格 + 两型 | 本会话 |
| `b74f9808` | Web 组件 `PreAcceptanceFinancialControlPolicyPublicationForm.tsx` 挂 `PublicationDraftFlow`；`CommercialPoliciesPage.tsx` 一 import + 一签（+10/−0），发布落定 `notePublished('PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY')` | 本会话 |
| `5d56ad2a` | 自审两轴修补：UI 标签改用 CONTEXT 原词（失败处置、失败或补偿责任方）、去一处跨文件计数、Go 测试载荷改 helper 拼装 | 本会话 |
| `bdebd840` | 机制清点在 `5d56ad2a` 干净检出重生成（partycommercial 生产 107→109、测试 111→114、管理台 20→21） | 本会话 |

**完成判据逐项**：

1. 接受前财务控制策略册旁多一签「发布接受前财务控制策略版本」：表单 → 预览摘要 → 存为待批准 → 批准 → 发布五步由公共半边 `PublicationDraftFlow` 走；载体到达发布那一步 `onPublished` → 页面既有 `PublishedNotice` 切到本册并重读（读面票 06 一行未改，正文列由「未登记」变「已登记」是它既有的显法）——`b74f9808`。今天四口与词表口都挂 `UnconfiguredIntake{}`，页面如实显示 403，不假装可用。
2. tsc / run-tests 绿——见验证强度。
3. Go 侧只加本册规范化一格——`896994ac` 的领域半边；要真正「接进同一号」还需应用层两个方向（`86b55e86`）与载荷一格（`896994ac`），合为本册的机制半边。**地盘外的一改**：`domain/pre_acceptance_financial_control_policy.go` 把 `NewPreAcceptanceFinancialControlPolicy` 里跨行的门抽成 `filedPreAcceptanceControlItems`（+26/−12），理由写在该函数注释——同一条 CONTEXT 规则要在发布时与预览时两个时刻答，两处各写一份就漂了无人报；行为不变，既有测试未改。

**硬句在场**：表单不算摘要、不收也不送批准人（`pre-acceptance-financial-control-policy-form.test.ts` 钉载荷里无 tenant / submitter / approver / contentDigest / approval / canonicalization，也无合同层声明的键）；预览与录入同一份载荷、同一段解码、同一处算摘要（`TestControlPolicyPreviewAndSubmissionShareTheDigest`）；表单不裁任何门——连「必填」都不在本地拦，条件 / 种类 / 处置集外、引用在不在册一律送上去；`localProblems` 只有各行判断顺序编不进 JSON 整数的文本（判据同 credit-policy-form）。**本册特有**：控制种类下拉里没有「无控制」——三个下拉只吃 `fetchPublicationVocabulary('PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY')` 的 `jointPassCondition` / `control` / `onFailure` 三集 × 本页词表，表单不内置任何一格，词表里没有它，表单也长不出它（Go 侧 `TestControlPolicyPayloadCollectsEveryFieldProblem` 钉 `NO_CONTROL` 作控制种类集外）；首发共同通过条件只有一值仍不预选（ADR-0115 Decision 三）；开放引用（费用范围、责任）收串不校验存在性。

**选形落地**：控制项可加行（`withControlRowAdded` / `withControlRowRemoved` / `withControlRowPatched`，空草稿带一行空控制项——可填的格，不是值）；「表单可以在提交前提示重复」落成 `controlRowHints`：两行抢同一个判断顺序、同一范围上同一种控制两行各提示一句，只显在表下、不进 problems、不挡预览，拒绝由服务端说；行本身（`controls[i]`）也认领，服务端点名整行时显在行下不显成别处的问题。判断顺序整数格空即缺席——载荷型 `order?: number` 与 Go 的 `int` 不逐字同形，理由写在 `publication-draft-api.ts` 该型注释（零与缺席在 Go 侧同义，表单不替操作者填 0）。

**共享文件各改了哪几处（生产文件全部纯加行；数字为 `git diff --numstat 802ae400 bdebd840`）**：`domain/publication_canonicalization.go`（+20/−0）；`application/publish_commercial_authority.go`（+11/−0）；`application/publication_draft.go`（+6/−0）；`adapters/http/publication_draft_payload.go`（+6/−0）；`apps/admin-web/src/pages/party/publication-draft-api.ts`（+27/−0）；`CommercialPoliciesPage.tsx`（+10/−0）。测试文件 `application/publish_commercial_authority_test.go`（+21/−14）：既有策略用例的壳改配算出的摘要（甲路下必然的一改）。**seed 未动**：`scripts/demo-seeds/data/commercial/publish-batch.json` 的 `SYN-FIN-CONTROL-01/v1` 只有壳没有正文，对账门对壳单独发布不比对（`TestAPreAcceptanceFinancialControlPolicyDeclaredDigestIsReconciled` 的「a shell without its body still publishes」），无需换 PCC-1。

**验证强度（钉 `bdebd840`，隔离 detached 检出 `%TEMP%\idp-verify-awf13`，验后已拆、status 零行）**：`gofmt -l .` 空；`go build ./...` / `go vet ./...` 退 0；`go test -count=1` 动过的三个 PC 包 + `go list` 反查的反向依赖（`cmd/parcel-api` / `parcel-commercial` / `parcel-dispatch`、PC postgres、NR / PS / SA / TF / VE 各自的 partycommercial 适配器、PS postgres）+ `./internal/architecture/...` 共 15 包 14 ok（ports 无测试）；**未设 DSN**（本票无 postgres / 迁移改动），PG 用例跳过；`tools/mechanism-inventory` 在同一检出重跑，`git status` 对清点零行；admin-web `tsc --noEmit` 退 0、`run-tests` 134 / 134（本票 11 条 + Go 侧 3 条传输面测试另计）——在分支树 `5d56ad2a` 内容上跑（树干净，`bdebd840` 只加 .md）。证据层级 **S**（隔离合成）。`.sql` 无变动、不加迁移、不动 `endpoints.go` / `ports.go` / `party/api.ts` / 0024。

**自审（本会话，两轴串行，基线 `86b55e86`；隔离子代理起不来——工具答鉴权错误——退为本人只读串行）**：Standards——注释全中文、跨文件引用用符号名；一处跨文件计数已改口（`5d56ad2a`）；`Field` / `useLoaded` / `vocabularyPlaceholder` / 整数解析在本页各表单里各有私有副本（Duplicated Code，判断题，见下）。Spec——完成判据三条、伞票硬句、本册硬句在场；UI 标签两处改用 CONTEXT 原词（`5d56ad2a`）；无票面未要的行为（提示重复是票面允许的）。非作者评审由通道 1 派，结论写「进 main 记录」。

**判断题（留给评审 / 伞票收口，不在本票动）**：(1) `Field` / `useLoaded` / `vocabularyPlaceholder` 与整数解析（`integerOf` / `integerProblem`）在本页几张表单里各有私有副本，宜抬到 party 共享层或从 credit-policy-form 导出——动别人的文件，另立票；(2) `order?: number` 与 Go `int` 不逐字同形（见上「选形落地」），若伞票要「线格式逐格镜像」读成必须同形，改成 `order: number` + 留空送 0 是一行的事，但那是表单替操作者填了一个值；(3) 词表读一次不重取：词表在表单打开后才就绪（如接入渠道配置那天）要重开表单，与 awf/15 同一取舍。

**未做（各归其票）**：五步状态机与各口答案中文归公共半边（票 16）；词表读口归票 20；四口与词表口点亮归操作者接入渠道（机制半边待接线，ADR-0100 那一族）；伞票 07 子票表本行的状态由推送方在进 main 时改（同票 15 的做法）。

**评审后补一笔（2026-09-09 11:1x，通道 5 代作者补；作者通道 6 当时无会话，推送方改派）**：代码 tip `e58a085a`——`cmd/parcel-commercial/publish_batch_test.go` 的 `controlPolicyBatchBody` 壳上摘要由随手写的 `sha256:fcp-1` 换成 `CanonicalizePublicationContent` 对这份正文算出的 `PCC-1:07cd1a96…`，头注补一句「改正文任一格要重算」。为何：本册在 `86b55e86` 接进服务端规范化后对账门对它开门，两串不等答 NOT_ACCEPTED，`TestAPublishedPreAcceptanceFinancialControlPolicyIsReadBackByTheContentView` 在含 DSN 时红（CI 有 DSN，合入即红）；上面「验证强度」自述未设 DSN，正是漏掉它的原因。只改夹具一串，不动对账门、不动其他夹具；带 DSN 跑 `./cmd/parcel-commercial/` ok。非作者评审全文由推送方在重放时写进 Comments。

## Comments

- **评审 ← 通道 4 · 钉 `2599816f` · 2026-09-08 22:03**（非作者；基线 `802ae400`，隔离 detached 检出 `%TEMP%\idp-review-awf13` 只读，验后已拆；隔离子代理本机答鉴权错误，退为本人两轴串行。）
  - **Standards · 阻断：无。非阻断 3**：(1) Duplicated Code——`PreAcceptanceFinancialControlPolicyPublicationForm.tsx` 私有 `Field` / `useLoaded` / `VocabularySelect` /
    `vocabularyPlaceholder`，`pre-acceptance-financial-control-policy-form.ts` 的 `integerOf` / `integerProblem`，与 credit-policy 等表单同形副本（判断题 (1) 属实）；抬到 party
    共享层另立票，不挡。(2) Repeated shape——`domain.PreAcceptanceControlKindNamed` / `ControlFailureDispositionNamed` / `JointPassConditionNamed` 三循环一字同形，仓内先例
    `CommercialObjectKindNamed` 亦然，判断题，不动。(3) 载荷 `PreAcceptanceControlItemPayload.item` 对缺席 order 答「收到 0」——操作者留空却读到 0，与判断题 (2) 同源，措辞一行可改，不挡。
    **无发现**：注释全中文、跨文件引用皆符号名、无行号无计数；领域新文件只 import fmt；六个共享生产文件逐 hunk 纯加行；`filedPreAcceptanceControlItems` 抽取——原四条 `||`
    同答 `ErrInvalidPreAcceptanceFinancialControlPolicy`，拆两段仍同错，逐行错误值与排序一字不变，既有测试未改即过；`PreAcceptanceControlKindNamed` 从 `PrepaidFreezeControl`
    迭代到 `valid()` 止、`NO_CONTROL` 钉集外（ADR-0115 D1）；加册仍 PCC-1（ADR-0126 D1）。
  - **Spec · 阻断：无。非阻断 2**：(1) `publish_commercial_authority_test.go`「a body without any control」由 error 改答 NOT_ACCEPTED + cause——对账门对本册开门的必然
    （ADR-0126 D2）；票面完成判据「Go 侧只加规范化一格」实际含应用两向 + 载荷一格，与 11 / 12 / 17 同读法，建议进 main 记录点明这句读法。(2) 「至少一项」无本地提示
    （删到零行只在预览答未受理）——票面只「允许」提示，不算缺，记一句。**无发现（派单五项逐核）**：① 三条门在 `filedPreAcceptanceControlItems` + `NewPreAcceptanceControlItem`
    一处，预览 / 发布同函数；开放引用 `requireField(NewChargeScopeReference / NewControlResponsibilityReference)` 只查非空。② 地盘外一改行为不变。③ 三个下拉只吃
    `fetchPublicationVocabulary('PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY')` 的 `jointPassCondition` / `control` / `onFailure` 三集，`VocabularySelect` 选项仅由服务端 codes 生成、
    `labels` 只作中文装饰；`emptyControlPolicyDraft` 不预选、首项「未选」；unconfigured 显「词表未就绪（接入渠道未配置，403）」。④ seed `SYN-FIN-CONTROL-01/v1` 无 declarations 块；
    `publicationContentOf` 无正文答 present=false → 对账门不比对，旧串照发，属实。⑤ 领域测试盖三门 + 共同通过条件缺席 + 零值行 + 换行序同摘要 + 折回再算同串；判断题三条与代码对上。
    硬句：载荷无身份 / 摘要 / 合同层键；预览录入同摘要（`TestControlPolicyPreviewAndSubmissionShareTheDigest`）。边界：无 .sql、无 SA 文件。
  - **验证钉 `2599816f`**：`gofmt -l` 空 / `go vet` 静 / `go test -count=1` PC 四包 + `cmd/parcel-api` + `internal/architecture` 全 ok（未设 DSN）；admin-web tsc 退 0；run-tests 134/134。
    **汇总**：两轴无阻断，可重放。
- **评审（第二份，09-09 接手的推送方未读到前一夜那份而再派）← 通道 5 · 钉 `2599816f` · 2026-09-09 11:04**（非作者；基线 `802ae400`；评 `86b55e86` / `eac0f65b` / `c4bbed53` /
  `b74f9808` / `5d56ad2a` 五笔、10 文件；只读、未跑测试；隔离子代理产出未能回收，两轴由本人串行只读完成。）
  - **Standards · 阻断：无。非阻断 4**：(1) `PreAcceptanceFinancialControlPolicyPublicationForm.tsx` 头注「十册没有一类是矩阵」——数的是 `CommercialObjectKind` 封闭集（别处的东西），
    未锚 SHA；AGENTS「改文档」计数条。同句抄自已在 main 的 `SupplierAgreementPublicationForm.tsx`，一词之改（「各册」）。(2) 同文件 `ControlRow` 标签「失败或补偿责任方」——
    CONTEXT / ADR-0115 槽名是「失败或补偿责任」，多一「方」字；AGENTS「引用领域术语时用文档里的原词」。判断题。(3) Duplicated Code（判断题）：`Field` 第 4 份；`integerText` /
    `integerOf` / `integerProblem` 与 `credit-policy-form.ts` 逐字同。`useLoaded` / `VocabularySelect` / `vocabularyPlaceholder` 本树内**无**第二份——票面判断题 (1) 对这三者的说法在
    `2599816f` 不成立。(4) `publish_commercial_authority.go` `publicationContentOf` 与 `publication_draft.go` `declarationsOfContent` 头注「今天只有信用政策一格」已不实（本票前已不实），
    加分支未顺手改口——非本票引入。**无发现**：注释全中文；无行号引用；无默认值（`emptyControlPolicyDraft` 的空行是可填格；`jointPassCondition: ''` 不预选；
    `<option value="">未选</option>` 是未选态）；Go 两支与 CreditPolicy / SupplierAgreement 同形；测试改动加严（多钉 `registry.loads != 0`）。
  - **Spec · 阻断：无（11:08 补记后改为 1，见下）。非阻断 2**：(1) 判据三「Go 侧只加本册规范化一格」字面 vs 实际：应用层两支 + 载荷一格 + 既有测试改配。两支是流程必需——
    无 `declarationsOfContent` 一支，载体发布只发壳不发正文；无 `publicationContentOf` 一支，对账门不对本册开门（票 08「甲路下只多一道相等校验」）。判据字面窄了、实施对；建议票面
    改口而非改码。(2) 留空 `order` → 不送键 → Go 解成 0 → 成因「收到 0」：操作者留空却被告知收到 0，可读性问题，不违句。**无发现**：一签「发布接受前财务控制策略版本」挂
    `PublicationDraftFlow`，`onPublished` → `notePublished('PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY')`；表单不算摘要、无身份 / 批准人键（`controlPolicyPayloadOf` + 禁键测试）；不裁门：
    `controlPolicyLocalProblems` 只有 order 文本形状；三下拉只吃词表三集，`controlPolicyCodesOf` 缺集回 null 显占位，无内置 ALL_CONTROLS_PASS / PREPAID_FREEZE / REJECT，无「无控制」；
    预览 / 录入同摘要、换行序摘要不变；边界：0024、SA 读路径、endpoints / ports 未动；零项用例由 error 改 NOT_ACCEPTED 是对账门开门的必然后果，非削弱。
  - **判断题**：(1) 非 Spec 事项、Standards 判断题；本树只 `Field` 与整数三函数有副本；另立票，不挡。(2) 07 硬句与 08 完成记录里**没有**「线格式逐格镜像」这句；08 只写
    「`CommercialPublicationPayload` 一份线格式供预览与录入同一段解码」，说的是服务端一段解码，不是 TS 型逐格同形。`order?: number` 对 Go `int` 是线上兼容的超集（缺键 → 0 →
    `payload.Order <= 0` 点名 `.order`）；既有 `limitMinor?: number` 对 `*int64 omitempty` 同一做法。改成必填送 0 = 表单替操作者填值，更靠近红线「不给默认」。结论：不违，维持。
    (3) 票 13 / 08 / 20 均无重取要求；与 awf/15 同一取舍；不挡。
  - **评审补记 · 11:08**（按推送方含 DSN 实测补入项）：**Spec 阻断 1**——`cmd/parcel-commercial/publish_batch_test.go` `TestAPublishedPreAcceptanceFinancialControlPolicyIsReadBackByTheContentView`：
    夹具壳上摘要 `sha256:fcp-1` 是旧式串；本册经 `publicationContentOf` 一支接进 PCC-1 后 `reconcileDeclaredDigest` 对它开门，算出 `PCC-1:07cd1a96…`，两串不等答 NOT_ACCEPTED。
    按票 08「甲路下只多一道相等校验」是开门的必然后果，作者在 `publish_commercial_authority_test.go` 改了应用层那一份（`controlPolicySpec`），漏了 cmd 层这一份。为什么漏：票面
    「验证强度」自述「未设 DSN，PG 用例跳过」，而这条正是含 DSN 才跑的；CI 有 DSN，合入即红。修法属作者：夹具改配算出的摘要，不动对账门。**门：有阻断，回作者同一分支补一笔**
    → 作者通道 6 无会话，由通道 5 代作者补 `e58a085a`（见上「评审后补一笔」），阻断修平。供入台账：下次同形票（接进 PCC-1 一格）应把「跑含 DSN 的 cmd 层发布用例」写进完成判据。
- **补评 ← 通道 3 · 钉 `da2736e5` · 2026-09-09 11:36**（推送方自查出前两份评审都按派单跳过了 `896994ac`（`chore(salvage)` 封存笔），但那一笔装的是本票 Go 领域 / 载荷的主体，
  且按「换过会话的同一通道也算同一人」通道 5 对它是作者；补派通道 3 重点直读 `896994ac` 四份 Go 全文 + `e58a085a` + `86b55e86` 与封存笔的接缝。基线 `802ae400`，隔离树
  `%TEMP%\idp-review-awf13b` 只读。）
  - **阻断：无。**
  - **非阻断 3**：(1) [Spec · 地盘外] `domain/pre_acceptance_financial_control_policy.go` `NewPreAcceptanceFinancialControlPolicy` → 抽 `filedPreAcceptanceControlItems`：票面「Go 侧只加
    本册规范化一格」之外的一改，完成记录已披露。逐行核 12 行删除：版本两格仍在构造函数；`jointPass.valid()` / `len(items)==0` / 逐项 `valid()` / （种类 × 范围）唯一 / 顺序唯一 /
    按顺序排序全部原样迁入 helper，三个错误值一一对应；门未削弱（ADR-0115 那一族无变）。理由合 AGENTS「单一权威」。放行。(2) [Standards · 判断项] `PreAcceptanceControlKindNamed` /
    `ControlFailureDispositionNamed` / `JointPassConditionNamed` 三个反查同形；awf/12 已有泛型 `closedCodeNamed[Code ~uint8]`，两票进 main 后可合一。另立票。(3) [Spec · 作者判断题 (2)]
    http `PreAcceptanceControlItemPayload.Order int`：零与缺席同答「须为从 1 起的正整数」，不给默认；TS `order?: number` 不同形 → 放行：表单不替操作者填 0 合硬句「不给默认」，
    逐字同形不是伞票硬句。
  - **无发现（已核）**：封存信里「http 新文件带 CRLF 未 gofmt」在 `da2736e5` 不成立：`gofmt -l` 两包空、该文件 CRLF 0。Standards：注释全中文、只写取舍；跨文件引用用符号名
    （`ControlBindingPayload.binding`、`CommercialObjectKindNamed`、`preAcceptanceControlItemDocument`），无行号 / 计数；领域新文件只 import fmt，无 HTTP / pgx。PCC-1 不换号；
    `canonicalPublicationDocument` 加一键 `preAcceptanceFinancialControlPolicy`，节内 `jointPassCondition` / `controls[{control,chargeScope,order,onFailure,responsibility}]` 镜像批文，
    控制项按判断顺序写出（换行序摘要不变）。「无控制」：`PreAcceptanceControlKind` 枚举只 PREPAID_FREEZE / CREDIT_CHECK，`Named` 集外答 false，http 测试
    `TestControlPolicyPayloadCollectsEveryFieldProblem` 钉 NO_CONTROL 集外（ADR-0115 Decision 一）；`JointPassConditionNamed` 空串 false，不折成 ALL_CONTROLS_PASS（Decision 三）。
    对账门新支与信用政策支同判据。`e58a085a` 夹具摘要：评审者在隔离树用一次性测试对同一份正文重算 → `PCC-1:07cd1a96103b19f524b8472a906f16afba5f96d1c7fecfdcf09c9bfa9e9a809c`，
    与夹具逐字节同。`da2736e5` 上 `go test` domain + http 两包 ok，`go vet` 退 0。
  - **证据层级**：`896994ac` 四份 Go 与 `e58a085a` 逐行直读；两份 Go 测试只看测试名与 NO_CONTROL 钉；其余五笔按派单只拄接缝。**结论：无阻断，可重放。**
- **推送方注（2026-09-09）**：三份评审共同指出的「判据三字面窄于实施」——本票的「Go 侧只加本册规范化一格」读作**领域规范化一格 + 应用层两个方向 + 载荷一格**，与 11 / 12 / 14 /
  15 / 17 同一读法，是伞票 07 那一族票面的通用口径，不改本票判据文字；「跑含 DSN 的 cmd 层发布用例」是否写进同形票完成判据归用户（tasks.md 归用户项）。

## 进 main 记录（推送方通道 1；2026-09-09 11:4x 起重放、12:xx 验证与推送——前一任会话解第一笔的冲突到一半即断，本会话据 git 现场接着做）

- 重放：隔离 detached 树 `%TEMP%\idp-replay-awf13` 基 main `568cc50f`（12 / 14 / 15 / 17 / 20 已在），本票 `802ae400..da2736e5` 跳清点笔 `bdebd840` 后十笔：
  `896994ac→53480472`、`1a87d061→ea1a6cc1`、`86b55e86→17050991`、`eac0f65b→bab8b213`、`c4bbed53→dbc930aa`、`b74f9808→7eccadcf`、`5d56ad2a→7a6385fd`、`2599816f→c4378298`、
  `e58a085a→8c8c068b`、`da2736e5→71a6983d`。封存笔 `896994ac` 照进（它是本票 Go 主体，非纯簿记；补评已覆盖）。六份共享文件各撞邻册的相邻加行——`publication_canonicalization.go`
  两处（`IsRegisterCanonicalized` 的 `case` 与 `canonicalPublicationDocument` 的一键）、`publication_draft_payload.go` 两处、`publication_draft.go` 一处、`publish_commercial_authority.go`
  一处、`publication-draft-api.ts` 一处（一格 + 两型整块后移）、`CommercialPoliciesPage.tsx` 三处（import / 签 / 内容），**逐 hunk 手工并**（各册的 `if` / `case` 各自闭合，本册加行一律
  排在 12 那一册之后）；每份并完核过「零删行、加行多重集与分支上那笔逐行相等」。十笔重放完，链 tip 对分支 tip 的差恰为 main 自 `802ae400` 起前进的 58 个文件，无一多余；八份
  本票与 main 都动过的文件（上六份 + `publish_commercial_authority_test.go` + `cmd/parcel-commercial/publish_batch_test.go`）上，链上本票的净差与分支上本票的净差逐行相等
  （+10 / +27 / +3−1 / +6 / +6 / +11 / +21−14 / +20）。
- 清点在链 tip 重生成 `c4b71675`（partycommercial 生产 115→117 / 测试 123→126、http 适配器 24→25，与分支清点笔 `bdebd840` 的增量 +2 / +3 / +1 同；那一笔量的是 `802ae400` 上的数，不重放）。
- 验证钉 `c4b71675`：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；admin-web `tsc --noEmit` 退 0、`run-tests` **172/172**（161 + 本票 11）；含 DSN `go test -p 1 -count=1 ./...`
  **100 ok / 0 FAIL / 16 无测试 / 0 cached**（572s）——含 `cmd/parcel-commercial` 那条曾红的用例。
- 本笔之后 `--ff-only` 进共享 main，推前 `ls-remote` 核 `568cc50f`。分支 `mcp6-awf13` 内容已全在 main，指针改名 `merged/`；`mcp5-awf13`（封存现场，`896994ac` 的来源）改名 `salvage/`；
  树 `D:/tops/idp-parcel-mcp6-awf13` 无主（通道 6 无会话）、干净、内容全在 main → 推送方拆，指针留。
