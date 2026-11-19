package nodeoperations_test

import (
	"context"
	"strings"
	"testing"
	"time"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/nodeoperations"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

func TestAParcelWithoutAnAssociatedUnitHasNoMeasurement(t *testing.T) {
	view := measurementView(t, nil, nil, false)
	load, err := view.LoadActualMeasurement(context.Background(), pricingTenant(t), "parcel-1")
	if err != nil {
		t.Fatal(err)
	}
	if load.Found || load.Undetermined != "" {
		t.Fatalf("没有实物却答了实测：%+v", load)
	}
}

// Covers: 一条测量同时带重量与尺寸，读口两样都交回，事实只有一条，版本是那次测量的来源身份。
func TestOneMeasurementAnswersBothTheWeightAndTheDimensions(t *testing.T) {
	only := measured(t, "measure-1", "2.10", time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC))
	view := measurementView(t, []string{"unit-1"}, []nodomain.ActualMeasurement{only}, false)
	load, err := view.LoadActualMeasurement(context.Background(), pricingTenant(t), "parcel-1")
	if err != nil {
		t.Fatal(err)
	}
	if !load.Found || load.Undetermined != "" || load.Weight.Value().String() != "2.1" {
		t.Fatalf("实测 = %+v", load)
	}
	if longest := load.Dimensions.LongestSide(); longest.Value().String() != "30" || load.Dimensions.Unit() != ppdomain.LengthUnitCentimeter {
		t.Fatalf("尺寸 = %+v，want 最长边 30 CM", load.Dimensions)
	}
	if load.Fact.Reference().Kind() != ppdomain.ArtifactActualMeasurement || load.Fact.Reference().Version() != "measure-1" {
		t.Fatalf("事实 = %+v", load.Fact)
	}
}

// 空白的包裹标识指不到任何一件包裹：答没有实测、不报错，也不拿它去问关联与测量的读口。夹具的读口对任何包裹都交回一件实物
// 与一条测量，所以真问了的话这里会读到实测。
func TestABlankParcelIdentityHasNoMeasurementAndAsksNoOne(t *testing.T) {
	only := measured(t, "measure-1", "2.10", time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC))
	view := measurementView(t, []string{"unit-1"}, []nodomain.ActualMeasurement{only}, false)
	for name, parcel := range map[string]string{"空串": "", "纯空白": "  \t"} {
		load, err := view.LoadActualMeasurement(context.Background(), pricingTenant(t), parcel)
		if err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if load.Found || load.Undetermined != "" {
			t.Fatalf("%s：空白标识答了实测：%+v", name, load)
		}
	}
}

func TestSeveralAssociatedUnitsAreUndetermined(t *testing.T) {
	view := measurementView(t, []string{"unit-1", "unit-2"}, nil, false)
	load, err := view.LoadActualMeasurement(context.Background(), pricingTenant(t), "parcel-1")
	if err != nil {
		t.Fatal(err)
	}
	if load.Undetermined != "输入不可得：这件包裹关联着多件作业实物，定不下" {
		t.Fatalf("定不下 = %q", load.Undetermined)
	}
}

// Covers: 复测是再登一条完整的测量。没采用形态时两条仍有效就定不下；采用之后取发生时间最近的那一条，重量与尺寸都是它的。
func TestARemeasurementStaysUndeterminedUntilTheFormIsAdopted(t *testing.T) {
	earlier := measured(t, "measure-1", "2.10", time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC))
	later := measured(t, "measure-2", "2.40", time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC))
	view := measurementView(t, []string{"unit-1"}, []nodomain.ActualMeasurement{earlier, later}, false)
	load, err := view.LoadActualMeasurement(context.Background(), pricingTenant(t), "parcel-1")
	if err != nil {
		t.Fatal(err)
	}
	if load.Undetermined != "输入不可得：实测多于一条" || load.Found {
		t.Fatalf("没采用 = %+v", load)
	}

	adopted := measurementView(t, []string{"unit-1"}, []nodomain.ActualMeasurement{earlier, later}, true)
	load, err = adopted.LoadActualMeasurement(context.Background(), pricingTenant(t), "parcel-1")
	if err != nil {
		t.Fatal(err)
	}
	if !load.Found || load.Weight.Value().String() != "2.4" || load.Undetermined != "" {
		t.Fatalf("采用后 = %+v", load)
	}
	if load.Fact.Reference().Kind() != ppdomain.ArtifactActualMeasurement || load.Fact.Reference().Version() != "measure-2" {
		t.Fatalf("事实 = %+v", load.Fact)
	}
}

func TestTheSameOccurredTimeDoesNotPickAMeasurement(t *testing.T) {
	at := time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC)
	view := measurementView(t, []string{"unit-1"}, []nodomain.ActualMeasurement{
		measured(t, "measure-1", "2.10", at),
		measured(t, "measure-2", "2.40", at),
	}, true)
	load, err := view.LoadActualMeasurement(context.Background(), pricingTenant(t), "parcel-1")
	if err != nil {
		t.Fatal(err)
	}
	if load.Undetermined != "输入不可得：两条实测的发生时间相同，定不下" || load.Found {
		t.Fatalf("定不下 = %+v", load)
	}
}

// 译不出重量或尺寸时整份输入不可得，原因要照实说是哪一种：单位不在计价单位表里，与数值计价读不成重量／外廓，是两条
// 不同的排查路。节点登记册只拦数字的形状与正负，不核单位表，也不核计价能表示的位数，所以两种都进得来。
func TestAMeasurementPricingCannotTranslateIsUndeterminedWithTheTrueReason(t *testing.T) {
	// 小数位数比计价的十进制数能收的上限多一位：形状是合法的正数，登记册收得下，计价读不了。
	tooPrecise := "1." + strings.Repeat("0", ppdomain.DecimalMaxScale) + "1"
	at := time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC)
	for _, testCase := range []struct {
		name        string
		measurement nodomain.ActualMeasurement
		want        string
	}{
		{
			"重量单位在表外",
			measuredAs(t, "measure-1", at, "STONE", "2.10", "CM", "30", "25", "20"),
			"输入不可得：实测重量的单位不在计价单位表里：STONE",
		},
		{
			"重量单位小写也在表外",
			measuredAs(t, "measure-1", at, "kg", "2.10", "CM", "30", "25", "20"),
			"输入不可得：实测重量的单位不在计价单位表里：kg",
		},
		{
			"重量数值超出计价能表示的位数",
			measuredAs(t, "measure-1", at, "KG", tooPrecise, "CM", "30", "25", "20"),
			"输入不可得：实测重量的数值不成立，计价读不成一个重量",
		},
		{
			"尺寸单位在表外",
			measuredAs(t, "measure-1", at, "KG", "2.10", "FATHOM", "30", "25", "20"),
			"输入不可得：实测尺寸的单位不在计价单位表里：FATHOM",
		},
		{
			"尺寸数值超出计价能表示的位数",
			measuredAs(t, "measure-1", at, "KG", "2.10", "CM", "30", tooPrecise, "20"),
			"输入不可得：实测尺寸的数值不成立，计价读不成一个外廓",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			view := measurementView(t, []string{"unit-1"}, []nodomain.ActualMeasurement{testCase.measurement}, false)
			load, err := view.LoadActualMeasurement(context.Background(), pricingTenant(t), "parcel-1")
			if err != nil {
				t.Fatal(err)
			}
			if load.Found || load.Undetermined != testCase.want {
				t.Fatalf("译不出 = %+v，want 不可得且原因 %q", load, testCase.want)
			}
		})
	}
}

type measurementFixture struct {
	units        []string
	measurements []nodomain.ActualMeasurement
	adopted      bool
}

func (fixture measurementFixture) ListHandlingUnits(context.Context, nodomain.TenantID, nodomain.ParcelAssociationReference) ([]nodomain.HandlingUnitID, error) {
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

func (fixture measurementFixture) ListStillValid(context.Context, nodomain.TenantID, nodomain.MeasurementObjectKind, string) ([]noports.ActualMeasurementRecord, error) {
	records := make([]noports.ActualMeasurementRecord, 0, len(fixture.measurements))
	for _, measurement := range fixture.measurements {
		records = append(records, noports.ActualMeasurementRecord{Measurement: measurement})
	}
	return records, nil
}

func (fixture measurementFixture) FindAdoption(context.Context, nodomain.TenantID) (noports.MeasurementSelectionAdoption, bool, error) {
	if !fixture.adopted {
		return noports.MeasurementSelectionAdoption{}, false, nil
	}
	return noports.MeasurementSelectionAdoption{Form: nodomain.LatestStillValidByOccurredAt, Basis: "basis-1"}, true, nil
}

func measurementView(t *testing.T, units []string, measurements []nodomain.ActualMeasurement, adopted bool) *adapter.ParcelMeasurementView {
	t.Helper()
	view, err := adapter.NewParcelMeasurementView(measurementFixture{units: units, measurements: measurements, adopted: adopted}, measurementFixture{units: units, measurements: measurements, adopted: adopted}, measurementFixture{units: units, measurements: measurements, adopted: adopted})
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func pricingTenant(t *testing.T) ppdomain.TenantID {
	t.Helper()
	tenant, err := ppdomain.NewTenantID("tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	return tenant
}

// measured 造一条完整的测量：毛重由用例给，尺寸 30×25×20 CM。
func measured(t *testing.T, sourceID, value string, occurred time.Time) nodomain.ActualMeasurement {
	t.Helper()
	return measuredAs(t, sourceID, occurred, "KG", value, "CM", "30", "25", "20")
}

// measuredAs 连单位与每一格数字都由用例给：登记册只拦数字形状与正负，表外的单位、计价读不了的位数都能登进去。
func measuredAs(t *testing.T, sourceID string, occurred time.Time, weightUnit, weightValue, dimensionUnit, length, width, height string) nodomain.ActualMeasurement {
	t.Helper()
	source, err := nodomain.NewWorkFactSource(
		sourceID,
		must(t, nodomain.NewPerformingPartyReference, "scale-7"),
		must(t, nodomain.NewExecutionEvidenceReference, "WEIGH/"+sourceID),
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
		Node: must(t, nodomain.NewNodeReference, "node-origin"), Location: must(t, nodomain.NewWorkLocationReference, "bench-2"),
		Basis:  must(t, nodomain.NewWorkBasisReference, "intake-1"),
		Weight: gross, Dimensions: outer,
	})
	if err != nil {
		t.Fatal(err)
	}
	return measurement
}

func must[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
