package shipmenthttp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// CustomerCancellationAuthenticator 认证一次首方客户的取消出示，交回取消请求自身的来源身份（取消口的信封，来源请求键带
// `parcel-cancellation:` 前缀）。次序同撤回口：accessRequestID 在令牌、册、授予与准入范围都过了之后才被调用。
type CustomerCancellationAuthenticator interface {
	AuthenticateCancellation(
		ctx context.Context,
		credential PresentedClientCredential,
		accessRequestID func() (string, error),
	) (domain.SourceIdentity, error)
}

// cancellationFutureSkew 是客户声明的请求时刻允许领先服务端时钟的幅度：时钟略有偏差不拒，明显的未来时刻拒。
const cancellationFutureSkew = 5 * time.Minute

// CustomerCancellationIntake 是首方客户渠道在取消口的 Intake（ADR-0139 决定四；ADR-0310 取舍 A、C；ADR-0313；票 first-party-api/11）。
//
// 委托只在信封账户、信封来源名下找：原提交的来源身份由取消信封的租户、客户账户、客户端来源加载荷里的原接入请求标识拼出；命令
// 另带信封账户（CustomerAccount），编排据此再核一次目标属该账户，他账户的委托、别的客户端提交的委托与不存在的委托同答
// 统一不可见结果。取消者（Requester）是客户账户本人，不收客户自报。
//
// 授权不在这里判：取消授权规则是租户取值（取舍 C），由编排的授权缝按接受时采用的授权规则读，没登记就如实未决。
//
// 取消的幂等键是（租户、原接入请求标识、包裹），内容摘要含请求时刻，所以请求时刻由载荷声明（requestedAt），不取服务端时钟：
// 取服务端时钟则每次重放都是新内容，被判成冲突。契约写明重放须逐字节同载荷。
type CustomerCancellationIntake struct {
	authenticator CustomerCancellationAuthenticator
	clock         ports.Clock
}

var _ CancellationIntake = (*CustomerCancellationIntake)(nil)

func NewCustomerCancellationIntake(authenticator CustomerCancellationAuthenticator, clock ports.Clock) (*CustomerCancellationIntake, error) {
	if authenticator == nil || clock == nil {
		return nil, fmt.Errorf("parcel shipment http: customer cancellation intake: needs an authenticator and a clock")
	}
	return &CustomerCancellationIntake{authenticator: authenticator, clock: clock}, nil
}

// cancellationDraft 是首方取消口的载荷形状（PCN-1 的字段，以 docs/api 为准）。
type cancellationDraft struct {
	// AccessRequestID 是取消请求自己的接入请求标识，铸成 `parcel-cancellation:` 前缀的来源请求键。
	AccessRequestID string `json:"accessRequestId"`
	// OriginalAccessRequestID 是原提交的接入请求标识，指名要取消其包裹的委托。
	OriginalAccessRequestID string `json:"originalAccessRequestId"`
	// ShipmentRequestID 是提交回执的委托号，必填，与找到的委托核对。
	ShipmentRequestID string `json:"shipmentRequestId"`
	// ParcelID 是提交回执里的包裹标识。
	ParcelID string `json:"parcelId"`
	// Reason 是结构化原因（原因目录的代码，不是自由文本）。
	Reason string `json:"reason"`
	// RequestedAt 是客户系统声明的取消请求时刻（RFC 3339）。
	RequestedAt string `json:"requestedAt"`
}

// IntakeCancellation 把一次首方取消译成取消命令。先认证、后读载荷；租户、客户账户与取消者取自认证结果，载荷里自报的任何
// 账户 / 租户 / 来源 / 操作者字段都是未知字段，整份拒。
func (intake *CustomerCancellationIntake) IntakeCancellation(
	ctx context.Context,
	request *http.Request,
) (application.CancelParcelCommand, error) {
	none := application.CancelParcelCommand{}
	var draft cancellationDraft
	cancellationIdentity, err := intake.authenticator.AuthenticateCancellation(ctx, presentedClientCredential(request), func() (string, error) {
		if err := decodeJSONPayload(request.Body, &draft); err != nil {
			return "", err
		}
		return draft.AccessRequestID, nil
	})
	if err != nil {
		return none, err
	}

	originalText := strings.TrimSpace(draft.OriginalAccessRequestID)
	originalKey, err := domain.NewSourceRequestKey(originalText)
	if err != nil {
		return none, malformedField("originalAccessRequestId", err)
	}
	originalIdentity, err := domain.NewSourceIdentity(
		cancellationIdentity.TenantID(), cancellationIdentity.CustomerAccountID(), cancellationIdentity.Source(), originalKey)
	if err != nil {
		return none, err
	}
	shipmentRequestID, err := domain.NewShipmentRequestID(strings.TrimSpace(draft.ShipmentRequestID))
	if err != nil {
		return none, malformedField("shipmentRequestId", err)
	}
	parcel, err := domain.NewDeclaredParcelID(strings.TrimSpace(draft.ParcelID))
	if err != nil {
		return none, malformedField("parcelId", err)
	}
	reason, err := domain.NewCancellationReasonReference(strings.TrimSpace(draft.Reason))
	if err != nil {
		return none, malformedField("reason", err)
	}
	requestedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(draft.RequestedAt))
	if err != nil {
		return none, malformedField("requestedAt", err)
	}
	if requestedAt.After(intake.clock.Now().Add(cancellationFutureSkew)) {
		return none, malformedField("requestedAt", fmt.Errorf("is in the future"))
	}
	requester, err := domain.NewCustomerAccountCancellationRequester(cancellationIdentity.CustomerAccountID())
	if err != nil {
		return none, err
	}
	// PCN-1 在这里当载荷是否立得住的一道门。取消命令没有摘要的位置：判重由取消决定库按（租户、原接入请求标识、包裹）为键、
	// 以包裹、取消者、原因与请求时刻为内容摘要承担，PCN-1 与它同一批输入，但不入库。
	if _, err := domain.CanonicalizeCustomerParcelCancellation(domain.CustomerParcelCancellationSpec{
		OriginalAccessRequestID: originalText,
		ShipmentRequestID:       shipmentRequestID,
		Parcel:                  parcel,
		Reason:                  reason,
		RequestedAt:             requestedAt,
	}); err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}

	return application.CancelParcelCommand{
		Identity:          originalIdentity,
		ShipmentRequestID: shipmentRequestID,
		CustomerAccount:   cancellationIdentity.CustomerAccountID(),
		Parcel:            parcel,
		Requester:         requester,
		Reason:            reason,
		RequestedAt:       requestedAt.UTC(),
	}, nil
}
