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
// 溢款之后还有一笔归属确认（`溢款` → `应付客户`），那是另一笔记账，本方法只说差额
// 落进的那一格。
func (item DiscrepancyItem) SettlementPosition() FundPosition {
	if item.kind == DiscrepancyShortfall {
		return PositionShortfall
	}
	return PositionSurplus
}

// SettlementSource 是差额落账唯一允许的来源位置（ADR-0271 决定四、五）。
//
// 短款只从`渠道在途`来：差额那部分钱从没回到运营企业，账上它只可能还在渠道名下；从
// `待清分`或`应付客户`挪，挪走的是已经到账、甚至已归属客户的本金，客户因此少拿钱。
// 溢款只从`待清分`来：它之后能归到`应付客户`，所以只能装已经到账的钱，从`渠道在途`
// 落入就让没收到的钱绕过了「只有真实到账才推进到待清分」那道门。
func (item DiscrepancyItem) SettlementSource() FundPosition {
	if item.kind == DiscrepancyShortfall {
		return PositionInTransitAtChannel
	}
	return PositionAwaitingAllocation
}
