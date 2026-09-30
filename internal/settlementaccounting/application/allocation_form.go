package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// RegisterAllocationFormHandler 登记一条分摊规则版本选用的分法。落库时刻不是载荷的一格。
type RegisterAllocationFormHandler struct {
	forms ports.AllocationFormRegister
	clock ports.Clock
}

func NewRegisterAllocationFormHandler(
	forms ports.AllocationFormRegister,
	clock ports.Clock,
) (*RegisterAllocationFormHandler, error) {
	if forms == nil || clock == nil {
		return nil, fmt.Errorf("%w: allocation form", ErrNilDependency)
	}
	return &RegisterAllocationFormHandler{forms: forms, clock: clock}, nil
}

func (handler *RegisterAllocationFormHandler) RegisterAllocationForm(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.AllocationFormRegistration,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" {
		return 0, fmt.Errorf("%w: tenant", domain.ErrBlankValue)
	}
	return handler.forms.SaveAllocationForm(ctx, tenant, registration, handler.clock.Now())
}

// AllocationFormOutcome 是一次按已登记分法展开的结果。未配置与不适用都不是一份份额。
type AllocationFormOutcome uint8

const (
	AllocationFormOutcomeInvalid AllocationFormOutcome = iota
	AllocationFormApportioned
	AllocationFormUnconfigured
	AllocationFormNotApplicable
	AllocationFormRejected
	AllocationFormViewUnavailable
)

func (outcome AllocationFormOutcome) String() string {
	switch outcome {
	case AllocationFormApportioned:
		return "ALLOCATION_APPORTIONED"
	case AllocationFormUnconfigured:
		return "ALLOCATION_FORM_UNCONFIGURED"
	case AllocationFormNotApplicable:
		return "ALLOCATION_NOT_APPLICABLE"
	case AllocationFormRejected:
		return "ALLOCATION_FORM_REJECTED"
	case AllocationFormViewUnavailable:
		return "ALLOCATION_FORM_VIEW_UNAVAILABLE"
	default:
		return ""
	}
}

// AllocationBasisInput 是这次分摊交入的一个对象权重。产品不预填。
type AllocationBasisInput struct {
	Target string
	Basis  int64
}

// ApportionCostsCommand 携带一次按已登记分法的展开。份额不在命令里，由执行器算出。
type ApportionCostsCommand struct {
	TenantID    domain.TenantID
	RuleVersion string
	SourceMinor int64
	Bases       []AllocationBasisInput
}

// AllocationFormResult 交回展开，或停在未配置 / 不适用 / 未决。没登记时份额为空。
type AllocationFormResult struct {
	outcome       AllocationFormOutcome
	apportionment domain.AllocationApportionment
	has           bool
}

func (result AllocationFormResult) Outcome() AllocationFormOutcome { return result.outcome }

func (result AllocationFormResult) Apportionment() (domain.AllocationApportionment, bool) {
	return result.apportionment, result.has
}

// ApportionCostsHandler 按租户登记的分法展开份额。册上没有这一行答未配置，不用均摊顶上。
// 算出的份额交给 AllocateCosts 的 Portions；本编排不写分摊结果。
type ApportionCostsHandler struct {
	forms ports.AllocationFormView
}

func NewApportionCostsHandler(forms ports.AllocationFormView) (*ApportionCostsHandler, error) {
	if forms == nil {
		return nil, fmt.Errorf("%w: allocation form view", ErrNilDependency)
	}
	return &ApportionCostsHandler{forms: forms}, nil
}

func (handler *ApportionCostsHandler) Apportion(
	ctx context.Context,
	command ApportionCostsCommand,
) (AllocationFormResult, error) {
	rule, err := domain.NewAllocationRuleVersionReference(command.RuleVersion)
	if err != nil || command.TenantID.String() == "" {
		return AllocationFormResult{outcome: AllocationFormRejected}, nil
	}
	form, found, err := handler.forms.LoadAllocationForm(ctx, command.TenantID, rule)
	if err != nil {
		return AllocationFormResult{outcome: AllocationFormViewUnavailable}, nil
	}
	if !found {
		return AllocationFormResult{outcome: AllocationFormUnconfigured}, nil
	}
	if form == domain.AllocationNotApplicable {
		return AllocationFormResult{outcome: AllocationFormNotApplicable}, nil
	}
	bases := make([]domain.AllocationBasis, 0, len(command.Bases))
	for _, input := range command.Bases {
		target, err := domain.NewAllocationTargetReference(input.Target)
		if err != nil {
			return AllocationFormResult{outcome: AllocationFormRejected}, nil
		}
		bases = append(bases, domain.AllocationBasis{Target: target, Basis: input.Basis})
	}
	apportionment, err := domain.Apportion(form, command.SourceMinor, bases)
	if errors.Is(err, domain.ErrAllocationFormOverflow) {
		return AllocationFormResult{}, err
	}
	if err != nil {
		return AllocationFormResult{outcome: AllocationFormRejected}, nil
	}
	return AllocationFormResult{
		outcome:       AllocationFormApportioned,
		apportionment: apportionment,
		has:           true,
	}, nil
}
