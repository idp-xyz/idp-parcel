// Package pricecardtemplate 把一份价卡导入模板读成领域值，也写出空白模板。
//
// 契约在 docs/design/pp-price-card-import-template-and-validation-spec.md：本包是它的实现，
// 表集合、列键、必填性与每列的中文说明都只在本文件定义一次，读取与写出模板共用，模板文件与
// 解析器因此不会漂开（规范第七节）。语义校验不在本包：每一格交给哪个领域构造函数写在规范
// 第四节，构造门拒了就如实回报，这里不预判、不另立规则（ADR-0101 Context）。
package pricecardtemplate

// TemplateVersion 是本构建认的唯一模板版本（规范第二节）。表集合、列集合或列语义变化时进位，
// 可取值的增减也算列语义变化。
const TemplateVersion = "PPT-1"

const (
	sheetCard                 = "card"
	sheetTables               = "tables"
	sheetRatesWeightZone      = "rates_weight_zone"
	sheetRatesFirstContinue   = "rates_first_continue"
	sheetRatesUnitPrice       = "rates_unit_price"
	sheetWeightRounding       = "weight_rounding"
	sheetFixedCharges         = "fixed_charges"
	sheetSurcharges           = "surcharges"
	sheetConditions           = "conditions"
	sheetCalculations         = "calculations"
	sheetChargeDependencies   = "charge_dependencies"
	sheetReferenceSeries      = "reference_series"
	sheetExclusions           = "exclusions"
	sheetReferenceCatalogues  = "reference_catalogues"
	sheetManifestDependencies = "manifest_dependencies"
	// sheetReadme 是解析时整张跳过的说明表。
	sheetReadme = "说明"
)

// requirement 是一列在模板说明里的必填性。只有 requiredAlways 由读取统一执行（空即必填格为空）；
// 有条件的那几列各在翻译处按规范逐条判。
type requirement uint8

const (
	requirementInvalid requirement = iota
	requiredAlways
	requiredOptional
	requiredConditional
)

func (value requirement) String() string {
	switch value {
	case requiredAlways:
		return "是"
	case requiredOptional:
		return "否"
	case requiredConditional:
		return "见说明"
	default:
		return ""
	}
}

type column struct {
	key         string
	requirement requirement
	note        string
}

type sheetSpec struct {
	name    string
	note    string
	columns []column
}

func (spec sheetSpec) column(key string) (column, int, bool) {
	for index, candidate := range spec.columns {
		if candidate.key == key {
			return candidate, index, true
		}
	}
	return column{}, 0, false
}

// 各封闭代码的可取值，与领域代码逐字相同。
var (
	directionCodes      = []string{"BUY", "SELL", "INTERNAL"}
	purposeCodes        = []string{"CUSTOMER_CHARGE", "SUPPLIER_COST", "INTERNAL_PRICE"}
	aggregationCodes    = []string{"PER_PACKAGE", "PER_SHIPMENT", "PER_MASTER_DOCUMENT"}
	weightMethodCodes   = []string{"ACTUAL_ONLY", "MAX"}
	lengthUnitCodes     = []string{"CM", "IN"}
	weightUnitCodes     = []string{"G", "KG", "OZ", "LB"}
	roundingModeCodes   = []string{"NONE", "CEILING", "HALF_UP"}
	roundingPointCodes  = []string{"PER_LINE", "AFTER_CONVERSION", "TOTAL"}
	familyCodes         = []string{"WEIGHT_ZONE", "FIRST_CONTINUE", "UNIT_PRICE"}
	roundingPolicyCodes = []string{"CHARGEABLE", "VOLUMETRIC"}
	effectCodes         = []string{"ADD", "DEDUCT"}
	chargeUnitCodes     = []string{"PER_PIECE"}
	exclusivityCodes    = []string{"STANDALONE", "GROUPED"}
	triggerKindCodes    = []string{"PREDICATE", "ALL_OF", "ANY_OF"}
	featureSourceCodes  = []string{
		"LONGEST_SIDE", "SECOND_LONGEST_SIDE", "LENGTH_AND_GIRTH", "VOLUME",
		"ACTUAL_WEIGHT", "VOLUMETRIC_WEIGHT", "CHARGEABLE_WEIGHT",
		"ZONE", "ADDRESS_TYPE", "SERVICE_OPTION",
	}
	operatorCodes      = []string{"GT", "GE", "LT", "LE", "EQ"}
	methodCodes        = []string{"FIXED_AMOUNT", "TABLE_LOOKUP", "PERCENT_OF_BASIS", "GREATER_OF", "SERIES_AMOUNT"}
	seriesKindCodes    = []string{"FUEL_RATE", "EXCHANGE_RATE", "PUBLISHED_AMOUNT"}
	outOfWindowCodes   = []string{"NOT_CHARGED", "PENDING"}
	compositionCodes   = []string{"ALL_CHARGES", "LISTED_CHARGES"}
	catalogueKindCodes = []string{"ZONE", "REMOTE_TIER"}
	artifactKindCodes  = []string{
		"pricing-plan", "rate-table", "weight-policy", "reference-series", "reference-catalogue",
		"commercial-policy", "commercial-authorization", "numeric-profile",
	}
)

// cardFields 是 card 表的字段集合，顺序即模板里的行序。
var cardFields = []column{
	{"templateVersion", requiredAlways, "模板版本，须为 " + TemplateVersion},
	{"planId", requiredAlways, "定价方案标识"},
	{"planVersion", requiredAlways, "定价方案版本号"},
	{"scope", requiredAlways, "计价范围"},
	{"direction", requiredAlways, "价格方向：BUY、SELL、INTERNAL"},
	{"purpose", requiredAlways, "计价目的：CUSTOMER_CHARGE、SUPPLIER_COST、INTERNAL_PRICE"},
	{"aggregation", requiredAlways, "计价对象：PER_PACKAGE（逐包）、PER_SHIPMENT、PER_MASTER_DOCUMENT；必须写明"},
	{"baseChargeCode", requiredAlways, "基础运费的费用代码（大写字母、数字、下划线）"},
	{"periodStartsAt", requiredAlways, "方案生效起点，带时区的时刻，如 2026-01-01T00:00:00Z"},
	{"periodEndsAt", requiredOptional, "方案生效止点；空即不设止点"},
	{"rateTableId", requiredAlways, "主价表标识，指 tables 表的一行"},
	{"weightPolicyId", requiredAlways, "计重策略标识"},
	{"weightPolicyVersion", requiredAlways, "计重策略版本号"},
	{"weightMethod", requiredAlways, "计重方法：ACTUAL_ONLY、MAX"},
	{"volumetricDivisor", requiredConditional, "体积重除数；与 volumetricLengthUnit 成对，进位段写在 weight_rounding 的 VOLUMETRIC 行"},
	{"volumetricLengthUnit", requiredConditional, "体积重的长度单位：CM、IN；与 volumetricDivisor 成对"},
	{"amountRoundingMode", requiredConditional, "金额取整模式：NONE、CEILING、HALF_UP；与下两格同有同无"},
	{"amountRoundingIncrement", requiredConditional, "金额取整的进位单位，按主价表币种"},
	{"amountRoundingPoints", requiredConditional, "金额取整的应用点，以英文分号分隔：PER_LINE、AFTER_CONVERSION、TOTAL"},
	{"directionAuthorizationId", requiredAlways, "价格方向授权标识（party-commercial 签发）"},
	{"directionAuthorizationVersion", requiredAlways, "价格方向授权版本号"},
}

// sheets 是表集合，顺序即模板里的表序，也是问题清单的排序依据。
var sheets = []sheetSpec{
	{sheetCard, "方案头与登记件：每个字段一行", []column{
		{"field", requiredAlways, "字段名，见本说明 card 各行"},
		{"value", requiredConditional, "字段值"},
	}},
	{sheetTables, "价表表头：主价表与附加费查表用的价表都在这里声明", []column{
		{"tableId", requiredAlways, "价表标识"},
		{"tableVersion", requiredAlways, "价表版本号"},
		{"family", requiredAlways, "价表族：WEIGHT_ZONE、FIRST_CONTINUE、UNIT_PRICE"},
		{"currency", requiredAlways, "本表各行金额的币种，如 CNY"},
		{"weightUnit", requiredAlways, "本表各行重量的单位：G、KG、OZ、LB"},
		{"periodStartsAt", requiredAlways, "价表生效起点，带时区的时刻"},
		{"periodEndsAt", requiredOptional, "价表生效止点；空即不设止点"},
	}},
	{sheetRatesWeightZone, "WEIGHT_ZONE 族的价表行：分区 × 重量区间 → 金额", []column{
		{"tableId", requiredAlways, "指一张 WEIGHT_ZONE 族的表"},
		{"entryId", requiredAlways, "价表行标识"},
		{"zone", requiredAlways, "分区名"},
		{"minimum", requiredAlways, "区间下界，按表的重量单位"},
		{"maximum", requiredOptional, "区间上界；空即不封顶"},
		{"amount", requiredAlways, "金额，按表的币种"},
	}},
	{sheetRatesFirstContinue, "FIRST_CONTINUE 族的价表行：首重 + 续重", []column{
		{"tableId", requiredAlways, "指一张 FIRST_CONTINUE 族的表"},
		{"entryId", requiredAlways, "价表行标识"},
		{"zone", requiredAlways, "分区名"},
		{"firstWeight", requiredAlways, "首重"},
		{"firstAmount", requiredAlways, "首重金额"},
		{"step", requiredAlways, "续重步长"},
		{"stepAmount", requiredAlways, "每步续重金额"},
	}},
	{sheetRatesUnitPrice, "UNIT_PRICE 族的价表行：计费重 × 单价", []column{
		{"tableId", requiredAlways, "指一张 UNIT_PRICE 族的表"},
		{"entryId", requiredAlways, "价表行标识"},
		{"zone", requiredAlways, "分区名"},
		{"amountPerUnit", requiredAlways, "每个重量单位的金额"},
	}},
	{sheetWeightRounding, "计费重与体积重的进位段：同一 policy 的各行按行序组成分段", []column{
		{"policy", requiredAlways, "CHARGEABLE（计费重，必须有）或 VOLUMETRIC（体积重，与 card 的体积重因子同有同无）"},
		{"mode", requiredAlways, "进位模式：NONE、CEILING、HALF_UP"},
		{"increment", requiredAlways, "进位单位"},
		{"maximum", requiredOptional, "本段上界；空即最后一段、不封顶"},
		{"unit", requiredAlways, "increment 与 maximum 的重量单位：G、KG、OZ、LB"},
	}},
	{sheetFixedCharges, "固定费用", []column{
		{"ruleId", requiredAlways, "规则标识"},
		{"chargeCode", requiredAlways, "费用代码"},
		{"description", requiredAlways, "说明"},
		{"effect", requiredAlways, "ADD 或 DEDUCT"},
		{"amount", requiredAlways, "金额，按主价表币种"},
		{"order", requiredAlways, "顺序号，整数"},
		{"unit", requiredOptional, "空即按计价对象计；PER_PIECE 为按件（只对非逐包方案成立）"},
	}},
	{sheetSurcharges, "附加费：触发条件指 conditions 的根，计算方式指 calculations 的根", []column{
		{"ruleId", requiredAlways, "规则标识"},
		{"chargeCode", requiredAlways, "费用代码"},
		{"description", requiredAlways, "说明"},
		{"effect", requiredAlways, "ADD 或 DEDUCT"},
		{"conditionId", requiredAlways, "触发条件：conditions 表里一个根节点的 nodeId"},
		{"calculationId", requiredAlways, "计算方式：calculations 表里一个根节点的 nodeId"},
		{"exclusivity", requiredOptional, "空即未声明互斥立场；STANDALONE 或 GROUPED"},
		{"exclusivityGroup", requiredConditional, "互斥组名；GROUPED 时必填，否则必空"},
		{"priority", requiredConditional, "组内优先级，整数；GROUPED 时必填，否则必空"},
		{"unit", requiredOptional, "空即按计价对象计；PER_PIECE 为按件"},
		{"minimumWeightId", requiredConditional, "条件最低计价重量的标识；与下两格同有同无"},
		{"minimumWeightConditionId", requiredConditional, "条件最低计价重量的条件：conditions 表里一个根节点的 nodeId"},
		{"minimumWeight", requiredConditional, "条件最低计价重量，按主价表的重量单位"},
	}},
	{sheetConditions, "条件树：一行一个节点，没有 parentId 的是根；兄弟节点的先后即行序", []column{
		{"nodeId", requiredAlways, "节点标识，本表内唯一"},
		{"parentId", requiredOptional, "父节点的 nodeId；空即根"},
		{"kind", requiredAlways, "PREDICATE（判定）、ALL_OF（全部满足）、ANY_OF（任一满足）"},
		{"source", requiredConditional, "判定的特征来源；PREDICATE 必填"},
		{"operator", requiredConditional, "比较：GT、GE、LT、LE、EQ；长度、体积、重量类来源必填，类别类来源空或 EQ"},
		{"threshold", requiredConditional, "阈值；长度、体积、重量类来源必填"},
		{"unit", requiredConditional, "阈值单位：长度与体积类为 CM、IN，重量类为 G、KG、OZ、LB"},
		{"category", requiredConditional, "期望的类别值；ZONE、ADDRESS_TYPE、SERVICE_OPTION 来源必填"},
	}},
	{sheetCalculations, "计算树：一行一个节点；只有 GREATER_OF 能当父节点，且恰好两个子节点", []column{
		{"nodeId", requiredAlways, "节点标识，本表内唯一"},
		{"parentId", requiredOptional, "父节点的 nodeId；空即根"},
		{"method", requiredAlways, "FIXED_AMOUNT、TABLE_LOOKUP、PERCENT_OF_BASIS、GREATER_OF、SERIES_AMOUNT"},
		{"amount", requiredConditional, "FIXED_AMOUNT 的金额，按主价表币种"},
		{"tableId", requiredConditional, "TABLE_LOOKUP 查的表，指 tables"},
		{"percentage", requiredConditional, "PERCENT_OF_BASIS 的百分数（12.5 即 12.5%）；与 seriesKind 二选一"},
		{"seriesKind", requiredConditional, "PERCENT_OF_BASIS 取序列读数作费率时的序列种类：FUEL_RATE、EXCHANGE_RATE、PUBLISHED_AMOUNT"},
		{"seriesFactor", requiredConditional, "与 seriesKind 成对：乘在序列读数上的系数"},
		{"basisDependencyId", requiredConditional, "PERCENT_OF_BASIS 的基数，指 charge_dependencies 的 dependencyId"},
		{"seriesId", requiredConditional, "SERIES_AMOUNT 取金额的序列标识"},
		{"outOfWindow", requiredConditional, "SERIES_AMOUNT 在序列无读数时：NOT_CHARGED、PENDING"},
	}},
	{sheetChargeDependencies, "费用依赖：百分比计算的基数", []column{
		{"dependencyId", requiredAlways, "依赖标识"},
		{"chargeCode", requiredAlways, "依附的费用代码"},
		{"composition", requiredAlways, "ALL_CHARGES（全部费用）或 LISTED_CHARGES（列出的费用）"},
		{"includes", requiredConditional, "LISTED_CHARGES 必填、否则必空：费用代码，以英文分号分隔"},
		{"excludes", requiredOptional, "排除的费用代码，以英文分号分隔"},
	}},
	{sheetReferenceSeries, "参考序列绑定：只绑序列标识，不绑版本", []column{
		{"kind", requiredAlways, "FUEL_RATE、EXCHANGE_RATE、PUBLISHED_AMOUNT"},
		{"seriesId", requiredAlways, "序列标识"},
	}},
	{sheetExclusions, "排除条款：条件指 conditions 的根", []column{
		{"exclusionId", requiredAlways, "条款标识"},
		{"clause", requiredAlways, "条款原文或出处"},
		{"conditionId", requiredAlways, "conditions 表里一个根节点的 nodeId"},
	}},
	{sheetReferenceCatalogues, "参考目录绑定", []column{
		{"kind", requiredAlways, "ZONE 或 REMOTE_TIER"},
		{"catalogueId", requiredAlways, "目录标识"},
	}},
	{sheetManifestDependencies, "版本清单依赖：方案另外依赖的版本化工件", []column{
		{"kind", requiredAlways, "工件种类：pricing-plan、rate-table、weight-policy、reference-series、reference-catalogue、commercial-policy、commercial-authorization、numeric-profile"},
		{"id", requiredAlways, "工件标识"},
		{"version", requiredAlways, "工件版本号"},
	}},
}

func sheetByName(name string) (sheetSpec, int, bool) {
	for index, spec := range sheets {
		if spec.name == name {
			return spec, index, true
		}
	}
	return sheetSpec{}, 0, false
}

func cardField(key string) (column, int, bool) {
	for index, candidate := range cardFields {
		if candidate.key == key {
			return candidate, index, true
		}
	}
	return column{}, 0, false
}
