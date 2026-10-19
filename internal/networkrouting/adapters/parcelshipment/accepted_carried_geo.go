package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var (
	// ErrAcceptedParcelUnreadable 说明此刻读不动 parcel-shipment：读口报错，或反查命中了一份已接受委托、聚合却读不回
	// 或已不在已接受。后两种是两次读之间委托在动，重投时反查会给出一致的答案。生产装配把它登进交接触发那一路的未决
	// 哨兵。
	ErrAcceptedParcelUnreadable = errors.New(
		"network routing parcelshipment adapter: the current accepted parcel is not readable right now")
	// ErrAcceptedRequestInconsistent 说明读回的已接受委托自相矛盾：没有受理基线，基线指的提交版本在聚合里找不到，或
	// 聚合与反查指着两份委托。仓储不变量已破，重投不会变，禁止进未决哨兵（ADR-0029）。
	ErrAcceptedRequestInconsistent = errors.New(
		"network routing parcelshipment adapter: the accepted shipment request contradicts itself")
)

// CurrentAcceptedParcelLookup 按（租户，包裹）反查当前已接受委托。由 parcel-shipment 的委托仓储满足
// （*pspostgres.ShipmentRequests 的同名方法）；只声明读的那一个方法，消费侧拿到的不该是 PS 的写口。
type CurrentAcceptedParcelLookup interface {
	FindCurrentAcceptedByParcel(
		ctx context.Context,
		tenant psdomain.TenantID,
		parcel psdomain.DeclaredParcelID,
	) (psdomain.CurrentAcceptedParcelTarget, bool, error)
}

// AcceptedCarriedGeo 是一件当前已接受包裹在路由判断键上的各维（缺服务目的，那是装配参数）与受理基线那一版提交的
// 地理解析投影。只有地理维：地址本体与身份内容都不在这里（ADR-0075 决定二仍成立）。
type AcceptedCarriedGeo struct {
	CustomerAccountID  nrdomain.CustomerAccountID
	ShipmentRequestID  nrdomain.ShipmentRequestID
	AcceptanceBaseline nrdomain.AcceptanceBaselineReference
	Geo                nrdomain.GeoResolutionProjection
}

// AcceptedCarriedGeoSource 是受理之后、不由 PS 信封触发的复核取所携地理投影的地方（ADR-0215，部分停用 ADR-0075
// 决定一）。交接触发的复核没有 PS 的请求可随带投影，所以由本适配器在消费侧按（租户，包裹）去读。
//
// 读的是受理基线那一版提交，不是当前提交版本：基线一经固定不再改，ADR-0075 防的「判断对象与判断输入异版」在这里
// 不存在；所取维度与接受决定、收寄采用两封信封所携的同一份投影，复核的判断键也落在同一条基线上。
type AcceptedCarriedGeoSource struct {
	targets  CurrentAcceptedParcelLookup
	requests AcceptedRequestSource
}

func NewAcceptedCarriedGeoSource(
	targets CurrentAcceptedParcelLookup,
	requests AcceptedRequestSource,
) (*AcceptedCarriedGeoSource, error) {
	if targets == nil {
		return nil, fmt.Errorf("network routing parcelshipment adapter: current accepted parcel lookup is nil")
	}
	if requests == nil {
		return nil, fmt.Errorf("network routing parcelshipment adapter: request source is nil")
	}
	return &AcceptedCarriedGeoSource{targets: targets, requests: requests}, nil
}

// LoadAcceptedCarriedGeo 答这件包裹此刻是不是一件当前已接受包裹，是则交回判断键各维与基线那一版的投影。
//
// 「不是」是业务答案（found=false）：没有当前已接受委托声明它，或声明它的委托在受理基线里没有它——集运单元号照原样
// 进反查，同样落这一格。反查歧义（两份已接受委托都声明它）原样上抛，要人去看，不挑一份（ADR-0060）。
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

	target, found, err := source.targets.FindCurrentAcceptedByParcel(ctx, psTenant, psParcel)
	if errors.Is(err, psdomain.ErrAmbiguousParcelTarget) {
		return none, false, fmt.Errorf("find current accepted parcel %q: %w", parcel, err)
	}
	if err != nil {
		return none, false, fmt.Errorf("%w: find current accepted parcel %q: %v", ErrAcceptedParcelUnreadable, parcel, err)
	}
	if !found {
		return none, false, nil
	}

	request, found, err := source.requests.FindBySourceIdentity(ctx, target.Identity())
	if err != nil {
		return none, false, fmt.Errorf("%w: load shipment request %q: %v",
			ErrAcceptedParcelUnreadable, target.ShipmentRequestID(), err)
	}
	if !found || request.State() != psdomain.ShipmentRequestAccepted {
		return none, false, fmt.Errorf("%w: shipment request %q is no longer read back as accepted",
			ErrAcceptedParcelUnreadable, target.ShipmentRequestID())
	}
	if request.ShipmentRequestID() != target.ShipmentRequestID() {
		return none, false, fmt.Errorf("%w: lookup names request %q, aggregate is %q",
			ErrAcceptedRequestInconsistent, target.ShipmentRequestID(), request.ShipmentRequestID())
	}
	baseline, present := request.AcceptanceBaseline()
	if !present {
		return none, false, fmt.Errorf("%w: accepted request %q carries no acceptance baseline",
			ErrAcceptedRequestInconsistent, target.ShipmentRequestID())
	}
	if !baselineCovers(baseline, psParcel) {
		return none, false, nil
	}
	submission, present := request.SubmissionVersionByID(baseline.SubmissionVersionID())
	if !present {
		return none, false, fmt.Errorf("%w: baseline submission version %q is not in request %q",
			ErrAcceptedRequestInconsistent, baseline.SubmissionVersionID(), target.ShipmentRequestID())
	}

	customer, err := nrdomain.NewCustomerAccountID(target.Identity().CustomerAccountID().String())
	if err != nil {
		return none, false, fmt.Errorf("%w: customer: %v", ErrUntranslatableAnswer, err)
	}
	shipment, err := nrdomain.NewShipmentRequestID(target.ShipmentRequestID().String())
	if err != nil {
		return none, false, fmt.Errorf("%w: shipment request: %v", ErrUntranslatableAnswer, err)
	}
	// 基线引用取基线的提交版本标识，与 RouteOnAcceptanceAdapter、ReassessOnIntakeAdapter 同源：三条链必须用同一个
	// 字符串指同一份基线，否则同一件包裹的初始判断与各次复核会落在不同的判断键上。
	reference, err := nrdomain.NewAcceptanceBaselineReference(baseline.SubmissionVersionID().String())
	if err != nil {
		return none, false, fmt.Errorf("%w: acceptance baseline: %v", ErrUntranslatableAnswer, err)
	}
	return AcceptedCarriedGeo{
		CustomerAccountID:  customer,
		ShipmentRequestID:  shipment,
		AcceptanceBaseline: reference,
		Geo:                geoFromProjection(submission.GeoProjection()),
	}, true, nil
}

func baselineCovers(baseline psdomain.AcceptanceBaseline, parcel psdomain.DeclaredParcelID) bool {
	for _, member := range baseline.DeclaredParcelIDs() {
		if member == parcel {
			return true
		}
	}
	return false
}

// geoFromProjection 逐格照抄 PS 的投影，与 PS 信封的线格式（经 geoFromCarried 译回）同一套取值：两条路到 NR 的投影
// 摘要必须相等，复核的触发指纹才不因取法不同而变。
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
