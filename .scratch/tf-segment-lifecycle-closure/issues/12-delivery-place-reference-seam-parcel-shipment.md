# 派送要求缝一：收件地点引用（`parcel-shipment`）——`DeliveryPlaceSource` 的消费侧适配器

Category: enhancement
Status: draft——由票 [09](09-arrival-triggers-dispatch-task.md) 裁决④与 ADR-0114 决定三/四拆出（2026-09-07，通道 4，task-79675845）；端口形状已在 TF `ports` 立住，适配器待 PS owner 裁「地点引用是什么身份、从哪个读面取」后才能开工
Blocked by: 无票阻塞；开工前置是下面「要裁的」三问得到 PS owner 的答复（本票只立票，不动 `internal/parcelshipment/**`）

## 缺口

末端派送任务七件里的**地点**归 `parcel-shipment`（UC-TF-006 输入契约「派送要求 ← parcel-shipment / party-commercial：……目的地……」；票 09 裁决④）。TF 侧的端口 `ports.DeliveryPlaceSource.LoadDeliveryPlace(tenant, object) → (place, resolution)` 已随执行器立住：交回的是**收件地点引用**，落进任务的 `Place`（`AttemptPlaceReference`）也是引用——地址本体留在 PS，需要它的一线作业端按引用向所有者取（ADR-0114 决定三；ADR-0075 要防的「不拥有地址的上下文长期持有明文地址」在这里守住）。

今天 TF 之外没有任何类型实现这个接口；PS 侧也**没有一个可被引用的「收件地点」身份**——寄收件范围与地址今天只在委托提交的规范化载荷里作为内容出现（`internal/parcelshipment/domain/payload_canonicalization.go` 头注写着「寄收件范围与服务要求尚无领域模型」）。缝两头都缺一格，本票记的是缝，不是其中任何一头的模型。

## 所有者

- 地点（地址、收件关系）：`parcel-shipment`。TF 只引用，不落地址本体，不判断地址对错。
- 载运对象 → 包裹身份：`CarriedObjectReference` 本就是对正式包裹身份（PS 拥有）或集运单元（NO 拥有）的引用；本缝只管包裹这一种——集运单元的派送目的地不是本票的事（集运单元整体末端派送是否成立本身未裁）。

## 缝的形状（TF 这一头已定）

- 端口：`internal/transportfulfillment/ports/delivery_requirement.go` 的 `DeliveryPlaceSource`。返回 `RequirementResolved` + 引用串，或 `RequirementMissing`（所有者说这个对象没有收件地点——业务答案，不是错误）。
- 适配器落位：`internal/transportfulfillment/adapters/parcelshipment/`（ADR-0025 消费侧；只翻译不判断；翻译是全函数，PS 读面封闭集合里的每一格都要有落点，不留 `default`）。
- 消费方：`application.TriggerDeliveryDispatchHandler`。适配器缺席时它答 `DISPATCH_UNDECIDED` / `DELIVERY_PLACE_SOURCE_NOT_WIRED`；接了线而 PS 答「没有」时它答 `REQUIREMENT_MISSING` / `DELIVERY_PLACE`，任务保持待形成，不填默认。

## 未接时 TF 停在哪

执行器按地点、时间窗、条件的顺序核缝；地点在最前，所以**今天每一拍都停在 `DELIVERY_PLACE_SOURCE_NOT_WIRED`**，续办引用非空、一个任务都不开、段与交接一行不动。这是 ADR-0114 决定三要的诚实停点，不是缺陷。本票接上之后下一拍停到票 [13](13-delivery-window-seam-network-routing.md) 的那一格。

## 要裁的（PS owner）

1. **「收件地点引用」是什么身份。** 候选：(a) PS 为每个已接受委托版本的收件范围铸一个地点身份，引用带版本；(b) 引用就是（委托、接受基线、收件槽位）的复合指针，不另铸身份；(c) 目的地节点引用（NR 的词）——ADR-0114 越权风险点 4 已把「收件地址引用 vs 目的地节点引用」留给本票，且指出那是 PS owner 与本票一起定的事。取 (c) 会让派送任务的地点变成网络节点而不是收件人处，UC-TF-006 的「目的地」读法要先对齐。
2. **从哪个读面取。** PS 今天没有按包裹身份答「收件地点引用」的读口；是在 PS `ports` 上开一个按包裹身份的窄读口（同版性问题：取哪一版委托的收件范围——当前提交版本还是接受基线），还是让 PS 在委托接受时把地点引用随既有的 TF 侧信封带过来（ADR-0075 的「随请求携带」路数，但这里 TF 的触发源在内部、没有请求可以顺路带——ADR-0114 决定三已因此选拉不选带，本问只剩「拉哪个读面」）。
3. **地址修订之后引用怎么动。** 委托修订（`amendment_stage`）改了收件地址，已形成任务上的地点引用指旧版还是自动跟到新版——任务表达工作范围，改约不改身份；地点变了是不是「新任务」是 TF 的判断，但引用是否带版本决定 TF 能不能判。

## 生产入口

ADR-0114 决定二末句：执行器的生产入口（谁按拍调它、拍频、`cmd/parcel-api` 装配）随**第一条**派送要求缝的实施票立。本票若先于 13 / 14 开工，那一格归本票；否则归先开的那一票，本票不重立。

## 红线

- 不落地址本体到 TF 任何表；任务 `Place` 是引用。
- 只翻译不判断：PS 答「没有」照传 `RequirementMissing`，不补默认地点、不拿目的地节点顶替。
- 不动 `internal/parcelshipment/**`（本票只立票；PS 侧读面若要开，由 PS owner 在其地盘立票）。
- 不写真实地址、真实客户；测试用合成引用。

## Comments

- 2026-09-07 · 通道 4（task-79675845）：由票 09 裁决④拆出立票，只写票面，未动代码。TF 侧端口与执行器已落在分支 `mcp4-tf09`（见票 09 完成记录）。
