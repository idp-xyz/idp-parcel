# Rating Runtime 计算语义规范

**文档版本：** V1.0.1  
**文档状态：** 计算语义终审基线 / 可进入 Golden Cases 与 API Contract 阶段  
**上游基线：**
- 《国际小包计费与结算平台最终解决方案 V1.2 审定版》
- 《国际小包计费领域模型 V1.0.1 终审版》

**适用范围：** 国际小包、国际专线、国际快递、海外仓尾程的单票确定性计费  
**权威边界：** 本文档是 Rating Runtime 算法顺序、数值精度、边界条件、价表查找、费用组合及结果证据的规范性来源

---

# 目录

1. 文档目标与规范等级  
2. 与上游文档的关系  
3. Rating Runtime 边界  
4. 核心不变量  
5. 规范性术语  
6. 基础数据类型  
7. CalculationPurpose  
8. 输入快照  
9. 业务时间与版本解析  
10. Fact 选择  
11. 单位换算  
12. Geometry 语义  
13. 地址与地理分类  
14. 包裹特征判定  
15. Actual Weight  
16. Volumetric Weight  
17. Conditional Minimum Weight  
18. Billable Weight  
19. Weight Rounding  
20. Rating Aggregation  
21. 价表家族  
22. Published Tariff 与折扣  
23. Charge 候选生成  
24. Charge Scope、Basis、Method  
25. Charge Method 算法  
26. Charge Composition  
27. 费用依赖图  
28. Fuel Policy  
29. BUY 与 SELL 计算  
30. 多币种与汇率  
31. 多段费用  
32. 宏观执行阶段  
33. Rating Evaluation  
34. 解释、证据与执行轨迹  
35. 错误模型  
36. 幂等与重放  
37. Compiled Pricing Plan  
38. Custom Function  
39. 性能与确定性  
40. Golden Cases  
41. 一致性追踪  
42. 自洽审查  
附录 A：枚举  
附录 B：算法伪代码  
附录 C：端到端案例  
附录 D：边界案例目录

---

# 0. 文档控制

## 0.1 文档目标

本文档将总体架构和领域模型进一步下沉为可实现、可测试、可重放的计算语义。

本文必须做到：

1. 两个独立研发团队按本文实现，对相同输入与相同版本产生完全相同的结果；
2. 每一个中间计算结果都有明确单位、精度、舍入阶段和来源；
3. 每一个费用行都能追踪到事实、策略、价表条目、基数和执行顺序；
4. 所有临界值均明确使用 `>`、`>=`、`<` 或 `<=`；
5. 未明确的业务规则不得由代码自行猜测；
6. 不允许通过数据库字段默认值、编程语言浮点行为或容器遍历顺序改变结果。

## 0.2 规范性词语

本文使用以下规范等级：

- **必须 / MUST：** 强制要求，违反即不符合规范；
- **不得 / MUST NOT：** 明确禁止；
- **应 / SHOULD：** 默认应遵守，仅在有正式版本化例外时偏离；
- **可以 / MAY：** 可选能力；
- **未定义 / UNDEFINED：** 不允许运行时猜测，必须返回配置错误。

## 0.3 权威顺序

发生表述冲突时，权威顺序为：

```text
已冻结业务合同与正式价卡原文
        ↓
本计算语义规范
        ↓
领域模型 V1.0.1
        ↓
总体方案 V1.2
        ↓
实现代码与数据库结构
```

实现代码和数据库结构不得反向改变本文语义。

## 0.4 文档不负责的内容

本文不定义：

- Quote 商业接受流程；
- ChargeAssessment 审核流程；
- FinancialChargeDocument 财务入账；
- 月度返利和周期 Settlement 的具体算法；
- 承运商发票格式适配；
- UI 交互；
- PostgreSQL 物理表结构；
- API 的完整 OpenAPI Schema。

本文只定义 Rating Runtime 如何得到不可变的 `RatingEvaluation`。

---

# 1. Rating Runtime 边界

## 1.1 输入

Rating Runtime 接受：

```text
RatingInputSnapshot
ResolvedContractPolicy
CompiledPricingPlan
SelectedFacts
CalculationPurpose
BusinessTime
SystemTime
RequestedPriceRole
```

## 1.2 输出

Rating Runtime 只输出：

```text
RatingEvaluation
├── Evaluation Header
├── Selected Fact References
├── Normalized Geometry
├── Weight Calculation
├── Aggregation Result
├── Charge Lines
├── Currency Conversion Evidence
├── Rounding Trace
├── Version Manifest
├── Execution Trace
└── Total Summary
```

## 1.3 不输出

Rating Runtime 不直接产生：

- Quote；
- 应收；
- 应付；
- 补扣款；
- 财务凭证；
- 承运商争议案件；
- SettlementStatement。

## 1.4 核心原则

```text
RatingEvaluation = PureFunction(
    InputSnapshot,
    SelectedFacts,
    CompiledPricingPlan,
    VersionManifest
)
```

在不考虑技术故障的前提下，相同输入必须产生相同输出。

## 1.5 不可承运的结果语义

产品准入失败属于业务不可计费，不是成功金额为 0。

处理规则：

```text
RatingEvaluation.status = FAILED
error.category = ELIGIBILITY_ERROR
no formal total
eligibility_decision_refs retained
```

多渠道比较由上层应用将该失败结果投影为 `INELIGIBLE` 候选；RatingEvaluation 聚合状态仍与领域模型保持 `FAILED`。

不得为不可承运产品生成金额为 0 的 COMPLETED Evaluation。

---

# 2. 核心不变量

1. 所有正式金额使用十进制数，不使用二进制浮点；
2. 正式费用金额为非负 `Money`；
3. 费用方向通过 `ChargeEffect: ADD | DEDUCT` 表达；
4. 物理测量和承运商计费认定不得相互覆盖；
5. BUY 与 SELL 是独立价格域；
6. SELL 可以显式引用已冻结 BUY Evaluation，但不得隐式继承 BUY 价卡；
7. Rating 只消费 ACTIVE PricingRelease manifest 中的版本；
8. 历史 REPLAY 必须使用原 Evaluation 冻结的版本；
9. 所有价表区间必须无重叠，并按本文统一边界解释；
10. 所有费用依赖图必须为 DAG；
11. 所有集合的执行顺序必须显式稳定；
12. 缺失规则、重复规则或冲突规则不得静默选取；
13. 中间结果未经配置不得隐式舍入；
14. Quote、Evaluation、Assessment 和 Financial 不得混为同一对象；
15. 单票 Rating 不读取月累计 Settlement 状态。

---

# 3. 规范性统一语言

## 3.1 Package

一个可独立测量、贴标或被承运商识别的物理包裹。

## 3.2 Shipment

一次运输业务请求，可包含一个或多个 Package。

## 3.3 Rating Subject

当前费用或计算步骤作用的对象：

```text
PACKAGE
SHIPMENT
ORDER
MANIFEST
CONTAINER
```

## 3.4 Fact

外部或内部已经确定的不可变事实，包括：

- MeasurementFact；
- CarrierAssessmentFact；
- AddressClassificationFact；
- GeographicClassificationFact；
- CommodityClassificationFact；
- EligibilityFact。

## 3.5 Policy

描述如何选择数据、如何计算或如何组合的版本化规则。

## 3.6 Plan

一组可发布、可编译的定价组件。

## 3.7 Evaluation

在已冻结事实、规则与版本下得到的纯计算结果。

---

# 4. 基础数据类型

## 4.1 Decimal

所有 Decimal 必须满足：

- 采用任意精度或足够高精度的十进制实现；
- 禁止使用 IEEE-754 binary float/double 作为正式计算类型；
- 解析字符串时不得先转 float；
- 序列化使用十进制字符串；
- 禁止科学计数法进入正式 API，除非 Schema 明确允许。

建议内部最低精度：

```text
precision >= 34
```

除明确舍入步骤外，内部计算保留完整十进制结果。

实现必须配置可接受的最大 precision、scale 和指数范围。超出范围必须返回：

```text
DECIMAL_PRECISION_EXCEEDED
```

不得截断、溢出或自动转为浮点数。

## 4.2 Money

```text
Money
├── amount: Decimal >= 0
└── currency: ISO-4217
```

`Money` 不允许负数。

正式方向：

```text
ChargeEffect.ADD
ChargeEffect.DEDUCT
```

分析差异可以使用：

```text
SignedMoneyDelta
```

但 SignedMoneyDelta 不能直接成为 ChargeLine 金额。

## 4.3 Quantity

```text
Quantity
├── value: Decimal >= 0
├── unit
└── dimension
```

dimension 至少包括：

```text
MASS
LENGTH
VOLUME
COUNT
TIME
PERCENTAGE
RATE
```

不同 dimension 不得直接运算。

## 4.4 Percentage 与 Ratio

Percentage 和 Ratio 使用 Decimal 表示：

```text
12.8% = 0.128
```

不得同时接受 `12.8` 和 `0.128` 表示同一含义。

配置 Schema 必须明确：

```text
representation: FRACTION
```

## 4.5 Timestamp

所有时间必须包含时区或使用 UTC。

业务日期解析必须同时保存：

- 原始时区；
- UTC；
- 用于价卡有效期比较的业务本地日期。

## 4.6 Stable Identifier

用于排序和余数分配的 ID 必须有稳定、全局唯一、不可变的字符串表示。

---

# 5. CalculationPurpose

允许值：

```text
QUOTE
ESTIMATED_COST
ACTUAL_COST
CUSTOMER_BILLING
SIMULATION
REPLAY
DISPUTE_REVIEW
```

## 5.1 QUOTE

使用报价时可获得的事实，输出商业报价计算依据，不代表应收。

## 5.2 ESTIMATED_COST

使用仓库或计划阶段事实，估算采购成本。

## 5.3 ACTUAL_COST

使用承运商认定或实际履约事实计算采购成本。

必须声明模式：

```text
SYSTEM_RECALCULATION
CARRIER_ASSESSMENT
```

### 5.3.1 SYSTEM_RECALCULATION

使用物理 MeasurementFact、系统 Geometry/Weight/Profile 和实际业务时间重新计算承运商采购成本。

### 5.3.2 CARRIER_ASSESSMENT

使用 CarrierAssessmentFact 中的承运商认定字段重建承运商费用语义。

权威优先级必须由 `CarrierAssessmentUsagePolicy` 明确：

```text
BILLED_WEIGHT_AUTHORITATIVE
ASSESSMENT_COMPONENTS_AUTHORITATIVE
CHARGE_LINES_AUTHORITATIVE
```

- `BILLED_WEIGHT_AUTHORITATIVE`：使用 billed_weight、billed_zone 和承运商分类作为基础价与附加费输入；
- `ASSESSMENT_COMPONENTS_AUTHORITATIVE`：使用 assessed actual、dimensional、minimum，再按系统规则确定 billed weight；
- `CHARGE_LINES_AUTHORITATIVE`：承运商账单费用行作为外部认定事实，不应由 Rating Runtime 伪装成系统重算；该模式通常由 Reconciliation 标准化，不生成重新计算的同名费用。

在任何模式下：

- CarrierAssessmentFact 不得覆盖 MeasurementFact；
- Evaluation 必须记录 `calculation_basis`；
- billed_weight 不得被标记为 physical actual weight；
- 未明确 UsagePolicy 必须返回 `CARRIER_ASSESSMENT_USAGE_UNDEFINED`。

## 5.4 CUSTOMER_BILLING

按客户合同约定的事实来源和价格方案计算最终候选客户费用。

## 5.5 SIMULATION

允许使用未激活 Draft/Test Pricing Plan，但必须标记：

```text
non_production = true
```

不得形成 Quote Acceptance、Assessment 或 Financial。

## 5.6 REPLAY

使用原 Evaluation 的输入、事实选择和 VersionManifest。

不得使用当前 ACTIVE 版本替换原版本。

## 5.7 DISPUTE_REVIEW

用于争议分析，可以同时展示：

- 系统重算；
- 承运商认定；
- 原始 Evaluation；
- 差异。

但每个结果必须是独立 Evaluation，不得混合为一套事实。

---

# 6. RatingInputSnapshot

## 6.1 快照内容

```text
RatingInputSnapshot
├── tenant_id
├── snapshot_id
├── shipment_snapshot
├── package_snapshots
├── address_snapshots
├── commodity_summary
├── customer_ref
├── contract_ref
├── service_product_ref
├── origin_ref
├── measurement_fact_refs
├── carrier_assessment_fact_refs
├── requested_price_roles
├── calculation_purpose
├── business_time
├── captured_at
└── source_references
```

## 6.2 快照不变量

1. 创建后不可修改；
2. Package ID 不得重复；
3. Shipment 与 Package 的 tenant 必须一致；
4. Package 顺序不是业务语义，执行时使用稳定排序；
5. 快照只保存引用时，引用对象必须不可变；
6. 可变上游对象必须复制为快照；
7. 快照必须保存来源系统和来源版本；
8. REPLAY 必须引用原 snapshot_id。

## 6.3 空值语义

以下三种情况必须区分：

```text
ABSENT：未提供
UNKNOWN：已知无法确定
NOT_APPLICABLE：不适用
```

不得全部使用 null 表达。

---

# 7. 业务时间与版本解析

## 7.1 Business Time Policy

允许值：

```text
PRICE_AS_OF_QUOTED_AT
PRICE_AS_OF_LABEL_CREATED
PRICE_AS_OF_WAREHOUSE_SHIPPED
PRICE_AS_OF_CARRIER_ACCEPTED
PRICE_AS_OF_INVOICE_DATE
EXPLICIT_BUSINESS_TIME
```

## 7.2 双时间解析

版本必须同时按以下条件解析：

```text
valid_time contains business_time
system_time contains knowledge_time
ownership_scope matches
status visible to purpose
```

生产计算：

```text
status = ACTIVE in selected PricingRelease manifest
```

生产请求必须先冻结唯一 `pricing_release_id`。后续所有版本只能从该 manifest 解析，不能在执行中查询新的 current release。

SIMULATION 可选择 Draft/Test 版本，但必须显式指定 VersionRef。

## 7.3 区间语义

时间区间统一采用：

```text
[valid_from, valid_to)
```

即：

- `valid_from` 包含；
- `valid_to` 不包含；
- 无结束时间表示正无穷。

## 7.4 冲突

同一作用范围、同一业务时间命中多个互斥版本：

```text
VERSION_RESOLUTION_CONFLICT
```

不得按创建时间静默选择最新。

## 7.5 无版本

未命中有效版本：

```text
VERSION_NOT_FOUND
```

不得回退到过期版本，除非合同显式定义版本化 fallback。

---

# 8. Fact 选择

## 8.1 MeasurementFactSelector

只选择物理测量事实。

不得选择：

- Carrier billed weight；
- Carrier minimum billed weight；
- Invoice total；
- 系统计算的 billable weight。

## 8.2 CarrierAssessmentResolver

独立解析承运商认定：

```text
assessed_actual_weight
assessed_dimensional_weight
applied_minimum_weight
billed_weight
billed_zone
classifications
charge_codes
```

## 8.3 默认 Measurement 优先级

| Purpose | 默认优先级 |
|---|---|
| QUOTE | CUSTOMER_DECLARED → ORDER_IMPORTED |
| ESTIMATED_COST | WAREHOUSE_DWS → WAREHOUSE_MANUAL → CUSTOMER_DECLARED |
| ACTUAL_COST / SYSTEM_RECALCULATION | CARRIER_SCAN → WAREHOUSE_DWS → WAREHOUSE_MANUAL |
| CUSTOMER_BILLING | 合同策略 |
| DISPUTE_REVIEW | 明确指定事实集合 |

实际优先级必须来自版本化 `FactSelectionPolicy`。

## 8.4 FactSelectionResult

必须记录：

```text
candidate_fact_refs
selected_fact_ref
selection_policy_version
rejection_reasons
fallback_used
```

## 8.5 同优先级冲突

两个同优先级事实均有效且无 supersedes 关系：

```text
FACT_SELECTION_AMBIGUOUS
```

## 8.6 事实有效性

Fact 必须满足：

- tenant 一致；
- subject 一致；
- status 可用；
- measured_at/assessed_at 不晚于目的允许的截止时间；
- 未被正式 supersede；
- 单位有效；
- 数值非负。

---

# 9. 单位换算

## 9.1 原则

1. 所有计算先转换到 Profile 指定的 normalized unit；
2. 单位换算本身不得舍入；
3. 换算常量必须版本化或来自固定标准；
4. 中间结果保持 Decimal；
5. 只有 RoundingPolicy 指定阶段可以舍入。

## 9.2 固定换算常量

```text
1 IN = 2.54 CM
1 FT = 12 IN
1 LB = 0.45359237 KG
1 KG = 1000 G
1 M = 100 CM
```

反向换算使用同一精确常量计算，不保存独立近似值。

## 9.3 非法单位

不支持的单位：

```text
UNSUPPORTED_UNIT
```

dimension 不匹配：

```text
UNIT_DIMENSION_MISMATCH
```

## 9.4 温度、压力等

不属于当前 Rating 核心，不应进入通用单位转换器，除非后续正式扩展。

---

# 10. Geometry 语义

## 10.1 输入

每个 Package 的原始几何输入：

```text
raw_length
raw_width
raw_height
input_unit
shape_type
packaging_type
```

## 10.2 原始尺寸校验

每个参与三维计算的边必须：

```text
value > 0
unit dimension = LENGTH
```

NaN、Infinity、空字符串或无法解析的数字均为 `INVALID_GEOMETRY`。

## 10.3 处理顺序

标准顺序必须为：

```text
1. 验证原始尺寸
2. 选择几何事实
3. 单位精确换算
4. 执行 dimension rounding（若 stage = BEFORE_ORDERING）
5. 按规则排序
6. 执行 dimension rounding（若 stage = AFTER_ORDERING）
7. 计算 volume
8. 计算 girth
9. 计算 longest/second/shortest 等派生特征
10. 执行几何阈值判断
```

同一 Profile 只能选择一个 dimension rounding stage。

## 10.4 尺寸排序

`SORT_DESC`：

```text
longest >= second >= shortest
```

相等时不需要额外排序语义；输出值相同。

原始字段名 length/width/height 在排序后不再具有最长边语义。

## 10.5 Dimension Rounding

允许：

```text
NONE
CEILING_TO_INCREMENT
FLOOR_TO_INCREMENT
HALF_UP_TO_INCREMENT
HALF_EVEN_TO_INCREMENT
```

通用公式：

```text
q = value / increment
rounded = RoundMode(q) * increment
```

value 和 increment 必须同单位。

## 10.6 Volume

长方体外接体积：

```text
volume = longest * second * shortest
```

单位：

```text
normalized_length_unit ^ 3
```

不得在 volume 计算后自动转整数。

## 10.7 Girth

默认承运商周长表达：

```text
girth = longest + 2 * second + 2 * shortest
```

若承运商使用不同公式，必须由 GeometryProfile 明确定义，不得复用默认结果。

## 10.8 Irregular Shape

允许策略：

```text
BOUNDING_BOX
DECLARED_DIMENSIONS
CARRIER_ASSESSED_GEOMETRY
CUSTOM_FUNCTION
REJECT
```

`BOUNDING_BOX` 表示使用能包围物体的最小合规长方体测量结果；Rating Runtime 不负责从图片推导外接箱。

## 10.9 Soft Package

软包装是否按压平尺寸、自然放置尺寸或承运商扫描尺寸，必须由版本化策略确定。

未定义：

```text
SOFT_PACKAGE_MEASUREMENT_POLICY_MISSING
```

## 10.10 零尺寸

任何参与体积重的边长为 0：

```text
INVALID_GEOMETRY
```

除非产品明确允许二维文件类并配置专用算法。

## 10.11 边界比较

阈值条件必须保存操作符：

```text
GT
GTE
LT
LTE
EQ
BETWEEN_CLOSED_OPEN
```

例如：

```text
longest > 48 IN
```

不能只保存 `threshold = 48`。

---

# 11. 地址与地理分类

## 11.1 标准化顺序

```text
1. 国家代码标准化
2. 邮编字符标准化
3. 地址类型分类
4. PO Box 分类
5. Zone 解析
6. Remote Area 解析
7. Country/Region Group 解析
```

## 11.2 Postal Code

邮编不得默认转整数，必须按字符串处理，以保留：

- 前导零；
- 字母；
- 空格；
- 连字符。

## 11.3 Zone Resolver

输入至少包括：

```text
origin postal/warehouse
destination country
destination postal code
service product
business time
zone scheme version
```

输出：

```text
zone_code
matched_mapping_entry
zone_scheme_version
```

## 11.4 Remote Area

Remote classification 与 Zone 分开解析。

一个地址可以：

```text
zone = 6
remote_type = EXTENDED
```

## 11.5 地址来源冲突

客户申报为商业、承运商认定为住宅时：

- QUOTE 使用合同指定来源；
- ACTUAL_COST/CARRIER_ASSESSMENT 可使用承运商认定；
- 两者不得相互覆盖；
- 差异由 Reconciliation 处理。

---

# 12. 包裹特征判定

## 12.1 Feature Fact

Rating Runtime 根据标准化事实产生派生特征：

```text
AHS_WEIGHT
AHS_DIMENSION
AHS_PACKAGING
LARGE_PACKAGE
UNAUTHORIZED_PACKAGE
RESIDENTIAL
REMOTE_AREA
PO_BOX
SIGNATURE_REQUIRED
ADULT_SIGNATURE_REQUIRED
```

## 12.2 Feature Rule

每个特征规则必须包含：

```text
rule_id
condition_ast_or_typed_condition
operator
thresholds
units
priority
effective_version
evidence_fields
```

## 12.3 多条件 OR

例如：

```text
AHS_DIMENSION =
    longest > 48 IN
 OR second > 30 IN
 OR girth > 105 IN
 OR volume > 10368 IN3
```

Evaluation 必须记录命中的具体分支，不能只记录最终 true。

## 12.4 禁运与收费

`UNAUTHORIZED_PACKAGE` 可能导致：

```text
REJECT_SERVICE
APPLY_PENALTY_AND_WARN
ALLOW_WITH_OVERRIDE
```

行为必须由产品准入策略决定。

Eligibility 和 Charge 是两个阶段：

- 不可承运通常在 Eligibility 阶段失败；
- 允许承运但收罚款时，才生成费用。

---

# 13. Actual Weight

## 13.1 定义

Actual Weight 是选定 MeasurementFact 中的物理质量，转换到 WeightProfile.normalized_unit 后的值。

## 13.2 顺序

```text
selected physical measurement
→ exact unit conversion
→ optional actual_weight rounding
→ actual_weight_normalized
```

## 13.3 舍入

只有 WeightProfile 明确配置：

```text
actual_weight_rounding_stage
```

才可在 Billable 比较前舍入。

默认：

```text
NO_PRE_COMPARISON_ROUNDING
```

## 13.4 Carrier Assessment 模式

当 `calculation_basis = CARRIER_ASSESSMENT` 且 UsagePolicy 为 `BILLED_WEIGHT_AUTHORITATIVE`：

- `billed_weight` 直接成为 rated weight 输入；
- 系统仍可计算 geometry/actual/volumetric 作为解释与差异证据；
- 系统计算结果不得覆盖 billed_weight；
- 不再对 billed_weight 执行 WeightProfile 的 Billable Method；
- 是否对 billed_weight 再执行查价前舍入必须由承运商账单语义明确，默认不重复舍入。

## 13.5 缺失

Actual-only 算法缺少实际重量：

```text
ACTUAL_WEIGHT_REQUIRED
```

MAX 算法是否允许只用体积重，必须由：

```text
missing_candidate_policy
```

定义。

---

# 14. Volumetric Weight

## 14.1 DIVISOR

公式：

```text
volumetric_weight = normalized_volume / divisor
```

`divisor` 必须携带单位语义：

```text
IN3_PER_LB
CM3_PER_KG
```

示例：

```text
50 IN × 20 IN × 10 IN / 250 IN3_PER_LB = 40 LB
```

严禁把 CM 尺寸直接除以 250。

## 14.2 DENSITY

公式：

```text
volumetric_weight = normalized_volume * density
```

density 必须携带：

```text
MASS / VOLUME
```

## 14.3 FIXED_FACTOR

公式：

```text
base_volumetric_weight = volume / divisor
volumetric_weight = base_volumetric_weight * factor
```

必须同时指定 divisor 与 factor。

## 14.4 CUSTOM_FUNCTION

必须满足第 38 节要求。

## 14.5 体积重舍入

可配置：

```text
BEFORE_COMPARISON
AFTER_COMPARISON
NONE
```

若为 BEFORE_COMPARISON：

```text
volumetric_candidate = round(volumetric_raw)
```

否则候选值保持 raw。

## 14.6 非法配置

```text
divisor <= 0 → INVALID_DIVISOR
density < 0 → INVALID_DENSITY
factor < 0 → INVALID_FACTOR
```

---

# 15. Conditional Minimum Weight

## 15.1 定义

条件最低计费重量不是物理重量，而是特征触发后的计费候选值。

```text
ConditionalMinimum
├── feature_code
├── minimum_weight
├── priority
├── combination_policy
└── version
```

## 15.2 多个最低重量同时命中

默认：

```text
conditional_minimum_weight = MAX(all matched minimums)
```

若业务使用不同逻辑，必须配置：

```text
MAX
FIRST_MATCH
EXCLUSIVE
CUSTOM_FUNCTION
```

## 15.3 证据

必须记录：

- 命中特征；
- 每个最低重量；
- 最终选择；
- 规则版本。

---

# 16. Billable Weight

## 16.1 MAX

```text
billable_raw = MAX(valid candidates)
```

候选可包括：

- actual；
- volumetric；
- conditional minimum；
- product minimum；
- contract minimum。

无有效候选：

```text
NO_BILLABLE_WEIGHT_CANDIDATE
```

## 16.2 ACTUAL_ONLY

```text
billable_raw = actual_weight
```

## 16.3 VOLUMETRIC_ONLY

```text
billable_raw = volumetric_weight
```

## 16.4 THRESHOLD_MAX

规范定义：

```text
ratio = volumetric_weight / actual_weight
if ratio > threshold:
    billable_raw = MAX(actual_weight, volumetric_weight)
else:
    billable_raw = actual_weight
```

必须明确：

- 比较操作符，默认 `GT`；
- actual_weight = 0 时行为；
- threshold 单位为无量纲 Ratio。

若 actual = 0 且 volumetric > 0：

```text
ratio = +∞
```

并命中阈值。

## 16.5 PARTIAL_DIMENSIONAL

规范定义：

```text
ratio = volumetric / actual

if ratio <= threshold:
    billable_raw = actual
else:
    billable_raw =
        actual + (volumetric - actual) * excess_factor
```

约束：

```text
0 <= excess_factor <= 1
threshold >= 0
```

如果 volumetric <= actual，结果必须为 actual。

## 16.6 BLENDED

```text
billable_raw =
    actual * actual_weight_factor
  + volumetric * volumetric_weight_factor
```

默认约束：

```text
factors >= 0
actual_factor + volumetric_factor = 1
```

若业务允许和不为 1，必须显式：

```text
normalization_policy = NONE
```

## 16.7 Minimum Application Stage

最低重量可配置：

```text
AS_CANDIDATE_BEFORE_BILLABLE_METHOD
AFTER_BILLABLE_METHOD_FLOOR
```

两者语义不同，必须冻结。

后置 floor：

```text
billable_raw = MAX(method_result, minimum)
```

## 16.8 Billable Weight 舍入

最终舍入必须仅执行一次，除非 Profile 明确区分：

- candidate rounding；
- final billable rounding。

不得隐式重复向上取整。

---

# 17. Weight Rounding

## 17.1 Increment Rounding

对非负值：

```text
scaled = value / increment
rounded_scaled = mode(scaled)
result = rounded_scaled * increment
```

## 17.2 模式

```text
CEILING
FLOOR
HALF_UP
HALF_EVEN
UP
DOWN
```

对于非负重量：

- CEILING 等于向正无穷；
- FLOOR 等于向零；
- UP 等于远离零；
- DOWN 等于向零。

为了避免混淆，重量策略应优先使用 CEILING/FLOOR/HALF_UP/HALF_EVEN。

## 17.3 例子

```text
17.01 LB, increment 1, CEILING → 18 LB
17.00 LB, increment 1, CEILING → 17 LB
17.25 KG, increment 0.5, CEILING → 17.5 KG
17.25 KG, increment 0.5, HALF_UP → 17.5 KG
17.24 KG, increment 0.5, HALF_UP → 17.0 KG
```

## 17.4 精确倍数

如果 `value / increment` 已是整数，不得再增加一个 increment。

## 17.5 Money Scale Rounding

金额按 scale 舍入时：

```text
increment = 10 ^ (-scale)
result = round_to_increment(amount, increment, mode)
```

例如 scale=2，最小单位为 0.01。

所有正式 `raw_amount`、`rounded_amount` 必须非负。折扣、封顶和贷项使用 `ChargeEffect.DEDUCT`，不得通过负金额表达。

---

# 18. Rating Aggregation

## 18.1 稳定包裹顺序

所有包裹按以下顺序处理：

```text
master_flag DESC
package_sequence ASC
package_id ASC
```

缺少 sequence 时使用 package_id。

不得依赖输入 JSON 顺序。

## 18.2 PER_PACKAGE

流程：

```text
每个 Package 独立：
geometry
weight
base rate
package charges

然后汇总 shipment-level charges
```

基础费和包裹附加费都保留 package subject。

## 18.3 SHIPMENT_TOTAL

流程：

```text
1. 每件计算标准化实际重/体积重或配置的预聚合重量
2. 按 weight_aggregation 聚合
3. 对聚合重量执行 shipment-level billable rounding
4. 使用整票重量查基础价
5. 包裹级附加费仍按 Package 计算
```

`weight_aggregation` 必须明确：

```text
SUM_ACTUAL_WEIGHT
SUM_VOLUMETRIC_WEIGHT
SUM_PACKAGE_BILLABLE_WEIGHT
RECALCULATE_FROM_AGGREGATED_VOLUME
CUSTOM_FUNCTION
```

不得默认等同。

## 18.4 FIRST_CONTINUE_SHIPMENT

整票先确定计费重量，再使用首重续重价表。

包裹级附加费仍按各 Package 特征生成。

## 18.5 MASTER_PLUS_CHILD

必须指定：

```text
master_package_selector
master_base_rate_policy
child_charge_policy
missing_master_policy
multiple_master_policy
```

多个 master 默认报错。

## 18.6 HYBRID

必须逐费用代码声明 scope：

```text
BASE_FREIGHT → SHIPMENT
AHS_DIMENSION → PACKAGE
RESIDENTIAL → SHIPMENT or PACKAGE
FUEL → derived
```

不得只写 `HYBRID` 而不定义明细。

## 18.7 Allocation

允许：

```text
NO_ALLOCATION
EQUAL
PROPORTIONAL_BY_ACTUAL_WEIGHT
PROPORTIONAL_BY_BILLABLE_WEIGHT
PROPORTIONAL_BY_VALUE
CUSTOM_FUNCTION
```

## 18.8 分摊边界

分摊必须针对单一：

```text
source_charge_line
charge_effect
currency
cost_leg
```

分别执行。不得把 ADD 与 DEDUCT、不同币种或不同费用代码先净额合并后再分摊。

## 18.9 比例分摊

对总金额 `T`、权重 `w_i`：

```text
raw_i = T * w_i / SUM(w)
```

先计算 raw，不立即丢失精度。

## 18.10 余数分配

当分摊行金额需要保留固定小数位时，采用确定性最大余数法：

1. 按目标精度向下截断每个 raw_i；
2. 计算剩余最小货币单位数量；
3. 按小数余数从大到小分配；
4. 余数相同，按 package_id 字典序升序；
5. 分配后行金额之和必须等于总额。

不得把所有余数加给“最后一件”，除非合同明确指定。

## 18.11 零权重

比例分摊时所有权重为 0：

```text
ALLOCATION_ZERO_DENOMINATOR
```

可通过正式配置 fallback 到 EQUAL。

---

# 19. 价表家族

## 19.1 通用区间语义

数值区间统一使用：

```text
[min_inclusive, max_exclusive)
```

最后一个无上限区间：

```text
[min_inclusive, +∞)
```

不得使用两个闭区间造成边界重叠。

## 19.2 WEIGHT_ZONE

输入：

```text
rated_weight
zone
```

查找：

1. 找到包含 rated_weight 的唯一重量区间；
2. 匹配 exact zone；
3. 返回唯一 RateEntry。

无结果：

```text
RATE_NOT_FOUND
```

多结果：

```text
RATE_TABLE_OVERLAP
```

## 19.3 COUNTRY_WEIGHT

输入：

```text
destination_country
rated_weight
```

country 使用 ISO 3166 标准代码。

## 19.4 REGION_WEIGHT

先通过版本化 RegionGroup 解析目的地，再查重量。

一个国家同一业务时间属于多个互斥 Region：

```text
REGION_MEMBERSHIP_CONFLICT
```

## 19.5 FIRST_CONTINUE

配置：

```text
first_weight > 0
first_price >= 0
continue_increment > 0
continue_price >= 0
minimum_charge optional
```

算法：

```text
if rated_weight <= first_weight:
    raw = first_price
else:
    excess = rated_weight - first_weight
    units = CEILING(excess / continue_increment)
    raw = first_price + units * continue_price
```

注意：

- 即使 WeightProfile 已舍入，续重单位仍按该公式独立计算；
- `excess = 0` 时 units = 0；
- 不得使用普通四舍五入计算续重次数。

## 19.6 PER_UNIT

```text
raw = quantity * unit_rate
```

quantity 的来源和单位必须与 rate unit 一致。

## 19.7 TIERED_BANDED

又称整段阶梯或落档价。

输入 quantity 命中一个唯一 band，整个 quantity 使用该 band 声明的方法。

每个 band 必须显式声明：

```text
band_method: FIXED | PER_UNIT
band_rate
rate_unit（PER_UNIT时）
```

示例：

```text
[0,5) KG: PER_UNIT 20/KG
[5,10) KG: PER_UNIT 18/KG
```

7 KG：

```text
7 * 18
```

如果 band_method=FIXED，则直接返回该 band 固定金额，不乘整个 quantity。

## 19.8 TIERED_PROGRESSIVE

每一段只计算落在本段的数量。

7 KG：

```text
5 * 20 + 2 * 18
```

区间必须连续或显式允许 gap。

## 19.9 FLAT

返回固定 Money，不依赖数量。

## 19.10 INDEXED

```text
result = base_value * index_value
```

或由类型化模板指定。

必须冻结 index version。

## 19.11 CUSTOM_LOOKUP

必须定义：

- 输入字段 Schema；
- key normalization；
- 唯一性约束；
- 未命中行为；
- 结果类型。

---

# 20. Published Tariff 与折扣

## 20.1 基础流程

```text
published_rate
→ base_discount
→ minimum_net_charge
→ net_base_rate
```

标准算法：

```text
discounted = published_rate * (1 - discount_rate)
net_base_rate = MAX(discounted, minimum_net_charge)
```

## 20.2 折扣率

必须满足：

```text
0 <= discount_rate <= 1
```

大于 1 或小于 0：

```text
INVALID_DISCOUNT_RATE
```

## 20.3 最低净收费

最低净收费只约束其声明的费用组。

不得默认用整票总费用与 minimum 比较。

## 20.4 附加费折扣

附加费折扣独立于基础费折扣。

标准顺序：

```text
raw surcharge
→ surcharge discount
→ surcharge minimum/cap
```

## 20.5 中间舍入

公布价折扣、附加费折扣和最低净收费比较默认使用未舍入 Decimal。

只有合同明确要求承运商在折扣后先按指定 scale 舍入时，才能配置 `intermediate_rounding_ref`，并必须保存舍入轨迹。

## 20.6 净价表

若 RatePlan 已明确为 NET_RATE：

- 不再应用 Published Tariff discount；
- 除非合同明确再应用客户销售转换。

---

# 21. Charge 候选生成

## 21.1 生成顺序

```text
1. 根据产品和计划加载 ChargeDefinition
2. 按稳定顺序评估 eligibility
3. 生成 CandidateCharge
4. 解析 Scope Subject
5. 解析 Basis
6. 执行 Method
7. 应用 Composition
8. 应用 Treatment 派生费用
9. 舍入
10. 生成 EvaluationChargeLine
```

## 21.2 稳定排序

ChargeDefinition 默认排序：

```text
phase ASC
explicit_order ASC
charge_code ASC
definition_version ASC
```

不得依赖数据库返回顺序。

## 21.3 Eligibility

Eligibility 只决定候选费用是否适用，不直接计算金额。

必须记录：

```text
matched
evaluated_conditions
condition_values
rule_version
```

## 21.4 未命中

未命中的费用不生成金额为 0 的正式 ChargeLine。

可在解释轨迹中记录 skipped candidate。

---

# 22. Charge Scope、Basis 与 Method

## 22.1 Scope

```text
PACKAGE
SHIPMENT
ORDER
MANIFEST
CONTAINER
INVOICE
SETTLEMENT_PERIOD
```

Rating Runtime 当前主要支持：

```text
PACKAGE
SHIPMENT
ORDER
MANIFEST
CONTAINER
```

INVOICE 和 SETTLEMENT_PERIOD 通常由后续上下文处理。

## 22.2 Basis

允许值与领域模型一致：

```text
NONE
WEIGHT
VOLUME
PIECE_COUNT
DECLARED_VALUE
BASE_FREIGHT
SELECTED_CHARGES
TOTAL_BEFORE_DISCOUNT
TOTAL_AFTER_DISCOUNT
EXCESS_WEIGHT
EXCESS_DIMENSION
DAYS
LOOKUP_RESULT
CUSTOM_BASIS
```

## 22.3 Method

```text
FIXED
PER_UNIT
PERCENTAGE
LOOKUP
TIERED_BANDED
TIERED_PROGRESSIVE
MINIMUM_ADJUSTMENT
MAXIMUM_CAP
MAX_OF
FORMULA_TEMPLATE
CUSTOM_FUNCTION
```

## 22.4 ChargeEffect

默认来自 ChargeDefinition：

```text
ADD
DEDUCT
```

Evaluation 期间除显式销售转换、折扣、Cap 或 Credit 模板外，不得改变默认 effect。

---

# 23. Charge Basis 解析

## 23.1 NONE

不需要数量或金额基数。

## 23.2 WEIGHT

必须声明：

```text
ACTUAL_WEIGHT
VOLUMETRIC_WEIGHT
BILLABLE_WEIGHT
AGGREGATED_WEIGHT
```

以及 subject scope。

## 23.3 VOLUME

使用标准化体积，必须声明单位。

## 23.4 PIECE_COUNT

使用对应 Scope 内 package count 或指定 item count。

## 23.5 DECLARED_VALUE

使用快照中的申报价值，并冻结币种和汇率处理规则。

## 23.6 BASE_FREIGHT

只包含 canonical charge category 为 BASE_FREIGHT 的已生成行。

## 23.7 SELECTED_CHARGES

必须显式定义：

- include charge codes/groups；
- exclude charge codes/groups；
- effect handling；
- scope filter；
- phase cutoff。

当前节点不得通过 include group 直接或间接包含自身。该约束在依赖图编译时检查。

## 23.8 TOTAL_BEFORE_DISCOUNT

使用折扣阶段之前、指定作用域内的净费用合计。

```text
ADD sum - DEDUCT sum
```

基数不得为负；负值视为 0 或报错必须由策略明确。

## 23.9 TOTAL_AFTER_DISCOUNT

使用折扣阶段之后、minimum/cap 前或后的时间点必须由 stage 明确。

## 23.10 EXCESS_WEIGHT

```text
excess = MAX(0, selected_weight - threshold)
```

threshold 与 selected_weight 同单位。

## 23.11 EXCESS_DIMENSION

必须声明具体维度：

```text
LONGEST
SECOND
SHORTEST
GIRTH
VOLUME
```

```text
excess = MAX(0, value - threshold)
```

## 23.12 LOOKUP_RESULT

引用前置 Lookup 节点结果。

## 23.13 CUSTOM_BASIS

只允许受控函数返回类型化 Quantity 或 Money。

---

# 24. Charge Method 算法

## 24.1 FIXED

返回固定 Money。

## 24.2 PER_UNIT

```text
raw_amount = basis_quantity * unit_rate
```

若收费单位要求向上计数：

```text
billable_units = CEILING(quantity / rate_increment)
raw_amount = billable_units * rate_per_increment
```

必须明确 `rate_increment`。

## 24.3 PERCENTAGE

```text
raw_amount = basis_money * rate
```

basis 与输出币种不同：

- 先按 CurrencyPolicy 转到计算币种；
- 再计算百分比；
- 保存 FX evidence。

## 24.4 LOOKUP

使用指定 RateTable 家族返回 Money 或 Rate。

## 24.5 TIERED_BANDED

按第 19.7 节。

## 24.6 TIERED_PROGRESSIVE

按第 19.8 节。

## 24.7 MINIMUM_ADJUSTMENT

```text
current = selected charge basis net amount
adjustment = MAX(0, minimum - current)
```

当 adjustment = 0 时默认不生成费用行，除非 audit policy 要求生成零行。

默认 ChargeEffect：

```text
ADD
```

## 24.8 MAXIMUM_CAP

```text
current = selected charge basis net amount
deduction = MAX(0, current - cap)
```

默认生成：

```text
ChargeEffect.DEDUCT
```

不得直接修改原费用行。

## 24.9 MAX_OF

计算多个候选 Money，选择最大值。

币种必须一致或先转到统一计算币种。

并列时按候选配置顺序，再按 candidate_id 排序，保证证据稳定。

## 24.10 FORMULA_TEMPLATE

只能使用注册的类型化模板，例如：

```text
COST_PLUS_FIXED
COST_PLUS_PERCENT
TARGET_MARGIN
PARTIAL_DIMENSIONAL
```

不得存储任意可执行脚本字符串。

## 24.11 CUSTOM_FUNCTION

按第 38 节。

---

# 25. Charge Composition

## 25.0 Composition Group Key

组合策略默认在以下完整 key 内执行：

```text
composition_group
scope
subject_ref
currency
price_role
cost_leg
```

除非 CompositionPlan 显式声明跨 subject 或跨 leg 组合。

不同币种候选不得直接执行 MAX、CAP 或 EXCLUSIVE 金额比较；必须先转换到明确的 comparison currency。

## 25.1 STACK

所有命中候选均保留。

## 25.2 MAX

同 composition group 中只保留净金额最大的候选。

比较时：

```text
net = ADD amount - DEDUCT amount
```

若候选方向不同，不建议放入同一 MAX group；若配置允许，必须定义比较规则。

## 25.3 FIRST_MATCH

按：

```text
priority DESC
specificity DESC
charge_code ASC
version ASC
```

选择第一条。

## 25.4 EXCLUSIVE

命中超过一条：

```text
CHARGE_EXCLUSIVITY_CONFLICT
```

## 25.5 REPLACE

替换必须显式声明：

```text
replaces_charge_code
replaces_group
replacement_scope
```

被替换费用不进入后续 basis。

## 25.6 CAP

CAP 不直接修改费用，生成 DEDUCT adjustment line。

## 25.7 Composition 顺序

标准顺序：

```text
REPLACE
→ EXCLUSIVE validation
→ FIRST_MATCH
→ MAX
→ STACK
→ CAP
```

若业务需要不同顺序，必须作为版本化 CompositionPlan 明确发布。

---

# 26. 费用依赖图

## 26.1 节点

节点类型：

```text
BASE_RATE_LOOKUP
CHARGE_CALCULATION
BASIS_AGGREGATION
INDEX_APPLICATION
COMMERCIAL_TRANSFORMATION
MINIMUM
CAP
CURRENCY_CONVERSION
ROUNDING
SUMMARY
```

## 26.2 DAG 约束

必须：

- 无环；
- 每个依赖存在；
- 依赖输出类型兼容；
- 不引用未来 phase；
- 不跨 PriceRole 隐式读取；
- 不引用未冻结外部数据。

## 26.3 拓扑排序

使用稳定 Kahn 算法：

1. 选择所有入度为 0 的节点；
2. 按 `phase, explicit_order, node_id` 排序；
3. 逐个执行；
4. 新可执行节点再次按同一规则排序。

## 26.4 循环

```text
CHARGE_DEPENDENCY_CYCLE
```

必须在发布编译阶段阻止激活。

## 26.5 缺失依赖

```text
CHARGE_DEPENDENCY_MISSING
```

不得将缺失金额视为 0，除非节点声明：

```text
missing_dependency_policy = ZERO
```

---

# 27. Fuel Policy

## 27.1 实际费率

标准模型：

```text
effective_fuel_rate =
    official_index_rate * discount_factor
```

约束：

```text
official_index_rate >= 0
discount_factor >= 0
```

若“8 折”表示 0.8，必须配置为 0.8，不允许配置 80。

## 27.2 基数

```text
fuel_basis =
    SUM(included ADD lines)
  - SUM(included DEDUCT lines)
```

exclude 优先于 include。

basis 小于 0：

```text
MAX(0, basis)
```

除非 FuelPolicy 明确报错。

## 27.3 计算

```text
fuel_raw = fuel_basis * effective_fuel_rate
```

## 27.4 舍入

燃油行使用 charge_component rounding，除非 FuelPolicy 有专用 rounding ref。

## 27.5 证据

必须保存：

- index series；
- index version；
- official rate；
- discount factor；
- effective rate；
- included line IDs；
- excluded line IDs；
- basis；
- raw；
- rounded。

## 27.6 多燃油策略

同一费用组同一作用域命中多个互斥 FuelPolicy：

```text
FUEL_POLICY_CONFLICT
```

---

# 28. BUY 与 SELL 计算

## 28.1 独立 Plan

BUY 和 SELL 分别解析：

```text
BuyPricingPlan
SellPricingPlan
```

不存在默认继承。

## 28.2 SELL 独立价卡

直接使用 SELL RatePlan 计算。

## 28.3 COST_PLUS_FIXED

```text
sell = selected_buy_cost + fixed_markup
```

selected_buy_cost 必须引用冻结的 BUY Evaluation 与费用范围。

## 28.4 COST_PLUS_PERCENT

若 markup_rate 表示加价率：

```text
sell = cost * (1 + markup_rate)
```

约束：

```text
markup_rate >= 0
```

折让或销售折扣必须通过独立 `ChargeEffect.DEDUCT` 费用规则表达，不得用负 markup_rate 隐式表达。

## 28.5 TARGET_MARGIN

margin 定义：

```text
margin = (sell - cost) / sell
```

因此：

```text
sell = cost / (1 - target_margin)
```

约束：

```text
0 <= target_margin < 1
```

不得错误使用：

```text
cost * (1 + target_margin)
```

## 28.6 转换范围

必须明确：

```text
ALL_BUY_CHARGES
SELECTED_BUY_CHARGES
BUY_BASE_ONLY
BUY_TOTAL_AFTER_DISCOUNT
```

## 28.7 BUY/SELL 无环

- BUY 不得引用 SELL；
- SELL 可以引用 BUY；
- SELL 不得间接引用自身；
- 引用必须冻结 BUY evaluation_id 和 line IDs。

## 28.8 最低毛利保护

可以生成附加 SELL adjustment：

```text
required_sell = cost / (1 - minimum_margin)
adjustment = MAX(0, required_sell - current_sell)
```

生成 ADD line，不修改原销售费用。

---

# 29. 多币种与汇率

## 29.1 Currency Policy

必须定义：

```text
source_currency
calculation_currency
output_currency
fx_source
fx_business_time_policy
fx_version
conversion_stage
rounding_policy
```

## 29.2 转换公式

若报价为：

```text
1 source_currency = fx_rate target_currency
```

则：

```text
target_amount = source_amount * fx_rate
```

必须在 FX evidence 中保存报价方向。

## 29.3 转换与比较币种

凡涉及多费用金额比较或聚合的节点，必须声明 `comparison_currency`。

若输入行币种不同：

1. 按各自行的 FX evidence 转为 comparison currency；
2. 执行 MAX、minimum、cap、percentage 或汇总；
3. 输出币种按节点策略确定；
4. 不得使用未转换金额直接比较。

## 29.4 转换阶段

允许：

```text
PER_LINE_BEFORE_COMPOSITION
PER_LINE_AFTER_CHARGE_CALCULATION
AFTER_SCOPE_SUMMARY
FINAL_TOTAL_ONLY
```

不同阶段可能导致舍入差异，必须冻结。

## 29.5 交叉汇率

若通过中间币种转换，必须保存完整路径：

```text
USD → CNY → EUR
```

不得只保存最终 rate。

## 29.6 缺少汇率

```text
FX_RATE_NOT_FOUND
```

不得使用最近汇率，除非 CurrencyPolicy 明确版本化 fallback window。

---

# 30. 多段费用

## 30.1 Cost Leg

```text
FIRST_MILE
ORIGIN_HANDLING
EXPORT_CUSTOMS
LINEHAUL
IMPORT_CUSTOMS
DESTINATION_HANDLING
LAST_MILE
RETURN
```

## 30.2 Leg 独立性

每个 ChargeLine 必须有明确 cost_leg 或 `NOT_APPLICABLE`。

## 30.3 跨 Leg 基数

默认禁止一个 Leg 的燃油或折扣隐式包含另一个 Leg。

跨 Leg basis 必须显式列出。

## 30.4 汇总

总价：

```text
net_total =
    SUM(ADD lines)
  - SUM(DEDUCT lines)
```

若 net_total < 0：

- RatingEvaluation 可保留 SignedMoneyDelta 分析；
- 正式报价总额是否允许负数由商业策略决定；
- 默认返回 `NEGATIVE_EVALUATION_TOTAL`。

---

# 31. 宏观执行阶段

规范执行顺序：

```text
01 RequestValidation
02 PurposeAndTimeResolution
03 SnapshotValidation
04 ProductEligibility
05 ContractPolicyResolution
06 VersionManifestResolution
07 FactSelection
08 UnitNormalization
09 GeometryNormalization
10 AddressAndGeographyClassification
11 PackageFeatureEvaluation
12 WeightCalculation
13 Aggregation
14 BaseRateLookup
15 CandidateChargeGeneration
16 ChargeBasisResolution
17 ChargeMethodExecution
18 ChargeComposition
19 DerivedCharges
20 BuySellTransformation
21 MinimumAndCap
22 MultiLegAndShipmentSummary
23 CurrencyConversion
24 RoundingFinalization
25 EvaluationValidation
26 RatingEvaluationBuild
```

## 31.1 阶段约束

- 阶段不能任意拖拽；
- 阶段内可通过 DAG；
- Minimum/Cap 的具体节点可位于商业转换前后，但必须通过 phase 固定；
- Currency Conversion 阶段可由 CurrencyPolicy选择更早位置，但执行计划必须固定；
- Rounding 只在策略指定节点执行。

---

# 32. Rating Evaluation

## 32.1 Header

```text
evaluation_id
tenant_id
calculation_purpose
price_role
input_snapshot_id
business_time
knowledge_time
resolution_signature
compiled_plan_ref
created_at
status
```

## 32.2 Weight Result

每个 Package 保存：

```text
selected_measurement_ref
raw_geometry
normalized_geometry
actual_weight_raw
actual_weight_normalized
volumetric_weight_raw
volumetric_weight_candidate
conditional_minimums
billable_method
billable_raw
billable_rounded
weight_unit
rounding_trace
```

## 32.3 Charge Line

```text
evaluation_line_id
charge_code
charge_effect
scope
subject_ref
cost_leg
basis_type
basis_snapshot
method
rate_ref
lookup_entry_ref
raw_amount
rounded_money
currency
dependency_refs
composition_group
rule_ref
evidence_refs
```

## 32.4 Summary

```text
gross_additions
gross_deductions
net_total
currency
package_summaries
shipment_summary
leg_summaries
```

## 32.5 Version Manifest

必须包含所有影响结果的版本：

- Product；
- Contract；
- PricingPlan；
- RateTable；
- GeometryProfile；
- WeightProfile；
- AggregationProfile；
- FactSelectionPolicy；
- ZoneScheme；
- PostalAreaSet；
- ChargeDefinitions；
- FuelPolicy；
- RoundingPolicy；
- CurrencyPolicy；
- IndexSeries；
- CustomFunctions；
- PricingRelease。

## 32.6 状态机

与领域模型一致：

```text
REQUESTED → EVALUATING → COMPLETED
                   └──→ FAILED
REQUESTED/EVALUATING → CANCELLED
```

不变量：

- COMPLETED 后整个 Evaluation 不可修改；
- FAILED 可以保存诊断，但不得保存部分正式总额；
- CANCELLED 仅允许在正式结果形成前；
- 同一幂等请求成功完成后不得再创建另一个 COMPLETED Evaluation。

---

# 33. 解释、证据与执行轨迹

## 33.1 Trace Node

每一步至少保存：

```text
node_id
node_type
input_refs
input_values
policy_ref
operation
raw_output
rounded_output
executed_at
duration
status
```

## 33.2 解释要求

系统必须能够回答：

1. 为什么选择这套合同和价卡；
2. 为什么选择这个 MeasurementFact；
3. 长宽高如何排序和取整；
4. 体积重怎么算；
5. 计费重为什么取这个值；
6. 为什么命中某项附加费；
7. 燃油包含哪些费用；
8. 折扣和最低收费按什么顺序；
9. 汇率从哪里来；
10. 最终金额在哪一步舍入。

## 33.3 敏感字段

SELL 用户不一定有权查看 BUY line。

解释输出必须经过 FieldAccessPolicy 投影，不能修改底层 Evaluation。

## 33.4 Evidence Hash

关键输入和编译计划可以保存内容哈希，支持重放一致性验证。

---

# 34. 错误模型

## 34.1 分类

```text
INPUT_ERROR
POLICY_ERROR
VERSION_ERROR
FACT_ERROR
ELIGIBILITY_ERROR
RATE_ERROR
CALCULATION_ERROR
CURRENCY_ERROR
CUSTOM_FUNCTION_ERROR
SYSTEM_ERROR
```

## 34.2 失败原则

以下错误不得静默降级：

- 价表重叠；
- 规则冲突；
- 缺少必需事实；
- 单位不兼容；
- 循环依赖；
- 汇率缺失；
- 未定义边界操作符；
- 多个互斥 PricingPlan；
- 无法唯一选择事实。

## 34.3 可配置 fallback

仅可对以下情况配置正式 fallback：

- Fact source fallback；
- Zone mapping fallback；
- FX recent window；
- Allocation zero denominator；
- 未命中可选附加费；
- CustomFunction failure fallback。

fallback 必须版本化并记录是否被使用。

## 34.4 部分计算

任何阶段失败时：

- 已产生的中间 Trace 可以保留为诊断；
- 不得生成可被 Quote、Assessment 或 Financial 引用的部分总额；
- Evaluation 状态必须为 FAILED；
- 上层若需要“部分渠道成功”，应由批量/比较应用分别聚合各独立 Evaluation。

## 34.5 错误响应

至少包含：

```text
error_code
category
message
subject_ref
node_ref
policy_ref
version_refs
retryable
details
```

---

# 35. 幂等与重放

## 35.1 Request Hash

规范化请求哈希必须包含：

- tenant；
- purpose；
- input snapshot；
- requested price roles；
- explicit version refs；
- business time；
- simulation options。

不得包含非语义字段，如 trace request ID。

## 35.2 幂等

相同：

```text
tenant_id + idempotency_key + request_hash
```

返回同一 Evaluation。

相同 key 不同 hash：

```text
IDEMPOTENCY_CONFLICT
```

## 35.3 REPLAY

REPLAY 使用：

```text
original snapshot
original selected fact refs
original version manifest
original custom function versions
```

若依赖工件无法取得：

```text
REPLAY_ARTIFACT_MISSING
```

不得自动使用当前版本。

## 35.4 一致性哈希

可计算：

```text
evaluation_content_hash
```

用于验证重放结果完全一致。

---

# 36. Compiled Pricing Plan

## 36.1 编译输入

```text
PricingReleaseManifest
ResolvedContractPolicy
BasePricingPlan
SparseOverrides
TypedProfiles
RateTables
ChargeDefinitions
Dependencies
IndexRefs
CustomFunctionRefs
```

## 36.2 编译检查

必须包括：

- Schema；
- 类型；
- 单位；
- 区间重叠；
- 区间空档；
- 依赖无环；
- Scope/Basis/Method 兼容；
- PriceRole 引用无环；
- 版本有效；
- 权限和 tenant scope；
- CustomFunction signature；
- Golden Case。

## 36.3 Resolution Signature

由所有语义版本 ID 和内容哈希计算。

不得只使用客户 ID 与渠道 ID。

## 36.4 缓存

```text
Reusable Base Artifact
+
Sparse Override Artifact
+
Resolved Hot Artifact
```

缓存失效必须以 Version/Release 为依据，不按时间 TTL 决定语义。

## 36.5 原子发布

同一 Rating 请求只使用一个完整 PricingRelease manifest。

不得执行过程中一半使用旧版本、一半使用新版本。

---

# 37. Custom Function

## 37.1 适用范围

仅用于标准策略无法覆盖的少量算法。

## 37.2 必须声明

```text
function_id
semantic_version
input_schema
output_schema
unit_contract
deterministic
timeout
memory_limit
content_hash
test_suite
```

## 37.3 数值要求

CustomFunction 必须使用平台提供的 Decimal、Money、Quantity 和 Unit 类型，不得在函数内部转换为 binary float。

## 37.4 禁止

- 任意数据库访问；
- 任意网络访问；
- 当前时间读取；
- 随机数；
- 环境变量影响结果；
- 未版本化外部文件；
- 修改输入；
- 写业务状态。

## 37.5 失败

默认：

```text
CUSTOM_FUNCTION_FAILED
```

若配置 fallback，必须记录失败与 fallback 结果。

## 37.6 性能

CustomFunction 的执行时间计入 Rating 超时预算。

---

# 38. 性能与确定性

## 38.1 性能目标

参考目标：

```text
单包裹 P95 < 50 ms
单票多包裹 P95 < 100 ms
批量 1000 票可并行执行
```

## 38.2 性能不得牺牲语义

不得通过以下方式提速：

- 使用 binary float；
- 省略版本快照；
- 忽略冲突检查；
- 随机选择多结果；
- 依赖数据库无序返回；
- 跳过证据记录；
- 使用 stale plan 而不记录版本。

## 38.3 并行

无依赖节点可并行，但最终结果排序必须稳定。

## 38.4 超时

超时产生 FAILED Evaluation 诊断，不得返回部分总价作为成功结果。

---

# 39. Golden Cases

## 39.1 用例结构

每个 Golden Case 必须包含：

```text
case_id
purpose
input_snapshot
fact_set
version_manifest
expected_normalized_geometry
expected_weight_trace
expected_rate_entries
expected_charge_lines
expected_total
expected_errors
explanation_assertions
```

## 39.2 必须覆盖

- 单位换算；
- 尺寸临界值；
- 重量临界值；
- 体积重方法；
- 每种 Billable 方法；
- 每种 Aggregation；
- 每种价表家族；
- 每种 Charge Method；
- 燃油；
- 折扣；
- minimum/cap；
- BUY/SELL 转换；
- 多币种；
- 余数分摊；
- 缺失和冲突错误；
- REPLAY。

## 39.3 精确断言

不能只断言 total。

必须断言：

- 中间值；
- 选中规则；
- 选中价表行；
- 每个费用行；
- 舍入轨迹；
- 版本清单；
- 解释证据。

## 39.4 变更规则

任何计算语义改变：

- 必须新增或更新 Golden Case；
- 必须声明向后兼容性；
- 不得静默改变已发布版本结果。

---

# 40. 上游一致性追踪

| 本文主题 | V1.2 | 领域模型 V1.0.1 |
|---|---|---|
| CalculationPurpose | 附录 A.1 | 20.1 |
| Input Snapshot | 11 | 24.2 |
| MeasurementFact | 12 | 24.3 |
| CarrierAssessmentFact | 13 | 24.4 |
| GeometryProfile | 15 | 25.4 |
| WeightProfile | 16 | 25.5 |
| AggregationProfile | 17 | 25.6 |
| Scope/Basis/Method | 19 | 20.1、22.7 |
| Charge Dependency | 22 | 25.2、25.7 |
| Fuel | 23 | Pricing Catalog/Rating |
| Rounding | 24 | Typed Strategy |
| Rating Stages | 25 | 25.7 |
| Compiled Plan | 26 | 25.2 |
| RatingEvaluation | 27–28 | 25.3 |
| BUY/SELL | 7–10 | 34 |
| Replay | 30、45 | 39、44 |
| Charge vs Financial | 27 | 27–28 |

---

# 41. 自洽审查

## 41.1 事实一致性

- 物理 Measurement 只进入实际重、几何和系统重算；
- CarrierAssessment 单独进入承运商认定模式；
- 后到事实不覆盖历史 Evaluation。

## 41.2 数值一致性

- 所有数值是 Decimal；
- 所有舍入点显式；
- 区间为左闭右开；
- 金额非负，方向由 Effect 表达。

## 41.3 价格一致性

- BUY 与 SELL 独立；
- SELL 引用 BUY 必须显式；
- TARGET_MARGIN 公式唯一；
- 净价和公布价折扣不重复应用。

## 41.4 费用一致性

- Scope、Basis、Method 分离；
- CAP 和 minimum 通过 adjustment line 表达；
- Fuel 通过明确 line basis；
- Composition 顺序固定。

## 41.5 聚合一致性

- Package 与 Shipment 计算粒度显式；
- Allocation 余数确定性；
- 输入顺序不影响结果。

## 41.6 时间一致性

- 双时间解析；
- PricingRelease 原子；
- REPLAY 不使用 current/latest。

## 41.7 结果一致性

- RatingEvaluation 只表示计算；
- 不产生应收应付；
- 所有证据和版本冻结。

---

# 附录 A：核心枚举

## A.1 CalculationPurpose

```text
QUOTE
ESTIMATED_COST
ACTUAL_COST
CUSTOMER_BILLING
SIMULATION
REPLAY
DISPUTE_REVIEW
```

## A.2 PriceRole

```text
CARRIER_TARIFF
BUY
INTERNAL
SELL
PUBLIC
```

## A.3 ChargeScope

```text
PACKAGE
SHIPMENT
ORDER
MANIFEST
CONTAINER
INVOICE
SETTLEMENT_PERIOD
```

## A.4 ChargeBasisType

```text
NONE
WEIGHT
VOLUME
PIECE_COUNT
DECLARED_VALUE
BASE_FREIGHT
SELECTED_CHARGES
TOTAL_BEFORE_DISCOUNT
TOTAL_AFTER_DISCOUNT
EXCESS_WEIGHT
EXCESS_DIMENSION
DAYS
LOOKUP_RESULT
CUSTOM_BASIS
```

## A.5 ChargeMethodType

```text
FIXED
PER_UNIT
PERCENTAGE
LOOKUP
TIERED_BANDED
TIERED_PROGRESSIVE
MINIMUM_ADJUSTMENT
MAXIMUM_CAP
MAX_OF
FORMULA_TEMPLATE
CUSTOM_FUNCTION
```

## A.6 RatingAggregationMode

```text
PER_PACKAGE
SHIPMENT_TOTAL
FIRST_CONTINUE_SHIPMENT
MASTER_PLUS_CHILD
HYBRID
```

## A.7 ChargeEffect

```text
ADD
DEDUCT
```

## A.8 RoundingMode

```text
CEILING
FLOOR
HALF_UP
HALF_EVEN
UP
DOWN
```

---

# 附录 B：核心算法伪代码

## B.1 Package Weight

```text
function calculate_package_weight(package, profiles, facts):
    measurement = select_measurement(facts, profiles.fact_selection)
    geometry = normalize_geometry(measurement, profiles.geometry)
    actual = normalize_actual_weight(measurement, profiles.weight)
    volumetric = calculate_volumetric(geometry, profiles.weight)
    features = evaluate_features(geometry, actual, package)
    minimums = resolve_conditional_minimums(features, profiles.weight)
    raw = apply_billable_method(actual, volumetric, minimums, profiles.weight)
    rounded = round_to_increment(raw, profiles.weight.final_rounding)
    return WeightResult(...)
```

## B.2 FIRST_CONTINUE

```text
function first_continue(weight, plan):
    if weight <= plan.first_weight:
        return plan.first_price
    excess = weight - plan.first_weight
    units = ceiling(excess / plan.continue_increment)
    return plan.first_price + units * plan.continue_price
```

## B.3 Fuel

```text
function fuel(lines, policy, index):
    basis_lines = select_lines(lines, policy.include, policy.exclude)
    basis = net_sum(basis_lines)
    basis = max(0, basis)
    rate = index.official_rate * policy.discount_factor
    raw = basis * rate
    return round(raw, policy.rounding)
```

## B.4 Allocation

```text
function allocate(total, weights, scale):
    if sum(weights) == 0:
        return apply_zero_policy()
    raws = [total * w / sum(weights)]
    floors = truncate_to_scale(raws)
    units = (total - sum(floors)) / minimum_currency_unit(scale)
    order = sort_by(decimal_remainder DESC, package_id ASC)
    distribute_one_unit(order, units)
    assert sum(result) == total
    return result
```

---

# 附录 C：端到端 UPS Ground 示例

## C.1 输入

```text
Package:
actual = 10 LB
dimensions = 50 × 20 × 10 IN
address = RESIDENTIAL
zone = 6
purpose = QUOTE
```

策略：

```text
Geometry: SORT_DESC, no dimension rounding
Volumetric divisor: 250 IN3/LB
Billable: MAX
AHS Dimension: longest > 48 IN
AHS minimum: 40 LB
Final weight rounding: CEILING 1 LB
Aggregation: PER_PACKAGE
Fuel: official 16% × 80% = 12.8%
```

## C.2 Geometry

```text
longest = 50
second = 20
shortest = 10
volume = 10000 IN3
girth = 50 + 40 + 20 = 110 IN
```

## C.3 Features

```text
longest > 48 → AHS_DIMENSION
girth > 105 → AHS_DIMENSION
```

保存两个命中证据，但特征只生成一次。

## C.4 Weight

```text
actual = 10 LB
volumetric = 10000 / 250 = 40 LB
conditional minimum = 40 LB
billable raw = MAX(10, 40, 40) = 40 LB
billable rounded = 40 LB
```

## C.5 Base Rate

```text
WEIGHT_ZONE(40 LB, Zone 6)
→ matched entry ID
→ base freight
```

## C.6 Charges

```text
BASE_FREIGHT
RESIDENTIAL_DELIVERY
AHS_DIMENSION
FUEL
```

Fuel basis 必须来自配置明确包含的行。

## C.7 Output

生成 SELL RatingEvaluation，保存：

- 所有输入和事实引用；
- 体积重和最低重量；
- 价表行；
- 每个费用规则；
- Fuel 基数；
- 舍入轨迹；
- 总额；
- VersionManifest。

不生成应收。

---

# 附录 D：首批边界案例目录

## D.1 重量

```text
0
0.01
0.99
1.00
1.01
49.99
50.00
50.01
110.00
150.00
150.01
```

## D.2 尺寸

```text
47.99 / 48.00 / 48.01 IN
95.99 / 96.00 / 96.01 IN
107.99 / 108.00 / 108.01 IN
girth 104.99 / 105 / 105.01
girth 129.99 / 130 / 130.01
volume 10367.99 / 10368 / 10368.01
volume 17279.99 / 17280 / 17280.01
```

## D.3 舍入

```text
17.00, 17.0001, 17.24, 17.25, 17.50, 17.75
increments: 0.1, 0.5, 1
modes: CEILING, HALF_UP, HALF_EVEN
```

## D.4 聚合

```text
1件
2件相同重量
3件余数分摊
全部权重为0
master缺失
多个master
```

## D.5 价表

```text
区间下界
区间上界前一最小单位
无上限区间
区间重叠
区间空档
Zone不存在
```

## D.6 费用

```text
住宅+偏远
超重+超尺寸
REPLACE
EXCLUSIVE冲突
MAX并列
minimum不补差
minimum补差
cap不触发
cap触发
```

## D.7 BUY/SELL

```text
独立SELL
cost plus fixed
cost plus percent
target margin 0
target margin 99%
无效 margin 100%
BUY/SELL循环引用
```

## D.8 回放

```text
原版本仍存在
当前版本已变化
原CustomFunction缺失
原ZoneScheme缺失
content hash不一致
```

---

# 附录 E：终审一致性要求

## E.1 与 V1.2 一致

- 固定宏观阶段未改变；
- Scope、Basis、Method 枚举完全一致；
- BUY 与 SELL 独立；
- 单票计费不消费周期 Settlement 状态；
- RatingEvaluation 不产生财务账。

## E.2 与领域模型 V1.0.1 一致

- CalculationPurpose 完全一致；
- Evaluation 状态机完全一致；
- MeasurementFact 与 CarrierAssessmentFact 分离；
- PricingRelease 是生产版本权威；
- 正式金额非负并使用 ChargeEffect；
- Replay 冻结原始 VersionManifest。

## E.3 实现符合性

实现只有在以下全部通过时才可声明符合本规范：

1. 全部 MUST/MUST NOT 规则有自动测试；
2. 首批 Golden Cases 全部通过；
3. Decimal 和舍入跨语言一致；
4. RateTable 边界测试通过；
5. DAG 稳定排序测试通过；
6. Allocation 余数测试通过；
7. BUY/SELL 无环测试通过；
8. REPLAY 内容哈希一致；
9. 无价、冲突和缺失事实均返回规范错误；
10. Pandoc/Markdown 结构检查不属于运行规范，但文档发布时必须通过。

---

# 结论

本文冻结了 Rating Runtime 的核心计算语义：

```text
事实选择
→ 单位与几何
→ 实重与体积重
→ 计费重
→ 多件聚合
→ 价表查找
→ 附加费
→ 费用依赖
→ 燃油与商业转换
→ 汇率与舍入
→ RatingEvaluation
```

任何实现、数据库 Schema、API 或配置工具都必须服从本文，而不能自行创造新的隐式计算顺序。
