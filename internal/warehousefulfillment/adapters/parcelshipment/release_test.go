package parcelshipment

import (
	"context"
	"testing"
	"time"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	warehouseapp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

type requestDouble struct {
	request psdomain.ShipmentRequest
	found   bool
}

func (double requestDouble) FindBySourceIdentity(context.Context, psdomain.SourceIdentity) (psdomain.ShipmentRequest, bool, error) {
	return double.request, double.found, nil
}

type withdrawDouble struct {
	called  int
	command psapplication.WithdrawShipmentRequestCommand
	outcome string
	state   string
	pending string
}

func (double *withdrawDouble) answer(_ context.Context, command psapplication.WithdrawShipmentRequestCommand) (string, string, string, error) {
	double.called++
	double.command = command
	return double.outcome, double.state, double.pending, nil
}

func shipmentAt(t *testing.T, state psdomain.ShipmentRequestState) psdomain.ShipmentRequest {
	t.Helper()
	at := time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC)
	version, err := psdomain.NewSubmissionVersionID("version-1")
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := psdomain.NewSourceSubmissionFingerprint(
		mustIdentity(t),
		mustDigest(t, "sha256:a"),
		at, at.Add(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	task, err := psdomain.NewAcceptanceDecisionTaskID("task-1")
	if err != nil {
		t.Fatal(err)
	}
	parcel, err := psdomain.NewDeclaredParcelID("parcel-1")
	if err != nil {
		t.Fatal(err)
	}
	shipment, err := psdomain.NewShipmentRequestID("SHR-1")
	if err != nil {
		t.Fatal(err)
	}
	batch, err := psdomain.NewSubmissionBatchID("batch-1")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := psdomain.RehydrateShipmentRequestSpec{
		Revision: 1, ShipmentRequestID: shipment, BatchID: batch, State: state, SubmittedAt: at,
		CurrentVersion: psdomain.RehydrateSubmissionVersionSpec{
			VersionID: version, SourceSubmission: fingerprint, DeclaredParcelIDs: []psdomain.DeclaredParcelID{parcel}, EstablishedAt: at,
		},
		AcceptanceTask: psdomain.RehydrateAcceptanceTaskSpec{
			TaskID: task, SubmissionVersionID: version, EstablishedAt: at, State: psdomain.AcceptanceTaskRunning,
		},
	}
	if state == psdomain.ShipmentRequestWithdrawn {
		decision, err := psdomain.NewAcceptanceDecisionID("decision-1")
		if err != nil {
			t.Fatal(err)
		}
		snapshot.DecisionFormed = true
		snapshot.AcceptanceTask.State = psdomain.AcceptanceTaskStopped
		snapshot.Withdrawal = psdomain.RehydrateWithdrawalSpec{
			DecisionID: decision,
			Authority:  mustText(t, psdomain.NewWithdrawalAuthorityReference, "grant/1"),
			Requester:  mustText(t, psdomain.NewWithdrawalRequesterReference, psdomain.WarehouseFulfillmentRequester),
			Reason:     mustText(t, psdomain.NewWithdrawalReasonReference, psdomain.OutboundCutoffReason),
			DecidedAt:  at,
		}
	}
	request, err := psdomain.RehydrateShipmentRequest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func mustIdentity(t *testing.T) psdomain.SourceIdentity {
	t.Helper()
	identity, err := warehouseOutboundIdentity("SYN-TENANT-01", "SYN-OWNER-01", "OBR-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func mustDigest(t *testing.T, value string) psdomain.PayloadDigest {
	t.Helper()
	digest, err := psdomain.NewPayloadDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func mustText[T interface{ String() string }](t *testing.T, construct func(string) (T, error), value string) T {
	t.Helper()
	got, err := construct(value)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func releaserOf(requests requestDouble, withdraw *withdrawDouble, granted bool) *Releaser {
	return &Releaser{
		agency: allowanceDouble{granted: granted}, requests: requests, withdraw: withdraw.answer,
		clock: frozenClock{at: time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC)},
	}
}

func TestAnUngrantedCutoffDoesNotWithdraw(t *testing.T) {
	withdraw := &withdrawDouble{outcome: psapplication.WithdrawalFormed.String()}
	releaser := releaserOf(requestDouble{request: shipmentAt(t, psdomain.ShipmentRequestSubmitted), found: true}, withdraw, false)
	word, err := releaser.Release(t.Context(), "SYN-TENANT-01", "SYN-OWNER-01", "SHR-1", "OBR-1", 1)
	if err != nil || word != warehouseapp.ShipmentNotConfigured || withdraw.called != 0 {
		t.Fatalf("word %s err %v called %d", word, err, withdraw.called)
	}
}

func TestAGrantedCutoffWithdrawsASubmittedShipmentAndLeavesADecidedOne(t *testing.T) {
	withdraw := &withdrawDouble{outcome: psapplication.WithdrawalFormed.String(), state: psdomain.ShipmentRequestWithdrawn.String()}
	releaser := releaserOf(requestDouble{request: shipmentAt(t, psdomain.ShipmentRequestSubmitted), found: true}, withdraw, true)
	word, err := releaser.Release(t.Context(), "SYN-TENANT-01", "SYN-OWNER-01", "SHR-1", "OBR-1", 1)
	if err != nil || word != warehouseapp.ShipmentReleased || withdraw.called != 1 {
		t.Fatalf("word %s err %v called %d", word, err, withdraw.called)
	}
	if withdraw.command.Requester.String() != psdomain.WarehouseFulfillmentRequester || withdraw.command.WithdrawalIdentity.RequestKey().String() != "warehouse-cutoff/SHR-1" {
		t.Fatalf("requester %s key %s", withdraw.command.Requester, withdraw.command.WithdrawalIdentity.RequestKey())
	}
	withdraw.outcome = psapplication.WithdrawalNotAuthorized.String()
	refused, err := releaser.Release(t.Context(), "SYN-TENANT-01", "SYN-OWNER-01", "SHR-1", "OBR-1", 1)
	if err != nil || refused != warehouseapp.ShipmentRefused {
		t.Fatalf("refused %s err %v", refused, err)
	}
	held := releaserOf(requestDouble{request: shipmentAt(t, psdomain.ShipmentRequestWithdrawn), found: true}, withdraw, true)
	before := withdraw.called
	released, err := held.Release(t.Context(), "SYN-TENANT-01", "SYN-OWNER-01", "SHR-1", "OBR-1", 1)
	if err != nil || released != warehouseapp.ShipmentReleased || withdraw.called != before {
		t.Fatalf("already withdrawn %s err %v called %d", released, err, withdraw.called)
	}
	accepted, err := releaseWord(psapplication.WithdrawalDecisionAlreadyFormed.String(), psdomain.ShipmentRequestAccepted.String(), "")
	if err != nil || accepted != warehouseapp.ShipmentNotConfigured {
		t.Fatalf("accepted %s err %v", accepted, err)
	}
}
