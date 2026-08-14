package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	inclusionAt   = time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	disputeOpened = time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	disputeRuled  = time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	adjustFormed  = time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
)

func TestASubsequentInclusionRoundTripsAndSecondSaveKeepsTheWinner(t *testing.T) {
	inclusions, _, _, _, transactor, _ := newCloseoutStores(t)
	ctx := t.Context()

	record := formedInclusionRecord(t, "tenant-a", "inclusion-1", domain.IncludedAdjustment)
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := inclusions.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.InclusionSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})

	found, exists, err := inclusions.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	adjustment, present := found.Inclusion.Adjustment()
	if found.Inclusion.Kind() != domain.IncludedAdjustment || !present || adjustment.String() != "adjustment-1" {
		t.Fatal("纳入调整往返变形")
	}

	late := formedInclusionRecord(t, "tenant-a", "inclusion-late", domain.IncludedLateCharge)
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := inclusions.Save(txCtx, late)
		if err != nil {
			return err
		}
		if outcome != ports.InclusionSaved {
			t.Fatalf("late save outcome = %d", outcome)
		}
		return nil
	})
	foundLate, _, err := inclusions.FindByKey(ctx, late.Key)
	if err != nil {
		t.Fatalf("迟到费用读回：%v", err)
	}
	if _, present := foundLate.Inclusion.Adjustment(); present {
		t.Fatal("迟到费用挂上了调整")
	}

	second := record
	second.ContentDigest = "digest-other"
	var outcome ports.InclusionSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := inclusions.Save(txCtx, second)
		if err != nil {
			return err
		}
		outcome = saved
		winner, found, err := inclusions.FindByKey(txCtx, record.Key)
		if err != nil || !found || winner.ContentDigest != "digest-inclusion-1" {
			t.Fatalf("同事务读回赢家失败：found=%v digest=%q err=%v", found, winner.ContentDigest, err)
		}
		return nil
	})
	if outcome != ports.InclusionAlreadyRecorded {
		t.Fatalf("第二份写入结果 = %d", outcome)
	}
}

func TestAStatementDisputeReplaceWritesResolutionNotTheStatement(t *testing.T) {
	_, disputes, _, _, transactor, _ := newCloseoutStores(t)
	ctx := t.Context()

	opened := formedDisputeRecord(t, "tenant-a", "dispute-1", false)
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := disputes.Save(txCtx, opened)
		if err != nil {
			return err
		}
		if outcome != ports.DisputeSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})

	found, exists, err := disputes.FindByKey(ctx, opened.Key)
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	if _, _, _, resolved := found.Dispute.Resolution(); resolved {
		t.Fatal("开立行凭空带上了裁定")
	}

	ruled := formedDisputeRecord(t, "tenant-a", "dispute-1", true)
	ruled.ContentDigest = "digest-ruled"
	ruled.RecordedAt = disputeRuled
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		ok, err := disputes.Replace(txCtx, ruled)
		if err != nil {
			return err
		}
		if !ok {
			t.Fatal("Replace 答 false")
		}
		return nil
	})

	after, _, err := disputes.FindByKey(ctx, opened.Key)
	if err != nil {
		t.Fatalf("裁定后读回：%v", err)
	}
	kind, basis, _, resolved := after.Dispute.Resolution()
	if !resolved || kind != domain.DisputeAccepted || basis.String() != "review-request-1" {
		t.Fatal("裁定没有落库")
	}
	if after.Dispute.Statement().String() != "statement-2026-08-001" ||
		after.Dispute.DisputedMinor() != 4000 {
		t.Fatal("Replace 改写了对账单或争议金额")
	}

	again := ruled
	again.ContentDigest = "digest-again"
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		ok, err := disputes.Replace(txCtx, again)
		if err != nil {
			return err
		}
		if ok {
			t.Fatal("终局裁定被第二次 Replace 改写")
		}
		return nil
	})
}

func TestRecoveryAndClaimAdjustmentsRoundTrip(t *testing.T) {
	_, _, recoveries, claims, transactor, _ := newCloseoutStores(t)
	ctx := t.Context()

	recovery := formedRecoveryAdjustmentRecord(t, "tenant-a", "recovery-adj-1")
	claim := formedClaimAdjustmentRecord(t, "tenant-a", "claim-adj-1")
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		if outcome, err := recoveries.Save(txCtx, recovery); err != nil || outcome != ports.RecoveryAdjustmentSaved {
			t.Fatalf("save recovery adj：outcome=%d err=%v", outcome, err)
		}
		if outcome, err := claims.Save(txCtx, claim); err != nil || outcome != ports.ClaimAdjustmentSaved {
			t.Fatalf("save claim adj：outcome=%d err=%v", outcome, err)
		}
		return nil
	})

	foundRecovery, exists, err := recoveries.FindByKey(ctx, recovery.Key)
	if err != nil || !exists || foundRecovery.Adjustment.Reason() != domain.TaxAssessmentCorrected {
		t.Fatalf("回收调整读回失败：exists=%v err=%v", exists, err)
	}
	foundClaim, exists, err := claims.FindByKey(ctx, claim.Key)
	if err != nil || !exists || foundClaim.Adjustment.TargetKind() != domain.AdjustsRecoveryReceivable {
		t.Fatalf("索赔调整读回失败：exists=%v err=%v", exists, err)
	}

	var recoveryOutcome ports.RecoveryAdjustmentSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		second := recovery
		second.ContentDigest = "digest-other"
		saved, err := recoveries.Save(txCtx, second)
		if err != nil {
			return err
		}
		recoveryOutcome = saved
		return nil
	})
	if recoveryOutcome != ports.RecoveryAdjustmentAlreadyFormed {
		t.Fatalf("第二份回收调整 = %d", recoveryOutcome)
	}
}

func TestCloseoutWritesRefuseToRunOutsideATransaction(t *testing.T) {
	inclusions, disputes, recoveries, claims, _, _ := newCloseoutStores(t)
	ctx := t.Context()

	if _, err := inclusions.Save(ctx, formedInclusionRecord(t, "tenant-a", "inc-ntx", domain.IncludedLateCharge)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存纳入应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := disputes.Save(ctx, formedDisputeRecord(t, "tenant-a", "disp-ntx", false)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存异议应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := recoveries.Save(ctx, formedRecoveryAdjustmentRecord(t, "tenant-a", "radj-ntx")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存回收调整应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := claims.Save(ctx, formedClaimAdjustmentRecord(t, "tenant-a", "cadj-ntx")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存索赔调整应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestCloseoutCheckConstraintsRejectImpossibleRows(t *testing.T) {
	_, _, _, _, _, pool := newCloseoutStores(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.subsequent_inclusion
			(tenant_id, inclusion_id, kind, statement_number, original_period,
			 subsequent_period, charge_id, adjustment_id, included_at,
			 content_digest, recorded_at)
		 VALUES ('tenant-a', 'i-bad-1', 'ADJUSTMENT', 'st-1', 'period-a',
		         'period-b', 'charge-1', NULL, now(), 'd', now())`); err == nil {
		t.Fatal("一行「纳入调整却没有调整」溜进了纳入库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.subsequent_inclusion
			(tenant_id, inclusion_id, kind, statement_number, original_period,
			 subsequent_period, charge_id, included_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'i-bad-2', 'LATE_CHARGE', 'st-1', 'period-a',
		         'period-a', 'charge-1', now(), 'd', now())`); err == nil {
		t.Fatal("一行「回填原周期」溜进了纳入库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.statement_dispute
			(tenant_id, dispute_id, statement_number, charge_id, disputed_minor,
			 reason_ref, opened_at, resolution, resolution_ref, resolved_at,
			 content_digest, recorded_at)
		 VALUES ('tenant-a', 'd-bad-1', 'st-1', 'charge-1', 100,
		         'reason', now(), 'ACCEPTED', NULL, now(), 'd', now())`); err == nil {
		t.Fatal("一行「已裁定却没有依据」溜进了异议库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.recovery_adjustment
			(tenant_id, adjustment_id, recovery_id, reason, new_basis, direction,
			 currency, amount_minor, period_ref, formed_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'r-bad-1', 'rec-1', 'TAX_ASSESSMENT_CORRECTED', 'basis',
		         'DEBIT', 'USD', 0, 'period-1', now(), 'd', now())`); err == nil {
		t.Fatal("一行「回收调整金额为零」溜进了调整库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.claim_amount_adjustment
			(tenant_id, adjustment_id, target_kind, target_ref, reason, basis_ref,
			 direction, currency, amount_minor, period_ref, formed_at,
			 content_digest, recorded_at)
		 VALUES ('tenant-a', 'c-bad-1', 'CUSTOMER_CLAIM_AMOUNT', 'amt-1',
		         'FUNDS_CHANGED', 'basis', 'DEBIT', 'USD', 100, 'period-1', now(), 'd', now())`); err == nil {
		t.Fatal("一行「资金变化当调整原因」溜进了索赔调整库")
	}
}

func newCloseoutStores(t *testing.T) (
	*adapter.SubsequentInclusions,
	*adapter.StatementDisputes,
	*adapter.RecoveryAdjustments,
	*adapter.ClaimAmountAdjustments,
	bentoapp.Transactor,
	*pgxpool.Pool,
) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	inclusions, err := adapter.NewSubsequentInclusions(db)
	if err != nil {
		t.Fatalf("构造纳入库：%v", err)
	}
	disputes, err := adapter.NewStatementDisputes(db)
	if err != nil {
		t.Fatalf("构造异议库：%v", err)
	}
	recoveries, err := adapter.NewRecoveryAdjustments(db)
	if err != nil {
		t.Fatalf("构造回收调整库：%v", err)
	}
	claims, err := adapter.NewClaimAmountAdjustments(db)
	if err != nil {
		t.Fatalf("构造索赔调整库：%v", err)
	}
	return inclusions, disputes, recoveries, claims, db.Transactor(), pool
}

func formedInclusionRecord(t *testing.T, tenant, id string, kind domain.InclusionKind) ports.InclusionRecord {
	t.Helper()
	spec := domain.RehydrateSubsequentInclusionSpec{
		Inclusion:        saValue(t, domain.NewInclusionReference, id),
		Kind:             kind,
		Statement:        saValue(t, domain.NewStatementNumber, "statement-2026-08-001"),
		OriginalPeriod:   saValue(t, domain.NewBillingPeriodReference, "period-2026-08"),
		SubsequentPeriod: saValue(t, domain.NewBillingPeriodReference, "period-2026-09"),
		Charge:           saValue(t, domain.NewCustomerChargeID, "charge-1"),
		IncludedAt:       inclusionAt,
	}
	if kind == domain.IncludedAdjustment {
		spec.Adjustment = saValue(t, domain.NewChargeAdjustmentID, "adjustment-1")
	}
	inclusion, err := domain.RehydrateSubsequentInclusion(spec)
	if err != nil {
		t.Fatalf("构造纳入：%v", err)
	}
	return ports.InclusionRecord{
		Key:           ports.InclusionKey{TenantID: saTenant(t, tenant), Inclusion: inclusion.Inclusion()},
		ContentDigest: "digest-" + id,
		Inclusion:     inclusion,
		RecordedAt:    inclusionAt,
	}
}

func formedDisputeRecord(t *testing.T, tenant, id string, resolved bool) ports.DisputeRecord {
	t.Helper()
	spec := domain.RehydrateStatementDisputeSpec{
		Dispute:       saValue(t, domain.NewDisputeID, id),
		Statement:     saValue(t, domain.NewStatementNumber, "statement-2026-08-001"),
		Charge:        saValue(t, domain.NewCustomerChargeID, "charge-1"),
		DisputedMinor: 4000,
		Reason:        saValue(t, domain.NewDisputeBasisReference, "weight-mismatch"),
		OpenedAt:      disputeOpened,
	}
	if resolved {
		spec.Resolution = domain.DisputeAccepted
		spec.ResolutionRef = saValue(t, domain.NewDisputeBasisReference, "review-request-1")
		spec.ResolvedAt = disputeRuled
	}
	dispute, err := domain.RehydrateStatementDispute(spec)
	if err != nil {
		t.Fatalf("构造异议：%v", err)
	}
	recorded := disputeOpened
	digest := "digest-" + id
	if resolved {
		recorded = disputeRuled
		digest = "digest-ruled"
	}
	return ports.DisputeRecord{
		Key:           ports.DisputeKey{TenantID: saTenant(t, tenant), Dispute: dispute.Dispute()},
		ContentDigest: digest,
		Dispute:       dispute,
		RecordedAt:    recorded,
	}
}

func formedRecoveryAdjustmentRecord(t *testing.T, tenant, id string) ports.RecoveryAdjustmentRecord {
	t.Helper()
	adjustment, err := domain.FormRecoveryAdjustment(domain.RecoveryAdjustmentSpec{
		ID:          saValue(t, domain.NewRecoveryAdjustmentID, id),
		Recovery:    saValue(t, domain.NewAdvanceRecoveryID, "recovery-1"),
		Reason:      domain.TaxAssessmentCorrected,
		NewBasis:    saValue(t, domain.NewAssessmentBasisReference, "tax-correction-1"),
		Direction:   domain.AdjustmentCredit,
		Currency:    saValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor: 1500,
		Period:      saValue(t, domain.NewBillingPeriodReference, "period-2026-09"),
		FormedAt:    adjustFormed,
	})
	if err != nil {
		t.Fatalf("构造回收调整：%v", err)
	}
	return ports.RecoveryAdjustmentRecord{
		Key:           ports.RecoveryAdjustmentKey{TenantID: saTenant(t, tenant), Adjustment: adjustment.ID()},
		ContentDigest: "digest-" + id,
		Adjustment:    adjustment,
		RecordedAt:    adjustFormed,
	}
}

func formedClaimAdjustmentRecord(t *testing.T, tenant, id string) ports.ClaimAdjustmentRecord {
	t.Helper()
	adjustment, err := domain.FormClaimAmountAdjustment(domain.ClaimAmountAdjustmentSpec{
		ID:          saValue(t, domain.NewClaimAmountAdjustmentID, id),
		TargetKind:  domain.AdjustsRecoveryReceivable,
		Target:      saValue(t, domain.NewAdjustedAmountReference, "receivable-1"),
		Reason:      domain.ResponsibilityRevised,
		Basis:       saValue(t, domain.NewResponsibilityConclusionReference, "responsibility/v2"),
		Direction:   domain.AdjustmentCredit,
		Currency:    saValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor: 2000,
		Period:      saValue(t, domain.NewBillingPeriodReference, "period-2026-10"),
		FormedAt:    adjustFormed,
	})
	if err != nil {
		t.Fatalf("构造索赔调整：%v", err)
	}
	return ports.ClaimAdjustmentRecord{
		Key:           ports.ClaimAdjustmentKey{TenantID: saTenant(t, tenant), Adjustment: adjustment.ID()},
		ContentDigest: "digest-" + id,
		Adjustment:    adjustment,
		RecordedAt:    adjustFormed,
	}
}
