package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// PreAcceptanceEstimateTraces 是受理前估价留痕（ADR-0179 决定六）。只有写口，写在环境事务里。
type PreAcceptanceEstimateTraces struct {
	db *bentopg.DB
}

func NewPreAcceptanceEstimateTraces(db *bentopg.DB) (*PreAcceptanceEstimateTraces, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &PreAcceptanceEstimateTraces{db: db}, nil
}

var _ ports.PreAcceptanceEstimateTraces = (*PreAcceptanceEstimateTraces)(nil)

// estimateTrialRow 是 trials 列里一件的形状。形成了的估价每一件都有合计，所以合计与币种两格不设缺席。
type estimateTrialRow struct {
	Parcel     string   `json:"parcel"`
	Evaluation string   `json:"evaluation"`
	Versions   []string `json:"versions"`
	TotalMinor int64    `json:"totalMinor"`
	Currency   string   `json:"currency"`
}

// RecordEstimate 插一行；键撞上即在库里逐列比内容，分重放与冲突，不覆盖。比较放在 SQL 里做：时点按同一种编码落库与比较，
// trials 按 jsonb 的相等比，Go 侧不必再猜库里那份截到了哪一位。trial_at 在比较之列、recorded_at 不在：跨重驱是重放靠的是判断时点
// 不动（端口 PreAcceptanceEstimateTraces 的头注写了这个前提）。
func (repository *PreAcceptanceEstimateTraces) RecordEstimate(
	ctx context.Context,
	trace ports.PreAcceptanceEstimateTrace,
) (ports.PreAcceptanceEstimateTraceEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("record pre-acceptance estimate: %w", err)
	}
	estimate := trace.Estimate
	trials, err := marshalEstimateTrials(estimate)
	if err != nil {
		return 0, fmt.Errorf("record pre-acceptance estimate: %w", err)
	}
	key := []any{
		trace.Tenant.String(),
		trace.Scope.LegalEntity().String(),
		trace.Scope.Account().String(),
		trace.Scope.Currency().String(),
		trace.Request.String(),
	}
	content := []any{
		trace.ShipmentRequest,
		trace.SubmissionVersion,
		trace.At.UTC(),
		estimate.Form().String(),
		estimate.PricingScope().String(),
		estimate.PriceCard(),
		estimate.TotalMinor(),
		trials,
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.pre_acceptance_estimate_trace
			(tenant_id, legal_entity, account_id, currency, control_request_id,
			 shipment_request_ref, submission_version_ref, trial_at, form, pricing_scope_id,
			 price_card, total_minor, trials, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 ON CONFLICT ON CONSTRAINT pre_acceptance_estimate_trace_pkey DO NOTHING`,
		append(append(append([]any{}, key...), content...), trace.RecordedAt.UTC())...,
	)
	if err != nil {
		return 0, fmt.Errorf("record pre-acceptance estimate: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.PreAcceptanceEstimateTraceRecorded, nil
	}

	var same bool
	if err := executor.QueryRow(ctx,
		`SELECT shipment_request_ref = $6 AND submission_version_ref = $7 AND trial_at = $8
		        AND form = $9 AND pricing_scope_id = $10 AND price_card = $11 AND total_minor = $12
		        AND trials = $13::jsonb
		   FROM settlement_accounting.pre_acceptance_estimate_trace
		  WHERE tenant_id = $1 AND legal_entity = $2 AND account_id = $3 AND currency = $4
		    AND control_request_id = $5`,
		append(append([]any{}, key...), content...)...,
	).Scan(&same); err != nil {
		return 0, fmt.Errorf("record pre-acceptance estimate: %w", err)
	}
	if same {
		return ports.PreAcceptanceEstimateTraceReplay, nil
	}
	return ports.PreAcceptanceEstimateTraceConflict, nil
}

func marshalEstimateTrials(estimate domain.PreAcceptanceEstimate) ([]byte, error) {
	trials := estimate.Trials()
	rows := make([]estimateTrialRow, 0, len(trials))
	for _, trial := range trials {
		minor, currency, present := trial.Total()
		if !present {
			// 形成了的估价里每一件都有合计（FormPerParcelSellTrialSum 的停法），走到这里是调用方交了一份没形成的估价。
			return nil, fmt.Errorf("trial %s carries no total", trial.Parcel())
		}
		rows = append(rows, estimateTrialRow{
			Parcel:     trial.Parcel(),
			Evaluation: trial.Evaluation(),
			Versions:   trial.Versions(),
			TotalMinor: minor,
			Currency:   currency.String(),
		})
	}
	return json.Marshal(rows)
}
