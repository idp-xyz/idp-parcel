package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// Covers: S02-AT-04（只有完整、范围匹配且可查询的确认才成立）
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

// Covers: S02-AT-05（任何不完整的结果都保持未决并保留续办引用）
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
		{
			// ADR-0128 决定三：通往他方权威的出向通道未配置时什么都没投递出去，它自成一格
			// 而不充作`查询不可用`——后者的证据形要求对方已经给过确认引用。
			name:        "channel unconfigured",
			observation: domain.HandoffObservationChannelUnconfigured,
			edit: func(spec *domain.SafeHandoffAssessmentSpec) {
				spec.ContinuationRef = continuation
			},
			wantReason: domain.HandoffUnresolvedChannelUnconfigured,
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
		{
			// 通道都没配置，任何确认都不可能是对方给的；带着一个就是编造证据。
			name: "channel unconfigured carries a confirmation",
			spec: func() domain.SafeHandoffAssessmentSpec {
				spec := baseHandoffSpec(t, domain.HandoffObservationChannelUnconfigured)
				spec.ContinuationRef = continuation
				spec.ConfirmationRef = mustValue(t, domain.NewHandoffConfirmationReference, "confirmation-1")
				return spec
			},
		},
		{
			name: "channel unconfigured without continuation",
			spec: func() domain.SafeHandoffAssessmentSpec {
				return baseHandoffSpec(t, domain.HandoffObservationChannelUnconfigured)
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

// Covers: S02-AT-04（没有确认引用就不能判定为 OTHER 权威）
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

// Covers: ADR-0128 决定四——`其他权威`决定上停写证据（HandoffRef）与安全交接评估分格：评估记上去
// 之后两格各自可读、HandoffRef 原义不动；已确认的评估带确认引用与生效时刻，未决的带原因与续办引用。
// 两格不合并：合并会让「有停写证据但交接失败」与「无停写证据」在记录上不可分。
func TestOtherAuthorityDecisionRecordsSafeHandoffBesideStopEvidence(t *testing.T) {
	decision := mustDecision(t, ownershipSpec(t, domain.ProductionAuthorityOther, domain.AdmissionControlOpen, "scope-1", "rev-1"))
	if _, recorded := decision.SafeHandoff(); recorded {
		t.Fatal("a fresh other-authority decision already carried a safe handoff assessment")
	}

	t.Run("confirmed", func(t *testing.T) {
		confirmed, err := domain.AssessSafeHandoff(completeHandoffSpec(t, "scope-1"))
		if err != nil {
			t.Fatalf("assess: %v", err)
		}
		withHandoff, err := decision.WithSafeHandoff(confirmed)
		if err != nil {
			t.Fatalf("with safe handoff: %v", err)
		}
		recorded, present := withHandoff.SafeHandoff()
		if !present || recorded != confirmed {
			t.Fatalf("safe handoff = %#v, %t; want the recorded assessment", recorded, present)
		}
		if stop, present := withHandoff.HandoffReference(); !present || stop.String() != "handoff-confirmation-1" {
			t.Fatalf("stop evidence = %v, %t; recording the assessment must not touch HandoffRef", stop, present)
		}
		if withHandoff.Authority() != domain.ProductionAuthorityOther || withHandoff.DecisionID() != decision.DecisionID() {
			t.Fatal("recording the assessment changed the decision's identity or authority")
		}
		if _, present := decision.SafeHandoff(); present {
			t.Fatal("recording mutated the original decision value")
		}
	})

	t.Run("unresolved", func(t *testing.T) {
		spec := baseHandoffSpec(t, domain.HandoffObservationTimedOut)
		spec.ContinuationRef = mustValue(t, domain.NewOwnershipContinuationReference, "continuation-1")
		unresolved, err := domain.AssessSafeHandoff(spec)
		if err != nil {
			t.Fatalf("assess: %v", err)
		}
		withHandoff, err := decision.WithSafeHandoff(unresolved)
		if err != nil {
			t.Fatalf("with safe handoff: %v", err)
		}
		recorded, present := withHandoff.SafeHandoff()
		if !present || recorded.Status() != domain.SafeHandoffUnresolved {
			t.Fatalf("safe handoff = %#v, %t; want the unresolved assessment", recorded, present)
		}
		if reason, ok := recorded.UnresolvedReason(); !ok || reason != domain.HandoffUnresolvedTimedOut {
			t.Fatalf("unresolved reason = %s, %t", reason, ok)
		}
		if _, _, present := withHandoff.UnresolvedDetails(); present {
			t.Fatal("a handoff that stayed unresolved leaked into the ownership-unresolved cell")
		}
	})
}

// Covers: ADR-0128 决定二与四——评估只能记到`其他权威`决定上，且必须是对同一范围、同一目标权威的
// 那一次；本产品或权威未决的决定没有这一格；记过一次不许再记（一次决定对应一次交接尝试）。
func TestSafeHandoffOnlyRecordsOntoTheMatchingOtherAuthorityDecision(t *testing.T) {
	confirmed, err := domain.AssessSafeHandoff(completeHandoffSpec(t, "scope-1"))
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	other := mustDecision(t, ownershipSpec(t, domain.ProductionAuthorityOther, domain.AdmissionControlOpen, "scope-1", "rev-1"))

	tests := []struct {
		name       string
		decision   domain.ProductionOwnershipDecision
		assessment domain.SafeHandoffAssessment
	}{
		{
			name:       "this product's decision has no handoff cell",
			decision:   mustDecision(t, ownershipSpec(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, "scope-1", "rev-1")),
			assessment: confirmed,
		},
		{
			name:       "an unresolved-ownership decision has no handoff cell",
			decision:   mustDecision(t, ownershipSpec(t, domain.ProductionAuthorityUnresolved, domain.AdmissionControlOpen, "scope-1", "rev-1")),
			assessment: confirmed,
		},
		{
			name:     "assessment for another scope",
			decision: other,
			assessment: func() domain.SafeHandoffAssessment {
				spec := completeHandoffSpec(t, "scope-2")
				spec.Scope = mustScope(t, "scope-ref-2", "scope-2")
				assessment, err := domain.AssessSafeHandoff(spec)
				if err != nil {
					t.Fatalf("assess: %v", err)
				}
				return assessment
			}(),
		},
		{
			name:     "assessment for another target authority",
			decision: other,
			assessment: func() domain.SafeHandoffAssessment {
				spec := completeHandoffSpec(t, "scope-1")
				spec.TargetAuthority = mustValue(t, domain.NewProductionAuthorityReference, "authority-other-2")
				assessment, err := domain.AssessSafeHandoff(spec)
				if err != nil {
					t.Fatalf("assess: %v", err)
				}
				return assessment
			}(),
		},
		{
			name:       "a zero assessment",
			decision:   other,
			assessment: domain.SafeHandoffAssessment{},
		},
		{
			name: "a second recording",
			decision: func() domain.ProductionOwnershipDecision {
				once, err := other.WithSafeHandoff(confirmed)
				if err != nil {
					t.Fatalf("first recording: %v", err)
				}
				return once
			}(),
			assessment: confirmed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.decision.WithSafeHandoff(test.assessment); !errors.Is(err, domain.ErrInvalidProductionOwnershipDecision) {
				t.Fatalf("error = %v, want ErrInvalidProductionOwnershipDecision", err)
			}
		})
	}
}

// Covers: ADR-0128 决定四的门禁半边——记了评估的`其他权威`决定仍是一份合法决定，门禁照样按`其他权威`
// 阻断；评估不改归属身份，只补「交出去了没有」这一格。
func TestFutureSubmissionGateStillBlocksOtherAuthorityAfterSafeHandoff(t *testing.T) {
	decision := mustDecision(t, ownershipSpec(t, domain.ProductionAuthorityOther, domain.AdmissionControlOpen, "scope-1", "rev-1"))
	confirmed, err := domain.AssessSafeHandoff(completeHandoffSpec(t, "scope-1"))
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	withHandoff, err := decision.WithSafeHandoff(confirmed)
	if err != nil {
		t.Fatalf("with safe handoff: %v", err)
	}

	gate, err := domain.EvaluateFutureSubmissionGate(
		withHandoff,
		mustValue(t, domain.NewAdmissionScopeDigest, "scope-1"),
		mustValue(t, domain.NewProductionOwnershipRevision, "rev-1"),
		time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("evaluate gate: %v", err)
	}
	if gate.IsAllowed() {
		t.Fatal("a confirmed handoff to another authority let this product build the request")
	}
	if reasons := gate.BlockReasons(); len(reasons) != 1 || reasons[0] != domain.FutureSubmissionOtherAuthority {
		t.Fatalf("block reasons = %v, want only OTHER_AUTHORITY", reasons)
	}
	if recorded, present := gate.Decision().SafeHandoff(); !present || recorded != confirmed {
		t.Fatal("the gate dropped the recorded safe handoff from its decision")
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
