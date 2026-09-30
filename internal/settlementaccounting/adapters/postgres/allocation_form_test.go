package postgres_test

import (
	"context"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestAllocationFormChoiceReplaysConflictsAndMissesAsUnconfigured(t *testing.T) {
	db := catalogueDB(t)
	forms, err := adapter.NewAllocationForms(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := registerTenant(t)
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	rule := allocationFormRule(t, "rule-1/v1")
	byWeight := allocationFormRegistration(t, rule, domain.AllocationByWeight)

	if _, found, err := forms.LoadAllocationForm(t.Context(), tenant, rule); err != nil || found {
		t.Fatalf("空册 found=%v err=%v", found, err)
	}

	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if effect, err := forms.SaveAllocationForm(ctx, tenant, byWeight, at); err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		if effect, err := forms.SaveAllocationForm(ctx, tenant, byWeight, at); err != nil || effect != ports.CatalogueReplay {
			return errOrEffect(err, effect, ports.CatalogueReplay)
		}
		other := allocationFormRegistration(t, rule, domain.AllocationByPiece)
		if effect, err := forms.SaveAllocationForm(ctx, tenant, other, at); err != nil || effect != ports.CatalogueConflict {
			return errOrEffect(err, effect, ports.CatalogueConflict)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	loaded, found, err := forms.LoadAllocationForm(t.Context(), tenant, rule)
	if err != nil || !found || loaded != domain.AllocationByWeight {
		t.Fatalf("读回 form=%s found=%v err=%v", loaded, found, err)
	}
}

func allocationFormRule(t *testing.T, value string) domain.AllocationRuleVersionReference {
	t.Helper()
	rule, err := domain.NewAllocationRuleVersionReference(value)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func allocationFormRegistration(
	t *testing.T,
	rule domain.AllocationRuleVersionReference,
	form domain.AllocationForm,
) domain.AllocationFormRegistration {
	t.Helper()
	registration, err := domain.NewAllocationFormRegistration(rule, form)
	if err != nil {
		t.Fatal(err)
	}
	return registration
}
