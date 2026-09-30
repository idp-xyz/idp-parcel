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

func TestChargeAttributionsRefuseToRunOutsideATransaction(t *testing.T) {
	attributions := newChargeAttributions(t)
	registration := chargeAttributionRegistration(t)

	if _, err := attributions.SaveChargeAttribution(t.Context(), registerTenant(t), registration, time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
}

func TestChargeAttributionReplaysAndMissesAsUnconfigured(t *testing.T) {
	db := catalogueDB(t)
	attributions := mustChargeAttributions(t, db)
	tenant := registerTenant(t)
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	registration := chargeAttributionRegistration(t)

	if _, found, err := attributions.LoadChargeAttribution(t.Context(), tenant, registration.FeeItem()); err != nil || found {
		t.Fatalf("空册 found=%v err=%v", found, err)
	}

	err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if effect, err := attributions.SaveChargeAttribution(ctx, tenant, registration, at); err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		if effect, err := attributions.SaveChargeAttribution(ctx, tenant, registration, at); err != nil || effect != ports.CatalogueReplay {
			return errOrEffect(err, effect, ports.CatalogueReplay)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	loaded, found, err := attributions.LoadChargeAttribution(t.Context(), tenant, registration.FeeItem())
	if err != nil || !found || !loaded.SameRegistration(registration) {
		t.Fatalf("读回 found=%v err=%v form=%s", found, err, loaded.Form())
	}
}

func newChargeAttributions(t *testing.T) *adapter.ChargeAttributions {
	t.Helper()
	return mustChargeAttributions(t, catalogueDB(t))
}

func mustChargeAttributions(t *testing.T, db *bentopg.DB) *adapter.ChargeAttributions {
	t.Helper()
	attributions, err := adapter.NewChargeAttributions(db)
	if err != nil {
		t.Fatal(err)
	}
	return attributions
}

func chargeAttributionRegistration(t *testing.T) domain.ChargeAttributionRegistration {
	t.Helper()
	fee, err := domain.NewFeeItemReference("BASE")
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domain.NewChargeAttributionRegistration(fee, domain.ChargeAttributionSourceOccurred, "UTC", 18*60)
	if err != nil {
		t.Fatal(err)
	}
	return registration
}
