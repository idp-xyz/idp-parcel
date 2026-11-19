package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// ChargeOccurrencesCarryingObject 按一件载运对象列出仍有效、载有它的收费发生项版本。
// 失效版本，以及已经被后一版纠正的版本，都不在这份清单里。
type ChargeOccurrencesCarryingObject interface {
	ListCurrentCarrying(
		ctx context.Context,
		tenant domain.TenantID,
		object domain.CarriedObjectReference,
	) ([]ChargeOccurrenceKey, error)
}
