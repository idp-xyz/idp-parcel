# `transport-fulfillment` 没有承运总单登记册——CONTEXT-MAP 判给它的身份与版本无处可登，主单级计费的身份来源缺

Category: enhancement
Status: draft
Blocked by: 无

由 [ADR-0111](../../../docs/adr/0111-shipment-and-mawb-level-billing-units-are-evaluation-subjects-in-parcel-pricing-and-settlement-allocates.md)
Decision 四「TF 今天没有承运总单登记册」与 Consequences「TF 立承运总单登记册是一张应立而未立的票，
归 TF owner」立票。本票**只写量到的事实与要裁的**，不写代码、不选形状。

## 事实（钉在 main `dc7d44e9`）

**文档侧，所有权与词条都在，只是没有册。**

- [CONTEXT-MAP](../../../docs/domain/CONTEXT-MAP.md) `transport-fulfillment` 拥有节：「承运总单、运输舱单及
  外部承运凭证的身份和版本，以及履约侧代收证据、渠道代收报告和渠道回款通知等来源证据」。
- [TF CONTEXT](../../../docs/domain/transport-fulfillment/CONTEXT.md) 用的词是「总单」不是「承运总单」：
  Rules 一节写「总单、运输舱单和外部承运凭证具有不同业务身份。总单表达主运输凭证范围……不能合并为一张
  可覆盖运输单」「总单可以引用运输委托或订舱关系，但总单本身不构成运输委托、订舱、承运接受、实际装载或
  权威交接」「对象列入总单或舱单不证明已经装载、交接或运输」「总单、舱单或外部凭证撤销不能删除已经发生的
  装载、交接和运输事实；替代凭证必须建立显式替代关系」；Boundaries 一节把「总单、运输舱单和外部承运凭证」
  列进本上下文拥有物。
- [GLOSSARY](../../../docs/domain/GLOSSARY.md)「总单」词条：「运营企业与外部运输服务提供方之间，针对明确运输
  范围形成的主运输凭证，例如适用场景中的主运单。总单可以引用运输委托或订舱关系，但自身不构成运输委托、
  订舱、承运接受或实际履约，也不证明其中每个包裹已经实际装载」；「一个总单可以关联一个或多个集运单元、
  包裹或运输履约范围，但关联必须明确且可追溯」。
- 「承运总单」这个写法只出现在 CONTEXT-MAP、ADR-0111 与 PP CONTEXT「评价对象」词条（「**承运总单**（主单级），
  身份与版本由 `transport-fulfillment` 拥有」）；TF 自己的 CONTEXT 与 GLOSSARY 都写「总单」。两个词指的是不是
  同一个对象，TF owner 开工时要先说一句。

**代码侧，一个类型都没有。**

- `internal/transportfulfillment/domain` 与 `ports` 里没有任何总单类型或引用：按 `MAWB`、`总单`、`MasterDocument`、
  `Waybill`、`Manifest`、`ConsignmentNote`、`CarrierDocument`、`TransportDocument` 逐一 grep，`domain/` 零命中。
- 「总单」只在两处注释里出现，说的都是它不存在：`internal/transportfulfillment/adapters/http/query_transport_fulfillment_records.go`
  `NewQueryTransportFulfillmentRecordsEndpoint` 头注——「页面五区里承运总单与运输舱单一区在存储上还没有登记册，
  本端点的册名封闭集刻意没有那一格——没有表就没有读法，答一份恒空的册子会把『无处可登』演成『登记册为空』」；
  `internal/transportfulfillment/ports/catalogue_read.go` 头注同一句（「本端口刻意没有那个方法」）。
- 票 [admin-skeleton-closure-batch/05](../../admin-skeleton-closure-batch/issues/05-node-operations-and-transport-fulfillment-read-faces.md)
  定稿表把「承运总单与运输舱单」记为「**无登记册（本批无端点）**——`transport_commission` 是委托订舱应答，不是
  总单/舱单；册名封闭集刻意没有这格」。
- `migrations/transport_fulfillment/` 里没有任何总单或舱单表（截至 0014；MCP-3 tf/08 在取 0015）。

**谁在等它。**

- ADR-0111 Decision 四：承运总单主体「引用 `transport-fulfillment` 拥有的承运总单身份与版本」，「主单级那一半
  **形状定、身份来源缺**：PP 侧的主体种类、聚合方式与快照形状照常落地，但生产上形成一次主单级评价要等 TF
  立册；在那之前 E2 转换工具对按 MAWB 的行如实列『未转换：等 TF 承运总单登记册』，不平摊、不折进按件定额」。
- 票 [price-card-shape-gaps/03](../../price-card-shape-gaps/issues/03-shipment-and-mawb-level-billing-units-have-no-evaluation-subject.md)
  「本票转实施票，范围」第 2 条：「主单级：领域形状随 1 一起落，快照里的承运总单引用暂无生产来源——不造替身、
  不用包裹引用顶替；等 TF 立册后接真引用」。其 Blocked by 行自本票起指向这里。

## 缺口属哪一半

- **登记册本身是机制半边**：CONTEXT-MAP 已把身份与版本判给 TF，ADR-0111 要一个可引的身份——「有一本册、
  身份怎么立、版本怎么演进、替代关系怎么记」是产品的事，今天就能做。
- **册里的内容是实例半边**：真实总单号由外部运输服务提供方分配（GLOSSARY「外部标识」词条把总单号列为外部
  标识的一种），哪家承运商、什么号段、关联哪些集运单元都是租户的事；仓库不持有一份，SYN 夹具只记 `S`。

## 可能的形状（列出，不选）

1. **独立登记册**：`transport_fulfillment` 下一张承运总单表，行身份是（租户 + 总单引用 + 版本），带主运输凭证
   范围、可缺的运输委托 / 订舱关联、显式替代关系（CONTEXT「替代凭证必须建立显式替代关系」）；关联的集运单元
   / 包裹 / 履约范围另立子表（GLOSSARY「关联必须明确且可追溯」）。读面接进 `query_transport_fulfillment_records.go`
   那个刻意留空的一格。
2. **总单与运输舱单同表按种类分格**：CONTEXT 明写两者「具有不同业务身份……不能合并为一张可覆盖运输单」，
   这一条形状要先答清「同表不等于同身份」能不能守住，否则违硬句。
3. **首发只登身份与版本，不登关联与替代**：只满足 ADR-0111「有一个身份可引」；代价是 CONTEXT 里关联与替代
   两句硬句在册上没有落点，要写明是分期而不是不做。

三条都不该在没有 TF owner 的情况下选；第 2 条与硬句的张力最大。

## 开工前置

- 先过 `/domain-modeling`：TF CONTEXT 的「总单」与 CONTEXT-MAP / ADR-0111 / PP CONTEXT 的「承运总单」是不是同一个
  词条——是则 TF CONTEXT 该加别名或统一用词，不是则要说清差别；「总单 / 运输舱单 / 外部承运凭证」三词条的
  身份边界（CONTEXT 已写「不同业务身份」）要落成登记册的键。
- 身份与版本的形状、替代关系怎么记、与运输舱单是不是分表——这三件难逆转，多半要一篇 ADR；由 owner 定要不要。
- 登记入口与读面：受控登记口归哪个 CLI（今天 TF 的登记走 `parcel-api` 端点与 dispatch，没有 `parcel-tf-register`
  这种东西——要不要开一个也是要说一句的事）；读面接进 `catalogue_read.go` 那个刻意没有的方法。

## 边界

- 不动 `parcel-pricing`：主单主体的形状归 shape-gaps/03，本票只提供身份。
- 不在仓内写任何真实总单号；不拿包裹引用或集运单元引用顶替总单身份。
- 不与 tf-segment-lifecycle-closure 混票：总单不是段生命周期的事（CONTEXT「实际履约段……不等同于……总单或舱单」）。

## Comments

- 2026-09-04 MCP-4：立票（draft），按 MCP-1 派单 task-62e8e262 只读取证。取证只到 `dc7d44e9` 上的 grep 与
  文档原句，未打开任何承运商总单样本。
