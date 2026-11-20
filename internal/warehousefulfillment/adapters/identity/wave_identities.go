package identity

import (
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

const pickTaskPrefix = "PKT"

// PickTaskIdentities 签发拣货任务标识。
type PickTaskIdentities struct {
	tasks platformidentity.Minter
}

func NewPickTaskIdentities(options ...platformidentity.Option) (*PickTaskIdentities, error) {
	tasks, err := platformidentity.NewMinter(pickTaskPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &PickTaskIdentities{tasks: tasks}, nil
}

func (identities *PickTaskIdentities) NextPickTaskID() (domain.PickTaskID, error) {
	value, err := identities.tasks.Next()
	if err != nil {
		return domain.PickTaskID{}, err
	}
	return domain.NewPickTaskID(value)
}
