package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestBuyEvaluationTriggersRefuseToRunOutsideATransaction(t *testing.T) {
	triggers := newBuyEvaluationTriggers(t)
	registration := buyEvaluationTriggerRegistration(t, "BOOKING")

	if _, err := triggers.SaveBuyEvaluationTrigger(t.Context(), registerTenant(t), registration, time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
}

func TestBuyEvaluationTriggerReplaysConflictsAndMissesAsUnconfigured(t *testing.T) {
	db := catalogueDB(t)
	triggers := mustBuyEvaluationTriggers(t, db)
	tenant := registerTenant(t)
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	booking := buyEvaluationTriggerRegistration(t, "BOOKING")
	reason := booking.Reason()

	if _, found, err := triggers.LoadBuyEvaluationTrigger(t.Context(), tenant, reason); err != nil || found {
		t.Fatalf("空册 found=%v err=%v", found, err)
	}

	err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if effect, err := triggers.SaveBuyEvaluationTrigger(ctx, tenant, booking, at); err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		if effect, err := triggers.SaveBuyEvaluationTrigger(ctx, tenant, booking, at); err != nil || effect != ports.CatalogueReplay {
			return errOrEffect(err, effect, ports.CatalogueReplay)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	loaded, found, err := triggers.LoadBuyEvaluationTrigger(t.Context(), tenant, reason)
	if err != nil || !found || loaded != domain.BuyEvaluationTriggerOnOccurrenceFormed {
		t.Fatalf("读回 moment=%s found=%v err=%v", loaded, found, err)
	}
	other, err := domain.NewOccurrenceReasonReference("CANCEL")
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := triggers.LoadBuyEvaluationTrigger(t.Context(), tenant, other); err != nil || found {
		t.Fatalf("未登记原因 found=%v err=%v", found, err)
	}
}

func newBuyEvaluationTriggers(t *testing.T) *adapter.BuyEvaluationTriggers {
	t.Helper()
	return mustBuyEvaluationTriggers(t, catalogueDB(t))
}

func mustBuyEvaluationTriggers(t *testing.T, db *bentopg.DB) *adapter.BuyEvaluationTriggers {
	t.Helper()
	triggers, err := adapter.NewBuyEvaluationTriggers(db)
	if err != nil {
		t.Fatal(err)
	}
	return triggers
}

func buyEvaluationTriggerRegistration(t *testing.T, reason string) domain.BuyEvaluationTriggerRegistration {
	t.Helper()
	reference, err := domain.NewOccurrenceReasonReference(reason)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domain.NewBuyEvaluationTriggerRegistration(reference, domain.BuyEvaluationTriggerOnOccurrenceFormed)
	if err != nil {
		t.Fatal(err)
	}
	return registration
}
