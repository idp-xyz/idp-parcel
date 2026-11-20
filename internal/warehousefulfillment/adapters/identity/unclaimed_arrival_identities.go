package identity

import (
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// unclaimedArrivalPrefix 只给日志和工单看。横线归拼接处，常量里不带。
// 与收货差异、库存流水各用一个签发器，不并进同一个类型。
const unclaimedArrivalPrefix = "UCA"

// UnclaimedArrivalIdentities 实现 ports.UnclaimedArrivalIdentities。
type UnclaimedArrivalIdentities struct {
	arrivals platformidentity.Minter
}

func NewUnclaimedArrivalIdentities(options ...platformidentity.Option) (*UnclaimedArrivalIdentities, error) {
	arrivals, err := platformidentity.NewMinter(unclaimedArrivalPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &UnclaimedArrivalIdentities{arrivals: arrivals}, nil
}

var _ ports.UnclaimedArrivalIdentities = (*UnclaimedArrivalIdentities)(nil)

func (identities *UnclaimedArrivalIdentities) NextUnclaimedArrivalID() (domain.UnclaimedArrivalID, error) {
	value, err := identities.arrivals.Next()
	if err != nil {
		return domain.UnclaimedArrivalID{}, err
	}
	return domain.NewUnclaimedArrivalID(value)
}
