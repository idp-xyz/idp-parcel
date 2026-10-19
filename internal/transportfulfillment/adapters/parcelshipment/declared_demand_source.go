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

// DeclaredDemand 实现 tfports.DeclaredDemandSource：把 PS 申报测量的封闭各格译成需求量快照的原料，交领域按内置取法
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

// errDemandUntranslatable 既是本包的 ErrUntranslatableAnswer，也是端口的 ErrDeclaredDemandUntranslatable：前者与其余跨
// 上下文适配器同义，后者让准备编排认出这一格不是读不动、不答未决。包级的 ErrUntranslatableAnswer 不直接包端口哨兵——
// 交付地点那一缝也用它，那边译不过去说的不是申报需求量。
var errDemandUntranslatable = fmt.Errorf("%w: %w", ErrUntranslatableAnswer, tfports.ErrDeclaredDemandUntranslatable)

// declarationAnchorOnBaseline 是锚在接受基线上时快照写下的锚词。与计价侧申报测量引用的写法一致：同一份申报在两个上下文
// 的依据里读作同一个锚。
const declarationAnchorOnBaseline = "baseline"

// LoadDeclaredDemand 是全函数，PS 的每一格都有落点，集外取值答译不过去：
//
//   - 基线锚带测量 → 锚 `baseline`，重量与外廓照抄；
//   - 已采用版本锚 → 锚取那一版的资料版本标识；那一版清空了测量时只带锚——绝不回退到基线值，那等于把已被客户更正的
//     申报当现行申报用；
//   - 未定、未申报、无 → 不带锚也不带重量与外廓：此刻说不出按哪一版、客户没报、对象不是已接受委托的成员，在需求量上
//     都是重量与体积`待补充`。这几格都不答 error，理由不尽相同：未申报与无，重投不会让客户补报；未定是当前采用判断
//     `待复核`，会由复核解开，可复核没有时限，答 error 只会把信封重投到放弃。复核出结论之后，这一次形成的结果不随之
//     重备（ADR-0201 票 18 补记的越权风险点「待复核期间的结果不随复核重备」）。
//
// PS 返 error（读不到、坏行、同一包裹被多份已接受委托声明）原样上抛，准备编排答未决——PS 的读口不分这几种，本缝也
// 分不出。集外取值、申报立不成快照、本上下文的值在 PS 立不起来，都包住 errDemandUntranslatable，编排原样上抛不答未决。
func (adapter *DeclaredDemand) LoadDeclaredDemand(
	ctx context.Context,
	tenant tfdomain.TenantID,
	object tfdomain.CarriedObjectReference,
) (tfdomain.DemandSnapshot, error) {
	psTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return tfdomain.DemandSnapshot{}, fmt.Errorf("%w: tenant: %v", errDemandUntranslatable, err)
	}
	parcel, err := psdomain.NewDeclaredParcelID(object.String())
	if err != nil {
		return tfdomain.DemandSnapshot{}, fmt.Errorf("%w: declared parcel: %v", errDemandUntranslatable, err)
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
			return tfdomain.DemandSnapshot{}, fmt.Errorf("%w: adopted version anchor carries no version", errDemandUntranslatable)
		}
		spec.Anchor = version.String()
	case psdomain.DeclaredMeasurementUndetermined,
		psdomain.DeclaredMeasurementNotDeclared,
		psdomain.NoDeclaredMeasurement:
		return translatedDemand(spec)
	default:
		return tfdomain.DemandSnapshot{}, fmt.Errorf("%w: declared measurement outcome %q", errDemandUntranslatable, resolution.Outcome())
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
		return tfdomain.DemandSnapshot{}, fmt.Errorf("%w: declared measurement: %v", errDemandUntranslatable, err)
	}
	return snapshot, nil
}
