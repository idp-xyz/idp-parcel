# 派送要求缝三：交付条件引用（`party-commercial`）——`DeliveryConditionSource` 的消费侧适配器

Category: enhancement
Status: draft——由票 [09](09-arrival-triggers-dispatch-task.md) 裁决④与 ADR-0114 决定三/四拆出（2026-09-07，通道 4，task-79675845）；端口形状已在 TF `ports` 立住，适配器待 PC owner 裁「条件引用指哪一版、从对象怎么走到合同」后才能开工
Blocked by: 无票阻塞；开工前置是下面「要裁的」三问得到 PC owner 的答复（本票只立票，不动 `internal/partycommercial/**`）

## 缺口

末端派送任务七件里的**条件**（交付方式、收件范围、合同责任）归 `party-commercial`（UC-TF-006 输入契约同一行；票 09 裁决④——`PAR-NET-09` 实例半边，机制只传引用）。TF 侧端口 `ports.DeliveryConditionSource.LoadDeliveryConditions(tenant, object) → (conditions, resolution)` 已随执行器立住：交回的是**条件引用**（落进任务的 `ServiceConditionReference`——「产品、合同与授权处置规则的快照」引用），不是条件内容；哪些交付方式被允许、收件范围怎么算，是 UC-TF-006 步骤 5「按服务/合同规则判断有效交付」那一步读引用去取的事，不在建任务这一拍展开。

PC 侧有相邻的东西：服务阶段正文里的责任结果声明（`DeclaredEffectiveDelivery`，ADR-0058）、客户合同版本与合同委派（ADR-0116）。但没有一个按包裹或按委托答「这个对象适用的交付条件版本引用」的读面；对象到合同的那一跳也不在 PC——委托采用了哪个合同版本是 PS 的商业依据解析结果（ADR-0080：商业闭环先解析合同再据它键结算）。缝要跨两个所有者才能接上。

## 所有者

- 交付方式、收件范围、合同责任的版本生命周期：`party-commercial`。TF 保存采用的条件引用快照，不修改商业版本。
- 对象 → 委托 → 采用的合同版本：`parcel-shipment`（商业依据解析）。TF 不自己算这一跳。
- `internal/transportfulfillment/adapters/partycommercial/` 今天已有一个消费侧适配器（承运主体身份目录，ADR-0103）；本缝的适配器与它同包并列，各答各的，不合并成一个「PC 门面」。

## 缝的形状（TF 这一头已定）

- 端口：`internal/transportfulfillment/ports/delivery_requirement.go` 的 `DeliveryConditionSource`。
- 适配器落位：`internal/transportfulfillment/adapters/partycommercial/`（ADR-0025 消费侧；只翻译不判断；全函数）。若对象→合同那一跳要先问 PS，适配器可能同时依赖 PS 的一个读面——那仍是「TF 消费两个提供方」，两份翻译各自在 TF 侧，不让 PC 去读 PS。
- 消费方：`application.TriggerDeliveryDispatchHandler`。缺席答 `DISPATCH_UNDECIDED` / `DELIVERY_CONDITION_SOURCE_NOT_WIRED`；PC 答「没有」（合同没登交付条件、或对象没有采用的合同版本）答 `REQUIREMENT_MISSING` / `DELIVERY_CONDITION`，任务保持待形成，不填默认——「不以通用签名规则替代合同」（UC-TF-006 输入契约使用约束）在这一拍就成立。

## 未接时 TF 停在哪

票 [12](12-delivery-place-reference-seam-parcel-shipment.md)、[13](13-delivery-window-seam-network-routing.md) 都接上之后，每一拍停在 `DELIVERY_CONDITION_SOURCE_NOT_WIRED`。停点语义同 12。

## 要裁的（PC owner，其中第 2 问要 PS owner 一起）

1. **条件引用指哪一版。** 候选：(a) 委托接受时解析到的合同版本引用（同版性：与 PS 商业闭环键结算用的是同一个版本，ADR-0080）；(b) 对象进派送段那一拍当前有效的合同版本；(c) 合同版本之下更细的「交付条件」子版本（若 PC 把交付方式允许集独立成版本）。任务的 `Conditions` 是快照引用，取 (a) 与「快照」一词一致；取 (b) 会让同一委托的不同包裹在不同拍拿到不同版本。
2. **对象怎么走到合同。** PS 的商业依据解析结果按什么键读（委托 / 接受基线 / 包裹）；那是 PS owner 的读面。本缝的适配器只翻译两头的答复，不自己解析商业依据。
3. **合同委派（ADR-0116）在这一格怎么读。** 委派让实际决定方与合同相对方不同；条件引用是指合同版本本身还是指委派后的决定版本，影响一线作业端拿引用去 PC 取内容时取到谁的规则。

## 生产入口

同票 12「生产入口」一节：随第一条接上线的缝的实施票立；本票若先开，那一格归本票。

## 红线

- 只传引用不传内容：交付方式允许集、收件范围规则是 `PAR-NET-09` / `PAR-COM-05/06` 实例半边，一个字都不进 TF。
- 只翻译不判断：不把「合同未登交付条件」读成「按默认签收」，不合并 PS 与 PC 的两个「没有」成一个。
- 不动 `internal/partycommercial/**` 与 `internal/parcelshipment/**`（本票只立票；两侧读面若要开，由各自 owner 在其地盘立票）。
- 不写真实合同、真实交付方式取值。

## Comments

- 2026-09-07 · 通道 4（task-79675845）：由票 09 裁决④拆出立票，只写票面，未动代码。
