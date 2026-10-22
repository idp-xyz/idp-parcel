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

// StatementArrangements 是结算账户的对账安排登记册：按（租户、结算账户）排版次，读口取最新一版。空册答 found=false。
type StatementArrangements struct {
	db *bentopg.DB
}

func NewStatementArrangements(db *bentopg.DB) (*StatementArrangements, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &StatementArrangements{db: db}, nil
}

var (
	_ ports.StatementArrangementRegister = (*StatementArrangements)(nil)
	_ ports.StatementArrangementView     = (*StatementArrangements)(nil)
)

// SaveStatementArrangement 追加一版。与最新一版同内容是重放；否则落下一版，旧版留在册上——已形成的归属日按
// 它形成时那一版落日，不随换版回改（ADR-0269 决定一）。与更早某一版同内容也落新一版：换回去是一次真的
// 换版，按「与某一版相同即重放」判，册上最新一版就不是登记人刚登的那一份。两拍并发追加同一版次，后到者
// 撞主键，回滚到保存点后按提交后的册再判一次。账户不在册上由外键拒，译成 ErrCatalogueTargetMissing。
func (repository *StatementArrangements) SaveStatementArrangement(
	ctx context.Context,
	tenant domain.TenantID,
	arrangement domain.StatementArrangement,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save statement arrangement: %w", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		effect, retry, err := repository.saveOnce(ctx, executor, tenant, arrangement, at)
		if err != nil {
			return 0, fmt.Errorf("save statement arrangement: %w", err)
		}
		if !retry {
			return effect, nil
		}
	}
	return 0, fmt.Errorf("save statement arrangement: concurrent revisions of account %s kept colliding", arrangement.Account())
}

func (repository *StatementArrangements) saveOnce(
	ctx context.Context,
	executor bentopg.Executor,
	tenant domain.TenantID,
	arrangement domain.StatementArrangement,
	at time.Time,
) (ports.CatalogueRegistrationEffect, bool, error) {
	latest, revision, found, err := latestStatementArrangement(ctx, executor, tenant, arrangement.Account())
	if err != nil {
		return 0, false, err
	}
	if found && latest.SameArrangement(arrangement) {
		return ports.CatalogueReplay, false, nil
	}

	if _, err := executor.Exec(ctx, "SAVEPOINT statement_arrangement"); err != nil {
		return 0, false, err
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.statement_arrangement (
			tenant_id, account_id, revision, timezone_name, cutoff_minute, registered_at
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		tenant.String(),
		arrangement.Account().String(),
		revision+1,
		arrangement.TimeZone(),
		arrangement.CutoffMinute(),
		at.UTC(),
	)
	if err == nil {
		return ports.CatalogueRegistered, false, nil
	}
	missing, duplicate := foreignKeyViolation(err), uniqueViolation(err)
	if !missing && !duplicate {
		return 0, false, err
	}
	if _, rollbackErr := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT statement_arrangement"); rollbackErr != nil {
		return 0, false, rollbackErr
	}
	if missing {
		return 0, false, domain.ErrCatalogueTargetMissing
	}
	return 0, true, nil
}

// LoadStatementArrangement 交回这一户最新的那一版。
func (repository *StatementArrangements) LoadStatementArrangement(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.SettlementAccountID,
) (domain.StatementArrangement, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.StatementArrangement{}, false, fmt.Errorf("load statement arrangement: %w", err)
	}
	arrangement, _, found, err := latestStatementArrangement(ctx, querier, tenant, account)
	if err != nil {
		return domain.StatementArrangement{}, false, fmt.Errorf("load statement arrangement: %w", err)
	}
	return arrangement, found, nil
}

// latestStatementArrangement 读最新一版连同版次。读回过同一个构造门，库里一格坏值报错，不折成「没有这一版」。
func latestStatementArrangement(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	account domain.SettlementAccountID,
) (domain.StatementArrangement, int, bool, error) {
	var (
		zone     string
		cutoff   int
		revision int
	)
	err := querier.QueryRow(ctx, `
		SELECT timezone_name, cutoff_minute, revision
		  FROM settlement_accounting.statement_arrangement
		 WHERE tenant_id = $1 AND account_id = $2
		 ORDER BY revision DESC LIMIT 1`,
		tenant.String(), account.String()).Scan(&zone, &cutoff, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StatementArrangement{}, 0, false, nil
	}
	if err != nil {
		return domain.StatementArrangement{}, 0, false, err
	}
	arrangement, err := domain.NewStatementArrangement(account, zone, cutoff)
	if err != nil {
		return domain.StatementArrangement{}, 0, false, err
	}
	return arrangement, revision, true, nil
}
