// follow_up_formed_charge.go 是包裹计费时点那一路预估客户费用形成之后、同一笔里要做的两件事（ADR-0336 决定一、二）：
// 把七项确认事实登上确认事实册；这件包裹若已终局为有效交付，补记那份确认依据并按时点确认。
package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrSellRequestAccountMissing 说明 SELL 请求指名的结算账户不在账户册上。请求是照册上那一户提交的，册上又不删行，读不到
// 是两侧说法矛盾，不是取值缺席。
var ErrSellRequestAccountMissing = errors.New("settlement accounting: sell request names a settlement account absent from the register")

// EstimatedChargeFollowUp 是形成编排外层在费用落下之后调的那一步。返回错误时外层整笔不作答，让事务回滚、信封重投。
type EstimatedChargeFollowUp interface {
	AfterEstimatedChargeFormed(
		ctx context.Context,
		tenant domain.TenantID,
		request domain.SellEvaluationRequest,
		charge domain.CustomerChargeID,
	) error
}

type FormedChargeConfirmationDeps struct {
	Accounts     ports.SettlementAccountByIDView
	Facts        ports.ChargeConfirmationFactRegister
	Finals       ports.EffectiveDeliveryFinalView
	Confirmation *ConfirmChargeOnBasisHandler
	Clock        ports.Clock
}

// FormedChargeConfirmationFollowUp 实现 EstimatedChargeFollowUp。
type FormedChargeConfirmationFollowUp struct {
	deps FormedChargeConfirmationDeps
}

var _ EstimatedChargeFollowUp = (*FormedChargeConfirmationFollowUp)(nil)

func NewFormedChargeConfirmationFollowUp(deps FormedChargeConfirmationDeps) (*FormedChargeConfirmationFollowUp, error) {
	if deps.Accounts == nil || deps.Facts == nil || deps.Finals == nil || deps.Confirmation == nil || deps.Clock == nil {
		return nil, fmt.Errorf("%w: formed charge confirmation follow-up", ErrNilDependency)
	}
	return &FormedChargeConfirmationFollowUp{deps: deps}, nil
}

// AfterEstimatedChargeFormed 见文件头注。
//
// 七项事实只从请求与它指名的那一户取：账户、计价范围、来源事实是请求提交时钉下的，法人、相对方、方向是账户的固定属性，
// 合同依据是账户的成立依据。都是形成那一刻已经固定的东西，不是 CONTEXT 禁的「按当前组织或客户属性临时推断」。册上已有
// 不同内容的一行（受控登记口先登过）不覆盖，确认照读册上那一行。
func (followUp *FormedChargeConfirmationFollowUp) AfterEstimatedChargeFormed(
	ctx context.Context,
	tenant domain.TenantID,
	request domain.SellEvaluationRequest,
	charge domain.CustomerChargeID,
) error {
	account, found, err := followUp.deps.Accounts.FindByID(ctx, tenant, request.Account())
	if err != nil {
		return fmt.Errorf("settlement account register: %w", err)
	}
	if !found {
		return fmt.Errorf("%w: request %s account %s", ErrSellRequestAccountMissing, request.ID(), request.Account())
	}
	registration, err := confirmationFactsOf(charge, request, account)
	if err != nil {
		return err
	}
	if _, err := followUp.deps.Facts.SaveChargeConfirmationFacts(ctx, tenant, registration, followUp.deps.Clock.Now()); err != nil {
		return fmt.Errorf("charge confirmation fact register: %w", err)
	}

	basis, finalized, err := followUp.deps.Finals.FindEffectiveDeliveryFinal(ctx, tenant, request.Parcel())
	if err != nil {
		return fmt.Errorf("effective delivery final: %w", err)
	}
	if !finalized {
		return nil
	}
	result, err := followUp.deps.Confirmation.OnCharge(ctx, tenant, charge, ConfirmationBasis{
		Kind: domain.EffectiveDeliveryFinalizedBasis(), Reference: basis,
	})
	if err != nil {
		return err
	}
	if result.Retryable() {
		return fmt.Errorf("confirm on effective delivery final: %s: %w", result.Outcome(), undecidedCauseOf(result))
	}
	return nil
}

func confirmationFactsOf(
	charge domain.CustomerChargeID,
	request domain.SellEvaluationRequest,
	account domain.SettlementAccount,
) (domain.ChargeConfirmationFactRegistration, error) {
	contract, err := domain.NewContractBasisReference(account.Responsibility().String())
	if err != nil {
		return domain.ChargeConfirmationFactRegistration{}, fmt.Errorf("account responsibility basis: %w", err)
	}
	scope, err := domain.NewChargingScopeReference(request.PricingScope().String())
	if err != nil {
		return domain.ChargeConfirmationFactRegistration{}, fmt.Errorf("request pricing scope: %w", err)
	}
	key := account.Key()
	return domain.NewChargeConfirmationFactRegistration(charge, domain.ConfirmedChargeFacts{
		ResponsibleEntity:    key.LegalEntity(),
		Counterparty:         key.Counterparty(),
		Direction:            key.Direction(),
		SettlementAccount:    account.ID(),
		ContractBasis:        contract,
		PrimaryChargingScope: scope,
		SourceFact:           request.Source(),
	})
}

func undecidedCauseOf(result ConfirmOnBasisResult) error {
	if cause := result.UndecidedCause(); cause != nil {
		return cause
	}
	return fmt.Errorf("charge confirmation undecided: %s", result.Confirmation().UndecidedReason())
}
