package customscompliance

import (
	"context"
	"errors"
	"fmt"

	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	// ErrVerificationNotVisible 表示按信封引用在提供方还读不回那一版核对。可见性滞后是续办，重投会改变结果；
	// 不当毒丸拒收。生产装配把它登进 WithUndecidedSentinels：运维据此去查提供方那一侧，而不是查传输。
	ErrVerificationNotVisible = errors.New(
		"settlement accounting customscompliance adapter: duty payment verification is not yet visible")
	// ErrAdoptionUndecided 表示采用编排停在自己的未决上（登记册不可用）。同样是续办，同样进 WithUndecidedSentinels。
	ErrAdoptionUndecided = errors.New(
		"settlement accounting customscompliance adapter: adopting the duty payment verification is undecided")
	// ErrUnexpectedAdoptionOutcome 表示编排交回了封闭集合以外的结果。静默入账等于替编排作判断，因此不留
	// default 兜底。
	ErrUnexpectedAdoptionOutcome = errors.New(
		"settlement accounting customscompliance adapter: unexpected adoption outcome")
)

// DutyPaymentVerificationAdopter 是结算输入采用编排的那一口（UC-SA-001 步 2 的付款核对一格）。真实装配接
// saapplication.AssessAdvanceRecoveryHandler。
type DutyPaymentVerificationAdopter interface {
	AdoptDutyPaymentVerification(
		ctx context.Context,
		command saapplication.AdoptDutyPaymentVerificationCommand,
	) (saapplication.SettlementInputResult, error)
}

// AdoptOnDutyPaymentVerificationAdapter 是 sainbox.DutyPaymentVerificationConsumer 的真实处理方：按信封引用
// 向提供方核那一版在册，译成采用命令交 AdoptDutyPaymentVerification。
//
// 只译不判（票 sa-cc/09 红线）：不读三态、不判代垫、不调 FormRecovery——核对形成不是代垫成立，其余输入
// 缺哪一项由编排保持待判断；同引用重放答`已存在`是编排已有的答案，这里照单入账。
type AdoptOnDutyPaymentVerificationAdapter struct {
	view    saports.DutyPaymentVerificationView
	adopter DutyPaymentVerificationAdopter
}

func NewAdoptOnDutyPaymentVerificationAdapter(
	view saports.DutyPaymentVerificationView,
	adopter DutyPaymentVerificationAdopter,
) (*AdoptOnDutyPaymentVerificationAdapter, error) {
	if view == nil {
		return nil, fmt.Errorf("settlement accounting customscompliance adapter: duty payment verification view is nil")
	}
	if adopter == nil {
		return nil, fmt.Errorf("settlement accounting customscompliance adapter: duty payment verification adopter is nil")
	}
	return &AdoptOnDutyPaymentVerificationAdapter{view: view, adopter: adopter}, nil
}

var _ sainbox.FormedDutyPaymentVerificationHandler = (*AdoptOnDutyPaymentVerificationAdapter)(nil)

// HandleFormedDutyPaymentVerification 译引用 → 核在册 → 交编排 → 把编排结果落成消费两格。
//
// 先核在册再采用，是为了让「采用」指的一定是提供方真有的一版：信封与核对行在提供方同一事务落库，同库
// 同进程下读不回只剩可见性滞后一种成因，如实答未决重投，不把一个还不存在的引用登进结算输入。
func (adapter *AdoptOnDutyPaymentVerificationAdapter) HandleFormedDutyPaymentVerification(
	ctx context.Context,
	formed sainbox.FormedDutyPaymentVerification,
) error {
	tenant, err := sadomain.NewTenantID(formed.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableReference, err)
	}
	verification, err := verificationReference(formed)
	if err != nil {
		return err
	}

	visible, err := adapter.view.DutyPaymentVerificationExists(ctx, tenant, verification)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrVerificationNotVisible, err)
	}
	if !visible {
		return fmt.Errorf("%w: scope %q duty %q funds %q digest %q",
			ErrVerificationNotVisible, formed.Scope, formed.Duty, formed.Funds, formed.Digest)
	}

	result, err := adapter.adopter.AdoptDutyPaymentVerification(ctx, saapplication.AdoptDutyPaymentVerificationCommand{
		TenantID: tenant,
		Scope:    formed.Scope,
		Duty:     formed.Duty,
		Funds:    formed.Funds,
		Version:  formed.Digest,
	})
	if err != nil {
		return err
	}
	return adoptionConsumption(result)
}

func verificationReference(formed sainbox.FormedDutyPaymentVerification) (sadomain.DutyPaymentVerificationReference, error) {
	scope, err := sadomain.NewDeclarationScopeReference(formed.Scope)
	if err != nil {
		return sadomain.DutyPaymentVerificationReference{}, fmt.Errorf("%w: scope: %v", ErrUntranslatableReference, err)
	}
	duty, err := sadomain.NewTaxObligationReference(formed.Duty)
	if err != nil {
		return sadomain.DutyPaymentVerificationReference{}, fmt.Errorf("%w: duty: %v", ErrUntranslatableReference, err)
	}
	funds, err := sadomain.NewFundsFactReference(formed.Funds)
	if err != nil {
		return sadomain.DutyPaymentVerificationReference{}, fmt.Errorf("%w: funds: %v", ErrUntranslatableReference, err)
	}
	version, err := sadomain.NewDutyVerificationVersion(formed.Digest)
	if err != nil {
		return sadomain.DutyPaymentVerificationReference{}, fmt.Errorf("%w: digest: %v", ErrUntranslatableReference, err)
	}
	reference, err := sadomain.NewDutyPaymentVerificationReference(scope, duty, funds, version)
	if err != nil {
		return sadomain.DutyPaymentVerificationReference{}, fmt.Errorf("%w: %v", ErrUntranslatableReference, err)
	}
	return reference, nil
}

// adoptionConsumption 把编排结果落成消费两格：采用、已存在、未受理都是编排给出的答案，入账；未决是等依赖，
// 重投。封闭集之外响亮报错。
//
// 未受理入账不重投，与 CC 侧 receiveConsumption 对 `DutyReconciliationNotAccepted` 的处置同形：重投同样内容
// 不会长出字段来；消费者译码已拒过五维缺席，这一格在生产上只剩两侧词汇分歧一类，入账让它留痕而不是
// 一路重投到失败预算耗尽。
func adoptionConsumption(result saapplication.SettlementInputResult) error {
	switch result.Outcome() {
	case saapplication.DutyPaymentVerificationAdopted,
		saapplication.DutyPaymentVerificationExistingResult,
		saapplication.SettlementInputNotAccepted:
		return nil
	case saapplication.SettlementInputUndecidedOutcome:
		return fmt.Errorf("%w: %s", ErrAdoptionUndecided, result.UndecidedReason())
	default:
		return fmt.Errorf("%w: %q", ErrUnexpectedAdoptionOutcome, result.Outcome())
	}
}
