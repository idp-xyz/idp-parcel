package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// ErrIncompleteRouteEvidence 说明证据端口的答复缺了判断必备件（修订标识、策略引用或
// 选中候选的段链）。上抛而不落未决：那不是依赖答不出，是答复本身坏了。
var ErrIncompleteRouteEvidence = errors.New("network routing: initial route evidence is incomplete")

// ErrUnexpectedRouteSaveOutcome 说明路由库交回了封闭集合以外的写入结果。
var ErrUnexpectedRouteSaveOutcome = errors.New("network routing: unexpected initial route save outcome")

// ErrNotFormedRegisterMissing 说一件包裹停在了重投推不动的一格，装配却没给未形成登记可写。只有装配缺漏到得了这里，响亮
// 报错：折成未决会让一个装配缺陷占着分区重投，假装在等依赖。
var ErrNotFormedRegisterMissing = errors.New("network routing: initial route has no not-formed register to record into")

// RouteHandoffOutcome 是整份交接的应用处理结果。逐包裹结果另有自己的集合：`已处理`只说
// 交接被受理且逐包裹各有答案，不说每个包裹都有了计划——委托级「已路由」正是被禁的。
type RouteHandoffOutcome uint8

const (
	RouteHandoffOutcomeInvalid RouteHandoffOutcome = iota
	RouteHandoffProcessed
	RouteHandoffNotAccepted
	RouteHandoffConflict
	RouteHandoffNotApplicable
	RouteHandoffUndecided
)

func (outcome RouteHandoffOutcome) String() string {
	switch outcome {
	case RouteHandoffProcessed:
		return "PROCESSED"
	case RouteHandoffNotAccepted:
		return "NOT_ACCEPTED"
	case RouteHandoffConflict:
		return "CONFLICT"
	case RouteHandoffNotApplicable:
		return "NOT_APPLICABLE"
	case RouteHandoffUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// ParcelRouteOutcome 是一个包裹的处理结果。`无当前有效路由`是领域判断、`未决`与`路由未形成`是应用结果，三者与`已形成`
// 分开——用例明写它们不能合并为一个通用失败状态。`未决`与`路由未形成`按恢复动作分（ADR-0029、ADR-0273 决定二）：前者重投
// 推得动，消费门回滚重投；后者重投推不动，原因落未形成登记、入账，同一交接里已判成的包裹照常提交。
type ParcelRouteOutcome uint8

const (
	ParcelRouteOutcomeInvalid ParcelRouteOutcome = iota
	ParcelRouteFormed
	ParcelNoCurrentRoute
	ParcelRouteUndecided
	ParcelExistingResult
	ParcelRouteNotFormed
)

func (outcome ParcelRouteOutcome) String() string {
	switch outcome {
	case ParcelRouteFormed:
		return "ROUTE_FORMED"
	case ParcelNoCurrentRoute:
		return "NO_CURRENT_ROUTE"
	case ParcelRouteUndecided:
		return "UNDECIDED"
	case ParcelExistingResult:
		return "EXISTING_RESULT"
	case ParcelRouteNotFormed:
		return "NOT_FORMED"
	default:
		return ""
	}
}

// RouteUndecidedReason 指名一个包裹为何停在未决。封闭集合，只收重投推得动的格——依赖读不回、证据换代、标识签不出、
// 原因写不进；重投推不动的归 ports.InitialRouteNotFormedReason，判据只在本编排一处（ADR-0273 决定二），适配器按结果折、
// 不持原因字面量。
type RouteUndecidedReason uint8

const (
	RouteUndecidedReasonNone RouteUndecidedReason = iota
	RouteStoreUnavailable
	// RouteEvidenceUnavailable 是证据视图读不回，与`路由未形成`的证据未配置分格：未配置要租户去登记网络定义，不可用要
	// 运维去救依赖，混成一格会把这两拨人都指错方向。
	RouteEvidenceUnavailable
	RouteIdentityUnavailable
	RouteHandoffLogUnavailable
	RoutingApplicabilityUnavailable
	RouteEvidenceSuperseded
	// RouteCostSourceUnavailable 是取数侧执行失败（依赖读不回）：等依赖恢复重试。取数侧答「用不了」不在这里，响亮报错
	// （ports.ErrRouteCostAnswerUnusable）。
	RouteCostSourceUnavailable
	// RouteNotFormedUnrecorded 是包裹停在了重投推不动的一格，原因却写不进未形成登记：原因没落库不得入账，改答未决、
	// 整封回滚重投。
	RouteNotFormedUnrecorded
)

func (reason RouteUndecidedReason) String() string {
	switch reason {
	case RouteStoreUnavailable:
		return "ROUTE_STORE_UNAVAILABLE"
	case RouteEvidenceUnavailable:
		return "ROUTE_EVIDENCE_UNAVAILABLE"
	case RouteIdentityUnavailable:
		return "ROUTE_IDENTITY_UNAVAILABLE"
	case RouteHandoffLogUnavailable:
		return "HANDOFF_LOG_UNAVAILABLE"
	case RoutingApplicabilityUnavailable:
		return "ROUTING_APPLICABILITY_UNAVAILABLE"
	case RouteEvidenceSuperseded:
		return "ROUTE_EVIDENCE_SUPERSEDED"
	case RouteCostSourceUnavailable:
		return "COST_SOURCE_UNAVAILABLE"
	case RouteNotFormedUnrecorded:
		return "NOT_FORMED_UNRECORDED"
	default:
		return ""
	}
}

// CreateInitialRouteCommand 携带路由交接的原料、服务目的和已接受解析标识。
// 目的是消费侧装配参数（与可达性适配器的 Purpose 同款）：属服务产品的话语，未配置时
// 交接未受理。解析标识是命令附加字段，不是判断维（ADR-0064）。
// Carried 是接受交接带来的地理解析投影。初始路由证据折叠还不收这格，指纹把它算进去，
// 同一交接换了地址就不再是重放。
type CreateInitialRouteCommand struct {
	Handoff    domain.RouteHandoffSpec
	Purpose    domain.ServicePurpose
	Resolution domain.CommercialResolutionReference
	Carried    ports.RequestCarriedContent
}

// ParcelRouteResult 是一个包裹的独立结果。计划与无路由各按在场标志给出，未决与路由未形成各带原因与续办引用；并列另带
// 全部候选、并列的那几家与它们的成本事实，授权角色裁的时候要看得见在哪几家之间、按什么金额打平；候选证据不全另带全部
// 候选与缺口。
type ParcelRouteResult struct {
	key          domain.InitialRouteJudgmentKey
	outcome      ParcelRouteOutcome
	plan         domain.InitialRoutePlan
	hasPlan      bool
	noRoute      domain.NoCurrentRouteJudgment
	hasNoRoute   bool
	reason       RouteUndecidedReason
	notFormed    ports.InitialRouteNotFormedReason
	continuation ContinuationReference
	handoff      ContinuationReference
	candidates   []domain.RouteCandidate
	tied         []domain.CandidateID
	tiedCosts    []domain.CandidateCostFact
	gaps         []domain.EvidenceGap
}

func (result ParcelRouteResult) Key() domain.InitialRouteJudgmentKey {
	return result.key
}

func (result ParcelRouteResult) Outcome() ParcelRouteOutcome {
	return result.outcome
}

func (result ParcelRouteResult) Plan() (domain.InitialRoutePlan, bool) {
	return result.plan, result.hasPlan
}

func (result ParcelRouteResult) NoCurrentRoute() (domain.NoCurrentRouteJudgment, bool) {
	return result.noRoute, result.hasNoRoute
}

func (result ParcelRouteResult) UndecidedReason() RouteUndecidedReason {
	return result.reason
}

// NotFormedReason 只在`路由未形成`时给出。
func (result ParcelRouteResult) NotFormedReason() ports.InitialRouteNotFormedReason {
	return result.notFormed
}

func (result ParcelRouteResult) ContinuationReference() ContinuationReference {
	return result.continuation
}

// RouteHandoffReference 非空说明结果已提交但意图还没交出去，重放会重发同一份。
func (result ParcelRouteResult) RouteHandoffReference() ContinuationReference {
	return result.handoff
}

// Candidates 只在候选并列与候选证据不全时给出：本轮评估过的全部候选及各自的选择或淘汰依据。
func (result ParcelRouteResult) Candidates() []domain.RouteCandidate {
	return append([]domain.RouteCandidate(nil), result.candidates...)
}

func (result ParcelRouteResult) TiedCandidates() []domain.CandidateID {
	return append([]domain.CandidateID(nil), result.tied...)
}

// TiedCosts 只在并列时给出：并列那几家各自的候选成本事实。
func (result ParcelRouteResult) TiedCosts() []domain.CandidateCostFact {
	return append([]domain.CandidateCostFact(nil), result.tiedCosts...)
}

// EvidenceGaps 只在候选证据不全时给出：缺什么、什么条件再判。
func (result ParcelRouteResult) EvidenceGaps() []domain.EvidenceGap {
	return append([]domain.EvidenceGap(nil), result.gaps...)
}

type CreateInitialRouteResult struct {
	outcome RouteHandoffOutcome
	parcels []ParcelRouteResult
	basis   domain.EligibilityBasisReference
	reason  RouteUndecidedReason
}

func (result CreateInitialRouteResult) Outcome() RouteHandoffOutcome {
	return result.outcome
}

func (result CreateInitialRouteResult) Parcels() []ParcelRouteResult {
	return append([]ParcelRouteResult(nil), result.parcels...)
}

// ApplicabilityBasis 只在`不适用`时给出：不适用必须带产品依据，与可达性同一条纪律。
func (result CreateInitialRouteResult) ApplicabilityBasis() domain.EligibilityBasisReference {
	return result.basis
}

func (result CreateInitialRouteResult) UndecidedReason() RouteUndecidedReason {
	return result.reason
}

type CreateInitialRouteDeps struct {
	Applicability ports.RoutingApplicabilityView
	Evidence      ports.InitialRouteEvidenceView
	Costs         ports.RouteCandidateCostSource
	Store         ports.InitialRouteStore
	// NotFormed 收重投推不动那几格的原因与对象。只有包裹真停在那几格时才问它；那时它为 nil 是装配缺漏，响亮报错。
	NotFormed  ports.InitialRouteNotFormedRegister
	Log        ports.RouteHandoffLog
	Downstream ports.InitialRouteHandoff
	Identities ports.RouteIdentityFactory
	Clock      ports.Clock
}

type CreateInitialRouteHandler struct {
	deps CreateInitialRouteDeps
}

func NewCreateInitialRouteHandler(deps CreateInitialRouteDeps) *CreateInitialRouteHandler {
	return &CreateInitialRouteHandler{deps: deps}
}

// Handle 把一份委托接受交接推进到逐包裹结果：受理 → 重放/冲突分界 → 适用性 → 逐包裹
// 「已有结果 / 评估管线 / 提交 / 发布意图 / 未形成留原因」。单个包裹无路由或路由未形成不回滚其他包裹已经合法形成的
// 计划（`AT-NR-012`），也不被一个委托级状态掩盖。依赖类未决由消费门整封回滚重投：同一交接里判成的包裹那时还没越过
// 提交边界，重投时按判断键再判一次。
func (handler *CreateInitialRouteHandler) Handle(
	ctx context.Context,
	command CreateInitialRouteCommand,
) (CreateInitialRouteResult, error) {
	handoff, err := domain.NewRouteHandoff(command.Handoff)
	if err != nil {
		return CreateInitialRouteResult{outcome: RouteHandoffNotAccepted}, nil
	}
	keys, err := handoff.JudgmentKeys(command.Purpose)
	if err != nil {
		return CreateInitialRouteResult{outcome: RouteHandoffNotAccepted}, nil
	}

	// 重放与冲突在读任何权威之前分界（步骤 2）：同关联同指纹是重放（继续走，逐包裹按键
	// 找回已有结果），异指纹是冲突——原交接与原结果不被覆盖。
	digest := handoffDigest(command.Handoff, command.Carried.Geo)
	recorded, found, err := handler.deps.Log.FindDigest(ctx, command.Handoff.TenantID, handoff.Correlation())
	if err != nil {
		return handler.undecidedHandoff(keys, RouteHandoffLogUnavailable), nil
	}
	if found && recorded != digest {
		return CreateInitialRouteResult{outcome: RouteHandoffConflict}, nil
	}
	if !found {
		if err := handler.deps.Log.Append(ctx, command.Handoff.TenantID, handoff.Correlation(), digest); err != nil {
			return handler.undecidedHandoff(keys, RouteHandoffLogUnavailable), nil
		}
	}

	// 适用性是服务级判断（步骤 3）：产品不要求网络路由时整份交接不适用，不虚构任何
	// 包裹级路由。读不回形成未决，不读成`不适用`。解析标识从命令来，不从判断键发明。
	if !command.Resolution.Valid() {
		return handler.undecidedHandoff(keys, RoutingApplicabilityUnavailable), nil
	}
	eligibility, err := handler.deps.Applicability.AssessRoutingApplicability(
		ctx, keys[0], command.Resolution)
	if err != nil {
		return handler.undecidedHandoff(keys, RoutingApplicabilityUnavailable), nil
	}
	if !eligibility.JudgmentRequired() {
		return CreateInitialRouteResult{
			outcome: RouteHandoffNotApplicable,
			basis:   eligibility.Basis(),
		}, nil
	}

	results := make([]ParcelRouteResult, 0, len(keys))
	for _, key := range keys {
		result, err := handler.routeOneParcel(ctx, handoff.Correlation(), key, handoff.AcceptedAt(), command.Carried)
		if err != nil {
			return CreateInitialRouteResult{}, err
		}
		if result.outcome == ParcelRouteNotFormed {
			result, err = handler.recordNotFormed(ctx, handoff.Correlation(), handoff.AcceptedAt(), result)
			if err != nil {
				return CreateInitialRouteResult{}, err
			}
		}
		results = append(results, result)
	}
	return CreateInitialRouteResult{outcome: RouteHandoffProcessed, parcels: results}, nil
}

// routeOneParcel 为一个包裹形成独立结果。任何一格的未决都只属于这个包裹。
//
// 提交前重校（`AT-NR-010`）：候选选出后、越过提交边界前重读证据修订，换代即用新依据
// 重新判断一轮——「原候选不得提交为当前计划」。连续换代说明视图正在滚动，保持未决
// 比追着一个动的目标提交要诚实。
//
// 路由库里落了册的一行重建不出来是坏数据，重投推不动，响亮报错；读不回才是依赖故障，答未决让消费门回滚重投。复核读同一
// 类坏行同一口径（ports.ErrStoredRouteRecordUnreadable 头注）。
func (handler *CreateInitialRouteHandler) routeOneParcel(
	ctx context.Context,
	correlation domain.RequestCorrelationID,
	key domain.InitialRouteJudgmentKey,
	acceptedAt time.Time,
	carried ports.RequestCarriedContent,
) (ParcelRouteResult, error) {
	existing, found, err := handler.deps.Store.FindByKey(ctx, key)
	if errors.Is(err, ports.ErrStoredRouteRecordUnreadable) {
		return ParcelRouteResult{}, fmt.Errorf("load initial route: %w", err)
	}
	if err != nil {
		return handler.undecidedParcel(key, RouteStoreUnavailable), nil
	}
	if found {
		return handler.existingResult(ctx, correlation, key, existing), nil
	}

	evidence, configured, err := handler.deps.Evidence.LoadInitialRouteEvidence(ctx, key, carried)
	if err != nil {
		return handler.undecidedParcel(key, RouteEvidenceUnavailable), nil
	}
	if !configured {
		return handler.notFormedParcel(key, ports.NotFormedEvidenceNotConfigured), nil
	}

	for attempt := 0; attempt < 2; attempt++ {
		record, undecided, err := handler.judgeParcel(ctx, key, acceptedAt, evidence, carried)
		if err != nil {
			return ParcelRouteResult{}, err
		}

		fresh, configured, err := handler.deps.Evidence.LoadInitialRouteEvidence(ctx, key, carried)
		if err != nil {
			return handler.undecidedParcel(key, RouteEvidenceUnavailable), nil
		}
		if !configured {
			// 判断中途登记册被撤下：重校对不出修订，提交这一版等于拿一份已经没有出处
			// 的证据定案。
			return handler.notFormedParcel(key, ports.NotFormedEvidenceNotConfigured), nil
		}
		if fresh.ViewRevision != evidence.ViewRevision {
			evidence = fresh
			continue
		}
		if undecided != nil {
			return *undecided, nil
		}
		return handler.commit(ctx, correlation, key, record)
	}
	return handler.undecidedParcel(key, RouteEvidenceSuperseded), nil
}

// judgeParcel 以一份证据执行评估管线并形成待提交记录。三种出口：记录、未决、装配错误。
func (handler *CreateInitialRouteHandler) judgeParcel(
	ctx context.Context,
	key domain.InitialRouteJudgmentKey,
	acceptedAt time.Time,
	evidence ports.InitialRouteEvidence,
	carried ports.RequestCarriedContent,
) (ports.InitialRouteRecord, *ParcelRouteResult, error) {
	none := ports.InitialRouteRecord{}
	if !evidence.ViewRevision.Valid() || !evidence.Strategy.Valid() {
		return none, nil, ErrIncompleteRouteEvidence
	}

	// 评估在领域执行（ADR-0046），按层次推进：区域 → 承诺分界 → 可执行性 → 硬约束 →
	// 时间可行性。事实本身不成立是端口坏了，响亮上抛。
	candidates, areaGaps, err := domain.EvaluateServiceAreas(evidence.ServiceAreas)
	if err != nil {
		return none, nil, fmt.Errorf("evaluate service areas: %w", err)
	}
	if len(candidates) == 0 {
		notFormed := handler.notFormedParcel(key, ports.NotFormedCandidateSpaceNotEstablished)
		return none, &notFormed, nil
	}
	candidates, err = domain.EvaluateRouteRequirements(candidates, evidence.RouteRequirements)
	if err != nil {
		return none, nil, fmt.Errorf("evaluate route requirements: %w", err)
	}
	candidates, err = domain.EvaluatePathExecutability(candidates, evidence.PathExecutability)
	if err != nil {
		return none, nil, fmt.Errorf("evaluate path executability: %w", err)
	}
	candidates, constraintGaps, err := domain.EvaluateHardConstraints(candidates, evidence.HardConstraints)
	if err != nil {
		return none, nil, fmt.Errorf("evaluate hard constraints: %w", err)
	}
	gaps := append(append([]domain.EvidenceGap(nil), areaGaps...), constraintGaps...)
	candidates, err = domain.EvaluateTimeFeasibility(candidates, evidence.Projections, evidence.CommittedBound)
	if err != nil {
		return none, nil, fmt.Errorf("evaluate time feasibility: %w", err)
	}

	judgedAt := handler.deps.Clock.Now()

	// 成本族在取数侧兑现（routing-first-cut/10）：取数侧没接是等部署、读不回是等依赖，都不拿缺依据的候选装成可排序的
	// 事实。取数侧答「用不了」重投多少次都一样，又不是一格等登记的缺口——折成未形成会在登记里留一条点名租户没登记的
	// 假原因——所以响亮报错。
	if handler.deps.Costs == nil {
		notFormed := handler.notFormedParcel(key, ports.NotFormedCostSourceNotConfigured)
		return none, &notFormed, nil
	}
	costs, err := handler.deps.Costs.LoadCandidateCosts(ctx, key, evidence, carried)
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrRouteCostSourceNotConfigured):
			notFormed := handler.notFormedParcel(key, ports.NotFormedCostSourceNotConfigured)
			return none, &notFormed, nil
		case errors.Is(err, ports.ErrRouteCostAnswerUnusable):
			return none, nil, fmt.Errorf("load candidate costs: %w", err)
		}
		undecided := handler.undecidedParcel(key, RouteCostSourceUnavailable)
		return none, &undecided, nil
	}
	evidence.CandidateCosts = costs.Facts

	ranking, err := domain.RankRouteCandidates(evidence.RankingForm, candidates, evidence.CandidateCosts)
	if err != nil {
		return none, nil, fmt.Errorf("rank route candidates: %w", err)
	}
	switch ranking.Outcome() {
	case domain.RankingSelected:
		selected, _ := ranking.Selected()
		return handler.planRecord(ctx, key, selected, candidates, evidence, costs.CitationsOf(selected), judgedAt, acceptedAt)
	case domain.RankingNoQualifiedCandidate:
		return handler.noRouteRecord(key, candidates, gaps, evidence, judgedAt)
	case domain.RankingTied:
		notFormed := handler.notFormedParcel(key, ports.NotFormedCandidatesTied)
		notFormed.candidates = append([]domain.RouteCandidate(nil), candidates...)
		notFormed.tied = ranking.Tied()
		notFormed.tiedCosts = costsOf(evidence.CandidateCosts, notFormed.tied)
		return none, &notFormed, nil
	case domain.RankingFormNotDeclared:
		notFormed := handler.notFormedParcel(key, ports.NotFormedRankingFormNotConfigured)
		return none, &notFormed, nil
	case domain.RankingCostsPending:
		notFormed := handler.notFormedParcel(key, ports.NotFormedCandidateCostsPending)
		return none, &notFormed, nil
	case domain.RankingCostsUnpriceable:
		notFormed := handler.notFormedParcel(key, ports.NotFormedCandidateCostsUnpriceable)
		return none, &notFormed, nil
	case domain.RankingCurrenciesDiffer:
		notFormed := handler.notFormedParcel(key, ports.NotFormedCandidateCostCurrenciesDiffer)
		return none, &notFormed, nil
	default:
		return none, nil, fmt.Errorf("network routing: unhandled ranking outcome %d", ranking.Outcome())
	}
}

// planRecord 为选中候选形成待提交的计划记录。选中候选的逐段成本出处随计划一并留痕
// （票 routing-first-cut/10）：审计回得到原币与汇率，不必重放整条取数链。
//
// 交进来的只是选中候选那一份。未选中候选的金额作为候选成本事实参与了排序，出处却不随计划留痕：计划的出处只按段号
// 对段链，没有候选维（票 demo-chain-gaps/105）。
//
// 计划从委托接受的时刻起生效，判断时刻另记处理时钟（CONTEXT「路由计划适用性」）。生效取处理时钟时，路由交接一积压，
// 此后业务时间落在接受与处理之间的控制触发让计划失效或被替代，离场早于生效，被领域离场门拒、整封信封响亮失败；复核形成
// 的计划同样按触发的业务时间生效（ADR-0272），两段接得上。
func (handler *CreateInitialRouteHandler) planRecord(
	ctx context.Context,
	key domain.InitialRouteJudgmentKey,
	selected domain.CandidateID,
	candidates []domain.RouteCandidate,
	evidence ports.InitialRouteEvidence,
	costCitations []domain.PlannedLegCostCitation,
	judgedAt time.Time,
	acceptedAt time.Time,
) (ports.InitialRouteRecord, *ParcelRouteResult, error) {
	none := ports.InitialRouteRecord{}
	var legs []domain.PlannedLeg
	for _, path := range evidence.Paths {
		if path.Candidate == selected {
			legs = path.PlanLegs()
			break
		}
	}
	if len(legs) == 0 {
		// 选中的候选没有段链，计划的节点与窗口无从表达——答复缺件，不是一种未决。
		return none, nil, ErrIncompleteRouteEvidence
	}
	version, err := handler.deps.Identities.NextRoutePlanVersionID(ctx)
	if err != nil {
		undecided := handler.undecidedParcel(key, RouteIdentityUnavailable)
		return none, &undecided, nil
	}
	plan, err := domain.FormInitialRoutePlan(domain.InitialRoutePlanSpec{
		Key:              key,
		Version:          version,
		Selected:         selected,
		Candidates:       candidates,
		Legs:             legs,
		Strategy:         evidence.Strategy,
		ViewRevision:     evidence.ViewRevision,
		JudgedAt:         judgedAt,
		EffectiveFrom:    acceptedAt,
		CostCitations:    costCitations,
		CustomsCitations: evidence.CustomsCitations,
	})
	if err != nil {
		return none, nil, fmt.Errorf("form initial route plan: %w", err)
	}
	return ports.InitialRouteRecord{Key: key, Plan: plan, HasPlan: true}, nil, nil
}

// noRouteRecord 在没有合格候选时区分两格：全部确定性淘汰是`无当前有效路由`，还有
// 证据未知候选在场只能未决（「必须证明不存在……证据未知……的可能候选」）。
func (handler *CreateInitialRouteHandler) noRouteRecord(
	key domain.InitialRouteJudgmentKey,
	candidates []domain.RouteCandidate,
	gaps []domain.EvidenceGap,
	evidence ports.InitialRouteEvidence,
	judgedAt time.Time,
) (ports.InitialRouteRecord, *ParcelRouteResult, error) {
	none := ports.InitialRouteRecord{}
	judgment, err := domain.FormNoCurrentRouteJudgment(domain.NoCurrentRouteJudgmentSpec{
		Key:          key,
		Candidates:   candidates,
		Strategy:     evidence.Strategy,
		ViewRevision: evidence.ViewRevision,
		JudgedAt:     judgedAt,
	})
	if errors.Is(err, domain.ErrCandidateSpaceUndecided) {
		notFormed := handler.notFormedParcel(key, ports.NotFormedCandidateEvidenceIncomplete)
		notFormed.candidates = append([]domain.RouteCandidate(nil), candidates...)
		notFormed.gaps = gaps
		return none, &notFormed, nil
	}
	if err != nil {
		return none, nil, fmt.Errorf("form no-current-route judgment: %w", err)
	}
	return ports.InitialRouteRecord{Key: key, NoRoute: judgment, HasNoRoute: true}, nil, nil
}

// commit 提交一份包裹级记录并交发布意图。并发下另一方先提交时读回赢家（`AT-NR-004`）。
func (handler *CreateInitialRouteHandler) commit(
	ctx context.Context,
	correlation domain.RequestCorrelationID,
	key domain.InitialRouteJudgmentKey,
	record ports.InitialRouteRecord,
) (ParcelRouteResult, error) {
	saved, err := handler.deps.Store.Save(ctx, record)
	if err != nil {
		return handler.undecidedParcel(key, RouteStoreUnavailable), nil
	}
	switch saved {
	case ports.InitialRouteSaved:
		result := resultFor(key, record)
		result.handoff = handler.handOff(ctx, correlation, record)
		return result, nil
	case ports.InitialRouteAlreadyRecorded:
		winner, found, err := handler.deps.Store.FindByKey(ctx, key)
		if errors.Is(err, ports.ErrStoredRouteRecordUnreadable) {
			return ParcelRouteResult{}, fmt.Errorf("load concurrent initial route: %w", err)
		}
		if err != nil || !found {
			return handler.undecidedParcel(key, RouteStoreUnavailable), nil
		}
		return handler.existingResult(ctx, correlation, key, winner), nil
	default:
		return ParcelRouteResult{}, fmt.Errorf("%w: %d", ErrUnexpectedRouteSaveOutcome, saved)
	}
}

// existingResult 按已有记录作答并重发同一份意图（重放不重判，但下游可能还没收到）。
func (handler *CreateInitialRouteHandler) existingResult(
	ctx context.Context,
	correlation domain.RequestCorrelationID,
	key domain.InitialRouteJudgmentKey,
	record ports.InitialRouteRecord,
) ParcelRouteResult {
	result := resultFor(key, record)
	result.outcome = ParcelExistingResult
	result.handoff = handler.handOff(ctx, correlation, record)
	return result
}

func resultFor(key domain.InitialRouteJudgmentKey, record ports.InitialRouteRecord) ParcelRouteResult {
	result := ParcelRouteResult{key: key}
	if record.HasPlan {
		result.outcome = ParcelRouteFormed
		result.plan = record.Plan
		result.hasPlan = true
	} else {
		result.outcome = ParcelNoCurrentRoute
		result.noRoute = record.NoRoute
		result.hasNoRoute = true
	}
	return result
}

// handOff 交发布意图。失败不翻结果：结果已越过提交边界，交不出去留续办引用重发同一份
// （ADR-0043 的重试纪律）。
func (handler *CreateInitialRouteHandler) handOff(
	ctx context.Context,
	correlation domain.RequestCorrelationID,
	record ports.InitialRouteRecord,
) ContinuationReference {
	err := handler.deps.Downstream.HandOffInitialRoute(ctx, ports.InitialRouteHandoffIntent{
		Correlation: correlation,
		Record:      record,
	})
	if err == nil {
		return ContinuationReference{}
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{
		"INITIAL_ROUTE_HANDOFF",
		record.Key.TenantID.String(),
		record.Key.ShipmentRequestID.String(),
		record.Key.DeclaredParcelID.String(),
		record.Key.AcceptanceBaseline.String(),
	}, "\x00")))
	return ContinuationReference{value: "CONT-" + hex.EncodeToString(digest[:8])}
}

// undecidedParcel 让一个包裹停在未决并带上续办引用；派生纪律与可达性一致：同一键同一
// 原因引用恒同。
func (handler *CreateInitialRouteHandler) undecidedParcel(
	key domain.InitialRouteJudgmentKey,
	reason RouteUndecidedReason,
) ParcelRouteResult {
	return ParcelRouteResult{
		key:          key,
		outcome:      ParcelRouteUndecided,
		reason:       reason,
		continuation: routeContinuation(key, reason.String()),
	}
}

// notFormedParcel 让一个包裹停在`路由未形成`。原因此刻还没落库：Handle 逐件判完之后经 recordNotFormed 落登记，落不下
// 就改回未决。
func (handler *CreateInitialRouteHandler) notFormedParcel(
	key domain.InitialRouteJudgmentKey,
	reason ports.InitialRouteNotFormedReason,
) ParcelRouteResult {
	return ParcelRouteResult{
		key:          key,
		outcome:      ParcelRouteNotFormed,
		notFormed:    reason,
		continuation: routeContinuation(key, reason.String()),
	}
}

// recordNotFormed 把一件`路由未形成`的原因与对象落进未形成登记（ADR-0273 决定二）。原因没落库不得入账：写不进时改答
// 未决，整封回滚重投。
func (handler *CreateInitialRouteHandler) recordNotFormed(
	ctx context.Context,
	correlation domain.RequestCorrelationID,
	acceptedAt time.Time,
	result ParcelRouteResult,
) (ParcelRouteResult, error) {
	if handler.deps.NotFormed == nil {
		return ParcelRouteResult{}, fmt.Errorf("%w: parcel %q stopped at %s",
			ErrNotFormedRegisterMissing, result.key.DeclaredParcelID, result.notFormed)
	}
	err := handler.deps.NotFormed.RecordNotFormed(ctx, ports.InitialRouteNotFormed{
		Key:         result.key,
		Correlation: correlation,
		Reason:      result.notFormed,
		Candidates:  result.Candidates(),
		Tied:        result.TiedCandidates(),
		TiedCosts:   result.TiedCosts(),
		Gaps:        result.EvidenceGaps(),
		AcceptedAt:  acceptedAt,
		SettledAt:   handler.deps.Clock.Now(),
	})
	if err != nil {
		return handler.undecidedParcel(result.key, RouteNotFormedUnrecorded), nil
	}
	return result, nil
}

// costsOf 取出指名那几家候选的成本事实，序照指名的序。
func costsOf(facts []domain.CandidateCostFact, candidates []domain.CandidateID) []domain.CandidateCostFact {
	out := make([]domain.CandidateCostFact, 0, len(candidates))
	for _, candidate := range candidates {
		for _, fact := range facts {
			if fact.Candidate() == candidate {
				out = append(out, fact)
				break
			}
		}
	}
	return out
}

// undecidedHandoff 让整份交接停在未决：日志或适用性读不回时，逐包裹都还没被判断过。
func (handler *CreateInitialRouteHandler) undecidedHandoff(
	keys []domain.InitialRouteJudgmentKey,
	reason RouteUndecidedReason,
) CreateInitialRouteResult {
	parcels := make([]ParcelRouteResult, 0, len(keys))
	for _, key := range keys {
		parcels = append(parcels, handler.undecidedParcel(key, reason))
	}
	return CreateInitialRouteResult{
		outcome: RouteHandoffUndecided,
		parcels: parcels,
		reason:  reason,
	}
}

func routeContinuation(key domain.InitialRouteJudgmentKey, reason string) ContinuationReference {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		reason,
		key.TenantID.String(),
		key.CustomerAccountID.String(),
		key.ShipmentRequestID.String(),
		key.AcceptanceBaseline.String(),
		key.DeclaredParcelID.String(),
		key.ServicePurpose.String(),
	}, "\x00")))
	return ContinuationReference{value: "CONT-" + hex.EncodeToString(digest[:8])}
}

// handoffDigest 是交接内容的稳定指纹：同关联异指纹即冲突。成员序不参与——同一批成员
// 换个顺序不是另一份交接。
func handoffDigest(spec domain.RouteHandoffSpec, geo domain.GeoResolutionProjection) string {
	members := make([]string, 0, len(spec.Parcels))
	for _, parcel := range spec.Parcels {
		members = append(members, parcel.String())
	}
	sortStrings(members)
	payload := strings.Join([]string{
		spec.TenantID.String(),
		spec.CustomerAccountID.String(),
		spec.ShipmentRequestID.String(),
		spec.AcceptanceDecision.String(),
		spec.AcceptanceBaseline.String(),
		strings.Join(members, ","),
		geo.ContentDigest(),
	}, "\x00")
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}

func sortStrings(values []string) {
	for outer := 1; outer < len(values); outer++ {
		for inner := outer; inner > 0 && values[inner] < values[inner-1]; inner-- {
			values[inner], values[inner-1] = values[inner-1], values[inner]
		}
	}
}
