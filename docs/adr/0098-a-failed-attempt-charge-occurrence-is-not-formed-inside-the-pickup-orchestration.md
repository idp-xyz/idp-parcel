# ADR-0098: 失败尝试费发生项不在揽收编排里形成，采购上下文显式给出而不推导

Status: Accepted
Date: 2026-09-02

## Context

`ChargeOccurrenceForFailedAttempt` 守着 `AT-TF-094`（失败尝试形成发生项，第二次成功不覆盖第一次），实现完整、有测试，**而它至今没有生产调用方**——[tf-unwired-seven/04](../../.scratch/tf-unwired-seven/issues/04-transport-charge-occurrence-registry.md) 要接的就是它。

票面原本写「接进 `perform_offsite_pickup.go`，那里已经有 `AttemptObjectResult`，判一下失败就能登记」。**动笔时核出这句是错的**（实测于 `eec39f2`）：

- `PerformOffsitePickupCommand` 只带物理事实——任务、尝试、执行方、地点、计划窗口、到场时刻、证据、改约来源、对象清单。
- `FormTransportChargeOccurrence` 必备的**旅程、采购责任法人、服务提供方、协议快照、发生范围、数量单位、有效性版本**七格，一个都不在那个 handler 里。手上有 `AttemptObjectResult` 只满足了事实依据那一格。

再往下一层，问题不是"把七个字段传进去"那么简单：

- CONTEXT 说自营履约「不虚构外部供应商、供应商协议或外部运输委托」，而发生项的定义是「可能依据**供应商协议**形成**外部运输成本**的业务事实范围」。两句合起来——**自营揽收失败不该形成发生项**，没有外部成本可言。
- 所以失败尝试费只在外包揽收下成立。而**「这次揽收是不是外包、依哪份协议」在整条揽收路径上不存在**：`OffsitePickup` 与 `FulfillmentAttempt` 都不带采购上下文，`PickupAttemptStore` 也不存。

采购上下文本身在本上下文里**是有的**——`TransportCommission` 持有 `AgreementSnapshotReference` 与 `ServiceProviderReference`。缺的不是能力，是**揽收与运输委托之间的那条连线**。

## Decision

**一、失败尝试费发生项不在揽收编排里形成。**

两条独立理由：

- CONTEXT：「运输委托、订舱、容量预占、承运接受和装载分配分别拥有业务身份、对象范围、数量、条件和生命周期。**任一对象的成功、失败或取消不能被压缩成其他对象的状态。**」把一次尝试失败当场变成一个采购发生项，正是这条禁的压缩。
- 工程后果：揽收登记必须在采购上下文不可得时照常成功（接货时间是责任起点锚，物理事实不因派生失败而回滚）。于是把形成发生项塞进同一个编排，就会造出一个**静默不发生且无人重试**的派生步骤——正是 ADR-0094 刚在接受判断链上裁掉的那一类。

**二、发生项由独立的登记用例形成，采购上下文由调用方显式给出，本上下文不推导。**

判据与 [ADR-0096](0096-a-fulfillment-segment-identity-is-declared-not-derived.md) 第二条同源：**一个可能根本不存在的东西当不了推导的输入**。自营揽收没有协议快照，不是"读不到"而是"不该有"。推导会在这一格上要么编造、要么把正确答案报成失败。

**三、自营揽收失败不形成发生项，这是正确答案不是缺席。**

用例对这一格显式作答（`不适用：自营履约无外部成本来源`），**不是停在未决、也不是报错**。三者的恢复动作完全不同：未决要重试，报错要修，而这一格**什么都不用做**。

**这一格是本 ADR 里最容易被后人当成缺陷去"修"的一处**：它看起来像"发生项没登上"。写在这里就是为了让下一个人先读到它再动手。

**四、揽收任务与运输委托之间的连线不在本 ADR 范围内，留作已知缺口。**

它是「这次揽收依哪份协议」的真正答案所在。**今天不建**：建它等于在没有任何租户实证的情况下先定一种采购组织方式（揽收是挂在委托下、还是各自独立由第三方关系相连），而那与 ADR-0096 第四条把粒度政策留给租户是同一条理由。

后果要说清：**本 ADR 之后，失败尝试费发生项的机制齐备且可达（有显式命令、有登记册），但生产触发链仍未接通**——因为那条连线还不在。这与「规则没有执行器」不是一回事，判据同 ADR-0096 的后果第一条：机制能做、缺的是声明或连线，而不是没人能做。

## Consequences

- 票 04 的范围随之改变：**建登记册 + 开显式登记用例**，不改 `perform_offsite_pickup.go` 一行。棘轮上 `ChargeOccurrenceForFailedAttempt` 那条由新用例剪掉。
- 体量比票面原估的大：成员是列表（第二张表），另有有效性版本链四件成组（`corrects` / `revisionKind` / `revisionBasis` / `revisedAt`），按票 01 的实测接近「大」而非「中」。
- 登记册本身**不得有金额或币种列**，这条不因本 ADR 改变：领域类型上就没有那些字段，金额由 `settlement-accounting` 依据发生项形成。迁移注释要写明，否则下一个人会"顺手"加一个 `amount_minor`，那等于把所有权搬过来。
- 「揽收↔委托连线」应作为一条独立的已知缺口记进[分类表](../../.scratch/mechanism-executor-triage/spec.md)的同族——**有语言无形状**：CONTEXT 用整节写了采购责任与协议快照，而揽收这条路上没有它的落点。

## Alternatives considered

- **扩 `PerformOffsitePickupCommand` 加七个采购字段。** 否决：把采购事实塞进一条只报物理事实的命令，而调用方（节点作业侧）多半不知道协议快照；更坏的是它让"没有协议"与"没填"在同一个字段上不可分辨。
- **开一个采购上下文读口，由揽收编排解析后当场形成。** 曾倾向此条，落笔时否决：它把决定一里两条理由都违反了——既压缩了对象状态，又造出一个读失败即静默不发生的派生步骤。读口本身没错，错在形成的位置。
- **由揽收出向意图触发，本上下文自己消费自己的 outbox。** 否决：本上下文今天没有自消费 inbox 的先例，为一条尚未接通的链先立一套跨进程回路，是拿架构复杂度换一个还不存在的触发方。真需要时它随时可加，且加的位置不受本 ADR 影响。
- **让自营揽收也形成发生项，协议快照留空。** 否决：`FormTransportChargeOccurrence` 会拒（协议快照必备），而放宽它等于允许一个没有外部成本来源的"外部成本发生项"存在——那正是 CONTEXT「不虚构外部供应商、供应商协议或外部运输委托」直接禁的。

## 裁决方的能力边界

本 ADR 由 MCP-3 受用户明确授权自决，与 [ADR-0096](0096-a-fulfillment-segment-identity-is-declared-not-derived.md)、[ADR-0097](0097-segment-evolution-writes-through-narrow-doors-not-a-general-update.md) 同批。读过：CONTEXT 全文、`domain/transport_charge_occurrence.go` 全文、`PerformOffsitePickupCommand` 与该编排调用的全部领域构造、`TransportCommission` 的构造入参、`ports.go` 十二组端口。**没有读**：`node-operations` 侧对揽收任务的组织方式（那是决定四留空的原因之一）、任何真实租户的采购组织方式（不存在）。

因此本 ADR 裁的是**形成发生项的位置与采购上下文的来源方向**。它**没有裁**揽收与委托怎么连（决定四显式留空），也没有裁登记用例的具体入参形状——那属票 04 的实现，届时按本 ADR 的三条约束走。

## Links

- [CONTEXT 运输收费发生项](../domain/transport-fulfillment/CONTEXT.md)
- [ADR-0096](0096-a-fulfillment-segment-identity-is-declared-not-derived.md)：「可能根本不存在的东西当不了推导的输入」同源判据
- [ADR-0094](0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)：静默不发生且无人重试的派生步骤，本 ADR 决定一第二条引它
- [tf-unwired-seven/04](../../.scratch/tf-unwired-seven/issues/04-transport-charge-occurrence-registry.md)
