package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidStatementDraft = errors.New("settlement accounting: invalid statement draft")
	// ErrChargeNotConfirmed：只有达到确认条件的费用进入对账单；预估与暂估保持在外
	// （AT-SA-060）——纳入它们等于把未定的数发给客户。
	ErrChargeNotConfirmed = errors.New("settlement accounting: the charge has not reached confirmation")
	ErrInvalidStatement   = errors.New("settlement accounting: invalid customer statement")
	// ErrStatementImbalance：费用行金额与声明总额不勾稽时发布被阻断，不修正为「约等于」
	// （AT-SA-065）。
	ErrStatementImbalance = errors.New("settlement accounting: statement lines do not reconcile with the declared total")
	ErrStatementVoided    = errors.New("settlement accounting: the statement is already voided")
	ErrInvalidInclusion   = errors.New("settlement accounting: invalid subsequent inclusion")
	// ErrInclusionBackfillsPeriod：截单快照不可扩张——发布后到达的费用与调整归后续
	// 账期，回填原周期就是改写已发布快照。
	ErrInclusionBackfillsPeriod = errors.New("settlement accounting: an inclusion cannot backfill the original period")
	ErrInvalidDispute           = errors.New("settlement accounting: invalid statement dispute")
	ErrDisputeResolved          = errors.New("settlement accounting: the dispute is already resolved")
)

// StatementNumber 是对账单的固定单号：同一发布意图不得生成两个单号，作废后替代单
// 使用新身份。
type StatementNumber struct{ requiredValue }

func NewStatementNumber(value string) (StatementNumber, error) {
	required, err := newRequiredValue("statement number", value)
	return StatementNumber{required}, err
}

// StatementDraftVersion 是草稿的截止版本：发布前收到新确认费用可重算，每次重算换
// 版本保存新范围（AT-SA-063）。
type StatementDraftVersion struct{ requiredValue }

func NewStatementDraftVersion(value string) (StatementDraftVersion, error) {
	required, err := newRequiredValue("statement draft version", value)
	return StatementDraftVersion{required}, err
}

// StatementVoidBasisReference 指名整单作废的依据。
type StatementVoidBasisReference struct{ requiredValue }

func NewStatementVoidBasisReference(value string) (StatementVoidBasisReference, error) {
	required, err := newRequiredValue("statement void basis reference", value)
	return StatementVoidBasisReference{required}, err
}

// StatementDraftSpec 是形成一次截单草稿所需的全部输入。
type StatementDraftSpec struct {
	Account     SettlementAccountID
	Period      BillingPeriodReference
	Version     StatementDraftVersion
	Currency    CurrencyCode
	CutOffAt    time.Time
	Charges     []CustomerCharge
	Adjustments []ChargeAdjustment
}

// StatementDraft 是当前可重算的候选费用集合（UC-SA-003「草稿」）。草稿不是已发布
// 对账单也不是客户应付确认；它只纳入唯一创建用例已形成的费用与既有调整，本用例不
// 创建任何金额。
type StatementDraft struct {
	account     SettlementAccountID
	period      BillingPeriodReference
	version     StatementDraftVersion
	currency    CurrencyCode
	cutOffAt    time.Time
	charges     []CustomerCharge
	adjustments []ChargeAdjustment
}

// CutStatementDraft 形成截单草稿。只有已确认费用可入（AT-SA-059/060）；调整必须
// 挂在草稿内的费用上、币种一致——别的费用的调整进来就是把别人的数记到这个账户头上。
func CutStatementDraft(spec StatementDraftSpec) (StatementDraft, error) {
	if !spec.Account.valid() ||
		!spec.Period.valid() ||
		!spec.Version.valid() ||
		!spec.Currency.valid() ||
		spec.CutOffAt.IsZero() ||
		len(spec.Charges) == 0 {
		return StatementDraft{}, ErrInvalidStatementDraft
	}
	chargeIDs := make(map[CustomerChargeID]struct{}, len(spec.Charges))
	for _, charge := range spec.Charges {
		if !charge.ID().valid() {
			return StatementDraft{}, ErrInvalidStatementDraft
		}
		if charge.Stage() != ChargeConfirmed {
			return StatementDraft{}, ErrChargeNotConfirmed
		}
		// 对账单以合同结算币立单，入单比对的是费用的结算币一对。
		currency, _ := charge.SettlementAmount()
		if currency != spec.Currency {
			return StatementDraft{}, ErrInvalidStatementDraft
		}
		if _, exists := chargeIDs[charge.ID()]; exists {
			return StatementDraft{}, ErrInvalidStatementDraft
		}
		chargeIDs[charge.ID()] = struct{}{}
	}
	for _, adjustment := range spec.Adjustments {
		if !adjustment.ID().valid() {
			return StatementDraft{}, ErrInvalidStatementDraft
		}
		if _, covered := chargeIDs[adjustment.Charge()]; !covered {
			return StatementDraft{}, ErrInvalidStatementDraft
		}
		currency, _ := adjustment.Amount()
		if currency != spec.Currency {
			return StatementDraft{}, ErrInvalidStatementDraft
		}
	}
	return StatementDraft{
		account:     spec.Account,
		period:      spec.Period,
		version:     spec.Version,
		currency:    spec.Currency,
		cutOffAt:    spec.CutOffAt.UTC(),
		charges:     append([]CustomerCharge(nil), spec.Charges...),
		adjustments: append([]ChargeAdjustment(nil), spec.Adjustments...),
	}, nil
}

func (draft StatementDraft) Account() SettlementAccountID {
	return draft.account
}

func (draft StatementDraft) Period() BillingPeriodReference {
	return draft.period
}

func (draft StatementDraft) Version() StatementDraftVersion {
	return draft.version
}

func (draft StatementDraft) CutOffAt() time.Time {
	return draft.cutOffAt
}

func (draft StatementDraft) Charges() []CustomerCharge {
	return append([]CustomerCharge(nil), draft.charges...)
}

func (draft StatementDraft) Adjustments() []ChargeAdjustment {
	return append([]ChargeAdjustment(nil), draft.adjustments...)
}

// NetTotalMinor 是草稿净额：费用之和加借项减贷项。
func (draft StatementDraft) NetTotalMinor() int64 {
	total := int64(0)
	for _, charge := range draft.charges {
		_, amount := charge.SettlementAmount()
		total += amount
	}
	for _, adjustment := range draft.adjustments {
		_, amount := adjustment.Amount()
		if adjustment.Direction() == AdjustmentDebit {
			total += amount
		} else {
			total -= amount
		}
	}
	return total
}

// Recut 以新范围重算草稿（发布前收到新确认费用，AT-SA-063）：换版本、原草稿不动。
func (draft StatementDraft) Recut(
	charges []CustomerCharge,
	adjustments []ChargeAdjustment,
	version StatementDraftVersion,
	cutOffAt time.Time,
) (StatementDraft, error) {
	if !version.valid() || version == draft.version {
		return StatementDraft{}, ErrInvalidStatementDraft
	}
	return CutStatementDraft(StatementDraftSpec{
		Account:     draft.account,
		Period:      draft.period,
		Version:     version,
		Currency:    draft.currency,
		CutOffAt:    cutOffAt,
		Charges:     charges,
		Adjustments: adjustments,
	})
}

// StatementLine 是对账单内一条费用金额快照。
type StatementLine struct {
	Charge      CustomerChargeID
	AmountMinor int64
}

// StatementAdjustmentLine 是对账单内一条既有调整金额快照。
type StatementAdjustmentLine struct {
	Adjustment  ChargeAdjustmentID
	Charge      CustomerChargeID
	Direction   AdjustmentDirection
	AmountMinor int64
}

// PublishedStatement 是固定单号、费用范围和金额的不可覆盖快照（UC-SA-003「已发布
// 对账单」）。类型上没有任何增删行或改额的方法——发布后到达的费用与调整只能经
// SubsequentInclusion 进入**后续**账期；整单确实无效时作废留痕，替代单用新身份
// （AT-SA-069），这里没有替代方法。
type PublishedStatement struct {
	number      StatementNumber
	account     SettlementAccountID
	period      BillingPeriodReference
	currency    CurrencyCode
	lines       []StatementLine
	adjustments []StatementAdjustmentLine
	totalMinor  int64
	publishedAt time.Time
	voidBasis   StatementVoidBasisReference
	voidedAt    time.Time
}

// PublishStatement 从草稿发布快照。声明总额必须与行勾稽——不一致即阻断，不修正为
// 「约等于」（AT-SA-065）。
func PublishStatement(
	draft StatementDraft,
	number StatementNumber,
	declaredTotalMinor int64,
	publishedAt time.Time,
) (PublishedStatement, error) {
	if !draft.account.valid() || !number.valid() || publishedAt.IsZero() || publishedAt.Before(draft.cutOffAt) {
		return PublishedStatement{}, ErrInvalidStatement
	}
	if declaredTotalMinor != draft.NetTotalMinor() {
		return PublishedStatement{}, ErrStatementImbalance
	}
	statement := PublishedStatement{
		number:      number,
		account:     draft.account,
		period:      draft.period,
		currency:    draft.currency,
		totalMinor:  declaredTotalMinor,
		publishedAt: publishedAt.UTC(),
	}
	for _, charge := range draft.charges {
		_, amount := charge.SettlementAmount()
		statement.lines = append(statement.lines, StatementLine{Charge: charge.ID(), AmountMinor: amount})
	}
	for _, adjustment := range draft.adjustments {
		_, amount := adjustment.Amount()
		statement.adjustments = append(statement.adjustments, StatementAdjustmentLine{
			Adjustment:  adjustment.ID(),
			Charge:      adjustment.Charge(),
			Direction:   adjustment.Direction(),
			AmountMinor: amount,
		})
	}
	return statement, nil
}

func (statement PublishedStatement) Number() StatementNumber {
	return statement.number
}

func (statement PublishedStatement) Account() SettlementAccountID {
	return statement.account
}

func (statement PublishedStatement) Period() BillingPeriodReference {
	return statement.period
}

func (statement PublishedStatement) Currency() CurrencyCode {
	return statement.currency
}

func (statement PublishedStatement) Lines() []StatementLine {
	return append([]StatementLine(nil), statement.lines...)
}

func (statement PublishedStatement) AdjustmentLines() []StatementAdjustmentLine {
	return append([]StatementAdjustmentLine(nil), statement.adjustments...)
}

func (statement PublishedStatement) LineFor(charge CustomerChargeID) (StatementLine, bool) {
	for _, line := range statement.lines {
		if line.Charge == charge {
			return line, true
		}
	}
	return StatementLine{}, false
}

func (statement PublishedStatement) TotalMinor() int64 {
	return statement.totalMinor
}

func (statement PublishedStatement) PublishedAt() time.Time {
	return statement.publishedAt
}

// Voided 报告整单作废依据与时刻（若已作废）。作废留痕不删内容：行与总额仍可读，
// 历史派生不断。
func (statement PublishedStatement) Voided() (StatementVoidBasisReference, time.Time, bool) {
	if statement.voidedAt.IsZero() {
		return StatementVoidBasisReference{}, time.Time{}, false
	}
	return statement.voidBasis, statement.voidedAt, true
}

// Void 依据作废整单（AT-SA-069）：原单保留、依据必备、只作废一次；替代单用新身份，
// 本类型上没有替代方法。
func (statement PublishedStatement) Void(
	basis StatementVoidBasisReference,
	voidedAt time.Time,
) (PublishedStatement, error) {
	if !basis.valid() || voidedAt.IsZero() || voidedAt.Before(statement.publishedAt) {
		return PublishedStatement{}, ErrInvalidStatement
	}
	if _, _, voided := statement.Voided(); voided {
		return PublishedStatement{}, ErrStatementVoided
	}
	voided := statement
	voided.lines = append([]StatementLine(nil), statement.lines...)
	voided.adjustments = append([]StatementAdjustmentLine(nil), statement.adjustments...)
	voided.voidBasis = basis
	voided.voidedAt = voidedAt.UTC()
	return voided, nil
}

// InclusionReference 指名一条后续账期纳入关系。
type InclusionReference struct{ requiredValue }

func NewInclusionReference(value string) (InclusionReference, error) {
	required, err := newRequiredValue("inclusion reference", value)
	return InclusionReference{required}, err
}

// InclusionKind 是后续账期纳入的封闭二来源：既有调整或迟到的已确认费用。
type InclusionKind uint8

const (
	InclusionKindInvalid InclusionKind = iota
	IncludedAdjustment
	IncludedLateCharge
)

func (kind InclusionKind) valid() bool {
	return kind == IncludedAdjustment || kind == IncludedLateCharge
}

func (kind InclusionKind) String() string {
	switch kind {
	case IncludedAdjustment:
		return "ADJUSTMENT"
	case IncludedLateCharge:
		return "LATE_CHARGE"
	default:
		return ""
	}
}

// SubsequentInclusion 是发布后到达的既有调整或新确认费用与后续账期的纳入关系
// （UC-SA-003「后续账期纳入」）。它只持身份引用——类型上没有任何金额字段，金额永远
// 在唯一创建用例形成的费用/调整本体上，本用例不创建也不复述金额（AT-SA-076/077）。
type SubsequentInclusion struct {
	inclusion        InclusionReference
	kind             InclusionKind
	statement        StatementNumber
	originalPeriod   BillingPeriodReference
	subsequentPeriod BillingPeriodReference
	charge           CustomerChargeID
	adjustment       ChargeAdjustmentID
	includedAt       time.Time
}

// IncludeAdjustmentInSubsequentPeriod 把既有调整纳入后续账期并关联原账单（AT-SA-055
// 后半/AT-SA-076）。调整必须挂在原账单内的费用上；纳入原周期就是回填已发布快照，
// 构造期拒绝。
func IncludeAdjustmentInSubsequentPeriod(
	statement PublishedStatement,
	adjustment ChargeAdjustment,
	inclusion InclusionReference,
	subsequentPeriod BillingPeriodReference,
	includedAt time.Time,
) (SubsequentInclusion, error) {
	if !statement.number.valid() || !inclusion.valid() || !subsequentPeriod.valid() ||
		!adjustment.ID().valid() || includedAt.IsZero() {
		return SubsequentInclusion{}, ErrInvalidInclusion
	}
	if _, found := statement.LineFor(adjustment.Charge()); !found {
		return SubsequentInclusion{}, ErrInvalidInclusion
	}
	if subsequentPeriod == statement.period {
		return SubsequentInclusion{}, ErrInclusionBackfillsPeriod
	}
	return SubsequentInclusion{
		inclusion:        inclusion,
		kind:             IncludedAdjustment,
		statement:        statement.number,
		originalPeriod:   statement.period,
		subsequentPeriod: subsequentPeriod,
		charge:           adjustment.Charge(),
		adjustment:       adjustment.ID(),
		includedAt:       includedAt.UTC(),
	}, nil
}

// IncludeLateChargeInSubsequentPeriod 把发布后才确认的费用纳入后续账期并关联原周期
// （AT-SA-064）。费用必须已确认——迟到不豁免确认条件；原单不动。
func IncludeLateChargeInSubsequentPeriod(
	statement PublishedStatement,
	charge CustomerCharge,
	inclusion InclusionReference,
	subsequentPeriod BillingPeriodReference,
	includedAt time.Time,
) (SubsequentInclusion, error) {
	if !statement.number.valid() || !inclusion.valid() || !subsequentPeriod.valid() ||
		!charge.ID().valid() || includedAt.IsZero() {
		return SubsequentInclusion{}, ErrInvalidInclusion
	}
	if charge.Stage() != ChargeConfirmed {
		return SubsequentInclusion{}, ErrChargeNotConfirmed
	}
	if subsequentPeriod == statement.period {
		return SubsequentInclusion{}, ErrInclusionBackfillsPeriod
	}
	return SubsequentInclusion{
		inclusion:        inclusion,
		kind:             IncludedLateCharge,
		statement:        statement.number,
		originalPeriod:   statement.period,
		subsequentPeriod: subsequentPeriod,
		charge:           charge.ID(),
		includedAt:       includedAt.UTC(),
	}, nil
}

func (inclusion SubsequentInclusion) Inclusion() InclusionReference {
	return inclusion.inclusion
}

func (inclusion SubsequentInclusion) Kind() InclusionKind {
	return inclusion.kind
}

func (inclusion SubsequentInclusion) Statement() StatementNumber {
	return inclusion.statement
}

func (inclusion SubsequentInclusion) OriginalPeriod() BillingPeriodReference {
	return inclusion.originalPeriod
}

func (inclusion SubsequentInclusion) SubsequentPeriod() BillingPeriodReference {
	return inclusion.subsequentPeriod
}

func (inclusion SubsequentInclusion) Charge() CustomerChargeID {
	return inclusion.charge
}

// Adjustment 只在纳入既有调整时给出。
func (inclusion SubsequentInclusion) Adjustment() (ChargeAdjustmentID, bool) {
	if !inclusion.adjustment.valid() {
		return ChargeAdjustmentID{}, false
	}
	return inclusion.adjustment, true
}

func (inclusion SubsequentInclusion) IncludedAt() time.Time {
	return inclusion.includedAt
}

// DisputeID 是费用争议的标识。
type DisputeID struct{ requiredValue }

func NewDisputeID(value string) (DisputeID, error) {
	required, err := newRequiredValue("dispute ID", value)
	return DisputeID{required}, err
}

// DisputeBasisReference 指名争议的理由/证据，或处理决定的依据。
type DisputeBasisReference struct{ requiredValue }

func NewDisputeBasisReference(value string) (DisputeBasisReference, error) {
	required, err := newRequiredValue("dispute basis reference", value)
	return DisputeBasisReference{required}, err
}

// DisputeResolutionKind 是争议处理的封闭四走向。接受与部分接受只产生「请求金额所有者
// 复核」的依据——相应唯一创建用例返回有效调整前，本用例不生成贷项或借项。
type DisputeResolutionKind uint8

const (
	DisputeResolutionKindInvalid DisputeResolutionKind = iota
	DisputeAccepted
	DisputePartiallyAccepted
	DisputeRejected
	DisputePendingReview
)

func (kind DisputeResolutionKind) valid() bool {
	return kind >= DisputeAccepted && kind <= DisputePendingReview
}

func (kind DisputeResolutionKind) String() string {
	switch kind {
	case DisputeAccepted:
		return "ACCEPTED"
	case DisputePartiallyAccepted:
		return "PARTIALLY_ACCEPTED"
	case DisputeRejected:
		return "REJECTED"
	case DisputePendingReview:
		return "PENDING_REVIEW"
	default:
		return ""
	}
}

// StatementDispute 是针对已发布对账单内明确金额范围的独立异议对象（UC-SA-003
// 「费用争议」）。它只持有对账单与费用的引用——原费用和原对账单保持不变，争议锁定
// 的只是争议金额，未争议范围继续处理（AT-SA-070）。
type StatementDispute struct {
	dispute       DisputeID
	statement     StatementNumber
	charge        CustomerChargeID
	disputedMinor int64
	reason        DisputeBasisReference
	openedAt      time.Time
	resolution    DisputeResolutionKind
	resolutionRef DisputeBasisReference
	resolvedAt    time.Time
}

// OpenStatementDispute 开立争议。费用范围必须指明且在单内（整单笼统异议要求补充
// 明确范围，AT-SA-071）；争议金额不得超过该行金额。
func OpenStatementDispute(
	statement PublishedStatement,
	charge CustomerChargeID,
	disputedMinor int64,
	reason DisputeBasisReference,
	dispute DisputeID,
	openedAt time.Time,
) (StatementDispute, error) {
	if !statement.number.valid() || !dispute.valid() || !reason.valid() || openedAt.IsZero() {
		return StatementDispute{}, ErrInvalidDispute
	}
	line, found := statement.LineFor(charge)
	if !found {
		return StatementDispute{}, ErrInvalidDispute
	}
	if disputedMinor <= 0 || disputedMinor > line.AmountMinor {
		return StatementDispute{}, ErrInvalidDispute
	}
	return StatementDispute{
		dispute:       dispute,
		statement:     statement.number,
		charge:        charge,
		disputedMinor: disputedMinor,
		reason:        reason,
		openedAt:      openedAt.UTC(),
	}, nil
}

func (dispute StatementDispute) Dispute() DisputeID {
	return dispute.dispute
}

func (dispute StatementDispute) Statement() StatementNumber {
	return dispute.statement
}

func (dispute StatementDispute) Charge() CustomerChargeID {
	return dispute.charge
}

func (dispute StatementDispute) DisputedMinor() int64 {
	return dispute.disputedMinor
}

func (dispute StatementDispute) Reason() DisputeBasisReference {
	return dispute.reason
}

func (dispute StatementDispute) OpenedAt() time.Time {
	return dispute.openedAt
}

// Resolution 报告处理三件（走向、依据、时刻），未处理时不给出。
func (dispute StatementDispute) Resolution() (DisputeResolutionKind, DisputeBasisReference, time.Time, bool) {
	if dispute.resolvedAt.IsZero() {
		return DisputeResolutionKindInvalid, DisputeBasisReference{}, time.Time{}, false
	}
	return dispute.resolution, dispute.resolutionRef, dispute.resolvedAt, true
}

// Resolve 形成争议处理结果：接受、部分接受、拒绝或待复核，依据必备。待复核可再处理；
// 终局三格不可再改——原费用与原对账单自始至终不被本方法触碰，接受产生的只是复核
// 请求依据，调整由金额所有者另行形成（AT-SA-072/073）。
func (dispute StatementDispute) Resolve(
	kind DisputeResolutionKind,
	basis DisputeBasisReference,
	resolvedAt time.Time,
) (StatementDispute, error) {
	if !kind.valid() || !basis.valid() || resolvedAt.IsZero() || resolvedAt.Before(dispute.openedAt) {
		return StatementDispute{}, ErrInvalidDispute
	}
	if kind, _, _, resolved := dispute.Resolution(); resolved && kind != DisputePendingReview {
		return StatementDispute{}, ErrDisputeResolved
	}
	resolved := dispute
	resolved.resolution = kind
	resolved.resolutionRef = basis
	resolved.resolvedAt = resolvedAt.UTC()
	return resolved, nil
}
