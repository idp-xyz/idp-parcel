package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// StatementCycles 是对账周期册与到点截单截止记录的仓储（迁移 0049，ADR-0339 决定三），也给到点截单收拢候选费用。
type StatementCycles struct {
	db *bentopg.DB
}

var (
	_ ports.StatementCycleRegister = (*StatementCycles)(nil)
	_ ports.StatementCycleLedger   = (*StatementCycles)(nil)
	_ ports.CycleCutCandidateView  = (*StatementCycles)(nil)
)

func NewStatementCycles(db *bentopg.DB) (*StatementCycles, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &StatementCycles{db: db}, nil
}

func (repository *StatementCycles) SaveStatementCycle(
	ctx context.Context,
	tenant domain.TenantID,
	cycle domain.StatementCycle,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save statement cycle: %w", err)
	}
	var weekday, monthDay *int
	if day, has := cycle.Weekday(); has {
		value := int(day)
		weekday = &value
	}
	if cycle.Kind() == domain.MonthlyCycle {
		value := cycle.MonthDay()
		monthDay = &value
	}
	tag, err := executor.Exec(ctx, `
		INSERT INTO settlement_accounting.statement_cycle (
			tenant_id, account_id, cycle_kind, closing_weekday, closing_month_day, since_date, registered_at
		) VALUES ($1, $2, $3, $4, $5, $6::date, $7)
		ON CONFLICT DO NOTHING`,
		tenant.String(), cycle.Account().String(), cycle.Kind().String(), weekday, monthDay, cycle.Since().String(), at.UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("save statement cycle: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.CatalogueRegistered, nil
	}
	stored, found, err := repository.loadCycle(ctx, executor, tenant, cycle.Account())
	if err != nil {
		return 0, fmt.Errorf("save statement cycle: %w", err)
	}
	if found && stored.SameCycle(cycle) {
		return ports.CatalogueReplay, nil
	}
	return ports.CatalogueConflict, nil
}

type cycleQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const statementCycleSelect = `
	SELECT tenant_id, account_id, cycle_kind, closing_weekday, closing_month_day, to_char(since_date, 'YYYY-MM-DD')
	  FROM settlement_accounting.statement_cycle`

func (repository *StatementCycles) loadCycle(
	ctx context.Context,
	querier cycleQuerier,
	tenant domain.TenantID,
	account domain.SettlementAccountID,
) (domain.StatementCycle, bool, error) {
	record, err := scanCycle(querier.QueryRow(ctx, statementCycleSelect+`
		 WHERE tenant_id = $1 AND account_id = $2`, tenant.String(), account.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StatementCycle{}, false, nil
	}
	if err != nil {
		return domain.StatementCycle{}, false, err
	}
	return record.Cycle, true, nil
}

func scanCycle(row pgx.Row) (ports.StatementCycleRecord, error) {
	var (
		tenantID, accountID, kindName, since string
		weekday, monthDay                    *int
	)
	if err := row.Scan(&tenantID, &accountID, &kindName, &weekday, &monthDay, &since); err != nil {
		return ports.StatementCycleRecord{}, err
	}
	tenant, err := domain.NewTenantID(tenantID)
	if err != nil {
		return ports.StatementCycleRecord{}, err
	}
	account, err := domain.NewSettlementAccountID(accountID)
	if err != nil {
		return ports.StatementCycleRecord{}, err
	}
	kind, err := domain.StatementCycleKindFromName(kindName)
	if err != nil {
		return ports.StatementCycleRecord{}, err
	}
	sinceDate, err := domain.ParseAttributionDate(since)
	if err != nil {
		return ports.StatementCycleRecord{}, err
	}
	var day *time.Weekday
	if weekday != nil {
		value := time.Weekday(*weekday)
		day = &value
	}
	cycle, err := domain.NewStatementCycle(account, kind, day, monthDay, sinceDate)
	if err != nil {
		return ports.StatementCycleRecord{}, err
	}
	return ports.StatementCycleRecord{Tenant: tenant, Cycle: cycle}, nil
}

// ListStatementCycles 交回册上全部租户的周期：到点截单在 dispatch 里跨租户扫。
func (repository *StatementCycles) ListStatementCycles(ctx context.Context) ([]ports.StatementCycleRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list statement cycles: %w", err)
	}
	rows, err := querier.Query(ctx, statementCycleSelect+` ORDER BY tenant_id, account_id`)
	if err != nil {
		return nil, fmt.Errorf("list statement cycles: %w", err)
	}
	defer rows.Close()
	var records []ports.StatementCycleRecord
	for rows.Next() {
		record, err := scanCycle(rows)
		if err != nil {
			return nil, fmt.Errorf("list statement cycles: %w", err)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// LastCycleCut 交回这一户最后截到的截止日。取行时加锁：两只 dispatch 进程同时扫到同一户，后到的那一只等前一只的事务结束再读，
// 不会把同一个截止日截两次。
func (repository *StatementCycles) LastCycleCut(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.SettlementAccountID,
) (domain.AttributionDate, bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return domain.AttributionDate{}, false, fmt.Errorf("last cycle cut: %w", err)
	}
	if _, err := executor.Exec(ctx, `
		SELECT 1 FROM settlement_accounting.statement_cycle
		 WHERE tenant_id = $1 AND account_id = $2 FOR UPDATE`, tenant.String(), account.String()); err != nil {
		return domain.AttributionDate{}, false, fmt.Errorf("last cycle cut: 锁周期行：%w", err)
	}
	var closing *string
	if err := executor.QueryRow(ctx, `
		SELECT to_char(max(closing_date), 'YYYY-MM-DD') FROM settlement_accounting.statement_cycle_cut
		 WHERE tenant_id = $1 AND account_id = $2`, tenant.String(), account.String()).Scan(&closing); err != nil {
		return domain.AttributionDate{}, false, fmt.Errorf("last cycle cut: %w", err)
	}
	if closing == nil {
		return domain.AttributionDate{}, false, nil
	}
	date, err := domain.ParseAttributionDate(*closing)
	if err != nil {
		return domain.AttributionDate{}, false, fmt.Errorf("last cycle cut: %w", err)
	}
	return date, true, nil
}

func (repository *StatementCycles) RecordCycleCut(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.SettlementAccountID,
	cut ports.StatementCycleCut,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record cycle cut: %w", err)
	}
	var statement *string
	if cut.Statement != "" {
		statement = &cut.Statement
	}
	if _, err := executor.Exec(ctx, `
		INSERT INTO settlement_accounting.statement_cycle_cut (
			tenant_id, account_id, closing_date, outcome, statement_number, cut_at
		) VALUES ($1, $2, $3::date, $4, $5, $6)`,
		tenant.String(), account.String(), cut.Closing.String(), cut.Outcome, statement, cut.CutAt.UTC(),
	); err != nil {
		return fmt.Errorf("record cycle cut: %w", err)
	}
	return nil
}

// ListCycleCutCandidates 见端口头注。「不在未作废的已发布单上」与截单出账锁（LockChargeBilling）同一判据。
func (repository *StatementCycles) ListCycleCutCandidates(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.SettlementAccountID,
	currency domain.CurrencyCode,
	through domain.AttributionDate,
) ([]ports.CycleCutCandidate, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cycle cut candidates: %w", err)
	}
	rows, err := executor.Query(ctx, `
		SELECT c.charge_id, c.settlement_minor
		  FROM settlement_accounting.customer_charge AS c
		 WHERE c.tenant_id = $1
		   AND c.stage = 'CONFIRMED'
		   AND c.settlement_account_id = $2
		   AND c.settlement_currency = $3
		   AND c.attribution_date <= $4::date
		   AND NOT EXISTS (
		       SELECT 1 FROM settlement_accounting.customer_statement AS s
		        WHERE s.tenant_id = c.tenant_id
		          AND s.voided_at IS NULL
		          AND s.lines @> jsonb_build_array(jsonb_build_object('charge', c.charge_id)))
		 ORDER BY c.charge_id`,
		tenant.String(), account.String(), currency.String(), through.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list cycle cut candidates: %w", err)
	}
	defer rows.Close()
	var candidates []ports.CycleCutCandidate
	for rows.Next() {
		var (
			chargeID string
			minor    int64
		)
		if err := rows.Scan(&chargeID, &minor); err != nil {
			return nil, fmt.Errorf("list cycle cut candidates: %w", err)
		}
		charge, err := domain.NewCustomerChargeID(chargeID)
		if err != nil {
			return nil, fmt.Errorf("list cycle cut candidates: %w", err)
		}
		candidates = append(candidates, ports.CycleCutCandidate{Charge: charge, Minor: minor})
	}
	return candidates, rows.Err()
}

func (repository *StatementCycles) ListUnattributedConfirmedCharges(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.SettlementAccountID,
) ([]domain.CustomerChargeID, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unattributed confirmed charges: %w", err)
	}
	rows, err := executor.Query(ctx, `
		SELECT charge_id FROM settlement_accounting.customer_charge
		 WHERE tenant_id = $1 AND stage = 'CONFIRMED' AND settlement_account_id = $2 AND attribution_date IS NULL
		 ORDER BY charge_id`, tenant.String(), account.String())
	if err != nil {
		return nil, fmt.Errorf("list unattributed confirmed charges: %w", err)
	}
	defer rows.Close()
	var charges []domain.CustomerChargeID
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("list unattributed confirmed charges: %w", err)
		}
		charge, err := domain.NewCustomerChargeID(raw)
		if err != nil {
			return nil, fmt.Errorf("list unattributed confirmed charges: %w", err)
		}
		charges = append(charges, charge)
	}
	return charges, rows.Err()
}
