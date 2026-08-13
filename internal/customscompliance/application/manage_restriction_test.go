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

var restrictionAt = time.Date(2026, 8, 13, 17, 0, 0, 0, time.UTC)

type restrictionStoreDouble struct {
	byID map[string]domain.RegulatoryRestriction
}

func (double *restrictionStoreDouble) FindByID(
	_ context.Context,
	_ domain.TenantID,
	id domain.RestrictionID,
) (domain.RegulatoryRestriction, bool, error) {
	restriction, found := double.byID[id.String()]
	return restriction, found, nil
}

func (double *restrictionStoreDouble) ListByScope(
	_ context.Context,
	_ domain.TenantID,
	scope domain.DecisionScopeReference,
) ([]domain.RegulatoryRestriction, error) {
	matching := make([]domain.RegulatoryRestriction, 0)
	for _, restriction := range double.byID {
		if restriction.Scope() == scope {
			matching = append(matching, restriction)
		}
	}
	return matching, nil
}

func (double *restrictionStoreDouble) Save(
	_ context.Context,
	_ domain.TenantID,
	restriction domain.RegulatoryRestriction,
) (ports.RestrictionSaveOutcome, error) {
	if _, exists := double.byID[restriction.ID().String()]; exists {
		return ports.RestrictionAlreadyRecorded, nil
	}
	double.byID[restriction.ID().String()] = restriction
	return ports.RestrictionSaved, nil
}

func (double *restrictionStoreDouble) Update(
	_ context.Context,
	_ domain.TenantID,
	restriction domain.RegulatoryRestriction,
) error {
	double.byID[restriction.ID().String()] = restriction
	return nil
}

type restrictionDownstreamDouble struct {
	intents []ports.RestrictionHandoffIntent
	err     error
}

func (double *restrictionDownstreamDouble) HandOffRestriction(
	_ context.Context,
	intent ports.RestrictionHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type restrictionFixture struct {
	handler    *application.ManageRestrictionHandler
	store      *restrictionStoreDouble
	downstream *restrictionDownstreamDouble
}

func newRestrictionFixture(t *testing.T) *restrictionFixture {
	t.Helper()
	fixture := &restrictionFixture{
		store:      &restrictionStoreDouble{byID: map[string]domain.RegulatoryRestriction{}},
		downstream: &restrictionDownstreamDouble{},
	}
	fixture.handler = application.NewManageRestrictionHandler(application.ManageRestrictionDeps{
		Store:      fixture.store,
		Downstream: fixture.downstream,
		Clock:      fixedClock{at: restrictionAt},
	})
	return fixture
}

func restrictionSpec(t *testing.T, id string, constrains ...domain.GuardedAction) domain.RegulatoryRestrictionSpec {
	t.Helper()
	return domain.RegulatoryRestrictionSpec{
		ID:          mustValue(t, domain.NewRestrictionID, id),
		Decision:    mustValue(t, domain.NewRegulatoryDecisionID, "decision-1"),
		Scope:       mustValue(t, domain.NewDecisionScopeReference, "parcel-1"),
		Constrains:  constrains,
		EffectiveAt: restrictionAt.Add(-time.Hour),
	}
}

// Covers: UC-CC-011 的编排面——同 ID 重放返原限制不重立（`AT-CC-345`「相同限制形成
// 请求……再次到达→返回已有限制结果，不创建第二限制」）；同 ID 异约束集是冒名冲突
// 不顶替（`AT-CC-346`「相同限制请求身份携带不同来源、范围、动作或依据→形成限制请求
// 冲突」；限制身份由建立它的监管决定给出，改内容要新限制）；解除只凭监管结果且重复
// 解除按已解除作答（幂等不是错误，`AT-CC-370`「相同解除请求和相同内容重复到达→返回
// 已有解除结果」）。
func TestRestrictionsEstablishOnceAndReleaseIdempotently(t *testing.T) {
	fixture := newRestrictionFixture(t)

	established, err := fixture.handler.Establish(context.Background(), application.EstablishRestrictionCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Spec:     restrictionSpec(t, "restriction-1", domain.OutboundRelease, domain.FinalDelivery),
	})
	if err != nil {
		t.Fatalf("establish: %v", err)
	}
	if established.Outcome() != application.RestrictionEstablished {
		t.Fatalf("outcome = %q", established.Outcome())
	}

	replay, err := fixture.handler.Establish(context.Background(), application.EstablishRestrictionCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Spec:     restrictionSpec(t, "restriction-1", domain.OutboundRelease, domain.FinalDelivery),
	})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.RestrictionExisting {
		t.Fatalf("replay = %q", replay.Outcome())
	}

	impostor, err := fixture.handler.Establish(context.Background(), application.EstablishRestrictionCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Spec:     restrictionSpec(t, "restriction-1", domain.CrossCustomsMovement),
	})
	if err != nil {
		t.Fatalf("impostor: %v", err)
	}
	if impostor.Outcome() != application.RestrictionContentConflict {
		t.Fatalf("impostor = %q; 同 ID 换约束集必须是冲突", impostor.Outcome())
	}

	release := application.ReleaseRestrictionCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		ID:       mustValue(t, domain.NewRestrictionID, "restriction-1"),
		Release:  mustValue(t, domain.NewRegulatoryReleaseReference, "RELEASE/final-clearance"),
		At:       restrictionAt,
	}
	released, err := fixture.handler.Release(context.Background(), release)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.Outcome() != application.RestrictionReleased {
		t.Fatalf("outcome = %q", released.Outcome())
	}

	again, err := fixture.handler.Release(context.Background(), release)
	if err != nil {
		t.Fatalf("release again: %v", err)
	}
	if again.Outcome() != application.RestrictionAlreadyReleased {
		t.Fatalf("again = %q", again.Outcome())
	}

	missing := release
	missing.ID = mustValue(t, domain.NewRestrictionID, "restriction-9")
	notFound, err := fixture.handler.Release(context.Background(), missing)
	if err != nil {
		t.Fatalf("release missing: %v", err)
	}
	if notFound.Outcome() != application.RestrictionNotFound {
		t.Fatalf("missing = %q", notFound.Outcome())
	}
}

// Covers: CC CONTEXT「只有作用于当前对象和拟执行动作的全部阻断性限制均已解除，相应
// 动作才可继续」的编排面——两条限制同时约束出库时部分解除仍阻断（清单列全）；全部
// 解除后放行；不约束此动作或不同范围的限制不参与。点名 `AT-CC-347`「两个独立来源均
// 限制同一对象……当前动作受全部有效限制共同阻断」与 `AT-CC-348`「S1 解除但 S2 仍
// 有效→S1 的解除有效，S2 继续阻断」。
func TestActionJudgmentListsEveryBlockerUntilAllRelease(t *testing.T) {
	fixture := newRestrictionFixture(t)
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")

	for _, id := range []string{"restriction-a", "restriction-b"} {
		if _, err := fixture.handler.Establish(context.Background(), application.EstablishRestrictionCommand{
			TenantID: tenant,
			Spec:     restrictionSpec(t, id, domain.OutboundRelease),
		}); err != nil {
			t.Fatalf("establish %s: %v", id, err)
		}
	}
	if _, err := fixture.handler.Establish(context.Background(), application.EstablishRestrictionCommand{
		TenantID: tenant,
		Spec:     restrictionSpec(t, "restriction-other-action", domain.FinalDelivery),
	}); err != nil {
		t.Fatalf("establish other action: %v", err)
	}

	scope := mustValue(t, domain.NewDecisionScopeReference, "parcel-1")
	blocked, err := fixture.handler.JudgeAction(context.Background(), tenant, domain.OutboundRelease, scope)
	if err != nil {
		t.Fatalf("judge blocked: %v", err)
	}
	if blocked.Admissible() || len(blocked.BlockedBy()) != 2 {
		t.Fatalf("admissible = %v blockers = %d; 两条都得在清单上", blocked.Admissible(), len(blocked.BlockedBy()))
	}

	if _, err := fixture.handler.Release(context.Background(), application.ReleaseRestrictionCommand{
		TenantID: tenant,
		ID:       mustValue(t, domain.NewRestrictionID, "restriction-a"),
		Release:  mustValue(t, domain.NewRegulatoryReleaseReference, "RELEASE/partial"),
		At:       restrictionAt,
	}); err != nil {
		t.Fatalf("release a: %v", err)
	}
	partial, err := fixture.handler.JudgeAction(context.Background(), tenant, domain.OutboundRelease, scope)
	if err != nil {
		t.Fatalf("judge partial: %v", err)
	}
	if partial.Admissible() {
		t.Fatal("部分解除就放行了——全部解除才可继续")
	}

	if _, err := fixture.handler.Release(context.Background(), application.ReleaseRestrictionCommand{
		TenantID: tenant,
		ID:       mustValue(t, domain.NewRestrictionID, "restriction-b"),
		Release:  mustValue(t, domain.NewRegulatoryReleaseReference, "RELEASE/full"),
		At:       restrictionAt,
	}); err != nil {
		t.Fatalf("release b: %v", err)
	}
	clear, err := fixture.handler.JudgeAction(context.Background(), tenant, domain.OutboundRelease, scope)
	if err != nil {
		t.Fatalf("judge clear: %v", err)
	}
	if !clear.Admissible() {
		t.Fatal("全部解除后仍被阻断")
	}

	// 意图失败不翻结果——建立一条新限制验证续办引用。
	fixture.downstream.err = errors.New("downstream unreachable")
	held, err := fixture.handler.Establish(context.Background(), application.EstablishRestrictionCommand{
		TenantID: tenant,
		Spec:     restrictionSpec(t, "restriction-held", domain.LoadingDeparture),
	})
	if err != nil {
		t.Fatalf("held establish: %v", err)
	}
	if held.Outcome() != application.RestrictionEstablished || held.HandoffReference() == "" {
		t.Fatalf("outcome = %q ref = %q", held.Outcome(), held.HandoffReference())
	}
}
