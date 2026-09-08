# `SourceDataRuleDeclaration`：允许矩阵是接单规则包版本下的第三族阶段内容声明；PS 要先长出「资料修订阶段」

Category: enhancement
Status: resolved——四段全落：**PC 半边**随 pc-gaps/10 进 main（ADR-0120，`9379c716`）；**PS 不依赖 PC 的那段**（分支 `mcp2-ps-ports` → main `d4bc4785` / `45ed2d04` / `9091f539`，ADR-0118）；**CC/NO 读面接线**随 [05](05-customs-and-node-operations-need-parcel-keyed-stage-fact-read-faces.md)（分支 `mcp2-psr05`）；**余段「消费适配器读 PC 声明 + 跨侧词比对 + 接真装配」2026-09-08 完工**（通道 2，分支 `mcp2-psr02-tail`：`2e085cad` / `5e5e2c05` / 清点 `e2a51ba8`，基 main `d1e6c094`；含 DSN 全仓 100 ok / 0 FAIL；**待合入前独立评审与推送方重放，main 上的 SHA 由进 main 记录补**）。四问裁决与各段完成记录见 Comments。此前 in-progress，余段曾 Blocked by PC 半边
Blocked by: —

## 端口今天说什么

`ports.SourceDataRuleDeclaration.DeclareSourceDataAmendment(ctx, SourceDataAmendmentQuery{Identity, Scope, Intent, Reason}) (SourceDataAmendmentAllowance, error)`，三值 `NotDeclared`（零值，最保守格）/ `Allowed` / `Disallowed`。头注：「字段、字段组、阶段与允许动作由 `PAR-COM-13` 与真实合同、产品、线路和关务规则登记，属实例半边；本上下文只消费登记结论，绝不自带一份矩阵。」消费方 `AmendCustomerSourceDataHandler`：只有 `Allowed` 才形成版本；`Disallowed` → 业务拒绝；`NotDeclared` → `AmendmentAwaitingReview`（「还没人说这能不能改」）。仓内无生产实现，只有 `sourceDataRuleDouble`；编排本身在 `cmd/` 无调用方。

**查询里没有「阶段」。** 而 UC-PS-002「资料范围与阶段边界」那张表按业务阶段分六行，`BD-PS-010` 的建议是「版本化阶段矩阵；未登记的字段和阶段不默认允许」，PS CONTEXT 硬句是「显式清空必须由版本化**字段和阶段**规则允许」。矩阵没有阶段这一维就登记不了 UC 写的任何一行——这是端口形状上的缺口，不只是实现缺席。

## 语言从哪里来

- UC-PS-002 六个阶段（原词）：已接受、尚未收寄 / 已收寄或已测量 / 已制签或已装袋 / 关务资料形成中、尚未提交 / 已提交关务 / 案件已关闭或服务已完成。它们跨三个上下文的事实：接受、收寄采用、面单交易、包裹终局归 PS；装袋归 NO；关务三格归 CC。
- 矩阵的另两维在 PS 领域已有：`SourceDataScope`（委托 + 可空包裹 + `SourceDataGroupReference` 资料组）与 `AmendmentIntent`（补充 / 更正 / 显式清空，`AT-PS-020` 要求意图进矩阵）。
- PC CONTEXT Rules：「接单规则包必须……区分接入最小身份、委托通用不变量、产品与合同资料、监管原始资料及跨字段条件」——规则包已经在按**资料组**说话；「规则包只装配各权威上下文的规则引用」——线路（NR）、关务（CC）那部分规则由规则包引用，不由规则包拥有正文，这与 `PAR-COM-13` 说的「与真实合同、产品、线路和关务规则登记」对得上。
- ADR-0058：阶段内容声明（收寄资格、终局规则、取消授权）按拥有规则对象归属，产品与合同只采用；无父行 = 未配置；消费侧不得从身份发明采用版本。0013 已有两族挂在接单规则包版本下，PS 消费侧 `DeclaredStageContent` 已把它们接进翻译适配器。
- UC-PS-002 元数据：「业务决定所有者：`parcel-shipment`；字段与阶段适用性依据来自已登记规则」——决定归 PS，规则归登记方。

## 裁决（通道 2，2026-09-07；owner 授权自决口径；取证锚远端 main `ffa6bd0e`）

**先答归属：矩阵正文归 party-commercial，作接单规则包版本下的第三族阶段内容声明；「当前处于哪个阶段」这一判断归 parcel-shipment。** 理由三条：`PAR-COM-13` 本就是商业参数行，登记责任写的是「客户接入、托运业务、商业、关务、节点运营、结算」共同出证，规则包正是「装配各权威上下文的规则引用」的那个对象；ADR-0058 已经把同类的「某阶段允许什么」（收寄资格、终局规则）判给规则包版本，第三族不另起归属；PS 若自建矩阵，就与端口头注「绝不自带一份矩阵」、与 UC「依据来自已登记规则」两处相违。反过来，**阶段是 PS 对自己对象生命周期位置的判断**，规则包只声明「在某阶段允许什么」，不判断「此刻在哪个阶段」——判断权在消费方，与收寄资格「PC 声明硬资格、PS 取证判断」的分工同形（ADR-0063）。

**① 机制半边现在能立什么。** 两半，先后有序：

- **PC 半边（先）**：第三族阶段内容声明「**资料修订允许声明**」，拥有对象 = 接单规则包版本（`ACCEPTANCE_RULE_PACKAGE`，形照 0013 的 `intake_qualification_*` / `final_rule_*`：父行一条 + 子行若干，主键 = 租户 + 拥有版本四元组 + 声明格）。子行的格：（资料组引用 × 修订阶段 × 修订意图）→ 允许 / 不允许。无父行 = 未配置（消费方译 `NotDeclared`）；父行在而子行坏 = error。读口 `pcports.SourceDataAmendmentAllowanceView`（名字随 PC 惯例定），发布随 `PublishCommercialAuthorityHandler` 加一条通道。**缺格语义**见问题 3。
- **PS 半边（后，三件）**：(a) CONTEXT 新词条「**资料修订阶段**」（提议，见下）+ 领域封闭集 `AmendmentStage`，取 UC-PS-002 六格原词；(b) `SourceDataAmendmentQuery` 加 `Stage`，由编排在问矩阵前判出：PS 自有事实（接受基线在、收寄采用有无、面单交易结果有无、包裹终局有无）本上下文直接读，装袋（NO）与关务三格（CC）是跨上下文输入——**输入缝的形状本票不裁**（随信封携带照 ADR-0075，还是消费侧读口照 ADR-0025，见问题 4）；判不出阶段时编排停未决，不猜一个阶段去问矩阵；(c) 消费适配器照 `DeclaredStageContent`：`AdoptedStageOwner.AcceptanceRulePackageFor` 回指接受时固定的规则包版本 → 读声明 → 查（资料组, 阶段, 意图）那一格 → 译三值。

**② 实例半边留什么、在哪一格拒默认。** 矩阵里每一格的取值、资料组的划分、哪些阶段对哪些资料组开放，全是 `PAR-COM-13` / `BD-PS-010` 待提供；拒默认落在 `NotDeclared` 零值（已有）与「判不出阶段则未决」（新增，不得默认成「已接受、尚未收寄」）。

**③ 形状照哪个先例。** ADR-0058（归属、分表分读、无父行=未配置）、0013 两族表、`DeclaredStageContent` + `AdoptedStageOwner`（消费侧）、ADR-0063（声明与取证分工）。

**④ 要不要 ADR。** 归属不要——ADR-0058 决定一的表加一行即可（它自己写「三件」，第四件按同一条纪律归规则包版本，PC owner 落地时在 ADR-0058 Consequences 或新 ADR 记一句由其自裁）。**阶段判断的跨上下文输入缝可能要一篇**：CC 申报阶段进 PS 是新的一条 CC → PS 缝，CONTEXT-MAP 今天没有这条箭头（有 PS → CC）；建模到那一步再判。

### 提议的 PS CONTEXT 词条（未写入，等你点头）

> **资料修订阶段** 一份已接受委托或其包裹在资料补充/更正请求到达时所处的生命周期位置，是判断「此刻允许改什么」的维度之一。封闭六格：已接受尚未收寄、已收寄或已测量、已制签或已装袋、关务资料形成中尚未提交、已提交关务、案件已关闭或服务已完成。阶段由 `parcel-shipment` 按自有事实与邻接上下文交出的事实判断，判断不成时修订请求停在待复核，不得默认为最早阶段；允许什么由所采用接单规则包版本的资料修订允许声明说。

## 要你答的问题

1. **归属确认**：矩阵正文归 PC（接单规则包版本下第三族声明）、阶段判断归 PS——同意吗？替代是 PS 自建矩阵登记册，代价是与端口头注、UC「依据来自已登记规则」和 ADR-0058 三处相违，且关务/线路那部分规则要由 PS 替它们登。
2. **阶段封闭集**就取 UC-PS-002 的六格原词？「已制签或已装袋」把 PS 事实（面单结果）与 NO 事实（装袋）合在一格，是照 UC 保留还是拆开？我的倾向：照 UC 保留，拆是规则出现后的事。
3. **缺格语义**：声明父行在、但（资料组, 阶段, 意图）那一格没有子行——是 `NotDeclared`（待复核）还是 `Disallowed`（拒绝）？我的倾向：**默认 `NotDeclared`**，另给父行一个「封闭」标记，标了封闭的声明缺格才读作 `Disallowed`——「未登记的字段和阶段不默认允许」两种读法都满足，差别在客户看到的是「等复核」还是「被拒」，那是登记方该显式说的。
4. **关务与装袋两处阶段事实怎么进 PS**：随 CC/NO 的信封携带过界（ADR-0075 对 NR 的裁法），还是 PS 建消费侧读口（ADR-0025）？这决定阶段判断能不能同步完成；答完再定要不要 ADR。

## 红线

- 不内置任何矩阵格；`NotDeclared` 仍是零值；判不出阶段不猜。
- 不改 `AT-PS-020`、UC-PS-002 阶段表与 PS CONTEXT 硬句；词条只作提议。
- PC 半边不在 PS 地盘动手。

## 参照

`ports.SourceDataRuleDeclaration` / `SourceDataAmendmentQuery` / `SourceDataAmendmentAllowance` 头注；`application/amend_customer_source_data.go` 矩阵那一格；`domain/customer_source_data.go` 的 `SourceDataScope` 与 `AmendmentIntent`；`adapters/partycommercial/stage_content_declarations.go`；`migrations/party_commercial/0013_stage_content_declarations.sql`；ADR-0058、ADR-0063、ADR-0062、ADR-0025、ADR-0075；UC-PS-002「资料范围与阶段边界」「待确认业务决策」（`BD-PS-010`）；`PAR-COM-13`。

## Comments

- 2026-09-07 · 通道 2：立票（draft），一次 `/domain-modeling` 的产物。**只写票面，未动代码，未改 CONTEXT。** 能力边界：读过端口与编排、PS 领域两型、PC 0013 表名与 `DeclaredStageContent`、ADR-0058 全文、UC-PS-002 全文、PC CONTEXT 相关句；**没读** CC/NO 侧今天有哪些能答「阶段」的读口——问题 4 因此留给建模那一步。
- 2026-09-07 · MCP-1 代裁，owner 授权（task-b77525c9，由通道 2 落票面）：**Q1** 同意——矩阵正文归 PC 作第三族阶段内容声明，阶段判断归 PS；**Q2** 阶段封闭集照 UC-PS-002 六格原词，不拆；**Q3** 缺格默认 `NotDeclared`，父行带「封闭」标记时缺格读 `Disallowed`；**Q4** CC/NO 阶段事实进 PS **取消费侧读口（ADR-0025 形）**——阶段是修订请求到达那一刻同步要问的，不是事件便车能保证齐全的；PS 立 `ports.*StageView` 一类读口，适配器读 CC/NO 已有读面，读面不存在或未接就是「判不出阶段 → 未决」；它给 CONTEXT-MAP 加 CC→PS、NO→PS 两条消费箭头，**要 ADR，取预留号 0118**；建模时以 CC/NO 代码为准，若发现既有信封已携带阶段事实且同步性不成问题，可在 ADR 里改选并写理由。上面「提议的 PS CONTEXT 词条」按 Q2 进 CONTEXT。Status 由 draft 改 in-progress（PS 不依赖 PC 的那段开工）。
- 2026-09-07 · 通道 2（task-d5558bc6）：**PS 半边不依赖 PC 的那段完成记录**（分支 `mcp2-ps-ports`，基线 `08f54867`）。
  - `7ec02162` 领域：`AmendmentStage` 六格原词（零值判不出，进 enum 门禁）、`StageFact` 三态、`EitherStageFact` 并格、`JudgeAmendmentStage`（靠后压过靠前，途中`不知道`即判不出）、`FurthestAmendmentStage`（委托级取成员最远）。
  - `3da37e07` 端口 / 编排 / 适配器 / 装配：`SourceDataAmendmentQuery.Stage`；`ResponsibilityStartView` / `CurrentFinalView` 窄读口 + `CustomsStageView` / `ConsolidationStageView` 两个消费侧读口；编排在基线核验之后、问矩阵之前 `judgeStage`，两格新未决原因 `SourceDataAmendmentStageUndetermined` / `SourceDataAmendmentStageFactUnavailable`；`UnconnectedCustomsStageView`（新目录 `adapters/customscompliance`）与 `UnconnectedConsolidationStageView` 一律答`不知道`；`cmd/parcel-api` 装配接三本真登记册 + 两只未接适配器。`sourceDataRuleDouble` 收到零值阶段即报错（跟查询形状）。
  - 文档：PS CONTEXT 词条「资料修订阶段」；ADR-0118 + README 一行；CONTEXT-MAP 两条关系约束 + 两条既有边标签各补一句；新立 05（CC/NO 按包裹键读面缺口，draft）。
  - **建模结论落到 ADR-0118 而不是改选事件便车**：取证 CC/NO 今天既没有按包裹键的读面、也没有同步携带阶段事实的信封可用；02-Q4 留的「若既有信封已携带且同步性不成问题可改选」那一格没有成立的证据。
  - **今天生产的停点**：授权过了之后停在`判不出阶段`（两只未接适配器），不默认最早阶段；阶段已知时才停在矩阵未登记（装配用例以已知事实替身证之）。
  - 验证见完工报（含 DSN 全仓 `-p 1 -count=1`、清点重生成单独成笔）。
- 2026-09-07 16:1x · 通道 1（推送方）**进 main 记录**：PS 半边三笔分支→main 对照 `7ec02162`→`d4bc4785`、`3da37e07`→`45ed2d04`、`8430ae09`→`9091f539`（ADR-0118 + README 行自动合并）；分支清点笔 `71b8817b` 未重放，清点在 tip 重生成 `61344989`（parcelshipment 生产 133→139、消费缝新增 PS→CC 1 文件、PS→NO 3→4、端口声明 348→352）。隔离树钉 `61344989` 含 DSN 全仓 100 ok / 0 FAIL。远端 main = `61344989`。依赖 PC 的那段仍 Blocked by pc-gaps 批（MCP-6 `aafca372` 在立 08–10）。
- 2026-09-07 · 通道 2（task-2f035050）：**读面接线段完成**——随 05 落地（分支 `mcp2-psr05`，`f91100c2`）：CC / NO 各立按包裹键的读面，PS 两只未接适配器换成真读法，装配点接真；生产停点从「授权过了停在判不出阶段」后移为「授权过了阶段按事实判出、停在矩阵未登记」，装配用例②照此改写（阶段由记录壳从矩阵查询取出证三步）。裁决全文在 05 Comments。本票余下只剩消费适配器读 PC 声明那段，仍等 PC 半边。
- 2026-09-08 · 通道 2（task-5307fbdc；新会话接续、无在途记忆，以下全按 git 与日志重取）：**余段完成记录**——
  消费适配器读 PC 声明 + 跨侧词比对测试 + 接真装配（分支 `mcp2-psr02-tail`，merge-base main `d1e6c094`，已推 origin）。
  - `2e085cad` `adapters/partycommercial/source_data_amendment_allowance.go`：`DeclaredSourceDataAmendmentAllowance`
    实现 `ports.SourceDataRuleDeclaration`——`AdoptedStageOwner.AcceptanceRulePackageFor` 回指接受时固定的接单规则包
    版本 → `pcports.SourceDataAmendmentAllowanceView.LoadSourceDataAmendmentAllowance` → `content.AllowanceFor(资料组,
    阶段, 意图)` → PC 三值一对一译 PS 三值（ADR-0120 决定四、五）。闭包不在与无父行都答 `NotDeclared`；读不回与声明
    立不住原样上抛；零值阶段 / 未声明意图在读任何东西之前拒答；阶段六格、意图三格逐格显式 switch，不按 `String()`
    对字译。测试七条，其中 `TestTheAdapterMirrorsParcelShipmentStageAndIntentWordsCellByCell` 钉两侧 `String()` 逐字
    相等、无第七格 / 第四格，并以「只登一格」对全部 6×3 查询证译到的正是那一格。
  - `5e5e2c05` `cmd/parcel-api/assemble_customer_amendment.go`：`buildSourceDataAmendmentAllowance` 装委托仓储 + PC
    解析库（`ResolvedAdoptedStageOwner`，与 parcel-dispatch 装收寄资格 / 终局规则同一条回指路径）+ `StageContentDeclarations`
    读口；生产装配换下 `UnconfiguredSourceDataRuleDeclaration`，该类型与其用例退场（授权那只未配置答复不动，归票 03）。
    装配用例：既有用例②改套生产读法；新增真库 `TestTheAmendmentAssemblyAnswersFromTheRegisteredAllowanceDeclaration`
    （未登记 → 待复核；登允许 → RECORDED 且形成版本、入队恰一封；登不允许 → DISALLOWED；未封闭声明缺格 → 待复核）与
    `TestTheAmendmentAssemblyReadsAClosedDeclarationAsDisallowingEveryUndeclaredCell`（封闭零格声明问任何一格都 DISALLOWED）。
  - `e2a51ba8` 机制清点在 `5e5e2c05` 干净检出上重生成（parcelshipment 生产 140→141 / 测试 139→140，退一进一；
    PS→PC 消费缝 12→13 文件；端口声明与端点数不变）。
  - **验证**：隔离树 `idp-verify-psr02` 钉 `e2a51ba8`，含 DSN 全仓 `go test ./...`，按日志 `^ok ` / `^FAIL` 计
    100 ok / 0 FAIL / 16 无测试（`%TEMP%\psr02-fulltest.log`，2026-09-08 00:17；真库包实跑——`visibilityexception/adapters/postgres`
    54.5s，非 SKIP 形态）。日志未留命令行，`-p 1 -count=1` 以推送方隔离树重跑为准。
  - **生产停点**自此后移：授权仍停在未配置（票 03）；授权过了之后阶段按事实判出、矩阵按登记的声明答——登了才按声明，
    没登仍是待复核。`NotDeclared` 零值与「判不出阶段不猜」两条红线未动，矩阵一格都没内置。
  - 至此本票四段全落，Status 转 resolved；**main 上的 SHA 待推送方重放后由进 main 记录补**，合入前独立评审按
    2026-09-08 规矩由推送方指派，有阻断回本分支修。
- 2026-09-08 10:56 · **评审 ← 通道 6 · 钉 `e2a51ba8`**（基线 `d1e6c094`；评 `2e085cad` + `5e5e2c05`；隔离树只读，未跑测试；
  两轴串行独立过）。原文由通道 2 代录：
  - **Standards** · 阻断：无。非阻断：
    1. `internal/parcelshipment/adapters/partycommercial/unconfigured_source_data_amendment.go` 文件末尾游离注释「矩阵那一口原先与本文件
       同居的 UnconfiguredSourceDataRuleDeclaration…已…退场」——AGENTS.md「写代码注释：不写变更说明」；不挂任何声明、只叙述一次删除，
       git log 已记。同类：`unconfigured_source_data_amendment_test.go` Covers 末句「矩阵那只未配置适配器原先也在这里证，已随票 02 余段
       接真退场」。删掉不改语义。
    2. `source_data_amendment_allowance.go`：`declaredStageOf` / `declaredIntentOf` / 资料组译不过去与 `allowanceOf` 集外共用
       `ErrUntranslatableAnswer`（`judgment_as_of.go` 定义为「untranslatable answer」）。前三处标**查询**侧译不过去（恢复：改编排），后者标
       **答复**侧封闭集分叉（恢复：查两侧词表）；同一哨兵让 `errors.Is` 的读者分不出。判断项；今天编排把所有 error 折成
       `SourceDataRuleUnavailable`，实际不受影响。
    3. `cmd/parcel-api/assemble_customer_amendment.go` `buildSourceDataAmendmentAllowance`：`requests + resolutions → NewResolvedAdoptedStageOwner`
       + `NewStageContentDeclarations` 与 `cmd/parcel-dispatch/assemble.go` 同形两处（现三处）；且 `pspostgres.NewShipmentRequests(db)` 在本函数与
       `assembleCustomerAmendmentOrchestration` 各建一只。Duplicated Code 判断项；跨二进制无处共享，无副作用。
    4. 同包构造门口径不一：`NewDeclaredSourceDataAmendmentAllowance` 对 nil 协作者返 error（用例给了理由），`NewDeclaredStageContent` 对 nil
       owners 换 `UnconfiguredAdoptedStageOwner`。新写法更诚实，只记分歧。

    无发现：红线「只实现已确认规则」——不含任何矩阵格 / 默认资料组或阶段，`NotDeclared` 仍零值；「证据层级诚实」——替身标 `S`，
    `seedAdoptedClosure` 写明「取证捷径，不是生产路径」；「所有权清晰」——PS 适配器只 import `pcdomain`/`pcports` 只读，
    `internal/partycommercial/` 零改动；注释全中文、跨文件引用无行号，「六格/三格」是 CONTEXT 封闭集本身且有测试钉住。PS CONTEXT
    「不得默认为最早阶段」——`declaredStageOf` 零值与集外一律上抛。ADR-0120 决定四——`allowanceOf` 只做 PC→PS 三值一对一，`closed` 未在
    PS 侧解释；决定五——`TestTheAdapterMirrorsParcelShipmentStageAndIntentWordsCellByCell` 钉 6+3 对 `String()` 逐字相等、无第七格/第四格，
    并以「只登一格」对 6×3 全查证 switch 译到正确格。ADR-0062——回指经 `NewResolvedAdoptedStageOwner(requests, resolutions)`，形参
    `CommercialResolutionView` 只读口，与 parcel-dispatch 同路径。
  - **Spec**（票 02 完成判据 + MCP-1 代裁 Q1–Q4 + 评审令点名五项）· 阻断：无。非阻断：
    1. 「封闭零格声明任一格 DISALLOWED」PS 侧只钉一格：`cmd/parcel-api` `TestTheAmendmentAssemblyReadsAClosedDeclarationAsDisallowingEveryUndeclaredCell`
       封闭零格只问（收件人 × 已接受尚未收寄 × 补充）；适配器单测 `TestTheAdapterTranslatesTheThreeValuesOneToOneAndLeavesTheClosedReadingToTheProvider`
       的封闭用例带一格声明而非零格。因适配器无逐格逻辑、`closed` 语义在 PC `AllowanceFor` 且 PC 领域测试已钉，一格足证透传；可补不必补。
    2. 票 02 Status/Comments 完成记录不在这两笔（落地时写），只记未见。

    无发现：裁决 (c) 路径——`DeclareSourceDataAmendment` 按 `AcceptanceRulePackageFor` → `LoadSourceDataAmendmentAllowance(tenant, owner)` →
    `AllowanceFor(group, stage, intent)` → 译三值逐步照做，租户与版本来源由 `TestTheAdapterReadsTheDeclarationOfTheAdoptedRulePackageForTheQueryingTenant`
    钉住。闭包不在 / 无父行答 NotDeclared、读不回原样上抛——两处 `!found → none, nil`，两处 error `%w` 包原错；
    `TestTheAdapterAnswersNotDeclaredWhenNobodyHasSaidAnything`（含闭包不在时 view.calls==0）与
    `TestTheAdapterSurfacesProviderFailuresInsteadOfFoldingThemIntoNotDeclared` 钉住。判不出阶段 / 未声明意图在读任何东西之前拒答——三项翻译都在
    `AcceptanceRulePackageFor` 之前，测试钉 `owner.calls==0 && view.calls==0`；编排侧核过 `amend_customer_source_data.go` 问矩阵前已
    `!command.Intent.Declared()` 上抛、`!stage.Determined()` 停未决，适配器注释属实。Q3 缺格语义——适配器单测三格 + 真库
    `TestTheAmendmentAssemblyAnswersFromTheRegisteredAllowanceDeclaration` 四格（未登→待复核；允许→RECORDED 形成版本且意图恰好一封；
    不允许→DISALLOWED 版本意图不多；未登资料组→待复核），声明经 PC 写口 `SaveSourceDataAmendmentAllowance` 登记。授权那只未动——
    `buildCustomerAmendmentOrchestration` 仍传 `UnconfiguredSourceDataAmendmentAuthorizer{}`，该类型与其用例原样在，票 03 地盘未碰。票 02 红线——
    未触 `AT-PS-020` / UC-PS-002 / PS CONTEXT。既有用例②停点语义如实改写为「`SYN-RES-1` 在 PC 解析库无闭包」。无多做——`amendmentCommandFor`
    仅让资料组与意图可变。
  - **结论**：Standards 0 阻断 / 4 非阻断；Spec 0 阻断 / 2 非阻断。**无阻断，可重放。**
  - 通道 2 处置（代录时记）：六条非阻断均随票记、不挡合入，本分支不再动 `.go`（推送方已在重放）。Standards (1) 两句变更说明注释与 (2) 哨兵
    分格，留待下一次碰这两个文件时顺手，不另立票；(3)(4) 判断项记分歧；Spec (1) 一格足证透传，认同「可补不必补」；Spec (2) 已由 `80f11fa2` 补齐。
