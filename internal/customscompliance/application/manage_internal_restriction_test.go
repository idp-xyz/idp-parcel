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

var internalRestrictionAt = time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)

// internalRestrictionStoreDouble 是内存里的内部合规限制册：决定追加后下一次读回就带着它，与真库同形。
type internalRestrictionStoreDouble struct {
	byID      map[string]domain.InternalRestriction
	readErr   error
	appendErr error
}

func (double *internalRestrictionStoreDouble) ListInternalByScope(
	context.Context, domain.TenantID, domain.DecisionScopeReference,
) ([]domain.InternalRestriction, error) {
	return nil, double.readErr
}

func (double *internalRestrictionStoreDouble) Save(
	_ context.Context, _ domain.TenantID, restriction domain.InternalRestriction,
) (ports.InternalRestrictionSaveOutcome, error) {
	if _, held := double.byID[restriction.ID().String()]; held {
		return ports.InternalRestrictionAlreadyRecorded, nil
	}
	double.byID[restriction.ID().String()] = restriction
	return ports.InternalRestrictionSaved, nil
}

func (double *internalRestrictionStoreDouble) FindForDecision(
	_ context.Context, _ domain.TenantID, id domain.InternalRestrictionID,
) (domain.InternalRestriction, bool, error) {
	restriction, found := double.byID[id.String()]
	return restriction, found, double.readErr
}

func (double *internalRestrictionStoreDouble) FindByID(
	ctx context.Context, tenant domain.TenantID, id domain.InternalRestrictionID,
) (domain.InternalRestriction, bool, error) {
	return double.FindForDecision(ctx, tenant, id)
}

func (double *internalRestrictionStoreDouble) AppendDecision(
	_ context.Context, _ domain.TenantID, id domain.InternalRestrictionID, decision domain.RestrictionDecision,
) error {
	if double.appendErr != nil {
		return double.appendErr
	}
	decided, err := double.byID[id.String()].Decide(decision)
	if err != nil {
		return err
	}
	double.byID[id.String()] = decided
	return nil
}

type internalRestrictionCase struct {
	t       *testing.T
	store   *internalRestrictionStoreDouble
	handler *application.ManageInternalRestrictionHandler
}

func newInternalRestrictionCase(t *testing.T) *internalRestrictionCase {
	t.Helper()
	store := &internalRestrictionStoreDouble{byID: map[string]domain.InternalRestriction{}}
	handler, err := application.NewManageInternalRestrictionHandler(application.ManageInternalRestrictionDeps{Store: store})
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	return &internalRestrictionCase{t: t, store: store, handler: handler}
}

func (c *internalRestrictionCase) spec(standing domain.CoverageStanding) domain.InternalRestrictionSpec {
	t := c.t
	scope := mustValue(t, domain.NewDecisionScopeReference, "SYN-PCL-IR-01")
	return domain.InternalRestrictionSpec{
		ID:        mustValue(t, domain.NewInternalRestrictionID, "SYN-IR-APP-1"),
		Source:    mustValue(t, domain.NewRestrictionSource, "SYN-SOURCE-TRADE-COMPLIANCE"),
		Basis:     mustValue(t, domain.NewRestrictionBasis, "SYN-RULE-DUAL-USE@v3"),
		Mode:      domain.ManualDecision,
		DecidedBy: mustValue(t, domain.NewRestrictionDecider, "SYN-ROLE-REVIEWER"),
		Condition: mustValue(t, domain.NewRestrictionReleaseCondition, "SYN-COND-LICENCE"),
		StartedAt: internalRestrictionAt,
		Coverage: []domain.CoverageSpec{
			{Scope: scope, Action: domain.LoadingDeparture, Standing: standing},
			{Scope: scope, Action: domain.FinalDelivery, Standing: standing},
		},
	}
}

func (c *internalRestrictionCase) decision(id, source string, kind domain.RestrictionDecisionKind, evidence string, at time.Time, actions ...domain.GuardedAction) application.DecideInternalRestrictionCommand {
	t := c.t
	scope := mustValue(t, domain.NewDecisionScopeReference, "SYN-PCL-IR-01")
	spec := domain.RestrictionDecisionSpec{
		ID:        mustValue(t, domain.NewRestrictionDecisionID, id),
		Kind:      kind,
		Source:    mustValue(t, domain.NewRestrictionSource, source),
		DecidedBy: mustValue(t, domain.NewRestrictionDecider, "SYN-ROLE-REVIEWER"),
		Authority: mustValue(t, domain.NewRestrictionAuthority, "SYN-DELEGATION-1"),
		At:        at,
	}
	if evidence != "" {
		spec.Evidence = mustValue(t, domain.NewRestrictionEvidence, evidence)
	}
	for _, action := range actions {
		spec.Covers = append(spec.Covers, domain.CoverageKey{Scope: scope, Action: action})
	}
	return application.DecideInternalRestrictionCommand{
		TenantID:      mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		RestrictionID: mustValue(t, domain.NewInternalRestrictionID, "SYN-IR-APP-1"),
		Spec:          spec,
	}
}

func (c *internalRestrictionCase) establish(standing domain.CoverageStanding) {
	c.t.Helper()
	result, err := c.handler.Establish(context.Background(), application.EstablishInternalRestrictionCommand{
		TenantID: mustValue(c.t, domain.NewTenantID, "SYN-TENANT-01"), Spec: c.spec(standing),
	})
	if err != nil || result.Outcome() != application.InternalRestrictionEstablished {
		c.t.Fatalf("建立 = %s / %v", result.Outcome(), err)
	}
}

func (c *internalRestrictionCase) decide(command application.DecideInternalRestrictionCommand) application.InternalRestrictionOutcome {
	c.t.Helper()
	result, err := c.handler.Decide(context.Background(), command)
	if err != nil {
		c.t.Fatalf("决定：%v", err)
	}
	return result.Outcome()
}

func expectOutcome(t *testing.T, got, want application.InternalRestrictionOutcome) {
	t.Helper()
	if got != want {
		t.Fatalf("结果 = %s，要 %s", got, want)
	}
}

// 建立：同标识同内容重放答已有，异内容答内容冲突不顶替；没有覆盖项的请求不形成占位限制。
// Covers: AT-CC-345、AT-CC-346（内部合规限制的建立重放与冲突）。
func TestEstablishingAnInternalRestrictionReplaysAndRefusesChangedContent(t *testing.T) {
	c := newInternalRestrictionCase(t)
	c.establish(domain.CoverageInForce)
	tenant := mustValue(t, domain.NewTenantID, "SYN-TENANT-01")

	replay, err := c.handler.Establish(context.Background(), application.EstablishInternalRestrictionCommand{TenantID: tenant, Spec: c.spec(domain.CoverageInForce)})
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, replay.Outcome(), application.InternalRestrictionExisting)

	changed, err := c.handler.Establish(context.Background(), application.EstablishInternalRestrictionCommand{TenantID: tenant, Spec: c.spec(domain.CoverageUnconfirmed)})
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, changed.Outcome(), application.InternalRestrictionContentConflict)

	empty := c.spec(domain.CoverageInForce)
	empty.ID = mustValue(t, domain.NewInternalRestrictionID, "SYN-IR-EMPTY")
	empty.Coverage = nil
	none, err := c.handler.Establish(context.Background(), application.EstablishInternalRestrictionCommand{TenantID: tenant, Spec: empty})
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, none.Outcome(), application.InternalRestrictionNotAccepted)
}

// 决定：只有责任来源的决定成立，部分解除不动其他覆盖项，重放先于核对，异内容冲突，证据缺不成立，限制不存在如实答。
// Covers: AT-CC-452、AT-CC-370（编排半边：确认与解除；相同决定重复到达返回已有决定）。
func TestOnlyTheResponsibleSourcesDecisionsAreMadeAndReplayedFirst(t *testing.T) {
	c := newInternalRestrictionCase(t)
	c.establish(domain.CoverageUnconfirmed)
	source := "SYN-SOURCE-TRADE-COMPLIANCE"
	at := internalRestrictionAt.Add(time.Hour)

	expectOutcome(t, c.decide(c.decision("D-X", "SYN-SOURCE-ADMIN", domain.ReleaseCoverage, "EV-1", at, domain.LoadingDeparture)),
		application.InternalRestrictionNotResponsibleSource)
	expectOutcome(t, c.decide(c.decision("D-X", source, domain.ReleaseCoverage, "", at, domain.LoadingDeparture)),
		application.InternalRestrictionDecisionRejected)
	if len(c.store.byID["SYN-IR-APP-1"].Decisions()) != 0 {
		t.Fatal("不成立的决定不该留下任何一部分")
	}

	confirm := c.decision("D-1", source, domain.ConfirmCoverage, "EV-1", at, domain.LoadingDeparture)
	expectOutcome(t, c.decide(confirm), application.InternalRestrictionDecided)
	expectOutcome(t, c.decide(confirm), application.InternalRestrictionDecisionExisting)
	changed := c.decision("D-1", source, domain.ConfirmCoverage, "EV-OTHER", at, domain.LoadingDeparture)
	expectOutcome(t, c.decide(changed), application.InternalRestrictionDecisionContentConflict)

	expectOutcome(t, c.decide(c.decision("D-2", source, domain.ConfirmCoverage, "EV-2", at.Add(time.Hour), domain.LoadingDeparture)),
		application.InternalRestrictionDecisionRejected)
	release := c.decision("D-3", source, domain.ReleaseCoverage, "EV-3", at.Add(time.Hour), domain.LoadingDeparture)
	expectOutcome(t, c.decide(release), application.InternalRestrictionDecided)
	expectOutcome(t, c.decide(release), application.InternalRestrictionDecisionExisting)
	expectOutcome(t, c.decide(c.decision("D-4", source, domain.ReleaseCoverage, "EV-4", at.Add(2*time.Hour), domain.LoadingDeparture)),
		application.InternalRestrictionDecisionRejected)

	other := c.decision("D-5", source, domain.ReleaseCoverage, "EV-5", at.Add(2*time.Hour), domain.FinalDelivery)
	expectOutcome(t, c.decide(other), application.InternalRestrictionDecided)

	missing := c.decision("D-6", source, domain.ReleaseCoverage, "EV-6", at, domain.LoadingDeparture)
	missing.RestrictionID = mustValue(t, domain.NewInternalRestrictionID, "SYN-IR-NONE")
	expectOutcome(t, c.decide(missing), application.InternalRestrictionNotFound)
}

// 册读写不动答未形成，不翻成业务结论；构造门拒 nil。
func TestInternalRestrictionStoreFailureIsUndecidedAndNilStoreIsRefused(t *testing.T) {
	c := newInternalRestrictionCase(t)
	c.establish(domain.CoverageInForce)
	c.store.appendErr = errors.New("write down")
	expectOutcome(t, c.decide(c.decision("D-1", "SYN-SOURCE-TRADE-COMPLIANCE", domain.ReleaseCoverage, "EV-1", internalRestrictionAt.Add(time.Hour), domain.LoadingDeparture)),
		application.InternalRestrictionUndecided)
	c.store.appendErr = nil
	c.store.readErr = errors.New("read down")
	expectOutcome(t, c.decide(c.decision("D-2", "SYN-SOURCE-TRADE-COMPLIANCE", domain.ReleaseCoverage, "EV-1", internalRestrictionAt.Add(time.Hour), domain.LoadingDeparture)),
		application.InternalRestrictionUndecided)

	if _, err := application.NewManageInternalRestrictionHandler(application.ManageInternalRestrictionDeps{}); !errors.Is(err, application.ErrNilDependency) {
		t.Fatalf("nil 册：err = %v", err)
	}
}
