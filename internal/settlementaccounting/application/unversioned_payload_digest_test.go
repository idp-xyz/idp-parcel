package application_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// unversionedDigestCase 是一口在 SAC-1 之前入库的一条记录。first 首次提交并交回替身里那条记录的
// 摘要与改写它的办法；随后把摘要改写成 unversioned，再走 replay（同内容，必须答已有结果）与
// conflict（同身份异内容，必须答内容冲突）。三步共用一个替身。
//
// unversioned 是基 1121ba61 上的旧代码对 first 那条命令落下的摘要，写成定值而不在这里现算：
// 拿恢复后的函数现算，恢复得不一致也照样绿。
type unversionedDigestCase struct {
	name        string
	unversioned string
	first       func(t *testing.T) (string, func(string))
	replay      func(t *testing.T)
	conflict    func(t *testing.T)
}

// onlyDigest 交回替身里唯一那条记录的摘要，与把它改写成别的串的办法。
func onlyDigest[K comparable, V any](t *testing.T, records map[K]V, digest func(*V) *string) (string, func(string)) {
	t.Helper()
	if len(records) != 1 {
		t.Fatalf("替身里有 %d 条记录，want 1", len(records))
	}
	for key := range records {
		return keyedDigest(t, records, key, digest)
	}
	return "", nil
}

// keyedDigest 交回替身里 key 那条记录的摘要，与把它改写成别的串的办法。
func keyedDigest[K comparable, V any](t *testing.T, records map[K]V, key K, digest func(*V) *string) (string, func(string)) {
	t.Helper()
	record, found := records[key]
	if !found {
		t.Fatalf("替身里没有 %v 那条记录", key)
	}
	return *digest(&record), func(rewritten string) {
		*digest(&record) = rewritten
		records[key] = record
	}
}

func wantOutcome[O comparable](t *testing.T, got O, err error, want O) {
	t.Helper()
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got != want {
		t.Fatalf("outcome = %v, want %v", got, want)
	}
}

func unversionedDigestCases(t *testing.T) []unversionedDigestCase {
	t.Helper()
	return []unversionedDigestCase{
		allocationUnversionedCase(t),
		operatingResultUnversionedCase(t),
		assessmentUnversionedCase(t),
		recoveryUnversionedCase(t),
		recoveryAdjustmentUnversionedCase(t),
		payableUnversionedCase(t),
		creditNoteUnversionedCase(t),
		statementUnversionedCase(t),
		lateChargeInclusionUnversionedCase(t),
		adjustmentInclusionUnversionedCase(t),
		disputeUnversionedCase(t),
		fundsAdoptionUnversionedCase(t),
		fundsCorrectionUnversionedCase(t),
		fundsMappingUnversionedCase(t),
		settlementApplicationUnversionedCase(t),
		billReceptionUnversionedCase(t),
		claimAmountUnversionedCase(t),
		receivableUnversionedCase(t),
		acknowledgementUnversionedCase(t),
		claimAdjustmentUnversionedCase(t),
	}
}

// Covers: ADR-0014「规范化版本不同不是冲突」——SAC-1 之前入库的记录存的是无版本摘要，同一条命令
// 重放按无版本那一版重算再比，答已有结果；同身份换内容仍是内容冲突。比已存摘要的定形口逐口走一遍；
// 重分摊、重派生与供应商预计成本不比已存摘要，不在此列。
func TestRecordsStoredBeforeSAC1AreComparedUnderTheirOwnShape(t *testing.T) {
	for _, testCase := range unversionedDigestCases(t) {
		t.Run(testCase.name, func(t *testing.T) {
			_, rewrite := testCase.first(t)
			rewrite(testCase.unversioned)
			testCase.replay(t)
			testCase.conflict(t)
		})
	}
}

// Covers: 新记录写的是 SAC-1 形——无版本那一版只用来比旧记录，误写进新记录时上一例照样绿，
// 要在这里断。
func TestNewRecordsAreStoredUnderSAC1(t *testing.T) {
	for _, testCase := range unversionedDigestCases(t) {
		t.Run(testCase.name, func(t *testing.T) {
			if stored, _ := testCase.first(t); !strings.HasPrefix(stored, "SAC-1:") {
				t.Fatalf("新记录落下的摘要是 %q，want SAC-1 形", stored)
			}
		})
	}
}

// Covers: 认不出的形状版本不答冲突——按该处读失败的既有答复作答（分摊口是存储未决）。
func TestAnUnknownStoredShapeIsUndecidedNotAConflict(t *testing.T) {
	fixture := newOperatingFixture(t)
	command := allocateCommand(t)
	first, err := fixture.handler.Allocate(context.Background(), command)
	wantOutcome(t, first.Outcome(), err, application.CostAllocated)
	_, rewrite := onlyDigest(t, fixture.allocations.records,
		func(record *ports.AllocationRecord) *string { return &record.ContentDigest })
	rewrite("SAC-9:0000000000000000000000000000000000000000000000000000000000000000")

	result, err := fixture.handler.Allocate(context.Background(), command)
	wantOutcome(t, result.Outcome(), err, application.OperatingUndecided)
	if result.UndecidedReason() != application.AllocationStoreUnavailable {
		t.Fatalf("reason = %q", result.UndecidedReason())
	}
}

func allocationUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newOperatingFixture(t)
	command := allocateCommand(t)
	return unversionedDigestCase{
		name:        "ALLOCATE_COST",
		unversioned: "15662f4ee2286a4d8c3923075271ed8e84de4432fcfa9591c59286cbd1e9402b",
		first: func(t *testing.T) (string, func(string)) {
			result, err := fixture.handler.Allocate(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.CostAllocated)
			return onlyDigest(t, fixture.allocations.records,
				func(record *ports.AllocationRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.Allocate(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.AllocationExistingResult)
		},
		conflict: func(t *testing.T) {
			changed := allocateCommand(t)
			changed.Portions[0].AmountMinor = 5000
			result, err := fixture.handler.Allocate(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.AllocationConflict)
		},
	}
}

func operatingResultUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newOperatingFixture(t)
	command := deriveCommand(t)
	return unversionedDigestCase{
		name:        "DERIVE_RESULT",
		unversioned: "f7314010695cc3270435a0718b9aacd9a22407f648906e320320dd47e1382549",
		first: func(t *testing.T) (string, func(string)) {
			result, err := fixture.handler.Derive(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.ResultDerived)
			return onlyDigest(t, fixture.results.records,
				func(record *ports.OperatingResultRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.Derive(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.ResultExistingResult)
		},
		conflict: func(t *testing.T) {
			changed := deriveCommand(t)
			changed.Components[0].AmountMinor = 21000
			result, err := fixture.handler.Derive(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.ResultConflict)
		},
	}
}

func assessmentUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newAdvanceFixture(t)
	command := assessCommand(t, domain.AdvanceEstablished)
	return unversionedDigestCase{
		name:        "ASSESS_ADVANCE",
		unversioned: "3c04a71528b39dcb83901221810a7376b26e27ec7e2c1fc5302d4f2489a8280d",
		first: func(t *testing.T) (string, func(string)) {
			result, err := fixture.handler.Assess(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.AdvanceAssessed)
			return onlyDigest(t, fixture.assessments.records,
				func(record *ports.AdvanceAssessmentRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.Assess(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.AssessmentExistingResult)
		},
		conflict: func(t *testing.T) {
			result, err := fixture.handler.Assess(context.Background(), assessCommand(t, domain.AdvanceNotEstablishedVerdict))
			wantOutcome(t, result.Outcome(), err, application.AssessmentConflict)
		},
	}
}

func recoveryUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newAdvanceFixture(t)
	command := formRecoveryCommand(t)
	return unversionedDigestCase{
		name:        "FORM_RECOVERY",
		unversioned: "d288381ef09f3dc09f875c2799f71bb3da84b09001b58ac2633dac8bce43c570",
		first: func(t *testing.T) (string, func(string)) {
			assessed, err := fixture.handler.Assess(context.Background(), assessCommand(t, domain.AdvanceEstablished))
			wantOutcome(t, assessed.Outcome(), err, application.AdvanceAssessed)
			result, err := fixture.handler.FormRecovery(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.RecoveryFormed)
			return onlyDigest(t, fixture.recoveries.records,
				func(record *ports.AdvanceRecoveryRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.FormRecovery(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.RecoveryExistingResult)
		},
		conflict: func(t *testing.T) {
			changed := formRecoveryCommand(t)
			changed.AmountMinor = 4000
			result, err := fixture.handler.FormRecovery(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.RecoveryConflict)
		},
	}
}

func recoveryAdjustmentUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newAdvanceFixture(t)
	tenant, _ := domain.NewTenantID("tenant-1")
	command := application.AdjustRecoveryCommand{
		TenantID:    tenant,
		Adjustment:  "adjustment-1",
		Recovery:    "recovery-1",
		Reason:      domain.TaxAssessmentCorrected,
		NewBasis:    "assessment-basis-corrected",
		Direction:   domain.AdjustmentCredit,
		Currency:    "USD",
		AmountMinor: 700,
		Period:      "2026-09",
		FormedAt:    advanceFormedAt.Add(time.Hour),
	}
	return unversionedDigestCase{
		name:        "ADJUST_RECOVERY",
		unversioned: "7f287a93ee8547517177c038e58c5805b2aa93c94dadbb7c157e531ed18362d0",
		first: func(t *testing.T) (string, func(string)) {
			assessed, err := fixture.handler.Assess(context.Background(), assessCommand(t, domain.AdvanceEstablished))
			wantOutcome(t, assessed.Outcome(), err, application.AdvanceAssessed)
			formed, err := fixture.handler.FormRecovery(context.Background(), formRecoveryCommand(t))
			wantOutcome(t, formed.Outcome(), err, application.RecoveryFormed)
			result, err := fixture.handler.Adjust(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.AdjustmentFormed)
			return onlyDigest(t, fixture.adjustments.records,
				func(record *ports.RecoveryAdjustmentRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.Adjust(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.AdjustmentExistingResult)
		},
		conflict: func(t *testing.T) {
			changed := command
			changed.AmountMinor = 900
			result, err := fixture.handler.Adjust(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.AdjustmentConflict)
		},
	}
}

func payableUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newBillFixture(t)
	command := auditCommand(t, "bill-1", "line-1", "payable-1")
	return unversionedDigestCase{
		name:        "AUDIT_BILL_LINE",
		unversioned: "5fa58c60604d8de4953108fcd4d4afa2045e7d467f0ec6fc5cee24d7bce77ede",
		first: func(t *testing.T) (string, func(string)) {
			received, err := fixture.handler.Handle(context.Background(), billCommand(t, "bill-1"))
			wantOutcome(t, received.Outcome(), err, application.BillReceived)
			result, err := fixture.handler.Audit(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.PayableFormed)
			return onlyDigest(t, fixture.payables.records,
				func(record *ports.AuditedPayableRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.Audit(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.PayableExistingResult)
		},
		conflict: func(t *testing.T) {
			received, err := fixture.handler.Handle(context.Background(), billCommand(t, "bill-2"))
			wantOutcome(t, received.Outcome(), err, application.BillReceived)
			result, err := fixture.handler.Audit(context.Background(), auditCommand(t, "bill-2", "line-1", "payable-1"))
			wantOutcome(t, result.Outcome(), err, application.PayableConflict)
		},
	}
}

func creditNoteUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newBillFixture(t)
	command := creditCommand(t, "note-1", "payable-1", 2000)
	return unversionedDigestCase{
		name:        "FORM_SUPPLIER_CREDIT_NOTE",
		unversioned: "f6185f2e1fa8f6e803f95937970844d484cdd7da2ccbe5075b1c4b6f10ddd141",
		first: func(t *testing.T) (string, func(string)) {
			received, err := fixture.handler.Handle(context.Background(), billCommand(t, "bill-1"))
			wantOutcome(t, received.Outcome(), err, application.BillReceived)
			audited, err := fixture.handler.Audit(context.Background(), auditCommand(t, "bill-1", "line-1", "payable-1"))
			wantOutcome(t, audited.Outcome(), err, application.PayableFormed)
			result, err := fixture.handler.Credit(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.CreditNoteFormed)
			return onlyDigest(t, fixture.creditNotes.records,
				func(record *ports.SupplierCreditNoteRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.Credit(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.CreditNoteExistingResult)
		},
		conflict: func(t *testing.T) {
			result, err := fixture.handler.Credit(context.Background(), creditCommand(t, "note-1", "payable-1", 2500))
			wantOutcome(t, result.Outcome(), err, application.CreditNoteConflict)
		},
	}
}

func statementUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newStatementFixture(t)
	command := publishCommand(t)
	return unversionedDigestCase{
		name:        "PUBLISH_STATEMENT",
		unversioned: "e857f1cfa1186eb4ec5a5e6565b8d5f6ce3180b888729f80838d1650c612fd69",
		first: func(t *testing.T) (string, func(string)) {
			result, err := fixture.handler.Publish(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.StatementPublished)
			return onlyDigest(t, fixture.statements.records,
				func(record *ports.StatementRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.Publish(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.StatementExistingResult)
		},
		conflict: func(t *testing.T) {
			changed := publishCommand(t)
			changed.ChargeIDs = []string{"charge-A"}
			changed.DeclaredTotalMinor = 12000
			result, err := fixture.handler.Publish(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.StatementConflict)
		},
	}
}

func lateChargeInclusionUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newStatementFixture(t)
	tenant, _ := domain.NewTenantID("tenant-1")
	command := application.IncludeLateChargeCommand{
		TenantID:         tenant,
		Inclusion:        "inclusion-1",
		Number:           "STMT-2026-08-001",
		ChargeID:         "charge-late",
		SubsequentPeriod: "2026-09",
		IncludedAt:       statementPublishAt.Add(48 * time.Hour),
	}
	return unversionedDigestCase{
		name:        "INCLUDE_LATE_CHARGE",
		unversioned: "5abc490655d4b5a1311ccb24a23202e4ab9d08b02e2868271bebac85446c0e75",
		first: func(t *testing.T) (string, func(string)) {
			published, err := fixture.handler.Publish(context.Background(), publishCommand(t))
			wantOutcome(t, published.Outcome(), err, application.StatementPublished)
			result, err := fixture.handler.IncludeLateCharge(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.LateChargeIncluded)
			return onlyDigest(t, fixture.inclusions.records,
				func(record *ports.InclusionRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.IncludeLateCharge(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.InclusionExistingResult)
		},
		conflict: func(t *testing.T) {
			changed := command
			changed.SubsequentPeriod = "2026-10"
			result, err := fixture.handler.IncludeLateCharge(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.InclusionConflict)
		},
	}
}

func adjustmentInclusionUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newStatementFixture(t)
	command := application.IncludeAdjustmentCommand{
		TenantID:         billValue(t, domain.NewTenantID, "tenant-1"),
		Inclusion:        "inclusion-adj-1",
		Number:           "STMT-2026-08-001",
		AdjustmentID:     "adj-1",
		SubsequentPeriod: "2026-09",
		IncludedAt:       statementPublishAt.Add(48 * time.Hour),
	}
	return unversionedDigestCase{
		name:        "INCLUDE_ADJUSTMENT",
		unversioned: "d55c46db4a6e4bf1bc43ead63e54235231a0d6a8fb2b8c7fa8210fe68d9da263",
		first: func(t *testing.T) (string, func(string)) {
			published, err := fixture.handler.Publish(context.Background(), publishCommand(t))
			wantOutcome(t, published.Outcome(), err, application.StatementPublished)
			fixture.adjustments.records["tenant-1|adj-1"] = pricingCorrectionOn(t, "adj-1", "charge-A", statementPublishAt.Add(36*time.Hour))
			result, err := fixture.handler.IncludeAdjustment(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.AdjustmentIncluded)
			return onlyDigest(t, fixture.inclusions.records,
				func(record *ports.InclusionRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.IncludeAdjustment(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.InclusionExistingResult)
		},
		conflict: func(t *testing.T) {
			changed := command
			changed.SubsequentPeriod = "2026-10"
			result, err := fixture.handler.IncludeAdjustment(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.InclusionConflict)
		},
	}
}

func disputeUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newStatementFixture(t)
	tenant, _ := domain.NewTenantID("tenant-1")
	command := application.OpenDisputeCommand{
		TenantID:      tenant,
		Dispute:       "dispute-1",
		Number:        "STMT-2026-08-001",
		ChargeID:      "charge-A",
		DisputedMinor: 5000,
		Reason:        "service-not-rendered",
		OpenedAt:      statementPublishAt.Add(24 * time.Hour),
	}
	return unversionedDigestCase{
		name:        "OPEN_DISPUTE",
		unversioned: "a76130a7dfa1787454306257c6baa68ea42b04f0597cbff8d24560d875e55bee",
		first: func(t *testing.T) (string, func(string)) {
			published, err := fixture.handler.Publish(context.Background(), publishCommand(t))
			wantOutcome(t, published.Outcome(), err, application.StatementPublished)
			result, err := fixture.handler.OpenDispute(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.DisputeOpened)
			return onlyDigest(t, fixture.disputes.records,
				func(record *ports.DisputeRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.OpenDispute(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.DisputeExistingResult)
		},
		conflict: func(t *testing.T) {
			changed := command
			changed.DisputedMinor = 4000
			result, err := fixture.handler.OpenDispute(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.DisputeConflict)
		},
	}
}

func fundsAdoptionUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newFundsFixture(t)
	command := adoptCommand(t, domain.FundsReceiptConfirmed)
	return unversionedDigestCase{
		name:        "ADOPT_FUNDS_FACT",
		unversioned: "b83ff6642e37bb036ed1775f1af7af72437e2051eb566187e6a4d84be9c159d3",
		first: func(t *testing.T) (string, func(string)) {
			result, err := fixture.handler.AdoptFact(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.FundsFactAdopted)
			return onlyDigest(t, fixture.facts.records,
				func(record *ports.FundsFactRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.AdoptFact(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.FundsFactExisting)
		},
		conflict: func(t *testing.T) {
			changed := command
			changed.AmountMinor = 21000
			result, err := fixture.handler.AdoptFact(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.FundsFactConflict)
		},
	}
}

// fundsCorrectionUnversionedCase 改写的是更正落下的那一版（v2）：首版 v1 是采用口的记录，另有一例。
func fundsCorrectionUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newFundsFixture(t)
	command := correctCommand(t)
	return unversionedDigestCase{
		name:        "CORRECT_FUNDS_FACT",
		unversioned: "016f06e8722d43b91c54916d005735348c50aa6b36d7d6515aff9fec13db3c5b",
		first: func(t *testing.T) (string, func(string)) {
			adoptFirstVersion(t, fixture)
			result, err := fixture.handler.CorrectFact(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.FundsFactAdopted)
			key := ports.FundsFactKey{TenantID: command.TenantID, Fact: saFact(t, command.Fact)}
			return keyedDigest(t, fixture.facts.records, fundsFactVersionKey(key, saVersion(t, command.Version)),
				func(record *ports.FundsFactRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.CorrectFact(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.FundsFactExisting)
		},
		conflict: func(t *testing.T) {
			changed := command
			changed.AmountMinor = 17000
			result, err := fixture.handler.CorrectFact(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.FundsFactConflict)
		},
	}
}

func fundsMappingUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newFundsFixture(t)
	command := mapCommand(t)
	return unversionedDigestCase{
		name:        "MAP_FUNDS",
		unversioned: "91281b5a0afd59c30b7bb0d15d8c5ef8a38c3fa5a04607a17a9c0a22bdad66e1",
		first: func(t *testing.T) (string, func(string)) {
			adopted, err := fixture.handler.AdoptFact(context.Background(), adoptCommand(t, domain.FundsReceiptConfirmed))
			wantOutcome(t, adopted.Outcome(), err, application.FundsFactAdopted)
			result, err := fixture.handler.Map(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.FundsMapped)
			return onlyDigest(t, fixture.mappings.records,
				func(record *ports.FundsMappingRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.Map(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.FundsMappingExisting)
		},
		conflict: func(t *testing.T) {
			changed := mapCommand(t)
			changed.Basis = "payment-instruction-2"
			result, err := fixture.handler.Map(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.FundsMappingConflict)
		},
	}
}

func settlementApplicationUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newFundsFixture(t)
	command := applyCommand(t)
	return unversionedDigestCase{
		name:        "APPLY_SETTLEMENT",
		unversioned: "27492e51a0794bb9e511cb5def943bcf8850b567c0a023716881d832dfb887f5",
		first: func(t *testing.T) (string, func(string)) {
			adopted, err := fixture.handler.AdoptFact(context.Background(), adoptCommand(t, domain.FundsReceiptConfirmed))
			wantOutcome(t, adopted.Outcome(), err, application.FundsFactAdopted)
			mapped, err := fixture.handler.Map(context.Background(), mapCommand(t))
			wantOutcome(t, mapped.Outcome(), err, application.FundsMapped)
			result, err := fixture.handler.Apply(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.SettlementApplied)
			return onlyDigest(t, fixture.applications.records,
				func(record *ports.SettlementApplicationRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.Apply(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.ApplicationExisting)
		},
		conflict: func(t *testing.T) {
			changed := applyCommand(t)
			changed.Basis = "unique-match-evidence-2"
			result, err := fixture.handler.Apply(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.ApplicationConflict)
		},
	}
}

func billReceptionUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newBillFixture(t)
	command := billCommand(t, "bill-1")
	return unversionedDigestCase{
		name:        "RECEIVE_SUPPLIER_BILL",
		unversioned: "a50240ce7b056c532b14b0ad7e6c4c32e48285f452cc41732cf42200064c2cc9",
		first: func(t *testing.T) (string, func(string)) {
			result, err := fixture.handler.Handle(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.BillReceived)
			return onlyDigest(t, fixture.store.records,
				func(record *ports.BillReceptionRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.Handle(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.BillExistingResult)
		},
		conflict: func(t *testing.T) {
			changed := billCommand(t, "bill-1")
			changed.Claim.Lines[0].ClaimedMinor = 99999
			result, err := fixture.handler.Handle(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.BillVersionConflict)
		},
	}
}

func claimAmountUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newClaimFixture(t)
	command := formClaimAmountCommand(t)
	return unversionedDigestCase{
		name:        "FORM_CLAIM_AMOUNT",
		unversioned: "a771cae041ddca13c2367b16aa1be5908afd1241d7fa42af913f2925aed08e61",
		first: func(t *testing.T) (string, func(string)) {
			result, err := fixture.handler.FormClaimAmount(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.ClaimAmountFormed)
			return onlyDigest(t, fixture.amounts.records,
				func(record *ports.ClaimAmountRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.FormClaimAmount(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.ClaimAmountExisting)
		},
		conflict: func(t *testing.T) {
			changed := formClaimAmountCommand(t)
			changed.AmountMinor = 9500
			result, err := fixture.handler.FormClaimAmount(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.ClaimAmountConflict)
		},
	}
}

func receivableUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newClaimFixture(t)
	command := formReceivableCommand(t)
	return unversionedDigestCase{
		name:        "FORM_RECEIVABLE",
		unversioned: "ccb16746cb74863a6772725dd68a4ad3a8ac4c2d901cefda0431a9df45ca913d",
		first: func(t *testing.T) (string, func(string)) {
			result, err := fixture.handler.FormReceivable(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.ReceivableFormed)
			return onlyDigest(t, fixture.receivables.records,
				func(record *ports.ReceivableRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.FormReceivable(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.ReceivableExisting)
		},
		conflict: func(t *testing.T) {
			changed := formReceivableCommand(t)
			changed.AmountMinor = 7500
			result, err := fixture.handler.FormReceivable(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.ReceivableConflict)
		},
	}
}

func acknowledgementUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newClaimFixture(t)
	tenant, _ := domain.NewTenantID("tenant-1")
	command := application.AcknowledgeCommand{
		TenantID:          tenant,
		Acknowledgement:   "acknowledgement-1",
		Receivable:        "receivable-1",
		Response:          "carrier-response-1",
		Standing:          domain.ResponsePartiallyAccepted,
		AcknowledgedMinor: 4000,
		AcknowledgedAt:    claimFormedAt.Add(24 * time.Hour),
	}
	return unversionedDigestCase{
		name:        "ACKNOWLEDGE_RECEIVABLE",
		unversioned: "7cb173d8d0d5fbb653707b3b3ee21042b261cd7ad9bccf9119e6af7f29ecbe7f",
		first: func(t *testing.T) (string, func(string)) {
			formed, err := fixture.handler.FormReceivable(context.Background(), formReceivableCommand(t))
			wantOutcome(t, formed.Outcome(), err, application.ReceivableFormed)
			result, err := fixture.handler.Acknowledge(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.RecoveryAcknowledged)
			return onlyDigest(t, fixture.acknowledgements.records,
				func(record *ports.AcknowledgementRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.Acknowledge(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.AcknowledgementExisting)
		},
		conflict: func(t *testing.T) {
			changed := command
			changed.AcknowledgedMinor = 5000
			result, err := fixture.handler.Acknowledge(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.AcknowledgementConflict)
		},
	}
}

func claimAdjustmentUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newClaimFixture(t)
	tenant, _ := domain.NewTenantID("tenant-1")
	command := application.AdjustClaimAmountCommand{
		TenantID:    tenant,
		Adjustment:  "claim-adjustment-1",
		TargetKind:  domain.AdjustsCustomerClaimAmount,
		Target:      "claim-amount-1",
		Reason:      domain.ResponsibilityRevised,
		Basis:       "responsibility-conclusion-2",
		Direction:   domain.AdjustmentCredit,
		Currency:    "USD",
		AmountMinor: 1500,
		Period:      "2026-10",
		FormedAt:    claimFormedAt.Add(48 * time.Hour),
	}
	return unversionedDigestCase{
		name:        "ADJUST_CLAIM_AMOUNT",
		unversioned: "d2398381b7310df7b7ef3cbe8783f5f3aaee38456d422d1cbe74dee7fc29d9a6",
		first: func(t *testing.T) (string, func(string)) {
			formed, err := fixture.handler.FormClaimAmount(context.Background(), formClaimAmountCommand(t))
			wantOutcome(t, formed.Outcome(), err, application.ClaimAmountFormed)
			result, err := fixture.handler.Adjust(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.ClaimAdjustmentFormed)
			return onlyDigest(t, fixture.adjustments.records,
				func(record *ports.ClaimAdjustmentRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := fixture.handler.Adjust(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.ClaimAdjustmentExisting)
		},
		conflict: func(t *testing.T) {
			changed := command
			changed.AmountMinor = 1800
			result, err := fixture.handler.Adjust(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.ClaimAdjustmentConflict)
		},
	}
}
