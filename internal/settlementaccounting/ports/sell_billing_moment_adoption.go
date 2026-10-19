package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// SellBillingMomentAdoptionView 是 SELL 计费时点采用册的只读口：按（租户、结算账户）交回那一行；册上没有这一户答
// false——发起编排据此答未配置，不预选时点、不默认计价范围（ADR-0190 决定三）。与写口分名：发起编排只该问采用，
// 不该拿到 Save。
type SellBillingMomentAdoptionView interface {
	FindSellBillingMomentAdoption(
		ctx context.Context,
		tenant domain.TenantID,
		account domain.SettlementAccountID,
	) (domain.SellBillingMomentAdoption, bool, error)
}

// SellBillingMomentAdoptionRegister 是采用册的写口。指向的结算账户还不在账户册上时交回的错误 errors.Is
// domain.ErrCatalogueTargetMissing：本册不代为建账户。
type SellBillingMomentAdoptionRegister interface {
	SaveSellBillingMomentAdoption(
		ctx context.Context,
		tenant domain.TenantID,
		adoption domain.SellBillingMomentAdoption,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}
