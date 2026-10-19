package parcelshipment

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// DeclaredMeasurementLookup 是本适配器向 parcel-shipment 取申报测量的窄口。真实装配交给 PS 的
// adapters/postgres.ShipmentRequests（它兼实现 PS ports.DeclaredMeasurementView）；只声明读的这一个方法，理由同
// DeliveryPlaceReferenceLookup。
type DeclaredMeasurementLookup interface {
	LoadDeclaredMeasurement(
		ctx context.Context,
		tenant psdomain.TenantID,
		parcel psdomain.DeclaredParcelID,
	) (psdomain.DeclaredMeasurementResolution, error)
}

// DeclaredDemand 实现 tfports.DeclaredDemandSource：把 PS 申报测量的封闭五格译成需求量快照的原料，交领域按内置取法
// 成快照（ADR-0201 决定八）。体积怎样由三边得出、件数为什么恒为一，归领域那一处，这里只抄数与单位。
type DeclaredDemand struct {
	measurements DeclaredMeasurementLookup
}

// NewDeclaredDemand 读口必备：缺了它这一缝永远答不出，而「答不出」在端口上会长成 error，日后有人会把它读成 PS 坏了。
func NewDeclaredDemand(measurements DeclaredMeasurementLookup) (*DeclaredDemand, error) {
	if measurements == nil {
		return nil, fmt.Errorf("transport fulfillment parcelshipment adapter: declared measurement lookup is nil")
	}
	return &DeclaredDemand{measurements: measurements}, nil
}

var _ tfports.DeclaredDemandSource = (*DeclaredDemand)(nil)

// declarationAnchorOnBaseline 是锚在接受基线上时快照写下的锚词。与计价侧申报测量引用的写法一致：同一份申报在两个上下文
// 的依据里读作同一个锚。
const declarationAnchorOnBaseline = "baseline"

// LoadDeclaredDemand 是全函数，五格各有落点、无 default：
//
//   - 基线锚带测量 → 锚 `baseline`，重量与外廓照抄；
//   - 已采用版本锚 → 锚取那一版的资料版本标识；那一版清空了测量时只带锚——绝不回退到基线值，那等于把已被客户更正的
//     申报当现行申报用；
//   - 未定、未申报、无 → 不带锚也不带重量与外廓：此刻说不出按哪一版、客户没报、对象不是已接受委托的成员，在需求量上
//     都是重量与体积`待补充`。三格不是读不动，不答 error——答 error 会让每封信封重投到放弃，而重投不会让客户补报。
//
// PS 返 error（读不到、坏行、同一包裹被多份已接受委托声明）原样上抛，准备编排答未决。
func (adapter *DeclaredDemand) LoadDeclaredDemand(
	ctx context.Context,
	tenant tfdomain.TenantID,
	object tfdomain.CarriedObjectReference,
) (tfdomain.DemandSnapshot, error) {
	psTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return tfdomain.DemandSnapshot{}, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	parcel, err := psdomain.NewDeclaredParcelID(object.String())
	if err != nil {
		return tfdomain.DemandSnapshot{}, fmt.Errorf("%w: declared parcel: %v", ErrUntranslatableAnswer, err)
	}
	resolution, err := adapter.measurements.LoadDeclaredMeasurement(ctx, psTenant, parcel)
	if err != nil {
		return tfdomain.DemandSnapshot{}, fmt.Errorf("load declared measurement: %w", err)
	}

	var spec tfdomain.DeclaredDemandSpec
	switch resolution.Outcome() {
	case psdomain.DeclaredMeasurementAnchoredOnBaseline:
		spec.Anchor = declarationAnchorOnBaseline
	case psdomain.DeclaredMeasurementAnchoredOnAdoptedVersion:
		anchor, _ := resolution.Anchor()
		version, adopted := anchor.AdoptedVersion()
		if !adopted {
			return tfdomain.DemandSnapshot{}, fmt.Errorf("%w: adopted version anchor carries no version", ErrUntranslatableAnswer)
		}
		spec.Anchor = version.String()
	case psdomain.DeclaredMeasurementUndetermined,
		psdomain.DeclaredMeasurementNotDeclared,
		psdomain.NoDeclaredMeasurement:
		return translatedDemand(spec)
	default:
		return tfdomain.DemandSnapshot{}, fmt.Errorf("%w: declared measurement outcome %q", ErrUntranslatableAnswer, resolution.Outcome())
	}

	if measurement, present := resolution.Measurement(); present {
		spec.WeightValue = measurement.Weight().Value().String()
		spec.WeightUnit = measurement.Weight().Unit().String()
		if dimensions, declared := measurement.Dimensions(); declared {
			spec.Length = dimensions.Length().String()
			spec.Width = dimensions.Width().String()
			spec.Height = dimensions.Height().String()
			spec.DimensionUnit = dimensions.Unit().String()
		}
	}
	return translatedDemand(spec)
}

func translatedDemand(spec tfdomain.DeclaredDemandSpec) (tfdomain.DemandSnapshot, error) {
	snapshot, err := tfdomain.DeclaredDemand(spec)
	if err != nil {
		return tfdomain.DemandSnapshot{}, fmt.Errorf("%w: declared measurement: %v", ErrUntranslatableAnswer, err)
	}
	return snapshot, nil
}
