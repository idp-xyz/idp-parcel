# 替棘轮门禁做它声明自己不做的那次判断：32 条未接线领域工厂逐条分类

Category: chore
Status: draft——分类已出，处置待裁

## 起因

一次「`party-commercial` 算不算做完了」的评估落成了
[四张票](../party-commercial-context-gaps/spec.md)。写票时发现那四处**一处都不在**
r27 交用户认可的留待清单上（那份清单只有五项：SA 三个目录读口 + PS `PAR-COM-13`、
`BD-PS-009`、VE 真实渠道凭证、外部标识关系子域、NR `PAR-NET-14`）。

追下去发现的不是疏忽，是**量具看不见这一类**。骨架完整判据的定义是

> 该切片的规则与判断都有执行器，不靠「尚未实现」分支蒙混过关

而 r27 给它的证据是三样：生产代码 `TODO|FIXME|not implemented|panic(` 命中 0 行、编排与
适配器文件计数、端口两口径缺口。三样对这一类**在构造上全盲**：

- 一个只有领域模型、无人消费的类型是**干净的完整代码**，不含任何标记；
- 它不是编排也不是适配器，计数看不见它；
- 端口清点两个口径都从 `internal/*/ports` **已声明的接口**出发（见 `portcensus.go` 头注），
  一条规则从来没被开过端口，它不在样本里——那不是缺席，是不在样本内。

r27 自己半知道这件事：它把 `PAR-NET-14` 手工拎进清单时特意注了「注意它不在判据 B 名单上，
是端口计数看不见的既知余量」。那是靠人手拎进来的一件。

## 但门禁已经有了，缺的是判断

`internal/architecture/production_wiring_ratchet_test.go` 是一道**生产接线棘轮**，
`internal/architecture/production_wiring_baseline.txt` 冻着 32 条。它盯的正是这一类：
`internal/<上下文>/domain` 下零生产（非测试）调用点的导出工厂函数。

它写得很讲究，而且明说自己不做产品判断：

> 这份名单不是一张缺陷清单，是一条基准线；它里面有多少是「接过线后来烂了」、多少是
> 「切片还没到」，本门禁分不出，也不打算分。

理由是「那是产品判断，门禁拿不到，也不该替它拍」。**而骨架完整判据要的恰恰是这个判断。**

于是链条闭合：门禁把 32 条「规则没有执行器」冻在那儿并声明自己不判断它们；宣布骨架完整时
没有人对这份名单做过那次判断。本目录做的就是那次判断。

## 方法与它的限度

**取证锚 `9d6063c`**（该 SHA 上 `go build ./...` 绿；落盘时 HEAD 已到 `0c4b5a1`，
`9d6063c..HEAD` 无 `.go` 无 `.sql`）。

**落盘那一刻工作树上有别人的在途改动，其中一部分正在改本文举证的东西**（2026-09-02 晚
`git status` 实测，未提交因而不在锚内）：新增 `migrations/party_commercial/0018_channel_account_use_authorization.sql`、
`internal/partycommercial/domain/channel_account_use_registration.go`、
`docs/adr/0093-channel-account-use-authorization-is-not-a-commercial-version.md`，并改动
`internal/partycommercial/domain/channel_account_use_authorization.go`；另有
`internal/parcelpricing/domain/batch_evaluation.go` 与
`internal/parcelshipment/domain/channel_candidate_cost.go` 两处新增。

记这一段是因为本文表里 `PublishChannelAccountUseAuthorization` 那一行写着「无表、无端口」
——**那句话锚在 `9d6063c` 上为真，而它可能在你读到时已经不为真了**。读表前先看该条目还在不在
`production_wiring_baseline.txt` 里：门禁名单是活的，本文是一次快照。

判据是：对每条工厂，查它所属能力在本上下文有没有端口、迁移表、应用编排、适配器四层，
再查编排实际调的是什么。**逐条核过的与按组推断的分开标**，见表内「取证」列。

另做了一支[探针](../domain-executor-audit/probe/main.go)补盲区：棘轮**刻意排除 `New*`**
（注释写明值对象构造器是另一族，列为后续），而探针按**类型可达性**量——从生产代码指名的
种子出发，沿字段类型与方法签名闭包，闭包外的导出领域类型即零生产消费者。1127 个导出领域
类型里报出 122 个。**122 是上界不是答案**，它至少混了三类，其中一类（`Resolution` 这种
只由未导出函数产出、导出了却只在包内用）是该改小写的设计瑕疵而非缺执行器。探针的价值在于
它捞出了棘轮网外的 `New*` 一族——`SupplierAgreement`、`ProductChannelMapping` 都在其中。

## 分类的关键：「等租户」解释不了缺调用点

原以为三分：等租户 / 真缺口 / 死码。**核完之后第一格塌了。**

> **一个缺失的调用点是代码事实，不是数据事实。**

如果接线在那儿，没有租户数据时它会照常返回「查无此行」或停在`未配置`——这正是本仓在别处
一贯的做法，`显式未配置`是全仓最常见的形状之一。所以「没有租户」解释得了**册子是空的**，
解释不了**没有人去读那本册子**。

基线里那句免责——「按本仓机制先行实例后到的顺序，切片还没开工的能力本来就该是这样」——
是一句**排期**主张，不是实例半边主张。而**八个切片现在全部记为达标，没有一个还没开工**，
那条免责对今天的任何一条都不再成立。

于是实际的分类是三格，都属机制半边：

| 格 | 含义 |
|---|---|
| **支路未接** | 同一用例的上游环节有编排，这一步没有调用方。规则实现了，触发不到 |
| **整能力未接** | 端口、表、编排、适配器一层都没有，能力从未接出 domain |
| **死码** | 同包已有正主，这是多余的第二个写法 |

## 32 条逐条

取证列：`核`＝逐条查过调用点与配套层；`组`＝按组结论推断，未逐条查。

### parcel-shipment（3）

| 条目 | 分类 | 依据 | 取证 |
|---|---|---|---|
| `AssessSafeHandoff` | 支路未接 | 产出`安全交接评估`，测试 9 次调用、包外零引用。而开发主线 PN-02 那一行把「安全交接判定」明列为达标的正面证据 | 核 |
| `CurrentPayloadCanonicalizationVersion` | 支路未接 | 同路径另一端（摘要函数）已由隔离 Intake 接上，版本报出口未接；基线注称等真渠道 `PAR-INT-01` | 核 |
| `RehydrateContinuedAttemptRegister` | 支路未接 | 票 label-channel/10 只落了领域层，读它的仓储适配器是下一层 | 核 |

### party-commercial（4）

| 条目 | 分类 | 依据 | 取证 |
|---|---|---|---|
| `PublishChannelAccountUseAuthorization` | 整能力未接 | 无表、无端口、无编排、无接入面。见[票 01](../party-commercial-context-gaps/issues/01-channel-account-use-authorization-has-no-executor.md) | 核 |
| `ResolveCreditPolicy` | 整能力未接 | 信用政策无正文表，`ports.go` 自注「版本壳可入册，正文册未建」。见[票 03](../party-commercial-context-gaps/issues/03-credit-policy-and-supplier-agreement-have-no-content-table.md) | 核 |
| `ManualReviewRequirementFor` | 支路未接 | 返回 `bool` 的谓词，非工厂；授权授予册已接，这一问未接 | 核 |
| `ValidateBeforeDecision` | 支路未接 | 返回同型 `Resolution` 的变换，即 `UC-PC-002` 步骤 8 的提交前重解。基线注称它「落在网里但不是工厂」——**成因分类仍成立，但它不是无关紧要的一条**：开发主线 PN-02 行把提交前重解写成已落地 | 核 |

### visibility-exception（6）

| 条目 | 分类 | 依据 | 取证 |
|---|---|---|---|
| `ResolveByBusinessTime` | 支路未接 | 按业务时间裁决事实冲突，产出 `ConflictJudgment` | 核 |
| `RaiseConflictSignal` | 支路未接 | 由 `ConflictJudgment` 立信号。编排 `raise_signal.go` 走的是 `OpenEpisode` + `ConcludeTriage` 那条路，**从不经过冲突这条支路** | 核 |
| `DecideDisclosure` | 支路未接 | 包外引用**全在测试里**（`notify_customer_test.go`），基线该注今日仍成立 | 核 |
| `EstablishCase` | 支路未接 | 应用层零引用（连测试引用都没有，只有 `NewCaseID`）。而案件的适配器与读面都在 | 核 |
| `SubmitEvidence` | 支路未接 | 同上，应用层零引用 | 核 |
| `PrepareDisclosure` | 支路未接 | 同上，应用层零引用 | 核 |

VE 被记为「达标—有裁定的显式留待」，留待项是**真实渠道凭证**（`NotificationChannelGateway`）。
这 6 条**没有一条**属于那一项：它们缺的是调用方，不是凭据。

### transport-fulfillment（7）

| 条目 | 分类 | 依据 | 取证 |
|---|---|---|---|
**七条已逐条核完**（2026-09-02，方法与逐条对表见[票 02](issues/02-transport-fulfillment-seven-need-per-entry-evidence.md)
的 Comments）：把 TF 八例编排各自调的领域构造摘出来与七条对表，**没有一例调过其中任何一条**。

| 条目 | 分类 | 依据 | 取证 |
|---|---|---|---|
| `EstablishSegmentWithPickup` | 支路未接 | 揽收两例编排造 `FormOffsitePickup` / `FormFulfillmentAttempt`，**不由揽收事实立履约段** | 核 |
| `EstablishSegmentWithHandover` | 支路未接 | `register_transport_handover.go` 造 `FormTransportHandover`，不立段 | 核 |
| `SummarizeHandovers` | 支路未接 | 同一编排，交接汇总从不派生 | 核 |
| `ChargeOccurrenceForFailedAttempt` | 支路未接 | 入参 `AttemptObjectResult` 就在 `perform_offsite_pickup.go` 里造出来，隔几行没人再用。守的是 `AT-TF-094` | 核 |
| `OpenDispatchTask` | 支路未接 | 编排造 `NewDispatchTaskReference`，任务本体零调用点 | 核 |
| `FormLoadAssignment` | 支路未接 | 编排造 `NewLoadAssignmentReference`，分配本体零调用点 | 核 |
| `RecordMovementFact` | 支路未接 | 八例编排无一处理实际移动 | 核 |

两件要点：

**重复三次的形状是「引用造得出，本体造不出」**——生产代码里流转着指向从未被创建过的东西的
引用。CONTEXT 明写「装载分配分别拥有业务身份」，而它今天只有引用没有身份。

**最重的是前两条合起来。** CONTEXT 这句是实际履约段成立的定义性边界：

> 载运对象通过有效收寄或权威交接进入运输方控制时，其履约参与关系和适用实际履约段才成立。
> 扫描、订舱确认、承运接受、列入总单或舱单、车辆到场、装载分配和物理装载中的任一单项均不能
> 替代该边界。

两条成立入口都无生产调用方：收寄登记得进去、交接登记得进去，而**那条 CONTEXT 称为边界的
边界，生产路径上跨不过去**。实际履约段、履约参与关系、实际承运商今天在真进程上一个都形成
不了。

TF 记为**达标**且差量列写**「无」**。按本轮取证，「差量：无」与名单上这七条对不上，二者必有
一句要改——改哪一句是产品判断，不在本目录范围内。

### settlement-accounting（4）与 customs-compliance（4）

八条**全部只出现在测试里**，无一有生产调用点。

| 条目 | 分类 | 依据 | 取证 |
|---|---|---|---|
| `FormSupplierExpectedCost` | 支路未接 | 适配器测试与应用测试都在用它，**正文都不用**。基线称它与 `DecideDisclosure` 是仅有的两个「已走出 domain 层」的，并预言「真烂会先烂在这两条上」——今日仍成立 | 核 |
| `FormAuditedPayable` | 支路未接 | 仅 `supplier_bill_test.go` | 核 |
| `FormSupplierCreditNote` | 支路未接 | 仅 `supplier_bill_test.go` | 核 |
| `IncludeAdjustmentInSubsequentPeriod` | 支路未接 | 仅 `customer_statement_test.go` | 核 |
| `RegisterCredential` | 支路未接 | 仅 `compliance_judgment_test.go` | 核 |
| `VerifyDutyPayment` | 支路未接 | 仅 `duty_release_test.go` | 核 |
| `ReceiveReleaseOutcome` | 支路未接 | 仅 `duty_release_test.go` | 核 |
| `FormDutyCollaboration` | 支路未接 | 仅 `duty_collaboration_test.go` | 核 |

CC 与 SA 均记为**达标**，且分别称「编排九例，十二个 UC 全部有对应机制触点」「编排九例对应
七个 UC 全触」。同上：**UC 有触点** 与 **规则有执行器** 不是同一个断言。

### parcel-pricing（4）

**四条已逐条核完**（2026-09-02）。**基线对这一组的注释已经过期**，逐条结论与它不同：

| 条目 | 分类 | 依据 | 取证 |
|---|---|---|---|
| `ReplayPricingEvaluation` | 支路未接 | 只在领域与契约测试里。重放是 PN-08 的治理能力，生产无入口 | 核 |
| `MarshalPricingPlanSnapshot` | **平行第二写法** | 见下 | 核 |
| `RehydratePricingPlanSnapshot` | **平行第二写法** | 见下 | 核 |
| `ParseCanonical` | 守卫未接到它自称的边界 | 见下 | 核 |

**基线注今天不成立。** 它写着这两个快照函数是「同一条未接线路径的两端：适配器
`price_card_catalog.go` 正文没用它们，只有它的测试用了」，并预言「一次修好会同时去掉两行」。
实测：`price_card_catalog.go` **正文确实在存取快照**，走的是
`MarshalPriceCardRegistration` / `RehydratePriceCardRegistration`，而那一对内部直接用未导出的
`pricingPlanDocumentOf` / `pricingPlanFrom`。**路径已经接上了，而这两行没有跟着消失**——
因为生产是在**登记**这一层持久化的，方案层那对导出函数成了平行的第二个公开写法。预言没兑现，
不是因为还没修，是因为修的时候绕过了它们。

**`ParseCanonical` 不只是死码。** 它的注释写着「用在序列化边界上——那里不允许同一个数的
不同写法产生不同的内容摘要」，而序列化边界 `decimalFrom` 是这样重建的：

    func decimalFrom(snapshot decimalSnapshot) Decimal {
        return Decimal{coefficient: snapshot.Coefficient, scale: snapshot.Scale}
    }

**直接按字段构造，不解析也不校验。** 这未必是缺陷——`evaluation_snapshot.go` 头注称重建后
必过 `evaluation.valid()` 的整图重验（含语义摘要自校），那道后置门可能拦得住非规范写法。
但**守卫与它自称的用处对不上**，两条路二选一：接上，或者改注释别再说它用在序列化边界上。

### 顺带撞见的：影子函数复发了，而棘轮这次看不见

基线记过一次事故——初版名单里的 `DecimalFromInt64` 与 `Evaluate` 是「一行转发给同文件里
正主」的影子函数，已删，并留话说「删掉之后**不会再有任何机制提醒下一个人这里曾有过一对
影子函数**；它俩当初能长出来，正是因为同文件里已有正主却没有东西拦住第二个写法」。

**同一个文件 `domain/decimal.go` 里今天有 `NewDecimal`**：

    func NewDecimal(raw string) (Decimal, error) {
        return ParseDecimal(raw)
    }

全仓 `\bNewDecimal\b` 只命中一处——它自己的声明。零调用点，连测试都没有。

**而这一次棘轮抓不到它**：旧那对叫 `DecimalFromInt64`（非 `New` 开头，在网内，所以被看见并
删掉了），这一个叫 `NewDecimal`（被 `New*` 排除规则挡在网外）。同一种缺陷，命名方向相反，
门禁只守得住一个方向。这是[票 04](issues/04-should-the-ratchet-cover-the-new-family.md)
那条论证的实例，不是设想。

## 结论

32 条**没有一条**能归到「实例半边等租户」——那一格解释不了缺调用点。绝大多数是**支路未接**：
用例的上游有编排，这一步没有调用方，规则实现了但触发不到。这正是骨架完整判据禁止的那件事，
判据原话是「不靠『尚未实现』分支蒙混过关」——而**一条没有调用方的规则比一个 `TODO` 更隐蔽**，
因为它在每一道现有门禁下都是绿的。

**这不等于产品就绪结论必须推翻。** 可能的处置有几种，都要人裁：把这批认可为显式留待（像
r27 对另五项做的那样，但要逐条写理由）、下调某几个切片的定级、或者裁定骨架完整判据的口径
就是「UC 有触点」而非「每条规则有执行器」——**若取第三条，判据正文要改**，因为它现在写的
是后者。

## 边界

- **本目录不改任何生产代码、不接任何线、不改基线名单、不动定级。** 只出分类与依据。
- **不擅自改开发主线的状态列。** 定级是人的决定，本仓已有明文（r27：「宣布本身是用户的决定」）。
- 探针是扔弃件，`.scratch` 下留着当取证过程，**不进 CI**；要不要把 `New*` 一族并进棘轮见
  [票 04](issues/04-should-the-ratchet-cover-the-new-family.md)。
- **本文覆盖的 32 条已全部逐条核完，表内不再有标「组」的行。** TF 七条与 PP 四条原本按组推断，
  2026-09-02 补核：TF 无一翻案且比推断更重，**PP 四条与基线注释不同**——那条注今天已过期，
  详见 PP 一节。

- **但「32」是取证时点的数，不是当前值——名单是活的。** 同日稍晚 MCP-6 随票 13 新增两条
  （`internal/parcelshipment/domain SelectChannelCandidateByCost` 与
  `internal/parcelpricing/domain EvaluatePricingAcrossPlans`），改前 32 改后重数 **34**；
  MCP-5 随票 party-commercial-context-gaps/01 剪掉一条，因而**共享工作树上一度是 33、干净
  检出仍 34**，两个数都对，差的是未提交的那一剪。

  **本文不追这个数。** 追它就会变成基线文件头部反复警告的那种错法——「存下一个数，人人信它
  而没人重导」，那份文件自己已经为同一种错栽过两次。要当前值请重数
  `production_wiring_baseline.txt`，本文是 `9d6063c` 那一刻的一次快照，逐条依据不因新增条目
  而失效。

  新增那两条的成因已由 MCP-6 报明且**不属本文分类的任何一格**：它们不是存量支路未接，是当天
  新生且调用方已有名有姓（两者都等票 `label-channel-service-first-release/12` 的候选装配，
  票 12 落地那天一起出名单）。按 owner 的裁决，那正是新增条目理由行该有的写法。
