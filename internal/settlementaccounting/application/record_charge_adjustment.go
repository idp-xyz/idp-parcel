// record_charge_adjustment.go 是普通客户费用调整的**唯一创建用例**（UC-SA-002，CONTEXT
// 「调整类型与唯一所有权」表：计价纠错与商业让利归本用例）。赔付与索赔退款归 UC-SA-007、
// 供应商账单贷项归 UC-SA-004、代垫回收调整归 UC-SA-001，各有自己的册与用例。
//
// 本文件是包内唯一够得着 ports.ChargeAdjustmentStore 的地方，由
// internal/architecture 的 TestOnlyTheOwningUseCaseReachesTheChargeAdjustmentRegister
// 守着——种类封闭集只管「册上表达得出什么」，管不了「包里谁写它」，而 CONTEXT 那张表
// 通篇讲的是后者（ADR-0087 决定二）。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// RecordChargeAdjustmentOutcome 是一次调整登记的应用处理结果。`费用未确认`是专格——
// 答案已知（等费用确认后再来），既不是拒绝也不是未决（ADR-0029 按恢复动作分格）。
type RecordChargeAdjustmentOutcome uint8

const (
	RecordChargeAdjustmentOutcomeInvalid RecordChargeAdjustmentOutcome = iota
	ChargeAdjustmentRecordedOutcome
	ChargeAdjustmentReplayedOutcome
	AdjustmentChargeNotConfirmed
	AdjustmentNotAccepted
	AdjustmentUndecided
)

func (outcome RecordChargeAdjustmentOutcome) String() string {
	switch outcome {
	case ChargeAdjustmentRecordedOutcome:
		return "ADJUSTMENT_RECORDED"
	case ChargeAdjustmentReplayedOutcome:
		return "ALREADY_RECORDED"
	case AdjustmentChargeNotConfirmed:
		return "CHARGE_NOT_CONFIRMED"
	case AdjustmentNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case AdjustmentUndecided:
		return "ADJUSTMENT_UNDECIDED"
	default:
		return ""
	}
}

// AdjustmentUndecidedReason 指名登记停在哪一步等谁。
type AdjustmentUndecidedReason uint8

const (
	AdjustmentUndecidedReasonNone AdjustmentUndecidedReason = iota
	AdjustmentChargeStoreUnavailable
	AdjustmentRegisterUnavailable
)

func (reason AdjustmentUndecidedReason) String() string {
	switch reason {
	case AdjustmentChargeStoreUnavailable:
		return "CHARGE_STORE_UNAVAILABLE"
	case AdjustmentRegisterUnavailable:
		return "ADJUSTMENT_REGISTER_UNAVAILABLE"
	default:
		return ""
	}
}

// RecordChargeAdjustmentCommand 指名要对哪笔费用追加哪一笔调整。种类与依据分格由领域
// 的形成门判（纠错必挂新评价、让利必挂商业授权，有此无彼）；形成时点由时钟给出，不由
// 调用方声称——「差异在哪个页面、账期或流程中被发现」不决定调整何时形成。
type RecordChargeAdjustmentCommand struct {
	TenantID           domain.TenantID
	AdjustmentID       string
	ChargeID           string
	Kind               domain.AdjustmentKind
	Direction          domain.AdjustmentDirection
	Evaluation         string
	Authorization      string
	OriginalCurrency   string
	OriginalMinor      int64
	SettlementCurrency string
	SettlementMinor    int64
	Conversion         string
}

type RecordChargeAdjustmentResult struct {
	outcome       RecordChargeAdjustmentOutcome
	reason        AdjustmentUndecidedReason
	adjustment    domain.ChargeAdjustment
	hasAdjustment bool
	continuation  string
}

func (result RecordChargeAdjustmentResult) Outcome() RecordChargeAdjustmentOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result RecordChargeAdjustmentResult) UndecidedReason() AdjustmentUndecidedReason {
	return result.reason
}

func (result RecordChargeAdjustmentResult) Adjustment() (domain.ChargeAdjustment, bool) {
	return result.adjustment, result.hasAdjustment
}

func (result RecordChargeAdjustmentResult) ContinuationReference() string {
	return result.continuation
}

type RecordChargeAdjustmentDeps struct {
	Charges     ports.CustomerChargeStore
	Adjustments ports.ChargeAdjustmentStore
	Clock       ports.Clock
}

type RecordChargeAdjustmentHandler struct {
	deps RecordChargeAdjustmentDeps
}

func NewRecordChargeAdjustmentHandler(deps RecordChargeAdjustmentDeps) *RecordChargeAdjustmentHandler {
	return &RecordChargeAdjustmentHandler{deps: deps}
}

// Handle 追加一笔调整：找回费用 → 未确认停专格 → 领域形成门 → 登记（同标识重放读回
// 原件不改写）。原费用一律不改——CONTEXT「原确认费用仍保留」，本编排连费用的写口都不碰。
func (handler *RecordChargeAdjustmentHandler) Handle(
	ctx context.Context,
	command RecordChargeAdjustmentCommand,
) (RecordChargeAdjustmentResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return RecordChargeAdjustmentResult{outcome: AdjustmentNotAccepted}, nil
	}
	adjustmentID, err := domain.NewChargeAdjustmentID(command.AdjustmentID)
	if err != nil {
		return RecordChargeAdjustmentResult{outcome: AdjustmentNotAccepted}, nil
	}
	chargeID, err := domain.NewCustomerChargeID(command.ChargeID)
	if err != nil {
		return RecordChargeAdjustmentResult{outcome: AdjustmentNotAccepted}, nil
	}
	key := ports.ChargeAdjustmentKey{TenantID: command.TenantID, Adjustment: adjustmentID}

	charge, found, err := handler.deps.Charges.FindByID(ctx, command.TenantID, chargeID)
	if err != nil {
		return RecordChargeAdjustmentResult{
			outcome:      AdjustmentUndecided,
			reason:       AdjustmentChargeStoreUnavailable,
			continuation: adjustmentContinuation("CHARGE_STORE_UNAVAILABLE", command.AdjustmentID),
		}, nil
	}
	if !found {
		// 指名了不存在的费用：提交矛盾，改单重来。
		return RecordChargeAdjustmentResult{outcome: AdjustmentNotAccepted}, nil
	}
	if charge.Stage() != domain.ChargeConfirmed {
		// 调预估或暂估不是调整，是重新预估——CONTEXT 的调整明细只挂在已确认费用上。
		return RecordChargeAdjustmentResult{
			outcome:      AdjustmentChargeNotConfirmed,
			continuation: adjustmentContinuation("CHARGE_NOT_CONFIRMED", command.AdjustmentID, command.ChargeID),
		}, nil
	}

	adjustment, err := handler.form(command, adjustmentID, chargeID)
	if err != nil {
		return RecordChargeAdjustmentResult{outcome: AdjustmentNotAccepted}, nil
	}

	saved, err := handler.deps.Adjustments.Save(ctx, ports.ChargeAdjustmentRecord{
		Key:        key,
		Adjustment: adjustment,
		RecordedAt: handler.deps.Clock.Now(),
	})
	if err != nil {
		return handler.registerUndecided(command.AdjustmentID), nil
	}
	if saved == ports.ChargeAdjustmentAlreadyRecorded {
		// 重复触发返回原结果（CONTEXT 同节）；语义或范围不同的那一笔本就该另起标识。
		existing, present, err := handler.deps.Adjustments.FindByKey(ctx, key)
		if err != nil || !present {
			return handler.registerUndecided(command.AdjustmentID), nil
		}
		return RecordChargeAdjustmentResult{
			outcome:       ChargeAdjustmentReplayedOutcome,
			adjustment:    existing.Adjustment,
			hasAdjustment: true,
		}, nil
	}
	return RecordChargeAdjustmentResult{
		outcome:       ChargeAdjustmentRecordedOutcome,
		adjustment:    adjustment,
		hasAdjustment: true,
	}, nil
}

// form 把命令交给领域的形成门。三个引用各自可缺：缺哪个该拒由种类决定，判据在领域里，
// 这里不预判——预判一次就等于把同一条判据抄成了两份。
func (handler *RecordChargeAdjustmentHandler) form(
	command RecordChargeAdjustmentCommand,
	adjustmentID domain.ChargeAdjustmentID,
	chargeID domain.CustomerChargeID,
) (domain.ChargeAdjustment, error) {
	spec := domain.ChargeAdjustmentSpec{
		ID:        adjustmentID,
		Charge:    chargeID,
		Kind:      command.Kind,
		Direction: command.Direction,
		FormedAt:  handler.deps.Clock.Now(),
	}
	var err error
	if strings.TrimSpace(command.Evaluation) != "" {
		if spec.Evaluation, err = domain.NewSellEvaluationReference(command.Evaluation); err != nil {
			return domain.ChargeAdjustment{}, err
		}
	}
	if strings.TrimSpace(command.Authorization) != "" {
		if spec.Authorization, err = domain.NewCommercialAuthorizationReference(command.Authorization); err != nil {
			return domain.ChargeAdjustment{}, err
		}
	}
	if strings.TrimSpace(command.Conversion) != "" {
		if spec.Conversion, err = domain.NewConversionStepReference(command.Conversion); err != nil {
			return domain.ChargeAdjustment{}, err
		}
	}
	if spec.OriginalCurrency, err = domain.NewCurrencyCode(command.OriginalCurrency); err != nil {
		return domain.ChargeAdjustment{}, err
	}
	if spec.SettlementCurrency, err = domain.NewCurrencyCode(command.SettlementCurrency); err != nil {
		return domain.ChargeAdjustment{}, err
	}
	spec.OriginalMinor = command.OriginalMinor
	spec.SettlementMinor = command.SettlementMinor
	return domain.FormChargeAdjustment(spec)
}

func (handler *RecordChargeAdjustmentHandler) registerUndecided(id string) RecordChargeAdjustmentResult {
	return RecordChargeAdjustmentResult{
		outcome:      AdjustmentUndecided,
		reason:       AdjustmentRegisterUnavailable,
		continuation: adjustmentContinuation("ADJUSTMENT_REGISTER_UNAVAILABLE", id),
	}
}

func adjustmentContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}
