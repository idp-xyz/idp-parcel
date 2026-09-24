# 10 候选成本：计划履约段的 BUY 评价经 parcel-pricing 合成候选成本

Category: enhancement
Status: draft
Blocked by: 02、03、09
父票：[psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md)「接路由证据取数侧」那一步（成本），也是「首个内置排序策略」的事实来源
地盘：network-routing 的出向端口与 parcel-pricing 消费方适配器（NR 侧）；目录线路版本上指向 BUY 价卡的引用列（新迁移号开工时预留）。parcel-pricing 若需新口，先在频道与 PP 地盘的主人约。
出处：02 的 ADR（成本口径）；[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md) `PAR-NET-16` 已确认的机制句；[`label-channel-service-first-release/13`](../../label-channel-service-first-release/issues/13-buy-evaluation-to-cost-score-bridge.md)（PP 批量评价口，与「PP 只答算不算得出」的分工）。

## 做什么

1. 目录线路版本带它的 BUY 价卡引用：形状归产品，引用哪一份卡是租户取值。
2. 取数侧对每个候选逐段向 PP 取 BUY 评价，按 02 的口径合成候选成本，交 03 定的成本事实形状；任一段待判断或不可计价，该候选成本即不可得。
3. 与 03 合起来：合成目录上，初始路由按成本单维选出唯一候选并形成计划；两候选成本相同则交冲突。

## 不做

- 不在 NR 复制计价规则；不定任何价卡内容。

## 完成判据

- [ ] 真库或进程级用例：两候选成本不同 → 形成计划（`S`）；成本相同 → 冲突；一段不可计价 → 该候选出局。
- [ ] 取数侧的成本缺口不折成零。
