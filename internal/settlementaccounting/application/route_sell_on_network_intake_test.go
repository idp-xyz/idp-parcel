package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

type shipmentPictureDouble struct {
	picture ports.ShipmentIntakePicture
	found   bool
}

func (double shipmentPictureDouble) LoadByParcel(context.Context, domain.TenantID, domain.ParcelReference) (ports.ShipmentIntakePicture, bool, error) {
	return double.picture, double.found, nil
}

type shipmentCardDouble struct {
	path ports.SellPriceCardPath
}

func (double shipmentCardDouble) ResolveSellPriceCardPath(
	context.Context, domain.TenantID, domain.EstimatePricingScope, time.Time, string,
) (ports.SellPriceCardPath, error) {
	return double.path, nil
}

type shipmentAdoptionDouble struct {
	adoption domain.ShipmentBillingMomentAdoption
	found    bool
}

func (double shipmentAdoptionDouble) FindShipmentBillingMomentAdoption(
	context.Context, domain.TenantID, domain.SettlementAccountID,
) (domain.ShipmentBillingMomentAdoption, bool, error) {
	return double.adoption, double.found, nil
}

type shipmentRegistryDouble struct {
	saved []ports.ShipmentSellEvaluationRequestRecord
}

func (double *shipmentRegistryDouble) Save(_ context.Context, record ports.ShipmentSellEvaluationRequestRecord) (ports.EvaluationRequestSaveOutcome, error) {
	double.saved = append(double.saved, record)
	return ports.EvaluationRequestSaved, nil
}

func (double *shipmentRegistryDouble) FindByNaturalKey(
	context.Context, domain.TenantID, domain.ShipmentSellEvaluationRequestNaturalKey,
) (ports.ShipmentSellEvaluationRequestRecord, bool, error) {
	if len(double.saved) == 0 {
		return ports.ShipmentSellEvaluationRequestRecord{}, false, nil
	}
	return double.saved[len(double.saved)-1], true, nil
}

func (double *shipmentRegistryDouble) FindByID(context.Context, ports.ShipmentSellEvaluationRequestKey) (ports.ShipmentSellEvaluationRequestRecord, bool, error) {
	return double.FindByNaturalKey(context.Background(), domain.TenantID{}, domain.ShipmentSellEvaluationRequestNaturalKey{})
}

type shipmentIdentityDouble struct{}

func (shipmentIdentityDouble) MintShipmentSellEvaluationRequestID(context.Context) (domain.ShipmentSellEvaluationRequestID, error) {
	return domain.NewShipmentSellEvaluationRequestID("SHPREQ-SYN-1")
}

type shipmentHandoffDouble struct {
	calls int
}

func (double *shipmentHandoffDouble) HandOffShipmentSellEvaluationRequest(context.Context, ports.ShipmentSellEvaluationRequestRecord) error {
	double.calls++
	return nil
}

func shipmentRoute(t *testing.T, picture ports.ShipmentIntakePicture, adopted bool, aggregation string) (
	*application.RouteSellOnNetworkIntakeHandler, *parcelMomentFixture, *shipmentRegistryDouble,
) {
	t.Helper()
	parcel := newParcelMomentFixture(t, domain.BillingMomentNetworkIntakeAdopted)
	account := value(t, domain.NewSettlementAccountID, "SYN-ACCT-1")
	var adoption domain.ShipmentBillingMomentAdoption
	if adopted {
		var err error
		adoption, err = domain.NewShipmentBillingMomentAdoption(account, domain.ShipmentMembersIntakeAdopted,
			value(t, domain.NewEstimatePricingScope, "SYN-SCOPE-SELL-1"))
		if err != nil {
			t.Fatalf("采用：%v", err)
		}
	}
	registry := &shipmentRegistryDouble{}
	handler, err := application.NewRouteSellOnNetworkIntakeHandler(application.RouteSellOnNetworkIntakeDeps{
		Parcels:           parcel.handler,
		Resolutions:       parcel.resolutions,
		Terms:             parcel.terms,
		Accounts:          parcel.accounts,
		SellPrices:        notDeclaredSellPrices{},
		ParcelAdoptions:   parcel.adoptions,
		ShipmentAdoptions: shipmentAdoptionDouble{adoption: adoption, found: adopted},
		Pictures:          shipmentPictureDouble{picture: picture, found: true},
		Cards: shipmentCardDouble{path: ports.SellPriceCardPath{
			Outcome: ports.SellPriceCardInForce, Aggregation: aggregation,
		}},
		Registry: registry, Identity: shipmentIdentityDouble{}, Downstream: &shipmentHandoffDouble{},
		Clock: fixedClock{at: parcelMomentNow},
	})
	if err != nil {
		t.Fatalf("构造改道：%v", err)
	}
	return handler, parcel, registry
}

func twoMemberPicture(firstReady, secondReady bool) ports.ShipmentIntakePicture {
	start := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	member := func(id string, ready bool) ports.ShipmentMemberIntake {
		item := ports.ShipmentMemberIntake{Parcel: id}
		if ready {
			item.Adopted = true
			item.ResponsibilityStart = start
			item.Source = "network-intake/" + id + "/NODE_INTAKE/v1"
		}
		return item
	}
	return ports.ShipmentIntakePicture{
		Shipment: "SYN-SHIP-1",
		Members:  []ports.ShipmentMemberIntake{member("SYN-PARCEL-1", firstReady), member("SYN-PARCEL-2", secondReady)},
	}
}

func TestAShipmentCardWaitsUntilEveryCurrentMemberIsIntaken(t *testing.T) {
	handler, parcel, registry := shipmentRoute(t, twoMemberPicture(true, false), true, "PER_SHIPMENT")
	result, err := handler.Handle(context.Background(), parcelMomentCommand(t, domain.BillingMomentNetworkIntakeAdopted, "network-intake/SYN-PARCEL-1/NODE_INTAKE/v1", parcelMomentNow))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.SellEvaluationMembersIncomplete {
		t.Fatalf("结果 = %s，想要还没齐", result.Outcome())
	}
	missing := result.MissingMembers()
	if len(missing) != 1 || missing[0].String() != "SYN-PARCEL-2" {
		t.Fatalf("缺件 = %v，想要 SYN-PARCEL-2", missing)
	}
	if len(registry.saved) != 0 || parcel.registry.saves != 0 {
		t.Fatalf("少一件却落了委托请求 %d 份、包裹请求 %d 份", len(registry.saved), parcel.registry.saves)
	}
}

func TestAnAccountWithoutTheShipmentMomentDoesNotTrigger(t *testing.T) {
	handler, parcel, registry := shipmentRoute(t, twoMemberPicture(true, true), false, "PER_SHIPMENT")
	result, err := handler.Handle(context.Background(), parcelMomentCommand(t, domain.BillingMomentNetworkIntakeAdopted, "network-intake/SYN-PARCEL-1/NODE_INTAKE/v1", parcelMomentNow))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.SellEvaluationRequestUnconfigured ||
		result.UnconfiguredReason() != application.SellRequestBillingMomentNotAdopted {
		t.Fatalf("结果 = %s/%s，想要未配置、没采用这一时点", result.Outcome(), result.UnconfiguredReason())
	}
	if len(registry.saved) != 0 || parcel.registry.saves != 0 {
		t.Fatal("没采用却发起了请求")
	}
}

func TestAShipmentCardDoesNotAlsoRequestParcelCharges(t *testing.T) {
	handler, parcel, registry := shipmentRoute(t, twoMemberPicture(true, true), true, "PER_SHIPMENT")
	result, err := handler.Handle(context.Background(), parcelMomentCommand(t, domain.BillingMomentNetworkIntakeAdopted, "network-intake/SYN-PARCEL-1/NODE_INTAKE/v1", parcelMomentNow))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.SellEvaluationRequested || len(registry.saved) != 1 {
		t.Fatalf("结果 = %s 请求 %d，想要一份委托级请求", result.Outcome(), len(registry.saved))
	}
	if parcel.registry.saves != 0 {
		t.Fatalf("同一委托又发了 %d 份包裹级请求", parcel.registry.saves)
	}
	request := registry.saved[0].Request
	if request.Shipment().String() != "SYN-SHIP-1" || request.Moment().String() != "SHIPMENT_MEMBERS_INTAKE_ADOPTED" ||
		!request.PricingBasisAt().Equal(time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)) || len(request.Members()) != 2 {
		t.Fatalf("请求 = 委托 %s 时点 %s 基准 %s 成员 %d", request.Shipment(), request.Moment(), request.PricingBasisAt(), len(request.Members()))
	}
}

func TestAFormedShipmentRequestIsNotRecomputedWhenAMemberIsCancelled(t *testing.T) {
	picture := twoMemberPicture(true, true)
	handler, _, registry := shipmentRoute(t, picture, true, "PER_SHIPMENT")
	command := parcelMomentCommand(t, domain.BillingMomentNetworkIntakeAdopted, "network-intake/SYN-PARCEL-1/NODE_INTAKE/v1", parcelMomentNow)
	if _, err := handler.Handle(context.Background(), command); err != nil {
		t.Fatalf("形成：%v", err)
	}
	picture.Members[1].Cancelled = true
	handler, _, again := shipmentRoute(t, picture, true, "PER_SHIPMENT")
	again.saved = registry.saved
	result, err := handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("取消后再问：%v", err)
	}
	if result.Outcome() != application.SellEvaluationRequestExisting || len(again.saved) != 1 {
		t.Fatalf("结果 = %s 请求 %d，取消一件不该另起一份", result.Outcome(), len(again.saved))
	}
}

func TestRouteOutcomeNamesCoverTheNewAnswers(t *testing.T) {
	for _, outcome := range []application.SellEvaluationRequestOutcome{
		application.SellEvaluationMembersIncomplete, application.SellEvaluationPriceCardConflict,
	} {
		if outcome.String() == "" {
			t.Fatalf("结果 %d 没有词", outcome)
		}
	}
}
