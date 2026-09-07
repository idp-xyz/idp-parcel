# SA `LoadControlPolicy` 的`要求`格仍从结算政策取方式，不读策略正文的控制项——ADR-0115 Decision 五预告的 SA 侧后继

Category: enhancement
Status: resolved——五问由通道 1 按 owner 授权自决口径裁并落 [ADR-0122](../../../docs/adr/0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md)（2026-09-07），实施见文末「完成记录」；PS 域逐项结果的表达拆到 [03](./03-parcel-shipment-expresses-per-item-control-results.md)（draft）
Blocked by: 无（前置 pc-gaps/07 已进 main `eb09c1ed`：`migrations/party_commercial/0024` 与 `ports.PreAcceptanceFinancialControlPolicyContentView` 都在）

## 从哪里来

[ADR-0115](../../../docs/adr/0115-pre-acceptance-financial-control-policy-content-is-a-row-per-control-and-no-control-stays-with-the-contract.md)
Decision 五原句：「SA 的 `LoadControlPolicy` **一字不改**：它今天从 `0007` 答『要不要』、从结算政策答『方式』，那两格仍然对；
改为经本点读口读『要执行哪些控制项』属 SA 地盘，另立票。**那一票落地前，接受前控制链在生产上的行为一字不变**。」
本票就是那一票。它与 [01](./01-sa-preacceptance-control-policy-view-has-no-production-adapter.md) 同一条缝：01 让 SA 凭回指
读到合同的「要不要」，本票让它读到策略的「怎么做」。

## 今天的读路径（实读 main `08f54867`，锚只作此刻取证）

`internal/settlementaccounting/adapters/partycommercial/pre_acceptance_control_policy.go`：`LoadControlPolicy` 分三段——
回指换闭包 → 闭包取已采用客户合同版本 → 按合同读 `0007` 声明（`PreAcceptanceControlDeclarationView`）。`policyFrom` 在
`要求`那一格经 `adoptedSettlementPolicy` 取闭包里已采用的结算政策，把它的 `SettlementMethod` 翻成 SA 的方式、版本身份翻成
`AdoptedPolicyReference`，交 `sadomain.NewRequiredControlPolicy(method, adoptedPolicy)`。**它从不读任何策略版本正文**——
写它时没有正文可读，代码注释自己写着「由本适配器从声明反推方式，就会从账期倒推出无需信用校验，那是 `pn-02-w03` 明禁的
那条推导」，于是方式取自结算政策；但结算方式与控制方式是两件事，那句禁令管的正是把前者当后者。

SA 侧结果形状 `sadomain.PreAcceptanceControlPolicy`：`Requirement`（要求 / 不要求）+ `Basis`（不适用依据）+ `Method`（一种
结算方式）+ `AdoptedPolicy`（一份采用政策引用）。**一种方式、一份政策**——装不下 ADR-0115 允许的组合：同一版策略多行
控制项，各带费用范围 / 判断顺序 / 失败处置 / 责任，父行一格共同通过条件。

于是「控制怎么做」今天仍是从结算方式派生的：预付 → 冻结、账期 → 信用暴露。对单项控制碰巧成立，对一份账期合同同时要求
预付保证金冻结、或一份预付合同同时要求信用校验，说不出话。PC 侧正文表落地后，这条派生第一次有了可替换的来源。

## 提供方现在给了什么（pc-gaps/07 落的，以 main 上重放后的代码为准）

- `ports.PreAcceptanceFinancialControlPolicyContentView.LoadPreAcceptanceFinancialControlPolicy(ctx, tenant, version)`
  → `(domain.PreAcceptanceFinancialControlPolicy, found, error)`：`found=false` = 正文未登记；有父无子走 error，不折成未登记。
- `domain.PreAcceptanceFinancialControlPolicy`：`JointPassCondition()`（首发一值 `ALL_CONTROLS_PASS`）、`Items()` 按判断顺序、
  `ItemsFor(scope ChargeScopeReference)`；每项 `Kind()`（`PREPAID_FREEZE` / `CREDIT_CHECK`）、`Scope()`、`EvaluationOrder()`、
  `FailureDisposition()`（`REJECT` / `AUTHORIZED_DISPOSITION`）、`Responsibility()`。
- 版本壳的选中仍走既有闭包：`ResolveCommercialClosure` 对 `PreAcceptanceFinancialControlPolicyObject` 一视同仁，
  `closure.AdoptedFor(PreAcceptanceFinancialControlPolicyObject)` 取得的版本就是点读的键。
- 合同 → 策略的绑定按费用范围在 `0012` 的 `customer_contract_control_binding`（`policy_id` 与 `inapplicability_basis` 恰一非空）。

## 开工前要答的（owner 或被授权者裁，本票不预设任何一格）

1. **SA 结果形状怎么长**：`PreAcceptanceControlPolicy` 从「一种方式 + 一份政策」长成「控制项集合 + 共同通过条件」，还是
   并列一个新类型让既有编排继续读旧形状？SA CONTEXT 说「只执行适用合同策略明确要求的控制项……不得把多个结果汇总成
   委托接受或拒绝决定」——多项控制在 SA 里各自成结果，编排怎么按 `JointPassCondition` 报「共同通过」而不越权成接受决定。
2. **信用校验按哪一版信用政策**：ADR-0115 Decision 四明写控制项不带信用政策引用，「那一版由消费侧按闭包解析或按版本壳的
   指名引用取得，两条路选哪条是 SA 消费票的问题」。闭包里今天有没有已采用的 `CREDIT_POLICY` 版本；没有时答未配置还是 error。
3. **费用范围从哪来**：`ItemsFor(scope)` 要一个 `ChargeScopeReference`；SA 的键是资金维（法人 / 账户 / 币种）+ 回指，委托的
   费用范围在 SA 命令上今天在不在、要不要随回指一起回显（与 01 加回指那一步同形，PS→SA 适配器跟随）。
4. **`0007` 与正文的关系**：合同版本级 `0007` 说`要求`而策略正文 `found=false` 时——是停 `CONTROL_POLICY_NOT_CONFIGURED`
   （今天的未登记格，恢复动作是租户补正文），还是沿用今天从结算政策推方式的路作过渡？后者等于把 `pn-02-w03` 禁的推导
   再留一段时间，要 owner 明说，不能由实现者默认。
5. **失败处置与责任怎么过缝**：`REJECT` / `AUTHORIZED_DISPOSITION` 只答委托去向、拒绝决定归 PS；SA 结果里带不带它，还是
   由 PS 自己经 PC 读？责任引用是开放引用，SA 结果要不要携带。

## 范围与红线

- SA 地盘：`internal/settlementaccounting/**`（ports / domain / application / adapters/partycommercial）与 SA 侧真库用例；
  PS→SA 适配器若要多回显一格费用范围，属跟随不属占号。
- 不动 PC 任何一格；不动 `0007` / `0012`；不给未登记正文的合同任何默认控制项；集外取值（新的控制种类 / 组合子 / 处置）
  报错不吸收（ADR-0025 全函数）。
- 裁决若改 SA 结果形状或缝的语义，随实现落一份 ADR（引本票与 ADR-0115），编号取当时下一号。

## 完成判据

`要求`格的控制项来自策略正文而不是结算政策的派生；正文未登记停 `CONTROL_POLICY_NOT_CONFIGURED`（与调不通格分开，
ADR-0054 / 0029 维持）；真库一正一反（一版多行正文可读回、缺正文 found=false、坏回指 error）；全仓含 DSN 绿并注明。

## 裁决（通道 1，2026-09-07，owner 授权自决；理由与备选见 ADR-0122）

1. **SA 结果形状**：`PreAcceptanceControlPolicy` 的`要求`形状从「一种方式 + 一份政策」长成「控制项集合（种类 × 判断顺序）+ 共同通过
   条件 + 采用的控制策略版本」，结算方式与结算政策引用**保留**在答复与结果上（CONTEXT 要冻结与暴露保存它们）但不再选路。编排按判断
   顺序逐项执行、控制种类决定走哪本账，**各项各自成结果**；共同通过条件原样交回，SA 不据它汇总——CONTEXT 明写那一步归 PS。「全部通过」
   之下第一处`业务限制`即停后续项、已执行项不回滚（账本对请求身份幂等）。
2. **信用政策版本**：本票不取。`CREDIT_CHECK` 照旧对 `CreditStandingView` 执行，哪一版信用政策决定额度是那一口提供方（SA→PC 信用缝，
   未接）的事；不在控制项上带一个没人读的引用。ADR-0115 留的两条路到接那条缝时再选。
3. **费用范围**：取闭包里已采用结算政策的 `Applicability().ChargeScope()`，不在 SA 命令上加格、不改 PS→SA 缝——资金作用域本就派生自
   那份结算政策，两处同源；正文里挂在别的范围上的项不进答复，本范围一项都没有时答`未配置`。
4. **`0007` 说要求而正文 found=false**：停 `CONTROL_POLICY_NOT_CONFIGURED`（恢复动作是租户补正文），**不留**「沿用结算政策推方式」的
   过渡——那正是 pn-02-w03 禁的推导。闭包未采用控制策略同格。
5. **失败处置与责任**：不随 SA 答复走。SA 结果只带执行控制所需的；委托去向归 PS，PS 要用时经自己的 PC 缝读。多项结果怎么合起来看，
   由 PS 在自己的消费适配器按共同通过条件折成今天的一个 `FinancialControlResult`；PS 域逐项结果的表达拆到票 03。

**越权风险点（供用户复核，不认可走 supersede）**：① 决定二「第一处限制即停后续项」是执行顺序规则，本会话判它不构成「汇总成接受/拒绝
决定」——若 owner 认为组合策略下每一项都必须执行到底（例如为了给客户完整的限制清单），应改；② 决定四让 PS 消费适配器在 PS 域建模之前
先折叠，折叠丢掉了「哪几项执行了」——票 03 是补它的地方，若 owner 要先建模再放行，应把本票的 PS 适配器改动回退到只支持单项。

## 完成记录（分支 `mcp1-sa02`，基 `80641ddc`；进 main 的 SHA 由推送方广播后补记）

| 笔 | 内容 |
|---|---|
| `f850108c` | SA 领域：`ControlKind` / `ControlItem` / `JointPassCondition` / `ControlPolicyReference`；`NewRequiredControlPolicy` 收控制项并核至少一项、种类唯一、顺序唯一，按判断顺序交回 |
| `75394a6a` | SA 编排：逐项执行、种类选路、一次请求一个控制时刻、「全部通过」下第一处限制即停、停下不回滚；结果加 `ExecutedControls` / `JointPassCondition` / `ControlPolicy`；释放两本账各认领 |
| `c5d3a711` | SA→PC 适配器改读正文（三半装配）、`parcel-dispatch` 接 `NewPreAcceptanceFinancialControlPolicyContents`、PS→SA 适配器按共同通过条件折叠；真库用例五向 |
| 本笔 | ADR-0122 + README 行、票 03 draft、本票收口 |

**验收对照**：`要求`格的控制项来自策略正文——适配器 `requiredPolicyFrom` 只经 `PreAcceptanceFinancialControlPolicyContentView` 取项，`policyFrom`
从结算方式推方式的那段已删；正文未登记停 `CONTROL_POLICY_NOT_CONFIGURED`——适配器答 found=false，编排既有分格不变（与调不通格分开，ADR-0054 / 0029
维持）；真库一正一反——一版多行正文可读回并按序（`TestATermsClosureCarriesEveryControlItemThePolicyContentRequires`）、缺正文 found=false 与闭包未采用
两向（`TestARequiredDeclarationWithoutPolicyContentIsNotConfiguredNotDerived`）、坏回指 error（既有 `TestABadResolutionReferenceIsAnErrorNotAnUnconfiguredGrade`
仍绿）；有父无子走 error 由提供方那口承重，本票不重证。

**验证强度**：见完工报（干净检出含 DSN 全仓 `-p 1 -count=1`、探针 `internal/settlementaccounting/adapters/partycommercial` 无 DSN SKIP / 含 DSN PASS、
清点重生成单独成笔）。

## Comments

- 2026-09-07 · MCP-3：立票。**只写票面，未动 SA 代码。** 能力边界：读过 ADR-0115 全文、`pre_acceptance_control_policy.go`
  全文（main `08f54867`）、SA `ports.PreAcceptanceControlPolicyView` 注释与签名、`sadomain.PreAcceptanceControlPolicy` 的构造函数
  与访问器、本目录票 01 全文；**没读** SA 编排 `apply_pre_acceptance_control.go` 全文与 `UC-SA-001`——上面第 1、5 问的措辞据
  SA CONTEXT 那两句与 ADR-0054 写，开工者以代码为准。
