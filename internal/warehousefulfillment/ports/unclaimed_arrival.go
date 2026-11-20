package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// UnclaimedArrivalRead 供入库预报读口问哪些预报有由收货事实转来的待认领到货。
// 不把到货的内容、标识或数量交出来，也不把作业事实直接登记时填的线索漏给货主。读不动走 error，不折成没有。
type UnclaimedArrivalRead interface {
	PreAdviceIDsWithUnclaimedArrival(
		ctx context.Context,
		tenant domain.TenantID,
		ids []domain.PreAdviceID,
	) ([]domain.PreAdviceID, error)
}

// UnclaimedArrivalCatalogueRead 是运营查阅待认领到货的读口：列表分页与单件全貌。客户面没有这一读口。
type UnclaimedArrivalCatalogueRead interface {
	ListUnclaimedArrivals(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
		receivedFrom, receivedTo time.Time,
	) (cataloguepage.Result[domain.UnclaimedArrivalDetail], error)
	LoadDetail(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.UnclaimedArrivalID,
	) (domain.UnclaimedArrivalDetail, bool, error)
}

// UnclaimedArrivalStore 是待认领到货的读写册。
// 锁序约束：收货确认引用到货时先拿预报锁再调 HoldArrival；认领、撤销、处置不拿预报锁，各自按
// HoldClaimDecision/HoldRevokeDecision → HoldArrival 的顺序加锁，避免成环。
// 所有加锁与写入方法在没有环境事务时拒绝。
type UnclaimedArrivalStore interface {
	UnclaimedArrivalRead
	UnclaimedArrivalCatalogueRead

	// Save 写入一件由收货事实转来的待认领到货，须与原收货事实在同一笔事务里。
	Save(ctx context.Context, arrival domain.UnclaimedArrival) (inserted bool, err error)

	// HoldFact、LoadByFact 与 SaveDirect 供一线作业直接登记待认领到货按事实身份串行、判重与落册。
	HoldFact(ctx context.Context, tenant domain.TenantID, factID domain.UnclaimedArrivalFactID) error
	LoadByFact(ctx context.Context, tenant domain.TenantID, factID domain.UnclaimedArrivalFactID) (domain.RecordedUnclaimedArrival, bool, error)
	SaveDirect(ctx context.Context, arrival domain.RecordedUnclaimedArrival) (inserted bool, err error)

	// HoldArrival 把同一件到货上的认领、撤销、处置与引用它的入账串行起来。
	HoldArrival(ctx context.Context, tenant domain.TenantID, id domain.UnclaimedArrivalID) error
	HoldClaimDecision(ctx context.Context, tenant domain.TenantID, decisionID domain.RequestKey) error
	HoldRevokeDecision(ctx context.Context, tenant domain.TenantID, decisionID domain.RequestKey) error

	LoadClaimByDecision(ctx context.Context, tenant domain.TenantID, decisionID domain.RequestKey) (domain.UnclaimedArrivalClaim, bool, error)
	LoadRevocationByDecision(ctx context.Context, tenant domain.TenantID, decisionID domain.RequestKey) (domain.UnclaimedArrivalClaimRevocation, bool, error)
	HasConfirmedReceiptReference(ctx context.Context, tenant domain.TenantID, id domain.UnclaimedArrivalID) (bool, error)

	RecordClaim(ctx context.Context, claim domain.UnclaimedArrivalClaim) error
	RecordRevocation(ctx context.Context, revocation domain.UnclaimedArrivalClaimRevocation) error
	RecordDisposition(ctx context.Context, disposition domain.UnclaimedArrivalDisposition) error
}

// UnclaimedArrivalIdentities 签发待认领到货的标识。只在确实要保全或登记一件时取。
type UnclaimedArrivalIdentities interface {
	NextUnclaimedArrivalID() (domain.UnclaimedArrivalID, error)
}
