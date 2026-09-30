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

// SettlementCatalogues 是四本结算读口登记册。写入在环境事务里；读口可以在事务外。
// 空册答 found=false，不默认放行、不虚构授权人、不从身份推导账户。
type SettlementCatalogues struct {
	db *bentopg.DB
}

func NewSettlementCatalogues(db *bentopg.DB) (*SettlementCatalogues, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &SettlementCatalogues{db: db}, nil
}

func (repository *SettlementCatalogues) SaveSupplierAuditAuthority(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.SupplierAuditAuthorityRegistration,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	return repository.save(ctx, "supplier_audit_authority", `
		INSERT INTO settlement_accounting.supplier_audit_authority (
			tenant_id, supplier_id, legal_entity_id, auditor_id, registered_at
		) VALUES ($1, $2, $3, $4, $5)`,
		[]any{
			tenant.String(), registration.Supplier().String(), registration.LegalEntity().String(),
			registration.Auditor().String(), at.UTC(),
		},
		func(ctx context.Context, querier catalogueQuerier) (bool, error) {
			var auditor string
			err := querier.QueryRow(ctx, `
				SELECT auditor_id
				  FROM settlement_accounting.supplier_audit_authority
				 WHERE tenant_id = $1 AND supplier_id = $2 AND legal_entity_id = $3`,
				tenant.String(), registration.Supplier().String(), registration.LegalEntity().String(),
			).Scan(&auditor)
			if errors.Is(err, pgx.ErrNoRows) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			return auditor == registration.Auditor().String(), nil
		},
	)
}

func (repository *SettlementCatalogues) LoadSupplierAuditAuthority(
	ctx context.Context,
	tenant domain.TenantID,
	supplier domain.SupplierPartyReference,
	legalEntity domain.LegalEntityReference,
) (domain.AuditorReference, bool, error) {
	var auditor string
	found, err := repository.load(ctx, `
		SELECT auditor_id
		  FROM settlement_accounting.supplier_audit_authority
		 WHERE tenant_id = $1 AND supplier_id = $2 AND legal_entity_id = $3`,
		[]any{tenant.String(), supplier.String(), legalEntity.String()}, &auditor)
	if err != nil || !found {
		return domain.AuditorReference{}, found, err
	}
	parsed, err := domain.NewAuditorReference(auditor)
	if err != nil {
		return domain.AuditorReference{}, false, fmt.Errorf("load supplier audit authority: %w", err)
	}
	return parsed, true, nil
}

func (repository *SettlementCatalogues) SaveSupplierPayableAccount(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.SupplierPayableAccountRegistration,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	return repository.save(ctx, "supplier_payable_account", `
		INSERT INTO settlement_accounting.supplier_payable_account (
			tenant_id, supplier_id, legal_entity_id, currency, account_id, registered_at
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		[]any{
			tenant.String(), registration.Supplier().String(), registration.LegalEntity().String(),
			registration.Currency().String(), registration.Account().String(), at.UTC(),
		},
		func(ctx context.Context, querier catalogueQuerier) (bool, error) {
			var account string
			err := querier.QueryRow(ctx, `
				SELECT account_id
				  FROM settlement_accounting.supplier_payable_account
				 WHERE tenant_id = $1 AND supplier_id = $2 AND legal_entity_id = $3 AND currency = $4`,
				tenant.String(), registration.Supplier().String(), registration.LegalEntity().String(),
				registration.Currency().String(),
			).Scan(&account)
			if errors.Is(err, pgx.ErrNoRows) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			return account == registration.Account().String(), nil
		},
	)
}

func (repository *SettlementCatalogues) LoadSupplierPayableAccount(
	ctx context.Context,
	tenant domain.TenantID,
	supplier domain.SupplierPartyReference,
	legalEntity domain.LegalEntityReference,
	currency domain.CurrencyCode,
) (domain.SettlementAccountID, bool, error) {
	var account string
	found, err := repository.load(ctx, `
		SELECT account_id
		  FROM settlement_accounting.supplier_payable_account
		 WHERE tenant_id = $1 AND supplier_id = $2 AND legal_entity_id = $3 AND currency = $4`,
		[]any{tenant.String(), supplier.String(), legalEntity.String(), currency.String()}, &account)
	if err != nil || !found {
		return domain.SettlementAccountID{}, found, err
	}
	parsed, err := domain.NewSettlementAccountID(account)
	if err != nil {
		return domain.SettlementAccountID{}, false, fmt.Errorf("load supplier payable account: %w", err)
	}
	return parsed, true, nil
}

func (repository *SettlementCatalogues) SaveClaimAmountRule(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.ClaimAmountRuleRegistration,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	return repository.save(ctx, "claim_amount_rule", `
		INSERT INTO settlement_accounting.claim_amount_rule (
			tenant_id, responsibility_conclusion_id, rule_version, registered_at
		) VALUES ($1, $2, $3, $4)`,
		[]any{
			tenant.String(), registration.Responsibility().String(), registration.Rule().String(), at.UTC(),
		},
		func(ctx context.Context, querier catalogueQuerier) (bool, error) {
			var rule string
			err := querier.QueryRow(ctx, `
				SELECT rule_version
				  FROM settlement_accounting.claim_amount_rule
				 WHERE tenant_id = $1 AND responsibility_conclusion_id = $2`,
				tenant.String(), registration.Responsibility().String(),
			).Scan(&rule)
			if errors.Is(err, pgx.ErrNoRows) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			return rule == registration.Rule().String(), nil
		},
	)
}

func (repository *SettlementCatalogues) LoadClaimAmountRule(
	ctx context.Context,
	tenant domain.TenantID,
	responsibility domain.ResponsibilityConclusionReference,
) (domain.AmountRuleVersionReference, bool, error) {
	var rule string
	found, err := repository.load(ctx, `
		SELECT rule_version
		  FROM settlement_accounting.claim_amount_rule
		 WHERE tenant_id = $1 AND responsibility_conclusion_id = $2`,
		[]any{tenant.String(), responsibility.String()}, &rule)
	if err != nil || !found {
		return domain.AmountRuleVersionReference{}, found, err
	}
	parsed, err := domain.NewAmountRuleVersionReference(rule)
	if err != nil {
		return domain.AmountRuleVersionReference{}, false, fmt.Errorf("load claim amount rule: %w", err)
	}
	return parsed, true, nil
}

func (repository *SettlementCatalogues) SaveChargeConfirmationFacts(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.ChargeConfirmationFactRegistration,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	facts := registration.Facts()
	return repository.save(ctx, "charge_confirmation_fact", `
		INSERT INTO settlement_accounting.charge_confirmation_fact (
			tenant_id, charge_id, responsible_entity, counterparty_ref, charge_direction,
			settlement_account_id, contract_basis, primary_charging_scope, source_fact_ref, registered_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		[]any{
			tenant.String(), registration.Charge().String(), facts.ResponsibleEntity.String(),
			facts.Counterparty.String(), facts.Direction.String(), facts.SettlementAccount.String(),
			facts.ContractBasis.String(), facts.PrimaryChargingScope.String(), facts.SourceFact.String(),
			at.UTC(),
		},
		func(ctx context.Context, querier catalogueQuerier) (bool, error) {
			stored, found, err := scanChargeConfirmationFacts(querier.QueryRow(ctx, `
				SELECT responsible_entity, counterparty_ref, charge_direction, settlement_account_id,
				       contract_basis, primary_charging_scope, source_fact_ref
				  FROM settlement_accounting.charge_confirmation_fact
				 WHERE tenant_id = $1 AND charge_id = $2`,
				tenant.String(), registration.Charge().String()))
			if err != nil || !found {
				return false, err
			}
			other, err := domain.NewChargeConfirmationFactRegistration(registration.Charge(), stored)
			if err != nil {
				return false, err
			}
			return registration.SameRegistration(other), nil
		},
	)
}

func (repository *SettlementCatalogues) LoadConfirmedChargeFacts(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerChargeID,
) (domain.ConfirmedChargeFacts, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ConfirmedChargeFacts{}, false, fmt.Errorf("load charge confirmation facts: %w", err)
	}
	facts, found, err := scanChargeConfirmationFacts(querier.QueryRow(ctx, `
		SELECT responsible_entity, counterparty_ref, charge_direction, settlement_account_id,
		       contract_basis, primary_charging_scope, source_fact_ref
		  FROM settlement_accounting.charge_confirmation_fact
		 WHERE tenant_id = $1 AND charge_id = $2`,
		tenant.String(), charge.String()))
	if err != nil {
		return domain.ConfirmedChargeFacts{}, false, fmt.Errorf("load charge confirmation facts: %w", err)
	}
	return facts, found, nil
}

type catalogueQuerier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type catalogueSame func(ctx context.Context, querier catalogueQuerier) (bool, error)

func (repository *SettlementCatalogues) save(
	ctx context.Context,
	name, statement string,
	args []any,
	same catalogueSame,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save %s: %w", name, err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT "+name); err != nil {
		return 0, fmt.Errorf("save %s: %w", name, err)
	}
	if _, err := executor.Exec(ctx, statement, args...); err == nil {
		return ports.CatalogueRegistered, nil
	} else if foreignKeyViolation(err) {
		_, _ = executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+name)
		return 0, fmt.Errorf("save %s: %w", name, domain.ErrCatalogueTargetMissing)
	} else if !uniqueViolation(err) {
		return 0, fmt.Errorf("save %s: %w", name, err)
	}
	if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+name); err != nil {
		return 0, fmt.Errorf("save %s: %w", name, err)
	}
	matches, err := same(ctx, executor)
	if err != nil {
		return 0, fmt.Errorf("save %s: %w", name, err)
	}
	if !matches {
		return ports.CatalogueConflict, nil
	}
	return ports.CatalogueReplay, nil
}

func (repository *SettlementCatalogues) load(
	ctx context.Context,
	statement string,
	args []any,
	dest *string,
) (bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return false, err
	}
	err = querier.QueryRow(ctx, statement, args...).Scan(dest)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func scanChargeConfirmationFacts(row pgx.Row) (domain.ConfirmedChargeFacts, bool, error) {
	var entity, counterparty, direction, account, basis, scope, source string
	err := row.Scan(&entity, &counterparty, &direction, &account, &basis, &scope, &source)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ConfirmedChargeFacts{}, false, nil
	}
	if err != nil {
		return domain.ConfirmedChargeFacts{}, false, err
	}
	parsedDirection, err := domain.ChargeDirectionFromName(direction)
	if err != nil {
		return domain.ConfirmedChargeFacts{}, false, err
	}
	responsible, err := domain.NewLegalEntityReference(entity)
	if err != nil {
		return domain.ConfirmedChargeFacts{}, false, err
	}
	party, err := domain.NewSettlementCounterpartyReference(counterparty)
	if err != nil {
		return domain.ConfirmedChargeFacts{}, false, err
	}
	accountID, err := domain.NewSettlementAccountID(account)
	if err != nil {
		return domain.ConfirmedChargeFacts{}, false, err
	}
	contract, err := domain.NewContractBasisReference(basis)
	if err != nil {
		return domain.ConfirmedChargeFacts{}, false, err
	}
	chargingScope, err := domain.NewChargingScopeReference(scope)
	if err != nil {
		return domain.ConfirmedChargeFacts{}, false, err
	}
	sourceFact, err := domain.NewSourceFactReference(source)
	if err != nil {
		return domain.ConfirmedChargeFacts{}, false, err
	}
	return domain.ConfirmedChargeFacts{
		ResponsibleEntity:    responsible,
		Counterparty:         party,
		Direction:            parsedDirection,
		SettlementAccount:    accountID,
		ContractBasis:        contract,
		PrimaryChargingScope: chargingScope,
		SourceFact:           sourceFact,
	}, true, nil
}

func foreignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
