// Package identity 是仓配履约库存账两类标识的生产实现。
//
// 标识是机制半边：签发一个不重的不透明编号不需要租户参数。两类标识各用一个签发器，
// 不并成一个全能签发器——流水标识与入库批次标识由记数这一处一起要，但类型互不相通。
// 编码、熵源与前缀里不放横线，理由在 internal/platform/identity，这里不复述。
package identity

import (
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 前缀只给日志和工单看。横线归拼接处，常量里不带。取值照 parcel-shipment 与 visibility-exception
// 的短大写字母，并避开全仓已经用过的前缀。
const (
	stockMovementPrefix = "SMV"
	inboundBatchPrefix  = "IBT"
)

// StockIdentities 实现 ports.StockIdentities。
type StockIdentities struct {
	movements platformidentity.Minter
	batches   platformidentity.Minter
}

func NewStockIdentities(options ...platformidentity.Option) (*StockIdentities, error) {
	movements, err := platformidentity.NewMinter(stockMovementPrefix, options...)
	if err != nil {
		return nil, err
	}
	batches, err := platformidentity.NewMinter(inboundBatchPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &StockIdentities{movements: movements, batches: batches}, nil
}

var _ ports.StockIdentities = (*StockIdentities)(nil)

func (identities *StockIdentities) NextStockMovementID() (domain.StockMovementID, error) {
	value, err := identities.movements.Next()
	if err != nil {
		return domain.StockMovementID{}, err
	}
	return domain.NewStockMovementID(value)
}

func (identities *StockIdentities) NextInboundBatchID() (domain.InboundBatchID, error) {
	value, err := identities.batches.Next()
	if err != nil {
		return domain.InboundBatchID{}, err
	}
	return domain.NewInboundBatchID(value)
}
