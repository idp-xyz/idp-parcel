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
	claimAmountFormedAt = time.Date(2026, 8, 12, 14, 0, 0, 0, time.UTC)
	acknowledgedAtTS    = time.Date(2026, 8, 13, 14, 0, 0, 0, time.UTC)
)

func TestAClaimAmountRoundTripsAndSecondSaveKeepsTheWinner(t *testing.T) {
	amounts, _, _, transactor, _ := newClaimAmountStores(t)
	ctx := t.Context()

	record := formedClaimAmountRecord(t, "tenant-a", "claim-amount-1", domain.CustomerCompensationPayable)
	var savedOutcome ports.ClaimAmountSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		savedOutcome, err = amounts.Save(txCtx, record)
		return err
	})
	if savedOutcome != ports.ClaimAmountSaved {
		t.Fatalf("save outcome = %d", savedOutcome)
	}

	found, exists, err := amounts.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	if found.Amount.Kind() != domain.CustomerCompensationPayable {
		t.Fatal("赔付种类往返变形")
	}
	if _, present := found.Amount.OriginalCharge(); present {
		t.Fatal("赔付义务凭空挂上了原费用")
	}
	_, amount := found.Amount.Amount()
	if amount != 5000 {
		t.Fatalf("amount = %d", amount)
	}

	second := record
	second.ContentDigest = "digest-other"
	var outcome ports.ClaimAmountSaveOutcome
	var winner ports.ClaimAmountRecord
	var winnerFound bool
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		if outcome, err = amounts.Save(txCtx, second); err != nil {
			return err
		}
		winner, winnerFound, err = amounts.FindByKey(txCtx, record.Key)
		return err
	})
	if outcome != ports.ClaimAmountAlreadyFormed {
		t.Fatalf("第二份写入结果 = %d", outcome)
	}
	if !winnerFound || winner.ContentDigest != "digest-claim-amount-1" {
		t.Fatalf("同事务读回赢家失败：found=%v digest=%q", winnerFound, winner.ContentDigest)
	}
}

func TestAClaimChargeRefundRoundTripsWithItsOriginalCharge(t *testing.T) {
	amounts, _, _, transactor, _ := newClaimAmountStores(t)
	ctx := t.Context()

	record := formedClaimAmountRecord(t, "tenant-a", "claim-refund-1", domain.ClaimChargeRefund)
	var refundOutcome ports.ClaimAmountSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		refundOutcome, err = amounts.Save(txCtx, record)
		return err
	})
	if refundOutcome != ports.ClaimAmountSaved {
		t.Fatalf("save outcome = %d", refundOutcome)
	}

	found, exists, err := amounts.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	original, present := found.Amount.OriginalCharge()
	if !present || original.String() != "charge-1" {
		t.Fatal("索赔退款丢了被贷记的原费用")
	}
}

func TestAReceivableAndAcknowledgementRoundTrip(t *testing.T) {
	_, receivables, acknowledgements, transactor, _ := newClaimAmountStores(t)
	ctx := t.Context()

	receivable := formedReceivableRecord(t, "tenant-a", "receivable-1")
	var savedReceivable ports.ReceivableSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		savedReceivable, err = receivables.Save(txCtx, receivable)
		return err
	})
	if savedReceivable != ports.ReceivableSaved {
		t.Fatalf("save receivable outcome = %d", savedReceivable)
	}

	foundReceivable, exists, err := receivables.FindByKey(ctx, receivable.Key)
	if err != nil || !exists {
		t.Fatalf("应追偿读回失败：err=%v exists=%v", err, exists)
	}
	if foundReceivable.Receivable.RuleVersion().String() != "amount-rule/v1" {
		t.Fatal("应追偿规则版本往返变形")
	}

	acknowledgement := formedAcknowledgementRecord(t, "tenant-a", "acknowledgement-1", foundReceivable.Receivable)
	var savedAcknowledgement ports.AcknowledgementSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		savedAcknowledgement, err = acknowledgements.Save(txCtx, acknowledgement)
		return err
	})
	if savedAcknowledgement != ports.AcknowledgementSaved {
		t.Fatalf("save acknowledgement outcome = %d", savedAcknowledgement)
	}

	foundAck, exists, err := acknowledgements.FindByKey(ctx, acknowledgement.Key)
	if err != nil || !exists {
		t.Fatalf("认可读回失败：err=%v exists=%v", err, exists)
	}
	if foundAck.Acknowledgement.Standing() != domain.ResponsePartiallyAccepted ||
		foundAck.Acknowledgement.UnacknowledgedMinor() != 4000 {
		t.Fatal("认可往返变形")
	}

	second := acknowledgement
	second.ContentDigest = "digest-other"
	var outcome ports.AcknowledgementSaveOutcome
	var ackWinner ports.AcknowledgementRecord
	var ackWinnerFound bool
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		if outcome, err = acknowledgements.Save(txCtx, second); err != nil {
			return err
		}
		ackWinner, ackWinnerFound, err = acknowledgements.FindByKey(txCtx, acknowledgement.Key)
		return err
	})
	if !ackWinnerFound || ackWinner.ContentDigest != "digest-acknowledgement-1" {
		t.Fatalf("同事务读回赢家失败：found=%v digest=%q", ackWinnerFound, ackWinner.ContentDigest)
	}
	if outcome != ports.AcknowledgementAlreadyRecorded {
		t.Fatalf("第二份写入结果 = %d", outcome)
	}
}

func TestClaimAmountScopesAreInvisibleToEachOther(t *testing.T) {
	amounts, receivables, acknowledgements, transactor, _ := newClaimAmountStores(t)
	ctx := t.Context()

	amount := formedClaimAmountRecord(t, "tenant-a", "claim-amount-iso", domain.CustomerCompensationPayable)
	receivable := formedReceivableRecord(t, "tenant-a", "receivable-iso")
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := amounts.Save(txCtx, amount); err != nil {
			return err
		}
		if _, err := receivables.Save(txCtx, receivable); err != nil {
			return err
		}
		return nil
	})
	acknowledgement := formedAcknowledgementRecord(t, "tenant-a", "acknowledgement-iso", receivable.Receivable)
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := acknowledgements.Save(txCtx, acknowledgement)
		return err
	})

	other := saTenant(t, "tenant-b")
	if _, present, err := amounts.FindByKey(ctx, ports.ClaimAmountKey{TenantID: other, Amount: amount.Key.Amount}); err != nil || present {
		t.Errorf("他租户读到了本租户的索赔金额：present=%v err=%v", present, err)
	}
	if _, present, err := receivables.FindByKey(ctx, ports.ReceivableKey{TenantID: other, Receivable: receivable.Key.Receivable}); err != nil || present {
		t.Errorf("他租户读到了本租户的应追偿：present=%v err=%v", present, err)
	}
	if _, present, err := acknowledgements.FindByKey(ctx, ports.AcknowledgementKey{TenantID: other, Acknowledgement: acknowledgement.Key.Acknowledgement}); err != nil || present {
		t.Errorf("他租户读到了本租户的认可：present=%v err=%v", present, err)
	}
}

func TestClaimAmountWritesRefuseToRunOutsideATransaction(t *testing.T) {
	amounts, receivables, acknowledgements, _, _ := newClaimAmountStores(t)
	ctx := t.Context()

	if _, err := amounts.Save(ctx, formedClaimAmountRecord(t, "tenant-a", "claim-amount-ntx", domain.CustomerCompensationPayable)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存索赔金额应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := receivables.Save(ctx, formedReceivableRecord(t, "tenant-a", "receivable-ntx")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存应追偿应返回 ErrTransactionRequired，实得：%v", err)
	}
	receivable := formedReceivableRecord(t, "tenant-a", "receivable-ntx").Receivable
	if _, err := acknowledgements.Save(ctx, formedAcknowledgementRecord(t, "tenant-a", "acknowledgement-ntx", receivable)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存认可应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestClaimAmountCheckConstraintsRejectImpossibleRows(t *testing.T) {
	_, _, _, _, pool := newClaimAmountStores(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.customer_claim_amount
			(tenant_id, amount_id, kind, claim_item, responsibility_ref, rule_version,
			 legal_entity, original_charge, currency, amount_minor, period_ref,
			 formed_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'c-bad-1', 'CLAIM_CHARGE_REFUND', 'item-1', 'resp-1', 'rule-1',
		         'legal-1', NULL, 'USD', 100, 'period-1', now(), 'd', now())`); err == nil {
		t.Fatal("一行「退款却没有原费用」溜进了索赔金额库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.customer_claim_amount
			(tenant_id, amount_id, kind, claim_item, responsibility_ref, rule_version,
			 legal_entity, original_charge, currency, amount_minor, period_ref,
			 formed_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'c-bad-2', 'COMPENSATION_PAYABLE', 'item-1', 'resp-1', 'rule-1',
		         'legal-1', 'charge-1', 'USD', 100, 'period-1', now(), 'd', now())`); err == nil {
		t.Fatal("一行「赔付却指名原费用」溜进了索赔金额库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.recovery_receivable
			(tenant_id, receivable_id, matter_ref, responsibility_ref, counterparty_ref,
			 rule_version, legal_entity, currency, amount_minor, formed_at,
			 content_digest, recorded_at)
		 VALUES ('tenant-a', 'r-bad-1', 'matter-1', 'resp-1', 'partner-1',
		         'rule-1', 'legal-1', 'USD', 0, now(), 'd', now())`); err == nil {
		t.Fatal("一行「应追偿金额为零」溜进了应追偿库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.recovery_acknowledgement
			(tenant_id, acknowledgement_id, receivable_id, response_ref, standing,
			 currency, acknowledged_minor, receivable_minor, acknowledged_at,
			 content_digest, recorded_at)
		 VALUES ('tenant-a', 'a-bad-1', 'receivable-1', 'response-1', 'ACCEPTED',
		         'USD', 6000, 10000, now(), 'd', now())`); err == nil {
		t.Fatal("一行「全部接受却不等额」溜进了认可库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.recovery_acknowledgement
			(tenant_id, acknowledgement_id, receivable_id, response_ref, standing,
			 currency, acknowledged_minor, receivable_minor, acknowledged_at,
			 content_digest, recorded_at)
		 VALUES ('tenant-a', 'a-bad-2', 'receivable-1', 'response-1', 'PARTIALLY_ACCEPTED',
		         'USD', 10000, 10000, now(), 'd', now())`); err == nil {
		t.Fatal("一行「部分接受占满全额」溜进了认可库")
	}
}

func newClaimAmountStores(t *testing.T) (
	*adapter.CustomerClaimAmounts,
	*adapter.RecoveryReceivables,
	*adapter.RecoveryAcknowledgements,
	bentoapp.Transactor,
	*pgxpool.Pool,
) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	amounts, err := adapter.NewCustomerClaimAmounts(db)
	if err != nil {
		t.Fatalf("构造索赔金额库：%v", err)
	}
	receivables, err := adapter.NewRecoveryReceivables(db)
	if err != nil {
		t.Fatalf("构造应追偿库：%v", err)
	}
	acknowledgements, err := adapter.NewRecoveryAcknowledgements(db)
	if err != nil {
		t.Fatalf("构造认可库：%v", err)
	}
	return amounts, receivables, acknowledgements, db.Transactor(), pool
}

func formedClaimAmountRecord(t *testing.T, tenant, id string, kind domain.CustomerClaimAmountKind) ports.ClaimAmountRecord {
	t.Helper()
	spec := domain.CustomerClaimAmountSpec{
		ID:             saValue(t, domain.NewCustomerClaimAmountID, id),
		Kind:           kind,
		ClaimItem:      saValue(t, domain.NewClaimItemReference, "claim-item-1"),
		Responsibility: saValue(t, domain.NewResponsibilityConclusionReference, "responsibility/v1"),
		RuleVersion:    saValue(t, domain.NewAmountRuleVersionReference, "amount-rule/v1"),
		LegalEntity:    saValue(t, domain.NewLegalEntityReference, "legal-1"),
		Currency:       saValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor:    5000,
		Period:         saValue(t, domain.NewBillingPeriodReference, "period-2026-09"),
		FormedAt:       claimAmountFormedAt,
	}
	if kind == domain.ClaimChargeRefund {
		spec.OriginalCharge = saValue(t, domain.NewCustomerChargeID, "charge-1")
	}
	amount, err := domain.FormCustomerClaimAmount(spec)
	if err != nil {
		t.Fatalf("构造索赔金额：%v", err)
	}
	return ports.ClaimAmountRecord{
		Key:           ports.ClaimAmountKey{TenantID: saTenant(t, tenant), Amount: amount.ID()},
		ContentDigest: "digest-" + id,
		Amount:        amount,
		RecordedAt:    claimAmountFormedAt,
	}
}

func formedReceivableRecord(t *testing.T, tenant, id string) ports.ReceivableRecord {
	t.Helper()
	receivable, err := domain.FormRecoveryReceivable(domain.RecoveryReceivableSpec{
		ID:             saValue(t, domain.NewRecoveryReceivableID, id),
		Matter:         saValue(t, domain.NewRecoveryMatterReference, "recovery-matter-1"),
		Responsibility: saValue(t, domain.NewResponsibilityConclusionReference, "responsibility/v1"),
		Counterparty:   saValue(t, domain.NewRecoveryCounterpartyReference, "partner-1"),
		RuleVersion:    saValue(t, domain.NewAmountRuleVersionReference, "amount-rule/v1"),
		LegalEntity:    saValue(t, domain.NewLegalEntityReference, "legal-1"),
		Currency:       saValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor:    10000,
		FormedAt:       claimAmountFormedAt,
	})
	if err != nil {
		t.Fatalf("构造应追偿：%v", err)
	}
	return ports.ReceivableRecord{
		Key:           ports.ReceivableKey{TenantID: saTenant(t, tenant), Receivable: receivable.ID()},
		ContentDigest: "digest-" + id,
		Receivable:    receivable,
		RecordedAt:    claimAmountFormedAt,
	}
}

func formedAcknowledgementRecord(t *testing.T, tenant, id string, receivable domain.RecoveryReceivable) ports.AcknowledgementRecord {
	t.Helper()
	acknowledgement, err := domain.AcknowledgeRecovery(
		receivable,
		saValue(t, domain.NewAcknowledgementID, id),
		saValue(t, domain.NewCounterpartyResponseReference, "response/v1"),
		domain.ResponsePartiallyAccepted,
		6000,
		acknowledgedAtTS,
	)
	if err != nil {
		t.Fatalf("构造认可：%v", err)
	}
	return ports.AcknowledgementRecord{
		Key:             ports.AcknowledgementKey{TenantID: saTenant(t, tenant), Acknowledgement: acknowledgement.ID()},
		ContentDigest:   "digest-" + id,
		Acknowledgement: acknowledgement,
		RecordedAt:      acknowledgedAtTS,
	}
}
