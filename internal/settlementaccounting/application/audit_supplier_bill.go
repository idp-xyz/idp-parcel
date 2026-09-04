// audit_supplier_bill.go 编排 UC-SA-004 的审核半程：步 5 由授权审核责任方对`已匹配`行形成
// 审核应付，步 6 接收供应商贷项回指原应付，步 7 把两者的引用分别发布。它续在
// receive_supplier_bill.go 的接收半程之后，共用同一个处理器与依赖——接收记录是审核的唯一
// 起点，命令里没有金额：审核改不了数字，只裁决要不要付。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	// ErrUnexpectedPayableSave 说明应付库交回了封闭集合以外的写入结果。
	ErrUnexpectedPayableSave = errors.New("settlement accounting: unexpected audited payable save outcome")
	// ErrUnexpectedCreditNoteSave 说明贷项库交回了封闭集合以外的写入结果。
	ErrUnexpectedCreditNoteSave = errors.New("settlement accounting: unexpected supplier credit note save outcome")
)

// AuditOutcome 是一次逐行审核的应用处理结果。`行不可审`是专格（ADR-0029 按恢复动作分格）：
// 量差、价差、无匹配发生项与重复计费的恢复动作是走争议/复核，不是改单也不是等依赖
// （AT-SA-086/087）。
type AuditOutcome uint8

const (
	AuditOutcomeInvalid AuditOutcome = iota
	PayableFormed
	PayableExistingResult
	PayableConflict
	LineNotAuditable
	AuditNotAccepted
	AuditUndecided
)

func (outcome AuditOutcome) String() string {
	switch outcome {
	case PayableFormed:
		return "PAYABLE_FORMED"
	case PayableExistingResult:
		return "EXISTING_PAYABLE"
	case PayableConflict:
		return "PAYABLE_CONFLICT"
	case LineNotAuditable:
		return "LINE_NOT_AUDITABLE"
	case AuditNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case AuditUndecided:
		return "AUDIT_UNDECIDED"
	default:
		return ""
	}
}

// AuditUndecidedReason 指名审核停在哪一步等谁。两格未配置都是实例半边：审核授权
// （AT-SA-088「无授权不得人工接受或拒绝」）与供应商应付结算账户（`PAR-SET-01`）未登记时
// 不默认放行、不虚构授权人、不推导账户。
type AuditUndecidedReason uint8

const (
	AuditUndecidedReasonNone AuditUndecidedReason = iota
	AuditBillStoreUnavailable
	AuditAuthorityUnavailable
	AuditAuthorityUnconfigured
	PayableAccountUnavailable
	PayableAccountUnconfigured
	PayableStoreUnavailable
)

func (reason AuditUndecidedReason) String() string {
	switch reason {
	case AuditBillStoreUnavailable:
		return "BILL_STORE_UNAVAILABLE"
	case AuditAuthorityUnavailable:
		return "AUDIT_AUTHORITY_UNAVAILABLE"
	case AuditAuthorityUnconfigured:
		return "AUDIT_AUTHORITY_UNCONFIGURED"
	case PayableAccountUnavailable:
		return "PAYABLE_ACCOUNT_UNAVAILABLE"
	case PayableAccountUnconfigured:
		return "PAYABLE_ACCOUNT_UNCONFIGURED"
	case PayableStoreUnavailable:
		return "PAYABLE_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// AuditBillLineCommand 指名要审核哪份接收里的哪一行，以及本次应付的稳定身份。刻意没有
// 金额、账户与审核人：金额从匹配取，账户与审核人从已登记实例取——命令带得了它们，
// 「供应商不能直接写入审核结果」就只剩一句话。
type AuditBillLineCommand struct {
	TenantID domain.TenantID
	Claim    domain.BillClaimID
	Version  domain.BillClaimVersion
	Line     domain.BillLineReference
	Payable  domain.PayableID
}

type AuditResult struct {
	outcome      AuditOutcome
	reason       AuditUndecidedReason
	record       ports.AuditedPayableRecord
	hasRecord    bool
	continuation string
	handoff      string
}

func (result AuditResult) Outcome() AuditOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result AuditResult) UndecidedReason() AuditUndecidedReason {
	return result.reason
}

func (result AuditResult) Payable() (ports.AuditedPayableRecord, bool) {
	return result.record, result.hasRecord
}

func (result AuditResult) ContinuationReference() string {
	return result.continuation
}

// PayableHandoffReference 非空说明应付已提交但它自己的发布意图还没交出去，重放会重发同一份。
func (result AuditResult) PayableHandoffReference() string {
	return result.handoff
}

// Audit 对一份已接收主张的一行形成审核应付（UC-SA-004 步 5）：找回接收 → 找到那一行的
// 匹配 → 同身份重放/冲突、同一行已有应付读回先到者 → 审核授权与结算账户从已登记实例取
// （未配置停在未决）→ FormAuditedPayable（只收`已匹配`，其余四格 → 行不可审）→ 提交 →
// 发布应付引用。每次只审一行：批量含通过、争议与拒绝时逐范围返回，合法通过行不被局部
// 失败回滚（AT-SA-099）。
func (handler *ReceiveSupplierBillHandler) Audit(
	ctx context.Context,
	command AuditBillLineCommand,
) (AuditResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" ||
		command.Claim.String() == "" || command.Version.String() == "" ||
		command.Line.String() == "" || command.Payable.String() == "" {
		return AuditResult{outcome: AuditNotAccepted}, nil
	}

	payableKey := ports.AuditedPayableKey{TenantID: command.TenantID, Payable: command.Payable}
	digest := auditDigest(command)
	existing, found, err := handler.deps.Payables.FindByKey(ctx, payableKey)
	if err != nil {
		return auditUndecided(PayableStoreUnavailable, command.Payable.String()), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一应付身份指向另一行：请求冲突，原应付不被覆盖。
			return AuditResult{outcome: PayableConflict}, nil
		}
		return handler.existingPayable(ctx, existing), nil
	}
	audited, found, err := handler.deps.Payables.FindByLine(ctx, command.TenantID, command.Claim, command.Line)
	if err != nil {
		return auditUndecided(PayableStoreUnavailable, command.Payable.String()), nil
	}
	if found {
		// 这一行已经通过审核：同一业务变化只形成一次，换个应付身份再提交交回先到的那份。
		return handler.existingPayable(ctx, audited), nil
	}

	reception, found, err := handler.deps.Receptions.FindByKey(ctx, ports.BillReceptionKey{
		TenantID: command.TenantID, Claim: command.Claim, Version: command.Version,
	})
	if err != nil {
		return auditUndecided(AuditBillStoreUnavailable, command.Claim.String()), nil
	}
	if !found {
		// 指名了不存在的接收：提交矛盾，改单重来——审核只从已提交的匹配开始。
		return AuditResult{outcome: AuditNotAccepted}, nil
	}
	match, found := matchForLine(reception, command.Line)
	if !found {
		return AuditResult{outcome: AuditNotAccepted}, nil
	}

	auditor, configured, err := handler.deps.Authority.LoadSupplierAuditAuthority(
		ctx, command.TenantID, reception.Claim.Supplier(), reception.Claim.LegalEntity())
	if err != nil {
		return auditUndecided(AuditAuthorityUnavailable, command.Claim.String()), nil
	}
	if !configured {
		return auditUndecided(AuditAuthorityUnconfigured, command.Claim.String()), nil
	}
	account, registered, err := handler.deps.Accounts.LoadSupplierPayableAccount(
		ctx, command.TenantID, reception.Claim.Supplier(), reception.Claim.LegalEntity(), reception.Claim.Currency())
	if err != nil {
		return auditUndecided(PayableAccountUnavailable, command.Claim.String()), nil
	}
	if !registered {
		return auditUndecided(PayableAccountUnconfigured, command.Claim.String()), nil
	}

	payable, err := domain.FormAuditedPayable(
		match, command.Payable, reception.Claim.LegalEntity(), account, auditor, handler.deps.Clock.Now())
	if errors.Is(err, domain.ErrNotAuditable) {
		// 量差、价差、无匹配发生项与重复计费先走争议/复核，不在这里被顺手接受（AT-SA-086）。
		return AuditResult{outcome: LineNotAuditable,
			continuation: billContinuation("LINE_NOT_AUDITABLE", command.Claim.String(), command.Line.String())}, nil
	}
	if err != nil {
		return AuditResult{outcome: AuditNotAccepted}, nil
	}

	record := ports.AuditedPayableRecord{
		Key:           payableKey,
		ContentDigest: digest,
		Payable:       payable,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Payables.Save(ctx, record)
	if err != nil {
		return auditUndecided(PayableStoreUnavailable, command.Payable.String()), nil
	}
	switch saved {
	case ports.AuditedPayableSaved:
		result := AuditResult{outcome: PayableFormed, record: record, hasRecord: true}
		result.handoff = handler.handOffPayable(ctx, record)
		return result, nil
	case ports.AuditedPayableAlreadyRecorded:
		// 并发下另一方先审了这一行（或先用了这个身份）：按行读回赢家，不覆盖。
		winner, found, err := handler.deps.Payables.FindByLine(ctx, command.TenantID, command.Claim, command.Line)
		if err != nil || !found {
			return auditUndecided(PayableStoreUnavailable, command.Payable.String()), nil
		}
		return handler.existingPayable(ctx, winner), nil
	default:
		return AuditResult{}, fmt.Errorf("%w: %d", ErrUnexpectedPayableSave, saved)
	}
}

func matchForLine(record ports.BillReceptionRecord, line domain.BillLineReference) (domain.BillLineMatch, bool) {
	for _, match := range record.Matches {
		if match.Line() == line {
			return match, true
		}
	}
	return domain.BillLineMatch{}, false
}

func auditUndecided(reason AuditUndecidedReason, subject string) AuditResult {
	return AuditResult{
		outcome:      AuditUndecided,
		reason:       reason,
		continuation: billContinuation(reason.String(), subject),
	}
}

// existingPayable 按已有应付作答并重发同一份意图。
func (handler *ReceiveSupplierBillHandler) existingPayable(
	ctx context.Context,
	record ports.AuditedPayableRecord,
) AuditResult {
	return AuditResult{
		outcome:   PayableExistingResult,
		record:    record,
		hasRecord: true,
		handoff:   handler.handOffPayable(ctx, record),
	}
}

// handOffPayable 发布应付引用（UC-SA-004 步 7）。投递失败不翻结果，只恢复应付自己的同一
// 发布意图，不推定贷项那一半已发布（AT-SA-098）。
func (handler *ReceiveSupplierBillHandler) handOffPayable(
	ctx context.Context,
	record ports.AuditedPayableRecord,
) string {
	if err := handler.deps.Downstream.HandOffSupplierBill(ctx, ports.SupplierBillHandoffIntent{Payable: record}); err == nil {
		return ""
	}
	return billContinuation("AUDITED_PAYABLE_HANDOFF", record.Key.TenantID.String(), record.Key.Payable.String())
}

// auditDigest 是同一应付身份的内容比对锚：主张、版本与行任一不同即是另一份内容。
func auditDigest(command AuditBillLineCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Claim.String(),
		command.Version.String(),
		command.Line.String(),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

// CreditNoteOutcome 是一次供应商费用贷项形成的应用处理结果。
type CreditNoteOutcome uint8

const (
	CreditNoteOutcomeInvalid CreditNoteOutcome = iota
	CreditNoteFormed
	CreditNoteExistingResult
	CreditNoteConflict
	CreditNotAccepted
	CreditUndecided
)

func (outcome CreditNoteOutcome) String() string {
	switch outcome {
	case CreditNoteFormed:
		return "CREDIT_NOTE_FORMED"
	case CreditNoteExistingResult:
		return "EXISTING_CREDIT_NOTE"
	case CreditNoteConflict:
		return "CREDIT_NOTE_CONFLICT"
	case CreditNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case CreditUndecided:
		return "CREDIT_UNDECIDED"
	default:
		return ""
	}
}

// CreditUndecidedReason 指名贷项形成停在哪一步等谁。
type CreditUndecidedReason uint8

const (
	CreditUndecidedReasonNone CreditUndecidedReason = iota
	CreditPayableStoreUnavailable
	CreditNoteStoreUnavailable
)

func (reason CreditUndecidedReason) String() string {
	switch reason {
	case CreditPayableStoreUnavailable:
		return "PAYABLE_STORE_UNAVAILABLE"
	case CreditNoteStoreUnavailable:
		return "CREDIT_NOTE_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// FormSupplierCreditNoteCommand 携带供应商更正/贷项对某份审核应付的贷记主张（UC-SA-004
// 步 6）。主张、结算账户与币种不在命令上——它们从被贷记的原应付取，贷项与原应付因此在
// 结构上不可能指向两本账或两种币。出具时点是供应商的来源事实，随命令带入。
type FormSupplierCreditNoteCommand struct {
	TenantID    domain.TenantID
	Note        domain.CreditNoteID
	Version     domain.CreditNoteVersion
	Payable     domain.PayableID
	AmountMinor int64
	Reason      domain.CreditReasonReference
	IssuedAt    time.Time
}

type CreditNoteResult struct {
	outcome      CreditNoteOutcome
	reason       CreditUndecidedReason
	record       ports.SupplierCreditNoteRecord
	hasRecord    bool
	continuation string
	handoff      string
}

func (result CreditNoteResult) Outcome() CreditNoteOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result CreditNoteResult) UndecidedReason() CreditUndecidedReason {
	return result.reason
}

func (result CreditNoteResult) CreditNote() (ports.SupplierCreditNoteRecord, bool) {
	return result.record, result.hasRecord
}

func (result CreditNoteResult) ContinuationReference() string {
	return result.continuation
}

// CreditNoteHandoffReference 非空说明贷项已提交但它自己的发布意图还没交出去，重放会重发同一份。
func (result CreditNoteResult) CreditNoteHandoffReference() string {
	return result.handoff
}

// Credit 对一份既有审核应付追加供应商费用贷项（UC-SA-004 步 6）：同（身份+版本）重放/冲突
// → 找回原应付（不存在是提交矛盾）→ FormSupplierCreditNote（账户、主张、币种取自原应付）
// → 提交 → 发布贷项引用。原应付一字不动：本编排连应付的写口都不碰（AT-SA-090）。
func (handler *ReceiveSupplierBillHandler) Credit(
	ctx context.Context,
	command FormSupplierCreditNoteCommand,
) (CreditNoteResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" ||
		command.Note.String() == "" || command.Version.String() == "" || command.Payable.String() == "" {
		return CreditNoteResult{outcome: CreditNotAccepted}, nil
	}

	key := ports.SupplierCreditNoteKey{TenantID: command.TenantID, Note: command.Note, Version: command.Version}
	digest := creditNoteDigest(command)
	existing, found, err := handler.deps.CreditNotes.FindByKey(ctx, key)
	if err != nil {
		return creditUndecided(CreditNoteStoreUnavailable, command.Note.String()), nil
	}
	if found {
		if existing.ContentDigest != digest {
			return CreditNoteResult{outcome: CreditNoteConflict}, nil
		}
		return handler.existingCreditNote(ctx, existing), nil
	}

	payable, found, err := handler.deps.Payables.FindByKey(ctx, ports.AuditedPayableKey{
		TenantID: command.TenantID, Payable: command.Payable,
	})
	if err != nil {
		return creditUndecided(CreditPayableStoreUnavailable, command.Payable.String()), nil
	}
	if !found {
		// 指名了不存在的应付：提交矛盾。贷项只能回指已成立的审核应付，不能先于它。
		return CreditNoteResult{outcome: CreditNotAccepted}, nil
	}

	currency, _ := payable.Payable.Amount()
	note, err := domain.FormSupplierCreditNote(domain.SupplierCreditNoteSpec{
		Note:        command.Note,
		Version:     command.Version,
		Payable:     payable.Payable.Payable(),
		Claim:       payable.Payable.Claim(),
		Account:     payable.Payable.Account(),
		Currency:    currency,
		AmountMinor: command.AmountMinor,
		Reason:      command.Reason,
		IssuedAt:    command.IssuedAt,
	})
	if err != nil {
		return CreditNoteResult{outcome: CreditNotAccepted}, nil
	}

	record := ports.SupplierCreditNoteRecord{
		Key:           key,
		ContentDigest: digest,
		Note:          note,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.CreditNotes.Save(ctx, record)
	if err != nil {
		return creditUndecided(CreditNoteStoreUnavailable, command.Note.String()), nil
	}
	switch saved {
	case ports.SupplierCreditNoteSaved:
		result := CreditNoteResult{outcome: CreditNoteFormed, record: record, hasRecord: true}
		result.handoff = handler.handOffCreditNote(ctx, record)
		return result, nil
	case ports.SupplierCreditNoteAlreadyRecorded:
		winner, found, err := handler.deps.CreditNotes.FindByKey(ctx, key)
		if err != nil || !found {
			return creditUndecided(CreditNoteStoreUnavailable, command.Note.String()), nil
		}
		return handler.existingCreditNote(ctx, winner), nil
	default:
		return CreditNoteResult{}, fmt.Errorf("%w: %d", ErrUnexpectedCreditNoteSave, saved)
	}
}

func creditUndecided(reason CreditUndecidedReason, subject string) CreditNoteResult {
	return CreditNoteResult{
		outcome:      CreditUndecided,
		reason:       reason,
		continuation: billContinuation(reason.String(), subject),
	}
}

// existingCreditNote 按已有贷项作答并重发同一份意图。
func (handler *ReceiveSupplierBillHandler) existingCreditNote(
	ctx context.Context,
	record ports.SupplierCreditNoteRecord,
) CreditNoteResult {
	return CreditNoteResult{
		outcome:   CreditNoteExistingResult,
		record:    record,
		hasRecord: true,
		handoff:   handler.handOffCreditNote(ctx, record),
	}
}

// handOffCreditNote 发布贷项引用（UC-SA-004 步 7 的另一半）。只恢复贷项自己的同一发布
// 意图，不把应付那一半的成败推定过来（AT-SA-098）。
func (handler *ReceiveSupplierBillHandler) handOffCreditNote(
	ctx context.Context,
	record ports.SupplierCreditNoteRecord,
) string {
	if err := handler.deps.Downstream.HandOffSupplierBill(ctx, ports.SupplierBillHandoffIntent{CreditNote: record}); err == nil {
		return ""
	}
	return billContinuation("SUPPLIER_CREDIT_NOTE_HANDOFF",
		record.Key.TenantID.String(), record.Key.Note.String(), record.Key.Version.String())
}

// creditNoteDigest 是同一贷项身份与版本的内容比对锚：原应付、金额、原因与出具时点任一不同
// 即是另一份内容。
func creditNoteDigest(command FormSupplierCreditNoteCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Payable.String(),
		fmt.Sprintf("%d", command.AmountMinor),
		command.Reason.String(),
		command.IssuedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}
