package identity

import (
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

const countPrefix = "CNT"

// CountIdentities 实现 ports.CountIdentities。
type CountIdentities struct {
	counts platformidentity.Minter
}

func NewCountIdentities(options ...platformidentity.Option) (*CountIdentities, error) {
	counts, err := platformidentity.NewMinter(countPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &CountIdentities{counts: counts}, nil
}

var _ ports.CountIdentities = (*CountIdentities)(nil)

func (identities *CountIdentities) NextCountID() (domain.CountID, error) {
	value, err := identities.counts.Next()
	if err != nil {
		return domain.CountID{}, err
	}
	return domain.NewCountID(value)
}
