package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var factOccurredAt = time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC)

func mustValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type factStoreDouble struct {
	byKey map[ports.FactKey]ports.FactRecord
	saved int
}

func newFactStore() *factStoreDouble {
	return &factStoreDouble{byKey: map[ports.FactKey]ports.FactRecord{}}
}

func (double *factStoreDouble) FindByKey(_ context.Context, key ports.FactKey) (ports.FactRecord, bool, error) {
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *factStoreDouble) FindByParcel(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
) ([]ports.FactRecord, error) {
	records := make([]ports.FactRecord, 0)
	for _, record := range double.byKey {
		if record.Key.Tenant == tenant && record.Fact.Parcel() == parcel {
			records = append(records, record)
		}
	}
	return records, nil
}

func (double *factStoreDouble) Save(_ context.Context, record ports.FactRecord) (ports.FactSaveOutcome, error) {
	if _, exists := double.byKey[record.Key]; exists {
		return ports.FactAlreadyRecorded, nil
	}
	double.byKey[record.Key] = record
	double.saved++
	return ports.FactSaved, nil
}

type mappingViewDouble struct {
	answer     ports.MilestoneAnswer
	configured bool
	err        error
}

func (double *mappingViewDouble) ClassifyFact(
	_ context.Context,
	_ domain.AcceptedSourceFact,
) (ports.MilestoneAnswer, bool, error) {
	if double.err != nil {
		return ports.MilestoneAnswer{}, false, double.err
	}
	return double.answer, double.configured, nil
}

type projectionStoreDouble struct {
	byKey map[projectionKey]domain.TrackingProjection
}

type projectionKey struct {
	tenant domain.TenantID
	parcel domain.TrackedParcelReference
}

func (double *projectionStoreDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
) (domain.TrackingProjection, bool, error) {
	projection, found := double.byKey[projectionKey{tenant: tenant, parcel: parcel}]
	return projection, found, nil
}

func (double *projectionStoreDouble) Save(_ context.Context, tenant domain.TenantID, projection domain.TrackingProjection) error {
	double.byKey[projectionKey{tenant: tenant, parcel: projection.Parcel()}] = projection
	return nil
}

type projectionIdentityDouble struct{ next int }

func (double *projectionIdentityDouble) NextProjectionVersionID(_ context.Context) (domain.ProjectionVersionID, error) {
	double.next++
	return domain.NewProjectionVersionID("projection-" + string(rune('0'+double.next)))
}

type projectionDownstreamDouble struct {
	intents []ports.ProjectionHandoffIntent
	err     error
}

func (double *projectionDownstreamDouble) HandOffProjection(
	_ context.Context,
	intent ports.ProjectionHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

type deriveFixture struct {
	handler     *application.DeriveProjectionHandler
	facts       *factStoreDouble
	mapping     *mappingViewDouble
	projections *projectionStoreDouble
	downstream  *projectionDownstreamDouble
}

func newDeriveFixture(t *testing.T) *deriveFixture {
	t.Helper()
	fixture := &deriveFixture{
		facts: newFactStore(),
		mapping: &mappingViewDouble{
			answer: ports.MilestoneAnswer{
				Milestone:  mustValue(t, domain.NewMilestoneReference, "PICKED_UP"),
				Classified: true,
				Mapping:    mustValue(t, domain.NewMappingVersionReference, "milestone-map/v1"),
			},
			configured: true,
		},
		projections: &projectionStoreDouble{byKey: map[projectionKey]domain.TrackingProjection{}},
		downstream:  &projectionDownstreamDouble{},
	}
	fixture.handler = application.NewDeriveProjectionHandler(application.DeriveProjectionDeps{
		Facts:       fixture.facts,
		Mapping:     fixture.mapping,
		Projections: fixture.projections,
		Identities:  &projectionIdentityDouble{},
		Downstream:  fixture.downstream,
		Clock:       fixedClock{at: factOccurredAt.Add(2 * time.Hour)},
	})
	return fixture
}

func deriveCommand(t *testing.T, factRef, version string) application.DeriveProjectionCommand {
	t.Helper()
	return application.DeriveProjectionCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Fact: domain.AcceptedSourceFactSpec{
			Source:      domain.SourceNodeOperations,
			Parcel:      mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
			Fact:        mustValue(t, domain.NewSourceFactReference, factRef),
			Version:     mustValue(t, domain.NewSourceFactVersion, version),
			OccurredAt:  factOccurredAt,
			EffectiveAt: factOccurredAt,
			ReceivedAt:  factOccurredAt.Add(time.Hour),
		},
	}
}

// Covers: ADR-0003 隔离半边——租户不随命令到达即未受理，编排不替来源补租户，也不读
// 任何依赖。
func TestACommandWithoutATenantIsNotAccepted(t *testing.T) {
	fixture := newDeriveFixture(t)

	command := deriveCommand(t, "NODE-INTAKE/a", "v1")
	command.TenantID = domain.TenantID{}
	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.FactNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED", result.Outcome())
	}
	if fixture.facts.saved != 0 {
		t.Fatal("缺租户的命令仍然写了事实库")
	}
}

// Covers: VE CONTEXT 生命周期「源上下文接受事实或有效性变化→按标准追踪里程碑映射
// ……形成新的投影版本」与「有效事实可以可靠排序或替代→重新派生……原投影版本继续
// 保留」——首事实派生首版，第二事实重派生指回前版；投影意图随提交交付。点名
// `AT-VE-038`「已接受节点收寄事实首次进入投影→形成来源引用和节点里程碑」与
// `AT-VE-044` 的版本追加半边「追加投影版本，原版本保留」。
func TestFactsDeriveAndRederiveTheProjection(t *testing.T) {
	fixture := newDeriveFixture(t)

	first, err := fixture.handler.Handle(context.Background(), deriveCommand(t, "NODE-INTAKE/a", "v1"))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if first.Outcome() != application.ProjectionDerived {
		t.Fatalf("outcome = %q", first.Outcome())
	}
	projection, _ := first.Projection()
	if _, rederived := projection.PriorVersion(); rederived {
		t.Fatal("首版凭空长出了前版")
	}

	second, err := fixture.handler.Handle(context.Background(), deriveCommand(t, "TRANSPORT-MOVE/b", "v1"))
	if err != nil {
		t.Fatalf("second handle: %v", err)
	}
	rederived, _ := second.Projection()
	prior, present := rederived.PriorVersion()
	if !present || prior != projection.Version() {
		t.Fatalf("prior = %s present = %v; 重派生必须指回前版", prior, present)
	}
	if len(rederived.Entries()) != 2 {
		t.Fatalf("entries = %d", len(rederived.Entries()))
	}
	if len(fixture.downstream.intents) != 2 {
		t.Fatalf("intents = %d", len(fixture.downstream.intents))
	}
}

// Covers: 幂等与冲突纪律——同键同内容重放按当前投影作答不重复派生；同键异内容是
// 来源冲突保留原事实（不按最后到达覆盖）；映射目录未配置即如实未归类、投影照常派生
// （无法可靠映射不强行映射也不阻断——那不是未决）。点名 `AT-VE-039`「同一来源版本
// 重复到达→返回原投影」、`AT-VE-040`「来源身份携带不同对象范围→形成冲突并待确认」
// 与 `AT-VE-042`「已接受事实没有可靠映射→形成未归类，不写成运输中」。
func TestReplayConflictAndUnconfiguredMappingStayHonest(t *testing.T) {
	fixture := newDeriveFixture(t)
	if _, err := fixture.handler.Handle(context.Background(), deriveCommand(t, "NODE-INTAKE/a", "v1")); err != nil {
		t.Fatalf("first handle: %v", err)
	}

	replay, err := fixture.handler.Handle(context.Background(), deriveCommand(t, "NODE-INTAKE/a", "v1"))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.FactExistingResult {
		t.Fatalf("replay = %q", replay.Outcome())
	}
	if fixture.facts.saved != 1 {
		t.Fatal("重放重复保存了事实")
	}

	conflicting := deriveCommand(t, "NODE-INTAKE/a", "v1")
	conflicting.Fact.OccurredAt = factOccurredAt.Add(time.Minute)
	conflict, err := fixture.handler.Handle(context.Background(), conflicting)
	if err != nil {
		t.Fatalf("conflict handle: %v", err)
	}
	if conflict.Outcome() != application.FactSourceConflict {
		t.Fatalf("conflict = %q", conflict.Outcome())
	}

	unconfigured := newDeriveFixture(t)
	unconfigured.mapping.configured = false
	derived, err := unconfigured.handler.Handle(context.Background(), deriveCommand(t, "NODE-INTAKE/a", "v1"))
	if err != nil {
		t.Fatalf("unconfigured handle: %v", err)
	}
	if derived.Outcome() != application.ProjectionDerived {
		t.Fatalf("outcome = %q; 映射未配置不阻断投影", derived.Outcome())
	}
	projection, _ := derived.Projection()
	if _, classified := projection.Entries()[0].Milestone(); classified {
		t.Fatal("映射未配置却归了类——强行映射被明禁")
	}

	broken := newDeriveFixture(t)
	broken.mapping.err = errors.New("mapping unreachable")
	stalled, err := broken.handler.Handle(context.Background(), deriveCommand(t, "NODE-INTAKE/a", "v1"))
	if err != nil {
		t.Fatalf("broken handle: %v", err)
	}
	if stalled.Outcome() != application.DeriveUndecided ||
		stalled.UndecidedReason() != application.MappingViewUnavailable {
		t.Fatalf("outcome = %q/%q; 依赖调不通才是未决", stalled.Outcome(), stalled.UndecidedReason())
	}
}
