# ADR-0130：收件地点引用是 `parcel-shipment` 按（租户，包裹身份）答出的委托级复合引用——指向收件资料范围与它的资料版本锚（接受基线，或当前采用的客户原始资料版本），不另铸地点身份、不是目的地节点；读口答查询时刻的当前采用判断，`待复核`不给引用；引用一经交出含义不变，地址修订只让新查询拿到换了锚的引用，要不要新任务由 `transport-fulfillment` 判

Status: Accepted（2026-09-09，通道 2 按通道 1 派单 task-ddb77473「用户授权代裁，PS owner 口径：硬句不改、拿不准的单列越权风险点」裁决。裁决能力边界：读过票 [tf/12](../../.scratch/tf-segment-lifecycle-closure/issues/12-delivery-place-reference-seam-parcel-shipment.md) 全文与票 13 / 14 的「要裁的」、[ADR-0114](./0114-delivery-dispatch-is-triggered-by-entering-a-declared-delivery-segment-and-pulls-requirements-by-reference.md) 决定三 / 四与越权风险点 4、[ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md) 全文、[PS CONTEXT](../domain/parcel-shipment/CONTEXT.md) 委托提交版本 / 委托接受基线 / 客户原始资料版本 / 当前客户资料版本采用判断 / 资料修订阶段那几条与「客户寄收件、货物和申报原始资料在接受时形成快照」那句 Rules、[TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)「派送要求」词条、[UC-TF-006](../application/transport-fulfillment/UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md) 输入契约「派送要求」那行、`internal/transportfulfillment/ports/delivery_requirement.go` 的 `DeliveryPlaceSource` 与 `RequirementResolution`、`internal/transportfulfillment/domain` 的 `AttemptPlaceReference`、`internal/parcelshipment/domain` 的 `AcceptanceBaseline`、`SourceDataScope`、`SourceDataBasis`、`CustomerSourceDataVersion`、`CurrentSourceDataAdoption` 与 `payload_canonicalization.go` 头注「寄收件范围与服务要求尚无领域模型」。未读 TF 执行器实现与 PS 持久化适配器。）
Date: 2026-09-09

## Context

ADR-0114 决定三把末端派送任务七件里的**地点**定为「收件地点引用（`parcel-shipment`）」，由 TF 消费侧适配器拉、落进任务的 `Place` 也是引用、地址本体留在 PS；决定四与越权风险点 4 把「引用是什么身份、从哪个读面取、粒度是收件地址还是目的地节点」留给票 tf/12 与 PS owner。TF 那一头已定：`DeliveryPlaceSource.LoadDeliveryPlace(tenant, object) → (place, resolution)`，`resolution` 两格——`RequirementResolved` 带引用串，`RequirementMissing` 表示「所有者说这个对象没有收件地点」，是业务答案不是错误。

PS 这一头今天是空的。收件地址只在两处以**内容**出现：委托接受时冻进「委托接受基线」的寄收件资料快照，与接受后按「客户原始资料版本」追加的修订；两处都没有一个「地点」对象——`payload_canonicalization.go` 头注写着「寄收件范围与服务要求尚无领域模型」，PSC-1 把它们当规范化条目承载。PS 已有的是三样能当锚的东西：`AcceptanceBaseline`（接受时固定的成员集合与提交版本）、`SourceDataScope`（委托必填、包裹可空、资料组）与 `SourceDataBasis`（两态：某份既有资料版本，或接受基线）、以及按范围派生的 `CurrentSourceDataAdoption`（`已采用` 指名一版，或 `待复核`）。

用三个场景试过候选：

- **接受后地址修订再派送。** 委托接受，基线里收件地址是 A；包裹凭`已交接`进派送段，执行器向 PS 拉地点，任务形成；随后客户按 UC-PS-002 形成一份收件资料新版本 V1（地址 B），当前采用判断移到 V1。任务上那格 `Place` 指的是 A 还是 B？若引用不带版本，事后谁也说不清任务是按哪个地址形成的——`AT-TF-072`「POD 后来被证明属于错误地址」要的正是这份可追溯。若引用带版本锚，旧任务钉在 A、新查询拿到 B，两者同一委托同一收件范围而锚不同，TF 拿这一对比出「地址内容变了」，再按自己的规则判新任务还是新尝试。
- **待路由产品无目的地节点但有收件人。** 产品不走路由计划，NR 没有为它解析过任何目的地节点，但委托接受基线里有完整收件人。它照样凭`已交接`进派送段。若「收件地点引用」是目的地节点引用（候选 c），PS 只能答「没有」——对一个明明有收件人的包裹答没有收件地点是假话；票 13 那一格（时间窗）答「没有」才是真话，两条缝各答各的。
- **集运单元不在本缝。** `CarriedObjectReference` 也可能指集运单元（`node-operations` 拥有）。PS 不拥有集运单元、也没有任何委托以它为成员，答「没有收件地点」是如实的；不由 PS 拆开成员去找一个共同地址——集运单元整体末端派送是否成立本身未裁，PS 替它拼出一个地点等于替 TF 与 NO 做了那个决定。

## Decision

**一、「收件地点引用」是 PS 拥有的委托级复合引用，由租户、委托、收件资料范围、资料版本锚四段组成；不另铸地点身份，也不是目的地节点。** 资料版本锚两态，与 `SourceDataBasis` 同形：接受基线（该范围上没有任何修订版本）或某份客户原始资料版本（当前采用的那一版）。不铸地点身份（候选 a）的判据是**单一权威**：收件地址在 PS 的生命周期就是「接受时形成快照、后续更正形成新版本」，资料版本已经是它的版本，再铸一个带版本的地点身份就是同一份内容的第二套版本线，两条线要永远手工对齐。不取目的地节点（候选 c）的判据在第二个场景：节点是网络位置（GLOSSARY「物流节点不是客户地址」），收件人处不是节点，待路由产品没有节点却有收件人；UC-TF-006 输入契约把「目的地」列在 `parcel-shipment` / `party-commercial` 名下，不在 `network-routing` 名下，本记录照它读。引用是委托级而不是包裹级，因为 CONTEXT 硬句「同一委托中的包裹必须共享……寄收件关系」——同一委托的两个包裹答出同一个引用，是任何合并政策（ADR-0114 留作实例半边）成立的前提；收件资料范围若按 `SourceDataScope` 形状指名了包裹，引用照样带上它，不在这里替 `BD-PS-010` 的资料组词表决定收件资料能不能逐包裹。引用的规范串形由 PS 一处定义、TF 只当不透明串存进 `AttemptPlaceReference`；串自带形状版本（先例 `PSC-1:` / `PCC-1:`），四段之外不多一字，地址内容一个字不进串。

**二、读口按（租户，包裹身份）问，答查询时刻的当前采用判断；委托与接受基线是 PS 内部走到答案的路，不是键。** 这一句供票 14 第 2 问直接引用：**PS 对外一律按（租户，包裹身份）答，不要求消费方持有委托、接受基线或提交版本；PS 内部经接受基线成员关系（含包裹身份谱系回到来源包裹）走到委托，再取该委托收件资料范围上的当前采用判断。** 取「当前采用」而不取「接受基线」也不取「当前提交版本」：对象进派送段时委托必已`已接受`（责任起点在接受之后），「当前提交版本」是接受前的词，在这一拍不存在第二个候选；「接受基线」与「当前采用」之间选后者，因为 CONTEXT 把「当前客户资料版本采用判断」定为「派生的当前消费引用」、把「其他上下文只能消费版本引用和适用范围」定为规则——它就是为这类消费方设计的读面，而按基线派送等于对一份已被客户更正的地址照旧送。ADR-0075 在 NR 那条边选「随请求携带」是为同版性，判断对象是「委托当前提交版本」；这里没有请求可带（ADR-0114 决定三），同版性改由引用里的资料版本锚承担——TF 记下的是「按哪一版形成的任务」，事后拿锚回 PS 对证，形同 ADR-0075 决定三用摘要留痕而不落地址。答法封闭四格：范围上无修订版本 → 引用带接受基线锚；`已采用` → 引用带该版本锚；`待复核`（两条修订分叉未并）→ **不给引用，答「收件地点未定」**——PS 此刻说不出该送哪一个，给基线锚是把一份被争议的地址当成没争议；对象不属于任何已接受委托的成员集合（含集运单元、不可见对象）→ 按统一不可见结果答「没有收件地点」，不区分不存在、他租户与未授权。「未定」与「没有」在 PS 读口分格，TF 端口今天只有两格，怎么落是 TF 的事（见越权风险点 1）。

**三、引用一经交出，含义永不改；地址修订后 PS 不推送、不改旧引用，只让新查询拿到换了锚的引用；「地点变了算不算新任务」由 TF 判。** 旧锚永远解析到那一版内容——版本不可覆盖是 CONTEXT 既有硬句，引用只是借它。修订形成新版本、当前采用判断移动时，PS 不向 TF 发任何东西：ADR-0114 决定三已因触发源在 TF 内部选拉不选带，形成任务之后什么时候再拉（下一次形成尝试前？每一拍？）同样是 TF 的节拍。TF 拿新旧两个引用，委托与收件资料范围两段相同、锚不同，读作「同一收件槽位、地址内容已修订」，据此按自己的规则判新任务或新尝试；PS 不替它判。已形成派送任务不是资料修订阶段的事实——CONTEXT 六格里没有「派送中」，阶段由 PS 自有事实与 CC / NO 交出的事实判——所以派送期间能不能改地址仍由所采用规则包的资料修订允许声明说，本记录不因 TF 手里有任务而多加一道门。

## Consequences

- PS `ports` 另立一个按（租户，包裹身份）的窄读口，答上面封闭四格与引用值对象，不拓宽既有 `ShipmentRequestRepository` 一类写口（判据同 ADR-0077 决定五：伴生读端口另立）；今天没有第二个消费方，读口不预设按委托或按引用反查的方法。读口的实施票另立于 PS 地盘（[ps-port-remainder/06](../../.scratch/ps-port-remainder/issues/06-delivery-place-reference-read-face.md)），票 tf/12 的适配器以它为前置。
- 「收件地点引用」进 PS CONTEXT Language 与 Rules 各一条、GLOSSARY 一条（随本记录同笔）。TF CONTEXT「派送要求」词条已用这个词，不改。
- 「按锚解析回地址内容」的读口（一线作业端拿引用取地址）不在本记录内：它是第二个消费方、有自己的授权作用域问题（PS CONTEXT「授权查询作用域」），届时另立。今天只定引用能被这样解析，不定谁来解析。
- 收件地址仍没有自己的领域模型：本记录把它锚在资料版本上，正是为了不在 PSC-1 上就地扩列；等寄收件范围拿到模型（PSC-2），引用四段不变，只是「收件资料范围」那一段从资料组引用换成模型里的槽位引用，串带形状版本因此能换号不改旧串。
- 不在本记录内：TF 适配器的落位与全函数翻译（票 tf/12 做法节）；`RequirementResolution` 要不要加第三格（TF owner）；多包裹合并政策；`BD-PS-010` 资料组词表里「收件」怎么叫（实例半边，`PAR-COM-13`）。

## Alternatives considered

- **(a) 为每个已接受委托版本的收件范围铸一个地点身份，引用带版本。** 否决：同一份内容第二条版本线；铸出的身份没有自己的行为（收不到修订——修订走资料版本），只是一张映射表，而映射表两头各自演进时没有任何东西逼它们同步。
- **(c) 目的地节点引用。** 否决：第二个场景答假话；且把派送任务的地点变成网络节点会让 UC-TF-006 的「到场地点」与「收件方」脱钩——有效交付要「控制转移到收件方」，节点不是收件方。目的地节点是 NR 为路由解析出的注入 / 交付候选，那是票 13 那条缝隔壁的事。
- **取接受基线、永不随修订动。** 否决：对已更正地址照旧送；也与 CONTEXT「当前客户资料版本采用判断」是「当前消费引用」这句直接相悖。
- **取当前采用，但引用不带锚（只有委托 + 范围）。** 否决：第一个场景里任务按哪版形成再也说不清，`AT-TF-072` 的更正版本没有对证物；TF 也比不出「地址变了」。
- **PS 在修订形成时向 TF 推「收件地点已变」。** 否决：ADR-0114 决定三的判据原样成立——PS 不知道 TF 此刻有没有任务、要不要重立，推等于让 PS 维护一份「TF 什么时候需要」的知识；且 CONTEXT 生命周期「当前资料版本采用判断变化 → 下游重新判断」列的消费方各自消费，没有一条是推。
- **`待复核`时给基线锚或最后一版锚。** 否决：两条分叉修订等人来并，正是 UC-PS-002 禁止「最后到达者获胜」的那一格；任选一版交出去就是在读口上偷做了那次合并。
- **按委托或按接受基线作读口的键。** 否决：TF 手里只有 `CarriedObjectReference`（包裹身份，CONTEXT-MAP `parcel-shipment → transport-fulfillment` 边），要它持委托就得让它长 PS 的内部结构；包裹 → 委托是 PS 自己的成员关系与谱系，本就该在 PS 内部走。

## 越权风险点

1. **`待复核`在 TF 端口上怎么落。** `RequirementResolution` 两格里没有「未定」；今天适配器只能把它译成 `RequirementMissing`（任务待形成，结果对），但那格的字面是「所有者说没有」，与「所有者说未定」不同。是否加第三格是 TF owner 的事，在票 tf/12 实施时定（端口是 TF 地盘）。本记录只要求 PS 读口把两格分开交。
2. **对象不在成员集合时按统一不可见答「没有」会把数据完整性问题藏进一个业务答案。** 一个 TF 段里出现 PS 不认识的包裹身份，读口与「集运单元」答得一样，任务会永远停在 `REQUIREMENT_MISSING / DELIVERY_PLACE`。停点可见，但原因不可见；要不要在 PS 侧留一条区分「不是包裹」与「是包裹但不可见」的审计线，PS owner 复核。
3. **形成任务之后 TF 何时再拉。** 决定三只说 PS 不推；TF 在每次形成尝试前重拉、还是只在形成任务那一拍拉一次，决定了地址修订多久能到一线。TF owner 定；若定为「只拉一次」，用户应知道派送期间的地址更正在 TF 侧不会自动生效。
4. **基线上已有值的首次更正走哪一格。** 顺带量到：`FormCustomerSourceDataVersion` 拒「更正 + 以接受基线为基准」，而收件地址首次出现就在基线里，第一个场景里客户把 A 改成 B 今天只能走「显式清空 + 补充」两步或另有约定。它不影响本记录（读口只消费既有版本与采用判断），但影响场景一在产品上能不能发生，PS owner 复核 UC-PS-002 的本意。
5. **交付到自提点 / 代收点时，「到场地点」是不是收件地点引用。** 本记录只说引用指客户声明的收件资料；若交付方式（`party-commercial` 条件）把实际到场地点改成一个网络节点或第三方点位，任务 `Place` 该填什么是票 14 与 TF 的事，本记录不预设。

## Links

- [PS CONTEXT](../domain/parcel-shipment/CONTEXT.md)：「收件地点引用」词条与 Rules 那条（本记录的落地处）；「委托接受基线」「客户原始资料版本」「当前客户资料版本采用判断」「资料修订阶段」（本记录借用的既有生命周期）
- [TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)：「派送要求」（消费方对这个词的用法）
- [ADR-0114](./0114-delivery-dispatch-is-triggered-by-entering-a-declared-delivery-segment-and-pulls-requirements-by-reference.md)：决定三 / 四与越权风险点 4（本记录回答的那一格）
- [ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md)：不拥有地址的上下文不持有明文；以引用与摘要留痕的先例
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：适配器落 TF 侧
- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：串带形状版本的先例
- 票 [tf/12](../../.scratch/tf-segment-lifecycle-closure/issues/12-delivery-place-reference-seam-parcel-shipment.md)（三问出处）、[tf/13](../../.scratch/tf-segment-lifecycle-closure/issues/13-delivery-window-seam-network-routing.md)、[tf/14](../../.scratch/tf-segment-lifecycle-closure/issues/14-delivery-condition-reference-seam-party-commercial.md)（第 2 问引用决定二那句）、[ps-port-remainder/06](../../.scratch/ps-port-remainder/issues/06-delivery-place-reference-read-face.md)（读口实施票）
