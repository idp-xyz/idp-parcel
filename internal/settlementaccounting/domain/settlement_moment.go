package domain

import "fmt"

// SettlementMoment 是确认或截单的触发时点。没有钟点，也没有账期。
type SettlementMoment uint8

const (
	SettlementMomentInvalid SettlementMoment = iota
	SettlementMomentConfirm
	SettlementMomentCutOff
)

func (moment SettlementMoment) String() string {
	switch moment {
	case SettlementMomentConfirm:
		return "CONFIRM"
	case SettlementMomentCutOff:
		return "CUT_OFF"
	default:
		return ""
	}
}

func (moment SettlementMoment) admits() bool {
	return moment == SettlementMomentConfirm || moment == SettlementMomentCutOff
}

// SettlementMomentFromName 只认确认与截单。词表外拒，不夹成其中一套。
func SettlementMomentFromName(name string) (SettlementMoment, error) {
	switch name {
	case "CONFIRM":
		return SettlementMomentConfirm, nil
	case "CUT_OFF":
		return SettlementMomentCutOff, nil
	default:
		return SettlementMomentInvalid, fmt.Errorf("%w: settlement moment", ErrBlankValue)
	}
}
