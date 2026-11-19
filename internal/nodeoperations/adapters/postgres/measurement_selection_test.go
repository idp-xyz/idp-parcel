package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func TestMeasurementSelectionAdoptionRefusesToRunOutsideATransaction(t *testing.T) {
	adoptions, _ := newMeasurementSelection(t)
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	_, err := adoptions.SaveAdoption(context.Background(), ports.MeasurementSelectionAdoption{
		TenantID: tenant, Form: domain.LatestStillValidByOccurredAt, Basis: "basis-1",
		RecordedAt: time.Date(2026, 11, 17, 1, 0, 0, 0, time.UTC),
	})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务 Save 应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestASecondAdoptionWithAnotherBasisDoesNotReplaceTheFirst(t *testing.T) {
	adoptions, transactor := newMeasurementSelection(t)
	ctx := context.Background()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	first := ports.MeasurementSelectionAdoption{
		TenantID: tenant, Form: domain.LatestStillValidByOccurredAt, Basis: "basis-1",
		RecordedAt: time.Date(2026, 11, 17, 1, 0, 0, 0, time.UTC),
	}
	// 事务回调里只做读写并交回结果，断言放在回调外。
	save := func(adoption ports.MeasurementSelectionAdoption) ports.MeasurementSelectionSaveOutcome {
		t.Helper()
		var outcome ports.MeasurementSelectionSaveOutcome
		err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			saved, err := adoptions.SaveAdoption(txCtx, adoption)
			outcome = saved
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return outcome
	}
	if outcome := save(first); outcome != ports.MeasurementSelectionSaved {
		t.Fatalf("首次 %s", outcome)
	}
	if outcome := save(first); outcome != ports.MeasurementSelectionAlreadyRecorded {
		t.Fatalf("重放 %s", outcome)
	}
	other := first
	other.Basis = "basis-2"
	if outcome := save(other); outcome != ports.MeasurementSelectionConflicts {
		t.Fatalf("冲突 %s", outcome)
	}
	found, ok, err := adoptions.FindAdoption(ctx, tenant)
	if err != nil || !ok || found.Basis != "basis-1" {
		t.Fatalf("留下 %s ok=%v err=%v", found.Basis, ok, err)
	}
}

func newMeasurementSelection(t *testing.T) (*adapter.MeasurementSelectionAdoptions, bentoapp.Transactor) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	adoptions, err := adapter.NewMeasurementSelectionAdoptions(db)
	if err != nil {
		t.Fatal(err)
	}
	return adoptions, db.Transactor()
}
