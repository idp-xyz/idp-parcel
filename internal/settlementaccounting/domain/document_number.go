package domain

import "fmt"

// DocumentNumber 是一张单据开立时按单据编号规则分配的单据号（ADR-0355 决定一）：给人看、给相对方引用，不是内部
// 标识，不参与幂等判重。费用争议与审核应付各带一个；对账单不用它——对账单的固定单号本来就是它的单据号
// （StatementNumber，决定九）。
type DocumentNumber struct{ requiredValue }

func NewDocumentNumber(value string) (DocumentNumber, error) {
	required, err := newRequiredValue("document number", value)
	return DocumentNumber{required}, err
}

// DocumentNumberAbsence 说一张单据为什么没有单据号。两种原因页面上写法不同（ADR-0355 决定七、十一），不能一律
// 写成「上线前开立」：时区没配而无号的是新单据，漏登的代价落在租户的上线检查上，页面得让人看出这一点。
type DocumentNumberAbsence uint8

const (
	DocumentNumberAbsenceInvalid DocumentNumberAbsence = iota
	// IssuedBeforeNumbering：本决定生效前开立，不补号（决定三）。
	IssuedBeforeNumbering
	// NumberingTimeZoneUnconfigured：开立时开立方没有业务时区、租户也没登单据编号时区，单据照常开立而不带号；之后
	// 补登也不补（决定七）。
	NumberingTimeZoneUnconfigured
)

func (absence DocumentNumberAbsence) String() string {
	switch absence {
	case IssuedBeforeNumbering:
		return "ISSUED_BEFORE_NUMBERING"
	case NumberingTimeZoneUnconfigured:
		return "NUMBERING_TIME_ZONE_NOT_CONFIGURED"
	default:
		return ""
	}
}

// DocumentNumbering 是单据上「号是多少，或为什么没号」那一格，两者恰有其一。零值不成立：新开立的单据必须由取号
// 那一步给出其中一种，落库前据此拦下漏接取号的路径——否则它会悄悄落成一张「上线前开立」的单。
type DocumentNumbering struct {
	number  DocumentNumber
	absence DocumentNumberAbsence
}

func NumberedDocument(number DocumentNumber) (DocumentNumbering, error) {
	if !number.valid() {
		return DocumentNumbering{}, fmt.Errorf("%w: document number", ErrBlankValue)
	}
	return DocumentNumbering{number: number}, nil
}

func UnnumberedDocument(absence DocumentNumberAbsence) (DocumentNumbering, error) {
	if absence.String() == "" {
		return DocumentNumbering{}, fmt.Errorf("settlement accounting: unknown document number absence %d", absence)
	}
	return DocumentNumbering{absence: absence}, nil
}

// Number 交回单据号；没有号时 false。
func (numbering DocumentNumbering) Number() (DocumentNumber, bool) {
	return numbering.number, numbering.number.valid()
}

// Absence 交回无号原因；有号时 false。
func (numbering DocumentNumbering) Absence() (DocumentNumberAbsence, bool) {
	return numbering.absence, !numbering.number.valid() && numbering.absence.String() != ""
}

// Valid 答这一格是否由取号那一步给出过（有号，或有一个具名的无号原因）。
func (numbering DocumentNumbering) Valid() bool {
	_, numbered := numbering.Number()
	_, absent := numbering.Absence()
	return numbered || absent
}
