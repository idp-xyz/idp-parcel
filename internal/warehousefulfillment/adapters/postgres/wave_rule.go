package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func (store *Waves) LoadWaveRule(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.WaveRuleCode) (ports.WaveRule, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.WaveRule{}, false, err
	}
	rule := ports.WaveRule{Warehouse: warehouse, Code: code}
	var owner, dispatch *string
	err = querier.QueryRow(ctx,
		`SELECT revision, rule_name, owner_account_id, dispatch, profile, max_requests, batch_pick, deactivated, operator_id, registered_at
		   FROM warehouse_fulfillment.wave_rule
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND rule_code = $3
		  ORDER BY revision DESC
		  LIMIT 1`,
		tenant.String(), warehouse.String(), code.String()).Scan(
		&rule.Revision, &rule.Name, &owner, &dispatch, &rule.Profile, &rule.MaxRequests, &rule.BatchPick, &rule.Deactivated,
		&rule.Operator, &rule.RegisteredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.WaveRule{}, false, nil
	}
	if err != nil {
		return ports.WaveRule{}, false, fmt.Errorf("load wave rule: %w", err)
	}
	rule.Owner, rule.Dispatch = valueOrEmpty(owner), valueOrEmpty(dispatch)
	return rule, true, nil
}

func (store *Waves) SaveWaveRule(ctx context.Context, tenant domain.TenantID, rule ports.WaveRule) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save wave rule: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.wave_rule (
		    tenant_id, warehouse_code, rule_code, revision, rule_name, owner_account_id, dispatch, profile,
		    max_requests, batch_pick, deactivated, operator_id, registered_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		tenant.String(), rule.Warehouse.String(), rule.Code.String(), rule.Revision, rule.Name,
		optionalText(rule.Owner, rule.Owner != ""), optionalText(rule.Dispatch, rule.Dispatch != ""), rule.Profile,
		rule.MaxRequests, rule.BatchPick, rule.Deactivated, rule.Operator, rule.RegisteredAt.UTC()); err != nil {
		return fmt.Errorf("save wave rule: %w", err)
	}
	return nil
}

// waveRuleMatches 是「这张已预占的请求归不归这条规则圈」的那一句。$1 租户，$2 仓，$3 货主（空是不限），
// $4 发运方式（空是不限），$5 件型。单品单件的口径与出库单列表按件型筛的是同一条：预占需求合计是 1 个基本单位。
const waveRuleMatches = `request.tenant_id = $1 AND request.warehouse_code = $2 AND request.status = 'RESERVED'
	AND ($3 = '' OR request.owner_account_id = $3)
	AND ($4 = '' OR request.dispatch = $4)
	AND ($5 = '` + ports.WaveRuleProfileAny + `'
	     OR ((SELECT coalesce(sum(reservation.wanted), 0)
	            FROM warehouse_fulfillment.outbound_reservation reservation
	           WHERE reservation.tenant_id = request.tenant_id
	             AND reservation.request_id = request.request_id) = 1) = ($5 = '` + ports.WaveRuleProfileSingleUnit + `'))`

func (store *Waves) WaveRuleCandidates(ctx context.Context, tenant domain.TenantID, rule ports.WaveRule, limit int) ([]domain.OutboundRequestID, int, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, 0, err
	}
	arguments := []any{tenant.String(), rule.Warehouse.String(), rule.Owner, rule.Dispatch, rule.Profile}
	var total int
	if err := querier.QueryRow(ctx,
		`SELECT count(*) FROM warehouse_fulfillment.outbound_request request WHERE `+waveRuleMatches, arguments...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("wave rule candidates: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT request.request_id FROM warehouse_fulfillment.outbound_request request
		  WHERE `+waveRuleMatches+`
		  ORDER BY request.received_at, request.request_id
		  LIMIT $6`, append(arguments, limit)...)
	if err != nil {
		return nil, 0, fmt.Errorf("wave rule candidates: %w", err)
	}
	defer rows.Close()
	var requests []domain.OutboundRequestID
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, 0, fmt.Errorf("wave rule candidates: %w", err)
		}
		request, err := domain.NewOutboundRequestID(raw)
		if err != nil {
			return nil, 0, fmt.Errorf("wave rule candidates: %w", err)
		}
		requests = append(requests, request)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("wave rule candidates: %w", err)
	}
	return requests, total, nil
}

func (store *Waves) LoadWaveRuleRun(ctx context.Context, tenant domain.TenantID, runID string) (ports.WaveRuleRun, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.WaveRuleRun{}, false, err
	}
	run := ports.WaveRuleRun{RunID: runID}
	var warehouse, rule string
	err = querier.QueryRow(ctx,
		`SELECT warehouse_code, rule_code, rule_revision, operator_id, ran_at, released, skipped, remaining
		   FROM warehouse_fulfillment.wave_rule_run
		  WHERE tenant_id = $1 AND run_id = $2`,
		tenant.String(), runID).Scan(&warehouse, &rule, &run.Revision, &run.Operator, &run.RanAt, &run.Released, &run.Skipped, &run.Remaining)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.WaveRuleRun{}, false, nil
	}
	if err != nil {
		return ports.WaveRuleRun{}, false, fmt.Errorf("load wave rule run: %w", err)
	}
	if run.Warehouse, err = domain.NewWarehouseCode(warehouse); err != nil {
		return ports.WaveRuleRun{}, false, fmt.Errorf("load wave rule run: %w", err)
	}
	if run.Rule, err = domain.NewWaveRuleCode(rule); err != nil {
		return ports.WaveRuleRun{}, false, fmt.Errorf("load wave rule run: %w", err)
	}
	return run, true, nil
}

func (store *Waves) SaveWaveRuleRun(ctx context.Context, tenant domain.TenantID, run ports.WaveRuleRun) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save wave rule run: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.wave_rule_run (
		    tenant_id, run_id, warehouse_code, rule_code, rule_revision, operator_id, ran_at, released, skipped, remaining
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		tenant.String(), run.RunID, run.Warehouse.String(), run.Rule.String(), run.Revision, run.Operator, run.RanAt.UTC(),
		run.Released, run.Skipped, run.Remaining); err != nil {
		return fmt.Errorf("save wave rule run: %w", err)
	}
	return nil
}

func (store *Waves) TagWaveRun(ctx context.Context, tenant domain.TenantID, wave domain.WaveID, rule domain.WaveRuleCode, runID string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("tag wave run: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.wave SET rule_code = $3, rule_run = $4
		  WHERE tenant_id = $1 AND wave_id = $2 AND rule_run IS NULL`,
		tenant.String(), wave.String(), rule.String(), runID)
	if err != nil {
		return fmt.Errorf("tag wave run: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("tag wave run: wave %s is not there or already belongs to a run", wave.String())
	}
	return nil
}

func (store *Waves) SaveAllocationException(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, source, reference string, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save allocation exception: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_allocation_exception (tenant_id, request_id, source, reference, recorded_at)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (tenant_id, request_id) DO UPDATE
		    SET source = EXCLUDED.source, reference = EXCLUDED.reference, recorded_at = EXCLUDED.recorded_at`,
		tenant.String(), request.String(), source, reference, at.UTC()); err != nil {
		return fmt.Errorf("save allocation exception: %w", err)
	}
	return nil
}

func (store *Waves) HasAllocationException(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return false, err
	}
	var has bool
	if err := querier.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM warehouse_fulfillment.outbound_allocation_exception WHERE tenant_id = $1 AND request_id = $2)`,
		tenant.String(), request.String()).Scan(&has); err != nil {
		return false, fmt.Errorf("has allocation exception: %w", err)
	}
	return has, nil
}

func (store *Waves) WavesOfRun(ctx context.Context, tenant domain.TenantID, runID string) ([]ports.RuleWave, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT wave.wave_id,
		        (SELECT count(*) FROM warehouse_fulfillment.wave_request member
		          WHERE member.tenant_id = wave.tenant_id AND member.wave_id = wave.wave_id)
		   FROM warehouse_fulfillment.wave wave
		  WHERE wave.tenant_id = $1 AND wave.rule_run = $2
		  ORDER BY wave.wave_id`,
		tenant.String(), runID)
	if err != nil {
		return nil, fmt.Errorf("waves of run: %w", err)
	}
	defer rows.Close()
	var waves []ports.RuleWave
	for rows.Next() {
		var wave ports.RuleWave
		if err := rows.Scan(&wave.Wave, &wave.Requests); err != nil {
			return nil, fmt.Errorf("waves of run: %w", err)
		}
		waves = append(waves, wave)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("waves of run: %w", err)
	}
	return waves, nil
}
