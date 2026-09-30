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

func TestPeriodicFeesRefuseToRunOutsideATransaction(t *testing.T) {
	fees := newPeriodicFees(t)
	registration := minimumSpendRegistration(t, "periodic-1/v1", 80_000)

	if _, err := fees.SavePeriodicFee(t.Context(), registerTenant(t), registration, time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
}

func TestEmptyPeriodicFeesStayUnconfigured(t *testing.T) {
	fees := newPeriodicFees(t)
	rule := periodicRule(t, "periodic-1/v1")

	_, found, err := fees.LoadPeriodicFee(t.Context(), registerTenant(t), rule)
	if err != nil || found {
		t.Fatalf("空册 found=%v err=%v", found, err)
	}
}

func TestAPeriodicFeeRegistersReplaysAndConflicts(t *testing.T) {
	db := catalogueDB(t)
	fees := mustPeriodicFees(t, db)
	tenant := registerTenant(t)
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	first := minimumSpendRegistration(t, "periodic-1/v1", 80_000)

	err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		effect, err := fees.SavePeriodicFee(ctx, tenant, first, at)
		if err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		effect, err = fees.SavePeriodicFee(ctx, tenant, first, at)
		if err != nil || effect != ports.CatalogueReplay {
			return errOrEffect(err, effect, ports.CatalogueReplay)
		}
		other := minimumSpendRegistration(t, "periodic-1/v1", 70_000)
		effect, err = fees.SavePeriodicFee(ctx, tenant, other, at)
		if err != nil || effect != ports.CatalogueConflict {
			return errOrEffect(err, effect, ports.CatalogueConflict)
		}
		rebate := tieredRebateRegistration(t, "periodic-2/v1")
		effect, err = fees.SavePeriodicFee(ctx, tenant, rebate, at)
		if err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	loaded, found, err := fees.LoadPeriodicFee(t.Context(), tenant, periodicRule(t, "periodic-1/v1"))
	if err != nil || !found || loaded.Form() != domain.MinimumSpend || loaded.MinimumMinor() != 80_000 {
		t.Fatalf("minimum found=%v err=%v form=%s minimum=%d", found, err, loaded.Form(), loaded.MinimumMinor())
	}
	rebate, found, err := fees.LoadPeriodicFee(t.Context(), tenant, periodicRule(t, "periodic-2/v1"))
	if err != nil || !found || rebate.Form() != domain.TieredRebate || len(rebate.Tiers()) != 2 {
		t.Fatalf("rebate found=%v err=%v tiers=%d", found, err, len(rebate.Tiers()))
	}
}

func newPeriodicFees(t *testing.T) *adapter.PeriodicFees {
	t.Helper()
	return mustPeriodicFees(t, catalogueDB(t))
}

func mustPeriodicFees(t *testing.T, db *bentopg.DB) *adapter.PeriodicFees {
	t.Helper()
	fees, err := adapter.NewPeriodicFees(db)
	if err != nil {
		t.Fatal(err)
	}
	return fees
}

func periodicRule(t *testing.T, value string) domain.PeriodicFeeRuleReference {
	t.Helper()
	rule, err := domain.NewPeriodicFeeRuleReference(value)
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	return rule
}

func minimumSpendRegistration(t *testing.T, rule string, minimum int64) domain.PeriodicFeeRegistration {
	t.Helper()
	terms, err := domain.NewMinimumSpendTerms(minimum)
	if err != nil {
		t.Fatalf("terms: %v", err)
	}
	registration, err := domain.NewPeriodicFeeRegistration(periodicRule(t, rule), terms)
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	return registration
}

func tieredRebateRegistration(t *testing.T, rule string) domain.PeriodicFeeRegistration {
	t.Helper()
	bounded, err := domain.NewRebateTier(10_000, 500)
	if err != nil {
		t.Fatalf("tier: %v", err)
	}
	open, err := domain.NewOpenRebateTier(1_000)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	terms, err := domain.NewTieredRebateTerms([]domain.RebateTier{bounded, open})
	if err != nil {
		t.Fatalf("terms: %v", err)
	}
	registration, err := domain.NewPeriodicFeeRegistration(periodicRule(t, rule), terms)
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	return registration
}
