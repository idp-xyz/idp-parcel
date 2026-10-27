package domain

import (
	"fmt"
	"time"
)

// AgreedOffsetSpec 是登记一条认可的协议抵扣所需的全部输入。
type AgreedOffsetSpec struct {
	ID          AgreedOffsetID
	Instruction CollectionInstructionID
	Amount      Money
	Approval    ApprovalReference
	ApprovedAt  time.Time
}

// AgreedOffset 是 settlement-accounting 认可的一笔协议抵扣在本上下文的登记：有效协议
// 允许渠道从回款里扣下的金额，认可后这部分本金不再是渠道还欠的，也不会付给客户。
//
// 认可与金额归 settlement-accounting，本类型只保存对它的引用和当时引用的金额，不据
// 此形成任何运营应收应付，也不在账内与服务费或运费净额。它与差异事项分开，是因为
// 两者的处置相反：差异事项是待处置的异常声明，认可的抵扣是已经按协议了结的正常扣减，
// 不进差异的处置流程。
//
// 登记只是把引用钉到一条指令上，不动账：落账要另有一笔以本记录为依据的记账
// （RecordSubledgerPosting 的认可抵扣门），分两步是为了让分户账键和按指令的上限由
// 依据推出，而不是由调用方传入。登记后不可改写；认可被更正走新认可新记录，被撤回走
// 冲正已落的那一笔。
type AgreedOffset struct {
	id          AgreedOffsetID
	instruction CollectionInstructionID
	amount      Money
	approval    ApprovalReference
	approvedAt  time.Time
}

func RegisterAgreedOffset(spec AgreedOffsetSpec) (AgreedOffset, error) {
	if !spec.ID.valid() {
		return AgreedOffset{}, fmt.Errorf("%w: agreed offset ID", ErrInvalidAgreedOffset)
	}
	if !spec.Instruction.valid() {
		return AgreedOffset{}, fmt.Errorf("%w: instruction ID", ErrInvalidAgreedOffset)
	}
	if !spec.Amount.valid() {
		return AgreedOffset{}, fmt.Errorf("%w: amount", ErrInvalidAgreedOffset)
	}
	if !spec.Approval.valid() {
		return AgreedOffset{}, fmt.Errorf("%w: approval reference", ErrInvalidAgreedOffset)
	}
	if spec.ApprovedAt.IsZero() {
		return AgreedOffset{}, fmt.Errorf("%w: approved at", ErrInvalidAgreedOffset)
	}
	return AgreedOffset{
		id:          spec.ID,
		instruction: spec.Instruction,
		amount:      spec.Amount,
		approval:    spec.Approval,
		approvedAt:  spec.ApprovedAt.UTC(),
	}, nil
}

func (offset AgreedOffset) ID() AgreedOffsetID { return offset.id }

func (offset AgreedOffset) Instruction() CollectionInstructionID { return offset.instruction }

func (offset AgreedOffset) Amount() Money { return offset.amount }

func (offset AgreedOffset) Approval() ApprovalReference { return offset.approval }

func (offset AgreedOffset) ApprovedAt() time.Time { return offset.approvedAt }
