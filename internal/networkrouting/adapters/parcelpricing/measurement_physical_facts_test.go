package parcelpricing_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelpricing"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	ppnodeops "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/nodeoperations"
	pptransport "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/transportfulfillment"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件钉路由的物理事实来源怎样译计价那只带时点的读法。夹具为隔离合成，只记 S。
// 取哪一条、单位怎么译不在这里重写：定不下的来由由 ParcelMeasurementView 自己答出来。

// Covers: 读口答取得了——重量、外廓与那条事实引用整份交回，依据 PHYSICAL，客户声明一格都不读。
func TestMeasurementPhysicalFactsMapFoundToObtainedAndCarryTheFact(t *testing.T) {
	occurred := time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC)
	source := measurementFacts(t, []string{"unit-1"}, []nodomain.ActualMeasurement{
		routingMeasured(t, "measure-1", occurred, "KG", "2.10", "CM", "30", "25", "20"),
	}, false)
	got, err := source.PhysicalParcelFactsFor(context.Background(), measurementParcelKey(t), occurred)
	if err != nil {
		t.Fatal(err)
	}
	if got.Answer() != adapter.PhysicalFactsObtained {
		t.Fatalf("答复 = %v，want 取得了", got.Answer())
	}

	declared := declaredFacts(t, "9.9", false)
	facts, found, err := pointedAt(t, declared, source, occurred.Add(time.Hour))
	if err != nil || !found {
		t.Fatalf("found=%v err=%v，want 按实测交出", found, err)
	}
	if facts.Basis != nrdomain.ParcelFactsPhysical || facts.Weight.Value().String() != "2.1" {
		t.Fatalf("事实 = %+v，want 实测 2.1 KG、PHYSICAL", facts)
	}
	if facts.Dimensions == nil || facts.Dimensions.LongestSide().Value().String() != "30" || facts.Dimensions.Unit() != ppdomain.LengthUnitCentimeter {
		t.Fatalf("外廓 = %+v，want 最长边 30 CM", facts.Dimensions)
	}
	if len(facts.Facts) != 1 || facts.Facts[0].Reference().Kind() != ppdomain.ArtifactActualMeasurement ||
		facts.Facts[0].Reference().ID() != "parcel-1" || facts.Facts[0].Reference().Version() != "measure-1" {
		t.Fatalf("事实引用 = %+v，want 这一件包裹的 measure-1", facts.Facts)
	}
	if declared.asked != 0 {
		t.Fatalf("取得了却读了 %d 次客户声明", declared.asked)
	}
}

// Covers: 读口一项都没取得——退回客户声明并标明 DECLARATION_FALLBACK。
func TestMeasurementPhysicalFactsMapAnEmptyLoadToNoneObtained(t *testing.T) {
	source := measurementFacts(t, nil, nil, false)
	got, err := source.PhysicalParcelFactsFor(context.Background(), measurementParcelKey(t), judgedAt)
	if err != nil {
		t.Fatal(err)
	}
	if got.Answer() != adapter.PhysicalFactsNotObtained {
		t.Fatalf("答复 = %v，want 没取得", got.Answer())
	}

	declared := declaredFacts(t, "2.5", false)
	facts, found, err := pointedAt(t, declared, source, judgedAt)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v，want 退回声明", found, err)
	}
	if facts.Basis != nrdomain.ParcelFactsDeclarationFallback || facts.Weight.Value().String() != "2.5" {
		t.Fatalf("事实 = %+v，want 声明 2.5 KG、DECLARATION_FALLBACK", facts)
	}
	if declared.asked != 1 {
		t.Fatalf("声明被问了 %d 次，want 1", declared.asked)
	}
}

// Covers: 读口定不下——多件实物、多条无采用、发生时间并列、单位译不出都映成定不下，不退回客户声明。
func TestMeasurementPhysicalFactsMapAnUndeterminedLoadToUnresolved(t *testing.T) {
	occurred := time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC)
	later := occurred.Add(time.Hour)
	for _, testCase := range []struct {
		name         string
		units        []string
		measurements []nodomain.ActualMeasurement
		adopted      bool
	}{
		{"多件实物", []string{"unit-1", "unit-2"}, nil, false},
		{"多条无采用", []string{"unit-1"}, []nodomain.ActualMeasurement{
			routingMeasured(t, "measure-1", occurred, "KG", "2.10", "CM", "30", "25", "20"),
			routingMeasured(t, "measure-2", later, "KG", "2.40", "CM", "30", "25", "20"),
		}, false},
		{"发生时间并列", []string{"unit-1"}, []nodomain.ActualMeasurement{
			routingMeasured(t, "measure-1", occurred, "KG", "2.10", "CM", "30", "25", "20"),
			routingMeasured(t, "measure-2", occurred, "KG", "2.40", "CM", "30", "25", "20"),
		}, true},
		{"单位译不出", []string{"unit-1"}, []nodomain.ActualMeasurement{
			routingMeasured(t, "measure-1", occurred, "STONE", "2.10", "CM", "30", "25", "20"),
		}, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			source := measurementFacts(t, testCase.units, testCase.measurements, testCase.adopted)
			got, err := source.PhysicalParcelFactsFor(context.Background(), measurementParcelKey(t), later)
			if err != nil {
				t.Fatal(err)
			}
			if got.Answer() != adapter.PhysicalFactsUnresolved {
				t.Fatalf("答复 = %v，want 定不下", got.Answer())
			}
			declared := declaredFacts(t, "2.5", false)
			facts, found, err := pointedAt(t, declared, source, later)
			if err != nil {
				t.Fatal(err)
			}
			if found || facts.Basis != nrdomain.ParcelFactsPhysicalUnresolved || declared.asked != 0 {
				t.Fatalf("found=%v 依据=%s 声明被问=%d，want 不定价、不读声明", found, facts.Basis, declared.asked)
			}
		})
	}
}

// Covers: 读口报错原样冒上去，不折成没取得或定不下。
func TestMeasurementPhysicalFactsReturnsTheReadError(t *testing.T) {
	readFailed := errors.New("读库失败")
	source, err := adapter.NewMeasurementPhysicalFacts(&measurementRead{err: readFailed})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.PhysicalParcelFactsFor(context.Background(), measurementParcelKey(t), judgedAt); err != readFailed {
		t.Fatalf("err = %v，want 原样冒上", err)
	}
}

// Covers: 判断点给出的时点原样传给读口，包裹标识用判断键的声明包裹。
func TestMeasurementPhysicalFactsPassesTheAsOfAndTheParcelThrough(t *testing.T) {
	read := &measurementRead{}
	source, err := adapter.NewMeasurementPhysicalFacts(read)
	if err != nil {
		t.Fatal(err)
	}
	key := measurementParcelKey(t)
	asOf := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	if _, err := source.PhysicalParcelFactsFor(context.Background(), key, asOf); err != nil {
		t.Fatal(err)
	}
	if !read.asOf.Equal(asOf) || read.object != key.DeclaredParcelID.String() || read.tenant != key.TenantID.String() {
		t.Fatalf("读口收到 tenant=%q object=%q asOf=%s，want %s / %s / %s",
			read.tenant, read.object, read.asOf, key.TenantID, key.DeclaredParcelID, asOf)
	}
}

func pointedAt(t *testing.T, declared *declaredDouble, physical *adapter.MeasurementPhysicalFacts, now time.Time) (adapter.ParcelFacts, bool, error) {
	t.Helper()
	pointed, err := adapter.NewPointedParcelFacts(declared, physical, fixedClock{at: now})
	if err != nil {
		t.Fatal(err)
	}
	return pointed.ParcelFactsFor(context.Background(), measurementParcelKey(t), nrdomain.NewArrivalFactsPoint())
}

func measurementFacts(t *testing.T, units []string, measurements []nodomain.ActualMeasurement, adopted bool) *adapter.MeasurementPhysicalFacts {
	t.Helper()
	fixture := routingMeasurementFixture{units: units, measurements: measurements, adopted: adopted}
	view, err := ppnodeops.NewParcelMeasurementView(fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	source, err := adapter.NewMeasurementPhysicalFacts(view)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func measurementParcelKey(t *testing.T) nrdomain.InitialRouteJudgmentKey {
	t.Helper()
	return nrdomain.InitialRouteJudgmentKey{
		TenantID:         value(t, nrdomain.NewTenantID, "tenant-a"),
		DeclaredParcelID: value(t, nrdomain.NewDeclaredParcelID, "parcel-1"),
	}
}

type measurementRead struct {
	load   pptransport.ActualMeasurementLoad
	err    error
	tenant string
	object string
	asOf   time.Time
}

func (read *measurementRead) LoadActualMeasurementAsOf(
	_ context.Context, tenant ppdomain.TenantID, object string, asOf time.Time,
) (pptransport.ActualMeasurementLoad, error) {
	read.tenant = tenant.String()
	read.object = object
	read.asOf = asOf
	return read.load, read.err
}

type routingMeasurementFixture struct {
	units        []string
	measurements []nodomain.ActualMeasurement
	adopted      bool
}

func (fixture routingMeasurementFixture) ListHandlingUnits(context.Context, nodomain.TenantID, nodomain.ParcelAssociationReference) ([]nodomain.HandlingUnitID, error) {
	units := make([]nodomain.HandlingUnitID, 0, len(fixture.units))
	for _, raw := range fixture.units {
		unit, err := nodomain.NewHandlingUnitID(raw)
		if err != nil {
			return nil, err
		}
		units = append(units, unit)
	}
	return units, nil
}

func (fixture routingMeasurementFixture) ListStillValid(context.Context, nodomain.TenantID, nodomain.MeasurementObjectKind, string) ([]noports.ActualMeasurementRecord, error) {
	records := make([]noports.ActualMeasurementRecord, 0, len(fixture.measurements))
	for _, measurement := range fixture.measurements {
		records = append(records, noports.ActualMeasurementRecord{Measurement: measurement})
	}
	return records, nil
}

func (fixture routingMeasurementFixture) FindAdoption(context.Context, nodomain.TenantID) (noports.MeasurementSelectionAdoption, bool, error) {
	if !fixture.adopted {
		return noports.MeasurementSelectionAdoption{}, false, nil
	}
	return noports.MeasurementSelectionAdoption{Form: nodomain.LatestStillValidByOccurredAt, Basis: "basis-1"}, true, nil
}

func routingMeasured(t *testing.T, sourceID string, occurred time.Time, weightUnit, weightValue, dimensionUnit, length, width, height string) nodomain.ActualMeasurement {
	t.Helper()
	source, err := nodomain.NewWorkFactSource(
		sourceID,
		value(t, nodomain.NewPerformingPartyReference, "scale-7"),
		value(t, nodomain.NewExecutionEvidenceReference, "WEIGH/"+sourceID),
		occurred,
	)
	if err != nil {
		t.Fatal(err)
	}
	gross, err := nodomain.NewGrossWeight(weightValue, weightUnit)
	if err != nil {
		t.Fatal(err)
	}
	outer, err := nodomain.NewOuterDimensions(length, width, height, dimensionUnit)
	if err != nil {
		t.Fatal(err)
	}
	measurement, err := nodomain.NewActualMeasurement(nodomain.ActualMeasurementSpec{
		Source: source, ObjectKind: nodomain.HandlingUnitObject, ObjectID: "unit-1",
		Node: value(t, nodomain.NewNodeReference, "node-origin"), Location: value(t, nodomain.NewWorkLocationReference, "bench-2"),
		Basis:  value(t, nodomain.NewWorkBasisReference, "intake-1"),
		Weight: gross, Dimensions: outer,
	})
	if err != nil {
		t.Fatal(err)
	}
	return measurement
}
