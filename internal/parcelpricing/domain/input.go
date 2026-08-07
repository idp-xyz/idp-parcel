package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// EvaluationSubjectKind separates an evaluation of a package the business has
// accepted from an estimate made before any package exists. Comparing several
// suppliers' cards happens before a customer commits, so the second kind is not
// a variant of the first — nothing downstream may turn an estimate into money.
type EvaluationSubjectKind string

const (
	SubjectAcceptedPackage EvaluationSubjectKind = "ACCEPTED_PACKAGE"
	SubjectEstimate        EvaluationSubjectKind = "ESTIMATE"
)

func (kind EvaluationSubjectKind) String() string { return string(kind) }

func (kind EvaluationSubjectKind) valid() bool {
	switch kind {
	case SubjectAcceptedPackage, SubjectEstimate:
		return true
	default:
		return false
	}
}

type EvaluationSubject struct {
	kind EvaluationSubjectKind
	id   string
}

func NewAcceptedPackageSubject(packageID PackageID) (EvaluationSubject, error) {
	if !packageID.valid() {
		return EvaluationSubject{}, ErrPricingInputInvalid
	}
	return EvaluationSubject{kind: SubjectAcceptedPackage, id: packageID.String()}, nil
}

// NewEstimateSubject names an object that only exists for the duration of one
// estimate. The reference is supplied by whoever asked for the estimate; this
// context does not mint identities.
func NewEstimateSubject(reference string) (EvaluationSubject, error) {
	if strings.TrimSpace(reference) == "" || strings.TrimSpace(reference) != reference {
		return EvaluationSubject{}, ErrPricingInputInvalid
	}
	return EvaluationSubject{kind: SubjectEstimate, id: reference}, nil
}

func (subject EvaluationSubject) Kind() EvaluationSubjectKind { return subject.kind }
func (subject EvaluationSubject) Reference() string           { return subject.id }

func (subject EvaluationSubject) valid() bool {
	return subject.kind.valid() && strings.TrimSpace(subject.id) != "" && strings.TrimSpace(subject.id) == subject.id
}

// PricingInputSnapshot carries the package's sides, not a volumetric weight
// worked out elsewhere. Volumetric weight is derived here from the card's own
// versioned divisor, so the same sides can only ever have one volumetric
// weight and a divisor change always reaches the content digest.
type PricingInputSnapshot struct {
	tenantID       TenantID
	scope          PricingScopeID
	subject        EvaluationSubject
	zone           string
	actualWeight   Weight
	dimensions     *Dimensions
	businessAt     time.Time
	factReferences []VersionedFactReference
	seriesValues   []ReferenceSeriesValue
}

// WithReferenceSeries returns a copy carrying the series readings resolved for
// this evaluation. It is a separate step from the constructor because the
// readings are looked up at the pricing base time, after the rest of the
// snapshot is already fixed.
func (input PricingInputSnapshot) WithReferenceSeries(values ...ReferenceSeriesValue) (PricingInputSnapshot, error) {
	if !input.valid() {
		return PricingInputSnapshot{}, ErrPricingInputInvalid
	}
	seen := make(map[ReferenceSeriesKind]struct{}, len(values))
	copyOfValues := make([]ReferenceSeriesValue, 0, len(values))
	for _, value := range values {
		if !value.valid() {
			return PricingInputSnapshot{}, ErrInvalidReferenceSeries
		}
		// Two readings of one series would leave the choice to iteration order.
		if _, exists := seen[value.kind]; exists {
			return PricingInputSnapshot{}, fmt.Errorf("%w: duplicate reading for %s", ErrInvalidReferenceSeries, value.kind)
		}
		seen[value.kind] = struct{}{}
		copyOfValues = append(copyOfValues, value)
	}
	sort.SliceStable(copyOfValues, func(left, right int) bool {
		return copyOfValues[left].kind < copyOfValues[right].kind
	})
	updated := copyInputSnapshot(input)
	updated.seriesValues = copyOfValues
	return updated, nil
}

// ReferenceSeriesValues reports the readings frozen into this snapshot.
func (input PricingInputSnapshot) ReferenceSeriesValues() []ReferenceSeriesValue {
	return append([]ReferenceSeriesValue(nil), input.seriesValues...)
}

func NewPricingInputSnapshot(
	tenantID TenantID,
	scope PricingScopeID,
	subject EvaluationSubject,
	zone string,
	actualWeight Weight,
	dimensions *Dimensions,
	businessAt time.Time,
	factReferences ...VersionedFactReference,
) (PricingInputSnapshot, error) {
	if !tenantID.valid() || !scope.valid() || !subject.valid() || strings.TrimSpace(zone) == "" || strings.TrimSpace(zone) != zone || !actualWeight.valid() || !validBusinessTime(businessAt) {
		return PricingInputSnapshot{}, ErrPricingInputInvalid
	}
	if dimensions != nil && !dimensions.valid() {
		return PricingInputSnapshot{}, ErrPricingInputInvalid
	}
	copyOfReferences := append([]VersionedFactReference(nil), factReferences...)
	for _, reference := range copyOfReferences {
		if !reference.valid() {
			return PricingInputSnapshot{}, ErrPricingInputInvalid
		}
	}
	var dimensionsCopy *Dimensions
	if dimensions != nil {
		copy := *dimensions
		dimensionsCopy = &copy
	}
	return PricingInputSnapshot{
		tenantID:       tenantID,
		scope:          scope,
		subject:        subject,
		zone:           zone,
		actualWeight:   actualWeight,
		dimensions:     dimensionsCopy,
		businessAt:     businessAt,
		factReferences: copyOfReferences,
	}, nil
}

func (input PricingInputSnapshot) TenantID() TenantID    { return input.tenantID }
func (input PricingInputSnapshot) Scope() PricingScopeID { return input.scope }
func (input PricingInputSnapshot) Zone() string          { return input.zone }

func (input PricingInputSnapshot) Subject() (EvaluationSubject, bool) {
	return input.subject, input.subject.valid()
}

// PackageID reports the accepted package this evaluation is for. An estimate
// has no package, so callers that turn evaluations into money must check the
// second return value rather than assume one exists.
func (input PricingInputSnapshot) PackageID() (PackageID, bool) {
	if input.subject.kind != SubjectAcceptedPackage {
		return PackageID{}, false
	}
	packageID, err := NewPackageID(input.subject.id)
	if err != nil {
		return PackageID{}, false
	}
	return packageID, true
}
func (input PricingInputSnapshot) ActualWeight() Weight  { return input.actualWeight }
func (input PricingInputSnapshot) BusinessAt() time.Time { return input.businessAt }
func (input PricingInputSnapshot) FactReferences() []VersionedFactReference {
	return append([]VersionedFactReference(nil), input.factReferences...)
}

func (input PricingInputSnapshot) Dimensions() (Dimensions, bool) {
	if input.dimensions == nil {
		return Dimensions{}, false
	}
	return *input.dimensions, true
}

// Features derives the decidable quantities once, so every rule in one
// evaluation reads the same values rather than each re-deriving them.
func (input PricingInputSnapshot) Features() (PackageFeatures, error) {
	if !input.valid() {
		return PackageFeatures{}, ErrPricingInputInvalid
	}
	sides, declared := input.Dimensions()
	if !declared {
		return PackageFeatures{}, ErrMissingDimensions
	}
	return NewPackageFeatures(sides, input.actualWeight)
}

func (input PricingInputSnapshot) valid() bool {
	if !input.tenantID.valid() || !input.scope.valid() || !input.subject.valid() || strings.TrimSpace(input.zone) == "" || strings.TrimSpace(input.zone) != input.zone || !input.actualWeight.valid() || !validBusinessTime(input.businessAt) {
		return false
	}
	if input.dimensions != nil && !input.dimensions.valid() {
		return false
	}
	for _, reference := range input.factReferences {
		if !reference.valid() {
			return false
		}
	}
	return true
}

type PricingWeightResult struct {
	method       PricingWeightMethod
	actual       Weight
	volumetric   *Weight
	raw          Weight
	rounded      Weight
	roundingMode RoundingMode
	increment    Weight
	explanation  string
}

// CalculatePricingWeight derives the weight, raises it to any floor the plan's
// conditional minimums imposed, and only then rounds. CONTEXT fixes that order
// — 派生、抬高、再进位 — and it is observable: a 1.2 KG floor under a whole-
// kilogram ceiling bills 2 KG, whereas raising after rounding would bill 1.2,
// which is not a whole increment at all.
func CalculatePricingWeight(input PricingInputSnapshot, policy PricingWeightPolicy, expectedUnit WeightUnit, floors ...Weight) (PricingWeightResult, error) {
	if !input.valid() || !policy.valid() {
		return PricingWeightResult{}, ErrPricingInputInvalid
	}
	if input.actualWeight.unit != expectedUnit {
		return PricingWeightResult{}, ErrWeightUnitMismatch
	}
	if policy.rounding.unit() != expectedUnit {
		return PricingWeightResult{}, ErrWeightUnitMismatch
	}
	var raw Weight
	var volumetric Weight
	hasVolumetric := false
	switch policy.method {
	case PricingWeightActualOnly:
		raw = input.actualWeight
	case PricingWeightMax:
		derived, err := deriveVolumetricWeight(input, policy)
		if err != nil {
			return PricingWeightResult{}, err
		}
		volumetric, hasVolumetric = derived, true
		if volumetric.unit != expectedUnit {
			return PricingWeightResult{}, ErrWeightUnitMismatch
		}
		comparison := input.actualWeight.value.Cmp(volumetric.value)
		if comparison >= 0 {
			raw = input.actualWeight
		} else {
			raw = volumetric
		}
	default:
		return PricingWeightResult{}, ErrPricingInputInvalid
	}
	derived := raw
	raised := false
	for _, floor := range floors {
		if !floor.valid() || floor.unit != raw.unit {
			return PricingWeightResult{}, ErrWeightUnitMismatch
		}
		if floor.value.Cmp(raw.value) > 0 {
			raw, raised = floor, true
		}
	}
	rounded, segment, err := policy.rounding.Apply(raw)
	if err != nil {
		return PricingWeightResult{}, err
	}
	// CONTEXT requires both sides of a raise in the explanation, so a reader can
	// see the billed weight was not the parcel's own.
	raiseText := ""
	if raised {
		raiseText = fmt.Sprintf("; raised from %s %s to %s %s by a conditional minimum", derived.value.String(), derived.unit, raw.value.String(), raw.unit)
	}
	explanation := fmt.Sprintf("pricing weight uses %s; raw=%s %s%s; rounding=%s increment=%s %s%s; rounded=%s %s", policy.method, derived.value.String(), derived.unit, raiseText, segment.mode, segment.increment.value.String(), segment.increment.unit, roundingSegmentScope(segment), rounded.value.String(), rounded.unit)
	return PricingWeightResult{
		method:       policy.method,
		actual:       input.actualWeight,
		volumetric:   optionalWeightCopy(volumetric, hasVolumetric),
		raw:          raw,
		rounded:      rounded,
		roundingMode: segment.mode,
		increment:    segment.increment,
		explanation:  explanation,
	}, nil
}

func (result PricingWeightResult) Method() PricingWeightMethod { return result.method }
func (result PricingWeightResult) ActualWeight() Weight        { return result.actual }
func (result PricingWeightResult) RawWeight() Weight           { return result.raw }
func (result PricingWeightResult) RoundedWeight() Weight       { return result.rounded }
func (result PricingWeightResult) RoundingMode() RoundingMode  { return result.roundingMode }
func (result PricingWeightResult) RoundingIncrement() Weight   { return result.increment }
func (result PricingWeightResult) Explanation() string         { return result.explanation }

func (result PricingWeightResult) VolumetricWeight() (Weight, bool) {
	if result.volumetric == nil {
		return Weight{}, false
	}
	return *result.volumetric, true
}

func (result PricingWeightResult) valid() bool {
	if !result.method.valid() || !result.actual.valid() || !result.raw.valid() || !result.rounded.valid() || !result.roundingMode.valid() || !result.increment.valid() || result.increment.value.Sign() <= 0 || result.increment.unit != result.actual.unit || strings.TrimSpace(result.explanation) == "" {
		return false
	}
	if result.volumetric != nil && (!result.volumetric.valid() || result.volumetric.unit != result.actual.unit) {
		return false
	}
	return true
}

// deriveVolumetricWeight applies the card's declared divisor to the package's
// sides. Sides that never arrived are a missing fact that can still turn up, so
// the caller keeps the evaluation waiting instead of falling back to the actual
// weight and quietly pricing a different package.
func deriveVolumetricWeight(input PricingInputSnapshot, policy PricingWeightPolicy) (Weight, error) {
	factor, declared := policy.VolumetricFactor()
	if !declared {
		return Weight{}, ErrInvalidRoundingPolicy
	}
	sides, measured := input.Dimensions()
	if !measured {
		return Weight{}, ErrMissingDimensions
	}
	volume, err := sides.Volume()
	if err != nil {
		return Weight{}, err
	}
	return factor.Apply(volume)
}

func optionalWeightCopy(value Weight, present bool) *Weight {
	if !present {
		return nil
	}
	copy := value
	return &copy
}
