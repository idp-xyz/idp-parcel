package visibilityhttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 操作者渠道在本包登记写面的三格答复（ADR-0100 决定四），与 ErrAccessChannelNotConfigured 并列，各对一种恢复动作
// （ADR-0029）。登记写面不判准入范围（ADR-0100：登记一个版本不形成任何生产事实），所以没有「不在准入范围」那一格。
var (
	// ErrOperatorCredentialRejected：令牌缺失、过期或校验不过（401）——持令牌的人去登录或换令牌。
	ErrOperatorCredentialRejected = errors.New("visibility http: operator credential rejected")
	// ErrOperatorNotGranted：令牌有效，但不在册、不绑这个租户或没有登记册配置写的授予（403）——管授予的人去登记。
	ErrOperatorNotGranted = errors.New("visibility http: operator holds no registry configuration grant")
	// ErrIdentityDependencyUnavailable：发行方公钥集或操作者册读不动（503）——运维去救依赖。
	ErrIdentityDependencyUnavailable = errors.New("visibility http: identity dependency unavailable")
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

// OperatorRegistryIntake 是操作者渠道在本包八个目录登记口的 Intake（ADR-0100 决定四；票 operator-channel/04）。
//
// 译装只用 registrationjson 那一份：受控批量口与在线口共用同一套形状，在线那一路租户取认证结果，批文带 tenantId 即拒。
// 先认证、后读批文：令牌不过的调用方看不到批文校验的结果。
type OperatorRegistryIntake struct {
	authenticator OperatorRegistryAuthenticator
}

var (
	_ MilestoneMappingRegistrationIntake         = (*OperatorRegistryIntake)(nil)
	_ TriageRulesRegistrationIntake              = (*OperatorRegistryIntake)(nil)
	_ NotificationPolicyRegistrationIntake       = (*OperatorRegistryIntake)(nil)
	_ ClaimEligibilityRegistrationIntake         = (*OperatorRegistryIntake)(nil)
	_ ClaimAuthorizationRegistrationIntake       = (*OperatorRegistryIntake)(nil)
	_ DisclosurePolicyRegistrationIntake         = (*OperatorRegistryIntake)(nil)
	_ ExceptionDisclosureRulesRegistrationIntake = (*OperatorRegistryIntake)(nil)
	_ ConflictSignalRuleRegistrationIntake       = (*OperatorRegistryIntake)(nil)
)

func NewOperatorRegistryIntake(authenticator OperatorRegistryAuthenticator) (*OperatorRegistryIntake, error) {
	if authenticator == nil {
		return nil, errors.New("visibility http: operator registry intake needs an authenticator")
	}
	return &OperatorRegistryIntake{authenticator: authenticator}, nil
}

func (intake *OperatorRegistryIntake) IntakeMilestoneMappingRegistration(ctx context.Context, request *http.Request) (application.RegisterMilestoneMappingCommand, error) {
	return translateOnline(ctx, intake, request, registrationjson.MilestoneMappingFromJSONForTenant)
}

func (intake *OperatorRegistryIntake) IntakeTriageRulesRegistration(ctx context.Context, request *http.Request) (application.RegisterTriageRulesCommand, error) {
	return translateOnline(ctx, intake, request, registrationjson.TriageRulesFromJSONForTenant)
}

func (intake *OperatorRegistryIntake) IntakeNotificationPolicyRegistration(ctx context.Context, request *http.Request) (application.RegisterNotificationPolicyCommand, error) {
	return translateOnline(ctx, intake, request, registrationjson.NotificationPolicyFromJSONForTenant)
}

func (intake *OperatorRegistryIntake) IntakeClaimEligibilityRegistration(ctx context.Context, request *http.Request) (application.RegisterClaimEligibilityCommand, error) {
	return translateOnline(ctx, intake, request, registrationjson.ClaimEligibilityFromJSONForTenant)
}

func (intake *OperatorRegistryIntake) IntakeClaimAuthorizationRegistration(ctx context.Context, request *http.Request) (application.RegisterClaimAuthorizationCommand, error) {
	return translateOnline(ctx, intake, request, registrationjson.ClaimAuthorizationFromJSONForTenant)
}

func (intake *OperatorRegistryIntake) IntakeDisclosurePolicyRegistration(ctx context.Context, request *http.Request) (application.RegisterDisclosurePolicyCommand, error) {
	return translateOnline(ctx, intake, request, registrationjson.DisclosurePolicyFromJSONForTenant)
}

func (intake *OperatorRegistryIntake) IntakeExceptionDisclosureRulesRegistration(ctx context.Context, request *http.Request) (application.RegisterExceptionDisclosureRulesCommand, error) {
	return translateOnline(ctx, intake, request, registrationjson.ExceptionDisclosureRulesFromJSONForTenant)
}

func (intake *OperatorRegistryIntake) IntakeConflictSignalRuleRegistration(ctx context.Context, request *http.Request) (application.RegisterConflictSignalRuleCommand, error) {
	return translateOnline(ctx, intake, request, registrationjson.ConflictSignalRuleFromJSONForTenant)
}

// translateOnline 是八口共用的那一段：认证、读批文、交 registrationjson 在线那一路。批文读不出、超限或译不出都答坏报文。
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
		return none, fmt.Errorf("%w: empty body", ErrMalformedRegistration)
	}
	raw, err := io.ReadAll(io.LimitReader(request.Body, maxRegistrationBytes+1))
	if err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRegistration, err)
	}
	if len(raw) > maxRegistrationBytes {
		return none, fmt.Errorf("%w: body exceeds %d bytes", ErrMalformedRegistration, maxRegistrationBytes)
	}
	command, err := translate(raw, tenant)
	if err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRegistration, err)
	}
	return command, nil
}
