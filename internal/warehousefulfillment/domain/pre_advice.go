package domain

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

var (
	// ErrInvalidPreAdviceSubmission 是提交途径、来源、键与依据引用凑不成一个自洽的提交。
	ErrInvalidPreAdviceSubmission = errors.New("warehouse fulfillment: invalid pre-advice submission")
	// ErrInvalidPreAdvice 是一份预报缺了身份、明细形状、告知或单据号那一格对不上。
	ErrInvalidPreAdvice = errors.New("warehouse fulfillment: invalid pre-advice")
	// ErrPreAdviceStatus 是当前状态不许做这一步（撤回、收货缝、完结、告知更新）。
	ErrPreAdviceStatus = errors.New("warehouse fulfillment: pre-advice status does not allow this")
)

// ArrivalMode 是到仓方式，产品内置封闭三种（CONTEXT「入库预报」）。
type ArrivalMode uint8

const (
	ArrivalModeInvalid ArrivalMode = iota
	ArrivalFullContainer
	ArrivalPallet
	ArrivalLooseCarton
)

func (mode ArrivalMode) String() string {
	switch mode {
	case ArrivalFullContainer:
		return "FULL_CONTAINER"
	case ArrivalPallet:
		return "PALLET"
	case ArrivalLooseCarton:
		return "LOOSE_CARTON"
	default:
		return ""
	}
}

// NewArrivalMode 按字面认到仓方式，只认 `FULL_CONTAINER`、`PALLET` 与 `LOOSE_CARTON`。
func NewArrivalMode(word string) (ArrivalMode, error) {
	for _, mode := range []ArrivalMode{ArrivalFullContainer, ArrivalPallet, ArrivalLooseCarton} {
		if mode.String() == word {
			return mode, nil
		}
	}
	return ArrivalModeInvalid, fmt.Errorf("%w: arrival mode", ErrInvalidPreAdvice)
}

// InboundMode 是入库方式：商品入库或整箱入库。
type InboundMode uint8

const (
	InboundModeInvalid InboundMode = iota
	InboundByGoods
	InboundByCarton
)

func (mode InboundMode) String() string {
	switch mode {
	case InboundByGoods:
		return "BY_GOODS"
	case InboundByCarton:
		return "BY_CARTON"
	default:
		return ""
	}
}

// NewInboundMode 按字面认入库方式，只认 `BY_GOODS` 与 `BY_CARTON`。
func NewInboundMode(word string) (InboundMode, error) {
	for _, mode := range []InboundMode{InboundByGoods, InboundByCarton} {
		if mode.String() == word {
			return mode, nil
		}
	}
	return InboundModeInvalid, fmt.Errorf("%w: inbound mode", ErrInvalidPreAdvice)
}

// PreAdviceStatus 是入库预报的状态。
type PreAdviceStatus uint8

const (
	PreAdviceStatusInvalid PreAdviceStatus = iota
	PreAdviceAdvised
	PreAdviceReceiving
	PreAdviceClosed
	PreAdviceCancelled
)

func (status PreAdviceStatus) String() string {
	switch status {
	case PreAdviceAdvised:
		return "ADVISED"
	case PreAdviceReceiving:
		return "RECEIVING"
	case PreAdviceClosed:
		return "CLOSED"
	case PreAdviceCancelled:
		return "CANCELLED"
	default:
		return ""
	}
}

// NewPreAdviceStatus 按字面认状态，只认 `ADVISED`、`RECEIVING`、`CLOSED` 与 `CANCELLED`。
func NewPreAdviceStatus(word string) (PreAdviceStatus, error) {
	for _, status := range []PreAdviceStatus{PreAdviceAdvised, PreAdviceReceiving, PreAdviceClosed, PreAdviceCancelled} {
		if status.String() == word {
			return status, nil
		}
	}
	return PreAdviceStatusInvalid, fmt.Errorf("%w: status", ErrInvalidPreAdvice)
}

// NumberingAbsence 是无号原因。零值就是没有这一格：有单据号时必须是零值。
type NumberingAbsence uint8

const (
	NumberingAbsenceNone NumberingAbsence = iota
	NumberingTimeZoneNotConfigured
)

func (absence NumberingAbsence) String() string {
	switch absence {
	case NumberingTimeZoneNotConfigured:
		return "NUMBERING_TIME_ZONE_NOT_CONFIGURED"
	default:
		return ""
	}
}

// PreAdviceContentIssueKind 是明细一处不合的种类。
type PreAdviceContentIssueKind uint8

const (
	PreAdviceContentIssueKindInvalid PreAdviceContentIssueKind = iota
	PreAdviceIssueNoLines
	PreAdviceIssueGoodsNotRegistered
	PreAdviceIssueUnitNotPackagingLevel
	PreAdviceIssueQuantityNotPositive
	PreAdviceIssueBatchNotTracked
	PreAdviceIssueExpiryNotTracked
	PreAdviceIssueCartonCountNotPositive
	PreAdviceIssueDuplicateCartonNumber
	PreAdviceIssueEmptyCarton
)

func (kind PreAdviceContentIssueKind) String() string {
	switch kind {
	case PreAdviceIssueNoLines:
		return "NO_LINES"
	case PreAdviceIssueGoodsNotRegistered:
		return "GOODS_NOT_REGISTERED"
	case PreAdviceIssueUnitNotPackagingLevel:
		return "UNIT_NOT_A_PACKAGING_LEVEL"
	case PreAdviceIssueQuantityNotPositive:
		return "QUANTITY_NOT_POSITIVE"
	case PreAdviceIssueBatchNotTracked:
		return "BATCH_NOT_TRACKED"
	case PreAdviceIssueExpiryNotTracked:
		return "EXPIRY_NOT_TRACKED"
	case PreAdviceIssueCartonCountNotPositive:
		return "CARTON_COUNT_NOT_POSITIVE"
	case PreAdviceIssueDuplicateCartonNumber:
		return "DUPLICATE_CARTON_NUMBER"
	case PreAdviceIssueEmptyCarton:
		return "EMPTY_CARTON"
	default:
		return ""
	}
}

// PreAdviceContentIssue 是明细的一处不合。Line 是行下标，整份用 -1。
// Carton 只在箱单里的行上有意义，其余是 -1：箱单行的 Line 是箱单内的行下标，Carton 是箱的下标。
type PreAdviceContentIssue struct {
	line   int
	carton int
	kind   PreAdviceContentIssueKind
}

func wholePreAdviceIssue(kind PreAdviceContentIssueKind) PreAdviceContentIssue {
	return PreAdviceContentIssue{line: -1, carton: -1, kind: kind}
}

func linePreAdviceIssue(line int, kind PreAdviceContentIssueKind) PreAdviceContentIssue {
	return PreAdviceContentIssue{line: line, carton: -1, kind: kind}
}

func cartonLinePreAdviceIssue(carton, line int, kind PreAdviceContentIssueKind) PreAdviceContentIssue {
	return PreAdviceContentIssue{line: line, carton: carton, kind: kind}
}

func (issue PreAdviceContentIssue) Line() int { return issue.line }

func (issue PreAdviceContentIssue) Carton() int { return issue.carton }

func (issue PreAdviceContentIssue) Kind() PreAdviceContentIssueKind { return issue.kind }

func (issue PreAdviceContentIssue) String() string {
	return fmt.Sprintf("%d:%d:%s", issue.line, issue.carton, issue.kind)
}

// PreAdviceID 是预报标识，由本上下文签发。
type PreAdviceID struct{ requiredValue }

func NewPreAdviceID(value string) (PreAdviceID, error) {
	required, err := newRequiredValue("pre-advice ID", value)
	return PreAdviceID{required}, err
}

// DocumentNumber 是单据号，逐字保存。零值就是没有：无号时另记 NumberingAbsence。
type DocumentNumber struct{ requiredValue }

func NewDocumentNumber(value string) (DocumentNumber, error) {
	required, err := newRequiredValue("document number", value)
	return DocumentNumber{required}, err
}

// Assigned 回答这一格有没有单据号。
func (number DocumentNumber) Assigned() bool { return number.valid() }

// CartonNumber 是整箱入库的箱号，逐字保存，一份预报内不重复。
type CartonNumber struct{ requiredValue }

func NewCartonNumber(value string) (CartonNumber, error) {
	required, err := newRequiredValue("carton number", value)
	return CartonNumber{required}, err
}

// ArrivalClue 是到货线索，逐字保存。零值就是没带。
type ArrivalClue struct{ value string }

func NewArrivalClue(value string) (ArrivalClue, error) {
	required, err := newRequiredValue("arrival clue", value)
	if err != nil {
		return ArrivalClue{}, err
	}
	return ArrivalClue{value: required.String()}, nil
}

func (clue ArrivalClue) String() string { return clue.value }

func (clue ArrivalClue) Given() bool { return clue.value != "" }

// OwnerReference 是货主参考号，逐字保存。零值就是没带。
type OwnerReference struct{ value string }

func NewOwnerReference(value string) (OwnerReference, error) {
	required, err := newRequiredValue("owner reference", value)
	if err != nil {
		return OwnerReference{}, err
	}
	return OwnerReference{value: required.String()}, nil
}

func (reference OwnerReference) String() string { return reference.value }

func (reference OwnerReference) Given() bool { return reference.value != "" }

// CartonCount 是商品入库行上可缺的箱数，只作告知。零值就是没给；给了的数是否为正由内容校验判，不拦在构造器里。
type CartonCount struct {
	value int64
	given bool
}

func GivenCartonCount(value int64) CartonCount {
	return CartonCount{value: value, given: true}
}

func (count CartonCount) Given() bool { return count.given }

func (count CartonCount) Value() int64 { return count.value }

// PreAdviceSubmission 是一笔预报提交、告知更新或撤回的提交。货主口要有来源而不带依据引用，代录口要有依据引用而没有来源。
// 不与商品修订的提交共用一个类型：两处的判重位置各自独立，混用会让商品的键撤得了一份预报。
type PreAdviceSubmission struct {
	path     SubmissionPath
	source   SubmissionSource
	key      RequestKey
	evidence EvidenceReference
}

// NewOwnerPreAdviceSubmission 构造货主经客户渠道的提交。
func NewOwnerPreAdviceSubmission(source SubmissionSource, key RequestKey) (PreAdviceSubmission, error) {
	if !source.valid() || !key.valid() {
		return PreAdviceSubmission{}, ErrInvalidPreAdviceSubmission
	}
	return PreAdviceSubmission{path: SubmissionByOwner, source: source, key: key}, nil
}

// NewOnBehalfPreAdviceSubmission 构造运营代录的提交。
func NewOnBehalfPreAdviceSubmission(key RequestKey, evidence EvidenceReference) (PreAdviceSubmission, error) {
	if !key.valid() || !evidence.valid() {
		return PreAdviceSubmission{}, ErrInvalidPreAdviceSubmission
	}
	return PreAdviceSubmission{path: SubmissionOnBehalf, key: key, evidence: evidence}, nil
}

func (submission PreAdviceSubmission) Path() SubmissionPath { return submission.path }

func (submission PreAdviceSubmission) Source() (SubmissionSource, bool) {
	return submission.source, submission.source.valid()
}

func (submission PreAdviceSubmission) Key() RequestKey { return submission.key }

func (submission PreAdviceSubmission) EvidenceReference() (EvidenceReference, bool) {
	return submission.evidence, submission.evidence.valid()
}

func (submission PreAdviceSubmission) valid() bool {
	switch submission.path {
	case SubmissionByOwner:
		return submission.source.valid() && submission.key.valid() && !submission.evidence.valid()
	case SubmissionOnBehalf:
		return !submission.source.valid() && submission.key.valid() && submission.evidence.valid()
	default:
		return false
	}
}

// SameSlotAs 回答两个提交是不是占同一个判重位置：途径、来源与键都相同。依据引用不属于位置。
func (submission PreAdviceSubmission) SameSlotAs(other PreAdviceSubmission) bool {
	return submission.path == other.path && submission.source == other.source && submission.key == other.key
}

// GoodsInboundLine 是商品入库的一行。数量与箱数是否为正不拦在这里：内容不合要逐行收齐，提前拒就报不出后面的行。
type GoodsInboundLine struct {
	goods       GoodsCode
	unit        GoodsUnitName
	quantity    int64
	batch       BatchNumber
	expiry      ExpiryDate
	cartonCount CartonCount
}

func NewGoodsInboundLine(
	goods GoodsCode,
	unit GoodsUnitName,
	quantity int64,
	batch BatchNumber,
	expiry ExpiryDate,
	cartonCount CartonCount,
) (GoodsInboundLine, error) {
	if !goods.valid() || !unit.valid() {
		return GoodsInboundLine{}, ErrInvalidPreAdvice
	}
	return GoodsInboundLine{
		goods: goods, unit: unit, quantity: quantity, batch: batch, expiry: expiry, cartonCount: cartonCount,
	}, nil
}

func (line GoodsInboundLine) Goods() GoodsCode         { return line.goods }
func (line GoodsInboundLine) Unit() GoodsUnitName      { return line.unit }
func (line GoodsInboundLine) Quantity() int64          { return line.quantity }
func (line GoodsInboundLine) Batch() BatchNumber       { return line.batch }
func (line GoodsInboundLine) Expiry() ExpiryDate       { return line.expiry }
func (line GoodsInboundLine) CartonCount() CartonCount { return line.cartonCount }

func (line GoodsInboundLine) structuralIssues(index int) []PreAdviceContentIssue {
	var issues []PreAdviceContentIssue
	if line.quantity <= 0 {
		issues = append(issues, linePreAdviceIssue(index, PreAdviceIssueQuantityNotPositive))
	}
	if line.cartonCount.given && line.cartonCount.value <= 0 {
		issues = append(issues, linePreAdviceIssue(index, PreAdviceIssueCartonCountNotPositive))
	}
	return issues
}

// CartonContentLine 是箱单的一行：商品编码与按基本单位计的数量。
type CartonContentLine struct {
	goods    GoodsCode
	quantity int64
}

func NewCartonContentLine(goods GoodsCode, quantity int64) (CartonContentLine, error) {
	if !goods.valid() {
		return CartonContentLine{}, ErrInvalidPreAdvice
	}
	return CartonContentLine{goods: goods, quantity: quantity}, nil
}

func (line CartonContentLine) Goods() GoodsCode { return line.goods }
func (line CartonContentLine) Quantity() int64  { return line.quantity }

// Carton 是整箱入库的一箱。箱单可以是空的：空箱单是内容不合，要和别的不合一起指出，不在构造器里丢掉这一箱。
type Carton struct {
	number CartonNumber
	lines  []CartonContentLine
}

func NewCarton(number CartonNumber, lines []CartonContentLine) (Carton, error) {
	if !number.valid() {
		return Carton{}, ErrInvalidPreAdvice
	}
	return Carton{number: number, lines: slices.Clone(lines)}, nil
}

func (carton Carton) Number() CartonNumber { return carton.number }

func (carton Carton) Lines() []CartonContentLine { return slices.Clone(carton.lines) }

func (carton Carton) clone() Carton {
	return Carton{number: carton.number, lines: slices.Clone(carton.lines)}
}

// PreAdviceGoodsFacts 是核一行明细时用的当前修订：没登记就是零值。包装层级名是计量单位要比的那一列。
type PreAdviceGoodsFacts struct {
	registered  bool
	units       []GoodsUnitName
	trackBatch  bool
	trackExpiry bool
}

func RegisteredPreAdviceGoods(units []GoodsUnitName, trackBatch, trackExpiry bool) PreAdviceGoodsFacts {
	return PreAdviceGoodsFacts{
		registered: true, units: slices.Clone(units), trackBatch: trackBatch, trackExpiry: trackExpiry,
	}
}

func (facts PreAdviceGoodsFacts) declares(unit GoodsUnitName) bool {
	return slices.Contains(facts.units, unit)
}

// CollectGoodsInboundIssues 逐行收齐商品入库明细的不合之处。facts 的键是商品编码；缺这一键就是没登记。
// 同一个商品的事实由调用方只准备一次。次序按行，一行内先登记与计量单位，再是数量、箱数，然后是不跟踪却带了的批号与效期。
func CollectGoodsInboundIssues(lines []GoodsInboundLine, facts map[string]PreAdviceGoodsFacts) []PreAdviceContentIssue {
	if len(lines) == 0 {
		return []PreAdviceContentIssue{wholePreAdviceIssue(PreAdviceIssueNoLines)}
	}
	var issues []PreAdviceContentIssue
	for index, line := range lines {
		known, registered := facts[line.goods.String()]
		if !registered || !known.registered {
			issues = append(issues, linePreAdviceIssue(index, PreAdviceIssueGoodsNotRegistered))
		} else if !known.declares(line.unit) {
			issues = append(issues, linePreAdviceIssue(index, PreAdviceIssueUnitNotPackagingLevel))
		}
		issues = append(issues, line.structuralIssues(index)...)
		if registered && known.registered {
			if !known.trackBatch && line.batch.Given() {
				issues = append(issues, linePreAdviceIssue(index, PreAdviceIssueBatchNotTracked))
			}
			if !known.trackExpiry && line.expiry.Given() {
				issues = append(issues, linePreAdviceIssue(index, PreAdviceIssueExpiryNotTracked))
			}
		}
	}
	return issues
}

// CollectCartonIssues 逐箱收齐整箱入库的不合之处。箱号重复的每一箱都指出；空箱单指出那一箱；
// 箱单里的行另带箱下标。registered 回答商品编码在当前修订里在不在。
func CollectCartonIssues(cartons []Carton, registered func(GoodsCode) bool) []PreAdviceContentIssue {
	if len(cartons) == 0 {
		return []PreAdviceContentIssue{wholePreAdviceIssue(PreAdviceIssueNoLines)}
	}
	counts := make(map[string]int, len(cartons))
	for _, carton := range cartons {
		counts[carton.number.String()]++
	}
	var issues []PreAdviceContentIssue
	for index, carton := range cartons {
		if counts[carton.number.String()] > 1 {
			issues = append(issues, linePreAdviceIssue(index, PreAdviceIssueDuplicateCartonNumber))
		}
		if len(carton.lines) == 0 {
			issues = append(issues, linePreAdviceIssue(index, PreAdviceIssueEmptyCarton))
		}
		for lineIndex, line := range carton.lines {
			if registered == nil || !registered(line.goods) {
				issues = append(issues, cartonLinePreAdviceIssue(index, lineIndex, PreAdviceIssueGoodsNotRegistered))
			}
			if line.quantity <= 0 {
				issues = append(issues, cartonLinePreAdviceIssue(index, lineIndex, PreAdviceIssueQuantityNotPositive))
			}
		}
	}
	return issues
}

// PreAdviceNotice 是告知的一版：预计到仓时间、到货线索与货主参考号，另记这一版的提交。版本从 1 起。
type PreAdviceNotice struct {
	version    int
	expectedAt time.Time
	clue       ArrivalClue
	reference  OwnerReference
	submission PreAdviceSubmission
	registrant Registrant
	recordedAt time.Time
}

func NewPreAdviceNotice(
	version int,
	expectedAt time.Time,
	clue ArrivalClue,
	reference OwnerReference,
	submission PreAdviceSubmission,
	registrant Registrant,
	recordedAt time.Time,
) (PreAdviceNotice, error) {
	if version < 1 || expectedAt.IsZero() || !submission.valid() || !registrant.valid() || recordedAt.IsZero() {
		return PreAdviceNotice{}, ErrInvalidPreAdvice
	}
	return PreAdviceNotice{
		version:    version,
		expectedAt: normalizedInstant(expectedAt),
		clue:       clue,
		reference:  reference,
		submission: submission,
		registrant: registrant,
		recordedAt: normalizedInstant(recordedAt),
	}, nil
}

func (notice PreAdviceNotice) Version() int                    { return notice.version }
func (notice PreAdviceNotice) ExpectedAt() time.Time           { return notice.expectedAt }
func (notice PreAdviceNotice) Clue() ArrivalClue               { return notice.clue }
func (notice PreAdviceNotice) Reference() OwnerReference       { return notice.reference }
func (notice PreAdviceNotice) Submission() PreAdviceSubmission { return notice.submission }
func (notice PreAdviceNotice) Registrant() Registrant          { return notice.registrant }
func (notice PreAdviceNotice) RecordedAt() time.Time           { return notice.recordedAt }

// SameContentAs 回答这一版告知与这次提交是不是同一份内容。登记责任方与时刻不比。
func (notice PreAdviceNotice) SameContentAs(
	expectedAt time.Time,
	clue ArrivalClue,
	reference OwnerReference,
	submission PreAdviceSubmission,
) bool {
	return notice.expectedAt.Equal(normalizedInstant(expectedAt)) &&
		notice.clue == clue &&
		notice.reference == reference &&
		notice.submission == submission
}

// PreAdviceWithdrawal 是成立的一次撤回。
type PreAdviceWithdrawal struct {
	submission PreAdviceSubmission
	registrant Registrant
	at         time.Time
}

func NewPreAdviceWithdrawal(
	submission PreAdviceSubmission,
	registrant Registrant,
	at time.Time,
) (PreAdviceWithdrawal, error) {
	if !submission.valid() || !registrant.valid() || at.IsZero() {
		return PreAdviceWithdrawal{}, ErrInvalidPreAdvice
	}
	return PreAdviceWithdrawal{submission: submission, registrant: registrant, at: normalizedInstant(at)}, nil
}

func (withdrawal PreAdviceWithdrawal) Recorded() bool { return withdrawal.submission.valid() }

func (withdrawal PreAdviceWithdrawal) Submission() PreAdviceSubmission { return withdrawal.submission }
func (withdrawal PreAdviceWithdrawal) Registrant() Registrant          { return withdrawal.registrant }
func (withdrawal PreAdviceWithdrawal) At() time.Time                   { return withdrawal.at }

// PreAdvice 是一份入库预报。货主、仓、到仓方式、入库方式与明细在受理时固定。
// 告知是单独的版本链；现行的是最后一版。单据号没有时记下无号原因，两格互斥。
type PreAdvice struct {
	tenant     TenantID
	id         PreAdviceID
	owner      OwnerAccountID
	warehouse  WarehouseCode
	arrival    ArrivalMode
	inbound    InboundMode
	goodsLines []GoodsInboundLine
	cartons    []Carton
	status     PreAdviceStatus
	number     DocumentNumber
	absence    NumberingAbsence
	submission PreAdviceSubmission
	registrant Registrant
	acceptedAt time.Time
	notices    []PreAdviceNotice
	withdrawal PreAdviceWithdrawal
}

// NewAdvisedPreAdvice 落一份刚受理的预报：状态是已预报，告知只有第 1 版，且第 1 版的提交与受理提交是同一笔。
// 明细必须已经过内容校验：这里只拒形状不成立的，不再逐行收齐。
func NewAdvisedPreAdvice(
	tenant TenantID,
	id PreAdviceID,
	owner OwnerAccountID,
	warehouse WarehouseCode,
	arrival ArrivalMode,
	inbound InboundMode,
	goodsLines []GoodsInboundLine,
	cartons []Carton,
	number DocumentNumber,
	absence NumberingAbsence,
	submission PreAdviceSubmission,
	registrant Registrant,
	acceptedAt time.Time,
	notice PreAdviceNotice,
) (PreAdvice, error) {
	advice := PreAdvice{
		tenant: tenant, id: id, owner: owner, warehouse: warehouse,
		arrival: arrival, inbound: inbound,
		goodsLines: slices.Clone(goodsLines), cartons: cloneCartons(cartons),
		status: PreAdviceAdvised, number: number, absence: absence,
		submission: submission, registrant: registrant, acceptedAt: normalizedInstant(acceptedAt),
		notices: []PreAdviceNotice{notice},
	}
	if err := advice.validate(); err != nil {
		return PreAdvice{}, err
	}
	if notice.version != 1 || notice.submission != submission || notice.registrant != registrant || !notice.recordedAt.Equal(advice.acceptedAt) {
		return PreAdvice{}, ErrInvalidPreAdvice
	}
	return advice, nil
}

// RehydratePreAdvice 把已落册的一份装回来。形状、版本链与状态对不上是册坏了，不是业务答案。
func RehydratePreAdvice(
	tenant TenantID,
	id PreAdviceID,
	owner OwnerAccountID,
	warehouse WarehouseCode,
	arrival ArrivalMode,
	inbound InboundMode,
	goodsLines []GoodsInboundLine,
	cartons []Carton,
	status PreAdviceStatus,
	number DocumentNumber,
	absence NumberingAbsence,
	submission PreAdviceSubmission,
	registrant Registrant,
	acceptedAt time.Time,
	notices []PreAdviceNotice,
	withdrawal PreAdviceWithdrawal,
) (PreAdvice, error) {
	advice := PreAdvice{
		tenant: tenant, id: id, owner: owner, warehouse: warehouse,
		arrival: arrival, inbound: inbound,
		goodsLines: slices.Clone(goodsLines), cartons: cloneCartons(cartons),
		status: status, number: number, absence: absence,
		submission: submission, registrant: registrant, acceptedAt: normalizedInstant(acceptedAt),
		notices: slices.Clone(notices), withdrawal: withdrawal,
	}
	if err := advice.validate(); err != nil {
		return PreAdvice{}, err
	}
	return advice, nil
}

func (advice PreAdvice) validate() error {
	if !advice.tenant.valid() || !advice.id.valid() || !advice.owner.valid() || !advice.warehouse.valid() {
		return ErrInvalidPreAdvice
	}
	if advice.arrival.String() == "" || advice.inbound.String() == "" || advice.status.String() == "" {
		return ErrInvalidPreAdvice
	}
	if !advice.submission.valid() || !advice.registrant.valid() || advice.acceptedAt.IsZero() {
		return ErrInvalidPreAdvice
	}
	if err := numberShape(advice.number, advice.absence); err != nil {
		return err
	}
	if err := advice.linesFit(); err != nil {
		return err
	}
	if err := advice.noticesFit(); err != nil {
		return err
	}
	if advice.status == PreAdviceCancelled {
		if !advice.withdrawal.Recorded() {
			return ErrInvalidPreAdvice
		}
		return nil
	}
	if advice.withdrawal.Recorded() {
		return ErrInvalidPreAdvice
	}
	return nil
}

func numberShape(number DocumentNumber, absence NumberingAbsence) error {
	switch {
	case number.Assigned() && absence == NumberingAbsenceNone:
		return nil
	case !number.Assigned() && absence == NumberingTimeZoneNotConfigured:
		return nil
	default:
		return ErrInvalidPreAdvice
	}
}

func (advice PreAdvice) linesFit() error {
	switch advice.inbound {
	case InboundByGoods:
		if len(advice.cartons) != 0 || len(advice.goodsLines) == 0 {
			return ErrInvalidPreAdvice
		}
		for index, line := range advice.goodsLines {
			if len(line.structuralIssues(index)) > 0 {
				return ErrInvalidPreAdvice
			}
		}
	case InboundByCarton:
		if len(advice.goodsLines) != 0 || len(advice.cartons) == 0 {
			return ErrInvalidPreAdvice
		}
		seen := make(map[string]struct{}, len(advice.cartons))
		for _, carton := range advice.cartons {
			if _, taken := seen[carton.number.String()]; taken || len(carton.lines) == 0 {
				return ErrInvalidPreAdvice
			}
			seen[carton.number.String()] = struct{}{}
			for _, line := range carton.lines {
				if line.quantity <= 0 {
					return ErrInvalidPreAdvice
				}
			}
		}
	default:
		return ErrInvalidPreAdvice
	}
	return nil
}

func (advice PreAdvice) noticesFit() error {
	if len(advice.notices) == 0 {
		return ErrInvalidPreAdvice
	}
	for index, notice := range advice.notices {
		if notice.version != index+1 || notice.expectedAt.IsZero() || !notice.submission.valid() {
			return ErrInvalidPreAdvice
		}
	}
	return nil
}

func (advice PreAdvice) Tenant() TenantID                   { return advice.tenant }
func (advice PreAdvice) ID() PreAdviceID                    { return advice.id }
func (advice PreAdvice) Owner() OwnerAccountID              { return advice.owner }
func (advice PreAdvice) Warehouse() WarehouseCode           { return advice.warehouse }
func (advice PreAdvice) Arrival() ArrivalMode               { return advice.arrival }
func (advice PreAdvice) Inbound() InboundMode               { return advice.inbound }
func (advice PreAdvice) Status() PreAdviceStatus            { return advice.status }
func (advice PreAdvice) NumberingAbsence() NumberingAbsence { return advice.absence }
func (advice PreAdvice) Submission() PreAdviceSubmission    { return advice.submission }
func (advice PreAdvice) Registrant() Registrant             { return advice.registrant }
func (advice PreAdvice) AcceptedAt() time.Time              { return advice.acceptedAt }

func (advice PreAdvice) DocumentNumber() (DocumentNumber, bool) {
	return advice.number, advice.number.Assigned()
}

func (advice PreAdvice) GoodsLines() []GoodsInboundLine { return slices.Clone(advice.goodsLines) }

func (advice PreAdvice) Cartons() []Carton { return cloneCartons(advice.cartons) }

func (advice PreAdvice) Notices() []PreAdviceNotice { return slices.Clone(advice.notices) }

func (advice PreAdvice) CurrentNotice() PreAdviceNotice {
	return advice.notices[len(advice.notices)-1]
}

func (advice PreAdvice) Withdrawal() (PreAdviceWithdrawal, bool) {
	return advice.withdrawal, advice.withdrawal.Recorded()
}

// SameSubmissionAs 回答这次提交与受理时落下的那一份是不是同一内容。比的是第 1 版告知，不是现行版：
// 告知后来更新过，原样重投提交键仍是已有结果。登记责任方与受理时刻不比。
func (advice PreAdvice) SameSubmissionAs(
	owner OwnerAccountID,
	warehouse WarehouseCode,
	arrival ArrivalMode,
	inbound InboundMode,
	goodsLines []GoodsInboundLine,
	cartons []Carton,
	expectedAt time.Time,
	clue ArrivalClue,
	reference OwnerReference,
	submission PreAdviceSubmission,
) bool {
	if advice.owner != owner || advice.warehouse != warehouse || advice.arrival != arrival || advice.inbound != inbound {
		return false
	}
	if !slices.Equal(advice.goodsLines, goodsLines) || !cartonsEqual(advice.cartons, cartons) {
		return false
	}
	return advice.notices[0].SameContentAs(expectedAt, clue, reference, submission)
}

func cartonsEqual(left, right []Carton) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].number != right[index].number || !slices.Equal(left[index].lines, right[index].lines) {
			return false
		}
	}
	return true
}

func cloneCartons(cartons []Carton) []Carton {
	cloned := make([]Carton, len(cartons))
	for index, carton := range cartons {
		cloned[index] = carton.clone()
	}
	return cloned
}

func (advice PreAdvice) clone() PreAdvice {
	advice.goodsLines = slices.Clone(advice.goodsLines)
	advice.cartons = cloneCartons(advice.cartons)
	advice.notices = slices.Clone(advice.notices)
	return advice
}

// AppendNotice 追加一版告知。只在已预报或收货中成立，不动状态、明细与单据号。版本必须接在现有链的下一号。
func (advice PreAdvice) AppendNotice(notice PreAdviceNotice) (PreAdvice, error) {
	if advice.status != PreAdviceAdvised && advice.status != PreAdviceReceiving {
		return PreAdvice{}, ErrPreAdviceStatus
	}
	if notice.version != len(advice.notices)+1 {
		return PreAdvice{}, ErrInvalidPreAdvice
	}
	next := advice.clone()
	next.notices = append(next.notices, notice)
	return next, nil
}

// Cancel 在已预报时进入已取消。单据号留在这份预报上。
func (advice PreAdvice) Cancel(withdrawal PreAdviceWithdrawal) (PreAdvice, error) {
	if advice.status != PreAdviceAdvised {
		return PreAdvice{}, ErrPreAdviceStatus
	}
	if !withdrawal.Recorded() {
		return PreAdvice{}, ErrInvalidPreAdvice
	}
	next := advice.clone()
	next.status = PreAdviceCancelled
	next.withdrawal = withdrawal
	return next, nil
}

// StartReceiving 在已预报时进入收货中。
func (advice PreAdvice) StartReceiving() (PreAdvice, error) {
	if advice.status != PreAdviceAdvised {
		return PreAdvice{}, ErrPreAdviceStatus
	}
	next := advice.clone()
	next.status = PreAdviceReceiving
	return next, nil
}

// CloseOut 在已预报或收货中时进入已完结。
func (advice PreAdvice) CloseOut() (PreAdvice, error) {
	if advice.status != PreAdviceAdvised && advice.status != PreAdviceReceiving {
		return PreAdvice{}, ErrPreAdviceStatus
	}
	next := advice.clone()
	next.status = PreAdviceClosed
	return next, nil
}
