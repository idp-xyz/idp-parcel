package ports

import "context"

// 实际履约补形成用到的两件（ADR-0231 决定一、五）：采用成立时交的门铃，与按委托列出采用了它的各段。另立文件、不进 ports.go，理由同
// SegmentCommissionAdoptionStore 的文件头。

// SegmentCommissionAdoptedIntent 是「这一段采用了委托」的门铃：信封只带（租户、段、委托），消费方按段回读采用连线与段，对段上已结束
// 的参与补形成实际履约发生项。
type SegmentCommissionAdoptedIntent struct {
	Record SegmentCommissionAdoptionRecord
}

// SegmentCommissionAdoptionHandoff 由 OutboxSegmentCommissionAdoptionHandoff 实现：门铃与采用连线同一事务入队。两者分两笔，就会有一份
// 采用落了册而它之前已结束的参与没有任何东西回头补形成。
type SegmentCommissionAdoptionHandoff interface {
	HandOffSegmentCommissionAdoption(ctx context.Context, intent SegmentCommissionAdoptedIntent) error
}

// SegmentCommissionAdoptionsByCommission 按委托列出此刻仍采用它的各段（各段的当前那一代，已解除的不列），按段排序。委托换版之后的补形成
// 据它找该去哪几段（ADR-0231 决定五）：门铃消费的那一刻才列，晚于换版落下的采用由它自己的门铃补。解除一段之后委托的开始按其余各段
// 重新派生（ADR-0232 决定二），要的也是这一张单子。
type SegmentCommissionAdoptionsByCommission interface {
	ListByCommission(ctx context.Context, key TransportCommissionKey) ([]SegmentCommissionAdoptionRecord, error)
}
