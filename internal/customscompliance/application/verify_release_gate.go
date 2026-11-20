package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ErrUnexpectedGateSave 说明门禁库交回了封闭集合以外的写入结果。
var ErrUnexpectedGateSave = errors.New("customs compliance: unexpected gate verification save outcome")

// VerifyGateOutcome 是放行门禁核对请求的应用处理结果。
type VerifyGateOutcome uint8

const (
	VerifyGateOutcomeInvalid VerifyGateOutcome = iota
	GateVerificationRecorded
	GateVerificationExisting
	GateVerificationNotAccepted
	GateVerificationUndecided
)

func (outcome VerifyGateOutcome) String() string {
	switch outcome {
	case GateVerificationRecorded:
		return "RECORDED"
	case GateVerificationExisting:
		return "EXISTING_RESULT"
	case GateVerificationNotAccepted:
		return "NOT_ACCEPTED"
	case GateVerificationUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// VerifyGateUndecidedReason 指名门禁核对停在哪一步等谁。「税费付款」那一道有三个各自的诚实停点
// （票 sa-cc/06，ADR-0137 决定三）——规则未配置 / 没有付款核对 / 三态待确认或冲突——它们都不是
// 「未满足」：未满足是规则判出来的结论，停点是还判不了。其余几格是目录未登记与依赖故障。
type VerifyGateUndecidedReason uint8

const (
	VerifyGateUndecidedReasonNone VerifyGateUndecidedReason = iota
	GateCatalogNotConfigured
	GateConditionViewUnavailable
	DutyPaymentGateRuleNotConfigured
	DutyPaymentGateRuleViewUnavailable
	DutyPaymentFindingRegisteredBesideRule
	DutyVerificationAbsent
	DutyVerificationViewUnavailable
	DutyVerificationPending
	WeightDiscrepancyGateRuleNotConfigured
	WeightDiscrepancyGateRuleViewUnavailable
	WeightDiscrepancyFindingRegisteredBesideRule
	DeclarationSubmissionAbsent
	DeclarationSubmissionViewUnavailable
	GateMemberMeasurementAbsent
	GateMemberMeasurementUndetermined
	GateMemberMeasurementViewUnavailable
	GateStoreUnavailable
)

func (reason VerifyGateUndecidedReason) String() string {
	switch reason {
	case GateCatalogNotConfigured:
		return "GATE_CATALOG_NOT_CONFIGURED"
	case GateConditionViewUnavailable:
		return "GATE_CONDITION_VIEW_UNAVAILABLE"
	case DutyPaymentGateRuleNotConfigured:
		return "DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED"
	case DutyPaymentGateRuleViewUnavailable:
		return "DUTY_PAYMENT_GATE_RULE_VIEW_UNAVAILABLE"
	case DutyPaymentFindingRegisteredBesideRule:
		return "DUTY_PAYMENT_FINDING_REGISTERED_BESIDE_RULE"
	case DutyVerificationAbsent:
		return "DUTY_VERIFICATION_ABSENT"
	case DutyVerificationViewUnavailable:
		return "DUTY_VERIFICATION_VIEW_UNAVAILABLE"
	case DutyVerificationPending:
		return "DUTY_VERIFICATION_PENDING"
	case WeightDiscrepancyGateRuleNotConfigured:
		return "WEIGHT_DISCREPANCY_GATE_RULE_NOT_CONFIGURED"
	case WeightDiscrepancyGateRuleViewUnavailable:
		return "WEIGHT_DISCREPANCY_GATE_RULE_VIEW_UNAVAILABLE"
	case WeightDiscrepancyFindingRegisteredBesideRule:
		return "WEIGHT_DISCREPANCY_FINDING_REGISTERED_BESIDE_RULE"
	case DeclarationSubmissionAbsent:
		return "DECLARATION_SUBMISSION_ABSENT"
	case DeclarationSubmissionViewUnavailable:
		return "DECLARATION_SUBMISSION_VIEW_UNAVAILABLE"
	case GateMemberMeasurementAbsent:
		return "MEMBER_MEASUREMENT_ABSENT"
	case GateMemberMeasurementUndetermined:
		return "MEMBER_MEASUREMENT_UNDETERMINED"
	case GateMemberMeasurementViewUnavailable:
		return "MEMBER_MEASUREMENT_VIEW_UNAVAILABLE"
	case GateStoreUnavailable:
		return "GATE_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// VerifyReleaseGateCommand 携带一次门禁核对请求：申报范围、拟执行动作与适用监管边界
// ——三者共同构成判断身份（CONTEXT「门禁满足不生成放行，也不能复用于其他动作或监管边界」）。
type VerifyReleaseGateCommand struct {
	TenantID domain.TenantID
	Scope    domain.DecisionScopeReference
	Action   domain.GuardedAction
	Boundary domain.CustomsProcedureReference
}

type VerifyReleaseGateResult struct {
	outcome    VerifyGateOutcome
	reason     VerifyGateUndecidedReason
	gate       domain.ReleaseGateVerification
	hasRecord  bool
	handoffRef string
}

func (result VerifyReleaseGateResult) Outcome() VerifyGateOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result VerifyReleaseGateResult) UndecidedReason() VerifyGateUndecidedReason {
	return result.reason
}

func (result VerifyReleaseGateResult) Gate() (domain.ReleaseGateVerification, bool) {
	return result.gate, result.hasRecord
}

// HandoffReference 非空说明核对已入册但意图还没交出去，重放会重发同一份。
func (result VerifyReleaseGateResult) HandoffReference() string {
	return result.handoffRef
}

func gateUndecided(reason VerifyGateUndecidedReason) VerifyReleaseGateResult {
	return VerifyReleaseGateResult{outcome: GateVerificationUndecided, reason: reason}
}

// VerifyReleaseGateDeps 是门禁核对要问的各口。税费付款与毛重差异各自的规则、以及毛重差异要读的提交版本和
// 成员实测，都必备——只走「不构成前置条件」那一形的装配点也得接上，构造门对每一口一视同仁。
type VerifyReleaseGateDeps struct {
	Conditions        ports.GateConditionView
	DutyRules         ports.DutyPaymentGateRuleView
	DutyVerifications ports.CurrentDutyVerificationView
	WeightRules       ports.WeightDiscrepancyGateRuleView
	Submissions       ports.CurrentDeclarationSubmissionView
	MemberWeights     ports.MemberMeasuredWeightView
	Store             ports.GateVerificationStore
	Downstream        ports.GateVerificationHandoff
	Clock             ports.Clock
}

type VerifyReleaseGateHandler struct {
	deps VerifyReleaseGateDeps
}

// NewVerifyReleaseGateHandler 构造门逐口拒 nil（形照 NewDutyPaymentReconciliationHandler；沿用本包的
// ErrNilDependency 作唯一答复，哪一口缺在包装信息里点名）。
func NewVerifyReleaseGateHandler(deps VerifyReleaseGateDeps) (*VerifyReleaseGateHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"gate condition view", deps.Conditions == nil},
		{"duty payment gate rule view", deps.DutyRules == nil},
		{"current duty verification view", deps.DutyVerifications == nil},
		{"weight discrepancy gate rule view", deps.WeightRules == nil},
		{"current declaration submission view", deps.Submissions == nil},
		{"member measured weight view", deps.MemberWeights == nil},
		{"gate verification store", deps.Store == nil},
		{"gate verification handoff", deps.Downstream == nil},
		{"clock", deps.Clock == nil},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("%w: release gate %s", ErrNilDependency, dependency.name)
		}
	}
	return &VerifyReleaseGateHandler{deps: deps}, nil
}

// Handle 把一次门禁核对推进到版本化判断：读门禁（assessReleaseGate，各停点答未决）→ 幂等按
// （范围+动作+边界+内容指纹）——条件状态或核对版本变化自然换指纹换版 → 提交与意图。门禁记录带
// 三态原值与核对版本引用，不带合成布尔；类型上没有放行字段：门禁满足不生成放行，放行结果仍由
// 外部事实接收。
func (handler *VerifyReleaseGateHandler) Handle(
	ctx context.Context,
	command VerifyReleaseGateCommand,
) (VerifyReleaseGateResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" ||
		strings.TrimSpace(command.Scope.String()) == "" ||
		strings.TrimSpace(command.Boundary.String()) == "" {
		return VerifyReleaseGateResult{outcome: GateVerificationNotAccepted}, nil
	}

	assessment, err := assessReleaseGate(ctx, releaseGateSources{
		conditions:        handler.deps.Conditions,
		dutyRules:         handler.deps.DutyRules,
		dutyVerifications: handler.deps.DutyVerifications,
		weightRules:       handler.deps.WeightRules,
		submissions:       handler.deps.Submissions,
		memberWeights:     handler.deps.MemberWeights,
	}, command.TenantID, command.Scope, command.Action, command.Boundary)
	if err != nil {
		return VerifyReleaseGateResult{}, err
	}
	if assessment.reason != VerifyGateUndecidedReasonNone {
		return gateUndecided(assessment.reason), nil
	}
	if assessment.foldRefused {
		// 请求没错，是册上读回的认定不合法——「未受理」指请求不能合法关联、续办是改请求，
		// 改请求换不来结果。与动作准入同一事实同一答法：读不动并指名门禁目录那一口，
		// 与目录读到坏行同归 GATE_CONDITION_VIEW_UNAVAILABLE；不另立原因码，恢复动作同样
		// 是运维去救登记（ADR-0029 按恢复动作分格）。
		return gateUndecided(GateConditionViewUnavailable), nil
	}
	findings := assessment.findings
	dutyReading, dutyApplied := assessment.dutyReading, assessment.dutyApplied
	weightReading, weightApplied := assessment.weightReading, assessment.weightApplied

	gate, err := domain.VerifyReleaseGate(
		command.Scope, command.Action, command.Boundary,
		assessment.preconditions(), assessment.conclusion, handler.deps.Clock.Now())
	if err != nil {
		return VerifyReleaseGateResult{outcome: GateVerificationNotAccepted}, nil
	}
	if dutyApplied {
		if gate, err = gate.WithDutyPayment(dutyReading); err != nil {
			return VerifyReleaseGateResult{}, err
		}
	}
	if weightApplied {
		if gate, err = gate.WithWeightDiscrepancy(weightReading); err != nil {
			return VerifyReleaseGateResult{}, err
		}
	}

	key := ports.GateVerificationKey{
		TenantID: command.TenantID,
		Scope:    command.Scope,
		Action:   command.Action,
		Boundary: command.Boundary,
		Digest:   ports.GateVersionDigest(findings, dutyReading, dutyApplied, weightReading, weightApplied),
	}
	saved, err := handler.deps.Store.Save(ctx, key, gate)
	if err != nil {
		return gateUndecided(GateStoreUnavailable), nil
	}
	switch saved {
	case ports.GateVerificationSaved:
		result := VerifyReleaseGateResult{
			outcome:   GateVerificationRecorded,
			gate:      gate,
			hasRecord: true,
		}
		result.handoffRef = handler.handOffGate(ctx, key, gate)
		return result, nil
	case ports.GateVerificationAlreadyRecorded:
		winner, found, err := handler.deps.Store.FindByKey(ctx, key)
		if err != nil || !found {
			return gateUndecided(GateStoreUnavailable), nil
		}
		result := VerifyReleaseGateResult{
			outcome:   GateVerificationExisting,
			gate:      winner,
			hasRecord: true,
		}
		result.handoffRef = handler.handOffGate(ctx, key, winner)
		return result, nil
	default:
		return VerifyReleaseGateResult{}, fmt.Errorf("%w: %d", ErrUnexpectedGateSave, saved)
	}
}

// handOffGate 交发布意图。失败不翻核对，留续办引用重发同一份。
func (handler *VerifyReleaseGateHandler) handOffGate(
	ctx context.Context,
	key ports.GateVerificationKey,
	gate domain.ReleaseGateVerification,
) string {
	if err := handler.deps.Downstream.HandOffGate(ctx, ports.GateVerificationHandoffIntent{
		Key:  key,
		Gate: gate,
	}); err == nil {
		return ""
	}
	return "CONT-GATE/" + key.Scope.String() + "/" + key.Action.String()
}

// releaseGateSources 是一道门禁读数要问的各口。放行门禁核对与动作准入判断（ADR-0206）各自把自己的口交进来，
// 读法只在 assessReleaseGate 一处定义——两条编排各写一遍，就是同一道门两种判法。
type releaseGateSources struct {
	conditions        ports.GateConditionView
	dutyRules         ports.DutyPaymentGateRuleView
	dutyVerifications ports.CurrentDutyVerificationView
	weightRules       ports.WeightDiscrepancyGateRuleView
	submissions       ports.CurrentDeclarationSubmissionView
	memberWeights     ports.MemberMeasuredWeightView
}

// releaseGateAssessment 是一道门禁读到哪一步、读出了什么。reason 非零即停在那个停点（含依赖读不动）；
// foldRefused 说认定清单折不出结论（读回的认定不合法）。它不并进 reason：两条编排各按自己的答复格归它，
// 门禁读法不替它们选格；两条编排都把它归到门禁目录读不动那一格（核对答未决、准入答读不动）。
type releaseGateAssessment struct {
	reason        VerifyGateUndecidedReason
	foldRefused   bool
	findings      []domain.PreconditionFinding
	conclusion    domain.GateConclusion
	dutyReading   domain.DutyPaymentGateReading
	dutyApplied   bool
	weightReading domain.WeightDiscrepancyGateReading
	weightApplied bool
}

func (assessment releaseGateAssessment) preconditions() []domain.PreconditionReference {
	preconditions := make([]domain.PreconditionReference, 0, len(assessment.findings))
	for _, finding := range assessment.findings {
		preconditions = append(preconditions, finding.Precondition)
	}
	return preconditions
}

// assessReleaseGate 读一道门禁：盘前置条件逐项判断（目录未登记即停——没有清单的门禁判断无从复核；空清单是
// 「此动作在此边界不受门禁」的如实答案）→ 取「税费付款」那一道的登记规则（没有即「规则未配置」停点，不取任何
// 默认折法）→ 规则要读核对时取当前那一版、按接受集合折出这一道（无核对 / 三态待确认或冲突各自停点）→ 领域
// 折叠（冲突压过满足与未满足）。error 只留给编程错误。
func assessReleaseGate(
	ctx context.Context,
	sources releaseGateSources,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
	action domain.GuardedAction,
	boundary domain.CustomsProcedureReference,
) (releaseGateAssessment, error) {
	stop := func(reason VerifyGateUndecidedReason) (releaseGateAssessment, error) {
		return releaseGateAssessment{reason: reason}, nil
	}
	findings, configured, err := sources.conditions.LoadPreconditionFindings(ctx, tenant, scope, action, boundary)
	if err != nil {
		return stop(GateConditionViewUnavailable)
	}
	if !configured {
		return stop(GateCatalogNotConfigured)
	}

	rule, found, err := sources.dutyRules.LoadDutyPaymentGateRule(ctx, tenant, scope, action, boundary)
	if err != nil {
		return stop(DutyPaymentGateRuleViewUnavailable)
	}
	if !found {
		return stop(DutyPaymentGateRuleNotConfigured)
	}
	for _, finding := range findings {
		if finding.Precondition == domain.DutyPaymentPrecondition {
			// 这一道登规则不登认定（ADR-0137 决定三）；册上两样都有，说的是两件事，等登记方收掉
			// 认定那一行——不替它选一边。
			return stop(DutyPaymentFindingRegisteredBesideRule)
		}
	}

	weightRule, found, err := sources.weightRules.LoadWeightDiscrepancyGateRule(ctx, tenant, scope, action, boundary)
	if err != nil {
		return stop(WeightDiscrepancyGateRuleViewUnavailable)
	}
	if !found {
		// 没登规则不是「零容差」，也不是「这一道放过」。容差是租户取值，缺了就停。
		return stop(WeightDiscrepancyGateRuleNotConfigured)
	}
	for _, finding := range findings {
		if finding.Precondition == domain.WeightDiscrepancyPrecondition {
			return stop(WeightDiscrepancyFindingRegisteredBesideRule)
		}
	}

	var reading domain.DutyPaymentGateReading
	applied := false
	if !rule.NotAPrecondition() {
		record, found, err := sources.dutyVerifications.LoadCurrentDutyVerification(ctx, tenant, scope)
		if err != nil {
			return stop(DutyVerificationViewUnavailable)
		}
		if !found {
			return stop(DutyVerificationAbsent)
		}
		reading, err = rule.Judge(record.Verification)
		switch {
		case errors.Is(err, domain.ErrDutyPaymentGateUndecided):
			return stop(DutyVerificationPending)
		case err != nil:
			// 规则与核对都经领域构造把过门，走到这里是编程错误，不折成任何业务格。
			return releaseGateAssessment{}, err
		}
		reading.Verification.Version = record.Key.Digest
		findings = append(findings, domain.PreconditionFinding{
			Precondition: domain.DutyPaymentPrecondition,
			State:        reading.State,
		})
		applied = true
	}

	var weightReading domain.WeightDiscrepancyGateReading
	weightApplied := false
	if !weightRule.NotAPrecondition() {
		judged, reason, err := judgeWeightDiscrepancy(ctx, sources, tenant, scope, boundary, weightRule)
		if err != nil {
			return releaseGateAssessment{}, err
		}
		if reason != VerifyGateUndecidedReasonNone {
			return stop(reason)
		}
		weightReading = judged
		findings = append(findings, domain.PreconditionFinding{
			Precondition: domain.WeightDiscrepancyPrecondition,
			State:        weightReading.State,
		})
		weightApplied = true
	}

	conclusion, err := domain.FoldGateConclusion(findings)
	if err != nil {
		return releaseGateAssessment{foldRefused: true}, nil
	}
	return releaseGateAssessment{
		findings:      findings,
		conclusion:    conclusion,
		dutyReading:   reading,
		dutyApplied:   applied,
		weightReading: weightReading,
		weightApplied: weightApplied,
	}, nil
}

// judgeWeightDiscrepancy 在规则要读这一道时，取当前提交版本与各成员此刻的实测，按容差折出读数。
// 提交版本的范围引用与门禁同一个字符串：申报单元身份就是门禁说的明确申报范围。
func judgeWeightDiscrepancy(
	ctx context.Context,
	sources releaseGateSources,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
	boundary domain.CustomsProcedureReference,
	rule domain.WeightDiscrepancyGateRule,
) (domain.WeightDiscrepancyGateReading, VerifyGateUndecidedReason, error) {
	none := domain.WeightDiscrepancyGateReading{}
	unitID, err := domain.NewDeclarationUnitID(scope.String())
	if err != nil {
		return none, DeclarationSubmissionAbsent, nil
	}
	submission, found, err := sources.submissions.FindByKey(ctx, ports.DeclarationSubmissionKey{
		TenantID:  tenant,
		Unit:      unitID,
		Procedure: boundary,
	})
	if err != nil {
		return none, DeclarationSubmissionViewUnavailable, nil
	}
	if !found {
		return none, DeclarationSubmissionAbsent, nil
	}
	members := submission.Version.Members()
	sort.Slice(members, func(i, j int) bool {
		return members[i].String() < members[j].String()
	})
	actuals := make(map[domain.DeclaredParcelReference]domain.WeightDiscrepancyMemberActual, len(members))
	for _, member := range members {
		reading, err := sources.memberWeights.LoadMemberMeasuredWeight(ctx, tenant, member)
		if err != nil {
			return none, GateMemberMeasurementViewUnavailable, nil
		}
		switch reading.Answer {
		case ports.MeasuredWeightUndetermined:
			return none, GateMemberMeasurementUndetermined, nil
		case ports.MeasuredWeightNone:
			return none, GateMemberMeasurementAbsent, nil
		case ports.MeasuredWeightOne:
			actuals[member] = domain.WeightDiscrepancyMemberActual{
				GrossWeight:       reading.GrossWeight,
				GrossWeightUnit:   reading.GrossWeightUnit,
				MeasurementSource: reading.MeasurementSource,
			}
		default:
			return none, VerifyGateUndecidedReasonNone, fmt.Errorf("customs compliance: member measurement answer %s", reading.Answer)
		}
	}
	judged, err := rule.Judge(submission.Version, actuals)
	if errors.Is(err, domain.ErrWeightDiscrepancyUnitUnconvertible) {
		return none, GateMemberMeasurementUndetermined, nil
	}
	if err != nil {
		return none, VerifyGateUndecidedReasonNone, err
	}
	return judged, VerifyGateUndecidedReasonNone, nil
}
