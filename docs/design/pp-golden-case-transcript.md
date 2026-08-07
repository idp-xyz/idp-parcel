# 计价治理案例转录本

状态：转录已完成并逐例核验；证据层级与阻断状态一律不变。本文是转录说明与审阅入口，不是新的验收依据，也不定义任何规则。

转录产物：[`docs/reference/golden-cases/parcel-pricing-rating-golden-cases-v1.0.1.json`](../reference/golden-cases/parcel-pricing-rating-golden-cases-v1.0.1.json)

## 为什么要转录

外部参考设计交付了一套治理案例集，本仓已确认它不是权威来源，但[小包计价上下文](../domain/parcel-pricing/CONTEXT.md)的源完整性门禁对这些案例作了具体判断——哪些必须隔离、哪些可作 `S` 使用、在什么条件下解除。门禁引用它们，而它们此前只存在于仓库外的参考树里。参考树一旦删除，门禁那几句话就失去对象。

转录把案例本体搬进本仓，使删除参考树不再丢失任何被引用的内容。转录不改变这些案例的效力：它们此前是什么级别，转录后仍是什么级别。

## 转录方式与保真核验

案例对象与源价卡差异声明**逐字复制**，未作字段增删、改名或值改写。转录由程序完成而非手工誊抄，因为 136 个案例逐个手抄必然出错，而金样例一旦抄错就比没有更糟。

核验方式：把转录本与原文件的案例分别按 `case_id` 排序、规范化为紧凑 JSON 后逐例比对。结果为 136 例全部一致、0 处差异，四条 `SRC-DISC` 声明一致。任何人都可以在参考树删除前重跑这个比对。

转录本另加了一个 `transcript` 头，记录来源、权威价卡身份、证据层级、未转录内容及其理由。头是本仓添加的说明，不属于原案例集内容。

## 证据层级不变

| 分组 | 数量 | 层级 | 附加约束 |
|---|---|---|---|
| `SOURCE_RATE_CARD` | 41 | `S` | 阻断中。`SRC-DISC-001` 至 `004` 取得业务裁决前不得用于金额验证、生产价卡金额、正式 `P` 验收或客户账单依据 |
| `NORMATIVE_SYNTHETIC` | 83 | `S` | 可验证规则语义与重放确定性，不得升级为 `R` 或 `P` |
| `UPSTREAM_CONSISTENCY` | 12 | `S` | 同上 |

裁决完成后，41 例所依据的源身份仍只有断言强度，不因本次转录提升。权威表述以[源完整性门禁](../domain/parcel-pricing/CONTEXT.md)为准，本表只作索引。

## 源身份

案例集自述源文件为「副本蜴国际-美线UPS-Ground-同行价卡-260729.xlsx」，SHA-256 为 `22ec1f55…`。该副本已删除、全仓不存在，哈希**不可复核**，且不会再闭合。

权威源为去掉`副本`前缀的同名文件，实际 SHA-256 为 `9edaf27ef93004e00f73a65471897f2cf7064d5d4df05014934ef7ac5861d33d`，工作表 `UPS-Residential-C-LA`，取数范围 `B4:J154` 与 `L3:R46`。两者的等价关系由仓库所有者断言，不由哈希证明。

**该文件已不在仓库内。** 它是第三方物流企业的商业价卡，按「敏感实例外置」红线只应留在受限证据库；仓库保留的是上面这组哈希与坐标。需要取数时向证据库按该哈希索取，不要在仓库里找。定性以[源完整性门禁](../domain/parcel-pricing/CONTEXT.md)为准。

各案例的 `source_refs` 里仍写着那个已删除的副本文件名。这是逐字转录的结果，不表示该文件存在。

## `SOURCE_RATE_CARD` 41 例索引

| case_id | 类别 | 操作 | 标题 | 引用范围 |
|---|---|---|---|---|
| `BND-001` | BOUNDARY | FEATURE | AHS重量50LB不命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-002` | BOUNDARY | FEATURE | AHS重量50.01LB命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-003` | BOUNDARY | FEATURE | 最长边48IN不命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-004` | BOUNDARY | FEATURE | 最长边48.01IN命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-005` | BOUNDARY | FEATURE | 次长边30IN不命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-006` | BOUNDARY | FEATURE | 次长边30.01IN命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-007` | BOUNDARY | FEATURE | girth 105IN不命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-008` | BOUNDARY | FEATURE | girth 105.01IN命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-009` | BOUNDARY | FEATURE | 体积10368IN3不命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-010` | BOUNDARY | FEATURE | 体积10368.01IN3命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-011` | BOUNDARY | FEATURE | Oversize最长边96IN不命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-012` | BOUNDARY | FEATURE | Oversize最长边96.01IN命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-013` | BOUNDARY | FEATURE | Oversize girth 130IN不命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-014` | BOUNDARY | FEATURE | Oversize girth 130.01IN命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-015` | BOUNDARY | FEATURE | Oversize体积17280IN3不命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-016` | BOUNDARY | FEATURE | Oversize体积17280.01IN3命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-017` | BOUNDARY | FEATURE | Oversize实际重110LB不命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-018` | BOUNDARY | FEATURE | Oversize实际重110.01LB命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-019` | BOUNDARY | FEATURE | Unauthorized实际重150LB不命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `BND-020` | BOUNDARY | FEATURE | Unauthorized实际重150.01LB命中 | `R20:R40`、`Rating Runtime V1.0.1 §10.11` |
| `CHG-001` | CHARGE | CHARGE_FIXED | 住宅派送固定费 | `L11:R46` |
| `CHG-002` | CHARGE | CHARGE_FIXED | 普通偏远商业地址费 | `L11:R46` |
| `CHG-003` | CHARGE | CHARGE_FIXED | 普通偏远住宅地址费 | `L11:R46` |
| `CHG-004` | CHARGE | CHARGE_FIXED | 扩展偏远商业地址费 | `L11:R46` |
| `CHG-005` | CHARGE | CHARGE_FIXED | 扩展偏远住宅地址费 | `L11:R46` |
| `CHG-006` | CHARGE | CHARGE_FIXED | 极偏远地区费 | `L11:R46` |
| `CHG-007` | CHARGE | CHARGE_FIXED | Alaska偏远费 | `L11:R46` |
| `CHG-008` | CHARGE | CHARGE_FIXED | Hawaii偏远费 | `L11:R46` |
| `CHG-009` | CHARGE | CHARGE_FIXED | AHS Weight Zone2 | `L11:R46` |
| `CHG-010` | CHARGE | CHARGE_FIXED | AHS Weight Zone7+ | `L11:R46` |
| `CHG-011` | CHARGE | CHARGE_FIXED | AHS Dimension Zone5–6 | `L11:R46` |
| `CHG-012` | CHARGE | CHARGE_FIXED | AHS Packaging Zone3–4 | `L11:R46` |
| `CHG-013` | CHARGE | CHARGE_FIXED | 普通签名费 | `L11:R46` |
| `CHG-014` | CHARGE | CHARGE_FIXED | 成人签名费 | `L11:R46` |
| `CHG-015` | CHARGE | CHARGE_FIXED | 地址修正费 | `L11:R46` |
| `CHG-016` | CHARGE | SHIPPING_CORRECTION | 运费复核费取1.782美元或运费12%较大值 | `L46:R46` |
| `RT-001` | RATE_TABLE | RATE_LOOKUP | UPS 1LB Zone2基础价 | `B4:J154` |
| `RT-002` | RATE_TABLE | RATE_LOOKUP | UPS 20LB Zone6基础价 | `B4:J154` |
| `RT-003` | RATE_TABLE | RATE_LOOKUP | UPS 40LB Zone7基础价 | `B4:J154` |
| `RT-004` | RATE_TABLE | RATE_LOOKUP | UPS 90LB Zone2基础价 | `B4:J154` |
| `RT-005` | RATE_TABLE | RATE_LOOKUP | UPS 150LB Zone8基础价 | `B4:J154` |

## 这 41 例实际断言了什么

转录时逐例比对了输入与期望，发现「来源为源价卡」并不等于「验证了源价卡的金额」。按断言内容分四组：

- **`BND-001` 至 `BND-020`（20 例）**：期望里除布尔结果外的每个值都等于输入里的同名值。它们断言的是比较运算在阈值上下的命中与否，阈值本身是输入。不验证任何金额。
- **`CHG-001` 至 `CHG-015`（15 例）**：`inputs.amount` 与 `expected.result.amount` 是同一个值，期望里唯一非回显的信息是费用方向 `ADD` 与币种 `USD`。它们断言的是固定费用行的形态，不是价卡上那个数额是否正确。
- **`CHG-016`（1 例）**：由输入算出 `max(1.782, 20 × 0.12) = 2.4`，是一次真实计算。
- **`RT-001` 至 `RT-005`（5 例）**：由重量与分区查出金额，期望里的金额不在输入中。**这 5 例是 41 例中唯一必须依赖价卡内容才能成立的案例。**

结论只是对转录数据的读法，任何人可从 JSON 复算，它不是业务裁决，也不改变任何一例的阻断状态。但它对 `SRC-DISC` 裁决的排期有直接影响：裁决解除的是金额验证能力，而 41 例里真正做金额验证的只有 5 例。

## 与 `SRC-DISC` 的关系

案例集**没有**逐例声明所涉差异，`source_refs` 只给到粗范围，因此无法按案例归因。按范围重叠只能分成三组：

| 引用范围 | 案例 | 该范围是否含争议单元格 |
|---|---|---|
| `R20:R40` | `BND-001` 至 `BND-020` | 含 `SRC-DISC-001` 的 `L40:R40`、`SRC-DISC-002` 的 `L32:R39` |
| `L11:R46` | `CHG-001` 至 `CHG-015` | 同上 |
| `L46:R46` | `CHG-016` | 不含 |
| `B4:J154` | `RT-001` 至 `RT-005` | 不含（四条争议分别位于 L 至 R 列与 `F1:G1`） |

**「不含争议单元格」不等于可以放行。** 门禁对 41 例是整体阻断，上表只是说明按现有引用信息无法做更细的归因。这本身是给裁决方的一条信息：即便将来只裁决其中一两条差异，也无法据此逐例解除 35 例的阻断，因为它们的引用范围粗到无法区分。要做细粒度解除，得先让每例给出确切单元格。

## 另 95 例

同样已全部转录。

| 分组 | 类别 | 数量 |
|---|---|---|
| `NORMATIVE_SYNTHETIC` | WEIGHT | 18 |
| `NORMATIVE_SYNTHETIC` | UNIT_GEOMETRY | 12 |
| `NORMATIVE_SYNTHETIC` | AGGREGATION | 12 |
| `NORMATIVE_SYNTHETIC` | RATE_TABLE | 11 |
| `NORMATIVE_SYNTHETIC` | FUEL_DISCOUNT_CAP | 10 |
| `NORMATIVE_SYNTHETIC` | BUY_SELL | 8 |
| `NORMATIVE_SYNTHETIC` | CURRENCY | 6 |
| `NORMATIVE_SYNTHETIC` | REPLAY | 6 |
| `UPSTREAM_CONSISTENCY` | ERROR | 12 |

原计划只转录 41 例，改为全部转录的理由是：源完整性门禁明写这 95 例「可以验证规则语义和重放确定性」。既然本仓权威文档声明它们可用，只誊 41 例就删参考树，会让那句话指向不存在的东西。95 例合计约 40 KB，与 41 例同构、自洽、无外部依赖，转录成本远低于让门禁失准的代价。

## 未转录的内容

**原案例集的 `fixtures`**：一张 `WEIGHT_ZONE` 价表（`UPS_GROUND_RESIDENTIAL_C_LA_260729`，150 个整数重量档 × Zone 2 至 8）与 25 项附加费金额的抽取结果。它**不进 `docs/reference/`，改为归档**在[参考设计的源价卡抽取](../archive/reference-design-rate-card-extraction-v1.0.1.json)。

理由是两难各让一步。不放进 `reference/`：没有任何案例引用它，它是对源价卡的未经核实抽取，而权威价卡本身已在本仓；把一份可直接取用的未核实价表放在现行数据目录，会诱使有人拿它当价卡数据用，与「未确认参数不写死为生产默认」冲突。也不直接丢弃：那样删除参考树就会永久失去「当初那套参考设计是怎么抽的」这一追溯线索，而这是删目录时本来唯一会真正消失的内容。

归档件带明确抬头：仅追溯、未经核实、不得作为价卡数据或验收证据。将来需要价表夹具时，从权威价卡重新抽取并留下核实记录，不要复活这份抽取。

**原案例集的 `canonical_enums`**：不转录。本仓词汇以[小包计价上下文](../domain/parcel-pricing/CONTEXT.md)为准，复制外部取值集合会制造第二套口径。案例中实际出现的 `WEIGHT_ZONE`、`ADD`、`BUY`、`SELL` 与本仓词汇一致；`REPLAY` 只作为案例的 `category`/`operation`/`tags` 出现，不是本仓的计算目的取值。

## 删除参考树前还缺什么

本次转录只解决了案例语料这一项。剩余条件见[去参考设计权威依赖交接](./pp-de-reference-authority-agent-handoff.md)：活文档中其余引用尚未清理，`fixtures` 的去留未定，且删除需人类确认。

## 相关文档

- [小包计价上下文](../domain/parcel-pricing/CONTEXT.md)：源完整性门禁与证据层级的权威位置
- [`PP-S03-W01` Golden Case 源证据闭合工作单](./pp-s03-w01-golden-case-source-evidence-request.md)：四条 `SRC-DISC` 的单元格核实与裁决要求
- [去参考设计权威依赖 Agent 交接](./pp-de-reference-authority-agent-handoff.md)：删除参考树的完整前置条件
- [参考设计吸收覆盖对照](./pp-reference-design-absorption-coverage.md)：历史索引
