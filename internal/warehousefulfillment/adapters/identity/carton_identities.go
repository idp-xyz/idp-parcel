package identity

import (
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 前缀只给日志和工单看。横线归拼接处，常量里不带。与商品库存流水、商品入库批次各用一个签发器。
const (
	cartonMovementPrefix     = "CMV"
	cartonInboundBatchPrefix = "CIB"
)

// CartonIdentities 实现 ports.CartonIdentities。
type CartonIdentities struct {
	movements platformidentity.Minter
	batches   platformidentity.Minter
}

func NewCartonIdentities(options ...platformidentity.Option) (*CartonIdentities, error) {
	movements, err := platformidentity.NewMinter(cartonMovementPrefix, options...)
	if err != nil {
		return nil, err
	}
	batches, err := platformidentity.NewMinter(cartonInboundBatchPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &CartonIdentities{movements: movements, batches: batches}, nil
}

var _ ports.CartonIdentities = (*CartonIdentities)(nil)

func (identities *CartonIdentities) NextCartonMovementID() (domain.CartonMovementID, error) {
	value, err := identities.movements.Next()
	if err != nil {
		return domain.CartonMovementID{}, err
	}
	return domain.NewCartonMovementID(value)
}

func (identities *CartonIdentities) NextCartonInboundBatchID() (domain.CartonInboundBatchID, error) {
	value, err := identities.batches.Next()
	if err != nil {
		return domain.CartonInboundBatchID{}, err
	}
	return domain.NewCartonInboundBatchID(value)
}
