package settlementhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 集成客户端族在外部资金事实两口的答复（ADR-0149 决定三、ADR-0151 决定四）。更正与首登同一份认证与授予，不另立事实类型。
var (
	// ErrIntegrationClientCredentialRejected：令牌缺失、过期、校验不过，或册上要求证书绑定而这次出示对不上（401）。
	ErrIntegrationClientCredentialRejected = errors.New("settlement http: integration client credential rejected")
	// ErrIntegrationClientNotGranted：令牌有效，但客户端不在册，或此刻没有外部资金事实的生效授予（403）。
	ErrIntegrationClientNotGranted = errors.New("settlement http: integration client holds no effective grant for this fact type")
	// ErrOutsideAdmissionScope：这笔生产写不落在治理登记的生产权威区间内（403）。
	ErrOutsideAdmissionScope = errors.New("settlement http: production write is outside the admission scope")
)

const (
	codeIntegrationClientCredentialRejected = "INTEGRATION_CLIENT_CREDENTIAL_REJECTED"
	codeIntegrationClientNotGranted         = "INTEGRATION_CLIENT_NOT_GRANTED"
	codeOutsideAdmissionScope               = "OUTSIDE_ADMISSION_SCOPE"
	codeIdentityDependencyUnavailable       = "IDENTITY_DEPENDENCY_UNAVAILABLE"
)

// ErrIdentityDependencyUnavailable：发行方公钥集、集成客户端册或准入范围读不动（503）。
var ErrIdentityDependencyUnavailable = errors.New("settlement http: identity dependency unavailable")

// FactExternalFunds 是采用与更正两口要核的事实类型，字面与集成客户端册的授予单位相同。更正不另立一类。
const FactExternalFunds = "EXTERNAL_FUNDS_FACT"

// PresentedClientCredential 是这一次 HTTP 出示：Bearer 令牌，与这次 TLS 连接上的客户端证书（DER；没有就为空）。
type PresentedClientCredential struct {
	Token       string
	Certificate []byte
}

// IntegrationClientAuthenticator 认证一次集成客户端出示，交回册上绑定的租户。失败只交回本包的格。
type IntegrationClientAuthenticator interface {
	AuthenticateExternalFunds(ctx context.Context, credential PresentedClientCredential) (domain.TenantID, error)
}

// IntegrationClientIntake 是集成客户端族在外部资金事实采用与更正两口的 Intake（ADR-0149 决定三、ADR-0151 决定四；
// 票 operator-channel/11）。两口同一份认证与授予。译装仍走 registrationjson，认证出的租户经它的在线译装注入命令；载荷里
// 自报租户即拒，租户键不论大小写。
type IntegrationClientIntake struct {
	authenticator IntegrationClientAuthenticator
}

var (
	_ ExternalFundsFactRegistrationIntake           = (*IntegrationClientIntake)(nil)
	_ ExternalFundsFactCorrectionRegistrationIntake = (*IntegrationClientIntake)(nil)
)

func NewIntegrationClientIntake(authenticator IntegrationClientAuthenticator) (*IntegrationClientIntake, error) {
	if authenticator == nil {
		return nil, errors.New("settlement http: integration client intake needs an authenticator")
	}
	return &IntegrationClientIntake{authenticator: authenticator}, nil
}

func (intake *IntegrationClientIntake) IntakeExternalFundsFactRegistration(
	ctx context.Context,
	request *http.Request,
) (application.AdoptFundsFactCommand, error) {
	tenant, err := intake.authenticator.AuthenticateExternalFunds(ctx, presentedClientCredential(request))
	if err != nil {
		return application.AdoptFundsFactCommand{}, err
	}
	payload, err := readSinglePayload(request.Body)
	if err != nil {
		return application.AdoptFundsFactCommand{}, err
	}
	command, err := registrationjson.ExternalFundsFactFromJSONForTenant(payload, tenant)
	if err != nil {
		return application.AdoptFundsFactCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return command, nil
}

func (intake *IntegrationClientIntake) IntakeExternalFundsFactCorrectionRegistration(
	ctx context.Context,
	request *http.Request,
) (application.CorrectFundsFactCommand, error) {
	tenant, err := intake.authenticator.AuthenticateExternalFunds(ctx, presentedClientCredential(request))
	if err != nil {
		return application.CorrectFundsFactCommand{}, err
	}
	payload, err := readSinglePayload(request.Body)
	if err != nil {
		return application.CorrectFundsFactCommand{}, err
	}
	command, err := registrationjson.ExternalFundsFactCorrectionFromJSONForTenant(payload, tenant)
	if err != nil {
		return application.CorrectFundsFactCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return command, nil
}

func presentedClientCredential(request *http.Request) PresentedClientCredential {
	var certificate []byte
	if request.TLS != nil && len(request.TLS.PeerCertificates) > 0 {
		certificate = request.TLS.PeerCertificates[0].Raw
	}
	return PresentedClientCredential{Token: httpapi.BearerToken(request), Certificate: certificate}
}

// readSinglePayload 收一份载荷原文：只许一个 JSON 对象、其后无尾随内容。认证租户不往原文里拼——拼回去再解，
// 载荷里 `tenantid` 一类变体会按 encoding/json 的大小写折叠盖掉拼进去的那一个（票 operator-channel/52）。
func readSinglePayload(body io.Reader) ([]byte, error) {
	decoder := json.NewDecoder(body)
	var payload json.RawMessage
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing content after the payload", ErrMalformedRequest)
	}
	if !bytes.HasPrefix(bytes.TrimSpace(payload), []byte("{")) {
		return nil, fmt.Errorf("%w: the payload must be a JSON object", ErrMalformedRequest)
	}
	return payload, nil
}
