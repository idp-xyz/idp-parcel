# `parcel-shipment` 对不通过的接受前财务控制一律拒绝——策略正文里的失败处置（`REJECT` / `AUTHORIZED_DISPOSITION`）与责任引用没有消费方，「进入授权处置」那条路在本上下文不存在

Category: enhancement
Status: draft——由票 [03](./03-parcel-shipment-expresses-per-item-control-results.md) 第 2 问拆出（2026-09-07，通道 3，ADR-0125）；PS 地盘，读口形状归 PC（已在）
Blocked by: 无代码阻塞；「授权处置」的领域语义要 owner 先裁（见「要裁的」）

## 缺口

ADR-0115 让策略正文逐项带失败处置（`ControlFailureDisposition`：`REJECT` / `AUTHORIZED_DISPOSITION`）与责任引用
（`ControlResponsibilityReference`），并明写它们「只答委托去向，不拥有拒绝决定」；ADR-0122 决定四让它们不随 SA 答复走，
PS 要用时经自己的商业缝读。UC-PS-001 校验组「接受前财务控制」行写「任一必需控制不通过时按策略拒绝或进入授权处置」。

`parcel-shipment` 今天没有读它们的地方：`FinancialControlCheckFor` 把接受侧结论 `RESTRICTED` 一律译成确定性不通过，
`Decide` 据此形成拒绝。等于对每份合同都按 `REJECT` 处置；租户若在正文里登记 `AUTHORIZED_DISPOSITION`，委托会被自动拒绝
而不是进入授权处置。ADR-0125 把这一格记为过渡状态，不是产品口径。

## 要裁的

1. 「授权处置」在本上下文是什么：授权角色对一项不通过的控制能做什么——CONTEXT 写「人工处理不得绕过硬规则或把缺少的
   权威结果改成通过」，所以它大概率不是「放行」而是「决定去向」（拒绝 / 让客户补 / 换范围形成关联新委托…），封闭集要裁。
2. 它是接受判断任务上的一种等待态（与`等待人工复核`、`等待受控补充`并列，各有续办方与读面，ADR-0086 / ADR-0106 的形状）
   还是复用`等待人工复核`。分格判据是续办方是不是同一个角色、完成动作是不是同一个命令。
3. 读处置的时机与范围（票 03 已裁，这里只落地）：只在有项受限时读；只读绑定到本次委托费用范围的那几行——闭包里已采用
   结算政策的 `Applicability().ChargeScope()` → `PreAcceptanceFinancialControlPolicy.ItemsFor`，与 SA 执行时同源；按受限项的
   控制种类对上那一行的处置。读口在 PC（`PreAcceptanceFinancialControlPolicyContentView`），PS 侧适配器归本票，落在
   `internal/parcelshipment/adapters/partycommercial/` 新文件。
4. 责任引用在 PS 侧的落点：拒绝决定的依据里带不带；补偿（释放失败）续办时用不用。

## 边界

- 不动 SA；不改 PC 正文形状（`ControlFailureDisposition` 两值是 ADR-0115 定的）。
- 票 03 落地后 `FinancialControlResult` 已逐项，受限项的种类与顺序可直接对到正文行。
- 读口与流程同票落地：没有消费方的读路不单独建（ADR-0122 决定五、ADR-0125 同一判据）。

## Comments

- 2026-09-07 · 通道 3：由票 03 第 2 问拆出立票，只写票面，未动代码。能力边界同票 03「裁决」节；此外读过 PC
  `pre_acceptance_financial_control_policy.go` 全文与 UC-PS-001 校验组行。
