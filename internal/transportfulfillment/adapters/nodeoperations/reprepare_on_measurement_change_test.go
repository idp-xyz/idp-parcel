package nodeoperations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	tfnode "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/nodeoperations"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证实测变化只在作业实物上恰一件包裹时交给再准备，以及编排各格怎样落成入账、重投或响亮失败。夹具全为合成值。

type associationDouble struct {
	parcels []nodomain.ParcelAssociationReference
	err     error
}

func (double associationDouble) ListHandlingUnits(context.Context, nodomain.TenantID, nodomain.ParcelAssociationReference) ([]nodomain.HandlingUnitID, error) {
	return nil, nil
}

func (double associationDouble) ListParcels(context.Context, nodomain.TenantID, nodomain.HandlingUnitID) ([]nodomain.ParcelAssociationReference, error) {
	return double.parcels, double.err
}

type calledPreparation struct {
	calls  int
	object string
}

func (double *calledPreparation) RePrepareOnMeasurementChange(
	_ context.Context, command tfapplication.RePrepareOnMeasurementChangeCommand,
) (tfapplication.RePrepareOnMeasurementChangeResult, error) {
	double.calls++
	double.object = command.Object
	return tfapplication.RePrepareOnMeasurementChangeResult{}, errors.New("called")
}

func changedMeasurement(kind, object string) tfinbox.ChangedMeasurement {
	return tfinbox.ChangedMeasurement{
		TenantID: "SYN-TENANT-01", SourceID: "SYN-MEASURE-1", ObjectKind: kind, ObjectID: object,
	}
}

func parcelRef(t *testing.T, raw string) nodomain.ParcelAssociationReference {
	t.Helper()
	parcel, err := nodomain.NewParcelAssociationReference(raw)
	if err != nil {
		t.Fatalf("包裹：%v", err)
	}
	return parcel
}

// Covers: 集运单元、零件、多件都不交给编排；恰一件才交，对象就是包裹号。
func TestOnlyASingleParcelOnAHandlingUnitIsRePrepared(t *testing.T) {
	one := parcelRef(t, "SYN-PARCEL-01")
	two := parcelRef(t, "SYN-PARCEL-02")
	for _, test := range []struct {
		name string
		kind string
		list associationDouble
	}{
		{"集运单元", nodomain.ConsolidationUnitObject.String(), associationDouble{parcels: []nodomain.ParcelAssociationReference{one}}},
		{"零件", nodomain.HandlingUnitObject.String(), associationDouble{}},
		{"多件", nodomain.HandlingUnitObject.String(), associationDouble{parcels: []nodomain.ParcelAssociationReference{one, two}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			prepare := &calledPreparation{}
			adapter, err := tfnode.NewRePrepareOnMeasurementChange(test.list, prepare)
			if err != nil {
				t.Fatal(err)
			}
			if err := adapter.HandleChangedMeasurement(context.Background(), changedMeasurement(test.kind, "SYN-HU-1")); err != nil || prepare.calls != 0 {
				t.Fatalf("%s = %v 调用 %d", test.name, err, prepare.calls)
			}
		})
	}

	prepare := &calledPreparation{}
	adapter, err := tfnode.NewRePrepareOnMeasurementChange(associationDouble{parcels: []nodomain.ParcelAssociationReference{one}}, prepare)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.HandleChangedMeasurement(context.Background(), changedMeasurement(nodomain.HandlingUnitObject.String(), "SYN-HU-1")); err == nil || prepare.calls != 1 || prepare.object != "SYN-PARCEL-01" {
		t.Fatalf("恰一件 = %v 调用 %d 对象 %q", err, prepare.calls, prepare.object)
	}
}

// Covers: 列包裹读不动重投；租户或作业实物立不成则响亮失败，不进未决名单。
func TestListingParcelsRetriesAndABadIdentityFailsLoudly(t *testing.T) {
	prepare := &calledPreparation{}
	down, err := tfnode.NewRePrepareOnMeasurementChange(associationDouble{err: errors.New("associations down")}, prepare)
	if err != nil {
		t.Fatal(err)
	}
	if err := down.HandleChangedMeasurement(context.Background(), changedMeasurement(nodomain.HandlingUnitObject.String(), "SYN-HU-1")); !errors.Is(err, tfnode.ErrRePreparationUndecided) || prepare.calls != 0 {
		t.Fatalf("读不动 = %v 调用 %d", err, prepare.calls)
	}
	if err := down.HandleChangedMeasurement(context.Background(), changedMeasurement(nodomain.HandlingUnitObject.String(), " ")); err == nil || errors.Is(err, tfnode.ErrRePreparationUndecided) {
		t.Fatalf("作业实物立不成 = %v", err)
	}
}

type reprepareClock struct{}

func (reprepareClock) Now() time.Time { return time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC) }

type reprepareDemand struct{}

func (reprepareDemand) LoadDeclaredDemand(context.Context, tfdomain.TenantID, tfdomain.CarriedObjectReference) (tfdomain.DemandSnapshot, error) {
	return tfdomain.DeclaredDemand(tfdomain.DeclaredDemandSpec{WeightValue: "1", WeightUnit: "KG"})
}

type reprepareSchedules struct{}

func (reprepareSchedules) ConnectionHasSchedules(context.Context, tfdomain.TenantID, tfdomain.NetworkConnectionReference) (bool, error) {
	return false, nil
}
func (reprepareSchedules) ListCurrentTemplatesOnConnection(context.Context, tfdomain.TenantID, tfdomain.NetworkConnectionReference) ([]tfports.ScheduleTemplateRecord, error) {
	return nil, nil
}
func (reprepareSchedules) ListCurrentSchedulesDepartingWithin(context.Context, tfdomain.TenantID, tfdomain.NetworkConnectionReference, time.Time, time.Time) ([]tfports.ScheduleRecord, error) {
	return nil, nil
}
func (reprepareSchedules) FindCurrentSchedules(context.Context, tfdomain.TenantID, []tfdomain.ScheduleReference) ([]tfports.ScheduleRecord, error) {
	return nil, nil
}

type repreparePools struct{}

func (repreparePools) ListPoolsOnSchedulesUnlocked(context.Context, tfdomain.TenantID, []tfdomain.ScheduleReference) ([]tfports.CapacityPoolRecord, error) {
	return nil, nil
}

type reprepareMeasured struct {
	reading tfports.MeasuredDemandReading
	err     error
}

func (double reprepareMeasured) LoadMeasuredDemand(context.Context, tfdomain.TenantID, tfdomain.CarriedObjectReference) (tfports.MeasuredDemandReading, error) {
	return double.reading, double.err
}

type reprepareStore struct {
	records map[tfports.TransportPreparationKey]tfports.TransportPreparationRecord
}

func (store *reprepareStore) FindByKey(_ context.Context, key tfports.TransportPreparationKey) (tfports.TransportPreparationRecord, bool, error) {
	record, found := store.records[key]
	return record, found, nil
}

func (store *reprepareStore) Save(_ context.Context, record tfports.TransportPreparationRecord) (tfports.TransportPreparationSaveOutcome, error) {
	store.records[record.Key] = record
	return tfports.TransportPreparationSaved, nil
}

func (store *reprepareStore) HoldPreparationObject(context.Context, tfdomain.TenantID, tfdomain.CarriedObjectReference) error {
	return nil
}

func (store *reprepareStore) ListDeclaredPreparationLegs(
	_ context.Context, tenant tfdomain.TenantID, object tfdomain.CarriedObjectReference, rule tfdomain.PreparationRuleVersion,
) ([]tfdomain.PreparationLeg, error) {
	var legs []tfdomain.PreparationLeg
	for key, record := range store.records {
		if key.TenantID == tenant && key.Object == object && key.Rule == rule && key.Source == tfdomain.DemandFromDeclaration {
			legs = append(legs, record.Preparation.Leg())
		}
	}
	return legs, nil
}

func reprepareFixture(t *testing.T, measured reprepareMeasured) (*tfnode.RePrepareOnMeasurementChange, *reprepareStore) {
	t.Helper()
	store := &reprepareStore{records: map[tfports.TransportPreparationKey]tfports.TransportPreparationRecord{}}
	handler := tfapplication.NewPrepareOnPlannedLegsHandler(tfapplication.PrepareOnPlannedLegsDeps{
		Demand: reprepareDemand{}, Schedules: reprepareSchedules{}, Pools: repreparePools{},
		Preparations: store, Clock: reprepareClock{}, Measured: measured,
	})
	adapter, err := tfnode.NewRePrepareOnMeasurementChange(
		associationDouble{parcels: []nodomain.ParcelAssociationReference{parcelRef(t, "SYN-PARCEL-01")}}, handler)
	if err != nil {
		t.Fatal(err)
	}
	return adapter, store
}

func seedOpenDeclaration(t *testing.T, store *reprepareStore, end time.Time) {
	t.Helper()
	tenant, err := tfdomain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	object, err := tfdomain.NewCarriedObjectReference("SYN-PARCEL-01")
	if err != nil {
		t.Fatal(err)
	}
	direction, err := tfdomain.NewScheduleDirection("SYN-CONN-1", "SYN-NODE-A", "SYN-NODE-B")
	if err != nil {
		t.Fatal(err)
	}
	reference, err := tfdomain.NewPlannedSegmentReference("SYN-RPV-1#1")
	if err != nil {
		t.Fatal(err)
	}
	leg, err := tfdomain.NewPreparationLeg(reference, direction, end.Add(-time.Hour), end)
	if err != nil {
		t.Fatal(err)
	}
	demand, err := tfdomain.DeclaredDemand(tfdomain.DeclaredDemandSpec{WeightValue: "1", WeightUnit: "KG"})
	if err != nil {
		t.Fatal(err)
	}
	preparation, err := tfdomain.PrepareTransportOpportunity(tfdomain.PrepareTransportOpportunitySpec{
		TenantID: tenant, Leg: leg, Object: object, Rule: tfdomain.BuiltInPreparationRuleVersion(),
		Demand: demand, JudgedAt: end,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := tfdomain.CanonicalizePreparationInput(leg, demand)
	if err != nil {
		t.Fatal(err)
	}
	record := tfports.TransportPreparationRecord{
		Key: tfports.TransportPreparationKey{
			TenantID: tenant, Leg: reference, Object: object, Rule: tfdomain.BuiltInPreparationRuleVersion(),
			Source: tfdomain.DemandFromDeclaration,
		},
		InputDigest: digest, Preparation: preparation, RecordedAt: end,
	}
	store.records[record.Key] = record
}

func oneReading(t *testing.T, occurred time.Time) tfports.MeasuredDemandReading {
	t.Helper()
	snapshot, err := tfdomain.MeasuredDemand(tfdomain.MeasuredDemandSpec{
		Anchor: "SYN-MEASURE-1", WeightValue: "2", WeightUnit: "KG",
		Length: "20", Width: "15", Height: "10", DimensionUnit: "CM",
	})
	if err != nil {
		t.Fatal(err)
	}
	return tfports.MeasuredDemandReading{Answer: tfports.MeasuredDemandOne, Snapshot: snapshot, OccurredAt: occurred}
}

func measuredRows(store *reprepareStore) int {
	count := 0
	for key := range store.records {
		if key.Source == tfdomain.DemandFromActualMeasurement {
			count++
		}
	}
	return count
}

// Covers: 没有实测、定不下、还没有申报、窗口已结束入账且零写入；新形成入账；读不动重投；译不成响亮失败。
func TestRePreparationResultsBecomeAckRetryOrALoudFailure(t *testing.T) {
	occurred := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	change := changedMeasurement(nodomain.HandlingUnitObject.String(), "SYN-HU-1")
	ack := []struct {
		name    string
		reading tfports.MeasuredDemandReading
		seed    bool
		end     time.Time
	}{
		{"没有实测", tfports.MeasuredDemandReading{Answer: tfports.MeasuredDemandNone}, true, occurred.Add(time.Hour)},
		{"定不下", tfports.MeasuredDemandReading{Answer: tfports.MeasuredDemandUndetermined}, true, occurred.Add(time.Hour)},
		{"还没有申报", oneReading(t, occurred), false, occurred},
		{"窗口已结束", oneReading(t, occurred), true, occurred.Add(-time.Second)},
	}
	for _, test := range ack {
		t.Run(test.name, func(t *testing.T) {
			adapter, store := reprepareFixture(t, reprepareMeasured{reading: test.reading})
			if test.seed {
				seedOpenDeclaration(t, store, test.end)
			}
			before := len(store.records)
			if err := adapter.HandleChangedMeasurement(context.Background(), change); err != nil || len(store.records) != before {
				t.Fatalf("%s = %v 行 %d→%d", test.name, err, before, len(store.records))
			}
		})
	}

	formed, store := reprepareFixture(t, reprepareMeasured{reading: oneReading(t, occurred)})
	seedOpenDeclaration(t, store, occurred.Add(time.Hour))
	if err := formed.HandleChangedMeasurement(context.Background(), change); err != nil || measuredRows(store) != 1 {
		t.Fatalf("新形成 = %v 实测行 %d", err, measuredRows(store))
	}

	down, _ := reprepareFixture(t, reprepareMeasured{err: errors.New("measurement down")})
	if err := down.HandleChangedMeasurement(context.Background(), change); !errors.Is(err, tfnode.ErrRePreparationUndecided) {
		t.Fatalf("读不动 = %v", err)
	}
	untranslatable, _ := reprepareFixture(t, reprepareMeasured{err: errors.Join(tfports.ErrMeasuredDemandUntranslatable, errors.New("edge"))})
	if err := untranslatable.HandleChangedMeasurement(context.Background(), change); !errors.Is(err, tfports.ErrMeasuredDemandUntranslatable) || errors.Is(err, tfnode.ErrRePreparationUndecided) {
		t.Fatalf("译不成 = %v", err)
	}
}

// Covers: 编排拒收，以及封闭集以外的结果，都响亮失败，不进本路的未决重投名单。
func TestRefusedAndUnexpectedRePreparationResultsAreNotRetried(t *testing.T) {
	// 结果字段不外露，适配器自己组的命令又过了受理门，拒收样本只能从一次空命令上取。
	refused, err := tfapplication.NewPrepareOnPlannedLegsHandler(tfapplication.PrepareOnPlannedLegsDeps{}).
		RePrepareOnMeasurementChange(context.Background(), tfapplication.RePrepareOnMeasurementChangeCommand{})
	if err != nil || refused.Outcome() != tfapplication.RePrepareRunNotAccepted {
		t.Fatalf("拒收样本 = %s %v", refused.Outcome(), err)
	}
	one := associationDouble{parcels: []nodomain.ParcelAssociationReference{parcelRef(t, "SYN-PARCEL-01")}}
	change := changedMeasurement(nodomain.HandlingUnitObject.String(), "SYN-HU-1")
	for _, test := range []struct {
		name   string
		result tfapplication.RePrepareOnMeasurementChangeResult
	}{
		{"拒收", refused},
		{"封闭集以外", tfapplication.RePrepareOnMeasurementChangeResult{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			adapter, err := tfnode.NewRePrepareOnMeasurementChange(one, fixedRePreparation{result: test.result})
			if err != nil {
				t.Fatal(err)
			}
			err = adapter.HandleChangedMeasurement(context.Background(), change)
			if err == nil || errors.Is(err, tfnode.ErrRePreparationUndecided) {
				t.Fatalf("%s = %v", test.name, err)
			}
		})
	}
}

type fixedRePreparation struct {
	result tfapplication.RePrepareOnMeasurementChangeResult
}

func (double fixedRePreparation) RePrepareOnMeasurementChange(
	context.Context, tfapplication.RePrepareOnMeasurementChangeCommand,
) (tfapplication.RePrepareOnMeasurementChangeResult, error) {
	return double.result, nil
}
