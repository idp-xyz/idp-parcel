# 第二十五轮端口盘点

Category: chore
Status: needs-triage

只读清点，无代码改动，无既有文档改动。承 MCP-1 派包。**盘于 `d40b03a`**——共享工作树在我开盘时已被推进到该提交
（`195cfe4` → `98ce0ee` → `d40b03a`），因此扫描到的文件含 ADR-0050 那两笔；差式表里 `d40b03a` 关掉的两口即为佐证。
盘点期间树上另有他会话的未提交内容（`.cursor/mcp.json`、`docs/review/`），均未纳入本盘点也未被改动。
基线为第二十四轮，盘于 `4c928b0`，记载在
[首发开发主线](../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)
的「横切缺口」一节：「十个 `ports.go` 共声明 199 个接口……65 个至今没有任何生产实现」。

## 计数口径（下一轮复盘要对着这一节）

两侧数字都由同一个程序在两棵树上重算，不取任何历史记载的数值。程序做三件事：

1. `go/ast` 解析 `internal/*/ports/*.go`，取 `type X interface` 的名字与方法集，同包嵌入接口展开。
2. 收集适配器生产文件：`internal/**` 下路径含 `\adapters\` 或 `\platform\` 且不以 `_test.go` 结尾者。
3. **判据 A（基线口径）**：端口名在上述文件的正文里出现过即记「有生产实现」。

判据 A 就是基线那句「按适配器源文件里出现过的端口名清点」。它是本报告的**头条数字**，因为只有它可比。
其余判据只作交叉核对，不改头条。

**口径偏差要先说**：用判据 A 重算 `4c928b0`，接口总数 **199 精确对上**基线，缺口我算 **62**，基线记 **65**，差 3。
把扫描范围收窄到只有 `adapters`（去掉 `platform`）仍是 62，因此差异不来自范围。我未能还原出那 3 个的来历，
推测是基线当时按人工清点或另有细则。**结论：绝对数与基线记载差 3，但两侧同程序算出的差式可比。**

## 一、总数与缺口（事实）

| 项 | 4c928b0（基线） | d40b03a（本轮） |
|---|---|---|
| `ports.go` 接口总数 | 199 | **200** |
| 判据 A 无生产实现 | 62（基线记 65） | **13** |
| 扫描到的适配器生产文件 | 129 | 166 |

接口总数 +1：`partycommercial.AuthorityGrantStore`，`6d6397f` 新增。

### 逐上下文

| 上下文 | 接口总数 | 基线缺口 | 本轮缺口 |
|---|---:|---:|---:|
| customscompliance | 30 | 11 | 0 |
| networkrouting | 14 | 7 | 3 |
| nodeoperations | 12 | 2 | 1 |
| parcelpricing | 3 | 0 | 0 |
| parcelshipment | 33 | 13 | 4 |
| partycommercial | 7 | 1 | 0 |
| pilotgovernance | 8 | 0 | 0 |
| settlementaccounting | 36 | 9 | 4 |
| transportfulfillment | 24 | 5 | 0 |
| visibilityexception | 33 | 14 | 1 |
| **合计** | **200** | **62** | **13** |

## 二、差式：49 口从无到有，0 口新增缺口（事实）

SHA 取「该端口名首次出现在 `internal/*/adapters/**` 的提交」（`git log --reverse -S<名> 4c928b0..HEAD`）。

| SHA | 口数 | 端口 |
|---|---:|---|
| `32cc780` | 12 | CC `CaseIdentityFactory`、`CaseRequirementView`、`DeclarationVersionFactory`、`ExecutionFactView`、`GateConditionView`、`InterpretationRuleView`、`ManifestCandidateView`、`ObligationInventoryView`、`ReadinessView`、`SubmissionAuthorityView`、`SubmissionIndex`；PC `CommercialAuthorityView` |
| `c45d0d7` | 10 | VE `ActiveCaseView`、`CustomerViewIdentityFactory`、`DispositionRequestIdentityFactory`、`ETAIdentityFactory`、`MilestoneMappingView`、`NotificationIdentityFactory`、`ProjectionIdentityFactory`、`RecoveryIdentityFactory`、`SignalEpisodeIdentityFactory`、`TriageRuleView` |
| `a5095ae` | 7 | NR `RouteHandoffLog`、`RouteIdentityFactory`；TF `DeliveryAttemptView`、`DeliveryIdentityFactory`、`OffsitePickupRegistry`、`PickupIdentityFactory`、`TransportHandoverRegistry` |
| `91af99c` | 6 | NO `IntakeIdentityFactory`；SA `AllocationRuleView`、`ConfirmationConditionView`、`CreditStandingView`、`ExpectedCostView`、`OperationalBalanceView` |
| `1b113cb` | 5 | PS `AcceptanceDecisionIdentity`、`CommitmentIdentityFactory`、`FinalIdentityFactory`、`SourceDataVersionIdentity`、`SubmissionIdentityFactory` |
| `8a1a6ff` | 2 | PS `AcceptanceJudgmentRecorder`、`RecordedJudgmentReader` |
| `d40b03a` | 2 | NR `CommercialEligibilityView`、`RoutingApplicabilityView` |
| `5a1eded` | 1 | VE `EligibilityRuleView` |
| `15465fb` | 1 | VE `NotificationPolicyView` |
| `ac34c69` | 1 | VE `DisclosurePolicyView` |
| `6d6397f` | 1 | PS `ActiveRejectionAuthorizer` |
| `195cfe4` | 1 | PS `CancellationAuthorityView` |

**新增缺口 0**：本轮唯一新声明的接口 `AuthorityGrantStore` 同笔带了 PostgreSQL 实现。

## 三、剩余 13 口的分类与卡点（分类与卡点是判断，依据逐条给出端口注释原话）

| 上下文 | 端口 | 类 | 卡点 | 依据（端口注释原话摘要） |
|---|---|---|---|---|
| NR | `NetworkEvidenceView` | 规则/政策/权威视图 | 提供方表面缺 | 「为一次判断取回版本化网络事实」——服务区、路由要求、可执行性、硬约束四类事实无人生产 |
| NR | `InitialRouteEvidenceView` | 同上 | 提供方表面缺 | 「为一次初始路由判断取回版本化事实」 |
| NR | `AutoRerouteFactsView` | 同上 | 实例半边 | 「第二个返回值为 false 即『事实目录未配置』——不猜」 |
| NO | `ParcelIdentityView` | 规则/政策/权威视图 | 提供方表面缺 | 「用正式包裹与外部标识关联核对身份（PS 侧只读引用）」——PS 侧未开该读口 |
| PS | `ProductionOwnershipAuthority` | 授权 | 实例半边 | 「试点准入控制……只消费该决定，绝不自行推导」；真实归属属 PN-08 `Go` |
| PS | `WithdrawalAuthorizer` | 授权 | 实例半边 | 「真实撤回授权角色与原因语义仍是 `PAR-COM-14` 待提供」，且明禁把无规则答成`不允许` |
| PS | `SourceDataAmendmentAuthorizer` | 授权 | 建模未决 | 「真实请求方、实际决定方与授权入口仍是 `BD-PS-009` 待确认」 |
| PS | `SourceDataRuleDeclaration` | 规则/政策/权威视图 | 实例半边 | 「由 `PAR-COM-13` 与真实合同、产品、线路和关务规则登记，属实例半边」 |
| SA | `PreAcceptanceControlPolicyView` | 规则/政策/权威视图 | 提供方表面缺 | 「本上下文只消费它，绝不自行推导」；生命周期判给 PC，PC 侧无对应发布口 |
| SA | `ContractResponsibilityView` | 规则/政策/权威视图 | 实例半边 | 「合同责任目录未配置……未配置停在未决，不默认可回收」 |
| SA | `ClaimAmountRuleView` | 规则/政策/权威视图 | 实例半边 | 「没有规则版本不形成金额（实例半边，AT-SA-147）」 |
| SA | `SupplierAuditAuthorityView` | 授权 | 实例半边 | 「授权未配置——实例半边未提供时审核停在未决」 |
| VE | `NotificationChannelGateway` | 其他（系统外通道） | 实例半边 | 「真实渠道与其凭证属实例参数，今天没有实现，唯一实现是测试替身」 |

按类合计：规则/政策/权威视图 **8**、授权 **4**、其他 **1**、标识签发 **0**。
按卡点合计：实例半边 **8**、提供方表面缺 **4**、建模未决 **1**、可直接做 **0**。

**标识签发这一类已清零**（`1b113cb`、`91af99c`、`a5095ae`、`c45d0d7`、`32cc780` 五笔合计关掉全部签发口），
基线「集中在规则/政策/权威视图、标识签发与授权三类」的三类之一至此不再是缺口。

## 四、判据 A 已知虚高（这一节是推断，但两个反例可复算）

判据 A 只看「名字出现过」，因此**把被适配器当依赖引用的端口也算成已实现**。两个已核实的反例：

**一、`Clock`：十个上下文各声明一个，全仓零生产实现。** 适配器里出现的是 `clock ports.Clock` 这样的**字段与构造参数**
（如 `internal/parcelshipment/adapters/postgres/*_handoff.go` 五处），不是实现。全仓搜 `Now() time.Time` 的方法定义，
非测试文件里一处都没有；`internal/platform/dispatch/dispatcher.go` 里那处也是 `type Clock interface` 声明。
真实时钟今天由测试替身与（未接线的）组合根承担。

**二、PC 的两个声明口被消费但无人生产。** `AsOfPolicyDeclaration` 与 `AcceptanceContentDeclaration` 在判据 A 下算「已实现」，
因为 PS 侧的 `CommercialBasisAdapter` 引用了它们；但 `LoadAsOfPolicies`、`LoadAcceptanceRuleContent`、
`LoadPendingRoutingPermission` 三个方法的定义只存在于测试替身（`judgment_as_of_test.go`、`commercial_basis_test.go`、
`form_judgment_as_of_test.go`）。这正是「提供方表面缺」的典型形状，而判据 A 看不见它。

**换判据的数**（仅供参照，不作头条，因为与基线不可比）：

| 判据 | 本轮无实现 | 说明 |
|---|---:|---|
| A：名字在适配器源文件出现过（基线口径） | 13 | 头条 |
| B：有具体类型的方法集覆盖该接口全部方法 | 25 | = 13 + `Clock`×10 + PC 两个声明口 |
| C：有 `var _ ports.X = (*T)(nil)` 断言 | 103 | 断言不是纪律，大量已实现端口没写，**不可用** |

判据 B 的 25 更接近「真的没有生产实现」，但它自身也不可靠：**单方法接口会撞名误判**。
程序把 `CustomsCaseStore` 判给了 `postgres.DeclarationSubmissions`、把 `InitialRouteStore` 判给了 `postgres.FundsMappings`
——只因两者都有同名方法。要精确只能上 `go/types` 做真正的 `types.Implements`，本轮没做。
**因此 25 是下界性质的参考值，不是结论。**

## 五、派发装配与单一消费者（事实）

**派发组合根仍未接线，但缺口从四个依赖收敛到一个。** 基线记「缺的是把 Claimer / Finalizer / Publisher / Clock
接到真实 Outbox 与发布通道」；现在 `cmd/parcel-dispatch/assemble.go` 的注释写明四个里已查明三个有真实来源
（Claimer 与 Finalizer 是同一个对象，即框架的 `postgres/outbox.Store`，构造链已在真库门禁跑过），
**只剩 `eventing.Publisher` 没有生产实现**，且框架按合同不拥有它。`assembleDispatcher()` 仍交回该错误，`loop_test.go` 钉着这一点。

**这里要更正本报告初稿的一处错**：初稿照抄 `assemble.go` 的注释写成「发布通道形态未定，提案见 ADR-0049」。
实际上 [ADR-0049](../../docs/adr/0049-publish-channel-is-in-process-delivery-until-load-evidence.md) 的
`Status: Accepted`（2026-08-14），已经裁定「首发的发布通道是进程内直投」，并且明确否决了
「让 `assembleDispatcher` 继续交回未决，等第一个真实租户再定」这一选项，理由是发布通道形态属机制半边。
因此这一处不是「等决定」，是**已决定而未实现**；`errDispatcherNotWired` 的错误串
`publish channel is undecided` 与 `assemble.go` 那句「本仓至今没定过发布通道形态」都已与 ADR 冲突，属过期注释。

**消费侧仍只有一个消费者。** 全仓非测试文件里引用 `inbox.Store` 的只有
`internal/networkrouting/adapters/inbox/acceptance_consumer.go` 的 `AcceptanceConsumer`，与基线记载一致，无变化。

**HTTP 装配缝无变化。** `cmd/parcel-api/endpoints.go` 的 `assembleBusinessEndpoints()` 仍 `return nil`，
注释理由仍是 Intake 认证方式属 `PAR-INT-01` 待提供。

## 六、本轮没做的事

- 没改任何代码、没改任何既有文档、没提方案（按派包红线，盘完由 MCP-1 定）。
- 没做 `go/types` 级的精确实现判定（见第四节），因此判据 B 的 25 只是参考。
- 没有还原基线 65 与本次重算 62 之间那 3 个的来历。
- 没有判断剩余 13 口里哪些「该现在做」——那是排期，不是清点。
- 本报告不入库（`.scratch` 本轮不提交），留待 MCP-1 与基线更新一并处理。
