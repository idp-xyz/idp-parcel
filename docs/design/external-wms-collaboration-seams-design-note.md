# 外部 WMS 协作缝设计备忘：商品库存与仓配系统从哪个缝和本产品交接

本文只做一件事：租户或它的客户用外部 WMS（商品库存与仓配系统）时，把 WMS 与本产品之间
每一类交接预先指到既有限界上下文的所有权与端口缝位上，防止第一个带 WMS 的租户到来那天，
库存、拣货或出库的语义被塞进节点作业，或者适配器落进错误的上下文。**只指缝，不实现，不排
期，不改首发范围。**

依据是 [ADR-0006](../adr/0006-parcel-node-operations-not-full-wms.md) Decision 里的
「真实业务同时需要完整仓储管理时，应通过明确边界协作或另行作出领域扩展决策。」本文是「明
确边界协作」那一支；「另行作出领域扩展决策」那一支是
[ADR-0368](../adr/0368-commodity-inventory-and-warehouse-fulfillment-join-the-product-as-a-new-context-and-node-operations-stays-non-wms.md)
（2026-11-06 接受）：本产品自己立了仓配履约上下文。两支并存——租户自带 WMS 时照本文接；
用本产品的仓配履约时，下表小包一侧的落点不变，变的只是对端与接入方式，见文末「触发与消费方式」。

进入条件沿用开发主线的[能力范围判据](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md#能力范围判据)：
指得出一类真实存在、在目标客户群里属常规形态的承运或渠道商业模式需要它，才进。指得出来时
按本文缝位立票；任何取值仍由价卡、商业政策版本或租户登记携带。

## WMS 在本产品眼里站在哪一侧

WMS 可以是货主自己的、海外仓服务商的，也可以是租户自己经营仓配时用的（[目标客户群](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md#目标客户群)
里「以仓配或集运为主业，跨境干线是其中一段」那一层）。不论归谁经营，它对本产品说话时都站
在货主一侧：委托总挂在某个货主客户账户下，WMS 以那个账户的名义经客户渠道提交——产品自有
的首方客户族，或与它并列的既有渠道适配族（两族的分工见
[ADR-0139](../adr/0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)）。

走首方客户族时有一处今天就要知道的限制。首方族首发只有直客 1:1：「客户系统在册上绑定唯一（租户，
客户账户）」（[ADR-0310](../adr/0310-first-party-customer-command-five-faces-launch-with-direct-customer-one-to-one-and-command-prefixed-request-keys.md)
决定二）。一个 WMS 代多个货主提交，是 ADR-0310 留给 first-party-api/01 的「代理人 / 平台
1:N」形态；在那之前，可行的做法是每个货主账户各登记一份客户系统。

## 缝位

| WMS 一侧发生的事 | 所有权上下文 | 既有缝位 | 要点 |
|---|---|---|---|
| 打包完成，要为这件货下运输委托 | `parcel-shipment` | 提交委托（[UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)；首方族的口是 `POST /v1/shipment-requests`） | 打包称得的重量与尺寸作「声明测量」随委托进来，不是节点实测，也不是计费重量；货物内容是「客户原始资料」，不引用 WMS 的商品主数据（两词见[小包托运 CONTEXT](../domain/parcel-shipment/CONTEXT.md)）。WMS 用自己稳定的单号（如出库单号）作接入请求标识，答复丢失时原样重发取回原结果（[首方 API 契约「重放」](../api/first-party-api-contract.md#重放)） |
| 货送到租户的网络节点 | `node-operations` | 送站预报与送站交接单（[UC-NO-006](../application/node-operations/UC-NO-006-ESTABLISH-A-VERIFIABLE-DELIVERY-SCOPE.md)） | 送站预报或绑定已全部确认的送站交接单给出可验证交付范围，帮节点在收寄时消歧（[ADR-0212](../adr/0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md)）；节点作业「不因它形成委托、包裹或客户承诺」。WMS 的「已出库」「已发运」不是节点收寄：节点收寄要节点明确接收并取得控制（[节点作业 CONTEXT](../domain/node-operations/CONTEXT.md)「节点收寄」） |
| 网络到仓库上门提货 | `transport-fulfillment` | 场外揽收（[UC-TF-002](../application/transport-fulfillment/UC-TF-002-PERFORM-OFFSITE-PICKUP.md)） | 揽收形成运输控制；WMS 记下的交货是对端的声明，不替代揽收事实 |
| 货主或 WMS 要截单 | `parcel-shipment` | 撤回尚未决定的委托（[UC-PS-005](../application/parcel-shipment/UC-PS-005-WITHDRAW-SUBMITTED-SHIPMENT-REQUEST.md)）；逐包裹取消与收寄后服务处置（[UC-PS-006](../application/parcel-shipment/UC-PS-006-CANCEL-PARCEL-OR-COORDINATE-POST-INTAKE-DISPOSITION.md)） | 委托尚无接受或拒绝决定时整份撤回；已接受、未越过取消边界时逐包裹取消；越过之后取消不成立，转由运营方形成收寄后服务处置决定（[ADR-0332](../adr/0332-a-post-intake-service-disposition-is-an-append-only-per-parcel-decision-chain-that-asks-object-owners-for-scoped-requests-through-an-outbox-envelope.md)）。停拣、出库拦截是 WMS 自己的决定，两边各判各的，不互相代判。WMS 知道结果靠「写口回执、同请求重放与客户追踪视图」（ADR-0310 决定四），没有委托状态读口 |
| 退件回到仓库 | `transport-fulfillment`（退运旅程与交付）、`node-operations`（回到网络节点时的收寄） | 退运旅程（[UC-TF-007](../application/transport-fulfillment/UC-TF-007-START-ALTERNATE-OR-RETURN-JOURNEY.md)）；派送与交付证明（[UC-TF-006](../application/transport-fulfillment/UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md)）；节点收寄与待识别实物 | 退运是关联而独立的新旅程，不倒退原旅程（[运输履约 CONTEXT](../domain/transport-fulfillment/CONTEXT.md)）；送回 WMS 仓库是这段旅程的交付，签收方是仓库。回到租户节点的退件照节点收寄办，认不出的建待识别实物，「不得冒充正式包裹、集运单元或库存项目」。拆包、质检与上架回库是 WMS 的库存记账，本产品不记 |
| 仓储作业要收费（入库、仓租、拣货打包、增值服务） | 无 | 无 | [结算与经营核算 CONTEXT](../domain/settlement-accounting/CONTEXT.md) 为费用列的引用来源里，仓储作业收费发生项只来自本产品的仓配履约（ADR-0368），外部 WMS 的作业不在其中。外部 WMS 的仓储费今天由它自己出账；要并进本产品的对账单是一项新的决定，不在本文 |
| WMS 的拣货、打包、出库事件要给客户看 | `visibility-exception` | 无 | 追踪只投影事实所有者已接受的事实；WMS 事件今天没有所有者收编，不进追踪。要进，先指定所有者并在它的 `CONTEXT.md` 定词，不直插投影——与[渠道适配缝设计备忘](./channel-adapter-seams-design-note.md)「轨迹采集 / 状态同步」一行同一条规则 |
| 货主、SKU 与库存 | `party-commercial`（货主客户账户）；外部 WMS 的 SKU 与库存不进本产品（本产品自营仓配的那份归 `warehouse-fulfillment`） | 客户系统登记（ADR-0310 决定二） | WMS 的货主编码对到本产品的货主客户账户，对应关系随客户系统登记绑定。节点的位置与控制「不等同于可用库存或仓储账面数量」（ADR-0006），本产品也不收 WMS 的库存快照 |

## 不做清单

- **不在节点作业加库存、库位或拣货字段。** ADR-0006 Consequences：「涉及完整仓储职责的客户
  场景需要与外部能力协作，不能通过不断增加场站字段隐式吸收。」
- **不立「仓储中心」或通用 WMS 适配上下文。** 上表各类交接分属既有上下文；合成一个读写面
  会穿 [CONTEXT-MAP](../domain/CONTEXT-MAP.md) 的所有权边界，与渠道适配缝设计备忘不立「渠道
  中心」上下文是同一条理由。
- **不替外部 WMS 记库存，不收它的库存快照，不替它做 SKU 主数据、FNSKU 换标或 FBA 入库计划。**
  这些是外部 WMS 自己的仓内商品作业；本产品自己的仓配履约记的是它自己经营的库存。
- **不实现任何适配器、不排期。** 本文是缝位登记，不是工作包。

## 触发与消费方式

第一个带外部 WMS 的租户或客户场景被指名时：按上表一行一张票，落在所有权上下文的
`adapters/` 下；涉及新的外部事实形态时，先回该上下文的 `CONTEXT.md` 对词汇，不在适配器里
发明第二套语言。委托、送站与截单这几类交接，首方渠道已有对应的口（[首方 API 契约「首方族
今天开着的口」](../api/first-party-api-contract.md#首方族今天开着的口)），通常只缺租户侧的
渠道登记与对接，不缺机制。

用本产品的仓配履约时，对端从外部 WMS 变成仓配履约上下文。小包一侧的落点不变；「打包完成」
「截单」「仓储作业要收费」「WMS 的拣货、打包、出库事件」几行的接入方式按 ADR-0368 走：委托由
仓配履约以货主名义提交（来源「仓配出库」），仓储作业费经它的收费发生项交计价与结算，其余行
不受影响。
