package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidDutyCollaboration = errors.New("customs compliance: invalid duty payment collaboration")
	ErrCollaborationNotFundable = errors.New("customs compliance: nothing to pay and no basis to say so")
)

// DutyObligationKind 是税费付款协作事项的义务依据封闭二值：已接受监管核定税费，或
// 真实程序对当前范围明确形成的无需付款依据（CONTEXT「税费付款协作事项」语言）。
// 「缺少税费结果不能被解释为无需付款」——第三种「没有结果所以不用付」在类型上没有格。
type DutyObligationKind uint8

const (
	DutyObligationKindInvalid DutyObligationKind = iota
	ObligationFromAssessedDuty
	ObligationExplicitlyNotRequired
)

func (kind DutyObligationKind) valid() bool {
	return kind == ObligationFromAssessedDuty || kind == ObligationExplicitlyNotRequired
}

func (kind DutyObligationKind) String() string {
	switch kind {
	case ObligationFromAssessedDuty:
		return "ASSESSED_DUTY"
	case ObligationExplicitlyNotRequired:
		return "EXPLICITLY_NOT_REQUIRED"
	default:
		return ""
	}
}

// LegalObligorReference 指名法定义务人。法定义务人、实际付款方和最终承担费用的客户
// 可以不同，不能互相推导（CONTEXT「不能互相推导」）——这里只记法定义务人，另两个各归其
// 所有者。
type LegalObligorReference struct{ requiredValue }

func NewLegalObligorReference(value string) (LegalObligorReference, error) {
	required, err := newRequiredValue("legal obligor reference", value)
	return LegalObligorReference{required}, err
}

// PaymentRequirementSource 指名付款要求来源（监管程序或核定文书）。
type PaymentRequirementSource struct{ requiredValue }

func NewPaymentRequirementSource(value string) (PaymentRequirementSource, error) {
	required, err := newRequiredValue("payment requirement source", value)
	return PaymentRequirementSource{required}, err
}

// ResponsibilityTargetReference 指名责任交接目标（谁去安排付款协作）。
type ResponsibilityTargetReference struct{ requiredValue }

func NewResponsibilityTargetReference(value string) (ResponsibilityTargetReference, error) {
	required, err := newRequiredValue("responsibility target reference", value)
	return ResponsibilityTargetReference{required}, err
}

// DutyCollaborationSpec 是形成一项税费付款协作事项所需的全部输入。
type DutyCollaborationSpec struct {
	Kind        DutyObligationKind
	Duty        AssessedDutyReference
	NoPayBasis  string
	Scope       DecisionScopeReference
	Obligor     LegalObligorReference
	Requirement PaymentRequirementSource
	Target      ResponsibilityTargetReference
	FormedAt    time.Time
}

// DutyPaymentCollaboration 是依据已接受监管核定税费（或明确无需付款依据）针对明确
// 申报范围形成的内部协作对象。它固定税费义务依据、法定义务范围、付款要求来源、责任
// 交接目标和核对入口，但不等于支付指令、付款交易、客户回收或监管放行——类型上没有
// 那些字段（CONTEXT「税费付款协作事项」语言逐句）。
type DutyPaymentCollaboration struct {
	kind        DutyObligationKind
	duty        AssessedDutyReference
	noPayBasis  string
	scope       DecisionScopeReference
	obligor     LegalObligorReference
	requirement PaymentRequirementSource
	target      ResponsibilityTargetReference
	formedAt    time.Time
}

// FormDutyCollaboration 形成协作事项。两格各有形状：核定税费格必带税费引用且不带
// 无需付款依据；明确无需付款格必带真实程序依据且不带税费引用——「缺少税费结果」
// 走不进任何一格（ErrCollaborationNotFundable 独立哨兵，编排据以保持未决，不形成
// 支付指令也不解释为无需付款）。
func FormDutyCollaboration(spec DutyCollaborationSpec) (DutyPaymentCollaboration, error) {
	if !spec.Scope.valid() ||
		!spec.Obligor.valid() ||
		!spec.Requirement.valid() ||
		!spec.Target.valid() ||
		spec.FormedAt.IsZero() {
		return DutyPaymentCollaboration{}, ErrInvalidDutyCollaboration
	}
	switch spec.Kind {
	case ObligationFromAssessedDuty:
		if !spec.Duty.valid() || spec.NoPayBasis != "" {
			return DutyPaymentCollaboration{}, ErrInvalidDutyCollaboration
		}
	case ObligationExplicitlyNotRequired:
		if spec.NoPayBasis == "" || spec.Duty.valid() {
			return DutyPaymentCollaboration{}, ErrInvalidDutyCollaboration
		}
	default:
		return DutyPaymentCollaboration{}, ErrCollaborationNotFundable
	}
	return DutyPaymentCollaboration{
		kind:        spec.Kind,
		duty:        spec.Duty,
		noPayBasis:  spec.NoPayBasis,
		scope:       spec.Scope,
		obligor:     spec.Obligor,
		requirement: spec.Requirement,
		target:      spec.Target,
		formedAt:    spec.FormedAt.UTC(),
	}, nil
}

func (collaboration DutyPaymentCollaboration) Kind() DutyObligationKind {
	return collaboration.kind
}

// Duty 只在核定税费格给出。
func (collaboration DutyPaymentCollaboration) Duty() (AssessedDutyReference, bool) {
	return collaboration.duty, collaboration.kind == ObligationFromAssessedDuty
}

// NoPayBasis 只在明确无需付款格给出。
func (collaboration DutyPaymentCollaboration) NoPayBasis() (string, bool) {
	return collaboration.noPayBasis, collaboration.kind == ObligationExplicitlyNotRequired
}

func (collaboration DutyPaymentCollaboration) Scope() DecisionScopeReference {
	return collaboration.scope
}

func (collaboration DutyPaymentCollaboration) Obligor() LegalObligorReference {
	return collaboration.obligor
}

func (collaboration DutyPaymentCollaboration) Requirement() PaymentRequirementSource {
	return collaboration.requirement
}

func (collaboration DutyPaymentCollaboration) Target() ResponsibilityTargetReference {
	return collaboration.target
}

func (collaboration DutyPaymentCollaboration) FormedAt() time.Time {
	return collaboration.formedAt
}
