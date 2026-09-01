// confirm_charge.go 编排普通客户费用的确认半步：条件核对、定格与对账单纳入信号。
// 确认与截单并发时归属由确认条件与截单快照决定（AT-SA-055 前半）——这里只负责把
// 确认本身做对，归属在 UC-SA-003 的截单上。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrUnexpectedChargeSave 说明费用库交回了封闭集合以外的写入结果。
var ErrUnexpectedChargeSave = errors.New("settlement accounting: unexpected charge save outcome")

// ConfirmChargeOutcome 是一次费用确认提交的应用处理结果。`条件未满足`是专格——答案
// 已知（恢复动作是等条件满足再来），不是拒绝也不是未决（ADR-0029 按恢复动作分格）。
type ConfirmChargeOutcome uint8

const (
	ConfirmChargeOutcomeInvalid ConfirmChargeOutcome = iota
	ChargeConfirmedOutcome
	ChargeAlreadyConfirmedOutcome
	ConfirmationConditionNotMet
	ConfirmNotAccepted
	ConfirmUndecided
)

func (outcome ConfirmChargeOutcome) String() string {
	switch outcome {
	case ChargeConfirmedOutcome:
		return "CHARGE_CONFIRMED"
	case ChargeAlreadyConfirmedOutcome:
		return "ALREADY_CONFIRMED"
	case ConfirmationConditionNotMet:
		return "CONDITION_NOT_MET"
	case ConfirmNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case ConfirmUndecided:
		return "CONFIRM_UNDECIDED"
	default:
		return ""
	}
}

// ConfirmUndecidedReason 指名提交停在哪一步等谁。确认条件目录未配置是实例半边的一格
// ——不默认转正。
type ConfirmUndecidedReason uint8

const (
	ConfirmUndecidedReasonNone ConfirmUndecidedReason = iota
	ChargeStoreUnavailable
	ConditionViewUnavailable
	ConditionUnconfigured
	FactsViewUnavailable
	ConfirmationFactsUnconfigured
)

func (reason ConfirmUndecidedReason) String() string {
	switch reason {
	case ChargeStoreUnavailable:
		return "CHARGE_STORE_UNAVAILABLE"
	case ConditionViewUnavailable:
		return "CONDITION_VIEW_UNAVAILABLE"
	case ConditionUnconfigured:
		return "CONDITION_UNCONFIGURED"
	case FactsViewUnavailable:
		return "FACTS_VIEW_UNAVAILABLE"
	case ConfirmationFactsUnconfigured:
		return "CONFIRMATION_FACTS_UNCONFIGURED"
	default:
		return ""
	}
}

// ConfirmChargeCommand 只指名要确认哪笔费用。刻意没有金额与依据字段——确认不改金额
// （金额变化走调整），确认依据由条件核对给出，不由调用方口头声称。确认时要固定的七项
// 事实同理由结算事实读口给出：命令带得了它们，CONTEXT 禁的「临时推断」就只是换了个人做
// （ADR-0087 决定一）。
type ConfirmChargeCommand struct {
	TenantID domain.TenantID
	ChargeID string
}

type ConfirmChargeResult struct {
	outcome      ConfirmChargeOutcome
	reason       ConfirmUndecidedReason
	charge       domain.CustomerCharge
	hasCharge    bool
	continuation string
	handoff      string
}

func (result ConfirmChargeResult) Outcome() ConfirmChargeOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result ConfirmChargeResult) UndecidedReason() ConfirmUndecidedReason {
	return result.reason
}

func (result ConfirmChargeResult) Charge() (domain.CustomerCharge, bool) {
	return result.charge, result.hasCharge
}

func (result ConfirmChargeResult) ContinuationReference() string {
	return result.continuation
}

// ConfirmationHandoffReference 非空说明确认已提交但意图还没交出去，重放会重发同一份。
func (result ConfirmChargeResult) ConfirmationHandoffReference() string {
	return result.handoff
}

type ConfirmChargeDeps struct {
	Charges    ports.CustomerChargeStore
	Conditions ports.ConfirmationConditionView
	Facts      ports.ConfirmedChargeFactsView
	Downstream ports.ChargeConfirmationHandoff
	Clock      ports.Clock
}

type ConfirmChargeHandler struct {
	deps ConfirmChargeDeps
}

func NewConfirmChargeHandler(deps ConfirmChargeDeps) *ConfirmChargeHandler {
	return &ConfirmChargeHandler{deps: deps}
}

// Handle 把一笔预估/暂估费用推进到确认：找回费用 → 已确认即重放返原确认不二确 →
// 条件核对（未配置→未决不默认转正；未满足→专格带缺口）→ CustomerCharge.Confirm
// （金额不改，领域已钉，本编排连金额输入都没有）→ 提交 → 发布意图。
func (handler *ConfirmChargeHandler) Handle(
	ctx context.Context,
	command ConfirmChargeCommand,
) (ConfirmChargeResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return ConfirmChargeResult{outcome: ConfirmNotAccepted}, nil
	}
	chargeID, err := domain.NewCustomerChargeID(command.ChargeID)
	if err != nil {
		return ConfirmChargeResult{outcome: ConfirmNotAccepted}, nil
	}

	charge, found, err := handler.deps.Charges.FindByID(ctx, command.TenantID, chargeID)
	if err != nil {
		return chargeStoreUndecided(command.ChargeID), nil
	}
	if !found {
		// 指名了不存在的费用：提交矛盾，改单重来。
		return ConfirmChargeResult{outcome: ConfirmNotAccepted}, nil
	}
	if charge.Stage() == domain.ChargeConfirmed {
		// 已确认费用重复提交：返回原确认，不二确、不换依据（幂等）。
		return handler.alreadyConfirmed(ctx, command.TenantID, charge), nil
	}

	condition, configured, err := handler.deps.Conditions.LoadConfirmationCondition(
		ctx, command.TenantID, chargeID, charge.FeeItem())
	if err != nil {
		return ConfirmChargeResult{outcome: ConfirmUndecided, reason: ConditionViewUnavailable,
			continuation: confirmContinuation("CONDITION_VIEW_UNAVAILABLE", command.ChargeID)}, nil
	}
	if !configured {
		// 确认条件目录是实例半边：未配置停在未决，不默认转正。
		return ConfirmChargeResult{outcome: ConfirmUndecided, reason: ConditionUnconfigured,
			continuation: confirmContinuation("CONDITION_UNCONFIGURED", command.ChargeID)}, nil
	}
	if !condition.Met {
		// 条件未满足是已知答案：带缺口的专格，等条件满足再来——不是拒绝这笔费用。
		return ConfirmChargeResult{outcome: ConfirmationConditionNotMet,
			continuation: confirmContinuation("CONDITION_NOT_MET", command.ChargeID, condition.Gap)}, nil
	}

	facts, registered, err := handler.deps.Facts.LoadConfirmedChargeFacts(ctx, command.TenantID, chargeID)
	if err != nil {
		return ConfirmChargeResult{outcome: ConfirmUndecided, reason: FactsViewUnavailable,
			continuation: confirmContinuation("FACTS_VIEW_UNAVAILABLE", command.ChargeID)}, nil
	}
	if !registered {
		// 七项事实是实例半边：无处可取停在未决，不用空值凑出一次确认——那正是 CONTEXT
		// 禁的「临时推断」，只不过推断的人换成了这段代码。
		return ConfirmChargeResult{outcome: ConfirmUndecided, reason: ConfirmationFactsUnconfigured,
			continuation: confirmContinuation("CONFIRMATION_FACTS_UNCONFIGURED", command.ChargeID)}, nil
	}

	// 缺项由领域的确认门拒：册上交出一份不全的，是册与本用例之间的提交矛盾，改单重来。
	confirmed, err := charge.Confirm(facts, condition.Basis, handler.deps.Clock.Now())
	if err != nil {
		return ConfirmChargeResult{outcome: ConfirmNotAccepted}, nil
	}

	saved, err := handler.deps.Charges.SaveConfirmed(ctx, command.TenantID, confirmed)
	if err != nil {
		return chargeStoreUndecided(command.ChargeID), nil
	}
	switch saved {
	case ports.ChargeSaved:
		result := ConfirmChargeResult{outcome: ChargeConfirmedOutcome, charge: confirmed, hasCharge: true}
		result.handoff = handler.handOff(ctx, command.TenantID, confirmed)
		return result, nil
	case ports.ChargeAlreadyConfirmed:
		// 并发二确落败：读回赢家的确认作答，不覆盖。
		winner, found, err := handler.deps.Charges.FindByID(ctx, command.TenantID, chargeID)
		if err != nil || !found {
			return chargeStoreUndecided(command.ChargeID), nil
		}
		return handler.alreadyConfirmed(ctx, command.TenantID, winner), nil
	default:
		return ConfirmChargeResult{}, fmt.Errorf("%w: %d", ErrUnexpectedChargeSave, saved)
	}
}

func chargeStoreUndecided(chargeID string) ConfirmChargeResult {
	return ConfirmChargeResult{
		outcome:      ConfirmUndecided,
		reason:       ChargeStoreUnavailable,
		continuation: confirmContinuation("CHARGE_STORE_UNAVAILABLE", chargeID),
	}
}

// alreadyConfirmed 按已有确认作答并重发同一份意图。
func (handler *ConfirmChargeHandler) alreadyConfirmed(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerCharge,
) ConfirmChargeResult {
	return ConfirmChargeResult{
		outcome:   ChargeAlreadyConfirmedOutcome,
		charge:    charge,
		hasCharge: true,
		handoff:   handler.handOff(ctx, tenant, charge),
	}
}

// handOff 把已确认费用交给对账单纳入消费。投递失败不翻结果，留续办引用重放时重发
// 同一份（ADR-0043 同款纪律）。
func (handler *ConfirmChargeHandler) handOff(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerCharge,
) string {
	if err := handler.deps.Downstream.HandOffChargeConfirmation(ctx, ports.ChargeConfirmationHandoffIntent{
		TenantID: tenant,
		Charge:   charge,
	}); err == nil {
		return ""
	}
	return confirmContinuation("CHARGE_CONFIRMATION_HANDOFF", tenant.String(), charge.ID().String())
}

func confirmContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}
