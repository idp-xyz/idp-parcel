package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// DirectionAuthorizationView 按身份读一版授权规则，回答它在某一时刻能不能为价卡提供方向。
//
// 只读。不在册是业务答复，不是错误；读不回（存储故障、快照坏了）才是错误。方向取自它指名的
// 商业价格政策里、同一时刻仍在效力内的那些版本，不在这里另造一份方向。
type DirectionAuthorizationView interface {
	ReadDirectionAuthorization(
		ctx context.Context,
		tenant domain.TenantID,
		objectID domain.CommercialObjectID,
		version domain.CommercialVersionLabel,
		at time.Time,
	) (domain.DirectionAuthorizationFact, error)
}
