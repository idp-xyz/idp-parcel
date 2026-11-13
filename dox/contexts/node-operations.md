# 节点作业（`node-operations`）

代码在 `internal/nodeoperations`。对照 main `99fc0037d`（2026-11-12）写成。

**这份文件核到什么程度**：取值表、收寄的结果、各口的应答码、入口四处是从代码里读出来的。「规则」与「生命周期」的句子是从旧 `docs/domain/node-operations/CONTEXT.md` 提炼的，每条只确认了代码里有对应的机制（类型、函数或码），没有逐条跑测试验证行为；旧文档写了而代码里没有的，单列在倒数第二节。

## 管什么，不管什么

管：客户直接送站的节点收寄与待识别实物，身份核对的判断链、重核与身份冲突处置，送站预报、送站交接单与可验证交付范围，节点控制的成立与转移，集运单元实例及其成员、封装快照，对关务执行协作事项的承接决定与执行事实。

不管：

- 委托、正式包裹身份、外部标识关联、服务结果——`parcel-shipment`。节点只问它要候选，不自行定身份。
- 物流节点的网络身份、路由计划、路由指令，以及是否构成路由偏离——`network-routing`。
- 场外揽收、节点与运输方之间的权威交接结果、出发到达——`transport-fulfillment`。节点只消费「已交接」来推进控制。
- 关务执行协作事项本身、监管决定与处置执行核对——`customs-compliance`。
- 谁可以替哪个客户账户送站的授权交付关系——`party-commercial`。节点只引用。
- 异常案件、损坏原因与责任——`visibility-exception`。计价重量与计费重量——`parcel-pricing`、`settlement-accounting`。
- 商品库存、可用量、库位、补货、拣货。节点作业不是 WMS。

## 词

| 词 | 意思 |
|---|---|
| 作业实物 | 节点手上的一件东西，按本租户内的作业实物标识认；外部标识只是身份线索 |
| 待识别实物 | 还不能可靠对上正式包裹的作业实物。不冒充包裹，也不是库存 |
| 节点收寄 | 客户或其授权交付方在节点直接交付、节点明确接收并取得控制的结果。扫描、卸载、发现实物都不算 |
| 判断链 | 一次交付的判断与此后接续它的各版判断连成的序列；一版至多一个后继 |
| 当前有效版本 | 沿后继走到头的那一版。被接续的版本永久保留，只是不再算数 |
| 身份冲突 | 候选选不出唯一一件，或一个号码指向的包裹已关联到另一件实物 |
| 身份冲突来由 | 冲突从哪一格来：只因歧义信号、多候选、一码两物。决定它能不能经重核解开 |
| 身份冲突处置 | 持授予的人在候选里指定一件，或确认都不是。只定关联，不改变实物去向 |
| 重核 | 用保全的外部标识观察再问一次身份；只追加新版，不改原判断 |
| 可验证交付范围 | 这次送站替哪个客户账户交付有据可查。只凭送站预报或绑定全部已确认的送站交接单成立 |
| 送站预报 | 客户账户经客户渠道交来的预告：节点、预计送达时刻、所列客户包裹件号 |
| 送站交接单 | 节点开立的一张单，记一次送站的交付方、声称的客户账户与成员实物 |
| 预报有效时窗 | 预报对一次送站算不算数的区间：预计送达时刻前的提前量与后的宽限量。租户取值 |
| 实物控制 | 节点对实物的占有责任。只因节点收寄或权威运输交接而成立、转移、结束 |
| 集运单元实例 | 袋、箱、笼车的一次完整使用周期；可关联可复用载具 |
| 集运成员关系 | 实物直接位于某个集运单元里的物理包含关系 |
| 封装成员快照 | 封装时冻结的成员、封签引用与作业依据 |
| 方向依据记录 | 移入落册时同一笔记下的身份、服务结果、路由依据三格当时所知 |
| 节点监管协作承接决定 | 节点对一项关务执行协作事项逐对象、逐动作作出的承接、拒绝或待补充 |
| 单据号 | 给人看的编号，与内部标识分开；预报与交接单各有一种 |

## 取值表（代码里的封闭集）

| 轴 | 取值 |
|---|---|
| 接收观察 | `RECEIVED` `REFUSED` `SCAN_ONLY` |
| 身份核对态 | `CHECKED` `UNCHECKED` |
| 身份核对结论 | `UNIQUE` `UNKNOWN` `SCOPE_UNVERIFIED` `AMBIGUITY_ONLY` `MULTIPLE_CANDIDATES` |
| 身份冲突来由 | `AMBIGUITY_ONLY` `MULTIPLE_CANDIDATES` `ONE_MARK_ON_TWO_UNITS` |
| 身份冲突处置结果 | `DESIGNATED` `NONE_OF_CANDIDATES` |
| 交付范围依据 | `DELIVERY_PRE_ADVICE` `DELIVERY_HANDOVER_NOTE` |
| 交付范围核对态 | `VERIFIED` `UNVERIFIED` `UNCONFIGURED` |
| 范围未证原因 | `NO_BASIS` `BASIS_NOT_REGISTERED` `BASIS_NOT_APPLICABLE` `NUMBER_NOT_PRE_ADVISED` `BINDINGS_NOT_ALL_CONFIRMED` |
| 依据不适用的细分 | `NODE_MISMATCH` `OUTSIDE_WINDOW` `WITHDRAWN` `NOT_A_HANDOVER_MEMBER` |
| 交接单成员来路 | `RECEPTION` `ASSIGNMENT` |
| 绑定结论／途径 | `CONFIRMED` `DENIED`／`CUSTOMER_CHANNEL` `OPERATION_DECISION` |
| 授权交付答复 | `AUTHORIZED` `NOT_AUTHORIZED` `NOT_CONFIGURED` |
| 控制成立方式／转移结果 | `NODE_INTAKE` `HANDOVER_IN`／`ADVANCED` `NOT_ADVANCED` |
| 交接对象 | `PARCEL` `CONSOLIDATION_UNIT` |
| 集运动作 | `OPEN_UNIT` `ADD_MEMBER` `REMOVE_MEMBER` `SEAL_UNIT` `UNSEAL_UNIT` `CLOSE_UNIT` |
| 集运单元阶段 | `OPEN` `SEALED` `CLOSED` |
| 方向依据的格／答复 | `IDENTITY` `SERVICE_RESULT` `ROUTING`／`ESTABLISHED` `MISSING` `NOT_CONFIGURED` `UNREADABLE` `NOT_JUDGED` |
| 方向依据缺少原因 | `NO_INTAKE_RECORD` `PENDING_IDENTIFICATION` `IDENTITY_CONFLICT` `CANCELLED` `FINAL` |
| 协作动作 | `UNSEAL` `ISOLATE` `PRESENT` `TALLY` `OBSERVE` |
| 承接决定 | `ACCEPTED` `DECLINED` `PARTIALLY_ACCEPTED` `PENDING_SUPPLEMENT` |
| 待补充缺口 | `OBJECT` `ACTION` `NODE` `AUTHORITY` `WORK_CONDITION` |
| 单据号类型／缺号原因 | `DELIVERY_PRE_ADVICE` `DELIVERY_HANDOVER_NOTE`／`ISSUED_BEFORE_NUMBERING` `NUMBERING_TIME_ZONE_NOT_CONFIGURED` |

控制不推进的原因：`NO_ASSOCIATED_UNIT` `SEVERAL_ASSOCIATED_UNITS` `CONSOLIDATION_UNIT_PENDING_RULE` `NO_ACTIVE_CONTROL` `SEVERAL_ACTIVE_CONTROLS` `HANDOVER_PRECEDES_CONTROL` `CONTROL_SOURCE_CHAIN_PENDING_RECOMPUTE` `CONTROL_STILL_ACTIVE` `HANDOVER_PRECEDES_RELEASE`。

## 收寄的结果

| 结果 | 什么时候 | 之后 |
|---|---|---|
| `INTAKE_FORMED` 收寄形成 | 明确接收，且身份可靠识别 | 控制成立，关联建立，交出收寄结果 |
| `PENDING_IDENTIFICATION` 待识别 | 明确接收，但没答复、没候选、缺范围证据或有身份冲突 | 控制照常成立；等重核或身份冲突处置 |
| `INTAKE_NOT_FORMED` 收寄未形成 | 明确拒收 | 来源照样保全，控制不成立 |
| `RECEPTION_UNDECIDED` 待确认 | 只有扫描或卸载 | 来源照样保全，控制不成立 |
| `EXISTING_RESULT` 已有结果 | 同一来源原样重投，或并入同一节点已有的收寄／控制 | 答原结果，不另起控制起点 |
| `SOURCE_CONFLICT` 来源冲突 | 同一来源身份、内容不同 | 拒，不改已有记录 |

并入在册上另记一行 `MERGED_INTO_EXISTING_INTAKE`，它不是判断，不占判断链。

其余各口的应答码：

- 重核：`INTAKE_FORMED` `IDENTITY_NOW_CHECKED` `STILL_UNIDENTIFIED` `STILL_AMBIGUOUS` `AMBIGUITY_GONE_SCOPE_UNVERIFIED` `IDENTITY_CONFLICT_RECORDED` `IDENTITY_CONFLICT` `NOT_PENDING_IDENTIFICATION` `SOURCE_NOT_PRESERVED` `EXISTING_RESULT`。
- 身份冲突处置：`IDENTITY_CONFLICT_DISPOSED` `EXISTING_RESULT` `NO_IDENTITY_CONFLICT` `NOT_A_CANDIDATE` `ASSOCIATED_ELSEWHERE` `VERSION_SUPERSEDED` `ALREADY_DISPOSED`。
- 集运：`OPENED` `MEMBER_ADDED` `MEMBER_REMOVED` `SEALED` `UNSEALED` `CLOSED` `EXISTING_RESULT` `SOURCE_CONFLICT` `UNIT_NOT_FOUND` `MEMBER_ALREADY_CONTAINED` `MEMBER_ELSEWHERE_CONTAINED` `CONTAINMENT_CYCLE` `NOT_ACCEPTED`。
- 送站交接单：`DELIVERY_HANDOVER_NOTE_OPENED` `UNIT_ASSIGNED` `DELIVERY_HANDOVER_NOTE_CLOSED` `BINDING_CONFIRMED` `BINDING_DENIED` `EXISTING_RESULT` `EXISTING_CONCLUSION` `SOURCE_CONFLICT` `UNIT_NOT_ASSIGNABLE` `HANDOVER_NOTE_STATE_MISMATCH`（`NOTE_CLOSED`／`NOTE_NOT_CLOSED`） `NOT_IN_AUTHORIZED_DELIVERY_RELATIONSHIP` `AUTHORIZED_DELIVERY_NOT_CONFIGURED` `NUMBERING_RULE_COLLISION`。
- 协作承接与执行：`COLLABORATION_DECIDED` `EXECUTION_RECORDED` `EXISTING_DECISION` `EXISTING_FACT` `DECISION_CONFLICT` `FACT_CONFLICT` `SOURCE_NOT_ACCEPTED` `OUTSIDE_ACCEPTED_SCOPE` `ITEM_DECLINED` `ITEM_PENDING_SUPPLEMENT` `PENDING_SUPPLEMENT`。
- 交接推进控制：`CONTROL_TRANSFERRED_OUT` `CONTROL_ESTABLISHED_IN` `CONTROL_NOT_ADVANCED` `HANDOVER_DOES_NOT_TRANSFER` `NO_RECEIVING_NODE_DECLARED` `TRANSITION_ALREADY_RECORDED` `ALREADY_TRANSFERRED_IN_CHAIN` `ALREADY_ESTABLISHED_IN_CHAIN` `CONTROL_ON_HANDOVER_UNDECIDED`。

## 规则

**来源与判断**

- 每次交付观察先保全来源：外部标识观察原样、接收观察、证据、交付方、交付范围依据引用、节点、发生时间。来源只写一次，此后不改不删。
- 判断只追加新版并指名前版，不覆盖。现场不得以「最后一次扫描为准」定身份。
- 没取得 `parcel-shipment` 的答复记「未核对」。它不是「查过、没有候选」，不得以空候选顶替。
- 已取消或已终局的包裹到站照记接收与控制；这两样只限制后续方向性作业，不重开服务。

**身份核对**

- 候选只来自 `parcel-shipment` 在租户内按号码的答复；号码逐字比对。范围不改变候选从哪里取，只改变哪一个算数。
- 恰一个候选、没有歧义信号、范围相容，才算可靠识别。
- 恰一个候选带歧义信号：待识别，保存候选，记身份冲突，来由是只因歧义信号。
- 多于一个候选：身份冲突，来由记多候选，不论带不带信号。号码唯一指向的包裹已关联到另一件实物：一码两物。
- 恰一个候选、没信号、只是不范围相容：待识别，保存候选，不记冲突——缺的是证据，不是有反证。没有候选：待识别，不记冲突。
- 有可验证交付范围时，只数范围相容的候选，只看范围内的信号；范围外的候选照旧保存。
- 交付范围核对态随判断记下。「未配置」指预报有效时窗没登记，与「查过、未证」分开；不推定默认时长。

**重核与身份冲突处置**

- 重核对象：当前有效版本是待识别、不带关联、来源已保全，且不记冲突或来由是只因歧义信号。多候选、一码两物与来由未记的冲突不在其中。
- 重核结果唯一才建立关联；关联沿用原接收时刻，不另起控制起点。
- 只因歧义信号的冲突：信号消失后重核唯一命中即解开；信号没了而只缺范围证据，追加一版不记冲突的待识别；信号还在，判断不变。
- 其余冲突只经身份冲突处置解除；再次收寄不解除。
- 处置要指名自己看到的那一版当前有效版本，只能在各条判断链当前有效版本记的候选与关联里指定一件，或确认都不是。
- 指定的那一件此刻关联在别的实物上时，同一次处置给那些实物各追加「不是它」，不留空窗。
- 处置确认过「不是」的包裹，重核再唯一命中它也不关联，照零候选办。谁持有处置授予是租户取值。

**送站预报与交接单**

- 预报只由客户账户经客户渠道提交、撤回。它不形成委托、包裹或客户承诺。
- 设备上报的交付方与交接单上的声称不构成可验证交付范围；交接单要声称的客户账户全部已确认才成立范围。
- 绑定由该客户账户经客户渠道确认或否认，或由持授予的人依授权交付关系确认。授权交付关系没登记时答未配置。
- 确认错了、预报撤回了，都不回滚已形成的关联。
- 单据号在登记的同一事务内取得，原样重投答原号；它不替代内部标识。日期按网点当时生效的业务时区，没有时按租户的单据编号时区。

**节点控制**

- 控制只因节点收寄或权威运输交接的「已交接」成立、转移、结束。已拒收、待确认、扫描、装卸都不动控制。
- 同一实物在同一节点已有当前收寄或交接转入的控制、且尚未转出时，再次交付观察并入，只保全来源。
- 交接按对象认实物：按包裹身份读当前关联，恰对上一件才推进；在身控制不是恰好一份，或交接早于那份控制成立，不推进。不推进照样留记录，不猜。
- 交接对象是集运单元时不推进成员的控制，记 `CONSOLIDATION_UNIT_PENDING_RULE`。单元控制规则尚未确立。
- 交接转入要「已交接」且声明了接收节点；同一次交接先结束交出一侧的控制，再成立接收一侧的。实物仍有在身控制，或最近一次转出晚于这次交接，不转入。
- 已推进控制的交接被更正为已拒收或待确认时，控制不回退，记 `CONTROL_SOURCE_CHAIN_PENDING_RECOMPUTE`。哪版更正哪版只认 `transport-fulfillment` 的更正回指。
- 转移记在只追加的节点控制转移册上，收寄判断不因转出改写。转出后再送到同一节点是新的收寄。

**集运与封装**

- 成员关系只由实际移入、移出事实形成。同一实物同一时点至多一个直接父级；包含不得成环。
- 封装冻结成员快照，要有封签引用、作业依据和至少一个成员。封装后改成员必须先开封。历史快照不被覆盖。
- 关闭前成员须已全部移出，或带明确处置依据。已关闭的实例不得重开或再装；载具再用另起实例。
- 已发生的移入照记，不因方向依据缺少或读不动而拒；同笔留下方向依据记录，事后不补判、不改记。其中身份一格只读各条判断链的当前有效版本。

**关务协作**

- 承接按对象和动作逐项决定，允许部分承接。条件不足是待补充，明确不允许是拒绝。
- 承接决定与执行事实分开保存。执行事实只在已承接的范围内收，范围外拒。

## 生命周期

- **判断链**：收寄判断 →（重核版、身份冲突版、处置版）逐版追加。实物从待识别走到已关联，或经处置确认都不是后仍待识别、可再重核。
- **送站预报**：已登记 → 已撤回（终态）。有效时窗不是状态，按那次送站的业务时间判。
- **送站交接单**：开放（收寄带入或归入成员）→ 已关闭（成员冻结）。每个绑定：待确认 → 已确认或已否认，都是终态。任一绑定被否认，这张单不再成立范围。
- **预报有效时窗**：登记一行（与在册行重叠即拒）→ 可给开放终点的那一行追加收口。
- **节点控制**：尚未控制 → 成立（节点收寄或交接转入）→ 转出（权威交接「已交接」）。
- **集运单元实例**：开放装入 → 已封装 → 开封回到开放 → 再封装（新快照）……→ 关闭（终局）。
- **协作承接**：收到协作事项 → 已承接、部分承接、待补充或拒绝 → 已承接范围内记执行事实。

## 入口

在线口（`cmd/parcel-api`，处理器在 `adapters/http`）：

| 做什么 | 路径 |
|---|---|
| 收寄（作业端） | `/node-operations/receptions` |
| 重核、身份冲突处置（运营决定） | `/node-operations/identity-rechecks` `/node-operations/identity-dispositions` |
| 送站交接单（作业端开立、归入、关闭；运营确认） | `/node-operations/delivery-handover-notes` `/node-operations/delivery-handover-assignments` `/node-operations/delivery-handover-closures` `/node-operations/delivery-handover-confirmations` |
| 客户渠道 | `/v1/node-operations/delivery-pre-advices` `/v1/node-operations/delivery-pre-advice-withdrawals` `/v1/node-operations/delivery-handover-conclusions` |
| 关务协作承接与执行 | `/node-operations/collaboration-acceptances` `/node-operations/collaboration-executions` |
| 预报有效时窗登记与收口 | `/node-operations-pre-advice-window-registrations` `/node-operations-pre-advice-window-closures` |
| 查阅 | `/node-operations-records`（`registry` 取 `reception` `unidentified-item` `identity-review-unit` `consolidation-unit` `consolidation-member` `physical-control`） `/node-operations-delivery-pre-advices` `/node-operations-delivery-handover-notes`（各带按标识取单条） |

事件消费（`cmd/parcel-dispatch`）：`transport-fulfillment.transport-handover.registered` 推进节点控制；`parcel-shipment.external-identifier-relation.formed`、`parcel-shipment.customer-parcel-number-declaration.retired`、`node-operations.unchecked-pending-identity.recorded`、`node-operations.delivery-scope.established` 触发重核。本上下文发出：`node-operations.node-intake.formed` `node-operations.sealed-snapshot.recorded` `node-operations.collaboration-acceptance.decided` `node-operations.execution-fact.recorded`。

离线口（数据库网络内手跑，不监听端口）：`cmd/parcel-node-operations-register`（预报有效时窗的登记与收口）、`cmd/parcel-frontline-import`（按 CSV 模板批量灌收寄与集运六个动作）。

## 旧文档写了、代码里还没有的

- **实际测量与当前有效实测、作业位置与当前位置派生**：搜 `Measure`、`Weigh`、`Location`、`测量`、`作业位置`，没有类型，只有注释提到。
- **在场核对、物理状况观察、节点作业限制**：搜 `Presence`、`Condition`、`Restriction` 及中文原词，没有对应类型。
- **作业任务与作业批次**：搜 `WorkTask`、`Batch`、`作业任务`，没有。协作承接之后直接记执行事实，中间没有任务这一层。
- **封签记录的状态与观察历史，封签缺失、破损、不一致时的隔离与逐件核对**：代码里只有封签引用 `SealReference`，搜 `SealStatus`、`SealState`、`破损` 没有。
- **受控开封要保存的开封前状况、内容物观察、差异**：`Unseal` 只收作业依据与时刻。
- **标签操作（内部作业标签的生成更换，正式标签的打印、遮盖、移除）、真实拆分合并与重新包装、物理装卸事实与节点侧交接证据**：搜 `Label`、`Split`、`Repack`、`Load`、`拆分`、`装载`、`卸载`，没有对应的事实类型。
- **方向依据随实物下发、每格带有效区间、作业端出区间即暂停方向性作业**：服务端只有落册时的方向依据记录；搜 `有效区间`、`ValidUntil`，下发与区间都没有。
- **协作事项被取消、替代或缩小范围后停止剩余范围**：在协作的领域与编排文件里搜 `Cancel`、`Supersed`、`取消`、`替代`，没有。
- **封签连续相符时成员「随单元受控」的派生判断**：代码只有「待单元控制规则」这一格，没有派生。

## 证据限制

集运六个动作没有在线口，目前只经 `cmd/parcel-frontline-import` 进来。这个导入口在源码里写死了拆除期限（`2026-12-31T23:59:59+08:00`），过期启动即拒。在那之前没有别的写入口接上，集运事实就没有来路。
