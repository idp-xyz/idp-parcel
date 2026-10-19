package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 动作准入判断（ADR-0206）——消费方（运输履约的出发、交出，以及交付）在动作时同步问：这个对象此刻能不能做这一步。
// 请求驱动（ADR-0137 决定二）：只在请求到达时判一次并记下，来源变化不后台重算。判断记录不入队：今天没有订阅者，
// 消费方把答复随自己的事实同笔保存（ADR-0205），这里的记录只为「当时凭什么这么答」可查、可重放。
//
// 三源各自的读法不在这里另写：限制按时点的适用性在 domain.JudgeActionAdmissibilityAt，门禁读数与放行门禁核对
// 共用 assessReleaseGate，放行事实只读 UC-CC-006 收下的那些。本编排只负责按对象解析出的范围把三源盘齐、交领域折。
// 现有 VerifyReleaseGate 与它交出的 gate-verification.recorded 不因本编排改变——准入判断不登门禁核对册，也
// 不替谁发门禁信封。

// ErrUnexpectedActionAdmissionSave 说判断册交回了封闭集合以外的写入结果。
var ErrUnexpectedActionAdmissionSave = errors.New("customs compliance: unexpected action admission save outcome")

// ActionAdmissionOutcome 是一次准入请求的应用处理结果。`判断已形成`与`已有判断`都带判断；读不动说判断根本没
// 形成——它与判断里的「未决」不是一回事：未决是本上下文读齐了三源后如实答「还缺什么」，读不动是没读齐。
type ActionAdmissionOutcome uint8

const (
	ActionAdmissionOutcomeInvalid ActionAdmissionOutcome = iota
	ActionAdmissionJudged
	ActionAdmissionExisting
	ActionAdmissionNotAccepted
	ActionAdmissionUnreadable
)

func (outcome ActionAdmissionOutcome) String() string {
	switch outcome {
	case ActionAdmissionJudged:
		return "JUDGED"
	case ActionAdmissionExisting:
		return "EXISTING"
	case ActionAdmissionNotAccepted:
		return "NOT_ACCEPTED"
	case ActionAdmissionUnreadable:
		return "UNREADABLE"
	default:
		return ""
	}
}

// ActionAdmissionUnreadableReason 指名读不动的是哪一口，运维据此去救哪个依赖。
type ActionAdmissionUnreadableReason uint8

const (
	ActionAdmissionUnreadableReasonNone ActionAdmissionUnreadableReason = iota
	NodeZoneViewUnavailable
	ObjectScopeViewUnavailable
	RestrictionViewUnavailable
	AdmissionGateConditionsUnavailable
	AdmissionDutyPaymentRuleUnavailable
	AdmissionDutyVerificationsUnavailable
	ReleaseOutcomeViewUnavailable
	ActionAdmissionStoreUnavailable
)

func (reason ActionAdmissionUnreadableReason) String() string {
	switch reason {
	case NodeZoneViewUnavailable:
		return "NODE_ZONE_VIEW_UNAVAILABLE"
	case ObjectScopeViewUnavailable:
		return "OBJECT_SCOPE_VIEW_UNAVAILABLE"
	case RestrictionViewUnavailable:
		return "RESTRICTION_VIEW_UNAVAILABLE"
	case AdmissionGateConditionsUnavailable:
		return "GATE_CONDITION_VIEW_UNAVAILABLE"
	case AdmissionDutyPaymentRuleUnavailable:
		return "DUTY_PAYMENT_GATE_RULE_VIEW_UNAVAILABLE"
	case AdmissionDutyVerificationsUnavailable:
		return "DUTY_VERIFICATION_VIEW_UNAVAILABLE"
	case ReleaseOutcomeViewUnavailable:
		return "RELEASE_OUTCOME_VIEW_UNAVAILABLE"
	case ActionAdmissionStoreUnavailable:
		return "ACTION_ADMISSION_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// JudgeActionAdmissionCommand 是一次准入请求：租户、对象、动作意图与动作时点。时点是动作发生（或拟发生）的
// 业务时刻，不是请求到达时刻——离线补登的出发按它出发那一刻判。
type JudgeActionAdmissionCommand struct {
	TenantID domain.TenantID
	Object   domain.DeclaredParcelReference
	Intent   domain.ActionIntent
	At       time.Time
}

type JudgeActionAdmissionResult struct {
	outcome     ActionAdmissionOutcome
	reason      ActionAdmissionUnreadableReason
	judgement   domain.ActionAdmissionJudgement
	judgementID string
}

func (result JudgeActionAdmissionResult) Outcome() ActionAdmissionOutcome { return result.outcome }

// UnreadableReason 只在`读不动`时非零。
func (result JudgeActionAdmissionResult) UnreadableReason() ActionAdmissionUnreadableReason {
	return result.reason
}

// Judgement 只在`判断已形成`与`已有判断`上给出。
func (result JudgeActionAdmissionResult) Judgement() (domain.ActionAdmissionJudgement, string, bool) {
	formed := result.outcome == ActionAdmissionJudged || result.outcome == ActionAdmissionExisting
	return result.judgement, result.judgementID, formed
}

func admissionUnreadable(reason ActionAdmissionUnreadableReason) JudgeActionAdmissionResult {
	return JudgeActionAdmissionResult{outcome: ActionAdmissionUnreadable, reason: reason}
}

// JudgeActionAdmissionDeps 是准入编排的九口。门禁那三口与放行门禁核对同型，生产上接同一批适配器。
type JudgeActionAdmissionDeps struct {
	Zones             ports.NodeCustomsZoneView
	Scopes            ports.ObjectCustomsScopeView
	Restrictions      ports.RestrictionScopeView
	Conditions        ports.GateConditionView
	DutyRules         ports.DutyPaymentGateRuleView
	DutyVerifications ports.CurrentDutyVerificationView
	Releases          ports.ReleaseOutcomeView
	Store             ports.ActionAdmissionJudgementStore
	Clock             ports.Clock
}

type JudgeActionAdmissionHandler struct {
	deps JudgeActionAdmissionDeps
}

// NewJudgeActionAdmissionHandler 构造门逐口拒 nil（形照 NewVerifyReleaseGateHandler）：装配疏漏在进程启动时炸出来，
// 不等第一次出发。
func NewJudgeActionAdmissionHandler(deps JudgeActionAdmissionDeps) (*JudgeActionAdmissionHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"node customs zone view", deps.Zones == nil},
		{"object customs scope view", deps.Scopes == nil},
		{"restriction scope view", deps.Restrictions == nil},
		{"gate condition view", deps.Conditions == nil},
		{"duty payment gate rule view", deps.DutyRules == nil},
		{"current duty verification view", deps.DutyVerifications == nil},
		{"release outcome view", deps.Releases == nil},
		{"action admission judgement store", deps.Store == nil},
		{"clock", deps.Clock == nil},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("%w: action admission %s", ErrNilDependency, dependency.name)
		}
	}
	return &JudgeActionAdmissionHandler{deps: deps}, nil
}

// Handle 读区域 → 定适用动作 → 解析对象范围 → 盘限制、门禁、放行 → 领域折叠 → 按内容派生标识记下。任何一口读不动
// 都在折叠前停下答`读不动`：半份三源折出的放行是假放行。error 只留给编程错误。
func (handler *JudgeActionAdmissionHandler) Handle(
	ctx context.Context,
	command JudgeActionAdmissionCommand,
) (JudgeActionAdmissionResult, error) {
	if blankTenant(command.TenantID) ||
		strings.TrimSpace(command.Object.String()) == "" ||
		command.Intent.Kind().String() == "" ||
		command.At.IsZero() {
		return JudgeActionAdmissionResult{outcome: ActionAdmissionNotAccepted}, nil
	}

	zones := make([]domain.NodeZoneReading, 0, 2)
	for _, node := range command.Intent.ZoneNodes() {
		entry, found, err := handler.deps.Zones.LoadNodeCustomsZone(ctx, command.TenantID, node, command.At)
		if err != nil {
			return admissionUnreadable(NodeZoneViewUnavailable), nil
		}
		reading := domain.NodeZoneReading{Node: node}
		if found {
			reading.Zone, reading.Registered = entry.Zone, true
		}
		zones = append(zones, reading)
	}
	applicability, err := domain.DetermineGuardedActions(command.Intent, zones)
	if err != nil {
		return JudgeActionAdmissionResult{}, err
	}

	scope, err := handler.deps.Scopes.LoadObjectCustomsScope(ctx, command.TenantID, command.Object)
	if err != nil {
		return admissionUnreadable(ObjectScopeViewUnavailable), nil
	}

	restrictionScopes := []domain.DecisionScopeReference{}
	objectScope, err := domain.NewDecisionScopeReference(command.Object.String())
	if err != nil {
		return JudgeActionAdmissionResult{}, err
	}
	restrictionScopes = append(restrictionScopes, objectScope)
	for _, unit := range scope.Units {
		restrictionScopes = append(restrictionScopes, unit.Scope)
	}
	var restrictions []domain.RegulatoryRestriction
	for _, decisionScope := range distinctScopes(restrictionScopes) {
		listed, err := handler.deps.Restrictions.ListByScope(ctx, command.TenantID, decisionScope)
		if err != nil {
			return admissionUnreadable(RestrictionViewUnavailable), nil
		}
		restrictions = append(restrictions, listed...)
	}

	gated := applicability.Guarded()
	if applicability.CrossZone() == domain.CrossZoneUndetermined {
		// 跨区待定时也要读跨区那一道：对象带不带跨区的门，决定这一道答未配置还是不列（领域判）。
		gated = append(gated, domain.CrossCustomsMovement)
	}
	sources := releaseGateSources{
		conditions:        handler.deps.Conditions,
		dutyRules:         handler.deps.DutyRules,
		dutyVerifications: handler.deps.DutyVerifications,
	}
	var gates []domain.GateReading
	for _, action := range gated {
		for _, unit := range scope.Units {
			reading, unreadable, err := readGate(ctx, sources, command.TenantID, action, unit)
			if err != nil {
				return JudgeActionAdmissionResult{}, err
			}
			if unreadable != ActionAdmissionUnreadableReasonNone {
				return admissionUnreadable(unreadable), nil
			}
			gates = append(gates, reading)
		}
	}

	var releases []domain.ReleaseReading
	unitScopes := make([]domain.DecisionScopeReference, 0, len(scope.Units))
	for _, unit := range scope.Units {
		unitScopes = append(unitScopes, unit.Scope)
	}
	for _, decisionScope := range distinctScopes(unitScopes) {
		listed, err := handler.deps.Releases.ListReleaseOutcomes(ctx, command.TenantID, decisionScope)
		if err != nil {
			return admissionUnreadable(ReleaseOutcomeViewUnavailable), nil
		}
		releases = append(releases, listed...)
	}

	judgement, err := domain.JudgeActionAdmission(domain.ActionAdmissionInput{
		Object:       command.Object,
		Intent:       command.Intent,
		At:           command.At,
		Zones:        zones,
		Scope:        scope,
		Restrictions: restrictions,
		Gates:        gates,
		Releases:     releases,
		JudgedAt:     handler.deps.Clock.Now(),
	})
	if err != nil {
		return JudgeActionAdmissionResult{}, err
	}

	judgementID := ports.ActionAdmissionJudgementID(command.TenantID, judgement)
	saved, err := handler.deps.Store.SaveActionAdmissionJudgement(ctx, command.TenantID, judgementID, judgement)
	if err != nil {
		return admissionUnreadable(ActionAdmissionStoreUnavailable), nil
	}
	switch saved {
	case ports.ActionAdmissionSaved:
		return JudgeActionAdmissionResult{outcome: ActionAdmissionJudged, judgement: judgement, judgementID: judgementID}, nil
	case ports.ActionAdmissionAlreadyRecorded:
		judgedAt, found, err := handler.deps.Store.FindActionAdmissionJudgedAt(ctx, command.TenantID, judgementID)
		if err != nil || !found {
			return admissionUnreadable(ActionAdmissionStoreUnavailable), nil
		}
		return JudgeActionAdmissionResult{
			outcome:     ActionAdmissionExisting,
			judgement:   judgement.AsRecordedAt(judgedAt),
			judgementID: judgementID,
		}, nil
	default:
		return JudgeActionAdmissionResult{}, fmt.Errorf("%w: %d", ErrUnexpectedActionAdmissionSave, saved)
	}
}

// readGate 读一个单元在其程序下对一个动作的门禁，把门禁编排的停点译成领域读数；依赖故障译成读不动。
func readGate(
	ctx context.Context,
	sources releaseGateSources,
	tenant domain.TenantID,
	action domain.GuardedAction,
	unit domain.UnitScope,
) (domain.GateReading, ActionAdmissionUnreadableReason, error) {
	assessment, err := assessReleaseGate(ctx, sources, tenant, unit.Scope, action, unit.Procedure)
	if err != nil {
		return domain.GateReading{}, ActionAdmissionUnreadableReasonNone, err
	}
	reading := domain.GateReading{Action: action, Scope: unit.Scope, Procedure: unit.Procedure}
	switch assessment.reason {
	case VerifyGateUndecidedReasonNone:
		if assessment.foldRefused {
			// 目录读口交回的认定都经领域构造重建过，折不出结论只能是读口坏了。
			return domain.GateReading{}, ActionAdmissionUnreadableReasonNone,
				fmt.Errorf("customs compliance: gate findings for %s %s %s do not fold", unit.Scope, action, unit.Procedure)
		}
		reading.State = domain.GateReadingFormed
		reading.Conclusion = assessment.conclusion
		reading.Preconditions = assessment.preconditions()
	case GateCatalogNotConfigured:
		reading.State = domain.GateReadingCatalogNotRegistered
	case DutyPaymentGateRuleNotConfigured:
		reading.State = domain.GateReadingDutyRuleNotRegistered
	case DutyPaymentFindingRegisteredBesideRule:
		reading.State = domain.GateReadingDutyFindingBesideRule
	case DutyVerificationAbsent:
		reading.State = domain.GateReadingDutyVerificationAbsent
	case DutyVerificationPending:
		reading.State = domain.GateReadingDutyVerificationPending
	case GateConditionViewUnavailable:
		return domain.GateReading{}, AdmissionGateConditionsUnavailable, nil
	case DutyPaymentGateRuleViewUnavailable:
		return domain.GateReading{}, AdmissionDutyPaymentRuleUnavailable, nil
	case DutyVerificationViewUnavailable:
		return domain.GateReading{}, AdmissionDutyVerificationsUnavailable, nil
	default:
		return domain.GateReading{}, ActionAdmissionUnreadableReasonNone,
			fmt.Errorf("customs compliance: gate assessment stopped at %s, which the admission judgement does not read", assessment.reason)
	}
	return reading, ActionAdmissionUnreadableReasonNone, nil
}

func distinctScopes(scopes []domain.DecisionScopeReference) []domain.DecisionScopeReference {
	seen := make(map[domain.DecisionScopeReference]bool, len(scopes))
	distinct := make([]domain.DecisionScopeReference, 0, len(scopes))
	for _, scope := range scopes {
		if seen[scope] {
			continue
		}
		seen[scope] = true
		distinct = append(distinct, scope)
	}
	return distinct
}
