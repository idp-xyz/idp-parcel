package domain

import (
	"errors"
	"math/big"
	"regexp"
	"sort"
	"strings"

	"go.idp.xyz/idp-parcel/internal/platform/refdata"
)

var (
	// ErrInvalidWeightDiscrepancyGateRule 是规则本身不成立，或拿零值 /「不构成前置条件」来判。
	// 未登记不能被读成一条零容差规则，否则没配容差的租户会被当成「必须分毫不差」或「这一道已满足」。
	ErrInvalidWeightDiscrepancyGateRule = errors.New("customs compliance: invalid weight discrepancy gate rule")
	// ErrWeightDiscrepancyUnitUnconvertible 是成员固定毛重或当前实测毛重换不到规则单位：单位不在重量目录里，或数值不是十进制有理数。
	ErrWeightDiscrepancyUnitUnconvertible = errors.New("customs compliance: weight discrepancy unit is not convertible")
)

// 非负十进制：只允许 digits 或 digits.digits。正负号、空白、分数、指数都不是容差，也不是能拿来比的毛重。
var nonNegativeDecimal = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// WeightDiscrepancyPrecondition 是「实测与提交版本毛重差异」那一道前置条件在门禁清单里的引用。
// 登记方给这一道登的是规则而不是认定，认定登记口对这个引用关门的理由与 DutyPaymentPrecondition 同一句。
var WeightDiscrepancyPrecondition = PreconditionReference{requiredValue{value: "WEIGHT_DISCREPANCY"}}

// WeightDiscrepancyGateRule 是规则正文，封闭两形之一：「实测与提交版本的毛重差异不构成本动作在本边界的前置条件」；
// 或一条非负容差加一个重量单位。容差的数值与单位是租户取值，这里不拟任何生产默认。
type WeightDiscrepancyGateRule struct {
	notAPrecondition bool
	maxDelta         string
	unit             string
}

// WeightDiscrepancyNotAPrecondition 形成「不构成前置条件」那一形：这一道不读实测、不比容差。
func WeightDiscrepancyNotAPrecondition() WeightDiscrepancyGateRule {
	return WeightDiscrepancyGateRule{notAPrecondition: true}
}

// AllowWeightDeltaUpTo 形成容差那一形。maxDelta 收成唯一十进制拼写，同值重登在读口与库列上对得上。
// 零容差（"0" / "0.00"）成立，意思是要求分毫不差，不是「这一道不存在」。
func AllowWeightDeltaUpTo(maxDelta, unit string) (WeightDiscrepancyGateRule, error) {
	canonical, ok := canonicalNonNegativeDecimal(maxDelta)
	if !ok {
		return WeightDiscrepancyGateRule{}, ErrInvalidWeightDiscrepancyGateRule
	}
	info, found := refdata.Unit(unit)
	if !found || info.Category != refdata.UnitCategoryWeight {
		return WeightDiscrepancyGateRule{}, ErrInvalidWeightDiscrepancyGateRule
	}
	return WeightDiscrepancyGateRule{maxDelta: canonical, unit: info.Code}, nil
}

func (rule WeightDiscrepancyGateRule) valid() bool {
	if rule.notAPrecondition {
		return rule.maxDelta == "" && rule.unit == ""
	}
	_, ok := canonicalNonNegativeDecimal(rule.maxDelta)
	info, found := refdata.Unit(rule.unit)
	return ok && found && info.Category == refdata.UnitCategoryWeight && rule.maxDelta == canonicalText(rule.maxDelta)
}

func canonicalText(raw string) string {
	text, _ := canonicalNonNegativeDecimal(raw)
	return text
}

// NotAPrecondition 报告规则是不是「不构成前置条件」那一形。
func (rule WeightDiscrepancyGateRule) NotAPrecondition() bool {
	return rule.notAPrecondition
}

// MaxDelta 交回规范化容差与重量单位。不构成前置条件或零值时 ok 为假，不把空串冒充成一条容差。
func (rule WeightDiscrepancyGateRule) MaxDelta() (maxDelta string, unit string, ok bool) {
	if !rule.valid() || rule.notAPrecondition {
		return "", "", false
	}
	return rule.maxDelta, rule.unit, true
}

// WeightDiscrepancyMemberActual 是某成员此刻已确知存在的实测毛重。没有实测、定不下、读不动停在编排，不进 Judge。
type WeightDiscrepancyMemberActual struct {
	GrossWeight       string
	GrossWeightUnit   string
	MeasurementSource string
}

// WeightDiscrepancyMemberReading 是读数里逐成员的一格。成版时没有固定毛重的，申报三格为空串。
type WeightDiscrepancyMemberReading struct {
	Parcel            DeclaredParcelReference
	DeclaredWeight    string
	DeclaredUnit      string
	DeclaredSource    string
	MeasuredWeight    string
	MeasuredUnit      string
	MeasurementSource string
	WithinTolerance   bool
}

// WeightDiscrepancyGateReading 是门禁记录上「毛重差异」那一道的读数。State 只有满足或未满足。
type WeightDiscrepancyGateReading struct {
	State             PreconditionState
	SubmissionVersion SubmissionVersionID
	MaxDelta          string
	Unit              string
	Members           []WeightDiscrepancyMemberReading
}

func (reading WeightDiscrepancyGateReading) valid() bool {
	if reading.State != PreconditionMet && reading.State != PreconditionUnmet {
		return false
	}
	if !reading.SubmissionVersion.valid() || len(reading.Members) == 0 {
		return false
	}
	if _, ok := canonicalNonNegativeDecimal(reading.MaxDelta); !ok || reading.MaxDelta != canonicalText(reading.MaxDelta) {
		return false
	}
	unit, found := refdata.Unit(reading.Unit)
	if !found || unit.Category != refdata.UnitCategoryWeight {
		return false
	}
	for _, member := range reading.Members {
		if !member.valid(reading.Unit) {
			return false
		}
	}
	return true
}

func (member WeightDiscrepancyMemberReading) valid(ruleUnit string) bool {
	if !member.Parcel.valid() ||
		strings.TrimSpace(member.MeasuredWeight) == "" ||
		strings.TrimSpace(member.MeasuredUnit) == "" ||
		strings.TrimSpace(member.MeasurementSource) == "" {
		return false
	}
	declaredBlank := member.DeclaredWeight == "" && member.DeclaredUnit == "" && member.DeclaredSource == ""
	declaredFull := member.DeclaredWeight != "" && member.DeclaredUnit != "" && member.DeclaredSource != ""
	if !declaredBlank && !declaredFull {
		return false
	}
	if _, err := weightInUnit(member.MeasuredWeight, member.MeasuredUnit, ruleUnit); err != nil {
		return false
	}
	if declaredFull {
		if _, err := weightInUnit(member.DeclaredWeight, member.DeclaredUnit, ruleUnit); err != nil {
			return false
		}
	}
	return true
}

// Judge 按版本固定下来的成员逐件比较此刻实测与成版时的毛重。成员清单在版本上（Members），
// 不在 DeclarationUnitID 上——单元身份没有组成。容差边界取小于等于：差得恰好等于容差算满足。
// 成版时没有固定毛重的成员，提交版本不可覆盖，重称不会把毛重补进那一版，这一件不在容差内。
func (rule WeightDiscrepancyGateRule) Judge(
	version CustomsSubmissionVersion,
	actuals map[DeclaredParcelReference]WeightDiscrepancyMemberActual,
) (WeightDiscrepancyGateReading, error) {
	if !rule.valid() || rule.notAPrecondition {
		return WeightDiscrepancyGateReading{}, ErrInvalidWeightDiscrepancyGateRule
	}
	members := version.Members()
	if !version.ID().valid() || len(members) == 0 {
		return WeightDiscrepancyGateReading{}, ErrInvalidWeightDiscrepancyGateRule
	}
	sort.Slice(members, func(i, j int) bool {
		return members[i].String() < members[j].String()
	})
	seen := make(map[DeclaredParcelReference]bool, len(members))
	for _, member := range members {
		if !member.valid() || seen[member] {
			return WeightDiscrepancyGateReading{}, ErrInvalidWeightDiscrepancyGateRule
		}
		seen[member] = true
	}
	limit, ok := new(big.Rat).SetString(rule.maxDelta)
	if !ok {
		return WeightDiscrepancyGateReading{}, ErrInvalidWeightDiscrepancyGateRule
	}
	fixed := make(map[DeclaredParcelReference]MemberMeasuredWeight, len(version.MemberWeights()))
	for _, weight := range version.MemberWeights() {
		fixed[weight.Parcel()] = weight
	}

	readings := make([]WeightDiscrepancyMemberReading, 0, len(members))
	allWithin := true
	for _, member := range members {
		actual, found := actuals[member]
		if !found || blankWeightFact(actual.GrossWeight) || blankWeightFact(actual.GrossWeightUnit) || blankWeightFact(actual.MeasurementSource) {
			return WeightDiscrepancyGateReading{}, ErrInvalidWeightDiscrepancyGateRule
		}
		if _, err := weightInUnit(actual.GrossWeight, actual.GrossWeightUnit, rule.unit); err != nil {
			return WeightDiscrepancyGateReading{}, err
		}
		row := WeightDiscrepancyMemberReading{
			Parcel:            member,
			MeasuredWeight:    actual.GrossWeight,
			MeasuredUnit:      actual.GrossWeightUnit,
			MeasurementSource: actual.MeasurementSource,
		}
		declared, hasDeclared := fixed[member]
		if !hasDeclared {
			// 成版时没称到的成员，提交版本里长不出固定毛重。此刻有了实测也不把这一件说成在容差内。
			row.WithinTolerance = false
			allWithin = false
			readings = append(readings, row)
			continue
		}
		measured, err := weightInUnit(actual.GrossWeight, actual.GrossWeightUnit, rule.unit)
		if err != nil {
			return WeightDiscrepancyGateReading{}, err
		}
		declaredInRule, err := weightInUnit(declared.GrossWeight(), declared.GrossWeightUnit(), rule.unit)
		if err != nil {
			return WeightDiscrepancyGateReading{}, err
		}
		delta := new(big.Rat).Sub(measured, declaredInRule)
		delta.Abs(delta)
		row.DeclaredWeight = declared.GrossWeight()
		row.DeclaredUnit = declared.GrossWeightUnit()
		row.DeclaredSource = declared.MeasurementSource()
		row.WithinTolerance = delta.Cmp(limit) <= 0
		if !row.WithinTolerance {
			allWithin = false
		}
		readings = append(readings, row)
	}
	state := PreconditionUnmet
	if allWithin {
		state = PreconditionMet
	}
	return WeightDiscrepancyGateReading{
		State:             state,
		SubmissionVersion: version.ID(),
		MaxDelta:          rule.maxDelta,
		Unit:              rule.unit,
		Members:           readings,
	}, nil
}

func blankWeightFact(value string) bool {
	return strings.TrimSpace(value) == ""
}

// canonicalNonNegativeDecimal 收下非负十进制并收成唯一拼写：去掉小数尾零与悬空小数点，前导零归一成一个 0。
func canonicalNonNegativeDecimal(raw string) (string, bool) {
	if !nonNegativeDecimal.MatchString(raw) {
		return "", false
	}
	if _, ok := new(big.Rat).SetString(raw); !ok {
		return "", false
	}
	parts := strings.SplitN(raw, ".", 2)
	integer := strings.TrimLeft(parts[0], "0")
	if integer == "" {
		integer = "0"
	}
	if len(parts) == 1 {
		return integer, true
	}
	fraction := strings.TrimRight(parts[1], "0")
	if fraction == "" {
		return integer, true
	}
	return integer + "." + fraction, true
}

func weightInUnit(raw, from, to string) (*big.Rat, error) {
	if !nonNegativeDecimal.MatchString(raw) {
		return nil, ErrWeightDiscrepancyUnitUnconvertible
	}
	value, ok := new(big.Rat).SetString(raw)
	if !ok {
		return nil, ErrWeightDiscrepancyUnitUnconvertible
	}
	info, found := refdata.Unit(from)
	if !found || info.Category != refdata.UnitCategoryWeight {
		return nil, ErrWeightDiscrepancyUnitUnconvertible
	}
	converted, err := refdata.Convert(value, from, to)
	if err != nil {
		return nil, ErrWeightDiscrepancyUnitUnconvertible
	}
	return converted, nil
}

func cloneWeightDiscrepancyReading(reading WeightDiscrepancyGateReading) WeightDiscrepancyGateReading {
	reading.Members = append([]WeightDiscrepancyMemberReading(nil), reading.Members...)
	return reading
}

// WithWeightDiscrepancy 交回挂上毛重差异读数的一份门禁判断副本。读数只能挂在清单含这一道的判断上。
func (gate ReleaseGateVerification) WithWeightDiscrepancy(reading WeightDiscrepancyGateReading) (ReleaseGateVerification, error) {
	if !reading.valid() || !contains(gate.preconditions, WeightDiscrepancyPrecondition) {
		return ReleaseGateVerification{}, ErrInvalidGateVerification
	}
	attached := gate
	attached.preconditions = append([]PreconditionReference(nil), gate.preconditions...)
	attached.dutyPayment = gate.dutyPayment
	attached.weightDiscrepancy = cloneWeightDiscrepancyReading(reading)
	attached.hasWeightDiscrepancy = true
	return attached, nil
}

// WeightDiscrepancy 交回毛重差异那一道的读数副本。没挂即这一道不是规则判出来的，或是加读数之前的旧版。
func (gate ReleaseGateVerification) WeightDiscrepancy() (WeightDiscrepancyGateReading, bool) {
	if !gate.hasWeightDiscrepancy {
		return WeightDiscrepancyGateReading{}, false
	}
	return cloneWeightDiscrepancyReading(gate.weightDiscrepancy), true
}
