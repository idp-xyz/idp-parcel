package parcelshipment

import (
	"context"
	"testing"
	"time"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

type allowanceDouble struct {
	granted bool
	err     error
}

func (double allowanceDouble) Allow(context.Context, string, string) (bool, error) {
	return double.granted, double.err
}

type shipmentDouble struct {
	called  int
	command psapplication.SubmitShipmentRequestCommand
}

func (double *shipmentDouble) Handle(_ context.Context, command psapplication.SubmitShipmentRequestCommand) (psapplication.SubmitShipmentRequestResult, error) {
	double.called++
	double.command = command
	return psapplication.SubmitShipmentRequestResult{}, nil
}

type ownershipDouble struct{}

func (ownershipDouble) DecideProductionOwnership(_ context.Context, scope psdomain.AdmissionScope) (psdomain.ProductionOwnershipDecision, error) {
	at := time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC)
	validity, err := psdomain.NewOwnershipValidityInterval(at.Add(-time.Hour), at.Add(24*time.Hour))
	if err != nil {
		return psdomain.ProductionOwnershipDecision{}, err
	}
	decisionID, err := psdomain.NewProductionOwnershipDecisionID("SYN-OWN-1")
	if err != nil {
		return psdomain.ProductionOwnershipDecision{}, err
	}
	rule, err := psdomain.NewProductionOwnershipRuleVersion("SYN-RULE-1")
	if err != nil {
		return psdomain.ProductionOwnershipDecision{}, err
	}
	revision, err := psdomain.NewProductionOwnershipRevision("SYN-REV-1")
	if err != nil {
		return psdomain.ProductionOwnershipDecision{}, err
	}
	return psdomain.NewProductionOwnershipDecision(psdomain.ProductionOwnershipDecisionSpec{
		DecisionID: decisionID, Scope: scope, Authority: psdomain.ProductionAuthorityIDPParcel,
		AdmissionControl: psdomain.AdmissionControlOpen, RuleVersion: rule, AsOf: at, Validity: validity,
		Revision: revision, DecisionAt: at,
	})
}

func commissionOf() ports.CommissionSubmission {
	return ports.CommissionSubmission{
		Tenant: "SYN-TENANT-01", Owner: "SYN-OWNER-01", Request: "OBR-1", Sequence: 2,
		Service: "SYN-SERVICE", Recipient: "收件人", Address: "地址",
		Parcel: ports.CommissionParcel{
			Sequence: 2, Weight: "1.2", WeightUnit: "KG", Length: "20", Width: "15", Height: "10", Dimension: "CM",
			Goods: "SYN-GOODS-01", Unit: "件", Quantity: 4,
		},
	}
}

func TestAnUngrantedAgencyDoesNotOpenAShipment(t *testing.T) {
	shipments := &shipmentDouble{}
	opener, err := NewOpener(allowanceDouble{}, shipments, ownershipDouble{}, frozenClock{at: time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	answer, err := opener.Submit(t.Context(), commissionOf())
	if err != nil || answer.Outcome != ports.CommissionNotConfiguredWord || shipments.called != 0 {
		t.Fatalf("answer %s err %v called %d", answer.Outcome, err, shipments.called)
	}
}

func TestAGrantedAgencySubmitsWarehouseOutboundUnderTheParcelKey(t *testing.T) {
	shipments := &shipmentDouble{}
	opener, err := NewOpener(allowanceDouble{granted: true}, shipments, ownershipDouble{}, frozenClock{at: time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opener.Submit(t.Context(), commissionOf()); err != nil {
		t.Fatal(err)
	}
	if shipments.called != 1 {
		t.Fatalf("called %d", shipments.called)
	}
	identity := shipments.command.Identity
	if identity.Source().String() != psdomain.SourceWarehouseOutbound || identity.RequestKey().String() != "OBR-1/2" || identity.CustomerAccountID().String() != "SYN-OWNER-01" {
		t.Fatalf("identity %s %s %s", identity.Source(), identity.RequestKey(), identity.CustomerAccountID())
	}
	if shipments.command.ExpectedRevision.String() != "SYN-REV-1" || len(shipments.command.DeclaredParcelIDs) != 1 || !shipments.command.DeclaredGoods.Declared() {
		t.Fatal("命令没有带上修订、成员或货物明细")
	}
}

func TestAPlatformStockingRoundSubmitsEveryParcelUnderOneKey(t *testing.T) {
	shipments := &shipmentDouble{}
	opener, err := NewOpener(allowanceDouble{granted: true}, shipments, ownershipDouble{}, frozenClock{at: time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	submission := commissionOf()
	submission.Round = 1
	submission.Sequence = 0
	second := submission.Parcel
	second.Sequence = 3
	second.Quantity = 2
	submission.Parcels = []ports.CommissionParcel{submission.Parcel, second}
	answer, err := opener.Submit(t.Context(), submission)
	if err != nil || shipments.called != 1 {
		t.Fatalf("err %v called %d", err, shipments.called)
	}
	if shipments.command.Identity.RequestKey().String() != "OBR-1/round/1" || len(shipments.command.DeclaredParcelIDs) != 2 || len(answer.Members) != 2 {
		t.Fatalf("key %s members %d answer %d", shipments.command.Identity.RequestKey(), len(shipments.command.DeclaredParcelIDs), len(answer.Members))
	}
	if answer.Members[0].Sequence != 2 || answer.Members[1].Sequence != 3 || answer.DeclaredParcelID != "" {
		t.Fatalf("members %+v single %s", answer.Members, answer.DeclaredParcelID)
	}
}

type frozenClock struct{ at time.Time }

func (clock frozenClock) Now() time.Time { return clock.at }

func TestTheOpenerRefusesToStartWithoutItsDependencies(t *testing.T) {
	if _, err := NewOpener(nil, &shipmentDouble{}, ownershipDouble{}, frozenClock{}); err == nil {
		t.Fatal("缺了代办判断也装起来了")
	}
}
