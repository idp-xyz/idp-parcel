# 10 候选成本：计划履约段的 BUY 评价经 parcel-pricing 合成候选成本

Category: enhancement
Status: in-progress——2026-10-08 TraeCode 会话认领（用户令「开始接下一张票」；阻塞 02、03、09 均已在 main）。此前：ready-for-agent
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

## 裁决（2026-10-08，用户代 PP / PC owner 拍板；据此地盘从「仅 NR 侧」扩为 NR + PP + PC）

- **比较币种换算随本票落地。** ADR-0148 越权风险点 1 的 PP 侧延伸——换算从结算币种延伸到比较币种、逐段保留原币 / 结算币种 / 所引汇率序列版本与比较币种金额——由本票实现；口径仍由价格政策声明、比较币种仍是路由策略版本上的租户取值。「与 PP 主人约」落地为这次用户裁决。
- **自营段随本票落地。** 越权风险点 2：内部价格政策承载自营段（含集团内另一法人承运段）的内部标准成本，经 PP 同一评价机制出价；引用形状同时承载外包 BUY 价卡与内部政策价卡两形。

## 实施方案（2026-10-08 认领时草定，随实现修订）

1. **PP · 比较币种换算（内部延伸，不新开对外口）**：`PricingInputSnapshot` 增 `WithComparisonCurrency`（与 `WithSettlementCurrency` 同款）；评价成形在结算换算之后，若声明比较币种且不等于卡币种，取同一 EXCHANGE_RATE 序列读数做第二笔换算（卡币→比较币），存 `comparison *ConversionStep`；比较金额**不**过卡的 `AmountRoundingAfterConversion`——它留给 NR 求和后取整一次。比较币种未声明 = 老行为，等于卡币种 = 无第二笔。`Currency` 增 ISO 4217 最小币单位（按公开基准内置，只录价卡出现过的币种，未录的如实答否 → 该段待判断）。摘要 canonical doc 增 comparison 节；解释行补一句；postgres 评价存储相应扩展 + 迁移。测试：同一读数两笔换算、等于卡币种无步、未声明向后兼容、摘要含 comparison。
2. **PC · 内部成本政策读口（只读新口）**：外部段引 PC 价格政策（InternalDirection），NR 取数侧要一个口把它解成绑定的 PP 方案引用（含 standing 三格）。先在仓库里核商业政策读侧有没有近似口，没有则页 `ports.InternalCostPolicyView` + postgres 适配器 + 真库测试。
3. **NR · 路由策略载体 + 目录（新迁移 `network_routing` 0016）**：策略版本加「比较币种」与所引价格政策两项（ADR-0148 Consequences：「随 /03 载体或 /10 接成本时补」）；线路版本段连各带一条成本依据引用：外包 BUY 价卡（方案 id/version）或内部政策（policy ref）两形；登记 / 读回 / 清单照既有目录写口同款。
4. **NR · 取数侧适配器（`internal/networkrouting/adapters/parcelpricing/`）**：逐候选逐段解引用 → 外包直接得方案，内部经 PC 口得方案 → 组装计价输入（实例半边口，`未配置`即停下，不编输入）→ `EvaluatePricingAcrossPlans` 批量评价 → 逐段译三态 → `Money.Add` 不舍入求和 → 按比较币种最小币单位取整**一次** → 合成 `CandidateCostFact`。段级明细（评价标识 + 方案引用 + 政策引用）随证据留存。策略版本没登比较币种时退回同币种合成，异币种由既有 `RankingCurrenciesDiffer` 未决格接。
5. **接线与验证**：`InitialRouteEvidence.CandidateCosts` 在证据装配处填上（组合点按装配器现状定）；新导出工厂补 `production_wiring_baseline`；真库用例三格（不同 → 计划；相同 → 冲突；一段不可计价 → 出局）+ 缺口不折零；全仓门禁后完成记录、评审、重放进 main。
