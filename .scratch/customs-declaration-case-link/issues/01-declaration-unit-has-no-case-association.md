# 申报单元 → 关务案件的关联在域模型里缺席，只活在文档

Category: bug
Status: ready-for-agent

CC CONTEXT 的「关务案件」词条写着「**一个案件可以关联多个申报单元和多次提交**」，而代码里
没有这条关联：`DeclarationSubmissionKey` 三维（租户+申报单元+程序）不含案件维，申报提交口的
载荷同样不含，`submit_declaration.go` 全文对 `Case`／`案件` 零命中。文档说得出的关系，代码里
取不出来。

> **「代码里没有这条关联」要收窄（2026-08-21 对 `9e5c5c0` 取证，见 Comments 第〇节）。**
> 逐条列举那三件属实，原句保留。但案件引用本身在 CC 里到处都是（一律为裸 `string`），
> 而这条边确实在 `FollowUpTarget` 上存在一次——它持有 `CaseRef` 与 `Unit` 两者，只是位于
> 提交**之后**的后续动作目标上，不在提交路径上。另有两件票面没有且改变代价估算的事实：
> 申报单元今天**没有任何持久化本体**（无表、无仓储、无装载口），以及拿着案件标识**反查不回
> 案件**（`CustomsCaseStore` 只有五维范围键读口，尽管库侧唯一约束已现成）。

从 [outbox-partition-key 票 03](../../outbox-partition-key/issues/03-step-two-scope-eight-ports-and-four-undecided.md)
的裁断轮里分出来（[ADR-0069](../../../docs/adr/0069-customs-case-chain-ordering-absorbed-by-reread-and-retry.md)
决定四点名另票）。那一轮要答「案件链要不要保序」，答案的一半卡在这里：**申报口今天连案件维
都取不出，想把它归进案件分区就无从谈起**。裁断因此绕开了它——四口不建立跨口保序，乱序由重读
与重试消化——但绕开的是保序需求，不是这条关联本身。

## 这条边缺席的最硬后果：案件关闭核对今天靠登记内容兜底，不是靠结构

拍板时真正要称重的是这一条，不是「关务案件」词条那句本身。

CONTEXT 要求案件关闭前「在明确业务截点盘点**全部适用申报**、限制、监管处置、税费及其他应履行
义务，并逐义务、逐范围形成关闭依据项」，并规定「任一未解决或冲突项都阻止关闭，**单个案件不存在
部分关闭**」。**而关闭核对是已经实现的编排**（`close_customs_case.go` 的 `CloseCustomsCaseHandler`）。

今天它只能盘点 `ObligationInventoryView.LoadObligationItems` 按 `caseRef` 交回的义务项清单。
**申报那一类义务是否齐备，代码里没有任何路径能自行核出来**——因为从案件走不到它的申报单元集。

于是：**「单个案件不存在部分关闭」这条不变量今天由登记内容承担，而不是由结构承担。** 有人把申报
义务作为一条人工登记项写进登记册，它就被盘到；没人写，关闭核对照样通过，且不会有任何东西变红。
这不是「缺一条边」的轻量级问题，它是一条已确认不变量的承载方式问题。

## 时序约束：这一条比两条候选路径本身更急

**若把案件维加进申报提交载荷并设为必填，必须赶在提交口接上生产装配之前做。**

依据（取证于 `9e5c5c0`，见 Comments 第二节）：上游 `SubmitDeclarationHandler` 今天零生产调用点，
生产上一封在途信封都没有；而**下游 VE 消费方已经接线**（`cmd/parcel-dispatch/assemble.go` 的
`deriveDeclarationSubmissionConsumer`），且 `decodeFormedDeclarationSubmission` 把**任一键维缺席
判为毒丸**（`TestADeclarationSubmissionEnvelopeMissingAnyKeyDimensionIsPoison` 逐维钉过四格）。

**今天迁移窗口是免费的；接线之后旧信封集体变毒丸，就要多一套兼容期。**

这一条与第一、三两问的性质不同，排期上要分开看：**后两问是领域裁断，拖一周没有代价；这一条拖过
接线那一刻就永久多一笔预算。** 它也不取决于第三问怎么裁——无论关联建在单元上还是案件上，只要
案件维进载荷，这条时序都成立。

> 同一形状的清单已开票承接：[棘轮门禁](../../production-wiring-ratchet-gate/issues/01-production-ports-wired-only-in-tests-have-no-ratchet.md)
> 扫出的「零生产调用点」同时就是「载荷还能免费改」的窗口清单。

## 卡在哪

要把案件维放进申报侧的任何位置（载荷、Subject、将来的分区维），先得有其中之一：

- 申报单元 → 案件的可查关联（域模型里的一条边，带持久化）；或
- 提交意图在输入侧就携带案件引用。

两者今天都没有。缺哪一个、由谁提供、关联建在申报单元上还是建在案件上，是建模问题不是接线
问题——`customs-compliance` 当前无主，需要先定所有权再动。

## 已经定死的边界（不要在本票里重开）

- 关联落地后，申报意图在**载荷/Subject** 补案件引用，**不进分区键**（ADR-0069 决定四）。
- 案件维一律用铸造 `CustomsCaseID`，不用五维范围键——范围键的职责收敛为建案幂等
  （ADR-0069 决定三）。
- 申报信封 ID 缺版本维是**另一件事**，归
  [declaration-envelope-version-dedup/01](../../declaration-envelope-version-dedup/issues/01-envelope-id-lacks-version-dimension.md)，
  两票互不吸收。

## 红线（取证于 `9e5c5c0` 新增，见 Comments 第三节）

- **不得经包裹推导这条关联。** 两端今天各自都持有包裹集（`CustomsCase.parcels` 与
  `DeclarationUnit.members`），所以求交在技术上做得出来——**而它给出的是看上去有值、实则未定义
  的答案**，两条 CONTEXT 各堵一头：「一个包裹可以先后关联多个关务案件」使交集是多对多而非唯一
  关联；受控跨客户合报允许「把不同货主客户账户的包裹纳入同一申报单元」，同一单元的成员可以散在
  不同案件里。**这条路最危险的地方是它能跑绿。**
- **若只把新加那一处做成铸造 `CustomsCaseID`，就会留下两种案件引用表达并存，而今天没有任何东西
  拦着。** CC 现有的案件引用（`FollowUpTarget.caseRef`、`ClosureVerification.caseRef`、
  `CustomsCaseClosure.caseRef`、`CloseCustomsCaseCommand.CaseRef`、
  `ObligationInventoryView.LoadObligationItems` 的形参、`CaseClosureStore.FindByCase`、案件配置
  登记册五本）全是裸 `string`。**这与分区键碰撞票是同一形状**：两处各自合规，撞在一起才错——
  一个铸造标识与一个裸串指同一个案件时，类型上分辨不出、编译器不会拦、用例各自全绿。统一与否
  属本票范围内要一并回答的问题，此处只钉住「不许无声地留成两种」。

## 本票不做的事

- 不改分区键。申报口的分区键已由 ADR-0069 判为「无先后」，门禁例外行已按此改写。
- 不替 `customs-compliance` 定所有权。

## Comments

- 2026-08-21 MCP-2（T3 裁断取证，基线 `9e5c5c0`，零代码；只写事实、候选与代价，不拍板）：

### 〇、票面「代码里没有这条关联」要收窄，收窄后的说法更难办

原句作为整体不成立，按两半拆开才准：

- **案件引用在 CC 里到处都是**，只是一律为裸 `string` 而非铸造标识：`FollowUpTarget.caseRef`、
  `ClosureVerification.caseRef`、`CustomsCaseClosure.caseRef`、`CloseCustomsCaseCommand.CaseRef`、
  `ObligationInventoryView.LoadObligationItems` 的 `caseRef` 形参、`CaseClosureStore.FindByCase`、
  案件配置登记册五本的 `CaseRef`。
- **而「申报单元 → 案件」这条边确实只在一处存在**：`FollowUpTarget` 是全域唯一同时持有
  `CaseRef` 与 `Unit DeclarationUnitID` 的类型。但它是**提交之后**的对象（后续动作目标），不在
  提交路径上。提交路径全程无案件维——`SubmitDeclarationCommand`、`formUnit` 造出的
  `DeclarationUnit`（只有 `id` / `procedure` / `members`）、`DeclarationSubmissionKey` 三维、
  `declarationSubmissionPayload` 四字段，一处都没有。

**两处新增的硬事实，票面都没有，且都改变代价估算：**

1. **申报单元今天没有持久化本体。** 全仓无 `declaration_unit` 表、无 `DeclarationUnitStore`、
   无 `DeclarationUnitView`（三个名字全仓零命中）。`DeclarationUnit` 是 `formUnit` 从命令入参
   现造的瞬时值，成版那一刻被快照进 `declaration_submission.members` 的 jsonb 就不再有独立行。
   **没有「申报单元这一行」可以挂边**——这一点直接决定第三问。
2. **拿着 `caseRef` today 反查不回案件。** `CustomsCaseStore` 只有
   `FindByKey(CustomsCaseKey)`，键是租户+辖区+方向+程序+义务范围五维范围键，**没有按案件标识
   反查的读口**（同一结论 `.scratch/ve-remaining-fact-sources/report.md` 已就 `case-closure`
   独立记过）。好消息是库这一层现成：`customs_case` 表已有
   `CONSTRAINT customs_case_id_unique UNIQUE (tenant_id, case_id)`，缺的只是端口方法与适配器
   一条 SQL。

### 一、CONTEXT 原句，与依赖这条关联的句子

原句在「关务案件」词条：

> 围绕明确监管辖区、进出口方向、监管程序和法定义务范围建立的稳定业务案件。**一个案件可以关联
> 多个申报单元和多次提交**；出口、进口及其他独立监管程序分别建立案件。

紧邻的上下文两句限定了它的形状，取证时不能漏：Rules 里「关务案件建立时必须固定监管辖区、进出口
方向、监管程序和法定义务范围」，以及「一个包裹可以先后关联多个关务案件，但任何案件都只能解释
自身固定的监管范围」。**后一句是关键**——见第三问。

CONTEXT 里依赖这条关联的句子（按依赖强度排，全部为原文摘引）：

| 依赖句 | 依赖什么 | 今天可满足否 |
|---|---|---|
| 「目标必须关联触发依据、**原案件、原申报单元**、原提交版本、明确范围和拟提交动作」 | 单元↔案件同时在场 | **唯一已实现的一处**（`FollowUpTargetSpec` 两者俱全），但案件维是裸 `string` |
| 「关务案件关闭前必须在明确业务截点盘点**全部适用申报**、限制、监管处置、税费……逐义务、逐范围形成关闭依据项」 | 案件 → 该案下全部申报单元 | 否。`ObligationInventoryView` 按 `caseRef` 取义务项，取不到申报单元集 |
| 「监管规则允许的更正或补充**在原案件内**形成新的正式申报资料和提交版本」 | 单元 → 案件 | 否 |
| 「原申报单元不能继续使用但**案件固定辖区、方向、程序和法定义务范围不变**时，建立替代申报单元；这些固定身份变化……建立替代或后续关务案件」 | 单元 → 案件的四维范围 | 否。分支判据取不出来 |
| 「进行中：可以形成**一个或多个申报单元**、资料版本、提交、监管决定、限制、税费结果和处置关系」 | 案件 → 单元集 | 否 |
| 「已接受引用能够与**关务案件、申报单元和运输对象**逐范围唯一匹配 → 形成业务关联」 | 三者可同时定位 | 部分。`ExternalManifestReference.Association()` 只关联到申报单元，案件那一维缺席 |
| Boundaries：「`customs-compliance` 拥有关务案件、**申报单元及其组成**、替代和后续案件关系……」 | 所有权语句本身预设了这条边 | 否 |

**第二行是最硬的一条，已按协调岗裁定提到票面正文单列一节**（「这条边缺席的最硬后果」）：案件
关闭核对是已实现的编排（`close_customs_case.go`），而 CONTEXT 要求它盘点「全部适用申报」；今天它
只能盘点 `ObligationInventoryView` 按 `caseRef` 交回的义务项清单，申报那一类义务是否齐备代码里
没有任何路径能自行核出来。**这不是缺一条边那么轻——「单个案件不存在部分关闭」这条不变量今天由
登记内容承担而非由结构承担，而拍板时要称重的正是这一条，不是词条句本身。**

### 二、两条候选路径的代价与影响面

**先给共同的好消息，它把两条路的迁移成本都压掉了一大截**：`NewSubmitDeclarationHandler` 与
`NewEstablishCaseHandler` **在全仓非测试代码里零调用点**，唯一构造点各自在
`submit_declaration_test.go` 与 `establish_customs_case_test.go`。**上游生产方今天没有接线**，因此
改命令、改键、改记录都不会波及任何生产调用方。

**但下游消费方已经接线，这一半必须单独算**：`cmd/parcel-dispatch/assemble.go` 里
`deriveDeclarationSubmissionConsumer` 与 `deriveCustomsCaseConsumer` 都在装。而
`decodeFormedDeclarationSubmission` 把**任一键维缺席判为毒丸**
（`TestADeclarationSubmissionEnvelopeMissingAnyKeyDimensionIsPoison` 逐维钉过四格）。

> **由此得到一条与本票内容无关、但对排期有决定性的结论**：若候选（b）把案件维加成载荷必填，
> 那么它**必须赶在提交口接上生产装配之前做**。今天生产上一封在途信封都没有（生产方未接线），
> 迁移窗口是免费的；接线之后再加，旧信封会集体变毒丸，就得多一套兼容期。

**候选（a）申报单元 → 案件的可查关联（域模型一条边 + 持久化）**

| 要改什么 | 具体 |
|---|---|
| 域 | `DeclarationUnit` 加案件维；`FormDeclarationUnit` 签名加参 |
| 类型 | `SubmitDeclarationCommand` 加案件入参；`formUnit` 跟着改 |
| 键 | `DeclarationSubmissionKey` **是否加案件维要另判**——加了会改幂等口径（同一单元换案件即换键），不加则边只落在记录不落在键 |
| 表 | **需要新建 `declaration_unit` 表**（今天不存在），或退而把案件列加进 `declaration_submission`；后者把边挂在「提交」而不是「单元」上，与 CONTEXT 的「一个案件可以关联多个申报单元」错位——未提交的单元就不在册 |
| 载荷 | `declarationSubmissionPayload` 加 `caseId` |
| 调用方 | 生产零；测试两处 |

**真代价不在改动量，在那张缺席的表。** 候选（a）的正解要求申报单元有独立持久化身份，而这是
CONTEXT 硬句 143 本来就要求的（「申报单元必须具有独立身份和可追溯组成」）。所以（a）**实质上是
一张「补建申报单元本体」的票**，比票面「域模型一条边 + 持久化」这句听起来大。

**候选（b）提交意图在输入侧就携带案件引用**

| 要改什么 | 具体 |
|---|---|
| 类型 | `SubmitDeclarationCommand` 加案件入参 |
| 域 | `DeclarationUnit` 可不动 |
| 记录 | `DeclarationSubmissionRecord` 加案件列；`declaration_submission` 表加一列 |
| 载荷 | 加 `caseId` |
| 校验 | **要不要核这个案件真的存在？** 要核就得先补 `CustomsCaseStore` 的按标识反查读口（库侧唯一约束现成，缺端口方法与一条 SQL）；不核则等于让调用方随手填一个字符串，与 CC 现有那一堆裸 `caseRef` 同病 |
| 调用方 | 生产零；测试一处 |

**（b）的真代价是它把关联的正确性推给了输入侧。** 单元与案件的对应关系不由结构保证，两次提交同一
单元可以携带不同案件而没有任何东西会红——除非补上反查并加一条「同一单元的案件维不得变更」的
不变量，而那条不变量需要一个能按单元查历史的地方，绕回（a）缺的那张表。

**一条两者都要面对、票面未提的形状债**：CC 现有的案件引用全是裸 `string`，而 ADR-0069 决定三要求
案件维用铸造 `CustomsCaseID`。本票落地时若只把新加的那一处做成 `CustomsCaseID`，仓里会同时存在两
种案件引用表达。**统一与否是本票要一并回答的**，我不预判该不该在本票里统一——那取决于所有权。

### 三、关联建在申报单元上还是建在案件上（不替 `customs-compliance` 定所有权）

**先排掉一条看起来可行、实则不可行的路：经包裹推导。** 两端今天各自都持有包裹集——
`CustomsCase.parcels []CaseParcelAssociation` 与 `DeclarationUnit.members []DeclaredParcelReference`
——所以「按包裹求交」在技术上做得出来。**但它不成立**，两条 CONTEXT 各堵一头：一是「一个包裹可以
先后关联多个关务案件」，交出来的是多对多不是唯一关联；二是受控跨客户合报允许「把不同货主客户
账户的包裹纳入同一申报单元」，同一单元的成员可以散在不同案件里。**推导会给出一个看起来有值、
实则未定义的答案**，这比没有更坏。建议把这一条写进将来那张票的红线。

两个候选端各自的形状与代价：

| 建在哪 | 形状 | 代价 | 与今天代码的距离 |
|---|---|---|---|
| **申报单元上**（单元持有案件） | 单元 → 案件多对一 | 需要申报单元的持久化本体（今天不存在）；换案件即改单元身份，与 CONTEXT「建立替代申报单元」的语言天然对齐 | 远：要补表、补仓储、补装载口 |
| **案件上**（案件持有单元集） | 案件 → 单元一对多，直接照抄 `parcels` 那一列的形状加一列 `units jsonb` | 表现成、改动最小；但**反向查不动**——拿着单元找案件要扫 jsonb，而提交路径要的正是反向 | 近：一条迁移加一个字段 |

**两者的取舍其实是同一个问题的两面：CONTEXT 那句是从案件方向写的（「一个案件可以关联多个申报
单元」），而提交路径需要的是反方向。** 建在案件上贴合文档语言但答不了提交路径的问题；建在单元上
答得了提交路径但要先造出单元本体。第三种是双向登记（独立关联表），代价是要自己维护一致性，好处
是两个方向都能查、且不动任何一端的既有形状。

**我的推荐，标明是推荐**：若所有权最终落定并允许开工，走**建在申报单元上**，理由不是提交路径方便
（那是（b）的理由），而是 CONTEXT 硬句 143 已经独立要求申报单元具有独立身份和可追溯组成——那张表
迟早要建，本票只是第一个撞上它的。**但这个推荐依赖所有权先定**：它把本票从「加一条边」放大成
「补一个聚合」，那不是取证方能替 owner 决定的规模。

### 四、Status

`needs-triage` → `ready-for-human`，已改到票面。三问取证已齐，两条候选的代价与影响面已具体到类型
与表，但**三件都需要 owner 先答**：申报单元要不要有持久化本体、关联建在哪一端、案件引用要不要统一
成铸造 `CustomsCaseID`。不存在可由 agent 直接开工的机制件。

未报 ADR：本票取证阶段未触及难逆转取舍。但若 owner 选「建在申报单元上」，那一步会新增一个聚合与
一张表，并可能改动 `DeclarationSubmissionKey` 的幂等口径——**那两件应当走 ADR**，本票不预判。

`Category` 保留 `bug`：CONTEXT 已确认的一条关系在代码里取不出来，且已实现的案件关闭核对因此
无法按结构盘点申报义务，不是新能力缺失。

- 2026-08-21 MCP-2（**三问已裁，本票转 ready-for-agent**。用户当日授权代裁，裁决全文与能力
  边界见 [ADR-0073](../../../docs/adr/0073-declaration-unit-is-a-persisted-aggregate-holding-its-case.md)）：
  ① 申报单元要有持久化本体——CONTEXT「独立身份和可追溯组成」本就要求，jsonb 快照承载不了
  替代关系与追溯；② 关联建在申报单元上，多对一、成立即定、换案件即替代单元；反向查询按
  单元表案件列，不建第二份存储；`DeclarationSubmissionKey` 三维不动，幂等口径不变；提交链
  带案件维必填、写入前按标识反查核存在（补 `CustomsCaseStore` 反查读口，库侧唯一约束现成）；
  ③ 案件引用统一走铸造 `CustomsCaseID`，新增处一律铸造，存量收敛开
  [02 票](./02-bare-case-refs-converge-to-minted-customs-case-id.md)显式跟踪。红线（不得经
  包裹推导）与时序门（案件维必填先于提交口接生产装配）均进 ADR 正文。实现方开工前按票面
  取证核对当时代码状态；新增聚合与表属实现票范围，走常规验证流程。
