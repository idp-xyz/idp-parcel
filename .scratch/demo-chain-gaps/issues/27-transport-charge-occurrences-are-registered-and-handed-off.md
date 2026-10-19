# 27 运输收费发生项显式登记与已形成信封（失败尝试、实际履约）

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [22](./22-supplier-agreement-version-point-read-and-tf-check-adapter.md)、[26](./26-settlement-consumes-charge-occurrence-formed-to-request-buy-evaluation.md)
地盘：TF `domain/transport_charge_occurrence.go`；`application` 新 `register_charge_occurrence.go` + `register_failed_attempt_charge.go`；新 `ports/charge_occurrence.go`；新 `postgres/charge_occurrence_handoff.go`；新 tfhttp `register_charge_occurrence.go`；`operation_decision.go`；新 `cmd/parcel-api/assemble_charge_occurrence.go` + `endpoints.go` 行；`internal/architecture/production_wiring_baseline.txt`；种子；ADR-0207、UC-TF-005 步与 AT、UC-TF-006/002 指向。
出处：[07](./07-transport-charge-occurrences-have-no-production-path.md)；方案「07 运输收费发生项」D07-1 至 D07-9，「需要的 ADR」ADR-0207。

## 做什么

- 运营决定 `TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION`，一个登记用例；本票落失败尝试与实际履约两种原因，不内联进交接或交付编排（方案 D07-3）。
- 发生条件先由登记者断言并留痕（方案 D07-1）；协议文法归 [29](./29-supplier-agreements-declare-charge-occurrence-conditions.md)。
- 采购上下文由登记方给出，经 [22](./22-supplier-agreement-version-point-read-and-tf-check-adapter.md) 的适配器核 PC；缺采购上下文与自营的答法照方案 D07-2、D07-7。
- 发生项身份与旅程照方案 D07-5、D07-6。
- 派送失败尝试经现有 `DeliveryAttemptView`，不建新存储（方案 D07-8）。
- 信封 `transport-fulfillment.charge-occurrence.formed` 的载荷、ID 与同笔保存照方案 D07-4；消费者 [26](./26-settlement-consumes-charge-occurrence-formed-to-request-buy-evaluation.md) 先登路由。
- 演示先走的原因与登记时点照方案 D07-9。
- 起草 ADR-0207（要点见方案「需要的 ADR」）；UC-TF-005 补步与 AT，UC-TF-006/002 加指向；生产接线棘轮剪掉一条。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 结果格 `REGISTERED` / `EXISTING` / `NOT_APPLICABLE`（仅显式自营）/ `UNDECIDED` / `INPUT_NOT_ACCEPTED`，加协议否定格。
- 保存与 Outbox 同一笔（真库），用指纹 ID。
- 棘轮剪掉一条。
- 不碰交接、交付编排。
- 演示：干线参与结束后登 `ACTUAL_FULFILLMENT` 答 201 → [26](./26-settlement-consumes-charge-occurrence-formed-to-request-buy-evaluation.md) → BUY 请求 → 计价评价 → ADR-0200 形成供应商预期成本（需 [30](./30-buy-cost-card-declares-amount-rounding.md)）。

## 规模

L
