package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// ReceiptRead 按预报读已经落册的收货事实，给预报查阅拼累计实收和逐次收货。
// 读不动走 error，不折成空：空是这份预报还没有收货事实。
type ReceiptRead interface {
	ListByPreAdvices(
		ctx context.Context,
		tenant domain.TenantID,
		ids []domain.PreAdviceID,
	) ([]domain.RecordedReceipt, error)
}

// ReceiptStore 是收货事实的册。事实身份在租户内唯一，由设备签发，本册不另签。
//
// HoldFact 与 Save 在没有环境事务时拒绝。List 与 Load 用环境里的读执行器。
// 落册用 ON CONFLICT DO NOTHING：唯一冲突会把当前事务打成不可用，而入账和预报状态还在同一笔里。
type ReceiptStore interface {
	ReceiptRead
	// HoldFact 把（租户，事实身份）上的收货串行起来，重放才不会和另一笔同时落册。
	HoldFact(ctx context.Context, tenant domain.TenantID, factID domain.ReceiptFactID) error
	Load(ctx context.Context, tenant domain.TenantID, factID domain.ReceiptFactID) (domain.RecordedReceipt, bool, error)
	// Save 写入事实头、行和差异。身份已经被占时 inserted 为假，调用方回滚这一笔：锁内不该发生，发生了就不能再入账。
	Save(ctx context.Context, receipt domain.RecordedReceipt) (inserted bool, err error)
}

// ReceiptDiscrepancyIdentities 签发收货差异的标识。只在这一次确实形成了差异时取。
// 不并进库存流水的签发器：流水标识与差异标识类型互不相通。
type ReceiptDiscrepancyIdentities interface {
	NextReceiptDiscrepancyID() (domain.ReceiptDiscrepancyID, error)
}
