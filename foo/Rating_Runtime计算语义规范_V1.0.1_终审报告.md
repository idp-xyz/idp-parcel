# Rating Runtime 计算语义规范 V1.0.1 终审报告

**审查对象：**《Rating Runtime 计算语义规范 V1.0.1 终审版》  
**上游基线：**
- 《国际小包计费与结算平台最终解决方案 V1.2 审定版》
- 《国际小包计费领域模型 V1.0.1 终审版》

**结论：** 通过计算语义冻结审查，可进入 Golden Cases、API Contract 和 Runtime 技术设计阶段。

---

## 1. 审查范围

本次终审覆盖：

1. CalculationPurpose 与 PriceRole；
2. 事实选择与承运商认定；
3. 单位换算、几何和尺寸临界值；
4. 实重、体积重、最低重量和计费重；
5. 重量舍入；
6. 多件聚合与分摊；
7. 有限价表家族；
8. Published Tariff、折扣和最低净收费；
9. Scope、Basis、Method 与 ChargeEffect；
10. Composition 和费用依赖 DAG；
11. Fuel、BUY/SELL、目标毛利与最低毛利；
12. 多币种；
13. Evaluation 状态、幂等与 Replay；
14. CustomFunction 的确定性；
15. 与 V1.2 和领域模型的一致性；
16. Markdown 结构和枚举完整性。

---

## 2. V1.0 终审发现并已修正

### 2.1 不可承运结果不够明确

已明确产品准入失败：

```text
RatingEvaluation.status = FAILED
category = ELIGIBILITY_ERROR
不生成正式总额
```

上层可投影为 INELIGIBLE，但不改变领域状态机。

### 2.2 Carrier Assessment 模式不够精确

已新增：

```text
BILLED_WEIGHT_AUTHORITATIVE
ASSESSMENT_COMPONENTS_AUTHORITATIVE
CHARGE_LINES_AUTHORITATIVE
```

明确 billed weight 不等于物理实际重，也不重复执行系统 Billable Method。

### 2.3 Decimal 缺少溢出规则

已增加 precision/scale 超限错误，禁止截断或转 binary float。

### 2.4 原始尺寸校验缺少严格定义

已明确三维边必须大于 0，NaN、Infinity 和不可解析值均失败。

### 2.5 分摊可能混合不同费用方向和币种

已规定每个 source line、effect、currency、cost leg 分别分摊。

### 2.6 TIERED_BANDED 未区分固定价和单位价

已为每个 band 增加：

```text
band_method: FIXED | PER_UNIT
```

### 2.7 Published Tariff 中间舍入不明确

已规定默认不做中间舍入；只有合同明确时才允许版本化 intermediate rounding。

### 2.8 Composition 分组维度不足

已冻结 group key：

```text
group + scope + subject + currency + price_role + cost_leg
```

### 2.9 Selected Charges 可能自引用

已增加直接和间接自引用检查。

### 2.10 COST_PLUS_PERCENT 允许负加价率

已禁止。折扣必须使用 DEDUCT 费用行，避免方向双重表达。

### 2.11 多币种金额比较不明确

已增加 comparison currency 和转换顺序。

### 2.12 Evaluation 状态与领域模型不完全一致

已统一为：

```text
REQUESTED → EVALUATING → COMPLETED / FAILED
REQUESTED/EVALUATING → CANCELLED
```

### 2.13 失败时部分结果语义不明确

已明确可保留诊断 Trace，但不得生成可被后续业务引用的部分总额。

### 2.14 CustomFunction 可能使用浮点

已明确必须使用平台 Decimal、Money、Quantity 和 Unit。

---

## 3. 自动结构审查

| 检查项 | 结果 |
|---|---:|
| 核心枚举与领域模型差异 | 0 |
| 代码块闭合 | 通过 |
| 重复标题 | 0 |
| 旧 EER/Agent/LLM 依赖 | 0 |
| Pandoc GFM 解析 | 通过 |
| V1.2 追踪项 | 完整 |
| Evaluation 状态机差异 | 0 |

---

## 4. 关键语义结论

| 主题 | 结论 |
|---|---|
| 数值 | Decimal，正式金额非负 |
| 单位 | 精确换算，不隐式舍入 |
| 区间 | 左闭右开 |
| 物理事实 | 与承运商认定分离 |
| Billable Weight | 六类方法有唯一算法 |
| Aggregation | 五类模式与确定性分摊 |
| Rate Table | 九类价表有明确边界 |
| Charge | Scope/Basis/Method/Effect 分离 |
| Fuel | 基数和指数版本可解释 |
| BUY/SELL | 独立，显式引用且无环 |
| Margin | target margin 公式正确 |
| Currency | 转换阶段和 comparison currency 明确 |
| Replay | 使用原快照和版本 |
| Result | 只生成 RatingEvaluation |

---

## 5. 后续仍需验证

本规范冻结算法语义，但以下内容必须继续通过交付物证明：

1. 100+ Golden Cases；
2. UPS Ground 当前价卡的全部边界案例；
3. 首重续重和国际专线案例；
4. Java/Go/Python Decimal 跨实现一致性；
5. API Schema 和错误码；
6. Compiled Pricing Plan 实现；
7. PostgreSQL 约束；
8. 性能压测；
9. 发布原子切换；
10. Replay 内容哈希验证。

---

## 6. 最终结论

V1.0.1 已达到计算语义可冻结状态。

下一步应并行开展：

```text
Golden Cases V1.0
API Contract V1.0
Rating Runtime 技术设计 V1.0
```

其中 Golden Cases 优先级最高，因为它是验证本文每一条算法语义是否真正可执行的证据。
