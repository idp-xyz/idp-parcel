package domain

import (
	"fmt"
	"time"
)

// DiscrepancyKind 是差异的方向，封闭二值：实收少于指令是短款，多于指令是溢款。
type DiscrepancyKind uint8

const (
	DiscrepancyKindUnknown DiscrepancyKind = iota
	DiscrepancyShortfall
	DiscrepancySurplus
)

func (kind DiscrepancyKind) String() string {
	switch kind {
	case DiscrepancyShortfall:
		return "SHORTFALL"
	case DiscrepancySurplus:
		return "SURPLUS"
	default:
		return ""
	}
}

func (kind DiscrepancyKind) valid() bool { return kind.String() != "" }

func ParseDiscrepancyKind(text string) (DiscrepancyKind, bool) {
	for _, candidate := range []DiscrepancyKind{DiscrepancyShortfall, DiscrepancySurplus} {
		if candidate.String() == text {
			return candidate, true
		}
	}
	return DiscrepancyKindUnknown, false
}

// DiscrepancyItemSpec 是登记一项差异事项所需的全部输入。
type DiscrepancyItemSpec struct {
	ID          DiscrepancyItemID
	Instruction CollectionInstructionID
	Kind        DiscrepancyKind
	Amount      Money
	Basis       BasisReference
	ObservedAt  time.Time
}

// DiscrepancyItem 是「实收≠指令」时登记的待处置事项。
//
// 它**不自动冲销**：本类型不改写代收指令，也不自行把差额记进任何资金位置。差额落账
// 要另有一笔以本事项为依据的记账（RecordSubledgerPosting 的差异门就是那一格）。
// 分成两步不是麻烦——短款靠缩小指令抹平、溢款直接计入应付客户款，都是一步就能做完
// 而且在账上看不出来的错法。
type DiscrepancyItem struct {
	id          DiscrepancyItemID
	instruction CollectionInstructionID
	kind        DiscrepancyKind
	amount      Money
	basis       BasisReference
	observedAt  time.Time
}

func RegisterDiscrepancyItem(spec DiscrepancyItemSpec) (DiscrepancyItem, error) {
	if !spec.ID.valid() {
		return DiscrepancyItem{}, fmt.Errorf("%w: discrepancy ID", ErrInvalidDiscrepancy)
	}
	if !spec.Instruction.valid() {
		return DiscrepancyItem{}, fmt.Errorf("%w: instruction ID", ErrInvalidDiscrepancy)
	}
	if !spec.Kind.valid() {
		return DiscrepancyItem{}, fmt.Errorf(
			"%w: unknown discrepancy kind %d", ErrInvalidDiscrepancy, spec.Kind)
	}
	if !spec.Amount.valid() {
		return DiscrepancyItem{}, fmt.Errorf("%w: amount", ErrInvalidDiscrepancy)
	}
	if !spec.Basis.valid() {
		return DiscrepancyItem{}, fmt.Errorf("%w: basis reference", ErrInvalidDiscrepancy)
	}
	if spec.ObservedAt.IsZero() {
		return DiscrepancyItem{}, fmt.Errorf("%w: observed at", ErrInvalidDiscrepancy)
	}
	return DiscrepancyItem{
		id:          spec.ID,
		instruction: spec.Instruction,
		kind:        spec.Kind,
		amount:      spec.Amount,
		basis:       spec.Basis,
		observedAt:  spec.ObservedAt.UTC(),
	}, nil
}

func (item DiscrepancyItem) ID() DiscrepancyItemID { return item.id }

func (item DiscrepancyItem) Instruction() CollectionInstructionID { return item.instruction }

func (item DiscrepancyItem) Kind() DiscrepancyKind { return item.kind }

func (item DiscrepancyItem) Amount() Money { return item.amount }

func (item DiscrepancyItem) Basis() BasisReference { return item.basis }

func (item DiscrepancyItem) ObservedAt() time.Time { return item.observedAt }

// SettlementPosition 是本事项处置后差额应落的资金位置：短款落`短款`，溢款落`溢款`。
// 记账方向仍由记账方给出——溢款既可能自`待清分`移入`溢款`，也可能在归属确认后自
// `溢款`移回`应付客户`，那是两笔不同的记账，本方法只说差额那一格在哪。
func (item DiscrepancyItem) SettlementPosition() FundPosition {
	if item.kind == DiscrepancyShortfall {
		return PositionShortfall
	}
	return PositionSurplus
}
