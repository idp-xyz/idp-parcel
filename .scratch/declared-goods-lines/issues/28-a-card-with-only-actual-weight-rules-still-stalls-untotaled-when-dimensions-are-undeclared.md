# 28 只带实重规则的卡在无尺寸申报时仍落无合计、归等运营登记——既登不开也没人补

Category: bug
Status: in-progress
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

## 分诊（2026-11-12，会话代小包计价 owner 定，可推翻）

- **取第一种：急读改懒。** 判据直接复用 ADR-0381 决定二已有的 `RequiresDeclaredDimensions`——引擎与试算分流同一把尺子：评价器在 `Features()` 报缺尺寸而方案不读任何按尺寸派生的量（判定条件的五种几何 / 体积来源都不读、重量策略不是 `MAX`）时，改派只带重量的特征，形状与聚合主体（票级 / 主单级，ADR-0111）的特征相同；几何条件读到缺席照旧报特征不可用。真读尺寸的卡照旧停 `PACKAGE_FEATURES_UNAVAILABLE`，分流照 ADR-0381 点名 `DIMENSIONS`。不取第二种（把这一停也点名成缺项 `DIMENSIONS`）：那会催客户补一件规则用不上的资料，且把「卡一侧」与「客户一侧」的边界说反。
- **影响面**：急读在共享的 `evaluate()` 里，受理前估价、正式 `SELL` 评价与运营试算三条路同受影响、同被本票修复；既有能形成的评价一条路径都不变（只动「缺尺寸且方案不读尺寸」这一支，此前它必停）。
- **机制依据**：`PackageFeatures` 本就允许几何量缺席（聚合主体只带重量与类别，`feature_condition.go` 的 valid/hasDimensions/weightOf）；重量策略只有 `ACTUAL_ONLY` 与 `MAX` 两种，`ACTUAL_ONLY` 的基础计价重量与派生自身重量都不要尺寸。
- 修复落地后，ADR-0381「对已接受记录的作用」里「那一格此后剩下的才真是价卡一侧的」对这个口子成立，给 ADR-0381 的 Status 行补一条读法注记指本票。

## 实现（2026-11-12）

- `internal/parcelpricing/domain/evaluation.go`：急读 `Features()` 报 `ErrMissingDimensions` 且 `!plan.RequiresDeclaredDimensions()` 时改派 `weightOnlyFeatures()`，评价继续；解释里记一行 `dimensions undeclared; plan reads no dimension-derived feature, judged on weight-only features`。其余错误与真读尺寸的卡照旧停 `PACKAGE_FEATURES_UNAVAILABLE`。
- `internal/parcelpricing/domain/input.go`：新增 `weightOnlyFeatures()`——只带实重的特征，形状与聚合主体分支相同。
- `internal/parcelpricing/domain/weight_only_card_no_dimensions_test.go`：三组用例——实重附加费的卡无尺寸评价形成、阈值照判（25kg 命中 / 10kg 不命中）；自身计价重量拒收条款（`ACTUAL_ONLY`）无尺寸判出拒收答不可计价；读最长边的卡无尺寸照旧停 `PACKAGE_FEATURES_UNAVAILABLE`。既有 `TestMissingDimensionsLeaveTheDerivedWeightsUnavailableNotZero`（`MAX` 策略）不动、仍绿——`MAX` 经 `RequiresDeclaredDimensions` 照旧要尺寸。
- `docs/adr/0381-…md`：Status 行补读法注记（本票），正文与分流不动。
- 编排一侧零改动：估价口的分流（`form_estimate_evaluations.go` 用同一判据预点名 `DIMENSIONS`）与结算折叠处照旧；受理前估价、正式 `SELL` 评价与运营试算三条路同走 `evaluate()`，同被修复。
