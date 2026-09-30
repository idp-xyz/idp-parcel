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

// SettlementAccountView 是登记册的只读口。按固定属性交回账户标识；册上没有这一组就答 false，不代拟账户。
// 与写口分名：接受前控制的目录只该问账户，不该拿到 Save。
type SettlementAccountView interface {
	Find(
		ctx context.Context,
		tenant domain.TenantID,
		key domain.SettlementAccountKey,
	) (domain.SettlementAccountID, bool, error)
}

// SettlementAccountRegister 是结算账户登记册的写口。
type SettlementAccountRegister interface {
	Save(
		ctx context.Context,
		tenant domain.TenantID,
		account domain.SettlementAccount,
		at time.Time,
	) (SettlementAccountRegistrationEffect, error)
}
