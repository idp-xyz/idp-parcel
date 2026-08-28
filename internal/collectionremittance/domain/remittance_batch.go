package domain

import (
	"fmt"
	"time"
)

// RemittanceBatchState 是回汇批次的状态，封闭二值且**单向推进**。
//
// 没有取消格、没有重开格：批次一经形成即冻结分户账键、币种与归集截点，改变归集范围
// 走新批次。给它一个取消格看似无害，实则会让「这批钱到底算不算已交出去」在同一行上
// 前后两种答案，而已依它落账的汇付记账不会跟着回退。
type RemittanceBatchState uint8

const (
	BatchStateUnknown RemittanceBatchState = iota
	// BatchCollected 已归集：范围已定，尚未交出汇付主张。
	BatchCollected
	// BatchHandedForPayment 已交出汇付主张。它**不等于真实付款**——付款执行、清算与
	// 到账由外部支付与银行系统拥有，本上下文只到主张为止。
	BatchHandedForPayment
)

func (state RemittanceBatchState) String() string {
	switch state {
	case BatchCollected:
		return "COLLECTED"
	case BatchHandedForPayment:
		return "HANDED_FOR_PAYMENT"
	default:
		return ""
	}
}

func (state RemittanceBatchState) valid() bool { return state.String() != "" }

func ParseRemittanceBatchState(text string) (RemittanceBatchState, bool) {
	for _, candidate := range []RemittanceBatchState{BatchCollected, BatchHandedForPayment} {
		if candidate.String() == text {
			return candidate, true
		}
	}
	return BatchStateUnknown, false
}

// RemittanceBatchSpec 是形成一个回汇批次所需的全部输入。状态不是输入：批次一律以
// `已归集`进册，交出汇付主张是此后的一次状态推进。
type RemittanceBatchSpec struct {
	ID               RemittanceBatchID
	Ledger           SubledgerKey
	CollectedThrough time.Time
	FormedAt         time.Time
}

// RemittanceBatch 是一次周期归集与汇付主张。
//
// **成员不在本类型里。** 批次成员就是「引用该批次的汇付记账」，本上下文不另立第二份
// 成员清单——两份成员口径必然在某一次部分失败后彼此不一致，而对得上与对不上在读面
// 上长着同一张脸。
//
// 归集截点由登记方给出，本类型不据周期推算：回汇周期属实例半边，算一个默认周期出来
// 就是替租户定了商业口径。
type RemittanceBatch struct {
	id               RemittanceBatchID
	ledger           SubledgerKey
	collectedThrough time.Time
	state            RemittanceBatchState
	formedAt         time.Time
}

// FormRemittanceBatch 形成一个批次，状态固定为`已归集`。
func FormRemittanceBatch(spec RemittanceBatchSpec) (RemittanceBatch, error) {
	if !spec.ID.valid() {
		return RemittanceBatch{}, fmt.Errorf("%w: batch ID", ErrInvalidBatch)
	}
	if !spec.Ledger.valid() {
		return RemittanceBatch{}, fmt.Errorf("%w: subledger key", ErrInvalidBatch)
	}
	if spec.CollectedThrough.IsZero() {
		return RemittanceBatch{}, fmt.Errorf("%w: collected through", ErrInvalidBatch)
	}
	if spec.FormedAt.IsZero() {
		return RemittanceBatch{}, fmt.Errorf("%w: formed at", ErrInvalidBatch)
	}
	return RemittanceBatch{
		id:               spec.ID,
		ledger:           spec.Ledger,
		collectedThrough: spec.CollectedThrough.UTC(),
		state:            BatchCollected,
		formedAt:         spec.FormedAt.UTC(),
	}, nil
}

// RehydrateRemittanceBatch 是批次行在库里的样子。它比 FormRemittanceBatch 多收一个
// 状态——已交出主张的批次读回来仍要是已交出，重放形成门会把它退回`已归集`。
func RehydrateRemittanceBatch(
	spec RemittanceBatchSpec,
	state RemittanceBatchState,
) (RemittanceBatch, error) {
	batch, err := FormRemittanceBatch(spec)
	if err != nil {
		return RemittanceBatch{}, err
	}
	if !state.valid() {
		return RemittanceBatch{}, fmt.Errorf("%w: unknown batch state %d", ErrInvalidBatch, state)
	}
	batch.state = state
	return batch, nil
}

func (batch RemittanceBatch) ID() RemittanceBatchID { return batch.id }

func (batch RemittanceBatch) Ledger() SubledgerKey { return batch.ledger }

func (batch RemittanceBatch) CollectedThrough() time.Time { return batch.collectedThrough }

func (batch RemittanceBatch) State() RemittanceBatchState { return batch.state }

func (batch RemittanceBatch) FormedAt() time.Time { return batch.formedAt }

// HandOverForPayment 把批次推进到`已交出汇付主张`。已经交出的批次再交一次交回原状态
// 与 false——重复交出不是错误（意图已达成），但也不是一次新的推进，调用方要分得开。
//
// 键、币种、归集截点与形成时刻没有任何改写入口：这个方法只动状态一格，且只朝一个方向。
func (batch RemittanceBatch) HandOverForPayment() (RemittanceBatch, bool) {
	if batch.state == BatchHandedForPayment {
		return batch, false
	}
	batch.state = BatchHandedForPayment
	return batch, true
}

// AcceptsRemittance 判断这个批次此刻还能不能被新的汇付记账引用。已交出汇付主张之后
// 不能再往里加成员——成员集合只增不改这条，界在「交出」那一刻，否则主张交出去之后
// 金额还会变。
func (batch RemittanceBatch) AcceptsRemittance() bool {
	return batch.state == BatchCollected
}
