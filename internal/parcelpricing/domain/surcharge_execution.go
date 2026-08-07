package domain

import (
	"fmt"
	"sort"
)

// surchargeOutcome records what happened to one declared rule, including the
// rules that did not fire. CONTEXT requires a miss to leave a trace: an
// explanation listing only hits cannot be checked against the card, because a
// reader cannot tell a rule that missed from a rule that was never evaluated.
type surchargeOutcome struct {
	rule     SurchargeRule
	matched  bool
	selected bool
	deferred bool
	amount   Money
	note     string
}

// unexecutable names the first declared structure this build cannot price.
// The gate exists so a plan is never priced on the base table alone while it
// declares charges nobody executes; it narrows as capabilities land rather than
// being removed.
func (structures PricingPlanStructures) unexecutable() (string, bool) {
	if len(structures.referenceSeries) > 0 {
		return "reference series bindings", true
	}
	for _, rule := range structures.surchargeRules {
		switch rule.calculation.method {
		case ChargeMethodFixedAmount, ChargeMethodTableLookup, ChargeMethodPercentOfBasis, ChargeMethodGreaterOf:
		default:
			return fmt.Sprintf("%s calculation on %s", rule.calculation.method, rule.id), true
		}
	}
	return "", false
}

// minimumRaise is one conditional minimum that fired, kept with its rule so the
// explanation can name the clause rather than only the number.
type minimumRaise struct {
	id      string
	minimum Weight
}

// resolveMinimums reports every conditional minimum whose clause holds. The
// clause lives inside a surcharge rule but raises the plan-level pricing weight,
// so it is resolved before the weight is fixed rather than alongside the
// surcharge it was declared with.
func (structures PricingPlanStructures) resolveMinimums(features PackageFeatures) ([]minimumRaise, error) {
	raises := make([]minimumRaise, 0, len(structures.surchargeRules))
	for _, rule := range structures.surchargeRules {
		if rule.minimumWeight == nil {
			continue
		}
		held, err := rule.minimumWeight.condition.Matches(features)
		if err != nil {
			return nil, err
		}
		if held {
			raises = append(raises, minimumRaise{id: rule.minimumWeight.id, minimum: rule.minimumWeight.minimum})
		}
	}
	return raises, nil
}

// highestMinimum picks the floor to apply. CONTEXT: 同一评价可存在多条，同时触发时
// 取其中最高者 — the card carries two, 40 LB and 90 LB, and a parcel tripping both
// is billed at 90.
func highestMinimum(raises []minimumRaise) (minimumRaise, bool) {
	var highest minimumRaise
	found := false
	for _, raise := range raises {
		if !found || raise.minimum.value.Cmp(highest.minimum.value) > 0 {
			highest, found = raise, true
		}
	}
	return highest, found
}

// surchargeContext is what a rule may read beyond the package's own features:
// the zone the shipment falls in and the pricing weight the base freight used.
// A banded surcharge reads the same weight as the base table so the two can
// never disagree about how heavy the parcel was.
type surchargeContext struct {
	features      PackageFeatures
	zone          string
	pricingWeight Weight
}

// resolveSurcharges decides every declared rule against the package's features
// and then applies the card's interaction rules. Rules that stand alone are all
// collected; rules in an exclusivity group compete, and at most one of the
// group is charged.
func (structures PricingPlanStructures) resolveSurcharges(reading surchargeContext) ([]surchargeOutcome, error) {
	outcomes := make([]surchargeOutcome, 0, len(structures.surchargeRules))
	for _, rule := range structures.surchargeRules {
		matched, err := rule.condition.Matches(reading.features)
		if err != nil {
			return nil, err
		}
		outcome := surchargeOutcome{rule: rule, matched: matched}
		if matched {
			// A charge that reads a basis cannot be valued yet: the basis sums
			// charge lines still being collected. It is priced in a second
			// pass, ordered by dependency.
			if rule.calculation.needsBasis() {
				outcome.deferred = true
				outcome.selected = true
			} else {
				amount, err := rule.calculation.resolve(reading, nil)
				if err != nil {
					return nil, err
				}
				outcome.amount = amount
				outcome.selected = true
			}
		}
		outcomes = append(outcomes, outcome)
	}
	if err := selectWithinExclusivityGroups(outcomes); err != nil {
		return nil, err
	}
	return outcomes, nil
}

// needsBasis reports whether valuing this calculation requires charge lines
// that are still being collected. A greater-of inherits the need from either
// operand: comparing a flat floor against an unresolved share would always pick
// the floor.
func (calculation SurchargeCalculation) needsBasis() bool {
	switch calculation.method {
	case ChargeMethodPercentOfBasis:
		return true
	case ChargeMethodGreaterOf:
		for _, operand := range calculation.operands {
			if operand.needsBasis() {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// resolve produces the amount a matched rule charges. A gap in a banded table
// is a gap in the card rather than a rule that missed — the condition did fire
// — so the lookup error travels out unchanged and the evaluation waits. The
// basis is nil in the first pass, where only calculations that do not read one
// are valued.
func (calculation SurchargeCalculation) resolve(reading surchargeContext, basis *dependencyBasis) (Money, error) {
	switch calculation.method {
	case ChargeMethodFixedAmount:
		amount, ok := calculation.FixedAmount()
		if !ok {
			return Money{}, ErrInvalidSurchargeRule
		}
		return amount, nil
	case ChargeMethodTableLookup:
		table, ok := calculation.LookupTable()
		if !ok {
			return Money{}, ErrInvalidSurchargeRule
		}
		selection, err := table.Lookup(reading.zone, reading.pricingWeight)
		if err != nil {
			return Money{}, err
		}
		return selection.amount, nil
	case ChargeMethodPercentOfBasis:
		if basis == nil {
			return Money{}, fmt.Errorf("%w: %s valued before its basis", ErrPlanStructuresNotExecutable, calculation.method)
		}
		return basis.share(calculation)
	case ChargeMethodGreaterOf:
		var best Money
		for index, operand := range calculation.operands {
			amount, err := operand.resolve(reading, basis)
			if err != nil {
				return Money{}, err
			}
			if index == 0 || amount.amount.Cmp(best.amount) > 0 {
				best = amount
			}
		}
		if !best.valid() {
			return Money{}, ErrInvalidSurchargeRule
		}
		return best, nil
	default:
		return Money{}, fmt.Errorf("%w: %s", ErrPlanStructuresNotExecutable, calculation.method)
	}
}

// selectWithinExclusivityGroups keeps one member per group.
//
// Precedence is by declared rank first and amount second. **Rank 1 is the
// highest**: the rank is declared as a positive ordinal starting at 1, so the
// first rank is the one that wins. The order matters — a group whose highest
// rank carries the smaller amount must still charge the smaller amount, which a
// plain "take the largest" would get wrong.
func selectWithinExclusivityGroups(outcomes []surchargeOutcome) error {
	best := make(map[string]int, len(outcomes))
	for index := range outcomes {
		outcome := &outcomes[index]
		if !outcome.matched || outcome.rule.exclusivity != ExclusivityGrouped {
			continue
		}
		group := outcome.rule.exclusivityGroup
		incumbent, seen := best[group]
		if !seen {
			best[group] = index
			continue
		}
		preferred, err := preferSurcharge(outcomes[incumbent], *outcome)
		if err != nil {
			return err
		}
		if preferred {
			outcomes[index].selected = false
			outcomes[index].note = fmt.Sprintf("suppressed by %s in exclusivity group %s", outcomes[incumbent].rule.id, group)
			continue
		}
		outcomes[incumbent].selected = false
		outcomes[incumbent].note = fmt.Sprintf("suppressed by %s in exclusivity group %s", outcome.rule.id, group)
		best[group] = index
	}
	return nil
}

// preferSurcharge reports whether the incumbent keeps the group. Two rules at
// the same rank carrying the same amount cannot be told apart, and CONTEXT
// forbids picking arbitrarily, so that is a conflict for a human to resolve.
func preferSurcharge(incumbent, challenger surchargeOutcome) (bool, error) {
	if incumbent.rule.priority != challenger.rule.priority {
		return incumbent.rule.priority < challenger.rule.priority, nil
	}
	comparison := incumbent.amount.amount.Cmp(challenger.amount.amount)
	if comparison == 0 {
		return false, fmt.Errorf("%w: %s and %s share rank %d and amount in group %s",
			ErrRateTableConflict, incumbent.rule.id, challenger.rule.id, incumbent.rule.priority, incumbent.rule.exclusivityGroup)
	}
	return comparison > 0, nil
}

// explain renders one line per rule, hit or miss, so the explanation can be
// read against the card rule by rule.
func (outcome surchargeOutcome) explain() string {
	switch {
	case !outcome.matched:
		return fmt.Sprintf("surcharge %s did not apply: %s not met",
			outcome.rule.id, outcome.rule.condition.describe())
	case !outcome.selected:
		return fmt.Sprintf("surcharge %s applied but was not collected: %s", outcome.rule.id, outcome.note)
	default:
		return fmt.Sprintf("surcharge %s applied for %s", outcome.rule.id, outcome.amount.amount.String())
	}
}

// sortedSurchargeOutcomes keeps the charge lines and the explanation in a
// deterministic order regardless of map iteration above.
func sortedSurchargeOutcomes(outcomes []surchargeOutcome) []surchargeOutcome {
	sorted := append([]surchargeOutcome(nil), outcomes...)
	sort.SliceStable(sorted, func(left, right int) bool {
		return sorted[left].rule.id < sorted[right].rule.id
	})
	return sorted
}
