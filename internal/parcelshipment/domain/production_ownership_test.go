package domain_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func TestProductionOwnershipDecisionKeepsAuthorityAndAdmissionOrthogonal(t *testing.T) {
	tests := []struct {
		name             string
		authority        domain.ProductionAuthorityKind
		admission        domain.AdmissionControl
		wantDisposition  domain.FutureSubmissionDisposition
		wantBlockReasons []domain.FutureSubmissionBlockReason
	}{
		{
			name:             "parcel open",
			authority:        domain.ProductionAuthorityIDPParcel,
			admission:        domain.AdmissionControlOpen,
			wantDisposition:  domain.FutureSubmissionAllowed,
			wantBlockReasons: nil,
		},
		{
			name:             "parcel paused",
			authority:        domain.ProductionAuthorityIDPParcel,
			admission:        domain.AdmissionControlPaused,
			wantDisposition:  domain.FutureSubmissionBlocked,
			wantBlockReasons: []domain.FutureSubmissionBlockReason{domain.FutureSubmissionAdmissionPaused},
		},
		{
			name:             "other open",
			authority:        domain.ProductionAuthorityOther,
			admission:        domain.AdmissionControlOpen,
			wantDisposition:  domain.FutureSubmissionBlocked,
			wantBlockReasons: []domain.FutureSubmissionBlockReason{domain.FutureSubmissionOtherAuthority},
		},
		{
			name:             "other paused",
			authority:        domain.ProductionAuthorityOther,
			admission:        domain.AdmissionControlPaused,
			wantDisposition:  domain.FutureSubmissionBlocked,
			wantBlockReasons: []domain.FutureSubmissionBlockReason{domain.FutureSubmissionOtherAuthority, domain.FutureSubmissionAdmissionPaused},
		},
		{
			name:             "unresolved open",
			authority:        domain.ProductionAuthorityUnresolved,
			admission:        domain.AdmissionControlOpen,
			wantDisposition:  domain.FutureSubmissionBlocked,
			wantBlockReasons: []domain.FutureSubmissionBlockReason{domain.FutureSubmissionAuthorityUnresolved},
		},
		{
			name:             "unresolved paused",
			authority:        domain.ProductionAuthorityUnresolved,
			admission:        domain.AdmissionControlPaused,
			wantDisposition:  domain.FutureSubmissionBlocked,
			wantBlockReasons: []domain.FutureSubmissionBlockReason{domain.FutureSubmissionAuthorityUnresolved, domain.FutureSubmissionAdmissionPaused},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := ownershipDecision(t, test.authority, test.admission, "scope-1", "rev-1")
			gate, err := domain.EvaluateFutureSubmissionGate(
				decision,
				decision.Scope().Digest(),
				decision.Revision(),
				time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC),
			)
			if err != nil {
				t.Fatalf("evaluate gate: %v", err)
			}
			if got := gate.Disposition(); got != test.wantDisposition {
				t.Fatalf("disposition = %s, want %s", got, test.wantDisposition)
			}
			if got := gate.BlockReasons(); !reflect.DeepEqual(got, test.wantBlockReasons) {
				t.Fatalf("block reasons = %v, want %v", got, test.wantBlockReasons)
			}
		})
	}
}

func TestProductionOwnershipDecisionRequiresConditionalEvidence(t *testing.T) {
	base := ownershipSpec(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, "scope-1", "rev-1")
	otherRef := mustValue(t, domain.NewProductionAuthorityReference, "authority-other-1")
	continuation := mustValue(t, domain.NewOwnershipContinuationReference, "continue-1")
	pause := mustValue(t, domain.NewOwnershipSuspensionReference, "pause-1")

	tests := []struct {
		name string
		edit func(*domain.ProductionOwnershipDecisionSpec)
	}{
		{
			name: "other without authority reference",
			edit: func(spec *domain.ProductionOwnershipDecisionSpec) {
				spec.Authority = domain.ProductionAuthorityOther
			},
		},
		{
			name: "unresolved without reason",
			edit: func(spec *domain.ProductionOwnershipDecisionSpec) {
				spec.Authority = domain.ProductionAuthorityUnresolved
				spec.ContinuationRef = continuation
			},
		},
		{
			name: "unresolved without continuation",
			edit: func(spec *domain.ProductionOwnershipDecisionSpec) {
				spec.Authority = domain.ProductionAuthorityUnresolved
				spec.UnresolvedReason = domain.OwnershipUnresolvedHandoffIncomplete
			},
		},
		{
			name: "paused without suspension reference",
			edit: func(spec *domain.ProductionOwnershipDecisionSpec) {
				spec.AdmissionControl = domain.AdmissionControlPaused
			},
		},
		{
			name: "parcel carries other authority reference",
			edit: func(spec *domain.ProductionOwnershipDecisionSpec) {
				spec.OtherAuthorityRef = otherRef
			},
		},
		{
			name: "open carries suspension reference",
			edit: func(spec *domain.ProductionOwnershipDecisionSpec) {
				spec.SuspensionRef = pause
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := base
			test.edit(&spec)
			if _, err := domain.NewProductionOwnershipDecision(spec); !errors.Is(err, domain.ErrInvalidProductionOwnershipDecision) {
				t.Fatalf("error = %v, want ErrInvalidProductionOwnershipDecision", err)
			}
		})
	}
}

func TestFutureSubmissionGateBlocksScopeRevisionAndExpiredDecisions(t *testing.T) {
	decision := ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, "scope-1", "rev-1")
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name             string
		scopeDigest      string
		revision         string
		evaluatedAt      time.Time
		wantBlockReasons []domain.FutureSubmissionBlockReason
	}{
		{
			name:             "scope mismatch",
			scopeDigest:      "scope-2",
			revision:         "rev-1",
			evaluatedAt:      now,
			wantBlockReasons: []domain.FutureSubmissionBlockReason{domain.FutureSubmissionScopeMismatch},
		},
		{
			name:             "revision mismatch",
			scopeDigest:      "scope-1",
			revision:         "rev-2",
			evaluatedAt:      now,
			wantBlockReasons: []domain.FutureSubmissionBlockReason{domain.FutureSubmissionDecisionStale},
		},
		{
			name:             "outside validity interval",
			scopeDigest:      "scope-1",
			revision:         "rev-1",
			evaluatedAt:      time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			wantBlockReasons: []domain.FutureSubmissionBlockReason{domain.FutureSubmissionDecisionStale},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gate, err := domain.EvaluateFutureSubmissionGate(
				decision,
				mustValue(t, domain.NewAdmissionScopeDigest, test.scopeDigest),
				mustValue(t, domain.NewProductionOwnershipRevision, test.revision),
				test.evaluatedAt,
			)
			if err != nil {
				t.Fatalf("evaluate gate: %v", err)
			}
			if gate.Disposition() != domain.FutureSubmissionBlocked {
				t.Fatalf("disposition = %s, want BLOCKED", gate.Disposition())
			}
			if got := gate.BlockReasons(); !reflect.DeepEqual(got, test.wantBlockReasons) {
				t.Fatalf("block reasons = %v, want %v", got, test.wantBlockReasons)
			}
		})
	}
}

func TestPausedAdmissionDoesNotAutoResumeAfterReasonClears(t *testing.T) {
	paused := ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlPaused, "scope-1", "rev-1")
	clearedAt := time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC)

	gate, err := domain.EvaluateFutureSubmissionGate(
		paused,
		paused.Scope().Digest(),
		paused.Revision(),
		clearedAt,
	)
	if err != nil {
		t.Fatalf("evaluate paused gate: %v", err)
	}
	if gate.Disposition() != domain.FutureSubmissionBlocked ||
		!reflect.DeepEqual(gate.BlockReasons(), []domain.FutureSubmissionBlockReason{domain.FutureSubmissionAdmissionPaused}) {
		t.Fatalf("reason-cleared pause was not retained: %v", gate.BlockReasons())
	}
	if paused.AdmissionControl() != domain.AdmissionControlPaused {
		t.Fatal("reason clearance mutated the original decision")
	}

	resumedSpec := ownershipSpec(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, "scope-1", "rev-2")
	resumedSpec.DecisionID = mustValue(t, domain.NewProductionOwnershipDecisionID, "decision-2")
	resumedSpec.DecisionAt = clearedAt.Add(time.Minute)
	resumed := mustDecision(t, resumedSpec)
	resumedGate, err := domain.EvaluateFutureSubmissionGate(
		resumed,
		resumed.Scope().Digest(),
		resumed.Revision(),
		clearedAt.Add(2*time.Minute),
	)
	if err != nil {
		t.Fatalf("evaluate resumed gate: %v", err)
	}
	if resumedGate.Disposition() != domain.FutureSubmissionAllowed {
		t.Fatalf("explicit resumed revision = %s, want ALLOWED", resumedGate.Disposition())
	}
}

func ownershipDecision(
	t *testing.T,
	authority domain.ProductionAuthorityKind,
	control domain.AdmissionControl,
	scopeDigest string,
	revision string,
) domain.ProductionOwnershipDecision {
	t.Helper()
	return mustDecision(t, ownershipSpec(t, authority, control, scopeDigest, revision))
}

func ownershipSpec(
	t *testing.T,
	authority domain.ProductionAuthorityKind,
	control domain.AdmissionControl,
	scopeDigest string,
	revision string,
) domain.ProductionOwnershipDecisionSpec {
	t.Helper()
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	scope := mustScope(t, "scope-ref-1", scopeDigest)
	spec := domain.ProductionOwnershipDecisionSpec{
		DecisionID:       mustValue(t, domain.NewProductionOwnershipDecisionID, "decision-1"),
		Scope:            scope,
		Authority:        authority,
		AdmissionControl: control,
		RuleVersion:      mustValue(t, domain.NewProductionOwnershipRuleVersion, "rule-1"),
		AsOf:             time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
		Validity:         mustValidity(t, from, until),
		Revision:         mustValue(t, domain.NewProductionOwnershipRevision, revision),
		DecisionAt:       time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC),
	}
	if authority == domain.ProductionAuthorityOther {
		spec.OtherAuthorityRef = mustValue(t, domain.NewProductionAuthorityReference, "authority-other-1")
		spec.HandoffRef = mustValue(t, domain.NewHandoffConfirmationReference, "handoff-confirmation-1")
	}
	if authority == domain.ProductionAuthorityUnresolved {
		spec.UnresolvedReason = domain.OwnershipUnresolvedHandoffIncomplete
		spec.ContinuationRef = mustValue(t, domain.NewOwnershipContinuationReference, "continue-1")
	}
	if control == domain.AdmissionControlPaused {
		spec.SuspensionRef = mustValue(t, domain.NewOwnershipSuspensionReference, "pause-1")
	}
	return spec
}

func mustScope(t *testing.T, reference, digest string) domain.AdmissionScope {
	t.Helper()
	scope, err := domain.NewAdmissionScope(
		mustValue(t, domain.NewAdmissionScopeReference, reference),
		mustValue(t, domain.NewAdmissionScopeDigest, digest),
	)
	if err != nil {
		t.Fatalf("new admission scope: %v", err)
	}
	return scope
}

func mustValidity(t *testing.T, from, until time.Time) domain.OwnershipValidityInterval {
	t.Helper()
	validity, err := domain.NewOwnershipValidityInterval(from, until)
	if err != nil {
		t.Fatalf("new ownership validity: %v", err)
	}
	return validity
}

func mustDecision(t *testing.T, spec domain.ProductionOwnershipDecisionSpec) domain.ProductionOwnershipDecision {
	t.Helper()
	decision, err := domain.NewProductionOwnershipDecision(spec)
	if err != nil {
		t.Fatalf("new ownership decision: %v", err)
	}
	return decision
}
