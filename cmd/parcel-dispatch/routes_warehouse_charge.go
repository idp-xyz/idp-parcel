package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"

	pricingstore "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	pricingdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	settlementapp "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	settlementdomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	whpostgres "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	whdomain "go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	whports "go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func init() {
	registerWarehouseFulfillmentRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		consumer, err := newWarehouseChargeConsumer(sources)
		if err != nil {
			return nil, err
		}
		return []routeLeg{{
			event:    eventing.EventType("warehouse-fulfillment.charge-occurrence.formed"),
			position: positionPrimary, consumer: consumer, untranslated: true, label: "warehouse charge",
		}}, nil
	})
}

type warehouseChargeConsumer struct {
	occurrences *whpostgres.Charges
	settlement  *settlementapp.WarehouseChargeHandler
	transactor  bentoapp.Transactor
}

func newWarehouseChargeConsumer(sources dispatchSources) (*warehouseChargeConsumer, error) {
	occurrences, err := whpostgres.NewCharges(sources.db)
	if err != nil {
		return nil, err
	}
	plans, err := pricingstore.NewWarehousePlans(sources.db)
	if err != nil {
		return nil, err
	}
	evaluations, err := platformidentity.NewMinter("WEV")
	if err != nil {
		return nil, err
	}
	charges, err := platformidentity.NewMinter("CHG")
	if err != nil {
		return nil, err
	}
	store, err := sapostgres.NewWarehouseCharges(sources.db)
	if err != nil {
		return nil, err
	}
	accounts, err := sapostgres.NewSettlementAccounts(sources.db)
	if err != nil {
		return nil, err
	}
	mappings, err := sapostgres.NewFeeItemMappings(sources.db)
	if err != nil {
		return nil, err
	}
	customerCharges, err := sapostgres.NewCustomerCharges(sources.db)
	if err != nil {
		return nil, err
	}
	conditions, err := sapostgres.NewChargeConfirmationConditions(sources.db)
	if err != nil {
		return nil, err
	}
	moments, err := sapostgres.NewSettlementMoments(sources.db)
	if err != nil {
		return nil, err
	}
	momentHandler, err := settlementapp.NewAdmitSettlementMomentHandler(moments)
	if err != nil {
		return nil, err
	}
	supersessions, err := sapostgres.NewChargeSupersessions(sources.db)
	if err != nil {
		return nil, err
	}
	pricing := dispatchPricing{handler: pricingapp.NewWarehousePlanHandler(plans, true, evaluations)}
	reweighAdoptions, err := sapostgres.NewReweighAdjustmentAdoptions(sources.db)
	if err != nil {
		return nil, err
	}
	adjustments, err := sapostgres.NewChargeAdjustments(sources.db)
	if err != nil {
		return nil, err
	}
	postings, err := sapostgres.NewCustomerReceiptPostings(sources.db)
	if err != nil {
		return nil, err
	}
	reweigh := settlementapp.NewRecordChargeAdjustmentHandler(settlementapp.RecordChargeAdjustmentDeps{
		Charges: customerCharges, Adjustments: adjustments, Clock: sources.clock, Balances: postings, Books: postings,
	})
	handler := settlementapp.NewWarehouseChargeHandler(
		store, pricing, accounts, mappings, customerCharges, conditions, conditions,
		momentHandler, supersessions, charges, sources.clock,
	).WithConfirmedDifference(reweighAdoptions, reweigh)
	return &warehouseChargeConsumer{occurrences: occurrences, settlement: handler, transactor: sources.db.Transactor()}, nil
}

func (consumer *warehouseChargeConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	var body struct {
		TenantID     string `json:"tenantId"`
		OccurrenceID string `json:"occurrenceId"`
		Version      int    `json:"version"`
	}
	if err := json.Unmarshal(envelope.Payload, &body); err != nil || body.TenantID == "" || body.OccurrenceID == "" || body.Version < 1 {
		return fmt.Errorf("parcel-dispatch: warehouse charge occurrence payload")
	}
	return consumer.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		tenant, err := whdomain.NewTenantID(body.TenantID)
		if err != nil {
			return err
		}
		version, found, err := consumer.occurrences.LoadVersion(txCtx, tenant, body.OccurrenceID, body.Version)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("parcel-dispatch: warehouse charge occurrence %s/%d is not on the register", body.OccurrenceID, body.Version)
		}
		settlementTenant, err := settlementdomain.NewTenantID(body.TenantID)
		if err != nil {
			return err
		}
		_, _, err = consumer.settlement.Settle(txCtx, settlementTenant, warehouseFact(version))
		return err
	})
}

func warehouseFact(version whports.ChargeVersion) settlementapp.WarehouseOccurrenceFact {
	fact := settlementapp.WarehouseOccurrenceFact{
		ID: version.ID, Version: version.Version, Kind: version.Kind, Owner: version.Owner, Warehouse: version.Warehouse,
		Occurred: version.Occurred, Correction: version.Correction,
	}
	for _, measure := range version.Measures {
		fact.Measures = append(fact.Measures, factMeasure(measure))
	}
	for _, attribute := range version.Attributes {
		fact.Attributes = append(fact.Attributes, settlementapp.WarehouseFactAttribute{Name: attribute.Name, Value: attribute.Value})
	}
	for _, line := range version.Lines {
		mapped := settlementapp.WarehouseFactLine{Kind: line.Kind}
		for _, measure := range line.Measures {
			mapped.Measures = append(mapped.Measures, factMeasure(measure))
		}
		for _, attribute := range line.Attributes {
			mapped.Attributes = append(mapped.Attributes, settlementapp.WarehouseFactAttribute{Name: attribute.Name, Value: attribute.Value})
		}
		fact.Lines = append(fact.Lines, mapped)
	}
	return fact
}

type dispatchPricing struct {
	handler *pricingapp.WarehousePlanHandler
}

func (pricing dispatchPricing) Price(ctx context.Context, tenant, scope string, at time.Time, fact settlementapp.WarehouseOccurrenceFact) (settlementapp.WarehouseChargePrice, error) {
	outcome, evaluation, id, err := pricing.handler.Price(ctx, tenant, scope, at, pricingdomain.WarehouseOccurrenceSnapshot{
		ID: fact.ID, Version: fact.Version, Kind: fact.Kind, Owner: fact.Owner, Warehouse: fact.Warehouse,
		Occurred: fact.Occurred, Correction: fact.Correction,
		Measures: pricingMeasures(fact.Measures), Attributes: pricingAttributes(fact.Attributes), Lines: pricingLines(fact.Lines),
	})
	if err != nil {
		return settlementapp.WarehouseChargePrice{}, err
	}
	status := "PENDING"
	switch outcome {
	case pricingapp.WarehousePlanNotConfigured:
		status = "NOT_CONFIGURED"
	case pricingapp.WarehousePlanPending:
		status = "PENDING"
	case pricingapp.WarehousePlanConflict:
		status = "CONFLICT"
	case pricingapp.WarehousePlanZero:
		status = "ZERO"
	case pricingapp.WarehousePlanPriced:
		status = "COMPLETED"
	}
	return settlementapp.WarehouseChargePrice{
		EvaluationID: id, Status: status, TotalMinor: evaluation.TotalMinor, Currency: evaluation.Currency, Codes: evaluation.Codes,
	}, nil
}

// 原值与原单位是给人看的证据，不到计价。计价按类找重量与体积，并只在声明单位对得上时采用声明值。
func factMeasure(measure whports.ChargeMeasure) settlementapp.WarehouseFactMeasure {
	return settlementapp.WarehouseFactMeasure{
		Unit: measure.Unit, Quantity: measure.Quantity, Unmeasured: measure.Unmeasured,
		Declared: measure.Declared, DeclaredUnit: measure.DeclaredUnit, Class: measure.Class,
	}
}

func pricingMeasures(measures []settlementapp.WarehouseFactMeasure) []pricingdomain.WarehouseMeasure {
	mapped := make([]pricingdomain.WarehouseMeasure, 0, len(measures))
	for _, measure := range measures {
		mapped = append(mapped, pricingdomain.WarehouseMeasure{
			Unit: measure.Unit, Quantity: measure.Quantity, Unmeasured: measure.Unmeasured,
			Declared: measure.Declared, DeclaredUnit: measure.DeclaredUnit, Class: measure.Class,
		})
	}
	return mapped
}

func pricingAttributes(attributes []settlementapp.WarehouseFactAttribute) []pricingdomain.WarehouseAttribute {
	mapped := make([]pricingdomain.WarehouseAttribute, 0, len(attributes))
	for _, attribute := range attributes {
		mapped = append(mapped, pricingdomain.WarehouseAttribute{Name: attribute.Name, Value: attribute.Value})
	}
	return mapped
}

func pricingLines(lines []settlementapp.WarehouseFactLine) []pricingdomain.WarehouseOccurrenceLine {
	mapped := make([]pricingdomain.WarehouseOccurrenceLine, 0, len(lines))
	for _, line := range lines {
		mapped = append(mapped, pricingdomain.WarehouseOccurrenceLine{
			Kind: line.Kind, Measures: pricingMeasures(line.Measures), Attributes: pricingAttributes(line.Attributes),
		})
	}
	return mapped
}
