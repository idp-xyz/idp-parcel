package domain

import (
	"fmt"
	"time"
)

// CollectionInstructionSpec 是成立一条代收指令所需的全部输入。
type CollectionInstructionSpec struct {
	ID           CollectionInstructionID
	Parcel       ParcelReference
	Requirement  ServiceRequirementReference
	Ledger       SubledgerKey
	Amount       Money
	InstructedAt time.Time
}

// CollectionInstruction 是一项代收资金义务：依据某份客户代收服务要求，该包裹应向
// 收件人收取此金额。它是**义务的登记，不是收款事实**——实收由 CollectionFact 表达，
// 已可付给客户的钱由分户账的`应付客户`余额表达，三者互不推导。
//
// 分户账四维随指令固定：本金记进哪一本账在义务成立时就已确定，不由后续记账挑选。
// 让记账挑账等于把「这笔钱是谁的」推迟到最后一刻，而那时判断依据（服务要求快照）
// 已经不在手边了。
type CollectionInstruction struct {
	id           CollectionInstructionID
	parcel       ParcelReference
	requirement  ServiceRequirementReference
	ledger       SubledgerKey
	amount       Money
	instructedAt time.Time
}

// IssueCollectionInstruction 成立一条代收指令。服务要求引用必填：没有它就看不出这项
// 受托义务凭什么成立，而缺依据的义务在册面上与「运营企业自己要收的钱」分不开。
func IssueCollectionInstruction(spec CollectionInstructionSpec) (CollectionInstruction, error) {
	if !spec.ID.valid() {
		return CollectionInstruction{}, fmt.Errorf("%w: instruction ID", ErrInvalidInstruction)
	}
	if !spec.Parcel.valid() {
		return CollectionInstruction{}, fmt.Errorf("%w: parcel reference", ErrInvalidInstruction)
	}
	if !spec.Requirement.valid() {
		return CollectionInstruction{}, fmt.Errorf("%w: service requirement reference", ErrInvalidInstruction)
	}
	if !spec.Ledger.valid() {
		return CollectionInstruction{}, fmt.Errorf("%w: subledger key", ErrInvalidInstruction)
	}
	if !spec.Amount.valid() {
		return CollectionInstruction{}, fmt.Errorf("%w: amount", ErrInvalidInstruction)
	}
	if spec.Amount.Currency() != spec.Ledger.Currency() {
		return CollectionInstruction{}, fmt.Errorf(
			"%w: amount currency %q is not the subledger currency %q",
			ErrInvalidInstruction, spec.Amount.Currency(), spec.Ledger.Currency())
	}
	if spec.InstructedAt.IsZero() {
		return CollectionInstruction{}, fmt.Errorf("%w: instructed at", ErrInvalidInstruction)
	}
	return CollectionInstruction{
		id:           spec.ID,
		parcel:       spec.Parcel,
		requirement:  spec.Requirement,
		ledger:       spec.Ledger,
		amount:       spec.Amount,
		instructedAt: spec.InstructedAt.UTC(),
	}, nil
}

func (instruction CollectionInstruction) ID() CollectionInstructionID { return instruction.id }

func (instruction CollectionInstruction) Parcel() ParcelReference { return instruction.parcel }

func (instruction CollectionInstruction) Requirement() ServiceRequirementReference {
	return instruction.requirement
}

func (instruction CollectionInstruction) Ledger() SubledgerKey { return instruction.ledger }

func (instruction CollectionInstruction) Amount() Money { return instruction.amount }

func (instruction CollectionInstruction) InstructedAt() time.Time { return instruction.instructedAt }
