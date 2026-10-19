package tfhttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 集成客户端族在本包的两格答复（ADR-0149 决定三；ADR-0202 决定二）。与操作者渠道的两格分开设：令牌由另一族的发行方签、
// 恢复动作落在另一组人身上（外部系统的运维去换令牌，管集成客户端授予的人去登记），合成一格就分不出该找谁。未配置、
// 不在准入范围、依赖读不动三格与操作者渠道同义，沿用同一组哨兵。
var (
	// ErrIntegrationClientCredentialRejected：令牌缺失、过期、校验不过，或册上要求证书绑定而这次出示对不上（401）。
	ErrIntegrationClientCredentialRejected = errors.New("transport fulfillment http: integration client credential rejected")
	// ErrIntegrationClientNotGranted：令牌有效，但客户端不在册，或此刻没有承运订舱答复这一类事实的生效授予（403）。
	ErrIntegrationClientNotGranted = errors.New("transport fulfillment http: integration client holds no effective grant for this fact type")
)

const (
	codeIntegrationClientCredentialRejected = "INTEGRATION_CLIENT_CREDENTIAL_REJECTED"
	codeIntegrationClientNotGranted         = "INTEGRATION_CLIENT_NOT_GRANTED"
)

// PresentedClientCredential 是这一次 HTTP 出示：Bearer 令牌，与这次 TLS 连接上的客户端证书（DER；没有就为空）。
type PresentedClientCredential struct {
	Token       string
	Certificate []byte
}

// CarrierAnswerIdentity 是一次承运答复出示认证出的身份：租户与客户端代表的来源身份，都取自集成客户端册上的绑定。
type CarrierAnswerIdentity struct {
	Tenant domain.TenantID
	Source string
}

// IntegrationClientAuthenticator 认证一次承运答复出示，交回租户与来源身份。失败只交回本包的格：ErrAccessChannelNotConfigured、
// ErrIntegrationClientCredentialRejected、ErrIntegrationClientNotGranted、ErrOutsideAdmissionScope、
// ErrIdentityDependencyUnavailable；从共享接入身份能力译过来的那一层在 adapters/accessidentity。
type IntegrationClientAuthenticator interface {
	AuthenticateCarrierBookingAnswer(ctx context.Context, credential PresentedClientCredential) (CarrierAnswerIdentity, error)
}

// IntegrationClientIntake 是集成客户端族在本包各口的 Intake（ADR-0149 决定三）。今天只有承运订舱答复一口（ADR-0202
// 决定二）：提交方是承运系统，不是运营人员。先认证、后解载荷：令牌不过的调用方看不到载荷校验的结果。租户与来源身份
// 只取认证结果，载荷里写了即拒（封闭解码）。
type IntegrationClientIntake struct {
	authenticator IntegrationClientAuthenticator
}

var _ BookingAnswerIntake = (*IntegrationClientIntake)(nil)

func NewIntegrationClientIntake(authenticator IntegrationClientAuthenticator) (*IntegrationClientIntake, error) {
	if authenticator == nil {
		return nil, errors.New("transport fulfillment http: integration client intake needs an authenticator")
	}
	return &IntegrationClientIntake{authenticator: authenticator}, nil
}

// IntakeBookingAnswer 译承运订舱答复（`/transport-fulfillment/booking-answers`）。
func (intake *IntegrationClientIntake) IntakeBookingAnswer(ctx context.Context, request *http.Request) (application.AnswerBookingCommand, error) {
	identity, err := intake.authenticator.AuthenticateCarrierBookingAnswer(ctx, presentedClientCredential(request))
	if err != nil {
		return application.AnswerBookingCommand{}, err
	}
	var payload BookingAnswerPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.AnswerBookingCommand{}, err
	}
	return payload.Command(identity)
}

func presentedClientCredential(request *http.Request) PresentedClientCredential {
	var certificate []byte
	if request.TLS != nil && len(request.TLS.PeerCertificates) > 0 {
		certificate = request.TLS.PeerCertificates[0].Raw
	}
	return PresentedClientCredential{Token: httpapi.BearerToken(request), Certificate: certificate}
}
