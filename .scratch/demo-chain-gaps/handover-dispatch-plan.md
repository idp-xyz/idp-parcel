# 交接之后与派送发起：统一代决与落地计划（收 04、06 与 TF 链「等 peer 的」各项）

钉 `origin/main 6b02b547`（2026-10-19）。授权：用户原话「你作为业务和系统专家自决……解决所有的问题」。下文各条都是会话代决，越权风险点留给各 owner 复核。实测证据一律记 `S`。

本文件是这批代决的单一权威，各票落地时把对应裁定写进各自 ADR（0213–0219，已占号），ADR 接受后本文件只作索引。

复核用的是 `origin/main` 的导出副本：分诊当时本地 `main` 落后于 `origin/main`，TF 链第三波（18、20、24、29、33）只在后者上。新票号接在第四波预留号之后，从 59 起；新 ADR 号从 0213 起（`git ls-tree origin/main docs/adr/` 核过，0204 留给在途的 [25](./issues/25-segment-adopts-transport-commission-and-cancellation-faces.md)）。

---

## 复核

### 04：现象成立；票面三处过时，另漏两条真阻断

**仍成立**

- 交接信封只投 VE：`cmd/parcel-dispatch/assemble.go` 路由表里 `veinbox.TransportHandoverRegisteredEventType` 只挂 `veHandoverRouted`；`veHandoverUndecidedSentinels` 的注释仍写「NO/NR 控制转移不在本票」。
- `internal/nodeoperations/adapters/transportfulfillment` 的 `ControlTransferAdapter.TransferOut` 没有非测试调用，对身份仍靠 `handover.Object().String() != control.Unit().String()` 字符串比对。
- 节点控制只有收寄一个生产来源：`application/receive_delivered_unit.go` 以 `EstablishedByNodeIntake` 立控制；`domain.EstablishedByHandoverIn` 没有生产方；`PhysicalControl.TransferOut` 只在读回重放 `postgres.rebuildControl` 里出现；`ReceptionStore` 没有转出写口。
- UC-NO-001、UC-NO-002、UC-NO-003 仍把权威交接排除在外。
- TF 的设计意图没兑现：`ports.TransportHandoverRegistrationIntent` 的注释写着 NO 做控制转移、NR 按 `TransportHandoverControl` 重判。

**更正**

- 「NR 已能处理 `TransportHandoverControl`」要收窄：今天唯一产出这一格的，是 `nrparcelshipment.controlKindFor` 把 PS 的 `OffsitePickupSource`（场外揽收）译过去；节点或承运方的交接从没到过 NR。
- [勘察报告](../no-control-transfer-out/report.md)「Q3 提供方那一侧的可重读性」里「从（租户 + 实物）读不回当前在身的控制」已过时：`Receptions.FindCurrentJudgmentsAtNode` 与 `FindUnitIntakes`（ADR-0199、ADR-0210）已能按实物读。同报告「Q4 可排期的判断」说的「NO 的第一个 inbox 消费门」也过时：`internal/nodeoperations/adapters/inbox` 已有身份重核消费者。
- UC-NO-004 已被身份冲突处置占用；[no-reception-identity/04](../no-reception-identity/issues/04-identity-conflicts-have-a-disposition-and-release.md) 已把 UC-NO-005 留给本事。
- 开发主线「机制半边现状」PN-04 一行写「交接→控制转出」适配器已接通，与代码不符。

**票面漏掉的真阻断**

- 交接记录没有节点维：`TransportHandoverSpec` 的 `ReleasedBy` / `ReceivedBy` 是不透明参与方引用，`Scope` 也不透明。NO 立 HANDOVER_IN 不知道是哪个节点，NR 复核没有 `ActualLocationReference`，ADR-0206 的交出与交付意图（`NewHandoverIntent(at, onward)`、`NewDeliveryIntent(at)`）也都要节点。
- NR 复核没有地理投影可用：`ReassessRouteCommand.Carried` 的注释写明空着时「只能答两端未知、复核停在看不清」；NR 只存投影摘要，ADR-0075 决定一禁开读口。

**副作用**

- 控制从不转出，`application.currentIntakeAmong`（`receive_same_unit.go`）就一直把那条旧收寄认作当前收寄；包裹日后退运回原节点，会被并进这条陈旧收寄。

### 06：现象成立；一处措辞更正，一处升级

**仍成立**

- `TriggerDeliveryDispatchHandler` 只在 `cmd/parcel-api/assemble_delivery_dispatch.go` 的 `deliveryDispatchTriggerOver` 里装配，挂在作业事实口 `/transport-fulfillment-delivery-dispatch-triggers` 后面；`cmd/parcel-dispatch` 没有任何派送消费者。`buildDeliveryDispatchTrigger` 头注仍说「谁按拍调这一口、拍频多大，是租户运营节拍的实例半边」。
- NR `application/calendar_projection.go` 的 `projectCandidateTime` / `plannedLeg` 只按连接成段。

**更正**

- 演示计划第三段是 SIN-HUB→SIN-LM（连接 `SYN-CONN-SIN-LM`），不是干线段。`851e9da7` 那次重走声明了它，答 `DISPATCH_TASK_FORMED`：问题在语义（到达交付节点的窗口冒充派送窗口），不是停点。

**升级**

- 不只是演示问题：NR CONTEXT 写明「网络服务的路由计划覆盖运营企业责任范围内的揽收、始发、转运、关务衔接、干线、尾程注入和交付段」，而计划生成从不出交付段——NR 机制缺口。
- 牵连 TF 运输准备消费者（`internal/transportfulfillment/adapters/networkrouting/planned_leg_source.go` 的 `connectionOf`）：不指名连接的段被判为译不出，信封成毒丸。加交付段必须同笔让它跳过。

**「等 peer」理由已失效**：[tf-face-algebra/09](../tf-face-algebra/issues/09-dispatch-trigger-task-reference-is-a-faithful-identity.md) 已 resolved；它新增的答格 `DISPATCH_TASK_REFERENCE_TAKEN`，新消费者要认。

### [tf-chain-plan](./tf-chain-plan.md)「等 peer 的」各项现状

- **交出门禁、交付门禁**：CC 一侧已就绪（[31](./issues/31-customs-action-admission-judgement-and-node-customs-zones.md) 的 `JudgeActionAdmissionHandler`）；TF 的 `register_transport_handover.go`、`register_effective_delivery.go` 都没接。阻断同上：没有节点。
- **自动形成 ACTUAL_FULFILLMENT**：[29](./issues/29-supplier-agreements-declare-charge-occurrence-conditions.md)（ADR-0209）已进 main；[25](./issues/25-segment-adopts-transport-commission-and-cancellation-faces.md) 未落；[28](./issues/28-booking-and-cancellation-occurrences-and-procurement-context-from-adoption.md) 未开。另有方案没提的缺口：`JourneyReference` 必填，而 ADR-0207 只给了「登记方声明」一个来源，自动路径无从取值。
- **`gateRequired`**：仍在 `RecordMovementFactCommand` 与 tfhttp `record_movement_fact.go` 载荷里，收而不读；operator-channel/32 已 resolved，不再算等 peer。注意 tfhttp `command_payload.go` 用 `DisallowUnknownFields`：直接删载荷键，仍发它的设备会收到 400，离线队列可能丢一次出发。`gateClearance` 按 ADR-0205 决定三保留，不删。
- **NO 实测触发的再准备**：已立为 [44](./issues/44-tf-re-prepares-on-node-actual-measurement.md)。

---

## 代决

| # | 待裁点 | 裁定 | 理由 | 越权风险点 | owner |
|---|---|---|---|---|---|
| C1 | 交接怎样指名节点 | 交接登记加三个可选声明：`releasingNode`、`receivingNode`、`onwardNode`。存 NR 节点代码，不透明，不核目录、不核设备范围；进内容摘要（出新形状版本，旧行按旧版比）；更正承继前版；缺席是一种答案 | ADR-0096：声明，不推导。ADR-0206 的意图必须带节点；从参与方或范围里解析节点就是推导 | 声明错了没人核；节点与参与方两套指称并存 | TF（NO、NR、CC 会签） |
| C2 | 用哪个 UC | 新立 UC-NO-005「按权威运输交接推进节点控制」，转出与转入同一用例；UC-NO-002、UC-NO-003 不动 | UC-NO-004 已占 | — | NO |
| C3 | 交接对象怎样对上作业实物 | 按正式包裹身份，用 NO 自己册上的当前关联（链头，与 `ParcelContainmentView` 同一读法）解析，去掉字符串比对。恰一件则推进；零件或多件入账不推进；对象是本租户集运单元时入账「待单元控制规则」 | 作业实物不是包裹身份；部分交接要逐对象 | 包裹号与集运单元号同串时按集运单元判；集运单元交接暂不推进 | NO（TF 会签） |
| C4 | 控制转移存在哪 | 新立只追加的「节点控制转移册」，记转出与 HANDOVER_IN 成立两种行，键为（租户、作业实物、交接键）。收寄行的 `control` 列不改写，读控制时把转出折进收寄链头；与收寄共用同一把实物锁 | 收寄册是只追加的判断链；控制转移不是判断 | 控制状态分在两处，必须经折叠读口读 | NO |
| C5 | 怎样判转出 | `已交接` 即转出该实物此刻唯一在身的控制（收寄成立或交接转入成立都算），时刻取交接业务时间；声明的交出节点与在身控制节点不符时照转、记信息格；拒收、待确认不写 | TF CONTEXT：「若证据已经表明实物占有发生变化……不能继续把已经没有实际保管能力的一方记为当前实物控制方」 | 一次转出可能掩盖漏登的中间交接 | NO |
| C6 | 转入 | 只在 `已交接` 且声明接收节点时，在该节点立 HANDOVER_IN；对不上作业实物就入账，不造待识别实物 | 待识别实物只能由现场观察建立 | 场外揽收直进干线的包裹在节点上没有控制记录（对账另票） | NO |
| C7 | 交接更正 | 后继版仍 `已交接`：幂等不写。撤回控制的更正不自动回退，入账「控制来源链待重算」，查阅口可见 | TF CONTEXT 禁止「直接把当前控制回退给早期交出方」 | 人工处置面未立 | NO、TF |
| C8 | NR 是否每次交接都复核 | 否。只对「`已交接` 且声明接收节点」复核：控制证据 `TransportHandoverControl`，位置取接收节点，来源版本与业务时间取交接。其余裁决、交给运输方（无接收节点）、对象不是当前已接受包裹，一律入账不复核 | NR CONTEXT 可控节点只认节点收寄或权威交接；交给运输方之后没有可控节点 | 外包段内部异常不触发复核；集运单元不逐成员复核 | NR |
| C9 | 复核用的地理投影从哪来 | 交接触发的路径上，NR 经消费侧适配器按（租户、包裹）取 PS 当前已接受委托（`ShipmentRequests.FindCurrentAcceptedByParcel` 与委托聚合），读受理基线那一版提交的 `SubmissionVersion.GeoProjection()`，不读地址本体 | 基线版本不可变，ADR-0075 防的同版性漂移在这里不存在；所取维度与信封所携一致 | NR 首次直接读 PS 委托内容（只到地理维） | NR、PS |
| C10 | 派送发起：事件驱动还是按拍 | 事件驱动：TF 自消费交接信封，按对象此刻在场的段调既有触发执行器，任务成立时间取派发处理时钟。这是 CONTEXT「任务在下一拍形成」的机制落法，不是租户节拍，`PAR-NET-09` 的拍频格退役；端点保留作补拍；不出按拍批量形态。形成、已在册、不是触发、引用被占、要求缺失入账不重投；未决登哨兵重投。演示即采用 | ADR-0114 决定二只缺生产入口；一拍一对象已定，没有拍频可选 | 要求缺失后不会自愈，要人补拍 | TF |
| C11 | 派送窗口从哪来 | NR 计划在交付节点之后带一个交付段：起点交付节点；终点交付范围（解析收件地理所用的服务区域版本）；窗口 = [离开交付节点的时刻, + 派送时长]，派送时长是节点服务日历新格（租户取值，演示登 SYN 值）。没登就不成段，TF 窗口缝照 ADR-0131 决定三答 `REQUIREMENT_MISSING`。段带种类（节点间 / 交付），已执行前缀与 TF 运输准备只取节点间段。登记方照旧声明 `#N` | NR CONTEXT 计划覆盖交付段；ADR-0114 禁止从段位置推「尾程」，所以段引用仍由登记方声明 | 总窗口变长影响时效可行性（演示无时间承诺）；交付段责任方暂指服务区域版本 | NR（TF 会签） |
| C12 | 末端自营还是外包 | 维持 [tf-chain-plan](./tf-chain-plan.md)「跨票通则」X1：演示末端自营，末端协议继续闲置 | — | — | 演示租户 |
| C13 | 外包干线交接的跨境门禁 | 写事务之前，对 `已交接` 逐对象按交出意图（at = 交出节点，onward = 去向节点）问 CC，读数随交接同笔照记（与 ADR-0205 同一封闭集），不拒收。没声明交出节点：记未决、不问 CC。没声明去向：照问，读数里写明 | ADR-0206 Consequences 已定挂法，只等节点 | 漏声明去向时跨区那一道不列 | TF |
| C14 | 有效交付的 FINAL_DELIVERY 门禁 | 有效交付按交付意图问 CC，at 取对象进入该派送段那次交接的交出节点，照记。凭收寄进段或未声明节点：记未决「交付节点不可得」 | at 取 TF 自有事实，不另增声明 | — | TF |
| C15 | 自动形成 ACTUAL_FULFILLMENT | 做，只对已采用委托的段：自消费交接与有效交付信封，找出被这次事实结束的参与，经段采用取采购上下文（[28](./issues/28-booking-and-cancellation-occurrences-and-procurement-context-from-adoption.md)），走既有登记用例。没有采用连线不形成、不推断自营；协议未声明条件答 `未配置` 入账。旅程（产品策略）：取对象在 TF 替代 / 退运旅程册上的当前旅程，否则按正式包裹身份确定性派生 `JRN/<包裹>`；人工登记同一身份答已存在 | 回答 ADR-0207 越权风险点「不自动形成」 | 演示约定 `SYN-JRN-<包裹号>` 会与派生拼写撞成员交集，演示约定要改 | TF、SA |
| C16 | `gateRequired` 删不删 | 从应用命令里删；HTTP 载荷照收照弃，作退役键注明；`gateClearance` 保留 | 严格解码下删键，离线设备会 400 并丢出发（违「已发生的物理事实照记」） | 退役键永远留在载荷形状里 | TF |
| C17 | 设备出发前门禁读口 | 作业事实渠道开只读问询，按（段、班次、拟出发时点）交回与出发登记同一套读数；TF 不落记录，CC 照 ADR-0206 决定六记判断；持移动事实登记授予即可问，不新增能力种类 | 回答 ADR-0205 越权风险点「挡下违规出发是出发前读口的事」 | 用写授予开读口；读到放行不等于出发那一刻仍放行 | TF、CC |

---

## 需要的 ADR

状态格写法沿用「Accepted（会话代决）」，越权风险点待 owner 复核。已接受 ADR 的正文不动，只在它的 Links 加一行后继指针，并在 README 状态格注明「部分停用」。

| 号 | 题 | owner | 票 | 对已接受 ADR 的作用 | 要点 |
|---|---|---|---|---|---|
| 0213 | 权威交接声明交出、接收、去向节点；控制转移、路由复核与交出 / 交付门禁的位置都取这份声明 | TF（NO、NR、CC 会签） | 59、65、66 | 回答 ADR-0206 Consequences「挂法等 peer」，加后继指针 | C1、C13、C14 |
| 0214 | 节点控制按权威运输交接转出与转入（UC-NO-005） | NO（TF 会签） | 61、62 | 无停用 | C2–C7 |
| 0215 | 交接触发的路由复核，及受理后所携地理投影的取法 | NR、PS | 63 | 部分停用 ADR-0075 决定一（限受理后、非 PS 信封触发的复核） | C8、C9 |
| 0216 | 派送发起随交接信封在进程内逐对象发起 | TF | 60 | 回答 ADR-0114 决定二末句，加后继指针；`PAR-NET-09` 拍频格退役 | C10 |
| 0217 | 路由计划带交付段 | NR（TF 会签） | 64 | 在 ADR-0131 决定一的引用形之内，加后继指针；ADR-0201 的运输准备只取节点间段 | C11 |
| 0218 | 实际履约发生项由交接与有效交付信封自动形成 | TF、SA | 67 | 部分停用 ADR-0207 决定七后半句与其越权风险点「不自动形成」 | C15 |
| 0219 | 设备出发前门禁问询 | TF、CC | 69 | 回答 ADR-0205 越权风险点「设备出发前读口」 | C17 |

C12 是演示租户取值，写进演示动线即可。C16 不另立 ADR：落在 ADR-0205 决定三之内，在它的复核记录里加一行。

---

## 拆票表

所有演示都用一次性库：照 `seed.sh` 原样灌，起 `parcel-api` 与 `parcel-dispatch`，证据记 `S`，走完删库。

| 号 | 标题 | Blocked by | 规模 |
|---|---|---|---|
| [59](./issues/59-handover-declares-releasing-receiving-and-onward-nodes.md) | TF 权威交接声明交出、接收、去向节点（ADR-0213） | 无（TF 规范化令牌排在 [21](./issues/21-load-assignment-allocates-against-capacity-reservations.md) 之后） | M |
| [60](./issues/60-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope.md) | TF 派送发起随交接信封进程内逐对象发起（ADR-0216） | 无 | M |
| [61](./issues/61-node-control-transfers-out-on-an-authoritative-handover.md) | NO 节点控制按权威交接转出（ADR-0214，UC-NO-005 上半） | 无（需与 idp-parcel-9b 协调） | L |
| [62](./issues/62-node-control-is-established-by-handover-in.md) | NO 节点控制按权威交接转入 HANDOVER_IN（UC-NO-005 下半） | 59、61（需与 idp-parcel-9b 协调） | M |
| [63](./issues/63-network-routing-reassesses-on-a-handover-into-a-node.md) | NR 交接触发路由复核（ADR-0215） | 59 | M |
| [64](./issues/64-route-plan-carries-a-delivery-leg.md) | NR 路由计划带交付段，TF 运输准备跳过它（ADR-0217） | 无（种子令牌） | L |
| [65](./issues/65-handover-records-an-outbound-release-gate-reading.md) | TF 交出门禁：已交接逐对象读交出准入并照记 | 59 | M |
| [66](./issues/66-effective-delivery-records-a-final-delivery-gate-reading.md) | TF 交付门禁：有效交付读 FINAL_DELIVERY 准入并照记 | 59、65 | M |
| [67](./issues/67-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) | TF 实际履约发生项随交接 / 有效交付信封自动形成（ADR-0218） | 25、28 | M |
| [68](./issues/68-movement-command-retires-gate-required.md) | TF 移动命令退役 `gateRequired` | 无 | S |
| [69](./issues/69-devices-preview-the-departure-gate-before-departing.md) | TF、CC 设备出发前门禁问询（ADR-0219） | 无 | M |

地盘与完成判据以各票为准，这里不复写。

**原票处置**

- [04](./issues/04-handover-does-not-move-node-control-or-trigger-reassessment.md) 改「拆分」，指向 59、61、62、63。
- [06](./issues/06-delivery-dispatch-has-no-in-process-trigger-and-no-recipient-leg.md) 改「拆分」，指向 60、64。
- [psb/09](../product-strategy-boundary/issues/09-tf-fulfillment-judgment-methods-and-connectors.md)「派送发起策略」一项、[psb/05](../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md)「格 13 · 派送发起」由 60 收口。

---

## 波次与令牌

**波次**

- A：59、60、64、68，可并行。
- B：61（等 idp-parcel-9b 的 [ps-external-mark-relations/11](../ps-external-mark-relations/issues/11-identity-check-judges-the-verifiable-delivery-scope.md) 定下 `reception.go`，或事先约好挂点）、63、65。
- C：62、66、67（在 25、28 之后）、69。

**串行令牌**（沿 [tf-chain-plan](./tf-chain-plan.md)「并行车道」的令牌定义，热点文件只追加行）

- **派发令牌**（`cmd/parcel-dispatch/assemble.go`、`assemble_test.go`）：60 → 61 → 63 → 67；与 idp-parcel-9b 的 [ps-external-mark-relations/12](../ps-external-mark-relations/issues/12-handover-notes-are-opened-filled-closed-and-confirmed.md) 门铃路由行串行。
- **TF 规范化令牌**（`domain/payload_canonicalization.go`）：59 排在 21 之后。
- **装配令牌**（`cmd/parcel-api/endpoints.go`、`main.go`）：69 排在 25 之后。
- **种子令牌**（`scripts/demo-seeds/seed.sh` 与 `data/`）：64；落地后把 SHA 通知 idp-parcel-9b。
- **文档令牌**（TF CONTEXT 与 UC-TF-005、UC-TF-006）：59 → 60 → 65 → 66 → 67 → 68 → 69。
- 迁移号、ADR 号认领时占号；`docs/product/MECHANISM-INVENTORY.md` 每次落地重生成。

---

## 「等 peer 的」各项去处

对 [tf-chain-plan](./tf-chain-plan.md)「等 peer 的」一节逐行：

| 事项 | 去处 |
|---|---|
| 04 交接后节点控制转移，NR 复核 | 59 + 61 + 62 + 63 |
| 06 派送发起与末端段 | 60 + 64；末端是否外包维持 X1（C12） |
| 外包干线交接的跨境门禁 | 65 |
| 有效交付的 `FINAL_DELIVERY` 门禁 | 66 |
| 删 `gateRequired` / `gateClearance`；设备出发前门禁读口 | 68（`gateClearance` 不删，C16）；69 |
| NO 实测触发的再准备 | 已有 [44](./issues/44-tf-re-prepares-on-node-actual-measurement.md)（读 NO 端口，同样要与 idp-parcel-9b 协调） |
| TF 自消费交接 / 有效交付信封，自动形成 `ACTUAL_FULFILLMENT` | 67 |
| admin-web 各页 | 前端切片（`apps/admin-web/**` 走前端直做流程） |
| 19 与 `buildSegmentOperations` 同函数相邻 | 19 已进 main，不再成立 |
| `cmd/parcel-api/assemble_operator_decisions.go` 与收寄接线相邻 | 本方案各票都不碰 |

---

## 后继（落地时再立，不占号）

- 集运单元交接：单元控制与成员「随单元受控」；NR 逐成员复核。
- 交接与交付门禁的阻断读数派生 VE 异常信号，与 [52](./issues/52-departure-gate-signal-rule-has-no-online-registration-or-catalogue-read.md) 同一套信号规则。
- HANDOVER_IN 对不上作业实物时的节点对账。
- 控制来源链重算的处置面。

---

## 与 idp-parcel-9b 的协调点

- **61、62 落在 `internal/nodeoperations/**`**，以新文件为主，必碰的共享点：
  1. `adapters/postgres/reception.go` 读控制处的那一次折叠——9b 的 ps-external-mark-relations/11 在改这个文件；要么等它进 main，要么事先约定 61 只在其中加一个辅助函数调用。
  2. `migrations/node_operations/` 的号——9b 的 ps-external-mark-relations/14 已占一号，11 可能再取；认领时现取。
  3. NO 查阅口注册新册的那一行（`adapters/postgres/review_catalogue.go` / `ports/catalogue_read.go`）——9b 的 11 也在改。
  4. NO CONTEXT「节点控制」一节与 UC-NO-005——9b 正在改「送站交接单」相关段落；本方案一律写全称「权威运输交接」，与「送站交接单」区分。
- `adapters/inbox/`：9b 的 12 也在同一包里加新文件，冲突低。
- 派发令牌、种子令牌见上节。
- 不碰：`cmd/parcel-api/assemble_reception*.go`、`assemble_operator_decisions.go`、PS 外部标识读口（63 读的是 `ShipmentRequests.FindCurrentAcceptedByParcel` 与委托聚合，不是外部标识口）。
- 69 已裁不改 access-identity 的能力种类（C17），不撞 9b 的 12。
- 另两条在途：`claude-dcg21` 持 TF 规范化令牌，59 排其后；`claude-dcg25` 持决定种类令牌与 `endpoints.go`，67、69 排其后。
