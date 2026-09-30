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

func TestSettlementMomentsRefuseToRunOutsideATransaction(t *testing.T) {
	moments := newSettlementMoments(t)
	if _, err := moments.SaveSettlementMoment(t.Context(), registerTenant(t), domain.SettlementMomentConfirm, time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
}

func TestSettlementMomentReplaysAndMissesAsUnconfigured(t *testing.T) {
	db := catalogueDB(t)
	moments := mustSettlementMoments(t, db)
	tenant := registerTenant(t)
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	if found, err := moments.LoadSettlementMoment(t.Context(), tenant, domain.SettlementMomentConfirm); err != nil || found {
		t.Fatalf("空册 found=%v err=%v", found, err)
	}
	err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if effect, err := moments.SaveSettlementMoment(ctx, tenant, domain.SettlementMomentConfirm, at); err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		if effect, err := moments.SaveSettlementMoment(ctx, tenant, domain.SettlementMomentConfirm, at); err != nil || effect != ports.CatalogueReplay {
			return errOrEffect(err, effect, ports.CatalogueReplay)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found, err := moments.LoadSettlementMoment(t.Context(), tenant, domain.SettlementMomentCutOff); err != nil || found {
		t.Fatalf("未登记截单 found=%v err=%v", found, err)
	}
}

func newSettlementMoments(t *testing.T) *adapter.SettlementMoments {
	t.Helper()
	return mustSettlementMoments(t, catalogueDB(t))
}

func mustSettlementMoments(t *testing.T, db *bentopg.DB) *adapter.SettlementMoments {
	t.Helper()
	moments, err := adapter.NewSettlementMoments(db)
	if err != nil {
		t.Fatal(err)
	}
	return moments
}
