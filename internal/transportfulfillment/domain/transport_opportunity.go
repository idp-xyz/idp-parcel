package domain

import (
	"errors"
	"math/big"
	"slices"
	"strings"
	"time"
)

// ErrInvalidTransportPreparation：运输准备结果不成立——身份缺格、段窗口倒挂、候选与池对不上，或读回的结论与候选依据
// 推不到一处。
var ErrInvalidTransportPreparation = errors.New("transport fulfillment: invalid transport preparation")

// PreparationRuleVersion 指名一次运输准备所按的规则：候选条件、择优次序、需求量取法与单位表（ADR-0201 决定六、八）。
// 它是准备结果身份的一维：内置规则换了，同一段同一包裹按新规则再判一次是另一份结果，旧结果不被改写。
type PreparationRuleVersion struct{ requiredValue }

func NewPreparationRuleVersion(value string) (PreparationRuleVersion, error) {
	required, err := newRequiredValue("preparation rule version", value)
	return PreparationRuleVersion{required}, err
}

// builtInPreparationRule 是今天内置的那一种形（ADR-0146 决定五：每个决定点至少一种内置形）。改了择优、取量或单位表
// 中的任何一条就该换这个串，否则新旧两种判法会在同一身份上撞成`结果冲突`。
const builtInPreparationRule = "TF-PREPARE/1"

// BuiltInPreparationRuleVersion 交回内置规则的版本。
func BuiltInPreparationRuleVersion() PreparationRuleVersion {
	return PreparationRuleVersion{requiredValue{value: builtInPreparationRule}}
}

// PreparationOutcome 是一段计划履约段对一个载运对象的准备结论（UC-TF-003 结果语义契约，ADR-0201 决定六）。`部分准备`
// 不在这里：单对象请求只有一个对象，没有「一部分准备好」可言，那一格留给批量请求。
type PreparationOutcome uint8

const (
	PreparationOutcomeInvalid PreparationOutcome = iota
	// PreparationNotConfigured：该连接上一份周期安排、一个具体班次都没登过。缺的是租户取值，不是依赖故障，也不是
	// 「这段时间没有车」——入账、不重投、不建占位班次（ADR-0094 判据）。
	PreparationNotConfigured
	// PreparationNoCurrentOpportunity：登过，但段窗口内没有合格候选；带候选与淘汰原因，不建占位班次（AT-TF-028）。
	PreparationNoCurrentOpportunity
	// PreparationCapacityUnconfirmed：有合格候选，但没有一班能确认容量够——没有池、池的维度没有需求量，或单位不可换算；
	// 也没有哪一班已确认够。不以默认容量继续。
	PreparationCapacityUnconfirmed
	// PreparationOpportunityUnavailable：有合格候选，每一班都至少在一个维度上硬容量不够。
	PreparationOpportunityUnavailable
	// PreparationOpportunityPrepared：按出发先后取到的第一班，已登池的每一维容量都够。不预占：准备只说这一班此刻放得下。
	PreparationOpportunityPrepared
)

func (outcome PreparationOutcome) String() string {
	switch outcome {
	case PreparationNotConfigured:
		return "NOT_CONFIGURED"
	case PreparationNoCurrentOpportunity:
		return "NO_CURRENT_OPPORTUNITY"
	case PreparationCapacityUnconfirmed:
		return "CAPACITY_UNCONFIRMED"
	case PreparationOpportunityUnavailable:
		return "OPPORTUNITY_UNAVAILABLE"
	case PreparationOpportunityPrepared:
		return "OPPORTUNITY_PREPARED"
	default:
		return ""
	}
}

// ParsePreparationOutcome 把 String 写出的词认回来。
func ParsePreparationOutcome(word string) (PreparationOutcome, error) {
	for outcome := PreparationNotConfigured; outcome <= PreparationOpportunityPrepared; outcome++ {
		if outcome.String() == word {
			return outcome, nil
		}
	}
	return PreparationOutcomeInvalid, ErrInvalidTransportPreparation
}

// CandidateVerdict 是一个候选班次在本次准备里的判词：前四个是淘汰原因（ADR-0201 决定六的候选条件），后三个是合格候选
// 的容量判断。
type CandidateVerdict uint8

const (
	CandidateVerdictInvalid CandidateVerdict = iota
	CandidateConnectionMismatch
	CandidateNodesMismatch
	CandidateDepartsBeforeWindow
	CandidateArrivesAfterWindow
	CandidateCapacitySufficient
	CandidateCapacityInsufficient
	CandidateCapacityUnconfirmed
)

func (verdict CandidateVerdict) String() string {
	switch verdict {
	case CandidateConnectionMismatch:
		return "CONNECTION_MISMATCH"
	case CandidateNodesMismatch:
		return "NODES_MISMATCH"
	case CandidateDepartsBeforeWindow:
		return "DEPARTS_BEFORE_WINDOW"
	case CandidateArrivesAfterWindow:
		return "ARRIVES_AFTER_WINDOW"
	case CandidateCapacitySufficient:
		return "CAPACITY_SUFFICIENT"
	case CandidateCapacityInsufficient:
		return "CAPACITY_INSUFFICIENT"
	case CandidateCapacityUnconfirmed:
		return "CAPACITY_UNCONFIRMED"
	default:
		return ""
	}
}

// ParseCandidateVerdict 把 String 写出的词认回来。
func ParseCandidateVerdict(word string) (CandidateVerdict, error) {
	for verdict := CandidateConnectionMismatch; verdict <= CandidateCapacityUnconfirmed; verdict++ {
		if verdict.String() == word {
			return verdict, nil
		}
	}
	return CandidateVerdictInvalid, ErrInvalidTransportPreparation
}

func (verdict CandidateVerdict) eliminated() bool {
	return verdict >= CandidateConnectionMismatch && verdict <= CandidateArrivesAfterWindow
}

// PoolVerdict 是一个池在一个候选上的判词。
type PoolVerdict uint8

const (
	PoolVerdictInvalid PoolVerdict = iota
	PoolCapacitySufficient
	PoolCapacityInsufficient
	// PoolDemandPending：需求量快照在这一维上没有数量——申报里没有来源（袋位、托盘位、舱位），或客户没报。`待补充`。
	PoolDemandPending
	// PoolUnitNotConvertible：池或需求的单位不在内置单位表里，或量纲不属这一维。不替登记方猜单位。
	PoolUnitNotConvertible
)

func (verdict PoolVerdict) String() string {
	switch verdict {
	case PoolCapacitySufficient:
		return "SUFFICIENT"
	case PoolCapacityInsufficient:
		return "INSUFFICIENT"
	case PoolDemandPending:
		return "DEMAND_PENDING"
	case PoolUnitNotConvertible:
		return "UNIT_NOT_CONVERTIBLE"
	default:
		return ""
	}
}

// ParsePoolVerdict 把 String 写出的词认回来。
func ParsePoolVerdict(word string) (PoolVerdict, error) {
	for verdict := PoolCapacitySufficient; verdict <= PoolUnitNotConvertible; verdict++ {
		if verdict.String() == word {
			return verdict, nil
		}
	}
	return PoolVerdictInvalid, ErrInvalidTransportPreparation
}

// PreparationLeg 是本上下文对一段计划履约段的读法：段引用、方向（连接代码加起讫节点）与计划时间窗口。怎样从 NR 的
// 计划段读出这几格归消费侧适配器（ADR-0201 决定二），这里只认本上下文的类型——方向与班次方向同一个类型，比对因此是
// 同类值相比，不是两套拼写互猜。
type PreparationLeg struct {
	reference   PlannedSegmentReference
	direction   ScheduleDirection
	windowStart time.Time
	windowEnd   time.Time
}

// NewPreparationLeg 立一段：引用与方向必备，窗口不得倒挂。窗口起讫相等合法——NR 的窗口是区间，退化成一点的区间
// 只是少见。
func NewPreparationLeg(reference PlannedSegmentReference, direction ScheduleDirection, windowStart, windowEnd time.Time) (PreparationLeg, error) {
	if !reference.valid() || !direction.valid() || windowStart.IsZero() || windowEnd.IsZero() || windowEnd.Before(windowStart) {
		return PreparationLeg{}, ErrInvalidTransportPreparation
	}
	return PreparationLeg{
		reference:   reference,
		direction:   direction,
		windowStart: AtRegistryPrecision(windowStart),
		windowEnd:   AtRegistryPrecision(windowEnd),
	}, nil
}

func (leg PreparationLeg) Reference() PlannedSegmentReference {
	return leg.reference
}

func (leg PreparationLeg) Direction() ScheduleDirection {
	return leg.direction
}

func (leg PreparationLeg) WindowStart() time.Time {
	return leg.windowStart
}

func (leg PreparationLeg) WindowEnd() time.Time {
	return leg.windowEnd
}

func (leg PreparationLeg) valid() bool {
	return leg.reference.valid() && leg.direction.valid() && !leg.windowStart.IsZero() && !leg.windowEnd.Before(leg.windowStart)
}

// PoolAssessment 是一个池在一个候选上的判断依据：池、维度、池的单位、判断时点的可用量、换成池单位的需求量与判词。
// 需求量换不成池单位时 Requested 为空串。它是留给人看的依据行，各格都照判断当时写下，读回不重算。
type PoolAssessment struct {
	Pool      CapacityPoolReference
	Dimension CapacityDimension
	Unit      QuantityUnitReference
	Available int64
	Requested string
	Verdict   PoolVerdict
}

func (assessment PoolAssessment) valid() bool {
	if !assessment.Pool.valid() || !assessment.Dimension.valid() || !assessment.Unit.valid() || assessment.Verdict == PoolVerdictInvalid {
		return false
	}
	comparable := assessment.Verdict == PoolCapacitySufficient || assessment.Verdict == PoolCapacityInsufficient
	return comparable == (assessment.Requested != "")
}

// PreparationCandidate 是一个候选班次在本次准备里的依据行：班次身份、登记册上的版本（只在评估时物化、没登过的那一班
// 记 0）、出发与到达、判词与逐池依据。淘汰的候选不带池依据——它在容量之前就出局了。
type PreparationCandidate struct {
	Schedule  ScheduleReference
	Version   ScheduleVersion
	DepartsAt time.Time
	ArrivesAt time.Time
	Verdict   CandidateVerdict
	Pools     []PoolAssessment
}

// Registered 报告这一班在登记册上有没有当前版。没登过的班次是准备评估时按周期安排物化、只算不存的那一班（ADR-0201
// 决定五）；后续订舱、装载分配要引用它，得先显式登记。
func (candidate PreparationCandidate) Registered() bool {
	return candidate.Version >= 1
}

func (candidate PreparationCandidate) valid() bool {
	if !candidate.Schedule.valid() || candidate.Version < 0 || candidate.DepartsAt.IsZero() ||
		!candidate.ArrivesAt.After(candidate.DepartsAt) || candidate.Verdict == CandidateVerdictInvalid {
		return false
	}
	if candidate.Verdict.eliminated() {
		return len(candidate.Pools) == 0
	}
	for _, pool := range candidate.Pools {
		if !pool.valid() {
			return false
		}
	}
	return capacityVerdictOf(candidate.Pools) == candidate.Verdict
}

// PreparationCandidateInput 是交给评估的一个候选：班次、它在不在登记册上、它的池。登过的班次带登记册上的池（含预占）；
// 只在评估时物化的那一班带周期安排容量声明物化出的池，只算不存。
type PreparationCandidateInput struct {
	Schedule   TransportSchedule
	Registered bool
	Pools      []CapacityPool
}

// PrepareTransportOpportunitySpec 是一次准备评估的全部输入。Configured 为假即该连接空册，Candidates 此时不读。
type PrepareTransportOpportunitySpec struct {
	TenantID   TenantID
	Leg        PreparationLeg
	Object     CarriedObjectReference
	Rule       PreparationRuleVersion
	Demand     DemandSnapshot
	Configured bool
	Candidates []PreparationCandidateInput
	JudgedAt   time.Time
}

// TransportPreparation 是一段计划履约段对一个载运对象形成的运输准备结果（UC-TF-003）：结论、按什么需求量判的、看过哪些
// 候选、每一班为什么出局或够不够。它不是预占、不是订舱，也不建班次——选中的那一班可能只是评估时物化出来的。
type TransportPreparation struct {
	tenantID   TenantID
	leg        PreparationLeg
	object     CarriedObjectReference
	rule       PreparationRuleVersion
	demand     DemandSnapshot
	outcome    PreparationOutcome
	candidates []PreparationCandidate
	selected   ScheduleReference
	judgedAt   time.Time
}

// PrepareTransportOpportunity 按内置的那一种形评估一段（ADR-0201 决定六，产品策略）：
//
//   - 候选条件：连接代码相同、起讫相符、出发不早于窗口起点、到达不晚于窗口终点；不合的记淘汰原因，连同候选一起留下；
//   - 排序：出发最早优先，同刻按班次标识；
//   - 取第一个已登池每一维都够的，答`运输机会已准备`；一个都不够而有哪一班容量说不准，答`容量待确认`；每一班都有一维
//     硬容量不够，答`当前机会不可用`；没有合格候选答`无当前运输机会`。
//
// 判断时点的可用量取 JudgedAt：预占在那一刻的占用由池自己算。不预占、不建占位班次——本函数造不出任何班次或预占。
func PrepareTransportOpportunity(spec PrepareTransportOpportunitySpec) (TransportPreparation, error) {
	if !spec.TenantID.valid() || !spec.Leg.valid() || !spec.Object.valid() || !spec.Rule.valid() ||
		!spec.Demand.valid() || spec.JudgedAt.IsZero() {
		return TransportPreparation{}, ErrInvalidTransportPreparation
	}
	judgedAt := AtRegistryPrecision(spec.JudgedAt)
	preparation := TransportPreparation{
		tenantID: spec.TenantID,
		leg:      spec.Leg,
		object:   spec.Object,
		rule:     spec.Rule,
		demand:   spec.Demand,
		judgedAt: judgedAt,
	}
	if !spec.Configured {
		preparation.outcome = PreparationNotConfigured
		return preparation, nil
	}

	candidates := make([]PreparationCandidate, 0, len(spec.Candidates))
	for _, input := range spec.Candidates {
		candidate, err := assessCandidate(spec.TenantID, spec.Leg, spec.Demand, input, judgedAt)
		if err != nil {
			return TransportPreparation{}, err
		}
		candidates = append(candidates, candidate)
	}
	slices.SortFunc(candidates, compareCandidates)
	preparation.candidates = candidates
	preparation.outcome, preparation.selected = outcomeOf(candidates)
	return preparation, nil
}

func assessCandidate(
	tenant TenantID,
	leg PreparationLeg,
	demand DemandSnapshot,
	input PreparationCandidateInput,
	at time.Time,
) (PreparationCandidate, error) {
	schedule := input.Schedule
	if schedule.TenantID() != tenant || !schedule.Schedule().valid() {
		return PreparationCandidate{}, ErrInvalidTransportPreparation
	}
	candidate := PreparationCandidate{
		Schedule:  schedule.Schedule(),
		DepartsAt: schedule.DepartsAt(),
		ArrivesAt: schedule.ArrivesAt(),
	}
	if input.Registered {
		candidate.Version = schedule.Version()
	}
	switch {
	case schedule.Direction().Connection() != leg.direction.Connection():
		candidate.Verdict = CandidateConnectionMismatch
	case schedule.Direction().From() != leg.direction.From() || schedule.Direction().To() != leg.direction.To():
		candidate.Verdict = CandidateNodesMismatch
	case schedule.DepartsAt().Before(leg.windowStart):
		candidate.Verdict = CandidateDepartsBeforeWindow
	case schedule.ArrivesAt().After(leg.windowEnd):
		candidate.Verdict = CandidateArrivesAfterWindow
	}
	if candidate.Verdict != CandidateVerdictInvalid {
		return candidate, nil
	}

	pools := append([]CapacityPool(nil), input.Pools...)
	slices.SortFunc(pools, func(left, right CapacityPool) int {
		return CompareCapacityDimensions(left.Dimension(), right.Dimension())
	})
	for index, pool := range pools {
		if pool.TenantID() != tenant || pool.Schedule() != schedule.Schedule() ||
			(index > 0 && pools[index-1].Dimension() == pool.Dimension()) {
			return PreparationCandidate{}, ErrInvalidTransportPreparation
		}
		candidate.Pools = append(candidate.Pools, assessPool(pool, demand, at))
	}
	candidate.Verdict = capacityVerdictOf(candidate.Pools)
	return candidate, nil
}

// assessPool 判一个池：需求量换到池的单位后与池在判断时点的可用量比。比用精确有理数；写下的需求量是给人看的那一格。
func assessPool(pool CapacityPool, demand DemandSnapshot, at time.Time) PoolAssessment {
	assessment := PoolAssessment{
		Pool:      pool.Pool(),
		Dimension: pool.Dimension(),
		Unit:      pool.Unit(),
		Available: pool.AvailableAt(at),
	}
	quantity, present := demand.quantityFor(pool.Dimension())
	if !present {
		assessment.Verdict = PoolDemandPending
		return assessment
	}
	amount, _ := parsePositiveDecimal(quantity.Amount)
	requested, convertible := convertQuantity(pool.Dimension(), amount, quantity.Unit, pool.Unit())
	if !convertible {
		assessment.Verdict = PoolUnitNotConvertible
		return assessment
	}
	assessment.Requested = trimDecimal(requested.FloatString(requestedDecimalPlaces))
	if requested.Cmp(new(big.Rat).SetInt64(assessment.Available)) <= 0 {
		assessment.Verdict = PoolCapacitySufficient
	} else {
		assessment.Verdict = PoolCapacityInsufficient
	}
	return assessment
}

// capacityVerdictOf 由逐池判词得出候选的容量判词：一维硬不够这一班就放不下，不论别的维说不说得准；没有不够的一维而有
// 说不准的，或者根本没有池，就是说不准；每一维都够才算够。
func capacityVerdictOf(pools []PoolAssessment) CandidateVerdict {
	if len(pools) == 0 {
		return CandidateCapacityUnconfirmed
	}
	unconfirmed := false
	for _, pool := range pools {
		switch pool.Verdict {
		case PoolCapacityInsufficient:
			return CandidateCapacityInsufficient
		case PoolDemandPending, PoolUnitNotConvertible:
			unconfirmed = true
		}
	}
	if unconfirmed {
		return CandidateCapacityUnconfirmed
	}
	return CandidateCapacitySufficient
}

// outcomeOf 由排好序的候选判词得出结论与选中的那一班。读回时用同一条推一遍，对不上即坏行。
func outcomeOf(candidates []PreparationCandidate) (PreparationOutcome, ScheduleReference) {
	qualified, unconfirmed := false, false
	for _, candidate := range candidates {
		switch candidate.Verdict {
		case CandidateCapacitySufficient:
			return PreparationOpportunityPrepared, candidate.Schedule
		case CandidateCapacityUnconfirmed:
			qualified, unconfirmed = true, true
		case CandidateCapacityInsufficient:
			qualified = true
		}
	}
	switch {
	case !qualified:
		return PreparationNoCurrentOpportunity, ScheduleReference{}
	case unconfirmed:
		return PreparationCapacityUnconfirmed, ScheduleReference{}
	default:
		return PreparationOpportunityUnavailable, ScheduleReference{}
	}
}

func compareCandidates(left, right PreparationCandidate) int {
	if order := left.DepartsAt.Compare(right.DepartsAt); order != 0 {
		return order
	}
	return strings.Compare(left.Schedule.String(), right.Schedule.String())
}

// RehydrateTransportPreparationSpec 是读回一份准备结果所需的全部。
type RehydrateTransportPreparationSpec struct {
	TenantID   TenantID
	Leg        PreparationLeg
	Object     CarriedObjectReference
	Rule       PreparationRuleVersion
	Demand     DemandSnapshot
	Outcome    PreparationOutcome
	Candidates []PreparationCandidate
	Selected   ScheduleReference
	JudgedAt   time.Time
}

// RehydrateTransportPreparation 是持久化适配器读回一份结果的门：逐格过构造门，并核结论、选中班次与候选依据推得到一处
// ——空册不带候选，候选按出发先后排，每个候选的判词与它的池依据相符，结论与选中的班次正是按同一条择优推出来的那个。
// 只校不重判：可用量、需求量换算照存下的写，不按此刻的池重算。
func RehydrateTransportPreparation(spec RehydrateTransportPreparationSpec) (TransportPreparation, error) {
	if !spec.TenantID.valid() || !spec.Leg.valid() || !spec.Object.valid() || !spec.Rule.valid() ||
		!spec.Demand.valid() || spec.JudgedAt.IsZero() || spec.Outcome == PreparationOutcomeInvalid {
		return TransportPreparation{}, ErrInvalidTransportPreparation
	}
	candidates := append([]PreparationCandidate(nil), spec.Candidates...)
	for index, candidate := range candidates {
		if !candidate.valid() || (index > 0 && compareCandidates(candidates[index-1], candidate) >= 0) {
			return TransportPreparation{}, ErrInvalidTransportPreparation
		}
		candidates[index].Pools = append([]PoolAssessment(nil), candidate.Pools...)
	}
	if spec.Outcome == PreparationNotConfigured {
		if len(candidates) != 0 || spec.Selected.valid() {
			return TransportPreparation{}, ErrInvalidTransportPreparation
		}
	} else if outcome, selected := outcomeOf(candidates); outcome != spec.Outcome || selected != spec.Selected {
		return TransportPreparation{}, ErrInvalidTransportPreparation
	}
	return TransportPreparation{
		tenantID:   spec.TenantID,
		leg:        spec.Leg,
		object:     spec.Object,
		rule:       spec.Rule,
		demand:     spec.Demand,
		outcome:    spec.Outcome,
		candidates: candidates,
		selected:   spec.Selected,
		judgedAt:   AtRegistryPrecision(spec.JudgedAt),
	}, nil
}

func (preparation TransportPreparation) TenantID() TenantID {
	return preparation.tenantID
}

func (preparation TransportPreparation) Leg() PreparationLeg {
	return preparation.leg
}

func (preparation TransportPreparation) Object() CarriedObjectReference {
	return preparation.object
}

func (preparation TransportPreparation) Rule() PreparationRuleVersion {
	return preparation.rule
}

func (preparation TransportPreparation) Demand() DemandSnapshot {
	return preparation.demand
}

func (preparation TransportPreparation) Outcome() PreparationOutcome {
	return preparation.outcome
}

// Candidates 交回按出发先后排的候选依据拷贝；空册时为空。
func (preparation TransportPreparation) Candidates() []PreparationCandidate {
	copied := make([]PreparationCandidate, 0, len(preparation.candidates))
	for _, candidate := range preparation.candidates {
		candidate.Pools = append([]PoolAssessment(nil), candidate.Pools...)
		copied = append(copied, candidate)
	}
	return copied
}

// Selected 只在`运输机会已准备`时交回选中的那一班。
func (preparation TransportPreparation) Selected() (ScheduleReference, bool) {
	return preparation.selected, preparation.selected.valid()
}

func (preparation TransportPreparation) JudgedAt() time.Time {
	return preparation.judgedAt
}
