package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// ChargeSupersessionStore 记下后一代预估替代了前一代仍未确认的费用。确认口据此不再确认被替代的那一笔。
type ChargeSupersessionStore interface {
	SaveSupersession(
		ctx context.Context,
		tenant domain.TenantID,
		charge domain.CustomerChargeID,
		supersededBy domain.CustomerChargeID,
		at time.Time,
	) error
	Superseded(ctx context.Context, tenant domain.TenantID, charge domain.CustomerChargeID) (bool, error)
}

// ReweighAdjustmentAdoptionView 问这一结算账户有没有采用「实测差异自动调整」。没有就是没采用。
type ReweighAdjustmentAdoptionView interface {
	ReweighAdjustmentAdopted(
		ctx context.Context,
		tenant domain.TenantID,
		account domain.SettlementAccountID,
	) (bool, error)
}

// ReweighAdjustmentAdoptionRegister 写下这一户的采用。同一户再登是重放，不改已经写下的依据。
type ReweighAdjustmentAdoptionRegister interface {
	SaveReweighAdjustmentAdoption(
		ctx context.Context,
		tenant domain.TenantID,
		account domain.SettlementAccountID,
		basis string,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}
