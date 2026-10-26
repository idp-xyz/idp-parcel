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

// ErrTieBaselineInconsistent 说受理基线那一版读回来与未形成登记的判断键对不上，或读成「不是当前已接受包裹」：登记行的判断键
// 就落在那一条基线上，基线一经固定不再改写，对不上是不变量破了，重发不会变（ADR-0029）。
var ErrTieBaselineInconsistent = errors.New("network routing: the acceptance baseline disagrees with the tie registration")

// DecideInitialRouteOutcome 是授权角色裁并列的封闭答复（ADR-0309 决定二）。除成立与未决外各格都是确定性答复，续办各不相同：
// 已有结果去看初始路由，没有并列登记去看这件包裹停在哪一格，所择不在并列里另择，被限制淘汰等来源责任方解除，不合格另择，
// 证据未知与证据未配置等登记。合成一格，授权角色就分不出下一步做什么。
type DecideInitialRouteOutcome uint8

const (
	DecideInitialRouteOutcomeInvalid DecideInitialRouteOutcome = iota
	InitialRouteTieDecided
	InitialRouteAlreadyFormed
	InitialRouteNoTieRegistered
	InitialRouteCandidateNotTied
	InitialRouteCandidateRestricted
	InitialRouteCandidateNotQualified
	InitialRouteCandidateEvidenceUnknown
	InitialRouteEvidenceNotConfigured
	InitialRouteDecisionUndecided
)

func (outcome DecideInitialRouteOutcome) String() string {
	switch outcome {
	case InitialRouteTieDecided:
		return "INITIAL_ROUTE_DECIDED"
	case InitialRouteAlreadyFormed:
		return "INITIAL_ROUTE_ALREADY_FORMED"
	case InitialRouteNoTieRegistered:
		return "NO_TIE_REGISTERED"
	case InitialRouteCandidateNotTied:
		return "CANDIDATE_NOT_TIED"
	case InitialRouteCandidateRestricted:
		return "CANDIDATE_RESTRICTED"
	case InitialRouteCandidateNotQualified:
		return "CANDIDATE_NOT_QUALIFIED"
	case InitialRouteCandidateEvidenceUnknown:
		return "CANDIDATE_EVIDENCE_UNKNOWN"
	case InitialRouteEvidenceNotConfigured:
		return "INITIAL_ROUTE_EVIDENCE_NOT_CONFIGURED"
	case InitialRouteDecisionUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// DecideInitialRouteUndecidedReason 只收重发推得动的格：依赖读不动、取号故障。
type DecideInitialRouteUndecidedReason uint8

const (
	DecideInitialRouteUndecidedReasonNone DecideInitialRouteUndecidedReason = iota
	TieJudgmentLockUnavailable
	TieRegisterUnavailable
	TieRouteStoreUnavailable
	TieAcceptedParcelUnreadable
	TieEvidenceUnavailable
	TieCostSourceUnavailable
	TieIdentityUnavailable
)

func (reason DecideInitialRouteUndecidedReason) String() string {
	switch reason {
	case TieJudgmentLockUnavailable:
		return "JUDGMENT_LOCK_UNAVAILABLE"
	case TieRegisterUnavailable:
		return "NOT_FORMED_REGISTER_UNAVAILABLE"
	case TieRouteStoreUnavailable:
		return "ROUTE_STORE_UNAVAILABLE"
	case TieAcceptedParcelUnreadable:
		return "ACCEPTED_PARCEL_UNREADABLE"
	case TieEvidenceUnavailable:
		return "ROUTE_EVIDENCE_UNAVAILABLE"
	case TieCostSourceUnavailable:
		return "ROUTE_COST_SOURCE_UNAVAILABLE"
	case TieIdentityUnavailable:
		return "ROUTE_IDENTITY_UNAVAILABLE"
	default:
		return ""
	}
}

// DecideInitialRouteCommand 是一次对并列的授权裁决。租户与决定方取认证结果；包裹、所择候选与依据由决定方给。
type DecideInitialRouteCommand struct {
	Tenant    domain.TenantID
	Parcel    domain.DeclaredParcelID
	Candidate domain.CandidateID
	Basis     domain.RerouteDecisionBasisReference
	DecidedBy domain.AuthorizedRoleReference
}

type DecideInitialRouteResult struct {
	outcome         DecideInitialRouteOutcome
	key             domain.InitialRouteJudgmentKey
	existing        ports.InitialRouteRecord
	hasExisting     bool
	plan            domain.InitialRoutePlan
	hasPlan         bool
	registered      ports.InitialRouteNotFormedReason
	candidateReason string
	reason          DecideInitialRouteUndecidedReason
	continuation    ContinuationReference
}

func (result DecideInitialRouteResult) Outcome() DecideInitialRouteOutcome { return result.outcome }

// Key 在成立与已有结果时给出：判断键各维。
func (result DecideInitialRouteResult) Key() domain.InitialRouteJudgmentKey { return result.key }

// Plan 在成立时给出这一次形成的首个计划。
func (result DecideInitialRouteResult) Plan() (domain.InitialRoutePlan, bool) {
	return result.plan, result.hasPlan
}

// Existing 在已有结果时给出判断库里的那一行（计划或无路由）。
func (result DecideInitialRouteResult) Existing() (ports.InitialRouteRecord, bool) {
	return result.existing, result.hasExisting
}

// RegisteredReason 在没有并列登记且登记行存在时给出它最近一次停住的原因，授权角色据此知道这件包裹卡在哪一格。
func (result DecideInitialRouteResult) RegisteredReason() ports.InitialRouteNotFormedReason {
	return result.registered
}

// CandidateReason 在此刻重判不成立的三格给出所择候选的依据。
func (result DecideInitialRouteResult) CandidateReason() string { return result.candidateReason }

func (result DecideInitialRouteResult) UndecidedReason() DecideInitialRouteUndecidedReason {
	return result.reason
}

func (result DecideInitialRouteResult) ContinuationReference() ContinuationReference {
	return result.continuation
}

type DecideInitialRouteDeps struct {
	// Locks 只借 LatestTriggerAt 取判断键锁：与复核、授权改路决定同一把（ADR-0235 决定四），读回的时刻不用。
	Locks      ports.ReassessmentStore
	NotFormed  ports.InitialRouteNotFormedRead
	Routes     ports.InitialRouteStore
	Decisions  ports.InitialRouteTieDecisions
	Baselines  ports.AcceptanceBaselineGeoSource
	Evidence   ports.InitialRouteEvidenceView
	Costs      ports.RouteCandidateCostSource
	Downstream ports.InitialRouteHandoff
	Identities ports.RouteIdentityFactory
	Clock      ports.Clock
}

// DecideInitialRouteHandler 是授权角色裁最低成本并列、形成首个计划的编排（UC-NR-001 步骤 8D；ADR-0309）。事务边界归装配点：
// 判断键锁随事务释放，读已有结果、重判、落判断库、落决定证据与交发布意图要在同一笔里。
type DecideInitialRouteHandler struct {
	deps DecideInitialRouteDeps
}

func NewDecideInitialRouteHandler(deps DecideInitialRouteDeps) *DecideInitialRouteHandler {
	return &DecideInitialRouteHandler{deps: deps}
}

// Decide 把一次授权裁决推到首个计划或一格具名的答复：由包裹读最近一次停住的登记 → 取判断键锁 → 已有结果 → 登记是不是并列 →
// 所择在不在并列里 → 受理基线那一版投影 → 此刻的证据重判所择候选 → 取成本、组计划 → 落判断库、决定证据与发布意图。CONTEXT
// 「人工决定同样不得绕过硬限制、商业服务范围或其他上下文拥有的既有事实与责任」由重判那一步守：登记之后才生效的限制，只有此刻
// 的证据看得见。
func (handler *DecideInitialRouteHandler) Decide(
	ctx context.Context,
	command DecideInitialRouteCommand,
) (DecideInitialRouteResult, error) {
	located, found, err := handler.deps.NotFormed.FindLatestNotFormedOfParcel(ctx, command.Tenant, command.Parcel)
	if errors.Is(err, ports.ErrStoredRouteRecordUnreadable) {
		return DecideInitialRouteResult{}, fmt.Errorf("load not-formed registration: %w", err)
	}
	if err != nil {
		return handler.undecided(command, TieRegisterUnavailable), nil
	}
	if !found {
		return DecideInitialRouteResult{outcome: InitialRouteNoTieRegistered}, nil
	}
	key := located.Key

	// 取判断键锁：与复核、授权改路决定同一把，此后读已有结果、读登记行、落册都在锁里。两个授权角色同时裁同一件包裹，后到的
	// 一笔等在这里，读到先成的那一份。
	if _, _, err := handler.deps.Locks.LatestTriggerAt(ctx, key); err != nil {
		return handler.undecided(command, TieJudgmentLockUnavailable), nil
	}
	existing, formed, err := handler.deps.Routes.FindByKey(ctx, key)
	if errors.Is(err, ports.ErrStoredRouteRecordUnreadable) {
		return DecideInitialRouteResult{}, fmt.Errorf("load initial route: %w", err)
	}
	if err != nil {
		return handler.undecided(command, TieRouteStoreUnavailable), nil
	}
	if formed {
		return DecideInitialRouteResult{outcome: InitialRouteAlreadyFormed, key: key, existing: existing, hasExisting: true}, nil
	}
	// 锁里重读登记行：锁外读到的那一行，等锁的工夫里可能已被同一判断键的又一次交接改写成别的原因。
	registered, found, err := handler.deps.NotFormed.FindNotFormed(ctx, key)
	if errors.Is(err, ports.ErrStoredRouteRecordUnreadable) {
		return DecideInitialRouteResult{}, fmt.Errorf("load not-formed registration: %w", err)
	}
	if err != nil {
		return handler.undecided(command, TieRegisterUnavailable), nil
	}
	if !found {
		return DecideInitialRouteResult{outcome: InitialRouteNoTieRegistered}, nil
	}
	if registered.Reason != ports.NotFormedCandidatesTied {
		return DecideInitialRouteResult{outcome: InitialRouteNoTieRegistered, registered: registered.Reason}, nil
	}
	if !containsCandidate(registered.Tied, command.Candidate) {
		return DecideInitialRouteResult{outcome: InitialRouteCandidateNotTied}, nil
	}

	baseline, accepted, err := handler.deps.Baselines.LoadAcceptanceBaselineGeo(ctx, command.Tenant, command.Parcel)
	if errors.Is(err, ports.ErrAcceptanceBaselineGeoUnreadable) {
		return handler.undecided(command, TieAcceptedParcelUnreadable), nil
	}
	if err != nil {
		return DecideInitialRouteResult{}, fmt.Errorf("load acceptance baseline geo: %w", err)
	}
	if !accepted || baseline.CustomerAccountID != key.CustomerAccountID ||
		baseline.ShipmentRequestID != key.ShipmentRequestID || baseline.AcceptanceBaseline != key.AcceptanceBaseline {
		return DecideInitialRouteResult{}, fmt.Errorf("%w: parcel %s", ErrTieBaselineInconsistent, command.Parcel)
	}
	carried := ports.RequestCarriedContent{Geo: baseline.Geo}

	evidence, configured, err := handler.deps.Evidence.LoadInitialRouteEvidence(ctx, key, carried)
	if err != nil {
		return handler.undecided(command, TieEvidenceUnavailable), nil
	}
	if !configured {
		return DecideInitialRouteResult{outcome: InitialRouteEvidenceNotConfigured}, nil
	}
	if !evidence.ViewRevision.Valid() || !evidence.Strategy.Valid() {
		return DecideInitialRouteResult{}, ErrIncompleteRouteEvidence
	}
	candidates, _, err := evaluateRouteCandidates(evidence)
	if err != nil {
		return DecideInitialRouteResult{}, err
	}
	if refused, ok := refusalOfTie(candidates, command.Candidate); !ok {
		return refused, nil
	}

	evidence, costs, unavailable, err := loadRouteCandidateCosts(ctx, handler.deps.Costs, key, evidence, carried)
	if err != nil {
		return DecideInitialRouteResult{}, err
	}
	if unavailable {
		return handler.undecided(command, TieCostSourceUnavailable), nil
	}
	var legs []domain.PlannedLeg
	for _, path := range evidence.Paths {
		if path.Candidate == command.Candidate {
			legs = path.PlanLegs()
			break
		}
	}
	if len(legs) == 0 {
		return DecideInitialRouteResult{}, ErrIncompleteRouteEvidence
	}
	version, err := handler.deps.Identities.NextRoutePlanVersionID(ctx)
	if err != nil {
		return handler.undecided(command, TieIdentityUnavailable), nil
	}

	// 计划从决定的时刻起生效，判断时刻同取（ADR-0309 决定三）：委托接受到决定之间包裹没有当前有效路由，这一段照留，不追认。
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
		return DecideInitialRouteResult{}, fmt.Errorf("form decided initial plan: %w", err)
	}
	record := ports.InitialRouteRecord{Key: key, Plan: plan, HasPlan: true}
	saved, err := handler.deps.Routes.Save(ctx, record)
	if err != nil {
		return handler.undecided(command, TieRouteStoreUnavailable), nil
	}
	switch saved {
	case ports.InitialRouteSaved:
	case ports.InitialRouteAlreadyRecorded:
		// 锁外的初始路由交接（它不取这把锁）抢先落了这件包裹的结果：读回赢家，答已有结果。
		winner, present, err := handler.deps.Routes.FindByKey(ctx, key)
		if errors.Is(err, ports.ErrStoredRouteRecordUnreadable) {
			return DecideInitialRouteResult{}, fmt.Errorf("load concurrent initial route: %w", err)
		}
		if err != nil || !present {
			return handler.undecided(command, TieRouteStoreUnavailable), nil
		}
		return DecideInitialRouteResult{outcome: InitialRouteAlreadyFormed, key: key, existing: winner, hasExisting: true}, nil
	default:
		return DecideInitialRouteResult{}, fmt.Errorf("%w: %d", ErrUnexpectedRouteSaveOutcome, saved)
	}
	if err := handler.deps.Decisions.Record(ctx, ports.InitialRouteTieDecision{
		Key:         key,
		Candidate:   command.Candidate,
		PlanVersion: version,
		Basis:       command.Basis,
		DecidedBy:   command.DecidedBy,
		DecidedAt:   now,
	}); err != nil {
		return DecideInitialRouteResult{}, fmt.Errorf("record tie decision: %w", err)
	}
	// 发布意图与判断库、决定证据同一条提交边界（票 code/97 修法第 3 条）：交不出去整笔回滚，重发即可。与初始路由形成不同——那边
	// 失败只留续办引用，是因为一封交接里别的包裹已判成不能连坐；这里一次只有一件。
	if err := handler.deps.Downstream.HandOffInitialRoute(ctx, ports.InitialRouteHandoffIntent{
		Correlation: registered.Correlation,
		Record:      record,
	}); err != nil {
		return DecideInitialRouteResult{}, fmt.Errorf("hand off decided initial route: %w", err)
	}
	return DecideInitialRouteResult{outcome: InitialRouteTieDecided, key: key, plan: plan, hasPlan: true}, nil
}

func containsCandidate(candidates []domain.CandidateID, candidate domain.CandidateID) bool {
	for _, listed := range candidates {
		if listed == candidate {
			return true
		}
	}
	return false
}

// refusalOfTie 按此刻的评估答所择候选成不成立；不成立时交回具名的那一格与依据（ADR-0309 决定二）。判法与授权改路决定的
// refusalOf 同一条，只是答复集合不同。
func refusalOfTie(candidates []domain.RouteCandidate, chosen domain.CandidateID) (DecideInitialRouteResult, bool) {
	refused, ok := refusalOf(candidates, chosen)
	if ok {
		return DecideInitialRouteResult{}, true
	}
	switch refused.outcome {
	case RerouteCandidateRestricted:
		return DecideInitialRouteResult{outcome: InitialRouteCandidateRestricted, candidateReason: refused.candidateReason}, false
	case RerouteCandidateEvidenceUnknown:
		return DecideInitialRouteResult{outcome: InitialRouteCandidateEvidenceUnknown, candidateReason: refused.candidateReason}, false
	default:
		return DecideInitialRouteResult{outcome: InitialRouteCandidateNotQualified, candidateReason: refused.candidateReason}, false
	}
}

func (handler *DecideInitialRouteHandler) undecided(
	command DecideInitialRouteCommand,
	reason DecideInitialRouteUndecidedReason,
) DecideInitialRouteResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		"INITIAL_ROUTE_TIE",
		reason.String(),
		command.Tenant.String(),
		command.Parcel.String(),
		command.Candidate.String(),
	}, "\x00")))
	return DecideInitialRouteResult{
		outcome:      InitialRouteDecisionUndecided,
		reason:       reason,
		continuation: ContinuationReference{value: "CONT-" + hex.EncodeToString(digest[:8])},
	}
}
