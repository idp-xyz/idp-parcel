package domain

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// 本文件是「动作准入判断」的领域折叠（ADR-0206）：对一个对象、一次动作意图、一个动作时点，逐适用动作把
// 三源——内部合规限制、放行门禁核对、外部放行结果——折成封闭答复。三源各自的判断仍在各自那里（限制适用性
// 在 regulatory_restriction.go，门禁结论在 duty_release.go 的 FoldGateConclusion，放行结果由 UC-CC-006
// 接收）；这里只决定三者怎么合在一起说「这一步能不能走」，不重算任何一源。

var (
	ErrInvalidAdmissionInput = errors.New("customs compliance: invalid action admission input")
	ErrGateReadingMissing    = errors.New("customs compliance: a declaration scope has no gate reading for an applicable action")
)

// AdmissionAnswer 是动作准入判断的封闭五格。取值次序即折叠时的压制次序（大者压小者）：阻断压过一切——
// 外部放行不解内部限制、门禁满足不答放行；未配置压过未决——缺配置的那一格重投不会自己好，要人去配；
// 未决压过放行——还缺东西就不能说能走；放行压过不受门禁——有一道门过了，就不是「本就没有门」。
type AdmissionAnswer uint8

const (
	AdmissionAnswerInvalid AdmissionAnswer = iota
	AdmissionNotGuarded
	AdmissionAdmitted
	AdmissionUndecided
	AdmissionNotConfigured
	AdmissionBlocked
)

func (answer AdmissionAnswer) valid() bool {
	return answer >= AdmissionNotGuarded && answer <= AdmissionBlocked
}

func (answer AdmissionAnswer) String() string {
	switch answer {
	case AdmissionNotGuarded:
		return "NOT_GUARDED"
	case AdmissionAdmitted:
		return "ADMITTED"
	case AdmissionUndecided:
		return "UNDECIDED"
	case AdmissionNotConfigured:
		return "NOT_CONFIGURED"
	case AdmissionBlocked:
		return "BLOCKED"
	default:
		return ""
	}
}

// AdmissionCause 是一条答复依据的种类。每种依据只属于一格答复（Answer）：答复由依据推出，不另存一份可与
// 依据对不上的结论。
type AdmissionCause uint8

const (
	AdmissionCauseInvalid AdmissionCause = iota

	// 不受门禁
	CauseGateNotApplicable
	CauseOutsideCustomsBooks

	// 放行
	CauseReleasedInFull

	// 未决
	CauseCustomsScopeUnresolved
	CauseDeclarationUnitNotFormed
	CauseDutyVerificationAbsent
	CauseDutyVerificationPending
	CauseReleasePartialOnly
	CauseReleaseConditionalOnly
	CauseGateReleaseDiscrepancy

	// 未配置
	CauseNodeZoneNotRegistered
	CauseGateCatalogNotRegistered
	CauseDutyPaymentRuleNotRegistered
	CauseDutyPaymentFindingBesideRule

	// 阻断
	CauseRestrictionInForce
	CauseGateUnmet
	CauseGatePartiallyMet
	CauseGateConflicting
	CauseReleaseNotReceived
)

// Answer 是这种依据所属的那一格。
func (cause AdmissionCause) Answer() AdmissionAnswer {
	switch cause {
	case CauseGateNotApplicable, CauseOutsideCustomsBooks:
		return AdmissionNotGuarded
	case CauseReleasedInFull:
		return AdmissionAdmitted
	case CauseCustomsScopeUnresolved, CauseDeclarationUnitNotFormed,
		CauseDutyVerificationAbsent, CauseDutyVerificationPending,
		CauseReleasePartialOnly, CauseReleaseConditionalOnly, CauseGateReleaseDiscrepancy:
		return AdmissionUndecided
	case CauseNodeZoneNotRegistered, CauseGateCatalogNotRegistered,
		CauseDutyPaymentRuleNotRegistered, CauseDutyPaymentFindingBesideRule:
		return AdmissionNotConfigured
	case CauseRestrictionInForce, CauseGateUnmet, CauseGatePartiallyMet,
		CauseGateConflicting, CauseReleaseNotReceived:
		return AdmissionBlocked
	default:
		return AdmissionAnswerInvalid
	}
}

func (cause AdmissionCause) String() string {
	switch cause {
	case CauseGateNotApplicable:
		return "GATE_NOT_APPLICABLE"
	case CauseOutsideCustomsBooks:
		return "OUTSIDE_CUSTOMS_BOOKS"
	case CauseReleasedInFull:
		return "RELEASED_IN_FULL"
	case CauseCustomsScopeUnresolved:
		return "CUSTOMS_SCOPE_UNRESOLVED"
	case CauseDeclarationUnitNotFormed:
		return "DECLARATION_UNIT_NOT_FORMED"
	case CauseDutyVerificationAbsent:
		return "DUTY_VERIFICATION_ABSENT"
	case CauseDutyVerificationPending:
		return "DUTY_VERIFICATION_PENDING"
	case CauseReleasePartialOnly:
		return "RELEASE_PARTIAL_ONLY"
	case CauseReleaseConditionalOnly:
		return "RELEASE_CONDITIONAL_ONLY"
	case CauseGateReleaseDiscrepancy:
		return "GATE_RELEASE_DISCREPANCY"
	case CauseNodeZoneNotRegistered:
		return "NODE_ZONE_NOT_REGISTERED"
	case CauseGateCatalogNotRegistered:
		return "GATE_CATALOG_NOT_REGISTERED"
	case CauseDutyPaymentRuleNotRegistered:
		return "DUTY_PAYMENT_RULE_NOT_REGISTERED"
	case CauseDutyPaymentFindingBesideRule:
		return "DUTY_PAYMENT_FINDING_BESIDE_RULE"
	case CauseRestrictionInForce:
		return "RESTRICTION_IN_FORCE"
	case CauseGateUnmet:
		return "GATE_UNMET"
	case CauseGatePartiallyMet:
		return "GATE_PARTIALLY_MET"
	case CauseGateConflicting:
		return "GATE_CONFLICTING"
	case CauseReleaseNotReceived:
		return "RELEASE_NOT_RECEIVED"
	default:
		return ""
	}
}

// AdmissionBasis 是一条答复依据：种类、它落在哪个申报范围与监管程序上（对象级依据两者皆为零值），以及
// 指得回去的来源——限制标识、前置条件引用、放行结果的来源标识或未登记的节点。来源按字典序存，同一份依据
// 只有一种写法，判断标识才可重算。
type AdmissionBasis struct {
	Cause     AdmissionCause
	Scope     DecisionScopeReference
	Procedure CustomsProcedureReference
	Sources   []string
}

func newBasis(cause AdmissionCause, scope DecisionScopeReference, procedure CustomsProcedureReference, sources ...string) AdmissionBasis {
	sorted := append([]string(nil), sources...)
	sort.Strings(sorted)
	return AdmissionBasis{Cause: cause, Scope: scope, Procedure: procedure, Sources: sorted}
}

// UnitScope 是对象当前所在的一个申报单元：单元即门禁与放行所说的「明确申报范围」，程序取单元自己的。
type UnitScope struct {
	Scope     DecisionScopeReference
	Procedure CustomsProcedureReference
}

// ObjectCustomsScope 是本上下文按对象引用从自己的册子里解析出的关务范围（ADR-0206 决定四）：对象当前所在
// 的申报单元（被替代的不算），以及对象已在案、却还不在该案任何当前单元里的那些案件的程序——后者说明对象
// 受关务管、只是范围还没成形。两格皆空即对象不在本上下文任何案件与单元里。
type ObjectCustomsScope struct {
	Units            []UnitScope
	CasesWithoutUnit []CustomsProcedureReference
}

func (scope ObjectCustomsScope) inBooks() bool {
	return len(scope.Units) > 0 || len(scope.CasesWithoutUnit) > 0
}

// GateReadingState 是某申报范围在某程序下对某动作的门禁读法。除「已形成」外的各格都是门禁编排自己的诚实
// 停点（verify_release_gate.go 的 VerifyGateUndecidedReason 里非依赖故障的那几格），依赖读不动不在此列——
// 那时判断根本没形成。
type GateReadingState uint8

const (
	GateReadingStateInvalid GateReadingState = iota
	GateReadingFormed
	GateReadingCatalogNotRegistered
	GateReadingDutyRuleNotRegistered
	GateReadingDutyFindingBesideRule
	GateReadingDutyVerificationAbsent
	GateReadingDutyVerificationPending
)

func (state GateReadingState) valid() bool {
	return state >= GateReadingFormed && state <= GateReadingDutyVerificationPending
}

// GateReading 是一次门禁读数。Conclusion 与 Preconditions 只在「已形成」上有意义。
type GateReading struct {
	Action        GuardedAction
	Scope         DecisionScopeReference
	Procedure     CustomsProcedureReference
	State         GateReadingState
	Conclusion    GateConclusion
	Preconditions []PreconditionReference
}

// ReleaseReading 是一份已接收、未记同层冲突的放行事实及其业务发生或适用时间。同层冲突的那份不进来：CC 对
// 冲突双方都留存、当前判断不因后到的一方改变（ADR-0197 决定五同一理由）。
type ReleaseReading struct {
	Source     string
	Release    CustomsReleaseOutcome
	OccurredAt time.Time
}

// ActionAdmissionFinding 是一个适用动作的答复与支撑它的依据（只列出与答复同格的那些）。
type ActionAdmissionFinding struct {
	Action GuardedAction
	Answer AdmissionAnswer
	Bases  []AdmissionBasis
}

// ActionAdmissionInput 是一次准入判断的全部输入。限制、门禁与放行读数由编排按对象解析出的范围盘好送进来；
// 本折叠不读任何册子。
type ActionAdmissionInput struct {
	Object       DeclaredParcelReference
	Intent       ActionIntent
	At           time.Time
	Zones        []NodeZoneReading
	Scope        ObjectCustomsScope
	Restrictions []RegulatoryRestriction
	Gates        []GateReading
	Releases     []ReleaseReading
	JudgedAt     time.Time
}

// ActionAdmissionJudgement 是一次动作准入判断。类型上没有「放行结果」——准入答放行只说这一步此刻可走，
// 放行结果仍只由外部事实接收（CONTEXT「门禁满足不生成放行」）。
type ActionAdmissionJudgement struct {
	object   DeclaredParcelReference
	intent   ActionIntent
	at       time.Time
	zones    []NodeZoneReading
	cross    CrossZoneState
	findings []ActionAdmissionFinding
	answer   AdmissionAnswer
	judgedAt time.Time
}

func (judgement ActionAdmissionJudgement) Object() DeclaredParcelReference { return judgement.object }
func (judgement ActionAdmissionJudgement) Intent() ActionIntent            { return judgement.intent }
func (judgement ActionAdmissionJudgement) At() time.Time                   { return judgement.at }
func (judgement ActionAdmissionJudgement) CrossZone() CrossZoneState       { return judgement.cross }
func (judgement ActionAdmissionJudgement) Answer() AdmissionAnswer         { return judgement.answer }
func (judgement ActionAdmissionJudgement) JudgedAt() time.Time             { return judgement.judgedAt }

func (judgement ActionAdmissionJudgement) Zones() []NodeZoneReading {
	return append([]NodeZoneReading(nil), judgement.zones...)
}

func (judgement ActionAdmissionJudgement) Findings() []ActionAdmissionFinding {
	findings := make([]ActionAdmissionFinding, 0, len(judgement.findings))
	for _, finding := range judgement.findings {
		bases := make([]AdmissionBasis, 0, len(finding.Bases))
		for _, basis := range finding.Bases {
			basis.Sources = append([]string(nil), basis.Sources...)
			bases = append(bases, basis)
		}
		finding.Bases = bases
		findings = append(findings, finding)
	}
	return findings
}

// AsRecordedAt 交回同一判断、判断时刻换成册上首次记下的那一刻。重放命中已有记录时用：内容由判断标识钉住，
// 册上那份与此刻重算的这份只差判断时刻。
func (judgement ActionAdmissionJudgement) AsRecordedAt(judgedAt time.Time) ActionAdmissionJudgement {
	recorded := judgement
	recorded.judgedAt = judgedAt.UTC()
	return recorded
}

// JudgeActionAdmission 逐适用动作折三源，再把各动作压成整次答复（压制次序见 AdmissionAnswer）。
//
// 每个动作的折法：
//   - 限制：对象自身与各当前单元上、动作时点有效且约束本动作的限制一律阻断——外部放行不解内部限制。
//   - 范围：对象不在任何案件与单元里时，跨区移动答未决（跨关务边界必须有案可依，等立案），同区动作答不受
//     门禁（本上下文没有任何东西约束它）；已在案却未成单元的程序答未决。
//   - 门禁 × 放行，逐单元：门禁未形成按停点归未配置或未决；不适用即不受门禁；满足时看动作时点前生效的放行
//     ——全部放行才放行，只有部分或附条件放行答未决（说不出本件放没放、条件成没成），一份都没有答阻断；
//     未满足、部分满足、冲突答阻断，但已有全部放行时答未决——内部判断与外部决定不一致要处置，不能用内部
//     结果覆盖外部事实，也不能当作没看见。
//
// 跨区待定（起讫有一端未登记区域）时，只有对象确实带着跨区的门——某单元的跨区门禁目录登了且这一道不是
// 「不受门禁」，或有限制约束跨区移动——才答未配置并指名未登记的节点；否则这一道不列，免得没有跨境业务的
// 租户为了不被挡而去登区域（ADR-0206 决定二）。
func JudgeActionAdmission(input ActionAdmissionInput) (ActionAdmissionJudgement, error) {
	if !input.Object.valid() || input.At.IsZero() || input.JudgedAt.IsZero() {
		return ActionAdmissionJudgement{}, ErrInvalidAdmissionInput
	}
	applicability, err := DetermineGuardedActions(input.Intent, input.Zones)
	if err != nil {
		return ActionAdmissionJudgement{}, err
	}
	if err := input.validateSources(); err != nil {
		return ActionAdmissionJudgement{}, err
	}

	findings := make([]ActionAdmissionFinding, 0, len(applicability.guarded)+1)
	for _, action := range applicability.guarded {
		finding, err := input.judgeAction(action)
		if err != nil {
			return ActionAdmissionJudgement{}, err
		}
		findings = append(findings, finding)
	}
	if applicability.crossZone == CrossZoneUndetermined {
		matters, err := input.crossZoneMatters()
		if err != nil {
			return ActionAdmissionJudgement{}, err
		}
		if matters {
			sources := make([]string, 0, len(applicability.unregistered))
			for _, node := range applicability.unregistered {
				sources = append(sources, node.String())
			}
			findings = append(findings, ActionAdmissionFinding{
				Action: CrossCustomsMovement,
				Answer: AdmissionNotConfigured,
				Bases: []AdmissionBasis{newBasis(CauseNodeZoneNotRegistered,
					DecisionScopeReference{}, CustomsProcedureReference{}, sources...)},
			})
		}
	}

	answer := AdmissionAnswerInvalid
	for _, finding := range findings {
		if finding.Answer > answer {
			answer = finding.Answer
		}
	}
	if !answer.valid() {
		return ActionAdmissionJudgement{}, ErrInvalidAdmissionInput
	}

	zones := make([]NodeZoneReading, 0, len(input.Zones))
	for _, node := range input.Intent.ZoneNodes() {
		for _, reading := range input.Zones {
			if reading.Node == node {
				zones = append(zones, reading)
				break
			}
		}
	}
	return ActionAdmissionJudgement{
		object:   input.Object,
		intent:   input.Intent,
		at:       input.At.UTC(),
		zones:    zones,
		cross:    applicability.crossZone,
		findings: findings,
		answer:   answer,
		judgedAt: input.JudgedAt.UTC(),
	}, nil
}

func (input ActionAdmissionInput) validateSources() error {
	for _, unit := range input.Scope.Units {
		if !unit.Scope.valid() || !unit.Procedure.valid() {
			return ErrInvalidAdmissionInput
		}
	}
	for _, procedure := range input.Scope.CasesWithoutUnit {
		if !procedure.valid() {
			return ErrInvalidAdmissionInput
		}
	}
	for _, gate := range input.Gates {
		if !gate.Action.valid() || !gate.Scope.valid() || !gate.Procedure.valid() || !gate.State.valid() {
			return ErrInvalidAdmissionInput
		}
		if gate.State == GateReadingFormed && !gate.Conclusion.valid() {
			return ErrInvalidAdmissionInput
		}
	}
	for _, release := range input.Releases {
		if release.Source == "" || !release.Release.kind.valid() || !release.Release.scope.valid() || release.OccurredAt.IsZero() {
			return ErrInvalidAdmissionInput
		}
	}
	return nil
}

// objectScope 是对象自身作为限制范围的写法：内部限制可以直接落在一个包裹上，不必经申报单元。
func (input ActionAdmissionInput) objectScope() DecisionScopeReference {
	return DecisionScopeReference{input.Object.requiredValue}
}

// restrictionBases 盘出动作时点有效、约束本动作的限制，按范围各成一条依据。
func (input ActionAdmissionInput) restrictionBases(action GuardedAction) ([]AdmissionBasis, error) {
	scopes := []DecisionScopeReference{input.objectScope()}
	for _, unit := range input.Scope.Units {
		scopes = append(scopes, unit.Scope)
	}
	seen := make(map[DecisionScopeReference]bool, len(scopes))
	var bases []AdmissionBasis
	for _, scope := range scopes {
		if seen[scope] {
			continue
		}
		seen[scope] = true
		admissibility, err := JudgeActionAdmissibilityAt(action, scope, input.Restrictions, input.At)
		if err != nil {
			return nil, err
		}
		if admissibility.Admissible() {
			continue
		}
		sources := make([]string, 0, len(admissibility.blockedBy))
		for _, id := range admissibility.blockedBy {
			sources = append(sources, id.String())
		}
		bases = append(bases, newBasis(CauseRestrictionInForce, scope, CustomsProcedureReference{}, sources...))
	}
	return bases, nil
}

func (input ActionAdmissionInput) gateFor(action GuardedAction, unit UnitScope) (GateReading, error) {
	for _, gate := range input.Gates {
		if gate.Action == action && gate.Scope == unit.Scope && gate.Procedure == unit.Procedure {
			return gate, nil
		}
	}
	return GateReading{}, fmt.Errorf("%w: %s %s %s", ErrGateReadingMissing, action, unit.Scope, unit.Procedure)
}

// unitBases 是一个单元上门禁 × 放行那一半的依据（折法见 JudgeActionAdmission 头注）。
func (input ActionAdmissionInput) unitBases(action GuardedAction, unit UnitScope) ([]AdmissionBasis, error) {
	gate, err := input.gateFor(action, unit)
	if err != nil {
		return nil, err
	}
	on := func(cause AdmissionCause, sources ...string) []AdmissionBasis {
		return []AdmissionBasis{newBasis(cause, unit.Scope, unit.Procedure, sources...)}
	}
	switch gate.State {
	case GateReadingCatalogNotRegistered:
		return on(CauseGateCatalogNotRegistered), nil
	case GateReadingDutyRuleNotRegistered:
		return on(CauseDutyPaymentRuleNotRegistered), nil
	case GateReadingDutyFindingBesideRule:
		return on(CauseDutyPaymentFindingBesideRule), nil
	case GateReadingDutyVerificationAbsent:
		return on(CauseDutyVerificationAbsent), nil
	case GateReadingDutyVerificationPending:
		return on(CauseDutyVerificationPending), nil
	}
	if gate.Conclusion == GateNotApplicable {
		return on(CauseGateNotApplicable), nil
	}

	var full, partial, conditional []string
	for _, release := range input.Releases {
		if release.Release.scope != unit.Scope || release.OccurredAt.After(input.At) {
			continue
		}
		switch release.Release.kind {
		case FullRelease:
			full = append(full, release.Source)
		case PartialRelease:
			partial = append(partial, release.Source)
		case ConditionalRelease:
			conditional = append(conditional, release.Source)
		}
	}
	preconditions := make([]string, 0, len(gate.Preconditions))
	for _, precondition := range gate.Preconditions {
		preconditions = append(preconditions, precondition.String())
	}

	if gate.Conclusion == GateMet {
		switch {
		case len(full) > 0:
			return on(CauseReleasedInFull, full...), nil
		case len(partial) > 0 || len(conditional) > 0:
			var bases []AdmissionBasis
			if len(partial) > 0 {
				bases = append(bases, on(CauseReleasePartialOnly, partial...)...)
			}
			if len(conditional) > 0 {
				bases = append(bases, on(CauseReleaseConditionalOnly, conditional...)...)
			}
			return bases, nil
		default:
			return on(CauseReleaseNotReceived, preconditions...), nil
		}
	}
	if len(full) > 0 {
		return on(CauseGateReleaseDiscrepancy, append(preconditions, full...)...), nil
	}
	switch gate.Conclusion {
	case GateUnmet:
		return on(CauseGateUnmet, preconditions...), nil
	case GatePartiallyMet:
		return on(CauseGatePartiallyMet, preconditions...), nil
	case GateConflicting:
		return on(CauseGateConflicting, preconditions...), nil
	default:
		return nil, ErrInvalidAdmissionInput
	}
}

// allBases 是一个动作的全部依据（未按答复筛选）。
func (input ActionAdmissionInput) allBases(action GuardedAction) ([]AdmissionBasis, error) {
	bases, err := input.restrictionBases(action)
	if err != nil {
		return nil, err
	}
	if !input.Scope.inBooks() {
		if action == CrossCustomsMovement {
			bases = append(bases, newBasis(CauseCustomsScopeUnresolved, DecisionScopeReference{}, CustomsProcedureReference{}))
		} else {
			bases = append(bases, newBasis(CauseOutsideCustomsBooks, DecisionScopeReference{}, CustomsProcedureReference{}))
		}
	}
	for _, procedure := range input.Scope.CasesWithoutUnit {
		bases = append(bases, newBasis(CauseDeclarationUnitNotFormed, DecisionScopeReference{}, procedure))
	}
	for _, unit := range input.Scope.Units {
		unitBases, err := input.unitBases(action, unit)
		if err != nil {
			return nil, err
		}
		bases = append(bases, unitBases...)
	}
	return bases, nil
}

func (input ActionAdmissionInput) judgeAction(action GuardedAction) (ActionAdmissionFinding, error) {
	bases, err := input.allBases(action)
	if err != nil {
		return ActionAdmissionFinding{}, err
	}
	return foldFinding(action, bases)
}

// foldFinding 取依据里最重的那一格作答复，只留同格依据——答复说阻断时，列着的就是挡住它的东西。
func foldFinding(action GuardedAction, bases []AdmissionBasis) (ActionAdmissionFinding, error) {
	answer := AdmissionAnswerInvalid
	for _, basis := range bases {
		if class := basis.Cause.Answer(); class > answer {
			answer = class
		}
	}
	if !answer.valid() {
		return ActionAdmissionFinding{}, ErrInvalidAdmissionInput
	}
	kept := make([]AdmissionBasis, 0, len(bases))
	for _, basis := range bases {
		if basis.Cause.Answer() == answer {
			kept = append(kept, basis)
		}
	}
	return ActionAdmissionFinding{Action: action, Answer: answer, Bases: kept}, nil
}

// crossZoneMatters 判跨区待定时这一道是否落在对象身上（判据见 JudgeActionAdmission 头注末段）。门禁目录
// 没登跨区那一道的单元不算——「有跨区目录行」是这一判据的前提，没登就说明这个范围本没有跨区的门。
func (input ActionAdmissionInput) crossZoneMatters() (bool, error) {
	restricted, err := input.restrictionBases(CrossCustomsMovement)
	if err != nil {
		return false, err
	}
	if len(restricted) > 0 {
		return true, nil
	}
	for _, unit := range input.Scope.Units {
		gate, err := input.gateFor(CrossCustomsMovement, unit)
		if err != nil {
			return false, err
		}
		if gate.State == GateReadingCatalogNotRegistered {
			continue
		}
		bases, err := input.unitBases(CrossCustomsMovement, unit)
		if err != nil {
			return false, err
		}
		for _, basis := range bases {
			if basis.Cause.Answer() != AdmissionNotGuarded {
				return true, nil
			}
		}
	}
	return false, nil
}
