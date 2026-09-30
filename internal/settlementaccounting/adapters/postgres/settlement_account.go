package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// SettlementAccounts 是结算账户登记册。写入在环境事务里；读口可以在事务外。
type SettlementAccounts struct {
	db *bentopg.DB
}

func NewSettlementAccounts(db *bentopg.DB) (*SettlementAccounts, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &SettlementAccounts{db: db}, nil
}

func (repository *SettlementAccounts) Save(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.SettlementAccount,
	at time.Time,
) (ports.SettlementAccountRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save settlement account: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT settlement_account_insert"); err != nil {
		return 0, fmt.Errorf("save settlement account: %w", err)
	}
	payer, distinct := account.Payer()
	var payerArg any
	if distinct {
		payerArg = payer.String()
	}
	key := account.Key()
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.settlement_account (
			tenant_id, account_id, legal_entity_id, counterparty_id, direction, currency,
			settlement_policy_id, payer_id, responsibility_basis, registered_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10
		)`,
		tenant.String(),
		account.ID().String(),
		key.LegalEntity().String(),
		key.Counterparty().String(),
		key.Direction().String(),
		key.Currency().String(),
		key.Policy().String(),
		payerArg,
		account.Responsibility().String(),
		at.UTC(),
	)
	if err == nil {
		return ports.SettlementAccountRegistered, nil
	}
	if !uniqueViolation(err) {
		return 0, fmt.Errorf("save settlement account: %w", err)
	}
	if _, rollbackErr := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT settlement_account_insert"); rollbackErr != nil {
		return 0, fmt.Errorf("save settlement account: %w", rollbackErr)
	}
	effect, classifyErr := repository.classify(ctx, executor, tenant, account)
	if classifyErr != nil {
		return 0, fmt.Errorf("save settlement account: %w", classifyErr)
	}
	return effect, nil
}

func (repository *SettlementAccounts) Find(
	ctx context.Context,
	tenant domain.TenantID,
	key domain.SettlementAccountKey,
) (domain.SettlementAccountID, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SettlementAccountID{}, false, fmt.Errorf("find settlement account: %w", err)
	}
	var accountID string
	err = querier.QueryRow(ctx, `
		SELECT account_id
		  FROM settlement_accounting.settlement_account
		 WHERE tenant_id = $1
		   AND legal_entity_id = $2
		   AND counterparty_id = $3
		   AND direction = $4
		   AND currency = $5
		   AND settlement_policy_id = $6`,
		tenant.String(),
		key.LegalEntity().String(),
		key.Counterparty().String(),
		key.Direction().String(),
		key.Currency().String(),
		key.Policy().String(),
	).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SettlementAccountID{}, false, nil
	}
	if err != nil {
		return domain.SettlementAccountID{}, false, fmt.Errorf("find settlement account: %w", err)
	}
	id, err := domain.NewSettlementAccountID(accountID)
	if err != nil {
		return domain.SettlementAccountID{}, false, fmt.Errorf("find settlement account: %w", err)
	}
	return id, true, nil
}

func (repository *SettlementAccounts) classify(
	ctx context.Context,
	querier interface {
		QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	},
	tenant domain.TenantID,
	account domain.SettlementAccount,
) (ports.SettlementAccountRegistrationEffect, error) {
	stored, found, err := scanAccount(querier.QueryRow(ctx, settlementAccountSelect+`
		 WHERE tenant_id = $1 AND account_id = $2`,
		tenant.String(), account.ID().String()))
	if err != nil {
		return 0, err
	}
	if found {
		if stored.SameRegistration(account) {
			return ports.SettlementAccountReplay, nil
		}
		return ports.SettlementAccountConflict, nil
	}
	key := account.Key()
	stored, found, err = scanAccount(querier.QueryRow(ctx, settlementAccountSelect+`
		 WHERE tenant_id = $1
		   AND legal_entity_id = $2
		   AND counterparty_id = $3
		   AND direction = $4
		   AND currency = $5
		   AND settlement_policy_id = $6`,
		tenant.String(),
		key.LegalEntity().String(),
		key.Counterparty().String(),
		key.Direction().String(),
		key.Currency().String(),
		key.Policy().String(),
	))
	if err != nil {
		return 0, err
	}
	if found {
		return ports.SettlementAccountConflict, nil
	}
	return ports.SettlementAccountConflict, nil
}

const settlementAccountSelect = `
	SELECT account_id, legal_entity_id, counterparty_id, direction, currency, settlement_policy_id,
	       payer_id, responsibility_basis
	  FROM settlement_accounting.settlement_account`

func scanAccount(row pgx.Row) (domain.SettlementAccount, bool, error) {
	var (
		accountID, legalEntity, counterparty, direction, currency, policy string
		payer                                                             *string
		basis                                                             string
	)
	err := row.Scan(
		&accountID, &legalEntity, &counterparty, &direction, &currency, &policy,
		&payer, &basis,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SettlementAccount{}, false, nil
	}
	if err != nil {
		return domain.SettlementAccount{}, false, err
	}
	id, err := domain.NewSettlementAccountID(accountID)
	if err != nil {
		return domain.SettlementAccount{}, false, err
	}
	entity, err := domain.NewLegalEntityReference(legalEntity)
	if err != nil {
		return domain.SettlementAccount{}, false, err
	}
	party, err := domain.NewSettlementCounterpartyReference(counterparty)
	if err != nil {
		return domain.SettlementAccount{}, false, err
	}
	parsedDirection, err := domain.ChargeDirectionFromName(direction)
	if err != nil {
		return domain.SettlementAccount{}, false, err
	}
	parsedCurrency, err := domain.NewCurrencyCode(currency)
	if err != nil {
		return domain.SettlementAccount{}, false, err
	}
	parsedPolicy, err := domain.NewSettlementPolicyReference(policy)
	if err != nil {
		return domain.SettlementAccount{}, false, err
	}
	key, err := domain.NewSettlementAccountKey(entity, party, parsedDirection, parsedCurrency, parsedPolicy)
	if err != nil {
		return domain.SettlementAccount{}, false, err
	}
	var payerRef domain.SettlementCounterpartyReference
	payerDistinct := payer != nil
	if payerDistinct {
		payerRef, err = domain.NewSettlementCounterpartyReference(*payer)
		if err != nil {
			return domain.SettlementAccount{}, false, err
		}
	}
	responsibility, err := domain.NewResponsibilityBasis(basis)
	if err != nil {
		return domain.SettlementAccount{}, false, err
	}
	account, err := domain.NewSettlementAccount(id, key, payerRef, payerDistinct, responsibility)
	if err != nil {
		return domain.SettlementAccount{}, false, err
	}
	return account, true, nil
}

func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
