# 纠错版本重述采购规则版本、协议引用与发生项版本

Category: bug
Status: in-progress
Blocked by: 01

按 [ADR-0067](../../../docs/adr/0067-cost-correction-restates-the-whole-evaluation-result.md) 决定二落地
**引用那一组**。金额与币种那组在 `02`。

## 病灶

`AppendCorrection` 换掉评价引用却把三处引用原样留在旧评价上：

- `ruleVersion`（采购价格规则版本）——`AT-SA-054` 的成因就是采购计费规则被更正，新评价必然引新
  规则版本，纠错版本却仍写旧的。
- `agreement`（供应商协议引用）——同属评价的版本清单。
- `occurrence`（运输收费发生项引用，含版本与业务时点）——`OccurrenceVersion` 的类型注释自己写着
  「发生项有效性更正换版本，预期成本据以追加计价纠错」，而 `AppendCorrection` 不换它。`AT-SA-164`
  的「运输履约使原发生项失效 → 保留原预期成本并追加计价纠错」正是这条路径。

与 `02` 是同一个毛病：换了评价只搬来一部分产物，一份版本的内容出自两个评价。

## 先取证再改

**这是本票与 `02` 分开的唯一理由。** `UC-SA-004` 逐行匹配是「按版本引用」读预期成本的，改完之后
它会读到一份采购规则版本与首版不同的纠错版本。要先回答：

1. `UC-SA-004` 的匹配是否依赖「同一成本链上各版本的规则版本一致」？今天读侧是
   `ports.ExpectedCostView` / `SupplierExpectedCosts.LoadExpectedCost`，按（租户+版本）取单行，
   看上去不依赖，但要核到用例文档而不只是当前实现。
2. 发生项引用的 `ID` 必须保持不变（那是身份四件之一，ADR-0067 决定三），只有版本与业务时点随
   评价走。要确认 `TransportChargeOccurrence` 换新引用时有一条挡住 ID 变化的检查。

取证结论写回本票 `## Comments` 再动代码。

## 要改的（取证通过后）

- `AppendCorrection` 入参长出发生项引用、采购规则版本、协议引用；发生项 `ID` 与被纠正版本不一致
  时拒。
- 幂等三维不必改：`supplier_expected_cost_first_version_unique` 的 `WHERE prior_version IS NULL` 已把
  纠错版本放在三维之外，纠错版本携带新规则版本不撞索引。这一条要有测试守住，否则下次有人看到
  「纠错版本的规则版本和首版不同」会以为是 bug 再改回去。

## Comments

- 2026-08-20 · MCP-3：随 `01` 的裁断创建，`Status: draft` 等 MCP-1 派工。取证那一步没做，本票不含结论。
- 2026-08-20 · MCP-1：**取证部分派 MCP-3**（只读：两问结论核到用例文档与代码后写回本票
  Comments）。「要改的」代码部分与 `02` 同受一道用户点头约束，取证结论落回前后都不得开工。
