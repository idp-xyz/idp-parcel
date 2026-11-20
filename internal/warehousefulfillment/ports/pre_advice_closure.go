package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// PreAdviceClosureRead 按预报读已经落册的完结。读不动走 error，不折成没有：没有是这份预报还没完结。
type PreAdviceClosureRead interface {
	ListByPreAdvices(
		ctx context.Context,
		tenant domain.TenantID,
		ids []domain.PreAdviceID,
	) ([]domain.PreAdviceClosure, error)
}

// PreAdviceClosureStore 是完结与少收的册。完结在（租户，预报）上至多一行，只追加。
//
// Save 在没有环境事务时拒绝。List 用环境里的读执行器。
// 落册用 ON CONFLICT DO NOTHING：唯一冲突会把当前事务打成不可用，而预报状态还在同一笔里。
type PreAdviceClosureStore interface {
	PreAdviceClosureRead
	// Save 写入完结头和少收行。这一行已经被占时 inserted 为假。锁内不该发生，发生了调用方必须回滚，不能再推进预报状态。
	Save(ctx context.Context, closure domain.PreAdviceClosure) (inserted bool, err error)
}
