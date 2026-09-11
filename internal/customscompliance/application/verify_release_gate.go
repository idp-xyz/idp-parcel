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

// VerifyReleaseGateDeps 是门禁核对的六口。DutyRules 与 DutyVerifications 是「税费付款」那一道的两半
// （票 sa-cc/06）：规则从门禁目录读，核对从付款核对册读当前版；两口都必备——只走「不构成前置条件」
// 那一形的装配点也得接真核对读口，构造门对每一口一视同仁。
type VerifyReleaseGateDeps struct {
	Conditions        ports.GateConditionView
	DutyRules         ports.DutyPaymentGateRuleView
	DutyVerifications ports.CurrentDutyVerificationView
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

// Handle 把一次门禁核对推进到版本化判断：盘前置条件逐项判断（目录未登记未决——没有清单的门禁判断
// 无从复核；空清单是「此动作在此边界不受门禁」的如实答案）→ 取「税费付款」那一道的登记规则（没有
// 即「规则未配置」停点，不取任何默认折法）→ 规则要读核对时取当前那一版、按接受集合折出这一道的
// 判断（无核对 / 三态待确认或冲突各自停点）→ 领域折叠（冲突压过满足与未满足）→ 幂等按
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

	findings, configured, err := handler.deps.Conditions.LoadPreconditionFindings(
		ctx, command.TenantID, command.Scope, command.Action, command.Boundary)
	if err != nil {
		return gateUndecided(GateConditionViewUnavailable), nil
	}
	if !configured {
		return gateUndecided(GateCatalogNotConfigured), nil
	}

	rule, found, err := handler.deps.DutyRules.LoadDutyPaymentGateRule(
		ctx, command.TenantID, command.Scope, command.Action, command.Boundary)
	if err != nil {
		return gateUndecided(DutyPaymentGateRuleViewUnavailable), nil
	}
	if !found {
		return gateUndecided(DutyPaymentGateRuleNotConfigured), nil
	}
	for _, finding := range findings {
		if finding.Precondition == domain.DutyPaymentPrecondition {
			// 这一道登规则不登认定（ADR-0137 决定三）；册上两样都有，说的是两件事，等登记方收掉
			// 认定那一行——不替它选一边。
			return gateUndecided(DutyPaymentFindingRegisteredBesideRule), nil
		}
	}

	var reading domain.DutyPaymentGateReading
	applied := false
	if !rule.NotAPrecondition() {
		record, found, err := handler.deps.DutyVerifications.LoadCurrentDutyVerification(ctx, command.TenantID, command.Scope)
		if err != nil {
			return gateUndecided(DutyVerificationViewUnavailable), nil
		}
		if !found {
			return gateUndecided(DutyVerificationAbsent), nil
		}
		reading, err = rule.Judge(record.Verification)
		switch {
		case errors.Is(err, domain.ErrDutyPaymentGateUndecided):
			return gateUndecided(DutyVerificationPending), nil
		case err != nil:
			// 规则与核对都经领域构造把过门，走到这里是编程错误，不折成任何业务格。
			return VerifyReleaseGateResult{}, err
		}
		reading.Verification.Version = record.Key.Digest
		findings = append(findings, domain.PreconditionFinding{
			Precondition: domain.DutyPaymentPrecondition,
			State:        reading.State,
		})
		applied = true
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
	if applied {
		if gate, err = gate.WithDutyPayment(reading); err != nil {
			return VerifyReleaseGateResult{}, err
		}
	}

	key := ports.GateVerificationKey{
		TenantID: command.TenantID,
		Scope:    command.Scope,
		Action:   command.Action,
		Boundary: command.Boundary,
		Digest:   ports.GateVersionDigest(findings, reading, applied),
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
