package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func TestSafeHandoffConfirmsOnlyMatchingQueryableCompleteScope(t *testing.T) {
	spec := completeHandoffSpec(t, "scope-1")
	assessment, err := domain.AssessSafeHandoff(spec)
	if err != nil {
		t.Fatalf("assess safe handoff: %v", err)
	}
	if assessment.Status() != domain.SafeHandoffConfirmed {
		t.Fatalf("status = %s, want CONFIRMED", assessment.Status())
	}
	if _, ok := assessment.UnresolvedReason(); ok {
		t.Fatal("confirmed handoff exposed an unresolved reason")
	}
	if got, ok := assessment.ConfirmationReference(); !ok || got.String() != "confirmation-1" {
		t.Fatalf("confirmation reference = %v, %t", got, ok)
	}
	if got, ok := assessment.QueryReference(); !ok || got.String() != "query-1" {
		t.Fatalf("query reference = %v, %t", got, ok)
	}
	if got, ok := assessment.EffectiveAt(); !ok || !got.Equal(spec.EffectiveAt) {
		t.Fatalf("effective at = %v, %t", got, ok)
	}
}

func TestSafeHandoffKeepsIncompleteOutcomesUnresolved(t *testing.T) {
	continuation := mustValue(t, domain.NewOwnershipContinuationReference, "continuation-1")
	confirmation := mustValue(t, domain.NewHandoffConfirmationReference, "confirmation-1")
	tests := []struct {
		name        string
		observation domain.HandoffObservation
		edit        func(*domain.SafeHandoffAssessmentSpec)
		wantReason  domain.HandoffUnresolvedReason
	}{
		{
			name:        "complete confirmation has different scope",
			observation: domain.HandoffObservationCompleteConfirmation,
			edit: func(spec *domain.SafeHandoffAssessmentSpec) {
				spec.ConfirmedScopeDigest = mustValue(t, domain.NewAdmissionScopeDigest, "scope-2")
				spec.ConfirmationRef = confirmation
				spec.QueryRef = mustValue(t, domain.NewHandoffQueryReference, "query-1")
				spec.ContinuationRef = continuation
				spec.EffectiveAt = time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC)
			},
			wantReason: domain.HandoffUnresolvedScopeMismatch,
		},
		{
			name:        "partial confirmation",
			observation: domain.HandoffObservationPartialConfirmation,
			edit: func(spec *domain.SafeHandoffAssessmentSpec) {
				spec.ConfirmedScopeDigest = mustValue(t, domain.NewAdmissionScopeDigest, "partial-scope")
				spec.ConfirmationRef = confirmation
				spec.ContinuationRef = continuation
			},
			wantReason: domain.HandoffUnresolvedPartialConfirmation,
		},
		{
			name:        "timeout",
			observation: domain.HandoffObservationTimedOut,
			edit: func(spec *domain.SafeHandoffAssessmentSpec) {
				spec.ContinuationRef = continuation
			},
			wantReason: domain.HandoffUnresolvedTimedOut,
		},
		{
			name:        "confirmation cannot be queried",
			observation: domain.HandoffObservationQueryUnavailable,
			edit: func(spec *domain.SafeHandoffAssessmentSpec) {
				spec.ConfirmedScopeDigest = spec.Scope.Digest()
				spec.ConfirmationRef = confirmation
				spec.ContinuationRef = continuation
			},
			wantReason: domain.HandoffUnresolvedQueryUnavailable,
		},
		{
			name:        "target failure",
			observation: domain.HandoffObservationFailed,
			edit: func(spec *domain.SafeHandoffAssessmentSpec) {
				spec.ContinuationRef = continuation
			},
			wantReason: domain.HandoffUnresolvedTargetFailure,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := baseHandoffSpec(t, test.observation)
			test.edit(&spec)
			assessment, err := domain.AssessSafeHandoff(spec)
			if err != nil {
				t.Fatalf("assess safe handoff: %v", err)
			}
			if assessment.Status() != domain.SafeHandoffUnresolved {
				t.Fatalf("status = %s, want UNRESOLVED", assessment.Status())
			}
			if got, ok := assessment.UnresolvedReason(); !ok || got != test.wantReason {
				t.Fatalf("unresolved reason = %s, %t; want %s", got, ok, test.wantReason)
			}
			if _, ok := assessment.EffectiveAt(); ok {
				t.Fatal("unresolved handoff exposed an effective boundary")
			}
			if got, ok := assessment.ContinuationReference(); !ok || got != continuation {
				t.Fatalf("continuation reference = %v, %t", got, ok)
			}
		})
	}
}

func TestSafeHandoffRejectsInconsistentObservationEvidence(t *testing.T) {
	continuation := mustValue(t, domain.NewOwnershipContinuationReference, "continuation-1")
	tests := []struct {
		name string
		spec func() domain.SafeHandoffAssessmentSpec
	}{
		{
			name: "complete confirmation without query reference",
			spec: func() domain.SafeHandoffAssessmentSpec {
				spec := completeHandoffSpec(t, "scope-1")
				spec.QueryRef = domain.HandoffQueryReference{}
				return spec
			},
		},
		{
			name: "timeout without continuation",
			spec: func() domain.SafeHandoffAssessmentSpec {
				return baseHandoffSpec(t, domain.HandoffObservationTimedOut)
			},
		},
		{
			name: "timeout carries a false confirmation",
			spec: func() domain.SafeHandoffAssessmentSpec {
				spec := baseHandoffSpec(t, domain.HandoffObservationTimedOut)
				spec.ContinuationRef = continuation
				spec.ConfirmationRef = mustValue(t, domain.NewHandoffConfirmationReference, "confirmation-1")
				return spec
			},
		},
		{
			name: "query unavailable claims a query reference",
			spec: func() domain.SafeHandoffAssessmentSpec {
				spec := baseHandoffSpec(t, domain.HandoffObservationQueryUnavailable)
				spec.ConfirmedScopeDigest = spec.Scope.Digest()
				spec.ConfirmationRef = mustValue(t, domain.NewHandoffConfirmationReference, "confirmation-1")
				spec.QueryRef = mustValue(t, domain.NewHandoffQueryReference, "query-1")
				spec.ContinuationRef = continuation
				return spec
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := domain.AssessSafeHandoff(test.spec()); !errors.Is(err, domain.ErrInvalidSafeHandoff) {
				t.Fatalf("error = %v, want ErrInvalidSafeHandoff", err)
			}
		})
	}
}

func TestOtherAuthorityDecisionRequiresConfirmedHandoff(t *testing.T) {
	spec := ownershipSpec(t, domain.ProductionAuthorityOther, domain.AdmissionControlOpen, "scope-1", "rev-1")
	spec.HandoffRef = domain.HandoffConfirmationReference{}
	if _, err := domain.NewProductionOwnershipDecision(spec); !errors.Is(err, domain.ErrInvalidProductionOwnershipDecision) {
		t.Fatalf("other authority without handoff error = %v", err)
	}

	unresolvedSpec := baseHandoffSpec(t, domain.HandoffObservationTimedOut)
	unresolvedSpec.ContinuationRef = mustValue(t, domain.NewOwnershipContinuationReference, "continuation-1")
	unresolved, err := domain.AssessSafeHandoff(unresolvedSpec)
	if err != nil {
		t.Fatalf("assess unresolved handoff: %v", err)
	}
	if _, ok := unresolved.ConfirmationReference(); ok {
		t.Fatal("unresolved handoff supplied confirmation evidence")
	}
}

func TestSafeHandoffReplayIsDeterministic(t *testing.T) {
	spec := completeHandoffSpec(t, "scope-1")
	first, err := domain.AssessSafeHandoff(spec)
	if err != nil {
		t.Fatalf("first assessment: %v", err)
	}
	second, err := domain.AssessSafeHandoff(spec)
	if err != nil {
		t.Fatalf("second assessment: %v", err)
	}
	if first != second {
		t.Fatalf("same immutable handoff evidence produced different assessments: %#v != %#v", first, second)
	}
}

func baseHandoffSpec(t *testing.T, observation domain.HandoffObservation) domain.SafeHandoffAssessmentSpec {
	t.Helper()
	return domain.SafeHandoffAssessmentSpec{
		AttemptID:       mustValue(t, domain.NewHandoffAttemptID, "handoff-attempt-1"),
		Scope:           mustScope(t, "scope-ref-1", "scope-1"),
		TargetAuthority: mustValue(t, domain.NewProductionAuthorityReference, "authority-other-1"),
		Observation:     observation,
		AssessedAt:      time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC),
	}
}

func completeHandoffSpec(t *testing.T, confirmedScopeDigest string) domain.SafeHandoffAssessmentSpec {
	t.Helper()
	spec := baseHandoffSpec(t, domain.HandoffObservationCompleteConfirmation)
	spec.ConfirmedScopeDigest = mustValue(t, domain.NewAdmissionScopeDigest, confirmedScopeDigest)
	spec.ConfirmationRef = mustValue(t, domain.NewHandoffConfirmationReference, "confirmation-1")
	spec.QueryRef = mustValue(t, domain.NewHandoffQueryReference, "query-1")
	spec.EffectiveAt = time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC)
	return spec
}
