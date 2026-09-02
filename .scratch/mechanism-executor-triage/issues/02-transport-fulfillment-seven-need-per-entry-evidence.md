# TF 七条：32 条里最大的一簇，而它记的是「达标」不是「留待」

Category: chore
Status: draft
Blocked by: 无

## 为什么单开一票

`transport-fulfillment` 在基线名单上占 7 条，是七组里最大的一簇，而它在
[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)
「机制半边现状」表里记的是 **达标**（不带「显式留待」注），差量列写的是 **「无」**。

PN-04 那一行还写着「TF 编排八例对应七个 UC 全触」「端口已接满（第二十七轮：TF 24 口两口径
0 缺）」。**端口 0 缺与七条工厂零调用点并不矛盾**——那正是[票 01](./01-skeleton-criterion-and-its-instruments-measure-different-things.md)
说的：端口口径量的是「声明的端口 → 实现」，量不到「规则 → 端口」。

## 七条

`internal/transportfulfillment/domain` 下，`production_wiring_baseline.txt` 冻着：

    ChargeOccurrenceForFailedAttempt
    EstablishSegmentWithHandover
    EstablishSegmentWithPickup
    FormLoadAssignment
    OpenDispatchTask
    RecordMovementFact
    SummarizeHandovers

基线对这一组只有一句笼统的组注释：「以下各条均产出带不变式校验的领域对象，包外零引用」。

## 已核的那一条，与它揭出的形状

`EstablishSegmentWithPickup` 逐条核过（锚 `9d6063c`）：

编排 `internal/transportfulfillment/application/perform_offsite_pickup.go` **存在**，且确实在
造领域对象——`FormFulfillmentAttempt`、`FormAttemptObjectResult`、`OffsitePickup`，以及一串
`NewXxxReference`。它**不调** `EstablishSegmentWithPickup`。

也就是说：**揽收尝试成了事实，但没有人由这个事实立起实际履约段。** 这不是「切片还没开工」
（编排在、适配器在、端口满），也不是「等租户」（缺的是调用点不是数据），是**同一用例里
下游那一环没接上**。

`OpenDispatchTask` 旁证同形：编排里用着 `NewDispatchTaskReference`（任务的**引用**），而
任务**本体**的开启入口零调用点。引用得到一个从来没被开出来过的东西。

## 本票要做的

逐条核余下六条，每条答四问，**不按组作答**：

1. 该工厂产出什么，`docs/domain/transport-fulfillment/CONTEXT.md` 哪条硬句要求它（摘原句）；
2. 该能力的端口 / 迁移表 / 应用编排 / 适配器四层各自在不在；
3. 若编排在而不调它，编排实际调的是什么——**「接错」与「没接」要分开**；
4. 归到哪一格：支路未接 / 整能力未接 / 死码。

## 为什么这一组的结论对全局最要紧

TF 是**达标且差量为「无」**的四个切片之一（另三个是 PN-01、PN-05、PN-08）。带「显式留待」
注的那四个至少还有一份认可清单可查；**差量为「无」的这一个，名单上却躺着七条**。若这七条里
有任何一条是支路未接，那么「差量：无」这句话本身要改。

反过来，若七条逐条核下来都有正当理由（例如都等某个尚未到来的上游事实来源），那就把理由
逐条写进基线注释——基线自己要求的正是这个：「加一条时把理由写在它上面」。今天这一组只有
一句组注释，而基线在 2026-08-24 逐条补核时已经吃过组注释的亏。

## 边界

本票**不接线、不改代码、不改基线名单**。产出是七行带举证的分类，交人裁。
