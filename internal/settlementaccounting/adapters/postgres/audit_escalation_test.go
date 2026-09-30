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

func TestAuditEscalationCeilingsRefuseToRunOutsideATransaction(t *testing.T) {
	ceilings := newEscalationCeilings(t)
	registration := escalationRegistration(t, 12_000)

	if _, err := ceilings.SaveAuditEscalationCeiling(t.Context(), registerTenant(t), registration, time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
}

func TestEmptyAuditEscalationCeilingsStayUnconfigured(t *testing.T) {
	ceilings := newEscalationCeilings(t)

	_, found, err := ceilings.LoadAuditEscalationCeiling(
		t.Context(), registerTenant(t), catalogueSupplier(t), catalogueEntity(t), catalogueCurrency(t))
	if err != nil || found {
		t.Fatalf("空册 found=%v err=%v", found, err)
	}
}

func TestAnAuditEscalationCeilingRegistersReplaysAndConflicts(t *testing.T) {
	db := catalogueDB(t)
	ceilings := mustEscalationCeilings(t, db)
	tenant := registerTenant(t)
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	first := escalationRegistration(t, 12_000)

	err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		effect, err := ceilings.SaveAuditEscalationCeiling(ctx, tenant, first, at)
		if err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		effect, err = ceilings.SaveAuditEscalationCeiling(ctx, tenant, first, at)
		if err != nil || effect != ports.CatalogueReplay {
			return errOrEffect(err, effect, ports.CatalogueReplay)
		}
		other := escalationRegistration(t, 11_999)
		effect, err = ceilings.SaveAuditEscalationCeiling(ctx, tenant, other, at)
		if err != nil || effect != ports.CatalogueConflict {
			return errOrEffect(err, effect, ports.CatalogueConflict)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	loaded, found, err := ceilings.LoadAuditEscalationCeiling(
		t.Context(), tenant, catalogueSupplier(t), catalogueEntity(t), catalogueCurrency(t))
	if err != nil || !found || loaded.LimitMinor() != 12_000 {
		t.Fatalf("ceiling found=%v err=%v limit=%d", found, err, loaded.LimitMinor())
	}
}

func newEscalationCeilings(t *testing.T) *adapter.AuditEscalationCeilings {
	t.Helper()
	return mustEscalationCeilings(t, catalogueDB(t))
}

func mustEscalationCeilings(t *testing.T, db *bentopg.DB) *adapter.AuditEscalationCeilings {
	t.Helper()
	ceilings, err := adapter.NewAuditEscalationCeilings(db)
	if err != nil {
		t.Fatal(err)
	}
	return ceilings
}

func escalationRegistration(t *testing.T, limit int64) domain.AuditEscalationCeilingRegistration {
	t.Helper()
	ceiling, err := domain.NewAuditEscalationCeiling(limit)
	if err != nil {
		t.Fatalf("ceiling: %v", err)
	}
	registration, err := domain.NewAuditEscalationCeilingRegistration(
		catalogueSupplier(t), catalogueEntity(t), catalogueCurrency(t), ceiling)
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	return registration
}
