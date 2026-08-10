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

// 本文件刻意只存在于测试侧。它承担 PN02-SYN 任务包里 SYN-CHAIN 联检的 parcel-shipment
// 半边，在既有的提交前值对象之上断言「来源保全 → 生产归属 → 未来建单门禁」这个次序。它不
// 引入编排器、端口、聚合、领域事件、仓储、事务或发件箱。
//
// SYN-CHAIN-04 有意缺席：它横跨 party-commercial 与 settlement-accounting，两者都不拥有
// 生产类型，所以它的两个半边归那两个上下文自己的合约测试。
//
// 每个测试带一行 `Covers:`，只写它确实断言到的东西，于是
// `rg "^// Covers:.*SYN-CHAIN-05"` 能机械地回答覆盖问题。要匹配标注行而不是裸 ID——裸 ID
// 也会命中上面那段关于 SYN-CHAIN-04 的散文，把一个缺席的场景报成已覆盖。
//
// 测试只断言场景的某个侧面而非全部时，在括号里写明是哪个侧面。不加限定的一行等于声称覆盖
// 了整个场景，而夸大的声称比不声称更糟：它让 grep 说谎。
//
// 每个场景各占一行 `// Covers:`。折行的标注会把第二个场景挤到锚定模式看不见的续行上，于是
// 一个已覆盖的场景被报成缺失。

const syntheticChainFixtureVersion = "SYN-CHAIN-FIXTURE-v1"

var (
	syntheticChainValidFrom  = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	syntheticChainValidUntil = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	syntheticChainAsOf       = time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	syntheticChainDecidedAt  = time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC)
	syntheticChainGateAt     = time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
)

// syntheticChainScope 从一份已保全的来源提交推导准入范围。生产侧刻意让 AdmissionScope 保持
// 不透明、也不与 PayloadDigest 挂钩，所以推导落在测试侧：联检需要这条血缘来断言次序，而领域
// 现在还不该拥有它。
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

// Covers: SYN-CHAIN-01, S02-AT-02
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

func syntheticChainHandoffSpec(
	t *testing.T,
	scope domain.AdmissionScope,
	observation domain.HandoffObservation,
) domain.SafeHandoffAssessmentSpec {
	t.Helper()
	return domain.SafeHandoffAssessmentSpec{
		AttemptID:       mustValue(t, domain.NewHandoffAttemptID, "SYN-CHAIN-HANDOFF-1"),
		Scope:           scope,
		TargetAuthority: mustValue(t, domain.NewProductionAuthorityReference, "SYN-CHAIN-AUTHORITY-OTHER-1"),
		Observation:     observation,
		AssessedAt:      syntheticChainDecidedAt,
	}
}

// Covers: SYN-CHAIN-03, S02-AT-04
func TestSyntheticChainConfirmedHandoffYieldsNeutralOtherAuthority(t *testing.T) {
	preserved := sourceFingerprint(t, "SYN-TENANT-1", "SYN-CUSTOMER-1", "SYN-SOURCE-A", "SYN-KEY-1", "SYN-DIGEST-1")
	scope := syntheticChainScope(t, preserved)

	spec := syntheticChainHandoffSpec(t, scope, domain.HandoffObservationCompleteConfirmation)
	spec.ConfirmedScopeDigest = scope.Digest()
	spec.ConfirmationRef = mustValue(t, domain.NewHandoffConfirmationReference, "SYN-CHAIN-CONFIRM-1")
	spec.QueryRef = mustValue(t, domain.NewHandoffQueryReference, "SYN-CHAIN-QUERY-1")
	spec.EffectiveAt = syntheticChainAsOf

	assessment, err := domain.AssessSafeHandoff(spec)
	if err != nil {
		t.Fatalf("assess safe handoff: %v", err)
	}
	if assessment.Status() != domain.SafeHandoffConfirmed {
		t.Fatalf("status = %s, want CONFIRMED", assessment.Status())
	}
	confirmation, ok := assessment.ConfirmationReference()
	if !ok {
		t.Fatal("confirmed handoff withheld its confirmation reference")
	}

	decisionSpec := syntheticChainDecisionSpec(t, scope, domain.ProductionAuthorityOther, "SYN-CHAIN-REV-1")
	decisionSpec.HandoffRef = confirmation
	decision := mustDecision(t, decisionSpec)

	gate, err := domain.EvaluateFutureSubmissionGate(
		decision,
		scope.Digest(),
		mustValue(t, domain.NewProductionOwnershipRevision, "SYN-CHAIN-REV-1"),
		syntheticChainGateAt,
	)
	if err != nil {
		t.Fatalf("evaluate future submission gate: %v", err)
	}
	reasons := gate.BlockReasons()
	if gate.IsAllowed() || len(reasons) != 1 || reasons[0] != domain.FutureSubmissionOtherAuthority {
		t.Fatalf("other authority did not stay neutral: allowed = %t, reasons = %v", gate.IsAllowed(), reasons)
	}
	if got, ok := decision.HandoffReference(); !ok || got != confirmation {
		t.Fatal("neutral association lost the handoff confirmation it must cite")
	}
	if _, _, ok := decision.UnresolvedDetails(); ok {
		t.Fatal("confirmed handoff still produced unresolved details")
	}
}

// Covers: SYN-CHAIN-03, S02-AT-05
func TestSyntheticChainUnresolvedHandoffCannotCarryOtherAuthority(t *testing.T) {
	preserved := sourceFingerprint(t, "SYN-TENANT-1", "SYN-CUSTOMER-1", "SYN-SOURCE-A", "SYN-KEY-1", "SYN-DIGEST-1")
	scope := syntheticChainScope(t, preserved)
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
			},
		},
		{
			name:        "confirmation cannot be queried",
			observation: domain.HandoffObservationQueryUnavailable,
			edit: func(spec *domain.SafeHandoffAssessmentSpec) {
				spec.ConfirmedScopeDigest = spec.Scope.Digest()
				spec.ConfirmationRef = confirmation
			},
		},
		{
			name:        "timed out",
			observation: domain.HandoffObservationTimedOut,
			edit:        func(*domain.SafeHandoffAssessmentSpec) {},
		},
		{
			name:        "target failure",
			observation: domain.HandoffObservationFailed,
			edit:        func(*domain.SafeHandoffAssessmentSpec) {},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := syntheticChainHandoffSpec(t, scope, test.observation)
			spec.ContinuationRef = continuation
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
			if got, ok := assessment.ContinuationReference(); !ok || got != continuation {
				t.Fatal("unresolved handoff dropped the continuation reference it must retain")
			}

			decisionSpec := syntheticChainDecisionSpec(t, scope, domain.ProductionAuthorityOther, "SYN-CHAIN-REV-1")
			decisionSpec.HandoffRef = handoffRef
			_, err = domain.NewProductionOwnershipDecision(decisionSpec)
			if !errors.Is(err, domain.ErrInvalidProductionOwnershipDecision) {
				t.Fatalf("OTHER authority accepted an unresolved handoff: err = %v", err)
			}
		})
	}
}

// Covers: SYN-CHAIN-05
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
	// 先前那个值对象不可变，读出来仍是 ALLOWED。这恰恰是链条下一步不得把它直接带走的原因：
	// 失效只有拿当前修订重新评价才看得见。
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

// syntheticOperationalFacts 把已保全的来源与其他上下文拥有的运营事实放在一起：装载单元归属
// 与封签（node-operations）、承运商分配的外部单号，以及路由计划（network-routing）。它们被
// 有意带到推导的调用点上——正是这一点让 S02-AT-09 在其中任何一个变成输入时失败。
type syntheticOperationalFacts struct {
	preserved          domain.SourceSubmissionFingerprint
	consignmentUnitID  string
	sealID             string
	lastMileTrackingNo string
	routingPlanID      string
}

func syntheticOperationalScope(
	t *testing.T,
	facts syntheticOperationalFacts,
) domain.AdmissionScope {
	t.Helper()
	return syntheticChainScope(t, facts.preserved)
}

// Covers: S02-AT-09
func TestSyntheticChainOperationalChangesDoNotMoveProductionAuthority(t *testing.T) {
	base := syntheticOperationalFacts{
		preserved:          sourceFingerprint(t, "SYN-TENANT-1", "SYN-CUSTOMER-1", "SYN-SOURCE-A", "SYN-KEY-1", "SYN-DIGEST-1"),
		consignmentUnitID:  "SYN-BAG-ORIGIN-01",
		sealID:             "SYN-SEAL-01",
		lastMileTrackingNo: "SYN-LASTMILE-01",
		routingPlanID:      "SYN-ROUTE-PLAN-01",
	}
	scope := syntheticOperationalScope(t, base)
	revision := mustValue(t, domain.NewProductionOwnershipRevision, "SYN-CHAIN-REV-1")
	decision := syntheticChainDecision(t, scope, domain.ProductionAuthorityIDPParcel, "SYN-CHAIN-REV-1")
	gate, err := domain.EvaluateFutureSubmissionGate(decision, scope.Digest(), revision, syntheticChainGateAt)
	if err != nil {
		t.Fatalf("evaluate future submission gate: %v", err)
	}
	if !gate.IsAllowed() {
		t.Fatalf("baseline gate blocked: reasons = %v", gate.BlockReasons())
	}

	changes := map[string]func(*syntheticOperationalFacts){
		"re-bagged at a transit hub": func(facts *syntheticOperationalFacts) {
			facts.consignmentUnitID = "SYN-BAG-TRANSIT-07"
			facts.sealID = "SYN-SEAL-07"
		},
		"last-mile carrier assigned a new number": func(facts *syntheticOperationalFacts) {
			facts.lastMileTrackingNo = "SYN-LASTMILE-99"
		},
		"lane closed and the plan was rerouted": func(facts *syntheticOperationalFacts) {
			facts.routingPlanID = "SYN-ROUTE-PLAN-02"
		},
	}

	for name, apply := range changes {
		t.Run(name, func(t *testing.T) {
			changed := base
			apply(&changed)
			if changed == base {
				t.Fatal("the operational change left the fixture untouched, so it proves nothing")
			}

			changedScope := syntheticOperationalScope(t, changed)
			if changedScope.Digest() != scope.Digest() {
				t.Fatal("an operational fact reached the admission scope digest")
			}
			changedDecision := syntheticChainDecision(t, changedScope, domain.ProductionAuthorityIDPParcel, "SYN-CHAIN-REV-1")
			if changedDecision != decision {
				t.Fatal("an operational fact produced a different production ownership decision")
			}
			changedGate, err := domain.EvaluateFutureSubmissionGate(
				changedDecision,
				changedScope.Digest(),
				revision,
				syntheticChainGateAt,
			)
			if err != nil {
				t.Fatalf("re-evaluate future submission gate: %v", err)
			}
			if !changedGate.IsAllowed() || len(changedGate.BlockReasons()) != 0 {
				t.Fatalf("an operational fact disturbed the gate: reasons = %v", changedGate.BlockReasons())
			}
		})
	}
}

// Covers: SYN-CHAIN-06
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

// Covers: SYN-CHAIN-07
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

	// 扫描范围远超当前枚举，好让新加的处置在这里冒出来，而不是悄悄混进去。
	const dispositionScanLimit = 32
	wantDispositions := map[string]bool{"ALLOWED": true, "BLOCKED": true}
	gotDispositions := map[string]bool{}
	for value := 1; value <= dispositionScanLimit; value++ {
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
