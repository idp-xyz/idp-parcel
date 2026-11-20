package identity

import (
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// receiptDiscrepancyPrefix 只给日志和工单看。横线归拼接处，常量里不带。
// 与库存流水、入库批次、预报各用一个签发器，不并进同一个类型。
const receiptDiscrepancyPrefix = "RDP"

// ReceiptDiscrepancyIdentities 实现 ports.ReceiptDiscrepancyIdentities。
type ReceiptDiscrepancyIdentities struct {
	discrepancies platformidentity.Minter
}

func NewReceiptDiscrepancyIdentities(options ...platformidentity.Option) (*ReceiptDiscrepancyIdentities, error) {
	discrepancies, err := platformidentity.NewMinter(receiptDiscrepancyPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &ReceiptDiscrepancyIdentities{discrepancies: discrepancies}, nil
}

var _ ports.ReceiptDiscrepancyIdentities = (*ReceiptDiscrepancyIdentities)(nil)

func (identities *ReceiptDiscrepancyIdentities) NextReceiptDiscrepancyID() (domain.ReceiptDiscrepancyID, error) {
	value, err := identities.discrepancies.Next()
	if err != nil {
		return domain.ReceiptDiscrepancyID{}, err
	}
	return domain.NewReceiptDiscrepancyID(value)
}
