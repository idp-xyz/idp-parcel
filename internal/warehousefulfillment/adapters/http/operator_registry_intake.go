package warehousehttp

import (
	"context"
	"errors"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// OperatorIdentity 是一次登记册配置写出示认证出的身份：操作者绑定的租户，与登记责任方——取操作者的主体引用，不取自载荷
// （UC-WF-001「输入」）。
type OperatorIdentity struct {
	Tenant     domain.TenantID
	Registrant domain.Registrant
}

// ApprovalDutyIntake 是批准职责规则登记口的 Intake。
type ApprovalDutyIntake interface {
	IntakeApprovalDuty(ctx context.Context, request *http.Request) (application.RegisterApprovalDutyCommand, error)
}

// OperatorRegistryAuthenticator 认证一次登记册配置写出示，交回认证出的身份。失败只交回本包的格；从共享接入身份能力译过来
// 的那一层在 adapters/accessidentity（跨上下文翻译的位置，见 internal/architecture 的边界门禁）。
type OperatorRegistryAuthenticator interface {
	AuthenticateRegistryWrite(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

// OperatorRegistryIntake 是操作者渠道在本包登记册配置写面的 Intake（ADR-0100 决定四），各册的登记口共用这一只：一个认证器，
// 每册一个方法。登记一个修订不形成生产事实，所以不判准入范围。先认证、后读请求体；租户与登记责任方填认证结果，报文里这两格
// 在场即拒。
type OperatorRegistryIntake struct {
	authenticator OperatorRegistryAuthenticator
}

var (
	_ WarehouseRegistrationIntake         = (*OperatorRegistryIntake)(nil)
	_ ZoneRegistrationIntake              = (*OperatorRegistryIntake)(nil)
	_ LocationTypeRegistrationIntake      = (*OperatorRegistryIntake)(nil)
	_ LocationRegistrationIntake          = (*OperatorRegistryIntake)(nil)
	_ PackagingMaterialRegistrationIntake = (*OperatorRegistryIntake)(nil)
	_ ApprovalDutyIntake                  = (*OperatorRegistryIntake)(nil)
	_ ShortagePolicyIntake                = (*OperatorRegistryIntake)(nil)
	_ AgentIntake                         = (*OperatorRegistryIntake)(nil)
	_ ChargeIntake                        = (*OperatorRegistryIntake)(nil)
	_ WarehouseAdoptionIntake             = (*OperatorRegistryIntake)(nil)
	_ AllocationStrategyIntake            = (*OperatorRegistryIntake)(nil)
	_ DisposalRuleRegistrationIntake      = (*OperatorRegistryIntake)(nil)
)

func NewOperatorRegistryIntake(authenticator OperatorRegistryAuthenticator) (*OperatorRegistryIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: operator registry intake needs an authenticator")
	}
	return &OperatorRegistryIntake{authenticator: authenticator}, nil
}

func (intake *OperatorRegistryIntake) IntakeWarehouseRegistration(
	ctx context.Context,
	request *http.Request,
) (application.RegisterWarehouseCommand, error) {
	return intakeRegistration(ctx, request, intake.authenticator, registrationjson.WarehouseCommand)
}

func (intake *OperatorRegistryIntake) IntakeZoneRegistration(
	ctx context.Context,
	request *http.Request,
) (application.RegisterZoneCommand, error) {
	return intakeRegistration(ctx, request, intake.authenticator, registrationjson.ZoneCommand)
}

func (intake *OperatorRegistryIntake) IntakeLocationTypeRegistration(
	ctx context.Context,
	request *http.Request,
) (application.RegisterLocationTypeCommand, error) {
	return intakeRegistration(ctx, request, intake.authenticator, registrationjson.LocationTypeCommand)
}

func (intake *OperatorRegistryIntake) IntakeLocationRegistration(
	ctx context.Context,
	request *http.Request,
) (application.RegisterLocationCommand, error) {
	return intakeRegistration(ctx, request, intake.authenticator, registrationjson.LocationCommand)
}

func (intake *OperatorRegistryIntake) IntakePackagingMaterialRegistration(
	ctx context.Context,
	request *http.Request,
) (application.RegisterPackagingMaterialCommand, error) {
	return intakeRegistration(ctx, request, intake.authenticator, registrationjson.PackagingMaterialCommand)
}

func (intake *OperatorRegistryIntake) IntakeDisposalRuleRegistration(
	ctx context.Context,
	request *http.Request,
) (application.RegisterDisposalRuleCommand, error) {
	return intakeRegistration(ctx, request, intake.authenticator, registrationjson.UnclaimedDisposalRuleCommand)
}

// AgentIntake 是代理仓连接器、声明与结果的登记 Intake。凭据不进载荷。
type AgentIntake interface {
	IntakeAgentConnector(ctx context.Context, request *http.Request) (domain.TenantID, domain.WarehouseCode, error)
	IntakeAgentStatement(ctx context.Context, request *http.Request) (domain.TenantID, ports.AgentStatement, error)
	IntakeAgentResult(ctx context.Context, request *http.Request) (domain.TenantID, ports.AgentResult, error)
}

// IntakeApprovalDuty 先认证再读批准职责规则。登记不判准入范围。
func (intake *OperatorRegistryIntake) IntakeAgentConnector(ctx context.Context, request *http.Request) (domain.TenantID, domain.WarehouseCode, error) {
	var none domain.WarehouseCode
	identity, err := intake.authenticator.AuthenticateRegistryWrite(ctx, httpapi.BearerToken(request))
	if err != nil {
		return domain.TenantID{}, none, err
	}
	warehouse, err := registrationjson.AgentConnectorWarehouse(bodyOrEmpty(request), identity.Tenant, identity.Registrant)
	if err != nil {
		return domain.TenantID{}, none, asMalformed(err)
	}
	return identity.Tenant, warehouse, nil
}

func (intake *OperatorRegistryIntake) IntakeAgentStatement(ctx context.Context, request *http.Request) (domain.TenantID, ports.AgentStatement, error) {
	identity, err := intake.authenticator.AuthenticateRegistryWrite(ctx, httpapi.BearerToken(request))
	if err != nil {
		return domain.TenantID{}, ports.AgentStatement{}, err
	}
	statement, err := registrationjson.AgentStatement(bodyOrEmpty(request), identity.Tenant, identity.Registrant)
	if err != nil {
		return domain.TenantID{}, ports.AgentStatement{}, asMalformed(err)
	}
	return identity.Tenant, statement, nil
}

func (intake *OperatorRegistryIntake) IntakeAgentResult(ctx context.Context, request *http.Request) (domain.TenantID, ports.AgentResult, error) {
	identity, err := intake.authenticator.AuthenticateRegistryWrite(ctx, httpapi.BearerToken(request))
	if err != nil {
		return domain.TenantID{}, ports.AgentResult{}, err
	}
	result, err := registrationjson.AgentResult(bodyOrEmpty(request), identity.Tenant, identity.Registrant)
	if err != nil {
		return domain.TenantID{}, ports.AgentResult{}, asMalformed(err)
	}
	return identity.Tenant, result, nil
}

func (intake *OperatorRegistryIntake) IntakeWarehouseAdoption(ctx context.Context, request *http.Request) (domain.TenantID, WarehouseAdoptionCommand, error) {
	identity, err := intake.authenticator.AuthenticateRegistryWrite(ctx, httpapi.BearerToken(request))
	if err != nil {
		return domain.TenantID{}, WarehouseAdoptionCommand{}, err
	}
	command, err := decodeWarehouseAdoption(bodyOrEmpty(request))
	if err != nil {
		return domain.TenantID{}, WarehouseAdoptionCommand{}, err
	}
	if command.Registrant == "" {
		command.Registrant = identity.Registrant.String()
	}
	return identity.Tenant, command, nil
}

func (intake *OperatorRegistryIntake) IntakeChargeUnits(ctx context.Context, request *http.Request) (domain.TenantID, application.RegisterChargeUnitsCommand, error) {
	return intakeCharge(ctx, request, intake.authenticator, registrationjson.ChargeUnits)
}

func (intake *OperatorRegistryIntake) IntakeChargeAging(ctx context.Context, request *http.Request) (domain.TenantID, application.RegisterChargeAgingCommand, error) {
	return intakeCharge(ctx, request, intake.authenticator, registrationjson.ChargeAging)
}

func (intake *OperatorRegistryIntake) IntakeChargeProject(ctx context.Context, request *http.Request) (domain.TenantID, application.RegisterChargeProjectCommand, error) {
	return intakeCharge(ctx, request, intake.authenticator, registrationjson.ChargeProject)
}

func (intake *OperatorRegistryIntake) IntakeStorageDay(ctx context.Context, request *http.Request) (domain.TenantID, application.CloseStorageDayCommand, error) {
	return intakeCharge(ctx, request, intake.authenticator, registrationjson.StorageDay)
}

func (intake *OperatorRegistryIntake) IntakeChargeVoid(ctx context.Context, request *http.Request) (domain.TenantID, application.ChargeCorrectionCommand, error) {
	return intakeCharge(ctx, request, intake.authenticator, registrationjson.ChargeVoid)
}

func (intake *OperatorRegistryIntake) IntakeChargeRestate(ctx context.Context, request *http.Request) (domain.TenantID, application.ChargeCorrectionCommand, error) {
	return intakeCharge(ctx, request, intake.authenticator, registrationjson.ChargeRestate)
}

func intakeCharge[Command any](
	ctx context.Context,
	request *http.Request,
	authenticator OperatorRegistryAuthenticator,
	translate func(io.Reader, domain.TenantID, domain.Registrant) (Command, error),
) (domain.TenantID, Command, error) {
	var none Command
	identity, err := authenticator.AuthenticateRegistryWrite(ctx, httpapi.BearerToken(request))
	if err != nil {
		return domain.TenantID{}, none, err
	}
	command, err := translate(bodyOrEmpty(request), identity.Tenant, identity.Registrant)
	if err != nil {
		return domain.TenantID{}, none, asMalformed(err)
	}
	return identity.Tenant, command, nil
}

func (intake *OperatorRegistryIntake) IntakeApprovalDuty(ctx context.Context, request *http.Request) (application.RegisterApprovalDutyCommand, error) {
	return intakeRegistration(ctx, request, intake.authenticator, registrationjson.ApprovalDutyCommand)
}

// intakeRegistration 是各册登记 Intake 共用的一步：令牌不过的请求，请求体一个字节都不读；过了才把请求体按这一册的译法译成命令，
// 租户与登记责任方填认证结果。译装交回的坏载荷译成本包的请求畸形，别的错原样上抛。
func intakeRegistration[Command any](
	ctx context.Context,
	request *http.Request,
	authenticator OperatorRegistryAuthenticator,
	translate func(io.Reader, domain.TenantID, domain.Registrant) (Command, error),
) (Command, error) {
	var none Command
	identity, err := authenticator.AuthenticateRegistryWrite(ctx, httpapi.BearerToken(request))
	if err != nil {
		return none, err
	}
	body := request.Body
	if body == nil {
		body = http.NoBody
	}
	command, err := translate(body, identity.Tenant, identity.Registrant)
	if err != nil {
		var malformed *registrationjson.MalformedDocumentError
		if errors.As(err, &malformed) {
			return none, &malformedRequest{reason: malformed.Reason}
		}
		return none, err
	}
	return command, nil
}
