package domain_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

type syntheticEvidenceReference string

func (reference syntheticEvidenceReference) valid() bool {
	return strings.TrimSpace(string(reference)) != ""
}

type syntheticReadiness uint8

const (
	syntheticReadinessInvalid syntheticReadiness = iota
	syntheticReady
	syntheticBlocked
)

type syntheticRecoveryBlockReason string

const (
	recoveryNotPaused              syntheticRecoveryBlockReason = "NOT_PAUSED"
	recoveryPauseReferenceMismatch syntheticRecoveryBlockReason = "PAUSE_REFERENCE_MISMATCH"
	recoveryScopeMismatch          syntheticRecoveryBlockReason = "SCOPE_MISMATCH"
	recoveryRevisionMismatch       syntheticRecoveryBlockReason = "CURRENT_REVISION_MISMATCH"
	recoveryCauseEvidenceMissing   syntheticRecoveryBlockReason = "CAUSE_RESOLUTION_MISSING"
	recoveryConsistencyMissing     syntheticRecoveryBlockReason = "CONSISTENCY_CHECK_MISSING"
	recoveryInventoryMissing       syntheticRecoveryBlockReason = "IN_FLIGHT_INVENTORY_MISSING"
	recoveryAuthorizationMissing   syntheticRecoveryBlockReason = "AUTHORIZATION_MISSING"
	recoveryDecisionMissing        syntheticRecoveryBlockReason = "RECOVERY_DECISION_MISSING"
	recoveryNewRevisionMissing     syntheticRecoveryBlockReason = "NEW_REVISION_REQUIRED"
	recoveryDecisionTimeInvalid    syntheticRecoveryBlockReason = "DECISION_TIME_INVALID"
)

type syntheticRecoveryEvidence struct {
	pauseReference    domain.OwnershipSuspensionReference
	scopeDigest       domain.AdmissionScopeDigest
	currentRevision   domain.ProductionOwnershipRevision
	causeResolution   syntheticEvidenceReference
	consistencyCheck  syntheticEvidenceReference
	inFlightInventory syntheticEvidenceReference
	authorization     syntheticEvidenceReference
	recoveryDecision  syntheticEvidenceReference
	newRevision       domain.ProductionOwnershipRevision
	decidedAt         time.Time
}

type syntheticRecoveryAssessment struct {
	readiness syntheticReadiness
	reasons   []syntheticRecoveryBlockReason
}

func assessSyntheticRecovery(
	paused domain.ProductionOwnershipDecision,
	evidence syntheticRecoveryEvidence,
) syntheticRecoveryAssessment {
	reasons := make([]syntheticRecoveryBlockReason, 0, 10)
	if paused.AdmissionControl() != domain.AdmissionControlPaused {
		reasons = append(reasons, recoveryNotPaused)
	}
	pauseReference, hasPause := paused.SuspensionReference()
	if !hasPause || pauseReference != evidence.pauseReference {
		reasons = append(reasons, recoveryPauseReferenceMismatch)
	}
	if paused.Scope().Digest() != evidence.scopeDigest {
		reasons = append(reasons, recoveryScopeMismatch)
	}
	if paused.Revision() != evidence.currentRevision {
		reasons = append(reasons, recoveryRevisionMismatch)
	}
	if !evidence.causeResolution.valid() {
		reasons = append(reasons, recoveryCauseEvidenceMissing)
	}
	if !evidence.consistencyCheck.valid() {
		reasons = append(reasons, recoveryConsistencyMissing)
	}
	if !evidence.inFlightInventory.valid() {
		reasons = append(reasons, recoveryInventoryMissing)
	}
	if !evidence.authorization.valid() {
		reasons = append(reasons, recoveryAuthorizationMissing)
	}
	if !evidence.recoveryDecision.valid() {
		reasons = append(reasons, recoveryDecisionMissing)
	}
	if evidence.newRevision.String() == "" || evidence.newRevision == paused.Revision() {
		reasons = append(reasons, recoveryNewRevisionMissing)
	}
	if evidence.decidedAt.IsZero() || evidence.decidedAt.Before(paused.DecisionAt()) {
		reasons = append(reasons, recoveryDecisionTimeInvalid)
	}
	readiness := syntheticReady
	if len(reasons) > 0 {
		readiness = syntheticBlocked
	}
	return syntheticRecoveryAssessment{readiness: readiness, reasons: reasons}
}

// Covers: S02-AT-07（没有显式恢复证据则准入仍被阻断）
func TestSyntheticAdmissionRecoveryRequiresAllExplicitEvidence(t *testing.T) {
	paused := ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlPaused, "scope-1", "rev-1")
	complete := completeSyntheticRecoveryEvidence(t, paused)

	tests := []struct {
		name       string
		edit       func(*syntheticRecoveryEvidence)
		wantReason syntheticRecoveryBlockReason
	}{
		{"cause resolution", func(value *syntheticRecoveryEvidence) { value.causeResolution = "" }, recoveryCauseEvidenceMissing},
		{"consistency check", func(value *syntheticRecoveryEvidence) { value.consistencyCheck = "" }, recoveryConsistencyMissing},
		{"in-flight inventory", func(value *syntheticRecoveryEvidence) { value.inFlightInventory = "" }, recoveryInventoryMissing},
		{"authorization", func(value *syntheticRecoveryEvidence) { value.authorization = "" }, recoveryAuthorizationMissing},
		{"recovery decision", func(value *syntheticRecoveryEvidence) { value.recoveryDecision = "" }, recoveryDecisionMissing},
		{"new revision", func(value *syntheticRecoveryEvidence) { value.newRevision = paused.Revision() }, recoveryNewRevisionMissing},
	}

	for _, test := range tests {
		t.Run("missing "+test.name, func(t *testing.T) {
			evidence := complete
			test.edit(&evidence)
			assessment := assessSyntheticRecovery(paused, evidence)
			if assessment.readiness != syntheticBlocked || !containsRecoveryReason(assessment.reasons, test.wantReason) {
				t.Fatalf("assessment = %#v, want blocker %s", assessment, test.wantReason)
			}
		})
	}
}

// Covers: S02-AT-07（恢复证据必须援引同一次暂停、同一范围与同一修订）
func TestSyntheticAdmissionRecoveryRejectsWrongPauseScopeOrRevision(t *testing.T) {
	paused := ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlPaused, "scope-1", "rev-1")
	complete := completeSyntheticRecoveryEvidence(t, paused)

	tests := []struct {
		name       string
		current    domain.ProductionOwnershipDecision
		edit       func(*syntheticRecoveryEvidence)
		wantReason syntheticRecoveryBlockReason
	}{
		{
			name:       "current decision is not paused",
			current:    ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, "scope-1", "rev-1"),
			edit:       func(*syntheticRecoveryEvidence) {},
			wantReason: recoveryNotPaused,
		},
		{
			name:    "pause reference mismatch",
			current: paused,
			edit: func(value *syntheticRecoveryEvidence) {
				value.pauseReference = mustValue(t, domain.NewOwnershipSuspensionReference, "pause-other")
			},
			wantReason: recoveryPauseReferenceMismatch,
		},
		{
			name:    "scope mismatch",
			current: paused,
			edit: func(value *syntheticRecoveryEvidence) {
				value.scopeDigest = mustValue(t, domain.NewAdmissionScopeDigest, "scope-2")
			},
			wantReason: recoveryScopeMismatch,
		},
		{
			name:    "current revision mismatch",
			current: paused,
			edit: func(value *syntheticRecoveryEvidence) {
				value.currentRevision = mustValue(t, domain.NewProductionOwnershipRevision, "rev-old")
			},
			wantReason: recoveryRevisionMismatch,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := complete
			test.edit(&evidence)
			assessment := assessSyntheticRecovery(test.current, evidence)
			if assessment.readiness != syntheticBlocked || !containsRecoveryReason(assessment.reasons, test.wantReason) {
				t.Fatalf("assessment = %#v, want blocker %s", assessment, test.wantReason)
			}
		})
	}
}

// Covers: S02-AT-07（准备度不开放准入；权威身份保持不变）
func TestSyntheticRecoveryReadinessDoesNotAutomaticallyOpenAdmission(t *testing.T) {
	paused := ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlPaused, "scope-1", "rev-1")
	evidence := completeSyntheticRecoveryEvidence(t, paused)
	assessment := assessSyntheticRecovery(paused, evidence)
	if assessment.readiness != syntheticReady || len(assessment.reasons) != 0 {
		t.Fatalf("recovery assessment = %#v, want READY", assessment)
	}

	gate, err := domain.EvaluateFutureSubmissionGate(
		paused,
		paused.Scope().Digest(),
		paused.Revision(),
		evidence.decidedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("evaluate paused gate: %v", err)
	}
	if gate.Disposition() != domain.FutureSubmissionBlocked ||
		!reflect.DeepEqual(gate.BlockReasons(), []domain.FutureSubmissionBlockReason{domain.FutureSubmissionAdmissionPaused}) {
		t.Fatalf("ready recovery changed the old pause: %s %v", gate.Disposition(), gate.BlockReasons())
	}

	resumedSpec := ownershipSpec(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, "scope-1", evidence.newRevision.String())
	resumedSpec.DecisionID = mustValue(t, domain.NewProductionOwnershipDecisionID, "decision-explicit-recovery")
	resumedSpec.DecisionAt = evidence.decidedAt
	resumed := mustDecision(t, resumedSpec)
	if resumed.AdmissionControl() != domain.AdmissionControlOpen || resumed.Revision() == paused.Revision() {
		t.Fatal("explicit recovery did not form a separate open revision")
	}
	if paused.AdmissionControl() != domain.AdmissionControlPaused {
		t.Fatal("explicit recovery mutated the prior paused decision")
	}
}

type syntheticRollbackOutcome uint8

const (
	syntheticRollbackOutcomeInvalid syntheticRollbackOutcome = iota
	syntheticReleaseRolledBackNoAuthorityChange
	syntheticRollbackEvidenceMissing
)

type syntheticRollbackAssessment struct {
	outcome    syntheticRollbackOutcome
	decisionID domain.ProductionOwnershipDecisionID
	authority  domain.ProductionAuthorityKind
	revision   domain.ProductionOwnershipRevision
}

func assessSyntheticReleaseRollback(
	current domain.ProductionOwnershipDecision,
	releaseRollbackReference syntheticEvidenceReference,
) syntheticRollbackAssessment {
	outcome := syntheticReleaseRolledBackNoAuthorityChange
	if !releaseRollbackReference.valid() {
		outcome = syntheticRollbackEvidenceMissing
	}
	return syntheticRollbackAssessment{
		outcome:    outcome,
		decisionID: current.DecisionID(),
		authority:  current.Authority(),
		revision:   current.Revision(),
	}
}

// Covers: S02-AT-08（发布回退不改变业务权威）
func TestSyntheticReleaseRollbackDoesNotChangeBusinessAuthority(t *testing.T) {
	current := ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, "scope-1", "rev-1")
	assessment := assessSyntheticReleaseRollback(current, "release-rollback-1")
	if assessment.outcome != syntheticReleaseRolledBackNoAuthorityChange ||
		assessment.decisionID != current.DecisionID() ||
		assessment.authority != current.Authority() ||
		assessment.revision != current.Revision() {
		t.Fatalf("release rollback changed ownership: %#v", assessment)
	}
}

type syntheticTakeoverBlockReason string

const (
	takeoverNewAdmissionsNotPaused    syntheticTakeoverBlockReason = "NEW_ADMISSIONS_NOT_PAUSED"
	takeoverPauseReferenceMismatch    syntheticTakeoverBlockReason = "PAUSE_REFERENCE_MISMATCH"
	takeoverCurrentDecisionMismatch   syntheticTakeoverBlockReason = "CURRENT_DECISION_MISMATCH"
	takeoverCurrentRevisionMismatch   syntheticTakeoverBlockReason = "CURRENT_REVISION_MISMATCH"
	takeoverScopeMissing              syntheticTakeoverBlockReason = "TRANSFER_SCOPE_MISSING"
	takeoverOldWritesNotStopped       syntheticTakeoverBlockReason = "OLD_WRITES_NOT_STOPPED"
	takeoverInFlightInventoryMissing  syntheticTakeoverBlockReason = "IN_FLIGHT_INVENTORY_MISSING"
	takeoverResponsibilitiesMissing   syntheticTakeoverBlockReason = "RESPONSIBILITY_INVENTORY_MISSING"
	takeoverTargetAuthorityMissing    syntheticTakeoverBlockReason = "TARGET_AUTHORITY_MISSING"
	takeoverTargetConfirmationMissing syntheticTakeoverBlockReason = "TARGET_CONFIRMATION_MISSING"
	takeoverTargetQueryMissing        syntheticTakeoverBlockReason = "TARGET_QUERY_MISSING"
	takeoverAuthorizationMissing      syntheticTakeoverBlockReason = "AUTHORIZATION_MISSING"
	takeoverEffectiveBoundaryMissing  syntheticTakeoverBlockReason = "EFFECTIVE_BOUNDARY_MISSING"
)

type syntheticTakeoverEvidence struct {
	pauseReference          domain.OwnershipSuspensionReference
	currentDecisionID       domain.ProductionOwnershipDecisionID
	currentRevision         domain.ProductionOwnershipRevision
	transferScopeReference  syntheticEvidenceReference
	transferScopeDigest     syntheticEvidenceReference
	oldWritesStopped        syntheticEvidenceReference
	inFlightInventory       syntheticEvidenceReference
	responsibilityInventory syntheticEvidenceReference
	targetAuthority         domain.ProductionAuthorityReference
	targetConfirmation      domain.HandoffConfirmationReference
	targetQuery             domain.HandoffQueryReference
	authorization           syntheticEvidenceReference
	effectiveAt             time.Time
}

type syntheticTakeoverAssessment struct {
	readiness syntheticReadiness
	reasons   []syntheticTakeoverBlockReason
}

func assessSyntheticTakeover(
	current domain.ProductionOwnershipDecision,
	evidence syntheticTakeoverEvidence,
) syntheticTakeoverAssessment {
	reasons := make([]syntheticTakeoverBlockReason, 0, 13)
	if current.AdmissionControl() != domain.AdmissionControlPaused {
		reasons = append(reasons, takeoverNewAdmissionsNotPaused)
	}
	pauseReference, hasPause := current.SuspensionReference()
	if !hasPause || pauseReference != evidence.pauseReference {
		reasons = append(reasons, takeoverPauseReferenceMismatch)
	}
	if current.DecisionID() != evidence.currentDecisionID {
		reasons = append(reasons, takeoverCurrentDecisionMismatch)
	}
	if current.Revision() != evidence.currentRevision {
		reasons = append(reasons, takeoverCurrentRevisionMismatch)
	}
	if !evidence.transferScopeReference.valid() || !evidence.transferScopeDigest.valid() {
		reasons = append(reasons, takeoverScopeMissing)
	}
	if !evidence.oldWritesStopped.valid() {
		reasons = append(reasons, takeoverOldWritesNotStopped)
	}
	if !evidence.inFlightInventory.valid() {
		reasons = append(reasons, takeoverInFlightInventoryMissing)
	}
	if !evidence.responsibilityInventory.valid() {
		reasons = append(reasons, takeoverResponsibilitiesMissing)
	}
	if evidence.targetAuthority.String() == "" {
		reasons = append(reasons, takeoverTargetAuthorityMissing)
	}
	if evidence.targetConfirmation.String() == "" {
		reasons = append(reasons, takeoverTargetConfirmationMissing)
	}
	if evidence.targetQuery.String() == "" {
		reasons = append(reasons, takeoverTargetQueryMissing)
	}
	if !evidence.authorization.valid() {
		reasons = append(reasons, takeoverAuthorizationMissing)
	}
	if evidence.effectiveAt.IsZero() || evidence.effectiveAt.Before(current.DecisionAt()) {
		reasons = append(reasons, takeoverEffectiveBoundaryMissing)
	}
	readiness := syntheticReady
	if len(reasons) > 0 {
		readiness = syntheticBlocked
	}
	return syntheticTakeoverAssessment{readiness: readiness, reasons: reasons}
}

// Covers: S02-AT-08（接管要求原权威停止新写入，并有完整的在途盘点）
func TestSyntheticObjectTakeoverRequiresStoppedWritesAndCompleteInventory(t *testing.T) {
	current := ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlPaused, "scope-1", "rev-1")
	complete := completeSyntheticTakeoverEvidence(t, current)

	tests := []struct {
		name       string
		edit       func(*syntheticTakeoverEvidence)
		wantReason syntheticTakeoverBlockReason
	}{
		{"old writes stopped", func(value *syntheticTakeoverEvidence) { value.oldWritesStopped = "" }, takeoverOldWritesNotStopped},
		{"in-flight inventory", func(value *syntheticTakeoverEvidence) { value.inFlightInventory = "" }, takeoverInFlightInventoryMissing},
		{"responsibility inventory", func(value *syntheticTakeoverEvidence) { value.responsibilityInventory = "" }, takeoverResponsibilitiesMissing},
		{"target confirmation", func(value *syntheticTakeoverEvidence) {
			value.targetConfirmation = domain.HandoffConfirmationReference{}
		}, takeoverTargetConfirmationMissing},
		{"target query", func(value *syntheticTakeoverEvidence) { value.targetQuery = domain.HandoffQueryReference{} }, takeoverTargetQueryMissing},
		{"authorization", func(value *syntheticTakeoverEvidence) { value.authorization = "" }, takeoverAuthorizationMissing},
	}

	for _, test := range tests {
		t.Run("missing "+test.name, func(t *testing.T) {
			evidence := complete
			test.edit(&evidence)
			assessment := assessSyntheticTakeover(current, evidence)
			if assessment.readiness != syntheticBlocked || !containsTakeoverReason(assessment.reasons, test.wantReason) {
				t.Fatalf("assessment = %#v, want blocker %s", assessment, test.wantReason)
			}
		})
	}
}

func TestSyntheticObjectTakeoverRejectsUnfixedCurrentBoundary(t *testing.T) {
	paused := ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlPaused, "scope-1", "rev-1")
	complete := completeSyntheticTakeoverEvidence(t, paused)

	tests := []struct {
		name       string
		current    domain.ProductionOwnershipDecision
		edit       func(*syntheticTakeoverEvidence)
		wantReason syntheticTakeoverBlockReason
	}{
		{
			name:       "new admissions are not paused",
			current:    ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, "scope-1", "rev-1"),
			edit:       func(*syntheticTakeoverEvidence) {},
			wantReason: takeoverNewAdmissionsNotPaused,
		},
		{
			name:    "pause reference mismatch",
			current: paused,
			edit: func(value *syntheticTakeoverEvidence) {
				value.pauseReference = mustValue(t, domain.NewOwnershipSuspensionReference, "pause-other")
			},
			wantReason: takeoverPauseReferenceMismatch,
		},
		{
			name:    "current decision mismatch",
			current: paused,
			edit: func(value *syntheticTakeoverEvidence) {
				value.currentDecisionID = mustValue(t, domain.NewProductionOwnershipDecisionID, "decision-other")
			},
			wantReason: takeoverCurrentDecisionMismatch,
		},
		{
			name:    "current revision mismatch",
			current: paused,
			edit: func(value *syntheticTakeoverEvidence) {
				value.currentRevision = mustValue(t, domain.NewProductionOwnershipRevision, "rev-old")
			},
			wantReason: takeoverCurrentRevisionMismatch,
		},
		{
			name:    "transfer scope missing",
			current: paused,
			edit: func(value *syntheticTakeoverEvidence) {
				value.transferScopeDigest = ""
			},
			wantReason: takeoverScopeMissing,
		},
		{
			name:    "effective boundary missing",
			current: paused,
			edit: func(value *syntheticTakeoverEvidence) {
				value.effectiveAt = time.Time{}
			},
			wantReason: takeoverEffectiveBoundaryMissing,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := complete
			test.edit(&evidence)
			assessment := assessSyntheticTakeover(test.current, evidence)
			if assessment.readiness != syntheticBlocked || !containsTakeoverReason(assessment.reasons, test.wantReason) {
				t.Fatalf("assessment = %#v, want blocker %s", assessment, test.wantReason)
			}
		})
	}
}

// Covers: S02-AT-08（准备度不转移权威，因而不打开双写窗口）
func TestSyntheticTakeoverReadinessDoesNotTransferAuthority(t *testing.T) {
	current := ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlPaused, "scope-1", "rev-1")
	evidence := completeSyntheticTakeoverEvidence(t, current)
	assessment := assessSyntheticTakeover(current, evidence)
	if assessment.readiness != syntheticReady || len(assessment.reasons) != 0 {
		t.Fatalf("takeover assessment = %#v, want READY", assessment)
	}
	if current.Authority() != domain.ProductionAuthorityIDPParcel ||
		current.AdmissionControl() != domain.AdmissionControlPaused ||
		current.Revision().String() != "rev-1" {
		t.Fatal("takeover readiness mutated the current ownership decision")
	}
}

func completeSyntheticRecoveryEvidence(
	t *testing.T,
	paused domain.ProductionOwnershipDecision,
) syntheticRecoveryEvidence {
	t.Helper()
	pauseReference, ok := paused.SuspensionReference()
	if !ok {
		t.Fatal("test decision is not paused")
	}
	return syntheticRecoveryEvidence{
		pauseReference:    pauseReference,
		scopeDigest:       paused.Scope().Digest(),
		currentRevision:   paused.Revision(),
		causeResolution:   "cause-resolution-1",
		consistencyCheck:  "consistency-check-1",
		inFlightInventory: "in-flight-inventory-1",
		authorization:     "recovery-authorization-1",
		recoveryDecision:  "recovery-decision-1",
		newRevision:       mustValue(t, domain.NewProductionOwnershipRevision, "rev-2"),
		decidedAt:         paused.DecisionAt().Add(time.Hour),
	}
}

func completeSyntheticTakeoverEvidence(
	t *testing.T,
	current domain.ProductionOwnershipDecision,
) syntheticTakeoverEvidence {
	t.Helper()
	pauseReference, ok := current.SuspensionReference()
	if !ok {
		t.Fatal("test decision is not paused")
	}
	return syntheticTakeoverEvidence{
		pauseReference:          pauseReference,
		currentDecisionID:       current.DecisionID(),
		currentRevision:         current.Revision(),
		transferScopeReference:  "takeover-scope-ref-1",
		transferScopeDigest:     "takeover-scope-digest-1",
		oldWritesStopped:        "old-writes-stopped-1",
		inFlightInventory:       "in-flight-inventory-1",
		responsibilityInventory: "responsibility-inventory-1",
		targetAuthority:         mustValue(t, domain.NewProductionAuthorityReference, "authority-other-1"),
		targetConfirmation:      mustValue(t, domain.NewHandoffConfirmationReference, "takeover-confirmation-1"),
		targetQuery:             mustValue(t, domain.NewHandoffQueryReference, "takeover-query-1"),
		authorization:           "takeover-authorization-1",
		effectiveAt:             current.DecisionAt().Add(2 * time.Hour),
	}
}

func containsRecoveryReason(reasons []syntheticRecoveryBlockReason, want syntheticRecoveryBlockReason) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func containsTakeoverReason(reasons []syntheticTakeoverBlockReason, want syntheticTakeoverBlockReason) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
