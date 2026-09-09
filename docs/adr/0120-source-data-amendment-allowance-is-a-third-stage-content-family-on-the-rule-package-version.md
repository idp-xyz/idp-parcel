# ADR-0120：资料修订允许声明是接单规则包版本下的第三族阶段内容声明——父行带「封闭」标记、子行按（资料组 × 阶段 × 意图）逐格登记允许 / 不允许，永不登「未声明」；缺格未封闭读「未声明」、封闭读「不允许」；三值由本上下文算出；阶段与意图两个封闭集取 `parcel-shipment` 原词只镜像不另定；随新通道 `SOURCE_DATA_AMENDMENT` 登记

Status: Accepted（2026-09-07，MCP-6 按 MCP-1 派单 task-390c4f53「owner 授权自决口径：硬句不改、决定写理由、越权风险点单列供用户复核」裁决。裁决能力边界：读过票 [pc-gaps/10](../../.scratch/party-commercial-context-gaps/issues/10-source-data-amendment-allowance-is-a-third-stage-content-family.md) 全文（七问与倾向）、票 [ps-port-remainder/02](../../.scratch/ps-port-remainder/issues/02-source-data-amendment-allowance-is-a-third-stage-content-declaration.md) 裁决节与 MCP-1 代裁四问、`migrations/party_commercial/0013` 三族全文、`internal/partycommercial/domain/service_stage_content.go` 全文（三族构造门与 `FinalKindFor` / `RuleFor` 的两值读法）、`ports.IntakeQualificationView` / `FinalRuleContentView` 头注与 `PublicationRegistry` 具名 Save 一族、`adapters/postgres/stage_content_declaration.go` 的 `LoadFinalRule` 与 `declaration_publication.go` 的 `SaveFinalRule`、发布用例 `CommercialDeclarations` / `DeclarationChannel` / `declarationWrites`、`cmd/parcel-commercial/translate.go` 的 `declarationsDocument`、PS `domain.AmendmentStage`（main `7bc47ec9` 起在）/ `AmendmentIntent` / `SourceDataScope` / `SourceDataGroupReference`、PS `ports.SourceDataRuleDeclaration` / `SourceDataAmendmentQuery` / `SourceDataAmendmentAllowance` 头注、ADR-0058 / 0115 / 0118 / 0119、PC CONTEXT Rules 上引两句；**未读** PS `adapters/partycommercial/stage_content_declarations.go` 全文与 `intake_qualification_*` 的 postgres 读写实现——本记录因此只裁这一族在 PC 侧的归属、形状、缺格语义与登记路径，不裁 PS 消费适配器怎么回指、怎么译）
Date: 2026-09-07

## Context

`parcel-shipment` 的 `ports.SourceDataRuleDeclaration.DeclareSourceDataAmendment(query)` 要按已登记规则回答一处资料范围在当前阶段、以某一意图能不能改，三值 `NotDeclared` / `Allowed` / `Disallowed`，零值 `NotDeclared`；头注写明「字段、字段组、阶段与允许动作由 `PAR-COM-13` 与真实合同、产品、线路和关务规则登记，属实例半边；本上下文只消费登记结论，绝不自带一份矩阵」。仓内只有 `sourceDataRuleDouble`。通道 2 在票 [ps-port-remainder/02](../../.scratch/ps-port-remainder/issues/02-source-data-amendment-allowance-is-a-third-stage-content-declaration.md) 建模后裁：矩阵正文归 PC，作接单规则包版本下的**第三族**阶段内容声明；「此刻在哪个阶段」的判断归 PS。MCP-1 代裁（owner 授权，2026-09-07）四问：**Q1** 同意归属；**Q2** 阶段封闭集照 `UC-PS-002` 六格原词、不拆；**Q3** 缺格默认 `NotDeclared`，父行带「封闭」标记时缺格读 `Disallowed`；**Q4** CC/NO 阶段事实进 PS 取消费侧读口（ADR-0118，PS 已落）。本记录是 PC 半边那一族的形状裁决，票 [pc-gaps/10](../../.scratch/party-commercial-context-gaps/issues/10-source-data-amendment-allowance-is-a-third-stage-content-family.md) 承接实施。

**今天的形状（取证锚 main `9379c716`）。** `0013` 在接单规则包版本（`object_kind = 4`）下挂两族：`intake_qualification_*`（父 + 允许来源子表 + 资格引用子表）与 `final_rule_*`（父 + 按责任结果的子表）；取消授权目录挂授权规则版本（`object_kind = 9`）。三族的读法一致：无父行 = 未配置（`found=false`）、父行在而零子行 / 坏子行 = error；领域构造门都守「拥有者是已生效的对应规则种类、至少一行、同键不两行」；逐格读法都是两值——`FinalKindFor(outcome) (kind, declared)`、`RuleFor(party) (rule, declared)`——缺行是真话（不形成终局 / 不许取消）。**没有任何一族按（资料组 × 阶段 × 意图）说「能不能改」。** `pcports` 只有两个规则包声明读口；`application.DeclarationChannel` 封闭集与 `translate.go` 的 `declarationsDocument` 都没有资料修订那一格。PS 侧：`SourceDataAmendmentQuery{Identity, Scope, Intent, Reason, Stage}`；`domain.AmendmentStage` 六格、零值`判不出`；`AmendmentIntent` 三格 `SUPPLEMENT` / `CORRECTION` / `EXPLICIT_CLEAR`，零值不合法；`SourceDataGroupReference` 是开放引用（`UC-PS-002` 只禁模糊指代）。

**为什么它是机制半边。** 矩阵的**三维**（资料组引用 × 修订阶段 × 修订意图）与**两值**（允许 / 不允许）由 `UC-PS-002`「资料范围与阶段边界」、`AT-PS-020`（意图进矩阵：显式清空与改成新值在同一阶段的允许性可以相反）、PS CONTEXT「显式清空必须由版本化字段和阶段规则允许」三处硬句推得出，一格不依赖租户取值；PC CONTEXT Rules 又写着规则包「区分接入最小身份、委托通用不变量、产品与合同资料、监管原始资料及跨字段条件」「只装配各权威上下文的规则引用」——规则包本就按资料组说话。**每一格填什么、资料组怎么划、哪些阶段对哪些资料组开放**是 `PAR-COM-13` / `BD-PS-010` 的实例半边，待提供。今天没有这一族，租户即便有了规则也无处登；PS 端口头注禁它自带矩阵——缺口只能在 PC 补。与 pc-gaps/07（ADR-0115）、pc-gaps/09（ADR-0119）「CONTEXT 有语言、代码只有壳」同形。

## Decision

**一、归属：接单规则包版本，第四件按 ADR-0058 决定一的同一条纪律，不改那条纪律。** ADR-0058 决定一的表列三件（收寄资格、终局规则 → 规则包版本；取消授权 → 授权规则版本）并写明「规则包正文（B7）仍是另一族，本记录不把它预支进本表」；资料修订允许声明是第四件，理由与前两件同一句：委托被接受时固定的是所采用的规则包版本（ADR-0062 的闭包钉住它），产品与合同是采用方不是拥有方；PS 的消费适配器也正是经 `AdoptedStageOwner.AcceptanceRulePackageFor` 回指规则包版本去读（ps-port-remainder/02 裁决 (c)）。挂授权规则版本被否决：授权规则答「谁能做」（资料修订是与人工复核、主动拒绝并列的授权动作，ADR-0116），本族答「能改什么」，两问分属两个对象，CONTEXT 词条「合同委派」那一句已经把三问分开说了。**本记录不改 ADR-0058 的表**，只在其 Links 加一行回指；「归属不要 ADR」那一半按 ps-port-remainder/02 裁 ④ 成立，本记录立 ADR 是为下面二、三、四三条取舍（见「七」）。

**二、形状 = 父行 + 逐格子行，形照 `0013` 两族，不用一个 JSON 列。** 新迁移 `0027`：父表 `source_data_amendment_content`（四元组 + `object_kind = 4` CHECK + **`closed boolean NOT NULL`** + `declared_at`）；子表 `source_data_amendment_allowance`（四元组 × `data_group_ref` × `stage` × `intent` → `allowance`），主键含三维，`stage` 六格、`intent` 三格、`allowance` 两格（`ALLOWED` / `DISALLOWED`）三个封闭集进 CHECK，`data_group_ref` 非空进 CHECK。否决整份矩阵一个 JSON 列的理由与 ADR-0115 Decision 二同一条：逐格可查、逐格可约束——CHECK 守得住三个封闭集，JSON 守不住；PS 适配器查的是一格，不该为一格解一整份。子表名取 `_allowance` 而不是票面倾向的 `_rule`：这一族没有规则引用列（对照 `final_rule_declaration.final_kind`、`cancellation_authority_declaration.rule_reference`），一行就是一格允许性，叫 `rule` 会让人去找不存在的那一列。不改已施加的 `0013`。

**三、缺格 / 零行 / 封闭的语义（MCP-1 代裁 Q3 的落地）。** 子行只登 `ALLOWED` / `DISALLOWED`，**永不登「未声明」**——「未声明」是缺格的读法不是一行的取值，把它登成一行会让「登记方没说」与「登记方说了『我不说』」在表里分不开。缺格 + `closed = false` → 「未声明」；缺格 + `closed = true` → 「不允许」——「未登记的字段和阶段不默认允许」（`BD-PS-010`）两种读法都满足，差别只在客户看到的是「等复核」还是「被拒」，那是登记方该显式说的话，所以它是父行上一格布尔而不是产品的默认。**零行父行**：`closed = true` 零行 = 「这一版什么都不许改」，是一句显式的话，允许登记；`closed = false` 零行 = 什么都没说，构造门拒（`ErrSourceDataAmendmentNotConfigured`，判据同 `NewFinalRuleContent` 拒零行）。`closed = true` 下的 `DISALLOWED` 行是冗余不是冲突，放行——显式重复一遍「不许」不该被拒。同一（资料组, 阶段, 意图）两行是冲突（`ErrConflictingSourceDataAmendment`）。库表达不了「未封闭要至少一行」，与 `0013` 头注同一句：无父行 = 未配置；父行在而正文立不住 = 装载 error，不得折成未配置。

**四、三值在本上下文算出，封闭标记的读法留在声明的拥有者这边。** 领域 `SourceDataAmendmentAllowanceContent.AllowanceFor(group, stage, intent) AmendmentAllowance`，三值 `AmendmentAllowanceNotDeclared`（零值）/ `AmendmentAllowed` / `AmendmentDisallowed`：有格按格答，缺格按 Decision 三答。PS 消费适配器只做 PC 三值 → PS `SourceDataAmendmentAllowance` 三值的一对一翻译（ADR-0025 消费方只翻译）。否决「PC 只交回行集合、PS 自己查格并解释 `closed`」：那会把「封闭意味着什么」这条 PC 的规则搬到消费方，下一个消费方（管理台读面、审计）还得再抄一遍；判据与 ADR-0063「声明列出之后如何取证归消费方，声明本身怎么读归提供方」同一条。零值落在「未声明」而不是「允许」：与 PS 端口同一个理由——零值必须落在最保守的那一格。

**五、阶段与意图两个封闭集的词，单一权威在 `parcel-shipment`，本上下文只镜像不另定。** 六格原词的权威是 PS CONTEXT 词条「资料修订阶段」与 `domain.AmendmentStage`（ADR-0118 / MCP-1 代裁 Q2），三格意图的权威是 PS `domain.AmendmentIntent`。两个限界上下文的领域包互不 import，所以 PC 领域立自己的封闭集 `DeclaredAmendmentStage`（六格）与 `DeclaredAmendmentIntent`（三格），`String()` 与 PS 逐字相等；命名取 `Declared*` 前缀，与本族既有的三个镜像（`DeclaredIntakeSource` / `DeclaredResponsibilityOutcome` / `DeclaredCancellationParty`——「本上下文对那几格的引用枚举」）同形，不取票面倾向的 `*Key`。**钉住相等的测试放在 PS 消费适配器旁**（那里本来就 import 两侧），随 ps-port-remainder/02 的适配器进 PS 地盘，本票不写；本票在 PC 领域测试里按字面钉六格 + 三格的原词，库上 CHECK 再镜像一份——三处相等由两条测试（PC 领域对字面、PC 真库对 CHECK）加一条将来的跨侧测试共同守。PC CONTEXT 词条写明「阶段键与意图键取 `parcel-shipment` 词条原词，本上下文只镜像不另定」；PC 不发明第七格、不拆「已制签或已装袋」。**这条镜像纪律是先例**：后续任何一个上下文要在自己的声明里引用另一个上下文拥有的封闭集，同一做法——自己的类型、原词、消费侧测试钉住。

**六、读口、写口、通道、批文。** 读口 `pcports.SourceDataAmendmentAllowanceView.LoadSourceDataAmendmentAllowance(ctx, tenant, rulePackage) (content, found, error)`，三格语义同 `IntakeQualificationView`：无父行 = 未配置、父行在而正文立不住 = error；实现落在既有 `StageContentDeclarations`（它就是这一族的家）。写口 `PublicationRegistry.SaveSourceDataAmendmentAllowance(ctx, content)`，先读回再判：同拥有版本、同 `closed`、同一份格集合是重放；`closed` 不同、格多一条少一条、同格不同值都是内容冲突，绝不覆盖也绝不并写。发布用例 `CommercialDeclarations` 多一格 `SourceDataAmendment *SourceDataAmendmentDeclaration{Closed, Rules}`——是指针不是切片，因为「封闭 + 零行」是一份合法声明，`len == 0` 表达不了在不在场；新通道 `SourceDataAmendmentChannel`（`SOURCE_DATA_AMENDMENT`），不折进既有通道——它有自己的父行与拥有对象，与 ADR-0119 有效期「没有独立拥有对象故不另开通道」正相反。受控批文 `declarations.sourceDataAmendment{closed, rules[{dataGroup, stage, intent, allowance}]}`：`closed` 必填不给默认（缺键 = 整节没声明；`closed` 是这一节的一部分，不是可省的旁注）；`allowance` 只收 `ALLOWED` / `DISALLOWED`，写 `NOT_DECLARED` 拒收；阶段、意图集外拒收；资料组引用只查非空。

**七、资料组引用是开放引用。** PC 侧 `SourceDataGroupReference`（`requiredValue` 形，非空即可），不校验存在性、不持词表——资料组的词表归 `PAR-COM-13`，PS 的 `SourceDataGroupReference` 同样是开放串，两侧只在「不得模糊指代」这一句上一致（`UC-PS-002`），而那是 PS 对请求的判断，不是 PC 对声明的校验。

**八、不做的，逐条写明。**

- 不内置任何矩阵格、不给任何资料组或阶段默认允许；`NotDeclared` 仍是 PS 的零值，本记录一字不改它。
- 不在本上下文判阶段：阶段是 PS 按事实判的（ADR-0118），`AllowanceFor` 收一个已判出的阶段；判不出阶段不猜，那是 PS 侧的话。
- 阶段六格与意图三格一格不拆、一格不加。
- 不改已施加迁移 `0013`；不改 `AT-PS-020`、`UC-PS-002` 阶段表与 PS CONTEXT 硬句；不改 ADR-0058 决定一那张表。
- 不开行级 UPDATE / DELETE：改矩阵发新规则包版本。
- PS 消费适配器与 PS 侧的封闭集比对测试不在本票（PS 地盘），本记录落地后由 ps-port-remainder/02 的认领人接。
- 不改管理台目录行与表单：分节表单多一节归 admin-write-faces/12。

## Consequences

- 票 pc-gaps/10 落地：迁移 `0027`（父子两表 + 三个封闭集 CHECK）；领域 `DeclaredAmendmentStage`、`DeclaredAmendmentIntent`、`AmendmentAllowance`、`SourceDataGroupReference`、`SourceDataAmendmentRule`、`SourceDataAmendmentAllowanceContent` 与 `NewSourceDataAmendmentAllowanceContent(owner, closed, rules)`、`AllowanceFor`；端口读口 + 具名 Save；postgres 读写；发布用例通道 `SOURCE_DATA_AMENDMENT`；受控批文 `sourceDataAmendment` 一节；真库端到端。
- PS 半边（票 ps-port-remainder/02 余下一段）据此解阻：`adapters/partycommercial/` 经 `AdoptedStageOwner.AcceptanceRulePackageFor` 回指 → `LoadSourceDataAmendmentAllowance` → `AllowanceFor(scope.DataGroup, query.Stage, query.Intent)` → 三值一对一译；声明缺席（`found=false`）→ `NotDeclared`；旁边加一条测试钉住两侧六格 + 三格原词逐字相等。
- ADR-0058 Links 加一行回指本记录；决定一那张表不动。
- CONTEXT：Language 新词条「资料修订允许声明」；Boundaries「`party-commercial` 拥有……面单服务终局规则（正文挂在接单规则包版本下）」一句里并列加上本族。Rules 各句不改。
- 管理台：票 admin-write-faces/12 分节表单多一节（`closed` 一格 + 逐格表），由那张票自己加。
- `PAR-COM-13` 在参数登记册里多一项「资料修订允许矩阵（资料组 × 阶段 × 意图 → 允许 / 不允许；是否封闭）」待提供；登记面从此在接单规则包版本的声明。登记册行由其所有者改口径，本记录不代改。

### 越权风险点（单列，供 owner 复核）

- **`closed` 是父行一格布尔而不是第三种行值。** 这是对代裁 Q3「父行带封闭标记」的字面落法。若 owner 认为「封闭」该按资料组分（某一资料组封闭、其余开放），Decision 三要改成子表加一种行或父行改成按资料组的封闭清单；今天 PS 端口问的是一格，没有按资料组的消费形状，所以我没取。
- **`closed = true` 下允许零行。** 「这一版什么都不许改」被我读成一句合法的显式话。若 owner 认为封闭也必须至少登一格（免得一个误写的 `closed: true` 静默锁死全部资料修订），改一处构造门判据与一条测试，表不动。
- **子表名 `source_data_amendment_allowance`、封闭集名 `DeclaredAmendmentStage` / `DeclaredAmendmentIntent`。** 与票面倾向（`_rule`、`*Key`）不同，理由在 Decision 二、五：贴本族既有命名。若 owner 更看重与票面 / PS 侧命名的对齐，改名不改形。
- **意图三格今天照 PS 原样镜像。** PS 头注写「来源更正 / 撤销关系随其机制实现时再加」；那天 PS 加第四格，PC 这边要同步加一格 + 一条 CHECK + 一次迁移。这是镜像纪律的已知代价，不是缺陷。

## Alternatives considered

- **挂在授权规则版本。** 否决：Decision 一——「能不能做」与「能改什么」分属两个对象；把矩阵挂到授权规则上，同一份规则包被两个授权规则采用时矩阵会分叉。
- **整份矩阵一个 JSON 列。** 否决：Decision 二，ADR-0115 Decision 二同一条。
- **子行三值（含 `NOT_DECLARED`）。** 否决：Decision 三——「没说」与「说了『我不说』」在表里分不开，且 `closed` 的读法要再和它叠一层。
- **缺格一律 `Disallowed`（不要 `closed`）。** 否决：MCP-1 代裁 Q3 已答——默认 `NotDeclared`；一律拒会让「还没登完」的租户把客户的补充资料请求全拒掉而不是转复核。
- **缺格一律 `NotDeclared`（不要 `closed`）。** 否决：同一条代裁——登记方要有办法显式说「除了这些，其余都不许」，否则必须把三维笛卡尔积逐格登满 `DISALLOWED`。
- **PC 只交行集合，PS 查格并解释 `closed`。** 否决：Decision 四。
- **PC 领域直接 import PS 的 `AmendmentStage`。** 否决：两个限界上下文的领域包互不依赖（CONTEXT-MAP 的箭头是 PS 消费 PC，不是反向）；Decision 五的镜像 + 消费侧测试是代价最小的一致性。
- **把 `closed` 做成批文可省、默认 `false`。** 否决：Decision 六——默认就是产品替登记方选了「缺格转复核」，与「不给任何默认」相悖。
- **折进 `INTAKE_QUALIFICATION` 或 `FINAL_RULE` 通道。** 否决：它有自己的父行与拥有对象，与 ADR-0119 有效期的情形相反；独立通道让报告与进程口能单独指出这一族的落点。

## Links

- 票 [pc-gaps/10](../../.scratch/party-commercial-context-gaps/issues/10-source-data-amendment-allowance-is-a-third-stage-content-family.md)：缺口事实、七问与倾向、实施与验收
- 票 [ps-port-remainder/02](../../.scratch/ps-port-remainder/issues/02-source-data-amendment-allowance-is-a-third-stage-content-declaration.md)：PS 半边与 MCP-1 代裁（Q1 归属 / Q2 六格原词 / Q3 缺格与封闭 / Q4 消费侧读口）
- [ADR-0058](./0058-stage-content-owned-by-rule-objects.md)：声明归拥有规则对象——Decision 一「第四件按同一纪律」的依据；决定一那张表不动，Links 回指本记录
- [ADR-0062](./0062-adopted-stage-owner-from-accepted-resolution.md)：回指接受时固定的规则包版本——PS 消费适配器的回指路径
- [ADR-0063](./0063-intake-qualification-proof-is-a-consumer-side-evidence-port.md)：声明怎么读归提供方、列出之后怎么取证归消费方——Decision 四的分工依据
- [ADR-0115](./0115-pre-acceptance-financial-control-policy-content-is-a-row-per-control-and-no-control-stays-with-the-contract.md)：Decision 二「子表逐行而非 JSON」——本记录 Decision 二同一判据
- [ADR-0116](./0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md)：资料修订是授权动作、委派解出决定方——「能不能做」那一半，与本族「能改什么」分立
- [ADR-0118](./0118-amendment-stage-facts-enter-parcel-shipment-through-consumer-side-read-ports.md)：阶段由 PS 判、六格取 `UC-PS-002` 原词——Decision 五镜像的权威出处
- [ADR-0119](./0119-label-validity-is-a-declaration-slot-on-the-final-rule-content.md)：同一批的前一族形状裁决；本记录 Decision 六「独立通道」与它 Decision 五「不另开通道」的分岔判据是有没有独立的拥有对象
- [party-commercial CONTEXT](../domain/party-commercial/CONTEXT.md)：新词条「资料修订允许声明」；Rules「区分接入最小身份、委托通用不变量、产品与合同资料、监管原始资料及跨字段条件」「只装配各权威上下文的规则引用」
- [parcel-shipment CONTEXT](../domain/parcel-shipment/CONTEXT.md)：词条「资料修订阶段」——六格原词的权威
- `internal/parcelshipment/domain/amendment_stage.go` / `customer_source_data.go`：`AmendmentStage` 六格、`AmendmentIntent` 三格、`SourceDataGroupReference`——镜像的对象
- `internal/parcelshipment/ports/ports.go`：`SourceDataRuleDeclaration` / `SourceDataAmendmentQuery` / `SourceDataAmendmentAllowance`——消费方的问法与三值
- `internal/partycommercial/domain/service_stage_content.go`：三族既有构造门与两值读法——本记录在其旁加第四族
- `migrations/party_commercial/0013_stage_content_declarations.sql`：两族表形先例——`0027` 形照它
- `PAR-COM-13` / `BD-PS-010`：实例半边的出处

## owner 复核记录

- owner 复核 2026-09-09 认可（用户 2026-09-09 12:3x 经 IDP 队列通道 1 授权「你自决，目标是全部解决」，通道 1 代裁，四条逐条）：1. `closed` 是父行一格布尔——PS 端口问的是一格，按资料组封闭没有消费形状；2. `closed = true` 下允许零行——「这一版什么都不许改」是一句合法的显式话，要求至少一行等于逼登记方编一行假的，违「不给默认」；`closed` 本身可缺且服务端点名（票 awf/12），误写的风险由「须在场」那道门挡；3. 命名贴本族——形不变名可改，不挡；4. 意图三格镜像 PS——镜像纪律的已知代价，PS 加第四格那天 PC 同步一格 + CHECK + 迁移。票 pc-gaps/10 的一处越权点即此。