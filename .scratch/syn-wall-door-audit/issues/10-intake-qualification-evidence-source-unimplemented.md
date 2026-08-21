# 收寄硬资格证据口无生产实现,资格证据无处可登

Category: enhancement
Status: ready-for-agent

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W11 证据面。

## 墙

采用链的硬资格证据口在两处装配(`cmd/parcel-dispatch/assemble.go` 的 `networkIntakeAdoption` 与 `adoptEffectiveDeliveryConsumer`)都给 `pspartycommercial.UnconfiguredIntakeQualificationEvidence{}`——ADR-0063 的「显式未配置」实现:资格要求(如 `INTAKE-QUAL/customs-precheck`)一经声明即答未证明,采用停在 `ELIGIBILITY_UNDECIDED`。

## 现状

- 端口与「诚实无门」实现在;证据的存储、装载口、写入方、登记口全缺。
- 合成种子(`syn_pc_seed_test.go` 的 `seedIntakeQualification`)只在测试里给证据,生产路径没有任何来源可登。
- 注意与票 03 的分界:资格**要求**由 PC 声明表携带(票 03 的发布面);本票管资格**证据**——某对象已满足要求的事实从哪来、登在哪。

## 缺的最小机制件

资格证据来源:证据登记存储 + 装载口 + 写入方(按 ADR-0063 的证据语义:逐对象/逐要求、带来源与有效性;证据可能来自 CC 预检结果等源上下文,归属先对照 CONTEXT 再定)。

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
