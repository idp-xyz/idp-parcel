package domain

// FeatureSource is the closed set of decidable quantities a condition may read.
// Keeping it closed is what lets an evaluation replay every rule's hit and miss
// from the version manifest alone; an open set would need the rule text itself.
type FeatureSource string

const FeatureLongestSide FeatureSource = "LONGEST_SIDE"

func (source FeatureSource) String() string { return string(source) }

func (source FeatureSource) valid() bool {
	switch source {
	case FeatureLongestSide:
		return true
	default:
		return false
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
	dimensions Dimensions
}

func NewPackageFeatures(dimensions Dimensions) (PackageFeatures, error) {
	if !dimensions.valid() {
		return PackageFeatures{}, ErrInvalidDimensions
	}
	return PackageFeatures{dimensions: dimensions}, nil
}

func (features PackageFeatures) valid() bool {
	return features.dimensions.valid()
}

func (features PackageFeatures) length(source FeatureSource) (Length, error) {
	switch source {
	case FeatureLongestSide:
		return features.dimensions.LongestSide(), nil
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
}

func NewLengthFeatureCondition(source FeatureSource, operator ComparisonOperator, threshold Length) (FeatureCondition, error) {
	condition := FeatureCondition{source: source, operator: operator, lengthThreshold: threshold}
	if !condition.valid() {
		return FeatureCondition{}, ErrInvalidFeatureCondition
	}
	return condition, nil
}

func (condition FeatureCondition) Source() FeatureSource        { return condition.source }
func (condition FeatureCondition) Operator() ComparisonOperator { return condition.operator }

func (condition FeatureCondition) Matches(features PackageFeatures) (bool, error) {
	if !condition.valid() || !features.valid() {
		return false, ErrInvalidFeatureCondition
	}
	value, err := features.length(condition.source)
	if err != nil {
		return false, err
	}
	if value.Unit() != condition.lengthThreshold.Unit() {
		return false, ErrLengthUnitMismatch
	}
	switch condition.operator {
	case ComparisonGreaterThan:
		return value.Value().Cmp(condition.lengthThreshold.Value()) > 0, nil
	default:
		return false, ErrInvalidFeatureCondition
	}
}

func (condition FeatureCondition) valid() bool {
	return condition.source.valid() && condition.operator.valid() && condition.lengthThreshold.valid()
}
