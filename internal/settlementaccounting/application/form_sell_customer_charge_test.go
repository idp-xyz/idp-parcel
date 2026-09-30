package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestAMissingSellTriggerDoesNotInitiate(t *testing.T) {
	handler, err := application.NewTriggerSellEvaluationHandler(&sellTriggerView{configured: false})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	reason, err := domain.NewOccurrenceReasonReference("booking")
	if err != nil {
		t.Fatalf("reason: %v", err)
	}
	result, err := handler.Trigger(context.Background(), application.TriggerSellEvaluationCommand{
		TenantID: tenant, Reason: reason,
	})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if result.Outcome() != application.SellEvaluationTriggerUnconfigured {
		t.Fatalf("outcome = %s", result.Outcome())
	}
}

func TestARegisteredSellTriggerFiresWithoutABuyRequest(t *testing.T) {
	handler, err := application.NewTriggerSellEvaluationHandler(&sellTriggerView{configured: true})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	tenant, _ := domain.NewTenantID("tenant-1")
	reason, _ := domain.NewOccurrenceReasonReference("booking")
	result, err := handler.Trigger(context.Background(), application.TriggerSellEvaluationCommand{
		TenantID: tenant, Reason: reason,
	})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if result.Outcome() != application.SellEvaluationTriggerFired {
		t.Fatalf("outcome = %s", result.Outcome())
	}
}

type sellTriggerView struct{ configured bool }

func (view *sellTriggerView) LoadSellEvaluationTrigger(
	context.Context,
	domain.TenantID,
	domain.OccurrenceReasonReference,
) (domain.SellEvaluationTriggerMoment, bool, error) {
	if !view.configured {
		return domain.SellEvaluationTriggerMomentInvalid, false, nil
	}
	return domain.SellEvaluationTriggerOnOccurrenceFormed, true, nil
}

func TestSellChargeUsesTheEvaluationAmount(t *testing.T) {
	currency, err := domain.NewCurrencyCode("CNY")
	if err != nil {
		t.Fatalf("currency: %v", err)
	}
	evaluation, err := domain.NewSellEvaluationReference("sell-eval-1")
	if err != nil {
		t.Fatalf("evaluation: %v", err)
	}
	store := &estimatedChargeStore{}
	handler, err := application.NewFormSellCustomerChargeHandler(&sellEvaluationView{
		adoption: ports.SellEvaluationAdoption{
			Evaluation:         evaluation,
			Outcome:            ports.SellEvaluationCompleted,
			OriginalCurrency:   currency,
			OriginalMinor:      12000,
			SettlementCurrency: currency,
			SettlementMinor:    12000,
		},
	}, store)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	tenant, _ := domain.NewTenantID("tenant-1")
	result, err := handler.Handle(context.Background(), application.FormSellCustomerChargeCommand{
		TenantID:   tenant,
		Charge:     "charge-1",
		FeeItem:    "BASE_FREIGHT",
		Evaluation: "sell-eval-1",
		FormedAt:   time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if result.Outcome() != application.SellChargeFormed {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	charge, ok := result.Charge()
	if !ok {
		t.Fatal("没有费用")
	}
	_, minor := charge.SettlementAmount()
	if minor != 12000 {
		t.Fatalf("金额 = %d，想要评价上的 12000", minor)
	}
	if len(store.saved) != 1 {
		t.Fatalf("saved = %d", len(store.saved))
	}
}

type sellEvaluationView struct {
	adoption ports.SellEvaluationAdoption
}

func (view *sellEvaluationView) LoadSellEvaluation(
	context.Context,
	domain.TenantID,
	domain.SellEvaluationReference,
) (ports.SellEvaluationAdoption, bool, error) {
	return view.adoption, true, nil
}

type estimatedChargeStore struct {
	saved []domain.CustomerCharge
}

func (store *estimatedChargeStore) SaveEstimated(
	context.Context,
	domain.TenantID,
	domain.CustomerCharge,
) (ports.ChargeSaveOutcome, error) {
	store.saved = append(store.saved, domain.CustomerCharge{})
	return ports.ChargeSaved, nil
}
