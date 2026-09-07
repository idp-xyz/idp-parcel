# 派送要求缝二：计划履约段时间窗口（`network-routing`）——`DeliveryWindowSource` 的消费侧适配器

Category: enhancement
Status: draft——由票 [09](09-arrival-triggers-dispatch-task.md) 裁决④与 ADR-0114 决定三/四拆出（2026-09-07，通道 4，task-79675845）；端口形状已在 TF `ports` 立住，适配器待 NR owner 裁「按什么键取哪一版计划的哪一段窗口」后才能开工
Blocked by: 无票阻塞；开工前置是下面「要裁的」三问得到 NR owner 的答复（本票只立票，不动 `internal/networkrouting/**`）

## 缺口

末端派送任务七件里的**时间窗**归 `network-routing`（TF CONTEXT Boundaries「`network-routing` 拥有……时间窗口」；票 09 裁决④——到达事实自己给不出时间窗）。TF 侧端口 `ports.DeliveryWindowSource.LoadDeliveryWindow(tenant, object) → (from, to, resolution)` 已随执行器立住：窗口是**计划**，任务照抄它作工作范围，不据它推任何实际事实（ADR-0004）。

NR 侧有这个东西：`InitialRoutePlan` 的每条 `PlannedLeg` 带 `PlannedTimeWindow{earliest, latest, basis}`（`internal/networkrouting/domain/initial_route_plan.go`）。但它在**判断本体（jsonb）**里，NR 的两个判断口按完整判断键取单行、伴生列表读口只上检索列面（`route_plan_read.go` 头注明写「本口不从 jsonb 里抠字段冒充列」）。没有一个按载运对象或按计划履约段引用答「这一段的窗口」的读面。

## 所有者

- 计划、计划履约段、时间窗口：`network-routing`。TF 引用有效计划形成执行准备，不修改计划。
- 对象与它关联的计划履约段的对应：TF 的履约参与关系上有 `PlannedSegment`（可缺席——待路由产品此刻没有计划段）。这一格是 TF 拥有的关联，不是 NR 的。

## 缝的形状（TF 这一头已定，但有一问可能拓宽端口）

- 端口：`internal/transportfulfillment/ports/delivery_requirement.go` 的 `DeliveryWindowSource`，按**对象**问。
- 适配器落位：`internal/transportfulfillment/adapters/networkrouting/`（ADR-0025 消费侧；只翻译不判断；全函数）。
- 消费方：`application.TriggerDeliveryDispatchHandler`。缺席答 `DISPATCH_UNDECIDED` / `DELIVERY_WINDOW_SOURCE_NOT_WIRED`；NR 答「没有」（对象没有计划段、或计划没有派送那一段）答 `REQUIREMENT_MISSING` / `DELIVERY_WINDOW`，任务保持待形成。
- **可能的拓宽**：执行器手里有该对象参与关系上的 `PlannedSegment` 引用，按它问比按对象问少一跳且无歧义；今天端口只收对象。若 NR owner 裁按计划履约段引用取，端口加一个入参（TF 地盘内的改动，随本票落，走三步法不打断替身）。

## 未接时 TF 停在哪

票 [12](12-delivery-place-reference-seam-parcel-shipment.md) 接上之后，每一拍停在 `DELIVERY_WINDOW_SOURCE_NOT_WIRED`；12 未接时轮不到本格。停点语义同 12：续办引用非空、不开任务、不动段与交接。

## 要裁的（NR owner）

1. **按什么键取。** 对象 → 委托 → 当前有效计划版本 → 哪一条 `PlannedLeg` 是派送段？NR 的计划里没有「服务动作」一格（TF 的段服务动作是 TF 登记方声明的，ADR-0114 决定一明说不从计划段位置推「尾程」）；反过来，从 TF 参与关系上的 `PlannedSegment` 引用直接定位到那条 leg 就不需要推——前提是那个引用与 NR 的 leg 身份是同一套词。要 NR owner 确认 `PlannedSegmentReference` 指的就是 NR 计划里的 leg 身份、且带计划版本。
2. **哪一版计划。** 计划有适用性（当前有效 / 已被替代 / 失效 / 结束）；对象进派送段那一拍，取的是当时当前有效的那一版，还是参与关系成立时关联的那一版（可能已被改路替代）。取后者与「任何关联都不得用实际事实覆盖原计划」一致；取前者会让任务窗口随改路变，而任务已形成后改约是「新尝试不是新任务」——两种读法对任务的影响不同，先裁。
3. **没有计划段的对象怎么答。** 待路由产品可以在没有计划段时照样实际揽收、进段；此时窗口不存在——按 ADR-0114 决定三答 `RequirementMissing`（任务待形成），还是由运营给一个显式窗口另立任务（那是票 07 admin 写面「建派送任务」的路，不是本缝）。本票倾向前者；NR owner 若认为 NR 应对无计划对象给「兜底窗口」，那是一条新规则，先改 NR CONTEXT 不在这里定。

## 生产入口

同票 12「生产入口」一节：随第一条接上线的缝的实施票立；本票若先开，那一格归本票。

## 红线

- 不从计划推事实（ADR-0004）：窗口只作工作范围，不据它认定到达、交付或任何实际事实。
- 只翻译不判断：不把「计划已失效」读成「没有窗口」也不读成「照用旧窗口」——那是判断，先裁再落。
- 不动 `internal/networkrouting/**`（本票只立票；NR 侧读面若要开，由 NR owner 在其地盘立票）。
- 不写真实时间窗取值；`PAR-NET-14` 实例半边不进本票。

## Comments

- 2026-09-07 · 通道 4（task-79675845）：由票 09 裁决④拆出立票，只写票面，未动代码。
