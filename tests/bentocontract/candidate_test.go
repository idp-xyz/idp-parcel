package bentocontract_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopostgres "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"
	"go.idp.xyz/idp-bento-go/testkit"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/tests/bentocontract"
)

var contractTime = time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)

// PBC-01: the contract compiles against the exact immutable candidate resolved
// through the official module path, with no replace directive and no source
// copy.
func TestPBC01CandidateIdentityMatchesTheBuild(t *testing.T) {
	output, err := exec.Command("go", "list", "-m", "-json", bentocontract.ModulePath).CombinedOutput()
	if err != nil {
		t.Fatalf("resolve candidate module: %v\n%s", err, output)
	}

	var resolved struct {
		Path    string
		Version string
		Replace *struct {
			Path string
			Dir  string
		}
	}
	if err := json.Unmarshal(output, &resolved); err != nil {
		t.Fatalf("decode module identity: %v", err)
	}

	if resolved.Path != bentocontract.ModulePath {
		t.Fatalf("candidate must resolve through the official module path\n got: %s\nwant: %s",
			resolved.Path, bentocontract.ModulePath)
	}
	if resolved.Replace != nil {
		t.Fatalf("the candidate must not be replaced, got %s => %s",
			resolved.Path, resolved.Replace.Path)
	}
	if resolved.Version != bentocontract.Version {
		t.Fatalf("candidate version mismatch\n got: %s\nwant: %s",
			resolved.Version, bentocontract.Version)
	}

	sums := recordedSums(t)
	moduleKey := bentocontract.ModulePath + " " + bentocontract.Version
	if got := sums[moduleKey]; got != bentocontract.ModuleSum {
		t.Fatalf("candidate checksum mismatch\n got: %s\nwant: %s", got, bentocontract.ModuleSum)
	}
	if got := sums[moduleKey+"/go.mod"]; got != bentocontract.GoModSum {
		t.Fatalf("candidate go.mod checksum mismatch\n got: %s\nwant: %s", got, bentocontract.GoModSum)
	}
}

func recordedSums(t *testing.T) map[string]string {
	t.Helper()

	content, err := os.ReadFile("../../go.sum")
	if err != nil {
		t.Fatalf("read go.sum: %v", err)
	}

	sums := make(map[string]string)
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		sums[fields[0]+" "+fields[1]] = fields[2]
	}
	return sums
}

func newFrameworkDB(t *testing.T) *bentopostgres.DB {
	t.Helper()

	db, err := bentopostgres.NewDB(pgtest.Pool(t))
	if err != nil {
		t.Fatalf("build framework db: %v", err)
	}
	if err := db.CheckSchema(t.Context()); err != nil {
		t.Fatalf("framework schema check: %v", err)
	}
	return db
}

// parcelEnvelope builds an envelope with the authoritative Parcel scope and
// partition key. The framework never completes either from a context.
func parcelEnvelope(id eventing.EventID, subject, partitionKey string, recordedAfter time.Duration) eventing.Envelope {
	recordedAt := contractTime.Add(recordedAfter)
	return eventing.Envelope{
		SpecVersion:   eventing.SpecVersionV1,
		ID:            id,
		Source:        application.EventSource,
		Type:          "idp.parcel.shipment-request.submitted",
		Version:       1,
		Scope:         "tenant-1/customer-1",
		Subject:       subject,
		PartitionKey:  partitionKey,
		OccurredAt:    recordedAt.Add(-time.Second),
		RecordedAt:    recordedAt,
		CorrelationID: "intake-0001",
		ContentType:   "application/json",
		Payload:       json.RawMessage(`{"shipmentRequestId":"` + subject + `"}`),
	}
}

// PBC-05: the Envelope v1 behaviour Parcel depends on. Parcel fills the
// authoritative scope and partition key; a missing one is invalid.
func TestPBC05EnvelopeV1BehaviourIsStable(t *testing.T) {
	envelope := parcelEnvelope("01J000000000000000000PCL1", "request-1", "tenant-1/customer-1/request-1", 0)

	if err := envelope.Validate(); err != nil {
		t.Fatalf("a complete Parcel envelope must be valid: %v", err)
	}

	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	var decoded eventing.Envelope
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if !decoded.OccurredAt.Equal(envelope.OccurredAt) {
		t.Fatalf("the source occurrence time must not be replaced, got %s", decoded.OccurredAt)
	}

	for name, mutate := range map[string]func(*eventing.Envelope){
		"missing scope":         func(e *eventing.Envelope) { e.Scope = "" },
		"missing partition key": func(e *eventing.Envelope) { e.PartitionKey = "" },
		"unknown spec version":  func(e *eventing.Envelope) { e.SpecVersion = "9.9" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := envelope.Clone()
			mutate(&invalid)
			if err := invalid.Validate(); !errors.Is(err, eventing.ErrInvalidEnvelope) {
				t.Fatalf("expected an invalid envelope error, got %v", err)
			}
		})
	}
}

// PBC-06: the framework migration assets render by checksum and the framework
// Outbox and Inbox contracts hold on the Parcel migrated PostgreSQL 16 schema.
func TestPBC06FrameworkContractsOnParcelSchema(t *testing.T) {
	t.Run("outbox", func(t *testing.T) {
		testkit.RunOutboxContract(t, func(t *testing.T) testkit.OutboxContractScenario {
			db := newFrameworkDB(t)
			store, err := outbox.NewStore(db)
			if err != nil {
				t.Fatalf("build outbox store: %v", err)
			}
			return testkit.OutboxContractScenario{
				Transactor: db.Transactor(),
				Writer:     store,
				Claimer:    store,
				Finalizer:  store,
				First: parcelEnvelope("01J000000000000000000PCL1", "request-1",
					"tenant-1/customer-1/request-1", 0),
				SamePartition: parcelEnvelope("01J000000000000000000PCL2", "request-1",
					"tenant-1/customer-1/request-1", time.Second),
				OtherPartition: parcelEnvelope("01J000000000000000000PCL3", "request-2",
					"tenant-1/customer-1/request-2", 2*time.Second),
				Now:      contractTime.Add(time.Minute),
				LeaseFor: 30 * time.Second,
			}
		})
	})

	t.Run("inbox", func(t *testing.T) {
		testkit.RunInboxContract(t, func(t *testing.T) testkit.InboxContractScenario {
			db := newFrameworkDB(t)
			store, err := inbox.NewStore(db)
			if err != nil {
				t.Fatalf("build inbox store: %v", err)
			}
			return testkit.InboxContractScenario{
				Transactor: db.Transactor(),
				Starter:    store,
				Completer:  store,
				Key: eventing.InboxKey{
					Consumer: "go.idp.xyz/idp-parcel/network-routing",
					Source:   application.EventSource,
					EventID:  "01J000000000000000000PCL1",
				},
				Now: contractTime,
			}
		})
	})
}

// PBC-08: no production package imports the framework testkit, and a business
// write cannot reach the database outside a transaction.
func TestPBC08ProductionGraphAndTransactionBoundary(t *testing.T) {
	const testkitPath = "go.idp.xyz/idp-bento-go/testkit"

	output, err := exec.Command("go", "list", "-deps", "../../cmd/...").CombinedOutput()
	if err != nil {
		t.Fatalf("list production dependencies: %v\n%s", err, output)
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.TrimSpace(line) == testkitPath {
			t.Fatalf("a production binary depends on %s", testkitPath)
		}
	}

	db := newFrameworkDB(t)
	if _, err := db.RequireExecutor(t.Context()); !errors.Is(err, bentopostgres.ErrTransactionRequired) {
		t.Fatalf("a write executor must not be available outside a transaction, got %v", err)
	}

	var insideTransaction bool
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if _, err := db.RequireExecutor(ctx); err != nil {
			return err
		}
		insideTransaction = true
		return nil
	}); err != nil {
		t.Fatalf("within transaction: %v", err)
	}
	if !insideTransaction {
		t.Fatal("the transaction body must have run")
	}
}
