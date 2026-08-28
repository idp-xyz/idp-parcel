package application

import (
	"context"
	"strings"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
)

// 回汇面的两个用例：形成批次、交出汇付主张。

// FormBatchCommand 携带一次回汇批次形成。
type FormBatchCommand struct {
	Tenant domain.TenantID
	Batch  domain.RemittanceBatch
}

// FormBatch 形成一个回汇批次。目标分户账必须已开立——批次冻结的是一本具体的账的
// 归集范围，账都没有就无从归集。
//
// 已在册时读回逐字段比：键、币种、归集截点任何一格不同都是冲突。**状态不参与比对**
// ——已交出主张的批次重放一次形成命令，比的是它形成时那几格，答案是重放而不是把它
// 退回`已归集`。
func (handler *Handler) FormBatch(ctx context.Context, command FormBatchCommand) (Outcome, error) {
	if blank(command.Tenant) || strings.TrimSpace(command.Batch.ID().String()) == "" {
		return OutcomeNotAccepted, nil
	}

	_, opened, err := handler.deps.Subledger.LoadSubledger(ctx, command.Tenant, command.Batch.Ledger())
	if err != nil {
		return OutcomeUndecided, nil
	}
	if !opened {
		return OutcomeBasisMissing, nil
	}

	saved, err := handler.deps.Remittances.FormBatch(ctx, command.Tenant, command.Batch)
	if err != nil {
		return OutcomeUndecided, nil
	}
	if saved == ports.SaveRegistered {
		return OutcomeRegistered, nil
	}

	existing, found, err := handler.deps.Remittance.LoadBatch(ctx, command.Tenant, command.Batch.ID())
	if err != nil {
		return OutcomeUndecided, nil
	}
	if !found ||
		existing.Ledger() != command.Batch.Ledger() ||
		!existing.CollectedThrough().Equal(command.Batch.CollectedThrough()) ||
		!existing.FormedAt().Equal(command.Batch.FormedAt()) {
		return OutcomeContentConflict, nil
	}
	return OutcomeExisting, nil
}

// HandOverBatchCommand 携带一次汇付主张交出。
type HandOverBatchCommand struct {
	Tenant domain.TenantID
	Batch  domain.RemittanceBatchID
}

// HandOverBatch 把批次推进到`已交出汇付主张`。
//
// 交出的是**主张**不是付款：付款执行、清算与到账由外部支付与银行系统拥有。这个用例
// 之后账上不会多出任何资金位置变化，成员那些汇付记账在交出之前就已经落好了。
func (handler *Handler) HandOverBatch(ctx context.Context, command HandOverBatchCommand) (Outcome, error) {
	if blank(command.Tenant) || strings.TrimSpace(command.Batch.String()) == "" {
		return OutcomeNotAccepted, nil
	}

	outcome, err := handler.deps.Remittances.HandOverBatchForPayment(ctx, command.Tenant, command.Batch)
	if err != nil {
		return OutcomeUndecided, nil
	}
	switch outcome {
	case ports.HandedOver:
		return OutcomeHandedOver, nil
	case ports.AlreadyHandedOver:
		return OutcomeAlreadyHandedOver, nil
	case ports.HandOverTargetMissing:
		return OutcomeBasisMissing, nil
	default:
		return OutcomeUndecided, nil
	}
}
