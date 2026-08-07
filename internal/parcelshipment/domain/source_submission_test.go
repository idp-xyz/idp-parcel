package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func TestClassifySourceSubmission(t *testing.T) {
	existing := sourceFingerprint(t, "tenant-1", "customer-1", "api", "request-1", "digest-1")
	tests := []struct {
		name string
		got  domain.SourceSubmissionFingerprint
		want domain.SourceClassification
	}{
		{"same scoped key and digest is replay", sourceFingerprint(t, "tenant-1", "customer-1", "api", "request-1", "digest-1"), domain.SourceReplay},
		{"same scoped key and different digest is conflict", sourceFingerprint(t, "tenant-1", "customer-1", "api", "request-1", "digest-2"), domain.SourceConflict},
		{"same external key in another tenant is distinct", sourceFingerprint(t, "tenant-2", "customer-1", "api", "request-1", "digest-1"), domain.SourceDistinct},
		{"same external key in another account is distinct", sourceFingerprint(t, "tenant-1", "customer-2", "api", "request-1", "digest-1"), domain.SourceDistinct},
		{"same external key from another source is distinct", sourceFingerprint(t, "tenant-1", "customer-1", "file", "request-1", "digest-1"), domain.SourceDistinct},
		{"another source request key is distinct", sourceFingerprint(t, "tenant-1", "customer-1", "api", "request-2", "digest-1"), domain.SourceDistinct},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := domain.ClassifySourceSubmission(existing, test.got)
			if err != nil {
				t.Fatalf("classify: %v", err)
			}
			if got != test.want {
				t.Fatalf("classification = %v, want %v", got, test.want)
			}
		})
	}
}

func TestClassifySourceSubmissionTreatsDifferentTimesAsReplay(t *testing.T) {
	identity := sourceIdentity(t, "tenant-1", "customer-1", "api", "request-1")
	digest := mustValue(t, domain.NewPayloadDigest, "digest-1")

	existing, err := domain.NewSourceSubmissionFingerprint(
		identity,
		digest,
		time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 5, 10, 0, 1, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new existing source fingerprint: %v", err)
	}
	incoming, err := domain.NewSourceSubmissionFingerprint(
		identity,
		digest,
		time.Date(2026, 8, 5, 11, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 5, 11, 0, 1, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new incoming source fingerprint: %v", err)
	}

	classification, err := domain.ClassifySourceSubmission(existing, incoming)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if classification != domain.SourceReplay {
		t.Fatalf("classification = %v, want replay", classification)
	}
}

func TestSourceSubmissionDigestSeparatesRequestEffectiveAtPresence(t *testing.T) {
	identity := sourceIdentity(t, "tenant-1", "customer-1", "api", "request-effective-1")
	occurredAt := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	receivedAt := time.Date(2026, 8, 5, 10, 0, 1, 0, time.UTC)

	missingEffectiveAt := mustValue(t, domain.NewPayloadDigest, "payload-effective-at:missing")
	explicitEffectiveAt := mustValue(t, domain.NewPayloadDigest, "payload-effective-at:2026-08-07T00:00:00Z")
	missing, err := domain.NewSourceSubmissionFingerprint(identity, missingEffectiveAt, occurredAt, receivedAt)
	if err != nil {
		t.Fatalf("missing effective-at fingerprint: %v", err)
	}
	explicit, err := domain.NewSourceSubmissionFingerprint(identity, explicitEffectiveAt, occurredAt, receivedAt)
	if err != nil {
		t.Fatalf("explicit effective-at fingerprint: %v", err)
	}
	classification, err := domain.ClassifySourceSubmission(missing, explicit)
	if err != nil {
		t.Fatalf("classify effective-at change: %v", err)
	}
	if classification != domain.SourceConflict {
		t.Fatalf("classification = %v, want conflict", classification)
	}

	retried, err := domain.NewSourceSubmissionFingerprint(
		identity,
		explicitEffectiveAt,
		occurredAt.Add(2*time.Hour),
		receivedAt.Add(2*time.Hour),
	)
	if err != nil {
		t.Fatalf("retried effective-at fingerprint: %v", err)
	}
	classification, err = domain.ClassifySourceSubmission(explicit, retried)
	if err != nil {
		t.Fatalf("classify retried effective-at request: %v", err)
	}
	if classification != domain.SourceReplay {
		t.Fatalf("classification = %v, want replay", classification)
	}
}

func TestClassifySourceSubmissionRejectsInvalidFingerprints(t *testing.T) {
	valid := sourceFingerprint(t, "tenant-1", "customer-1", "api", "request-1", "digest-1")
	tests := []struct {
		name     string
		existing domain.SourceSubmissionFingerprint
		incoming domain.SourceSubmissionFingerprint
	}{
		{"invalid existing fingerprint", domain.SourceSubmissionFingerprint{}, valid},
		{"invalid incoming fingerprint", valid, domain.SourceSubmissionFingerprint{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			classification, err := domain.ClassifySourceSubmission(test.existing, test.incoming)
			if !errors.Is(err, domain.ErrInvalidSourceSubmission) {
				t.Fatalf("error = %v, want invalid source submission", err)
			}
			if classification != 0 {
				t.Fatalf("classification = %v, want no classification", classification)
			}
		})
	}
}

func TestSourceIdentityRejectsMissingScopeComponents(t *testing.T) {
	tenantID := mustValue(t, domain.NewTenantID, "tenant-1")
	customerAccountID := mustValue(t, domain.NewCustomerAccountID, "customer-1")
	source := mustValue(t, domain.NewSource, "api")
	requestKey := mustValue(t, domain.NewSourceRequestKey, "request-1")

	tests := []struct {
		name              string
		tenantID          domain.TenantID
		customerAccountID domain.CustomerAccountID
		source            domain.Source
		requestKey        domain.SourceRequestKey
	}{
		{"missing tenant", domain.TenantID{}, customerAccountID, source, requestKey},
		{"missing customer account", tenantID, domain.CustomerAccountID{}, source, requestKey},
		{"missing source", tenantID, customerAccountID, domain.Source{}, requestKey},
		{"missing request key", tenantID, customerAccountID, source, domain.SourceRequestKey{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := domain.NewSourceIdentity(test.tenantID, test.customerAccountID, test.source, test.requestKey)
			if !errors.Is(err, domain.ErrInvalidSourceSubmission) {
				t.Fatalf("error = %v, want invalid source submission", err)
			}
		})
	}
}

func TestSourceSubmissionKeepsTimesIndependent(t *testing.T) {
	identity := sourceIdentity(t, "tenant-1", "customer-1", "api", "request-1")
	digest := mustValue(t, domain.NewPayloadDigest, "digest-1")
	occurredAt := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	receivedAt := occurredAt.Add(-time.Hour)

	submission, err := domain.NewSourceSubmissionFingerprint(identity, digest, occurredAt, receivedAt)
	if err != nil {
		t.Fatalf("new source submission: %v", err)
	}
	if submission.OccurredAt() != occurredAt || submission.ReceivedAt() != receivedAt {
		t.Fatalf("times changed: occurred=%v received=%v", submission.OccurredAt(), submission.ReceivedAt())
	}
}

func TestSourceSubmissionRejectsMissingInputs(t *testing.T) {
	validIdentity := sourceIdentity(t, "tenant-1", "customer-1", "api", "request-1")
	validDigest := mustValue(t, domain.NewPayloadDigest, "digest-1")
	validTime := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		identity   domain.SourceIdentity
		digest     domain.PayloadDigest
		occurredAt time.Time
		receivedAt time.Time
	}{
		{"missing identity", domain.SourceIdentity{}, validDigest, validTime, validTime},
		{"missing digest", validIdentity, domain.PayloadDigest{}, validTime, validTime},
		{"missing source time", validIdentity, validDigest, time.Time{}, validTime},
		{"missing receive time", validIdentity, validDigest, validTime, time.Time{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := domain.NewSourceSubmissionFingerprint(test.identity, test.digest, test.occurredAt, test.receivedAt)
			if !errors.Is(err, domain.ErrInvalidSourceSubmission) {
				t.Fatalf("error = %v, want invalid source submission", err)
			}
		})
	}
}
