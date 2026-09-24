package customshttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/customscompliance/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 操作者渠道在本包登记写面的三格答复（ADR-0100 决定四），与 ErrAccessChannelNotConfigured 并列，各对一种恢复动作
// （ADR-0029）。登记写面不判准入范围（ADR-0100：登记一个版本不形成任何生产事实），所以没有「不在准入范围」那一格。
var (
	// ErrOperatorCredentialRejected：令牌缺失、过期或校验不过（401）——持令牌的人去登录或换令牌。
	ErrOperatorCredentialRejected = errors.New("customs http: operator credential rejected")
	// ErrOperatorNotGranted：令牌有效，但不在册、不绑这个租户或没有登记册配置写的授予（403）——管授予的人去登记。
	ErrOperatorNotGranted = errors.New("customs http: operator holds no registry configuration grant")
	// ErrIdentityDependencyUnavailable：发行方公钥集或操作者册读不动（503）——运维去救依赖。
	ErrIdentityDependencyUnavailable = errors.New("customs http: identity dependency unavailable")
)

const (
	codeOperatorCredentialRejected    = "OPERATOR_CREDENTIAL_REJECTED"
	codeOperatorNotGranted            = "OPERATOR_NOT_GRANTED"
	codeIdentityDependencyUnavailable = "IDENTITY_DEPENDENCY_UNAVAILABLE"
)

// maxRegistrationBytes 给一份登记批文设读取上限：目录登记以条目计，一兆字节已宽出几个量级，设限只为不让一个坏掉的
// 调用方把整段请求读进内存。超限答坏报文。
const maxRegistrationBytes = 1 << 20

// OperatorRegistryAuthenticator 认证一次登记册配置写出示，交回出示者的租户。失败只交回本包的格；从共享接入身份能力
// 译过来的那一层在 adapters/accessidentity（跨上下文翻译的位置，见 internal/architecture 的边界门禁）。
type OperatorRegistryAuthenticator interface {
	AuthenticateRegistryWrite(ctx context.Context, bearerToken string) (domain.TenantID, error)
}

// OperatorRegistryIntake 是操作者渠道在本包七个登记口的 Intake（ADR-0100 决定四；票 operator-channel/04）。
//
// 译装只用 registrationjson 那一份：受控批量口与在线口共用同一套形状，在线那一路租户取认证结果，批文带 tenantId 即拒。
// 先认证、后读批文：令牌不过的调用方看不到批文校验的结果。
type OperatorRegistryIntake struct {
	authenticator OperatorRegistryAuthenticator
}

var (
	_ InterpretationRuleRegistrationIntake      = (*OperatorRegistryIntake)(nil)
	_ GateCatalogRegistrationIntake             = (*OperatorRegistryIntake)(nil)
	_ CandidatePortRegistrationIntake           = (*OperatorRegistryIntake)(nil)
	_ DeclarationPathRegistrationIntake         = (*OperatorRegistryIntake)(nil)
	_ CaseRequirementRegistrationIntake         = (*OperatorRegistryIntake)(nil)
	_ DutyCollaborationRegistrationIntake       = (*OperatorRegistryIntake)(nil)
	_ DutyPaymentVerificationRegistrationIntake = (*OperatorRegistryIntake)(nil)
)

func NewOperatorRegistryIntake(authenticator OperatorRegistryAuthenticator) (*OperatorRegistryIntake, error) {
	if authenticator == nil {
		return nil, errors.New("customs http: operator registry intake needs an authenticator")
	}
	return &OperatorRegistryIntake{authenticator: authenticator}, nil
}

func (intake *OperatorRegistryIntake) IntakeInterpretationRuleRegistration(ctx context.Context, request *http.Request) (application.RegisterInterpretationRuleCommand, error) {
	return translateOnline(ctx, intake, request, registrationjson.InterpretationRuleFromJSONForTenant)
}

func (intake *OperatorRegistryIntake) IntakeGateCatalogRegistration(ctx context.Context, request *http.Request) (application.RegisterGateCatalogCommand, error) {
	return translateOnline(ctx, intake, request, registrationjson.GateCatalogFromJSONForTenant)
}

func (intake *OperatorRegistryIntake) IntakeCandidatePortRegistration(ctx context.Context, request *http.Request) (application.RegisterCandidatePortCommand, error) {
	return translateOnline(ctx, intake, request, registrationjson.CandidatePortFromJSONForTenant)
}

func (intake *OperatorRegistryIntake) IntakeDeclarationPathRegistration(ctx context.Context, request *http.Request) (application.RegisterDeclarationPathCommand, error) {
	return translateOnline(ctx, intake, request, registrationjson.DeclarationPathFromJSONForTenant)
}

func (intake *OperatorRegistryIntake) IntakeCaseRequirementRegistration(ctx context.Context, request *http.Request) (application.RegisterCaseRequirementRuleCommand, error) {
	return translateOnline(ctx, intake, request, registrationjson.CaseRequirementFromJSONForTenant)
}

func (intake *OperatorRegistryIntake) IntakeDutyCollaborationRegistration(ctx context.Context, request *http.Request) (application.FormDutyCollaborationCommand, error) {
	return translateOnline(ctx, intake, request, registrationjson.DutyCollaborationFromJSONForTenant)
}

func (intake *OperatorRegistryIntake) IntakeDutyPaymentVerificationRegistration(ctx context.Context, request *http.Request) (application.VerifyDutyPaymentCommand, error) {
	return translateOnline(ctx, intake, request, registrationjson.DutyPaymentVerificationFromJSONForTenant)
}

// translateOnline 是七口共用的那一段：认证、读批文、交 registrationjson 在线那一路。批文读不出、超限或译不出都答坏报文。
func translateOnline[C any](
	ctx context.Context,
	intake *OperatorRegistryIntake,
	request *http.Request,
	translate func(raw []byte, tenant domain.TenantID) (C, error),
) (C, error) {
	var none C
	tenant, err := intake.authenticator.AuthenticateRegistryWrite(ctx, httpapi.BearerToken(request))
	if err != nil {
		return none, err
	}
	if request.Body == nil {
		return none, fmt.Errorf("%w: empty body", ErrMalformedRequest)
	}
	raw, err := io.ReadAll(io.LimitReader(request.Body, maxRegistrationBytes+1))
	if err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if len(raw) > maxRegistrationBytes {
		return none, fmt.Errorf("%w: body exceeds %d bytes", ErrMalformedRequest, maxRegistrationBytes)
	}
	command, err := translate(raw, tenant)
	if err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return command, nil
}
