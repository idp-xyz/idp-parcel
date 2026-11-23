package identity

import (
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

const returnAdvicePrefix = "RPA"

// ReturnIdentities 签发退货预报标识。
type ReturnIdentities struct {
	advices platformidentity.Minter
}

func NewReturnIdentities(options ...platformidentity.Option) (*ReturnIdentities, error) {
	advices, err := platformidentity.NewMinter(returnAdvicePrefix, options...)
	if err != nil {
		return nil, err
	}
	return &ReturnIdentities{advices: advices}, nil
}

var _ ports.ReturnIdentities = (*ReturnIdentities)(nil)

func (identities *ReturnIdentities) NextReturnAdviceID() (domain.ReturnAdviceID, error) {
	value, err := identities.advices.Next()
	if err != nil {
		return domain.ReturnAdviceID{}, err
	}
	return domain.NewReturnAdviceID(value)
}
