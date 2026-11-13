# 网络与路由（`network-routing`）

代码在 `internal/networkrouting`。对照 main `99fc0037d`（2026-11-12）写成。

**这份文件核到什么程度**：取值表、判断结果、原因码、入口四节是从代码里读出来的。「规则」与「生命周期」两节的句子是从旧 `docs/domain/network-routing/CONTEXT.md` 提炼的，每条只用搜索确认了代码里有对应的类型、函数或码值，没有逐条跑测试验证行为；旧文档写了而代码里没有的，单列在最后一节。

## 管什么，不管什么

管：网络目录（物流节点、网络连接、线路、服务区域、服务日历、路由策略版本）及其版本，网络可用性调整，可达性判断，包裹的路由计划及其适用性，收寄与交接后的复核，改路建议与改路决定。

不管：

- 委托、包裹身份、客户地址、客户承诺、接受或拒绝——`parcel-shipment`。本上下文只交回可达性和路由结果。
- 服务产品、客户合同、法人与资格——`party-commercial`。只引用。
- 节点收寄、实测、扫描、集运单元——`node-operations`。只消费其有效结果。
- 场外揽收、交接、班次、容量、装载分配——`transport-fulfillment`。路由计划不绑定、不预占班次与容量。
- 关务区域、口岸、申报路径是否合规——`customs-compliance`。本上下文只在合格候选里选。
- 候选的成本金额——`parcel-pricing` 算，本上下文只比较不折算。
- 异常案件——`visibility-exception`。客户价格、费用、毛利——`settlement-accounting`。

## 词

| 词 | 意思 |
|---|---|
| 物流节点 | 网络里有身份的作业点。客户地址不是物流节点 |
| 网络连接 | 两个相邻节点之间一个方向上的连接 |
| 线路 | 一个或多个网络连接按顺序组成 |
| 服务区域 | 一版覆盖范围，用来解析寄件与收件地址落在哪 |
| 服务日历 | 节点、连接或线路在明确业务时区下的截单、处理时长、派送时长与衔接缓冲，按版本生效 |
| 网络可用性调整 | 对节点、连接或线路的临时停运、关闭、恢复或范围调整，不覆盖稳定定义 |
| 路由策略版本 | 一个范围与有效期内声明排序形态、冻结形态、自动改路形态及其取值的版本 |
| 可达性判断 | 委托接受前，包裹级的可达、不可达或资料不足 |
| 路由候选 | 一次判断里被评估的可能路径，保留淘汰原因；未被选中前不形成执行意图 |
| 候选成本事实 | 一个候选可比的成本：已计价（金额与币种）、待判断或不可计价。没有金额不以零表示 |
| 段计价区域 | 租户为线路某一段登记的价卡区域。本上下文原样持有，不解释 |
| 路由计划 | 一件包裹当前计划经过的节点与计划履约段，按版本保存 |
| 计划履约段 | 计划里两个边界之间的逻辑移动段，分节点间段与交付段 |
| 计划时间窗口 | 段或节点的预计范围。不是班次时刻，不是客户承诺 |
| 当前可控节点 | 依据节点收寄，或把包裹交进声明接收节点的权威运输交接，运营企业此刻能控制包裹的节点 |
| 当前控制位置 | 路由历史里业务时间最晚的那一次已判控制触发，用来判后到的触发是否过时 |
| 路由冻结边界 | 越过后轻微改善不再触发自动切换的业务边界 |
| 改路建议 | 自动条件不成立时形成的候选处置意见，本身不改当前计划 |
| 改路决定 | 对尚未执行旅程形成的新计划版本及其生效边界 |
| 无当前有效路由 | 包裹此刻没有当前有效计划。不是空计划，也不是终局失败 |
| 替代候选评估 | 对剩余旅程候选的独立评估，与原计划是否适用分开作答 |

## 取值表（代码里的封闭集）

| 轴 | 取值 |
|---|---|
| 可达性判断值 | `REACHABLE` `UNREACHABLE` `INSUFFICIENT_EVIDENCE` |
| 候选结论 | `QUALIFIED` `ELIMINATED` `EVIDENCE_UNKNOWN` |
| 硬约束结论 | `SATISFIED` `RESTRICTION_APPLIES` `STATUS_UNKNOWN` |
| 服务区域解析 | `COVERS_DESTINATION` `EXCLUDES_DESTINATION` `EXCLUDES_ORIGIN` `ADDRESS_INFORMATION_INSUFFICIENT` |
| 路由要求约束力 | `PLAIN_PREFERENCE` `COMMITTED_REQUIREMENT` |
| 排序形态 | `COST_SINGLE_DIMENSION`（未声明即未配置） |
| 候选成本状态 | `PRICED` `PENDING` `UNPRICEABLE` |
| 冻结形态 | `REMAINING_SEGMENT_COUNT`（未声明即未配置） |
| 自动改路形态 | `COST_IMPROVEMENT`（未声明即未配置） |
| 改路权限 | `AUTOMATIC_ALLOWED` `SUGGESTION_ONLY` `BARRED` |
| 改路决定方式 | `AUTOMATIC` `AUTHORIZED_ROLE` |
| 计划适用性 | `CURRENTLY_EFFECTIVE` `SUPERSEDED` `LAPSED` `CONCLUDED` |
| 计划履约段种类 | `BETWEEN_NODES` `DELIVERY` |
| 控制依据 | `NODE_INTAKE_CONTROL` `OFFSITE_PICKUP_CONTROL` `TRANSPORT_HANDOVER_CONTROL` `SOURCE_HINT` |
| 包裹事实出处 | `DECLARATION` `PHYSICAL` `PHYSICAL_WITH_DECLARATION` `DECLARATION_FALLBACK` `PHYSICAL_UNRESOLVED` `NONE` |
| 可用性调整种类 | `SUSPENSION` `CLOSURE` `RESUMPTION` `SCOPE_ADJUSTMENT` |
| 调整对象 | `NODE` `CONNECTION` `LINE` |
| 线路段成本依据 | `SUPPLIER_BUY_PLAN` `INTERNAL_POLICY` |
| 建议状态（读面） | `PENDING` `DECIDED` `SUPERSEDED` |
| 并列状态（读面） | `PENDING` `DECIDED` `FORMED_OTHERWISE` |

## 三种判断各自的结果

| 判断 | 结果 | 之后 |
|---|---|---|
| 可达性判断 | `JUDGMENT_FORMED` `EXISTING_JUDGMENT` `JUDGMENT_NOT_FORMED` `REQUEST_CONFLICT` `REQUEST_NOT_ACCEPTED` `NOT_APPLICABLE` | 形成的判断不可变；依据变了由消费方请求新判断 |
| 初始路由（每件包裹） | `ROUTE_FORMED` `NO_CURRENT_ROUTE` `NOT_FORMED` `UNDECIDED` `EXISTING_RESULT` | 路由未形成等登记补齐或授权裁决；未决回滚重投 |
| 复核 | `STILL_APPLICABLE` `PLAN_LAPSED` `FIRST_PLAN_FORMED` `REROUTED` `TRIGGER_NOT_JUDGED` `UNDECIDED` `EXISTING_RESULT` `TRIGGER_CONFLICT` `TRIGGER_NOT_ACCEPTED` | 触发未判不动计划；未决回滚重投 |

`路由未形成`、`触发未判`与未决不得互相替代：前两者是重投推不动的缺口，入账并记原因；未决是依赖故障，整封回滚重投。

路由未形成的原因：`ROUTE_EVIDENCE_NOT_CONFIGURED` `CANDIDATE_SPACE_NOT_ESTABLISHED` `CANDIDATE_EVIDENCE_INCOMPLETE` `RANKING_FORM_NOT_CONFIGURED` `CANDIDATES_TIED` `CANDIDATE_COSTS_PENDING` `CANDIDATE_COSTS_UNPRICEABLE` `CANDIDATE_COST_CURRENCIES_DIFFER` `COST_SOURCE_NOT_CONFIGURED`。

触发未判的原因：`NO_ROUTING_HISTORY` `REASSESS_EVIDENCE_NOT_CONFIGURED` `FREEZE_FORM_UNCONFIGURED` `PLAN_APPLICABILITY_UNCLEAR` `TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE` `TRIGGER_PRECEDES_CURRENT_POSITION` `TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM`。

建议上「为什么没自动」的阻塞：`AUTO_REROUTE_UNCONFIGURED` `POLICY_DOES_NOT_ALLOW_AUTOMATIC` `NOT_AT_CONTROLLED_NODE` `EXECUTED_PREFIX_AFFECTED` `IMPROVEMENT_BELOW_THRESHOLD` `UNRESOLVED_RESTRICTION/…` `OUTSTANDING_RESPONSIBILITY/…` `LOWEST_COST_TIED/…`。

授权决定被拒的结果：改路 `SUGGESTION_NOT_FOUND` `SUGGESTION_SUPERSEDED` `SUGGESTION_ALREADY_DECIDED` `CANDIDATE_NOT_SUGGESTED`；裁并列 `NO_TIE_REGISTERED` `INITIAL_ROUTE_ALREADY_FORMED` `CANDIDATE_NOT_TIED`；两者共有 `CANDIDATE_RESTRICTED` `CANDIDATE_NOT_QUALIFIED` `CANDIDATE_EVIDENCE_UNKNOWN`。

## 规则

**网络目录**

- 节点、连接、线路、服务区域、服务日历、路由策略的变化都形成新版本，带明确生效时间与适用范围，不改写既有判断。
- 连接有明确方向，两端不得相同；节点、连接、线路都必须有业务时区。
- 客户地址不入库。本上下文只存服务区域版本和当次解析所用地理投影的内容摘要。
- 服务日历的截单、处理时长、派送时长、衔接缓冲没登记就不补；登记了 0 才是零分钟。跨节点时间按绝对时刻比。
- 临时调整与稳定定义分开存，必须记来源、范围和起止时间。
- 节点的显示名称随那一版登记，只供人辨认，不进任何判断。
- 每一版可以带登记依据；采用随产品发布的参考配置是一次普通登记，不点名就不进任何租户的目录。
- 登记行缺件在入库前拒绝并指名差哪一格；目录内容是租户取值，登记口不带默认值。

**可达性**

- 可达性判断与初始路由计划是两个不同的结果，前者在委托接受前，后者在接受后。
- 资料不足不是不可达。候选证据未知时答 `INSUFFICIENT_EVIDENCE`。
- 服务产品不要求网络判断时答 `NOT_APPLICABLE`，不虚构路由。
- 判断保留判断依据时点（`asOf`）、策略版本、网络与日历依据、调整、区域解析与每个候选的淘汰原因。依据换代后旧判断答 `JUDGMENT_SUPERSEDED`，由消费方请求新判断。

**选路**

- 以包裹为计划单位。一件包裹在同一业务时点只有一个当前有效路由计划。
- 顺序固定：先排除硬约束，再过客户承诺、截单与衔接的时间可行性，最后按策略版本声明的排序形态择优。
- 客户提出的线路、口岸、节点要求只有被合同或服务产品承诺时才是约束；一般偏好不是。
- 关务不合格的候选出局；关务来源没接、目录为空或缺端点国家时限制状态是未知，不当成通过。
- 策略版本没声明排序形态时答未配置，不代租户选一种。
- 首发排序形态是成本单维：只在合格候选之间比，待判断或不可计价的候选出局，不以零金额顶替。
- 策略版本登了比较币种就换算后再比；没登而币种不齐就停下不比。
- 最低成本并列是冲突，交授权角色裁决，不按候选标识或任何无业务依据的次序收尾。同一份依据再判仍是同一个并列。
- 没有可行候选时形成无当前有效路由，不建空计划、默认计划或占位计划。
- 同一封交接里，一件包裹路由未形成不连累已判成的包裹。同一判断键再停一次，登记按最新那一次改写。
- 交付节点的服务日历登了派送时长，计划才带一段交付段；没登就止于交付节点。冻结与已执行前缀只数节点间段。
- 计划履约段在计划之外以「路由计划版本 + 段序位」被引用；按引用取到的是那一版的段内容，该版之后被替代也不变。

**包裹事实**

- 预路由用客户声明；首次收寄复核用截到收寄业务时间的物理事实；到站复核与授权改路决定用此刻仍有效的实测。
- 物理事实一项也没有才退回客户声明，并标明是退回；只有一部分时逐项取，声明只补缺的项。
- 物理事实有多条而定不下用哪一条时不退回声明，各段待判断。

**复核**

- 复核触发只认带控制依据的来源：节点收寄、场外揽收、把包裹交进声明接收节点的权威运输交接。扫描、位置一类线索不受理。
- 节点收寄与交进节点的交接建立当前可控节点；场外揽收不建立，已执行前缀为空，只按硬约束核原计划，原计划失效时只形成建议。
- 原计划是否适用与替代候选评估分开作答。原计划被硬依据证明失效就立即离场，哪怕候选评估还没出结果。
- 原计划失效且没有新计划时，包裹处于无当前有效路由；不取消委托，不回写客户承诺。
- 触发按业务时间排，不按到达顺序。早于委托接受、早于当前控制位置的触发只入账为触发未判；同一时刻照判。
- 判出失效或改善替代，而触发早于起点计划的生效时刻时，同样入账为触发未判，计划不动。
- 交接更正是新的一版，另成一次复核，原复核不被覆盖。

**改路**

- 改路只改尚未执行的部分；已执行前缀由计划段链和当前可控节点切出，原计划及其依据永久保留。
- 原计划仍适用时，越过冻结边界就保留原计划。首版冻结形态是剩余段数不超过登记的段数；没声明时答未配置，不当成未冻结。
- 自动改路要同时满足：策略版本声明了形态与阈值、包裹在当前可控节点、只动未执行部分、没有未解除的硬限制和未处理的既有责任。
- 首版自动形态是成本改善：差额严格大于阈值才自动切换。原计划失效后同样按此折，够不上阈值只形成建议。
- 有未解除的硬限制时禁行；其余任一条件不成立只形成改路建议。形态或阈值没声明也只形成建议。
- 授权角色从建议里的候选择一，决定按此刻的证据重判所择候选；被限制淘汰、不合格或证据未知都不成立。
- 建议被同一判断键上更晚的复核取代即过时，不再据以决定。一份建议至多一份决定。决定不推当前控制位置。
- 授权角色裁初始路由的最低成本并列，只能在并列的那几家里择一。

## 生命周期

- **网络定义各族**：登记一版 → 自生效时间起参与新判断 → 被更新的版本接替。只增不改。
- **可用性调整**：登记 → 到期或被恢复类调整解除。解除只恢复候选资格，不把已改路的包裹切回去。
- **可达性判断**：形成即定；依据换代后由新判断接替。
- **路由计划**：形成即当前有效 → 已被替代（新版生效）、已失效（不可执行）或已结束。离开当前有效后不得原地恢复，原路径重新可用也要重判出新版本。
- **生效时刻**取业务时间：初始计划从委托接受那一刻起；复核形成的计划从触发的业务时间起，被替代的计划同一刻离场；授权决定形成的计划从决定的时刻起，其间的无当前有效路由照留。
- **改路建议**：待决 → 已决定，或被更晚的复核取代。
- **最低成本并列**：待决 → 已裁决，或经别的路形成了计划。

## 入口

在线口（路径在 `adapters/http`，装在 `cmd/parcel-api`）：

| 做什么 | 路径 |
|---|---|
| 网络目录登记 | `/network-catalog-node-registrations` `/network-catalog-connection-registrations` `/network-catalog-line-registrations` `/network-catalog-service-area-registrations` `/network-catalog-service-calendar-registrations` `/network-catalog-availability-adjustment-registrations` `/network-catalog-route-strategy-registrations` |
| 查目录 | `/network-catalog`（`family` 取 `node` `connection` `line` `service-area` `service-calendar` `availability-adjustment` `route-strategy`） |
| 查路由判断 | `/route-plans`（`register` 取 `initial-route` `reassessment`） |
| 改路建议与决定 | `/route-reroute-suggestions` `/route-reroute-decisions` |
| 并列与裁决 | `/route-initial-route-ties` `/route-initial-route-decisions` |

事件口（`adapters/inbox`，由 `cmd/parcel-dispatch` 派发）：

| 事件 | 触发什么 |
|---|---|
| `parcel-shipment.acceptance-decision.formed` | 初始路由 |
| `parcel-shipment.network-intake.recorded` | 收寄后复核（节点收寄与场外揽收） |
| `transport-fulfillment.transport-handover.registered` | 交接进节点后复核 |

可达性判断及其有效性核对没有 HTTP 路径，由 `cmd/parcel-dispatch` 在进程内装配给委托接受链调用。计划履约段的时间窗口由 `transport-fulfillment` 经读口（`PlannedLegWindowRead`）取。

离线口：`cmd/parcel-network-register`，运营在数据库网络内手跑，不监听端口；登的是网络目录各族、段计价区域与自动改路事实（后者复核已不读，自动条件由策略版本当场折出）。

## 旧文档写了、代码里还没有的

- **路由指令**（分拣码、标签指令及改路后的作废）：搜 `路由指令` `Instruction` `分拣码`，只有一处注释提到，没有类型。
- **集运兼容判断**：搜 `集运兼容` `Consolidation` `Compatib`，本上下文零命中。`node-operations` 自己经适配器读路由依据，这里不形成判断。
- **路由偏离与改路/装载并发裁决**：`domain` 里有 `ArbitrateLoadingConcurrency`，但没有任何用例或适配器调用它，偏离结果不落库。
- **真实拆分、合并后给后继包裹另立路由**：搜 `拆分` `Split`，没有。
- **退运的独立路由计划**：搜 `退运`，没有。
- **计划「已结束」**：状态值与 `Conclude` 方法在，生产代码里没有调用方。
- **外部不透明履约段**：计划段上有 `Opaque` 一格并能存取，但没有任何生产路径把它置为真。
- **班次或装载分配失败、运输机会消失、异常处置请求、网络变化触发重评**：复核只有上面三个事件口，没有这几类触发。
- **委托层面汇总成员包裹的判断**：没有找到对应类型。
