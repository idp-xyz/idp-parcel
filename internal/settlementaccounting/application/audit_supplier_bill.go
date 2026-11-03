// audit_supplier_bill.go 编排 UC-SA-004 的审核半程：步 5 由授权审核责任方对一行上的一段金额给通过、争议或拒绝，通过的那一段
// 形成审核应付（ADR-0357），步 6 接收供应商贷项回指原应付，步 7 把两者的引用分别发布。它续在 receive_supplier_bill.go 的接收
// 半程之后，共用同一个处理器与依赖——接收记录是审核的唯一起点，命令里没有金额：审核改不了数字，只裁决这一行付多少、付哪一段。
package application

import (
	"context"
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
	// ErrUnexpectedAuditDecisionSave 说明审核决定库交回了封闭集合以外的写入结果。
	ErrUnexpectedAuditDecisionSave = errors.New("settlement accounting: unexpected audit decision save outcome")
	// ErrUnexpectedCreditNoteSave 说明贷项库交回了封闭集合以外的写入结果。
	ErrUnexpectedCreditNoteSave = errors.New("settlement accounting: unexpected supplier credit note save outcome")
)

// AuditOutcome 是一次审核决定的应用处理结果（ADR-0357 决定五）。按恢复动作分格（ADR-0029）：`行不可审`是通过超出了已匹配
// 金额，续办是争议或拒绝那一段、或先更正预期成本再以新版本匹配；`范围已裁决`是那一段已有结果，续办是重读这一行另挑一段。
type AuditOutcome uint8

const (
	AuditOutcomeInvalid AuditOutcome = iota
	PayableFormed
	PayableExistingResult
	PayableConflict
	LineNotAuditable
	AuditMustEscalate
	AuditNotAccepted
	AuditUndecided
	// PayableNumberingRuleCollision：形成审核应付时单据编号规则跳号到上限仍撞上已用的号（ADR-0355 决定八）。是答案不是故障：
	// 这一次不形成应付，要去规则册改格式；调用方必须回滚这一笔事务，取过的流水才释放。
	PayableNumberingRuleCollision
	// AuditDisputeRecorded 与 AuditRejectionRecorded：争议、拒绝那一段已记下，不形成应付、不取号。
	AuditDisputeRecorded
	AuditRejectionRecorded
	// AuditDecisionExistingResult：同一份争议或拒绝重放，答原决定。通过的重放仍答 PayableExistingResult，带回应付与它的号。
	AuditDecisionExistingResult
	// AuditDecisionConflict：同一决定身份已用于另一份内容，这次是争议或拒绝；这次是通过时仍答 PayableConflict（旧载荷的词不变）。
	AuditDecisionConflict
	// AuditRangeAlreadyDecided：这一段与同一行上已有的决定重叠。通过与拒绝终局，争议只经再审解除。
	AuditRangeAlreadyDecided
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
	case AuditMustEscalate:
		return "AUDIT_MUST_ESCALATE"
	case AuditNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case AuditUndecided:
		return "AUDIT_UNDECIDED"
	case PayableNumberingRuleCollision:
		return "NUMBERING_RULE_COLLISION"
	case AuditDisputeRecorded:
		return "DISPUTE_RECORDED"
	case AuditRejectionRecorded:
		return "REJECTION_RECORDED"
	case AuditDecisionExistingResult:
		return "EXISTING_DECISION"
	case AuditDecisionConflict:
		return "DECISION_CONFLICT"
	case AuditRangeAlreadyDecided:
		return "RANGE_ALREADY_DECIDED"
	default:
		return ""
	}
}

// AuditUndecidedReason 指名审核停在哪一步等谁。各 *Unconfigured 格都是实例半边：审核授权
// （UC-SA-004「无授权不得人工接受或拒绝」，`AT-SA-088`）、供应商应付结算账户（`PAR-SET-01`）
// 与审核越权升级上限（ADR-0163）未登记时不默认放行、不虚构授权人、不推导账户、不把没有上限
// 当成不设限。
type AuditUndecidedReason uint8

const (
	AuditUndecidedReasonNone AuditUndecidedReason = iota
	AuditBillStoreUnavailable
	AuditAuthorityUnavailable
	AuditAuthorityUnconfigured
	PayableAccountUnavailable
	PayableAccountUnconfigured
	PayableStoreUnavailable
	AuditEscalationViewUnavailable
	AuditEscalationUnconfigured
	// AuditArrangementViewUnavailable：读不动应付账户的对账安排，定不了应付单据号的日期时区。
	AuditArrangementViewUnavailable
	// AuditDecisionStoreUnavailable：读不动或写不进审核决定册。
	AuditDecisionStoreUnavailable
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
	case AuditEscalationViewUnavailable:
		return "AUDIT_ESCALATION_VIEW_UNAVAILABLE"
	case AuditEscalationUnconfigured:
		return "AUDIT_ESCALATION_UNCONFIGURED"
	case AuditArrangementViewUnavailable:
		return "ARRANGEMENT_VIEW_UNAVAILABLE"
	case AuditDecisionStoreUnavailable:
		return "AUDIT_DECISION_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// AuditBillLineCommand 指名要审核哪份接收里的哪一行、哪一段，给哪种结果。刻意没有金额、账户与审核人：金额是那一段的长度、
// 上限是册上的已匹配金额，账户与审核人从已登记实例取——命令带得了它们，「供应商不能直接写入审核结果」就只剩一句话。
//
// 两种形状（ADR-0357 决定五）：
//   - 新形给 Decision、Verdict、Range，可带 Basis 与 Resolves。Decision 是调用方生成的幂等键，通过时它就是所形成审核应付的
//     标识（ADR-0355 决定十）。
//   - 旧形只给 Payable：以它为标识，通过这一行的整段主张金额；摘要沿用旧形，旧应付的重放照旧认得。
//
// 两者只能给一种，混给答未受理。
type AuditBillLineCommand struct {
	TenantID domain.TenantID
	Claim    domain.BillClaimID
	Version  domain.BillClaimVersion
	Line     domain.BillLineReference
	Payable  domain.PayableID
	Decision domain.AuditDecisionID
	Verdict  domain.AuditVerdict
	Range    domain.AmountRange
	Basis    domain.AuditBasisReference
	Resolves domain.AuditDecisionID
	// OperationEvidence 只进审计，见 ports.AuditDecisionRecord 的同名格。
	OperationEvidence string
}

// wholeLine 说这是不是旧形载荷。
func (command AuditBillLineCommand) wholeLine() bool {
	return command.Payable.String() != ""
}

// decisionID 是这次决定的身份：旧形取应付标识。
func (command AuditBillLineCommand) decisionID() domain.AuditDecisionID {
	if command.wholeLine() {
		decision, _ := domain.NewAuditDecisionID(command.Payable.String())
		return decision
	}
	return command.Decision
}

func (command AuditBillLineCommand) verdict() domain.AuditVerdict {
	if command.wholeLine() {
		return domain.AuditPassed
	}
	return command.Verdict
}

type AuditResult struct {
	outcome      AuditOutcome
	reason       AuditUndecidedReason
	record       ports.AuditedPayableRecord
	hasRecord    bool
	decision     ports.AuditDecisionRecord
	hasDecision  bool
	matchedMinor int64
	passedMinor  int64
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

// Decision 交回形成或读回的那份审核决定；通过、争议、拒绝与各自的重放都有它。
func (result AuditResult) Decision() (ports.AuditDecisionRecord, bool) {
	return result.decision, result.hasDecision
}

// MatchedAmounts 只在`行不可审`时有意义：这一行的已匹配金额，与此前已通过的金额。
func (result AuditResult) MatchedAmounts() (matchedMinor, passedMinor int64) {
	return result.matchedMinor, result.passedMinor
}

func (result AuditResult) ContinuationReference() string {
	return result.continuation
}

// PayableHandoffReference 非空说明应付已提交但它自己的发布意图还没交出去，重放会重发同一份。
func (result AuditResult) PayableHandoffReference() string {
	return result.handoff
}

// Audit 对一份已接收主张的一行给一份审核决定（UC-SA-004 步 5，ADR-0357）：按决定身份与行排队 → 同身份重放/冲突 → 找回接收与
// 那一行的匹配 → 读这一行的全部已有决定，同一段同一结果已有的读回先到者 → 审核授权从已登记实例取（未配置停在未决）→
// DecideBillLineAudit（范围守恒、通过不超已匹配金额、再审只落在争议之内）→ 争议与拒绝就此落册；通过的再取应付账户、
// FormAuditedPayable、越权上限、取应付单据号，决定与应付同笔落库 → 发布应付引用。每次只给一段：批量含通过、争议与拒绝时逐段
// 返回，合法通过的段不被局部失败回滚（AT-SA-099）。
//
// 单据号只在确定要形成应付的那一步取（ADR-0355 决定三）：重放读回原应付连同它的号；争议、拒绝、范围已裁决、行不可审、要升级、
// 未配置都答业务答案而不落应付，那几路不取号。
func (handler *ReceiveSupplierBillHandler) Audit(
	ctx context.Context,
	command AuditBillLineCommand,
) (AuditResult, error) {
	if handler.deps.Numbers == nil || handler.deps.Arrangements == nil || handler.deps.Decisions == nil {
		return AuditResult{}, fmt.Errorf("%w: audit needs document numbers, statement arrangements and the audit decision store", ErrNilDependency)
	}
	if strings.TrimSpace(command.TenantID.String()) == "" ||
		command.Claim.String() == "" || command.Version.String() == "" || command.Line.String() == "" ||
		command.wholeLine() == (command.Decision.String() != "") {
		return AuditResult{outcome: AuditNotAccepted}, nil
	}

	key := ports.AuditDecisionKey{TenantID: command.TenantID, Decision: command.decisionID()}
	digest, unversioned := auditDigests(command)
	// 同一决定身份、同一行的两笔并发审核在这里排队：后到者往下读时先到者已提交，读到它就按身份或按范围答原决定、或答范围已裁决，
	// 不会两笔都取了号、输家再撞约束。
	if err := handler.deps.Decisions.LockLine(ctx, key, command.Claim, command.Line); err != nil {
		return auditUndecided(AuditDecisionStoreUnavailable, key.Decision.String()), nil
	}
	existing, found, err := handler.deps.Decisions.FindByKey(ctx, key)
	if err != nil {
		return auditUndecided(AuditDecisionStoreUnavailable, key.Decision.String()), nil
	}
	if found {
		switch domain.CompareStoredDigest(existing.ContentDigest, digest, unversioned) {
		case domain.DifferentPayload:
			// 同一决定身份指向另一段、另一行或另一种结果：请求冲突，原决定不被覆盖。
			if command.verdict() == domain.AuditPassed {
				return AuditResult{outcome: PayableConflict}, nil
			}
			return AuditResult{outcome: AuditDecisionConflict}, nil
		case domain.UnknownPayloadShape:
			return auditUndecided(AuditDecisionStoreUnavailable, key.Decision.String()), nil
		}
		return handler.existingDecision(ctx, existing), nil
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
	span := command.Range
	if command.wholeLine() {
		span, err = domain.NewAmountRange(0, match.ClaimedMinor())
		if err != nil {
			return AuditResult{outcome: AuditNotAccepted}, nil
		}
	}

	priorRecords, err := handler.deps.Decisions.ListByLine(ctx, command.TenantID, command.Claim, command.Line)
	if err != nil {
		return auditUndecided(AuditDecisionStoreUnavailable, command.Claim.String()), nil
	}
	prior := make([]domain.BillLineAuditDecision, 0, len(priorRecords))
	for _, record := range priorRecords {
		if sameRequest(record.Decision, command, span) {
			// 同一段同一结果换个身份再提交，交回先到的那份——同一业务变化只形成一次（原先「换个应付身份再审同一行」那一条）。
			return handler.existingDecision(ctx, record), nil
		}
		prior = append(prior, record.Decision)
	}

	auditor, configured, err := handler.deps.Authority.LoadSupplierAuditAuthority(
		ctx, command.TenantID, reception.Claim.Supplier(), reception.Claim.LegalEntity())
	if err != nil {
		return auditUndecided(AuditAuthorityUnavailable, command.Claim.String()), nil
	}
	if !configured {
		return auditUndecided(AuditAuthorityUnconfigured, command.Claim.String()), nil
	}

	decision, err := domain.DecideBillLineAudit(match, prior, domain.BillLineAuditDecisionSpec{
		Decision: key.Decision, Version: command.Version, Verdict: command.verdict(), Span: span,
		Basis: command.Basis, Resolves: command.Resolves, Auditor: auditor, AuditedAt: handler.deps.Clock.Now(),
	})
	switch {
	case errors.Is(err, domain.ErrAuditRangeTaken):
		return AuditResult{outcome: AuditRangeAlreadyDecided,
			continuation: billContinuation("RANGE_ALREADY_DECIDED", command.Claim.String(), command.Line.String())}, nil
	case errors.Is(err, domain.ErrNotAuditable):
		// 通过超出了已匹配金额：差异那一段、没有成本撑着的整行只能争议或拒绝，不在这里被顺手接受（AT-SA-086）。
		return AuditResult{outcome: LineNotAuditable, matchedMinor: match.MatchedMinor(), passedMinor: domain.TallyLineDecisions(match.ClaimedMinor(), prior).PassedMinor,
			continuation: billContinuation("LINE_NOT_AUDITABLE", command.Claim.String(), command.Line.String())}, nil
	case err != nil:
		// 范围出了这一行或出了所解争议、点名的不是这一行上的争议、形状不全：提交矛盾，改请求。
		return AuditResult{outcome: AuditNotAccepted}, nil
	}

	record := ports.AuditDecisionRecord{
		Key: key, ContentDigest: digest, Decision: decision,
		RecordedAt: handler.deps.Clock.Now(), OperationEvidence: command.OperationEvidence,
	}
	if decision.Verdict() != domain.AuditPassed {
		return handler.recordDecision(ctx, record)
	}
	return handler.formPayable(ctx, command, reception, match, record)
}

// sameRequest 说一份已有决定是不是正是这次请求要的那一段、那一种结果（domain.BillLineAuditDecision.Repeats）。
func sameRequest(earlier domain.BillLineAuditDecision, command AuditBillLineCommand, span domain.AmountRange) bool {
	return earlier.Repeats(command.verdict(), span, command.Basis, command.Resolves)
}

// recordDecision 落一份争议或拒绝。它们不形成应付、不取号、不交下游（ADR-0357 Consequences）。
func (handler *ReceiveSupplierBillHandler) recordDecision(ctx context.Context, record ports.AuditDecisionRecord) (AuditResult, error) {
	saved, err := handler.deps.Decisions.Save(ctx, record)
	if err != nil {
		return auditUndecided(AuditDecisionStoreUnavailable, record.Key.Decision.String()), nil
	}
	switch saved {
	case ports.AuditDecisionSaved:
		outcome := AuditDisputeRecorded
		if record.Decision.Verdict() == domain.AuditRejected {
			outcome = AuditRejectionRecorded
		}
		return AuditResult{outcome: outcome, decision: record, hasDecision: true}, nil
	case ports.AuditDecisionAlreadyRecorded:
		// 身份与这一行都在 LockLine 上排过队，走到这里说明有人没排队就写了库；按身份读回赢家，读不到就是范围被别处占了。
		winner, found, err := handler.deps.Decisions.FindByKey(ctx, record.Key)
		if err != nil {
			return auditUndecided(AuditDecisionStoreUnavailable, record.Key.Decision.String()), nil
		}
		if !found {
			return AuditResult{outcome: AuditRangeAlreadyDecided}, nil
		}
		return handler.existingDecision(ctx, winner), nil
	default:
		return AuditResult{}, fmt.Errorf("%w: %d", ErrUnexpectedAuditDecisionSave, saved)
	}
}

// formPayable 给一份通过的决定形成审核应付：应付账户、越权上限、取号，决定与应付同笔落库，再发布应付引用。
func (handler *ReceiveSupplierBillHandler) formPayable(
	ctx context.Context,
	command AuditBillLineCommand,
	reception ports.BillReceptionRecord,
	match domain.BillLineMatch,
	decisionRecord ports.AuditDecisionRecord,
) (AuditResult, error) {
	account, registered, err := handler.deps.Accounts.LoadSupplierPayableAccount(
		ctx, command.TenantID, reception.Claim.Supplier(), reception.Claim.LegalEntity(), reception.Claim.Currency())
	if err != nil {
		return auditUndecided(PayableAccountUnavailable, command.Claim.String()), nil
	}
	if !registered {
		return auditUndecided(PayableAccountUnconfigured, command.Claim.String()), nil
	}
	payable, err := domain.FormAuditedPayable(match, decisionRecord.Decision, reception.Claim.LegalEntity(), account)
	if err != nil {
		return AuditResult{outcome: AuditNotAccepted}, nil
	}
	// 上限比这一行的已匹配金额，不比这一段：按段比，拆成几段小的就绕过了上限（ADR-0357 决定三、ADR-0163 决定一）。
	if stopped, ok := handler.escalationStop(
		ctx, command.TenantID, reception.Claim.Supplier(), reception.Claim.LegalEntity(),
		match.Currency(), match.MatchedMinor(), command.Claim.String()); ok {
		return stopped, nil
	}

	payableKey := ports.AuditedPayableKey{TenantID: command.TenantID, Payable: payable.Payable()}
	numbering, stopped, stop, err := handler.numberPayable(ctx, command.TenantID, payableKey.Payable, account)
	if err != nil || stop {
		return stopped, err
	}
	_, numbered := numbering.Number()

	saved, err := handler.deps.Decisions.Save(ctx, decisionRecord)
	if err != nil {
		if numbered {
			return AuditResult{}, fmt.Errorf("%w: save audit decision %s: %w", ErrNumberedIssueInterrupted, payableKey.Payable, err)
		}
		return auditUndecided(AuditDecisionStoreUnavailable, payableKey.Payable.String()), nil
	}
	if saved != ports.AuditDecisionSaved {
		// 有人没在 LockLine 上排队就写了同一身份或同一段。取到了号就交回错误整笔回滚——读回赢家作答会让这个号随事务提交成为
		// 空号；没取到号时照旧读回赢家，不覆盖。
		if numbered {
			return AuditResult{}, fmt.Errorf("%w: audit decision %s was saved by another transaction", ErrNumberedIssueInterrupted, payableKey.Payable)
		}
		return handler.recordDecision(ctx, decisionRecord)
	}
	record := ports.AuditedPayableRecord{
		Key:               payableKey,
		ContentDigest:     decisionRecord.ContentDigest,
		Payable:           payable,
		RecordedAt:        decisionRecord.RecordedAt,
		OperationEvidence: command.OperationEvidence,
		Numbering:         numbering,
	}
	savedPayable, err := handler.deps.Payables.Save(ctx, record)
	if err != nil || savedPayable != ports.AuditedPayableSaved {
		// 决定已在这一笔里写下，应付却落不进：交回错误整笔回滚，不留一份没有应付的通过。
		return AuditResult{}, fmt.Errorf("%w: save payable %s: outcome %d: %v", ErrNumberedIssueInterrupted, payableKey.Payable, savedPayable, err)
	}
	result := AuditResult{outcome: PayableFormed, record: record, hasRecord: true, decision: decisionRecord, hasDecision: true}
	result.handoff = handler.handOffPayable(ctx, record)
	return result, nil
}

// numberPayable 在审核事务里给应付取单据号。日期按应付归入的结算账户对账安排的业务时区，账户没登对账安排就交空，由
// 编号组件退到租户的单据编号时区（domain.AuditedPayableDocumentNumberType）。交回的布尔为真时审核就此作答。
func (handler *ReceiveSupplierBillHandler) numberPayable(
	ctx context.Context,
	tenant domain.TenantID,
	payable domain.PayableID,
	account domain.SettlementAccountID,
) (domain.DocumentNumbering, AuditResult, bool, error) {
	zone := ""
	arrangement, arranged, err := handler.deps.Arrangements.LoadStatementArrangement(ctx, tenant, account)
	if err != nil {
		return domain.DocumentNumbering{}, auditUndecided(AuditArrangementViewUnavailable, payable.String()), true, nil
	}
	if arranged {
		zone = arrangement.TimeZone()
	}
	numbering, collided, err := issueDocumentNumber(ctx, handler.deps.Numbers, ports.DocumentNumberRequest{
		Tenant:         tenant,
		Type:           domain.AuditedPayableDocumentNumberType.Code,
		IssuerTimeZone: zone,
		IsUsed: func(ctx context.Context, number string) (bool, error) {
			return handler.deps.Payables.DocumentNumberUsed(ctx, tenant, number)
		},
	})
	if err != nil {
		return domain.DocumentNumbering{}, AuditResult{}, true, fmt.Errorf("settlement accounting: number payable %s: %w", payable, err)
	}
	if collided {
		return domain.DocumentNumbering{}, AuditResult{outcome: PayableNumberingRuleCollision}, true, nil
	}
	return numbering, AuditResult{}, false, nil
}

func matchForLine(record ports.BillReceptionRecord, line domain.BillLineReference) (domain.BillLineMatch, bool) {
	for _, match := range record.Matches {
		if match.Line() == line {
			return match, true
		}
	}
	return domain.BillLineMatch{}, false
}

func (handler *ReceiveSupplierBillHandler) escalationStop(
	ctx context.Context,
	tenant domain.TenantID,
	supplier domain.SupplierPartyReference,
	legalEntity domain.LegalEntityReference,
	currency domain.CurrencyCode,
	amountMinor int64,
	subject string,
) (AuditResult, bool) {
	if handler.deps.Escalation == nil {
		return auditUndecided(AuditEscalationViewUnavailable, subject), true
	}
	ceiling, found, err := handler.deps.Escalation.LoadAuditEscalationCeiling(
		ctx, tenant, supplier, legalEntity, currency)
	if err != nil {
		return auditUndecided(AuditEscalationViewUnavailable, subject), true
	}
	if !found {
		return auditUndecided(AuditEscalationUnconfigured, subject), true
	}
	judgment, err := ceiling.Judge(amountMinor)
	if err != nil {
		return AuditResult{outcome: AuditNotAccepted}, true
	}
	if judgment == domain.AuditMustEscalate {
		return AuditResult{
			outcome:      AuditMustEscalate,
			continuation: billContinuation("AUDIT_MUST_ESCALATE", subject),
		}, true
	}
	return AuditResult{}, false
}

func auditUndecided(reason AuditUndecidedReason, subject string) AuditResult {
	return AuditResult{
		outcome:      AuditUndecided,
		reason:       reason,
		continuation: billContinuation(reason.String(), subject),
	}
}

// existingDecision 按已有决定作答。通过的读回它的应付连同应付号并重发同一份发布意图；争议与拒绝答原决定。
func (handler *ReceiveSupplierBillHandler) existingDecision(
	ctx context.Context,
	record ports.AuditDecisionRecord,
) AuditResult {
	if record.Decision.Verdict() != domain.AuditPassed {
		return AuditResult{outcome: AuditDecisionExistingResult, decision: record, hasDecision: true}
	}
	payableID, err := domain.NewPayableID(record.Key.Decision.String())
	if err != nil {
		return auditUndecided(PayableStoreUnavailable, record.Key.Decision.String())
	}
	payable, found, err := handler.deps.Payables.FindByKey(ctx, ports.AuditedPayableKey{TenantID: record.Key.TenantID, Payable: payableID})
	if err != nil || !found {
		// 通过的决定在而应付不在，是册上矛盾——两者同笔落库；不当成没形成过，也不补形成。
		return auditUndecided(PayableStoreUnavailable, record.Key.Decision.String())
	}
	return AuditResult{
		outcome:     PayableExistingResult,
		record:      payable,
		hasRecord:   true,
		decision:    record,
		hasDecision: true,
		handoff:     handler.handOffPayable(ctx, payable),
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

// auditDigests 是同一决定身份的内容比对锚。旧形照旧按「审核账单行」那一面（主张、版本与行）算，存量旧摘要的无版本那一版也
// 照旧算；新形按「按范围给审核决定」那一面算，没有无版本那一版——那一面在 SAC-1 之后才有，交空串让任何无前缀的旧摘要都比
// 不上它（同一身份换一种形状来答冲突）。
func auditDigests(command AuditBillLineCommand) (string, string) {
	if command.wholeLine() {
		return settlementCanonicalDigest(domain.CanonicalizeAuditBillLinePayload(command.Claim.String(), command.Version.String(), command.Line.String())),
			unversionedAuditDigest(command)
	}
	return settlementCanonicalDigest(domain.CanonicalizeAuditBillLineRangePayload(
		command.Claim.String(), command.Version.String(), command.Line.String(), command.Verdict.String(),
		command.Range.From(), command.Range.To(), command.Basis.String(), command.Resolves.String())), ""
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
		switch domain.CompareStoredDigest(existing.ContentDigest, digest, unversionedCreditNoteDigest(command)) {
		case domain.DifferentPayload:
			return CreditNoteResult{outcome: CreditNoteConflict}, nil
		case domain.UnknownPayloadShape:
			return creditUndecided(CreditNoteStoreUnavailable, command.Note.String()), nil
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
	return settlementCanonicalDigest(domain.CanonicalizeSupplierCreditNotePayload(command.Payable.String(), command.AmountMinor, command.Reason.String(), command.IssuedAt))
}
