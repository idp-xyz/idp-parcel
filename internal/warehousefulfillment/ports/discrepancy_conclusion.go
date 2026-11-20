package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// DiscrepancySubject 是一份差异在哪张预报、哪个货主名下。收货形成的差异与完结形成的少收都从这里认，货主取自预报。
type DiscrepancySubject struct {
	Discrepancy domain.ReceiptDiscrepancyID
	PreAdvice   domain.PreAdviceID
	Owner       domain.OwnerAccountID
}

// DiscrepancyConclusionRead 按差异标识读已经落下的结论，给预报查阅拼现行结论与被取代的代录结论。
// 读不动走 error，不折成没有：没有是这份差异还没有结论。
type DiscrepancyConclusionRead interface {
	ListByDiscrepancies(
		ctx context.Context,
		tenant domain.TenantID,
		ids []domain.ReceiptDiscrepancyID,
	) ([]domain.DiscrepancyConclusion, error)
}

// DiscrepancyConclusionStore 是结论册。Hold 与 Record 在没有环境事务时拒绝。
// Hold 拿的是差异标识上的事务级咨询锁，同一份差异上的货主与代录因此串行。
// HoldRequestKey 再把同一来源请求键串行起来，已落册的键才不会被两笔同时写成两份差异。
type DiscrepancyConclusionStore interface {
	DiscrepancyConclusionRead
	Hold(ctx context.Context, tenant domain.TenantID, discrepancy domain.ReceiptDiscrepancyID) error
	HoldRequestKey(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) error
	FindSubject(ctx context.Context, tenant domain.TenantID, discrepancy domain.ReceiptDiscrepancyID) (DiscrepancySubject, bool, error)
	FindRequestKey(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) (domain.ReceiptDiscrepancyID, bool, error)
	History(ctx context.Context, tenant domain.TenantID, discrepancy domain.ReceiptDiscrepancyID) ([]domain.DiscrepancyConclusion, error)
	// Record 追加一条结论。supersede 为真时先把这份差异上现行的那一条改成不再现行，被取代的行留下。
	Record(ctx context.Context, conclusion domain.DiscrepancyConclusion, supersede bool) error
}
