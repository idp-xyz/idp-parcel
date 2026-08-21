# ADR-0074: TF 载运对象与 VE 包裹是两个排队主体，TF 对象链分区键带口名段

Status: Accepted  
Date: 2026-08-21

> 裁决授权：用户 2026-08-21 授权本会话（MCP-2）对
> [分区键碰撞票](../../.scratch/partition-key-space-collision/issues/01-tf-object-partitions-collide-with-ve-parcel-partitions.md)
> 的第一问代为拍板。能力边界：裁决依据为该票全文（含 T3 取证与协调岗三裁）、三个 TF 交接口
> 源码与测试、下列引文在 `b394adf` 上的原文复验；未重读 GLOSSARY 与 TF CONTEXT 全文，
> 未读 VE 四口之外的其余 outbox 口。裁的是排队主体的粒度归属，不改任何一侧的身份语义。

## Context

分区由**键值字符串**决定，不由上下文、事件类型或 handoff 决定。`transport-fulfillment` 的
交接登记与交付生效两口取（租户+对象），`visibility-exception` 的投影等口取（租户+包裹）——
而**载运对象引用与申报包裹标识是同一个字符串**（TF CONTEXT：「载运对象引用其来源上下文
拥有的身份」）。两个上下文各自合规，各自用例全绿，信封却落进同一分区并互相排队。

实测过一次（票面，基线 `f47f698`）：把场外揽收登记口并进（租户+对象）后，
`TestARegisteredOffsitePickupStopsAtUnprovenIntakeEligibility` 立刻红——一封停在
`dispatch.consumer_undecided` 的揽收信封把同一包裹**已经由 VE 受理并派生**的追踪投影堵在
分区头；换成（租户+对象+类型段）即转绿。由于「硬资格未证明」在实例半边为空的今天是常态，
这不是罕见边缘。该口因此先行取了类型段，并在注释里把「那两口要不要跟」留给了本裁决。

要裁的问题一句话：**TF 的「载运对象」与 VE 的「包裹」在分区意义上是不是同一个排队主体。**

### 两边的文本

支持「不同主体」：

- 载运对象是**包裹与集运单元的并集**（TF CONTEXT：「例如包裹或集运单元」），而 GLOSSARY
  「包裹」显式写「包裹不是委托行、面单交易、外部号码或集运单元」——两者在集合上就不相等。
- 代码里已有成建制立场：`intake_qualification_evidence.go` 专设来源种类闸，注释原话
  「场外揽收的对象来自 transport-fulfillment，两侧编号偶然同名就会误证成已证明」——仓内
  已把跨侧同名判为**偶然**并为它设防，不是判为同一主体。

支持「同一主体」：

- 载运对象按定义引用来源身份，当它是包裹时那个字符串**就是**包裹身份；
- 物理上同一件实物，且 VE 的投影正由 TF 的事实派生，两边有真实因果。

张力收在一句：**身份相同不等于排队主体相同。**「是哪一件东西」这一维引用包裹身份；
「哪些先后拍必须排一条队」这一维是另一个集合。

### 独立于文本的一个维度：那条队买到了什么

即使判为同一主体，也必须正面回答共队的收益。答案是零：

- VE 侧：投影的取代关系由来源给出（[ADR-0065](./0065-projection-versions-are-append-only-and-supersession-is-source-given.md)），
  **本就不靠到达先后**；
- TF/PS 侧：交付生效的载荷是指针式的，下游按引用**重读当前版**，不消费到达序
  （`effective_delivery_handoff.go` 的载荷注释）；
- 成本侧：头端阻塞已实测（上引红测试），且把已经成立的可见性扣到失败预算烧完。

共队因此是**只有成本没有收益**的一格。

## Decision

**一、TF 载运对象与 VE 包裹判为两个排队主体。** 同一字符串跨上下文不构成同队；
排队主体由「哪些先后拍必须排一条队」定义，不由「指的是不是同一件实物」定义。

**二、TF 对象链三口分区键统一取（租户+对象+口名段）。** 交接登记取
`/transport-handover-registration`，交付生效取 `/effective-delivery`，场外揽收登记
维持 `/offsite-pickup-registration`。口名段取各口事件名段，不新造词。三口形状取齐的
理由：同属 TF 对象链，形状不一致会让下一个人以为其中某口是特意的。

**三、随之放弃的是 TF 三口之间的跨口到达序，保住的是每口之内的对象链序。**
交接版本链、交付版本链、揽收两段成功各自仍按对象排队（`AT-TF-098` 与两条版本链的
既有裁定不动）；跨口序按上节论证买不到东西，若将来出现真实消费方依赖跨口序，凭证据
另立记录翻案——翻案成本是删一个字符串段。

**四、VE 四口维持（租户+包裹）不动。** 包裹正是 VE 的排队主体：投影、分诊、缺口、
预计的先后拍按包裹排。本裁决不动 VE 任何代码。

**五、主体名进中心登记表时以本记录为权威。**
[登记式分区主体声明票](../../.scratch/partition-key-space-collision/issues/02-partition-subject-registry-gate.md)
落地时，TF 三口的主体登记为「租户/载运对象/口名」，VE 四口登记为「租户/包裹」。

## Consequences

- 两口一测三处改动随本记录同笔落地（`transportHandoverPartitionKey`、
  `effectiveDeliveryPartitionKey` 及两份测试的分区断言）；场外揽收口注释里「那两口取的是
  光秃秃的（租户+对象）」一段随之收窄——它描述的状态已不存在。
- 两口在生产装配里零构造调用点（票面第〇节取证），无在途信封，改键**免迁移**。夹具接线的
  集成测试照常构造信封，形状随适配器走。
- `cmd/parcel-dispatch` 交付终局诚实停点用例的重拍语义随之改判：原先重拍得 0 靠的是未决
  交付信封把同分区的派生投影信封堵在队头——那正是本记录裁掉的行为；现与场外揽收用例同形，
  重拍得 1（仅派生信封经视图链定稿），交付信封自身照旧停在未决。
- 「TF 信封堵 VE 投影」这一类从此在键空间上不可能——不是靠消费方快，是键就不同。
- 本记录不解决「两个主体名其实是同一个字符串」的一般形状；那一格由登记表票承接
  （其票面已明写守不住这一类，勿删）。

## Alternatives considered

- **判同一主体，三口回到光（租户+对象）**：否。必须接受「一封未决信封扣住已成立的可见性」
  写进权威文档，而换来的跨口序没有任何消费方需要（上节论证）。
- **只留揽收口的类型段，两口不动**：否。三口同链不同形，下一个读代码的人会把差异读成语义。
- **以上下文名作段（如 `/tf`）而不是口名**：否。TF 三口彼此之间的跨口序同样买不到东西
  （各口链语义独立、下游按引用重读），上下文段会让三口重新共队，把同一类头端阻塞留在
  TF 内部；口名粒度与揽收口先例一致。
- **等登记表门禁先落地再改键**：否。登记表守的是「主体声明可见」，不守键值本身；两件事
  不互相阻塞（票面已裁「互相等会死锁」）。

## Links

- [分区键碰撞票](../../.scratch/partition-key-space-collision/issues/01-tf-object-partitions-collide-with-ve-parcel-partitions.md)：三问取证与两分支代价表，本记录裁其第一、二问
- [ADR-0065](./0065-projection-versions-are-append-only-and-supersession-is-source-given.md)：「投影的取代关系由来源给出」——跨口保序在 VE 侧无收益的出处
- [ADR-0069](./0069-customs-case-chain-ordering-absorbed-by-reread-and-retry.md)：同族先例——乱序由重读与重试消化，不建跨口保序
- [登记式分区主体声明票](../../.scratch/partition-key-space-collision/issues/02-partition-subject-registry-gate.md)：主体名的登记去处（决定五）
- [TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)：载运对象引用来源身份、包裹与集运单元并集的出处
- [GLOSSARY](../domain/GLOSSARY.md)：「包裹不是……集运单元」的出处
