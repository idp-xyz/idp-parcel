package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// DeliveryPreAdviceIdentityFactory 签发送站预报标识（ADR-0212 决定二「标识由 NO 签发」）。一个端口一个类型，不并进
// IntakeIdentityFactory：预报编排不该依赖它根本不签发的收寄结果版本。签不出时如实报错，编排据此不形成答案、不落册。
type DeliveryPreAdviceIdentityFactory interface {
	NextDeliveryPreAdviceID(ctx context.Context) (domain.DeliveryPreAdviceID, error)
}
