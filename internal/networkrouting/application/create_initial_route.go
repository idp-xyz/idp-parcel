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

// ParcelRouteOutcome 是一个包裹的处理结果。`无当前有效路由`是领域判断、`未决`是应用
// 结果，两者与`已形成`三分——用例明写三者不能合并为一个通用失败状态。
type ParcelRouteOutcome uint8

const (
	ParcelRouteOutcomeInvalid ParcelRouteOutcome = iota
	ParcelRouteFormed
	ParcelNoCurrentRoute
	ParcelRouteUndecided
	ParcelExistingResult
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
	default:
		return ""
	}
}

// RouteUndecidedReason 指名一个包裹为何停在未决。封闭集合，按依赖阶段分类统计。
type RouteUndecidedReason uint8

const (
	RouteUndecidedReasonNone RouteUndecidedReason = iota
	RouteStoreUnavailable
	RouteEvidenceUnavailable
	// RouteEvidenceNotConfigured 是网络定义登记册对这个范围未配置（ADR-0052）。它与
	// RouteEvidenceUnavailable 分格，因为恢复动作相反：未配置要租户去登记网络定义，
	// 不可用要运维去救依赖。也绝不译成`无当前有效路由`——那是领域从「全部候选确定性
	// 淘汰」得出的判断，而这里是还没人说过网络长什么样。
	RouteEvidenceNotConfigured
	RouteCandidateSpaceNotEstablished
	RouteCandidateEvidenceIncomplete
	RouteIdentityUnavailable
	RouteHandoffLogUnavailable
	RoutingApplicabilityUnavailable
	RouteEvidenceSuperseded
	// RouteRankingFormNotConfigured 是路由策略版本没有声明排序形态：等租户选形态（ADR-0146），
	// 与证据未配置同属「等登记」，但要登记的是另一样东西。
	RouteRankingFormNotConfigured
	// RouteCandidatesTied 是最低成本并列、选不出唯一一条（`PAR-NET-16`）：候选可行、等授权角色
	// 裁，所以既不是计划也不是`无当前有效路由`。
	RouteCandidatesTied
	// RouteCandidateCostsNotPriced 是合格候选全部缺成本事实（待判断或不可计价）而无一可比。
	RouteCandidateCostsNotPriced
	RouteCandidateCostCurrenciesDiffer
)

func (reason RouteUndecidedReason) String() string {
	switch reason {
	case RouteStoreUnavailable:
		return "ROUTE_STORE_UNAVAILABLE"
	case RouteEvidenceUnavailable:
		return "ROUTE_EVIDENCE_UNAVAILABLE"
	case RouteEvidenceNotConfigured:
		return "ROUTE_EVIDENCE_NOT_CONFIGURED"
	case RouteCandidateSpaceNotEstablished:
		return "CANDIDATE_SPACE_NOT_ESTABLISHED"
	case RouteCandidateEvidenceIncomplete:
		return "CANDIDATE_EVIDENCE_INCOMPLETE"
	case RouteIdentityUnavailable:
		return "ROUTE_IDENTITY_UNAVAILABLE"
	case RouteHandoffLogUnavailable:
		return "HANDOFF_LOG_UNAVAILABLE"
	case RoutingApplicabilityUnavailable:
		return "ROUTING_APPLICABILITY_UNAVAILABLE"
	case RouteEvidenceSuperseded:
		return "ROUTE_EVIDENCE_SUPERSEDED"
	case RouteRankingFormNotConfigured:
		return "RANKING_FORM_NOT_CONFIGURED"
	case RouteCandidatesTied:
		return "CANDIDATES_TIED"
	case RouteCandidateCostsNotPriced:
		return "CANDIDATE_COSTS_NOT_PRICED"
	case RouteCandidateCostCurrenciesDiffer:
		return "CANDIDATE_COST_CURRENCIES_DIFFER"
	default:
		return ""
	}
}

// CreateInitialRouteCommand 携带路由交接的原料、服务目的和已接受解析标识。
// 目的是消费侧装配参数（与可达性适配器的 Purpose 同款）：属服务产品的话语，未配置时
// 交接未受理。解析标识是命令附加字段，不是判断维（ADR-0064）。
type CreateInitialRouteCommand struct {
	Handoff    domain.RouteHandoffSpec
	Purpose    domain.ServicePurpose
	Resolution domain.CommercialResolutionReference
}

// ParcelRouteResult 是一个包裹的独立结果。计划与无路由各按在场标志给出，未决带原因与
// 续办引用；并列的未决另带全部候选与并列的那几家，授权角色裁的时候要看得见在哪几家之间裁。
type ParcelRouteResult struct {
	key          domain.InitialRouteJudgmentKey
	outcome      ParcelRouteOutcome
	plan         domain.InitialRoutePlan
	hasPlan      bool
	noRoute      domain.NoCurrentRouteJudgment
	hasNoRoute   bool
	reason       RouteUndecidedReason
	continuation ContinuationReference
	handoff      ContinuationReference
	candidates   []domain.RouteCandidate
	tied         []domain.CandidateID
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

func (result ParcelRouteResult) ContinuationReference() ContinuationReference {
	return result.continuation
}

// RouteHandoffReference 非空说明结果已提交但意图还没交出去，重放会重发同一份。
func (result ParcelRouteResult) RouteHandoffReference() ContinuationReference {
	return result.handoff
}

// Candidates 只在`候选并列`时给出：本轮评估过的全部候选及各自的选择或淘汰依据。
func (result ParcelRouteResult) Candidates() []domain.RouteCandidate {
	return append([]domain.RouteCandidate(nil), result.candidates...)
}

func (result ParcelRouteResult) TiedCandidates() []domain.CandidateID {
	return append([]domain.CandidateID(nil), result.tied...)
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
	Store         ports.InitialRouteStore
	Log           ports.RouteHandoffLog
	Downstream    ports.InitialRouteHandoff
	Identities    ports.RouteIdentityFactory
	Clock         ports.Clock
}

type CreateInitialRouteHandler struct {
	deps CreateInitialRouteDeps
}

func NewCreateInitialRouteHandler(deps CreateInitialRouteDeps) *CreateInitialRouteHandler {
	return &CreateInitialRouteHandler{deps: deps}
}

// Handle 把一份委托接受交接推进到逐包裹结果：受理 → 重放/冲突分界 → 适用性 → 逐包裹
// 「已有结果 / 评估管线 / 提交 / 发布意图」。单个包裹未决或无路由不回滚其他包裹已经
// 合法形成的计划（`AT-NR-012`），也不被一个委托级状态掩盖。
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
	digest := handoffDigest(command.Handoff)
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
		result, err := handler.routeOneParcel(ctx, handoff.Correlation(), key)
		if err != nil {
			return CreateInitialRouteResult{}, err
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
func (handler *CreateInitialRouteHandler) routeOneParcel(
	ctx context.Context,
	correlation domain.RequestCorrelationID,
	key domain.InitialRouteJudgmentKey,
) (ParcelRouteResult, error) {
	existing, found, err := handler.deps.Store.FindByKey(ctx, key)
	if err != nil {
		return handler.undecidedParcel(key, RouteStoreUnavailable), nil
	}
	if found {
		return handler.existingResult(ctx, correlation, key, existing), nil
	}

	evidence, configured, err := handler.deps.Evidence.LoadInitialRouteEvidence(ctx, key)
	if err != nil {
		return handler.undecidedParcel(key, RouteEvidenceUnavailable), nil
	}
	if !configured {
		return handler.undecidedParcel(key, RouteEvidenceNotConfigured), nil
	}

	for attempt := 0; attempt < 2; attempt++ {
		record, undecided, err := handler.judgeParcel(ctx, key, evidence)
		if err != nil {
			return ParcelRouteResult{}, err
		}
		if undecided != nil {
			return *undecided, nil
		}

		fresh, configured, err := handler.deps.Evidence.LoadInitialRouteEvidence(ctx, key)
		if err != nil {
			return handler.undecidedParcel(key, RouteEvidenceUnavailable), nil
		}
		if !configured {
			// 判断中途登记册被撤下：重校对不出修订，提交这一版等于拿一份已经没有出处
			// 的证据定案。
			return handler.undecidedParcel(key, RouteEvidenceNotConfigured), nil
		}
		if fresh.ViewRevision == evidence.ViewRevision {
			return handler.commit(ctx, correlation, key, record)
		}
		evidence = fresh
	}
	return handler.undecidedParcel(key, RouteEvidenceSuperseded), nil
}

// judgeParcel 以一份证据执行评估管线并形成待提交记录。三种出口：记录、未决、装配错误。
func (handler *CreateInitialRouteHandler) judgeParcel(
	ctx context.Context,
	key domain.InitialRouteJudgmentKey,
	evidence ports.InitialRouteEvidence,
) (ports.InitialRouteRecord, *ParcelRouteResult, error) {
	none := ports.InitialRouteRecord{}
	if !evidence.ViewRevision.Valid() || !evidence.Strategy.Valid() {
		return none, nil, ErrIncompleteRouteEvidence
	}

	// 评估在领域执行（ADR-0046），按层次推进：区域 → 承诺分界 → 可执行性 → 硬约束 →
	// 时间可行性。事实本身不成立是端口坏了，响亮上抛。
	candidates, _, err := domain.EvaluateServiceAreas(evidence.ServiceAreas)
	if err != nil {
		return none, nil, fmt.Errorf("evaluate service areas: %w", err)
	}
	if len(candidates) == 0 {
		undecided := handler.undecidedParcel(key, RouteCandidateSpaceNotEstablished)
		return none, &undecided, nil
	}
	candidates, err = domain.EvaluateRouteRequirements(candidates, evidence.RouteRequirements)
	if err != nil {
		return none, nil, fmt.Errorf("evaluate route requirements: %w", err)
	}
	candidates, err = domain.EvaluatePathExecutability(candidates, evidence.PathExecutability)
	if err != nil {
		return none, nil, fmt.Errorf("evaluate path executability: %w", err)
	}
	candidates, _, err = domain.EvaluateHardConstraints(candidates, evidence.HardConstraints)
	if err != nil {
		return none, nil, fmt.Errorf("evaluate hard constraints: %w", err)
	}
	candidates, err = domain.EvaluateTimeFeasibility(candidates, evidence.Projections, evidence.CommittedBound)
	if err != nil {
		return none, nil, fmt.Errorf("evaluate time feasibility: %w", err)
	}

	judgedAt := handler.deps.Clock.Now()
	ranking, err := domain.RankRouteCandidates(evidence.RankingForm, candidates, evidence.CandidateCosts)
	if err != nil {
		return none, nil, fmt.Errorf("rank route candidates: %w", err)
	}
	switch ranking.Outcome() {
	case domain.RankingSelected:
		selected, _ := ranking.Selected()
		return handler.planRecord(ctx, key, selected, candidates, evidence, judgedAt)
	case domain.RankingNoQualifiedCandidate:
		return handler.noRouteRecord(key, candidates, evidence, judgedAt)
	case domain.RankingTied:
		undecided := handler.undecidedParcel(key, RouteCandidatesTied)
		undecided.candidates = append([]domain.RouteCandidate(nil), candidates...)
		undecided.tied = ranking.Tied()
		return none, &undecided, nil
	case domain.RankingFormNotDeclared:
		undecided := handler.undecidedParcel(key, RouteRankingFormNotConfigured)
		return none, &undecided, nil
	case domain.RankingNoPricedCandidate:
		undecided := handler.undecidedParcel(key, RouteCandidateCostsNotPriced)
		return none, &undecided, nil
	case domain.RankingCurrenciesDiffer:
		undecided := handler.undecidedParcel(key, RouteCandidateCostCurrenciesDiffer)
		return none, &undecided, nil
	default:
		return none, nil, fmt.Errorf("network routing: unhandled ranking outcome %d", ranking.Outcome())
	}
}

// planRecord 为选中候选形成待提交的计划记录。
func (handler *CreateInitialRouteHandler) planRecord(
	ctx context.Context,
	key domain.InitialRouteJudgmentKey,
	selected domain.CandidateID,
	candidates []domain.RouteCandidate,
	evidence ports.InitialRouteEvidence,
	judgedAt time.Time,
) (ports.InitialRouteRecord, *ParcelRouteResult, error) {
	none := ports.InitialRouteRecord{}
	var legs []domain.PlannedLeg
	for _, path := range evidence.Paths {
		if path.Candidate == selected {
			legs = path.Legs
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
		Key:           key,
		Version:       version,
		Selected:      selected,
		Candidates:    candidates,
		Legs:          legs,
		Strategy:      evidence.Strategy,
		ViewRevision:  evidence.ViewRevision,
		JudgedAt:      judgedAt,
		EffectiveFrom: judgedAt,
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
		undecided := handler.undecidedParcel(key, RouteCandidateEvidenceIncomplete)
		return none, &undecided, nil
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
		continuation: routeContinuation(key, reason),
	}
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

func routeContinuation(key domain.InitialRouteJudgmentKey, reason RouteUndecidedReason) ContinuationReference {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		reason.String(),
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
func handoffDigest(spec domain.RouteHandoffSpec) string {
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
