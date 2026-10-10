package parcelpricing_test

import (
	"context"
	"errors"
	"testing"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelpricing"
)

// 本文件钉票 routing-first-cut/17 的「包裹事实来自客户声明并带来源标记」：预路由按判断键里的声明包裹问
// parcel-shipment 的客户声明，实重与外廓原样译进计价输入，来源标在事实引用里；声明不可用时如实答没有。

// Covers: 基线上的客户声明译成包裹事实——实重与外廓照声明，事实引用标明来源是客户声明（与 ADR-0171 同形）。
func TestTheDeclaredMeasurementBecomesTheParcelFactsTaggedAsADeclaration(t *testing.T) {
	declared := &declaredView{resolution: baselineDeclaration(t, "2.5", "KG", &dimensionsSpec{"30", "20", "10", "CM"})}
	facts := mustDeclaredFacts(t, declared)

	got, found, err := facts.ParcelFactsFor(context.Background(), declaredKey(t))
	if err != nil || !found {
		t.Fatalf("found=%v err=%v, want 基线声明可用", found, err)
	}
	if declared.asked != (askedParcel{tenant: "tenant-1", parcel: "parcel-1"}) {
		t.Fatalf("问的是 %+v，want 判断键里的租户与声明包裹", declared.asked)
	}
	if got.Weight.Value().String() != "2.5" || got.Weight.Unit() != ppdomain.WeightUnitKilogram {
		t.Fatalf("实重 = %s %s，want 2.5 KG", got.Weight.Value(), got.Weight.Unit())
	}
	if got.Dimensions == nil {
		t.Fatal("声明带外廓，事实里却没有")
	}
	if len(got.Facts) != 1 {
		t.Fatalf("事实引用 %d 条，want 1", len(got.Facts))
	}
	reference := got.Facts[0].Reference()
	if reference.Kind() != ppdomain.ArtifactDeclaredMeasurement || reference.ID() != "parcel-1" ||
		reference.Version() != "declaration@baseline" {
		t.Fatalf("事实引用 = %s %s %s，want 申报测量 parcel-1 declaration@baseline",
			reference.Kind(), reference.ID(), reference.Version())
	}
}

// Covers: 只报重量的声明照样可用，外廓如实缺席，不补默认尺寸。
func TestAWeightOnlyDeclarationLeavesTheDimensionsAbsent(t *testing.T) {
	facts := mustDeclaredFacts(t, &declaredView{resolution: baselineDeclaration(t, "1", "KG", nil)})

	got, found, err := facts.ParcelFactsFor(context.Background(), declaredKey(t))
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if got.Dimensions != nil {
		t.Fatalf("外廓 = %+v，want 缺席", got.Dimensions)
	}
}

// Covers: 声明不可用时答没有，不代拟一份重量——没有画像、单位不在计价封闭集（与 ADR-0171 决定三同一张表）、
// 对象不属任何已接受委托，三格都是 found=false 且不报错。
func TestAnUnusableDeclarationAnswersNoParcelFacts(t *testing.T) {
	for name, resolution := range map[string]psdomain.DeclaredMeasurementResolution{
		"没有画像":    psdomain.DeclaredMeasurementNotDeclaredResolution(),
		"单位不在表内":  baselineDeclaration(t, "2.5", "kg", nil),
		"不属已接受委托": psdomain.NoDeclaredMeasurementResolution(),
	} {
		t.Run(name, func(t *testing.T) {
			facts := mustDeclaredFacts(t, &declaredView{resolution: resolution})
			if _, found, err := facts.ParcelFactsFor(context.Background(), declaredKey(t)); err != nil || found {
				t.Fatalf("found=%v err=%v, want 没有包裹事实", found, err)
			}
		})
	}
}

// Covers: 读口答不出是依赖故障，上抛，不折成「没有声明」。
func TestADeclaredMeasurementReadFailureSurfaces(t *testing.T) {
	broken := errors.New("读库失败")
	facts := mustDeclaredFacts(t, &declaredView{err: broken})

	if _, _, err := facts.ParcelFactsFor(context.Background(), declaredKey(t)); !errors.Is(err, broken) {
		t.Fatalf("err = %v, want 上抛读口故障", err)
	}
}

func TestDeclaredParcelFactsRefuseANilView(t *testing.T) {
	if _, err := adapter.NewDeclaredParcelFacts(nil); err == nil {
		t.Fatal("nil 读口被收下了")
	}
}

type askedParcel struct{ tenant, parcel string }

// declaredView 是 parcel-shipment 申报测量读口的替身：记下被问的（租户，包裹），交回预置的答格。
type declaredView struct {
	resolution psdomain.DeclaredMeasurementResolution
	err        error
	asked      askedParcel
}

func (view *declaredView) LoadDeclaredMeasurement(
	_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psdomain.DeclaredMeasurementResolution, error) {
	view.asked = askedParcel{tenant: tenant.String(), parcel: parcel.String()}
	return view.resolution, view.err
}

func mustDeclaredFacts(t *testing.T, view *declaredView) *adapter.DeclaredParcelFacts {
	t.Helper()
	facts, err := adapter.NewDeclaredParcelFacts(view)
	if err != nil {
		t.Fatalf("构造包裹事实取数侧：%v", err)
	}
	return facts
}

func declaredKey(t *testing.T) nrdomain.InitialRouteJudgmentKey {
	t.Helper()
	return nrdomain.InitialRouteJudgmentKey{
		TenantID:         value(t, nrdomain.NewTenantID, "tenant-1"),
		DeclaredParcelID: value(t, nrdomain.NewDeclaredParcelID, "parcel-1"),
	}
}

type dimensionsSpec struct{ length, width, height, unit string }

// baselineDeclaration 造一格「基线上的客户声明」：值与单位原样，外廓可缺。
func baselineDeclaration(t *testing.T, weight, unit string, sides *dimensionsSpec) psdomain.DeclaredMeasurementResolution {
	t.Helper()
	declaredWeight, err := psdomain.NewDeclaredWeight(
		value(t, psdomain.NewMeasurementValue, weight), value(t, psdomain.NewMeasurementUnitReference, unit))
	if err != nil {
		t.Fatalf("申报重量：%v", err)
	}
	var dimensions psdomain.DeclaredDimensions
	if sides != nil {
		dimensions, err = psdomain.NewDeclaredDimensions(
			value(t, psdomain.NewMeasurementValue, sides.length),
			value(t, psdomain.NewMeasurementValue, sides.width),
			value(t, psdomain.NewMeasurementValue, sides.height),
			value(t, psdomain.NewMeasurementUnitReference, sides.unit),
		)
		if err != nil {
			t.Fatalf("申报外廓：%v", err)
		}
	}
	measurement, err := psdomain.NewDeclaredMeasurement(declaredWeight, dimensions)
	if err != nil {
		t.Fatalf("申报测量：%v", err)
	}
	resolution, err := psdomain.DeclaredMeasurementOnBaseline(measurement)
	if err != nil {
		t.Fatalf("基线答格：%v", err)
	}
	return resolution
}
