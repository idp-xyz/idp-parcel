package domain

import (
	"slices"
	"time"
)

// ReceiptFactID 是收货事实的身份，由设备签发，本上下文不另签一个。
type ReceiptFactID struct{ requiredValue }

func NewReceiptFactID(value string) (ReceiptFactID, error) {
	required, err := newRequiredValue("receipt fact ID", value)
	return ReceiptFactID{required}, err
}

// ReceiptDeviceID 是签发这条收货事实的设备，逐字保存。
type ReceiptDeviceID struct{ requiredValue }

func NewReceiptDeviceID(value string) (ReceiptDeviceID, error) {
	required, err := newRequiredValue("receipt device", value)
	return ReceiptDeviceID{required}, err
}

// ReceiptOperatorID 是确认这次收货的操作者，逐字保存。它不是预报的登记责任方。
type ReceiptOperatorID struct{ requiredValue }

func NewReceiptOperatorID(value string) (ReceiptOperatorID, error) {
	required, err := newRequiredValue("receipt operator", value)
	return ReceiptOperatorID{required}, err
}

// ReceiptDiscrepancyID 是一份收货差异的标识，由本上下文签发。前缀不在这个类型里：签发器单独打，不并进库存流水。
type ReceiptDiscrepancyID struct{ requiredValue }

func NewReceiptDiscrepancyID(value string) (ReceiptDiscrepancyID, error) {
	required, err := newRequiredValue("receipt discrepancy ID", value)
	return ReceiptDiscrepancyID{required}, err
}

// ReceiptLineResult 是收货已确认时一行的结果。没入账的不在这一格里，原因另记。
type ReceiptLineResult uint8

const (
	ReceiptLineResultNone ReceiptLineResult = iota
	ReceiptLinePosted
	ReceiptLineUnknownGoods
)

func (result ReceiptLineResult) String() string {
	switch result {
	case ReceiptLinePosted:
		return "POSTED"
	case ReceiptLineUnknownGoods:
		return "UNKNOWN_GOODS"
	default:
		return ""
	}
}

// ReceiptUnpostedReason 是一行没有入账的原因。已入账的行没有这一格。
type ReceiptUnpostedReason uint8

const (
	ReceiptUnpostedNone ReceiptUnpostedReason = iota
	ReceiptUnpostedUnknownGoods
	ReceiptUnpostedNotReceivable
	ReceiptUnpostedWrongCarton
	ReceiptUnpostedCartonNumberInUse
)

func (reason ReceiptUnpostedReason) String() string {
	switch reason {
	case ReceiptUnpostedUnknownGoods:
		return "UNKNOWN_GOODS"
	case ReceiptUnpostedNotReceivable:
		return "NOT_RECEIVABLE"
	case ReceiptUnpostedWrongCarton:
		return "WRONG_CARTON"
	case ReceiptUnpostedCartonNumberInUse:
		return "CARTON_NUMBER_IN_USE"
	default:
		return ""
	}
}

// ReceiptDiscrepancyKind 是这一次形成的收货差异种类。少收不在商品入库这一段。
type ReceiptDiscrepancyKind uint8

const (
	ReceiptDiscrepancyKindInvalid ReceiptDiscrepancyKind = iota
	ReceiptDiscrepancyOverage
	ReceiptDiscrepancyWrongGoods
	ReceiptDiscrepancyDamage
)

func (kind ReceiptDiscrepancyKind) String() string {
	switch kind {
	case ReceiptDiscrepancyOverage:
		return "OVERAGE"
	case ReceiptDiscrepancyWrongGoods:
		return "WRONG_GOODS"
	case ReceiptDiscrepancyDamage:
		return "DAMAGE"
	default:
		return ""
	}
}

func NewReceiptDiscrepancyKind(word string) (ReceiptDiscrepancyKind, error) {
	for _, kind := range []ReceiptDiscrepancyKind{
		ReceiptDiscrepancyOverage, ReceiptDiscrepancyWrongGoods, ReceiptDiscrepancyDamage,
	} {
		if kind.String() == word {
			return kind, nil
		}
	}
	return ReceiptDiscrepancyKindInvalid, ErrInvalidReceipt
}

// RecordedReceiptOutcome 是落了册的收货事实的结果。没落册的答复不进这一格。
type RecordedReceiptOutcome uint8

const (
	RecordedReceiptOutcomeInvalid RecordedReceiptOutcome = iota
	RecordedReceiptConfirmed
	RecordedReceiptDivertedToUnclaimed
)

func (outcome RecordedReceiptOutcome) String() string {
	switch outcome {
	case RecordedReceiptConfirmed:
		return "CONFIRMED"
	case RecordedReceiptDivertedToUnclaimed:
		return "DIVERTED_TO_UNCLAIMED"
	default:
		return ""
	}
}

// ReceiptLine 是收货事实的一行。Observed 是扫到的码或描述，逐字保存。
// 认得出时 Goods 是解析到的商品；结果与未入账原因在落册时才有，提交比内容时不看它们。
type ReceiptLine struct {
	observed string
	unit     GoodsUnitName
	state    StockState
	quantity int64
	batch    BatchNumber
	expiry   ExpiryDate
	alias    GoodsAliasCode
	location LocationCode
	goods    GoodsCode
	result   ReceiptLineResult
	unposted ReceiptUnpostedReason
}

func NewReceiptLine(
	observed string,
	unit GoodsUnitName,
	state StockState,
	quantity int64,
	batch BatchNumber,
	expiry ExpiryDate,
	alias GoodsAliasCode,
	location LocationCode,
) (ReceiptLine, error) {
	text, err := newRequiredValue("observed code", observed)
	if err != nil || !unit.valid() || state.String() == "" || !location.valid() {
		return ReceiptLine{}, ErrInvalidReceipt
	}
	return ReceiptLine{
		observed: text.String(), unit: unit, state: state, quantity: quantity,
		batch: batch, expiry: expiry, alias: alias, location: location,
	}, nil
}

func (line ReceiptLine) Observed() string          { return line.observed }
func (line ReceiptLine) Unit() GoodsUnitName       { return line.unit }
func (line ReceiptLine) State() StockState         { return line.state }
func (line ReceiptLine) Quantity() int64           { return line.quantity }
func (line ReceiptLine) Batch() BatchNumber        { return line.batch }
func (line ReceiptLine) Expiry() ExpiryDate        { return line.expiry }
func (line ReceiptLine) Alias() GoodsAliasCode     { return line.alias }
func (line ReceiptLine) Location() LocationCode    { return line.location }
func (line ReceiptLine) Goods() GoodsCode          { return line.goods }
func (line ReceiptLine) Result() ReceiptLineResult { return line.result }
func (line ReceiptLine) Unposted() ReceiptUnpostedReason {
	return line.unposted
}

// AsPosted 交回已入账的副本。商品必须已经解析出来。
func (line ReceiptLine) AsPosted(goods GoodsCode) (ReceiptLine, error) {
	if !goods.valid() || line.quantity <= 0 {
		return ReceiptLine{}, ErrInvalidReceipt
	}
	line.goods = goods
	line.result = ReceiptLinePosted
	line.unposted = ReceiptUnpostedNone
	return line, nil
}

// AsUnknown 交回认不出、不入账的副本。码或描述留在 Observed 上。
func (line ReceiptLine) AsUnknown() (ReceiptLine, error) {
	if line.quantity <= 0 {
		return ReceiptLine{}, ErrInvalidReceipt
	}
	line.goods = GoodsCode{}
	line.result = ReceiptLineUnknownGoods
	line.unposted = ReceiptUnpostedUnknownGoods
	return line, nil
}

// AsNotReceivable 交回预报已不能收货、这一行不入账的副本。
func (line ReceiptLine) AsNotReceivable() (ReceiptLine, error) {
	if line.quantity <= 0 {
		return ReceiptLine{}, ErrInvalidReceipt
	}
	line.goods = GoodsCode{}
	line.result = ReceiptLineResultNone
	line.unposted = ReceiptUnpostedNotReceivable
	return line, nil
}

func (line ReceiptLine) sameInput(other ReceiptLine) bool {
	return line.observed == other.observed &&
		line.unit == other.unit &&
		line.state == other.state &&
		line.quantity == other.quantity &&
		line.batch == other.batch &&
		line.expiry == other.expiry &&
		line.alias == other.alias &&
		line.location == other.location
}

// CartonReceiptLine 是整箱收货的一行。Number 是货主的箱号。WarehouseNumber 是仓另编的箱号，零值就是没带。
// 结果与未入账原因在落册时才有，提交比内容时不看它们。箱单不在这一行上，它留在预报上。
type CartonReceiptLine struct {
	number          CartonNumber
	warehouseNumber CartonNumber
	damaged         bool
	location        LocationCode
	result          ReceiptLineResult
	unposted        ReceiptUnpostedReason
}

func NewCartonReceiptLine(number, warehouseNumber CartonNumber, damaged bool, location LocationCode) (CartonReceiptLine, error) {
	if !number.valid() || !location.valid() {
		return CartonReceiptLine{}, ErrInvalidReceipt
	}
	return CartonReceiptLine{number: number, warehouseNumber: warehouseNumber, damaged: damaged, location: location}, nil
}

func (line CartonReceiptLine) Number() CartonNumber          { return line.number }
func (line CartonReceiptLine) WarehouseNumber() CartonNumber { return line.warehouseNumber }
func (line CartonReceiptLine) Damaged() bool                 { return line.damaged }
func (line CartonReceiptLine) Location() LocationCode        { return line.location }
func (line CartonReceiptLine) Result() ReceiptLineResult     { return line.result }
func (line CartonReceiptLine) Unposted() ReceiptUnpostedReason {
	return line.unposted
}

// BookedNumber 是这一行若入账会用的箱号。仓另编了箱号，并且货主箱号已经在库时，用仓另编的；否则用货主的。
// 货主箱号还没在库时，即使载荷带了仓另编的箱号也不拿来入账：那一格只在箱号与在库的重复时用。
func (line CartonReceiptLine) BookedNumber(ownerOnHand bool) CartonNumber {
	if ownerOnHand && line.warehouseNumber.valid() && line.warehouseNumber != line.number {
		return line.warehouseNumber
	}
	return line.number
}

// OwnerCartonOnBook 是入账后要记下的货主原箱号。只有真的按仓另编的箱号入账时才有。
func (line CartonReceiptLine) OwnerCartonOnBook(ownerOnHand bool) CartonNumber {
	if line.BookedNumber(ownerOnHand) == line.number {
		return CartonNumber{}
	}
	return line.number
}

func (line CartonReceiptLine) AsPosted() (CartonReceiptLine, error) {
	if !line.number.valid() || !line.location.valid() {
		return CartonReceiptLine{}, ErrInvalidReceipt
	}
	line.result = ReceiptLinePosted
	line.unposted = ReceiptUnpostedNone
	return line, nil
}

func (line CartonReceiptLine) AsWrongCarton() (CartonReceiptLine, error) {
	if !line.number.valid() {
		return CartonReceiptLine{}, ErrInvalidReceipt
	}
	line.result = ReceiptLineResultNone
	line.unposted = ReceiptUnpostedWrongCarton
	return line, nil
}

func (line CartonReceiptLine) AsCartonNumberInUse() (CartonReceiptLine, error) {
	if !line.number.valid() {
		return CartonReceiptLine{}, ErrInvalidReceipt
	}
	line.result = ReceiptLineResultNone
	line.unposted = ReceiptUnpostedCartonNumberInUse
	return line, nil
}

func (line CartonReceiptLine) AsNotReceivable() (CartonReceiptLine, error) {
	if !line.number.valid() {
		return CartonReceiptLine{}, ErrInvalidReceipt
	}
	line.result = ReceiptLineResultNone
	line.unposted = ReceiptUnpostedNotReceivable
	return line, nil
}

func (line CartonReceiptLine) sameInput(other CartonReceiptLine) bool {
	return line.number == other.number &&
		line.warehouseNumber == other.warehouseNumber &&
		line.damaged == other.damaged &&
		line.location == other.location
}

// ReceiptDiscrepancyLine 是一份收货差异里的一行：类型与数量。结论不在这一段，这里没有结论格。
// 认不出的错货把扫到的码或描述留在 Observed；认得出的带商品。
type ReceiptDiscrepancyLine struct {
	kind     ReceiptDiscrepancyKind
	quantity int64
	goods    GoodsCode
	observed string
	carton   CartonNumber
}

func NewReceiptDiscrepancyLine(kind ReceiptDiscrepancyKind, quantity int64, goods GoodsCode, observed string) (ReceiptDiscrepancyLine, error) {
	if kind.String() == "" || quantity <= 0 {
		return ReceiptDiscrepancyLine{}, ErrInvalidReceipt
	}
	switch kind {
	case ReceiptDiscrepancyOverage, ReceiptDiscrepancyDamage:
		if !goods.valid() || observed != "" {
			return ReceiptDiscrepancyLine{}, ErrInvalidReceipt
		}
	case ReceiptDiscrepancyWrongGoods:
		known := goods.valid()
		if known == (observed != "") || (known && !goods.valid()) {
			return ReceiptDiscrepancyLine{}, ErrInvalidReceipt
		}
		if !known {
			text, err := newRequiredValue("observed code", observed)
			if err != nil {
				return ReceiptDiscrepancyLine{}, ErrInvalidReceipt
			}
			observed = text.String()
		}
	default:
		return ReceiptDiscrepancyLine{}, ErrInvalidReceipt
	}
	return ReceiptDiscrepancyLine{kind: kind, quantity: quantity, goods: goods, observed: observed}, nil
}

// NewCartonDiscrepancyLine 记一整箱的错货或破损。数量是一箱，主体是箱号，不带商品。
func NewCartonDiscrepancyLine(kind ReceiptDiscrepancyKind, carton CartonNumber) (ReceiptDiscrepancyLine, error) {
	switch kind {
	case ReceiptDiscrepancyWrongGoods, ReceiptDiscrepancyDamage:
	default:
		return ReceiptDiscrepancyLine{}, ErrInvalidReceipt
	}
	if !carton.valid() {
		return ReceiptDiscrepancyLine{}, ErrInvalidReceipt
	}
	return ReceiptDiscrepancyLine{kind: kind, quantity: 1, carton: carton}, nil
}

func (line ReceiptDiscrepancyLine) Kind() ReceiptDiscrepancyKind { return line.kind }
func (line ReceiptDiscrepancyLine) Quantity() int64              { return line.quantity }
func (line ReceiptDiscrepancyLine) Goods() GoodsCode             { return line.goods }
func (line ReceiptDiscrepancyLine) Observed() string             { return line.observed }
func (line ReceiptDiscrepancyLine) Carton() CartonNumber         { return line.carton }

// ReceiptSubmission 是拿来比重放的提交内容。接收时刻、行结果与差异标识都不在里面：
// 那些是落册时才有的，同一份内容再投一次不该因为它们被判成内容冲突。
type ReceiptSubmission struct {
	Device           ReceiptDeviceID
	Operator         ReceiptOperatorID
	OccurredAt       time.Time
	Warehouse        WarehouseCode
	PreAdvice        PreAdviceID
	UnclaimedArrival UnclaimedArrivalID
	Shape            InboundMode
	Lines            []ReceiptLine
	CartonLines      []CartonReceiptLine
}

// RecordedReceipt 是已经落册的一条收货事实。
type RecordedReceipt struct {
	tenant           TenantID
	factID           ReceiptFactID
	device           ReceiptDeviceID
	operator         ReceiptOperatorID
	occurredAt       time.Time
	receivedAt       time.Time
	warehouse        WarehouseCode
	preAdvice        PreAdviceID
	unclaimedArrival UnclaimedArrivalID
	shape            InboundMode
	outcome          RecordedReceiptOutcome
	lines            []ReceiptLine
	cartonLines      []CartonReceiptLine
	discrepancy      ReceiptDiscrepancyID
	differences      []ReceiptDiscrepancyLine
}

// RecordedReceiptSpec 组装一条已落册的收货事实。差异标识的零值表示这一次没有形成差异；有标识就必须有行。
type RecordedReceiptSpec struct {
	Tenant           TenantID
	FactID           ReceiptFactID
	Device           ReceiptDeviceID
	Operator         ReceiptOperatorID
	OccurredAt       time.Time
	ReceivedAt       time.Time
	Warehouse        WarehouseCode
	PreAdvice        PreAdviceID
	UnclaimedArrival UnclaimedArrivalID
	Shape            InboundMode
	Outcome          RecordedReceiptOutcome
	Lines            []ReceiptLine
	CartonLines      []CartonReceiptLine
	Discrepancy      ReceiptDiscrepancyID
	Differences      []ReceiptDiscrepancyLine
}

func NewRecordedReceipt(spec RecordedReceiptSpec) (RecordedReceipt, error) {
	receipt := RecordedReceipt{
		tenant: spec.Tenant, factID: spec.FactID, device: spec.Device, operator: spec.Operator,
		occurredAt: normalizedInstant(spec.OccurredAt), receivedAt: normalizedInstant(spec.ReceivedAt),
		warehouse: spec.Warehouse, preAdvice: spec.PreAdvice, unclaimedArrival: spec.UnclaimedArrival,
		shape: spec.Shape, outcome: spec.Outcome,
		lines: slices.Clone(spec.Lines), cartonLines: slices.Clone(spec.CartonLines),
		discrepancy: spec.Discrepancy, differences: slices.Clone(spec.Differences),
	}
	if err := receipt.validate(); err != nil {
		return RecordedReceipt{}, err
	}
	return receipt, nil
}

func (receipt RecordedReceipt) validate() error {
	if !receipt.tenant.valid() || !receipt.factID.valid() || !receipt.device.valid() || !receipt.operator.valid() ||
		receipt.occurredAt.IsZero() || receipt.receivedAt.IsZero() || !receipt.warehouse.valid() || !receipt.preAdvice.valid() ||
		receipt.shape.String() == "" || receipt.outcome.String() == "" {
		return ErrInvalidReceipt
	}
	if receipt.outcome == RecordedReceiptDivertedToUnclaimed && receipt.unclaimedArrival.valid() {
		return ErrInvalidReceipt
	}
	hasDifference := receipt.discrepancy.valid()
	if hasDifference == (len(receipt.differences) == 0) {
		return ErrInvalidReceipt
	}
	if receipt.shape == InboundByCarton {
		return receipt.validateCarton(hasDifference)
	}
	if receipt.shape != InboundByGoods || len(receipt.lines) == 0 || len(receipt.cartonLines) != 0 {
		return ErrInvalidReceipt
	}
	for _, difference := range receipt.differences {
		if difference.carton.valid() {
			return ErrInvalidReceipt
		}
	}
	for _, line := range receipt.lines {
		if line.quantity <= 0 || line.observed == "" {
			return ErrInvalidReceipt
		}
		switch receipt.outcome {
		case RecordedReceiptConfirmed:
			switch line.result {
			case ReceiptLinePosted:
				if !line.goods.valid() || line.unposted != ReceiptUnpostedNone {
					return ErrInvalidReceipt
				}
			case ReceiptLineUnknownGoods:
				if line.goods.valid() || line.unposted != ReceiptUnpostedUnknownGoods {
					return ErrInvalidReceipt
				}
			default:
				return ErrInvalidReceipt
			}
		case RecordedReceiptDivertedToUnclaimed:
			if line.result != ReceiptLineResultNone || line.unposted != ReceiptUnpostedNotReceivable || line.goods.valid() || hasDifference {
				return ErrInvalidReceipt
			}
		default:
			return ErrInvalidReceipt
		}
	}
	return nil
}

func (receipt RecordedReceipt) validateCarton(hasDifference bool) error {
	if len(receipt.cartonLines) == 0 || len(receipt.lines) != 0 {
		return ErrInvalidReceipt
	}
	for _, line := range receipt.cartonLines {
		if !line.number.valid() || !line.location.valid() {
			return ErrInvalidReceipt
		}
		switch receipt.outcome {
		case RecordedReceiptConfirmed:
			switch line.unposted {
			case ReceiptUnpostedNone:
				if line.result != ReceiptLinePosted {
					return ErrInvalidReceipt
				}
			case ReceiptUnpostedWrongCarton, ReceiptUnpostedCartonNumberInUse:
				if line.result != ReceiptLineResultNone {
					return ErrInvalidReceipt
				}
			default:
				return ErrInvalidReceipt
			}
		case RecordedReceiptDivertedToUnclaimed:
			if line.result != ReceiptLineResultNone || line.unposted != ReceiptUnpostedNotReceivable || hasDifference {
				return ErrInvalidReceipt
			}
		default:
			return ErrInvalidReceipt
		}
	}
	for _, difference := range receipt.differences {
		if !difference.carton.valid() || difference.goods.valid() || difference.observed != "" {
			return ErrInvalidReceipt
		}
		if difference.kind != ReceiptDiscrepancyWrongGoods && difference.kind != ReceiptDiscrepancyDamage {
			return ErrInvalidReceipt
		}
	}
	return nil
}

func (receipt RecordedReceipt) Tenant() TenantID            { return receipt.tenant }
func (receipt RecordedReceipt) FactID() ReceiptFactID       { return receipt.factID }
func (receipt RecordedReceipt) Device() ReceiptDeviceID     { return receipt.device }
func (receipt RecordedReceipt) Operator() ReceiptOperatorID { return receipt.operator }
func (receipt RecordedReceipt) OccurredAt() time.Time       { return receipt.occurredAt }
func (receipt RecordedReceipt) ReceivedAt() time.Time       { return receipt.receivedAt }
func (receipt RecordedReceipt) Warehouse() WarehouseCode    { return receipt.warehouse }
func (receipt RecordedReceipt) PreAdvice() PreAdviceID      { return receipt.preAdvice }
func (receipt RecordedReceipt) UnclaimedArrival() (UnclaimedArrivalID, bool) {
	return receipt.unclaimedArrival, receipt.unclaimedArrival.valid()
}
func (receipt RecordedReceipt) Shape() InboundMode              { return receipt.shape }
func (receipt RecordedReceipt) Outcome() RecordedReceiptOutcome { return receipt.outcome }
func (receipt RecordedReceipt) Lines() []ReceiptLine            { return slices.Clone(receipt.lines) }
func (receipt RecordedReceipt) CartonLines() []CartonReceiptLine {
	return slices.Clone(receipt.cartonLines)
}
func (receipt RecordedReceipt) Discrepancy() (ReceiptDiscrepancyID, bool) {
	return receipt.discrepancy, receipt.discrepancy.valid()
}
func (receipt RecordedReceipt) Differences() []ReceiptDiscrepancyLine {
	return slices.Clone(receipt.differences)
}

// SameContent 按提交内容比，不比接收时刻、行结果和差异标识。时刻收到同一精度：同一微秒里的纳秒差存不下来。
func (receipt RecordedReceipt) SameContent(submission ReceiptSubmission) bool {
	if receipt.device != submission.Device || receipt.operator != submission.Operator ||
		receipt.warehouse != submission.Warehouse || receipt.preAdvice != submission.PreAdvice ||
		receipt.unclaimedArrival != submission.UnclaimedArrival || receipt.shape != submission.Shape {
		return false
	}
	if !receipt.occurredAt.Equal(normalizedInstant(submission.OccurredAt)) {
		return false
	}
	if receipt.shape == InboundByCarton {
		if len(submission.Lines) != 0 || len(receipt.cartonLines) != len(submission.CartonLines) {
			return false
		}
		for index := range receipt.cartonLines {
			if !receipt.cartonLines[index].sameInput(submission.CartonLines[index]) {
				return false
			}
		}
		return true
	}
	if len(submission.CartonLines) != 0 || len(receipt.lines) != len(submission.Lines) {
		return false
	}
	for index := range receipt.lines {
		if !receipt.lines[index].sameInput(submission.Lines[index]) {
			return false
		}
	}
	return true
}

// BaseQuantity 把一行的数量按包装层级的折合数收成基本单位。折合数不是正整数，或乘出来放不进 int64，答不出。
func BaseQuantity(quantity, factor int64) (int64, bool) {
	if quantity <= 0 || factor <= 0 {
		return 0, false
	}
	return scaleQuantity(quantity, factor, false)
}

// OverageQuantity 是这一次入账让累计实收跨过预报数量的那一段。
// 已经入账的数量不改：先前已经超过的部分不记到这一次上，这一次自己跨过去的才记。
func OverageQuantity(advised, prior, this int64) (int64, bool) {
	if advised < 0 || prior < 0 || this < 0 {
		return 0, false
	}
	total, ok := AddQuantity(prior, this)
	if !ok || total <= advised {
		return 0, ok
	}
	crossed, ok := AddQuantity(total, -advised)
	if !ok {
		return 0, false
	}
	if crossed > this {
		return this, true
	}
	return crossed, true
}

// ErrInvalidReceipt 是一条收货事实或它的差异凑不成自洽的一份。
var ErrInvalidReceipt = errInvalidReceipt{}

type errInvalidReceipt struct{}

func (errInvalidReceipt) Error() string { return "warehouse fulfillment: invalid receipt" }
