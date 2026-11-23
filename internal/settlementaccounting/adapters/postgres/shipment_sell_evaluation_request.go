package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ShipmentSellEvaluationRequests 是委托级销售评价请求册。包裹那一路的请求册不写这张表。
type ShipmentSellEvaluationRequests struct {
	db *bentopg.DB
}

func NewShipmentSellEvaluationRequests(db *bentopg.DB) (*ShipmentSellEvaluationRequests, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &ShipmentSellEvaluationRequests{db: db}, nil
}

var (
	_ ports.ShipmentSellEvaluationRequestRegistry           = (*ShipmentSellEvaluationRequests)(nil)
	_ ports.ShipmentSellEvaluationRequestView               = (*ShipmentSellEvaluationRequests)(nil)
	_ ports.ShipmentSellEvaluationRequestConclusionRecorder = (*ShipmentSellEvaluationRequests)(nil)
)

func (registry *ShipmentSellEvaluationRequests) Save(
	ctx context.Context,
	record ports.ShipmentSellEvaluationRequestRecord,
) (ports.EvaluationRequestSaveOutcome, error) {
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.EvaluationRequestSaveOutcomeInvalid, fmt.Errorf("save shipment sell evaluation request: %w", err)
	}
	request := record.Request
	if record.State != domain.SellRequestSubmitted {
		return ports.EvaluationRequestSaveOutcomeInvalid, fmt.Errorf("save shipment sell evaluation request: only a submitted request is inserted")
	}
	scopes := []string{request.PricingScope().String()}
	if listed, ordered := request.PricingScopes(); ordered {
		scopes = make([]string, len(listed))
		for index, scope := range listed {
			scopes[index] = scope.String()
		}
	}
	members := make([]string, 0, len(request.Members()))
	for _, member := range request.Members() {
		members = append(members, member.String())
	}
	var previous, basis *string
	if earlier, ok := request.Previous(); ok {
		value := earlier.String()
		previous = &value
	}
	if resume := request.ResumeBasis(); resume.Present() {
		value := resume.String()
		basis = &value
	}
	tag, err := executor.Exec(ctx, `
		INSERT INTO settlement_accounting.shipment_sell_evaluation_request (
			tenant_id, request_id, shipment_id, member_ids, account_id, settlement_currency,
			pricing_scope_id, pricing_scope_ids, billing_moment, source_fact_ref, pricing_basis_at,
			recorded_at, state, generation, previous_request_id, resume_basis
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Request.String(),
		request.Shipment().String(),
		members,
		request.Account().String(),
		request.SettlementCurrency().String(),
		request.PricingScope().String(),
		scopes,
		request.Moment().String(),
		request.Source().String(),
		request.PricingBasisAt().UTC(),
		record.RecordedAt.UTC(),
		domain.SellRequestSubmitted.String(),
		request.Generation(),
		previous,
		basis,
	)
	if err != nil {
		return ports.EvaluationRequestSaveOutcomeInvalid, fmt.Errorf("save shipment sell evaluation request: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.EvaluationRequestAlreadyRequested, nil
	}
	return ports.EvaluationRequestSaved, nil
}

func (registry *ShipmentSellEvaluationRequests) RecordConclusion(
	ctx context.Context,
	key ports.ShipmentSellEvaluationRequestKey,
	conclusion domain.SellEvaluationRequestConclusion,
) (ports.SellEvaluationRequestConclusionEffect, error) {
	none := ports.SellEvaluationRequestConclusionEffectInvalid
	if strings.TrimSpace(key.TenantID.String()) == "" || strings.TrimSpace(key.Request.String()) == "" {
		return none, fmt.Errorf("record shipment sell evaluation request conclusion: the tenant or the request ID is blank")
	}
	if conclusion.State() == domain.SellEvaluationRequestStateInvalid {
		return none, fmt.Errorf("record shipment sell evaluation request conclusion: the conclusion is the zero value")
	}
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return none, fmt.Errorf("record shipment sell evaluation request conclusion: %w", err)
	}
	var (
		charge, cause *string
		stoppedAt     *time.Time
	)
	if formed, ok := conclusion.FormedCharge(); ok {
		value := formed.String()
		charge = &value
	}
	if stopped, ok := conclusion.StopCause(); ok {
		value := stopped.String()
		at := conclusion.StoppedAt().UTC()
		cause, stoppedAt = &value, &at
	}
	tag, err := executor.Exec(ctx, `
		UPDATE settlement_accounting.shipment_sell_evaluation_request
		   SET state = $3, customer_charge_id = $4, stop_cause = $6, stopped_at = $7
		 WHERE tenant_id = $1 AND request_id = $2 AND state = $5`,
		key.TenantID.String(),
		key.Request.String(),
		conclusion.State().String(),
		charge,
		domain.SellRequestSubmitted.String(),
		cause,
		stoppedAt,
	)
	if err != nil {
		return none, fmt.Errorf("record shipment sell evaluation request conclusion: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.SellEvaluationRequestConcluded, nil
	}
	stored, found, err := registry.FindByID(ctx, key)
	if err != nil {
		return none, fmt.Errorf("record shipment sell evaluation request conclusion: %w", err)
	}
	if !found {
		return none, fmt.Errorf("record shipment sell evaluation request conclusion: request %s is not in the register", key.Request)
	}
	if conclusion.Same(stored.State, stored.FormedCharge, stored.StopCause) {
		return ports.SellEvaluationRequestConclusionReplayed, nil
	}
	return ports.SellEvaluationRequestConcludedOtherwise, nil
}

func (registry *ShipmentSellEvaluationRequests) FindByID(
	ctx context.Context,
	key ports.ShipmentSellEvaluationRequestKey,
) (ports.ShipmentSellEvaluationRequestRecord, bool, error) {
	return registry.findOne(ctx, `
		WHERE tenant_id = $1 AND request_id = $2`,
		key.TenantID.String(), key.Request.String())
}

func (registry *ShipmentSellEvaluationRequests) FindByNaturalKey(
	ctx context.Context,
	tenant domain.TenantID,
	key domain.ShipmentSellEvaluationRequestNaturalKey,
) (ports.ShipmentSellEvaluationRequestRecord, bool, error) {
	return registry.findOne(ctx, `
		WHERE tenant_id = $1 AND shipment_id = $2 AND billing_moment = $3
		ORDER BY generation DESC LIMIT 1`,
		tenant.String(), key.Shipment.String(), key.Moment.String())
}

func (registry *ShipmentSellEvaluationRequests) findOne(
	ctx context.Context,
	where string,
	args ...any,
) (ports.ShipmentSellEvaluationRequestRecord, bool, error) {
	none := ports.ShipmentSellEvaluationRequestRecord{}
	querier, err := registry.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("find shipment sell evaluation request: %w", err)
	}
	row := querier.QueryRow(ctx, `
		SELECT tenant_id, request_id, shipment_id, member_ids, account_id, settlement_currency,
		       pricing_scope_id, pricing_scope_ids, billing_moment, source_fact_ref, pricing_basis_at,
		       recorded_at, state, customer_charge_id, generation, previous_request_id, resume_basis,
		       stop_cause
		  FROM settlement_accounting.shipment_sell_evaluation_request
		`+where, args...)
	record, err := scanShipmentSellRequest(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("find shipment sell evaluation request: %w", err)
	}
	return record, true, nil
}

func scanShipmentSellRequest(row pgx.Row) (ports.ShipmentSellEvaluationRequestRecord, error) {
	var (
		tenantID, requestID, shipmentID, accountID, currency, scopeID, moment, source, state string
		members, scopes                                                                      []string
		basisAt, recordedAt                                                                  time.Time
		charge, previous, resume, stop                                                       *string
		generation                                                                           int
	)
	if err := row.Scan(
		&tenantID, &requestID, &shipmentID, &members, &accountID, &currency,
		&scopeID, &scopes, &moment, &source, &basisAt,
		&recordedAt, &state, &charge, &generation, &previous, &resume, &stop,
	); err != nil {
		return ports.ShipmentSellEvaluationRequestRecord{}, err
	}
	tenant, err := domain.NewTenantID(tenantID)
	if err != nil {
		return ports.ShipmentSellEvaluationRequestRecord{}, err
	}
	id, err := domain.NewShipmentSellEvaluationRequestID(requestID)
	if err != nil {
		return ports.ShipmentSellEvaluationRequestRecord{}, err
	}
	shipment, err := domain.NewShipmentReference(shipmentID)
	if err != nil {
		return ports.ShipmentSellEvaluationRequestRecord{}, err
	}
	parcelMembers := make([]domain.ParcelReference, 0, len(members))
	for _, member := range members {
		parsed, err := domain.NewParcelReference(member)
		if err != nil {
			return ports.ShipmentSellEvaluationRequestRecord{}, err
		}
		parcelMembers = append(parcelMembers, parsed)
	}
	account, err := domain.NewSettlementAccountID(accountID)
	if err != nil {
		return ports.ShipmentSellEvaluationRequestRecord{}, err
	}
	settlementCurrency, err := domain.NewCurrencyCode(currency)
	if err != nil {
		return ports.ShipmentSellEvaluationRequestRecord{}, err
	}
	pricingScope, err := domain.NewEstimatePricingScope(scopeID)
	if err != nil {
		return ports.ShipmentSellEvaluationRequestRecord{}, err
	}
	pricingScopes, err := sellPriceScopesFrom(scopes)
	if err != nil {
		return ports.ShipmentSellEvaluationRequestRecord{}, err
	}
	billingMoment, err := domain.ShipmentBillingMomentFromName(moment)
	if err != nil {
		return ports.ShipmentSellEvaluationRequestRecord{}, err
	}
	fact, err := domain.NewSourceFactReference(source)
	if err != nil {
		return ports.ShipmentSellEvaluationRequestRecord{}, err
	}
	spec := domain.ShipmentSellEvaluationRequestSpec{
		ID: id, Shipment: shipment, Members: parcelMembers, Account: account,
		SettlementCurrency: settlementCurrency, PricingScope: pricingScope, PricingScopes: pricingScopes,
		Moment: billingMoment, Source: fact, PricingBasisAt: basisAt, Generation: generation,
	}
	if previous != nil {
		spec.Previous, err = domain.NewShipmentSellEvaluationRequestID(*previous)
		if err != nil {
			return ports.ShipmentSellEvaluationRequestRecord{}, err
		}
	}
	if resume != nil {
		spec.ResumeBasis, err = domain.ResumeBasisFromName(*resume)
		if err != nil {
			return ports.ShipmentSellEvaluationRequestRecord{}, err
		}
	}
	request, err := domain.SubmitShipmentSellEvaluationRequest(spec)
	if err != nil {
		return ports.ShipmentSellEvaluationRequestRecord{}, err
	}
	parsedState, err := domain.SellEvaluationRequestStateFromName(state)
	if err != nil {
		return ports.ShipmentSellEvaluationRequestRecord{}, err
	}
	record := ports.ShipmentSellEvaluationRequestRecord{
		Key:        ports.ShipmentSellEvaluationRequestKey{TenantID: tenant, Request: id},
		Request:    request,
		RecordedAt: recordedAt,
		State:      parsedState,
	}
	if charge != nil {
		record.FormedCharge, err = domain.NewCustomerChargeID(*charge)
		if err != nil {
			return ports.ShipmentSellEvaluationRequestRecord{}, err
		}
	}
	if stop != nil {
		record.StopCause, err = domain.EvaluationStopCauseFromName(*stop)
		if err != nil {
			return ports.ShipmentSellEvaluationRequestRecord{}, err
		}
	}
	return record, nil
}
