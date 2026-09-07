# 终局规则声明没有「有效期」这一格——「接受时固定的有效期规则」在 PC 无处登记，PS 的面单失效判断因此永远答未配置

Category: enhancement
Status: resolved——2026-09-07，MCP-6（task-390c4f53，分支 `mcp6-pcgaps09` 基 `250e5a43`）：ADR-0119 六问一次答完（越权风险点三条单列在 ADR 里供 owner 复核）、PC CONTEXT「面单服务终局规则」词条补段、迁移 0026、领域 / PG / 发布用例 / 批文全部落地，真库往返 PASS；完成记录见 Comments 末条。此前 in-progress：六问由 [ADR-0119](../../../docs/adr/0119-label-validity-is-a-declaration-slot-on-the-final-rule-content.md) 一次答完（2026-09-07，MCP-6 按 MCP-1 派单 task-390c4f53「owner 授权自决口径」裁，越权风险点三条单列在 ADR 里供 owner 复核），PC CONTEXT「面单服务终局规则」词条补段已随 ADR 同笔落；实施（迁移 0026 → 领域 → PG 读写 → 发布用例 → 批文 → 真库往返）按下面「要建什么」逐笔接。此前 draft：PC 半边（`FinalRuleContent` 长一格有效期声明 + 读口 + 发布通道）由 MCP-1 2026-09-07 代裁归 PC 批（原文在
[ps-port-remainder/01](../../ps-port-remainder/issues/01-label-validity-rule-is-a-lapse-declaration-on-the-final-rule.md) 裁决节与 Comments，取证时该目录只在分支 `mcp2-ps-ports` 上、尚未进 main）；本票是那一格在 PC 侧的建模票，待 `/domain-modeling` 答完下面「要你答的问题」再转 ready-for-agent。ADR 号 **0119** 由 MCP-1 预留（task-aafca372）——ps-port-remainder/01 裁 ④ 写「不要 ADR，PC owner 落地时若认为改了 `FinalRuleContent` 的领域形状要记则自裁」，预留号是给那个「若」用的，建模那一步定写不写；迁移号以开工那刻 `party_commercial` 最大序号 + 1 重取（立票时最大 `0024`）
Blocked by: 无

## 从哪里来

PS 的 `ports.LabelValidityRuleView.JudgeLabelLapsed` 要按「接受时固定的有效期规则」判一笔成功面单结果是否已不可逆失效，仓内只有替身；
通道 2 建模后裁：那条规则是**终局规则上的一格有效期声明**，PC 先长这一格，PS 消费适配器随后。MCP-1 代裁（owner 授权，2026-09-07）：
**Q1** 起算时刻种类首发只开「渠道结果业务时间」一格；**Q2** 按 CONTEXT 字面归终局规则（接受时固定），不落产品—渠道映射侧——日后真规则
按渠道走，那是新一版声明不是改归属；**Q3**「渠道确认」那一来源的失效不在本批。本票只承接 PC 那一半。

## 缺口事实（锚 main `92579b0a`）

- `internal/partycommercial/domain/service_stage_content.go`：`FinalRuleContent` = 拥有规则包版本 + `map[DeclaredResponsibilityOutcome]RuleReference`
  （责任结果 → 终局类型）；`NewFinalRuleContent` 守拥有者是已生效接单规则包、至少一行、同一结果不两行。**没有任何一格说有效期。**
- `migrations/party_commercial/0013_stage_content_declarations.sql`：`final_rule_content`（父，声明壳）+ `final_rule_declaration`（子，
  `outcome` 封闭四值进主键 × `final_kind`）；父行上只有四元组与 `declared_at`。
- 读口 `pcports.FinalRuleContentView.LoadFinalRule`（`PAR-COM-17`）：无父行 = 未配置，父行在而零子行 = error；写口 `PublicationRegistry.SaveFinalRule`；
  发布通道 `application.FinalRuleChannel`（`FINAL_RULE`）；受控批文 `declarationsDocument.FinalRules []finalRuleDocument{outcome, finalKind}`。
- PC CONTEXT Rules：「……此时才可依据明确失败、成功作废或**接受时固定规则下的不可逆失效**形成终局」；Boundaries：「面单服务终局规则
  （正文挂在接单规则包版本下）」；Rules：「委托被接受时固定其适用的……面单服务终局规则」。语言在，格没有。
- PS 侧（消费方，不在本票）：`JudgeLabelServiceFinalHandler.lapsedTransactions` 对 `configured=false` 一律不失效；锚点时间是
  `LabelTransaction.ResultObservedAt()`。

## 为什么是机制半边

「有效期声明长什么样」（起算时刻种类的封闭集 + 一个时长；一版至多一条；缺席可分辨）由 CONTEXT 硬句推得出，一格不依赖租户取值；
**时长是多少、要不要声明**是 `PAR-COM-17` 的实例半边，待提供。今天没有这一格，租户即便有了规则也无处登——与 pc-gaps/07「CONTEXT 有
语言、代码只有壳」同形。拒默认落在「无声明 → `LoadFinalRule` 交回的内容里有效期缺席 → PS 译 `configured=false` → 不失效」，既有语义一字不改。

## 要你答的问题（`/domain-modeling` 一次答完；答案落 PC CONTEXT「面单服务终局规则」词条 +（若写）ADR-0119）

1. **有效期声明落在父行还是子表？** 一版至多一条 → 倾向 **父行加两列**（起算时刻种类 + 时长），CHECK「同在同缺」（形照 `0022`
   汇率口径三格同在同缺的写法），新迁移 `ALTER TABLE final_rule_content ADD COLUMN …`，不改 `0013`。替代是 1:1 子表——多一张表换来的
   只是「缺席」多一种表达法，而缺席已由 NULL 说清。
2. **起算时刻种类的封闭集与词。** 按代裁首发一格：`CHANNEL_RESULT_OBSERVED`（渠道结果业务时间，对应 PS `ResultObservedAt`）。要不要
   在领域里就把它做成封闭集类型（`ValidityAnchorKind`）而不是常量？倾向要——加格是新一版声明的事，类型在，加格只改 CHECK 与 `valid()`。
3. **时长的形态。** 候选：(a) PostgreSQL `interval` ↔ Go `time.Duration`，CHECK `> interval '0'`；(b) 整数 + 单位封闭集（天 / 小时）。
   倾向 **(a)**：锚是带时刻精度的业务时间，天粒度是租户的选择不是产品的假设；`interval` 直接表达，不为「N 天」预设单位。
4. **领域形状。** 倾向 `FinalRuleContent` 直接带 `Validity() (LabelValidityDeclaration, bool)`（一次 `LoadFinalRule` 读回，PS 适配器本来就要
   调它），不在 `FinalRuleContentView` 上再开一个方法。`NewFinalRuleContent` 多一个可选入参还是另起 `WithValidity`？倾向可选入参
   （零值 = 未声明），构造门守「声明在则种类合法且时长为正」。
5. **发布批文的键。** `declarations.finalRules` 今天是数组；有效期是版本级 → 倾向在 `declarationsDocument` 加**兄弟键**
   `finalRuleValidity{anchor, duration}`，`translate.go` 把它与 `finalRules` 折进**同一份** `FinalRuleContent`、同一 `FinalRuleChannel`
   （ps-port-remainder/01 裁「同一发布通道多一项正文」）；缺键 = 未声明，既有 seed 一字不改。`duration` 的批文形态取 ISO-8601 时长串
   还是 PostgreSQL interval 串？倾向 ISO-8601（`PT72H` / `P3D`），解析在批文翻译层，集外拒收。
6. **admin-write-faces/12 的表单**随之在终局规则节多一格——本票落地时只改票 12 的一句，不建表单。

## 裁决（2026-09-07，MCP-6；正文在 ADR-0119，此处只对号）

① 父行加两列（起算时刻种类 + 时长），同在同缺 CHECK，新迁移 `0026` `ALTER TABLE`，不改 `0013`，不开 1:1 子表（Decision 四）；② 起算时刻种类做成封闭集类型 `ValidityAnchorKind`，首发一值 `CHANNEL_RESULT_OBSERVED`（Decision 二）；③ 时长取 (a)：PostgreSQL `interval` ↔ Go `time.Duration`，CHECK 严格为正，不预设日粒度（Decision 二）；④ `FinalRuleContent.Validity() (LabelValidityDeclaration, bool)`，**不用可选入参**——新构造门 `NewFinalRuleContentWithValidity` 与既有 `NewFinalRuleContent` 分立表达缺席，既有判据一字不动（Decision 三，与倾向不同：零时长与没声明要人做的事相反，一个入参装不下）；⑤ 批文加兄弟键 `finalRuleValidity{anchor, duration}`，翻译层折进同一份 `FinalRuleContent`、同一 `FinalRuleChannel`；`duration` 取 ISO-8601 子集 `P[nD][T[nH][nM][nS]]`，禁年 / 月 / 周，只给有效期不给终局规则行整项拒；冲突判据把有效期算进去（Decision 五）；⑥ admin-write-faces/12 终局规则节加一格，本票只改票 12 一句（Consequences）。**越权风险点三条**（时长按时刻精度而非整数天 / 一版至多一条落父行 / 批文子集排除年月周）单列在 ADR-0119，等 owner 复核。

## 要建什么（裁决已落，逐笔实施；每笔 pathspec 提交）

PC CONTEXT 词条补一句 →（若写）ADR-0119 → 迁移（父表加两列 + 同在同缺 CHECK + 正时长 CHECK）→ 领域（`ValidityAnchorKind`、
`LabelValidityDeclaration`、`FinalRuleContent.Validity`）→ postgres 读写口跟随（`LoadFinalRule` 读回、`SaveFinalRule` 写入，同内容重放 /
异内容冲突判据把有效期算进去）→ 发布用例 `FinalRuleChannel` 收它 → `cmd/parcel-commercial` 批文 `finalRuleValidity` →
真库端到端读回（形照 pc-gaps/07 四笔 `6dc3db12..265da8d8`）。PS 适配器 `label_validity_rule.go` 不在本票。

## 在等本票的

- `ps-port-remainder/01` PS 半边：`adapters/partycommercial/label_validity_rule.go` 实现 `LabelValidityRuleView`（包裹 → 当前已接受委托 →
  `AdoptedStageOwner.AcceptanceRulePackageFor` → `LoadFinalRule` → 有效期 → 以 `ResultObservedAt` 为锚判 `lapsed`）。
- 连带（不是等）：`admin-write-faces/12` 终局规则节加一格。

## 红线

- 不填任何时长；不给「30 天」之类默认；不拿墙钟推算过期——PC 只登声明，判失效是 PS 读时按声明算。
- 无声明恒不失效：`LoadFinalRule` 交回的内容里有效期缺席必须可分辨，不得因为终局规则其它行在场就把有效期当成已配置。
- 起算时刻种类首发只开一格，不预开「面单签发时刻」「委托接受时刻」。
- 不改已施加迁移 `0013`；不改 `DeclaredResponsibilityOutcome` 四值与 `FinalKindFor` 语义；`NewFinalRuleContent` 的既有判据（拥有者、
  至少一行、不两行）一字不动。
- 领域包不依赖 HTTP / pgx；集外取值报错不吸收；同内容重放 / 异内容冲突照 `DeclarationSaveOutcome` 既有代数。
- 撞上要把有效期归到产品—渠道映射侧、或要改「接受时固定」硬句 → 停下报 MCP-1（代裁 Q2 已答，改口要回到 owner）。

## 参照

`internal/partycommercial/domain/service_stage_content.go`（`FinalRuleContent` / `NewFinalRuleContent` / `FinalKindFor` / `Declarations`）；
`pcports.FinalRuleContentView` 与 `PublicationRegistry.SaveFinalRule`；`application.FinalRuleChannel`；`cmd/parcel-commercial/translate.go`
的 `declarationsDocument.FinalRules`；`migrations/party_commercial/0013_stage_content_declarations.sql` 的 `final_rule_*` 与
`0022_price_policy_caliber.sql` 同在同缺 CHECK 的写法；PC CONTEXT「面单服务终局规则」词条与 Rules 上引三句；ADR-0058（归属）、
ADR-0062（回指）；`PAR-COM-17`；PS `ports.LabelValidityRuleView` 头注与 `application/judge_label_service_final.go` 的 `lapsedTransactions`。

## Comments

- 2026-09-07 · MCP-6（task-aafca372，分支 `mcp6-awf07`）：立票（draft）。**只写票面，未动代码、未改 CONTEXT。** 素材是
  `ps-port-remainder/01` 的裁决节与 MCP-1 代裁 Comment（`git show mcp2-ps-ports:…`）；PC 侧事实在 `92579b0a` 上逐条重取
  （`service_stage_content.go` 的 `FinalRuleContent` 段、`0013` 全文、`ports.go` 的 `FinalRuleContentView` 段、`DeclarationChannel`
  封闭集、`translate.go` 的 `finalRuleDocument`）。能力边界：**没读** `FinalRuleChannel` 在 `PublishCommercialAuthorityHandler.Handle`
  里的分支细节与 postgres `SaveFinalRule` 的同内容判据实现——问题 5「冲突判据把有效期算进去」怎么落，开工时以代码为准。
- 2026-09-07 18:09 · MCP-6（17:4x 新绑会话，按 MCP-1 18:05 广播补记）：**封存出处。** 立票笔 `8c1e6ed5`（分支 `mcp6-awf07`）
  → main `52001f02`（远端 main = `e374b1ea`）。对照全表在 `admin-write-faces/07` 的同时刻 Comment。
- 2026-09-07 20:3x · MCP-6（task-390c4f53，分支 `mcp6-pcgaps09` 基 `250e5a43`）：**裁决落 ADR-0119**，六问按上面「裁决」节取
  （问题 4 与倾向不同：分立构造器而非可选入参，理由在 ADR Decision 三）；PC CONTEXT「面单服务终局规则」词条补段，本票转
  in-progress。同笔只有文档；代码从下一笔起。能力边界写在 ADR 头部。
- 2026-09-07 20:5x · MCP-6（task-390c4f53，分支 `mcp6-pcgaps09`）：**完成记录，转 resolved。** 逐笔：`e078a508` docs（ADR-0119 +
  README 一行 + CONTEXT 词条补段 + 本票 in-progress）；`7ea33e60` feat（迁移 `0026_final_rule_label_validity.sql`——`final_rule_content`
  加 `validity_anchor` / `validity_duration`，同在同缺 / 时长为正 / 种类封闭三条 CHECK；领域 `ValidityAnchorKind`、
  `LabelValidityDeclaration`、`FinalRuleContent.Validity()`、`NewFinalRuleContentWithValidity`；postgres `LoadFinalRule` 读回按在不在场选
  构造门、`SaveFinalRule` 写入且重放 / 冲突判据含有效期，helper 在 `final_rule_validity.go`；发布用例
  `CommercialDeclarations.FinalRuleValidity` 折进同一 `FinalRuleChannel`，只给有效期不给终局规则行整项拒；批文
  `declarations.finalRuleValidity{anchor, duration}` 兄弟键 + `parseISODurationSubset`）；票 12 一句。**六问对号**：①父行两列 ②封闭集类型
  ③`interval` ↔ `time.Duration` ④分立构造器（与倾向不同，理由 ADR Decision 三）⑤兄弟键 + ISO-8601 子集 ⑥票 12 一句。
  **验收**：真库（DSN 指向门禁容器，`-v` 下 PASS）——带有效期按时刻精度往返（84h30m）、未声明读回即没有、同行同有效期重放 /
  换时长冲突 / 去掉有效期也冲突且原行不动、CHECK 拒半缺 / 零 / 负 / 集外种类、两列皆空的既有形状仍放行；应用层四格（同通道登记 /
  缺键即没有 / 只给有效期拒 / 挂错拥有对象拒）；翻译层子集一正一反各一表。**不做的**：PS 适配器 `label_validity_rule.go`
  （ps-port-remainder/01 PS 半边，据此解阻）；`AcceptanceRulePackageRow` 目录不加格（随 admin-write-faces/12）；起算时刻不加格。
  **共享接线文件各加了哪一段**：`publish_commercial_authority.go`（`CommercialDeclarations` 一格 + `declarationWrites` 终局规则分支，
  gofmt 重对齐前七字段属纯格式）、`declaration_publication.go`（`SaveFinalRule` 一函数）、`stage_content_declaration.go`
  （`LoadFinalRule` 一函数 + pgtype 导入）、`translate.go`（`declarationsDocument` 一键 + 三个函数 + strconv 导入）、
  `docs/adr/README.md`（0118 与 0122 之间一行）、PC CONTEXT（一个词条末尾一段）。SHA 只作此刻取证，MCP-1 重放后以 main 上的为准。
