package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

func TestAdoptingTheMeasurementFormTwiceKeepsTheFirstBasis(t *testing.T) {
	store := &adoptionMemory{}
	handler, err := application.NewAdoptMeasurementSelectionHandler(store, clockAt(time.Date(2026, 11, 17, 1, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	tenant := mustTenant(t, "tenant-a")
	first, err := handler.Handle(context.Background(), application.AdoptMeasurementSelectionCommand{
		Tenant: tenant, Form: domain.LatestStillValidByOccurredAt, Basis: "basis-1",
	})
	if err != nil || first != application.MeasurementSelectionAdopted {
		t.Fatalf("首次 %s err=%v", first, err)
	}
	again, err := handler.Handle(context.Background(), application.AdoptMeasurementSelectionCommand{
		Tenant: tenant, Form: domain.LatestStillValidByOccurredAt, Basis: "basis-1",
	})
	if err != nil || again != application.MeasurementSelectionExisting {
		t.Fatalf("重放 %s err=%v", again, err)
	}
	conflict, err := handler.Handle(context.Background(), application.AdoptMeasurementSelectionCommand{
		Tenant: tenant, Form: domain.LatestStillValidByOccurredAt, Basis: "basis-2",
	})
	if err != nil || conflict != application.MeasurementSelectionConflict || store.saved.Basis != "basis-1" {
		t.Fatalf("冲突 %s basis=%s err=%v", conflict, store.saved.Basis, err)
	}
}

func TestABlankBasisIsNotAnAdoption(t *testing.T) {
	handler, err := application.NewAdoptMeasurementSelectionHandler(&adoptionMemory{}, clockAt(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := handler.Handle(context.Background(), application.AdoptMeasurementSelectionCommand{
		Tenant: mustTenant(t, "tenant-a"), Form: domain.LatestStillValidByOccurredAt, Basis: "  ",
	})
	if err != nil || outcome != application.MeasurementSelectionNotAccepted {
		t.Fatalf("outcome=%s err=%v", outcome, err)
	}
}

type adoptionMemory struct {
	saved ports.MeasurementSelectionAdoption
	set   bool
}

func (store *adoptionMemory) FindAdoption(context.Context, domain.TenantID) (ports.MeasurementSelectionAdoption, bool, error) {
	return store.saved, store.set, nil
}

func (store *adoptionMemory) SaveAdoption(_ context.Context, adoption ports.MeasurementSelectionAdoption) (ports.MeasurementSelectionSaveOutcome, error) {
	if !store.set {
		store.saved = adoption
		store.set = true
		return ports.MeasurementSelectionSaved, nil
	}
	if store.saved.Form == adoption.Form && store.saved.Basis == adoption.Basis {
		return ports.MeasurementSelectionAlreadyRecorded, nil
	}
	return ports.MeasurementSelectionConflicts, nil
}

type clockAt time.Time

func (clock clockAt) Now() time.Time { return time.Time(clock) }

func mustTenant(t *testing.T, raw string) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return tenant
}
