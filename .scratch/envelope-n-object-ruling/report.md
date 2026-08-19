# 「一封信 N 个载运对象」裁定备料（ENVELOPE-N-OBJECT-RULING-PREP）

取证基准：`73cd97d`（detached worktree，非共享树）。只读勘察，未改任何 `.go` / `.sql`，不裁决——本报告列证据与选项，裁决留给 TF/CC/VE owner 的裁定票。

背景：事实源勘察（`.scratch/ve-remaining-fact-sources/report.md`）把 TF 四条（`exception-journey.recorded`、`disposition-execution.recorded`、`regulatory-acceptance.recorded`、`transport-commission.submitted`）与 CC 两条（`declaration-submission.formed`、`customs-case.established`）判为「需裁定」，全卡在同一件事：按信封键重读回的本体带成员清单（`[]CarriedObjectReference` 或 `[]DeclaredParcelReference`），而现行 VE 投影链是一封信派生一个包裹的投影。这六条没有对象级替代品，不裁就永远到不了 VE。

## 结论先说

1. **「一封信一对象」不是用例或 ADR 的硬句，是两处消费者注释确立的实现纪律**；UC-VE-002 与 VE CONTEXT 反而只锚「一份投影一个包裹」，对「一封信触发几次派生」没有话。领域构造器把「一份投影装 N 个包裹」焊死了（`DeriveTrackingProjection` 逐条目校验 `entry.fact.parcel != parcel` 即拒绝），但没有任何一层挡「一封信循环派 N 条单包裹命令」。
2. **消费侧循环拆分机制上走得通，前提有一条硬约束**：成员维必须编进 `SourceFactReference`（前缀方案，`transport-handover/` 等五个前例同法）。不编进去必炸，且炸法是静默的——同键第二个成员撞 `FactSourceConflict`，`veconsume.Consumption` 把它当业务终局入账，后 N-1 个成员的投影无声丢失。
3. **六条事实不必同一个答案**。`regulatory-acceptance` 拒接格结构上零对象、`transport-commission` 有「提交≠取得控制」的里程碑资格坎，这两条即便技术上可拆，拆出来的东西该不该进投影是业务问题；TF 侧另有完整的「对象级登记」前例（`offsite-pickup.registered` 整条链），把拆分放回提供方也是有先例可抄的路。

---

## (a) 「一封信一对象」纪律写在哪

逐处引原文。检索范围：`veinbox`（`internal/visibilityexception/adapters/inbox`）与 `psinbox`（`internal/parcelshipment/adapters/inbox`）全部消费者注释、UC-VE-002 全文、VE CONTEXT 硬句、`docs/adr/` 全目录（按文件名过目 0001–0065 并全文检索「一封信/一对象/对象级」）。

**出处一：`psinbox` 的 `offsite_pickup_consumer.go`，`OffsitePickupRegisteredEventType` 常量注释**（表述最完整的一处）：

> 是本消费者认的事件类型：TF 的**对象级**揽收登记。不认尝试级的 `offsite-pickup.formed`——那一封信带一批成功对象，而采用判断逐对象成立，**一封信一对象才与消费门的入账/回滚两格对得上**。

**出处二：`veinbox` 的 `offsite_pickup_consumer.go`，同名常量注释**：

> 不认尝试级的 `offsite-pickup.formed`——那一封信带一批成功对象，而投影锚在包裹引用上逐对象成立；两条都登记会让同一份揽收被派生两次。

注意两处的理由并不相同：psinbox 说的是**消费门形状**（入账/回滚两格表达不了成员级部分结果），veinbox 说的是**双登记去重**（对象级 `.registered` 已覆盖同批事实）。后者对本票六条不适用——六条没有对象级替代品，不存在「派生两次」问题；前者才是六条真正要面对的形状问题。

**UC-VE-002**：全文没有「一封信一对象」或任何等价禁止句。与多对象相关的话见 (e)。

**VE CONTEXT（`docs/domain/visibility-exception/CONTEXT.md`）**：锚的是投影对象粒度，不是信封粒度——

> **全程追踪投影** 面向明确包裹及其身份谱系，对节点、运输、关务、路由和托运上下文已经接受的事实进行语义化编排后形成的只读旅程视图。

> 集运、装载或同批关系不合并包裹身份和客户轨迹。

**ADR**：`docs/adr/` 0001–0065 无任何一条陈述信封与对象的数量关系。ADR-0025（适配器在消费方侧）、ADR-0043（发布意图由结果标识认领）、ADR-0049（接不住不登记）约束的是翻译落位、意图幂等与路由登记，均不涉及一封信派生几条。

**领域层的真正硬门**：`vedomain.DeriveTrackingProjection` 的注释与实现——

> 全部条目必须属于同一包裹——旅程视图面向明确包裹，混入别人的事实就是把两条轨迹拼成一条。

实现逐条目校验 `entry.fact.parcel != parcel` 即返回 `ErrInvalidTrackingProjection`。也就是说「一份投影带全体成员」这条路在构造器上就立不住；被挡的是投影粒度，不是派生次数。

## (b) 现行六个消费适配器的形状，与 N 对象会破坏哪一环

### 现状表

六个消费者（`veinbox`）全部同构：`inboxconsume.Gate` 一封信一笔事务（类型校验 → 译码 → Start → 重复跳过/毒丸拒收/处理/MarkProcessed，处理失败整体回滚）；处理方（派生适配器）按信封键 `FindByKey` 重读提供方本体，构造**恰一条** `DeriveProjectionCommand`（内含单个 `Parcel`），交 `DeriveProjectionHandler.Handle`，结果经 `veconsume.Consumption` 译成入账（nil）或回滚（error）。

| 消费者 | 信封载荷（全部只带键，无成员清单） | 重读口 | 事实引用（SourceFactReference）构造 | 事实版本维 | 接线状态（73cd97d） |
|---|---|---|---|---|---|
| node-intake | tenantId + sourceId | `ReceptionStore.FindByKey` | 裸 `SourceID` | 收寄版本 | 已在路由表（经 fan-out 与 PS 采用并行） |
| offsite-pickup | tenantId + object + attempt | `OffsitePickupRegistry.FindByKey` | `offsite-pickup/<object>/<attempt>` | 揽收版本 | 已在路由表（fan-out） |
| effective-delivery | tenantId + object + attempt + version | `EffectiveDeliveryStore.FindByKeyAndVersion` | `effective-delivery/<object>/<attempt>` | 交付结果版本 | 已在路由表（fan-out） |
| transport-handover | tenantId + object + scope + version | `TransportHandoverRegistry.FindByKey` | `transport-handover/<object>/<scope>` | 交接结果版本 | 已在路由表 |
| initial-route | tenantId + customerAccountId + shipment + parcel + baseline + purpose | `InitialRouteStore.FindByKey` | `initial-route/<parcel>/<purpose>` | 接受基线 | **门与适配器已建，未登记进 `wireDispatcher` 路由表**（`cmd/` 下无 `InitialRouteFormedEventType` 引用） |
| final-outcome | tenantId + parcel + kind + version | `FinalOutcomeStore.FindByKey` | `final-outcome/<parcel>/<kind>` | 责任结果版本 | 已在路由表 |

投影编排（`veapplication.DeriveProjectionHandler.Handle`）的链：受理（`NewAcceptedSourceFact` 构造期拦）→ 幂等/冲突（`FactKey` 命中后比内容指纹）→ 事实落库（只增）→ `FindByParcel` 取该包裹全部事实逐条归类 → 派生新投影版本（有当前投影则 `Rederive` 指回）→ `Save` → `HandOffProjection`（失败留 `handoffRef` 续办引用）。

### 四环逐个过

**环一：事实幂等键（最危险，且是静默的）。** `ports.FactKey` =（租户 + 源上下文 + 事实引用 + 来源版本）；内容指纹 `factContentDigest` =（parcel + kind + occurredAt + effectiveAt）。若适配器对一封信循环 N 个成员而**事实引用不带成员维**（N 条命令共用同一引用同一版本），第一个成员落库后，第二个成员同键但指纹含不同 parcel → 指纹不等 → `FactSourceConflict`。而 `veconsume.Consumption` 把 `SOURCE_CONFLICT` 列为业务终局（连同 `PROJECTION_DERIVED`/`EXISTING_RESULT`/`NOT_ACCEPTED` 一起译 nil 入账）——注释原话「重试不会让映射目录或另一份源事实长出来，入账收工」。后果：**信封入账，后 N-1 个成员的投影永不派生，无错误、无重试、无日志分格**。所以「循环拆分」与「成员维进事实引用」是绑死的一对，不可拆开裁。顺带：UC-VE-002 `AT-VE-040`「来源身份携带不同对象范围 → 形成冲突并待确认」描述的正是这个冲突格本来要抓的场景——把 N 成员塞同一引用等于自己制造 AT-VE-040。

**环二：映射目录键（前缀方案不伤它，但有一条边界）。** 目录键经 `1709872`（`.scratch/ve-milestone-mapping-key/issues/01`，已 resolved）定为（租户 + 映射版本 + 源上下文 + **事实类型**），`rule_catalog.go` 注释：「条目按源上下文与事实类型建键，一行覆盖此后同类型事实，不按单条事实引用查目录」；`vedomain.SourceFactKind` 注释同句并加「**不得按单条事实引用建目录**」。因此成员维进**引用**对目录零影响；但若有人把成员维编进**类型**（例如每对象一个 kind），就重蹈 issue 01「结构上填不满的表」。拆分方案的第二条硬约束：成员只进引用，类型仍按事实语义取字面量（交接三格、初始路由两格、终局两格皆是先例——分格依据是**语义分支**，从不是对象身份）。

**环三：投影版本链（无结构冲突）。** 投影库按（租户 + 包裹）管当前版（`ProjectionStore.FindCurrent`/`Save`），历史靠 `Rederive` 的 `priorVersion` 指回。N 个成员各是各的包裹，各推各的链，互不触碰。`NextProjectionVersionID` 是全局签发，循环 N 次拿 N 个版本号，无冲突。下游意图按投影版本认领（ADR-0043），每成员一份，重放重发同一份，形状不变。

**环四：部分失败重试（能收敛，但消费门两格确实表达不了成员级部分终局）。** 一封信一笔事务：循环中任一成员命中 `DeriveUndecided`（事实库/映射视图/投影库调不通）或 `HandoffReference` 非空 → error → **整封回滚**（前面成员的落库一并回滚，无部分状态）→ 重投从头再跑，已无残留，幂等键上是全新保存；若重投时前面成员已在先前某次成功提交（不可能——回滚是整笔的）则走 `FactExistingResult` 短路，也收敛。真正的语义坑有两个：
- **头端阻塞**：某一个成员持续未决（如该成员引用坏、或其包裹身份迟迟不可见），整封信全部成员一起卡在重投循环里。对象级信封则各卡各的（`FanOut` 注释的既有口径：「两本 inbox 互不隶属，不能因为 A 停在实例墙就把 B 的入账也卡住；已提交的那路靠 inbox 在重投时跳过」——这说的是消费者间，成员间同理但现机制给不出）。
- **成员级冲突被吞**：环一修好（成员进引用）之后，单成员的 `SOURCE_CONFLICT`/`NOT_ACCEPTED` 仍译 nil，循环继续、整封入账。这对单对象信封是对的（冲突是业务终局，UC-VE-002 有冲突待确认格）；对 N 成员信封则意味着「9 个派生 1 个冲突」与「10 个全派生」在消费门上同一格。psinbox 那句「一封信一对象才与消费门的入账/回滚两格对得上」指的就是这个表达力缺口。冲突本身在事实库有行（`AT-VE-040` 格），不是完全不可见，但不在消费账上。

## (c) 可选形状：支持/反对证据

### 形状 A：消费侧循环——适配器按键重读本体，对 `Members()` 逐成员构造命令（成员维进事实引用）

支持证据：
- 机制全通：信封已带重读所需全维键（六条载荷逐个读过，见 (d) 表），成员从本体读回，**信封形状一字不动**，与事实源勘察「不塞快照」的红线一致。
- 前缀前例充分：五个现行适配器全部在消费侧构造带业务维的事实引用（`offsite-pickup/`、`effective-delivery/`、`transport-handover/`、`initial-route/`、`final-outcome/`），构造材料同样取自重读回的本体键维；成员维只是再加一段。
- 领域层不设障：`DeriveTrackingProjection` 挡的是一份投影混包裹，不挡一封信派 N 份单包裹投影；UC-VE-002 无禁止句且 `AT-VE-045`/`AT-VE-046` 本就预期一次来源触发多包裹分别投影（见 (e)）。
- 跨客户隔离天然保持：投影按包裹、客户视图按（租户+客户+包裹）各自派生，成员拆开后互不可见——正合 VE CONTEXT「受影响对象必须按货主客户账户和责任法人分区」（customs-case 成员自带 `Customer`，这条尤其顺）。
- 不需要 TF/CC 改任何代码。
- 事务原子性由消费门现成提供：整封全成或全滚，无部分状态。

反对证据：
- 正面违反 psinbox 注释确立的纪律及其理由（消费门两格表达不了成员级部分终局，见环四）。裁定若走此路，等于宣布该纪律只约束「有对象级替代品」的场合——两处注释需要改写口径，否则下一个读代码的人会拿旧句挡新路。
- 头端阻塞（环四）：一个成员卡住全信。
- `regulatory-acceptance` 拒接格 `accepted` 结构上为空（`FormRegulatoryTransportDisposition` 强制 DECLINED 不带对象），循环体零次——信封入账、零派生。「拒接」这件事本身进不了任何包裹的投影；若业务想让「监管处置被拒」在追踪上可见，这条路给不出，见 (d)。
- 一笔事务里 N 次 `FindByParcel` + N 次全量重归类 + N 个投影版本 + N 份意图,N 大时单事务变重（无证据表明当前有上限约束成员数；`AlternateJourney`/`TransportCommission`/`CustomsSubmissionVersion` 构造器只查非空无重）。

### 形状 B：提供方拆——TF/CC 增发对象级信封，VE 按单对象消费（照抄 `offsite-pickup.registered` 模式）

支持证据：
- TF 已有完整前例可抄：尝试级 `offsite-pickup.formed` 与对象级 `offsite-pickup.registered` 并存，后者有自己的用例（`RegisterOffsitePickup`）、登记仓储（`OffsitePickupRegistry`）、出账口（`OutboxOffsitePickupRegistrationHandoff`）；VE 与 PS 都只认对象级那条。
- TF 的领域语言本来就是对象级的（CONTEXT 硬句）：「权威运输交接**逐载运对象**判断并允许部分成立」「整批、整车、整袋和整单结论只能由对象级结果派生」「监管运输处置**按载运对象和运输动作逐项承接**并允许部分结果……不得以整批结果覆盖未承接对象」「一个任务和一次尝试可以覆盖多个载运对象，但每个对象必须分别保存……结果」。旅程/委托/承接聚合带成员清单是存储形状，不是领域粒度的表态。
- 消费门两格语义干净：每对象独立入账/回滚/重试，无头端阻塞，无成员级冲突被吞。
- 保住「一封信一对象」纪律原句不动。

反对证据：
- 要 TF/CC owner 立新事件类型、新出账口（可能还要新登记仓储或至少新意图口）——不是 VE 单方面能开工的票；四条 TF + 两条 CC 各一套，工作量与评审面数倍于形状 A。
- 语义发明风险在 CC 侧更大：`customs-case.established` 是「案件建立」一件事，拆成 N 封「案件成员关联」信要 CC 承认「成员关联」是独立可发布事实；`declaration-submission.formed` 同理（「提交版本固定」是一件事）。TF 侧的旅程成员还能挂到「对象进入新旅程」这类语义，CC 侧的拆分语义要 owner 先答「这算不算我拥有的事实」。
- `regulatory-acceptance` 拒接格问题在此形状下同样存在：零对象拆不出对象级信封。
- 时间成本：六条全走提供方拆，VE 侧这批事实到位时间取决于三个上下文的排期。

### 形状 C：拒接（这六类不进 VE 投影）

支持证据：
- 纪律与现状零改动。
- `transport-commission.submitted` 有独立的资格疑问（见 (d)）——不接它未必是损失。

反对证据：
- 事实源勘察已判：这六条**没有对象级替代品**，拒接等于这些事实永远到不了 VE。TF 中间四条覆盖的是异常旅程、监管处置、退运、委外——恰是「全程追踪与异常」上下文的核心可见性素材；UC-VE-002 边界句「本用例从各源上下文提交已经接受、范围明确且可追溯的事实……开始」并没有把多成员事实排除在「范围明确」之外。
- VE CONTEXT 消费面把 TF 列为「场外揽收、班次、运输委托、订舱、容量、装载分配、权威交接、实际移动、实际承运商、交付结果和交付证明」的提供方——运输委托明文在列。

### 混合裁法（证据指向，非裁决）

六条不必同答案。区分两个正交问题可以让裁定票小很多：**①这条事实该不该进投影**（业务问题，逐条答）；**②该进的用哪个形状拆**（机制问题，一次答）。①的证据见 (d)——`transport-commission` 与 `regulatory-acceptance` 拒接格有独立的业务坎，其余四条（旅程两条、申报提交、案件建立）没有发现类似障碍。

## (d) 六条事实的成员清单语义差异

| 事实 | 聚合与成员 | 成员类型 | 成员语义 | 空清单可能性 | 特有问题 |
|---|---|---|---|---|---|
| `exception-journey.recorded` | `AlternateJourney.Members()` | `[]CarriedObjectReference`（非空无重，构造器强制） | 进入新旅程（备用切换/改送/退运）的载运对象 | 不可能为空 | 与 disposition-execution 共用聚合、同 store 同键（`AlternateJourneyKey` = 租户+原旅程+目的+处置依据），仅信封 ID 类型段不同（`/exception-journey` vs `/disposition-execution`）。监管来路一次启动**两口同事务入队**——若两条都接，同一批成员会各派生一次，事实引用必须把两条错开（kind 已天然不同） |
| `disposition-execution.recorded` | 同上 | 同上 | 同上（仅监管来路，`RegulatoryOrigin` 为真的旅程走此口） | 同上 | 同上；此口消费方本意是 CC 处置执行核对（`DispositionExecutionHandoff` 端口注释），VE 若也接，语义是「处置已执行」而非「旅程已启动」 |
| `regulatory-acceptance.recorded` | `RegulatoryTransportDisposition.AcceptedObjects()` | `[]CarriedObjectReference` | **已承接**的对象（ACCEPTED/PARTIALLY_ACCEPTED 格） | **DECLINED 格结构上恒空**（构造器强制拒接不带对象）；且 PARTIALLY_ACCEPTED 的**未承接对象连清单都没有**——只有 `DeclineBasis` 一个字符串 | 逐对象拆只能拆出「已承接」半边；「被拒」与「部分承接中被拒的那部分」在这份聚合上没有对象可锚。TF CONTEXT 说「不得以整批结果覆盖未承接对象」，但聚合只给了拒因文本，不给未承接对象集 |
| `transport-commission.submitted` | `TransportCommission.Members()` | `[]CarriedObjectReference`（非空无重） | 委托范围内的载运对象 | 不可能为空 | **提交≠取得控制**在类型上就有对照：`SubmittedAt` 是提交时刻，`MarkTransportStarted` 另以「首个控制事实（有效收寄或权威交接）」登记开始，且 TF CONTEXT 明句「运输委托、订舱、承运接受和装载分配表达执行准备，也不能提前制造实际履约段」、CarrierAcceptance 注释「面单生成、渠道受理、预报成功、订舱接受……均不构成实际承运商首次有效收寄」。把提交映成任何「运输中/已交承运」类里程碑都与源语义冲突；它顶多是「已委外」这类准备性里程碑——这不是 VE 单方能定的映射语义 |
| `declaration-submission.formed` | `CustomsSubmissionVersion.Members()` | `[]DeclaredParcelReference`（快照固定，非空无重） | 提交版本固定那一刻申报单元的组成包裹 | 不可能为空 | 成员是**包裹级引用**（CC 侧无集运混入问题，与 TF 的 CarriedObject 不同）。版本语义清晰（重报换 `SubmissionVersionID`）。附带观察见下 |
| `customs-case.established` | `CustomsCase.Parcels()` | `[]CaseParcelAssociation`（非空、按包裹无重、每项 Parcel+Customer+SourceRef 完整） | 案件建立时关联的包裹，**每项自带客户归属** | 不可能为空 | CC 里唯一直接带客户维的成员清单，跨客户隔离素材现成。坎在事实语义：聚合注释明说案件「不是『清关中』状态也不是可提交的申报」——它是责任容器的建立，映成哪个客户可见里程碑（或者根本不该客户可见、只进内部维度）是映射目录的事；另「一个包裹可以关联多个彼此独立的案件」，同包裹会收到多条 case 事实，事实引用需含案件维（案件键或 CaseID）才不相互冲突 |

成员类型上的一条贯穿差异：TF 四条的成员是 `CarriedObjectReference`——领域注释「对正式包裹身份**或集运单元**的引用……本上下文不铸造它们」，集运单元混入的可能性与现行三路投影相同（事实源勘察已确认现行标准是「拿得出 CarriedObjectReference 即算数」，`derive_on_offsite_pickup.go` 注释明写「集运单元不猜、不跳过」）；CC 两条的成员是包裹级（`DeclaredParcelReference` / `CaseParcelAssociation.Parcel`），没有这层含混。

**附带观察（不在票面五问内，裁定时最好知道）**：`declaration-submission.formed` 的信封 ID 由 `declarationSubmissionEventID` 构造 =（租户/单元/程序），**不含版本维**；版本只在载荷（`versionId`）与 Subject 里。同一（单元+程序）重报换版本时（ADR-0045 撤销重报换新版本），新版本的出账信封与旧版本同 ID，`outboxintent.EnqueueOnce` 会把它当同一封信去重——若属实，重报版本永远发不出第二封。本条是**推断**（`EnqueueOnce` 的去重语义按其名字与 ADR-0043「意图由结果标识认领」推断，未打开 `outboxintent` 实现核对），真要接这条前应单独核实；它同时影响「按版本拆事实」的可行性评估。

## (e) UC-VE-002 对一次派生多包裹的原文

全文检索 `UC-VE-002-BUILD-TRACKING-PROJECTION.md`，与「一次来源、多个包裹」相关的原文全部列此：

- 规则节：「**集运、同批或共同来源不合并包裹身份；拆分/合并沿 `parcel-shipment` 谱系继续投影。**」
- 规则节：「**跨客户投影按客户账户和责任法人隔离；共同来源事故不泄露其他客户对象或数量。**」
- `AT-VE-045`：「包裹真实拆分 → **后继包裹分别投影**，共同历史只保留谱系」
- `AT-VE-046`：「跨客户共同集运 → **物理来源可共同存在，客户投影隔离**」
- 输入定义：「源事实 | 来源身份、**对象范围**、事实类型、来源版本、接受结果和业务时间；只消费已接受事实」——「对象范围」这个词本身不限定单包裹。
- `AT-VE-040`：「来源身份携带不同对象范围 → 形成冲突并待确认」——这条约束的是**同一来源身份**（同一事实引用）不得携带不同对象范围，恰是环一「成员必须进引用」的用例层对应物。

没有任何句子禁止一次来源触发派生多个包裹的投影；「共同来源、多包裹、分别投影、按客户隔离」反而是用例明文预期的场景。用例层真正的锚是：每份投影面向一个明确包裹，共同来源不得把包裹并成一条轨迹。

另有一条对 `transport-commission` 资格问题相关的边界句：「本用例……到 `visibility-exception` 形成标准追踪里程碑……或明确返回重复、冲突、未归类、待补充或未形成结果为止」加上映射目录「无法可靠映射则形成未归类」——即便委托提交进了事实库，映不映成里程碑仍由版本化目录决定，`未归类` 是合法稳态。这削弱「资格疑问」作为**拒接**理由的分量（事实进来但如实未归类也是一条路），但把包袱转给了映射目录的实例登记。

## 覆盖声明

**逐条读过代码/文档取证的（打开文件读到字段、方法或原句本身）：**

- `veinbox` 六个消费者文件全读（node_intake / offsite_pickup / effective_delivery / transport_handover / initial_route / final_outcome）；`psinbox` 的 offsite_pickup_consumer 读到常量注释段。
- 六个派生适配器全读（`venodeops.DeriveOnNodeIntakeAdapter`、`vetf` 三个、`venr.DeriveOnInitialRouteAdapter`、`veps.DeriveOnFinalOutcomeAdapter`），含各自 projectionCommand 的事实引用构造行。
- 派生编排 `derive_projection.go` 全读（FactKey、内容指纹、冲突/重放分支、FindByParcel 重归类、Rederive、HandoffReference）；`veconsume.Consumption` 全读；`inboxconsume.Gate.Consume` 全读；`dispatch.FanOut` 全读。
- `vedomain` 的 `tracking_projection.go` 全读（SourceFactKind 注释、AcceptedSourceFact、DeriveTrackingProjection 同包裹校验、Rederive）。
- VE `ports.go` 全读（FactKey、AcceptedFactStore、MilestoneMappingView、ProjectionStore、ProjectionHandoff）。
- `rule_catalog.go` 的 ClassifyFact 读到两级查找与键注释；`.scratch/ve-milestone-mapping-key/issues/01` 全读（含 Completion 的映射键四维）。
- TF 领域：`alternate_journey.go`、`regulatory_disposition.go`、`transport_commission.go`（含 BookingRequest/CarrierAcceptance 段）全读；`CarriedObjectReference` 注释读到原句。
- CC 领域：`declaration_submission.go`（DeclarationUnit / CustomsSubmissionVersion / SubmissionAttempt）、`customs_case.go` 全读。
- 六条事实的 outbox handoff 全读（exception_journey / disposition_execution / regulatory_acceptance / transport_commission / declaration_submission / customs_case），载荷结构体与信封 ID 构造逐个到行。
- TF/CC `ports.go` 中五个键与 store 接口段（AlternateJourneyKey/Store、DispositionAcceptanceKey/Store、TransportCommissionKey/Store、DeclarationSubmissionKey/Store/Record、CustomsCaseKey/Store）grep 到定义原文。
- UC-VE-002 全文；VE CONTEXT 与 TF CONTEXT 按关键词（集运/合并/隔离/对象级/载运对象）取到的全部命中段；ADR-0025、ADR-0043 全文。
- `wireDispatcher` 的路由表段与三个 FanOut 装配段；`cmd/` 全目录 grep 确认 initial-route 未登记。

**按同类归并推断、没有逐条打开的（推断不是取证）：**

- `outboxintent.EnqueueOnce` 的去重语义（信封 ID 幂等）未打开实现核对——declaration-submission 信封 ID 不含版本维的后果因此标为推断。
- ADR 目录对「一封信一对象」的沉默按文件名过目 + 全 `docs/adr` 关键词检索（一封信/一对象/对象级/每对象）判定，65 份 ADR 正文只逐读了 0025 与 0043 两份。
- 六条事实在 `wireDispatcher` 均未登记（撞 `dispatch.no_subscriber`）取自事实源勘察报告与本次路由表读回的交叉印证，未逐条验证六条的 outbox handoff 是否已在各自应用编排装上（事实源勘察记录 `OutboxInitialRouteHandoff`/`OutboxFinalOutcomeHandoff` 已装配入队，本票六条的装配状态未查）。
- CC `DeclarationSubmissionStore` 同键重报（新版本）的写入代数（Save 撞键还是 Replace）未读 postgres 实现——(d) 表 declaration-submission 行的版本语义以领域类型与 ADR-0045 为据。
- `inboxconsume` 内 store 写入与处理方共享同一事务上下文（txCtx 贯穿）取自 `Gate.Consume` 代码与包注释「处理失败整体回滚」，未向下核对 Bento `Transactor`/`inbox.Store` 的实现。
- N 成员单事务的性能形状（环四末条）是结构推断，无任何压测证据。
- 完全没有扫：本票六条之外其余「不可接/可接」事实的结论一律沿用事实源勘察报告，本次未复核。
