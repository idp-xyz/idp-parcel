// Package transportfulfillment 是 settlement-accounting 消费 transport-fulfillment 的一侧（ADR-0025）：发生项已形成信封
// 到了之后，按信封上的键回读那一份运输收费发生项，译成结算的发起命令交给 BUY 触发编排（ADR-0208）。它只翻译不判断：
// 要不要发起、费用项目与主要范围从哪来，都在编排里。
package transportfulfillment

import (
	"context"
	"errors"
	"fmt"

	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	// ErrChargeOccurrenceNotVisible 表示按信封所指的键还读不回那一份发生项，或登记册自己答不出。发生项与信封在提供方
	// 同一事务落库，同库同进程下读不回只剩可见性滞后，重投会改变结果。生产装配把它登进 WithUndecidedSentinels。
	ErrChargeOccurrenceNotVisible = errors.New(
		"settlement accounting transportfulfillment adapter: charge occurrence is not yet visible")
	// ErrBuyEvaluationTriggerUndecided 表示触发编排停在自己的未决上（触发册、协议范围读口、请求册、铸造口或交接口）。
	// 同样是续办，同样进 WithUndecidedSentinels；停在哪一口留在错误正文里。
	ErrBuyEvaluationTriggerUndecided = errors.New(
		"settlement accounting transportfulfillment adapter: triggering the buy evaluation is undecided")
	// ErrChargeOccurrenceInconsistent 表示按键取回的发生项指着另一个键。提供方仓储不变量已破，不是等谁；禁止进
	// WithUndecidedSentinels（ADR-0029）。
	ErrChargeOccurrenceInconsistent = errors.New(
		"settlement accounting transportfulfillment adapter: charge occurrence disagrees with its key")
	// ErrUntranslatableAnswer 语义与其他消费方适配器的同名哨兵一致：某一侧交出了词汇表之外的内容，或本包译出的命令被
	// 编排拒收。重投同一份不会变。
	ErrUntranslatableAnswer = errors.New("settlement accounting transportfulfillment adapter: untranslatable answer")
	// ErrUnexpectedBuyEvaluationTriggerOutcome 表示编排交回了封闭集合以外的结果。静默入账等于替编排作判断。
	ErrUnexpectedBuyEvaluationTriggerOutcome = errors.New(
		"settlement accounting transportfulfillment adapter: unexpected buy evaluation trigger outcome")
)

// ChargeOccurrenceSource 按键整条取回发生项，由运输履约的 *tfpostgres.ChargeOccurrences 满足；只声明读的这一口。
// 不用成员窄读口（ChargeOccurrenceMemberView）：那一口刻意不交协议与修订，而本路要的正是这两样。
type ChargeOccurrenceSource interface {
	FindByKey(ctx context.Context, key tfports.ChargeOccurrenceKey) (tfports.ChargeOccurrenceRecord, bool, error)
}

// BuyEvaluationTrigger 是触发编排的那一口。真实装配接 saapplication.TriggerBuyEvaluationOnOccurrenceFormedHandler。
type BuyEvaluationTrigger interface {
	Trigger(
		ctx context.Context,
		command saapplication.TriggerBuyEvaluationOnOccurrenceFormedCommand,
	) (saapplication.BuyEvaluationTriggerResult, error)
}

// RequestOnChargeOccurrenceFormedAdapter 是 sainbox.ChargeOccurrenceFormedConsumer 的真实处理方。
type RequestOnChargeOccurrenceFormedAdapter struct {
	occurrences ChargeOccurrenceSource
	trigger     BuyEvaluationTrigger
}

func NewRequestOnChargeOccurrenceFormedAdapter(
	occurrences ChargeOccurrenceSource,
	trigger BuyEvaluationTrigger,
) (*RequestOnChargeOccurrenceFormedAdapter, error) {
	if occurrences == nil {
		return nil, fmt.Errorf("settlement accounting transportfulfillment adapter: charge occurrence source is nil")
	}
	if trigger == nil {
		return nil, fmt.Errorf("settlement accounting transportfulfillment adapter: buy evaluation trigger is nil")
	}
	return &RequestOnChargeOccurrenceFormedAdapter{occurrences: occurrences, trigger: trigger}, nil
}

var _ sainbox.FormedChargeOccurrenceHandler = (*RequestOnChargeOccurrenceFormedAdapter)(nil)

// HandleFormedChargeOccurrence 回读 → 译成发起命令 → 交编排 → 把结果落成消费两格。
func (adapter *RequestOnChargeOccurrenceFormedAdapter) HandleFormedChargeOccurrence(
	ctx context.Context,
	formed sainbox.FormedChargeOccurrence,
) error {
	key, err := chargeOccurrenceKeyOf(formed)
	if err != nil {
		return err
	}
	record, found, err := adapter.occurrences.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrChargeOccurrenceNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: occurrence %q version %q", ErrChargeOccurrenceNotVisible, formed.Occurrence, formed.Version)
	}
	occurrence := record.Occurrence
	if record.Key != key || occurrence.TenantID() != key.TenantID || occurrence.Occurrence() != key.Occurrence ||
		occurrence.Validity() != key.Validity {
		return fmt.Errorf("%w: occurrence %q version %q", ErrChargeOccurrenceInconsistent, formed.Occurrence, formed.Version)
	}

	command, err := triggerCommandOf(occurrence)
	if err != nil {
		return err
	}
	result, err := adapter.trigger.Trigger(ctx, command)
	if err != nil {
		return err
	}
	return triggerConsumption(result)
}

// triggerCommandOf 把发生项译成发起命令。四件引用与协议原样取自发生项：协议是发生项形成时固定下来的来源事实
// （ADR-0208 决定二），原因用运输履约的原词，结算触发册按同一个词登记。修订与否只交一格，判不判归编排。
func triggerCommandOf(occurrence tfdomain.TransportChargeOccurrence) (saapplication.TriggerBuyEvaluationOnOccurrenceFormedCommand, error) {
	none := saapplication.TriggerBuyEvaluationOnOccurrenceFormedCommand{}
	tenant, err := sadomain.NewTenantID(occurrence.TenantID().String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	id, err := sadomain.NewChargeOccurrenceID(occurrence.Occurrence().String())
	if err != nil {
		return none, fmt.Errorf("%w: occurrence: %v", ErrUntranslatableAnswer, err)
	}
	reason, err := sadomain.NewOccurrenceReasonReference(occurrence.Reason().String())
	if err != nil {
		return none, fmt.Errorf("%w: occurrence reason: %v", ErrUntranslatableAnswer, err)
	}
	version, err := sadomain.NewOccurrenceVersion(occurrence.Validity().String())
	if err != nil {
		return none, fmt.Errorf("%w: occurrence version: %v", ErrUntranslatableAnswer, err)
	}
	reference, err := sadomain.NewTransportChargeOccurrence(id, reason, version, occurrence.OccurredAt())
	if err != nil {
		return none, fmt.Errorf("%w: occurrence reference: %v", ErrUntranslatableAnswer, err)
	}
	agreement, err := sadomain.NewSupplierAgreementReference(occurrence.Agreement().String())
	if err != nil {
		return none, fmt.Errorf("%w: supplier agreement: %v", ErrUntranslatableAnswer, err)
	}
	_, revised := occurrence.Corrects()
	return saapplication.TriggerBuyEvaluationOnOccurrenceFormedCommand{
		TenantID:   tenant,
		Occurrence: reference,
		Agreement:  agreement,
		Revised:    revised,
	}, nil
}

// triggerConsumption 把编排结果落成消费两格，按「重投会不会改变结果」分：已发起、未配置、修订版不发起都是终局答案，
// 入账不重投（未配置等租户登记，补登之后这一份不回补）；未决重投；未受理在这里是本包译出的命令被拒，属适配器缺陷，
// 响亮上抛不入账。
func triggerConsumption(result saapplication.BuyEvaluationTriggerResult) error {
	switch result.Outcome() {
	case saapplication.BuyEvaluationTriggerFired,
		saapplication.BuyEvaluationTriggerUnconfigured,
		saapplication.BuyEvaluationTriggerOccurrenceRevised:
		return nil
	case saapplication.BuyEvaluationTriggerUndecided:
		return fmt.Errorf("%w: %s", ErrBuyEvaluationTriggerUndecided, result.UndecidedReason())
	case saapplication.BuyEvaluationTriggerNotAccepted:
		return fmt.Errorf("%w: the command built from the charge occurrence was refused", ErrUntranslatableAnswer)
	default:
		return fmt.Errorf("%w: %q", ErrUnexpectedBuyEvaluationTriggerOutcome, result.Outcome())
	}
}

// chargeOccurrenceKeyOf 把信封三维译回登记册的键。
func chargeOccurrenceKeyOf(formed sainbox.FormedChargeOccurrence) (tfports.ChargeOccurrenceKey, error) {
	none := tfports.ChargeOccurrenceKey{}
	tenant, err := tfdomain.NewTenantID(formed.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	occurrence, err := tfdomain.NewChargeOccurrenceReference(formed.Occurrence)
	if err != nil {
		return none, fmt.Errorf("%w: occurrence: %v", ErrUntranslatableAnswer, err)
	}
	version, err := tfdomain.NewOccurrenceValidityVersion(formed.Version)
	if err != nil {
		return none, fmt.Errorf("%w: occurrence version: %v", ErrUntranslatableAnswer, err)
	}
	return tfports.ChargeOccurrenceKey{TenantID: tenant, Occurrence: occurrence, Validity: version}, nil
}
