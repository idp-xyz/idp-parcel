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

// 派生编排不消费按版本读回的口（那是审计与读侧的入口）；替身只存当前版，如实答未找到。
func (double *projectionStoreDouble) FindByVersion(
	_ context.Context,
	_ domain.TenantID,
	_ domain.ProjectionVersionID,
) (domain.TrackingProjection, bool, error) {
	return domain.TrackingProjection{}, false, nil
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

type conflictRuleDouble struct {
	rule       ports.ConflictSignalRule
	configured bool
	err        error
}

func (double *conflictRuleDouble) ConflictSignalRule(_ context.Context) (ports.ConflictSignalRule, bool, error) {
	if double.err != nil {
		return ports.ConflictSignalRule{}, false, double.err
	}
	return double.rule, double.configured, nil
}

type deriveFixture struct {
	handler       *application.DeriveProjectionHandler
	facts         *factStoreDouble
	mapping       *mappingViewDouble
	projections   *projectionStoreDouble
	conflictRules *conflictRuleDouble
	// signals 是真的 RaiseSignalHandler（UC-VE-004 入口）配替身端口：冲突信号进分诊
	// 走的是同一段编排，不另造一个假入口——假入口绿了证明不了信号命令的形状对得上。
	signals    *raiseFixture
	downstream *projectionDownstreamDouble
}

func newDeriveFixture(t *testing.T) *deriveFixture {
	t.Helper()
	signals := newRaiseFixture(t)
	// 冲突信号在这里只求进分诊：分诊规则答人工复核，免得每个分叉用例还要顺带建一个案件。
	signals.triage.answer = ports.TriageAnswer{
		Outcome: domain.ManualReviewRequired,
		Rule:    mustValue(t, domain.NewSignalRuleVersionReference, "triage-rules/v2"),
	}
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
		conflictRules: &conflictRuleDouble{
			rule: ports.ConflictSignalRule{
				Kind:       mustValue(t, domain.NewExceptionSignalKindReference, "FACT_CONFLICT_UNRESOLVED"),
				Rule:       mustValue(t, domain.NewSignalRuleVersionReference, "signal-rules/v3"),
				Confidence: mustValue(t, domain.NewConfidenceReference, "SOURCE_FORK"),
			},
			configured: true,
		},
		signals:    signals,
		downstream: &projectionDownstreamDouble{},
	}
	fixture.handler = application.NewDeriveProjectionHandler(application.DeriveProjectionDeps{
		Facts:         fixture.facts,
		Mapping:       fixture.mapping,
		Projections:   fixture.projections,
		Identities:    &projectionIdentityDouble{},
		ConflictRules: fixture.conflictRules,
		Signals:       signals.handler,
		Downstream:    fixture.downstream,
		Clock:         fixedClock{at: factOccurredAt.Add(2 * time.Hour)},
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
			Kind:        mustValue(t, domain.NewSourceFactKind, "node-intake"),
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

// deriveCommandSuperseding 造一份带来源事实替代关系的命令：源上下文随更正给出前身
// 版本，编排只登记不裁决。
func deriveCommandSuperseding(t *testing.T, factRef, version, supersedes string) application.DeriveProjectionCommand {
	t.Helper()
	command := deriveCommand(t, factRef, version)
	command.Fact.Supersedes = mustValue(t, domain.NewSourceFactVersion, supersedes)
	return command
}

func entryVersions(projection domain.TrackingProjection) map[string]bool {
	versions := map[string]bool{}
	for _, entry := range projection.Entries() {
		versions[entry.Fact().Version().String()] = true
	}
	return versions
}

// Covers: VE CONTEXT「被替代的条目……标准追踪里程碑、各追踪维度和追踪摘要只由当前
// 有效即未被替代的条目派生」与 `AT-VE-044` 的替代半边「更正按源上下文给出的替代关系
// 登记，被替代条目保留且不参与派生」——前身留档在事实库（只增不删），条目里只剩
// 更正后那一代，不同时呈现两个互斥结果。
func TestASupersededFactStaysArchivedButLeavesTheEntries(t *testing.T) {
	fixture := newDeriveFixture(t)
	if _, err := fixture.handler.Handle(context.Background(),
		deriveCommand(t, "TRANSPORT-HANDOVER/a", "handover/v1")); err != nil {
		t.Fatalf("first handle: %v", err)
	}

	corrected, err := fixture.handler.Handle(context.Background(),
		deriveCommandSuperseding(t, "TRANSPORT-HANDOVER/a", "handover/v2", "handover/v1"))
	if err != nil {
		t.Fatalf("corrected handle: %v", err)
	}
	if corrected.Outcome() != application.ProjectionDerived {
		t.Fatalf("outcome = %q", corrected.Outcome())
	}
	projection, _ := corrected.Projection()
	versions := entryVersions(projection)
	if versions["handover/v1"] {
		t.Fatal("被替代的 v1 仍进了条目——两个互斥结果被同时呈现")
	}
	if !versions["handover/v2"] {
		t.Fatalf("entries = %v; 更正后那一代必须在条目里", versions)
	}
	if len(fixture.facts.byKey) != 2 {
		t.Fatalf("事实库 = %d 份, want 2；被替代事实必须留档不删", len(fixture.facts.byKey))
	}
}

// Covers: `AT-VE-043`「含同一前身被两份事实同时指名的替代链分叉→保留双方和冲突关系，
// 不择一」——编排不替源上下文挑后继：两个后继都进条目，前身按已被替代出条目但留档；
// 投影据此把各方摆在场、按信息待确认表达。分叉同时按业务时间裁决：两个后继同刻发生即
// 无法裁决，形成适用异常信号进 `UC-VE-004` 分诊（CONTEXT「冲突仍无法裁决时……投影保持
// 信息待确认并形成适用异常信号」）——信号携带全部保留事实、类型与规则版本取自已登记的
// 冲突信号规则；裁决与信号都不使任何一方失效。
func TestAForkedSupersessionKeepsBothSuccessorsInTheProjection(t *testing.T) {
	fixture := newDeriveFixture(t)
	if _, err := fixture.handler.Handle(context.Background(),
		deriveCommand(t, "TRANSPORT-HANDOVER/a", "handover/v1")); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if _, err := fixture.handler.Handle(context.Background(),
		deriveCommandSuperseding(t, "TRANSPORT-HANDOVER/a", "handover/v2a", "handover/v1")); err != nil {
		t.Fatalf("left successor handle: %v", err)
	}

	forked, err := fixture.handler.Handle(context.Background(),
		deriveCommandSuperseding(t, "TRANSPORT-HANDOVER/a", "handover/v2b", "handover/v1"))
	if err != nil {
		t.Fatalf("right successor handle: %v", err)
	}
	if forked.Outcome() != application.ProjectionDerived {
		t.Fatalf("outcome = %q", forked.Outcome())
	}
	projection, _ := forked.Projection()
	versions := entryVersions(projection)
	if !versions["handover/v2a"] || !versions["handover/v2b"] {
		t.Fatalf("entries = %v; 分叉两方必须都保留，不择一", versions)
	}
	if versions["handover/v1"] {
		t.Fatal("被两方指名的前身仍进了条目")
	}
	if len(fixture.facts.byKey) != 3 {
		t.Fatalf("事实库 = %d 份, want 3；各方与关系必须都留档", len(fixture.facts.byKey))
	}

	conflicts := forked.Conflicts()
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1；同刻的两个后继按业务时间裁不出先后", len(conflicts))
	}
	conflict := conflicts[0]
	if conflict.Judgment().Resolved() || len(conflict.Judgment().Retained()) != 2 {
		t.Fatalf("judgment = %+v; 未裁决的判断必须保留双方", conflict.Judgment())
	}
	signal, present := conflict.Signal()
	if !present || signal.Kind().String() != "FACT_CONFLICT_UNRESOLVED" || signal.Parcel().String() != "parcel-1" {
		t.Fatalf("signal = %+v present = %v; 信号类型取已登记的冲突信号规则", signal, present)
	}
	if len(signal.Facts()) != 2 {
		t.Fatalf("signal facts = %d, want 2；信号携带全部保留事实引用", len(signal.Facts()))
	}
	if conflict.Disposition() != application.ConflictSignalRaised {
		t.Fatalf("disposition = %s, want SIGNAL_RAISED", conflict.Disposition())
	}
	// 信号真的进了分诊：发作期按（租户+对象+类型）开启，规则版本与可信度是登记的那两样。
	episode, found := fixture.signals.episodes.latest[episodeKey{
		tenant: mustValue(t, domain.NewTenantID, "tenant-1"),
		parcel: signal.Parcel(),
		kind:   signal.Kind(),
	}]
	if !found || !episode.Active() {
		t.Fatal("冲突信号没有在分诊侧开启发作期")
	}
	if snapshot := episode.Snapshot(); snapshot.Rule.String() != "signal-rules/v3" || snapshot.Confidence.String() != "SOURCE_FORK" {
		t.Fatalf("episode = %+v; 规则版本与可信度必须来自登记的冲突信号规则", snapshot)
	}
	if !versions["handover/v2a"] || !versions["handover/v2b"] {
		t.Fatal("信号形成后有一方被挤出了投影——信号不使任何源事实失效")
	}
}

// Covers: 裁决半边——两个后继业务时间不同即按业务时间排得出全序（CONTEXT 允许的裁决维度
// 之一），那不是异常：不进结果、不立信号、分诊侧无发作期；投影仍把双方留在场（裁决只给
// 顺序，不使任何一方失效）。
func TestAForkResolvableByBusinessTimeRaisesNoSignal(t *testing.T) {
	fixture := newDeriveFixture(t)
	if _, err := fixture.handler.Handle(context.Background(),
		deriveCommand(t, "TRANSPORT-HANDOVER/a", "handover/v1")); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if _, err := fixture.handler.Handle(context.Background(),
		deriveCommandSuperseding(t, "TRANSPORT-HANDOVER/a", "handover/v2a", "handover/v1")); err != nil {
		t.Fatalf("left successor handle: %v", err)
	}
	later := deriveCommandSuperseding(t, "TRANSPORT-HANDOVER/a", "handover/v2b", "handover/v1")
	later.Fact.OccurredAt = factOccurredAt.Add(30 * time.Minute)
	later.Fact.EffectiveAt = later.Fact.OccurredAt

	resolved, err := fixture.handler.Handle(context.Background(), later)
	if err != nil {
		t.Fatalf("right successor handle: %v", err)
	}
	if len(resolved.Conflicts()) != 0 {
		t.Fatalf("conflicts = %d, want 0；业务时间排得出先后的分叉不是异常", len(resolved.Conflicts()))
	}
	if len(fixture.signals.episodes.latest) != 0 {
		t.Fatal("可裁决的分叉仍进了分诊")
	}
	projection, _ := resolved.Projection()
	if versions := entryVersions(projection); !versions["handover/v2a"] || !versions["handover/v2b"] {
		t.Fatalf("entries = %v; 裁决只给顺序，双方仍留在场", versions)
	}
}

// Covers: 「适用」二字——冲突信号的类型、规则版本与可信度属 `PAR-VIS-04` 待登记实例参数，
// 未配置时如实记「无适用信号规则」：冲突仍进结果（保留双方的判断在），不虚构一种异常类型
// 去分诊，投影照常派生。
func TestAnUnresolvedForkWithoutAConflictSignalRuleIsRecordedNotInvented(t *testing.T) {
	fixture := newDeriveFixture(t)
	fixture.conflictRules.configured = false
	forkTwice(t, fixture)

	forked, err := fixture.handler.Handle(context.Background(),
		deriveCommandSuperseding(t, "TRANSPORT-HANDOVER/a", "handover/v2b", "handover/v1"))
	if err != nil {
		t.Fatalf("right successor handle: %v", err)
	}
	if forked.Outcome() != application.ProjectionDerived {
		t.Fatalf("outcome = %q; 无信号规则不阻断派生", forked.Outcome())
	}
	conflicts := forked.Conflicts()
	if len(conflicts) != 1 || conflicts[0].Disposition() != application.ConflictSignalRuleNotConfigured {
		t.Fatalf("conflicts = %+v, want one SIGNAL_RULE_NOT_CONFIGURED", conflicts)
	}
	if _, present := conflicts[0].Signal(); present {
		t.Fatal("没有规则却形成了信号——类型是编造的")
	}
	if len(fixture.signals.episodes.latest) != 0 {
		t.Fatal("没有规则却有东西进了分诊")
	}
}

// Covers: 搁置半边——分诊那一侧本轮停在未决（发作期库读不回）时，信号已形成但没进去，
// 记为搁置留给重放；派生结果不因此推翻，投影已把双方留在场。
func TestAConflictSignalTheTriageSideCannotTakeIsDeferredNotLost(t *testing.T) {
	fixture := newDeriveFixture(t)
	fixture.signals.episodes.findErr = errors.New("episode store unavailable")
	forkTwice(t, fixture)

	forked, err := fixture.handler.Handle(context.Background(),
		deriveCommandSuperseding(t, "TRANSPORT-HANDOVER/a", "handover/v2b", "handover/v1"))
	if err != nil {
		t.Fatalf("right successor handle: %v", err)
	}
	if forked.Outcome() != application.ProjectionDerived {
		t.Fatalf("outcome = %q", forked.Outcome())
	}
	conflicts := forked.Conflicts()
	if len(conflicts) != 1 || conflicts[0].Disposition() != application.ConflictSignalDeferred {
		t.Fatalf("conflicts = %+v, want one SIGNAL_DEFERRED", conflicts)
	}
	if _, present := conflicts[0].Signal(); !present {
		t.Fatal("搁置的冲突必须仍带着已形成的信号")
	}
}

// forkTwice 落前身与左后继，留右后继给用例自己派生——分叉在第三份到达那一轮才出现。
func forkTwice(t *testing.T, fixture *deriveFixture) {
	t.Helper()
	if _, err := fixture.handler.Handle(context.Background(),
		deriveCommand(t, "TRANSPORT-HANDOVER/a", "handover/v1")); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if _, err := fixture.handler.Handle(context.Background(),
		deriveCommandSuperseding(t, "TRANSPORT-HANDOVER/a", "handover/v2a", "handover/v1")); err != nil {
		t.Fatalf("left successor handle: %v", err)
	}
}

// Covers: 前身维是内容维（与事实类型进指纹同理）——同一来源版本重投带不同前身，
// 说的已是另一份替代关系，形成来源冲突保留原事实，不当重放静默入账。
func TestARedeliveryWithADifferentPredecessorIsAConflict(t *testing.T) {
	fixture := newDeriveFixture(t)
	if _, err := fixture.handler.Handle(context.Background(),
		deriveCommandSuperseding(t, "TRANSPORT-HANDOVER/a", "handover/v2", "handover/v1")); err != nil {
		t.Fatalf("first handle: %v", err)
	}

	conflicting, err := fixture.handler.Handle(context.Background(),
		deriveCommandSuperseding(t, "TRANSPORT-HANDOVER/a", "handover/v2", "handover/v0"))
	if err != nil {
		t.Fatalf("conflicting handle: %v", err)
	}
	if conflicting.Outcome() != application.FactSourceConflict {
		t.Fatalf("outcome = %q, want SOURCE_CONFLICT；换前身不是重放", conflicting.Outcome())
	}

	dropped, err := fixture.handler.Handle(context.Background(),
		deriveCommand(t, "TRANSPORT-HANDOVER/a", "handover/v2"))
	if err != nil {
		t.Fatalf("dropped-predecessor handle: %v", err)
	}
	if dropped.Outcome() != application.FactSourceConflict {
		t.Fatalf("outcome = %q, want SOURCE_CONFLICT；丢前身重投同样不是重放", dropped.Outcome())
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
