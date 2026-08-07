package domain

// ExclusionRule is a card clause that refuses the parcel outright instead of
// charging for it. Public channel terms state oversize handling in tiers — a
// surcharge past the first bound, a larger one past the second, and refusal
// past the last — so an exclusion reads the same features through the same
// trigger grammar as a surcharge and differs only in effect: a surcharge adds
// money, an exclusion means no price exists at all.
//
// It carries the clause it came from because CONTEXT requires the outcome to
// name the ground it was excluded on; a bare status cannot be checked against
// the card in a dispute.
type ExclusionRule struct {
	id        string
	clause    string
	condition TriggerCondition
}

func NewExclusionRule(id, clause string, condition TriggerCondition) (ExclusionRule, error) {
	rule := ExclusionRule{id: id, clause: clause, condition: condition}
	if !rule.valid() {
		return ExclusionRule{}, ErrInvalidExclusionRule
	}
	return rule, nil
}

func (rule ExclusionRule) ID() string                  { return rule.id }
func (rule ExclusionRule) Clause() string              { return rule.clause }
func (rule ExclusionRule) Condition() TriggerCondition { return rule.condition }

func (rule ExclusionRule) valid() bool {
	return trimmed(rule.id) && trimmed(rule.clause) && rule.condition.valid()
}

// resolveExclusions reports the first declared clause that refuses this parcel.
// Every clause is decided even after one holds: a mismatched unit anywhere is a
// declaration error the card has to fix, and letting an earlier hit hide it
// would make the same plan refuse quietly on one parcel and error on the next.
// Which clause is reported is stable because the structures keep them ordered
// by id.
func (structures PricingPlanStructures) resolveExclusions(features PackageFeatures) (ExclusionRule, bool, error) {
	var excluded ExclusionRule
	found := false
	for _, rule := range structures.exclusions {
		held, err := rule.condition.Matches(features)
		if err != nil {
			return ExclusionRule{}, false, err
		}
		if held && !found {
			excluded, found = rule, true
		}
	}
	return excluded, found, nil
}
