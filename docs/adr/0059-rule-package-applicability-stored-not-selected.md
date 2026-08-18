# ADR-0059: 规则包五维适用性照存但不参与选择

Status: Accepted  
Date: 2026-08-18

## Context

`AcceptanceRulePackage` 的正文含 `RulePackageApplicability` 五维（服务产品、客户合同、责任法人、范围、期间）与按 `RuleCategory` 归档的规则引用。评审第 7 条要求这份正文落到持久化面。

[design.md §1](../../.scratch/product-version-closure/design.md) 的族归属判据是「进不进 `ViewRevision`」：凡参与「选哪个候选」的内容必须进视图，否则失效检测会答「还是同一个视图」；凡选出之后才读的内容绝不能进，否则一次与选择无关的改动会把该范围全部在途解析判成已失效。

今天 `ResolveCommercialBasis` 选接单规则包时**完全不看这五维**——只按版本壳的 `scope` 相等与 `selectionInterval` 命中筛。正文里的五维在解析路径上是死字段。把它们写进 `CommercialRegistry` / `ViewRevision`，等于用一次迁移把「尚未被选择逻辑消费的字段」变成失效检测的基数，而 `AT-PC-021`「同一范围两个合同同时命中」一路的冲突判定会跟着变。

这是领域决定，不是持久化能代拍的形状问题（open-decisions D-3）。本记录把当前解析行为如实落成族 B 点读，并把反向的代价写清楚。

## Decision

**一、规则包正文按「不参与选择」落族 B 点读。** 装载口是独立只读端口，按已唯一选出的接单规则包版本取回。无父行 = `found=false`（未配置）；父行在场而零子行 = error（领域要求至少一条规则，空包等于无条件接受）；读取失败是另一格。本上下文不提供默认正文，也不提供 Save。

**二、五维适用性照存，但不进 `CommercialRegistry` / `ViewRevision`，也不改 `ResolveCommercialBasis` 的候选过滤。** 存内容不等于参与选择。适用范围的 `scope` / 期间与版本壳上的范围 / 生效区间是两份事实，选包继续走壳。

**三、日后若改为参与选择，走新决策与新迁移。** 反向（点读族改成视图族）需要把五维纳入 `ViewRevision` 派生、改写候选过滤，并重新评估 `AT-PC-021` 的冲突判定基数。本表不预支那条装载路径，也不把五维列改成可空「以后再用」。

## Consequences

- 规则包正文改动不推动该范围的 `ViewRevision`。一份已固定解析不会因为租户补登记了规则引用而在提交前失效检测里变成「视图已变」。
- 五维继续无人被选择逻辑读取——这是本决策的显式代价，说明它要么该被删、要么该被用；删或用都不是本记录的范围。
- PostgreSQL 点读口按租户与拥有版本一次快照取回；显式租户必须与拥有规则版本同一身份（ADR-0003 / ADR-0040）。`category` CHECK 镜像 `RuleCategory` 五值封闭集。
- 阶段内容声明族（ADR-0058）仍是另一组点读口，本记录不把它并进规则包正文表。

## Alternatives considered

- **五维参与选择，进 `CommercialRegistry` 与 `ViewRevision`。** 否决：那是改解析语义，不是给现状补一张表。当前选择逻辑不读这五维；提前写进视图会让正文改动误伤全部在途解析，且 `AT-PC-021` 的冲突判定基数会在没有用例改写的情况下被悄悄拓宽。
- **不存五维，只存规则引用。** 否决：正文类型含这五维，漏存会让装载重建不出 `AcceptanceRulePackage`，等于用持久化面删字段。
- **未配置时由装载口代拟空规则包。** 否决：空包等于无条件接受，正是 `NewAcceptanceRulePackage` 要挡住的默认。

## Links

- [ADR-0042：接受内容声明按对象归属建模](./0042-acceptance-content-declarations-by-owning-object.md)
- [ADR-0003：集团租户边界](./0003-group-tenant-legal-entity-customer-account.md)
- [ADR-0040：商业版本身份键携带 TenantID](./0040-commercial-version-key-carries-tenant-id.md)
- [UC-PC-002](../application/party-commercial/UC-PC-002-RESOLVE-COMMERCIAL-BASIS.md)：`AT-PC-021`
