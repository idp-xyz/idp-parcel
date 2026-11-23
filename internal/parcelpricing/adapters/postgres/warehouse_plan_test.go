package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func TestAWarehousePlanNeedsTheNewerCanonicalizationSwitch(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	store, err := adapter.NewWarehousePlans(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	plan, issue := domain.NewWarehousePlan(warehousePlanFixture())
	if issue != "" {
		t.Fatal(issue)
	}
	if err := store.SavePlan(ctx, "SYN-TENANT-01", plan); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SavePlan：%v", err)
	}
	if err := store.SaveEvaluation(ctx, "SYN-TENANT-01", "WEV-1", "CHO-1", 1, plan.ID, domain.WarehouseEvaluation{Status: "ZERO", Currency: "CNY"}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveEvaluation：%v", err)
	}
	closed := application.NewWarehousePlanHandler(store, false, &sequenceIDs{})
	if outcome, err := inPricing(t, db, func(ctx context.Context) (application.WarehousePlanOutcome, error) {
		return closed.Register(ctx, "SYN-TENANT-01", warehousePlanFixture())
	}); err != nil || outcome != application.WarehousePlanNotEnabled {
		t.Fatalf("关着 %s %v", outcome, err)
	}
	opened := application.NewWarehousePlanHandler(store, true, &sequenceIDs{})
	if outcome, err := inPricing(t, db, func(ctx context.Context) (application.WarehousePlanOutcome, error) {
		return opened.Register(ctx, "SYN-TENANT-01", warehousePlanFixture())
	}); err != nil || outcome != application.WarehousePlanRecorded {
		t.Fatalf("开着 %s %v", outcome, err)
	}
	if outcome, err := inPricing(t, db, func(ctx context.Context) (application.WarehousePlanOutcome, error) {
		return opened.Register(ctx, "SYN-TENANT-01", warehousePlanFixture())
	}); err != nil || outcome != application.WarehousePlanExisting {
		t.Fatalf("重放 %s %v", outcome, err)
	}
}

func warehousePlanFixture() domain.WarehousePlan {
	return domain.WarehousePlan{
		ID: "SYN-PLAN-WH-01", Version: "1", Scope: "SYN-SCOPE-WH", Currency: "CNY", DeclaredValue: "REFUSE",
		EffectiveFrom: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		Groups: []domain.WarehouseChargeGroup{{
			Name: "INBOUND", Kind: "INBOUND_HANDLING", Level: "OCCURRENCE", OnMiss: "PENDING",
		}},
		Rules: []domain.WarehouseChargeRule{{
			Code: "INBOUND_PIECE", Kind: "INBOUND_HANDLING", Level: "OCCURRENCE", Group: "INBOUND", Priority: 1,
			Method: "CHARGE", Unit: "PIECE", UnitQuantity: "1", UnitPrice: 100, Rounding: "EXACT",
		}},
	}
}

func inPricing(t *testing.T, db *bentopg.DB, run func(context.Context) (application.WarehousePlanOutcome, error)) (application.WarehousePlanOutcome, error) {
	t.Helper()
	var outcome application.WarehousePlanOutcome
	err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		var runErr error
		outcome, runErr = run(ctx)
		return runErr
	})
	return outcome, err
}

type sequenceIDs struct{ n int }

func (ids *sequenceIDs) Next() (string, error) {
	ids.n++
	return "WEV-1", nil
}
