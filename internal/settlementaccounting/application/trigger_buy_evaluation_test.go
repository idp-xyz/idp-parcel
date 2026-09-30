package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

type buyEvaluationTriggerViewDouble struct {
	moment domain.BuyEvaluationTriggerMoment
	found  bool
	err    error
	reason string
}

func (double buyEvaluationTriggerViewDouble) LoadBuyEvaluationTrigger(
	_ context.Context,
	_ domain.TenantID,
	reason domain.OccurrenceReasonReference,
) (domain.BuyEvaluationTriggerMoment, bool, error) {
	if double.reason != "" && reason.String() != double.reason {
		return domain.BuyEvaluationTriggerMomentInvalid, false, nil
	}
	return double.moment, double.found, double.err
}

func TestTriggerBuyEvaluationStaysUnconfiguredWhenTheReasonIsNotRegistered(t *testing.T) {
	fixture := newEvaluationRequestFixture(t)
	handler := newTriggerHandler(t, buyEvaluationTriggerViewDouble{}, fixture.handler)
	result, err := handler.Trigger(t.Context(), requestBuyEvaluationCommand(t, "SYN-FEE"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.BuyEvaluationTriggerUnconfigured {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	if _, fired := result.Request(); fired {
		t.Fatal("没登记仍发起了请求")
	}
	if fixture.identity.next != 0 {
		t.Fatalf("没登记却铸了 %d 个请求 ID", fixture.identity.next)
	}
}

func TestTriggerBuyEvaluationRequestsWhenTheReasonIsRegistered(t *testing.T) {
	fixture := newEvaluationRequestFixture(t)
	handler := newTriggerHandler(t, buyEvaluationTriggerViewDouble{
		moment: domain.BuyEvaluationTriggerOnOccurrenceFormed,
		found:  true,
		reason: "BOOKING",
	}, fixture.handler)
	result, err := handler.Trigger(t.Context(), requestBuyEvaluationCommand(t, "SYN-FEE"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.BuyEvaluationTriggerFired {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	requested, fired := result.Request()
	if !fired || requested.Outcome() != application.EvaluationRequested {
		t.Fatalf("请求 outcome = %s fired=%v", requested.Outcome(), fired)
	}
}

func TestTriggerBuyEvaluationDoesNotRequestADifferentReason(t *testing.T) {
	fixture := newEvaluationRequestFixture(t)
	handler := newTriggerHandler(t, buyEvaluationTriggerViewDouble{
		moment: domain.BuyEvaluationTriggerOnOccurrenceFormed,
		found:  true,
		reason: "BOOKING",
	}, fixture.handler)
	command := requestBuyEvaluationCommand(t, "SYN-FEE")
	occurrence, err := domain.NewTransportChargeOccurrence(
		command.Occurrence.ID(),
		mustReason(t, "CANCEL"),
		command.Occurrence.Version(),
		command.Occurrence.OccurredAt(),
	)
	if err != nil {
		t.Fatal(err)
	}
	command.Occurrence = occurrence
	result, err := handler.Trigger(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.BuyEvaluationTriggerUnconfigured {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	if fixture.identity.next != 0 {
		t.Fatalf("另一原因却铸了 %d 个请求 ID", fixture.identity.next)
	}
}

func TestTriggerBuyEvaluationStopsWhenTheTriggerViewFails(t *testing.T) {
	fixture := newEvaluationRequestFixture(t)
	handler := newTriggerHandler(t, buyEvaluationTriggerViewDouble{err: errors.New("trigger view down")}, fixture.handler)
	result, err := handler.Trigger(t.Context(), requestBuyEvaluationCommand(t, "SYN-FEE"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.BuyEvaluationTriggerViewUnavailable {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	if fixture.identity.next != 0 {
		t.Fatal("读口故障仍发起了请求")
	}
}

func newTriggerHandler(
	t *testing.T,
	view ports.BuyEvaluationTriggerView,
	requests *application.RequestBuyEvaluationHandler,
) *application.TriggerBuyEvaluationHandler {
	t.Helper()
	handler, err := application.NewTriggerBuyEvaluationHandler(view, requests)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func mustReason(t *testing.T, value string) domain.OccurrenceReasonReference {
	t.Helper()
	reason, err := domain.NewOccurrenceReasonReference(value)
	if err != nil {
		t.Fatal(err)
	}
	return reason
}
