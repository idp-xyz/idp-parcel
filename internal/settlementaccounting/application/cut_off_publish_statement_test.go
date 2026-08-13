package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	statementCutOffAt  = time.Date(2026, 8, 31, 23, 59, 0, 0, time.UTC)
	statementPublishAt = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	statementNowAt     = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
)

type statementStoreDouble struct {
	records map[string]ports.StatementRecord
	findErr error
}

func newStatementStore() *statementStoreDouble {
	return &statementStoreDouble{records: map[string]ports.StatementRecord{}}
}

func statementKey(key ports.StatementKey) string {
	return key.TenantID.String() + "|" + key.Number.String()
}

func (double *statementStoreDouble) FindByKey(
	_ context.Context,
	key ports.StatementKey,
) (ports.StatementRecord, bool, error) {
	if double.findErr != nil {
		return ports.StatementRecord{}, false, double.findErr
	}
	record, found := double.records[statementKey(key)]
	return record, found, nil
}

func (double *statementStoreDouble) Save(
	_ context.Context,
	record ports.StatementRecord,
) (ports.StatementSaveOutcome, error) {
	if _, exists := double.records[statementKey(record.Key)]; exists {
		return ports.StatementAlreadyPublished, nil
	}
	double.records[statementKey(record.Key)] = record
	return ports.StatementSaved, nil
}

func (double *statementStoreDouble) Replace(
	_ context.Context,
	record ports.StatementRecord,
) (bool, error) {
	if _, exists := double.records[statementKey(record.Key)]; !exists {
		return false, nil
	}
	double.records[statementKey(record.Key)] = record
	return true, nil
}

type inclusionStoreDouble struct {
	records map[string]ports.InclusionRecord
	findErr error
}

func newInclusionStore() *inclusionStoreDouble {
	return &inclusionStoreDouble{records: map[string]ports.InclusionRecord{}}
}

func inclusionKey(key ports.InclusionKey) string {
	return key.TenantID.String() + "|" + key.Inclusion.String()
}

func (double *inclusionStoreDouble) FindByKey(
	_ context.Context,
	key ports.InclusionKey,
) (ports.InclusionRecord, bool, error) {
	if double.findErr != nil {
		return ports.InclusionRecord{}, false, double.findErr
	}
	record, found := double.records[inclusionKey(key)]
	return record, found, nil
}

func (double *inclusionStoreDouble) Save(
	_ context.Context,
	record ports.InclusionRecord,
) (ports.InclusionSaveOutcome, error) {
	if _, exists := double.records[inclusionKey(record.Key)]; exists {
		return ports.InclusionAlreadyRecorded, nil
	}
	double.records[inclusionKey(record.Key)] = record
	return ports.InclusionSaved, nil
}

type disputeStoreDouble struct {
	records map[string]ports.DisputeRecord
	findErr error
}

func newDisputeStore() *disputeStoreDouble {
	return &disputeStoreDouble{records: map[string]ports.DisputeRecord{}}
}

func disputeKey(key ports.DisputeKey) string {
	return key.TenantID.String() + "|" + key.Dispute.String()
}

func (double *disputeStoreDouble) FindByKey(
	_ context.Context,
	key ports.DisputeKey,
) (ports.DisputeRecord, bool, error) {
	if double.findErr != nil {
		return ports.DisputeRecord{}, false, double.findErr
	}
	record, found := double.records[disputeKey(key)]
	return record, found, nil
}

func (double *disputeStoreDouble) Save(
	_ context.Context,
	record ports.DisputeRecord,
) (ports.DisputeSaveOutcome, error) {
	if _, exists := double.records[disputeKey(record.Key)]; exists {
		return ports.DisputeAlreadyOpened, nil
	}
	double.records[disputeKey(record.Key)] = record
	return ports.DisputeSaved, nil
}

func (double *disputeStoreDouble) Replace(
	_ context.Context,
	record ports.DisputeRecord,
) (bool, error) {
	if _, exists := double.records[disputeKey(record.Key)]; !exists {
		return false, nil
	}
	double.records[disputeKey(record.Key)] = record
	return true, nil
}

type statementHandoffDouble struct {
	intents []ports.StatementIntent
	err     error
}

func (double *statementHandoffDouble) HandOffStatement(
	_ context.Context,
	intent ports.StatementIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type statementClock struct{ at time.Time }

func (clock statementClock) Now() time.Time { return clock.at }

type statementFixture struct {
	statements *statementStoreDouble
	charges    *chargeStoreDouble
	inclusions *inclusionStoreDouble
	disputes   *disputeStoreDouble
	handoff    *statementHandoffDouble
	handler    *application.CutOffPublishStatementHandler
}

func newStatementFixture(t *testing.T) *statementFixture {
	t.Helper()
	fixture := &statementFixture{
		statements: newStatementStore(),
		charges:    newChargeStore(),
		inclusions: newInclusionStore(),
		disputes:   newDisputeStore(),
		handoff:    &statementHandoffDouble{},
	}
	fixture.handler = application.NewCutOffPublishStatementHandler(application.CutOffPublishStatementDeps{
		Statements: fixture.statements,
		Charges:    fixture.charges,
		Inclusions: fixture.inclusions,
		Disputes:   fixture.disputes,
		Downstream: fixture.handoff,
		Clock:      statementClock{at: statementNowAt},
	})
	fixture.charges.charges["charge-A"] = confirmedStatementCharge(t, "charge-A", 12000)
	fixture.charges.charges["charge-B"] = confirmedStatementCharge(t, "charge-B", 8000)
	fixture.charges.charges["charge-late"] = confirmedStatementCharge(t, "charge-late", 3000)
	fixture.charges.charges["charge-estimated"] = estimatedOnlyCharge(t, "charge-estimated", 500)
	return fixture
}

func confirmedStatementCharge(t *testing.T, id string, amount int64) domain.CustomerCharge {
	t.Helper()
	charge, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID:          billValue(t, domain.NewCustomerChargeID, id),
		FeeItem:     billValue(t, domain.NewFeeItemReference, "fee-freight"),
		Evaluation:  billValue(t, domain.NewSellEvaluationReference, "sell-evaluation-1"),
		Currency:    billValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor: amount,
		Stage:       domain.ChargeProvisional,
		FormedAt:    statementCutOffAt.Add(-72 * time.Hour),
	})
	if err != nil {
		t.Fatalf("form charge: %v", err)
	}
	confirmed, err := charge.Confirm(
		billValue(t, domain.NewConfirmationBasisReference, "delivery-confirmed-"+id),
		statementCutOffAt.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("confirm charge: %v", err)
	}
	return confirmed
}

func estimatedOnlyCharge(t *testing.T, id string, amount int64) domain.CustomerCharge {
	t.Helper()
	charge, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID:          billValue(t, domain.NewCustomerChargeID, id),
		FeeItem:     billValue(t, domain.NewFeeItemReference, "fee-freight"),
		Evaluation:  billValue(t, domain.NewSellEvaluationReference, "sell-evaluation-1"),
		Currency:    billValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor: amount,
		Stage:       domain.ChargeEstimated,
		FormedAt:    statementCutOffAt.Add(-72 * time.Hour),
	})
	if err != nil {
		t.Fatalf("form estimated charge: %v", err)
	}
	return charge
}

func publishCommand(t *testing.T) application.PublishStatementCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.PublishStatementCommand{
		TenantID:           tenant,
		Number:             "STMT-2026-08-001",
		Account:            "settlement-account-1",
		Period:             "2026-08",
		Version:            "draft/v1",
		Currency:           "USD",
		ChargeIDs:          []string{"charge-A", "charge-B"},
		DeclaredTotalMinor: 20000,
		CutOffAt:           statementCutOffAt,
		PublishedAt:        statementPublishAt,
	}
}

// Covers: `AT-SA-065`「申报总额与行勾稽不平即阻断，不修正为约等于」与 UC-SA-003
// 「截单快照只纳已确认费用」的编排面——费用从库读回（未确认 → CHARGE_NOT_CONFIRMED
// 业务负向）；同一单号只发布一次，重放返原、异集合冲突（替代走作废+新单号）。
func TestAStatementPublishesOnlyConfirmedCharges(t *testing.T) {
	fixture := newStatementFixture(t)
	command := publishCommand(t)

	first, err := fixture.handler.Publish(context.Background(), command)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if first.Outcome() != application.StatementPublished {
		t.Fatalf("outcome = %q, want STATEMENT_PUBLISHED", first.Outcome())
	}
	record, _ := first.Statement()
	// 总额锚定本夹具：12000+8000（USD）。
	if record.Statement.TotalMinor() != 20000 {
		t.Fatalf("total = %d, want 20000", record.Statement.TotalMinor())
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(fixture.handoff.intents))
	}

	t.Run("an imbalance blocks the publication", func(t *testing.T) {
		imbalanced := publishCommand(t)
		imbalanced.Number = "STMT-2026-08-002"
		imbalanced.DeclaredTotalMinor = 19999
		result, err := fixture.handler.Publish(context.Background(), imbalanced)
		if err != nil {
			t.Fatalf("publish: %v", err)
		}
		if result.Outcome() != application.StatementImbalanceOutcome {
			t.Fatalf("outcome = %q, want STATEMENT_IMBALANCE（不修正为约等于）", result.Outcome())
		}
	})

	t.Run("an estimated charge cannot enter the cut-off", func(t *testing.T) {
		estimated := publishCommand(t)
		estimated.Number = "STMT-2026-08-003"
		estimated.ChargeIDs = []string{"charge-A", "charge-estimated"}
		estimated.DeclaredTotalMinor = 12500
		result, err := fixture.handler.Publish(context.Background(), estimated)
		if err != nil {
			t.Fatalf("publish: %v", err)
		}
		if result.Outcome() != application.StatementChargeNotConfirmed {
			t.Fatalf("outcome = %q, want CHARGE_NOT_CONFIRMED（先确认再截单）", result.Outcome())
		}
	})

	t.Run("a replay returns the original statement", func(t *testing.T) {
		replay, err := fixture.handler.Publish(context.Background(), command)
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.StatementExistingResult {
			t.Fatalf("outcome = %q", replay.Outcome())
		}
		if len(fixture.handoff.intents) != 2 {
			t.Fatalf("intents = %d（重放重发同一份）", len(fixture.handoff.intents))
		}
	})

	t.Run("a different cut-off set under the same number is a conflict", func(t *testing.T) {
		flipped := publishCommand(t)
		flipped.ChargeIDs = []string{"charge-A"}
		flipped.DeclaredTotalMinor = 12000
		result, err := fixture.handler.Publish(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict publish: %v", err)
		}
		if result.Outcome() != application.StatementConflict {
			t.Fatalf("outcome = %q（替代走作废+新单号，不顶替快照）", result.Outcome())
		}
	})
}

// Covers: `AT-SA-069`「依据作废整单：原单保留、依据必备、只作废一次；替代单用新身份」
// 的编排面——已作废重放返原不二废；无单可废未受理。
func TestVoidKeepsTheOriginalOnce(t *testing.T) {
	fixture := newStatementFixture(t)
	if _, err := fixture.handler.Publish(context.Background(), publishCommand(t)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	tenant, _ := domain.NewTenantID("tenant-1")

	voided, err := fixture.handler.Void(context.Background(), application.VoidStatementCommand{
		TenantID: tenant,
		Number:   "STMT-2026-08-001",
		Basis:    "void-basis-1",
		VoidedAt: statementPublishAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("void: %v", err)
	}
	if voided.Outcome() != application.StatementVoidedOutcome {
		t.Fatalf("outcome = %q, want STATEMENT_VOIDED", voided.Outcome())
	}
	record, _ := voided.Statement()
	if _, _, isVoided := record.Statement.Voided(); !isVoided {
		t.Fatal("作废没有落在快照上")
	}

	t.Run("a second void replays to the original", func(t *testing.T) {
		again, err := fixture.handler.Void(context.Background(), application.VoidStatementCommand{
			TenantID: tenant,
			Number:   "STMT-2026-08-001",
			Basis:    "void-basis-2",
			VoidedAt: statementPublishAt.Add(2 * time.Hour),
		})
		if err != nil {
			t.Fatalf("second void: %v", err)
		}
		if again.Outcome() != application.StatementAlreadyVoided {
			t.Fatalf("outcome = %q, want ALREADY_VOIDED（只作废一次）", again.Outcome())
		}
		record, _ := again.Statement()
		basis, _, _ := record.Statement.Voided()
		if basis.String() != "void-basis-1" {
			t.Fatal("二废顶替了原作废依据")
		}
	})

	t.Run("voiding an absent statement is not accepted", func(t *testing.T) {
		result, err := fixture.handler.Void(context.Background(), application.VoidStatementCommand{
			TenantID: tenant,
			Number:   "STMT-2026-08-009",
			Basis:    "void-basis-1",
			VoidedAt: statementPublishAt.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("void absent: %v", err)
		}
		if result.Outcome() != application.StatementNotAccepted {
			t.Fatalf("outcome = %q", result.Outcome())
		}
	})
}

// Covers: `AT-SA-055` 后半「已发布对账单不改写，后到费用归后续账期并关联原账单」的
// 编排面——纳入原周期 → INCLUSION_BACKFILLS_PERIOD 业务负向；未确认的后到费用 →
// CHARGE_NOT_CONFIRMED；纳入幂等分重放/冲突并交意图。
func TestLateChargesGoToSubsequentPeriods(t *testing.T) {
	fixture := newStatementFixture(t)
	if _, err := fixture.handler.Publish(context.Background(), publishCommand(t)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	tenant, _ := domain.NewTenantID("tenant-1")
	include := application.IncludeLateChargeCommand{
		TenantID:         tenant,
		Inclusion:        "inclusion-1",
		Number:           "STMT-2026-08-001",
		ChargeID:         "charge-late",
		SubsequentPeriod: "2026-09",
		IncludedAt:       statementPublishAt.Add(48 * time.Hour),
	}

	included, err := fixture.handler.IncludeLateCharge(context.Background(), include)
	if err != nil {
		t.Fatalf("include: %v", err)
	}
	if included.Outcome() != application.LateChargeIncluded {
		t.Fatalf("outcome = %q, want LATE_CHARGE_INCLUDED", included.Outcome())
	}
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("intents = %d, want 2（纳入也交下游）", len(fixture.handoff.intents))
	}

	t.Run("backfilling the original period is refused", func(t *testing.T) {
		backfill := include
		backfill.Inclusion = "inclusion-2"
		backfill.SubsequentPeriod = "2026-08"
		result, err := fixture.handler.IncludeLateCharge(context.Background(), backfill)
		if err != nil {
			t.Fatalf("backfill: %v", err)
		}
		if result.Outcome() != application.InclusionBackfillsOutcome {
			t.Fatalf("outcome = %q, want INCLUSION_BACKFILLS_PERIOD（已发布快照不回填）", result.Outcome())
		}
	})

	t.Run("an estimated late charge cannot be included", func(t *testing.T) {
		estimated := include
		estimated.Inclusion = "inclusion-3"
		estimated.ChargeID = "charge-estimated"
		result, err := fixture.handler.IncludeLateCharge(context.Background(), estimated)
		if err != nil {
			t.Fatalf("include estimated: %v", err)
		}
		if result.Outcome() != application.StatementChargeNotConfirmed {
			t.Fatalf("outcome = %q", result.Outcome())
		}
	})

	t.Run("a replay returns the original inclusion", func(t *testing.T) {
		replay, err := fixture.handler.IncludeLateCharge(context.Background(), include)
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.InclusionExistingResult {
			t.Fatalf("outcome = %q", replay.Outcome())
		}
	})
}

// Covers: UC-SA-003「客户异议是独立对象不改写对账单」的编排面——异议限单内行、金额
// 不越行（领域把门）；终局裁定不二裁（ErrDisputeResolved 哨兵分格）、待审可再裁。
func TestDisputesAreIndependentAndResolvedOnce(t *testing.T) {
	fixture := newStatementFixture(t)
	if _, err := fixture.handler.Publish(context.Background(), publishCommand(t)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	tenant, _ := domain.NewTenantID("tenant-1")
	open := application.OpenDisputeCommand{
		TenantID:      tenant,
		Dispute:       "dispute-1",
		Number:        "STMT-2026-08-001",
		ChargeID:      "charge-A",
		DisputedMinor: 5000,
		Reason:        "service-not-rendered",
		OpenedAt:      statementPublishAt.Add(24 * time.Hour),
	}

	opened, err := fixture.handler.OpenDispute(context.Background(), open)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if opened.Outcome() != application.DisputeOpened {
		t.Fatalf("outcome = %q, want DISPUTE_OPENED", opened.Outcome())
	}

	t.Run("a dispute cannot exceed the line amount", func(t *testing.T) {
		over := open
		over.Dispute = "dispute-2"
		over.DisputedMinor = 12001
		result, err := fixture.handler.OpenDispute(context.Background(), over)
		if err != nil {
			t.Fatalf("open over: %v", err)
		}
		if result.Outcome() != application.StatementNotAccepted {
			t.Fatalf("outcome = %q（异议金额不越行）", result.Outcome())
		}
	})

	t.Run("a pending review can be re-resolved but a final cannot", func(t *testing.T) {
		pending, err := fixture.handler.ResolveDispute(context.Background(), application.ResolveDisputeCommand{
			TenantID:   tenant,
			Dispute:    "dispute-1",
			Kind:       domain.DisputePendingReview,
			Basis:      "first-look",
			ResolvedAt: statementPublishAt.Add(48 * time.Hour),
		})
		if err != nil {
			t.Fatalf("resolve pending: %v", err)
		}
		if pending.Outcome() != application.DisputeResolvedOutcome {
			t.Fatalf("outcome = %q", pending.Outcome())
		}

		final, err := fixture.handler.ResolveDispute(context.Background(), application.ResolveDisputeCommand{
			TenantID:   tenant,
			Dispute:    "dispute-1",
			Kind:       domain.DisputeAccepted,
			Basis:      "evidence-verified",
			ResolvedAt: statementPublishAt.Add(72 * time.Hour),
		})
		if err != nil {
			t.Fatalf("resolve final: %v", err)
		}
		if final.Outcome() != application.DisputeResolvedOutcome {
			t.Fatalf("outcome = %q（待审可再裁）", final.Outcome())
		}

		again, err := fixture.handler.ResolveDispute(context.Background(), application.ResolveDisputeCommand{
			TenantID:   tenant,
			Dispute:    "dispute-1",
			Kind:       domain.DisputeRejected,
			Basis:      "second-thoughts",
			ResolvedAt: statementPublishAt.Add(96 * time.Hour),
		})
		if err != nil {
			t.Fatalf("resolve again: %v", err)
		}
		if again.Outcome() != application.DisputeAlreadyResolved {
			t.Fatalf("outcome = %q, want DISPUTE_ALREADY_RESOLVED（终局不二裁）", again.Outcome())
		}
	})
}

// Covers: ADR-0029（依赖故障归未决且指名等谁）、ADR-0031（写入代数封闭）与 ADR-0043
// （投递失败不翻结果、重放重发同一份）在本编排的恢复面；未决原因集封闭。
func TestStatementRecoveryDiscipline(t *testing.T) {
	t.Run("store failures are undecided with their reasons", func(t *testing.T) {
		fixture := newStatementFixture(t)
		fixture.statements.findErr = errors.New("store down")
		result, err := fixture.handler.Publish(context.Background(), publishCommand(t))
		if err != nil {
			t.Fatalf("publish: %v", err)
		}
		if result.UndecidedReason() != application.StatementStoreUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}

		chargeFixture := newStatementFixture(t)
		chargeFixture.charges.findErr = errors.New("charge store down")
		result, err = chargeFixture.handler.Publish(context.Background(), publishCommand(t))
		if err != nil {
			t.Fatalf("publish: %v", err)
		}
		if result.UndecidedReason() != application.StatementChargeStoreUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}
	})

	t.Run("a handoff failure keeps the outcome and is resent on replay", func(t *testing.T) {
		fixture := newStatementFixture(t)
		fixture.handoff.err = errors.New("downstream unavailable")
		first, err := fixture.handler.Publish(context.Background(), publishCommand(t))
		if err != nil {
			t.Fatalf("publish: %v", err)
		}
		if first.Outcome() != application.StatementPublished || first.StatementHandoffReference() == "" {
			t.Fatalf("outcome = %q handoff = %q（投递失败不翻结果）", first.Outcome(), first.StatementHandoffReference())
		}
		fixture.handoff.err = nil
		replay, err := fixture.handler.Publish(context.Background(), publishCommand(t))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.StatementHandoffReference() != "" || len(fixture.handoff.intents) != 1 {
			t.Fatalf("intents = %d handoff = %q", len(fixture.handoff.intents), replay.StatementHandoffReference())
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.StatementUndecidedReason{
			application.StatementStoreUnavailable, application.StatementChargeStoreUnavailable,
			application.InclusionStoreUnavailable, application.DisputeStoreUnavailable,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 4 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.StatementUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第五个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
