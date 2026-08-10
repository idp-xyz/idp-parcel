# 国际小包计费 Golden Cases

**文档版本：** V1.0.1  
**文档状态：** 终审基线 / 机器可执行  
**案例总数：** 136  
**配套文件：**
- `international-parcel-rating-golden-cases-v1.0.1.json`
- `international-parcel-rating-golden-cases-v1.0.1.schema.json`
- `validate_golden_cases_v1_0_1.py`
- 《国际小包计费 Golden Cases V1.0.1 终审报告》

**规范基线：**
- 《国际小包计费与结算平台最终解决方案 V1.2 审定版》
- 《国际小包计费领域模型 V1.0.1 终审版》
- 《Rating Runtime 计算语义规范 V1.0.1 终审版》

---

## 1. 目的

Golden Cases 是领域模型与计算语义规范的可执行证明。

本案例集不只验证最终总价，还验证：

1. 输入快照与事实选择；
2. 单位换算；
3. 尺寸排序、体积和周长；
4. 实重、体积重、条件最低重量和计费重量；
5. 舍入时点与增量；
6. 多件聚合与确定性分摊；
7. 价表家族和区间边界；
8. 附加费 Scope、Basis、Method、Effect；
9. 燃油、折扣、最低收费和封顶；
10. BUY/SELL 转换；
11. 汇率方向、转换阶段和金额精度；
12. 错误、冲突、幂等和历史重放。

任何实现只有在全部适用案例通过后，才可以声明符合 Rating Runtime V1.0.1。

---

## 2. 审查与生成方法

```text
读取 V1.2 总体方案
        ↓
读取领域模型 V1.0.1
        ↓
读取计算语义规范 V1.0.1
        ↓
提取枚举、算法、不变量和临界值
        ↓
读取源 UPS Ground 价卡
        ↓
区分源价卡事实与规范性合成数据
        ↓
生成 136 个机器案例
        ↓
参考计算器重新执行
        ↓
JSON Schema 校验
        ↓
覆盖率、唯一性和源数据比对
        ↓
终审与问题登记
```

案例分为三类：

| 类型 | 含义 |
|---|---|
| `SOURCE_RATE_CARD` | 金额或阈值直接来自用户提供的 UPS Ground 价卡 |
| `NORMATIVE_SYNTHETIC` | 为验证标准算法而构造，不宣称是承运商实际合同 |
| `UPSTREAM_CONSISTENCY` | 用于验证上游文档不变量、冲突和错误语义 |

---

## 3. 源价卡审查结果

### 3.1 可直接使用的数据

源文件：

```text
副本蜴国际-美线UPS-Ground-同行价卡-260729.xlsx
Sheet: UPS-Residential-C-LA
```

已提取：

- 1–150 LB；
- Zone 2–8；
- 共 1,050 个基础费率单元格；
- 住宅、偏远、AHS、签名、地址修正和运费复核等清晰费用；
- 工作簿中未发现公式，所有金额均为静态值。

在 Golden Cases 中：

- LB 列是基础价查找的权威重量键；
- KG 列仅作为价卡显示值，不反向推导 LB；
- 源产品注明住宅渠道仅一件代发，因此多件聚合案例均标记为规范性合成案例。

### 3.2 不得自动解释的歧义

| 编号 | 问题 | 处理 |
|---|---|---|
| SRC-DISC-001 | Unauthorized Package 同时出现 2025 USD 与 5000 USD | 阻止发布，ERR-012 预期失败 |
| SRC-DISC-002 | UPS 表中出现 FedEx Commercial Ground / Home Delivery 标签 | 不作为 UPS 权威费用代码 |
| SRC-DISC-003 | “燃油8折”与“燃油附加费率80%”表述容易混淆 | 明确按官方燃油率 × 0.8 建模 |
| SRC-DISC-004 | “AHS/OS 1.5折起”缺少基准和完整算法 | 不纳入可发布金额逻辑 |
| SRC-DISC-005 | KG 列按 0.45 KG/LB 展示，不是精确换算 | 查价使用 LB；单位转换使用 0.45359237 KG/LB |

---

## 4. 规范性冻结

### 4.1 Canonical RateTableFamily

案例集按领域模型冻结以下 11 类能力：

```text
FLAT
PER_UNIT
WEIGHT_ZONE
COUNTRY_WEIGHT
REGION_WEIGHT
FIRST_CONTINUE
TIERED_BANDED
TIERED_PROGRESSIVE
INDEXED
PUBLISHED_DISCOUNT
CUSTOM_LOOKUP
```

`PUBLISHED_DISCOUNT` 在计算语义规范中作为独立公布价折扣章节描述，但在领域模型中属于 RateTableFamily。两者执行语义一致，本案例集按领域枚举进行覆盖。

### 4.2 正式金额

```text
amount >= 0
direction = ADD | DEDUCT
```

不得通过负金额表达折扣或封顶。

### 4.3 区间

```text
[min_inclusive, max_exclusive)
```

精确上界进入下一段或未命中当前段。

### 4.4 结果层次

Golden Cases 只生成 `RatingEvaluation` 级结果，不生成：

- Quote 接受；
- ChargeAssessment；
- 应收应付；
- Financial Ledger。

---

## 5. 案例覆盖

| 类别 | 数量 |
|---|---:|
| 单位与几何 | 12 || 计费重量 | 18 || 临界值 | 20 || 多件聚合与分摊 | 12 || 价表家族 | 16 || 附加费 | 16 || 燃油、折扣、保底与封顶 | 10 || 采购价与销售价 | 8 || 多币种与汇率 | 6 || 错误与冲突 | 12 || 重放与幂等 | 6 || **合计** | **136** |
---

## 6. 机器案例结构

每个案例包含：

```json
{
  "case_id": "WGT-011",
  "category": "WEIGHT",
  "operation": "WEIGHT",
  "source_kind": "NORMATIVE_SYNTHETIC",
  "source_refs": ["Rating Runtime V1.0.1 §§13–17"],
  "inputs": {},
  "expected": {
    "status": "COMPLETED",
    "result": {}
  },
  "tags": []
}
```

`validate_golden_cases_v1_0_1.py` 会：

1. 用 JSON Schema 校验文件；
2. 重新执行每个案例；
3. 将实际结果与 `expected` 做结构化精确比较；
4. 检查案例 ID 唯一；
5. 检查案例总数；
6. 失败时输出完整 expected/actual 差异。

运行方式：

```bash
python validate_golden_cases_v1_0_1.py   international-parcel-rating-golden-cases-v1.0.1.json   --schema international-parcel-rating-golden-cases-v1.0.1.schema.json
```

预期输出：

```json
{
  "status": "PASSED",
  "case_count": 136,
  "completed_cases": 112,
  "expected_failure_cases": 24,
  "schema": "PASSED",
  "recalculation": "PASSED",
  "unique_ids": "PASSED"
}
```

---

## 7. 全部案例目录

### 7.1 单位与几何
| Case ID | 标题 | 操作 | 来源 | 预期 |
|---|---|---|---|---|
| `GEO-001` | 尺寸排序与标准周长 | `GEOMETRY` | 规范合成 | {"longest":"50","second":"20","shortest":"10","volume":"10000","girth":"110","unit":"IN"} |
| `GEO-002` | 厘米精确换算为英寸 | `GEOMETRY` | 规范合成 | {"longest":"50","second":"20","shortest":"10","volume":"10000","girth":"110","unit":"IN"} |
| `GEO-003` | 排序前按1英寸向上取整 | `GEOMETRY` | 规范合成 | {"longest":"48","second":"31","shortest":"11","volume":"16368","girth":"132","unit":"IN"} |
| `GEO-004` | 排序后按1英寸向上取整 | `GEOMETRY` | 规范合成 | {"longest":"48","second":"31","shortest":"10","volume":"14880","girth":"130","unit":"IN"} |
| `GEO-005` | 不进行尺寸舍入保留小数 | `GEOMETRY` | 规范合成 | {"longest":"47.25","second":"20.5","shortest":"10.125","volume":"9807.328125","girth":"108.5","unit":"IN"} |
| `GEO-006` | 尺寸按0.5英寸HALF_UP | `GEOMETRY` | 规范合成 | {"longest":"20","second":"10.5","shortest":"5.5","volume":"1155","girth":"52","unit":"IN"} |
| `GEO-007` | 标准girth公式验证 | `GEOMETRY` | 规范合成 | {"longest":"45","second":"20","shortest":"10","volume":"9000","girth":"105","unit":"IN"} |
| `GEO-008` | 体积阈值10368精确值 | `GEOMETRY` | 规范合成 | {"longest":"36","second":"24","shortest":"12","volume":"10368","girth":"108","unit":"IN"} |
| `GEO-009` | 体积阈值上方小数 | `GEOMETRY` | 规范合成 | {"longest":"36.0001","second":"24","shortest":"12","volume":"10368.0288","girth":"108.0001","unit":"IN"} |
| `GEO-010` | 软包装使用申报外形尺寸 | `GEOMETRY` | 规范合成 | {"longest":"18","second":"13","shortest":"5","volume":"1170","girth":"54","unit":"IN"} |
| `GEO-011` | 不规则件使用外接长方体 | `GEOMETRY` | 规范合成 | {"longest":"40","second":"15","shortest":"12","volume":"7200","girth":"94","unit":"IN"} |
| `GEO-012` | 121.92厘米精确等于48英寸 | `GEOMETRY` | 规范合成 | {"longest":"48","second":"20","shortest":"10","volume":"9600","girth":"108","unit":"IN"} |

### 7.2 计费重量
| Case ID | 标题 | 操作 | 来源 | 预期 |
|---|---|---|---|---|
| `WGT-001` | ACTUAL_ONLY与最终1LB向上取整 | `WEIGHT` | 规范合成 | billable_rounded=11 |
| `WGT-002` | VOLUMETRIC_ONLY 40LB | `WEIGHT` | 规范合成 | billable_rounded=40 |
| `WGT-003` | MAX由实际重胜出 | `WEIGHT` | 规范合成 | billable_rounded=45 |
| `WGT-004` | MAX由体积重胜出 | `WEIGHT` | 规范合成 | billable_rounded=40 |
| `WGT-005` | MAX由条件最低40LB胜出 | `WEIGHT` | 规范合成 | billable_rounded=40 |
| `WGT-006` | ACTUAL_ONLY缺少实际重失败 | `WEIGHT` | 规范合成 | FAILED / ACTUAL_WEIGHT_REQUIRED |
| `WGT-007` | THRESHOLD_MAX比例等于1.5且GT不命中 | `WEIGHT` | 规范合成 | billable_rounded=10 |
| `WGT-008` | THRESHOLD_MAX比例高于1.5命中 | `WEIGHT` | 规范合成 | billable_rounded=15.01 |
| `WGT-009` | THRESHOLD_MAX实际重为0时比例为无穷 | `WEIGHT` | 规范合成 | billable_rounded=5 |
| `WGT-010` | PARTIAL_DIMENSIONAL比例不超过阈值 | `WEIGHT` | 规范合成 | billable_rounded=10 |
| `WGT-011` | PARTIAL_DIMENSIONAL按70%计泡 | `WEIGHT` | 规范合成 | billable_rounded=17 |
| `WGT-012` | BLENDED 40%实重加60%体积重 | `WEIGHT` | 规范合成 | billable_rounded=16 |
| `WGT-013` | BLENDED系数和不为1失败 | `WEIGHT` | 规范合成 | FAILED / INVALID_BLEND_FACTORS |
| `WGT-014` | 候选舍入发生在比较之前 | `WEIGHT` | 规范合成 | billable_rounded=11 |
| `WGT-015` | CEILING精确倍数不增加重量 | `WEIGHT` | 规范合成 | billable_rounded=17 |
| `WGT-016` | CEILING 17.01升至18 | `WEIGHT` | 规范合成 | billable_rounded=18 |
| `WGT-017` | 多个条件最低重量默认取最大值 | `WEIGHT` | 规范合成 | billable_rounded=90 |
| `WGT-018` | 互斥最低重量冲突失败 | `STATIC_ERROR` | 规范合成 | FAILED / CONDITIONAL_MINIMUM_EXCLUSIVITY_CONFLICT |

### 7.3 临界值
| Case ID | 标题 | 操作 | 来源 | 预期 |
|---|---|---|---|---|
| `BND-001` | AHS重量50LB不命中 | `FEATURE` | 源价卡 | matched=False |
| `BND-002` | AHS重量50.01LB命中 | `FEATURE` | 源价卡 | matched=True |
| `BND-003` | 最长边48IN不命中 | `FEATURE` | 源价卡 | matched=False |
| `BND-004` | 最长边48.01IN命中 | `FEATURE` | 源价卡 | matched=True |
| `BND-005` | 次长边30IN不命中 | `FEATURE` | 源价卡 | matched=False |
| `BND-006` | 次长边30.01IN命中 | `FEATURE` | 源价卡 | matched=True |
| `BND-007` | girth 105IN不命中 | `FEATURE` | 源价卡 | matched=False |
| `BND-008` | girth 105.01IN命中 | `FEATURE` | 源价卡 | matched=True |
| `BND-009` | 体积10368IN3不命中 | `FEATURE` | 源价卡 | matched=False |
| `BND-010` | 体积10368.01IN3命中 | `FEATURE` | 源价卡 | matched=True |
| `BND-011` | Oversize最长边96IN不命中 | `FEATURE` | 源价卡 | matched=False |
| `BND-012` | Oversize最长边96.01IN命中 | `FEATURE` | 源价卡 | matched=True |
| `BND-013` | Oversize girth 130IN不命中 | `FEATURE` | 源价卡 | matched=False |
| `BND-014` | Oversize girth 130.01IN命中 | `FEATURE` | 源价卡 | matched=True |
| `BND-015` | Oversize体积17280IN3不命中 | `FEATURE` | 源价卡 | matched=False |
| `BND-016` | Oversize体积17280.01IN3命中 | `FEATURE` | 源价卡 | matched=True |
| `BND-017` | Oversize实际重110LB不命中 | `FEATURE` | 源价卡 | matched=False |
| `BND-018` | Oversize实际重110.01LB命中 | `FEATURE` | 源价卡 | matched=True |
| `BND-019` | Unauthorized实际重150LB不命中 | `FEATURE` | 源价卡 | matched=False |
| `BND-020` | Unauthorized实际重150.01LB命中 | `FEATURE` | 源价卡 | matched=True |

### 7.4 多件聚合与分摊
| Case ID | 标题 | 操作 | 来源 | 预期 |
|---|---|---|---|---|
| `AGG-001` | PER_PACKAGE两件独立计费后汇总 | `AGGREGATION` | 规范合成 | total=14.0616 |
| `AGG-002` | SHIPMENT_TOTAL汇总包裹计费重 | `AGGREGATION` | 规范合成 | aggregated_weight=26 |
| `AGG-003` | SHIPMENT_TOTAL汇总实际重 | `AGGREGATION` | 规范合成 | aggregated_weight=26 |
| `AGG-004` | 聚合体积后重新计算体积重 | `AGGREGATION` | 规范合成 | billable_weight=48 |
| `AGG-005` | 整票首重续重计费 | `AGGREGATION` | 规范合成 | amount=9.8 USD |
| `AGG-006` | MASTER_PLUS_CHILD单一主件 | `AGGREGATION` | 规范合成 | total=14 |
| `AGG-007` | MASTER_PLUS_CHILD缺少主件失败 | `AGGREGATION` | 规范合成 | FAILED / MASTER_PACKAGE_MISSING |
| `AGG-008` | MASTER_PLUS_CHILD多个主件失败 | `AGGREGATION` | 规范合成 | FAILED / MULTIPLE_MASTER_PACKAGES |
| `AGG-009` | 三件10美元等额分摊最大余数法 | `AGGREGATION` | 规范合成 | P1=3.34, P2=3.33, P3=3.33 |
| `AGG-010` | 按重量1比2比3比例分摊 | `AGGREGATION` | 规范合成 | P1=1.67, P2=3.33, P3=5 |
| `AGG-011` | 比例分摊分母为0失败 | `AGGREGATION` | 规范合成 | FAILED / ALLOCATION_ZERO_DENOMINATOR |
| `AGG-012` | HYBRID整票基础费加包裹附加费 | `AGGREGATION` | 规范合成 | total=29.882 |

### 7.5 价表家族
| Case ID | 标题 | 操作 | 来源 | 预期 |
|---|---|---|---|---|
| `RT-001` | UPS 1LB Zone2基础价 | `RATE_LOOKUP` | 源价卡 | amount=6.804 USD |
| `RT-002` | UPS 20LB Zone6基础价 | `RATE_LOOKUP` | 源价卡 | amount=7.2576 USD |
| `RT-003` | UPS 40LB Zone7基础价 | `RATE_LOOKUP` | 源价卡 | amount=14.2884 USD |
| `RT-004` | UPS 90LB Zone2基础价 | `RATE_LOOKUP` | 源价卡 | amount=13.716 USD |
| `RT-005` | UPS 150LB Zone8基础价 | `RATE_LOOKUP` | 源价卡 | amount=39.9924 USD |
| `RT-006` | COUNTRY_WEIGHT国家重量价 | `RATE_LOOKUP` | 规范合成 | amount=12.5 USD |
| `RT-007` | REGION_WEIGHT区域重量价 | `RATE_LOOKUP` | 规范合成 | amount=16 EUR |
| `RT-008` | FIRST_CONTINUE重量等于首重 | `RATE_LOOKUP` | 规范合成 | amount=5 USD |
| `RT-009` | FIRST_CONTINUE超过首重1.01KG | `RATE_LOOKUP` | 规范合成 | amount=8.6 USD |
| `RT-010` | PER_UNIT按0.5KG单位向上计数 | `RATE_LOOKUP` | 规范合成 | amount=6 USD |
| `RT-011` | TIERED_BANDED整段使用命中价 | `RATE_LOOKUP` | 规范合成 | amount=126 USD |
| `RT-012` | TIERED_PROGRESSIVE分段累计 | `RATE_LOOKUP` | 规范合成 | amount=136 USD |
| `RT-013` | FLAT固定金额 | `RATE_LOOKUP` | 规范合成 | amount=3.5 USD |
| `RT-014` | INDEXED指数价 | `RATE_LOOKUP` | 规范合成 | amount=15 USD |
| `RT-015` | PUBLISHED_DISCOUNT公布价折扣与最低净收费 | `RATE_LOOKUP` | 规范合成 | amount=9 USD |
| `RT-016` | CUSTOM_LOOKUP固定Schema查找 | `RATE_LOOKUP` | 规范合成 | amount=12.34 USD |

### 7.6 附加费
| Case ID | 标题 | 操作 | 来源 | 预期 |
|---|---|---|---|---|
| `CHG-001` | 住宅派送固定费 | `CHARGE_FIXED` | 源价卡 | amount=2.4624 USD |
| `CHG-002` | 普通偏远商业地址费 | `CHARGE_FIXED` | 源价卡 | amount=1.944 USD |
| `CHG-003` | 普通偏远住宅地址费 | `CHARGE_FIXED` | 源价卡 | amount=2.8296 USD |
| `CHG-004` | 扩展偏远商业地址费 | `CHARGE_FIXED` | 源价卡 | amount=2.4624 USD |
| `CHG-005` | 扩展偏远住宅地址费 | `CHARGE_FIXED` | 源价卡 | amount=3.52 USD |
| `CHG-006` | 极偏远地区费 | `CHARGE_FIXED` | 源价卡 | amount=7.128 USD |
| `CHG-007` | Alaska偏远费 | `CHARGE_FIXED` | 源价卡 | amount=49.95 USD |
| `CHG-008` | Hawaii偏远费 | `CHARGE_FIXED` | 源价卡 | amount=7.128 USD |
| `CHG-009` | AHS Weight Zone2 | `CHARGE_FIXED` | 源价卡 | amount=7.5384 USD |
| `CHG-010` | AHS Weight Zone7+ | `CHARGE_FIXED` | 源价卡 | amount=9.5148 USD |
| `CHG-011` | AHS Dimension Zone5–6 | `CHARGE_FIXED` | 源价卡 | amount=6.2424 USD |
| `CHG-012` | AHS Packaging Zone3–4 | `CHARGE_FIXED` | 源价卡 | amount=5.022 USD |
| `CHG-013` | 普通签名费 | `CHARGE_FIXED` | 源价卡 | amount=4.104 USD |
| `CHG-014` | 成人签名费 | `CHARGE_FIXED` | 源价卡 | amount=5.4 USD |
| `CHG-015` | 地址修正费 | `CHARGE_FIXED` | 源价卡 | amount=13.77 USD |
| `CHG-016` | 运费复核费取1.782美元或运费12%较大值 | `SHIPPING_CORRECTION` | 源价卡 | amount=2.4 USD |

### 7.7 燃油、折扣、保底与封顶
| Case ID | 标题 | 操作 | 来源 | 预期 |
|---|---|---|---|---|
| `FDC-001` | 燃油官方16%按8折计算并包含基础费和住宅费 | `FUEL` | 规范合成 | amount=1.6 USD |
| `FDC-002` | 燃油排除运费复核费 | `FUEL` | 规范合成 | amount=1.28 USD |
| `FDC-003` | 燃油基数扣除DEDUCT费用行 | `FUEL` | 规范合成 | amount=1.92 USD |
| `FDC-004` | 公布价20%折扣不触发最低净收费 | `RATE_LOOKUP` | 规范合成 | amount=16 USD |
| `FDC-005` | 公布价20%折扣触发最低净收费 | `RATE_LOOKUP` | 规范合成 | amount=9 USD |
| `FDC-006` | 附加费15%折扣 | `SURCHARGE_DISCOUNT` | 规范合成 | amount=8.5 USD |
| `FDC-007` | 最低收费无需补差 | `MINIMUM_ADJUSTMENT` | 规范合成 | amount=0 USD |
| `FDC-008` | 最低收费补差2美元 | `MINIMUM_ADJUSTMENT` | 规范合成 | amount=2 USD |
| `FDC-009` | 费用封顶未触发 | `MAXIMUM_CAP` | 规范合成 | amount=0 USD |
| `FDC-010` | 费用封顶生成3美元DEDUCT | `MAXIMUM_CAP` | 规范合成 | amount=3 USD |

### 7.8 采购价与销售价
| Case ID | 标题 | 操作 | 来源 | 预期 |
|---|---|---|---|---|
| `BS-001` | SELL使用独立销售价卡 | `SELL` | 规范合成 | sell=15 |
| `BS-002` | 成本加固定金额 | `SELL` | 规范合成 | sell=12.5 |
| `BS-003` | 成本加20%加价率 | `SELL` | 规范合成 | sell=12 |
| `BS-004` | 目标毛利率25% | `SELL` | 规范合成 | sell=13.33 |
| `BS-005` | 最低毛利20%保护生成补差 | `SELL` | 规范合成 | {"required_sell":"12.5","adjustment_effect":"ADD","adjustment":"0.5"} |
| `BS-006` | 销售折扣使用DEDUCT而非负加价率 | `SELL` | 规范合成 | net_sell=13 |
| `BS-007` | BUY与SELL循环引用失败 | `SELL` | 规范合成 | FAILED / BUY_SELL_REFERENCE_CYCLE |
| `BS-008` | 只对选定BUY费用加固定金额 | `SELL` | 规范合成 | sell=15 |

### 7.9 多币种与汇率
| Case ID | 标题 | 操作 | 来源 | 预期 |
|---|---|---|---|---|
| `FX-001` | USD直接转换CNY | `FX` | 规范合成 | amount=72 CNY |
| `FX-002` | CNY按USD报价方向反向转换 | `FX` | 规范合成 | amount=10 USD |
| `FX-003` | USD经CNY交叉转换EUR | `FX` | 规范合成 | amount=9.36 EUR |
| `FX-004` | 逐行转换与最终合计转换产生舍入差异 | `FX` | 规范合成 | {"per_line_total":"4.7","final_total":"4.69"} |
| `FX-005` | 不同币种MAX比较使用comparison currency | `FX` | 规范合成 | selected_candidate_id=A |
| `FX-006` | 缺少汇率失败 | `FX` | 规范合成 | FAILED / FX_RATE_NOT_FOUND |

### 7.10 错误与冲突
| Case ID | 标题 | 操作 | 来源 | 预期 |
|---|---|---|---|---|
| `ERR-001` | 不支持的长度单位 | `GEOMETRY` | 一致性审查 | FAILED / UNSUPPORTED_UNIT |
| `ERR-002` | 质量与长度单位维度不匹配 | `STATIC_ERROR` | 一致性审查 | FAILED / UNIT_DIMENSION_MISMATCH |
| `ERR-003` | 零尺寸非法几何 | `GEOMETRY` | 一致性审查 | FAILED / INVALID_GEOMETRY |
| `ERR-004` | 同优先级物理事实歧义 | `STATIC_ERROR` | 一致性审查 | FAILED / FACT_SELECTION_AMBIGUOUS |
| `ERR-005` | 同作用域双版本冲突 | `STATIC_ERROR` | 一致性审查 | FAILED / VERSION_RESOLUTION_CONFLICT |
| `ERR-006` | 费率区间重叠 | `RATE_LOOKUP` | 一致性审查 | FAILED / RATE_TABLE_OVERLAP |
| `ERR-007` | 费率未命中 | `RATE_LOOKUP` | 一致性审查 | FAILED / RATE_NOT_FOUND |
| `ERR-008` | 费用依赖形成循环 | `STATIC_ERROR` | 一致性审查 | FAILED / CHARGE_DEPENDENCY_CYCLE |
| `ERR-009` | 互斥费用同时命中 | `STATIC_ERROR` | 一致性审查 | FAILED / CHARGE_EXCLUSIVITY_CONFLICT |
| `ERR-010` | 分摊分母为0且无fallback | `AGGREGATION` | 一致性审查 | FAILED / ALLOCATION_ZERO_DENOMINATOR |
| `ERR-011` | 折扣率超过100% | `RATE_LOOKUP` | 一致性审查 | FAILED / INVALID_DISCOUNT_RATE |
| `ERR-012` | Unauthorized费用在源价卡中2025与5000冲突，禁止发布 | `STATIC_ERROR` | 一致性审查 | FAILED / SOURCE_RATE_CARD_CONFLICT_UNRESOLVED |

### 7.11 重放与幂等
| Case ID | 标题 | 操作 | 来源 | 预期 |
|---|---|---|---|---|
| `RPL-001` | 当前版本变化时REPLAY仍使用原版本 | `REPLAY` | 规范合成 | {"used_version":"RATE-V1","current_version_ignored":"RATE-V2","content_hash":"sha256:aaa"} |
| `RPL-002` | 原CustomFunction工件缺失 | `REPLAY` | 规范合成 | FAILED / REPLAY_ARTIFACT_MISSING |
| `RPL-003` | 原ZoneScheme工件缺失 | `REPLAY` | 规范合成 | FAILED / REPLAY_ARTIFACT_MISSING |
| `RPL-004` | 原工件内容哈希不一致 | `REPLAY` | 规范合成 | FAILED / REPLAY_CONTENT_HASH_MISMATCH |
| `RPL-005` | 相同幂等键与相同请求复用Evaluation | `REPLAY` | 规范合成 | evaluation_id=EVAL-001 |
| `RPL-006` | 相同幂等键但请求哈希不同失败 | `REPLAY` | 规范合成 | FAILED / IDEMPOTENCY_CONFLICT |

---

## 8. 代表性案例详解

### 8.1 GEO-002 — 厘米精确换算为英寸

**来源类型：** `NORMATIVE_SYNTHETIC`  
**来源：** Rating Runtime V1.0.1 §§9–10

输入：

```json
{
  "dimensions": [
    "127",
    "50.8",
    "25.4"
  ],
  "unit": "CM",
  "normalized_unit": "IN"
}
```

预期：

```json
{
  "status": "COMPLETED",
  "result": {
    "longest": "50",
    "second": "20",
    "shortest": "10",
    "volume": "10000",
    "girth": "110",
    "unit": "IN"
  }
}
```

### 8.2 WGT-011 — PARTIAL_DIMENSIONAL按70%计泡

**来源类型：** `NORMATIVE_SYNTHETIC`  
**来源：** Rating Runtime V1.0.1 §§13–17

输入：

```json
{
  "actual": "10",
  "volumetric": "20",
  "method": "PARTIAL_DIMENSIONAL",
  "threshold": "1.5",
  "excess_factor": "0.7"
}
```

预期：

```json
{
  "status": "COMPLETED",
  "result": {
    "billable_raw": "17",
    "billable_rounded": "17",
    "ratio": "2"
  }
}
```

### 8.3 BND-008 — girth 105.01IN命中

**来源类型：** `SOURCE_RATE_CARD`  
**来源：** 副本蜴国际-美线UPS-Ground-同行价卡-260729.xlsx!R20:R40；Rating Runtime V1.0.1 §10.11

输入：

```json
{
  "value": "105.01",
  "operator": "GT",
  "threshold": "105",
  "feature": "AHS_GIRTH"
}
```

预期：

```json
{
  "status": "COMPLETED",
  "result": {
    "matched": true,
    "value": "105.01",
    "operator": "GT",
    "threshold": "105"
  }
}
```

### 8.4 AGG-009 — 三件10美元等额分摊最大余数法

**来源类型：** `NORMATIVE_SYNTHETIC`  
**来源：** Rating Runtime V1.0.1 §18

输入：

```json
{
  "mode": "ALLOCATION",
  "total": "10.00",
  "package_ids": [
    "P1",
    "P2",
    "P3"
  ],
  "method": "EQUAL",
  "scale": 2
}
```

预期：

```json
{
  "status": "COMPLETED",
  "result": {
    "allocations": [
      {
        "package_id": "P1",
        "amount": "3.34"
      },
      {
        "package_id": "P2",
        "amount": "3.33"
      },
      {
        "package_id": "P3",
        "amount": "3.33"
      }
    ],
    "sum": "10"
  }
}
```

### 8.5 RT-002 — UPS 20LB Zone6基础价

**来源类型：** `SOURCE_RATE_CARD`  
**来源：** 副本蜴国际-美线UPS-Ground-同行价卡-260729.xlsx!B4:J154

输入：

```json
{
  "family": "WEIGHT_ZONE",
  "weight": "20",
  "zone": 6
}
```

预期：

```json
{
  "status": "COMPLETED",
  "result": {
    "amount": "7.2576",
    "currency": "USD",
    "matched_key": {
      "weight_lb": "20",
      "zone": "6"
    }
  }
}
```

### 8.6 RT-015 — PUBLISHED_DISCOUNT公布价折扣与最低净收费

**来源类型：** `NORMATIVE_SYNTHETIC`  
**来源：** 国际小包计费领域模型 V1.0.1 §22.5；Rating Runtime V1.0.1 §§19–20

输入：

```json
{
  "family": "PUBLISHED_DISCOUNT",
  "published_rate": "10",
  "discount_rate": "0.2",
  "minimum_net_charge": "9",
  "currency": "USD"
}
```

预期：

```json
{
  "status": "COMPLETED",
  "result": {
    "published_rate": "10",
    "discounted": "8",
    "minimum_net_charge": "9",
    "amount": "9",
    "currency": "USD"
  }
}
```

### 8.7 CHG-016 — 运费复核费取1.782美元或运费12%较大值

**来源类型：** `SOURCE_RATE_CARD`  
**来源：** 副本蜴国际-美线UPS-Ground-同行价卡-260729.xlsx!L46:R46

输入：

```json
{
  "fixed": "1.782",
  "freight": "20",
  "rate": "0.12",
  "currency": "USD"
}
```

预期：

```json
{
  "status": "COMPLETED",
  "result": {
    "fixed_candidate": "1.782",
    "percentage_candidate": "2.4",
    "amount": "2.4",
    "currency": "USD"
  }
}
```

### 8.8 FDC-001 — 燃油官方16%按8折计算并包含基础费和住宅费

**来源类型：** `NORMATIVE_SYNTHETIC`  
**来源：** Rating Runtime V1.0.1 §§20,24,27

输入：

```json
{
  "official_rate": "0.16",
  "discount_factor": "0.8",
  "lines": [
    {
      "id": "BASE",
      "amount": "10",
      "effect": "ADD",
      "included": true
    },
    {
      "id": "RES",
      "amount": "2.4624",
      "effect": "ADD",
      "included": true
    }
  ],
  "scale": 2
}
```

预期：

```json
{
  "status": "COMPLETED",
  "result": {
    "basis": "12.4624",
    "effective_rate": "0.128",
    "raw_amount": "1.5951872",
    "amount": "1.6",
    "currency": "USD"
  }
}
```

### 8.9 BS-004 — 目标毛利率25%

**来源类型：** `NORMATIVE_SYNTHETIC`  
**来源：** Rating Runtime V1.0.1 §28；国际小包计费领域模型 V1.0.1 §34

输入：

```json
{
  "method": "TARGET_MARGIN",
  "cost": "10",
  "target_margin": "0.25",
  "scale": 2
}
```

预期：

```json
{
  "status": "COMPLETED",
  "result": {
    "sell_raw": "13.333333333333333333333333333333333333333333333333",
    "sell": "13.33"
  }
}
```

### 8.10 FX-004 — 逐行转换与最终合计转换产生舍入差异

**来源类型：** `NORMATIVE_SYNTHETIC`  
**来源：** Rating Runtime V1.0.1 §29

输入：

```json
{
  "mode": "ROUNDING_STAGE",
  "amounts": [
    "0.335",
    "0.335"
  ],
  "rate": "7",
  "scale": 2
}
```

预期：

```json
{
  "status": "COMPLETED",
  "result": {
    "per_line_total": "4.7",
    "final_total": "4.69"
  }
}
```

### 8.11 ERR-012 — Unauthorized费用在源价卡中2025与5000冲突，禁止发布

**来源类型：** `UPSTREAM_CONSISTENCY`  
**来源：** 副本蜴国际-美线UPS-Ground-同行价卡-260729.xlsx!L8:R8；副本蜴国际-美线UPS-Ground-同行价卡-260729.xlsx!L40:R40

输入：

```json
{
  "error_code": "SOURCE_RATE_CARD_CONFLICT_UNRESOLVED"
}
```

预期：

```json
{
  "status": "FAILED",
  "error_code": "SOURCE_RATE_CARD_CONFLICT_UNRESOLVED"
}
```

### 8.12 RPL-001 — 当前版本变化时REPLAY仍使用原版本

**来源类型：** `NORMATIVE_SYNTHETIC`  
**来源：** Rating Runtime V1.0.1 §35；国际小包计费领域模型 V1.0.1 §39

输入：

```json
{
  "mode": "ORIGINAL_VERSION",
  "original_version": "RATE-V1",
  "current_version": "RATE-V2",
  "original_hash": "sha256:aaa"
}
```

预期：

```json
{
  "status": "COMPLETED",
  "result": {
    "used_version": "RATE-V1",
    "current_version_ignored": "RATE-V2",
    "content_hash": "sha256:aaa"
  }
}
```

---

## 9. 通过标准

### 9.1 Suite 级

- 案例总数必须为 136；
- Case ID 必须唯一；
- 所有类别数量必须与 coverage 一致；
- JSON Schema 必须通过；
- 参考计算器必须 136/136 通过；
- 源价卡基础费率 fixture 必须与工作簿 1,050 个单元格一致；
- 所有 Canonical RateTableFamily 至少一个案例；
- 所有已冻结 CalculationPurpose、PriceRole、ChargeScope、ChargeEffect 与上游文档一致。

### 9.2 Case 级

成功案例必须精确匹配：

- 状态；
- 中间结果；
- 金额字符串；
- 币种；
- 选中候选；
- 舍入结果；
- 分摊结果。

失败案例必须精确匹配：

- `status = FAILED`；
- `error_code`；
- 不得包含正式 result。

### 9.3 金额比较

金额使用十进制字符串做精确比较，不使用二进制浮点误差容差。

对账业务未来可以定义容差，但 Golden Cases 的计算语义验证不使用容差。

---

## 10. 与上游文档的一致性结论

| 主题 | 结论 |
|---|---|
| 项目边界 | 独立国际小包计费平台，不依赖 EER |
| 结果对象 | 仅 RatingEvaluation |
| 事实 | MeasurementFact 与 CarrierAssessmentFact 分离 |
| 价格域 | BUY 与 SELL 独立 |
| 费用 | Scope、Basis、Method、Effect 分离 |
| 聚合 | 五类模式均覆盖 |
| 价表 | 11 类领域能力均覆盖 |
| 时间 | Replay 使用原版本 |
| 金额 | Decimal、非负金额和方向分离 |
| 错误 | 冲突和缺失不得静默降级 |

---

## 11. 后续使用

本案例集将作为以下工作的共同验收基线：

```text
Rating API Contract V1.0
Rating Runtime 技术设计 V1.0
PostgreSQL 数据模型
Go / Java / Python 参考实现
价卡导入校验
发布前回归测试
历史重放验证
```

API、数据库和实现代码不得修改 Golden Case 的预期结果来迁就实现。

需要改变计算结果时，必须：

1. 修改正式业务规则；
2. 创建新版本语义规范；
3. 说明兼容性；
4. 新建或升级 Golden Case；
5. 保留旧版本案例用于 Replay。

---

## 12. 最终结论

Golden Cases V1.0.1 已把总体方案、领域模型、计算语义和真实 UPS Ground 价卡连接成一套可执行证据。

其核心价值不是“有 136 个测试”，而是确保：

```text
相同事实
+ 相同业务时间
+ 相同 PricingRelease
+ 相同计算策略
= 完全相同的 RatingEvaluation
```
