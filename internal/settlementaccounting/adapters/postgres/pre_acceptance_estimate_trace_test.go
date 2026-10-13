package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// estimateOf 经领域的形态合出一份估价：两件各有合计，版本清单里含价卡与燃油序列。
func estimateOf(t *testing.T, minors ...int64) domain.PreAcceptanceEstimate {
	t.Helper()
	cny := mustAccountRef(t, domain.NewCurrencyCode, "CNY")
	trials := make([]domain.PreAcceptanceTrial, 0, len(minors))
	for index, minor := range minors {
		parcel := []string{"P-1", "P-2", "P-3"}[index]
		trial, err := domain.NewPreAcceptanceTrial(parcel, "EST-"+parcel,
			[]string{"PRICE_CARD:SYN-PLAN-SELL@v1", "REFERENCE_SERIES:SYN-FUEL@v3"}, minor, cny)
		if err != nil {
			t.Fatalf("trial: %v", err)
		}
		trials = append(trials, trial)
	}
	estimate, gap, err := domain.FormPerParcelSellTrialSum(estimateAdoption(t, "ACCT-1", "SCOPE-SELL-1"),
		"PRICE_CARD:SYN-PLAN-SELL@v1", cny, trials)
	if err != nil || gap != domain.PreAcceptanceEstimateFormed {
		t.Fatalf("estimate: gap = %s err = %v", gap, err)
	}
	return estimate
}

func estimateTrace(t *testing.T, request string, estimate domain.PreAcceptanceEstimate, at time.Time) ports.PreAcceptanceEstimateTrace {
	t.Helper()
	scope, err := domain.NewSettlementScope(
		mustAccountRef(t, domain.NewLegalEntityReference, "LE-1"),
		mustAccountRef(t, domain.NewSettlementAccountID, "ACCT-1"),
		mustAccountRef(t, domain.NewCurrencyCode, "CNY"),
	)
	if err != nil {
		t.Fatalf("scope: %v", err)
	}
	return ports.PreAcceptanceEstimateTrace{
		Tenant:            settlementTenant(t),
		Scope:             scope,
		Request:           mustAccountRef(t, domain.NewControlRequestID, request),
		ShipmentRequest:   "SHR-SYN-01",
		SubmissionVersion: "SV-SYN-01",
		At:                time.Date(2026, 10, 13, 8, 0, 0, 123456789, time.UTC),
		Estimate:          estimate,
		RecordedAt:        at,
	}
}

// Covers: ADR-0179 决定六——估价随控制结果留痕、可复算：一行带形态、计价范围、价卡、逐件评价标识与版本清单、合计，按 trials 那一列
// 逐件加回得同一合计；同一次控制请求带同一份估价再写是重放（落库时刻换了也算同一份，时点的纳秒尾数不让重放变冲突），带另一份是冲突，
// 都不覆盖；写口在事务外被调按框架合同拒。
func TestAPreAcceptanceEstimateTraceIsWrittenOnceAndReproducible(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	traces, err := adapter.NewPreAcceptanceEstimateTraces(db)
	if err != nil {
		t.Fatalf("构造留痕册：%v", err)
	}
	ctx := t.Context()
	at := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	record := func(trace ports.PreAcceptanceEstimateTrace) (ports.PreAcceptanceEstimateTraceEffect, error) {
		var effect ports.PreAcceptanceEstimateTraceEffect
		err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var recordErr error
			effect, recordErr = traces.RecordEstimate(txCtx, trace)
			return recordErr
		})
		return effect, err
	}

	if _, err := traces.RecordEstimate(ctx, estimateTrace(t, "SHR-SYN-01/SV-SYN-01", estimateOf(t, 1250, 800), at)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 RecordEstimate 应答 ErrTransactionRequired，实得：%v", err)
	}
	for _, step := range []struct {
		name   string
		trace  ports.PreAcceptanceEstimateTrace
		effect ports.PreAcceptanceEstimateTraceEffect
	}{
		{"首写", estimateTrace(t, "SHR-SYN-01/SV-SYN-01", estimateOf(t, 1250, 800), at), ports.PreAcceptanceEstimateTraceRecorded},
		{"重放", estimateTrace(t, "SHR-SYN-01/SV-SYN-01", estimateOf(t, 1250, 800), at.Add(time.Hour)), ports.PreAcceptanceEstimateTraceReplay},
		{"同合计异逐件", estimateTrace(t, "SHR-SYN-01/SV-SYN-01", estimateOf(t, 1050, 1000), at), ports.PreAcceptanceEstimateTraceConflict},
		{"换合计", estimateTrace(t, "SHR-SYN-01/SV-SYN-01", estimateOf(t, 1250), at), ports.PreAcceptanceEstimateTraceConflict},
		{"另一次请求", estimateTrace(t, "SHR-SYN-01/SV-SYN-02", estimateOf(t, 1250), at), ports.PreAcceptanceEstimateTraceRecorded},
	} {
		effect, err := record(step.trace)
		if err != nil || effect != step.effect {
			t.Fatalf("%s：effect = %d err = %v, 想要 %d", step.name, effect, err, step.effect)
		}
	}

	var (
		form, scope, card string
		total             int64
		raw               []byte
	)
	if err := pool.QueryRow(ctx,
		`SELECT form, pricing_scope_id, price_card, total_minor, trials
		   FROM settlement_accounting.pre_acceptance_estimate_trace
		  WHERE tenant_id = 'tenant-a' AND control_request_id = 'SHR-SYN-01/SV-SYN-01'`,
	).Scan(&form, &scope, &card, &total, &raw); err != nil {
		t.Fatalf("读回留痕：%v", err)
	}
	if form != "PER_PARCEL_SELL_TRIAL_SUM" || scope != "SCOPE-SELL-1" || card != "PRICE_CARD:SYN-PLAN-SELL@v1" || total != 2050 {
		t.Fatalf("留痕 = %s %s %s %d, 想要首写那一份、冲突不覆盖", form, scope, card, total)
	}
	var trials []struct {
		Evaluation string   `json:"evaluation"`
		Versions   []string `json:"versions"`
		TotalMinor int64    `json:"totalMinor"`
	}
	if err := json.Unmarshal(raw, &trials); err != nil {
		t.Fatalf("trials 列：%v", err)
	}
	var recomputed int64
	for _, trial := range trials {
		if trial.Evaluation == "" || len(trial.Versions) != 2 {
			t.Fatalf("逐件 = %+v, 想要评价标识与版本清单原样", trial)
		}
		recomputed += trial.TotalMinor
	}
	if len(trials) != 2 || recomputed != total {
		t.Fatalf("按留痕逐件加回得 %d（%d 件）, 想要 %d", recomputed, len(trials), total)
	}
}
