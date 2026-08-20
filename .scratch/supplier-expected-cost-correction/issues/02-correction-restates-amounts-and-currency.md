# 纠错版本重述金额与原币币种，撤掉同币种等值的首版例外

Category: bug
Status: draft
Blocked by: 01

按 [ADR-0067](../../../docs/adr/0067-cost-correction-restates-the-whole-evaluation-result.md) 决定二、
四、五落地**金额与币种那一组**。三处引用（采购规则版本、协议引用、发生项版本）不在本票，见 `03`。

拆票理由：金额那组今天没有应用层调用方（`01` 已取证），改它代价近零；三处引用牵到幂等三维与
`UC-SA-004` 逐行匹配的读侧，要另外取证。

## 要改的

**`internal/settlementaccounting/domain/supplier_expected_cost.go`**

- `AppendCorrection` 入参长出原币币种与原币金额；两者与结算金额、换算步骤一同取自新评价。
- 换算步骤必备那条检查改按**本版**的币种对判断（新原币币种 vs 合同结算币），不再按被纠正版本的
  币种对。首版跨币种而纠错版本同币种、以及反过来，都是合法形状。
- 同币种两额必须相等这条检查加进 `AppendCorrection`；`FormSupplierExpectedCost` 那条不动。
- 合同结算币不接受入参：它是合同交给评价的输入，纠错不改合同。

**`internal/settlementaccounting/domain/supplier_cost_rehydration.go`**

- `RehydrateSupplierExpectedCost` 的同币种等值检查从「只对非纠错版本」改为对所有版本。
- 重建门保留（ADR-0028），但那段解释它为何不走形成门的注释**整段作废**。现在的理由是回指与
  原因成对且不自指这两条形成门不接的字段，不再是「形成门的等值只对首版成立」。

**`migrations/settlement_accounting/`**

- 新文件（现有最大号 `0011`，取 `0012`；**推送前先在频道占号**，MCP-2 的 OUTBOX-PARTITION-KEY-A
  也在 SA 的持久化面作业）：`DROP CONSTRAINT supplier_expected_cost_same_currency_amounts_agree`
  后按无条件形式重建。表此刻无真实数据，直接换无需清洗。
- **不就地改 `0008`**——已施加的迁移不可改写，校验和把关。0008 里那条 CHECK 的注释此后与现行
  口径相反，新文件的注释要写清它取代了哪一条以及为什么。

## 验证

- 领域用例补一条：同币种首版经 `AppendCorrection` 得到的版本，两额相等且能原样喂回形成门。
- `adapters/postgres/supplier_expected_cost_test.go` 的 `TestACorrectionVersionKeepsItsBackReference`
  是当初把分歧打红的那条，改口径后它要在新 CHECK 下仍绿。
- 现有 `TestACorrectionAppendsWithoutRewritingTheOriginal` 走的是跨币种，签名变了要跟着改，
  且「原费用与原换算依据保留」那两条断言必须留着——那是 `AT-SA-178` 要的。

## Comments

- 2026-08-20 · MCP-3：随 `01` 的裁断创建，`Status: draft` 等 MCP-1 派工。
- 2026-08-20 · MCP-1：裁断（ADR-0067 与 CONTEXT 改动）已入库。本票是领域不变量实现，按 `01`
  票面硬约束**等用户点头后再派工**；点头前保持 draft，不开工。
