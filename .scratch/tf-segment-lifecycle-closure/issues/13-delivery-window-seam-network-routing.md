# 派送要求缝二：计划履约段时间窗口（`network-routing`）——`DeliveryWindowSource` 的消费侧适配器

Category: enhancement
Status: ready-for-agent——由票 [09](09-arrival-triggers-dispatch-task.md) 裁决④与 ADR-0114 决定三/四拆出（2026-09-07，通道 4，task-79675845）；「要裁的」三问已由 [ADR-0131](../../../docs/adr/0131-planned-leg-is-referenced-by-plan-version-and-ordinal-and-the-delivery-window-seam-answers-content-not-applicability.md) 答复（2026-09-09，通道 5 代裁，task-8f6d8f94），裁决摘要与做法见下
Blocked by: [nr-route-evidence-views/03](../../nr-route-evidence-views/issues/03-planned-leg-window-read-face-by-reference.md)（NR 侧计划履约段引用值类型 + 按引用取段窗口的窄读口；本票的适配器要逐字对它的拼写与序位起点）

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

## 裁决（通道 5 代裁，2026-09-09，用户经队列授权；全文与理由在 [ADR-0131](../../../docs/adr/0131-planned-leg-is-referenced-by-plan-version-and-ordinal-and-the-delivery-window-seam-answers-content-not-applicability.md)，此处只摘结论）

**① 按什么键取：按计划履约段引用取，不按对象取；引用 = 路由计划版本标识 + 段在段链中的序位。** NR 的 `PlannedLeg` 没有独立身份，是 `InitialRoutePlan.Legs()` 有序段链里的值；版本不可变、段链连续由构造门保证，「版本 + 序位」已是完整身份，NR 不另铸段标识。TF 的 `PlannedSegmentReference` 就是这个复合引用的不透明载体，拼写由 NR 一处定义（票 nr-route-evidence-views/03），TF 只存不解读。按对象取被否：NR 计划里没有服务动作，ADR-0114 决定一禁止从段位置推「尾程」；对象在执行哪条段本来就是参与关系上登记方声明的事实。**端口拓宽成立**：`LoadDeliveryWindow` 改收计划履约段引用，随本票落。

**② 哪一版：引用所钉那一版，即参与关系成立时登记方关联的版本。适用性四态不折进窗口答案。** 段序位跨版本不对应（改路的新段链从下一个可控节点起算）；对象凭`已交接`进了派送段，这一段就在执行中，「改路只能改变尚未执行的剩余旅程」说不到它；不得用后来的计划覆盖已登记的关联。当前有效 / 已被替代 / 已失效 / 已结束**一律交回该版本的段内容、答 `RESOLVED`**——适配器不读适用性表；「这一版还算不算数」由 NR 的无当前有效路由与路由偏离两个口专门回答，不借本缝的 `MISSING` 说话。这是本票红线「只翻译不判断」那一句等的裁决：不是留 `default`，是四态对窗口答案无差别。

**③ 没有计划段的对象：答 `RequirementMissing`，NR 不给兜底窗口。** 参与关系上 `PlannedSegment` 缺席时适配器不问 NR 直接答缺失（兜底窗口即 CONTEXT 禁止的「占位计划」，且 NR 造不出没有形成依据的窗口）；引用指向的版本或段在本租户下不存在也答缺失而不是读不到——重跑不会让它长出来，该显示为 `REQUIREMENT_MISSING / DELIVERY_WINDOW` 由人核声明。运营给显式窗口走票 07 的 admin 写面，不是本缝。

越权风险点五条在 ADR-0131 末节（四态常函数 / 悬空引用同答缺失 / 序位而非节点对 / 引用怎样到登记方手里未裁 / 代裁阅读范围）。

## 做法（顺序固定）

1. **等阻塞边**：nr-route-evidence-views/03 进 main，读它完成记录里的拼写形状与序位起点；本票不动 `internal/networkrouting/**`。
2. **端口拓宽（三步法，不打断替身）**：`ports.DeliveryWindowSource.LoadDeliveryWindow` 加计划履约段引用入参（对象入参是否保留由本票定，NR 侧不需要它）；expand 段先让既有替身与执行器测试同时编过，再 migrate 执行器把 `participation.PlannedSegment()` 交给端口，最后 contract 删旧签名。头注改写：按引用取、缺席由适配器答。
3. **适配器**：新建 `internal/transportfulfillment/adapters/networkrouting/`（ADR-0025 消费侧），包一层 NR 的窄读口。翻译是全函数、封闭集逐格落位：引用缺席 → `RequirementMissing`（不出 TF、不调 NR）；引用解析失败（拼写不合 NR 定义）→ `RequirementMissing`；NR 答 `found=false` → `RequirementMissing`；NR 答 `found=true` → `RequirementResolved` 带 `Earliest()/Latest()`；NR 返 error → 原样上抛（执行器读成 `DELIVERY_WINDOW_SOURCE_UNAVAILABLE`）。**不读** `PlanApplicability`，不按适用性分支——裁决②。注释中文写清为什么四态不分支、为什么悬空引用不是 unavailable。
4. **执行器**：`TriggerDeliveryDispatchHandler.pullRequirements` 把参与关系上的引用交给端口；判断逻辑不改（缺席那一格由适配器答，执行器不重复判）。既有测试替身按新签名改，不改既有断言。
5. **测试**：适配器四格各一例（缺席 / 未找到 / 找到 / 读失败）+ 一例「引用指已被替代版本的段仍答 RESOLVED」钉裁决②；执行器既有用例全绿；`internal/architecture/` 门禁绿。夹具全部合成，不写真实时间窗。
6. **生产入口**：按下节——若本票先于 12 / 14 接上线，装配（谁按拍调执行器、拍频、`cmd/parcel-api` 装配点）归本票，同笔落并在票 12 / 14 各加一条 Comment 指回；否则不重立，只把本适配器接进已有装配。
7. **票面**：完成记录逐笔 SHA、四格对照表、判断题（对象入参留不留 / 四态常函数）各一句；Status resolved；ADR-0114 决定四那句「NR 那条对没有计划段的对象怎么答」在本票完成记录写一句「已由 ADR-0131 决定三答」，不改 ADR-0114 正文。

## 完成判据

- `DeliveryWindowSource` 按计划履约段引用问；执行器把参与关系上的引用交出去，缺席不出 TF。
- 适配器对「缺席 / 解析失败 / 未找到 / 找到 / 读失败」五种输入各有唯一答格，无 `default`；对已被替代版本的段照答 `RESOLVED`（钉裁决②的测试在场）。
- 接线后执行器在票 12 已接的前提下从 `DELIVERY_WINDOW_SOURCE_NOT_WIRED` 走到下一格；票 12 未接时本票的适配器已装配但停点仍是 12 的（如实记，不算本票缺陷）。
- 不动 `internal/networkrouting/**`、不改 ADR-0114 / 0131 正文、不写真实取值；`go build ./... && go vet ./...` 退 0，TF 全包与 `internal/architecture/` 绿。

## 生产入口

同票 12「生产入口」一节：随第一条接上线的缝的实施票立；本票若先开，那一格归本票（做法第 6 步）。

## 红线

- 不从计划推事实（ADR-0004）：窗口只作工作范围，不据它认定到达、交付或任何实际事实。
- 只翻译不判断：不把「计划已失效」读成「没有窗口」也不读成「照用旧窗口」——裁决②已定：四态一律交内容，适配器不读适用性。
- 不动 `internal/networkrouting/**`（NR 侧读面归 nr-route-evidence-views/03）。
- 不写真实时间窗取值；`PAR-NET-14` 实例半边不进本票。

## Comments

- 2026-09-07 · 通道 4（task-79675845）：由票 09 裁决④拆出立票，只写票面，未动代码。
- 2026-09-09 · 通道 5（task-8f6d8f94，用户经队列授权代裁）：三问答复落 ADR-0131，NR CONTEXT 加一条规则、GLOSSARY 加一行，NR 侧窄读口立票 nr-route-evidence-views/03 并作本票阻塞边；Status draft → ready-for-agent，补做法与完成判据。只裁不码，未动 `internal/**`。
