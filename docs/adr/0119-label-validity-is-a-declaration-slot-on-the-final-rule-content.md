# ADR-0119：面单成功结果的有效期是终局规则声明上的一格——起算时刻种类 × 时长，一版至多一条、落在终局规则父行，缺席即不失效；起算时刻首发只开「渠道结果业务时间」；时长以区间原样登记，不预设日粒度、不给任何默认；随 `FINAL_RULE` 通道同一份正文登记

Status: Accepted（2026-09-07，MCP-6 按 MCP-1 派单 task-390c4f53「owner 授权自决口径：硬句不改、决定写理由、越权风险点单列供用户复核」裁决。裁决能力边界：读过票 [pc-gaps/09](../../.scratch/party-commercial-context-gaps/issues/09-final-rule-content-has-no-validity-declaration.md) 全文（六问与倾向）、票 [ps-port-remainder/01](../../.scratch/ps-port-remainder/issues/01-label-validity-rule-is-a-lapse-declaration-on-the-final-rule.md) 裁决节与 MCP-1 代裁 Comment、`internal/partycommercial/domain/service_stage_content.go` 的 `FinalRuleContent` 段全文、`migrations/party_commercial/0013` 的 `final_rule_*` 与 `0022` 同在同缺 CHECK 的写法、`adapters/postgres/stage_content_declaration.go` 的 `LoadFinalRule` 与 `declaration_publication.go` 的 `SaveFinalRule`、`ports.FinalRuleContentView`、发布用例 `FinalRuleChannel` 分支、`cmd/parcel-commercial/translate.go` 的 `finalRuleDocument`、PS `ports.LabelValidityRuleView` 头注与 `JudgeLabelServiceFinalHandler.lapsedTransactions`、PC CONTEXT「面单服务终局规则」词条与 Rules 上引三句、ADR-0058 / 0062；**未读** PS `LabelTransaction` 全文与 `label-channel-service-first-release/19` 的轨迹源有效时间规则目录——本记录因此只裁有效期声明的归属、形状与登记路径，不裁 PS 适配器怎么判失效）
Date: 2026-09-07

## Context

`parcel-shipment` 的 `ports.LabelValidityRuleView.JudgeLabelLapsed` 要按「接受时固定的有效期规则」判一笔成功面单结果是否已不可逆失效，`configured=false` 即规则未配置——「没有规则就没有失效，那笔成功照常阻止终局，不按墙钟推算过期」。仓内只有替身。通道 2 在票 [ps-port-remainder/01](../../.scratch/ps-port-remainder/issues/01-label-validity-rule-is-a-lapse-declaration-on-the-final-rule.md) 建模后裁：那条规则是**终局规则上的一格有效期声明**，PC 先长这一格，PS 消费适配器随后；MCP-1 代裁（owner 授权，2026-09-07）三问：**Q1** 起算时刻种类首发只开「渠道结果业务时间」；**Q2** 按 CONTEXT 字面归终局规则（接受时固定），不落产品—渠道映射侧；**Q3**「渠道确认」那一来源的失效不在本批。本记录是 PC 半边那一格的形状裁决，票 [pc-gaps/09](../../.scratch/party-commercial-context-gaps/issues/09-final-rule-content-has-no-validity-declaration.md) 承接实施。

**今天的形状（取证锚 `92579b0a`，本记录在 `250e5a43` 上复核了下列几处）。** `FinalRuleContent` = 拥有规则包版本 + `map[DeclaredResponsibilityOutcome]RuleReference`（责任结果 → 终局类型）；`NewFinalRuleContent` 守拥有者是已生效接单规则包、至少一行、同一结果不两行。`0013` 的 `final_rule_content` 父行只有四元组与 `declared_at`，子表 `final_rule_declaration` 按责任结果封闭四值进主键。读口 `FinalRuleContentView.LoadFinalRule` 无父行 = 未配置、父行在而零子行 = error；写口 `SaveFinalRule` 先读回再判重放 / 冲突；发布通道 `FinalRuleChannel`；批文 `declarations.finalRules` 是数组。**没有任何一格说有效期。**

**为什么它是机制半边。** PC CONTEXT Rules 写着「……此时才可依据明确失败、成功作废或**接受时固定规则下的不可逆失效**形成终局」，Boundaries 把「面单服务终局规则（正文挂在接单规则包版本下）」判给本上下文，Rules 又写「委托被接受时固定其适用的……面单服务终局规则」。语言在，格没有：租户即便有了规则也无处登。「有效期声明长什么样」（起算时刻种类的封闭集 + 一个时长；一版至多一条；缺席可分辨）由这三句推得出，一格不依赖租户取值；**时长是多少、要不要声明**是 `PAR-COM-17` 的实例半边，待提供。与 pc-gaps/07（ADR-0115）「CONTEXT 有语言、代码只有壳」同形。

## Decision

**一、有效期归终局规则声明，是它上面的一格，不是新一族、也不落产品—渠道映射侧。** 归属由 ADR-0058 决定一那一行（`FinalRuleContent` → 接单规则包版本）直接覆盖，本记录不改它；CONTEXT 两处都写「接受时固定的有效期规则」，接受时固定的是规则包版本（ADR-0062 的闭包钉住它），映射是「后续发生渠道选择时再固定」的另一层。**若日后真规则按渠道走，那是新一版声明的事，不是改归属**（MCP-1 代裁 Q2 原句）。一版至多一条：有效期说的是「这份终局规则下成功面单多久失效」，不按责任结果分——责任结果那一维答的是「哪种结果形成终局」，两维正交，硬拼进子表会让「有效期」在四行里重复四遍且必须相等。

**二、形状 = 起算时刻种类 × 时长。** 起算时刻种类是领域封闭集 `ValidityAnchorKind`，首发一值 `CHANNEL_RESULT_OBSERVED`（渠道结果业务时间，对应 PS `LabelTransaction.ResultObservedAt`）；做成类型而不是常量，因为加格是新一版声明的事，类型在，加格只改 CHECK 与 `valid()`。时长是一段**正**的持续时间，库上 `interval`、领域 `time.Duration`，不预设日粒度：锚是带时刻精度的业务时间，「N 天」是租户的选择不是产品的假设；整数天加单位那种表形会在租户要「72 小时」那天逼出一次迁移。**不按渠道产品细分**（Decision 一）。

**三、缺席由构造器分立表达，不用零值。** `FinalRuleContent` 长 `Validity() (LabelValidityDeclaration, bool)`；新构造门 `NewFinalRuleContentWithValidity(owner, declarations, validity)` 与既有 `NewFinalRuleContent` 分立——前者要求声明合法（种类在集内、时长为正），后者就是「没有这一格」。既有构造门的判据（拥有者、至少一行、不两行）一字不动，PS 消费侧既有调用点因此不必动。选分立构造器而不是「可选入参、零值 = 未声明」：零值 `time.Duration` 是 0，而 0 与「没声明」要人做的事不同——前者是坏声明该拒，后者是合法缺席；一个入参装不下这两件（判据同 ADR-0116 Decision 二「委派方恰一由两个构造器分立」）。

**四、库上落父行两列，同在同缺。** 新迁移 `0026` 在 `final_rule_content` 上 `ADD COLUMN validity_anchor text`、`validity_duration interval`，三条 CHECK：两列同在同缺（形照 `0022` 汇率口径三格全有或全无）、时长严格为正、种类在封闭集内。不改已施加的 `0013`。不另开 1:1 子表——多一张表换来的只是「缺席」多一种表达法，而缺席已由两列同为 NULL 说清；若哪天一版要多条有效期，那是 Decision 一被推翻，届时迁子表。

**五、随同一 `FINAL_RULE` 通道登记，批文加兄弟键。** 有效期是终局规则声明这一份正文的一部分，不另开发布通道（ps-port-remainder/01 裁「同一发布通道多一项正文」）：发布用例的 `CommercialDeclarations` 多一格 `FinalRuleValidity`，与 `FinalRules` 一起折进同一份 `FinalRuleContent` 再交 `SaveFinalRule`；**只给有效期不给终局规则行整项拒**——有效期没有独立的拥有者。受控批文 `declarations` 加兄弟键 `finalRuleValidity{anchor, duration}`，缺键 = 未声明，既有 seed 一字不改。`duration` 的批文形态取 ISO-8601 时长的一个**子集**：`P[nD][T[nH][nM][nS]]`，至少一项、整数、不接受年 / 月 / 周——年与月不是固定时长，周只是天的别写；解析在批文翻译层，集外拒收。写口的重放 / 冲突判据把有效期算进去：同行集合但有效期不同即`内容冲突`。

**六、不做的，逐条写明。**

- 不填任何时长、不给「30 天」之类默认；无声明恒不失效——`LoadFinalRule` 交回的内容里有效期缺席必须可分辨，不得因为终局规则其它行在场就把有效期当成已配置。
- 不在本上下文判失效：PC 只登声明，「`asOf ≥ 锚 + 时长`」是 PS 消费适配器（票 ps-port-remainder/01 PS 半边）读时算的事，本记录不裁它怎么算。
- 不开第二种起算时刻（面单签发时刻、委托接受时刻）。
- 不动 `DeclaredResponsibilityOutcome` 四值与 `FinalKindFor` 语义；不动 `NewFinalRuleContent` 既有判据。
- 不改管理台目录行 `AcceptanceRulePackageRow`：表单多一格归票 admin-write-faces/12，读面随那张票。

## Consequences

- 票 pc-gaps/09 落地：迁移 `0026`（父表加两列 + 三条 CHECK）；领域 `ValidityAnchorKind`、`LabelValidityDeclaration`、`FinalRuleContent.Validity`、`NewFinalRuleContentWithValidity`；postgres `LoadFinalRule` 读回、`SaveFinalRule` 写入且冲突判据含有效期；发布用例 `CommercialDeclarations.FinalRuleValidity` 折进 `FinalRuleChannel`；受控批文 `finalRuleValidity` 兄弟键与 ISO-8601 子集解析；真库往返。
- PS 半边（票 ps-port-remainder/01）据此解阻：`adapters/partycommercial/label_validity_rule.go` 经 `AdoptedStageOwner` 回指接受时固定的规则包版本 → `LoadFinalRule` → `Validity()` → 以 `ResultObservedAt` 为锚判 `lapsed`；声明缺席 → `configured=false`。
- 管理台：票 admin-write-faces/12 终局规则节多一格有效期（起算时刻种类 + 时长），由那张票自己加，本记录不建表单也不改目录行。
- `PAR-COM-17` 在参数登记册里多一项「面单有效期（起算时刻种类、时长）」待提供；登记面从此在终局规则声明。登记册行由其所有者改口径，本记录不代改。
- CONTEXT：「面单服务终局规则」词条补一段（有效期一格、一版至多一条、首发一格起算时刻、缺席即不失效、判失效归 PS）。Rules 各句不改。

### 越权风险点（单列，供 owner 复核）

- **时长用 `interval` / `time.Duration` 而不是「整数天」。** 这是对「接受时固定的有效期规则」的一次解释：锚是带时刻精度的业务时间，所以时长也按时刻精度登。若 owner 认为面单有效期在商业上只以日计，改一格是批文子集收窄到 `PnD` 与 CHECK 加一条整日约束，领域类型不换。
- **有效期落父行、一版至多一条。** 若 owner 认为同一版终局规则要按责任结果各给不同有效期，Decision 一被推翻，迁子表；今天 PS 端口只问「这一笔成功结果」，没有按结果分的消费形状，所以我没取。
- **批文子集排除年 / 月 / 周。** 排除年月是因为它们不是固定时长（无法与 `interval` 的微秒段无损往返）；排除周是为了子集最小。若租户真按「自然月」失效，那不是本记录的形状能装的，要回到 owner。

## Alternatives considered

- **有效期落产品—渠道映射侧（渠道约束）。** 否决：MCP-1 代裁 Q2 按 CONTEXT 字面归终局规则；接受时固定的是规则包版本，映射是渠道选择时才固定的另一层，PS 的回指路径也会随之从 `AdoptedStageOwner` 换成「该交易实际使用的映射」。
- **1:1 子表。** 否决：Decision 四——缺席已由同为 NULL 说清，多一张表只多一种表达法。
- **整数 + 单位封闭集（天 / 小时）。** 否决：Decision 二——预设粒度是产品替租户作的假设。
- **`NewFinalRuleContent` 加可选入参、零值 = 未声明。** 否决：Decision 三——零时长与没声明要人做的事相反，一个入参装不下。
- **另开一条 `FINAL_RULE_VALIDITY` 发布通道。** 否决：有效期没有独立的拥有对象，独立通道会让「只给有效期不给终局规则行」在结构上写得出来。
- **批文时长收 PostgreSQL interval 串。** 否决：那是持久化面的方言，写批文的人要学库的语法；ISO-8601 子集在翻译层解析，集外拒收。
- **起算时刻种类首发就开三格。** 否决：MCP-1 代裁 Q1 只开一格；加格是新一版声明的事，不是默认。

## Links

- 票 [pc-gaps/09](../../.scratch/party-commercial-context-gaps/issues/09-final-rule-content-has-no-validity-declaration.md)：缺口事实、六问与倾向、实施与验收
- 票 [ps-port-remainder/01](../../.scratch/ps-port-remainder/issues/01-label-validity-rule-is-a-lapse-declaration-on-the-final-rule.md)：PS 半边与 MCP-1 代裁（Q1 一格 / Q2 归终局规则 / Q3 不在本批）
- [ADR-0058](./0058-stage-content-owned-by-rule-objects.md)：声明归拥有规则对象——Decision 一「归属不改」的依据
- [ADR-0062](./0062-adopted-stage-owner-from-accepted-resolution.md)：回指接受时固定的规则包版本——「接受时固定」在 PS 侧的落点
- [ADR-0116](./0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md)：Decision 二「恰一由两个构造器分立」——Decision 三同一判据
- [party-commercial CONTEXT](../domain/party-commercial/CONTEXT.md)：「面单服务终局规则」词条与 Rules「接受时固定规则下的不可逆失效」——本记录给那句第一份可登记的形状
- `internal/partycommercial/domain/service_stage_content.go`：`FinalRuleContent` / `NewFinalRuleContent`——本记录在其旁加一格与一个构造门，不改既有判据
- `migrations/party_commercial/0013_stage_content_declarations.sql`：`final_rule_content` 父行——`0026` 在其上加列
- `migrations/party_commercial/0022_price_policy_caliber.sql`：同在同缺 CHECK 的写法先例
- PS `ports.LabelValidityRuleView` 头注与 `application/judge_label_service_final.go` 的 `lapsedTransactions`：消费方对缺席的读法（`configured=false` 不失效）
- `PAR-COM-17`：实例半边的出处
