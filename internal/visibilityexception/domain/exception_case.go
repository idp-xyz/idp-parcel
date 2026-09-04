package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidEpisode     = errors.New("visibility exception: invalid signal episode")
	ErrEpisodeEnded       = errors.New("visibility exception: the episode has ended")
	ErrEpisodeStillActive = errors.New("visibility exception: the episode is still active")
	ErrInvalidTriage      = errors.New("visibility exception: invalid triage conclusion")
	ErrInvalidCase        = errors.New("visibility exception: invalid exception case operation")
	ErrCaseClosed         = errors.New("visibility exception: the case is closed")
)

// SignalRuleVersionReference 指名识别信号所用的规则版本。防抖、宽限和恢复边界必须
// 具有规则版本（CONTEXT 硬句）——没有版本的边界改一次就没人说得清历史发作期按哪套
// 规则算的。
type SignalRuleVersionReference struct{ requiredValue }

func NewSignalRuleVersionReference(value string) (SignalRuleVersionReference, error) {
	required, err := newRequiredValue("signal rule version reference", value)
	return SignalRuleVersionReference{required}, err
}

// EpisodeID 是信号发作期的标识。
type EpisodeID struct{ requiredValue }

func NewEpisodeID(value string) (EpisodeID, error) {
	required, err := newRequiredValue("episode ID", value)
	return EpisodeID{required}, err
}

// ConfidenceReference 指名信号可信度的判断依据。
type ConfidenceReference struct{ requiredValue }

func NewConfidenceReference(value string) (ConfidenceReference, error) {
	required, err := newRequiredValue("confidence reference", value)
	return ConfidenceReference{required}, err
}

// SignalEpisode 是同一对象、信号类型、因果条件和连续影响期间的一次信号生命周期。
// 连续命中更新同一发作期；明确恢复后再次命中形成关联的新发作期（CONTEXT 语言）。
// 类型上没有案件字段——发作期结束不自动关闭关联案件，两个生命周期各走各的。
type SignalEpisode struct {
	id           EpisodeID
	kind         ExceptionSignalKindReference
	parcel       TrackedParcelReference
	rule         SignalRuleVersionReference
	confidence   ConfidenceReference
	hits         int
	startedAt    time.Time
	lastHitAt    time.Time
	releaseBasis string
	endedAt      time.Time
	priorEpisode EpisodeID
}

// OpenEpisode 依据首次满足条件建立发作期。
func OpenEpisode(
	id EpisodeID,
	kind ExceptionSignalKindReference,
	parcel TrackedParcelReference,
	rule SignalRuleVersionReference,
	confidence ConfidenceReference,
	firstHitAt time.Time,
) (*SignalEpisode, error) {
	if !id.valid() || !kind.valid() || !parcel.valid() || !rule.valid() ||
		!confidence.valid() || firstHitAt.IsZero() {
		return nil, ErrInvalidEpisode
	}
	return &SignalEpisode{
		id:         id,
		kind:       kind,
		parcel:     parcel,
		rule:       rule,
		confidence: confidence,
		hits:       1,
		startedAt:  firstHitAt.UTC(),
		lastHitAt:  firstHitAt.UTC(),
	}, nil
}

func (episode *SignalEpisode) ID() EpisodeID {
	return episode.id
}

func (episode *SignalEpisode) Hits() int {
	return episode.hits
}

func (episode *SignalEpisode) Active() bool {
	return episode.endedAt.IsZero()
}

// PriorEpisode 只在恢复后重启的关联发作期上给出。
func (episode *SignalEpisode) PriorEpisode() (EpisodeID, bool) {
	return episode.priorEpisode, episode.priorEpisode.valid()
}

// RecordHit 记录同一连续影响期内的重复命中：更新判断历史，不建立重复发作期
// （CONTEXT 生命周期）。已结束的发作期不再吸收命中——那是新发作期的事。
func (episode *SignalEpisode) RecordHit(at time.Time) error {
	if !episode.Active() {
		return ErrEpisodeEnded
	}
	if at.IsZero() || at.Before(episode.lastHitAt) {
		return ErrInvalidEpisode
	}
	episode.hits++
	episode.lastHitAt = at.UTC()
	return nil
}

// End 依据恢复边界结束发作期并保存解除依据。关联案件不因此自动关闭——本方法只动
// 发作期自己。
func (episode *SignalEpisode) End(releaseBasis string, at time.Time) error {
	if !episode.Active() {
		return ErrEpisodeEnded
	}
	if releaseBasis == "" || at.IsZero() || at.Before(episode.lastHitAt) {
		return ErrInvalidEpisode
	}
	episode.releaseBasis = releaseBasis
	episode.endedAt = at.UTC()
	return nil
}

// ReopenAsLinked 在明确恢复后再次命中时建立关联的新发作期：新标识、指回前发作期；
// 原发作期保持已结束。是否建案重新经过分诊规则——这里不带任何案件动作。
func (episode *SignalEpisode) ReopenAsLinked(
	id EpisodeID,
	firstHitAt time.Time,
) (*SignalEpisode, error) {
	if episode.Active() {
		return nil, ErrEpisodeStillActive
	}
	if !id.valid() || id == episode.id || firstHitAt.IsZero() || firstHitAt.Before(episode.endedAt) {
		return nil, ErrInvalidEpisode
	}
	linked, err := OpenEpisode(id, episode.kind, episode.parcel, episode.rule, episode.confidence, firstHitAt)
	if err != nil {
		return nil, err
	}
	linked.priorEpisode = episode.id
	return linked, nil
}

// SignalEpisodeSnapshot 是持久化层重建发作期所需的全量状态。命中数与判断历史是已
// 发生的事实——重建不重演 RecordHit（重演会把历史命中时间压成最后一次），前期指回
// 与结束依据同理随快照携带。
type SignalEpisodeSnapshot struct {
	ID           EpisodeID
	Kind         ExceptionSignalKindReference
	Parcel       TrackedParcelReference
	Rule         SignalRuleVersionReference
	Confidence   ConfidenceReference
	Hits         int
	StartedAt    time.Time
	LastHitAt    time.Time
	ReleaseBasis string
	EndedAt      time.Time
	PriorEpisode EpisodeID
}

// Snapshot 折出发作期的全量状态供持久化。
func (episode *SignalEpisode) Snapshot() SignalEpisodeSnapshot {
	return SignalEpisodeSnapshot{
		ID:           episode.id,
		Kind:         episode.kind,
		Parcel:       episode.parcel,
		Rule:         episode.rule,
		Confidence:   episode.confidence,
		Hits:         episode.hits,
		StartedAt:    episode.startedAt,
		LastHitAt:    episode.lastHitAt,
		ReleaseBasis: episode.releaseBasis,
		EndedAt:      episode.endedAt,
		PriorEpisode: episode.priorEpisode,
	}
}

// RehydrateSignalEpisode 从快照重建发作期。读回的东西同样要过一遍不变量——生命周期
// 形状（命中序、结束依据与结束时间同在场、指回不指自己）在这里重验，一次坏写入不得
// 变成一个看起来合法的发作期。
func RehydrateSignalEpisode(snapshot SignalEpisodeSnapshot) (*SignalEpisode, error) {
	if !snapshot.ID.valid() || !snapshot.Kind.valid() || !snapshot.Parcel.valid() ||
		!snapshot.Rule.valid() || !snapshot.Confidence.valid() ||
		snapshot.Hits < 1 ||
		snapshot.StartedAt.IsZero() || snapshot.LastHitAt.IsZero() ||
		snapshot.LastHitAt.Before(snapshot.StartedAt) {
		return nil, ErrInvalidEpisode
	}
	ended := !snapshot.EndedAt.IsZero()
	if ended != (snapshot.ReleaseBasis != "") {
		return nil, ErrInvalidEpisode
	}
	if ended && snapshot.EndedAt.Before(snapshot.LastHitAt) {
		return nil, ErrInvalidEpisode
	}
	if snapshot.PriorEpisode.valid() && snapshot.PriorEpisode == snapshot.ID {
		return nil, ErrInvalidEpisode
	}
	return &SignalEpisode{
		id:           snapshot.ID,
		kind:         snapshot.Kind,
		parcel:       snapshot.Parcel,
		rule:         snapshot.Rule,
		confidence:   snapshot.Confidence,
		hits:         snapshot.Hits,
		startedAt:    snapshot.StartedAt.UTC(),
		lastHitAt:    snapshot.LastHitAt.UTC(),
		releaseBasis: snapshot.ReleaseBasis,
		endedAt:      snapshot.EndedAt.UTC(),
		priorEpisode: snapshot.PriorEpisode,
	}, nil
}

// TriageOutcome 是异常分诊的封闭四走向（CONTEXT 语言：关联既有案件、自动建立案件、
// 进入人工复核或不建案）。
type TriageOutcome uint8

const (
	TriageOutcomeInvalid TriageOutcome = iota
	AttachToExistingCase
	AutoEstablishCase
	ManualReviewRequired
	NoCaseNeeded
)

func (outcome TriageOutcome) valid() bool {
	return outcome >= AttachToExistingCase && outcome <= NoCaseNeeded
}

func (outcome TriageOutcome) String() string {
	switch outcome {
	case AttachToExistingCase:
		return "ATTACH_TO_EXISTING"
	case AutoEstablishCase:
		return "AUTO_ESTABLISH"
	case ManualReviewRequired:
		return "MANUAL_REVIEW"
	case NoCaseNeeded:
		return "NO_CASE"
	default:
		return ""
	}
}

// TriageConclusion 是一次分诊判断的记录：走向、所依据的分诊规则版本与被分诊的
// 发作期。自动建案只允许来自版本化规则的命中（「高可信、高影响且命中版本化分诊
// 规则的信号可以自动建立案件」）——规则版本必备正是这半句的落点。
type TriageConclusion struct {
	episode   EpisodeID
	outcome   TriageOutcome
	rule      SignalRuleVersionReference
	triagedAt time.Time
}

func ConcludeTriage(
	episode EpisodeID,
	outcome TriageOutcome,
	rule SignalRuleVersionReference,
	triagedAt time.Time,
) (TriageConclusion, error) {
	if !episode.valid() || !outcome.valid() || !rule.valid() || triagedAt.IsZero() {
		return TriageConclusion{}, ErrInvalidTriage
	}
	return TriageConclusion{
		episode:   episode,
		outcome:   outcome,
		rule:      rule,
		triagedAt: triagedAt.UTC(),
	}, nil
}

func (conclusion TriageConclusion) Episode() EpisodeID {
	return conclusion.episode
}

func (conclusion TriageConclusion) Outcome() TriageOutcome {
	return conclusion.outcome
}

func (conclusion TriageConclusion) Rule() SignalRuleVersionReference {
	return conclusion.rule
}

func (conclusion TriageConclusion) TriagedAt() time.Time {
	return conclusion.triagedAt
}

// CaseID 是异常案件的标识。
type CaseID struct{ requiredValue }

func NewCaseID(value string) (CaseID, error) {
	required, err := newRequiredValue("case ID", value)
	return CaseID{required}, err
}

// ImpactScopeReference 指名版本化的案件影响范围。范围变化保留依据与历史，不动态
// 继承集运/舱单/班次分组。
type ImpactScopeReference struct{ requiredValue }

func NewImpactScopeReference(value string) (ImpactScopeReference, error) {
	required, err := newRequiredValue("impact scope reference", value)
	return ImpactScopeReference{required}, err
}

// ResponsibleTeamReference 指名责任团队。
type ResponsibleTeamReference struct{ requiredValue }

func NewResponsibleTeamReference(value string) (ResponsibleTeamReference, error) {
	required, err := newRequiredValue("responsible team reference", value)
	return ResponsibleTeamReference{required}, err
}

// CasePhase 是异常案件的精简主生命周期三态（CONTEXT 硬句）。等待客户、监控中、已
// 升级等是当前工作条件和下一行动，刻意不在此枚举——不扩展为互斥主状态是结构性的。
type CasePhase uint8

const (
	CasePhaseInvalid CasePhase = iota
	CaseAwaitingResponse
	CaseInProgress
	CaseClosed
)

func (phase CasePhase) String() string {
	switch phase {
	case CaseAwaitingResponse:
		return "AWAITING_RESPONSE"
	case CaseInProgress:
		return "IN_PROGRESS"
	case CaseClosed:
		return "CLOSED"
	default:
		return ""
	}
}

// ExceptionCase 是围绕同一因果链和处置范围建立的业务案件。根对象与版本化影响范围
// 在建立时固定；归并关闭保留原案件（关联主案件，历史不删不重置）。
type ExceptionCase struct {
	id            CaseID
	root          TrackedParcelReference
	scope         ImpactScopeReference
	team          ResponsibleTeamReference
	phase         CasePhase
	establishedAt time.Time
	firstResponse time.Time
	closedAt      time.Time
	conclusion    string
	mergedInto    CaseID
}

// EstablishCase 建立案件：固定根对象、初始影响范围与责任团队，主状态待响应。
func EstablishCase(
	id CaseID,
	root TrackedParcelReference,
	scope ImpactScopeReference,
	team ResponsibleTeamReference,
	at time.Time,
) (*ExceptionCase, error) {
	if !id.valid() || !root.valid() || !scope.valid() || !team.valid() || at.IsZero() {
		return nil, ErrInvalidCase
	}
	return &ExceptionCase{
		id:            id,
		root:          root,
		scope:         scope,
		team:          team,
		phase:         CaseAwaitingResponse,
		establishedAt: at.UTC(),
	}, nil
}

func (exceptionCase *ExceptionCase) ID() CaseID {
	return exceptionCase.id
}

func (exceptionCase *ExceptionCase) Phase() CasePhase {
	return exceptionCase.phase
}

func (exceptionCase *ExceptionCase) Root() TrackedParcelReference {
	return exceptionCase.root
}

// FirstResponseAt 只在接单后非零——首次响应时间随接单形成（响应周期口径）。
func (exceptionCase *ExceptionCase) FirstResponseAt() time.Time {
	return exceptionCase.firstResponse
}

// MergedInto 只在归并关闭的案件上给出。
func (exceptionCase *ExceptionCase) MergedInto() (CaseID, bool) {
	return exceptionCase.mergedInto, exceptionCase.mergedInto.valid()
}

// ExceptionCaseSnapshot 是持久化层落案件所需的全量状态。与 SignalEpisodeSnapshot 同一
// 形状纪律：已发生的事实（建立、接单、关闭、归并）随快照携带，不由持久化层重演转移。
type ExceptionCaseSnapshot struct {
	ID            CaseID
	Root          TrackedParcelReference
	Scope         ImpactScopeReference
	Team          ResponsibleTeamReference
	Phase         CasePhase
	EstablishedAt time.Time
	FirstResponse time.Time
	ClosedAt      time.Time
	Conclusion    string
	MergedInto    CaseID
}

// Snapshot 折出案件的全量状态供持久化。
func (exceptionCase *ExceptionCase) Snapshot() ExceptionCaseSnapshot {
	return ExceptionCaseSnapshot{
		ID:            exceptionCase.id,
		Root:          exceptionCase.root,
		Scope:         exceptionCase.scope,
		Team:          exceptionCase.team,
		Phase:         exceptionCase.phase,
		EstablishedAt: exceptionCase.establishedAt,
		FirstResponse: exceptionCase.firstResponse,
		ClosedAt:      exceptionCase.closedAt,
		Conclusion:    exceptionCase.conclusion,
		MergedInto:    exceptionCase.mergedInto,
	}
}

// TakeUp 接单：待响应 → 处理中，首次响应时间形成。
func (exceptionCase *ExceptionCase) TakeUp(at time.Time) error {
	if exceptionCase.phase == CaseClosed {
		return ErrCaseClosed
	}
	if exceptionCase.phase != CaseAwaitingResponse || at.IsZero() || at.Before(exceptionCase.establishedAt) {
		return ErrInvalidCase
	}
	exceptionCase.phase = CaseInProgress
	exceptionCase.firstResponse = at.UTC()
	return nil
}

// Close 以明确结论关闭案件。
func (exceptionCase *ExceptionCase) Close(conclusion string, at time.Time) error {
	if exceptionCase.phase == CaseClosed {
		return ErrCaseClosed
	}
	if conclusion == "" || at.IsZero() || at.Before(exceptionCase.establishedAt) {
		return ErrInvalidCase
	}
	exceptionCase.phase = CaseClosed
	exceptionCase.conclusion = conclusion
	exceptionCase.closedAt = at.UTC()
	return nil
}

// MergeInto 受控归并：本案件以`已归并`结论关闭并关联主案件继续处置；原编号与历史
// 不删不重置（CONTEXT 硬句），自己归并进自己不成立。
func (exceptionCase *ExceptionCase) MergeInto(main CaseID, at time.Time) error {
	if exceptionCase.phase == CaseClosed {
		return ErrCaseClosed
	}
	if !main.valid() || main == exceptionCase.id || at.IsZero() {
		return ErrInvalidCase
	}
	exceptionCase.mergedInto = main
	return exceptionCase.Close("MERGED", at)
}
