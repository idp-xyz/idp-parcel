package identity

import (
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

const chargePrefix = "CHO"

// ChargeIdentities 签发仓储作业收费发生项标识。
type ChargeIdentities struct {
	occurrences platformidentity.Minter
}

func NewChargeIdentities(options ...platformidentity.Option) (*ChargeIdentities, error) {
	occurrences, err := platformidentity.NewMinter(chargePrefix, options...)
	if err != nil {
		return nil, err
	}
	return &ChargeIdentities{occurrences: occurrences}, nil
}

var _ ports.ChargeIdentities = (*ChargeIdentities)(nil)

func (identities *ChargeIdentities) NextChargeOccurrenceID() (string, error) {
	return identities.occurrences.Next()
}
