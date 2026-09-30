package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func TestAttributionDateUsesSourceOccurredBeforeCutoff(t *testing.T) {
	registration := attributionRegistration(t, domain.ChargeAttributionSourceOccurred, "UTC", 18*60)
	// 2026-09-30 17:59 UTC，截单 18:00，归属日仍是当天。
	date, err := registration.Judge(
		time.Date(2026, 9, 30, 17, 59, 0, 0, time.UTC),
		time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if date.String() != "2026-09-30" {
		t.Fatalf("归属日 = %s", date)
	}
}

func TestAttributionDateRollsForwardAtCutoff(t *testing.T) {
	registration := attributionRegistration(t, domain.ChargeAttributionSourceOccurred, "UTC", 18*60)
	date, err := registration.Judge(
		time.Date(2026, 9, 30, 18, 0, 0, 0, time.UTC),
		time.Time{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if date.String() != "2026-10-01" {
		t.Fatalf("归属日 = %s", date)
	}
}

func TestAttributionDateUsesChargeConfirmedWhenThatFormIsRegistered(t *testing.T) {
	registration := attributionRegistration(t, domain.ChargeAttributionChargeConfirmed, "UTC", 18*60)
	date, err := registration.Judge(
		time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if date.String() != "2026-10-02" {
		t.Fatalf("归属日 = %s", date)
	}
}

func TestAttributionDateRejectsAMissingInstantAndAnUnknownForm(t *testing.T) {
	registration := attributionRegistration(t, domain.ChargeAttributionChargeConfirmed, "UTC", 18*60)
	if _, err := registration.Judge(time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC), time.Time{}); !errors.Is(err, domain.ErrInvalidChargeAttribution) {
		t.Fatalf("缺确认时点 err = %v", err)
	}
	if _, err := domain.ChargeAttributionFormFromName("DELIVERED"); !errors.Is(err, domain.ErrInvalidChargeAttribution) {
		t.Fatalf("签收被收成归属日形态：%v", err)
	}
	fee, err := domain.NewFeeItemReference("BASE")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.NewChargeAttributionRegistration(fee, domain.ChargeAttributionSourceOccurred, "Not/AZone", 18*60); !errors.Is(err, domain.ErrInvalidChargeAttribution) {
		t.Fatalf("坏时区 err = %v", err)
	}
}

func attributionRegistration(t *testing.T, form domain.ChargeAttributionForm, zone string, cutoff int) domain.ChargeAttributionRegistration {
	t.Helper()
	fee, err := domain.NewFeeItemReference("BASE")
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domain.NewChargeAttributionRegistration(fee, form, zone, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	return registration
}
