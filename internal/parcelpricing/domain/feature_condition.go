package domain

// FeatureSource 是判定条件可以读取的可判定量的封闭集合。保持封闭，评价才能仅凭版本清单
// 重放每条规则的命中与未命中；集合一旦开放，就还需要规则正文本身。
//
// 十项与 CONTEXT「特征」词条的闭合集合一一对应：七项数值量按量纲比较，三项类别量
// （分区、地址类型、服务选项）按取值相等——类别没有大小，比较运算配它们在构造期就
// 拦下。体积重与计价重量是评价过程的派生量：条件要读它们，评价器必须先把算出的值
// 填进 PackageFeatures，缺席时判定报特征不可用而不是悄悄未命中——判定条件必须可
// 逐条解释命中与否，缺量解释不出。
type FeatureSource string

const (
	FeatureLongestSide       FeatureSource = "LONGEST_SIDE"
	FeatureSecondLongestSide FeatureSource = "SECOND_LONGEST_SIDE"
	FeatureLengthAndGirth    FeatureSource = "LENGTH_AND_GIRTH"
	FeatureVolume            FeatureSource = "VOLUME"
	FeatureActualWeight      FeatureSource = "ACTUAL_WEIGHT"
	FeatureVolumetricWeight  FeatureSource = "VOLUMETRIC_WEIGHT"
	FeatureChargeableWeight  FeatureSource = "CHARGEABLE_WEIGHT"
	FeatureZone              FeatureSource = "ZONE"
	FeatureAddressType       FeatureSource = "ADDRESS_TYPE"
	FeatureServiceOption     FeatureSource = "SERVICE_OPTION"
)

func (source FeatureSource) String() string { return string(source) }

func (source FeatureSource) valid() bool {
	return source.measure() != measureUnknown
}

// featureMeasure 是一个特征来源产出的物理量。阈值只有对着同一种量纲才有意义，所以配对
// 在这里检查一次，而不是在每次比较时都查；类别量自成一纲，只有相等可言。
type featureMeasure int

const (
	measureUnknown featureMeasure = iota
	measureLength
	measureVolume
	measureWeight
	measureCategory
)

func (source FeatureSource) measure() featureMeasure {
	switch source {
	case FeatureLongestSide, FeatureSecondLongestSide, FeatureLengthAndGirth:
		return measureLength
	case FeatureVolume:
		return measureVolume
	case FeatureActualWeight, FeatureVolumetricWeight, FeatureChargeableWeight:
		return measureWeight
	case FeatureZone, FeatureAddressType, FeatureServiceOption:
		return measureCategory
	default:
		return measureUnknown
	}
}

// ComparisonOperator 是判定条件可以使用的比较运算的封闭集合。卡上的条件从不需要算术，
// 所以这里不提供任何表达式引擎。
//
// 每个边界的两种读法都保留，因为承运商两种写法都用，而区间型条款本来就需要一个上界：
// DHL 的 Non-Conveyable Piece 适用于「56 至 150 磅之间」，两端含端；而本卡自己的超限
// 条款读作「超过」。把含端边界改写成严格边界，会迫使转抄者自造下一个可表示值。
//
// EQ 只配类别特征：类别量没有大小，四个次序运算配它们在构造期拦下；反过来数值特征
// 配 EQ 也拦——数值上的「恰好等于」在卡上从未出现过，出现时按 CONTEXT 126 作为比较
// 运算集合扩充进版本内容摘要，不在这里顺手放行。
type ComparisonOperator string

const (
	ComparisonGreaterThan        ComparisonOperator = "GT"
	ComparisonGreaterThanOrEqual ComparisonOperator = "GE"
	ComparisonLessThan           ComparisonOperator = "LT"
	ComparisonLessThanOrEqual    ComparisonOperator = "LE"
	ComparisonEquals             ComparisonOperator = "EQ"
)

func (operator ComparisonOperator) String() string { return string(operator) }

func (operator ComparisonOperator) valid() bool {
	switch operator {
	case ComparisonGreaterThan, ComparisonGreaterThanOrEqual, ComparisonLessThan, ComparisonLessThanOrEqual:
		return true
	case ComparisonEquals:
		return true
	default:
		return false
	}
}

// ordersNumbers 报告运算是否用于数值次序比较。
func (operator ComparisonOperator) ordersNumbers() bool {
	return operator != ComparisonEquals && operator.valid()
}

// holds 把比较运算施加到一个已经过单位校验的比较结果上。
func (operator ComparisonOperator) holds(comparison int) (bool, error) {
	switch operator {
	case ComparisonGreaterThan:
		return comparison > 0, nil
	case ComparisonGreaterThanOrEqual:
		return comparison >= 0, nil
	case ComparisonLessThan:
		return comparison < 0, nil
	case ComparisonLessThanOrEqual:
		return comparison <= 0, nil
	default:
		return false, ErrInvalidFeatureCondition
	}
}

// CategoryValue 是类别特征的一个取值（分区代号、地址类型、服务选项标识）。取值目录
// 属价卡与商业规则的实例半边，这里只要求非空。
type CategoryValue string

func (value CategoryValue) String() string { return string(value) }

func (value CategoryValue) valid() bool { return value != "" }

// PackageFeatures 保存为一次评价一次性派生出来的可判定量，使每条规则读到相同的值，
// 而不是各自从计价输入快照重新派生一遍。体积重与计价重量由评价器算出后经 With* 填入；
// 类别值来自计价输入快照的地址与商业维度。缺席的派生量与类别值不是零值命中——条件
// 读到缺席即报特征不可用。
type PackageFeatures struct {
	dimensions       Dimensions
	actualWeight     Weight
	volumetricWeight Weight
	chargeableWeight Weight
	zone             CategoryValue
	addressType      CategoryValue
	serviceOptions   []CategoryValue
}

func NewPackageFeatures(dimensions Dimensions, actualWeight Weight) (PackageFeatures, error) {
	if !dimensions.valid() {
		return PackageFeatures{}, ErrInvalidDimensions
	}
	if !actualWeight.valid() {
		return PackageFeatures{}, ErrInvalidWeight
	}
	return PackageFeatures{dimensions: dimensions, actualWeight: actualWeight}, nil
}

// WithDerivedWeights 交回带上派生重量的副本。评价器在体积重与计价重量确定后调用，
// 之后的判定条件才能读它们。
func (features PackageFeatures) WithDerivedWeights(volumetric, chargeable Weight) PackageFeatures {
	enriched := features
	enriched.volumetricWeight = volumetric
	enriched.chargeableWeight = chargeable
	return enriched
}

// WithCategories 交回带上类别值的副本。服务选项可多值——一个包裹可以同时带签名与
// 周六派，条件按「含该选项」判。
func (features PackageFeatures) WithCategories(
	zone CategoryValue,
	addressType CategoryValue,
	serviceOptions ...CategoryValue,
) PackageFeatures {
	enriched := features
	enriched.zone = zone
	enriched.addressType = addressType
	enriched.serviceOptions = append([]CategoryValue(nil), serviceOptions...)
	return enriched
}

func (features PackageFeatures) valid() bool {
	return features.dimensions.valid() && features.actualWeight.valid()
}

// weightOf 交回条件要读的重量特征；派生量缺席即特征不可用。
func (features PackageFeatures) weightOf(source FeatureSource) (Weight, error) {
	switch source {
	case FeatureActualWeight:
		return features.actualWeight, nil
	case FeatureVolumetricWeight:
		if !features.volumetricWeight.valid() {
			return Weight{}, ErrFeatureUnavailable
		}
		return features.volumetricWeight, nil
	case FeatureChargeableWeight:
		if !features.chargeableWeight.valid() {
			return Weight{}, ErrFeatureUnavailable
		}
		return features.chargeableWeight, nil
	default:
		return Weight{}, ErrInvalidFeatureCondition
	}
}

// categoryMatches 判类别特征是否等于期望值；类别值缺席即特征不可用。服务选项按
// 「含该选项」判——多值集合里任一相等即命中。
func (features PackageFeatures) categoryMatches(source FeatureSource, expected CategoryValue) (bool, error) {
	switch source {
	case FeatureZone:
		if !features.zone.valid() {
			return false, ErrFeatureUnavailable
		}
		return features.zone == expected, nil
	case FeatureAddressType:
		if !features.addressType.valid() {
			return false, ErrFeatureUnavailable
		}
		return features.addressType == expected, nil
	case FeatureServiceOption:
		if len(features.serviceOptions) == 0 {
			return false, ErrFeatureUnavailable
		}
		for _, option := range features.serviceOptions {
			if option == expected {
				return true, nil
			}
		}
		return false, nil
	default:
		return false, ErrInvalidFeatureCondition
	}
}

func (features PackageFeatures) length(source FeatureSource) (Length, error) {
	switch source {
	case FeatureLongestSide:
		return features.dimensions.LongestSide(), nil
	case FeatureSecondLongestSide:
		return features.dimensions.SecondLongestSide(), nil
	case FeatureLengthAndGirth:
		return features.dimensions.LengthPlusGirth()
	default:
		return Length{}, ErrInvalidFeatureCondition
	}
}

// FeatureCondition 是卡唯一需要的判定条件形状：一个特征、一个比较运算、一个阈值
// （数值特征）或一个期望取值（类别特征）。阈值与取值都由价卡版本声明，本包从不自带。
type FeatureCondition struct {
	source           FeatureSource
	operator         ComparisonOperator
	lengthThreshold  Length
	volumeThreshold  Volume
	weightThreshold  Weight
	categoryExpected CategoryValue
}

func NewLengthFeatureCondition(source FeatureSource, operator ComparisonOperator, threshold Length) (FeatureCondition, error) {
	if source.measure() != measureLength {
		return FeatureCondition{}, ErrInvalidFeatureCondition
	}
	condition := FeatureCondition{source: source, operator: operator, lengthThreshold: threshold}
	if !condition.valid() {
		return FeatureCondition{}, ErrInvalidFeatureCondition
	}
	return condition, nil
}

func NewVolumeFeatureCondition(source FeatureSource, operator ComparisonOperator, threshold Volume) (FeatureCondition, error) {
	if source.measure() != measureVolume {
		return FeatureCondition{}, ErrInvalidFeatureCondition
	}
	condition := FeatureCondition{source: source, operator: operator, volumeThreshold: threshold}
	if !condition.valid() {
		return FeatureCondition{}, ErrInvalidFeatureCondition
	}
	return condition, nil
}

func NewWeightFeatureCondition(source FeatureSource, operator ComparisonOperator, threshold Weight) (FeatureCondition, error) {
	if source.measure() != measureWeight {
		return FeatureCondition{}, ErrInvalidFeatureCondition
	}
	condition := FeatureCondition{source: source, operator: operator, weightThreshold: threshold}
	if !condition.valid() {
		return FeatureCondition{}, ErrInvalidFeatureCondition
	}
	return condition, nil
}

// NewCategoryFeatureCondition 声明一个类别相等条件。运算固定为 EQ——类别没有大小，
// 由调用方传次序运算是把「分区大于 5」这类无意义条款放进卡里，构造期拦下。
func NewCategoryFeatureCondition(source FeatureSource, expected CategoryValue) (FeatureCondition, error) {
	if source.measure() != measureCategory || !expected.valid() {
		return FeatureCondition{}, ErrInvalidFeatureCondition
	}
	return FeatureCondition{
		source:           source,
		operator:         ComparisonEquals,
		categoryExpected: expected,
	}, nil
}

func (condition FeatureCondition) Source() FeatureSource        { return condition.source }
func (condition FeatureCondition) Operator() ComparisonOperator { return condition.operator }

// ThresholdValue 与 ThresholdUnit 报出已声明的阈值，无论它度量的是哪种量，
// 使规范化与解释不必自己再按量纲分支。类别条件没有数值阈值，两者交回零值与空串，
// 期望取值从 ExpectedCategory 读。
func (condition FeatureCondition) ThresholdValue() Decimal {
	switch condition.source.measure() {
	case measureLength:
		return condition.lengthThreshold.value
	case measureVolume:
		return condition.volumeThreshold.value
	case measureWeight:
		return condition.weightThreshold.value
	default:
		return Decimal{}
	}
}

func (condition FeatureCondition) ThresholdUnit() string {
	switch condition.source.measure() {
	case measureLength:
		return condition.lengthThreshold.unit.String()
	case measureVolume:
		return condition.volumeThreshold.unit.String()
	case measureWeight:
		return condition.weightThreshold.unit.String()
	default:
		return ""
	}
}

// ExpectedCategory 只在类别条件上给出期望取值。
func (condition FeatureCondition) ExpectedCategory() (CategoryValue, bool) {
	return condition.categoryExpected, condition.source.measure() == measureCategory
}

func (condition FeatureCondition) describe() string {
	if expected, categorical := condition.ExpectedCategory(); categorical {
		return condition.source.String() + " " + condition.operator.String() + " " + expected.String()
	}
	return condition.source.String() + " " + condition.operator.String() + " " +
		condition.ThresholdValue().String() + " " + condition.ThresholdUnit()
}

func (condition FeatureCondition) Matches(features PackageFeatures) (bool, error) {
	if !condition.valid() || !features.valid() {
		return false, ErrInvalidFeatureCondition
	}
	var observed, threshold Decimal
	switch condition.source.measure() {
	case measureLength:
		value, err := features.length(condition.source)
		if err != nil {
			return false, err
		}
		if value.unit != condition.lengthThreshold.unit {
			return false, ErrLengthUnitMismatch
		}
		observed, threshold = value.value, condition.lengthThreshold.value
	case measureVolume:
		value, err := features.dimensions.Volume()
		if err != nil {
			return false, err
		}
		if value.unit != condition.volumeThreshold.unit {
			return false, ErrLengthUnitMismatch
		}
		observed, threshold = value.value, condition.volumeThreshold.value
	case measureWeight:
		value, err := features.weightOf(condition.source)
		if err != nil {
			return false, err
		}
		if value.unit != condition.weightThreshold.unit {
			return false, ErrWeightUnitMismatch
		}
		observed, threshold = value.value, condition.weightThreshold.value
	case measureCategory:
		return features.categoryMatches(condition.source, condition.categoryExpected)
	default:
		return false, ErrInvalidFeatureCondition
	}
	return condition.operator.holds(observed.Cmp(threshold))
}

func (condition FeatureCondition) valid() bool {
	if !condition.source.valid() || !condition.operator.valid() {
		return false
	}
	switch condition.source.measure() {
	case measureLength:
		return condition.operator.ordersNumbers() && condition.lengthThreshold.valid()
	case measureVolume:
		return condition.operator.ordersNumbers() && condition.volumeThreshold.valid()
	case measureWeight:
		return condition.operator.ordersNumbers() && condition.weightThreshold.valid()
	case measureCategory:
		return condition.operator == ComparisonEquals && condition.categoryExpected.valid()
	default:
		return false
	}
}
