package transportfulfillment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/transportfulfillment"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证撤下预期成本那一路的消费侧翻译（ADR-0232 决定七）：只对失效版本动手，命令带失效版本自己的引用与它让哪一版失效；编排各格
// 落成消费两格。夹具同发起那一路。

type withdrawalDouble struct {
	commands []saapplication.WithdrawSupplierExpectedCostCommand
	result   saapplication.SupplierCostWithdrawalResult
	err      error
}

func (double *withdrawalDouble) Withdraw(_ context.Context, command saapplication.WithdrawSupplierExpectedCostCommand) (saapplication.SupplierCostWithdrawalResult, error) {
	double.commands = append(double.commands, command)
	return double.result, double.err
}

func TestOnlyAnInvalidatedVersionIsHandedToTheWithdrawal(t *testing.T) {
	original := trunkOccurrence(t)
	invalidated := must[tfdomain.TransportChargeOccurrence](t)(original.ReviseValidity(
		tfdomain.OccurrenceInvalidated,
		must[tfdomain.OccurrenceValidityVersion](t)(tfdomain.NewOccurrenceValidityVersion("V2")),
		must[tfdomain.OccurrenceBasisReference](t)(tfdomain.NewOccurrenceBasisReference("SYN-EVIDENCE")),
		occurredAt.Add(time.Hour)))

	untouched := &withdrawalDouble{}
	handler := must[*adapter.WithdrawOnChargeOccurrenceInvalidatedAdapter](t)(adapter.NewWithdrawOnChargeOccurrenceInvalidatedAdapter(sourceWith(original), untouched))
	if err := handler.HandleInvalidatedChargeOccurrence(t.Context(), formedOf(original)); err != nil || len(untouched.commands) != 0 {
		t.Fatalf("首版 = %v，交了 %d 次", err, len(untouched.commands))
	}

	withdrawal := &withdrawalDouble{}
	handler = must[*adapter.WithdrawOnChargeOccurrenceInvalidatedAdapter](t)(adapter.NewWithdrawOnChargeOccurrenceInvalidatedAdapter(sourceWith(invalidated), withdrawal))
	err := handler.HandleInvalidatedChargeOccurrence(t.Context(), formedOf(invalidated))
	if !errors.Is(err, adapter.ErrUnexpectedSupplierCostWithdrawalOutcome) || len(withdrawal.commands) != 1 {
		t.Fatalf("失效版 err = %v，交了 %d 次", err, len(withdrawal.commands))
	}
	command := withdrawal.commands[0]
	if command.Occurrence.Version().String() != "V2" || command.Invalidates.String() != "V1" ||
		command.Occurrence.ID().String() != original.Occurrence().String() {
		t.Fatalf("撤下命令 = %+v", command)
	}
}
