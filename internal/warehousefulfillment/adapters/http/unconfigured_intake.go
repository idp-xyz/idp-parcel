package warehousehttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// UnconfiguredIntake 是「接入渠道未配置」的如实答复：对每个请求不读业务内容、不采信任何自报身份、不构造命令或查询，一律交回
// ErrAccessChannelNotConfigured（ADR-0055）。它不是被禁的「开发用」采信实现：那条红线禁的是采信自报租户，本类型恰是其反面。
//
// 仓配的部署开关打开、而操作者渠道的发行方参数没设时，装配点挂它。开关关着时不挂它：那时答的是
// WAREHOUSE_FULFILLMENT_NOT_ENABLED，恢复动作是部署方打开开关，不是去配接入渠道。
type UnconfiguredIntake struct{}

var (
	_ WarehouseRegistrationIntake         = UnconfiguredIntake{}
	_ ZoneRegistrationIntake              = UnconfiguredIntake{}
	_ LocationTypeRegistrationIntake      = UnconfiguredIntake{}
	_ LocationRegistrationIntake          = UnconfiguredIntake{}
	_ PackagingMaterialRegistrationIntake = UnconfiguredIntake{}
	_ DisposalRuleRegistrationIntake      = UnconfiguredIntake{}
	_ CatalogueIntake                     = UnconfiguredIntake{}
	_ ApprovalDutyIntake                  = UnconfiguredIntake{}
	_ ShortagePolicyIntake                = UnconfiguredIntake{}
	_ AgentIntake                         = UnconfiguredIntake{}
	_ ChargeIntake                        = UnconfiguredIntake{}
	_ AllocationStrategyIntake            = UnconfiguredIntake{}
	_ GoodsOnBehalfIntake                 = UnconfiguredIntake{}
	_ PreAdviceOnBehalfIntake             = UnconfiguredIntake{}
	_ ClosePreAdviceIntake                = UnconfiguredIntake{}
	_ DiscrepancyConclusionIntake         = UnconfiguredIntake{}
	_ UnclaimedArrivalClaimIntake         = UnconfiguredIntake{}
	_ UnclaimedArrivalDispositionIntake   = UnconfiguredIntake{}
)

// IntakeWarehouseRegistration 不读请求。参数刻意匿名：连签名都不给「读一眼再决定」留位置。
func (UnconfiguredIntake) IntakeWarehouseRegistration(
	context.Context, *http.Request,
) (application.RegisterWarehouseCommand, error) {
	return application.RegisterWarehouseCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeApprovalDuty(context.Context, *http.Request) (application.RegisterApprovalDutyCommand, error) {
	return application.RegisterApprovalDutyCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeZoneRegistration(context.Context, *http.Request) (application.RegisterZoneCommand, error) {
	return application.RegisterZoneCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeLocationTypeRegistration(
	context.Context, *http.Request,
) (application.RegisterLocationTypeCommand, error) {
	return application.RegisterLocationTypeCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeLocationRegistration(
	context.Context, *http.Request,
) (application.RegisterLocationCommand, error) {
	return application.RegisterLocationCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakePackagingMaterialRegistration(
	context.Context, *http.Request,
) (application.RegisterPackagingMaterialCommand, error) {
	return application.RegisterPackagingMaterialCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeCatalogueQuery(context.Context, *http.Request) (CatalogueQuery, error) {
	return CatalogueQuery{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeGoodsOnBehalf(context.Context, *http.Request) (application.RegisterGoodsCommand, error) {
	return application.RegisterGoodsCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakePreAdvice(context.Context, *http.Request) (application.AcceptPreAdviceCommand, error) {
	return application.AcceptPreAdviceCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakePreAdviceNotice(
	context.Context, *http.Request,
) (application.UpdatePreAdviceNoticeCommand, error) {
	return application.UpdatePreAdviceNoticeCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakePreAdviceWithdrawal(
	context.Context, *http.Request,
) (application.WithdrawPreAdviceCommand, error) {
	return application.WithdrawPreAdviceCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeClosePreAdvice(
	context.Context, *http.Request,
) (application.ClosePreAdviceCommand, error) {
	return application.ClosePreAdviceCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeDisposalRuleRegistration(
	context.Context, *http.Request,
) (application.RegisterDisposalRuleCommand, error) {
	return application.RegisterDisposalRuleCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeAgentConnector(context.Context, *http.Request) (domain.TenantID, domain.WarehouseCode, error) {
	return domain.TenantID{}, domain.WarehouseCode{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeAgentStatement(context.Context, *http.Request) (domain.TenantID, ports.AgentStatement, error) {
	return domain.TenantID{}, ports.AgentStatement{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeAgentResult(context.Context, *http.Request) (domain.TenantID, ports.AgentResult, error) {
	return domain.TenantID{}, ports.AgentResult{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeChargeUnits(context.Context, *http.Request) (domain.TenantID, application.RegisterChargeUnitsCommand, error) {
	return domain.TenantID{}, application.RegisterChargeUnitsCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeChargeAging(context.Context, *http.Request) (domain.TenantID, application.RegisterChargeAgingCommand, error) {
	return domain.TenantID{}, application.RegisterChargeAgingCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeChargeProject(context.Context, *http.Request) (domain.TenantID, application.RegisterChargeProjectCommand, error) {
	return domain.TenantID{}, application.RegisterChargeProjectCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeStorageDay(context.Context, *http.Request) (domain.TenantID, application.CloseStorageDayCommand, error) {
	return domain.TenantID{}, application.CloseStorageDayCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeChargeVoid(context.Context, *http.Request) (domain.TenantID, application.ChargeCorrectionCommand, error) {
	return domain.TenantID{}, application.ChargeCorrectionCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeChargeRestate(context.Context, *http.Request) (domain.TenantID, application.ChargeCorrectionCommand, error) {
	return domain.TenantID{}, application.ChargeCorrectionCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeClaimUnclaimedArrival(
	context.Context, *http.Request,
) (application.ClaimUnclaimedArrivalCommand, error) {
	return application.ClaimUnclaimedArrivalCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeRevokeUnclaimedClaim(
	context.Context, *http.Request,
) (application.RevokeUnclaimedClaimCommand, error) {
	return application.RevokeUnclaimedClaimCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeDisposeUnclaimedArrival(
	context.Context, *http.Request,
) (application.DisposeUnclaimedArrivalCommand, error) {
	return application.DisposeUnclaimedArrivalCommand{}, ErrAccessChannelNotConfigured
}
