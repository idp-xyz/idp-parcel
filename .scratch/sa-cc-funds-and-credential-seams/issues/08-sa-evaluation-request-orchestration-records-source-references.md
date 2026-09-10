# UC-SA-002 步 2「请求评价」的编排今天不存在：没有任何东西记下一次 BUY 评价请求携带的发生项 / 费用项目 / 供应商协议三件引用，`FormSupplierExpectedCostHandler` 永远凑不齐命令

Category: enhancement
Status: draft——2026-09-10 17:3x 通道 5 立票（task-9a2ff746；票 [01](01-buy-evaluation-to-sa-inbox-consumer.md)「要裁的」1 裁 (c) 时点名另立），只写票面未动代码；取证锚 `66cad4c4`。要裁的两条，归 SA owner
Blocked by: 无（[01](01-buy-evaluation-to-sa-inbox-consumer.md) 的 (c) 范围不等本票；本票落地后 01 的「形成」那条路由本票或其后继补）

## 缺口（取证于 `66cad4c4`）

- `Get-ChildItem internal/settlementaccounting/application` 里与评价相关的只有 `form_supplier_expected_cost.go`（mech/06 SA-c）；没有任何文件实现 UC-SA-002 步 2「结算提交主要范围、计算目的和合格来源引用」——SA 今天不会向 `parcel-pricing` 请求评价，也不记请求。
- `ports.EvaluationHandoffIntent` 的信封 `parcel-pricing.evaluation.recorded` 只带 `{tenantId, evaluationId}`（票 01 缺口节）；SA 消费者按评价引用取得评价本身，但形成供应商预期成本还要 TF 运输收费发生项、费用项目与供应商协议三件引用（UC-SA-002「供应商预期成本已形成」行：「发生项、协议、评价、费用项目、主要范围……」），它们只在请求评价那一步聚在一处。
- 票 01「要裁的」1 三个选项：(a) 先立本编排、评价请求记下三件；(b) 消费者反查 TF 登记册（否——推导不是引用）；(c) 01 只做信封到未决。裁 (c) + 立本票。

## 语言从哪里来

- SA `CONTEXT.md`：「结算输入已接收：固定关务交接、税费、付款核对、外部资金事实、付款方、合同责任和结算依据的采用版本；接收不表示代垫或回收已经成立。」——采用的是**引用的版本**，不是反查出来的匹配。
- UC-SA-002 步 2：「`settlement-accounting` / `parcel-pricing`：结算提交主要范围、计算目的和合格来源引用；`parcel-pricing` 采用测量、运输收费发生项、其他履约、面单及已解析商业依据形成不可变计价输入快照 → 计价输入版本或待判断」。
- mech/06「SA-c：缝的形状」：「形成预期成本还要发生项（TF）、费用项目与供应商协议（PC/TF）——它们只在 SA 请求评价那一步（UC-SA-002 步 2）才聚在一处，而那条编排今天同样不存在。」

## 做法

1. SA 新登记册「评价请求」：一行 = 租户、主要范围、计算目的（BUY·SUPPLIER_COST 起步）、合格来源引用三件（TF 运输收费发生项引用、费用项目、供应商协议版本引用）、请求时刻、请求方；同键同内容重放`已存在`，换内容新版本。三件引用是**引用**——本票不读 TF / PC 的内容，只登记调用方交进来的引用（内容由 `parcel-pricing` 在步 2 后半采用）。
2. 请求评价的编排 `RequestBuyEvaluation`：登记评价请求 → 向 `parcel-pricing` 提交计价输入（形状见「要裁的」1）→ 收回评价引用或待判断；评价引用与评价请求的对应关系记在请求行上（评价请求 ↔ `evaluationId`）。
3. 票 01 的消费者收到 `parcel-pricing.evaluation.recorded` 后按 `evaluationId` 回查评价请求行取三件引用，命令齐 → `FormSupplierExpectedCostHandler` 形成首版；请求行不在 → 仍答未决并指名（01 (c) 那一格不变）。
4. 真库：请求登记往返 + 重放 / 换内容；应用层：请求 → 评价引用 / 待判断两条路；01 那条「形成」路的用例随本票补进 01 或本票。

## 红线

- 三件引用由调用方（结算作业或上游编排）交进来，本票不替它从 TF 登记册推——推导出来的匹配不是「合格来源引用」。
- 金额、币种、换算全在评价里（ADR-0107 / ADR-0013），本票不碰。
- 不改 PP 的评价形与信封形；不改 `form_supplier_expected_cost.go` 的结果代数。
- 真实供应商协议、真实费用项目、真实发生条件属实例半边 `PAR-SET-03`，夹具全是合成串。

## 完成判据

1. SA `application` 有 `RequestBuyEvaluation`（或等价名）编排且 `cmd/` 有非测试装配点；评价请求登记册真库往返。
2. 应用层：请求 → 评价引用 / 待判断；重放`已存在`；换内容新版本。
3. 票 01 的消费者对「请求行在场」的信封形成首版（用例落 01 或本票，票面写明）。
4. 机制清点 tip 重生成；CONTEXT-MAP 若加 SA→PP 请求边随同笔。

## 地盘

`internal/settlementaccounting/application/`（新编排文件）、`internal/settlementaccounting/ports/`、`internal/settlementaccounting/adapters/postgres/`（新登记册）、`migrations/settlement_accounting/`（新序号）、`cmd/` 装配点（先核在哪）。不动 `internal/parcelpricing/**`。

## 要裁的

1. **SA 向 PP 提交计价输入走哪条缝**：进程内消费侧适配器同步调 PP 的应用入口（形照 SA 读 PP 的 `BuyEvaluationAdapter`，但这是写不是读），还是发一封「计价输入已提交」信封由 PP 的 inbox 消费。UC-SA-002 步 2 一行里两个上下文并列写，没说同步还是异步；ADR-0049 只裁了进程内直投的投递方式。跨上下文写向缝的形状，归 SA owner（连 PP owner 口径）。
2. **评价请求登记册的身份**：按（租户、主要范围、计算目的、三件引用）合成键，还是铸造请求 ID 由信封 / 评价回指。ADR-0069 决定三「案件维统一用铸造 ID」是同形先例；但合成键让重放判定不依赖 ID 生成。归 SA owner。

## 参照

票 [01](01-buy-evaluation-to-sa-inbox-consumer.md)「要裁的」1 与「裁决」；[mech/06](../../mechanism-executor-triage/issues/06-sa-four-executors-behind-existing-uc-steps.md)「SA-c：缝的形状」；UC-SA-002 步 2 与「供应商预期成本已形成」行；ADR-0107、ADR-0013、ADR-0069 决定三；`internal/settlementaccounting/adapters/parcelpricing/buy_evaluation.go`（SA 读 PP 的消费侧适配器先例）。

## Comments

- 2026-09-10 · 通道 5（task-9a2ff746）：立票，未动代码。能力边界：读过票 01 全文、mech/06 SA-c 段、UC-SA-002 步 2 与结果行、SA CONTEXT「结算输入已接收」句；没读 `form_supplier_expected_cost.go` 的命令形与 PP 的计价输入入口——「要裁的」1 两条路的件数按接口名估，开工时以代码为准。
