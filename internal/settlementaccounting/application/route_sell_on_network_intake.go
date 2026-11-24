package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// RouteSellOnNetworkIntakeDeps 在有效网络收寄这一拍上决定走包裹计费还是委托级计费。
// 包裹那一路的发起编排原样留着；这里只在价卡是逐委托时改道。
type RouteSellOnNetworkIntakeDeps struct {
	Parcels           *RequestSellEvaluationOnParcelMomentHandler
	Resolutions       ports.ParcelCommercialResolutionView
	Terms             ports.AdoptedSettlementTermsView
	Accounts          ports.SettlementAccountView
	SellPrices        ports.AdoptedSellPriceArrangementView
	ParcelAdoptions   ports.SellBillingMomentAdoptionView
	ShipmentAdoptions ports.ShipmentBillingMomentAdoptionView
	Pictures          ports.ShipmentIntakePictureView
	Cards             ports.SellPriceCardPathView
	Registry          ports.ShipmentSellEvaluationRequestRegistry
	Identity          ports.ShipmentSellEvaluationRequestIdentityFactory
	Downstream        ports.ShipmentSellEvaluationRequestHandoff
	Clock             ports.Clock
}

// RouteSellOnNetworkIntakeHandler 实现收寄采用消费方要的那一只 Handle。终局那一拍不走这里。
type RouteSellOnNetworkIntakeHandler struct {
	deps RouteSellOnNetworkIntakeDeps
}

func NewRouteSellOnNetworkIntakeHandler(deps RouteSellOnNetworkIntakeDeps) (*RouteSellOnNetworkIntakeHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"parcel sell requester", deps.Parcels == nil},
		{"parcel commercial resolution view", deps.Resolutions == nil},
		{"adopted settlement terms view", deps.Terms == nil},
		{"settlement account view", deps.Accounts == nil},
		{"adopted sell price arrangement view", deps.SellPrices == nil},
		{"parcel billing moment adoption view", deps.ParcelAdoptions == nil},
		{"shipment billing moment adoption view", deps.ShipmentAdoptions == nil},
		{"shipment intake picture view", deps.Pictures == nil},
		{"sell price card path view", deps.Cards == nil},
		{"shipment sell evaluation request registry", deps.Registry == nil},
		{"shipment sell evaluation request identity factory", deps.Identity == nil},
		{"shipment sell evaluation request handoff", deps.Downstream == nil},
		{"clock", deps.Clock == nil},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("%w: %s", ErrNilDependency, dependency.name)
		}
	}
	return &RouteSellOnNetworkIntakeHandler{deps: deps}, nil
}

func (handler *RouteSellOnNetworkIntakeHandler) Handle(
	ctx context.Context,
	command RequestSellEvaluationOnParcelMomentCommand,
) (RequestSellEvaluationOnParcelMomentResult, error) {
	if command.Moment != domain.BillingMomentNetworkIntakeAdopted {
		return handler.deps.Parcels.Handle(ctx, command)
	}
	picture, found, err := handler.deps.Pictures.LoadByParcel(ctx, command.TenantID, command.Parcel)
	if err != nil {
		return sellRequestUndecided(SellRequestShipmentPictureUnavailable), nil
	}
	if !found || picture.Shipment == "" {
		return handler.deps.Parcels.Handle(ctx, command)
	}
	shipment, err := domain.NewShipmentReference(picture.Shipment)
	if err != nil {
		return RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationRequestNotAccepted}, nil
	}
	if existing, stop, err := handler.existingShipmentRequest(ctx, command.TenantID, shipment); err != nil || stop {
		return existing, err
	}

	valid, missing := classifyShipmentMembers(picture.Members)
	if len(valid) == 0 {
		return RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationMembersIncomplete}, nil
	}
	// 选卡与写进委托级请求的计价基准必须是同一刻，否则跨换卡时两边看到的卡不是一张。
	// 这一刻是当前有效成员里已经收寄的最早责任起点：用触发这一拍的那一件，或声明位次的第一件，都会在两件收寄顺序不同时错开。
	basis, known := earliestAdoptedResponsibilityStart(valid)
	if !known {
		return membersIncomplete(missing), nil
	}

	scopes, account, currency, halted, err := handler.pricingScopes(ctx, command)
	if err != nil {
		return RequestSellEvaluationOnParcelMomentResult{}, err
	}
	if halted != nil {
		// 发起停办只写在包裹那本册上。账户没登记时改道自己答未配置，这一拍没有格子，登记账户之后续办读不到它。
		// 包裹坐在委托上也不改这一句：委托级请求要等账户，停办仍是原来那一格。
		if halted.UnconfiguredReason() == SellRequestSettlementAccountNotRegistered {
			return handler.deps.Parcels.Handle(ctx, command)
		}
		return *halted, nil
	}
	if len(scopes) == 0 {
		return handler.billParcels(ctx, command, valid)
	}
	path, err := handler.cardPath(ctx, command.TenantID, shipment, valid, currency, scopes, basis, picture.ServiceProduct)
	if err != nil {
		return sellRequestUndecided(SellRequestShipmentPriceCardUnavailable), nil
	}
	switch path.Outcome {
	case ports.SellPriceCardConflict:
		return RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationPriceCardConflict}, nil
	case ports.SellPriceCardInForce:
		if path.Aggregation == "PER_SHIPMENT" {
			// 少一件与没采用都留给 billShipment：它先查账户是否采用委托级时点，再答等齐。
			// 在这里因少一件提前返回，会把「没采用」说成「等齐」。
			return handler.billShipment(ctx, command, shipment, valid, missing, account, currency, scopes, basis)
		}
	}
	return handler.billParcels(ctx, command, valid)
}

func (handler *RouteSellOnNetworkIntakeHandler) existingShipmentRequest(
	ctx context.Context,
	tenant domain.TenantID,
	shipment domain.ShipmentReference,
) (RequestSellEvaluationOnParcelMomentResult, bool, error) {
	record, found, err := handler.deps.Registry.FindByNaturalKey(ctx, tenant, domain.ShipmentSellEvaluationRequestNaturalKey{
		Shipment: shipment,
		Moment:   domain.ShipmentMembersIntakeAdopted,
	})
	if err != nil {
		return sellRequestUndecided(SellRequestShipmentRegistryUnavailable), true, nil
	}
	if !found {
		return RequestSellEvaluationOnParcelMomentResult{}, false, nil
	}
	if err := handler.deps.Downstream.HandOffShipmentSellEvaluationRequest(ctx, record); err != nil {
		if errors.Is(err, ports.ErrSellEvaluationRequestHandoffRejected) {
			return RequestSellEvaluationOnParcelMomentResult{}, true, fmt.Errorf("route sell on network intake: %w", err)
		}
		return sellRequestUndecided(SellRequestShipmentHandoffUnavailable), true, nil
	}
	return RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationRequestExisting}, true, nil
}

func (handler *RouteSellOnNetworkIntakeHandler) billShipment(
	ctx context.Context,
	command RequestSellEvaluationOnParcelMomentCommand,
	shipment domain.ShipmentReference,
	valid, missing []ports.ShipmentMemberIntake,
	account domain.SettlementAccountID,
	currency domain.CurrencyCode,
	scopes []domain.EstimatePricingScope,
	basis time.Time,
) (RequestSellEvaluationOnParcelMomentResult, error) {
	adoption, adopted, err := handler.deps.ShipmentAdoptions.FindShipmentBillingMomentAdoption(ctx, command.TenantID, account)
	if err != nil {
		return sellRequestUndecided(SellRequestShipmentAdoptionUnavailable), nil
	}
	if !adopted || adoption.Moment() != domain.ShipmentMembersIntakeAdopted {
		return sellRequestUnconfigured(SellRequestBillingMomentNotAdopted), nil
	}
	if len(missing) > 0 {
		return membersIncomplete(missing), nil
	}
	if len(scopes) == 1 && scopes[0].String() == "" {
		return sellRequestUnconfigured(SellRequestBillingMomentScopeAbsent), nil
	}
	members := make([]domain.ParcelReference, 0, len(valid))
	for _, member := range valid {
		reference, err := domain.NewParcelReference(member.Parcel)
		if err != nil {
			return RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationRequestNotAccepted}, nil
		}
		members = append(members, reference)
	}
	source, err := domain.NewSourceFactReference("shipment-members-intake/" + shipment.String())
	if err != nil {
		return RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationRequestNotAccepted}, nil
	}
	id, err := handler.deps.Identity.MintShipmentSellEvaluationRequestID(ctx)
	if err != nil {
		return sellRequestUndecided(SellRequestShipmentIdentityUnavailable), nil
	}
	request, err := domain.SubmitShipmentSellEvaluationRequest(domain.ShipmentSellEvaluationRequestSpec{
		ID:                 id,
		Shipment:           shipment,
		Members:            members,
		Account:            account,
		SettlementCurrency: currency,
		PricingScope:       scopes[0],
		PricingScopes:      scopes,
		Moment:             domain.ShipmentMembersIntakeAdopted,
		Source:             source,
		PricingBasisAt:     basis,
	})
	if err != nil {
		return RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationRequestNotAccepted}, nil
	}
	record := ports.ShipmentSellEvaluationRequestRecord{
		Key:        ports.ShipmentSellEvaluationRequestKey{TenantID: command.TenantID, Request: id},
		Request:    request,
		RecordedAt: handler.deps.Clock.Now(),
		State:      domain.SellRequestSubmitted,
	}
	saved, err := handler.deps.Registry.Save(ctx, record)
	if err != nil {
		return sellRequestUndecided(SellRequestShipmentRegistryUnavailable), nil
	}
	switch saved {
	case ports.EvaluationRequestSaved:
	case ports.EvaluationRequestAlreadyRequested:
		existing, _, err := handler.existingShipmentRequest(ctx, command.TenantID, shipment)
		return existing, err
	default:
		return RequestSellEvaluationOnParcelMomentResult{}, fmt.Errorf("%w: %d", ErrUnexpectedSellEvaluationRequestSave, saved)
	}
	if err := handler.deps.Downstream.HandOffShipmentSellEvaluationRequest(ctx, record); err != nil {
		if errors.Is(err, ports.ErrSellEvaluationRequestHandoffRejected) {
			return RequestSellEvaluationOnParcelMomentResult{}, fmt.Errorf("route sell on network intake: %w", err)
		}
		return sellRequestUndecided(SellRequestShipmentHandoffUnavailable), nil
	}
	return RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationRequested}, nil
}

func (handler *RouteSellOnNetworkIntakeHandler) billParcels(
	ctx context.Context,
	command RequestSellEvaluationOnParcelMomentCommand,
	valid []ports.ShipmentMemberIntake,
) (RequestSellEvaluationOnParcelMomentResult, error) {
	var trigger RequestSellEvaluationOnParcelMomentResult
	triggered := false
	for _, member := range valid {
		if !member.Adopted || member.ResponsibilityStart.IsZero() || member.Source == "" {
			continue
		}
		parcel, err := domain.NewParcelReference(member.Parcel)
		if err != nil {
			return RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationRequestNotAccepted}, nil
		}
		source, err := domain.NewSourceFactReference(member.Source)
		if err != nil {
			return RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationRequestNotAccepted}, nil
		}
		memberCommand := RequestSellEvaluationOnParcelMomentCommand{
			TenantID:       command.TenantID,
			Parcel:         parcel,
			Moment:         domain.BillingMomentNetworkIntakeAdopted,
			Source:         source,
			PricingBasisAt: member.ResponsibilityStart,
		}
		result, err := handler.deps.Parcels.Handle(ctx, memberCommand)
		if err != nil {
			return RequestSellEvaluationOnParcelMomentResult{}, err
		}
		if result.Outcome() == SellEvaluationRequestUndecided {
			return result, nil
		}
		if member.Parcel == command.Parcel.String() {
			trigger, triggered = result, true
		}
	}
	if triggered {
		return trigger, nil
	}
	return handler.deps.Parcels.Handle(ctx, command)
}

func (handler *RouteSellOnNetworkIntakeHandler) pricingScopes(
	ctx context.Context,
	command RequestSellEvaluationOnParcelMomentCommand,
) ([]domain.EstimatePricingScope, domain.SettlementAccountID, domain.CurrencyCode, *RequestSellEvaluationOnParcelMomentResult, error) {
	none := domain.EstimatePricingScope{}
	resolution, resolved, err := handler.deps.Resolutions.LoadParcelCommercialResolution(ctx, command.TenantID, command.Parcel)
	if err != nil {
		halted := sellRequestUndecided(SellRequestCommercialResolutionUnavailable)
		return nil, domain.SettlementAccountID{}, domain.CurrencyCode{}, &halted, nil
	}
	if !resolved {
		halted := RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationRequestParcelUnresolved}
		return nil, domain.SettlementAccountID{}, domain.CurrencyCode{}, &halted, nil
	}
	arrangement, err := handler.deps.SellPrices.LoadAdoptedSellPriceArrangement(ctx, command.TenantID, resolution)
	if err != nil {
		halted := sellRequestUndecided(SellRequestSellPriceArrangementUnavailable)
		return nil, domain.SettlementAccountID{}, domain.CurrencyCode{}, &halted, nil
	}
	if _, notPriced := arrangement.NoSellPriceBasis(); notPriced {
		halted := RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationNotPriced}
		return nil, domain.SettlementAccountID{}, domain.CurrencyCode{}, &halted, nil
	}
	account, currency, accountHalt, err := handler.accountOf(ctx, command, resolution)
	if err != nil || accountHalt != nil {
		return nil, domain.SettlementAccountID{}, domain.CurrencyCode{}, accountHalt, err
	}
	if sources, ordered := arrangement.Sources(); ordered {
		return sources, account, currency, nil, nil
	}
	if !arrangement.NotDeclared() {
		return nil, domain.SettlementAccountID{}, domain.CurrencyCode{}, nil, fmt.Errorf("route sell on network intake: sell price arrangement is not one of the three answers")
	}
	shipmentAdoption, adopted, err := handler.deps.ShipmentAdoptions.FindShipmentBillingMomentAdoption(ctx, command.TenantID, account)
	if err != nil {
		halted := sellRequestUndecided(SellRequestShipmentAdoptionUnavailable)
		return nil, domain.SettlementAccountID{}, domain.CurrencyCode{}, &halted, nil
	}
	if adopted && shipmentAdoption.PricingScope().String() != "" {
		return []domain.EstimatePricingScope{shipmentAdoption.PricingScope()}, account, currency, nil, nil
	}
	parcelAdoption, parcelAdopted, err := handler.deps.ParcelAdoptions.FindSellBillingMomentAdoption(ctx, command.TenantID, account)
	if err != nil {
		halted := sellRequestUndecided(SellRequestBillingMomentAdoptionUnavailable)
		return nil, domain.SettlementAccountID{}, domain.CurrencyCode{}, &halted, nil
	}
	if parcelAdopted && parcelAdoption.PricingScope().String() != "" {
		return []domain.EstimatePricingScope{parcelAdoption.PricingScope()}, account, currency, nil, nil
	}
	_ = none
	return nil, account, currency, nil, nil
}

func (handler *RouteSellOnNetworkIntakeHandler) accountOf(
	ctx context.Context,
	command RequestSellEvaluationOnParcelMomentCommand,
	resolution domain.CommercialResolutionReference,
) (domain.SettlementAccountID, domain.CurrencyCode, *RequestSellEvaluationOnParcelMomentResult, error) {
	terms, adopted, err := handler.deps.Terms.LoadAdoptedSettlementTerms(ctx, command.TenantID, resolution)
	if err != nil {
		halted := sellRequestUndecided(SellRequestSettlementTermsUnavailable)
		return domain.SettlementAccountID{}, domain.CurrencyCode{}, &halted, nil
	}
	if !adopted {
		halted := sellRequestUnconfigured(SellRequestSettlementPolicyNotAdopted)
		return domain.SettlementAccountID{}, domain.CurrencyCode{}, &halted, nil
	}
	key, err := terms.AccountKey(domain.ChargeReceivable)
	if err != nil {
		halted := RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationRequestNotAccepted}
		return domain.SettlementAccountID{}, domain.CurrencyCode{}, &halted, nil
	}
	account, registered, err := handler.deps.Accounts.Find(ctx, command.TenantID, key)
	if err != nil {
		halted := sellRequestUndecided(SellRequestSettlementAccountUnavailable)
		return domain.SettlementAccountID{}, domain.CurrencyCode{}, &halted, nil
	}
	if !registered {
		halted := sellRequestUnconfigured(SellRequestSettlementAccountNotRegistered)
		return domain.SettlementAccountID{}, domain.CurrencyCode{}, &halted, nil
	}
	return account, key.Currency(), nil, nil
}

func (handler *RouteSellOnNetworkIntakeHandler) cardPath(
	ctx context.Context,
	tenant domain.TenantID,
	shipment domain.ShipmentReference,
	valid []ports.ShipmentMemberIntake,
	currency domain.CurrencyCode,
	scopes []domain.EstimatePricingScope,
	at time.Time,
	serviceProduct string,
) (ports.SellPriceCardPath, error) {
	// 单范围没有下一格可跳，只问这一格在用卡。只传选卡口的测试走的也是这一格。
	if len(scopes) == 1 {
		return handler.deps.Cards.ResolveSellPriceCardPath(ctx, tenant, scopes[0], at, serviceProduct)
	}
	if len(scopes) > 1 {
		if listed, ok := handler.deps.Cards.(ports.SellPriceCardListView); ok {
			members := make([]domain.ParcelReference, 0, len(valid))
			for _, member := range valid {
				reference, err := domain.NewParcelReference(member.Parcel)
				if err != nil {
					return ports.SellPriceCardPath{}, err
				}
				members = append(members, reference)
			}
			return listed.ResolveSellPriceCardList(ctx, tenant, shipment, members, currency, scopes, at, serviceProduct)
		}
	}
	return firstInForceSellPriceCard(ctx, handler.deps.Cards, tenant, scopes, at, serviceProduct)
}

func firstInForceSellPriceCard(
	ctx context.Context,
	cards ports.SellPriceCardPathView,
	tenant domain.TenantID,
	scopes []domain.EstimatePricingScope,
	at time.Time,
	serviceProduct string,
) (ports.SellPriceCardPath, error) {
	var last ports.SellPriceCardPath
	for _, scope := range scopes {
		path, err := cards.ResolveSellPriceCardPath(ctx, tenant, scope, at, serviceProduct)
		if err != nil {
			return ports.SellPriceCardPath{}, err
		}
		switch path.Outcome {
		case ports.SellPriceCardNotConfigured:
			last = path
		case ports.SellPriceCardConflict, ports.SellPriceCardInForce:
			return path, nil
		default:
			return ports.SellPriceCardPath{}, fmt.Errorf("route sell on network intake: price card path %d", path.Outcome)
		}
	}
	if last.Outcome == ports.SellPriceCardPathOutcomeInvalid {
		last.Outcome = ports.SellPriceCardNotConfigured
	}
	return last, nil
}

// earliestAdoptedResponsibilityStart 取当前未取消的有效成员里，已经采用有效网络收寄且责任起点非零的那一份里最早的时刻。
// 并列就是同一刻。一件都还没收寄时第二个返回值为假，调用方答等齐，不拿零时刻去选卡。
func earliestAdoptedResponsibilityStart(members []ports.ShipmentMemberIntake) (time.Time, bool) {
	var earliest time.Time
	found := false
	for _, member := range members {
		if !member.Adopted || member.ResponsibilityStart.IsZero() {
			continue
		}
		if !found || member.ResponsibilityStart.Before(earliest) {
			earliest = member.ResponsibilityStart
			found = true
		}
	}
	return earliest, found
}

func classifyShipmentMembers(members []ports.ShipmentMemberIntake) (valid, missing []ports.ShipmentMemberIntake) {
	for _, member := range members {
		if member.Cancelled {
			continue
		}
		valid = append(valid, member)
		if !member.Adopted || member.ResponsibilityStart.IsZero() {
			missing = append(missing, member)
		}
	}
	return valid, missing
}

func membersIncomplete(missing []ports.ShipmentMemberIntake) RequestSellEvaluationOnParcelMomentResult {
	parcels := make([]domain.ParcelReference, 0, len(missing))
	for _, member := range missing {
		reference, err := domain.NewParcelReference(member.Parcel)
		if err != nil {
			continue
		}
		parcels = append(parcels, reference)
	}
	return RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationMembersIncomplete, missing: parcels}
}
