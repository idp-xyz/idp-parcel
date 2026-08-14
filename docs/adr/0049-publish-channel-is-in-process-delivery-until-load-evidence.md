# ADR-0049: 集成事件的发布通道首发采用进程内直投，消息中间件等负载证据

Status: Accepted  
Date: 2026-08-14

## Context

派发一拍（`internal/platform/dispatch` 的 `Dispatcher`）与进程循环（`cmd/parcel-dispatch` 的 `Loop`）都已成型并有测试，组合根 `assembleDispatcher` 卡在一个依赖上：`eventing.Publisher` 没有生产实现。

**框架按合同不拥有它。** Bento 的 eventing 合同开篇即写，框架「只拥有技术信封、发布端口、Outbox/Inbox 幂等协议和失败分类，不拥有消费者的业务事件、订阅关系、重试业务规则或消息中间件」。整个 `go.idp.xyz/idp-bento-go` 只有两处 `Publish` 实现，都不能用于生产：`eventing.PublisherFunc` 是纯函数适配器（合同明写它不增加任何副作用，func 体仍要有人写），`testkit.RecordingPublisher` 是测试替身且被框架自己的 `production-no-testkit` 依赖规则挡在生产之外。

**本仓也从未定过发布通道形态。** [决策简报](../design/parcel-go-first-consumer-slice-decision-brief.md)的 `P-11`（`CONFIRMED`）定了 At-Least-Once、发布进程拥有运行生命周期与超时/退避/失败预算、消费方用 Inbox 或等价幂等抑制重复副作用——它没有要求传输是网络的。发布通道因此是一处真空，不是既有决定的落实。

两端都已就位，缺的只是中间那一段：投递侧十个上下文共 46 个 `Outbox*Handoff` 适配器把「发布意图与业务结果同一提交」证在真实 PostgreSQL 上（[ADR-0043](./0043-publish-intent-claimed-by-result-identity.md) 的落库形态）；消费侧有唯一一个消费者 `nrinbox.AcceptanceConsumer`，它接受一份 `eventing.Envelope` 并在自己的事务里走消费门四条。

必须现在选，因为组合根不能长期停在未决上，而红线禁止用开发替身顶住——那样 Outbox 会被真的定稿，事件真的消失。

## Decision

**首发的发布通道是进程内直投。**

1. `cmd/parcel-dispatch` 装配一个 `eventing.Publisher` 实现，按 `Envelope.Type` 路由到本进程已注册的消费者。不引入消息中间件，不动 `go.mod`，不要求租户部署任何 broker。
2. **`Publish` 返回 nil 只在消费者的 inbox 事务已提交后才成立。** 框架合同要求 nil 表示「消息传输已明确、持久地接受该事件」；进程内没有独立的传输可以充当那个「接受方」，唯一能承担这个含义的持久事实就是消费者 inbox 记录的提交。直投适配器因此同步调用消费者，其返回 nil 才算持久接受；提交结果不确定时按 `eventing.ErrPublishUncertain` 交回，由 At-Least-Once 重投，消费侧 inbox 抑制重复副作用。
3. **路由表是显式清单，没有订阅者的事件类型显式失败并入账，不静默丢弃，也不得回退成「未知类型即放行」。** 该失败按可重试处理，因而会阻塞其所在分区；这个代价可接受，论证见 Consequences。
4. **认下「消费者慢会吃掉发布失败预算」这个耦合**，并把它约束成显式配置：直投适配器对每次投递施加显式超时，与 `dispatch.Config` 的其余节奏参数一起由装配方给出、不设默认，且必须明显小于 `LeaseFor`。
5. **`eventing.Publisher` 保持为端口，直投只是它的第一个适配器。** 出现负载、故障隔离或多进程部署的证据时，换适配器即可。

## Consequences

### 什么仍然成立

进程内直投把 Outbox 从「跨进程投递的缓冲」变成了同进程内的一个泵，形状变近了，但下面三条一条不失效——它们守的从来不是距离：

- **意图与业务结果同一提交。** 46 个 handoff 证的是原子性：业务写入回滚，发布意图跟着消失。直投不碰这一点，意图仍先落 Outbox 表、仍在业务事务里。真会破坏它的是「取消 Outbox、在业务事务里直接调消费者」，见 Alternatives。
- **At-Least-Once。** 失败点没有减少，只是换了地方：消费者事务提交结果不确定时，派发方一样判不出它成没成。`P-11` 的语义原样成立。
- **消费侧 Inbox 幂等。** 上一条的直接推论。重投会真的发生（租约过期、结果不确定），inbox 恰一次门是唯一挡住重复副作用的东西，不因为调用近在咫尺就可以省。

失效的只有一条：**分区串行的代价从「跨进程的投递延迟」变成了「同进程的处理时长」。** 框架文档已写明单个无法发布的事件阻塞其所在分区的全部后续事件直到进入 `ABANDONED`；直投把消费者的整个处理时间算进「发布」里，阻塞时长因此变长。

### 为什么「无订阅者即阻塞分区」可接受

静默丢弃不可选。本仓的诚实纪律在消费门上已经表过态：毒丸也要显式拒收入账，因为不落账的拒收会让同一份毒丸永远重投。丢弃一份没人订阅的事件比毒丸更糟——它连「被拒过」的痕迹都不留，而生产方已经在自己的事务里承诺了这份意图会送出去。

那就只剩显式失败，而显式失败必然阻塞该分区。这个代价可接受，两个理由：

- **它有界。** 失败预算（`DeliveryFailure.MaxFailures`）耗尽后该条进 `ABANDONED`，分区随即解冻，事件留在库里可查。它不会永久冻结任何东西。
- **它指向正确的人。** 在进程内直投下「没有订阅者」只可能是装配错误——路由表是 `cmd/parcel-dispatch` 里的一份显式清单，缺一项就是漏装配，不是运行时的正常状态。让漏装配在第一份事件上就响亮地卡住一个分区，比让它静默吞掉一整类事件好：后者要等到有人发现下游少了数据才暴露。

代价是部署顺序有了要求：**某类事件的生产方上线之前，它的消费者必须先在路由表里。** 这是真实成本，接受它。

### 直投特有的约束与已知缺口

- **超时必须显式且明显小于 `LeaseFor`。** 否则租约会在投递返回之前过期，另一个派发器可以重新认领同一条，出现真正的并发重复处理——inbox 挡得住副作用，但两边都白跑。这条约束是直投特有的：broker 下 `Publish` 只等 broker 应答，不等消费者。
- **慢消费者同时吃掉一拍的时间与该分区的失败预算。** 框架文档专门警告过这个形状（「仅仅是慢的 Publisher 被判定为永久失败」），直投让消费者的处理时长直接落进这个判定里。第 4 条的显式超时就是为把它约束成一个能被调的数，而不是一个隐式的意外。
- **已知缺口：失败码分不出「漏装配」与「下游挂了」。** `Dispatcher.DispatchOnce` 目前对所有发布失败统一记 `dispatch.publish_failed`。无订阅者与消费者真的失败共用一个码，运维读不出该改装配还是该救下游。这需要在 `internal/platform/dispatch` 补一次失败码分格，属本记录被接受后的随附改动，不在本记录里定取值。
- **组合根的依赖面变宽。** 派发进程与被投递的消费者必然同进程，`cmd/parcel-dispatch` 因此要导入各上下文的 inbox 适配器与它们的编排。组合根本就该知道全部装配，但这使它成为全仓依赖面最宽的包。

### 可逆性的具体含义

换成 broker 时要改的只有这一个适配器与装配点。派发器、消费门、Outbox 表与 46 个 handoff 一处不动——这是「方向可逆」在本记录里的确切内容，不是一句安慰。

## Alternatives considered

- **引入消息中间件（NATS / Kafka / RabbitMQ 等）。** 否决：[ADR-0009](./0009-go-modular-monolith-and-versioned-bento-contracts.md) 已经对同一类取舍裁过——它否决「按限界上下文从首版拆分微服务」的理由是「当前没有负载、团队或发布节奏证据支撑其一致性与运维成本」。今天引入 broker 是同一类无证据分布，且它把一项部署形态要求压给尚不存在的租户。本仓开发的是卖给物流企业的产品，租户数为零，这项要求现在无从验证其代价。
- **取消 Outbox，在业务事务里直接调用消费者。** 否决：消费者的写入会挤进生产方的事务，两边成败互相拖，生产方的提交时长开始取决于消费者。这会同时推翻 ADR-0043 与决策简报的 `P-10`。
- **用 `eventing.PublisherFunc` 包一个日志或丢弃函数先跑起来。** 否决：这正是 `assembleDispatcher` 注释里那条红线要拦的东西。它让「未接线」看起来像「已接线」，而 Outbox 会被真的定稿——事件真的消失，且现场看不出来。
- **进程内直投，但静默丢弃无订阅者的事件。** 否决：见 Consequences。它与消费门「拒收也是账」的既有纪律直接冲突。
- **让 `assembleDispatcher` 继续交回未决，等第一个真实租户再定。** 否决：发布通道形态属机制半边（产品怎么把事件送到消费者），不是实例半边（租户的参数）。按[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)的两半划分，机制现在就做。

## Links

- [ADR-0009：采用 Go 模块化单体并复用版本化 Bento 技术合同](./0009-go-modular-monolith-and-versioned-bento-contracts.md)：本记录沿用其「无负载证据不分布」的判据
- [ADR-0043：发布意图由结果标识认领，重放重发同一份](./0043-publish-intent-claimed-by-result-identity.md)：本记录不改变它，只补上它落地后缺的那一段传输
- [ADR-0025：跨上下文调用的适配器落在消费侧](./0025-cross-context-adapters-live-on-the-consumer-side.md)：`nrinbox.AcceptanceConsumer` 所在位置的出处
- [Parcel Go 首个消费者切片实施决策简报](../design/parcel-go-first-consumer-slice-decision-brief.md)：`P-10`、`P-11`、`P-13` 的出处
- [开发主线：派发未装配](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)：本记录被接受并接线后，该行随之更新
