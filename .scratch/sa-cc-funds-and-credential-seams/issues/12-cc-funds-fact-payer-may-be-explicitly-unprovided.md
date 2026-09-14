# CC 入向登记要付款人非空，而 SA 采用的事实可以没有付款人：来源未提供且真实程序不要求时，CC 应「明确记录」而不是拒收

Category: enhancement
Status: ready-for-agent——2026-09-14 10:2x 通道 1 按用户 10:1x「授权代裁」（CC owner 口径）裁「要裁的」1：「真实程序要求付款人」是**登记的规则一格**（形照 ADR-0137 决定三规则型目录行），不是 `PAR-CUS-*` 待提供参数；机制半边现在做、实例半边留空拒默认值；做法 3 的停格一并裁定，全文见文末「裁决」。取证锚仍是票面的 `f96169d2`，作者开工先在 main 重量。此前 draft——2026-09-10 22:0x 通道 5 立票（sa-cc/03 实施中按通道 1 裁决「CC 放宽另立 draft」，task-764b20a1）。只写票面未动代码；取证锚 `f96169d2`
Blocked by: 无（03 已落地；本票要裁的一条归 CC owner）

## 缺口（取证于 `f96169d2`）

- `internal/customscompliance/application/reconcile_duty_payment.go` 的 `ReceiveFundsFact` 对空 `Payer` 答 `未受理`；迁移 `customs_compliance/0016` 的 `external_funds_fact.payer_ref text NOT NULL` + 非空 CHECK。
- SA 侧（sa-cc/03 落地）：`domain.ExternalFundsFact.Payer()` 第二值为 false 即「来源未提供」，采用不拒；`settlement_accounting/0018` `payer_ref` 可空。
- 于是一条来源没给付款人的事实，经 `settlement-accounting.external-funds-fact.adopted` 信封到 CC 消费者，`ReceiveOnAdoptedFundsFactAdapter` 如实交空、编排答 `未受理`、消费门入账不重投（`receive_on_adopted_funds_fact.go` 头注）——事实进不了税费付款核对的入向登记册，**这是有意的诚实停点，不是 bug**（03 判断题 ③）。

## 语言从哪里来

- CC `CONTEXT.md`「税费付款核对」词条：「按明确申报范围、法定义务以及**来源提供或真实程序要求的**付款人、金额、币种、业务时间等维度进行的版本化比较判断」——付款人是「来源提供**或**真实程序要求」的维度，两种来处都成立时才必备。
- CC `CONTEXT.md`（sa-cc/03 票面引）：「来源未提供且程序不要求的维度要『明确记录』」。
- mech/07 CC-c 把付款人列为关联核对最少要读的四件之一——与上一句的张力正是本票要裁的。

## 做法（待裁后）

1. `ExternalFundsFactRegistration.Payer` 允许「来源未提供」的显式形（不是空串默认：领域上一格，或 `(string, bool)`），`ReceiveFundsFact` 不再因付款人缺席答 `未受理`。
2. 新迁移（序号重取）放宽 `customs_compliance.external_funds_fact.payer_ref` 为可空 + 拒空白 CHECK；`0016` 不改。
3. `VerifyPayment` 的调用方在关联核对时看得见「付款人未提供」这一格——真实程序要求付款人而来源没给时，核对该停在哪一格（待确认？不适用？）随裁决定。
4. `ReceiveOnAdoptedFundsFactAdapter` 不改：它今天已如实转述缺席。

## 红线

- 不拿 SA 的来源身份或别的维顶替付款人；不写任何真实银行 / 支付字段。
- 「程序要不要求付款人」若属实例半边（`PAR-CUS-*`），一行都不预填。

## 完成判据

1. 应用层：来源未提供付款人的事实 → `已接收`，登记里付款人显式「未提供」；同引用重放 → `已存在`；程序要求而未提供时核对的停格如裁决。
2. 真库：放宽后的往返；`0016` 一字未动。
3. sa-cc/03 的越权风险点 (b) 由 CC owner 在本票一并复核。

## 地盘

`internal/customscompliance/{ports,application,adapters/postgres}`、`migrations/customs_compliance/`（新序号）。SA 侧不动。

## 要裁的

1. **（已裁，见「裁决」）**「真实程序要求付款人」是实例半边还是登记的规则：是每个真实程序登记进门禁目录 / 核对规则的一格（形照 ADR-0137 决定三的规则型目录行），还是 `PAR-CUS-*` 待提供参数——归 CC owner，一句。裁前 `ReceiveFundsFact` 保持必填。

## 裁决（2026-09-14 10:2x，通道 1 推送方按用户「授权代裁」以 CC owner 口径裁）

1. **是登记的规则一格，不是参数。** 参数登记册登的是取值（税率、口岸代码那一类），「某真实程序核对时要不要付款人」是核对规则本体的一维：按（租户、真实程序 / 申报路径）登记「付款人必备否」，形照 ADR-0137 决定三的规则型目录行——登记面 + 读口 + 「未登记 → 未配置」诚实格，全是机制半边，现在就做；哪个程序要、哪个不要是实例半边，一行不预填、不给默认值（AGENTS.md 红线）。已有的门禁 / 核对规则目录若装得下这一格就加一格，装不下另立一册——作者按 ADR-0137 落地的表形定，票面完成记录写为何。
2. **做法 3 的停格**：程序**要求**付款人而来源未提供 → 核对停在**未决（原因：要求而未提供）**并点名缺付款人（10:5x 改口：原写「待确认」，作者重量后指出 CC CONTEXT 原词是「规则要求但缺失时保持未决」，且既有 `DutyReconciliationUndecided` + reason 的形让 CLI / HTTP / inbox 零改——以 CONTEXT 原词为准；不是「不适用」——不适用是「这条维度与本程序无关」，与「该有而没有」是两格）；程序**不要求** → 付款人显式记「未提供」，核对照常进行；程序**未登记要不要** → **未决（原因：规则未配置）**，核对不进行，点名缺的是规则不是事实（同 10:5x 改口：两格都落既有 `DutyReconciliationUndecided`，靠 reason 分恢复动作，不加新 outcome）。三格的恢复动作各不相同（补事实 / 无 / 补规则），按 ADR-0029 分格。
3. **做法 1 的形**：「来源未提供」取领域上一格（显式值），不取 `(string, bool)`——它要进登记册、读回、进核对判断三处，一格比一对更难写错；SA 侧 `Payer()` 的 `(value, bool)` 是 SA 的形，CC 消费侧适配器负责译，不要求两边同形。
4. **不改的**：`0016` 不改（新迁移放宽）；SA 侧不动；`ReceiveOnAdoptedFundsFactAdapter` 不改；不拿任何别的维顶替付款人（票面红线）。**完成判据 3**（复核 sa-cc/03 越权风险点 (b)）照旧归本票。

## 参照

[03](03-cc-inbox-consumer-receives-external-funds-fact.md)（裁决与越权风险点 (b)）；[02](02-sa-external-funds-fact-adoption-hands-off-an-envelope.md)；ADR-0137 决定四。

## Comments

- 2026-09-10 · 通道 5：立票（按通道 1 于 sa-cc/03 的裁决「CC 放宽（B）另立 sa-cc 新票 draft」）。未动代码。
