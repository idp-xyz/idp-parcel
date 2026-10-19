package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// AuthorizedDeliveryQuery 是运营决定确认送站交接单时问 party-commercial 的那一问：（租户，交付方参与方，客户账户，开立节点，
// 开立时刻），见 CONTEXT-MAP `party-commercial → node-operations`。
//
// DeliveringParty 是决定载荷指名的 PC 参与方标识，不是设备上报的交付方线索：线索只照记，不参与判断（ADR-0212 决定一）。
// CustomerAccount 是 PC 的客户账户标识，本上下文只转交、不解读，所以照原串收。
type AuthorizedDeliveryQuery struct {
	Tenant          domain.TenantID
	DeliveringParty domain.DeliveringPartyReference
	CustomerAccount string
	Node            domain.NodeReference
	OpenedAt        time.Time
}

// AuthorizedDeliveryAnswer 是授权交付判断落到本上下文的三格：是即可确认；否答不在授权交付关系内；未配置如实答未配置，
// 不默认放行（UC-NO-006 结果语义契约）。读不动不是第四格，走 error、不形成答案。
type AuthorizedDeliveryAnswer uint8

const (
	AuthorizedDeliveryAnswerInvalid AuthorizedDeliveryAnswer = iota
	DeliveryAuthorized
	DeliveryNotAuthorized
	DeliveryAuthorizationNotConfigured
)

func (answer AuthorizedDeliveryAnswer) String() string {
	switch answer {
	case DeliveryAuthorized:
		return "AUTHORIZED"
	case DeliveryNotAuthorized:
		return "NOT_AUTHORIZED"
	case DeliveryAuthorizationNotConfigured:
		return "NOT_CONFIGURED"
	default:
		return ""
	}
}

// AuthorizedDeliveryView 问 party-commercial 的授权交付判断（PC 侧只读引用，同 ParcelIdentityView）。本上下文只问、不登记
// 也不修改授权交付关系（NO CONTEXT「Boundaries and ownership」）。
type AuthorizedDeliveryView interface {
	JudgeAuthorizedDelivery(ctx context.Context, query AuthorizedDeliveryQuery) (AuthorizedDeliveryAnswer, error)
}
