package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ErrUnexpectedTriageOutcome 说明分诊规则端口交回了封闭四走向以外的答复。上抛而不译成
// 业务结果：集合外的取值没有恢复动作可派。
var ErrUnexpectedTriageOutcome = errors.New("visibility exception: unexpected triage outcome")

// ErrAutoEstablishWithoutTeam 说明分诊规则端口答了`自动建案`却没说归哪个责任团队。
// 「每个开放案件始终必须有一个内部案件责任团队」（CONTEXT）——没有团队的案件建不
// 起来，而团队是自动建案条目登记时必带的一维（ports.TriageAnswer）。这是登记面漏了
// 一格的端口坏答复，上抛而不吞：吞成人工复核会把「规则已命中」写成「规则没命中」。
var ErrAutoEstablishWithoutTeam = errors.New("visibility exception: auto-establish triage answer names no responsible team")

// triageRulesNotConfigured 是「分诊规则未配置」这一格落进结论时的规则版本占位引用。
// 它不是一套规则：结论要能按规则维度复核，这一格要能与任何真实规则版本分得开。
const triageRulesNotConfigured = "TRIAGE_RULES_NOT_CONFIGURED"

// RaiseSignalOutcome 是信号命中推进发作期与分诊的应用处理结果。开启、命中与重开分成
// 三格：三者对下游的含义不同——只有开启与重开产生新的分诊结论，命中只更新判断历史。
type RaiseSignalOutcome uint8

const (
	RaiseSignalOutcomeInvalid RaiseSignalOutcome = iota
	SignalEpisodeOpened
	SignalHitRecorded
	SignalEpisodeReopened
	RaiseSignalUndecided
	RaiseSignalNotAccepted
)

func (outcome RaiseSignalOutcome) String() string {
	switch outcome {
	case SignalEpisodeOpened:
		return "EPISODE_OPENED"
	case SignalHitRecorded:
		return "HIT_RECORDED"
	case SignalEpisodeReopened:
		return "EPISODE_REOPENED"
	case RaiseSignalUndecided:
		return "UNDECIDED"
	case RaiseSignalNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// RaiseSignalUndecidedReason 指名本轮停在哪一步。
type RaiseSignalUndecidedReason uint8

const (
	RaiseSignalUndecidedReasonNone RaiseSignalUndecidedReason = iota
	SignalEpisodeStoreUnavailable
	TriageRuleUnavailable
	SignalEpisodeIdentityUnavailable
	CaseIdentityUnavailable
)

func (reason RaiseSignalUndecidedReason) String() string {
	switch reason {
	case SignalEpisodeStoreUnavailable:
		return "SIGNAL_EPISODE_STORE_UNAVAILABLE"
	case TriageRuleUnavailable:
		return "TRIAGE_RULE_UNAVAILABLE"
	case SignalEpisodeIdentityUnavailable:
		return "SIGNAL_EPISODE_IDENTITY_UNAVAILABLE"
	case CaseIdentityUnavailable:
		return "CASE_IDENTITY_UNAVAILABLE"
	default:
		return ""
	}
}

// RaiseSignalCommand 携带一次信号命中。规则版本与可信度必备且来自命中事实——「每个
// 信号必须保存对象、类型、规则版本、判断时间、事实依据、可信度」（CONTEXT），编排不
// 替命中补任何一样。租户显式随命令到达（ADR-0003）：发作期按租户内的对象+类型定位，
// 编排不替命中补租户。
type RaiseSignalCommand struct {
	TenantID   domain.TenantID
	Kind       domain.ExceptionSignalKindReference
	Parcel     domain.TrackedParcelReference
	Rule       domain.SignalRuleVersionReference
	Confidence domain.ConfidenceReference
	HitAt      time.Time
}

type RaiseSignalResult struct {
	outcome       RaiseSignalOutcome
	episode       *domain.SignalEpisode
	conclusion    domain.TriageConclusion
	hasConclusion bool
	exceptionCase *domain.ExceptionCase
	reason        RaiseSignalUndecidedReason
	handoffRef    string
}

func (result RaiseSignalResult) Outcome() RaiseSignalOutcome {
	return result.outcome
}

// Episode 只在开启、命中或重开成立时给出。
func (result RaiseSignalResult) Episode() (*domain.SignalEpisode, bool) {
	return result.episode, result.episode != nil
}

// TriageConclusion 只在开启或重开时给出——命中不重新分诊，「是否建立或重开案件重新
// 经过分诊规则」说的是重开，不是同一发作期内的每次命中。
func (result RaiseSignalResult) TriageConclusion() (domain.TriageConclusion, bool) {
	return result.conclusion, result.hasConclusion
}

// Case 只在本轮分诊走向为`自动建案`且案件随结论落库时给出。命中与重开不建案的那些
// 轮次没有它——建不建案由走向决定，不由信号存在决定。
func (result RaiseSignalResult) Case() (*domain.ExceptionCase, bool) {
	return result.exceptionCase, result.exceptionCase != nil
}

func (result RaiseSignalResult) UndecidedReason() RaiseSignalUndecidedReason {
	return result.reason
}

// HandoffReference 非空说明分诊结论已落库但意图还没交出去，重放会重发同一份。
func (result RaiseSignalResult) HandoffReference() string {
	return result.handoffRef
}

type RaiseSignalDeps struct {
	Episodes   ports.SignalEpisodeStore
	Triage     ports.TriageRuleView
	Identities ports.SignalEpisodeIdentityFactory
	Cases      ports.CaseIdentityFactory
	Downstream ports.TriageHandoff
	Clock      ports.Clock
}

type RaiseSignalHandler struct {
	deps RaiseSignalDeps
}

func NewRaiseSignalHandler(deps RaiseSignalDeps) *RaiseSignalHandler {
	return &RaiseSignalHandler{deps: deps}
}

// Handle 把一次信号命中推进到发作期与分诊：受理（命中事实缺一即未受理，不读依赖）→
// 按对象+类型找最近发作期 → 活跃则记命中不建重复、不重新分诊、不重复建案 → 已结束或
// 无既往则分诊（规则未配置即人工复核格，如实不自动建案）→ 开启或重开（重开指回前
// 发作期）→ 走向为`自动建案`时建立案件（根对象取信号对象、责任团队取规则条目登记的
// 那个、主状态待响应）→ 发作期、分诊结论与案件同一提交 → 发布意图。
//
// 信号与案件始终是两个对象（CONTEXT）：案件有自己的标识与生命周期，这里只做「结论说
// 建案就当场建」这一步，接单、关闭、归并与重开都不在本编排；关联既有案件那一走向
// （ATTACH_TO_EXISTING）要先答「同因果链的既有案件是哪个」，那道检索今天没有读口，
// 结论照常落库、由消费方续办——本编排不替它猜一个案件挂上去。
func (handler *RaiseSignalHandler) Handle(
	ctx context.Context,
	command RaiseSignalCommand,
) (RaiseSignalResult, error) {
	// 先判命中事实再读依赖：规则版本或可信度立不起来的输入构不成信号，而一次已经发出
	// 的查询收不回来。
	if command.TenantID.String() == "" ||
		command.Kind.String() == "" ||
		command.Parcel.String() == "" ||
		command.Rule.String() == "" ||
		command.Confidence.String() == "" ||
		command.HitAt.IsZero() {
		return RaiseSignalResult{outcome: RaiseSignalNotAccepted}, nil
	}

	latest, found, err := handler.deps.Episodes.FindLatest(ctx, command.TenantID, command.Parcel, command.Kind)
	if err != nil {
		return RaiseSignalResult{outcome: RaiseSignalUndecided, reason: SignalEpisodeStoreUnavailable}, nil
	}
	if found && latest.Active() {
		// 同一连续影响期内的重复命中：更新判断历史，不建重复发作期，也不重新分诊。
		// 命中没有自有身份，重复到达按新命中计入——发作期的幂等在「不建第二个发作期」，
		// 不在命中计数。
		if err := latest.RecordHit(command.HitAt); err != nil {
			return RaiseSignalResult{}, fmt.Errorf("record signal hit: %w", err)
		}
		if err := handler.deps.Episodes.SaveHit(ctx, command.TenantID, latest); err != nil {
			return RaiseSignalResult{outcome: RaiseSignalUndecided, reason: SignalEpisodeStoreUnavailable}, nil
		}
		return RaiseSignalResult{outcome: SignalHitRecorded, episode: latest}, nil
	}

	// 分诊先于身份签发：规则视图答不出时本轮什么都没写，重试不浪费一个发作期标识。
	// 重开的分诊查询用本次命中的事实——「恢复后再次满足条件……是否建立或重开案件重新
	// 经过分诊规则」，重新经过的是这次命中，不是上一期的旧账。
	verdict, err := handler.triage(ctx, command)
	if err != nil {
		var undecided *triageUnavailable
		if errors.As(err, &undecided) {
			return RaiseSignalResult{outcome: RaiseSignalUndecided, reason: TriageRuleUnavailable}, nil
		}
		return RaiseSignalResult{}, err
	}

	episodeID, err := handler.deps.Identities.NextEpisodeID(ctx)
	if err != nil {
		return RaiseSignalResult{outcome: RaiseSignalUndecided, reason: SignalEpisodeIdentityUnavailable}, nil
	}
	var caseID domain.CaseID
	if verdict.outcome == domain.AutoEstablishCase {
		// 案件标识在写库之前签：签不出就整轮不写——结论「自动建案」与案件必须同一提交，
		// 先落结论再补案件的重放会走进「已有活跃发作期」那一支去记命中。
		caseID, err = handler.deps.Cases.NextCaseID(ctx)
		if err != nil {
			return RaiseSignalResult{outcome: RaiseSignalUndecided, reason: CaseIdentityUnavailable}, nil
		}
	}

	var episode *domain.SignalEpisode
	var raiseOutcome RaiseSignalOutcome
	if found {
		// 条件明确解除后再次发生：建立关联的新发作期指回前期，原发作期保持已结束。
		episode, err = latest.ReopenAsLinked(episodeID, command.HitAt)
		raiseOutcome = SignalEpisodeReopened
	} else {
		episode, err = domain.OpenEpisode(
			episodeID,
			command.Kind,
			command.Parcel,
			command.Rule,
			command.Confidence,
			command.HitAt,
		)
		raiseOutcome = SignalEpisodeOpened
	}
	if err != nil {
		return RaiseSignalResult{}, fmt.Errorf("raise signal episode: %w", err)
	}

	now := handler.deps.Clock.Now()
	conclusion, err := domain.ConcludeTriage(episode.ID(), verdict.outcome, verdict.rule, now)
	if err != nil {
		return RaiseSignalResult{}, fmt.Errorf("conclude triage: %w", err)
	}

	var exceptionCase *domain.ExceptionCase
	if verdict.outcome == domain.AutoEstablishCase {
		exceptionCase, err = establishCaseFor(caseID, command.Parcel, verdict.team, now)
		if err != nil {
			return RaiseSignalResult{}, err
		}
	}

	if err := handler.deps.Episodes.SaveRaised(ctx, ports.RaisedSignalRecord{
		Tenant:     command.TenantID,
		Parcel:     command.Parcel,
		Kind:       command.Kind,
		Episode:    episode,
		Conclusion: conclusion,
		Case:       exceptionCase,
	}); err != nil {
		// 发作期、结论与案件没能越过提交边界就都不算成立，这一支不发意图。
		return RaiseSignalResult{outcome: RaiseSignalUndecided, reason: SignalEpisodeStoreUnavailable}, nil
	}

	result := RaiseSignalResult{
		outcome:       raiseOutcome,
		episode:       episode,
		conclusion:    conclusion,
		hasConclusion: true,
		exceptionCase: exceptionCase,
	}
	if err := handler.deps.Downstream.HandOffTriage(ctx, ports.TriageHandoffIntent{
		TenantID:   command.TenantID,
		Parcel:     command.Parcel,
		Kind:       command.Kind,
		Conclusion: conclusion,
	}); err != nil {
		result.handoffRef = "CONT-" + shortDigest("TRIAGE_HANDOFF", episode.ID().String())
	}
	return result, nil
}

// triageUnavailable 区分「规则视图调不通」与其余错误：前者是未决，后者上抛。用哨兵
// 类型而不是布尔返回，让 triage 的签名只有一条错误通道。
type triageUnavailable struct{ cause error }

func (unavailable *triageUnavailable) Error() string {
	return "triage rule view unavailable: " + unavailable.cause.Error()
}

// triageVerdict 是一次分诊查询折出的三件：走向、所依据的规则版本、以及走向为`自动
// 建案`时规则条目登记的责任团队。
type triageVerdict struct {
	outcome domain.TriageOutcome
	rule    domain.SignalRuleVersionReference
	team    domain.ResponsibleTeamReference
}

// triage 取分诊走向与规则版本。未配置不是未决：真实分诊规则属待登记实例参数，没有
// 规则时信号如实进人工复核格——「高可信、高影响且命中版本化分诊规则的信号可以自动
// 建立或关联案件」，没命中版本化规则就没有自动建案，把空白读成放行或未决都是虚构。
func (handler *RaiseSignalHandler) triage(
	ctx context.Context,
	command RaiseSignalCommand,
) (triageVerdict, error) {
	answer, configured, err := handler.deps.Triage.TriageSignal(ctx, ports.TriageQuery{
		Kind:       command.Kind,
		Parcel:     command.Parcel,
		Rule:       command.Rule,
		Confidence: command.Confidence,
	})
	if err != nil {
		return triageVerdict{}, &triageUnavailable{cause: err}
	}
	if !configured {
		fallback, err := domain.NewSignalRuleVersionReference(triageRulesNotConfigured)
		if err != nil {
			return triageVerdict{}, err
		}
		return triageVerdict{outcome: domain.ManualReviewRequired, rule: fallback}, nil
	}
	// 逐取值分派，不留兜底：规则端口日后新增一种走向时这里报错，而不是静默归入某一格。
	switch answer.Outcome {
	case domain.AutoEstablishCase:
		if answer.Team.String() == "" {
			return triageVerdict{}, fmt.Errorf("%w: rule %s", ErrAutoEstablishWithoutTeam, answer.Rule)
		}
		return triageVerdict{outcome: answer.Outcome, rule: answer.Rule, team: answer.Team}, nil
	case domain.AttachToExistingCase, domain.ManualReviewRequired, domain.NoCaseNeeded:
		return triageVerdict{outcome: answer.Outcome, rule: answer.Rule}, nil
	default:
		return triageVerdict{}, fmt.Errorf("%w: %d", ErrUnexpectedTriageOutcome, answer.Outcome)
	}
}

// establishCaseFor 依据自动建案走向建立案件：根对象取信号对象，责任团队取规则条目登记
// 的那个，主状态待响应（`AT-VE-062`）。
//
// 初始影响范围只含根对象——「每个案件必须具有一个根对象和显式、版本化的影响范围」
// （CONTEXT），而一次单对象信号的自动建案能明确覆盖的就只有那件对象；范围扩大、缩小
// 与拆出各形成后续版本、保留依据，不在建案这一步。范围引用因此直接指名根对象，不另
// 造一个没有内容的范围标识：集运、装载或同批关系不会让别的对象隐式进范围（CONTEXT
// 硬句），这一格没有任何东西可供推导。
func establishCaseFor(
	id domain.CaseID,
	root domain.TrackedParcelReference,
	team domain.ResponsibleTeamReference,
	at time.Time,
) (*domain.ExceptionCase, error) {
	scope, err := domain.NewImpactScopeReference(root.String())
	if err != nil {
		return nil, fmt.Errorf("fix initial impact scope: %w", err)
	}
	exceptionCase, err := domain.EstablishCase(id, root, scope, team, at)
	if err != nil {
		return nil, fmt.Errorf("establish exception case: %w", err)
	}
	return exceptionCase, nil
}
