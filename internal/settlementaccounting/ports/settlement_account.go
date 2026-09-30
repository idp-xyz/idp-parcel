package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// SettlementAccountRegistrationEffect 是登记一笔结算账户的三种落点。重放是同一标识、同一内容再交一次；
// 冲突是同一标识不同内容，或同一组固定属性挂到另一个标识上。两种都不改已落的行。
type SettlementAccountRegistrationEffect uint8

const (
	SettlementAccountRegistered SettlementAccountRegistrationEffect = iota + 1
	SettlementAccountReplay
	SettlementAccountConflict
)

// SettlementAccountRegister 是结算账户登记册的写口与按固定属性的读口。
// 读口按键交回账户标识；册上没有这一组就答 false，不代拟账户。
type SettlementAccountRegister interface {
	Save(
		ctx context.Context,
		tenant domain.TenantID,
		account domain.SettlementAccount,
		at time.Time,
	) (SettlementAccountRegistrationEffect, error)
	Find(
		ctx context.Context,
		tenant domain.TenantID,
		key domain.SettlementAccountKey,
	) (domain.SettlementAccountID, bool, error)
}
