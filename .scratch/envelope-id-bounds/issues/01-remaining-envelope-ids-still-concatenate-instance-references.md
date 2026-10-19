# 01 余下各口的信封 ID 仍原样串接实例引用：标识一长就入不了队，交接只留没人重发的续办引用

Category: bug
Status: in-progress——除运输履约外各口已进 main（至 `20a06645`）；余下运输履约各口待做，门禁登着去处。2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决」授权），出自 [routing-first-cut/22](../../routing-first-cut/issues/22-initial-route-envelope-id-exceeds-the-event-id-limit.md) 的取证。
Blocked by: 无
出处：`outboxintent.FingerprintEventID` 头注（票 sa-cc/34 裁决 2）：信封 ID 上限是 `eventing.MaxEventIDLength`，租户、委托、包裹这类引用多长归实例半边，本仓给不出上界，原样串接就是把上限押在别人的长度上。

## 现状（读码，钉 `45b9a088`）

关务各口与结算部分口已改指纹形，初始路由随 routing-first-cut/22 改。下列构造函数仍原样串接：

- node-operations：`collaborationAcceptanceEventID`、`executionFactEventID`、`nodeIntakeEventID`、`sealedSnapshotEventID`
- parcel-shipment：`continuedAttemptDecisionEventID`、`finalOutcomeEventID`、`labelTransactionJudgmentEventID`、`manualReviewCompletedEventID`、`networkIntakeEventID`、`parcelCancellationEventID`、`shipmentRequestSubmittedEventID`、`submissionVersionFormedEventID`
- party-commercial：`operatorRegistrationCompletedEventID`
- pilot-governance：`resumptionEventID`、`suspensionEventID`、`takeoverEventID`
- settlement-accounting：`chargeConfirmationEventID`、`supplierBillEventID`
- transport-fulfillment：`capacityConsumptionEventID`、`carrierFirstEffectivePickupEventID`、`dispositionExecutionEventID`、`effectiveDeliveryEventID`、`exceptionJourneyEventID`、`externalTrackingFactEventID`、`offsitePickupEventID`、`offsitePickupRegistrationEventID`、`regulatoryAcceptanceEventID`、`transportCommissionEventID`、`transportHandoverRegistrationEventID`
  - 2026-10-19 补（票 demo-chain-gaps/23）：其中 `transportCommissionEventID` 已在该票改走 `FingerprintEventID`（该口第一次有生产方），信封 ID 门禁的例外清单随之剪掉这一行；运输履约半边做时跳过它。
- visibility-exception：`triageEventID`、`visibilityGapEventID`

不经具名函数、在信封构造处原地串接的（全在 settlement-accounting 的 postgres 适配器里）：

- `operating_handoff.go`：分摊、经营结果两种信封
- `claim_settlement_handoff.go`：索赔金额、应收、确认、索赔调整
- `advance_recovery_handoff.go`：调整、追偿
- `statement_handoff.go`：纳入；另一处 ID 直接取分区键
- `supplier_bill_handoff.go`：应付、贷项（同文件只有一种形状走 `supplierBillEventID`）
- `settlement_application_handoff.go`：ID 直接取分区键
- `evaluation_request_handoff.go`：分区键加 `/submitted`

取证方法：按 `eventing.Envelope` 字面量里 `ID:` 那一格的表达式扫，不只扫具名的 `…EventID` 函数。

演示数据下哪些已越过上限，本票没逐个核。初始路由那一口是实测越过的。

## 做什么

- 逐口改走 `FingerprintEventID`：口名前缀加该口认领键的全部维。分区键不动；原先与 ID 共用一串的，分区键取那一串、一字不变（先例 `customsCasePartitionKey`）。原先就有独立分区键的口（如 `carrierFirstEffectivePickupPartitionKey`、`takeoverPartitionKey`）照旧。
- 每口补一条用真实长度标识的入队用例（先例 `TestAnInitialRouteWithFullLengthIdentifiersStillEnqueues`）。
- 要定的一格：交接因信封校验被拒是确定性失败，重发同一份照样被拒。今天它只化成一条续办引用，不报日志。要不要在交接口把这类失败另作一格响亮报出来，由本票定。

## 不做

- 不改分区键的语义，不改载荷形状。

## 完成记录（2026-10-19，Claude Code 会话，分支 `claude-eib01`）

运输履约各口没做：对端会话正在改那几个文件，等它收口后另做，门禁里登着去处。其余各口全部改完。

- **具名构造函数**（节点作业、委托、商业、治理、可视异常各口）改走 `outboxintent.FingerprintEventID`，口名前缀按口定，维度恰是旧 ID 的全部变量维。构造函数照旧交回 `string`，编排里的调用点不动。分区键：
  - 原先就有独立分区键的口，分区键不动。
  - 原先拿 ID 兼作分区键的口，都登在分区门禁的「同源例外」里（节点作业的协作承接、执行事实、收寄，以及结算的费用确认）。分区键跟着取指纹，每封仍自成一区，顺序语义不变，丢的只是可读性。
- **结算侧原地串接的各口**：在信封处对原先那串可读身份取指纹，状态段、类型段都还在那串里。分区键处理同上：原先独立的不动；索赔结算、预付回收原先与 ID 同源，分区键跟着取同一个指纹，口名抽成常量。
- **可达性判断与责任认定两口是门禁另查出来的**，票面原先没列。前者 ID 是四个标识串起来的，演示值已近上限；后者的索赔项标识来自登记批文，长度由外部定。两口都改成指纹，Subject 与分区键留可读形。可视异常缺口那一口的 Subject 也改回三维可读串。
- **新门禁 `TestEveryEnvelopeIDIsMintedAsAFingerprint`**：构造带 ID 的 `eventing.Envelope` 的生产文件必须经指纹铸 ID，例外只许变短；另有一条自检用例证它真能红。只取单一产品铸造定长标识的几口登为例外，理由点名铸造工厂，并写明哪两种情形会让例外失效。
- **用例**：各口的模板用例、两个进程与 `tests/bentocontract` 里照抄 ID 拼法的用例改成同一公式。按 ID 数信封的反向用例改按事件类型数，不然 ID 拼法一变它就恒真。结算侧加测试助手 `saID(口名, 可读串)`。

### 「做什么」里两项的裁决

- **逐口补真实长度标识的入队用例：不逐口补，改由门禁兜。** 指纹 ID 定长（口名前缀加六十四位，最长九十六字节），与任何一维多长无关，逐口补用例证的是同一件事。门禁让新开的口原样串接当场变红，比逐口补用例更能防回退。残余：可读的分区键与 Subject 仍随各维长度涨，上限是 `eventing.MaxPartitionKeyLength`（512），越过仍会被确定性拒收。
- **交接被信封校验确定性拒收时要不要另作一格响亮报出：本票不另造。** 先例 sa-cc/34 裁决 4 只在唯一有生产装配的那一路分了格。本票之后这些口的 ID 已不会因长度被拒，剩下的触发条件只有分区键或 Subject 越过 512，眼下不值得在各口逐个分格。运输履约那一半收口时如再碰到，再议。

### 换形边界

部署前已入队、部署后又被重放的意图，会算出新 ID 再入队一封。消费方 inbox 按事件 ID 去重，会把它当新信封再处理一次。本产品首发前没有租户的在途数据，本票按此口径不做迁移；各消费方大多另有业务键幂等（重放答「已存在」），即便有在途数据，后果也是多一次幂等处理。

## 进 main 记录（2026-10-19，除运输履约外各口）

- 隔离子代理一次过两轴评审：阻断 2 条，已修。一是有一条反向用例因 ID 拼法变了而恒真，已改按事件类型数；二是完成记录缺「做什么」里两项的裁决，已补。非阻断已修：常量挪到函数头注之上、可视异常缺口的 Subject 回可读形、结算口名抽常量、例外失效条件写准、测试头注去掉「加类型段」。
- 全量 `go test -p 1 -count=1 ./...`（带 55432 DSN）在干净游离检出上钉 `d2ee98db`：143 包 ok，0 FAIL，0 SKIP。
- 之后 rebase 跨过 ADR-0190 文档与对端会话的运输履约几笔。重跑了架构门禁、运输履约、`cmd/parcel-api`、`cmd/parcel-dispatch`，全部 ok。
- 快进推到 main：`e564ecbd..20a06645`。机制清点在钉 `20a06645` 的干净树上重生成，另起一笔。

2026-10-19 补：运输履约那一半另吸收容量消耗交接的事件 ID 与 `Consume` 重放幂等，裁定见 [demo-chain-gaps/tf-chain-plan](../../demo-chain-gaps/tf-chain-plan.md)「03 装载分配与容量」D03-7。

2026-10-19 补（票 demo-chain-gaps/17 的隔离评审）：容量池身份自票 17 起由（班次、维度）派生，写作 `<班次身份>/<维度词>`，自带 `/`，而班次身份本身也可能带 `/`（物化班次写作 `<周期安排代码>/<运行日期>`）。`capacityConsumptionEventID` 按 `/` 拼租户、池与预占，两组不同的键更容易拼成同一个 ID；运输履约半边改指纹形时一并消掉，不要只加长上限。
