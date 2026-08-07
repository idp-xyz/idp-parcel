package partycommercial

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"testing"
	"time"
)

// This file is deliberately test-only. It is the S01-W02 contract for
// two-stage commercial resolution; it does not authorize a production port,
// repository, transaction, outbox, or external financial call.

type syntheticResolutionStatus string

const (
	syntheticResolutionUnique         syntheticResolutionStatus = "UNIQUE_RESOLVED"
	syntheticResolutionNoBasis        syntheticResolutionStatus = "NO_APPLICABLE_BASIS"
	syntheticResolutionConflict       syntheticResolutionStatus = "APPLICABILITY_CONFLICT"
	syntheticResolutionPending        syntheticResolutionStatus = "RESOLUTION_PENDING"
	syntheticResolutionInputNotAccept syntheticResolutionStatus = "INPUT_NOT_ACCEPTED"
	syntheticResolutionStale          syntheticResolutionStatus = "STALE"
)

const (
	syntheticPurposeAcceptanceControl = "ACCEPTANCE_CONTROL"
	syntheticPurposePricing           = "PRICING"
	syntheticBasisSlotSettlement      = "SETTLEMENT_CONTROL_POLICY"
	syntheticBasisSlotPricing         = "COMMERCIAL_PRICING_BASIS"
)

const (
	syntheticPendingAnchorPolicy      = "ANCHOR_POLICY_UNAVAILABLE"
	syntheticPendingDependency        = "DEPENDENCY_UNAVAILABLE"
	syntheticPendingAuthority         = "AUTHORITY_UNAVAILABLE"
	syntheticPendingStale             = "CURRENT_RESOLUTION_CHANGED"
	syntheticInputMissing             = "MINIMUM_SCOPE_NOT_ESTABLISHED"
	syntheticNoApplicableScope        = "NO_AUTHORITY_CANDIDATE"
	syntheticApplicabilityOverlap     = "OVERLAPPING_APPLICABLE_CANDIDATES"
	syntheticCommercialFixtureVersion = "SYN-COM-FIXTURE-v1"
)

type syntheticCommercialSelectionAnchor struct {
	at            time.Time
	policyVersion string
}

func newSyntheticCommercialSelectionAnchor(at time.Time, policyVersion string) syntheticCommercialSelectionAnchor {
	return syntheticCommercialSelectionAnchor{at: at.UTC(), policyVersion: policyVersion}
}

func (anchor syntheticCommercialSelectionAnchor) value() string {
	if anchor.at.IsZero() {
		return ""
	}
	return anchor.at.UTC().Format(time.RFC3339Nano)
}

type syntheticCommercialResolutionQuery struct {
	tenantID              string
	customerAccountID     string
	legalEntityCandidate  string
	serviceOrControlScope string
	purpose               string
	priceDirection        string
	basisSlot             string
	selectionAnchor       syntheticCommercialSelectionAnchor
	requestEffectiveAt    *time.Time
}

func (query syntheticCommercialResolutionQuery) valid() bool {
	if query.tenantID == "" || query.customerAccountID == "" ||
		query.legalEntityCandidate == "" || query.serviceOrControlScope == "" ||
		query.purpose == "" || query.basisSlot == "" ||
		query.selectionAnchor.at.IsZero() || query.selectionAnchor.policyVersion == "" {
		return false
	}
	switch query.purpose {
	case syntheticPurposeAcceptanceControl:
		if query.basisSlot != syntheticBasisSlotSettlement {
			return false
		}
	case syntheticPurposePricing:
		if query.priceDirection == "" || query.basisSlot != syntheticBasisSlotPricing {
			return false
		}
	default:
		return false
	}
	return true
}

func (query syntheticCommercialResolutionQuery) fingerprint() string {
	// requestEffectiveAt is intentionally excluded. The independent,
	// versioned selection anchor is the only time used to choose a baseline.
	return strings.Join([]string{
		query.tenantID,
		query.customerAccountID,
		query.legalEntityCandidate,
		query.serviceOrControlScope,
		query.purpose,
		query.priceDirection,
		query.basisSlot,
		query.selectionAnchor.value(),
		query.selectionAnchor.policyVersion,
	}, "\x00")
}

type syntheticCommercialReference struct {
	kind     syntheticCommercialObjectKind
	objectID string
	version  string
}

func (reference syntheticCommercialReference) key() syntheticCommercialKey {
	return syntheticCommercialKey{
		kind:     reference.kind,
		objectID: reference.objectID,
		version:  reference.version,
	}
}

type syntheticAsOfPolicyDeclaration struct {
	rulePackage     syntheticCommercialReference
	judgmentType    string
	semantic        string
	strategyVersion string
}

type syntheticCommercialCandidate struct {
	candidateID           string
	baselineID            string
	tenantID              string
	customerAccountID     string
	legalEntityCandidate  string
	serviceOrControlScope string
	purpose               string
	priceDirection        string
	basisSlot             string
	policyKind            syntheticCommercialObjectKind
	references            []syntheticCommercialReference
}

type syntheticCandidateResolutionState string

const (
	syntheticCandidateApplicable    syntheticCandidateResolutionState = "APPLICABLE"
	syntheticCandidateNotApplicable syntheticCandidateResolutionState = "NOT_APPLICABLE"
	syntheticCandidatePending       syntheticCandidateResolutionState = "PENDING"
	syntheticCandidateConflict      syntheticCandidateResolutionState = "CONFLICT"
)

type syntheticAdoptedCommercialObject struct {
	kind            syntheticCommercialObjectKind
	objectID        string
	version         string
	effectivePeriod syntheticEffectivePeriod
	currentRevision string
	scopeReference  string
	fixtureVersion  string
}

func (object syntheticAdoptedCommercialObject) reference() syntheticCommercialReference {
	return syntheticCommercialReference{kind: object.kind, objectID: object.objectID, version: object.version}
}

type syntheticConflictCandidateSnapshot struct {
	candidateID     string
	policyKind      syntheticCommercialObjectKind
	policyReference syntheticCommercialReference
	effectivePeriod syntheticEffectivePeriod
	currentRevision string
	fixtureVersion  string
}

type syntheticCommercialResolution struct {
	status              syntheticResolutionStatus
	resolutionID        string
	query               syntheticCommercialResolutionQuery
	selectionAnchor     syntheticCommercialSelectionAnchor
	baselineID          string
	fixtureVersion      string
	adopted             []syntheticAdoptedCommercialObject
	selectedRulePackage syntheticCommercialReference
	asOfPolicies        []syntheticAsOfPolicyDeclaration
	conflictCandidates  []syntheticConflictCandidateSnapshot
	missingDependencies []syntheticCommercialReference
	judgedAt            time.Time
	evidence            string
	evidenceIndex       string
	authorityRevision   string
	continuationRef     string
	reason              string
}

func (resolution syntheticCommercialResolution) semanticDigest() string {
	parts := []string{
		string(resolution.status),
		resolution.query.fingerprint(),
		resolution.selectionAnchor.value(),
		resolution.selectionAnchor.policyVersion,
		resolution.fixtureVersion,
		resolution.evidence,
		resolution.evidenceIndex,
		resolution.authorityRevision,
		resolution.reason,
	}
	for _, object := range resolution.adopted {
		parts = append(parts,
			string(object.kind),
			object.objectID,
			object.version,
			object.effectivePeriod.startsAt.UTC().Format(time.RFC3339Nano),
			object.effectivePeriod.endsAt.UTC().Format(time.RFC3339Nano),
			object.currentRevision,
			object.scopeReference,
			object.fixtureVersion,
		)
	}
	for _, policy := range resolution.asOfPolicies {
		parts = append(parts,
			string(policy.rulePackage.kind),
			policy.rulePackage.objectID,
			policy.rulePackage.version,
			policy.judgmentType,
			policy.semantic,
			policy.strategyVersion,
		)
	}
	return digestSyntheticResolution(parts...)
}

type syntheticAnchorPolicy struct {
	version string
	active  bool
}

type syntheticCommercialResolverFixture struct {
	registry            syntheticPublicationRegistry
	candidates          []syntheticCommercialCandidate
	rulePackagePolicies map[syntheticCommercialKey][]syntheticAsOfPolicyDeclaration
	anchorPolicies      map[string]syntheticAnchorPolicy
	authorityAvailable  bool
	authorityRevision   string
	fixtureVersion      string
	judgedAt            time.Time
}

func newSyntheticCommercialResolverFixture() syntheticCommercialResolverFixture {
	return syntheticCommercialResolverFixture{
		registry:            newSyntheticPublicationRegistry(),
		rulePackagePolicies: make(map[syntheticCommercialKey][]syntheticAsOfPolicyDeclaration),
		anchorPolicies: map[string]syntheticAnchorPolicy{
			"SYN-ANCHOR-POLICY-v1": {version: "SYN-ANCHOR-POLICY-v1", active: true},
		},
		authorityAvailable: true,
		authorityRevision:  "SYN-COMMERCIAL-VIEW-rev-1",
		fixtureVersion:     syntheticCommercialFixtureVersion,
		judgedAt:           time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	}
}

type syntheticCommercialBaselineFixture struct {
	baselineID  string
	references  []syntheticCommercialReference
	rulePackage syntheticCommercialReference
}

func (fixture *syntheticCommercialResolverFixture) publishBaseline(
	t *testing.T,
	baselineID string,
	period syntheticEffectivePeriod,
	revision string,
	publishRulePackage bool,
) syntheticCommercialBaselineFixture {
	t.Helper()
	objects := []struct {
		kind     syntheticCommercialObjectKind
		objectID string
	}{
		{kind: syntheticCustomerAccountKind, objectID: "SYN-CUST-01"},
		{kind: syntheticLegalEntityKind, objectID: "SYN-LEGAL-01"},
		{kind: syntheticServiceProductKind, objectID: baselineID + "-SERVICE-PRODUCT"},
		{kind: syntheticContractKind, objectID: baselineID + "-CUSTOMER-CONTRACT"},
		{kind: syntheticRulePackageKind, objectID: baselineID + "-ACCEPTANCE-RULE-PACKAGE"},
	}
	references := make([]syntheticCommercialReference, 0, len(objects))
	var rulePackage syntheticCommercialReference
	for _, object := range objects {
		version := syntheticCommercialVersionFor(
			t,
			object.kind,
			object.objectID,
			"v1",
			"SYN-COMMERCIAL-BASELINE-"+baselineID,
		)
		version.period = period
		version.currentRevision = revision
		reference := syntheticCommercialReference{kind: object.kind, objectID: object.objectID, version: version.version}
		references = append(references, reference)
		if object.kind == syntheticRulePackageKind {
			rulePackage = reference
			fixture.rulePackagePolicies[reference.key()] = nil
		}
		if object.kind != syntheticRulePackageKind || publishRulePackage {
			publishSyntheticResolutionVersion(t, &fixture.registry, version)
		}
	}
	return syntheticCommercialBaselineFixture{baselineID: baselineID, references: references, rulePackage: rulePackage}
}

func (fixture *syntheticCommercialResolverFixture) addPolicyCandidate(
	t *testing.T,
	candidateID string,
	baseline syntheticCommercialBaselineFixture,
	scope string,
	policyKind syntheticCommercialObjectKind,
	period syntheticEffectivePeriod,
	revision string,
) syntheticCommercialCandidate {
	t.Helper()
	policy := syntheticCommercialVersionFor(t, policyKind, candidateID+"-"+string(policyKind), "v1", scope)
	policy.period = period
	policy.currentRevision = revision
	publishSyntheticResolutionVersion(t, &fixture.registry, policy)

	references := append([]syntheticCommercialReference(nil), baseline.references...)
	references = append(references, syntheticCommercialReference{
		kind:     policy.kind,
		objectID: policy.objectID,
		version:  policy.version,
	})
	candidate := syntheticCommercialCandidate{
		candidateID:           candidateID,
		baselineID:            baseline.baselineID,
		tenantID:              "SYN-TENANT-01",
		customerAccountID:     "SYN-CUST-01",
		legalEntityCandidate:  "SYN-LEGAL-01",
		serviceOrControlScope: scope,
		purpose:               syntheticPurposeAcceptanceControl,
		basisSlot:             syntheticBasisSlotSettlement,
		policyKind:            policyKind,
		references:            references,
	}
	fixture.candidates = append(fixture.candidates, candidate)
	return candidate
}

func (fixture *syntheticCommercialResolverFixture) declareRulePackagePolicies(
	t *testing.T,
	baseline syntheticCommercialBaselineFixture,
	policies []syntheticAsOfPolicyDeclaration,
) {
	t.Helper()
	rulePackage, ok := fixture.registry.get(
		baseline.rulePackage.kind,
		baseline.rulePackage.objectID,
		baseline.rulePackage.version,
	)
	if !ok || rulePackage.status != syntheticPublished || rulePackage.fixtureVersion != fixture.fixtureVersion {
		t.Fatalf("rule package is not a published fixture: %#v", baseline.rulePackage)
	}
	declared := make([]syntheticAsOfPolicyDeclaration, 0, len(policies))
	seen := make(map[string]bool)
	for _, policy := range policies {
		if policy.rulePackage != baseline.rulePackage || policy.judgmentType == "" ||
			policy.semantic == "" || policy.strategyVersion == "" || seen[policy.judgmentType] {
			t.Fatalf("invalid rule package asOf policy: %#v", policy)
		}
		seen[policy.judgmentType] = true
		declared = append(declared, policy)
	}
	sort.Slice(declared, func(i, j int) bool { return declared[i].judgmentType < declared[j].judgmentType })
	fixture.rulePackagePolicies[baseline.rulePackage.key()] = declared
}

func publishSyntheticResolutionVersion(t *testing.T, registry *syntheticPublicationRegistry, version syntheticCommercialVersion) {
	t.Helper()
	outcome, err := registry.publish(version)
	if err != nil || outcome != syntheticPublicationCreated {
		t.Fatalf("publish resolution fixture %s/%s = %s, %v", version.kind, version.objectID, outcome, err)
	}
}

func syntheticResolutionQueryForScope(scope string) syntheticCommercialResolutionQuery {
	return syntheticCommercialResolutionQuery{
		tenantID:              "SYN-TENANT-01",
		customerAccountID:     "SYN-CUST-01",
		legalEntityCandidate:  "SYN-LEGAL-01",
		serviceOrControlScope: scope,
		purpose:               syntheticPurposeAcceptanceControl,
		basisSlot:             syntheticBasisSlotSettlement,
		selectionAnchor: newSyntheticCommercialSelectionAnchor(
			time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
			"SYN-ANCHOR-POLICY-v1",
		),
	}
}

func syntheticPeriodFor(start, end time.Time) syntheticEffectivePeriod {
	return syntheticEffectivePeriod{startsAt: start.UTC(), endsAt: end.UTC()}
}

func syntheticPeriodContains(period syntheticEffectivePeriod, at time.Time) bool {
	at = at.UTC()
	if at.Before(period.startsAt) {
		return false
	}
	return period.endsAt.IsZero() || at.Before(period.endsAt)
}

type syntheticCandidateCheck struct {
	candidate syntheticCommercialCandidate
	state     syntheticCandidateResolutionState
	objects   []syntheticAdoptedCommercialObject
	missing   []syntheticCommercialReference
}

func (fixture syntheticCommercialResolverFixture) checkCandidate(
	candidate syntheticCommercialCandidate,
	query syntheticCommercialResolutionQuery,
) syntheticCandidateCheck {
	check := syntheticCandidateCheck{candidate: candidate}
	seenKinds := make(map[syntheticCommercialObjectKind]bool)
	objects := make([]syntheticAdoptedCommercialObject, 0, len(candidate.references))
	missing := make([]syntheticCommercialReference, 0)
	notApplicable := false
	conflict := false

	for _, reference := range candidate.references {
		if seenKinds[reference.kind] {
			conflict = true
			continue
		}
		seenKinds[reference.kind] = true
		version, ok := fixture.registry.get(reference.kind, reference.objectID, reference.version)
		if !ok || version.status != syntheticPublished || version.evidence != syntheticEvidenceLevel ||
			version.fixtureVersion != fixture.fixtureVersion {
			missing = append(missing, reference)
			continue
		}
		if !syntheticPeriodContains(version.period, query.selectionAnchor.at) {
			notApplicable = true
			continue
		}
		objects = append(objects, syntheticAdoptedCommercialObject{
			kind:            version.kind,
			objectID:        version.objectID,
			version:         version.version,
			effectivePeriod: version.period,
			currentRevision: version.currentRevision,
			scopeReference:  version.scopeReference,
			fixtureVersion:  version.fixtureVersion,
		})
		if version.kind == candidate.policyKind && version.scopeReference != query.serviceOrControlScope {
			conflict = true
		}
	}

	requiredKinds := map[syntheticCommercialObjectKind]bool{
		syntheticCustomerAccountKind: true,
		syntheticLegalEntityKind:     true,
		syntheticServiceProductKind:  true,
		syntheticContractKind:        true,
		syntheticRulePackageKind:     true,
		candidate.policyKind:         true,
	}
	for kind := range requiredKinds {
		if !seenKinds[kind] {
			missing = append(missing, syntheticCommercialReference{kind: kind})
		}
	}
	if seenKinds[syntheticPrepaidPolicyKind] && seenKinds[syntheticTermsPolicyKind] {
		conflict = true
	}
	if candidate.basisSlot == syntheticBasisSlotSettlement &&
		candidate.policyKind != syntheticPrepaidPolicyKind &&
		candidate.policyKind != syntheticTermsPolicyKind {
		conflict = true
	}

	customerPresent, customerMismatch := syntheticObjectIdentityMismatch(objects, syntheticCustomerAccountKind, query.customerAccountID)
	legalPresent, legalMismatch := syntheticObjectIdentityMismatch(objects, syntheticLegalEntityKind, query.legalEntityCandidate)
	if (customerPresent && customerMismatch) || (legalPresent && legalMismatch) {
		conflict = true
	}
	rulePackageReference := syntheticRulePackageReference(candidate.references)
	declaredPolicies, declarationsKnown := fixture.rulePackagePolicies[rulePackageReference.key()]
	if !declarationsKnown {
		missing = append(missing, rulePackageReference)
	}
	for _, policy := range declaredPolicies {
		if policy.rulePackage != syntheticRulePackageReference(candidate.references) ||
			policy.judgmentType == "" || policy.semantic == "" || policy.strategyVersion == "" {
			conflict = true
		}
	}

	sort.Slice(objects, func(i, j int) bool {
		if objects[i].kind != objects[j].kind {
			return objects[i].kind < objects[j].kind
		}
		return objects[i].objectID < objects[j].objectID
	})
	sort.Slice(missing, func(i, j int) bool {
		if missing[i].kind != missing[j].kind {
			return missing[i].kind < missing[j].kind
		}
		return missing[i].objectID < missing[j].objectID
	})

	check.objects = objects
	check.missing = missing
	switch {
	case notApplicable:
		check.state = syntheticCandidateNotApplicable
	case conflict:
		check.state = syntheticCandidateConflict
	case len(missing) > 0:
		check.state = syntheticCandidatePending
	default:
		check.state = syntheticCandidateApplicable
	}
	return check
}

func syntheticObjectIdentityMismatch(
	objects []syntheticAdoptedCommercialObject,
	kind syntheticCommercialObjectKind,
	objectID string,
) (present bool, mismatch bool) {
	for _, object := range objects {
		if object.kind == kind {
			return true, object.objectID != objectID
		}
	}
	return false, false
}

func syntheticRulePackageReference(references []syntheticCommercialReference) syntheticCommercialReference {
	for _, reference := range references {
		if reference.kind == syntheticRulePackageKind {
			return reference
		}
	}
	return syntheticCommercialReference{}
}

func syntheticCandidateMatches(candidate syntheticCommercialCandidate, query syntheticCommercialResolutionQuery) bool {
	return candidate.tenantID == query.tenantID &&
		candidate.customerAccountID == query.customerAccountID &&
		candidate.legalEntityCandidate == query.legalEntityCandidate &&
		candidate.serviceOrControlScope == query.serviceOrControlScope &&
		candidate.purpose == query.purpose &&
		candidate.priceDirection == query.priceDirection &&
		candidate.basisSlot == query.basisSlot
}

func (fixture syntheticCommercialResolverFixture) resolveFirstPhase(
	query syntheticCommercialResolutionQuery,
) syntheticCommercialResolution {
	resolution := syntheticCommercialResolution{
		status:            syntheticResolutionInputNotAccept,
		query:             query,
		selectionAnchor:   query.selectionAnchor,
		fixtureVersion:    fixture.fixtureVersion,
		judgedAt:          fixture.judgedAt.UTC(),
		evidence:          syntheticEvidenceLevel,
		evidenceIndex:     "SYN-EVIDENCE-COMMERCIAL-RESOLUTION",
		authorityRevision: fixture.authorityRevision,
		reason:            syntheticInputMissing,
	}
	if !query.valid() {
		resolution.resolutionID = digestSyntheticResolution(string(resolution.status), query.fingerprint(), fixture.fixtureVersion)
		return resolution
	}
	policy, ok := fixture.anchorPolicies[query.selectionAnchor.policyVersion]
	if !ok || !policy.active || policy.version != query.selectionAnchor.policyVersion {
		resolution.status = syntheticResolutionPending
		resolution.reason = syntheticPendingAnchorPolicy
		resolution.continuationRef = continuationSyntheticResolution(query, resolution.reason)
		resolution.resolutionID = digestSyntheticResolution(string(resolution.status), query.fingerprint(), resolution.reason, fixture.fixtureVersion)
		return resolution
	}
	if !fixture.authorityAvailable || fixture.authorityRevision == "" {
		resolution.status = syntheticResolutionPending
		resolution.reason = syntheticPendingAuthority
		resolution.continuationRef = continuationSyntheticResolution(query, resolution.reason)
		resolution.resolutionID = digestSyntheticResolution(string(resolution.status), query.fingerprint(), resolution.reason, fixture.fixtureVersion)
		return resolution
	}

	applicable := make([]syntheticCandidateCheck, 0)
	pending := make([]syntheticCandidateCheck, 0)
	conflicting := make([]syntheticCandidateCheck, 0)
	for _, candidate := range fixture.candidates {
		if !syntheticCandidateMatches(candidate, query) {
			continue
		}
		check := fixture.checkCandidate(candidate, query)
		switch check.state {
		case syntheticCandidateApplicable:
			applicable = append(applicable, check)
		case syntheticCandidatePending:
			pending = append(pending, check)
		case syntheticCandidateConflict:
			conflicting = append(conflicting, check)
		}
	}

	// A confirmed pair of applicable candidates is already a deterministic
	// conflict, even if a third candidate is temporarily unreadable.
	if len(applicable) >= 2 || len(conflicting) > 0 {
		resolution.status = syntheticResolutionConflict
		resolution.reason = syntheticApplicabilityOverlap
		for _, check := range append(applicable, conflicting...) {
			resolution.conflictCandidates = append(
				resolution.conflictCandidates,
				syntheticConflictSnapshot(check),
			)
		}
		sort.Slice(resolution.conflictCandidates, func(i, j int) bool {
			return resolution.conflictCandidates[i].candidateID < resolution.conflictCandidates[j].candidateID
		})
		resolution.resolutionID = digestSyntheticResolution(
			string(resolution.status),
			query.fingerprint(),
			resolution.reason,
			fixture.authorityRevision,
			fixture.fixtureVersion,
			syntheticConflictDigest(resolution.conflictCandidates),
		)
		return resolution
	}

	if len(applicable) == 1 && len(pending) == 0 {
		check := applicable[0]
		resolution.status = syntheticResolutionUnique
		resolution.reason = ""
		resolution.baselineID = check.candidate.baselineID
		resolution.adopted = check.objects
		resolution.selectedRulePackage = syntheticRulePackageReference(check.candidate.references)
		resolution.asOfPolicies = append(
			[]syntheticAsOfPolicyDeclaration(nil),
			fixture.rulePackagePolicies[resolution.selectedRulePackage.key()]...,
		)
		sort.Slice(resolution.asOfPolicies, func(i, j int) bool {
			return resolution.asOfPolicies[i].judgmentType < resolution.asOfPolicies[j].judgmentType
		})
		resolution.resolutionID = digestSyntheticResolution(
			string(resolution.status),
			query.fingerprint(),
			fixture.authorityRevision,
			fixture.fixtureVersion,
			syntheticAdoptedDigest(resolution.adopted),
			syntheticAsOfPolicyDigest(resolution.asOfPolicies),
		)
		return resolution
	}

	if len(pending) > 0 {
		resolution.status = syntheticResolutionPending
		resolution.reason = syntheticPendingDependency
		for _, check := range pending {
			resolution.missingDependencies = append(resolution.missingDependencies, check.missing...)
		}
		resolution.missingDependencies = uniqueSyntheticReferences(resolution.missingDependencies)
		resolution.continuationRef = continuationSyntheticResolution(query, resolution.reason)
		resolution.resolutionID = digestSyntheticResolution(
			string(resolution.status),
			query.fingerprint(),
			resolution.reason,
			fixture.authorityRevision,
			fixture.fixtureVersion,
			syntheticReferenceDigest(resolution.missingDependencies),
		)
		return resolution
	}

	resolution.status = syntheticResolutionNoBasis
	resolution.reason = syntheticNoApplicableScope
	resolution.resolutionID = digestSyntheticResolution(
		string(resolution.status),
		query.fingerprint(),
		resolution.reason,
		fixture.authorityRevision,
		fixture.fixtureVersion,
	)
	return resolution
}

func syntheticConflictSnapshot(check syntheticCandidateCheck) syntheticConflictCandidateSnapshot {
	snapshot := syntheticConflictCandidateSnapshot{
		candidateID: check.candidate.candidateID,
		policyKind:  check.candidate.policyKind,
	}
	for _, object := range check.objects {
		if object.kind == check.candidate.policyKind {
			snapshot.policyReference = object.reference()
			snapshot.effectivePeriod = object.effectivePeriod
			snapshot.currentRevision = object.currentRevision
			snapshot.fixtureVersion = object.fixtureVersion
			break
		}
	}
	if snapshot.policyReference.objectID == "" {
		for _, reference := range check.candidate.references {
			if reference.kind == check.candidate.policyKind {
				snapshot.policyReference = reference
				break
			}
		}
	}
	return snapshot
}

func uniqueSyntheticReferences(references []syntheticCommercialReference) []syntheticCommercialReference {
	unique := make(map[syntheticCommercialReference]struct{})
	for _, reference := range references {
		unique[reference] = struct{}{}
	}
	result := make([]syntheticCommercialReference, 0, len(unique))
	for reference := range unique {
		result = append(result, reference)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].kind != result[j].kind {
			return result[i].kind < result[j].kind
		}
		if result[i].objectID != result[j].objectID {
			return result[i].objectID < result[j].objectID
		}
		return result[i].version < result[j].version
	})
	return result
}

func continuationSyntheticResolution(query syntheticCommercialResolutionQuery, reason string) string {
	return "SYN-CONT-" + strings.TrimPrefix(digestSyntheticResolution(query.fingerprint(), reason), "SYN-RES-")
}

func syntheticAdoptedDigest(objects []syntheticAdoptedCommercialObject) string {
	parts := make([]string, 0, len(objects))
	for _, object := range objects {
		parts = append(parts,
			string(object.kind)+":"+object.objectID+":"+object.version+":"+object.currentRevision+":"+object.fixtureVersion,
		)
	}
	sort.Strings(parts)
	return digestSyntheticResolution(parts...)
}

func syntheticAsOfPolicyDigest(policies []syntheticAsOfPolicyDeclaration) string {
	parts := make([]string, 0, len(policies))
	for _, policy := range policies {
		parts = append(parts,
			policy.rulePackage.objectID+":"+policy.rulePackage.version+":"+
				policy.judgmentType+":"+policy.semantic+":"+policy.strategyVersion,
		)
	}
	sort.Strings(parts)
	return digestSyntheticResolution(parts...)
}

func syntheticReferenceDigest(references []syntheticCommercialReference) string {
	parts := make([]string, 0, len(references))
	for _, reference := range references {
		parts = append(parts, string(reference.kind)+":"+reference.objectID+":"+reference.version)
	}
	sort.Strings(parts)
	return digestSyntheticResolution(parts...)
}

func syntheticConflictDigest(snapshots []syntheticConflictCandidateSnapshot) string {
	parts := make([]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		parts = append(parts,
			snapshot.candidateID+":"+string(snapshot.policyKind)+":"+
				snapshot.policyReference.objectID+":"+snapshot.policyReference.version+":"+
				snapshot.currentRevision+":"+snapshot.fixtureVersion,
		)
	}
	sort.Strings(parts)
	return digestSyntheticResolution(parts...)
}

func (fixture syntheticCommercialResolverFixture) validateBeforeDecision(
	resolution syntheticCommercialResolution,
) syntheticCommercialResolution {
	current := fixture.resolveFirstPhase(resolution.query)
	if resolution.status == syntheticResolutionUnique &&
		current.status == syntheticResolutionUnique &&
		current.resolutionID == resolution.resolutionID {
		return resolution
	}
	return syntheticCommercialResolution{
		status:            syntheticResolutionStale,
		resolutionID:      digestSyntheticResolution(string(syntheticResolutionStale), resolution.query.fingerprint(), current.resolutionID),
		query:             resolution.query,
		selectionAnchor:   resolution.selectionAnchor,
		fixtureVersion:    fixture.fixtureVersion,
		judgedAt:          fixture.judgedAt.UTC(),
		evidence:          syntheticEvidenceLevel,
		evidenceIndex:     resolution.evidenceIndex,
		authorityRevision: fixture.authorityRevision,
		continuationRef:   continuationSyntheticResolution(resolution.query, syntheticPendingStale),
		reason:            syntheticPendingStale,
	}
}

type syntheticAsOfValue struct {
	judgmentType    string
	strategyVersion string
	value           time.Time
}

type syntheticAuthorityJudgment struct {
	baselineID      string
	rulePackage     syntheticCommercialReference
	judgmentType    string
	strategyVersion string
	asOf            time.Time
	judgmentID      string
	adopted         syntheticAdoptedCommercialObject
	judgedAt        time.Time
	available       bool
}

type syntheticAsOfEcho struct {
	judgmentType    string
	semantic        string
	strategyVersion string
	asOf            time.Time
	judgmentID      string
	adopted         syntheticAdoptedCommercialObject
	judgedAt        time.Time
}

type syntheticSecondPhaseResolution struct {
	status          syntheticResolutionStatus
	resolutionID    string
	baselineID      string
	selectionAnchor syntheticCommercialSelectionAnchor
	fixtureVersion  string
	echos           []syntheticAsOfEcho
	evidence        string
	continuationRef string
	reason          string
}

func resolveSyntheticSecondPhase(
	baseline syntheticCommercialResolution,
	values []syntheticAsOfValue,
	authorities map[string]syntheticAuthorityJudgment,
) syntheticSecondPhaseResolution {
	result := syntheticSecondPhaseResolution{
		status:          syntheticResolutionPending,
		baselineID:      baseline.resolutionID,
		selectionAnchor: baseline.selectionAnchor,
		fixtureVersion:  baseline.fixtureVersion,
		evidence:        syntheticEvidenceLevel,
		reason:          syntheticPendingDependency,
		continuationRef: continuationSyntheticResolution(baseline.query, "SECOND_PHASE"),
	}
	if baseline.status != syntheticResolutionUnique {
		result.reason = "BASELINE_NOT_UNIQUE"
		result.resolutionID = digestSyntheticResolution(string(result.status), result.baselineID, result.reason, result.fixtureVersion)
		return result
	}
	if len(baseline.asOfPolicies) == 0 || len(values) == 0 {
		result.reason = "AS_OF_POLICY_OR_VALUE_MISSING"
		result.resolutionID = digestSyntheticResolution(string(result.status), result.baselineID, result.reason, result.fixtureVersion)
		return result
	}

	declarations := make(map[string]syntheticAsOfPolicyDeclaration, len(baseline.asOfPolicies))
	for _, declaration := range baseline.asOfPolicies {
		if declaration.rulePackage != baseline.selectedRulePackage ||
			declaration.judgmentType == "" || declaration.semantic == "" ||
			declaration.strategyVersion == "" {
			result.reason = "RULE_PACKAGE_POLICY_INVALID"
			result.resolutionID = digestSyntheticResolution(string(result.status), result.baselineID, result.reason, result.fixtureVersion)
			return result
		}
		if _, exists := declarations[declaration.judgmentType]; exists {
			result.reason = "RULE_PACKAGE_POLICY_DUPLICATE"
			result.resolutionID = digestSyntheticResolution(string(result.status), result.baselineID, result.reason, result.fixtureVersion)
			return result
		}
		declarations[declaration.judgmentType] = declaration
	}

	seenValues := make(map[string]bool)
	echos := make([]syntheticAsOfEcho, 0, len(values))
	for _, value := range values {
		declaration, ok := declarations[value.judgmentType]
		if !ok || seenValues[value.judgmentType] || value.value.IsZero() ||
			value.strategyVersion != declaration.strategyVersion {
			result.reason = "AS_OF_VALUE_INVALID"
			result.resolutionID = digestSyntheticResolution(string(result.status), result.baselineID, result.reason, result.fixtureVersion)
			return result
		}
		seenValues[value.judgmentType] = true
		judgment, ok := authorities[value.judgmentType]
		if !ok || !judgment.available ||
			judgment.baselineID != baseline.resolutionID ||
			judgment.rulePackage != baseline.selectedRulePackage ||
			judgment.judgmentType != value.judgmentType ||
			judgment.strategyVersion != declaration.strategyVersion ||
			!judgment.asOf.Equal(value.value) ||
			judgment.judgmentID == "" || judgment.judgedAt.IsZero() ||
			!syntheticBaselineContainsObject(baseline, judgment.adopted) ||
			!syntheticPeriodContains(judgment.adopted.effectivePeriod, value.value) {
			result.reason = "AUTHORITY_ECHO_UNAVAILABLE"
			result.resolutionID = digestSyntheticResolution(string(result.status), result.baselineID, result.reason, result.fixtureVersion)
			return result
		}
		echos = append(echos, syntheticAsOfEcho{
			judgmentType:    value.judgmentType,
			semantic:        declaration.semantic,
			strategyVersion: declaration.strategyVersion,
			asOf:            value.value.UTC(),
			judgmentID:      judgment.judgmentID,
			adopted:         judgment.adopted,
			judgedAt:        judgment.judgedAt.UTC(),
		})
	}
	if len(seenValues) != len(declarations) {
		result.reason = "AS_OF_VALUE_MISSING"
		result.resolutionID = digestSyntheticResolution(string(result.status), result.baselineID, result.reason, result.fixtureVersion)
		return result
	}
	sort.Slice(echos, func(i, j int) bool { return echos[i].judgmentType < echos[j].judgmentType })
	result.status = syntheticResolutionUnique
	result.reason = ""
	result.continuationRef = ""
	result.echos = echos
	phaseParts := []string{string(result.status), result.baselineID, result.fixtureVersion}
	for _, echo := range echos {
		phaseParts = append(phaseParts,
			echo.judgmentType,
			echo.strategyVersion,
			echo.asOf.UTC().Format(time.RFC3339Nano),
			echo.judgmentID,
		)
	}
	result.resolutionID = digestSyntheticResolution(phaseParts...)
	return result
}

func syntheticBaselineContainsObject(
	baseline syntheticCommercialResolution,
	candidate syntheticAdoptedCommercialObject,
) bool {
	for _, object := range baseline.adopted {
		if object.kind == candidate.kind &&
			object.objectID == candidate.objectID &&
			object.version == candidate.version &&
			object.currentRevision == candidate.currentRevision &&
			object.fixtureVersion == candidate.fixtureVersion &&
			object.scopeReference == candidate.scopeReference &&
			object.effectivePeriod.startsAt.Equal(candidate.effectivePeriod.startsAt) &&
			object.effectivePeriod.endsAt.Equal(candidate.effectivePeriod.endsAt) {
			return true
		}
	}
	return false
}

func adoptedSyntheticObject(
	resolution syntheticCommercialResolution,
	kind syntheticCommercialObjectKind,
) syntheticAdoptedCommercialObject {
	for _, object := range resolution.adopted {
		if object.kind == kind {
			return object
		}
	}
	return syntheticAdoptedCommercialObject{}
}

func digestSyntheticResolution(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "SYN-RES-" + hex.EncodeToString(digest[:8])
}

func assertSyntheticResolutionTrace(t *testing.T, resolution syntheticCommercialResolution) {
	t.Helper()
	if resolution.resolutionID == "" || resolution.selectionAnchor.value() == "" ||
		resolution.selectionAnchor.policyVersion == "" || resolution.judgedAt.IsZero() ||
		resolution.evidence != syntheticEvidenceLevel || resolution.evidenceIndex == "" ||
		resolution.authorityRevision == "" || resolution.fixtureVersion == "" {
		t.Fatalf("resolution trace incomplete: %#v", resolution)
	}
}

func TestSyntheticCommercialResolutionUsesSharedBaselineAndScopeSpecificPolicy(t *testing.T) {
	fixture := newSyntheticCommercialResolverFixture()
	period := syntheticPeriodFor(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	baseline := fixture.publishBaseline(t, "SYN-CONTRACT-FAMILY-01", period, "rev-baseline-1", true)
	fixture.addPolicyCandidate(t, "SYN-COM-01", baseline, "SYN-SCOPE-PREPAID-01", syntheticPrepaidPolicyKind, period, "rev-prepaid-1")
	fixture.addPolicyCandidate(t, "SYN-COM-02", baseline, "SYN-SCOPE-TERMS-01", syntheticTermsPolicyKind, period, "rev-terms-1")

	prepaid := fixture.resolveFirstPhase(syntheticResolutionQueryForScope("SYN-SCOPE-PREPAID-01"))
	terms := fixture.resolveFirstPhase(syntheticResolutionQueryForScope("SYN-SCOPE-TERMS-01"))
	if prepaid.status != syntheticResolutionUnique || terms.status != syntheticResolutionUnique {
		t.Fatalf("scope-specific resolutions = prepaid %#v terms %#v", prepaid, terms)
	}
	assertSyntheticResolutionTrace(t, prepaid)
	assertSyntheticResolutionTrace(t, terms)
	if adoptedSyntheticObject(prepaid, syntheticPrepaidPolicyKind).objectID == "" ||
		adoptedSyntheticObject(terms, syntheticTermsPolicyKind).objectID == "" {
		t.Fatalf("settlement policy result missing: prepaid %#v terms %#v", prepaid.adopted, terms.adopted)
	}
	if adoptedSyntheticObject(prepaid, syntheticContractKind).objectID != adoptedSyntheticObject(terms, syntheticContractKind).objectID ||
		adoptedSyntheticObject(prepaid, syntheticServiceProductKind).objectID != adoptedSyntheticObject(terms, syntheticServiceProductKind).objectID ||
		adoptedSyntheticObject(prepaid, syntheticRulePackageKind).objectID != adoptedSyntheticObject(terms, syntheticRulePackageKind).objectID {
		t.Fatalf("SYN-COM-01/02 did not share a commercial baseline")
	}
	if adoptedSyntheticObject(prepaid, syntheticCustomerAccountKind).objectID != "SYN-CUST-01" ||
		adoptedSyntheticObject(prepaid, syntheticLegalEntityKind).objectID != "SYN-LEGAL-01" {
		t.Fatalf("resolved identity did not match query: %#v", prepaid.adopted)
	}
	for _, object := range append(prepaid.adopted, terms.adopted...) {
		if object.fixtureVersion != syntheticCommercialFixtureVersion {
			t.Fatalf("fixture version missing from adopted object: %#v", object)
		}
	}
}

func TestSyntheticCommercialResolutionDistinguishesZeroConflictAndPending(t *testing.T) {
	period := syntheticPeriodFor(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)

	zeroFixture := newSyntheticCommercialResolverFixture()
	zero := zeroFixture.resolveFirstPhase(syntheticResolutionQueryForScope("SYN-SCOPE-UNMAPPED-01"))
	if zero.status != syntheticResolutionNoBasis || zero.reason != syntheticNoApplicableScope ||
		len(zero.adopted) != 0 || len(zero.conflictCandidates) != 0 {
		t.Fatalf("zero result = %#v", zero)
	}
	assertSyntheticResolutionTrace(t, zero)

	conflictFixture := newSyntheticCommercialResolverFixture()
	baseline := conflictFixture.publishBaseline(t, "SYN-CONTRACT-FAMILY-OVERLAP", period, "rev-overlap-base", true)
	conflictFixture.addPolicyCandidate(t, "SYN-COM-03-PREPAID", baseline, "SYN-SCOPE-CONTROL-OVERLAP-01", syntheticPrepaidPolicyKind, period, "rev-overlap-p")
	conflictFixture.addPolicyCandidate(t, "SYN-COM-03-TERMS", baseline, "SYN-SCOPE-CONTROL-OVERLAP-01", syntheticTermsPolicyKind, period, "rev-overlap-t")
	pendingCandidate := conflictFixture.candidates[0]
	pendingCandidate.candidateID = "SYN-COM-03-PENDING-THIRD"
	pendingCandidate.references = append([]syntheticCommercialReference(nil), pendingCandidate.references...)
	for index, reference := range pendingCandidate.references {
		if reference.kind == syntheticRulePackageKind {
			pendingCandidate.references[index] = syntheticCommercialReference{
				kind:     syntheticRulePackageKind,
				objectID: "SYN-MISSING-RULE-PACKAGE",
				version:  "v1",
			}
		}
	}
	conflictFixture.candidates = append(conflictFixture.candidates, pendingCandidate)
	conflictQuery := syntheticResolutionQueryForScope("SYN-SCOPE-CONTROL-OVERLAP-01")
	conflict := conflictFixture.resolveFirstPhase(conflictQuery)
	if conflict.status != syntheticResolutionConflict || conflict.reason != syntheticApplicabilityOverlap ||
		len(conflict.conflictCandidates) != 2 || len(conflict.adopted) != 0 {
		t.Fatalf("conflict result = %#v", conflict)
	}
	for _, snapshot := range conflict.conflictCandidates {
		if snapshot.policyReference.objectID == "" || snapshot.effectivePeriod.startsAt.IsZero() ||
			snapshot.currentRevision == "" {
			t.Fatalf("conflict trace incomplete: %#v", snapshot)
		}
	}

	pendingFixture := newSyntheticCommercialResolverFixture()
	missingBaseline := pendingFixture.publishBaseline(t, "SYN-CONTRACT-FAMILY-PENDING", period, "rev-pending-base", false)
	pendingFixture.addPolicyCandidate(t, "SYN-COM-04-PENDING", missingBaseline, "SYN-SCOPE-PENDING-01", syntheticPrepaidPolicyKind, period, "rev-pending-policy")
	pending := pendingFixture.resolveFirstPhase(syntheticResolutionQueryForScope("SYN-SCOPE-PENDING-01"))
	if pending.status != syntheticResolutionPending || pending.reason != syntheticPendingDependency ||
		len(pending.missingDependencies) == 0 || pending.continuationRef == "" ||
		len(pending.adopted) != 0 {
		t.Fatalf("pending result = %#v", pending)
	}
}

func TestSyntheticCommercialResolutionRejectsIncompleteInputAndUnknownAnchor(t *testing.T) {
	fixture := newSyntheticCommercialResolverFixture()
	query := syntheticResolutionQueryForScope("SYN-SCOPE-PREPAID-01")
	query.tenantID = ""
	input := fixture.resolveFirstPhase(query)
	if input.status != syntheticResolutionInputNotAccept || input.reason != syntheticInputMissing ||
		len(input.adopted) != 0 || len(input.conflictCandidates) != 0 {
		t.Fatalf("incomplete input = %#v", input)
	}

	query = syntheticResolutionQueryForScope("SYN-SCOPE-PREPAID-01")
	query.selectionAnchor.policyVersion = "SYN-ANCHOR-POLICY-MISSING"
	pending := fixture.resolveFirstPhase(query)
	if pending.status != syntheticResolutionPending || pending.reason != syntheticPendingAnchorPolicy ||
		pending.continuationRef == "" {
		t.Fatalf("unknown anchor policy = %#v", pending)
	}

	query = syntheticResolutionQueryForScope("SYN-SCOPE-PREPAID-01")
	query.basisSlot = string(syntheticPrepaidPolicyKind)
	invalidSlot := fixture.resolveFirstPhase(query)
	if invalidSlot.status != syntheticResolutionInputNotAccept || invalidSlot.reason != syntheticInputMissing {
		t.Fatalf("caller-selected policy mode was accepted as a basis slot: %#v", invalidSlot)
	}
}

func TestSyntheticCommercialResolutionRejectsMixedModeCandidateAndPolicyScopeRebinding(t *testing.T) {
	period := syntheticPeriodFor(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)

	mixedFixture := newSyntheticCommercialResolverFixture()
	mixedBaseline := mixedFixture.publishBaseline(t, "SYN-CONTRACT-FAMILY-MIXED", period, "rev-mixed-base", true)
	mixedFixture.addPolicyCandidate(t, "SYN-MIXED-MODE", mixedBaseline, "SYN-SCOPE-MIXED-01", syntheticPrepaidPolicyKind, period, "rev-mixed-prepaid")
	terms := syntheticCommercialVersionFor(t, syntheticTermsPolicyKind, "SYN-MIXED-MODE-TERMS", "v1", "SYN-SCOPE-MIXED-01")
	terms.period = period
	terms.currentRevision = "rev-mixed-terms"
	publishSyntheticResolutionVersion(t, &mixedFixture.registry, terms)
	mixedFixture.candidates[0].references = append(
		mixedFixture.candidates[0].references,
		syntheticCommercialReference{kind: terms.kind, objectID: terms.objectID, version: terms.version},
	)
	mixed := mixedFixture.resolveFirstPhase(syntheticResolutionQueryForScope("SYN-SCOPE-MIXED-01"))
	if mixed.status != syntheticResolutionConflict || len(mixed.conflictCandidates) != 1 ||
		len(mixed.adopted) != 0 {
		t.Fatalf("single candidate with prepaid and terms was accepted: %#v", mixed)
	}

	scopeFixture := newSyntheticCommercialResolverFixture()
	scopeBaseline := scopeFixture.publishBaseline(t, "SYN-CONTRACT-FAMILY-SCOPE", period, "rev-scope-base", true)
	scopeFixture.addPolicyCandidate(t, "SYN-SCOPE-REBIND", scopeBaseline, "SYN-SCOPE-A", syntheticPrepaidPolicyKind, period, "rev-scope-policy")
	scopeFixture.candidates[0].serviceOrControlScope = "SYN-SCOPE-B"
	rebound := scopeFixture.resolveFirstPhase(syntheticResolutionQueryForScope("SYN-SCOPE-B"))
	if rebound.status != syntheticResolutionConflict || len(rebound.adopted) != 0 {
		t.Fatalf("published policy scope was rebound by candidate metadata: %#v", rebound)
	}
}

func TestSyntheticCommercialResolutionAuthorityFailureIsPendingNotZero(t *testing.T) {
	fixture := newSyntheticCommercialResolverFixture()
	fixture.authorityAvailable = false
	resolution := fixture.resolveFirstPhase(syntheticResolutionQueryForScope("SYN-SCOPE-UNMAPPED-01"))
	if resolution.status != syntheticResolutionPending || resolution.reason != syntheticPendingAuthority ||
		resolution.continuationRef == "" || len(resolution.adopted) != 0 {
		t.Fatalf("authority failure = %#v", resolution)
	}
}

func TestSyntheticCommercialResolutionUsesIndependentAnchorAndDoesNotReviveExpiredBasis(t *testing.T) {
	fixture := newSyntheticCommercialResolverFixture()
	expired := syntheticPeriodFor(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	)
	baseline := fixture.publishBaseline(t, "SYN-CONTRACT-FAMILY-EXPIRED", expired, "rev-expired-base", true)
	fixture.addPolicyCandidate(t, "SYN-EXPIRED-POLICY", baseline, "SYN-SCOPE-EXPIRED-01", syntheticPrepaidPolicyKind, expired, "rev-expired-policy")

	oldBusinessTime := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	query := syntheticResolutionQueryForScope("SYN-SCOPE-EXPIRED-01")
	query.requestEffectiveAt = &oldBusinessTime
	withBackfill := fixture.resolveFirstPhase(query)
	query.requestEffectiveAt = nil
	withoutBackfill := fixture.resolveFirstPhase(query)
	if withBackfill.status != syntheticResolutionNoBasis ||
		withBackfill.resolutionID != withoutBackfill.resolutionID {
		t.Fatalf("backfilled time affected independent selection: with=%#v without=%#v", withBackfill, withoutBackfill)
	}
}

func TestSyntheticCommercialResolutionReplayAndRevisionChange(t *testing.T) {
	fixture := newSyntheticCommercialResolverFixture()
	period := syntheticPeriodFor(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	baseline := fixture.publishBaseline(t, "SYN-CONTRACT-FAMILY-REPLAY", period, "rev-replay-base-1", true)
	fixture.addPolicyCandidate(t, "SYN-REPLAY-POLICY", baseline, "SYN-SCOPE-REPLAY-01", syntheticPrepaidPolicyKind, period, "rev-replay-policy-1")
	query := syntheticResolutionQueryForScope("SYN-SCOPE-REPLAY-01")
	first := fixture.resolveFirstPhase(query)
	replay := fixture.resolveFirstPhase(query)
	if first.status != syntheticResolutionUnique || replay.status != syntheticResolutionUnique ||
		first.resolutionID != replay.resolutionID || first.semanticDigest() != replay.semanticDigest() {
		t.Fatalf("same input/revision did not replay: first=%#v replay=%#v", first, replay)
	}
	fixture.judgedAt = fixture.judgedAt.Add(5 * time.Minute)
	replayWithDifferentClock := fixture.resolveFirstPhase(query)
	if replayWithDifferentClock.resolutionID != first.resolutionID ||
		replayWithDifferentClock.semanticDigest() != first.semanticDigest() {
		t.Fatalf("judgement clock changed replay semantics: first=%#v replay=%#v", first, replayWithDifferentClock)
	}

	contractRef := baseline.references[3]
	changed := fixture.registry.versions[contractRef.key()]
	changed.currentRevision = "rev-replay-base-2"
	fixture.registry.versions[contractRef.key()] = changed
	fixture.authorityRevision = "SYN-COMMERCIAL-VIEW-rev-2"
	stale := fixture.validateBeforeDecision(first)
	if stale.status != syntheticResolutionStale || stale.reason != syntheticPendingStale ||
		len(stale.adopted) != 0 || stale.continuationRef == "" {
		t.Fatalf("revision change did not stale result: %#v", stale)
	}
	resolvedAgain := fixture.resolveFirstPhase(query)
	if resolvedAgain.status != syntheticResolutionUnique || resolvedAgain.resolutionID == first.resolutionID {
		t.Fatalf("re-resolution did not capture new revision: old=%s new=%s", first.resolutionID, resolvedAgain.resolutionID)
	}
}

func TestSyntheticCommercialResolutionNewOverlapInvalidatesPriorUniqueResult(t *testing.T) {
	fixture := newSyntheticCommercialResolverFixture()
	period := syntheticPeriodFor(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	baseline := fixture.publishBaseline(t, "SYN-CONTRACT-FAMILY-NEW-OVERLAP", period, "rev-overlap-base-1", true)
	fixture.addPolicyCandidate(t, "SYN-PRIOR-UNIQUE", baseline, "SYN-SCOPE-NEW-OVERLAP-01", syntheticPrepaidPolicyKind, period, "rev-prior-1")
	query := syntheticResolutionQueryForScope("SYN-SCOPE-NEW-OVERLAP-01")
	prior := fixture.resolveFirstPhase(query)
	if prior.status != syntheticResolutionUnique {
		t.Fatalf("prior result = %#v", prior)
	}

	fixture.addPolicyCandidate(t, "SYN-NEW-TERMS", baseline, "SYN-SCOPE-NEW-OVERLAP-01", syntheticTermsPolicyKind, period, "rev-new-terms-1")
	fixture.authorityRevision = "SYN-COMMERCIAL-VIEW-rev-2"
	current := fixture.resolveFirstPhase(query)
	stale := fixture.validateBeforeDecision(prior)
	if current.status != syntheticResolutionConflict || len(current.conflictCandidates) != 2 {
		t.Fatalf("new overlap did not form conflict: %#v", current)
	}
	if stale.status != syntheticResolutionStale || len(stale.adopted) != 0 {
		t.Fatalf("prior unique result remained usable: %#v", stale)
	}
}

func TestSyntheticCommercialResolutionSecondPhaseUsesSelectedRulePackagePolicies(t *testing.T) {
	fixture := newSyntheticCommercialResolverFixture()
	period := syntheticPeriodFor(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	baselineFixture := fixture.publishBaseline(t, "SYN-CONTRACT-FAMILY-ASOF", period, "rev-asof-base-1", true)
	declarations := []syntheticAsOfPolicyDeclaration{
		{
			rulePackage:     baselineFixture.rulePackage,
			judgmentType:    "NETWORK_REACHABILITY",
			semantic:        "CURRENT_SUBMISSION_RECEIVED_AT",
			strategyVersion: "v1",
		},
		{
			rulePackage:     baselineFixture.rulePackage,
			judgmentType:    "FINANCIAL_CONTROL",
			semantic:        "CONTROL_EVALUATION_AT",
			strategyVersion: "v2",
		},
	}
	fixture.declareRulePackagePolicies(t, baselineFixture, declarations)
	fixture.addPolicyCandidate(t, "SYN-ASOF-POLICY", baselineFixture, "SYN-SCOPE-ASOF-01", syntheticPrepaidPolicyKind, period, "rev-asof-policy-1")
	baseline := fixture.resolveFirstPhase(syntheticResolutionQueryForScope("SYN-SCOPE-ASOF-01"))
	if baseline.status != syntheticResolutionUnique || len(baseline.asOfPolicies) != 2 ||
		baseline.selectedRulePackage != baselineFixture.rulePackage {
		t.Fatalf("baseline policies = %#v", baseline)
	}

	networkAsOf := time.Date(2026, 8, 7, 10, 1, 0, 0, time.UTC)
	financialAsOf := time.Date(2026, 8, 6, 23, 0, 0, 0, time.UTC)
	values := []syntheticAsOfValue{
		{judgmentType: "NETWORK_REACHABILITY", strategyVersion: "v1", value: networkAsOf},
		{judgmentType: "FINANCIAL_CONTROL", strategyVersion: "v2", value: financialAsOf},
	}
	authorities := map[string]syntheticAuthorityJudgment{
		"NETWORK_REACHABILITY": {
			baselineID:      baseline.resolutionID,
			rulePackage:     baseline.selectedRulePackage,
			judgmentType:    "NETWORK_REACHABILITY",
			strategyVersion: "v1",
			asOf:            networkAsOf,
			judgmentID:      "SYN-JUDGMENT-NETWORK",
			adopted:         adoptedSyntheticObject(baseline, syntheticServiceProductKind),
			judgedAt:        fixture.judgedAt,
			available:       true,
		},
		"FINANCIAL_CONTROL": {
			baselineID:      baseline.resolutionID,
			rulePackage:     baseline.selectedRulePackage,
			judgmentType:    "FINANCIAL_CONTROL",
			strategyVersion: "v2",
			asOf:            financialAsOf,
			judgmentID:      "SYN-JUDGMENT-FINANCIAL",
			adopted:         adoptedSyntheticObject(baseline, syntheticPrepaidPolicyKind),
			judgedAt:        fixture.judgedAt,
			available:       true,
		},
	}
	second := resolveSyntheticSecondPhase(baseline, values, authorities)
	if second.status != syntheticResolutionUnique || second.baselineID != baseline.resolutionID ||
		len(second.echos) != 2 || second.evidence != syntheticEvidenceLevel ||
		second.fixtureVersion != syntheticCommercialFixtureVersion || second.resolutionID == "" {
		t.Fatalf("second phase = %#v", second)
	}
	if second.echos[0].asOf.Equal(second.echos[1].asOf) {
		t.Fatalf("independent asOf values were collapsed: %#v", second.echos)
	}
	for _, echo := range second.echos {
		if echo.semantic == "" || echo.strategyVersion == "" || echo.judgmentID == "" ||
			echo.adopted.currentRevision == "" || echo.judgedAt.IsZero() {
			t.Fatalf("authority echo incomplete: %#v", echo)
		}
	}

	tamperedScope := cloneSyntheticAuthorityJudgments(authorities)
	scopeJudgment := tamperedScope["FINANCIAL_CONTROL"]
	scopeJudgment.adopted.scopeReference = "SYN-TAMPERED-SCOPE"
	tamperedScope["FINANCIAL_CONTROL"] = scopeJudgment
	rejectedScope := resolveSyntheticSecondPhase(baseline, values, tamperedScope)
	if rejectedScope.status != syntheticResolutionPending || rejectedScope.reason != "AUTHORITY_ECHO_UNAVAILABLE" {
		t.Fatalf("tampered authority scope was accepted: %#v", rejectedScope)
	}

	tamperedPeriod := cloneSyntheticAuthorityJudgments(authorities)
	periodJudgment := tamperedPeriod["FINANCIAL_CONTROL"]
	periodJudgment.adopted.effectivePeriod.startsAt = periodJudgment.adopted.effectivePeriod.startsAt.Add(-24 * time.Hour)
	tamperedPeriod["FINANCIAL_CONTROL"] = periodJudgment
	rejectedPeriod := resolveSyntheticSecondPhase(baseline, values, tamperedPeriod)
	if rejectedPeriod.status != syntheticResolutionPending || rejectedPeriod.reason != "AUTHORITY_ECHO_UNAVAILABLE" {
		t.Fatalf("tampered authority period was accepted: %#v", rejectedPeriod)
	}
}

func cloneSyntheticAuthorityJudgments(
	source map[string]syntheticAuthorityJudgment,
) map[string]syntheticAuthorityJudgment {
	clone := make(map[string]syntheticAuthorityJudgment, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func TestSyntheticCommercialResolutionSecondPhaseRejectsInventedOrMissingPolicy(t *testing.T) {
	fixture := newSyntheticCommercialResolverFixture()
	period := syntheticPeriodFor(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	baselineFixture := fixture.publishBaseline(t, "SYN-CONTRACT-FAMILY-NO-ASOF", period, "rev-no-asof-base", true)
	fakeRulePackage := syntheticCommercialReference{
		kind:     syntheticRulePackageKind,
		objectID: "SYN-UNRELATED-RULE-PACKAGE",
		version:  "v1",
	}
	fixture.rulePackagePolicies[fakeRulePackage.key()] = []syntheticAsOfPolicyDeclaration{{
		rulePackage:     fakeRulePackage,
		judgmentType:    "FINANCIAL_CONTROL",
		semantic:        "CALLER_INVENTED",
		strategyVersion: "caller-v1",
	}}
	fixture.addPolicyCandidate(t, "SYN-NO-ASOF-POLICY", baselineFixture, "SYN-SCOPE-NO-ASOF-01", syntheticPrepaidPolicyKind, period, "rev-no-asof-policy")
	baseline := fixture.resolveFirstPhase(syntheticResolutionQueryForScope("SYN-SCOPE-NO-ASOF-01"))
	if baseline.status != syntheticResolutionUnique || len(baseline.asOfPolicies) != 0 {
		t.Fatalf("baseline = %#v", baseline)
	}
	invented := resolveSyntheticSecondPhase(
		baseline,
		[]syntheticAsOfValue{{
			judgmentType:    "FINANCIAL_CONTROL",
			strategyVersion: "caller-v1",
			value:           fixture.judgedAt,
		}},
		nil,
	)
	if invented.status != syntheticResolutionPending || invented.reason != "AS_OF_POLICY_OR_VALUE_MISSING" ||
		invented.continuationRef == "" || len(invented.echos) != 0 {
		t.Fatalf("caller invented an asOf policy: %#v", invented)
	}
}
