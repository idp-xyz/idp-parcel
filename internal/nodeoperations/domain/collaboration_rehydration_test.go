package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

func TestRehydrateNodeExecutionFactRebuildsFromRowFields(t *testing.T) {
	spec := domain.RehydrateNodeExecutionFactSpec{
		TenantID:    collabValue(t, domain.NewTenantID, "tenant-1"),
		Node:        collabValue(t, domain.NewNodeReference, "node-1"),
		Item:        collabValue(t, domain.NewCollaborationItemReference, "item-1"),
		Unit:        collabValue(t, domain.NewHandlingUnitID, "unit-1"),
		Action:      domain.UnsealAction,
		Evidence:    collabValue(t, domain.NewExecutionEvidenceReference, "evidence-1"),
		PerformedAt: collaborationDecidedAt.Add(time.Hour),
	}
	fact, err := domain.RehydrateNodeExecutionFact(spec)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if fact.TenantID() != spec.TenantID ||
		fact.Item() != spec.Item ||
		fact.Unit() != spec.Unit ||
		fact.Action() != domain.UnsealAction ||
		fact.Evidence() != spec.Evidence ||
		!fact.PerformedAt().Equal(spec.PerformedAt.UTC()) {
		t.Fatalf("往返变形：%+v", fact)
	}
}

func TestRehydrateNodeExecutionFactRejectsIncompleteRows(t *testing.T) {
	valid := domain.RehydrateNodeExecutionFactSpec{
		TenantID:    collabValue(t, domain.NewTenantID, "tenant-1"),
		Node:        collabValue(t, domain.NewNodeReference, "node-1"),
		Item:        collabValue(t, domain.NewCollaborationItemReference, "item-1"),
		Unit:        collabValue(t, domain.NewHandlingUnitID, "unit-1"),
		Action:      domain.PresentAction,
		Evidence:    collabValue(t, domain.NewExecutionEvidenceReference, "evidence-1"),
		PerformedAt: collaborationDecidedAt,
	}

	t.Run("missing evidence", func(t *testing.T) {
		spec := valid
		spec.Evidence = domain.ExecutionEvidenceReference{}
		if _, err := domain.RehydrateNodeExecutionFact(spec); !errors.Is(err, domain.ErrInvalidRehydratedExecutionFact) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("invalid action", func(t *testing.T) {
		spec := valid
		spec.Action = domain.CollaborationActionKindInvalid
		if _, err := domain.RehydrateNodeExecutionFact(spec); !errors.Is(err, domain.ErrInvalidRehydratedExecutionFact) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("zero performed at", func(t *testing.T) {
		spec := valid
		spec.PerformedAt = time.Time{}
		if _, err := domain.RehydrateNodeExecutionFact(spec); !errors.Is(err, domain.ErrInvalidRehydratedExecutionFact) {
			t.Fatalf("err = %v", err)
		}
	})
}
