# 同来源同对象的更正版本在 PS 采用口被当作第二责任起点：`AT-PS-050` 无代码实现

Category: bug
Status: draft——MCP-3 2026-09-04 随票 tf-segment-lifecycle-closure/08 收口立票，只写量到的事实与用例原句，不写方案；未动 PS 代码
Blocked by: 无

## 量到的事实（main `4cc1bc34` 上读得，逐符号名）

transport-fulfillment 自票 tf-segment-lifecycle-closure/08 起会对同一（租户+对象+尝试）的揽收落**第二个版本**
（`OffsitePickup.Correct` 回指前版，`RegisterOffsitePickupHandler.Correct` 落新行并重交 `OffsitePickupRegistrationIntent`），
并在信封 ID 上加版本段让更正版本各自入队。parcel-shipment 这一侧收到它之后发生的事：

1. `psinbox.OffsitePickupConsumer` 译信封只取 `tenantId`/`object`/`attempt` 三维（`decodeRegisteredOffsitePickup`），
   信封里的 `pickupVersion` 一格不读。
2. `AdoptOnOffsitePickupAdapter.HandleRegisteredOffsitePickup` 按（租户+对象+尝试）`FindByKey` 读回揽收——TF 的
   `OffsitePickupRegistrations.FindByKey` 自迁移 `transport_fulfillment/0015` 起答**当前版**（链尾），所以它拿到的是
   更正后的那一代，不是信封所指的那一代。
3. `AdoptNetworkIntakeHandler.Handle` 的采用键 `ports.IntakeAdoptionKey{TenantID, Parcel, Kind, Version}` 带版本——
   新版本不撞幂等，走到后面。
4. 同一函数随后调 `handler.deps.Adoptions.FindResponsibilityStart(ctx, key.TenantID, source.Parcel())`（`AT-PS-049`
   责任起点唯一）：首登版本已采用时它命中，于是更正版本经 `handler.refuse` 落成一条**不采用**记录，答
   `IntakeSourceNotAdopted`（`SOURCE_NOT_ADOPTED`），`RefusalBasis` 为
   `RESPONSIBILITY_ALREADY_STARTED/OFFSITE_PICKUP/<首登版本>`。

也就是说，PS 今天把「同来源同对象的更正版本」与「另一个来源想开第二个责任起点」当成同一件事处理。前者是
`AT-PS-049` 要挡的竞争，后者是 `AT-PS-050` 要接的更正——代码里没有分这两格的那一道判断。

`FindResponsibilityStart` 的实现（`internal/parcelshipment/adapters/postgres/intake_adoption.go`）靠部分唯一索引
「每租户+包裹至多一行 adopted」承担唯一性；不采用记录不受该索引约束，所以第 4 步的不采用行照常落库。

## 用例原句

`UC-PS-003`「一致性、幂等与并发」节：

> 来源更正、撤销或身份关系变化形成新的采用判断版本。原责任判断和承诺历史不能删除；当前承诺影响通过带原因的新版本表达。

同用例验收表 `AT-PS-050`：

> 来源后来被更正或撤销有效性 → 形成新采用判断和必要的承诺调整版本，不删除原历史

同表 `AT-PS-049`（今天代码实现的那一条）：

> 客户送站与场外揽收都指向同一包裹时不能形成两个责任起点，先合法形成者保留

票 tf-segment-lifecycle-closure/08 裁决里「新版本再采用一次是正确行为——PS 的责任起点与正式承诺生效时间锚在
`OccurredAt` 上，发生时刻被更正时 PS 必须再判一次」是按 `PickupResultVersion` 自注推的；裁决方自陈没读 PS 侧
编排。本票记的是读了之后量到的差。

## 与本票相邻、但不在本票的

- TF 侧信封已带 `pickupVersion`，PS 消费者要不要按版本而不是按键读回（交付那一路的 `FindByKeyAndVersion`
  是为同一件事开的口），与本票同源，但先由本票裁「更正版本在采用口算什么」再定读法。
- 段侧「来源更正 → 参与关系重派生」是 TF 自己的半边，在 tf-segment-lifecycle-closure/10。

## Comments

- 2026-09-04 · MCP-3：立票。起因是 tf/08 落更正链时按 MCP-1 指令核 PS 采用口对第二版本的处置——与裁决预期不同，
  以 PS 票面为准；TF 侧照裁决落新版本并重交意图，**不改 PS**。只写票面，未动代码。
