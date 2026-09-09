# 到达事实触发建立派送任务：触发条件先裁

Category: enhancement
Status: in-progress——**转父票**（2026-09-07，通道 4，task-79675845）。四条裁决里 ①②③ 已落（派送段声明建模、ADR-0114、段服务动作、触发执行器，见「完成记录」）；④ 的三条输入缝各自成票 [12](12-delivery-place-reference-seam-parcel-shipment.md)、[13](13-delivery-window-seam-network-routing.md)、[14](14-delivery-condition-reference-seam-party-commercial.md)（均 draft，等各所有者裁），按 issue-tracker「Complete a parent」——子票全 resolved 本票才 resolved。此前状态行写的「开工前置一次 `/domain-modeling` 与三条缝各自成票」两件都已做
Blocked by: 12, 13, 14（父票阻塞边；本票自身不再有代码可做——执行器已落，缝接上一条它就往下走一格，不必回本票改代码）

## 完成记录（分支 `mcp4-tf09`，基 `origin/main` = `299f2a2e`；进 main 后的 SHA 由 MCP-1 重放时补记）

| 裁决 | 落在哪 | 分支 SHA | 触及文件 |
|---|---|---|---|
| ①「到达」= 对象凭`已交接`进入派送段；「尾程」由登记方声明 | `/domain-modeling`：TF CONTEXT 立「段服务动作」「派送要求」词条 + 生命周期①末句；UC-TF-006 触发行改口 | `8cf3b8f3` | `docs/domain/transport-fulfillment/CONTEXT.md`、`docs/application/transport-fulfillment/UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md` |
| ①②③ 成文 | ADR-0114（决定一至四、越权风险点四条）+ README 目录行 | `85956673` | `docs/adr/0114-*.md`、`docs/adr/README.md`（只加一行） |
| ① 声明落代码 | `SegmentServiceAction` 三格封闭、`DeclareServiceAction` 只在成立那一刻开门、`IsDeliverySegment`；迁移 `0018` 段表可空 `service_action` + CHECK；两条登记命令带 `SegmentServiceAction`；`SEGMENT_SERVICE_ACTION_UNKNOWN` / `CONFLICT` 两格 | `54b7ab04` | `internal/transportfulfillment/domain/segment_service_action.go`（+测试）、`actual_fulfillment_segment.go`、`segment_rehydration.go`、`application/enter_fulfillment_segment.go`、`register_transport_handover.go`、`register_offsite_pickup.go`（+测试）、`adapters/postgres/fulfillment_segment_registry.go`（+测试）、`migrations/transport_fulfillment/0018_segment_service_action.sql` |
| ③④ 三条缝的 TF 侧形状 | `DeliveryPlaceSource` / `DeliveryWindowSource` / `DeliveryConditionSource`，`RequirementResolution` 封闭二格；头注写明今天一条都没接 | `9634b635` | `internal/transportfulfillment/ports/delivery_requirement.go` |
| ③ 触发执行器 | `TriggerDeliveryDispatchHandler`：唯一触发事实、异步一拍、一对象一任务、调既有 `OpenDispatchTask`（签名不动）、缝未接答未决点名缝、所有者答「没有」任务待形成、任务引用按（段，对象，入场依据）铸、对段只读 | `e4e63503` | `internal/transportfulfillment/application/trigger_delivery_dispatch.go`（+测试） |
| ④ 三条缝各立票 | 12 / 13 / 14 | `e5a79302` | 本目录 `issues/12-*`、`13-*`、`14-*` |

**验证强度**（分支 tip 上，通道 4 量得）：`gofmt -l` 空；`go build ./...` 与 `go vet` 0；`internal/architecture` 门禁全过；全仓 `go test -p 1 -count=1 ./...`（含 DSN，PG 用例 PASS 非 SKIP）结果记在完工报与 spec 状态行——本票面不复述计数。

**进 main 记录**（2026-09-07 20:0x，MCP-1 重放到 `250e5a43` 之上；分支 → main）：`8cf3b8f3→081cfc56`、`85956673→dd757ef7`、`54b7ab04→d6c118b2`、`9634b635→d9d112d5`、`e4e63503→9f6ac00e`、`e5a79302→6ebc2aec`、`a155e11d→2346c96e`；分支清点笔 `3b1ee383` 不重放，清点在 `2346c96e` 干净检出上重生成为 `7bc47ec9`（与 `3b1ee383` 逐字节同）。七对逐笔树比对除 `.scratch/tasks.md` 外零差。隔离 detached 树钉 `7bc47ec9`：`gofmt -l` 空、build/vet 0、清点门零差、含 DSN `go test -p 1 -count=1 ./...` 100 ok / 0 FAIL / 0 cached、探针 `-run ServiceAction` 无 DSN SKIP / 有 DSN PASS。远端 `main = 7bc47ec9`（推前 ls-remote = `250e5a43`）。树 `idp-parcel-mcp4-tf09` 已拆（`git cherry main` 八笔全 `-`），指针 `mcp4-tf09@3b1ee383`、`mcp4-tf09-precut@5be2686e`、`mcp5-tf-cmdr@502a6856` 保留。

**不在本票、已另立**：三条缝的适配器（12–14）；执行器的生产入口与拍频（随第一条接上线的缝的票，ADR-0114 决定二末句）；多对象合并成一任务的政策（不立票，运营政策）；参与失效格（票 [11](11-control-withdrawing-correction-voids-participation.md)）。

**越权风险点（供 owner 复核，实现票不顺手定）**：
1. **凭有效收寄进入派送段的对象不触发**（`ENTRY_NOT_BY_HANDOVER`）——按 CONTEXT 硬句「内部触发只有一种事实：凭`已交接`进入」字面落；若 owner 认为揽收即派送的段（同城直送）也该触发，要先改硬句再改执行器一处判据。
2. **任务引用含入场依据**——照 ADR-0114 决定二字面铸（段，对象，入场依据）；来源更正后的替代参与换了入场依据，同对象同段会铸出第二个任务引用。是否该按（段，对象）铸以免更正后重开任务，owner 定；改法是 `deliveryDispatchTaskReference` 一处。
3. **成立时间取这一拍的业务时间**（命令携带 `OccurredAt`），不取对象进段时刻——依据 CONTEXT「任务在下一拍形成」；若 owner 认为成立时间该锚在触发事实上，改执行器一处。

## 裁决（MCP-3，2026-09-04）

**先纠一处前提，四问都建在它上面。** 票面写「输入是移动事实（`ARRIVAL`），所以 Blocked by 05」。CONTEXT 把这条推导链
封死了：移动事实挂班次；从班次到达推到「哪些对象到了」只能经装载分配成员，而 CONTEXT 硬句是「**装载分配不证明节点已经
完成物理装载，也不证明运输控制已经转移**」「不得仅因……装载、卸载、位置变化……认定控制已经转移」。装载分配是执行意图，
用它推对象级事实就是用计划覆盖事实（ADR-0004）。所以**班次到达事实推不出「对象到达」**，本票的输入不是 05 的产物。

对象级的「到了」在本上下文只有一种表达：**对象经权威交接进入尾程履约方的控制**——这正是 UC-TF-006 自己的第一句
「本用例从明确载运对象已经由尾程履约方取得运输控制……开始」，也是其输入契约「尾程控制」那一行。触发行的措辞
「尾程实际履约段到达派送范围」与首句不一致，以首句为准；触发行随实施票同笔对齐（`docs/application` 改文档纪律）。

**① 「到达」等于什么事实：对象凭`已交接`进入尾程实际履约段（`JoinWithHandover`），不是班次到达。** 推导链「到达事实 →
装载分配成员 → 对象」**不是 TF 拥有的判断**——它根本不是判断，是被 CONTEXT 禁止的推断。「尾程」怎么认：**由控制事实
登记方在交接登记时显式声明该段的服务动作为派送段**，照 ADR-0096「段的身份由登记方声明、不从承运商/计划段/交接范围推导」
的同一判据，不从计划段在路由里的位置推（那是 NR 的知识，且走它就连上 `PAR-NET-14`）。这一格今天领域里没有——段与参与关系
都没有「服务动作」属性，CONTEXT「揽派任务」词条里有「末端派送」这个动作词但段上没有。**这是开工前置一：`/domain-modeling`
落词条**（段的派送动作声明属机制半边，形状由硬句推得出、不依赖取值）。

**② 「派送范围」：不立 TF 词条，也不读 NR `ServiceArea`。** 按①，「到达派送范围」折成「进入派送段」，那个词不再承担任何
判断；NR 服务区域今天无地理内容列（`PAR-NET-14`），读它等于把本票挂到实例半边上。派送任务七件里的**地点**另有来源，见④。

**③ 触发落在哪一层：两条候选都否。** 不在 `RecordMovementFactHandler`（①已否其输入）；也不在 `RegisterTransportHandoverHandler`
同事务——票 06 把 `End` 放进交接编排同事务的理由是「结束参与是 TF 自己的生命周期规则、输入全在 TF」，而建派送任务的七件
里地点、时间窗、条件三样**都不在 TF**（见④），同事务调会让一条控制事实登记编排去读三个外部上下文。**触发是一个跨上下文
的内部执行器**：以「对象进入派送段」为触发，以 PS/NR/PC 各自拥有的派送要求为输入，调既有 `OpenDispatchTask`（签名不动）。
它是异步一拍不是同事务：控制事实先如实落库，任务在下一拍形成，形成失败重跑本拍、不翻交接。**输入怎么到 TF 手里**（TF
消费侧适配器拉，还是 PS/NR 把「派送要求」随信封推过界）本裁决不定——收件地址是个人信息，ADR-0075 对 NR 的裁法是「随判断请求
携带过界、不建读取端口」，TF 很可能同形；这一格在开工前置一那次 `/domain-modeling` 里一并定，很可能要一篇 ADR。

**④ 七件从哪来（每件一个所有者，TF 只拥有对象集）：**

| 件 | 来源 | 依据 |
|---|---|---|
| 对象集 | TF：进入该派送段的在场参与 | 本上下文拥有履约参与关系 |
| 地点 | PS：包裹的目的地/收件地址 | UC-TF-006 输入契约「派送要求 ← parcel-shipment / party-commercial：……目的地……」；地址不落 TF 长期表（ADR-0075 同形） |
| 时间窗 | NR：该对象计划履约段的时间窗口 | TF Boundaries「`network-routing` 拥有……时间窗口」；到达事实自己给不出时间窗，票面已察觉 |
| 条件 | PC：交付方式、收件范围、合同责任 | UC-TF-006 输入契约同一行；`PAR-NET-09` 实例半边，机制只传引用 |
| 任务引用 / 种类 / 成立时间 | TF 自铸 / `DispatchTaskKind` 派送 / 触发那一拍的业务时间 | `OpenDispatchTaskCommand` 既有形状 |

三条外部输入今天在 TF **一条缝都没有**（`internal/transportfulfillment/adapters/` 只有 `http postgres trackingsource`）。
**这是开工前置二**：每条缝各立票，落法按 ADR-0025 消费侧适配器或按 ADR-0075 随信封携带，由前置一那次建模定。

**裁后本票的去向**：不再是一张实现票——它拆成「派送段声明（领域 + CONTEXT）」「三条输入缝」「触发执行器」至少三张；拆出后
本票作父票记阻塞边，或转 superseded 指向新票。**不做的**：不改 `RecordMovementFact` / `OpenDispatchTask` 签名（票面边界不变）；
不在装载分配上长任何「对象已到达」的派生；不为「派送范围」造词。

**能力边界**：读过本票、TF `CONTEXT.md` 全文、UC-TF-006 全文、`open_dispatch_task.go` 的命令与 `Open`、
`register_transport_handover.go` 的 `Register`/`Correct` 与 Deps、`actual_fulfillment_segment.go` 的 `JoinWithHandover`、
ADR-0096/0075 的判据句。**没读** NR `CONTEXT.md` 全文与 PS 侧派送要求今天在哪个读面——③末段「随信封携带 vs 消费侧拉」因此
留给建模，不在此定。裁的是**触发的事实来源与七件的所有权归属**，不含任何时间窗、地址或交付方式取值。

## 从哪里来

票 [03](03-parcel-api-wiring-for-segment-orchestrations.md) 决策简报第 7 行：`OpenDispatchTask` 的两个触发源
是「尾程实际履约段到达派送范围」（内部触发）与「授权角色建立派送任务」（admin 写面，归票 [07](07-admin-write-faces-segment-closure-dispatch-task-load-assignment.md)）。
内部触发那一半拆票时没落进 04–07 任何一张；MCP-3 2026-09-03 裁：**另立本票，不并进 06**——并进去会把
06 从接线票变成裁决票。

输入是移动事实（票 [05](05-movement-fact-endpoint.md) 的 `RecordMovementFact`，种类 `ARRIVAL`），所以 Blocked by 05。

## 要裁的

UC-TF-006 的触发句「尾程实际履约段到达派送范围」里有两个词今天没有定义在 TF 的 CONTEXT 里：

- **「到达」等于什么事实**：到达事实的地点 = 该对象计划段的终点节点？还是班次的终点？移动事实今天
  挂在班次（`Schedule`）上，不挂在对象或段上——从一条到达事实推到「哪些对象到了」要经装载分配
  （`FormLoadAssignment` 的成员）再到对象，这条推导链是不是 TF 拥有的判断。
- **「派送范围」是谁的词**：它像 network-routing 的服务区域（`ServiceArea`），若是，TF 触发时要读 NR 的
  判断结果还是收 NR 的事件；若是 TF 自己的词，CONTEXT 要先加词条。

裁完才知道触发放在哪一层：`RecordMovementFactHandler` 落库后同事务调 `OpenDispatchTask`（同票 06 的
形状），还是由一个消费到达事实的内部执行器异步建任务。

## 边界

裁前不写代码；不改 `RecordMovementFact` 与 `OpenDispatchTask` 的既有签名。派送任务的工作范围七件
（`OpenDispatchTaskCommand`）从哪里来——对象集、地点、时间窗、条件——也在裁的范围内：到达事实
自己给不出时间窗。

## Comments

- 2026-09-07 · 通道 5（task-895fbabf → 04ffe457，旧会话）：落 `/domain-modeling` 词条、ADR-0114、段服务动作三笔；触发执行器与端口写到一半，会话失去响应，三件现场由该会话封存（`mcp5-tf-cmdr@502a6856`）。
- 2026-09-07 · 通道 4（task-79675845）：接手。按镜像测试纪律先写自己的 red 再读封存件交叉验证（判据全对上，四处不同各有取舍，记在 `e4e63503` 提交信）；封存笔按意图重切为端口一笔 + 执行器一笔，`chore(salvage)` 不进 main，证据句移进端口那一笔；立 12–14；本票转父票。分支 SHA 见「完成记录」，进 main 后由 MCP-1 补记。
- **owner 复核 2026-09-09 认可**（用户经 IDP 队列通道 1 授权代裁）：越权点即 ADR-0114 四条，逐条认可，理由在 ADR-0114「owner 复核记录」。
