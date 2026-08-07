package domain_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func TestProductionOwnershipDecisionRetainsVersionedEvidence(t *testing.T) {
	decision := ownershipDecision(t, domain.ProductionAuthorityOther, domain.AdmissionControlPaused, "scope-1", "rev-1")

	if got := decision.DecisionID().String(); got != "decision-1" {
		t.Fatalf("decision ID = %q, want decision-1", got)
	}
	if got := decision.Scope().Reference().String(); got != "scope-ref-1" {
		t.Fatalf("scope reference = %q, want scope-ref-1", got)
	}
	if got := decision.Scope().Digest().String(); got != "scope-1" {
		t.Fatalf("scope digest = %q, want scope-1", got)
	}
	if got := decision.RuleVersion().String(); got != "rule-1" {
		t.Fatalf("rule version = %q, want rule-1", got)
	}
	if got := decision.Revision().String(); got != "rev-1" {
		t.Fatalf("revision = %q, want rev-1", got)
	}
	if got, want := decision.AsOf(), time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("as-of = %v, want %v", got, want)
	}
	if got, want := decision.DecisionAt(), time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("decision time = %v, want %v", got, want)
	}
	if got, want := decision.Validity().ValidFrom(), time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("valid from = %v, want %v", got, want)
	}
	if got, want := decision.Validity().ValidUntil(), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("valid until = %v, want %v", got, want)
	}
	if got, ok := decision.OtherAuthorityReference(); !ok || got.String() != "authority-other-1" {
		t.Fatalf("other authority reference = %v, %t", got, ok)
	}
	if got, ok := decision.HandoffReference(); !ok || got.String() != "handoff-confirmation-1" {
		t.Fatalf("handoff reference = %v, %t", got, ok)
	}
	if got, continuation, ok := decision.UnresolvedDetails(); ok || got != 0 || continuation.String() != "" {
		t.Fatalf("other-authority decision exposed unresolved details: %v, %v, %t", got, continuation, ok)
	}
	if got, ok := decision.SuspensionReference(); !ok || got.String() != "pause-1" {
		t.Fatalf("suspension reference = %v, %t", got, ok)
	}
}

func TestProductionOwnershipDecisionRejectsMissingCoreEvidence(t *testing.T) {
	base := ownershipSpec(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, "scope-1", "rev-1")
	tests := []struct {
		name string
		edit func(*domain.ProductionOwnershipDecisionSpec)
	}{
		{"decision ID", func(spec *domain.ProductionOwnershipDecisionSpec) {
			spec.DecisionID = domain.ProductionOwnershipDecisionID{}
		}},
		{"scope", func(spec *domain.ProductionOwnershipDecisionSpec) { spec.Scope = domain.AdmissionScope{} }},
		{"authority", func(spec *domain.ProductionOwnershipDecisionSpec) { spec.Authority = domain.ProductionAuthorityInvalid }},
		{"admission control", func(spec *domain.ProductionOwnershipDecisionSpec) {
			spec.AdmissionControl = domain.AdmissionControlInvalid
		}},
		{"rule version", func(spec *domain.ProductionOwnershipDecisionSpec) {
			spec.RuleVersion = domain.ProductionOwnershipRuleVersion{}
		}},
		{"validity interval", func(spec *domain.ProductionOwnershipDecisionSpec) { spec.Validity = domain.OwnershipValidityInterval{} }},
		{"revision", func(spec *domain.ProductionOwnershipDecisionSpec) {
			spec.Revision = domain.ProductionOwnershipRevision{}
		}},
		{"as-of", func(spec *domain.ProductionOwnershipDecisionSpec) { spec.AsOf = time.Time{} }},
		{"decision time", func(spec *domain.ProductionOwnershipDecisionSpec) { spec.DecisionAt = time.Time{} }},
		{"as-of outside validity", func(spec *domain.ProductionOwnershipDecisionSpec) { spec.AsOf = spec.Validity.ValidUntil() }},
	}
	for _, test := range tests {
		t.Run("missing "+test.name, func(t *testing.T) {
			spec := base
			test.edit(&spec)
			if _, err := domain.NewProductionOwnershipDecision(spec); !errors.Is(err, domain.ErrInvalidProductionOwnershipDecision) {
				t.Fatalf("error = %v, want ErrInvalidProductionOwnershipDecision", err)
			}
		})
	}
}

func TestFutureSubmissionGateUsesHalfOpenValidityAndDecisionTime(t *testing.T) {
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	spec := ownershipSpec(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, "scope-1", "rev-1")
	spec.AsOf = from
	spec.DecisionAt = from
	spec.Validity = mustValidity(t, from, until)
	decision := mustDecision(t, spec)

	atStart, err := domain.EvaluateFutureSubmissionGate(decision, decision.Scope().Digest(), decision.Revision(), from)
	if err != nil {
		t.Fatalf("evaluate at validity start: %v", err)
	}
	if !atStart.IsAllowed() {
		t.Fatalf("at validity start = %s (%v), want ALLOWED", atStart.Disposition(), atStart.BlockReasons())
	}

	atEnd, err := domain.EvaluateFutureSubmissionGate(decision, decision.Scope().Digest(), decision.Revision(), until)
	if err != nil {
		t.Fatalf("evaluate at validity end: %v", err)
	}
	if atEnd.IsAllowed() || !reflect.DeepEqual(atEnd.BlockReasons(), []domain.FutureSubmissionBlockReason{domain.FutureSubmissionDecisionStale}) {
		t.Fatalf("at validity end = %s (%v), want stale BLOCKED", atEnd.Disposition(), atEnd.BlockReasons())
	}

	beforeDecision := from.Add(-time.Nanosecond)
	lateDecisionSpec := spec
	lateDecisionSpec.DecisionAt = from.Add(time.Hour)
	lateDecision := mustDecision(t, lateDecisionSpec)
	before, err := domain.EvaluateFutureSubmissionGate(lateDecision, lateDecision.Scope().Digest(), lateDecision.Revision(), beforeDecision)
	if err != nil {
		t.Fatalf("evaluate before decision time: %v", err)
	}
	if before.IsAllowed() || !reflect.DeepEqual(before.BlockReasons(), []domain.FutureSubmissionBlockReason{domain.FutureSubmissionDecisionStale}) {
		t.Fatalf("before decision time = %s (%v), want stale BLOCKED", before.Disposition(), before.BlockReasons())
	}
}

func TestFutureSubmissionGateRetainsAndCopiesMultipleBlockReasons(t *testing.T) {
	decision := ownershipDecision(t, domain.ProductionAuthorityOther, domain.AdmissionControlPaused, "scope-1", "rev-1")
	evaluatedAt := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	gate, err := domain.EvaluateFutureSubmissionGate(
		decision,
		mustValue(t, domain.NewAdmissionScopeDigest, "scope-2"),
		mustValue(t, domain.NewProductionOwnershipRevision, "rev-2"),
		evaluatedAt,
	)
	if err != nil {
		t.Fatalf("evaluate gate: %v", err)
	}
	want := []domain.FutureSubmissionBlockReason{
		domain.FutureSubmissionScopeMismatch,
		domain.FutureSubmissionDecisionStale,
		domain.FutureSubmissionOtherAuthority,
		domain.FutureSubmissionAdmissionPaused,
	}
	if !reflect.DeepEqual(gate.BlockReasons(), want) {
		t.Fatalf("block reasons = %v, want %v", gate.BlockReasons(), want)
	}
	if gate.Disposition() != domain.FutureSubmissionBlocked {
		t.Fatalf("disposition = %s, want BLOCKED", gate.Disposition())
	}
	if gate.Decision().DecisionID() != decision.DecisionID() ||
		gate.ExpectedScopeDigest() != mustValue(t, domain.NewAdmissionScopeDigest, "scope-2") ||
		gate.ExpectedRevision() != mustValue(t, domain.NewProductionOwnershipRevision, "rev-2") ||
		!gate.EvaluatedAt().Equal(evaluatedAt) {
		t.Fatal("gate did not retain decision/evaluation evidence")
	}

	reasons := gate.BlockReasons()
	reasons[0] = domain.FutureSubmissionAdmissionPaused
	if reflect.DeepEqual(gate.BlockReasons(), reasons) {
		t.Fatal("gate block reasons changed through returned slice")
	}
}

func TestPausedDecisionRemainsImmutableAfterExplicitRecovery(t *testing.T) {
	paused := ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlPaused, "scope-1", "rev-1")
	pauseRef, ok := paused.SuspensionReference()
	if !ok {
		t.Fatal("paused decision has no suspension reference")
	}
	resumedSpec := ownershipSpec(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, "scope-1", "rev-2")
	resumedSpec.DecisionID = mustValue(t, domain.NewProductionOwnershipDecisionID, "decision-2")
	resumedSpec.DecisionAt = time.Date(2026, 8, 8, 9, 1, 0, 0, time.UTC)
	resumed := mustDecision(t, resumedSpec)
	if resumed.AdmissionControl() != domain.AdmissionControlOpen || resumed.Revision() == paused.Revision() {
		t.Fatal("explicit recovery did not create a new open revision")
	}
	if got, ok := paused.SuspensionReference(); !ok || got != pauseRef {
		t.Fatal("prior pause evidence was changed after recovery")
	}
}
