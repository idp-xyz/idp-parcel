package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

func TestRehydrateConsolidationUnitCoversAllThreePhases(t *testing.T) {
	member := mustValue(t, domain.NewHandlingUnitID, "unit-1")
	snapshot := domain.RehydrateSealedSnapshotSpec{
		Members:  []domain.HandlingUnitID{member},
		Seal:     mustValue(t, domain.NewSealReference, "seal-1"),
		Basis:    mustValue(t, domain.NewWorkBasisReference, "PACK/1"),
		SealedAt: sealedAt,
	}

	t.Run("open with members and historical snapshots", func(t *testing.T) {
		unit, err := domain.RehydrateConsolidationUnit(domain.RehydrateConsolidationUnitSpec{
			ID:        mustValue(t, domain.NewConsolidationUnitID, "bag-1"),
			Asset:     mustValue(t, domain.NewCarrierAssetReference, "asset-7"),
			Phase:     domain.ConsolidationPhaseOpen,
			Members:   []domain.HandlingUnitID{member},
			Snapshots: []domain.RehydrateSealedSnapshotSpec{snapshot},
		})
		if err != nil {
			t.Fatalf("rehydrate open: %v", err)
		}
		if unit.Sealed() || unit.Closed() || len(unit.Members()) != 1 || len(unit.Snapshots()) != 1 {
			t.Fatalf("开放态往返变形：sealed=%v closed=%v members=%d snapshots=%d",
				unit.Sealed(), unit.Closed(), len(unit.Members()), len(unit.Snapshots()))
		}
	})

	t.Run("sealed members match last snapshot", func(t *testing.T) {
		unit, err := domain.RehydrateConsolidationUnit(domain.RehydrateConsolidationUnitSpec{
			ID:        mustValue(t, domain.NewConsolidationUnitID, "bag-1"),
			Asset:     mustValue(t, domain.NewCarrierAssetReference, "asset-7"),
			Phase:     domain.ConsolidationPhaseSealed,
			Members:   []domain.HandlingUnitID{member},
			Snapshots: []domain.RehydrateSealedSnapshotSpec{snapshot},
		})
		if err != nil {
			t.Fatalf("rehydrate sealed: %v", err)
		}
		if !unit.Sealed() || unit.Closed() {
			t.Fatal("封装态没有落回")
		}
	})

	t.Run("closed keeps remaining members", func(t *testing.T) {
		unit, err := domain.RehydrateConsolidationUnit(domain.RehydrateConsolidationUnitSpec{
			ID:        mustValue(t, domain.NewConsolidationUnitID, "bag-1"),
			Asset:     mustValue(t, domain.NewCarrierAssetReference, "asset-7"),
			Phase:     domain.ConsolidationPhaseClosed,
			Members:   []domain.HandlingUnitID{member},
			Snapshots: []domain.RehydrateSealedSnapshotSpec{snapshot},
			ClosedAt:  sealedAt.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("rehydrate closed: %v", err)
		}
		if !unit.Closed() || unit.ClosedAt().IsZero() || len(unit.Members()) != 1 {
			t.Fatal("关闭态往返丢失成员或时刻")
		}
	})
}

func TestRehydrateConsolidationUnitRejectsImpossibleRows(t *testing.T) {
	member := mustValue(t, domain.NewHandlingUnitID, "unit-1")
	snapshot := domain.RehydrateSealedSnapshotSpec{
		Members:  []domain.HandlingUnitID{member},
		Seal:     mustValue(t, domain.NewSealReference, "seal-1"),
		Basis:    mustValue(t, domain.NewWorkBasisReference, "PACK/1"),
		SealedAt: sealedAt,
	}
	base := domain.RehydrateConsolidationUnitSpec{
		ID:    mustValue(t, domain.NewConsolidationUnitID, "bag-1"),
		Asset: mustValue(t, domain.NewCarrierAssetReference, "asset-7"),
	}

	t.Run("sealed without snapshots", func(t *testing.T) {
		spec := base
		spec.Phase = domain.ConsolidationPhaseSealed
		spec.Members = []domain.HandlingUnitID{member}
		if _, err := domain.RehydrateConsolidationUnit(spec); !errors.Is(err, domain.ErrInvalidRehydratedConsolidation) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("sealed members diverge from last snapshot", func(t *testing.T) {
		spec := base
		spec.Phase = domain.ConsolidationPhaseSealed
		spec.Members = []domain.HandlingUnitID{mustValue(t, domain.NewHandlingUnitID, "unit-9")}
		spec.Snapshots = []domain.RehydrateSealedSnapshotSpec{snapshot}
		if _, err := domain.RehydrateConsolidationUnit(spec); !errors.Is(err, domain.ErrInvalidRehydratedConsolidation) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("closed without closedAt", func(t *testing.T) {
		spec := base
		spec.Phase = domain.ConsolidationPhaseClosed
		if _, err := domain.RehydrateConsolidationUnit(spec); !errors.Is(err, domain.ErrInvalidRehydratedConsolidation) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("open with closedAt", func(t *testing.T) {
		spec := base
		spec.Phase = domain.ConsolidationPhaseOpen
		spec.ClosedAt = sealedAt
		if _, err := domain.RehydrateConsolidationUnit(spec); !errors.Is(err, domain.ErrInvalidRehydratedConsolidation) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("unknown phase", func(t *testing.T) {
		spec := base
		spec.Phase = "PACKING"
		if _, err := domain.RehydrateConsolidationUnit(spec); !errors.Is(err, domain.ErrInvalidRehydratedConsolidation) {
			t.Fatalf("err = %v", err)
		}
	})
}
