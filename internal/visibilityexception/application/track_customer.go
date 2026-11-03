package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// CustomerParcelTrackingOutcome 是客户追踪查询对一件包裹的封闭三格（ADR-0246 决定一），按客户系统的续办分（ADR-0029）。按包裹查
// 答的是它，按运单号查时列出的每一件答的也是它（ADR-0356 决定三）。
type CustomerParcelTrackingOutcome uint8

const (
	CustomerParcelTrackingOutcomeInvalid CustomerParcelTrackingOutcome = iota
	// CustomerParcelCurrentView：读到请求方账户下这件包裹的当前视图；各维待确认或不展示也是这一格（ADR-0246 决定四）。
	CustomerParcelCurrentView
	// CustomerParcelNoVisibleResult：读不到视图，而 parcel-shipment 的当前已接受委托把这件包裹唯一归属于请求方账户。
	CustomerParcelNoVisibleResult
	// CustomerParcelViewNotFound：其余一切读不到视图的情形，同一格（ADR-0246 决定一、五）。
	CustomerParcelViewNotFound
)

func (outcome CustomerParcelTrackingOutcome) String() string {
	switch outcome {
	case CustomerParcelCurrentView:
		return "CURRENT_VIEW"
	case CustomerParcelNoVisibleResult:
		return "NO_VISIBLE_RESULT"
	case CustomerParcelViewNotFound:
		return "VIEW_NOT_FOUND"
	default:
		return ""
	}
}

// CustomerParcelTracking 是一件包裹的查询结果；View 只随 CustomerParcelCurrentView 在场。
type CustomerParcelTracking struct {
	Outcome CustomerParcelTrackingOutcome
	View    domain.CustomerTrackingView
}

// TrackCustomerParcel 判一件包裹答哪一格，是单件判据唯一的一处：按包裹查与按运单号逐件列出都经这里（ADR-0356 决定三「单件的判据
// 只在一处定义」）。
//
// 先读视图；读到即答，热路径不多一次读。读不到才问一次归属，每一次读不到视图都问且只问一次，不论答出什么——问不问取决于答案的话，
// 读的次数本身就成了探针看得见的区别（ADR-0246 决定二、三）。比的只是「唯一归属账户等于请求方账户」这一位。多于一份当前已接受
// 委托同时声明（ports.ErrAmbiguousCustomerAccount）是读不到视图的又一种，答 VIEW_NOT_FOUND（决定五）。
//
// 视图读不回、归属读不动或译不成，都没形成结论，交回错误：降成任何一格都会让客户系统把一次该重试的故障当成终局（UC-VE-008
// AT-VE-168）。视图读不回时不再问归属——没有可依据的结论，问了也无从作答。
func TrackCustomerParcel(
	ctx context.Context,
	views ports.CurrentCustomerViewReader,
	attributions ports.ParcelCustomerAccountView,
	tenant domain.TenantID,
	customer domain.CustomerAccountReference,
	parcel domain.TrackedParcelReference,
) (CustomerParcelTracking, error) {
	view, found, err := views.FindCurrent(ctx, tenant, customer, parcel)
	if err != nil {
		return CustomerParcelTracking{}, fmt.Errorf("track customer parcel: read the current view: %w", err)
	}
	if found {
		return CustomerParcelTracking{Outcome: CustomerParcelCurrentView, View: view}, nil
	}
	account, attributed, err := attributions.FindCustomerAccount(ctx, tenant, parcel)
	switch {
	case errors.Is(err, ports.ErrAmbiguousCustomerAccount):
		return CustomerParcelTracking{Outcome: CustomerParcelViewNotFound}, nil
	case err != nil:
		return CustomerParcelTracking{}, fmt.Errorf("track customer parcel: find the customer account: %w", err)
	case attributed && account == customer:
		return CustomerParcelTracking{Outcome: CustomerParcelNoVisibleResult}, nil
	default:
		return CustomerParcelTracking{Outcome: CustomerParcelViewNotFound}, nil
	}
}

// CustomerShipmentTrackingQuery 是一次按运单号的追踪查询：租户与账户来自认证结果，号与页来自请求与渠道契约。
type CustomerShipmentTrackingQuery struct {
	Tenant   domain.TenantID
	Customer domain.CustomerAccountReference
	Number   domain.ShipmentDocumentNumberReference
	Page     ports.ShipmentParcelPage
}

// TrackedShipmentParcel 是逐件列出的一件：parcel-shipment 给的那一件，加上它按单件判据答出的那一格。
type TrackedShipmentParcel struct {
	ports.ShipmentParcel
	Tracking CustomerParcelTracking
}

// CustomerShipmentTracking 是按运单号查询的结果：整单那一格，列出时另带本页各件、全集件数与本页之后还有没有。
type CustomerShipmentTracking struct {
	Standing ports.ShipmentTrackingStanding
	Parcels  []TrackedShipmentParcel
	Total    int
	More     bool
}

// TrackCustomerShipment 编排按运单号的查询（ADR-0356 决定二、三）：先问 parcel-shipment 这个号在请求方账户名下是哪一格，逐件列出时
// 对本页每一件走 TrackCustomerParcel。
//
// 委托属请求方不省掉逐件的归属核对：委托是请求方的，不等于这件包裹此刻唯一归属请求方（例如另有已接受委托同时声明它），省掉了，
// 同一件在两口上就可能答出不同的格。任一读不动整页交回错误，不出半页：依赖故障的续办是整页退避重发，逐件另立「未形成」一格会让
// 客户系统多处置一种逐件状态（ADR-0356 决定二；UC-VE-008 规则里按单据号查询那一句例外）。
func TrackCustomerShipment(
	ctx context.Context,
	shipments ports.CustomerShipmentTrackingView,
	views ports.CurrentCustomerViewReader,
	attributions ports.ParcelCustomerAccountView,
	query CustomerShipmentTrackingQuery,
) (CustomerShipmentTracking, error) {
	located, err := shipments.LocateCustomerShipment(ctx, query.Tenant, query.Customer, query.Number, query.Page)
	if err != nil {
		return CustomerShipmentTracking{}, fmt.Errorf("track customer shipment: locate the shipment: %w", err)
	}
	switch located.Standing {
	case ports.ShipmentAwaitingAcceptance, ports.ShipmentClosedWithoutService, ports.ShipmentNotFound:
		return CustomerShipmentTracking{Standing: located.Standing}, nil
	case ports.ShipmentParcelsListed:
	default:
		return CustomerShipmentTracking{}, fmt.Errorf("track customer shipment: standing %d is outside the closed four", located.Standing)
	}

	tracked := make([]TrackedShipmentParcel, 0, len(located.Parcels))
	for _, parcel := range located.Parcels {
		tracking, err := TrackCustomerParcel(ctx, views, attributions, query.Tenant, query.Customer, parcel.Parcel)
		if err != nil {
			return CustomerShipmentTracking{}, fmt.Errorf("track customer shipment: parcel %q: %w", parcel.Parcel.String(), err)
		}
		tracked = append(tracked, TrackedShipmentParcel{ShipmentParcel: parcel, Tracking: tracking})
	}
	return CustomerShipmentTracking{
		Standing: ports.ShipmentParcelsListed,
		Parcels:  tracked,
		Total:    located.Total,
		More:     located.More,
	}, nil
}
