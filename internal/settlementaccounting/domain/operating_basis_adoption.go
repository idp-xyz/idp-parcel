package domain

import "errors"

// ErrOperatingBasisAdoptionEmpty 说明两个口径都没采用。采用是租户取值：可以只采用预估、
// 只采用已确认，或两者都采用，但不能一行都是假——那一行与「没登记」分不开。
var ErrOperatingBasisAdoptionEmpty = errors.New("settlement accounting: operating basis adoption adopts nothing")

// OperatingBasisAdoption 是一户对两种内置经营口径的采用。已结算口径不在这行里：本票不给它
// 生产触发，领域里的口径本身保留。没这一行不是「两个都是假」，是册上没有这户。
type OperatingBasisAdoption struct {
	estimated bool
	confirmed bool
}

// NewOperatingBasisAdoption 收两个都必须由调用方给出的布尔。两个都是假拒，不把缺席当成不采用。
func NewOperatingBasisAdoption(estimated, confirmed bool) (OperatingBasisAdoption, error) {
	if !estimated && !confirmed {
		return OperatingBasisAdoption{}, ErrOperatingBasisAdoptionEmpty
	}
	return OperatingBasisAdoption{estimated: estimated, confirmed: confirmed}, nil
}

func (adoption OperatingBasisAdoption) Estimated() bool { return adoption.estimated }

func (adoption OperatingBasisAdoption) Confirmed() bool { return adoption.confirmed }

// Adopts 只认预估与已确认。已结算与零值一律不采用，避免没点名的口径被这行放行。
func (adoption OperatingBasisAdoption) Adopts(basis OperatingBasis) bool {
	switch basis {
	case EstimatedBasis:
		return adoption.estimated
	case ConfirmedBasis:
		return adoption.confirmed
	default:
		return false
	}
}

// Equal 是同一内容：重放与冲突按这两个布尔比，不按落库时刻。
func (adoption OperatingBasisAdoption) Equal(other OperatingBasisAdoption) bool {
	return adoption.estimated == other.estimated && adoption.confirmed == other.confirmed
}
