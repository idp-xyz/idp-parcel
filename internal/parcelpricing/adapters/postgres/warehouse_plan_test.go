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

func TestWarehousePlanConditionsClassesAndDigestRoundTrip(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	store, err := adapter.NewWarehousePlans(db)
	if err != nil {
		t.Fatal(err)
	}
	plan, issue := domain.NewWarehousePlan(classedWarehousePlan("30"))
	if issue != "" {
		t.Fatal(issue)
	}
	var loaded domain.WarehousePlan
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if err := store.SavePlan(ctx, "SYN-TENANT-01", plan); err != nil {
			return err
		}
		got, found, err := store.LoadPlan(ctx, "SYN-TENANT-01", plan.ID, plan.Version)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("方案没读回")
		}
		loaded = got
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest() != plan.Digest() {
		t.Fatalf("摘要 %s → %s", plan.Digest(), loaded.Digest())
	}
	rule := loaded.Rules[0]
	if rule.Code != "STORAGE_M3" || rule.MeasureClass != "VOLUME" || rule.Unit != "M3" || len(rule.When) != 3 {
		t.Fatalf("规则 %+v", rule)
	}
	if rule.When[0].Name != "FROZEN" || rule.When[0].Equals != "无" {
		t.Fatalf("属性条件 %+v", rule.When[0])
	}
	if rule.When[1].MeasureUnit != "DAY" || rule.When[1].Operator != "GE" || rule.When[1].Threshold != "30" {
		t.Fatalf("库龄条件 %+v", rule.When[1])
	}
	if rule.When[2].MeasureClass != "VOLUME" || rule.When[2].MeasureUnit != "M3" || rule.When[2].Operator != "GT" || rule.When[2].Threshold != "0" {
		t.Fatalf("体积条件 %+v", rule.When[2])
	}
	changed, issue := domain.NewWarehousePlan(classedWarehousePlan("31"))
	if issue != "" || changed.Digest() == plan.Digest() {
		t.Fatalf("条件变了摘要 %s %s", issue, changed.Digest())
	}
}

func classedWarehousePlan(threshold string) domain.WarehousePlan {
	return domain.WarehousePlan{
		ID: "SYN-PLAN-WH-02", Version: "1", Scope: "SYN-SCOPE-WH", Currency: "CNY", DeclaredValue: "REFUSE",
		EffectiveFrom: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		Groups: []domain.WarehouseChargeGroup{{
			Name: "STORAGE", Kind: "STORAGE", Level: "LINE", Line: "INBOUND_BATCH", OnMiss: "PENDING",
		}},
		Rules: []domain.WarehouseChargeRule{{
			Code: "STORAGE_M3", Kind: "STORAGE", Level: "LINE", Line: "INBOUND_BATCH", Group: "STORAGE", Priority: 1,
			Method: "CHARGE", MeasureClass: "VOLUME", Unit: "M3", UnitQuantity: "1", UnitPrice: 100, Rounding: "EXACT",
			When: []domain.WarehouseCondition{
				{Name: "FROZEN", Equals: "无"},
				{MeasureUnit: "DAY", Operator: "GE", Threshold: threshold},
				{MeasureClass: "VOLUME", MeasureUnit: "M3", Operator: "GT", Threshold: "0"},
			},
		}},
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
