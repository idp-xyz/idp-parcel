package parcelshipment

import (
	"context"
	"errors"
	"fmt"
	"time"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var (
	// ErrAcceptedParcelUnreadable 说明此刻读不动 parcel-shipment：受理基线地理投影读口报错，且不是歧义、也不是基线自相
	// 矛盾。生产装配把它登进交接触发与收寄采用两路复核的未决哨兵。
	ErrAcceptedParcelUnreadable = errors.New(
		"network routing parcelshipment adapter: the current accepted parcel is not readable right now")
	// ErrAcceptedRequestInconsistent 说明读回的已接受委托自相矛盾：PS 读口答接受基线自相矛盾（各形见
	// psdomain.ErrAcceptanceBaselineInconsistent 头注），或与一份收寄采用记录指着两份委托、两条基线。仓储不变量已破，
	// 重投不会变，禁止进未决哨兵（ADR-0029）。
	ErrAcceptedRequestInconsistent = errors.New(
		"network routing parcelshipment adapter: the accepted shipment request contradicts itself")
)

// AcceptedCarriedGeo 是一件当前已接受包裹在路由判断键上的各维（缺服务目的，那是装配参数）、委托接受的时刻与受理基线
// 那一版提交的地理解析投影。只有地理维：地址本体与身份内容都不在这里（ADR-0075 决定二仍成立）。
type AcceptedCarriedGeo struct {
	CustomerAccountID  nrdomain.CustomerAccountID
	ShipmentRequestID  nrdomain.ShipmentRequestID
	AcceptanceBaseline nrdomain.AcceptanceBaselineReference
	// AcceptedAt 是接受基线固定的时刻，与初始路由交接带来的接受时刻同源：复核拿它作业务时间下限（ADR-0279），两处
	// 取法一不同，初始计划的生效与复核的下限就对不上。
	AcceptedAt time.Time
	Geo        nrdomain.GeoResolutionProjection
}

// AcceptedCarriedGeoSource 是受理之后、不由 PS 信封触发的复核取所携地理投影的地方（ADR-0215，部分停用 ADR-0075
// 决定一）。交接触发的复核没有 PS 的请求可随带投影，所以由本适配器在消费侧按（租户，包裹）去读。
//
// 读的是 PS 的受理基线地理投影读口（ADR-0301），不是委托聚合：这一路经手的只有判断键要的几格引用、接受时刻与地理维，
// 聚合的其余部分（申报测量、接受决定与商业依据、接受后的资料修订等）不进来。读的是受理基线那一版提交，不是当前提交
// 版本：基线一经固定不再改，ADR-0075 防的「判断对象与判断输入异版」在这里不存在；所取维度与接受决定、收寄采用两封
// 信封所携的同一份投影，复核的判断键也落在同一条基线上。
type AcceptedCarriedGeoSource struct {
	projections psports.AcceptanceBaselineGeoProjectionView
}

func NewAcceptedCarriedGeoSource(
	projections psports.AcceptanceBaselineGeoProjectionView,
) (*AcceptedCarriedGeoSource, error) {
	if projections == nil {
		return nil, fmt.Errorf("network routing parcelshipment adapter: acceptance baseline geo projection view is nil")
	}
	return &AcceptedCarriedGeoSource{projections: projections}, nil
}

// LoadAcceptedCarriedGeo 答这件包裹此刻是不是一件当前已接受包裹，是则交回判断键各维与基线那一版的投影。
//
// 「不是」是业务答案（found=false）：没有当前已接受委托声明它，或声明它的委托在受理基线里没有它——集运单元号照原样
// 进读口，同样落这一格。反查歧义（两份已接受委托都声明它）原样上抛，要人去看，不挑一份（ADR-0060）。
func (source *AcceptedCarriedGeoSource) LoadAcceptedCarriedGeo(
	ctx context.Context,
	tenant nrdomain.TenantID,
	parcel nrdomain.DeclaredParcelID,
) (AcceptedCarriedGeo, bool, error) {
	none := AcceptedCarriedGeo{}
	psTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, false, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	psParcel, err := psdomain.NewDeclaredParcelID(parcel.String())
	if err != nil {
		return none, false, fmt.Errorf("%w: parcel: %v", ErrUntranslatableAnswer, err)
	}

	projection, found, err := source.projections.LoadAcceptanceBaselineGeoProjection(ctx, psTenant, psParcel)
	switch {
	case errors.Is(err, psdomain.ErrAmbiguousParcelTarget):
		return none, false, fmt.Errorf("load acceptance baseline geo of parcel %q: %w", parcel, err)
	case errors.Is(err, psdomain.ErrAcceptanceBaselineInconsistent):
		return none, false, fmt.Errorf("%w: parcel %q: %v", ErrAcceptedRequestInconsistent, parcel, err)
	case err != nil:
		return none, false, fmt.Errorf("%w: parcel %q: %v", ErrAcceptedParcelUnreadable, parcel, err)
	case !found:
		return none, false, nil
	}

	customer, err := nrdomain.NewCustomerAccountID(projection.CustomerAccountID().String())
	if err != nil {
		return none, false, fmt.Errorf("%w: customer: %v", ErrUntranslatableAnswer, err)
	}
	shipment, err := nrdomain.NewShipmentRequestID(projection.ShipmentRequestID().String())
	if err != nil {
		return none, false, fmt.Errorf("%w: shipment request: %v", ErrUntranslatableAnswer, err)
	}
	// 基线引用取基线的提交版本标识，与 RouteOnAcceptanceAdapter、ReassessOnIntakeAdapter 同源：这几条链必须用同一个
	// 字符串指同一份基线，否则同一件包裹的初始判断与各次复核会落在不同的判断键上。
	reference, err := nrdomain.NewAcceptanceBaselineReference(projection.BaselineVersion().String())
	if err != nil {
		return none, false, fmt.Errorf("%w: acceptance baseline: %v", ErrUntranslatableAnswer, err)
	}
	return AcceptedCarriedGeo{
		CustomerAccountID:  customer,
		ShipmentRequestID:  shipment,
		AcceptanceBaseline: reference,
		AcceptedAt:         projection.FixedAt(),
		Geo:                geoFromProjection(projection.Geo()),
	}, true, nil
}

// geoFromProjection 逐格照抄 PS 的投影，与 PS 信封的线格式（经 geoFromCarried 译回）同一套取值：随信封携带与经本适配器读取，
// 到 NR 的投影摘要必须相等，复核的触发指纹才不因取法不同而变。
func geoFromProjection(projection psdomain.CarriedGeoProjection) nrdomain.GeoResolutionProjection {
	if !projection.Carried() {
		return nrdomain.GeoResolutionProjection{}
	}
	senderCountry, hasSenderCountry := projection.Sender().Country()
	senderPostal, hasSenderPostal := projection.Sender().PostalCode()
	deliveryCountry, hasDeliveryCountry := projection.Delivery().Country()
	deliveryPostal, hasDeliveryPostal := projection.Delivery().PostalCode()
	return nrdomain.NewGeoResolutionProjection(
		nrdomain.NewGeoResolutionSide(senderCountry, hasSenderCountry, senderPostal, hasSenderPostal),
		nrdomain.NewGeoResolutionSide(deliveryCountry, hasDeliveryCountry, deliveryPostal, hasDeliveryPostal),
	)
}
