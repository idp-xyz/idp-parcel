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
	at   *time.Time
}

func (double shipmentCardDouble) ResolveSellPriceCardPath(
	_ context.Context, _ domain.TenantID, _ domain.EstimatePricingScope, at time.Time, _ string,
) (ports.SellPriceCardPath, error) {
	if double.at != nil {
		*double.at = at
	}
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
	return shipmentRouteAt(t, picture, adopted, aggregation, nil)
}

func shipmentRouteAt(t *testing.T, picture ports.ShipmentIntakePicture, adopted bool, aggregation string, asked *time.Time) (
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
		}, at: asked},
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

func TestAnUnregisteredAccountOnAShipmentStillRecordsTheParcelStall(t *testing.T) {
	handler, parcel, registry := shipmentRoute(t, twoMemberPicture(true, true), true, "PER_SHIPMENT")
	parcel.accounts.accounts = map[string]domain.SettlementAccountID{}
	command := parcelMomentCommand(t, domain.BillingMomentNetworkIntakeAdopted, "network-intake/SYN-PARCEL-1/NODE_INTAKE/v1", parcelMomentNow)

	result, err := handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.SellEvaluationRequestUnconfigured ||
		result.UnconfiguredReason() != application.SellRequestSettlementAccountNotRegistered {
		t.Fatalf("结果 = %s/%s，想要未配置、账户没登记", result.Outcome(), result.UnconfiguredReason())
	}
	if cause, stalled := result.StopCause(); !stalled || cause != domain.StopSettlementAccountNotRegistered {
		t.Fatalf("停因 = %s %v，想要账户没登记", cause, stalled)
	}
	stall, found := parcel.stalls.sell[sellStallKey(command.TenantID, domain.SellEvaluationRequestNaturalKey{
		Parcel: command.Parcel, Moment: command.Moment,
	})]
	if !found || stall.Cause() != domain.StopSettlementAccountNotRegistered || stall.Source() != command.Source ||
		!stall.PricingBasisAt().Equal(command.PricingBasisAt) {
		t.Fatalf("发起停办 found=%v cause=%s，想要原来那本册记下这一格", found, stall.Cause())
	}
	if len(registry.saved) != 0 || parcel.registry.saves != 0 {
		t.Fatalf("账户没登记却落了委托请求 %d 份、包裹请求 %d 份", len(registry.saved), parcel.registry.saves)
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

func laterMemberCommand(t *testing.T) application.RequestSellEvaluationOnParcelMomentCommand {
	t.Helper()
	command := parcelMomentCommand(t, domain.BillingMomentNetworkIntakeAdopted, "network-intake/SYN-PARCEL-2/NODE_INTAKE/v1", parcelMomentNow)
	command.Parcel = value(t, domain.NewParcelReference, "SYN-PARCEL-2")
	return command
}

func recognizeLaterMember(t *testing.T, parcel *parcelMomentFixture) {
	t.Helper()
	resolution, found := parcel.resolutions.resolutions["SYN-PARCEL-1"]
	if !found {
		t.Fatal("夹具没有第一件的解析")
	}
	parcel.resolutions.resolutions["SYN-PARCEL-2"] = resolution
}

func TestAParcelCardBillsALaterShipmentMemberWhileTheFirstIsNotIntaken(t *testing.T) {
	var asked time.Time
	handler, parcel, registry := shipmentRouteAt(t, twoMemberPicture(false, true), true, "PER_PACKAGE", &asked)
	recognizeLaterMember(t, parcel)
	result, err := handler.Handle(context.Background(), laterMemberCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	record, ok := result.Request()
	if result.Outcome() != application.SellEvaluationRequested || !ok || record.Request.Parcel().String() != "SYN-PARCEL-2" ||
		!record.Request.PricingBasisAt().Equal(time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("结果 = %s 包裹 = %s 基准 = %s，想要第二件按自己的责任起点形成包裹级请求",
			result.Outcome(), record.Request.Parcel(), record.Request.PricingBasisAt())
	}
	if parcel.registry.saves != 1 || len(registry.saved) != 0 {
		t.Fatalf("包裹请求 %d 份、委托请求 %d 份，想要只落第二件的包裹级请求", parcel.registry.saves, len(registry.saved))
	}
	if !asked.Equal(time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("看价卡的时刻 = %s，想要这一拍这一件在画面上的责任起点", asked)
	}
}

func TestAShipmentCardNamesTheFirstMemberWhileItIsNotIntaken(t *testing.T) {
	var asked time.Time
	handler, parcel, registry := shipmentRouteAt(t, twoMemberPicture(false, true), true, "PER_SHIPMENT", &asked)
	recognizeLaterMember(t, parcel)
	result, err := handler.Handle(context.Background(), laterMemberCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.SellEvaluationMembersIncomplete {
		t.Fatalf("结果 = %s，想要还没齐", result.Outcome())
	}
	missing := result.MissingMembers()
	if len(missing) != 1 || missing[0].String() != "SYN-PARCEL-1" {
		t.Fatalf("缺件 = %v，想要 SYN-PARCEL-1", missing)
	}
	if len(registry.saved) != 0 || parcel.registry.saves != 0 {
		t.Fatalf("第一件未到却落了委托请求 %d 份、包裹请求 %d 份", len(registry.saved), parcel.registry.saves)
	}
	if !asked.Equal(time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("看价卡的时刻 = %s，想要这一拍这一件在画面上的责任起点", asked)
	}
}

func memberAt(id string, at time.Time) ports.ShipmentMemberIntake {
	return ports.ShipmentMemberIntake{
		Parcel: id, Adopted: true, ResponsibilityStart: at,
		Source: "network-intake/" + id + "/NODE_INTAKE/v1",
	}
}

func TestTheShipmentBasisIsTheEarliestResponsibilityStart(t *testing.T) {
	t0 := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	later := t0.Add(2 * time.Hour)
	picture := ports.ShipmentIntakePicture{
		Shipment: "SYN-SHIP-1",
		Members:  []ports.ShipmentMemberIntake{memberAt("SYN-PARCEL-1", later), memberAt("SYN-PARCEL-2", t0)},
	}
	var asked time.Time
	handler, parcel, registry := shipmentRouteAt(t, picture, true, "PER_SHIPMENT", &asked)
	recognizeLaterMember(t, parcel)
	result, err := handler.Handle(context.Background(), parcelMomentCommand(t, domain.BillingMomentNetworkIntakeAdopted, "network-intake/SYN-PARCEL-1/NODE_INTAKE/v1", later))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.SellEvaluationRequested || len(registry.saved) != 1 {
		t.Fatalf("结果 = %s 请求 %d", result.Outcome(), len(registry.saved))
	}
	if got := registry.saved[0].Request.PricingBasisAt(); !got.Equal(t0) || !asked.Equal(t0) {
		t.Fatalf("基准 %s 选卡 %s，第二件先收寄时都应是 %s", got, asked, t0)
	}

	asked = time.Time{}
	forward := ports.ShipmentIntakePicture{
		Shipment: "SYN-SHIP-1",
		Members:  []ports.ShipmentMemberIntake{memberAt("SYN-PARCEL-1", t0), memberAt("SYN-PARCEL-2", later)},
	}
	handler, parcel, registry = shipmentRouteAt(t, forward, true, "PER_SHIPMENT", &asked)
	recognizeLaterMember(t, parcel)
	command := laterMemberCommand(t)
	command.PricingBasisAt = later
	result, err = handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("第一件先收寄：%v", err)
	}
	if result.Outcome() != application.SellEvaluationRequested || len(registry.saved) != 1 {
		t.Fatalf("结果 = %s 请求 %d", result.Outcome(), len(registry.saved))
	}
	if got := registry.saved[0].Request.PricingBasisAt(); !got.Equal(t0) || !asked.Equal(t0) {
		t.Fatalf("基准 %s 选卡 %s，凑齐时仍应是第一件的 %s，不能改用第二件", got, asked, t0)
	}
}

func TestAMissingPieceOnAnUnadoptedShipmentMomentStaysUnconfigured(t *testing.T) {
	handler, parcel, registry := shipmentRoute(t, twoMemberPicture(true, false), false, "PER_SHIPMENT")
	result, err := handler.Handle(context.Background(), parcelMomentCommand(t, domain.BillingMomentNetworkIntakeAdopted, "network-intake/SYN-PARCEL-1/NODE_INTAKE/v1", parcelMomentNow))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.SellEvaluationRequestUnconfigured ||
		result.UnconfiguredReason() != application.SellRequestBillingMomentNotAdopted {
		t.Fatalf("结果 = %s/%s，少一件且没采用时应答未配置，不能答等齐", result.Outcome(), result.UnconfiguredReason())
	}
	if len(registry.saved) != 0 || parcel.registry.saves != 0 {
		t.Fatal("没采用却发起了请求")
	}
}

type listingCards struct {
	single ports.SellPriceCardPath
	list   ports.SellPriceCardPath
	listed int
}

func (cards *listingCards) ResolveSellPriceCardPath(
	context.Context, domain.TenantID, domain.EstimatePricingScope, time.Time, string,
) (ports.SellPriceCardPath, error) {
	return cards.single, nil
}

func (cards *listingCards) ResolveSellPriceCardList(
	context.Context, domain.TenantID, domain.ShipmentReference, []domain.ParcelReference, domain.CurrencyCode, []domain.EstimatePricingScope, time.Time, string,
) (ports.SellPriceCardPath, error) {
	cards.listed++
	return cards.list, nil
}

type orderedSellPrices struct {
	scopes []domain.EstimatePricingScope
}

func (prices orderedSellPrices) LoadAdoptedSellPriceArrangement(
	context.Context, domain.TenantID, domain.CommercialResolutionReference,
) (domain.AdoptedSellPriceArrangement, error) {
	return domain.NewOrderedSellPriceSources(prices.scopes)
}

func TestAMultiScopeListProbeChoosesTheShipmentCard(t *testing.T) {
	parcel := newParcelMomentFixture(t, domain.BillingMomentNetworkIntakeAdopted)
	account := value(t, domain.NewSettlementAccountID, "SYN-ACCT-1")
	adoption, err := domain.NewShipmentBillingMomentAdoption(account, domain.ShipmentMembersIntakeAdopted,
		value(t, domain.NewEstimatePricingScope, "SYN-SCOPE-A"))
	if err != nil {
		t.Fatalf("采用：%v", err)
	}
	cards := &listingCards{
		single: ports.SellPriceCardPath{Outcome: ports.SellPriceCardInForce, Aggregation: "PER_PACKAGE"},
		list:   ports.SellPriceCardPath{Outcome: ports.SellPriceCardInForce, Aggregation: "PER_SHIPMENT"},
	}
	registry := &shipmentRegistryDouble{}
	handler, err := application.NewRouteSellOnNetworkIntakeHandler(application.RouteSellOnNetworkIntakeDeps{
		Parcels: parcel.handler, Resolutions: parcel.resolutions, Terms: parcel.terms, Accounts: parcel.accounts,
		SellPrices: orderedSellPrices{scopes: []domain.EstimatePricingScope{
			value(t, domain.NewEstimatePricingScope, "SYN-SCOPE-A"),
			value(t, domain.NewEstimatePricingScope, "SYN-SCOPE-B"),
		}},
		ParcelAdoptions: parcel.adoptions, ShipmentAdoptions: shipmentAdoptionDouble{adoption: adoption, found: true},
		Pictures: shipmentPictureDouble{picture: twoMemberPicture(true, true), found: true},
		Cards:    cards, Registry: registry, Identity: shipmentIdentityDouble{}, Downstream: &shipmentHandoffDouble{},
		Clock: fixedClock{at: parcelMomentNow},
	})
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	result, err := handler.Handle(context.Background(), parcelMomentCommand(t, domain.BillingMomentNetworkIntakeAdopted, "network-intake/SYN-PARCEL-1/NODE_INTAKE/v1", parcelMomentNow))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if cards.listed != 1 || result.Outcome() != application.SellEvaluationRequested || len(registry.saved) != 1 || parcel.registry.saves != 0 {
		t.Fatalf("探路 %d 次，结果 %s，委托请求 %d，包裹请求 %d", cards.listed, result.Outcome(), len(registry.saved), parcel.registry.saves)
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
