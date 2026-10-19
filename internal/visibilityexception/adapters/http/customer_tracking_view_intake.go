package visibilityhttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 首方客户族在本包的两格答复（ADR-0139 决定四），与 ErrAccessChannelNotConfigured、ErrIdentityDependencyUnavailable 并列，各对一种
// 恢复动作（ADR-0029）。操作者族那两格不复用：持令牌的是货主客户的系统，错误码若仍叫操作者，管授予的人会去操作者册里找一个不存在
// 的人。查阅口不判准入范围（ADR-0139 决定八），所以没有「不在准入范围」那一格。依赖读不动沿用 ErrIdentityDependencyUnavailable：
// 恢复动作同是运维去救依赖，码也与各族相同。
var (
	// ErrCustomerClientCredentialRejected：令牌缺失、过期、校验不过，或册上要求证书绑定而这次出示对不上（401）——持凭据的客户系统
	// 去换令牌。
	ErrCustomerClientCredentialRejected = errors.New("visibility http: customer client credential rejected")
	// ErrCustomerClientNotGranted：令牌有效，但客户端不在客户渠道册，或此刻没有追踪视图查阅的生效授予（403）——租户运营去登记或
	// 恢复授予。几种情形同答这一格，不说哪一半不对。
	ErrCustomerClientNotGranted = errors.New("visibility http: customer client holds no effective grant for the tracking view query")
)

const (
	codeCustomerClientCredentialRejected = "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"
	codeCustomerClientNotGranted         = "CUSTOMER_CLIENT_NOT_GRANTED"
)

// trackingViewParcelParameter 是查询串里唯一认的一格：包裹定位。
const trackingViewParcelParameter = "parcel"

// PresentedCustomerCredential 是这一次 HTTP 出示：Bearer 令牌，与这次 TLS 连接上的客户端证书（DER；没有就为空）。证书只用来核对
// 证书绑定令牌（RFC 8705）。
type PresentedCustomerCredential struct {
	Token       string
	Certificate []byte
}

// CustomerViewer 是认证结果：租户与货主客户账户，都取自客户渠道册上的绑定。
type CustomerViewer struct {
	Tenant   domain.TenantID
	Customer domain.CustomerAccountReference
}

// CustomerTrackingViewAuthenticator 认证一次客户追踪视图查阅的首方出示。失败只交回本包的格；从共享接入身份能力译过来的那一层在
// adapters/accessidentity（跨上下文翻译的位置，见 internal/architecture 的边界门禁）。
type CustomerTrackingViewAuthenticator interface {
	AuthenticateTrackingViewQuery(ctx context.Context, credential PresentedCustomerCredential) (CustomerViewer, error)
}

// CustomerTrackingViewIntake 是首方客户渠道在 `/customer-tracking-view` 的 Intake（ADR-0139 决定四；票 demo-chain-gaps/96）。
//
// 查询键的租户与账户只取自认证结果，请求能给的只有包裹定位（UC-VE-008「标识只用于定位候选，不是访问凭证」）。先认证、后读查询串：
// 令牌不过、不在册或没有授予的调用方看不到查询串校验的结果。查询串封闭：除包裹定位之外带任何一格都答坏请求——自报的账户、租户
// 落在这一类，悄悄忽略会让客户系统以为它指名的账户起了作用。
type CustomerTrackingViewIntake struct {
	authenticator CustomerTrackingViewAuthenticator
}

var _ QueryIntake = (*CustomerTrackingViewIntake)(nil)

func NewCustomerTrackingViewIntake(authenticator CustomerTrackingViewAuthenticator) (*CustomerTrackingViewIntake, error) {
	if authenticator == nil {
		return nil, errors.New("visibility http: customer tracking view intake needs an authenticator")
	}
	return &CustomerTrackingViewIntake{authenticator: authenticator}, nil
}

func (intake *CustomerTrackingViewIntake) IntakeQuery(ctx context.Context, request *http.Request) (TrackingViewQuery, error) {
	viewer, err := intake.authenticator.AuthenticateTrackingViewQuery(ctx, presentedCustomerCredential(request))
	if err != nil {
		return TrackingViewQuery{}, err
	}
	// 不用 URL.Query()：它吞掉 ParseQuery 的错误，带分号或坏百分号转义的那一段会被悄悄丢掉，余下的照常作答——正是查询串封闭要拒的
	// 「悄悄忽略」。
	parameters, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return TrackingViewQuery{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	for name := range parameters {
		if name != trackingViewParcelParameter {
			return TrackingViewQuery{}, fmt.Errorf("%w: query parameter %q is not part of the tracking view query", ErrMalformedRequest, name)
		}
	}
	located := parameters[trackingViewParcelParameter]
	if len(located) != 1 {
		return TrackingViewQuery{}, fmt.Errorf("%w: exactly one %s is required", ErrMalformedRequest, trackingViewParcelParameter)
	}
	parcel, err := domain.NewTrackedParcelReference(located[0])
	if err != nil {
		return TrackingViewQuery{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return TrackingViewQuery{Tenant: viewer.Tenant, Customer: viewer.Customer, Parcel: parcel}, nil
}

func presentedCustomerCredential(request *http.Request) PresentedCustomerCredential {
	var certificate []byte
	if request.TLS != nil && len(request.TLS.PeerCertificates) > 0 {
		certificate = request.TLS.PeerCertificates[0].Raw
	}
	return PresentedCustomerCredential{Token: httpapi.BearerToken(request), Certificate: certificate}
}
