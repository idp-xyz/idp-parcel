package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// WarehouseChargeOutcome 是仓储这一路发起、形成与确认的结果。
type WarehouseChargeOutcome string

const (
	WarehouseChargeFormed         WarehouseChargeOutcome = "FORMED"
	WarehouseChargeConfirmed      WarehouseChargeOutcome = "CONFIRMED"
	WarehouseChargeExisting       WarehouseChargeOutcome = "EXISTING_RESULT"
	WarehouseChargeNotConfigured  WarehouseChargeOutcome = "NOT_CONFIGURED"
	WarehouseChargePending        WarehouseChargeOutcome = "PENDING"
	WarehouseChargeZero           WarehouseChargeOutcome = "ZERO"
	WarehouseChargeDifference     WarehouseChargeOutcome = "DIFFERENCE"
	WarehouseChargeAwaitingPerson WarehouseChargeOutcome = "AWAITING_PERSON"
	WarehouseChargeSuperseded     WarehouseChargeOutcome = "SUPERSEDED"
	WarehouseChargeNotAccepted    WarehouseChargeOutcome = "NOT_ACCEPTED"
	WarehouseChargeConflict       WarehouseChargeOutcome = "CONTENT_CONFLICT"
)

// WarehouseOccurrenceFact 是结算回读到的一版发生项。计价按它算，不在这里带金额。
type WarehouseOccurrenceFact struct {
	ID         string
	Version    int
	Kind       string
	Owner      string
	Warehouse  string
	Occurred   time.Time
	Correction string
	Measures   []WarehouseFactMeasure
	Attributes []WarehouseFactAttribute
	Lines      []WarehouseFactLine
}

// WarehouseFactMeasure 把计价要读的计量带过去。Class 区分重量与体积；声明值只在单位相同（计数类可以不带单位）时采用，所以单位必须跟着走。
type WarehouseFactMeasure struct {
	Unit         string
	Quantity     string
	Unmeasured   bool
	Declared     string
	DeclaredUnit string
	Class        string
}

type WarehouseFactAttribute struct {
	Name  string
	Value string
}

type WarehouseFactLine struct {
	Kind       string
	Measures   []WarehouseFactMeasure
	Attributes []WarehouseFactAttribute
}

// WarehouseChargePrice 是计价交回的结果。金额只在评价里，不在发生项上。
type WarehouseChargePrice struct {
	EvaluationID string
	Status       string
	TotalMinor   int64
	Currency     string
	Codes        []string
}

// WarehouseChargePricing 按采用行上的计价范围评价一版发生项。
type WarehouseChargePricing interface {
	Price(ctx context.Context, tenant, scope string, at time.Time, fact WarehouseOccurrenceFact) (WarehouseChargePrice, error)
}

// WarehouseAdoption 是一位货主在一个仓的仓储计费采用。
type WarehouseAdoption struct {
	Owner      string
	Warehouse  string
	Revision   int
	Effective  time.Time
	Account    string
	Scope      string
	Basis      string
	Registrant string
}

// WarehouseChargeRequest 是这一路的 SELL 请求。自然键是发生项与版本。有采用行时带铸出来的请求标识、结算账户、结算币种、计价范围与计价基准时点。金额不在请求上。
type WarehouseChargeRequest struct {
	Occurrence   string
	Version      int
	Status       string
	ChargeID     string
	EvaluationID string
	RequestID    string
	Account      string
	Currency     string
	Scope        string
	BasisAt      time.Time
}

// WarehouseChargeStore 是采用册与这一路请求册。
type WarehouseChargeStore interface {
	SaveAdoption(ctx context.Context, tenant domain.TenantID, adoption WarehouseAdoption) error
	LoadAdoptions(ctx context.Context, tenant domain.TenantID, owner, warehouse string) ([]WarehouseAdoption, error)
	SaveRequest(ctx context.Context, tenant domain.TenantID, request WarehouseChargeRequest) error
	LoadRequest(ctx context.Context, tenant domain.TenantID, occurrence string, version int) (WarehouseChargeRequest, bool, error)
	LoadCharges(ctx context.Context, tenant domain.TenantID, occurrence string) ([]WarehouseChargeRequest, error)
}

// WarehouseChargeHandler 凭采用行发起仓储 SELL，形成预估客户费用，并在确认时点已登记时确认。
type WarehouseChargeHandler struct {
	adoptions WarehouseChargeStore
	pricing   WarehouseChargePricing
	accounts  ports.SettlementAccountByIDView
	mappings  ports.FeeItemMappingView
	charges   interface {
		SaveEstimated(context.Context, domain.TenantID, domain.CustomerCharge) (ports.ChargeSaveOutcome, error)
		SaveConfirmed(context.Context, domain.TenantID, domain.CustomerCharge) (ports.ChargeSaveOutcome, error)
		FindByID(context.Context, domain.TenantID, domain.CustomerChargeID) (domain.CustomerCharge, bool, error)
	}
	bases      ports.ConfirmationBasisRecorder
	conditions ports.ConfirmationConditionView
	moments    *AdmitSettlementMomentHandler
	supersede  ports.ChargeSupersessionStore
	identities interface{ Next() (string, error) }
	clock      ports.Clock
	// reweighAdoptions 与 reweigh 一起才结已确认费用的差额。缺一截答未配置，不写调整。
	reweighAdoptions ports.ReweighAdjustmentAdoptionView
	reweigh          interface {
		SettleReweigh(context.Context, ReweighDifferenceCommand) (ReweighDifferenceOutcome, error)
	}
	sellIDs interface{ Next() (string, error) }
}

func NewWarehouseChargeHandler(
	adoptions WarehouseChargeStore,
	pricing WarehouseChargePricing,
	accounts ports.SettlementAccountByIDView,
	mappings ports.FeeItemMappingView,
	charges interface {
		SaveEstimated(context.Context, domain.TenantID, domain.CustomerCharge) (ports.ChargeSaveOutcome, error)
		SaveConfirmed(context.Context, domain.TenantID, domain.CustomerCharge) (ports.ChargeSaveOutcome, error)
		FindByID(context.Context, domain.TenantID, domain.CustomerChargeID) (domain.CustomerCharge, bool, error)
	},
	bases ports.ConfirmationBasisRecorder,
	conditions ports.ConfirmationConditionView,
	moments *AdmitSettlementMomentHandler,
	supersede ports.ChargeSupersessionStore,
	identities interface{ Next() (string, error) },
	clock ports.Clock,
) *WarehouseChargeHandler {
	return &WarehouseChargeHandler{
		adoptions: adoptions, pricing: pricing, accounts: accounts, mappings: mappings, charges: charges,
		bases: bases, conditions: conditions, moments: moments, supersede: supersede, identities: identities, clock: clock,
	}
}

// WithConfirmedDifference 接上已确认费用的差额。采用册与调整用例都要有；只装一半仍答未配置。
func (handler *WarehouseChargeHandler) WithConfirmedDifference(
	adoptions ports.ReweighAdjustmentAdoptionView,
	reweigh interface {
		SettleReweigh(context.Context, ReweighDifferenceCommand) (ReweighDifferenceOutcome, error)
	},
) *WarehouseChargeHandler {
	handler.reweighAdoptions = adoptions
	handler.reweigh = reweigh
	return handler
}

// WithSellRequests 给有采用行的那一版铸 SELL 请求标识。没接时用发生项与版本拼一个稳定标识，重投仍是同一份。
func (handler *WarehouseChargeHandler) WithSellRequests(identities interface{ Next() (string, error) }) *WarehouseChargeHandler {
	handler.sellIDs = identities
	return handler
}

func (handler *WarehouseChargeHandler) Adopt(ctx context.Context, tenant domain.TenantID, adoption WarehouseAdoption) (WarehouseChargeOutcome, error) {
	if adoption.Owner == "" || adoption.Warehouse == "" || adoption.Revision < 1 || adoption.Effective.IsZero() ||
		adoption.Account == "" || adoption.Scope == "" || adoption.Basis == "" || adoption.Registrant == "" {
		return WarehouseChargeNotAccepted, nil
	}
	account, err := domain.NewSettlementAccountID(adoption.Account)
	if err != nil {
		return WarehouseChargeNotAccepted, nil
	}
	if _, found, err := handler.accounts.FindByID(ctx, tenant, account); err != nil {
		return "", fmt.Errorf("adopt warehouse charge: %w", err)
	} else if !found {
		return WarehouseChargeNotConfigured, nil
	}
	rows, err := handler.adoptions.LoadAdoptions(ctx, tenant, adoption.Owner, adoption.Warehouse)
	if err != nil {
		return "", fmt.Errorf("adopt warehouse charge: %w", err)
	}
	for _, row := range rows {
		if row.Revision == adoption.Revision {
			if sameAdoption(row, adoption) {
				return WarehouseChargeExisting, nil
			}
			return WarehouseChargeConflict, nil
		}
	}
	if !adoptionFits(rows, adoption.Effective) {
		return WarehouseChargeNotAccepted, nil
	}
	if err := handler.adoptions.SaveAdoption(ctx, tenant, adoption); err != nil {
		return "", fmt.Errorf("adopt warehouse charge: %w", err)
	}
	return WarehouseChargeFormed, nil
}

// Settle 处理一版已形成的发生项。没有适用的采用行答未配置。已确认费用上的更正按差额结清；这一户没采用实测差异自动调整答未配置。贷记会让费用为负时答待人处理，不重投。
func (handler *WarehouseChargeHandler) Settle(ctx context.Context, tenant domain.TenantID, fact WarehouseOccurrenceFact) (WarehouseChargeOutcome, string, error) {
	if fact.ID == "" || fact.Version < 1 || fact.Occurred.IsZero() {
		return WarehouseChargeNotAccepted, "", nil
	}
	existing, found, err := handler.adoptions.LoadRequest(ctx, tenant, fact.ID, fact.Version)
	if err != nil {
		return "", "", fmt.Errorf("settle warehouse charge: %w", err)
	}
	if found {
		return WarehouseChargeExisting, existing.ChargeID, nil
	}
	head, err := handler.isHead(ctx, tenant, fact)
	if err != nil {
		return "", "", err
	}
	if !head {
		if err := handler.adoptions.SaveRequest(ctx, tenant, WarehouseChargeRequest{Occurrence: fact.ID, Version: fact.Version, Status: "SUPERSEDED"}); err != nil {
			return "", "", err
		}
		return WarehouseChargeSuperseded, "", nil
	}
	prior, priorCharge, err := handler.priorCharge(ctx, tenant, fact.ID, fact.Version)
	if err != nil {
		return "", "", err
	}
	if fact.Correction == "VOID" {
		return handler.voidPrior(ctx, tenant, fact, prior, priorCharge)
	}
	adoption, found, err := handler.adoptionAt(ctx, tenant, fact)
	if err != nil || !found {
		if err != nil {
			return "", "", err
		}
		if err := handler.adoptions.SaveRequest(ctx, tenant, WarehouseChargeRequest{Occurrence: fact.ID, Version: fact.Version, Status: "NOT_CONFIGURED"}); err != nil {
			return "", "", err
		}
		return WarehouseChargeNotConfigured, "", nil
	}
	price, err := handler.pricing.Price(ctx, tenant.String(), adoption.Scope, fact.Occurred, fact)
	if err != nil {
		return "", "", fmt.Errorf("settle warehouse charge: %w", err)
	}
	switch price.Status {
	case "NOT_CONFIGURED":
		if err := handler.note(ctx, tenant, fact, &adoption, &price, WarehouseChargeRequest{Status: "NOT_CONFIGURED", EvaluationID: price.EvaluationID}); err != nil {
			return "", "", err
		}
		return WarehouseChargeNotConfigured, "", nil
	case "PENDING", "CONFLICT":
		if err := handler.note(ctx, tenant, fact, &adoption, &price, WarehouseChargeRequest{Status: "PENDING", EvaluationID: price.EvaluationID}); err != nil {
			return "", "", err
		}
		return WarehouseChargePending, "", nil
	case "ZERO":
		if priorCharge.ID().String() != "" && priorCharge.Stage() == domain.ChargeConfirmed {
			return handler.settleConfirmed(ctx, tenant, fact, prior, &adoption, price)
		}
		if err := handler.note(ctx, tenant, fact, &adoption, &price, WarehouseChargeRequest{Status: "ZERO", EvaluationID: price.EvaluationID}); err != nil {
			return "", "", err
		}
		return WarehouseChargeZero, "", nil
	}
	if priorCharge.ID().String() != "" && priorCharge.Stage() == domain.ChargeConfirmed {
		return handler.settleConfirmed(ctx, tenant, fact, prior, &adoption, price)
	}
	charge, err := handler.form(ctx, tenant, fact, adoption, price)
	if errors.Is(err, errPendingMapping) {
		if err := handler.note(ctx, tenant, fact, &adoption, &price, WarehouseChargeRequest{Status: "PENDING", EvaluationID: price.EvaluationID}); err != nil {
			return "", "", err
		}
		return WarehouseChargePending, "", nil
	}
	if err != nil {
		return "", "", err
	}
	if priorCharge.ID().String() != "" && priorCharge.Stage() == domain.ChargeEstimated {
		if err := handler.supersede.SaveSupersession(ctx, tenant, priorCharge.ID(), charge.ID(), handler.clock.Now()); err != nil {
			return "", "", fmt.Errorf("settle warehouse charge: %w", err)
		}
	}
	confirmed, err := handler.confirm(ctx, tenant, fact, adoption, charge)
	if err != nil {
		return "", "", err
	}
	status := "FORMED"
	outcome := WarehouseChargeFormed
	if confirmed {
		status = "CONFIRMED"
		outcome = WarehouseChargeConfirmed
	}
	if err := handler.note(ctx, tenant, fact, &adoption, &price, WarehouseChargeRequest{
		Status: status, ChargeID: charge.ID().String(), EvaluationID: price.EvaluationID,
	}); err != nil {
		return "", "", err
	}
	return outcome, charge.ID().String(), nil
}

// 读不动不是链头。当成链头会让这一版接着形成费用；这里上抛，调用方回滚后重投。
func (handler *WarehouseChargeHandler) isHead(ctx context.Context, tenant domain.TenantID, fact WarehouseOccurrenceFact) (bool, error) {
	rows, err := handler.adoptions.LoadCharges(ctx, tenant, fact.ID)
	if err != nil {
		return false, fmt.Errorf("settle warehouse charge: %w", err)
	}
	for _, row := range rows {
		if row.Version > fact.Version {
			return false, nil
		}
	}
	return true, nil
}

func (handler *WarehouseChargeHandler) priorCharge(ctx context.Context, tenant domain.TenantID, occurrence string, version int) (WarehouseChargeRequest, domain.CustomerCharge, error) {
	rows, err := handler.adoptions.LoadCharges(ctx, tenant, occurrence)
	if err != nil {
		return WarehouseChargeRequest{}, domain.CustomerCharge{}, fmt.Errorf("settle warehouse charge: %w", err)
	}
	var prior WarehouseChargeRequest
	for _, row := range rows {
		if row.Version < version && row.ChargeID != "" && row.Version >= prior.Version {
			prior = row
		}
	}
	if prior.ChargeID == "" {
		return prior, domain.CustomerCharge{}, nil
	}
	id, err := domain.NewCustomerChargeID(prior.ChargeID)
	if err != nil {
		return prior, domain.CustomerCharge{}, fmt.Errorf("settle warehouse charge: %w", err)
	}
	charge, found, err := handler.charges.FindByID(ctx, tenant, id)
	if err != nil {
		return prior, domain.CustomerCharge{}, fmt.Errorf("settle warehouse charge: %w", err)
	}
	// 请求册点了费用标识，费用册却没有这一行：两册矛盾，上抛。不是「此前没有费用」。
	if !found {
		return prior, domain.CustomerCharge{}, fmt.Errorf("settle warehouse charge: charge %s named by %s/%d is not on the register", prior.ChargeID, occurrence, prior.Version)
	}
	return prior, charge, nil
}

func (handler *WarehouseChargeHandler) voidPrior(ctx context.Context, tenant domain.TenantID, fact WarehouseOccurrenceFact, prior WarehouseChargeRequest, charge domain.CustomerCharge) (WarehouseChargeOutcome, string, error) {
	if charge.ID().String() == "" {
		if err := handler.adoptions.SaveRequest(ctx, tenant, WarehouseChargeRequest{Occurrence: fact.ID, Version: fact.Version, Status: "ZERO"}); err != nil {
			return "", "", err
		}
		return WarehouseChargeZero, "", nil
	}
	if charge.Stage() == domain.ChargeConfirmed {
		currency, _ := charge.SettlementAmount()
		adoption, found, err := handler.adoptionAt(ctx, tenant, fact)
		if err != nil {
			return "", "", err
		}
		var adopted *WarehouseAdoption
		if found {
			adopted = &adoption
		} else if facts, ok := charge.ConfirmedFacts(); ok {
			adopted = &WarehouseAdoption{Account: facts.SettlementAccount.String(), Scope: facts.PrimaryChargingScope.String()}
		}
		return handler.settleConfirmed(ctx, tenant, fact, prior, adopted, WarehouseChargePrice{
			EvaluationID: fact.ID + "/" + fmt.Sprint(fact.Version),
			Status:       "ZERO",
			TotalMinor:   0,
			Currency:     currency.String(),
		})
	}
	if err := handler.supersede.SaveSupersessionWithoutSuccessor(ctx, tenant, charge.ID(), handler.clock.Now()); err != nil {
		return "", "", err
	}
	_ = prior
	if err := handler.adoptions.SaveRequest(ctx, tenant, WarehouseChargeRequest{Occurrence: fact.ID, Version: fact.Version, Status: "ZERO"}); err != nil {
		return "", "", err
	}
	return WarehouseChargeZero, "", nil
}

// settleConfirmed 对已确认费用记差额，不另形成一笔费用，也不改原确认。没采用实测差异自动调整答未配置。
// 贷记会使费用为负时记下待人处理：那是确定性的，重投也不会变。
func (handler *WarehouseChargeHandler) settleConfirmed(
	ctx context.Context,
	tenant domain.TenantID,
	fact WarehouseOccurrenceFact,
	prior WarehouseChargeRequest,
	adoption *WarehouseAdoption,
	price WarehouseChargePrice,
) (WarehouseChargeOutcome, string, error) {
	chargeID, err := domain.NewCustomerChargeID(prior.ChargeID)
	if err != nil {
		return "", "", fmt.Errorf("settle warehouse charge: %w", err)
	}
	charge, found, err := handler.charges.FindByID(ctx, tenant, chargeID)
	if err != nil {
		return "", "", fmt.Errorf("settle warehouse charge: %w", err)
	}
	if !found {
		return "", "", fmt.Errorf("settle warehouse charge: charge %s named by %s/%d is not on the register", prior.ChargeID, fact.ID, prior.Version)
	}
	facts, confirmed := charge.ConfirmedFacts()
	if !confirmed || handler.reweighAdoptions == nil || handler.reweigh == nil {
		if err := handler.note(ctx, tenant, fact, adoption, &price, WarehouseChargeRequest{Status: "NOT_CONFIGURED", EvaluationID: price.EvaluationID}); err != nil {
			return "", "", err
		}
		return WarehouseChargeNotConfigured, "", nil
	}
	adopted, err := handler.reweighAdoptions.ReweighAdjustmentAdopted(ctx, tenant, facts.SettlementAccount)
	if err != nil {
		return "", "", fmt.Errorf("settle warehouse charge: %w", err)
	}
	if !adopted {
		if err := handler.note(ctx, tenant, fact, adoption, &price, WarehouseChargeRequest{Status: "NOT_CONFIGURED", EvaluationID: price.EvaluationID}); err != nil {
			return "", "", err
		}
		return WarehouseChargeNotConfigured, "", nil
	}
	outcome, err := handler.reweigh.SettleReweigh(ctx, ReweighDifferenceCommand{
		TenantID: tenant, ChargeID: prior.ChargeID, Evaluation: price.EvaluationID,
		OriginalCurrency: price.Currency, OriginalMinor: price.TotalMinor,
		SettlementCurrency: price.Currency, SettlementMinor: price.TotalMinor,
	})
	if err != nil {
		return "", "", fmt.Errorf("settle warehouse charge: %w", err)
	}
	switch outcome {
	case ReweighDifferenceNone, ReweighDifferenceRecorded, ReweighDifferenceReplayed:
		if err := handler.note(ctx, tenant, fact, adoption, &price, WarehouseChargeRequest{
			Status: "DIFFERENCE", ChargeID: prior.ChargeID, EvaluationID: price.EvaluationID,
		}); err != nil {
			return "", "", err
		}
		return WarehouseChargeDifference, prior.ChargeID, nil
	case ReweighDifferenceExceedsCharge:
		if err := handler.note(ctx, tenant, fact, adoption, &price, WarehouseChargeRequest{
			Status: "AWAITING_PERSON", ChargeID: prior.ChargeID, EvaluationID: price.EvaluationID,
		}); err != nil {
			return "", "", err
		}
		return WarehouseChargeAwaitingPerson, prior.ChargeID, nil
	case ReweighDifferenceUndecided:
		return "", "", fmt.Errorf("settle warehouse charge: reweigh undecided")
	default:
		return "", "", fmt.Errorf("settle warehouse charge: reweigh %s", outcome)
	}
}

// 有采用行才是一次 SELL 请求：账户、币种、范围和基准时点记在这一版上，评价标识指向刚刚算出的评价。
// 不另投一份去计价。没接铸造口时用发生项与版本，重投仍是同一份；铸造失败不拿替代号顶上。
func (handler *WarehouseChargeHandler) note(
	ctx context.Context,
	tenant domain.TenantID,
	fact WarehouseOccurrenceFact,
	adoption *WarehouseAdoption,
	price *WarehouseChargePrice,
	request WarehouseChargeRequest,
) error {
	request.Occurrence = fact.ID
	request.Version = fact.Version
	if price != nil && request.EvaluationID == "" {
		request.EvaluationID = price.EvaluationID
	}
	if adoption == nil {
		return handler.adoptions.SaveRequest(ctx, tenant, request)
	}
	request.Account = adoption.Account
	request.Scope = adoption.Scope
	request.BasisAt = fact.Occurred
	if request.RequestID == "" {
		if handler.sellIDs != nil {
			minted, err := handler.sellIDs.Next()
			if err != nil {
				return err
			}
			request.RequestID = minted
		} else {
			request.RequestID = fact.ID + "/" + fmt.Sprint(fact.Version)
		}
	}
	if request.Currency == "" && handler.accounts != nil && adoption.Account != "" {
		accountID, err := domain.NewSettlementAccountID(adoption.Account)
		if err != nil {
			return err
		}
		account, found, err := handler.accounts.FindByID(ctx, tenant, accountID)
		if err != nil {
			return fmt.Errorf("settle warehouse charge: %w", err)
		}
		if found {
			request.Currency = account.Key().Currency().String()
		}
	}
	return handler.adoptions.SaveRequest(ctx, tenant, request)
}

func (handler *WarehouseChargeHandler) adoptionAt(ctx context.Context, tenant domain.TenantID, fact WarehouseOccurrenceFact) (WarehouseAdoption, bool, error) {
	rows, err := handler.adoptions.LoadAdoptions(ctx, tenant, fact.Owner, fact.Warehouse)
	if err != nil {
		return WarehouseAdoption{}, false, err
	}
	var chosen WarehouseAdoption
	found := false
	for _, row := range rows {
		if row.Effective.After(fact.Occurred) {
			continue
		}
		if !found || row.Effective.After(chosen.Effective) || (row.Effective.Equal(chosen.Effective) && row.Revision > chosen.Revision) {
			chosen = row
			found = true
		}
	}
	return chosen, found, nil
}

func (handler *WarehouseChargeHandler) form(ctx context.Context, tenant domain.TenantID, fact WarehouseOccurrenceFact, adoption WarehouseAdoption, price WarehouseChargePrice) (domain.CustomerCharge, error) {
	accountID, err := domain.NewSettlementAccountID(adoption.Account)
	if err != nil {
		return domain.CustomerCharge{}, err
	}
	account, found, err := handler.accounts.FindByID(ctx, tenant, accountID)
	if err != nil {
		return domain.CustomerCharge{}, fmt.Errorf("settle warehouse charge: %w", err)
	}
	// 采用行点名的账户不在册上是两册矛盾。读失败同样上抛，不换成未配置。
	if !found {
		return domain.CustomerCharge{}, fmt.Errorf("settle warehouse charge: account %s named by the adoption is not on the register", adoption.Account)
	}
	if account.Key().Currency().String() != price.Currency {
		return domain.CustomerCharge{}, fmt.Errorf("settle warehouse charge: currency")
	}
	var feeItem domain.FeeItemReference
	seen := ""
	for _, code := range price.Codes {
		chargeCode, err := domain.NewPricingChargeCode(code)
		if err != nil {
			return domain.CustomerCharge{}, err
		}
		mapping, found, err := handler.mappings.FindFeeItemMapping(ctx, tenant, chargeCode)
		if err != nil {
			return domain.CustomerCharge{}, err
		}
		if !found {
			return domain.CustomerCharge{}, errPendingMapping
		}
		if seen == "" {
			seen = mapping.FeeItem().String()
			feeItem = mapping.FeeItem()
		} else if seen != mapping.FeeItem().String() {
			return domain.CustomerCharge{}, errPendingMapping
		}
	}
	rawID, err := handler.identities.Next()
	if err != nil {
		return domain.CustomerCharge{}, err
	}
	chargeID, err := domain.NewCustomerChargeID(rawID)
	if err != nil {
		return domain.CustomerCharge{}, err
	}
	evaluation, err := domain.NewSellEvaluationReference(price.EvaluationID)
	if err != nil {
		return domain.CustomerCharge{}, err
	}
	currency, err := domain.NewCurrencyCode(price.Currency)
	if err != nil {
		return domain.CustomerCharge{}, err
	}
	charge, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID: chargeID, FeeItem: feeItem, Evaluation: evaluation,
		OriginalCurrency: currency, OriginalMinor: price.TotalMinor,
		SettlementCurrency: currency, SettlementMinor: price.TotalMinor,
		Stage: domain.ChargeEstimated, FormedAt: handler.clock.Now(), SourceOccurredAt: fact.Occurred,
	})
	if err != nil {
		return domain.CustomerCharge{}, err
	}
	if _, err := handler.charges.SaveEstimated(ctx, tenant, charge); err != nil {
		return domain.CustomerCharge{}, err
	}
	return charge, nil
}

var errPendingMapping = fmt.Errorf("warehouse charge mapping pending")

func (handler *WarehouseChargeHandler) confirm(ctx context.Context, tenant domain.TenantID, fact WarehouseOccurrenceFact, adoption WarehouseAdoption, charge domain.CustomerCharge) (bool, error) {
	basisKind := domain.WarehouseWorkOccurredBasis()
	reference, err := domain.NewConfirmationBasisReference(fact.ID + "/" + fmt.Sprint(fact.Version))
	if err != nil {
		return false, err
	}
	if _, err := handler.bases.RecordConfirmationBasis(ctx, tenant, charge.ID(), charge.FeeItem(), basisKind, reference, handler.clock.Now()); err != nil {
		return false, err
	}
	moment, err := handler.moments.Admit(ctx, tenant, domain.SettlementMomentConfirm)
	if err != nil {
		return false, err
	}
	// 没登记确认时点是答案：费用形成、当场不确认。读册失败在 Admit 里不带 error，收成另一种结果；那一格必须上抛。
	if moment == SettlementMomentUnconfigured {
		return false, nil
	}
	if moment != SettlementMomentAdmitted {
		return false, fmt.Errorf("settle warehouse charge: confirmation moment %s", moment)
	}
	condition, found, err := handler.conditions.LoadConfirmationCondition(ctx, tenant, charge.ID(), charge.FeeItem())
	if err != nil {
		return false, err
	}
	if !found || !condition.Met {
		return false, nil
	}
	accountID, err := domain.NewSettlementAccountID(adoption.Account)
	if err != nil {
		return false, err
	}
	account, accountFound, err := handler.accounts.FindByID(ctx, tenant, accountID)
	if err != nil {
		return false, fmt.Errorf("settle warehouse charge: %w", err)
	}
	if !accountFound {
		return false, fmt.Errorf("settle warehouse charge: account %s named by the adoption is not on the register", adoption.Account)
	}
	contract, err := domain.NewContractBasisReference(account.Responsibility().String())
	if err != nil {
		return false, err
	}
	scope, err := domain.NewChargingScopeReference(adoption.Scope)
	if err != nil {
		return false, err
	}
	source, err := domain.NewSourceFactReference(fact.ID)
	if err != nil {
		return false, err
	}
	confirmed, err := charge.Confirm(domain.ConfirmedChargeFacts{
		ResponsibleEntity: account.Key().LegalEntity(), Counterparty: account.Key().Counterparty(),
		Direction: account.Key().Direction(), SettlementAccount: account.ID(),
		ContractBasis: contract, PrimaryChargingScope: scope, SourceFact: source,
	}, condition.Basis, handler.clock.Now())
	if err != nil {
		return false, err
	}
	if _, err := handler.charges.SaveConfirmed(ctx, tenant, confirmed); err != nil {
		return false, err
	}
	return true, nil
}

func sameAdoption(left, right WarehouseAdoption) bool {
	return left.Effective.Equal(right.Effective) && left.Account == right.Account && left.Scope == right.Scope && left.Basis == right.Basis
}

func adoptionFits(rows []WarehouseAdoption, effective time.Time) bool {
	if len(rows) == 0 {
		return true
	}
	earliest, latest := rows[0].Effective, rows[0].Effective
	for _, row := range rows[1:] {
		if row.Effective.Before(earliest) {
			earliest = row.Effective
		}
		if row.Effective.After(latest) {
			latest = row.Effective
		}
	}
	return !effective.Before(latest) || effective.Before(earliest)
}
