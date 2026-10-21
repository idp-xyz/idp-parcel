package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// RegisterSellBillingMomentAdoptionCommand 是一行 SELL 计费时点采用的登记（ADR-0190 决定三）。租户在命令上，不从上下文里补。
type RegisterSellBillingMomentAdoptionCommand struct {
	Tenant   domain.TenantID
	Adoption domain.SellBillingMomentAdoption
}

// RegisterSellBillingMomentAdoptionHandler 把一行采用交给采用册，并在同一事务里交出「评价参数已登记」信封：没登记时到来的
// 那一拍停在发起停办册上，补登之后由它续办（ADR-0229 决定四、五）。落库时刻由时钟给出。
type RegisterSellBillingMomentAdoptionHandler struct {
	adoptions ports.SellBillingMomentAdoptionRegister
	handoff   ports.EvaluationParameterRegisteredHandoff
	clock     ports.Clock
}

// NewRegisterSellBillingMomentAdoptionHandler 三件都不许为 nil：交接口缺席时登记照样成功，停着的那一拍却再也等不来续办——
// ADR-0094 决定四说的「更安静的永久停滞」。
func NewRegisterSellBillingMomentAdoptionHandler(
	adoptions ports.SellBillingMomentAdoptionRegister,
	handoff ports.EvaluationParameterRegisteredHandoff,
	clock ports.Clock,
) (*RegisterSellBillingMomentAdoptionHandler, error) {
	if adoptions == nil || handoff == nil || clock == nil {
		return nil, fmt.Errorf("%w: sell billing moment adoption register", ErrNilDependency)
	}
	return &RegisterSellBillingMomentAdoptionHandler{adoptions: adoptions, handoff: handoff, clock: clock}, nil
}

func (handler *RegisterSellBillingMomentAdoptionHandler) Register(
	ctx context.Context,
	command RegisterSellBillingMomentAdoptionCommand,
) (ports.CatalogueRegistrationEffect, error) {
	if command.Tenant.String() == "" || command.Adoption.Account().String() == "" {
		return 0, fmt.Errorf("%w: sell billing moment adoption command", domain.ErrBlankValue)
	}
	registeredAt := handler.clock.Now()
	effect, err := handler.adoptions.SaveSellBillingMomentAdoption(ctx, command.Tenant, command.Adoption, registeredAt)
	if err != nil {
		return 0, fmt.Errorf("register sell billing moment adoption: %w", err)
	}
	if err := announceEvaluationParameter(ctx, handler.handoff, effect, ports.EvaluationParameterRegisteredIntent{
		Tenant:       command.Tenant,
		Kind:         domain.ParameterSellBillingMomentAdoption,
		Subject:      command.Adoption.Account().String(),
		RegisteredAt: registeredAt,
	}); err != nil {
		return 0, fmt.Errorf("register sell billing moment adoption: %w", err)
	}
	return effect, nil
}

// announceEvaluationParameter 按落点交「评价参数已登记」：`已登记`与`重放`都交（重放重发同一封，由适配器按同一个 ID
// 去重；本口落地之前登进册的行也借重放补上它那一封），`冲突`不交——冲突那一行没写进册，续办只会再撞见同一个缺口。
// 交接失败上抛，信封与登记行同生共死。
func announceEvaluationParameter(
	ctx context.Context,
	handoff ports.EvaluationParameterRegisteredHandoff,
	effect ports.CatalogueRegistrationEffect,
	intent ports.EvaluationParameterRegisteredIntent,
) error {
	switch effect {
	case ports.CatalogueRegistered, ports.CatalogueReplay:
		return handoff.HandOffEvaluationParameterRegistered(ctx, intent)
	case ports.CatalogueConflict:
		return nil
	default:
		return fmt.Errorf("unknown registration effect %d", effect)
	}
}

// RegisterFeeItemMappingHandler 登记一行费用项目映射（ADR-0190 决定五），同一事务交「评价参数已登记」：映射不全而停在
// `待判断`的 SELL 请求由它续办（ADR-0229 决定五）。只增不改的判定在册上，这里不重判。
type RegisterFeeItemMappingHandler struct {
	mappings ports.FeeItemMappingRegister
	handoff  ports.EvaluationParameterRegisteredHandoff
	clock    ports.Clock
}

func NewRegisterFeeItemMappingHandler(
	mappings ports.FeeItemMappingRegister,
	handoff ports.EvaluationParameterRegisteredHandoff,
	clock ports.Clock,
) (*RegisterFeeItemMappingHandler, error) {
	if mappings == nil || handoff == nil || clock == nil {
		return nil, fmt.Errorf("%w: fee item mapping register", ErrNilDependency)
	}
	return &RegisterFeeItemMappingHandler{mappings: mappings, handoff: handoff, clock: clock}, nil
}

func (handler *RegisterFeeItemMappingHandler) RegisterFeeItemMapping(
	ctx context.Context,
	tenant domain.TenantID,
	mapping domain.FeeItemMapping,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" || mapping.ChargeCode().String() == "" {
		return 0, fmt.Errorf("%w: fee item mapping command", domain.ErrBlankValue)
	}
	registeredAt := handler.clock.Now()
	effect, err := handler.mappings.SaveFeeItemMapping(ctx, tenant, mapping, registeredAt)
	if err != nil {
		return 0, fmt.Errorf("register fee item mapping: %w", err)
	}
	if err := announceEvaluationParameter(ctx, handler.handoff, effect, ports.EvaluationParameterRegisteredIntent{
		Tenant:       tenant,
		Kind:         domain.ParameterFeeItemMapping,
		Subject:      mapping.ChargeCode().String(),
		RegisteredAt: registeredAt,
	}); err != nil {
		return 0, fmt.Errorf("register fee item mapping: %w", err)
	}
	return effect, nil
}
