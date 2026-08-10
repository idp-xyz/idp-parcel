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

// AsOfSemanticsReference 标明一项判断锚定到哪个业务时点。它保持为不透明引用：
// 真正的语义属于登记为 `PAR-COM-14` 的版本化试点政策，本上下文只携带引用，
// 既不解释它，也不提供一组取值供人挑选。
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

// JudgmentType 是规则包当前会为其声明时点锚的下游判断的封闭集合。取值随产生该
// 判断的规则一并加入，因此这个集合只在那些判断被实现时才增长。
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

// AsOfPolicy 是规则包针对一项判断所声明的内容：适用哪种时点语义，以及依据该政策
// 的哪个版本。它从不携带时点值本身——取值由消费方逐项形成，正是这一点让一个全局
// 时间无法替代所有下游判断时点。
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

// AsOfDeclaration 是第二阶段的输入：由第一阶段唯一选出的规则包，为各类判断声明的
// 时点锚。
type AsOfDeclaration struct {
	rulePackage CommercialVersion
	policies    map[JudgmentType]AsOfPolicy
}

// DeclareAsOfPolicies 把各类判断的时点锚绑定到一个规则包上。该包必须是当前可用的
// 接单规则包：`草稿` 尚未发布，而 `已到期`、`已退役` 或 `已替代` 的版本不再用于
// 新的判断。
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

// JudgmentAsOf 是一个已形成的时点锚：消费方选定的时点，连同授权这次选择的政策，
// 以便权威提供方对两者校验回显。
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

// FormAsOf 形成一项判断的时点锚。未声明的判断直接失败，而不去借用另一项判断的
// 政策；零值时点也直接失败，而不被读成「此刻」。
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
