package identity

import (
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 前缀只给日志和工单看。与收货差异 RDP 分开：库内作业形成的是库存差异，不是收货差异。
const stockDiscrepancyPrefix = "SDP"

// StockDiscrepancyIdentities 实现 ports.StockDiscrepancyIdentities。
type StockDiscrepancyIdentities struct {
	discrepancies platformidentity.Minter
}

func NewStockDiscrepancyIdentities(options ...platformidentity.Option) (*StockDiscrepancyIdentities, error) {
	discrepancies, err := platformidentity.NewMinter(stockDiscrepancyPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &StockDiscrepancyIdentities{discrepancies: discrepancies}, nil
}

var _ ports.StockDiscrepancyIdentities = (*StockDiscrepancyIdentities)(nil)

func (identities *StockDiscrepancyIdentities) NextStockDiscrepancyID() (domain.StockDiscrepancyID, error) {
	value, err := identities.discrepancies.Next()
	if err != nil {
		return domain.StockDiscrepancyID{}, err
	}
	return domain.NewStockDiscrepancyID(value)
}
