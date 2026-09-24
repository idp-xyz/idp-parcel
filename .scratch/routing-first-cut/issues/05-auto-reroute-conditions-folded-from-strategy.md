# 05 自动改路条件由策略与计划事实折出，改善阈值按策略判

Category: enhancement
Status: ready-for-agent
Blocked by: 04
父票：[psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md)「路由策略族」那一步
地盘：network-routing 领域与复核编排；路由策略版本内容（自动改路开关与改善阈值的形态）；与自动改路事实目录的关系。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定七；network-routing [`CONTEXT.md`](../../../docs/domain/network-routing/CONTEXT.md)「路由稳定性与受控改路」诸句；[UC-NR-003](../../../docs/application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md) `AT-NR-037`、`AT-NR-041`、`AT-NR-042`。

## 做什么

1. 今天 `EvaluateAutoRerouteConditions` 收的是已经折好的布尔，`AutoRerouteFacts` 的注释把折法记为「PAR-NET-14 实例半边」。ADR-0146 之后折法是产品策略：由策略版本（是否允许自动、改善阈值的形态与取值）、04 的前缀与冻结判定、复核触发的可控节点折出。
2. **改善阈值**：当前计划仍可执行时，新候选按首个内置形态那一维（成本）的改善超过策略阈值、且在冻结边界前，才允许自动切换。
3. **定自动改路事实目录此后的角色**（[`syn-wall-door-audit/05`](../../syn-wall-door-audit/issues/05-auto-reroute-facts-catalog-unimplemented.md) 建的那一册）：保留为登记来源、改为折叠结果的留痕，还是退场——写理由，不留两处权威。

## 不做

- 不定任何租户的阈值与授权分派；人工决定所需的授权目录归 party-commercial。

## 完成判据

- [ ] 复核用例覆盖 `AT-NR-037`（允许自动 → 新计划）、`AT-NR-041`（过冻结不切）、`AT-NR-042`（不满足 → 只建议）。
- [ ] 策略版本未声明自动改路时只形成建议、不自动，与今天「事实目录未配置即不猜」同一纪律。
- [ ] 事实目录的去留写进 CONTEXT 或 ADR，代码与之一致。
