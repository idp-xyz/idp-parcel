package identity

import (
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

const freezePrefix = "FRZ"

// FreezeIdentities 实现 ports.FreezeIdentities。
type FreezeIdentities struct {
	freezes platformidentity.Minter
}

func NewFreezeIdentities(options ...platformidentity.Option) (*FreezeIdentities, error) {
	freezes, err := platformidentity.NewMinter(freezePrefix, options...)
	if err != nil {
		return nil, err
	}
	return &FreezeIdentities{freezes: freezes}, nil
}

var _ ports.FreezeIdentities = (*FreezeIdentities)(nil)

func (identities *FreezeIdentities) NextFreezeID() (domain.FreezeID, error) {
	value, err := identities.freezes.Next()
	if err != nil {
		return domain.FreezeID{}, err
	}
	return domain.NewFreezeID(value)
}
