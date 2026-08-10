package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidCreditPolicy      = errors.New("party commercial: invalid credit policy")
	ErrInvalidCreditPolicyQuery = errors.New("party commercial: invalid credit policy query")
	ErrNoApplicableCreditPolicy = errors.New("party commercial: no applicable credit policy")
	ErrCreditPolicyConflict     = errors.New("party commercial: several credit policies cover one range")
)

// ChargeTypeReference names the kind of charge a credit arrangement covers.
// Credit is granted per charge type rather than per customer, so freight credit
// cannot silently fund a surcharge.
type ChargeTypeReference struct{ requiredValue }

func NewChargeTypeReference(value string) (ChargeTypeReference, error) {
	required, err := newRequiredValue("charge type reference", value)
	return ChargeTypeReference{required}, err
}

// CreditPolicy is the content of one credit policy version: how much credit is
// authorised, for which legal entity, business authority level and charge type,
// over which interval.
type CreditPolicy struct {
	version     CommercialVersion
	legalEntity LegalEntityReference
	level       AuthorityLevel
	chargeType  ChargeTypeReference
	limitMinor  int64
	effective   EffectiveInterval
}

func NewCreditPolicy(
	version CommercialVersion,
	legalEntity LegalEntityReference,
	level AuthorityLevel,
	chargeType ChargeTypeReference,
	limitMinor int64,
	effective EffectiveInterval,
) (CreditPolicy, error) {
	if version.kind != CreditPolicyObject ||
		version.status != CommercialVersionEffective ||
		!legalEntity.valid() || !level.valid() || !chargeType.valid() ||
		limitMinor < 0 || !effective.valid() {
		return CreditPolicy{}, ErrInvalidCreditPolicy
	}
	return CreditPolicy{
		version:     version,
		legalEntity: legalEntity,
		level:       level,
		chargeType:  chargeType,
		limitMinor:  limitMinor,
		effective:   effective,
	}, nil
}

func (policy CreditPolicy) Version() CommercialVersion {
	return policy.version
}

func (policy CreditPolicy) AuthorizedLimitMinor() int64 {
	return policy.limitMinor
}

func (policy CreditPolicy) covers(query CreditPolicyQuery) bool {
	return policy.legalEntity == query.legalEntity &&
		policy.level == query.level &&
		policy.chargeType == query.chargeType &&
		policy.effective.Contains(query.at)
}

type CreditPolicyQuery struct {
	legalEntity LegalEntityReference
	level       AuthorityLevel
	chargeType  ChargeTypeReference
	at          time.Time
}

func NewCreditPolicyQuery(
	legalEntity LegalEntityReference,
	level AuthorityLevel,
	chargeType ChargeTypeReference,
	at time.Time,
) (CreditPolicyQuery, error) {
	if !legalEntity.valid() || !level.valid() || !chargeType.valid() || at.IsZero() {
		return CreditPolicyQuery{}, ErrInvalidCreditPolicyQuery
	}
	return CreditPolicyQuery{legalEntity: legalEntity, level: level, chargeType: chargeType, at: at.UTC()}, nil
}

// CreditBasis is what this context hands to settlement-accounting: the limit a
// policy authorises and where that came from. It carries no balance, no
// adjustment and nothing already applied, because the policy supplies a basis
// for judgement and never acts on a settlement balance itself.
//
// Applicable distinguishes a granted limit from an absent one. Without it a zero
// limit and no policy at all would read identically, and "no credit policy" would
// silently become "zero credit granted".
type CreditBasis struct {
	policyVersion CommercialVersion
	limitMinor    int64
	applicable    bool
}

func (basis CreditBasis) PolicyVersion() CommercialVersion {
	return basis.policyVersion
}

func (basis CreditBasis) AuthorizedLimitMinor() int64 {
	return basis.limitMinor
}

func (basis CreditBasis) Applicable() bool {
	return basis.applicable
}

// ResolveCreditPolicy selects the single policy covering one range. Nothing
// matching is reported rather than answered: an absent policy is neither
// unlimited credit nor a zero limit, and only the owning commercial party can
// say which it should be. Overlapping policies conflict instead of resolving to
// the larger — or smaller — limit.
func ResolveCreditPolicy(policies []CreditPolicy, query CreditPolicyQuery) (CreditBasis, error) {
	matches := make([]CreditPolicy, 0, 2)
	for _, policy := range policies {
		if policy.covers(query) {
			matches = append(matches, policy)
		}
	}

	switch len(matches) {
	case 0:
		return CreditBasis{}, ErrNoApplicableCreditPolicy
	case 1:
		return CreditBasis{
			policyVersion: matches[0].version,
			limitMinor:    matches[0].limitMinor,
			applicable:    true,
		}, nil
	default:
		return CreditBasis{}, ErrCreditPolicyConflict
	}
}
