# 19 派送尝试结果没有生产写入方

Category: enhancement
Status: ready-for-agent——2026-09-24 通道 2 立票（用户令通道 2 独立承接）；纯机制缺口，UC-TF-006 已写明来源与不变量，无待裁项
Blocked by: 无
地盘：`internal/transportfulfillment`（写口、持久化写入、编排、`adapters/http` 端点）、`cmd/parcel-api` 端点表一行与隔离写准入。
出处：[票 05](./05-demo-journey-criterion-evidence.md) 格 12「08 重走」新停点：交付答 `SOURCE_NOT_ACCEPTED`。

## 现象

有效交付只能落在已登记的派送尝试结果上（`RegisterEffectiveDelivery` 经 `DeliveryAttemptView` 读），而派送尝试登记册 `DeliveryAttempts` 只读、全仓没有生产写入方。它的头注写着派送尝试「由执行侧登记（与揽收侧的 PickupAttempts 对称）」——揽收侧那一半在，派送侧这一半没有。领域面在：`FormDeliveryAttemptResult`（逐对象、失败带依据、妥投不带依据、不早于到场），库表 `delivery_attempt` 与 `delivery_attempt_result` 在。

## 做什么

1. 写口与持久化：派送尝试与逐对象结果追加式登记，照揽收侧同形——同一尝试身份同内容重放答已存在，换内容答冲突；每次到场是新尝试，不覆盖旧尝试（UC-TF-006「派送尝试来源」一行）。
2. 编排：登记派送尝试结果，派送任务须已开且覆盖所报对象；业务时间与执行方随命令进来，不在服务端代铸（ADR-0023）。
3. 入口：`adapters/http` 一口，进 `cmd/parcel-api` 端点表；隔离形态照 operator-channel/08 经写开关放行，生产形态照常答 `ACCESS_CHANNEL_NOT_CONFIGURED`，等操作者渠道（前线操作者族，operator-channel/10 的作业事实能力面）。

## 不做

- 外部履约方的派送尝试来源（入向连接器）：连接器形态归[票 09](./09-tf-fulfillment-judgment-methods-and-connectors.md)。
- 由派送尝试形成运输收费发生项：UC-TF-006 另一步，本票不碰。

## 完成判据

- 领域与编排用例：首登、同身份重放、换内容冲突、任务未开或对象不在任务范围各自答复。
- 真库（含 DSN）：写入与 `LoadDeliveryResult` 读回同一份。
- 演示动线（只记 `S`）：隔离形态下登记一次妥投结果后，交付不再答 `SOURCE_NOT_ACCEPTED`，结果写回票 05 格 12。
