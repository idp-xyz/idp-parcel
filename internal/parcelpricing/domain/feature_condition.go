package domain

// FeatureSource is the closed set of decidable quantities a condition may read.
// Keeping it closed is what lets an evaluation replay every rule's hit and miss
// from the version manifest alone; an open set would need the rule text itself.
//
// The set the CONTEXT declares also names 体积重, 计价重量, 分区, 地址类型 and
// 服务选项. None of them is a threshold on this card: the first two are what a
// conditional minimum weight raises rather than what any condition reads, and
// the last three are categorical, needing an equality against a value rather
// than a comparison. They arrive with the rules that need them.
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

// featureMeasure is the physical quantity a source produces. A threshold only
// means something against the same measure, so the pairing is checked once here
// rather than at every comparison.
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

// ComparisonOperator is the closed set of comparisons a condition may use. The
// card's conditions never need arithmetic, so no expression engine is offered.
type ComparisonOperator string

const ComparisonGreaterThan ComparisonOperator = "GT"

func (operator ComparisonOperator) String() string { return string(operator) }

func (operator ComparisonOperator) valid() bool {
	switch operator {
	case ComparisonGreaterThan:
		return true
	default:
		return false
	}
}

// PackageFeatures holds the decidable quantities derived once for an
// evaluation, so every rule reads the same values instead of each re-deriving
// them from the input snapshot.
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

// FeatureCondition is the only predicate shape the card needs: one feature, one
// comparison, one threshold. The threshold is declared by the rate card
// version, never carried by this package.
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

// ThresholdValue and ThresholdUnit report the declared threshold whatever it
// measures, so canonicalisation and explanations do not need to switch on the
// measure themselves.
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
	switch condition.operator {
	case ComparisonGreaterThan:
		return observed.Cmp(threshold) > 0, nil
	default:
		return false, ErrInvalidFeatureCondition
	}
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
