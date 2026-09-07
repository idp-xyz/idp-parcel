# 08 商业发布表单路径的公共半边：内容摘要改由服务端按册规范化算出、批准从操作者信封来、预览与发布同一路径

Category: enhancement
Status: in-progress——三问由 [ADR-0126](../../../docs/adr/0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md) 一次答完（2026-09-08，MCP-5 按 MCP-1 派单 task-cc7313e8「owner 授权自决口径」裁，越权风险点四条单列在 ADR 里供 owner 复核），对号见下面「裁决」节；PC CONTEXT 词条「发布规范化版本」「待批准发布」+ Rules 一句、参数登记册 `PAR-COM-18` 已随 ADR 同笔落；实施（服务端规范化 + 摘要首例信用政策 → 待批准载体 + 批准门 → 预览口进端点表）按「完成判据」逐笔接。ADR-0127 预留号未用、释回。此前 draft——伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出的第一张，其余各册子票全部 Blocked by 本票（MCP-6 2026-09-07 立，锚 `95182b9d`）。**不转 ready-for-agent 的理由**：下面「要裁的」三问都是新裁决（摘要由谁算与受控批文那一半走甲还是乙、要不要待批准载体、预览口形状），伞票收口纪律是「形状裁清且不需要新裁决才转 ready」。**等什么**：(1) MCP-1 给 ADR 取号；(2) 三问由 owner 裁，或 owner 授权认领人经 `/domain-modeling` 自决——授权到了，认领人把三问裁决落进本票「裁决」节与 ADR，再转 in-progress 开工（裁与做可同人同票，但裁决先于代码）。接管会话 2026-09-07 于 `92579b0a` 逐句核过票面取证（`translate.go` 的 `contentDigest` / `approval` / `approvalRoleStanding` 三格、`NewCommercialContentDigest` 只查非空、`/commercial-publications` 挂 `UnconfiguredIntake{}`），见伞票 Comments
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
