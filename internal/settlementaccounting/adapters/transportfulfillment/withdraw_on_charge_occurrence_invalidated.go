package transportfulfillment

import (
	"context"
	"errors"
	"fmt"

	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var (
	// ErrSupplierCostWithdrawalUndecided 表示撤下编排停在依赖上（预期成本库或失效记录答不出）。续办，进 WithUndecidedSentinels。
	ErrSupplierCostWithdrawalUndecided = errors.New(
		"settlement accounting transportfulfillment adapter: withdrawing the supplier cost is undecided")
	// ErrUnexpectedSupplierCostWithdrawalOutcome 表示编排交回了封闭集合以外的结果。
	ErrUnexpectedSupplierCostWithdrawalOutcome = errors.New(
		"settlement accounting transportfulfillment adapter: unexpected supplier cost withdrawal outcome")
)

// SupplierCostWithdrawal 是撤下编排那一口。真实装配接 saapplication.WithdrawSupplierExpectedCostHandler。
type SupplierCostWithdrawal interface {
	Withdraw(ctx context.Context, command saapplication.WithdrawSupplierExpectedCostCommand) (saapplication.SupplierCostWithdrawalResult, error)
}

// WithdrawOnChargeOccurrenceInvalidatedAdapter 是 sainbox.ChargeOccurrenceInvalidatedConsumer 的真实处理方：按信封上的键回读那一版发生项，
// 是失效版本就译成撤下命令（ADR-0232 决定七），不是就入账——首版与替代版本归发起 BUY 评价那一路。它只翻译不判断。
type WithdrawOnChargeOccurrenceInvalidatedAdapter struct {
	occurrences ChargeOccurrenceSource
	withdrawal  SupplierCostWithdrawal
}

func NewWithdrawOnChargeOccurrenceInvalidatedAdapter(
	occurrences ChargeOccurrenceSource,
	withdrawal SupplierCostWithdrawal,
) (*WithdrawOnChargeOccurrenceInvalidatedAdapter, error) {
	if occurrences == nil {
		return nil, fmt.Errorf("settlement accounting transportfulfillment adapter: charge occurrence source is nil")
	}
	if withdrawal == nil {
		return nil, fmt.Errorf("settlement accounting transportfulfillment adapter: supplier cost withdrawal is nil")
	}
	return &WithdrawOnChargeOccurrenceInvalidatedAdapter{occurrences: occurrences, withdrawal: withdrawal}, nil
}

var _ sainbox.InvalidatedChargeOccurrenceHandler = (*WithdrawOnChargeOccurrenceInvalidatedAdapter)(nil)

// HandleInvalidatedChargeOccurrence 回读 → 不是失效版本就入账 → 译成撤下命令 → 交编排 → 落成消费两格。回读与键对不上、词汇表外的格与
// 发起那一路同一套哨兵。
func (adapter *WithdrawOnChargeOccurrenceInvalidatedAdapter) HandleInvalidatedChargeOccurrence(
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
	if !occurrence.Invalidated() {
		return nil
	}
	corrects, revised := occurrence.Corrects()
	if !revised {
		return fmt.Errorf("%w: invalidated occurrence %q version %q corrects nothing", ErrChargeOccurrenceInconsistent, formed.Occurrence, formed.Version)
	}
	reference, err := triggerCommandOf(occurrence)
	if err != nil {
		return err
	}
	invalidates, err := sadomain.NewOccurrenceVersion(corrects.String())
	if err != nil {
		return fmt.Errorf("%w: invalidated version: %v", ErrUntranslatableAnswer, err)
	}
	result, err := adapter.withdrawal.Withdraw(ctx, saapplication.WithdrawSupplierExpectedCostCommand{
		TenantID:    reference.TenantID,
		Occurrence:  reference.Occurrence,
		Invalidates: invalidates,
	})
	if err != nil {
		return err
	}
	switch result.Outcome() {
	case saapplication.SupplierCostWithdrawn, saapplication.SupplierCostWithdrawalExisting, saapplication.SupplierCostWithdrawalNothingFormed:
		return nil
	case saapplication.SupplierCostWithdrawalUndecided:
		return fmt.Errorf("%w: %v", ErrSupplierCostWithdrawalUndecided, result.UndecidedCause())
	case saapplication.SupplierCostWithdrawalNotAccepted:
		return fmt.Errorf("%w: the command built from the invalidated occurrence was refused", ErrUntranslatableAnswer)
	default:
		return fmt.Errorf("%w: %q", ErrUnexpectedSupplierCostWithdrawalOutcome, result.Outcome())
	}
}
