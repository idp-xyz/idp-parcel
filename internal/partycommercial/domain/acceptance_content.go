package domain

import "errors"

var (
	// ErrAcceptanceContentNotConfigured 是声明缺件：组集合为空/含非法组，或人工复核指令
	// 未明确。恢复动作是把规则包正文声明补齐（实例半边），不是本上下文代拟默认。
	ErrAcceptanceContentNotConfigured  = errors.New("party commercial: acceptance rule content is not configured")
	ErrConflictingCheckGroup           = errors.New("party commercial: conflicting applicable check group")
	ErrInvalidPendingRoutingPermission = errors.New("party commercial: invalid pending routing permission")
)

// AcceptanceCheckGroupType 是规则包会为其声明适用性的下游校验组封闭集合（ADR-0042）。
// 与 RuleCategory 不同物：那是装配规则引用的归档分区；这里是 UC-PS-001 校验组清单的
// 本上下文引用，取值随消费方实现的校验组增长（先例：JudgmentType）。
type AcceptanceCheckGroupType uint8

const (
	AcceptanceCheckGroupTypeInvalid AcceptanceCheckGroupType = iota
	CustomerRelationshipCheckGroup
	LegalEntityAndContractCheckGroup
	ProductAndServiceCheckGroup
	MemberBaselineCheckGroup
	RequiredDocumentCheckGroup
	PreAcceptanceFinancialControlCheckGroup
	NetworkReachabilityCheckGroup
)

func (group AcceptanceCheckGroupType) valid() bool {
	return group >= CustomerRelationshipCheckGroup && group <= NetworkReachabilityCheckGroup
}

func (group AcceptanceCheckGroupType) String() string {
	switch group {
	case CustomerRelationshipCheckGroup:
		return "CUSTOMER_RELATIONSHIP"
	case LegalEntityAndContractCheckGroup:
		return "LEGAL_ENTITY_AND_CONTRACT"
	case ProductAndServiceCheckGroup:
		return "PRODUCT_AND_SERVICE"
	case MemberBaselineCheckGroup:
		return "MEMBER_BASELINE"
	case RequiredDocumentCheckGroup:
		return "REQUIRED_DOCUMENT"
	case PreAcceptanceFinancialControlCheckGroup:
		return "PRE_ACCEPTANCE_FINANCIAL_CONTROL"
	case NetworkReachabilityCheckGroup:
		return "NETWORK_REACHABILITY"
	default:
		return ""
	}
}

// ManualReviewDirective 是规则包对「这类委托要不要人工业务复核」的声明。零值 = 未声明。
// 它与授权治理分格（ADR-0042）：Authorize 带 ManualReviewAction 答「谁有权复核」，这里答「要不要」。
// 复核做没做完属消费方的任务状态，本上下文不声明它。
type ManualReviewDirective uint8

const (
	ManualReviewUndeclared ManualReviewDirective = iota
	ManualReviewNotRequired
	ManualReviewRequired
)

func (directive ManualReviewDirective) Declared() bool {
	return directive == ManualReviewNotRequired || directive == ManualReviewRequired
}

func (directive ManualReviewDirective) String() string {
	switch directive {
	case ManualReviewNotRequired:
		return "NOT_REQUIRED"
	case ManualReviewRequired:
		return "REQUIRED"
	default:
		return ""
	}
}

// AcceptanceRuleContent 是一个已生效接单规则包的内容声明：哪些下游校验组适用、要不要
// 人工复核。声明属规则包正文；消费方按已选出的包读取并翻译，不代它拟默认（ADR-0042）。
type AcceptanceRuleContent struct {
	rulePackage  CommercialVersion
	applicable   []AcceptanceCheckGroupType
	manualReview ManualReviewDirective
}

// DeclareAcceptanceRuleContent 把内容声明绑定到规则包上。与 DeclareAsOfPolicies 同判据：
// 该包必须是当前可用的接单规则包。空组集合等于无条件接受，规则包表达不了那种东西；
// 人工复核不是默认步骤，未明确即未配置——两个方向都不由本上下文兜底。
func DeclareAcceptanceRuleContent(
	rulePackage CommercialVersion,
	applicable []AcceptanceCheckGroupType,
	manualReview ManualReviewDirective,
) (AcceptanceRuleContent, error) {
	if rulePackage.kind != AcceptanceRulePackageObject ||
		rulePackage.status != CommercialVersionEffective {
		return AcceptanceRuleContent{}, ErrUnusableRulePackage
	}
	if len(applicable) == 0 || !manualReview.Declared() {
		return AcceptanceRuleContent{}, ErrAcceptanceContentNotConfigured
	}
	seen := make(map[AcceptanceCheckGroupType]struct{}, len(applicable))
	for _, group := range applicable {
		if !group.valid() {
			return AcceptanceRuleContent{}, ErrAcceptanceContentNotConfigured
		}
		if _, exists := seen[group]; exists {
			return AcceptanceRuleContent{}, ErrConflictingCheckGroup
		}
		seen[group] = struct{}{}
	}
	return AcceptanceRuleContent{
		rulePackage:  rulePackage,
		applicable:   append([]AcceptanceCheckGroupType(nil), applicable...),
		manualReview: manualReview,
	}, nil
}

func (content AcceptanceRuleContent) RulePackage() CommercialVersion {
	return content.rulePackage
}

func (content AcceptanceRuleContent) ApplicableGroups() []AcceptanceCheckGroupType {
	return append([]AcceptanceCheckGroupType(nil), content.applicable...)
}

func (content AcceptanceRuleContent) Applies(group AcceptanceCheckGroupType) bool {
	for _, declared := range content.applicable {
		if declared == group {
			return true
		}
	}
	return false
}

func (content AcceptanceRuleContent) ManualReview() ManualReviewDirective {
	return content.manualReview
}

// PendingRoutingBasisReference 指向服务产品允许待路由所依据的商业事实。消费方保存的
// 正是这条引用（AT-PS-007「保存允许依据」）。
type PendingRoutingBasisReference struct{ requiredValue }

func NewPendingRoutingBasisReference(value string) (PendingRoutingBasisReference, error) {
	required, err := newRequiredValue("pending routing basis reference", value)
	return PendingRoutingBasisReference{required}, err
}

// PendingRoutingPermission 是服务产品对待路由的明确许可。零值 = 未许可/未配置。
// 许可必须携带依据：没有依据的许可与一次默认放行分不开（ADR-0042）。
type PendingRoutingPermission struct {
	product CommercialVersion
	basis   PendingRoutingBasisReference
}

// DeclarePendingRoutingPermission 由已生效的服务产品声明待路由许可。规则包声明不了它：
// UC 把这份许可判给服务产品。
func DeclarePendingRoutingPermission(
	product CommercialVersion,
	basis PendingRoutingBasisReference,
) (PendingRoutingPermission, error) {
	if product.kind != ServiceProductObject ||
		product.status != CommercialVersionEffective ||
		!basis.valid() {
		return PendingRoutingPermission{}, ErrInvalidPendingRoutingPermission
	}
	return PendingRoutingPermission{product: product, basis: basis}, nil
}

func (permission PendingRoutingPermission) Allowed() bool {
	return permission.basis.valid()
}

func (permission PendingRoutingPermission) Product() CommercialVersion {
	return permission.product
}

func (permission PendingRoutingPermission) Basis() PendingRoutingBasisReference {
	return permission.basis
}
