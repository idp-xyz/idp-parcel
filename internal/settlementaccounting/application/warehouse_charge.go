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

type WarehouseFactMeasure struct {
	Unit       string
	Quantity   string
	Unmeasured bool
	Declared   string
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

// WarehouseChargeRequest 是这一路已经记下的一版请求。
type WarehouseChargeRequest struct {
	Occurrence   string
	Version      int
	Status       string
	ChargeID     string
	EvaluationID string
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

// Settle 处理一版已形成的发生项。没有适用的采用行答未配置。已确认费用上的更正答待人处理。
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
	if !handler.isHead(ctx, tenant, fact) {
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
		if err := handler.adoptions.SaveRequest(ctx, tenant, WarehouseChargeRequest{Occurrence: fact.ID, Version: fact.Version, Status: "NOT_CONFIGURED"}); err != nil {
			return "", "", err
		}
		return WarehouseChargeNotConfigured, "", nil
	case "PENDING", "CONFLICT":
		if err := handler.adoptions.SaveRequest(ctx, tenant, WarehouseChargeRequest{Occurrence: fact.ID, Version: fact.Version, Status: "PENDING"}); err != nil {
			return "", "", err
		}
		return WarehouseChargePending, "", nil
	case "ZERO":
		if priorCharge.ID().String() != "" && priorCharge.Stage() == domain.ChargeConfirmed {
			return handler.awaitPerson(ctx, tenant, fact)
		}
		if err := handler.adoptions.SaveRequest(ctx, tenant, WarehouseChargeRequest{Occurrence: fact.ID, Version: fact.Version, Status: "ZERO", EvaluationID: price.EvaluationID}); err != nil {
			return "", "", err
		}
		return WarehouseChargeZero, "", nil
	}
	if priorCharge.ID().String() != "" && priorCharge.Stage() == domain.ChargeConfirmed {
		return handler.awaitPerson(ctx, tenant, fact)
	}
	charge, err := handler.form(ctx, tenant, fact, adoption, price)
	if errors.Is(err, errPendingMapping) {
		if err := handler.adoptions.SaveRequest(ctx, tenant, WarehouseChargeRequest{Occurrence: fact.ID, Version: fact.Version, Status: "PENDING"}); err != nil {
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
	if err := handler.adoptions.SaveRequest(ctx, tenant, WarehouseChargeRequest{
		Occurrence: fact.ID, Version: fact.Version, Status: status, ChargeID: charge.ID().String(),
	}); err != nil {
		return "", "", err
	}
	return outcome, charge.ID().String(), nil
}

func (handler *WarehouseChargeHandler) isHead(ctx context.Context, tenant domain.TenantID, fact WarehouseOccurrenceFact) bool {
	rows, err := handler.adoptions.LoadCharges(ctx, tenant, fact.ID)
	if err != nil {
		return true
	}
	for _, row := range rows {
		if row.Version > fact.Version {
			return false
		}
	}
	return true
}

func (handler *WarehouseChargeHandler) priorCharge(ctx context.Context, tenant domain.TenantID, occurrence string, version int) (WarehouseChargeRequest, domain.CustomerCharge, error) {
	rows, err := handler.adoptions.LoadCharges(ctx, tenant, occurrence)
	if err != nil {
		return WarehouseChargeRequest{}, domain.CustomerCharge{}, err
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
		return prior, domain.CustomerCharge{}, err
	}
	charge, found, err := handler.charges.FindByID(ctx, tenant, id)
	if err != nil || !found {
		return prior, domain.CustomerCharge{}, err
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
		return handler.awaitPerson(ctx, tenant, fact)
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

func (handler *WarehouseChargeHandler) awaitPerson(ctx context.Context, tenant domain.TenantID, fact WarehouseOccurrenceFact) (WarehouseChargeOutcome, string, error) {
	if err := handler.adoptions.SaveRequest(ctx, tenant, WarehouseChargeRequest{Occurrence: fact.ID, Version: fact.Version, Status: "AWAITING_PERSON"}); err != nil {
		return "", "", err
	}
	return WarehouseChargeAwaitingPerson, "", nil
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
	if err != nil || !found {
		return domain.CustomerCharge{}, fmt.Errorf("settle warehouse charge: account")
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
	if err != nil || moment != SettlementMomentAdmitted {
		return false, err
	}
	condition, found, err := handler.conditions.LoadConfirmationCondition(ctx, tenant, charge.ID(), charge.FeeItem())
	if err != nil || !found || !condition.Met {
		return false, err
	}
	accountID, err := domain.NewSettlementAccountID(adoption.Account)
	if err != nil {
		return false, err
	}
	account, _, err := handler.accounts.FindByID(ctx, tenant, accountID)
	if err != nil {
		return false, err
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
