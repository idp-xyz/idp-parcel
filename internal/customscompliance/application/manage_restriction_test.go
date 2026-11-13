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
) (ports.RestrictionUpdateOutcome, error) {
	if current, exists := double.byID[restriction.ID().String()]; !exists || !current.Current() {
		return ports.RestrictionAlreadySettled, nil
	}
	double.byID[restriction.ID().String()] = restriction
	return ports.RestrictionReleaseRecorded, nil
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

// Covers: 监管限制的编排面（不是 UC-CC-011 的对象，ADR-0241 决定二）——同 ID 重放返原限制不重立；同 ID 异约束集是冒名冲突
// 不顶替，限制身份由建立它的监管决定给出，改内容要新限制；解除只凭监管结果且重复解除按已解除作答（幂等不是错误）。
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

// Covers: CC CONTEXT「只有作用于当前对象和拟执行动作的全部阻断性限制均已解除，相应动作才可继续」的编排面——两条限制同时约束出库时部分解除仍阻断（清单列全）；全部
// 解除后放行；不约束此动作或不同范围的限制不参与。这是监管限制册的事（ADR-0241 决定二）；
// 内部合规限制的同形判据在 domain 包的适用性四格测试里。
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

type acceptedResultsDouble struct {
	bySource map[string]ports.ExternalResultRecord
	err      error
}

func (double *acceptedResultsDouble) FindByKey(_ context.Context, key ports.ExternalResultKey) (ports.ExternalResultRecord, bool, error) {
	if double.err != nil {
		return ports.ExternalResultRecord{}, false, double.err
	}
	record, found := double.bySource[key.SourceID]
	return record, found, nil
}

// regulatoryResult 造一条已接收的外部监管结果记录：层、范围与业务发生时间由用例给。
func regulatoryResult(t *testing.T, source string, layer domain.ResultLayer, scope string, occurredAt time.Time) ports.ExternalResultRecord {
	t.Helper()
	result, err := domain.InterpretExternalResult(domain.ExternalResultSpec{
		Layer:        layer,
		SourceID:     source,
		Role:         mustValue(t, domain.NewSourceAuthorityRole, "customs-authority"),
		RawSemantics: "SYN 监管原文",
		Rule:         mustValue(t, domain.NewInterpretationRuleReference, "interpretation-rule/v1"),
		Version:      mustValue(t, domain.NewSubmissionVersionID, "submission/v1"),
		Attempt:      1,
		Scope:        mustValue(t, domain.NewDecisionScopeReference, scope),
		OccurredAt:   occurredAt,
		ReceivedAt:   occurredAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("interpret %s: %v", source, err)
	}
	return ports.ExternalResultRecord{
		Key:    ports.ExternalResultKey{TenantID: mustValue(t, domain.NewTenantID, "tenant-1"), SourceID: source},
		Result: result,
	}
}

func newBasedRestrictionFixture(t *testing.T, records ...ports.ExternalResultRecord) (*restrictionFixture, *acceptedResultsDouble) {
	t.Helper()
	results := &acceptedResultsDouble{bySource: map[string]ports.ExternalResultRecord{}}
	for _, record := range records {
		results.bySource[record.Key.SourceID] = record
	}
	fixture := &restrictionFixture{
		store:      &restrictionStoreDouble{byID: map[string]domain.RegulatoryRestriction{}},
		downstream: &restrictionDownstreamDouble{},
	}
	fixture.handler = application.NewManageRestrictionHandler(application.ManageRestrictionDeps{
		Results:    results,
		Store:      fixture.store,
		Downstream: fixture.downstream,
		Clock:      fixedClock{at: restrictionAt},
	})
	return fixture, results
}

// Covers: CC CONTEXT「只有责任来源接受的监管结果才能解除相应监管限制」与监管限制要由监管决定立——依据一条已接收的过程
// 决定建立限制：决定身份、范围与生效时刻都取自那条结果，人只声明约束哪些动作；依据同范围、生效之后的一条放行结果解除，
// 解除时刻取放行结果的业务发生时间。
func TestARestrictionIsEstablishedAndReleasedOnAcceptedRegulatoryResults(t *testing.T) {
	heldAt := restrictionAt.Add(-2 * time.Hour)
	releasedAt := restrictionAt.Add(-time.Hour)
	fixture, _ := newBasedRestrictionFixture(t,
		regulatoryResult(t, "hold-1", domain.ProcessDecisionLayer, "parcel-1", heldAt),
		regulatoryResult(t, "release-1", domain.ReleaseResultLayer, "parcel-1", releasedAt),
	)
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	id := mustValue(t, domain.NewRestrictionID, "restriction-1")

	established, err := fixture.handler.EstablishOnResult(context.Background(), application.EstablishRestrictionOnResultCommand{
		TenantID: tenant, ID: id, Decision: "hold-1",
		Constrains: []domain.GuardedAction{domain.OutboundRelease, domain.LoadingDeparture},
	})
	if err != nil || established.Outcome() != application.RestrictionEstablished {
		t.Fatalf("establish: outcome = %q err = %v", established.Outcome(), err)
	}
	restriction, _ := established.Restriction()
	if restriction.Decision().String() != "hold-1" || restriction.Scope().String() != "parcel-1" || !restriction.EffectiveAt().Equal(heldAt) {
		t.Fatalf("限制没有取所引结果的身份、范围与发生时间：%+v", restriction)
	}

	released, err := fixture.handler.ReleaseOnResult(context.Background(), application.ReleaseRestrictionOnResultCommand{
		TenantID: tenant, ID: id, Result: "release-1",
	})
	if err != nil || released.Outcome() != application.RestrictionReleased {
		t.Fatalf("release: outcome = %q err = %v", released.Outcome(), err)
	}
	after, _ := released.Restriction()
	reference, at, done := after.Release()
	if !done || reference.String() != "release-1" || !at.Equal(releasedAt) {
		t.Fatalf("解除依据或时刻不对：%q %v done=%v", reference, at, done)
	}
	if len(fixture.downstream.intents) != 2 {
		t.Fatalf("意图数 = %d，want 2（建立与解除各一份）", len(fixture.downstream.intents))
	}
}

// Covers: 人造不出一条没有监管决定撑着的限制，也凭不了不相干的结果解除——所引结果不在册、归属不上原提交、处在同层冲突
// 里、层次不是决定层、范围不是限制的范围、发生在限制生效之前，或解除时引的就是建立它的那条决定，都答依据不成立，限制册
// 一行不动；外部结果册读不动是没形成答案。
func TestARestrictionBasisMustBeAnAcceptedResultOfTheRightLayerAndScope(t *testing.T) {
	heldAt := restrictionAt.Add(-2 * time.Hour)
	unattributable := regulatoryResult(t, "hold-orphan", domain.ProcessDecisionLayer, "parcel-1", heldAt)
	unattributable.Unattributable = true
	conflicted := regulatoryResult(t, "hold-conflicted", domain.ProcessDecisionLayer, "parcel-1", heldAt)
	conflicted.LayerConflict = true
	fixture, results := newBasedRestrictionFixture(t,
		regulatoryResult(t, "hold-1", domain.ProcessDecisionLayer, "parcel-1", heldAt),
		regulatoryResult(t, "receipt-1", domain.RegulatoryReceiptLayer, "parcel-1", heldAt),
		regulatoryResult(t, "release-elsewhere", domain.ReleaseResultLayer, "parcel-2", heldAt.Add(time.Hour)),
		regulatoryResult(t, "release-early", domain.ReleaseResultLayer, "parcel-1", heldAt.Add(-time.Hour)),
		regulatoryResult(t, "duty-1", domain.AssessedDutyLayer, "parcel-1", heldAt.Add(time.Hour)),
		unattributable, conflicted,
	)
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	id := mustValue(t, domain.NewRestrictionID, "restriction-1")
	constrains := []domain.GuardedAction{domain.OutboundRelease}

	for _, decision := range []string{"hold-missing", "hold-orphan", "hold-conflicted", "receipt-1"} {
		refused, err := fixture.handler.EstablishOnResult(context.Background(), application.EstablishRestrictionOnResultCommand{
			TenantID: tenant, ID: id, Decision: decision, Constrains: constrains,
		})
		if err != nil || refused.Outcome() != application.RestrictionBasisNotAccepted {
			t.Fatalf("以 %s 建立：outcome = %q err = %v, want BASIS_NOT_ACCEPTED", decision, refused.Outcome(), err)
		}
	}
	if len(fixture.store.byID) != 0 {
		t.Fatal("依据不成立的建立留下了限制")
	}

	if established, err := fixture.handler.EstablishOnResult(context.Background(), application.EstablishRestrictionOnResultCommand{
		TenantID: tenant, ID: id, Decision: "hold-1", Constrains: constrains,
	}); err != nil || established.Outcome() != application.RestrictionEstablished {
		t.Fatalf("establish: outcome = %q err = %v", established.Outcome(), err)
	}
	for _, result := range []string{"release-missing", "release-elsewhere", "release-early", "duty-1", "hold-1"} {
		refused, err := fixture.handler.ReleaseOnResult(context.Background(), application.ReleaseRestrictionOnResultCommand{
			TenantID: tenant, ID: id, Result: result,
		})
		if err != nil || refused.Outcome() != application.RestrictionBasisNotAccepted {
			t.Fatalf("以 %s 解除：outcome = %q err = %v, want BASIS_NOT_ACCEPTED", result, refused.Outcome(), err)
		}
	}
	if kept := fixture.store.byID["restriction-1"]; !kept.Current() {
		t.Fatal("依据不成立的解除把限制解除了")
	}

	results.err = errors.New("connection refused")
	undecided, err := fixture.handler.ReleaseOnResult(context.Background(), application.ReleaseRestrictionOnResultCommand{
		TenantID: tenant, ID: id, Result: "release-1",
	})
	if err != nil || undecided.Outcome() != application.RestrictionUndecided {
		t.Fatalf("结果册读不动：outcome = %q err = %v, want UNDECIDED", undecided.Outcome(), err)
	}
}

// Covers: 解除只落一次——两笔解除都读到限制当前有效，先落的一笔成；后落的一笔答已解除并交回在册那一份，先落的解除依据
// 不被盖掉。
func TestALateReleaseDoesNotOverwriteTheEarlierReleaseBasis(t *testing.T) {
	fixture := newRestrictionFixture(t)
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	spec := restrictionSpec(t, "restriction-1", domain.OutboundRelease)
	if _, err := fixture.handler.Establish(context.Background(), application.EstablishRestrictionCommand{TenantID: tenant, Spec: spec}); err != nil {
		t.Fatalf("establish: %v", err)
	}
	stale := fixture.store.byID["restriction-1"]
	racing := application.NewManageRestrictionHandler(application.ManageRestrictionDeps{
		Store:      &staleRestrictionRead{restrictionStoreDouble: fixture.store, stale: stale},
		Downstream: fixture.downstream,
		Clock:      fixedClock{at: restrictionAt},
	})
	release := func(handler *application.ManageRestrictionHandler, basis string) application.RestrictionResult {
		t.Helper()
		result, err := handler.Release(context.Background(), application.ReleaseRestrictionCommand{
			TenantID: tenant, ID: spec.ID,
			Release: mustValue(t, domain.NewRegulatoryReleaseReference, basis), At: restrictionAt,
		})
		if err != nil {
			t.Fatalf("release on %s: %v", basis, err)
		}
		return result
	}
	if first := release(fixture.handler, "release-first"); first.Outcome() != application.RestrictionReleased {
		t.Fatalf("先落的解除：outcome = %q", first.Outcome())
	}
	late := release(racing, "release-late")
	if late.Outcome() != application.RestrictionAlreadyReleased {
		t.Fatalf("后落的解除：outcome = %q, want ALREADY_RELEASED", late.Outcome())
	}
	kept, _, _ := fixture.store.byID["restriction-1"].Release()
	if kept.String() != "release-first" {
		t.Fatalf("在册解除依据 = %q，被后到的那一笔盖掉了", kept)
	}
}

// staleRestrictionRead 第一次读交回一份早先读到的旧限制，此后与写都落到真替身上：模拟「读到当前有效之后、落解除之前
// 另一笔先落了」。
type staleRestrictionRead struct {
	*restrictionStoreDouble
	stale domain.RegulatoryRestriction
	read  bool
}

func (store *staleRestrictionRead) FindByID(ctx context.Context, tenant domain.TenantID, id domain.RestrictionID) (domain.RegulatoryRestriction, bool, error) {
	if !store.read {
		store.read = true
		return store.stale, true, nil
	}
	return store.restrictionStoreDouble.FindByID(ctx, tenant, id)
}
