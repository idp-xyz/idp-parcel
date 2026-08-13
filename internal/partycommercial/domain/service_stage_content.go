package domain

import "errors"

var (
	// ErrIntakeContentNotConfigured 是收寄资格声明缺件：允许来源为空或资格项非法。
	// 恢复动作是把规则正文声明补齐（PAR-COM-16 实例半边），不是本上下文代拟默认。
	ErrIntakeContentNotConfigured = errors.New("party commercial: intake qualification content is not configured")
	// ErrFinalContentNotConfigured 是终局规则声明缺件。恢复动作同上（PAR-COM-17）。
	ErrFinalContentNotConfigured = errors.New("party commercial: final rule content is not configured")
	ErrConflictingIntakeSource   = errors.New("party commercial: conflicting intake source declaration")
	ErrConflictingFinalization   = errors.New("party commercial: conflicting finalization declaration")
)

// DeclaredIntakeSource 是规则可声明的收寄来源封闭二值，与 parcel-shipment 来源联合
// 的两格语义对应（客户送站的节点收寄、场外揽收）；这是本上下文对那两格的引用枚举，
// 先例同 AcceptanceCheckGroupType 之于校验组。
type DeclaredIntakeSource uint8

const (
	DeclaredIntakeSourceInvalid DeclaredIntakeSource = iota
	DeclaredNodeIntake
	DeclaredOffsitePickup
)

func (source DeclaredIntakeSource) valid() bool {
	return source == DeclaredNodeIntake || source == DeclaredOffsitePickup
}

func (source DeclaredIntakeSource) String() string {
	switch source {
	case DeclaredNodeIntake:
		return "NODE_INTAKE"
	case DeclaredOffsitePickup:
		return "OFFSITE_PICKUP"
	default:
		return ""
	}
}

// IntakeQualificationContent 是一个已生效规则包的收寄阶段资格声明（UC-PS-003 资格
// 核对第 5 条的提供方半边）：允许哪些收寄来源、要过哪些硬资格。资格项是开放引用——
// 目录属实例参数；声明为空集不是「没有资格要求」而是没声明（缺件错误），真没有要求
// 也要显式声明空清单带来源允许。
type IntakeQualificationContent struct {
	allowedSources map[DeclaredIntakeSource]bool
	qualifications []RuleReference
}

// NewIntakeQualificationContent 组装声明。允许来源至少一格且不重（一个收寄模式都
// 不允许的产品谈不上收寄资格）；资格项可为空清单（显式声明无硬资格），但引用必须
// 逐项合法。
func NewIntakeQualificationContent(
	sources []DeclaredIntakeSource,
	qualifications []RuleReference,
) (IntakeQualificationContent, error) {
	if len(sources) == 0 {
		return IntakeQualificationContent{}, ErrIntakeContentNotConfigured
	}
	allowed := make(map[DeclaredIntakeSource]bool, len(sources))
	for _, source := range sources {
		if !source.valid() {
			return IntakeQualificationContent{}, ErrIntakeContentNotConfigured
		}
		if allowed[source] {
			return IntakeQualificationContent{}, ErrConflictingIntakeSource
		}
		allowed[source] = true
	}
	for _, qualification := range qualifications {
		if !qualification.valid() {
			return IntakeQualificationContent{}, ErrIntakeContentNotConfigured
		}
	}
	return IntakeQualificationContent{
		allowedSources: allowed,
		qualifications: append([]RuleReference(nil), qualifications...),
	}, nil
}

// Allows 报告声明是否允许该来源。
func (content IntakeQualificationContent) Allows(source DeclaredIntakeSource) bool {
	return content.allowedSources[source]
}

// Qualifications 给出声明的硬资格清单（副本）。
func (content IntakeQualificationContent) Qualifications() []RuleReference {
	return append([]RuleReference(nil), content.qualifications...)
}

// DeclaredResponsibilityOutcome 是规则可为其声明终局的责任结果封闭四值，与
// parcel-shipment 结果联合的四格语义对应。
type DeclaredResponsibilityOutcome uint8

const (
	DeclaredResponsibilityOutcomeInvalid DeclaredResponsibilityOutcome = iota
	DeclaredEffectiveDelivery
	DeclaredReturnCompleted
	DeclaredServiceTerminated
	DeclaredRegulatoryDisposition
)

func (outcome DeclaredResponsibilityOutcome) valid() bool {
	return outcome >= DeclaredEffectiveDelivery && outcome <= DeclaredRegulatoryDisposition
}

func (outcome DeclaredResponsibilityOutcome) String() string {
	switch outcome {
	case DeclaredEffectiveDelivery:
		return "EFFECTIVE_DELIVERY"
	case DeclaredReturnCompleted:
		return "RETURN_COMPLETED"
	case DeclaredServiceTerminated:
		return "SERVICE_TERMINATED"
	case DeclaredRegulatoryDisposition:
		return "REGULATORY_DISPOSITION"
	default:
		return ""
	}
}

// FinalizationDeclaration 是一行终局声明：某种责任结果在此产品下形成哪种终局类型。
// 终局类型是开放引用——网络服务不统一规定跨产品终局集合（UC-PS-004），类型话语由
// 声明给出。
type FinalizationDeclaration struct {
	Outcome   DeclaredResponsibilityOutcome
	FinalKind RuleReference
}

// FinalRuleContent 是一个已生效规则包的终局规则声明（UC-PS-004 终局形成规则的提供方
// 半边）：哪些责任结果在此产品下形成终局、形成哪种。没有声明行的责任结果不形成终局
// ——「有效交付不在所有产品中自动等于终局」正是靠缺行表达，缺行是真话不是缺件。
type FinalRuleContent struct {
	declarations map[DeclaredResponsibilityOutcome]RuleReference
}

// NewFinalRuleContent 组装声明。至少一行（一行都没有的产品谈不上网络服务终局——
// 那是没声明不是「永不终局」）；同一责任结果声明两行是冲突。
func NewFinalRuleContent(declarations []FinalizationDeclaration) (FinalRuleContent, error) {
	if len(declarations) == 0 {
		return FinalRuleContent{}, ErrFinalContentNotConfigured
	}
	byOutcome := make(map[DeclaredResponsibilityOutcome]RuleReference, len(declarations))
	for _, declaration := range declarations {
		if !declaration.Outcome.valid() || !declaration.FinalKind.valid() {
			return FinalRuleContent{}, ErrFinalContentNotConfigured
		}
		if _, exists := byOutcome[declaration.Outcome]; exists {
			return FinalRuleContent{}, ErrConflictingFinalization
		}
		byOutcome[declaration.Outcome] = declaration.FinalKind
	}
	return FinalRuleContent{declarations: byOutcome}, nil
}

// FinalKindFor 报告该责任结果是否形成终局及形成哪种类型。第二个返回值为 false 即
// 「此产品下这种结果不形成终局」——那是声明的真话，消费方据以保持未决等其他责任
// 结果，不是配置缺件。
func (content FinalRuleContent) FinalKindFor(outcome DeclaredResponsibilityOutcome) (RuleReference, bool) {
	kind, declared := content.declarations[outcome]
	return kind, declared
}
