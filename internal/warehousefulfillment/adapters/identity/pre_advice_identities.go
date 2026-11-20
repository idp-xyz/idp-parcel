package identity

import (
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// preAdvicePrefix 只给日志和工单看。横线归拼接处，常量里不带。与库存流水、入库批次各用一个签发器，不并进同一个类型。
const preAdvicePrefix = "IPA"

// PreAdviceIdentities 实现 ports.PreAdviceIdentities。
type PreAdviceIdentities struct {
	preAdvices platformidentity.Minter
}

func NewPreAdviceIdentities(options ...platformidentity.Option) (*PreAdviceIdentities, error) {
	preAdvices, err := platformidentity.NewMinter(preAdvicePrefix, options...)
	if err != nil {
		return nil, err
	}
	return &PreAdviceIdentities{preAdvices: preAdvices}, nil
}

var _ ports.PreAdviceIdentities = (*PreAdviceIdentities)(nil)

func (identities *PreAdviceIdentities) NextPreAdviceID() (domain.PreAdviceID, error) {
	value, err := identities.preAdvices.Next()
	if err != nil {
		return domain.PreAdviceID{}, err
	}
	return domain.NewPreAdviceID(value)
}
