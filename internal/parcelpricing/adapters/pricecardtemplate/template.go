// Package pricecardtemplate 把一份价卡导入模板读成领域值，也写出空白模板。
//
// 契约在 docs/design/pp-price-card-import-template-and-validation-spec.md：本包是它的实现，
// 表集合、列键、必填性、每列的中文名与说明、封闭取值都只在本文件定义一次，读取、写出模板与
// 模板规格只读口共用，模板文件、解析器与规格口因此不会漂开（规范第七节）。语义校验不在本包：
// 每一格交给哪个领域构造函数写在规范第四节，构造门拒了就如实回报，这里不预判、不另立规则
// （ADR-0101 Context）。
//
// 模板有两种载体：电子表格文件（Reader.ReadPriceCardTemplate）与页面表单提交的结构化工作簿
// （Reader.ReadPriceCardWorkbook）。两者先译成同一个内部表示（spreadsheet.Workbook）再走同一套
// 译码与校验，不存在第二套判定。
package pricecardtemplate

import (
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/refdata"
)

// TemplateVersion 是本构建写出的模板版本，也是最新的一版（规范「版本」一节）。表集合、列集合或列语义变化时进位，
// 可取值的增减也算列语义变化。进位不等于旧版作废：旧版文件仍按旧版的表集合与取值读，读出的内容与进位之前
// 逐位相同（acceptedTemplateVersions）。
const TemplateVersion = "PPT-2"

// templateVersionV1 是第一版。PPT-2 只增不改：新增 total_bounds 表与 TABLE_LOOKUP_BY_REMOTE_TIER 计算类型，
// 既有的表、列与取值一个不动，所以 PPT-1 文件照旧读通，且不会因新增的东西而多出任何问题。
const templateVersionV1 = "PPT-1"

// acceptedTemplateVersions 按先后排列，最后一格是 TemplateVersion。
var acceptedTemplateVersions = []string{templateVersionV1, TemplateVersion}

// versionRank 报出一版在先后序里的位置；空串与不认的串取最新一格（给不带版本的调用方：规格口）。
func versionRank(version string) int {
	for index, accepted := range acceptedTemplateVersions {
		if accepted == version {
			return index
		}
	}
	return len(acceptedTemplateVersions) - 1
}

func acceptedVersion(version string) bool {
	for _, accepted := range acceptedTemplateVersions {
		if accepted == version {
			return true
		}
	}
	return false
}

// availableIn 报出自 since 起才有的东西在 version 这一版里有没有；since 为空即从第一版就有。
func availableIn(since, version string) bool {
	return since == "" || versionRank(version) >= versionRank(since)
}

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
	sheetTotalBounds          = "total_bounds"
	sheetReferenceSeries      = "reference_series"
	sheetExclusions           = "exclusions"
	sheetReferenceCatalogues  = "reference_catalogues"
	sheetManifestDependencies = "manifest_dependencies"
	// sheetReadme 是解析时整张跳过的说明表。
	sheetReadme = "说明"
)

// methodTableLookupByRemoteTier 是 PPT-2 新增的计算类型：附加费价表按偏远档位查档（ADR-0348 决定三）。
const methodTableLookupByRemoteTier = "TABLE_LOOKUP_BY_REMOTE_TIER"

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

// code 是规格口里的英文写法：与说明表里的中文写法是同一个枚举，不另存一份。
func (value requirement) code() string {
	switch value {
	case requiredAlways:
		return "always"
	case requiredOptional:
		return "optional"
	case requiredConditional:
		return "conditional"
	default:
		return ""
	}
}

// column 是一列（card 表则是一个字段）的全部定义：键、中文名、必填性与说明。封闭取值不挂在这里而在
// closedVocabulary，因为同一份取值常被几张表的几列共用。
type column struct {
	key         string
	title       string
	requirement requirement
	note        string
}

type sheetSpec struct {
	name  string
	title string
	// single 为真：这张表至多一行（card 表则是固定的一组字段），不是可以任意增行的明细表。
	single bool
	// since 非空：这张表自该版起才有；更早的版本里它是未知表。
	since   string
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
		"ZONE", "ADDRESS_TYPE", "ADDRESS_NATURE", "SERVICE_OPTION",
	}
	operatorCodes    = []string{"GT", "GE", "LT", "LE", "EQ"}
	methodCodes      = []string{"FIXED_AMOUNT", "TABLE_LOOKUP", "PERCENT_OF_BASIS", "GREATER_OF", "SERIES_AMOUNT", methodTableLookupByRemoteTier}
	seriesKindCodes  = []string{"FUEL_RATE", "EXCHANGE_RATE", "PUBLISHED_AMOUNT"}
	outOfWindowCodes = []string{"NOT_CHARGED", "PENDING"}
	compositionCodes = []string{"ALL_CHARGES", "LISTED_CHARGES"}
	// conditionUnitCodes 是 conditions.unit 的并集：重量类来源收重量单位，长度与体积类来源收长度单位，哪一类
	// 由 source 定（翻译处按来源逐格判），所以规格口只能给并集。
	conditionUnitCodes = append(append([]string{}, weightUnitCodes...), lengthUnitCodes...)
	catalogueKindCodes = []string{"ZONE", "REMOTE_TIER"}
	// currencyCodes 是产品内置币种目录里的码（refdata，ISO 4217 常用集）：价表表头的币种只收它们，译码的门（refdata.CheckCurrency）
	// 与这份清单是同一个目录。
	currencyCodes = refdataCurrencyCodes()
	// addressNatureCodes 是 ADDRESS_NATURE 来源的期望取值，领域的封闭词表；译码交 domain.NewCategoryFeatureCondition，由领域判。
	addressNatureCodes = []string{domain.AddressNatureCommercial.String(), domain.AddressNatureResidential.String()}
	artifactKindCodes  = []string{
		"pricing-plan", "rate-table", "weight-policy", "reference-series", "reference-catalogue",
		"commercial-policy", "commercial-authorization", "numeric-profile",
	}
)

// vocabulary 是一列的封闭取值。since 登记自某一版才有的取值：更早的版本里它不是可取值，且读到它要说清是版本问题。
type vocabulary struct {
	values []string
	// multi 为真：这一格是以英文分号分隔的列表，values 是每个元素的可取值。
	multi bool
	since map[string]string
}

// valuesFor 报出 version 这一版里这一列的可取值，保持 values 的先后。
func (vocab vocabulary) valuesFor(version string) []string {
	if len(vocab.since) == 0 {
		return vocab.values
	}
	kept := make([]string, 0, len(vocab.values))
	for _, value := range vocab.values {
		if availableIn(vocab.since[value], version) {
			kept = append(kept, value)
		}
	}
	return kept
}

var remoteTierMethodSince = map[string]string{methodTableLookupByRemoteTier: TemplateVersion}

func refdataCurrencyCodes() []string {
	currencies := refdata.Currencies()
	codes := make([]string, 0, len(currencies))
	for _, currency := range currencies {
		codes = append(codes, currency.Code)
	}
	return codes
}

// conditionalVocabulary 登记「只在同一行另一格取某值时才封闭」的列：这一列平时是开放词汇（分区名、偏远档位、服务选项由租户填），
// 满足条件时取值才限在 values 里。规格口把它们作为 allowedValuesWhen 交出，列本身不带 allowedValues，免得编辑器把开放词汇
// 的格也画成下拉。读取不经这里——这类取值的判在领域构造门，规格口的清单取自领域的封闭词表。
type conditionalVocabulary struct {
	column string
	equals string
	values []string
}

var conditionalClosedVocabulary = map[string]map[string][]conditionalVocabulary{
	sheetConditions: {
		"category": {{column: "source", equals: "ADDRESS_NATURE", values: addressNatureCodes}},
	},
}

// closedVocabulary 是全部封闭取值列的登记处，表名 → 列键 → 取值。读取（decoder.closedCode）与规格口都只从这里取：
// 一列的可取值只有这一处定义，规格口答的与读取实际收的不会是两份。
var closedVocabulary = map[string]map[string]vocabulary{
	sheetCard: {
		"direction":            {values: directionCodes},
		"purpose":              {values: purposeCodes},
		"aggregation":          {values: aggregationCodes},
		"weightMethod":         {values: weightMethodCodes},
		"volumetricLengthUnit": {values: lengthUnitCodes},
		"amountRoundingMode":   {values: roundingModeCodes},
		"amountRoundingPoints": {values: roundingPointCodes, multi: true},
	},
	sheetTables: {
		"family":     {values: familyCodes},
		"currency":   {values: currencyCodes},
		"weightUnit": {values: weightUnitCodes},
	},
	sheetWeightRounding: {
		"policy": {values: roundingPolicyCodes},
		"mode":   {values: roundingModeCodes},
		"unit":   {values: weightUnitCodes},
	},
	sheetFixedCharges: {
		"effect": {values: effectCodes},
		"unit":   {values: chargeUnitCodes},
	},
	sheetSurcharges: {
		"effect":      {values: effectCodes},
		"exclusivity": {values: exclusivityCodes},
		"unit":        {values: chargeUnitCodes},
	},
	sheetConditions: {
		"kind":     {values: triggerKindCodes},
		"source":   {values: featureSourceCodes},
		"operator": {values: operatorCodes},
		"unit":     {values: conditionUnitCodes},
	},
	sheetCalculations: {
		"method":      {values: methodCodes, since: remoteTierMethodSince},
		"seriesKind":  {values: seriesKindCodes},
		"outOfWindow": {values: outOfWindowCodes},
	},
	sheetChargeDependencies: {
		"composition": {values: compositionCodes},
	},
	sheetReferenceSeries: {
		"kind": {values: seriesKindCodes},
	},
	sheetReferenceCatalogues: {
		"kind": {values: catalogueKindCodes},
	},
	sheetManifestDependencies: {
		"kind": {values: artifactKindCodes},
	},
}

// vocabularyOf 取一列的封闭取值；没登记是编程错误（给 closedCode 的调用点：新加封闭列忘了登记，第一次读它就停）。
func vocabularyOf(sheet, key string) vocabulary {
	vocab, found := closedVocabulary[sheet][key]
	if !found {
		panic("pricecardtemplate: " + sheet + "." + key + " 没有登记封闭取值")
	}
	return vocab
}

// cardFields 是 card 表的字段集合，顺序即模板里的行序。
var cardFields = []column{
	{"templateVersion", "模板版本", requiredAlways, "模板版本，写 " + TemplateVersion + "（旧版 " + templateVersionV1 + " 的文件仍然接受）"},
	{"planId", "方案标识", requiredAlways, "定价方案标识"},
	{"planVersion", "方案版本号", requiredAlways, "定价方案版本号"},
	{"scope", "计价范围", requiredAlways, "计价范围"},
	{"direction", "价格方向", requiredAlways, "价格方向：BUY、SELL、INTERNAL"},
	{"purpose", "计价目的", requiredAlways, "计价目的：CUSTOMER_CHARGE、SUPPLIER_COST、INTERNAL_PRICE"},
	{"aggregation", "计价对象", requiredAlways, "计价对象：PER_PACKAGE（逐包）、PER_SHIPMENT、PER_MASTER_DOCUMENT；必须写明"},
	{"baseChargeCode", "基础运费代码", requiredAlways, "基础运费的费用代码（大写字母、数字、下划线）"},
	{"periodStartsAt", "方案生效起点", requiredAlways, "方案生效起点，带时区的时刻，如 2026-01-01T00:00:00Z"},
	{"periodEndsAt", "方案生效止点", requiredOptional, "方案生效止点；空即不设止点"},
	{"rateTableId", "主价表标识", requiredAlways, "主价表标识，指 tables 表的一行"},
	{"weightPolicyId", "计重策略标识", requiredAlways, "计重策略标识"},
	{"weightPolicyVersion", "计重策略版本号", requiredAlways, "计重策略版本号"},
	{"weightMethod", "计重方法", requiredAlways, "计重方法：ACTUAL_ONLY、MAX"},
	{"volumetricDivisor", "体积重除数", requiredConditional, "体积重除数；与 volumetricLengthUnit 成对，进位段写在 weight_rounding 的 VOLUMETRIC 行"},
	{"volumetricLengthUnit", "体积重长度单位", requiredConditional, "体积重的长度单位：CM、IN；与 volumetricDivisor 成对"},
	{"amountRoundingMode", "金额取整模式", requiredConditional, "金额取整模式：NONE、CEILING、HALF_UP；与下两格同有同无"},
	{"amountRoundingIncrement", "金额取整进位单位", requiredConditional, "金额取整的进位单位，按主价表币种"},
	{"amountRoundingPoints", "金额取整应用点", requiredConditional, "金额取整的应用点，以英文分号分隔：PER_LINE、AFTER_CONVERSION、TOTAL"},
	{"directionAuthorizationId", "价格方向授权标识", requiredAlways, "价格方向授权标识（party-commercial 签发）"},
	{"directionAuthorizationVersion", "价格方向授权版本号", requiredAlways, "价格方向授权版本号"},
}

// sheets 是表集合，顺序即模板里的表序，也是问题清单的排序依据。
var sheets = []sheetSpec{
	{name: sheetCard, title: "方案头与登记件", single: true, note: "方案头与登记件：每个字段一行", columns: []column{
		{"field", "字段名", requiredAlways, "字段名，见本说明 card 各行"},
		{"value", "字段值", requiredConditional, "字段值"},
	}},
	{name: sheetTables, title: "价表表头", note: "价表表头：主价表与附加费查表用的价表都在这里声明", columns: []column{
		{"tableId", "价表标识", requiredAlways, "价表标识"},
		{"tableVersion", "价表版本号", requiredAlways, "价表版本号"},
		{"family", "价表族", requiredAlways, "价表族：WEIGHT_ZONE、FIRST_CONTINUE、UNIT_PRICE"},
		{"currency", "币种", requiredAlways, "本表各行金额的币种，取产品内置币种目录里的码，如 CNY"},
		{"weightUnit", "重量单位", requiredAlways, "本表各行重量的单位：G、KG、OZ、LB"},
		{"periodStartsAt", "价表生效起点", requiredAlways, "价表生效起点，带时区的时刻"},
		{"periodEndsAt", "价表生效止点", requiredOptional, "价表生效止点；空即不设止点"},
	}},
	{name: sheetRatesWeightZone, title: "重量分区价表行", note: "WEIGHT_ZONE 族的价表行：分区 × 重量区间 → 金额", columns: []column{
		{"tableId", "价表标识", requiredAlways, "指一张 WEIGHT_ZONE 族的表"},
		{"entryId", "价表行标识", requiredAlways, "价表行标识"},
		{"zone", "分区名", requiredAlways, "分区名；被 TABLE_LOOKUP_BY_REMOTE_TIER 查的价表里填偏远档位（T1、T2……）"},
		{"minimum", "区间下界", requiredAlways, "区间下界，按表的重量单位"},
		{"maximum", "区间上界", requiredOptional, "区间上界；空即不封顶"},
		{"amount", "金额", requiredAlways, "金额，按表的币种"},
	}},
	{name: sheetRatesFirstContinue, title: "首续重价表行", note: "FIRST_CONTINUE 族的价表行：首重 + 续重", columns: []column{
		{"tableId", "价表标识", requiredAlways, "指一张 FIRST_CONTINUE 族的表"},
		{"entryId", "价表行标识", requiredAlways, "价表行标识"},
		{"zone", "分区名", requiredAlways, "分区名；被 TABLE_LOOKUP_BY_REMOTE_TIER 查的价表里填偏远档位（T1、T2……）"},
		{"firstWeight", "首重", requiredAlways, "首重"},
		{"firstAmount", "首重金额", requiredAlways, "首重金额"},
		{"step", "续重步长", requiredAlways, "续重步长"},
		{"stepAmount", "续重金额", requiredAlways, "每步续重金额"},
	}},
	{name: sheetRatesUnitPrice, title: "单价价表行", note: "UNIT_PRICE 族的价表行：计费重 × 单价", columns: []column{
		{"tableId", "价表标识", requiredAlways, "指一张 UNIT_PRICE 族的表"},
		{"entryId", "价表行标识", requiredAlways, "价表行标识"},
		{"zone", "分区名", requiredAlways, "分区名；被 TABLE_LOOKUP_BY_REMOTE_TIER 查的价表里填偏远档位（T1、T2……）"},
		{"amountPerUnit", "单位金额", requiredAlways, "每个重量单位的金额"},
	}},
	{name: sheetWeightRounding, title: "计重进位段", note: "计费重与体积重的进位段：同一 policy 的各行按行序组成分段", columns: []column{
		{"policy", "进位对象", requiredAlways, "CHARGEABLE（计费重，必须有）或 VOLUMETRIC（体积重，与 card 的体积重因子同有同无）"},
		{"mode", "进位模式", requiredAlways, "进位模式：NONE、CEILING、HALF_UP"},
		{"increment", "进位单位", requiredAlways, "进位单位"},
		{"maximum", "本段上界", requiredOptional, "本段上界；空即最后一段、不封顶"},
		{"unit", "重量单位", requiredAlways, "increment 与 maximum 的重量单位：G、KG、OZ、LB"},
	}},
	{name: sheetFixedCharges, title: "固定费用", note: "固定费用", columns: []column{
		{"ruleId", "规则标识", requiredAlways, "规则标识"},
		{"chargeCode", "费用代码", requiredAlways, "费用代码"},
		{"description", "说明", requiredAlways, "说明"},
		{"effect", "收费方向", requiredAlways, "ADD 或 DEDUCT"},
		{"amount", "金额", requiredAlways, "金额，按主价表币种"},
		{"order", "顺序号", requiredAlways, "顺序号，整数"},
		{"unit", "计费单位", requiredOptional, "空即按计价对象计；PER_PIECE 为按件（只对非逐包方案成立）"},
	}},
	{name: sheetSurcharges, title: "附加费", note: "附加费：触发条件指 conditions 的根，计算方式指 calculations 的根", columns: []column{
		{"ruleId", "规则标识", requiredAlways, "规则标识"},
		{"chargeCode", "费用代码", requiredAlways, "费用代码"},
		{"description", "说明", requiredAlways, "说明"},
		{"effect", "收费方向", requiredAlways, "ADD 或 DEDUCT"},
		{"conditionId", "触发条件", requiredAlways, "触发条件：conditions 表里一个根节点的 nodeId"},
		{"calculationId", "计算方式", requiredAlways, "计算方式：calculations 表里一个根节点的 nodeId"},
		{"exclusivity", "互斥立场", requiredOptional, "空即未声明互斥立场；STANDALONE 或 GROUPED"},
		{"exclusivityGroup", "互斥组名", requiredConditional, "互斥组名；GROUPED 时必填，否则必空"},
		{"priority", "组内优先级", requiredConditional, "组内优先级，整数；GROUPED 时必填，否则必空"},
		{"unit", "计费单位", requiredOptional, "空即按计价对象计；PER_PIECE 为按件"},
		{"minimumWeightId", "条件最低计价重量标识", requiredConditional, "条件最低计价重量的标识；与下两格同有同无"},
		{"minimumWeightConditionId", "条件最低计价重量的条件", requiredConditional, "条件最低计价重量的条件：conditions 表里一个根节点的 nodeId"},
		{"minimumWeight", "条件最低计价重量", requiredConditional, "条件最低计价重量，按主价表的重量单位"},
	}},
	{name: sheetConditions, title: "条件树", note: "条件树：一行一个节点，没有 parentId 的是根；兄弟节点的先后即行序", columns: []column{
		{"nodeId", "节点标识", requiredAlways, "节点标识，本表内唯一"},
		{"parentId", "父节点", requiredOptional, "父节点的 nodeId；空即根"},
		{"kind", "节点类型", requiredAlways, "PREDICATE（判定）、ALL_OF（全部满足）、ANY_OF（任一满足）"},
		{"source", "判定的特征来源", requiredConditional, "判定的特征来源；PREDICATE 必填。CHARGEABLE_WEIGHT 是包裹自身的计价重量（派生、进位之后），不含条件最低计价重量的抬高"},
		{"operator", "比较运算", requiredConditional, "比较：GT、GE、LT、LE、EQ；长度、体积、重量类来源必填，类别类来源空或 EQ"},
		{"threshold", "阈值", requiredConditional, "阈值；长度、体积、重量类来源必填"},
		{"unit", "阈值单位", requiredConditional, "阈值单位：长度与体积类为 CM、IN，重量类为 G、KG、OZ、LB"},
		{"category", "类别值", requiredConditional, "期望的类别值；类别类来源（ZONE、ADDRESS_TYPE、SERVICE_OPTION、ADDRESS_NATURE）必填。前三者是开放词汇（分区名、偏远档位、服务选项由租户填）；source 为 ADDRESS_NATURE 时只收 COMMERCIAL、RESIDENTIAL"},
	}},
	{name: sheetCalculations, title: "计算树", note: "计算树：一行一个节点；只有 GREATER_OF 能当父节点，且恰好两个子节点", columns: []column{
		{"nodeId", "节点标识", requiredAlways, "节点标识，本表内唯一"},
		{"parentId", "父节点", requiredOptional, "父节点的 nodeId；空即根"},
		{"method", "计算类型", requiredAlways, "FIXED_AMOUNT、TABLE_LOOKUP、PERCENT_OF_BASIS、GREATER_OF、SERIES_AMOUNT；" + methodTableLookupByRemoteTier + "（" + TemplateVersion + " 起）按偏远档位查价表，价表行的 zone 列写档位，并须在 reference_catalogues 绑一份 REMOTE_TIER 目录"},
		{"amount", "固定金额", requiredConditional, "FIXED_AMOUNT 的金额，按主价表币种"},
		{"tableId", "查的价表", requiredConditional, "TABLE_LOOKUP 与 " + methodTableLookupByRemoteTier + " 查的表，指 tables"},
		{"percentage", "百分数", requiredConditional, "PERCENT_OF_BASIS 的百分数（12.5 即 12.5%）；与 seriesKind 二选一"},
		{"seriesKind", "序列种类", requiredConditional, "PERCENT_OF_BASIS 取序列读数作费率时的序列种类：FUEL_RATE、EXCHANGE_RATE、PUBLISHED_AMOUNT"},
		{"seriesFactor", "序列系数", requiredConditional, "与 seriesKind 成对：乘在序列读数上的系数"},
		{"basisDependencyId", "基数依赖", requiredConditional, "PERCENT_OF_BASIS 的基数，指 charge_dependencies 的 dependencyId"},
		{"seriesId", "序列标识", requiredConditional, "SERIES_AMOUNT 取金额的序列标识"},
		{"outOfWindow", "无读数时的处置", requiredConditional, "SERIES_AMOUNT 在序列无读数时：NOT_CHARGED、PENDING"},
	}},
	{name: sheetChargeDependencies, title: "费用依赖", note: "费用依赖：百分比计算的基数", columns: []column{
		{"dependencyId", "依赖标识", requiredAlways, "依赖标识"},
		{"chargeCode", "费用代码", requiredAlways, "依附的费用代码"},
		{"composition", "基数构成", requiredAlways, "ALL_CHARGES（全部费用）或 LISTED_CHARGES（列出的费用）"},
		{"includes", "计入的费用", requiredConditional, "LISTED_CHARGES 必填、否则必空：费用代码，以英文分号分隔"},
		{"excludes", "排除的费用", requiredOptional, "排除的费用代码，以英文分号分隔"},
	}},
	{name: sheetTotalBounds, title: "最低 / 最高收费", single: true, since: TemplateVersion,
		note: "最低 / 最高收费：对整票合计声明下限与上限，至多一行，两端可只填一端。各行合计低于最低收费时补足到最低收费，高于最高收费时封顶到最高收费", columns: []column{
			{"minimum", "最低收费", requiredConditional, "最低收费（整票合计的下限，按主价表币种）：合计低于它就补一条 MINIMUM_CHARGE 补足行；与 maximum 至少填一个。卡声明了金额取整时，须落在进位单位的整数倍上"},
			{"maximum", "最高收费", requiredConditional, "最高收费（整票合计的上限，按主价表币种）：合计高于它就补一条 MAXIMUM_CHARGE 冲减行；不低于 minimum。卡声明了金额取整时，须落在进位单位的整数倍上"},
		}},
	{name: sheetReferenceSeries, title: "参考序列绑定", note: "参考序列绑定：只绑序列标识，不绑版本", columns: []column{
		{"kind", "序列种类", requiredAlways, "FUEL_RATE、EXCHANGE_RATE、PUBLISHED_AMOUNT"},
		{"seriesId", "序列标识", requiredAlways, "序列标识"},
	}},
	{name: sheetExclusions, title: "排除条款", note: "排除条款：条件指 conditions 的根", columns: []column{
		{"exclusionId", "条款标识", requiredAlways, "条款标识"},
		{"clause", "条款原文", requiredAlways, "条款原文或出处"},
		{"conditionId", "条件", requiredAlways, "conditions 表里一个根节点的 nodeId"},
	}},
	{name: sheetReferenceCatalogues, title: "参考目录绑定", note: "参考目录绑定", columns: []column{
		{"kind", "目录种类", requiredAlways, "ZONE 或 REMOTE_TIER"},
		{"catalogueId", "目录标识", requiredAlways, "目录标识"},
	}},
	{name: sheetManifestDependencies, title: "版本清单依赖", note: "版本清单依赖：方案另外依赖的版本化工件", columns: []column{
		{"kind", "工件种类", requiredAlways, "工件种类：pricing-plan、rate-table、weight-policy、reference-series、reference-catalogue、commercial-policy、commercial-authorization、numeric-profile"},
		{"id", "工件标识", requiredAlways, "工件标识"},
		{"version", "工件版本号", requiredAlways, "工件版本号"},
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
