package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

type settlementMomentViewDouble struct {
	found  bool
	err    error
	moment domain.SettlementMoment
}

func (double settlementMomentViewDouble) LoadSettlementMoment(
	_ context.Context,
	_ domain.TenantID,
	moment domain.SettlementMoment,
) (bool, error) {
	if double.moment != 0 && moment != double.moment {
		return false, nil
	}
	return double.found, double.err
}

func TestAdmitSettlementMomentStaysUnconfiguredWhenNotRegistered(t *testing.T) {
	handler := newMomentHandler(t, settlementMomentViewDouble{})
	outcome, err := handler.Admit(t.Context(), momentTenant(t), domain.SettlementMomentConfirm)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != application.SettlementMomentUnconfigured {
		t.Fatalf("outcome = %s", outcome)
	}
}

func TestAdmitSettlementMomentAdmitsTheRegisteredMomentOnly(t *testing.T) {
	handler := newMomentHandler(t, settlementMomentViewDouble{found: true, moment: domain.SettlementMomentConfirm})
	outcome, err := handler.Admit(t.Context(), momentTenant(t), domain.SettlementMomentConfirm)
	if err != nil || outcome != application.SettlementMomentAdmitted {
		t.Fatalf("confirm outcome = %s err=%v", outcome, err)
	}
	outcome, err = handler.Admit(t.Context(), momentTenant(t), domain.SettlementMomentCutOff)
	if err != nil || outcome != application.SettlementMomentUnconfigured {
		t.Fatalf("cutoff outcome = %s err=%v", outcome, err)
	}
}

func TestAdmitSettlementMomentStopsWhenTheViewFails(t *testing.T) {
	handler := newMomentHandler(t, settlementMomentViewDouble{err: errors.New("moment view down")})
	outcome, err := handler.Admit(t.Context(), momentTenant(t), domain.SettlementMomentCutOff)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != application.SettlementMomentViewUnavailable {
		t.Fatalf("outcome = %s", outcome)
	}
}

func newMomentHandler(t *testing.T, view ports.SettlementMomentView) *application.AdmitSettlementMomentHandler {
	t.Helper()
	handler, err := application.NewAdmitSettlementMomentHandler(view)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func momentTenant(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	return tenant
}
