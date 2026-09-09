# 资料修订允许矩阵是接单规则包版本下的第三族阶段内容声明——PC 今天没有这一族，PS 的 `SourceDataRuleDeclaration` 因此只有替身

Category: enhancement
Status: resolved——2026-09-07，MCP-6（task-390c4f53，分支 `mcp6-pcgaps10` 基 `9379c716`）：ADR-0120 七问一次答完（越权风险点四条单列在 ADR 里供 owner 复核）、PC CONTEXT 新词条 + Boundaries 一句、迁移 `0027`、领域 / 端口 / PG / 发布通道 `SOURCE_DATA_AMENDMENT` / 批文 `sourceDataAmendment` 全部落地，真库往返与进程口端到端 PASS；完成记录见 Comments 末条。此前 in-progress：七问由 [ADR-0120](../../../docs/adr/0120-source-data-amendment-allowance-is-a-third-stage-content-family-on-the-rule-package-version.md) 一次答完（2026-09-07，MCP-6 按 MCP-1 派单 task-390c4f53「owner 授权自决口径」裁），PC CONTEXT 新词条「资料修订允许声明」+ Boundaries 一句、ADR-0058 Links 一行已随 ADR 同笔落；实施（迁移 `0027` → 领域 → 端口 / PG → 发布用例通道 / 批文 → 真库端到端）按下面「要建什么」逐笔接。开工前置已核：PS `domain.AmendmentStage` 在 main `9379c716`（`internal/parcelshipment/domain/amendment_stage.go`），以它为准，不再看分支。此前 draft：PC 半边（第三族阶段内容声明「资料修订允许声明」表族 + 读口 + 发布通道）由 MCP-1 2026-09-07 代裁归 PC 批（原文在
[ps-port-remainder/02](../../ps-port-remainder/issues/02-source-data-amendment-allowance-is-a-third-stage-content-declaration.md) 裁决节与 Comments，取证时该目录只在分支 `mcp2-ps-ports` 上、尚未进 main；PS 不依赖 PC 的那段已在该分支落地，含 ADR-0118）；本票是那一族在 PC 侧的建模票，待 `/domain-modeling` 答完下面「要你答的问题」再转 ready-for-agent。ADR 号 **0120** 由 MCP-1 预留（task-aafca372）——ps-port-remainder/02 裁 ④ 写「归属不要 ADR，ADR-0058 决定一的表加一行即可，PC owner 落地时在 ADR-0058 Consequences 或新 ADR 记一句由其自裁」，预留号是给那个「或」用的；迁移号以开工那刻 `party_commercial` 最大序号 + 1 重取（立票时最大 `0024`）
Blocked by: 无。**但一件顺序要守**：阶段六格原词的权威是 PS CONTEXT 词条「资料修订阶段」与 `domain.AmendmentStage`，两者在 `mcp2-ps-ports` 上（`7ec02162`）、尚未进 main；本票开工前先确认它们已进 main，没进就以那条分支为准并在完工报点名

## 从哪里来

PS 的 `ports.SourceDataRuleDeclaration.DeclareSourceDataAmendment` 头注：「字段、字段组、阶段与允许动作由 `PAR-COM-13` 与真实合同、产品、
线路和关务规则登记，属实例半边；本上下文只消费登记结论，绝不自带一份矩阵。」仓内只有 `sourceDataRuleDouble`。通道 2 建模后裁：矩阵正文
归 PC，作接单规则包版本下的**第三族**阶段内容声明；「此刻在哪个阶段」的判断归 PS。MCP-1 代裁（owner 授权，2026-09-07）：**Q1** 同意归属；
**Q2** 阶段封闭集照 `UC-PS-002` 六格原词、不拆；**Q3** 缺格默认 `NotDeclared`，父行带「封闭」标记时缺格读 `Disallowed`；**Q4** CC/NO 阶段
事实进 PS 取消费侧读口（ADR-0118，PS 已落）。本票只承接 PC 那一半。

## 缺口事实（锚 main `92579b0a`；分支事实锚 `mcp2-ps-ports` `71b8817b`）

- `migrations/party_commercial/0013_stage_content_declarations.sql`：两族挂接单规则包版本（`object_kind = 4`）——`intake_qualification_*`
  （父 + 允许来源子表 + 资格引用子表）与 `final_rule_*`（父 + 按责任结果的子表）；第三族取消授权目录挂授权规则版本（`object_kind = 9`）。
  **没有任何一族按（资料组 × 阶段 × 意图）说「能不能改」。**
- `pcports` 只有 `IntakeQualificationView` / `FinalRuleContentView` 两个规则包声明读口；`application.DeclarationChannel` 封闭集里没有资料修订
  那一格；`translate.go` 的 `declarationsDocument` 亦无。
- ADR-0058 决定一的表列三件（收寄资格、终局规则 → 规则包版本；取消授权 → 授权规则版本），Consequences 写「规则包正文（B7）仍是另一族，
  本记录不把它预支进本表」——第四件按同一条纪律归规则包版本，是加一行不是改纪律。
- PC CONTEXT Rules：「接单规则包必须……区分接入最小身份、委托通用不变量、产品与合同资料、监管原始资料及跨字段条件」——规则包已经按
  **资料组**说话；「规则包只装配各权威上下文的规则引用」。`PAR-COM-13` 登记责任列「客户接入、托运业务、商业、关务、节点运营、结算」，
  正是规则包「装配各权威上下文的规则引用」那个对象。
- PS 侧（分支 `mcp2-ps-ports`）：`SourceDataAmendmentQuery{Identity, Scope, Intent, Reason, Stage}`；`domain.AmendmentStage` 六格
  `ACCEPTED_NOT_YET_RECEIVED` / `RECEIVED_OR_MEASURED` / `LABELLED_OR_BAGGED` / `CUSTOMS_DATA_FORMING_NOT_SUBMITTED` / `CUSTOMS_SUBMITTED` /
  `CASE_CLOSED_OR_SERVICE_COMPLETED`，零值`判不出`；main 上已有 `SourceDataScope`（委托 + 可空包裹 + `SourceDataGroupReference` 开放引用）、
  `AmendmentIntent` 三格 `SUPPLEMENT` / `CORRECTION` / `EXPLICIT_CLEAR`、`SourceDataAmendmentAllowance` 三值（`NotDeclared` 零值）。消费适配器
  先例 `adapters/partycommercial/stage_content_declarations.go` 的 `DeclaredStageContent` + `AdoptedStageOwner`。

## 为什么是机制半边

矩阵的**三维**（资料组引用 × 修订阶段 × 修订意图）与**两值**（允许 / 不允许）由 `UC-PS-002`「资料范围与阶段边界」、`AT-PS-020`（意图进矩阵）、
PS CONTEXT「显式清空必须由版本化字段和阶段规则允许」三处硬句推得出，一格不依赖租户取值；**每一格填什么、资料组怎么划、哪些阶段对哪些
资料组开放**是 `PAR-COM-13` / `BD-PS-010` 的实例半边，待提供。今天没有这一族，租户即便有了规则也无处登；PS 端口头注禁它自带矩阵——缺口
只能在 PC 补。拒默认落在 PS 既有的 `NotDeclared` 零值与「判不出阶段则未决」，本票一字不改它们。

## 要你答的问题（`/domain-modeling` 一次答完；答案落 PC CONTEXT 新词条「资料修订允许声明」+ ADR-0058 加一行或 ADR-0120）

1. **表族形状。** 倾向形照 `0013` 两族：父行 `source_data_amendment_content`（四元组 + `object_kind = 4` + **`closed` 布尔** + `declared_at`）
   + 子行 `source_data_amendment_rule`（四元组 × `data_group_ref` × `stage` × `intent` → `allowance`），主键含三维，`stage` / `intent` /
   `allowance` 三个封闭集进 CHECK。替代是整份矩阵一个 JSON 列——否决理由与 ADR-0115 Decision 二同一条：逐格可查、逐格可约束。
2. **阶段与意图两个封闭集的词，单一权威放哪？** 阶段六格原词归 PS（CONTEXT 词条 + `domain.AmendmentStage`），意图三格归 PS
   （`domain.AmendmentIntent`）；PC 的领域包不能 import PS 的领域包（两个限界上下文），所以 PC 要有自己的封闭集镜像。倾向：PC CONTEXT
   词条写明「阶段键与意图键取 PS 词条原词，本上下文只镜像不另定」；PC 领域 `AmendmentStageKey` / `AmendmentIntentKey` 的 `String()` 与
   PS 逐字相等，**由 PS 消费适配器旁的一条测试钉住**（那里本来就 import 两侧）。两侧各改一次是代价，换来的是两个 CONTEXT 都不必引对方
   的代码。
3. **缺格 / 零行 / 封闭的语义。** 按代裁 Q3：缺格 + `closed=false` → 消费方译 `NotDeclared`；缺格 + `closed=true` → `Disallowed`；
   子行只存 `ALLOWED` / `DISALLOWED`，永不存「未声明」。**零行父行**：`closed=true` 零行 = 「全部不允许」是一句显式的话，允许登记；
   `closed=false` 零行 = 什么都没说，构造门拒（与 `NewFinalRuleContent` 拒零行同判据）。同意吗？
4. **三值在哪一侧算出？** 倾向 PC 领域给 `AllowanceFor(group, stage, intent)` 三值（允许 / 不允许 / 未声明），封闭标记的读法留在声明的
   拥有者这边；PS 适配器只做 PC 三值 → PS `SourceDataAmendmentAllowance` 三值的一对一翻译。替代是 PC 只交回行集合、PS 自己查格并
   解释 `closed`——那会把「封闭意味着什么」这条 PC 的规则搬到消费方。
5. **读口。** `pcports.SourceDataAmendmentAllowanceView.LoadSourceDataAmendmentAllowance(ctx, tenant, rulePackage) (content, found, error)`，
   三格语义同 `IntakeQualificationView`：无父行 = 未配置、父行在而子行坏 = error。写口 `PublicationRegistry.SaveSourceDataAmendmentAllowance`，
   发布通道 `SourceDataAmendmentChannel`（`SOURCE_DATA_AMENDMENT`），批文 `declarations.sourceDataAmendment{closed, rules[{dataGroup,
   stage, intent, allowance}]}`，集外拒收（含把 `NOT_DECLARED` 当 allowance 写进来）。
6. **资料组引用是开放引用**（PS 的 `SourceDataGroupReference` 是开放串，`UC-PS-002` 只禁模糊指代）——PC 侧同样收串不校验存在性，
   资料组的词表归 `PAR-COM-13`。同意吗？
7. **要不要 ADR-0120。** 归属按代裁不要（ADR-0058 决定一加一行）；问题 2 那条「封闭集跨上下文镜像、由消费侧测试钉住」是不是一条
   值得记的取舍？倾向：若只加一行就写进 ADR-0058 Consequences；若问题 2 的镜像纪律要成为其它跨上下文封闭集的先例，就用 0120。

## 要建什么（裁完之后，本票不实施——「只立票」）

PC CONTEXT 新词条 → ADR-0058 加一行或 ADR-0120 → 迁移（父子两表）→ 领域（`SourceDataAmendmentAllowanceContent`、两个封闭集镜像、
`AllowanceFor`）→ 端口（读口 + 具名 Save）→ postgres → 发布用例通道 → `cmd/parcel-commercial` 批文一节 → 真库端到端读回（形照
pc-gaps/07 四笔 `6dc3db12..265da8d8`）。PS 消费适配器与 PS 侧的封闭集比对测试不在本票（PS 地盘），本票落地后由 ps-port-remainder/02
的认领人接。

## 在等本票的

- `ps-port-remainder/02` 余下两段之一：消费适配器（`AdoptedStageOwner.AcceptanceRulePackageFor` 回指 → 读本票声明 → 查格 → 译三值）。
  另一段（CC/NO 读面接线）等 `ps-port-remainder/05`，与本票无关。
- 连带（不是等）：`admin-write-faces/12` 分节表单多一节；`PAR-COM-13` 那一行的实例登记有了落点。

## 红线

- 不内置任何矩阵格、不给任何资料组或阶段默认允许；`NotDeclared` 仍是 PS 的零值；判不出阶段不猜（PS 侧，本票不碰）。
- 阶段六格与意图三格**一格不拆、一格不加**，词取 PS 原词；PC 不发明第七格。
- 不改已施加迁移 `0013`；不改 `AT-PS-020`、`UC-PS-002` 阶段表与 PS CONTEXT 硬句。
- 领域包不依赖 HTTP / pgx，也不 import PS 领域包；集外取值报错不吸收；不开行级 UPDATE / DELETE，改矩阵走新规则包版本。
- 撞上要改 ADR-0058 决定一的纪律本身、或阶段词表要由 PC 拥有 → 停下报 MCP-1（代裁 Q1 / Q2 已答，改口要回到 owner）。

## 参照

`migrations/party_commercial/0013_stage_content_declarations.sql`；`pcports.IntakeQualificationView` / `FinalRuleContentView` 与
`PublicationRegistry` 具名 Save 一族；`application.DeclarationChannel`；`cmd/parcel-commercial/translate.go` 的 `declarationsDocument`；
ADR-0058（归属与三格语义）、ADR-0042、ADR-0063（声明与取证分工）、ADR-0115 Decision 二（子表逐行而非 JSON）、ADR-0118（分支
`mcp2-ps-ports`）；PC CONTEXT Rules 上引两句；PS `ports.SourceDataRuleDeclaration` / `SourceDataAmendmentQuery` / `SourceDataAmendmentAllowance`、
`domain.AmendmentStage`（分支）、`domain.SourceDataScope` / `AmendmentIntent`、`adapters/partycommercial/stage_content_declarations.go`；
`UC-PS-002`「资料范围与阶段边界」与 `BD-PS-010`；`AT-PS-020`；`PAR-COM-13`。

## Comments

- 2026-09-07 · MCP-6（task-aafca372，分支 `mcp6-awf07`）：立票（draft）。**只写票面，未动代码、未改 CONTEXT。** 素材是
  `ps-port-remainder/02` 的裁决节、MCP-1 代裁 Comment 与通道 2 的 PS 半边完成记录（`git show mcp2-ps-ports:…`）；PC 侧事实在
  `92579b0a` 上逐条重取（`0013` 全文、`ports.go` 两个声明读口、`DeclarationChannel` 封闭集、ADR-0058 全文、CONTEXT 两句），PS 侧
  `AmendmentStage` 全文取自分支 `71b8817b`。能力边界：**没读** `intake_qualification_*` 的 postgres 读写实现与
  `stage_content_declarations.go` 全文——问题 4「三值在哪一侧算出」的倾向是按 ADR-0063 分工推的，建模时以两侧代码为准。
- 2026-09-07 18:09 · MCP-6（17:4x 新绑会话，按 MCP-1 18:05 广播补记）：**封存出处。** 立票笔 `8c1e6ed5`（分支 `mcp6-awf07`）
  → main `52001f02`（远端 main = `e374b1ea`）。对照全表在 `admin-write-faces/07` 的同时刻 Comment。
- 2026-09-07 22:3x · MCP-6（task-390c4f53，21:2x 新绑会话；分支 `mcp6-pcgaps10` 基 `9379c716`）：**裁决落 ADR-0120**，七问按「要你答的问题」
  逐条答：①父行 + 逐格子行（`source_data_amendment_content` / `source_data_amendment_allowance`，子表名与倾向 `_rule` 不同——这一族没有
  规则引用列）；②两个封闭集在 PC 立 `DeclaredAmendmentStage` / `DeclaredAmendmentIntent` 镜像（与倾向 `*Key` 不同——贴本族既有
  `Declared*` 三个镜像），原词由 PS 消费侧测试钉住、PC 领域测试按字面钉；③同意：缺格 + 未封闭 = 未声明、缺格 + 封闭 = 不允许、子行永不登
  「未声明」、`closed=true` 零行合法、`closed=false` 零行拒；④三值在 PC `AllowanceFor` 算出，PS 一对一译；⑤读口 / 写口 / 通道
  `SOURCE_DATA_AMENDMENT` / 批文 `sourceDataAmendment{closed, rules[]}`，`closed` 必填；⑥同意开放引用；⑦立 ADR-0120（形状 + 缺格语义 +
  镜像纪律三条取舍够格），ADR-0058 决定一那张表不动、只在 Links 回指。PC CONTEXT 新词条 + Boundaries 一句、README 一行。同笔只有文档；
  代码从下一笔起。能力边界写在 ADR 头部（未读 PS 消费适配器全文与 intake 的 PG 读写实现）。
- 2026-09-07 23:0x · MCP-6（task-390c4f53，分支 `mcp6-pcgaps10` 基 `9379c716`）：**完成记录，转 resolved。** 逐笔：`76fb081a` docs
  （ADR-0120 + README 一行 + CONTEXT 词条与 Boundaries 一句 + ADR-0058 Links 一行 + 本票 in-progress）；`91aa3c85` feat（迁移
  `0027_source_data_amendment_allowance.sql`——父表 `source_data_amendment_content`（四元组 + `object_kind = 4` + `closed`）与子表
  `source_data_amendment_allowance`（四元组 × `data_group_ref` × `stage` × `intent` → `allowance`，主键含三维，阶段六格 / 意图三格 /
  允许性两格三个封闭集进 CHECK、资料组非空、外键级联）；领域 `SourceDataGroupReference`、`DeclaredAmendmentStage` 六格、
  `DeclaredAmendmentIntent` 三格、`AmendmentAllowance` 三值（零值「未声明」只两值可登）、`SourceDataAmendmentRule`、
  `SourceDataAmendmentAllowanceContent` + `NewSourceDataAmendmentAllowanceContent(owner, closed, rules)` + `AllowanceFor` + `Rules`）；
  `709a87c1` feat（`ports.SourceDataAmendmentAllowanceView` + `PublicationRegistry.SaveSourceDataAmendmentAllowance`；postgres 两半单独成
  文件 `adapters/postgres/source_data_amendment.go`：读口挂 StageContentDeclarations、写口挂 CommercialPublications；两处替身跟随、
  事务守卫加一行）；`44e80389` feat（发布用例 `CommercialDeclarations.SourceDataAmendment` 指针 + 新通道 `SourceDataAmendmentChannel`
  `SOURCE_DATA_AMENDMENT`；批文 `declarations.sourceDataAmendment{closed, rules[]}`，`closed` 是 `*bool` 必填；admin-write-faces/12 一句改
  「已落地」）；`ac6b5241` chore（机制清点重生成：partycommercial 生产 90→92 / 测试 92→94 / PG 适配器 30→31；迁移 151→152；端口声明
  361→362）。**七问对号**：①父子两表（子表名 `_allowance`）②`Declared*` 镜像 + PC 领域测试按字面钉原词 ③缺格 + 未封闭 = 未声明、缺格 +
  封闭 = 不允许、子行永不登「未声明」、封闭零格合法、未封闭零格拒 ④`AllowanceFor` 在 PC 算三值 ⑤读口 / 具名 Save / 独立通道 / 批文一节
  ⑥资料组开放引用只查非空 ⑦ADR-0120，ADR-0058 只加 Links。**验收**：干净检出 `ac6b5241`（隔离树 `idp-parcel-mcp6-pcgaps10`）——`gofmt -l`
  空；`go build ./...` / `go vet ./...` 退 0；清点工具 vet + test ok、清点门零差；`scripts/ci/test-shards.sh check` 116 包无重叠无遗漏；
  含 DSN（门禁容器 55432）`go test -p 1 -count=1 ./...` 100 ok / 0 FAIL / 16 无用例 / 0 cached，9m33s；探针 `-run
  'SourceDataAmendment|OpenParentRow'` 两包无 DSN SKIP 6 + PASS 1（翻译层）↔ 含 DSN PASS 7。真库用例证：未封闭逐格往返且缺格未声明、
  封闭零格读回缺格一律不允许、封闭带格按格答、未登记 found=false、别租户拒；重放 / 翻封闭 / 多一格 / 少一格 / 同格换值五种冲突且原正文
  不动；库上 CHECK 镜像七条（父行挂授权规则 / 阶段集外 / 意图集外 / NOT_DECLARED / 资料组空 / 同格两行 / 无父行）；未封闭零格父行读口
  报坏声明不折成未配置；进程口两份规则包（未封闭两格 / 封闭零格）经批文发布、经 `LoadSourceDataAmendmentAllowance` 读回逐格与缺格读法。
  应用层：三格正向（未封闭带格 / 封闭零格 / 缺键）+ 五格门（未封闭零格 / NOT_DECLARED / 同格两行 / 挂合同 / 挂授权规则）+ 冲突进报告；
  翻译层一正（两格 + 封闭零格 + 缺键）七反（closed 缺席 / NOT_DECLARED / 集外允许性 / 集外阶段 / 集外意图 / 资料组空 / 格里未知键）。
  **不做的**：PS 消费适配器与跨侧封闭集比对测试（ps-port-remainder/02 余下一段，据此解阻）；`AcceptanceRulePackageRow` 目录不加格；
  管理台分节表单那一节（admin-write-faces/12 自己加）；`PAR-COM-13` 登记册行不代改；tasks.md 不动。**共享接线文件各加了哪一段**：
  `ports.go`（`SourceDataAmendmentAllowanceView` 一接口 + `PublicationRegistry` 末尾一方法）、`publish_commercial_authority.go`
  （`CommercialDeclarations` 末尾一格 + `SourceDataAmendmentDeclaration` 一类型 + `DeclarationChannel` 末尾一值 + `declarationWrites`
  末尾一段）、`translate.go`（`declarationsDocument` 末尾一键 + 两个文档类型 + `declarationsFrom` 末尾一段 + 四个函数）、
  `publish_commercial_authority_test.go` / `register_commercial_test.go`（替身各一方法）、`transaction_guard_test.go`（一行）、
  `docs/adr/README.md`（0119 之后一行）、ADR-0058（Links 一行）、PC CONTEXT（一个词条 + Boundaries 一句里并列加本族）。SHA 只作此刻
  取证，MCP-1 重放后以 main 上的为准；分支清点笔不重放、推送方在 tip 上重生成兑底。
- 2026-09-08 00:2x · MCP-6（task-390c4f53）：**进 main 记录。** MCP-1 00:1x 重放到 `5fd4d942` 之上（cherry-pick 零冲突，main 自本支基线
  `9379c716` 起只多 .md）；分支→main：`76fb081a→3ce4615f`、`91aa3c85→0932d6c3`、`709a87c1→053da518`、`44e80389→2b4ae6aa`、
  `c6cefab9→7a1ce11f`；分支清点笔 `ac6b5241` 不重放，清点在 `7a1ce11f` 干净检出上重生成为 `534493d4`（与 `ac6b5241` 逐字节同）。
  推送方验 `534493d4`：重放 tip 的非 .md 树与本支已含 DSN 全仓验过的 `ac6b5241` 零差，按 parallel-sessions「代码同一全仓验仍有效」
  不重跑全仓；本侧钩 gofmt 空、build / vet 0、清点门零差、探针 PC postgres `-run 'SourceDataAmendment|OpenParentRow'` 无 DSN SKIP 5 /
  含 DSN PASS 5、含 DSN `-p 1` 跑 partycommercial/** + cmd/parcel-commercial + cmd/parcel-api + architecture + migrations 全 ok（83s）。
  **远端 `main = 534493d4`**（推前 ls-remote = `5fd4d942`）。本侧核对：五对 SHA 与清点笔均为 `origin/main` 祖先；本支触及的 23 件
  对 `origin/main` 零差。树 `idp-parcel-mcp6-pcgaps10` 已拆（status 零行），指针改名 `merged/mcp6-pcgaps10@c6cefab9`，远端同名分支
  已删。本条记录走分支 `mcp6-pcgaps10-record`（基 `534493d4`）交推送方重放。解阻两件已由 MCP-1 记入台账：ps-port-remainder/02 余段、
  admin-write-faces/12 一节。PC 迁移下一号 `0028`。
- **owner 复核 2026-09-09 认可**（用户经 IDP 队列通道 1 授权代裁）：越权点即 ADR-0120 四条，逐条认可，理由在 ADR-0120「owner 复核记录」。
