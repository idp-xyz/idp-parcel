package domain

// UnboxDifferenceKind 是开箱实点与箱单比出来的差额。结论口仍是收货差异的那一个，这里先把差额留在开箱事实上。
type UnboxDifferenceKind uint8

const (
	UnboxDifferenceKindInvalid UnboxDifferenceKind = iota
	UnboxDifferenceShort
	UnboxDifferenceOverage
	UnboxDifferenceDamage
	UnboxDifferenceWrong
)

func (kind UnboxDifferenceKind) String() string {
	switch kind {
	case UnboxDifferenceShort:
		return "SHORT"
	case UnboxDifferenceOverage:
		return "OVERAGE"
	case UnboxDifferenceDamage:
		return "DAMAGE"
	case UnboxDifferenceWrong:
		return "WRONG_GOODS"
	default:
		return ""
	}
}
