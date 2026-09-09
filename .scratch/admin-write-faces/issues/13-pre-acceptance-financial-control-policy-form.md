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
