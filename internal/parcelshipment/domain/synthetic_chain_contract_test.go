package domain_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// This file is deliberately test-only. It carries the parcel-shipment half of
// the SYN-CHAIN joint checks in the PN02-SYN task pack, asserting the order
// source preservation -> production ownership -> future submission gate over
// existing pre-submission value objects. It introduces no orchestrator, port,
// aggregate, domain event, repository, transaction, or outbox.
//
// SYN-CHAIN-04 is absent on purpose: it spans party-commercial and
// settlement-accounting, neither of which owns production types, so its two
// halves belong to those contexts' own contract tests.

const syntheticChainFixtureVersion = "SYN-CHAIN-FIXTURE-v1"

var (
	syntheticChainValidFrom  = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	syntheticChainValidUntil = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	syntheticChainAsOf       = time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	syntheticChainDecidedAt  = time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC)
	syntheticChainGateAt     = time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
)

// syntheticChainScope derives the admission scope from a preserved source
// submission. Production keeps AdmissionScope opaque and unlinked from
// PayloadDigest on purpose, so the derivation lives on the test side: the joint
// checks need the lineage to assert ordering, the domain must not yet own it.
func syntheticChainScope(
	t *testing.T,
	preserved domain.SourceSubmissionFingerprint,
) domain.AdmissionScope {
	t.Helper()
	identity := preserved.Identity()
	sum := sha256.Sum256([]byte(strings.Join([]string{
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
		preserved.Digest().String(),
		syntheticChainFixtureVersion,
	}, "\x00")))
	return mustScope(
		t,
		"SYN-SCOPE-"+identity.Source().String()+"-"+identity.RequestKey().String(),
		hex.EncodeToString(sum[:]),
	)
}

func syntheticChainDecision(
	t *testing.T,
	scope domain.AdmissionScope,
	authority domain.ProductionAuthorityKind,
	revision string,
) domain.ProductionOwnershipDecision {
	t.Helper()
	return mustDecision(t, syntheticChainDecisionSpec(t, scope, authority, revision))
}

func syntheticChainDecisionSpec(
	t *testing.T,
	scope domain.AdmissionScope,
	authority domain.ProductionAuthorityKind,
	revision string,
) domain.ProductionOwnershipDecisionSpec {
	t.Helper()
	spec := domain.ProductionOwnershipDecisionSpec{
		DecisionID:       mustValue(t, domain.NewProductionOwnershipDecisionID, "SYN-CHAIN-DECISION-1"),
		Scope:            scope,
		Authority:        authority,
		AdmissionControl: domain.AdmissionControlOpen,
		RuleVersion:      mustValue(t, domain.NewProductionOwnershipRuleVersion, "SYN-CHAIN-RULE-v1"),
		AsOf:             syntheticChainAsOf,
		Validity:         mustValidity(t, syntheticChainValidFrom, syntheticChainValidUntil),
		Revision:         mustValue(t, domain.NewProductionOwnershipRevision, revision),
		DecisionAt:       syntheticChainDecidedAt,
	}
	switch authority {
	case domain.ProductionAuthorityOther:
		spec.OtherAuthorityRef = mustValue(t, domain.NewProductionAuthorityReference, "SYN-CHAIN-AUTHORITY-OTHER-1")
		spec.HandoffRef = mustValue(t, domain.NewHandoffConfirmationReference, "SYN-CHAIN-CONFIRM-1")
	case domain.ProductionAuthorityUnresolved:
		spec.UnresolvedReason = domain.OwnershipUnresolvedRuleUnavailable
		spec.ContinuationRef = mustValue(t, domain.NewOwnershipContinuationReference, "SYN-CHAIN-CONTINUE-1")
	}
	return spec
}

// Covers: SYN-CHAIN-01, S02-AT-01, S02-AT-02
func TestSyntheticChainRequiresSourceAndOwnershipBeforeFutureGate(t *testing.T) {
	preserved := sourceFingerprint(t, "SYN-TENANT-1", "SYN-CUSTOMER-1", "SYN-SOURCE-A", "SYN-KEY-1", "SYN-DIGEST-1")
	scope := syntheticChainScope(t, preserved)
	revision := mustValue(t, domain.NewProductionOwnershipRevision, "SYN-CHAIN-REV-1")

	_, err := domain.EvaluateFutureSubmissionGate(
		domain.ProductionOwnershipDecision{},
		scope.Digest(),
		revision,
		syntheticChainGateAt,
	)
	if !errors.Is(err, domain.ErrInvalidFutureSubmissionGate) {
		t.Fatalf("gate evaluated without an ownership decision: err = %v", err)
	}

	scopelessSpec := syntheticChainDecisionSpec(t, scope, domain.ProductionAuthorityIDPParcel, "SYN-CHAIN-REV-1")
	scopelessSpec.Scope = domain.AdmissionScope{}
	if _, err := domain.NewProductionOwnershipDecision(scopelessSpec); !errors.Is(err, domain.ErrInvalidProductionOwnershipDecision) {
		t.Fatalf("ownership decided without an admission scope: err = %v", err)
	}

	decision := syntheticChainDecision(t, scope, domain.ProductionAuthorityIDPParcel, "SYN-CHAIN-REV-1")
	gate, err := domain.EvaluateFutureSubmissionGate(decision, scope.Digest(), revision, syntheticChainGateAt)
	if err != nil {
		t.Fatalf("evaluate future submission gate: %v", err)
	}
	if gate.Decision().Scope().Digest() != scope.Digest() {
		t.Fatal("gate lost the scope digest derived from the preserved source")
	}

	replay, err := domain.ClassifySourceSubmission(preserved, preserved)
	if err != nil {
		t.Fatalf("classify replay: %v", err)
	}
	if replay != domain.SourceReplay {
		t.Fatalf("classification = %d, want SourceReplay", replay)
	}
	if syntheticChainScope(t, preserved).Digest() != scope.Digest() {
		t.Fatal("replayed source produced a second admission scope")
	}
}

// Covers: SYN-CHAIN-02, S02-AT-01
func TestSyntheticChainOwnProductPathYieldsOnlyFutureAllowance(t *testing.T) {
	preserved := sourceFingerprint(t, "SYN-TENANT-1", "SYN-CUSTOMER-1", "SYN-SOURCE-A", "SYN-KEY-1", "SYN-DIGEST-1")
	scope := syntheticChainScope(t, preserved)
	revision := mustValue(t, domain.NewProductionOwnershipRevision, "SYN-CHAIN-REV-1")
	decision := syntheticChainDecision(t, scope, domain.ProductionAuthorityIDPParcel, "SYN-CHAIN-REV-1")

	gate, err := domain.EvaluateFutureSubmissionGate(decision, scope.Digest(), revision, syntheticChainGateAt)
	if err != nil {
		t.Fatalf("evaluate future submission gate: %v", err)
	}
	if !gate.IsAllowed() {
		t.Fatalf("own-product path blocked: reasons = %v", gate.BlockReasons())
	}
	if reasons := gate.BlockReasons(); len(reasons) != 0 {
		t.Fatalf("allowed gate carried block reasons %v", reasons)
	}
	if got := gate.Disposition().String(); got != "ALLOWED" {
		t.Fatalf("disposition = %q; the gate must not express an acceptance decision", got)
	}
	if _, ok := decision.OtherAuthorityReference(); ok {
		t.Fatal("own-product decision referenced another production authority")
	}
	if _, _, ok := decision.UnresolvedDetails(); ok {
		t.Fatal("own-product decision carried unresolved details")
	}
}

// Covers: SYN-CHAIN-03, S02-AT-05
func TestSyntheticChainUnresolvedHandoffCannotCarryOtherAuthority(t *testing.T) {
	continuation := mustValue(t, domain.NewOwnershipContinuationReference, "SYN-CHAIN-CONTINUE-1")
	confirmation := mustValue(t, domain.NewHandoffConfirmationReference, "SYN-CHAIN-CONFIRM-1")

	tests := []struct {
		name        string
		observation domain.HandoffObservation
		edit        func(*domain.SafeHandoffAssessmentSpec)
	}{
		{
			name:        "partial confirmation",
			observation: domain.HandoffObservationPartialConfirmation,
			edit: func(spec *domain.SafeHandoffAssessmentSpec) {
				spec.ConfirmedScopeDigest = spec.Scope.Digest()
				spec.ConfirmationRef = confirmation
				spec.ContinuationRef = continuation
			},
		},
		{
			name:        "confirmation cannot be queried",
			observation: domain.HandoffObservationQueryUnavailable,
			edit: func(spec *domain.SafeHandoffAssessmentSpec) {
				spec.ConfirmedScopeDigest = spec.Scope.Digest()
				spec.ConfirmationRef = confirmation
				spec.ContinuationRef = continuation
			},
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

			handoffRef, ok := assessment.ConfirmationReference()
			if ok {
				t.Fatalf("unresolved handoff handed out confirmation evidence %q", handoffRef)
			}
			if _, ok := assessment.ContinuationReference(); !ok {
				t.Fatal("unresolved handoff dropped its continuation reference")
			}

			decisionSpec := ownershipSpec(
				t,
				domain.ProductionAuthorityOther,
				domain.AdmissionControlOpen,
				"scope-1",
				"rev-1",
			)
			decisionSpec.HandoffRef = handoffRef
			_, err = domain.NewProductionOwnershipDecision(decisionSpec)
			if !errors.Is(err, domain.ErrInvalidProductionOwnershipDecision) {
				t.Fatalf("OTHER authority accepted an unresolved handoff: err = %v", err)
			}
		})
	}
}

// Covers: SYN-CHAIN-05, S02-AT-09
func TestSyntheticChainStaleOwnershipMustBeReevaluatedNotReused(t *testing.T) {
	preserved := sourceFingerprint(t, "SYN-TENANT-1", "SYN-CUSTOMER-1", "SYN-SOURCE-A", "SYN-KEY-1", "SYN-DIGEST-1")
	scope := syntheticChainScope(t, preserved)
	decision := syntheticChainDecision(t, scope, domain.ProductionAuthorityIDPParcel, "SYN-CHAIN-REV-1")

	allowed, err := domain.EvaluateFutureSubmissionGate(
		decision,
		scope.Digest(),
		mustValue(t, domain.NewProductionOwnershipRevision, "SYN-CHAIN-REV-1"),
		syntheticChainGateAt,
	)
	if err != nil {
		t.Fatalf("evaluate future submission gate: %v", err)
	}
	if !allowed.IsAllowed() {
		t.Fatalf("baseline gate blocked: reasons = %v", allowed.BlockReasons())
	}

	reevaluated, err := domain.EvaluateFutureSubmissionGate(
		decision,
		scope.Digest(),
		mustValue(t, domain.NewProductionOwnershipRevision, "SYN-CHAIN-REV-2"),
		syntheticChainGateAt,
	)
	if err != nil {
		t.Fatalf("re-evaluate future submission gate: %v", err)
	}
	if !syntheticChainHasBlockReason(reevaluated, domain.FutureSubmissionDecisionStale) {
		t.Fatalf("revision moved on but the gate stayed usable: reasons = %v", reevaluated.BlockReasons())
	}
	// The earlier value object is immutable and still reads ALLOWED. That is
	// exactly why a chain step may not carry it forward: staleness is only
	// visible by re-evaluating against the current revision.
	if !allowed.IsAllowed() {
		t.Fatal("the earlier gate mutated instead of staying a fixed record")
	}

	unresolved := syntheticChainDecision(t, scope, domain.ProductionAuthorityUnresolved, "SYN-CHAIN-REV-1")
	pending, err := domain.EvaluateFutureSubmissionGate(
		unresolved,
		scope.Digest(),
		mustValue(t, domain.NewProductionOwnershipRevision, "SYN-CHAIN-REV-1"),
		syntheticChainGateAt,
	)
	if err != nil {
		t.Fatalf("evaluate unresolved gate: %v", err)
	}
	if !syntheticChainHasBlockReason(pending, domain.FutureSubmissionAuthorityUnresolved) {
		t.Fatalf("unavailable re-judgement did not stay unresolved: reasons = %v", pending.BlockReasons())
	}
	reason, continuation, ok := unresolved.UnresolvedDetails()
	if !ok || reason != domain.OwnershipUnresolvedRuleUnavailable || continuation.String() == "" {
		t.Fatalf("unresolved decision lost its continuation: reason = %s, ok = %t", reason, ok)
	}
}

// Covers: SYN-CHAIN-06, S02-AT-10
func TestSyntheticChainScopeIsolationReachesTheFutureGate(t *testing.T) {
	base := sourceFingerprint(t, "SYN-TENANT-1", "SYN-CUSTOMER-1", "SYN-SOURCE-A", "SYN-KEY-1", "SYN-DIGEST-1")
	baseScope := syntheticChainScope(t, base)
	revision := mustValue(t, domain.NewProductionOwnershipRevision, "SYN-CHAIN-REV-1")
	decision := syntheticChainDecision(t, baseScope, domain.ProductionAuthorityIDPParcel, "SYN-CHAIN-REV-1")

	neighbours := map[string]domain.SourceSubmissionFingerprint{
		"other tenant":   sourceFingerprint(t, "SYN-TENANT-2", "SYN-CUSTOMER-1", "SYN-SOURCE-A", "SYN-KEY-1", "SYN-DIGEST-1"),
		"other customer": sourceFingerprint(t, "SYN-TENANT-1", "SYN-CUSTOMER-2", "SYN-SOURCE-A", "SYN-KEY-1", "SYN-DIGEST-1"),
		"other source":   sourceFingerprint(t, "SYN-TENANT-1", "SYN-CUSTOMER-1", "SYN-SOURCE-B", "SYN-KEY-1", "SYN-DIGEST-1"),
	}

	for name, neighbour := range neighbours {
		t.Run(name, func(t *testing.T) {
			classification, err := domain.ClassifySourceSubmission(base, neighbour)
			if err != nil {
				t.Fatalf("classify neighbour: %v", err)
			}
			if classification != domain.SourceDistinct {
				t.Fatalf("classification = %d; the same request key was reused across scopes", classification)
			}

			neighbourScope := syntheticChainScope(t, neighbour)
			if neighbourScope.Digest() == baseScope.Digest() {
				t.Fatal("a neighbouring scope derived the same admission digest")
			}

			gate, err := domain.EvaluateFutureSubmissionGate(
				decision,
				neighbourScope.Digest(),
				revision,
				syntheticChainGateAt,
			)
			if err != nil {
				t.Fatalf("evaluate cross-scope gate: %v", err)
			}
			if !syntheticChainHasBlockReason(gate, domain.FutureSubmissionScopeMismatch) {
				t.Fatalf("one scope's decision answered for another: reasons = %v", gate.BlockReasons())
			}
		})
	}
}

// Covers: SYN-CHAIN-07, S02-AT-01
func TestSyntheticChainExposesNoProductionEscalationSurface(t *testing.T) {
	preserved := sourceFingerprint(t, "SYN-TENANT-1", "SYN-CUSTOMER-1", "SYN-SOURCE-A", "SYN-KEY-1", "SYN-DIGEST-1")
	scope := syntheticChainScope(t, preserved)
	decision := syntheticChainDecision(t, scope, domain.ProductionAuthorityIDPParcel, "SYN-CHAIN-REV-1")
	gate, err := domain.EvaluateFutureSubmissionGate(
		decision,
		scope.Digest(),
		mustValue(t, domain.NewProductionOwnershipRevision, "SYN-CHAIN-REV-1"),
		syntheticChainGateAt,
	)
	if err != nil {
		t.Fatalf("evaluate future submission gate: %v", err)
	}
	handoff, err := domain.AssessSafeHandoff(completeHandoffSpec(t, "scope-1"))
	if err != nil {
		t.Fatalf("assess safe handoff: %v", err)
	}

	forbidden := []string{
		"Accept", "Reject", "Approve", "Submit", "Persist", "Save", "Store",
		"Publish", "Emit", "Send", "Notify", "Charge", "Freeze", "Settle",
		"Commit", "Enqueue", "Dispatch",
	}
	for _, subject := range []any{preserved, handoff, decision, gate} {
		subjectType := reflect.TypeOf(subject)
		for index := 0; index < subjectType.NumMethod(); index++ {
			method := subjectType.Method(index).Name
			for _, verb := range forbidden {
				if strings.Contains(method, verb) {
					t.Fatalf("%s exposes %s, which reaches past the pre-submission boundary", subjectType.Name(), method)
				}
			}
		}
	}

	wantDispositions := map[string]bool{"ALLOWED": true, "BLOCKED": true}
	gotDispositions := map[string]bool{}
	for value := 1; value <= 12; value++ {
		if name := domain.FutureSubmissionDisposition(value).String(); name != "" {
			gotDispositions[name] = true
		}
	}
	if !reflect.DeepEqual(gotDispositions, wantDispositions) {
		t.Fatalf("future submission dispositions = %v, want %v", gotDispositions, wantDispositions)
	}
}

func syntheticChainHasBlockReason(
	gate domain.FutureSubmissionGate,
	want domain.FutureSubmissionBlockReason,
) bool {
	for _, reason := range gate.BlockReasons() {
		if reason == want {
			return true
		}
	}
	return false
}
