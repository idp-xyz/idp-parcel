package parcelpricing_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelpricing"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestTheSellGateIgnoresABuyEvaluation(t *testing.T) {
	gate, err := adapter.NewFormOnSellEvaluationRecordedAdapter(&sellView{err: fmt.Errorf("%w: BUY", adapter.ErrNotASellEvaluation)})
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if err := gate.HandleRecordedSellEvaluation(context.Background(), sainbox.RecordedBuyEvaluation{
		TenantID: "tenant-1", EvaluationID: "eval-1",
	}); err != nil {
		t.Fatalf("BUY 评价被 SELL 消费门当成了错误：%v", err)
	}
}

func TestTheSellGateNamesAMissingFeeItem(t *testing.T) {
	evaluation, err := domain.NewSellEvaluationReference("eval-1")
	if err != nil {
		t.Fatalf("evaluation: %v", err)
	}
	gate, err := adapter.NewFormOnSellEvaluationRecordedAdapter(&sellView{
		adoption: ports.SellEvaluationAdoption{
			Evaluation: evaluation,
			Outcome:    ports.SellEvaluationCompleted,
		},
	})
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	err = gate.HandleRecordedSellEvaluation(context.Background(), sainbox.RecordedBuyEvaluation{
		TenantID: "tenant-1", EvaluationID: "eval-1",
	})
	if !errors.Is(err, adapter.ErrSellChargeReferencesUnrecorded) {
		t.Fatalf("err = %v", err)
	}
}

type sellView struct {
	adoption ports.SellEvaluationAdoption
	err      error
}

func (view *sellView) LoadSellEvaluation(
	context.Context,
	domain.TenantID,
	domain.SellEvaluationReference,
) (ports.SellEvaluationAdoption, bool, error) {
	if view.err != nil {
		return ports.SellEvaluationAdoption{}, false, view.err
	}
	return view.adoption, true, nil
}
