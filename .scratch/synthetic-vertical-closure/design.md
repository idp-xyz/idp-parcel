# 合成纵向闭环设计（SYN-*）

- 证据点：`origin/main` **`1fff679`**（`test(pc): disambiguate rule-package fixtures after B6/B7 integration`）
- 性质：只读设计。本文件之外未改生产代码、未改 `docs/adr`、未更新开发主线。
- 目的：给出一条**可执行、证据诚实**的合成纵向测试路径：委托 → 路由 → 收寄 → 履约 → 追踪 → 计价结算。所有实例值标 **S**，不冒充真实租户、不升级为 `R`/`P`。

## 0. 先说结论

今天**没有**一条从 HTTP 入口到结算的生产闭环。组合根已经能把两封真实 Outbox 信封经 Dispatcher 投到两个 NR 消费者，但：

1. 八个业务 HTTP 端点一律「未配置即拒」（ADR-0055），进不了提交编排。
2. `SubmitShipmentRequestHandler` / `FormAcceptanceDecisionHandler` **没有**任何 `cmd/` 构造；生产归属口 `ProductionOwnershipAuthority` 全仓只有测试替身。
3. 商业解析键映射（`ResolutionKeySource`）与 NR 适用性映射在生产装配里是 `nil`——首发诚实未配置。
4. 阶段内容 `AdoptedStageOwner` 只有 `UnconfiguredAdoptedStageOwner`，且未进 `cmd/`。
5. 网络定义登记册无写入方；即便插入一行，无 `PAR-NET-14` 解析层也会答 `ErrNetworkDefinitionUnresolvable`（ADR-0053），不得拿空事实冒充「无当前有效路由」。
6. 46 个 Outbox handoff / 55 类信封里，路由表只登记 **2** 类。发出 `network-routing.initial-route.formed` 会撞 `dispatch.no_subscriber` 并阻塞该分区（ADR-0049 第三条，有意）。

因此首个 SYN 测试**不得** pretends 跑完整「委托→…→结算」。它应该用真实 PostgreSQL + 真实 Outbox/Inbox + 真实 Dispatcher，从应用编排（不是 HTTP）把一次 **S** 接受决定送过已接线的那一跳，并断言下游停在**具名未决格**，而不是编一条路由。

现有 `SYN-CHAIN-*` 联检（`internal/parcelshipment/domain/synthetic_chain_contract_test.go` 等）是领域值对象合约，**不经过** handler、仓储、Outbox、Dispatcher。不得把它报成跨上下文闭环。

---

## 1. 取证口径（1fff679）

| 测量面 | 数量 | 出处 |
|---|---|---|
| Outbox handoff 适配器 | 46 | `internal/*/adapters/postgres/*handoff.go` |
| Inbox 消费者 | 2 | `internal/networkrouting/adapters/inbox/` |
| 路由表条目 | 2 | `cmd/parcel-dispatch/assemble.go` `wireDispatcher` |
| `application` 生产文件 | 59 | 与开发主线「59 份编排」一致；`cmd/` 只构造其中 **2** 个：`CreateInitialRouteHandler`、`ReassessRouteHandler` |
| HTTP 业务端点 | 8 | `cmd/parcel-api/endpoints.go`，全部 `UnconfiguredIntake` |

路由表（生产）：

| 信封类型 | 消费者名 | 编排 |
|---|---|---|
| `parcel-shipment.acceptance-decision.formed` | `network-routing/initial-route-on-acceptance` | `CreateInitialRouteHandler` |
| `parcel-shipment.network-intake.recorded` | `network-routing/reassess-on-network-intake` | `ReassessRouteHandler`（只写复核，不发下游信封） |

`assemble.go` 对接受链的注释仍然成立（1fff679 代码）：商业适用性映射 `nil` → 整份交接 `ROUTING_APPLICABILITY_UNAVAILABLE`；过了才会碰到网络定义册 `未配置`。顺序写反会排错下一票。

---

## 2. 逐段表

缺口四态：**缺消费者** / **缺消费适配器** / **缺实例参数（含 SYN 映射）** / **用例本身没有这条事件**。另标「领域件在位、cmd 未构造」。

所有 SYN 标识前缀约定：`SYN-TENANT-01`、`SYN-CUSTOMER-01`、`SYN-SCOPE-01`、`SYN-PRC-SELL-01` 等，与参数登记册「可登记隔离执行索引、不得当作确认依据」同纪律。

### 2.1 委托（parcel-shipment）

| 段 | 输入 | 生产 handler | 真库 repo | handoff | 消费方 | 期望业务结果 | 当前缺口 |
|---|---|---|---|---|---|---|---|
| 接入 | HTTP POST `/shipment-requests` | `SubmitShipmentRequestEndpoint` | — | — | — | 形成提交命令 | **缺实例参数**：`UnconfiguredIntake` → 403 `ACCESS_CHANNEL_NOT_CONFIGURED`。SYN 测试**绕过 HTTP**，不把未配置 Intake 换成默认渠道 |
| 提交 | `SubmitShipmentRequestCommand` | `SubmitShipmentRequestHandler`（cmd 未构造） | `ShipmentRequests`、`SourceSubmissions` 已有 | 无（提交不发接受信封） | — | `已提交`，不形成接受 | **缺消费适配器/权威实现**：`ProductionOwnershipAuthority` 无生产实现，仅测试替身。SYN 测试用具名 `SYN` 归属替身，标 S |
| 判断推进 | `AdvanceAcceptanceJudgmentCommand` 等 | 对应 handler（cmd 未构造） | `AcceptanceJudgments` 已有 | 无 | NR 可达性目前是**同步读口**，不是事件 | 任务可续办，委托仍 `已提交` | **缺实例参数**：`ResolutionKeySource` / `AsOfValueSource` 未配置则商业依据停在 `COMMERCIAL_RESOLUTION_KEY_NOT_CONFIGURED`。可达性事件半边 UC 未明文（同步链已能拿到判断） |
| 形成决定 | `FormAcceptanceDecisionCommand` | `FormAcceptanceDecisionHandler`（cmd 未构造） | 同上 + `OutboxAcceptanceDecisionHandoff` | `parcel-shipment.acceptance-decision.formed` | NR `AcceptanceConsumer` **已接线** | 接受/拒绝落库且意图入队 | 要形成 `DECIDED`，须先有已记录判断 + 唯一商业解析 + 已声明复核策略。全用 S 夹具。未齐则诚实 `UNDECIDED`，不得直接调 `request.Decide` 冒充 |
| 阶段规则 | 收寄/终局/取消 | `ServiceStageRulesAdapter` | PC 点读口已有 | — | — | 按采用的规则版本翻译 | **缺实例参数**：`AdoptedStageOwner` 在 `SourceIdentity` 上取不到采用版本；生产形状是 `UnconfiguredAdoptedStageOwner`。未进 cmd |

### 2.2 路由（network-routing）

| 段 | 输入 | 生产 handler | 真库 repo | handoff | 消费方 | 期望业务结果 | 当前缺口 |
|---|---|---|---|---|---|---|---|
| 初始路由 | 接受决定信封 | `CreateInitialRouteHandler`（**cmd 已构造**） | `InitialRoutes`、`RouteHandoffLogs`、`RouteIdentities` | `network-routing.initial-route.formed` | 应有 NO/TF/VE，**未开** | 形成初始路由计划 | **缺实例参数**（适用性映射、网络定义）；即便登记定义也 **缺解析层**（ADR-0053）。形成后立刻 **缺消费者** → `no_subscriber` 堵分区 |
| 收寄复核 | 网络收寄采用信封 | `ReassessRouteHandler`（cmd 已构造） | `RouteReassessments` 等 | 无 | — | 失效落库；自动改路 `AutoReroute=nil` 不做 | 上游 `network-intake.recorded` 要有人发。今天发它的是 PS `AdoptNetworkIntakeHandler`，cmd 未构造 |
| 可达性判断 | 同步端口 | `AssessParcelReachabilityHandler`（cmd 未构造） | 可达性仓储已有 | `network-routing.reachability-judgment.formed` | PS 续办 **缺消费适配器**（事件侧）；同步读口已有 | 三值判断参与接受 | 事件 vs 同步分工未裁。首个 SYN **走同步口**，不新开事件消费者 |

### 2.3 收寄（node-operations / transport-fulfillment → PS）

| 段 | 输入 | 生产 handler | 真库 repo | handoff | 消费方 | 期望业务结果 | 当前缺口 |
|---|---|---|---|---|---|---|---|
| 节点收寄 | HTTP `/node-operations/receptions` | `ReceiveDeliveredUnitHandler` | NO 收寄库已有 | `node-operations.node-intake.formed` | PS `AdoptNetworkIntakeHandler` + `intake_source.go` **成型未接线** | 采认为有效网络收寄 | HTTP 未配置即拒。跨上下文：**缺消费者**；更深的缺口是 **按包裹反查来源身份**（委托 jsonb 无包裹索引）。见勘察 `.scratch/outbox-handoff-consumption-map/next-consumer-survey.md`，1fff679 代码形状未变 |
| 场外揽收 | TF 两拍 | `PerformOffsitePickup` / `RegisterOffsitePickup`（cmd 未构造） | TF 库已有 | `.formed` / `.registered` | 同上，`pickup_source.go` 成型未接线 | 解释为有效网络收寄 | 同反查缺口 |
| 采用后复核 | `parcel-shipment.network-intake.recorded` | （PS 发、NR 收） | `IntakeAdoptions` 已有 | 该信封 | NR `NetworkIntakeConsumer` **已接线** | 复核 | 要先有采用记录。反查未做则发不出诚实采用 |

### 2.4 履约（transport-fulfillment → PS 终局）

| 段 | 输入 | 生产 handler | 真库 repo | handoff | 消费方 | 期望业务结果 | 当前缺口 |
|---|---|---|---|---|---|---|---|
| 有效交付 | HTTP `/transport-fulfillment/deliveries` | `RegisterEffectiveDeliveryHandler` | TF 交付库已有 | `transport-fulfillment.effective-delivery.registered` | PS `FormParcelFinalHandler` + `delivery_outcome.go` 成型未接线 | 终局判断 | HTTP 未配置；**缺消费者** + 同一反查口 |
| 初始路由落地作业 | `initial-route.formed` | — | — | 见上 | NO 接路由指令 | 节点按计划作业 | **缺领域建模**：NO 三个编排不对口「版本化路由指令」。不可塞进首个 SYN |
| 承运委托 | `CommissionTransport` | cmd 未构造 | 已有 | `transport-commission.submitted` | SA 成本预期 **缺消费者** | 供应商预期成本上游 | 不进首切 |

### 2.5 追踪（visibility-exception）

| 段 | 输入 | 生产 handler | 真库 repo | handoff | 消费方 | 期望业务结果 | 当前缺口 |
|---|---|---|---|---|---|---|---|
| 投影 | 终局/履约事实 | `DeriveProjectionHandler`（cmd 未构造） | VE 投影库已有 | `tracking-projection.derived` | 同上下文 `DeriveCustomerViewHandler` **缺消费者** | 追踪投影 | 缺事件消费者（同上下文也要配，ADR-0049） |
| 客户视图 | 投影 | `DeriveCustomerViewHandler` | 已有 | `customer-view.published` | 通知/门户 | 可查询视图 | 投影不带客户账户，`FindCurrent` 要账户 → **反查缺口**（VE 面） |
| HTTP 查询 | `/customer-tracking-view` | 查询端点 | — | — | — | 视图 | 未配置即拒 |

### 2.6 计价结算（parcel-pricing / settlement-accounting）

| 段 | 输入 | 生产 handler | 真库 repo | handoff | 消费方 | 期望业务结果 | 当前缺口 |
|---|---|---|---|---|---|---|---|
| 评价 | 接受后计费请求 | `EvaluatePricingHandler`（cmd 未构造） | 评价册已有 | `parcel-pricing.evaluation.recorded` | SA **缺消费者** | 不可变评价 | **缺实例参数**：价卡 `PAR-SET-02/03` 待提供。SYN 可用 `SYN-PRC-SELL-01` 索引，**不得**把夹具写成已确认价卡 |
| 费用确认 | 评价 | `ConfirmChargeHandler`（cmd 未构造） | SA 库已有 | `charge-confirmation.formed` | SA 对账单纳入（同上下文 **缺消费者**） | 已确认费用 | 同上下文仍须路由条目 |
| 预控 | 接受前 | PS→SA 同步适配已有部分 | 冻结账已有 | 部分直调，不全靠事件 | — | 冻结/信用 | SYN 财务控制须标 S；不得调用外部资金系统 |

关务旁路（CC 九口）**不进入首个 SYN 主链**。主线是网络服务；独立面单渠道 `PAR-COM-12` 首发不适用。

---

## 3. 最小可执行切片（SYN-V0）

### 3.1 覆盖哪几段

**只覆盖「已提交委托 →（S 夹具凑齐判断）→ 接受决定落库 → Outbox → Dispatcher 一拍 → NR AcceptanceConsumer → CreateInitialRoute 跑到诚实未决」。**

这是今天唯一两端都有**真实**生产适配器的跨上下文跳：PS `OutboxAcceptanceDecisionHandoff` ↔ NR `AcceptanceConsumer` ↔ `CreateInitialRouteHandler`。

断言（全部要看见，缺一则假绿）：

1. 真库有 `shipment_request` 行，状态为接受（或本轮若判断未齐，则停在 `UNDECIDED` 并**不得**入队接受信封——两条路径分两个测试，不要混）。
2. `DECIDED` 路径：Outbox 有且仅有一封 `parcel-shipment.acceptance-decision.formed`，ID = 决定标识（ADR-0043）。
3. Dispatcher `Beat` 之后 Inbox 有 `network-routing/initial-route-on-acceptance` 的已处理账。
4. NR 编排进入后停在 **适用性不可用**（`nil` 映射）或翻译后的未决哨兵，**不**写入一条可执行路由计划。
5. **不**发出可被无订阅者卡住的 `initial-route.formed`（未决路径不应 handoff）。若误发，测试必须失败——这正是「纵向一跑就堵分区」的回归钉。

### 3.2 明确不塞进同一测试

| 段 | 为什么现在塞会假绿或违规 |
|---|---|
| HTTP 八端点 | 未配置即拒是机制正确答案；换成 SYN Intake 等于给生产装配一条默认渠道 |
| 收寄 / 揽收 / 交付 | 缺反查读口；硬编码 `TargetShipment` 是发明来源身份 |
| `initial-route.formed` → NO/TF | 缺 UC；登记消费者却处理不了，违反 ADR-0049 第三条 |
| VE 投影 / 客户视图 | 缺同上下文消费者 + 账户反查 |
| PP 评价 / SA 费用 | 价卡实例半边；且 `evaluation.recorded` 无消费者 |
| 关务主路径 | 旁路；另票 |
| 直接 `domain.SubmitShipmentRequest` / `Decide` 当闭环 | 绕过仓储与 Outbox，不是跨上下文 |

### 3.3 测试落点与装配缝

- **新包**（建议）：`tests/synthetic/` 或 `internal/platform/syntest/`，**禁止**把 SYN 夹具写进 `cmd/parcel-dispatch/assemble.go` / `cmd/parcel-api/endpoints.go`。
- 测试进程自己 `wire`：真 `pgtest.Pool`（完整迁移）+ 真 Outbox/Inbox Store + 抽 `wireDispatcher` 同款路由表（可测已导出的 `wireDispatcher`，不要复制一份字符串路由）。
- PS 侧：真 `ShipmentRequests` / `SourceSubmissions` / `AcceptanceJudgments` / `OutboxAcceptanceDecisionHandoff`。
- SYN 替身（必须点名、注释写 S）：`ProductionOwnershipAuthority`、`ResolutionKeySource`（若走真实 PC 解析则改为种子登记册 + SYN 键映射）、`AsOfValueSource`、判断记录的种子或 `Advance*` 的 SYN 可达性答复。
- 迁移前置：现有计划原样施加，**不新增业务迁移**。种子用各上下文已有 `Save*` 写入 SYN 行；没有 Save 的册（网络定义）**不要 INSERT**——会掉进「已登记但无解析层」。

### 3.4 数据库 fixture（S）

最低集（名称均带 `SYN-`）：

| 对象 | 怎么进库 | 没有它会怎样 |
|---|---|---|
| 租户 / 客户账户 / 来源身份 | 命令字段，不落 PC 参与方表也可（PS 自己的身份值对象） | 提交立不起来 |
| 生产归属决定 | SYN 替身返回「本范围由本产品承接」 | 提交停在归属未决 |
| 商业唯一解析 | **优先**：PC `PublicationRegistry.SaveVersion` + `CommercialResolutionStore` + SYN `ResolutionKeySource` 指向 `SYN-SCOPE-01`。**次选**（若首票要更瘦）：PS 测试替身 `CommercialBasisResolver` 交回唯一适用快照，仍标 S，并在测试名写明「未跑 PC 持久化解析」 | 形成决定停在依据未配置/未决 |
| 已记录可达性 + 财务控制 | 真 `AcceptanceJudgments` 写入 SYN 判断，或先跑 `Advance*` 同步口 | `Decide` 因校验组未齐保持未决 |
| 接单规则包内容（适用组 + 复核指令） | 若走真 PC 点读：0012/0006 类声明表种子；否则打进 SYN 快照 | 停在 `ManualReviewPolicyNotDeclared` |

网络定义、价卡、授权规则采用版本：**V0 不种**。

---

## 4. 后续实现票（依赖边）

每票可独立验绿：有自己的 `go test` 范围与失败语义。标 S 的夹具不得进生产组合根。

| ID | 内容 | 依赖 | 可并行 | 验绿 | 共享文件风险 |
|---|---|---|---|---|---|
| **SYN-V0** | 进程测试：提交（S 归属）→ 形成决定（S 依据/判断）→ 真 Outbox → 真 Dispatcher → NR 消费门 → 诚实未决 | 无新生产代码亦可先写测试（红）；绿需要测试装配，可能抽 `wireDispatcher` 为可测函数 | 与反查口可并行 | `tests/synthetic` 真 PG，断言 §3.1 | `cmd/parcel-dispatch/assemble.go` 若抽取签名 |
| **SYN-PC-SEED** | SYN 商业版本种子走真 `SaveVersion`/`Save` 解析，替换 V0 里的 `CommercialBasisResolver` 替身 | V0 | 可与 V0 后半合并 | 解析闭包 `UniquelyResolved`，证据级 S | `internal/partycommercial/ports.go` 不改；种子测试文件 |
| **PS-PARCEL-INDEX** | 按包裹反查来源身份 + 委托号（机制，不是实例） | 无 | **高杠杆，可与 V0 并行** | 新读口 + 迁移；四条采认方向的单测 | `migrations/parcel_shipment/00xx` 占号；`shipment_request` 模式 |
| **VE-ACCOUNT-INDEX** | 投影/视图按包裹反查客户账户 | 无 | 可与 PS-PARCEL-INDEX 并行 | VE 读口 + 可能迁移 | `migrations/visibility_exception/00xx` |
| **NR-APPLICABILITY-S** | SYN 判断键→解析标识映射（测试装配，不进生产 `nil`） | SYN-PC-SEED | 接在 V0 之后 | 过适用性关，停在网络定义未配置（第二道哨兵） | **不要**改生产 `assemble.go` 的 `nil`；只改 syntest 装配 |
| **NR-RESOLVER** | `PAR-NET-14` 解析层机制（无真实线路实例） | 领域/ADR 已有 0053 | 与 V0 并行但更大 | 有定义时交出九族或显式未配置，不再 `Unresolvable` | NR ports、migrations |
| **CONS-INTAKE** | `node-intake.formed` → PS 采认消费者 + 路由表第三条 | PS-PARCEL-INDEX | 互斥改 assemble.go | Dispatcher 投递后 `IntakeAdoptions` 有行并可能发 `network-intake.recorded` | **`cmd/parcel-dispatch/assemble.go`**、NR/PS inbox 包 |
| **CONS-DELIVERY** | `effective-delivery.registered` → `FormParcelFinal` | PS-PARCEL-INDEX | 与 CONS-INTAKE 抢 assemble.go | 终局落库 + `final-outcome.formed` 入队 | 同上 |
| **CONS-INTAKE-REASSESS** | 采用信封已有消费者；接 CONS-INTAKE 后跑第二跳 | CONS-INTAKE | — | 复核行落库；`AutoReroute` 仍 nil | 低 |
| **CONS-ROUTE-NO** | `initial-route.formed` → NO | **先要 NO 的「接收路由指令」UC** | 不可抢跑 | — | assemble.go、NO CONTEXT |
| **CONS-EVAL-SA** | `evaluation.recorded` → SA | SYN 价卡索引 + EvaluatePricing cmd 测试装配 | 晚 | 评价入 SA | assemble.go、SA ports |
| **HTTP-SYN-INTAKE** | 测试专用 Intake，**永远不进** `assembleBusinessEndpoints` | V0 已绿 | 晚 | 仅 syntest HTTP 或明确 `testing` build tag | **`cmd/parcel-api/endpoints.go` 严禁**默认渠道 |

**不要做的票**：把 `UnconfiguredAdoptedStageOwner` 换成默认规则包；给网络定义 INSERT 一行却不交解析层；为 53 类未订阅信封先登记空消费者。

---

## 5. 验收：真跑 vs 只记 S

### 必须实跑（生产适配器 + 真 PostgreSQL）

- `pgtest.Pool` 施加现行迁移计划（含 party_commercial 0013 等已在 1fff679 的号）。
- PS：`SourceSubmissions`、`ShipmentRequests`、`AcceptanceJudgments`、`OutboxAcceptanceDecisionHandoff`。
- 平台：`outbox.Store`、`inbox.Store`、`dispatch.Dispatcher`、`dispatch.DirectPublisher`。
- NR：`AcceptanceConsumer`、`RouteOnAcceptanceAdapter`、`CreateInitialRouteHandler` 及其 postgres 仓储（`InitialRoutes`、`NetworkDefinitions` 读口、handoff log）。
- 第二跳若做：`NetworkIntakeConsumer` + `IntakeAdoptions` + `ReassessRouteHandler`。

### 只许 SYN 替身（标 S，测试装配）

- `ProductionOwnershipAuthority`
- `ResolutionKeySource` / `AsOfValueSource`（直到 SYN-PC-SEED 换真解析）
- 任何 `PAR-*` 未提供的目录内容（价卡、渠道 Intake、网络定义正文、采用规则版本）
- `AdoptedStageOwner`（V0 不测收寄资格）

### 禁止

- 测试里直接调领域 `SubmitShipmentRequest` / `Decide` / `CreateInitialRoute` 的纯函数路径，却声称「Dispatcher 闭环」。
- 把 SYN 映射写进 `cmd/parcel-dispatch` 生产 `nil` 缝。
- 把 SYN Intake 写进 `assembleBusinessEndpoints`。
- 用 `SKIP` 的 PG 测试当绿（CI 与本机门禁：无 DSN 本机跳过、CI 必须 FAIL）。
- 证据层级写成 `R` 或 `P`。

---

## 6. 与旧清点的关系

`.scratch/outbox-handoff-consumption-map/report.md` 取证于更早 HEAD，**判据栏仍可用**；「已有消费者 = 1」「路由表一条」已过期。以 **1fff679** 重取证：消费者 **2**、路由表 **2**、handoff **46** 未变。第三条「只差接线」的方向在 `next-consumer-survey.md` 已判空，1fff679 仍空——反查口未做。

开发主线「机制半边现状」仍写「唯一消费者 / 路由表一条」，与 1fff679 代码不一致。本设计**不改**那份基线。

---

## 7. 建议首批开工（2–3 票）

1. **SYN-V0**：纵向第一跳进程测试（真 PG + Outbox + Dispatcher + AcceptanceConsumer + 诚实未决）。这是唯一立刻能证「不是直接调领域函数」的票。
2. **PS-PARCEL-INDEX**：按包裹反查来源身份。做完之后收寄/交付四条才可能退化成「接线」。与 V0 无代码依赖，可另一会话并行；占号 `migrations/parcel_shipment/`。
3. **SYN-PC-SEED**（若 V0 用了商业替身）：把唯一解析换成真 PC 登记册种子，去掉 V0 里最容易被误读成「闭环」的那一处替身。

**不要**把 CONS-INTAKE 或 HTTP-SYN-INTAKE 放进第一批：前者被反查口挡住，后者会诱惑人改生产装配。
