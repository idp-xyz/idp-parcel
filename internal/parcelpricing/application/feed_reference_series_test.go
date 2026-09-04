package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outbound"
)

// 本文件证来源喂价编排（ADR-0099 决定六；票 06）：抓取 → 存放 → 转录 → 走同一登记用例 → 免复核
// 声明为「是」时由连接器身份代写复核、依据为绑定版本；「未声明」与「否」都不代写；同一工件重跑
// 答重放且不再写复核；抓取失败与转录被拒不登、各出恰一条可观察记录；登记册与复核册的治理答案
// 原样交回；本体存放未配置照登且凭证仍 VERIFIABLE。全部替身，SYN 夹具（S 级）。

var (
	feedClockMoment = time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)
	feedDayOne      = time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
)

type feedClock struct{ now time.Time }

func (clock feedClock) Now() time.Time { return clock.now }

type bindingLoaderDouble struct {
	binding domain.SourceConnectorBinding
	found   bool
	err     error
}

func (double bindingLoaderDouble) LoadCurrentBinding(context.Context, domain.TenantID, string) (domain.SourceConnectorBinding, bool, error) {
	return double.binding, double.found, double.err
}

// connectorDouble 是一个连接器替身：Fetch 交回预置的记录或错误；Transcribe 把预置观测交给领域的
// 整版重述——它只做真连接器会做的那一件事（读观测），其余不重写。
type connectorDouble struct {
	kind          string
	record        domain.PublishedRecord
	fetchErr      error
	observation   domain.SeriesObservation
	transcribeErr error
	fetched       []ports.FetchSpec
	transcribed   []ports.TranscriptionInput
}

func (double *connectorDouble) Kind() string { return double.kind }

func (double *connectorDouble) Fetch(_ context.Context, spec ports.FetchSpec) (domain.PublishedRecord, error) {
	double.fetched = append(double.fetched, spec)
	if double.fetchErr != nil {
		return domain.PublishedRecord{}, double.fetchErr
	}
	return double.record, nil
}

func (double *connectorDouble) Transcribe(input ports.TranscriptionInput) (domain.ReferenceSeriesRegistrationSpec, error) {
	double.transcribed = append(double.transcribed, input)
	if double.transcribeErr != nil {
		return domain.ReferenceSeriesRegistrationSpec{}, double.transcribeErr
	}
	storage, _ := input.Placement.StoredLocator()
	return domain.TranscribeSourceFeed(domain.SourceFeedTranscription{
		Binding:        input.Binding,
		Record:         input.Record,
		StorageLocator: storage,
		Prior:          input.Prior,
		Observation:    double.observation,
	})
}

type resolverDouble struct {
	connectors map[string]ports.SourceConnector
}

func (double resolverDouble) ConnectorFor(kind string) (ports.SourceConnector, bool) {
	connector, found := double.connectors[kind]
	return connector, found
}

type artifactStoreDouble struct {
	placement ports.ArtifactPlacement
	err       error
	stored    int
}

func (double *artifactStoreDouble) Store(context.Context, domain.PublishedRecord) (ports.ArtifactPlacement, error) {
	double.stored++
	return double.placement, double.err
}

type latestLoaderDouble struct {
	registration domain.ReferenceSeriesRegistration
	found        bool
	err          error
}

func (double latestLoaderDouble) LoadLatestVersion(context.Context, domain.TenantID, string) (domain.ReferenceSeriesRegistration, bool, error) {
	return double.registration, double.found, double.err
}

// feedSeriesRegisterDouble 同时扮登记册写口与按版本读口：复核用例要按（序列、版本）读回刚登的那一版。
type feedSeriesRegisterDouble struct {
	outcome    ports.ReferenceSeriesRegistrationOutcome
	err        error
	registered []domain.ReferenceSeriesRegistration
}

func (double *feedSeriesRegisterDouble) Register(_ context.Context, registration domain.ReferenceSeriesRegistration) (ports.ReferenceSeriesRegistrationOutcome, error) {
	if double.err != nil {
		return ports.ReferenceSeriesRegistrationOutcomeInvalid, double.err
	}
	double.registered = append(double.registered, registration)
	return double.outcome, nil
}

func (double *feedSeriesRegisterDouble) ResolveAt(context.Context, domain.TenantID, domain.VersionReference, time.Time) (domain.ResolvedSeriesReading, bool, error) {
	return domain.ResolvedSeriesReading{}, false, nil
}

func (double *feedSeriesRegisterDouble) LoadVersion(_ context.Context, tenant domain.TenantID, seriesID, seriesVersion string) (domain.ReferenceSeriesRegistration, bool, error) {
	for _, registration := range double.registered {
		if registration.Tenant() == tenant && registration.Reference().ID() == seriesID && registration.Reference().Version() == seriesVersion {
			return registration, true, nil
		}
	}
	return domain.ReferenceSeriesRegistration{}, false, nil
}

type feedReviewRegisterDouble struct {
	outcome  ports.ReferenceSeriesReviewOutcome
	err      error
	recorded []domain.SeriesReview
}

func (double *feedReviewRegisterDouble) Record(_ context.Context, review domain.SeriesReview) (ports.ReferenceSeriesReviewOutcome, error) {
	if double.err != nil {
		return ports.ReferenceSeriesReviewOutcomeInvalid, double.err
	}
	double.recorded = append(double.recorded, review)
	return double.outcome, nil
}

type feedFixture struct {
	binding      domain.SourceConnectorBinding
	connector    *connectorDouble
	store        *artifactStoreDouble
	latest       latestLoaderDouble
	register     *feedSeriesRegisterDouble
	reviews      *feedReviewRegisterDouble
	observations []ports.SourceFeedObservation
	bindings     bindingLoaderDouble
	resolver     resolverDouble
}

func feedBinding(t *testing.T, exemption domain.ReviewExemption, registrant string) domain.SourceConnectorBinding {
	t.Helper()
	policy, err := domain.NewVersionReference(domain.ArtifactCommercialPolicy, "SYN-PRC-FX-POLICY", "v1", "sha256:syn-fx-policy")
	if err != nil {
		t.Fatalf("构造口径引用：%v", err)
	}
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("构造租户：%v", err)
	}
	binding, err := domain.NewSourceConnectorBinding(domain.SourceConnectorBindingSpec{
		Tenant:           tenant,
		SeriesID:         "SYN-PRC-USD-CNY",
		Version:          "b1",
		ConnectorKind:    "FILE",
		SourceIdentifier: "SYN-SOURCE/usd-cny-daily",
		SourceLocator:    "rates/usd-cny/2026-09-04.json",
		SeriesKind:       domain.ReferenceSeriesExchangeRate,
		QuoteBasis:       policy,
		Registrant:       registrant,
		ReviewExemption:  exemption,
	})
	if err != nil {
		t.Fatalf("构造绑定：%v", err)
	}
	return binding
}

func newFeedFixture(t *testing.T, exemption domain.ReviewExemption) *feedFixture {
	t.Helper()
	binding := feedBinding(t, exemption, "SYN-PRC-SERIES-REGISTRAR")
	record, err := domain.NewPublishedRecord([]byte(`{"value":"7.1234"}`), feedClockMoment, "file:rates/usd-cny/2026-09-04.json", feedDayOne)
	if err != nil {
		t.Fatalf("构造记录：%v", err)
	}
	value, err := domain.ParseDecimal("7.1234")
	if err != nil {
		t.Fatalf("构造取值：%v", err)
	}
	observed, err := domain.NewSeriesObservation(feedDayOne, value)
	if err != nil {
		t.Fatalf("构造观测：%v", err)
	}
	connector := &connectorDouble{kind: "FILE", record: record, observation: observed}
	fixture := &feedFixture{
		binding:   binding,
		connector: connector,
		store:     &artifactStoreDouble{placement: ports.ArtifactPlacement{Outcome: outbound.Accept(), Locator: record.SourceLocator()}},
		register:  &feedSeriesRegisterDouble{outcome: ports.ReferenceSeriesRegistered},
		reviews:   &feedReviewRegisterDouble{outcome: ports.ReferenceSeriesReviewRecorded},
		bindings:  bindingLoaderDouble{binding: binding, found: true},
		resolver:  resolverDouble{connectors: map[string]ports.SourceConnector{"FILE": connector}},
	}
	return fixture
}

func (fixture *feedFixture) handler() *application.FeedReferenceSeriesHandler {
	return application.NewFeedReferenceSeriesHandler(application.FeedReferenceSeriesDeps{
		Bindings:   fixture.bindings,
		Connectors: fixture.resolver,
		Artifacts:  fixture.store,
		Latest:     fixture.latest,
		Register:   fixture.register,
		Versions:   fixture.register,
		Reviews:    fixture.reviews,
		Clock:      feedClock{now: feedClockMoment},
		Observer: func(observation ports.SourceFeedObservation) {
			fixture.observations = append(fixture.observations, observation)
		},
	})
}

func (fixture *feedFixture) feed(t *testing.T) application.FeedReferenceSeriesResult {
	t.Helper()
	result, err := fixture.handler().Handle(t.Context(), application.FeedReferenceSeriesCommand{
		Tenant: fixture.binding.Tenant(), SeriesID: fixture.binding.SeriesID(),
	})
	if err != nil {
		t.Fatalf("喂价：%v", err)
	}
	return result
}

// TestFeedRegistersAndWritesTheExemptionReview 证绿路径：抓取按绑定定位符、本体交存放端口、登记
// 走同一登记用例（引用版本 = 公布日期、digest = 工件摘要、整版 VERIFIABLE）；声明免复核时复核由
// 连接器身份代写、结论通过、依据指回绑定版本、时刻取时钟当下；全程零可观察记录。
func TestFeedRegistersAndWritesTheExemptionReview(t *testing.T) {
	fixture := newFeedFixture(t, domain.ReviewExemptionGranted)

	result := fixture.feed(t)

	if result.Outcome != application.FeedVersionRegistered || result.Registration != application.ReferenceSeriesRecorded {
		t.Fatalf("outcome = %s registration = %s，想要 REGISTERED / RECORDED", result.Outcome, result.Registration)
	}
	if len(fixture.connector.fetched) != 1 || fixture.connector.fetched[0].Locator != "rates/usd-cny/2026-09-04.json" {
		t.Fatalf("抓取规格走样：%+v", fixture.connector.fetched)
	}
	if fixture.store.stored != 1 {
		t.Fatalf("本体存放调用 %d 次", fixture.store.stored)
	}
	if len(fixture.register.registered) != 1 {
		t.Fatalf("登记 %d 次", len(fixture.register.registered))
	}
	registered := fixture.register.registered[0]
	if registered.Reference().Version() != "2026-09-04" || registered.Reference().Digest() != fixture.connector.record.ContentDigest() ||
		registered.EvidenceGrade() != domain.SeriesEvidenceVerifiable || result.Reference != registered.Reference() {
		t.Fatalf("登记走样：%+v result=%+v", registered.Reference(), result.Reference)
	}
	if !result.ReviewRequested || result.Review != application.SeriesReviewRecorded || len(fixture.reviews.recorded) != 1 {
		t.Fatalf("免复核声明下复核没写：requested=%v review=%s recorded=%d", result.ReviewRequested, result.Review, len(fixture.reviews.recorded))
	}
	review := fixture.reviews.recorded[0]
	if review.Reviewer() != "connector:FILE" || review.Decision() != domain.SeriesReviewApproved ||
		review.Basis() != fixture.binding.ExemptionReviewBasis() || !review.ReviewedAt().Equal(feedClockMoment) ||
		review.Reference() != registered.Reference() {
		t.Fatalf("代写的复核走样：reviewer=%s decision=%s basis=%q at=%v", review.Reviewer(), review.Decision(), review.Basis(), review.ReviewedAt())
	}
	if len(fixture.observations) != 0 {
		t.Fatalf("绿路径出了可观察记录：%+v", fixture.observations)
	}
}

// TestFeedLeavesReviewToHumansUnlessExempt 证「未声明 = 需人工复核」与「否」都不代写复核：版本照登，
// 进不进在用留给人。
func TestFeedLeavesReviewToHumansUnlessExempt(t *testing.T) {
	for _, exemption := range []domain.ReviewExemption{domain.ReviewExemptionUndeclared, domain.ReviewExemptionWithheld} {
		fixture := newFeedFixture(t, exemption)
		result := fixture.feed(t)
		if result.Outcome != application.FeedVersionRegistered || len(fixture.register.registered) != 1 {
			t.Fatalf("%s：outcome = %s registered=%d", exemption, result.Outcome, len(fixture.register.registered))
		}
		if result.ReviewRequested || len(fixture.reviews.recorded) != 0 {
			t.Fatalf("%s：系统替人复核了", exemption)
		}
	}
}

// TestFeedReplayOfTheSameArtifactDoesNotReviewAgain 证同一工件重跑：前一版就是它、登记册答幂等
// 重放、编排答 REPLAYED 且不再代写复核——第一次那条复核已经在册。
func TestFeedReplayOfTheSameArtifactDoesNotReviewAgain(t *testing.T) {
	fixture := newFeedFixture(t, domain.ReviewExemptionGranted)
	first := fixture.feed(t)
	prior := fixture.register.registered[0]

	fixture.latest = latestLoaderDouble{registration: prior, found: true}
	fixture.register.outcome = ports.ReferenceSeriesAlreadyRegistered
	second := fixture.feed(t)

	if second.Outcome != application.FeedVersionReplayed || second.Registration != application.ReferenceSeriesAlreadyOnRegister || second.Reference != first.Reference {
		t.Fatalf("重跑 outcome = %s registration = %s", second.Outcome, second.Registration)
	}
	if second.ReviewRequested || len(fixture.reviews.recorded) != 1 {
		t.Fatalf("重跑又写了复核：requested=%v recorded=%d", second.ReviewRequested, len(fixture.reviews.recorded))
	}
	if len(fixture.observations) != 0 {
		t.Fatalf("重跑出了可观察记录：%+v", fixture.observations)
	}
}

// TestFeedFetchFailureLeavesAGapAndOneObservation 证抓取失败：不登、不复核、不存放，恰一条可观察
// 记录停在 FETCH 站，原始错误原样在。
func TestFeedFetchFailureLeavesAGapAndOneObservation(t *testing.T) {
	fixture := newFeedFixture(t, domain.ReviewExemptionGranted)
	boom := errors.New("file not found")
	fixture.connector.fetchErr = boom

	result := fixture.feed(t)

	if result.Outcome != application.FeedFetchFailed {
		t.Fatalf("outcome = %s，想要 FETCH_FAILED", result.Outcome)
	}
	if fixture.store.stored != 0 || len(fixture.register.registered) != 0 || len(fixture.reviews.recorded) != 0 {
		t.Fatal("抓取失败后仍有东西被写")
	}
	if len(fixture.observations) != 1 {
		t.Fatalf("可观察记录 %d 条，想要恰一条", len(fixture.observations))
	}
	observation := fixture.observations[0]
	if observation.Stage != ports.SourceFeedStageFetch || !errors.Is(observation.Err, boom) ||
		observation.SeriesID != "SYN-PRC-USD-CNY" || observation.BindingVersion != "b1" || observation.ConnectorKind != "FILE" ||
		observation.Locator != "rates/usd-cny/2026-09-04.json" || observation.Tenant != fixture.binding.Tenant() {
		t.Fatalf("可观察记录走样：%+v", observation)
	}
}

// TestFeedTranscriptionRefusalIsObserved 证转录被拒（如公布顺序倒置）：不登，恰一条可观察记录停在
// TRANSCRIBE 站。
func TestFeedTranscriptionRefusalIsObserved(t *testing.T) {
	fixture := newFeedFixture(t, domain.ReviewExemptionGranted)
	fixture.connector.transcribeErr = domain.ErrSourceFeedOutOfOrder

	result := fixture.feed(t)

	if result.Outcome != application.FeedTranscriptionRefused || len(fixture.register.registered) != 0 {
		t.Fatalf("outcome = %s registered=%d", result.Outcome, len(fixture.register.registered))
	}
	if len(fixture.observations) != 1 || fixture.observations[0].Stage != ports.SourceFeedStageTranscribe ||
		!errors.Is(fixture.observations[0].Err, domain.ErrSourceFeedOutOfOrder) {
		t.Fatalf("可观察记录走样：%+v", fixture.observations)
	}
}

// TestFeedAnswersBindingUnknownAndConnectorUnavailable 证两格治理答案：没有绑定先去登记绑定；绑定
// 声明了本部署没装配的连接器种类（CFETS 段留 draft 期间正是如此）答连接器不可用。两格都不抓不登。
func TestFeedAnswersBindingUnknownAndConnectorUnavailable(t *testing.T) {
	fixture := newFeedFixture(t, domain.ReviewExemptionGranted)
	fixture.bindings = bindingLoaderDouble{}
	if result := fixture.feed(t); result.Outcome != application.FeedBindingUnknown {
		t.Fatalf("无绑定 outcome = %s", result.Outcome)
	}

	fixture = newFeedFixture(t, domain.ReviewExemptionGranted)
	fixture.resolver = resolverDouble{connectors: map[string]ports.SourceConnector{}}
	if result := fixture.feed(t); result.Outcome != application.FeedConnectorUnavailable {
		t.Fatalf("无连接器 outcome = %s", result.Outcome)
	}
	if len(fixture.connector.fetched) != 0 || len(fixture.register.registered) != 0 {
		t.Fatal("连接器不可用仍抓了或登了")
	}
}

// TestFeedRegistrationConflictIsAGovernanceAnswer 证登记册的治理答案原样交回：同版本内容冲突不算
// 成功也不算故障，不代写复核，出一条 REGISTER 站的可观察记录带答案字面。
func TestFeedRegistrationConflictIsAGovernanceAnswer(t *testing.T) {
	fixture := newFeedFixture(t, domain.ReviewExemptionGranted)
	fixture.register.outcome = ports.ReferenceSeriesContentConflict

	result := fixture.feed(t)

	if result.Outcome != application.FeedRegistrationRefused || result.Registration != application.ReferenceSeriesRegistrationConflict {
		t.Fatalf("outcome = %s registration = %s", result.Outcome, result.Registration)
	}
	if result.ReviewRequested || len(fixture.reviews.recorded) != 0 {
		t.Fatal("冲突之后仍代写了复核")
	}
	if len(fixture.observations) != 1 || fixture.observations[0].Stage != ports.SourceFeedStageRegister ||
		fixture.observations[0].Answer != "CONTENT_CONFLICT" || fixture.observations[0].Err != nil {
		t.Fatalf("可观察记录走样：%+v", fixture.observations)
	}
}

// TestFeedUnconfiguredStorageStillRegistersVerifiable 证本体存放未配置不是丢弃：照登，凭证不带定位符
// 而等级仍 VERIFIABLE，不出可观察记录；存放答别的失败格时同样照登，但记一条 STORE 站的观察。
func TestFeedUnconfiguredStorageStillRegistersVerifiable(t *testing.T) {
	fixture := newFeedFixture(t, domain.ReviewExemptionUndeclared)
	fixture.store.placement = ports.ArtifactPlacement{Outcome: outbound.Unconfigured()}

	result := fixture.feed(t)
	if result.Outcome != application.FeedVersionRegistered || len(fixture.observations) != 0 {
		t.Fatalf("未配置存放：outcome = %s observations=%d", result.Outcome, len(fixture.observations))
	}
	registered := fixture.register.registered[0]
	if evidence, _ := registered.Periods()[0].Evidence(); evidence != fixture.connector.record.EvidenceReference("") {
		t.Fatalf("未配置存放的凭证带了定位符：%q", evidence)
	}
	if registered.EvidenceGrade() != domain.SeriesEvidenceVerifiable {
		t.Fatal("凭证等级不该依赖本体在不在")
	}

	fixture = newFeedFixture(t, domain.ReviewExemptionUndeclared)
	fixture.store.placement = ports.ArtifactPlacement{Outcome: outbound.Undetermined()}
	result = fixture.feed(t)
	if result.Outcome != application.FeedVersionRegistered || len(fixture.observations) != 1 ||
		fixture.observations[0].Stage != ports.SourceFeedStageStore || fixture.observations[0].Answer != "ANSWER_UNDETERMINED" {
		t.Fatalf("存放答未确定：outcome = %s observations=%+v", result.Outcome, fixture.observations)
	}
}

// TestFeedReviewRefusalIsObservedButTheRegistrationStands 证四眼门在结构上仍关得住：租户把登记责任方
// 填成连接器身份，复核用例拒（NEEDS_ANOTHER_REVIEWER）；版本照登、答案原样交回、一条 REVIEW 站的
// 可观察记录。
func TestFeedReviewRefusalIsObservedButTheRegistrationStands(t *testing.T) {
	fixture := newFeedFixture(t, domain.ReviewExemptionGranted)
	fixture.binding = feedBinding(t, domain.ReviewExemptionGranted, "connector:FILE")
	fixture.bindings = bindingLoaderDouble{binding: fixture.binding, found: true}

	result := fixture.feed(t)

	if result.Outcome != application.FeedVersionRegistered || !result.ReviewRequested || result.Review != application.SeriesReviewNeedsAnotherReviewer {
		t.Fatalf("outcome = %s requested=%v review = %s", result.Outcome, result.ReviewRequested, result.Review)
	}
	if len(fixture.reviews.recorded) != 0 {
		t.Fatal("四眼门没关住")
	}
	if len(fixture.observations) != 1 || fixture.observations[0].Stage != ports.SourceFeedStageReview ||
		fixture.observations[0].Answer != "NEEDS_ANOTHER_REVIEWER" {
		t.Fatalf("可观察记录走样：%+v", fixture.observations)
	}
}

// TestFeedDependencyFailuresAreUndecided 证依赖故障不吞：登记册、绑定读口、前一版读口、存放端口
// 的技术错误都译成 UNDECIDED 并把原因随错误交回。
func TestFeedDependencyFailuresAreUndecided(t *testing.T) {
	boom := errors.New("store is down")
	mutations := map[string]func(*feedFixture){
		"登记册":   func(fixture *feedFixture) { fixture.register.err = boom },
		"绑定读口":  func(fixture *feedFixture) { fixture.bindings = bindingLoaderDouble{err: boom} },
		"前一版读口": func(fixture *feedFixture) { fixture.latest = latestLoaderDouble{err: boom} },
		"存放端口":  func(fixture *feedFixture) { fixture.store.err = boom },
		"复核册":   func(fixture *feedFixture) { fixture.reviews.err = boom },
	}
	for label, mutate := range mutations {
		fixture := newFeedFixture(t, domain.ReviewExemptionGranted)
		mutate(fixture)
		result, err := fixture.handler().Handle(t.Context(), application.FeedReferenceSeriesCommand{
			Tenant: fixture.binding.Tenant(), SeriesID: fixture.binding.SeriesID(),
		})
		if result.Outcome != application.FeedUndecided || !errors.Is(err, boom) {
			t.Fatalf("%s：outcome = %s err = %v", label, result.Outcome, err)
		}
	}
}

// TestFeedNotAcceptedWithoutTenantOrSeries 证受理门：缺租户或序列的命令不到达任何依赖。
func TestFeedNotAcceptedWithoutTenantOrSeries(t *testing.T) {
	fixture := newFeedFixture(t, domain.ReviewExemptionGranted)
	result, err := fixture.handler().Handle(t.Context(), application.FeedReferenceSeriesCommand{SeriesID: "SYN-PRC-USD-CNY"})
	if err != nil || result.Outcome != application.FeedNotAccepted {
		t.Fatalf("缺租户：outcome = %s err = %v", result.Outcome, err)
	}
	result, err = fixture.handler().Handle(t.Context(), application.FeedReferenceSeriesCommand{Tenant: fixture.binding.Tenant()})
	if err != nil || result.Outcome != application.FeedNotAccepted || len(fixture.connector.fetched) != 0 {
		t.Fatalf("缺序列：outcome = %s err = %v fetched=%d", result.Outcome, err, len(fixture.connector.fetched))
	}
}

// TestFeedOutcomeStringsAreClosed 证每一格都有名字：退出码与可观察记录都拿它说话，空串会静默。
func TestFeedOutcomeStringsAreClosed(t *testing.T) {
	for _, outcome := range []application.FeedReferenceSeriesOutcome{
		application.FeedVersionRegistered, application.FeedVersionReplayed, application.FeedBindingUnknown,
		application.FeedConnectorUnavailable, application.FeedFetchFailed, application.FeedTranscriptionRefused,
		application.FeedRegistrationRefused, application.FeedNotAccepted, application.FeedUndecided,
	} {
		if outcome.String() == "" {
			t.Fatalf("outcome %d 没有名字", outcome)
		}
	}
	if application.FeedReferenceSeriesOutcomeInvalid.String() != "" {
		t.Fatal("零值 outcome 有了名字")
	}
}
