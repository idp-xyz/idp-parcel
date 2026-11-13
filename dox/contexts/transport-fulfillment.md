# 运输履约（`transport-fulfillment`）

代码在 `internal/transportfulfillment`。对照 main `99fc0037d`（2026-11-12）写成。

**这份文件核到什么程度**：取值表、门禁读数的五格与停点、入口三节是从代码里读出来的。「词」「规则」「生命周期」的句子是从旧 `docs/domain/transport-fulfillment/CONTEXT.md` 提炼的，每条只用 grep 确认了代码里有对应的类型、函数或结果码，没有逐条跑测试验证行为；旧文档写了而代码里没有的，单列在倒数第二节。

## 管什么，不管什么

管：场外揽收、节点间运输、末端派送三种运输动作里的控制事实（场外揽收、实际承运商首次有效收寄、权威运输交接、有效交付），实际履约段与履约参与关系，实际承运商判断，运输委托、订舱与承运答复，周期安排、具体班次、容量池、容量预占与装载分配，实际移动与三种门禁读数，运输收费发生项，总单与外部承运凭证，外部承运轨迹事实，监管运输处置承接，替代与退运旅程的起点。

不管：

- 客户委托、包裹身份、取消与客户服务终局——`parcel-shipment`。本上下文只提供控制事实。
- 线路、路由计划、计划履约段、时间窗口、改路——`network-routing`。本上下文不改计划。
- 集运单元与成员关系、节点收寄、节点内控制与装卸——`node-operations`。
- 监管决定、放行、门禁判断本身——`customs-compliance`。本上下文只拉读数、只承接要求移动的协作事项。
- 参与方身份、供应商协议及其版本——`party-commercial`。本上下文不为承运方铸身份。
- 价格、预期成本、应付——`parcel-pricing` 与 `settlement-accounting`。发生项不是金额。
- 客户可见全程状态、异常案件——`visibility-exception`。代收分户账——`collection-remittance`。

## 词

| 词 | 意思 |
|---|---|
| 载运对象 | 可独立分配、交接、参与运输的实物范围：包裹或集运单元。只引用来源上下文的身份 |
| 实际履约段 | 由有效收寄或权威交接形成的共同运输控制责任范围。不是计划履约段、班次、委托、订舱或总单 |
| 履约参与关系 | 一个载运对象参与一个实际履约段的对象级关系：计划段、控制起止、依据。逐对象成立、结束、更正 |
| 段服务动作 | 登记方在段成立时声明的动作。只声明，不推导；未声明也是一种答案。声明为末端派送的段即派送段 |
| 实际承运商判断 | 以实际履约段为单位、带版本的「谁在承运」判断。待确认是有起止的版本，不是缺席 |
| 实际承运商首次有效收寄 | 载运对象首次进入某实际承运商运输控制的独立控制事实，经显式判断形成，带版本 |
| 权威运输交接结果 | 逐载运对象的跨边界控制转移判断。可声明交出节点、接收节点、去向节点，三格只声明不推导 |
| 有效交付结果／交付证明 | 一次派送尝试满足规则后对明确对象形成的交付与控制转移；交付证明（POD）是支持它的证据集合 |
| 派送待形成 | 派送段里某对象的收件地点、时间窗口或交付条件，有所有者答了「没有」，任务没形成，记在派送待形成册上 |
| 运输委托 | 向外部运输服务提供方提出的业务请求，引用供应商协议与条件快照。不是订舱、承运接受或实际履约 |
| 订舱／承运接受 | 对班次或容量范围的申请；承运接受就是订舱上答复为`接受`的那一值，带接受量 |
| 承运答复挡回 | 承运方交来的答复没被收下（订舱不在册，或答复方不是委托的服务方），对外同答`订舱不在册`，依据记在挡回册 |
| 周期安排／班次 | 周期安排是运行规律；它在一个运行日期上物化出的具体班次才能被订舱、分配、移动引用 |
| 容量池／容量预占／装载分配 | 池是一班在一个维度上的运力与硬上限；预占保留容量；装载分配是把对象排到班次上的意图 |
| 运输收费发生项 | 已经发生、可能按供应商协议形成成本的业务事实范围。不是价格、成本或应付 |
| 总单／外部承运凭证 | 总单是与外部服务方之间的主运输凭证，带版本；外部承运凭证是对方给委托、订舱、班次、对象或段分配的标识 |
| 轨迹源／外部承运轨迹事实 | 轨迹源报来原始素材；本上下文认领后才成为事实。它是来源事实，不是收寄、交接、移动或交付 |
| 出发／交出／交付门禁读数 | 出发、`已交接`、有效交付落册时同一笔保存的「当时关务怎么答」。是拉来的记录，不是拒收理由 |

## 取值表（代码里的封闭集）

| 轴 | 取值 |
|---|---|
| 段服务动作 | `OFFSITE_PICKUP` `LINEHAUL` `FINAL_DELIVERY`（另有未声明） |
| 参与入场依据 | `OFFSITE_PICKUP` `TRANSPORT_HANDOVER` `CARRIER_FIRST_EFFECTIVE_PICKUP` |
| 参与结束依据 | `EFFECTIVE_DELIVERY` `NEXT_HANDOVER` `CONTROL_TERMINATED` |
| 交接结果 | `HANDED_OVER` 已交接 `REFUSED` 已拒收 `PENDING_CONFIRMATION` 待确认 |
| 揽收尝试的对象结果 | `PICKED_UP` `CUSTOMER_ABSENT` `GOODS_NOT_READY` `PACKAGING_UNACCEPTABLE` `EXECUTOR_REFUSED` `PENDING_CONFIRMATION` |
| 派送尝试的对象结果 | `DELIVERED` `REFUSED` `NO_ONE_TO_RECEIVE` `WRONG_ADDRESS` `PENDING_CONFIRMATION` |
| 揽派任务种类／状态 | `PICKUP` `DELIVERY`；`OPEN` `TERMINATED` `COMPLETED` |
| 承运证据来源 | `CARRIER_PICKUP_SCAN` `CARRIER_RECEIPT_VOUCHER` `TRANSPORT_HANDOVER_TO_CARRIER` `TRUSTED_CHANNEL_CALLBACK` |
| 承运商待确认原因 | `NO_QUALIFIED_EVIDENCE` `SOURCE_CONFLICT` `IDENTITY_NOT_REGISTERED` |
| 首次有效收寄结果 | `FORMED` `PENDING`（原因 `SOURCE_CONFLICT` `IDENTITY_NOT_REGISTERED`） `VOIDED` |
| 承运答复 | `ACCEPTED` `REFUSED` `EXPIRED` `WITHDRAWN` `WAITLISTED` |
| 容量维度 | `WEIGHT` `VOLUME` `PIECES` `PALLET_POSITIONS` `BAG_POSITIONS` `CARGO_SLOTS` |
| 运输准备结果 | `NOT_CONFIGURED` `NO_CURRENT_OPPORTUNITY` `CAPACITY_UNCONFIRMED` `OPPORTUNITY_UNAVAILABLE` `OPPORTUNITY_PREPARED` |
| 移动事实种类 | `DEPARTURE` `IN_TRANSIT` `ARRIVAL` |
| 发生项原因 | `BOOKING` `CANCELLATION` `FAILED_ATTEMPT` `ACTUAL_FULFILLMENT` |
| 总单适用状态／修订 | `IN_FORCE` `REVOKED` `SUPERSEDED`；`REVOKE` `SUPERSEDE` `RESTATE_ASSOCIATIONS` |
| 轨迹事实有效时间依据 | `PENDING` `JUDGED_EXPLICITLY` `JUDGED_BY_RULE` |
| 监管处置承接决定 | `ACCEPTED` `PARTIALLY_ACCEPTED` `DECLINED` |

## 门禁读数的五格

三种读数（出发、交出、交付）共用一套答复，逐对象各一格，整条取最重的一格。下表由轻到重。

| 答复 | 什么时候 |
|---|---|
| `NOT_GUARDED` 不受门禁 | 关务答这个动作不受门禁 |
| `ADMITTED` 放行 | 关务答放行 |
| `UNDECIDED` 未决 | 关务答未决，或本侧说不出问谁、自哪到哪（见停点），没去问关务 |
| `UNREADABLE` 读不动 | 关务那一问没形成答复，或本侧某本册子读不动 |
| `BLOCKED` 阻断 | 关务答阻断 |

**读数阻断、未决或读不动都不挡落册。** 挡下违规动作是动作之前那一问的事（出发前、交付前门禁问询）。

本侧停点（读数里点名停在哪）：

- 出发：`SEGMENT_NOT_DECLARED` `SEGMENT_NOT_REGISTERED` `NO_OBJECT_UNDER_CONTROL` `SCHEDULE_NOT_REGISTERED` `SCHEDULE_NOT_SERVING_SEGMENT` `RECORDED_BEFORE_GATE_READING` `SEGMENT_REGISTRY_UNAVAILABLE` `SCHEDULE_REGISTRY_UNAVAILABLE` `HANDOVER_REGISTRY_UNAVAILABLE` `SEGMENT_RECORD_UNREADABLE` `SCHEDULE_RECORD_UNREADABLE`。
- 出发时的集运单元成员：`CONSOLIDATION_UNIT_WITHOUT_MEMBERS` `CONSOLIDATION_MEMBER_IDENTITY_UNRESOLVED` `CONSOLIDATION_MEMBER_IDENTITY_UNREADABLE` `CONSOLIDATION_MEMBERSHIP_CYCLE`。
- 交出：`RELEASING_NODE_NOT_DECLARED` `ONWARD_SAME_AS_RELEASING` `RECORDED_BEFORE_GATE_READING` `HANDOVER_REGISTRY_UNAVAILABLE`。
- 交付：`NO_ACTIVE_SEGMENT` `ENTERED_BY_PICKUP` `RELEASING_NODE_NOT_DECLARED` `ENTRY_HANDOVER_NOT_ON_REGISTER` `ENTRY_HANDOVER_AMBIGUOUS` `RECORDED_BEFORE_GATE_READING` `DELIVERY_VIEW_UNAVAILABLE` `SEGMENT_REGISTRY_UNAVAILABLE` `HANDOVER_REGISTRY_UNAVAILABLE`。

各用例的拒绝与结果码有几百个，分散在 `application/` 各文件的 `String()` 里，本文件不列全。

## 规则

**段与参与**

- 只有场外揽收、实际承运商首次有效收寄、`已交接`的权威交接能让对象进入实际履约段。扫描、订舱确认、承运接受、列入总单、装载分配都不能。
- 参与只以有效交付、下一次权威交接或明确控制终止结束。中断、折返、异常案件不结束控制。
- 一个对象同时只在一个段里在场；库里读出多于一条在场参与是不一致，响亮报错，不挑一个。
- 已成立的段与参与不取消、不删除。证据更正在同一条链上形成替代或失效参与版本，原版不动，不回退到更早的控制版本。
- 段服务动作在段成立时固定；后来的对象给出矛盾声明不被接受。

**实际承运商**

- 段成立即有首个判断版本：有合格证据且身份在册为已识别，否则待确认并带原因。
- 判断以段为单位。同段内证据指向不同承运主体时形成待确认（来源冲突），不逐对象判，也不挑一个。
- 判断版本只追加。段结束后判断封存，只接受来源更正引起的重新派生。
- 对象已在运输控制中时，新到的承运证据不构成首次有效收寄；一个对象至多一条首次有效收寄版本链。待确认的收寄不进段、不提供给 `parcel-shipment`。

**交接**

- 交接逐对象判断，允许部分成立。已交接转移控制；已拒收与待确认保持此前的控制关系。
- 集运单元的交接只对单元本身形成结果与参与，不为成员形成参与。
- `已交接`一律落册，交出门禁读数同一笔保存。同内容重投交回首次读数，不重问。

**委托、订舱、容量、装载**

- 委托的取消是整份的。已开始的委托不能取消；取消不向订舱、预占传递，它们各自经运营决定停下。
- 段在哪份委托下履约由运营决定逐段声明，不推导。一段此刻至多一份当前采用；指错了经解除，只在这一段上还没有按该委托形成仍有效的发生项时允许。解除与再采用都要点名是哪一代。
- 委托下的运输以已采用各段里最早的首个参与为开始；解除后重新派生，一段不剩时撤回到已提交。
- 承运答复候补之后还要一份定局；定局之后再来不同的答复是冲突。接受量不超过申请量。
- 订舱与承运接受不占容量，只有当前有效的预占扣减池的可用量。池的硬上限不可突破。
- 班次登记了容量池时，装载分配必须对每个池引用一份当前有效预占并声明分配量；没登记池的班次不要求。

**班次与运输准备**

- 班次方向是连接代码加起点、终点节点，三格缺一不可。周期安排物化的班次身份由（周期安排代码、运行日期）派生。同一身份再登记，内容相同是已有结果，内容不同成下一版。
- 连接上没有任何当前版的周期安排或班次时，运输准备答`未配置`，不建占位班次。

**收费发生项**

- 发生条件、数量与单位取自委托所凭那一版供应商协议的声明。协议没声明这个原因答`未配置`；声明不计费则不形成。
- 段已采用委托时，对象的参与以下一次交接或有效交付结束就自动形成实际履约发生项；没采用的段不自动形成。
- 采用晚于参与结束或委托换了版时回头补形成；补不成的记在实际履约未形成册上。
- 发生项不删。运营决定失效形成失效版本；失效后同一事实再登形成替代版本。

**揽收、派送、交付**

- 任务与尝试分离；尝试逐对象记结果。待确认是对象的一格结果，不转移控制，不算成功也不算失败。
- 失败的揽收不制造段；失败的派送不结束参与，当前运输方继续承担控制。
- 派送任务的内部触发只有一种：对象凭`已交接`进入派送段。收件地点、时间窗口、交付条件任一条拿不到，任务保持待形成并记进派送待形成册，不用默认值补；补齐后按册重问，到期重问的间隔是租户取值。
- 有效交付必带交付证明，同时结束该对象的参与。有效交付一律落册，交付门禁读数同一笔保存；POD 更正承继前版读数，不重问。

**实际移动与出发门禁**

- 出发、移动、到达照实落册。移动事实带登记方声明的段与运输资源，都是引用；移动不成立也不结束段。
- 出发读数问的对象取声明的段里出发那一刻在控的当前参与，不经装载分配推。
- 在控对象是集运单元时按出发那一刻的直接成员逐层展开到包裹，单元那一格取成员里最重的一格。交出读数相反：按单元本身问。
- 出发时没有定论的对象，由它此后的控制事实（或成员日后识别成功）按原出发时点补问；同一条出发替同一件对象至多补问三次，首版不改。

**轨迹事实、单证、监管处置**

- 认领不解释原始状态词。有效时间只来自逐条显式判断或该轨迹源已登记的规则；两者都没有时保持待判断，不提供给 `visibility-exception`。
- 总单一份只有一个首版；撤销、替代、关联重述都是新版本。已撤销或已替代的总单不再接受新版本。
- 只有要求实际移动的协作事项才形成承接决定；原地处置不承接。缺当前有效移动授权时答待补充，不落承接记录。

## 生命周期

- **参与**：控制事实入场 →（证据更正：替代或失效版本）→ 以有效交付、下一次交接或控制终止结束。段在全部参与结束后由运营关闭。
- **实际承运商判断**：段成立出首版（已识别或待确认）→ 新证据追加版本 → 段结束封存。
- **委托**：已提交 →（换版，可多次）→ 运输已开始（采用只前移开始；解除可后移或撤回到已提交）；未开始时可整份取消。
- **订舱**：已申请（读作待确认）→ 候补 → 定局（接受、拒绝、失效、撤回）；或由运营取消。
- **交接判断**：待确认 → 已交接或已拒收；任何更正都是新版本。**发生项**：已登记 → 失效 → 替代。
- **其余**：班次、周期安排、总单、外部凭证、轨迹事实、首次有效收寄都是版本链，新版本回指前版，原版不动；揽派任务 `OPEN` → `COMPLETED` 或 `TERMINATED`；监管处置待承接 → 承接决定三值之一。

## 入口

在线口挂在 `cmd/parcel-api`，处理器在 `adapters/http`。路径带斜杠的是执行方与集成客户端报事实的口，带连字符的是运营决定、登记与查阅口。

| 做什么 | 路径 |
|---|---|
| 揽收与交接（执行方） | `/transport-fulfillment/offsite-pickup-attempts` `/offsite-pickups` `/offsite-pickup-corrections` `/offsite-pickup-attempt-result-corrections` `/handovers` `/handover-corrections` |
| 派送与交付（执行方） | `/transport-fulfillment/delivery-attempts` `/deliveries` `/delivery-proof-corrections` `/delivery-attempt-result-corrections` `/delivery-gate-previews` |
| 移动（执行方） | `/transport-fulfillment/movement-facts` `/departure-gate-previews` |
| 承运答复（集成客户端） | `/transport-fulfillment/booking-answers` |
| 委托、订舱、发生项 | `/transport-fulfillment-transport-commission-submissions` `-transport-commission-cancellations` `-transport-commission-agreement-revisions` `-booking-request-submissions` `-booking-request-cancellations` `-charge-occurrence-registrations` `-charge-occurrence-invalidations` |
| 段 | `/transport-fulfillment-segment-commission-adoptions` `-segment-commission-adoption-withdrawals` `-segment-closures` `-participation-terminations` |
| 容量与装载 | `/transport-fulfillment-capacity-reservations` `-capacity-releases` `-load-assignment-registrations` |
| 揽派任务与派送发起 | `/transport-fulfillment-dispatch-task-registrations` `-delivery-dispatch-triggers` `-delivery-dispatch-reasks` `-pickup-attempt-result-confirmations` `-delivery-attempt-result-confirmations` |
| 判断、监管处置、旅程 | `/transport-fulfillment-carrier-first-effective-pickup-judgments` `-effective-time-judgments` `-regulatory-disposition-acceptances` `-alternate-journey-starts` |
| 登记册写口 | `/transport-fulfillment-transport-schedule-template-registrations` `-transport-schedule-registrations` `-capacity-pool-registrations` `-carrier-master-document-registrations` `-carrier-master-document-revisions` `-external-carrier-credential-registrations` `-external-carrier-credential-applicability-changes` `-effective-time-rule-registrations` `-delivery-dispatch-reask-cadence-registrations` |
| 查阅 | `/transport-fulfillment-records`（`registry=` 取下列册名） `-handover-scope-summary` `-external-tracking-facts` `-carrier-first-effective-pickups` |
查阅的册名：`transport-schedule` `transport-schedule-template` `capacity-pool` `transport-handover` `effective-delivery` `carrier-master-document` `transport-preparation` `delivery-dispatch-pending` `delivery-dispatch-reask-cadence` `actual-fulfillment-unformed` `booking-request` `booking-answer-bounce` `pending-attempt-result`。

离线口：`cmd/parcel-transport-register`（周期安排、班次、容量池三本册子的受控登记，运营在数据库网络内手跑，不监听端口；命令 `schedule-template` `schedule` `capacity-pool`）。

后台节拍在 `cmd/parcel-dispatch`（各上下文共用的分发进程）。本上下文挂在上面的有：随交接发起派送任务、参与结束时形成实际履约发生项及其补形成、出发门禁补问（随控制事实、随成员识别）、初始路由形成后的运输准备。

## 旧文档写了、代码里还没有的

- **运输舱单**（草稿、提交、接受、拒绝、更正版本，实际装载与舱单比较出短装、多装、错装）：搜 `manifest`、`舱单`，只有注释；查阅口的注释写明「运输舱单在存储上仍没有登记册」。
- **履约侧代收证据、渠道代收报告、渠道回款通知**：搜 `代收`、`COD`、`CashOnDelivery`、`回款`，没有类型也没有入口。
- **软容量超配、超配缺口与容量缩减处置**：搜 `超配`、`overbook`、`缺口`，只有注释写明「另票」。代码里只有硬上限。
- **班次的执行准备判断**（开放或关闭订舱、暂停、延误、取消、替代班次）与**班次完成的派生**：搜 `暂停`、`延误`、`替代班次`、`ScheduleCancel`、`班次完成`，没有；读口注释写明这些「在存储上还没有登记格」。班次只有登记与换版。
- **运输资源的分配按版本保存**：搜 `ResourceAssign`、`资源分配`，没有。运输资源只作为移动事实上的一个引用。
- **实际装载量作为独立事实**与分配比较：搜 `实际装载`、`ActualLoad`，只有注释说它该走移动事实那一口；移动事实种类只有出发、在途、到达。旧文档写的中断、折返、改降等实际结果也没有对应的事实种类。
- **协作事项取消、替代或缩小范围时停止未消耗的委托、订舱、预占与分配**：搜 `协作事项取消`、`ItemSuperseded`、`ScopeNarrow`，没有联动；只有「已发生的事实保留」这一半由版本链守住。
- **交付方式的允许集**（安全投放、智能柜、邻居代收只在产品与合同允许时成立）：代码只保存交付方式引用与交付条件引用，不读条件内容、不校验允许集。

## 证据限制

轨迹源素材的认领编排（`AdoptTrackingMaterialHandler`）与有效时间规则都在，但认领没有生产入口：`adapters/trackingsource` 下只有一份合成替身，真源的拉取节拍还没立。外部承运轨迹事实这一条链目前只有 `S` 证据。
