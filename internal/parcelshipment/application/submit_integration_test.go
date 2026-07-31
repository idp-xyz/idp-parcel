package application_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	bentopostgres "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"
	"go.idp.xyz/idp-bento-go/testkit"

	parcelpostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

var submittedAt = time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)

type harness struct {
	handler *application.SubmitHandler
	sources ports.SourceSubmissionRepository
	batches ports.SubmissionBatchRepository
	db      *bentopostgres.DB
	pool    *pgxpool.Pool
}

func newHarness(t *testing.T, eventIDs ...string) harness {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopostgres.NewDB(pool)
	if err != nil {
		t.Fatalf("build framework db: %v", err)
	}
	if err := db.CheckSchema(t.Context()); err != nil {
		t.Fatalf("framework schema check: %v", err)
	}

	sources, err := parcelpostgres.NewSourceSubmissionRepository(db)
	if err != nil {
		t.Fatalf("build source repository: %v", err)
	}
	requests, err := parcelpostgres.NewShipmentRequestRepository(db)
	if err != nil {
		t.Fatalf("build shipment request repository: %v", err)
	}
	batches, err := parcelpostgres.NewSubmissionBatchRepository(db)
	if err != nil {
		t.Fatalf("build batch repository: %v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("build outbox store: %v", err)
	}

	if len(eventIDs) == 0 {
		eventIDs = []string{"event-1", "event-2", "event-3", "event-4"}
	}
	clock := testkit.NewFixedClock(submittedAt)

	handler, err := application.NewSubmitHandler(
		db.Transactor(), sources, requests, batches, store,
		testkit.NewSequenceIDGenerator(eventIDs...), clock)
	if err != nil {
		t.Fatalf("build submit handler: %v", err)
	}
	return harness{handler: handler, sources: sources, batches: batches, db: db, pool: pool}
}

func scope() domain.Scope {
	return domain.Scope{TenantID: "tenant-1", CustomerAccountID: "customer-1"}
}

func sourceKey() domain.SourceKey {
	return domain.SourceKey{
		Scope:            scope(),
		Source:           "anchor-portal",
		SourceRequestKey: "intake-0001",
	}
}

func command() application.SubmitCommand {
	return application.SubmitCommand{
		SourceKey:         sourceKey(),
		RawContentRef:     "s3://parcel-intake/raw/intake-0001",
		PayloadDigest:     "sha256:aaaa",
		SubmissionBatchID: "batch-1",
		Requests: []application.DeclaredShipmentRequest{{
			ShipmentRequestID: "request-1",
			DeclaredParcels: []domain.DeclaredParcel{
				{ID: "parcel-1", CustomerReference: "CUST-P1"},
				{ID: "parcel-2", CustomerReference: "CUST-P2"},
			},
		}},
		SourceOccurredAt: submittedAt.Add(-time.Minute),
		SystemReceivedAt: submittedAt.Add(-30 * time.Second),
		CorrelationID:    "corr-1",
		ParseRuleVersion: "intake-rules-1",
	}
}

func TestSourcePreservationAndSubmittedRequestCommitTogether(t *testing.T) {
	h := newHarness(t)

	response, err := h.handler.Handle(t.Context(), command())
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if response.Result != application.ResultSubmitted {
		t.Fatalf("expected SUBMITTED, got %s", response.Result)
	}
	if len(response.Requests) != 1 {
		t.Fatalf("expected one request result, got %d", len(response.Requests))
	}
	result := response.Requests[0]
	if result.State != domain.StateSubmitted {
		t.Fatalf("this slice must only produce SUBMITTED, got %s", result.State)
	}
	if result.SubmissionVersion != 1 || result.EventID == "" {
		t.Fatalf("unexpected request result: %+v", result)
	}

	preserved, err := h.sources.Find(t.Context(), sourceKey())
	if err != nil {
		t.Fatalf("read preserved submission: %v", err)
	}
	if preserved.Outcome() != domain.IntakeAccepted {
		t.Fatalf("expected an accepted intake outcome, got %s", preserved.Outcome())
	}
	if !preserved.OccurredAt().Equal(submittedAt.Add(-time.Minute)) {
		t.Fatalf("the source occurrence time must be preserved, got %s", preserved.OccurredAt())
	}

	assertOutboxRows(t, h.pool, 1)
	assertNoDecision(t, h.pool)
}

func TestSameKeySameDigestReturnsTheExistingResult(t *testing.T) {
	h := newHarness(t)

	first, err := h.handler.Handle(t.Context(), command())
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}

	second, err := h.handler.Handle(t.Context(), command())
	if err != nil {
		t.Fatalf("repeat submit: %v", err)
	}

	if second.Result != application.ResultExisting {
		t.Fatalf("a repeat must return the existing result, got %s", second.Result)
	}
	if len(second.Requests) != 1 {
		t.Fatalf("expected one existing request, got %d", len(second.Requests))
	}
	if second.Requests[0].EventID != first.Requests[0].EventID {
		t.Fatalf("a repeat must reuse the original event id, got %s want %s",
			second.Requests[0].EventID, first.Requests[0].EventID)
	}

	assertShipmentRequestCount(t, h.pool, 1)
	assertOutboxRows(t, h.pool, 1)
}

func TestSameKeyDifferentDigestIsAnIntakeConflict(t *testing.T) {
	h := newHarness(t)

	if _, err := h.handler.Handle(t.Context(), command()); err != nil {
		t.Fatalf("first submit: %v", err)
	}

	conflicting := command()
	conflicting.PayloadDigest = "sha256:bbbb"
	conflicting.RawContentRef = "s3://parcel-intake/raw/intake-0001-v2"
	conflicting.Requests[0].ShipmentRequestID = "request-2"

	response, err := h.handler.Handle(t.Context(), conflicting)
	if err != nil {
		t.Fatalf("conflicting submit: %v", err)
	}
	if response.Result != application.ResultIntakeConflict {
		t.Fatalf("expected INTAKE_CONFLICT, got %s", response.Result)
	}

	preserved, err := h.sources.Find(t.Context(), sourceKey())
	if err != nil {
		t.Fatalf("read preserved submission: %v", err)
	}
	if preserved.PayloadDigest() != "sha256:aaaa" {
		t.Fatalf("the original content must not be overwritten, digest is %s", preserved.PayloadDigest())
	}
	if preserved.RawContentRef() != "s3://parcel-intake/raw/intake-0001" {
		t.Fatalf("the original raw content reference must not be overwritten, got %s",
			preserved.RawContentRef())
	}
	assertShipmentRequestCount(t, h.pool, 1)
}

func TestTheSameExternalKeyInAnotherScopeStaysIsolated(t *testing.T) {
	h := newHarness(t)

	if _, err := h.handler.Handle(t.Context(), command()); err != nil {
		t.Fatalf("first submit: %v", err)
	}

	other := command()
	other.SourceKey.Scope.CustomerAccountID = "customer-2"
	other.Requests[0].ShipmentRequestID = "request-2"

	response, err := h.handler.Handle(t.Context(), other)
	if err != nil {
		t.Fatalf("submit in another customer account: %v", err)
	}
	if response.Result != application.ResultSubmitted {
		t.Fatalf("the same external key in another scope must be a first submission, got %s",
			response.Result)
	}
	assertShipmentRequestCount(t, h.pool, 2)
}

func TestNotAdmittedIntakeCreatesNoPlaceholderRequest(t *testing.T) {
	h := newHarness(t)

	notAdmitted := command()
	notAdmitted.Requests = nil
	notAdmitted.NotAdmittedReason = "customer account could not be resolved from the raw content"

	response, err := h.handler.Handle(t.Context(), notAdmitted)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if response.Result != application.ResultNotAdmitted {
		t.Fatalf("expected NOT_ADMITTED, got %s", response.Result)
	}

	preserved, err := h.sources.Find(t.Context(), sourceKey())
	if err != nil {
		t.Fatalf("the raw submission must still be preserved: %v", err)
	}
	if preserved.NotAdmittedReason() == "" {
		t.Fatal("a not admitted intake must record its deterministic reason")
	}
	assertShipmentRequestCount(t, h.pool, 0)
	assertOutboxRows(t, h.pool, 0)
	assertNoDecision(t, h.pool)
}

func TestOneFailingRequestDoesNotRollBackItsSibling(t *testing.T) {
	h := newHarness(t)

	batch := command()
	batch.Requests = []application.DeclaredShipmentRequest{
		{
			ShipmentRequestID: "request-1",
			DeclaredParcels:   []domain.DeclaredParcel{{ID: "parcel-1"}},
		},
		{
			// No declared parcel: the domain refuses to build this request.
			ShipmentRequestID: "request-2",
		},
		{
			ShipmentRequestID: "request-3",
			DeclaredParcels:   []domain.DeclaredParcel{{ID: "parcel-3"}},
		},
	}

	response, err := h.handler.Handle(t.Context(), batch)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if response.Result != application.ResultSubmitted {
		t.Fatalf("expected the batch to report SUBMITTED, got %s", response.Result)
	}

	var undecided int
	for _, result := range response.Requests {
		if result.Undecided {
			undecided++
			if result.ShipmentRequestID != "request-2" {
				t.Fatalf("unexpected undecided request %s", result.ShipmentRequestID)
			}
		}
	}
	if undecided != 1 {
		t.Fatalf("expected exactly one undecided request, got %d", undecided)
	}

	assertShipmentRequestCount(t, h.pool, 2)
	assertOutboxRows(t, h.pool, 2)
}

func TestBusinessWriteAndOutboxIntentRollBackTogether(t *testing.T) {
	h := newHarness(t)

	if _, err := h.handler.Handle(t.Context(), command()); err != nil {
		t.Fatalf("first submit: %v", err)
	}

	// Reusing the same shipment request id under a new source key forces the
	// insert to fail after the batch row exists.
	duplicate := command()
	duplicate.SourceKey.SourceRequestKey = "intake-0002"
	duplicate.SubmissionBatchID = "batch-2"

	response, err := h.handler.Handle(t.Context(), duplicate)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if response.Result != application.ResultUndecided {
		t.Fatalf("a failed insert must not report SUBMITTED, got %s", response.Result)
	}

	// The failed transaction left neither a second request nor a second outbox
	// row behind.
	assertShipmentRequestCount(t, h.pool, 1)
	assertOutboxRows(t, h.pool, 1)
}

func TestEnqueuedEnvelopeCarriesTheAuthoritativeScopeAndMinimalPayload(t *testing.T) {
	h := newHarness(t)

	if _, err := h.handler.Handle(t.Context(), command()); err != nil {
		t.Fatalf("handle: %v", err)
	}

	var (
		source       string
		eventType    string
		scopeValue   string
		subject      string
		partitionKey string
		payload      []byte
		occurredAt   time.Time
	)
	err := h.pool.QueryRow(t.Context(), `
		SELECT source, event_type, scope, subject, partition_key, payload, occurred_at
		FROM bento.outbox`).
		Scan(&source, &eventType, &scopeValue, &subject, &partitionKey, &payload, &occurredAt)
	if err != nil {
		t.Fatalf("read outbox row: %v", err)
	}

	if source != application.EventSource {
		t.Fatalf("unexpected event source %q", source)
	}
	if eventType != string(domain.EventTypeShipmentRequestSubmitted) {
		t.Fatalf("unexpected event type %q", eventType)
	}
	if scopeValue != "tenant-1/customer-1" {
		t.Fatalf("the envelope must carry the authoritative scope, got %q", scopeValue)
	}
	if subject != "request-1" || partitionKey != "tenant-1/customer-1/request-1" {
		t.Fatalf("unexpected subject %q or partition key %q", subject, partitionKey)
	}
	if !occurredAt.Equal(submittedAt.Add(-time.Minute)) {
		t.Fatalf("the domain occurrence time must not be replaced by the write time, got %s", occurredAt)
	}

	for _, forbidden := range []string{"address", "contact", "goods", "declaration", "CUST-P1"} {
		if containsFold(string(payload), forbidden) {
			t.Fatalf("the payload must stay minimal, found %q in %s", forbidden, payload)
		}
	}
}

func TestRepositoryWritesRequireAnExplicitTransaction(t *testing.T) {
	h := newHarness(t)

	submission, err := domain.PreserveSource(sourceKey(), "ref", "sha256:aaaa",
		submittedAt, submittedAt, "corr-1")
	if err != nil {
		t.Fatalf("build submission: %v", err)
	}

	// Outside a transaction the framework refuses to hand out a write executor
	// rather than silently falling back to the pool.
	if err := h.sources.Preserve(t.Context(), submission); !errors.Is(err, bentopostgres.ErrTransactionRequired) {
		t.Fatalf("expected a transaction requirement, got %v", err)
	}
}

func assertShipmentRequestCount(t *testing.T, pool *pgxpool.Pool, want int) {
	t.Helper()

	var got int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM parcel_shipment.shipment_request`).Scan(&got); err != nil {
		t.Fatalf("count shipment requests: %v", err)
	}
	if got != want {
		t.Fatalf("expected %d shipment requests, got %d", want, got)
	}
}

func assertOutboxRows(t *testing.T, pool *pgxpool.Pool, want int) {
	t.Helper()

	var got int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM bento.outbox`).Scan(&got); err != nil {
		t.Fatalf("count outbox rows: %v", err)
	}
	if got != want {
		t.Fatalf("expected %d outbox rows, got %d", want, got)
	}
}

// assertNoDecision proves unimplemented code never produced an acceptance or a
// rejection.
func assertNoDecision(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	var other int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM parcel_shipment.shipment_request WHERE lifecycle_state <> 'SUBMITTED'`).
		Scan(&other); err != nil {
		t.Fatalf("count non submitted requests: %v", err)
	}
	if other != 0 {
		t.Fatalf("this slice must never produce a state other than SUBMITTED, found %d", other)
	}
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
