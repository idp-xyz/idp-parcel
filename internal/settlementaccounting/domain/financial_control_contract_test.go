package domain_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// 本文件刻意只存在于测试侧。它是财务控制边界的 S01-W03 合约，不引入生产的结算模型、仓储、
// 事务、发件箱或对外财务适配器。

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
	// 结算账户刻意缺席。party-commercial 的结算政策固定了方式、币种与范围；同样固定这些的
	// 那个账户是 settlement-accounting 自己的对象，由这几个维度解析得出，而不是跨边界带过来。
	resolutionID     string
	resolutionStatus string
	mode             syntheticControlMode
	scope            string
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
		basis.scope == "" ||
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
	currency      string
	amountMinor   int64
	mode          syntheticControlMode
	basis         syntheticResolvedBasis
	requestedAt   time.Time
}

func (request syntheticControlRequest) minimumValid() bool {
	return request.requestID != "" && request.submissionID != "" &&
		request.associationID != "" && request.tenantID != "" &&
		request.customerID != "" && request.scope != "" &&
		request.currency != "" && request.amountMinor > 0 && !request.requestedAt.IsZero()
}

func (request syntheticControlRequest) matchesBasis() bool {
	return request.mode == request.basis.mode && request.scope == request.basis.scope &&
		request.currency == request.basis.currency
}

func (request syntheticControlRequest) identityKey() string {
	return strings.Join([]string{request.tenantID, request.customerID, request.requestID}, "\x00")
}

func (request syntheticControlRequest) fingerprint() string {
	return strings.Join([]string{
		request.tenantID, request.customerID, request.requestID, request.scope,
		request.currency, fmt.Sprint(request.amountMinor),
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
	status        syntheticControlStatus
	controlID     string
	requestDigest string
	associationID string
	mode          syntheticControlMode
	// account 由结算侧从依据的各维度解析得出，绝不取自调用方。之所以记录它，是因为一笔已确认
	// 的计费必须固定自己的结算账户（settlement-accounting CONTEXT.md，费用形成与证据）。
	account         string
	basis           syntheticResolvedBasis
	judgedAt        time.Time
	estimate        syntheticEstimate
	freeze          syntheticFreezeRecord
	credit          syntheticCreditRecord
	continuationRef string
	reason          string
	// 显式的反向边界标记：本切片不产生这些下游财务事实。没有任何控制路径会置位它们，而这正是
	// 要点：TestSyntheticControlBoundaryGuardsAreFalsifiable 证明有一个被置位时守卫会反应，
	// 于是别处那些零值断言的含义是「没有东西置位过它」，而不是「没有东西能置位它」。
	hasFee            bool
	hasReceivable     bool
	hasPayment        bool
	hasReconciliation bool
}

func crossesFinancialOwnershipBoundary(result syntheticControlResult) bool {
	return result.hasFee || result.hasReceivable || result.hasPayment || result.hasReconciliation
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

func syntheticBasis(mode syntheticControlMode, scope, revision string) syntheticResolvedBasis {
	return syntheticResolvedBasis{
		resolutionID:     "SYN-RES-" + strings.ToLower(scope),
		resolutionStatus: syntheticUniqueResolution,
		mode:             mode,
		scope:            scope,
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

func syntheticRequest(mode syntheticControlMode, scope, submission, association string) syntheticControlRequest {
	basis := syntheticBasis(mode, scope, "rev-1")
	return syntheticControlRequest{
		requestID:     "SYN-REQUEST-" + submission,
		submissionID:  submission,
		associationID: association,
		tenantID:      "SYN-TENANT-01",
		customerID:    "SYN-CUSTOMER-01",
		scope:         scope,
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
		account:       stub.resolveSettlementAccount(request.basis),
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
			account:       stub.resolveSettlementAccount(request.basis),
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
		result.account == "" || result.basis.resolutionID == "" || result.basis.policyVersion == "" ||
		result.basis.currentRevision == "" || result.basis.evidence != syntheticEvidenceLevel ||
		result.basis.evidenceIndex == "" || result.judgedAt.IsZero() {
		t.Fatalf("control trace incomplete: %#v", result)
	}
	if crossesFinancialOwnershipBoundary(result) {
		t.Fatalf("control result crossed financial ownership boundary: %#v", result)
	}
}

// callExternalFinancialSystem 是任何对外财务调用都必须经过的唯一咽喉。这个离线桩里没有东西
// 调用它；它存在是为了让 externalCalls 成为一个真会动的仪表，这才让 externalCalls == 0 的
// 那些断言有意义。
func (stub *syntheticControlStub) callExternalFinancialSystem() {
	stub.externalCalls++
}

// resolveSettlementAccount 模拟 settlement-accounting 解析自己的账户。商业依据固定了法律
// 主体、对手方、方向与币种，却不发布账户；结算侧从这些维度推导出一个，于是调用方既不能指名
// 一个账户，也不能借用为另一个范围、方式或币种解析出来的账户。
func (stub *syntheticControlStub) resolveSettlementAccount(basis syntheticResolvedBasis) string {
	if !basis.valid(stub.now) {
		return ""
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{
		string(basis.mode), basis.currency, basis.scope, basis.policyVersion,
	}, "\x00")))
	return "SYN-ACCOUNT-" + hex.EncodeToString(digest[:6])
}

// Covers: SYN-CHAIN-04（消费侧半边：账户由依据的各维度解析得出，与依据不符的币种绝不冻结）
func TestSyntheticControlResolvesItsOwnAccountFromTheBasis(t *testing.T) {
	stub := newSyntheticControlStub()
	basis := syntheticBasis(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "rev-1")

	resolved := stub.resolveSettlementAccount(basis)
	if resolved == "" {
		t.Fatal("a valid basis resolved no settlement account")
	}
	if stub.resolveSettlementAccount(basis) != resolved {
		t.Fatal("the same basis resolved two different settlement accounts")
	}

	dimensions := map[string]func(*syntheticResolvedBasis){
		"currency": func(value *syntheticResolvedBasis) { value.currency = "SYN2" },
		"scope":    func(value *syntheticResolvedBasis) { value.scope = "SYN-SCOPE-PREPAID-02" },
		"mode":     func(value *syntheticResolvedBasis) { value.mode = syntheticTermsMode },
	}
	for name, change := range dimensions {
		t.Run("account follows "+name, func(t *testing.T) {
			changed := basis
			change(&changed)
			if stub.resolveSettlementAccount(changed) == resolved {
				t.Fatalf("changing %s reused the same settlement account", name)
			}
		})
	}

	t.Run("neither the basis nor the caller can name an account", func(t *testing.T) {
		for _, subject := range []any{syntheticResolvedBasis{}, syntheticControlRequest{}} {
			subjectType := reflect.TypeOf(subject)
			for index := 0; index < subjectType.NumField(); index++ {
				name := subjectType.Field(index).Name
				if strings.Contains(strings.ToLower(name), "account") {
					t.Fatalf("%s carries %s, so an account can cross the boundary unresolved",
						subjectType.Name(), name)
				}
			}
		}
	})

	t.Run("currency disagreeing with the basis never freezes", func(t *testing.T) {
		stub := newSyntheticControlStub()
		request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SUB-13", "ACCEPT-13")
		request.currency = "SYN2"
		result := stub.execute(request)
		if result.status != syntheticControlConflict || stub.freezeCalls != 0 {
			t.Fatalf("a currency mismatch was accepted: %#v freezes=%d", result, stub.freezeCalls)
		}
	})
}

// Covers: S01-AT-08
func TestSyntheticControlBoundaryGuardsAreFalsifiable(t *testing.T) {
	if crossesFinancialOwnershipBoundary(syntheticControlResult{}) {
		t.Fatal("a clean control result reported a boundary crossing")
	}
	markers := map[string]func(*syntheticControlResult){
		"fee":            func(result *syntheticControlResult) { result.hasFee = true },
		"receivable":     func(result *syntheticControlResult) { result.hasReceivable = true },
		"payment":        func(result *syntheticControlResult) { result.hasPayment = true },
		"reconciliation": func(result *syntheticControlResult) { result.hasReconciliation = true },
	}
	for name, mark := range markers {
		t.Run(name, func(t *testing.T) {
			crossed := syntheticControlResult{}
			mark(&crossed)
			if !crossesFinancialOwnershipBoundary(crossed) {
				t.Fatalf("%s marker did not trip the ownership guard", name)
			}
		})
	}

	stub := newSyntheticControlStub()
	if stub.externalCalls != 0 {
		t.Fatalf("fresh stub started at %d external calls", stub.externalCalls)
	}
	stub.callExternalFinancialSystem()
	if stub.externalCalls != 1 {
		t.Fatalf("external call counter did not move: %d", stub.externalCalls)
	}
}

// Covers: S01-AT-01
func TestSyntheticPrepaidControlFormsEstimateAndSingleFreeze(t *testing.T) {
	stub := newSyntheticControlStub()
	request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SUB-01", "ACCEPT-01")
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

// Covers: S01-AT-02
func TestSyntheticTermsControlIsIndependentFromPrepaid(t *testing.T) {
	stub := newSyntheticControlStub()
	request := syntheticRequest(syntheticTermsMode, "SYN-SCOPE-TERMS-01", "SUB-02", "ACCEPT-02")
	result := stub.execute(request)
	if result.status != syntheticControlCreditWithinPolicy ||
		result.credit.account != stub.resolveSettlementAccount(request.basis) ||
		result.credit.limitMinor != stub.creditLimit || result.freeze.freezeID != "" || stub.freezeCalls != 0 {
		t.Fatalf("terms control borrowed prepaid state: %#v calls=%d", result, stub.freezeCalls)
	}
	assertSyntheticControlTrace(t, result)

	stub.creditPosture = syntheticCreditRestricted
	restricted := stub.execute(syntheticRequest(syntheticTermsMode, "SYN-SCOPE-TERMS-02", "SUB-03", "ACCEPT-03"))
	if restricted.status != syntheticControlCreditRestricted || restricted.reason == "" {
		t.Fatalf("restricted terms result = %#v", restricted)
	}
	assertSyntheticControlTrace(t, restricted)
}

func TestSyntheticControlCannotOverrideResolvedModeOrCrossScope(t *testing.T) {
	stub := newSyntheticControlStub()
	request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SUB-04", "ACCEPT-04")
	request.mode = syntheticTermsMode
	invalid := stub.execute(request)
	if invalid.status != syntheticControlConflict || invalid.reason != "RESOLVED_BASIS_SCOPE_CONFLICT" || stub.freezeCalls != 0 {
		t.Fatalf("caller mode override was accepted: %#v", invalid)
	}

	request = syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SUB-05", "ACCEPT-05")
	request.scope = "SYN-SCOPE-OTHER"
	conflict := stub.execute(request)
	if conflict.status != syntheticControlConflict || conflict.reason != "RESOLVED_BASIS_SCOPE_CONFLICT" || stub.freezeCalls != 0 {
		t.Fatalf("cross-scope basis was accepted: %#v", conflict)
	}
}

// Covers: S01-AT-06
func TestSyntheticUncertainAcceptanceQueriesBeforeCompensation(t *testing.T) {
	stub := newSyntheticControlStub()
	stub.freezeSubmit = syntheticFreezeSubmitUncertain
	request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SUB-06", "ACCEPT-06")
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

// Covers: S01-AT-06
func TestSyntheticAcceptedSubmissionRetainsLegalFreeze(t *testing.T) {
	stub := newSyntheticControlStub()
	request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SUB-07", "ACCEPT-07")
	frozen := stub.execute(request)
	retained := stub.compensateAfterAcceptance(frozen, syntheticAcceptanceEstablished)
	if retained.status != syntheticControlRetained || retained.freeze.state != syntheticFreezeConfirmed || stub.releaseCalls != 0 {
		t.Fatalf("accepted submission released a legal freeze: %#v", retained)
	}
	assertSyntheticControlTrace(t, retained)
}

// Covers: S01-AT-05, S01-AT-06
func TestSyntheticCompensationFailureRemainsPendingAndQueryable(t *testing.T) {
	stub := newSyntheticControlStub()
	request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SUB-08", "ACCEPT-08")
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

// Covers: S01-AT-05, S01-AT-07
func TestSyntheticFinancialControlRejectsStaleOrUnavailableBasis(t *testing.T) {
	stub := newSyntheticControlStub()
	request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SUB-09", "ACCEPT-09")
	request.basis.currentRevision = ""
	stale := stub.execute(request)
	if stale.status != syntheticControlPending || stale.reason != "BASIS_UNAVAILABLE" || stub.freezeCalls != 0 {
		t.Fatalf("stale basis was used: %#v", stale)
	}

	request = syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SUB-09B", "ACCEPT-09B")
	request.basis.resolutionStatus = "APPLICABILITY_CONFLICT"
	conflict := stub.execute(request)
	if conflict.status != syntheticControlPending || conflict.reason != "BASIS_UNAVAILABLE" || stub.freezeCalls != 0 {
		t.Fatalf("non-unique commercial result was used: %#v", conflict)
	}

	stub = newSyntheticControlStub()
	stub.authorityReady = false
	unavailable := stub.execute(syntheticRequest(syntheticTermsMode, "SYN-SCOPE-TERMS-01", "SUB-10", "ACCEPT-10"))
	if unavailable.status != syntheticControlPending || unavailable.continuationRef == "" || stub.externalCalls != 0 {
		t.Fatalf("unavailable authority = %#v", unavailable)
	}
	assertSyntheticControlTrace(t, unavailable)
}

// Covers: S01-AT-08
func TestSyntheticControlPreservesHistoryAndNeverCreatesCashFacts(t *testing.T) {
	stub := newSyntheticControlStub()
	request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SUB-11", "ACCEPT-11")
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
	request := syntheticRequest(syntheticPrepaidMode, "SYN-SCOPE-PREPAID-01", "SUB-12", "ACCEPT-12")
	first := stub.execute(request)
	request.amountMinor++
	second := stub.execute(request)
	if first.status != syntheticControlFrozen || second.status != syntheticControlConflict ||
		second.reason != "REQUEST_IDENTITY_CONFLICT" || stub.freezeCalls != 1 {
		t.Fatalf("conflicting replay was not isolated: first=%#v second=%#v calls=%d", first, second, stub.freezeCalls)
	}
	assertSyntheticControlTrace(t, second)
}
