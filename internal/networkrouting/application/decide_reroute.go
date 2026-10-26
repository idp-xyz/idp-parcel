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

// ErrUnsupportedRerouteSuggestion 说建议所在的那次复核不在失效线上。今天只有失效线形成建议；仍适用线形成建议时决定要替代
// 一份当前有效计划，那一格随仍适用线的建议一并定（ADR-0284 越权风险点 7）。走到这里是有了新的建议来处而决定口没跟上，响亮
// 报错，不折成哪一种业务答案。
var ErrUnsupportedRerouteSuggestion = errors.New("network routing: the reroute suggestion is not on the lapse line")

// ErrSuggestionBaselineInconsistent 说受理基线那一版读回来与建议的判断键对不上，或读成「不是当前已接受包裹」：建议的判断键
// 就落在那一条基线上，基线一经固定不再改写，对不上是不变量破了，重发不会变（ADR-0029）。
var ErrSuggestionBaselineInconsistent = errors.New("network routing: the acceptance baseline disagrees with the reroute suggestion")

// candidateOutsideTheSpace 是此刻的候选空间里已没有所择候选时答出的依据：没有评估结局可引，如实说它不在。
const candidateOutsideTheSpace = "NOT_IN_CANDIDATE_SPACE"

// DecideRerouteOutcome 是授权改路决定的封闭答复（ADR-0284 决定二）。除成立与未决外各格都是确定性答复，续办各不相同：已决定
// 去看原决定，建议不存在或过时去看建议册，不在建议之列另择一条，被限制淘汰等来源责任方解除，不合格另择，证据未知与证据未配置
// 等登记。合成一格，授权角色就分不出下一步做什么。
type DecideRerouteOutcome uint8

const (
	DecideRerouteOutcomeInvalid DecideRerouteOutcome = iota
	RerouteDecided
	RerouteSuggestionAlreadyDecided
	RerouteSuggestionNotFound
	RerouteSuggestionSuperseded
	RerouteCandidateNotSuggested
	RerouteCandidateRestricted
	RerouteCandidateNotQualified
	RerouteCandidateEvidenceUnknown
	RerouteEvidenceNotConfigured
	RerouteDecisionUndecided
)

func (outcome DecideRerouteOutcome) String() string {
	switch outcome {
	case RerouteDecided:
		return "REROUTE_DECIDED"
	case RerouteSuggestionAlreadyDecided:
		return "SUGGESTION_ALREADY_DECIDED"
	case RerouteSuggestionNotFound:
		return "SUGGESTION_NOT_FOUND"
	case RerouteSuggestionSuperseded:
		return "SUGGESTION_SUPERSEDED"
	case RerouteCandidateNotSuggested:
		return "CANDIDATE_NOT_SUGGESTED"
	case RerouteCandidateRestricted:
		return "CANDIDATE_RESTRICTED"
	case RerouteCandidateNotQualified:
		return "CANDIDATE_NOT_QUALIFIED"
	case RerouteCandidateEvidenceUnknown:
		return "CANDIDATE_EVIDENCE_UNKNOWN"
	case RerouteEvidenceNotConfigured:
		return "REROUTE_EVIDENCE_NOT_CONFIGURED"
	case RerouteDecisionUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// DecideRerouteUndecidedReason 只收重发推得动的格：依赖读不动、写不进、取号故障。
type DecideRerouteUndecidedReason uint8

const (
	DecideRerouteUndecidedReasonNone DecideRerouteUndecidedReason = iota
	DecisionReassessmentStoreUnavailable
	DecisionStoreUnavailable
	DecisionRoutingHistoryUnavailable
	DecisionAcceptedParcelUnreadable
	DecisionEvidenceUnavailable
	DecisionCostSourceUnavailable
	DecisionIdentityUnavailable
)

func (reason DecideRerouteUndecidedReason) String() string {
	switch reason {
	case DecisionReassessmentStoreUnavailable:
		return "REASSESSMENT_STORE_UNAVAILABLE"
	case DecisionStoreUnavailable:
		return "DECISION_STORE_UNAVAILABLE"
	case DecisionRoutingHistoryUnavailable:
		return "ROUTING_HISTORY_UNAVAILABLE"
	case DecisionAcceptedParcelUnreadable:
		return "ACCEPTED_PARCEL_UNREADABLE"
	case DecisionEvidenceUnavailable:
		return "REROUTE_EVIDENCE_UNAVAILABLE"
	case DecisionCostSourceUnavailable:
		return "REROUTE_COST_SOURCE_UNAVAILABLE"
	case DecisionIdentityUnavailable:
		return "REROUTE_IDENTITY_UNAVAILABLE"
	default:
		return ""
	}
}

// DecideRerouteCommand 是一次授权改路决定。租户与决定方取认证结果（ADR-0284 决定一）；包裹、建议所在复核的触发关联、所择
// 候选与依据由决定方给。
type DecideRerouteCommand struct {
	Tenant     domain.TenantID
	Parcel     domain.DeclaredParcelID
	Suggestion domain.RequestCorrelationID
	Candidate  domain.CandidateID
	Basis      domain.RerouteDecisionBasisReference
	DecidedBy  domain.AuthorizedRoleReference
}

type DecideRerouteResult struct {
	outcome         DecideRerouteOutcome
	decision        ports.AuthorizedRerouteDecision
	hasDecision     bool
	candidateReason string
	reason          DecideRerouteUndecidedReason
	continuation    ContinuationReference
}

func (result DecideRerouteResult) Outcome() DecideRerouteOutcome { return result.outcome }

// Decision 在成立与已决定时给出：前者是这一次形成的，后者是先成的那一份。
func (result DecideRerouteResult) Decision() (ports.AuthorizedRerouteDecision, bool) {
	return result.decision, result.hasDecision
}

// CandidateReason 在此刻重判不成立的三格给出所择候选的依据。
func (result DecideRerouteResult) CandidateReason() string { return result.candidateReason }

func (result DecideRerouteResult) UndecidedReason() DecideRerouteUndecidedReason {
	return result.reason
}

func (result DecideRerouteResult) ContinuationReference() ContinuationReference {
	return result.continuation
}

type DecideRerouteDeps struct {
	// Reassessments 读建议所在的那次复核，并以 LatestTriggerAt 取与复核同一把判断键锁。
	Reassessments ports.ReassessmentStore
	Decisions     ports.AuthorizedRerouteDecisionStore
	Applicability ports.PlanApplicabilityStore
	Baselines     ports.AcceptanceBaselineGeoSource
	Evidence      ports.InitialRouteEvidenceView
	Costs         ports.RouteCandidateCostSource
	Identities    ports.RouteIdentityFactory
	Clock         ports.Clock
}

// DecideRerouteHandler 是授权角色据改路建议形成改路决定的编排（UC-NR-003 步骤 7C；ADR-0284）。事务边界归装配点：判断键锁随
// 事务释放，读已有决定、判过时、重判与落册要在同一笔里。
type DecideRerouteHandler struct {
	deps DecideRerouteDeps
}

func NewDecideRerouteHandler(deps DecideRerouteDeps) *DecideRerouteHandler {
	return &DecideRerouteHandler{deps: deps}
}

// Decide 把一次授权角色的择定推到决定或一格具名的答复：读建议 → 取判断键锁 → 已决定 → 过时 → 不在建议之列 → 原计划的适用性
// → 受理基线那一版投影 → 此刻的证据重判所择候选 → 取成本、组计划 → 形成决定、落册。CONTEXT「人工决定同样不得绕过硬限制、
// 商业服务范围或其他上下文拥有的既有事实与责任」由重判那一步守：建议形成之后才生效的限制，只有此刻的证据看得见。
func (handler *DecideRerouteHandler) Decide(
	ctx context.Context,
	command DecideRerouteCommand,
) (DecideRerouteResult, error) {
	record, present, err := handler.deps.Reassessments.FindByCorrelation(ctx, command.Tenant, command.Suggestion)
	if err != nil {
		return handler.undecided(command, DecisionReassessmentStoreUnavailable), nil
	}
	// 判断键上的包裹必须是请求指名的那一件：否则同一租户里拿另一件包裹的建议关联也能改这件的路。
	if !present || !record.HasSuggestion || record.Key.DeclaredParcelID != command.Parcel {
		return DecideRerouteResult{outcome: RerouteSuggestionNotFound}, nil
	}
	if record.Conclusion != ports.ReassessmentPlanLapsed || record.ReviewedPlan.String() == "" {
		return DecideRerouteResult{}, fmt.Errorf("%w: suggestion %s concludes %s",
			ErrUnsupportedRerouteSuggestion, command.Suggestion, record.Conclusion)
	}
	key := record.Key

	// 取判断键锁：与复核同一把（ADR-0235 决定四），此后读已有决定、判过时、落册都在锁里。两个授权角色同时决定同一份建议，
	// 后到的一笔等在这里，读到先成的那一份；新一次复核与决定也按取锁先后排。读回的时刻不另用，过时由下面那一问答：同一时刻
	// 另有复核也要算，最晚时刻本身答不出这一格。
	if _, _, err := handler.deps.Reassessments.LatestTriggerAt(ctx, key); err != nil {
		return handler.undecided(command, DecisionReassessmentStoreUnavailable), nil
	}
	existing, decided, err := handler.deps.Decisions.FindBySuggestion(ctx, command.Tenant, command.Suggestion)
	if errors.Is(err, ports.ErrStoredRouteRecordUnreadable) {
		return DecideRerouteResult{}, fmt.Errorf("load reroute decision: %w", err)
	}
	if err != nil {
		return handler.undecided(command, DecisionStoreUnavailable), nil
	}
	if decided {
		return DecideRerouteResult{outcome: RerouteSuggestionAlreadyDecided, decision: existing, hasDecision: true}, nil
	}
	// 过时：建议所在的那次复核已不是判断键上业务时间最晚的那一次（ADR-0284 决定二）。迁移前没有业务时间的复核比不出先后，
	// 按过时答——包裹此后多半已在别处判过，拿它改路是在一份读不清新旧的建议上下决定。
	if record.TriggerOccurredAt.IsZero() {
		return DecideRerouteResult{outcome: RerouteSuggestionSuperseded}, nil
	}
	overtaken, err := handler.deps.Decisions.SuggestionOvertaken(ctx, key, command.Suggestion, record.TriggerOccurredAt)
	if err != nil {
		return handler.undecided(command, DecisionReassessmentStoreUnavailable), nil
	}
	if overtaken {
		return DecideRerouteResult{outcome: RerouteSuggestionSuperseded}, nil
	}
	if !suggestedAndQualified(record.Suggestion, command.Candidate) {
		return DecideRerouteResult{outcome: RerouteCandidateNotSuggested}, nil
	}

	// 原计划是失效线上被复核的那一份：它在那次复核里已离场。不是已失效，说明路由历史与复核册对不上，凭它决定就是在一份没离场
	// 的计划旁边再立一份当前有效计划。
	applicability, registered, err := handler.deps.Applicability.FindByPlan(ctx, record.ReviewedPlan)
	if errors.Is(err, ports.ErrStoredRouteRecordUnreadable) {
		return DecideRerouteResult{}, fmt.Errorf("load lapsed plan applicability: %w", err)
	}
	if err != nil {
		return handler.undecided(command, DecisionRoutingHistoryUnavailable), nil
	}
	if !registered || applicability.State() != domain.PlanLapsed {
		return DecideRerouteResult{}, fmt.Errorf("%w: suggestion %s reviews plan %s, which has not lapsed",
			ErrInconsistentRouteStart, command.Suggestion, record.ReviewedPlan)
	}

	baseline, accepted, err := handler.deps.Baselines.LoadAcceptanceBaselineGeo(ctx, command.Tenant, command.Parcel)
	if errors.Is(err, ports.ErrAcceptanceBaselineGeoUnreadable) {
		return handler.undecided(command, DecisionAcceptedParcelUnreadable), nil
	}
	if err != nil {
		return DecideRerouteResult{}, fmt.Errorf("load acceptance baseline geo: %w", err)
	}
	if !accepted || baseline.CustomerAccountID != key.CustomerAccountID ||
		baseline.ShipmentRequestID != key.ShipmentRequestID || baseline.AcceptanceBaseline != key.AcceptanceBaseline {
		return DecideRerouteResult{}, fmt.Errorf("%w: parcel %s", ErrSuggestionBaselineInconsistent, command.Parcel)
	}
	carried := ports.RequestCarriedContent{Geo: baseline.Geo}

	evidence, configured, err := handler.deps.Evidence.LoadInitialRouteEvidence(ctx, key, carried)
	if err != nil {
		return handler.undecided(command, DecisionEvidenceUnavailable), nil
	}
	if !configured {
		return DecideRerouteResult{outcome: RerouteEvidenceNotConfigured}, nil
	}
	if !evidence.ViewRevision.Valid() || !evidence.Strategy.Valid() {
		return DecideRerouteResult{}, ErrIncompleteRouteEvidence
	}
	candidates, _, err := evaluateRouteCandidates(evidence)
	if err != nil {
		return DecideRerouteResult{}, err
	}
	if refused, ok := refusalOf(candidates, command.Candidate); !ok {
		return refused, nil
	}

	evidence, costs, unavailable, err := loadRouteCandidateCosts(ctx, handler.deps.Costs, key, evidence, carried)
	if err != nil {
		return DecideRerouteResult{}, err
	}
	if unavailable {
		return handler.undecided(command, DecisionCostSourceUnavailable), nil
	}
	var legs []domain.PlannedLeg
	for _, path := range evidence.Paths {
		if path.Candidate == command.Candidate {
			legs = path.PlanLegs()
			break
		}
	}
	if len(legs) == 0 {
		return DecideRerouteResult{}, ErrIncompleteRouteEvidence
	}
	version, err := handler.deps.Identities.NextRoutePlanVersionID(ctx)
	if err != nil {
		return handler.undecided(command, DecisionIdentityUnavailable), nil
	}

	// 计划从决定的时刻起生效，判断时刻同取（ADR-0284 决定四）：失效到决定之间的无当前有效路由照留在适用性时间线上。
	now := handler.deps.Clock.Now()
	plan, err := domain.FormInitialRoutePlan(domain.InitialRoutePlanSpec{
		Key:              key,
		Version:          version,
		Selected:         command.Candidate,
		Candidates:       candidates,
		Legs:             legs,
		Strategy:         evidence.Strategy,
		ViewRevision:     evidence.ViewRevision,
		JudgedAt:         now,
		EffectiveFrom:    now,
		CostCitations:    costs.CitationsOf(command.Candidate),
		CustomsCitations: evidence.CustomsCitations,
	})
	if err != nil {
		return DecideRerouteResult{}, fmt.Errorf("form decided plan: %w", err)
	}
	decision, err := domain.FormRerouteDecision(domain.RerouteDecisionSpec{
		Authority:    record.RerouteState,
		Mode:         domain.AuthorizedRoleReroute,
		DecidedBy:    command.DecidedBy,
		Trigger:      record.Suggestion.Trigger(),
		OriginalPlan: record.ReviewedPlan,
		NewPlan:      plan,
		DecidedAt:    now,
	})
	if err != nil {
		return DecideRerouteResult{}, fmt.Errorf("form authorized reroute decision: %w", err)
	}
	formed := ports.AuthorizedRerouteDecision{
		Suggestion: command.Suggestion,
		Key:        key,
		Candidate:  command.Candidate,
		Basis:      command.Basis,
		Decision:   decision,
	}
	saved, err := handler.deps.Decisions.Save(ctx, formed)
	if err != nil {
		return handler.undecided(command, DecisionStoreUnavailable), nil
	}
	switch saved {
	case ports.AuthorizedRerouteDecisionSaved:
		return DecideRerouteResult{outcome: RerouteDecided, decision: formed, hasDecision: true}, nil
	case ports.AuthorizedRerouteDecisionAlreadyRecorded:
		winner, found, err := handler.deps.Decisions.FindBySuggestion(ctx, command.Tenant, command.Suggestion)
		if err != nil || !found {
			return handler.undecided(command, DecisionStoreUnavailable), nil
		}
		return DecideRerouteResult{outcome: RerouteSuggestionAlreadyDecided, decision: winner, hasDecision: true}, nil
	default:
		return DecideRerouteResult{}, fmt.Errorf("network routing: unexpected reroute decision save outcome %d", saved)
	}
}

// suggestedAndQualified 答所择候选是不是建议里合格的一条：建议形成时就被淘汰或证据未知的，授权角色也不能据这份建议选它。
func suggestedAndQualified(suggestion domain.RerouteSuggestion, candidate domain.CandidateID) bool {
	for _, suggested := range suggestion.Candidates() {
		if suggested.ID() == candidate {
			return suggested.Outcome() == domain.CandidateQualified
		}
	}
	return false
}

// refusalOf 按此刻的评估答所择候选成不成立；不成立时交回具名的那一格与依据（ADR-0284 决定二）。被有效硬限制淘汰与别的层次的
// 淘汰分开答：前者只有限制的来源责任方解除得了，后者是另择一条。
func refusalOf(candidates []domain.RouteCandidate, chosen domain.CandidateID) (DecideRerouteResult, bool) {
	for _, candidate := range candidates {
		if candidate.ID() != chosen {
			continue
		}
		switch {
		case candidate.Outcome() == domain.CandidateQualified:
			return DecideRerouteResult{}, true
		case candidate.EliminatedByRestriction():
			return DecideRerouteResult{outcome: RerouteCandidateRestricted, candidateReason: candidate.Reason().String()}, false
		case candidate.Outcome() == domain.CandidateEvidenceUnknown:
			return DecideRerouteResult{outcome: RerouteCandidateEvidenceUnknown, candidateReason: candidate.Reason().String()}, false
		default:
			return DecideRerouteResult{outcome: RerouteCandidateNotQualified, candidateReason: candidate.Reason().String()}, false
		}
	}
	return DecideRerouteResult{outcome: RerouteCandidateNotQualified, candidateReason: candidateOutsideTheSpace}, false
}

func (handler *DecideRerouteHandler) undecided(
	command DecideRerouteCommand,
	reason DecideRerouteUndecidedReason,
) DecideRerouteResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		reason.String(),
		command.Tenant.String(),
		command.Suggestion.String(),
		command.Candidate.String(),
	}, "\x00")))
	return DecideRerouteResult{
		outcome:      RerouteDecisionUndecided,
		reason:       reason,
		continuation: ContinuationReference{value: "CONT-" + hex.EncodeToString(digest[:8])},
	}
}
