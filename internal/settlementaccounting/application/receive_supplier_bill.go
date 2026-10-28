// receive_supplier_bill.go 编排 UC-SA-004 的接收半程：受理、幂等、逐行匹配提交与发布
// 意图。审核通过形成应付是另一步——匹配完成不是应付，Handle 不铸应付；步 5–7 的审核、
// 贷项与各自的发布在 audit_supplier_bill.go，同一个处理器、同一组依赖。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrUnexpectedBillSave 说明账单接收库交回了封闭集合以外的写入结果。
var ErrUnexpectedBillSave = errors.New("settlement accounting: unexpected bill save outcome")

// BillReceptionOutcome 是一次账单提交的应用处理结果。`未受理`与`未决`分格（ADR-0029
// 按恢复动作分格）：前者改请求，后者等依赖。
type BillReceptionOutcome uint8

const (
	BillReceptionOutcomeInvalid BillReceptionOutcome = iota
	BillReceived
	BillExistingResult
	BillVersionConflict
	BillNotAccepted
	BillUndecided
	// BillExpectedCostNotCurrent：一行点名的预期成本版本不是此刻的当前有效预期成本——它是失效纠错版本（依的发生项已失效），或已有
	// 后继（被纠正过）。照它匹配，就会给一份已不成立、或已重述过的成本对上账（ADR-0232 决定七）。续办是改点名当前那一版，或不匹配。
	BillExpectedCostNotCurrent
)

func (outcome BillReceptionOutcome) String() string {
	switch outcome {
	case BillReceived:
		return "BILL_RECEIVED"
	case BillExistingResult:
		return "EXISTING_RESULT"
	case BillVersionConflict:
		return "VERSION_CONFLICT"
	case BillNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case BillUndecided:
		return "BILL_UNDECIDED"
	case BillExpectedCostNotCurrent:
		return "EXPECTED_COST_NOT_CURRENT"
	default:
		return ""
	}
}

// BillUndecidedReason 指名提交停在哪一步等谁。封闭集合：换算依据未配置也是一格——
// 跨币种匹配答未配置（原因 CONVERSION_UNCONFIGURED）是等配置，不是改单（AT-SA-096）。
type BillUndecidedReason uint8

const (
	BillUndecidedReasonNone BillUndecidedReason = iota
	BillStoreUnavailable
	BillExpectedCostUnavailable
	BillConversionUnconfigured
)

func (reason BillUndecidedReason) String() string {
	switch reason {
	case BillStoreUnavailable:
		return "BILL_STORE_UNAVAILABLE"
	case BillExpectedCostUnavailable:
		return "EXPECTED_COST_UNAVAILABLE"
	case BillConversionUnconfigured:
		return "CONVERSION_UNCONFIGURED"
	default:
		return ""
	}
}

// LineMatchDirective 是对主张一行的匹配裁决：分类由适用规则得出后进入，`无匹配发生项`
// 不带预期成本版本，其余四格必须指名所引的预期成本。
type LineMatchDirective struct {
	Line            domain.BillLineReference
	Classification  domain.MatchClassification
	ExpectedVersion domain.SupplierCostVersionID
	Basis           domain.MatchBasisReference
}

// ReceiveSupplierBillCommand 携带一份供应商账单的全部来源与逐行匹配裁决。
type ReceiveSupplierBillCommand struct {
	TenantID   domain.TenantID
	Claim      domain.SupplierBillClaimSpec
	Directives []LineMatchDirective
	// OperationEvidence 只进审计，见 ports.BillReceptionRecord 的同名格。
	OperationEvidence string
}

type ReceiveSupplierBillResult struct {
	outcome      BillReceptionOutcome
	reason       BillUndecidedReason
	record       ports.BillReceptionRecord
	hasRecord    bool
	continuation string
	handoff      string
	notCurrent   ExpectedCostNotCurrent
}

// ExpectedCostNotCurrent 说点名的那一版为什么不是当前：失效纠错版本，或已有后继（Successor）。
type ExpectedCostNotCurrent struct {
	Line      domain.BillLineReference
	Version   domain.SupplierCostVersionID
	Withdrawn bool
	Successor domain.SupplierCostVersionID
}

// NotCurrent 只在`预期成本不是当前`时有值。
func (result ReceiveSupplierBillResult) NotCurrent() (ExpectedCostNotCurrent, bool) {
	return result.notCurrent, result.outcome == BillExpectedCostNotCurrent
}

func (result ReceiveSupplierBillResult) Outcome() BillReceptionOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result ReceiveSupplierBillResult) UndecidedReason() BillUndecidedReason {
	return result.reason
}

func (result ReceiveSupplierBillResult) Record() (ports.BillReceptionRecord, bool) {
	return result.record, result.hasRecord
}

func (result ReceiveSupplierBillResult) ContinuationReference() string {
	return result.continuation
}

// BillHandoffReference 非空说明记录已提交但意图还没交出去，重放会重发同一份。
func (result ReceiveSupplierBillResult) BillHandoffReference() string {
	return result.handoff
}

// AuditUndecided 报告审核步骤是否因授权未配置而停在未决。接收与匹配照常成立（机制
// 半边），审核授权是实例半边——未配置不默认放行、不虚构授权人，应付一格也不形成。
func (result ReceiveSupplierBillResult) AuditUndecided() bool {
	return result.hasRecord && !result.record.AuditAuthorityConfigured
}

// ReceiveSupplierBillDeps 是 UC-SA-004 全程的依赖：接收半程用 Receptions/Costs/Authority，
// 审核半程再用 Accounts/Payables，贷项用 CreditNotes；三种结果同走一个 Downstream 分别发布。
type ReceiveSupplierBillDeps struct {
	Receptions ports.BillReceptionStore
	Costs      ports.ExpectedCostView
	// Successions 判点名的预期成本版本有没有后继（ADR-0232 决定七）。必备：缺了它，一行就可能对上一份已被纠正过的成本。
	Successions ports.ExpectedCostSuccessionView
	Authority   ports.SupplierAuditAuthorityView
	Accounts    ports.SupplierPayableAccountView
	Escalation  ports.AuditEscalationCeilingView
	Payables    ports.AuditedPayableStore
	CreditNotes ports.SupplierCreditNoteStore
	Downstream  ports.SupplierBillHandoff
	Clock       ports.Clock
}

type ReceiveSupplierBillHandler struct {
	deps ReceiveSupplierBillDeps
}

func NewReceiveSupplierBillHandler(deps ReceiveSupplierBillDeps) *ReceiveSupplierBillHandler {
	return &ReceiveSupplierBillHandler{deps: deps}
}

// Handle 把一份供应商账单推进到接收与逐行匹配：幂等/冲突按内容指纹分界（AT-SA-092/093）
// → 主张成形 → 逐行匹配（每行必须有裁决，跨币种停在待判断）→ 审核授权探查（未配置即
// 审核未决，不铸应付）→ 原子提交 → 发布意图。意图投递失败不翻结果，重放重发同一份。
func (handler *ReceiveSupplierBillHandler) Handle(
	ctx context.Context,
	command ReceiveSupplierBillCommand,
) (ReceiveSupplierBillResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" ||
		!commandIdentityPresent(command) {
		return ReceiveSupplierBillResult{outcome: BillNotAccepted}, nil
	}

	key := ports.BillReceptionKey{
		TenantID: command.TenantID,
		Claim:    command.Claim.Claim,
		Version:  command.Claim.Version,
	}
	digest := billContentDigest(command)
	existing, found, err := handler.deps.Receptions.FindByKey(ctx, key)
	if err != nil {
		return billStoreUndecided(command.Claim.Claim.String()), nil
	}
	if found {
		switch domain.CompareStoredDigest(existing.ContentDigest, digest, unversionedBillContentDigest(command)) {
		case domain.DifferentPayload:
			// 同一主张身份和版本携带不同金额或范围：版本冲突保留原结果，不按最后到达
			// 覆盖（AT-SA-093）。
			return ReceiveSupplierBillResult{outcome: BillVersionConflict}, nil
		case domain.UnknownPayloadShape:
			return billStoreUndecided(command.Claim.Claim.String()), nil
		}
		// 同一主张重复到达：返回原结果，不重复匹配或审核（AT-SA-092）。
		return handler.existingResult(ctx, existing), nil
	}

	claim, err := domain.ReceiveSupplierBillClaim(command.Claim)
	if err != nil {
		return ReceiveSupplierBillResult{outcome: BillNotAccepted}, nil
	}
	// 每行必须有且仅有一个裁决：漏行会让未匹配金额凭空消失，多余裁决指向主张外的行。
	if len(command.Directives) != len(claim.Lines()) {
		return ReceiveSupplierBillResult{outcome: BillNotAccepted}, nil
	}

	record := ports.BillReceptionRecord{
		Key:               key,
		ContentDigest:     digest,
		Claim:             claim,
		RecordedAt:        handler.deps.Clock.Now(),
		OperationEvidence: command.OperationEvidence,
	}
	for _, directive := range command.Directives {
		expected := domain.SupplierExpectedCost{}
		if directive.ExpectedVersion.String() != "" {
			loaded, found, err := handler.deps.Costs.LoadExpectedCost(ctx, command.TenantID, directive.ExpectedVersion)
			if err != nil {
				return ReceiveSupplierBillResult{outcome: BillUndecided, reason: BillExpectedCostUnavailable,
					continuation: billContinuation("EXPECTED_COST_UNAVAILABLE", command.Claim.Claim.String())}, nil
			}
			if !found {
				// 指名了不存在的预期成本版本：提交矛盾，改单重来。
				return ReceiveSupplierBillResult{outcome: BillNotAccepted}, nil
			}
			if answer, stale, err := handler.notCurrent(ctx, command, directive, loaded); err != nil || stale {
				return answer, err
			}
			expected = loaded
		}
		match, err := domain.MatchBillLine(claim, directive.Line, directive.Classification, expected, directive.Basis, record.RecordedAt)
		if errors.Is(err, domain.ErrCrossCurrencyMatch) {
			// 币种不同且无换算依据：答未配置（原因 CONVERSION_UNCONFIGURED），不用当前汇率猜测（AT-SA-096）。
			return ReceiveSupplierBillResult{outcome: BillUndecided, reason: BillConversionUnconfigured,
				continuation: billContinuation("CONVERSION_UNCONFIGURED", command.Claim.Claim.String(), directive.Line.String())}, nil
		}
		if err != nil {
			return ReceiveSupplierBillResult{outcome: BillNotAccepted}, nil
		}
		record.Matches = append(record.Matches, match)
	}

	_, configured, err := handler.deps.Authority.LoadSupplierAuditAuthority(ctx, command.TenantID, claim.Supplier(), claim.LegalEntity())
	if err != nil {
		return billStoreUndecided(command.Claim.Claim.String()), nil
	}
	record.AuditAuthorityConfigured = configured

	return handler.commit(ctx, record)
}

// notCurrent 判点名的那一版是不是此刻的当前有效预期成本：失效纠错版本不是，有后继的也不是。两格都不落任何行。
func (handler *ReceiveSupplierBillHandler) notCurrent(
	ctx context.Context,
	command ReceiveSupplierBillCommand,
	directive LineMatchDirective,
	loaded domain.SupplierExpectedCost,
) (ReceiveSupplierBillResult, bool, error) {
	if handler.deps.Successions == nil {
		return ReceiveSupplierBillResult{}, false, errors.New("settlement accounting: supplier bill reception is wired without the expected cost succession view")
	}
	stale := ExpectedCostNotCurrent{Line: directive.Line, Version: loaded.Version(), Withdrawn: loaded.Withdrawn()}
	successor, superseded, err := handler.deps.Successions.LoadExpectedCostSuccessor(ctx, command.TenantID, loaded.Version())
	if err != nil {
		return ReceiveSupplierBillResult{outcome: BillUndecided, reason: BillExpectedCostUnavailable,
			continuation: billContinuation("EXPECTED_COST_UNAVAILABLE", command.Claim.Claim.String())}, true, nil
	}
	if superseded {
		stale.Successor = successor
	}
	if !stale.Withdrawn && !superseded {
		return ReceiveSupplierBillResult{}, false, nil
	}
	return ReceiveSupplierBillResult{outcome: BillExpectedCostNotCurrent, notCurrent: stale,
		continuation: billContinuation("EXPECTED_COST_NOT_CURRENT", command.Claim.Claim.String(), directive.Line.String())}, true, nil
}

func commandIdentityPresent(command ReceiveSupplierBillCommand) bool {
	return command.Claim.Claim.String() != "" && command.Claim.Version.String() != ""
}

func billStoreUndecided(claimID string) ReceiveSupplierBillResult {
	return ReceiveSupplierBillResult{
		outcome:      BillUndecided,
		reason:       BillStoreUnavailable,
		continuation: billContinuation("BILL_STORE_UNAVAILABLE", claimID),
	}
}

// commit 提交记录并交发布意图；并发下另一方先提交时读回赢家。
func (handler *ReceiveSupplierBillHandler) commit(
	ctx context.Context,
	record ports.BillReceptionRecord,
) (ReceiveSupplierBillResult, error) {
	saved, err := handler.deps.Receptions.Save(ctx, record)
	if err != nil {
		return billStoreUndecided(record.Key.Claim.String()), nil
	}
	switch saved {
	case ports.BillSaved:
		result := ReceiveSupplierBillResult{outcome: BillReceived, record: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, record)
		return result, nil
	case ports.BillAlreadyRecorded:
		winner, found, err := handler.deps.Receptions.FindByKey(ctx, record.Key)
		if err != nil || !found {
			return billStoreUndecided(record.Key.Claim.String()), nil
		}
		return handler.existingResult(ctx, winner), nil
	default:
		return ReceiveSupplierBillResult{}, fmt.Errorf("%w: %d", ErrUnexpectedBillSave, saved)
	}
}

// existingResult 按已有记录作答并重发同一份意图（AT-SA-092）。
func (handler *ReceiveSupplierBillHandler) existingResult(
	ctx context.Context,
	record ports.BillReceptionRecord,
) ReceiveSupplierBillResult {
	return ReceiveSupplierBillResult{
		outcome:   BillExistingResult,
		record:    record,
		hasRecord: true,
		handoff:   handler.handOff(ctx, record),
	}
}

// handOff 交发布意图。接收记录本身就是审核与对账的消费物，提交即交；投递失败不翻
// 结果，留续办引用重放时重发同一份（AT-SA-098 的接收半程）。
func (handler *ReceiveSupplierBillHandler) handOff(
	ctx context.Context,
	record ports.BillReceptionRecord,
) string {
	if err := handler.deps.Downstream.HandOffSupplierBill(ctx, ports.SupplierBillHandoffIntent{Record: record}); err == nil {
		return ""
	}
	return billContinuation("SUPPLIER_BILL_HANDOFF", record.Key.TenantID.String(), record.Key.Claim.String())
}

func billContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

// billContentDigest 是同一主张身份的内容比对锚：供应商、账期、币种、行金额与逐行裁决
// 任一不同即是另一份内容。行先排序——提交顺序不构成不同的内容。
func billContentDigest(command ReceiveSupplierBillCommand) string {
	lines := make([]domain.SupplierBillLineContent, len(command.Claim.Lines))
	for i, line := range command.Claim.Lines {
		lines[i] = domain.SupplierBillLineContent{Line: line.Line.String(), FeeItem: line.FeeItem.String(), ClaimedMinor: line.ClaimedMinor}
	}
	directives := make([]domain.SupplierBillDirectiveContent, len(command.Directives))
	for i, directive := range command.Directives {
		directives[i] = domain.SupplierBillDirectiveContent{
			Line: directive.Line.String(), Classification: int64(directive.Classification),
			ExpectedVersion: directive.ExpectedVersion.String(), Basis: directive.Basis.String(),
		}
	}
	return settlementCanonicalDigest(domain.CanonicalizeSupplierBillPayload(
		command.Claim.Supplier.String(), command.Claim.Period.String(), command.Claim.Currency.String(), command.Claim.ReceivedAt, lines, directives))
}
