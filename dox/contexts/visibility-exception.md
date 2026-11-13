# 全程追踪与异常（`visibility-exception`）

代码在 `internal/visibilityexception`。对照 main `99fc0037d`（2026-11-12）写成。

**这份文件核到什么程度**：取值表、结果表、入口三节是从代码里读出来的。「规则」一节的句子是从旧 `docs/domain/visibility-exception/CONTEXT.md` 提炼的，每条只确认了代码里有对应的机制（类型、函数或结果码），没有逐条跑测试验证行为。本上下文有一批机制只在领域层和应用层，生产装配没有调用方；这些规则仍留在「规则」里，但在末节逐项点名。旧文档写了而代码里没有的，也单列在末节。

## 管什么，不管什么

管：全程追踪投影与标准追踪里程碑，客户全程追踪视图，ETA 与 ETA 客户可见性判断，可见性缺口，异常信号、发作期、分诊与异常案件，处置请求，客户可见异常与客户异常通知，证据项，客户索赔项，追偿事项。

不管：

- 源事实本身（收寄、交接、移动、交付、关务结果、路由、终局）——各源上下文。本上下文只引用，不使任何源事实失效，不解除任何限制。
- 货主客户账户、合同、客户服务规则版本——`party-commercial`。
- 委托、包裹身份与谱系、客户归属、客户服务终局——`parcel-shipment`。
- 改路、重作业、重新履约、重新申报是否成立——处置请求的目标上下文。本上下文只提请求。
- 赔付与追偿的金额、到账、核销——`settlement-accounting`。
- 消息渠道的技术投递、认证与授权。

## 词

| 词 | 意思 |
|---|---|
| 全程追踪投影 | 对各源上下文已接受事实编排出的只读旅程视图，按包裹成版本，可重新派生 |
| 来源事实替代关系 | 源上下文随更正给出的「新版本取代旧版本」。本上下文只登记，不自己推断 |
| 标准追踪里程碑 | 事实按版本化映射归入的统一语义；归不进就保持未归类 |
| 入账不派生 | 记下这份事实已消费，但不形成投影条目，也不重投 |
| 客户全程追踪视图 | 面向已授权货主客户账户的查询视图，按账户隔离 |
| 运营追踪查阅 | 租户内运营人员对投影及留存版本的只读查阅，不构成客户披露 |
| 客户归属 | 某包裹此刻可确定的货主客户账户，由 `parcel-shipment` 给出 |
| 预计到达时间（ETA） | 对包裹某个目标里程碑的版本化时间预测。不是客户承诺、路由计划或实际时间 |
| ETA 客户可见性判断 | 对一份候选 ETA 能否给某客户看的版本化判断。不是 ETA 本身 |
| 可见性缺口 | 预期的观察过了窗口仍未取得。只说明数据没到，不说明延误或遗失 |
| 异常信号 | 按规则或事实识别出的风险或偏差。不是案件、责任结论或限制 |
| 信号发作期 | 同一对象、信号类型、因果条件下一段连续影响期 |
| 异常案件 | 围绕同一因果链和处置范围需要持续处置的业务案件 |
| 处置请求 | 案件向源上下文提出的结构化请求。已发送、已接受、已取消、已执行各是一回事 |
| 客户可见异常 | 按披露规则从内部信号形成的、按客户账户隔离的披露决定 |
| 客户异常通知 | 针对明确客户和内容快照的通知，生成到客户确认逐步分开记录 |
| 证据项 | 可追溯的材料及其评价。被提交不等于内容被认定 |
| 客户索赔项 | 一个账户针对一个包裹或服务范围的独立补救请求，逐项审核 |
| 追偿事项 | 对供应商、承运商或保险方的独立主张，不等客户索赔 |

## 取值表（代码里的封闭集）

| 轴 | 取值 |
|---|---|
| 源上下文 | `PARCEL_SHIPMENT` `NETWORK_ROUTING` `NODE_OPERATIONS` `TRANSPORT_FULFILLMENT` `CUSTOMS_COMPLIANCE` |
| 冲突裁决依据 | `BUSINESS_TIME` `CAUSAL_ORDER` `AUTHORITY_SCOPE`（后两格占位，还没有产生函数） |
| 视图维度 | 里程碑、ETA、终局、追踪说明四维，每维 `SHOWN` `PENDING_CONFIRMATION` `NOT_DISCLOSED` |
| ETA 来源 | `OPERATOR_DERIVED` `CARRIER_PROVIDED` |
| ETA 对客表达 | `POINT_VALUE` `TIME_RANGE` |
| ETA 可见性结论 | `CUSTOMER_VISIBLE_ETA` `ETA_NOT_VISIBLE` `ETA_VISIBILITY_PENDING` |
| ETA 可见性理由 | `QUALITY_MET_GATE` `QUALITY_BELOW_GATE` `RULE_NOT_CONFIGURED` `RULE_CONFLICT` |
| 分诊结论 | `ATTACH_TO_EXISTING` `AUTO_ESTABLISH` `MANUAL_REVIEW` `NO_CASE` |
| 案件主状态 | `AWAITING_RESPONSE` `IN_PROGRESS` `CLOSED` |
| 源上下文对处置请求的判断 | `ACCEPTED` `PARTIALLY_ACCEPTED` `REFUSED` `SUPPLEMENT_REQUIRED` |
| 取消答复 | `CANCELLATION_ACCEPTED` `PARTIALLY_CANCELLED` `NO_LONGER_CANCELLABLE` `CANCELLATION_REFUSED` |
| 披露结论 | `DISCLOSE` `NOT_YET_DISCLOSABLE` `AWAITING_AUTHORIZATION` |
| 通知节点 | `GENERATED` `SUBMITTED_TO_CHANNEL` `CHANNEL_ACCEPTED` `DELIVERED` `FAILED` `CUSTOMER_CONFIRMED` |
| 证据评价 | `RECEIVED` `CREDITED` `DISCREDITED` |
| 索赔资格审核 | `ELIGIBLE` `INELIGIBLE` `AWAITING_SUPPLEMENT` |
| 索赔责任结论 | `FULLY_ESTABLISHED` `PARTIALLY_ESTABLISHED` `NOT_ESTABLISHED` `UNDETERMINABLE` |
| 追偿动作种类 | `PRELIMINARY_NOTICE` `FORMAL_ASSERTION` |
| 追偿动作节点 | `PREPARED` `SUBMITTED` `CHANNEL_ACCEPTED` `DELIVERED` `ACKNOWLEDGED` `SUBMISSION_FAILED` `DELIVERY_FAILED` |
| 单据类型；无号原因 | `EXCEPTION_CASE` `CLAIM`；`ISSUED_BEFORE_NUMBERING` `NUMBERING_TIME_ZONE_NOT_CONFIGURED` |
| 目录种类 | `MILESTONE_MAPPING` `TRIAGE_RULE` `NOTIFICATION_POLICY` `CLAIM_ELIGIBILITY` `CLAIM_AUTHORIZATION` `DISCLOSURE_POLICY` `EXCEPTION_DISCLOSURE_RULE` `CONFLICT_SIGNAL_RULE` |

事实类型词（各适配器产出的全部）：`node-intake`；`offsite-pickup`；`handover-handed-over` `handover-refused` `handover-pending-confirmation`；`movement-departure` `movement-in-transit` `movement-arrival`；`effective-delivery`；`exception-journey-alternate` `exception-journey-return`；`external-carrier-tracking`；`customs-case-established`；`declaration-submission-formed`；`external-result-regulatory-receipt` `external-result-business-acceptance` `external-result-process-decision` `external-result-assessed-duty` `external-result-disposition-decision` `external-result-release-full` `external-result-release-partial` `external-result-release-conditional`；`final-outcome-adopted` `final-outcome-not-adopted`；`initial-route-formed` `initial-route-no-current-route`。词一经租户映射引用就是租户可见的配置键。

## 各编排的结果

每个编排除下表外都还有 `EXISTING_RESULT`（重放答原结果）、`UNDECIDED`（依赖读不动，带原因码，等重投）和 `NOT_ACCEPTED`（输入立不住）。三者不得互相替代。规则册没登记时如实停在 `…_NOT_CONFIGURED` 一类的格上，不套默认。

| 编排 | 业务结果 |
|---|---|
| 派生投影 | `PROJECTION_DERIVED` `SOURCE_CONFLICT` |
| 冲突与出发门禁信号 | `SIGNAL_RAISED` `SIGNAL_RULE_NOT_CONFIGURED` `SIGNAL_DEFERRED` `CASE_NUMBERING_RULE_COLLISION` |
| 信号命中 | `EPISODE_OPENED` `HIT_RECORDED` `EPISODE_REOPENED` `CASE_NUMBERING_RULE_COLLISION` |
| 形成 ETA 与缺口 | `ETA_FORMED` `ETA_REFRESHED` `GAP_FORMED` `GAP_WINDOW_NOT_ELAPSED` |
| ETA 可见性判断 | `JUDGED` `REJUDGED` `NO_CANDIDATE_ETA` `OUTSIDE_RULE_TARGET` |
| 派生客户视图 | `PUBLISHED` |
| 客户查询（按包裹） | `CURRENT_VIEW` `NO_VISIBLE_RESULT` `VIEW_NOT_FOUND` |
| 客户查询（按委托单据号） | `PARCELS_LISTED` `AWAITING_ACCEPTANCE` `CLOSED_WITHOUT_SERVICE` `SHIPMENT_NOT_FOUND` |
| 运营追踪查阅 | `PROJECTIONS_LISTED` `CURRENT_PROJECTION` `PROJECTION_VERSION` `PROJECTION_NOT_FORMED` `VERSION_NOT_FOUND` |
| 处置请求 | `SENT` `SUPERSEDED` `JUDGMENT_RECORDED` `ACCEPTANCE_WINDOW_EXPIRED` `CANCELLATION_RECORDED` |
| 披露决定与通知 | `DECIDED` `AUTHORIZED` `NOT_AWAITING_AUTHORIZATION` `DISCLOSURE_CONDITION_NO_LONGER_MET`；`SUBMITTED` `SUBMISSION_FAILED` |
| 索赔 | `CLAIM_RECEIVED` `CLAIM_SCREENED` `CLAIM_AWAITING_SUPPLEMENT` `CLAIM_CONCLUDED` `CLAIM_REVIEWED` `REVIEW_WINDOW_CLOSED` `CLAIM_CONCURRENTLY_CHANGED` `NUMBERING_RULE_COLLISION` |
| 追偿；证据 | `RECOVERY_OPENED` `RECOVERY_ACTION_RECORDED`；`EVIDENCE_SUBMITTED` `EVIDENCE_APPRAISED` `DISCLOSURE_PREPARED` |
| 目录登记 | `REGISTERED` `REFUSED`（`SCOPE_MISSING` `VERSION_MISSING` `APPROVAL_MISSING` `EFFECTIVE_RANGE_MISSING` `EFFECTIVE_RANGE_REVERSED` `ENTRIES_MISSING` `ENTRY_INCOMPLETE` `ENTRY_DUPLICATED` `VERSION_NOT_OVERWRITABLE` `VERSION_OVERLAPS_EXISTING`） |
| 索赔材料收讫 | `REGISTERED` `ALREADY_REGISTERED` `REVOKED` `ALREADY_REVOKED` `REFUSED` |

## 规则

**投影**

- 投影只消费源上下文已接受的事实；字段里只有引用与归类，不存第二套源事实，任何人不能直接编辑。
- 里程碑映射按源上下文与事实类型版本化登记；归不进的保持未归类。
- 投影版本只增不改写；重新派生形成新版本并指回前版，原版本可按版本读回。
- 替代关系只在同一源上下文、同一事实引用的版本之间成立，前身只取源上下文给出的指名。「已被替代」是派生问答，不另存标记；被替代的事实留档，不参与派生。
- 冲突裁决不用来源排名，也不用最后消息覆盖；裁不出时保留各方并形成异常信号。
- 源上下文已判为同层冲突的那一份、没有可投追踪对象的事实，入账不派生。
- 移动事实的在控对象是集运单元时，投到事实发生那一刻的成员包裹上，逐层展开，同一件只投一次；单元本身不形成投影。交接、对象级场外揽收、有效交付与外部承运轨迹的对象是单元时，不投到成员上。
- 移动事实早于对象进段的，由后来让对象进段的控制事实补派生；成员日后才识别成功的，由节点收寄补派生。

**ETA 与缺口（部分无生产入口，见末节）**

- 每版 ETA 带预测对象、目标里程碑、预测时点、输入、模型版本、时间范围和可信度；信息不足就不形成。观察窗口届满才形成可见性缺口。
- 一条预测线由包裹、目标里程碑和来源确定；新版本只接替本线前一版。同一输入再到是重复到达，答原版本。
- 可见性判断只有三格。规则缺失或冲突落待判断，不按默认判可见；质量不达门槛落不可见。
- 一个账户在一件包裹上只有一条判断线，只认客户服务规则指名的那个目标里程碑。
- 两路来源都有当前版时，按运营企业派生在前、承运商提供在后，取第一个达门槛的；都不达标取排在最前的那一版。候选的来源或版本变了才形成新判断并指回原判断。

**信号、分诊与案件**

- 信号与案件是两个对象，都不产生业务限制，也不认定责任。同一发作期内重复命中只更新判断历史；结束后再命中开关联的新发作期并重新分诊。
- 分诊按租户登记的版本化规则；规则没登记时不建案。
- 案件主状态只有待响应、处理中、已关闭三格。接单、关闭、归并是三种分开授予的运营决定，时刻取服务端落决定的那一刻，接单人与关闭人记在案件上。接单只形成一次首次响应时间；待响应可不经接单直接关闭，关闭必带结论。重复案件只能并进一份仍在处置的主案件，以「已归并」关闭并指向它，两边的单据号都不动。两个人同时处置同一份案件，后落的不盖掉先落的；归并先把两份案件一起锁住再读，对向的两笔归并不会把两份都关掉。
- 建案时在同一事务内取单据号；归并不改两边的号。租户没登记单据编号时区时照常建案，记无号原因。撞号到跳号上限时这一次建案不落，记进重驱队列，按时原样再交分诊。

**处置请求（无生产入口，见末节）**

- 请求固定目标上下文、动作、范围、原因、证据和时限，只能挂在活案件下。
- 发送、源上下文判断、取消答复分别记录；判断不证明动作已执行。
- 受理有效期届满后不再接受判断；替代形成新请求并指回旧请求，不撤销已发生的事实。

**客户视图与查询**

- 客户视图只基于当前有效投影、披露策略、ETA 可见性判断与客户服务终局形成，按货主客户账户隔离。
- ETA 一维以披露策略为上限：策略不准的不展示；准许时，判断可见才展示，不可见不展示，待判断或没有判断答待确认。某一维收住时只收那一维，不虚构内容。
- 视图换版标明替代关系，原版本保留；投影换版而各维内容不变时不出新版本。
- 客户归属确立晚于源事实时，在确立那一刻按当前投影形成视图，不等下一份源事实。
- 查询同时核对请求方身份、货主客户账户与目标对象；标识只用于定位，不证明权限。
- 包裹唯一归属请求方账户而视图未形成时答 `NO_VISIBLE_RESULT`；其余没有视图的情形对外同一答复 `VIEW_NOT_FOUND`。
- 按委托单据号查询时，已接受的委托才逐件列出当前有效包裹；号不存在与属他人账户对外同一答复。
- 运营追踪查阅作用域是租户级，「投影未形成」如实作答；查阅不构成客户披露。

**披露与通知（通知无生产入口，见末节）**

- 披露决定按客户分别形成；案件存在不自动披露，条件不成立与授权不足分别答暂不披露和待授权。发起披露判断与确认发布是两种分开授予的运营决定：发起的人只指名包裹、信号类型与哪一期发作期（同一包裹同一类信号可因不同原因同时有几期，决定钉在指名的那一期上），结论由披露规则给，客户由已接受的委托反查。同一条信号对同一个客户只有一份按规则形成的决定；待授权的决定由授权角色确认后另形成一份披露决定，原决定不改，每份待授权只被确认一次。确认时重取此刻的规则：内容取规则的快照，规则已改成不披露则不放行。发起人与确认人记在决定上。
- 通知保存内容快照、披露依据、时限与渠道；各节点分别记录，前一步不推出后一步；义务由哪个节点满足，按策略指名的节点判断。

**证据、索赔与追偿（部分无生产入口，见末节）**

- 证据的登记、评价、披露版本准备是三种分开授予的运营决定。本产品只记内容指纹，不存材料本体。登记只表示收到，收到时刻取服务端登记的那一刻，同一提供方的同一份材料是同一项。评价只能是采信或不采信、依据必备、只落一次，已评价的不改写。披露版本锚住原件指纹，脱敏指纹不得与原件相同，必带披露范围。登记人、评价人与评价时刻记在证据项上。
- 索赔逐项受理，同一批次、同一项重放答原结果；内容不同的重放不接受。受理、资格审核、责任结论是三个判断。
- 首次索赔期限与最低材料按目标包裹所属委托接受时固定的规则版本取；取不到就停在未登记。
- 资料不足进入限期补充，固定缺少的材料与截止时间；延期形成新期限版本，原期限保留。
- 复核在期限内形成新结论版本，原结论保留；过期答 `REVIEW_WINDOW_CLOSED`，原结论不变。已有责任结论的索赔撤不回。
- 追偿事项按相对方与责任依据分别建立，不等客户索赔；预先通知与正式主张是不同动作；各节点分别记录，失败可重试，每次尝试保留。

## 生命周期

- **投影**：源事实入账 → 派生新版本（或入账不派生）→ 后续事实、更正、补派生再出新版本。
- **ETA 可见性判断与客户视图**：ETA 新版本 → 判断或改判；投影派生、归属确立或判断换版 → 发布新视图版本，或沿用现版本。
- **发作期**：开启 → 重复命中 → 结束 → 再命中时开关联的新发作期。
- **案件**：待响应 → 处理中 → 已关闭（含已归并）。
- **处置请求**：已发送 → 源上下文判断（或受理有效期届满）→ 取消答复或被替代。
- **索赔项**：已收到 → 资格审核（合格、不合格、限期补充）→ 责任结论 → 期限内复核；结论前可撤回。
- **通知、追偿动作**：从生成或准备起逐节点记录，到客户或对方确认为止。

## 入口

在线口（`cmd/parcel-api`，路径在 `adapters/http`）：

| 做什么 | 路径 |
|---|---|
| 客户查追踪 | `/v1/customer-tracking-view` `/v1/customer-shipment-tracking` |
| 客户提交索赔 | `/v1/claims` |
| 案件决定 | `/exception-case-take-ups` `/exception-case-closures` `/exception-case-merges` |
| 证据决定 | `/evidence-submissions` `/evidence-appraisals` `/evidence-disclosure-preparations` |
| 披露决定 | `/customer-disclosure-decisions` `/customer-disclosure-authorizations` |
| 运营只读 | `/tracking-projections` `/exception-triage-records` `/exception-case-records` `/claims-recovery-records` `/visibility-catalogues` |
| 目录登记 | `/visibility-catalogue-<种类>-registrations`，种类为 `milestone-mapping` `triage-rule` `notification-policy` `claim-eligibility` `claim-authorization` `disclosure-policy` `exception-disclosure-rule` `conflict-signal-rule` |

事件消费（`cmd/parcel-dispatch`）：节点收寄，场外揽收，运输交接，移动事实，有效交付，异常旅程，外部承运轨迹，出发门禁补问，关务案件建立，申报提交，关务外部结果，初始路由，接受决定，终局，以及本上下文自己的投影派生与 ETA 预测两种事件。撞号重驱也在这个进程里。

离线口：`cmd/parcel-ve-register`，在数据库网络内手跑。除上面八种目录外，还登记出发门禁信号规则（`departure-gate-signal-rule`）、索赔材料收讫及其撤销。

## 旧文档写了、代码里还没有的

**有机制、生产装配没有调用方**（只被测试调用）：

- **形成 ETA 与可见性缺口**：`FormETAHandler` 没有任何 `cmd/` 装配。下游的 ETA 可见性判断已接在 `parcel-dispatch` 上，但没有预测来源入队，客户视图的 ETA 一维在生产里只会答待确认或不展示。
- **处置请求**（`SendDispositionRequestHandler`）、**客户异常通知**（`NotifyCustomerHandler`）：都没有装配。对应的目录可以登记，运营读口可以读，但没有东西写入。披露决定已接上，但只由操作员逐条发起，信号命中时不自动判。
- **索赔受理之后的各步**：`/v1/claims` 只接受理。资格审核已装配但没有端点；责任结论、复核、开立追偿、记录追偿动作没有调用方。
- **发作期结束，索赔撤回，补充延期**：领域方法在（`End` `Withdraw` `ExtendSupplementDeadline`），应用层没有编排调用它们。案件关闭前「未完成的处置请求都已移交或完成取消核对」此刻没有可核的对象，因为处置请求没有生产入口。
- **生产里信号的来源只有两处**：替代链分叉裁不出，以及出发门禁阻断。旧文档说的关务异常信号、ETA 与缺口信号没有接到信号编排上。

**代码里找不到机制**：

- **替代链分叉一律待确认**：目标规则是分叉不裁决——前身与各后继留在投影里标明分叉、不参与派生，那一条事实流保持信息待确认并立信号。代码现在的做法不同：`judgeForks` 用 `ResolveByBusinessTime` 给分叉排序，只有同刻的才立信号，各后继照常归类。已裁定代码暂不改：今天接进来的生产源造不出分叉。**接入任何可能造出分叉的来源之前，先把代码改成目标规则。**
- **追踪摘要与并行维度**：投影只有条目列表，搜 `Summary`、追踪摘要无类型。
- **严重度、处置优先级、异常响应策略、责任团队转交、响应周期**：搜 `Severity` `Priority` 响应策略，只有读口注释说明「存储上没有登记格」。
- **案件重开、拆出、影响范围版本**：`ExceptionCase` 没有重开方法，范围只是建案时的一个引用。
- **原因结论与分方向的责任结论**：搜根本原因、促成因素无结果；责任结论只有索赔项上的四格。
- **处置请求的实际执行结果**、**追偿的期限延期、失权复核、对方响应分类**：请求上没有执行结果一格；追偿事项只有一个截止时间和动作节点。
- **索赔补充期限届满后的后果规则**：届满只停在 `ELIGIBILITY_SUPPLEMENT_WINDOW_CLOSED`，没有按规则转不予受理或继续审核。
- **客户归属转手或失效时的终止版本**：搜 `terminat`、终止版本、不再可见，非测试代码无结果。来源系统整体中断的来源级信号、规则存量重评也无对应类型。
- **`warehouse-outbound-confirmed` 事实类型**：代码里没有这个词，源上下文枚举也没有仓配履约。它属可选的仓配履约，已搁置。
