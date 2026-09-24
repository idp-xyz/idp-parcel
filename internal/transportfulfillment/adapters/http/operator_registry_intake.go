package tfhttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// OperatorRegistryAuthenticator 认证一次登记册配置写出示，交回操作者绑定的租户。登记写面不判准入范围（ADR-0100
// 决定四），所以它交不出「不在准入范围」那一格；其余三格与运营决定口共用本包的哨兵。
type OperatorRegistryAuthenticator interface {
	AuthenticateRegistryWrite(ctx context.Context, bearerToken string) (domain.TenantID, error)
}

// OperatorRegistryIntake 是操作者渠道在本包外部承运凭证、有效时间规则与总单三本册子五口的 Intake（ADR-0100
// 决定四；票 operator-channel/04）。三本册子都是逐字段表单（ADR-0101 决定八），线格式就是各自的载荷类型，租户取
// 认证结果；本包没有登记 CLI，载荷只此一份。先认证、后解载荷。
type OperatorRegistryIntake struct {
	authenticator OperatorRegistryAuthenticator
}

var (
	_ CredentialIntake        = (*OperatorRegistryIntake)(nil)
	_ EffectiveTimeRuleIntake = (*OperatorRegistryIntake)(nil)
	_ MasterDocumentIntake    = (*OperatorRegistryIntake)(nil)
)

func NewOperatorRegistryIntake(authenticator OperatorRegistryAuthenticator) (*OperatorRegistryIntake, error) {
	if authenticator == nil {
		return nil, errors.New("transport fulfillment http: operator registry intake needs an authenticator")
	}
	return &OperatorRegistryIntake{authenticator: authenticator}, nil
}

func (intake *OperatorRegistryIntake) IntakeCredentialRegistration(ctx context.Context, request *http.Request) (application.RegisterExternalCarrierCredentialCommand, error) {
	return registryCommand(ctx, intake, request, CredentialRegistrationPayload.Command)
}

func (intake *OperatorRegistryIntake) IntakeCredentialApplicabilityChange(ctx context.Context, request *http.Request) (application.ChangeCredentialApplicabilityCommand, error) {
	return registryCommand(ctx, intake, request, CredentialApplicabilityChangePayload.Command)
}

func (intake *OperatorRegistryIntake) IntakeEffectiveTimeRuleRegistration(ctx context.Context, request *http.Request) (application.RegisterEffectiveTimeRuleCommand, error) {
	return registryCommand(ctx, intake, request, EffectiveTimeRuleRegistrationPayload.Command)
}

func (intake *OperatorRegistryIntake) IntakeMasterDocumentRegistration(ctx context.Context, request *http.Request) (application.RegisterMasterDocumentCommand, error) {
	return registryCommand(ctx, intake, request, MasterDocumentRegistrationPayload.Command)
}

func (intake *OperatorRegistryIntake) IntakeMasterDocumentRevision(ctx context.Context, request *http.Request) (application.ReviseMasterDocumentCommand, error) {
	return registryCommand(ctx, intake, request, MasterDocumentRevisionPayload.Command)
}

// registryCommand 是五口共用的一段：认证 → 封闭解码（decodeClosedPayload，与运营决定口同一份判据）→ 载荷连同认证出
// 的租户译成命令。
func registryCommand[Payload any, Command any](
	ctx context.Context,
	intake *OperatorRegistryIntake,
	request *http.Request,
	translate func(Payload, domain.TenantID) (Command, error),
) (Command, error) {
	var none Command
	tenant, err := intake.authenticator.AuthenticateRegistryWrite(ctx, httpapi.BearerToken(request))
	if err != nil {
		return none, err
	}
	var payload Payload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return none, err
	}
	return translate(payload, tenant)
}
