package parcelpricing

import (
	"context"
	"errors"
	"fmt"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// errUntranslatableParcelFacts 说判断键或客户声明译不进计价输入的词汇（租户、声明包裹、事实引用）。那是两侧词汇表
// 出了分歧，不是这件包裹没有事实，所以上抛，不答 found=false。
var errUntranslatableParcelFacts = errors.New("network routing: untranslatable parcel facts")

// DeclaredParcelFacts 是预路由的包裹事实取数侧：按判断键里的声明包裹问 parcel-shipment 的客户声明，译成计价输入要的
// 实重与外廓（network-routing CONTEXT「预路由使用客户声明快照」，ADR-0148 决定四第 7 条）。来源标在事实引用的版本串
// 里，标法与 ADR-0171 同形；单位只认计价封闭集，也同那一份（决定三）。
//
// 它落在本包而不在 adapters/parcelshipment：读的是 parcel-shipment，交出的是计价输入的原料，与 ADR-0171 越权风险点 1
// 同一处取舍。
type DeclaredParcelFacts struct {
	declared psports.DeclaredMeasurementView
}

func NewDeclaredParcelFacts(declared psports.DeclaredMeasurementView) (*DeclaredParcelFacts, error) {
	if declared == nil {
		return nil, fmt.Errorf("network routing: declared parcel facts: declared measurement view is nil")
	}
	return &DeclaredParcelFacts{declared: declared}, nil
}

var _ ParcelFactsSource = (*DeclaredParcelFacts)(nil)

// ParcelFactsFor 交出这件声明包裹的客户声明。没有可用的声明（不属已接受委托、待复核、未申报、已采用那一版没带测量、
// 单位不在计价封闭集）一律答 found=false：各段照旧待判断，不代拟一份重量。读口答不出才是错误。
func (source *DeclaredParcelFacts) ParcelFactsFor(
	ctx context.Context,
	key nrdomain.InitialRouteJudgmentKey,
) (ParcelFacts, bool, error) {
	tenant, err := psdomain.NewTenantID(key.TenantID.String())
	if err != nil {
		return ParcelFacts{}, false, fmt.Errorf("%w: tenant %q is not a parcel-shipment tenant: %v",
			errUntranslatableParcelFacts, key.TenantID, err)
	}
	parcel, err := psdomain.NewDeclaredParcelID(key.DeclaredParcelID.String())
	if err != nil {
		return ParcelFacts{}, false, fmt.Errorf("%w: parcel %q is not a declared parcel: %v",
			errUntranslatableParcelFacts, key.DeclaredParcelID, err)
	}
	resolution, err := source.declared.LoadDeclaredMeasurement(ctx, tenant, parcel)
	if err != nil {
		return ParcelFacts{}, false, fmt.Errorf("load declared measurement: %w", err)
	}
	measurement, present := resolution.Measurement()
	if !present {
		return ParcelFacts{}, false, nil
	}
	weight, ok := declaredWeight(measurement.Weight())
	if !ok {
		return ParcelFacts{}, false, nil
	}
	var dimensions *ppdomain.Dimensions
	if sides, declared := measurement.Dimensions(); declared {
		translated, ok := declaredDimensions(sides)
		if !ok {
			return ParcelFacts{}, false, nil
		}
		dimensions = &translated
	}
	fact, err := declarationFact(parcel.String(), resolution)
	if err != nil {
		return ParcelFacts{}, false, err
	}
	return ParcelFacts{Weight: weight, Dimensions: dimensions, Facts: []ppdomain.VersionedFactReference{fact}}, true, nil
}

// declarationFact 记这份包裹事实出自客户声明，以及哪一版：基线上的、某一已采用版本的，或锚缺席时只记来源。
func declarationFact(parcel string, resolution psdomain.DeclaredMeasurementResolution) (ppdomain.VersionedFactReference, error) {
	version := "declaration"
	if anchor, anchored := resolution.Anchor(); anchored {
		if adopted, ok := anchor.AdoptedVersion(); ok {
			version = "declaration@" + adopted.String()
		} else if anchor.OnAcceptanceBaseline() {
			version = "declaration@baseline"
		}
	}
	reference, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactDeclaredMeasurement, parcel, version)
	if err != nil {
		return ppdomain.VersionedFactReference{}, fmt.Errorf("%w: declaration fact: %v", errUntranslatableParcelFacts, err)
	}
	return ppdomain.NewVersionedFactReference(reference)
}

// declaredWeight 只认计价封闭集里的单位，原样不折大小写：猜错单位会让计价重量差出几个量级。
func declaredWeight(weight psdomain.DeclaredWeight) (ppdomain.Weight, bool) {
	unit, err := ppdomain.NewWeightUnit(weight.Unit().String())
	if err != nil {
		return ppdomain.Weight{}, false
	}
	translated, err := ppdomain.NewWeightFromString(weight.Value().String(), unit)
	if err != nil {
		return ppdomain.Weight{}, false
	}
	return translated, true
}

func declaredDimensions(dimensions psdomain.DeclaredDimensions) (ppdomain.Dimensions, bool) {
	unit, err := ppdomain.NewLengthUnit(dimensions.Unit().String())
	if err != nil {
		return ppdomain.Dimensions{}, false
	}
	sides := make([]ppdomain.Decimal, 0, 3)
	for _, raw := range []string{dimensions.Length().String(), dimensions.Width().String(), dimensions.Height().String()} {
		side, err := ppdomain.ParseDecimal(raw)
		if err != nil {
			return ppdomain.Dimensions{}, false
		}
		sides = append(sides, side)
	}
	translated, err := ppdomain.NewDimensions(sides[0], sides[1], sides[2], unit)
	if err != nil {
		return ppdomain.Dimensions{}, false
	}
	return translated, true
}
