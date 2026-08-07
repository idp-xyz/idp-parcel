package partycommercial

import (
	"errors"
	"testing"
	"time"
)

// This file is a test-only contract for S01-W01. There is intentionally no
// production party-commercial model, repository, or publication endpoint yet.
type syntheticCommercialObjectKind string

const (
	syntheticCustomerAccountKind syntheticCommercialObjectKind = "CUSTOMER_ACCOUNT"
	syntheticLegalEntityKind     syntheticCommercialObjectKind = "LEGAL_ENTITY"
	syntheticServiceProductKind  syntheticCommercialObjectKind = "SERVICE_PRODUCT"
	syntheticContractKind        syntheticCommercialObjectKind = "CUSTOMER_CONTRACT"
	syntheticRulePackageKind     syntheticCommercialObjectKind = "ACCEPTANCE_RULE_PACKAGE"
	syntheticPrepaidPolicyKind   syntheticCommercialObjectKind = "PREPAID_POLICY"
	syntheticTermsPolicyKind     syntheticCommercialObjectKind = "TERMS_POLICY"
)

type syntheticCommercialStatus string

const (
	syntheticDraft     syntheticCommercialStatus = "DRAFT"
	syntheticApproved  syntheticCommercialStatus = "APPROVED"
	syntheticPublished syntheticCommercialStatus = "PUBLISHED"
)

const syntheticEvidenceLevel = "S"

var (
	errSyntheticCommercialInvalid    = errors.New("synthetic commercial version is invalid")
	errSyntheticCommercialTransition = errors.New("synthetic commercial lifecycle transition is invalid")
	errSyntheticCommercialConflict   = errors.New("synthetic commercial version conflicts with published content")
)

const (
	syntheticPublicationCreated  = "CREATED"
	syntheticPublicationReplay   = "REPLAY"
	syntheticPublicationConflict = "CONFLICT"
)

type syntheticEffectivePeriod struct {
	startsAt time.Time
	endsAt   time.Time
}

func newSyntheticEffectivePeriod(startsAt, endsAt time.Time) (syntheticEffectivePeriod, error) {
	if startsAt.IsZero() || (!endsAt.IsZero() && !endsAt.After(startsAt)) {
		return syntheticEffectivePeriod{}, errSyntheticCommercialInvalid
	}
	return syntheticEffectivePeriod{startsAt: startsAt.UTC(), endsAt: endsAt.UTC()}, nil
}

type syntheticCommercialVersion struct {
	kind            syntheticCommercialObjectKind
	objectID        string
	version         string
	fixtureVersion  string
	scopeReference  string
	contentDigest   string
	currentRevision string
	period          syntheticEffectivePeriod
	evidence        string
	status          syntheticCommercialStatus
	approvalRef     string
	approvedAt      time.Time
	publicationRef  string
	publishedAt     time.Time
}

func newSyntheticCommercialDraft(
	kind syntheticCommercialObjectKind,
	objectID string,
	version string,
	fixtureVersion string,
	scopeReference string,
	contentDigest string,
	currentRevision string,
	period syntheticEffectivePeriod,
	evidence string,
) (syntheticCommercialVersion, error) {
	if kind == "" || objectID == "" || version == "" || fixtureVersion == "" ||
		scopeReference == "" || contentDigest == "" || currentRevision == "" ||
		evidence != syntheticEvidenceLevel || period.startsAt.IsZero() ||
		(!period.endsAt.IsZero() && !period.endsAt.After(period.startsAt)) {
		return syntheticCommercialVersion{}, errSyntheticCommercialInvalid
	}
	return syntheticCommercialVersion{
		kind:            kind,
		objectID:        objectID,
		version:         version,
		fixtureVersion:  fixtureVersion,
		scopeReference:  scopeReference,
		contentDigest:   contentDigest,
		currentRevision: currentRevision,
		period:          period,
		evidence:        evidence,
		status:          syntheticDraft,
	}, nil
}

func (version syntheticCommercialVersion) approve(approvalRef string, approvedAt time.Time) (syntheticCommercialVersion, error) {
	if version.status != syntheticDraft || approvalRef == "" || approvedAt.IsZero() {
		return syntheticCommercialVersion{}, errSyntheticCommercialTransition
	}
	version.status = syntheticApproved
	version.approvalRef = approvalRef
	version.approvedAt = approvedAt.UTC()
	return version, nil
}

func (version syntheticCommercialVersion) publish(publicationRef string, publishedAt time.Time) (syntheticCommercialVersion, error) {
	if version.status != syntheticApproved || publicationRef == "" || publishedAt.IsZero() || publishedAt.Before(version.approvedAt) {
		return syntheticCommercialVersion{}, errSyntheticCommercialTransition
	}
	version.status = syntheticPublished
	version.publicationRef = publicationRef
	version.publishedAt = publishedAt.UTC()
	return version, nil
}

type syntheticCommercialKey struct {
	kind     syntheticCommercialObjectKind
	objectID string
	version  string
}

type syntheticPublicationRegistry struct {
	versions map[syntheticCommercialKey]syntheticCommercialVersion
}

func newSyntheticPublicationRegistry() syntheticPublicationRegistry {
	return syntheticPublicationRegistry{versions: make(map[syntheticCommercialKey]syntheticCommercialVersion)}
}

func (registry *syntheticPublicationRegistry) publish(version syntheticCommercialVersion) (string, error) {
	if version.status != syntheticPublished || version.evidence != syntheticEvidenceLevel {
		return "", errSyntheticCommercialInvalid
	}
	key := syntheticCommercialKey{kind: version.kind, objectID: version.objectID, version: version.version}
	if existing, exists := registry.versions[key]; exists {
		if sameSyntheticPublishedContent(existing, version) {
			return syntheticPublicationReplay, nil
		}
		return syntheticPublicationConflict, errSyntheticCommercialConflict
	}
	registry.versions[key] = version
	return syntheticPublicationCreated, nil
}

func sameSyntheticPublishedContent(left, right syntheticCommercialVersion) bool {
	return left.kind == right.kind &&
		left.objectID == right.objectID &&
		left.version == right.version &&
		left.fixtureVersion == right.fixtureVersion &&
		left.scopeReference == right.scopeReference &&
		left.contentDigest == right.contentDigest &&
		left.currentRevision == right.currentRevision &&
		left.period.startsAt.Equal(right.period.startsAt) &&
		left.period.endsAt.Equal(right.period.endsAt) &&
		left.evidence == right.evidence &&
		left.status == right.status
}

func (registry syntheticPublicationRegistry) get(kind syntheticCommercialObjectKind, objectID, version string) (syntheticCommercialVersion, bool) {
	value, ok := registry.versions[syntheticCommercialKey{kind: kind, objectID: objectID, version: version}]
	return value, ok
}

func syntheticCommercialVersionFor(t *testing.T, kind syntheticCommercialObjectKind, objectID, version, scope string) syntheticCommercialVersion {
	t.Helper()
	period, err := newSyntheticEffectivePeriod(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("effective period: %v", err)
	}
	draft, err := newSyntheticCommercialDraft(
		kind,
		objectID,
		version,
		"SYN-COM-FIXTURE-v1",
		scope,
		"sha256:"+objectID+"-"+version,
		"rev-1",
		period,
		syntheticEvidenceLevel,
	)
	if err != nil {
		t.Fatalf("commercial draft: %v", err)
	}
	approved, err := draft.approve("SYN-APPROVAL-"+objectID+"-"+version, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("commercial approval: %v", err)
	}
	published, err := approved.publish("SYN-PUBLICATION-"+objectID+"-"+version, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("commercial publication: %v", err)
	}
	return published
}

func TestSyntheticCommercialPublicationPublishesIndependentObjects(t *testing.T) {
	registry := newSyntheticPublicationRegistry()
	versions := []syntheticCommercialVersion{
		syntheticCommercialVersionFor(t, syntheticCustomerAccountKind, "SYN-CUST-01", "v1", "SYN-SCOPE-CUSTOMER-01"),
		syntheticCommercialVersionFor(t, syntheticLegalEntityKind, "SYN-LEGAL-01", "v1", "SYN-SCOPE-LEGAL-01"),
		syntheticCommercialVersionFor(t, syntheticServiceProductKind, "SYN-PRODUCT-01", "v1", "SYN-SCOPE-PRODUCT-01"),
		syntheticCommercialVersionFor(t, syntheticContractKind, "SYN-CONTRACT-01", "v1", "SYN-SCOPE-CONTRACT-01"),
		syntheticCommercialVersionFor(t, syntheticRulePackageKind, "SYN-RULES-01", "v1", "SYN-SCOPE-RULES-01"),
		syntheticCommercialVersionFor(t, syntheticPrepaidPolicyKind, "SYN-PREPAID-01", "v1", "SYN-SCOPE-PREPAID-01"),
		syntheticCommercialVersionFor(t, syntheticTermsPolicyKind, "SYN-TERMS-01", "v1", "SYN-SCOPE-TERMS-01"),
	}

	for _, version := range versions {
		outcome, err := registry.publish(version)
		if err != nil || outcome != syntheticPublicationCreated {
			t.Fatalf("publish %s/%s = %s, %v", version.kind, version.objectID, outcome, err)
		}
		stored, ok := registry.get(version.kind, version.objectID, version.version)
		if !ok || stored != version {
			t.Fatalf("stored version = %#v, want %#v", stored, version)
		}
		if stored.status != syntheticPublished || stored.evidence != syntheticEvidenceLevel ||
			stored.approvalRef == "" || stored.publicationRef == "" || stored.fixtureVersion == "" ||
			stored.scopeReference == "" || stored.currentRevision == "" {
			t.Fatalf("publication traceability incomplete: %#v", stored)
		}
	}
	if len(registry.versions) != len(versions) {
		t.Fatalf("published object count = %d, want %d", len(registry.versions), len(versions))
	}
}

func TestSyntheticCommercialPublicationRequiresApprovalAndPreservesHistory(t *testing.T) {
	period, err := newSyntheticEffectivePeriod(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	draft, err := newSyntheticCommercialDraft(syntheticServiceProductKind, "SYN-PRODUCT-02", "v1", "SYN-COM-FIXTURE-v1", "SYN-SCOPE-02", "sha256:product-2-v1", "rev-1", period, syntheticEvidenceLevel)
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if _, err := draft.publish("SYN-PUBLICATION-INVALID", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)); !errors.Is(err, errSyntheticCommercialTransition) {
		t.Fatalf("publish without approval error = %v", err)
	}
	approved, err := draft.approve("SYN-APPROVAL-02", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if draft.status != syntheticDraft || draft.approvalRef != "" {
		t.Fatalf("approval mutated draft history: %#v", draft)
	}
	published, err := approved.publish("SYN-PUBLICATION-02", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if published.status != syntheticPublished || published.approvalRef != "SYN-APPROVAL-02" || published.publicationRef != "SYN-PUBLICATION-02" {
		t.Fatalf("published trace = %#v", published)
	}
	if published.approvedAt.IsZero() || published.publishedAt.IsZero() || !published.period.startsAt.Equal(period.startsAt) {
		t.Fatalf("published timing/period = %#v", published)
	}
}

func TestSyntheticCommercialPublicationDoesNotOverwritePublishedVersion(t *testing.T) {
	registry := newSyntheticPublicationRegistry()
	original := syntheticCommercialVersionFor(t, syntheticServiceProductKind, "SYN-PRODUCT-03", "v1", "SYN-SCOPE-03")
	if outcome, err := registry.publish(original); err != nil || outcome != syntheticPublicationCreated {
		t.Fatalf("publish original = %s, %v", outcome, err)
	}

	changed := original
	changed.contentDigest = "sha256:product-3-v1-changed"
	if outcome, err := registry.publish(changed); outcome != syntheticPublicationConflict || !errors.Is(err, errSyntheticCommercialConflict) {
		t.Fatalf("publish changed = %s, %v", outcome, err)
	}
	stored, ok := registry.get(original.kind, original.objectID, original.version)
	if !ok || stored.contentDigest != original.contentDigest || stored.publicationRef != original.publicationRef {
		t.Fatalf("published history was overwritten: %#v", stored)
	}

	next := syntheticCommercialVersionFor(t, syntheticServiceProductKind, "SYN-PRODUCT-03", "v2", "SYN-SCOPE-03")
	if outcome, err := registry.publish(next); err != nil || outcome != syntheticPublicationCreated {
		t.Fatalf("publish new version = %s, %v", outcome, err)
	}
	if len(registry.versions) != 2 {
		t.Fatalf("version count = %d, want 2", len(registry.versions))
	}
}

func TestSyntheticCommercialPublicationReplayKeepsOriginalPublicationMetadata(t *testing.T) {
	registry := newSyntheticPublicationRegistry()
	original := syntheticCommercialVersionFor(t, syntheticContractKind, "SYN-CONTRACT-REPLAY", "v1", "SYN-SCOPE-REPLAY")
	if outcome, err := registry.publish(original); err != nil || outcome != syntheticPublicationCreated {
		t.Fatalf("publish original = %s, %v", outcome, err)
	}
	retry := original
	retry.approvalRef = "SYN-APPROVAL-RETRY"
	retry.publicationRef = "SYN-PUBLICATION-RETRY"
	outcome, err := registry.publish(retry)
	if err != nil || outcome != syntheticPublicationReplay {
		t.Fatalf("publish retry = %s, %v", outcome, err)
	}
	stored, _ := registry.get(original.kind, original.objectID, original.version)
	if stored.approvalRef != original.approvalRef || stored.publicationRef != original.publicationRef {
		t.Fatalf("replay changed original metadata: %#v", stored)
	}
}

func TestSyntheticCommercialPublicationRejectsNonSyntheticEvidence(t *testing.T) {
	period, err := newSyntheticEffectivePeriod(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	for _, evidence := range []string{"R", "P", ""} {
		if _, err := newSyntheticCommercialDraft(syntheticContractKind, "SYN-EVIDENCE", "v1", "SYN-COM-FIXTURE-v1", "SYN-SCOPE-EVIDENCE", "sha256:evidence", "rev-1", period, evidence); !errors.Is(err, errSyntheticCommercialInvalid) {
			t.Fatalf("evidence %q error = %v, want invalid synthetic evidence", evidence, err)
		}
	}
}
