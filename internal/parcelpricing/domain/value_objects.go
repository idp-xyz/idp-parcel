package domain

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

type identifier struct {
	value string
}

func newIdentifier(name, value string) (identifier, error) {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
		return identifier{}, fmt.Errorf("%w: %s", ErrInvalidIdentifier, name)
	}
	return identifier{value: value}, nil
}

func (value identifier) String() string {
	return value.value
}

func (value identifier) valid() bool {
	return value.value != "" && strings.TrimSpace(value.value) == value.value
}

type TenantID struct{ identifier }

func NewTenantID(value string) (TenantID, error) {
	identifier, err := newIdentifier("tenant ID", value)
	return TenantID{identifier: identifier}, err
}

type PricingScopeID struct{ identifier }

func NewPricingScopeID(value string) (PricingScopeID, error) {
	identifier, err := newIdentifier("pricing scope ID", value)
	return PricingScopeID{identifier: identifier}, err
}

func NewPricingScope(value string) (PricingScopeID, error) {
	return NewPricingScopeID(value)
}

type BusinessScopeID = PricingScopeID

func NewBusinessScopeID(value string) (BusinessScopeID, error) {
	return NewPricingScopeID(value)
}

type PackageID struct{ identifier }

func NewPackageID(value string) (PackageID, error) {
	identifier, err := newIdentifier("package ID", value)
	return PackageID{identifier: identifier}, err
}

type PricingPlanID struct{ identifier }

func NewPricingPlanID(value string) (PricingPlanID, error) {
	identifier, err := newIdentifier("pricing plan ID", value)
	return PricingPlanID{identifier: identifier}, err
}

type RateTableID struct{ identifier }

func NewRateTableID(value string) (RateTableID, error) {
	identifier, err := newIdentifier("rate table ID", value)
	return RateTableID{identifier: identifier}, err
}

type RateEntryID struct{ identifier }

func NewRateEntryID(value string) (RateEntryID, error) {
	identifier, err := newIdentifier("rate entry ID", value)
	return RateEntryID{identifier: identifier}, err
}

type WeightPolicyID struct{ identifier }

func NewWeightPolicyID(value string) (WeightPolicyID, error) {
	identifier, err := newIdentifier("weight policy ID", value)
	return WeightPolicyID{identifier: identifier}, err
}

type EvaluationID struct{ identifier }

func NewEvaluationID(value string) (EvaluationID, error) {
	identifier, err := newIdentifier("evaluation ID", value)
	return EvaluationID{identifier: identifier}, err
}

type Currency struct {
	code string
}

type CurrencyCode = Currency

func NewCurrency(code string) (Currency, error) {
	if len(code) != 3 || code != strings.ToUpper(code) || !allLetters(code) {
		return Currency{}, ErrInvalidCurrency
	}
	return Currency{code: code}, nil
}

func (currency Currency) String() string {
	return currency.code
}

func (currency Currency) valid() bool {
	return len(currency.code) == 3 && currency.code == strings.ToUpper(currency.code) && allLetters(currency.code)
}

type ChargeCode struct {
	value string
}

var chargeCodePattern = regexp.MustCompile("^[A-Z][A-Z0-9_]*$")

func NewChargeCode(value string) (ChargeCode, error) {
	if !chargeCodePattern.MatchString(value) {
		return ChargeCode{}, ErrInvalidChargeCode
	}
	return ChargeCode{value: value}, nil
}

func (code ChargeCode) String() string { return code.value }

func (code ChargeCode) valid() bool {
	return chargeCodePattern.MatchString(code.value)
}

type WeightUnit string

const (
	WeightUnitGram     WeightUnit = "G"
	WeightUnitKilogram WeightUnit = "KG"
	WeightUnitOunce    WeightUnit = "OZ"
	WeightUnitPound    WeightUnit = "LB"
)

func NewWeightUnit(value string) (WeightUnit, error) {
	unit := WeightUnit(value)
	if !unit.valid() {
		return "", ErrInvalidWeightUnit
	}
	return unit, nil
}

func (unit WeightUnit) String() string {
	return string(unit)
}

func (unit WeightUnit) valid() bool {
	switch unit {
	case WeightUnitGram, WeightUnitKilogram, WeightUnitOunce, WeightUnitPound:
		return true
	default:
		return false
	}
}

type Weight struct {
	value Decimal
	unit  WeightUnit
}

func NewWeight(value Decimal, unit WeightUnit) (Weight, error) {
	if !value.valid() || value.IsNegative() || !unit.valid() {
		return Weight{}, ErrInvalidWeight
	}
	return Weight{value: value, unit: unit}, nil
}

func NewWeightFromString(value string, unit WeightUnit) (Weight, error) {
	decimal, err := ParseDecimal(value)
	if err != nil {
		return Weight{}, err
	}
	return NewWeight(decimal, unit)
}

func (weight Weight) Value() Decimal {
	return weight.value
}

func (weight Weight) Unit() WeightUnit {
	return weight.unit
}

func (weight Weight) valid() bool {
	return weight.value.valid() && !weight.value.IsNegative() && weight.unit.valid()
}

func (weight Weight) Compare(other Weight) (int, error) {
	if !weight.valid() || !other.valid() {
		return 0, ErrInvalidWeight
	}
	if weight.unit != other.unit {
		return 0, ErrWeightUnitMismatch
	}
	return weight.value.Cmp(other.value), nil
}

func (weight Weight) Equal(other Weight) bool {
	return weight.valid() && other.valid() && weight.unit == other.unit && weight.value.Equal(other.value)
}

type PricingDirection string

const (
	PricingDirectionBuy      PricingDirection = "BUY"
	PricingDirectionSell     PricingDirection = "SELL"
	PricingDirectionInternal PricingDirection = "INTERNAL"
)

func NewPricingDirection(value string) (PricingDirection, error) {
	direction := PricingDirection(value)
	if !direction.valid() {
		return "", ErrInvalidDirection
	}
	return direction, nil
}

func (direction PricingDirection) valid() bool {
	switch direction {
	case PricingDirectionBuy, PricingDirectionSell, PricingDirectionInternal:
		return true
	default:
		return false
	}
}

func (direction PricingDirection) String() string {
	return string(direction)
}

type PricingPurpose string

const (
	PricingPurposeCustomerCharge PricingPurpose = "CUSTOMER_CHARGE"
	PricingPurposeSupplierCost   PricingPurpose = "SUPPLIER_COST"
	PricingPurposeInternalPrice  PricingPurpose = "INTERNAL_PRICE"
)

func NewPricingPurpose(value string) (PricingPurpose, error) {
	purpose := PricingPurpose(value)
	if !purpose.valid() {
		return "", ErrInvalidPurpose
	}
	return purpose, nil
}

func (purpose PricingPurpose) valid() bool {
	return purpose.pairedDirection().valid()
}

// pairedDirection is the one direction this purpose may be declared with. The
// first release keeps the two axes one-to-one, so the purpose carries no
// information the direction does not already carry; it is reserved for telling
// same-direction evaluations apart once real parameters prove that is needed.
// Widening the axis means revisiting this pairing, not removing it silently.
func (purpose PricingPurpose) pairedDirection() PricingDirection {
	switch purpose {
	case PricingPurposeCustomerCharge:
		return PricingDirectionSell
	case PricingPurposeSupplierCost:
		return PricingDirectionBuy
	case PricingPurposeInternalPrice:
		return PricingDirectionInternal
	default:
		return ""
	}
}

func (purpose PricingPurpose) String() string {
	return string(purpose)
}

type AggregationMode string

const AggregationPerPackage AggregationMode = "PER_PACKAGE"

func (mode AggregationMode) valid() bool {
	return mode == AggregationPerPackage
}

type PricingWeightMethod string

const (
	PricingWeightActualOnly PricingWeightMethod = "ACTUAL_ONLY"
	PricingWeightMax        PricingWeightMethod = "MAX"
)

func (method PricingWeightMethod) valid() bool {
	return method == PricingWeightActualOnly || method == PricingWeightMax
}

type RoundingMode string

const (
	RoundingNone    RoundingMode = "NONE"
	RoundingCeiling RoundingMode = "CEILING"
)

func (mode RoundingMode) valid() bool {
	switch mode {
	case RoundingNone, RoundingCeiling:
		return true
	default:
		return false
	}
}

type ChargeEffect string

const (
	ChargeEffectAdd    ChargeEffect = "ADD"
	ChargeEffectDeduct ChargeEffect = "DEDUCT"
)

func (effect ChargeEffect) valid() bool {
	return effect == ChargeEffectAdd || effect == ChargeEffectDeduct
}

type ChargeScope string

const ChargeScopePackage ChargeScope = "PACKAGE"

func (scope ChargeScope) valid() bool {
	return scope == ChargeScopePackage
}

type ChargeBasis string

const (
	ChargeBasisRateEntry   ChargeBasis = "RATE_ENTRY"
	ChargeBasisFixedAmount ChargeBasis = "FIXED_AMOUNT"
)

func (basis ChargeBasis) valid() bool {
	return basis == ChargeBasisRateEntry || basis == ChargeBasisFixedAmount
}

// ChargeMethod is how one evaluation charge line's amount is produced. CONTEXT
// closes the set at four and scopes it to any charge line, so a declaring rule
// and the line it produces name the same method rather than each keeping a
// private vocabulary. Widening the set is a version content change.
type ChargeMethod string

const (
	ChargeMethodFixedAmount    ChargeMethod = "FIXED_AMOUNT"
	ChargeMethodTableLookup    ChargeMethod = "TABLE_LOOKUP"
	ChargeMethodPercentOfBasis ChargeMethod = "PERCENT_OF_BASIS"
	ChargeMethodGreaterOf      ChargeMethod = "GREATER_OF"
)

func (method ChargeMethod) String() string { return string(method) }

func (method ChargeMethod) valid() bool {
	switch method {
	case ChargeMethodFixedAmount, ChargeMethodTableLookup, ChargeMethodPercentOfBasis, ChargeMethodGreaterOf:
		return true
	default:
		return false
	}
}

type EvidenceKind string

const (
	EvidenceSynthetic  EvidenceKind = "S"
	EvidenceReplay     EvidenceKind = "R"
	EvidenceProduction EvidenceKind = "P"
)

func (kind EvidenceKind) valid() bool {
	switch kind {
	case EvidenceSynthetic, EvidenceReplay, EvidenceProduction:
		return true
	default:
		return false
	}
}

type ArtifactKind string

const (
	ArtifactPricingPlan     ArtifactKind = "pricing-plan"
	ArtifactRateTable       ArtifactKind = "rate-table"
	ArtifactWeightPolicy    ArtifactKind = "weight-policy"
	ArtifactReferenceSeries ArtifactKind = "reference-series"
	// A commercial policy version is what declares an exchange rate's quote
	// basis. Pricing references it; party-commercial owns it.
	ArtifactCommercialPolicy ArtifactKind = "commercial-policy"
	ArtifactNumericProfile   ArtifactKind = "numeric-profile"
)

func NumericProfileV1Reference() VersionReference {
	return VersionReference{
		kind:    ArtifactNumericProfile,
		id:      "decimal-bigint",
		version: "v1",
		digest:  "builtin:decimal-bigint-v1",
	}
}

type VersionReference struct {
	kind    ArtifactKind
	id      string
	version string
	digest  string
}

// EffectivePeriod uses [startsAt, endsAt). A zero endsAt means no upper bound.
type EffectivePeriod struct {
	startsAt time.Time
	endsAt   time.Time
}

func NewEffectivePeriod(startsAt, endsAt time.Time) (EffectivePeriod, error) {
	if startsAt.IsZero() || (!endsAt.IsZero() && !endsAt.After(startsAt)) {
		return EffectivePeriod{}, ErrInvalidEffectivePeriod
	}
	return EffectivePeriod{startsAt: startsAt.UTC(), endsAt: endsAt.UTC()}, nil
}

func NewUnboundedEffectivePeriod(startsAt time.Time) (EffectivePeriod, error) {
	return NewEffectivePeriod(startsAt, time.Time{})
}

func (period EffectivePeriod) StartsAt() time.Time { return period.startsAt }
func (period EffectivePeriod) EndsAt() time.Time   { return period.endsAt }

func (period EffectivePeriod) Contains(value time.Time) bool {
	if !period.valid() || value.IsZero() {
		return false
	}
	value = value.UTC()
	if value.Before(period.startsAt) {
		return false
	}
	return period.endsAt.IsZero() || value.Before(period.endsAt)
}

func (period EffectivePeriod) Within(outer EffectivePeriod) bool {
	if !period.valid() || !outer.valid() || period.startsAt.Before(outer.startsAt) {
		return false
	}
	if outer.endsAt.IsZero() {
		return true
	}
	return !period.endsAt.IsZero() && !period.endsAt.After(outer.endsAt)
}

func (period EffectivePeriod) valid() bool {
	return !period.startsAt.IsZero() && (period.endsAt.IsZero() || period.endsAt.After(period.startsAt))
}

func (period EffectivePeriod) canonicalString() string {
	if !period.valid() {
		return ""
	}
	end := "+INF"
	if !period.endsAt.IsZero() {
		end = period.endsAt.Format(time.RFC3339Nano)
	}
	return period.startsAt.Format(time.RFC3339Nano) + "|" + end
}

func NewVersionReference(kind ArtifactKind, id, version, digest string) (VersionReference, error) {
	if kind == "" || strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id || strings.TrimSpace(version) == "" || strings.TrimSpace(version) != version || strings.TrimSpace(digest) == "" || strings.TrimSpace(digest) != digest {
		return VersionReference{}, ErrInvalidVersionReference
	}
	return VersionReference{kind: kind, id: id, version: version, digest: digest}, nil
}

func (reference VersionReference) Kind() ArtifactKind { return reference.kind }
func (reference VersionReference) ID() string         { return reference.id }
func (reference VersionReference) Version() string    { return reference.version }
func (reference VersionReference) Digest() string     { return reference.digest }

func (reference VersionReference) valid() bool {
	return strings.TrimSpace(string(reference.kind)) != "" && strings.TrimSpace(reference.id) != "" && strings.TrimSpace(reference.id) == reference.id &&
		strings.TrimSpace(reference.version) != "" && strings.TrimSpace(reference.version) == reference.version &&
		strings.TrimSpace(reference.digest) != "" && strings.TrimSpace(reference.digest) == reference.digest
}

type VersionManifest struct {
	references []VersionReference
}

func NewVersionManifest(references []VersionReference) (VersionManifest, error) {
	if len(references) == 0 {
		return VersionManifest{}, ErrInvalidVersionReference
	}
	copyOfReferences := append([]VersionReference(nil), references...)
	type referenceIdentity struct {
		kind    ArtifactKind
		id      string
		version string
	}
	seen := make(map[referenceIdentity]struct{}, len(copyOfReferences))
	for _, reference := range copyOfReferences {
		if !reference.valid() {
			return VersionManifest{}, ErrInvalidVersionReference
		}
		key := referenceIdentity{kind: reference.kind, id: reference.id, version: reference.version}
		if _, exists := seen[key]; exists {
			return VersionManifest{}, ErrDuplicateVersionReference
		}
		seen[key] = struct{}{}
	}
	sort.Slice(copyOfReferences, func(left, right int) bool {
		return compareVersionReferences(copyOfReferences[left], copyOfReferences[right]) < 0
	})
	return VersionManifest{references: copyOfReferences}, nil
}

func compareVersionReferences(left, right VersionReference) int {
	for _, pair := range [][2]string{
		{string(left.kind), string(right.kind)},
		{left.id, right.id},
		{left.version, right.version},
		{left.digest, right.digest},
	} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}

func (manifest VersionManifest) References() []VersionReference {
	return append([]VersionReference(nil), manifest.references...)
}

func (manifest VersionManifest) valid() bool {
	if len(manifest.references) == 0 {
		return false
	}
	normalized, err := NewVersionManifest(manifest.references)
	return err == nil && manifest.Equal(normalized)
}

func (manifest VersionManifest) Equal(other VersionManifest) bool {
	if len(manifest.references) != len(other.references) {
		return false
	}
	for index, reference := range manifest.references {
		if reference != other.references[index] {
			return false
		}
	}
	return true
}

type Money struct {
	amount   Decimal
	currency Currency
}

func NewMoney(amount Decimal, currency Currency) (Money, error) {
	if !amount.valid() || amount.IsNegative() || !currency.valid() {
		return Money{}, ErrInvalidMoney
	}
	return Money{amount: amount, currency: currency}, nil
}

func NewMoneyFromString(amount string, currency Currency) (Money, error) {
	decimal, err := ParseDecimal(amount)
	if err != nil {
		return Money{}, err
	}
	return NewMoney(decimal, currency)
}

func (money Money) Amount() Decimal    { return money.amount }
func (money Money) Currency() Currency { return money.currency }

func (money Money) valid() bool {
	return money.amount.valid() && !money.amount.IsNegative() && money.currency.valid()
}

func (money Money) Add(other Money) (Money, error) {
	if !money.valid() || !other.valid() {
		return Money{}, ErrInvalidMoney
	}
	if money.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}
	amount, err := money.amount.Add(other.amount)
	if err != nil {
		return Money{}, err
	}
	return NewMoney(amount, money.currency)
}

func (money Money) Equal(other Money) bool {
	return money.valid() && other.valid() && money.currency == other.currency && money.amount.Equal(other.amount)
}

type VersionedFactReference struct {
	reference VersionReference
}

func NewVersionedFactReference(reference VersionReference) (VersionedFactReference, error) {
	if !reference.valid() {
		return VersionedFactReference{}, ErrInvalidVersionReference
	}
	return VersionedFactReference{reference: reference}, nil
}

func (reference VersionedFactReference) Reference() VersionReference {
	return reference.reference
}

func (reference VersionedFactReference) valid() bool {
	return reference.reference.valid()
}

func validBusinessTime(value time.Time) bool {
	return !value.IsZero()
}

func allLetters(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') {
			return false
		}
	}
	return true
}
