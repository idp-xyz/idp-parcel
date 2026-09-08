# 08 商业发布表单路径的公共半边：内容摘要改由服务端按册规范化算出、批准从操作者信封来、预览与发布同一路径

Category: enhancement
Status: resolved——2026-09-08，MCP-1 接 MCP-5 `mcp5-awf08@c003c845` 两笔续做完（用户 12:4x 指示通道 1 自办、不派工；分支 `mcp1-awf08` 基 main `5aedbb0f`，逐笔 SHA 与验证强度见文末「完成记录」；main 上的 SHA 待重放后对照）。完成判据四条全落：ADR-0126 + CONTEXT 两词条一句 + `PAR-COM-18`；服务端规范化 + 摘要首例信用政策 + 受控批文对账门；待批准载体（PC 迁移 0028）+ 审批职责规则批准门（未登记答`未配置`）+ 发布交既有用例答案代数一格不改；预览口与载体三口进端点表带未配置格。**子票 09–17 的 Blocked by 由此解除。** 此前 in-progress——三问由 [ADR-0126](../../../docs/adr/0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md) 一次答完（2026-09-08，MCP-5 按 MCP-1 派单 task-cc7313e8「owner 授权自决口径」裁，越权风险点四条单列在 ADR 里供 owner 复核），对号见下面「裁决」节；PC CONTEXT 词条「发布规范化版本」「待批准发布」+ Rules 一句、参数登记册 `PAR-COM-18` 已随 ADR 同笔落；实施（服务端规范化 + 摘要首例信用政策 → 待批准载体 + 批准门 → 预览口进端点表）按「完成判据」逐笔接。ADR-0127 预留号未用、释回。此前 draft——伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出的第一张，其余各册子票全部 Blocked by 本票（MCP-6 2026-09-07 立，锚 `95182b9d`）。**不转 ready-for-agent 的理由**：下面「要裁的」三问都是新裁决（摘要由谁算与受控批文那一半走甲还是乙、要不要待批准载体、预览口形状），伞票收口纪律是「形状裁清且不需要新裁决才转 ready」。**等什么**：(1) MCP-1 给 ADR 取号；(2) 三问由 owner 裁，或 owner 授权认领人经 `/domain-modeling` 自决——授权到了，认领人把三问裁决落进本票「裁决」节与 ADR，再转 in-progress 开工（裁与做可同人同票，但裁决先于代码）。接管会话 2026-09-07 于 `92579b0a` 逐句核过票面取证（`translate.go` 的 `contentDigest` / `approval` / `approvalRoleStanding` 三格、`NewCommercialContentDigest` 只查非空、`/commercial-publications` 挂 `UnconfiguredIntake{}`），见伞票 Comments
Blocked by: 无（等的是裁决与 ADR 号，不是别的票）

## 为什么每册子票都先卡在这里

伞票的硬句「表单不算摘要、不裁任何门：`contentDigest` 与受理都由服务端答，表单只呈现」，今天的发布口做不到——
它**要求调用方声明摘要**：

- 受控批文（`cmd/parcel-commercial/translate.go` 的 `publicationItemDocument`）每一项带 `contentDigest`，域上
  `NewCommercialContentDigest` 只查非空，**不从正文算、也不与正文对**；`application/publish_commercial_authority.go`
  里没有一处算或比对摘要（仅「声明只能随发布登记」那句注释提到正文由内容摘要盖住——盖住的是调用方声明的那个串）。
  今天「同键异内容答`内容冲突`」（ADR-0031）比的也是它。
- 批准是载荷里的三格字符串 `approval{reference, source, approvedAt}` + `approvalRoleStanding`，谁填的、凭什么填，
  登记册答不上来——ADR-0101 Context 对价卡说的那句在这里逐字成立。
- `/commercial-publications` 挂的是 `UnconfiguredIntake{}`，操作者身份（ADR-0100 `OperatorEnvelope`）还没有到这个端点。

逐字段表单若照今天的口来：要么前端算摘要（硬句禁）、要么让操作者手填摘要与批准人（那不是表单，是 JSON 镜像换了
皮）。所以先把这三件做成机制半边，各册表单才有东西可挂。

## 要裁的（`/domain-modeling`，难逆转处落 ADR——预留号已尽，向 MCP-1 取号）

1. **摘要由谁算、按什么形状算。** 每一类商业版本的正文都有自己的封闭结构（十类见 `CommercialObjectKind`），
   服务端按册规范化后算摘要，形状版本化（照 ADR-0014 给 PC 一个自己的规范化版本号，与 PS 的 `PSC-*`、PP 的
   `PPC-*` 并列）。**受控批文那一半怎么办**要一并裁：(甲) CLI 继续声明摘要，服务端算出后**必须相等**，不等即
   `NOT_ACCEPTED` 带出两个串——批量口从此有了一道对账门；(乙) CLI 也不再声明，摘要一律服务端算——批文少一列，
   既有 seed 与已施加的登记要重放一遍对照。倾向甲：不动已登记行、不动 seed，且让「调用方算错摘要」第一次变得可见。
2. **批准从哪来。** 照 ADR-0101 决定五、六：批准是独立的操作者动作，身份取 `OperatorEnvelope`；录入者与批准者
   两身份都记且可比；审批职责规则是租户治理参数，**未登记即不放行**。要答的是：低频、结构简单的册（伞票印象里的
   大多数）要不要也走「草稿 → 批准 → 发布」的载体——没有载体，录入与批准必然同人同刻，任何审批规则都立不住
   （ADR-0101 Alternatives 第三条）。倾向：给商业发布一个**轻量的待批准载体**（一版一行：正文快照 + 服务端摘要 +
   录入者 + 状态），批准动作推进它，发布 = 交既有 `PublishCommercialAuthorityHandler`；模板导入那一套（决定二、三）
   只在真有矩阵型册时才引入，本目录十册没有一类是矩阵。
3. **预览与发布同一条路径。** 一个不落库的预览口（形状待定：`POST /commercial-publications/preview` 或读面旁的
   校验口），过与发布**同一份**规范化与构造门，交回摘要与逐格问题；同一份载荷过预览与过发布逐字节同摘要
   （ADR-0101 决定四）。端点行进 `cmd/parcel-api/endpoints.go` 按共享接线文件纪律占号，Intake 挂 ADR-0100 操作者信封、
   未配置即拒（ADR-0085 两阶段）。

## 裁决（2026-09-08，MCP-5；正文在 ADR-0126，此处只对号）

1. **摘要**：服务端按册规范化算，PC 规范化版本号 **`PCC-1`**，摘要串自带版本（`PCC-1:<sha256 hex>`，先例 `PSC-1:<sha256>`）、不给 `commercial_version` 加列；文档只盖正文不盖壳（范围与区间是 `SaveVersion` 逐列比对的项）；首例只接信用政策，其余九册各子票接进同一号不换号。**受控批文走甲**：CLI 仍声明摘要，已接的册算出后必须逐字节相等，不等即发布用例新一格 `NOT_ACCEPTED` 带出两串、整项一行不写；没接的册与正文缺席的版本没有可比对象，照今天登记声明的串；seed 一字不动（今天没有 seed 发布信用政策）。`NOT_ACCEPTED`（改批文）与 `CONTENT_CONFLICT`（换版本号）分格（Decision 一、二）。
2. **批准**：轻量待批准载体 `PublicationDraft`，键 = 版本身份四元、一版一行，状态三格 `待批准` → `已批准` → `已发布`；录入者与批准者身份都由 Intake 从 `OperatorEnvelope` 交进（载荷无身份格、出现即拒），两者都记且可比；批准门先读审批职责规则（`PAR-COM-18`，实例半边「待提供」），未登记即答`未配置`不放行；发布只接`已批准`载体，交既有 `PublishCommercialAuthorityHandler`（`ContentDigest` = 算出的那一个、`ApprovalBasis` = 批准者 / 载体引用 / 批准时刻、`RoleStanding` = 已确认），答案代数一格不改（Decision 三）。
3. **预览**：`POST /commercial-publication-previews`，与录入口同一份载荷、同一段解码、同一个 `CanonicalizePublicationContent`，交回规范化版本、摘要与逐格问题；不落库；挂字面量 `UnconfiguredIntake{}`，隔离读放行装不进（编译期）；不做发布口的 dry-run 参数（Decision 四）。

## 完成判据

- 三问的裁决落 PC `CONTEXT.md`（规范化版本词条、批准是操作者动作那一句）与 ADR；参数登记册增审批职责规则一行
  （实例半边，只登「待提供」）。
- 服务端按册规范化 + 摘要的机制半边至少覆盖一类（建议拿子票 [16](./16-credit-policy-form.md) 信用政策做首例：正文
  最小、二选一约束现成），其余各册在各自子票里逐册接。
- 待批准载体 + 批准动作 + 发布交既有用例，答案代数一格不改；缺审批职责规则时批准门答`未配置`。
- 预览口落端点表；`tsc` / `run-tests` / 含 DSN `go test -count=1` 绿并注明。

## 边界

不动任何一册的正文形状、不动解析、不改受控批文的既有字段语义（甲路下只多一道相等校验）；不做蓝图第 38 节的其余
门禁（ADR-0101 决定七）。JSON 镜像签保留为高级口。

## 完成记录（2026-09-08，MCP-1；分支 `mcp1-awf08`，基 main `5aedbb0f`）

**承接**：MCP-5 在 `mcp5-awf08`（基 `d1e6c094`）交付两笔后 00:12 起无提交、树干净；用户 12:4x 指示通道 1 自办、不派工。
两笔 cherry-pick 到 main `5aedbb0f` 上零冲突（`d1e6c094..5aedbb0f` 与那两笔无 `.go/.sql` 重叠），SHA 换、作者不动；
`mcp5-awf08` 指针留作封存出处。

**逐笔（分支 SHA；main 上的 SHA 由进 main 记录补）**：

| 分支 SHA | 内容 | 出处 |
|---|---|---|
| `6152b9f5` | ADR-0126 + PC CONTEXT 两词条一句 + `PAR-COM-18` + README 行 + 票 08 转 in-progress 落「裁决」节 | MCP-5 `db9a05ac` |
| `5baf3348` | 领域 `CanonicalizePublicationContent`（PCC-1，首例信用政策）、`ReconcileDeclaredDigest`；发布用例 `NOT_ACCEPTED` 对账门；HTTP 与受控 CLI 带两串 | MCP-5 `c003c845` |
| `e8045f6b` | 领域 `PublicationDraft`（三格只向前）、`ApprovalDutyRule` 批准门、`OperatorSubject`（引用 + 授予集）、`PublicationApproval`（批准引用 = 批准者 / 来源 = 载体引用 / 时刻 = 批准时刻，逐字节确定）；规范化结果随带文档字节，`RehydratePublicationContent` / `RehydratePublicationDraft` | MCP-1 |
| `81b9a622` | `ports.PublicationDraftRegistry` / `ApprovalDutyRuleView`+`Registry`（新文件，`ports.go` 不动）；应用四用例 `PreviewCommercialPublication` / `SubmitPublicationDraft` / `ApprovePublicationDraft` / `PublishPublicationDraft`（收具体的 `*PublishCommercialAuthorityHandler`）；领域 `PreviewPublication` 与录入共用 `constructPublication` | MCP-1 |
| `c55127c0` | PC 迁移 **0028**（`publication_draft` 一版一行就地更新、`publication_approval_duty_rule` 一租户一条）；`PublicationDrafts`（ON CONFLICT DO UPDATE 带 WHERE + `xmax = 0`；推进带前态与摘要条件）、`ApprovalDutyRules`；领域 `SameSubmissionAs`；真库用例六组 | MCP-1 |
| `9e23eb3f` | 传输面：`CommercialPublicationPayload` 一份线格式供预览与录入同一段解码（身份 / 摘要键按未知键拒、逐格问题收齐为 `PublicationPayloadProblems`）；预览口、载体三口；`publicationAnswerOf` 抽出供两口共用；`UnconfiguredIntake` 长四口；领域 `CommercialObjectKindNamed` 导出 | MCP-1 |
| `ed61d8e3` | `cmd/parcel-api`：四行进端点表带未配置格 + 探针 + unwired 占位 + 第五族装配（发布用例与批文口**同一个实例**）+ 真库装配用例；PBC-08 门补三个写口负向证据；`CurrentPublicationCanonicalizationVersion` 无生产调用点、删（棘轮门） | MCP-1 |
| `d2b131e1` | 机制清点在 `ed61d8e3` 干净检出重生成 | MCP-1 |

**完成判据逐项**：

1. 三问裁决落 ADR-0126、PC CONTEXT（「发布规范化版本」「待批准发布」词条 + Rules「批准是操作者动作」一句）、参数登记册
   `PAR-COM-18`（待提供）——`6152b9f5`。
2. 服务端按册规范化 + 摘要覆盖信用政策一类（`PCC-1`），受控批文走甲：声明摘要必须与算出的相等、不等即 `NOT_ACCEPTED`
   带两串——`5baf3348`。其余九册在各自子票接进同一号不换号。
3. 待批准载体 + 批准动作 + 发布交既有 `PublishCommercialAuthorityHandler`，答案代数一格不改（发布口把它的整份答案嵌进自己的
   响应，同一个用例的答案两口不换形）；审批职责规则未登记时批准门答 `NOT_CONFIGURED`、不放行——`e8045f6b` / `81b9a622` /
   `c55127c0`。
4. 预览口 `POST /commercial-publication-previews` 与载体三口 `/commercial-publication-drafts`、`-draft-approvals`、
   `-draft-publications` 进端点表带未配置格——`ed61d8e3`。`tsc` / `run-tests` 不适用：本票不碰 `apps/admin-web`（逐册表单归
   09–17）；含 DSN `go test -p 1 -count=1 ./...` 见下。

**验证强度（钉 `ed61d8e3`，隔离 detached 检出 `%TEMP%\idp-verify-awf08`）**：`gofmt -l .` 空；`go build ./...` / `go vet ./...`
退 0；`internal/architecture` 全绿（棘轮、类型可达、enum、PBC-08、admin-web 端点消费者）；含 DSN `go test -p 1 -count=1 ./...`
退 0，**100 ok / 0 FAIL / 16 无测试 / 0 cached**（565s）；探针 `cmd/parcel-api -run TestTheWiredPublicationDraftPath`
无 DSN **SKIP** / 含 DSN **PASS**；PC postgres 包含 DSN 单跑 ok（60s）。证据层级 **S**（隔离合成）。

**实施中量到的两处（不改裁决，记下让子票别再撞）**：

- **载体区间未开时发布答 `DRAFT_AWAITS_EFFECTIVE_START`**：发布用例对挂在`已计划生效`版本上的声明整项拒（只增仓储没有日后补
  声明的口，届期改为到界发布——它的既有语义），而载体永远带正文，所以到界前不交给它，届期再发布。这是用例侧一格答案，不是 error。
  管理台表单落地时要把这一格显给操作者（子票 09–17 各自接时留意）。
- **「同一次录入」= 摘要 + 壳**（`SameSubmissionAs`）：换范围再录是修订不是重放，否则新范围会被静默丢掉。ADR-0126 Decision 三只说
  「换内容是修订」，这里把「内容」读成正文 + 壳；身份四元是键、不比。

**未做（各归其票）**：逐册表单页与草稿签（09–17）；审批职责规则的登记入口（治理写面那一族，ADR-0085 决定四另裁）；其余九册的
规范化文档与 seed 摘要换算（各子票）；旧式 `sha256:` 声明串何时开始拒收（伞票收口时裁）。ADR-0126 的越权风险点四条仍待 owner
复核（`scripts/owner-review-queue.ps1` 会列出）。
