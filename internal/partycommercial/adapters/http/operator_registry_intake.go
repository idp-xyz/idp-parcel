package commercialhttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 操作者渠道在本包登记写面的三格答复（ADR-0100 决定四），与 ErrAccessChannelNotConfigured 并列，各对一种恢复动作
// （ADR-0029）。登记写面不判准入范围，所以没有「不在准入范围」那一格。
var (
	// ErrOperatorCredentialRejected：令牌缺失、过期或校验不过（401）——持令牌的人去登录或换令牌。
	ErrOperatorCredentialRejected = errors.New("party commercial http: operator credential rejected")
	// ErrOperatorNotGranted：令牌有效，但不在册、不绑这个租户或没有登记册配置写的授予（403）——管授予的人去登记。
	ErrOperatorNotGranted = errors.New("party commercial http: operator holds no registry configuration grant")
	// ErrIdentityDependencyUnavailable：发行方公钥集或操作者册读不动（503）——运维去救依赖。
	ErrIdentityDependencyUnavailable = errors.New("party commercial http: identity dependency unavailable")
)

const (
	codeOperatorCredentialRejected    = "OPERATOR_CREDENTIAL_REJECTED"
	codeOperatorNotGranted            = "OPERATOR_NOT_GRANTED"
	codeIdentityDependencyUnavailable = "IDENTITY_DEPENDENCY_UNAVAILABLE"
)

// OperatorRegistryAuthenticator 认证一次登记册配置写出示，交回操作者绑定的租户。失败只交回本包的格；从共享接入身份
// 能力译过来的那一层在 adapters/accessidentity（跨上下文翻译的位置，见 internal/architecture 的边界门禁）。
type OperatorRegistryAuthenticator interface {
	AuthenticateRegistryWrite(ctx context.Context, bearerToken string) (domain.TenantID, error)
}

// OperatorRegistryIntake 是操作者渠道在本包服务形态、产品—渠道映射、注册号类型目录与渠道账号使用授权六口的 Intake
// （ADR-0100 决定四；票 operator-channel/04）。
//
// 线格式照本包在线口的既有惯例（IsolatedPartyIdentityIntake 注释）：镜像受控 CLI 的批文，去掉整批的 tenantId——键在场
// 即拒，本口恰一项、别的口的项拒；租户格填认证结果。前四口的外壳与逐项翻译就是受控 CLI 那一份（registrationjson），
// 两口对同一项译出同一条命令；使用授权族没有受控 CLI，载荷只有在线这一份（channel_account_use_payload.go）。先认证、
// 后解载荷。
type OperatorRegistryIntake struct {
	authenticator OperatorRegistryAuthenticator
}

var (
	_ ServiceProductFormRegistrationIntake     = (*OperatorRegistryIntake)(nil)
	_ ProductChannelMappingRegistrationIntake  = (*OperatorRegistryIntake)(nil)
	_ RegistrationNumberTypeRegistrationIntake = (*OperatorRegistryIntake)(nil)
	_ RegistrationNumberTypeDeactivationIntake = (*OperatorRegistryIntake)(nil)
	_ ChannelAccountUseRegistrationIntake      = (*OperatorRegistryIntake)(nil)
	_ ChannelAccountUseRevocationIntake        = (*OperatorRegistryIntake)(nil)
)

func NewOperatorRegistryIntake(authenticator OperatorRegistryAuthenticator) (*OperatorRegistryIntake, error) {
	if authenticator == nil {
		return nil, errors.New("party commercial http: operator registry intake needs an authenticator")
	}
	return &OperatorRegistryIntake{authenticator: authenticator}, nil
}

func (intake *OperatorRegistryIntake) IntakeServiceProductFormRegistration(ctx context.Context, request *http.Request) (application.RegisterServiceProductFormCommand, error) {
	var none application.RegisterServiceProductFormCommand
	tenant, document, err := authenticateAndDecode[registrationjson.ProductBatchDocument](ctx, intake, request, productBatchTenant)
	if err != nil {
		return none, err
	}
	if err := exactlyOneItem("forms", len(document.Forms), len(document.Forms)+len(document.Mappings)); err != nil {
		return none, err
	}
	scope, err := domain.NewCommercialScopeReference(document.Scope)
	if err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return malformedUnless(registrationjson.ServiceProductFormCommand(tenant, scope, document.Forms[0]))
}

func (intake *OperatorRegistryIntake) IntakeProductChannelMappingRegistration(ctx context.Context, request *http.Request) (application.RegisterProductChannelMappingCommand, error) {
	var none application.RegisterProductChannelMappingCommand
	tenant, document, err := authenticateAndDecode[registrationjson.ProductBatchDocument](ctx, intake, request, productBatchTenant)
	if err != nil {
		return none, err
	}
	if err := exactlyOneItem("mappings", len(document.Mappings), len(document.Forms)+len(document.Mappings)); err != nil {
		return none, err
	}
	scope, err := domain.NewCommercialScopeReference(document.Scope)
	if err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return malformedUnless(registrationjson.ProductChannelMappingCommand(tenant, scope, document.Mappings[0]))
}

func (intake *OperatorRegistryIntake) IntakeRegistrationNumberTypeRegistration(ctx context.Context, request *http.Request) (application.RegisterRegistrationNumberTypeCommand, error) {
	var none application.RegisterRegistrationNumberTypeCommand
	tenant, document, err := authenticateAndDecode[registrationjson.RegistrationNumberTypeBatchDocument](ctx, intake, request, numberTypeBatchTenant)
	if err != nil {
		return none, err
	}
	if err := exactlyOneItem("types", len(document.Types), len(document.Types)+len(document.Deactivations)); err != nil {
		return none, err
	}
	command, _, err := registrationjson.RegistrationNumberTypeCommand(tenant, document.Types[0])
	return malformedUnless(command, err)
}

func (intake *OperatorRegistryIntake) IntakeRegistrationNumberTypeDeactivation(ctx context.Context, request *http.Request) (application.DeactivateRegistrationNumberTypeCommand, error) {
	var none application.DeactivateRegistrationNumberTypeCommand
	tenant, document, err := authenticateAndDecode[registrationjson.RegistrationNumberTypeBatchDocument](ctx, intake, request, numberTypeBatchTenant)
	if err != nil {
		return none, err
	}
	if err := exactlyOneItem("deactivations", len(document.Deactivations), len(document.Types)+len(document.Deactivations)); err != nil {
		return none, err
	}
	return malformedUnless(registrationjson.RegistrationNumberTypeDeactivationCommand(tenant, document.Deactivations[0]))
}

func (intake *OperatorRegistryIntake) IntakeChannelAccountUseRegistration(ctx context.Context, request *http.Request) (application.RegisterChannelAccountUseCommand, error) {
	tenant, document, err := authenticateAndDecode[channelAccountUseDocument](ctx, intake, request, channelAccountUseTenant)
	if err != nil {
		return application.RegisterChannelAccountUseCommand{}, err
	}
	return malformedUnless(document.command(tenant))
}

func (intake *OperatorRegistryIntake) IntakeChannelAccountUseRevocation(ctx context.Context, request *http.Request) (application.RevokeChannelAccountUseCommand, error) {
	tenant, document, err := authenticateAndDecode[channelAccountUseRevocationDocument](ctx, intake, request, channelAccountUseRevocationTenant)
	if err != nil {
		return application.RevokeChannelAccountUseCommand{}, err
	}
	return malformedUnless(document.command(tenant))
}

// authenticateAndDecode 是六口共用的前半段：先认证，再以封闭形状解外壳（decodeClosedDocument），再执行自报租户那道拒
// （refuseSelfReportedTenant）——两道判据与本包隔离形态同一份，同一形状在本包只有一种答复。
func authenticateAndDecode[Document any](
	ctx context.Context,
	intake *OperatorRegistryIntake,
	request *http.Request,
	selfReportedTenant func(Document) []byte,
) (domain.TenantID, Document, error) {
	var none Document
	tenant, err := intake.authenticator.AuthenticateRegistryWrite(ctx, httpapi.BearerToken(request))
	if err != nil {
		return domain.TenantID{}, none, err
	}
	document, err := decodeClosedDocument[Document](request)
	if err != nil {
		return domain.TenantID{}, none, err
	}
	if err := refuseSelfReportedTenant(selfReportedTenant(document)); err != nil {
		return domain.TenantID{}, none, err
	}
	return tenant, document, nil
}

func productBatchTenant(document registrationjson.ProductBatchDocument) []byte {
	return document.TenantID
}

func numberTypeBatchTenant(document registrationjson.RegistrationNumberTypeBatchDocument) []byte {
	return document.TenantID
}

func channelAccountUseTenant(document channelAccountUseDocument) []byte { return document.TenantID }

func channelAccountUseRevocationTenant(document channelAccountUseRevocationDocument) []byte {
	return document.TenantID
}

// malformedUnless 把逐项翻译的拒绝归到「请求畸形」：翻译只过形状与领域构造门，过不去就是改载荷才会好。
func malformedUnless[Command any](command Command, err error) (Command, error) {
	if err != nil {
		var none Command
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return command, nil
}
