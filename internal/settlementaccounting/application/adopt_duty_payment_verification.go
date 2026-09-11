package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrUnexpectedAdoptionSave 说明采用登记册交回了封闭集合以外的写入结果。
var ErrUnexpectedAdoptionSave = errors.New("settlement accounting: unexpected duty payment verification adoption save outcome")

// SettlementInputOutcome 是结算输入采用（UC-SA-001 步 2「采用明确版本的……付款核对 → 形成结算输入版本」）
// 的应用处理结果。它与 AdvanceOutcome 是两套代数，有意不并：采用一版输入是「结算输入已接收」，判断是
// 「实际代垫成立 / 不成立 / 待判断 / 冲突」——CONTEXT 生命周期把两者写成先后两格，且明写「接收不表示
// 代垫或回收已经成立」；并进同一枚举，读的人会把「输入已接收」当成判断的一种。
type SettlementInputOutcome uint8

const (
	SettlementInputOutcomeInvalid SettlementInputOutcome = iota
	DutyPaymentVerificationAdopted
	DutyPaymentVerificationExistingResult
	SettlementInputNotAccepted
	SettlementInputUndecidedOutcome
)

func (outcome SettlementInputOutcome) String() string {
	switch outcome {
	case DutyPaymentVerificationAdopted:
		return "DUTY_PAYMENT_VERIFICATION_ADOPTED"
	case DutyPaymentVerificationExistingResult:
		return "EXISTING_DUTY_PAYMENT_VERIFICATION"
	case SettlementInputNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case SettlementInputUndecidedOutcome:
		return "SETTLEMENT_INPUT_UNDECIDED"
	default:
		return ""
	}
}

// SettlementInputUndecidedReason 指名采用停在哪一步等谁。今天只有一格：登记册不可用。
type SettlementInputUndecidedReason uint8

const (
	SettlementInputUndecidedReasonNone SettlementInputUndecidedReason = iota
	InputStoreUnavailable
)

func (reason SettlementInputUndecidedReason) String() string {
	switch reason {
	case InputStoreUnavailable:
		return "INPUT_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// AdoptDutyPaymentVerificationCommand 携带一版 customs-compliance 税费付款核对的引用：核对身份三维加
// 版本指纹。刻意没有三态、金额或裁决字段——调用方（inbox 消费者）只译引用不判业务，而本上下文按引用
// 回读核对、不复制结论。
type AdoptDutyPaymentVerificationCommand struct {
	TenantID domain.TenantID
	Scope    string
	Duty     string
	Funds    string
	Version  string
}

type SettlementInputResult struct {
	outcome      SettlementInputOutcome
	reason       SettlementInputUndecidedReason
	adoption     ports.DutyPaymentVerificationAdoptionRecord
	hasRecord    bool
	continuation string
}

func (result SettlementInputResult) Outcome() SettlementInputOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result SettlementInputResult) UndecidedReason() SettlementInputUndecidedReason {
	return result.reason
}

func (result SettlementInputResult) Adoption() (ports.DutyPaymentVerificationAdoptionRecord, bool) {
	return result.adoption, result.hasRecord
}

func (result SettlementInputResult) ContinuationReference() string {
	return result.continuation
}

// AdoptDutyPaymentVerification 把一版付款核对采用进结算输入版本：引用四维构造 → 幂等按（租户 + 完整引用）
// 分重放 → 落一行「引用 + 采用时刻」。
//
// 它**不形成实际代垫判断**，也不读其余输入到没到。UC-SA-001 步 2 的结果是「形成结算输入版本；缺失保持
// 待判断」——付款方、外部资金事实的采用、合同责任几格今天没有采用口，判断因此保持待判断；那是本步的
// 如实答案，不是错误，所以这里没有任何一条路会去调 Assess，也没有任何一条路会因为「别的输入不在」而
// 报错或停在未决。采用了核对也不等于代垫成立（CONTEXT「不由任一单项输入直接推导」），
// FormRecovery 更不在本路上。
func (handler *AssessAdvanceRecoveryHandler) AdoptDutyPaymentVerification(
	ctx context.Context,
	command AdoptDutyPaymentVerificationCommand,
) (SettlementInputResult, error) {
	// 租户零值与引用四维缺席同一格：它是导出方法，不能指望调用方都像 inbox 适配器那样先过 NewTenantID。
	// 不在这里拦，空租户会走到登记册撞 refs_not_blank 而被译成`未决`重投——重投不会长出租户来（与
	// RequestBuyEvaluationHandler.Handle 对租户空白先答`未受理`同形）。
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return SettlementInputResult{outcome: SettlementInputNotAccepted}, nil
	}
	verification, err := verificationReferenceFrom(command)
	if err != nil {
		return SettlementInputResult{outcome: SettlementInputNotAccepted}, nil
	}
	adoption, err := domain.AdoptDutyPaymentVerification(verification, handler.deps.Clock.Now())
	if err != nil {
		return SettlementInputResult{outcome: SettlementInputNotAccepted}, nil
	}

	key := ports.DutyPaymentVerificationAdoptionKey{TenantID: command.TenantID, Verification: verification}
	existing, found, err := handler.deps.SettlementInputs.FindByKey(ctx, key)
	if err != nil {
		return settlementInputUndecided(InputStoreUnavailable, command), nil
	}
	if found {
		return SettlementInputResult{outcome: DutyPaymentVerificationExistingResult, adoption: existing, hasRecord: true}, nil
	}

	record := ports.DutyPaymentVerificationAdoptionRecord{Key: key, Adoption: adoption}
	saved, err := handler.deps.SettlementInputs.Save(ctx, record)
	if err != nil {
		return settlementInputUndecided(InputStoreUnavailable, command), nil
	}
	switch saved {
	case ports.DutyPaymentVerificationAdoptionSaved:
		return SettlementInputResult{outcome: DutyPaymentVerificationAdopted, adoption: record, hasRecord: true}, nil
	case ports.DutyPaymentVerificationAlreadyAdopted:
		winner, found, err := handler.deps.SettlementInputs.FindByKey(ctx, key)
		if err != nil || !found {
			return settlementInputUndecided(InputStoreUnavailable, command), nil
		}
		return SettlementInputResult{outcome: DutyPaymentVerificationExistingResult, adoption: winner, hasRecord: true}, nil
	default:
		return SettlementInputResult{}, fmt.Errorf("%w: %d", ErrUnexpectedAdoptionSave, saved)
	}
}

func verificationReferenceFrom(command AdoptDutyPaymentVerificationCommand) (domain.DutyPaymentVerificationReference, error) {
	scope, err := domain.NewDeclarationScopeReference(command.Scope)
	if err != nil {
		return domain.DutyPaymentVerificationReference{}, err
	}
	duty, err := domain.NewTaxObligationReference(command.Duty)
	if err != nil {
		return domain.DutyPaymentVerificationReference{}, err
	}
	funds, err := domain.NewFundsFactReference(command.Funds)
	if err != nil {
		return domain.DutyPaymentVerificationReference{}, err
	}
	version, err := domain.NewDutyVerificationVersion(command.Version)
	if err != nil {
		return domain.DutyPaymentVerificationReference{}, err
	}
	return domain.NewDutyPaymentVerificationReference(scope, duty, funds, version)
}

func settlementInputUndecided(reason SettlementInputUndecidedReason, command AdoptDutyPaymentVerificationCommand) SettlementInputResult {
	return SettlementInputResult{
		outcome: SettlementInputUndecidedOutcome,
		reason:  reason,
		continuation: advanceContinuation(reason.String(),
			command.TenantID.String(), command.Scope, command.Duty, command.Funds, command.Version),
	}
}
