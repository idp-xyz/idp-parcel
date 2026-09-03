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

// ChargeTypeReference 标明一份信用安排覆盖的费用类型。
// 信用按费用类型而非按客户授予，运费信用因此不会静默地为附加费买单。
type ChargeTypeReference struct{ requiredValue }

func NewChargeTypeReference(value string) (ChargeTypeReference, error) {
	required, err := newRequiredValue("charge type reference", value)
	return ChargeTypeReference{required}, err
}

// CreditPolicy 是一个信用政策版本的正文：为哪个责任法人、商业权限等级和费用类型，
// 在哪个有效区间内授权多少信用额度（金额或比例，见 CreditLimit）。
type CreditPolicy struct {
	version     CommercialVersion
	legalEntity LegalEntityReference
	level       AuthorityLevel
	chargeType  ChargeTypeReference
	limit       CreditLimit
	effective   EffectiveInterval
}

func NewCreditPolicy(
	version CommercialVersion,
	legalEntity LegalEntityReference,
	level AuthorityLevel,
	chargeType ChargeTypeReference,
	limit CreditLimit,
	effective EffectiveInterval,
) (CreditPolicy, error) {
	if version.kind != CreditPolicyObject ||
		version.status != CommercialVersionEffective ||
		!legalEntity.valid() || !level.valid() || !chargeType.valid() ||
		!limit.valid() || !effective.valid() {
		return CreditPolicy{}, ErrInvalidCreditPolicy
	}
	return CreditPolicy{
		version:     version,
		legalEntity: legalEntity,
		level:       level,
		chargeType:  chargeType,
		limit:       limit,
		effective:   effective,
	}, nil
}

func (policy CreditPolicy) Version() CommercialVersion {
	return policy.version
}

func (policy CreditPolicy) LegalEntity() LegalEntityReference {
	return policy.legalEntity
}

func (policy CreditPolicy) Level() AuthorityLevel {
	return policy.level
}

func (policy CreditPolicy) ChargeType() ChargeTypeReference {
	return policy.chargeType
}

func (policy CreditPolicy) AuthorizedLimit() CreditLimit {
	return policy.limit
}

func (policy CreditPolicy) Effective() EffectiveInterval {
	return policy.effective
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

// CreditBasis 是本上下文交给 settlement-accounting 的东西：一份政策授权的额度，
// 以及该额度出自哪个版本。它不带余额、不带调整、不带任何已占用量——政策只提供
// 业务判断依据，不直接修改结算余额。
//
// applicable 用来区分「授予了额度」与「根本没有额度」。没有它，零额度与无政策
// 读起来完全一样，`无适用依据` 会静默变成「授予零信用」。
type CreditBasis struct {
	policyVersion CommercialVersion
	limit         CreditLimit
	applicable    bool
}

func (basis CreditBasis) PolicyVersion() CommercialVersion {
	return basis.policyVersion
}

// AuthorizedLimit 交回政策授权的额度。无适用依据时它是零值 CreditLimit——两个访问器都答
// 「不在场」，与 applicable 为假一致；调用方不该从一个零值里读出任何数。
func (basis CreditBasis) AuthorizedLimit() CreditLimit {
	return basis.limit
}

func (basis CreditBasis) Applicable() bool {
	return basis.applicable
}

// ResolveCreditPolicy 在一个范围内选出唯一适用的信用政策。零候选如实报出而不作答：
// 缺政策既不是无限信用也不是零额度，该是哪一种只有拥有该商业依据的一方能说。
// 区间重叠形成`适用冲突`，而不是取额度较大或较小的那条。
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
			limit:         matches[0].limit,
			applicable:    true,
		}, nil
	default:
		return CreditBasis{}, ErrCreditPolicyConflict
	}
}
