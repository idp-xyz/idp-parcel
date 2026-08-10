package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidPricePolicy      = errors.New("party commercial: invalid commercial price policy")
	ErrInvalidPricePolicyQuery = errors.New("party commercial: invalid price policy query")
	ErrNoApplicablePricePolicy = errors.New("party commercial: no applicable commercial price policy")
	ErrPricePolicyConflict     = errors.New("party commercial: one direction and scope is covered by several price policies")
)

// PricingPlanReference points at an executable pricing plan version owned by
// parcel-pricing. It is a reference and stays one: rate cards, rate tables and
// charge-dependency execution belong to that context, so nothing here can
// evaluate a price.
type PricingPlanReference struct{ requiredValue }

func NewPricingPlanReference(value string) (PricingPlanReference, error) {
	required, err := newRequiredValue("pricing plan reference", value)
	return PricingPlanReference{required}, err
}

// CommercialPricePolicy is the content of one price rule version: which price
// direction it authorises, which executable pricing plan it binds, over which
// scope and interval.
//
// A bare price-rule version cannot be used to price anything, because the
// direction and the plan binding live here rather than on the version. That is
// deliberate: it means a generic resolution that returns only a version has not
// produced a usable pricing basis, and the direction cannot be skipped.
type CommercialPricePolicy struct {
	version   CommercialVersion
	direction PriceDirection
	plan      PricingPlanReference
	scope     CommercialScopeReference
	effective EffectiveInterval
}

func NewCommercialPricePolicy(
	version CommercialVersion,
	direction PriceDirection,
	plan PricingPlanReference,
	scope CommercialScopeReference,
	effective EffectiveInterval,
) (CommercialPricePolicy, error) {
	if version.kind != PriceRuleObject ||
		version.status != CommercialVersionEffective ||
		!direction.valid() || !plan.valid() || !scope.valid() || !effective.valid() {
		return CommercialPricePolicy{}, ErrInvalidPricePolicy
	}
	return CommercialPricePolicy{
		version:   version,
		direction: direction,
		plan:      plan,
		scope:     scope,
		effective: effective,
	}, nil
}

func (policy CommercialPricePolicy) Version() CommercialVersion {
	return policy.version
}

func (policy CommercialPricePolicy) Direction() PriceDirection {
	return policy.direction
}

func (policy CommercialPricePolicy) PricingPlan() PricingPlanReference {
	return policy.plan
}

func (policy CommercialPricePolicy) Scope() CommercialScopeReference {
	return policy.scope
}

func (policy CommercialPricePolicy) covers(query PricePolicyQuery) bool {
	return policy.direction == query.direction &&
		policy.scope == query.scope &&
		policy.effective.Contains(query.at)
}

type PricePolicyQuery struct {
	direction PriceDirection
	scope     CommercialScopeReference
	at        time.Time
}

func NewPricePolicyQuery(
	direction PriceDirection,
	scope CommercialScopeReference,
	at time.Time,
) (PricePolicyQuery, error) {
	if !direction.valid() || !scope.valid() || at.IsZero() {
		return PricePolicyQuery{}, ErrInvalidPricePolicyQuery
	}
	return PricePolicyQuery{direction: direction, scope: scope, at: at.UTC()}, nil
}

// ResolveCommercialPricePolicy selects the single policy covering one direction
// and scope. Direction is part of the match, so a BUY policy never answers a
// SELL request and a direction with no policy of its own gets none.
//
// Nothing matching and several matching are both reported rather than resolved:
// the context forbids falling back to a default price, and picking one of two
// overlapping policies would be doing exactly that under another name.
func ResolveCommercialPricePolicy(
	policies []CommercialPricePolicy,
	query PricePolicyQuery,
) (CommercialPricePolicy, error) {
	matches := make([]CommercialPricePolicy, 0, 2)
	for _, policy := range policies {
		if policy.covers(query) {
			matches = append(matches, policy)
		}
	}

	switch len(matches) {
	case 0:
		return CommercialPricePolicy{}, ErrNoApplicablePricePolicy
	case 1:
		return matches[0], nil
	default:
		return CommercialPricePolicy{}, ErrPricePolicyConflict
	}
}
