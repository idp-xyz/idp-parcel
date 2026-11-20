package identity

import (
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

const outboundPrefix = "OBR"

// OutboundIdentities 签发出库请求标识。
type OutboundIdentities struct {
	requests platformidentity.Minter
}

func NewOutboundIdentities(options ...platformidentity.Option) (*OutboundIdentities, error) {
	requests, err := platformidentity.NewMinter(outboundPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &OutboundIdentities{requests: requests}, nil
}

var _ ports.OutboundIdentities = (*OutboundIdentities)(nil)

func (identities *OutboundIdentities) NextOutboundRequestID() (domain.OutboundRequestID, error) {
	value, err := identities.requests.Next()
	if err != nil {
		return domain.OutboundRequestID{}, err
	}
	return domain.NewOutboundRequestID(value)
}
