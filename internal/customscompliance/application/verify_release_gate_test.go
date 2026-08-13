package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

var gateAt = time.Date(2026, 8, 13, 18, 0, 0, 0, time.UTC)

type gateConditionViewDouble struct {
	findings   []domain.PreconditionFinding
	configured bool
	err        error
}

func (double *gateConditionViewDouble) LoadPreconditionFindings(
	_ context.Context,
	_ domain.TenantID,
	_ domain.DecisionScopeReference,
	_ domain.GuardedAction,
	_ domain.CustomsProcedureReference,
) ([]domain.PreconditionFinding, bool, error) {
	if double.err != nil {
		return nil, false, double.err
	}
	return double.findings, double.configured, nil
}

type gateStoreDouble struct {
	byKey map[ports.GateVerificationKey]domain.ReleaseGateVerification
	saved int
}

func (double *gateStoreDouble) FindByKey(
	_ context.Context,
	key ports.GateVerificationKey,
) (domain.ReleaseGateVerification, bool, error) {
	gate, found := double.byKey[key]
	return gate, found, nil
}

func (double *gateStoreDouble) Save(
	_ context.Context,
	key ports.GateVerificationKey,
	gate domain.ReleaseGateVerification,
) (ports.GateVerificationSaveOutcome, error) {
	if _, exists := double.byKey[key]; exists {
		return ports.GateVerificationAlreadyRecorded, nil
	}
	double.byKey[key] = gate
	double.saved++
	return ports.GateVerificationSaved, nil
}

type gateDownstreamDouble struct {
	intents []ports.GateVerificationHandoffIntent
	err     error
}

func (double *gateDownstreamDouble) HandOffGate(
	_ context.Context,
	intent ports.GateVerificationHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type gateFixture struct {
	handler    *application.VerifyReleaseGateHandler
	conditions *gateConditionViewDouble
	store      *gateStoreDouble
	downstream *gateDownstreamDouble
}

func newGateFixture(t *testing.T) *gateFixture {
	t.Helper()
	fixture := &gateFixture{
		conditions: &gateConditionViewDouble{configured: true},
		store:      &gateStoreDouble{byKey: map[ports.GateVerificationKey]domain.ReleaseGateVerification{}},
		downstream: &gateDownstreamDouble{},
	}
	fixture.handler = application.NewVerifyReleaseGateHandler(application.VerifyReleaseGateDeps{
		Conditions: fixture.conditions,
		Store:      fixture.store,
		Downstream: fixture.downstream,
		Clock:      fixedClock{at: gateAt},
	})
	return fixture
}

func gateCommand(t *testing.T) application.VerifyReleaseGateCommand {
	t.Helper()
	return application.VerifyReleaseGateCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Scope:    mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		Action:   domain.OutboundRelease,
		Boundary: mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
	}
}

func finding(t *testing.T, reference string, state domain.PreconditionState) domain.PreconditionFinding {
	t.Helper()
	return domain.PreconditionFinding{
		Precondition: mustValue(t, domain.NewPreconditionReference, reference),
		State:        state,
	}
}

// Covers: CC CONTEXT「放行门禁核对」五值经折叠入册——冲突压过满足与未满足（事实打架
// 不是进度）；混合为部分满足；条件状态变化换指纹换版且两版并存；同状态重复核对返原
// 不出第二版。
func TestGateConclusionsFoldAndVersionByConditionState(t *testing.T) {
	fixture := newGateFixture(t)
	fixture.conditions.findings = []domain.PreconditionFinding{
		finding(t, "DUTY_PAYMENT/verified", domain.PreconditionMet),
		finding(t, "RESTRICTION/released", domain.PreconditionUnmet),
	}

	partial, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("partial handle: %v", err)
	}
	gate, _ := partial.Gate()
	if partial.Outcome() != application.GateVerificationRecorded ||
		gate.Conclusion() != domain.GatePartiallyMet {
		t.Fatalf("outcome = %q conclusion = %q", partial.Outcome(), gate.Conclusion())
	}

	replay, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.GateVerificationExisting || fixture.store.saved != 1 {
		t.Fatalf("replay = %q saved = %d", replay.Outcome(), fixture.store.saved)
	}

	fixture.conditions.findings = []domain.PreconditionFinding{
		finding(t, "DUTY_PAYMENT/verified", domain.PreconditionMet),
		finding(t, "RESTRICTION/released", domain.PreconditionMet),
	}
	met, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("met handle: %v", err)
	}
	metGate, _ := met.Gate()
	if met.Outcome() != application.GateVerificationRecorded ||
		metGate.Conclusion() != domain.GateMet || fixture.store.saved != 2 {
		t.Fatalf("outcome = %q conclusion = %q saved = %d", met.Outcome(), metGate.Conclusion(), fixture.store.saved)
	}

	fixture.conditions.findings = []domain.PreconditionFinding{
		finding(t, "DUTY_PAYMENT/verified", domain.PreconditionConflicting),
		finding(t, "RESTRICTION/released", domain.PreconditionMet),
	}
	conflicting, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("conflicting handle: %v", err)
	}
	conflictGate, _ := conflicting.Gate()
	if conflictGate.Conclusion() != domain.GateConflicting {
		t.Fatalf("conclusion = %q; 冲突必须压过其余状态", conflictGate.Conclusion())
	}
}

// Covers: 编排纪律——目录未登记未决（没有清单的门禁判断无从复核）；空清单折成不适用
// （此动作在此边界不受门禁——如实答案，与未登记分开）；意图失败核对不翻留续办。
func TestGateInventoryGapsStallAndEmptyListsMeanNotApplicable(t *testing.T) {
	fixture := newGateFixture(t)
	fixture.conditions.configured = false

	unconfigured, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("unconfigured handle: %v", err)
	}
	if unconfigured.Outcome() != application.GateVerificationUndecided {
		t.Fatalf("outcome = %q", unconfigured.Outcome())
	}

	fixture.conditions.configured = true
	fixture.conditions.findings = nil
	notApplicable, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("not applicable handle: %v", err)
	}
	gate, _ := notApplicable.Gate()
	if gate.Conclusion() != domain.GateNotApplicable {
		t.Fatalf("conclusion = %q", gate.Conclusion())
	}

	fixture.downstream.err = errors.New("downstream unreachable")
	fixture.conditions.findings = []domain.PreconditionFinding{
		finding(t, "DUTY_PAYMENT/verified", domain.PreconditionMet),
	}
	held, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("held handle: %v", err)
	}
	if held.Outcome() != application.GateVerificationRecorded || held.HandoffReference() == "" {
		t.Fatalf("outcome = %q ref = %q", held.Outcome(), held.HandoffReference())
	}
}
