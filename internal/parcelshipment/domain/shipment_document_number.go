package domain

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidShipmentDocumentNumbering 表示单据号这一格立不起来：既没有号也没有说清无号原因，或原因不在封闭集合里。
	ErrInvalidShipmentDocumentNumbering = errors.New("parcel shipment: invalid shipment document numbering")
	// ErrShipmentDocumentNumberingAlreadyRecorded 表示委托的单据号这一格已经有了结果。号开立后不变、事后不补
	// （ADR-0355 决定三；GLOSSARY「单据号」），所以这一格只记一次。
	ErrShipmentDocumentNumberingAlreadyRecorded = errors.New("parcel shipment: shipment document numbering is already recorded")
)

// ShipmentDocumentNumber 是委托的单据号（GLOSSARY「单据号」；管理台与客户面前叫「运单号」）。它不是委托标识，也不是
// 客户单号等外部标识：委托标识照旧做主键、幂等键与跨上下文引用，单据号只给人看、给相对方引用，不参与判重。
// 逐字保存，不做大小写或空白折算——号由编号规则渲染出来，折算就是另出了一个号。
type ShipmentDocumentNumber struct{ requiredValue }

func NewShipmentDocumentNumber(value string) (ShipmentDocumentNumber, error) {
	required, err := newRequiredValue("shipment document number", value)
	return ShipmentDocumentNumber{required}, err
}

// DocumentNumberAbsence 是委托没有单据号时的那一种情形。GLOSSARY「单据号」列了两种，开立都照常：没有单据号的委托
// 记下是哪一种，管理台按它注明，不一律写成「上线前开立」。
type DocumentNumberAbsence uint8

const (
	DocumentNumberAbsenceInvalid DocumentNumberAbsence = iota
	// IssuedBeforeDocumentNumbering：本规则生效前建的委托。库里两列皆空就是它，不另写值。
	IssuedBeforeDocumentNumbering
	// DocumentNumberingTimeZoneNotConfigured：开立时开立方没有业务时区、租户也没登单据编号时区（ADR-0355 决定七）。
	// 之后补登也不给它补号。
	DocumentNumberingTimeZoneNotConfigured
)

func (absence DocumentNumberAbsence) String() string {
	switch absence {
	case IssuedBeforeDocumentNumbering:
		return "ISSUED_BEFORE_NUMBERING"
	case DocumentNumberingTimeZoneNotConfigured:
		return "NUMBERING_TIME_ZONE_NOT_CONFIGURED"
	default:
		return ""
	}
}

// ShipmentDocumentNumbering 是委托单据号这一格的结果：有号，或记下无号的那一种情形。
//
// 零值是「还没开立」，只在建单那一笔事务里出现：委托先落库，确定落单了才取号（ADR-0355 决定三「取号只发生在确定要
// 落单的那一步」），取完同一笔记上。仓储读回库里两列皆空的行写 IssuedBeforeDocumentNumbering，不留零值——零值若能从
// 库里读出来，那份委托就能被补上号。
type ShipmentDocumentNumbering struct {
	number  ShipmentDocumentNumber
	absence DocumentNumberAbsence
}

// NumberedShipment 是取到了号的那一格。
func NumberedShipment(number ShipmentDocumentNumber) (ShipmentDocumentNumbering, error) {
	if !number.valid() {
		return ShipmentDocumentNumbering{}, fmt.Errorf("%w: blank number", ErrInvalidShipmentDocumentNumbering)
	}
	return ShipmentDocumentNumbering{number: number}, nil
}

// UnnumberedShipment 是没有号、记下原因的那一格。
func UnnumberedShipment(absence DocumentNumberAbsence) (ShipmentDocumentNumbering, error) {
	if absence.String() == "" {
		return ShipmentDocumentNumbering{}, fmt.Errorf("%w: absence %d", ErrInvalidShipmentDocumentNumbering, absence)
	}
	return ShipmentDocumentNumbering{absence: absence}, nil
}

// Number 交回单据号；没有号时答 false。
func (numbering ShipmentDocumentNumbering) Number() (ShipmentDocumentNumber, bool) {
	return numbering.number, numbering.number.valid()
}

// Absence 交回无号的那一种情形；有号或还没开立时答 false。
func (numbering ShipmentDocumentNumbering) Absence() (DocumentNumberAbsence, bool) {
	return numbering.absence, numbering.absence.String() != ""
}

// Recorded 答这一格是否已有结果（有号或记了无号原因）。
func (numbering ShipmentDocumentNumbering) Recorded() bool {
	_, numbered := numbering.Number()
	_, absent := numbering.Absence()
	return numbered || absent
}

// DocumentNumbering 交回委托单据号这一格。
func (request ShipmentRequest) DocumentNumbering() ShipmentDocumentNumbering {
	return request.documentNumbering
}

// RecordDocumentNumbering 在刚建成的委托上记下开立的结果。只记一次：已经有结果的委托（含读回的上线前委托）一律拒，
// 开立后不换号、事后不补号（ADR-0355 决定三）。只对`已提交`开放——单据号在草稿 → 已提交那一刻开立（PS CONTEXT「委托」）。
func (request ShipmentRequest) RecordDocumentNumbering(numbering ShipmentDocumentNumbering) (ShipmentRequest, error) {
	if !numbering.Recorded() {
		return ShipmentRequest{}, fmt.Errorf("%w: nothing to record", ErrInvalidShipmentDocumentNumbering)
	}
	if request.documentNumbering.Recorded() {
		return ShipmentRequest{}, ErrShipmentDocumentNumberingAlreadyRecorded
	}
	if request.state != ShipmentRequestSubmitted {
		return ShipmentRequest{}, fmt.Errorf("%w: document numbering is recorded on submission, state is %s",
			ErrInvalidShipmentDocumentNumbering, request.state)
	}
	request.documentNumbering = numbering
	return request, nil
}
