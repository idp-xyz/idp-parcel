package identity

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
)

// deliveryPreAdviceIDPrefix 让一个预报标识在日志、工单与司机出示的单据上一眼认得出是预报。它不参与唯一性。
const deliveryPreAdviceIDPrefix = "PREADV"

// DeliveryPreAdviceIDs 实现 ports.DeliveryPreAdviceIdentityFactory。取随机而不取库序列，理由同 platform/identity 的头注：预报标识
// 答回客户、随送站出示，自增号会让一个租户的客户从自己拿到的号推出别的客户的预报量。
type DeliveryPreAdviceIDs struct {
	minter platformidentity.Minter
}

func NewDeliveryPreAdviceIDs(options ...platformidentity.Option) (*DeliveryPreAdviceIDs, error) {
	minter, err := platformidentity.NewMinter(deliveryPreAdviceIDPrefix, options...)
	if err != nil {
		return nil, fmt.Errorf("node operations identity: %w", err)
	}
	return &DeliveryPreAdviceIDs{minter: minter}, nil
}

var _ ports.DeliveryPreAdviceIdentityFactory = (*DeliveryPreAdviceIDs)(nil)

// NextDeliveryPreAdviceID 签发一个新的预报标识。不看 ctx，理由同 NextIntakeResultVersion。
func (factory *DeliveryPreAdviceIDs) NextDeliveryPreAdviceID(_ context.Context) (domain.DeliveryPreAdviceID, error) {
	minted, err := factory.minter.Next()
	if err != nil {
		return domain.DeliveryPreAdviceID{}, fmt.Errorf("next delivery pre-advice ID: %w", err)
	}
	return domain.NewDeliveryPreAdviceID(minted)
}
