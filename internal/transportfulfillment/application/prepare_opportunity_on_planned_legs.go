package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUnexpectedPreparationSave 说明准备结果库交回了封闭集合以外的写入结果。
var ErrUnexpectedPreparationSave = errors.New("transport fulfillment: unexpected transport preparation save outcome")

// PlannedRouteState 说触发这次准备的那份路由判断是什么结论。无路线判断也会到这里：它与计划已形成走同一封信，分支在
// 判断本体上，不在信封上。
type PlannedRouteState uint8

const (
	PlannedRouteStateInvalid PlannedRouteState = iota
	PlannedRouteFormed
	PlannedRouteNotFormed
)

// PrepareOnPlannedLegsCommand 是一份路由判断对一个载运对象交来的全部计划段。段已由消费侧适配器译成本上下文的读法
// （ADR-0201 决定二）。
type PrepareOnPlannedLegsCommand struct {
	TenantID domain.TenantID
	Object   string
	Route    PlannedRouteState
	Legs     []domain.PreparationLeg
}

// PreparationRunOutcome 是一次「按计划段准备」的整体答案。只有`未决`要重投：其余各格都是已经入账或本就不该写的答案。
type PreparationRunOutcome uint8

const (
	PreparationRunOutcomeInvalid PreparationRunOutcome = iota
	// PreparationRunAnswered：每一段都有了答案——新形成、已有结果或结果冲突。
	PreparationRunAnswered
	// PreparationRunNoRoute：路由判断是无当前路由，没有计划段可准备，零写入（ADR-0201 决定七）。
	PreparationRunNoRoute
	PreparationRunNotAccepted
	PreparationRunUndecided
)

func (outcome PreparationRunOutcome) String() string {
	switch outcome {
	case PreparationRunAnswered:
		return "PREPARATIONS_ANSWERED"
	case PreparationRunNoRoute:
		return "NO_ROUTE_NOTHING_TO_PREPARE"
	case PreparationRunNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case PreparationRunUndecided:
		return "PREPARATION_UNDECIDED"
	default:
		return ""
	}
}

// PreparationUndecidedReason 指名这次准备停在哪一口等谁。
type PreparationUndecidedReason uint8

const (
	PreparationUndecidedReasonNone PreparationUndecidedReason = iota
	DeclaredDemandUnavailable
	ScheduleCandidatesUnavailable
	CapacityPoolsUnavailable
	TransportPreparationStoreUnavailable
)

func (reason PreparationUndecidedReason) String() string {
	switch reason {
	case DeclaredDemandUnavailable:
		return "DECLARED_DEMAND_UNAVAILABLE"
	case ScheduleCandidatesUnavailable:
		return "SCHEDULE_CANDIDATES_UNAVAILABLE"
	case CapacityPoolsUnavailable:
		return "CAPACITY_POOLS_UNAVAILABLE"
	case TransportPreparationStoreUnavailable:
		return "TRANSPORT_PREPARATION_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// LegPreparationAnswer 是一段的答案种类。
type LegPreparationAnswer uint8

const (
	LegPreparationAnswerInvalid LegPreparationAnswer = iota
	LegPreparationFormed
	LegPreparationExisting
	LegPreparationConflict
)

func (answer LegPreparationAnswer) String() string {
	switch answer {
	case LegPreparationFormed:
		return "PREPARATION_FORMED"
	case LegPreparationExisting:
		return "EXISTING_RESULT"
	case LegPreparationConflict:
		return "RESULT_CONFLICT"
	default:
		return ""
	}
}

// LegPreparation 是一段的答案：新形成的结果，或库里已有的那一份（重放与冲突都交回原结果——冲突不覆盖，读的人要看见
// 挡住这一次的是什么）。
type LegPreparation struct {
	Answer LegPreparationAnswer
	Record ports.TransportPreparationRecord
}

type PrepareOnPlannedLegsResult struct {
	outcome      PreparationRunOutcome
	reason       PreparationUndecidedReason
	legs         []LegPreparation
	continuation string
}

func (result PrepareOnPlannedLegsResult) Outcome() PreparationRunOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result PrepareOnPlannedLegsResult) UndecidedReason() PreparationUndecidedReason {
	return result.reason
}

// Legs 按命令里段的次序交回各段答案；只在`已作答`时有内容。
func (result PrepareOnPlannedLegsResult) Legs() []LegPreparation {
	return append([]LegPreparation(nil), result.legs...)
}

func (result PrepareOnPlannedLegsResult) ContinuationReference() string {
	return result.continuation
}

// PrepareOnPlannedLegsDeps 是本编排的缝：申报需求量、班次候选口、池册与准备结果库。不接任何 Outbox：准备结果没有消费者，
// 不入队信封（ADR-0201 决定七、计划 X3）。
type PrepareOnPlannedLegsDeps struct {
	Demand       ports.DeclaredDemandSource
	Schedules    ports.ScheduleCandidateSource
	Pools        ports.CapacityPoolsOnSchedules
	Preparations ports.TransportPreparationStore
	Clock        ports.Clock
}

// PrepareOnPlannedLegsHandler 按一份路由判断上的全部计划段，逐段为一个载运对象形成运输准备结果（UC-TF-003 步 2–7）。
// 运营再准备请求（票 36）与改路后的新计划（票 34）复用它，各自由自己的入口把段交进来。
type PrepareOnPlannedLegsHandler struct {
	deps       PrepareOnPlannedLegsDeps
	candidates *ScheduleCandidatesHandler
}

func NewPrepareOnPlannedLegsHandler(deps PrepareOnPlannedLegsDeps) *PrepareOnPlannedLegsHandler {
	return &PrepareOnPlannedLegsHandler{
		deps:       deps,
		candidates: NewScheduleCandidatesHandler(ScheduleCandidatesDeps{Source: deps.Schedules}),
	}
}

// Prepare 先受理，再取一次需求量，再逐段作答。一段未决整次未决：调用方在同一笔事务里，整封回滚，前面已落的段行一并
// 回滚，不留半封结果；重投从头判，结论可能因登记册其间变了而不同。`已有结果`只在同一身份已由另一次提交落过时出现
// （同键的另一封信、并发撞键）。
//
// 译不过去的申报与读不懂的库行不答未决，原样以 error 上抛：重投不会让它们变好（ADR-0029）。
func (handler *PrepareOnPlannedLegsHandler) Prepare(
	ctx context.Context,
	command PrepareOnPlannedLegsCommand,
) (PrepareOnPlannedLegsResult, error) {
	notAccepted := PrepareOnPlannedLegsResult{outcome: PreparationRunNotAccepted}
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return notAccepted, nil
	}
	switch command.Route {
	case PlannedRouteNotFormed:
		// 无路线判断零写入：没有段，就没有可以挂准备结果的身份；记一份「无路线」的准备结果等于替 NR 复述它的结论。
		return PrepareOnPlannedLegsResult{outcome: PreparationRunNoRoute}, nil
	case PlannedRouteFormed:
	default:
		return notAccepted, nil
	}
	object, err := domain.NewCarriedObjectReference(command.Object)
	if err != nil || len(command.Legs) == 0 {
		return notAccepted, nil
	}
	seen := map[string]bool{}
	for _, leg := range command.Legs {
		if strings.TrimSpace(leg.Reference().String()) == "" || seen[leg.Reference().String()] {
			return notAccepted, nil
		}
		seen[leg.Reference().String()] = true
	}

	demand, err := handler.deps.Demand.LoadDeclaredDemand(ctx, command.TenantID, object)
	if errors.Is(err, ports.ErrDeclaredDemandUntranslatable) {
		return PrepareOnPlannedLegsResult{}, err
	}
	if err != nil {
		return preparationUndecided(DeclaredDemandUnavailable, command.TenantID.String(), object.String()), nil
	}
	rule := domain.BuiltInPreparationRuleVersion()
	legs := make([]LegPreparation, 0, len(command.Legs))
	for _, leg := range command.Legs {
		answered, undecided, err := handler.prepareLeg(ctx, command.TenantID, object, rule, demand, leg)
		if err != nil {
			return PrepareOnPlannedLegsResult{}, err
		}
		if undecided != nil {
			return *undecided, nil
		}
		legs = append(legs, answered)
	}
	return PrepareOnPlannedLegsResult{outcome: PreparationRunAnswered, legs: legs}, nil
}

// prepareLeg 先按身份看有没有结果：有就按依据摘要分重放与冲突，不再判——判断依赖此刻的登记册，重投时再判一次可能
// 判出另一个结论，而同一依据只该有一份当前有效结果（UC-TF-003「一致性」）。
func (handler *PrepareOnPlannedLegsHandler) prepareLeg(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
	rule domain.PreparationRuleVersion,
	demand domain.DemandSnapshot,
	leg domain.PreparationLeg,
) (LegPreparation, *PrepareOnPlannedLegsResult, error) {
	key := ports.TransportPreparationKey{TenantID: tenant, Leg: leg.Reference(), Object: object, Rule: rule}
	subject := []string{tenant.String(), leg.Reference().String(), object.String()}
	undecided := func(reason PreparationUndecidedReason) (LegPreparation, *PrepareOnPlannedLegsResult, error) {
		result := preparationUndecided(reason, subject...)
		return LegPreparation{}, &result, nil
	}
	digest, err := domain.CanonicalizePreparationInput(leg, demand)
	if err != nil {
		return LegPreparation{}, nil, err
	}
	existing, found, err := handler.deps.Preparations.FindByKey(ctx, key)
	if errors.Is(err, ports.ErrTransportPreparationRecordUnreadable) {
		return LegPreparation{}, nil, err
	}
	if err != nil {
		return undecided(TransportPreparationStoreUnavailable)
	}
	if found {
		answered, err := preparationReplayOrConflict(existing, digest)
		if err != nil {
			return LegPreparation{}, nil, err
		}
		return answered, nil, nil
	}

	spec := domain.PrepareTransportOpportunitySpec{
		TenantID: tenant,
		Leg:      leg,
		Object:   object,
		Rule:     rule,
		Demand:   demand,
		JudgedAt: handler.deps.Clock.Now(),
	}
	listed, err := handler.candidates.Candidates(ctx, ScheduleCandidatesQuery{
		TenantID:   tenant,
		Connection: leg.Direction().Connection().String(),
		DepartFrom: leg.WindowStart(),
		DepartTo:   leg.WindowEnd(),
	})
	if err != nil {
		return LegPreparation{}, nil, err
	}
	switch listed.Outcome() {
	case ScheduleCandidatesNotConfigured:
	case ScheduleCandidatesListed:
		spec.Configured = true
		if spec.Candidates, err = handler.candidateInputs(ctx, tenant, listed.Entries()); err != nil {
			return undecided(CapacityPoolsUnavailable)
		}
	case ScheduleCandidatesUndecided:
		return undecided(ScheduleCandidatesUnavailable)
	default:
		// 段已过受理门，候选口仍不受理，是两边对「段」的认法分了叉，不是等谁。
		return LegPreparation{}, nil, fmt.Errorf("transport fulfillment: schedule candidates answered %q for leg %q",
			listed.Outcome(), leg.Reference())
	}

	preparation, err := domain.PrepareTransportOpportunity(spec)
	if err != nil {
		return LegPreparation{}, nil, err
	}
	record := ports.TransportPreparationRecord{
		Key:         key,
		InputDigest: digest,
		Preparation: preparation,
		RecordedAt:  handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Preparations.Save(ctx, record)
	if err != nil {
		return undecided(TransportPreparationStoreUnavailable)
	}
	switch saved {
	case ports.TransportPreparationSaved:
		return LegPreparation{Answer: LegPreparationFormed, Record: record}, nil, nil
	case ports.TransportPreparationAlreadyRecorded:
		// 并发下另一方先落了同一身份：读回赢家按依据分重放与冲突，不宣称自己形成了它。
		winner, found, err := handler.deps.Preparations.FindByKey(ctx, key)
		if errors.Is(err, ports.ErrTransportPreparationRecordUnreadable) {
			return LegPreparation{}, nil, err
		}
		if err != nil || !found {
			return undecided(TransportPreparationStoreUnavailable)
		}
		answered, err := preparationReplayOrConflict(winner, digest)
		if err != nil {
			return LegPreparation{}, nil, err
		}
		return answered, nil, nil
	default:
		return LegPreparation{}, nil, fmt.Errorf("%w: %d", ErrUnexpectedPreparationSave, saved)
	}
}

// candidateInputs 给每个候选配池：登过的班次一次成批问池册，没登过的用候选口按周期安排声明物化出的池。
func (handler *PrepareOnPlannedLegsHandler) candidateInputs(
	ctx context.Context,
	tenant domain.TenantID,
	entries []ScheduleCandidate,
) ([]domain.PreparationCandidateInput, error) {
	var registered []domain.ScheduleReference
	for _, entry := range entries {
		if entry.Registered {
			registered = append(registered, entry.Schedule.Schedule())
		}
	}
	poolsBySchedule := map[string][]domain.CapacityPool{}
	if len(registered) > 0 {
		records, err := handler.deps.Pools.ListPoolsOnSchedules(ctx, tenant, registered)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			schedule := record.Pool.Schedule().String()
			poolsBySchedule[schedule] = append(poolsBySchedule[schedule], record.Pool)
		}
	}
	inputs := make([]domain.PreparationCandidateInput, 0, len(entries))
	for _, entry := range entries {
		input := domain.PreparationCandidateInput{Schedule: entry.Schedule, Registered: entry.Registered}
		if entry.Registered {
			input.Pools = poolsBySchedule[entry.Schedule.Schedule().String()]
		} else {
			input.Pools = entry.TemplatePools
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}

// preparationReplayOrConflict 比库里那一份的依据摘要：同即`已有结果`，异即`结果冲突`，两格都交回原结果、不覆盖。认不出
// 摘要形状就分不了这两格，那是坏行，答读不懂。
func preparationReplayOrConflict(existing ports.TransportPreparationRecord, digest string) (LegPreparation, error) {
	switch domain.CompareStoredDigest(existing.InputDigest, digest, "") {
	case domain.SamePayload:
		return LegPreparation{Answer: LegPreparationExisting, Record: existing}, nil
	case domain.DifferentPayload:
		return LegPreparation{Answer: LegPreparationConflict, Record: existing}, nil
	default:
		return LegPreparation{}, fmt.Errorf("%w: leg %q object %q: stored input digest shape is not recognized",
			ports.ErrTransportPreparationRecordUnreadable, existing.Key.Leg, existing.Key.Object)
	}
}

func preparationUndecided(reason PreparationUndecidedReason, subject ...string) PrepareOnPlannedLegsResult {
	digest := sha256.Sum256([]byte(strings.Join(append([]string{reason.String()}, subject...), "\x00")))
	return PrepareOnPlannedLegsResult{
		outcome:      PreparationRunUndecided,
		reason:       reason,
		continuation: "CONT-" + hex.EncodeToString(digest[:8]),
	}
}
