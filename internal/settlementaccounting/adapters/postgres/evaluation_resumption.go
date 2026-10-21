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

// EvaluationResumptions 实现两本发起停办册与续办队列（ADR-0229 决定四、五）。队列跨四本册读：两本发起停办册与两本
// 请求册，口径写在 ports.StalledEvaluationQueue 头注。同一个适配器而不是四个：「等着」的口径要四本册一起定序、一起分页，
// 拆开就得在调用方再合一次。
type EvaluationResumptions struct {
	db *bentopg.DB
}

func NewEvaluationResumptions(db *bentopg.DB) (*EvaluationResumptions, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &EvaluationResumptions{db: db}, nil
}

var (
	_ ports.SellEvaluationInitiationStallRegister = (*EvaluationResumptions)(nil)
	_ ports.BuyEvaluationInitiationStallRegister  = (*EvaluationResumptions)(nil)
	_ ports.StalledEvaluationQueue                = (*EvaluationResumptions)(nil)
)

// RecordSellInitiationStall 落一行或换上这一次的停因、时刻与依据。来源事实与计价基准时点不换：同一件包裹在同一时点的
// 那一拍只有一组，再来的是重放或续办重跑，带的仍是记下的那组。
func (stalls *EvaluationResumptions) RecordSellInitiationStall(
	ctx context.Context,
	tenant domain.TenantID,
	stall domain.SellEvaluationInitiationStall,
) error {
	if strings.TrimSpace(tenant.String()) == "" || stall.Cause() == domain.EvaluationStopCauseInvalid {
		return fmt.Errorf("record sell evaluation initiation stall: the tenant or the stall is blank")
	}
	executor, err := stalls.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record sell evaluation initiation stall: %w", err)
	}
	key := stall.NaturalKey()
	_, err = executor.Exec(ctx,
		`INSERT INTO settlement_accounting.sell_evaluation_initiation_stall
			(tenant_id, parcel_id, billing_moment, source_fact_ref, pricing_basis_at, stop_cause, stopped_at, resume_basis)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (tenant_id, parcel_id, billing_moment) DO UPDATE
		    SET stop_cause = EXCLUDED.stop_cause, stopped_at = EXCLUDED.stopped_at, resume_basis = EXCLUDED.resume_basis`,
		tenant.String(), key.Parcel.String(), key.Moment.String(), stall.Source().String(), stall.PricingBasisAt().UTC(),
		stall.Cause().String(), stall.StoppedAt().UTC(), optionalBasis(stall.Basis()))
	if err != nil {
		return fmt.Errorf("record sell evaluation initiation stall: %w", err)
	}
	return nil
}

func (stalls *EvaluationResumptions) FindSellInitiationStall(
	ctx context.Context,
	tenant domain.TenantID,
	key domain.SellEvaluationRequestNaturalKey,
) (domain.SellEvaluationInitiationStall, bool, error) {
	none := domain.SellEvaluationInitiationStall{}
	querier, err := stalls.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("find sell evaluation initiation stall: %w", err)
	}
	var (
		source, cause      string
		basis              *string
		basisAt, stoppedAt time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT source_fact_ref, pricing_basis_at, stop_cause, stopped_at, resume_basis
		   FROM settlement_accounting.sell_evaluation_initiation_stall
		  WHERE tenant_id = $1 AND parcel_id = $2 AND billing_moment = $3`,
		tenant.String(), key.Parcel.String(), key.Moment.String(),
	).Scan(&source, &basisAt, &cause, &stoppedAt, &basis)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("find sell evaluation initiation stall: %w", err)
	}
	sourceRef, err := domain.NewSourceFactReference(source)
	if err != nil {
		return none, false, fmt.Errorf("find sell evaluation initiation stall: %w", err)
	}
	stopCause, err := domain.EvaluationStopCauseFromName(cause)
	if err != nil {
		return none, false, fmt.Errorf("find sell evaluation initiation stall: %w", err)
	}
	resume, err := rebuildResumeBasis(derefText(basis))
	if err != nil {
		return none, false, fmt.Errorf("find sell evaluation initiation stall: %w", err)
	}
	stall, err := domain.NewSellEvaluationInitiationStall(domain.SellEvaluationInitiationStallSpec{
		Parcel:         key.Parcel,
		Moment:         key.Moment,
		Source:         sourceRef,
		PricingBasisAt: basisAt,
		Cause:          stopCause,
		StoppedAt:      stoppedAt,
		Basis:          resume,
	})
	if err != nil {
		return none, false, fmt.Errorf("find sell evaluation initiation stall: %w", err)
	}
	return stall, true, nil
}

// RecordBuyInitiationStall 落一行或换上这一次的停因、时刻与依据，口径同 SELL 那一本。
func (stalls *EvaluationResumptions) RecordBuyInitiationStall(
	ctx context.Context,
	tenant domain.TenantID,
	stall domain.BuyEvaluationInitiationStall,
) error {
	if strings.TrimSpace(tenant.String()) == "" || stall.Cause() == domain.EvaluationStopCauseInvalid {
		return fmt.Errorf("record buy evaluation initiation stall: the tenant or the stall is blank")
	}
	executor, err := stalls.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record buy evaluation initiation stall: %w", err)
	}
	occurrence := stall.Occurrence()
	_, err = executor.Exec(ctx,
		`INSERT INTO settlement_accounting.buy_evaluation_initiation_stall
			(tenant_id, occurrence_id, occurrence_version, occurrence_reason, occurred_at, agreement_ref,
			 stop_cause, stopped_at, resume_basis)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (tenant_id, occurrence_id, occurrence_version) DO UPDATE
		    SET stop_cause = EXCLUDED.stop_cause, stopped_at = EXCLUDED.stopped_at, resume_basis = EXCLUDED.resume_basis`,
		tenant.String(), occurrence.ID().String(), occurrence.Version().String(), occurrence.Reason().String(),
		occurrence.OccurredAt().UTC(), stall.Agreement().String(),
		stall.Cause().String(), stall.StoppedAt().UTC(), optionalBasis(stall.Basis()))
	if err != nil {
		return fmt.Errorf("record buy evaluation initiation stall: %w", err)
	}
	return nil
}

func (stalls *EvaluationResumptions) FindBuyInitiationStall(
	ctx context.Context,
	tenant domain.TenantID,
	occurrenceID domain.ChargeOccurrenceID,
	version domain.OccurrenceVersion,
) (domain.BuyEvaluationInitiationStall, bool, error) {
	none := domain.BuyEvaluationInitiationStall{}
	querier, err := stalls.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("find buy evaluation initiation stall: %w", err)
	}
	var (
		reason, agreement, cause string
		basis                    *string
		occurredAt, stoppedAt    time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT occurrence_reason, occurred_at, agreement_ref, stop_cause, stopped_at, resume_basis
		   FROM settlement_accounting.buy_evaluation_initiation_stall
		  WHERE tenant_id = $1 AND occurrence_id = $2 AND occurrence_version = $3`,
		tenant.String(), occurrenceID.String(), version.String(),
	).Scan(&reason, &occurredAt, &agreement, &cause, &stoppedAt, &basis)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("find buy evaluation initiation stall: %w", err)
	}
	occurrence, err := rebuildChargeOccurrence(occurrenceID.String(), reason, version.String(), occurredAt)
	if err != nil {
		return none, false, fmt.Errorf("find buy evaluation initiation stall: %w", err)
	}
	agreementRef, err := domain.NewSupplierAgreementReference(agreement)
	if err != nil {
		return none, false, fmt.Errorf("find buy evaluation initiation stall: %w", err)
	}
	stopCause, err := domain.EvaluationStopCauseFromName(cause)
	if err != nil {
		return none, false, fmt.Errorf("find buy evaluation initiation stall: %w", err)
	}
	resume, err := rebuildResumeBasis(derefText(basis))
	if err != nil {
		return none, false, fmt.Errorf("find buy evaluation initiation stall: %w", err)
	}
	stall, err := domain.NewBuyEvaluationInitiationStall(domain.BuyEvaluationInitiationStallSpec{
		Occurrence: occurrence,
		Agreement:  agreementRef,
		Cause:      stopCause,
		StoppedAt:  stoppedAt,
		Basis:      resume,
	})
	if err != nil {
		return none, false, fmt.Errorf("find buy evaluation initiation stall: %w", err)
	}
	return stall, true, nil
}

// ListStalled 把四本册里等着的东西合成一张队列。每一支都带同一组条件：停因在名单里；这条代次链上没有哪一代凭被排除的
// 依据续办出来、发起停办也没有凭它重跑过；请求那两支再要求还没有下一代，发起停办那两支要求这一拍还没有任何一份请求。
// 计价范围在场时发起停办两支整支不取；点名包裹时只取 SELL 两支、点名发生项时只取 BUY 两支。
//
// 定序按停下的时刻、再按种类与键；位置在场时只取排在它之后的——按位置翻页，办过却没写回的件不会把后面的挡住。
func (stalls *EvaluationResumptions) ListStalled(
	ctx context.Context,
	tenant domain.TenantID,
	query ports.StalledEvaluationQuery,
	limit int,
) ([]ports.StalledEvaluation, error) {
	if len(query.Causes) == 0 || limit <= 0 {
		return nil, fmt.Errorf("%w: causes %d, limit %d", ports.ErrStalledEvaluationQueryInvalid, len(query.Causes), limit)
	}
	if query.Occurrence.String() != "" && query.OccurrenceVersion.String() == "" {
		return nil, fmt.Errorf("%w: the occurrence is named without its version", ports.ErrStalledEvaluationQueryInvalid)
	}
	causes := make([]string, 0, len(query.Causes))
	for _, cause := range query.Causes {
		if cause.String() == "" {
			return nil, fmt.Errorf("%w: a blank stop cause", ports.ErrStalledEvaluationQueryInvalid)
		}
		causes = append(causes, cause.String())
	}
	var afterAt *time.Time
	var afterKind, afterFirst, afterSecond *string
	if after := query.After; after.Present() {
		at := after.StoppedAt.UTC()
		afterAt, afterKind, afterFirst, afterSecond = &at, &after.Kind, &after.FirstKey, &after.SecondKey
	}
	querier, err := stalls.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list stalled evaluations: %w", err)
	}
	rows, err := querier.Query(ctx, stalledEvaluationsSQL,
		tenant.String(), causes, optionalBasis(query.ExcludeBasis), optionalText(query.PricingScope),
		optionalText(query.Parcel.String()), optionalText(query.Occurrence.String()),
		optionalText(query.OccurrenceVersion.String()), limit,
		afterAt, afterKind, afterFirst, afterSecond)
	if err != nil {
		return nil, fmt.Errorf("list stalled evaluations: %w", err)
	}
	defer rows.Close()

	var stalled []ports.StalledEvaluation
	for rows.Next() {
		var (
			kind, cause, first, second string
			stoppedAt                  time.Time
		)
		if err := rows.Scan(&kind, &cause, &first, &second, &stoppedAt); err != nil {
			return nil, fmt.Errorf("list stalled evaluations: %w", err)
		}
		item, err := rebuildStalledEvaluation(kind, cause, first, second)
		if err != nil {
			return nil, fmt.Errorf("list stalled evaluations: %w", err)
		}
		item.Position = ports.StalledEvaluationPosition{StoppedAt: stoppedAt.UTC(), Kind: kind, FirstKey: first, SecondKey: second}
		stalled = append(stalled, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list stalled evaluations: %w", err)
	}
	return stalled, nil
}

// stalledEvaluationsSQL 的参数：$1 租户、$2 停因名单、$3 排除的依据（可空）、$4 计价范围（可空）、$5 包裹（可空）、
// $6 发生项（可空）、$7 发生项版本（可空）、$8 上限、$9–$12 翻页位置（可空，四格同在同缺）。各支交回同形四列：种类、
// 停因、两段键、停下时刻。排除依据对请求那两支查整条代次链（同一自然键上任一代），不只看链头：一条链先凭 B 续办、
// 再凭 C 续办出又停下的一代，B 重投时不该再推它。
const stalledEvaluationsSQL = `
SELECT kind, stop_cause, first_key, second_key, stopped_at FROM (
	SELECT 'SELL_INITIATION' AS kind, s.stop_cause, s.parcel_id AS first_key, s.billing_moment AS second_key, s.stopped_at
	  FROM settlement_accounting.sell_evaluation_initiation_stall s
	 WHERE s.tenant_id = $1 AND s.stop_cause = ANY($2)
	   AND ($3::text IS NULL OR s.resume_basis IS DISTINCT FROM $3)
	   AND $4::text IS NULL AND $6::text IS NULL
	   AND ($5::text IS NULL OR s.parcel_id = $5)
	   AND NOT EXISTS (SELECT 1 FROM settlement_accounting.sell_evaluation_request r
	                    WHERE r.tenant_id = s.tenant_id AND r.parcel_id = s.parcel_id
	                      AND r.billing_moment = s.billing_moment)
	UNION ALL
	SELECT 'BUY_INITIATION', b.stop_cause, b.occurrence_id, b.occurrence_version, b.stopped_at
	  FROM settlement_accounting.buy_evaluation_initiation_stall b
	 WHERE b.tenant_id = $1 AND b.stop_cause = ANY($2)
	   AND ($3::text IS NULL OR b.resume_basis IS DISTINCT FROM $3)
	   AND $4::text IS NULL AND $5::text IS NULL
	   AND ($6::text IS NULL OR (b.occurrence_id = $6 AND b.occurrence_version = $7))
	   AND NOT EXISTS (SELECT 1 FROM settlement_accounting.evaluation_request r
	                    WHERE r.tenant_id = b.tenant_id AND r.occurrence_id = b.occurrence_id
	                      AND r.occurrence_version = b.occurrence_version)
	UNION ALL
	SELECT 'SELL_REQUEST', q.stop_cause, q.request_id, '', q.stopped_at
	  FROM settlement_accounting.sell_evaluation_request q
	 WHERE q.tenant_id = $1 AND q.stop_cause = ANY($2)
	   AND ($3::text IS NULL OR NOT EXISTS (
	         SELECT 1 FROM settlement_accounting.sell_evaluation_request c
	          WHERE c.tenant_id = q.tenant_id AND c.parcel_id = q.parcel_id
	            AND c.billing_moment = q.billing_moment AND c.resume_basis = $3))
	   AND ($4::text IS NULL OR q.pricing_scope_id = $4)
	   AND $6::text IS NULL
	   AND ($5::text IS NULL OR q.parcel_id = $5)
	   AND NOT EXISTS (SELECT 1 FROM settlement_accounting.sell_evaluation_request n
	                    WHERE n.tenant_id = q.tenant_id AND n.previous_request_id = q.request_id)
	UNION ALL
	SELECT 'BUY_REQUEST', e.stop_cause, e.request_id, '', e.stopped_at
	  FROM settlement_accounting.evaluation_request e
	 WHERE e.tenant_id = $1 AND e.stop_cause = ANY($2)
	   AND ($3::text IS NULL OR NOT EXISTS (
	         SELECT 1 FROM settlement_accounting.evaluation_request c
	          WHERE c.tenant_id = e.tenant_id AND c.scope_ref = e.scope_ref AND c.purpose = e.purpose
	            AND c.source_digest = e.source_digest AND c.resume_basis = $3))
	   AND ($4::text IS NULL OR e.scope_ref = $4)
	   AND $5::text IS NULL
	   AND ($6::text IS NULL OR (e.occurrence_id = $6 AND e.occurrence_version = $7))
	   AND NOT EXISTS (SELECT 1 FROM settlement_accounting.evaluation_request n
	                    WHERE n.tenant_id = e.tenant_id AND n.previous_request_id = e.request_id)
) stalled
WHERE $9::timestamptz IS NULL OR (stopped_at, kind, first_key, second_key) > ($9::timestamptz, $10::text, $11::text, $12::text)
ORDER BY stopped_at, kind, first_key, second_key
LIMIT $8`

func rebuildStalledEvaluation(kind, cause, first, second string) (ports.StalledEvaluation, error) {
	stopCause, err := domain.EvaluationStopCauseFromName(cause)
	if err != nil {
		return ports.StalledEvaluation{}, err
	}
	item := ports.StalledEvaluation{Cause: stopCause}
	switch kind {
	case ports.StalledSellInitiation.String():
		item.Kind = ports.StalledSellInitiation
		parcel, err := domain.NewParcelReference(first)
		if err != nil {
			return ports.StalledEvaluation{}, err
		}
		moment, err := domain.ParcelBillingMomentFromName(second)
		if err != nil {
			return ports.StalledEvaluation{}, err
		}
		item.SellMoment = domain.SellEvaluationRequestNaturalKey{Parcel: parcel, Moment: moment}
	case ports.StalledBuyInitiation.String():
		item.Kind = ports.StalledBuyInitiation
		if item.Occurrence, err = domain.NewChargeOccurrenceID(first); err != nil {
			return ports.StalledEvaluation{}, err
		}
		if item.OccurrenceVersion, err = domain.NewOccurrenceVersion(second); err != nil {
			return ports.StalledEvaluation{}, err
		}
	case ports.StalledSellRequest.String():
		item.Kind = ports.StalledSellRequest
		if item.SellRequest, err = domain.NewSellEvaluationRequestID(first); err != nil {
			return ports.StalledEvaluation{}, err
		}
	case ports.StalledBuyRequest.String():
		item.Kind = ports.StalledBuyRequest
		if item.BuyRequest, err = domain.NewEvaluationRequestID(first); err != nil {
			return ports.StalledEvaluation{}, err
		}
	default:
		return ports.StalledEvaluation{}, fmt.Errorf("stalled evaluation kind %q", kind)
	}
	return item, nil
}

// optionalBasis 把没有依据写成 NULL，可缺的文本条件经同包的 optionalText 同样写成 NULL：可空列与可缺条件都以 NULL
// 表示「没有」，空串会被 CHECK 拒，或在条件里被当成一个真要比的值。
func optionalBasis(basis domain.ResumeBasis) *string {
	if !basis.Present() {
		return nil
	}
	return optionalText(basis.String())
}
