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
		Members: []domain.HandlingUnitID{member},
		Seal:    mustValue(t, domain.NewSealReference, "seal-1"),
		Basis:   mustValue(t, domain.NewWorkBasisReference, "PACK/1"),
		Source:  workSource(t, "src-seal-1", sealedAt),
	}
	openedBy := workSource(t, "src-open", sealedAt.Add(-time.Hour))

	t.Run("open with members and historical snapshots", func(t *testing.T) {
		unit, err := domain.RehydrateConsolidationUnit(domain.RehydrateConsolidationUnitSpec{
			ID:        mustValue(t, domain.NewConsolidationUnitID, "bag-1"),
			Asset:     mustValue(t, domain.NewCarrierAssetReference, "asset-7"),
			OpenedBy:  openedBy,
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
		// 来源那一层同样要原样落回——重建门放行却丢掉来源，等于读回一个比写入时少
		// 一层的聚合，而两者在三相上看不出差别。
		if unit.OpenedBy().SourceID() != "src-open" ||
			unit.Snapshots()[0].Source().SourceID() != "src-seal-1" ||
			!unit.Snapshots()[0].SealedAt().Equal(sealedAt) {
			t.Fatalf("来源往返变形：opened=%+v snapshot=%+v",
				unit.OpenedBy(), unit.Snapshots()[0].Source())
		}
	})

	t.Run("sealed members match last snapshot", func(t *testing.T) {
		unit, err := domain.RehydrateConsolidationUnit(domain.RehydrateConsolidationUnitSpec{
			ID:        mustValue(t, domain.NewConsolidationUnitID, "bag-1"),
			Asset:     mustValue(t, domain.NewCarrierAssetReference, "asset-7"),
			OpenedBy:  openedBy,
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
			OpenedBy:  openedBy,
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
		Members: []domain.HandlingUnitID{member},
		Seal:    mustValue(t, domain.NewSealReference, "seal-1"),
		Basis:   mustValue(t, domain.NewWorkBasisReference, "PACK/1"),
		Source:  workSource(t, "src-seal-1", sealedAt),
	}
	base := domain.RehydrateConsolidationUnitSpec{
		ID:       mustValue(t, domain.NewConsolidationUnitID, "bag-1"),
		Asset:    mustValue(t, domain.NewCarrierAssetReference, "asset-7"),
		OpenedBy: workSource(t, "src-open", sealedAt.Add(-time.Hour)),
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
	// 来源那一层缺席的行同样进不来。它拦的是「旧口径写下的行被读成合法聚合」——
	// 那样的单元说不出谁开的，而三相、成员与快照全都对得上，光看它们看不出差别。
	t.Run("opened without a source", func(t *testing.T) {
		spec := base
		spec.OpenedBy = domain.WorkFactSource{}
		spec.Phase = domain.ConsolidationPhaseOpen
		if _, err := domain.RehydrateConsolidationUnit(spec); !errors.Is(err, domain.ErrInvalidRehydratedConsolidation) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("snapshot without a source", func(t *testing.T) {
		spec := base
		spec.Phase = domain.ConsolidationPhaseOpen
		sourceless := snapshot
		sourceless.Source = domain.WorkFactSource{}
		spec.Snapshots = []domain.RehydrateSealedSnapshotSpec{sourceless}
		if _, err := domain.RehydrateConsolidationUnit(spec); !errors.Is(err, domain.ErrInvalidRehydratedConsolidation) {
			t.Fatalf("err = %v", err)
		}
	})
}
