# `parcel-shipment` 的 `FinancialControlResult` 一个请求只装一个结果——组合控制的逐项结果与共同通过条件在 PS 域里无处表达，今天靠消费适配器折叠

Category: enhancement
Status: draft——由 [ADR-0122](../../../docs/adr/0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md) 决定四拆出（2026-09-07，通道 1，票 02 落地时）；PS 地盘，认领者按开工那刻的 tip 重核事实链
Blocked by: 无（SA 侧已逐项交回，见票 [02](./02-load-control-policy-reads-policy-content-items.md)）

## 缺口

ADR-0122 起，`settlement-accounting` 对一次接受前控制请求按策略正文逐项执行，`ApplyPreAcceptanceControlResult` 交回
`ExecutedControls()`（每项的种类、判断顺序、有没有形成`业务限制`）、`Freeze()` 与 `Exposure()`（可同时在场）与
`JointPassCondition()`。CONTEXT 写「合同明确组合多项接受前控制时，每项结果必须保持独立依据和有效性，由 `parcel-shipment`
按策略的共同通过条件形成接受判断」。

`parcel-shipment` 今天的 `psdomain.FinancialControlResult` 是**一个请求一个结果**：`HELD` / `CREDIT_EXPOSED` / `RESTRICTED` /
`NOT_APPLICABLE` 四格加一条依据。逐项结果装不进去，于是 `internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control.go`
的 `appliedControlAssessment` 在适配器里按共同通过条件折成一个：任一项受限即 `RESTRICTED`（带那一项自己的原因），全部成立时有冻结即
`HELD`、只有暴露即 `CREDIT_EXPOSED`。

折叠的位置对（判接受的是 PS），但它把三件事压成了一格：

- 哪几项执行了、各自的结论——事后看接受判断时只见一个 `HELD`，看不见同一请求还记了一笔信用暴露。
- 共同通过条件本身——今天只有 `ALL_CONTROLS_PASS`，折叠等价于「任一受限即拒」；放宽出第二种组合子时适配器会先炸（全函数），
  但 PS 域里没有一格能装「按哪个条件判的」。
- 「成立但有多项」与「成立只一项」在 PS 结果上分不开——释放与追溯都按同一请求身份认领，今天没出事，是因为 SA 那侧释放
  已改为两本账各认领一次（ADR-0122 决定三），不是因为 PS 知道有两项。

## 要裁的

1. `FinancialControlResult` 长成「逐项结果集合 + 共同通过条件 + 按条件判出的接受侧结论」，还是并列一个新对象让既有接受编排继续读旧形状？
   UC-PS-001「接受前财务控制」那一行与 `AT-PS-*` 里引用 `HELD` / `CREDIT_EXPOSED` 的判据要跟着改口的有哪些。
2. 失败处置（`REJECT` / `AUTHORIZED_DISPOSITION`）与责任引用今天不随 SA 答复走（ADR-0122 决定四）：PS 形成接受判断时要用它们，
   经自己的 PC 消费缝读策略正文，还是只读绑定到本次委托费用范围的那几行？读口形状归 PC（`PreAcceptanceFinancialControlPolicyContentView`
   已在），PS 侧适配器归本票。
3. 一项受限即停后续项（ADR-0122 决定二）之下，`ExecutedControls` 短于策略控制项——PS 结果要不要区分「未执行」与「执行且成立」。

## 边界

- 不动 SA 任何一格；不改 `ApplyPreAcceptanceControlResult` 的形状（要加格回 SA 票）。
- 裁前 `appliedControlAssessment` 的折叠照旧；裁决若改 PS 结果形状随实现落 ADR（引本票与 ADR-0122），编号取当时下一号。

## Comments

- 2026-09-07 · 通道 1：由 ADR-0122 决定四拆出立票，只写票面，未动 PS 代码。能力边界：读过 PS 消费适配器全文与 SA 侧全部改动；
  **没读** PS 接受编排读取 `FinancialControlResult` 的那一段与 UC-PS-001 全文——第 1 问的措辞据 CONTEXT 与适配器代码写，开工者以代码为准。
