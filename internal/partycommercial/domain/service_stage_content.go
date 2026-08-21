package domain

import (
	"errors"
	"sort"
)

var (
	// ErrIntakeContentNotConfigured 是收寄资格声明缺件：允许来源为空或资格项非法。
	// 恢复动作是把规则正文声明补齐（PAR-COM-16 实例半边），不是本上下文代拟默认。
	ErrIntakeContentNotConfigured = errors.New("party commercial: intake qualification content is not configured")
	// ErrFinalContentNotConfigured 是终局规则声明缺件。恢复动作同上（PAR-COM-17）。
	ErrFinalContentNotConfigured = errors.New("party commercial: final rule content is not configured")
	// ErrCancellationAuthorityNotConfigured 是取消授权目录缺件。恢复动作同上
	// （PAR-COM-17 实例半边），不是本上下文代拟「客户可取消」。
	ErrCancellationAuthorityNotConfigured = errors.New("party commercial: cancellation authority content is not configured")
	ErrConflictingIntakeSource            = errors.New("party commercial: conflicting intake source declaration")
	ErrConflictingFinalization            = errors.New("party commercial: conflicting finalization declaration")
	ErrConflictingCancellationAuthority   = errors.New("party commercial: conflicting cancellation authority declaration")
	// ErrUnusableAuthorizationRule：非已生效授权规则承载不了取消授权目录。与
	// ErrUnusableRulePackage 同判据同恢复动作（换当前可用的版本），只是拥有对象不同。
	ErrUnusableAuthorizationRule = errors.New("party commercial: authorization rule cannot carry declarations")
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

// IntakeQualificationContent 是一个已生效接单规则包的收寄阶段资格声明（UC-PS-003
// 资格核对第 5 条的提供方半边）：允许哪些收寄来源、要过哪些硬资格。产品与合同是采用
// 方，不拥有这份正文（ADR-0058）。资格项是开放引用——目录属实例参数；声明为空集不是
// 「没有资格要求」而是没声明（缺件错误），真没有要求也要显式声明空清单带来源允许。
type IntakeQualificationContent struct {
	owner          CommercialVersion
	allowedSources map[DeclaredIntakeSource]bool
	qualifications []RuleReference
}

// NewIntakeQualificationContent 组装声明。拥有对象必须是当前可用的接单规则包；允许
// 来源至少一格且不重（一个收寄模式都不允许的产品谈不上收寄资格）；资格项可为空清单
// （显式声明无硬资格），但引用必须逐项合法。
func NewIntakeQualificationContent(
	owner CommercialVersion,
	sources []DeclaredIntakeSource,
	qualifications []RuleReference,
) (IntakeQualificationContent, error) {
	if owner.kind != AcceptanceRulePackageObject ||
		owner.status != CommercialVersionEffective {
		return IntakeQualificationContent{}, ErrUnusableRulePackage
	}
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
		owner:          owner,
		allowedSources: allowed,
		qualifications: append([]RuleReference(nil), qualifications...),
	}, nil
}

func (content IntakeQualificationContent) Owner() CommercialVersion {
	return content.owner
}

// Allows 报告声明是否允许该来源。
func (content IntakeQualificationContent) Allows(source DeclaredIntakeSource) bool {
	return content.allowedSources[source]
}

// AllowedSources 给出声明允许的来源（按名称排序的副本）。
func (content IntakeQualificationContent) AllowedSources() []DeclaredIntakeSource {
	sources := make([]DeclaredIntakeSource, 0, len(content.allowedSources))
	for source := range content.allowedSources {
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].String() < sources[j].String()
	})
	return sources
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

// FinalizationDeclaration 是一行终局声明：某种责任结果在此规则包下形成哪种终局类型。
// 终局类型是开放引用——网络服务不统一规定跨产品终局集合（UC-PS-004），类型话语由
// 声明给出。产品与合同采用这份规则包之后，缺行在采用层读成「此产品下不形成终局」。
type FinalizationDeclaration struct {
	Outcome   DeclaredResponsibilityOutcome
	FinalKind RuleReference
}

// FinalRuleContent 是一个已生效接单规则包的终局规则声明（UC-PS-004 终局形成规则的
// 提供方半边）：哪些责任结果在此规则包下形成终局、形成哪种。产品与合同是采用方，不
// 拥有这份正文（ADR-0058）。没有声明行的责任结果不形成终局——「有效交付不在所有产品
// 中自动等于终局」正是靠缺行表达，缺行是真话不是缺件。
type FinalRuleContent struct {
	owner        CommercialVersion
	declarations map[DeclaredResponsibilityOutcome]RuleReference
}

// NewFinalRuleContent 组装声明。拥有对象必须是当前可用的接单规则包；至少一行（一行
// 都没有的规则包谈不上网络服务终局——那是没声明不是「永不终局」）；同一责任结果声明
// 两行是冲突。
func NewFinalRuleContent(
	owner CommercialVersion,
	declarations []FinalizationDeclaration,
) (FinalRuleContent, error) {
	if owner.kind != AcceptanceRulePackageObject ||
		owner.status != CommercialVersionEffective {
		return FinalRuleContent{}, ErrUnusableRulePackage
	}
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
	return FinalRuleContent{owner: owner, declarations: byOutcome}, nil
}

func (content FinalRuleContent) Owner() CommercialVersion {
	return content.owner
}

// FinalKindFor 报告该责任结果是否形成终局及形成哪种类型。第二个返回值为 false 即
// 「此规则包下这种结果不形成终局」——那是声明的真话，消费方据以保持未决等其他责任
// 结果，不是配置缺件。
func (content FinalRuleContent) FinalKindFor(outcome DeclaredResponsibilityOutcome) (RuleReference, bool) {
	kind, declared := content.declarations[outcome]
	return kind, declared
}

// Declarations 按责任结果的稳定顺序交回全部终局声明（副本）。发布写入面按整份声明
// 登记，需要枚举；逐结果取用仍走 FinalKindFor。
func (content FinalRuleContent) Declarations() []FinalizationDeclaration {
	declarations := make([]FinalizationDeclaration, 0, len(content.declarations))
	for outcome, kind := range content.declarations {
		declarations = append(declarations, FinalizationDeclaration{Outcome: outcome, FinalKind: kind})
	}
	sort.Slice(declarations, func(left, right int) bool {
		return declarations[left].Outcome < declarations[right].Outcome
	})
	return declarations
}

// DeclaredCancellationParty 是规则可声明的取消请求方封闭二值，对应 CONTEXT
// 「客户或授权运营角色」。零值不合法。将来若出现第三格，扩本封闭集，不开活口。
type DeclaredCancellationParty uint8

const (
	DeclaredCancellationPartyInvalid DeclaredCancellationParty = iota
	DeclaredCustomerCancellation
	DeclaredOperationsCancellation
)

func (party DeclaredCancellationParty) valid() bool {
	return party == DeclaredCustomerCancellation || party == DeclaredOperationsCancellation
}

func (party DeclaredCancellationParty) String() string {
	switch party {
	case DeclaredCustomerCancellation:
		return "CUSTOMER"
	case DeclaredOperationsCancellation:
		return "OPERATIONS"
	default:
		return ""
	}
}

// CancellationAuthorityDeclaration 是一行取消授权：哪种请求方格被允许，依据哪条规则。
type CancellationAuthorityDeclaration struct {
	Party DeclaredCancellationParty
	Rule  RuleReference
}

// CancellationAuthorityContent 是一个已生效授权规则的取消授权目录（PAR-COM-17
// 提供方半边）。产品与合同是采用方，不拥有这份正文（ADR-0058）。目录按请求方格说话：
// 有行即允许并带规则引用；缺行是真话（此授权规则下这种请求方不许取消），不是配置缺件。
// 零行才是缺件——没声明不等于「谁都不许」，更不等于默认放行。
type CancellationAuthorityContent struct {
	owner        CommercialVersion
	declarations map[DeclaredCancellationParty]RuleReference
}

// NewCancellationAuthorityContent 组装目录。拥有对象必须是当前可用的授权规则；至少
// 一行（一行都没有是没声明，不是「永不允许」）；同一请求方格两行是冲突。
func NewCancellationAuthorityContent(
	owner CommercialVersion,
	declarations []CancellationAuthorityDeclaration,
) (CancellationAuthorityContent, error) {
	if owner.kind != AuthorizationRuleObject ||
		owner.status != CommercialVersionEffective {
		return CancellationAuthorityContent{}, ErrUnusableAuthorizationRule
	}
	if len(declarations) == 0 {
		return CancellationAuthorityContent{}, ErrCancellationAuthorityNotConfigured
	}
	byParty := make(map[DeclaredCancellationParty]RuleReference, len(declarations))
	for _, declaration := range declarations {
		if !declaration.Party.valid() || !declaration.Rule.valid() {
			return CancellationAuthorityContent{}, ErrCancellationAuthorityNotConfigured
		}
		if _, exists := byParty[declaration.Party]; exists {
			return CancellationAuthorityContent{}, ErrConflictingCancellationAuthority
		}
		byParty[declaration.Party] = declaration.Rule
	}
	return CancellationAuthorityContent{owner: owner, declarations: byParty}, nil
}

func (content CancellationAuthorityContent) Owner() CommercialVersion {
	return content.owner
}

// RuleFor 报告该请求方格是否被允许取消及依据哪条规则。第二个返回值为 false 即
// 「此授权规则下这种请求方不许取消」——那是声明的真话，消费方据以拒绝带依据，不是配置缺件。
func (content CancellationAuthorityContent) RuleFor(party DeclaredCancellationParty) (RuleReference, bool) {
	rule, declared := content.declarations[party]
	return rule, declared
}

// Declarations 按请求方的稳定顺序交回整份目录（副本）。发布写入面按整份目录登记，
// 需要枚举；逐请求方取用仍走 RuleFor。
func (content CancellationAuthorityContent) Declarations() []CancellationAuthorityDeclaration {
	declarations := make([]CancellationAuthorityDeclaration, 0, len(content.declarations))
	for party, rule := range content.declarations {
		declarations = append(declarations, CancellationAuthorityDeclaration{Party: party, Rule: rule})
	}
	sort.Slice(declarations, func(left, right int) bool {
		return declarations[left].Party < declarations[right].Party
	})
	return declarations
}
