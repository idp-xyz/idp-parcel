package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"sort"
	"time"
)

// WarehousePlanCanonicalization 是仓储计费方案的规范化版本。它不是基础版，登记要过 ADR-0352 的门。
const WarehousePlanCanonicalization = "PPC-8"

// WarehousePlan 是逐仓储发生项的方案。它没有重量价表。规则、单价与未命中行为写在卡上，是租户取值。
type WarehousePlan struct {
	ID            string
	Version       string
	Scope         string
	Currency      string
	DeclaredValue string
	EffectiveFrom time.Time
	Groups        []WarehouseChargeGroup
	Rules         []WarehouseChargeRule
	digest        string
}

// WarehouseChargeGroup 是一个互斥组。同一组在同一个层面实例上至多计收一条。
type WarehouseChargeGroup struct {
	Name   string
	Kind   string
	Level  string
	Line   string
	OnMiss string
}

// WarehouseChargeRule 是一条计量费用规则。收法只有按计量计费与免收。
type WarehouseChargeRule struct {
	Code         string
	Kind         string
	Level        string
	Line         string
	Group        string
	Priority     int
	Method       string
	Unit         string
	UnitQuantity string
	UnitPrice    int64
	BaseFee      int64
	Allowance    string
	Rounding     string
	When         []WarehouseCondition
}

// WarehouseCondition 是作业属性相等。空条件是无条件。
type WarehouseCondition struct {
	Name   string
	Equals string
}

// WarehouseOccurrenceSnapshot 是计价读到的一版发生项。身份归仓配履约，这里只留下算价要的内容。
type WarehouseOccurrenceSnapshot struct {
	ID         string
	Version    int
	Kind       string
	Owner      string
	Warehouse  string
	Occurred   time.Time
	Correction string
	Measures   []WarehouseMeasure
	Attributes []WarehouseAttribute
	Lines      []WarehouseOccurrenceLine
}

// WarehouseMeasure 是一个计量。未测量可以另附货主声明值。
type WarehouseMeasure struct {
	Unit       string
	Quantity   string
	Unmeasured bool
	Declared   string
}

// WarehouseAttribute 是一个作业属性。
type WarehouseAttribute struct {
	Name  string
	Value string
}

// WarehouseOccurrenceLine 是一行明细。
type WarehouseOccurrenceLine struct {
	Kind       string
	Measures   []WarehouseMeasure
	Attributes []WarehouseAttribute
}

// WarehouseEvaluation 是一次仓储计费方案的评价。合计为零也是已完成。
type WarehouseEvaluation struct {
	Status     string
	TotalMinor int64
	Currency   string
	Codes      []string
	Reason     string
}

// NewWarehousePlan 校验并记下内容摘要。形状不立时交回空方案与原因。
func NewWarehousePlan(plan WarehousePlan) (WarehousePlan, string) {
	if plan.ID == "" || plan.Version == "" || plan.Scope == "" || plan.Currency == "" || plan.EffectiveFrom.IsZero() {
		return WarehousePlan{}, "CONTENT_INVALID"
	}
	if plan.DeclaredValue != "USE" && plan.DeclaredValue != "REFUSE" {
		return WarehousePlan{}, "CONTENT_INVALID"
	}
	groups := map[string]WarehouseChargeGroup{}
	for _, group := range plan.Groups {
		if group.Name == "" || group.Kind == "" || (group.Level != "OCCURRENCE" && group.Level != "LINE") ||
			(group.OnMiss != "PENDING" && group.OnMiss != "NOT_CHARGED") {
			return WarehousePlan{}, "CONTENT_INVALID"
		}
		if group.Level == "LINE" && group.Line == "" {
			return WarehousePlan{}, "CONTENT_INVALID"
		}
		if _, exists := groups[group.Name]; exists {
			return WarehousePlan{}, "CONTENT_INVALID"
		}
		groups[group.Name] = group
	}
	seenCodes := map[string]struct{}{}
	for _, rule := range plan.Rules {
		group, ok := groups[rule.Group]
		if !ok || rule.Code == "" || rule.Kind != group.Kind || rule.Level != group.Level || rule.Line != group.Line {
			return WarehousePlan{}, "CONTENT_INVALID"
		}
		if _, exists := seenCodes[rule.Code]; exists {
			return WarehousePlan{}, "CONTENT_INVALID"
		}
		seenCodes[rule.Code] = struct{}{}
		if rule.Method != "CHARGE" && rule.Method != "WAIVE" {
			return WarehousePlan{}, "CONTENT_INVALID"
		}
		if rule.Method == "CHARGE" && (rule.Unit == "" || rule.UnitQuantity == "" || (rule.Rounding != "EXACT" && rule.Rounding != "UP")) {
			return WarehousePlan{}, "CONTENT_INVALID"
		}
		if rule.Method == "CHARGE" && rule.Rounding == "EXACT" && rule.UnitQuantity != "1" {
			return WarehousePlan{}, "CONTENT_INVALID"
		}
	}
	sort.Slice(plan.Groups, func(i, j int) bool { return plan.Groups[i].Name < plan.Groups[j].Name })
	sort.Slice(plan.Rules, func(i, j int) bool { return plan.Rules[i].Code < plan.Rules[j].Code })
	raw, err := json.Marshal(plan)
	if err != nil {
		return WarehousePlan{}, "CONTENT_INVALID"
	}
	sum := sha256.Sum256(raw)
	plan.digest = hex.EncodeToString(sum[:])
	return plan, ""
}

func (plan WarehousePlan) Digest() string { return plan.digest }

// EvaluateWarehousePlan 按计量费用规则评价一版发生项。方案没有这种作业的规则时待判断。
func EvaluateWarehousePlan(plan WarehousePlan, occurrence WarehouseOccurrenceSnapshot) WarehouseEvaluation {
	evaluation := WarehouseEvaluation{Status: "PENDING", Currency: plan.Currency, Reason: "NO_RULE"}
	if occurrence.Kind == "" {
		evaluation.Reason = "CONTENT_INVALID"
		return evaluation
	}
	groups := groupsFor(plan, occurrence.Kind)
	if len(groups) == 0 {
		return evaluation
	}
	var total int64
	var codes []string
	for _, group := range groups {
		instances := instancesOf(occurrence, group)
		if len(instances) == 0 {
			continue
		}
		for _, instance := range instances {
			amount, code, status, reason := chargeInstance(plan, group, instance, occurrence.Attributes)
			if status != "" {
				return WarehouseEvaluation{Status: status, Currency: plan.Currency, Reason: reason}
			}
			if code == "" {
				continue
			}
			total += amount
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		return WarehouseEvaluation{Status: "ZERO", Currency: plan.Currency}
	}
	sort.Strings(codes)
	return WarehouseEvaluation{Status: "COMPLETED", TotalMinor: total, Currency: plan.Currency, Codes: codes}
}

type chargeInstanceView struct {
	lineKind   string
	measures   []WarehouseMeasure
	attributes []WarehouseAttribute
}

func groupsFor(plan WarehousePlan, kind string) []WarehouseChargeGroup {
	var groups []WarehouseChargeGroup
	for _, group := range plan.Groups {
		if group.Kind == kind {
			groups = append(groups, group)
		}
	}
	return groups
}

func instancesOf(occurrence WarehouseOccurrenceSnapshot, group WarehouseChargeGroup) []chargeInstanceView {
	if group.Level == "OCCURRENCE" {
		return []chargeInstanceView{{measures: occurrence.Measures, attributes: occurrence.Attributes}}
	}
	var instances []chargeInstanceView
	for _, line := range occurrence.Lines {
		if line.Kind == group.Line {
			instances = append(instances, chargeInstanceView{lineKind: line.Kind, measures: line.Measures, attributes: line.Attributes})
		}
	}
	return instances
}

func chargeInstance(plan WarehousePlan, group WarehouseChargeGroup, instance chargeInstanceView, shared []WarehouseAttribute) (int64, string, string, string) {
	var winners []WarehouseChargeRule
	best := 0
	seen := false
	for _, rule := range plan.Rules {
		if rule.Group != group.Name || !conditionsHold(rule.When, instance.attributes, shared) {
			continue
		}
		if !seen || rule.Priority > best {
			winners = []WarehouseChargeRule{rule}
			best = rule.Priority
			seen = true
			continue
		}
		if rule.Priority == best {
			winners = append(winners, rule)
		}
	}
	if len(winners) == 0 {
		if group.OnMiss == "PENDING" {
			return 0, "", "PENDING", "MISS:" + group.Name
		}
		return 0, "", "", ""
	}
	if len(winners) > 1 {
		return 0, "", "CONFLICT", "GROUP:" + group.Name
	}
	rule := winners[0]
	if rule.Method == "WAIVE" {
		return 0, "", "", ""
	}
	quantity, status, reason := measureQuantity(plan, rule.Unit, instance.measures)
	if status != "" {
		return 0, "", status, reason
	}
	amount, ok := chargeAmount(rule, quantity)
	if !ok {
		return 0, "", "CONFLICT", "AMOUNT:" + rule.Code
	}
	if amount == 0 {
		return 0, "", "", ""
	}
	return amount, rule.Code, "", ""
}

func conditionsHold(conditions []WarehouseCondition, own, shared []WarehouseAttribute) bool {
	for _, condition := range conditions {
		if attributeOf(own, condition.Name) != condition.Equals && attributeOf(shared, condition.Name) != condition.Equals {
			return false
		}
	}
	return true
}

func attributeOf(attributes []WarehouseAttribute, name string) string {
	for _, attribute := range attributes {
		if attribute.Name == name {
			return attribute.Value
		}
	}
	return ""
}

func measureQuantity(plan WarehousePlan, unit string, measures []WarehouseMeasure) (*big.Rat, string, string) {
	for _, measure := range measures {
		if measure.Unit != unit {
			continue
		}
		if measure.Unmeasured {
			if plan.DeclaredValue == "USE" && measure.Declared != "" {
				quantity, ok := parseRat(measure.Declared)
				if !ok {
					return nil, "CONFLICT", "QUANTITY:" + unit
				}
				return quantity, "", ""
			}
			return nil, "PENDING", "UNMEASURED:" + unit
		}
		quantity, ok := parseRat(measure.Quantity)
		if !ok {
			return nil, "CONFLICT", "QUANTITY:" + unit
		}
		return quantity, "", ""
	}
	return nil, "PENDING", "MISSING:" + unit
}

func chargeAmount(rule WarehouseChargeRule, quantity *big.Rat) (int64, bool) {
	allowance, ok := parseRat(rule.Allowance)
	if rule.Allowance == "" {
		allowance = big.NewRat(0, 1)
		ok = true
	}
	unitQuantity, unitOK := parseRat(rule.UnitQuantity)
	if !ok || !unitOK || unitQuantity.Sign() <= 0 {
		return 0, false
	}
	part := new(big.Rat).Sub(quantity, allowance)
	if part.Sign() < 0 {
		part = big.NewRat(0, 1)
	}
	units := new(big.Rat).Quo(part, unitQuantity)
	if rule.Rounding == "UP" {
		units = ceilRat(units)
	}
	priced := new(big.Rat).Mul(units, big.NewRat(rule.UnitPrice, 1))
	priced.Add(priced, big.NewRat(rule.BaseFee, 1))
	if !priced.IsInt() {
		return 0, false
	}
	return priced.Num().Int64(), true
}

func parseRat(value string) (*big.Rat, bool) {
	if value == "" {
		return nil, false
	}
	quantity := new(big.Rat)
	if _, ok := quantity.SetString(value); !ok {
		return nil, false
	}
	return quantity, true
}

func ceilRat(value *big.Rat) *big.Rat {
	if value.IsInt() {
		return value
	}
	quotient := new(big.Int).Quo(value.Num(), value.Denom())
	if value.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return new(big.Rat).SetInt(quotient)
}
