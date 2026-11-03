package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证按运单号查询的编排（ADR-0356 决定二、三）：整单四格原样交回；逐件列出时每一件都走单件判据，委托属请求方也照问视图与
// 归属；任一读不动整页交回错误。单件三格的判据本身另由按包裹查的 HTTP 用例钉住（同一个 TrackCustomerParcel）。

type trackingViewsDouble struct {
	views map[string]domain.CustomerTrackingView
	// failOn 里的包裹读视图答读不动。
	failOn map[string]bool
	asked  []string
}

func (double *trackingViewsDouble) FindCurrent(
	_ context.Context, _ domain.TenantID, _ domain.CustomerAccountReference, parcel domain.TrackedParcelReference,
) (domain.CustomerTrackingView, bool, error) {
	double.asked = append(double.asked, parcel.String())
	if double.failOn[parcel.String()] {
		return domain.CustomerTrackingView{}, false, errors.New("view store down")
	}
	view, found := double.views[parcel.String()]
	return view, found, nil
}

type trackingAttributionsDouble struct {
	owners    map[string]string
	ambiguous map[string]bool
	err       error
	asked     []string
}

func (double *trackingAttributionsDouble) FindCustomerAccount(
	_ context.Context, _ domain.TenantID, parcel domain.TrackedParcelReference,
) (domain.CustomerAccountReference, bool, error) {
	double.asked = append(double.asked, parcel.String())
	if double.err != nil {
		return domain.CustomerAccountReference{}, false, double.err
	}
	if double.ambiguous[parcel.String()] {
		return domain.CustomerAccountReference{}, false, fmt.Errorf("%w: %s", ports.ErrAmbiguousCustomerAccount, parcel.String())
	}
	owner, found := double.owners[parcel.String()]
	if !found {
		return domain.CustomerAccountReference{}, false, nil
	}
	account, err := domain.NewCustomerAccountReference(owner)
	return account, err == nil, err
}

type shipmentLocatorDouble struct {
	located ports.LocatedCustomerShipment
	err     error
	pages   []ports.ShipmentParcelPage
}

func (double *shipmentLocatorDouble) LocateCustomerShipment(
	_ context.Context, _ domain.TenantID, _ domain.CustomerAccountReference, _ domain.ShipmentDocumentNumberReference,
	page ports.ShipmentParcelPage,
) (ports.LocatedCustomerShipment, error) {
	double.pages = append(double.pages, page)
	return double.located, double.err
}

func listedShipment(t *testing.T, parcels ...string) ports.LocatedCustomerShipment {
	t.Helper()
	located := ports.LocatedCustomerShipment{Standing: ports.ShipmentParcelsListed, Total: len(parcels)}
	for index, parcel := range parcels {
		located.Parcels = append(located.Parcels, ports.ShipmentParcel{
			Parcel:   mustValue(t, domain.NewTrackedParcelReference, parcel),
			Position: index + 1,
		})
	}
	return located
}

func publishedView(t *testing.T, customer, parcel string) domain.CustomerTrackingView {
	t.Helper()
	view, err := domain.PublishCustomerView(
		mustValue(t, domain.NewCustomerViewVersionID, "view-"+parcel),
		mustValue(t, domain.NewCustomerAccountReference, customer),
		mustValue(t, domain.NewTrackedParcelReference, parcel),
		mustValue(t, domain.NewProjectionVersionID, "projection-"+parcel),
		domain.CustomerViewDimensions{Milestones: domain.PendDimension(), ETA: domain.PendDimension(), Final: domain.PendDimension(), Note: domain.PendDimension()},
		time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func shipmentQuery(t *testing.T) application.CustomerShipmentTrackingQuery {
	t.Helper()
	return application.CustomerShipmentTrackingQuery{
		Tenant:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Customer: mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		Number:   mustValue(t, domain.NewShipmentDocumentNumberReference, "P20261102000001"),
		Page:     ports.ShipmentParcelPage{AfterPosition: 3, Limit: 50},
	}
}

// Covers: `AT-VE-231` 的编排半边——C1 名下一份已接受委托的三件：一件有视图答当前视图，一件视图未形成、唯一归属 C1 答当前无可见结果，
// 一件被另一份已接受委托同时声明答无权查询（VIEW_NOT_FOUND）。三件次序照 parcel-shipment 交来的次序；读到视图的那件不问归属，
// 另两件各问一次。页请求原样交给 parcel-shipment。
func TestAListedShipmentTracksEveryParcelByTheSingleParcelJudgement(t *testing.T) {
	locator := &shipmentLocatorDouble{located: listedShipment(t, "parcel-viewed", "parcel-pending", "parcel-conflict")}
	views := &trackingViewsDouble{views: map[string]domain.CustomerTrackingView{"parcel-viewed": publishedView(t, "customer-1", "parcel-viewed")}}
	attributions := &trackingAttributionsDouble{
		owners:    map[string]string{"parcel-viewed": "customer-1", "parcel-pending": "customer-1"},
		ambiguous: map[string]bool{"parcel-conflict": true},
	}

	tracked, err := application.TrackCustomerShipment(t.Context(), locator, views, attributions, shipmentQuery(t))
	if err != nil {
		t.Fatalf("按运单号查：%v", err)
	}
	if tracked.Standing != ports.ShipmentParcelsListed || tracked.Total != 3 || tracked.More {
		t.Fatalf("整单 = %s total=%d more=%v", tracked.Standing, tracked.Total, tracked.More)
	}
	var got []string
	for _, parcel := range tracked.Parcels {
		got = append(got, parcel.Parcel.String()+":"+parcel.Tracking.Outcome.String())
	}
	if fmt.Sprint(got) != "[parcel-viewed:CURRENT_VIEW parcel-pending:NO_VISIBLE_RESULT parcel-conflict:VIEW_NOT_FOUND]" {
		t.Fatalf("逐件 = %v", got)
	}
	if tracked.Parcels[0].Tracking.View.Version().String() != "view-parcel-viewed" {
		t.Fatalf("当前视图那件没带视图：%+v", tracked.Parcels[0].Tracking)
	}
	if fmt.Sprint(attributions.asked) != "[parcel-pending parcel-conflict]" {
		t.Fatalf("问归属的件 = %v, want 只问读不到视图的两件", attributions.asked)
	}
	if len(locator.pages) != 1 || locator.pages[0] != (ports.ShipmentParcelPage{AfterPosition: 3, Limit: 50}) {
		t.Fatalf("交给 parcel-shipment 的页 = %v", locator.pages)
	}
}

// Covers: `AT-VE-233` 与 `AT-VE-232` 的编排半边——待接受、没有服务、查无此单原样交回，不读视图也不问归属。
func TestTheOtherStandingsReadNoParcel(t *testing.T) {
	for _, standing := range []ports.ShipmentTrackingStanding{ports.ShipmentAwaitingAcceptance, ports.ShipmentClosedWithoutService, ports.ShipmentNotFound} {
		views, attributions := &trackingViewsDouble{}, &trackingAttributionsDouble{}
		tracked, err := application.TrackCustomerShipment(t.Context(), &shipmentLocatorDouble{located: ports.LocatedCustomerShipment{Standing: standing}},
			views, attributions, shipmentQuery(t))
		if err != nil || tracked.Standing != standing || len(tracked.Parcels) != 0 {
			t.Errorf("%s：%+v %v", standing, tracked, err)
		}
		if len(views.asked) != 0 || len(attributions.asked) != 0 {
			t.Errorf("%s 读了视图 %v、问了归属 %v", standing, views.asked, attributions.asked)
		}
	}
}

// Covers: `AT-VE-236` 的编排半边——委托读不动、某一件的视图读不动、某一件的归属读不动，整页都交回错误，不出半页；整单答复越出封闭
// 四格也是错误，不兜成查无此单。
func TestAnyUnreadablePieceLeavesTheWholePageUnformed(t *testing.T) {
	for name, testCase := range map[string]struct {
		locator      *shipmentLocatorDouble
		views        *trackingViewsDouble
		attributions *trackingAttributionsDouble
	}{
		"shipment read fails": {&shipmentLocatorDouble{err: errors.New("parcel shipment down")}, &trackingViewsDouble{}, &trackingAttributionsDouble{}},
		"one view read fails": {&shipmentLocatorDouble{located: listedShipment(t, "parcel-1", "parcel-2")},
			&trackingViewsDouble{failOn: map[string]bool{"parcel-2": true}}, &trackingAttributionsDouble{}},
		"one attribution read fails": {&shipmentLocatorDouble{located: listedShipment(t, "parcel-1")},
			&trackingViewsDouble{}, &trackingAttributionsDouble{err: errors.New("parcel shipment down")}},
		"standing outside the four": {&shipmentLocatorDouble{located: ports.LocatedCustomerShipment{}}, &trackingViewsDouble{}, &trackingAttributionsDouble{}},
	} {
		tracked, err := application.TrackCustomerShipment(t.Context(), testCase.locator, testCase.views, testCase.attributions, shipmentQuery(t))
		if err == nil || tracked.Standing != ports.ShipmentTrackingStandingInvalid || len(tracked.Parcels) != 0 {
			t.Errorf("%s：%+v %v, want 整页未形成", name, tracked, err)
		}
	}
}
