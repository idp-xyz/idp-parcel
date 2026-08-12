package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// ErrUnexpectedReassessmentSave 说明复核库交回了封闭集合以外的写入结果。
var ErrUnexpectedReassessmentSave = errors.New("network routing: unexpected reassessment save outcome")

// ReassessOutcome 是复核请求的应用处理结果。三种领域走向（仍适用/已失效/首个计划）各占
// 一格，未决、已有结果、冲突与未受理是应用格——用例明写这些结果不能合并为通用失败。
type ReassessOutcome uint8

const (
	ReassessOutcomeInvalid ReassessOutcome = iota
	ReassessedStillApplicable
	ReassessedPlanLapsed
	ReassessedFirstPlanFormed
	ReassessUndecided
	ReassessExistingResult
	ReassessTriggerConflict
	ReassessTriggerNotAccepted
)

func (outcome ReassessOutcome) String() string {
	switch outcome {
	case ReassessedStillApplicable:
		return "STILL_APPLICABLE"
	case ReassessedPlanLapsed:
		return "PLAN_LAPSED"
	case ReassessedFirstPlanFormed:
		return "FIRST_PLAN_FORMED"
	case ReassessUndecided:
		return "UNDECIDED"
	case ReassessExistingResult:
		return "EXISTING_RESULT"
	case ReassessTriggerConflict:
		return "TRIGGER_CONFLICT"
	case ReassessTriggerNotAccepted:
		return "TRIGGER_NOT_ACCEPTED"
	default:
		return ""
	}
}

// ReassessUndecidedReason 指名复核停在哪一步。封闭集合，按依赖阶段分类统计。
type ReassessUndecidedReason uint8

const (
	ReassessUndecidedReasonNone ReassessUndecidedReason = iota
	ReassessLogUnavailable
	RoutingHistoryUnavailable
	NoRoutingHistory
	ReassessEvidenceUnavailable
	PlanReviewInconclusive
	ApplicabilityStoreUnavailable
	ReassessStoreUnavailable
	ReassessIdentityUnavailable
)

func (reason ReassessUndecidedReason) String() string {
	switch reason {
	case ReassessLogUnavailable:
		return "REASSESS_LOG_UNAVAILABLE"
	case RoutingHistoryUnavailable:
		return "ROUTING_HISTORY_UNAVAILABLE"
	case NoRoutingHistory:
		return "NO_ROUTING_HISTORY"
	case ReassessEvidenceUnavailable:
		return "REASSESS_EVIDENCE_UNAVAILABLE"
	case PlanReviewInconclusive:
		return "PLAN_REVIEW_INCONCLUSIVE"
	case ApplicabilityStoreUnavailable:
		return "APPLICABILITY_STORE_UNAVAILABLE"
	case ReassessStoreUnavailable:
		return "REASSESS_STORE_UNAVAILABLE"
	case ReassessIdentityUnavailable:
		return "REASSESS_IDENTITY_UNAVAILABLE"
	default:
		return ""
	}
}

type ReassessRouteCommand struct {
	Trigger domain.ReassessmentTriggerSpec
}

type ReassessRouteResult struct {
	outcome      ReassessOutcome
	record       ports.ReassessmentRecord
	hasRecord    bool
	reason       ReassessUndecidedReason
	continuation ContinuationReference
}

func (result ReassessRouteResult) Outcome() ReassessOutcome {
	return result.outcome
}

// Record 只在越过提交边界（或找回已有结果）时给出。
func (result ReassessRouteResult) Record() (ports.ReassessmentRecord, bool) {
	return result.record, result.hasRecord
}

func (result ReassessRouteResult) UndecidedReason() ReassessUndecidedReason {
	return result.reason
}

func (result ReassessRouteResult) ContinuationReference() ContinuationReference {
	return result.continuation
}

type ReassessRouteDeps struct {
	Routes        ports.InitialRouteStore
	Evidence      ports.InitialRouteEvidenceView
	Applicability ports.PlanApplicabilityStore
	Store         ports.ReassessmentStore
	Log           ports.RouteHandoffLog
	Identities    ports.RouteIdentityFactory
	Clock         ports.Clock
}

type ReassessRouteHandler struct {
	deps ReassessRouteDeps
}

func NewReassessRouteHandler(deps ReassessRouteDeps) *ReassessRouteHandler {
	return &ReassessRouteHandler{deps: deps}
}

// Handle 把一次权威触发推进到复核结论：触发受理（线索在构造期就被拒）→ 重复/冲突分界
// → 载入历史结果（没有历史不得凭空假定初始计划）→ 两条独立判断线（6A 计划适用性、
// 6B 替代候选）→ 提交。失效即失效：候选评估未决拦不住失效落库，三件并存（硬句）。
func (handler *ReassessRouteHandler) Handle(
	ctx context.Context,
	command ReassessRouteCommand,
) (ReassessRouteResult, error) {
	trigger, err := domain.NewReassessmentTrigger(command.Trigger)
	if err != nil {
		return ReassessRouteResult{outcome: ReassessTriggerNotAccepted}, nil
	}
	key := trigger.Key()

	digest := triggerDigest(trigger)
	recorded, found, err := handler.deps.Log.FindDigest(ctx, key.TenantID, trigger.Correlation())
	if err != nil {
		return handler.undecided(key, ReassessLogUnavailable), nil
	}
	if found && recorded != digest {
		return ReassessRouteResult{outcome: ReassessTriggerConflict}, nil
	}
	if found {
		existing, present, err := handler.deps.Store.FindByCorrelation(ctx, key.TenantID, trigger.Correlation())
		if err != nil || !present {
			return handler.undecided(key, ReassessStoreUnavailable), nil
		}
		return ReassessRouteResult{
			outcome:   outcomeFor(existing),
			record:    existing,
			hasRecord: true,
		}, nil
	}
	if err := handler.deps.Log.Append(ctx, key.TenantID, trigger.Correlation(), digest); err != nil {
		return handler.undecided(key, ReassessLogUnavailable), nil
	}

	history, found, err := handler.deps.Routes.FindByKey(ctx, key)
	if err != nil {
		return handler.undecided(key, RoutingHistoryUnavailable), nil
	}
	if !found {
		// 「没有历史结果时不得凭空假定初始计划」——复核复的是一份已有判断。
		return handler.undecided(key, NoRoutingHistory), nil
	}

	evidence, err := handler.deps.Evidence.LoadInitialRouteEvidence(ctx, key)
	if err != nil {
		return handler.undecided(key, ReassessEvidenceUnavailable), nil
	}
	if !evidence.ViewRevision.Valid() || !evidence.Strategy.Valid() {
		return ReassessRouteResult{}, ErrIncompleteRouteEvidence
	}

	if history.HasPlan {
		return handler.reassessPlan(ctx, trigger, history.Plan, evidence)
	}
	return handler.reassessNoRoute(ctx, trigger, evidence)
}

// reassessPlan 是原来有计划的那条线：6A 独立判定，仍适用即保留、失效即失效（候选状态
// 另存）、看不清停未决。
func (handler *ReassessRouteHandler) reassessPlan(
	ctx context.Context,
	trigger domain.ReassessmentTrigger,
	plan domain.InitialRoutePlan,
	evidence ports.InitialRouteEvidence,
) (ReassessRouteResult, error) {
	key := trigger.Key()
	review, err := domain.ReviewPlanApplicability(plan, trigger.Location(), evidence.HardConstraints)
	if err != nil {
		return ReassessRouteResult{}, fmt.Errorf("review plan applicability: %w", err)
	}

	switch review.Outcome() {
	case domain.PlanReviewUndecided:
		// 适用性本身没有足够依据：不动计划、不动适用性，未决续办。
		return handler.undecided(key, PlanReviewInconclusive), nil

	case domain.PlanStillApplicable:
		record := ports.ReassessmentRecord{
			Correlation:  trigger.Correlation(),
			Key:          key,
			Conclusion:   ports.ReassessmentStillApplicable,
			ReviewedPlan: plan.Version(),
			ReassessedAt: handler.deps.Clock.Now(),
		}
		return handler.commit(ctx, trigger, record)

	case domain.PlanNoLongerApplicable:
		basis, _ := review.LapseBasis()
		applicability, err := handler.loadApplicability(ctx, plan)
		if err != nil {
			return handler.undecided(key, ApplicabilityStoreUnavailable), nil
		}
		lapsed, err := applicability.Lapse(basis, trigger.OccurredAt())
		if err != nil {
			// 已经离场的计划再失效一次是重复触发撞上并发赢家，按已有结果读回。
			if errors.Is(err, domain.ErrPlanNoLongerCurrent) {
				existing, present, findErr := handler.deps.Store.FindByCorrelation(ctx, key.TenantID, trigger.Correlation())
				if findErr == nil && present {
					return ReassessRouteResult{outcome: outcomeFor(existing), record: existing, hasRecord: true}, nil
				}
				return handler.undecided(key, ReassessStoreUnavailable), nil
			}
			return ReassessRouteResult{}, fmt.Errorf("lapse plan applicability: %w", err)
		}
		if err := handler.deps.Applicability.Save(ctx, lapsed); err != nil {
			return handler.undecided(key, ApplicabilityStoreUnavailable), nil
		}

		record := ports.ReassessmentRecord{
			Correlation:    trigger.Correlation(),
			Key:            key,
			Conclusion:     ports.ReassessmentPlanLapsed,
			ReviewedPlan:   plan.Version(),
			LapseBasis:     basis,
			CandidateState: handler.reviewCandidates(evidence),
			ReassessedAt:   handler.deps.Clock.Now(),
		}
		return handler.commit(ctx, trigger, record)
	default:
		return ReassessRouteResult{}, fmt.Errorf("network routing: unhandled review outcome %d", review.Outcome())
	}
}

// reassessNoRoute 是原先无路由的那条线：候选评估完成且有合格候选时形成首个当前有效
// 计划（CONTEXT 生命周期），未决则继续保持无路由并单独记录评估未决。
func (handler *ReassessRouteHandler) reassessNoRoute(
	ctx context.Context,
	trigger domain.ReassessmentTrigger,
	evidence ports.InitialRouteEvidence,
) (ReassessRouteResult, error) {
	key := trigger.Key()
	candidates, undecided, err := handler.evaluateCandidates(evidence)
	if err != nil {
		return ReassessRouteResult{}, err
	}
	if undecided {
		record := ports.ReassessmentRecord{
			Correlation:    trigger.Correlation(),
			Key:            key,
			Conclusion:     ports.ReassessmentPlanLapsed,
			CandidateState: ports.CandidateReviewUndecided,
			ReassessedAt:   handler.deps.Clock.Now(),
		}
		// 原先就无当前有效路由：结论仍是无路由，候选评估未决单独记录。
		return handler.commit(ctx, trigger, record)
	}

	selected, err := domain.SelectRouteCandidate(candidates, evidence.Scores, evidence.Priority)
	if errors.Is(err, domain.ErrNoQualifiedCandidate) {
		record := ports.ReassessmentRecord{
			Correlation:    trigger.Correlation(),
			Key:            key,
			Conclusion:     ports.ReassessmentPlanLapsed,
			CandidateState: ports.NoQualifiedCandidates,
			ReassessedAt:   handler.deps.Clock.Now(),
		}
		return handler.commit(ctx, trigger, record)
	}
	if err != nil {
		return ReassessRouteResult{}, fmt.Errorf("select route candidate: %w", err)
	}

	var legs []domain.PlannedLeg
	for _, path := range evidence.Paths {
		if path.Candidate == selected {
			legs = path.Legs
			break
		}
	}
	if len(legs) == 0 {
		return ReassessRouteResult{}, ErrIncompleteRouteEvidence
	}
	version, err := handler.deps.Identities.NextRoutePlanVersionID(ctx)
	if err != nil {
		return handler.undecided(key, ReassessIdentityUnavailable), nil
	}
	judgedAt := handler.deps.Clock.Now()
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
		return ReassessRouteResult{}, fmt.Errorf("form first current plan: %w", err)
	}
	record := ports.ReassessmentRecord{
		Correlation:    trigger.Correlation(),
		Key:            key,
		Conclusion:     ports.ReassessmentFirstPlanFormed,
		CandidateState: ports.CandidatesAvailable,
		NewPlan:        plan,
		HasNewPlan:     true,
		ReassessedAt:   judgedAt,
	}
	return handler.commit(ctx, trigger, record)
}

// evaluateCandidates 跑 6B 的评估管线到候选空间收敛。第二个返回值为真表示空间里还有
// 证据未知候选——评估未决。
func (handler *ReassessRouteHandler) evaluateCandidates(
	evidence ports.InitialRouteEvidence,
) ([]domain.RouteCandidate, bool, error) {
	candidates, _, err := domain.EvaluateServiceAreas(evidence.ServiceAreas)
	if err != nil {
		return nil, false, fmt.Errorf("evaluate service areas: %w", err)
	}
	if len(candidates) == 0 {
		return nil, true, nil
	}
	candidates, err = domain.EvaluateRouteRequirements(candidates, evidence.RouteRequirements)
	if err != nil {
		return nil, false, fmt.Errorf("evaluate route requirements: %w", err)
	}
	candidates, err = domain.EvaluatePathExecutability(candidates, evidence.PathExecutability)
	if err != nil {
		return nil, false, fmt.Errorf("evaluate path executability: %w", err)
	}
	candidates, _, err = domain.EvaluateHardConstraints(candidates, evidence.HardConstraints)
	if err != nil {
		return nil, false, fmt.Errorf("evaluate hard constraints: %w", err)
	}
	candidates, err = domain.EvaluateTimeFeasibility(candidates, evidence.Projections, evidence.CommittedBound)
	if err != nil {
		return nil, false, fmt.Errorf("evaluate time feasibility: %w", err)
	}
	for _, candidate := range candidates {
		if candidate.Outcome() == domain.CandidateEvidenceUnknown {
			return candidates, true, nil
		}
	}
	return candidates, false, nil
}

// reviewCandidates 在失效路径上给出候选评估状态（另存，不拦失效）。
func (handler *ReassessRouteHandler) reviewCandidates(evidence ports.InitialRouteEvidence) ports.CandidateReviewState {
	candidates, undecided, err := handler.evaluateCandidates(evidence)
	if err != nil || undecided {
		return ports.CandidateReviewUndecided
	}
	if _, err := domain.SelectRouteCandidate(candidates, evidence.Scores, evidence.Priority); err != nil {
		return ports.NoQualifiedCandidates
	}
	return ports.CandidatesAvailable
}

// commit 提交复核记录。并发下另一方先提交时读回赢家。
func (handler *ReassessRouteHandler) commit(
	ctx context.Context,
	trigger domain.ReassessmentTrigger,
	record ports.ReassessmentRecord,
) (ReassessRouteResult, error) {
	key := trigger.Key()
	saved, err := handler.deps.Store.Save(ctx, trigger.Correlation(), record)
	if err != nil {
		return handler.undecided(key, ReassessStoreUnavailable), nil
	}
	switch saved {
	case ports.ReassessmentSaved:
		return ReassessRouteResult{outcome: outcomeFor(record), record: record, hasRecord: true}, nil
	case ports.ReassessmentAlreadyRecorded:
		winner, present, err := handler.deps.Store.FindByCorrelation(ctx, key.TenantID, trigger.Correlation())
		if err != nil || !present {
			return handler.undecided(key, ReassessStoreUnavailable), nil
		}
		return ReassessRouteResult{outcome: ReassessExistingResult, record: winner, hasRecord: true}, nil
	default:
		return ReassessRouteResult{}, fmt.Errorf("%w: %d", ErrUnexpectedReassessmentSave, saved)
	}
}

func (handler *ReassessRouteHandler) loadApplicability(
	ctx context.Context,
	plan domain.InitialRoutePlan,
) (domain.PlanApplicability, error) {
	existing, found, err := handler.deps.Applicability.FindByPlan(ctx, plan.Version())
	if err != nil {
		return domain.PlanApplicability{}, err
	}
	if found {
		return existing, nil
	}
	return domain.EstablishPlanApplicability(plan.Version(), plan.EffectiveFrom())
}

func (handler *ReassessRouteHandler) undecided(
	key domain.InitialRouteJudgmentKey,
	reason ReassessUndecidedReason,
) ReassessRouteResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		reason.String(),
		key.TenantID.String(),
		key.ShipmentRequestID.String(),
		key.DeclaredParcelID.String(),
		key.AcceptanceBaseline.String(),
	}, "\x00")))
	return ReassessRouteResult{
		outcome:      ReassessUndecided,
		reason:       reason,
		continuation: ContinuationReference{value: "CONT-" + hex.EncodeToString(digest[:8])},
	}
}

func outcomeFor(record ports.ReassessmentRecord) ReassessOutcome {
	switch record.Conclusion {
	case ports.ReassessmentStillApplicable:
		return ReassessedStillApplicable
	case ports.ReassessmentPlanLapsed:
		return ReassessedPlanLapsed
	case ports.ReassessmentFirstPlanFormed:
		return ReassessedFirstPlanFormed
	default:
		return ReassessOutcomeInvalid
	}
}

// triggerDigest 是触发内容的稳定指纹：同关联异指纹即触发冲突。
func triggerDigest(trigger domain.ReassessmentTrigger) string {
	key := trigger.Key()
	digest := sha256.Sum256([]byte(strings.Join([]string{
		key.TenantID.String(),
		key.ShipmentRequestID.String(),
		key.DeclaredParcelID.String(),
		key.AcceptanceBaseline.String(),
		trigger.Control().String(),
		trigger.Location().String(),
		trigger.SourceVersion().String(),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}
