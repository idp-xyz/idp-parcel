# 运输收费发生项登记册与失败尝试入口

Category: enhancement
Status: resolved——登记册层（`f5a0f38`）+ 显式登记用例（`81957c7`）两层齐，棘轮那条已剪；
生产触发链未接通是 ADR-0098 决定四留的已知缺口，不是本票遗留，见文末末条
Blocked by: 无

## 这一票为什么刺眼

`ChargeOccurrenceForFailedAttempt` 要的入参 `AttemptObjectResult`，**就在
`application/perform_offsite_pickup.go` 里由 `FormAttemptObjectResult` 造出来**，隔几行没人
再用。它守的是 `AT-TF-094`（失败尝试形成发生项，第二次成功不覆盖第一次），验收判据编号写在
函数注释里。

## CONTEXT 与既有领域约束

领域侧 `TransportChargeOccurrence` 已实现：**无金额字段**（金额归 `settlement-accounting`）、
原/替代旅程不合并、事实依据与业务时间取自结果本身、**揽收到手的结果被拒**（只有失败结果能
形成失败尝试费）。

开发主线 PN-04 把「收费发生项（无金额字段、原/替代旅程不合并）」列为已收口的领域件——领域件
确实在，**入口不在**。

## 现状（锚 `9d6063c`）

领域 `domain/transport_charge_occurrence.go` 有 `TransportChargeOccurrence`、
`TransportChargeOccurrenceSpec` 与两个构造入口（常规一个、失败尝试一个）。**无表、无端口。**

## 要做的

**表**：新开迁移。**不得有金额列**——这条要写进迁移注释，因为下一个人很容易「顺手」加一个
`amount_minor`，而那会把 `settlement-accounting` 的所有权搬过来。

**约束**：
- 同一尝试的失败发生项只登一次，第二次成功**不覆盖**第一次（`AT-TF-094` 的库面镜像）。
- 原旅程与替代旅程的发生项分别成立、不合并（封闭的旅程归属列）。

**端口 + 适配器 + 编排**：把失败入口接进 `perform_offsite_pickup.go`。

> **⚠ 上面这句话是错的，2026-09-02 动笔时核出来，见文末 Comment。** 手上有
> `AttemptObjectResult` 只满足事实依据那一格，其余七格都不在那个 handler 里。这一票**不是
> 机械活**，接线前需要一次裁决。
>
> 裁决落在 [ADR-0098](../../../docs/adr/0098-a-failed-attempt-charge-occurrence-is-not-formed-inside-the-pickup-orchestration.md)：
> **不接进揽收编排**，另开显式登记用例 `application/register_failed_attempt_charge.go`，
> 采购上下文由调用方显式给出。落地情况见文末 2026-09-03 那条 Comment。

## 陷阱

- **失败尝试费是费用的「发生」不是「金额」。** 登记它不表示要收多少钱，也不表示应收应付成立；
  金额与责任由 `settlement-accounting` 依据它形成。迁移与端口注释都要说清，否则下游会把它
  当账。
- 成功尝试不形成失败尝试费——这一条领域已经拒了（`ErrNotAFailedAttempt`），库面也要镜像，
  否则绕过构造门的写入路径能落进一条不该存在的行。

## 完工判据

`ChargeOccurrenceForFailedAttempt` 有生产调用路径，棘轮基线那一行可剪。

## Comments

- 2026-09-02 · MCP-3：**动笔时核出本票不是机械活，且我票面正文那句接线方案是错的。已就地
  标注作废，未改代码。**

  **一、`perform_offsite_pickup.go` 手上没有形成发生项所需的东西。**
  `PerformOffsitePickupCommand` 的字段是租户、来源、任务、尝试、执行方、地点、计划窗口、
  到场时刻、证据、改约来源、对象清单——**没有旅程、没有责任法人、没有服务提供方、没有协议
  快照、没有范围、没有数量单位、没有有效性版本**。而 `FormTransportChargeOccurrence` 这些
  全是必备（缺一即 `ErrInvalidChargeOccurrence`）。

  我正文里写「那里已经有 `AttemptObjectResult`，判一下失败就能登记」——**那只满足了事实
  依据一格**，把「入参之一在场」当成了「入参齐备」。

  **二、再往下想一层，这一票的范围本身要收窄。** CONTEXT 说自营履约「不虚构外部供应商、
  供应商协议或外部运输委托」，而发生项的定义是「**可能依据供应商协议形成外部运输成本**的
  业务事实范围」。两句合起来：**自营揽收失败不该形成发生项**——没有外部成本可言。

  所以失败尝试费只在**外包揽收**下成立。而「这次揽收是不是外包、依哪份协议」这件事，
  **在整条揽收路径上今天不存在**：`OffsitePickup` 与 `FulfillmentAttempt` 都不带采购上下文，
  `PickupAttemptStore` 也不存。这是[分类表](../../mechanism-executor-triage/spec.md)里同一族
  的第四例——**有语言无形状**：CONTEXT 用整节写了采购责任与协议快照，而揽收这条路上没有它的
  落点。

  **三、因此本票要先裁一件**：失败尝试费发生项的采购上下文从哪来。三条路各有代价：

  - **扩 `PerformOffsitePickupCommand`**：加七个字段。代价是把采购事实塞进一条本来只报
    物理事实的命令，而调用方（节点作业侧）多半不知道协议快照。
  - **开一个采购上下文读口**（按揽收任务或旅程解析出法人/提供方/协议）。代价是多一个端口，
    但它与 CONTEXT 的所有权划分一致——采购责任本就不属揽收登记方。**我倾向这条。**
  - **不在揽收编排里形成，另开一条编排**（由持有采购上下文的一方在失败结果之后形成）。
    代价是多一次跨编排的触发，好处是两种事实各归其位。

  **裁之前不动代码。** 猜错的代价不是返工，是给一批自营揽收凭空造出外部成本来源。

  **四、体量也比票面写的大**：成员是列表（`Members []CarriedObjectReference`），要第二张
  表；另有有效性版本链（`corrects` / `revisionKind` / `revisionBasis` / `revisedAt`）四件
  成组。按票 01 的实测，这一票接近「大」而不是「中」。

- 2026-09-03 · MCP-3：**两层齐，本票 resolved。票面此前停在 `draft` 是漏登——`81957c7` 收口了
  代码，没回写票面；本条补齐。**

  **落地两笔**：`f5a0f38` 登记册层——迁移 `0007_transport_charge_occurrence.sql`（发生项与
  成员两张表，**无金额列无币种列**，迁移头注写明了为什么）、`ports/charge_occurrence.go`、
  真库适配器 `charge_occurrence_registry.go`、重建门 `charge_occurrence_rehydration.go`；
  `81957c7` 显式登记用例 `application/register_failed_attempt_charge.go`，它是
  `ChargeOccurrenceForFailedAttempt` 全仓唯一的 domain 包外非测试调用点（声明也只一处，
  不是同名误判）。

  **ADR-0098 三条各落在哪**：决定一——用例独立于揽收编排，`perform_offsite_pickup.go`
  一行未动；决定二——采购上下文七格由 `RegisterFailedAttemptChargeCommand` 显式携带，本上下文
  不推导；决定三——`Agreement` 留空即自营，答 `FailedAttemptChargeNotApplicable` /
  `SelfOperatedHasNoExternalCostSource`，且**判在 `chargeSpecFrom` 与构造门之前**：构造门把
  协议快照当必备项，落过去会报成 `ErrInvalidChargeOccurrence`，那是同一件事的错误措辞，会把
  一个正确答案说成缺件。`不适用`、`未决`、`输入未受理`三格分开，因为三者的恢复动作分别是
  什么都不用做、重试、改请求。

  **端口收窄一格**：用例只读揽收尝试，新开只读口 `FailedAttemptSource`，不依赖带 `Save` 的
  `PickupAttemptStore`——依赖它等于声明自己可能写揽收那一侧，而按 ADR-0098 本用例恰恰不碰。
  `PickupAttempts` 适配器天然满足，无第二实现。

  **棘轮**：隔离检出 `f5a0f38` 上剪前 32、剪后 31（`81957c7` 提交信）；2026-09-03 在 `371f6cb`
  上用 `git show` 取内容重数仍为 31。两个数各只对各自的 SHA 成立，谁要拿一个数当底请自己重取。

  **完工判据要读准**：「有生产调用路径」按棘轮口径成立，即 domain 包外有非测试调用方。但
  `NewRegisterFailedAttemptChargeHandler` **未进 `cmd/parcel-api` 装配**，全仓引用它的只有 TF
  包自身。这与 ADR-0098 决定四的后果原话一致——「机制齐备且可达，生产触发链仍未接通」，因为
  揽收↔委托那条连线今天不在，且 ADR 裁定今天不建（建它等于在无租户实证下先定一种采购组织
  方式）。**这是 ADR 显式留下的已知缺口，不是本票遗留**；接通那天的入口按 ADR-0098 三条约束
  走，不回头改 `perform_offsite_pickup.go`。

  **ADR-0098 后果里另一条此前无落点**：「揽收↔委托连线作为独立已知缺口记进分类表同族」。
  本日已追加到[分类表](../../mechanism-executor-triage/spec.md)末尾一节，只追加不动既有行。

  **验证（2026-09-03，共享树）**：先在 `371f6cb`、后在 `74ab82f` 各跑一遍——`go build ./...`
  与 `go vet` 退 0；`internal/architecture` 门禁 ok；TF 四包 + `migrations` 对门禁容器实跑全
  ok（postgres 包 25.3 / 25.2 秒，DSN 已设，不是跳过冒充的绿）。`-race` 仍归收尾批。
