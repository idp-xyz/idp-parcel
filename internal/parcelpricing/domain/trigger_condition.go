package domain

import "strings"

// TriggerKind is how a trigger condition is built: a single predicate, or every
// one of its operands, or any one of them. The set is closed at three, which is
// what keeps a trigger enumerable — every leaf's hit and miss can be reported
// from the version manifest alone, which an expression engine could not offer.
type TriggerKind string

const (
	TriggerPredicate TriggerKind = "PREDICATE"
	TriggerAllOf     TriggerKind = "ALL_OF"
	TriggerAnyOf     TriggerKind = "ANY_OF"
)

func (kind TriggerKind) String() string { return string(kind) }

func (kind TriggerKind) valid() bool {
	switch kind {
	case TriggerPredicate, TriggerAllOf, TriggerAnyOf:
		return true
	default:
		return false
	}
}

// TriggerCondition is what a rule declares as the thing that sets it off. The
// card writes one clause with several alternatives rather than several clauses:
// `R40` charges once for a piece over 67.5 KG **or** longer than 274 CM **or**
// over 419 CM in length plus girth. Splitting that into three rules would
// charge three times for what the card charges once, so the combination has to
// be part of the rule rather than a property of how rules were listed.
type TriggerCondition struct {
	kind      TriggerKind
	predicate FeatureCondition
	operands  []TriggerCondition
}

// NewTrigger wraps a single predicate. A rule always declares a trigger, so a
// one-predicate rule states that fact rather than being a different shape.
func NewTrigger(predicate FeatureCondition) (TriggerCondition, error) {
	trigger := TriggerCondition{kind: TriggerPredicate, predicate: predicate}
	if !trigger.valid() {
		return TriggerCondition{}, ErrInvalidFeatureCondition
	}
	return trigger, nil
}

func NewAllOfTrigger(operands ...TriggerCondition) (TriggerCondition, error) {
	return newCombinedTrigger(TriggerAllOf, operands)
}

func NewAnyOfTrigger(operands ...TriggerCondition) (TriggerCondition, error) {
	return newCombinedTrigger(TriggerAnyOf, operands)
}

func newCombinedTrigger(kind TriggerKind, operands []TriggerCondition) (TriggerCondition, error) {
	// An empty combination has no reading the card could have meant; treating
	// it as always-true or always-false would invent a rule nobody declared.
	if len(operands) == 0 {
		return TriggerCondition{}, ErrInvalidFeatureCondition
	}
	trigger := TriggerCondition{kind: kind, operands: append([]TriggerCondition(nil), operands...)}
	if !trigger.valid() {
		return TriggerCondition{}, ErrInvalidFeatureCondition
	}
	return trigger, nil
}

func (trigger TriggerCondition) Kind() TriggerKind { return trigger.kind }

func (trigger TriggerCondition) Predicate() (FeatureCondition, bool) {
	if trigger.kind != TriggerPredicate {
		return FeatureCondition{}, false
	}
	return trigger.predicate, true
}

func (trigger TriggerCondition) Operands() []TriggerCondition {
	return append([]TriggerCondition(nil), trigger.operands...)
}

// Matches reports whether the trigger holds. It does not short-circuit: a
// mismatched unit anywhere in the combination is a declaration error the card
// has to fix, and hiding it behind an alternative that happened to hold first
// would make the same plan explain itself differently run to run.
func (trigger TriggerCondition) Matches(features PackageFeatures) (bool, error) {
	if !trigger.valid() || !features.valid() {
		return false, ErrInvalidFeatureCondition
	}
	if trigger.kind == TriggerPredicate {
		return trigger.predicate.Matches(features)
	}
	held := trigger.kind == TriggerAllOf
	for _, operand := range trigger.operands {
		matched, err := operand.Matches(features)
		if err != nil {
			return false, err
		}
		switch trigger.kind {
		case TriggerAllOf:
			held = held && matched
		case TriggerAnyOf:
			held = held || matched
		}
	}
	return held, nil
}

// describe renders the whole combination, not just its first leaf, so an
// explanation of a miss says which clause was not met rather than naming one
// alternative and leaving the rest invisible.
func (trigger TriggerCondition) describe() string {
	switch trigger.kind {
	case TriggerPredicate:
		return trigger.predicate.describe()
	case TriggerAllOf, TriggerAnyOf:
		parts := make([]string, 0, len(trigger.operands))
		for _, operand := range trigger.operands {
			parts = append(parts, operand.describe())
		}
		label := "all of"
		if trigger.kind == TriggerAnyOf {
			label = "any of"
		}
		return label + " (" + strings.Join(parts, ", ") + ")"
	default:
		return ""
	}
}

func (trigger TriggerCondition) valid() bool {
	if !trigger.kind.valid() {
		return false
	}
	if trigger.kind == TriggerPredicate {
		return len(trigger.operands) == 0 && trigger.predicate.valid()
	}
	if len(trigger.operands) == 0 {
		return false
	}
	for _, operand := range trigger.operands {
		if !operand.valid() {
			return false
		}
	}
	return true
}
