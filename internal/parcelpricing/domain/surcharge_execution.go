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
	amount   Money
	note     string
}

// unexecutable names the first declared structure this build cannot price.
// The gate exists so a plan is never priced on the base table alone while it
// declares charges nobody executes; it narrows as capabilities land rather than
// being removed.
func (structures PricingPlanStructures) unexecutable() (string, bool) {
	if len(structures.dependencies) > 0 {
		return "charge dependencies", true
	}
	if len(structures.referenceSeries) > 0 {
		return "reference series bindings", true
	}
	for _, rule := range structures.surchargeRules {
		if rule.minimumWeight != nil {
			return fmt.Sprintf("conditional minimum weight on %s", rule.id), true
		}
		if rule.calculation.method != ChargeMethodFixedAmount {
			return fmt.Sprintf("%s calculation on %s", rule.calculation.method, rule.id), true
		}
	}
	return "", false
}

// resolveSurcharges decides every declared rule against the package's features
// and then applies the card's interaction rules. Rules that stand alone are all
// collected; rules in an exclusivity group compete, and at most one of the
// group is charged.
func (structures PricingPlanStructures) resolveSurcharges(features PackageFeatures) ([]surchargeOutcome, error) {
	outcomes := make([]surchargeOutcome, 0, len(structures.surchargeRules))
	for _, rule := range structures.surchargeRules {
		matched, err := rule.condition.Matches(features)
		if err != nil {
			return nil, err
		}
		outcome := surchargeOutcome{rule: rule, matched: matched}
		if matched {
			amount, ok := rule.calculation.FixedAmount()
			if !ok {
				return nil, fmt.Errorf("%w: %s", ErrPlanStructuresNotExecutable, rule.id)
			}
			outcome.amount = amount
			outcome.selected = true
		}
		outcomes = append(outcomes, outcome)
	}
	if err := selectWithinExclusivityGroups(outcomes); err != nil {
		return nil, err
	}
	return outcomes, nil
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
