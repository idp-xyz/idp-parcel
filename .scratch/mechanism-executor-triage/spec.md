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
| `EstablishSegmentWithPickup` | 支路未接 | 编排 `perform_offsite_pickup.go` 造了 `FulfillmentAttempt`（`FormFulfillmentAttempt` / `FormAttemptObjectResult`）却**不由揽收事实立履约段** | 核 |
| `EstablishSegmentWithHandover` | 支路未接 | 同族，交接那一端 | 组 |
| `ChargeOccurrenceForFailedAttempt` | 支路未接 | 失败尝试的收费发生项 | 组 |
| `FormLoadAssignment` | 支路未接 | 装载分配 | 组 |
| `OpenDispatchTask` | 支路未接 | 派送任务开启；`NewDispatchTaskReference` 被编排用着，任务本体没有 | 组 |
| `RecordMovementFact` | 支路未接 | 实际移动事实 | 组 |
| `SummarizeHandovers` | 支路未接 | 交接汇总 | 组 |

TF 记为**达标**（非「留待」），且开发主线称「TF 编排八例对应七个 UC 全触」。这 7 条是 32 条里
最大的一簇，**这一组最该优先逐条核**——若「全触」指的是每个 UC 有触点而非每条规则有执行器，
那两句话说的不是一件事。

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

| 条目 | 分类 | 依据 | 取证 |
|---|---|---|---|
| `ReplayPricingEvaluation` | 支路未接 | 重放入口无生产调用方 | 组 |
| `MarshalPricingPlanSnapshot` | 支路未接 | 基线称适配器 `price_card_catalog.go` 正文没用、只有测试用 | 组 |
| `RehydratePricingPlanSnapshot` | 支路未接 | 同上，同一条路径两端 | 组 |
| `ParseCanonical` | 待定 | 基线称是值解析助手，「留两行比写排除规则便宜」 | 组 |

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
- **「组」标的 12 条尚未逐条核。** 按本仓「写证据不写结论」的纪律，那 12 行是推断不是取证，
  引用时请照此读；TF 那 7 条最该先补。
