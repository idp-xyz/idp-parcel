# 价卡导入模板与校验规范（parcel-pricing）

- **模板版本**：`PPT-1`
- **对应规范化版本**：`PPC-5`（[ADR-0014](../adr/0014-versioned-canonicalization-shape-for-content-digest.md)；即本构建领域常量 `canonicalizationVersion` 的值）
- **状态**：生效，2026-09-25，票 `.scratch/price-card-import/issues/01`
- **依据**：
  - [ADR-0101](../adr/0101-operator-facing-registration-payload-shape-is-product-defined.md) 决定二至四；
  - [ADR-0015](../adr/0015-closed-grammar-open-vocabulary-for-multi-carrier-rating.md) 封闭文法；
  - parcel-pricing [`CONTEXT.md`](../domain/parcel-pricing/CONTEXT.md)「定价方案与价表版本」生命周期与「金额保真度：不可默认成立」。

## 一、这份文档管什么

模板是产品契约（ADR-0101 决定二）：租户按模板填，租户自有格式到模板的转换属实施服务，不进产品。

本文只定四件事：
- 模板文件长什么样；
- 每一格怎么读、交给哪一个领域构造函数；
- 读不通时怎么逐格回报；
- 模板文件本身怎么生成。

**语义校验不在本文。** 结构、依赖、区间、单位、币种与规则语义的检查一律由领域构造门承担（`NewPricingPlanVersion`、`NewRateTableVersion` 及其下各构造函数）。ADR-0101 Context 写明「校验语义不缺，缺的是翻译与回显」：模板只要一段「表格的一行 → 构造函数的入参」的翻译，再加「构造门拒了哪一行、为什么」的回显，不另造第二套校验规则。本文凡说「交构造门」，就是把判断留在领域，模板这一层不预判。

## 二、版本

- 模板版本号形如 `PPT-<n>`，**只在表集合、列集合或列语义变化时进位**。
- 规范化版本进位而表与列不变时，模板版本不动，只在下表加一行。
- 每个构建只认一个模板版本，同 ADR-0014 对规范化版本的做法。别的版本整份不收，答复里点名本构建认的那一个。
  - 理由：同时认两个版本就要同时维护两套翻译，而旧模板里缺的列无从补——补任何值都是替租户填卡。

| 模板版本 | 规范化版本 | 自 |
|---|---|---|
| `PPT-1` | `PPC-5` | 2026-09-25 |

## 三、物理格式

**一张价卡的一个版本 = 一个 Office Open XML 工作簿（`.xlsx`），一张表一个工作表。**

为什么不用 CSV（本仓现场作业导入模板用的是 CSV，见 [ADR-0089](../adr/0089-frontline-transition-controlled-import-with-structural-sunset.md) 与 `cmd/parcel-frontline-import`）：

1. **源文件身份是一份文件。** 登记记的是一个文件名加一个 SHA-256（`SourceFileIdentity`）。一张价卡是方案头、价表、计重、附加费、条件树等多张表，一份 CSV 只装得下一张表；拆成多份 CSV 就没有「那一份源文件」可记。
2. **矩阵型、多表的工件，工作簿是原生容器。** 定价人员用的就是电子表格，承运商的价卡也多以工作簿形式到达。
3. **单表的现场汇总不受影响。** 那边一份文件一张表，CSV 正合适；两处各按自己的形状取格式，不强求一致。

**读取件**放在 `internal/platform/spreadsheet`，只用标准库（`archive/zip`、`encoding/xml`）。它读出工作表名、逐行逐格的文本与格类型，不做任何业务判断——翻译与领域构造都在 parcel-pricing 一侧（ADR-0101 Alternatives 否决把导入器放进 platform，指的是翻译那一半，读格不在此列）。

模板文件的写出也在这个包里（见第七节），只写本模板用得到的那一小部分：工作表、列键行、文本格样式与 `说明` 表，同样只用标准库。

不引第三方表格库，两条理由：
- 要的是一个严格子集，而且要**拒收**通用库会默默转换的格：公式格、带浮点尾数的数值格（见第五节）。
- 全仓直接依赖极少，为读表引进一整套写入、样式与公式引擎不划算。

**整份不收**的文件形态：
- `.xls`（二进制格式）；
- 加密的工作簿；
- 严格 OOXML 命名空间的工作簿（在 Excel 或 WPS 里另存为「Excel 工作簿（*.xlsx）」即可）；
- 缺工作簿必要部件的压缩包。

**技术上限**是防御性的，不是业务取值：
- 文件本身不超过 10 MiB；
- 解压后读入的部件合计不超过 64 MiB；
- 单张表不超过 100 000 行。

## 四、工作簿结构

**表集合封闭**，就是下面这些，各表的列在本节各小节定：

`card`、`tables`、`rates_weight_zone`、`rates_first_continue`、`rates_unit_price`、`weight_rounding`、`fixed_charges`、`surcharges`、`conditions`、`calculations`、`charge_dependencies`、`reference_series`、`exclusions`、`reference_catalogues`、`manifest_dependencies`。

每一张都要在，空表可以。另可有一张名为 `说明` 的表，解析时整张跳过。
- 其他表名报 `SHEET_UNKNOWN`，缺表报 `SHEET_MISSING`。
- 理由：模板文件是生成的，所有表本来都在；改名或删表几乎总是误操作，悄悄当作「没有这张表」会让一整类规则无声消失。

**第 1 行是列键，第 2 行起是数据行。**
- 列键用英文，多与登记快照 JSON 的字段名同源，便于对照；中文说明在 `说明` 表里。
- 列键集合封闭、顺序不限。缺列、未知列、重复列各报一格。
- 整行空白的数据行跳过：电子表格常留下带格式的空行，不该长出幻影规则。

**`card` 表是键值表**，只有 `field` 与 `value` 两列，每个字段一行。字段集合同样封闭：缺字段、未知字段、重复字段各报一格。

**行号**一律按电子表格里看到的写：列键行是第 1 行。

### 4.1 `card`（方案头与登记件）

| 字段 | 必填 | 交给 |
|---|---|---|
| `templateVersion` | 是 | 须等于 `PPT-1`，否则整份不收 |
| `planId`、`planVersion` | 是 | `NewVersionReferenceIdentity(ArtifactPricingPlan, …)`，即方案版本引用；读不出即整份不收 |
| `scope` | 是 | `NewPricingScopeID` |
| `direction` | 是 | `PricingDirection`：`BUY`、`SELL`、`INTERNAL` |
| `purpose` | 是 | `PricingPurpose`：`CUSTOMER_CHARGE`、`SUPPLIER_COST`、`INTERNAL_PRICE`（与方向的配对交构造门） |
| `aggregation` | 是 | `AggregationMode`：`PER_PACKAGE` 走 `NewPricingPlanVersion`，`PER_SHIPMENT`、`PER_MASTER_DOCUMENT` 走 `NewAggregatePricingPlanVersion`。必须写明，不以逐包为默认 |
| `baseChargeCode` | 是 | `NewChargeCode` |
| `periodStartsAt` | 是 | `NewEffectivePeriod` 的起点 |
| `periodEndsAt` | 否 | `NewEffectivePeriod` 的止点；空即不设止点 |
| `rateTableId` | 是 | 指 `tables` 表的一行，作方案的主价表 |
| `weightPolicyId`、`weightPolicyVersion` | 是 | `NewVersionReferenceIdentity(ArtifactWeightPolicy, …)` |
| `weightMethod` | 是 | `PricingWeightMethod`：`ACTUAL_ONLY`、`MAX` |
| `volumetricDivisor`、`volumetricLengthUnit` | 成对 | `NewVolumetricFactor` 的除数（`ParseDecimal`）与长度单位（`CM`、`IN`）；进位段见 `weight_rounding` 的 `VOLUMETRIC` 行。计重方法与体积重因子怎么搭配交构造门 |
| `amountRoundingMode`、`amountRoundingIncrement`、`amountRoundingPoints` | 三格同有同无 | `NewAmountRoundingPolicy`：模式（`NONE`、`CEILING`、`HALF_UP`）、进位单位（按主价表币种，`NewMoneyFromString`）、应用点列表（`PER_LINE`、`AFTER_CONVERSION`、`TOTAL`） |
| `directionAuthorizationId`、`directionAuthorizationVersion` | 是 | `NewVersionReferenceIdentity(ArtifactCommercialAuthorization, …)`，即 `NewPriceCardRegistration` 的方向授权引用 |

**不进模板的**：
- 租户、录入者、批准者：取自操作者信封，ADR-0101 决定三、五。
- 源文件身份：按上传字节算出。
- 版本清单与内容摘要：构造门算出。

### 4.2 `tables` 与三张价表行

主价表与附加费「查表」用的价表同形，所以都在 `tables` 里声明表头，行按价表族分放三张表，每行用 `tableId` 指回表头。

**`tables`**：

| 列 | 必填 | 交给 |
|---|---|---|
| `tableId`、`tableVersion` | 是 | `NewVersionReferenceIdentity(ArtifactRateTable, …)` |
| `family` | 是 | `RateTableFamily`：`WEIGHT_ZONE`、`FIRST_CONTINUE`、`UNIT_PRICE` |
| `currency` | 是 | `NewCurrency`；本表各行金额的币种 |
| `weightUnit` | 是 | `NewWeightUnit`（`G`、`KG`、`OZ`、`LB`）；本表各行重量的单位 |
| `periodStartsAt` | 是 | `NewEffectivePeriod` 的起点 |
| `periodEndsAt` | 否 | 止点；空即不设 |

**`rates_weight_zone`**（`WEIGHT_ZONE` 族，交 `NewRateTableVersion`）：

| 列 | 必填 | 交给 |
|---|---|---|
| `tableId` | 是 | 须指一张 `WEIGHT_ZONE` 族的表 |
| `entryId` | 是 | `NewRateEntryID` |
| `zone` | 是 | 分区名，开放词汇 |
| `minimum` | 是 | 区间下界，按表的重量单位 |
| `maximum` | 否 | 区间上界；空即不封顶，交 `NewOpenEndedRateEntry`，否则交 `NewRateEntry` |
| `amount` | 是 | 按表的币种 |

**`rates_first_continue`**（`FIRST_CONTINUE` 族，交 `NewFirstContinueRate` 与 `NewFirstContinueRateTable`）：列为 `tableId`、`entryId`、`zone`、`firstWeight`、`firstAmount`、`step`、`stepAmount`，全部必填。

**`rates_unit_price`**（`UNIT_PRICE` 族，交 `NewUnitPriceRate` 与 `NewUnitPriceRateTable`）：列为 `tableId`、`entryId`、`zone`、`amountPerUnit`，全部必填。

**行继承表头的单位与币种。** 构造门本来就要求一张表内单位、币种一致，逐行再写一遍只多一处写错的机会。

**每张声明了的表都必须被用到：** 要么是 `card.rateTableId`，要么被某个 `TABLE_LOOKUP` 计算节点指到。否则报 `REFERENCE_UNUSED`，不悄悄丢掉。

### 4.3 `weight_rounding`（计费重与体积重的进位段）

| 列 | 必填 | 交给 |
|---|---|---|
| `policy` | 是 | `CHARGEABLE`（计重策略的进位）或 `VOLUMETRIC`（体积重因子的进位） |
| `mode` | 是 | `RoundingMode`：`NONE`、`CEILING`、`HALF_UP` |
| `increment` | 是 | 进位单位 |
| `maximum` | 否 | 本段上界；空即最后一段、不封顶 |
| `unit` | 是 | `increment` 与 `maximum` 的重量单位 |

- 同一 `policy` 的各行按行序组成分段，交 `NewWeightRoundingSegment` 或 `NewOpenEndedWeightRoundingSegment`，再交 `NewSegmentedWeightRoundingPolicy`。单段进位就是一段不封顶的分段，领域里 `NewWeightRoundingPolicy` 也是这么立的。
- `CHARGEABLE` 必须有行。
- `VOLUMETRIC` 与 `card` 的体积重因子同有同无。
- 进位单位与主价表单位的一致性交构造门。

### 4.4 `fixed_charges`（固定费用）

| 列 | 必填 | 交给 |
|---|---|---|
| `ruleId`、`chargeCode`、`description`、`effect`、`amount`、`order` | 是 | `NewFixedChargeRule`：`effect` 为 `ADD` 或 `DEDUCT`；`amount` 按主价表币种；`order` 为整数 |
| `unit` | 否 | 空即按计价对象计；`PER_PIECE` 交 `FixedChargeRule.PerPiece`（只对非逐包方案成立，交构造门） |

### 4.5 `surcharges`（附加费）

| 列 | 必填 | 交给 |
|---|---|---|
| `ruleId`、`chargeCode`、`description`、`effect` | 是 | `NewSurchargeRule` |
| `conditionId` | 是 | 指 `conditions` 的一个根节点，即触发条件。领域要求每条规则都声明触发条件，空组合不当恒真也不当恒假，所以这一格没有「无条件」的写法 |
| `calculationId` | 是 | 指 `calculations` 的一个根节点，即计算方式 |
| `exclusivity` | 否 | 空即未声明互斥立场；`STANDALONE` 交 `SurchargeRule.Standalone`；`GROUPED` 交 `SurchargeRule.InExclusivityGroup` |
| `exclusivityGroup`、`priority` | `GROUPED` 时必填，否则必空 | 互斥组名与组内优先级（整数） |
| `unit` | 否 | 空即按计价对象计；`PER_PIECE` 交 `SurchargeRule.PerPiece` |
| `minimumWeightId`、`minimumWeightConditionId`、`minimumWeight` | 三格同有同无 | `NewConditionalMinimumWeight`（条件指 `conditions` 的根节点；重量按主价表单位），再交 `SurchargeRule.WithConditionalMinimumWeight` |

### 4.6 `conditions`（条件树）

一行一个节点。**没有 `parentId` 的节点是根**。凡是按 `nodeId` 指进来的（`surcharges` 的触发条件与条件最低计价重量的条件、`exclusions` 的条件），指到的都必须是根。

| 列 | 必填 | 交给 |
|---|---|---|
| `nodeId` | 是 | 本表内唯一 |
| `parentId` | 否 | 父节点的 `nodeId` |
| `kind` | 是 | `PREDICATE` 交 `NewTrigger`；`ALL_OF`、`ANY_OF` 以子节点为操作数，交 `NewAllOfTrigger`、`NewAnyOfTrigger` |
| `source` | `PREDICATE` 必填 | `FeatureSource`，决定下面几格怎么读 |
| `operator` | 见下 | `ComparisonOperator`：`GT`、`GE`、`LT`、`LE`、`EQ` |
| `threshold`、`unit` | 见下 | 阈值与单位 |
| `category` | 见下 | 期望的类别值 |

`source` 决定 `PREDICATE` 行填哪几格：

| `source` | 交给 | 填的格 |
|---|---|---|
| `LONGEST_SIDE`、`SECOND_LONGEST_SIDE`、`LENGTH_AND_GIRTH` | `NewLengthFeatureCondition` | `operator`、`threshold`、`unit`（`CM`、`IN`） |
| `VOLUME` | `NewVolumeFeatureCondition` | `operator`、`threshold`、`unit`（`CM`、`IN`） |
| `ACTUAL_WEIGHT`、`VOLUMETRIC_WEIGHT`、`CHARGEABLE_WEIGHT` | `NewWeightFeatureCondition` | `operator`、`threshold`、`unit`（`G`、`KG`、`OZ`、`LB`） |
| `ZONE`、`ADDRESS_TYPE`、`SERVICE_OPTION` | `NewCategoryFeatureCondition` | `category`；`operator` 空或 `EQ`（领域里类别判定恒为等于） |

树的纪律：
- `PREDICATE` 不得有子节点；`ALL_OF`、`ANY_OF` 至少一个子节点。
- 兄弟节点的先后即行序。
- 父节点必须在本表，不得成环。
- 每个根都必须被指到，否则报 `REFERENCE_UNUSED`。

### 4.7 `calculations`（计算树）

| 列 | 必填 | 交给 |
|---|---|---|
| `nodeId` | 是 | 本表内唯一 |
| `parentId` | 否 | 只有 `GREATER_OF` 能当父节点 |
| `method` | 是 | 见下表 |
| `amount`、`tableId`、`percentage`、`seriesKind`、`seriesFactor`、`basisDependencyId`、`seriesId`、`outOfWindow` | 见下 | 按 `method` 取用 |

| `method` | 交给 | 填的格 |
|---|---|---|
| `FIXED_AMOUNT` | `NewFixedAmountSurcharge` | `amount`（主价表币种） |
| `TABLE_LOOKUP` | `NewTableLookupSurcharge` | `tableId`（指 `tables`） |
| `PERCENT_OF_BASIS` | `NewPercentOfBasisSurcharge` 或 `NewSeriesRateSurcharge` | `basisDependencyId`（指 `charge_dependencies`），再二选一：`percentage`（百分数，`12.5` 即 12.5%），或 `seriesKind` 与 `seriesFactor`（费率取序列读数，再乘卡上写明的系数） |
| `GREATER_OF` | `NewGreaterOfSurcharge` | 不填值格；恰好两个子节点，行序即先后 |
| `SERIES_AMOUNT` | `NewSeriesAmountSurcharge` | `seriesId`、`outOfWindow`（`NOT_CHARGED`、`PENDING`） |

- 「按序列费率」不另立一种 `method`。领域里它仍是按基数百分比，只是费率的来源不同（见 `NewSeriesRateSurcharge` 的注释：费率的来源与算术是两个轴），模板照此表达。
- 取较大值的操作数不得自身又是取较大值，交构造门；树因此最多两层。
- 每个根都必须被 `surcharges` 指到。

### 4.8 `charge_dependencies`（费用依赖）

| 列 | 必填 | 交给 |
|---|---|---|
| `dependencyId`、`chargeCode` | 是 | 依赖标识与依附的费用代码 |
| `composition` | 是 | `ALL_CHARGES` 交 `NewAllChargesDependency`；`LISTED_CHARGES` 交 `NewListedChargeDependency` |
| `includes` | `LISTED_CHARGES` 必填，否则必空 | 费用代码列表 |
| `excludes` | 否 | 费用代码列表 |

### 4.9 其余四张

| 表 | 列 | 交给 |
|---|---|---|
| `reference_series` | `kind`（`FUEL_RATE`、`EXCHANGE_RATE`、`PUBLISHED_AMOUNT`）、`seriesId` | `NewReferenceSeriesBinding`。只绑序列标识不绑版本（ADR-0099） |
| `exclusions` | `exclusionId`、`clause`、`conditionId`（指 `conditions` 的根） | `NewExclusionRule`，再交 `PricingPlanStructures.WithExclusionRules` |
| `reference_catalogues` | `kind`（`ZONE`、`REMOTE_TIER`）、`catalogueId` | `NewReferenceCatalogueLink`，再交 `PricingPlanStructures.WithReferenceCatalogues` |
| `manifest_dependencies` | `kind`（`ArtifactKind` 的封闭取值）、`id`、`version` | `NewVersionReferenceIdentity`，作 `NewPricingPlanVersion` 末位的版本清单依赖 |

结构件的组装顺序：
1. `NewPricingPlanStructures`（附加费、费用依赖、参考序列）；
2. `WithExclusionRules`；
3. `WithAmountRounding`；
4. `WithReferenceCatalogues`。

**行序不进摘要。** 领域在构造时把价表行、各类规则、依赖、绑定、排除条款与目录链接排好序，所以这些表的行序不影响内容摘要。条件树与计算树里兄弟节点的先后除外，它们按行序进树。

## 五、格的读法

先说判据：**每一格按文本读；凡是电子表格可能替人改过的值，一律不收，并告诉人怎么改。** 理由是 CONTEXT 的「金额保真度：不可默认成立」：已经有一张卡因人工录入整卡降为不作生产金额来源，导入器不能再添一条悄悄改数的路。

| 格的类型 | 怎么办 |
|---|---|
| 文本（共享字符串、行内字符串） | 取其文本，去掉首尾空白（含全角空格），同现场作业导入模板 |
| 数值 | 取存储的字面量。只有它形如 `-?数字(.数字)?`、不带指数、有效数字不超过 15 位时才收，否则报 `CELL_NUMERIC_NOT_EXACT` |
| 公式（带公式的格，不论类型） | 报 `CELL_FORMULA`：公式的缓存值可能没重算，而且公式本身不是一个被声明的值。请「粘贴为值」 |
| 错误值（如 `#N/A`） | 报 `CELL_ERROR_VALUE` |
| 布尔、日期类型的格 | 报 `CELL_NOT_TEXT` |
| 空格 | 即缺席。必填格空报 `CELL_REQUIRED`；可缺的格空就是没有，不补默认值 |
| 多填的格 | 本行用不到的格填了值（例如 `FIXED_AMOUNT` 行填了 `percentage`），报 `CELL_NOT_APPLICABLE`，不悄悄忽略 |

**为什么数值格要卡 15 位有效数字。** 电子表格把数值存成二进制浮点，人输入的 `2.3` 可能存成 `2.2999999999999998`。15 位是电子表格自己的显示精度，超过的尾数只可能是浮点痕迹。短字面量（`55`、`12.5`、`0.5`）原样收。模板把所有金额、小数、重量列预设为文本格，正常填写不会碰到这一格；碰到了就把该列设为文本后重填。

**各类值的写法：**
- **金额、小数、重量**：`ParseDecimal` 的写法，十进制、可带正负号、不收科学计数法。单位与币种由列或表头给出，不写在格里。
- **时刻**：RFC 3339，必须带时区，如 `2026-01-01T00:00:00Z` 或 `2026-01-01T08:00:00+08:00`。
  - 日期类型的格与不带时区的文本都不收：电子表格的日期序列号没有时区，替它选一个时区就是替租户定生效时刻。
- **列表**（`amountRoundingPoints`、`includes`、`excludes`）：元素以英文分号 `;` 分隔，逐个去首尾空白。空元素、重复元素报 `CELL_INVALID`。元素都是封闭代码或费用代码，费用代码只由大写字母、数字与下划线组成，分号不会有歧义。
- **封闭代码**（方向、目的、方法、单位……）：原样大写书写，不做大小写折叠。写错报 `CELL_INVALID`，并列出可取值。

## 六、问题清单

读完一份文件交回的问题逐条带坐标：

| 字段 | 含义 |
|---|---|
| `sheet` | 表名；整份文件层面的问题为空 |
| `row` | 电子表格行号；表层面的问题为 0 |
| `column` | 列键；`card` 表写字段名；行层面的问题为空 |
| `code` | 问题码，封闭集合，见下 |
| `message` | 中文说明：哪一格、为什么、怎么改；构造门拒收时附领域错误原文 |

问题按模板的表序、行号、列序排序，先后稳定：同一份文件两次读出的清单逐条相同。

**整份不收**：不落草稿行，由 ADR-0101 决定三的录入口答未受理。

| 码 | 情形 |
|---|---|
| `FILE_TOO_LARGE` | 超过第三节的技术上限 |
| `FILE_NOT_WORKBOOK` | 不是可读的 `.xlsx`，含第三节列出的各种整份不收的形态 |
| `TEMPLATE_VERSION_UNSUPPORTED` | `card` 表缺，或 `templateVersion` 缺、不是本构建认的那一个 |
| `PLAN_IDENTITY_UNREADABLE` | `planId`、`planVersion` 读不出一个方案版本引用。草稿按方案身份一版一行，没有身份就没有行可落 |

这四格按表中先后逐项查，撞到第一格就只答这一格：前一格不成立时，后面几格没有意义。

**进 `草稿` 带问题**：方案身份读得出来，其余任何一格有问题。

| 码 | 情形 |
|---|---|
| `SHEET_MISSING`、`SHEET_UNKNOWN` | 缺表、多出未知表 |
| `COLUMN_MISSING`、`COLUMN_UNKNOWN`、`COLUMN_DUPLICATED` | 列键行的缺、多、重 |
| `FIELD_MISSING`、`FIELD_UNKNOWN`、`FIELD_DUPLICATED` | `card` 表字段的缺、多、重 |
| `CELL_REQUIRED`、`CELL_NOT_APPLICABLE` | 必填格空、不适用的格填了值 |
| `CELL_FORMULA`、`CELL_ERROR_VALUE`、`CELL_NOT_TEXT`、`CELL_NUMERIC_NOT_EXACT` | 第五节的格类型问题 |
| `CELL_INVALID` | 值立不住：封闭代码不认识、小数或时刻写法不对、列表不合格、值对象构造门拒收 |
| `ID_DUPLICATED` | 同一表内的节点、表头或规则标识重复 |
| `REFERENCE_UNRESOLVED`、`REFERENCE_UNUSED` | 指向不存在或指错了种类（如指到非根节点、指到族不合的表）；声明了却没人用 |
| `TREE_INVALID` | 父节点不存在、成环、叶子带子节点、组合没有子节点、取较大值不是恰好两个子节点 |
| `CONSTRUCTION_REJECTED` | 格都读得通，领域构造门拒收；坐标指向这次构造用到的行，方案级构造门拒收时指 `card` 表、行号为 0 |

- **问题收齐再答**，不撞第一格就停：表单与导入都要的是「哪几格不对」（ADR-0126 决定四）。
- 但**上游格有问题时，依赖它的构造门不再跑**，免得一个错格引出一串连带问题。例如某条价表行金额写错，这张表就不再交 `NewRateTableVersion`，方案也不再交 `NewPricingPlanVersion`。
- **没有任何问题时**，方案立得住、内容摘要已算出，即生命周期的 `已校验`。预览交回的摘要与录入、发布时用的是同一份（ADR-0101 决定四）。

**两个摘要各管一件事，不要混用：**
- 源文件的 SHA-256 是上传那一串字节的身份。同一张卡在电子表格里另存一次，它就变了。
- 方案的内容摘要是规范化后的规则内容。换了文件、规则没变，它不变。

## 七、模板文件

- **解析与模板同源。** 表集合、列键、必填性、`说明` 表里每列的中文解释，都来自实现里同一张列定义。模板文件由它生成，不手工维护，所以模板与解析器不会漂开。
- **生成与存放。** 由命令 `cmd/parcel-pricing-template` 写出，入库在管理台静态资源下，供「导入价卡」签下载（票 `.scratch/price-card-import/issues/05`）。一个测试用生成器重算一遍、与入库文件逐字节比对，不等即失败并提示重新生成。为此生成器的输出必须确定：压缩包内的时间戳固定、部件顺序固定。
- **文本格。** 生成的模板把所有金额、小数、重量与标识列预设为文本格，正常填写就不会碰到第五节的浮点问题。
- **不带示例值。** 模板里不预填任何业务值，同 ADR-0089「模板属机制半边，模板里的取值属实例半边」。样例单独成文件，只用合成 `SYN-` 值，见下节。

## 八、合成样例

演示种子 `scripts/demo-seeds/seedgen` 里的两张卡，用本模板原样表达，作为实现的样例与验收：

- `SYN-PLAN-CN-SG-COST-01` `v1`：采购成本卡。`BUY`、`SUPPLIER_COST`，`WEIGHT_ZONE` 族，一个分区三段区间，只按实际重计，`CEILING` 进位 0.5 KG，没有附加费。
- `SYN-PLAN-CN-SG-01` `v1`：客户售价卡。`SELL`、`CUSTOMER_CHARGE`，`FIRST_CONTINUE` 族，两个分区，`MAX` 计费重（体积重除数 5000、`CM`，体积重 `CEILING` 进位 0.1 KG），一条固定费用，绑燃油序列。

**验收判据**：把这两份样例交给导入器，立出的方案内容摘要必须与种子经领域构造立出的逐字节相等。两条构造路径互相独立，相等即证明翻译没有漏格、没有改值。

以采购成本卡为例，各表的数据行如下（列键行与空表从略；`card` 表只列有值的字段，其余字段行照样在、值留空）。

`card`：

| field | value |
|---|---|
| templateVersion | PPT-1 |
| planId | SYN-PLAN-CN-SG-COST-01 |
| planVersion | v1 |
| scope | SYN-SCOPE-01 |
| direction | BUY |
| purpose | SUPPLIER_COST |
| aggregation | PER_PACKAGE |
| baseChargeCode | BASE_FREIGHT |
| periodStartsAt | 2026-01-01T00:00:00Z |
| rateTableId | SYN-TABLE-CN-SG-COST-01 |
| weightPolicyId | SYN-WEIGHT-CN-SG-COST-01 |
| weightPolicyVersion | v1 |
| weightMethod | ACTUAL_ONLY |
| directionAuthorizationId | SYN-AUTH-COST-DIR-01 |
| directionAuthorizationVersion | v1 |

`tables`：

| tableId | tableVersion | family | currency | weightUnit | periodStartsAt | periodEndsAt |
|---|---|---|---|---|---|---|
| SYN-TABLE-CN-SG-COST-01 | v1 | WEIGHT_ZONE | CNY | KG | 2026-01-01T00:00:00Z | |

`rates_weight_zone`：

| tableId | entryId | zone | minimum | maximum | amount |
|---|---|---|---|---|---|
| SYN-TABLE-CN-SG-COST-01 | SYN-COST-Z1-A | Z1 | 0 | 1 | 30 |
| SYN-TABLE-CN-SG-COST-01 | SYN-COST-Z1-B | Z1 | 1 | 5 | 85 |
| SYN-TABLE-CN-SG-COST-01 | SYN-COST-Z1-C | Z1 | 5 | 30 | 260 |

`weight_rounding`：

| policy | mode | increment | maximum | unit |
|---|---|---|---|---|
| CHARGEABLE | CEILING | 0.5 | | KG |

样例文件与种子不一致时，以验收判据（摘要逐字节相等）判谁错。

## 九、`PPT-1` 不覆盖的

- **带指纹或摘要的版本引用。** 模板里的引用一律只带种类、标识、版本三元，因为登记链路今天不读引用上的摘要：登记用例对方向授权只记引用、不回查；价表与计重策略的引用嵌在方案里，内容已由方案的内容摘要覆盖。加这几列只会多出没人读的格。合成数据的引用本来也只带三元（ADR-0108）。日后哪个消费方要用引用上的摘要，随那张票进位 `PPT-2`。
- **CSV 编码**：理由见第三节。
- **租户自有格式的转换**：实施服务，不进产品（ADR-0101 决定二）。
- **原始文件本体的存放**：ADR-0008 外置证据库的连接器尚无，草稿与登记都只记文件名与 SHA-256。
