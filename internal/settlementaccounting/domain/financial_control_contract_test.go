package domain_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"
)

// This file is deliberately test-only. It is the S01-W03 contract for the
// financial-control boundary. It does not introduce a production settlement
// model, repository, transaction, outbox, or external financial adapter.

const (
	syntheticEvidenceLevel    = "S"
	syntheticUniqueResolution = "UNIQUE_RESOLVED"
)

type syntheticControlMode string

const (
	syntheticPrepaidMode syntheticControlMode = "PREPAID"
	syntheticTermsMode   syntheticControlMode = "TERMS"
)

type syntheticControlStatus string

const (
	syntheticControlFrozen              syntheticControlStatus = "FROZEN"
	syntheticControlRetained            syntheticControlStatus = "RETAINED_AFTER_ACCEPTANCE"
	syntheticControlReleased            syntheticControlStatus = "RELEASED"
	syntheticControlCreditWithinPolicy  syntheticControlStatus = "CREDIT_WITHIN_POLICY"
	syntheticControlCreditRestricted    syntheticControlStatus = "CREDIT_RESTRICTED"
	syntheticControlPending             syntheticControlStatus = "CONTROL_PENDING"
	syntheticControlCompensationPending syntheticControlStatus = "COMPENSATION_PENDING"
	syntheticControlConflict            syntheticControlStatus = "CONTROL_CONFLICT"
	syntheticControlInputNotAccepted    syntheticControlStatus = "INPUT_NOT_ACCEPTED"
	syntheticControlUnavailable         syntheticControlStatus = "CONTROL_UNAVAILABLE"
)

type syntheticFreezeState string

const (
	syntheticFreezeConfirmed syntheticFreezeState = "CONFIRMED"
	syntheticFreezeUnknown   syntheticFreezeState = "UNKNOWN"
	syntheticFreezeReleased  syntheticFreezeState = "RELEASED"
)

type syntheticAcceptanceState string

const (
	syntheticAcceptanceEstablished    syntheticAcceptanceState = "ESTABLISHED"
	syntheticAcceptanceNotEstablished syntheticAcceptanceState = "NOT_ESTABLISHED"
	syntheticAcceptanceUnknown        syntheticAcceptanceState = "UNKNOWN"
)

type syntheticFreezeSubmitBehavior string

const (
	syntheticFreezeSubmitConfirms  syntheticFreezeSubmitBehavior = "CONFIRMS"
	syntheticFreezeSubmitUncertain syntheticFreezeSubmitBehavior = "UNCERTAIN"
	syntheticFreezeSubmitRejects   syntheticFreezeSubmitBehavior = "REJECTS"
)

type syntheticReleaseBehavior string

const (
	syntheticReleaseConfirms  syntheticReleaseBehavior = "CONFIRMS"
	syntheticReleaseUncertain syntheticReleaseBehavior = "UNCERTAIN"
)

type syntheticCreditPosture string

const (
	syntheticCreditAvailable   syntheticCreditPosture = "AVAILABLE"
	syntheticCreditRestricted  syntheticCreditPosture = "RESTRICTED"
	syntheticCreditUnavailable syntheticCreditPosture = "UNAVAILABLE"
)

type syntheticResolvedBasis struct {
	resolutionID     string
	resolutionStatus string
	mode             syntheticControlMode
	scope            string
	account          string
	currency         string
	policyVersion    string
	asOf             time.Time
	validFrom        time.Time
	validUntil       time.Time
	currentRevision  string
	fixtureVersion   string
	evidence         string
	evidenceIndex    string
}

func (basis syntheticResolvedBasis) valid(at time.Time) bool {
	if basis.resolutionID == "" || basis.resolutionStatus != syntheticUniqueResolution ||
		basis.scope == "" || basis.account == "" ||
		basis.currency == "" || basis.policyVersion == "" || basis.asOf.IsZero() ||
		basis.validFrom.IsZero() || basis.currentRevision == "" ||
		basis.fixtureVersion == "" || basis.evidence != syntheticEvidenceLevel ||
		basis.evidenceIndex == "" {
		return false
	}
	if basis.mode != syntheticPrepaidMode && basis.mode != syntheticTermsMode {
		return false
	}
	if basis.asOf.Before(basis.validFrom) ||
		(!basis.validUntil.IsZero() && !basis.asOf.Before(basis.validUntil)) {
		return false
	}
	if basis.validUntil.IsZero() {
		return !at.Before(basis.validFrom)
	}
	return !at.Before(basis.validFrom) && at.Before(basis.validUntil)
}

type syntheticControlRequest struct {
	requestID     string
	submissionID  string
	associationID string
	tenantID      string
	customerID    string
	scope         string
	account       string
	currency      string
	amountMinor   int64
	mode          syntheticControlMode
	basis         syntheticResolvedBasis
	requestedAt   time.Time
}

func (request syntheticControlRequest) minimumValid() bool {
	return request.requestID != "" && request.submissionID != "" &&
		request.associationID != "" && request.tenantID != "" &&
		request.customerID != "" && request.scope != "" && request.account != "" &&
		request.currency != "" && request.amountMinor > 0 && !request.requestedAt.IsZero()
}

func (request syntheticControlRequest) matchesBasis() bool {
	return request.mode == request.basis.mode && request.scope == request.basis.scope &&
		request.account == request.basis.account && request.currency == request.basis.currency
}

func (request syntheticControlRequest) identityKey() string {
	return strings.Join([]string{request.tenantID, request.customerID, request.requestID}, "\x00")
}

func (request syntheticControlRequest) fingerprint() string {
	return strings.Join([]string{
		request.tenantID, request.customerID, request.requestID, request.scope,
		request.account, request.currency, fmt.Sprint(request.amountMinor),
		string(request.mode), request.basis.resolutionID, request.basis.policyVersion,
		request.basis.currentRevision, request.submissionID, request.associationID,
	}, "\x00")
}

type syntheticEstimate struct {
	amountMinor int64
	currency    string
	purpose     string
}

type syntheticFreezeRecord struct {
	freezeID      string
	associationID string
	requestDigest string
	amountMinor   int64
	currency      string
	state         syntheticFreezeState
	revision      int
}

type syntheticCreditRecord struct {
	account       string
	exposureMinor int64
	limitMinor    int64
	posture       syntheticCreditPosture
	reason        string
}

type syntheticControlResult struct {
	status          syntheticControlStatus
	controlID       string
	requestDigest   string
	associationID   string
	mode            syntheticControlMode
	basis           syntheticResolvedBasis
	judgedAt        time.Time
	estimate        syntheticEstimate
	freeze          syntheticFreezeRecord
	credit          syntheticCreditRecord
	continuationRef string
	reason          string
	// Explicit negative-boundary markers: this slice does not create these
	// downstream financial facts.
	hasFee            bool
	hasReceivable     bool
	hasPayment        bool
	hasReconciliation bool
}

type syntheticControlStub struct {
	now            time.Time
	freezeSubmit   syntheticFreezeSubmitBehavior
	release        syntheticReleaseBehavior
	creditPosture  syntheticCreditPosture
	creditLimit    int64
	creditExposure int64
	authorityReady bool
	freezeQuery    map[string]syntheticFreezeState
	freezes        map[string]syntheticFreezeRecord
	requestDigests map[string]string
	results        map[string]syntheticControlResult
	freezeCalls    int
	releaseCalls   int
	queryCalls     int
	externalCalls  int
	nextID         int
}

func newSyntheticControlStub() *syntheticControlStub {
	return &syntheticControlStub{
		now:            time.Date(2026, 8, 7, 7, 0, 0, 0, time.UTC),
		freezeSubmit:   syntheticFreezeSubmitConfirms,
		release:        syntheticReleaseConfirms,
		creditPosture:  syntheticCreditAvailable,
		creditLimit:    10000,
		creditExposure: 1000,
		authorityReady: true,
		freezeQuery:    make(map[string]syntheticFreezeState),
		freezes:        make(map[string]syntheticFreezeRecord),
		requestDigests: make(map[string]string),
		results:        make(map[string]syntheticControlResult),
	}
}

func syntheticBasis(mode syntheticControlMode, scope, account, revision string) syntheticResolvedBasis {
	return syntheticResolvedBasis{
		resolutionID:     "SYN-RES-" + strings.ToLower(scope),
		resolutionStatus: syntheticUniqueResolution,
		mode:             mode,
		scope:            scope,
		account:          account,
		currency:         "SYN",
		policyVersion:    "SYN-POLICY-" + string(mode) + "-v1",
		asOf:             time.Date(2026, 8, 7, 6, 0, 0, 0, time.UTC),
		validFrom:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		validUntil:       time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		currentRevision:  revision,
		fixtureVersion:   "SYN-FINANCIAL-FIXTURE-v1",
		evidence:         syntheticEvidenceLevel,
		evidenceIndex:    "SYN-EVIDENCE-FIN-01",
	}
}

func syntheticRequest(mode syntheticControlMode, scope, account, submission, association string) syntheticControlRequest {
	basis := syntheticBasis(mode, scope, account, "rev-1")
	return syntheticControlRequest{
		requestID:     "SYN-REQUEST-" + submission,
		submissionID:  submission,
		associationID: association,
		tenantID:      "SYN-TENANT-01",
		customerID:    "SYN-CUSTOMER-01",
		scope:         scope,
		account:       account,
		currency:      basis.currency,
		amountMinor:   1250,
		mode:          mode,
		basis:         basis,
		requestedAt:   time.Date(2026, 8, 7, 7, 1, 0, 0, time.UTC),
	}
}

func digestSyntheticControl(request syntheticControlRequest) string {
	digest := sha256.Sum256([]byte(request.fingerprint()))
	return "SYN-CTRL-" + hex.EncodeToString(digest[:8])
}

func (stub *syntheticControlStub) next(prefix string) string {
	stub.nextID++
	return fmt.Sprintf("%s-%02d", prefix, stub.nextID)
}

func (stub *syntheticControlStub) resultFor(request syntheticControlRequest, digest string) syntheticControlResult {
	return syntheticControlResult{
		controlID:     stub.next("SYN-CONTROL"),
		requestDigest: digest,
		associationID: request.associationID,
		mode:          request.mode,
		basis:         request.basis,
		judgedAt:      stub.now,
		estimate: syntheticEstimate{
			amountMinor: request.amountMinor,
			currency:    request.currency,
			purpose:     "ACCEPTANCE_CONTROL_ESTIMATE",
		},
	}
}

func (stub *syntheticControlStub) submitFreeze(request syntheticControlRequest, digest string) syntheticFreezeRecord {
	stub.freezeCalls++
	record := syntheticFreezeRecord{
		freezeID:      stub.next("SYN-FREEZE"),
		associationID: request.associationID,
		requestDigest: digest,
		amountMinor:   request.amountMinor,
		currency:      request.currency,
		state:         syntheticFreezeUnknown,
		revision:      1,
	}
	switch stub.freezeSubmit {
	case syntheticFreezeSubmitConfirms:
		record.state = syntheticFreezeConfirmed
	case syntheticFreezeSubmitRejects:
		record.state = syntheticFreezeReleased
	}
	stub.freezes[record.freezeID] = record
	stub.freezeQuery[record.freezeID] = record.state
	return record
}

func (stub *syntheticControlStub) queryFreeze(freezeID string) (syntheticFreezeRecord, bool) {
	stub.queryCalls++
	record, ok := stub.freezes[freezeID]
	if !ok {
		return syntheticFreezeRecord{}, false
	}
	if state, exists := stub.freezeQuery[freezeID]; exists {
		record.state = state
		stub.freezes[freezeID] = record
	}
	return record, true
}

func (stub *syntheticControlStub) releaseFreeze(freezeID string) (syntheticFreezeRecord, bool) {
	record, ok := stub.freezes[freezeID]
	if !ok {
		return syntheticFreezeRecord{}, false
	}
	if record.state == syntheticFreezeReleased {
		return record, true
	}
	stub.releaseCalls++
	if stub.release == syntheticReleaseUncertain {
		return record, false
	}
	record.state = syntheticFreezeReleased
	record.revision++
	stub.freezes[freezeID] = record
	stub.freezeQuery[freezeID] = record.state
	return record, true
}

func (stub *syntheticControlStub) execute(request syntheticControlRequest) syntheticControlResult {
	digest := digestSyntheticControl(request)
	if !request.minimumValid() {
		result := stub.resultFor(request, digest)
		result.status = syntheticControlInputNotAccepted
		result.reason = "MINIMUM_CONTROL_SCOPE_NOT_ESTABLISHED"
		return result
	}
	if previousDigest, exists := stub.requestDigests[request.identityKey()]; exists {
		if previousDigest == digest {
			return stub.results[digest]
		}
		result := stub.resultFor(request, digest)
		result.status = syntheticControlConflict
		result.reason = "REQUEST_IDENTITY_CONFLICT"
		return result
	}
	if !request.matchesBasis() {
		result := stub.resultFor(request, digest)
		result.status = syntheticControlConflict
		result.reason = "RESOLVED_BASIS_SCOPE_CONFLICT"
		return result
	}
	if !stub.authorityReady || !request.basis.valid(stub.now) {
		result := stub.resultFor(request, digest)
		result.status = syntheticControlPending
		result.reason = "BASIS_UNAVAILABLE"
		result.continuationRef = stub.next("SYN-CONTINUE")
		return result
	}

	result := stub.resultFor(request, digest)
	stub.requestDigests[request.identityKey()] = digest
	switch request.mode {
	case syntheticPrepaidMode:
		freeze := stub.submitFreeze(request, digest)
		result.freeze = freeze
		switch freeze.state {
		case syntheticFreezeConfirmed:
			result.status = syntheticControlFrozen
		case syntheticFreezeReleased:
			result.status = syntheticControlUnavailable
			result.reason = "FUNDS_NOT_AVAILABLE"
		default:
			result.status = syntheticControlPending
			result.reason = "FREEZE_RESULT_UNKNOWN"
			result.continuationRef = stub.next("SYN-CONTINUE")
		}
	case syntheticTermsMode:
		if stub.creditPosture == syntheticCreditUnavailable {
			result.status = syntheticControlPending
			result.reason = "CREDIT_AUTHORITY_UNAVAILABLE"
			result.continuationRef = stub.next("SYN-CONTINUE")
			break
		}
		result.credit = syntheticCreditRecord{
			account:       request.account,
			exposureMinor: stub.creditExposure,
			limitMinor:    stub.creditLimit,
			posture:       stub.creditPosture,
		}
		if stub.creditPosture == syntheticCreditRestricted || stub.creditExposure+request.amountMinor > stub.creditLimit {
			result.status = syntheticControlCreditRestricted
			result.reason = "CREDIT_LIMIT_OR_RESTRICTION"
		} else {
			result.status = syntheticControlCreditWithinPolicy
		}
	}
	stub.results[digest] = result
	return result
}

func (stub *syntheticControlStub) compensateAfterAcceptance(result syntheticControlResult, state syntheticAcceptanceState) syntheticControlResult {
	if result.mode != syntheticPrepaidMode || result.freeze.freezeID == "" {
		return result
	}
	if state == syntheticAcceptanceEstablished {
		result.status = syntheticControlRetained
		return result
	}
	if state == syntheticAcceptanceUnknown {
		queried, ok := stub.queryFreeze(result.freeze.freezeID)
		if !ok || queried.state == syntheticFreezeUnknown {
			result.status = syntheticControlPending
			result.reason = "ACCEPTANCE_AND_FREEZE_RESULT_UNKNOWN"
			result.continuationRef = stub.next("SYN-CONTINUE")
			return result
		}
		result.freeze = queried
		if queried.state == syntheticFreezeConfirmed {
			result.status = syntheticControlRetained
			return result
		}
		return result
	}
	if state != syntheticAcceptanceNotEstablished {
		return result
	}
	released, ok := stub.releaseFreeze(result.freeze.freezeID)
	if !ok {
		result.status = syntheticControlCompensationPending
		result.reason = "FREEZE_RELEASE_UNKNOWN"
		result.continuationRef = stub.next("SYN-CONTINUE")
		return result
	}
	result.freeze = released
	result.status = syntheticControlReleased
	return result
}

func assertSyntheticControlTrace(t *testing.T, result syntheticControlResult) {
	t.Helper()
	if result.controlID == "" || result.requestDigest == "" || result.associationID == "" ||
		result.basis.resolutionID == "" || result.basis.policyVersion == "" ||
		result.basis.currentRevision == "" || result.basis.evidence != syntheticEvidenceLevel ||
		result.basis.evidenceIndex == "" || result.judgedAt.IsZero() {
		t.Fatalf("control trace incomplete: %#v", result)
	}
	if result.hasFee || result.hasReceivable || result.hasPayment || result.hasReconciliation {
		t.Fatalf("control result crossed financial ownership boundary: %#v", result)
	}
}

func TestSyntheticPrepaidControlFormsEstimateAndSingleFreeze(t *testing.T) {
	stub := newSyntheticControlStub()
	request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SYN-ACCOUNT-PREPAID-01", "SUB-01", "ACCEPT-01")
	first := stub.execute(request)
	replay := stub.execute(request)
	if first.status != syntheticControlFrozen || replay.status != syntheticControlFrozen ||
		first.controlID != replay.controlID || first.freeze.freezeID != replay.freeze.freezeID || stub.freezeCalls != 1 {
		t.Fatalf("prepaid idempotency/freeze = first %#v replay %#v calls %d", first, replay, stub.freezeCalls)
	}
	if first.estimate.amountMinor != request.amountMinor || first.estimate.currency != request.currency || first.estimate.purpose == "" {
		t.Fatalf("prepaid estimate missing: %#v", first.estimate)
	}
	assertSyntheticControlTrace(t, first)
}

func TestSyntheticTermsControlIsIndependentFromPrepaid(t *testing.T) {
	stub := newSyntheticControlStub()
	request := syntheticRequest(syntheticTermsMode, "SYN-SCOPE-TERMS-01", "SYN-ACCOUNT-TERMS-01", "SUB-02", "ACCEPT-02")
	result := stub.execute(request)
	if result.status != syntheticControlCreditWithinPolicy || result.credit.account != request.account ||
		result.credit.limitMinor != stub.creditLimit || result.freeze.freezeID != "" || stub.freezeCalls != 0 {
		t.Fatalf("terms control borrowed prepaid state: %#v calls=%d", result, stub.freezeCalls)
	}
	assertSyntheticControlTrace(t, result)

	stub.creditPosture = syntheticCreditRestricted
	restricted := stub.execute(syntheticRequest(syntheticTermsMode, "SYN-SCOPE-TERMS-02", "SYN-ACCOUNT-TERMS-02", "SUB-03", "ACCEPT-03"))
	if restricted.status != syntheticControlCreditRestricted || restricted.reason == "" {
		t.Fatalf("restricted terms result = %#v", restricted)
	}
	assertSyntheticControlTrace(t, restricted)
}

func TestSyntheticControlCannotOverrideResolvedModeOrCrossScope(t *testing.T) {
	stub := newSyntheticControlStub()
	request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SYN-ACCOUNT-PREPAID-01", "SUB-04", "ACCEPT-04")
	request.mode = syntheticTermsMode
	invalid := stub.execute(request)
	if invalid.status != syntheticControlConflict || invalid.reason != "RESOLVED_BASIS_SCOPE_CONFLICT" || stub.freezeCalls != 0 {
		t.Fatalf("caller mode override was accepted: %#v", invalid)
	}

	request = syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SYN-ACCOUNT-PREPAID-01", "SUB-05", "ACCEPT-05")
	request.scope = "SYN-SCOPE-OTHER"
	conflict := stub.execute(request)
	if conflict.status != syntheticControlConflict || conflict.reason != "RESOLVED_BASIS_SCOPE_CONFLICT" || stub.freezeCalls != 0 {
		t.Fatalf("cross-scope basis was accepted: %#v", conflict)
	}
}

func TestSyntheticUncertainAcceptanceQueriesBeforeCompensation(t *testing.T) {
	stub := newSyntheticControlStub()
	stub.freezeSubmit = syntheticFreezeSubmitUncertain
	request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SYN-ACCOUNT-PREPAID-01", "SUB-06", "ACCEPT-06")
	pending := stub.execute(request)
	if pending.status != syntheticControlPending || pending.freeze.state != syntheticFreezeUnknown {
		t.Fatalf("initial uncertain freeze = %#v", pending)
	}
	stub.freezeQuery[pending.freeze.freezeID] = syntheticFreezeConfirmed
	retained := stub.compensateAfterAcceptance(pending, syntheticAcceptanceUnknown)
	if retained.status != syntheticControlRetained || stub.queryCalls != 1 || stub.releaseCalls != 0 {
		t.Fatalf("unknown acceptance did not query/retain: %#v queries=%d releases=%d", retained, stub.queryCalls, stub.releaseCalls)
	}
	assertSyntheticControlTrace(t, retained)

	released := stub.compensateAfterAcceptance(retained, syntheticAcceptanceNotEstablished)
	if released.status != syntheticControlReleased || released.freeze.state != syntheticFreezeReleased || stub.releaseCalls != 1 {
		t.Fatalf("confirmed acceptance failure did not release: %#v releases=%d", released, stub.releaseCalls)
	}
	releasedReplay := stub.compensateAfterAcceptance(released, syntheticAcceptanceNotEstablished)
	if releasedReplay.status != syntheticControlReleased || stub.releaseCalls != 1 {
		t.Fatalf("release replay duplicated compensation: %#v releases=%d", releasedReplay, stub.releaseCalls)
	}
}

func TestSyntheticAcceptedSubmissionRetainsLegalFreeze(t *testing.T) {
	stub := newSyntheticControlStub()
	request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SYN-ACCOUNT-PREPAID-01", "SUB-07", "ACCEPT-07")
	frozen := stub.execute(request)
	retained := stub.compensateAfterAcceptance(frozen, syntheticAcceptanceEstablished)
	if retained.status != syntheticControlRetained || retained.freeze.state != syntheticFreezeConfirmed || stub.releaseCalls != 0 {
		t.Fatalf("accepted submission released a legal freeze: %#v", retained)
	}
	assertSyntheticControlTrace(t, retained)
}

func TestSyntheticCompensationFailureRemainsPendingAndQueryable(t *testing.T) {
	stub := newSyntheticControlStub()
	request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SYN-ACCOUNT-PREPAID-01", "SUB-08", "ACCEPT-08")
	frozen := stub.execute(request)
	stub.release = syntheticReleaseUncertain
	pending := stub.compensateAfterAcceptance(frozen, syntheticAcceptanceNotEstablished)
	if pending.status != syntheticControlCompensationPending || pending.continuationRef == "" || stub.releaseCalls != 1 {
		t.Fatalf("compensation failure was not pending: %#v", pending)
	}
	stub.release = syntheticReleaseConfirms
	released := stub.compensateAfterAcceptance(pending, syntheticAcceptanceNotEstablished)
	if released.status != syntheticControlReleased || released.freeze.state != syntheticFreezeReleased || stub.releaseCalls != 2 {
		t.Fatalf("compensation retry did not complete: %#v releases=%d", released, stub.releaseCalls)
	}
	assertSyntheticControlTrace(t, released)
}

func TestSyntheticFinancialControlRejectsStaleOrUnavailableBasis(t *testing.T) {
	stub := newSyntheticControlStub()
	request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SYN-ACCOUNT-PREPAID-01", "SUB-09", "ACCEPT-09")
	request.basis.currentRevision = ""
	stale := stub.execute(request)
	if stale.status != syntheticControlPending || stale.reason != "BASIS_UNAVAILABLE" || stub.freezeCalls != 0 {
		t.Fatalf("stale basis was used: %#v", stale)
	}

	request = syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SYN-ACCOUNT-PREPAID-01", "SUB-09B", "ACCEPT-09B")
	request.basis.resolutionStatus = "APPLICABILITY_CONFLICT"
	conflict := stub.execute(request)
	if conflict.status != syntheticControlPending || conflict.reason != "BASIS_UNAVAILABLE" || stub.freezeCalls != 0 {
		t.Fatalf("non-unique commercial result was used: %#v", conflict)
	}

	stub = newSyntheticControlStub()
	stub.authorityReady = false
	unavailable := stub.execute(syntheticRequest(syntheticTermsMode, "SYN-SCOPE-TERMS-01", "SYN-ACCOUNT-TERMS-01", "SUB-10", "ACCEPT-10"))
	if unavailable.status != syntheticControlPending || unavailable.continuationRef == "" || stub.externalCalls != 0 {
		t.Fatalf("unavailable authority = %#v", unavailable)
	}
	assertSyntheticControlTrace(t, unavailable)
}

func TestSyntheticControlPreservesHistoryAndNeverCreatesCashFacts(t *testing.T) {
	stub := newSyntheticControlStub()
	request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SYN-ACCOUNT-PREPAID-01", "SUB-11", "ACCEPT-11")
	frozen := stub.execute(request)
	released := stub.compensateAfterAcceptance(frozen, syntheticAcceptanceNotEstablished)
	if _, ok := stub.freezes[frozen.freeze.freezeID]; !ok {
		t.Fatalf("release deleted freeze history")
	}
	if stub.externalCalls != 0 {
		t.Fatalf("synthetic control called external financial system %d times", stub.externalCalls)
	}
	assertSyntheticControlTrace(t, released)
}

func TestSyntheticControlRejectsConflictingReplayWithoutSideEffect(t *testing.T) {
	stub := newSyntheticControlStub()
	request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SYN-ACCOUNT-PREPAID-01", "SUB-12", "ACCEPT-12")
	first := stub.execute(request)
	request.amountMinor++
	second := stub.execute(request)
	if first.status != syntheticControlFrozen || second.status != syntheticControlConflict ||
		second.reason != "REQUEST_IDENTITY_CONFLICT" || stub.freezeCalls != 1 {
		t.Fatalf("conflicting replay was not isolated: first=%#v second=%#v calls=%d", first, second, stub.freezeCalls)
	}
	assertSyntheticControlTrace(t, second)
}
