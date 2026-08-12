package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidBillClaim     = errors.New("settlement accounting: invalid supplier bill claim")
	ErrInvalidBillLineMatch = errors.New("settlement accounting: invalid bill line match")
	// ErrCrossCurrencyMatch：账单币种与预期成本结算币不同且无换算依据时保持待判断
	// （AT-SA-096）——不用当前汇率猜测，域内直接立不成匹配。
	ErrCrossCurrencyMatch = errors.New("settlement accounting: cross-currency match needs a conversion basis")
	// ErrNotAuditable：只有「已匹配」的无争议金额可通过审核形成应付；量差、价差、无匹配
	// 发生项与重复计费都要先走争议/复核，不得自动接受（AT-SA-086）。
	ErrNotAuditable          = errors.New("settlement accounting: only a matched line can form an audited payable")
	ErrInvalidAuditedPayable = errors.New("settlement accounting: invalid audited payable")
	ErrInvalidCreditNote     = errors.New("settlement accounting: invalid supplier credit note")
)

// SupplierPartyReference 指名提交主张的供应商。
type SupplierPartyReference struct{ requiredValue }

func NewSupplierPartyReference(value string) (SupplierPartyReference, error) {
	required, err := newRequiredValue("supplier party reference", value)
	return SupplierPartyReference{required}, err
}

// BillingPeriodReference 指名业务账期。
type BillingPeriodReference struct{ requiredValue }

func NewBillingPeriodReference(value string) (BillingPeriodReference, error) {
	required, err := newRequiredValue("billing period reference", value)
	return BillingPeriodReference{required}, err
}

// BillClaimID 是账单主张的稳定身份。
type BillClaimID struct{ requiredValue }

func NewBillClaimID(value string) (BillClaimID, error) {
	required, err := newRequiredValue("bill claim ID", value)
	return BillClaimID{required}, err
}

// BillClaimVersion 是主张的版本：同一身份内容变化形成版本冲突，不按最后到达覆盖。
type BillClaimVersion struct{ requiredValue }

func NewBillClaimVersion(value string) (BillClaimVersion, error) {
	required, err := newRequiredValue("bill claim version", value)
	return BillClaimVersion{required}, err
}

// BillLineReference 指名主张内的一行。匹配、差异与审核都按行与金额范围进行，整单
// 结论吞并不了成员差异。
type BillLineReference struct{ requiredValue }

func NewBillLineReference(value string) (BillLineReference, error) {
	required, err := newRequiredValue("bill line reference", value)
	return BillLineReference{required}, err
}

// BillLine 是主张的一行：费用项目、计费范围与主张金额。
type BillLine struct {
	Line         BillLineReference
	FeeItem      FeeItemReference
	ClaimedMinor int64
}

// SupplierBillClaimSpec 是接收一份供应商账单主张所需的全部输入。SupplementsClaim
// 只在追加主张/迟到账单时给出，指向被追加的原主张。
type SupplierBillClaimSpec struct {
	Claim            BillClaimID
	Version          BillClaimVersion
	Supplier         SupplierPartyReference
	LegalEntity      LegalEntityReference
	Period           BillingPeriodReference
	Currency         CurrencyCode
	Lines            []BillLine
	ReceivedAt       time.Time
	SupplementsClaim BillClaimID
}

// SupplierBillClaim 是供应商提交的收费主张（UC-SA-004「主张已接收」）。到达不表示
// 匹配或应付——类型上没有任何审核、应付或付款字段，也没有从主张直接铸出应付的方法；
// 应付只能经匹配与审核另行形成（AT-SA-079/100）。原始主张不可覆盖：更正与追加走新
// 主张回指原主张。
type SupplierBillClaim struct {
	claim       BillClaimID
	version     BillClaimVersion
	supplier    SupplierPartyReference
	legalEntity LegalEntityReference
	period      BillingPeriodReference
	currency    CurrencyCode
	lines       []BillLine
	receivedAt  time.Time
	supplements BillClaimID
}

func ReceiveSupplierBillClaim(spec SupplierBillClaimSpec) (SupplierBillClaim, error) {
	if !spec.Claim.valid() ||
		!spec.Version.valid() ||
		!spec.Supplier.valid() ||
		!spec.LegalEntity.valid() ||
		!spec.Period.valid() ||
		!spec.Currency.valid() ||
		len(spec.Lines) == 0 ||
		spec.ReceivedAt.IsZero() {
		return SupplierBillClaim{}, ErrInvalidBillClaim
	}
	if spec.SupplementsClaim.valid() && spec.SupplementsClaim == spec.Claim {
		// 追加主张指向自己就是把原主张重开——原始主张不可覆盖。
		return SupplierBillClaim{}, ErrInvalidBillClaim
	}
	seen := make(map[BillLineReference]struct{}, len(spec.Lines))
	for _, line := range spec.Lines {
		if !line.Line.valid() || !line.FeeItem.valid() || line.ClaimedMinor <= 0 {
			return SupplierBillClaim{}, ErrInvalidBillClaim
		}
		if _, exists := seen[line.Line]; exists {
			return SupplierBillClaim{}, ErrInvalidBillClaim
		}
		seen[line.Line] = struct{}{}
	}
	return SupplierBillClaim{
		claim:       spec.Claim,
		version:     spec.Version,
		supplier:    spec.Supplier,
		legalEntity: spec.LegalEntity,
		period:      spec.Period,
		currency:    spec.Currency,
		lines:       append([]BillLine(nil), spec.Lines...),
		receivedAt:  spec.ReceivedAt.UTC(),
		supplements: spec.SupplementsClaim,
	}, nil
}

func (claim SupplierBillClaim) Claim() BillClaimID {
	return claim.claim
}

func (claim SupplierBillClaim) Version() BillClaimVersion {
	return claim.version
}

func (claim SupplierBillClaim) Supplier() SupplierPartyReference {
	return claim.supplier
}

func (claim SupplierBillClaim) LegalEntity() LegalEntityReference {
	return claim.legalEntity
}

func (claim SupplierBillClaim) Period() BillingPeriodReference {
	return claim.period
}

func (claim SupplierBillClaim) Currency() CurrencyCode {
	return claim.currency
}

func (claim SupplierBillClaim) Lines() []BillLine {
	return append([]BillLine(nil), claim.lines...)
}

func (claim SupplierBillClaim) LineFor(reference BillLineReference) (BillLine, bool) {
	for _, line := range claim.lines {
		if line.Line == reference {
			return line, true
		}
	}
	return BillLine{}, false
}

func (claim SupplierBillClaim) TotalClaimedMinor() int64 {
	total := int64(0)
	for _, line := range claim.lines {
		total += line.ClaimedMinor
	}
	return total
}

func (claim SupplierBillClaim) ReceivedAt() time.Time {
	return claim.receivedAt
}

// Supplements 交回被追加的原主张（若本主张为追加/迟到主张）。
func (claim SupplierBillClaim) Supplements() (BillClaimID, bool) {
	if !claim.supplements.valid() {
		return BillClaimID{}, false
	}
	return claim.supplements, true
}

// MatchClassification 是逐行匹配走向的封闭五值：已匹配一格可进审核；量差、价差、无
// 匹配发生项与重复计费是四种差异，各自引不同依据，不得自动接受或拒绝（UC-SA-004
// 结果契约「差异/争议」行）。
type MatchClassification uint8

const (
	MatchClassificationInvalid MatchClassification = iota
	LineMatched
	QuantityVariance
	PriceVariance
	NoMatchingOccurrence
	DuplicateBilling
)

func (classification MatchClassification) valid() bool {
	return classification >= LineMatched && classification <= DuplicateBilling
}

func (classification MatchClassification) String() string {
	switch classification {
	case LineMatched:
		return "MATCHED"
	case QuantityVariance:
		return "QUANTITY_VARIANCE"
	case PriceVariance:
		return "PRICE_VARIANCE"
	case NoMatchingOccurrence:
		return "NO_MATCHING_OCCURRENCE"
	case DuplicateBilling:
		return "DUPLICATE_BILLING"
	default:
		return ""
	}
}

// MatchBasisReference 指名差异分类的依据：量差引计费重量/数量证据，价差引协议价格
// 依据，无匹配发生项引检索范围，重复计费引已计费的原主张/匹配。
type MatchBasisReference struct{ requiredValue }

func NewMatchBasisReference(value string) (MatchBasisReference, error) {
	required, err := newRequiredValue("match basis reference", value)
	return MatchBasisReference{required}, err
}

// BillLineMatch 是账单行与内部预期成本的金额范围关系（UC-SA-004 步骤 4）。差异分类
// 由适用规则裁决后作为输入进入，构造器守每格的完备性；金额与币种从行与预期成本取，
// 不允许旁路第二套数字。
type BillLineMatch struct {
	claim          BillClaimID
	line           BillLineReference
	classification MatchClassification
	expected       SupplierCostVersionID
	claimedMinor   int64
	expectedMinor  int64
	currency       CurrencyCode
	basis          MatchBasisReference
	matchedAt      time.Time
}

// MatchBillLine 逐格校验五值各自的完备性：
//   - `已匹配`要求预期成本在场、币种一致且金额相等，不携带差异依据；
//   - `量差`/`价差`要求预期成本在场、币种一致、金额不等且带差异依据；
//   - `无匹配发生项`要求预期成本**缺席**（没有可指的东西）且带检索依据；
//   - `重复计费`要求预期成本在场并以依据指向已计费的原主张/匹配。
//
// 币种不一致且无换算依据 → ErrCrossCurrencyMatch，保持待判断（AT-SA-096）。
func MatchBillLine(
	claim SupplierBillClaim,
	line BillLineReference,
	classification MatchClassification,
	expected SupplierExpectedCost,
	basis MatchBasisReference,
	matchedAt time.Time,
) (BillLineMatch, error) {
	if !claim.claim.valid() || !classification.valid() || matchedAt.IsZero() {
		return BillLineMatch{}, ErrInvalidBillLineMatch
	}
	claimedLine, found := claim.LineFor(line)
	if !found {
		return BillLineMatch{}, ErrInvalidBillLineMatch
	}
	expectedPresent := expected.Version().valid()
	expectedMinor := int64(0)
	if expectedPresent {
		settlementCurrency, settlementMinor := expected.SettlementAmount()
		if settlementCurrency != claim.currency {
			return BillLineMatch{}, ErrCrossCurrencyMatch
		}
		expectedMinor = settlementMinor
	}

	switch classification {
	case LineMatched:
		if !expectedPresent || claimedLine.ClaimedMinor != expectedMinor {
			return BillLineMatch{}, ErrInvalidBillLineMatch
		}
		if basis.valid() {
			// 已匹配的依据就是预期成本本身；这里塞「差异依据」只会与差异家族混格。
			return BillLineMatch{}, ErrInvalidBillLineMatch
		}
	case QuantityVariance, PriceVariance:
		if !expectedPresent || claimedLine.ClaimedMinor == expectedMinor {
			return BillLineMatch{}, ErrInvalidBillLineMatch
		}
		if !basis.valid() {
			return BillLineMatch{}, ErrInvalidBillLineMatch
		}
	case NoMatchingOccurrence:
		if expectedPresent {
			return BillLineMatch{}, ErrInvalidBillLineMatch
		}
		if !basis.valid() {
			return BillLineMatch{}, ErrInvalidBillLineMatch
		}
	case DuplicateBilling:
		if !expectedPresent || !basis.valid() {
			return BillLineMatch{}, ErrInvalidBillLineMatch
		}
	}

	return BillLineMatch{
		claim:          claim.claim,
		line:           line,
		classification: classification,
		expected:       expected.Version(),
		claimedMinor:   claimedLine.ClaimedMinor,
		expectedMinor:  expectedMinor,
		currency:       claim.currency,
		basis:          basis,
		matchedAt:      matchedAt.UTC(),
	}, nil
}

func (match BillLineMatch) Claim() BillClaimID {
	return match.claim
}

func (match BillLineMatch) Line() BillLineReference {
	return match.line
}

func (match BillLineMatch) Classification() MatchClassification {
	return match.classification
}

// Expected 交回匹配所引的预期成本版本；无匹配发生项没有它。
func (match BillLineMatch) Expected() (SupplierCostVersionID, bool) {
	if !match.expected.valid() {
		return SupplierCostVersionID{}, false
	}
	return match.expected, true
}

func (match BillLineMatch) ClaimedMinor() int64 {
	return match.claimedMinor
}

func (match BillLineMatch) ExpectedMinor() int64 {
	return match.expectedMinor
}

// VarianceMinor 是主张相对预期的差额：正数为供应商多收（AT-SA-086 的超额进入争议，
// 不自动接受），负数为少收。
func (match BillLineMatch) VarianceMinor() int64 {
	return match.claimedMinor - match.expectedMinor
}

func (match BillLineMatch) Currency() CurrencyCode {
	return match.currency
}

// Basis 在差异四格交回依据；已匹配没有它。
func (match BillLineMatch) Basis() (MatchBasisReference, bool) {
	if !match.basis.valid() {
		return MatchBasisReference{}, false
	}
	return match.basis, true
}

func (match BillLineMatch) MatchedAt() time.Time {
	return match.matchedAt
}

// PayableID 是审核应付的稳定身份。
type PayableID struct{ requiredValue }

func NewPayableID(value string) (PayableID, error) {
	required, err := newRequiredValue("payable ID", value)
	return PayableID{required}, err
}

// AuditorReference 指名形成审核决定的授权责任方。无授权不得人工接受或拒绝。
type AuditorReference struct{ requiredValue }

func NewAuditorReference(value string) (AuditorReference, error) {
	required, err := newRequiredValue("auditor reference", value)
	return AuditorReference{required}, err
}

// AuditedPayable 是通过协议、履约和费用审核的明确金额范围（UC-SA-004「审核应付」）。
// 它不表示供应商已付款或已收款——类型上没有付款、收款或净额字段；供应商贷项也不
// 净入它：贷项是独立对象回指本应付，这里没有任何吸收贷项的方法。
type AuditedPayable struct {
	payable     PayableID
	claim       BillClaimID
	line        BillLineReference
	expected    SupplierCostVersionID
	legalEntity LegalEntityReference
	account     SettlementAccountID
	currency    CurrencyCode
	amountMinor int64
	auditor     AuditorReference
	auditedAt   time.Time
}

// FormAuditedPayable 只接受`已匹配`的行：到达不是应付，匹配也不是应付，审核通过才是
// （AT-SA-089）；量差、价差、无匹配与重复计费先走争议，不得在这里被顺手接受。金额与
// 币种从匹配取——审核改不了数字，只裁决要不要付。
func FormAuditedPayable(
	match BillLineMatch,
	payable PayableID,
	legalEntity LegalEntityReference,
	account SettlementAccountID,
	auditor AuditorReference,
	auditedAt time.Time,
) (AuditedPayable, error) {
	if !match.claim.valid() {
		return AuditedPayable{}, ErrInvalidAuditedPayable
	}
	if match.classification != LineMatched {
		return AuditedPayable{}, ErrNotAuditable
	}
	if !payable.valid() || !legalEntity.valid() || !account.valid() || !auditor.valid() ||
		auditedAt.IsZero() || auditedAt.Before(match.matchedAt) {
		return AuditedPayable{}, ErrInvalidAuditedPayable
	}
	return AuditedPayable{
		payable:     payable,
		claim:       match.claim,
		line:        match.line,
		expected:    match.expected,
		legalEntity: legalEntity,
		account:     account,
		currency:    match.currency,
		amountMinor: match.claimedMinor,
		auditor:     auditor,
		auditedAt:   auditedAt.UTC(),
	}, nil
}

func (payable AuditedPayable) Payable() PayableID {
	return payable.payable
}

func (payable AuditedPayable) Claim() BillClaimID {
	return payable.claim
}

func (payable AuditedPayable) Line() BillLineReference {
	return payable.line
}

func (payable AuditedPayable) Expected() SupplierCostVersionID {
	return payable.expected
}

func (payable AuditedPayable) LegalEntity() LegalEntityReference {
	return payable.legalEntity
}

func (payable AuditedPayable) Account() SettlementAccountID {
	return payable.account
}

func (payable AuditedPayable) Amount() (CurrencyCode, int64) {
	return payable.currency, payable.amountMinor
}

func (payable AuditedPayable) Auditor() AuditorReference {
	return payable.auditor
}

func (payable AuditedPayable) AuditedAt() time.Time {
	return payable.auditedAt
}

// CreditNoteID 是供应商费用贷项的稳定身份。
type CreditNoteID struct{ requiredValue }

func NewCreditNoteID(value string) (CreditNoteID, error) {
	required, err := newRequiredValue("credit note ID", value)
	return CreditNoteID{required}, err
}

// CreditNoteVersion 是贷项的版本。
type CreditNoteVersion struct{ requiredValue }

func NewCreditNoteVersion(value string) (CreditNoteVersion, error) {
	required, err := newRequiredValue("credit note version", value)
	return CreditNoteVersion{required}, err
}

// CreditReasonReference 指名贷项的原因来源（供应商更正、协议纠错、争议裁决……）。
type CreditReasonReference struct{ requiredValue }

func NewCreditReasonReference(value string) (CreditReasonReference, error) {
	required, err := newRequiredValue("credit reason reference", value)
	return CreditReasonReference{required}, err
}

// SupplierCreditNoteSpec 是形成一份供应商费用贷项所需的全部输入。
type SupplierCreditNoteSpec struct {
	Note        CreditNoteID
	Version     CreditNoteVersion
	Payable     PayableID
	Claim       BillClaimID
	Account     SettlementAccountID
	Currency    CurrencyCode
	AmountMinor int64
	Reason      CreditReasonReference
	IssuedAt    time.Time
}

// SupplierCreditNote 是对既有主张/应付追加形成的贷记金额（AT-SA-090）。原主张与原
// 审核应付保留：贷项只回指它们，不改写原账单行、不静默净入原应付、也不另建净额应付
// ——本类型与 AuditedPayable 之间没有任何净额通道；预期成本的纠错归 UC-SA-002，这里
// 连预期成本的字段都没有。
type SupplierCreditNote struct {
	note        CreditNoteID
	version     CreditNoteVersion
	payable     PayableID
	claim       BillClaimID
	account     SettlementAccountID
	currency    CurrencyCode
	amountMinor int64
	reason      CreditReasonReference
	issuedAt    time.Time
}

func FormSupplierCreditNote(spec SupplierCreditNoteSpec) (SupplierCreditNote, error) {
	if !spec.Note.valid() ||
		!spec.Version.valid() ||
		!spec.Payable.valid() ||
		!spec.Claim.valid() ||
		!spec.Account.valid() ||
		!spec.Currency.valid() ||
		spec.AmountMinor <= 0 ||
		!spec.Reason.valid() ||
		spec.IssuedAt.IsZero() {
		return SupplierCreditNote{}, ErrInvalidCreditNote
	}
	return SupplierCreditNote{
		note:        spec.Note,
		version:     spec.Version,
		payable:     spec.Payable,
		claim:       spec.Claim,
		account:     spec.Account,
		currency:    spec.Currency,
		amountMinor: spec.AmountMinor,
		reason:      spec.Reason,
		issuedAt:    spec.IssuedAt.UTC(),
	}, nil
}

func (note SupplierCreditNote) Note() CreditNoteID {
	return note.note
}

func (note SupplierCreditNote) Version() CreditNoteVersion {
	return note.version
}

// Payable 交回被贷记的原审核应付——原应付关系是贷项的必备件，不是可选注记。
func (note SupplierCreditNote) Payable() PayableID {
	return note.payable
}

func (note SupplierCreditNote) Claim() BillClaimID {
	return note.claim
}

func (note SupplierCreditNote) Account() SettlementAccountID {
	return note.account
}

// Amount 是贷记金额，方向恒为贷（冲减应付）；它不产生一个新的净额应付。
func (note SupplierCreditNote) Amount() (CurrencyCode, int64) {
	return note.currency, note.amountMinor
}

func (note SupplierCreditNote) Reason() CreditReasonReference {
	return note.reason
}

func (note SupplierCreditNote) IssuedAt() time.Time {
	return note.issuedAt
}
