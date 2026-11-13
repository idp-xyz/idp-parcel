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
	"fmt"
	"strings"
	"time"

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
	// ChargeAdjustmentConflict：同一个调整标识下登的是另一笔内容（另一笔费用、种类、方向或金额）。不当重放交回——只看答复
	// 名字的调用方会以为这次的金额落了。
	ChargeAdjustmentConflict
	// AdjustmentExceedsCharge：这笔贷项落下之后，费用连同它的全部调整会成负数。一笔让利最多让到零。
	AdjustmentExceedsCharge
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
	case ChargeAdjustmentConflict:
		return "ADJUSTMENT_CONFLICT"
	case AdjustmentExceedsCharge:
		return "ADJUSTMENT_EXCEEDS_CHARGE"
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
	// Balances 与 Books 让调整落下之后重算客户作用域的已确认未结（与费用确认调用的是同一个函数）。生产装配必给；
	// 不给时不重算，只供不关心余额的单元测试用。
	Balances ports.OperationalBalanceAdvance
	Books    ports.CustomerReceiptBooks
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
	chargeCurrency, chargeMinor := charge.SettlementAmount()
	adjustmentCurrency, adjustmentMinor := adjustment.SettlementAmount()
	if adjustmentCurrency != chargeCurrency {
		// 调整进的是费用所在的那个结算作用域：结算币种与费用不同的调整落下之后不进任何余额，登记的人却看到「已登记」。
		return RecordChargeAdjustmentResult{outcome: AdjustmentNotAccepted}, nil
	}

	// 应收方向的费用：先锁它的客户作用域，再读既有调整、落这一笔、重算——同一账户上的调整与确认排成一列，两笔各自
	// 不越界的贷项不会合起来把费用让成负数。
	facts, _ := charge.ConfirmedFacts()
	recompute := facts.Direction == domain.ChargeReceivable && handler.deps.Balances != nil && handler.deps.Books != nil
	var scope domain.SettlementScope
	if recompute {
		if scope, err = domain.NewSettlementScope(facts.ResponsibleEntity, facts.SettlementAccount, chargeCurrency); err != nil {
			return RecordChargeAdjustmentResult{}, fmt.Errorf("record charge adjustment: settlement scope: %w", err)
		}
		if err := handler.deps.Balances.LockOperationalBalanceScope(ctx, command.TenantID, scope); err != nil {
			return handler.registerUndecided(command.AdjustmentID), nil
		}
	}

	if existing, present, err := handler.deps.Adjustments.FindByKey(ctx, key); err != nil {
		return handler.registerUndecided(command.AdjustmentID), nil
	} else if present {
		return replayOrConflict(existing.Adjustment, adjustment), nil
	}

	recorded, err := handler.deps.Adjustments.ListByCharge(ctx, command.TenantID, chargeID)
	if err != nil {
		return handler.registerUndecided(command.AdjustmentID), nil
	}
	net := chargeMinor + signedAdjustment(adjustment.Direction(), adjustmentMinor)
	for _, record := range recorded {
		_, minor := record.Adjustment.SettlementAmount()
		net += signedAdjustment(record.Adjustment.Direction(), minor)
	}
	if net < 0 {
		return RecordChargeAdjustmentResult{outcome: AdjustmentExceedsCharge}, nil
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
		return replayOrConflict(existing.Adjustment, adjustment), nil
	}
	if recompute {
		// 重算失败（账不平、余额没推进）作错误交回：装配点整笔回滚，调整与余额同生同灭。
		if _, err := recomputeCustomerScope(ctx, command.TenantID, scope, handler.deps.Balances, handler.deps.Books, handler.deps.Clock); err != nil {
			return RecordChargeAdjustmentResult{}, fmt.Errorf("record charge adjustment: recompute: %w", err)
		}
	}
	return RecordChargeAdjustmentResult{
		outcome:       ChargeAdjustmentRecordedOutcome,
		adjustment:    adjustment,
		hasAdjustment: true,
	}, nil
}

// signedAdjustment 把一笔调整折成对费用金额的增减：借项加、贷项减。
func signedAdjustment(direction domain.AdjustmentDirection, minor int64) int64 {
	if direction == domain.AdjustmentCredit {
		return -minor
	}
	return minor
}

// replayOrConflict 判同一标识下库里那一份与这次要登的是不是同一笔：费用、种类、方向、依据与两格金额都相同即重放，
// 否则是冲突。两种都交回库里那一份，操作者看得到自己撞上的是什么。形成时刻不比——它是服务端每次给的。
func replayOrConflict(stored, submitted domain.ChargeAdjustment) RecordChargeAdjustmentResult {
	storedEvaluation, _ := stored.Evaluation()
	submittedEvaluation, _ := submitted.Evaluation()
	storedAuthorization, _ := stored.Authorization()
	submittedAuthorization, _ := submitted.Authorization()
	storedOriginalCurrency, storedOriginalMinor := stored.OriginalAmount()
	submittedOriginalCurrency, submittedOriginalMinor := submitted.OriginalAmount()
	storedCurrency, storedMinor := stored.SettlementAmount()
	submittedCurrency, submittedMinor := submitted.SettlementAmount()
	same := stored.Charge() == submitted.Charge() && stored.Kind() == submitted.Kind() && stored.Direction() == submitted.Direction() &&
		storedEvaluation == submittedEvaluation && storedAuthorization == submittedAuthorization &&
		storedOriginalCurrency == submittedOriginalCurrency && storedOriginalMinor == submittedOriginalMinor &&
		storedCurrency == submittedCurrency && storedMinor == submittedMinor
	outcome := ChargeAdjustmentConflict
	if same {
		outcome = ChargeAdjustmentReplayedOutcome
	}
	return RecordChargeAdjustmentResult{outcome: outcome, adjustment: stored, hasAdjustment: true}
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
		// 截到微秒：库里的时刻只到微秒，登记的人与随后读回的人该看到同一个形成时刻。
		FormedAt: handler.deps.Clock.Now().UTC().Truncate(time.Microsecond),
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
