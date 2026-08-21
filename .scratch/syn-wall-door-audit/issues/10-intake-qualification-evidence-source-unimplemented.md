# 收寄硬资格证据口无生产实现,资格证据无处可登

Category: enhancement
Status: resolved

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W11 证据面。

## 墙

采用链的硬资格证据口在两处装配(`cmd/parcel-dispatch/assemble.go` 的 `networkIntakeAdoption` 与 `adoptEffectiveDeliveryConsumer`)都给 `pspartycommercial.UnconfiguredIntakeQualificationEvidence{}`——ADR-0063 的「显式未配置」实现:资格要求(如 `INTAKE-QUAL/customs-precheck`)一经声明即答未证明,采用停在 `ELIGIBILITY_UNDECIDED`。

## 现状

- 端口与「诚实无门」实现在;证据的存储、装载口、写入方、登记口全缺。
- 合成种子(`syn_pc_seed_test.go` 的 `seedIntakeQualification`)只在测试里给证据,生产路径没有任何来源可登。
- 注意与票 03 的分界:资格**要求**由 PC 声明表携带(票 03 的发布面);本票管资格**证据**——某对象已满足要求的事实从哪来、登在哪。

> **上面第一条的「全缺」已不成立（机制半边已落地，见 Comments 2026-08-20 MCP-2 与
> 2026-08-21 MCP-4 两条）。** 原句保留——它记的是审计基线 `49a2ab0`
> 当时的实况。今天四件各有着落：存储与写入方是 node-operations 已有的执行事实表与
> `RecordExecutionFact`（不另起第二本册子），装载口是 `NodeExecutionQualificationEvidence`，
> 登记口是装配点的 `nodeQualificationAuthority`。**生产路径今天仍答未证明，但那是没租户、
> 不是没来源**：缺的已收敛为实例半边——认领哪一段权威、哪条引用由哪件执行事实证。

## 缺的最小机制件

资格证据来源:证据登记存储 + 装载口 + 写入方(按 ADR-0063 的证据语义:逐对象/逐要求、带来源与有效性;证据可能来自 CC 预检结果等源上下文,归属先对照 CONTEXT 再定)。

> **落点已更正为「给 `KnownPrefix` 缝后面接真源」，不是另起证据口（见 Comments 2026-08-20
> MCP-2 那条）。** 原句保留，但「另起」这个读法不要再照着做：组合缝
> `KnownPrefixIntakeQualificationEvidence` 在审计基线之前就已存在（`7cb39b6`，ADR-0063 同笔），
> 票面当时漏记了它。归属那一问也已裁定，且答案不是原句设想的那个：证据来自 **node-operations
> 的执行事实**，不是 CC 预检结果——节点说得了做过什么，说不了监管认定了什么，正式关务判断归
> customs-compliance（ADR-0063 决定五）。逐项理由见 Comments 2026-08-21 MCP-5 那条。

## 红线

- nil 与显式未配置都不得默认 `ESTABLISHED`(装配点注释原话);本票不放松。
- 证据来源归属未裁前不擅自选边;若归属要改 CONTEXT,先走文档再落码。

## 参照

ADR-0063;PAR-COM-16;`parcelshipment/adapters/partycommercial` 的 `ServiceStageRulesAdapter` 装配注释。

## Comments

- 2026-08-20 MCP-2：对 `3324ecb` 重核四件，**结论不变：证据的仓储/装载口/写入方/登记口
  全缺**。两处装配点仍显式未配置——`cmd/parcel-dispatch/assemble.go` 的终局图与
  `networkIntakeAdoption` 图都传 `UnconfiguredIntakeQualificationEvidence{}`（注释原话
  「nil 会在非空清单上变成依赖错误，两者都不得默认 ESTABLISHED」原样）；
  `seedIntakeQualification` 仍只在测试（`cmd/parcel-dispatch/syn_pc_seed_test.go`）。
  **票面一处漏记照实更正**：`KnownPrefixIntakeQualificationEvidence`（按引用前缀把证明
  路由到已登记权威口）在审计基线之前就已存在（`7cb39b6`，08-18，ADR-0063 同笔），
  票面「端口与『诚实无门』实现在」少数了这件——**组合缝已有**，今天缺的是缝后面的
  证据来源四件与把组合口装进两处装配点那一步；「缺的最小机制件」范围不因此变小，
  但落点应从「另起证据口」改读为「给 KnownPrefix 缝后面接真源」。

- 2026-08-21 MCP-5（A 半边：节点权威口本体，不接线）：新增
  `internal/parcelshipment/adapters/nodeoperations/intake_qualification_evidence.go`
  与同名 `_test.go`。落点照 MCP-2 的更正——不另起证据口，给 KnownPrefix 缝后面接
  node-operations 这个真源，实现 `psports.IntakeQualificationEvidenceView`。
  **端口一行未改**：`IntakeQualificationEvidenceView` 与 `IntakeQualificationProof`
  的现有签名正好够用，`internal/parcelshipment/ports/ports.go` 因此没有进入本票范围
  （MCP-1 已放行改它，用不上）。`assemble.go` 一行未动。

- 2026-08-21 MCP-5（逐项理由：哪些 NO 事实能为哪些引用作证）：
  **先说清判的是什么。** 没有租户就没有资格声明，具体引用清单属实例半边，今天造不
  出来；所以这里定的是**可作证的事实类别与其边界**，不是一张引用清单。
  - **能作证的只有 `NodeExecutionFact` 一类**，它自述的内容是「租户 T 下、为协作事项
    I、对作业实物 U、在时刻 P 执行了动作 A，有证据 E」。`RecordExecutionFact` 已经在
    NO 侧拦掉越出承接范围的对象与动作，所以能读回来的事实天然在授权范围内。
  - **动作只有封闭五值**（`UnsealAction`/`IsolateAction`/`PresentAction`/`TallyAction`/
    `ObserveAction`），逐个说得了什么：开封只证箱体已开，隔离只证该实物已被隔离存放，
    呈验**只证已呈交查验**，清点只证已清点，观察只证已到场看过。
  - **逐个说不了什么，且这才是要紧的一半**：呈验证不出「监管已查验」「已放行」「查验
    结论如何」，清点证不出「清点数与申报相符」，隔离证不出「隔离理由成立」。这些是
    结论不是动作，`NodeExecutionFact` 类型上没有任何字段能承载它们（NO CONTEXT 与
    ADR-0063 决定五：正式关务判断归 customs-compliance）。
  - **命名空间边界**：本口只为它认领的那一个权威段作证。跨段转写（把「呈验完成」兑成
    `INTAKE-QUAL/customs-precheck` 已证明）一律禁止——那正是 ADR-0063 Consequences
    点名的「为了变绿拆假关务身份」。
  - **来源边界**：只有 `NodeIntakeSource` 这一路的来源对象是节点作业实物。场外揽收对象
    来自 transport-fulfillment，两侧编号偶然同名就会误证，因此直接答未证明且不去问节点。

- 2026-08-21 MCP-5（MCP-1 三道护栏的落点，逐条）：
  1. **命名空间不得跨界** — 不止写进注释，另加两道结构拦截：构造期拒收前缀不属本口的
     登记项，判断期对段外引用直接答未证明（组合口误路由时的兜底，答未证明不答错）。
     用例 `TestTheRegistryRefusesReferencesFromAnotherAuthoritySegment` 与
     `TestAMisroutedSeamStillCannotProveAnotherAuthoritySegment` 各守一道。
     **残余照实记**：装配点若**同时**把前缀声明成别的权威段、又登进该段的引用，机制拦
     不住——但那要在两处显式写下同一个外段前缀，不再是手滑，注释已点名其为禁止。
  2. **前缀字面量是实例值** — 生产代码里一个前缀字面量都没有：`authorityPrefix` 由装配
     点交进来，`AuthorityPrefix()` 供组合口按它登记，两处不各写一份。测试里的
     `NODE-OPS` 只是取值，注释已标明生产装配为空。
  3. **asOf 不得碰时钟** — 有效性只比 `record.Fact.PerformedAt()` 与传入的 `asOf`，全文件
     无 `time.Now()`。`TestEvidenceIsValidOnlyIfItPrecedesTheIntakeBusinessTime` 的
     `receivedAt` 是固定过去时刻，改用时钟这条会红。

- 2026-08-21 MCP-5（B 票要接什么）：
  - 两处装配点（`cmd/parcel-dispatch/assemble.go` 的 `networkIntakeAdoption` 与
    `adoptEffectiveDeliveryConsumer`）把 `UnconfiguredIntakeQualificationEvidence{}`
    换成 `NewKnownPrefixIntakeQualificationEvidence`，登记键取 `view.AuthorityPrefix()`，
    不要另写字面量。
  - `view` 由 `NewNodeExecutionQualificationEvidence(facts, 前缀, 登记表)` 构造；`facts`
    传 `nodeoperations/adapters/postgres.ExecutionFacts`（形状已由测试里的编译期断言钉住）。
  - **一个决策留给 B**：构造器要求前缀非空，而无租户时登记表为空。是「不配前缀就干脆
    不接节点口、继续显式未配置」，还是「接一个空表节点口」——两者行为等价（都恒答未
    证明），但恢复动作的可读性不同，B 定并在装配注释里说清。
  - 前缀与登记表本身是实例配置，无租户前不得写死默认值。

- 2026-08-21 MCP-4（B 半边：接线完工）：装配点已走证据口这道缝，机制半边到此闭合，
  实例半边（认领哪一段、哪条引用由哪件事实证）照旧留白。
  - **（a）/（b）取 (a)，但把恢复动作摆到台面上**。(b) 立不住：构造器拒空前缀，接空表
    节点口就得先在生产代码里写下一个前缀字面量，撞红线「生产代码里一个前缀字面量都不
    该有」。所以未认领时仍交 `UnconfiguredIntakeQualificationEvidence{}`；差别在于它现在
    由 `intakeQualificationEvidence(db, nodeQualificationAuthority{})` 交回，而
    `nodeQualificationAuthority` 的两个字段就是租户出现那天要填的全部东西——一个前缀与
    一张表。原方案那句「继续显式未配置」的问题不在行为而在可读性：读的人得先翻一遍
    适配器包才认得出还有个节点权威口可以接。
  - **零值读作「本部署没认领任何段」，不是「忘了填」**；两者判断结果相同而要人做的事
    相反，所以另加一道装配期校验：填了登记表却没认领前缀直接拒绝启动（那是一张永远
    问不到的表）。
  - **`adoptEffectiveDeliveryConsumer` 那处刻意不接，与 MCP-5 上一条不同**，理由是类型
    层的：`FormParcelFinalDeps.Rules` 的类型是 `psports.FinalRuleView`，该接口只有
    `JudgeFinalOutcome`；`JudgeIntakeEligibility` 属 `IntakeEligibilityView`。终局链静态
    走不到硬资格那一格，在那里接上权威段等于替它写下一条并不存在的依赖。该处保留显式
    未配置并在注释里写明为何留白。两道实例墙本就分开登：收寄停在
    `INTAKE_QUALIFICATION_UNPROVEN`（PAR-COM-16），终局停在 `FINAL_RULE_UNCONFIGURED`
    （PAR-COM-17），接进来就是拿前者顶后者的停点。
  - **「反正行为一样」不是留白的理由，这一句是 MCP-1 复核时纠正的**：一样只在
    `FinalRuleView` 单方法这个静态事实成立时成立。哪天它不成立——接口长出第二个方法，
    或有人在终局链上把该适配器当 `IntakeEligibilityView` 用——显式未配置答未证明是安全
    的那一边，真适配器则会默默给出一个没人要的判断。所以这是 fail-safe 的选择而非中性。
  - **ADR-0063 决定五那道残余仍在**：前缀声明成 customs-compliance 才说得了的段、又把该
    段引用登进去，构造期两道校验只比两者是否同段，拦不住。注释已点名，由填值的人守。
  - 装配级用例四条（`cmd/parcel-dispatch/intake_qualification_evidence_wiring_test.go`）：
    生产零值恒答未证明且不碰库、认领段后答案跟着真库执行事实走（先未证明后已证明）、
    段外引用不被兑成已证明、半份配置被拒。走真库而非替身——替身换掉 `ExecutionFacts`
    就绕开了本文件唯一要证的那一段。
  - **落地**：接线本体 `eafb2b1` 与终局装配点注释 `0280d51`，两笔**均可从 `main` 到达**
    （集成当时 `origin/main` 为 `d5e5d20`，此后 main 一直在动——锚点写成等式会过期，
    写成可达性不会）。集成树上重跑全量真库套件：退出码 0、FAIL 0、**SKIP 0**、71 包、
    `-p 1` 串行。SKIP 0 是关键项——退出码 0 与「全部跳过」兼容，SKIP 0 不兼容。
  - **本票只闭合机制半边**：实例半边（认领哪一段权威、哪条引用由哪件执行事实证）等租户，
    Status 是否转终态留给 triage，我不自行改。

- 2026-08-21 MCP-3 提出 / MCP-4 记入：**本票那句「零值读作没认领、不是忘了填」有前提，
  引作判例前先核前提。** 它成立靠的是**零值与漏填在类型上分得开**：本口的实例配置是
  `prefix` 与 `qualifying` **成对**的，半份（有表无前缀）检得出来，所以装配期拒得掉；
  两份都空则读作「没认领」，而那正是今天的真话，恢复动作也一致。
  **单字段的维度不满足这个前提。** 反例就在同一个 `assemble.go` 里：`tenantBoundProjectionDerive`
  与 `tenantBoundCustomerViewDerive` 的租户维只有 `domain.TenantID` 一个字段，`domain.TenantID{}`
  说不出自己是「没认领」还是「忘了填」——注释已把危险写成禁令（「禁止
  `NewMilestoneMappings(db, 空租户)`——看起来接了库、永远读不到行」），却只能靠每个包装各
  手抄一道空值守卫，抄漏一份没有任何东西会红。
  **差别不在认识水平而在结构**：本口所有消费者都过同一个工厂，检查写一次且有用例钉住；
  那两处各建各的包装，检查各抄一份且有一份没钉。**一帖好药方被当通用解药开出去比没有
  药方危险**——照抄本票结论前，先问这一维的零值与漏填分不分得开。

- 2026-08-21 MCP-1 裁定 / MCP-4 执行：**转 `resolved`**。判据是「缺的最小机制件」列的三件
  今天都有着落（存储与写入方＝NO 的执行事实表与 `RecordExecutionFact`，装载口＝A 半边，
  接线＝B 半边），**实例半边留空不阻终态**，与票 02 同一标准。转态前已按票 09 的做法把
  「现状」与「缺的最小机制件」两处过时前提就地标注、原句保留——不标的话，下一个人读到
  「全缺」与「另起证据口」会把已结的票重开。ADR-0063 决定五那道机制拦不住的残余不另开票：
  它写在装配注释里，而会踩到它的人正在改那段装配代码，那就是它该在的地方。
