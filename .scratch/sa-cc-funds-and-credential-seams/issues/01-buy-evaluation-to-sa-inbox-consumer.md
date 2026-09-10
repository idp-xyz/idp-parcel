# BUY 评价已发出的信封没有 SA 侧消费者：`parcel-pricing.evaluation.recorded` 落进 Outbox 后无人接，`FormSupplierExpectedCostHandler` 只有测试调得到

Category: enhancement
Status: ready-for-agent——2026-09-10 17:3x 通道 5 按通道 1 派单 task-9a2ff746（用户授权代裁）写入裁决：「要裁的」1 取 (c)，本票范围是「信封到未决」，三件引用等 [08](08-sa-evaluation-request-orchestration-records-source-references.md)（见「要裁的」下「裁决」）。此前 draft——2026-09-10 通道 4 立票（task-9880bbc9），只写票面未动代码；取证锚 `3f485e97`
Blocked by: 无（(c) 范围不等 08；08 落地后「形成」那条路另补）

## 缺口（取证于 `3f485e97`）

- 提供方已发：`internal/parcelpricing/adapters/postgres/evaluation_handoff.go` 的 `evaluationEventType = "parcel-pricing.evaluation.recorded"`，指针载荷 `{tenantId, evaluationId}`，`ports.EvaluationHandoffIntent` 注释写「费用采用在 settlement-accounting」。
- SA 侧编排已落：`internal/settlementaccounting/application/form_supplier_expected_cost.go` 的 `FormSupplierExpectedCostHandler.Handle`（mech/06 SA-c，`1ea2597`），读口 `ports.BuyEvaluationView.LoadBuyEvaluation`，登记面 `ports.ExpectedCostRegistry`。
- 中间那层没有：`git ls-files internal/settlementaccounting/adapters` 无 `inbox`；`cmd/parcel-dispatch/assemble.go` 路由表无 `parcel-pricing.evaluation.recorded`（`git grep -n 'evaluation.recorded' -- cmd/` 零）；`git grep -E 'SupplierExpectedCost' -- cmd/` 只命中 `unwired_orchestration.go` 读面桩。
- 消费侧适配器**已在**：`internal/settlementaccounting/adapters/parcelpricing/buy_evaluation.go` 的 `BuyEvaluationAdapter` 实现 `saports.BuyEvaluationView`（pricing-amount-precision/02 消费侧那一项，ADR-0107 决定五「单位换写不是算术」）。所以从信封到编排之间缺的**只有** inbox 消费者与 dispatch 路由行。

## 语言从哪里来

- SA `CONTEXT.md`：「结算输入已接收：固定关务交接、税费、付款核对、外部资金事实、付款方、合同责任和结算依据的采用版本；接收不表示代垫或回收已经成立。」
- UC-SA-002 步 5 BUY 方向（`form_supplier_expected_cost.go` 文件头：「把一份已完成的 BUY `PricingEvaluation` 与 TF 的运输收费发生项、费用项目和供应商协议一起采用为供应商预期成本的首版」）。
- mech/06「SA-c：缝的形状」原句：「触发用提供方已发的 `parcel-pricing.evaluation.recorded`……内容按引用查」「SA 侧的 inbox 消费者是接线的下一层，不在本票：那封信封只带评价引用，而形成预期成本还要发生项（TF）、费用项目与供应商协议（PC/TF）——它们只在 SA 请求评价那一步（UC-SA-002 步 2）才聚在一处，而那条编排今天同样不存在。」

## 做法

1. `internal/settlementaccounting/adapters/inbox/`（新目录）：`BuyEvaluationRecordedConsumer`，形照 PS 的 `psinbox.*Consumer`（按事件类型取指针载荷、租户在信封上、幂等由 inbox 账本守）。
2. 消费者按 `evaluationId` 经 `BuyEvaluationView` 取评价，方向 / 目的不是 BUY·SUPPLIER_COST 的信封**不处理不报错**（不是本消费者的信封）。
3. 命令里的发生项 / 费用项目 / 供应商协议引用从哪来——见「要裁的」第 1 条；裁前消费者只能对「命令齐不齐」答**未决并指名等谁**（`ExpectedCostUndecided`），不造引用。
4. `cmd/parcel-dispatch/assemble.go` 路由表加一行（共享接线文件，动前占号）。
5. 真库装配用例：入队一封 → 消费一次 → `ExpectedCostRegistry` 一版或未决一格；重投不翻倍。

## 红线

- 金额、币种、换算步骤、规则版本整组出自评价，消费者不复制、不取整、不补默认（ADR-0107：消费方不得在评价之外取整）。
- 不为「命令缺三件引用」发明来源：缺就未决。
- 不改 `form_supplier_expected_cost.go` 的结果代数；不改 PP 的信封形。

## 完成判据

1. `git grep -w NewFormSupplierExpectedCostHandler -- cmd/` 有非测试调用点（dispatch 装配）。
2. 应用层：BUY 信封 → 未决并指名「等评价请求记录（票 08）」一条路有用例；「形成」那条路等 08 落地后由 08 或其后继票补，本票不做；非 BUY 信封不处理。
3. 真库：`cmd/parcel-dispatch` 装配用例一正一反（含 DSN PASS，无 DSN SKIP）。
4. 基线不加宽；机制清点 tip 重生成（消费缝新增 SA→PP 一组）。

## 地盘

`internal/settlementaccounting/adapters/inbox/`（新）、`cmd/parcel-dispatch/assemble.go` 一行 + `assemble_test.go`。不动 `internal/parcelpricing/**`，不动 `adapters/parcelpricing/buy_evaluation.go`。

## 要裁的

1. **三件引用从哪来**：发生项（TF）、费用项目、供应商协议引用不在信封里。选项 (a) 先立 UC-SA-002 步 2「结算提交主要范围、计算目的和合格来源引用」那条请求评价的编排，评价请求本身记下三件、消费者按评价引用回查；(b) 消费者从 TF 收费发生项登记册按评价的输入反查；(c) 本票只做「信封到未决」，三件等 (a) 另票。归 SA owner。

### 裁决

- **1 → (c) + 立 [08](08-sa-evaluation-request-orchestration-records-source-references.md)**（通道 1 推送方裁、通道 5 写入，2026-09-10 17:0x；task-b941ce87 分类 B、task-9a2ff746 落笔）。本票只做「信封到未决」：消费者收 BUY 信封、按评价引用取评价、对「命令齐不齐」答 `ExpectedCostUndecided` 并指名等评价请求记录；三件引用由 08「UC-SA-002 步 2 请求评价编排（评价请求记下三件引用）」提供，08 是实现 UC-SA-002 步 2 已写明的「结算提交主要范围、计算目的和合格来源引用」。**(b) 否**：SA→TF 新跨上下文读口要动 CONTEXT-MAP，且按评价输入反查登记册是推导不是引用（SA CONTEXT「结算输入已接收……的采用版本」——采用的是引用，不是反查出来的匹配）。本票 Status 转 ready-for-agent（(c) 范围），完成判据 2 随改；08 立票时若冒出 UC 空白列进 08 的「要裁的」。

## 参照

[mech/06](../../mechanism-executor-triage/issues/06-sa-four-executors-behind-existing-uc-steps.md)「SA-c：缝的形状」「不在本票」；[remaining-work-dd5ed934.md](../../unresolved-review-20260904/remaining-work-dd5ed934.md) 五-7；ADR-0107；ADR-0029（按恢复动作分格）；PS `adapters/inbox/operator_registration_completed_consumer.go`（消费者形状先例）。

## Comments

- 2026-09-10 · 通道 4：立票。未动代码。
