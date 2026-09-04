package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outbound"
)

// FeedReferenceSeriesOutcome 是一次来源喂价的应用处理结果。按调用方（受控批量口或调度）的续办
// 分格：登了 / 早就登过 / 先去登绑定 / 等连接器就位 / 来源那边坏了（缺口已留） / 原文装不进
// 序列（缺口已留） / 登记册要人裁 / 命令不合法 / 依赖故障未知。
type FeedReferenceSeriesOutcome uint8

const (
	FeedReferenceSeriesOutcomeInvalid FeedReferenceSeriesOutcome = iota
	// FeedVersionRegistered：新序列版本已入册；免复核声明为「是」时复核由连接器身份代写（结果
	// 在 Review 一格里，可能是拒）。
	FeedVersionRegistered
	// FeedVersionReplayed：同一份工件早已入册，幂等重放；不再代写复核。
	FeedVersionReplayed
	// FeedBindingUnknown：该租户对该序列没有来源连接器绑定——先去登记绑定。
	FeedBindingUnknown
	// FeedConnectorUnavailable：绑定声明的连接器种类本部署没装配（出网连接器留 draft 期间如此）。
	FeedConnectorUnavailable
	// FeedFetchFailed：来源不可达或原文解不开；不补数、不沿用旧值、不登空版本，缺口留着，一条
	// 可观察记录已出。
	FeedFetchFailed
	// FeedTranscriptionRefused：原文读得出但装不进这条序列（公布顺序倒置、序列不符、立不住）；
	// 同样留缺口、出一条可观察记录。
	FeedTranscriptionRefused
	// FeedRegistrationRefused：登记册给了治理答案（同版本内容冲突、形状不同、不受理）；原行未被
	// 顶替，人工续办。
	FeedRegistrationRefused
	// FeedNotAccepted：命令不合法（缺租户或序列），未到达任何依赖。
	FeedNotAccepted
	// FeedUndecided：依赖故障，做到哪一步未知，原因随错误交回。
	FeedUndecided
)

func (outcome FeedReferenceSeriesOutcome) String() string {
	switch outcome {
	case FeedVersionRegistered:
		return "REGISTERED"
	case FeedVersionReplayed:
		return "REPLAYED"
	case FeedBindingUnknown:
		return "BINDING_UNKNOWN"
	case FeedConnectorUnavailable:
		return "CONNECTOR_UNAVAILABLE"
	case FeedFetchFailed:
		return "FETCH_FAILED"
	case FeedTranscriptionRefused:
		return "TRANSCRIPTION_REFUSED"
	case FeedRegistrationRefused:
		return "REGISTRATION_REFUSED"
	case FeedNotAccepted:
		return "NOT_ACCEPTED"
	case FeedUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// FeedReferenceSeriesCommand 指名喂哪个租户的哪条序列；其余（连接器、定位符、口径、责任方、
// 免复核）全在绑定里。
type FeedReferenceSeriesCommand struct {
	Tenant   domain.TenantID
	SeriesID string
}

// FeedReferenceSeriesResult 是一次喂价的全部答案。Registration 与 Review 是两个被组合的用例各自
// 的答案，原样带出：受控批量口要拿它们译退出码。
type FeedReferenceSeriesResult struct {
	Outcome FeedReferenceSeriesOutcome
	// Reference 在已入册或幂等重放时给出。
	Reference    domain.VersionReference
	Registration RegisterReferenceSeriesOutcome
	// ReviewRequested 为真表示绑定声明免复核且登记为新版本，系统尝试代写复核；Review 是复核用例
	// 的答案（拒也如实带出，版本照登）。
	ReviewRequested bool
	Review          ReviewReferenceSeriesOutcome
}

type FeedReferenceSeriesDeps struct {
	Bindings   ports.SourceConnectorBindingLoader
	Connectors ports.SourceConnectorResolver
	Artifacts  ports.SourceArtifactStore
	Latest     ports.ReferenceSeriesLatestVersionLoader
	Register   ports.ReferenceSeriesRegister
	Versions   ports.ReferenceSeriesVersionLoader
	Reviews    ports.ReferenceSeriesReviewRegister
	Clock      ports.Clock
	// Observer 由装配方注入（ADR-0095 决定二形状）；nil 即不出声——受控批量口必须注入，抓取失败
	// 的「一条可观察记录」是它承诺的。
	Observer ports.SourceFeedObserver
}

// FeedReferenceSeriesHandler 编排来源喂价（ADR-0099 决定六）：抓取 → 存放本体 → 转录 → 走同一
// 登记用例 → 免复核时由系统代写复核。它自己不判任何领域规则：整版重述在领域，四眼门在复核用例，
// 冲突判定在登记册；这里只做受理、接线与答案翻译。
type FeedReferenceSeriesHandler struct {
	deps      FeedReferenceSeriesDeps
	registrar *RegisterReferenceSeriesHandler
	reviewer  *ReviewReferenceSeriesHandler
}

// NewFeedReferenceSeriesHandler 在内部组装登记用例与复核用例——连接器是登记责任方的转录代理，
// 不是第二种登记，所以它登记与复核走的是同两条路。
func NewFeedReferenceSeriesHandler(deps FeedReferenceSeriesDeps) *FeedReferenceSeriesHandler {
	return &FeedReferenceSeriesHandler{
		deps:      deps,
		registrar: NewRegisterReferenceSeriesHandler(RegisterReferenceSeriesDeps{Register: deps.Register}),
		reviewer: NewReviewReferenceSeriesHandler(ReviewReferenceSeriesDeps{
			Versions: deps.Versions, Reviews: deps.Reviews, Clock: deps.Clock,
		}),
	}
}

// Handle 把一次喂价推进到答案。依赖故障不吞；来源与转录的失败不是故障——它们是决定了的答案
// （缺口已留），各出一条可观察记录后照答。
func (handler *FeedReferenceSeriesHandler) Handle(
	ctx context.Context,
	command FeedReferenceSeriesCommand,
) (FeedReferenceSeriesResult, error) {
	if command.Tenant.String() == "" || command.SeriesID == "" {
		return FeedReferenceSeriesResult{Outcome: FeedNotAccepted}, nil
	}

	binding, found, err := handler.deps.Bindings.LoadCurrentBinding(ctx, command.Tenant, command.SeriesID)
	if err != nil {
		return FeedReferenceSeriesResult{Outcome: FeedUndecided}, fmt.Errorf("feed reference series: %w", err)
	}
	if !found {
		return FeedReferenceSeriesResult{Outcome: FeedBindingUnknown}, nil
	}
	connector, available := handler.deps.Connectors.ConnectorFor(binding.ConnectorKind())
	if !available {
		return FeedReferenceSeriesResult{Outcome: FeedConnectorUnavailable}, nil
	}

	observation := ports.SourceFeedObservation{
		Tenant:         binding.Tenant(),
		SeriesID:       binding.SeriesID(),
		BindingVersion: binding.Version(),
		ConnectorKind:  binding.ConnectorKind(),
		Locator:        binding.SourceLocator(),
	}

	record, err := connector.Fetch(ctx, ports.FetchSpec{Tenant: binding.Tenant(), SeriesID: binding.SeriesID(), Locator: binding.SourceLocator()})
	if err != nil {
		handler.observe(observation, ports.SourceFeedStageFetch, err, "")
		return FeedReferenceSeriesResult{Outcome: FeedFetchFailed}, nil
	}

	placement, err := handler.deps.Artifacts.Store(ctx, record)
	if err != nil {
		return FeedReferenceSeriesResult{Outcome: FeedUndecided}, fmt.Errorf("feed reference series: store artifact: %w", err)
	}
	// 未配置是诚实答案不是失败（ADR-0092 决定二）；其余非入库格才值得一声——本体没存成而登记
	// 照常，凭证不带定位符。
	if disposition := placement.Outcome.Disposition(); disposition != outbound.Accepted && disposition != outbound.NotConfigured {
		handler.observe(observation, ports.SourceFeedStageStore, nil, disposition.String())
	}

	var prior *domain.ReferenceSeriesRegistration
	latest, found, err := handler.deps.Latest.LoadLatestVersion(ctx, binding.Tenant(), binding.SeriesID())
	if err != nil {
		return FeedReferenceSeriesResult{Outcome: FeedUndecided}, fmt.Errorf("feed reference series: %w", err)
	}
	if found {
		prior = &latest
	}

	spec, err := connector.Transcribe(ports.TranscriptionInput{Record: record, Placement: placement, Binding: binding, Prior: prior})
	if err != nil {
		handler.observe(observation, ports.SourceFeedStageTranscribe, err, "")
		return FeedReferenceSeriesResult{Outcome: FeedTranscriptionRefused}, nil
	}
	registration, err := domain.NewReferenceSeriesRegistration(spec)
	if err != nil {
		handler.observe(observation, ports.SourceFeedStageTranscribe, err, "")
		return FeedReferenceSeriesResult{Outcome: FeedTranscriptionRefused}, nil
	}

	registered, err := handler.registrar.Handle(ctx, RegisterReferenceSeriesCommand{Registration: registration})
	if err != nil {
		return FeedReferenceSeriesResult{Outcome: FeedUndecided, Registration: registered}, fmt.Errorf("feed reference series: %w", err)
	}
	result := FeedReferenceSeriesResult{Registration: registered, Reference: registration.Reference()}
	switch registered {
	case ReferenceSeriesRecorded:
		result.Outcome = FeedVersionRegistered
	case ReferenceSeriesAlreadyOnRegister:
		result.Outcome = FeedVersionReplayed
		return result, nil
	default:
		handler.observe(observation, ports.SourceFeedStageRegister, nil, registered.String())
		result.Outcome = FeedRegistrationRefused
		result.Reference = domain.VersionReference{}
		return result, nil
	}

	if binding.RequiresManualReview() {
		return result, nil
	}
	// 免复核声明：复核由连接器身份代写、依据为该绑定的版本（ADR-0099 决定六）。时刻留零让复核
	// 用例取时钟当下；四眼门在那里——租户把登记责任方填成连接器身份，这里会如实拿到拒。
	result.ReviewRequested = true
	review, err := handler.reviewer.Handle(ctx, ReviewReferenceSeriesCommand{
		Tenant:        binding.Tenant(),
		SeriesID:      registration.Reference().ID(),
		SeriesVersion: registration.Reference().Version(),
		Reviewer:      binding.ConnectorIdentity(),
		Decision:      domain.SeriesReviewApproved,
		Basis:         binding.ExemptionReviewBasis(),
	})
	result.Review = review
	if err != nil {
		result.Outcome = FeedUndecided
		return result, fmt.Errorf("feed reference series: %w", err)
	}
	if review != SeriesReviewRecorded && review != SeriesReviewAlreadyOnRegister {
		handler.observe(observation, ports.SourceFeedStageReview, nil, review.String())
	}
	return result, nil
}

func (handler *FeedReferenceSeriesHandler) observe(observation ports.SourceFeedObservation, stage ports.SourceFeedStage, err error, answer string) {
	if handler.deps.Observer == nil {
		return
	}
	observation.Stage = stage
	observation.Err = err
	observation.Answer = answer
	handler.deps.Observer(observation)
}
