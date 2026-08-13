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

var signalHitAt = time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

type episodeKey struct {
	tenant domain.TenantID
	parcel domain.TrackedParcelReference
	kind   domain.ExceptionSignalKindReference
}

type episodeStoreDouble struct {
	latest      map[episodeKey]*domain.SignalEpisode
	conclusions []domain.TriageConclusion
	findErr     error
	raiseErr    error
	hitErr      error
	raisedSaves int
	hitSaves    int
}

func newEpisodeStore() *episodeStoreDouble {
	return &episodeStoreDouble{latest: map[episodeKey]*domain.SignalEpisode{}}
}

func (double *episodeStoreDouble) FindLatest(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
	kind domain.ExceptionSignalKindReference,
) (*domain.SignalEpisode, bool, error) {
	if double.findErr != nil {
		return nil, false, double.findErr
	}
	episode, found := double.latest[episodeKey{tenant: tenant, parcel: parcel, kind: kind}]
	return episode, found, nil
}

func (double *episodeStoreDouble) SaveRaised(_ context.Context, record ports.RaisedSignalRecord) error {
	if double.raiseErr != nil {
		return double.raiseErr
	}
	key := episodeKey{tenant: record.Tenant, parcel: record.Parcel, kind: record.Kind}
	double.latest[key] = record.Episode
	double.conclusions = append(double.conclusions, record.Conclusion)
	double.raisedSaves++
	return nil
}

func (double *episodeStoreDouble) SaveHit(_ context.Context, _ domain.TenantID, episode *domain.SignalEpisode) error {
	if double.hitErr != nil {
		return double.hitErr
	}
	double.hitSaves++
	return nil
}

type triageRuleDouble struct {
	answer     ports.TriageAnswer
	configured bool
	err        error
	calls      int
}

func (double *triageRuleDouble) TriageSignal(
	_ context.Context,
	_ ports.TriageQuery,
) (ports.TriageAnswer, bool, error) {
	double.calls++
	if double.err != nil {
		return ports.TriageAnswer{}, false, double.err
	}
	return double.answer, double.configured, nil
}

type episodeIdentityDouble struct {
	next int
	err  error
}

func (double *episodeIdentityDouble) NextEpisodeID(_ context.Context) (domain.EpisodeID, error) {
	if double.err != nil {
		return domain.EpisodeID{}, double.err
	}
	double.next++
	return domain.NewEpisodeID("episode-" + string(rune('0'+double.next)))
}

type triageDownstreamDouble struct {
	intents []ports.TriageHandoffIntent
	err     error
}

func (double *triageDownstreamDouble) HandOffTriage(
	_ context.Context,
	intent ports.TriageHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type raiseFixture struct {
	handler    *application.RaiseSignalHandler
	episodes   *episodeStoreDouble
	triage     *triageRuleDouble
	identities *episodeIdentityDouble
	downstream *triageDownstreamDouble
}

func newRaiseFixture(t *testing.T) *raiseFixture {
	t.Helper()
	fixture := &raiseFixture{
		episodes: newEpisodeStore(),
		triage: &triageRuleDouble{
			answer: ports.TriageAnswer{
				Outcome: domain.AutoEstablishCase,
				Rule:    mustValue(t, domain.NewSignalRuleVersionReference, "triage-rules/v2"),
			},
			configured: true,
		},
		identities: &episodeIdentityDouble{},
		downstream: &triageDownstreamDouble{},
	}
	fixture.handler = application.NewRaiseSignalHandler(application.RaiseSignalDeps{
		Episodes:   fixture.episodes,
		Triage:     fixture.triage,
		Identities: fixture.identities,
		Downstream: fixture.downstream,
		Clock:      fixedClock{at: signalHitAt.Add(time.Minute)},
	})
	return fixture
}

func raiseCommand(t *testing.T, hitAt time.Time) application.RaiseSignalCommand {
	t.Helper()
	return application.RaiseSignalCommand{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Kind:       mustValue(t, domain.NewExceptionSignalKindReference, "ETA_BREACH_RISK"),
		Parcel:     mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Rule:       mustValue(t, domain.NewSignalRuleVersionReference, "signal-rules/v3"),
		Confidence: mustValue(t, domain.NewConfidenceReference, "HIGH/route-deviation"),
		HitAt:      hitAt,
	}
}

// Covers: CONTEXT 生命周期「规则或事实首次满足条件 → 建立信号发作期，保存规则版本、
// 范围、依据和可信度」与「每个信号必须保存对象、类型、规则版本、判断时间、事实依据、
// 可信度」——首启建发作期、分诊结论与发作期同一提交、意图由发作期标识认领。点名
// `AT-VE-062` 的分诊半边「高可信高影响命中自动规则→建案」——命中版本化规则形成
// 自动建案走向，案件本体由结论的消费方建立。
func TestAFirstHitOpensAnEpisodeAndConcludesTriage(t *testing.T) {
	fixture := newRaiseFixture(t)

	result, err := fixture.handler.Handle(context.Background(), raiseCommand(t, signalHitAt))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.SignalEpisodeOpened {
		t.Fatalf("outcome = %q, want EPISODE_OPENED", result.Outcome())
	}
	episode, present := result.Episode()
	if !present || episode.Hits() != 1 || !episode.Active() {
		t.Fatalf("episode = %+v; a first hit must open an active episode with one hit", episode)
	}
	if _, linked := episode.PriorEpisode(); linked {
		t.Fatal("a first episode claims to link back to a prior one")
	}
	conclusion, concluded := result.TriageConclusion()
	if !concluded || conclusion.Outcome() != domain.AutoEstablishCase {
		t.Fatalf("conclusion = %+v; the configured rule hit must carry AUTO_ESTABLISH", conclusion)
	}
	if conclusion.Rule().String() != "triage-rules/v2" {
		t.Fatalf("conclusion rule = %q, want the versioned triage rule that hit", conclusion.Rule())
	}
	if fixture.episodes.raisedSaves != 1 || len(fixture.episodes.conclusions) != 1 {
		t.Fatal("the episode and its triage conclusion must cross the commit boundary together")
	}
	if len(fixture.downstream.intents) != 1 ||
		fixture.downstream.intents[0].Conclusion.Episode() != episode.ID() {
		t.Fatal("exactly one handoff intent claimed by the raised episode was expected")
	}
}

// Covers: CONTEXT「同一对象、类型、因果条件和连续影响期内的重复命中更新同一信号发作期」
// 与生命周期「重复命中 → 更新判断历史……不建立重复发作期」——命中不签新身份、不重新
// 分诊、不产生第二份结论。点名 `AT-VE-064`「同一连续期重复命中→更新同一发作期，
// 不重复建案或重置时钟」。
func TestARepeatHitOnAnActiveEpisodeUpdatesItWithoutASecondEpisodeOrTriage(t *testing.T) {
	fixture := newRaiseFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, raiseCommand(t, signalHitAt))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	firstEpisode, _ := first.Episode()
	issuedBefore, triageBefore := fixture.identities.next, fixture.triage.calls

	repeat, err := fixture.handler.Handle(ctx, raiseCommand(t, signalHitAt.Add(time.Hour)))
	if err != nil {
		t.Fatalf("repeat handle: %v", err)
	}

	if repeat.Outcome() != application.SignalHitRecorded {
		t.Fatalf("outcome = %q, want HIT_RECORDED", repeat.Outcome())
	}
	episode, _ := repeat.Episode()
	if episode.ID() != firstEpisode.ID() {
		t.Fatal("a repeat hit within the same impact period opened a second episode")
	}
	if episode.Hits() != 2 {
		t.Fatalf("hits = %d, want the judgment history updated to 2", episode.Hits())
	}
	if _, concluded := repeat.TriageConclusion(); concluded {
		t.Fatal("a repeat hit within the same episode re-triaged the signal")
	}
	if fixture.identities.next != issuedBefore {
		t.Fatal("a repeat hit consumed a new episode identity")
	}
	if fixture.triage.calls != triageBefore {
		t.Fatal("a repeat hit asked the triage rules again")
	}
	if fixture.episodes.hitSaves != 1 || fixture.episodes.raisedSaves != 1 {
		t.Fatal("a repeat hit must be saved as a hit, not as a second raised episode")
	}
	if len(fixture.downstream.intents) != 1 {
		t.Fatal("a repeat hit handed off a second triage intent")
	}
}

// Covers: CONTEXT「条件明确解除后，该发作期结束；再次发生时建立关联的新发作期」与生命
// 周期「恢复后再次满足条件 → 建立关联的新发作期；是否建立或重开案件重新经过分诊规则」
// ——重开新期指回前期、重新分诊、旧期保持已结束。点名 `AT-VE-065`「恢复后再次发生→
// 建立关联新发作期，重新分诊」。
func TestAHitAfterRecoveryReopensALinkedEpisodeAndRetriages(t *testing.T) {
	fixture := newRaiseFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, raiseCommand(t, signalHitAt))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	firstEpisode, _ := first.Episode()
	if err := firstEpisode.End("CONDITION_CLEARED", signalHitAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("end first episode: %v", err)
	}
	triageBefore := fixture.triage.calls

	reopened, err := fixture.handler.Handle(ctx, raiseCommand(t, signalHitAt.Add(3*time.Hour)))
	if err != nil {
		t.Fatalf("reopen handle: %v", err)
	}

	if reopened.Outcome() != application.SignalEpisodeReopened {
		t.Fatalf("outcome = %q, want EPISODE_REOPENED", reopened.Outcome())
	}
	episode, _ := reopened.Episode()
	if episode.ID() == firstEpisode.ID() {
		t.Fatal("the reopened episode reused the ended episode's identity")
	}
	prior, linked := episode.PriorEpisode()
	if !linked || prior != firstEpisode.ID() {
		t.Fatalf("prior episode = %q, want a link back to %q", prior, firstEpisode.ID())
	}
	if firstEpisode.Active() {
		t.Fatal("reopening resurrected the ended episode instead of linking a new one")
	}
	if fixture.triage.calls != triageBefore+1 {
		t.Fatal("reopening must go through the triage rules again")
	}
	conclusion, concluded := reopened.TriageConclusion()
	if !concluded || conclusion.Episode() != episode.ID() {
		t.Fatal("the reopened episode carries no triage conclusion of its own")
	}
}

// Covers: 派工约束「未配置 → 人工复核格 ManualReviewRequired 如实不自动建案，不是未决」
// 与 CONTEXT「高可信、高影响且命中版本化分诊规则的信号可以自动建立或关联案件」的反面
// ——没命中版本化规则就没有自动建案，空白不是放行也不是故障。
func TestUnconfiguredTriageRulesFallToManualReviewInsteadOfAutoCase(t *testing.T) {
	fixture := newRaiseFixture(t)
	fixture.triage.configured = false

	result, err := fixture.handler.Handle(context.Background(), raiseCommand(t, signalHitAt))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.SignalEpisodeOpened {
		t.Fatalf("outcome = %q, want EPISODE_OPENED; unconfigured rules must not block the episode", result.Outcome())
	}
	conclusion, concluded := result.TriageConclusion()
	if !concluded || conclusion.Outcome() != domain.ManualReviewRequired {
		t.Fatalf("conclusion = %+v, want MANUAL_REVIEW when no triage rule is registered", conclusion)
	}
	if conclusion.Rule().String() != "TRIAGE_RULES_NOT_CONFIGURED" {
		t.Fatalf("conclusion rule = %q; the unconfigured slot must stay distinguishable from a real rule", conclusion.Rule())
	}
}

// Covers: 未决语义——规则视图调不通是依赖故障不是「规则未配置」，形成未决且不消耗
// 发作期标识、不写库；混起来会让一次故障被读成租户还没登记分诊规则。
func TestAnUnavailableTriageRuleViewIsUndecidedWithoutConsumingAnIdentity(t *testing.T) {
	fixture := newRaiseFixture(t)
	fixture.triage.err = errors.New("triage rule view unavailable")

	result, err := fixture.handler.Handle(context.Background(), raiseCommand(t, signalHitAt))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.RaiseSignalUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.UndecidedReason() != application.TriageRuleUnavailable {
		t.Fatalf("reason = %q, want TRIAGE_RULE_UNAVAILABLE", result.UndecidedReason())
	}
	if fixture.identities.next != 0 {
		t.Fatal("an undecided round consumed a scarce episode identity")
	}
	if fixture.episodes.raisedSaves != 0 || fixture.episodes.hitSaves != 0 {
		t.Fatal("an undecided round wrote the episode store")
	}
}

// Covers: 同一条未决语义的存储半边——发作期库读不回时停在未决，不问分诊也不虚构首启。
func TestAnUnreadableEpisodeStoreIsUndecidedBeforeTriage(t *testing.T) {
	fixture := newRaiseFixture(t)
	fixture.episodes.findErr = errors.New("episode store unavailable")

	result, err := fixture.handler.Handle(context.Background(), raiseCommand(t, signalHitAt))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.RaiseSignalUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.UndecidedReason() != application.SignalEpisodeStoreUnavailable {
		t.Fatalf("reason = %q, want SIGNAL_EPISODE_STORE_UNAVAILABLE", result.UndecidedReason())
	}
	if fixture.triage.calls != 0 {
		t.Fatal("the round asked the triage rules before it knew the episode state")
	}
}

// Covers: ADR-0043「首次交付失败不改写业务结果……另留一条发布续办引用」——意图交不
// 出去时发作期与结论保持已成立，只留可续办引用。
func TestAFailedTriageHandoffKeepsTheRaisedEpisodeWithAResumableReference(t *testing.T) {
	fixture := newRaiseFixture(t)
	fixture.downstream.err = errors.New("downstream unavailable")

	result, err := fixture.handler.Handle(context.Background(), raiseCommand(t, signalHitAt))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.SignalEpisodeOpened {
		t.Fatalf("outcome = %q, want EPISODE_OPENED; a failed handoff must not rewrite the episode", result.Outcome())
	}
	if result.HandoffReference() == "" {
		t.Fatal("a failed handoff left no resumable reference")
	}
	if fixture.episodes.raisedSaves != 1 {
		t.Fatal("the raised episode must stay committed")
	}
}

// Covers: 受理半边「规则版本+可信度必备——来自命中事实不自造」——缺任何一样即未受理，
// 不读依赖、不补默认值。
func TestACommandWithoutItsSignalFactsIsNotAccepted(t *testing.T) {
	fixture := newRaiseFixture(t)

	missingRule := raiseCommand(t, signalHitAt)
	missingRule.Rule = domain.SignalRuleVersionReference{}
	result, err := fixture.handler.Handle(context.Background(), missingRule)
	if err != nil {
		t.Fatalf("handle without rule: %v", err)
	}
	if result.Outcome() != application.RaiseSignalNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED", result.Outcome())
	}

	missingHitAt := raiseCommand(t, signalHitAt)
	missingHitAt.HitAt = time.Time{}
	noTime, err := fixture.handler.Handle(context.Background(), missingHitAt)
	if err != nil {
		t.Fatalf("handle without hit time: %v", err)
	}
	if noTime.Outcome() != application.RaiseSignalNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED", noTime.Outcome())
	}

	missingTenant := raiseCommand(t, signalHitAt)
	missingTenant.TenantID = domain.TenantID{}
	noTenant, err := fixture.handler.Handle(context.Background(), missingTenant)
	if err != nil {
		t.Fatalf("handle without tenant: %v", err)
	}
	if noTenant.Outcome() != application.RaiseSignalNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED；租户是隔离边界不是可补的默认值", noTenant.Outcome())
	}
	if fixture.triage.calls != 0 || fixture.episodes.raisedSaves != 0 {
		t.Fatal("an unaccepted command still reached a dependency")
	}
}

// Covers: 翻译全函数纪律——分诊端口交回封闭四走向以外的答复时上抛，不静默归入某一格。
func TestATriageAnswerOutsideTheClosedSetIsRaisedNotTranslated(t *testing.T) {
	fixture := newRaiseFixture(t)
	fixture.triage.answer = ports.TriageAnswer{Outcome: domain.TriageOutcome(99)}

	_, err := fixture.handler.Handle(context.Background(), raiseCommand(t, signalHitAt))
	if err == nil {
		t.Fatal("an out-of-set triage outcome was swallowed instead of raised")
	}
	if !errors.Is(err, application.ErrUnexpectedTriageOutcome) {
		t.Fatalf("err = %v, want ErrUnexpectedTriageOutcome", err)
	}
	if fixture.episodes.raisedSaves != 0 {
		t.Fatal("a defective triage answer still raised an episode")
	}
}
