package transportfulfillment_test

import (
	"context"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/transportfulfillment"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var deliveredAt = time.Date(2026, 8, 10, 15, 0, 0, 0, time.UTC)

type finalRuleDouble struct {
	judgment psports.FinalRuleJudgment
}

func (double *finalRuleDouble) JudgeFinalOutcome(
	_ context.Context, _ psdomain.SourceIdentity, _ psdomain.ResponsibilityOutcome,
) (psports.FinalRuleJudgment, bool, error) {
	return double.judgment, true, nil
}

type finalStoreDouble struct {
	byKey map[psports.FinalAdoptionKey]psports.FinalOutcomeRecord
}

func (double *finalStoreDouble) FindByKey(
	_ context.Context, key psports.FinalAdoptionKey,
) (psports.FinalOutcomeRecord, bool, error) {
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *finalStoreDouble) FindCurrentFinal(
	_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psports.FinalOutcomeRecord, bool, error) {
	for _, record := range double.byKey {
		if record.Key.TenantID == tenant && record.Key.Parcel == parcel && record.Finalized {
			return record, true, nil
		}
	}
	return psports.FinalOutcomeRecord{}, false, nil
}

func (double *finalStoreDouble) Save(
	_ context.Context, record psports.FinalOutcomeRecord,
) (psports.FinalOutcomeSaveOutcome, error) {
	double.byKey[record.Key] = record
	return psports.FinalOutcomeSaved, nil
}

type finalDownstreamDouble struct{}

func (finalDownstreamDouble) HandOffFinalOutcome(_ context.Context, _ psports.FinalOutcomeHandoffIntent) error {
	return nil
}

type finalIdentityDouble struct{ next int }

func (double *finalIdentityDouble) NextFinalOutcomeVersionID(
	_ context.Context,
) (psdomain.FinalOutcomeVersionID, error) {
	double.next++
	return psdomain.NewFinalOutcomeVersionID("final-" + string(rune('0'+double.next)))
}

type cancellationViewDouble struct{}

func (cancellationViewDouble) FindCancellation(
	_ context.Context, _ psdomain.TenantID, _ psdomain.DeclaredParcelID,
) (psdomain.ParcelCancellation, bool, error) {
	return psdomain.ParcelCancellation{}, false, nil
}

// effectiveDelivery 真经 TF 领域链构造：尝试→妥投结果→有效交付（假对象钉不住翻译
// 取值的来处）。
func effectiveDelivery(t *testing.T) tfdomain.EffectiveDelivery {
	t.Helper()
	attempt, err := tfdomain.FormFulfillmentAttempt(tfdomain.FulfillmentAttemptSpec{
		TenantID:    value(t, tfdomain.NewTenantID, "tenant-1"),
		Attempt:     value(t, tfdomain.NewAttemptReference, "attempt-9"),
		Task:        value(t, tfdomain.NewDispatchTaskReference, "delivery-task-1"),
		ExecutedBy:  value(t, tfdomain.NewExecutingPartyReference, "courier-1"),
		Place:       value(t, tfdomain.NewAttemptPlaceReference, "recipient-door"),
		PlannedFrom: deliveredAt.Add(-2 * time.Hour),
		PlannedTo:   deliveredAt.Add(2 * time.Hour),
		ArrivedAt:   deliveredAt.Add(-10 * time.Minute),
		Objects:     []tfdomain.CarriedObjectReference{value(t, tfdomain.NewCarriedObjectReference, "parcel-1")},
		Evidence:    value(t, tfdomain.NewAttemptEvidenceReference, "GPS-TRACE/9"),
	})
	if err != nil {
		t.Fatalf("form fulfillment attempt: %v", err)
	}
	result, err := tfdomain.FormDeliveryAttemptResult(
		attempt,
		value(t, tfdomain.NewCarriedObjectReference, "parcel-1"),
		tfdomain.ObjectDelivered,
		tfdomain.AttemptResultBasisReference{},
		deliveredAt,
	)
	if err != nil {
		t.Fatalf("form delivery attempt result: %v", err)
	}
	delivery, err := tfdomain.FormEffectiveDelivery(attempt, result, tfdomain.EffectiveDeliverySpec{
		Method:    value(t, tfdomain.NewDeliveryMethodReference, "HAND_TO_RECIPIENT"),
		Recipient: value(t, tfdomain.NewReceivingPartyReference, "recipient-1"),
		Proof:     value(t, tfdomain.NewDeliveryProofReference, "POD-3"),
		Version:   value(t, tfdomain.NewDeliveryResultVersion, "delivery-result/v1"),
	})
	if err != nil {
		t.Fatalf("form effective delivery: %v", err)
	}
	return delivery
}

// Covers: `AT-PS-053` 经适配器端到端——真 TF 有效交付译成责任结果走完真实终局编排：
// 终局形成、生效恒等于实际交付时间、POD 与交付判断双引用逐维译到位；「有效交付不在
// 所有产品中自动等于终局」由 PS 侧规则视图回答（此处规则放行），适配器只翻译不判断。
func TestAnEffectiveDeliveryFlowsThroughToAFinalOutcome(t *testing.T) {
	requests := &requestStoreDouble{records: map[psdomain.SourceIdentity]psdomain.ShipmentRequest{
		identity(t): acceptedRequest(t),
	}}
	handler := psapplication.NewFormParcelFinalHandler(psapplication.FormParcelFinalDeps{
		Requests: requests,
		Rules: &finalRuleDouble{judgment: psports.FinalRuleJudgment{
			Satisfied:   true,
			Kind:        value(t, psdomain.NewFinalKindReference, "NETWORK_SERVICE_DELIVERED"),
			RuleVersion: value(t, psdomain.NewFinalRuleVersionReference, "final-rules/v1"),
		}},
		Finals:        &finalStoreDouble{byKey: map[psports.FinalAdoptionKey]psports.FinalOutcomeRecord{}},
		Cancellations: cancellationViewDouble{},
		Identities:    &finalIdentityDouble{},
		Downstream:    finalDownstreamDouble{},
		Clock:         fixedClock{at: deliveredAt.Add(time.Minute)},
	})
	subject := adapter.NewDeliveryOutcomeAdapter(handler)

	result, err := subject.AdoptFromEffectiveDelivery(context.Background(), effectiveDelivery(t), adapter.TargetShipment{
		Identity:          identity(t),
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
	})
	if err != nil {
		t.Fatalf("adopt from effective delivery: %v", err)
	}

	if result.Outcome() != psapplication.ParcelFinalFormed {
		t.Fatalf("outcome = %q, want FINAL_FORMED", result.Outcome())
	}
	record, _ := result.Record()
	if record.Key.Kind != psdomain.EffectiveDeliveryOutcome {
		t.Fatalf("kind = %q", record.Key.Kind)
	}
	if !record.Final.EffectiveAt().Equal(deliveredAt) {
		t.Fatalf("effective at = %s, want the delivery time", record.Final.EffectiveAt())
	}
	if record.Final.Source().Execution().String() != "POD/POD-3" {
		t.Fatalf("execution = %q; POD 必须带来源前缀译过来", record.Final.Source().Execution())
	}
	if record.Final.Source().Decision().String() != "DELIVERY-JUDGMENT/parcel-1/attempt-9" {
		t.Fatalf("decision = %q", record.Final.Source().Decision())
	}
}
