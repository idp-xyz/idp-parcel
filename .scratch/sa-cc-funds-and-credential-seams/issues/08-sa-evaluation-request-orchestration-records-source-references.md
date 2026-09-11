# UC-SA-002 步 2「请求评价」的编排今天不存在：没有任何东西记下一次 BUY 评价请求携带的发生项 / 费用项目 / 供应商协议三件引用，`FormSupplierExpectedCostHandler` 永远凑不齐命令

Category: enhancement
Status: in-progress——2026-09-11 14:0x 通道 6 按通道 1 派单 task-b5cadbb5 认领，分支 `mcp6-sacc08` 基 `51ca1270`，做「做法」1 / 2 / 4 与判据 1 / 2 / 3 / 5（判据 4 等 11）。此前 ready-for-agent——2026-09-10 17:5x 通道 5 按通道 1 派单 task-a93cb825 写入裁决：SA→PP 走信封不同步调用、评价请求身份用铸造 ID + 自然键唯一约束守幂等（见「要裁的」下「裁决」），本票再无待裁问题。此前 draft——2026-09-10 17:3x 通道 5 立票（task-9a2ff746；票 [01](01-buy-evaluation-to-sa-inbox-consumer.md)「要裁的」1 裁 (c) 时点名另立），只写票面未动代码；取证锚 `66cad4c4`
Blocked by: [11](11-pp-inbox-consumer-receives-evaluation-request-envelope.md)（PP 今天没有 inbox 消费者先例，`internal/parcelpricing/adapters` 只有 `http` / `postgres` / `sourcefeed`；本票的登记册与发信封半边可先落，但完成判据「请求 → 评价引用」要 11 在场）。[01](01-buy-evaluation-to-sa-inbox-consumer.md) 的 (c) 范围不等本票

## 缺口（取证于 `66cad4c4`）

- `Get-ChildItem internal/settlementaccounting/application` 里与评价相关的只有 `form_supplier_expected_cost.go`（mech/06 SA-c）；没有任何文件实现 UC-SA-002 步 2「结算提交主要范围、计算目的和合格来源引用」——SA 今天不会向 `parcel-pricing` 请求评价，也不记请求。
- `ports.EvaluationHandoffIntent` 的信封 `parcel-pricing.evaluation.recorded` 只带 `{tenantId, evaluationId}`（票 01 缺口节）；SA 消费者按评价引用取得评价本身，但形成供应商预期成本还要 TF 运输收费发生项、费用项目与供应商协议三件引用（UC-SA-002「供应商预期成本已形成」行：「发生项、协议、评价、费用项目、主要范围……」），它们只在请求评价那一步聚在一处。
- 票 01「要裁的」1 三个选项：(a) 先立本编排、评价请求记下三件；(b) 消费者反查 TF 登记册（否——推导不是引用）；(c) 01 只做信封到未决。裁 (c) + 立本票。

## 语言从哪里来

- SA `CONTEXT.md`：「结算输入已接收：固定关务交接、税费、付款核对、外部资金事实、付款方、合同责任和结算依据的采用版本；接收不表示代垫或回收已经成立。」——采用的是**引用的版本**，不是反查出来的匹配。
- UC-SA-002 步 2：「`settlement-accounting` / `parcel-pricing`：结算提交主要范围、计算目的和合格来源引用；`parcel-pricing` 采用测量、运输收费发生项、其他履约、面单及已解析商业依据形成不可变计价输入快照 → 计价输入版本或待判断」。
- mech/06「SA-c：缝的形状」：「形成预期成本还要发生项（TF）、费用项目与供应商协议（PC/TF）——它们只在 SA 请求评价那一步（UC-SA-002 步 2）才聚在一处，而那条编排今天同样不存在。」

## 做法

1. SA 新登记册「评价请求」：身份是**铸造的 `EvaluationRequestID`**（裁决 2）；一行 = ID、租户、结算提交主要范围、计算目的（BUY·SUPPLIER_COST 起步）、合格来源引用三件（TF 运输收费发生项引用、费用项目、供应商协议版本引用）、请求时刻、请求方。**自然键**（租户 + 主要范围 + 计算目的 + 合格来源引用集合的稳定摘要）上唯一约束：同自然键重复提交答`已存在`并交回原 ID（照登记册三态先例，不答失败）；成分不同就是另一份请求、另一个 ID。三件引用是**引用**——本票不读 TF / PC 的内容，只登记调用方交进来的引用（内容由 `parcel-pricing` 在步 2 后半采用）。
2. 请求评价的编排 `RequestBuyEvaluation`：登记评价请求 → **同事务经 Outbox 发信封**（裁决 1）`settlement-accounting.evaluation-request.submitted`，载荷只带引用 `{tenantId, evaluationRequestId}`，分区主体「租户 / 评价请求」（ID 管幂等、分区键管顺序，ADR-0069 决定二）；`ports.EvaluationRequestHandoff` + `adapters/postgres/evaluation_request_handoff.go`，形照 SA 既有 handoff 一族（`supplier_bill_handoff.go`）。`已存在` 不重发（重放靠 `EnqueueOnce`）；交接失败按仓内既有形（登记已落、意图未入队 → 技术未形成 + 续办引用）。PP 侧消费者在 [11](11-pp-inbox-consumer-receives-evaluation-request-envelope.md)。
3. PP 形成评价后发既有的 `parcel-pricing.evaluation.recorded`，票 01 的消费者按 `evaluationId` 回查——评价与评价请求的对应由 PP 在评价上回指 `evaluationRequestId`（11 的事）、SA 侧按评价读口取回指再查请求行取三件引用；命令齐 → `FormSupplierExpectedCostHandler` 形成首版；请求行不在 → 仍答未决并指名（01 (c) 那一格不变）。
4. 真库：请求登记往返 + 自然键重放`已存在` + 信封入队一封且分区键如裁 + `EnqueueOnce` 不翻倍；应用层：请求 → 已登记并已发信封 / `已存在` 两条路；01 那条「形成」路的用例在 11 落地后随本票或 01 补。

## 红线

- 三件引用由调用方（结算作业或上游编排）交进来，本票不替它从 TF 登记册推——推导出来的匹配不是「合格来源引用」。
- 金额、币种、换算全在评价里（ADR-0107 / ADR-0013），本票不碰。
- 不改 PP 的评价形与信封形；不改 `form_supplier_expected_cost.go` 的结果代数。
- 真实供应商协议、真实费用项目、真实发生条件属实例半边 `PAR-SET-03`，夹具全是合成串。

## 完成判据

1. SA `application` 有 `RequestBuyEvaluation`（或等价名）编排且 `cmd/` 有非测试装配点；评价请求登记册真库往返，自然键唯一约束在库上。
2. 应用层：请求 → 已登记 + 信封一封；同自然键重复 → `已存在` 交回原 ID、不重发；交接失败 → 技术未形成 + 续办引用。
3. 真库：`evaluation_request_handoff_test.go` 入队一封、分区键「租户 / 评价请求」、`EnqueueOnce` 同键不翻倍；主体名按 ADR-0074 决定五进中心登记表。
4. 票 01 的消费者对「请求行在场」的信封形成首版——等 11 落地，用例落 01 或本票，票面写明。
5. 机制清点 tip 重生成（outbox handoff +1）；CONTEXT-MAP 加 SA→PP 请求边随同笔。

## 地盘

`internal/settlementaccounting/application/`（新编排文件）、`internal/settlementaccounting/ports/`（登记册写读 + `EvaluationRequestHandoff`）、`internal/settlementaccounting/adapters/postgres/`（新登记册 + `evaluation_request_handoff.go`）、`migrations/settlement_accounting/`（新序号）、`cmd/` 装配点（先核在哪）、`docs/domain/CONTEXT-MAP.md` 一条边。不动 `internal/parcelpricing/**`（PP 侧归 11）。

## 要裁的

1. **SA 向 PP 提交计价输入走哪条缝**：进程内消费侧适配器同步调 PP 的应用入口（形照 SA 读 PP 的 `BuyEvaluationAdapter`，但这是写不是读），还是发一封「计价输入已提交」信封由 PP 的 inbox 消费。UC-SA-002 步 2 一行里两个上下文并列写，没说同步还是异步；ADR-0049 只裁了进程内直投的投递方式。跨上下文写向缝的形状，归 SA owner（连 PP owner 口径）。
2. **评价请求登记册的身份**：按（租户、主要范围、计算目的、三件引用）合成键，还是铸造请求 ID 由信封 / 评价回指。ADR-0069 决定三「案件维统一用铸造 ID」是同形先例；但合成键让重放判定不依赖 ID 生成。归 SA owner。

### 裁决

（通道 1 推送方代裁 2026-09-10 17:5x，通道 5 写入；task-a93cb825。用户授权同前。）

- **1 → 信封，不同步调用。** 理由：① PP→SA 那条缝（票 01 `parcel-pricing.evaluation.recorded` → SA inbox）已是信封，反向同形——一条缝两个方向一种机制；② 仓内跨上下文写向交接的纪律是「意图与登记同事务入队」（`FinalOutcomeHandoff` / `ExternalTrackingFactHandoff` / ADR-0134 那一族），同步调用会把 PP 的可用性压进 SA 的写路径；③ 评价请求登记册就是那份同事务的登记。**越权风险点（PP owner）**：PP 的入口是否愿意以消费者形式接——通道 5 核过 `66cad4c4`：`internal/parcelpricing/adapters` 只有 `http` / `postgres` / `sourcefeed`，**PP 今天没有 inbox 消费者先例**（仓内先例在 NR / PS / VE 的 `adapters/inbox`），故 PP 侧消费者另立 [11](11-pp-inbox-consumer-receives-evaluation-request-envelope.md) 并作本票 Blocked by。
- **2 → 铸造 ID，自然键唯一约束守幂等。** 理由：仓内口径「ID 管幂等、分区键管顺序」（ADR-0069 决定二）；评价请求会被后续信封与票 01 的消费者引用，引一个铸造的稳定 ID 比引一串合成键稳（合成键成分一变引用就断）；自然键（租户 + 结算提交主要范围 + 计算目的 + 合格来源引用集合的稳定摘要）上唯一约束，重复提交答`已存在`不答失败（照登记册三态先例）。**越权风险点（SA owner）**：自然键的成分——尤其「合格来源引用集合的稳定摘要」怎么算（排序后摘要，照 PCC-1 方式集合稳定序的先例）。

## 参照

票 [01](01-buy-evaluation-to-sa-inbox-consumer.md)「要裁的」1 与「裁决」；[mech/06](../../mechanism-executor-triage/issues/06-sa-four-executors-behind-existing-uc-steps.md)「SA-c：缝的形状」；UC-SA-002 步 2 与「供应商预期成本已形成」行；ADR-0107、ADR-0013、ADR-0069 决定三；`internal/settlementaccounting/adapters/parcelpricing/buy_evaluation.go`（SA 读 PP 的消费侧适配器先例）。

## Comments

- 2026-09-10 · 通道 5（task-9a2ff746）：立票，未动代码。能力边界：读过票 01 全文、mech/06 SA-c 段、UC-SA-002 步 2 与结果行、SA CONTEXT「结算输入已接收」句；没读 `form_supplier_expected_cost.go` 的命令形与 PP 的计价输入入口——「要裁的」1 两条路的件数按接口名估，开工时以代码为准。
