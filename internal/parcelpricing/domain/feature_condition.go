package domain

// FeatureSource 是判定条件可以读取的可判定量的封闭集合。保持封闭，评价才能仅凭版本清单
// 重放每条规则的命中与未命中；集合一旦开放，就还需要规则正文本身。
//
// CONTEXT 声明的特征来源集合还包括体积重、计价重量、分区、地址类型和服务选项。它们在
// 这张卡上都不是阈值：前两个是条件最低计价重量抬高的对象，而不是任何判定条件读取的量；
// 后三个是类别量，需要的是与某个取值相等，而不是比较大小。它们会随需要它们的规则一起
// 进来。
type FeatureSource string

const (
	FeatureLongestSide       FeatureSource = "LONGEST_SIDE"
	FeatureSecondLongestSide FeatureSource = "SECOND_LONGEST_SIDE"
	FeatureLengthAndGirth    FeatureSource = "LENGTH_AND_GIRTH"
	FeatureVolume            FeatureSource = "VOLUME"
	FeatureActualWeight      FeatureSource = "ACTUAL_WEIGHT"
)

func (source FeatureSource) String() string { return string(source) }

func (source FeatureSource) valid() bool {
	return source.measure() != measureUnknown
}

// featureMeasure 是一个特征来源产出的物理量。阈值只有对着同一种量纲才有意义，所以配对
// 在这里检查一次，而不是在每次比较时都查。
type featureMeasure int

const (
	measureUnknown featureMeasure = iota
	measureLength
	measureVolume
	measureWeight
)

func (source FeatureSource) measure() featureMeasure {
	switch source {
	case FeatureLongestSide, FeatureSecondLongestSide, FeatureLengthAndGirth:
		return measureLength
	case FeatureVolume:
		return measureVolume
	case FeatureActualWeight:
		return measureWeight
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
type ComparisonOperator string

const (
	ComparisonGreaterThan        ComparisonOperator = "GT"
	ComparisonGreaterThanOrEqual ComparisonOperator = "GE"
	ComparisonLessThan           ComparisonOperator = "LT"
	ComparisonLessThanOrEqual    ComparisonOperator = "LE"
)

func (operator ComparisonOperator) String() string { return string(operator) }

func (operator ComparisonOperator) valid() bool {
	switch operator {
	case ComparisonGreaterThan, ComparisonGreaterThanOrEqual, ComparisonLessThan, ComparisonLessThanOrEqual:
		return true
	default:
		return false
	}
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

// PackageFeatures 保存为一次评价一次性派生出来的可判定量，使每条规则读到相同的值，
// 而不是各自从计价输入快照重新派生一遍。
type PackageFeatures struct {
	dimensions   Dimensions
	actualWeight Weight
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

func (features PackageFeatures) valid() bool {
	return features.dimensions.valid() && features.actualWeight.valid()
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

// FeatureCondition 是卡唯一需要的判定条件形状：一个特征、一个比较运算、一个阈值。
// 阈值由价卡版本声明，本包从不自带。
type FeatureCondition struct {
	source          FeatureSource
	operator        ComparisonOperator
	lengthThreshold Length
	volumeThreshold Volume
	weightThreshold Weight
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

func (condition FeatureCondition) Source() FeatureSource        { return condition.source }
func (condition FeatureCondition) Operator() ComparisonOperator { return condition.operator }

// ThresholdValue 与 ThresholdUnit 报出已声明的阈值，无论它度量的是哪种量，
// 使规范化与解释不必自己再按量纲分支。
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

func (condition FeatureCondition) describe() string {
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
		if features.actualWeight.unit != condition.weightThreshold.unit {
			return false, ErrWeightUnitMismatch
		}
		observed, threshold = features.actualWeight.value, condition.weightThreshold.value
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
		return condition.lengthThreshold.valid()
	case measureVolume:
		return condition.volumeThreshold.valid()
	case measureWeight:
		return condition.weightThreshold.valid()
	default:
		return false
	}
}
