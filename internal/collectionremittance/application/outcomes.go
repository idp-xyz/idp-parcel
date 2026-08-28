// Package application 是代收与清分的用例编排：把一次登记或记账推进到一个封闭答案。
//
// 事务由进程级入口给出（受控 CLI 一次调用一笔事务）。写口 RequireExecutor 无环境事务
// 即拒，因此「读回账面」与「落账」必然看同一份快照——余额守卫依赖这一点。
package application

// Outcome 是本上下文全部用例的封闭答案。格分得细不是为了好看：每一格对应的下一步
// 动作都不同，合并任意两格都会让处置者对着一个答案猜该干什么。
type Outcome uint8

const (
	OutcomeInvalid Outcome = iota
	// OutcomeNotAccepted 受理门拒：输入缺格或形状不对。改请求，重跑同一份没有意义。
	OutcomeNotAccepted
	// OutcomeRegistered 本次落册或落账。
	OutcomeRegistered
	// OutcomeExisting 同键已在册且内容逐字段相同——重放。
	OutcomeExisting
	// OutcomeContentConflict 同键已在册但内容不同。**绝不覆盖**，要人核对既有登记。
	OutcomeContentConflict
	// OutcomeBasisMissing 依据不在册：记账指名的代收事实、代收指令、回汇批次、差异
	// 事项或原记账查不到，或这本分户账还没开立。它与受理门拒分开——输入形状没问题，
	// 缺的是上游那一笔，续办动作是先把依据登进来。
	OutcomeBasisMissing
	// OutcomeUnderfunded 来源位置余额不足。请求可能完全正确，只是此刻账上还没那么
	// 多钱：等实收或先清分，不是改请求。
	OutcomeUnderfunded
	// OutcomeBatchClosed 批次已交出汇付主张，不再收新成员。成员只增不改的界就在
	// 那一刻——要继续归集就形成新批次。
	OutcomeBatchClosed
	// OutcomeHandedOver 本次交出汇付主张。
	OutcomeHandedOver
	// OutcomeAlreadyHandedOver 已经交出过。意图已达成，但不是一次新的推进。
	OutcomeAlreadyHandedOver
	// OutcomeUndecided 依赖故障：登记与否未知，重跑同一命令即可续办。
	OutcomeUndecided
)

func (outcome Outcome) String() string {
	switch outcome {
	case OutcomeNotAccepted:
		return "NOT_ACCEPTED"
	case OutcomeRegistered:
		return "REGISTERED"
	case OutcomeExisting:
		return "EXISTING"
	case OutcomeContentConflict:
		return "CONTENT_CONFLICT"
	case OutcomeBasisMissing:
		return "BASIS_MISSING"
	case OutcomeUnderfunded:
		return "UNDERFUNDED"
	case OutcomeBatchClosed:
		return "BATCH_CLOSED"
	case OutcomeHandedOver:
		return "HANDED_OVER"
	case OutcomeAlreadyHandedOver:
		return "ALREADY_HANDED_OVER"
	case OutcomeUndecided:
		return "UNDECIDED"
	default:
		return "UNKNOWN"
	}
}
