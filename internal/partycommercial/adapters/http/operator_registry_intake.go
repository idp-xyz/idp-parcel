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

// OperatorIdentity 是一次登记册配置写出示认证出的身份：操作者绑定的租户，与本上下文消费信封的那一个面——操作者主体
// （主体引用 + 授予集，ADR-0126 决定五）。录入者取它的引用、批准者取它整个；其余各口只认租户。
type OperatorIdentity struct {
	Tenant   domain.TenantID
	Operator domain.OperatorSubject
}

// OperatorRegistryAuthenticator 认证一次登记册配置写出示，交回认证出的身份。失败只交回本包的格；从共享接入身份
// 能力译过来的那一层在 adapters/accessidentity（跨上下文翻译的位置，见 internal/architecture 的边界门禁）。
type OperatorRegistryAuthenticator interface {
	AuthenticateRegistryWrite(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

// OperatorRegistryIntake 是操作者渠道在本包登记写面各口的 Intake（ADR-0100 决定四；票 operator-channel/04）：服务形态、
// 产品—渠道映射、注册号类型目录、渠道账号使用授权，以及参与方身份族（四类登记、停用与法人资料登记）。运营操作者面发布
// 路径的预览、载体三口与词表读口也挂它（票 operator-channel/06），译法各在该口自己的文件里。
//
// 线格式照本包在线口的惯例：镜像受控 CLI 的批文，去掉整批的 tenantId——键在场即拒（理由在
// partyIdentityBatchDocument.TenantID），本口恰一项、别的口的项拒；租户格填认证结果。服务形态、映射与注册号类型的外壳与
// 逐项翻译就是受控 CLI 那一份（registrationjson），两口对同一项译出同一条命令；身份族的外壳与逐项翻译在
// party_identity_payload.go，与受控 CLI 的一项逐字同形；使用授权族没有受控 CLI，载荷只有在线这一份
// （channel_account_use_payload.go）。先认证、后解载荷。
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
	_ BusinessPartyRegistrationIntake          = (*OperatorRegistryIntake)(nil)
	_ LegalEntityRegistrationIntake            = (*OperatorRegistryIntake)(nil)
	_ CustomerAccountRegistrationIntake        = (*OperatorRegistryIntake)(nil)
	_ PartyRelationshipRegistrationIntake      = (*OperatorRegistryIntake)(nil)
	_ PartyIdentityDeactivationIntake          = (*OperatorRegistryIntake)(nil)
	_ LegalEntityProfileRegistrationIntake     = (*OperatorRegistryIntake)(nil)
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

// 身份族各口的逐项翻译自己就把形状错包成 ErrMalformedRequest 并带上字段路径（party_identity_payload.go），所以下面
// 不再过 malformedUnless——再包一层，答复的 detail 里会多出一遍哨兵原句。

func (intake *OperatorRegistryIntake) IntakeBusinessPartyRegistration(ctx context.Context, request *http.Request) (application.RegisterBusinessPartyCommand, error) {
	tenant, document, err := authenticateAndDecode[partyIdentityBatchDocument](ctx, intake, request, partyIdentityBatchTenant)
	if err != nil {
		return application.RegisterBusinessPartyCommand{}, err
	}
	if err := document.exactlyOne("businessParties", len(document.BusinessParties)); err != nil {
		return application.RegisterBusinessPartyCommand{}, err
	}
	return document.BusinessParties[0].command(tenant)
}

func (intake *OperatorRegistryIntake) IntakeLegalEntityRegistration(ctx context.Context, request *http.Request) (application.RegisterLegalEntityCommand, error) {
	tenant, document, err := authenticateAndDecode[partyIdentityBatchDocument](ctx, intake, request, partyIdentityBatchTenant)
	if err != nil {
		return application.RegisterLegalEntityCommand{}, err
	}
	if err := document.exactlyOne("legalEntities", len(document.LegalEntities)); err != nil {
		return application.RegisterLegalEntityCommand{}, err
	}
	return document.LegalEntities[0].command(tenant)
}

func (intake *OperatorRegistryIntake) IntakeCustomerAccountRegistration(ctx context.Context, request *http.Request) (application.RegisterCustomerAccountCommand, error) {
	tenant, document, err := authenticateAndDecode[partyIdentityBatchDocument](ctx, intake, request, partyIdentityBatchTenant)
	if err != nil {
		return application.RegisterCustomerAccountCommand{}, err
	}
	if err := document.exactlyOne("customerAccounts", len(document.CustomerAccounts)); err != nil {
		return application.RegisterCustomerAccountCommand{}, err
	}
	return document.CustomerAccounts[0].command(tenant)
}

func (intake *OperatorRegistryIntake) IntakePartyRelationshipRegistration(ctx context.Context, request *http.Request) (application.RegisterPartyRelationshipCommand, error) {
	tenant, document, err := authenticateAndDecode[partyIdentityBatchDocument](ctx, intake, request, partyIdentityBatchTenant)
	if err != nil {
		return application.RegisterPartyRelationshipCommand{}, err
	}
	if err := document.exactlyOne("relationships", len(document.Relationships)); err != nil {
		return application.RegisterPartyRelationshipCommand{}, err
	}
	return document.Relationships[0].command(tenant)
}

func (intake *OperatorRegistryIntake) IntakePartyIdentityDeactivation(ctx context.Context, request *http.Request) (application.DeactivatePartyIdentityCommand, error) {
	tenant, document, err := authenticateAndDecode[deactivationBatchDocument](ctx, intake, request, deactivationBatchTenant)
	if err != nil {
		return application.DeactivatePartyIdentityCommand{}, err
	}
	if err := exactlyOneItem("deactivations", len(document.Deactivations), len(document.Deactivations)); err != nil {
		return application.DeactivatePartyIdentityCommand{}, err
	}
	return document.Deactivations[0].command(tenant)
}

func (intake *OperatorRegistryIntake) IntakeLegalEntityProfileRegistration(ctx context.Context, request *http.Request) (application.RegisterLegalEntityProfileCommand, error) {
	tenant, document, err := authenticateAndDecode[legalEntityProfileBatchDocument](ctx, intake, request, legalEntityProfileBatchTenant)
	if err != nil {
		return application.RegisterLegalEntityProfileCommand{}, err
	}
	if err := exactlyOneItem("profiles", len(document.Profiles), len(document.Profiles)); err != nil {
		return application.RegisterLegalEntityProfileCommand{}, err
	}
	return document.Profiles[0].command(tenant)
}

// authenticateAndDecode 是各口共用的前半段：先认证，再以封闭形状解外壳（decodeClosedDocument），再执行自报租户那道拒
// （refuseSelfReportedTenant）——各口走同一份判据，同一形状在本包只有一种答复。
func authenticateAndDecode[Document any](
	ctx context.Context,
	intake *OperatorRegistryIntake,
	request *http.Request,
	selfReportedTenant func(Document) []byte,
) (domain.TenantID, Document, error) {
	var none Document
	operator, err := intake.authenticate(ctx, request)
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
	return operator.Tenant, document, nil
}

// authenticate 是各口的第一步：令牌不过的请求，请求体一个字节都不读。
func (intake *OperatorRegistryIntake) authenticate(ctx context.Context, request *http.Request) (OperatorIdentity, error) {
	return intake.authenticator.AuthenticateRegistryWrite(ctx, httpapi.BearerToken(request))
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

func partyIdentityBatchTenant(document partyIdentityBatchDocument) []byte { return document.TenantID }

func deactivationBatchTenant(document deactivationBatchDocument) []byte { return document.TenantID }

func legalEntityProfileBatchTenant(document legalEntityProfileBatchDocument) []byte {
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
