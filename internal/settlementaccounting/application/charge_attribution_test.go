package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

type chargeAttributionViewDouble struct {
	registration domain.ChargeAttributionRegistration
	found        bool
	err          error
}

func (double chargeAttributionViewDouble) LoadChargeAttribution(
	context.Context,
	domain.TenantID,
	domain.FeeItemReference,
) (domain.ChargeAttributionRegistration, bool, error) {
	return double.registration, double.found, double.err
}

func TestJudgeChargeAttributionStaysUnconfiguredWhenTheFeeItemIsNotRegistered(t *testing.T) {
	handler := newAttributionHandler(t, chargeAttributionViewDouble{})
	result, err := handler.Judge(t.Context(), attributionCommand())
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.ChargeAttributionUnconfigured {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	if _, judged := result.Date(); judged {
		t.Fatal("没登记仍交出了归属日")
	}
}

func TestJudgeChargeAttributionUsesTheRegisteredForm(t *testing.T) {
	handler := newAttributionHandler(t, chargeAttributionViewDouble{
		registration: mustAttributionRegistration(t),
		found:        true,
	})
	result, err := handler.Judge(t.Context(), attributionCommand())
	if err != nil {
		t.Fatal(err)
	}
	date, judged := result.Date()
	if result.Outcome() != application.ChargeAttributionJudged || !judged || date.String() != "2026-09-30" {
		t.Fatalf("outcome = %s date = %s judged=%v", result.Outcome(), date, judged)
	}
}

func TestJudgeChargeAttributionStopsWhenTheViewFails(t *testing.T) {
	handler := newAttributionHandler(t, chargeAttributionViewDouble{err: errors.New("attribution view down")})
	result, err := handler.Judge(t.Context(), attributionCommand())
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.ChargeAttributionViewUnavailable {
		t.Fatalf("outcome = %s", result.Outcome())
	}
}

func newAttributionHandler(t *testing.T, view ports.ChargeAttributionView) *application.JudgeChargeAttributionHandler {
	t.Helper()
	handler, err := application.NewJudgeChargeAttributionHandler(view)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func attributionCommand() application.JudgeChargeAttributionCommand {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		panic(err)
	}
	return application.JudgeChargeAttributionCommand{
		TenantID:       tenant,
		FeeItem:        "BASE",
		SourceOccurred: time.Date(2026, 9, 30, 17, 59, 0, 0, time.UTC),
	}
}

func mustAttributionRegistration(t *testing.T) domain.ChargeAttributionRegistration {
	t.Helper()
	fee, err := domain.NewFeeItemReference("BASE")
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domain.NewChargeAttributionRegistration(fee, domain.ChargeAttributionSourceOccurred, "UTC", 18*60)
	if err != nil {
		t.Fatal(err)
	}
	return registration
}
