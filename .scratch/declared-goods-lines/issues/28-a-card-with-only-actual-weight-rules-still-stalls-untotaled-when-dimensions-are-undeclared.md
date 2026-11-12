# 28 只带实重规则的卡在无尺寸申报时仍落无合计、归等运营登记——既登不开也没人补

Category: bug
Status: needs-triage
Blocked by: 无

依据：票 26 评审发现（2026-11-12）；[ADR-0381](../../../docs/adr/0381-an-incomplete-estimate-declaration-waits-for-customer-supplement-or-operator-registration-by-who-supplies-the-gap.md) 决定二的边界之外。

## 现状

计价引擎对带任何附加费规则或拒收条款的卡急读特征派生量：`internal/parcelpricing/domain/evaluation.go` 在 `surchargeRules` 或 `exclusions` 非空时无条件调 `request.input.Features()`，而 `Features()` 对单件输入无条件要尺寸（`ErrMissingDimensions`）。于是一张只按**实重**判附加费或拒收的卡、申报没带尺寸时，评价落 `PACKAGE_FEATURES_UNAVAILABLE` 的无合计 → 结算折 `ESTIMATE_TRIAL_WITHOUT_TOTAL` → 等运营登记（ADR-0383 决定一）。

这一停两头都不对：运营登不出什么能解开它（卡本身没问题），客户也收不到「缺尺寸」的信号。ADR-0381 决定二只分流「卡有按尺寸判的规则而尺寸没报」（`RequiresDeclaredDimensions` 按判定条件读的派生源与重量策略体积重判），只读实重的规则不在其内——与 ADR 文字一致，但 ADR-0381「对已接受记录的作用」里「那一格此后剩下的才真是价卡一侧的」对这个口子不成立。

## 要定的

- 改计价引擎把急读特征改懒（只在规则真读尺寸派生量时才要尺寸），还是把这一停也点名成缺项 `DIMENSIONS`（等客户补，哪怕规则实际用不上尺寸）。前者更准，动的是评价引擎的派生时机；后者改动小，但会催客户补一件可能用不上的资料。
- 正式评价与试算同受影响吗（`Features()` 的急读不只在估价路上）。

## 不做的

本票不改 ADR-0381 已定的分流；定下改法后如动领域语言再回 ADR。
