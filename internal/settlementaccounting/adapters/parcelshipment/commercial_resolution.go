// Package parcelshipment 是 settlement-accounting 读 parcel-shipment 的消费侧适配器（ADR-0025：`internal/<consumer>/
// adapters/<provider>/`）。跨上下文翻译只落在这里，`application` 不 import 小包托运；本包只读提供方的只读半边，不写
// 它的库。它做两件事：答「这件包裹在委托接受时固定的商业解析」（包裹 → 结算账户那条链的第一段），以及把收寄采用与
// 终局两种信封按键回读、译成包裹计费时点交给发起编排（ADR-0190 决定二）。
package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrUntranslatableAnswer 表示某一侧交出了本适配器词汇表之外的内容——编程错误或提供方阶段契约被打破，不是业务答案，
// 重投不自愈。哨兵与本仓其余跨上下文适配器同名同义。
var ErrUntranslatableAnswer = errors.New("settlement accounting parcelshipment adapter: untranslatable answer")

// CommercialResolutionReferenceSource 是本适配器向小包托运取商业解析回指的窄口。真实装配交给 PS 的
// adapters/postgres.ShipmentRequests（它兼实现 PS 的 CommercialResolutionReferenceView）；这里只声明读的那一个方法，
// 写口在类型上就不进本上下文。
type CommercialResolutionReferenceSource interface {
	LoadCommercialResolutionReference(
		ctx context.Context,
		tenant psdomain.TenantID,
		parcel psdomain.DeclaredParcelID,
	) (psdomain.CommercialResolutionID, bool, error)
}

// ParcelCommercialResolutions 实现 saports.ParcelCommercialResolutionView：按（租户，包裹）向小包托运问委托接受时固定的
// 商业解析回指，原样译成本上下文的回指。受理前财务控制拿的也是这一份回指（那一侧经委托与提交版本问到，这一侧按
// 包裹问到），所以两拍落到的结算政策是同一份。
//
// 三格照提供方原样：回指；没有（包裹不属任何已接受委托，含谱系未建模的拆分合并包裹）；error（含「已接受却缺回指」
// 这种接受流的装配缺陷）。不把 error 折成「没有」：那会让一份读坏的回指看起来像这件包裹不该计费。
type ParcelCommercialResolutions struct {
	source CommercialResolutionReferenceSource
}

func NewParcelCommercialResolutions(source CommercialResolutionReferenceSource) (*ParcelCommercialResolutions, error) {
	if source == nil {
		return nil, fmt.Errorf("settlement accounting parcelshipment adapter: commercial resolution reference source is nil")
	}
	return &ParcelCommercialResolutions{source: source}, nil
}

var _ saports.ParcelCommercialResolutionView = (*ParcelCommercialResolutions)(nil)

// LoadParcelCommercialResolution 租户与包裹在两个上下文里是同一串字面，回指同样逐字带过来，不拆不拼。
func (view *ParcelCommercialResolutions) LoadParcelCommercialResolution(
	ctx context.Context,
	tenant sadomain.TenantID,
	parcel sadomain.ParcelReference,
) (sadomain.CommercialResolutionReference, bool, error) {
	none := sadomain.CommercialResolutionReference{}
	psTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, false, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	psParcel, err := psdomain.NewDeclaredParcelID(parcel.String())
	if err != nil {
		return none, false, fmt.Errorf("%w: parcel: %v", ErrUntranslatableAnswer, err)
	}
	resolution, present, err := view.source.LoadCommercialResolutionReference(ctx, psTenant, psParcel)
	if err != nil {
		return none, false, fmt.Errorf("load parcel commercial resolution: %w", err)
	}
	if !present {
		return none, false, nil
	}
	reference, err := sadomain.NewCommercialResolutionReference(resolution.String())
	if err != nil {
		// 答了「有」却交回空回指：提供方读口违约，不是「没有」。
		return none, false, fmt.Errorf("%w: commercial resolution reference: %v", ErrUntranslatableAnswer, err)
	}
	return reference, true, nil
}
