package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ParcelDirectFacts 从本上下文的费用与成本册，以及服务形态、发生项成员、集运与订舱，读一件包裹的直接成本选料。
// 结算应用不 import party-commercial 的领域包；形态只取形态册上的词。
type ParcelDirectFacts struct {
	db *bentopg.DB
}

func NewParcelDirectFacts(db *bentopg.DB) (*ParcelDirectFacts, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &ParcelDirectFacts{db: db}, nil
}

var _ ports.ParcelDirectFactsView = (*ParcelDirectFacts)(nil)

func (reader *ParcelDirectFacts) LoadParcelDirectFacts(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelReference,
) (ports.ParcelDirectFacts, error) {
	querier, err := reader.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ParcelDirectFacts{}, fmt.Errorf("load parcel direct facts: %w", err)
	}
	var facts ports.ParcelDirectFacts
	if err := querier.QueryRow(ctx, serviceFormSQL, tenant.String(), parcel.String()).Scan(&facts.ServiceForm); err != nil {
		return ports.ParcelDirectFacts{}, fmt.Errorf("load parcel direct facts: service form: %w", err)
	}
	if err := querier.QueryRow(ctx, sharedCostSQL, tenant.String(), parcel.String()).Scan(&facts.SharedCost); err != nil {
		return ports.ParcelDirectFacts{}, fmt.Errorf("load parcel direct facts: shared cost: %w", err)
	}
	if err := querier.QueryRow(ctx, nonUniqueSQL, tenant.String(), parcel.String()).Scan(&facts.NonUniqueSupplier); err != nil {
		return ports.ParcelDirectFacts{}, fmt.Errorf("load parcel direct facts: occurrence members: %w", err)
	}
	facts.Estimates, err = directAmounts(ctx, querier, customerAmountSQL, tenant.String(), parcel.String(), false)
	if err != nil {
		return ports.ParcelDirectFacts{}, fmt.Errorf("load parcel direct facts: estimates: %w", err)
	}
	facts.Receivables, err = directAmounts(ctx, querier, customerAmountSQL, tenant.String(), parcel.String(), true)
	if err != nil {
		return ports.ParcelDirectFacts{}, fmt.Errorf("load parcel direct facts: receivables: %w", err)
	}
	facts.ExpectedCosts, err = directAmounts(ctx, querier, expectedCostSQL, tenant.String(), parcel.String(), false)
	if err != nil {
		return ports.ParcelDirectFacts{}, fmt.Errorf("load parcel direct facts: expected costs: %w", err)
	}
	facts.Payables, err = directAmounts(ctx, querier, payableSQL, tenant.String(), parcel.String(), false)
	if err != nil {
		return ports.ParcelDirectFacts{}, fmt.Errorf("load parcel direct facts: payables: %w", err)
	}
	facts.Credits, err = directAmounts(ctx, querier, creditSQL, tenant.String(), parcel.String(), false)
	if err != nil {
		return ports.ParcelDirectFacts{}, fmt.Errorf("load parcel direct facts: credits: %w", err)
	}
	return facts, nil
}

func (reader *ParcelDirectFacts) ParcelsOfCharge(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerChargeID,
) ([]domain.ParcelReference, error) {
	return reader.parcels(ctx,
		`SELECT DISTINCT parcel_id
		   FROM settlement_accounting.sell_evaluation_request
		  WHERE tenant_id = $1 AND customer_charge_id = $2`,
		tenant.String(), charge.String())
}

func (reader *ParcelDirectFacts) ParcelsOfOccurrence(
	ctx context.Context,
	tenant domain.TenantID,
	occurrence domain.ChargeOccurrenceID,
	version domain.OccurrenceVersion,
) ([]domain.ParcelReference, error) {
	return reader.parcels(ctx, occurrenceParcelsSQL, tenant.String(), occurrence.String(), version.String())
}

func (reader *ParcelDirectFacts) ParcelsOfPayable(
	ctx context.Context,
	tenant domain.TenantID,
	payable domain.PayableID,
) ([]domain.ParcelReference, error) {
	querier, err := reader.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("parcels of payable: %w", err)
	}
	var occurrence, version string
	err = querier.QueryRow(ctx,
		`SELECT cost.occurrence_id, cost.occurrence_version
		   FROM settlement_accounting.audited_payable AS payable
		   JOIN settlement_accounting.supplier_expected_cost AS cost
		     ON cost.tenant_id = payable.tenant_id
		    AND cost.version = payable.expected_version
		  WHERE payable.tenant_id = $1 AND payable.payable_id = $2`,
		tenant.String(), payable.String(),
	).Scan(&occurrence, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("parcels of payable: %w", err)
	}
	occurrenceID, err := domain.NewChargeOccurrenceID(occurrence)
	if err != nil {
		return nil, fmt.Errorf("parcels of payable: %w", err)
	}
	occurrenceVersion, err := domain.NewOccurrenceVersion(version)
	if err != nil {
		return nil, fmt.Errorf("parcels of payable: %w", err)
	}
	return reader.ParcelsOfOccurrence(ctx, tenant, occurrenceID, occurrenceVersion)
}

func (reader *ParcelDirectFacts) parcels(ctx context.Context, query string, args ...any) ([]domain.ParcelReference, error) {
	querier, err := reader.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("parcel refs: %w", err)
	}
	rows, err := querier.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("parcel refs: %w", err)
	}
	defer rows.Close()
	var parcels []domain.ParcelReference
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("parcel refs: %w", err)
		}
		parcel, err := domain.NewParcelReference(raw)
		if err != nil {
			return nil, fmt.Errorf("parcel refs: %w", err)
		}
		parcels = append(parcels, parcel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("parcel refs: %w", err)
	}
	return parcels, nil
}

type directAmountQuery interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func directAmounts(ctx context.Context, querier directAmountQuery, query, tenant, parcel string, confirmed bool) ([]ports.DirectAmount, error) {
	rows, err := querier.Query(ctx, query, tenant, parcel, confirmed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var amounts []ports.DirectAmount
	for rows.Next() {
		var amount ports.DirectAmount
		if err := rows.Scan(&amount.Source, &amount.Currency, &amount.AmountMinor, &amount.Period, &amount.PeriodStated); err != nil {
			return nil, err
		}
		amounts = append(amounts, amount)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return amounts, nil
}

// 客户费用自己的账期是已确认费用的归属日。预估行上还没有归属日，账期就空着，选料答待判断，
// 不用形成时刻顶上。confirmed 为真时只取已确认行。
const customerAmountSQL = `
SELECT charge.charge_id,
       charge.settlement_currency,
       charge.settlement_minor,
       COALESCE(to_char(charge.attribution_date, 'YYYY-MM-DD'), ''),
       charge.attribution_date IS NOT NULL
  FROM settlement_accounting.sell_evaluation_request AS request
  JOIN settlement_accounting.customer_charge AS charge
    ON charge.tenant_id = request.tenant_id
   AND charge.charge_id = request.customer_charge_id
 WHERE request.tenant_id = $1
   AND request.parcel_id = $2
   AND request.state = 'FORMED'
   AND NOT EXISTS (
        SELECT 1
          FROM settlement_accounting.customer_charge_supersession AS superseded
         WHERE superseded.tenant_id = charge.tenant_id
           AND superseded.charge_id = charge.charge_id
   )
   AND ($3::boolean IS FALSE OR charge.stage = 'CONFIRMED')`

// 链头且发生项恰有这一件成员。预期成本没有账期列，PeriodStated 恒假。
const expectedCostSQL = `
SELECT cost.version,
       cost.settlement_currency,
       cost.settlement_minor,
       '',
       FALSE
  FROM settlement_accounting.supplier_expected_cost AS cost
 WHERE cost.tenant_id = $1
   AND NOT cost.withdrawn
   AND NOT EXISTS (
        SELECT 1
          FROM settlement_accounting.supplier_expected_cost AS later
         WHERE later.tenant_id = cost.tenant_id
           AND later.prior_version = cost.version
   )
   AND (
        SELECT count(*)
          FROM transport_fulfillment.transport_charge_occurrence_member AS counted
         WHERE counted.tenant_id = cost.tenant_id
           AND counted.occurrence_ref = cost.occurrence_id
           AND counted.validity_version = cost.occurrence_version
   ) = 1
   AND EXISTS (
        SELECT 1
          FROM transport_fulfillment.transport_charge_occurrence_member AS member
         WHERE member.tenant_id = cost.tenant_id
           AND member.occurrence_ref = cost.occurrence_id
           AND member.validity_version = cost.occurrence_version
           AND member.object_ref = $2
   )
   AND $3::boolean IS FALSE`

// 应付的账期是主张的账期。同一主张各版本账期不一致时 Period 空而 PeriodStated 为真，选料答待判断。
const payableSQL = `
SELECT payable.payable_id,
       payable.currency,
       payable.amount_minor,
       CASE WHEN periods.period_count = 1 THEN periods.period_ref ELSE '' END,
       TRUE
  FROM settlement_accounting.audited_payable AS payable
  JOIN settlement_accounting.supplier_expected_cost AS cost
    ON cost.tenant_id = payable.tenant_id
   AND cost.version = payable.expected_version
  JOIN LATERAL (
        SELECT count(DISTINCT reception.period_ref)::int AS period_count,
               min(reception.period_ref) AS period_ref
          FROM settlement_accounting.supplier_bill_reception AS reception
         WHERE reception.tenant_id = payable.tenant_id
           AND reception.claim_id = payable.claim_id
  ) AS periods ON TRUE
 WHERE payable.tenant_id = $1
   AND (
        SELECT count(*)
          FROM transport_fulfillment.transport_charge_occurrence_member AS counted
         WHERE counted.tenant_id = cost.tenant_id
           AND counted.occurrence_ref = cost.occurrence_id
           AND counted.validity_version = cost.occurrence_version
   ) = 1
   AND EXISTS (
        SELECT 1
          FROM transport_fulfillment.transport_charge_occurrence_member AS member
         WHERE member.tenant_id = cost.tenant_id
           AND member.occurrence_ref = cost.occurrence_id
           AND member.validity_version = cost.occurrence_version
           AND member.object_ref = $2
   )
   AND $3::boolean IS FALSE`

// 贷项的账期随它所回指的应付，不另立一套。
const creditSQL = `
SELECT note.note_id || '@' || note.note_version,
       note.currency,
       note.amount_minor,
       CASE WHEN periods.period_count = 1 THEN periods.period_ref ELSE '' END,
       TRUE
  FROM settlement_accounting.supplier_credit_note AS note
  JOIN settlement_accounting.audited_payable AS payable
    ON payable.tenant_id = note.tenant_id
   AND payable.payable_id = note.payable_id
  JOIN settlement_accounting.supplier_expected_cost AS cost
    ON cost.tenant_id = payable.tenant_id
   AND cost.version = payable.expected_version
  JOIN LATERAL (
        SELECT count(DISTINCT reception.period_ref)::int AS period_count,
               min(reception.period_ref) AS period_ref
          FROM settlement_accounting.supplier_bill_reception AS reception
         WHERE reception.tenant_id = payable.tenant_id
           AND reception.claim_id = payable.claim_id
  ) AS periods ON TRUE
 WHERE note.tenant_id = $1
   AND note.recorded_at = (
        SELECT max(current_note.recorded_at)
          FROM settlement_accounting.supplier_credit_note AS current_note
         WHERE current_note.tenant_id = note.tenant_id
           AND current_note.note_id = note.note_id
   )
   AND (
        SELECT count(*)
          FROM transport_fulfillment.transport_charge_occurrence_member AS counted
         WHERE counted.tenant_id = cost.tenant_id
           AND counted.occurrence_ref = cost.occurrence_id
           AND counted.validity_version = cost.occurrence_version
   ) = 1
   AND EXISTS (
        SELECT 1
          FROM transport_fulfillment.transport_charge_occurrence_member AS member
         WHERE member.tenant_id = cost.tenant_id
           AND member.occurrence_ref = cost.occurrence_id
           AND member.validity_version = cost.occurrence_version
           AND member.object_ref = $2
   )
   AND $3::boolean IS FALSE`

// 取已接受委托上、接受基线那一版提交申报的服务产品，再取它当前生效版本的形态。
// 处理尝试上的服务产品只在停顿时写下，接受之后认的是基线那一版，不是最近一次停顿。
// 两份已接受委托同时盖住这件包裹时不挑一份，交空串。申报值两侧空白不是另一个产品。
// 不可观察时交空串，不把空当成面单渠道。
const serviceFormSQL = `
SELECT COALESCE((
    SELECT form.form
      FROM parcel_shipment.shipment_request AS request
      JOIN LATERAL (
            SELECT CASE
                     WHEN request.snapshot #>> '{currentVersion,versionId}'
                          = request.snapshot #>> '{baseline,submissionVersion}'
                     THEN request.snapshot #>> '{currentVersion,requestedProduct}'
                     ELSE (
                         SELECT prior #>> '{requestedProduct}'
                           FROM jsonb_array_elements(
                                  COALESCE(request.snapshot -> 'priorVersions', '[]'::jsonb)
                                ) AS prior
                          WHERE prior #>> '{versionId}'
                                = request.snapshot #>> '{baseline,submissionVersion}'
                          LIMIT 1
                     )
                   END AS product
      ) AS declared ON TRUE
      JOIN party_commercial.service_product_form AS form
        ON form.tenant_id = request.tenant_id
       AND form.object_kind = 1
       AND form.object_id = btrim(declared.product)
      JOIN party_commercial.commercial_version AS version
        ON version.tenant_id = form.tenant_id
       AND version.object_kind = form.object_kind
       AND version.object_id = form.object_id
       AND version.version_label = form.version_label
       AND version.status = 3
     WHERE request.tenant_id = $1
       AND request.state = 2
       AND request.declared_parcel_ids @> ARRAY[$2]::text[]
       AND request.snapshot -> 'baseline' -> 'declaredParcelIds' ? $2
       AND declared.product IS NOT NULL
       AND btrim(declared.product) <> ''
       AND (
            SELECT count(*)
              FROM parcel_shipment.shipment_request AS other
             WHERE other.tenant_id = request.tenant_id
               AND other.state = 2
               AND other.declared_parcel_ids @> ARRAY[$2]::text[]
       ) = 1
     ORDER BY version.published_at DESC
     LIMIT 1
), '')`

// 挂在集运单元上，或挂在一份未取消且已接受的订舱所指名的班次上。
const sharedCostSQL = `
SELECT EXISTS (
    SELECT 1
      FROM node_operations.containment_current AS containment
     WHERE containment.tenant_id = $1
       AND containment.member_id = $2
) OR EXISTS (
    SELECT 1
      FROM transport_fulfillment.booking_request AS booking
      JOIN transport_fulfillment.transport_commission AS commission
        ON commission.tenant_id = booking.tenant_id
       AND commission.commission_id = booking.commission_id
       AND commission.cancelled_at IS NULL
      JOIN transport_fulfillment.booking_answer AS answer
        ON answer.tenant_id = booking.tenant_id
       AND answer.booking_id = booking.booking_id
       AND answer.outcome = 'ACCEPTED'
     WHERE booking.tenant_id = $1
       AND booking.cancelled_at IS NULL
       AND commission.members @> jsonb_build_array($2::text)
)`

// 当前链头或已形成应付所依的发生项，成员里有这一件但不止这一件。
const nonUniqueSQL = `
SELECT EXISTS (
    SELECT 1
      FROM (
            SELECT cost.tenant_id, cost.occurrence_id, cost.occurrence_version
              FROM settlement_accounting.supplier_expected_cost AS cost
             WHERE cost.tenant_id = $1
               AND NOT cost.withdrawn
               AND NOT EXISTS (
                    SELECT 1
                      FROM settlement_accounting.supplier_expected_cost AS later
                     WHERE later.tenant_id = cost.tenant_id
                       AND later.prior_version = cost.version
               )
            UNION
            SELECT cost.tenant_id, cost.occurrence_id, cost.occurrence_version
              FROM settlement_accounting.audited_payable AS payable
              JOIN settlement_accounting.supplier_expected_cost AS cost
                ON cost.tenant_id = payable.tenant_id
               AND cost.version = payable.expected_version
             WHERE payable.tenant_id = $1
      ) AS touched
      JOIN transport_fulfillment.transport_charge_occurrence_member AS member
        ON member.tenant_id = touched.tenant_id
       AND member.occurrence_ref = touched.occurrence_id
       AND member.validity_version = touched.occurrence_version
       AND member.object_ref = $2
     WHERE (
            SELECT count(*)
              FROM transport_fulfillment.transport_charge_occurrence_member AS counted
             WHERE counted.tenant_id = touched.tenant_id
               AND counted.occurrence_ref = touched.occurrence_id
               AND counted.validity_version = touched.occurrence_version
     ) <> 1
)`

// 成员恰一件时，那一件就是这件包裹。多于一件时只交回销售评价请求认得的包裹，避免把集运单元标识当成包裹去派生。
const occurrenceParcelsSQL = `
WITH members AS (
    SELECT object_ref
      FROM transport_fulfillment.transport_charge_occurrence_member
     WHERE tenant_id = $1
       AND occurrence_ref = $2
       AND validity_version = $3
)
SELECT object_ref
  FROM members
 WHERE (SELECT count(*) FROM members) = 1
    OR object_ref IN (
        SELECT request.parcel_id
          FROM settlement_accounting.sell_evaluation_request AS request
         WHERE request.tenant_id = $1
    )`
