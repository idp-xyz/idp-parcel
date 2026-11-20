package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// UnclaimedArrivalRead 只回答哪些预报有待认领到货指着它。
// 不把到货的内容、标识或数量交出来：预报读口只标有无。读不动走 error，不折成没有。
type UnclaimedArrivalRead interface {
	PreAdviceIDsWithUnclaimedArrival(
		ctx context.Context,
		tenant domain.TenantID,
		ids []domain.PreAdviceID,
	) ([]domain.PreAdviceID, error)
}

// UnclaimedArrivalStore 是待认领到货的册。到货标识由本上下文签发，不并进收货事实的身份。
//
// Save 在没有环境事务时拒绝，并且必须和它所指向的收货事实在同一笔里：事实先落，到货再指向它。
type UnclaimedArrivalStore interface {
	UnclaimedArrivalRead
	// Save 写入一件到货。同一事实已经有一件时 inserted 为假，调用方回滚：锁内不该发生。
	Save(ctx context.Context, arrival domain.UnclaimedArrival) (inserted bool, err error)
}

// UnclaimedArrivalIdentities 签发待认领到货的标识。只在确实要保全一件时取。
type UnclaimedArrivalIdentities interface {
	NextUnclaimedArrivalID() (domain.UnclaimedArrivalID, error)
}
