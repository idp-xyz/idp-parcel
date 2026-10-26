package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ErrUnexpectedDepartureGateSignalOutcome 说明信号编排交回了本用例接不住的一格（未受理或集外）。命令由已成形的领域值拼成，
// 走到未受理只能是实现坏了；上抛而不吸收成某一格。
var ErrUnexpectedDepartureGateSignalOutcome = errors.New("visibility exception: unexpected departure gate signal outcome")

// DepartureGateSignalOutcome 是一件被阻断出发对象在信号这一步落到了哪里。三格的恢复动作各不相同：已进分诊什么都不必做；
// 规则未配置等租户登记 `PAR-VIS-04`；搁置的要整封重放（规则视图调不通，或分诊那一侧本轮停在未决）。
type DepartureGateSignalOutcome uint8

const (
	DepartureGateSignalOutcomeInvalid DepartureGateSignalOutcome = iota
	DepartureGateSignalRaised
	DepartureGateSignalRuleNotConfigured
	DepartureGateSignalDeferred
)

func (outcome DepartureGateSignalOutcome) String() string {
	switch outcome {
	case DepartureGateSignalRaised:
		return "SIGNAL_RAISED"
	case DepartureGateSignalRuleNotConfigured:
		return "SIGNAL_RULE_NOT_CONFIGURED"
	case DepartureGateSignalDeferred:
		return "SIGNAL_DEFERRED"
	default:
		return ""
	}
}

// RaiseDepartureGateSignalCommand 是一件出发时被关务判为阻断的对象，连同那一次出发：命中条件（阻断）由调用方按源事实判过，
// 本用例不重判关务的答复，也不另存一份读数。
//
// Fact 与 Version 指回那一条移动事实在这件包裹上的引用与版本（`movement-fact/<对象>/<事实>`，与投影那一路同一把键），Judgement
// 是读数里这件对象那一格的关务判断标识。三样一起是信号的事实依据，Fact 同时是因果条件——一次违规出发就是一个原因，各开一期、
// 各自分诊（代决复核报告 M18）。
type RaiseDepartureGateSignalCommand struct {
	TenantID  domain.TenantID
	Parcel    domain.TrackedParcelReference
	Fact      domain.SourceFactReference
	Version   domain.SourceFactVersion
	Judgement string
}

type RaiseDepartureGateSignalResult struct {
	outcome    DepartureGateSignalOutcome
	handoffRef string
}

func (result RaiseDepartureGateSignalResult) Outcome() DepartureGateSignalOutcome {
	return result.outcome
}

// HandoffReference 非空说明分诊结论已落库但意图还没交出去，重放会重发同一份。
func (result RaiseDepartureGateSignalResult) HandoffReference() string {
	return result.handoffRef
}

// RaiseDepartureGateSignalDeps 的 Rules 在装配期绑租户，Signals 是 `UC-VE-004` 的入口（RaiseSignalHandler）。
type RaiseDepartureGateSignalDeps struct {
	Rules   ports.DepartureGateSignalRuleView
	Signals SignalRaiser
	Clock   ports.Clock
}

type RaiseDepartureGateSignalHandler struct {
	deps RaiseDepartureGateSignalDeps
}

func NewRaiseDepartureGateSignalHandler(deps RaiseDepartureGateSignalDeps) (*RaiseDepartureGateSignalHandler, error) {
	if deps.Rules == nil || deps.Signals == nil || deps.Clock == nil {
		return nil, errors.New("visibility exception: the departure gate signal handler needs the rule view, the signal raiser and a clock")
	}
	return &RaiseDepartureGateSignalHandler{deps: deps}, nil
}

// Handle 按本租户登记的出发门禁阻断信号规则形成一次信号命中，交分诊（ADR-0205 决定五）。
//
// 类型、规则版本与可信度三样全来自登记——未配置即如实答「规则未配置」，不替租户拟一种异常类型。因果条件是那一条移动事实：
// 同一件包裹另一次出发被阻断是另一个原因，另开一期、重新分诊；同一次出发的信封重投落在同一期，只记一次命中。这一期没有恢复
// 边界——原因是一次已经发生的出发，事后不会「不再成立」，替它结束发作期就是虚构一次恢复；后续处置归分诊与案件。命中时刻取
// 判断那一刻而不取出发的业务时间：CONTEXT 要信号保存的是「判断时间」，同一期的重复命中按判断时刻也不会倒退。
func (handler *RaiseDepartureGateSignalHandler) Handle(
	ctx context.Context,
	command RaiseDepartureGateSignalCommand,
) (RaiseDepartureGateSignalResult, error) {
	rule, configured, err := handler.deps.Rules.DepartureGateSignalRule(ctx)
	if err != nil {
		return RaiseDepartureGateSignalResult{outcome: DepartureGateSignalDeferred}, nil
	}
	if !configured {
		return RaiseDepartureGateSignalResult{outcome: DepartureGateSignalRuleNotConfigured}, nil
	}
	cause, err := domain.SourceFactCausalCondition(domain.SourceTransportFulfillment, command.Fact)
	if err != nil {
		return RaiseDepartureGateSignalResult{}, fmt.Errorf("%w: causal condition: %v", ErrUnexpectedDepartureGateSignalOutcome, err)
	}
	raised, err := handler.deps.Signals.Handle(ctx, RaiseSignalCommand{
		TenantID:   command.TenantID,
		Kind:       rule.Kind,
		Parcel:     command.Parcel,
		Cause:      cause,
		Rule:       rule.Rule,
		Confidence: rule.Confidence,
		Basis: []domain.SignalFactBasis{{
			Source:    domain.SourceTransportFulfillment,
			Fact:      command.Fact,
			Version:   command.Version,
			Judgement: command.Judgement,
		}},
		HitAt: handler.deps.Clock.Now(),
	})
	if err != nil {
		return RaiseDepartureGateSignalResult{}, fmt.Errorf("hand departure gate signal to triage: %w", err)
	}
	switch raised.Outcome() {
	case SignalEpisodeOpened, SignalHitRecorded, SignalEpisodeReopened:
		return RaiseDepartureGateSignalResult{outcome: DepartureGateSignalRaised, handoffRef: raised.HandoffReference()}, nil
	case RaiseSignalUndecided:
		return RaiseDepartureGateSignalResult{outcome: DepartureGateSignalDeferred}, nil
	default:
		return RaiseDepartureGateSignalResult{}, fmt.Errorf("%w: %q", ErrUnexpectedDepartureGateSignalOutcome, raised.Outcome())
	}
}
