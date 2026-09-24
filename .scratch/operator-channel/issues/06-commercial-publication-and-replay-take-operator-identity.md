# 06 商业发布批准链与计价回放端点接上操作者身份

Category: enhancement
Status: ready-for-agent——2026-09-24 拆法经用户授权通道 4 自决认可
Blocked by: 04
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`cmd/parcel-api` 里商业发布（`/commercial-publications`）与计价回放（`/pricing-evaluation-replays`）两处的 Intake 装配，以及把 `OperatorEnvelope` 译成 party-commercial「操作者主体引用 + 授予集」消费面的那一段。
出处：[ADR-0126](../../../docs/adr/0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md) 决定五「Intake 到位那天把信封译成它」；[ADR-0124](../../../docs/adr/0124-evaluation-replay-is-triggered-through-a-parcel-api-command-endpoint-and-never-handed-to-settlement.md) 决定六「本端点的真操作者 Intake 随 ADR-0100 那一族在装配点逐端点换」。

## 做什么

1. 商业发布：录入者与批准者两身份都从 `OperatorEnvelope` 来；审批职责规则未登记时批准门照旧答未配置。
2. 计价回放端点换操作者 Intake。

## 不做

- 审批职责规则的取值与登记入口（租户取值与另一张票）；角色模型里有哪些授予格归 [psb/07](../../product-strategy-boundary/issues/07-pc-authorization-coordinates-and-role-models.md)，本票只接信封，不阻于它（ADR-0126 已把规则形态定成两格）。

## 完成判据

- 发布待批准载体上录入者与批准者都来自信封（带测试）；回放端点三格各答其格。
