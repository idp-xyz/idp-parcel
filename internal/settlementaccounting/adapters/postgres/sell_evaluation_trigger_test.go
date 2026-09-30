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

func TestSellEvaluationTriggersRefuseToRunOutsideATransaction(t *testing.T) {
	triggers := newSellTriggers(t)
	registration := sellTriggerRegistration(t, "booking")

	if _, err := triggers.SaveSellEvaluationTrigger(t.Context(), registerTenant(t), registration, time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
}

func TestEmptySellEvaluationTriggersStayUnconfigured(t *testing.T) {
	triggers := newSellTriggers(t)
	reason := sellReason(t, "booking")

	_, found, err := triggers.LoadSellEvaluationTrigger(t.Context(), registerTenant(t), reason)
	if err != nil || found {
		t.Fatalf("空册 found=%v err=%v", found, err)
	}
}

func TestASellEvaluationTriggerRegistersAndReplays(t *testing.T) {
	db := catalogueDB(t)
	triggers := mustSellTriggers(t, db)
	tenant := registerTenant(t)
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	registration := sellTriggerRegistration(t, "booking")

	err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		effect, err := triggers.SaveSellEvaluationTrigger(ctx, tenant, registration, at)
		if err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		effect, err = triggers.SaveSellEvaluationTrigger(ctx, tenant, registration, at)
		if err != nil || effect != ports.CatalogueReplay {
			return errOrEffect(err, effect, ports.CatalogueReplay)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func newSellTriggers(t *testing.T) *adapter.SellEvaluationTriggers {
	t.Helper()
	return mustSellTriggers(t, catalogueDB(t))
}

func mustSellTriggers(t *testing.T, db *bentopg.DB) *adapter.SellEvaluationTriggers {
	t.Helper()
	triggers, err := adapter.NewSellEvaluationTriggers(db)
	if err != nil {
		t.Fatal(err)
	}
	return triggers
}

func sellReason(t *testing.T, value string) domain.OccurrenceReasonReference {
	t.Helper()
	reason, err := domain.NewOccurrenceReasonReference(value)
	if err != nil {
		t.Fatalf("reason: %v", err)
	}
	return reason
}

func sellTriggerRegistration(t *testing.T, reason string) domain.SellEvaluationTriggerRegistration {
	t.Helper()
	registration, err := domain.NewSellEvaluationTriggerRegistration(
		sellReason(t, reason), domain.SellEvaluationTriggerOnOccurrenceFormed)
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	return registration
}
