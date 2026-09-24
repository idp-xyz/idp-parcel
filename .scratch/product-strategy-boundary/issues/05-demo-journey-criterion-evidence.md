# 05 第四条判据的动线取证：逐步列出停在`未配置`的每一格

Category: task
Status: ready-for-agent（盘点半 2026-09-24 通道 2 交付，见文末「盘点（钉 `64b37f27`）」；收口即第 3 步 Blocked by 02、03、04）
Blocked by: 02、03、04（只挡收口）
地盘：[合成演示动线](../../../docs/design/synthetic-demo-journey-script.md)与本目录票面。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定五第四条、越权风险点 2。

## 做什么

1. 在隔离环境按动线从建产品走到终局与结算，逐步记下今天停在`未配置`或未决哨兵的每一格。
2. 每一格按分界检验归类：产品策略 → 指向对应工作票；租户取值 → 用参考配置在演示租户上采用。
3. 各票收口后重走一遍，动线走通即第四条判据成立的证据（只记 `S`），把结果写回动线脚本「这条动线什么时候会变」一节。

## 不做

- 不为走通而在演示租户上代拟生产默认；每一格要么有内置策略，要么经参考配置显式采用。

## 完成判据

- 每一格都有归类与去处；收口时动线走通并留取证。

## 盘点（钉 `64b37f27`）

2026-09-24 通道 2 按通道 1 派单 task-c7df5ff3 做：第 1 步全做，第 2 步做到能做的程度，第 3 步未做。

**取证环境。** 代码钉 `64b37f27`（detached 检出）。库是 55432 上两只一次性库，走完已删：

- `idp_mcp2_journey`：按 `scripts/demo-seeds/seed.sh` 原样灌，是本次动线本身。
- `idp_mcp2_journey_cf`：反事实变体，只为看清格 1 之后停在哪。灌之前从发布批、服务形态、产品—渠道映射三处输入里去掉 `SYN-PROD-CN-SG-ECON`（即 `d61f2b7d` 之前的发布批形状），灌完即还原，仓里种子一字未动。它不是对演示租户的任何配置建议。

进程按动线脚本「前置」起：`cmd/parcel-api` 读写两个隔离开关同取 `SYN-TENANT-01`，`cmd/parcel-dispatch` 七个变量取脚本里的演示值。委托草案只用 `SYN-` 值（国家码 `CN` / `SG` 与种子同款）。取证时段 16:59–17:10。

**证据层级。** 「实测」是真进程或受控 CLI 上看到的答复；「探针」是一次性检出里的一次性测试（跑完即删、未入库），只为取回未决整笔回滚后库里不留痕的那一格原因；「代码」是钉 `64b37f27` 读装配点与注释所得，**没有走到**——上游已停。全部只记 `S`。

### 走到哪

- 第 1–4 步的读面全部答 `200`，没有一格停在哨兵上（`/commercial-*`、`/pricing-*`、`/network-catalog` 七族、`/customs-compliance-rules` 两册）。价格政策仍是有答案的空册（`COMMERCIAL_POLICIES_LISTED`，零行），不是哨兵。
- 第 5 步提交放行：`POST /shipment-requests` 答 `201 SUBMITTED`，生产归属 `IDP_PARCEL` / `OPEN`——墙一、墙二在隔离形态下确实过了。
- 此后按种子原样灌的演示租户**停在格 1**；反事实变体越过格 1 后**停在格 2**。格 2 在今天的装配上任何登记都解不开，所以格 3 起端到端都走不到，对外命令口逐个实打，进程内的链按代码记。

### 各格

#### 主链阶段 1 · 受理链（`AdvanceAcceptanceChainHandler`：逐包裹可达性 → 整份委托财务控制 → 形成决定）

**格 1 · 商业依据第一阶段：`适用冲突`**（实测 + 探针）

- 口：dispatch 投 `parcel-shipment.shipment-request.submitted` → 可达性段开头的 `formAdoptedBasis` → `CommercialBasisAdapter.ResolveCommercialBasis`。
- 答复：dispatch 记 `dispatch.consumer_undecided`，错误正文「acceptance chain is undecided: stage REACHABILITY_JUDGMENT, reason COMMERCIAL_BASIS_UNDETERMINED」；重投 3 次后 outbox 那一行落 `ABANDONED`，委托停在`已提交`，`acceptance_processing_attempt` 零行（未决整笔回滚）。PC 那一层的原因在进程上取不到，探针取回：闭包 `APPLICABILITY_CONFLICT`，`ConflictingBases()` 为 `[SERVICE_PRODUCT]`。
- 成因：发布批里 `SYN-PROD-CN-SG-EXPRESS` 与 `SYN-PROD-CN-SG-ECON` 两个服务产品同在 `SYN-SCOPE-01`（后者随 `d61f2b7d` 于 08-28 加入）。解析键按（租户，客户账户）登记一行，范围固定为 `SYN-SCOPE-01`、没有产品维；`CommercialResolutionKeys.FormResolutionKey` 只读那一行，委托草案里的 `requestedServiceProduct` 与 `destinationServiceScope` 进摘要、不进键。多候选答`适用冲突`是 UC-PC-002 的设计行为。
- 归类：机制缺口 + 产品策略缺执行器。登记面没有让委托声明参与折键的形状，属机制；「按委托声明选服务范围或产品」是不看任何租户就答得出的判断方法，属产品策略。不是租户取值：租户登记得再全，同一客户账户下两个产品在这张登记面上也只能冲突。演示种子同范围两个产品是这一格的触发条件，不是缺陷——两半落地后那是正当形态。
- 去处：[票 17](./17-requested-service-product-narrows-commercial-basis.md)（2026-09-24 通道 2 立，承接[票 06](./06-ps-acceptance-and-label-selection-judgment-methods.md) 第 8 项——那一项由通道 4 于 `ca26a1ec` 补入，本格此前一直写着待立票）。

**格 2 · 商业依据第二阶段：可达性判断时点`未配置`**（实测，反事实变体）

- 口：同上，第一阶段唯一解出之后的 `FormJudgmentAsOf`。
- 答复：outbox 那一行第 1 次即 `PUBLISHED`（入账，不再烧重投预算）；`acceptance_processing_attempt` 一行，`reason_ref = REACHABILITY_AS_OF_NOT_CONFIGURED`、`resume_path = OPERATOR_REGISTRATION`；`acceptance_adopted_resolution` 一行（`CLO-911126d21b3cc037`）；委托`已提交`，等待态`等待运营登记`。
- 证据：`cmd/parcel-dispatch/assemble.go` 的 `acceptanceCommercialBasis` 把 `Values` 留空；`internal/parcelshipment/adapters/partycommercial/judgment_as_of.go` 的 `FormJudgmentAsOf` 在 `values == nil` 时直接答`未配置`；`FormAsOfValue` 全仓只有测试替身实现。
- 归类：产品策略缺执行器（按声明语义折出时点值）；截点这类值属租户取值。与票 01 表 PN-02 第三项「受理链逐项时点 `Values` 为 nil」同格。补一条实测事实：续办路径写的是`等待运营登记`，而 `Values` 为 nil 时库里登记什么都解不开，恢复动作指向了一件做了也没用的事。
- 去处：[票 06](./06-ps-acceptance-and-label-selection-judgment-methods.md) 第 1 项（即票 01 表 PN-02 第三项，改判待 PC owner 复核）；截点取值经参考配置在演示租户上采用（采用路径归票 03）。

**格 3 · 可达性资格视图的闭包标识**（代码）

- 证据：`acceptanceReachability` 以 `nrpartycommercial.NewCommercialEligibility(resolutions, nil)` 装资格视图，函数注释原话「留 nil 时资格视图答未配置，编排形成`未形成判断`」。
- 归类：机制缺口。去处：票 01 表 PN-02 第一项。

**格 4 · 可达性网络证据**（代码）

- 证据：`acceptanceReachability` 的网络证据取 `nrpostgres.NewNetworkDefinitions`；那本册没有写入方、定义原语未设计，解析层不存在（`ErrNetworkDefinitionUnresolvable`）。
- 归类：机制缺口（登记册无写入方）+ 产品策略缺执行器（路由解析层）。去处：票 01 表 PN-02 两项；解析层归票 04。

**格 5 · 受理前财务控制**（代码）

- 证据：财务控制段同样先经 `formAdoptedBasis`，`Values` 为 nil 时先停在 `FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED`，与格 2 是同一个执行器缺口。越过之后，`acceptanceFinancialControl` 里 `PolicyBackedControlScopeSource` 的账户目录留空（注释：答 `CONTROL_SCOPE_NOT_CONFIGURED`），`Amounts` 也留空。
- 归类：账户目录背后没有结算账户登记册，属机制缺口；估价方法属产品策略缺执行器。去处：账户目录归票 01 表 PN-02 第一项；估价方法归票 06 第 2 项。

#### 主链阶段 2 · 初始路由

**格 6 · 初始路由证据**（代码）

- 证据：`acceptanceConsumer` 的初始路由证据视图与格 4 同由 `NetworkDefinitions` 实现，停在 `ROUTE_EVIDENCE_NOT_CONFIGURED`（动线脚本「墙三」）；商业适用性已按 ADR-0064 从已接受解析回指，不是缺口。
- 归类：同格 4。去处：票 01 表 PN-03 第三项与 PN-02 的网络定义登记册；解析层归票 04。
- 补一格（2026-09-24 通道 5，routing-first-cut/02 取证，钉 `1f7da903`）：**候选的关务适用性**。解析层接上之后，含关务段的候选还要 CC 答它的关务区域、口岸、申报路径是否合规可用（CONTEXT-MAP「customs-compliance ↔ network-routing」）；CC 今天只有口岸目录与申报路径目录的登记册（`ports.PortsPathsRegistry`），没有按路由候选作答的判断口。归类：产品策略缺执行器（CC 侧）。去处：[routing-first-cut/12](../../routing-first-cut/issues/12-cc-customs-applicability-judgment-for-route-candidates.md)；在它补上之前，这类候选按 [ADR-0148](../../../docs/adr/0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md)（Proposed）决定三答状态未知，初始路由停在路由判断未决。

#### 主链阶段 3 · 有效网络收寄与节点作业

**格 7 · 节点收寄与场外揽收的命令口**（实测）

- 答复：`POST /node-operations/receptions`、`/transport-fulfillment/offsite-pickups`、`/transport-fulfillment/offsite-pickup-attempts`、`/transport-fulfillment-carrier-first-effective-pickup-judgments` 全部 `403`，`{"error":{"code":"ACCESS_CHANNEL_NOT_CONFIGURED"}}`。
- 证据：`cmd/parcel-api/endpoints.go` 的 `assembleBusinessEndpoints` 里这几行挂字面量 `UnconfiguredIntake{}`，两个隔离开关都换不了。
- 归类：机制缺口。去处：票 01 表横切第一项（ADR-0100 操作者渠道未落地）。
- **08 重走**（2026-09-24 20:44，通道 6，[operator-channel/08](../../operator-channel/issues/08-isolated-release-of-main-chain-command-faces.md)；代码钉 `06a35347`，一次性库按 `seed.sh` 原样灌、走完即删，两个隔离开关同取 `SYN-TENANT-01`；只记 `S`）：写开关逐口放行后四口都越过渠道、答业务结果——收寄明确接收 `200 RECEPTION_UNDECIDED`（身份核对缝显式未配置，即格 8 同一缝）、明确拒收 `200 INTAKE_NOT_FORMED`；单对象揽收 `201 PICKUP_REGISTERED`；多对象揽收执行 `201 ATTEMPT_RECORDED`；承运商首次有效收寄判断 `200 PICKUP_PENDING`（`IDENTITY_NOT_REGISTERED`：合成承运方不在 PC 身份登记册）。格 7 的 403 不再成立；生产形态（开关不设）仍是 403，那一半等 ADR-0149 的实施票（10、13、14）。

**格 8 · 过渡期收寄批量口**（实测，反事实库）

- 答复：`parcel-frontline-import intake -tenant SYN-TENANT-01` 灌一行 `RECEIVED`，答「RECEPTION_UNDECIDED [未决] 未落库」，退出码 3，`node_operations.reception` 零行；工具开跑前自己打印「身份核对缝未配置（.scratch/ps-external-mark-relations/01）」。
- 证据：`cmd/parcel-api/assemble_reception.go` 的 `unconfiguredParcelIdentityView`，批量口同一处置。
- 归类：机制缺口。去处：票 01 表 PN-03 第一项，票 `ps-external-mark-relations/01`。

#### 主链阶段 4 · 关务

**格 9 · 立案与提交申报没有生产入口**（代码）

- 证据：`internal/customscompliance/application` 的 `NewEstablishCaseHandler`、`NewSubmitDeclarationHandler` 在 `cmd/` 零引用；接受决定的扇出 `acceptanceFan` 只到 VE 与初始路由。生产进程里没有任何端点或消费门会建案或提交。
- 归类：装配缺，属机制缺口；「谁在什么业务时点为哪些包裹建案、提交」的触发面属产品策略缺执行器。
- 去处：[票 18](./18-customs-case-and-declaration-submission-entry.md)（2026-09-24 通道 2 立；此前待立票——[票 10](./10-cc-declaration-channel-and-public-regulatory-reference-configuration.md) 只含申报发送连接器（格 10），立案与提交的装配和触发面不在 06–14 里）。

**格 10 · 申报发送通道**（代码）

- 证据：`customs-compliance.declaration-submission.formed` 唯一的消费方是 VE 投影，没有交给海关或报关服务商的连接器。
- 归类：产品策略缺执行器（连接器形态）。去处：票 10 第 1 项（即票 01 表 PN-05 第三项）。

**格 11 · 外部结果与凭证登记口**（实测）

- 答复：`POST /customs/external-results`、`/customs-regulatory-credential-registrations` 均 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。
- 归类：同格 7。去处：票 01 表横切第一项。
- **08 重走**（同格 7 那次取证）：外部结果 `200 UNATTRIBUTABLE`——声称的申报版本不在提交索引里，编排留存原始响应不猜；它之后的解释、辖区、层次事实要等格 9（立案与提交申报没有生产入口）先有一份提交才走得到。监管凭证登记 `201 REGISTERED`。

#### 主链阶段 5 · 运输履约、交付与终局

**格 12 · 交接、移动、派送与交付的命令口**（实测）

- 答复：`/transport-fulfillment/handovers`、`/transport-fulfillment/movement-facts`、`/transport-fulfillment-dispatch-task-registrations`、`/transport-fulfillment-delivery-dispatch-triggers`、`/transport-fulfillment/deliveries`、`/transport-fulfillment-segment-closures`、`/transport-fulfillment-effective-time-judgments` 全部 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。
- 归类：同格 7。去处：票 01 表横切第一项。
- **08 重走**（同格 7 那次取证）：各口都越过渠道——交接 `201 HANDOVER_REGISTERED`；移动（自营到达）`201 MOVEMENT_FACT_RECORDED`；手工建派送任务 `201 DISPATCH_TASK_OPENED`；段关闭对不在册的段 `200 SEGMENT_NOT_FOUND`；有效时间判断对不在册的轨迹事实 `200 INPUT_NOT_ACCEPTED`（外部轨迹只经 `TrackingSource` 入站口进，即格 14）。往下串一步：交接带 `segmentServiceAction=FINAL_DELIVERY` 进派送段后，派送发起 `200 REQUIREMENT_MISSING`（`DELIVERY_PLACE`、`DELIVERY_WINDOW`、`DELIVERY_CONDITION`——即格 13 的执行器缺口），同段关闭 `200 SEGMENT_STILL_ACTIVE`。**新停点一处**：交付 `200 SOURCE_NOT_ACCEPTED`——交付只能落在已登记的派送尝试结果上，而派送尝试登记册（`tfpostgres.DeliveryAttempts`，只读）全仓没有生产写入方。归类：机制缺口（登记册无写入方）；去处：[票 19](./19-delivery-attempt-result-has-no-production-writer.md)（2026-09-24 通道 2 立；此前待立票——06–14 与 operator-channel 目录都不含这一格）。

**格 13 · 派送发起**（代码）

- 证据：`cmd/parcel-api/assemble_delivery_dispatch.go`：执行器与端点都在，缺内置的发起策略；拍频是租户取值。
- 归类：产品策略缺执行器。去处：[票 09](./09-tf-fulfillment-judgment-methods-and-connectors.md) 第 2 项（即票 01 表 PN-04 第三项）；拍频经参考配置在演示租户上采用。

**格 14 · 外部轨迹来源连接器**（代码）

- 证据：`internal/transportfulfillment/ports` 的 `TrackingSource` 一家实现都没有。
- 归类：产品策略缺执行器（连接器形态）。去处：票 09 第 1 项（即票 01 表 PN-04 第三项）。

终局本身不记格：`adoptEffectiveDeliveryConsumer` 已接在 dispatch 路由表上，未决面是交付可见性、目标委托与终局规则三样（`effectiveDeliveryUndecidedSentinels`），而上游没有一条交付事实进得来，本次看不出它走到时会不会再停。

#### 主链阶段 6 · 追踪、异常与客户披露

**格 15 · 客户侧读面与写面**（实测）

- 答复：`GET /customer-tracking-view`、`POST /claims` 均 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。
- 归类：产品策略（客户侧接入渠道族；ADR-0139 至 0142 仍 Proposed，接受与否归用户）。去处：票 01 表横切第三项。

**格 16 · 客户通知出向连接器**（代码）

- 证据：`NotificationChannelGateway` 无实现。
- 归类：产品策略缺执行器。去处：[票 11](./11-ve-eta-triage-disclosure-methods-and-notification-connector.md) 第 1 项（即票 01 表 PN-06 第三项）。

内部追踪投影没有自己的停格：`/tracking-projections` 答 `PROJECTIONS_LISTED` 空册，VE 那几条投影消费的源事实（初始路由、交接、终局等）一条都没进来。

#### 主链阶段 7 · 计价与结算

**格 17 · BUY 评价请求没有触发面**（代码）

- 证据：`cmd/parcel-api/assemble_evaluation_request.go` 的 `buildEvaluationRequestOrchestration` 头注原话「今天没有运营端点、也没有进程内触发面调它」，「谁在什么业务时点为哪些发生项发起请求是产品题，触发面另票」；`main` 装它只为 fail-fast，产物即丢。
- 归类：产品策略缺执行器（触发策略）。去处：[票 20](./20-buy-evaluation-request-trigger.md)（2026-09-24 通道 2 立；此前待立票——06–14 都不含；[票 12](./12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md) 管金额文法与形态，不管评价请求的触发面）。

**格 18 · BUY 评价到供应商预期成本：来源引用两头没接上**（代码）

- 证据：`cmd/parcel-dispatch/assemble.go` 的 `buyEvaluationRecordedUndecidedSentinels` 里 `sapricing.ErrSourceReferencesUnrecorded` 一格：评价回指评价请求标识归 PP 侧，注释原话「今天两头没接上」。
- 归类：机制缺口（回指）。去处：票 `sa-cc-funds-and-credential-seams/11`。

**格 19 · SELL 评价到客户费用**（代码）

- 证据：SELL 评价没有请求面——PP 的生产入口只有 SA 评价请求信封一条，而 SA 只发 BUY 目的的请求（`RequestBuyEvaluation`）。评价已记录信封的消费门只收 BUY·供应商成本，同处注释原话「SELL 那一半只能在这里安静地走掉」。SA 应用层没有由评价形成客户费用的编排，`domain.FormCustomerCharge` 唯一的生产调用在库适配器的重建路径上。
- 归类：缺消费门与形成编排，属机制缺口；SELL 评价何时发起属产品策略缺执行器（触发策略）。去处：[票 21](./21-sell-evaluation-to-customer-charge.md)（2026-09-24 通道 2 立；此前待立票——06–14 都不含）。

**格 20 · 费用确认、对账单截单发布及其余结算编排没有生产入口**（代码）

- 证据：`internal/settlementaccounting/application` 的 `NewConfirmChargeHandler`、`NewCutOffPublishStatementHandler`、`NewRecordChargeAdjustmentHandler`、`NewAllocateCostsHandler`、`NewReceiveSupplierBillHandler`、`NewAuditSupplierBillHandler`、`NewSettleClaimAmountsHandler` 在 `cmd/` 零引用。这一类不在生产接线棘轮的网里（棘轮排除 `New*` 构造，见开发主线「2026-09-02 裁决」一段）。
- 归类：装配与入口缺，属机制缺口；何时确认、何时截单的触发面属产品策略缺执行器；账期是租户取值，经参考配置在演示租户上采用。去处：赔付金额文法与分摊形态归票 12 第 1、2 项；这几个编排的装配与触发面归[票 22](./22-settlement-orchestrations-assembly-and-triggers.md)（2026-09-24 通道 2 立；此前待立票——06–14 都不含）。

**格 21 · 结算四口没有生产实现**（代码）

- 证据：`ClaimAmountRuleView`、`ConfirmedChargeFactsView`、`SupplierAuditAuthorityView`、`SupplierPayableAccountView`。
- 归类：机制缺口。去处：票 01 表 PN-07 第一项；审核授权那一口的越权升级判断结构另归票 12 第 5 项。

**格 22 · 外部资金事实在线口**（实测）

- 答复：`POST /settlement-external-funds-fact-registrations` 答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`；受控 CLI `parcel-settlement-register` 是同一用例的另一口，本次未跑。
- 归类：同格 7。去处：票 01 表横切第一项。
- **08 重走**（同格 7 那次取证）：一条收款确认 `201 FUNDS_FACT_ADOPTED`；采用之后的映射与核销要格 19–21 那一段先有客户费用，本次未往下走。

**不在主路径上的命令面**（实测，不计格）：`/shipment-requests/` 下的撤回、取消、复核完成、主动拒绝、授权处置、受控补充六口，以及 `/pricing-evaluation-replays`、`/commercial-publications`，全部 `403 ACCESS_CHANNEL_NOT_CONFIGURED`，根因同格 7 或格 15。

### 归类汇总

| 归类 | 格 |
|---|---|
| 只缺机制 | 3、7、8、11、12、18、21、22（7、11、12、22 是同一根因：操作者渠道） |
| 只缺产品策略执行器 | 2、10、13、14、15、16、17 |
| 机制与产品策略两半都缺 | 1、4、5、6、9、19、20 |
| 租户取值（只作半格出现，经参考配置在演示租户上采用） | 2 的截点、13 的拍频、20 的账期 |

### 与票 01 缺口表对不上的格

- **格 1**：票 01 第四项推断受理链首停在第二阶段逐项时点；实测首停早一格，在第一阶段 `SERVICE_PRODUCT` 适用冲突，表里没有这一格。格 2 本身与票 01 对得上（反事实变体实测）。
- **格 9**：关务立案与提交申报没有生产入口；票 01 表 PN-05 第一项记「未见缺口」。
- **格 17、18、19、20**：计价到结算这一段的触发面、回指、SELL 形成路与结算编排的装配；票 01 表 PN-07 第一项只列了格 21 的四口与 `PricingInputResolver`，第三项记「未核」。

票 01 表里有、这条动线的主路径没碰到、本次未取证的：面单择优链各格（PN-02）、授权请求坐标 `RequestSource`（横切第三项前半）。

**票 02 于 `17865872` 立的 06–14 也没接住的**：格 1、9、17、19，以及格 20 的装配与触发面那一半——盘点时没有任何票，去处写「待立票」；2026-09-24 通道 2 按格各立一票（17–22，格 12「08 重走」新停点那一格归 19），各格去处已改指新票。

### 给第 3 步的旁记（动线脚本与今天的现状不符处；本单不改脚本正文）

- 「起 dispatch」一节说 dispatch 对逐条投递失败一行日志都不打。今天每次失败都打一条 `WARN`「Delivery failed and was recorded for retry」，未决时正文带 PS 那一层的 `stage` 与 `reason`；PC 那一层的细分原因仍取不到，格 1 靠探针才拿到。
- 「前置」一节以启动日志为放行判据。`Parcel API listening` 这一行在绑定端口**之前**打出，端口被占时照样出现，随后才是 `ERROR`「Parcel API stopped」「address already in use」——只看那一行会把没起来的进程当成起来了。
- 「取证」一节的行数随种子变了：`/commercial-service-products` 今天 2 行，`/commercial-policies?kind=SETTLEMENT_POLICY` 今天 1 行（表里分别是 1 与 0）。
- 第 5 步「委托侧两种跑法」的第二种只写到`已提交`。按种子原样灌，受理链停在格 1，重投烧完落 `ABANDONED`——与 `first-tenant-runway/07` 当年只灌治理那一行的现场同形，成因不同。

### 判断项

1. 格 1 的两半是按票 01 的三分判的。UC-PC-002 把解析键上的区分维定为「服务范围」，所以缺的也可能只是「从委托推出服务范围」这一步，而不是给解析键加产品维；归 PC owner 复核。
2. 格 3 起全部是代码取证，没有端到端走到。它们是缺口的下界，真走到时也可能换成别的停法。
3. 反事实变体只去掉 ECON，用来看清格 1 之后的停点，不代表对演示租户的配置建议。
