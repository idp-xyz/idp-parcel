package shipmenthttp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// CustomerWithdrawalAuthenticator 认证一次首方客户的撤回出示，交回撤回请求自身的来源身份（撤回口的信封，来源请求键带
// `withdrawal:` 前缀）。次序与 CustomerClientAuthenticator 相同：accessRequestID 在令牌、册与授予都过了之后才被调用。
type CustomerWithdrawalAuthenticator interface {
	AuthenticateWithdrawal(
		ctx context.Context,
		credential PresentedClientCredential,
		accessRequestID func() (string, error),
	) (domain.SourceIdentity, error)
}

// CustomerWithdrawalIntake 是首方客户渠道在撤回口的 Intake（ADR-0139 决定四；ADR-0310 取舍 A、B；票 first-party-api/07）。
//
// 授权就是账户绑定加授予（取舍 B，撤回无 PC 裁定）：委托只在信封账户、信封来源名下找——原提交的来源身份由撤回信封的租户、
// 客户账户、客户端来源加载荷里的原接入请求标识拼出，所以他账户的委托、别的客户端提交的委托与不存在的委托同样查不到，答
// `统一不可见结果`，同字节。撤回者（Requester）取渠道行上的客户端来源，不收客户自报。
//
// 摘要走 PWD-1（domain.CanonicalizeCustomerWithdrawal），把原接入请求标识、回执委托号与原因纳入：同一撤回接入请求标识
// 改指另一份原提交、另一个委托号或另一个原因是来源冲突，不是重放。
//
// 命令里的委托号与提交版本在本 Intake 里还拿不到（要读委托）：载荷若带 `shipmentRequestId` 就先放进命令，其余交
// NewCustomerWithdrawalHandler 在编排前补齐并核对。
type CustomerWithdrawalIntake struct {
	authenticator CustomerWithdrawalAuthenticator
	clock         ports.Clock
}

var _ WithdrawalIntake = (*CustomerWithdrawalIntake)(nil)

func NewCustomerWithdrawalIntake(authenticator CustomerWithdrawalAuthenticator, clock ports.Clock) (*CustomerWithdrawalIntake, error) {
	if authenticator == nil || clock == nil {
		return nil, fmt.Errorf("parcel shipment http: customer withdrawal intake: needs an authenticator and a clock")
	}
	return &CustomerWithdrawalIntake{authenticator: authenticator, clock: clock}, nil
}

// withdrawalDraft 是首方撤回口的载荷形状（PWD-1 的字段，ADR-0140 的契约生成接手正式发布前以 docs/api 为准）。
type withdrawalDraft struct {
	// AccessRequestID 是撤回请求自己的接入请求标识，铸成 `withdrawal:` 前缀的来源请求键。
	AccessRequestID string `json:"accessRequestId"`
	// OriginalAccessRequestID 是原提交的接入请求标识，指名要撤的委托。
	OriginalAccessRequestID string `json:"originalAccessRequestId"`
	// ShipmentRequestID 是提交回执的委托号，可不带；带了就与找到的委托核对。
	ShipmentRequestID string `json:"shipmentRequestId"`
	// Reason 是结构化原因（原因目录的代码，不是自由文本）。
	Reason string `json:"reason"`
}

// IntakeWithdrawal 把一次首方撤回译成撤回命令。先认证、后读载荷；租户、客户账户、来源与撤回者取自认证结果，载荷里自报的任何
// 账户 / 租户 / 操作者字段都是未知字段，整份拒。
func (intake *CustomerWithdrawalIntake) IntakeWithdrawal(
	ctx context.Context,
	request *http.Request,
) (application.WithdrawShipmentRequestCommand, error) {
	var draft withdrawalDraft
	withdrawalIdentity, err := intake.authenticator.AuthenticateWithdrawal(ctx, presentedClientCredential(request), func() (string, error) {
		if err := decodeJSONPayload(request.Body, &draft); err != nil {
			return "", err
		}
		return draft.AccessRequestID, nil
	})
	if err != nil {
		return application.WithdrawShipmentRequestCommand{}, err
	}

	originalKey, err := domain.NewSourceRequestKey(strings.TrimSpace(draft.OriginalAccessRequestID))
	if err != nil {
		return application.WithdrawShipmentRequestCommand{}, malformedField("originalAccessRequestId", err)
	}
	originalIdentity, err := domain.NewSourceIdentity(
		withdrawalIdentity.TenantID(), withdrawalIdentity.CustomerAccountID(), withdrawalIdentity.Source(), originalKey)
	if err != nil {
		return application.WithdrawShipmentRequestCommand{}, err
	}
	var shipmentRequestID domain.ShipmentRequestID
	if named := strings.TrimSpace(draft.ShipmentRequestID); named != "" {
		if shipmentRequestID, err = domain.NewShipmentRequestID(named); err != nil {
			return application.WithdrawShipmentRequestCommand{}, malformedField("shipmentRequestId", err)
		}
	}
	reason, err := domain.NewWithdrawalReasonReference(strings.TrimSpace(draft.Reason))
	if err != nil {
		return application.WithdrawShipmentRequestCommand{}, malformedField("reason", err)
	}
	requester, err := domain.NewWithdrawalRequesterReference(withdrawalIdentity.Source().String())
	if err != nil {
		return application.WithdrawShipmentRequestCommand{}, err
	}
	digest, err := domain.CanonicalizeCustomerWithdrawal(domain.CustomerWithdrawalSpec{
		OriginalAccessRequestID: draft.OriginalAccessRequestID,
		ShipmentRequestID:       draft.ShipmentRequestID,
		Reason:                  reason,
	})
	if err != nil {
		return application.WithdrawShipmentRequestCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}

	now := intake.clock.Now().UTC()
	return application.WithdrawShipmentRequestCommand{
		Identity:           originalIdentity,
		WithdrawalIdentity: withdrawalIdentity,
		PayloadDigest:      digest,
		OccurredAt:         now,
		ReceivedAt:         now,
		ShipmentRequestID:  shipmentRequestID,
		Requester:          requester,
		Reason:             reason,
	}, nil
}

// NewCustomerWithdrawalHandler 在编排前按撤回命令里的原提交身份找委托，补齐客户没带的委托号与提交版本（取委托此刻的当前
// 版本；释放放的是各版已成立的占用，不只指名那一版），并核对客户带的委托号：与找到的委托不一致按找不到答
// （ErrShipmentRequestNotVisible，与不存在同字节），免得续办引用被客户填成别的委托号。查不到同样直接答找不到——命令没有
// 委托号与版本，编排的未决路径无从签续办引用，也就不进编排；这一格本来就是调用方的错，没有要保全的业务到达。委托读不动
// 答错误（5xx，客户原样重放即可）。
func NewCustomerWithdrawalHandler(requests ports.ShipmentRequestRepository, inner WithdrawalHandler) WithdrawalHandler {
	return namedWithdrawalChecker{requests: requests, inner: inner}
}

type namedWithdrawalChecker struct {
	requests ports.ShipmentRequestRepository
	inner    WithdrawalHandler
}

func (checker namedWithdrawalChecker) Handle(
	ctx context.Context,
	command application.WithdrawShipmentRequestCommand,
) (application.WithdrawShipmentRequestResult, error) {
	request, found, err := checker.requests.FindBySourceIdentity(ctx, command.Identity)
	if err != nil {
		return application.WithdrawShipmentRequestResult{}, fmt.Errorf("withdraw shipment request: find named request: %w", err)
	}
	if !found || (command.ShipmentRequestID != (domain.ShipmentRequestID{}) && command.ShipmentRequestID != request.ShipmentRequestID()) {
		return application.WithdrawShipmentRequestResult{}, fmt.Errorf("withdraw shipment request: %w", application.ErrShipmentRequestNotVisible)
	}
	command.ShipmentRequestID = request.ShipmentRequestID()
	command.SubmissionVersion = request.CurrentSubmissionVersion().VersionID()
	return checker.inner.Handle(ctx, command)
}
