package domain

import (
	"errors"
	"fmt"
)

// 送站预报与送站交接单的单据号（GLOSSARY「单据号」；NO CONTEXT「送站预报」「送站交接单」；ADR-0355）。单据号是开立时按单据编号
// 规则分配、给人看的业务编号，不是标识：预报标识与交接单标识照旧做主键、重放与跨上下文引用，单据号不参与判重。

// ErrInvalidDocumentNumbering 说明一份单据号记录立不起来：号码空白，或无号原因认不出、与号码同在。读回时遇到它说明库里那一行坏了。
var ErrInvalidDocumentNumbering = errors.New("node operations: invalid document numbering")

// DocumentNumber 是一个单据号，原样保存：规则册渲染出什么就是什么，本上下文不折算。
type DocumentNumber struct{ requiredValue }

func NewDocumentNumber(value string) (DocumentNumber, error) {
	required, err := newRequiredValue("document number", value)
	return DocumentNumber{required}, err
}

// DocumentNumberAbsence 是一份单据没有单据号的原因。GLOSSARY「单据号」只认两种情形，事后都不补号；两者的续办不同——前者没有续办，
// 后者是租户漏登了单据编号时区——所以记下是哪一种，页面据此分开说，不一律写成「上线前开立」。
type DocumentNumberAbsence uint8

const (
	DocumentNumberAbsenceInvalid DocumentNumberAbsence = iota
	// IssuedBeforeDocumentNumbering：单据号规则生效之前开立。库里号码与原因两格都空的旧行就是这一格，它只从读回来，开立不会写出它。
	IssuedBeforeDocumentNumbering
	// NumberingTimeZoneNotConfigured：开立时开立网点没有生效的业务时区，租户也没登单据编号时区（ADR-0355 决定七）。单据照常开立。
	NumberingTimeZoneNotConfigured
)

func (absence DocumentNumberAbsence) String() string {
	switch absence {
	case IssuedBeforeDocumentNumbering:
		return "ISSUED_BEFORE_DOCUMENT_NUMBERING"
	case NumberingTimeZoneNotConfigured:
		return "NUMBERING_TIME_ZONE_NOT_CONFIGURED"
	default:
		return ""
	}
}

// DocumentNumbering 是一份单据上记下的单据号，或没有它的原因——两者恰有其一。零值即「还没定」：开立编排取号之前的那一刻，册的写口
// 拒收这样的单据，免得新单据落成两格都空、读回来被当成上线前开立。
type DocumentNumbering struct {
	number  DocumentNumber
	absence DocumentNumberAbsence
}

// NumberedDocument 是开立时取到了号。
func NumberedDocument(number DocumentNumber) DocumentNumbering {
	return DocumentNumbering{number: number}
}

// UnnumberedForMissingTimeZone 是开立时单据编号时区未配置、不带号（ADR-0355 决定七）。
func UnnumberedForMissingTimeZone() DocumentNumbering {
	return DocumentNumbering{absence: NumberingTimeZoneNotConfigured}
}

// RehydrateDocumentNumbering 从库里那两格还原：号码在场就是有号；两格都空是上线前开立；只有原因的按原因还原。号码与原因同在、原因
// 认不出，都是那一行坏了。
func RehydrateDocumentNumbering(number, absence string) (DocumentNumbering, error) {
	switch {
	case number != "" && absence != "":
		return DocumentNumbering{}, fmt.Errorf("%w: a number %q and an absence %q at once", ErrInvalidDocumentNumbering, number, absence)
	case number != "":
		parsed, err := NewDocumentNumber(number)
		if err != nil {
			return DocumentNumbering{}, fmt.Errorf("%w: %v", ErrInvalidDocumentNumbering, err)
		}
		return NumberedDocument(parsed), nil
	case absence == "":
		return DocumentNumbering{absence: IssuedBeforeDocumentNumbering}, nil
	case absence == NumberingTimeZoneNotConfigured.String():
		return UnnumberedForMissingTimeZone(), nil
	default:
		return DocumentNumbering{}, fmt.Errorf("%w: absence %q", ErrInvalidDocumentNumbering, absence)
	}
}

// Number 交回单据号；没有号答 false。
func (numbering DocumentNumbering) Number() (DocumentNumber, bool) {
	return numbering.number, numbering.number.valid()
}

// Absence 交回没有单据号的原因；有号或还没定答 false。
func (numbering DocumentNumbering) Absence() (DocumentNumberAbsence, bool) {
	return numbering.absence, !numbering.number.valid() && numbering.absence != DocumentNumberAbsenceInvalid
}

// Decided 报告单据号这一格定了没有：有号，或记下了无号原因。
func (numbering DocumentNumbering) Decided() bool {
	_, numbered := numbering.Number()
	_, absent := numbering.Absence()
	return numbered || absent
}
