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

// AmountGrammars 是限额、比例、免赔三项取值的登记册。空册答 found=false。
// 文法本身不在行上。
type AmountGrammars struct {
	db *bentopg.DB
}

func NewAmountGrammars(db *bentopg.DB) (*AmountGrammars, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &AmountGrammars{db: db}, nil
}

func (repository *AmountGrammars) SaveAmountGrammar(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.AmountGrammarRegistration,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save amount grammar: %w", err)
	}
	grammar := registration.Grammar()
	if _, err := executor.Exec(ctx, "SAVEPOINT amount_grammar_parameter"); err != nil {
		return 0, fmt.Errorf("save amount grammar: %w", err)
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.amount_grammar_parameter (
			tenant_id, subject_kind, subject_ref, limit_minor, ratio_basis_points,
			deductible_minor, registered_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tenant.String(),
		registration.Subject().String(),
		registration.Ref(),
		grammar.LimitMinor(),
		grammar.RatioBasisPoints(),
		grammar.DeductibleMinor(),
		at.UTC(),
	)
	if err == nil {
		return ports.CatalogueRegistered, nil
	}
	if !uniqueViolation(err) {
		return 0, fmt.Errorf("save amount grammar: %w", err)
	}
	if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT amount_grammar_parameter"); err != nil {
		return 0, fmt.Errorf("save amount grammar: %w", err)
	}
	loaded, found, err := scanAmountGrammar(executor.QueryRow(ctx, `
		SELECT limit_minor, ratio_basis_points, deductible_minor
		  FROM settlement_accounting.amount_grammar_parameter
		 WHERE tenant_id = $1 AND subject_kind = $2 AND subject_ref = $3`,
		tenant.String(), registration.Subject().String(), registration.Ref()))
	if err != nil {
		return 0, fmt.Errorf("save amount grammar: %w", err)
	}
	if !found || !registration.Grammar().Same(loaded) {
		return ports.CatalogueConflict, nil
	}
	return ports.CatalogueReplay, nil
}

func (repository *AmountGrammars) LoadAmountGrammar(
	ctx context.Context,
	tenant domain.TenantID,
	subject domain.AmountGrammarSubject,
	ref string,
) (domain.AmountGrammar, bool, error) {
	return repository.load(ctx, tenant, subject, ref)
}

func (repository *AmountGrammars) load(
	ctx context.Context,
	tenant domain.TenantID,
	subject domain.AmountGrammarSubject,
	ref string,
) (domain.AmountGrammar, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.AmountGrammar{}, false, fmt.Errorf("load amount grammar: %w", err)
	}
	grammar, found, err := scanAmountGrammar(querier.QueryRow(ctx, `
		SELECT limit_minor, ratio_basis_points, deductible_minor
		  FROM settlement_accounting.amount_grammar_parameter
		 WHERE tenant_id = $1 AND subject_kind = $2 AND subject_ref = $3`,
		tenant.String(), subject.String(), ref))
	if err != nil {
		return domain.AmountGrammar{}, false, fmt.Errorf("load amount grammar: %w", err)
	}
	return grammar, found, nil
}

func scanAmountGrammar(row pgx.Row) (domain.AmountGrammar, bool, error) {
	var limit, ratio, deductible int64
	err := row.Scan(&limit, &ratio, &deductible)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AmountGrammar{}, false, nil
	}
	if err != nil {
		return domain.AmountGrammar{}, false, err
	}
	grammar, err := domain.NewAmountGrammar(limit, ratio, deductible)
	if err != nil {
		return domain.AmountGrammar{}, false, err
	}
	return grammar, true, nil
}
