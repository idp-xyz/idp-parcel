# E1 · 客户报价表规则特征 → parcel-pricing 既有形状的表达力核对

Category: chore
Status: done——只读核对，一次交全表；三处形状缺口各立子票（`issues/01`–`03`，draft 待 owner 裁），其余逐项指到承载它的已有形状。不填任何取值。

## 这份报告是什么、不是什么

[实施检查单](../tenant-implementation-01/implementation-checklist.md) E1 的完成判据：「DIM 250、1LB 向上取整、体积重与实重取大、燃油率及折扣、PSS 生效窗、at-cost 转嫁项、按 MAWB/票/KG/件的多计费单位——每一项都能指出在现有价卡与参考系列模型里由**哪个已有形状**承载，或明确记为形状缺口。」本报告逐项答这一句，并把 [登记册草案](../tenant-implementation-01/instance-register-draft.md) 与 [需求对照](../tenant-implementation-01/requirement-mapping.md) 里已提取的其余规则特征（分区×重量矩阵、附加费按区间、偏远邮编分类、生效/失效日期、双币）一并核。

**这是表达力核对，不是填数**：核的是产品模型够不够用，填数属实例半边，等 D2。**核对对象是仓内已提取的特征清单，不是重新解析客户 xlsx**——那批文件（`docs/reference/xls/`，`.gitignore` 挡着、从未进历史，本轮 `git ls-files` 复核为零）含客户全套成本结构，本报告未打开任何一份；特征清单出自 MCP-4 2026-08-31 逐表解析后写进登记册草案的那几行。E2 转换工具开工时若在表里撞见本清单之外的特征，追加在本目录。

**取证锚**：`internal/parcelpricing/domain` 读于 `a17bfac`；形态决定引 [`pp-pricing-rule-model-final-design.md`](../../docs/design/pp-pricing-rule-model-final-design.md) 与 [parcel-pricing CONTEXT](../../docs/domain/parcel-pricing/CONTEXT.md)。证据等级 `S`（读符号，未跑评价）。渠道以公开承运商名指称，不写客户名、渠道账号与任何金额。

## 逐项核对

| # | 客户表里的规则特征 | 承载它的已有形状（符号） | 结论 |
|---|---|---|---|
| 1 | 分区 × 重量段矩阵（末端六渠道基础运费） | `RateTableFamilyWeightZone` 的 `RateEntry`（分区 + 重量上下界 + 金额），`RateTableVersion.Lookup` 按分区与计价重量查档；区间空档/重叠在构造期拒 | **已承载** |
| 2 | 按 KG 计价（专线 CNY/KG） | `RateTableFamilyUnitPrice` 的 `UnitPriceRate`（每单位计费重一个单价） | **已承载** |
| 3 | 首重 + 续重（若某渠道表如此） | `RateTableFamilyFirstContinue` 的 `FirstContinueRate` | **已承载** |
| 4 | DIM 250（体积磅 = 英寸长宽高 / 250） | `VolumetricFactor{divisor, lengthUnit=IN, rounding}`；注释原句即「L4 写的是…/250，所以本包从不自带任何系数」 | **已承载**；250 是实例值，不入仓 |
| 5 | 体积重与实重取大 | `PricingWeightMethod` 的 `PricingWeightMax`（`ACTUAL_ONLY` 不得声明系数，`MAX` 必须声明） | **已承载** |
| 6 | 1 LB 向上取整；不足 1 LB 按 1 LB | `WeightRoundingPolicy{RoundingCeiling, increment 1 LB}`；分段进位（某重量以下按 oz、以上按 lb）由 `WeightRoundingSegment` 表达；`WeightUnit` 有 `OZ`/`LB`/`G`/`KG` | **已承载**；`RoundingMode` 只有 `NONE`/`CEILING`，客户表未见四舍五入或向下，够用 |
| 7 | 燃油率（承运商每周公布）及折扣（公布率 × 折扣系数） | `ReferenceSeriesBinding{FUEL_RATE, seriesID}` 绑序列标识（ADR-0099）；`NewSeriesRateSurcharge(kind, factor, basisDependencyID)` 把「当周费率 × 系数」两数分开保留；基数由 `ChargeDependency`（`ALL_CHARGES` 减排除集 / `LISTED_CHARGES`）声明 | **已承载**；序列取值是实例半边，经 `pricing-reference-series-operations` 那套登记/复核进在用 |
| 8 | 附加费按分区/重量区间分档 | `NewTableLookupSurcharge(table)`（附加费价表按分区分档）；阈值型（超尺寸/超重）用 `TriggerCondition` 上的 `FeatureCondition`（`GT`/`GE`/`LT`/`LE`，长度/重量/体积三纲）+ `NewFixedAmountSurcharge` | **已承载** |
| 9 | 定额与百分比取大、最低计费重 | `NewGreaterOfSurcharge`；`ConditionalMinimumWeight` 抬高方案级计价重量 | **已承载** |
| 10 | 折扣（基础运费 × 折扣率） | `SurchargeRule` 用 `ChargeEffectDeduct` + `NewPercentOfBasisSurcharge`，基数指名一条 `ChargeDependency` | **已承载** |
| 11 | 附加费互斥（AHS 三类取一）与并列计收（住宅 + 偏远同时收） | `ExclusivityStance` 三值（未声明的卡拒发布）、`InExclusivityGroup(group, priority)` / `Standalone()` | **已承载**（形态决定五、六） |
| 12 | 拒收条款（超限不计价） | `ExclusionRule` 与附加费共用触发文法，结果指名条款 | **已承载** |
| 13 | 生效/失效日期（每张表一个区间；同一表内某日新增条款） | `EffectivePeriod` 在 `PricingPlanVersion` 与 `RateTableVersion` 上，方案期须落在价表期内；表内某日新增 = 新方案版本自该日生效（形态决定七、需求依据第 7 条已这么处理） | **已承载** |
| 14 | 双币（专线 CNY、末端与清关 USD） | 每张方案一个币种（`RateTableVersion.currency`，附加费币种须一致，构造期拒）；结算币种经 `PricingInputSnapshot.WithSettlementCurrency` 进来，换算走 `EXCHANGE_RATE` 序列 + `ConversionStep`，口径由商业价格政策版本化声明 | **已承载**；「一个合同一个结算币种」的商业解释归 D-04，不是计价形状问题 |
| 15 | 偏远（DAS 普通/扩展/极偏远/Alaska/Hawaii，按邮编分类）与住宅派送 | 触发侧：`FeatureAddressType` / `FeatureZone` 类别特征 + `ComparisonEquals`；CONTEXT「地址分类」词条「由邮编分类事实提供的偏远档位」 | **触发侧已承载；提供侧缺**——邮编 → 档位 / 邮编 → 分区 的分类事实在仓内没有任何登记载体，见 [issues/01](./issues/01-zip-classification-facts-have-no-register.md) |
| 16 | PSS / 高峰附加费：只在某日期窗内生效，金额逐周变 | 无：`FeatureSource` 十项里没有日期/时点；`ReferenceSeriesKind` 只有 `FUEL_RATE`/`EXCHANGE_RATE`，序列取值只作百分比费率用；`EffectivePeriod` 是整张方案的 | **形状缺口**——今天唯一能表达的是把方案按窗口切成三个版本，逐周金额则要逐周重登；见 [issues/02](./issues/02-date-windowed-surcharge-has-no-shape.md) |
| 17 | 清关报价：按 MAWB / 按票 / 按 KG / 按件多计费单位 | 按 KG → `UnitPriceRate`；按件 → `FixedChargeRule`（`AggregationPerPackage`）；**按票、按 MAWB → 无**：`AggregationMode` 唯一取值 `PER_PACKAGE`，`EvaluationSubjectKind` 只有已受理包裹与试算 | **形状缺口**（一半）——见 [issues/03](./issues/03-shipment-and-mawb-level-billing-units-have-no-evaluation-subject.md) |
| 18 | at-cost 转嫁项（实报实销，供应商账单为准） | **已有决定，不是缺口**：形态决定九「实报实销费用不进计价评价，由 `settlement-accounting` 作为独立费用项形成，来源为供应商账单」（`UC-SA-004` 账单主张路径）。SA CONTEXT 有「客户代垫回收」（税费类）与「供应商账单主张 → 审核应付」 | **已承载于 SA**；一处待核（不在本报告范围）：服务类 at-cost 项对客转嫁的客户费用项如何引用供应商费用项、加不加处理费——归 SA 核，`UC-SA-004`/`UC-SA-001` |
| 19 | 金额精度（CNY/KG × 汇率 → 六位小数） | 已单独立票 [`pricing-amount-precision/01`](../pricing-amount-precision/issues/01-evaluation-amount-scale-has-no-declared-source.md)（draft，MCP-3 本波裁） | **已在票上**，本报告只指过去 |
| 20 | 一表多签（每渠道/每产品一张） | 每签一张方案（`PricingScopeID` 区分）；签间共用的燃油/汇率序列各自绑定 | **已承载**；拆签属 E2 转换工具 |

## 三处形状缺口的共同形状

三处都不是「参数没填」，是**表达位不存在**：邮编分类事实没有登记载体（15）、日期窗与逐周金额没有落点（16）、票级/主单级没有评价主体（17）。前一处是提供侧（谁把事实交给计价），后两处是计价模型本身。三处都属难逆转取舍（改 CONTEXT 闭合集合 / 改规范化形状 / 改聚合方式），按 AGENTS「改文档」走 owner 裁或 ADR，本报告只把选项摆出来，写在各子票里。

**不要为了让 E2 转换工具跑通而在工具里绕**：把 PSS 折进基础运费、把 MAWB 费按包裹平摊后写成定额、把邮编档位手写进每张卡——三种都是在没人声明过的地方发明规则，与 AGENTS 红线「只实现已确认规则」正面相抵。E2 在三票裁定前只转 1–14、18–20 那些已承载的部分，缺口项如实列「未转换：等票 0N」。

## 对检查单的影响

- E1 的完成判据已满足（每项或指到形状、或记为缺口）；**E2 可开工**，范围收窄如上。
- D-02（证据目录移出工作区）票面写「排在 E1 取证结束后同日执行」——本报告未打开那批文件，E1 不再是它的前置，**A2 今天就可以做**，归用户。
