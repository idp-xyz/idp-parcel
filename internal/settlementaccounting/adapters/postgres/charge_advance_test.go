package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
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
	chargeFormedAt    = time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	chargeConfirmedAt = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	advanceJudgedAt   = time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	recoveryFormedAt  = time.Date(2026, 8, 11, 11, 0, 0, 0, time.UTC)
)

func TestAConfirmedChargeRoundTripsAndSecondConfirmationKeepsTheWinner(t *testing.T) {
	charges, _, _, transactor, _ := newChargeAdvanceStores(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-a")

	confirmed := confirmedCharge(t, "charge-1", "DELIVERY_FINALIZED/final-1")
	var savedCharge ports.ChargeSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		savedCharge, err = charges.SaveConfirmed(txCtx, tenant, confirmed)
		return err
	})
	if savedCharge != ports.ChargeSaved {
		t.Fatalf("save outcome = %d", savedCharge)
	}

	found, exists, err := charges.FindByID(ctx, tenant, confirmed.ID())
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	basis, ok := found.Confirmation()
	if !ok || basis.String() != "DELIVERY_FINALIZED/final-1" || found.Stage() != domain.ChargeConfirmed {
		t.Fatal("确认往返变形")
	}
	_, amount := found.SettlementAmount()
	if amount != 45600 {
		t.Fatalf("amount = %d", amount)
	}

	other := confirmedCharge(t, "charge-1", "DELIVERY_FINALIZED/other")
	var outcome ports.ChargeSaveOutcome
	var chargeWinner domain.CustomerCharge
	var chargeWinnerFound bool
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		if outcome, err = charges.SaveConfirmed(txCtx, tenant, other); err != nil {
			return err
		}
		chargeWinner, chargeWinnerFound, err = charges.FindByID(txCtx, tenant, confirmed.ID())
		return err
	})
	if !chargeWinnerFound {
		t.Fatal("同事务读回赢家失败")
	}
	if basis, ok := chargeWinner.Confirmation(); !ok || basis.String() != "DELIVERY_FINALIZED/final-1" {
		t.Fatal("二确覆盖了先到者的依据")
	}
	if outcome != ports.ChargeAlreadyConfirmed {
		t.Fatalf("第二份写入结果 = %d", outcome)
	}
}

func TestSaveConfirmedPromotesAnEstimatedRowWithoutRewritingAmount(t *testing.T) {
	charges, _, _, transactor, pool := newChargeAdvanceStores(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-a")

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.customer_charge
			(tenant_id, charge_id, fee_item, evaluation_ref, original_currency,
			 original_minor, settlement_currency, settlement_minor,
			 stage, formed_at, recorded_at)
		 VALUES ('tenant-a', 'charge-est-1', 'BASE_FREIGHT', 'evaluation-sell-1', 'CNY',
		         45600, 'CNY', 45600, 'ESTIMATED', $1, $1)`, chargeFormedAt); err != nil {
		t.Fatalf("预插预估行：%v", err)
	}

	confirmed := confirmedCharge(t, "charge-est-1", "DELIVERY_FINALIZED/final-1")
	var promoted ports.ChargeSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		promoted, err = charges.SaveConfirmed(txCtx, tenant, confirmed)
		return err
	})
	if promoted != ports.ChargeSaved {
		t.Fatalf("promote outcome = %d", promoted)
	}

	found, exists, err := charges.FindByID(ctx, tenant, confirmed.ID())
	if err != nil || !exists || found.Stage() != domain.ChargeConfirmed {
		t.Fatalf("晋升后读回失败：exists=%v err=%v", exists, err)
	}
	originalCurrency, originalMinor := found.OriginalAmount()
	settlementCurrency, settlementMinor := found.SettlementAmount()
	if settlementMinor != 45600 {
		t.Fatal("确认改写了金额")
	}
	if originalCurrency != settlementCurrency || originalMinor != 45600 {
		t.Fatal("晋升丢了原币一对——三件组读回变形")
	}
}

func TestAnAdvanceAssessmentAndRecoveryRoundTrip(t *testing.T) {
	_, assessments, recoveries, transactor, _ := newChargeAdvanceStores(t)
	ctx := t.Context()

	established := establishedAssessmentRecord(t, "tenant-a", "assessment-1")
	var savedAssessment ports.AdvanceAssessmentSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		savedAssessment, err = assessments.Save(txCtx, established)
		return err
	})
	if savedAssessment != ports.AdvanceAssessmentSaved {
		t.Fatalf("assessment save = %d", savedAssessment)
	}

	foundAssessment, exists, err := assessments.FindByKey(ctx, established.Key)
	if err != nil || !exists || foundAssessment.Assessment.Verdict() != domain.AdvanceEstablished {
		t.Fatalf("评估往返失败：exists=%v err=%v", exists, err)
	}
	if _, ok := foundAssessment.Assessment.FundsFact(); !ok {
		t.Fatal("成立评估丢了资金事实")
	}

	notEstablished := notEstablishedAssessmentRecord(t, "tenant-a", "assessment-2")
	var savedNotEstablished ports.AdvanceAssessmentSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		savedNotEstablished, err = assessments.Save(txCtx, notEstablished)
		return err
	})
	if savedNotEstablished != ports.AdvanceAssessmentSaved {
		t.Fatalf("not-established save = %d", savedNotEstablished)
	}
	foundNegative, exists, err := assessments.FindByKey(ctx, notEstablished.Key)
	if err != nil || !exists || foundNegative.Assessment.Verdict() != domain.AdvanceNotEstablishedVerdict {
		t.Fatalf("不成立评估往返失败：exists=%v err=%v", exists, err)
	}
	if _, ok := foundNegative.Assessment.Basis(); !ok {
		t.Fatal("不成立评估丢了负向依据")
	}

	recovery := formedRecoveryRecord(t, "tenant-a", "recovery-1", foundAssessment.Assessment)
	var savedRecovery ports.AdvanceRecoverySaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		savedRecovery, err = recoveries.Save(txCtx, recovery)
		return err
	})
	if savedRecovery != ports.AdvanceRecoverySaved {
		t.Fatalf("recovery save = %d", savedRecovery)
	}
	foundRecovery, exists, err := recoveries.FindByKey(ctx, recovery.Key)
	if err != nil || !exists {
		t.Fatalf("回收往返失败：exists=%v err=%v", exists, err)
	}
	_, amount := foundRecovery.Recovery.Amount()
	if amount != 8000 || foundRecovery.Recovery.Assessment().String() != "assessment-1" {
		t.Fatalf("回收往返变形：amount=%d assessment=%s", amount, foundRecovery.Recovery.Assessment())
	}

	secondAssessment := established
	secondAssessment.ContentDigest = "digest-other"
	var assessmentOutcome ports.AdvanceAssessmentSaveOutcome
	var assessmentWinner ports.AdvanceAssessmentRecord
	var assessmentWinnerFound bool
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		if assessmentOutcome, err = assessments.Save(txCtx, secondAssessment); err != nil {
			return err
		}
		assessmentWinner, assessmentWinnerFound, err = assessments.FindByKey(txCtx, established.Key)
		return err
	})
	if !assessmentWinnerFound || assessmentWinner.ContentDigest != established.ContentDigest {
		t.Fatalf("同事务读回评估赢家失败：found=%v digest=%q",
			assessmentWinnerFound, assessmentWinner.ContentDigest)
	}
	if assessmentOutcome != ports.AdvanceAssessmentAlreadyRecorded {
		t.Fatalf("第二份评估结果 = %d", assessmentOutcome)
	}

	secondRecovery := recovery
	secondRecovery.ContentDigest = "digest-other"
	var recoveryOutcome ports.AdvanceRecoverySaveOutcome
	var recoveryWinner ports.AdvanceRecoveryRecord
	var recoveryWinnerFound bool
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		if recoveryOutcome, err = recoveries.Save(txCtx, secondRecovery); err != nil {
			return err
		}
		recoveryWinner, recoveryWinnerFound, err = recoveries.FindByKey(txCtx, recovery.Key)
		return err
	})
	if !recoveryWinnerFound || recoveryWinner.ContentDigest != recovery.ContentDigest {
		t.Fatalf("同事务读回回收赢家失败：found=%v digest=%q",
			recoveryWinnerFound, recoveryWinner.ContentDigest)
	}
	if recoveryOutcome != ports.AdvanceRecoveryAlreadyFormed {
		t.Fatalf("第二份回收结果 = %d", recoveryOutcome)
	}
}

func TestChargeAdvanceRecordsAreInvisibleAcrossTenants(t *testing.T) {
	charges, assessments, recoveries, transactor, _ := newChargeAdvanceStores(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-a")

	charge := confirmedCharge(t, "charge-shared", "DELIVERY_FINALIZED/final-1")
	assessment := establishedAssessmentRecord(t, "tenant-a", "assessment-shared")
	recovery := formedRecoveryRecord(t, "tenant-a", "recovery-shared", assessment.Assessment)
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := charges.SaveConfirmed(txCtx, tenant, charge); err != nil {
			return err
		}
		if _, err := assessments.Save(txCtx, assessment); err != nil {
			return err
		}
		_, err := recoveries.Save(txCtx, recovery)
		return err
	})

	other := saTenant(t, "tenant-b")
	if _, exists, err := charges.FindByID(ctx, other, charge.ID()); err != nil || exists {
		t.Errorf("另一个租户读到了费用：exists=%v err=%v", exists, err)
	}
	if _, exists, err := assessments.FindByKey(ctx, ports.AdvanceAssessmentKey{TenantID: other, Assessment: assessment.Key.Assessment}); err != nil || exists {
		t.Errorf("另一个租户读到了评估：exists=%v err=%v", exists, err)
	}
	if _, exists, err := recoveries.FindByKey(ctx, ports.AdvanceRecoveryKey{TenantID: other, Recovery: recovery.Key.Recovery}); err != nil || exists {
		t.Errorf("另一个租户读到了回收：exists=%v err=%v", exists, err)
	}
}

func TestChargeAdvanceWritesRefuseToRunOutsideATransaction(t *testing.T) {
	charges, assessments, recoveries, _, _ := newChargeAdvanceStores(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-a")

	if _, err := charges.SaveConfirmed(ctx, tenant, confirmedCharge(t, "charge-1", "DELIVERY_FINALIZED/final-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 SaveConfirmed：%v", err)
	}
	if _, err := assessments.Save(ctx, establishedAssessmentRecord(t, "tenant-a", "assessment-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Save 评估：%v", err)
	}
	assessment := establishedAssessmentRecord(t, "tenant-a", "assessment-1")
	if _, err := recoveries.Save(ctx, formedRecoveryRecord(t, "tenant-a", "recovery-1", assessment.Assessment)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Save 回收：%v", err)
	}
}

func TestChargeAdvanceRollbackLeavesNothingBehind(t *testing.T) {
	charges, assessments, recoveries, transactor, _ := newChargeAdvanceStores(t)
	ctx := t.Context()
	rollback := errors.New("回滚")
	tenant := saTenant(t, "tenant-a")
	charge := confirmedCharge(t, "charge-1", "DELIVERY_FINALIZED/final-1")
	assessment := establishedAssessmentRecord(t, "tenant-a", "assessment-1")
	recovery := formedRecoveryRecord(t, "tenant-a", "recovery-1", assessment.Assessment)

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := charges.SaveConfirmed(txCtx, tenant, charge); err != nil {
			return err
		}
		if _, err := assessments.Save(txCtx, assessment); err != nil {
			return err
		}
		if _, err := recoveries.Save(txCtx, recovery); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if _, exists, err := charges.FindByID(ctx, tenant, charge.ID()); err != nil || exists {
		t.Errorf("回滚后费用仍在：exists=%v err=%v", exists, err)
	}
	if _, exists, err := assessments.FindByKey(ctx, assessment.Key); err != nil || exists {
		t.Errorf("回滚后评估仍在：exists=%v err=%v", exists, err)
	}
	if _, exists, err := recoveries.FindByKey(ctx, recovery.Key); err != nil || exists {
		t.Errorf("回滚后回收仍在：exists=%v err=%v", exists, err)
	}
}

func TestChargeAdvanceCheckConstraintsRejectImpossibleRows(t *testing.T) {
	_, _, _, _, pool := newChargeAdvanceStores(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.customer_charge
			(tenant_id, charge_id, fee_item, evaluation_ref, original_currency,
			 original_minor, settlement_currency, settlement_minor,
			 stage, confirmation_basis, formed_at, confirmed_at, recorded_at)
		 VALUES ('tenant-a', 'c-bad-1', 'BASE_FREIGHT', 'eval-1', 'CNY', 100, 'CNY', 100,
		         'CONFIRMED', NULL, now(), now(), now())`); err == nil {
		t.Fatal("一行「已确认却没有依据」溜进了费用库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.customer_charge
			(tenant_id, charge_id, fee_item, evaluation_ref, original_currency,
			 original_minor, settlement_currency, settlement_minor,
			 stage, formed_at, recorded_at)
		 VALUES ('tenant-a', 'c-bad-2', 'BASE_FREIGHT', 'eval-1', 'CNY', 0, 'CNY', 0,
		         'ESTIMATED', now(), now())`); err == nil {
		t.Fatal("一行「金额为零」溜进了费用库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.customer_charge
			(tenant_id, charge_id, fee_item, evaluation_ref, original_currency,
			 original_minor, settlement_currency, settlement_minor,
			 stage, formed_at, recorded_at)
		 VALUES ('tenant-a', 'c-bad-3', 'BASE_FREIGHT', 'eval-1', 'CNY', 100, 'CNY', 90,
		         'ESTIMATED', now(), now())`); err == nil {
		t.Fatal("一行「同币种两额不等」溜进了费用库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.customer_charge
			(tenant_id, charge_id, fee_item, evaluation_ref, original_currency,
			 original_minor, settlement_currency, settlement_minor,
			 stage, formed_at, recorded_at)
		 VALUES ('tenant-a', 'c-bad-4', 'BASE_FREIGHT', 'eval-1', 'USD', 100, 'CNY', 700,
		         'ESTIMATED', now(), now())`); err == nil {
		t.Fatal("一行「跨币种却没有换算依据」溜进了费用库")
	}

	// 以下四格钉 ADR-0087 决定一的库面：CONTEXT 那句「任何一项不能通过当前组织、当前
	// 客户属性或报表筛选临时推断」，在库上唯一可核对的形式就是确认行七项俱全。
	// confirmed_at 与 formed_at 都由库侧 now() 出（同事务同值，满足 confirmed_at >=
	// formed_at）：Go 侧取一次时间再传进来，两个时钟谁先谁后不确定，行会被旧的
	// customer_charge_confirmation_coupled 先拒掉，于是这几格测的就不是它们要测的东西。
	confirmedWithFacts := `INSERT INTO settlement_accounting.customer_charge
			(tenant_id, charge_id, fee_item, evaluation_ref, original_currency,
			 original_minor, settlement_currency, settlement_minor,
			 stage, confirmation_basis, formed_at, confirmed_at, recorded_at,
			 responsible_entity, counterparty_ref, charge_direction,
			 settlement_account_id, contract_basis, primary_charging_scope,
			 source_fact_ref)
		 VALUES ('tenant-a', $1, 'BASE_FREIGHT', 'eval-1', 'CNY', 100, 'CNY', 100,
		         $2, $3, now(), CASE WHEN $2 = 'CONFIRMED' THEN now() END, now(),
		         $4, $5, $6, $7, $8, $9, $10)`

	// 逐格钉住**是哪条约束**拒的，不只钉「拒了」：只断言 err != nil 时，一个写错的
	// INSERT 与一条真正生效的 CHECK 在测试里长着同一张脸。
	for _, refusal := range []struct {
		name       string
		args       []any
		constraint string
		complaint  string
	}{
		{
			name:       "c-bad-5",
			args:       []any{"CONFIRMED", "basis-1", nil, nil, nil, nil, nil, nil, nil},
			constraint: "customer_charge_confirmation_facts_coupled",
			complaint:  "一行「已确认却一项事实都不带」溜进了费用库——那句硬句在库上还是空转",
		},
		{
			name: "c-bad-6",
			args: []any{"CONFIRMED", "basis-1",
				"entity-1", "counterparty-1", "RECEIVABLE", "account-1", "contract-1", nil, "source-1"},
			constraint: "customer_charge_confirmation_facts_coupled",
			complaint:  "一行「已确认却缺主要计费范围」溜进了费用库——七项不是同在或同缺",
		},
		{
			name: "c-bad-7",
			args: []any{"ESTIMATED", nil,
				"entity-1", "counterparty-1", "RECEIVABLE", "account-1", "contract-1", "scope-1", "source-1"},
			constraint: "customer_charge_confirmation_facts_coupled",
			complaint:  "一行「预估却带着确认才该固定的事实」溜进了费用库——领域产不出这样一行",
		},
		{
			name: "c-bad-8",
			args: []any{"CONFIRMED", "basis-1",
				"entity-1", "counterparty-1", "DEBIT", "account-1", "contract-1", "scope-1", "source-1"},
			constraint: "customer_charge_direction_closed",
			complaint:  "借贷方向被当成收付方向收下了——两个词表不是一回事",
		},
	} {
		_, err := pool.Exec(ctx, confirmedWithFacts, append([]any{refusal.name}, refusal.args...)...)
		if err == nil {
			t.Fatal(refusal.complaint)
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != refusal.constraint {
			t.Fatalf("%s 被拒了，但不是 %s 拒的：%v", refusal.name, refusal.constraint, err)
		}
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.advance_assessment
			(tenant_id, assessment_id, obligation_ref, verdict, funds_fact, payer_ref,
			 responsibility_ref, basis, currency, amount_minor, version, judged_at,
			 content_digest, recorded_at)
		 VALUES ('tenant-a', 'a-bad-1', 'tax-1', 'ESTABLISHED', 'funds-1', 'payer-1',
		         'resp-1', 'why', 'USD', 100, 'v1', now(), 'd', now())`); err == nil {
		t.Fatal("一行「成立却带着负向依据」溜进了评估库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.advance_assessment
			(tenant_id, assessment_id, obligation_ref, verdict, basis, currency,
			 amount_minor, version, judged_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'a-bad-2', 'tax-1', 'NOT_ESTABLISHED', NULL, 'USD',
		         100, 'v1', now(), 'd', now())`); err == nil {
		t.Fatal("一行「不成立却没有依据」溜进了评估库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.advance_recovery
			(tenant_id, recovery_id, assessment_id, customer_ref, contract_basis,
			 account_id, currency, amount_minor, formed_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'r-bad-1', 'a-1', 'cust-1', 'contract-1', 'acct-1', 'USD',
		         0, now(), 'd', now())`); err == nil {
		t.Fatal("一行「回收金额为零」溜进了回收库")
	}
}

func newChargeAdvanceStores(t *testing.T) (
	*adapter.CustomerCharges,
	*adapter.AdvanceAssessments,
	*adapter.AdvanceRecoveries,
	bentoapp.Transactor,
	*pgxpool.Pool,
) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	charges, err := adapter.NewCustomerCharges(db)
	if err != nil {
		t.Fatalf("构造费用库：%v", err)
	}
	assessments, err := adapter.NewAdvanceAssessments(db)
	if err != nil {
		t.Fatalf("构造评估库：%v", err)
	}
	recoveries, err := adapter.NewAdvanceRecoveries(db)
	if err != nil {
		t.Fatalf("构造回收库：%v", err)
	}
	return charges, assessments, recoveries, db.Transactor(), pool
}

func confirmedCharge(t *testing.T, id, basis string) domain.CustomerCharge {
	t.Helper()
	charge, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID:                 saValue(t, domain.NewCustomerChargeID, id),
		FeeItem:            saValue(t, domain.NewFeeItemReference, "BASE_FREIGHT"),
		Evaluation:         saValue(t, domain.NewSellEvaluationReference, "evaluation-sell-1"),
		OriginalCurrency:   saValue(t, domain.NewCurrencyCode, "CNY"),
		OriginalMinor:      45600,
		SettlementCurrency: saValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    45600,
		Stage:              domain.ChargeEstimated,
		FormedAt:           chargeFormedAt,
	})
	if err != nil {
		t.Fatalf("构造预估费用：%v", err)
	}
	confirmed, err := charge.Confirm(
		saConfirmationFacts(t),
		saValue(t, domain.NewConfirmationBasisReference, basis),
		chargeConfirmedAt,
	)
	if err != nil {
		t.Fatalf("确认费用：%v", err)
	}
	return confirmed
}

func saConfirmationFacts(t *testing.T) domain.ConfirmedChargeFacts {
	t.Helper()
	return domain.ConfirmedChargeFacts{
		ResponsibleEntity:    saValue(t, domain.NewLegalEntityReference, "LEGAL-ENTITY/syn-1"),
		Counterparty:         saValue(t, domain.NewSettlementCounterpartyReference, "COUNTERPARTY/syn-1"),
		Direction:            domain.ChargeReceivable,
		SettlementAccount:    saValue(t, domain.NewSettlementAccountID, "ACCOUNT/syn-1"),
		ContractBasis:        saValue(t, domain.NewContractBasisReference, "CONTRACT/syn-1"),
		PrimaryChargingScope: saValue(t, domain.NewChargingScopeReference, "SCOPE/syn-1"),
		SourceFact:           saValue(t, domain.NewSourceFactReference, "SOURCE-FACT/syn-1"),
	}
}

func establishedAssessmentRecord(t *testing.T, tenant, id string) ports.AdvanceAssessmentRecord {
	t.Helper()
	assessment, err := domain.AssessActualAdvance(domain.ActualAdvanceAssessmentSpec{
		ID:             saValue(t, domain.NewAdvanceAssessmentID, id),
		Obligation:     saValue(t, domain.NewTaxObligationReference, "tax-obligation-1"),
		Verdict:        domain.AdvanceEstablished,
		FundsFact:      saValue(t, domain.NewFundsFactReference, "funds-fact-1"),
		Payer:          saValue(t, domain.NewAdvancePayerReference, "operator-legal-1"),
		Responsibility: saValue(t, domain.NewAdvanceResponsibilityReference, "duty-1"),
		Currency:       saValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor:    10000,
		Version:        saValue(t, domain.NewAdvanceAssessmentVersion, "assessment/v1"),
		JudgedAt:       advanceJudgedAt,
	})
	if err != nil {
		t.Fatalf("构造成立评估：%v", err)
	}
	return ports.AdvanceAssessmentRecord{
		Key:           ports.AdvanceAssessmentKey{TenantID: saTenant(t, tenant), Assessment: assessment.ID()},
		ContentDigest: "digest-" + id,
		Assessment:    assessment,
		RecordedAt:    advanceJudgedAt,
	}
}

func notEstablishedAssessmentRecord(t *testing.T, tenant, id string) ports.AdvanceAssessmentRecord {
	t.Helper()
	assessment, err := domain.AssessActualAdvance(domain.ActualAdvanceAssessmentSpec{
		ID:          saValue(t, domain.NewAdvanceAssessmentID, id),
		Obligation:  saValue(t, domain.NewTaxObligationReference, "tax-obligation-1"),
		Verdict:     domain.AdvanceNotEstablishedVerdict,
		Basis:       saValue(t, domain.NewAssessmentBasisReference, "customer-paid-directly"),
		Currency:    saValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor: 10000,
		Version:     saValue(t, domain.NewAdvanceAssessmentVersion, "assessment/v1"),
		JudgedAt:    advanceJudgedAt,
	})
	if err != nil {
		t.Fatalf("构造不成立评估：%v", err)
	}
	return ports.AdvanceAssessmentRecord{
		Key:           ports.AdvanceAssessmentKey{TenantID: saTenant(t, tenant), Assessment: assessment.ID()},
		ContentDigest: "digest-" + id,
		Assessment:    assessment,
		RecordedAt:    advanceJudgedAt,
	}
}

func formedRecoveryRecord(t *testing.T, tenant, id string, assessment domain.ActualAdvanceAssessment) ports.AdvanceRecoveryRecord {
	t.Helper()
	recovery, err := domain.FormCustomerAdvanceRecovery(
		assessment,
		saValue(t, domain.NewAdvanceRecoveryID, id),
		saValue(t, domain.NewRecoveryCustomerReference, "customer-1"),
		saValue(t, domain.NewContractResponsibilityReference, "contract-1"),
		saValue(t, domain.NewSettlementAccountID, "account-1"),
		8000,
		recoveryFormedAt,
	)
	if err != nil {
		t.Fatalf("构造回收：%v", err)
	}
	return ports.AdvanceRecoveryRecord{
		Key:           ports.AdvanceRecoveryKey{TenantID: saTenant(t, tenant), Recovery: recovery.ID()},
		ContentDigest: "digest-" + id,
		Recovery:      recovery,
		RecordedAt:    recoveryFormedAt,
	}
}
