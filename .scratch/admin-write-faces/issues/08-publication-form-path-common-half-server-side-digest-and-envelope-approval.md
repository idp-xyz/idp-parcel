# 08 商业发布表单路径的公共半边：内容摘要改由服务端按册规范化算出、批准从操作者信封来、预览与发布同一路径

Category: enhancement
Status: ready-for-agent——伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出的第一张，其余各册子票全部 Blocked by 本票（MCP-6 2026-09-07 立，锚 `95182b9d`）
Blocked by: 无

## 为什么每册子票都先卡在这里

伞票的硬句「表单不算摘要、不裁任何门：`contentDigest` 与受理都由服务端答，表单只呈现」，今天的发布口做不到——
它**要求调用方声明摘要**：

- 受控批文（`cmd/parcel-commercial/translate.go` 的 `publicationItemDocument`）每一项带 `contentDigest`，域上
  `NewCommercialContentDigest` 只查非空，**不从正文算、也不与正文对**；`application/publish_commercial_authority.go`
  里没有一处提到摘要。今天「同键异内容答`内容冲突`」（ADR-0031）比的是调用方声明的那个串。
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
