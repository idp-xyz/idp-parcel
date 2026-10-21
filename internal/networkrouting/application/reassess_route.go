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

// ErrUnexpectedReassessmentSave 说明复核库交回了封闭集合以外的写入结果。
var ErrUnexpectedReassessmentSave = errors.New("network routing: unexpected reassessment save outcome")

// ErrInconsistentRouteStart 说明路由历史答不出判断起点：最近一份计划既不当前有效，也不是失效而无接班。
var ErrInconsistentRouteStart = errors.New("network routing: the routing history yields no reassessment starting point")

// ReassessOutcome 是复核请求的应用处理结果。三种领域走向（仍适用/已失效/首个计划）各占
// 一格，未决、已有结果、冲突、未受理与未判是应用格——用例明写这些结果不能合并为通用失败。
type ReassessOutcome uint8

const (
	ReassessOutcomeInvalid ReassessOutcome = iota
	ReassessedStillApplicable
	ReassessedPlanLapsed
	ReassessedFirstPlanFormed
	ReassessedRerouted
	ReassessUndecided
	ReassessExistingResult
	ReassessTriggerConflict
	ReassessTriggerNotAccepted
	// ReassessTriggerNotJudged 是触发入账、这一次不形成判断，原因已落未判登记（ADR-0235）。
	// 与未决分开，因为两者的恢复动作不同：未决等依赖恢复、回滚重投；这一格重投推不动，让它
	// 占着分区重投只会烧到 ABANDONED，照样丢掉这次复核，还挡住同一分区后面的信封。
	ReassessTriggerNotJudged
)

func (outcome ReassessOutcome) String() string {
	switch outcome {
	case ReassessedStillApplicable:
		return "STILL_APPLICABLE"
	case ReassessedPlanLapsed:
		return "PLAN_LAPSED"
	case ReassessedFirstPlanFormed:
		return "FIRST_PLAN_FORMED"
	case ReassessedRerouted:
		return "REROUTED"
	case ReassessUndecided:
		return "UNDECIDED"
	case ReassessExistingResult:
		return "EXISTING_RESULT"
	case ReassessTriggerConflict:
		return "TRIGGER_CONFLICT"
	case ReassessTriggerNotAccepted:
		return "TRIGGER_NOT_ACCEPTED"
	case ReassessTriggerNotJudged:
		return "TRIGGER_NOT_JUDGED"
	default:
		return ""
	}
}

// ReassessUndecidedReason 指名复核停在哪一步。封闭集合，按依赖阶段分类统计。
//
// 只收重投推得动的格（ADR-0235，判据同 ADR-0029 按恢复动作分格）；重投推不动的归
// ports.NotJudgedReason，入账留原因。往这里加一格之前先答它重投会不会变：不会变的放进来，
// 消费门就会对着它烧完失败预算。
type ReassessUndecidedReason uint8

const (
	ReassessUndecidedReasonNone ReassessUndecidedReason = iota
	ReassessLogUnavailable
	RoutingHistoryUnavailable
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

// ReassessRouteCommand 携带复核触发与随触发带来的内容。Carried 与 CreateInitialRouteCommand 的同名格一个来处、
// 一种用法：复核取证据时用它，关务适用性才问得到两端国家；空着就只能答两端未知、复核停在看不清（ADR-0075）。
type ReassessRouteCommand struct {
	Trigger domain.ReassessmentTriggerSpec
	Carried ports.RequestCarriedContent
}

type ReassessRouteResult struct {
	outcome      ReassessOutcome
	record       ports.ReassessmentRecord
	hasRecord    bool
	reason       ReassessUndecidedReason
	notJudged    ports.NotJudgedReason
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

// NotJudgedReason 只在结果为`未判`时有值。
func (result ReassessRouteResult) NotJudgedReason() ports.NotJudgedReason {
	return result.notJudged
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
	// AutoReroute 是事实目录读口。复核不读它（ADR-0173）：自动改路由策略版本当场折出。
	AutoReroute ports.AutoRerouteFactsView
}

type ReassessRouteHandler struct {
	deps ReassessRouteDeps
}

func NewReassessRouteHandler(deps ReassessRouteDeps) *ReassessRouteHandler {
	return &ReassessRouteHandler{deps: deps}
}

// Handle 把一次权威触发推进到复核结论：触发受理（线索在构造期就被拒）→ 重复/冲突分界
// → 载入历史结果（没有历史不得凭空假定初始计划）→ 定判断起点（当前有效计划，或无当前有效路由）→ 两条独立判断线
// （6A 计划适用性、6B 替代候选）→ 提交。失效即失效：候选评估未决拦不住失效落库，三件并存（硬句）。
// 判断前提里重投推不动的那几格不答未决，入账未判、留下原因；早于当前位置的触发同样入账不判（ADR-0235）。
func (handler *ReassessRouteHandler) Handle(
	ctx context.Context,
	command ReassessRouteCommand,
) (ReassessRouteResult, error) {
	trigger, err := domain.NewReassessmentTrigger(command.Trigger)
	if err != nil {
		return ReassessRouteResult{outcome: ReassessTriggerNotAccepted}, nil
	}
	key := trigger.Key()

	digest := triggerDigest(trigger, command.Carried.Geo)
	recorded, logged, err := handler.deps.Log.FindDigest(ctx, key.TenantID, trigger.Correlation())
	if err != nil {
		return handler.undecided(key, ReassessLogUnavailable), nil
	}
	if logged && recorded != digest {
		return ReassessRouteResult{outcome: ReassessTriggerConflict}, nil
	}
	if !logged {
		if err := handler.deps.Log.Append(ctx, key.TenantID, trigger.Correlation(), digest); err != nil {
			return handler.undecided(key, ReassessLogUnavailable), nil
		}
	}

	// 读当前位置即取判断键锁，此后读起点、判、落册都在锁里（ADR-0235）。
	positionAt, positioned, err := handler.deps.Store.LatestTriggerAt(ctx, key)
	if err != nil {
		return handler.undecided(key, ReassessStoreUnavailable), nil
	}
	// 已有结果分界放在锁里：同一触发的另一笔事务可能刚在锁前提交了复核，这时起点已被它换过，按新起点再判一次就是重复
	// 决定，还会再写一次适用性。指纹先前就在册的，是重放，按原结论作答；指纹是本笔刚登的，是并发里输给了赢家，答已有
	// 结果，与提交时撞键同一格。
	existing, present, err := handler.deps.Store.FindByCorrelation(ctx, key.TenantID, trigger.Correlation())
	if err != nil {
		return handler.undecided(key, ReassessStoreUnavailable), nil
	}
	if present {
		outcome := outcomeFor(existing)
		if !logged {
			outcome = ReassessExistingResult
		}
		return ReassessRouteResult{outcome: outcome, record: existing, hasRecord: true}, nil
	}
	// 指纹在册而复核不在，只会是上一次这个触发入账未判：其余出口要么连指纹一起回滚，要么同笔落了复核。携带同一触发的
	// 另一封信封走到这里时往下重新判——前提此刻齐了就判，仍缺就再入账；答「登记册读不回」会把一份合法的未判卡回重投，
	// 烧到 ABANDONED（ADR-0235）。同一封信封入账之后由消费门按 inbox 跳过，到不了这里。

	// UC-NR-003 复核顺序「触发有效性」那一层问业务时间是否当前：包裹已经在更晚的控制位置上判过，这次触发不再据以
	// 失效或改路（ADR-0235）。放在已有结果分界之后：已判过的触发再投读回原结果，不因它后来被更晚的复核超过而改答。
	if positioned && trigger.PrecedesPosition(positionAt) {
		return handler.notJudged(ctx, trigger, ports.NotJudgedTriggerPrecedesCurrentPosition, positionAt)
	}

	history, found, err := handler.deps.Routes.FindByKey(ctx, key)
	if err != nil {
		return handler.undecided(key, RoutingHistoryUnavailable), nil
	}
	if !found {
		// 「没有历史结果时不得凭空假定初始计划」——复核复的是一份已有判断。
		return handler.notJudged(ctx, trigger, ports.NotJudgedNoRoutingHistory, time.Time{})
	}
	start, stalled, err := handler.startingPoint(ctx, key, history)
	if err != nil {
		return ReassessRouteResult{}, err
	}
	if stalled != ReassessUndecidedReasonNone {
		return handler.undecided(key, stalled), nil
	}

	evidence, configured, err := handler.deps.Evidence.LoadInitialRouteEvidence(ctx, key, command.Carried)
	if err != nil {
		return handler.undecided(key, ReassessEvidenceUnavailable), nil
	}
	if !configured {
		return handler.notJudged(ctx, trigger, ports.NotJudgedEvidenceNotConfigured, time.Time{})
	}
	if !evidence.ViewRevision.Valid() || !evidence.Strategy.Valid() {
		return ReassessRouteResult{}, ErrIncompleteRouteEvidence
	}

	if start.hasPlan {
		return handler.reassessPlan(ctx, trigger, start, evidence)
	}
	return handler.reassessNoRoute(ctx, trigger, evidence)
}

// routeStart 是复核的判断起点：这件包裹此刻的当前有效计划连同它的适用性，或无当前有效路由（hasPlan 为假）。
type routeStart struct {
	plan          domain.InitialRoutePlan
	applicability domain.PlanApplicability
	hasPlan       bool
}

// startingPoint 定复核从哪里判。受理时落的初始路由那一行此后不变，复核的结论与新计划只落复核册与适用性册，所以起点要
// 三本册合起来读：最近一份计划是复核册里最近形成的新计划，没有就是初始计划；它的适用性答它是否仍当前有效。已失效而无
// 接班即无当前有效路由，下一次复核走无路由那条线（CONTEXT 生命周期）——拿已失效的计划再复核一次，限制仍在时撞上
// 「不得二次离场」，限制已撤时把它原地答回仍适用，两样都是生命周期不许的。
//
// 适用性没有登记行即当前有效：计划形成时不另写一行，离场才写。最近一份计划已被替代或已结束是坏起点：替代必有更晚的
// 接班计划，结束之后不再有复核；照哪一边判都是凭空假定，响亮报错。
func (handler *ReassessRouteHandler) startingPoint(
	ctx context.Context,
	key domain.InitialRouteJudgmentKey,
	history ports.InitialRouteRecord,
) (routeStart, ReassessUndecidedReason, error) {
	plan, formed, err := handler.deps.Store.LatestNewPlan(ctx, key)
	if err != nil {
		return routeStart{}, RoutingHistoryUnavailable, nil
	}
	if !formed {
		if !history.HasPlan {
			return routeStart{}, ReassessUndecidedReasonNone, nil
		}
		plan = history.Plan
	}
	applicability, registered, err := handler.deps.Applicability.FindByPlan(ctx, plan.Version())
	if err != nil {
		return routeStart{}, ApplicabilityStoreUnavailable, nil
	}
	if !registered {
		applicability, err = domain.EstablishPlanApplicability(plan.Version(), plan.EffectiveFrom())
		if err != nil {
			return routeStart{}, ReassessUndecidedReasonNone, fmt.Errorf("establish plan applicability: %w", err)
		}
	}
	switch applicability.State() {
	case domain.PlanCurrentlyEffective:
		return routeStart{plan: plan, applicability: applicability, hasPlan: true}, ReassessUndecidedReasonNone, nil
	case domain.PlanLapsed:
		return routeStart{}, ReassessUndecidedReasonNone, nil
	default:
		return routeStart{}, ReassessUndecidedReasonNone, fmt.Errorf(
			"%w: the latest plan %s is %s", ErrInconsistentRouteStart, plan.Version().String(), applicability.State())
	}
}

// planNodesOf 取计划的节点序列，只由节点间段推（ADR-0217）：交付段之后没有可控节点，把它计入
// 剩余段数，冻结边界会整体提前一个节点。答不出只可能是计划绕过了构造门——FormInitialRoutePlan
// 保证节点间段连续且至少一段——那是编程错误，响亮报错；折成「冻结形态未配置」会把它伪装成租户
// 配置缺口。
func planNodesOf(plan domain.InitialRoutePlan) ([]string, error) {
	nodes, ok := domain.PlanNodes(plan.NodeLegs())
	if !ok {
		return nil, fmt.Errorf("network routing: plan %s yields no node sequence", plan.Version().String())
	}
	return nodes, nil
}

// freezeOf 在原计划仍可执行时判断有没有越过冻结边界。未声明形态答未配置，不当成未冻结。
// 硬约束失效不走这里。
//
// 只问建立了当前可控节点的复核；场外揽收在调用方就分走了，不判冻结（ADR-0249 决定二）。
//
// 只有「真的未声明」答未配置：它会落成未判登记里的 FREEZE_FORM_UNCONFIGURED，是一条给租户看的
// 持久原因（ADR-0235）。另两种答不出的都是编程错误，响亮报错，理由同 planNodesOf：
//   - 前缀立不起来：走到这里的触发建立了可控节点，仍适用又说明位置就在计划节点上，前缀必立得起来。
//   - 冻结事实不成立（形态在封闭族外、段数为负）：策略版本册的 CHECK 拒这两种写入，证据视图交出它们
//     是装配坏了，不是租户没登记。
func freezeOf(
	plan domain.InitialRoutePlan,
	trigger domain.ReassessmentTrigger,
	evidence ports.InitialRouteEvidence,
) (domain.FreezeJudgment, bool, error) {
	nodes, err := planNodesOf(plan)
	if err != nil {
		return domain.FreezeJudgment{}, false, err
	}
	prefix := domain.JudgeExecutedPrefix(nodes, trigger.Control(), trigger.Location().String())
	if prefix.Grade() != domain.ExecutedPrefixEstablished {
		return domain.FreezeJudgment{}, false, fmt.Errorf(
			"network routing: plan %s is still applicable at %s yet yields no executed prefix",
			plan.Version().String(), trigger.Location().String())
	}
	judgment, err := domain.JudgeFreezeBoundary(evidence.FreezeForm, evidence.FreezeRemainingSegmentLimit, prefix.RemainingSegments())
	if err != nil {
		return domain.FreezeJudgment{}, false, fmt.Errorf("judge freeze boundary: %w", err)
	}
	return judgment, judgment.Unconfigured(), nil
}

// reassessPlan 是原来有计划的那条线：6A 独立判定，仍适用即保留、失效即失效（候选状态
// 另存）、看不清停未决。可控节点上仍适用时先看冻结：未声明停未配置；已越过边界则保留原计划，
// 轻微改善不改路。场外揽收不建立可控节点，仍适用即保留，不看冻结也不试改善（ADR-0249 决定二）：
// 冻结与改善问的都是「从这个节点起往后怎么走」，揽收时没有这个节点；改善等包裹交进首个节点后的交接复核。
func (handler *ReassessRouteHandler) reassessPlan(
	ctx context.Context,
	trigger domain.ReassessmentTrigger,
	start routeStart,
	evidence ports.InitialRouteEvidence,
) (ReassessRouteResult, error) {
	key := trigger.Key()
	plan := start.plan
	review, err := domain.ReviewPlanApplicability(plan, trigger.Control(), trigger.Location(), evidence.HardConstraints)
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
		if !trigger.Control().EstablishesControllableNode() {
			return handler.commit(ctx, trigger, record)
		}
		judgment, unconfigured, err := freezeOf(plan, trigger, evidence)
		if err != nil {
			return ReassessRouteResult{}, err
		}
		if unconfigured {
			return handler.notJudged(ctx, trigger, ports.NotJudgedFreezeFormUnconfigured, time.Time{})
		}
		// 已越过冻结边界则保留原计划，轻微改善不改路。未越过时，成本改善严格大于
		// 已登记阈值才允许自动切换。
		if judgment.Crossed() {
			return handler.commit(ctx, trigger, record)
		}
		switched, err := handler.tryImprovementReroute(ctx, trigger, start, evidence, record)
		if err != nil {
			return ReassessRouteResult{}, err
		}
		if switched.HasNewPlan {
			return handler.commit(ctx, trigger, switched)
		}
		return handler.commit(ctx, trigger, record)

	case domain.PlanNoLongerApplicable:
		basis, _ := review.LapseBasis()
		// 起点在判断键锁里读、只取当前有效的计划，重复触发也在锁里按关联读回了，走到这里的计划不会已经离场；离场失败
		// 是编程错误，响亮报错。
		lapsed, err := start.applicability.Lapse(basis, trigger.OccurredAt())
		if err != nil {
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
		// 7B/7C：失效已定且候选可用时评估受控改路。任何一步评估不成都退回纯失效
		// 记录——失效不等改路，改路也拦不住失效（硬句的另一半）。
		if record.CandidateState == ports.CandidatesAvailable {
			record, err = handler.rerouteAfterLapse(ctx, trigger, plan, evidence, record)
			if err != nil {
				return ReassessRouteResult{}, err
			}
		}
		return handler.commit(ctx, trigger, record)
	default:
		return ReassessRouteResult{}, fmt.Errorf("network routing: unhandled review outcome %d", review.Outcome())
	}
}

// rerouteAfterLapse 在失效记录之上评估 7B 自动改路与 7C 改路建议。条件由策略版本折出，
// 不读事实目录。未声明只形成建议。条件全立且成本改善严格大于已登记阈值时形成自动改路
// 决定（结论升格为`已改路`）；有阻塞形成建议等授权角色；硬限制禁行只记录禁行依据。
func (handler *ReassessRouteHandler) rerouteAfterLapse(
	ctx context.Context,
	trigger domain.ReassessmentTrigger,
	lapsedPlan domain.InitialRoutePlan,
	evidence ports.InitialRouteEvidence,
	record ports.ReassessmentRecord,
) (ports.ReassessmentRecord, error) {
	authority, blockers, err := structuralAutoReroute(trigger, lapsedPlan, evidence)
	if err != nil {
		return ports.ReassessmentRecord{}, err
	}
	record.RerouteState = authority
	record.RerouteBlockers = blockers

	if authority == domain.RerouteBarred {
		return record, nil
	}

	candidates, undecided, err := handler.evaluateCandidates(evidence)
	if err != nil || undecided {
		// reviewCandidates 已判定候选可用，这里失配说明证据在两次取数间变了——
		// 退回纯失效，authority 也一并撤下（它的候选依据不成立了）。
		record.RerouteState = domain.RerouteAuthorityInvalid
		record.RerouteBlockers = nil
		return record, nil
	}
	triggerRef, err := domain.NewRerouteTriggerReference(trigger.Correlation().String())
	if err != nil {
		return ports.ReassessmentRecord{}, fmt.Errorf("reroute trigger reference: %w", err)
	}
	now := handler.deps.Clock.Now()

	switch authority {
	case domain.SuggestionOnly:
		suggestion, err := domain.NewRerouteSuggestion(domain.RerouteSuggestionSpec{
			Key:         trigger.Key(),
			Trigger:     triggerRef,
			Candidates:  candidates,
			Blockers:    blockers,
			SuggestedAt: now,
		})
		if err != nil {
			return ports.ReassessmentRecord{}, fmt.Errorf("new reroute suggestion: %w", err)
		}
		record.Suggestion = suggestion
		record.HasSuggestion = true
		return record, nil

	case domain.AutomaticRerouteAllowed:
		selection, err := handler.formPlanFromEvidence(ctx, trigger, candidates, evidence)
		if errors.Is(err, errRouteIdentityUnavailable) {
			// 取号依赖故障：失效已定不回滚，自动改路留给重触发——记录保留判定，
			// 决定缺席如实表示「允许了但没形成」。
			return record, nil
		}
		if err != nil {
			return ports.ReassessmentRecord{}, err
		}
		switch selection.ranking.Outcome() {
		case domain.RankingSelected:
			improved, comparable := costImprovement(
				evidence.CandidateCosts,
				lapsedPlan.SelectedCandidate(),
				selection.plan.SelectedCandidate(),
			)
			threshold := evidence.AutoRerouteImprovementThresholdMinor
			if !comparable || threshold == nil || improved <= int64(*threshold) {
				return handler.suggestionOnly(record, trigger.Key(), triggerRef, candidates, now,
					[]string{"IMPROVEMENT_BELOW_THRESHOLD"})
			}
		case domain.RankingTied:
			// 条件都立而选路选不出唯一一条：只形成建议，由授权角色在并列那几家之间裁。
			blockers := domain.LowestCostTieBlockers(selection.ranking.Tied())
			suggestion, err := domain.NewRerouteSuggestion(domain.RerouteSuggestionSpec{
				Key:         trigger.Key(),
				Trigger:     triggerRef,
				Candidates:  candidates,
				Blockers:    blockers,
				SuggestedAt: now,
			})
			if err != nil {
				return ports.ReassessmentRecord{}, fmt.Errorf("new tie reroute suggestion: %w", err)
			}
			record.RerouteState = domain.SuggestionOnly
			record.RerouteBlockers = blockers
			record.Suggestion = suggestion
			record.HasSuggestion = true
			return record, nil
		case domain.RankingNoQualifiedCandidate,
			domain.RankingFormNotDeclared,
			domain.RankingCostsPending,
			domain.RankingCostsUnpriceable,
			domain.RankingCurrenciesDiffer:
			// 选路不成：自动改路形不成，退回纯失效并保留判定与阻塞（空清单如实表示
			// 「条件都立、是选路不成」）。
			return record, nil
		default:
			return ports.ReassessmentRecord{}, fmt.Errorf("network routing: unhandled ranking outcome %d",
				selection.ranking.Outcome())
		}
		decision, err := domain.FormRerouteDecision(domain.RerouteDecisionSpec{
			Authority:    authority,
			Mode:         domain.AutomaticReroute,
			Trigger:      triggerRef,
			OriginalPlan: lapsedPlan.Version(),
			NewPlan:      selection.plan,
			DecidedAt:    now,
		})
		if err != nil {
			return ports.ReassessmentRecord{}, fmt.Errorf("form reroute decision: %w", err)
		}
		record.Conclusion = ports.ReassessmentRerouted
		record.NewPlan = selection.plan
		record.HasNewPlan = true
		record.Decision = decision
		record.HasDecision = true
		return record, nil

	default:
		return ports.ReassessmentRecord{}, fmt.Errorf("network routing: unhandled reroute authority %d", authority)
	}
}

func (handler *ReassessRouteHandler) suggestionOnly(
	record ports.ReassessmentRecord,
	key domain.InitialRouteJudgmentKey,
	triggerRef domain.RerouteTriggerReference,
	candidates []domain.RouteCandidate,
	now time.Time,
	blockers []string,
) (ports.ReassessmentRecord, error) {
	suggestion, err := domain.NewRerouteSuggestion(domain.RerouteSuggestionSpec{
		Key:         key,
		Trigger:     triggerRef,
		Candidates:  candidates,
		Blockers:    blockers,
		SuggestedAt: now,
	})
	if err != nil {
		return ports.ReassessmentRecord{}, fmt.Errorf("new reroute suggestion: %w", err)
	}
	record.RerouteState = domain.SuggestionOnly
	record.RerouteBlockers = blockers
	record.Suggestion = suggestion
	record.HasSuggestion = true
	return record, nil
}

// tryImprovementReroute 在计划仍可执行、且尚未越过冻结边界时，按成本改善决定是否自动切换。
// 没形成新计划时交回原记录，调用方保留原计划。
func (handler *ReassessRouteHandler) tryImprovementReroute(
	ctx context.Context,
	trigger domain.ReassessmentTrigger,
	start routeStart,
	evidence ports.InitialRouteEvidence,
	record ports.ReassessmentRecord,
) (ports.ReassessmentRecord, error) {
	plan := start.plan
	authority, _, err := structuralAutoReroute(trigger, plan, evidence)
	if err != nil {
		return ports.ReassessmentRecord{}, err
	}
	if authority != domain.AutomaticRerouteAllowed {
		return record, nil
	}
	candidates, undecided, err := handler.evaluateCandidates(evidence)
	if err != nil || undecided {
		return record, nil
	}
	selection, err := handler.formPlanFromEvidence(ctx, trigger, candidates, evidence)
	if errors.Is(err, errRouteIdentityUnavailable) {
		return record, nil
	}
	if err != nil {
		return ports.ReassessmentRecord{}, err
	}
	if selection.ranking.Outcome() != domain.RankingSelected {
		return record, nil
	}
	improved, comparable := costImprovement(
		evidence.CandidateCosts, plan.SelectedCandidate(), selection.plan.SelectedCandidate(),
	)
	threshold := evidence.AutoRerouteImprovementThresholdMinor
	if !comparable || threshold == nil || improved <= int64(*threshold) {
		return record, nil
	}
	triggerRef, err := domain.NewRerouteTriggerReference(trigger.Correlation().String())
	if err != nil {
		return ports.ReassessmentRecord{}, fmt.Errorf("reroute trigger reference: %w", err)
	}
	now := handler.deps.Clock.Now()
	decision, err := domain.FormRerouteDecision(domain.RerouteDecisionSpec{
		Authority:    authority,
		Mode:         domain.AutomaticReroute,
		Trigger:      triggerRef,
		OriginalPlan: plan.Version(),
		NewPlan:      selection.plan,
		DecidedAt:    now,
	})
	if err != nil {
		return ports.ReassessmentRecord{}, fmt.Errorf("form reroute decision: %w", err)
	}
	basis, err := domain.NewApplicabilityBasisReference("COST_IMPROVEMENT")
	if err != nil {
		return ports.ReassessmentRecord{}, err
	}
	// 原计划在接班计划生效的那一刻离场，两者同取触发的业务时间（见 formPlanFromEvidence）。
	superseded, err := start.applicability.Supersede(selection.plan.Version(), basis, trigger.OccurredAt())
	if err != nil {
		return ports.ReassessmentRecord{}, fmt.Errorf("supersede plan applicability: %w", err)
	}
	if err := handler.deps.Applicability.Save(ctx, superseded); err != nil {
		return ports.ReassessmentRecord{}, err
	}
	successor, err := domain.EstablishPlanApplicability(selection.plan.Version(), selection.plan.EffectiveFrom())
	if err != nil {
		return ports.ReassessmentRecord{}, err
	}
	if err := handler.deps.Applicability.Save(ctx, successor); err != nil {
		return ports.ReassessmentRecord{}, err
	}
	record.Conclusion = ports.ReassessmentRerouted
	record.RerouteState = authority
	record.NewPlan = selection.plan
	record.HasNewPlan = true
	record.Decision = decision
	record.HasDecision = true
	return record, nil
}

// structuralAutoReroute 折自动改路的结构条件。场外揽收时「位于当前可控节点」为假、「只改未执行部分」为真：失效线上
// 只形成建议，阻塞只点名不在可控节点，授权决定从包裹交进的首个节点生效（ADR-0249 决定二）。
func structuralAutoReroute(
	trigger domain.ReassessmentTrigger,
	plan domain.InitialRoutePlan,
	evidence ports.InitialRouteEvidence,
) (domain.RerouteAuthority, []string, error) {
	nodes, err := planNodesOf(plan)
	if err != nil {
		return domain.RerouteAuthorityInvalid, nil, err
	}
	prefix := domain.JudgeExecutedPrefix(nodes, trigger.Control(), trigger.Location().String())
	authority, blockers := domain.FoldAutoReroute(
		evidence.AutoRerouteForm,
		evidence.AutoRerouteImprovementThresholdMinor,
		trigger.Control().EstablishesControllableNode(),
		prefix.OnlyUnexecutedAffected(),
		evidence.UnresolvedRestrictions,
		evidence.OutstandingResponsibilities,
	)
	return authority, blockers, nil
}

func costImprovement(costs []domain.CandidateCostFact, current, next domain.CandidateID) (int64, bool) {
	var currentFact, nextFact domain.CandidateCostFact
	var haveCurrent, haveNext bool
	for _, cost := range costs {
		if cost.Candidate() == current {
			currentFact, haveCurrent = cost, true
		}
		if cost.Candidate() == next {
			nextFact, haveNext = cost, true
		}
	}
	if !haveCurrent || !haveNext {
		return 0, false
	}
	return domain.CostImprovementMinor(currentFact, nextFact)
}

// errRouteIdentityUnavailable 让调用方把「取号依赖故障」与领域故障分开转成未决续办。
var errRouteIdentityUnavailable = errors.New("network routing: route identity factory unavailable")

// evidenceSelection 是从收敛候选选路的结果：排序结局，以及选中时按证据组好的计划。
type evidenceSelection struct {
	ranking domain.RouteRanking
	plan    domain.InitialRoutePlan
}

// formPlanFromEvidence 从收敛候选选路，选中时按证据组计划；选不中的各格原样交回排序结局，
// 由调用方按自己那条线分派——形不成计划不是故障。证据答了候选却缺执行路径是装配坏，响亮
// 报错不静默。
//
// 复核形成的计划从这次触发的业务时间起生效，判断时刻仍取处理时钟。新计划是下一次复核的起点，那一次的失效时刻取它
// 自己触发的业务时间，不得早于生效（domain.PlanApplicability 的离场门）；生效取处理时钟，业务时间落后于处理时钟的
// 下一次触发就失效不了。按当前位置判据（ADR-0235），能判的下一次触发都不早于这一次。
func (handler *ReassessRouteHandler) formPlanFromEvidence(
	ctx context.Context,
	trigger domain.ReassessmentTrigger,
	candidates []domain.RouteCandidate,
	evidence ports.InitialRouteEvidence,
) (evidenceSelection, error) {
	key := trigger.Key()
	ranking, err := domain.RankRouteCandidates(evidence.RankingForm, candidates, evidence.CandidateCosts)
	if err != nil {
		return evidenceSelection{}, fmt.Errorf("rank route candidates: %w", err)
	}
	selected, chosen := ranking.Selected()
	if !chosen {
		return evidenceSelection{ranking: ranking}, nil
	}
	var legs []domain.PlannedLeg
	for _, path := range evidence.Paths {
		if path.Candidate == selected {
			legs = path.PlanLegs()
			break
		}
	}
	if len(legs) == 0 {
		return evidenceSelection{}, ErrIncompleteRouteEvidence
	}
	version, err := handler.deps.Identities.NextRoutePlanVersionID(ctx)
	if err != nil {
		return evidenceSelection{}, errRouteIdentityUnavailable
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
		EffectiveFrom: trigger.OccurredAt(),
	})
	if err != nil {
		return evidenceSelection{}, fmt.Errorf("form plan from evidence: %w", err)
	}
	return evidenceSelection{ranking: ranking, plan: plan}, nil
}

// reassessNoRoute 是原先无当前有效路由的那条线：初始路由本就无路由，或最近一份计划已失效、没有接班。候选评估完成且
// 有合格候选时形成首个当前有效计划（CONTEXT 生命周期），未决则继续保持无路由并单独记录评估未决。失效之后来到这里的，
// 形成的是新的计划版本，不是把原计划恢复回当前有效；也不是改路，不比改善阈值、不要求可控节点（ADR-0249 决定二）。
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

	selection, err := handler.formPlanFromEvidence(ctx, trigger, candidates, evidence)
	if errors.Is(err, errRouteIdentityUnavailable) {
		return handler.undecided(key, ReassessIdentityUnavailable), nil
	}
	if err != nil {
		return ReassessRouteResult{}, err
	}
	stillNoRoute := ports.ReassessmentRecord{
		Correlation:  trigger.Correlation(),
		Key:          key,
		Conclusion:   ports.ReassessmentPlanLapsed,
		ReassessedAt: handler.deps.Clock.Now(),
	}
	switch selection.ranking.Outcome() {
	case domain.RankingSelected:
	case domain.RankingNoQualifiedCandidate:
		stillNoRoute.CandidateState = ports.NoQualifiedCandidates
		return handler.commit(ctx, trigger, stillNoRoute)
	case domain.RankingTied,
		domain.RankingFormNotDeclared,
		domain.RankingCostsPending,
		domain.RankingCostsUnpriceable,
		domain.RankingCurrenciesDiffer:
		// 候选在而排不出唯一一条：不形成首个计划，候选评估记未决。并列在这条线上也不挂建议——
		// 这里没有被复核计划，改路三件落不了库（route_reassessment_reroute_on_reviewed_plan）。
		stillNoRoute.CandidateState = ports.CandidateReviewUndecided
		return handler.commit(ctx, trigger, stillNoRoute)
	default:
		return ReassessRouteResult{}, fmt.Errorf("network routing: unhandled ranking outcome %d",
			selection.ranking.Outcome())
	}
	record := ports.ReassessmentRecord{
		Correlation:    trigger.Correlation(),
		Key:            key,
		Conclusion:     ports.ReassessmentFirstPlanFormed,
		CandidateState: ports.CandidatesAvailable,
		NewPlan:        selection.plan,
		HasNewPlan:     true,
		ReassessedAt:   handler.deps.Clock.Now(),
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

// reviewCandidates 在失效路径上给出候选评估状态（另存，不拦失效）。并列算有候选：候选可行，
// 只是要授权角色裁；排不出唯一一条的各格与证据装配坏了都记未决，不冒充「没有合格候选」这个
// 业务结论。
func (handler *ReassessRouteHandler) reviewCandidates(evidence ports.InitialRouteEvidence) ports.CandidateReviewState {
	candidates, undecided, err := handler.evaluateCandidates(evidence)
	if err != nil || undecided {
		return ports.CandidateReviewUndecided
	}
	ranking, err := domain.RankRouteCandidates(evidence.RankingForm, candidates, evidence.CandidateCosts)
	if err != nil {
		return ports.CandidateReviewUndecided
	}
	switch ranking.Outcome() {
	case domain.RankingSelected, domain.RankingTied:
		return ports.CandidatesAvailable
	case domain.RankingNoQualifiedCandidate:
		return ports.NoQualifiedCandidates
	case domain.RankingFormNotDeclared,
		domain.RankingCostsPending,
		domain.RankingCostsUnpriceable,
		domain.RankingCurrenciesDiffer:
		return ports.CandidateReviewUndecided
	default:
		// 这里交不出错误：失效必须照常落库，候选评估不得拦它（硬句）。认不出的结局落「未决」——
		// 候选评估状态里只有它不下结论，不会让失效记录冒出一个没人判过的「有候选」或「无候选」。
		return ports.CandidateReviewUndecided
	}
}

// commit 提交复核记录。并发下另一方先提交时读回赢家。触发的业务时间在这里一处写上：每条提交路都经过这里，
// 漏写一条，那次复核就不再算进当前位置（ADR-0235）。
func (handler *ReassessRouteHandler) commit(
	ctx context.Context,
	trigger domain.ReassessmentTrigger,
	record ports.ReassessmentRecord,
) (ReassessRouteResult, error) {
	key := trigger.Key()
	record.TriggerOccurredAt = trigger.OccurredAt()
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

// notJudged 先把原因落进未判登记，再答`未判`。原因没落库就不得答这一格（护栏同 ADR-0094 决定五）：
// 消费门见`未判`就入账，原因随之只剩一份没人读的处理结果；落库失败改答登记册那一格的未决，
// 照旧回滚重投，下一轮重新走到这里。positionAt 只在触发早于当前位置时给。
func (handler *ReassessRouteHandler) notJudged(
	ctx context.Context,
	trigger domain.ReassessmentTrigger,
	reason ports.NotJudgedReason,
	positionAt time.Time,
) (ReassessRouteResult, error) {
	key := trigger.Key()
	if err := handler.deps.Store.RecordNotJudged(ctx, ports.NotJudgedTrigger{
		Correlation:       trigger.Correlation(),
		Key:               key,
		Reason:            reason,
		TriggerOccurredAt: trigger.OccurredAt(),
		CurrentPositionAt: positionAt,
		SettledAt:         handler.deps.Clock.Now(),
	}); err != nil {
		return handler.undecided(key, ReassessStoreUnavailable), nil
	}
	return ReassessRouteResult{outcome: ReassessTriggerNotJudged, notJudged: reason}, nil
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
	case ports.ReassessmentRerouted:
		return ReassessedRerouted
	default:
		return ReassessOutcomeInvalid
	}
}

// triggerDigest 是触发内容的稳定指纹：同关联异指纹即触发冲突。携带的地理投影也算进去，理由同 handoffDigest：同一
// 触发换了地址就不是重放。
func triggerDigest(trigger domain.ReassessmentTrigger, geo domain.GeoResolutionProjection) string {
	key := trigger.Key()
	digest := sha256.Sum256([]byte(strings.Join([]string{
		key.TenantID.String(),
		key.ShipmentRequestID.String(),
		key.DeclaredParcelID.String(),
		key.AcceptanceBaseline.String(),
		trigger.Control().String(),
		trigger.Location().String(),
		trigger.SourceVersion().String(),
		geo.ContentDigest(),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}
