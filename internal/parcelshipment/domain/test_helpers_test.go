package domain_test

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

func sourceFingerprint(
	t *testing.T,
	tenant, customer, source, key, digest string,
) domain.SourceSubmissionFingerprint {
	t.Helper()
	value, err := domain.NewSourceSubmissionFingerprint(
		sourceIdentity(t, tenant, customer, source, key),
		mustValue(t, domain.NewPayloadDigest, digest),
		time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 5, 10, 0, 1, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new source fingerprint: %v", err)
	}
	return value
}

func stringValues(values []domain.DeclaredParcelID) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.String()
	}
	return result
}
