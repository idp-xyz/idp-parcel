package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// payloadCanonicalizationVersion 是结算命令口的现行形状。摘要带版本前缀，已保存的旧摘要
// 不在原处改写（ADR-0014）。哪些字段进摘要，沿用各口原先的内容判据；没有内容判据的口
// 不在这里发明形状。
const payloadCanonicalizationVersion = "SAC-1"

func canonicalPayloadDigest(document []byte) string {
	sum := sha256.Sum256(document)
	return payloadCanonicalizationVersion + ":" + hex.EncodeToString(sum[:])
}

func marshalPayload(document any) ([]byte, string, error) {
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, "", fmt.Errorf("settlement accounting: canonicalize command payload: %w", err)
	}
	return encoded, canonicalPayloadDigest(encoded), nil
}

func canonicalInstant(at time.Time) string {
	return at.UTC().Format(time.RFC3339Nano)
}

func sortedStrings(values []string) []string {
	copied := append([]string(nil), values...)
	slices.Sort(copied)
	if copied == nil {
		return []string{}
	}
	return copied
}

// CanonicalizeSupplierExpectedCostPayload 定形供应商预计成本的内容判据。时间的单调读数
// 与地点不进摘要，只保留发生时刻。
func CanonicalizeSupplierExpectedCostPayload(version, occurrenceID, occurrenceReason, occurrenceVersion string, occurredAt time.Time, feeItem, ruleVersion, agreement, evaluation, originalCurrency string, originalMinor int64, settlementCurrency string, settlementMinor int64, conversion string) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization   string `json:"canonicalization"`
		Face               string `json:"face"`
		Version            string `json:"version"`
		OccurrenceID       string `json:"occurrence_id"`
		OccurrenceReason   string `json:"occurrence_reason"`
		OccurrenceVersion  string `json:"occurrence_version"`
		OccurredAt         string `json:"occurred_at"`
		FeeItem            string `json:"fee_item"`
		RuleVersion        string `json:"rule_version"`
		Agreement          string `json:"agreement"`
		Evaluation         string `json:"evaluation"`
		OriginalCurrency   string `json:"original_currency"`
		OriginalMinor      int64  `json:"original_minor"`
		SettlementCurrency string `json:"settlement_currency"`
		SettlementMinor    int64  `json:"settlement_minor"`
		Conversion         string `json:"conversion"`
	}{
		Canonicalization:   payloadCanonicalizationVersion,
		Face:               "FORM_SUPPLIER_EXPECTED_COST",
		Version:            version,
		OccurrenceID:       occurrenceID,
		OccurrenceReason:   occurrenceReason,
		OccurrenceVersion:  occurrenceVersion,
		OccurredAt:         canonicalInstant(occurredAt),
		FeeItem:            feeItem,
		RuleVersion:        ruleVersion,
		Agreement:          agreement,
		Evaluation:         evaluation,
		OriginalCurrency:   originalCurrency,
		OriginalMinor:      originalMinor,
		SettlementCurrency: settlementCurrency,
		SettlementMinor:    settlementMinor,
		Conversion:         conversion,
	})
}

// CanonicalizePublishStatementPayload 定形「发布对账单」。费用集合排序后进入摘要。
func CanonicalizePublishStatementPayload(account, period, version, currency string, declaredTotalMinor int64, cutOffAt time.Time, charges []string) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization   string   `json:"canonicalization"`
		Face               string   `json:"face"`
		Account            string   `json:"account"`
		Period             string   `json:"period"`
		Version            string   `json:"version"`
		Currency           string   `json:"currency"`
		DeclaredTotalMinor int64    `json:"declared_total_minor"`
		CutOffAt           string   `json:"cut_off_at"`
		Charges            []string `json:"charges"`
	}{
		Canonicalization:   payloadCanonicalizationVersion,
		Face:               "PUBLISH_STATEMENT",
		Account:            account,
		Period:             period,
		Version:            version,
		Currency:           currency,
		DeclaredTotalMinor: declaredTotalMinor,
		CutOffAt:           canonicalInstant(cutOffAt),
		Charges:            sortedStrings(charges),
	})
}

// CanonicalizeLateChargeInclusionPayload 定形「纳入迟到费用」。
func CanonicalizeLateChargeInclusionPayload(number, chargeID, subsequentPeriod string, includedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Number           string `json:"number"`
		ChargeID         string `json:"charge_id"`
		SubsequentPeriod string `json:"subsequent_period"`
		IncludedAt       string `json:"included_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "INCLUDE_LATE_CHARGE",
		Number:           number,
		ChargeID:         chargeID,
		SubsequentPeriod: subsequentPeriod,
		IncludedAt:       canonicalInstant(includedAt),
	})
}

// CanonicalizeAdjustmentInclusionPayload 定形「纳入调整」。与迟到费用分面，同一纳入标识
// 不能因为其余字面相同而被读成重放。
func CanonicalizeAdjustmentInclusionPayload(number, adjustmentID, subsequentPeriod string, includedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Number           string `json:"number"`
		AdjustmentID     string `json:"adjustment_id"`
		SubsequentPeriod string `json:"subsequent_period"`
		IncludedAt       string `json:"included_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "INCLUDE_ADJUSTMENT",
		Number:           number,
		AdjustmentID:     adjustmentID,
		SubsequentPeriod: subsequentPeriod,
		IncludedAt:       canonicalInstant(includedAt),
	})
}

// CanonicalizeOpenDisputePayload 定形「开立争议」。
func CanonicalizeOpenDisputePayload(number, chargeID string, disputedMinor int64, reason string, openedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Number           string `json:"number"`
		ChargeID         string `json:"charge_id"`
		DisputedMinor    int64  `json:"disputed_minor"`
		Reason           string `json:"reason"`
		OpenedAt         string `json:"opened_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "OPEN_DISPUTE",
		Number:           number,
		ChargeID:         chargeID,
		DisputedMinor:    disputedMinor,
		Reason:           reason,
		OpenedAt:         canonicalInstant(openedAt),
	})
}

// CostPortionContent 是一条分摊份额。提交顺序不进摘要。
type CostPortionContent struct {
	Target      string
	AmountMinor int64
}

type portionDocument struct {
	Target      string `json:"target"`
	AmountMinor int64  `json:"amount_minor"`
}

func portionDocuments(portions []CostPortionContent) []portionDocument {
	lines := make([]portionDocument, 0, len(portions))
	for _, portion := range portions {
		lines = append(lines, portionDocument{Target: portion.Target, AmountMinor: portion.AmountMinor})
	}
	slices.SortFunc(lines, func(left, right portionDocument) int {
		if left.Target != right.Target {
			if left.Target < right.Target {
				return -1
			}
			return 1
		}
		if left.AmountMinor < right.AmountMinor {
			return -1
		}
		if left.AmountMinor > right.AmountMinor {
			return 1
		}
		return 0
	})
	return lines
}

// CanonicalizeAllocateCostPayload 定形「分摊成本」。
func CanonicalizeAllocateCostPayload(source string, sourceMinor int64, currency, version string, allocatedAt time.Time, portions []CostPortionContent) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string            `json:"canonicalization"`
		Face             string            `json:"face"`
		Source           string            `json:"source"`
		SourceMinor      int64             `json:"source_minor"`
		Currency         string            `json:"currency"`
		Version          string            `json:"version"`
		AllocatedAt      string            `json:"allocated_at"`
		Portions         []portionDocument `json:"portions"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "ALLOCATE_COST",
		Source:           source,
		SourceMinor:      sourceMinor,
		Currency:         currency,
		Version:          version,
		AllocatedAt:      canonicalInstant(allocatedAt),
		Portions:         portionDocuments(portions),
	})
}

// CanonicalizeReallocateCostPayload 定形「重分摊」。
func CanonicalizeReallocateCostPayload(newVersion string, allocatedAt time.Time, portions []CostPortionContent) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string            `json:"canonicalization"`
		Face             string            `json:"face"`
		NewVersion       string            `json:"new_version"`
		AllocatedAt      string            `json:"allocated_at"`
		Portions         []portionDocument `json:"portions"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "REALLOCATE_COST",
		NewVersion:       newVersion,
		AllocatedAt:      canonicalInstant(allocatedAt),
		Portions:         portionDocuments(portions),
	})
}

// CostComponentContent 是一条经营结果分量。提交顺序不进摘要。
type CostComponentContent struct {
	Source      string
	Effect      int64
	AmountMinor int64
}

type componentDocument struct {
	Source      string `json:"source"`
	Effect      int64  `json:"effect"`
	AmountMinor int64  `json:"amount_minor"`
}

func componentDocuments(components []CostComponentContent) []componentDocument {
	lines := make([]componentDocument, 0, len(components))
	for _, component := range components {
		lines = append(lines, componentDocument{Source: component.Source, Effect: component.Effect, AmountMinor: component.AmountMinor})
	}
	slices.SortFunc(lines, func(left, right componentDocument) int {
		if left.Source != right.Source {
			if left.Source < right.Source {
				return -1
			}
			return 1
		}
		if left.Effect != right.Effect {
			if left.Effect < right.Effect {
				return -1
			}
			return 1
		}
		if left.AmountMinor < right.AmountMinor {
			return -1
		}
		if left.AmountMinor > right.AmountMinor {
			return 1
		}
		return 0
	})
	return lines
}

// CanonicalizeDeriveResultPayload 定形「派生经营结果」。
func CanonicalizeDeriveResultPayload(currency, version string, asOf time.Time, components []CostComponentContent) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string              `json:"canonicalization"`
		Face             string              `json:"face"`
		Currency         string              `json:"currency"`
		Version          string              `json:"version"`
		AsOf             string              `json:"as_of"`
		Components       []componentDocument `json:"components"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "DERIVE_RESULT",
		Currency:         currency,
		Version:          version,
		AsOf:             canonicalInstant(asOf),
		Components:       componentDocuments(components),
	})
}

// CanonicalizeRederiveResultPayload 定形「重派生经营结果」。
func CanonicalizeRederiveResultPayload(newVersion string, asOf time.Time, components []CostComponentContent) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string              `json:"canonicalization"`
		Face             string              `json:"face"`
		NewVersion       string              `json:"new_version"`
		AsOf             string              `json:"as_of"`
		Components       []componentDocument `json:"components"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "REDERIVE_RESULT",
		NewVersion:       newVersion,
		AsOf:             canonicalInstant(asOf),
		Components:       componentDocuments(components),
	})
}

// CanonicalizeAuditBillLinePayload 定形「审核账单行」。
func CanonicalizeAuditBillLinePayload(claim, version, line string) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Claim            string `json:"claim"`
		Version          string `json:"version"`
		Line             string `json:"line"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "AUDIT_BILL_LINE",
		Claim:            claim,
		Version:          version,
		Line:             line,
	})
}

// CanonicalizeSupplierCreditNotePayload 定形「形成供应商贷项」。
func CanonicalizeSupplierCreditNotePayload(payable string, amountMinor int64, reason string, issuedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Payable          string `json:"payable"`
		AmountMinor      int64  `json:"amount_minor"`
		Reason           string `json:"reason"`
		IssuedAt         string `json:"issued_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "FORM_SUPPLIER_CREDIT_NOTE",
		Payable:          payable,
		AmountMinor:      amountMinor,
		Reason:           reason,
		IssuedAt:         canonicalInstant(issuedAt),
	})
}

// CanonicalizeClaimAmountPayload 定形「形成索赔金额」。
func CanonicalizeClaimAmountPayload(kind int64, claimItem, responsibility, legalEntity, originalCharge, currency string, amountMinor int64, period string, formedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Kind             int64  `json:"kind"`
		ClaimItem        string `json:"claim_item"`
		Responsibility   string `json:"responsibility"`
		LegalEntity      string `json:"legal_entity"`
		OriginalCharge   string `json:"original_charge"`
		Currency         string `json:"currency"`
		AmountMinor      int64  `json:"amount_minor"`
		Period           string `json:"period"`
		FormedAt         string `json:"formed_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "FORM_CLAIM_AMOUNT",
		Kind:             kind,
		ClaimItem:        claimItem,
		Responsibility:   responsibility,
		LegalEntity:      legalEntity,
		OriginalCharge:   originalCharge,
		Currency:         currency,
		AmountMinor:      amountMinor,
		Period:           period,
		FormedAt:         canonicalInstant(formedAt),
	})
}

// CanonicalizeReceivablePayload 定形「形成应收」。
func CanonicalizeReceivablePayload(matter, responsibility, counterparty, legalEntity, currency string, amountMinor int64, formedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Matter           string `json:"matter"`
		Responsibility   string `json:"responsibility"`
		Counterparty     string `json:"counterparty"`
		LegalEntity      string `json:"legal_entity"`
		Currency         string `json:"currency"`
		AmountMinor      int64  `json:"amount_minor"`
		FormedAt         string `json:"formed_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "FORM_RECEIVABLE",
		Matter:           matter,
		Responsibility:   responsibility,
		Counterparty:     counterparty,
		LegalEntity:      legalEntity,
		Currency:         currency,
		AmountMinor:      amountMinor,
		FormedAt:         canonicalInstant(formedAt),
	})
}

// CanonicalizeAcknowledgeReceivablePayload 定形「认可应收」。
func CanonicalizeAcknowledgeReceivablePayload(receivable, response string, standing, acknowledgedMinor int64, acknowledgedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization  string `json:"canonicalization"`
		Face              string `json:"face"`
		Receivable        string `json:"receivable"`
		Response          string `json:"response"`
		Standing          int64  `json:"standing"`
		AcknowledgedMinor int64  `json:"acknowledged_minor"`
		AcknowledgedAt    string `json:"acknowledged_at"`
	}{
		Canonicalization:  payloadCanonicalizationVersion,
		Face:              "ACKNOWLEDGE_RECEIVABLE",
		Receivable:        receivable,
		Response:          response,
		Standing:          standing,
		AcknowledgedMinor: acknowledgedMinor,
		AcknowledgedAt:    canonicalInstant(acknowledgedAt),
	})
}

// CanonicalizeAdjustClaimAmountPayload 定形「调整索赔金额」。
func CanonicalizeAdjustClaimAmountPayload(targetKind int64, target string, reason int64, basis string, direction int64, currency string, amountMinor int64, period string, formedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		TargetKind       int64  `json:"target_kind"`
		Target           string `json:"target"`
		Reason           int64  `json:"reason"`
		Basis            string `json:"basis"`
		Direction        int64  `json:"direction"`
		Currency         string `json:"currency"`
		AmountMinor      int64  `json:"amount_minor"`
		Period           string `json:"period"`
		FormedAt         string `json:"formed_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "ADJUST_CLAIM_AMOUNT",
		TargetKind:       targetKind,
		Target:           target,
		Reason:           reason,
		Basis:            basis,
		Direction:        direction,
		Currency:         currency,
		AmountMinor:      amountMinor,
		Period:           period,
		FormedAt:         canonicalInstant(formedAt),
	})
}

// SupplierBillLineContent 是供应商账单的一行主张。提交顺序不进摘要。
type SupplierBillLineContent struct {
	Line         string
	FeeItem      string
	ClaimedMinor int64
}

// SupplierBillDirectiveContent 是对一行主张的裁决。提交顺序不进摘要。
type SupplierBillDirectiveContent struct {
	Line            string
	Classification  int64
	ExpectedVersion string
	Basis           string
}

// CanonicalizeSupplierBillPayload 定形「接收供应商账单」。
func CanonicalizeSupplierBillPayload(supplier, period, currency string, receivedAt time.Time, lines []SupplierBillLineContent, directives []SupplierBillDirectiveContent) ([]byte, string, error) {
	type lineDocument struct {
		Line         string `json:"line"`
		FeeItem      string `json:"fee_item"`
		ClaimedMinor int64  `json:"claimed_minor"`
	}
	type directiveDocument struct {
		Line            string `json:"line"`
		Classification  int64  `json:"classification"`
		ExpectedVersion string `json:"expected_version"`
		Basis           string `json:"basis"`
	}
	encodedLines := make([]lineDocument, 0, len(lines))
	for _, line := range lines {
		encodedLines = append(encodedLines, lineDocument{Line: line.Line, FeeItem: line.FeeItem, ClaimedMinor: line.ClaimedMinor})
	}
	slices.SortFunc(encodedLines, func(left, right lineDocument) int {
		if left.Line != right.Line {
			if left.Line < right.Line {
				return -1
			}
			return 1
		}
		if left.FeeItem != right.FeeItem {
			if left.FeeItem < right.FeeItem {
				return -1
			}
			return 1
		}
		if left.ClaimedMinor < right.ClaimedMinor {
			return -1
		}
		if left.ClaimedMinor > right.ClaimedMinor {
			return 1
		}
		return 0
	})
	encodedDirectives := make([]directiveDocument, 0, len(directives))
	for _, directive := range directives {
		encodedDirectives = append(encodedDirectives, directiveDocument{
			Line: directive.Line, Classification: directive.Classification, ExpectedVersion: directive.ExpectedVersion, Basis: directive.Basis,
		})
	}
	slices.SortFunc(encodedDirectives, func(left, right directiveDocument) int {
		if left.Line != right.Line {
			if left.Line < right.Line {
				return -1
			}
			return 1
		}
		if left.Classification != right.Classification {
			if left.Classification < right.Classification {
				return -1
			}
			return 1
		}
		if left.ExpectedVersion != right.ExpectedVersion {
			if left.ExpectedVersion < right.ExpectedVersion {
				return -1
			}
			return 1
		}
		if left.Basis < right.Basis {
			return -1
		}
		if left.Basis > right.Basis {
			return 1
		}
		return 0
	})
	return marshalPayload(struct {
		Canonicalization string              `json:"canonicalization"`
		Face             string              `json:"face"`
		Supplier         string              `json:"supplier"`
		Period           string              `json:"period"`
		Currency         string              `json:"currency"`
		ReceivedAt       string              `json:"received_at"`
		Lines            []lineDocument      `json:"lines"`
		Directives       []directiveDocument `json:"directives"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "RECEIVE_SUPPLIER_BILL",
		Supplier:         supplier,
		Period:           period,
		Currency:         currency,
		ReceivedAt:       canonicalInstant(receivedAt),
		Lines:            encodedLines,
		Directives:       encodedDirectives,
	})
}

// CanonicalizeAssessAdvancePayload 定形「评估垫付」。
func CanonicalizeAssessAdvancePayload(verdict int64, obligation, fundsFact, payer, responsibility, basis, currency string, amountMinor int64, version string, judgedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Verdict          int64  `json:"verdict"`
		Obligation       string `json:"obligation"`
		FundsFact        string `json:"funds_fact"`
		Payer            string `json:"payer"`
		Responsibility   string `json:"responsibility"`
		Basis            string `json:"basis"`
		Currency         string `json:"currency"`
		AmountMinor      int64  `json:"amount_minor"`
		Version          string `json:"version"`
		JudgedAt         string `json:"judged_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "ASSESS_ADVANCE",
		Verdict:          verdict,
		Obligation:       obligation,
		FundsFact:        fundsFact,
		Payer:            payer,
		Responsibility:   responsibility,
		Basis:            basis,
		Currency:         currency,
		AmountMinor:      amountMinor,
		Version:          version,
		JudgedAt:         canonicalInstant(judgedAt),
	})
}

// CanonicalizeFormRecoveryPayload 定形「形成追偿」。
func CanonicalizeFormRecoveryPayload(assessment, customer, account string, amountMinor int64, formedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Assessment       string `json:"assessment"`
		Customer         string `json:"customer"`
		Account          string `json:"account"`
		AmountMinor      int64  `json:"amount_minor"`
		FormedAt         string `json:"formed_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "FORM_RECOVERY",
		Assessment:       assessment,
		Customer:         customer,
		Account:          account,
		AmountMinor:      amountMinor,
		FormedAt:         canonicalInstant(formedAt),
	})
}

// CanonicalizeAdjustRecoveryPayload 定形「调整追偿」。
func CanonicalizeAdjustRecoveryPayload(recovery string, reason int64, newBasis string, direction int64, currency string, amountMinor int64, period string, formedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Recovery         string `json:"recovery"`
		Reason           int64  `json:"reason"`
		NewBasis         string `json:"new_basis"`
		Direction        int64  `json:"direction"`
		Currency         string `json:"currency"`
		AmountMinor      int64  `json:"amount_minor"`
		Period           string `json:"period"`
		FormedAt         string `json:"formed_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "ADJUST_RECOVERY",
		Recovery:         recovery,
		Reason:           reason,
		NewBasis:         newBasis,
		Direction:        direction,
		Currency:         currency,
		AmountMinor:      amountMinor,
		Period:           period,
		FormedAt:         canonicalInstant(formedAt),
	})
}

// CanonicalizeAdoptFundsFactPayload 定形「采用外部资金事实」。付款人去空白后进入摘要。
func CanonicalizeAdoptFundsFactPayload(source, payer string, kind int64, currency string, amountMinor int64, version string, occurredAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Source           string `json:"source"`
		Payer            string `json:"payer"`
		Kind             int64  `json:"kind"`
		Currency         string `json:"currency"`
		AmountMinor      int64  `json:"amount_minor"`
		Version          string `json:"version"`
		OccurredAt       string `json:"occurred_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "ADOPT_FUNDS_FACT",
		Source:           source,
		Payer:            strings.TrimSpace(payer),
		Kind:             kind,
		Currency:         currency,
		AmountMinor:      amountMinor,
		Version:          version,
		OccurredAt:       canonicalInstant(occurredAt),
	})
}

// CanonicalizeCorrectFundsFactPayload 定形「更正外部资金事实」。来源、付款人、种类、币种
// 与发生时刻从链头照抄，不进这一口的摘要。
func CanonicalizeCorrectFundsFactPayload(corrects, version string, amountMinor int64, correctedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Corrects         string `json:"corrects"`
		Version          string `json:"version"`
		AmountMinor      int64  `json:"amount_minor"`
		CorrectedAt      string `json:"corrected_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "CORRECT_FUNDS_FACT",
		Corrects:         corrects,
		Version:          version,
		AmountMinor:      amountMinor,
		CorrectedAt:      canonicalInstant(correctedAt),
	})
}

// CanonicalizeMapFundsPayload 定形「映射资金」。
func CanonicalizeMapFundsPayload(fact string, targetKind int64, target, basis string, mappedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Fact             string `json:"fact"`
		TargetKind       int64  `json:"target_kind"`
		Target           string `json:"target"`
		Basis            string `json:"basis"`
		MappedAt         string `json:"mapped_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "MAP_FUNDS",
		Fact:             fact,
		TargetKind:       targetKind,
		Target:           target,
		Basis:            basis,
		MappedAt:         canonicalInstant(mappedAt),
	})
}

// SettlementAllocationContent 是一次核销里的一条分配。分配行保留提交顺序。
type SettlementAllocationContent struct {
	Mapping     string
	TargetKind  int64
	Target      string
	Direction   int64
	AmountMinor int64
}

// CanonicalizeApplySettlementPayload 定形「核销」。映射引用排序；分配行保留提交顺序，
// 与原先的内容判据一致。
func CanonicalizeApplySettlementPayload(fact, targetCurrency, basis string, appliedAt time.Time, mappings []string, allocations []SettlementAllocationContent) ([]byte, string, error) {
	type allocationDocument struct {
		Mapping     string `json:"mapping"`
		TargetKind  int64  `json:"target_kind"`
		Target      string `json:"target"`
		Direction   int64  `json:"direction"`
		AmountMinor int64  `json:"amount_minor"`
	}
	encoded := make([]allocationDocument, 0, len(allocations))
	for _, allocation := range allocations {
		encoded = append(encoded, allocationDocument{
			Mapping: allocation.Mapping, TargetKind: allocation.TargetKind, Target: allocation.Target,
			Direction: allocation.Direction, AmountMinor: allocation.AmountMinor,
		})
	}
	return marshalPayload(struct {
		Canonicalization string               `json:"canonicalization"`
		Face             string               `json:"face"`
		Fact             string               `json:"fact"`
		TargetCurrency   string               `json:"target_currency"`
		Basis            string               `json:"basis"`
		AppliedAt        string               `json:"applied_at"`
		Mappings         []string             `json:"mappings"`
		Allocations      []allocationDocument `json:"allocations"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "APPLY_SETTLEMENT",
		Fact:             fact,
		TargetCurrency:   targetCurrency,
		Basis:            basis,
		AppliedAt:        canonicalInstant(appliedAt),
		Mappings:         sortedStrings(mappings),
		Allocations:      encoded,
	})
}
