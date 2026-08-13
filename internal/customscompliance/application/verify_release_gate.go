package application

import (
	"context"
	"errors"
	"fmt"
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

// VerifyReleaseGateCommand 携带一次门禁核对请求：申报范围、拟执行动作与适用监管边界
// ——三者共同构成判断身份（门禁判断不能复用于其他动作或边界，硬句 216）。
type VerifyReleaseGateCommand struct {
	TenantID domain.TenantID
	Scope    domain.DecisionScopeReference
	Action   domain.GuardedAction
	Boundary domain.CustomsProcedureReference
}

type VerifyReleaseGateResult struct {
	outcome    VerifyGateOutcome
	gate       domain.ReleaseGateVerification
	hasRecord  bool
	handoffRef string
}

func (result VerifyReleaseGateResult) Outcome() VerifyGateOutcome {
	return result.outcome
}

func (result VerifyReleaseGateResult) Gate() (domain.ReleaseGateVerification, bool) {
	return result.gate, result.hasRecord
}

// HandoffReference 非空说明核对已入册但意图还没交出去，重放会重发同一份。
func (result VerifyReleaseGateResult) HandoffReference() string {
	return result.handoffRef
}

type VerifyReleaseGateDeps struct {
	Conditions ports.GateConditionView
	Store      ports.GateVerificationStore
	Downstream ports.GateVerificationHandoff
	Clock      ports.Clock
}

type VerifyReleaseGateHandler struct {
	deps VerifyReleaseGateDeps
}

func NewVerifyReleaseGateHandler(deps VerifyReleaseGateDeps) *VerifyReleaseGateHandler {
	return &VerifyReleaseGateHandler{deps: deps}
}

// Handle 把一次门禁核对推进到版本化判断：盘前置条件逐项判断（目录未登记未决——没有
// 清单的门禁判断无从复核；空清单是「此动作在此边界不受门禁」的如实答案）→ 领域折叠
// （冲突压过满足与未满足）→ 幂等按（范围+动作+边界+逐项判断指纹）——条件状态变化
// 自然换指纹换版 → 提交与意图。类型上没有放行字段：门禁满足不生成放行，放行结果仍由
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

	findings, configured, err := handler.deps.Conditions.LoadPreconditionFindings(
		ctx, command.TenantID, command.Scope, command.Action, command.Boundary)
	if err != nil || !configured {
		return VerifyReleaseGateResult{outcome: GateVerificationUndecided}, nil
	}

	conclusion, err := domain.FoldGateConclusion(findings)
	if err != nil {
		return VerifyReleaseGateResult{outcome: GateVerificationNotAccepted}, nil
	}
	preconditions := make([]domain.PreconditionReference, 0, len(findings))
	for _, finding := range findings {
		preconditions = append(preconditions, finding.Precondition)
	}

	gate, err := domain.VerifyReleaseGate(
		command.Scope, command.Action, command.Boundary,
		preconditions, conclusion, handler.deps.Clock.Now())
	if err != nil {
		return VerifyReleaseGateResult{outcome: GateVerificationNotAccepted}, nil
	}

	key := ports.GateVerificationKey{
		TenantID: command.TenantID,
		Scope:    command.Scope,
		Action:   command.Action,
		Boundary: command.Boundary,
		Digest:   ports.FindingsDigest(findings),
	}
	saved, err := handler.deps.Store.Save(ctx, key, gate)
	if err != nil {
		return VerifyReleaseGateResult{outcome: GateVerificationUndecided}, nil
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
			return VerifyReleaseGateResult{outcome: GateVerificationUndecided}, nil
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
