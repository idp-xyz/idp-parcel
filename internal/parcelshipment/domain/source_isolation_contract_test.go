package domain_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 这些夹具模拟 parcel-shipment 所消费的那条边界。它们刻意只存在于测试侧：认证、授权与存储
// 仍归共享平台能力或拥有它的业务上下文所有。
type syntheticAuthorizedScope struct {
	reference         string
	tenantID          string
	customerAccountID string
}

func syntheticScopeFor(
	t *testing.T,
	reference string,
	tenant string,
	customer string,
) syntheticAuthorizedScope {
	t.Helper()
	if strings.TrimSpace(reference) == "" || strings.TrimSpace(tenant) == "" ||
		strings.TrimSpace(customer) == "" {
		t.Fatal("synthetic authorized scope must be complete")
	}
	return syntheticAuthorizedScope{
		reference:         reference,
		tenantID:          tenant,
		customerAccountID: customer,
	}
}

func (scope syntheticAuthorizedScope) permits(identity domain.SourceIdentity) bool {
	return scope.tenantID == identity.TenantID().String() &&
		scope.customerAccountID == identity.CustomerAccountID().String()
}

type syntheticSourceKey struct {
	tenantID          string
	customerAccountID string
	source            string
	requestKey        string
}

func sourceKey(identity domain.SourceIdentity) syntheticSourceKey {
	return syntheticSourceKey{
		tenantID:          identity.TenantID().String(),
		customerAccountID: identity.CustomerAccountID().String(),
		source:            identity.Source().String(),
		requestKey:        identity.RequestKey().String(),
	}
}

type syntheticSourceRecord struct {
	fingerprint         domain.SourceSubmissionFingerprint
	objectReference     string
	resultReference     string
	confidentialPayload string
	audit               syntheticAuditRecord
}

type syntheticSourceIndex struct {
	records map[syntheticSourceKey]syntheticSourceRecord
}

func newSyntheticSourceIndex() syntheticSourceIndex {
	return syntheticSourceIndex{records: make(map[syntheticSourceKey]syntheticSourceRecord)}
}

func (index syntheticSourceIndex) put(record syntheticSourceRecord) {
	index.records[sourceKey(record.fingerprint.Identity())] = record
}

type syntheticLookupDisposition string

const (
	syntheticVisibleResult        syntheticLookupDisposition = "VISIBLE"
	syntheticNotFoundOrNotVisible syntheticLookupDisposition = "NOT_FOUND_OR_NOT_VISIBLE"
	syntheticConflictResult       syntheticLookupDisposition = "CONFLICT"
)

type syntheticLookupResult struct {
	disposition     syntheticLookupDisposition
	objectReference string
	resultReference string
}

func (index syntheticSourceIndex) lookup(
	scope syntheticAuthorizedScope,
	fingerprint domain.SourceSubmissionFingerprint,
) syntheticLookupResult {
	record, exists := index.records[sourceKey(fingerprint.Identity())]
	if !exists || !scope.permits(fingerprint.Identity()) {
		// 这个否定结果有意不区分「不存在」与「在调用方授权范围之外」。
		return syntheticLookupResult{disposition: syntheticNotFoundOrNotVisible}
	}
	if record.fingerprint.Digest() != fingerprint.Digest() {
		return syntheticLookupResult{disposition: syntheticConflictResult}
	}
	return syntheticLookupResult{
		disposition:     syntheticVisibleResult,
		objectReference: record.objectReference,
		resultReference: record.resultReference,
	}
}

type syntheticAuditRecord struct {
	ScopeReference  string    `json:"scopeRef"`
	ObjectReference string    `json:"objectRef"`
	ResultCategory  string    `json:"resultCategory"`
	Version         string    `json:"version"`
	EvaluatedAt     time.Time `json:"evaluatedAt"`
	EvidenceIndex   string    `json:"evidenceIndex"`
}

func syntheticSourceRecordFor(
	t *testing.T,
	tenant string,
	customer string,
	source string,
	requestKey string,
	digest string,
	objectReference string,
	resultReference string,
	scopeReference string,
) syntheticSourceRecord {
	t.Helper()
	fingerprint := sourceFingerprint(t, tenant, customer, source, requestKey, digest)
	return syntheticSourceRecord{
		fingerprint:         fingerprint,
		objectReference:     objectReference,
		resultReference:     resultReference,
		confidentialPayload: "customer-name=synthetic-only;address=restricted;goods=restricted",
		audit: syntheticAuditRecord{
			ScopeReference:  scopeReference,
			ObjectReference: objectReference,
			ResultCategory:  "SOURCE_REPLAY_RESULT",
			Version:         "SYN-W06-v1",
			EvaluatedAt:     time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC),
			EvidenceIndex:   "SYN-EVIDENCE-W06-1",
		},
	}
}

func TestSyntheticSourceIsolationSeparatesSameExternalKeyAcrossScopes(t *testing.T) {
	index := newSyntheticSourceIndex()
	records := []syntheticSourceRecord{
		syntheticSourceRecordFor(t, "tenant-1", "customer-1", "api", "request-1", "digest-1", "parcel-1", "result-1", "scope-1"),
		syntheticSourceRecordFor(t, "tenant-2", "customer-1", "api", "request-1", "digest-1", "parcel-2", "result-2", "scope-2"),
		syntheticSourceRecordFor(t, "tenant-1", "customer-2", "api", "request-1", "digest-1", "parcel-3", "result-3", "scope-3"),
		syntheticSourceRecordFor(t, "tenant-1", "customer-1", "file", "request-1", "digest-1", "parcel-4", "result-4", "scope-4"),
	}
	for _, record := range records {
		index.put(record)
	}

	tests := []struct {
		name        string
		scope       syntheticAuthorizedScope
		fingerprint domain.SourceSubmissionFingerprint
		wantObject  string
		wantResult  string
	}{
		{
			name:        "tenant one customer one api",
			scope:       syntheticScopeFor(t, "scope-1", "tenant-1", "customer-1"),
			fingerprint: records[0].fingerprint,
			wantObject:  "parcel-1",
			wantResult:  "result-1",
		},
		{
			name:        "tenant two same customer and key",
			scope:       syntheticScopeFor(t, "scope-2", "tenant-2", "customer-1"),
			fingerprint: records[1].fingerprint,
			wantObject:  "parcel-2",
			wantResult:  "result-2",
		},
		{
			name:        "same tenant different customer",
			scope:       syntheticScopeFor(t, "scope-3", "tenant-1", "customer-2"),
			fingerprint: records[2].fingerprint,
			wantObject:  "parcel-3",
			wantResult:  "result-3",
		},
		{
			name:        "same customer different source",
			scope:       syntheticScopeFor(t, "scope-4", "tenant-1", "customer-1"),
			fingerprint: records[3].fingerprint,
			wantObject:  "parcel-4",
			wantResult:  "result-4",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := index.lookup(test.scope, test.fingerprint)
			want := syntheticLookupResult{
				disposition:     syntheticVisibleResult,
				objectReference: test.wantObject,
				resultReference: test.wantResult,
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("lookup = %#v, want %#v", got, want)
			}
		})
	}

	for index, record := range records[1:] {
		classification, err := domain.ClassifySourceSubmission(records[0].fingerprint, record.fingerprint)
		if err != nil {
			t.Fatalf("classify cross-scope source %d: %v", index+1, err)
		}
		if classification != domain.SourceDistinct {
			t.Fatalf("cross-scope source %d classification = %v, want distinct", index+1, classification)
		}
	}
}

func TestSyntheticUnauthorizedLookupUsesUniformUnavailableSemantics(t *testing.T) {
	index := newSyntheticSourceIndex()
	record := syntheticSourceRecordFor(
		t,
		"tenant-1",
		"customer-1",
		"api",
		"request-1",
		"digest-1",
		"parcel-1",
		"result-1",
		"scope-1",
	)
	index.put(record)

	ownerScope := syntheticScopeFor(t, "scope-1", "tenant-1", "customer-1")
	absent := sourceFingerprint(t, "tenant-1", "customer-1", "api", "request-missing", "digest-1")
	wantUnavailable := index.lookup(ownerScope, absent)
	if wantUnavailable.disposition != syntheticNotFoundOrNotVisible {
		t.Fatalf("missing lookup = %#v, want unavailable", wantUnavailable)
	}

	unauthorized := []syntheticAuthorizedScope{
		syntheticScopeFor(t, "scope-other-tenant", "tenant-2", "customer-1"),
		syntheticScopeFor(t, "scope-other-customer", "tenant-1", "customer-2"),
	}
	for _, scope := range unauthorized {
		t.Run(scope.reference, func(t *testing.T) {
			got := index.lookup(scope, record.fingerprint)
			if !reflect.DeepEqual(got, wantUnavailable) {
				t.Fatalf("unauthorized lookup = %#v, missing lookup = %#v", got, wantUnavailable)
			}
			if got.objectReference != "" || got.resultReference != "" {
				t.Fatalf("unauthorized lookup leaked references: %#v", got)
			}
		})
	}
}

func TestSyntheticAuthorizedLookupIsLimitedToItsScope(t *testing.T) {
	index := newSyntheticSourceIndex()
	owner := syntheticSourceRecordFor(t, "tenant-1", "customer-1", "api", "request-1", "digest-1", "parcel-1", "result-1", "scope-1")
	otherCustomer := syntheticSourceRecordFor(t, "tenant-1", "customer-2", "api", "request-1", "digest-1", "parcel-2", "result-2", "scope-2")
	index.put(owner)
	index.put(otherCustomer)

	scope := syntheticScopeFor(t, "scope-1", "tenant-1", "customer-1")
	if got := index.lookup(scope, owner.fingerprint); got.disposition != syntheticVisibleResult {
		t.Fatalf("owner lookup = %#v, want visible", got)
	}
	if got := index.lookup(scope, otherCustomer.fingerprint); got.disposition != syntheticNotFoundOrNotVisible {
		t.Fatalf("cross-customer lookup = %#v, want unavailable", got)
	}
}

func TestSyntheticSameIdentityDifferentDigestDoesNotReuseReplayResult(t *testing.T) {
	index := newSyntheticSourceIndex()
	record := syntheticSourceRecordFor(
		t,
		"tenant-1",
		"customer-1",
		"api",
		"request-1",
		"digest-1",
		"parcel-1",
		"result-1",
		"scope-1",
	)
	index.put(record)

	scope := syntheticScopeFor(t, "scope-1", "tenant-1", "customer-1")
	incoming := sourceFingerprint(t, "tenant-1", "customer-1", "api", "request-1", "digest-2")
	got := index.lookup(scope, incoming)
	if got.disposition != syntheticConflictResult || got.objectReference != "" || got.resultReference != "" {
		t.Fatalf("different digest lookup = %#v, want conflict without replay references", got)
	}
}

func TestSyntheticAuditEvidenceUsesOnlyMinimumFields(t *testing.T) {
	record := syntheticSourceRecordFor(
		t,
		"tenant-1",
		"customer-1",
		"api",
		"request-1",
		"digest-1",
		"parcel-1",
		"result-1",
		"scope-1",
	)
	raw, err := json.Marshal(record.audit)
	if err != nil {
		t.Fatalf("marshal audit evidence: %v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode audit evidence: %v", err)
	}
	wantFields := map[string]struct{}{
		"scopeRef":       {},
		"objectRef":      {},
		"resultCategory": {},
		"version":        {},
		"evaluatedAt":    {},
		"evidenceIndex":  {},
	}
	actualFields := make(map[string]struct{}, len(fields))
	for field := range fields {
		actualFields[field] = struct{}{}
	}
	if !reflect.DeepEqual(actualFields, wantFields) {
		actual := make([]string, 0, len(actualFields))
		for field := range fields {
			actual = append(actual, field)
		}
		t.Fatalf("audit fields = %v, want only %v", actual, wantFields)
	}
	if strings.Contains(string(raw), record.confidentialPayload) ||
		strings.Contains(string(raw), "customer-name") ||
		strings.Contains(string(raw), "address") ||
		strings.Contains(string(raw), "goods") {
		t.Fatalf("audit evidence leaked confidential source content: %s", raw)
	}
}
