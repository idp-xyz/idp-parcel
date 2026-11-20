package nodeoperations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/nodeoperations"
	nrparcelshipment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	tfadapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/transportfulfillment"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件钉实测变化到路由复核的翻译：对不上恰好一件已接受包裹就入账不复核；复核未决映射到本路自己的哨兵。

type associationDouble struct {
	parcels []nodomain.ParcelAssociationReference
	err     error
	asked   int
}

func (double *associationDouble) ListHandlingUnits(
	context.Context, nodomain.TenantID, nodomain.ParcelAssociationReference,
) ([]nodomain.HandlingUnitID, error) {
	return nil, nil
}

func (double *associationDouble) ListParcels(
	context.Context, nodomain.TenantID, nodomain.HandlingUnitID,
) ([]nodomain.ParcelAssociationReference, error) {
	double.asked++
	return double.parcels, double.err
}

type acceptedDouble struct {
	accepted nrparcelshipment.AcceptedCarriedGeo
	found    bool
	asked    int
}

func (double *acceptedDouble) LoadAcceptedCarriedGeo(
	context.Context, nrdomain.TenantID, nrdomain.DeclaredParcelID,
) (nrparcelshipment.AcceptedCarriedGeo, bool, error) {
	double.asked++
	return double.accepted, double.found, nil
}

type logDouble struct{}

func (logDouble) FindDigest(context.Context, nrdomain.TenantID, nrdomain.RequestCorrelationID) (string, bool, error) {
	return "", false, nil
}

func (logDouble) Append(context.Context, nrdomain.TenantID, nrdomain.RequestCorrelationID, string) error {
	return nil
}

// stallingStore 在取判断键锁时失败，让复核答未决，用来看适配器把未决映射到哪一只哨兵。
type stallingStore struct {
	locked bool
}

func (store *stallingStore) FindByCorrelation(
	context.Context, nrdomain.TenantID, nrdomain.RequestCorrelationID,
) (nrports.ReassessmentRecord, bool, error) {
	return nrports.ReassessmentRecord{}, false, nil
}

func (store *stallingStore) Save(
	context.Context, nrdomain.RequestCorrelationID, nrports.ReassessmentRecord,
) (nrports.ReassessmentSaveOutcome, error) {
	return 0, errors.New("save")
}

func (store *stallingStore) RecordNotJudged(context.Context, nrports.NotJudgedTrigger) error {
	return nil
}

func (store *stallingStore) LatestTriggerAt(context.Context, nrdomain.InitialRouteJudgmentKey) (time.Time, bool, error) {
	store.locked = true
	return time.Time{}, false, errors.New("store down")
}

func (store *stallingStore) FormedPlans(context.Context, nrdomain.InitialRouteJudgmentKey) ([]nrdomain.InitialRoutePlan, error) {
	return nil, nil
}

func (store *stallingStore) LatestControlPosition(context.Context, nrdomain.InitialRouteJudgmentKey) (nrports.ControlPosition, error) {
	return nrports.ControlPosition{}, nil
}

func (store *stallingStore) AwaitingMeasurementChanges(context.Context, nrdomain.InitialRouteJudgmentKey) ([]nrdomain.RequestCorrelationID, error) {
	return nil, nil
}

func mustAdapter(
	t *testing.T,
	associations noports.CurrentParcelAssociation,
	accepted adapter.AcceptedCarriedGeoLookup,
	handler *nrapplication.ReassessRouteHandler,
) *adapter.ReassessOnMeasurementChangeAdapter {
	t.Helper()
	purpose, err := nrdomain.NewServicePurpose("NETWORK_SERVICE")
	if err != nil {
		t.Fatal(err)
	}
	built, err := adapter.NewReassessOnMeasurementChangeAdapter(associations, accepted, handler, purpose)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func changed(kind, objectID string) nrinbox.ChangedActualMeasurement {
	return nrinbox.ChangedActualMeasurement{
		TenantID:   "tenant-1",
		SourceID:   "src-1",
		ObjectKind: kind,
		ObjectID:   objectID,
		OccurredAt: time.Date(2026, 8, 21, 16, 0, 0, 0, time.UTC),
	}
}

func parcelRef(t *testing.T, raw string) nodomain.ParcelAssociationReference {
	t.Helper()
	ref, err := nodomain.NewParcelAssociationReference(raw)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func acceptedOne(t *testing.T) *acceptedDouble {
	t.Helper()
	customer, err := nrdomain.NewCustomerAccountID("customer-1")
	if err != nil {
		t.Fatal(err)
	}
	request, err := nrdomain.NewShipmentRequestID("request-1")
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := nrdomain.NewAcceptanceBaselineReference("version-1")
	if err != nil {
		t.Fatal(err)
	}
	return &acceptedDouble{found: true, accepted: nrparcelshipment.AcceptedCarriedGeo{
		CustomerAccountID:  customer,
		ShipmentRequestID:  request,
		AcceptanceBaseline: baseline,
		AcceptedAt:         time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC),
		Geo: nrdomain.NewGeoResolutionProjection(
			nrdomain.NewGeoResolutionSide("CN", true, "200000", true),
			nrdomain.NewGeoResolutionSide("SG", true, "018956", true),
		),
	}}
}

// Covers: 集运单元、零件（没有包裹）、多件、不是当前已接受包裹，各自入账收工，不进复核。
func TestAMeasurementChangeThatIsNotExactlyOneAcceptedParcelIsAcknowledged(t *testing.T) {
	store := &stallingStore{}
	handler := nrapplication.NewReassessRouteHandler(nrapplication.ReassessRouteDeps{
		Store: store,
		Log:   logDouble{},
	})
	for name, tc := range map[string]struct {
		kind         string
		parcels      []nodomain.ParcelAssociationReference
		found        bool
		askedUnits   int
		askedParcels int
	}{
		"集运单元": {kind: nodomain.ConsolidationUnitObject.String()},
		"零件":   {kind: nodomain.HandlingUnitObject.String(), askedUnits: 1},
		"多件": {
			kind: nodomain.HandlingUnitObject.String(), askedUnits: 1,
			parcels: []nodomain.ParcelAssociationReference{parcelRef(t, "parcel-1"), parcelRef(t, "parcel-2")},
		},
		"不是已接受包裹": {
			kind: nodomain.HandlingUnitObject.String(), askedUnits: 1, askedParcels: 1,
			parcels: []nodomain.ParcelAssociationReference{parcelRef(t, "parcel-1")},
		},
	} {
		t.Run(name, func(t *testing.T) {
			store.locked = false
			associations := &associationDouble{parcels: tc.parcels}
			accepted := acceptedOne(t)
			accepted.found = tc.found
			subject := mustAdapter(t, associations, accepted, handler)
			if err := subject.HandleChangedActualMeasurement(context.Background(), changed(tc.kind, "unit-1")); err != nil {
				t.Fatalf("入账失败：%v", err)
			}
			if store.locked || associations.asked != tc.askedUnits || accepted.asked != tc.askedParcels {
				t.Fatalf("进了复核 %v、问了关联 %d 次、问了已接受 %d 次，want 不复核、%d、%d",
					store.locked, associations.asked, accepted.asked, tc.askedUnits, tc.askedParcels)
			}
		})
	}
}

// Covers: 复核未决映射到本路自己的哨兵，不与收寄、交接两路合用。
func TestAnUndecidedMeasurementReassessmentUsesItsOwnSentinel(t *testing.T) {
	store := &stallingStore{}
	handler := nrapplication.NewReassessRouteHandler(nrapplication.ReassessRouteDeps{
		Store: store,
		Log:   logDouble{},
	})
	associations := &associationDouble{parcels: []nodomain.ParcelAssociationReference{parcelRef(t, "parcel-1")}}
	subject := mustAdapter(t, associations, acceptedOne(t), handler)
	err := subject.HandleChangedActualMeasurement(context.Background(), changed(nodomain.HandlingUnitObject.String(), "unit-1"))
	if !errors.Is(err, adapter.ErrMeasurementReassessmentUndecided) {
		t.Fatalf("err = %v，want 本路未决哨兵", err)
	}
	if errors.Is(err, nrparcelshipment.ErrReassessmentUndecided) || errors.Is(err, tfadapter.ErrHandoverReassessmentUndecided) {
		t.Fatal("未决哨兵与收寄或交接那一路合用了")
	}
}
