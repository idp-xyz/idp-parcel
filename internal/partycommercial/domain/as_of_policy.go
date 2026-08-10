package domain

import (
	"errors"
	"time"
)

var (
	ErrUnusableRulePackage     = errors.New("party commercial: rule package cannot declare as-of policies")
	ErrAsOfPolicyNotConfigured = errors.New("party commercial: as-of policy is not configured")
	ErrConflictingAsOfPolicy   = errors.New("party commercial: conflicting as-of policy for one judgment")
	ErrInvalidAsOfValue        = errors.New("party commercial: invalid as-of value")
)

// AsOfSemanticsReference names which business instant a judgment is anchored
// to. It stays an opaque reference: the actual semantics belong to the versioned
// pilot policy registered as `PAR-COM-14`, so this context carries the reference
// without interpreting it and without offering a set of values to choose from.
type AsOfSemanticsReference struct{ requiredValue }

func NewAsOfSemanticsReference(value string) (AsOfSemanticsReference, error) {
	required, err := newRequiredValue("as-of semantics reference", value)
	return AsOfSemanticsReference{required}, err
}

type AsOfPolicyVersion struct{ requiredValue }

func NewAsOfPolicyVersion(value string) (AsOfPolicyVersion, error) {
	required, err := newRequiredValue("as-of policy version", value)
	return AsOfPolicyVersion{required}, err
}

// JudgmentType is the closed set of downstream judgments a rule package
// currently declares an anchor for. Values are added together with the rules
// that produce them, so this set grows only as those judgments are implemented.
type JudgmentType uint8

const (
	JudgmentTypeInvalid JudgmentType = iota
	NetworkReachabilityJudgment
	PreAcceptanceFinancialControlJudgment
)

func (judgment JudgmentType) valid() bool {
	return judgment >= NetworkReachabilityJudgment && judgment <= PreAcceptanceFinancialControlJudgment
}

func (judgment JudgmentType) String() string {
	switch judgment {
	case NetworkReachabilityJudgment:
		return "NETWORK_REACHABILITY"
	case PreAcceptanceFinancialControlJudgment:
		return "PRE_ACCEPTANCE_FINANCIAL_CONTROL"
	default:
		return ""
	}
}

// AsOfPolicy is what a rule package declares for one judgment: which instant
// semantics apply, and under which version of the policy that says so. It never
// carries the instant itself — choosing the value is the consumer's, per
// judgment, which is what keeps one global time from standing in for all.
type AsOfPolicy struct {
	judgment      JudgmentType
	semantics     AsOfSemanticsReference
	policyVersion AsOfPolicyVersion
}

func NewAsOfPolicy(
	judgment JudgmentType,
	semantics AsOfSemanticsReference,
	policyVersion AsOfPolicyVersion,
) (AsOfPolicy, error) {
	if !judgment.valid() || !semantics.valid() || !policyVersion.valid() {
		return AsOfPolicy{}, ErrAsOfPolicyNotConfigured
	}
	return AsOfPolicy{judgment: judgment, semantics: semantics, policyVersion: policyVersion}, nil
}

func (policy AsOfPolicy) Judgment() JudgmentType {
	return policy.judgment
}

func (policy AsOfPolicy) Semantics() AsOfSemanticsReference {
	return policy.semantics
}

func (policy AsOfPolicy) PolicyVersion() AsOfPolicyVersion {
	return policy.policyVersion
}

// AsOfDeclaration is phase two's input: the per-judgment anchors declared by the
// rule package phase one uniquely selected.
type AsOfDeclaration struct {
	rulePackage CommercialVersion
	policies    map[JudgmentType]AsOfPolicy
}

// DeclareAsOfPolicies binds per-judgment anchors to a rule package. The package
// must be an acceptance rule package that is currently usable: a draft has not
// been released, and one that expired, retired or was superseded no longer
// governs new judgments.
func DeclareAsOfPolicies(rulePackage CommercialVersion, policies []AsOfPolicy) (AsOfDeclaration, error) {
	if rulePackage.kind != AcceptanceRulePackageObject ||
		rulePackage.status != CommercialVersionEffective {
		return AsOfDeclaration{}, ErrUnusableRulePackage
	}
	if len(policies) == 0 {
		return AsOfDeclaration{}, ErrAsOfPolicyNotConfigured
	}

	declared := make(map[JudgmentType]AsOfPolicy, len(policies))
	for _, policy := range policies {
		if !policy.judgment.valid() || !policy.semantics.valid() || !policy.policyVersion.valid() {
			return AsOfDeclaration{}, ErrAsOfPolicyNotConfigured
		}
		if _, exists := declared[policy.judgment]; exists {
			return AsOfDeclaration{}, ErrConflictingAsOfPolicy
		}
		declared[policy.judgment] = policy
	}
	return AsOfDeclaration{rulePackage: rulePackage, policies: declared}, nil
}

func (declaration AsOfDeclaration) RulePackage() CommercialVersion {
	return declaration.rulePackage
}

func (declaration AsOfDeclaration) PolicyFor(judgment JudgmentType) (AsOfPolicy, bool) {
	policy, found := declaration.policies[judgment]
	return policy, found
}

// JudgmentAsOf is one formed anchor: the instant the consumer chose, together
// with the policy that authorised choosing it, so the authority provider can
// verify and echo both.
type JudgmentAsOf struct {
	judgment JudgmentType
	at       time.Time
	policy   AsOfPolicy
}

func (asOf JudgmentAsOf) Judgment() JudgmentType {
	return asOf.judgment
}

func (asOf JudgmentAsOf) At() time.Time {
	return asOf.at
}

func (asOf JudgmentAsOf) Policy() AsOfPolicy {
	return asOf.policy
}

// FormAsOf produces one judgment's anchor. An undeclared judgment fails rather
// than borrowing another judgment's policy, and a zero instant fails rather
// than being read as "now".
func (declaration AsOfDeclaration) FormAsOf(judgment JudgmentType, at time.Time) (JudgmentAsOf, error) {
	policy, found := declaration.policies[judgment]
	if !found {
		return JudgmentAsOf{}, ErrAsOfPolicyNotConfigured
	}
	if at.IsZero() {
		return JudgmentAsOf{}, ErrInvalidAsOfValue
	}
	return JudgmentAsOf{judgment: judgment, at: at.UTC(), policy: policy}, nil
}
