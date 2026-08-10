package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

type stringValue interface {
	String() string
}

func mustValue[T stringValue](t *testing.T, constructor func(string) (T, error), value string) T {
	t.Helper()
	got, err := constructor(value)
	if err != nil {
		t.Fatalf("construct %q: %v", value, err)
	}
	return got
}

func sourceIdentity(t *testing.T, tenant, customer, source, key string) domain.SourceIdentity {
	t.Helper()
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, tenant),
		mustValue(t, domain.NewCustomerAccountID, customer),
		mustValue(t, domain.NewSource, source),
		mustValue(t, domain.NewSourceRequestKey, key),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	return identity
}

func validity(t *testing.T) domain.OwnershipValidityInterval {
	t.Helper()
	interval, err := domain.NewOwnershipValidityInterval(
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new validity interval: %v", err)
	}
	return interval
}
