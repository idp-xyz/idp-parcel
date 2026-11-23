package parcelshipment

import (
	"context"
	"testing"
	"time"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
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
	word, err := releaser.Release(t.Context(), "SYN-TENANT-01", "SYN-OWNER-01", "SHR-1", "parcel-1", "OBR-1", 1)
	if err != nil || word != warehouseapp.ShipmentNotConfigured || withdraw.called != 0 {
		t.Fatalf("word %s err %v called %d", word, err, withdraw.called)
	}
}

func TestAGrantedCutoffWithdrawsASubmittedShipmentAndLeavesADecidedOne(t *testing.T) {
	withdraw := &withdrawDouble{outcome: psapplication.WithdrawalFormed.String(), state: psdomain.ShipmentRequestWithdrawn.String()}
	releaser := releaserOf(requestDouble{request: shipmentAt(t, psdomain.ShipmentRequestSubmitted), found: true}, withdraw, true)
	word, err := releaser.Release(t.Context(), "SYN-TENANT-01", "SYN-OWNER-01", "SHR-1", "parcel-1", "OBR-1", 1)
	if err != nil || word != warehouseapp.ShipmentReleased || withdraw.called != 1 {
		t.Fatalf("word %s err %v called %d", word, err, withdraw.called)
	}
	if withdraw.command.Requester.String() != psdomain.WarehouseFulfillmentRequester || withdraw.command.WithdrawalIdentity.RequestKey().String() != "warehouse-cutoff/SHR-1" {
		t.Fatalf("requester %s key %s", withdraw.command.Requester, withdraw.command.WithdrawalIdentity.RequestKey())
	}
	withdraw.outcome = psapplication.WithdrawalNotAuthorized.String()
	refused, err := releaser.Release(t.Context(), "SYN-TENANT-01", "SYN-OWNER-01", "SHR-1", "parcel-1", "OBR-1", 1)
	if err != nil || refused != warehouseapp.ShipmentRefused {
		t.Fatalf("refused %s err %v", refused, err)
	}
	held := releaserOf(requestDouble{request: shipmentAt(t, psdomain.ShipmentRequestWithdrawn), found: true}, withdraw, true)
	before := withdraw.called
	released, err := held.Release(t.Context(), "SYN-TENANT-01", "SYN-OWNER-01", "SHR-1", "parcel-1", "OBR-1", 1)
	if err != nil || released != warehouseapp.ShipmentReleased || withdraw.called != before {
		t.Fatalf("already withdrawn %s err %v called %d", released, err, withdraw.called)
	}
	accepted, err := releaseWord(psapplication.WithdrawalDecisionAlreadyFormed.String(), psdomain.ShipmentRequestAccepted.String(), "")
	if err != nil || accepted != warehouseapp.ShipmentNotConfigured {
		t.Fatalf("accepted %s err %v", accepted, err)
	}
}

type cancelDouble struct {
	called    int
	command   psapplication.CancelParcelCommand
	outcome   string
	kind      string
	undecided string
}

func (double *cancelDouble) answer(_ context.Context, command psapplication.CancelParcelCommand) (string, string, string, error) {
	double.called++
	double.command = command
	return double.outcome, double.kind, double.undecided, nil
}

// Covers: 已接受的委托走现有的包裹取消。成立才放行；拒绝、待处置与既有终局不放行；授权没登记答未配置。缺包裹标识不调用取消。
func TestAnAcceptedShipmentUsesParcelCancellation(t *testing.T) {
	withdraw := &withdrawDouble{
		outcome: psapplication.WithdrawalDecisionAlreadyFormed.String(),
		state:   psdomain.ShipmentRequestAccepted.String(),
	}
	cancel := &cancelDouble{outcome: psapplication.ParcelCancelled.String(), kind: psports.RecordParcelCancelled.String()}
	releaser := releaserOf(requestDouble{request: shipmentAt(t, psdomain.ShipmentRequestSubmitted), found: true}, withdraw, true)
	releaser.cancel = cancel.answer
	word, err := releaser.Release(t.Context(), "SYN-TENANT-01", "SYN-OWNER-01", "SHR-1", "parcel-1", "OBR-1", 1)
	if err != nil || word != warehouseapp.ShipmentReleased || cancel.called != 1 || withdraw.called != 1 {
		t.Fatalf("word %s err %v cancel %d withdraw %d", word, err, cancel.called, withdraw.called)
	}
	owner, err := psdomain.NewCustomerAccountID("SYN-OWNER-01")
	if err != nil {
		t.Fatal(err)
	}
	if !cancel.command.Requester.IsCustomerAccount(owner) || cancel.command.Reason.String() != psdomain.OutboundCutoffReason ||
		cancel.command.Parcel.String() != "parcel-1" || cancel.command.ShipmentRequestID.String() != "SHR-1" ||
		cancel.command.Identity.RequestKey().String() != "OBR-1/1" || !cancel.command.RequestedAt.Equal(time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC)) {
		t.Fatalf("command requester %s reason %s parcel %s key %s at %s", cancel.command.Requester, cancel.command.Reason, cancel.command.Parcel, cancel.command.Identity.RequestKey(), cancel.command.RequestedAt)
	}
	cancel.outcome = psapplication.CancellationRefused.String()
	refused, err := releaser.Release(t.Context(), "SYN-TENANT-01", "SYN-OWNER-01", "SHR-1", "parcel-1", "OBR-1", 1)
	if err != nil || refused != warehouseapp.ShipmentRefused {
		t.Fatalf("refused %s err %v", refused, err)
	}
	cancel.outcome = psapplication.CancellationUndecided.String()
	cancel.undecided = psapplication.CancelAuthorityUnconfigured.String()
	unconfigured, err := releaser.Release(t.Context(), "SYN-TENANT-01", "SYN-OWNER-01", "SHR-1", "parcel-1", "OBR-1", 1)
	if err != nil || unconfigured != warehouseapp.ShipmentNotConfigured {
		t.Fatalf("unconfigured %s err %v", unconfigured, err)
	}
	before := cancel.called
	if _, err := releaser.Release(t.Context(), "SYN-TENANT-01", "SYN-OWNER-01", "SHR-1", "", "OBR-1", 1); err == nil || cancel.called != before {
		t.Fatalf("缺包裹标识 err %v called %d", err, cancel.called)
	}
	word, err = cancellationWord(psapplication.CancellationExistingResult.String(), psports.RecordParcelCancelled.String(), "")
	if err != nil || word != warehouseapp.ShipmentReleased {
		t.Fatalf("replay %s err %v", word, err)
	}
	word, err = cancellationWord(psapplication.DispositionPending.String(), "", "")
	if err != nil || word != warehouseapp.ShipmentRefused {
		t.Fatalf("pending %s err %v", word, err)
	}
	unwired := releaserOf(requestDouble{request: shipmentAt(t, psdomain.ShipmentRequestSubmitted), found: true}, withdraw, true)
	held, err := unwired.Release(t.Context(), "SYN-TENANT-01", "SYN-OWNER-01", "SHR-1", "parcel-1", "OBR-1", 1)
	if err != nil || held != warehouseapp.ShipmentNotConfigured {
		t.Fatalf("没接上取消 %s err %v", held, err)
	}
}
